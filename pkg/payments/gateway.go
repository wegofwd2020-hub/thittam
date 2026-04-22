// Package payments provides a provider-agnostic interface for receiving
// (Intent) and making (Payout) payments through Payment Service Providers
// like Stripe and Razorpay.
//
// See ADR-015 (docs/developers/adr/ADR-015-payment-gateway-abstraction.md in the
// thittam_docs repo) for the architecture rationale. Key points:
//
//   - One Gateway interface; each PSP is a separate driver satisfying it.
//   - Per-tenant provider selection driven by tenants.country_code.
//   - PCI scope is SAQ-A: card data never touches our servers. All card
//     collection is via provider-hosted UIs (Stripe Elements, Razorpay
//     Checkout). Our backend only sees opaque tokens.
//   - Money is decimal.Decimal strings in the major unit at every boundary
//     of this package (Rule #1). Drivers convert to provider minor units
//     at the PSP boundary.
//   - Every write takes an IdempotencyKey (Rule #5).
//   - State transitions are audit-logged (Rule #7).
package payments

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ProviderName identifies a payment-service provider. Typed rather than a
// bare string so the compiler catches typos and the value can be compared
// safely across package boundaries.
type ProviderName string

const (
	// ProviderStripe — Stripe Inc. Used for CA, US, EU, AU, and fallback.
	ProviderStripe ProviderName = "stripe"

	// ProviderRazorpay — Razorpay Software Pvt. Ltd. Used for IN (UPI + cards).
	ProviderRazorpay ProviderName = "razorpay"

	// ProviderStub — in-memory deterministic driver for unit tests.
	// Never selected in production; failing closed is safer than a silent
	// stub-in-prod bug.
	ProviderStub ProviderName = "stub"
)

// Method is the payment instrument the customer chooses. A single Intent
// carries one method; if the customer abandons a card flow and re-tries
// UPI, that is a new Intent.
type Method string

const (
	MethodCard    Method = "card"    // Visa / Mastercard / Amex / RuPay
	MethodUPI     Method = "upi"     // India Unified Payments Interface
	MethodInterac Method = "interac" // Canada e-Transfer / Debit
	MethodACH     Method = "ach"     // US bank transfer
	MethodSEPA    Method = "sepa"    // EU bank transfer
	MethodBACS    Method = "bacs"    // UK bank transfer
	MethodNetbanking Method = "netbanking" // India netbanking
)

// Status tracks the lifecycle of an Intent or Payout. State transitions
// are one-way: once Succeeded or Failed, an Intent is terminal (refunds
// are modelled as separate Events against the original Intent).
type Status string

const (
	StatusPending    Status = "pending"    // created, awaiting customer action
	StatusProcessing Status = "processing" // customer submitted, PSP working
	StatusSucceeded  Status = "succeeded"  // terminal: money received/sent
	StatusFailed     Status = "failed"     // terminal: PSP rejected, customer cancelled, etc.
	StatusRefunded   Status = "refunded"   // terminal: post-success refund completed
	StatusCancelled  Status = "cancelled"  // terminal: cancelled before processing
)

// EventType names a webhook event from the PSP. The canonical set is
// intentionally small; provider-specific events map onto these or are
// dropped. Drivers add internal types with a "driver.*" prefix when they
// must expose something without a generic equivalent.
type EventType string

const (
	EventIntentSucceeded EventType = "intent.succeeded"
	EventIntentFailed    EventType = "intent.failed"
	EventIntentCancelled EventType = "intent.cancelled"
	EventPayoutSucceeded EventType = "payout.succeeded"
	EventPayoutFailed    EventType = "payout.failed"
	EventRefundCreated   EventType = "refund.created"
	EventChargebackOpened EventType = "chargeback.opened"
)

// Intent represents a request to receive a payment (customer → us / tenant).
// The ID is our UUID, not the PSP's — ProviderIntentID carries the PSP's
// identifier so drivers can round-trip status lookups.
type Intent struct {
	ID               uuid.UUID
	TenantID         uuid.UUID
	ProviderName     ProviderName
	ProviderIntentID string // empty until CreateIntent returns

	Amount   decimal.Decimal // major unit, e.g. "150.00"
	Currency string          // ISO 4217, 3 letters, uppercase
	Method   Method
	Status   Status

	// IdempotencyKey MUST be stable for a given logical attempt. A retry of
	// the same attempt (network timeout, client retry, etc.) passes the
	// same key; the driver short-circuits to return the prior result.
	// Format: caller's choice; UUIDv4 is fine. Max 64 chars.
	IdempotencyKey string

	// Description shown on the customer's statement (Stripe truncates at
	// ~22 chars; Razorpay allows longer; keep ≤22 for portability).
	Description string

	// CustomerID is an optional PSP-side customer identifier for vaulted
	// payment methods. Empty for one-time guest checkouts.
	CustomerID string

	// Metadata is attached to the PSP-side object for operator debugging.
	// Keys ≤40 chars, values ≤500 chars, max 50 entries (Stripe limits;
	// Razorpay is similar). NOT used for business logic; state lives in
	// our own columns.
	Metadata map[string]string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Payout represents a request to send money outward (us / tenant → vendor,
// crew, contractor). Mirror of Intent plus a RecipientToken identifying
// the destination bank / UPI handle / card. RecipientToken is opaque and
// PSP-scoped — never a raw account number; always a tokenised reference
// the PSP vaulted previously.
type Payout struct {
	ID               uuid.UUID
	TenantID         uuid.UUID
	ProviderName     ProviderName
	ProviderPayoutID string

	Amount   decimal.Decimal
	Currency string
	Method   Method
	Status   Status

	IdempotencyKey string
	Description    string

	// RecipientToken identifies where the money goes, tokenised by the PSP
	// during a prior recipient-onboarding flow. Never a raw account number
	// or UPI handle — that would fail PCI / DPP equivalents.
	RecipientToken string

	Metadata map[string]string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Event is a normalised PSP webhook callback. Drivers translate provider-
// specific events into one of the canonical EventType values and surface
// the raw PSP payload for audit.
type Event struct {
	ID           string       // PSP event ID — used for dedup against replayed webhooks
	ProviderName ProviderName
	Type         EventType

	// Exactly one of IntentID / PayoutID is set, matching the entity the
	// event pertains to. uuid.Nil means the event concerns an object we
	// don't track (rare; usually a driver bug or an orphaned PSP object).
	IntentID uuid.UUID
	PayoutID uuid.UUID

	// Raw is the original PSP payload, retained for audit and for any
	// provider-specific fields callers might need. Parse cautiously — shape
	// depends on the provider.
	Raw json.RawMessage

	ReceivedAt time.Time
}

// Gateway is the provider-agnostic contract. All payment code in the
// platform depends on this interface, never on a specific driver.
//
// Implementations MUST:
//   - Honour IdempotencyKey — retry with the same key returns the same
//     Intent/Payout, not a duplicate charge.
//   - Verify webhook signatures before calling HandleWebhook.
//   - Return sentinel errors (ErrNotFound, ErrInvalidSignature, etc.) so
//     callers can branch on type, not message text.
//   - Never mutate Amount / Currency between API input and PSP call; if
//     conversion to minor units is needed, do it at the PSP boundary.
type Gateway interface {
	// Name returns the provider identifier (e.g. ProviderStripe).
	Name() ProviderName

	// CreateIntent creates a new payment intent with the PSP. On success
	// the returned Intent has ProviderIntentID and Status (usually
	// StatusPending) populated. On retry with the same IdempotencyKey,
	// implementations MUST return the original Intent without creating a
	// duplicate at the PSP.
	CreateIntent(ctx context.Context, intent Intent) (*Intent, error)

	// GetIntent fetches the latest state from the PSP. Used by the
	// reconciliation sweep to catch webhooks that were lost or delayed.
	GetIntent(ctx context.Context, providerIntentID string) (*Intent, error)

	// CapturePayout initiates an outbound payment. Like CreateIntent,
	// idempotent on IdempotencyKey.
	CapturePayout(ctx context.Context, payout Payout) (*Payout, error)

	// GetPayout fetches the latest state of an outbound payment from the
	// PSP. Mirror of GetIntent for payouts — used by the reconciliation
	// sweep so a lost payout.succeeded / payout.failed webhook does not
	// strand a Payout in StatusProcessing indefinitely.
	GetPayout(ctx context.Context, providerPayoutID string) (*Payout, error)

	// HandleWebhook verifies the signature against body, parses the event,
	// and returns a normalised Event. Callers MUST NOT act on the event
	// until this returns without error — a signature failure means the
	// request is forged or replayed.
	//
	// provider identifies which PSP the webhook originated from, as
	// determined by the caller (e.g. from the URL path or a routing
	// header). A shared dispatcher can multiplex webhooks from multiple
	// PSPs onto a single endpoint; per-provider drivers may validate that
	// it matches their Name().
	//
	// signature format is provider-specific (Stripe header
	// `Stripe-Signature`, Razorpay header `X-Razorpay-Signature`, etc.).
	// The caller passes it verbatim.
	HandleWebhook(ctx context.Context, provider ProviderName, body []byte, signature string) (*Event, error)
}

// --- Sentinel errors ---

var (
	// ErrNotFound — the PSP has no object with the supplied identifier.
	ErrNotFound = errors.New("payments: not found at provider")

	// ErrInvalidSignature — webhook signature verification failed. MUST
	// result in the HTTP webhook endpoint returning a 4xx so the PSP does
	// not retry the same (forged) payload.
	ErrInvalidSignature = errors.New("payments: invalid webhook signature")

	// ErrDuplicateEvent — the event ID has been processed before (webhook
	// replay). Caller should return 200 to the PSP so it stops retrying,
	// but not re-dispatch the event internally.
	ErrDuplicateEvent = errors.New("payments: duplicate webhook event")

	// ErrProviderRejected — the PSP refused the request for a business
	// reason (insufficient funds, blocked card, rate limit, etc.). Wrap
	// with fmt.Errorf to preserve the PSP's message.
	ErrProviderRejected = errors.New("payments: provider rejected")

	// ErrInvalidAmount — amount is non-positive, has more precision than
	// the currency allows, or the currency is unknown.
	ErrInvalidAmount = errors.New("payments: invalid amount")

	// ErrIdempotencyKeyMissing — a write was attempted without
	// IdempotencyKey. All writes require one; reject at the driver edge.
	ErrIdempotencyKeyMissing = errors.New("payments: idempotency_key required")
)
