package reporting

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

// --- Mock repository ---

type mockRepo struct {
	getExpenseFactsFn      func(ctx context.Context, tenantID, productionID uuid.UUID) ([]ExpenseFact, error)
	getBudgetFactsFn       func(ctx context.Context, tenantID, productionID uuid.UUID) ([]BudgetFact, error)
	getDashboardSummaryFn  func(ctx context.Context, tenantID uuid.UUID) (*DashboardSummary, error)
	getPortfolioOverviewFn func(ctx context.Context, tenantID uuid.UUID) (*PortfolioOverview, error)
	getFinancialSummaryFn  func(ctx context.Context, tenantID uuid.UUID) (*FinancialSummary, error)
	getApprovalPipelineFn  func(ctx context.Context, tenantID uuid.UUID) (*ApprovalPipeline, error)
	getTeamUtilizationFn   func(ctx context.Context, tenantID uuid.UUID) (*TeamUtilization, error)
	getComplianceStatusFn  func(ctx context.Context, tenantID uuid.UUID) (*ComplianceStatus, error)
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
	if m.getPortfolioOverviewFn != nil {
		return m.getPortfolioOverviewFn(ctx, tenantID)
	}
	return &PortfolioOverview{TenantID: tenantID}, nil
}
func (m *mockRepo) GetFinancialSummary(ctx context.Context, tenantID uuid.UUID) (*FinancialSummary, error) {
	if m.getFinancialSummaryFn != nil {
		return m.getFinancialSummaryFn(ctx, tenantID)
	}
	return &FinancialSummary{TenantID: tenantID}, nil
}
func (m *mockRepo) GetApprovalPipeline(ctx context.Context, tenantID uuid.UUID) (*ApprovalPipeline, error) {
	if m.getApprovalPipelineFn != nil {
		return m.getApprovalPipelineFn(ctx, tenantID)
	}
	return &ApprovalPipeline{TenantID: tenantID}, nil
}
func (m *mockRepo) GetTeamUtilization(ctx context.Context, tenantID uuid.UUID) (*TeamUtilization, error) {
	if m.getTeamUtilizationFn != nil {
		return m.getTeamUtilizationFn(ctx, tenantID)
	}
	return &TeamUtilization{TenantID: tenantID}, nil
}
func (m *mockRepo) GetComplianceStatus(ctx context.Context, tenantID uuid.UUID) (*ComplianceStatus, error) {
	if m.getComplianceStatusFn != nil {
		return m.getComplianceStatusFn(ctx, tenantID)
	}
	return &ComplianceStatus{TenantID: tenantID}, nil
}
func (m *mockRepo) GetDashboardSummary(ctx context.Context, tenantID uuid.UUID) (*DashboardSummary, error) {
	if m.getDashboardSummaryFn != nil {
		return m.getDashboardSummaryFn(ctx, tenantID)
	}
	return &DashboardSummary{
		TenantID:       tenantID,
		ProjectCount:   5,
		TotalBudgeted:  decimal.NewFromInt(1000000),
		TotalActual:    decimal.NewFromInt(750000),
		TotalCommitted: decimal.NewFromInt(100000),
		Variance:       decimal.NewFromInt(150000),
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
				{Role: "coordinator", MaxAmount: decimal.NewFromInt(200000)},
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

// --- Additional coverage tests ---

func TestGetExpenseFacts_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	prodID := uuid.New()
	svc := NewService(&mockRepo{
		getExpenseFactsFn: func(_ context.Context, tid, pid uuid.UUID) ([]ExpenseFact, error) {
			return []ExpenseFact{
				{TenantID: tid, ProductionID: pid, CategoryID: "location_rental"},
			}, nil
		},
	})

	facts, err := svc.GetExpenseFacts(context.Background(), tenantID, prodID)
	require.NoError(t, err)
	assert.Len(t, facts, 1)
	assert.Equal(t, "location_rental", facts[0].CategoryID)
}

func TestGetBudgetFacts_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	prodID := uuid.New()
	svc := NewService(&mockRepo{
		getBudgetFactsFn: func(_ context.Context, tid, pid uuid.UUID) ([]BudgetFact, error) {
			return []BudgetFact{
				{TenantID: tid, ProductionID: pid, TotalBudgeted: decimal.NewFromInt(500000)},
			}, nil
		},
	})

	facts, err := svc.GetBudgetFacts(context.Background(), tenantID, prodID)
	require.NoError(t, err)
	assert.Len(t, facts, 1)
	assert.True(t, decimal.NewFromInt(500000).Equal(facts[0].TotalBudgeted))
}

func TestGetDashboardSummary_RepoError(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getDashboardSummaryFn: func(_ context.Context, _ uuid.UUID) (*DashboardSummary, error) {
			return nil, fmt.Errorf("db error")
		},
	})

	_, err := svc.GetDashboardSummary(ctxWithVertical(), uuid.New())
	require.Error(t, err)
}

// --- Dashboard service tests ---

func TestGetPortfolioOverview_EnrichesLabel(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	svc := NewService(&mockRepo{})

	overview, err := svc.GetPortfolioOverview(ctxWithVertical(), tenantID)
	require.NoError(t, err)
	assert.Equal(t, "Productions", overview.ProjectLabel)
}

func TestGetPortfolioOverview_ComputesHealthCounts(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	svc := NewService(&mockRepo{
		getPortfolioOverviewFn: func(_ context.Context, tid uuid.UUID) (*PortfolioOverview, error) {
			return &PortfolioOverview{
				TenantID: tid,
				Projects: []ProjectSummary{
					{Health: "on_track"},
					{Health: "on_track"},
					{Health: "at_risk"},
					{Health: "over_budget"},
				},
			}, nil
		},
	})

	overview, err := svc.GetPortfolioOverview(ctxWithVertical(), tenantID)
	require.NoError(t, err)
	assert.Equal(t, 2, overview.HealthCounts.OnTrack)
	assert.Equal(t, 1, overview.HealthCounts.AtRisk)
	assert.Equal(t, 1, overview.HealthCounts.OverBudget)
}

func TestGetFinancialSummary_ComputesAvailable(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	svc := NewService(&mockRepo{
		getFinancialSummaryFn: func(_ context.Context, tid uuid.UUID) (*FinancialSummary, error) {
			return &FinancialSummary{
				TenantID:       tid,
				TotalBudgeted:  decimal.NewFromInt(1000000),
				TotalActual:    decimal.NewFromInt(600000),
				TotalCommitted: decimal.NewFromInt(150000),
			}, nil
		},
	})

	summary, err := svc.GetFinancialSummary(ctxWithVertical(), tenantID)
	require.NoError(t, err)
	assert.True(t, decimal.NewFromInt(250000).Equal(summary.TotalAvailable))
}

func TestGetApprovalPipeline_ComputesAgeBands(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	now := time.Now()
	svc := NewService(&mockRepo{
		getApprovalPipelineFn: func(_ context.Context, tid uuid.UUID) (*ApprovalPipeline, error) {
			return &ApprovalPipeline{
				TenantID: tid,
				PendingItems: []PendingApproval{
					{SubmittedAt: now.Add(-12 * time.Hour)},  // under 24h
					{SubmittedAt: now.Add(-48 * time.Hour)},  // 1-3 days
					{SubmittedAt: now.Add(-96 * time.Hour)},  // 3-7 days
					{SubmittedAt: now.Add(-200 * time.Hour)}, // over 7 days
				},
			}, nil
		},
	})

	pipeline, err := svc.GetApprovalPipeline(context.Background(), tenantID)
	require.NoError(t, err)
	assert.Equal(t, 1, pipeline.ByAge.Under24h)
	assert.Equal(t, 1, pipeline.ByAge.OneToThreeDays)
	assert.Equal(t, 1, pipeline.ByAge.ThreeToSevenDays)
	assert.Equal(t, 1, pipeline.ByAge.OverSevenDays)
}

func TestGetTeamUtilization_ComputesPct(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	svc := NewService(&mockRepo{
		getTeamUtilizationFn: func(_ context.Context, tid uuid.UUID) (*TeamUtilization, error) {
			return &TeamUtilization{
				TenantID:     tid,
				TotalMembers: 10,
				Assigned:     8,
			}, nil
		},
	})

	util, err := svc.GetTeamUtilization(ctxWithVertical(), tenantID)
	require.NoError(t, err)
	assert.Equal(t, "Crew Members", util.TeamMemberLabel)
	assert.True(t, decimal.NewFromFloat(80).Equal(util.UtilizationPct))
}

func TestGetComplianceStatus_Success(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{})

	status, err := svc.GetComplianceStatus(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.NotNil(t, status)
}
