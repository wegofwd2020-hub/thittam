package ledger

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ledgerv1 "github.com/wegofwd2020/thittam/gen/ledger/v1"
	"github.com/wegofwd2020/thittam/pkg/interceptor"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newHandler() *Handler {
	return NewHandler(NewService(&mockRepo{}))
}

// callerCtx returns a context carrying a verified caller in tenant tid, as
// UnaryAuthInterceptor would have produced from a valid token (#138).
// Handler tests bypass the interceptor, so they must inject the caller themselves.
func callerCtx(tid uuid.UUID) context.Context {
	return interceptor.WithCaller(context.Background(), interceptor.CallerInfo{
		UserID:   uuid.New(),
		TenantID: tid,
		Email:    "user@example.com",
		Roles:    []string{"member"},
	})
}

// --- CreateAccount ---

func TestHandler_CreateAccount_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		createAccountFn: func(_ context.Context, a *Account) error { return nil },
		getAccountByCodeFn: func(_ context.Context, _ uuid.UUID, _ string) (*Account, error) {
			return nil, ErrAccountNotFound
		},
		getAccountByIDFn: func(_ context.Context, _, id uuid.UUID) (*Account, error) {
			return &Account{ID: id, TenantID: tenantID, Code: "5100", Name: "Production Costs", AccountType: "expense", IsActive: true}, nil
		},
	}))

	resp, err := h.CreateAccount(callerCtx(tenantID), &ledgerv1.CreateAccountRequest{
		TenantId:    tenantID.String(),
		Code:        "5100",
		Name:        "Production Costs",
		AccountType: "expense",
	})
	require.NoError(t, err)
	assert.Equal(t, "5100", resp.GetCode())
}

func TestHandler_CreateAccount_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreateAccount(callerCtx(uuid.New()), &ledgerv1.CreateAccountRequest{TenantId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreateAccount_InvalidParentID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().CreateAccount(callerCtx(tid), &ledgerv1.CreateAccountRequest{
		TenantId:    tid.String(),
		Code:        "5100",
		AccountType: "expense",
		ParentId:    "not-a-uuid",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- GetAccount ---

func TestHandler_GetAccount_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	accountID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getAccountByIDFn: func(_ context.Context, _, id uuid.UUID) (*Account, error) {
			return &Account{ID: id, TenantID: tenantID, Code: "5100", AccountType: "expense", IsActive: true}, nil
		},
	}))

	resp, err := h.GetAccount(callerCtx(tenantID), &ledgerv1.GetAccountRequest{
		TenantId: tenantID.String(),
		Id:       accountID.String(),
	})
	require.NoError(t, err)
	assert.Equal(t, accountID.String(), resp.GetId())
}

func TestHandler_GetAccount_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetAccount(callerCtx(uuid.New()), &ledgerv1.GetAccountRequest{TenantId: "bad", Id: uuid.New().String()})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_GetAccount_InvalidID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().GetAccount(callerCtx(tid), &ledgerv1.GetAccountRequest{TenantId: tid.String(), Id: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ListAccounts ---

func TestHandler_ListAccounts_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listAccountsFn: func(_ context.Context, _ uuid.UUID, _, _ int) ([]Account, error) {
			return []Account{{ID: uuid.New(), TenantID: tenantID, Code: "5100", AccountType: "expense", IsActive: true}}, nil
		},
	}))

	resp, err := h.ListAccounts(callerCtx(tenantID), &ledgerv1.ListAccountsRequest{TenantId: tenantID.String()})
	require.NoError(t, err)
	assert.Len(t, resp.GetAccounts(), 1)
}

func TestHandler_ListAccounts_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListAccounts(callerCtx(uuid.New()), &ledgerv1.ListAccountsRequest{TenantId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- SeedChartOfAccounts ---

func TestHandler_SeedChartOfAccounts_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getAccountByCodeFn: func(_ context.Context, _ uuid.UUID, _ string) (*Account, error) {
			return nil, ErrAccountNotFound
		},
	}))

	resp, err := h.SeedChartOfAccounts(callerCtx(tenantID), &ledgerv1.SeedChartOfAccountsRequest{
		TenantId: tenantID.String(),
		Entries: []*ledgerv1.ChartOfAccountEntry{
			{Code: "5100", Name: "Production Costs", AccountType: "expense"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.GetCreated())
}

func TestHandler_SeedChartOfAccounts_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SeedChartOfAccounts(callerCtx(uuid.New()), &ledgerv1.SeedChartOfAccountsRequest{TenantId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- OpenAccountingPeriod ---

func TestHandler_OpenAccountingPeriod_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getOpenPeriodFn: func(_ context.Context, _ uuid.UUID, _, _ int) (*AccountingPeriod, error) {
			return nil, ErrPeriodNotFound
		},
	}))

	resp, err := h.OpenAccountingPeriod(callerCtx(tenantID), &ledgerv1.OpenAccountingPeriodRequest{
		TenantId: tenantID.String(),
		Year:     2026,
		Month:    4,
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestHandler_OpenAccountingPeriod_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().OpenAccountingPeriod(callerCtx(uuid.New()), &ledgerv1.OpenAccountingPeriodRequest{TenantId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- CloseAccountingPeriod ---

func TestHandler_CloseAccountingPeriod_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	periodID := uuid.New()
	closedBy := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getPeriodByIDFn: func(_ context.Context, _, id uuid.UUID) (*AccountingPeriod, error) {
			return &AccountingPeriod{ID: id, TenantID: tenantID, Year: 2026, Month: 4, Status: "open"}, nil
		},
		closePeriodFn: func(_ context.Context, _, _, _ uuid.UUID, _ time.Time) error { return nil },
	}))

	resp, err := h.CloseAccountingPeriod(callerCtx(tenantID), &ledgerv1.CloseAccountingPeriodRequest{
		TenantId: tenantID.String(),
		PeriodId: periodID.String(),
		ClosedBy: closedBy.String(),
	})
	require.NoError(t, err)
	assert.Equal(t, periodID.String(), resp.GetId())
}

func TestHandler_CloseAccountingPeriod_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CloseAccountingPeriod(callerCtx(uuid.New()), &ledgerv1.CloseAccountingPeriodRequest{
		TenantId: "bad", PeriodId: uuid.New().String(), ClosedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CloseAccountingPeriod_InvalidPeriodID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().CloseAccountingPeriod(callerCtx(tid), &ledgerv1.CloseAccountingPeriodRequest{
		TenantId: tid.String(), PeriodId: "bad", ClosedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CloseAccountingPeriod_InvalidClosedBy(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().CloseAccountingPeriod(callerCtx(tid), &ledgerv1.CloseAccountingPeriodRequest{
		TenantId: tid.String(), PeriodId: uuid.New().String(), ClosedBy: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- CreateJournalEntry ---

func TestHandler_CreateJournalEntry_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	periodID := uuid.New()
	acct1 := uuid.New()
	acct2 := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getPeriodByIDFn: func(_ context.Context, _, id uuid.UUID) (*AccountingPeriod, error) {
			return &AccountingPeriod{ID: id, Status: "open"}, nil
		},
		getAccountByIDFn: func(_ context.Context, _, id uuid.UUID) (*Account, error) {
			return &Account{ID: id, IsActive: true, AccountType: "expense"}, nil
		},
		allocateEntryNumberFn: func(_ context.Context, _ uuid.UUID, _ int) (string, error) {
			return "JE-2026-001", nil
		},
		createJournalEntryFn: func(_ context.Context, je *JournalEntry) error { return nil },
		getJournalEntryFn: func(_ context.Context, _, id uuid.UUID) (*JournalEntry, error) {
			return &JournalEntry{ID: id, TenantID: tenantID, Status: "draft"}, nil
		},
	}))

	resp, err := h.CreateJournalEntry(callerCtx(tenantID), &ledgerv1.CreateJournalEntryRequest{
		TenantId:  tenantID.String(),
		PeriodId:  periodID.String(),
		Reference: "REF-001",
		Lines: []*ledgerv1.JournalLineInput{
			{AccountId: acct1.String(), DebitAmount: "10000.00", CreditAmount: "0.00", Currency: "INR"},
			{AccountId: acct2.String(), DebitAmount: "0.00", CreditAmount: "10000.00", Currency: "INR"},
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetId())
}

func TestHandler_CreateJournalEntry_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreateJournalEntry(callerCtx(uuid.New()), &ledgerv1.CreateJournalEntryRequest{TenantId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreateJournalEntry_InvalidPeriodID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().CreateJournalEntry(callerCtx(tid), &ledgerv1.CreateJournalEntryRequest{
		TenantId: tid.String(), PeriodId: "bad",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_CreateJournalEntry_InvalidLineAccountID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().CreateJournalEntry(callerCtx(tid), &ledgerv1.CreateJournalEntryRequest{
		TenantId: tid.String(),
		PeriodId: uuid.New().String(),
		Lines:    []*ledgerv1.JournalLineInput{{AccountId: "bad", DebitAmount: "100.00", CreditAmount: "0.00"}},
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- PostJournalEntry ---

func TestHandler_PostJournalEntry_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	jeID := uuid.New()
	postedBy := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getJournalEntryFn: func(_ context.Context, _, id uuid.UUID) (*JournalEntry, error) {
			return &JournalEntry{ID: id, TenantID: tenantID, Status: "draft",
				Lines: []JournalLine{
					{ID: uuid.New(), AccountID: uuid.New(), DebitAmount: decimal.NewFromInt(100), CreditAmount: decimal.Zero},
					{ID: uuid.New(), AccountID: uuid.New(), DebitAmount: decimal.Zero, CreditAmount: decimal.NewFromInt(100)},
				},
			}, nil
		},
		updateJournalStatusFn: func(_ context.Context, _ uuid.UUID, _ string, _ uuid.UUID, _ time.Time) error { return nil },
	}))

	resp, err := h.PostJournalEntry(callerCtx(tenantID), &ledgerv1.PostJournalEntryRequest{
		TenantId: tenantID.String(),
		Id:       jeID.String(),
		PostedBy: postedBy.String(),
	})
	require.NoError(t, err)
	assert.Equal(t, jeID.String(), resp.GetId())
}

func TestHandler_PostJournalEntry_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().PostJournalEntry(callerCtx(uuid.New()), &ledgerv1.PostJournalEntryRequest{
		TenantId: "bad", Id: uuid.New().String(), PostedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- GetJournalEntry ---

func TestHandler_GetJournalEntry_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	jeID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getJournalEntryFn: func(_ context.Context, _, id uuid.UUID) (*JournalEntry, error) {
			return &JournalEntry{ID: id, TenantID: tenantID, Status: "draft"}, nil
		},
	}))

	resp, err := h.GetJournalEntry(callerCtx(tenantID), &ledgerv1.GetJournalEntryRequest{
		TenantId: tenantID.String(),
		Id:       jeID.String(),
	})
	require.NoError(t, err)
	assert.Equal(t, jeID.String(), resp.GetId())
}

func TestHandler_GetJournalEntry_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetJournalEntry(callerCtx(uuid.New()), &ledgerv1.GetJournalEntryRequest{TenantId: "bad", Id: uuid.New().String()})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- ListJournalEntries ---

func TestHandler_ListJournalEntries_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listJournalEntriesFn: func(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _ string, _, _ int) ([]JournalEntry, error) {
			return []JournalEntry{{ID: uuid.New(), TenantID: tenantID}}, nil
		},
	}))

	resp, err := h.ListJournalEntries(callerCtx(tenantID), &ledgerv1.ListJournalEntriesRequest{TenantId: tenantID.String()})
	require.NoError(t, err)
	assert.Len(t, resp.GetEntries(), 1)
}

func TestHandler_ListJournalEntries_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ListJournalEntries(callerCtx(uuid.New()), &ledgerv1.ListJournalEntriesRequest{TenantId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestHandler_ListJournalEntries_InvalidPeriodID(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	_, err := newHandler().ListJournalEntries(callerCtx(tid), &ledgerv1.ListJournalEntriesRequest{
		TenantId: tid.String(), PeriodId: "not-a-uuid",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- VoidJournalEntry ---

func TestHandler_VoidJournalEntry_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	jeID := uuid.New()
	voidedBy := uuid.New()
	callCount := 0
	h := NewHandler(NewService(&mockRepo{
		getJournalEntryFn: func(_ context.Context, _, id uuid.UUID) (*JournalEntry, error) {
			callCount++
			status := "posted"
			if callCount > 1 {
				status = "draft"
			}
			return &JournalEntry{ID: id, TenantID: tenantID, Status: status,
				Lines: []JournalLine{
					{ID: uuid.New(), AccountID: uuid.New(), DebitAmount: decimal.NewFromInt(100), CreditAmount: decimal.Zero},
					{ID: uuid.New(), AccountID: uuid.New(), DebitAmount: decimal.Zero, CreditAmount: decimal.NewFromInt(100)},
				},
			}, nil
		},
		allocateEntryNumberFn: func(_ context.Context, _ uuid.UUID, _ int) (string, error) {
			return "JE-2026-002", nil
		},
		createJournalEntryFn:  func(_ context.Context, _ *JournalEntry) error { return nil },
		updateJournalStatusFn: func(_ context.Context, _ uuid.UUID, _ string, _ uuid.UUID, _ time.Time) error { return nil },
	}))

	resp, err := h.VoidJournalEntry(callerCtx(tenantID), &ledgerv1.VoidJournalEntryRequest{
		TenantId: tenantID.String(),
		Id:       jeID.String(),
		VoidedBy: voidedBy.String(),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.GetId())
}

func TestHandler_VoidJournalEntry_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().VoidJournalEntry(callerCtx(uuid.New()), &ledgerv1.VoidJournalEntryRequest{
		TenantId: "bad", Id: uuid.New().String(), VoidedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- GetTrialBalance ---

func TestHandler_GetTrialBalance_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getTrialBalanceFn: func(_ context.Context, _ uuid.UUID, _ time.Time) ([]TrialBalanceEntry, error) {
			return []TrialBalanceEntry{{AccountID: uuid.New(), Code: "5100", Name: "Costs", AccountType: "expense"}}, nil
		},
	}))

	resp, err := h.GetTrialBalance(callerCtx(tenantID), &ledgerv1.GetTrialBalanceRequest{TenantId: tenantID.String()})
	require.NoError(t, err)
	assert.Len(t, resp.GetEntries(), 1)
}

func TestHandler_GetTrialBalance_InvalidTenantID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().GetTrialBalance(callerCtx(uuid.New()), &ledgerv1.GetTrialBalanceRequest{TenantId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// --- Tenant boundary (#144) ---

func TestHandler_CrossTenantRead_Denied(t *testing.T) {
	t.Parallel()
	callerTenant := uuid.New()
	victimTenant := uuid.New()
	require.NotEqual(t, callerTenant, victimTenant)

	h := NewHandler(NewService(&mockRepo{
		listJournalEntriesFn: func(context.Context, uuid.UUID, *uuid.UUID, string, int, int) ([]JournalEntry, error) {
			t.Fatal("repository must not be reached on a cross-tenant request")
			return nil, nil
		},
	}))

	_, err := h.ListJournalEntries(callerCtx(callerTenant), &ledgerv1.ListJournalEntriesRequest{
		TenantId: victimTenant.String(),
	})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_ListJournalEntries_UsesTokenTenant(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	var gotTenant uuid.UUID
	h := NewHandler(NewService(&mockRepo{
		listJournalEntriesFn: func(_ context.Context, tenantID uuid.UUID, _ *uuid.UUID, _ string, _, _ int) ([]JournalEntry, error) {
			gotTenant = tenantID
			return nil, nil
		},
	}))

	// Request carries NO tenant at all: the token supplies it.
	_, err := h.ListJournalEntries(callerCtx(tid), &ledgerv1.ListJournalEntriesRequest{})
	require.NoError(t, err)
	assert.Equal(t, tid, gotTenant, "the repository must receive the token's tenant")
}

// --- grpcErr ---

func TestGrpcErr_AllCodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err      error
		wantCode codes.Code
	}{
		{ErrAccountNotFound, codes.NotFound},
		{ErrAccountInactive, codes.FailedPrecondition},
		{ErrAccountCodeExists, codes.AlreadyExists},
		{ErrInvalidAccountType, codes.InvalidArgument},
		{ErrPeriodNotFound, codes.NotFound},
		{ErrPeriodClosed, codes.FailedPrecondition},
		{ErrPeriodAlreadyExists, codes.AlreadyExists},
		{ErrJournalNotFound, codes.NotFound},
		{ErrJournalAlreadyPosted, codes.FailedPrecondition},
		{ErrJournalVoid, codes.FailedPrecondition},
		{ErrJournalNotBalanced, codes.InvalidArgument},
		{ErrJournalTooFewLines, codes.InvalidArgument},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.wantCode, status.Code(grpcErr(tc.err)))
	}
}
