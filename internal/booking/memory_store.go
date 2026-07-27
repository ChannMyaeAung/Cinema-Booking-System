package booking

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type MemoryStore struct {
	bookings map[string]Booking
	sessions map[string]string
	sync.RWMutex
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		bookings: map[string]Booking{},
		sessions: map[string]string{},
	}
}

func (s *MemoryStore) Book(b Booking) (Booking, error) {
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

func (s *MemoryStore) ListBookings(movieID string) []Booking {
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

func (s *MemoryStore) Confirm(ctx context.Context, sessionID string, userID string) (Booking, error) {
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

	b.Status = "confirmed"
	s.bookings[seatID] = b

	return b, nil
}

func (s *MemoryStore) Release(ctx context.Context, sessionID string, userID string) error {
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

	delete(s.bookings, seatID)
	delete(s.sessions, sessionID)

	return nil
}
