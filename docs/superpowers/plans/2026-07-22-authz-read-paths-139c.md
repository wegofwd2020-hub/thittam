# Read-path Authorization (project / budget / reporting) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Gate twelve tenant-data read RPCs on the `<resource>:read` permission each already-granted-but-never-checked string implies — 4 in project, 5 in budget, 3 in reporting-analytics — and wire reporting for authorization for the first time.

**Architecture:** project and budget already carry a `perm` field and gate their writes; their reads get one `RequirePermission` line each. reporting has no checker at all: it gains a required `NewHandler` parameter and a refuse-to-start IAM dial (the #149 ledger pattern), then gates its three fact RPCs. The two report-definition RPCs stay AUTH — they read vertical config, not tenant data.

**Tech Stack:** Go 1.25 (CI pins 1.25.12), gRPC, `google/uuid`, `stretchr/testify`.

**Spec:** `docs/superpowers/specs/2026-07-22-authz-read-paths-139-sliceC-design.md`
**Issue:** #139, slice C. **Branch:** `feat/authz-read-paths-139c`.

## Global Constraints

- **Security change touching authorization.** Senior review + 2 approvals. Every task ends green.
- **NO Docker. NO database.** Never run `docker compose … -v` / `down` / `up` against `infra/local/` — project-scoped, `-v` deletes ALL local volumes. A disposable `git worktree` OUTSIDE the repo is fine; remove it after. Integration tests SKIP without `THITTAM_TEST_DSN`; leave it unset.
- **Whole-tree `go vet ./...` is the gate.** reporting's `NewHandler` signature change breaks its only production call site (`cmd/reporting-analytics/main.go`); a focused package build will not catch it.
- `errcheck` runs in CI; `golangci-lint` is **not installed locally**. The deferred IAM close needs `defer func() { _ = closeIAM() }()`.
- `gh pr checks <n>` is the real gate. Local green is not CI green.
- **No migration. No new permission string. No proto change. `git diff --stat gen/` must be empty.**
- Coverage must not regress in `services/project`, `services/budget`, `services/reporting` (all in the `others ≥ 75%` tier). Record each baseline before touching it.
- **The denial-test rule (cost two rounds on slice A):** a denial test installs a `t.Fatal` on the **first** repository/service fn its gated path would reach on the happy path — not merely a status-code assertion. A status-code-only assertion can pass against ungated code when a mock default produces the same code by another route.
- **If the number of pre-existing tests that flip differs from a task's prediction, STOP and report.** A surprise means the reading was wrong (the discipline that caught defects in #146, #149 and slice A).

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `services/project/handler.go` | **Modify.** 4 gates. | 1 |
| `services/project/handler_test.go` | **Modify.** Add `denyPerm`; 4 denial tests; repair any flipped read test. | 1 |
| `services/budget/handler.go` | **Modify.** 5 gates. | 2 |
| `services/budget/handler_test.go` | **Modify.** Add `denyPerm`; 5 denial tests; repair any flipped read test. | 2 |
| `services/reporting/handler.go` | **Modify.** `perm` field, required `NewHandler` param, 3 gates. | 3 |
| `services/reporting/handler_test.go` | **Modify.** `allowAllPerm`/`denyPerm`, caller helper, repair 3 flipped tests, 3 denial tests. | 3 |
| `cmd/reporting-analytics/main.go` | **Modify.** Dial IAM; refuse to start on nil checker. | 3 |
| `docs/superpowers/specs/2026-07-22-authz-policy-table-139.md` | **Modify.** Correct the two report-definition rows to AUTH. | 4 |

Tasks 1 and 2 are independent gate-insertion. Task 3 is atomic (signature change breaks the build until `cmd/` is updated in the same commit). Task 4 is a doc correction, separable.

---

## Verified scaffolding facts

Established by reading the tree at the branch base. Trust these; do not re-derive.

- `interceptor.RequirePermission(ctx, checker, permission string) error` — takes no tenant and no projectID argument; derives projectID from `caller.ProjectID` internally. Needs `caller.UserID` in context (populated by `UnaryAuthInterceptor`). Returns `Unauthenticated` (no caller), `Internal` (nil checker), `PermissionDenied` (not allowed). Returns a `*status.Status` error — return it **directly**, never through `grpcErr`.
- **project and budget scope the tenant with `tenant.IDFromContext(ctx)`**, not `interceptor.TenantFromRequest`. The existing write RPCs gate immediately after the `if !ok { return Unauthenticated }` block (`CreateBudget`, `services/budget/handler.go:43-50`; `CreateProduction`, `services/project/handler.go:43-50`). Reads follow the same order.
- Both `services/project/handler_test.go` and `services/budget/handler_test.go` already declare `allowAllPerm{}` and wire it via `.WithPermissionChecker(allowAllPerm{})`. **Neither has a `denyPerm`.** Each task adds one.
- project read RPC lines: `GetProduction:77`, `ListProductions:95`, `ListPhases:215`, `ListCrewMembers:307`. First repo fn each: `getProductionFn`, `listProductionsFn`, `listPhasesFn`, `listCrewMembersFn` (mock in `service_test.go`).
- budget read RPC lines: `GetBudget:81`, `ListBudgets:99`, `GetLineItem:261`, `ListLineItems:274`, `CheckLineAvailability:327`. First repo fn each: `getBudgetFn`, `listBudgetsFn`, `getLineItemFn`, `listLineItemsFn`, `checkLineAvailabilityFn`.
- reporting handler struct (`services/reporting/handler.go:17`) has **no `perm` field**; `NewHandler(svc)` takes one arg. `cmd/reporting-analytics/main.go:81` calls `reporting.NewHandler(svc)`.
- reporting gated-RPC lines: `GetExpenseFacts:47`, `GetBudgetFacts:70`, `GetDashboardSummary:93`. First repo fn each: `getExpenseFactsFn`, `getBudgetFactsFn`, `getDashboardSummaryFn`.
- reporting NON-gated (stay AUTH): `GetReportDefinition:30`, `ListReportDefinitions:38`. They call `vertical.MustFromContext` only — no repo, no tenant, no caller needed.
- reporting tests inject no caller: `ctxWithVertical()` (`service_test.go:129`) = `vertical.WithConfig(context.Background(), …)`; `ctxWithTenant(tid)` = `tenant.WithID(ctxWithVertical(), tid)`. Neither calls `interceptor.WithCaller`. So the three gated RPCs' `_Success` tests flip to `Unauthenticated` once gated.
- `iamclient.DialFromEnv(serviceName string) (*iamclient.PermissionChecker, func() error, error)` — returns `(nil, no-op, nil)` when `IAM_SERVICE_ADDR` unset. `iamclient.EnvAddr` is the env-var name. The nil check must be on the concrete `*iamclient.PermissionChecker` before it is assigned to the interface field (typed-nil trap).
- `cmd/general-ledger/main.go` is the reference for the reporting startup block (#149).

## Traps

1. **reporting is atomic.** `NewHandler`'s new required parameter breaks `cmd/reporting-analytics/main.go` — that fix lands in Task 3's single commit or the commit does not build (the un-bisectable lesson from #149's Task 3).
2. **Do not gate the two report-definition RPCs.** They read vertical config; gating them would break every reporting UI's menu (decision D3) and would also need a caller they currently never receive. If a denial test is written for them, it is wrong.
3. **reporting's `_Success` tests for the three fact RPCs will flip** to `Unauthenticated` — the tests pass a tenant but no caller. Repair by adding a caller helper AND an `allowAllPerm{}` checker. Do not blank the assertion.
4. **`GetDashboardSummary` calls `vertical.MustFromContext` before the repo.** Its `t.Fatal` tripwire is `getDashboardSummaryFn`, and its denial test context must still carry the vertical config (use the caller helper layered on `ctxWithVertical`), or it panics on the missing vertical instead of denying — a false green.

---

## Task 1: Gate the four project reads

**Files:** Modify `services/project/handler.go`; modify `services/project/handler_test.go`.

**Interfaces:**
- Consumes: `interceptor.RequirePermission` (exists); `allowAllPerm` (exists in the test file).
- Produces: nothing new.

- [ ] **Step 1: Add `denyPerm` and write the four failing denial tests**

In `services/project/handler_test.go`, after the existing `allowAllPerm` declaration, add:

```go
// denyPerm denies every permission, so a denial test can prove the gate fires
// before the repository is reached.
type denyPerm struct{}

func (denyPerm) CheckPermission(_ context.Context, _ uuid.UUID, _ string, _ *uuid.UUID) (bool, error) {
	return false, nil
}
```

Then append (using the same `mockRepo`, `ctxWithTenant`-equivalent, and `NewHandler(...).WithPermissionChecker(...)` construction the file already uses — read the existing `_Success` tests for the exact context helper name and mock field types before writing these):

```go
func TestHandler_GetProduction_Denied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getProductionFn: func(context.Context, uuid.UUID, uuid.UUID) (*Production, error) {
			t.Fatal("gate must fire before the repository is read")
			return nil, nil
		},
	})).WithPermissionChecker(denyPerm{})

	_, err := h.GetProduction(ctxWithTenant(tenantID), &projectv1.GetProductionRequest{
		Id: uuid.New().String(),
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}
```

Repeat for `ListProductions` (`listProductionsFn`), `ListPhases` (`listPhasesFn`), `ListCrewMembers` (`listCrewMembersFn`), each with the `t.Fatal` on that RPC's first repo fn. Use the exact fn signatures from `services/project/service_test.go:19-…`. For the context: `ctxWithTenant(tenantID)` (`handler_test.go:28`) already injects a caller via `interceptor.WithCaller` (`:30`), so `RequirePermission` reaches the deny rather than short-circuiting at `Unauthenticated`. Use it.

- [ ] **Step 2: Run the denial tests — expect FAIL**

Run: `go test ./services/project/ -run '_Denied' -v 2>&1 | tail -20`
Expected: all four FAIL — the ungated handler reaches the repo and fires `t.Fatal`.

- [ ] **Step 3: Insert the four gates**

In `services/project/handler.go`, in each of `GetProduction` (`:77`), `ListProductions` (`:95`), `ListPhases` (`:215`), `ListCrewMembers` (`:307`), immediately after the `tenant.IDFromContext` `if !ok` block and before any `uuid.Parse`, insert:

```go
	if err := interceptor.RequirePermission(ctx, h.perm, "production:read"); err != nil {
		return nil, err
	}
```

Return the error directly; do not wrap it in `grpcErr`.

- [ ] **Step 4: Run the package — expect PASS, and note any flipped tests**

Run: `go test ./services/project/ 2>&1 | tail -15`
The four `_Denied` tests now pass. **Verified fact:** several read `_Success` tests construct the handler as `NewHandler(NewService(&mockRepo{…}))` with **no** `.WithPermissionChecker` (e.g. `TestHandler_GetProduction_Success`, `handler_test.go:72`), so `h.perm` is nil and the gate now returns `Internal` (`RequirePermission`'s nil-checker branch). Others that used `newHandler()` (which wires `allowAllPerm`) but a caller-less `ctxWithVertical()` return `Unauthenticated`. **Read each failure**: repair by wiring `allowAllPerm{}` and a caller, never by changing an assertion. Count them; if the count is not what you found by reading the tests in Step 1, STOP and report.

- [ ] **Step 5: Verify**

Run: `go vet ./...` — clean.
Run: `go test -race ./services/project/` — PASS.
Run: `gofmt -l services/project/handler.go services/project/handler_test.go` — no output.
Run: `go test -cover ./services/project/` — record; must not regress the baseline you took at Step 1.

- [ ] **Step 6: Teeth check**

```bash
WT="${TMPDIR:-/tmp}/teeth-139c-proj-$$"
rm -rf "$WT"; git worktree add --detach -q "$WT" HEAD
cp services/project/handler_test.go "$WT/services/project/handler_test.go"
(cd "$WT" && go test ./services/project/ -run '_Denied' 2>&1 | tail -15)
git worktree remove "$WT" --force; git worktree prune
```
Expected: all four `_Denied` tests FAIL against the ungated handler (each fires its `t.Fatal`). `git worktree list` shows no leftovers afterward.

- [ ] **Step 7: Commit**

```bash
git add services/project/handler.go services/project/handler_test.go
git commit -m "fix(project): gate the four production read RPCs on production:read (#139)

GetProduction, ListProductions, ListPhases and ListCrewMembers returned
tenant data to any authenticated member. production:read was granted in
systemRoles and checked nowhere; the gates make the existing grant live.
No new permission string, no migration. Each denial test trips a t.Fatal
on the RPC's first repository call, so it fails against the ungated
handler rather than passing on a coincidental status code.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Gate the five budget reads

**Files:** Modify `services/budget/handler.go`; modify `services/budget/handler_test.go`.

Identical shape to Task 1. Permission is `"budget:read"`. RPCs and first-repo-fn tripwires:

| RPC | handler.go | t.Fatal on |
|---|---|---|
| `GetBudget` | `:81` | `getBudgetFn` |
| `ListBudgets` | `:99` | `listBudgetsFn` |
| `GetLineItem` | `:261` | `getLineItemFn` |
| `ListLineItems` | `:274` | `listLineItemsFn` |
| `CheckLineAvailability` | `:327` | `checkLineAvailabilityFn` |

- [ ] **Step 1: Add `denyPerm` (same code as Task 1) and five failing denial tests**, each with the `t.Fatal` on the fn above, a caller in context, and `.WithPermissionChecker(denyPerm{})`. Use the exact fn signatures from `services/budget/service_test.go:19-26`. Read a passing `_Success` test first for the context helper.

- [ ] **Step 2: Run — expect all five FAIL.** `go test ./services/budget/ -run '_Denied' -v 2>&1 | tail -24`

- [ ] **Step 3: Insert five gates** after each RPC's `tenant.IDFromContext` block, before any parse:
```go
	if err := interceptor.RequirePermission(ctx, h.perm, "budget:read"); err != nil {
		return nil, err
	}
```

- [ ] **Step 4: Run the package — expect PASS.** Repair any flipped `_Success` test by wiring `allowAllPerm{}` + a caller; do not weaken assertions; if the flip count surprises you, STOP and report.

- [ ] **Step 5: Verify** — `go vet ./...` clean; `go test -race ./services/budget/` PASS; `gofmt -l` on the two files empty; coverage not regressed.

- [ ] **Step 6: Teeth check** — same as Task 1 with `budget` substituted and worktree name `teeth-139c-budget-$$`.

- [ ] **Step 7: Commit**
```bash
git add services/budget/handler.go services/budget/handler_test.go
git commit -m "fix(budget): gate the five budget read RPCs on budget:read (#139)

GetBudget, ListBudgets, GetLineItem, ListLineItems and
CheckLineAvailability returned tenant data to any authenticated member.
budget:read was granted in systemRoles and checked nowhere. No new
permission string, no migration. Each denial test trips a t.Fatal on the
RPC's first repository call.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 3: Wire reporting for authorization and gate its three fact RPCs

**Files:** Modify `services/reporting/handler.go`, `services/reporting/handler_test.go`, `cmd/reporting-analytics/main.go`.

**Interfaces:**
- Consumes: `interceptor.RequirePermission`, `interceptor.PermissionChecker`, `iamclient.DialFromEnv`, `iamclient.EnvAddr`.
- Produces: `reporting.NewHandler(svc *Service, perm interceptor.PermissionChecker) *Handler` — required parameter.

**Atomic:** the signature change breaks `cmd/reporting-analytics/main.go`; both land in one commit.

- [ ] **Step 1: Add the test doubles, caller helper, and the three failing denial tests**

In `services/reporting/handler_test.go`, add:

```go
type allowAllPerm struct{}

func (allowAllPerm) CheckPermission(context.Context, uuid.UUID, string, *uuid.UUID) (bool, error) {
	return true, nil
}

// denyPerm proves a gate fires before the repository is reached.
type denyPerm struct{}

func (denyPerm) CheckPermission(context.Context, uuid.UUID, string, *uuid.UUID) (bool, error) {
	return false, nil
}

// ctxWithCaller layers a verified caller onto the vertical+tenant context, as
// UnaryAuthInterceptor would. RequirePermission needs caller.UserID.
func ctxWithCaller(tenantID uuid.UUID) context.Context {
	return interceptor.WithCaller(ctxWithTenant(tenantID), interceptor.CallerInfo{
		UserID:   uuid.New(),
		TenantID: tenantID,
		Roles:    []string{"member"},
	})
}
```

Then the three denial tests, each `t.Fatal` on its first repo fn and constructed with `NewHandler(NewService(&mockRepo{…}), denyPerm{})`:

```go
func TestHandler_GetExpenseFacts_Denied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	h := NewHandler(NewService(&mockRepo{
		getExpenseFactsFn: func(context.Context, uuid.UUID, uuid.UUID) ([]ExpenseFact, error) {
			t.Fatal("gate must fire before the repository is read")
			return nil, nil
		},
	}), denyPerm{})

	_, err := h.GetExpenseFacts(ctxWithCaller(tenantID), &reportingv1.GetExpenseFactsRequest{
		ProductionId: uuid.New().String(),
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}
```

Repeat for `GetBudgetFacts` (`getBudgetFactsFn`) and `GetDashboardSummary` (`getDashboardSummaryFn`). `GetDashboardSummary`'s context must still carry the vertical config — `ctxWithCaller` layers on `ctxWithTenant`→`ctxWithVertical`, so it does; do not use a bare `context.Background()` caller, or the handler panics in `vertical.MustFromContext` before the gate.

- [ ] **Step 2: Run — expect the three FAIL to COMPILE** (`NewHandler` still takes one arg). That is the atomic signal.

Run: `go test ./services/reporting/ 2>&1 | head -10`
Expected: build failure, `not enough arguments in call to NewHandler`.

- [ ] **Step 3: Add the `perm` field and required parameter**

In `services/reporting/handler.go`, replace the struct and constructor (`:17-25`):

```go
type Handler struct {
	reportingv1.UnimplementedReportingServiceServer
	svc  *Service
	perm interceptor.PermissionChecker
}

// NewHandler creates a Handler wrapping the given Service.
//
// perm is required, not optional. cmd/reporting-analytics refuses to start when the
// checker is nil, so a live handler always has one; a nil here is a test or a bug, and
// RequirePermission fails such a call closed with Internal.
func NewHandler(svc *Service, perm interceptor.PermissionChecker) *Handler {
	return &Handler{svc: svc, perm: perm}
}
```

Add the `interceptor` import if not present (it likely is — check).

- [ ] **Step 4: Gate the three fact RPCs**

In `GetExpenseFacts` (`:47`), `GetBudgetFacts` (`:70`), `GetDashboardSummary` (`:93`), immediately after the `tenant.IDFromContext` `if !ok` block and before any parse:

```go
	if err := interceptor.RequirePermission(ctx, h.perm, "report:read"); err != nil {
		return nil, err
	}
```

Leave `GetReportDefinition` (`:30`) and `ListReportDefinitions` (`:38`) untouched — they stay AUTH.

- [ ] **Step 5: Wire `cmd/reporting-analytics/main.go`**

Add `"github.com/wegofwd2020/thittam/pkg/iamclient"` to imports. Replace `handler := reporting.NewHandler(svc)` (`:81`) with the block from the spec §3.2 (dial, `log.Fatalf` on error, `defer func() { _ = closeIAM() }()`, nil check, `NewHandler(svc, iamPerm)`). Copy the exact shape from `cmd/general-ledger/main.go`.

- [ ] **Step 6: Repair the three flipped `_Success` tests and every other `NewHandler(` call site**

Every `NewHandler(NewService(&mockRepo{…}))` in `handler_test.go` becomes `NewHandler(NewService(&mockRepo{…}), allowAllPerm{})`. The `newHandler()` helper (`:20`) becomes `NewHandler(NewService(&mockRepo{}), allowAllPerm{})`.

The three fact RPCs' `_Success` tests additionally need a caller — change their context from `ctxWithTenant(tenantID)` to `ctxWithCaller(tenantID)`. The two definition-RPC tests (`GetReportDefinition`, `ListReportDefinitions`) keep `ctxWithVertical()` and get only the `allowAllPerm{}` constructor argument — they are not gated, so they need no caller. If any test other than the three fact `_Success` tests needs a caller added, STOP and report — it means an RPC is gated that should not be.

- [ ] **Step 7: Run and verify**

Run: `go test ./services/reporting/ 2>&1 | tail -15` — PASS.
Run: `go test ./services/reporting/ -run '_Denied' -v 2>&1 | tail -12` — three PASS.
Run: `go build ./cmd/reporting-analytics/` — builds.
Run: `go vet ./...` — clean.
Run: `go test -race ./services/reporting/` — PASS.
Run: `gofmt -l services/reporting/handler.go services/reporting/handler_test.go cmd/reporting-analytics/main.go` — no output.
Run: `go test -cover ./services/reporting/` — record; not regressed.

- [ ] **Step 8: Teeth check**

The signature change means `HEAD` (pre-commit) will not compile the new tests — so test against a worktree at HEAD with ONLY the handler+cmd reverted is not clean. Instead, after committing (Step 9), verify teeth by neutering the gate in a scratch worktree at the commit:

```bash
WT="${TMPDIR:-/tmp}/teeth-139c-rep-$$"
rm -rf "$WT"; git worktree add --detach -q "$WT" HEAD
python3 - "$WT" <<'PY'
import sys, pathlib
p = pathlib.Path(sys.argv[1]) / "services/reporting/handler.go"
s = p.read_text()
s = s.replace('if err := interceptor.RequirePermission(ctx, h.perm, "report:read"); err != nil {\n\t\treturn nil, err\n\t}',
              '// TEETH: gate neutered', 3)
p.write_text(s)
PY
(cd "$WT" && go test ./services/reporting/ -run '_Denied' 2>&1 | tail -15)
git worktree remove "$WT" --force; git worktree prune
```
Expected: all three `_Denied` tests FAIL (fire their `t.Fatal`). Confirm `git worktree list` clean. (This runs after Step 9 — do the commit first, then this, then amend only if it reveals a problem.)

- [ ] **Step 9: Commit (before Step 8's check; then run Step 8 against it)**

```bash
git add services/reporting/handler.go services/reporting/handler_test.go cmd/reporting-analytics/main.go
git commit -m "fix(reporting): gate the fact RPCs on report:read; refuse to start without IAM (#139)

reporting-analytics enforced no authorization and never dialled IAM.
GetExpenseFacts, GetBudgetFacts and GetDashboardSummary returned tenant
financial data to any authenticated member. They now require report:read.

NewHandler takes the checker as a required parameter and
cmd/reporting-analytics refuses to start without it, the #149 ledger
pattern — forgetting the checker is a build error, not a service that
returns Internal on every call. This makes reporting the second
fail-closed service. IAM_SERVICE_ADDR already reaches the pod via the
shared configmap, so no manifest change.

GetReportDefinition and ListReportDefinitions stay AUTH: they read the
vertical's report catalogue, not tenant data (decision D3).

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 4: Correct the policy-table rows

**Files:** Modify `docs/superpowers/specs/2026-07-22-authz-policy-table-139.md`.

- [ ] **Step 1: Fix the two reporting rows.** In §4.10, change `GetReportDefinition` and `ListReportDefinitions` from `report:read` / R1 to AUTH / R3, with a note that they read the vertical report catalogue, not tenant data — the same class as `GetBudgetTemplates`. Leave `GetExpenseFacts`, `GetBudgetFacts`, `GetDashboardSummary` on `report:read`. Update the section's count if it states one.

- [ ] **Step 2: Commit**
```bash
git add docs/superpowers/specs/2026-07-22-authz-policy-table-139.md
git commit -m "docs(security): report definitions are config, not tenant data — AUTH not report:read (#139)

GetReportDefinition and ListReportDefinitions read vertical config via
vertical.MustFromContext and touch no repository and no tenant. They are
the report catalogue for the vertical, the same class as
GetBudgetTemplates, so decision D3 makes them AUTH. Found while
implementing slice C, which gates only the three fact RPCs.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Verification (whole branch, before PR)

- [ ] `go vet ./...` — clean.
- [ ] `go test ./... -short` — PASS.
- [ ] `go test -race ./services/project/ ./services/budget/ ./services/reporting/` — PASS.
- [ ] `go build ./cmd/...` — all ten entrypoints build (reporting's signature change).
- [ ] `git diff --stat gen/` — empty. `git diff --stat <base>..HEAD -- migrations/` — empty.
- [ ] `grep -c 'RequirePermission' services/project/handler.go` increased by 4; `services/budget/handler.go` by 5; `services/reporting/handler.go` is 3.
- [ ] `grep -c 'report:read' services/reporting/handler.go` — 3, and NOT present in `GetReportDefinition`/`ListReportDefinitions`.
- [ ] Coverage project/budget/reporting — no regression. Record before and after per service.
- [ ] `gofmt -l` on every touched file — no output.
- [ ] **`gh pr checks <n>` after opening the PR.**

## What this does not fix

- **Inventory reads** — slice D, with the D10 backfill (`inventory:read` grant matrix is wrong).
- **Tenant isolation of the read paths** — reporting scopes tenant via `tenant.IDFromContext` (the `X-Tenant-ID` header), not the verified token. #139 §3 / slice H. This slice adds a permission gate on top of whatever tenant scoping exists; it does not certify that scoping.
- **The four log-and-proceed services' startup behaviour** — only reporting converts.
- **iam completion (B), expense (D), document/billing/notifications (E/F/G).**
