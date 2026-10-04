package booking

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cinema-booking-system/internal/auth"
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
	payload := `{"type":"checkout.session.completed","payment_status":"paid","metadata":{"session_ids":"` +
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

	payload := `{"type":"checkout.session.completed","payment_status":"paid","metadata":{"session_ids":"` +
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

// postWebhook delivers a (fake-gateway) webhook payload to the handler.
func postWebhook(t *testing.T, h *handler, payload string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/stripe/webhook", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	h.StripeWebhook(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("webhook status = %d, want 200", rec.Code)
	}
}

// TestPayToConfirm_UnpaidCompletionDoesNotConfirm covers delayed payment
// methods: checkout completes as "unpaid" and the seat must stay held.
func TestPayToConfirm_UnpaidCompletionDoesNotConfirm(t *testing.T) {
	svc := NewService(NewConcurrentStore())
	h := NewHandler(svc, testCatalog(), payment.NewFakeGateway())

	session, err := svc.Book(Booking{MovieID: "m1", SeatID: "A1", UserID: "user-1"})
	if err != nil {
		t.Fatalf("book: %v", err)
	}

	postWebhook(t, h, `{"type":"checkout.session.completed","payment_status":"unpaid","metadata":{"session_ids":"`+
		session.ID+`","user_id":"user-1"}}`)

	got, err := svc.GetSession(context.Background(), session.ID, "user-1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.Status != "held" {
		t.Fatalf("expected seat to stay held on unpaid completion, got %q", got.Status)
	}
}

// TestPayToConfirm_ExpiredCheckoutReleasesHold frees seats as soon as Stripe
// reports the checkout page expired unpaid.
func TestPayToConfirm_ExpiredCheckoutReleasesHold(t *testing.T) {
	svc := NewService(NewConcurrentStore())
	h := NewHandler(svc, testCatalog(), payment.NewFakeGateway())

	session, err := svc.Book(Booking{MovieID: "m1", SeatID: "A2", UserID: "user-1"})
	if err != nil {
		t.Fatalf("book: %v", err)
	}

	postWebhook(t, h, `{"type":"checkout.session.expired","payment_status":"unpaid","metadata":{"session_ids":"`+
		session.ID+`","user_id":"user-1"}}`)

	if listings := svc.ListBookings("m1"); len(listings) != 0 {
		t.Fatalf("expected seat released after checkout expiry, got %+v", listings)
	}
}

// TestPayToConfirm_CheckoutRejectsConfirmedSeat prevents charging twice for a
// seat that is already paid for.
func TestPayToConfirm_CheckoutRejectsConfirmedSeat(t *testing.T) {
	svc := NewService(NewConcurrentStore())
	gw := payment.NewFakeGateway()
	h := NewHandler(svc, testCatalog(), gw)

	session, _ := svc.Book(Booking{MovieID: "m1", SeatID: "B1", UserID: "user-1"})
	if _, err := svc.ConfirmSeat(context.Background(), session.ID, "user-1"); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/sessions/checkout",
		strings.NewReader(`{"session_ids":["`+session.ID+`"],"success_url":"http://x/","cancel_url":"http://x/"}`))
	req = req.WithContext(auth.WithUser(req.Context(), "user-1"))
	rec := httptest.NewRecorder()
	h.CreateCheckout(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("checkout for confirmed seat = %d, want 409", rec.Code)
	}
	if n := len(gw.Created()); n != 0 {
		t.Fatalf("expected no checkout created, got %d", n)
	}
}

// TestPayToConfirm_CheckoutSetsExpiry ensures the hosted checkout page closes
// before the extended seat hold does.
func TestPayToConfirm_CheckoutSetsExpiry(t *testing.T) {
	svc := NewService(NewConcurrentStore())
	gw := payment.NewFakeGateway()
	h := NewHandler(svc, testCatalog(), gw)

	session, _ := svc.Book(Booking{MovieID: "m1", SeatID: "B2", UserID: "user-1"})
	req := httptest.NewRequest(http.MethodPost, "/sessions/checkout",
		strings.NewReader(`{"session_ids":["`+session.ID+`"],"success_url":"http://x/","cancel_url":"http://x/"}`))
	req = req.WithContext(auth.WithUser(req.Context(), "user-1"))
	rec := httptest.NewRecorder()
	h.CreateCheckout(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("checkout status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	created := gw.Created()
	if len(created) != 1 || created[0].ExpiresAt.IsZero() {
		t.Fatalf("expected checkout with an expiry, got %+v", created)
	}
	if !created[0].ExpiresAt.Before(time.Now().Add(checkoutHoldTTL)) {
		t.Fatalf("checkout must expire before the seat hold")
	}
}
