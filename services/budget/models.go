// Package budget implements the budget-planning service.
package budget

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Budget represents a budget version for a production.
type Budget struct {
	ID           uuid.UUID       `json:"id"`
	ProductionID uuid.UUID       `json:"production_id"`
	TenantID     uuid.UUID       `json:"tenant_id"`
	Label        string          `json:"label"`
	Status       string          `json:"status"` // draft, submitted, approved, locked
	Currency     string          `json:"currency"`
	TotalAmount  decimal.Decimal `json:"total_amount"`
	SubmittedBy  *uuid.UUID      `json:"submitted_by,omitempty"`
	ApprovedBy   *uuid.UUID      `json:"approved_by,omitempty"`
	SubmittedAt  *time.Time      `json:"submitted_at,omitempty"`
	ApprovedAt   *time.Time      `json:"approved_at,omitempty"`
	CreatedBy    uuid.UUID       `json:"created_by"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// BudgetLineItem represents a single line item within a budget.
type BudgetLineItem struct {
	ID              uuid.UUID       `json:"id"`
	BudgetID        uuid.UUID       `json:"budget_id"`
	TenantID        uuid.UUID       `json:"tenant_id"`
	CategoryID      string          `json:"category_id"`
	Description     string          `json:"description"`
	AccountCode     string          `json:"account_code"`
	BudgetedAmount  decimal.Decimal `json:"budgeted_amount"`
	ActualAmount    decimal.Decimal `json:"actual_amount"`
	CommittedAmount decimal.Decimal `json:"committed_amount"`
	IsLocked        bool            `json:"is_locked"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}
