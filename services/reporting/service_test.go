package reporting

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
	getExpenseFactsFn     func(ctx context.Context, tenantID, productionID uuid.UUID) ([]ExpenseFact, error)
	getBudgetFactsFn      func(ctx context.Context, tenantID, productionID uuid.UUID) ([]BudgetFact, error)
	getDashboardSummaryFn func(ctx context.Context, tenantID uuid.UUID) (*DashboardSummary, error)
}

func (m *mockRepo) GetExpenseFacts(ctx context.Context, tenantID, productionID uuid.UUID) ([]ExpenseFact, error) {
	if m.getExpenseFactsFn != nil {
		return m.getExpenseFactsFn(ctx, tenantID, productionID)
	}
	return nil, nil
}
func (m *mockRepo) GetBudgetFacts(ctx context.Context, tenantID, productionID uuid.UUID) ([]BudgetFact, error) {
	if m.getBudgetFactsFn != nil {
		return m.getBudgetFactsFn(ctx, tenantID, productionID)
	}
	return nil, nil
}
func (m *mockRepo) GetPortfolioOverview(ctx context.Context, tenantID uuid.UUID) (*PortfolioOverview, error) {
	return &PortfolioOverview{TenantID: tenantID}, nil
}
func (m *mockRepo) GetFinancialSummary(ctx context.Context, tenantID uuid.UUID) (*FinancialSummary, error) {
	return &FinancialSummary{TenantID: tenantID}, nil
}
func (m *mockRepo) GetApprovalPipeline(ctx context.Context, tenantID uuid.UUID) (*ApprovalPipeline, error) {
	return &ApprovalPipeline{TenantID: tenantID}, nil
}
func (m *mockRepo) GetTeamUtilization(ctx context.Context, tenantID uuid.UUID) (*TeamUtilization, error) {
	return &TeamUtilization{TenantID: tenantID}, nil
}
func (m *mockRepo) GetComplianceStatus(ctx context.Context, tenantID uuid.UUID) (*ComplianceStatus, error) {
	return &ComplianceStatus{TenantID: tenantID}, nil
}
func (m *mockRepo) GetDashboardSummary(ctx context.Context, tenantID uuid.UUID) (*DashboardSummary, error) {
	if m.getDashboardSummaryFn != nil {
		return m.getDashboardSummaryFn(ctx, tenantID)
	}
	return &DashboardSummary{
		TenantID:      tenantID,
		ProjectCount:  5,
		TotalBudgeted: decimal.NewFromInt(1000000),
		TotalActual:   decimal.NewFromInt(750000),
		TotalCommitted: decimal.NewFromInt(100000),
		Variance:      decimal.NewFromInt(150000),
	}, nil
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
		},
		BudgetCategories: []vertical.BudgetCategory{
			{ID: "above_the_line", Label: "Above the Line", DefaultAccountCode: "5100"},
		},
		ExpenseCategories: []vertical.ExpenseCategory{
			{ID: "location_rental", Label: "Location Rental", TaxTreatment: "input_gst", DefaultAccountCode: "5200", RequiresPO: true},
		},
		ApprovalWorkflow: vertical.ApprovalWorkflow{
			Limits: []vertical.ApprovalLimit{
				{Role: "line_producer", MaxAmount: decimal.NewFromInt(200000)},
			},
			DualApprovalAbove: decimal.NewFromInt(1000000),
		},
		InventoryCategories: []vertical.InventoryCategory{
			{ID: "camera", Label: "Camera Equipment", IsTrackable: true},
		},
		ReportDefinitions: []vertical.ReportDefinition{
			{ID: "cost_report", Name: "Daily Cost Report"},
			{ID: "variance_report", Name: "Budget Variance Report"},
		},
	}
}

func ctxWithVertical() context.Context {
	return vertical.WithConfig(context.Background(), movieProductionConfig())
}

// --- Tests ---

func TestGetReportDefinition_ValidID(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	rd, err := svc.GetReportDefinition(ctxWithVertical(), "cost_report")
	require.NoError(t, err)
	assert.Equal(t, "cost_report", rd.ID)
	assert.Equal(t, "Daily Cost Report", rd.Name)
}

func TestGetReportDefinition_InvalidID(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	_, err := svc.GetReportDefinition(ctxWithVertical(), "nonexistent_report")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrReportNotFound)
}

func TestGetReportDefinition_EmptyID(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	_, err := svc.GetReportDefinition(ctxWithVertical(), "")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidReportID)
}

func TestListReportDefinitions(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	defs := svc.ListReportDefinitions(ctxWithVertical())
	assert.Len(t, defs, 2)
	assert.Equal(t, "cost_report", defs[0].ID)
	assert.Equal(t, "variance_report", defs[1].ID)
}

func TestGetDashboardSummary_EntityLabels(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()

	svc := NewService(&mockRepo{})

	summary, err := svc.GetDashboardSummary(ctxWithVertical(), tenantID)
	require.NoError(t, err)
	assert.Equal(t, "Productions", summary.ProjectLabel)
	assert.Equal(t, 5, summary.ProjectCount)
	assert.True(t, summary.TotalBudgeted.Equal(decimal.NewFromInt(1000000)))
}
