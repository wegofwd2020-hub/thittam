# Tenant-Isolation Guard-by-Type (#139 slice H / #159) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-23
**Issue:** #159 (tenant-isolation audit of billing/document/notifications/iam), #139 slice H
**Branch:** `fix/tenant-isolation-guard-139h` off `main` (`b24b390`)
**Migration:** none

## Goal

Convert seven tenant-isolation invariants that are currently held by *caller
discipline* into *compile-time guarantees*, by threading `tenantID` as a
required parameter and enforcing it in SQL. No live cross-tenant defect exists
to fix; this hardens the seven sites that are each one refactor away from
becoming one.

## Context — the audit found zero live defects

#159 asked for a call-graph-traced tenant-isolation audit of the four services
#157 did not cover (`billing`, `document`, `notifications`, `iam`), because
"grep cannot decide it" — a query's isolation depends on whether its id arrives
from the request or from an already-tenant-scoped parent read.

Four independent read-only audits (one per service, three scans each: `queries.sql`
WHERE-clauses, `SELECT tenant_id FROM` tautologies, inline SQL literals) were run
and every candidate traced through its call graph:

| service | LIVE | LATENT | safe-by-discipline | correct-by-design |
|---|---|---|---|---|
| billing | 0 | 16 | 0 | 15 |
| document | 0 | 4 | 0 | 12 |
| notifications | 0 | 2 | 7 | 3 |
| iam | 0 | 0 | 4 | ~24 |

**Zero live cross-tenant defects.** The structural reason: every RPC-reachable
path derives `tenantID` from `interceptor.TenantFromRequest` (the JWT), never the
request body (#144), and slices E/F/G plus slice F's payment-cluster fix already
closed the reachable holes. So slice H is a *hardening* slice, not an *exploit-fix*
slice — exactly the outcome the audit-before-fix discipline was meant to establish.

### What is deliberately deferred (filed, not fixed here)

- **#172** — dead unscoped sqlc-generated queries (billing's entire `queries.sql`
  accessor layer is unwired; document's 4; notifications' 2). Zero callers today;
  footguns if resurrected. Delete-vs-scope decided when someone needs them.
- **#173** — billing `ListDunningAttempts`/`RecordDunningAttempt` have no tenant
  parameter at all; a leak the moment an RPC exposes them.
- **#174** — document `InitiateUpload` sets `folder_id` from the request without a
  folder-ownership check (referential-integrity gap, not a read leak).

## Design

**Fix shape (uniform):** thread `tenantID uuid.UUID` as a **required** parameter
through `Repository` → `Service` → SQL (guard-by-type — a caller cannot compile
without supplying it; a check returning only `error` can be skipped, a required
value cannot). SQL gains the tenant predicate. A cross-tenant id then matches
zero rows → the method's **existing** `pgx.ErrNoRows` / `RowsAffected()==0` branch
→ `NotFound`. Deliberately the same answer as a genuinely missing row: nothing
confirms a row exists in another tenant (no `PermissionDenied` oracle). The
signature change plus **all** call sites plus **all** mock/double implementers
(across `services/*`, `tests/integration/`, and `e2e/critical_path/`) land in one
commit per service; whole-tree `go vet ./...` is the completion gate — it is the
only detector that catches the hidden e2e/integration doubles a package-scoped
build misses.

**No migration:** every table receiving a predicate already declares `tenant_id
UUID NOT NULL` (`budgets`, `journal_entries`, `users`, `tenant_purge_requests`).
The one child table (`journal_lines`) has no `tenant_id` column and uses its
parent's via a JOIN.

### Task 1 — ledger (2 sites)

1. **`ListJournalLines`** — `journal_lines` has **no `tenant_id` column** (only
   `journal_id` FK → `journal_entries`). Thread `tenantID` and rewrite the read to
   JOIN the parent, enforcing the tenant in the same atomic query:

   ```sql
   SELECT l.id, l.journal_id, l.account_id, l.debit_amount, l.credit_amount,
          l.currency, l.description
     FROM journal_lines l
     JOIN journal_entries je ON l.journal_id = je.id
    WHERE l.journal_id = $1
      AND je.tenant_id = $2
   ```

   Signature: `ListJournalLines(ctx, tenantID, journalID uuid.UUID)`. Callers
   already hold `tenantID` (they precede this with a scoped `GetJournalEntry`);
   pass it in. `CreateJournalLine` is **not** a target — it runs inside the tx that
   just inserted the header with `TenantID: je.TenantID`, `journalID` not
   caller-supplied (correct-by-design, per #157 §2.4).

2. **`UpdateJournalStatus`** — `journal_entries` has `tenant_id`. Today the repo
   compensates with `SELECT tenant_id FROM journal_entries WHERE id = $1` and
   **discards that query's error** (`services/ledger/db/postgres.go:302`,
   `_ = p.db.QueryRow(...)`). Thread `tenantID` as a parameter, drop the
   compensating self-lookup, add `AND tenant_id = $N` to the UPDATE, and
   `RowsAffected()==0 → ErrNotFound`. This closes both the tautology and the
   discarded error.

### Task 2 — budget (1 site)

3. **`UpdateBudgetStatus`** — `budgets` has `tenant_id`. Same shape as the
   tautological predicate today (tenant read off the target row). Thread `tenantID`,
   add `AND tenant_id = $N`, `RowsAffected()==0 → ErrNotFound`. Callers
   (`SubmitBudget`/`ApproveBudget`) already do a scoped `GetBudget(ctx, tenantID, id)`
   first, so they hold `tenantID`.

### Task 3 — iam (4 sites)

4. **`GetUserByID`** (the `ChangePassword` path) — `users` has `tenant_id`. The
   query is PK-only (`WHERE id=$1`); the only caller, `ChangePassword`, derives the
   id from `interceptor.ActorFromRequest` (the caller's own verified identity —
   safe today). Thread `tenantID` and add `AND tenant_id = $N`. Note the sibling
   `GetUser` already enforces tenant via a post-fetch Go check
   (`if row.TenantID != tenantID`) — this brings `GetUserByID` to parity.
5. **`UpdatePasswordHash`** — `users`, `UPDATE ... WHERE id=$1`. Two callers
   (`rehashIfNeeded` off a scoped `GetUserByEmail`; `ChangePassword` off the
   self-derived id). Thread `tenantID`, add `AND tenant_id = $N`,
   `RowsAffected()==0 → ErrNotFound` (or the method's existing error contract).
6. **`ApproveTenantPurgeRequest`** — `tenant_purge_requests` has `tenant_id`.
   `UPDATE ... WHERE id=$1 AND status='pending'`. Caller `ApproveTenantPurge`
   fetches `GetOpenTenantPurgeRequest(ctx, tenantID)` first. Add `AND tenant_id = $N`
   (thread if the method lacks the param).
7. **`CancelTenantPurgeRequest`** — same table/shape as #6. Add `AND tenant_id = $N`.

## Testing

Each threaded signature gets an integration assertion (`//go:build integration`,
real-Postgres CI job) that a cross-tenant id returns `NotFound`/`ErrNoRows` and
touches no other tenant's row. Per the audit's testing note:

- **Assert the `tenantID` the repository actually received** (or that a cross-tenant
  id yields zero rows) — a status-code-only assertion passes against the vulnerable
  code, because mock defaults return usable rows.
- **A predicate-revert "teeth check" is worthless here** — handler/service tests are
  mock-driven, so removing a SQL predicate never reaches Postgres. CI's real-Postgres
  job is the only proof of the predicate. Do not alter a test to manufacture a failure.
- Existing unit tests (`ledger` `callerCtx` mints a random UserID; add a
  `callerCtxAs(tid, uid)`-style helper where an actor/tenant must be named) are
  updated to the new signatures in the same commit.

## Coverage

Floors: iam/general-ledger ≥ 85%, budget ≥ 80%. The threaded params add branches
already exercised by happy-path tests; the new assertions are on the tenant value
received, not new lines — consistent with #144/#146, where guarding tests moved
line coverage ~0pp because the lines were already executed.

## Files

- **Task 1 (ledger):** `services/ledger/db/queries.sql` (+`.sql.go` regen or raw SQL
  in `db/postgres.go`), `services/ledger/repository.go`, `services/ledger/service.go`,
  `services/ledger/service_test.go` (`mockRepo`), `e2e/critical_path/helpers_test.go`
  (ledger double), `tests/integration/` ledger cases.
- **Task 2 (budget):** `services/budget/db/queries.sql`/`postgres.go`,
  `repository.go`, `service.go`, `service_test.go` (`mockRepo`),
  `tests/integration/vertical/mocks_test.go` (budget double), `e2e/critical_path/helpers_test.go`
  (budget double).
- **Task 3 (iam):** `services/iam/db/queries.sql`/`postgres.go`, `services/iam/repository.go`,
  `services/iam/service.go`, `services/iam/service_test.go` (`mockRepo`),
  `e2e/critical_path/helpers_test.go` (iam double), `tests/integration/` iam cases.

Exact call-site and double inventories are the writing-plans grounding pass; the
whole-tree `go vet ./...` gate is the backstop for any double missed there.

## Non-goals

- No migration, no schema change.
- No proto/RPC change (all signatures are internal repo/service).
- No fix to the dead sqlc layers, billing dunning methods, or InitiateUpload — filed
  as #172/#173/#174.
- No Postgres row-level security (the #159 "longer-term alternative" — evaluated
  platform-wide separately, once the audit has established which tables are
  genuinely tenant-scoped).

## Review weight

Touches `iam` and `general-ledger` — both require a senior engineer per CLAUDE.md.
The whole-branch review runs on the most capable model.
