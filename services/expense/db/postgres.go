package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/wegofwd2020/thittam/services/expense"
)

// Postgres implements expense.Repository backed by PostgreSQL.
type Postgres struct {
	q  *Queries
	db *pgxpool.Pool
}

// NewPostgres creates a Postgres-backed expense repository.
func NewPostgres(db *pgxpool.Pool) *Postgres {
	return &Postgres{q: New(db), db: db}
}

// Compile-time interface check.
var _ expense.Repository = (*Postgres)(nil)

// --- Purchase Orders ---

func (p *Postgres) CreatePurchaseOrder(ctx context.Context, po *expense.PurchaseOrder) error {
	_, err := p.q.CreatePurchaseOrder(ctx, CreatePurchaseOrderParams{
		ID:           po.ID,
		ProductionID: po.ProductionID,
		TenantID:     po.TenantID,
		BudgetLineID: pgUUIDFromPtr(po.BudgetLineID),
		PoNumber:     po.PONumber,
		VendorName:   po.VendorName,
		VendorGstin:  pgTextFromString(po.VendorGSTIN),
		Description:  pgTextFromString(po.Description),
		Amount:       pgNumericFromDecimal(po.Amount),
		Currency:     po.Currency,
		RaisedBy:     po.RaisedBy,
	})
	if err != nil {
		return fmt.Errorf("expense: create purchase order: %w", err)
	}
	return nil
}

func (p *Postgres) GetPurchaseOrder(ctx context.Context, tenantID, id uuid.UUID) (*expense.PurchaseOrder, error) {
	row, err := p.q.GetPurchaseOrder(ctx, GetPurchaseOrderParams{ID: id, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, expense.ErrPONotFound
		}
		return nil, fmt.Errorf("expense: get purchase order: %w", err)
	}
	return purchaseOrderFromDB(row), nil
}

// ListPurchaseOrders lists POs for a tenant/production. The status parameter is
// not filterable in the current SQL query and is silently ignored.
func (p *Postgres) ListPurchaseOrders(ctx context.Context, tenantID, productionID uuid.UUID, _ string, limit, offset int) ([]expense.PurchaseOrder, error) {
	rows, err := p.q.ListPurchaseOrders(ctx, ListPurchaseOrdersParams{
		TenantID: tenantID,
		Column2:  productionID,
		Limit:    int32(limit),
		Offset:   int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("expense: list purchase orders: %w", err)
	}
	result := make([]expense.PurchaseOrder, len(rows))
	for i, row := range rows {
		result[i] = *purchaseOrderFromDB(row)
	}
	return result, nil
}

func (p *Postgres) UpdatePurchaseOrder(ctx context.Context, po *expense.PurchaseOrder) error {
	var approvedBy pgtype.UUID
	if po.ApprovedBy != nil {
		approvedBy = pgtype.UUID{Bytes: *po.ApprovedBy, Valid: true}
	}
	_, err := p.q.UpdatePurchaseOrderStatus(ctx, UpdatePurchaseOrderStatusParams{
		ID:         po.ID,
		TenantID:   po.TenantID,
		Status:     po.Status,
		ApprovedBy: approvedBy,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return expense.ErrPONotFound
		}
		return fmt.Errorf("expense: update purchase order: %w", err)
	}
	return nil
}

// --- Expenses ---

func (p *Postgres) CreateExpense(ctx context.Context, e *expense.Expense) error {
	_, err := p.q.CreateExpense(ctx, CreateExpenseParams{
		ID:              e.ID,
		ProductionID:    e.ProductionID,
		TenantID:        e.TenantID,
		BudgetLineID:    pgUUIDFromPtr(e.BudgetLineID),
		PurchaseOrderID: pgUUIDFromPtr(e.PurchaseOrderID),
		CategoryID:      e.CategoryID,
		Description:     pgTextFromString(e.Description),
		Amount:          pgNumericFromDecimal(e.Amount),
		Currency:        e.Currency,
		TaxAmount:       pgNumericFromDecimal(e.TaxAmount),
		SubmittedBy:     e.SubmittedBy,
	})
	if err != nil {
		return fmt.Errorf("expense: create expense: %w", err)
	}
	return nil
}

func (p *Postgres) GetExpense(ctx context.Context, tenantID, id uuid.UUID) (*expense.Expense, error) {
	row, err := p.q.GetExpense(ctx, GetExpenseParams{ID: id, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, expense.ErrExpenseNotFound
		}
		return nil, fmt.Errorf("expense: get expense: %w", err)
	}
	return expenseFromDB(row), nil
}

func (p *Postgres) ListExpenses(ctx context.Context, tenantID, productionID uuid.UUID, status string, limit, offset int, submittedBy uuid.UUID) ([]expense.Expense, error) {
	rows, err := p.q.ListExpenses(ctx, ListExpensesParams{
		TenantID:     tenantID,
		ProductionID: nullableUUID(productionID),
		SubmittedBy:  nullableUUID(submittedBy),
		Status:       status,
		Limit:        int32(limit),
		Offset:       int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("expense: list expenses: %w", err)
	}
	result := make([]expense.Expense, len(rows))
	for i, row := range rows {
		result[i] = *expenseFromDB(row)
	}
	return result, nil
}

func (p *Postgres) UpdateExpense(ctx context.Context, e *expense.Expense) error {
	var approvedBy pgtype.UUID
	if e.ApprovedBy != nil {
		approvedBy = pgtype.UUID{Bytes: *e.ApprovedBy, Valid: true}
	}
	_, err := p.q.UpdateExpenseStatus(ctx, UpdateExpenseStatusParams{
		ID:         e.ID,
		TenantID:   e.TenantID,
		Status:     e.Status,
		ApprovedBy: approvedBy,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return expense.ErrExpenseNotFound
		}
		return fmt.Errorf("expense: update expense: %w", err)
	}
	return nil
}

func (p *Postgres) RejectExpense(ctx context.Context, tenantID, expenseID, rejecterID uuid.UUID, reason string) error {
	_, err := p.q.RejectExpense(ctx, RejectExpenseParams{
		ID:              expenseID,
		TenantID:        tenantID,
		RejectionReason: pgtype.Text{String: reason, Valid: true},
		RejectedBy:      pgtype.UUID{Bytes: rejecterID, Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return expense.ErrExpenseNotFound
		}
		return fmt.Errorf("expense/db: reject expense: %w", err)
	}
	return nil
}

// --- Petty Cash ---

func (p *Postgres) CreatePettyCashAdvance(ctx context.Context, pc *expense.PettyCashAdvance) error {
	_, err := p.q.CreatePettyCashAdvance(ctx, CreatePettyCashAdvanceParams{
		ID:           pc.ID,
		ProductionID: pc.ProductionID,
		TenantID:     pc.TenantID,
		IssuedTo:     pc.IssuedTo,
		Amount:       pgNumericFromDecimal(pc.Amount),
		Purpose:      pc.Purpose,
	})
	if err != nil {
		return fmt.Errorf("expense: create petty cash advance: %w", err)
	}
	return nil
}

func (p *Postgres) GetPettyCashAdvance(ctx context.Context, tenantID, id uuid.UUID) (*expense.PettyCashAdvance, error) {
	row, err := p.q.GetPettyCashAdvance(ctx, GetPettyCashAdvanceParams{ID: id, TenantID: tenantID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, expense.ErrExpenseNotFound
		}
		return nil, fmt.Errorf("expense: get petty cash advance: %w", err)
	}
	return pettyCashFromDB(row), nil
}

// ListPettyCashAdvances lists advances for a tenant/production. The status
// parameter is not filterable in the current SQL query and is silently ignored.
func (p *Postgres) ListPettyCashAdvances(ctx context.Context, tenantID, productionID uuid.UUID, _ string, limit, offset int) ([]expense.PettyCashAdvance, error) {
	rows, err := p.q.ListPettyCashAdvances(ctx, ListPettyCashAdvancesParams{
		TenantID: tenantID,
		Column2:  productionID,
		Limit:    int32(limit),
		Offset:   int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("expense: list petty cash advances: %w", err)
	}
	result := make([]expense.PettyCashAdvance, len(rows))
	for i, row := range rows {
		result[i] = *pettyCashFromDB(row)
	}
	return result, nil
}

func (p *Postgres) UpdatePettyCashAdvance(ctx context.Context, pc *expense.PettyCashAdvance) error {
	_, err := p.q.SettlePettyCashAdvance(ctx, SettlePettyCashAdvanceParams{
		ID:            pc.ID,
		TenantID:      pc.TenantID,
		Status:        pc.Status,
		UnspentAmount: pgNumericFromDecimal(pc.UnspentAmount),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return expense.ErrExpenseNotFound
		}
		return fmt.Errorf("expense: update petty cash advance: %w", err)
	}
	return nil
}

// --- model conversion helpers ---

func purchaseOrderFromDB(row PurchaseOrder) *expense.PurchaseOrder {
	po := &expense.PurchaseOrder{
		ID:          row.ID,
		ProductionID: row.ProductionID,
		TenantID:    row.TenantID,
		PONumber:    row.PoNumber,
		VendorName:  row.VendorName,
		VendorGSTIN: stringFromPgText(row.VendorGstin),
		Description: stringFromPgText(row.Description),
		Amount:      decimalFromPgNumeric(row.Amount),
		Currency:    row.Currency,
		Status:      row.Status,
		RaisedBy:    row.RaisedBy,
		RaisedAt:    row.RaisedAt,
		CreatedAt:   row.CreatedAt,
	}
	if row.BudgetLineID.Valid {
		id := uuid.UUID(row.BudgetLineID.Bytes)
		po.BudgetLineID = &id
	}
	if row.ApprovedBy.Valid {
		id := uuid.UUID(row.ApprovedBy.Bytes)
		po.ApprovedBy = &id
	}
	if row.ApprovedAt.Valid {
		t := row.ApprovedAt.Time
		po.ApprovedAt = &t
	}
	return po
}

func expenseFromDB(row Expense) *expense.Expense {
	e := &expense.Expense{
		ID:           row.ID,
		ProductionID: row.ProductionID,
		TenantID:     row.TenantID,
		CategoryID:   row.CategoryID,
		Description:  stringFromPgText(row.Description),
		Amount:       decimalFromPgNumeric(row.Amount),
		Currency:     row.Currency,
		TaxAmount:    decimalFromPgNumeric(row.TaxAmount),
		Status:       row.Status,
		SubmittedBy:  row.SubmittedBy,
		CreatedAt:    row.CreatedAt,
	}
	if row.BudgetLineID.Valid {
		id := uuid.UUID(row.BudgetLineID.Bytes)
		e.BudgetLineID = &id
	}
	if row.PurchaseOrderID.Valid {
		id := uuid.UUID(row.PurchaseOrderID.Bytes)
		e.PurchaseOrderID = &id
	}
	if row.ApprovedBy.Valid {
		id := uuid.UUID(row.ApprovedBy.Bytes)
		e.ApprovedBy = &id
	}
	if row.SubmittedAt.Valid {
		t := row.SubmittedAt.Time
		e.SubmittedAt = &t
	}
	if row.ApprovedAt.Valid {
		t := row.ApprovedAt.Time
		e.ApprovedAt = &t
	}
	if row.RejectedBy.Valid {
		id := uuid.UUID(row.RejectedBy.Bytes)
		e.RejectedBy = &id
	}
	if row.RejectionReason.Valid {
		s := row.RejectionReason.String
		e.RejectionReason = &s
	}
	if row.RejectedAt.Valid {
		t := row.RejectedAt.Time
		e.RejectedAt = &t
	}
	return e
}

func pettyCashFromDB(row PettyCashAdvance) *expense.PettyCashAdvance {
	pc := &expense.PettyCashAdvance{
		ID:            row.ID,
		ProductionID:  row.ProductionID,
		TenantID:      row.TenantID,
		IssuedTo:      row.IssuedTo,
		Amount:        decimalFromPgNumeric(row.Amount),
		Purpose:       row.Purpose,
		Status:        row.Status,
		IssuedAt:      row.IssuedAt,
		UnspentAmount: decimalFromPgNumeric(row.UnspentAmount),
	}
	if row.SettledAt.Valid {
		t := row.SettledAt.Time
		pc.SettledAt = &t
	}
	return pc
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

// pgUUIDFromPtr converts an optional uuid.UUID pointer to pgtype.UUID.
func pgUUIDFromPtr(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

// nullableUUID maps the zero UUID to SQL NULL (an "omit this filter" sentinel).
func nullableUUID(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}

// pgTextFromString converts a Go string to pgtype.Text (NULL when empty).
func pgTextFromString(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

// stringFromPgText converts a pgtype.Text to a Go string (empty for NULL).
func stringFromPgText(t pgtype.Text) string {
	if t.Valid {
		return t.String
	}
	return ""
}
