# Self-scoped expense reads for submitters (#165) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-26
**Issue:** #165 (a `member` can submit an expense but can't read it back — `expense:read` excludes `member`) — split from #139 slice D
**Branch:** `feat/expense-self-scope-read-165` off `main`
**Migration:** none · **Proto:** add `ListExpensesRequest.submitted_by_me` (buf-safe) · **sqlc:** rewrite `ListExpenses` query

## Goal

Slice D gated the expense read RPCs on `expense:read`, granted to super_admin/manager/
coordinator/accountant/project_supervisor — **not `member`**. A member holds `expense:submit`
(can create) but has no path to read their own submission back. Add a self-scoped path where
authz is **`expense:read` OR caller-is-submitter** — never a widening of `expense:read`:
1. **`ListExpenses`** gains `submitted_by_me`: when set, the list is scoped to the caller's own
   submissions and needs no `expense:read` (self-scope IS the authz, mirroring `ListNotifications`).
2. **`GetExpense`** self-scopes owner-first: the submitter reads their own expense; everyone else
   still needs `expense:read`.

## Context (grounding facts, `main` @ 3d58573)

- **`member` = `{production:read, expense:submit, document:read}`** (`services/iam/service.go:110`);
  `expense:read` granted only to the 5 roles above (migration `iam/020`). Confirmed.
- **Read handlers** (`services/expense/handler.go`): `GetExpense` (:260, gate :266) and
  `ListExpenses` (:282, gate :288) both call `RequirePermission(ctx, h.perm, "expense:read")`
  and never read the caller. `SubmitExpense` sets `SubmittedBy = caller.UserID` (:230, the real
  verified subject). `Expense.SubmittedBy uuid.UUID` is populated on every read
  (`expenseFromDB`). So an owner check is `expense.SubmittedBy == caller.UserID`.
- **Self-scope precedent** — `ListNotifications`/`GetNotification` (`services/notifications/handler.go:187-228`)
  derive the recipient via **`interceptor.ActorFromRequest(ctx, "")`** (token subject, never
  request-supplied) + an `AND recipient_id = $N` predicate, with **no `RequirePermission`** — the
  self-scope is the authz. `ActorFromRequest` (`pkg/interceptor/actor.go:22`) is the ready primitive.
  There is **no existing `permission-OR-owner` combinator**, so the `GetExpense` OR-gate is composed
  in the handler (owner-first, then perm).
- **REST:** `ListExpenses` = `GET /api/v1/expenses`, `GetExpense` = `GET /api/v1/expenses/{id}`
  (grpc-gateway, Kong-fronted). A new bool request field → `?submitted_by_me=true` query param
  automatically; **no annotation/Kong change**. The web client always calls
  `listExpenses(productionId, …)` with a production_id.
- **⚠️ Latent bug that blocks a cross-production self-list** — the `ListExpenses` optional
  `production_id` filter is `($2::uuid IS NULL OR production_id = $2)`, but the sqlc
  `uuid → uuid.UUID` override makes `$2` **non-nullable**; the handler passes `uuid.Nil` (all-zeros,
  **not** SQL NULL) when no production is given, so the query filters `production_id = '000…0'` →
  **returns nothing**. Unexercised today (the UI always sends a production_id), but a member's
  "my expenses" is naturally cross-production and would hit it. **Fixing this is a prerequisite**
  (see Design §2). Unit tests are blind to it (mock repo ignores params) — a real-Postgres
  integration test is required ([[reference-sqlc-where-clause-blind-spot]]).
- **`expense.Repository.ListExpenses`** (`repository.go:20`) has implementers `Postgres`, `mockRepo`
  (service_test.go), `expenseMock` (tests/integration/vertical/mocks_test.go) + possibly an e2e
  double — widening its signature needs whole-tree `go vet` ([[reference-iam-repository-implementers]]).

## Design

### 1. Proto — `ListExpensesRequest.submitted_by_me`

`proto/thittam/expense/v1/expense.proto`, add field 5 (additive, buf-safe):
```proto
message ListExpensesRequest {
  string production_id = 1;
  string status = 2;
  int32 limit = 3;
  string after = 4;
  // When true, the list is scoped to the caller's own submissions (submitted_by =
  // verified token subject) and requires no expense:read — a member reading their own.
  // When false/absent, requires expense:read and lists tenant-wide (unchanged). (#165)
  bool submitted_by_me = 5;
}
```
`buf generate proto`. `GetExpense` needs no proto change.

### 2. sqlc — rewrite `ListExpenses` to genuinely-nullable optional filters

Convert `ListExpenses` (`services/expense/db/queries.sql:34`) to named params so both optional
UUID filters are properly nullable (`sqlc.narg` → `pgtype.UUID`; `nil → SQL NULL`). This fixes the
`production_id` no-filter bug **and** adds the `submitted_by` filter in one idiom:
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
`sqlc generate` → `ListExpensesParams{TenantID uuid.UUID, ProductionID pgtype.UUID, SubmittedBy pgtype.UUID, Status ..., Limit int32, Offset int32}` (named — no more `Column2`/`Column3`).

**Repo** (`services/expense/db/postgres.go` `ListExpenses`) builds the nullable params from the
domain `uuid.UUID` args (`uuid.Nil` → NULL), mirroring the existing `ApprovedBy pgtype.UUID`
conversion in `UpdateExpense`:
```go
func (p *Postgres) ListExpenses(ctx context.Context, tenantID, productionID uuid.UUID, status string, limit, offset int, submittedBy uuid.UUID) ([]expense.Expense, error) {
	rows, err := p.q.ListExpenses(ctx, ListExpensesParams{
		TenantID:     tenantID,
		ProductionID: nullableUUID(productionID), // pgtype.UUID{Valid: id != uuid.Nil, Bytes: id}
		SubmittedBy:  nullableUUID(submittedBy),
		Status:       status,
		Limit:        int32(limit),
		Offset:       int32(offset),
	})
	// ...unchanged mapping...
}
```
(Add a small `nullableUUID(id uuid.UUID) pgtype.UUID` helper in `db/postgres.go` if one doesn't
exist.) **Safety:** a real production UUID still filters exactly as before; only the no-filter case
changes (empty → all). No migration.

### 3. Repository interface + Service — thread `submittedBy`

- `repository.go:20`: `ListExpenses(ctx, tenantID, productionID uuid.UUID, status string, limit, offset int, submittedBy uuid.UUID) ([]Expense, error)`.
- `Service.ListExpenses` (`service.go:131`) gains `submittedBy uuid.UUID` (passed through; `uuid.Nil`
  = tenant-wide). Limit clamp unchanged.
- Update all implementers in the same commit (whole-tree `go vet`): `Postgres`, `mockRepo`,
  `expenseMock`, and any e2e double (grep `func.*ListExpenses(ctx`).

### 4. Handlers

**`ListExpenses`** (`handler.go:282`) — branch on the flag:
```go
	var submittedBy uuid.UUID
	if req.GetSubmittedByMe() {
		// Self-scope IS the authz — a member can only ever get their own submissions.
		caller, err := interceptor.ActorFromRequest(ctx, "")
		if err != nil {
			return nil, err
		}
		submittedBy = caller
	} else {
		if err := interceptor.RequirePermission(ctx, h.perm, "expense:read"); err != nil {
			return nil, err
		}
	}
	// ...parse production_id (unchanged)...
	expenses, err := h.svc.ListExpenses(ctx, tenantID, productionID, req.GetStatus(), int(req.GetLimit()), 0, submittedBy)
```
(The tenant gate stays first. The old unconditional `RequirePermission` moves into the `else`.)

**`GetExpense`** (`handler.go:260`) — owner-first OR-gate (fetch, then authz):
```go
	tenantID, ok := tenant.IDFromContext(ctx)          // unchanged
	if !ok { return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context") }
	caller, ok := interceptor.CallerFromContext(ctx)
	if !ok { return nil, status.Error(codes.Unauthenticated, "caller identity not found in context") }
	id, err := uuid.Parse(req.GetId())
	if err != nil { return nil, status.Error(codes.InvalidArgument, "invalid expense ID") }
	e, err := h.svc.GetExpense(ctx, tenantID, id)      // tenant-scoped; NotFound if absent
	if err != nil { return nil, grpcErr(err) }
	if e.SubmittedBy != caller.UserID {
		if err := interceptor.RequirePermission(ctx, h.perm, "expense:read"); err != nil {
			return nil, err
		}
	}
	return expenseToProto(e), nil
```
Order change: the permission gate now runs AFTER the fetch (needed to know the submitter). A
non-owner without `expense:read` probing an id gets `NotFound` (absent) or `PermissionDenied`
(present) — existence is visible only to authenticated same-tenant callers; acceptable.

## Testing

- **Handler** (`handler_test.go`): `ListExpenses` with `submitted_by_me=true` + `denyPerm{}` →
  succeeds and passes the caller's id as the `submittedBy` filter (spy the repo arg), proving
  self-scope needs no `expense:read`; `submitted_by_me=false` + `denyPerm{}` → `PermissionDenied`;
  `submitted_by_me=true` with no caller → `Unauthenticated`. `GetExpense`: owner (caller ==
  `SubmittedBy`) + `denyPerm{}` → success; non-owner + `denyPerm{}` → `PermissionDenied`; non-owner
  + `allowAllPerm{}` → success; no caller → `Unauthenticated`.
- **Integration** (`//go:build integration`, real Postgres — the authoritative SQL proof): seed
  expenses for two submitters across two productions in one tenant; assert
  `ListExpenses(submittedBy=A, productionID=nil)` → only A's across both productions (proves the
  nullable `submitted_by` filter + the `production_id` no-filter FIX); `ListExpenses(nil, nil)` →
  all tenant expenses (proves production_id fix); `ListExpenses(nil, productionID=P)` → only P's
  (proves the per-production path still works). Mirror an existing expense integration test's
  tenant/seed (country_code/currency + unique name — [[reference-integration-test-tenant-seed]]).
- Gates: `buf lint`+`buf breaking`; `sqlc generate` clean (Codegen Freshness); `go test
  ./services/expense/... -race`; **whole-tree `go vet ./...`** (Repository widening); `go build
  ./...`; `gofmt -l` touched files. Real-Postgres Integration + Migration Validate in CI.

## Non-goals

- **PO / petty-cash self-scope** — `GetPurchaseOrder`/`ListPurchaseOrders` (`RaisedBy`) and
  `GetPettyCashAdvance`/`ListPettyCashAdvances` (`IssuedTo`) are also `expense:read`-gated; #165 is
  scoped to *expenses*. If members need to read those back, that's a follow-up (same shape).
- No change to the `expense:read` grant or to `member`'s role.
- No pagination fix for `ListExpenses` (`after`/offset is a separate pre-existing gap; offset stays 0).
- The `production_id` filter fix is folded in **only because the self-list requires a working
  optional-UUID filter** — not a general audit of other services' optional filters.

## Review weight

`expense` **authorization** (self-scope of money data) + proto + sqlc → security-sensitive; senior
per CLAUDE.md. Whole-branch review on the most capable model.
