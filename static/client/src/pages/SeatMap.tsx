import { Link, useNavigate, useParams } from "react-router-dom";
import type { SeatInfo } from "../api/api";
import type { ActiveHold } from "../session-context";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { ArrowLeft, PartyPopper, Timer, X } from "lucide-react";
import {
  formatCountdown,
  seatStateStyles,
  useSeatMap,
  type SeatState,
} from "./seat-map/useSeatMap";

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

function SeatGrid({
  rows,
  getState,
  isAdmin,
  onSeatClick,
}: {
  rows: [string, SeatInfo[]][];
  getState: (seat: SeatInfo) => SeatState;
  isAdmin: boolean;
  onSeatClick: (seat: SeatInfo) => void;
}) {
  return (
    <div className="mx-auto mt-8 max-w-155">
        <div
          className="mx-auto mb-7 grid h-11 w-full place-items-center rounded-[50%_50%_4px_4px] bg-linear-to-b from-primary/40 to-primary/5 text-[0.7rem] uppercase tracking-[0.3em] text-white/70 shadow-[0_-10px_40px_-8px] shadow-primary/20"
          aria-hidden="true"
        >
          Screen
        </div>

        <div className="mb-6 flex flex-col gap-2" role="grid" aria-label="Seat map">
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
                    onClick={() => onSeatClick(seat)}
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
                  "inline-block h-3.5 w-4 rounded-lg border",
                  seatStateStyles[state],
                )}
              />
              {label}
            </span>
          ))}
        </div>
      </div>
  );
}

function CheckoutBar({
  holdsForMovie,
  remaining,
  checkoutPending,
  isAdmin,
  onRemoveHold,
  onConfirm,
}: {
  holdsForMovie: ActiveHold[];
  remaining: number;
  checkoutPending: boolean;
  isAdmin: boolean;
  onRemoveHold: (hold: ActiveHold) => void;
  onConfirm: () => void;
}) {
  return (
    <div className="fixed inset-x-0 bottom-4 z-30 px-4">
      <div className="mx-auto flex max-w-160 flex-wrap items-center gap-3 rounded-xl border border-border/60 bg-card/95 p-4 shadow-[0_16px_48px_-16px] shadow-black/60 backdrop-blur-xl">
        <div className="flex flex-wrap gap-2">
          {holdsForMovie.map((hold) => (
            <button
              key={hold.sessionID}
              type="button"
              className="inline-flex items-center gap-1 rounded-full border border-primary/50 bg-primary/10 px-3 py-1 text-xs font-bold text-primary transition-colors hover:bg-primary/20"
              onClick={() => onRemoveHold(hold)}
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

        <Button onClick={onConfirm} disabled={checkoutPending} size="lg">
          {checkoutPending
            ? "Confirming…"
            : isAdmin
              ? "Confirm (paid at counter)"
              : "Pay & Confirm"}
        </Button>
      </div>
    </div>
  );
}

function SuccessDialog({
  open,
  onClose,
  title,
  seats,
  isAdmin,
  onDone,
}: {
  open: boolean;
  onClose: () => void;
  title: string | undefined;
  seats: string[] | null;
  isAdmin: boolean;
  onDone: () => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onClose}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-lg">
            <PartyPopper className="size-5 text-primary" />
            Booking confirmed
          </DialogTitle>
          <DialogDescription>{title}</DialogDescription>
        </DialogHeader>

        <div className="flex flex-wrap gap-2">
          {seats?.map((seatID) => (
            <Badge key={seatID} className="px-3 py-1 text-sm font-bold">
              {seatID}
            </Badge>
          ))}
        </div>

        <Separator />

        <p className="text-sm text-muted-foreground">
          {seats?.length === 1
            ? "Your seat is locked in."
            : "Your seats are locked in."}{" "}
          {isAdmin
            ? "Payment collected at the counter."
            : "Show this screen when you arrive at the counter."}
        </p>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Back to seats
          </Button>
          <Button onClick={onDone} autoFocus>
            Done
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function VoidDialog({
  title,
  voidTarget,
  isPending,
  onDismiss,
  onConfirm,
}: {
  title: string | undefined;
  voidTarget: SeatInfo | null;
  isPending: boolean;
  onDismiss: () => void;
  onConfirm: () => void;
}) {
  return (
    <Dialog open={voidTarget !== null} onOpenChange={onDismiss}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="text-lg">Void booking</DialogTitle>
          <DialogDescription>
            {title} - Seat {voidTarget?.seat_id}
          </DialogDescription>
        </DialogHeader>

        <Separator />

        <p className="text-sm text-muted-foreground">
          Void the booking on this seat? The seat becomes free again and can be
          held or booked by anyone. This cannot be undone.
        </p>

        <DialogFooter>
          <Button variant="outline" onClick={onDismiss} disabled={isPending}>
            Keep booking
          </Button>
          <Button
            variant="destructive"
            onClick={onConfirm}
            disabled={isPending}
          >
            {isPending ? "Voiding…" : "Void booking"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default function SeatMap() {
  const { movieID = "" } = useParams<{ movieID: string }>();
  const navigate = useNavigate();
  const {
    movieTitle,
    isAdmin,
    isSeatsPending,
    isSeatsError,
    seatsLoaded,
    rows,
    getState,
    holdsForMovie,
    remaining,
    checkoutPending,
    confirmedSeats,
    voidTarget,
    voidPending,
    onSeatClick,
    onReleaseHold,
    onCheckout,
    onAdminConfirm,
    onConfirmVoid,
    closeDialog,
    dismissVoid,
  } = useSeatMap(movieID);

  return (
    <section>
      <Button className="p-5 mb-5" variant="secondary">
        <Link to="/" className=" inline-flex items-center gap-1.5 text-sm">
          <ArrowLeft className="size-4 text-white" />
          <span className="text-white">All movies</span>
        </Link>
      </Button>

      <h1 className="font-heading text-3xl font-bold tracking-tight">
        {movieTitle ?? "Select seats"}
      </h1>
      <p className="mt-1 text-muted-foreground">
        {isAdmin
          ? "Staff mode: tap a free seat to hold it, then confirm at the counter — no card payment needed. Tap a booked seat to void it."
          : "Tap a free seat to hold it. You have 2 min to pay. Once paid, your seat is locked in."}
      </p>

      {isSeatsPending && (
        <div className="mt-8" aria-hidden="true">
          <div className="mx-auto max-w-155">
            <div className="mx-auto mb-7 h-11 w-full rounded-[50%_50%_4px_4px] bg-primary/20" />
            <div className="mb-6 flex flex-col gap-2">
              {Array.from({ length: 5 }).map((_, r) => (
                <div className="flex items-center justify-center gap-2" key={r}>
                  {Array.from({ length: 8 }).map((_, c) => (
                    <Skeleton key={c} className="h-7.5 w-8.5 rounded-[6px]" />
                  ))}
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {isSeatsError && (
        <div
          role="alert"
          className="mt-8 rounded-xl border border-destructive/40 bg-destructive/10 p-4 text-sm text-destructive"
        >
          Could not load the seat map. Is the server running?
        </div>
      )}

      {seatsLoaded && (
        <>
          <SeatGrid
            rows={rows}
            getState={getState}
            isAdmin={isAdmin}
            onSeatClick={onSeatClick}
          />

          {holdsForMovie.length > 0 && (
            <CheckoutBar
              holdsForMovie={holdsForMovie}
              remaining={remaining}
              checkoutPending={checkoutPending}
              isAdmin={isAdmin}
              onRemoveHold={onReleaseHold}
              onConfirm={isAdmin ? onAdminConfirm : onCheckout}
            />
          )}
        </>
      )}

      <SuccessDialog
        open={confirmedSeats !== null}
        onClose={closeDialog}
        title={movieTitle}
        seats={confirmedSeats}
        isAdmin={isAdmin}
        onDone={() => navigate("/")}
      />

      <VoidDialog
        title={movieTitle}
        voidTarget={voidTarget}
        isPending={voidPending}
        onDismiss={dismissVoid}
        onConfirm={onConfirmVoid}
      />
    </section>
  );
}