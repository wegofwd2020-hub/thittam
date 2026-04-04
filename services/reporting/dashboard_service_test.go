package reporting

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Extended mock for dashboard methods ---

type dashboardMockRepo struct {
	mockRepo // embed existing mock
	getPortfolioFn  func(ctx context.Context, tenantID uuid.UUID) (*PortfolioOverview, error)
	getFinancialFn  func(ctx context.Context, tenantID uuid.UUID) (*FinancialSummary, error)
	getApprovalFn   func(ctx context.Context, tenantID uuid.UUID) (*ApprovalPipeline, error)
	getTeamFn       func(ctx context.Context, tenantID uuid.UUID) (*TeamUtilization, error)
	getComplianceFn func(ctx context.Context, tenantID uuid.UUID) (*ComplianceStatus, error)
}

func (m *dashboardMockRepo) GetPortfolioOverview(ctx context.Context, tenantID uuid.UUID) (*PortfolioOverview, error) {
	if m.getPortfolioFn != nil {
		return m.getPortfolioFn(ctx, tenantID)
	}
	return &PortfolioOverview{
		TenantID:    tenantID,
		TotalActive: 3,
		TotalBudget: decimal.NewFromInt(85000000),
		TotalActual: decimal.NewFromInt(45000000),
		Projects: []ProjectSummary{
			{ID: uuid.New(), Title: "The Last Horizon", Status: "production", BudgetTotal: decimal.NewFromInt(85000000), ActualSpend: decimal.NewFromInt(45000000), Health: "on_track"},
			{ID: uuid.New(), Title: "Midnight Express", Status: "post_production", BudgetTotal: decimal.NewFromInt(120000000), ActualSpend: decimal.NewFromInt(110000000), Health: "at_risk"},
			{ID: uuid.New(), Title: "Project Starfall", Status: "development", BudgetTotal: decimal.NewFromInt(40000000), ActualSpend: decimal.NewFromInt(50000000), Health: "over_budget"},
		},
	}, nil
}

func (m *dashboardMockRepo) GetFinancialSummary(ctx context.Context, tenantID uuid.UUID) (*FinancialSummary, error) {
	if m.getFinancialFn != nil {
		return m.getFinancialFn(ctx, tenantID)
	}
	return &FinancialSummary{
		TenantID:       tenantID,
		TotalBudgeted:  decimal.NewFromInt(245000000),
		TotalActual:    decimal.NewFromInt(205000000),
		TotalCommitted: decimal.NewFromInt(15000000),
		ByCategory: []CategorySpend{
			{CategoryID: "above_the_line", Budgeted: decimal.NewFromInt(50000000), Actual: decimal.NewFromInt(50000000)},
			{CategoryID: "below_the_line", Budgeted: decimal.NewFromInt(80000000), Actual: decimal.NewFromInt(75000000)},
		},
		MonthlyTrend: []MonthlySpend{
			{Month: "2026-01", Actual: decimal.NewFromInt(30000000), Budget: decimal.NewFromInt(35000000)},
			{Month: "2026-02", Actual: decimal.NewFromInt(45000000), Budget: decimal.NewFromInt(40000000)},
		},
	}, nil
}

func (m *dashboardMockRepo) GetApprovalPipeline(ctx context.Context, tenantID uuid.UUID) (*ApprovalPipeline, error) {
	if m.getApprovalFn != nil {
		return m.getApprovalFn(ctx, tenantID)
	}
	now := time.Now()
	return &ApprovalPipeline{
		TenantID:     tenantID,
		TotalPending: 4,
		TotalAmount:  decimal.NewFromInt(850000),
		PendingItems: []PendingApproval{
			{ID: uuid.New(), Type: "expense", Description: "Location rental", Amount: decimal.NewFromInt(150000), SubmittedAt: now.Add(-2 * time.Hour)},
			{ID: uuid.New(), Type: "expense", Description: "Equipment", Amount: decimal.NewFromInt(300000), SubmittedAt: now.Add(-48 * time.Hour)},
			{ID: uuid.New(), Type: "budget", Description: "Budget V2", Amount: decimal.NewFromInt(200000), SubmittedAt: now.Add(-5 * 24 * time.Hour)},
			{ID: uuid.New(), Type: "expense", Description: "Catering", Amount: decimal.NewFromInt(200000), SubmittedAt: now.Add(-10 * 24 * time.Hour)},
		},
	}, nil
}

func (m *dashboardMockRepo) GetTeamUtilization(ctx context.Context, tenantID uuid.UUID) (*TeamUtilization, error) {
	if m.getTeamFn != nil {
		return m.getTeamFn(ctx, tenantID)
	}
	return &TeamUtilization{
		TenantID:     tenantID,
		TotalMembers: 40,
		Assigned:     32,
		Available:    8,
		ByProject: []ProjectTeam{
			{ProjectID: uuid.New(), ProjectTitle: "The Last Horizon", MemberCount: 25},
			{ProjectID: uuid.New(), ProjectTitle: "Midnight Express", MemberCount: 7},
		},
	}, nil
}

func (m *dashboardMockRepo) GetComplianceStatus(ctx context.Context, tenantID uuid.UUID) (*ComplianceStatus, error) {
	if m.getComplianceFn != nil {
		return m.getComplianceFn(ctx, tenantID)
	}
	return &ComplianceStatus{
		TenantID:         tenantID,
		OverdueApprovals: 2,
		PolicyViolations: 1,
		AuditFlags:       0,
		RecentViolations: []ComplianceItem{
			{ID: uuid.New(), Type: "overdue_approval", Description: "Expense pending > 7 days", Severity: "warning"},
			{ID: uuid.New(), Type: "missing_po", Description: "Expense without required PO", Severity: "critical"},
		},
	}, nil
}

// --- Tests ---

func TestGetPortfolioOverview(t *testing.T) {
	t.Parallel()
	svc := NewService(&dashboardMockRepo{})
	tenantID := uuid.New()

	overview, err := svc.GetPortfolioOverview(ctxWithVertical(), tenantID)
	require.NoError(t, err)

	assert.Equal(t, "Productions", overview.ProjectLabel) // from vertical
	assert.Equal(t, 3, overview.TotalActive)
	assert.Len(t, overview.Projects, 3)

	// Health counts computed from projects
	assert.Equal(t, 1, overview.HealthCounts.OnTrack)
	assert.Equal(t, 1, overview.HealthCounts.AtRisk)
	assert.Equal(t, 1, overview.HealthCounts.OverBudget)
}

func TestGetFinancialSummary_EnrichesCategoryNames(t *testing.T) {
	t.Parallel()
	svc := NewService(&dashboardMockRepo{})
	tenantID := uuid.New()

	summary, err := svc.GetFinancialSummary(ctxWithVertical(), tenantID)
	require.NoError(t, err)

	// Category name enriched from vertical config
	assert.Equal(t, "Above the Line", summary.ByCategory[0].CategoryName)

	// Available computed
	expected := decimal.NewFromInt(245000000).Sub(decimal.NewFromInt(205000000)).Sub(decimal.NewFromInt(15000000))
	assert.True(t, summary.TotalAvailable.Equal(expected))

	assert.Len(t, summary.MonthlyTrend, 2)
}

func TestGetApprovalPipeline_AgeBands(t *testing.T) {
	t.Parallel()
	svc := NewService(&dashboardMockRepo{})
	tenantID := uuid.New()

	pipeline, err := svc.GetApprovalPipeline(ctxWithVertical(), tenantID)
	require.NoError(t, err)

	assert.Equal(t, 4, pipeline.TotalPending)
	assert.Len(t, pipeline.PendingItems, 4)

	// Verify age bands
	assert.Equal(t, 1, pipeline.ByAge.Under24h)       // 2h old
	assert.Equal(t, 1, pipeline.ByAge.OneToThreeDays)  // 48h old
	assert.Equal(t, 1, pipeline.ByAge.ThreeToSevenDays) // 5 days old
	assert.Equal(t, 1, pipeline.ByAge.OverSevenDays)    // 10 days old

	// Age hours populated
	for _, item := range pipeline.PendingItems {
		assert.Greater(t, item.AgeHours, 0)
	}
}

func TestGetTeamUtilization_VerticalLabels(t *testing.T) {
	t.Parallel()
	svc := NewService(&dashboardMockRepo{})
	tenantID := uuid.New()

	util, err := svc.GetTeamUtilization(ctxWithVertical(), tenantID)
	require.NoError(t, err)

	assert.Equal(t, "Crew Members", util.TeamMemberLabel) // from vertical
	assert.Equal(t, 40, util.TotalMembers)
	assert.Equal(t, 32, util.Assigned)
	assert.Equal(t, 8, util.Available)

	// 32/40 = 80%
	assert.True(t, util.UtilizationPct.Equal(decimal.NewFromFloat(80.0)))

	assert.Len(t, util.ByProject, 2)
}

func TestGetTeamUtilization_ZeroMembers(t *testing.T) {
	t.Parallel()
	svc := NewService(&dashboardMockRepo{
		getTeamFn: func(ctx context.Context, tenantID uuid.UUID) (*TeamUtilization, error) {
			return &TeamUtilization{TenantID: tenantID, TotalMembers: 0, Assigned: 0, Available: 0}, nil
		},
	})

	util, err := svc.GetTeamUtilization(ctxWithVertical(), uuid.New())
	require.NoError(t, err)
	assert.True(t, util.UtilizationPct.IsZero()) // no division by zero
}

func TestGetComplianceStatus(t *testing.T) {
	t.Parallel()
	svc := NewService(&dashboardMockRepo{})
	tenantID := uuid.New()

	status, err := svc.GetComplianceStatus(ctxWithVertical(), tenantID)
	require.NoError(t, err)

	assert.Equal(t, 2, status.OverdueApprovals)
	assert.Equal(t, 1, status.PolicyViolations)
	assert.Len(t, status.RecentViolations, 2)
	assert.Equal(t, "critical", status.RecentViolations[1].Severity)
}

func TestComputeHealthCounts(t *testing.T) {
	t.Parallel()
	projects := []ProjectSummary{
		{Health: "on_track"}, {Health: "on_track"}, {Health: "on_track"},
		{Health: "at_risk"}, {Health: "at_risk"},
		{Health: "over_budget"},
	}
	counts := computeHealthCounts(projects)
	assert.Equal(t, 3, counts.OnTrack)
	assert.Equal(t, 2, counts.AtRisk)
	assert.Equal(t, 1, counts.OverBudget)
}

func TestComputeAgeBands(t *testing.T) {
	t.Parallel()
	items := []PendingApproval{
		{AgeHours: 5},    // under 24h
		{AgeHours: 23},   // under 24h
		{AgeHours: 50},   // 1-3 days
		{AgeHours: 100},  // 3-7 days
		{AgeHours: 200},  // over 7 days
	}
	bands := computeAgeBands(items)
	assert.Equal(t, 2, bands.Under24h)
	assert.Equal(t, 1, bands.OneToThreeDays)
	assert.Equal(t, 1, bands.ThreeToSevenDays)
	assert.Equal(t, 1, bands.OverSevenDays)
}
