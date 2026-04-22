// Package billing implements the SaaS billing service: subscription management,
// usage metering, invoice generation, payment method management, and dunning.
package billing

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Subscription tracks a tenant's active plan and billing cycle.
type Subscription struct {
	ID                  uuid.UUID  `json:"id"`
	TenantID            uuid.UUID  `json:"tenant_id"`
	Plan                string     `json:"plan"`               // starter, professional, enterprise
	Status              string     `json:"status"`             // trialing, active, past_due, cancelled, suspended
	BillingCycle        string     `json:"billing_cycle"`      // monthly, annual
	CurrentPeriodStart  time.Time  `json:"current_period_start"`
	CurrentPeriodEnd    time.Time  `json:"current_period_end"`
	TrialEndsAt         *time.Time `json:"trial_ends_at,omitempty"`
	CancelledAt         *time.Time `json:"cancelled_at,omitempty"`
	SuspendedAt         *time.Time `json:"suspended_at,omitempty"`
	RazorpaySubID       string     `json:"razorpay_sub_id,omitempty"`
	StripeSubID         string     `json:"stripe_sub_id,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// Invoice records a billing invoice for one subscription period.
type Invoice struct {
	ID             uuid.UUID       `json:"id"`
	TenantID       uuid.UUID       `json:"tenant_id"`
	SubscriptionID uuid.UUID       `json:"subscription_id"`
	InvoiceNumber  string          `json:"invoice_number"` // INV-YYYY-NNNNN
	PeriodStart    time.Time       `json:"period_start"`
	PeriodEnd      time.Time       `json:"period_end"`
	Plan           string          `json:"plan"`
	Amount         decimal.Decimal `json:"amount"`
	TaxAmount      decimal.Decimal `json:"tax_amount"`
	TotalAmount    decimal.Decimal `json:"total_amount"`
	Currency       string          `json:"currency"` // INR, USD
	Status         string          `json:"status"`   // pending, paid, overdue, void
	DueDate        time.Time       `json:"due_date"`
	PaidAt         *time.Time      `json:"paid_at,omitempty"`
	PaymentMethod  string          `json:"payment_method,omitempty"`
	GatewayTxnID   string          `json:"gateway_txn_id,omitempty"`
	PDFDocumentID  *uuid.UUID      `json:"pdf_document_id,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

// PaymentMethod is a stored payment instrument for a tenant.
type PaymentMethod struct {
	ID            uuid.UUID  `json:"id"`
	TenantID      uuid.UUID  `json:"tenant_id"`
	Type          string     `json:"type"`          // card, upi, netbanking, bank_transfer
	DisplayName   string     `json:"display_name"`  // e.g. "Visa ending 4242"
	IsDefault     bool       `json:"is_default"`
	RazorpayToken string     `json:"razorpay_token,omitempty"` // never returned to clients
	StripePMID    string     `json:"stripe_pm_id,omitempty"`   // never returned to clients
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// UsageRecord captures a tenant's metered resource consumption for a period.
// Collected nightly by a scheduled job that queries each service's database.
type UsageRecord struct {
	ID                uuid.UUID       `json:"id"`
	TenantID          uuid.UUID       `json:"tenant_id"`
	PeriodStart       time.Time       `json:"period_start"`
	PeriodEnd         time.Time       `json:"period_end"`
	ActiveProductions int             `json:"active_productions"`
	UserCount         int             `json:"user_count"`
	StorageGB         decimal.Decimal `json:"storage_gb"`
	RecordedAt        time.Time       `json:"recorded_at"`
}

// DunningAttempt records one payment retry attempt for an overdue invoice.
type DunningAttempt struct {
	ID            uuid.UUID  `json:"id"`
	InvoiceID     uuid.UUID  `json:"invoice_id"`
	AttemptNumber int        `json:"attempt_number"`
	AttemptedAt   time.Time  `json:"attempted_at"`
	Result        string     `json:"result"` // success, failed, card_declined, insufficient_funds
	NextRetryAt   *time.Time `json:"next_retry_at,omitempty"`
}

// PlanLimits defines the resource ceiling for a subscription plan.
// -1 means unlimited.
type PlanLimits struct {
	MaxActiveProductions int
	MaxUsers             int
	MaxStorageGB         int
}

// PlanLimitsByName is the authoritative map of plan → resource limits.
// Amounts must match the pricing table in docs/developers/services/billing.md.
var PlanLimitsByName = map[string]PlanLimits{
	"starter":      {MaxActiveProductions: 2, MaxUsers: 10, MaxStorageGB: 5},
	"professional": {MaxActiveProductions: 10, MaxUsers: 50, MaxStorageGB: 50},
	"enterprise":   {MaxActiveProductions: -1, MaxUsers: -1, MaxStorageGB: -1},
}

// CheckLimitRequest carries the tenant's current resource usage to validate
// against their plan. Used by the IAM interceptor before allowing resource creation.
type CheckLimitRequest struct {
	TenantID          uuid.UUID
	Resource          string // "production", "user"
	CurrentProductions int
	CurrentUsers       int
}

// LimitResult is the response from CheckPlanLimit.
type LimitResult struct {
	Allowed bool
	Reason  string
}

// UsageSummary packages the latest usage record alongside the plan limits
// so the frontend can render usage gauges.
type UsageSummary struct {
	TenantID   uuid.UUID
	Plan       string
	Usage      UsageRecord
	Limits     PlanLimits
	UpdatedAt  time.Time
}

// TrialDays is the length of the initial trial period for new subscriptions.
const TrialDays = 14
