# Approve* state guard (#202) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Block approving an expense/PO that is not in its awaiting-approval state — replace the single `status == "approved"` guard in `ApproveExpense`/`ApprovePurchaseOrder` with a whitelist (expense approvable only from `submitted`, PO only from `draft`), closing #202 and the sibling `paid`/`cancelled`/`closed` re-approve bugs.

**Architecture:** Service-only. Two `switch` guards + one new `ErrNotApprovable` error + one `grpcErr` arm. No migration/proto/sqlc, no signature change.

**Tech Stack:** Go 1.25, `services/expense`, testify.

## Global Constraints

- **Expense approvable ONLY from `submitted`; PO approvable ONLY from `draft`.** `approved`→`ErrAlreadyApproved`; expense `rejected`→`ErrAlreadyRejected` (PO has no rejected status); everything else (expense `draft`/`paid`, PO `partially_invoiced`/`closed`/`cancelled`, any unknown)→`ErrNotApprovable`.
- **Behavior-preserving for real flows** — verified nothing approves from a non-whitelisted state (expense always `submitted`, PO always `draft` in every test/e2e). Do NOT change the approve-success path.
- `ErrAlreadyApproved`/`ErrAlreadyRejected` already exist and `grpcErr` already maps them → `FailedPrecondition`; reuse, don't redefine. Add `ErrNotApprovable` → `FailedPrecondition`.
- No migration, proto, sqlc, or signature change. Money/limit/dual-approval/attribution logic untouched.
- Gate: `go test ./services/expense/... -race && go vet ./... && go build ./...` + `gofmt -l <touched .go files>` (empty).
- **Security-sensitive** (money-approval state guard) → senior review per CLAUDE.md.
- Commit Conventional-Commits (scope `expense`), ending `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

---

## File Structure

| File | Responsibility |
|---|---|
| `services/expense/errors.go` | `ErrNotApprovable` |
| `services/expense/service.go` | `ApproveExpense` + `ApprovePurchaseOrder` state whitelist |
| `services/expense/handler.go` | `grpcErr` arm for `ErrNotApprovable` |
| `services/expense/service_test.go` | state-guard tests |
| `services/expense/handler_test.go` | one grpcErr-mapping test |

---

### Task 1: State-whitelist guard for Approve* + ErrNotApprovable

**Files:** Modify `services/expense/errors.go`, `service.go`, `handler.go`, `service_test.go`, `handler_test.go`

**Interfaces:**
- Produces: `ErrNotApprovable`; unchanged `ApproveExpense`/`ApprovePurchaseOrder` signatures.

- [ ] **Step 1: Write failing service tests**

Add to `services/expense/service_test.go` (use the existing `ctxWithVertical()` + `mockRepo` `getExpenseFn`/`getPOFn` patterns already in this file; `movieProductionConfig()` gives `manager` a 1M limit so `[]string{"manager"}` passes the limit check and the test reaches — or is blocked before — the state guard):
```go
func TestService_ApproveExpense_RejectedNotApprovable(t *testing.T) {
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, Status: "rejected", Amount: decimal.NewFromInt(5000)}, nil
		},
	})
	err := svc.ApproveExpense(ctxWithVertical(), uuid.New(), uuid.New(), uuid.New(), []string{"manager"})
	assert.ErrorIs(t, err, ErrAlreadyRejected)
}

func TestService_ApproveExpense_PaidNotApprovable(t *testing.T) {
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, Status: "paid", Amount: decimal.NewFromInt(5000)}, nil
		},
	})
	err := svc.ApproveExpense(ctxWithVertical(), uuid.New(), uuid.New(), uuid.New(), []string{"manager"})
	assert.ErrorIs(t, err, ErrNotApprovable)
}

func TestService_ApproveExpense_DraftNotApprovable(t *testing.T) {
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, Status: "draft", Amount: decimal.NewFromInt(5000)}, nil
		},
	})
	err := svc.ApproveExpense(ctxWithVertical(), uuid.New(), uuid.New(), uuid.New(), []string{"manager"})
	assert.ErrorIs(t, err, ErrNotApprovable)
}

func TestService_ApprovePurchaseOrder_CancelledNotApprovable(t *testing.T) {
	svc := NewService(&mockRepo{
		getPOFn: func(_ context.Context, tid, id uuid.UUID) (*PurchaseOrder, error) {
			return &PurchaseOrder{ID: id, TenantID: tid, Status: "cancelled", Amount: decimal.NewFromInt(5000)}, nil
		},
	})
	err := svc.ApprovePurchaseOrder(ctxWithVertical(), uuid.New(), uuid.New(), uuid.New(), []string{"manager"})
	assert.ErrorIs(t, err, ErrNotApprovable)
}
```
(These assert the guard fires BEFORE the limit/dual checks — the switch is the first thing after the fetch, so `manager` role vs amount is irrelevant here. Keep the existing `_AlreadyApproved` / success / `_WithinLimit` tests unchanged — they use `submitted`/`draft`/`approved` and must stay green.)

- [ ] **Step 2: Run — expect FAIL**

`go test ./services/expense/ -run 'TestService_ApproveExpense_Rejected|TestService_ApproveExpense_Paid|TestService_ApproveExpense_Draft|TestService_ApprovePurchaseOrder_Cancelled'`
Expected: FAIL — today `rejected`/`paid`/`draft` expenses and `cancelled` POs fall through the single `approved` guard (and either approve or hit a limit/dual error), and `ErrNotApprovable` is undefined.

- [ ] **Step 3: Add the error**

In `services/expense/errors.go`, add to the `var (...)` block (after `ErrNotAdvanceHolder`):
```go
	ErrNotApprovable = errors.New("expense: record is not in an approvable state")
```

- [ ] **Step 4: Replace the guards**

In `services/expense/service.go` `ApproveExpense`, replace:
```go
	if exp.Status == "approved" {
		return ErrAlreadyApproved
	}
```
with:
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
In `ApprovePurchaseOrder`, replace:
```go
	if po.Status == "approved" {
		return ErrAlreadyApproved
	}
```
with (PO has no `rejected` status; approvable only from `draft`):
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

- [ ] **Step 5: Map the error in grpcErr**

In `services/expense/handler.go` `grpcErr`, add an arm next to the existing `ErrAlreadyApproved`/`ErrAlreadyRejected` case (`~:643`):
```go
	case errors.Is(err, ErrNotApprovable):
		return status.Error(codes.FailedPrecondition, "record is not in an approvable state")
```

- [ ] **Step 6: Handler grpcErr-mapping test**

Add to `services/expense/handler_test.go` (mirror the existing `TestHandler_ApproveExpense_*` style — `ctxWithCaller` with a role that has a limit so it passes the caller+limit gates and reaches the service, which returns `ErrNotApprovable` for a `paid` expense):
```go
func TestHandler_ApproveExpense_PaidNotApprovable(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	caller := interceptor.CallerInfo{UserID: uuid.New(), TenantID: tenantID, Roles: []string{"manager"}}
	h := NewHandler(NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, Status: "paid", Amount: decimal.NewFromInt(5000)}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})
	_, err := h.ApproveExpense(ctxWithCaller(caller), &expensev1.ApproveExpenseRequest{ExpenseId: uuid.New().String()})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}
```
(Confirm the `ApproveExpenseRequest` field name is `ExpenseId` and `ctxWithCaller`/`allowAllPerm` exist — they do, from #196/#197. Adjust to the real request field if it differs.)

- [ ] **Step 7: Run — expect PASS + gate**
```bash
go test ./services/expense/ -race && go vet ./... && go build ./...
gofmt -l services/expense/errors.go services/expense/service.go services/expense/handler.go services/expense/service_test.go services/expense/handler_test.go
```
All green; `gofmt -l` prints nothing. The pre-existing approve tests (success from `submitted`/`draft`, `_AlreadyApproved`, limit/dual) must still pass.

- [ ] **Step 8: Commit**
```bash
git add services/expense/errors.go services/expense/service.go services/expense/handler.go services/expense/service_test.go services/expense/handler_test.go
git commit -m "fix(expense): approve only from the awaiting-approval state (#202)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- `ErrNotApprovable` added → Step 3 ✅
- ApproveExpense whitelist (`submitted`; `rejected`→ErrAlreadyRejected; else ErrNotApprovable) → Step 4 ✅
- ApprovePurchaseOrder whitelist (`draft`; else ErrNotApprovable) → Step 4 ✅
- grpcErr `ErrNotApprovable`→FailedPrecondition → Step 5 ✅
- Tests: rejected/paid/draft expense + cancelled PO + handler mapping; existing success/already-approved kept → Steps 1,6 ✅
- Non-goals honored (no migration/proto/sqlc, no resubmit transition, no PO reject) ✅

**Placeholder scan:** none — full error line, both switches, grpcErr arm, and all test bodies given. The "confirm request field name" note is compiler-checked.

**Type consistency:** `ErrNotApprovable` (Step 3) used in both service switches (Step 4), grpcErr (Step 5), and tests (Steps 1,6). Signatures of `ApproveExpense`/`ApprovePurchaseOrder` UNCHANGED (no ripple). `ErrAlreadyApproved`/`ErrAlreadyRejected` reused (already mapped in grpcErr). Test helpers `ctxWithVertical`/`ctxWithCaller`/`allowAllPerm`/`mockRepo` fields all pre-exist.

**Single task:** both methods are one symmetric, small deliverable a reviewer gates together; no meaningful split point.
