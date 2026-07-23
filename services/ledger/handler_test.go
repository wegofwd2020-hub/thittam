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

// allowAllPerm grants every permission. Authorization semantics are covered by
// pkg/interceptor's own tests; these handler tests exercise handler logic.
type allowAllPerm struct{}

func (allowAllPerm) CheckPermission(context.Context, uuid.UUID, string, *uuid.UUID) (bool, error) {
	return true, nil
}

// denyPerm denies everything, so a test can prove a gate fires before any repository
// call. mockRepo's unset fn-fields return benign defaults and never panic, so a
// denial test that only asserts the status code would also pass against a handler
// that wrote first and denied second. Denial tests must install a t.Fatal repo fn.
type denyPerm struct{}

func (denyPerm) CheckPermission(context.Context, uuid.UUID, string, *uuid.UUID) (bool, error) {
	return false, nil
}

func newHandler() *Handler {
	return NewHandler(NewService(&mockRepo{}), allowAllPerm{})
}

// callerCtxAs returns a context carrying a verified caller with a specific user id,
// as UnaryAuthInterceptor would have produced from a valid token (#138). Tests that
// assert on the recorded actor must name their caller.
func callerCtxAs(tid, uid uuid.UUID) context.Context {
	return interceptor.WithCaller(context.Background(), interceptor.CallerInfo{
		UserID:   uid,
		TenantID: tid,
		Email:    "user@example.com",
		Roles:    []string{"member"},
	})
}

// callerCtx returns a context carrying a verified caller in tenant tid, with an
// arbitrary user id.
func callerCtx(tid uuid.UUID) context.Context {
	return callerCtxAs(tid, uuid.New())
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
	}), allowAllPerm{})

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
	}), allowAllPerm{})

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
	}), allowAllPerm{})

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
	}), allowAllPerm{})

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
	}), allowAllPerm{})

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
	}), allowAllPerm{})

	resp, err := h.CloseAccountingPeriod(callerCtxAs(tenantID, closedBy), &ledgerv1.CloseAccountingPeriodRequest{
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
	}), allowAllPerm{})

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
		updateJournalStatusFn: func(_ context.Context, _, _ uuid.UUID, _ string, _ uuid.UUID, _ time.Time) error { return nil },
	}), allowAllPerm{})

	resp, err := h.PostJournalEntry(callerCtxAs(tenantID, postedBy), &ledgerv1.PostJournalEntryRequest{
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
	}), allowAllPerm{})

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
	}), allowAllPerm{})

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
		updateJournalStatusFn: func(_ context.Context, _, _ uuid.UUID, _ string, _ uuid.UUID, _ time.Time) error { return nil },
	}), allowAllPerm{})

	resp, err := h.VoidJournalEntry(callerCtxAs(tenantID, voidedBy), &ledgerv1.VoidJournalEntryRequest{
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
	}), allowAllPerm{})

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
	}), allowAllPerm{})

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
	}), allowAllPerm{})

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

// --- Task 3: the permission gates ---

func TestHandler_PostJournalEntry_MemberDenied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getJournalEntryFn: func(context.Context, uuid.UUID, uuid.UUID) (*JournalEntry, error) {
			t.Fatal("gate must fire before the repository is read")
			return nil, nil
		},
		updateJournalStatusFn: func(context.Context, uuid.UUID, uuid.UUID, string, uuid.UUID, time.Time) error {
			t.Fatal("gate must fire before the journal is written")
			return nil
		},
	}), denyPerm{})

	_, err := h.PostJournalEntry(callerCtx(tenantID), &ledgerv1.PostJournalEntryRequest{
		TenantId: tenantID.String(), Id: uuid.New().String(),
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_VoidJournalEntry_MemberDenied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getJournalEntryFn: func(context.Context, uuid.UUID, uuid.UUID) (*JournalEntry, error) {
			t.Fatal("gate must fire before the repository is read")
			return nil, nil
		},
		updateJournalStatusFn: func(context.Context, uuid.UUID, uuid.UUID, string, uuid.UUID, time.Time) error {
			t.Fatal("gate must fire before the journal is written")
			return nil
		},
	}), denyPerm{})

	_, err := h.VoidJournalEntry(callerCtx(tenantID), &ledgerv1.VoidJournalEntryRequest{
		TenantId: tenantID.String(), Id: uuid.New().String(),
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_CloseAccountingPeriod_MemberDenied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getPeriodByIDFn: func(context.Context, uuid.UUID, uuid.UUID) (*AccountingPeriod, error) {
			t.Fatal("gate must fire before the repository is read")
			return nil, nil
		},
		closePeriodFn: func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, time.Time) error {
			t.Fatal("gate must fire before the period is closed")
			return nil
		},
	}), denyPerm{})

	_, err := h.CloseAccountingPeriod(callerCtx(tenantID), &ledgerv1.CloseAccountingPeriodRequest{
		TenantId: tenantID.String(), PeriodId: uuid.New().String(),
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_CreateJournalEntry_MemberDenied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		createJournalEntryFn: func(context.Context, *JournalEntry) error {
			t.Fatal("gate must fire before the entry is created")
			return nil
		},
	}), denyPerm{})

	_, err := h.CreateJournalEntry(callerCtx(tenantID), &ledgerv1.CreateJournalEntryRequest{
		TenantId: tenantID.String(), PeriodId: uuid.New().String(),
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_OpenAccountingPeriod_MemberDenied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		createPeriodFn: func(context.Context, *AccountingPeriod) error {
			t.Fatal("gate must fire before the period is created")
			return nil
		},
	}), denyPerm{})

	_, err := h.OpenAccountingPeriod(callerCtx(tenantID), &ledgerv1.OpenAccountingPeriodRequest{
		TenantId: tenantID.String(), Year: 2026, Month: 4,
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// The request MUST carry an entry. With an empty Entries slice the service loops
// zero times, so createAccountFn's t.Fatal would never fire even against an ungated
// handler — the test would be vacuous.
func TestHandler_SeedChartOfAccounts_MemberDenied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getAccountByCodeFn: func(context.Context, uuid.UUID, string) (*Account, error) {
			t.Fatal("gate must fire before the chart is read")
			return nil, nil
		},
		createAccountFn: func(context.Context, *Account) error {
			t.Fatal("gate must fire before any account is created")
			return nil
		},
	}), denyPerm{})

	_, err := h.SeedChartOfAccounts(callerCtx(tenantID), &ledgerv1.SeedChartOfAccountsRequest{
		TenantId: tenantID.String(),
		Entries: []*ledgerv1.ChartOfAccountEntry{
			{Code: "5100", Name: "Production Costs", AccountType: "expense"},
		},
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_CreateAccount_MemberDenied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		createAccountFn: func(context.Context, *Account) error {
			t.Fatal("gate must fire before the account is created")
			return nil
		},
	}), denyPerm{})

	_, err := h.CreateAccount(callerCtx(tenantID), &ledgerv1.CreateAccountRequest{
		TenantId: tenantID.String(), Code: "5100", Name: "Production Costs", AccountType: "expense",
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_GetTrialBalance_MemberDenied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getTrialBalanceFn: func(context.Context, uuid.UUID, time.Time) ([]TrialBalanceEntry, error) {
			t.Fatal("gate must fire before the trial balance is read")
			return nil, nil
		},
	}), denyPerm{})

	_, err := h.GetTrialBalance(callerCtx(tenantID), &ledgerv1.GetTrialBalanceRequest{
		TenantId: tenantID.String(),
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_ListAccounts_MemberDenied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		listAccountsFn: func(context.Context, uuid.UUID, int, int) ([]Account, error) {
			t.Fatal("gate must fire before the repository is read")
			return nil, nil
		},
	}), denyPerm{})

	_, err := h.ListAccounts(callerCtx(tenantID), &ledgerv1.ListAccountsRequest{TenantId: tenantID.String()})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// A nil checker is a misconfiguration, not an authorization. It must not pass.
func TestHandler_NilPermissionChecker_Denies(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		updateJournalStatusFn: func(context.Context, uuid.UUID, uuid.UUID, string, uuid.UUID, time.Time) error {
			t.Fatal("a nil checker must never reach the journal")
			return nil
		},
	}), nil)

	_, err := h.PostJournalEntry(callerCtx(tenantID), &ledgerv1.PostJournalEntryRequest{
		TenantId: tenantID.String(), Id: uuid.New().String(),
	})
	assert.Equal(t, codes.Internal, status.Code(err))
}

// --- Task 3: actor integrity ---

func TestHandler_PostJournalEntry_ForgedActorDenied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	callerID := uuid.New()
	victimID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getJournalEntryFn: func(context.Context, uuid.UUID, uuid.UUID) (*JournalEntry, error) {
			t.Fatal("a forged actor must be refused before the repository is read")
			return nil, nil
		},
		updateJournalStatusFn: func(context.Context, uuid.UUID, uuid.UUID, string, uuid.UUID, time.Time) error {
			t.Fatal("a forged actor must never be written")
			return nil
		},
	}), allowAllPerm{})

	_, err := h.PostJournalEntry(callerCtxAs(tenantID, callerID), &ledgerv1.PostJournalEntryRequest{
		TenantId: tenantID.String(), Id: uuid.New().String(), PostedBy: victimID.String(),
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_VoidJournalEntry_ForgedActorDenied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	callerID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getJournalEntryFn: func(context.Context, uuid.UUID, uuid.UUID) (*JournalEntry, error) {
			t.Fatal("a forged actor must be refused before the repository is read")
			return nil, nil
		},
	}), allowAllPerm{})

	_, err := h.VoidJournalEntry(callerCtxAs(tenantID, callerID), &ledgerv1.VoidJournalEntryRequest{
		TenantId: tenantID.String(), Id: uuid.New().String(), VoidedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_CloseAccountingPeriod_ForgedActorDenied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	callerID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getPeriodByIDFn: func(context.Context, uuid.UUID, uuid.UUID) (*AccountingPeriod, error) {
			t.Fatal("a forged actor must be refused before the repository is read")
			return nil, nil
		},
	}), allowAllPerm{})

	_, err := h.CloseAccountingPeriod(callerCtxAs(tenantID, callerID), &ledgerv1.CloseAccountingPeriodRequest{
		TenantId: tenantID.String(), PeriodId: uuid.New().String(), ClosedBy: uuid.New().String(),
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// Service.PostJournalEntry(ctx, tenantID, id, postedBy) takes three consecutive
// uuid.UUID, so a transposition compiles silently and mockRepo's fn-fields ignore
// arguments they are not asked about. Capture what actually reaches the repository.
func TestHandler_PostJournalEntry_RecordsTheCallerAsActor(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	callerID := uuid.New()
	jeID := uuid.New()
	var gotEntryID, gotActorID uuid.UUID
	h := NewHandler(NewService(&mockRepo{
		getJournalEntryFn: func(_ context.Context, _, id uuid.UUID) (*JournalEntry, error) {
			return &JournalEntry{ID: id, TenantID: tenantID, Status: "draft",
				Lines: []JournalLine{
					{ID: uuid.New(), AccountID: uuid.New(), DebitAmount: decimal.NewFromInt(100), CreditAmount: decimal.Zero},
					{ID: uuid.New(), AccountID: uuid.New(), DebitAmount: decimal.Zero, CreditAmount: decimal.NewFromInt(100)},
				},
			}, nil
		},
		updateJournalStatusFn: func(_ context.Context, _, id uuid.UUID, _ string, actorID uuid.UUID, _ time.Time) error {
			gotEntryID, gotActorID = id, actorID
			return nil
		},
	}), allowAllPerm{})

	// PostedBy left empty: a well-behaved client stops sending the deprecated field.
	_, err := h.PostJournalEntry(callerCtxAs(tenantID, callerID), &ledgerv1.PostJournalEntryRequest{
		TenantId: tenantID.String(), Id: jeID.String(),
	})
	require.NoError(t, err)
	assert.Equal(t, callerID, gotActorID, "the posting is attributed to the caller")
	assert.Equal(t, jeID, gotEntryID)
	assert.NotEqual(t, gotEntryID, gotActorID, "entry id and actor id must not be transposed")
}
