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
type Booking struct{
	ID string
	MovieID string 
	SeatID string 
	UserID string 
	Status string 
	ExpiresAt time.Time
}

type BookingStore interface{
	Book(b Booking) (Booking, error)
	ListBookings(movieID string) []Booking

	Confirm(ctx context.Context, sessionID string, userID string) (Booking, error)
	Release(ctx context.Context, sessionID string, userID string) error
}