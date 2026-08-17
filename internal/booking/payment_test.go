package booking

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cinema-booking-system/internal/payment"
)

func testCatalog() *Catalog {
	return NewCatalog([]Movie{
		{ID: "m1", Title: "Movie One", Rows: 2, SeatsPerRow: 2, PriceCents: 1500},
	})
}

func TestPayToConfirm_CheckoutThenWebhookConfirms(t *testing.T) {
	store := NewConcurrentStore()
	svc := NewService(store)
	gw := payment.NewFakeGateway()
	h := NewHandler(svc, testCatalog(), gw)

	const userID = "user-1"
	session, err := svc.Book(Booking{MovieID: "m1", SeatID: "A1", UserID: userID})
	if err != nil {
		t.Fatalf("book: %v", err)
	}

	cs, err := gw.CreateCheckoutSession(context.Background(), payment.CheckoutParams{
		SessionIDs: []string{session.ID},
		UserID:     userID,
		LineItems: []payment.LineItem{
			{Name: "Movie One — Seat A1", Amount: 1500, Qty: 1},
		},
		SuccessURL: "http://localhost:5173/#/movies/m1",
		CancelURL:  "http://localhost:5173/#/movies/m1",
	})
	if err != nil {
		t.Fatalf("create checkout: %v", err)
	}
	if cs.URL == "" {
		t.Fatal("expected a checkout URL")
	}

	// Creating a checkout must NOT confirm the seat — the seat stays held.
	got, err := svc.GetSession(context.Background(), session.ID, userID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.Status != "held" {
		t.Fatalf("expected status held after checkout creation, got %q", got.Status)
	}

	// Simulate Stripe's checkout.session.completed webhook.
	payload := `{"type":"checkout.session.completed","metadata":{"session_ids":"` +
		session.ID + `","user_id":"` + userID + `"}}`
	req := httptest.NewRequest(http.MethodPost, "/stripe/webhook", strings.NewReader(payload))
	req.Header.Set("Stripe-Signature", "fake")
	rec := httptest.NewRecorder()
	h.StripeWebhook(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook status = %d, want 200", rec.Code)
	}

	confirmed, err := svc.GetSession(context.Background(), session.ID, userID)
	if err != nil {
		t.Fatalf("get session after webhook: %v", err)
	}
	if confirmed.Status != "confirmed" {
		t.Fatalf("expected seat confirmed after webhook, got %q", confirmed.Status)
	}

	listings := svc.ListBookings("m1")
	if len(listings) != 1 || listings[0].Status != "confirmed" {
		t.Fatalf("expected 1 confirmed booking, got %+v", listings)
	}
}

// TestPayToConfirm_WebhookIdempotent verifies replaying a webhook for an
// already-confirmed session is a no-op (Stripe retries webhooks).
func TestPayToConfirm_WebhookIdempotent(t *testing.T) {
	store := NewConcurrentStore()
	svc := NewService(store)
	gw := payment.NewFakeGateway()
	h := NewHandler(svc, testCatalog(), gw)

	const userID = "user-1"
	session, err := svc.Book(Booking{MovieID: "m1", SeatID: "B2", UserID: userID})
	if err != nil {
		t.Fatalf("book: %v", err)
	}

	payload := `{"type":"checkout.session.completed","metadata":{"session_ids":"` +
		session.ID + `","user_id":"` + userID + `"}}`

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/stripe/webhook", strings.NewReader(payload))
		req.Header.Set("Stripe-Signature", "fake")
		rec := httptest.NewRecorder()
		h.StripeWebhook(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("webhook replay %d status = %d, want 200", i, rec.Code)
		}
	}

	listings := svc.ListBookings("m1")
	if len(listings) != 1 {
		t.Fatalf("expected exactly 1 booking after webhook replays, got %d", len(listings))
	}
}

// TestPayToConfirm_UnauthenticatedCheckout ensures the checkout endpoint is
// protected (creating a checkout requires a verified session).
func TestPayToConfirm_UnauthenticatedCheckout(t *testing.T) {
	gw := payment.NewFakeGateway()
	h := NewHandler(NewService(NewConcurrentStore()), testCatalog(), gw)

	req := httptest.NewRequest(http.MethodPost, "/sessions/checkout",
		strings.NewReader(`{"session_ids":["abc"],"success_url":"http://localhost:5173/","cancel_url":"http://localhost:5173/"}`))
	rec := httptest.NewRecorder()
	h.CreateCheckout(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("checkout without auth = %d, want 401", rec.Code)
	}
}