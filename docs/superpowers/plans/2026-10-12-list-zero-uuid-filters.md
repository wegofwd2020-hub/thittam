# Zero-UUID list filters + GetExpense owner guard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix unfiltered list queries in `budget` (`ListBudgets`) and `ledger` (`ListJournalEntries`) that bind the zero UUID instead of SQL NULL, so an unfiltered list returns nothing. Backport the nil-subject owner guard to `GetExpense`.

**Architecture:** Same fix as #220. Move the nullable filter to `sqlc.narg(...)::uuid` and pass `nullableUUID(...)` from the Postgres wrapper. No interface changes, so no implementer outside the two services changes. `GetExpense` gets the same `caller.UserID == uuid.Nil ||` guard that the #220 Get handlers have.

**Tech Stack:** Go 1.26, sqlc v1.26.0, pgx v5, testify, `pkg/testdb` for real-Postgres integration tests.

**Spec:** none. The defect is documented in the design doc for #220 (Sibling scan section, PR #230) and in the PR #231 body.

## Global Constraints

- Monetary values stay NUMERIC / decimal. No float64.
- All SQL parameterized through sqlc. No string interpolation.
- No migration, no permission change, no interface change.
- `general-ledger` is a universal service (CLAUDE.md). Its PR needs a senior reviewer.
- Commits are local only until the PR is opened. No push without asking.
- Commit trailer: `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>`

## Review Focus

1. An unfiltered list (nil production or nil period) returns every tenant row. Test: Task 1 and Task 2 integration.
2. A period filter must match only that period's entries and not widen to others. Test: Task 2 integration.
3. A nil caller subject never owns a GetExpense record, even when `SubmittedBy` is also zero. Test: Task 3 unit, `TestHandler_GetExpense_NilSubjectNeverOwns`.
4. The status filter in ledger still composes with the period filter. Test: Task 2 integration.

---

### Task 1: budget `ListBudgets` nil production filter

**Files:**
- Modify: `services/budget/db/queries.sql:13-16`
- Regenerate: `services/budget/db/queries.sql.go`
- Modify: `services/budget/db/postgres.go:58-72`
- Create: `services/budget/db/list_budgets_filter_integration_test.go`

**Interfaces:**
- Consumes: none.
- Produces: none. `Postgres.ListBudgets` keeps its signature.

- [ ] **Step 1: Write the failing integration test**

Create `services/budget/db/list_budgets_filter_integration_test.go`:

```go
//go:build integration

// Real-Postgres proof that ListBudgets treats a nil production_id as "no filter"
// (zero-UUID trap, same class as #185 and #220). SKIPs without THITTAM_TEST_DSN.
package db_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/testdb"
	"github.com/wegofwd2020/thittam/services/budget"
	budgetdb "github.com/wegofwd2020/thittam/services/budget/db"
)

func TestBudget_ListBudgets_NilProductionIsNoFilter(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := budgetdb.NewPostgres(pool)

	tenantID := seedBudgetListTenant(t, pool)
	prodA := uuid.New()
	prodB := uuid.New()
	bA := seedBudget(t, pool, tenantID, prodA)
	bB := seedBudget(t, pool, tenantID, prodB)

	t.Run("nil production returns every tenant budget", func(t *testing.T) {
		all, err := repo.ListBudgets(ctx, tenantID, uuid.Nil, "", 20, 0)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{bA, bB}, budgetIDs(all),
			"nil production_id must not filter (zero-UUID trap)")
	})

	t.Run("production filter returns only that production", func(t *testing.T) {
		onlyA, err := repo.ListBudgets(ctx, tenantID, prodA, "", 20, 0)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{bA}, budgetIDs(onlyA))
	})
}

func budgetIDs(rows []budget.Budget) []uuid.UUID {
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids
}

// seedBudgetListTenant inserts a tenant and removes its budgets on cleanup.
// country_code and primary_currency_code are NOT NULL since migration 014; the
// name must be unique under tenants_name_ci_unique.
func seedBudgetListTenant(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	tenantID := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'IN', 'INR')`,
		tenantID, "Budget List Tenant "+tenantID.String(), "budget-list-"+tenantID.String())
	require.NoError(t, err, "insert tenant")
	t.Cleanup(func() {
		ctx := context.Background()
		_, err := pool.Exec(ctx, `DELETE FROM budgets WHERE tenant_id = $1`, tenantID)
		assert.NoError(t, err, "cleanup: delete budgets")
		_, err = pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
		assert.NoError(t, err, "cleanup: delete tenant")
	})
	return tenantID
}

func seedBudget(t *testing.T, pool *pgxpool.Pool, tenantID, productionID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO budgets (id, production_id, tenant_id, label, created_by)
		 VALUES ($1, $2, $3, 'test budget', $4)`,
		id, productionID, tenantID, uuid.New())
	require.NoError(t, err, "insert budget")
	return id
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `THITTAM_TEST_DSN=<disposable container DSN> go test -tags=integration ./services/budget/db/ -run TestBudget_ListBudgets_NilProductionIsNoFilter -v`
Expected: the `nil production` subtest FAILS with an empty result list, because the zero UUID is bound.
If no disposable database is available, `go vet -tags=integration ./services/budget/db/` must still compile, and the report must say the RED run did not happen.

- [ ] **Step 3: Fix the SQL**

In `services/budget/db/queries.sql`, replace the `ListBudgets` block:

```sql
-- name: ListBudgets :many
SELECT * FROM budgets
WHERE tenant_id = sqlc.arg('tenant_id')
  AND (sqlc.narg('production_id')::uuid IS NULL OR production_id = sqlc.narg('production_id'))
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');
```

- [ ] **Step 4: Regenerate and check the param type**

Run: `sqlc generate` (from the repo root). Then: `grep -n -A6 "type ListBudgetsParams struct" services/budget/db/queries.sql.go`
Expected: `ProductionID pgtype.UUID`. If it is still `uuid.UUID`, the narg did not take. Stop and check.

- [ ] **Step 5: Update the wrapper**

In `services/budget/db/postgres.go`, replace the `ListBudgets` body's params:

```go
	rows, err := p.q.ListBudgets(ctx, ListBudgetsParams{
		TenantID:     tenantID,
		ProductionID: nullableUUID(productionID),
		Limit:        int32(limit),
		Offset:       int32(offset),
	})
```

Confirm `nullableUUID` exists in `services/budget/db/` (`git grep -n 'func nullableUUID' -- services/budget`). If it does not, add it beside the other helpers, copying the expense version:

```go
func nullableUUID(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}
```

Note: `status` is still discarded in this wrapper. That is a separate defect and is out of scope for this PR.

- [ ] **Step 6: Run the tests**

Run: `go vet ./services/budget/... && go vet -tags=integration ./services/budget/...`
Run: `go test ./services/budget/... -race`
Run (real Postgres): `THITTAM_TEST_DSN=... go test -tags=integration ./services/budget/db/ -run TestBudget_ListBudgets -v`
Expected: all PASS. The `nil production` subtest now returns both budgets.

- [ ] **Step 7: Commit**

```bash
git add services/budget/
git commit -m "fix(budget): nil production_id is no filter in ListBudgets (zero-UUID trap)

Bind SQL NULL for an unfiltered list instead of the zero UUID, which matched
no rows. Same fix shape as #220 and #185.

Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>"
```

---

### Task 2: ledger `ListJournalEntries` nil period filter

**Files:**
- Modify: `services/ledger/db/queries.sql:67-75`
- Regenerate: `services/ledger/db/queries.sql.go`
- Modify: `services/ledger/db/postgres.go:240-262`
- Create: `services/ledger/db/list_journal_entries_filter_integration_test.go`

**Interfaces:**
- Consumes: none.
- Produces: none. `Postgres.ListJournalEntries` keeps its signature (`periodID *uuid.UUID`).

- [ ] **Step 1: Write the failing integration test**

Create `services/ledger/db/list_journal_entries_filter_integration_test.go`:

```go
//go:build integration

// Real-Postgres proof that ListJournalEntries treats a nil period as "no filter"
// (zero-UUID trap, same class as #185 and #220), that a period filter does not
// widen, and that status composes with period. SKIPs without THITTAM_TEST_DSN.
package db_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/testdb"
	"github.com/wegofwd2020/thittam/services/ledger"
	ledgerdb "github.com/wegofwd2020/thittam/services/ledger/db"
)

func TestLedger_ListJournalEntries_Filters(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := ledgerdb.NewPostgres(pool)

	tenantID := seedLedgerListTenant(t, pool)
	periodA := seedPeriod(t, pool, tenantID, 2026, 1)
	periodB := seedPeriod(t, pool, tenantID, 2026, 2)

	jeA1 := seedJournalEntry(t, pool, tenantID, periodA, "draft")
	jeA2 := seedJournalEntry(t, pool, tenantID, periodA, "posted")
	jeB1 := seedJournalEntry(t, pool, tenantID, periodB, "draft")

	t.Run("nil period returns every tenant entry", func(t *testing.T) {
		all, err := repo.ListJournalEntries(ctx, tenantID, nil, "", 20, 0)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{jeA1, jeA2, jeB1}, journalIDs(all),
			"nil period must not filter (zero-UUID trap)")
	})

	t.Run("period filter returns only that period and does not widen", func(t *testing.T) {
		got, err := repo.ListJournalEntries(ctx, tenantID, &periodA, "", 20, 0)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{jeA1, jeA2}, journalIDs(got))
	})

	t.Run("period AND status compose", func(t *testing.T) {
		got, err := repo.ListJournalEntries(ctx, tenantID, &periodA, "posted", 20, 0)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uuid.UUID{jeA2}, journalIDs(got))
	})
}

func journalIDs(rows []ledger.JournalEntry) []uuid.UUID {
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids
}

// seedLedgerListTenant inserts a tenant and removes its rows on cleanup. Children
// go first: journal entries reference accounting periods.
func seedLedgerListTenant(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	tenantID := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'IN', 'INR')`,
		tenantID, "Ledger List Tenant "+tenantID.String(), "ledger-list-"+tenantID.String())
	require.NoError(t, err, "insert tenant")
	t.Cleanup(func() {
		ctx := context.Background()
		_, err := pool.Exec(ctx, `DELETE FROM journal_entries WHERE tenant_id = $1`, tenantID)
		assert.NoError(t, err, "cleanup: delete journal entries")
		_, err = pool.Exec(ctx, `DELETE FROM accounting_periods WHERE tenant_id = $1`, tenantID)
		assert.NoError(t, err, "cleanup: delete periods")
		_, err = pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
		assert.NoError(t, err, "cleanup: delete tenant")
	})
	return tenantID
}

func seedPeriod(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID, year, month int) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO accounting_periods (id, tenant_id, year, month) VALUES ($1, $2, $3, $4)`,
		id, tenantID, year, month)
	require.NoError(t, err, "insert accounting period")
	return id
}

// seedJournalEntry inserts one entry. entry_number is UNIQUE, so it is suffixed with a fresh UUID.
func seedJournalEntry(t *testing.T, pool *pgxpool.Pool, tenantID, periodID uuid.UUID, status string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO journal_entries (id, tenant_id, period_id, entry_number, narration, status)
		 VALUES ($1, $2, $3, $4, 'test entry', $5)`,
		id, tenantID, periodID, "JE-"+id.String(), status)
	require.NoError(t, err, "insert journal entry")
	return id
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `THITTAM_TEST_DSN=<disposable container DSN> go test -tags=integration ./services/ledger/db/ -run TestLedger_ListJournalEntries_Filters -v`
Expected: the `nil period` subtest FAILS with an empty result.
If no disposable database is available, `go vet -tags=integration ./services/ledger/db/` must still compile, and the report must say so.

- [ ] **Step 3: Fix the SQL**

In `services/ledger/db/queries.sql`, replace the `ListJournalEntries` block:

```sql
-- name: ListJournalEntries :many
SELECT * FROM journal_entries
WHERE tenant_id = sqlc.arg('tenant_id')
  AND (sqlc.narg('period_id')::uuid IS NULL OR period_id = sqlc.narg('period_id'))
  AND (sqlc.arg('status') = '' OR status = sqlc.arg('status'))
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');
```

- [ ] **Step 4: Regenerate and check the param type**

Run: `sqlc generate`. Then: `grep -n -A8 "type ListJournalEntriesParams struct" services/ledger/db/queries.sql.go`
Expected: `PeriodID pgtype.UUID`. Status is `string` or `interface{}`; both are acceptable, and the same as ListExpensesParams.

- [ ] **Step 5: Update the wrapper**

In `services/ledger/db/postgres.go`, replace the body of `ListJournalEntries` so it no longer computes `col2`:

```go
func (p *Postgres) ListJournalEntries(ctx context.Context, tenantID uuid.UUID, periodID *uuid.UUID, status string, limit, offset int) ([]ledger.JournalEntry, error) {
	var period uuid.UUID
	if periodID != nil {
		period = *periodID
	}

	rows, err := p.q.ListJournalEntries(ctx, ListJournalEntriesParams{
		TenantID: tenantID,
		PeriodID: nullableUUID(period),
		Status:   status,
		Limit:    int32(limit),
		Offset:   int32(offset),
	})
```

Keep the rest of the function as it is. Confirm `nullableUUID` exists in `services/ledger/db/`. If not, add the helper as in Task 1.

- [ ] **Step 6: Run the tests**

Run: `go vet ./services/ledger/... && go vet -tags=integration ./services/ledger/...`
Run: `go test ./services/ledger/... -race`
Run (real Postgres): `THITTAM_TEST_DSN=... go test -tags=integration ./services/ledger/db/ -run TestLedger_ListJournalEntries -v`
Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add services/ledger/
git commit -m "fix(ledger): nil period is no filter in ListJournalEntries (zero-UUID trap)

Bind SQL NULL for an unfiltered list instead of the zero UUID, which matched
no rows. Period and status still compose.

Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>"
```

---

### Task 3: `GetExpense` nil-subject owner guard

**Files:**
- Modify: `services/expense/handler.go` (GetExpense, the owner check near line 305)
- Modify: `services/expense/handler_test.go` (append one test)

**Interfaces:**
- Consumes: `interceptor.CallerFromContext`, `interceptor.RequirePermission`.
- Produces: none.

- [ ] **Step 1: Write the failing test**

Append to `services/expense/handler_test.go`:

```go
// A nil caller subject must never own an expense, even when SubmittedBy is also zero.
// Without the guard, the owner check passes (Nil == Nil) and the record is returned.
func TestHandler_GetExpense_NilSubjectNeverOwns(t *testing.T) {
	t.Parallel()
	caller := interceptor.CallerInfo{UserID: uuid.Nil, TenantID: uuid.New()}
	h := NewHandler(NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, SubmittedBy: uuid.Nil}, nil
		},
	}), denyPerm{})

	_, err := h.GetExpense(ctxWithCaller(caller), &expensev1.GetExpenseRequest{Id: uuid.New().String()})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}
```

Before running, confirm the `getExpenseFn` field exists on `mockRepo` (`git grep -n getExpenseFn -- services/expense/service_test.go`). If it does not, use the field the existing `TestHandler_GetExpense_OwnerReadsOwn` uses, and say so in the report.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./services/expense/ -run TestHandler_GetExpense_NilSubjectNeverOwns -v`
Expected: FAIL. The handler returns success, because the owner check passes.

- [ ] **Step 3: Add the guard**

In `services/expense/handler.go`, in `GetExpense`, replace:

```go
	if e.SubmittedBy != caller.UserID {
```

with:

```go
	// A nil subject owns nothing. Without this guard, a zero SubmittedBy would match.
	if caller.UserID == uuid.Nil || e.SubmittedBy != caller.UserID {
```

- [ ] **Step 4: Run the tests**

Run: `go test ./services/expense/ -race`
Expected: all PASS, including the existing GetExpense tests.

- [ ] **Step 5: Commit**

```bash
git add services/expense/handler.go services/expense/handler_test.go
git commit -m "fix(expense): GetExpense nil subject never owns a record

Backport the guard the #220 Get handlers have. A zero SubmittedBy now cannot
match a nil caller subject and skip the permission gate.

Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>"
```

---

## Self-Review

1. **Coverage:** budget list (Task 1), ledger list (Task 2), GetExpense guard (Task 3). Review Focus items 1 to 4 each map to a step.
2. **Placeholders:** the only conditional steps are the `nullableUUID` and `getExpenseFn` existence checks. Each names what to do if the check fails.
3. **Types:** `Postgres.ListBudgets` and `Postgres.ListJournalEntries` keep their signatures, so no interface or implementer changes.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-10-12-list-zero-uuid-filters.md`. Recommended: subagent-driven, with Tasks 1 and 2 batched into one implementer dispatch, since they are the same SQL-and-wrapper shape, and Task 3 separate. Reviews are per batch, then a whole-branch review. Ledger ruling: the ledger PR needs a senior reviewer.
