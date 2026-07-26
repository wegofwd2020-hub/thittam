# RejectExpense rejecter attribution (#201) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Record who rejected an expense (a `rejected_by` column wired from the caller) and surface the full rejection record (`rejected_by` + the stored-but-never-read `rejection_reason`/`rejected_at`) on the domain object and API, mirroring `ApprovedBy`.

**Architecture:** Three layers, three tasks. (1) Schema/contract: migration `003` + the dedicated `RejectExpense` SQL query + sqlc regen + 3 new proto fields + buf regen. (2) Domain/repo/service: model field, `rejecterID` threaded through the `RejectExpense` service + repo interface + `postgres.go` (write + the read-gap fix). (3) Handler: caller gate mirroring `ApproveExpense` + `expenseToProto` surfacing. Each commit builds tree-wide.

**Tech Stack:** Go 1.25, PostgreSQL, sqlc (pinned v1.26.0), buf, `pgx/v5`/`pgtype`, `decimal.Decimal`, `interceptor.CallerFromContext`, testify.

## Global Constraints

- **`rejected_by` is `UUID` NULL, no FK** — matches `approved_by`/`submitted_by` (migration 001). Migration `003_add_expense_rejected_by` needs BOTH `.up.sql` and `.down.sql` (CI `migration-validate` runs `expense` up AND `down -all`, no leniency past `001`).
- **The `RejectExpense` SQL is a DEDICATED query** (`queries.sql:75-79`), isolated from `UpdateExpenseStatus` (the approve path). Do NOT touch `UpdateExpenseStatus`.
- **sqlc does NOT validate bare SET columns** — the real-Postgres `Migration Validate` + integration jobs are the authoritative gate that `rejected_by` exists and the UPDATE runs. sqlc is pinned to **v1.26.0**; use the repo's pinned sqlc (a different local version reformats unrelated files → false Codegen-Freshness drift). Scope `git add` to `services/expense/db/`.
- **`buf generate proto`** (target `proto`, NOT bare; no root buf.work.yaml). buf regenerates the WHOLE gen tree → after running, revert any cross-service `gen/` drift and commit ONLY `gen/expense/`. Adding fields is non-breaking under the FILE breaking category (`buf breaking` passes).
- **Changing the `expense.Repository.RejectExpense` signature ripples to EVERY implementer.** Known implementers: `db.Postgres`, `mockRepo` (service_test.go). There have historically been HIDDEN doubles (e2e/integration) — see [[reference-iam-repository-implementers]]. `go build ./...` does NOT compile other packages' `_test.go`; **only whole-tree `go vet ./...` catches them.** Task 2 MUST grep the whole tree for `RejectExpense` implementers and fix all in the same commit, gated by `go vet ./...`.
- **Money** stays `decimal.Decimal` (not touched here). Caller identity ONLY from `interceptor.CallerFromContext`; `!ok`→`codes.Unauthenticated`, read after the tenant+permission gates.
- **Gate every Go/codegen task** with `gofmt -l <touched .go files>` (empty) in addition to `go test`/`go vet ./...`/`go build ./...`. `db/postgres.go` was gofmt-clean on main; keep it so.
- **Security-sensitive** (money-decision authz + migration + proto) → senior review per CLAUDE.md.
- Commits Conventional-Commits (scope `expense`), ending `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `migrations/expense/003_add_expense_rejected_by.{up,down}.sql` | add `rejected_by` column | 1 |
| `services/expense/db/queries.sql` | `RejectExpense` SET `rejected_by = $4` | 1 |
| `services/expense/db/{queries.sql.go,models.go}` | sqlc-regenerated | 1 |
| `proto/thittam/expense/v1/expense.proto` | `message Expense` +3 fields (17-19) | 1 |
| `gen/expense/v1/*.pb.go` | buf-regenerated | 1 |
| `services/expense/models.go` | `RejectedBy *uuid.UUID` | 2 |
| `services/expense/repository.go` | `RejectExpense` sig +`rejecterID` | 2 |
| `services/expense/db/postgres.go` | write param + read mapping (RejectedBy/Reason/At) | 2 |
| `services/expense/service.go` | `RejectExpense` sig +`rejecterID` | 2 |
| `services/expense/service_test.go` | mock sig + tests | 2 |
| `services/expense/handler.go` | caller gate + `expenseToProto` surfacing | 3 |
| `services/expense/handler_test.go` | `_NoCaller` + `_Success` assert RejectedBy | 3 |

---

### Task 1: Schema + contract (migration, SQL, sqlc, proto, buf)

**Files:** Create the 2 migration files; Modify `queries.sql`, `expense.proto`; regenerate `db/{queries.sql.go,models.go}` + `gen/expense/`.

**Interfaces:**
- Produces (consumed by Tasks 2/3): db-layer `RejectExpenseParams.RejectedBy pgtype.UUID` + db-row `Expense.RejectedBy pgtype.UUID`; proto `expensev1.Expense` fields `RejectedBy string`, `RejectionReason string`, `RejectedAt *timestamppb.Timestamp`.

- [ ] **Step 1: Migration files**

Create `migrations/expense/003_add_expense_rejected_by.up.sql`:
```sql
-- 003_add_expense_rejected_by.up.sql
-- #201: record WHO rejected an expense (mirrors approved_by). Plain UUID, no FK,
-- matching approved_by / submitted_by precedent in 001.
ALTER TABLE expenses
    ADD COLUMN rejected_by UUID;
```
Create `migrations/expense/003_add_expense_rejected_by.down.sql`:
```sql
ALTER TABLE expenses
    DROP COLUMN IF EXISTS rejected_by;
```

- [ ] **Step 2: Edit the RejectExpense query**

In `services/expense/db/queries.sql`, the `RejectExpense` query (lines 75-79) — add `rejected_by = $4` to the SET (leave `UpdateExpenseStatus` untouched):
```sql
-- name: RejectExpense :one
UPDATE expenses
SET status = 'rejected', rejection_reason = $3, rejected_at = now(), rejected_by = $4
WHERE id = $1 AND tenant_id = $2
RETURNING *;
```

- [ ] **Step 3: Regenerate sqlc**

Run the repo's pinned sqlc (prefer a Makefile target if one exists, e.g. `make generate-sqlc`/`make sqlc`; else `sqlc generate` with the v1.26.0 binary). Confirm `services/expense/db/queries.sql.go` now has `RejectedBy pgtype.UUID` in `RejectExpenseParams` and the db-row `Expense` struct has `RejectedBy pgtype.UUID`. If sqlc reformats files in OTHER service packages, that's a version mismatch — discard those and re-run with the pinned version so only `services/expense/db/` changes.

- [ ] **Step 4: Proto — add 3 fields**

In `proto/thittam/expense/v1/expense.proto`, `message Expense` (after `google.protobuf.Timestamp created_at = 16;`):
```proto
  string rejected_by = 17;
  string rejection_reason = 18;
  google.protobuf.Timestamp rejected_at = 19;
```

- [ ] **Step 5: Regenerate buf**

Run `buf generate proto`. Then `git status` — if any `gen/` files OUTSIDE `gen/expense/` changed, revert them (`git checkout -- <those paths>`). Confirm `gen/expense/v1/expense.pb.go`'s `Expense` struct gained `RejectedBy`, `RejectionReason`, `RejectedAt`.

- [ ] **Step 6: Verify — build + buf checks + freshness**

Run:
```bash
buf lint proto && buf breaking proto --against '.git#branch=main,subdir=proto'
go build ./... && go vet ./... && go test ./services/expense/ -race
gofmt -l services/expense/db/
```
`go build`/`go test` still pass: `postgres.go`'s `RejectExpense` still builds (it constructs `RejectExpenseParams` by named fields; the new `RejectedBy` field defaults to the zero value = NULL) and existing reject tests don't assert `rejected_by`. `buf breaking` passes (fields added, none removed/renumbered).

- [ ] **Step 7: Commit**
```bash
git add migrations/expense/003_add_expense_rejected_by.up.sql migrations/expense/003_add_expense_rejected_by.down.sql \
        services/expense/db/queries.sql services/expense/db/queries.sql.go services/expense/db/models.go \
        proto/thittam/expense/v1/expense.proto gen/expense/
git commit -m "feat(expense): add rejected_by column + proto rejection fields (#201)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```
(Double-check `git status` is clean of stray cross-service `gen/`/`db/` drift before committing.)

---

### Task 2: Domain + repo + service — thread rejecterID

**Files:** Modify `services/expense/models.go`, `repository.go`, `db/postgres.go`, `service.go`, `service_test.go`

**Interfaces:**
- Consumes: Task 1's `RejectExpenseParams.RejectedBy`, db-row `Expense.RejectedBy/RejectionReason/RejectedAt`.
- Produces (consumed by Task 3): `Service.RejectExpense(ctx, tenantID, expenseID, rejecterID uuid.UUID, reason string) error`; domain `Expense.RejectedBy *uuid.UUID`.

- [ ] **Step 1: Write failing service tests**

In `services/expense/service_test.go`: update `mockRepo`'s `rejectExpenseFn` field + `RejectExpense` method to the new signature, and update the existing reject tests' call sites. Add an assertion that the rejecterID reaches the repo:
```go
// mockRepo field (was: func(ctx, tenantID, expenseID uuid.UUID, reason string) error)
rejectExpenseFn func(ctx context.Context, tenantID, expenseID, rejecterID uuid.UUID, reason string) error
```
```go
func (m *mockRepo) RejectExpense(ctx context.Context, tenantID, expenseID, rejecterID uuid.UUID, reason string) error {
	if m.rejectExpenseFn != nil {
		return m.rejectExpenseFn(ctx, tenantID, expenseID, rejecterID, reason)
	}
	return nil
}
```
Rewrite `TestService_RejectExpense_Success` to assert both reason and rejecterID:
```go
func TestService_RejectExpense_Success(t *testing.T) {
	rejecter := uuid.New()
	var gotReason string
	var gotRejecter uuid.UUID
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, Status: "submitted"}, nil
		},
		rejectExpenseFn: func(_ context.Context, _, _, rejecterID uuid.UUID, reason string) error {
			gotReason = reason
			gotRejecter = rejecterID
			return nil
		},
	})
	require.NoError(t, svc.RejectExpense(context.Background(), uuid.New(), uuid.New(), rejecter, "over budget"))
	assert.Equal(t, "over budget", gotReason)
	assert.Equal(t, rejecter, gotRejecter)
}
```
Update `TestService_RejectExpense_AlreadyApproved` / `_AlreadyRejected` call sites to pass a `uuid.New()` rejecterID arg (their assertions unchanged — they fail before the repo call).

- [ ] **Step 2: Run — expect FAIL** (signature mismatch): `go test ./services/expense/ -run TestService_RejectExpense`

- [ ] **Step 3: Domain model**

In `services/expense/models.go`, add after the `ApprovedBy` field (`:47`):
```go
	RejectedBy      *uuid.UUID      `json:"rejected_by,omitempty"`
```

- [ ] **Step 4: Repository interface + service**

`services/expense/repository.go:22`:
```go
	RejectExpense(ctx context.Context, tenantID, expenseID, rejecterID uuid.UUID, reason string) error
```
`services/expense/service.go` `RejectExpense` — add `rejecterID uuid.UUID` (after `expenseID`, before `reason`) and pass it through:
```go
func (s *Service) RejectExpense(ctx context.Context, tenantID, expenseID, rejecterID uuid.UUID, reason string) error {
	exp, err := s.repo.GetExpense(ctx, tenantID, expenseID)
	if err != nil {
		return fmt.Errorf("get expense: %w", err)
	}
	if exp.Status == "approved" {
		return ErrAlreadyApproved
	}
	if exp.Status == "rejected" {
		return ErrAlreadyRejected
	}
	return s.repo.RejectExpense(ctx, tenantID, expenseID, rejecterID, reason)
}
```

- [ ] **Step 5: Repo write + read mapping**

`services/expense/db/postgres.go` — `RejectExpense` write: add the param + set `RejectedBy`:
```go
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
```
`expenseFromDB` — add the `RejectedBy` mapping AND the pre-existing-gap fix for `RejectionReason`/`RejectedAt`, after the `ApprovedAt` block (`~:317`):
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
(If the file has a `stringFromPgText` helper used elsewhere, either style is fine — match the surrounding `if row.X.Valid { ... }` pattern used for `ApprovedBy`/`ApprovedAt`. Confirm `row.RejectionReason` is `pgtype.Text` and `row.RejectedAt` is `pgtype.Timestamptz`.)

- [ ] **Step 6: Find & fix ALL other Repository implementers**

Grep the WHOLE tree for other implementers of the `RejectExpense` repo method (hidden e2e/integration doubles have bitten this repo before — [[reference-iam-repository-implementers]]):
```bash
grep -rn "RejectExpense(ctx" --include=*.go | grep -v "_test.go:.*rejectExpenseFn"
grep -rln "func.*RejectExpense(ctx context.Context" --include=*.go
```
Update every type that implements `expense.Repository` to the new signature. If none exist beyond `Postgres`/`mockRepo`, note that explicitly.

- [ ] **Step 7: Run — expect PASS + whole-tree vet**
```bash
go test ./services/expense/ -race && go vet ./... && go build ./...
gofmt -l services/expense/
```
`go vet ./...` (WHOLE TREE) is the gate that proves no other-package implementer was missed — it MUST pass.

- [ ] **Step 8: Commit**
```bash
git add services/expense/models.go services/expense/repository.go services/expense/db/postgres.go services/expense/service.go services/expense/service_test.go
# plus any other implementer files found in Step 6
git commit -m "feat(expense): thread rejecter identity through RejectExpense (#201)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Handler caller gate + proto surfacing

**Files:** Modify `services/expense/handler.go`, `handler_test.go`

**Interfaces:**
- Consumes: Task 2's `Service.RejectExpense(...rejecterID...)`, domain `Expense.RejectedBy`; Task 1's proto `Expense.RejectedBy/RejectionReason/RejectedAt`.

- [ ] **Step 1: Write failing handler tests**

In `services/expense/handler_test.go`. Add `_NoCaller` (MUST use a NON-empty reason so it reaches the caller gate, which sits AFTER the empty-reason check) and rewrite `_Success` to assert `RejectedBy` surfaces:
```go
func TestHandler_RejectExpense_NoCaller(t *testing.T) {
	t.Parallel()
	_, err := newHandler().RejectExpense(ctxTenantNoCaller(uuid.New()),
		&expensev1.RejectExpenseRequest{ExpenseId: uuid.New().String(), Reason: "over budget"})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_RejectExpense_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	expID := uuid.New()
	rejecter := uuid.New()
	caller := interceptor.CallerInfo{UserID: rejecter, TenantID: tenantID}
	callCount := 0
	h := NewHandler(NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			callCount++
			if callCount > 1 {
				// second call: GetExpense after reject → return the rejected record
				return &Expense{ID: id, TenantID: tid, Status: "rejected", Amount: decimal.NewFromInt(5000), RejectedBy: &rejecter}, nil
			}
			return &Expense{ID: id, TenantID: tid, Status: "submitted", Amount: decimal.NewFromInt(5000)}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})
	resp, err := h.RejectExpense(ctxWithCaller(caller), &expensev1.RejectExpenseRequest{ExpenseId: expID.String(), Reason: "over budget"})
	require.NoError(t, err)
	assert.Equal(t, "rejected", resp.GetStatus())
	assert.Equal(t, rejecter.String(), resp.GetRejectedBy())
}
```
Keep `_Denied/_NoTenant/_InvalidID/_EmptyReason` unchanged (they fail before the caller gate — verify `_EmptyReason` still returns `InvalidArgument`, i.e. the empty-reason check stays BEFORE the caller gate).

- [ ] **Step 2: Run — expect FAIL** (`RejectExpense` handler passes no caller; `GetRejectedBy` empty / signature mismatch): `go test ./services/expense/ -run TestHandler_RejectExpense`

- [ ] **Step 3: Wire the handler**

In `services/expense/handler.go` `RejectExpense` (`:343-371`), after the empty-reason check and before the service call, insert the caller gate and pass `caller.UserID`:
```go
	caller, ok := interceptor.CallerFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "caller identity not found in context")
	}
	if err := h.svc.RejectExpense(ctx, tenantID, expenseID, caller.UserID, req.GetReason()); err != nil {
		return nil, grpcErr(err)
	}
```
(Order: tenant → `expense:approve` perm → parse ID → empty-reason → caller gate → service.)

- [ ] **Step 4: Surface the rejection record in `expenseToProto`**

In `services/expense/handler.go` `expenseToProto` (`:573-603`), after the `ApprovedAt` block, add (mirror `ApprovedBy`/`ApprovedAt`):
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

- [ ] **Step 5: Run — expect PASS**
```bash
go test ./services/expense/ -race && go vet ./... && go build ./...
gofmt -l services/expense/handler.go services/expense/handler_test.go
```

- [ ] **Step 6: Commit**
```bash
git add services/expense/handler.go services/expense/handler_test.go
git commit -m "feat(expense): record + surface rejecter identity on RejectExpense (#201)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- Migration `003` (rejected_by, up+down) → Task 1 ✅
- `RejectExpense` SQL `rejected_by = $4` + sqlc regen → Task 1 ✅
- Proto `message Expense` +3 fields + buf regen → Task 1 ✅
- Domain `RejectedBy *uuid.UUID` → Task 2 ✅
- `rejecterID` threaded service→repo→postgres.go write → Task 2 ✅
- `expenseFromDB` maps RejectedBy + the #191 reason/rejected_at read-gap → Task 2 ✅
- Handler caller gate (`!ok`→Unauthenticated) + pass caller.UserID → Task 3 ✅
- `expenseToProto` surfaces all 3 → Task 3 ✅
- Tests: service rejecterID→repo, handler `_NoCaller` (non-empty reason) + `_Success` asserts RejectedBy → Tasks 2/3 ✅
- Non-goals honored (no request-wire actor, no FK, no #202, no approve-path change) ✅

**Placeholder scan:** none — full migration SQL, query, proto, service/repo/handler diffs, and tests. The "confirm pgtype accessor" and "find all implementers" notes are compiler/grep-checked instructions.

**Type consistency:** `RejectExpense(ctx, tenantID, expenseID, rejecterID uuid.UUID, reason string) error` identical across repo interface (Task 2), service def (Task 2), `Postgres` (Task 2), `mockRepo` (Task 2), and the handler call site (Task 3). `RejectedBy *uuid.UUID` (domain, Task 2) ↔ `pgtype.UUID` (db-row, Task 1) mapped in `expenseFromDB`. Proto `RejectedBy string`/`RejectionReason string`/`RejectedAt *timestamppb.Timestamp` (Task 1) consumed by `expenseToProto` (Task 3). `RejectExpenseParams.RejectedBy pgtype.UUID` (Task 1) written in `postgres.go` (Task 2).

**Ordering:** Task 1 (schema/contract; tree still builds because `postgres.go` names params and the new field defaults to zero) → Task 2 (thread rejecterID; whole-tree `go vet` for Repository doubles) → Task 3 (handler + surfacing; needs both proto + service sig). Every commit builds and passes its gate.
