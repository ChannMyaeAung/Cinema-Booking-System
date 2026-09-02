package payment

import "context"

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
}

// CheckoutSession is a payment-provider session ready to be opened.
type CheckoutSession struct {
	ID  string
	URL string
}

// WebhookEvent is a parsed, signature-verified webhook event.
type WebhookEvent struct {
	Type          string // e.g. "checkout.session.completed"
	SessionIDs    []string
	UserID        string
	PaymentStatus string
}

// PaymentGateway abstracts payment processing so the booking lifecycle can be
// tested without a real provider. Mirrors the BookingStore interface pattern.
type PaymentGateway interface {
	// CreateCheckoutSession creates a hosted checkout for the given line items
	// and returns the URL the customer should be redirected to.
	CreateCheckoutSession(ctx context.Context, params CheckoutParams) (CheckoutSession, error)

	// ParseWebhookEvent verifies the webhook signature and returns a parsed
	// event. Unrecognized event types return a WebhookEvent with an empty Type.
	ParseWebhookEvent(ctx context.Context, payload []byte, signature string) (WebhookEvent, error)
}
