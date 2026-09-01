import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useClerk } from "@clerk/react";
import { ApiError, type SeatInfo } from "../api/api";
import {
  HOLDS_MS,
  useCreateCheckout,
  useHoldSeat,
  useMovies,
  useReleaseSession,
  useSeats,
} from "../api/queries";
import { holdFromResponse, useSession } from "../session-context";
import { useToast } from "../toast-context";

type SeatState = "available" | "selected" | "held" | "booked";

interface PendingCheckout {
  movieID: string;
  sessionIDs: string[];
  seats: string[];
  expiresAt: number;
}

// Matches the backend checkoutHoldTTL so local holds don't expire client-side
// while the user is on the Stripe checkout page.
const CHECKOUT_HOLD_MS = 30 * 60 * 1000;
const PENDING_CHECKOUT_KEY = "cinema.pendingCheckout";

function formatCountdown(ms: number): string {
  const total = Math.max(0, Math.ceil(ms / 1000));
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

export default function SeatMap() {
  const { movieID = "" } = useParams<{ movieID: string }>();
  const navigate = useNavigate();
  const { userID, activeHolds, addHold, removeHold } = useSession();
  const { push } = useToast();
  const { openSignIn } = useClerk();
  const [now, setNow] = useState(() => Date.now());
  const [confirmedSeats, setConfirmedSeats] = useState<string[] | null>(null);

  const moviesQuery = useMovies();
  const seatsQuery = useSeats(movieID);
  const holdMutation = useHoldSeat();
  const checkoutMutation = useCreateCheckout();
  const releaseMutation = useReleaseSession();

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

  function handleSeatClick(seat: SeatInfo) {
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
  const checkoutPending = checkoutMutation.isPending || holdMutation.isPending;

  // Track a checkout that is currently on the Stripe hosted page so we can
  // detect its successful return and show the confirmation dialog.
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

    // Remember what we're paying for; extended so a slow Stripe visit doesn't
    // expire the local hold (matches the backend checkoutHoldTTL).
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

  // After Stripe redirects back (or on a fresh load while a checkout is in
  // flight), poll the seat map: once every seat we paid for is confirmed by
  // the webhook, clear the holds and show the success dialog.
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
    push("Payment received — seats confirmed", "success");
  }, [seatsQuery.data, movieID, userID, removeHold, push]);

  function closeDialog() {
    setConfirmedSeats(null);
  }

  return (
    <>
      <Link to="/" className="back-link">
        ← All movies
      </Link>
      <h1 className="page-title">{movie?.title ?? "Select seats"}</h1>
      <p className="page-subtitle">
        Tap a free seat to hold it. You have {Math.round(HOLDS_MS / 60000)} min
        to pay. Once paid, your seat is locked in.
      </p>

      {seatsQuery.isPending && (
        <>
          <div className="screen" aria-hidden="true">
            Screen
          </div>
          <div className="seat-grid skeleton-grid" aria-hidden="true">
            {Array.from({ length: 5 }).map((_, r) => (
              <div className="seat-row" key={r}>
                <span className="seat-row-label" />
                {Array.from({ length: 8 }).map((_, c) => (
                  <span className="seat seat-skeleton" key={c} />
                ))}
              </div>
            ))}
          </div>
        </>
      )}

      {seatsQuery.isError && (
        <div className="error-banner">
          Could not load the seat map. Is the server running?
        </div>
      )}

      {seatsQuery.data && (
        <>
          <div className="screen" aria-hidden="true">
            Screen
          </div>
          <div className="seat-grid" role="grid" aria-label="Seat map">
            {rows.map(([label, seats]) => (
              <div className="seat-row" role="row" key={label}>
                <span className="seat-row-label" aria-hidden="true">
                  {label}
                </span>
                {seats.map((seat) => (
                  <button
                    type="button"
                    role="gridcell"
                    key={seat.seat_id}
                    className={`seat seat-${getState(seat)}`}
                    onClick={() => handleSeatClick(seat)}
                    disabled={
                      getState(seat) === "booked" || getState(seat) === "held"
                    }
                    aria-label={`Seat ${seat.seat_id}, ${getState(seat)}`}
                    aria-pressed={getState(seat) === "selected"}
                  >
                    {seat.seat_id.slice(1)}
                  </button>
                ))}
              </div>
            ))}
          </div>

          <div className="seat-legend" aria-label="Legend">
            <span className="legend-item">
              <span className="legend-swatch seat-available" /> Available
            </span>
            <span className="legend-item">
              <span className="legend-swatch seat-selected" /> Selected
            </span>
            <span className="legend-item">
              <span className="legend-swatch seat-held" /> Held by others
            </span>
            <span className="legend-item">
              <span className="legend-swatch seat-booked" /> Booked
            </span>
          </div>
        </>
      )}

      {holdsForMovie.length > 0 && (
        <div className="checkout-bar" role="region" aria-label="Checkout">
          <div className="checkout-seats">
            {holdsForMovie.map((hold) => (
              <button
                key={hold.sessionID}
                type="button"
                className="checkout-seat"
                onClick={() =>
                  handleSeatClick({ seat_id: hold.seatID } as SeatInfo)
                }
                aria-label={`Remove hold on ${hold.seatID}`}
              >
                {hold.seatID} ×
              </button>
            ))}
          </div>
          <div className="checkout-countdown">
            Time left{" "}
            <strong className={remaining < 30_000 ? "urgent" : ""}>
              {formatCountdown(remaining)}
            </strong>
          </div>
          <button
            type="button"
            className="checkout-confirm"
            onClick={handleCheckout}
            disabled={checkoutPending}
          >
            {checkoutPending ? "Starting checkout…" : "Pay & Confirm"}
          </button>
        </div>
      )}

      {confirmedSeats !== null && (
        <div
          className="dialog-backdrop"
          onMouseDown={(e) => {
            if (e.target === e.currentTarget) closeDialog();
          }}
        >
          <div
            className="dialog"
            role="dialog"
            aria-modal="true"
            aria-labelledby="dialog-title"
          >
            <h2 id="dialog-title" className="dialog-title">
              Booking confirmed
            </h2>
            <p className="dialog-movie">{movie?.title}</p>
            <div className="dialog-seats">
              {confirmedSeats.map((seatID) => (
                <span key={seatID} className="dialog-seat">
                  {seatID}
                </span>
              ))}
            </div>
            <p className="dialog-note">
              {confirmedSeats.length === 1
                ? "Your seat is locked in."
                : "Your seats are locked in."}{" "}
              Show this screen when you arrive at the counter.
            </p>
            <div className="dialog-actions">
              <button
                type="button"
                className="checkout-confirm"
                onClick={() => navigate("/")}
                autoFocus
              >
                Done
              </button>
              <button
                type="button"
                className="dialog-close"
                onClick={closeDialog}
              >
                Back to seats
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
