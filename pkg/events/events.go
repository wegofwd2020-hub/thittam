// Package events defines the shared event envelope and domain event payloads
// published to NATS JetStream by Thittam services.
//
// # Subjects
//
// All subjects follow the pattern "thittam.<domain>.<action>".
// Reporting-analytics subscribes to a durable consumer on "thittam.>" so
// it receives every domain event in a single subscription.
//
// # Envelope
//
// Every NATS message is a JSON-encoded Envelope. The Type field identifies
// which concrete payload struct the Payload bytes should be decoded into.
// Consumers must skip (Ack) unknown event types to remain forward-compatible.
package events

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Well-known subject constants. Publishers and consumers both reference these
// to avoid subject-string drift.
const (
	SubjectExpenseSubmitted   = "thittam.expense.submitted"
	SubjectExpenseApproved    = "thittam.expense.approved"
	SubjectBudgetApproved     = "thittam.budget.approved"
	SubjectProjectCreated     = "thittam.project.created"
	SubjectProjectStatusChanged = "thittam.project.status_changed"
	SubjectProjectMemberAssigned = "thittam.project.member_assigned"
	SubjectTenantCreated      = "thittam.iam.tenant.created"
	SubjectLedgerJournalPosted = "thittam.ledger.journal.posted"

	// Billing domain events
	SubjectBillingSubscriptionCreated   = "thittam.billing.subscription.created"
	SubjectBillingSubscriptionUpgraded  = "thittam.billing.subscription.upgraded"
	SubjectBillingSubscriptionCancelled = "thittam.billing.subscription.cancelled"
	SubjectBillingSubscriptionSuspended = "thittam.billing.subscription.suspended"
	SubjectBillingInvoicePaid           = "thittam.billing.invoice.paid"
	SubjectBillingInvoiceOverdue        = "thittam.billing.invoice.overdue"

	// Document domain events
	SubjectDocumentSigned = "thittam.document.signed"
)

// Envelope wraps every domain event published to NATS JetStream.
// Consumers decode the outer envelope first, switch on Type, then decode
// Payload into the matching struct.
type Envelope struct {
	// EventID is a stable deduplication key. Consumers use this to detect and
	// discard duplicate deliveries (NATS at-least-once delivery guarantee).
	EventID uuid.UUID `json:"event_id"`

	// Type identifies the concrete payload. Must match a well-known constant
	// (e.g. SubjectExpenseApproved). Unknown types must be acknowledged and skipped.
	Type string `json:"type"`

	// TenantID scopes the event. Consumers that maintain per-tenant projections
	// use this to route to the correct tenant schema.
	TenantID uuid.UUID `json:"tenant_id"`

	// OccurredAt is when the domain event happened, in UTC.
	// Used by LagChecker to measure projection staleness.
	OccurredAt time.Time `json:"occurred_at"`

	// Payload is the JSON-encoded event-specific struct.
	Payload json.RawMessage `json:"payload"`
}

// NewEnvelope wraps a typed payload into an Envelope.
// eventType should be one of the Subject* constants.
func NewEnvelope(eventType string, tenantID uuid.UUID, payload interface{}) (*Envelope, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("events: marshal payload: %w", err)
	}
	return &Envelope{
		EventID:    uuid.New(),
		Type:       eventType,
		TenantID:   tenantID,
		OccurredAt: time.Now().UTC(),
		Payload:    data,
	}, nil
}

// --- Expense domain events ---

// ExpenseSubmittedPayload is published when a new expense is raised and awaits approval.
// Subject: SubjectExpenseSubmitted
type ExpenseSubmittedPayload struct {
	ExpenseID    uuid.UUID `json:"expense_id"`
	ProductionID uuid.UUID `json:"production_id"`
	CategoryID   string    `json:"category_id"`
	// Amount serialized as string per Rule #1 (money is never a float).
	Amount       string    `json:"amount"`
	SubmittedBy  string    `json:"submitted_by"` // display name
	SubmittedAt  time.Time `json:"submitted_at"`
	Description  string    `json:"description"`
	ProjectTitle string    `json:"project_title"`
}

// ExpenseApprovedPayload is published when an expense is approved and committed.
// Subject: SubjectExpenseApproved
type ExpenseApprovedPayload struct {
	ExpenseID    uuid.UUID  `json:"expense_id"`
	ProductionID uuid.UUID  `json:"production_id"`
	BudgetLineID *uuid.UUID `json:"budget_line_id,omitempty"`
	CategoryID   string     `json:"category_id"`
	CategoryName string     `json:"category_name"`
	// Amount serialized as string per Rule #1.
	Amount     string    `json:"amount"`
	ApprovedAt time.Time `json:"approved_at"`
	// Month is "YYYY-MM" — used to update the monthly_spend projection.
	Month string `json:"month"`
}

// --- Budget domain events ---

// BudgetApprovedPayload is published when a budget version is approved.
// Subject: SubjectBudgetApproved
type BudgetApprovedPayload struct {
	BudgetID     uuid.UUID `json:"budget_id"`
	ProductionID uuid.UUID `json:"production_id"`
	// Amounts serialized as strings per Rule #1.
	TotalBudgeted  string `json:"total_budgeted"`
	TotalActual    string `json:"total_actual"`
	TotalCommitted string `json:"total_committed"`
}

// --- Project domain events ---

// ProjectCreatedPayload is published when a new production/project is created.
// Subject: SubjectProjectCreated
type ProjectCreatedPayload struct {
	ProjectID uuid.UUID `json:"project_id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	StartDate *string   `json:"start_date,omitempty"` // "YYYY-MM-DD"
}

// ProjectStatusChangedPayload is published when a project transitions status.
// Subject: SubjectProjectStatusChanged
type ProjectStatusChangedPayload struct {
	ProjectID    uuid.UUID `json:"project_id"`
	Title        string    `json:"title"`
	OldStatus    string    `json:"old_status"`
	NewStatus    string    `json:"new_status"`
	CurrentPhase string    `json:"current_phase"`
}

// --- Ledger domain events ---

// LedgerJournalPostedPayload is published when a journal entry is posted to
// the general ledger. This is a financial event: silent loss is a critical risk
// because the ledger would fall out of sync with the originating service.
// Subject: SubjectLedgerJournalPosted
type LedgerJournalPostedPayload struct {
	JournalEntryID string    `json:"journal_entry_id"`
	EntryNumber    string    `json:"entry_number"` // e.g. "JE-2024-00001"
	ProductionID   string    `json:"production_id,omitempty"`
	PeriodID       string    `json:"period_id"`
	// TotalDebit and TotalCredit are serialized as strings per Rule #1.
	TotalDebit  string    `json:"total_debit"`
	TotalCredit string    `json:"total_credit"`
	Narration   string    `json:"narration"`
	PostedAt    time.Time `json:"posted_at"`
}

// --- Project domain events ---

// --- Billing domain events ---

// BillingSubscriptionCreatedPayload is published when a new tenant subscription is created.
// Subject: SubjectBillingSubscriptionCreated
type BillingSubscriptionCreatedPayload struct {
	SubscriptionID string `json:"subscription_id"`
	Plan           string `json:"plan"`
	BillingCycle   string `json:"billing_cycle"` // monthly, annual
	TrialEndsAt    string `json:"trial_ends_at"` // RFC 3339
}

// BillingSubscriptionUpgradedPayload is published when a tenant changes their plan.
// Subject: SubjectBillingSubscriptionUpgraded
type BillingSubscriptionUpgradedPayload struct {
	SubscriptionID string `json:"subscription_id"`
	OldPlan        string `json:"old_plan"`
	NewPlan        string `json:"new_plan"`
}

// BillingSubscriptionCancelledPayload is published when a tenant cancels.
// Subject: SubjectBillingSubscriptionCancelled
type BillingSubscriptionCancelledPayload struct {
	SubscriptionID string `json:"subscription_id"`
	Plan           string `json:"plan"`
	CancelledAt    string `json:"cancelled_at"` // RFC 3339
	// ActiveUntil is CurrentPeriodEnd — access continues until this date.
	ActiveUntil string `json:"active_until"` // RFC 3339
}

// BillingSubscriptionSuspendedPayload is published when an account is suspended
// after exhausting dunning retries.
// Subject: SubjectBillingSubscriptionSuspended
type BillingSubscriptionSuspendedPayload struct {
	SubscriptionID string `json:"subscription_id"`
	SuspendedAt    string `json:"suspended_at"` // RFC 3339
	// PurgeAfter is 30 days after suspension — tenant data is deleted on this date.
	PurgeAfter string `json:"purge_after"` // RFC 3339
}

// BillingInvoicePaidPayload is published when a payment succeeds.
// Subject: SubjectBillingInvoicePaid
type BillingInvoicePaidPayload struct {
	InvoiceID     string `json:"invoice_id"`
	InvoiceNumber string `json:"invoice_number"` // INV-YYYY-NNNNN
	// TotalAmount serialized as string per Rule #1.
	TotalAmount  string `json:"total_amount"`
	Currency     string `json:"currency"`
	GatewayTxnID string `json:"gateway_txn_id"`
	PaidAt       string `json:"paid_at"` // RFC 3339
}

// BillingInvoiceOverduePayload is published when an invoice becomes overdue
// after all dunning retries fail.
// Subject: SubjectBillingInvoiceOverdue
type BillingInvoiceOverduePayload struct {
	InvoiceID     string `json:"invoice_id"`
	InvoiceNumber string `json:"invoice_number"`
	// TotalAmount serialized as string per Rule #1.
	TotalAmount string `json:"total_amount"`
	DunningAttempts int `json:"dunning_attempts"`
}

// DocumentSignedPayload is published when an e-signature is completed.
// Subject: SubjectDocumentSigned
type DocumentSignedPayload struct {
	DocumentID   string    `json:"document_id"`
	DocumentName string    `json:"document_name"`
	SignedBy     uuid.UUID `json:"signed_by"`
	SignedAt     time.Time `json:"signed_at"`
}

// ProjectMemberAssignedPayload is published when a crew member is assigned to a project.
// Subject: SubjectProjectMemberAssigned
type ProjectMemberAssignedPayload struct {
	ProjectID    uuid.UUID `json:"project_id"`
	ProjectTitle string    `json:"project_title"`
	// MemberCount is the new total member count after this assignment.
	MemberCount int `json:"member_count"`
}
