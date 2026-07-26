# Expense caller-identity authz (#196/#197 + ApproveExpense fix) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the caller's real identity (`UserID` + `Roles`) into the expense handlers — fixing the broken `ApproveExpense` limit check, adding the same check to `ApprovePurchaseOrder` (#196), gating `SettlePettyCash` on advance ownership (#197), and attributing creators.

**Architecture:** `interceptor.CallerFromContext(ctx)` already carries `CallerInfo{UserID, Roles}` on every authenticated expense RPC; the handlers just don't call it. A new `pkg/vertical.MaxLimitForRoles` picks the caller's best role limit. Four vertical slices (helper, approval parity, settle ownership, attribution), each service+handler+tests together. No proto/migration/sqlc.

**Tech Stack:** Go 1.25, `pkg/interceptor` (JWT-derived caller), `pkg/vertical` (per-tenant approval config), `decimal.Decimal`.

## Global Constraints

- **Caller identity comes ONLY from `interceptor.CallerFromContext(ctx) (CallerInfo, bool)`** — never from the request. On `!ok` return `codes.Unauthenticated`. Read it AFTER the existing tenant + `RequirePermission` gates.
- **`MaxLimitForRoles(caller.Roles)`**: the max configured limit across the caller's roles; `nil` → the caller has no approval authority → `ErrApprovalLimitExceeded` (fail-closed). **No `super_admin` bypass.**
- **#197:** `caller.UserID == advance.IssuedTo`, else `ErrNotAdvanceHolder` → `codes.PermissionDenied`. Keep the `expense:submit` gate (raising to `:approve` would lock out `member`, the advance holder).
- **Money** stays `decimal.Decimal`; approval compares against `po.Amount`/`exp.Amount`.
- **Whole-tree `go vet ./...` after ANY Service signature change** — `services/expense/service.go`'s `ApproveExpense`/`ApprovePurchaseOrder`/`SettlePettyCash` are called outside the package (e.g. `e2e/critical_path/critical_path_test.go:150` calls `ApproveExpense`). `go build ./...` skips `_test.go`; only `go vet ./...` catches these. Update every caller in the same commit as the signature change.
- No proto, migration, sqlc, or Kong changes.
- **Security-sensitive** (money approval + ownership) → senior review required per CLAUDE.md.
- Commits Conventional-Commits (scope `vertical` for Task 1, else `expense`), ending `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `pkg/vertical/helpers.go` + `helpers_test.go` | `MaxLimitForRoles` | 1 |
| `services/expense/service.go` | ApproveExpense sig, ApprovePurchaseOrder logic, SettlePettyCash ownership | 2,3 |
| `services/expense/errors.go` | `ErrNotAdvanceHolder` | 3 |
| `services/expense/handler.go` | caller wiring in 5 handlers + grpcErr | 2,3,4 |
| `services/expense/{service,handler}_test.go` | tests + `ctxWithCaller`/`ctxTenantNoCaller` helpers | 2,3,4 |
| `e2e/critical_path/critical_path_test.go` | fix the `ApproveExpense` call site | 2 |

---

### Task 1: `MaxLimitForRoles` helper

**Files:** Modify `pkg/vertical/helpers.go`; Test `pkg/vertical/helpers_test.go`

**Interfaces:**
- Produces: `func (a *ApprovalWorkflow) MaxLimitForRoles(roles []string) *decimal.Decimal` (consumed by Task 2).

- [ ] **Step 1: Write the failing test**

Add to `pkg/vertical/helpers_test.go` (mirror the existing `LimitForRole` tests' construction of an `ApprovalWorkflow`):
```go
func TestMaxLimitForRoles(t *testing.T) {
	aw := &ApprovalWorkflow{Limits: []ApprovalLimit{
		{Role: "coordinator", MaxAmount: decimal.NewFromInt(200000)},
		{Role: "manager", MaxAmount: decimal.NewFromInt(1000000)},
	}}
	// max across multiple configured roles
	got := aw.MaxLimitForRoles([]string{"coordinator", "manager"})
	if got == nil || !got.Equal(decimal.NewFromInt(1000000)) {
		t.Fatalf("want 1000000, got %v", got)
	}
	// single configured role
	got = aw.MaxLimitForRoles([]string{"coordinator"})
	if got == nil || !got.Equal(decimal.NewFromInt(200000)) {
		t.Fatalf("want 200000, got %v", got)
	}
	// no configured role → nil (fail-closed)
	if aw.MaxLimitForRoles([]string{"member", "inventory_manager"}) != nil {
		t.Fatal("want nil for unconfigured roles")
	}
	// empty set → nil
	if aw.MaxLimitForRoles(nil) != nil {
		t.Fatal("want nil for empty roles")
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`MaxLimitForRoles` undefined): `go test ./pkg/vertical/ -run TestMaxLimitForRoles`

- [ ] **Step 3: Implement**

In `pkg/vertical/helpers.go` (imports already include `shopspring/decimal`), after `LimitForRole`:
```go
// MaxLimitForRoles returns the highest configured approval limit among the given
// roles, or nil if none of them has a configured limit. A caller holding several
// roles gets their most permissive entitlement; nil means no approval authority.
func (a *ApprovalWorkflow) MaxLimitForRoles(roles []string) *decimal.Decimal {
	var max *decimal.Decimal
	for _, r := range roles {
		if lim := a.LimitForRole(r); lim != nil {
			if max == nil || lim.GreaterThan(*max) {
				max = lim
			}
		}
	}
	return max
}
```

- [ ] **Step 4: Run — expect PASS**: `go test ./pkg/vertical/ -run TestMaxLimitForRoles`

- [ ] **Step 5: Commit**
```bash
git add pkg/vertical/helpers.go pkg/vertical/helpers_test.go
git commit -m "feat(vertical): add ApprovalWorkflow.MaxLimitForRoles for multi-role callers

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Approval-limit parity — fix ApproveExpense + add ApprovePurchaseOrder check (#196)

**Files:** Modify `services/expense/service.go`, `handler.go`, `service_test.go`, `handler_test.go`, `e2e/critical_path/critical_path_test.go`

**Interfaces:**
- Consumes: `MaxLimitForRoles` (Task 1); `interceptor.CallerFromContext`, `CallerInfo{UserID, Roles}`.
- Produces: `Service.ApproveExpense(ctx, tenantID, expenseID, approverID uuid.UUID, roles []string) error`; `Service.ApprovePurchaseOrder(ctx, tenantID, poID, approverID uuid.UUID, roles []string) error`; test helpers `ctxWithCaller(caller interceptor.CallerInfo) context.Context` + `ctxTenantNoCaller(tenantID uuid.UUID) context.Context`.

- [ ] **Step 1: Write failing service tests**

Add to `services/expense/service_test.go` (`movieProductionConfig()` gives coordinator=200000, manager=1000000, dual_above=1000000; `ctxWithVertical()` injects it):
```go
func TestService_ApproveExpense_SucceedsWithRoleLimit(t *testing.T) {
	// Regression: proves the P0 fix — an empty role used to always fail.
	var saved *Expense
	svc := NewService(&mockRepo{
		getExpenseFn:    func(_ context.Context, tid, id uuid.UUID) (*Expense, error) { return &Expense{ID: id, TenantID: tid, Status: "submitted", Amount: decimal.NewFromInt(5000)}, nil },
		updateExpenseFn: func(_ context.Context, e *Expense) error { saved = e; return nil },
	})
	err := svc.ApproveExpense(ctxWithVertical(), uuid.New(), uuid.New(), uuid.New(), []string{"manager"})
	require.NoError(t, err)
	assert.Equal(t, "approved", saved.Status)
}

func TestService_ApproveExpense_NoLimitRoleFails(t *testing.T) {
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) { return &Expense{ID: id, TenantID: tid, Status: "submitted", Amount: decimal.NewFromInt(5000)}, nil },
	})
	err := svc.ApproveExpense(ctxWithVertical(), uuid.New(), uuid.New(), uuid.New(), []string{"member"})
	assert.ErrorIs(t, err, ErrApprovalLimitExceeded)
}

func TestService_ApprovePurchaseOrder_WithinLimit(t *testing.T) {
	var saved *PurchaseOrder
	svc := NewService(&mockRepo{
		getPOFn:    func(_ context.Context, tid, id uuid.UUID) (*PurchaseOrder, error) { return &PurchaseOrder{ID: id, TenantID: tid, Status: "draft", Amount: decimal.NewFromInt(5000)}, nil },
		updatePOFn: func(_ context.Context, po *PurchaseOrder) error { saved = po; return nil },
	})
	require.NoError(t, svc.ApprovePurchaseOrder(ctxWithVertical(), uuid.New(), uuid.New(), uuid.New(), []string{"manager"}))
	assert.Equal(t, "approved", saved.Status)
}

func TestService_ApprovePurchaseOrder_OverLimit(t *testing.T) {
	svc := NewService(&mockRepo{
		getPOFn: func(_ context.Context, tid, id uuid.UUID) (*PurchaseOrder, error) { return &PurchaseOrder{ID: id, TenantID: tid, Status: "draft", Amount: decimal.NewFromInt(500000)}, nil },
	})
	assert.ErrorIs(t, svc.ApprovePurchaseOrder(ctxWithVertical(), uuid.New(), uuid.New(), uuid.New(), []string{"coordinator"}), ErrApprovalLimitExceeded)
}

func TestService_ApprovePurchaseOrder_DualApproval(t *testing.T) {
	svc := NewService(&mockRepo{
		getPOFn: func(_ context.Context, tid, id uuid.UUID) (*PurchaseOrder, error) { return &PurchaseOrder{ID: id, TenantID: tid, Status: "draft", Amount: decimal.NewFromInt(2000000)}, nil },
	})
	assert.ErrorIs(t, svc.ApprovePurchaseOrder(ctxWithVertical(), uuid.New(), uuid.New(), uuid.New(), []string{"manager"}), ErrDualApprovalRequired)
}

func TestService_ApprovePurchaseOrder_AlreadyApproved(t *testing.T) {
	svc := NewService(&mockRepo{
		getPOFn: func(_ context.Context, tid, id uuid.UUID) (*PurchaseOrder, error) { return &PurchaseOrder{ID: id, TenantID: tid, Status: "approved", Amount: decimal.NewFromInt(5000)}, nil },
	})
	assert.ErrorIs(t, svc.ApprovePurchaseOrder(ctxWithVertical(), uuid.New(), uuid.New(), uuid.New(), []string{"manager"}), ErrAlreadyApproved)
}
```

- [ ] **Step 2: Run — expect FAIL** (signature mismatch): `go test ./services/expense/ -run 'TestService_Approve'`

- [ ] **Step 3: Change the service methods**

In `services/expense/service.go`, change `ApproveExpense`'s signature `approverRole string` → `roles []string` and its limit line:
```go
func (s *Service) ApproveExpense(ctx context.Context, tenantID, expenseID, approverID uuid.UUID, roles []string) error {
	vcfg := vertical.MustFromContext(ctx)
	exp, err := s.repo.GetExpense(ctx, tenantID, expenseID)
	if err != nil {
		return fmt.Errorf("get expense: %w", err)
	}
	if exp.Status == "approved" {
		return ErrAlreadyApproved
	}
	limit := vcfg.ApprovalWorkflow.MaxLimitForRoles(roles)
	if limit == nil {
		return fmt.Errorf("%w: caller roles %v have no configured approval limit", ErrApprovalLimitExceeded, roles)
	}
	if exp.Amount.GreaterThan(*limit) {
		return fmt.Errorf("%w: %s exceeds %s limit", ErrApprovalLimitExceeded, exp.Amount, *limit)
	}
	if exp.Amount.GreaterThan(vcfg.ApprovalWorkflow.DualApprovalAbove) {
		return fmt.Errorf("%w: %s exceeds dual approval threshold %s", ErrDualApprovalRequired, exp.Amount, vcfg.ApprovalWorkflow.DualApprovalAbove)
	}
	now := time.Now()
	exp.Status = "approved"
	exp.ApprovedBy = &approverID
	exp.ApprovedAt = &now
	if err := s.repo.UpdateExpense(ctx, exp); err != nil {
		return err
	}
	s.publish(ctx, func() error { return s.publisher.PublishExpenseApproved(ctx, exp) })
	return nil
}
```
Replace `ApprovePurchaseOrder` with the mirrored logic:
```go
func (s *Service) ApprovePurchaseOrder(ctx context.Context, tenantID, poID, approverID uuid.UUID, roles []string) error {
	vcfg := vertical.MustFromContext(ctx)
	po, err := s.repo.GetPurchaseOrder(ctx, tenantID, poID)
	if err != nil {
		return fmt.Errorf("get purchase order: %w", err)
	}
	if po.Status == "approved" {
		return ErrAlreadyApproved
	}
	limit := vcfg.ApprovalWorkflow.MaxLimitForRoles(roles)
	if limit == nil {
		return fmt.Errorf("%w: caller roles %v have no configured approval limit", ErrApprovalLimitExceeded, roles)
	}
	if po.Amount.GreaterThan(*limit) {
		return fmt.Errorf("%w: %s exceeds %s limit", ErrApprovalLimitExceeded, po.Amount, *limit)
	}
	if po.Amount.GreaterThan(vcfg.ApprovalWorkflow.DualApprovalAbove) {
		return fmt.Errorf("%w: %s exceeds dual approval threshold %s", ErrDualApprovalRequired, po.Amount, vcfg.ApprovalWorkflow.DualApprovalAbove)
	}
	now := time.Now()
	po.Status = "approved"
	po.ApprovedBy = &approverID
	po.ApprovedAt = &now
	return s.repo.UpdatePurchaseOrder(ctx, po)
}
```

- [ ] **Step 4: Wire the handlers + fix the e2e caller**

In `services/expense/handler.go`, in `ApproveExpense`, replace the `uuid.Nil, ""` call:
```go
	caller, ok := interceptor.CallerFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "caller identity not found in context")
	}
	if err := h.svc.ApproveExpense(ctx, tenantID, expenseID, caller.UserID, caller.Roles); err != nil {
		return nil, grpcErr(err)
	}
```
In `ApprovePurchaseOrder`, replace the `uuid.Nil` call the same way:
```go
	caller, ok := interceptor.CallerFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "caller identity not found in context")
	}
	if err := h.svc.ApprovePurchaseOrder(ctx, tenantID, poID, caller.UserID, caller.Roles); err != nil {
		return nil, grpcErr(err)
	}
```
In `e2e/critical_path/critical_path_test.go:150`, change the call to the new signature:
```go
	err = svc.ApproveExpense(ctx, fixedTenantID, fixedExpenseID, approverID, []string{"coordinator"})
```

- [ ] **Step 5: Add test helpers + handler tests**

In `services/expense/handler_test.go`, add two helpers near `ctxWithTenant`:
```go
// ctxWithCaller injects the vertical config, the caller's tenant, and the given caller.
func ctxWithCaller(caller interceptor.CallerInfo) context.Context {
	return interceptor.WithCaller(tenant.WithID(ctxWithVertical(), caller.TenantID), caller)
}

// ctxTenantNoCaller injects vertical + tenant but NO caller (to exercise the caller guard).
func ctxTenantNoCaller(tenantID uuid.UUID) context.Context {
	return tenant.WithID(ctxWithVertical(), tenantID)
}
```
Update the pre-existing `ApproveExpense` handler success test (it appended a `Role: ""` limit and injected a caller) so it instead supplies a caller with a real role and drops the `""`-limit hack — the config already grants `manager` a limit:
```go
func TestHandler_ApproveExpense_Success(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	expID := uuid.New()
	caller := interceptor.CallerInfo{UserID: uuid.New(), TenantID: tenantID, Roles: []string{"manager"}}
	callCount := 0
	h := NewHandler(NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) {
			callCount++
			st := "submitted"
			if callCount > 1 {
				st = "approved"
			}
			return &Expense{ID: id, TenantID: tid, Status: st, Amount: decimal.NewFromInt(5000)}, nil
		},
	})).WithPermissionChecker(allowAllPerm{})
	resp, err := h.ApproveExpense(ctxWithCaller(caller), &expensev1.ApproveExpenseRequest{ExpenseId: expID.String()})
	require.NoError(t, err)
	assert.Equal(t, "approved", resp.GetStatus())
}

func TestHandler_ApproveExpense_NoCaller(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ApproveExpense(ctxTenantNoCaller(uuid.New()), &expensev1.ApproveExpenseRequest{ExpenseId: uuid.New().String()})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}
```
Add the `ApprovePurchaseOrder` success (caller role `manager`, PO amount 5000, `getPOFn` returns draft then approved) + `_NoCaller` + keep the existing `_Denied`/`_NoTenant`/`_InvalidID` (those don't need a role). For `_Success`, follow the two-call `getPOFn` pattern above.

- [ ] **Step 6: Gate + commit**
```bash
go test ./services/expense/ -race && go vet ./... && go build ./...
git add services/expense/service.go services/expense/handler.go services/expense/service_test.go services/expense/handler_test.go e2e/critical_path/critical_path_test.go
git commit -m "fix(expense): thread caller roles into expense + PO approval limits (#196)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```
`go vet ./...` (whole tree) MUST pass — it is what proves the e2e call site was fixed.

---

### Task 3: SettlePettyCash advance-ownership gate (#197)

**Files:** Modify `services/expense/service.go`, `errors.go`, `handler.go`, `service_test.go`, `handler_test.go`

**Interfaces:**
- Consumes: `interceptor.CallerFromContext`; `ctxWithCaller` (Task 2).
- Produces: `Service.SettlePettyCash(ctx, tenantID, advanceID, callerID uuid.UUID, unspent decimal.Decimal) error`; `ErrNotAdvanceHolder`.

- [ ] **Step 1: Failing service tests**
```go
func TestService_SettlePettyCash_NotHolder(t *testing.T) {
	svc := NewService(&mockRepo{
		getPettyCashFn: func(_ context.Context, tid, id uuid.UUID) (*PettyCashAdvance, error) {
			return &PettyCashAdvance{ID: id, TenantID: tid, Status: "issued", Amount: decimal.NewFromInt(1000), IssuedTo: uuid.New()}, nil
		},
	})
	err := svc.SettlePettyCash(context.Background(), uuid.New(), uuid.New(), uuid.New(), decimal.NewFromInt(10))
	assert.ErrorIs(t, err, ErrNotAdvanceHolder)
}

func TestService_SettlePettyCash_HolderSucceeds(t *testing.T) {
	holder := uuid.New()
	var saved *PettyCashAdvance
	svc := NewService(&mockRepo{
		getPettyCashFn:    func(_ context.Context, tid, id uuid.UUID) (*PettyCashAdvance, error) { return &PettyCashAdvance{ID: id, TenantID: tid, Status: "issued", Amount: decimal.NewFromInt(1000), IssuedTo: holder}, nil },
		updatePettyCashFn: func(_ context.Context, pc *PettyCashAdvance) error { saved = pc; return nil },
	})
	require.NoError(t, svc.SettlePettyCash(context.Background(), uuid.New(), uuid.New(), holder, decimal.NewFromInt(10)))
	assert.Equal(t, "settled", saved.Status)
}
```

- [ ] **Step 2: Run — expect FAIL** (signature + `ErrNotAdvanceHolder` undefined): `go test ./services/expense/ -run TestService_SettlePettyCash_`

- [ ] **Step 3: Implement**

`services/expense/errors.go`, add to the `var (...)` block:
```go
	ErrNotAdvanceHolder = errors.New("expense: caller is not the petty cash advance holder")
```
`services/expense/service.go`, change `SettlePettyCash`'s signature + add the ownership guard right after the fetch:
```go
func (s *Service) SettlePettyCash(ctx context.Context, tenantID, advanceID, callerID uuid.UUID, unspent decimal.Decimal) error {
	pc, err := s.repo.GetPettyCashAdvance(ctx, tenantID, advanceID)
	if err != nil {
		return fmt.Errorf("get petty cash advance: %w", err)
	}
	if pc.IssuedTo != callerID {
		return ErrNotAdvanceHolder
	}
	if pc.Status == "settled" {
		return ErrAlreadySettled
	}
	if unspent.GreaterThan(pc.Amount) {
		return ErrUnspentExceedsAdvance
	}
	now := time.Now()
	pc.Status = "settled"
	pc.UnspentAmount = unspent
	pc.SettledAt = &now
	return s.repo.UpdatePettyCashAdvance(ctx, pc)
}
```

- [ ] **Step 4: Wire handler + grpcErr**

In `handler.go` `SettlePettyCash`, after the `unspent` negative check and before the service call, read the caller and pass its UserID:
```go
	caller, ok := interceptor.CallerFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "caller identity not found in context")
	}
	if err := h.svc.SettlePettyCash(ctx, tenantID, advanceID, caller.UserID, unspent); err != nil {
		return nil, grpcErr(err)
	}
```
In `grpcErr`, add a new arm:
```go
	case errors.Is(err, ErrNotAdvanceHolder):
		return status.Error(codes.PermissionDenied, err.Error())
```

- [ ] **Step 5: Handler tests**
```go
func TestHandler_SettlePettyCash_NotHolder(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	caller := interceptor.CallerInfo{UserID: uuid.New(), TenantID: tenantID}
	h := NewHandler(NewService(&mockRepo{
		getPettyCashFn: func(_ context.Context, tid, id uuid.UUID) (*PettyCashAdvance, error) { return &PettyCashAdvance{ID: id, TenantID: tid, Status: "issued", Amount: decimal.NewFromInt(1000), IssuedTo: uuid.New()}, nil },
	})).WithPermissionChecker(allowAllPerm{})
	_, err := h.SettlePettyCash(ctxWithCaller(caller), &expensev1.SettlePettyCashRequest{Id: uuid.New().String(), UnspentAmount: "10.00"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_SettlePettyCash_HolderSucceeds(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	holder := uuid.New()
	caller := interceptor.CallerInfo{UserID: holder, TenantID: tenantID}
	h := NewHandler(NewService(&mockRepo{
		getPettyCashFn:    func(_ context.Context, tid, id uuid.UUID) (*PettyCashAdvance, error) { return &PettyCashAdvance{ID: id, TenantID: tid, Status: "issued", Amount: decimal.NewFromInt(1000), IssuedTo: holder}, nil },
		updatePettyCashFn: func(_ context.Context, _ *PettyCashAdvance) error { return nil },
	})).WithPermissionChecker(allowAllPerm{})
	resp, err := h.SettlePettyCash(ctxWithCaller(caller), &expensev1.SettlePettyCashRequest{Id: uuid.New().String(), UnspentAmount: "10.00"})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestHandler_SettlePettyCash_NoCaller(t *testing.T) {
	t.Parallel()
	_, err := newHandler().SettlePettyCash(ctxTenantNoCaller(uuid.New()), &expensev1.SettlePettyCashRequest{Id: uuid.New().String(), UnspentAmount: "10.00"})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}
```

- [ ] **Step 6: Gate + commit**
```bash
go test ./services/expense/ -race && go vet ./... && go build ./...
git add services/expense/service.go services/expense/errors.go services/expense/handler.go services/expense/service_test.go services/expense/handler_test.go
git commit -m "feat(expense): gate SettlePettyCash on advance ownership (#197)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 4: Creator attribution — RaisedBy / SubmittedBy

**Files:** Modify `services/expense/handler.go`, `handler_test.go`

**Interfaces:**
- Consumes: `interceptor.CallerFromContext`; `ctxWithCaller` (Task 2).

- [ ] **Step 1: Failing tests**

Add recording-mock tests asserting the caller becomes the creator:
```go
func TestHandler_CreatePurchaseOrder_SetsRaisedBy(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	caller := interceptor.CallerInfo{UserID: uuid.New(), TenantID: tenantID}
	var saved *PurchaseOrder
	h := NewHandler(NewService(&mockRepo{
		createPOFn: func(_ context.Context, po *PurchaseOrder) error { saved = po; return nil },
		getPOFn:    func(_ context.Context, tid, id uuid.UUID) (*PurchaseOrder, error) { return &PurchaseOrder{ID: id, TenantID: tid}, nil },
	})).WithPermissionChecker(allowAllPerm{})
	_, err := h.CreatePurchaseOrder(ctxWithCaller(caller), &expensev1.CreatePurchaseOrderRequest{ProductionId: uuid.New().String(), VendorName: "v", Amount: "100"})
	require.NoError(t, err)
	assert.Equal(t, caller.UserID, saved.RaisedBy)
}

func TestHandler_SubmitExpense_SetsSubmittedBy(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	caller := interceptor.CallerInfo{UserID: uuid.New(), TenantID: tenantID}
	var saved *Expense
	h := NewHandler(NewService(&mockRepo{
		createExpenseFn: func(_ context.Context, e *Expense) error { saved = e; return nil },
		getExpenseFn:    func(_ context.Context, tid, id uuid.UUID) (*Expense, error) { return &Expense{ID: id, TenantID: tid}, nil },
	})).WithPermissionChecker(allowAllPerm{})
	_, err := h.SubmitExpense(ctxWithCaller(caller), &expensev1.SubmitExpenseRequest{ProductionId: uuid.New().String(), CategoryId: "cat", Amount: "100"})
	require.NoError(t, err)
	assert.Equal(t, caller.UserID, saved.SubmittedBy)
}
```
(Confirm `mockRepo` has `createExpenseFn`/`createPOFn` fields — they exist. `SubmitExpense`'s repo write is `CreateExpense`.)

- [ ] **Step 2: Run — expect FAIL** (`RaisedBy`/`SubmittedBy` are `uuid.Nil`): `go test ./services/expense/ -run 'SetsRaisedBy|SetsSubmittedBy'`

- [ ] **Step 3: Implement**

In `handler.go` `CreatePurchaseOrder`, read the caller (after the existing tenant + perm gates, before building `po`) and set `RaisedBy`:
```go
	caller, ok := interceptor.CallerFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "caller identity not found in context")
	}
```
then in the `po := &PurchaseOrder{...}` literal change `RaisedBy: uuid.Nil` → `RaisedBy: caller.UserID`.
In `SubmitExpense`, add the same caller read (after its existing perm + tenant gates, before building `e`) and change `SubmittedBy: uuid.Nil` → `SubmittedBy: caller.UserID`.

- [ ] **Step 4: Run — expect PASS**: `go test ./services/expense/ -run 'SetsRaisedBy|SetsSubmittedBy'`

- [ ] **Step 5: Gate + commit**
```bash
go test ./services/expense/ -race && go vet ./... && go build ./... && gofmt -l services/expense/
git add services/expense/handler.go services/expense/handler_test.go
git commit -m "feat(expense): attribute PO/expense creator from caller identity (#196)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- `MaxLimitForRoles` → Task 1 ✅
- ApproveExpense P0 fix (real roles) → Task 2 ✅
- #196 ApprovePurchaseOrder limit + dual-approval parity → Task 2 ✅
- #197 SettlePettyCash ownership + `ErrNotAdvanceHolder`→PermissionDenied → Task 3 ✅
- Attribution ApprovedBy (Tasks 2), RaisedBy/SubmittedBy (Task 4) ✅
- Caller from `CallerFromContext`, `!ok`→Unauthenticated → Tasks 2–4 ✅
- Keep `expense:submit` on settle; no super_admin bypass; fail-closed → honored ✅
- Whole-tree `go vet` after sig changes + fix e2e caller → Task 2/3 gates ✅
- #199 (config reconciliation), no proto/migration → non-goals honored ✅

**Placeholder scan:** none — full code for helper, service, handler, errors, grpcErr, tests. "Confirm mock field exists / follow the two-call pattern" notes are compiler-checked, not placeholders.

**Type consistency:** `ApproveExpense(…, roles []string)` / `ApprovePurchaseOrder(…, roles []string)` / `SettlePettyCash(…, callerID uuid.UUID, unspent decimal.Decimal)` are identical across service def (Tasks 2/3), handler call sites (Tasks 2/3), and the e2e caller (Task 2). `MaxLimitForRoles(roles []string) *decimal.Decimal` (Task 1) == service call (Task 2). `ctxWithCaller`/`ctxTenantNoCaller` defined in Task 2, reused in Tasks 3/4. `ErrNotAdvanceHolder` (Task 3) mapped in grpcErr (Task 3). `caller.UserID`/`caller.Roles` fields match `CallerInfo`.

**Ordering:** Task 1 (helper) → Task 2 (needs helper; changes ApproveExpense/PO sigs + fixes e2e + adds test helpers) → Task 3 (settle; reuses helpers) → Task 4 (attribution; reuses helpers). Each commit builds tree-wide (`go vet ./...` gate).
