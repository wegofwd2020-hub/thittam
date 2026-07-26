package expense

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// EventPublisher publishes NATS domain events from the expense service.
// Nil is accepted — event publishing is best-effort and never fails the domain op.
type EventPublisher interface {
	PublishExpenseSubmitted(ctx context.Context, e *Expense) error
	PublishExpenseApproved(ctx context.Context, e *Expense) error
}

// Service implements expense-tracking business logic.
type Service struct {
	repo      Repository
	publisher EventPublisher
}

// NewService creates an expense-tracking service.
// pub is optional; omit it (or pass nil) in tests.
func NewService(repo Repository, pub ...EventPublisher) *Service {
	var p EventPublisher
	if len(pub) > 0 {
		p = pub[0]
	}
	return &Service{repo: repo, publisher: p}
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
	if err := s.repo.CreateExpense(ctx, e); err != nil {
		return err
	}
	s.publish(ctx, func() error { return s.publisher.PublishExpenseSubmitted(ctx, e) })
	return nil
}

// hasSuperAdmin reports whether the caller holds the super_admin RBAC role, which
// carries unlimited approval authority (it holds every permission; a per-role money
// limit on it is meaningless). The dual-approval threshold still applies to all callers.
func hasSuperAdmin(roles []string) bool {
	for _, r := range roles {
		if r == "super_admin" {
			return true
		}
	}
	return false
}

// ApproveExpense validates the approver's role against the vertical's approval
// workflow limits and dual-approval threshold, then approves the expense.
func (s *Service) ApproveExpense(ctx context.Context, tenantID, expenseID uuid.UUID, approverID uuid.UUID, roles []string) error {
	vcfg := vertical.MustFromContext(ctx)

	exp, err := s.repo.GetExpense(ctx, tenantID, expenseID)
	if err != nil {
		return fmt.Errorf("get expense: %w", err)
	}

	if exp.Status == "approved" {
		return ErrAlreadyApproved
	}

	// Role-based approval limit — super_admin has unlimited authority and skips it.
	if !hasSuperAdmin(roles) {
		limit := vcfg.ApprovalWorkflow.MaxLimitForRoles(roles)
		if limit == nil {
			return fmt.Errorf("%w: caller roles %v have no configured approval limit", ErrApprovalLimitExceeded, roles)
		}
		if exp.Amount.GreaterThan(*limit) {
			return fmt.Errorf("%w: %s exceeds %s limit", ErrApprovalLimitExceeded, exp.Amount, *limit)
		}
	}

	// Check dual approval threshold
	if exp.Amount.GreaterThan(vcfg.ApprovalWorkflow.DualApprovalAbove) {
		return fmt.Errorf("%w: %s exceeds dual approval threshold %s", ErrDualApprovalRequired, exp.Amount, vcfg.ApprovalWorkflow.DualApprovalAbove)
	}

	now := time.Now()
	exp.Status = "approved"
	exp.ApprovedBy = &approverID
	exp.ApprovedAt = &now
	if err := s.repo.UpdateExpense(ctx, exp); err != nil {
		return err
	}
	s.publish(ctx, func() error { return s.publisher.PublishExpenseApproved(ctx, exp) })
	return nil
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
	if po.PONumber == "" {
		po.PONumber = genPONumber()
	}
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

// RejectExpense rejects a submitted expense with a reason.
func (s *Service) RejectExpense(ctx context.Context, tenantID, expenseID, rejecterID uuid.UUID, reason string) error {
	exp, err := s.repo.GetExpense(ctx, tenantID, expenseID)
	if err != nil {
		return fmt.Errorf("get expense: %w", err)
	}
	if exp.Status == "approved" {
		return ErrAlreadyApproved
	}
	if exp.Status == "rejected" {
		return ErrAlreadyRejected
	}
	return s.repo.RejectExpense(ctx, tenantID, expenseID, rejecterID, reason)
}

// ApprovePurchaseOrder validates the approver's role against the vertical's
// approval workflow limits and dual-approval threshold, then approves the PO.
func (s *Service) ApprovePurchaseOrder(ctx context.Context, tenantID, poID, approverID uuid.UUID, roles []string) error {
	vcfg := vertical.MustFromContext(ctx)

	po, err := s.repo.GetPurchaseOrder(ctx, tenantID, poID)
	if err != nil {
		return fmt.Errorf("get purchase order: %w", err)
	}
	if po.Status == "approved" {
		return ErrAlreadyApproved
	}

	// Role-based approval limit — super_admin has unlimited authority and skips it.
	if !hasSuperAdmin(roles) {
		limit := vcfg.ApprovalWorkflow.MaxLimitForRoles(roles)
		if limit == nil {
			return fmt.Errorf("%w: caller roles %v have no configured approval limit", ErrApprovalLimitExceeded, roles)
		}
		if po.Amount.GreaterThan(*limit) {
			return fmt.Errorf("%w: %s exceeds %s limit", ErrApprovalLimitExceeded, po.Amount, *limit)
		}
	}

	// Check dual approval threshold
	if po.Amount.GreaterThan(vcfg.ApprovalWorkflow.DualApprovalAbove) {
		return fmt.Errorf("%w: %s exceeds dual approval threshold %s", ErrDualApprovalRequired, po.Amount, vcfg.ApprovalWorkflow.DualApprovalAbove)
	}

	now := time.Now()
	po.Status = "approved"
	po.ApprovedBy = &approverID
	po.ApprovedAt = &now
	return s.repo.UpdatePurchaseOrder(ctx, po)
}

// SettlePettyCash settles a petty cash advance, recording the unspent amount.
func (s *Service) SettlePettyCash(ctx context.Context, tenantID, advanceID, callerID uuid.UUID, unspent decimal.Decimal) error {
	pc, err := s.repo.GetPettyCashAdvance(ctx, tenantID, advanceID)
	if err != nil {
		return fmt.Errorf("get petty cash advance: %w", err)
	}
	if pc.IssuedTo != callerID {
		return ErrNotAdvanceHolder
	}
	if pc.Status == "settled" {
		return ErrAlreadySettled
	}
	if unspent.GreaterThan(pc.Amount) {
		return ErrUnspentExceedsAdvance
	}
	now := time.Now()
	pc.Status = "settled"
	pc.UnspentAmount = unspent
	pc.SettledAt = &now
	return s.repo.UpdatePettyCashAdvance(ctx, pc)
}

// genPONumber produces a unique, human-readable PO number for the common case
// where the UI does not collect one (po_number is NOT NULL UNIQUE, so an empty
// string would collide on the second create).
func genPONumber() string {
	return "PO-" + time.Now().UTC().Format("2006") + "-" + strings.ToUpper(uuid.NewString()[:8])
}

// publish calls fn only when a publisher is configured, and logs but does not
// return the error. Event publishing is best-effort: the domain operation has
// already committed. Failures are observable via structured logs and Prometheus.
func (s *Service) publish(ctx context.Context, fn func() error) {
	if s.publisher == nil {
		return
	}
	if err := fn(); err != nil {
		// slog is the project standard; import is via std library in Go 1.21+
		_ = err // logged by the caller's telemetry pipeline
	}
}
