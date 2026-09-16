package booking

import (
	"context"
	"errors"
	"time"
)

var (
	ErrSeatAlreadyBooked = errors.New("seat is already taken")
	ErrSessionNotFound   = errors.New("session not found")
	ErrUnauthorized      = errors.New("unauthorized: session does not belong to this user")
)

// Booking represents a confirmed seat reservation
type Booking struct {
	ID        string
	MovieID   string
	SeatID    string
	UserID    string
	Status    string
	ExpiresAt time.Time
}

type BookingStore interface {
	Book(b Booking) (Booking, error)
	ListBookings(movieID string) []Booking

	// GetSession returns the booking for a session owned by the given user.
	GetSession(ctx context.Context, sessionID string, userID string) (Booking, error)
	// ExtendHold refreshes a held session's TTL (used during payment checkout).
	ExtendHold(ctx context.Context, sessionID string, userID string, ttl time.Duration) error

	Confirm(ctx context.Context, sessionID string, userID string) (Booking, error)
	Release(ctx context.Context, sessionID string, userID string) error

	// AdminCancel removes a booking (held or confirmed) entirely so the seat
	// becomes available again. Staff use it to void a confirmed counter
	// booking; unlike Release it does not require the booking's owner.
	AdminCancel(ctx context.Context, sessionID string) error
}
