# Expense Write RPCs — Reject / PO-Approve / Settle (#191) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-25
**Issue:** #191 (expense: RejectExpense / ApprovePurchaseOrder / SettlePettyCash RPCs) — #60 Phase C follow-up
**Branch:** `feat/expense-write-rpcs-191` off `main` (`ab868b2`)
**Migration:** one (expense `002`)

## Goal

Add the three write RPCs the expense UI already calls but that have no backing
gRPC: `RejectExpense`, `ApprovePurchaseOrder`, `SettlePettyCash` — plus annotate
them onto the existing C2 expense gateway, and fix the adjacent
`CreatePurchaseOrder` `po_number` collision (the UI never sends it, yet it's
`NOT NULL UNIQUE`).

## Context

The expense gateway (`:9082`, #60 C2) routes `/api/v1/expenses`,
`/api/v1/purchase-orders`, `/api/v1/petty-cash` to expense-tracking. The UI
(`web/src/lib/api/expenses.ts`) already calls three sub-paths that 404 today:
`POST …/expenses/{id}/reject` (`{reason}`), `POST …/purchase-orders/{id}/approve`,
`POST …/petty-cash/{id}/settle` (`{unspent_amount}`). None have a proto RPC.

The grounding map shows most plumbing already exists — the work is the missing
service+handler+proto layers, plus one small migration:

| RPC | DB columns | SQL | repo | service | handler | proto |
|---|---|---|---|---|---|---|
| ApprovePurchaseOrder | `status`(has `approved`)/`approved_by`/`approved_at` **exist** | `UpdatePurchaseOrderStatus` **exists** | `UpdatePurchaseOrder` **exists** | build | build | build |
| SettlePettyCash | `status`/`settled_at`/`unspent_amount` **exist** | `SettlePettyCashAdvance` **exists** | `UpdatePettyCashAdvance` **exists** | build | build | build |
| RejectExpense | `status` has `rejected`; **no `rejection_reason`** | build | build | build | build | build |

The `ApproveExpense` chain (handler `handler.go:272`, service `service.go:64`,
repo `UpdateExpense`→`UpdateExpenseStatus`) is the end-to-end pattern to copy.

## Design

### 1. Migration `expense/002` — rejection reason

`migrations/expense/002_add_expense_rejection.up.sql`:
```sql
ALTER TABLE expenses
    ADD COLUMN rejection_reason TEXT,
    ADD COLUMN rejected_at      TIMESTAMPTZ;
```
Down drops both columns. (`status='rejected'` is already in the CHECK; only the
reason/timestamp are missing. `expense/001` has a `.down.sql`, so the CI
`Migration Validate (up + down)` leg is fine.)

### 2. Proto (`expense.proto`) — 3 RPCs + annotations

The annotations import is already present (C2). Add:
```proto
  rpc RejectExpense(RejectExpenseRequest) returns (Expense) {
    option (google.api.http) = { post: "/api/v1/expenses/{expense_id}/reject" body: "*" };
  }
  rpc ApprovePurchaseOrder(ApprovePurchaseOrderRequest) returns (PurchaseOrder) {
    option (google.api.http) = { post: "/api/v1/purchase-orders/{id}/approve" body: "*" };
  }
  rpc SettlePettyCash(SettlePettyCashRequest) returns (PettyCashAdvance) {
    option (google.api.http) = { post: "/api/v1/petty-cash/{id}/settle" body: "*" };
  }
```
Request messages:
```proto
message RejectExpenseRequest { string expense_id = 1; string reason = 2; }
message ApprovePurchaseOrderRequest { string id = 1; }
message SettlePettyCashRequest { string id = 1; string unspent_amount = 2; }
```
Path placeholders bind by field name (`{expense_id}`, `{id}`). Adding RPCs +
messages is not a buf breaking change. `buf generate proto`, scope the commit to
`gen/expense/v1/` (revert whole-tree drift, per C2).

### 3. Service layer (`services/expense/service.go`)

- **`RejectExpense(ctx, tenantID, expenseID uuid.UUID, reason string) error`** —
  `repo.GetExpense`; reject only a non-terminal expense (return
  `ErrAlreadyApproved` if `status=="approved"`, a new `ErrAlreadyRejected` if
  `status=="rejected"`); then `repo.RejectExpense(ctx, tenantID, expenseID,
  reason)` (new repo method). Publishes nothing new (no event defined; matches
  the parenthetical scope).
- **`ApprovePurchaseOrder(ctx, tenantID, poID uuid.UUID, approverID uuid.UUID) error`** —
  `repo.GetPurchaseOrder`; return `ErrAlreadyApproved` if already `approved`;
  set `Status="approved"`, `ApprovedBy=&approverID`, `ApprovedAt=&now`; call the
  existing `repo.UpdatePurchaseOrder`. **No approval-limit / dual-approval logic**
  (unlike `ApproveExpense`) — a permission-gated status flip; the richer workflow
  is out of scope (YAGNI).
- **`SettlePettyCash(ctx, tenantID, advanceID uuid.UUID, unspent decimal.Decimal) error`** —
  `repo.GetPettyCashAdvance`; set `Status="settled"`, `UnspentAmount=unspent`,
  `SettledAt=&now`; call the existing `repo.UpdatePettyCashAdvance`. (Settle
  closes the advance; `unspent_amount` is what the holder returns.)

### 4. Repo (`services/expense/`)

- Add `RejectExpense(ctx, tenantID, expenseID uuid.UUID, reason string) error` to
  the `Repository` interface + Postgres impl, backed by a new sqlc query:
  ```sql
  -- name: RejectExpense :one
  UPDATE expenses
  SET status = 'rejected', rejection_reason = $3, rejected_at = now()
  WHERE id = $1 AND tenant_id = $2
  RETURNING *;
  ```
- PO-approve and settle reuse the existing `UpdatePurchaseOrder` /
  `UpdatePettyCashAdvance` (no new repo methods).
- The `Expense`/`PurchaseOrder`/`PettyCashAdvance` domain structs gain the fields
  they need (`Expense.RejectionReason *string`, `RejectedAt *time.Time`;
  `PettyCashAdvance` already has `UnspentAmount`/`SettledAt`). `sqlc generate`
  refreshes the generated params/rows.

### 5. Handlers (`services/expense/handler.go`) — copy the `ApproveExpense` shape

Each: `tenant.IDFromContext` → `RequirePermission` → parse ID(s) → `svc` call →
re-fetch → `<x>ToProto`. Permissions:
- `RejectExpense` → `expense:approve` (an approval-authority action).
- `ApprovePurchaseOrder` → `expense:approve`.
- `SettlePettyCash` → `expense:submit` (operational recording, not an approval).
`SettlePettyCash` parses `unspent_amount` with `decimal.NewFromString` →
`InvalidArgument` on error.

### 6. `CreatePurchaseOrder` po_number fix (`handler.go` / `service.go`)

When `req.GetPoNumber() == ""`, generate a unique code server-side rather than
inserting `''` (which collides on the 2nd create via `po_number NOT NULL
UNIQUE`). A small helper `genPONumber() string` → e.g. `"PO-2026-" +
<8-hex-of-uuid>` (uppercased), assigned before the repo insert. Human-readable,
collision-free, no migration. (Only the empty case is touched; a UI-supplied
number passes through unchanged.)

## Testing

- **Handler unit tests** (`handler_test.go`), per new RPC, copying the
  `ApproveExpense` trio: `_Success` (asserts the mapped status —
  e.g. `rejected`/`approved`/`settled`), `_Denied` (`denyPerm` + a repo fn that
  `t.Fatal`s if reached → `PermissionDenied`), `_NoTenant` (`Unauthenticated`),
  `_InvalidID` (`InvalidArgument`); `SettlePettyCash` adds a bad-`unspent_amount`
  → `InvalidArgument` case.
- **Service tests** for the status transitions + the guards (already-approved →
  `ErrAlreadyApproved`, etc.), reusing the existing `mockRepo` fn-fields
  (`updateExpenseFn`/`updatePOFn`/`updatePettyCashFn`) + the new `rejectExpenseFn`.
- **`genPONumber`**: a create-with-empty-po_number test asserts a non-empty
  generated number reaches the repo (via a recording mock).
- `buf generate proto` clean; grep the generated `.pb.gw.go` for the 3 routes;
  `go build ./...` + `go vet ./...` + `go test ./services/expense/... -race`.
- **No Kong change** — the 3 sub-paths fall under the existing `/api/v1/expenses`,
  `/api/v1/purchase-orders`, `/api/v1/petty-cash` routes (expense:9082).

## Non-goals

- No approval-limit / dual-approval workflow for PO approve (status flip only).
- No new NATS events for reject/settle (none defined; out of scope).
- No `rejection_reason` surfaced in the proto `Expense` response — stored for
  audit, not returned (a UI-display follow-up if wanted).
- Not #192 (inventory) — separate slice.

## Review weight

Touches `expense` + a migration + proto/generated code → standard 2 approvals.
Whole-branch review on the most capable model.
