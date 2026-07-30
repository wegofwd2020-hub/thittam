# Self-scoped reads for purchase-orders + petty-cash (#220) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-30
**Issue:** #220 (mirror of #165 for PO + petty-cash)
**Branch:** `security/expense-self-scope-po-pettycash-220` off `main` (8ae2d53)
**Migration:** none · **Proto:** additive (2 fields) · **sqlc:** yes (2 query rewrites + regen)

## Goal

A member who **raised** a purchase order or **holds** a petty-cash advance can read their own records
without the tenant-wide `expense:read` permission — mirroring #165's expense self-scope. The two List
queries are rewritten to fix the same latent `sqlc` nullable-filter trap #165 fixed (an unfiltered list
currently returns nothing) and to wire the silently-dropped `status` filter.

## Context (grounding facts, `main` @ 8ae2d53, module `github.com/wegofwd2020/thittam`)

- **#165 pattern (to mirror), `services/expense/handler.go`:**
  - `ListExpenses`: `if req.GetSubmittedByMe()` → `caller, err := interceptor.ActorFromRequest(ctx, "")`
    (token subject; errors `Unauthenticated`/`InvalidArgument`/`PermissionDenied`), scope to `caller`, NO
    `expense:read`. Else `RequirePermission(ctx, h.perm, "expense:read")` and `submittedBy = uuid.Nil`.
    Calls `h.svc.ListExpenses(ctx, tenantID, productionID, status, limit, 0, submittedBy)`.
  - `GetExpense`: owner-first — fetch `e` tenant-scoped, then `if e.SubmittedBy != caller.UserID {
    RequirePermission(expense:read) }`. Gate runs AFTER fetch (ownership needs the row).
  - `interceptor.ActorFromRequest(ctx, "")` (`pkg/interceptor/actor.go:22`) returns the caller's
    `uuid.UUID` subject, rejecting a missing caller / empty subject with `Unauthenticated`.
- **PO reads (`handler.go:99-150`):** `GetPurchaseOrder` and `ListPurchaseOrders` both gate on a flat
  unconditional `RequirePermission(ctx, h.perm, "expense:read")`, tenant-scoped only, no owner branch.
  `PurchaseOrder.RaisedBy uuid.UUID` (`models.go:24`; proto `raised_by = 12`) is set from
  `caller.UserID` at `CreatePurchaseOrder` (`handler.go:38-78`) — an actor-trusted field, exactly like
  `SubmittedBy`.
- **Petty-cash reads (`handler.go:446-497`):** `GetPettyCashAdvance` / `ListPettyCashAdvances` same flat
  `expense:read`. `PettyCashAdvance.IssuedTo uuid.UUID` (`models.go:59`; proto `issued_to = 4`) is set
  at create from `req.GetIssuedTo()` — the **holder**, who may differ from the creator. So the
  self-scope owner is `IssuedTo` (the holder), consistent with the existing `SettlePettyCash` guard
  (`service.go:253-272`: `pc.IssuedTo != callerID → ErrNotAdvanceHolder`).
- **The sqlc trap (unfixed for these two), `services/expense/db/queries.sql`:**
  `ListPurchaseOrders` (`:12`) and `ListPettyCashAdvances` (`:62`) use positional
  `WHERE tenant_id = $1 AND ($2::uuid IS NULL OR production_id = $2)`. `sqlc.yaml:112` overrides
  `uuid → uuid.UUID`, so `$2` generates a plain `uuid.UUID` (field `Column2`), and `postgres.go` passes
  `productionID` (`uuid.Nil` when unfiltered) straight in → binds the literal zero UUID, never SQL NULL
  → the `IS NULL` branch never fires → an unfiltered list filters `production_id = '000…0'` and returns
  nothing. Both queries also **ignore `status`** (a discarded `_ string` param in `Postgres.ListPurchaseOrders`
  / `ListPettyCashAdvances`; status is never in the SQL). Contrast the #165-fixed `ListExpenses`
  (`queries.sql:35`) which uses `sqlc.arg`/`sqlc.narg('x')::uuid` → `pgtype.UUID` params +
  `nullableUUID(id)` (`postgres.go:380`, `uuid.Nil → pgtype.UUID{}` = NULL).
- **Proto (`proto/thittam/expense/v1/expense.proto`):** `ListPurchaseOrdersRequest` (`:142`) and
  `ListPettyCashAdvancesRequest` (`:179`) both top out at field `4` (`production_id`, `status`, `limit`,
  `after`). Next free = `5` (same slot #165 used for `bool submitted_by_me = 5`). `proto/buf.yaml`
  breaking mode is `FILE` — appending a field is additive/safe.
- **`expense.Repository` (`repository.go:10`) has FOUR implementers** — widening `ListPurchaseOrders` /
  `ListPettyCashAdvances` requires editing all in one commit (whole-tree `go vet` is the gate):
  `db.Postgres` (`db/postgres.go:65`,`:220`), `mockRepo` (`service_test.go:80`,`:104`), `expenseMock`
  (`tests/integration/vertical/mocks_test.go:60`,`:69`), `expenseRepo`
  (`e2e/critical_path/helpers_test.go:420`,`:432`) — plus regenerated `db/queries.sql.go`.
- **Coverage floor:** `services/expense` ≥ 80% (CLAUDE.md).

## Design

### Proto (additive)

```protobuf
message ListPurchaseOrdersRequest {
  string production_id = 1; string status = 2; int32 limit = 3; string after = 4;
  bool raised_by_me = 5;   // (#220) self-scope to the PO raiser; no expense:read required
}
message ListPettyCashAdvancesRequest {
  string production_id = 1; string status = 2; int32 limit = 3; string after = 4;
  bool issued_to_me = 5;   // (#220) self-scope to the advance holder; no expense:read required
}
```
`buf generate` → regenerate `gen/`. (Whole-tree gen drift is reverted per the usual buf discipline; only expense gen changes are intended.)

### Handlers (`services/expense/handler.go`) — mirror #165 exactly

- **`ListPurchaseOrders`:**
  ```go
  var raisedBy uuid.UUID
  if req.GetRaisedByMe() {
      caller, err := interceptor.ActorFromRequest(ctx, "")
      if err != nil { return nil, err }
      raisedBy = caller
  } else {
      if err := interceptor.RequirePermission(ctx, h.perm, "expense:read"); err != nil { return nil, err }
      raisedBy = uuid.Nil
  }
  // …parse productionID as today…
  pos, err := h.svc.ListPurchaseOrders(ctx, tenantID, productionID, req.GetStatus(), int(req.GetLimit()), 0, raisedBy)
  ```
- **`GetPurchaseOrder`:** drop the leading `expense:read`; fetch first, then
  `if po.RaisedBy != caller.UserID { RequirePermission(expense:read) }`. Needs `caller` from
  `interceptor.CallerFromContext` (Unauthenticated if absent), like `GetExpense`.
- **`ListPettyCashAdvances`:** identical shape keyed on `issued_to_me` → `issuedTo`, passed to
  `h.svc.ListPettyCashAdvances(ctx, tenantID, productionID, status, limit, 0, issuedTo)`.
- **`GetPettyCashAdvance`:** fetch first, then `if pc.IssuedTo != caller.UserID { RequirePermission(expense:read) }`.

### Service + Repository (`service.go`, `repository.go`, 4 implementers)

Widen both List signatures by one trailing `uuid.UUID` (mirroring `ListExpenses`'s `submittedBy`):
- `ListPurchaseOrders(ctx, tenantID, productionID uuid.UUID, status string, limit, offset int, raisedBy uuid.UUID) ([]PurchaseOrder, error)`
- `ListPettyCashAdvances(ctx, tenantID, productionID uuid.UUID, status string, limit, offset int, issuedTo uuid.UUID) ([]PettyCashAdvance, error)`

`Service` passes through. `mockRepo`/`expenseMock`/`expenseRepo` gain the trailing param (e2e/mock: filter or ignore consistent with their existing behavior — the e2e `expenseRepo` should honor the filter so the e2e self-scope path is real, mirroring how #165's helpers were made discriminating). All four edited in the SAME commit as the interface change.

### SQL (`services/expense/db/queries.sql`) + regen

Rewrite both to the `ListExpenses` shape (fixes the production_id trap AND wires status):
```sql
-- name: ListPurchaseOrders :many
SELECT * FROM purchase_orders
WHERE tenant_id = sqlc.arg('tenant_id')
  AND (sqlc.narg('production_id')::uuid IS NULL OR production_id = sqlc.narg('production_id'))
  AND (sqlc.narg('raised_by')::uuid IS NULL OR raised_by = sqlc.narg('raised_by'))
  AND (sqlc.arg('status') = '' OR status = sqlc.arg('status'))
ORDER BY raised_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: ListPettyCashAdvances :many
SELECT * FROM petty_cash_advances
WHERE tenant_id = sqlc.arg('tenant_id')
  AND (sqlc.narg('production_id')::uuid IS NULL OR production_id = sqlc.narg('production_id'))
  AND (sqlc.narg('issued_to')::uuid IS NULL OR issued_to = sqlc.narg('issued_to'))
  AND (sqlc.arg('status') = '' OR status = sqlc.arg('status'))
ORDER BY issued_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');
```
`sqlc generate` (pinned v1.26.0) → `queries.sql.go` params become `pgtype.UUID` for the two nullable
filters. `Postgres.ListPurchaseOrders`/`ListPettyCashAdvances` pass `nullableUUID(productionID)`,
`nullableUUID(raisedBy|issuedTo)`, and the real `status` (no longer discarded). Confirm `raised_at` /
`issued_at` are the actual sort columns (they are today).

## Testing

- **Unit (`handler_test.go`, `service_test.go`):** self-scope path returns without `expense:read`
  (use a `denyPerm`/no-`expense:read` caller); non-self path still requires `expense:read`;
  `GetPurchaseOrder`/`GetPettyCashAdvance` owner reads without the permission, non-owner needs it,
  cross-tenant id → NotFound. Assert the message/branch as #165's tests do. The unit mock is blind to
  SQL semantics — it only proves the handler wiring.
- **Real-Postgres integration (mirror #165's `//go:build integration` test):** the ONLY place that
  proves the rewritten SQL. Seed POs/advances across two raisers/holders and productions/statuses;
  assert `raised_by_me`/`issued_to_me` returns self-only; no-filter → all (proving the production_id
  trap is fixed — this would return empty on the pre-rewrite query); per-production; per-status; and the
  AND-composition (self AND production AND status) via `ElementsMatch` on exact IDs. Integration tests
  SKIP without `THITTAM_TEST_DSN`; CI's real-Postgres job is authoritative.
- **Gates:** `buf lint`, `buf breaking` (FILE — additive passes); `sqlc generate` (no drift =
  Codegen-Freshness green); `go build ./...`; whole-tree `go vet ./...` (catches all 4 repo
  implementers); `go test ./services/expense/... -race`; `gofmt -l` touched files; expense coverage
  ≥ 80%.

## Non-goals

- No new permission; no change to write RPCs or `Create*`/`Approve*`/`Settle*`.
- No `after`-cursor / pagination change (leave `after` as today).
- No self-scope for other verticals (budget/inventory/etc.) — expense only.
- No change to `ListExpenses` (already done in #165).

## Review weight

Authorization surface (self-scope replacing a permission gate) + a real SQL correctness fix — the
integration test is load-bearing (the unit mock cannot see the nullable-filter behavior, the exact class
that shipped broken in #165/#185). Senior per the security nature. Whole-branch review on a capable
model, attention on: the owner-first gate ordering, `ActorFromRequest` token-subject trust (never the
request), the petty-cash owner = `IssuedTo` (holder, not creator) distinction, and the four-implementer
widening.
