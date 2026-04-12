-- Billing service queries.
-- subscriptions, invoices, payments, plan_limits live in the shared (public) schema
-- since billing is cross-tenant (the billing service manages tenant-level data).

-- name: CreateSubscription :one
INSERT INTO subscriptions (id, tenant_id, plan, status, billing_cycle, current_period_start, current_period_end)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (tenant_id) DO NOTHING
RETURNING *;

-- name: GetSubscription :one
SELECT * FROM subscriptions WHERE tenant_id = $1;

-- name: UpdateSubscriptionStatus :one
UPDATE subscriptions SET status = $2, updated_at = now()
WHERE tenant_id = $1
RETURNING *;

-- name: UpdateSubscriptionPlan :one
UPDATE subscriptions
SET plan                  = $2,
    current_period_start  = $3,
    current_period_end    = $4,
    updated_at            = now()
WHERE tenant_id = $1
RETURNING *;

-- name: CreateInvoice :one
INSERT INTO invoices (id, tenant_id, subscription_id, invoice_number, amount, tax_amount, currency, status, due_date, period_start, period_end)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT (id) DO NOTHING
RETURNING *;

-- name: GetInvoice :one
SELECT * FROM invoices WHERE id = $1 AND tenant_id = $2;

-- name: ListInvoices :many
SELECT * FROM invoices
WHERE tenant_id = $1 AND ($2 = '' OR status = $2)
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: UpdateInvoiceStatus :one
UPDATE invoices SET status = $3, paid_at = CASE WHEN $3 = 'paid' THEN now() ELSE paid_at END
WHERE id = $1 AND tenant_id = $2
RETURNING *;

-- name: RecordDunningAttempt :one
INSERT INTO dunning_attempts (id, invoice_id, attempt_number, outcome, gateway_response)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO NOTHING
RETURNING *;

-- name: GetDunningAttemptCount :one
SELECT COUNT(*) FROM dunning_attempts WHERE invoice_id = $1 AND outcome = 'failed';
