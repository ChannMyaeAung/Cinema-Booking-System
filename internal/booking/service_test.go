package booking

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	adapterredis "cinema-booking-system/internal/adapters/redis"

	"github.com/google/uuid"
)

func TestConcurrentBooking_ExactlyOneWins(t *testing.T) {
	rdb, err := adapterredis.NewClient("localhost:6379")
	if err != nil {
		t.Skipf("redis not available, skipping: %v", err)
	}
	store := NewRedisStore(rdb)
	svc := NewService(store)

	const numGoroutines = 100_000 // 100k users trying to book a seat at the same time

	movieID := "test-" + uuid.New().String()

	var (
		successes atomic.Int64
		failures  atomic.Int64
		wg        sync.WaitGroup
	)

	wg.Add(numGoroutines)
	for i := range numGoroutines {
		go func(userNum int) {
			defer wg.Done()
			_, err := svc.Book(Booking{
				MovieID: movieID,
				SeatID:  "A1",
				UserID:  uuid.New().String(),
			})
			if err == nil {
				successes.Add(1)
			} else {
				failures.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if got := successes.Load(); got != 1 {
		t.Errorf("expected exactly 1 success, got %d", got)
	}
	if got := failures.Load(); got != int64(numGoroutines-1) {
		t.Errorf("expected %d failures, got %d", numGoroutines-1, got)
	}
}

func TestConcurrentStore_ExactlyOneWins(t *testing.T) {
	store := NewConcurrentStore()
	svc := NewService(store)

	const numGoroutines = 100

	var (
		successes atomic.Int64
		failures  atomic.Int64
		wg        sync.WaitGroup
	)

	wg.Add(numGoroutines)
	for i := range numGoroutines {
		go func(userNum int) {
			defer wg.Done()
			_, err := svc.Book(Booking{
				MovieID: "screen-1",
				SeatID:  "A1",
				UserID:  uuid.New().String(),
			})
			if err == nil {
				successes.Add(1)
			} else {
				failures.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if got := successes.Load(); got != 1 {
		t.Errorf("expected exactly 1 success, got %d", got)
	}
	if got := failures.Load(); got != int64(numGoroutines-1) {
		t.Errorf("expected %d failures, got %d", numGoroutines-1, got)
	}
}

func TestConcurrentStore_ConfirmAndRelease(t *testing.T) {
	store := NewConcurrentStore()
	svc := NewService(store)
	userID := uuid.New().String()

	session, err := svc.Book(Booking{
		MovieID: "screen-1",
		SeatID:  "A1",
		UserID:  userID,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	confirmed, err := svc.ConfirmSeat(context.Background(), session.ID, userID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if confirmed.Status != "confirmed" {
		t.Errorf("expected status confirmed, got %s", confirmed.Status)
	}

	err = svc.ReleaseSeat(context.Background(), session.ID, userID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	listings := svc.ListBookings("screen-1")
	if len(listings) != 0 {
		t.Errorf("expected 0 bookings after release, got %d", len(listings))
	}
}

func TestConcurrentStore_ConfirmWrongUser(t *testing.T) {
	store := NewConcurrentStore()
	svc := NewService(store)

	session, err := svc.Book(Booking{
		MovieID: "screen-1",
		SeatID:  "A1",
		UserID:  uuid.New().String(),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	_, err = svc.ConfirmSeat(context.Background(), session.ID, uuid.New().String())
	if err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestConcurrentStore_ReleaseWrongUser(t *testing.T) {
	store := NewConcurrentStore()
	svc := NewService(store)

	session, err := svc.Book(Booking{
		MovieID: "screen-1",
		SeatID:  "A1",
		UserID:  uuid.New().String(),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	err = svc.ReleaseSeat(context.Background(), session.ID, uuid.New().String())
	if err != ErrUnauthorized {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestConcurrentStore_ListBookings(t *testing.T) {
	store := NewConcurrentStore()
	svc := NewService(store)
	userID := uuid.New().String()

	svc.Book(Booking{MovieID: "m1", SeatID: "A1", UserID: userID})
	svc.Book(Booking{MovieID: "m1", SeatID: "A2", UserID: userID})
	svc.Book(Booking{MovieID: "m2", SeatID: "B1", UserID: userID})

	listings := svc.ListBookings("m1")
	if len(listings) != 2 {
		t.Errorf("expected 2 bookings for m1, got %d", len(listings))
	}

	listings = svc.ListBookings("m2")
	if len(listings) != 1 {
		t.Errorf("expected 1 booking for m2, got %d", len(listings))
	}
}
