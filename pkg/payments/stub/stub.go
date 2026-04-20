// Package stub provides a deterministic, in-memory Gateway for unit tests.
//
// The stub driver never talks to a real PSP. It stores created Intents /
// Payouts in a map keyed by IdempotencyKey so retries return the same
// result, and exposes hooks for tests to drive state transitions (e.g.
// "succeed intent X" or "emit a chargeback webhook for payout Y").
//
// Import guideline: only test code should import this package. Production
// code takes a payments.Gateway interface; wiring picks Stripe/Razorpay
// based on tenant country.
package stub

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/wegofwd2020/thittam/pkg/payments"
)

// Driver is a no-op implementation of payments.Gateway backed by an
// in-memory map. Safe for concurrent use within a single test.
type Driver struct {
	mu             sync.Mutex
	intentsByID    map[string]*payments.Intent // keyed by ProviderIntentID
	intentsByIdem  map[string]*payments.Intent // keyed by IdempotencyKey
	payoutsByIdem  map[string]*payments.Payout
	events         map[string]struct{} // dedup for webhook replay tests

	// signatureValidator is called by HandleWebhook. Default: accept
	// anything non-empty as valid. Tests can override to simulate
	// signature failures.
	signatureValidator func(body []byte, signature string) error
}

// New returns a fresh Driver with empty state.
func New() *Driver {
	return &Driver{
		intentsByID:   make(map[string]*payments.Intent),
		intentsByIdem: make(map[string]*payments.Intent),
		payoutsByIdem: make(map[string]*payments.Payout),
		events:        make(map[string]struct{}),
		signatureValidator: func(_ []byte, sig string) error {
			if sig == "" {
				return payments.ErrInvalidSignature
			}
			return nil
		},
	}
}

// Name identifies this driver.
func (d *Driver) Name() payments.ProviderName { return payments.ProviderStub }

// CreateIntent creates a new Intent, honouring IdempotencyKey.
func (d *Driver) CreateIntent(_ context.Context, in payments.Intent) (*payments.Intent, error) {
	if in.IdempotencyKey == "" {
		return nil, payments.ErrIdempotencyKeyMissing
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	if existing, ok := d.intentsByIdem[in.IdempotencyKey]; ok {
		return existing, nil // idempotent retry
	}

	now := time.Now().UTC()
	if in.ID == uuid.Nil {
		in.ID = uuid.New()
	}
	in.ProviderName = payments.ProviderStub
	in.ProviderIntentID = "stub_intent_" + in.ID.String()
	in.Status = payments.StatusPending
	in.CreatedAt = now
	in.UpdatedAt = now

	intent := in
	d.intentsByIdem[in.IdempotencyKey] = &intent
	d.intentsByID[in.ProviderIntentID] = &intent
	out := intent
	return &out, nil
}

// GetIntent returns the current state of a stub intent.
func (d *Driver) GetIntent(_ context.Context, providerIntentID string) (*payments.Intent, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	i, ok := d.intentsByID[providerIntentID]
	if !ok {
		return nil, payments.ErrNotFound
	}
	copy := *i
	return &copy, nil
}

// CapturePayout creates a new Payout, honouring IdempotencyKey.
func (d *Driver) CapturePayout(_ context.Context, p payments.Payout) (*payments.Payout, error) {
	if p.IdempotencyKey == "" {
		return nil, payments.ErrIdempotencyKeyMissing
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	if existing, ok := d.payoutsByIdem[p.IdempotencyKey]; ok {
		return existing, nil
	}
	now := time.Now().UTC()
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	p.ProviderName = payments.ProviderStub
	p.ProviderPayoutID = "stub_payout_" + p.ID.String()
	p.Status = payments.StatusProcessing
	p.CreatedAt = now
	p.UpdatedAt = now

	payout := p
	d.payoutsByIdem[p.IdempotencyKey] = &payout
	out := payout
	return &out, nil
}

// HandleWebhook verifies the signature (stub: non-empty) and dedups on
// the embedded event ID. Tests drive this directly by calling EmitEvent,
// which crafts a signed payload this accepts.
func (d *Driver) HandleWebhook(_ context.Context, body []byte, signature string) (*payments.Event, error) {
	if err := d.signatureValidator(body, signature); err != nil {
		return nil, err
	}
	var evt payments.Event
	if err := json.Unmarshal(body, &evt); err != nil {
		return nil, fmt.Errorf("payments/stub: parse event: %w", err)
	}
	if evt.ID == "" {
		return nil, fmt.Errorf("payments/stub: event.id required")
	}
	evt.ProviderName = payments.ProviderStub
	evt.ReceivedAt = time.Now().UTC()

	d.mu.Lock()
	defer d.mu.Unlock()

	if _, seen := d.events[evt.ID]; seen {
		return nil, payments.ErrDuplicateEvent
	}
	d.events[evt.ID] = struct{}{}
	return &evt, nil
}

// --- Test helpers ---

// TransitionIntent changes an intent's status and bumps UpdatedAt. Used
// by tests to simulate PSP state changes without going through webhooks.
// Does nothing if the providerIntentID is unknown.
func (d *Driver) TransitionIntent(providerIntentID string, next payments.Status) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if i, ok := d.intentsByID[providerIntentID]; ok {
		i.Status = next
		i.UpdatedAt = time.Now().UTC()
	}
}

// SetSignatureValidator overrides the default (accept non-empty) validator.
// Tests use this to simulate signature failures.
func (d *Driver) SetSignatureValidator(fn func(body []byte, signature string) error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.signatureValidator = fn
}
