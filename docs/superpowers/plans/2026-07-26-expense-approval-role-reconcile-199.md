# Approval-limit role reconciliation (#199) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make expense/PO approval work in every vertical and for `super_admin` — remap the 3 broken configs' approval-limit roles to RBAC role names, add an unlimited `super_admin` bypass, and add a validator guardrail so the vocabulary can't drift again.

**Architecture:** `ApprovalLimit.Role` is already string-compared to `caller.Roles`, so the vocabulary fix is pure DATA (3 YAML files). `super_admin` gets a code-level bypass in `services/expense/service.go` (it holds every permission; a money cap is meaningless — but the dual-approval threshold still applies to everyone). A new rule in `pkg/vertical.Validate()` fails any config whose `limits[].role` is not a system role. No proto/migration/sqlc.

**Tech Stack:** Go 1.25, `pkg/vertical` (YAML configs + validator), `services/expense`, `decimal.Decimal`, `gopkg.in/yaml.v3`, testify.

## Global Constraints

- **Approval-limit role names MUST be RBAC system roles** (`super_admin`, `manager`, `coordinator`, `accountant`, `member`, `inventory_manager`, `project_supervisor`) — the exact `vertical.SystemRoleNames` set (`pkg/vertical/validator.go:40`).
- **`super_admin` bypass:** in `ApproveExpense` and `ApprovePurchaseOrder`, skip the `MaxLimitForRoles` limit check when the caller holds `super_admin`. **The dual-approval-above-threshold check still applies to everyone, super_admin included.** No other bypass. Check order otherwise unchanged: already-approved → [limit unless super_admin] → dual-approval → set approved.
- **Money** stays `decimal.Decimal`; comparisons via decimal methods, never `float64`.
- **Config amounts are exact** — copy the per-vertical values in Task 1 verbatim. `dual_approval_above` unchanged per vertical. `movie-production.yaml` is NOT edited.
- **No schema/type change** (`ApprovalWorkflow`/`ApprovalLimit` Go types, `schema.json` untouched). No proto, migration, sqlc, or Kong changes. `tests/integration/vertical/fixtures.go` untouched.
- **Gate every Go task with `gofmt -l <touched files>`** (must print nothing) in addition to `go test`/`go vet ./...`/`go build ./...` — the CI `Lint` job runs gofmt and `go test`/`vet`/`build` all accept gofmt-dirty code. `db/postgres.go` is gofmt-dirty on `main` (pre-existing) — not your concern; only touched files must be clean.
- **`make validate-verticals`** (= `TestValidateAllProductionVerticals`, `pkg/vertical/schema_test.go:72`) must stay green after Tasks 1 and 2.
- **Security-sensitive** (money approval authz + industry configs) → senior review required per CLAUDE.md.
- Commits Conventional-Commits (scope `vertical` for Tasks 1–2, `expense` for Task 3), ending `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `pkg/vertical/configs/{construction,events-management,software-development}.yaml` | remap `limits[].role` to RBAC names | 1 |
| `pkg/vertical/schema_test.go` | real-YAML role-membership test | 1 |
| `pkg/vertical/validator.go` | `limits[].role ∈ SystemRoleNames` rule | 2 |
| `pkg/vertical/validator_test.go` | negative test (bogus role rejected) | 2 |
| `services/expense/service.go` | `hasSuperAdmin` + bypass in both approve methods | 3 |
| `services/expense/service_test.go` | super_admin bypass + accountant-tier tests | 3 |

**Ordering rationale:** Task 1 fixes the DATA first so that Task 2's new validator rule keeps `TestValidateAllProductionVerticals` green (adding the rule before the data fix would red the tree). Task 3 is code-only and independent.

---

### Task 1: Remap config approval-limit roles to RBAC names

**Files:** Modify `pkg/vertical/configs/construction.yaml`, `events-management.yaml`, `software-development.yaml`; Test `pkg/vertical/schema_test.go`

**Interfaces:**
- Produces: 3 configs whose `approval_workflow.limits[].role` values are all in `SystemRoleNames` (consumed at runtime by `MaxLimitForRoles`; asserted by Task 2's validator rule).

- [ ] **Step 1: Write the failing test**

Add to `pkg/vertical/schema_test.go` (package `vertical`; it can use `VerticalYAML`, `SystemRoleNames`, and `gopkg.in/yaml.v3` — add the `yaml` import to the file's import block):
```go
// TestApprovalLimitRolesAreSystemRoles loads every real config YAML and asserts
// each approval_workflow.limits[].role is a canonical RBAC system role. This is
// the regression guard for #199 — the hand-rolled fixtures in service_test.go and
// tests/integration/vertical never load the real YAMLs, so they were blind to it.
func TestApprovalLimitRolesAreSystemRoles(t *testing.T) {
	t.Parallel()

	valid := make(map[string]bool, len(SystemRoleNames))
	for _, name := range SystemRoleNames {
		valid[name] = true
	}

	paths, err := filepath.Glob(filepath.Join("configs", "*.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, paths)

	for _, path := range paths {
		path := path
		verticalID := strings.TrimSuffix(filepath.Base(path), ".yaml")
		t.Run(verticalID, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			var root struct {
				Vertical VerticalYAML `yaml:"vertical"`
			}
			require.NoError(t, yaml.Unmarshal(data, &root))
			for _, lim := range root.Vertical.ApprovalWorkflow.Limits {
				assert.True(t, valid[lim.Role],
					"%s: approval limit role %q is not a system role", verticalID, lim.Role)
			}
		})
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (construction/events/software use non-RBAC role names): `go test ./pkg/vertical/ -run TestApprovalLimitRolesAreSystemRoles -v`
Expected: subtests `construction`, `events-management`, `software-development` FAIL (roles `site_manager`, `project_manager`, `contracts_manager`, `event_coordinator`, `event_manager`, `account_director`, `team_lead`, `director` are not system roles); `movie-production` PASSES.

- [ ] **Step 3: Fix the 3 configs**

In `pkg/vertical/configs/construction.yaml`, replace the `approval_workflow:` block's `limits:` with (keep `dual_approval_above: 1000000`):
```yaml
  approval_workflow:
    limits:
      - role: coordinator
        max_amount: 100000
      - role: accountant
        max_amount: 500000
      - role: manager
        max_amount: 2000000
    dual_approval_above: 1000000
```
In `pkg/vertical/configs/events-management.yaml` (keep `dual_approval_above: 500000`):
```yaml
  approval_workflow:
    limits:
      - role: coordinator
        max_amount: 50000
      - role: accountant
        max_amount: 250000
      - role: manager
        max_amount: 1000000
    dual_approval_above: 500000
```
In `pkg/vertical/configs/software-development.yaml` (keep `dual_approval_above: 500000`):
```yaml
  approval_workflow:
    limits:
      - role: coordinator
        max_amount: 50000
      - role: project_supervisor
        max_amount: 200000
      - role: manager
        max_amount: 1000000
    dual_approval_above: 500000
```
Match the existing YAML indentation exactly (2-space `approval_workflow:` under `vertical:`, `limits:` +2, list items +4, keys +6). Do NOT touch `movie-production.yaml` or any other block.

- [ ] **Step 4: Run — expect PASS**: `go test ./pkg/vertical/ -run TestApprovalLimitRolesAreSystemRoles -v` (all 4 subtests pass). Also run the existing suite to confirm nothing regressed: `go test ./pkg/vertical/ -run TestValidateAll` (still green — the validator rule isn't added yet, so these already passed and continue to).

- [ ] **Step 5: Commit**
```bash
gofmt -l pkg/vertical/schema_test.go   # must print nothing
go test ./pkg/vertical/ -race && go build ./...
git add pkg/vertical/configs/construction.yaml pkg/vertical/configs/events-management.yaml pkg/vertical/configs/software-development.yaml pkg/vertical/schema_test.go
git commit -m "fix(vertical): remap approval-limit roles to RBAC names (#199)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Validator guardrail — limits[].role must be a system role

**Files:** Modify `pkg/vertical/validator.go`; Test `pkg/vertical/validator_test.go`

**Interfaces:**
- Consumes: the now-fixed configs (Task 1) so `TestValidateAllProductionVerticals` stays green.
- Produces: a `Validate()` that appends a `ValidationError{"vertical.approval_workflow.limits", ...}` for any limit role not in `SystemRoleNames`.

- [ ] **Step 1: Write the failing test**

Add to `pkg/vertical/validator_test.go` (mirror the existing negative tests in that file — reuse whatever minimal-valid-YAML helper / builder they use; if they inline a full valid YAML string, copy that pattern and set one bogus limit role). The test must assert BOTH that errors are returned AND that one targets the approval-limit field:
```go
func TestValidate_ApprovalLimitRoleNotSystemRole(t *testing.T) {
	t.Parallel()

	// A config identical to a valid one except one approval limit uses a
	// non-RBAC role name. (Build from the same minimal-valid YAML the other
	// negative tests in this file use; set limits[].role to "site_manager".)
	yamlData := validYAMLWithApprovalLimitRole(t, "site_manager")

	errs, parseErr := Validate(yamlData)
	require.NoError(t, parseErr)
	require.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if e.Field == "vertical.approval_workflow.limits" &&
			strings.Contains(e.Message, "site_manager") {
			found = true
		}
	}
	assert.True(t, found, "expected an approval-limit role validation error, got: %v", errs)
}
```
**Implementer note:** if `validator_test.go` has no reusable minimal-valid-YAML builder, do NOT invent a fragile new one — instead read a real config and string-replace one limit role:
```go
func validYAMLWithApprovalLimitRole(t *testing.T, role string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("configs", "movie-production.yaml"))
	require.NoError(t, err)
	// movie-production's first limit role is "coordinator"; swap it for the bogus role.
	out := strings.Replace(string(data), "- role: coordinator", "- role: "+role, 1)
	require.NotEqual(t, string(data), out, "expected to replace a limit role")
	return []byte(out)
}
```
Confirm `ValidationError`'s field/message accessor names (`.Field`, `.Message`) against the struct definition in `validator.go` before finalizing — adjust if they differ (it is a 2-field struct: the source shows positional `ValidationError{"vertical.role_labels", fmt.Sprintf(...)}`).

- [ ] **Step 2: Run — expect FAIL** (`Validate` doesn't check limit roles yet, so no such error is produced): `go test ./pkg/vertical/ -run TestValidate_ApprovalLimitRoleNotSystemRole -v`

- [ ] **Step 3: Implement the rule**

In `pkg/vertical/validator.go` `Validate()`, immediately AFTER the existing `role_labels` coverage loop (ends `validator.go:272`, just before the `if len(errs) > 0` return at `:274`), insert:
```go
	// Approval-limit roles must be canonical RBAC role names — otherwise
	// MaxLimitForRoles(caller.Roles) never matches and approvals fail-closed (#199).
	validRole := make(map[string]bool, len(SystemRoleNames))
	for _, name := range SystemRoleNames {
		validRole[name] = true
	}
	for _, lim := range v.ApprovalWorkflow.Limits {
		if !validRole[lim.Role] {
			errs = append(errs, ValidationError{
				"vertical.approval_workflow.limits",
				fmt.Sprintf("limit role %q is not a system role", lim.Role),
			})
		}
	}
```
(`v` is the `VerticalYAML` already in scope in `Validate()`; `v.ApprovalWorkflow.Limits` is `[]ApprovalLimitYAML{Role, MaxAmount}` — `validator.go:98-106`. `fmt` is already imported.)

- [ ] **Step 4: Run — expect PASS**: `go test ./pkg/vertical/ -run 'TestValidate_ApprovalLimitRoleNotSystemRole|TestValidateAll' -v` — the negative test passes AND all 4 real configs still validate clean (they were fixed in Task 1).

- [ ] **Step 5: Commit**
```bash
gofmt -l pkg/vertical/validator.go pkg/vertical/validator_test.go   # must print nothing
go test ./pkg/vertical/ -race && go vet ./... && go build ./...
git add pkg/vertical/validator.go pkg/vertical/validator_test.go
git commit -m "feat(vertical): reject approval-limit roles that aren't system roles (#199)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: super_admin unlimited approval bypass

**Files:** Modify `services/expense/service.go`, `service_test.go`

**Interfaces:**
- Consumes: nothing from Tasks 1–2 (code-only; independent).
- Produces: `hasSuperAdmin(roles []string) bool`; `ApproveExpense`/`ApprovePurchaseOrder` skip the limit check for super_admin (signatures UNCHANGED).

- [ ] **Step 1: Write the failing tests**

Add to `services/expense/service_test.go`. First a construction-shaped config helper (movie's `movieProductionConfig()` has no `accountant` tier and its max limit equals its dual threshold, so it can't exercise "approve above the limit but below dual"):
```go
// approvalConfigConstruction mirrors the (post-#199) construction approval tiers:
// coordinator 100k / accountant 500k / manager 2M, dual-approval above 1M.
func approvalConfigConstruction() *vertical.Config {
	c := movieProductionConfig()
	c.ApprovalWorkflow = vertical.ApprovalWorkflow{
		Limits: []vertical.ApprovalLimit{
			{Role: "coordinator", MaxAmount: decimal.NewFromInt(100000)},
			{Role: "accountant", MaxAmount: decimal.NewFromInt(500000)},
			{Role: "manager", MaxAmount: decimal.NewFromInt(2000000)},
		},
		DualApprovalAbove: decimal.NewFromInt(1000000),
	}
	return c
}

func ctxConstruction() context.Context {
	return vertical.WithConfig(context.Background(), approvalConfigConstruction())
}
```
Then the tests (super_admin bypass + accountant tier, for both approve methods):
```go
func TestService_ApproveExpense_SuperAdminBypassesLimit(t *testing.T) {
	// 800k exceeds every configured per-role limit reachable by ["super_admin"]
	// (which has no limit entry → MaxLimitForRoles=nil), but is below the 1M dual
	// threshold. Without the bypass this would fail ErrApprovalLimitExceeded.
	var saved *Expense
	svc := NewService(&mockRepo{
		getExpenseFn:    func(_ context.Context, tid, id uuid.UUID) (*Expense, error) { return &Expense{ID: id, TenantID: tid, Status: "submitted", Amount: decimal.NewFromInt(800000)}, nil },
		updateExpenseFn: func(_ context.Context, e *Expense) error { saved = e; return nil },
	})
	err := svc.ApproveExpense(ctxConstruction(), uuid.New(), uuid.New(), uuid.New(), []string{"super_admin"})
	require.NoError(t, err)
	assert.Equal(t, "approved", saved.Status)
}

func TestService_ApproveExpense_SuperAdminStillHitsDualApproval(t *testing.T) {
	// 1.5M is above the 1M dual threshold — dual approval applies to everyone.
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) { return &Expense{ID: id, TenantID: tid, Status: "submitted", Amount: decimal.NewFromInt(1500000)}, nil },
	})
	err := svc.ApproveExpense(ctxConstruction(), uuid.New(), uuid.New(), uuid.New(), []string{"super_admin"})
	assert.ErrorIs(t, err, ErrDualApprovalRequired)
}

func TestService_ApproveExpense_AccountantWithinTier(t *testing.T) {
	var saved *Expense
	svc := NewService(&mockRepo{
		getExpenseFn:    func(_ context.Context, tid, id uuid.UUID) (*Expense, error) { return &Expense{ID: id, TenantID: tid, Status: "submitted", Amount: decimal.NewFromInt(400000)}, nil },
		updateExpenseFn: func(_ context.Context, e *Expense) error { saved = e; return nil },
	})
	require.NoError(t, svc.ApproveExpense(ctxConstruction(), uuid.New(), uuid.New(), uuid.New(), []string{"accountant"}))
	assert.Equal(t, "approved", saved.Status)
}

func TestService_ApproveExpense_AccountantAboveTierFails(t *testing.T) {
	// 600k exceeds accountant's 500k limit (accountant is not super_admin).
	svc := NewService(&mockRepo{
		getExpenseFn: func(_ context.Context, tid, id uuid.UUID) (*Expense, error) { return &Expense{ID: id, TenantID: tid, Status: "submitted", Amount: decimal.NewFromInt(600000)}, nil },
	})
	assert.ErrorIs(t, svc.ApproveExpense(ctxConstruction(), uuid.New(), uuid.New(), uuid.New(), []string{"accountant"}), ErrApprovalLimitExceeded)
}

func TestService_ApprovePurchaseOrder_SuperAdminBypassesLimit(t *testing.T) {
	var saved *PurchaseOrder
	svc := NewService(&mockRepo{
		getPOFn:    func(_ context.Context, tid, id uuid.UUID) (*PurchaseOrder, error) { return &PurchaseOrder{ID: id, TenantID: tid, Status: "draft", Amount: decimal.NewFromInt(800000)}, nil },
		updatePOFn: func(_ context.Context, po *PurchaseOrder) error { saved = po; return nil },
	})
	require.NoError(t, svc.ApprovePurchaseOrder(ctxConstruction(), uuid.New(), uuid.New(), uuid.New(), []string{"super_admin"}))
	assert.Equal(t, "approved", saved.Status)
}

func TestService_ApprovePurchaseOrder_SuperAdminStillHitsDualApproval(t *testing.T) {
	svc := NewService(&mockRepo{
		getPOFn: func(_ context.Context, tid, id uuid.UUID) (*PurchaseOrder, error) { return &PurchaseOrder{ID: id, TenantID: tid, Status: "draft", Amount: decimal.NewFromInt(1500000)}, nil },
	})
	assert.ErrorIs(t, svc.ApprovePurchaseOrder(ctxConstruction(), uuid.New(), uuid.New(), uuid.New(), []string{"super_admin"}), ErrDualApprovalRequired)
}
```
Confirm the `mockRepo` field names (`getExpenseFn`/`updateExpenseFn`/`getPOFn`/`updatePOFn`) and the `Expense`/`PurchaseOrder` field names against the real source — they are the same ones used by the existing `TestService_Approve*` tests in this file (added in #196/#197). If any differ, match reality and report it.

- [ ] **Step 2: Run — expect FAIL** (super_admin has no limit → `MaxLimitForRoles`=nil → `ErrApprovalLimitExceeded`, so the two `_SuperAdminBypassesLimit` tests fail): `go test ./services/expense/ -run 'SuperAdmin|AccountantWithinTier|AccountantAboveTier'`

- [ ] **Step 3: Implement**

In `services/expense/service.go`, add the helper (anywhere at file scope, e.g. just above `ApproveExpense`; no new import — it uses a range loop):
```go
// hasSuperAdmin reports whether the caller holds the super_admin RBAC role, which
// carries unlimited approval authority (it holds every permission; a per-role money
// limit on it is meaningless). The dual-approval threshold still applies to all callers.
func hasSuperAdmin(roles []string) bool {
	for _, r := range roles {
		if r == "super_admin" {
			return true
		}
	}
	return false
}
```
In `ApproveExpense`, replace the existing limit-check block (`service.go:80-87`, the `// Check role-based approval limit` comment through the `exp.Amount.GreaterThan(*limit)` return) with:
```go
	// Role-based approval limit — super_admin has unlimited authority and skips it.
	if !hasSuperAdmin(roles) {
		limit := vcfg.ApprovalWorkflow.MaxLimitForRoles(roles)
		if limit == nil {
			return fmt.Errorf("%w: caller roles %v have no configured approval limit", ErrApprovalLimitExceeded, roles)
		}
		if exp.Amount.GreaterThan(*limit) {
			return fmt.Errorf("%w: %s exceeds %s limit", ErrApprovalLimitExceeded, exp.Amount, *limit)
		}
	}
```
Leave the dual-approval block (`service.go:89-92`) exactly as-is, AFTER this — it applies to everyone.
In `ApprovePurchaseOrder`, make the identical change to its limit-check block (`service.go:203-210`) using `po.Amount`:
```go
	// Role-based approval limit — super_admin has unlimited authority and skips it.
	if !hasSuperAdmin(roles) {
		limit := vcfg.ApprovalWorkflow.MaxLimitForRoles(roles)
		if limit == nil {
			return fmt.Errorf("%w: caller roles %v have no configured approval limit", ErrApprovalLimitExceeded, roles)
		}
		if po.Amount.GreaterThan(*limit) {
			return fmt.Errorf("%w: %s exceeds %s limit", ErrApprovalLimitExceeded, po.Amount, *limit)
		}
	}
```
Leave `ApprovePurchaseOrder`'s dual-approval block untouched, after this.

- [ ] **Step 4: Run — expect PASS**: `go test ./services/expense/ -run 'SuperAdmin|AccountantWithinTier|AccountantAboveTier' -v`, then the whole package `go test ./services/expense/ -race` (the pre-existing #196/#197 approval tests must still pass — they used non-super_admin roles with configured limits, unaffected).

- [ ] **Step 5: Commit**
```bash
gofmt -l services/expense/service.go services/expense/service_test.go   # must print nothing
go test ./services/expense/ -race && go vet ./... && go build ./...
git add services/expense/service.go services/expense/service_test.go
git commit -m "feat(expense): super_admin bypasses approval limit, dual-approval still applies (#199)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- Config remap of construction/events/software to RBAC names, movie unchanged → Task 1 ✅
- Validator rule `limits[].role ∈ SystemRoleNames` + negative test → Task 2 ✅
- Real-YAML role-membership test (closes the fixture blind spot) → Task 1 ✅
- super_admin unlimited bypass in both approve methods; dual-approval still applies to all → Task 3 ✅
- accountant/project_supervisor gain approval authority (via the data) + proven for accountant → Task 3 ✅
- No schema/type/proto/migration/sqlc; fixtures.go untouched → honored ✅

**Placeholder scan:** none — full YAML blocks, full validator rule, full service diff, full tests. The two "confirm accessor/field names against source" notes are compiler-checked instructions, not placeholders; the `validYAMLWithApprovalLimitRole` helper is fully written as a fallback.

**Type consistency:** `hasSuperAdmin(roles []string) bool` (Task 3) matches its two call sites. `ApproveExpense`/`ApprovePurchaseOrder` signatures UNCHANGED (no whole-tree ripple, unlike #196/#197). `ValidationError{field, message}` positional literal matches the existing `role_labels` usage. `VerticalYAML.ApprovalWorkflow.Limits []ApprovalLimitYAML{Role, MaxAmount}` used in Task 1 test + Task 2 rule matches `validator.go:98-106`. `vertical.ApprovalWorkflow{Limits []ApprovalLimit{Role, MaxAmount}, DualApprovalAbove}` in the Task 3 helper matches `movieProductionConfig()` at `service_test.go:134`.

**Ordering:** Task 1 (data) before Task 2 (rule) so `TestValidateAllProductionVerticals` never reds. Task 3 code-only, independent. Every commit builds and passes its gate.
