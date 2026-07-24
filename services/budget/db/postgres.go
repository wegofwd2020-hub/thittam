package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/wegofwd2020/thittam/services/budget"
)

// Postgres implements budget.Repository backed by PostgreSQL.
type Postgres struct {
	q *Queries
}

// NewPostgres creates a Postgres-backed budget repository.
func NewPostgres(db *pgxpool.Pool) *Postgres {
	return &Postgres{q: New(db)}
}

// Compile-time interface check.
var _ budget.Repository = (*Postgres)(nil)

// --- Budgets ---

func (p *Postgres) CreateBudget(ctx context.Context, b *budget.Budget) error {
	_, err := p.q.CreateBudget(ctx, CreateBudgetParams{
		ID:           b.ID,
		ProductionID: b.ProductionID,
		TenantID:     b.TenantID,
		Label:        b.Label,
		Status:       b.Status,
		Currency:     b.Currency,
		CreatedBy:    b.CreatedBy,
	})
	if err != nil {
		return fmt.Errorf("budget: create budget: %w", err)
	}
	return nil
}

func (p *Postgres) GetBudget(ctx context.Context, tenantID, id uuid.UUID) (*budget.Budget, error) {
	row, err := p.q.GetBudget(ctx, GetBudgetParams{ID: id, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, budget.ErrBudgetNotFound
		}
		return nil, fmt.Errorf("budget: get budget: %w", err)
	}
	return budgetFromDB(row), nil
}

func (p *Postgres) ListBudgets(ctx context.Context, tenantID, productionID uuid.UUID, status string, limit, offset int) ([]budget.Budget, error) {
	rows, err := p.q.ListBudgets(ctx, ListBudgetsParams{
		TenantID: tenantID,
		Column2:  productionID,
		Limit:    int32(limit),
		Offset:   int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("budget: list budgets: %w", err)
	}
	result := make([]budget.Budget, len(rows))
	for i, row := range rows {
		result[i] = *budgetFromDB(row)
	}
	return result, nil
}

func (p *Postgres) UpdateBudgetStatus(ctx context.Context, tenantID, id uuid.UUID, status string, approvedBy *uuid.UUID) error {
	// The SQL uses the same $4 parameter for both submitted_by and approved_by
	// (CASE WHEN $3='submitted' THEN $4 ELSE ... END).
	var actor pgtype.UUID
	if approvedBy != nil {
		actor = pgtype.UUID{Bytes: *approvedBy, Valid: true}
	}

	_, err := p.q.UpdateBudgetStatus(ctx, UpdateBudgetStatusParams{
		ID:          id,
		TenantID:    tenantID,
		Status:      status,
		SubmittedBy: actor,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return budget.ErrBudgetNotFound
		}
		return fmt.Errorf("budget: update budget status: %w", err)
	}
	return nil
}

// --- Line Items ---

func (p *Postgres) CreateLineItem(ctx context.Context, li *budget.BudgetLineItem) error {
	_, err := p.q.CreateLineItem(ctx, CreateLineItemParams{
		ID:             li.ID,
		BudgetID:       li.BudgetID,
		TenantID:       li.TenantID,
		CategoryID:     li.CategoryID,
		Description:    li.Description,
		AccountCode:    li.AccountCode,
		BudgetedAmount: pgNumericFromDecimal(li.BudgetedAmount),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("budget: line item already exists: %w", err)
		}
		return fmt.Errorf("budget: create line item: %w", err)
	}

	// Keep budget total_amount in sync after every new line item.
	if err := p.q.UpdateBudgetTotals(ctx, UpdateBudgetTotalsParams{BudgetID: li.BudgetID, TenantID: li.TenantID}); err != nil {
		return fmt.Errorf("budget: update budget totals: %w", err)
	}
	return nil
}

func (p *Postgres) GetLineItem(ctx context.Context, tenantID, id uuid.UUID) (*budget.BudgetLineItem, error) {
	row, err := p.q.GetLineItem(ctx, GetLineItemParams{ID: id, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, budget.ErrLineItemNotFound
		}
		return nil, fmt.Errorf("budget: get line item: %w", err)
	}
	return lineItemFromDB(row), nil
}

func (p *Postgres) ListLineItems(ctx context.Context, tenantID, budgetID uuid.UUID, limit, offset int) ([]budget.BudgetLineItem, error) {
	rows, err := p.q.ListLineItems(ctx, ListLineItemsParams{
		BudgetID: budgetID,
		TenantID: tenantID,
		Limit:    int32(limit),
		Offset:   int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("budget: list line items: %w", err)
	}
	result := make([]budget.BudgetLineItem, len(rows))
	for i, row := range rows {
		result[i] = *lineItemFromDB(row)
	}
	return result, nil
}

func (p *Postgres) UpdateLineItemActuals(ctx context.Context, tenantID, id uuid.UUID, actualAmount, committedAmount decimal.Decimal) error {
	li, err := p.q.UpdateLineItemAmounts(ctx, UpdateLineItemAmountsParams{
		ID:              id,
		TenantID:        tenantID,
		BudgetedAmount:  pgtype.Numeric{Valid: false}, // COALESCE(NULL, budgeted_amount) — keep existing
		ActualAmount:    pgNumericFromDecimal(actualAmount),
		CommittedAmount: pgNumericFromDecimal(committedAmount),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return budget.ErrLineItemNotFound
		}
		return fmt.Errorf("budget: update line item actuals: %w", err)
	}

	// Keep budget total in sync.
	if err := p.q.UpdateBudgetTotals(ctx, UpdateBudgetTotalsParams{BudgetID: li.BudgetID, TenantID: tenantID}); err != nil {
		return fmt.Errorf("budget: update budget totals after actuals: %w", err)
	}
	return nil
}

// CheckLineAvailability returns budgeted - actual - committed for a line item.
func (p *Postgres) CheckLineAvailability(ctx context.Context, tenantID, id uuid.UUID) (decimal.Decimal, error) {
	li, err := p.GetLineItem(ctx, tenantID, id)
	if err != nil {
		return decimal.Zero, err
	}
	available := li.BudgetedAmount.Sub(li.ActualAmount).Sub(li.CommittedAmount)
	return available, nil
}

// --- model conversion helpers ---

func budgetFromDB(row Budget) *budget.Budget {
	b := &budget.Budget{
		ID:           row.ID,
		ProductionID: row.ProductionID,
		TenantID:     row.TenantID,
		Label:        row.Label,
		Status:       row.Status,
		Currency:     row.Currency,
		TotalAmount:  decimalFromPgNumeric(row.TotalAmount),
		CreatedBy:    row.CreatedBy,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
	if row.SubmittedBy.Valid {
		id := uuid.UUID(row.SubmittedBy.Bytes)
		b.SubmittedBy = &id
	}
	if row.ApprovedBy.Valid {
		id := uuid.UUID(row.ApprovedBy.Bytes)
		b.ApprovedBy = &id
	}
	if row.SubmittedAt.Valid {
		t := row.SubmittedAt.Time
		b.SubmittedAt = &t
	}
	if row.ApprovedAt.Valid {
		t := row.ApprovedAt.Time
		b.ApprovedAt = &t
	}
	return b
}

func lineItemFromDB(row BudgetLineItem) *budget.BudgetLineItem {
	return &budget.BudgetLineItem{
		ID:              row.ID,
		BudgetID:        row.BudgetID,
		TenantID:        row.TenantID,
		CategoryID:      row.CategoryID,
		Description:     row.Description,
		AccountCode:     row.AccountCode,
		BudgetedAmount:  decimalFromPgNumeric(row.BudgetedAmount),
		ActualAmount:    decimalFromPgNumeric(row.ActualAmount),
		CommittedAmount: decimalFromPgNumeric(row.CommittedAmount),
		IsLocked:        row.IsLocked,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

// --- pgtype conversion helpers ---

// pgNumericFromDecimal converts a decimal.Decimal to pgtype.Numeric via string.
func pgNumericFromDecimal(d decimal.Decimal) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(d.String())
	return n
}

// decimalFromPgNumeric converts a pgtype.Numeric to decimal.Decimal.
// Returns decimal.Zero for NULL / NaN values.
func decimalFromPgNumeric(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.NaN || n.Int == nil {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}

// isUniqueViolation returns true when err is a PostgreSQL unique-constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
