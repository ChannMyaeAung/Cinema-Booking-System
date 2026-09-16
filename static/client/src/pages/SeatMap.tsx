import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useClerk, useUser } from "@clerk/react";
import { ApiError, type SeatInfo } from "../api/api";
import {
  useAdminCancel,
  useAdminConfirm,
  useCreateCheckout,
  useHoldSeat,
  useMovies,
  useReleaseSession,
  useSeats,
} from "../api/queries";
import { holdFromResponse, useSession } from "../session-context";
import { useToast } from "../toast-context";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Separator } from "@/components/ui/separator";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ArrowLeft, PartyPopper, Timer, X } from "lucide-react";

type SeatState = "available" | "selected" | "held" | "booked";

interface PendingCheckout {
  movieID: string;
  sessionIDs: string[];
  seats: string[];
  expiresAt: number;
}

const CHECKOUT_HOLD_MS = 30 * 60 * 1000;
const PENDING_CHECKOUT_KEY = "cinema.pendingCheckout";

function formatCountdown(ms: number): string {
  const total = Math.max(0, Math.ceil(ms / 1000));
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

const seatStateStyles: Record<SeatState, string> = {
  available: "border-ok/50 bg-surface-2 text-ok hover:border-ok hover:bg-ok/15",
  selected:
    "border-primary bg-primary text-primary-foreground shadow-[0_0_18px_-4px] shadow-primary/70",
  held: "border-[#ff9d3c]/50 bg-[#ff9d3c]/10 text-[#ff9d3c] cursor-not-allowed",
  booked: "border-transparent bg-white/[0.05] text-white/30 cursor-not-allowed",
};

function SeatButton({
  seat,
  state,
  disabled,
  onClick,
}: {
  seat: SeatInfo;
  state: SeatState;
  disabled?: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      className={cn(
        "grid h-7.5 w-8.5 place-items-center rounded-[6px_6px_3px_3px] border text-xs font-semibold transition-all hover:-translate-y-0.5 max-[560px]:h-6.5 max-[560px]:w-7",
        seatStateStyles[state],
      )}
      onClick={onClick}
      disabled={disabled}
      aria-label={`Seat ${seat.seat_id}, ${state}`}
      aria-pressed={state === "selected"}
    >
      {seat.seat_id.slice(1)}
    </button>
  );
}

export default function SeatMap() {
  const { movieID = "" } = useParams<{ movieID: string }>();
  const navigate = useNavigate();
  const { userID, activeHolds, addHold, removeHold } = useSession();
  const { push } = useToast();
  const { openSignIn } = useClerk();
  const { user } = useUser();

  // check with stripe public metadata if user is admin
  const isAdmin = user?.publicMetadata?.role === "admin";
  const [now, setNow] = useState(() => Date.now());
  const [confirmedSeats, setConfirmedSeats] = useState<string[] | null>(null);

  const moviesQuery = useMovies();
  const seatsQuery = useSeats(movieID);
  const holdMutation = useHoldSeat();
  const checkoutMutation = useCreateCheckout();
  const releaseMutation = useReleaseSession();
  const confirmMutation = useAdminConfirm();
  const cancelMutation = useAdminCancel();

  const movie = moviesQuery.data?.find((m) => m.id === movieID);

  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, []);

  useEffect(() => {
    if (confirmedSeats === null) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") closeDialog();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [confirmedSeats]);

  const holdsForMovie = useMemo(
    () => activeHolds.filter((h) => h.movieID === movieID),
    [activeHolds, movieID],
  );

  const rows = useMemo(() => {
    const map = new Map<string, SeatInfo[]>();
    for (const seat of seatsQuery.data ?? []) {
      const row = seat.seat_id.charAt(0);
      if (!map.has(row)) map.set(row, []);
      map.get(row)!.push(seat);
    }
    return [...map.entries()];
  }, [seatsQuery.data]);

  const handlesRef = useRef(new Set<string>());
  useEffect(() => {
    for (const hold of holdsForMovie) {
      if (hold.expiresAt <= now && !handlesRef.current.has(hold.sessionID)) {
        handlesRef.current.add(hold.sessionID);
        removeHold(hold.sessionID);
        push(`Hold on ${hold.seatID} expired`, "info");
        void releaseMutation
          .mutateAsync({ movieID, sessionID: hold.sessionID })
          .catch(() => {});
      }
    }
  }, [now, holdsForMovie, movieID, userID, removeHold, push, releaseMutation]);

  function getState(seat: SeatInfo): SeatState {
    if (
      holdMutation.isPending &&
      holdMutation.variables?.seatID === seat.seat_id
    )
      return "selected";
    if (holdsForMovie.some((h) => h.seatID === seat.seat_id)) return "selected";
    if (seat.booked && seat.confirmed) return "booked";
    if (seat.booked && seat.user_id === userID) return "selected";
    if (seat.booked) return "held";
    return "available";
  }

  function handleAdminVoid(seat: SeatInfo) {
    if (!seat.session_id) return;
    const ok = window.confirm(
      `Void the booking on seat ${seat.seat_id}? The seat becomes free again.`,
    );
    if (!ok) return;
    cancelMutation.mutate(
      { movieID, sessionID: seat.session_id },
      {
        onSuccess: () =>
          push(`Booking on ${seat.seat_id} cancelled`, "success"),
        onError: () =>
          push(`Could not cancel booking on ${seat.seat_id}`, "error"),
      },
    );
  }

  function handleSeatClick(seat: SeatInfo) {
    if (isAdmin && seat.booked && seat.confirmed) {
      handleAdminVoid(seat);
      return;
    }

    const state = getState(seat);
    if (state === "booked" || state === "held") return;

    if (!userID) {
      push("Sign in to book a seat", "info");
      openSignIn();
      return;
    }

    if (state === "selected") {
      const hold = holdsForMovie.find((h) => h.seatID === seat.seat_id);
      if (hold) {
        removeHold(hold.sessionID);
        releaseMutation.mutate(
          { movieID, sessionID: hold.sessionID },
          {
            onError: () => push(`Could not release ${seat.seat_id}`, "error"),
          },
        );
      }
      return;
    }

    if (holdMutation.isPending) return;

    holdMutation.mutate(
      { movieID, seatID: seat.seat_id },
      {
        onSuccess: (res) => {
          addHold(holdFromResponse(res));
          push(`${seat.seat_id} held`, "success");
        },
        onError: (err) => {
          if (err instanceof ApiError && err.status === 409) {
            push(`Seat ${seat.seat_id} was just taken`, "error");
          } else {
            push(`Could not hold ${seat.seat_id}`, "error");
          }
        },
      },
    );
  }

  const remaining = holdsForMovie.length
    ? Math.min(...holdsForMovie.map((h) => h.expiresAt - now))
    : 0;
  const checkoutPending =
    checkoutMutation.isPending ||
    holdMutation.isPending ||
    confirmMutation.isPending;

  const pendingCheckoutRef = useRef<PendingCheckout | null>(null);

  useEffect(() => {
    const raw = sessionStorage.getItem(PENDING_CHECKOUT_KEY);
    if (!raw) return;
    try {
      const pc = JSON.parse(raw) as PendingCheckout;
      if (pc.movieID === movieID) pendingCheckoutRef.current = pc;
    } catch {
      sessionStorage.removeItem(PENDING_CHECKOUT_KEY);
    }
  }, [movieID]);

  async function handleCheckout() {
    if (checkoutPending || holdsForMovie.length === 0) return;

    const sessionIDs = holdsForMovie.map((h) => h.sessionID);
    const seats = holdsForMovie.map((h) => h.seatID);
    const returnURL = window.location.href;

    const pc: PendingCheckout = {
      movieID,
      sessionIDs,
      seats,
      expiresAt: Date.now() + CHECKOUT_HOLD_MS,
    };
    sessionStorage.setItem(PENDING_CHECKOUT_KEY, JSON.stringify(pc));

    for (const hold of holdsForMovie) {
      addHold({ ...hold, expiresAt: pc.expiresAt });
    }

    checkoutMutation.mutate(
      { movieID, sessionIDs, successURL: returnURL, cancelURL: returnURL },
      {
        onSuccess: ({ res }) => {
          window.location.href = res.url;
        },
        onError: () => {
          sessionStorage.removeItem(PENDING_CHECKOUT_KEY);
          pendingCheckoutRef.current = null;
          push("Could not start checkout", "error");
        },
      },
    );
  }

  function handleAdminConfirm() {
    if (checkoutPending || holdsForMovie.length === 0) return;

    const sessionIDs = holdsForMovie.map((h) => h.sessionID);
    confirmMutation.mutate(
      { movieID, sessionIDs },
      {
        onSuccess: ({ confirmed }) => {
          const seats = confirmed.map((c) => c.seat_id);
          for (const hold of holdsForMovie) removeHold(hold.sessionID);
          setConfirmedSeats(seats);
          push("Booking confirmed — paid at counter", "success");
        },
        onError: () => {
          push("Could not confirm booking", "error");
        },
      },
    );
  }

  useEffect(() => {
    const pc = pendingCheckoutRef.current;
    if (!pc || pc.expiresAt <= Date.now() || !seatsQuery.data) return;

    const confirmed = pc.seats.filter((seatID) => {
      const seat = seatsQuery.data?.find((s) => s.seat_id === seatID);
      return seat?.booked && seat.confirmed && seat.user_id === userID;
    });
    if (confirmed.length === 0 || confirmed.length !== pc.seats.length) return;

    sessionStorage.removeItem(PENDING_CHECKOUT_KEY);
    pendingCheckoutRef.current = null;
    for (const sessionID of pc.sessionIDs) removeHold(sessionID);
    setConfirmedSeats(confirmed);
    push("Payment received - seats confirmed", "success");
  }, [seatsQuery.data, movieID, userID, removeHold, push]);

  function closeDialog() {
    setConfirmedSeats(null);
  }

  return (
    <section>
      <Link
        to="/"
        className="mb-5 inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
      >
        <ArrowLeft className="size-4" />
        All movies
      </Link>

      <h1 className="font-heading text-3xl font-bold tracking-tight">
        {movie?.title ?? "Select seats"}
      </h1>
      <p className="mt-1 text-muted-foreground">
        {isAdmin
          ? "Staff mode: tap a free seat to hold it, then confirm at the counter — no card payment needed. Tap a booked seat to void it."
          : "Tap a free seat to hold it. You have 2 min to pay. Once paid, your seat is locked in."}
      </p>

      {seatsQuery.isPending && (
        <div className="mt-8" aria-hidden="true">
          <div className="mx-auto max-w-[620px]">
            <div className="mx-auto mb-7 h-11 w-full rounded-[50%_50%_4px_4px] bg-primary/20" />
            <div className="mb-6 flex flex-col gap-2">
              {Array.from({ length: 5 }).map((_, r) => (
                <div className="flex items-center justify-center gap-2" key={r}>
                  {Array.from({ length: 8 }).map((_, c) => (
                    <Skeleton
                      key={c}
                      className="h-[30px] w-[34px] rounded-[6px]"
                    />
                  ))}
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {seatsQuery.isError && (
        <div
          role="alert"
          className="mt-8 rounded-xl border border-destructive/40 bg-destructive/10 p-4 text-sm text-destructive"
        >
          Could not load the seat map. Is the server running?
        </div>
      )}

      {seatsQuery.data && (
        <>
          <div className="mx-auto mt-8 max-w-155">
            <div
              className="mx-auto mb-7 grid h-11 w-full place-items-center rounded-[50%_50%_4px_4px] bg-linear-to-b from-primary/40 to-primary/5 text-[0.7rem] uppercase tracking-[0.3em] text-white/70 shadow-[0_-10px_40px_-8px] shadow-primary/20"
              aria-hidden="true"
            >
              Screen
            </div>

            <div
              className="mb-6 flex flex-col gap-2"
              role="grid"
              aria-label="Seat map"
            >
              {rows.map(([label, seats]) => (
                <div
                  className="flex items-center justify-center gap-2"
                  role="row"
                  key={label}
                >
                  <span
                    className="w-5 text-center text-[0.8rem] font-semibold text-muted-foreground"
                    aria-hidden="true"
                  >
                    {label}
                  </span>
                  {seats.map((seat) => {
                    const state = getState(seat);
                    return (
                      <SeatButton
                        key={seat.seat_id}
                        seat={seat}
                        state={state}
                        disabled={
                          state === "held" || (state === "booked" && !isAdmin)
                        }
                        onClick={() => handleSeatClick(seat)}
                      />
                    );
                  })}
                </div>
              ))}
            </div>

            <div
              className="mb-10 flex flex-wrap items-center justify-center gap-5 text-[0.8rem] text-muted-foreground"
              aria-label="Legend"
            >
              {(
                [
                  ["available", "Available"],
                  ["selected", "Selected"],
                  ["held", "Held by others"],
                  ["booked", "Booked"],
                ] as const
              ).map(([state, label]) => (
                <span className="inline-flex items-center gap-2" key={state}>
                  <span
                    className={cn(
                      "inline-block h-3.5 w-4 rounded-[4px] border",
                      seatStateStyles[state],
                    )}
                  />
                  {label}
                </span>
              ))}
            </div>
          </div>

          {holdsForMovie.length > 0 && (
            <div className="fixed inset-x-0 bottom-4 z-30 px-4">
              <div className="mx-auto flex max-w-160 flex-wrap items-center gap-3 rounded-xl border border-border/60 bg-card/95 p-4 shadow-[0_16px_48px_-16px] shadow-black/60 backdrop-blur-xl">
                <div className="flex flex-wrap gap-2">
                  {holdsForMovie.map((hold) => (
                    <button
                      key={hold.sessionID}
                      type="button"
                      className="inline-flex items-center gap-1 rounded-full border border-primary/50 bg-primary/10 px-3 py-1 text-xs font-bold text-primary transition-colors hover:bg-primary/20"
                      onClick={() =>
                        handleSeatClick({ seat_id: hold.seatID } as SeatInfo)
                      }
                      aria-label={`Remove hold on ${hold.seatID}`}
                    >
                      {hold.seatID}
                      <X className="size-3" />
                    </button>
                  ))}
                </div>

                <span className="ml-auto inline-flex items-center gap-1.5 whitespace-nowrap text-sm text-muted-foreground">
                  <Timer className="size-4" />
                  <strong
                    className={cn(
                      "font-semibold tabular-nums",
                      remaining < 30_000
                        ? "animate-pulse text-destructive"
                        : "text-foreground",
                    )}
                  >
                    {formatCountdown(remaining)}
                  </strong>
                </span>

                <Button
                  onClick={isAdmin ? handleAdminConfirm : handleCheckout}
                  disabled={checkoutPending}
                  size="lg"
                >
                  {checkoutPending
                    ? "Confirming…"
                    : isAdmin
                      ? "Confirm (paid at counter)"
                      : "Pay & Confirm"}
                </Button>
              </div>
            </div>
          )}
        </>
      )}

      <Dialog open={confirmedSeats !== null} onOpenChange={closeDialog}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-lg">
              <PartyPopper className="size-5 text-primary" />
              Booking confirmed
            </DialogTitle>
            <DialogDescription>{movie?.title}</DialogDescription>
          </DialogHeader>

          <div className="flex flex-wrap gap-2">
            {confirmedSeats?.map((seatID) => (
              <Badge key={seatID} className="px-3 py-1 text-sm font-bold">
                {seatID}
              </Badge>
            ))}
          </div>

          <Separator />

          <p className="text-sm text-muted-foreground">
            {confirmedSeats?.length === 1
              ? "Your seat is locked in."
              : "Your seats are locked in."}{" "}
            {isAdmin
              ? "Payment collected at the counter."
              : "Show this screen when you arrive at the counter."}
          </p>

          <DialogFooter>
            <Button variant="outline" onClick={closeDialog}>
              Back to seats
            </Button>
            <Button onClick={() => navigate("/")} autoFocus>
              Done
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}
