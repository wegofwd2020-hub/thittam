package reporting

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// GetPortfolioOverview returns all active projects with budget health indicators.
// Enriches project labels from the tenant's vertical config.
func (s *Service) GetPortfolioOverview(ctx context.Context, tenantID uuid.UUID) (*PortfolioOverview, error) {
	vcfg := vertical.MustFromContext(ctx)

	overview, err := s.repo.GetPortfolioOverview(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get portfolio: %w", err)
	}

	// Enrich with vertical labels
	overview.ProjectLabel = vcfg.EntityLabels.ProjectPlural

	// Compute health counts from projects
	overview.HealthCounts = computeHealthCounts(overview.Projects)

	return overview, nil
}

// GetFinancialSummary returns aggregated financial data across all projects.
// Category names are enriched from the vertical's budget categories.
func (s *Service) GetFinancialSummary(ctx context.Context, tenantID uuid.UUID) (*FinancialSummary, error) {
	vcfg := vertical.MustFromContext(ctx)

	summary, err := s.repo.GetFinancialSummary(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get financial summary: %w", err)
	}

	// Enrich category names from vertical config
	for i := range summary.ByCategory {
		cat := vcfg.FindBudgetCategory(summary.ByCategory[i].CategoryID)
		if cat != nil {
			summary.ByCategory[i].CategoryName = cat.Label
		}
	}

	// Compute available = budgeted - actual - committed
	summary.TotalAvailable = summary.TotalBudgeted.Sub(summary.TotalActual).Sub(summary.TotalCommitted)

	return summary, nil
}

// GetApprovalPipeline returns all pending approvals with age breakdown.
func (s *Service) GetApprovalPipeline(ctx context.Context, tenantID uuid.UUID) (*ApprovalPipeline, error) {
	pipeline, err := s.repo.GetApprovalPipeline(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get approval pipeline: %w", err)
	}

	// Compute age bands from pending items
	now := time.Now()
	for i := range pipeline.PendingItems {
		pipeline.PendingItems[i].AgeHours = int(now.Sub(pipeline.PendingItems[i].SubmittedAt).Hours())
	}
	pipeline.ByAge = computeAgeBands(pipeline.PendingItems)

	return pipeline, nil
}

// GetTeamUtilization returns crew/team allocation across projects.
// Enriches team member label from the vertical config.
func (s *Service) GetTeamUtilization(ctx context.Context, tenantID uuid.UUID) (*TeamUtilization, error) {
	vcfg := vertical.MustFromContext(ctx)

	util, err := s.repo.GetTeamUtilization(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get team utilization: %w", err)
	}

	// Enrich with vertical labels
	util.TeamMemberLabel = vcfg.EntityLabels.TeamMemberPlural

	// Compute utilization percentage
	if util.TotalMembers > 0 {
		util.UtilizationPct = decimal.NewFromInt(int64(util.Assigned)).
			Div(decimal.NewFromInt(int64(util.TotalMembers))).
			Mul(decimal.NewFromInt(100)).
			Round(1)
	}

	return util, nil
}

// GetComplianceStatus returns compliance flags and overdue approvals.
func (s *Service) GetComplianceStatus(ctx context.Context, tenantID uuid.UUID) (*ComplianceStatus, error) {
	status, err := s.repo.GetComplianceStatus(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get compliance status: %w", err)
	}
	return status, nil
}

// computeHealthCounts categorises projects by budget health.
func computeHealthCounts(projects []ProjectSummary) BudgetHealthCount {
	var counts BudgetHealthCount
	for _, p := range projects {
		switch p.Health {
		case "on_track":
			counts.OnTrack++
		case "at_risk":
			counts.AtRisk++
		case "over_budget":
			counts.OverBudget++
		}
	}
	return counts
}

// computeAgeBands categorises pending items by age.
func computeAgeBands(items []PendingApproval) ApprovalAgeBands {
	var bands ApprovalAgeBands
	for _, item := range items {
		switch {
		case item.AgeHours < 24:
			bands.Under24h++
		case item.AgeHours < 72:
			bands.OneToThreeDays++
		case item.AgeHours < 168:
			bands.ThreeToSevenDays++
		default:
			bands.OverSevenDays++
		}
	}
	return bands
}
