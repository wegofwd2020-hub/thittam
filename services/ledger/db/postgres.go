package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/wegofwd2020/thittam/services/ledger"
)

// Postgres implements ledger.Repository backed by PostgreSQL.
type Postgres struct {
	q  *Queries
	db *pgxpool.Pool
}

// NewPostgres creates a Postgres-backed general-ledger repository.
func NewPostgres(db *pgxpool.Pool) *Postgres {
	return &Postgres{q: New(db), db: db}
}

// Compile-time interface check.
var _ ledger.Repository = (*Postgres)(nil)

// --- Accounts ---

func (p *Postgres) CreateAccount(ctx context.Context, account *ledger.Account) error {
	row, err := p.q.CreateAccount(ctx, CreateAccountParams{
		ID:          account.ID,
		TenantID:    account.TenantID,
		Code:        account.Code,
		Name:        account.Name,
		AccountType: account.AccountType,
		ParentID:    pgUUIDFromPtr(account.ParentID),
		IsActive:    account.IsActive,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return ledger.ErrAccountCodeExists
		}
		return fmt.Errorf("ledger: create account: %w", err)
	}
	// ON CONFLICT DO NOTHING returns no rows on conflict.
	if row.ID == uuid.Nil {
		return ledger.ErrAccountCodeExists
	}
	// Populate generated fields back into the struct.
	account.ID = row.ID
	account.IsActive = row.IsActive
	return nil
}

func (p *Postgres) GetAccountByID(ctx context.Context, tenantID, id uuid.UUID) (*ledger.Account, error) {
	row, err := p.q.GetAccount(ctx, GetAccountParams{ID: id, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ledger.ErrAccountNotFound
		}
		return nil, fmt.Errorf("ledger: get account by ID: %w", err)
	}
	return accountFromDB(row), nil
}

func (p *Postgres) GetAccountByCode(ctx context.Context, tenantID uuid.UUID, code string) (*ledger.Account, error) {
	row, err := p.q.GetAccountByCode(ctx, GetAccountByCodeParams{Code: code, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ledger.ErrAccountNotFound
		}
		return nil, fmt.Errorf("ledger: get account by code: %w", err)
	}
	return accountFromDB(row), nil
}

func (p *Postgres) ListAccounts(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]ledger.Account, error) {
	// Pass "" as the account_type filter to return all account types.
	rows, err := p.q.ListAccounts(ctx, ListAccountsParams{
		TenantID: tenantID,
		Column2:  "",
		Limit:    int32(limit),
		Offset:   int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("ledger: list accounts: %w", err)
	}
	result := make([]ledger.Account, len(rows))
	for i, row := range rows {
		result[i] = *accountFromDB(row)
	}
	return result, nil
}

// --- Accounting Periods ---

func (p *Postgres) CreatePeriod(ctx context.Context, period *ledger.AccountingPeriod) error {
	_, err := p.q.CreateAccountingPeriod(ctx, CreateAccountingPeriodParams{
		ID:       period.ID,
		TenantID: period.TenantID,
		Year:     int32(period.Year),
		Month:    int32(period.Month),
		Status:   period.Status,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return ledger.ErrPeriodAlreadyExists
		}
		return fmt.Errorf("ledger: create accounting period: %w", err)
	}
	return nil
}

func (p *Postgres) GetPeriodByID(ctx context.Context, tenantID, id uuid.UUID) (*ledger.AccountingPeriod, error) {
	row, err := p.q.GetAccountingPeriod(ctx, GetAccountingPeriodParams{ID: id, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ledger.ErrPeriodNotFound
		}
		return nil, fmt.Errorf("ledger: get period by ID: %w", err)
	}
	return periodFromDB(row), nil
}

// GetOpenPeriod returns the most recent open period for the tenant.
// The year/month parameters are accepted by the interface but are not used
// in the current SQL query — it returns the most recently opened period.
func (p *Postgres) GetOpenPeriod(ctx context.Context, tenantID uuid.UUID, _, _ int) (*ledger.AccountingPeriod, error) {
	row, err := p.q.GetOpenPeriod(ctx, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ledger.ErrPeriodNotFound
		}
		return nil, fmt.Errorf("ledger: get open period: %w", err)
	}
	return periodFromDB(row), nil
}

func (p *Postgres) ClosePeriod(ctx context.Context, tenantID, periodID, closedBy uuid.UUID, _ time.Time) error {
	// The SQL uses now() for closed_at; the closedAt parameter is accepted for
	// interface compatibility but is not forwarded.
	_, err := p.q.CloseAccountingPeriod(ctx, CloseAccountingPeriodParams{
		ID:       periodID,
		TenantID: tenantID,
		ClosedBy: pgtype.UUID{Bytes: closedBy, Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ledger.ErrPeriodNotFound
		}
		return fmt.Errorf("ledger: close period: %w", err)
	}
	return nil
}

// --- Journal Entries ---

// AllocateEntryNumber atomically increments the per-tenant/year counter and
// returns a formatted entry number such as "JE-2024-00001".
func (p *Postgres) AllocateEntryNumber(ctx context.Context, tenantID uuid.UUID, year int) (string, error) {
	seq, err := p.q.NextJournalEntrySeq(ctx, NextJournalEntrySeqParams{
		TenantID: tenantID,
		Year:     int32(year),
	})
	if err != nil {
		return "", fmt.Errorf("ledger: allocate entry number: %w", err)
	}
	return fmt.Sprintf("JE-%d-%05d", year, seq), nil
}

// CreateJournalEntry inserts the journal header and all its lines atomically.
func (p *Postgres) CreateJournalEntry(ctx context.Context, je *ledger.JournalEntry) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ledger: begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := p.q.WithTx(tx)

	_, err = q.CreateJournalEntry(ctx, CreateJournalEntryParams{
		ID:           je.ID,
		TenantID:     je.TenantID,
		ProductionID: pgUUIDFromPtr(je.ProductionID),
		PeriodID:     je.PeriodID,
		EntryNumber:  je.EntryNumber,
		Reference:    pgTextFromString(je.Reference),
		Narration:    je.Narration,
		Status:       je.Status,
	})
	if err != nil {
		return fmt.Errorf("ledger: insert journal entry header: %w", err)
	}

	for i, line := range je.Lines {
		_, err = q.CreateJournalLine(ctx, CreateJournalLineParams{
			ID:           line.ID,
			JournalID:    je.ID,
			AccountID:    line.AccountID,
			DebitAmount:  pgNumericFromDecimal(line.DebitAmount),
			CreditAmount: pgNumericFromDecimal(line.CreditAmount),
			Description:  pgTextFromString(line.Description),
		})
		if err != nil {
			return fmt.Errorf("ledger: insert journal line %d: %w", i, err)
		}
	}

	return tx.Commit(ctx)
}

func (p *Postgres) GetJournalEntry(ctx context.Context, tenantID, id uuid.UUID) (*ledger.JournalEntry, error) {
	row, err := p.q.GetJournalEntry(ctx, GetJournalEntryParams{ID: id, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ledger.ErrJournalNotFound
		}
		return nil, fmt.Errorf("ledger: get journal entry: %w", err)
	}

	je := journalEntryFromDB(row)

	lineRows, err := p.q.ListJournalLines(ctx, ListJournalLinesParams{JournalID: je.ID, TenantID: tenantID})
	if err != nil {
		return nil, fmt.Errorf("ledger: list journal lines for %s: %w", je.ID, err)
	}
	je.Lines = make([]ledger.JournalLine, len(lineRows))
	for i, lr := range lineRows {
		je.Lines[i] = journalLineFromDB(lr)
	}

	return je, nil
}

// ListJournalEntries returns journal entry headers without lines (lightweight for listing).
func (p *Postgres) ListJournalEntries(ctx context.Context, tenantID uuid.UUID, periodID *uuid.UUID, status string, limit, offset int) ([]ledger.JournalEntry, error) {
	var col2 uuid.UUID
	if periodID != nil {
		col2 = *periodID
	}

	rows, err := p.q.ListJournalEntries(ctx, ListJournalEntriesParams{
		TenantID: tenantID,
		Column2:  col2,
		Column3:  status,
		Limit:    int32(limit),
		Offset:   int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("ledger: list journal entries: %w", err)
	}

	result := make([]ledger.JournalEntry, len(rows))
	for i, row := range rows {
		result[i] = *journalEntryFromDB(row)
	}
	return result, nil
}

// UpdateJournalStatus dispatches to the appropriate sqlc query based on status.
// "posted" → PostJournalEntry; "void" → VoidJournalEntry.
func (p *Postgres) UpdateJournalStatus(ctx context.Context, tenantID, id uuid.UUID, status string, actorID uuid.UUID, _ time.Time) error {
	switch status {
	case "posted":
		_, err := p.q.PostJournalEntry(ctx, PostJournalEntryParams{
			ID:       id,
			TenantID: tenantID,
			PostedBy: pgtype.UUID{Bytes: actorID, Valid: true},
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ledger.ErrJournalNotFound
			}
			return fmt.Errorf("ledger: post journal entry: %w", err)
		}
	case "void":
		_, err := p.q.VoidJournalEntry(ctx, VoidJournalEntryParams{
			ID:       id,
			TenantID: tenantID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ledger.ErrJournalNotFound
			}
			return fmt.Errorf("ledger: void journal entry: %w", err)
		}
	default:
		return fmt.Errorf("ledger: unsupported journal status %q", status)
	}
	return nil
}

// GetTrialBalance returns all active accounts with their posted debit/credit totals
// up to and including asOf.
func (p *Postgres) GetTrialBalance(ctx context.Context, tenantID uuid.UUID, asOf time.Time) ([]ledger.TrialBalanceEntry, error) {
	const sql = `
SELECT
    a.id, a.code, a.name, a.account_type,
    COALESCE(SUM(jl.debit_amount),  0) AS total_debit,
    COALESCE(SUM(jl.credit_amount), 0) AS total_credit
FROM accounts a
LEFT JOIN journal_lines jl   ON jl.account_id  = a.id
LEFT JOIN journal_entries je ON je.id = jl.journal_id
    AND je.status = 'posted'
    AND je.created_at <= $2
WHERE a.tenant_id = $1 AND a.is_active = true
GROUP BY a.id, a.code, a.name, a.account_type
ORDER BY a.code`

	rows, err := p.db.Query(ctx, sql, tenantID, asOf)
	if err != nil {
		return nil, fmt.Errorf("ledger: get trial balance: %w", err)
	}
	defer rows.Close()

	var result []ledger.TrialBalanceEntry
	for rows.Next() {
		var (
			id          uuid.UUID
			code        string
			name        string
			accountType string
			debitNum    pgtype.Numeric
			creditNum   pgtype.Numeric
		)
		if err := rows.Scan(&id, &code, &name, &accountType, &debitNum, &creditNum); err != nil {
			return nil, fmt.Errorf("ledger: scan trial balance row: %w", err)
		}
		debit := decimalFromPgNumeric(debitNum)
		credit := decimalFromPgNumeric(creditNum)
		result = append(result, ledger.TrialBalanceEntry{
			AccountID:   id,
			Code:        code,
			Name:        name,
			AccountType: accountType,
			Debit:       debit,
			Credit:      credit,
			Balance:     debit.Sub(credit),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ledger: trial balance rows: %w", err)
	}
	return result, nil
}

// --- model conversion helpers ---

func accountFromDB(row Account) *ledger.Account {
	a := &ledger.Account{
		ID:          row.ID,
		TenantID:    row.TenantID,
		Code:        row.Code,
		Name:        row.Name,
		AccountType: row.AccountType,
		IsActive:    row.IsActive,
	}
	if row.ParentID.Valid {
		id := uuid.UUID(row.ParentID.Bytes)
		a.ParentID = &id
	}
	return a
}

func periodFromDB(row AccountingPeriod) *ledger.AccountingPeriod {
	p := &ledger.AccountingPeriod{
		ID:       row.ID,
		TenantID: row.TenantID,
		Year:     int(row.Year),
		Month:    int(row.Month),
		Status:   row.Status,
	}
	if row.ClosedBy.Valid {
		id := uuid.UUID(row.ClosedBy.Bytes)
		p.ClosedBy = &id
	}
	if row.ClosedAt.Valid {
		t := row.ClosedAt.Time
		p.ClosedAt = &t
	}
	return p
}

func journalEntryFromDB(row JournalEntry) *ledger.JournalEntry {
	je := &ledger.JournalEntry{
		ID:          row.ID,
		TenantID:    row.TenantID,
		PeriodID:    row.PeriodID,
		EntryNumber: row.EntryNumber,
		Reference:   stringFromPgText(row.Reference),
		Narration:   row.Narration,
		Status:      row.Status,
		CreatedAt:   row.CreatedAt,
	}
	if row.ProductionID.Valid {
		id := uuid.UUID(row.ProductionID.Bytes)
		je.ProductionID = &id
	}
	if row.PostedBy.Valid {
		id := uuid.UUID(row.PostedBy.Bytes)
		je.PostedBy = &id
	}
	if row.PostedAt.Valid {
		t := row.PostedAt.Time
		je.PostedAt = &t
	}
	return je
}

func journalLineFromDB(row JournalLine) ledger.JournalLine {
	return ledger.JournalLine{
		ID:           row.ID,
		JournalID:    row.JournalID,
		AccountID:    row.AccountID,
		DebitAmount:  decimalFromPgNumeric(row.DebitAmount),
		CreditAmount: decimalFromPgNumeric(row.CreditAmount),
		Currency:     row.Currency,
		Description:  stringFromPgText(row.Description),
	}
}

// --- pgtype conversion helpers ---

func pgNumericFromDecimal(d decimal.Decimal) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(d.String())
	return n
}

func decimalFromPgNumeric(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.NaN || n.Int == nil {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}

func pgUUIDFromPtr(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

func pgTextFromString(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

func stringFromPgText(t pgtype.Text) string {
	if t.Valid {
		return t.String
	}
	return ""
}

// isUniqueViolation reports whether err is a PostgreSQL unique-constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
