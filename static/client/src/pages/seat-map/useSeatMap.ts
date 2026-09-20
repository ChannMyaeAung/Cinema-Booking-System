import { useEffect, useMemo, useRef, useState } from "react";
import { useClerk, useUser } from "@clerk/react";
import { ApiError, type SeatInfo } from "../../api/api";
import {
  useAdminCancel,
  useAdminConfirm,
  useCreateCheckout,
  useHoldSeat,
  useMovies,
  useReleaseSession,
  useSeats,
} from "../../api/queries";
import {
  holdFromResponse,
  useSession,
  type ActiveHold,
} from "../../session-context";
import { useToast } from "../../toast-context";

export type SeatState = "available" | "selected" | "held" | "booked";

interface PendingCheckout {
  movieID: string;
  sessionIDs: string[];
  seats: string[];
  expiresAt: number;
}

const CHECKOUT_HOLD_MS = 30 * 60 * 1000;
const PENDING_CHECKOUT_KEY = "cinema.pendingCheckout";

export function formatCountdown(ms: number): string {
  const total = Math.max(0, Math.ceil(ms / 1000));
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

export const seatStateStyles: Record<SeatState, string> = {
  available: "border-ok/50 bg-surface-2 text-ok hover:border-ok hover:bg-ok/15",
  selected:
    "border-primary bg-primary text-primary-foreground shadow-[0_0_18px_-4px] shadow-primary/70",
  held: "border-[#ff9d3c]/50 bg-[#ff9d3c]/10 text-[#ff9d3c] cursor-not-allowed",
  booked: "border-transparent bg-white/[0.05] text-white/30 cursor-not-allowed",
};

export interface SeatMapState {
  movieTitle: string | undefined;
  isAdmin: boolean;
  isSeatsPending: boolean;
  isSeatsError: boolean;
  seatsLoaded: boolean;
  rows: [string, SeatInfo[]][];
  getState: (seat: SeatInfo) => SeatState;
  holdsForMovie: ActiveHold[];
  remaining: number;
  checkoutPending: boolean;
  confirmedSeats: string[] | null;
  voidTarget: SeatInfo | null;
  voidPending: boolean;
  onSeatClick: (seat: SeatInfo) => void;
  onReleaseHold: (hold: ActiveHold) => void;
  onCheckout: () => void;
  onAdminConfirm: () => void;
  onConfirmVoid: () => void;
  closeDialog: () => void;
  dismissVoid: () => void;
}

export function useSeatMap(movieID: string): SeatMapState {
  const { userID, activeHolds, addHold, removeHold } = useSession();
  const { push } = useToast();
  const { openSignIn } = useClerk();
  const { user } = useUser();

  const isAdmin = user?.publicMetadata?.role === "admin";
  const [now, setNow] = useState(() => Date.now());
  const [confirmedSeats, setConfirmedSeats] = useState<string[] | null>(null);
  const [voidTarget, setVoidTarget] = useState<SeatInfo | null>(null);

  const moviesQuery = useMovies();
  const seatsQuery = useSeats(movieID);
  const holdMutation = useHoldSeat();
  const checkoutMutation = useCreateCheckout();
  const releaseMutation = useReleaseSession();
  const confirmMutation = useAdminConfirm();
  const cancelMutation = useAdminCancel();

  const movieTitle = moviesQuery.data?.find((m) => m.id === movieID)?.title;

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

  function onSeatClick(seat: SeatInfo) {
    if (isAdmin && seat.booked && seat.confirmed) {
      setVoidTarget(seat);
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
      if (hold) onReleaseHold(hold);
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

  function onReleaseHold(hold: ActiveHold) {
    removeHold(hold.sessionID);
    releaseMutation.mutate(
      { movieID, sessionID: hold.sessionID },
      {
        onError: () => push(`Could not release ${hold.seatID}`, "error"),
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

  async function onCheckout() {
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

  function onAdminConfirm() {
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

  function onConfirmVoid() {
    if (!voidTarget?.session_id || cancelMutation.isPending) return;
    cancelMutation.mutate(
      { movieID, sessionID: voidTarget.session_id },
      {
        onSuccess: () => {
          push(`Booking on ${voidTarget.seat_id} cancelled`, "success");
          setVoidTarget(null);
        },
        onError: () => {
          setVoidTarget(null);
          push(`Could not cancel booking on ${voidTarget.seat_id}`, "error");
        },
      },
    );
  }

  return {
    movieTitle,
    isAdmin,
    isSeatsPending: seatsQuery.isPending,
    isSeatsError: seatsQuery.isError,
    seatsLoaded: !!seatsQuery.data,
    rows,
    getState,
    holdsForMovie,
    remaining,
    checkoutPending,
    confirmedSeats,
    voidTarget,
    voidPending: cancelMutation.isPending,
    onSeatClick,
    onReleaseHold,
    onCheckout,
    onAdminConfirm,
    onConfirmVoid,
    closeDialog,
    dismissVoid: () => setVoidTarget(null),
  };
}