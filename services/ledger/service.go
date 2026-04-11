package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// EventPublisher publishes NATS domain events from the ledger service.
// Nil is accepted — event publishing is best-effort and never fails the domain op.
type EventPublisher interface {
	PublishJournalPosted(ctx context.Context, je *JournalEntry) error
}

// Service implements general-ledger business logic.
// It is a universal service — no vertical config is loaded per request.
type Service struct {
	repo      Repository
	publisher EventPublisher
}

// NewService creates a general-ledger service.
// pub is optional; omit it (or pass nil) in tests.
func NewService(repo Repository, pub ...EventPublisher) *Service {
	var p EventPublisher
	if len(pub) > 0 {
		p = pub[0]
	}
	return &Service{repo: repo, publisher: p}
}

// --- Accounts ---

// CreateAccount creates a new account in the chart of accounts.
func (s *Service) CreateAccount(ctx context.Context, account *Account) (*Account, error) {
	if !isValidAccountType(account.AccountType) {
		return nil, ErrInvalidAccountType
	}
	if account.ID == uuid.Nil {
		account.ID = uuid.New()
	}
	account.IsActive = true
	if err := s.repo.CreateAccount(ctx, account); err != nil {
		return nil, fmt.Errorf("ledger: create account %s: %w", account.Code, err)
	}
	return account, nil
}

// GetAccount retrieves an account by ID.
func (s *Service) GetAccount(ctx context.Context, tenantID, id uuid.UUID) (*Account, error) {
	a, err := s.repo.GetAccountByID(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("ledger: get account %s: %w", id, err)
	}
	return a, nil
}

// ListAccounts returns all accounts for a tenant.
func (s *Service) ListAccounts(ctx context.Context, tenantID uuid.UUID) ([]Account, error) {
	accounts, err := s.repo.ListAccounts(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("ledger: list accounts: %w", err)
	}
	return accounts, nil
}

// SeedChartOfAccounts creates the default accounts for a tenant from a vertical's
// chart-of-accounts definition. Idempotent: existing codes are skipped.
// Implements registration.LedgerSeeder via a gRPC client adapter in production.
func (s *Service) SeedChartOfAccounts(ctx context.Context, tenantID uuid.UUID, entries []vertical.ChartOfAccountEntry) error {
	for _, entry := range entries {
		var parentID *uuid.UUID
		if entry.ParentCode != nil {
			parent, err := s.repo.GetAccountByCode(ctx, tenantID, *entry.ParentCode)
			if err != nil {
				return fmt.Errorf("ledger: seed CoA — resolve parent code %q: %w", *entry.ParentCode, err)
			}
			parentID = &parent.ID
		}

		account := &Account{
			ID:          uuid.New(),
			TenantID:    tenantID,
			Code:        entry.Code,
			Name:        entry.Name,
			AccountType: entry.AccountType,
			ParentID:    parentID,
			IsActive:    true,
		}
		if err := s.repo.CreateAccount(ctx, account); err != nil {
			// Idempotent: if the account already exists, skip it.
			// The concrete repository signals this via ErrAccountCodeExists.
			if isAccountCodeExists(err) {
				continue
			}
			return fmt.Errorf("ledger: seed CoA — create account %s: %w", entry.Code, err)
		}
	}
	return nil
}

// --- Accounting Periods ---

// OpenAccountingPeriod creates a new open accounting period for the given
// tenant/year/month. Idempotent: returns nil if the period already exists.
func (s *Service) OpenAccountingPeriod(ctx context.Context, tenantID uuid.UUID, year int, month time.Month) error {
	period := &AccountingPeriod{
		ID:       uuid.New(),
		TenantID: tenantID,
		Year:     year,
		Month:    int(month),
		Status:   "open",
	}
	if err := s.repo.CreatePeriod(ctx, period); err != nil {
		if isPeriodAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("ledger: open accounting period %d/%02d: %w", year, month, err)
	}
	return nil
}

// CloseAccountingPeriod marks a period as closed, preventing new postings.
func (s *Service) CloseAccountingPeriod(ctx context.Context, tenantID, periodID, closedBy uuid.UUID) (*AccountingPeriod, error) {
	period, err := s.repo.GetPeriodByID(ctx, tenantID, periodID)
	if err != nil {
		return nil, fmt.Errorf("ledger: get period: %w", err)
	}
	if period.Status == "closed" {
		return period, nil // idempotent
	}
	now := time.Now().UTC()
	if err := s.repo.ClosePeriod(ctx, tenantID, periodID, closedBy, now); err != nil {
		return nil, fmt.Errorf("ledger: close period %s: %w", periodID, err)
	}
	period.Status = "closed"
	period.ClosedBy = &closedBy
	period.ClosedAt = &now
	return period, nil
}

// --- Journal Entries ---

// CreateJournalEntry creates a draft journal entry with its lines.
// Lines are validated: at least two required, all accounts must exist.
func (s *Service) CreateJournalEntry(ctx context.Context, je *JournalEntry) (*JournalEntry, error) {
	if len(je.Lines) < 2 {
		return nil, ErrJournalTooFewLines
	}
	// Validate all referenced accounts exist and are active.
	for i, line := range je.Lines {
		acct, err := s.repo.GetAccountByID(ctx, je.TenantID, line.AccountID)
		if err != nil {
			return nil, fmt.Errorf("ledger: line %d — %w", i, ErrAccountNotFound)
		}
		if !acct.IsActive {
			return nil, fmt.Errorf("ledger: line %d account %s — %w", i, acct.Code, ErrAccountInactive)
		}
		if line.ID == uuid.Nil {
			je.Lines[i].ID = uuid.New()
		}
	}

	if je.ID == uuid.Nil {
		je.ID = uuid.New()
	}
	je.Status = "draft"

	// Allocate an immutable entry number.
	year := time.Now().Year()
	entryNumber, err := s.repo.AllocateEntryNumber(ctx, je.TenantID, year)
	if err != nil {
		return nil, fmt.Errorf("ledger: allocate entry number: %w", err)
	}
	je.EntryNumber = entryNumber

	if err := s.repo.CreateJournalEntry(ctx, je); err != nil {
		return nil, fmt.Errorf("ledger: create journal entry: %w", err)
	}
	return je, nil
}

// PostJournalEntry validates balance and period status, then marks the entry posted.
// Posted entries are immutable — only VoidJournalEntry can alter them.
func (s *Service) PostJournalEntry(ctx context.Context, tenantID, id, postedBy uuid.UUID) (*JournalEntry, error) {
	je, err := s.repo.GetJournalEntry(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("ledger: get journal entry: %w", err)
	}
	if je.Status == "posted" {
		return nil, ErrJournalAlreadyPosted
	}
	if je.Status == "void" {
		return nil, ErrJournalVoid
	}

	// Validate period is open.
	period, err := s.repo.GetPeriodByID(ctx, tenantID, je.PeriodID)
	if err != nil {
		return nil, fmt.Errorf("ledger: get period for posting: %w", err)
	}
	if period.Status == "closed" {
		return nil, ErrPeriodClosed
	}

	// Validate balance: sum(debit) must equal sum(credit).
	if err := validateBalance(je.Lines); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	if err := s.repo.UpdateJournalStatus(ctx, id, "posted", postedBy, now); err != nil {
		return nil, fmt.Errorf("ledger: post journal entry %s: %w", id, err)
	}
	je.Status = "posted"
	je.PostedBy = &postedBy
	je.PostedAt = &now
	s.publish(ctx, func() error { return s.publisher.PublishJournalPosted(ctx, je) })
	return je, nil
}

// GetJournalEntry retrieves a journal entry with its lines.
func (s *Service) GetJournalEntry(ctx context.Context, tenantID, id uuid.UUID) (*JournalEntry, error) {
	je, err := s.repo.GetJournalEntry(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("ledger: get journal entry %s: %w", id, err)
	}
	return je, nil
}

// ListJournalEntries lists journal entries with optional period and status filters.
func (s *Service) ListJournalEntries(ctx context.Context, tenantID uuid.UUID, periodID *uuid.UUID, status string, limit, offset int) ([]JournalEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	entries, err := s.repo.ListJournalEntries(ctx, tenantID, periodID, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("ledger: list journal entries: %w", err)
	}
	return entries, nil
}

// VoidJournalEntry marks a posted entry as void and creates a reversing entry
// to cancel the financial effect. Returns the new reversing entry.
func (s *Service) VoidJournalEntry(ctx context.Context, tenantID, id, voidedBy uuid.UUID) (*JournalEntry, error) {
	original, err := s.repo.GetJournalEntry(ctx, tenantID, id)
	if err != nil {
		return nil, fmt.Errorf("ledger: get journal entry for void: %w", err)
	}
	if original.Status == "void" {
		return nil, ErrJournalVoid
	}
	if original.Status != "posted" {
		return nil, fmt.Errorf("ledger: only posted entries can be voided")
	}

	// Build the reversing entry (swap DR and CR on every line).
	reversingLines := make([]JournalLine, len(original.Lines))
	for i, line := range original.Lines {
		reversingLines[i] = JournalLine{
			ID:           uuid.New(),
			AccountID:    line.AccountID,
			DebitAmount:  line.CreditAmount,
			CreditAmount: line.DebitAmount,
			Currency:     line.Currency,
			Description:  fmt.Sprintf("REVERSAL: %s", line.Description),
		}
	}

	reversing := &JournalEntry{
		TenantID:     tenantID,
		ProductionID: original.ProductionID,
		PeriodID:     original.PeriodID,
		Reference:    original.EntryNumber, // link back to original
		Narration:    fmt.Sprintf("REVERSAL OF %s: %s", original.EntryNumber, original.Narration),
		Lines:        reversingLines,
	}

	// CreateJournalEntry allocates the entry number and validates lines.
	reversing, err = s.CreateJournalEntry(ctx, reversing)
	if err != nil {
		return nil, fmt.Errorf("ledger: create reversing entry: %w", err)
	}
	// Post the reversing entry immediately.
	reversing, err = s.PostJournalEntry(ctx, tenantID, reversing.ID, voidedBy)
	if err != nil {
		return nil, fmt.Errorf("ledger: post reversing entry: %w", err)
	}

	// Mark the original as void.
	now := time.Now().UTC()
	if err := s.repo.UpdateJournalStatus(ctx, id, "void", voidedBy, now); err != nil {
		return nil, fmt.Errorf("ledger: mark entry void: %w", err)
	}

	return reversing, nil
}

// GetTrialBalance returns a trial balance for all accounts as of asOf.
func (s *Service) GetTrialBalance(ctx context.Context, tenantID uuid.UUID, asOf time.Time) ([]TrialBalanceEntry, error) {
	entries, err := s.repo.GetTrialBalance(ctx, tenantID, asOf)
	if err != nil {
		return nil, fmt.Errorf("ledger: get trial balance: %w", err)
	}
	return entries, nil
}

// --- Helpers ---

func validateBalance(lines []JournalLine) error {
	var totalDebit, totalCredit decimal.Decimal
	for _, l := range lines {
		totalDebit = totalDebit.Add(l.DebitAmount)
		totalCredit = totalCredit.Add(l.CreditAmount)
	}
	if !totalDebit.Equal(totalCredit) {
		return fmt.Errorf("%w: debit=%s credit=%s", ErrJournalNotBalanced, totalDebit, totalCredit)
	}
	return nil
}

func isValidAccountType(t string) bool {
	switch t {
	case "asset", "liability", "equity", "revenue", "expense":
		return true
	}
	return false
}

// isAccountCodeExists detects the duplicate-code sentinel so SeedChartOfAccounts
// can skip existing accounts without aborting the whole seed.
// The concrete repository wraps ErrAccountCodeExists on ON CONFLICT.
func isAccountCodeExists(err error) bool {
	return err == ErrAccountCodeExists
}

// isPeriodAlreadyExists detects the duplicate-period sentinel for idempotent
// OpenAccountingPeriod calls.
func isPeriodAlreadyExists(err error) bool {
	return err == ErrPeriodAlreadyExists
}

// publish calls fn only when a publisher is configured; errors are best-effort.
func (s *Service) publish(ctx context.Context, fn func() error) {
	if s.publisher == nil {
		return
	}
	_ = fn()
}
