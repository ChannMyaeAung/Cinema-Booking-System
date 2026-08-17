package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// FakeGateway is an in-memory PaymentGateway for tests. It never contacts a
// real provider: CreateCheckoutSession records the request and returns a
// fake URL, and ParseWebhookEvent decodes a JSON payload that mirrors the
// fields Stripe's webhook would send.
type FakeGateway struct {
	mu      sync.Mutex
	created []CheckoutParams
	// pendingEvents, when non-empty, are returned in order by ParseWebhookEvent
	// instead of parsing the payload (lets tests drive the lifecycle directly).
	pendingEvents []WebhookEvent
}

// NewFakeGateway returns a ready-to-use fake gateway.
func NewFakeGateway() *FakeGateway {
	return &FakeGateway{}
}

// CreateCheckoutSession records the params and returns a fake checkout URL.
func (f *FakeGateway) CreateCheckoutSession(ctx context.Context, params CheckoutParams) (CheckoutSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, params)
	url := "https://checkout.stripe.example/fake/" + strings.Join(params.SessionIDs, ",")
	return CheckoutSession{ID: fmt.Sprintf("cs_fake_%d", len(f.created)), URL: url}, nil
}

// Created returns the checkout requests made so far.
func (f *FakeGateway) Created() []CheckoutParams {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]CheckoutParams, len(f.created))
	copy(out, f.created)
	return out
}

// QueueEvent queues a webhook event to be returned by the next
// ParseWebhookEvent call.
func (f *FakeGateway) QueueEvent(ev WebhookEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pendingEvents = append(f.pendingEvents, ev)
}

// ParseWebhookEvent returns the next queued event, or decodes a payload shaped
// like {"type": "...", "metadata": {"session_ids": "a,b", "user_id": "u"}}.
func (f *FakeGateway) ParseWebhookEvent(ctx context.Context, payload []byte, signature string) (WebhookEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.pendingEvents) > 0 {
		ev := f.pendingEvents[0]
		f.pendingEvents = f.pendingEvents[1:]
		return ev, nil
	}

	var raw struct {
		Type     string `json:"type"`
		Metadata struct {
			UserID     string `json:"user_id"`
			SessionIDs string `json:"session_ids"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return WebhookEvent{}, err
	}

	ev := WebhookEvent{Type: raw.Type, UserID: raw.Metadata.UserID}
	for _, s := range strings.Split(raw.Metadata.SessionIDs, ",") {
		if s != "" {
			ev.SessionIDs = append(ev.SessionIDs, s)
		}
	}
	return ev, nil
}
