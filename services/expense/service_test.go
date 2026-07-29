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
	createExpenseFn func(ctx context.Context, e *Expense) error
	getExpenseFn    func(ctx context.Context, tenantID, id uuid.UUID) (*Expense, error)
	listExpensesFn  func(ctx context.Context, tenantID, prodID uuid.UUID, status string, limit, offset int, submittedBy uuid.UUID) ([]Expense, error)
	// lastListExpensesSubmittedBy records the submittedBy arg from the most
	// recent ListExpenses call, so Task 3's handler test can assert the
	// submitted_by_me flag actually reached the repo call.
	lastListExpensesSubmittedBy uuid.UUID
	updateExpenseFn             func(ctx context.Context, e *Expense) error
	rejectExpenseFn             func(ctx context.Context, tenantID, expenseID, rejecterID uuid.UUID, reason string) error
	createPOFn                  func(ctx context.Context, po *PurchaseOrder) error
	getPOFn                     func(ctx context.Context, tenantID, id uuid.UUID) (*PurchaseOrder, error)
	listPOsFn                   func(ctx context.Context, tenantID, prodID uuid.UUID, status string, limit, offset int) ([]PurchaseOrder, error)
	updatePOFn                  func(ctx context.Context, po *PurchaseOrder) error
	createPettyCashFn           func(ctx context.Context, pc *PettyCashAdvance) error
	getPettyCashFn              func(ctx context.Context, tenantID, id uuid.UUID) (*PettyCashAdvance, error)
	listPettyCashFn             func(ctx context.Context, tenantID, prodID uuid.UUID, status string, limit, offset int) ([]PettyCashAdvance, error)
	updatePettyCashFn           func(ctx context.Context, pc *PettyCashAdvance) error
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
func (m *mockRepo) ListExpenses(ctx context.Context, tenantID, prodID uuid.UUID, status string, limit, offset int, submittedBy uuid.UUID) ([]Expense, error) {
	m.lastListExpensesSubmittedBy = submittedBy
	if m.listExpensesFn != nil {
		return m.listExpensesFn(ctx, tenantID, prodID, status, limit, offset, submittedBy)
	}
	return nil, nil
}
func (m *mockRepo) UpdateExpense(ctx context.Context, e *Expense) error {
	if m.updateExpenseFn != nil {
		return m.updateExpenseFn(ctx, e)
	}
	return nil
}
func (m *mockRepo) RejectExpense(ctx context.Context, tenantID, expenseID, rejecterID uuid.UUID, reason string) error {
	if m.rejectExpenseFn != nil {
		return m.rejectExpenseFn(ctx, tenantID, expenseID, rejecterID, reason)
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

	err := svc.ApproveExpense(ctxWithVertical(), tenantID, expenseID, uuid.New(), []string{"coordinator"})
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

	err := svc.ApproveExpense(ctxWithVertical(), tenantID, expenseID, uuid.New(), []string{"coordinator"})
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
	err := svc.ApproveExpense(ctxWithVertical(), tenantID, expenseID, uuid.New(), []string{"manager"})
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
	err := svc.ApproveExpense(ctxWithVertical(), tenantID, expenseID, uuid.New(), []string{"manager"})
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

	err := svc.ApproveExpense(ctxWithVertical(), uuid.New(), uuid.New(), uuid.New(), []string{"coordinator"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAlreadyApproved)
}

func TestApproveExpense_UnknownRole(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	err := svc.ApproveExpense(ctxWithVertical(), uuid.New(), uuid.New(), uuid.New(), []string{"receptionist"})
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
		listExpensesFn: func(_ context.Context, _, _ uuid.UUID, _ string, limit, _ int, _ uuid.UUID) ([]Expense, error) {
			capturedLimit = limit
			return nil, nil
		},
	})

	_, err := svc.ListExpenses(ctxWithVertical(), uuid.New(), uuid.New(), "", 0, 0, uuid.Nil)
	require.NoError(t, err)
	assert.Equal(t, 20, capturedLimit)
}

func TestListExpenses_MaxLimitEnforced(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	svc := NewService(&mockRepo{
		listExpensesFn: func(_ context.Context, _, _ uuid.UUID, _ string, limit, _ int, _ uuid.UUID) ([]Expense, error) {
			capturedLimit = limit
			return nil, nil
		},
	})

	_, err := svc.ListExpenses(ctxWithVertical(), uuid.New(), uuid.New(), "", 9999, 0, uuid.Nil)
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
	err := svc.RejectExpense(context.Background(), uuid.New(), uuid.New(), uuid.New(), "dup")
	require.ErrorIs(t, err, ErrAlreadyApproved)
}

func TestService_RejectExpense_AlreadyRejected(t *testing.T) {
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, Status: "rejected"}, nil
		},
	})
	err := svc.RejectExpense(context.Background(), uuid.New(), uuid.New(), uuid.New(), "dup")
	require.ErrorIs(t, err, ErrAlreadyRejected)
}

func TestService_RejectExpense_Success(t *testing.T) {
	rejecter := uuid.New()
	var gotReason string
	var gotRejecter uuid.UUID
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, Status: "submitted"}, nil
		},
		rejectExpenseFn: func(_ context.Context, _, _, rejecterID uuid.UUID, reason string) error {
			gotReason = reason
			gotRejecter = rejecterID
			return nil
		},
	})
	require.NoError(t, svc.RejectExpense(context.Background(), uuid.New(), uuid.New(), rejecter, "over budget"))
	assert.Equal(t, "over budget", gotReason)
	assert.Equal(t, rejecter, gotRejecter)
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
	require.NoError(t, svc.ApprovePurchaseOrder(ctxWithVertical(), uuid.New(), uuid.New(), approverID, []string{"coordinator"}))
	assert.Equal(t, "approved", saved.Status)
	require.NotNil(t, saved.ApprovedAt)
	require.NotNil(t, saved.ApprovedBy)
	assert.Equal(t, approverID, *saved.ApprovedBy)
}

func TestService_SettlePettyCash_AlreadySettled(t *testing.T) {
	holder := uuid.New()
	svc := NewService(&mockRepo{
		getPettyCashFn: func(_ context.Context, tid, id uuid.UUID) (*PettyCashAdvance, error) {
			return &PettyCashAdvance{ID: id, TenantID: tid, Status: "settled", IssuedTo: holder}, nil
		},
	})
	require.ErrorIs(t, svc.SettlePettyCash(context.Background(), uuid.New(), uuid.New(), holder, decimal.RequireFromString("12.50")), ErrAlreadySettled)
}

func TestService_SettlePettyCash_SetsSettled(t *testing.T) {
	holder := uuid.New()
	var saved *PettyCashAdvance
	svc := NewService(&mockRepo{
		getPettyCashFn: func(_ context.Context, tid, id uuid.UUID) (*PettyCashAdvance, error) {
			return &PettyCashAdvance{ID: id, TenantID: tid, Status: "issued", Amount: decimal.RequireFromString("50.00"), IssuedTo: holder}, nil
		},
		updatePettyCashFn: func(_ context.Context, pc *PettyCashAdvance) error { saved = pc; return nil },
	})
	require.NoError(t, svc.SettlePettyCash(context.Background(), uuid.New(), uuid.New(), holder, decimal.RequireFromString("12.50")))
	assert.Equal(t, "settled", saved.Status)
	assert.Equal(t, "12.50", saved.UnspentAmount.StringFixed(2))
	require.NotNil(t, saved.SettledAt)
}

func TestService_SettlePettyCash_ExceedsAdvance(t *testing.T) {
	holder := uuid.New()
	svc := NewService(&mockRepo{
		getPettyCashFn: func(_ context.Context, tid, id uuid.UUID) (*PettyCashAdvance, error) {
			return &PettyCashAdvance{ID: id, TenantID: tid, Status: "issued", Amount: decimal.RequireFromString("100.00"), IssuedTo: holder}, nil
		},
	})
	err := svc.SettlePettyCash(context.Background(), uuid.New(), uuid.New(), holder, decimal.RequireFromString("150.00"))
	require.ErrorIs(t, err, ErrUnspentExceedsAdvance)
}

func TestService_SettlePettyCash_NotHolder(t *testing.T) {
	svc := NewService(&mockRepo{
		getPettyCashFn: func(_ context.Context, tid, id uuid.UUID) (*PettyCashAdvance, error) {
			return &PettyCashAdvance{ID: id, TenantID: tid, Status: "issued", Amount: decimal.NewFromInt(1000), IssuedTo: uuid.New()}, nil
		},
	})
	err := svc.SettlePettyCash(context.Background(), uuid.New(), uuid.New(), uuid.New(), decimal.NewFromInt(10))
	assert.ErrorIs(t, err, ErrNotAdvanceHolder)
}

func TestService_SettlePettyCash_HolderSucceeds(t *testing.T) {
	holder := uuid.New()
	var saved *PettyCashAdvance
	svc := NewService(&mockRepo{
		getPettyCashFn: func(_ context.Context, tid, id uuid.UUID) (*PettyCashAdvance, error) {
			return &PettyCashAdvance{ID: id, TenantID: tid, Status: "issued", Amount: decimal.NewFromInt(1000), IssuedTo: holder}, nil
		},
		updatePettyCashFn: func(_ context.Context, pc *PettyCashAdvance) error { saved = pc; return nil },
	})
	require.NoError(t, svc.SettlePettyCash(context.Background(), uuid.New(), uuid.New(), holder, decimal.NewFromInt(10)))
	assert.Equal(t, "settled", saved.Status)
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

func TestService_ApproveExpense_SucceedsWithRoleLimit(t *testing.T) {
	// Regression: proves the P0 fix — an empty role used to always fail.
	var saved *Expense
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, Status: "submitted", Amount: decimal.NewFromInt(5000)}, nil
		},
		updateExpenseFn: func(_ context.Context, e *Expense) error { saved = e; return nil },
	})
	err := svc.ApproveExpense(ctxWithVertical(), uuid.New(), uuid.New(), uuid.New(), []string{"manager"})
	require.NoError(t, err)
	assert.Equal(t, "approved", saved.Status)
}

func TestService_ApproveExpense_NoLimitRoleFails(t *testing.T) {
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, Status: "submitted", Amount: decimal.NewFromInt(5000)}, nil
		},
	})
	err := svc.ApproveExpense(ctxWithVertical(), uuid.New(), uuid.New(), uuid.New(), []string{"member"})
	assert.ErrorIs(t, err, ErrApprovalLimitExceeded)
}

func TestService_ApprovePurchaseOrder_WithinLimit(t *testing.T) {
	var saved *PurchaseOrder
	svc := NewService(&mockRepo{
		getPOFn: func(_ context.Context, tid, id uuid.UUID) (*PurchaseOrder, error) {
			return &PurchaseOrder{ID: id, TenantID: tid, Status: "draft", Amount: decimal.NewFromInt(5000)}, nil
		},
		updatePOFn: func(_ context.Context, po *PurchaseOrder) error { saved = po; return nil },
	})
	require.NoError(t, svc.ApprovePurchaseOrder(ctxWithVertical(), uuid.New(), uuid.New(), uuid.New(), []string{"manager"}))
	assert.Equal(t, "approved", saved.Status)
}

func TestService_ApprovePurchaseOrder_OverLimit(t *testing.T) {
	svc := NewService(&mockRepo{
		getPOFn: func(_ context.Context, tid, id uuid.UUID) (*PurchaseOrder, error) {
			return &PurchaseOrder{ID: id, TenantID: tid, Status: "draft", Amount: decimal.NewFromInt(500000)}, nil
		},
	})
	assert.ErrorIs(t, svc.ApprovePurchaseOrder(ctxWithVertical(), uuid.New(), uuid.New(), uuid.New(), []string{"coordinator"}), ErrApprovalLimitExceeded)
}

func TestService_ApprovePurchaseOrder_DualApproval(t *testing.T) {
	// movieProductionConfig's manager limit (1,000,000) equals DualApprovalAbove,
	// so no configured role can ever clear the limit check and still reach the
	// dual-approval check. Raise manager's limit here so 2,000,000 clears the
	// role-limit gate but still trips the dual-approval threshold.
	cfg := movieProductionConfig()
	cfg.ApprovalWorkflow.Limits[1] = vertical.ApprovalLimit{Role: "manager", MaxAmount: decimal.NewFromInt(5000000)}
	ctx := vertical.WithConfig(context.Background(), cfg)

	svc := NewService(&mockRepo{
		getPOFn: func(_ context.Context, tid, id uuid.UUID) (*PurchaseOrder, error) {
			return &PurchaseOrder{ID: id, TenantID: tid, Status: "draft", Amount: decimal.NewFromInt(2000000)}, nil
		},
	})
	assert.ErrorIs(t, svc.ApprovePurchaseOrder(ctx, uuid.New(), uuid.New(), uuid.New(), []string{"manager"}), ErrDualApprovalRequired)
}

func TestService_ApproveExpense_RejectedNotApprovable(t *testing.T) {
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, Status: "rejected", Amount: decimal.NewFromInt(5000)}, nil
		},
	})
	err := svc.ApproveExpense(ctxWithVertical(), uuid.New(), uuid.New(), uuid.New(), []string{"manager"})
	assert.ErrorIs(t, err, ErrAlreadyRejected)
}

func TestService_ApproveExpense_PaidNotApprovable(t *testing.T) {
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, Status: "paid", Amount: decimal.NewFromInt(5000)}, nil
		},
	})
	err := svc.ApproveExpense(ctxWithVertical(), uuid.New(), uuid.New(), uuid.New(), []string{"manager"})
	assert.ErrorIs(t, err, ErrNotApprovable)
}

func TestService_ApproveExpense_DraftNotApprovable(t *testing.T) {
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, Status: "draft", Amount: decimal.NewFromInt(5000)}, nil
		},
	})
	err := svc.ApproveExpense(ctxWithVertical(), uuid.New(), uuid.New(), uuid.New(), []string{"manager"})
	assert.ErrorIs(t, err, ErrNotApprovable)
}

func TestService_ApprovePurchaseOrder_CancelledNotApprovable(t *testing.T) {
	svc := NewService(&mockRepo{
		getPOFn: func(_ context.Context, tid, id uuid.UUID) (*PurchaseOrder, error) {
			return &PurchaseOrder{ID: id, TenantID: tid, Status: "cancelled", Amount: decimal.NewFromInt(5000)}, nil
		},
	})
	err := svc.ApprovePurchaseOrder(ctxWithVertical(), uuid.New(), uuid.New(), uuid.New(), []string{"manager"})
	assert.ErrorIs(t, err, ErrNotApprovable)
}

func TestService_ApprovePurchaseOrder_AlreadyApproved(t *testing.T) {
	svc := NewService(&mockRepo{
		getPOFn: func(_ context.Context, tid, id uuid.UUID) (*PurchaseOrder, error) {
			return &PurchaseOrder{ID: id, TenantID: tid, Status: "approved", Amount: decimal.NewFromInt(5000)}, nil
		},
	})
	assert.ErrorIs(t, svc.ApprovePurchaseOrder(ctxWithVertical(), uuid.New(), uuid.New(), uuid.New(), []string{"manager"}), ErrAlreadyApproved)
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

// --- super_admin approval bypass (#199) ---

// approvalConfigConstruction mirrors the (post-#199) construction approval tiers:
// coordinator 100k / accountant 500k / manager 2M, dual-approval above 1M.
func approvalConfigConstruction() *vertical.Config {
	c := movieProductionConfig()
	c.ApprovalWorkflow = vertical.ApprovalWorkflow{
		Limits: []vertical.ApprovalLimit{
			{Role: "coordinator", MaxAmount: decimal.NewFromInt(100000)},
			{Role: "accountant", MaxAmount: decimal.NewFromInt(500000)},
			{Role: "manager", MaxAmount: decimal.NewFromInt(2000000)},
		},
		DualApprovalAbove: decimal.NewFromInt(1000000),
	}
	return c
}

func ctxConstruction() context.Context {
	return vertical.WithConfig(context.Background(), approvalConfigConstruction())
}

func TestService_ApproveExpense_SuperAdminBypassesLimit(t *testing.T) {
	// 800k exceeds every configured per-role limit reachable by ["super_admin"]
	// (which has no limit entry → MaxLimitForRoles=nil), but is below the 1M dual
	// threshold. Without the bypass this would fail ErrApprovalLimitExceeded.
	var saved *Expense
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, Status: "submitted", Amount: decimal.NewFromInt(800000)}, nil
		},
		updateExpenseFn: func(_ context.Context, e *Expense) error { saved = e; return nil },
	})
	err := svc.ApproveExpense(ctxConstruction(), uuid.New(), uuid.New(), uuid.New(), []string{"super_admin"})
	require.NoError(t, err)
	assert.Equal(t, "approved", saved.Status)
}

func TestService_ApproveExpense_SuperAdminStillHitsDualApproval(t *testing.T) {
	// 1.5M is above the 1M dual threshold — dual approval applies to everyone.
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, Status: "submitted", Amount: decimal.NewFromInt(1500000)}, nil
		},
	})
	err := svc.ApproveExpense(ctxConstruction(), uuid.New(), uuid.New(), uuid.New(), []string{"super_admin"})
	assert.ErrorIs(t, err, ErrDualApprovalRequired)
}

func TestService_ApproveExpense_AccountantWithinTier(t *testing.T) {
	var saved *Expense
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, Status: "submitted", Amount: decimal.NewFromInt(400000)}, nil
		},
		updateExpenseFn: func(_ context.Context, e *Expense) error { saved = e; return nil },
	})
	require.NoError(t, svc.ApproveExpense(ctxConstruction(), uuid.New(), uuid.New(), uuid.New(), []string{"accountant"}))
	assert.Equal(t, "approved", saved.Status)
}

func TestService_ApproveExpense_AccountantAboveTierFails(t *testing.T) {
	// 600k exceeds accountant's 500k limit (accountant is not super_admin).
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, Status: "submitted", Amount: decimal.NewFromInt(600000)}, nil
		},
	})
	assert.ErrorIs(t, svc.ApproveExpense(ctxConstruction(), uuid.New(), uuid.New(), uuid.New(), []string{"accountant"}), ErrApprovalLimitExceeded)
}

func TestService_ApprovePurchaseOrder_SuperAdminBypassesLimit(t *testing.T) {
	var saved *PurchaseOrder
	svc := NewService(&mockRepo{
		getPOFn: func(_ context.Context, tid, id uuid.UUID) (*PurchaseOrder, error) {
			return &PurchaseOrder{ID: id, TenantID: tid, Status: "draft", Amount: decimal.NewFromInt(800000)}, nil
		},
		updatePOFn: func(_ context.Context, po *PurchaseOrder) error { saved = po; return nil },
	})
	require.NoError(t, svc.ApprovePurchaseOrder(ctxConstruction(), uuid.New(), uuid.New(), uuid.New(), []string{"super_admin"}))
	assert.Equal(t, "approved", saved.Status)
}

func TestService_ApprovePurchaseOrder_SuperAdminStillHitsDualApproval(t *testing.T) {
	svc := NewService(&mockRepo{
		getPOFn: func(_ context.Context, tid, id uuid.UUID) (*PurchaseOrder, error) {
			return &PurchaseOrder{ID: id, TenantID: tid, Status: "draft", Amount: decimal.NewFromInt(1500000)}, nil
		},
	})
	assert.ErrorIs(t, svc.ApprovePurchaseOrder(ctxConstruction(), uuid.New(), uuid.New(), uuid.New(), []string{"super_admin"}), ErrDualApprovalRequired)
}
