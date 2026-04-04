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

	// Dashboard
	GetDashboardSummary(ctx context.Context, tenantID uuid.UUID) (*DashboardSummary, error)
}
