package budget

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// EventPublisher publishes NATS domain events from the budget service.
// Nil is accepted — event publishing is best-effort and never fails the domain op.
type EventPublisher interface {
	PublishBudgetApproved(ctx context.Context, b *Budget) error
}

// Service implements budget-planning business logic.
type Service struct {
	repo      Repository
	publisher EventPublisher
}

// NewService creates a budget-planning service.
// pub is optional; omit it (or pass nil) in tests.
func NewService(repo Repository, pub ...EventPublisher) *Service {
	var p EventPublisher
	if len(pub) > 0 {
		p = pub[0]
	}
	return &Service{repo: repo, publisher: p}
}

// CreateBudget creates a new budget for a production.
func (s *Service) CreateBudget(ctx context.Context, b *Budget) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	if b.Status == "" {
		b.Status = "draft"
	}
	return s.repo.CreateBudget(ctx, b)
}

// GetBudget retrieves a budget by ID.
func (s *Service) GetBudget(ctx context.Context, tenantID, id uuid.UUID) (*Budget, error) {
	return s.repo.GetBudget(ctx, tenantID, id)
}

// ListBudgets lists budgets for a production.
func (s *Service) ListBudgets(ctx context.Context, tenantID, productionID uuid.UUID, status string, limit, offset int) ([]Budget, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.ListBudgets(ctx, tenantID, productionID, status, limit, offset)
}

// SubmitBudget transitions a budget from draft to submitted.
func (s *Service) SubmitBudget(ctx context.Context, tenantID, id uuid.UUID, submittedBy uuid.UUID) error {
	b, err := s.repo.GetBudget(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("get budget: %w", err)
	}
	if b.Status != "draft" {
		return fmt.Errorf("%w: current status is %q, must be draft", ErrBudgetLocked, b.Status)
	}
	return s.repo.UpdateBudgetStatus(ctx, id, "submitted", nil)
}

// ApproveBudget approves a budget. The budget must be in submitted status.
func (s *Service) ApproveBudget(ctx context.Context, tenantID, id uuid.UUID, approvedBy uuid.UUID) error {
	b, err := s.repo.GetBudget(ctx, tenantID, id)
	if err != nil {
		return fmt.Errorf("get budget: %w", err)
	}
	if b.Status == "approved" || b.Status == "locked" {
		return fmt.Errorf("%w: current status is %q", ErrAlreadyApproved, b.Status)
	}
	if b.Status != "submitted" {
		return fmt.Errorf("%w: current status is %q, must be submitted", ErrBudgetNotApproved, b.Status)
	}
	if err := s.repo.UpdateBudgetStatus(ctx, id, "approved", &approvedBy); err != nil {
		return err
	}
	b.Status = "approved"
	b.ApprovedBy = &approvedBy
	s.publish(ctx, func() error { return s.publisher.PublishBudgetApproved(ctx, b) })
	return nil
}

// CreateLineItem creates a line item, validating category_id against the vertical config.
// If account_code is empty, uses the default from the vertical's budget category.
func (s *Service) CreateLineItem(ctx context.Context, li *BudgetLineItem) error {
	vcfg := vertical.MustFromContext(ctx)

	cat := vcfg.FindBudgetCategory(li.CategoryID)
	if cat == nil {
		return fmt.Errorf("%w: %q is not valid for vertical %s (valid: %s)",
			ErrInvalidCategory, li.CategoryID, vcfg.ID, vcfg.BudgetCategoryIDs())
	}

	// Map default account code from vertical config if not provided
	if li.AccountCode == "" {
		li.AccountCode = cat.DefaultAccountCode
	}

	if li.ID == uuid.Nil {
		li.ID = uuid.New()
	}
	return s.repo.CreateLineItem(ctx, li)
}

// GetLineItem retrieves a line item by ID.
func (s *Service) GetLineItem(ctx context.Context, id uuid.UUID) (*BudgetLineItem, error) {
	return s.repo.GetLineItem(ctx, id)
}

// ListLineItems lists line items for a budget.
func (s *Service) ListLineItems(ctx context.Context, budgetID uuid.UUID) ([]BudgetLineItem, error) {
	return s.repo.ListLineItems(ctx, budgetID)
}

// UpdateLineItemActuals updates actual and committed amounts on a line item.
func (s *Service) UpdateLineItemActuals(ctx context.Context, id uuid.UUID, actualAmount, committedAmount decimal.Decimal) error {
	return s.repo.UpdateLineItemActuals(ctx, id, actualAmount, committedAmount)
}

// CheckLineAvailability returns the available amount on a line item
// (budgeted - actual - committed).
func (s *Service) CheckLineAvailability(ctx context.Context, id uuid.UUID) (decimal.Decimal, error) {
	return s.repo.CheckLineAvailability(ctx, id)
}

// CreateBudgetFromTemplate creates a budget pre-populated with line items from a
// vertical-defined template.
func (s *Service) CreateBudgetFromTemplate(ctx context.Context, b *Budget, templateName string) error {
	vcfg := vertical.MustFromContext(ctx)

	tmpl := vcfg.FindBudgetTemplate(templateName)
	if tmpl == nil {
		return fmt.Errorf("budget: template %q not found in vertical %s", templateName, vcfg.ID)
	}

	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	if b.Status == "" {
		b.Status = "draft"
	}
	if err := s.repo.CreateBudget(ctx, b); err != nil {
		return fmt.Errorf("create budget: %w", err)
	}

	for _, tli := range tmpl.LineItems {
		li := &BudgetLineItem{
			ID:             uuid.New(),
			BudgetID:       b.ID,
			TenantID:       b.TenantID,
			CategoryID:     tli.CategoryID,
			Description:    tli.Description,
			AccountCode:    tli.AccountCode,
			BudgetedAmount: tli.DefaultAmount,
		}
		if err := s.repo.CreateLineItem(ctx, li); err != nil {
			return fmt.Errorf("create template line item %q: %w", tli.Description, err)
		}
	}
	return nil
}

// GetBudgetCategories returns the vertical's budget categories for the frontend.
func (s *Service) GetBudgetCategories(ctx context.Context) []vertical.BudgetCategory {
	vcfg := vertical.MustFromContext(ctx)
	return vcfg.BudgetCategories
}

// GetBudgetTemplates returns the vertical's budget templates for the frontend.
func (s *Service) GetBudgetTemplates(ctx context.Context) []vertical.BudgetTemplate {
	vcfg := vertical.MustFromContext(ctx)
	return vcfg.BudgetTemplates
}

// publish calls fn only when a publisher is configured; logs but does not return errors.
func (s *Service) publish(ctx context.Context, fn func() error) {
	if s.publisher == nil {
		return
	}
	_ = fn() // best-effort: failures observable via structured logs / Prometheus
}
