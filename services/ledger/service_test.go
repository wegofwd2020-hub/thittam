package ledger

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// --- Fixed test IDs ---

var (
	fixedTenantID  = uuid.MustParse("a1000000-0000-0000-0000-000000000001")
	fixedPeriodID  = uuid.MustParse("b2000000-0000-0000-0000-000000000002")
	fixedAccountID = uuid.MustParse("c3000000-0000-0000-0000-000000000003")
	fixedAccount2  = uuid.MustParse("d4000000-0000-0000-0000-000000000004")
	fixedUserID    = uuid.MustParse("e5000000-0000-0000-0000-000000000005")
	fixedJEID      = uuid.MustParse("f6000000-0000-0000-0000-000000000006")
)

// --- Mock Repository ---

type mockRepo struct {
	createAccountFn        func(ctx context.Context, account *Account) error
	getAccountByIDFn       func(ctx context.Context, tenantID, id uuid.UUID) (*Account, error)
	getAccountByCodeFn     func(ctx context.Context, tenantID uuid.UUID, code string) (*Account, error)
	listAccountsFn         func(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]Account, error)
	createPeriodFn         func(ctx context.Context, period *AccountingPeriod) error
	getPeriodByIDFn        func(ctx context.Context, tenantID, id uuid.UUID) (*AccountingPeriod, error)
	getOpenPeriodFn        func(ctx context.Context, tenantID uuid.UUID, year, month int) (*AccountingPeriod, error)
	closePeriodFn          func(ctx context.Context, tenantID, periodID, closedBy uuid.UUID, closedAt time.Time) error
	allocateEntryNumberFn  func(ctx context.Context, tenantID uuid.UUID, year int) (string, error)
	createJournalEntryFn   func(ctx context.Context, je *JournalEntry) error
	getJournalEntryFn      func(ctx context.Context, tenantID, id uuid.UUID) (*JournalEntry, error)
	listJournalEntriesFn   func(ctx context.Context, tenantID uuid.UUID, periodID *uuid.UUID, status string, limit, offset int) ([]JournalEntry, error)
	updateJournalStatusFn  func(ctx context.Context, id uuid.UUID, status string, actorID uuid.UUID, at time.Time) error
	getTrialBalanceFn      func(ctx context.Context, tenantID uuid.UUID, asOf time.Time) ([]TrialBalanceEntry, error)
}

func (m *mockRepo) CreateAccount(ctx context.Context, account *Account) error {
	if m.createAccountFn != nil {
		return m.createAccountFn(ctx, account)
	}
	return nil
}
func (m *mockRepo) GetAccountByID(ctx context.Context, tenantID, id uuid.UUID) (*Account, error) {
	if m.getAccountByIDFn != nil {
		return m.getAccountByIDFn(ctx, tenantID, id)
	}
	return &Account{ID: id, TenantID: tenantID, Code: "5000", AccountType: "expense", IsActive: true}, nil
}
func (m *mockRepo) GetAccountByCode(ctx context.Context, tenantID uuid.UUID, code string) (*Account, error) {
	if m.getAccountByCodeFn != nil {
		return m.getAccountByCodeFn(ctx, tenantID, code)
	}
	return &Account{ID: uuid.New(), TenantID: tenantID, Code: code, IsActive: true}, nil
}
func (m *mockRepo) ListAccounts(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]Account, error) {
	if m.listAccountsFn != nil {
		return m.listAccountsFn(ctx, tenantID, limit, offset)
	}
	return nil, nil
}
func (m *mockRepo) CreatePeriod(ctx context.Context, period *AccountingPeriod) error {
	if m.createPeriodFn != nil {
		return m.createPeriodFn(ctx, period)
	}
	return nil
}
func (m *mockRepo) GetPeriodByID(ctx context.Context, tenantID, id uuid.UUID) (*AccountingPeriod, error) {
	if m.getPeriodByIDFn != nil {
		return m.getPeriodByIDFn(ctx, tenantID, id)
	}
	return &AccountingPeriod{ID: id, TenantID: tenantID, Status: "open"}, nil
}
func (m *mockRepo) GetOpenPeriod(ctx context.Context, tenantID uuid.UUID, year, month int) (*AccountingPeriod, error) {
	if m.getOpenPeriodFn != nil {
		return m.getOpenPeriodFn(ctx, tenantID, year, month)
	}
	return &AccountingPeriod{ID: fixedPeriodID, TenantID: tenantID, Year: year, Month: month, Status: "open"}, nil
}
func (m *mockRepo) ClosePeriod(ctx context.Context, tenantID, periodID, closedBy uuid.UUID, closedAt time.Time) error {
	if m.closePeriodFn != nil {
		return m.closePeriodFn(ctx, tenantID, periodID, closedBy, closedAt)
	}
	return nil
}
func (m *mockRepo) AllocateEntryNumber(ctx context.Context, tenantID uuid.UUID, year int) (string, error) {
	if m.allocateEntryNumberFn != nil {
		return m.allocateEntryNumberFn(ctx, tenantID, year)
	}
	return fmt.Sprintf("JE-%d-00001", year), nil
}
func (m *mockRepo) CreateJournalEntry(ctx context.Context, je *JournalEntry) error {
	if m.createJournalEntryFn != nil {
		return m.createJournalEntryFn(ctx, je)
	}
	return nil
}
func (m *mockRepo) GetJournalEntry(ctx context.Context, tenantID, id uuid.UUID) (*JournalEntry, error) {
	if m.getJournalEntryFn != nil {
		return m.getJournalEntryFn(ctx, tenantID, id)
	}
	return &JournalEntry{
		ID:       id,
		TenantID: tenantID,
		PeriodID: fixedPeriodID,
		Status:   "draft",
		Lines: []JournalLine{
			{ID: uuid.New(), AccountID: fixedAccountID, DebitAmount: decimal.NewFromInt(100)},
			{ID: uuid.New(), AccountID: fixedAccount2, CreditAmount: decimal.NewFromInt(100)},
		},
	}, nil
}
func (m *mockRepo) ListJournalEntries(ctx context.Context, tenantID uuid.UUID, periodID *uuid.UUID, status string, limit, offset int) ([]JournalEntry, error) {
	if m.listJournalEntriesFn != nil {
		return m.listJournalEntriesFn(ctx, tenantID, periodID, status, limit, offset)
	}
	return nil, nil
}
func (m *mockRepo) UpdateJournalStatus(ctx context.Context, id uuid.UUID, status string, actorID uuid.UUID, at time.Time) error {
	if m.updateJournalStatusFn != nil {
		return m.updateJournalStatusFn(ctx, id, status, actorID, at)
	}
	return nil
}
func (m *mockRepo) GetTrialBalance(ctx context.Context, tenantID uuid.UUID, asOf time.Time) ([]TrialBalanceEntry, error) {
	if m.getTrialBalanceFn != nil {
		return m.getTrialBalanceFn(ctx, tenantID, asOf)
	}
	return nil, nil
}

// --- Tests: Accounts ---

func TestCreateAccount_GeneratesID(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	acct, err := svc.CreateAccount(context.Background(), &Account{
		TenantID:    fixedTenantID,
		Code:        "5000",
		Name:        "Above the Line",
		AccountType: "expense",
	})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, acct.ID)
	assert.True(t, acct.IsActive)
}

func TestCreateAccount_InvalidType(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	_, err := svc.CreateAccount(context.Background(), &Account{
		TenantID:    fixedTenantID,
		Code:        "9999",
		AccountType: "nonsense",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidAccountType)
}

// --- Tests: Accounting Periods ---

func TestOpenAccountingPeriod_Success(t *testing.T) {
	t.Parallel()
	var saved *AccountingPeriod
	svc := NewService(&mockRepo{
		createPeriodFn: func(_ context.Context, p *AccountingPeriod) error {
			saved = p
			return nil
		},
	})

	err := svc.OpenAccountingPeriod(context.Background(), fixedTenantID, 2024, time.April)
	require.NoError(t, err)
	assert.Equal(t, "open", saved.Status)
	assert.Equal(t, 2024, saved.Year)
	assert.Equal(t, 4, saved.Month)
}

func TestOpenAccountingPeriod_Idempotent(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		createPeriodFn: func(_ context.Context, _ *AccountingPeriod) error {
			return ErrPeriodAlreadyExists
		},
	})

	// Must not return an error when period already exists.
	err := svc.OpenAccountingPeriod(context.Background(), fixedTenantID, 2024, time.April)
	require.NoError(t, err)
}

func TestCloseAccountingPeriod_AlreadyClosed(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getPeriodByIDFn: func(_ context.Context, _, id uuid.UUID) (*AccountingPeriod, error) {
			return &AccountingPeriod{ID: id, Status: "closed"}, nil
		},
	})

	// Closing an already-closed period is a no-op.
	p, err := svc.CloseAccountingPeriod(context.Background(), fixedTenantID, fixedPeriodID, fixedUserID)
	require.NoError(t, err)
	assert.Equal(t, "closed", p.Status)
}

func TestCloseAccountingPeriod_SetsClosedBy(t *testing.T) {
	t.Parallel()
	var capturedClosedBy uuid.UUID
	svc := NewService(&mockRepo{
		closePeriodFn: func(_ context.Context, _, _, closedBy uuid.UUID, _ time.Time) error {
			capturedClosedBy = closedBy
			return nil
		},
	})

	_, err := svc.CloseAccountingPeriod(context.Background(), fixedTenantID, fixedPeriodID, fixedUserID)
	require.NoError(t, err)
	assert.Equal(t, fixedUserID, capturedClosedBy)
}

// --- Tests: Journal Entries ---

func TestCreateJournalEntry_TooFewLines(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	_, err := svc.CreateJournalEntry(context.Background(), &JournalEntry{
		TenantID:  fixedTenantID,
		PeriodID:  fixedPeriodID,
		Narration: "Test",
		Lines:     []JournalLine{{AccountID: fixedAccountID, DebitAmount: decimal.NewFromInt(100)}},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrJournalTooFewLines)
}

func TestCreateJournalEntry_InactiveAccount(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getAccountByIDFn: func(_ context.Context, _, _ uuid.UUID) (*Account, error) {
			return &Account{IsActive: false, Code: "5000"}, nil
		},
	})

	_, err := svc.CreateJournalEntry(context.Background(), &JournalEntry{
		TenantID:  fixedTenantID,
		PeriodID:  fixedPeriodID,
		Narration: "Test",
		Lines: []JournalLine{
			{AccountID: fixedAccountID, DebitAmount: decimal.NewFromInt(100)},
			{AccountID: fixedAccount2, CreditAmount: decimal.NewFromInt(100)},
		},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAccountInactive)
}

func TestCreateJournalEntry_Success(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	je := &JournalEntry{
		TenantID:  fixedTenantID,
		PeriodID:  fixedPeriodID,
		Narration: "Location rental",
		Lines: []JournalLine{
			{AccountID: fixedAccountID, DebitAmount: decimal.NewFromInt(50000)},
			{AccountID: fixedAccount2, CreditAmount: decimal.NewFromInt(50000)},
		},
	}
	result, err := svc.CreateJournalEntry(context.Background(), je)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, result.ID)
	assert.Equal(t, "draft", result.Status)
	assert.NotEmpty(t, result.EntryNumber)
}

func TestPostJournalEntry_NotBalanced(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getJournalEntryFn: func(_ context.Context, _, id uuid.UUID) (*JournalEntry, error) {
			return &JournalEntry{
				ID:       id,
				PeriodID: fixedPeriodID,
				Status:   "draft",
				Lines: []JournalLine{
					{DebitAmount: decimal.NewFromInt(100)},
					{CreditAmount: decimal.NewFromInt(90)}, // unbalanced
				},
			}, nil
		},
	})

	_, err := svc.PostJournalEntry(context.Background(), fixedTenantID, fixedJEID, fixedUserID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrJournalNotBalanced)
}

func TestPostJournalEntry_ClosedPeriod(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getJournalEntryFn: func(_ context.Context, _, id uuid.UUID) (*JournalEntry, error) {
			return &JournalEntry{
				ID:       id,
				PeriodID: fixedPeriodID,
				Status:   "draft",
				Lines: []JournalLine{
					{DebitAmount: decimal.NewFromInt(100)},
					{CreditAmount: decimal.NewFromInt(100)},
				},
			}, nil
		},
		getPeriodByIDFn: func(_ context.Context, _, _ uuid.UUID) (*AccountingPeriod, error) {
			return &AccountingPeriod{Status: "closed"}, nil
		},
	})

	_, err := svc.PostJournalEntry(context.Background(), fixedTenantID, fixedJEID, fixedUserID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPeriodClosed)
}

func TestPostJournalEntry_AlreadyPosted(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getJournalEntryFn: func(_ context.Context, _, id uuid.UUID) (*JournalEntry, error) {
			return &JournalEntry{ID: id, Status: "posted"}, nil
		},
	})

	_, err := svc.PostJournalEntry(context.Background(), fixedTenantID, fixedJEID, fixedUserID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrJournalAlreadyPosted)
}

func TestPostJournalEntry_Success(t *testing.T) {
	t.Parallel()
	var postedStatus string
	svc := NewService(&mockRepo{
		updateJournalStatusFn: func(_ context.Context, _ uuid.UUID, status string, _ uuid.UUID, _ time.Time) error {
			postedStatus = status
			return nil
		},
	})

	je, err := svc.PostJournalEntry(context.Background(), fixedTenantID, fixedJEID, fixedUserID)
	require.NoError(t, err)
	assert.Equal(t, "posted", je.Status)
	assert.Equal(t, "posted", postedStatus)
	assert.NotNil(t, je.PostedAt)
	assert.Equal(t, fixedUserID, *je.PostedBy)
}

func TestVoidJournalEntry_NotPosted(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getJournalEntryFn: func(_ context.Context, _, id uuid.UUID) (*JournalEntry, error) {
			return &JournalEntry{ID: id, Status: "draft"}, nil
		},
	})

	_, err := svc.VoidJournalEntry(context.Background(), fixedTenantID, fixedJEID, fixedUserID)
	require.Error(t, err)
}

func TestVoidJournalEntry_AlreadyVoid(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getJournalEntryFn: func(_ context.Context, _, id uuid.UUID) (*JournalEntry, error) {
			return &JournalEntry{ID: id, Status: "void"}, nil
		},
	})

	_, err := svc.VoidJournalEntry(context.Background(), fixedTenantID, fixedJEID, fixedUserID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrJournalVoid)
}

func TestVoidJournalEntry_ReversesLines(t *testing.T) {
	t.Parallel()
	var createdLines []JournalLine
	svc := NewService(&mockRepo{
		getJournalEntryFn: func(_ context.Context, _, id uuid.UUID) (*JournalEntry, error) {
			if id == fixedJEID {
				// Original posted entry.
				return &JournalEntry{
					ID:          fixedJEID,
					TenantID:    fixedTenantID,
					PeriodID:    fixedPeriodID,
					EntryNumber: "JE-2024-00001",
					Narration:   "Original",
					Status:      "posted",
					Lines: []JournalLine{
						{ID: uuid.New(), AccountID: fixedAccountID, DebitAmount: decimal.NewFromInt(500), Currency: "INR"},
						{ID: uuid.New(), AccountID: fixedAccount2, CreditAmount: decimal.NewFromInt(500), Currency: "INR"},
					},
				}, nil
			}
			// Reversing entry (new UUID) — must be draft with balanced lines for PostJournalEntry to succeed.
			return &JournalEntry{
				ID:       id,
				TenantID: fixedTenantID,
				PeriodID: fixedPeriodID,
				Status:   "draft",
				Lines: []JournalLine{
					{ID: uuid.New(), AccountID: fixedAccount2, DebitAmount: decimal.NewFromInt(500), Currency: "INR"},
					{ID: uuid.New(), AccountID: fixedAccountID, CreditAmount: decimal.NewFromInt(500), Currency: "INR"},
				},
			}, nil
		},
		createJournalEntryFn: func(_ context.Context, je *JournalEntry) error {
			createdLines = je.Lines
			return nil
		},
		// UpdateJournalStatus is called twice: once for the reversing post, once for the original void.
	})

	_, err := svc.VoidJournalEntry(context.Background(), fixedTenantID, fixedJEID, fixedUserID)
	require.NoError(t, err)

	require.Len(t, createdLines, 2)
	// Lines should be swapped: original DR 500 becomes CR 500, original CR 500 becomes DR 500.
	assert.True(t, createdLines[0].CreditAmount.Equal(decimal.NewFromInt(500)))
	assert.True(t, createdLines[0].DebitAmount.IsZero())
	assert.True(t, createdLines[1].DebitAmount.Equal(decimal.NewFromInt(500)))
	assert.True(t, createdLines[1].CreditAmount.IsZero())
}

func TestListJournalEntries_DefaultLimit(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	svc := NewService(&mockRepo{
		listJournalEntriesFn: func(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _ string, limit, _ int) ([]JournalEntry, error) {
			capturedLimit = limit
			return nil, nil
		},
	})

	_, err := svc.ListJournalEntries(context.Background(), fixedTenantID, nil, "", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 20, capturedLimit)
}

// --- Tests: SeedChartOfAccounts ---

func TestSeedChartOfAccounts_Idempotent(t *testing.T) {
	t.Parallel()
	callCount := 0
	svc := NewService(&mockRepo{
		createAccountFn: func(_ context.Context, _ *Account) error {
			callCount++
			// Simulate that the account already exists on the second call.
			if callCount > 1 {
				return ErrAccountCodeExists
			}
			return nil
		},
	})

	entries := []vertical.ChartOfAccountEntry{
		{Code: "1000", Name: "Bank & Cash", AccountType: "asset"},
		{Code: "5000", Name: "Expenses", AccountType: "expense"},
	}
	err := svc.SeedChartOfAccounts(context.Background(), fixedTenantID, entries)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
}

func TestSeedChartOfAccounts_ResolvesParentCode(t *testing.T) {
	t.Parallel()
	parentCode := "5000"
	var capturedParentID *uuid.UUID
	parentID := uuid.New()

	svc := NewService(&mockRepo{
		getAccountByCodeFn: func(_ context.Context, _ uuid.UUID, code string) (*Account, error) {
			if code == parentCode {
				return &Account{ID: parentID, Code: parentCode}, nil
			}
			return nil, ErrAccountNotFound
		},
		createAccountFn: func(_ context.Context, a *Account) error {
			if a.Code == "5001" {
				capturedParentID = a.ParentID
			}
			return nil
		},
	})

	entries := []vertical.ChartOfAccountEntry{
		{Code: "5001", Name: "Location Rental", AccountType: "expense", ParentCode: &parentCode},
	}
	err := svc.SeedChartOfAccounts(context.Background(), fixedTenantID, entries)
	require.NoError(t, err)
	require.NotNil(t, capturedParentID)
	assert.Equal(t, parentID, *capturedParentID)
}

// --- Tests: validateBalance helper ---

func TestValidateBalance_Balanced(t *testing.T) {
	t.Parallel()
	lines := []JournalLine{
		{DebitAmount: decimal.NewFromInt(1000)},
		{CreditAmount: decimal.NewFromInt(600)},
		{CreditAmount: decimal.NewFromInt(400)},
	}
	err := validateBalance(lines)
	require.NoError(t, err)
}

func TestValidateBalance_Unbalanced(t *testing.T) {
	t.Parallel()
	lines := []JournalLine{
		{DebitAmount: decimal.NewFromInt(1000)},
		{CreditAmount: decimal.NewFromInt(999)},
	}
	err := validateBalance(lines)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrJournalNotBalanced)
}

// --- Additional coverage tests ---

func TestGetAccount_Success(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	acct, err := svc.GetAccount(context.Background(), fixedTenantID, fixedAccountID)
	require.NoError(t, err)
	assert.Equal(t, fixedAccountID, acct.ID)
}

func TestGetAccount_Error(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getAccountByIDFn: func(_ context.Context, _, _ uuid.UUID) (*Account, error) {
			return nil, ErrAccountNotFound
		},
	})

	_, err := svc.GetAccount(context.Background(), fixedTenantID, fixedAccountID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAccountNotFound)
}

func TestListAccounts_Success(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		listAccountsFn: func(_ context.Context, _ uuid.UUID, _, _ int) ([]Account, error) {
			return []Account{{ID: fixedAccountID, Code: "5000"}}, nil
		},
	})

	accounts, err := svc.ListAccounts(context.Background(), fixedTenantID, 200, 0)
	require.NoError(t, err)
	assert.Len(t, accounts, 1)
	assert.Equal(t, "5000", accounts[0].Code)
}

func TestGetJournalEntry_Success(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getJournalEntryFn: func(_ context.Context, _, id uuid.UUID) (*JournalEntry, error) {
			return &JournalEntry{ID: id, Status: "posted"}, nil
		},
	})

	je, err := svc.GetJournalEntry(context.Background(), fixedTenantID, fixedJEID)
	require.NoError(t, err)
	assert.Equal(t, fixedJEID, je.ID)
	assert.Equal(t, "posted", je.Status)
}

func TestGetJournalEntry_Error(t *testing.T) {
	t.Parallel()
	sentinel := fmt.Errorf("not found")
	svc := NewService(&mockRepo{
		getJournalEntryFn: func(_ context.Context, _, _ uuid.UUID) (*JournalEntry, error) {
			return nil, sentinel
		},
	})

	_, err := svc.GetJournalEntry(context.Background(), fixedTenantID, fixedJEID)
	require.Error(t, err)
}

func TestGetTrialBalance_Success(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getTrialBalanceFn: func(_ context.Context, _ uuid.UUID, _ time.Time) ([]TrialBalanceEntry, error) {
			return []TrialBalanceEntry{
				{Code: "5000", Debit: decimal.NewFromInt(10000)},
			}, nil
		},
	})

	entries, err := svc.GetTrialBalance(context.Background(), fixedTenantID, time.Now())
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "5000", entries[0].Code)
}

func TestCreateAccount_RepoError(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		createAccountFn: func(_ context.Context, _ *Account) error {
			return fmt.Errorf("db error")
		},
	})

	_, err := svc.CreateAccount(context.Background(), &Account{
		TenantID:    fixedTenantID,
		Code:        "5001",
		AccountType: "expense",
	})
	require.Error(t, err)
}

func TestOpenAccountingPeriod_RepoError(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		createPeriodFn: func(_ context.Context, _ *AccountingPeriod) error {
			return fmt.Errorf("db error")
		},
	})

	err := svc.OpenAccountingPeriod(context.Background(), fixedTenantID, 2024, time.April)
	require.Error(t, err)
}

func TestCloseAccountingPeriod_RepoError(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		closePeriodFn: func(_ context.Context, _, _, _ uuid.UUID, _ time.Time) error {
			return fmt.Errorf("db error")
		},
	})

	_, err := svc.CloseAccountingPeriod(context.Background(), fixedTenantID, fixedPeriodID, fixedUserID)
	require.Error(t, err)
}

func TestPostJournalEntry_VoidStatus(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getJournalEntryFn: func(_ context.Context, _, id uuid.UUID) (*JournalEntry, error) {
			return &JournalEntry{ID: id, Status: "void"}, nil
		},
	})

	_, err := svc.PostJournalEntry(context.Background(), fixedTenantID, fixedJEID, fixedUserID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrJournalVoid)
}

func TestListJournalEntries_MaxLimitEnforced(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	svc := NewService(&mockRepo{
		listJournalEntriesFn: func(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _ string, limit, _ int) ([]JournalEntry, error) {
			capturedLimit = limit
			return nil, nil
		},
	})

	_, err := svc.ListJournalEntries(context.Background(), fixedTenantID, nil, "", 9999, 0)
	require.NoError(t, err)
	assert.Equal(t, 20, capturedLimit)
}

func TestSeedChartOfAccounts_CreateError(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		createAccountFn: func(_ context.Context, _ *Account) error {
			return fmt.Errorf("unexpected db error")
		},
	})

	entries := []vertical.ChartOfAccountEntry{
		{Code: "5000", Name: "Expenses", AccountType: "expense"},
	}
	err := svc.SeedChartOfAccounts(context.Background(), fixedTenantID, entries)
	require.Error(t, err)
}

func TestListAccounts_OutOfRangeLimit_DefaultsTo200(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	svc := NewService(&mockRepo{
		listAccountsFn: func(_ context.Context, _ uuid.UUID, limit, _ int) ([]Account, error) {
			capturedLimit = limit
			return nil, nil
		},
	})

	// limit = 0 → should default to 200
	_, err := svc.ListAccounts(context.Background(), fixedTenantID, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 200, capturedLimit)

	// limit > 500 → should default to 200
	_, err = svc.ListAccounts(context.Background(), fixedTenantID, 9999, 0)
	require.NoError(t, err)
	assert.Equal(t, 200, capturedLimit)
}

func TestPublish_NilPublisher_IsNoOp(t *testing.T) {
	t.Parallel()
	// NewService with no publisher — publish() must not panic.
	svc := NewService(&mockRepo{})
	called := false
	svc.publish(context.Background(), func() error {
		called = true
		return nil
	})
	assert.False(t, called, "nil publisher: fn must not be called")
}
