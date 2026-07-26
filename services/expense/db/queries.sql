-- Expense tracking service queries.

-- name: CreatePurchaseOrder :one
INSERT INTO purchase_orders (id, production_id, tenant_id, budget_line_id, po_number, vendor_name, vendor_gstin, description, amount, currency, raised_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (id) DO NOTHING
RETURNING *;

-- name: GetPurchaseOrder :one
SELECT * FROM purchase_orders WHERE id = $1 AND tenant_id = $2;

-- name: ListPurchaseOrders :many
SELECT * FROM purchase_orders
WHERE tenant_id = $1 AND ($2::uuid IS NULL OR production_id = $2)
ORDER BY raised_at DESC
LIMIT $3 OFFSET $4;

-- name: UpdatePurchaseOrderStatus :one
UPDATE purchase_orders
SET status      = $3,
    approved_by = CASE WHEN $3 = 'approved' THEN $4 ELSE approved_by END,
    approved_at = CASE WHEN $3 = 'approved' THEN now() ELSE approved_at END
WHERE id = $1 AND tenant_id = $2
RETURNING *;

-- name: CreateExpense :one
INSERT INTO expenses (id, production_id, tenant_id, budget_line_id, purchase_order_id, category_id, description, amount, currency, tax_amount, submitted_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (id) DO NOTHING
RETURNING *;

-- name: GetExpense :one
SELECT * FROM expenses WHERE id = $1 AND tenant_id = $2;

-- name: ListExpenses :many
SELECT * FROM expenses
WHERE tenant_id = $1
  AND ($2::uuid IS NULL OR production_id = $2)
  AND ($3 = '' OR status = $3)
ORDER BY created_at DESC
LIMIT $4 OFFSET $5;

-- name: UpdateExpenseStatus :one
UPDATE expenses
SET status      = $3,
    approved_by = CASE WHEN $3 = 'approved' THEN $4 ELSE approved_by END,
    submitted_at = CASE WHEN $3 = 'submitted' THEN now() ELSE submitted_at END,
    approved_at  = CASE WHEN $3 = 'approved'  THEN now() ELSE approved_at  END
WHERE id = $1 AND tenant_id = $2
RETURNING *;

-- name: CreatePettyCashAdvance :one
INSERT INTO petty_cash_advances (id, production_id, tenant_id, issued_to, amount, purpose)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (id) DO NOTHING
RETURNING *;

-- name: GetPettyCashAdvance :one
SELECT * FROM petty_cash_advances WHERE id = $1 AND tenant_id = $2;

-- name: ListPettyCashAdvances :many
SELECT * FROM petty_cash_advances
WHERE tenant_id = $1 AND ($2::uuid IS NULL OR production_id = $2)
ORDER BY issued_at DESC
LIMIT $3 OFFSET $4;

-- name: SettlePettyCashAdvance :one
UPDATE petty_cash_advances
SET status         = $3,
    unspent_amount = $4,
    settled_at     = CASE WHEN $3 = 'settled' THEN now() ELSE settled_at END
WHERE id = $1 AND tenant_id = $2
RETURNING *;

-- name: RejectExpense :one
UPDATE expenses
SET status = 'rejected', rejection_reason = $3, rejected_at = now(), rejected_by = $4
WHERE id = $1 AND tenant_id = $2
RETURNING *;
