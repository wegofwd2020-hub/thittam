# Approval-limit role reconciliation (#199) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-26
**Issue:** #199 (approval-limit role names don't match RBAC roles → approvals fail-closed outside movie-production)
**Branch:** `feat/expense-approval-role-reconcile-199` off `main`
**Migration:** none · **Proto:** none · **sqlc:** none

## Goal

Make expense/PO approval actually work in every vertical (not just movie-production) and
for `super_admin`. Two root causes, both surfaced by the #196/#197 caller-identity slice:

1. **Vocabulary mismatch (data bug).** `approval_workflow.limits[].role` in
   `construction`, `events-management`, and `software-development` uses invented
   job-title strings that match no RBAC role, so `MaxLimitForRoles(caller.Roles)`
   returns `nil` for every real caller → `ErrApprovalLimitExceeded`. Only
   `movie-production` (which happens to use `coordinator`/`manager`) works today.
2. **`super_admin` has no approval authority.** No config grants `super_admin` a
   limit and no code special-cases it, so despite holding `expense:approve` (and every
   other permission) it cannot approve any expense/PO in ANY vertical, including
   movie-production. The #196/#197 "no bypass, fail-closed" decision made this concrete.

Plus a **guardrail** so this class of bug cannot recur silently.

## Context (grounding facts, `main` @ cb528e5)

- `ApprovalLimit.Role` (`pkg/vertical/types.go:90`) is compared by exact string equality
  to `caller.Roles` entries in `LimitForRole` (`helpers.go:58`) and `MaxLimitForRoles`
  (`helpers.go:69`). `caller.Roles` are the RBAC role-name strings from the JWT
  (`pkg/interceptor/authjwt.go:57` ← `roles.name` in the DB). **No type/shape change is
  needed** — only the DATA in `limits[].role` must be RBAC role names.
- Every config already has a correct `role_labels:` map keyed by canonical RBAC role
  name (e.g. `construction.yaml` `coordinator: Site Manager`). This proves the intended
  pattern: the broken configs put the *display label text* (snake-cased) into
  `limits[].role` instead of the RBAC key.
- The 7 RBAC system roles: `super_admin`, `manager`, `coordinator`, `accountant`,
  `member`, `inventory_manager`, `project_supervisor` — authoritative in
  `services/iam.systemRoles` (unexported) and mirrored as the exported
  `vertical.SystemRoleNames` (`validator.go:40`, "keep in sync" by convention).
- `expense` is the **sole** production consumer of `ApprovalWorkflow` limits
  (`services/expense/service.go` `ApproveExpense` :68, `ApprovePurchaseOrder` :190).
  budget/project/reporting/inventory do not read it. Small blast radius.
- `Validate(yamlData)` (`validator.go:152`) validates many fields incl. `role_labels`
  coverage (:262) but **never** checks `limits[].role` against anything — the #199 gap.
  `make validate-verticals` runs `TestValidateAllProductionVerticals`
  (`schema_test.go:72`), which globs the real `configs/*.yaml` and asserts `Validate()`
  returns no errors — the natural home for the new rule.
- **Test blind spot:** `tests/integration/vertical/fixtures.go` and
  `services/expense/service_test.go`'s `movieProductionConfig()` hand-roll configs with
  already-RBAC-correct role names, so they never load the real (broken) YAMLs and cannot
  catch this class. A new test must load `pkg/vertical/configs/*.yaml`.

## Design

### 1. Config data — remap `limits[].role` to RBAC role names (3 files; movie unchanged)

Faithful remap using each config's own `role_labels` correspondence + amount rank.
`dual_approval_above` unchanged per vertical.

**`pkg/vertical/configs/construction.yaml`** (`coordinator`=Site Manager, `accountant`=Quantity Surveyor, `manager`=Project Director):
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

**`pkg/vertical/configs/events-management.yaml`** (`coordinator`=Event Coordinator, `accountant`=Finance Controller, `manager`=Event Director):
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

**`pkg/vertical/configs/software-development.yaml`** (`coordinator`=Tech Lead, `project_supervisor`=Project Manager, `manager`=Engineering Manager):
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

**`pkg/vertical/configs/movie-production.yaml`** — already correct (`coordinator` 200000,
`manager` 1000000, `dual_approval_above` 1000000). **No change.**

No `super_admin` entry in any config (handled by the code bypass below). All roles used
(coordinator/accountant/manager/project_supervisor) are in `SystemRoleNames`.

### 2. `super_admin` unlimited bypass (`services/expense/service.go`)

Add a private helper:
```go
// hasSuperAdmin reports whether the caller holds the super_admin RBAC role, which
// carries unlimited approval authority (it holds every permission; a per-role money
// limit on it is meaningless). Dual-approval threshold still applies to all callers.
func hasSuperAdmin(roles []string) bool {
	for _, r := range roles {
		if r == "super_admin" {
			return true
		}
	}
	return false
}
```
In **`ApproveExpense`**, wrap the limit check (the dual-approval check stays outside,
applying to everyone):
```go
	if !hasSuperAdmin(roles) {
		limit := vcfg.ApprovalWorkflow.MaxLimitForRoles(roles)
		if limit == nil {
			return fmt.Errorf("%w: caller roles %v have no configured approval limit", ErrApprovalLimitExceeded, roles)
		}
		if exp.Amount.GreaterThan(*limit) {
			return fmt.Errorf("%w: %s exceeds %s limit", ErrApprovalLimitExceeded, exp.Amount, *limit)
		}
	}

	// Dual approval threshold applies to everyone, including super_admin.
	if exp.Amount.GreaterThan(vcfg.ApprovalWorkflow.DualApprovalAbove) {
		return fmt.Errorf("%w: %s exceeds dual approval threshold %s", ErrDualApprovalRequired, exp.Amount, vcfg.ApprovalWorkflow.DualApprovalAbove)
	}
```
Apply the identical shape in **`ApprovePurchaseOrder`** against `po.Amount`. No new
import (helper uses a range loop). Check order otherwise unchanged
(already-approved → [limit unless super_admin] → dual-approval → set approved).

### 3. Guardrail — validator rule (`pkg/vertical/validator.go`)

In `Validate()`, after the existing `role_labels` coverage block (`:262-272`), add:
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
(`v.ApprovalWorkflow.Limits` is `[]ApprovalLimitYAML{Role, MaxAmount}` — `validator.go:98-106`.)

### 4. Tests

- **Validator negative** (`pkg/vertical/validator_test.go` or `schema_test.go`): a minimal
  YAML with `limits: [{role: bogus_role, max_amount: 1}]` → `Validate()` returns a
  `ValidationError` whose field is `vertical.approval_workflow.limits`. Prove the rule has
  teeth (assert the specific error, not just non-empty).
- **Real-YAML coverage** (`pkg/vertical`, new test): load each `configs/*.yaml`, and assert
  every `approval_workflow.limits[].role` ∈ `SystemRoleNames`. This is the test that would
  have caught #199 (existing fixtures hand-roll correct names). `TestValidateAllProductionVerticals`
  (`schema_test.go:72`) now also passes for all 4 configs (only movie did before) — confirm.
- **Service** (`services/expense/service_test.go`): (a) `super_admin` approves an expense whose
  amount exceeds every configured limit → success (bypass proof); (b) `super_admin` on an amount
  above `DualApprovalAbove` → still `ErrDualApprovalRequired` (dual-approval applies to all);
  (c) a non-super_admin `accountant` approves within its tier and is rejected above it — use a
  config with an `accountant` limit (construction-shaped: coordinator 100k / accountant 500k /
  manager 2M). Mirror an equivalent `super_admin`/`accountant` pair for `ApprovePurchaseOrder`.
- `go test ./pkg/vertical/... ./services/expense/... -race`; `go vet ./...`; `go build ./...`;
  `gofmt -l` on touched Go files (must be empty — CI Lint gate; go test/vet/build accept
  gofmt-dirty code). No proto/sqlc/migration → no codegen gates. `make validate-verticals`
  (= `TestValidateAllProductionVerticals`) green.

## Rollout / behavior change

For tenants already on construction/events/software, users holding
`coordinator`/`accountant`/`manager`/`project_supervisor` gain approval authority they did
not have (previously everyone fail-closed). `super_admin` gains unlimited approval in every
vertical. This is the intended fix, not a regression — but it is a real authorization
behavior change worth calling out to reviewers.

## Non-goals

- No schema/type change; `schema.json`'s `role` stays `minLength:1` (Go `Validate()` is what
  `make validate-verticals` enforces; a JSON-schema enum keyed to Go `SystemRoleNames` would
  duplicate the list — deferred).
- `tests/integration/vertical/fixtures.go` untouched (already RBAC-correct; the new pkg/vertical
  real-YAML test closes the guardrail without reworking the integration suite).
- No real two-party dual-approval flow — `DualApprovalAbove` keeps its existing "block single
  approval above threshold" behavior, and it applies to `super_admin` too.
- No change to `budget`/`project`/other services (they don't consume approval limits).
- No unification of `services/iam.systemRoles` ↔ `vertical.SystemRoleNames` (still two hand-synced
  lists; out of scope).

## Review weight

Touches money-approval **authorization** + industry configs → security-sensitive; senior
engineer required per CLAUDE.md. Standard 2 approvals + senior. Whole-branch review on the most
capable model.
