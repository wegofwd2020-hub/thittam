# RejectExpense rejecter attribution + full rejection record (#201) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-26
**Issue:** #201 (RejectExpense records no rejecter identity) — split from #200's review
**Branch:** `feat/expense-reject-attribution-201` off `main`
**Migration:** `migrations/expense/003_add_expense_rejected_by` (up + down) · **Proto:** +3 fields on `message Expense` (non-breaking) · **sqlc:** regen

## Goal

Give expense rejection the same audit trail approval already has. `ApproveExpense`
records `ApprovedBy = caller.UserID`; `RejectExpense` records who rejected today only as
a reason string with no actor. Add a `rejected_by` column, wire it from the caller, and
surface the **full rejection record** (`rejected_by` + the already-stored-but-never-read
`rejection_reason`/`rejected_at`) on the domain object and the API response.

## Context (grounding facts, `main` @ a7b0996)

- **Domain model** `services/expense/models.go:32-51`: `ApprovedBy *uuid.UUID`,
  `ApprovedAt *time.Time`, `RejectionReason *string`, `RejectedAt *time.Time`. **No
  `RejectedBy` field.**
- **Service** `service.go:189-202`: `RejectExpense(ctx, tenantID, expenseID uuid.UUID, reason string) error`
  — guards already-approved/already-rejected, then calls the **dedicated**
  `s.repo.RejectExpense(ctx, tenantID, expenseID, reason)` (NOT `UpdateExpense`). Takes no
  rejecter param today.
- **`ApproveExpense` mirror** `service.go:80,108-114`: takes `approverID uuid.UUID`, sets
  `exp.ApprovedBy = &approverID`, persists via `UpdateExpense`.
- **SQL** `services/expense/db/queries.sql`: dedicated `RejectExpense` query (lines 75-79):
  ```sql
  -- name: RejectExpense :one
  UPDATE expenses
  SET status = 'rejected', rejection_reason = $3, rejected_at = now()
  WHERE id = $1 AND tenant_id = $2
  RETURNING *;
  ```
  Isolated from `UpdateExpenseStatus` (the approve path) — no cross-impact. `approved_by`
  is set in `UpdateExpenseStatus` as `$4`, sqlc param `ApprovedBy pgtype.UUID`.
- **sqlc.yaml** (repo root, expense block): overrides `uuid`→`uuid.UUID`,
  `numeric`→`decimal.Decimal`, `timestamptz`→`time.Time`. A **nullable** `rejected_by UUID`
  generates as `pgtype.UUID` (the override governs non-null base type only), matching the
  existing `ApprovedBy pgtype.UUID` on the db-row struct (`db/models.go:14-33`).
- **Repo write** `db/postgres.go:173-186`: `RejectExpense` builds `RejectExpenseParams{ID,
  TenantID, RejectionReason}` — no rejecter. **Read** `expenseFromDB` (`postgres.go:284-319`)
  maps `ApprovedBy` via `if row.ApprovedBy.Valid { id := uuid.UUID(row.ApprovedBy.Bytes); e.ApprovedBy = &id }`
  but **does NOT map `RejectionReason`/`RejectedAt` at all** (pre-existing gap since #191 —
  `GetExpense`/`ListExpenses` silently drop them).
- **Handler** `handler.go:343-371`: gates tenant → `RequirePermission(ctx, h.perm, "expense:approve")`
  → parse ID → empty-reason check → `h.svc.RejectExpense(ctx, tenantID, expenseID, req.GetReason())`.
  **Does NOT read `interceptor.CallerFromContext`** (the #201 gap). `ApproveExpense`
  (`handler.go:328-334`) reads the caller after tenant+perm+parse, `!ok`→Unauthenticated.
- **Proto** `proto/thittam/expense/v1/expense.proto`: `message Expense` fields 1-16
  (highest `created_at = 16`); has `approved_by=13`/`approved_at=15` but **no
  `rejected_by`/`rejection_reason`/`rejected_at`**. `RejectExpenseRequest` = `{expense_id, reason}`
  (no actor field — like `ApproveExpenseRequest`). `expenseToProto` (`handler.go:573-603`)
  surfaces `ApprovedBy`/`ApprovedAt` but nothing rejection-related.
- **Migrations** `migrations/expense/`: `001`, `002` (added `rejection_reason`+`rejected_at`),
  both with up+down. Pattern `NNN_<snake>.{up,down}.sql`. CI `migration-validate` runs
  `expense` through up AND `down -all` (no leniency past `001`) — `003` needs both files.
- **Tests**: `mockRepo.rejectExpenseFn func(ctx, tenantID, expenseID uuid.UUID, reason string) error`
  (`service_test.go:22,57`); `TestService_RejectExpense_{Success,AlreadyApproved,AlreadyRejected}`;
  handler `TestHandler_RejectExpense_{Success,Denied,NoTenant,InvalidID,EmptyReason}`
  (no `_NoCaller` yet). Helpers `ctxWithCaller`/`ctxTenantNoCaller` exist (from #200).

## Design

### 1. Migration `migrations/expense/003_add_expense_rejected_by.{up,down}.sql`

up:
```sql
-- 003_add_expense_rejected_by.up.sql
-- #201: record WHO rejected an expense (mirrors approved_by). Plain UUID, no FK,
-- matching approved_by / submitted_by precedent in 001.
ALTER TABLE expenses
    ADD COLUMN rejected_by UUID;
```
down:
```sql
ALTER TABLE expenses
    DROP COLUMN IF EXISTS rejected_by;
```

### 2. SQL + sqlc regen (`services/expense/db/queries.sql`)

`RejectExpense` query gains `rejected_by = $4`:
```sql
-- name: RejectExpense :one
UPDATE expenses
SET status = 'rejected', rejection_reason = $3, rejected_at = now(), rejected_by = $4
WHERE id = $1 AND tenant_id = $2
RETURNING *;
```
`sqlc generate` → `RejectExpenseParams` gains `RejectedBy pgtype.UUID`; db-row `Expense`
gains `RejectedBy pgtype.UUID`. Commit the regenerated `queries.sql.go` + `models.go`
(Codegen Freshness gate). **sqlc does NOT validate bare SET columns** (memory:
[[reference-sqlc-where-clause-blind-spot]]) — the real-Postgres `Migration Validate` +
integration jobs are the authoritative gate that `rejected_by` exists and the UPDATE runs.

### 3. Domain model (`services/expense/models.go`)

Add after `ApprovedBy` (mirror its type/tag):
```go
	RejectedBy      *uuid.UUID      `json:"rejected_by,omitempty"`
```

### 4. Service + repo (`service.go`, `repository.go`, `db/postgres.go`)

- `repository.go:22`: `RejectExpense(ctx context.Context, tenantID, expenseID, rejecterID uuid.UUID, reason string) error`.
- `service.go` `RejectExpense`: add `rejecterID uuid.UUID` param (positioned like
  `ApproveExpense`'s `approverID` — after `expenseID`, before `reason`); pass it through:
  `return s.repo.RejectExpense(ctx, tenantID, expenseID, rejecterID, reason)`. Guards unchanged.
- `db/postgres.go` write `RejectExpense`: add the param and set it:
  ```go
  func (p *Postgres) RejectExpense(ctx context.Context, tenantID, expenseID, rejecterID uuid.UUID, reason string) error {
  	_, err := p.q.RejectExpense(ctx, RejectExpenseParams{
  		ID:              expenseID,
  		TenantID:        tenantID,
  		RejectionReason: pgtype.Text{String: reason, Valid: true},
  		RejectedBy:      pgtype.UUID{Bytes: rejecterID, Valid: true},
  	})
  	// ...unchanged error handling...
  }
  ```
- `db/postgres.go` read `expenseFromDB`: add the `RejectedBy` mapping AND fix the
  pre-existing gap (map `RejectionReason`/`RejectedAt`), mirroring `ApprovedBy`/`ApprovedAt`:
  ```go
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
  ```
  (Use the existing `stringFromPgText`/pgtype accessors consistent with the file; confirm
  `row.RejectionReason` is `pgtype.Text` and `row.RejectedAt` is `pgtype.Timestamptz`.)

### 5. Handler (`services/expense/handler.go`)

`RejectExpense`: after the ID parse (and keeping the empty-reason check), insert the caller
gate mirroring `ApproveExpense`, then pass `caller.UserID`:
```go
	caller, ok := interceptor.CallerFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "caller identity not found in context")
	}
	if err := h.svc.RejectExpense(ctx, tenantID, expenseID, caller.UserID, req.GetReason()); err != nil {
		return nil, grpcErr(err)
	}
```
Permission gate stays `"expense:approve"` (unchanged). Order: tenant → perm → parse ID →
empty-reason → caller gate → service call.

### 6. Proto + surfacing (`expense.proto`, `expenseToProto`)

`message Expense` gains (after `created_at = 16`):
```proto
  string rejected_by = 17;
  string rejection_reason = 18;
  google.protobuf.Timestamp rejected_at = 19;
```
`buf generate proto` (target `proto`, no root buf.work.yaml; revert any cross-service gen
drift). Adding fields is non-breaking under the FILE breaking category.
`expenseToProto` (`handler.go:573-603`) gains, mirroring `ApprovedBy`/`ApprovedAt`:
```go
	if e.RejectedBy != nil {
		out.RejectedBy = e.RejectedBy.String()
	}
	if e.RejectionReason != nil {
		out.RejectionReason = *e.RejectionReason
	}
	if e.RejectedAt != nil {
		out.RejectedAt = timestamppb.New(*e.RejectedAt)
	}
```

## Testing

- **Service** (`service_test.go`): update `mockRepo.rejectExpenseFn` signature +
  `mockRepo.RejectExpense` impl + the `TestService_RejectExpense_Success` call site for the
  new `rejecterID` param; assert the rejecterID passed to the service reaches the repo (spy
  the arg). Keep the already-approved/already-rejected guard tests (adjust their call args).
- **Handler** (`handler_test.go`): add `TestHandler_RejectExpense_NoCaller`
  (`ctxTenantNoCaller` → `codes.Unauthenticated`), mirroring `TestHandler_ApproveExpense_NoCaller`;
  update `TestHandler_RejectExpense_Success` to inject a known caller via `ctxWithCaller` and
  assert the response carries `RejectedBy` (+ reason/rejected_at surfaced). Keep
  `_Denied/_NoTenant/_InvalidID/_EmptyReason` (they fail before the caller gate; verify
  ordering keeps their expected codes).
- **Gates:** `sqlc generate` clean (Codegen Freshness); `buf lint` + `buf breaking`
  (FILE — adding fields passes); `go test ./services/expense/... -race`; `go vet ./...`;
  `go build ./...`; `gofmt -l` on touched Go files (empty). Real-Postgres `Migration Validate`
  (up + down for 003) + Integration is the authoritative SQL/column gate.

## Non-goals

- No `rejected_by` on the request wire (`RejectExpenseRequest` unchanged — actor comes from
  the caller, like `ApproveExpenseRequest`).
- No FK constraint on `rejected_by` (matches `approved_by`/`submitted_by`).
- No proto changes beyond the 3 `message Expense` fields; no other RPC touched.
- No `#202` work (approve-an-already-rejected-record) — separate issue.
- No change to the approve path / `UpdateExpenseStatus` query.

## Review weight

Money-decision **authorization** + a new migration + proto change → security-sensitive;
senior engineer required per CLAUDE.md. Standard 2 approvals + senior. Whole-branch review
on the most capable model.
