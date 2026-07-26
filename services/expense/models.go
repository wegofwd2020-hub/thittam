// Package expense implements the expense-tracking service.
package expense

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// PurchaseOrder represents a purchase order raised against a vendor.
type PurchaseOrder struct {
	ID           uuid.UUID       `json:"id"`
	ProductionID uuid.UUID       `json:"production_id"`
	TenantID     uuid.UUID       `json:"tenant_id"`
	BudgetLineID *uuid.UUID      `json:"budget_line_id,omitempty"`
	PONumber     string          `json:"po_number"`
	VendorName   string          `json:"vendor_name"`
	VendorGSTIN  string          `json:"vendor_gstin,omitempty"`
	Description  string          `json:"description,omitempty"`
	Amount       decimal.Decimal `json:"amount"`
	Currency     string          `json:"currency"`
	Status       string          `json:"status"` // draft, approved, partially_invoiced, closed, cancelled
	RaisedBy     uuid.UUID       `json:"raised_by"`
	ApprovedBy   *uuid.UUID      `json:"approved_by,omitempty"`
	RaisedAt     time.Time       `json:"raised_at"`
	ApprovedAt   *time.Time      `json:"approved_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

// Expense represents a submitted expense claim.
type Expense struct {
	ID              uuid.UUID       `json:"id"`
	ProductionID    uuid.UUID       `json:"production_id"`
	TenantID        uuid.UUID       `json:"tenant_id"`
	BudgetLineID    *uuid.UUID      `json:"budget_line_id,omitempty"`
	PurchaseOrderID *uuid.UUID      `json:"purchase_order_id,omitempty"`
	CategoryID      string          `json:"category_id"`
	Description     string          `json:"description,omitempty"`
	Amount          decimal.Decimal `json:"amount"`
	Currency        string          `json:"currency"`
	TaxAmount       decimal.Decimal `json:"tax_amount"`
	Status          string          `json:"status"` // draft, submitted, approved, rejected, paid
	SubmittedBy     uuid.UUID       `json:"submitted_by"`
	ApprovedBy      *uuid.UUID      `json:"approved_by,omitempty"`
	RejectedBy      *uuid.UUID      `json:"rejected_by,omitempty"`
	SubmittedAt     *time.Time      `json:"submitted_at,omitempty"`
	ApprovedAt      *time.Time      `json:"approved_at,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	RejectionReason *string         `json:"rejection_reason,omitempty"`
	RejectedAt      *time.Time      `json:"rejected_at,omitempty"`
}

// PettyCashAdvance represents a petty cash advance issued to a team member.
type PettyCashAdvance struct {
	ID            uuid.UUID       `json:"id"`
	ProductionID  uuid.UUID       `json:"production_id"`
	TenantID      uuid.UUID       `json:"tenant_id"`
	IssuedTo      uuid.UUID       `json:"issued_to"`
	Amount        decimal.Decimal `json:"amount"`
	Purpose       string          `json:"purpose"`
	Status        string          `json:"status"` // issued, partially_settled, settled
	IssuedAt      time.Time       `json:"issued_at"`
	SettledAt     *time.Time      `json:"settled_at,omitempty"`
	UnspentAmount decimal.Decimal `json:"unspent_amount"`
}
