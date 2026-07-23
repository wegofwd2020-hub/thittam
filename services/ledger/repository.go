package ledger

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository defines all data access required by the general-ledger service.
type Repository interface {
	// Accounts
	CreateAccount(ctx context.Context, account *Account) error
	GetAccountByID(ctx context.Context, tenantID, id uuid.UUID) (*Account, error)
	GetAccountByCode(ctx context.Context, tenantID uuid.UUID, code string) (*Account, error)
	ListAccounts(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]Account, error)

	// Accounting Periods
	CreatePeriod(ctx context.Context, period *AccountingPeriod) error
	GetPeriodByID(ctx context.Context, tenantID, id uuid.UUID) (*AccountingPeriod, error)
	GetOpenPeriod(ctx context.Context, tenantID uuid.UUID, year, month int) (*AccountingPeriod, error)
	ClosePeriod(ctx context.Context, tenantID, periodID, closedBy uuid.UUID, closedAt time.Time) error

	// Journal Entries
	// AllocateEntryNumber atomically reserves the next sequence number for the tenant/year
	// and returns a formatted entry number (e.g. "JE-2024-00001").
	AllocateEntryNumber(ctx context.Context, tenantID uuid.UUID, year int) (string, error)
	CreateJournalEntry(ctx context.Context, je *JournalEntry) error
	GetJournalEntry(ctx context.Context, tenantID, id uuid.UUID) (*JournalEntry, error)
	ListJournalEntries(ctx context.Context, tenantID uuid.UUID, periodID *uuid.UUID, status string, limit, offset int) ([]JournalEntry, error)
	UpdateJournalStatus(ctx context.Context, tenantID, id uuid.UUID, status string, actorID uuid.UUID, at time.Time) error

	// Trial Balance
	GetTrialBalance(ctx context.Context, tenantID uuid.UUID, asOf time.Time) ([]TrialBalanceEntry, error)
}
