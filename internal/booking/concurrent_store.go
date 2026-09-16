package booking

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type ConcurrentStore struct {
	bookings map[string]Booking // key: seatID
	sessions map[string]string  // key: sessionID -> seatID
	sync.RWMutex
}

func NewConcurrentStore() *ConcurrentStore {
	return &ConcurrentStore{
		bookings: map[string]Booking{},
		sessions: map[string]string{},
	}
}

// Book places a temporary hold on the seat.
func (s *ConcurrentStore) Book(b Booking) (Booking, error) {
	s.Lock()
	defer s.Unlock()

	if _, exists := s.bookings[b.SeatID]; exists {
		return Booking{}, ErrSeatAlreadyBooked
	}

	id := uuid.New().String()
	b.ID = id
	b.Status = "held"
	b.ExpiresAt = time.Now().Add(2 * time.Minute)

	s.bookings[b.SeatID] = b
	s.sessions[id] = b.SeatID

	return b, nil
}

// ListBookings returns all active booking sessions for the given movie ID.
func (s *ConcurrentStore) ListBookings(movieID string) []Booking {
	s.RLock()
	defer s.RUnlock()

	var result []Booking
	for _, b := range s.bookings {
		if b.MovieID == movieID {
			result = append(result, b)
		}
	}
	return result
}

// GetSession returns the booking payload for a session owned by userID.
func (s *ConcurrentStore) GetSession(ctx context.Context, sessionID string, userID string) (Booking, error) {
	s.RLock()
	defer s.RUnlock()

	seatID, exists := s.sessions[sessionID]
	if !exists {
		return Booking{}, ErrSessionNotFound
	}

	b, exists := s.bookings[seatID]
	if !exists {
		return Booking{}, ErrSessionNotFound
	}

	if b.UserID != userID {
		return Booking{}, ErrUnauthorized
	}

	return b, nil
}

// ExtendHold refreshes a held session's expiry.
func (s *ConcurrentStore) ExtendHold(ctx context.Context, sessionID string, userID string, ttl time.Duration) error {
	s.Lock()
	defer s.Unlock()

	seatID, exists := s.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	b, exists := s.bookings[seatID]
	if !exists {
		return ErrSessionNotFound
	}

	if b.UserID != userID {
		return ErrUnauthorized
	}

	if b.Status == "confirmed" {
		return nil
	}

	b.ExpiresAt = time.Now().Add(ttl)
	s.bookings[seatID] = b

	return nil
}

// Confirm converts a held session into a permanent booking.
// Confirming an already-confirmed session is a no-op (idempotent).
func (s *ConcurrentStore) Confirm(ctx context.Context, sessionID string, userID string) (Booking, error) {
	s.Lock()
	defer s.Unlock()

	seatID, exists := s.sessions[sessionID]
	if !exists {
		return Booking{}, ErrSessionNotFound
	}

	b, exists := s.bookings[seatID]
	if !exists {
		return Booking{}, ErrSessionNotFound
	}

	if b.UserID != userID {
		return Booking{}, ErrUnauthorized
	}

	if b.Status == "confirmed" {
		return b, nil
	}

	b.Status = "confirmed"
	s.bookings[seatID] = b

	return b, nil
}

// Release removes an active hold.
func (s *ConcurrentStore) Release(ctx context.Context, sessionID string, userID string) error {
	s.Lock()
	defer s.Unlock()

	seatID, exists := s.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	b, exists := s.bookings[seatID]
	if !exists {
		return ErrSessionNotFound
	}

	if b.UserID != userID {
		return ErrUnauthorized
	}

	// A confirmed booking is permanent; never delete it via release.
	if b.Status == "confirmed" {
		return nil
	}

	delete(s.bookings, seatID)
	delete(s.sessions, sessionID)

	return nil
}

// AdminCancel removes a booking (held or confirmed) so the seat can be
// booked again. There is no ownership requirement.
func (s *ConcurrentStore) AdminCancel(ctx context.Context, sessionID string) error {
	s.Lock()
	defer s.Unlock()

	seatID, exists := s.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	delete(s.bookings, seatID)
	delete(s.sessions, sessionID)

	return nil
}
