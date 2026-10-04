package payment

import (
	"context"
	"time"
)

// LineItem describes a single chargeable item in a checkout.
type LineItem struct {
	Name   string
	Amount int64 // unit amount in cents
	Qty    int64
}

// CheckoutParams describes what a checkout session should collect payment for.
type CheckoutParams struct {
	SessionIDs []string
	UserID     string
	LineItems  []LineItem
	SuccessURL string
	CancelURL  string
	// ExpiresAt closes the hosted checkout page so a customer cannot pay
	// after their seat hold has lapsed. Zero means the provider default.
	ExpiresAt time.Time
}

// CheckoutSession is a payment-provider session ready to be opened.
type CheckoutSession struct {
	ID  string
	URL string
}

// WebhookEvent is a parsed, signature-verified webhook event.
type WebhookEvent struct {
	Type          string // e.g. "checkout.session.completed"
	CheckoutID    string
	SessionIDs    []string
	UserID        string
	PaymentStatus string // "paid", "unpaid" or "no_payment_required"
}

// PaymentGateway abstracts payment processing so the booking lifecycle can be
// tested without a real provider. Mirrors the BookingStore interface pattern.
type PaymentGateway interface {
	// CreateCheckoutSession creates a hosted checkout for the given line items
	// and returns the URL the customer should be redirected to.
	CreateCheckoutSession(ctx context.Context, params CheckoutParams) (CheckoutSession, error)

	// ParseWebhookEvent verifies the webhook signature and returns a parsed
	// event. It MUST return an error when the signature does not verify —
	// a webhook is the only trusted proof of payment.
	ParseWebhookEvent(ctx context.Context, payload []byte, signature string) (WebhookEvent, error)
}
