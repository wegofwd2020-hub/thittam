# Guard Approve* to the approvable source state (#202) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-26
**Issue:** #202 (Approve* allows approving an already-rejected record) — split from #200's review
**Branch:** `fix/expense-approve-state-guard-202` off `main`
**Migration:** none · **Proto:** none · **sqlc:** none

## Goal

`ApproveExpense`/`ApprovePurchaseOrder` guard only against re-approving a record already
in status `approved`. A `rejected` record (and, same class, a `paid` expense or a
`cancelled`/`closed`/`partially_invoiced` PO) can still be approved. Replace the single
`status == "approved"` check in each method with a **whitelist of the one approvable source
state**, so approval is a valid state transition only from the awaiting-approval state.

## Context (grounding facts, `main` @ a95bf4b)

- **Expense statuses** (`models.go:43`): `draft, submitted, approved, rejected, paid`.
  `ApproveExpense` (`service.go:80-82`) only does `if exp.Status == "approved" { return ErrAlreadyApproved }`
  — so `rejected`, `paid`, and `draft` all fall through to approval.
- **PurchaseOrder statuses** (`models.go:23`): `draft, approved, partially_invoiced, closed, cancelled`.
  **No `rejected` status** (there is no `RejectPurchaseOrder`). `ApprovePurchaseOrder`
  (`service.go:199-201`) only guards `approved`.
- `ErrAlreadyApproved`/`ErrAlreadyRejected` already exist (`errors.go:8-9`). `grpcErr`
  (`handler.go:643`) already maps both (and `ErrAlreadySettled`) → `codes.FailedPrecondition`.
- **The whitelist is behavior-preserving for real flows** — verified nothing approves from a
  non-approvable state: every expense approve path/test/e2e starts from `submitted`
  (`e2e/critical_path/critical_path_test.go:137,150`; all `service_test.go` approve tests use
  `submitted`), and every PO approve path/test starts from `draft`
  (`handler_test.go:141`, `service_test.go` `getPOFn` returns `draft`).

## Design

### 1. New error (`services/expense/errors.go`)

Add to the `var (...)` block:
```go
	ErrNotApprovable = errors.New("expense: record is not in an approvable state")
```

### 2. `ApproveExpense` (`services/expense/service.go`)

Replace the `if exp.Status == "approved" { return ErrAlreadyApproved }` block with a state
switch (expense is approvable only from `submitted`):
```go
	switch exp.Status {
	case "approved":
		return ErrAlreadyApproved
	case "rejected":
		return ErrAlreadyRejected
	case "submitted":
		// approvable — proceed
	default:
		return ErrNotApprovable // draft, paid, or any unknown/terminal status
	}
```
Everything after (limit check, dual-approval, set approved) unchanged.

### 3. `ApprovePurchaseOrder` (`services/expense/service.go`)

Replace the `if po.Status == "approved" { return ErrAlreadyApproved }` block (PO has no
`rejected` status; approvable only from `draft`):
```go
	switch po.Status {
	case "approved":
		return ErrAlreadyApproved
	case "draft":
		// approvable — proceed
	default:
		return ErrNotApprovable // partially_invoiced, closed, cancelled, or unknown
	}
```
Everything after unchanged.

### 4. `grpcErr` (`services/expense/handler.go`)

Add `ErrNotApprovable` to the `FailedPrecondition` group, or a dedicated arm with a clearer
message:
```go
	case errors.Is(err, ErrNotApprovable):
		return status.Error(codes.FailedPrecondition, "record is not in an approvable state")
```
(Placed alongside the existing `ErrAlreadyApproved`/`ErrAlreadyRejected` arm at `handler.go:643`.)

## Testing (`services/expense/service_test.go`, `handler_test.go`)

- **ApproveExpense**: `rejected` → `ErrAlreadyRejected`; `paid` → `ErrNotApprovable`;
  `draft` → `ErrNotApprovable`; `submitted` still succeeds (existing tests); `approved` →
  `ErrAlreadyApproved` (existing).
- **ApprovePurchaseOrder**: `cancelled` (or `closed`) → `ErrNotApprovable`; `draft` still
  succeeds (existing); `approved` → `ErrAlreadyApproved` (existing).
- **Handler** (optional, thin): one arm mapping `ErrNotApprovable` → `FailedPrecondition`
  (e.g. reject-then-approve through the handler, or a direct grpcErr assertion if the suite
  has that style).
- Gates: `go test ./services/expense/... -race`; `go vet ./...`; `go build ./...`;
  `gofmt -l` on touched Go files (empty). No proto/sqlc/migration → no codegen gates. No
  signature change → no whole-tree ripple (but `go vet ./...` is cheap and run anyway).

## Non-goals

- No resubmit / un-reject state transition (a rejected record stays rejected; re-approval
  requires a separate future flow).
- No `RejectPurchaseOrder` RPC (POs have no rejected status; out of scope).
- No migration, proto, sqlc, or handler signature change.
- No change to the approval limit / dual-approval / attribution logic.

## Review weight

Money-decision **authorization** state guard → security-sensitive; senior engineer required
per CLAUDE.md. Standard 2 approvals + senior. Small diff; whole-branch review on the most
capable model.
