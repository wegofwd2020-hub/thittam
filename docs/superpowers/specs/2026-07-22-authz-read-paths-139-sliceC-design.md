# Read-path authorization — project, budget, reporting — design

**Issue:** #139, slice C. **Branch:** `feat/authz-read-paths-139c`, base = current `main`.
**Follows:** #138 (authentication), #144 (tenant boundary), #146 (role-assignment), #149 (ledger authorization + `ActorFromRequest`), #139 slice A (`ChangePassword`).
**Policy table:** `docs/superpowers/specs/2026-07-22-authz-policy-table-139.md` (rules R1 and R3), now on `main`.

## 1. The problem

Twelve read RPCs across three services return tenant business data to any authenticated member, with no permission check:

- **project (4):** `GetProduction`, `ListProductions`, `ListPhases`, `ListCrewMembers`
- **budget (5):** `GetBudget`, `ListBudgets`, `GetLineItem`, `ListLineItems`, `CheckLineAvailability`
- **reporting-analytics (3):** `GetExpenseFacts`, `GetBudgetFacts`, `GetDashboardSummary`

**`GetReportDefinition` and `ListReportDefinitions` are deliberately excluded.** They read only vertical config — `Service.GetReportDefinition` calls `vertical.MustFromContext(ctx).FindReportDefinition(...)` (`services/reporting/service.go:22-30`) and touches no repository and no tenant. They are the *catalogue of report types for the vertical*, the same class as `GetBudgetTemplates` / `GetPhaseTypes`, so rule R3 / decision D3 makes them AUTH, not `report:read`. The policy table currently lists them under `report:read`; that row is an error (a report definition is configuration, not tenant data) and is corrected on this slice's branch, the same way slice A corrected the `GetCurrentUser` row.

Each is tenant-scoped, so this is not a cross-tenant read — after #138 the tenant comes from a verified source. But rule R1 of the policy table requires a read of tenant business data to hold `<resource>:read`, and none of these check anything. A `member` who holds only `production:read` today can also read every budget and every report in the tenant, because `budget:read` and `report:read` are enforced nowhere.

## 2. Why this slice needs no new vocabulary and no migration

Three permission strings already exist in `systemRoles`, granted and checked nowhere:

| Permission | super_admin | manager | coordinator | accountant | member | project_supervisor |
|---|:-:|:-:|:-:|:-:|:-:|:-:|
| `production:read` | ✓ | ✓ | ✓ | — | ✓ | ✓ |
| `budget:read` | ✓ | ✓ | ✓ | ✓ | — | ✓ |
| `report:read` | ✓ | ✓ | ✓ | ✓ | — | — |

All three are granted to a sensible set. Wiring the gates makes the existing grants live; it invents no string and edits no role, so `seedSystemRoles` is untouched and **no cross-schema backfill is needed**. That is the defining property of this slice and the reason it is sequenced first among the gating slices.

### 2.1 Why inventory is NOT in this slice

`inventory:read` is granted to **`inventory_manager` only** — not even `super_admin`. Gating `GetAsset` / `ListAssets` / `ListCheckouts` on it would lock out every role that can currently check an asset out (`inventory:checkout` is held by super_admin, manager, coordinator and project_supervisor). That is the grant matrix being wrong, and fixing it means editing `systemRoles`, which reaches new tenants only — the D10 backfill this slice exists to avoid.

Inventory's three reads move to the slice that builds the backfill (slice D). This is recorded against policy-table decision **D7** (`inventory:read`/`inventory:retire` are partly dead vocabulary): the read string is not dead, but its grant set is wrong.

### 2.2 Vertical-configuration lookups stay AUTH (decision D3)

`GetEntityLabels`, `GetPhaseTypes` (project), `GetBudgetCategories`, `GetBudgetTemplates` (budget) are **not** gated. They ignore their request entirely (`_ *Request`), read vertical config from the context, and every form needs them to render. Per rule R3 and decision D3 they require only authentication, which the interceptor chain already enforces. Confirmed in code: `GetEntityLabels` (`services/project/handler.go:346`) and `GetBudgetCategories` (`services/budget/handler.go:345`) call neither a repository nor `TenantFromRequest`.

## 3. Design

The slice splits by prior wiring state, not by service.

### 3.1 project and budget — gate insertion only

Both handlers **already** carry `perm interceptor.PermissionChecker` and the `WithPermissionChecker` setter, and both already gate their write RPCs. The read RPCs simply lack the guard. Insert, in each of the nine read RPCs, immediately after the existing tenant block and before any `uuid.Parse`:

```go
	if err := interceptor.RequirePermission(ctx, h.perm, "budget:read"); err != nil {
		return nil, err
	}
```

**Guard order is tenant → permission → parse**, matching the write RPCs in the same file (`CreateBudget`, `services/budget/handler.go:43-50`, gates immediately after the tenant block). These services scope the tenant with `tenant.IDFromContext(ctx)` — the `X-Tenant-ID` path — **not** `interceptor.TenantFromRequest`; the gate goes after that existing `if !ok` block. An unauthorized caller therefore learns nothing about whether their ids were well-formed.

The permission strings are inline literals, matching how these services already pass `"production:write"` / `"budget:write"` to `RequirePermission`. No constants — that convention belongs to services that own their vocabulary (iam, ledger); project and budget pass literals today and this slice does not change that.

### 3.2 reporting-analytics — full authorization wiring (3 gated + 2 AUTH)

reporting has **no** `perm` field, and `cmd/reporting-analytics/main.go:81` never dials IAM. It needs the wiring #149 built for the ledger. Following #149 and the two decisions taken for this slice:

```go
type Handler struct {
	reportingv1.UnimplementedReportingServiceServer
	svc  *Service
	perm interceptor.PermissionChecker
}

// NewHandler creates a Handler wrapping the given Service.
//
// perm is required, not optional. The four log-and-proceed services take it via a
// WithPermissionChecker setter, which compiles fine when nobody calls it — yielding a
// handler whose every RPC returns Internal. Here, forgetting the checker is a build error.
// cmd/reporting-analytics refuses to start when the checker is nil.
func NewHandler(svc *Service, perm interceptor.PermissionChecker) *Handler {
	return &Handler{svc: svc, perm: perm}
}
```

`cmd/reporting-analytics/main.go` gains the dial and refuses to start without a checker, exactly as `cmd/general-ledger` does:

```go
	iamPerm, closeIAM, err := iamclient.DialFromEnv("reporting-analytics")
	if err != nil {
		log.Fatalf("reporting-analytics: startup: dial IAM: %v", err)
	}
	defer func() { _ = closeIAM() }()
	if iamPerm == nil {
		log.Fatalf("reporting-analytics: startup: %s is not set; the service cannot authorize without a permission checker", iamclient.EnvAddr)
	}
	handler := reporting.NewHandler(svc, iamPerm)
```

`iamPerm` is the concrete `*iamclient.PermissionChecker` at the nil check, so this is a plain pointer comparison, not the typed-nil trap. `IAM_SERVICE_ADDR` already reaches this pod via `infra/k8s/config/configmap.yaml` and the shared `envFrom: configMapRef: thittam-common` block (`infra/k8s/services/reporting-analytics.yaml:68-70`), so **no manifest change is needed**.

Then gate the **three** fact RPCs (`GetExpenseFacts`, `GetBudgetFacts`, `GetDashboardSummary`) on `"report:read"`. reporting scopes the tenant with `tenant.IDFromContext` (`GetExpenseFacts`, `handler.go:48`), so the gate goes after that `if !ok` block, before any parse. `GetReportDefinition` and `ListReportDefinitions` are left AUTH (§1) — they are not gated, and because they never call `RequirePermission` they need no caller in context.

reporting's existing handler tests inject no caller — `ctxWithVertical()` (`service_test.go:129`) carries only the vertical config, and `ctxWithTenant` builds on it. Once the three fact RPCs are gated, their `_Success` tests will fail with `Unauthenticated`, because `RequirePermission` needs `caller.UserID`. Task 3 adds a caller-injecting helper (mirroring `callerCtx` in the other services) and repairs those three tests by wiring both a caller and an `allowAllPerm{}` checker. The two definition-RPC tests are untouched, since those RPCs stay AUTH.

This makes reporting the **second** service to adopt fail-closed startup (2 fail-closed: ledger, reporting; 4 log-and-proceed: project, budget, expense, inventory). Converting the other four is out of scope, as it was for #149 — it carries a five-service deploy-ordering hazard.

### 3.3 What does not change

`RequirePermission` needs `caller.UserID`, which `UnaryAuthInterceptor` populates. reporting authenticates (`cmd/reporting-analytics/main.go:104`), so the caller is present. No service or repository layer changes in any of the three services. No proto change. No migration.

## 4. Testing

Each newly gated RPC gets a denial test proving the gate fires **before** the service or repository is reached. project and budget have an `allowAllPerm` double but **no** `denyPerm`; each task adds one. reporting has neither; Task 3 adds both.

**The denial-test rule, learned at cost in slice A:** a denial test must install a `t.Fatal` on the **first** repository or service call its path should never reach — not merely assert a status code. A status-code-only assertion can pass against ungated code when the mock's default return happens to produce the same code by another route (slice A's `grpcError` mapping `ErrInvalidCredentials` → `Unauthenticated`). For each gated read, install the `t.Fatal` on the repository fn that RPC would call on the happy path (`getBudgetFn`, `listBudgetsFn`, `getExpenseFactsFn`, …), and confirm which fn that is by reading the handler.

**Guard-order preservation.** project and budget scope tenant with `tenant.IDFromContext`, which returns `Unauthenticated` on a missing tenant — so an existing `_InvalidTenantID`-style test short-circuits before the gate and is unaffected. Any existing read test that passes a valid tenant but no permission-granting checker will flip once the gate lands; repair by wiring `allowAllPerm{}`, never by weakening an assertion. Predict the count per task by reading the tests before changing the handler, and **if the actual count differs, stop and report** — a surprise means the reading was wrong (the discipline that caught defects in #146, #149 and slice A).

**Teeth check per task:** in a scratch `git worktree` OUTSIDE the repo at the task's parent commit, copy the task's test file over and confirm every new denial test fails against the ungated handler. Remove the worktree afterwards; confirm `git worktree list` is clean.

**Coverage** must not regress in any of the three services. Record the baseline per service before starting.

## 5. Constraints

- Security change touching authorization. Senior review; 2 approvals. `general-ledger`/`iam`/security changes need a senior engineer per `CLAUDE.md` — reporting is a security change here.
- **No Docker, no database.** Never `docker compose … -v` / `down` / `up` against `infra/local/`. A disposable worktree outside the repo is fine; remove it after.
- Whole-tree `go vet ./...` is the gate. reporting's `NewHandler` signature change breaks its only production call site (`cmd/reporting-analytics/main.go`) — that fix lands in the **same commit** as the signature change, or the commit does not build. (This is the un-bisectable-commit lesson from #149's Task 3.)
- `errcheck` runs in CI; `golangci-lint` is not installed locally. The deferred IAM close needs `defer func() { _ = closeIAM() }()`.
- No migration. No new permission string. No proto field added, removed or renumbered. `git diff --stat gen/` must be empty.
- Coverage floors: project/budget/reporting are in the `others ≥ 75%` tier; do not regress the current figure.
- `gh pr checks` before declaring the PR ready.

## 6. Out of scope

- **Inventory reads** (3 RPCs) — deferred to slice D with the D10 backfill (§2.1).
- **The four log-and-proceed services keeping their startup behaviour.** Only reporting converts here.
- **Proving the read paths are tenant-isolated.** reporting scopes tenant via `tenant.IDFromContext` (the `X-Tenant-ID` header), not the verified token's tenant claim. Whether that header is trustworthy is #139 §3 / slice H, not this slice. This slice adds a permission gate on top of whatever tenant scoping exists; it does not change or certify that scoping.
- **iam completion** (slice B), **expense reads** (slice D), **document/billing/notifications** (E/F/G).
