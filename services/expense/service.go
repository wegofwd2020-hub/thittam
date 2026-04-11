package expense

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// Service implements expense-tracking business logic.
type Service struct {
	repo Repository
}

// NewService creates an expense-tracking service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// SubmitExpense validates the expense category against the vertical config,
// enforces PO requirements, sets tax treatment, and persists the expense.
func (s *Service) SubmitExpense(ctx context.Context, e *Expense) error {
	vcfg := vertical.MustFromContext(ctx)

	// Validate category exists in this vertical
	cat := vcfg.FindExpenseCategory(e.CategoryID)
	if cat == nil {
		return fmt.Errorf("%w: %q is not valid for vertical %s", ErrInvalidCategory, e.CategoryID, vcfg.ID)
	}

	// Enforce PO requirement
	if cat.RequiresPO && e.PurchaseOrderID == nil {
		return fmt.Errorf("%w: category %q requires a purchase order", ErrPORequired, e.CategoryID)
	}

	// Set status to submitted
	e.Status = "submitted"

	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return s.repo.CreateExpense(ctx, e)
}

// ApproveExpense validates the approver's role against the vertical's approval
// workflow limits and dual-approval threshold, then approves the expense.
func (s *Service) ApproveExpense(ctx context.Context, tenantID, expenseID uuid.UUID, approverID uuid.UUID, approverRole string) error {
	vcfg := vertical.MustFromContext(ctx)

	exp, err := s.repo.GetExpense(ctx, tenantID, expenseID)
	if err != nil {
		return fmt.Errorf("get expense: %w", err)
	}

	if exp.Status == "approved" {
		return ErrAlreadyApproved
	}

	// Check role-based approval limit
	limit := vcfg.ApprovalWorkflow.LimitForRole(approverRole)
	if limit == nil {
		return fmt.Errorf("%w: role %q has no configured approval limit", ErrApprovalLimitExceeded, approverRole)
	}
	if exp.Amount.GreaterThan(*limit) {
		return fmt.Errorf("%w: %s exceeds %s limit for %q", ErrApprovalLimitExceeded, exp.Amount, *limit, approverRole)
	}

	// Check dual approval threshold
	if exp.Amount.GreaterThan(vcfg.ApprovalWorkflow.DualApprovalAbove) {
		return fmt.Errorf("%w: %s exceeds dual approval threshold %s", ErrDualApprovalRequired, exp.Amount, vcfg.ApprovalWorkflow.DualApprovalAbove)
	}

	now := time.Now()
	exp.Status = "approved"
	exp.ApprovedBy = &approverID
	exp.ApprovedAt = &now
	return s.repo.UpdateExpense(ctx, exp)
}

// GetExpense retrieves an expense by ID.
func (s *Service) GetExpense(ctx context.Context, tenantID, id uuid.UUID) (*Expense, error) {
	return s.repo.GetExpense(ctx, tenantID, id)
}

// ListExpenses lists expenses for a production.
func (s *Service) ListExpenses(ctx context.Context, tenantID, productionID uuid.UUID, status string, limit, offset int) ([]Expense, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.ListExpenses(ctx, tenantID, productionID, status, limit, offset)
}

// CreatePurchaseOrder creates a new purchase order.
func (s *Service) CreatePurchaseOrder(ctx context.Context, po *PurchaseOrder) error {
	if po.ID == uuid.Nil {
		po.ID = uuid.New()
	}
	return s.repo.CreatePurchaseOrder(ctx, po)
}

// GetPurchaseOrder retrieves a purchase order by ID.
func (s *Service) GetPurchaseOrder(ctx context.Context, tenantID, id uuid.UUID) (*PurchaseOrder, error) {
	return s.repo.GetPurchaseOrder(ctx, tenantID, id)
}

// ListPurchaseOrders lists POs for a production.
func (s *Service) ListPurchaseOrders(ctx context.Context, tenantID, productionID uuid.UUID, status string, limit, offset int) ([]PurchaseOrder, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.ListPurchaseOrders(ctx, tenantID, productionID, status, limit, offset)
}

// CreatePettyCashAdvance creates a new petty cash advance.
func (s *Service) CreatePettyCashAdvance(ctx context.Context, pc *PettyCashAdvance) error {
	if pc.ID == uuid.Nil {
		pc.ID = uuid.New()
	}
	return s.repo.CreatePettyCashAdvance(ctx, pc)
}

// GetPettyCashAdvance retrieves a petty cash advance by ID.
func (s *Service) GetPettyCashAdvance(ctx context.Context, tenantID, id uuid.UUID) (*PettyCashAdvance, error) {
	return s.repo.GetPettyCashAdvance(ctx, tenantID, id)
}

// ListPettyCashAdvances lists petty cash advances for a production.
func (s *Service) ListPettyCashAdvances(ctx context.Context, tenantID, productionID uuid.UUID, status string, limit, offset int) ([]PettyCashAdvance, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.ListPettyCashAdvances(ctx, tenantID, productionID, status, limit, offset)
}

// GetExpenseCategories returns the vertical's configured expense categories.
func (s *Service) GetExpenseCategories(ctx context.Context) []vertical.ExpenseCategory {
	vcfg := vertical.MustFromContext(ctx)
	return vcfg.ExpenseCategories
}

// GetApprovalLimits returns the vertical's configured approval workflow.
func (s *Service) GetApprovalLimits(ctx context.Context) vertical.ApprovalWorkflow {
	vcfg := vertical.MustFromContext(ctx)
	return vcfg.ApprovalWorkflow
}
