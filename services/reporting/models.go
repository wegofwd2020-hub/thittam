// Package reporting implements the reporting-analytics service.
package reporting

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ExpenseFact represents a denormalized expense record for reporting.
type ExpenseFact struct {
	ExpenseID    uuid.UUID       `json:"expense_id"`
	ProductionID uuid.UUID       `json:"production_id"`
	TenantID     uuid.UUID       `json:"tenant_id"`
	BudgetLineID *uuid.UUID      `json:"budget_line_id,omitempty"`
	CategoryID   string          `json:"category_id"`
	Amount       decimal.Decimal `json:"amount"`
	ApprovedAt   *time.Time      `json:"approved_at,omitempty"`
}

// BudgetFact represents a denormalized budget summary for reporting.
type BudgetFact struct {
	BudgetID       uuid.UUID       `json:"budget_id"`
	ProductionID   uuid.UUID       `json:"production_id"`
	TenantID       uuid.UUID       `json:"tenant_id"`
	TotalBudgeted  decimal.Decimal `json:"total_budgeted"`
	TotalActual    decimal.Decimal `json:"total_actual"`
	TotalCommitted decimal.Decimal `json:"total_committed"`
}

// DashboardSummary represents a high-level dashboard for a tenant.
type DashboardSummary struct {
	TenantID       uuid.UUID       `json:"tenant_id"`
	ProjectLabel   string          `json:"project_label"`   // from entity labels
	ProjectCount   int             `json:"project_count"`
	TotalBudgeted  decimal.Decimal `json:"total_budgeted"`
	TotalActual    decimal.Decimal `json:"total_actual"`
	TotalCommitted decimal.Decimal `json:"total_committed"`
	Variance       decimal.Decimal `json:"variance"`
}
