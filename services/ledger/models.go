// Package ledger implements the general-ledger service.
// It is a universal service (not vertical-aware) and is the source of truth
// for all financial transactions across the platform.
package ledger

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Account is a node in the chart of accounts hierarchy.
type Account struct {
	ID          uuid.UUID  `json:"id"`
	TenantID    uuid.UUID  `json:"tenant_id"`
	Code        string     `json:"code"`
	Name        string     `json:"name"`
	AccountType string     `json:"account_type"` // asset|liability|equity|revenue|expense
	ParentID    *uuid.UUID `json:"parent_id,omitempty"`
	IsActive    bool       `json:"is_active"`
}

// AccountingPeriod represents a calendar month for a tenant.
// Posting journal entries to a closed period is forbidden.
type AccountingPeriod struct {
	ID       uuid.UUID  `json:"id"`
	TenantID uuid.UUID  `json:"tenant_id"`
	Year     int        `json:"year"`
	Month    int        `json:"month"`
	Status   string     `json:"status"` // open|closed
	ClosedBy *uuid.UUID `json:"closed_by,omitempty"`
	ClosedAt *time.Time `json:"closed_at,omitempty"`
}

// JournalEntry is the header for a double-entry transaction.
// Once posted it is immutable; corrections require a reversing entry.
type JournalEntry struct {
	ID           uuid.UUID    `json:"id"`
	TenantID     uuid.UUID    `json:"tenant_id"`
	ProductionID *uuid.UUID   `json:"production_id,omitempty"`
	PeriodID     uuid.UUID    `json:"period_id"`
	EntryNumber  string       `json:"entry_number"` // JE-2024-00001
	Reference    string       `json:"reference,omitempty"`
	Narration    string       `json:"narration"`
	Status       string       `json:"status"` // draft|posted|void
	Lines        []JournalLine `json:"lines,omitempty"`
	PostedBy     *uuid.UUID   `json:"posted_by,omitempty"`
	PostedAt     *time.Time   `json:"posted_at,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
}

// JournalLine is one side of a double-entry transaction.
// Exactly one of DebitAmount or CreditAmount is non-zero.
type JournalLine struct {
	ID           uuid.UUID       `json:"id"`
	JournalID    uuid.UUID       `json:"journal_id"`
	AccountID    uuid.UUID       `json:"account_id"`
	DebitAmount  decimal.Decimal `json:"debit_amount"`
	CreditAmount decimal.Decimal `json:"credit_amount"`
	Currency     string          `json:"currency"`
	Description  string          `json:"description,omitempty"`
}

// TrialBalanceEntry is a computed row in the trial balance report.
type TrialBalanceEntry struct {
	AccountID   uuid.UUID       `json:"account_id"`
	Code        string          `json:"code"`
	Name        string          `json:"name"`
	AccountType string          `json:"account_type"`
	Debit       decimal.Decimal `json:"debit"`
	Credit      decimal.Decimal `json:"credit"`
	Balance     decimal.Decimal `json:"balance"` // Debit − Credit
}
