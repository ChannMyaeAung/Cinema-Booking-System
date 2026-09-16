package booking

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cinema-booking-system/internal/auth"

	"cinema-booking-system/internal/payment"
)

func adminHandler() *handler {
	return NewHandler(NewService(NewConcurrentStore()), testCatalog(), payment.NewFakeGateway())
}

// TestAdminConfirm_DirectConfirm skips the payment step entirely: staff hold a
// seat as themselves, then confirm it from the counter.
func TestAdminConfirm_DirectConfirm(t *testing.T) {
	h := adminHandler()
	const adminID = "admin-1"

	session, err := h.svc.Book(Booking{MovieID: "m1", SeatID: "A1", UserID: adminID})
	if err != nil {
		t.Fatalf("book: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/sessions/confirm",
		strings.NewReader(`{"session_ids":["`+session.ID+`"]}`))
	req = req.WithContext(auth.WithUser(req.Context(), adminID))
	rec := httptest.NewRecorder()
	h.AdminConfirmSeats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("confirm status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"seat_id":"A1"`) {
		t.Fatalf("expected confirmed seat A1 in response, got %s", rec.Body.String())
	}

	got, err := h.svc.GetSession(context.Background(), session.ID, adminID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.Status != "confirmed" {
		t.Fatalf("expected status confirmed after admin confirm, got %q", got.Status)
	}
}

// TestAdminConfirm_Unauthenticated ensures the admin endpoint rejects a request
// with no verified user in context (the AdminMiddleware would catch it first).
func TestAdminConfirm_Unauthenticated(t *testing.T) {
	h := adminHandler()
	req := httptest.NewRequest(http.MethodPost, "/admin/sessions/confirm",
		strings.NewReader(`{"session_ids":["abc"]}`))
	rec := httptest.NewRecorder()
	h.AdminConfirmSeats(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("confirm without auth = %d, want 401", rec.Code)
	}
}

// TestAdminConfirm_WrongOwner ensures an admin cannot confirm a session held by
// another account (ownership is still enforced even in the admin flow).
func TestAdminConfirm_WrongOwner(t *testing.T) {
	h := adminHandler()
	session, err := h.svc.Book(Booking{MovieID: "m1", SeatID: "B1", UserID: "customer-1"})
	if err != nil {
		t.Fatalf("book: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/sessions/confirm",
		strings.NewReader(`{"session_ids":["`+session.ID+`"]}`))
	req = req.WithContext(auth.WithUser(req.Context(), "admin-1"))
	rec := httptest.NewRecorder()
	h.AdminConfirmSeats(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("confirm someone else's hold = %d, want 403", rec.Code)
	}
}

// TestAdminCancel_VoidsConfirmedBooking ensures staff can void a confirmed
// booking, freeing the seat again.
func TestAdminCancel_VoidsConfirmedBooking(t *testing.T) {
	h := adminHandler()
	const adminID = "admin-1"

	session, err := h.svc.Book(Booking{MovieID: "m1", SeatID: "C1", UserID: adminID})
	if err != nil {
		t.Fatalf("book: %v", err)
	}
	if _, err := h.svc.ConfirmSeat(context.Background(), session.ID, adminID); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/admin/sessions/"+session.ID, nil)
	req.SetPathValue("sessionID", session.ID)
	req = req.WithContext(auth.WithUser(req.Context(), adminID))
	rec := httptest.NewRecorder()
	h.AdminCancelSession(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("cancel status = %d, want 204", rec.Code)
	}
	if listings := h.svc.ListBookings("m1"); len(listings) != 0 {
		t.Fatalf("expected 0 bookings after cancel, got %+v", listings)
	}
}

// TestAdminCancel_MissingSession returns 404 for an unknown session.
func TestAdminCancel_MissingSession(t *testing.T) {
	h := adminHandler()
	req := httptest.NewRequest(http.MethodDelete, "/admin/sessions/nope", nil)
	req.SetPathValue("sessionID", "nope")
	req = req.WithContext(auth.WithUser(req.Context(), "admin-1"))
	rec := httptest.NewRecorder()
	h.AdminCancelSession(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cancel missing session = %d, want 404", rec.Code)
	}
}