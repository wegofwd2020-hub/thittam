# Expense caller-identity authz — #196 / #197 (+ ApproveExpense fix) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-26
**Issues:** #196 (ApprovePurchaseOrder approval-limit parity), #197 (SettlePettyCash ownership) — both from #191's security review
**Branch:** `feat/expense-caller-authz-196-197` off `main`
**Migration:** none · **Proto:** none

## Goal

Wire the caller's real identity (`UserID` + `Roles`) — already present in context on
every authenticated expense RPC but ignored — into the expense handlers, which:
1. **Fixes a live P0:** `ApproveExpense` currently passes `approverRole=""` →
   `LimitForRole("")` returns nil → every approval fails `ErrApprovalLimitExceeded`.
   No expense can be approved in production today.
2. **#196:** `ApprovePurchaseOrder` gains the same role-limit + dual-approval check
   `ApproveExpense` has.
3. **#197:** `SettlePettyCash` requires the caller to be the advance holder.
4. Sets `ApprovedBy` / `RaisedBy` / `SubmittedBy` to the real caller (audit
   attribution; today all `uuid.Nil`), closing the #139 actor-from-token class.

## Context (grounding facts, `main`)

- `interceptor.CallerFromContext(ctx) (CallerInfo, bool)` (`pkg/interceptor/auth.go:65`)
  returns `CallerInfo{UserID, TenantID, Roles []string, ...}`, set from the verified
  JWT by `UnaryAuthInterceptor` on every non-public RPC. **The expense handlers never
  call it** — they hardcode `uuid.Nil`/`""` (handler.go:77, 168, 217, 316, 387).
  `services/ledger` and `services/iam` already use this pattern.
- `ApprovalWorkflow.LimitForRole(role string) *decimal.Decimal` (`pkg/vertical/helpers.go:58`)
  matches ONE role name against the per-vertical `limits`. `caller.Roles` is a SET.
- **Known limitation (out of scope, tracked in #199):** the approval-limit configs key
  off role names that only match RBAC roles for **movie-production** (`coordinator`,
  `manager`). For construction/software/events verticals and for
  `super_admin`/`accountant`/`member`, no limit is configured → approval fail-closes.
  This slice does the identity wiring correctly and mirrors the existing check; the
  cross-vertical config reconciliation is #199.
- `PettyCashAdvance.IssuedTo uuid.UUID` (`models.go:58`) is returned by
  `GetPettyCashAdvance`; `SettlePettyCash` already fetches the advance.
- Expense perms: `expense:approve` gates ApproveExpense/ApprovePurchaseOrder/RejectExpense;
  `expense:submit` gates SubmitExpense/CreatePurchaseOrder/CreatePettyCashAdvance/
  SettlePettyCash. The `member` role (natural advance holder) has `expense:submit`
  but NOT `expense:approve` — so #197 must be an ownership check, not a permission bump.

## Design

### 1. `pkg/vertical` — `MaxLimitForRoles` helper

`caller.Roles` is a set; the limit check needs one bound. Add
(`pkg/vertical/helpers.go`):
```go
// MaxLimitForRoles returns the highest configured approval limit among the
// given roles, or nil if none of them has a configured limit.
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
A caller holding multiple roles gets their most permissive entitlement. Nil → the
caller has no approval authority → fail-closed. **No `super_admin` bypass** (faithful
mirror of the existing logic; `super_admin` approves only where a limit is configured
for it — see #199).

### 2. Service layer (`services/expense/service.go`)

- **`ApproveExpense(ctx, tenantID, expenseID, approverID uuid.UUID, roles []string) error`**
  (was `approverRole string`): replace `LimitForRole(approverRole)` with
  `vcfg.ApprovalWorkflow.MaxLimitForRoles(roles)`. Everything else (nil→
  `ErrApprovalLimitExceeded`, `Amount.GreaterThan(*limit)`, `DualApprovalAbove`,
  set approved) unchanged.
- **`ApprovePurchaseOrder(ctx, tenantID, poID, approverID uuid.UUID, roles []string) error`**
  — add the SAME limit + dual-approval logic before the status flip (mirror
  ApproveExpense against `po.Amount`): `vcfg := vertical.MustFromContext(ctx)`;
  `limit := vcfg.ApprovalWorkflow.MaxLimitForRoles(roles)`; nil → `ErrApprovalLimitExceeded`;
  `po.Amount.GreaterThan(*limit)` → `ErrApprovalLimitExceeded`;
  `po.Amount.GreaterThan(vcfg.ApprovalWorkflow.DualApprovalAbove)` → `ErrDualApprovalRequired`;
  then `ErrAlreadyApproved` guard + set `Status="approved"`, `ApprovedBy=&approverID`,
  `ApprovedAt=&now`, `UpdatePurchaseOrder`. (Keep the already-approved guard.)
- **`SettlePettyCash(ctx, tenantID, advanceID, callerID uuid.UUID, unspent decimal.Decimal) error`**
  — after `GetPettyCashAdvance` and before the existing already-settled /
  exceeds-advance guards, add: `if pc.IssuedTo != callerID { return ErrNotAdvanceHolder }`.
  (Tenant scoping already guarantees same-tenant; this adds intra-tenant ownership.)

### 3. `services/expense/errors.go`

Add `ErrNotAdvanceHolder = errors.New("expense: caller is not the petty cash advance holder")`.
(`ErrApprovalLimitExceeded` / `ErrDualApprovalRequired` already exist.)

### 4. Handlers (`services/expense/handler.go`)

Each affected handler, after the existing tenant + `RequirePermission` gates, reads
`caller, ok := interceptor.CallerFromContext(ctx)` (→ `Unauthenticated` if `!ok`),
then threads real identity:
- **`ApproveExpense`** → `svc.ApproveExpense(ctx, tenantID, expenseID, caller.UserID, caller.Roles)`.
- **`ApprovePurchaseOrder`** → `svc.ApprovePurchaseOrder(ctx, tenantID, poID, caller.UserID, caller.Roles)`.
- **`SettlePettyCash`** → `svc.SettlePettyCash(ctx, tenantID, advanceID, caller.UserID, unspent)`.
- **`CreatePurchaseOrder`** → `po.RaisedBy = caller.UserID` (was `uuid.Nil`).
- **`SubmitExpense`** → `SubmittedBy = caller.UserID` (was `uuid.Nil`).

`grpcErr`: add `ErrNotAdvanceHolder` → `codes.PermissionDenied` (its own arm;
`ErrApprovalLimitExceeded`/`ErrDualApprovalRequired` already map to `FailedPrecondition`).

**Not touched:** `CreatePettyCashAdvance.IssuedTo` stays request-supplied (it's the
advance recipient, not the creator). `RejectExpense` needs no identity (no actor field
recorded today; leaving it avoids scope creep — a rejection audit actor is a separate
concern).

## Testing

- **`pkg/vertical`**: `MaxLimitForRoles` — max across multiple configured roles, nil
  when none configured, single-role parity with `LimitForRole`.
- **Service** (`service_test.go`): `ApproveExpense` now SUCCEEDS with a role that has a
  configured limit ≥ amount (regression proving the P0 fix) and fails
  `ErrApprovalLimitExceeded` with no-limit roles; `ApprovePurchaseOrder` — approves
  within limit, `ErrApprovalLimitExceeded` over limit / no-limit role,
  `ErrDualApprovalRequired` above threshold, `ErrAlreadyApproved` idempotency;
  `SettlePettyCash` — `ErrNotAdvanceHolder` when `callerID != IssuedTo`, success when
  equal. Use a vertical config with a known role limit (the test helper already builds
  `movieProductionConfig()`; add a limit for the test's role as the existing
  `ApproveExpense` test does).
- **Handler** (`handler_test.go`): each wired handler asserts the caller identity from
  context reaches the service (recording mock / spy) and that a missing caller →
  `Unauthenticated`; `SettlePettyCash` non-holder → `PermissionDenied`;
  `CreatePurchaseOrder`/`SubmitExpense` set the caller as `RaisedBy`/`SubmittedBy`.
  Update the existing `ApproveExpense` handler test that injected a `""`-role limit —
  it now supplies `caller.Roles` via context (`interceptor.WithCaller`).
- `go test ./services/expense/... ./pkg/vertical/... -race`; `go build ./...` +
  `go vet ./...` (signature changes ripple to expense test doubles — update all,
  whole-tree vet); no proto/sqlc/migration, so no codegen gates.

## Non-goals

- **#199** — reconciling the 3 non-movie verticals' approval-limit role names to RBAC
  roles + `super_admin` semantics. This slice wires identity + mirrors the existing
  check; it does not redesign the approval model or edit industry configs.
- No `super_admin` unlimited bypass (fail-closed, per the design decision).
- No real two-party dual-approval flow — `DualApprovalAbove` keeps its existing
  "block single approval above threshold" behavior.
- No `RejectExpense`/`CreatePettyCashAdvance` actor changes.
- No proto, migration, or Kong changes.

## Review weight

Touches expense **authorization** (money approval + ownership) → security-sensitive;
per CLAUDE.md a senior engineer is required for security changes. Standard 2 approvals +
senior. Whole-branch review on the most capable model.
