package budget

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Repository defines data access for the budget-planning service.
type Repository interface {
	// Budgets
	CreateBudget(ctx context.Context, b *Budget) error
	GetBudget(ctx context.Context, tenantID, id uuid.UUID) (*Budget, error)
	ListBudgets(ctx context.Context, tenantID, productionID uuid.UUID, status string, limit, offset int) ([]Budget, error)
	UpdateBudgetStatus(ctx context.Context, id uuid.UUID, status string, approvedBy *uuid.UUID) error

	// Line Items
	CreateLineItem(ctx context.Context, li *BudgetLineItem) error
	GetLineItem(ctx context.Context, tenantID, id uuid.UUID) (*BudgetLineItem, error)
	ListLineItems(ctx context.Context, tenantID, budgetID uuid.UUID, limit, offset int) ([]BudgetLineItem, error)
	UpdateLineItemActuals(ctx context.Context, tenantID, id uuid.UUID, actualAmount, committedAmount decimal.Decimal) error
	CheckLineAvailability(ctx context.Context, tenantID, id uuid.UUID) (decimal.Decimal, error)
}
