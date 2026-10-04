package payment

import (
	"context"
	"fmt"
	"strings"

	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	"github.com/stripe/stripe-go/v81/webhook"
)

// StripeGateway implements PaymentGateway against Stripe Checkout.
type StripeGateway struct {
	secretKey     string
	webhookSecret string
}

// NewStripeGateway creates a Stripe-backed payment gateway.
func NewStripeGateway(secretKey, webhookSecret string) *StripeGateway {
	return &StripeGateway{secretKey: secretKey, webhookSecret: webhookSecret}
}

// CreateCheckoutSession creates a hosted Stripe Checkout Session.
func (g *StripeGateway) CreateCheckoutSession(ctx context.Context, params CheckoutParams) (CheckoutSession, error) {
	lineItems := make([]*stripe.CheckoutSessionLineItemParams, 0, len(params.LineItems))
	for _, li := range params.LineItems {
		lineItems = append(lineItems, &stripe.CheckoutSessionLineItemParams{
			Quantity: stripe.Int64(li.Qty),
			PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
				Currency:   stripe.String("usd"),
				UnitAmount: stripe.Int64(li.Amount),
				ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
					Name: stripe.String(li.Name),
				},
			},
		})
	}

	scParams := &stripe.CheckoutSessionParams{
		Mode:       stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL: stripe.String(params.SuccessURL),
		CancelURL:  stripe.String(params.CancelURL),
		LineItems:  lineItems,
		SubmitType: stripe.String(string(stripe.CheckoutSessionSubmitTypePay)),
		Metadata: map[string]string{
			"user_id":     params.UserID,
			"session_ids": strings.Join(params.SessionIDs, ","),
		},
	}

	if !params.ExpiresAt.IsZero() {
		scParams.ExpiresAt = stripe.Int64(params.ExpiresAt.Unix())
	}

	client := session.Client{B: stripe.GetBackend(stripe.APIBackend), Key: g.secretKey}
	sc, err := client.New(scParams)
	if err != nil {
		return CheckoutSession{}, fmt.Errorf("creating checkout session: %w", err)
	}

	return CheckoutSession{ID: sc.ID, URL: sc.URL}, nil
}

// ParseWebhookEvent verifies the Stripe signature and returns a parsed event.
// An unverifiable payload is rejected outright: falling back to trusting the
// body would let anyone confirm seats by POSTing a fake event.
func (g *StripeGateway) ParseWebhookEvent(ctx context.Context, payload []byte, signature string) (WebhookEvent, error) {
	// The stripe-go SDK pins an older API version than the account/CLI may
	// send; the signature check is what matters, so ignore the mismatch.
	ev, err := webhook.ConstructEventWithOptions(payload, signature, g.webhookSecret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
	if err != nil {
		return WebhookEvent{}, fmt.Errorf("verifying webhook signature: %w", err)
	}

	we := WebhookEvent{Type: string(ev.Type)}
	if strings.HasPrefix(we.Type, "checkout.session.") {
		we.CheckoutID = ev.GetObjectValue("id")
		we.UserID = ev.GetObjectValue("metadata", "user_id")
		we.SessionIDs = splitIDs(ev.GetObjectValue("metadata", "session_ids"))
		we.PaymentStatus = ev.GetObjectValue("payment_status")
	}
	return we, nil
}

// splitIDs parses the comma-joined session_ids checkout metadata.
func splitIDs(joined string) []string {
	var ids []string
	for _, s := range strings.Split(joined, ",") {
		if s != "" {
			ids = append(ids, s)
		}
	}
	return ids
}
