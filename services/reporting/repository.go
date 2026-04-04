package reporting

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines data access for the reporting-analytics service.
type Repository interface {
	// Expense facts
	GetExpenseFacts(ctx context.Context, tenantID, productionID uuid.UUID) ([]ExpenseFact, error)

	// Budget facts
	GetBudgetFacts(ctx context.Context, tenantID, productionID uuid.UUID) ([]BudgetFact, error)

	// Basic dashboard
	GetDashboardSummary(ctx context.Context, tenantID uuid.UUID) (*DashboardSummary, error)

	// Executive dashboard
	GetPortfolioOverview(ctx context.Context, tenantID uuid.UUID) (*PortfolioOverview, error)
	GetFinancialSummary(ctx context.Context, tenantID uuid.UUID) (*FinancialSummary, error)
	GetApprovalPipeline(ctx context.Context, tenantID uuid.UUID) (*ApprovalPipeline, error)
	GetTeamUtilization(ctx context.Context, tenantID uuid.UUID) (*TeamUtilization, error)
	GetComplianceStatus(ctx context.Context, tenantID uuid.UUID) (*ComplianceStatus, error)
}
