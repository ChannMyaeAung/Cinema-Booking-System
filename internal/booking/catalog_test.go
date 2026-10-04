package booking

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"cinema-booking-system/internal/auth"
	"cinema-booking-system/internal/payment"
)

func TestMovie_HasSeat(t *testing.T) {
	m := Movie{Rows: 2, SeatsPerRow: 3}
	for seat, want := range map[string]bool{
		"A1": true, "B3": true,
		"C1": false, "A4": false, "A0": false, "A01": false,
		"a1": false, "A": false, "": false, "A-1": false,
	} {
		if got := m.HasSeat(seat); got != want {
			t.Errorf("HasSeat(%q) = %v, want %v", seat, got, want)
		}
	}
}

func TestHoldSeat_RejectsUnknownMovieAndSeat(t *testing.T) {
	h := NewHandler(NewService(NewConcurrentStore()), testCatalog(), payment.NewFakeGateway())

	for _, tc := range []struct{ movie, seat string }{
		{"nope", "A1"},
		{"m1", "Z9"},
	} {
		req := httptest.NewRequest(http.MethodPost, "/movies/"+tc.movie+"/seats/"+tc.seat+"/hold", nil)
		req.SetPathValue("movieID", tc.movie)
		req.SetPathValue("seatID", tc.seat)
		req = req.WithContext(auth.WithUser(req.Context(), "user-1"))
		rec := httptest.NewRecorder()
		h.HoldSeat(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("hold %s/%s = %d, want 404", tc.movie, tc.seat, rec.Code)
		}
	}
}

// TestConcurrentStore_SameSeatDifferentMovies: seat A1 in one screening must
// not block A1 in another.
func TestConcurrentStore_SameSeatDifferentMovies(t *testing.T) {
	svc := NewService(NewConcurrentStore())
	if _, err := svc.Book(Booking{MovieID: "m1", SeatID: "A1", UserID: "u"}); err != nil {
		t.Fatalf("book m1/A1: %v", err)
	}
	if _, err := svc.Book(Booking{MovieID: "m2", SeatID: "A1", UserID: "u"}); err != nil {
		t.Fatalf("book m2/A1: %v", err)
	}
}
