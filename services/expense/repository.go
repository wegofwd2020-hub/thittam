package expense

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines data access for the expense-tracking service.
type Repository interface {
	// Purchase Orders
	CreatePurchaseOrder(ctx context.Context, po *PurchaseOrder) error
	GetPurchaseOrder(ctx context.Context, tenantID, id uuid.UUID) (*PurchaseOrder, error)
	ListPurchaseOrders(ctx context.Context, tenantID, productionID uuid.UUID, status string, limit, offset int) ([]PurchaseOrder, error)
	UpdatePurchaseOrder(ctx context.Context, po *PurchaseOrder) error

	// Expenses
	CreateExpense(ctx context.Context, e *Expense) error
	GetExpense(ctx context.Context, tenantID, id uuid.UUID) (*Expense, error)
	ListExpenses(ctx context.Context, tenantID, productionID uuid.UUID, status string, limit, offset int) ([]Expense, error)
	UpdateExpense(ctx context.Context, e *Expense) error
	RejectExpense(ctx context.Context, tenantID, expenseID, rejecterID uuid.UUID, reason string) error

	// Petty Cash
	CreatePettyCashAdvance(ctx context.Context, pc *PettyCashAdvance) error
	GetPettyCashAdvance(ctx context.Context, tenantID, id uuid.UUID) (*PettyCashAdvance, error)
	ListPettyCashAdvances(ctx context.Context, tenantID, productionID uuid.UUID, status string, limit, offset int) ([]PettyCashAdvance, error)
	UpdatePettyCashAdvance(ctx context.Context, pc *PettyCashAdvance) error
}
