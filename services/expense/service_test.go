package expense

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// --- Mock repository ---

type mockRepo struct {
	createExpenseFn     func(ctx context.Context, e *Expense) error
	getExpenseFn        func(ctx context.Context, tenantID, id uuid.UUID) (*Expense, error)
	listExpensesFn      func(ctx context.Context, tenantID, prodID uuid.UUID, status string, limit, offset int) ([]Expense, error)
	updateExpenseFn     func(ctx context.Context, e *Expense) error
	createPOFn          func(ctx context.Context, po *PurchaseOrder) error
	getPOFn             func(ctx context.Context, tenantID, id uuid.UUID) (*PurchaseOrder, error)
	listPOsFn           func(ctx context.Context, tenantID, prodID uuid.UUID, status string, limit, offset int) ([]PurchaseOrder, error)
	updatePOFn          func(ctx context.Context, po *PurchaseOrder) error
	createPettyCashFn   func(ctx context.Context, pc *PettyCashAdvance) error
	getPettyCashFn      func(ctx context.Context, tenantID, id uuid.UUID) (*PettyCashAdvance, error)
	listPettyCashFn     func(ctx context.Context, tenantID, prodID uuid.UUID, status string, limit, offset int) ([]PettyCashAdvance, error)
	updatePettyCashFn   func(ctx context.Context, pc *PettyCashAdvance) error
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
				{Role: "line_producer", MaxAmount: decimal.NewFromInt(200000)},
				{Role: "executive_producer", MaxAmount: decimal.NewFromInt(1000000)},
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
				Amount:   decimal.NewFromInt(150000), // within line_producer's 200k limit
				Status:   "submitted",
			}, nil
		},
	})

	err := svc.ApproveExpense(ctxWithVertical(), tenantID, expenseID, uuid.New(), "line_producer")
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
				Amount:   decimal.NewFromInt(300000), // exceeds line_producer's 200k limit
				Status:   "submitted",
			}, nil
		},
	})

	err := svc.ApproveExpense(ctxWithVertical(), tenantID, expenseID, uuid.New(), "line_producer")
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

	// executive_producer has 1M limit, but amount is 1.5M — exceeds their limit
	err := svc.ApproveExpense(ctxWithVertical(), tenantID, expenseID, uuid.New(), "executive_producer")
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

	// executive_producer has 1M limit — amount exceeds it
	err := svc.ApproveExpense(ctxWithVertical(), tenantID, expenseID, uuid.New(), "executive_producer")
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
	assert.Equal(t, "line_producer", wf.Limits[0].Role)
	assert.True(t, wf.DualApprovalAbove.Equal(decimal.NewFromInt(1000000)))
}
