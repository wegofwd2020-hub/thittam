-- General ledger service queries.

-- name: CreateAccount :one
INSERT INTO accounts (id, tenant_id, code, name, account_type, parent_id, is_active)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (tenant_id, code) DO NOTHING
RETURNING *;

-- name: GetAccount :one
SELECT * FROM accounts WHERE id = $1 AND tenant_id = $2;

-- name: GetAccountByCode :one
SELECT * FROM accounts WHERE code = $1 AND tenant_id = $2;

-- name: ListAccounts :many
SELECT * FROM accounts
WHERE tenant_id = $1 AND ($2 = '' OR account_type = $2) AND is_active = true
ORDER BY code ASC
LIMIT $3 OFFSET $4;

-- name: CreateAccountingPeriod :one
INSERT INTO accounting_periods (id, tenant_id, year, month, status)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO NOTHING
RETURNING *;

-- name: GetOpenPeriod :one
SELECT * FROM accounting_periods
WHERE tenant_id = $1 AND status = 'open'
ORDER BY year DESC, month DESC
LIMIT 1;

-- name: GetAccountingPeriod :one
SELECT * FROM accounting_periods WHERE id = $1 AND tenant_id = $2;

-- name: CloseAccountingPeriod :one
UPDATE accounting_periods SET status = 'closed', closed_at = now(), closed_by = $3
WHERE id = $1 AND tenant_id = $2
RETURNING *;

-- name: NextJournalEntrySeq :one
INSERT INTO journal_entry_counters (tenant_id, year, next_seq)
VALUES ($1, $2, 1)
ON CONFLICT (tenant_id, year) DO UPDATE
    SET next_seq = journal_entry_counters.next_seq + 1
RETURNING next_seq;

-- name: CreateJournalEntry :one
INSERT INTO journal_entries (id, tenant_id, production_id, period_id, entry_number, reference, narration, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (id) DO NOTHING
RETURNING *;

-- name: GetJournalEntry :one
SELECT * FROM journal_entries WHERE id = $1 AND tenant_id = $2;

-- name: PostJournalEntry :one
UPDATE journal_entries SET status = 'posted', posted_by = $3, posted_at = now()
WHERE id = $1 AND tenant_id = $2 AND status = 'draft'
RETURNING *;

-- name: VoidJournalEntry :one
UPDATE journal_entries SET status = 'void'
WHERE id = $1 AND tenant_id = $2 AND status = 'posted'
RETURNING *;

-- name: ListJournalEntries :many
SELECT * FROM journal_entries
WHERE tenant_id = $1
  AND ($2::uuid IS NULL OR period_id = $2)
  AND ($3 = '' OR status = $3)
ORDER BY created_at DESC
LIMIT $4 OFFSET $5;

-- name: CreateJournalLine :one
INSERT INTO journal_lines (id, journal_id, account_id, debit_amount, credit_amount, description)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (id) DO NOTHING
RETURNING *;

-- name: ListJournalLines :many
SELECT * FROM journal_lines WHERE journal_id = $1 ORDER BY id ASC;

-- name: GetAccountBalance :one
SELECT
    a.id,
    a.code,
    a.name,
    a.account_type,
    COALESCE(SUM(jl.debit_amount),  0) AS total_debits,
    COALESCE(SUM(jl.credit_amount), 0) AS total_credits
FROM accounts a
LEFT JOIN journal_lines jl   ON jl.account_id  = a.id
LEFT JOIN journal_entries je ON je.id = jl.journal_id AND je.status = 'posted'
WHERE a.id = $1 AND a.tenant_id = $2
GROUP BY a.id, a.code, a.name, a.account_type;
