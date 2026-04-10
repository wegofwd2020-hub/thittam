package budget

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// --- Mock repository ---

type mockRepo struct {
	createBudgetFn          func(ctx context.Context, b *Budget) error
	getBudgetFn             func(ctx context.Context, tenantID, id uuid.UUID) (*Budget, error)
	listBudgetsFn           func(ctx context.Context, tenantID, productionID uuid.UUID, status string, limit, offset int) ([]Budget, error)
	updateBudgetStatusFn    func(ctx context.Context, id uuid.UUID, status string, approvedBy *uuid.UUID) error
	createLineItemFn        func(ctx context.Context, li *BudgetLineItem) error
	getLineItemFn           func(ctx context.Context, id uuid.UUID) (*BudgetLineItem, error)
	listLineItemsFn         func(ctx context.Context, budgetID uuid.UUID) ([]BudgetLineItem, error)
	updateLineItemActualsFn func(ctx context.Context, id uuid.UUID, actual, committed decimal.Decimal) error
	checkLineAvailabilityFn func(ctx context.Context, id uuid.UUID) (decimal.Decimal, error)
}

func (m *mockRepo) CreateBudget(ctx context.Context, b *Budget) error {
	if m.createBudgetFn != nil {
		return m.createBudgetFn(ctx, b)
	}
	return nil
}
func (m *mockRepo) GetBudget(ctx context.Context, tenantID, id uuid.UUID) (*Budget, error) {
	if m.getBudgetFn != nil {
		return m.getBudgetFn(ctx, tenantID, id)
	}
	return &Budget{ID: id, TenantID: tenantID, Label: "Test Budget", Status: "draft"}, nil
}
func (m *mockRepo) ListBudgets(ctx context.Context, tenantID, productionID uuid.UUID, status string, limit, offset int) ([]Budget, error) {
	if m.listBudgetsFn != nil {
		return m.listBudgetsFn(ctx, tenantID, productionID, status, limit, offset)
	}
	return nil, nil
}
func (m *mockRepo) UpdateBudgetStatus(ctx context.Context, id uuid.UUID, status string, approvedBy *uuid.UUID) error {
	if m.updateBudgetStatusFn != nil {
		return m.updateBudgetStatusFn(ctx, id, status, approvedBy)
	}
	return nil
}
func (m *mockRepo) CreateLineItem(ctx context.Context, li *BudgetLineItem) error {
	if m.createLineItemFn != nil {
		return m.createLineItemFn(ctx, li)
	}
	return nil
}
func (m *mockRepo) GetLineItem(ctx context.Context, id uuid.UUID) (*BudgetLineItem, error) {
	if m.getLineItemFn != nil {
		return m.getLineItemFn(ctx, id)
	}
	return &BudgetLineItem{ID: id, CategoryID: "above_the_line"}, nil
}
func (m *mockRepo) ListLineItems(ctx context.Context, budgetID uuid.UUID) ([]BudgetLineItem, error) {
	if m.listLineItemsFn != nil {
		return m.listLineItemsFn(ctx, budgetID)
	}
	return nil, nil
}
func (m *mockRepo) UpdateLineItemActuals(ctx context.Context, id uuid.UUID, actual, committed decimal.Decimal) error {
	if m.updateLineItemActualsFn != nil {
		return m.updateLineItemActualsFn(ctx, id, actual, committed)
	}
	return nil
}
func (m *mockRepo) CheckLineAvailability(ctx context.Context, id uuid.UUID) (decimal.Decimal, error) {
	if m.checkLineAvailabilityFn != nil {
		return m.checkLineAvailabilityFn(ctx, id)
	}
	return decimal.NewFromInt(0), nil
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
		PhaseTypes: []vertical.PhaseType{
			{ID: "development", Label: "Development", Order: 1, IsBillable: false, AllowedTransitions: []string{"pre_production"}},
			{ID: "pre_production", Label: "Pre-Production", Order: 2, IsBillable: false, AllowedTransitions: []string{"production"}},
			{ID: "production", Label: "Production", Order: 3, IsBillable: true, AllowedTransitions: []string{"post_production"}},
			{ID: "post_production", Label: "Post-Production", Order: 4, IsBillable: true, AllowedTransitions: []string{"released"}},
			{ID: "released", Label: "Released", Order: 5, IsBillable: false, AllowedTransitions: []string{}},
		},
		BudgetCategories: []vertical.BudgetCategory{
			{ID: "above_the_line", Label: "Above the Line", DefaultAccountCode: "5100"},
			{ID: "below_the_line", Label: "Below the Line", DefaultAccountCode: "5200"},
		},
		BudgetTemplates: []vertical.BudgetTemplate{
			{
				Name:        "standard_feature",
				Description: "Standard feature film budget",
				LineItems: []vertical.TemplateLineItem{
					{CategoryID: "above_the_line", Description: "Director Fees", AccountCode: "5100", DefaultAmount: decimal.NewFromInt(500000), IsRequired: true},
					{CategoryID: "above_the_line", Description: "Producer Fees", AccountCode: "5100", DefaultAmount: decimal.NewFromInt(300000), IsRequired: true},
					{CategoryID: "below_the_line", Description: "Camera Equipment", AccountCode: "5200", DefaultAmount: decimal.NewFromInt(200000), IsRequired: false},
				},
			},
		},
		ExpenseCategories: []vertical.ExpenseCategory{
			{ID: "location_rental", Label: "Location Rental", TaxTreatment: "input_gst", DefaultAccountCode: "5200", RequiresPO: true},
		},
		ApprovalWorkflow: vertical.ApprovalWorkflow{
			Limits: []vertical.ApprovalLimit{
				{Role: "line_producer", MaxAmount: decimal.NewFromInt(200000)},
				{Role: "executive_producer", MaxAmount: decimal.NewFromInt(1000000)},
			},
			DualApprovalAbove: decimal.NewFromInt(1000000),
		},
		InventoryCategories: []vertical.InventoryCategory{
			{ID: "camera", Label: "Camera Equipment", IsTrackable: true},
		},
		ReportDefinitions: []vertical.ReportDefinition{
			{ID: "cost_report", Name: "Daily Cost Report"},
		},
	}
}

func ctxWithVertical() context.Context {
	return vertical.WithConfig(context.Background(), movieProductionConfig())
}

// --- Tests ---

func TestCreateLineItem_ValidCategory(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	li := &BudgetLineItem{
		BudgetID:       uuid.New(),
		TenantID:       uuid.New(),
		CategoryID:     "above_the_line",
		Description:    "Director Fees",
		BudgetedAmount: decimal.NewFromInt(500000),
	}
	err := svc.CreateLineItem(ctxWithVertical(), li)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, li.ID)
	// Should have picked up default account code from vertical config
	assert.Equal(t, "5100", li.AccountCode)
}

func TestCreateLineItem_InvalidCategory(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	li := &BudgetLineItem{
		BudgetID:       uuid.New(),
		TenantID:       uuid.New(),
		CategoryID:     "nonexistent_category",
		Description:    "Bad Category",
		BudgetedAmount: decimal.NewFromInt(1000),
	}
	err := svc.CreateLineItem(ctxWithVertical(), li)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidCategory)
}

func TestCreateLineItem_ExplicitAccountCode(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	li := &BudgetLineItem{
		BudgetID:       uuid.New(),
		TenantID:       uuid.New(),
		CategoryID:     "above_the_line",
		Description:    "Director Fees",
		AccountCode:    "5150", // explicit override
		BudgetedAmount: decimal.NewFromInt(500000),
	}
	err := svc.CreateLineItem(ctxWithVertical(), li)
	require.NoError(t, err)
	// Should keep the explicitly provided account code
	assert.Equal(t, "5150", li.AccountCode)
}

func TestCreateBudgetFromTemplate(t *testing.T) {
	t.Parallel()
	var createdItems []BudgetLineItem
	svc := NewService(&mockRepo{
		createLineItemFn: func(ctx context.Context, li *BudgetLineItem) error {
			createdItems = append(createdItems, *li)
			return nil
		},
	})

	b := &Budget{
		ProductionID: uuid.New(),
		TenantID:     uuid.New(),
		Label:        "Feature Film Budget",
		CreatedBy:    uuid.New(),
	}
	err := svc.CreateBudgetFromTemplate(ctxWithVertical(), b, "standard_feature")
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, b.ID)
	assert.Equal(t, "draft", b.Status)
	assert.Len(t, createdItems, 3)
	assert.Equal(t, "Director Fees", createdItems[0].Description)
	assert.Equal(t, "5100", createdItems[0].AccountCode)
	assert.True(t, decimal.NewFromInt(500000).Equal(createdItems[0].BudgetedAmount))
}

func TestCreateBudgetFromTemplate_NotFound(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	b := &Budget{TenantID: uuid.New(), Label: "Test"}
	err := svc.CreateBudgetFromTemplate(ctxWithVertical(), b, "nonexistent_template")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "template")
}

func TestApproveBudget_SubmittedStatus(t *testing.T) {
	t.Parallel()
	approver := uuid.New()
	var capturedStatus string
	svc := NewService(&mockRepo{
		getBudgetFn: func(ctx context.Context, tenantID, id uuid.UUID) (*Budget, error) {
			return &Budget{ID: id, TenantID: tenantID, Status: "submitted"}, nil
		},
		updateBudgetStatusFn: func(ctx context.Context, id uuid.UUID, status string, approvedBy *uuid.UUID) error {
			capturedStatus = status
			return nil
		},
	})

	err := svc.ApproveBudget(context.Background(), uuid.New(), uuid.New(), approver)
	require.NoError(t, err)
	assert.Equal(t, "approved", capturedStatus)
}

func TestApproveBudget_DraftStatus_Rejected(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getBudgetFn: func(ctx context.Context, tenantID, id uuid.UUID) (*Budget, error) {
			return &Budget{ID: id, TenantID: tenantID, Status: "draft"}, nil
		},
	})

	err := svc.ApproveBudget(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBudgetNotApproved)
}

func TestApproveBudget_AlreadyApproved(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getBudgetFn: func(ctx context.Context, tenantID, id uuid.UUID) (*Budget, error) {
			return &Budget{ID: id, TenantID: tenantID, Status: "approved"}, nil
		},
	})

	err := svc.ApproveBudget(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAlreadyApproved)
}

func TestCheckLineAvailability(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		checkLineAvailabilityFn: func(ctx context.Context, id uuid.UUID) (decimal.Decimal, error) {
			// Simulates: budgeted 500000 - actual 100000 - committed 50000 = 350000
			return decimal.NewFromInt(350000), nil
		},
	})

	available, err := svc.CheckLineAvailability(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.True(t, decimal.NewFromInt(350000).Equal(available))
}

func TestGetBudgetCategories(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})
	cats := svc.GetBudgetCategories(ctxWithVertical())
	assert.Len(t, cats, 2)
	assert.Equal(t, "above_the_line", cats[0].ID)
	assert.Equal(t, "below_the_line", cats[1].ID)
}

func TestGetBudgetTemplates(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})
	tmpls := svc.GetBudgetTemplates(ctxWithVertical())
	assert.Len(t, tmpls, 1)
	assert.Equal(t, "standard_feature", tmpls[0].Name)
}

func TestCreateBudget_GeneratesID(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})
	b := &Budget{TenantID: uuid.New(), Label: "Test Budget", CreatedBy: uuid.New()}
	err := svc.CreateBudget(context.Background(), b)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, b.ID)
	assert.Equal(t, "draft", b.Status)
}

func TestListBudgets_DefaultLimit(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	svc := NewService(&mockRepo{
		listBudgetsFn: func(ctx context.Context, tenantID, productionID uuid.UUID, status string, limit, offset int) ([]Budget, error) {
			capturedLimit = limit
			return nil, nil
		},
	})
	svc.ListBudgets(context.Background(), uuid.New(), uuid.New(), "", 0, 0)
	assert.Equal(t, 20, capturedLimit)
}

// --- Additional coverage tests ---

func TestGetBudget_Success(t *testing.T) {
	t.Parallel()
	budgetID := uuid.New()
	svc := NewService(&mockRepo{
		getBudgetFn: func(_ context.Context, tenantID, id uuid.UUID) (*Budget, error) {
			return &Budget{ID: id, TenantID: tenantID, Status: "draft"}, nil
		},
	})

	b, err := svc.GetBudget(context.Background(), uuid.New(), budgetID)
	require.NoError(t, err)
	assert.Equal(t, budgetID, b.ID)
}

func TestSubmitBudget_Success(t *testing.T) {
	t.Parallel()
	var capturedStatus string
	svc := NewService(&mockRepo{
		getBudgetFn: func(_ context.Context, tenantID, id uuid.UUID) (*Budget, error) {
			return &Budget{ID: id, TenantID: tenantID, Status: "draft"}, nil
		},
		updateBudgetStatusFn: func(_ context.Context, _ uuid.UUID, status string, _ *uuid.UUID) error {
			capturedStatus = status
			return nil
		},
	})

	err := svc.SubmitBudget(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.NoError(t, err)
	assert.Equal(t, "submitted", capturedStatus)
}

func TestSubmitBudget_NotDraft_Rejected(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getBudgetFn: func(_ context.Context, tenantID, id uuid.UUID) (*Budget, error) {
			return &Budget{ID: id, TenantID: tenantID, Status: "submitted"}, nil
		},
	})

	err := svc.SubmitBudget(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBudgetLocked)
}

func TestApproveBudget_LockedStatus_Rejected(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getBudgetFn: func(_ context.Context, tenantID, id uuid.UUID) (*Budget, error) {
			return &Budget{ID: id, TenantID: tenantID, Status: "locked"}, nil
		},
	})

	err := svc.ApproveBudget(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAlreadyApproved)
}

func TestGetLineItem_Success(t *testing.T) {
	t.Parallel()
	lineID := uuid.New()
	svc := NewService(&mockRepo{
		getLineItemFn: func(_ context.Context, id uuid.UUID) (*BudgetLineItem, error) {
			return &BudgetLineItem{ID: id, CategoryID: "above_the_line"}, nil
		},
	})

	li, err := svc.GetLineItem(context.Background(), lineID)
	require.NoError(t, err)
	assert.Equal(t, lineID, li.ID)
}

func TestListLineItems_Success(t *testing.T) {
	t.Parallel()
	budgetID := uuid.New()
	svc := NewService(&mockRepo{
		listLineItemsFn: func(_ context.Context, bid uuid.UUID) ([]BudgetLineItem, error) {
			return []BudgetLineItem{
				{ID: uuid.New(), BudgetID: bid, CategoryID: "above_the_line"},
				{ID: uuid.New(), BudgetID: bid, CategoryID: "below_the_line"},
			}, nil
		},
	})

	items, err := svc.ListLineItems(context.Background(), budgetID)
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

func TestUpdateLineItemActuals_Success(t *testing.T) {
	t.Parallel()
	var capturedActual, capturedCommitted decimal.Decimal
	lineID := uuid.New()
	svc := NewService(&mockRepo{
		updateLineItemActualsFn: func(_ context.Context, id uuid.UUID, actual, committed decimal.Decimal) error {
			capturedActual = actual
			capturedCommitted = committed
			return nil
		},
	})

	err := svc.UpdateLineItemActuals(context.Background(), lineID, decimal.NewFromInt(100000), decimal.NewFromInt(50000))
	require.NoError(t, err)
	assert.True(t, decimal.NewFromInt(100000).Equal(capturedActual))
	assert.True(t, decimal.NewFromInt(50000).Equal(capturedCommitted))
}

func TestCreateBudget_ExplicitStatus(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})
	b := &Budget{TenantID: uuid.New(), Label: "Test Budget", Status: "submitted", CreatedBy: uuid.New()}
	err := svc.CreateBudget(context.Background(), b)
	require.NoError(t, err)
	// Pre-set status should be preserved
	assert.Equal(t, "submitted", b.Status)
}

func TestCreateBudgetFromTemplate_LineItemError(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		createLineItemFn: func(_ context.Context, _ *BudgetLineItem) error {
			return fmt.Errorf("db error on line item")
		},
	})

	b := &Budget{TenantID: uuid.New(), Label: "Test", CreatedBy: uuid.New()}
	err := svc.CreateBudgetFromTemplate(ctxWithVertical(), b, "standard_feature")
	require.Error(t, err)
}
