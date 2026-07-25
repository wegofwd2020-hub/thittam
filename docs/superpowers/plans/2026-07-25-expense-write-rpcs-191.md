# Expense Write RPCs — Reject / PO-Approve / Settle (#191) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the RejectExpense / ApprovePurchaseOrder / SettlePettyCash RPCs (+ handlers, service logic, one migration) and fix the CreatePurchaseOrder po_number collision, so the expense UI's three 404'ing endpoints work through the existing gateway.

**Architecture:** One migration adds the expense rejection columns; the reject data layer, the proto RPCs, the service methods, and the handlers land as four layered tasks copying the existing `ApproveExpense` end-to-end pattern. PO-approve and settle reuse existing SQL/repo; only reject needs new schema/SQL.

**Tech Stack:** Go 1.25, protobuf + buf 1.32.0, sqlc, Postgres, `pkg/server` gateway (already wired for expense at :9082).

## Global Constraints

- **Copy the `ApproveExpense` chain** (handler.go:272 → service.go:64 → repo `UpdateExpense`→`UpdateExpenseStatus`): handler = tenant → `RequirePermission` → parse IDs → `svc` call → re-fetch → `<x>ToProto` → `grpcErr`.
- **Permissions:** RejectExpense + ApprovePurchaseOrder → `expense:approve`; SettlePettyCash → `expense:submit`. (These three strings are the only expense perms in use; no `expense:write` exists.)
- **PO-approve is a simple status flip** — NO approval-limit / dual-approval logic (that lives only in `ApproveExpense`).
- **Settle** → `Status="settled"`, `UnspentAmount=<input>`, `SettledAt=now`.
- **po_number** — generated server-side in `Service.CreatePurchaseOrder` when the request's is empty; a supplied number passes through.
- **buf:** `buf generate proto` (no root buf.work.yaml); adding RPCs+messages is not breaking; scope the gen commit to `gen/expense/v1/` (revert cross-service drift). **sqlc:** `sqlc generate`; scope to `services/expense/db/`.
- **No Kong change** — the 3 sub-paths fall under the existing `/api/v1/expenses`, `/api/v1/purchase-orders`, `/api/v1/petty-cash` routes. LOCAL DB SAFETY: never `docker compose -v`/`down`/`up` on `infra/local/`.
- **Commits:** Conventional Commits, scope `expense`; end every message with `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `migrations/expense/002_add_expense_rejection.{up,down}.sql` | rejection_reason + rejected_at columns | 1 |
| `services/expense/db/queries.sql` + regenerated `.go` | `RejectExpense` query | 1 |
| `services/expense/models.go` | `Expense.RejectionReason`/`RejectedAt` | 1 |
| `services/expense/repository.go` + `db/postgres.go` | `RejectExpense` repo method | 1 |
| `proto/thittam/expense/v1/expense.proto` + `gen/expense/v1/*` | 3 RPCs + messages + annotations | 2 |
| `services/expense/service.go` + `errors.go` + `service_test.go` | 3 service methods + `ErrAlreadyRejected` + `genPONumber` + po_number fix | 3 |
| `services/expense/handler.go` + `handler_test.go` | 3 handler methods + grpcErr mapping | 4 |

---

### Task 1: Migration + reject data layer

**Files:**
- Create: `migrations/expense/002_add_expense_rejection.up.sql` / `.down.sql`
- Modify: `services/expense/db/queries.sql` (+ regenerated `queries.sql.go`)
- Modify: `services/expense/models.go`
- Modify: `services/expense/repository.go`, `services/expense/db/postgres.go`

**Interfaces:**
- Produces: `Repository.RejectExpense(ctx context.Context, tenantID, expenseID uuid.UUID, reason string) error` (Task 3 consumes it); `Expense.RejectionReason *string`, `Expense.RejectedAt *time.Time`.

- [ ] **Step 1: Write the migration**

`migrations/expense/002_add_expense_rejection.up.sql`:
```sql
-- 002_add_expense_rejection.up.sql
-- RejectExpense (#191) records why an expense was rejected. status='rejected'
-- is already in the CHECK; only the reason + timestamp were missing.
ALTER TABLE expenses
    ADD COLUMN rejection_reason TEXT,
    ADD COLUMN rejected_at      TIMESTAMPTZ;
```
`002_add_expense_rejection.down.sql`:
```sql
ALTER TABLE expenses
    DROP COLUMN IF EXISTS rejected_at,
    DROP COLUMN IF EXISTS rejection_reason;
```

- [ ] **Step 2: Add the RejectExpense SQL query**

Append to `services/expense/db/queries.sql`:
```sql
-- name: RejectExpense :one
UPDATE expenses
SET status = 'rejected', rejection_reason = $3, rejected_at = now()
WHERE id = $1 AND tenant_id = $2
RETURNING *;
```

- [ ] **Step 3: Regenerate sqlc + confirm scope**

Run from repo root:
```bash
sqlc generate
git status --porcelain services/ | grep -v 'services/expense/db/' || echo "SCOPED OK"
```
Expected: only `services/expense/db/` changed (RETURNING * refreshes the `Expense` row struct with the two new columns; `RejectExpenseParams` gains `RejectionReason pgtype.Text`). If any OTHER service's db gen changed (pre-existing drift), revert it: `git checkout -- $(git status --porcelain services/*/db/ | grep -v 'services/expense/db/' | awk '{print $2}')`.

- [ ] **Step 4: Add the domain fields**

In `services/expense/models.go`, add to the `Expense` struct (after `CreatedAt`, matching the pointer style of `ApprovedAt`):
```go
	RejectionReason *string    `json:"rejection_reason,omitempty"`
	RejectedAt      *time.Time `json:"rejected_at,omitempty"`
```

- [ ] **Step 5: Add the repo method**

In `services/expense/repository.go`, add to the `// Expenses` block:
```go
	RejectExpense(ctx context.Context, tenantID, expenseID uuid.UUID, reason string) error
```
In `services/expense/db/postgres.go`, implement it (mirror `UpdateExpense`'s error-wrap style; `q.RejectExpense` is the generated method):
```go
func (p *Postgres) RejectExpense(ctx context.Context, tenantID, expenseID uuid.UUID, reason string) error {
	_, err := p.q.RejectExpense(ctx, RejectExpenseParams{
		ID:              expenseID,
		TenantID:        tenantID,
		RejectionReason: pgtype.Text{String: reason, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("expense/db: reject expense: %w", err)
	}
	return nil
}
```
(`pgtype` and `fmt` are already imported in `postgres.go`. Confirm the generated `RejectExpenseParams` field name for the reason is `RejectionReason` and its type is `pgtype.Text`; match it.)

- [ ] **Step 6: Build**

```bash
go build ./services/expense/...
```
Expected: exit 0.

- [ ] **Step 7: Commit**

```bash
git add migrations/expense/002_add_expense_rejection.up.sql migrations/expense/002_add_expense_rejection.down.sql services/expense/db services/expense/models.go services/expense/repository.go
git commit -m "feat(expense): add reject data layer — migration + RejectExpense query/repo (#191)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Proto — 3 RPCs + annotations

**Files:**
- Modify: `proto/thittam/expense/v1/expense.proto`
- Modify (regenerated): `gen/expense/v1/*`

**Interfaces:**
- Produces: `RejectExpenseRequest{expense_id, reason}`, `ApprovePurchaseOrderRequest{id}`, `SettlePettyCashRequest{id, unspent_amount}`, and the handler-side methods on `expensev1.ExpenseServiceServer` for Task 4.

- [ ] **Step 1: Add the 3 RPCs**

In `expense.proto`'s `service ExpenseService` block, add (annotations import is already present from C2):
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

- [ ] **Step 2: Add the request messages**

```proto
message RejectExpenseRequest { string expense_id = 1; string reason = 2; }
message ApprovePurchaseOrderRequest { string id = 1; }
message SettlePettyCashRequest { string id = 1; string unspent_amount = 2; }
```

- [ ] **Step 3: Regenerate + scope + confirm routes**

```bash
buf generate proto
git checkout -- $(git status --porcelain gen/ | grep -v 'gen/expense/' | awk '{print $2}')  # revert unrelated drift
grep -oE '/api/v1/(expenses/\{expense_id\}/reject|purchase-orders/\{id\}/approve|petty-cash/\{id\}/settle)' gen/expense/v1/expense.pb.gw.go | sort -u
go build ./...
```
Expected: the 3 route patterns appear; `go build` exits 0 (the handler is now `Unimplemented` for the 3 new methods until Task 4 — that still compiles, since `Handler` embeds `UnimplementedExpenseServiceServer`).

- [ ] **Step 4: Commit**

```bash
git add proto/thittam/expense/v1/expense.proto gen/expense/v1
git commit -m "feat(expense): add reject/PO-approve/settle RPCs + REST annotations (#191)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Service methods + po_number fix

**Files:**
- Modify: `services/expense/service.go`
- Modify: `services/expense/errors.go`
- Modify: `services/expense/service_test.go`

**Interfaces:**
- Consumes: `Repository.RejectExpense` (Task 1), the existing `UpdatePurchaseOrder`/`UpdatePettyCashAdvance`/`GetExpense`/`GetPurchaseOrder`/`GetPettyCashAdvance`.
- Produces: `Service.RejectExpense(ctx, tenantID, expenseID uuid.UUID, reason string) error`, `Service.ApprovePurchaseOrder(ctx, tenantID, poID, approverID uuid.UUID) error`, `Service.SettlePettyCash(ctx, tenantID, advanceID uuid.UUID, unspent decimal.Decimal) error`, `ErrAlreadyRejected`.

- [ ] **Step 1: Add `ErrAlreadyRejected`**

In `services/expense/errors.go`, add to the `var (...)` block:
```go
	ErrAlreadyRejected = errors.New("expense: expense already rejected")
```

- [ ] **Step 2: Add the three service methods + genPONumber**

Append to `services/expense/service.go` (it imports `context`, `fmt`, `time`, `uuid`; add `"strings"` and `"github.com/shopspring/decimal"` to the import block):
```go
func (s *Service) RejectExpense(ctx context.Context, tenantID, expenseID uuid.UUID, reason string) error {
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
	return s.repo.RejectExpense(ctx, tenantID, expenseID, reason)
}

func (s *Service) ApprovePurchaseOrder(ctx context.Context, tenantID, poID, approverID uuid.UUID) error {
	po, err := s.repo.GetPurchaseOrder(ctx, tenantID, poID)
	if err != nil {
		return fmt.Errorf("get purchase order: %w", err)
	}
	if po.Status == "approved" {
		return ErrAlreadyApproved
	}
	now := time.Now()
	po.Status = "approved"
	po.ApprovedBy = &approverID
	po.ApprovedAt = &now
	return s.repo.UpdatePurchaseOrder(ctx, po)
}

func (s *Service) SettlePettyCash(ctx context.Context, tenantID, advanceID uuid.UUID, unspent decimal.Decimal) error {
	pc, err := s.repo.GetPettyCashAdvance(ctx, tenantID, advanceID)
	if err != nil {
		return fmt.Errorf("get petty cash advance: %w", err)
	}
	now := time.Now()
	pc.Status = "settled"
	pc.UnspentAmount = unspent
	pc.SettledAt = &now
	return s.repo.UpdatePettyCashAdvance(ctx, pc)
}

// genPONumber produces a unique, human-readable PO number for the common case
// where the UI does not collect one (po_number is NOT NULL UNIQUE, so an empty
// string would collide on the second create).
func genPONumber() string {
	return "PO-" + time.Now().UTC().Format("2006") + "-" + strings.ToUpper(uuid.NewString()[:8])
}
```

- [ ] **Step 3: Fill po_number in `Service.CreatePurchaseOrder`**

In the existing `Service.CreatePurchaseOrder`, before the `repo.CreatePurchaseOrder` call, add:
```go
	if po.PONumber == "" {
		po.PONumber = genPONumber()
	}
```
(Find the method — it's the one the `CreatePurchaseOrder` handler calls as `h.svc.CreatePurchaseOrder(ctx, po)`. If it currently just delegates to the repo, insert the guard at its top.)

- [ ] **Step 4: Write the service tests**

Add to `services/expense/service_test.go` (the `mockRepo` already has `updatePOFn`/`updatePettyCashFn`; add a `rejectExpenseFn func(ctx context.Context, tenantID, expenseID uuid.UUID, reason string) error` field + its method to the mock, defaulting to `return nil`):
```go
func TestService_RejectExpense_AlreadyApproved(t *testing.T) {
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, Status: "approved"}, nil
		},
	})
	err := svc.RejectExpense(context.Background(), uuid.New(), uuid.New(), "dup")
	require.ErrorIs(t, err, ErrAlreadyApproved)
}

func TestService_RejectExpense_Success(t *testing.T) {
	var gotReason string
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			return &Expense{ID: id, TenantID: tid, Status: "submitted"}, nil
		},
		rejectExpenseFn: func(_ context.Context, _, _ uuid.UUID, reason string) error {
			gotReason = reason
			return nil
		},
	})
	require.NoError(t, svc.RejectExpense(context.Background(), uuid.New(), uuid.New(), "over budget"))
	assert.Equal(t, "over budget", gotReason)
}

func TestService_ApprovePurchaseOrder_SetsApproved(t *testing.T) {
	var saved *PurchaseOrder
	svc := NewService(&mockRepo{
		getPOFn:    func(_ context.Context, tid, id uuid.UUID) (*PurchaseOrder, error) { return &PurchaseOrder{ID: id, TenantID: tid, Status: "draft"}, nil },
		updatePOFn: func(_ context.Context, po *PurchaseOrder) error { saved = po; return nil },
	})
	require.NoError(t, svc.ApprovePurchaseOrder(context.Background(), uuid.New(), uuid.New(), uuid.New()))
	assert.Equal(t, "approved", saved.Status)
	require.NotNil(t, saved.ApprovedAt)
}

func TestService_ApprovePurchaseOrder_AlreadyApproved(t *testing.T) {
	svc := NewService(&mockRepo{
		getPOFn: func(_ context.Context, tid, id uuid.UUID) (*PurchaseOrder, error) { return &PurchaseOrder{ID: id, TenantID: tid, Status: "approved"}, nil },
	})
	assert.ErrorIs(t, svc.ApprovePurchaseOrder(context.Background(), uuid.New(), uuid.New(), uuid.New()), ErrAlreadyApproved)
}

func TestService_SettlePettyCash_SetsSettled(t *testing.T) {
	var saved *PettyCashAdvance
	svc := NewService(&mockRepo{
		getPettyCashFn:    func(_ context.Context, tid, id uuid.UUID) (*PettyCashAdvance, error) { return &PettyCashAdvance{ID: id, TenantID: tid, Status: "issued"}, nil },
		updatePettyCashFn: func(_ context.Context, pc *PettyCashAdvance) error { saved = pc; return nil },
	})
	require.NoError(t, svc.SettlePettyCash(context.Background(), uuid.New(), uuid.New(), decimal.RequireFromString("12.50")))
	assert.Equal(t, "settled", saved.Status)
	assert.Equal(t, "12.50", saved.UnspentAmount.StringFixed(2))
	require.NotNil(t, saved.SettledAt)
}

func TestService_CreatePurchaseOrder_GeneratesPONumberWhenEmpty(t *testing.T) {
	var saved *PurchaseOrder
	svc := NewService(&mockRepo{
		createPOFn: func(_ context.Context, po *PurchaseOrder) error { saved = po; return nil },
	})
	po := &PurchaseOrder{TenantID: uuid.New(), ProductionID: uuid.New(), Amount: decimal.NewFromInt(100)}
	require.NoError(t, svc.CreatePurchaseOrder(context.Background(), po))
	assert.NotEmpty(t, saved.PONumber)
	assert.True(t, strings.HasPrefix(saved.PONumber, "PO-"))
}
```
(Add `"strings"` and `"github.com/shopspring/decimal"` to the test imports if not present. If `NewService` needs more args than `repo` — e.g. a publisher — pass `nil` as the existing tests do.)

- [ ] **Step 5: Run the service suite**

```bash
go test ./services/expense/ -race
```
Expected: pass, including the 6 new tests.

- [ ] **Step 6: Commit**

```bash
git add services/expense/service.go services/expense/errors.go services/expense/service_test.go
git commit -m "feat(expense): reject/PO-approve/settle service logic + po_number generation (#191)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 4: Handlers

**Files:**
- Modify: `services/expense/handler.go`
- Modify: `services/expense/handler_test.go`

**Interfaces:**
- Consumes: Task 2's request types; Task 3's `Service` methods + `ErrAlreadyRejected`.

`handler.go` already imports everything needed (`decimal`, `uuid`, `expensev1`, `interceptor`, `tenant`, `codes`, `status`, `errors`).

- [ ] **Step 1: Map `ErrAlreadyRejected` in `grpcErr`**

In `services/expense/handler.go`'s `grpcErr` function, add `ErrAlreadyRejected` to the same `case` arm that already handles `ErrAlreadyApproved` (so both map to the same gRPC code):
```go
	case errors.Is(err, ErrAlreadyApproved), errors.Is(err, ErrAlreadyRejected):
		// (keep the existing return for this arm)
```

- [ ] **Step 2: Add the three handler methods**

Append to `handler.go` (copying the `ApproveExpense` shape):
```go
func (h *Handler) RejectExpense(ctx context.Context, req *expensev1.RejectExpenseRequest) (*expensev1.Expense, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}
	if err := interceptor.RequirePermission(ctx, h.perm, "expense:approve"); err != nil {
		return nil, err
	}
	expenseID, err := uuid.Parse(req.GetExpenseId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid expense_id")
	}
	if err := h.svc.RejectExpense(ctx, tenantID, expenseID, req.GetReason()); err != nil {
		return nil, grpcErr(err)
	}
	e, err := h.svc.GetExpense(ctx, tenantID, expenseID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return expenseToProto(e), nil
}

func (h *Handler) ApprovePurchaseOrder(ctx context.Context, req *expensev1.ApprovePurchaseOrderRequest) (*expensev1.PurchaseOrder, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}
	if err := interceptor.RequirePermission(ctx, h.perm, "expense:approve"); err != nil {
		return nil, err
	}
	poID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid id")
	}
	if err := h.svc.ApprovePurchaseOrder(ctx, tenantID, poID, uuid.Nil); err != nil {
		return nil, grpcErr(err)
	}
	po, err := h.svc.GetPurchaseOrder(ctx, tenantID, poID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return purchaseOrderToProto(po), nil
}

func (h *Handler) SettlePettyCash(ctx context.Context, req *expensev1.SettlePettyCashRequest) (*expensev1.PettyCashAdvance, error) {
	tenantID, ok := tenant.IDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "tenant ID not found in context")
	}
	if err := interceptor.RequirePermission(ctx, h.perm, "expense:submit"); err != nil {
		return nil, err
	}
	advanceID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid id")
	}
	unspent, err := decimal.NewFromString(req.GetUnspentAmount())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid unspent_amount: must be a decimal string")
	}
	if err := h.svc.SettlePettyCash(ctx, tenantID, advanceID, unspent); err != nil {
		return nil, grpcErr(err)
	}
	pc, err := h.svc.GetPettyCashAdvance(ctx, tenantID, advanceID)
	if err != nil {
		return nil, grpcErr(err)
	}
	return pettyCashAdvanceToProto(pc), nil
}
```
(Confirm the proto converter names — `expenseToProto`, `purchaseOrderToProto`, `pettyCashAdvanceToProto` — against the existing converters in `handler.go`; the `ApproveExpense`/`CreatePurchaseOrder` handlers already call `expenseToProto`/`purchaseOrderToProto`. Match the petty-cash converter's exact name.)

- [ ] **Step 3: Write the handler tests**

Add to `handler_test.go`, per RPC, copying the `ApproveExpense` trio (`_Success`/`_Denied`/`_NoTenant`/`_InvalidID`). Use the existing `ctxWithTenant`/`ctxWithVertical`/`allowAllPerm`/`denyPerm`/`newHandler` helpers. Representative (reject); write the analogous set for ApprovePurchaseOrder and SettlePettyCash:
```go
func TestHandler_RejectExpense_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	expID := uuid.New()
	callCount := 0
	h := NewHandler(NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			callCount++
			st := "submitted"
			if callCount > 1 {
				st = "rejected"
			}
			return &Expense{ID: id, TenantID: tid, Status: st, Amount: decimal.NewFromInt(5000)}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})
	resp, err := h.RejectExpense(ctxWithTenant(tenantID), &expensev1.RejectExpenseRequest{ExpenseId: expID.String(), Reason: "over budget"})
	require.NoError(t, err)
	assert.Equal(t, "rejected", resp.GetStatus())
}

func TestHandler_RejectExpense_Denied(t *testing.T) {
	t.Parallel()
	h := NewHandler(NewService(&mockRepo{
		getExpenseFn: func(context.Context, uuid.UUID, uuid.UUID) (*Expense, error) {
			t.Fatal("repo must not be reached when permission is denied")
			return nil, nil
		},
	})).WithPermissionChecker(denyPerm{})
	_, err := h.RejectExpense(ctxWithTenant(uuid.New()), &expensev1.RejectExpenseRequest{ExpenseId: uuid.New().String(), Reason: "x"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_RejectExpense_NoTenant(t *testing.T) {
	t.Parallel()
	_, err := newHandler().RejectExpense(ctxWithVertical(), &expensev1.RejectExpenseRequest{ExpenseId: uuid.New().String(), Reason: "x"})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestHandler_RejectExpense_InvalidID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().RejectExpense(ctxWithTenant(uuid.New()), &expensev1.RejectExpenseRequest{ExpenseId: "bad", Reason: "x"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}
```
For **ApprovePurchaseOrder** the `_Success` uses `getPOFn` returning `Status:"draft"` first then `"approved"` (two-call like reject) or asserts `GetStatus()=="approved"` on a single re-fetch stubbed as approved; `_InvalidID` sends `Id:"bad"`. For **SettlePettyCash** add a `_InvalidAmount` test (`UnspentAmount:"abc"` → `InvalidArgument`) and `_Success` (`getPettyCashFn` returns `Status:"issued"` then a settled one). Follow the exact structure above; only the method, request type, mock fn, permission, and asserted status differ.

- [ ] **Step 4: Run + build the tree**

```bash
go test ./services/expense/... -race && go build ./... && go vet ./services/expense/
```
Expected: all pass / exit 0.

- [ ] **Step 5: Commit**

```bash
git add services/expense/handler.go services/expense/handler_test.go
git commit -m "feat(expense): wire reject/PO-approve/settle handlers + tests (#191)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- Migration `expense/002` (rejection_reason + rejected_at) → Task 1 ✅
- RejectExpense SQL/repo/model → Task 1; service → Task 3; handler → Task 4 ✅
- ApprovePurchaseOrder (reuse UpdatePurchaseOrder) service → Task 3; handler → Task 4 ✅
- SettlePettyCash (reuse UpdatePettyCashAdvance) service → Task 3; handler → Task 4 ✅
- 3 proto RPCs + messages + annotations + gen → Task 2 ✅
- po_number server-generate → Task 3 (Service.CreatePurchaseOrder + genPONumber) ✅
- Permissions expense:approve/approve/submit → Task 4 handlers ✅
- Tests (service + handler trios + po_number) → Tasks 3–4 ✅
- No approval-limit logic for PO, no Kong change, no new events → honored ✅

**Placeholder scan:** none — full migration, SQL, proto, service, handler, and test code. The few "confirm the generated field/converter name" notes are concrete compiler-checked steps, not placeholders.

**Type consistency:** `Repository.RejectExpense(ctx, tenantID, expenseID uuid.UUID, reason string)` (Task 1) == the `Service.RejectExpense` call (Task 3) == the handler call (Task 4). `Service.ApprovePurchaseOrder(…, approverID uuid.UUID)` / `SettlePettyCash(…, unspent decimal.Decimal)` match their handler call sites (`uuid.Nil`, parsed `decimal`). Proto request field names (`expense_id`/`reason`, `id`, `id`/`unspent_amount`) match the handler `req.Get…()` calls and the annotation placeholders (`{expense_id}`, `{id}`).

**Ordering:** Task 1 (data layer) & Task 2 (proto) are independent; Task 3 (service) needs Task 1's repo method; Task 4 (handlers) needs Task 2's proto types + Task 3's service methods. Each task builds.
