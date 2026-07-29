# Self-scoped expense reads (#165) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a `member` read their own expense submissions — `ListExpenses` gains a `submitted_by_me` self-scope (no `expense:read`), `GetExpense` self-scopes owner-first — authz is `expense:read` OR caller-is-submitter, never a widening of `expense:read`.

**Architecture:** Three tasks, each builds tree-wide. (1) Contract + the prerequisite optional-UUID-filter fix: proto `submitted_by_me` field + rewrite the `ListExpenses` sqlc query to genuinely-nullable filters (fixes the latent `production_id` no-filter bug) + regen + repo internal conversion. (2) Thread `submittedBy` through `Repository`/`Service`/all implementers. (3) Wire the handlers (`ListExpenses` flag branch, `GetExpense` owner-first) + tests.

**Tech Stack:** Go 1.25, gRPC/buf, sqlc (pinned v1.26.0), pgx/v5 `pgtype.UUID`, `interceptor.ActorFromRequest`/`RequirePermission`, testify.

## Global Constraints

- **Self-scope IS the authz** for `submitted_by_me=true` (mirrors `ListNotifications`): derive the id via `interceptor.ActorFromRequest(ctx, "")` (verified token subject, NEVER request-supplied), NO `expense:read` gate on that path. `submitted_by_me=false/absent` keeps the current `expense:read` gate + tenant-wide list.
- **`GetExpense` owner-first:** fetch (tenant-scoped) → if `e.SubmittedBy == caller.UserID` serve; else `RequirePermission(expense:read)`. Never widen `expense:read`.
- **Optional-UUID filters must be genuinely nullable** — `sqlc.narg('x')::uuid` → `pgtype.UUID`, converted from `uuid.Nil → SQL NULL` in the repo (mirror the `ApprovedBy pgtype.UUID` conversion in `UpdateExpense`). A real UUID still filters exactly as before; only the no-filter case changes (empty → all). This FIXES the pre-existing `production_id` bug — do not leave either filter using a non-nullable `uuid.UUID`.
- **Widening `Repository.ListExpenses` (add `submittedBy`) breaks all implementers** — `Postgres`, `mockRepo`, `expenseMock` (tests/integration/vertical), + any e2e double; grep `func.*ListExpenses(ctx` and fix all in Task 2's commit. `go build` misses other-package `_test.go` doubles — **whole-tree `go vet ./...`** is the gate.
- **sqlc can't validate the bare WHERE/nullable filter** — a real-Postgres `//go:build integration` test is the authoritative proof ([[reference-sqlc-where-clause-blind-spot]]); seed tenants with `country_code`/`primary_currency_code` + a unique name ([[reference-integration-test-tenant-seed]]).
- Proto field-add + adding no RPC → buf-safe (`FILE` category). sqlc pinned v1.26.0; scope `git add` to touched packages; revert cross-service gen drift.
- Gate every Go/codegen task with `gofmt -l <touched .go>` (empty; some expense files carry pre-existing gofmt debt on `main` — diff-vs-main, add none new) + `go vet ./...` + `go build ./...`.
- **Security-sensitive** (expense authz self-scope) → senior review per CLAUDE.md.
- Commits Conventional-Commits (scope `expense`), ending `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `proto/thittam/expense/v1/expense.proto` | `ListExpensesRequest.submitted_by_me` | 1 |
| `gen/expense/v1/*.pb.go` | buf-regenerated | 1 |
| `services/expense/db/queries.sql` + `queries.sql.go` | nullable `ListExpenses` filters | 1 |
| `services/expense/db/postgres.go` | nullable-UUID conversion + `submittedBy` param | 1,2 |
| `services/expense/repository.go` | `ListExpenses` +`submittedBy` | 2 |
| `services/expense/service.go` | `Service.ListExpenses` +`submittedBy`; `GetExpense`/`ListExpenses` handlers | 2,3 |
| `services/expense/service_test.go`, `tests/integration/vertical/mocks_test.go` (+ e2e double) | mock sigs | 2 |
| `services/expense/db/*_integration_test.go` | real-Postgres filter proof | 2 |
| `services/expense/handler.go`, `handler_test.go` | flag branch + owner-first + tests | 3 |

---

### Task 1: Contract + nullable-filter fix (proto, sqlc, repo internal)

**Files:** Modify `expense.proto`, `db/queries.sql`, `db/postgres.go`; regenerate `gen/expense/`, `db/queries.sql.go`.

**Interfaces:**
- Produces: `expensev1.ListExpensesRequest.SubmittedByMe bool`; `db.ListExpensesParams{TenantID uuid.UUID, ProductionID pgtype.UUID, SubmittedBy pgtype.UUID, Status, Limit, Offset}`. `Postgres.ListExpenses` external signature UNCHANGED this task (submitted_by always NULL until Task 2).

- [ ] **Step 1: Proto field**

In `proto/thittam/expense/v1/expense.proto` `ListExpensesRequest`, add:
```proto
  // When true, scoped to the caller's own submissions (submitted_by = verified token
  // subject); requires no expense:read. False/absent = expense:read + tenant-wide (#165).
  bool submitted_by_me = 5;
```
`buf generate proto` (or `make generate-proto`). Revert any `gen/` drift outside `gen/expense/`.

- [ ] **Step 2: Rewrite the ListExpenses query (nullable filters)**

In `services/expense/db/queries.sql`, replace the `ListExpenses` query with:
```sql
-- name: ListExpenses :many
SELECT * FROM expenses
WHERE tenant_id = sqlc.arg('tenant_id')
  AND (sqlc.narg('production_id')::uuid IS NULL OR production_id = sqlc.narg('production_id'))
  AND (sqlc.narg('submitted_by')::uuid  IS NULL OR submitted_by  = sqlc.narg('submitted_by'))
  AND (sqlc.arg('status') = '' OR status = sqlc.arg('status'))
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');
```
`sqlc generate`. Confirm `ListExpensesParams` now has `ProductionID pgtype.UUID`, `SubmittedBy pgtype.UUID`, `Status string`, `TenantID uuid.UUID`, `Limit`/`Offset int32` (named, no `Column2`/`Column3`). Only `services/expense/db/` should change.

- [ ] **Step 3: Repo conversion (fix production_id no-filter; submitted_by NULL for now)**

In `services/expense/db/postgres.go`, add a helper if none exists:
```go
// nullableUUID maps the zero UUID to SQL NULL (an "omit this filter" sentinel).
func nullableUUID(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}
```
Update `Postgres.ListExpenses` (signature UNCHANGED this task) to build the new params:
```go
	rows, err := p.q.ListExpenses(ctx, ListExpensesParams{
		TenantID:     tenantID,
		ProductionID: nullableUUID(productionID),
		SubmittedBy:  pgtype.UUID{}, // NULL until Task 2 threads the arg
		Status:       status,
		Limit:        int32(limit),
		Offset:       int32(offset),
	})
```

- [ ] **Step 4: Verify + commit**
```bash
buf lint proto && buf breaking proto --against '.git#branch=main,subdir=proto'
go build ./... && go vet ./... && go test ./services/expense/ -race
gofmt -l services/expense/db/postgres.go
```
Tree builds (interface unchanged; `submitted_by_me` unused in Go yet); `buf breaking` passes; existing expense tests pass. The `production_id` no-filter bug is now fixed at the SQL layer (proven in Task 2's integration test).
```bash
git add proto/thittam/expense/v1/expense.proto gen/expense/ services/expense/db/queries.sql services/expense/db/queries.sql.go services/expense/db/postgres.go
git commit -m "feat(expense): add submitted_by_me field + nullable ListExpenses filters (#165)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Thread `submittedBy` through Repository/Service/implementers + integration proof

**Files:** Modify `repository.go`, `db/postgres.go`, `service.go`, `service_test.go`, `tests/integration/vertical/mocks_test.go` (+ any e2e double); add `db/*_integration_test.go`

**Interfaces:**
- Consumes: Task 1's `ListExpensesParams.SubmittedBy`.
- Produces: `Repository.ListExpenses(ctx, tenantID, productionID uuid.UUID, status string, limit, offset int, submittedBy uuid.UUID)`; `Service.ListExpenses(...submittedBy)`.

- [ ] **Step 1: Widen interface + service + repo**

`repository.go:20`:
```go
	ListExpenses(ctx context.Context, tenantID, productionID uuid.UUID, status string, limit, offset int, submittedBy uuid.UUID) ([]Expense, error)
```
`service.go` `Service.ListExpenses` — add `submittedBy uuid.UUID` (last param), pass through:
```go
func (s *Service) ListExpenses(ctx context.Context, tenantID, productionID uuid.UUID, status string, limit, offset int, submittedBy uuid.UUID) ([]Expense, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.ListExpenses(ctx, tenantID, productionID, status, limit, offset, submittedBy)
}
```
`db/postgres.go` `Postgres.ListExpenses` — add `submittedBy uuid.UUID` and use it: `SubmittedBy: nullableUUID(submittedBy)`.

- [ ] **Step 2: Fix all other implementers (whole-tree)**

Grep `func.*ListExpenses(ctx context.Context` across the tree; add `submittedBy uuid.UUID` to every `expense.Repository` implementer: `mockRepo` (service_test.go — a recording stub; capture the arg for the Task 3 handler test), `expenseMock` (tests/integration/vertical/mocks_test.go), and any e2e double. Update the handler call site in `service.go`/`handler.go` ListExpenses to pass `uuid.Nil` for now (Task 3 wires the flag) so the tree builds.

- [ ] **Step 3: Integration test (real-Postgres filter proof)**

Add `services/expense/db/list_expenses_filter_integration_test.go` (`//go:build integration`), mirroring an existing expense integration test's seeding. Seed one tenant, two submitters (A, B), two productions (P1, P2), an expense for each (A/P1, A/P2, B/P1). Assert:
- `ListExpenses(tenant, productionID=uuid.Nil, "", 20, 0, submittedBy=A)` → A/P1 + A/P2 (2), none of B (proves submitted_by filter + production_id no-filter FIX across productions).
- `ListExpenses(tenant, uuid.Nil, "", 20, 0, uuid.Nil)` → all 3 (proves production_id no-filter FIX).
- `ListExpenses(tenant, productionID=P1, "", 20, 0, uuid.Nil)` → A/P1 + B/P1 (proves per-production still works).
Skips locally without `THITTAM_TEST_DSN`; CI real-Postgres runs it.

- [ ] **Step 4: Verify + commit (whole-tree vet)**
```bash
go test ./services/expense/ -race && go vet ./... && go build ./...
gofmt -l services/expense/repository.go services/expense/service.go services/expense/db/postgres.go services/expense/service_test.go tests/integration/vertical/mocks_test.go
```
`go vet ./...` (WHOLE TREE) MUST pass — proves every implementer updated.
```bash
git add services/expense/repository.go services/expense/service.go services/expense/db/postgres.go services/expense/service_test.go tests/integration/vertical/mocks_test.go services/expense/db/list_expenses_filter_integration_test.go
# + any e2e double + the handler call-site file if separate
git commit -m "feat(expense): thread submittedBy filter through ListExpenses repo/service (#165)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Handlers — self-scope + owner-first + tests

**Files:** Modify `services/expense/handler.go`, `handler_test.go`

**Interfaces:**
- Consumes: Task 1's `req.GetSubmittedByMe()`; Task 2's `Service.ListExpenses(...submittedBy)`.

- [ ] **Step 1: Write failing handler tests**

In `services/expense/handler_test.go` (use `allowAllPerm{}`/`denyPerm{}`, `ctxWithCaller`, and a recording `mockRepo.listExpensesFn`/`getExpenseFn`). Add:
- `TestHandler_ListExpenses_SubmittedByMe_SelfScope` — `denyPerm{}` (no expense:read), `submitted_by_me=true`, caller `X`; asserts the repo receives `submittedBy == X` and it succeeds (self-scope needs no expense:read).
- `TestHandler_ListExpenses_TenantWide_RequiresRead` — `denyPerm{}`, `submitted_by_me=false` → `codes.PermissionDenied` (repo never called).
- `TestHandler_ListExpenses_SubmittedByMe_NoCaller` — `ctxTenantNoCaller`, `submitted_by_me=true` → `codes.Unauthenticated`.
- `TestHandler_GetExpense_OwnerReadsOwn` — `denyPerm{}`, `getExpenseFn` returns `Expense{SubmittedBy: X}`, caller `X` → success.
- `TestHandler_GetExpense_NonOwnerDenied` — `denyPerm{}`, expense `SubmittedBy: Y`, caller `X` → `codes.PermissionDenied`.
- `TestHandler_GetExpense_NonOwnerWithRead` — `allowAllPerm{}`, expense `SubmittedBy: Y`, caller `X` → success.
- `TestHandler_GetExpense_NoCaller` — `ctxTenantNoCaller` → `codes.Unauthenticated`.
Keep/adjust the existing `_Success/_Denied/_NoTenant/_InvalidID` tests (the plain `ListExpenses_Denied` now needs `submitted_by_me=false`; `GetExpense_Denied` needs a non-owner caller so the perm gate is reached).

- [ ] **Step 2: Run — expect FAIL**: `go test ./services/expense/ -run 'TestHandler_ListExpenses|TestHandler_GetExpense'`

- [ ] **Step 3: Wire `ListExpenses`**

In `handler.go` `ListExpenses`, after the tenant gate, replace the unconditional `RequirePermission` with the flag branch (spec §4), then pass `submittedBy` as the new last arg to `h.svc.ListExpenses(...)`. Parse `production_id` unchanged.

- [ ] **Step 4: Wire `GetExpense` owner-first**

In `handler.go` `GetExpense`, reorder to: tenant gate → `caller, ok := interceptor.CallerFromContext(ctx)` (`!ok`→Unauthenticated) → parse id → `e := h.svc.GetExpense(...)` → `if e.SubmittedBy != caller.UserID { RequirePermission(expense:read) }` → `return expenseToProto(e)` (spec §4).

- [ ] **Step 5: Run — expect PASS + gate**
```bash
go test ./services/expense/ -race && go vet ./... && go build ./...
gofmt -l services/expense/handler.go services/expense/handler_test.go
```

- [ ] **Step 6: Commit**
```bash
git add services/expense/handler.go services/expense/handler_test.go
git commit -m "feat(expense): self-scoped reads — submitted_by_me list + owner-first GetExpense (#165)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- `submitted_by_me` proto field → Task 1 ✅
- nullable `ListExpenses` filters (fixes production_id) + regen → Task 1 ✅
- `submittedBy` threaded Repository/Service/Postgres/all implementers → Task 2 ✅
- real-Postgres integration proof (self / no-filter / per-production) → Task 2 ✅
- `ListExpenses` self-scope branch (ActorFromRequest, no expense:read) → Task 3 ✅
- `GetExpense` owner-first OR-gate → Task 3 ✅
- Non-goals honored (no PO/petty-cash, no grant change, no pagination) ✅

**Placeholder scan:** full proto/SQL/repo/handler/test code given. "grep all implementers" / "mirror an existing integration seed" are compiler/grep-checked instructions, not TODOs.

**Type consistency:** `ListExpenses(ctx, tenantID, productionID uuid.UUID, status string, limit, offset int, submittedBy uuid.UUID)` identical across `Repository`/`Service`/`Postgres`/`mockRepo`/`expenseMock`/e2e-double (Task 2) + handler call site (Task 3). `ListExpensesParams.{ProductionID,SubmittedBy} pgtype.UUID` (Task 1) built via `nullableUUID` (Task 1/2). `req.GetSubmittedByMe() bool` (Task 1) consumed in the handler (Task 3). `ActorFromRequest`→`uuid.UUID`, compared to `Expense.SubmittedBy uuid.UUID`.

**Ordering:** Task 1 (contract + sqlc + repo-internal; tree builds, interface unchanged, submitted_by NULL) → Task 2 (thread submittedBy; whole-tree vet; handler passes uuid.Nil) → Task 3 (handler flag branch + owner-first). Every commit builds tree-wide and passes its gate.
