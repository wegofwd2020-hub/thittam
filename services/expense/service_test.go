package expense

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// --- Mock repository ---

type mockRepo struct {
	createExpenseFn   func(ctx context.Context, e *Expense) error
	getExpenseFn      func(ctx context.Context, tenantID, id uuid.UUID) (*Expense, error)
	listExpensesFn    func(ctx context.Context, tenantID, prodID uuid.UUID, status string, limit, offset int) ([]Expense, error)
	updateExpenseFn   func(ctx context.Context, e *Expense) error
	rejectExpenseFn   func(ctx context.Context, tenantID, expenseID uuid.UUID, reason string) error
	createPOFn        func(ctx context.Context, po *PurchaseOrder) error
	getPOFn           func(ctx context.Context, tenantID, id uuid.UUID) (*PurchaseOrder, error)
	listPOsFn         func(ctx context.Context, tenantID, prodID uuid.UUID, status string, limit, offset int) ([]PurchaseOrder, error)
	updatePOFn        func(ctx context.Context, po *PurchaseOrder) error
	createPettyCashFn func(ctx context.Context, pc *PettyCashAdvance) error
	getPettyCashFn    func(ctx context.Context, tenantID, id uuid.UUID) (*PettyCashAdvance, error)
	listPettyCashFn   func(ctx context.Context, tenantID, prodID uuid.UUID, status string, limit, offset int) ([]PettyCashAdvance, error)
	updatePettyCashFn func(ctx context.Context, pc *PettyCashAdvance) error
}

func (m *mockRepo) CreateExpense(ctx context.Context, e *Expense) error {
	if m.createExpenseFn != nil {
		return m.createExpenseFn(ctx, e)
	}
	return nil
}
func (m *mockRepo) GetExpense(ctx context.Context, tenantID, id uuid.UUID) (*Expense, error) {
	if m.getExpenseFn != nil {
		return m.getExpenseFn(ctx, tenantID, id)
	}
	return &Expense{ID: id, TenantID: tenantID, Amount: decimal.NewFromInt(5000), Status: "submitted"}, nil
}
func (m *mockRepo) ListExpenses(ctx context.Context, tenantID, prodID uuid.UUID, status string, limit, offset int) ([]Expense, error) {
	if m.listExpensesFn != nil {
		return m.listExpensesFn(ctx, tenantID, prodID, status, limit, offset)
	}
	return nil, nil
}
func (m *mockRepo) UpdateExpense(ctx context.Context, e *Expense) error {
	if m.updateExpenseFn != nil {
		return m.updateExpenseFn(ctx, e)
	}
	return nil
}
func (m *mockRepo) RejectExpense(ctx context.Context, tenantID, expenseID uuid.UUID, reason string) error {
	if m.rejectExpenseFn != nil {
		return m.rejectExpenseFn(ctx, tenantID, expenseID, reason)
	}
	return nil
}
func (m *mockRepo) CreatePurchaseOrder(ctx context.Context, po *PurchaseOrder) error {
	if m.createPOFn != nil {
		return m.createPOFn(ctx, po)
	}
	return nil
}
func (m *mockRepo) GetPurchaseOrder(ctx context.Context, tenantID, id uuid.UUID) (*PurchaseOrder, error) {
	if m.getPOFn != nil {
		return m.getPOFn(ctx, tenantID, id)
	}
	return &PurchaseOrder{ID: id, TenantID: tenantID}, nil
}
func (m *mockRepo) ListPurchaseOrders(ctx context.Context, tenantID, prodID uuid.UUID, status string, limit, offset int) ([]PurchaseOrder, error) {
	if m.listPOsFn != nil {
		return m.listPOsFn(ctx, tenantID, prodID, status, limit, offset)
	}
	return nil, nil
}
func (m *mockRepo) UpdatePurchaseOrder(ctx context.Context, po *PurchaseOrder) error {
	if m.updatePOFn != nil {
		return m.updatePOFn(ctx, po)
	}
	return nil
}
func (m *mockRepo) CreatePettyCashAdvance(ctx context.Context, pc *PettyCashAdvance) error {
	if m.createPettyCashFn != nil {
		return m.createPettyCashFn(ctx, pc)
	}
	return nil
}
func (m *mockRepo) GetPettyCashAdvance(ctx context.Context, tenantID, id uuid.UUID) (*PettyCashAdvance, error) {
	if m.getPettyCashFn != nil {
		return m.getPettyCashFn(ctx, tenantID, id)
	}
	return &PettyCashAdvance{ID: id, TenantID: tenantID}, nil
}
func (m *mockRepo) ListPettyCashAdvances(ctx context.Context, tenantID, prodID uuid.UUID, status string, limit, offset int) ([]PettyCashAdvance, error) {
	if m.listPettyCashFn != nil {
		return m.listPettyCashFn(ctx, tenantID, prodID, status, limit, offset)
	}
	return nil, nil
}
func (m *mockRepo) UpdatePettyCashAdvance(ctx context.Context, pc *PettyCashAdvance) error {
	if m.updatePettyCashFn != nil {
		return m.updatePettyCashFn(ctx, pc)
	}
	return nil
}

// --- Vertical config fixture (movie-production) ---

func movieProductionConfig() *vertical.Config {
	return &vertical.Config{
		ID:      "movie-production",
		Name:    "Movie & Film Production",
		Version: "1.0.0",
		EntityLabels: vertical.EntityLabels{
			Project:          "Production",
			ProjectPlural:    "Productions",
			Phase:            "Phase",
			PhasePlural:      "Phases",
			TeamMember:       "Crew Member",
			TeamMemberPlural: "Crew Members",
			RateLabel:        "Day Rate",
			RateUnit:         "day",
		},
		ExpenseCategories: []vertical.ExpenseCategory{
			{ID: "location_rental", Label: "Location Rental", TaxTreatment: "input_gst", DefaultAccountCode: "5200", RequiresPO: true},
			{ID: "catering", Label: "Catering", TaxTreatment: "input_gst", DefaultAccountCode: "5300", RequiresPO: false},
			{ID: "transport", Label: "Transport", TaxTreatment: "none", DefaultAccountCode: "5400", RequiresPO: false},
		},
		ApprovalWorkflow: vertical.ApprovalWorkflow{
			Limits: []vertical.ApprovalLimit{
				{Role: "coordinator", MaxAmount: decimal.NewFromInt(200000)},
				{Role: "manager", MaxAmount: decimal.NewFromInt(1000000)},
			},
			DualApprovalAbove: decimal.NewFromInt(1000000),
		},
	}
}

func ctxWithVertical() context.Context {
	return vertical.WithConfig(context.Background(), movieProductionConfig())
}

// --- Tests ---

func TestSubmitExpense_ValidCategory(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	poID := uuid.New()
	exp := &Expense{
		ProductionID:    uuid.New(),
		TenantID:        uuid.New(),
		CategoryID:      "location_rental",
		Amount:          decimal.NewFromInt(50000),
		Currency:        "INR",
		SubmittedBy:     uuid.New(),
		PurchaseOrderID: &poID,
	}
	err := svc.SubmitExpense(ctxWithVertical(), exp)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, exp.ID)
	assert.Equal(t, "submitted", exp.Status)
}

func TestSubmitExpense_InvalidCategory(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	exp := &Expense{
		ProductionID: uuid.New(),
		TenantID:     uuid.New(),
		CategoryID:   "cloud_hosting", // not in movie-production vertical
		Amount:       decimal.NewFromInt(10000),
		Currency:     "INR",
		SubmittedBy:  uuid.New(),
	}
	err := svc.SubmitExpense(ctxWithVertical(), exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidCategory)
}

func TestSubmitExpense_PO_Required_But_Missing(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	exp := &Expense{
		ProductionID: uuid.New(),
		TenantID:     uuid.New(),
		CategoryID:   "location_rental", // RequiresPO = true
		Amount:       decimal.NewFromInt(50000),
		Currency:     "INR",
		SubmittedBy:  uuid.New(),
		// PurchaseOrderID intentionally nil
	}
	err := svc.SubmitExpense(ctxWithVertical(), exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPORequired)
}

func TestSubmitExpense_NoPO_Category_Without_Requirement(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	exp := &Expense{
		ProductionID: uuid.New(),
		TenantID:     uuid.New(),
		CategoryID:   "catering", // RequiresPO = false
		Amount:       decimal.NewFromInt(5000),
		Currency:     "INR",
		SubmittedBy:  uuid.New(),
	}
	err := svc.SubmitExpense(ctxWithVertical(), exp)
	require.NoError(t, err)
	assert.Equal(t, "submitted", exp.Status)
}

func TestApproveExpense_WithinLimit(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	expenseID := uuid.New()

	svc := NewService(&mockRepo{
		getExpenseFn: func(ctx context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{
				ID:       id,
				TenantID: tid,
				Amount:   decimal.NewFromInt(150000), // within coordinator's 200k limit
				Status:   "submitted",
			}, nil
		},
	})

	err := svc.ApproveExpense(ctxWithVertical(), tenantID, expenseID, uuid.New(), "coordinator")
	require.NoError(t, err)
}

func TestApproveExpense_ExceedsLimit(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	expenseID := uuid.New()

	svc := NewService(&mockRepo{
		getExpenseFn: func(ctx context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{
				ID:       id,
				TenantID: tid,
				Amount:   decimal.NewFromInt(300000), // exceeds coordinator's 200k limit
				Status:   "submitted",
			}, nil
		},
	})

	err := svc.ApproveExpense(ctxWithVertical(), tenantID, expenseID, uuid.New(), "coordinator")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrApprovalLimitExceeded)
}

func TestApproveExpense_DualApprovalThreshold(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	expenseID := uuid.New()

	svc := NewService(&mockRepo{
		getExpenseFn: func(ctx context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{
				ID:       id,
				TenantID: tid,
				Amount:   decimal.NewFromInt(1500000), // exceeds dual approval threshold of 1M
				Status:   "submitted",
			}, nil
		},
	})

	// manager has 1M limit, but amount is 1.5M — exceeds their limit
	err := svc.ApproveExpense(ctxWithVertical(), tenantID, expenseID, uuid.New(), "manager")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrApprovalLimitExceeded)
}

func TestApproveExpense_ExactDualApprovalBoundary(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	expenseID := uuid.New()

	svc := NewService(&mockRepo{
		getExpenseFn: func(ctx context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{
				ID:       id,
				TenantID: tid,
				Amount:   decimal.NewFromInt(1000001), // just over dual approval threshold
				Status:   "submitted",
			}, nil
		},
	})

	// manager has 1M limit — amount exceeds it
	err := svc.ApproveExpense(ctxWithVertical(), tenantID, expenseID, uuid.New(), "manager")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrApprovalLimitExceeded)
}

func TestGetExpenseCategories(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})
	cats := svc.GetExpenseCategories(ctxWithVertical())
	assert.Len(t, cats, 3)
	assert.Equal(t, "location_rental", cats[0].ID)
	assert.True(t, cats[0].RequiresPO)
	assert.False(t, cats[1].RequiresPO)
}

func TestGetApprovalLimits(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})
	wf := svc.GetApprovalLimits(ctxWithVertical())
	assert.Len(t, wf.Limits, 2)
	assert.Equal(t, "coordinator", wf.Limits[0].Role)
	assert.True(t, wf.DualApprovalAbove.Equal(decimal.NewFromInt(1000000)))
}

// --- Additional coverage tests ---

func TestApproveExpense_AlreadyApproved(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tenantID, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tenantID, Amount: decimal.NewFromInt(10000), Status: "approved"}, nil
		},
	})

	err := svc.ApproveExpense(ctxWithVertical(), uuid.New(), uuid.New(), uuid.New(), "coordinator")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAlreadyApproved)
}

func TestApproveExpense_UnknownRole(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	err := svc.ApproveExpense(ctxWithVertical(), uuid.New(), uuid.New(), uuid.New(), "receptionist")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrApprovalLimitExceeded)
}

func TestGetExpense_Success(t *testing.T) {
	t.Parallel()
	expID := uuid.New()
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tenantID, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tenantID, Amount: decimal.NewFromInt(5000), Status: "submitted"}, nil
		},
	})

	exp, err := svc.GetExpense(ctxWithVertical(), uuid.New(), expID)
	require.NoError(t, err)
	assert.Equal(t, expID, exp.ID)
}

func TestListExpenses_DefaultLimit(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	svc := NewService(&mockRepo{
		listExpensesFn: func(_ context.Context, _, _ uuid.UUID, _ string, limit, _ int) ([]Expense, error) {
			capturedLimit = limit
			return nil, nil
		},
	})

	_, err := svc.ListExpenses(ctxWithVertical(), uuid.New(), uuid.New(), "", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 20, capturedLimit)
}

func TestListExpenses_MaxLimitEnforced(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	svc := NewService(&mockRepo{
		listExpensesFn: func(_ context.Context, _, _ uuid.UUID, _ string, limit, _ int) ([]Expense, error) {
			capturedLimit = limit
			return nil, nil
		},
	})

	_, err := svc.ListExpenses(ctxWithVertical(), uuid.New(), uuid.New(), "", 9999, 0)
	require.NoError(t, err)
	assert.Equal(t, 20, capturedLimit)
}

func TestCreatePurchaseOrder_GeneratesID(t *testing.T) {
	t.Parallel()
	var saved *PurchaseOrder
	svc := NewService(&mockRepo{
		createPOFn: func(_ context.Context, po *PurchaseOrder) error {
			saved = po
			return nil
		},
	})

	po := &PurchaseOrder{TenantID: uuid.New(), ProductionID: uuid.New()}
	err := svc.CreatePurchaseOrder(context.Background(), po)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, saved.ID)
}

func TestGetPurchaseOrder_Success(t *testing.T) {
	t.Parallel()
	poID := uuid.New()
	svc := NewService(&mockRepo{
		getPOFn: func(_ context.Context, tenantID, id uuid.UUID) (*PurchaseOrder, error) {
			return &PurchaseOrder{ID: id, TenantID: tenantID}, nil
		},
	})

	po, err := svc.GetPurchaseOrder(context.Background(), uuid.New(), poID)
	require.NoError(t, err)
	assert.Equal(t, poID, po.ID)
}

func TestListPurchaseOrders_DefaultLimit(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	svc := NewService(&mockRepo{
		listPOsFn: func(_ context.Context, _, _ uuid.UUID, _ string, limit, _ int) ([]PurchaseOrder, error) {
			capturedLimit = limit
			return nil, nil
		},
	})

	_, err := svc.ListPurchaseOrders(context.Background(), uuid.New(), uuid.New(), "", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 20, capturedLimit)
}

func TestCreatePettyCashAdvance_GeneratesID(t *testing.T) {
	t.Parallel()
	var saved *PettyCashAdvance
	svc := NewService(&mockRepo{
		createPettyCashFn: func(_ context.Context, pc *PettyCashAdvance) error {
			saved = pc
			return nil
		},
	})

	pc := &PettyCashAdvance{TenantID: uuid.New()}
	err := svc.CreatePettyCashAdvance(context.Background(), pc)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, saved.ID)
}

func TestService_RejectExpense_AlreadyApproved(t *testing.T) {
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, Status: "approved"}, nil
		},
	})
	err := svc.RejectExpense(context.Background(), uuid.New(), uuid.New(), "dup")
	require.ErrorIs(t, err, ErrAlreadyApproved)
}

func TestService_RejectExpense_AlreadyRejected(t *testing.T) {
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, Status: "rejected"}, nil
		},
	})
	err := svc.RejectExpense(context.Background(), uuid.New(), uuid.New(), "dup")
	require.ErrorIs(t, err, ErrAlreadyRejected)
}

func TestService_RejectExpense_Success(t *testing.T) {
	var gotReason string
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, Status: "submitted"}, nil
		},
		rejectExpenseFn: func(_ context.Context, _, _ uuid.UUID, reason string) error {
			gotReason = reason
			return nil
		},
	})
	require.NoError(t, svc.RejectExpense(context.Background(), uuid.New(), uuid.New(), "over budget"))
	assert.Equal(t, "over budget", gotReason)
}

func TestService_ApprovePurchaseOrder_SetsApproved(t *testing.T) {
	var saved *PurchaseOrder
	svc := NewService(&mockRepo{
		getPOFn: func(_ context.Context, tid, id uuid.UUID) (*PurchaseOrder, error) {
			return &PurchaseOrder{ID: id, TenantID: tid, Status: "draft"}, nil
		},
		updatePOFn: func(_ context.Context, po *PurchaseOrder) error { saved = po; return nil },
	})
	approverID := uuid.New()
	require.NoError(t, svc.ApprovePurchaseOrder(context.Background(), uuid.New(), uuid.New(), approverID))
	assert.Equal(t, "approved", saved.Status)
	require.NotNil(t, saved.ApprovedAt)
	require.NotNil(t, saved.ApprovedBy)
	assert.Equal(t, approverID, *saved.ApprovedBy)
}

func TestService_ApprovePurchaseOrder_AlreadyApproved(t *testing.T) {
	svc := NewService(&mockRepo{
		getPOFn: func(_ context.Context, tid, id uuid.UUID) (*PurchaseOrder, error) {
			return &PurchaseOrder{ID: id, TenantID: tid, Status: "approved"}, nil
		},
	})
	assert.ErrorIs(t, svc.ApprovePurchaseOrder(context.Background(), uuid.New(), uuid.New(), uuid.New()), ErrAlreadyApproved)
}

func TestService_SettlePettyCash_AlreadySettled(t *testing.T) {
	svc := NewService(&mockRepo{
		getPettyCashFn: func(_ context.Context, tid, id uuid.UUID) (*PettyCashAdvance, error) {
			return &PettyCashAdvance{ID: id, TenantID: tid, Status: "settled"}, nil
		},
	})
	require.ErrorIs(t, svc.SettlePettyCash(context.Background(), uuid.New(), uuid.New(), decimal.RequireFromString("12.50")), ErrAlreadySettled)
}

func TestService_SettlePettyCash_SetsSettled(t *testing.T) {
	var saved *PettyCashAdvance
	svc := NewService(&mockRepo{
		getPettyCashFn: func(_ context.Context, tid, id uuid.UUID) (*PettyCashAdvance, error) {
			return &PettyCashAdvance{ID: id, TenantID: tid, Status: "issued"}, nil
		},
		updatePettyCashFn: func(_ context.Context, pc *PettyCashAdvance) error { saved = pc; return nil },
	})
	require.NoError(t, svc.SettlePettyCash(context.Background(), uuid.New(), uuid.New(), decimal.RequireFromString("12.50")))
	assert.Equal(t, "settled", saved.Status)
	assert.Equal(t, "12.50", saved.UnspentAmount.StringFixed(2))
	require.NotNil(t, saved.SettledAt)
}

func TestService_CreatePurchaseOrder_GeneratesPONumberWhenEmpty(t *testing.T) {
	var saved *PurchaseOrder
	svc := NewService(&mockRepo{
		createPOFn: func(_ context.Context, po *PurchaseOrder) error { saved = po; return nil },
	})
	po := &PurchaseOrder{TenantID: uuid.New(), ProductionID: uuid.New(), Amount: decimal.NewFromInt(100)}
	require.NoError(t, svc.CreatePurchaseOrder(context.Background(), po))
	assert.NotEmpty(t, saved.PONumber)
	assert.True(t, strings.HasPrefix(saved.PONumber, "PO-"))
}

func TestService_CreatePurchaseOrder_KeepsSuppliedPONumber(t *testing.T) {
	var saved *PurchaseOrder
	svc := NewService(&mockRepo{
		createPOFn: func(_ context.Context, po *PurchaseOrder) error { saved = po; return nil },
	})
	po := &PurchaseOrder{TenantID: uuid.New(), ProductionID: uuid.New(), Amount: decimal.NewFromInt(100), PONumber: "PO-CUSTOM-1"}
	require.NoError(t, svc.CreatePurchaseOrder(context.Background(), po))
	assert.Equal(t, "PO-CUSTOM-1", saved.PONumber)
}
