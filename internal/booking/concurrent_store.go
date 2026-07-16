package booking

import "sync"

type ConcurrentStore struct{
	bookings map[string]Booking
	sync.RWMutex
}

func NewConcurrentStore() *ConcurrentStore{
	return &ConcurrentStore{
		bookings: map[string]Booking{},
	}
}

// Locks the store for writing and checks if the seat is already booked. If not, it books the seat.
func (s *ConcurrentStore) Book(b Booking) (Booking, error){
	s.Lock()
	defer s.Unlock()

	if _, exists := s.bookings[b.SeatID]; exists{
		return Booking{}, ErrSeatAlreadyBooked
	}

	s.bookings[b.SeatID] = b
	return b, nil
}

// Locks the store for reading and returns a list of bookings for the given movie ID.
func (s *ConcurrentStore) ListBookings(movieID string) []Booking{

	// Uses s.RLock() because it only reads data, not writes.
	// RLock allows multiple readers at the same time
	// which is more efficient than using a full write lock for reads
	s.RLock()
	defer s.RUnlock()

	var result []Booking
	for _, b :=range s.bookings{
		if b.MovieID == movieID{
			result = append(result, b)
		}
	}
	return result
}