# Required permission checker (#167) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the IAM permission checker a required constructor param for `project`/`budget`/`expense`/`inventory` (omitting it = build error) and make each `cmd/*` refuse to start when the dial yields no checker — so a service that can't authorize refuses to exist instead of serving `codes.Internal` on every gated RPC. Also correct the false `dial.go` log.

**Architecture:** Four near-identical per-service conversions mirroring the proven ledger/reporting/billing/document/notifications convention. Each service's change (handler.go + cmd/*/main.go + handler_test.go) is atomic and builds green. Task 1 also carries the shared `pkg/iamclient/dial.go` log/doc fix.

**Tech Stack:** Go 1.25, `interceptor.PermissionChecker`, `iamclient.DialFromEnv`, testify.

## Global Constraints

- **The target pattern is fixed** — mirror `services/ledger/handler.go` `NewHandler(svc, perm)` (no setter) and `cmd/general-ledger/main.go`'s two-step dial check (`err != nil → Fatalf`; `iamPerm == nil → Fatalf`; then `NewHandler(svc, iamPerm)`). Do not invent variations.
- **`DialFromEnv` behavior is UNCHANGED** — it still returns `(nil, noop, nil)` for an unset addr; only its log line + doc comment change. The 4 `cmd/*` add the missing `iamPerm == nil` Fatalf.
- **Per-service atomicity:** `handler.go` (signature change + delete setter) + `cmd/<svc>/main.go` (Fatalf + `NewHandler(svc, iamPerm)`, drop the `!= nil` block) + `handler_test.go` (migrate every call site) land in ONE commit — `go build ./...` catches the cmd site, `go vet`/`go test` catch the tests. No hidden e2e/integration doubles exist (grounding confirmed).
- **Test migration rule:** `NewHandler(NewService(&mockRepo{…})).WithPermissionChecker(x)` → `NewHandler(NewService(&mockRepo{…}), x)`; bare `NewHandler(NewService(&mockRepo{…}))` → `NewHandler(NewService(&mockRepo{…}), nil)`. Keep the `_NoPermissionChecker_Denies` sentinel tests (nil still fails closed → `Internal`).
- No proto/sqlc/migration/k8s. Gate every task with `gofmt -l <touched .go>` (some of these files carry pre-existing gofmt debt on `main` — diff-vs-main, add none new) + `go vet ./...` + `go build ./...`.
- **Security-sensitive** (startup authz guarantee) → senior review per CLAUDE.md.
- Commits Conventional-Commits (scope per service: `project`/`budget`/`expense`/`inventory`), ending `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

---

## File Structure

| File | Task |
|---|---|
| `pkg/iamclient/dial.go` (log + doc), `services/project/handler.go`, `cmd/project-management/main.go`, `services/project/handler_test.go` | 1 |
| `services/budget/handler.go`, `cmd/budget-planning/main.go` (+ stale comment), `services/budget/handler_test.go` | 2 |
| `services/expense/handler.go`, `cmd/expense-tracking/main.go`, `services/expense/handler_test.go` | 3 |
| `services/inventory/handler.go`, `cmd/inventory-management/main.go`, `services/inventory/handler_test.go` | 4 |

---

### Task 1: project + shared dial.go log/doc fix

**Files:** Modify `pkg/iamclient/dial.go`, `services/project/handler.go`, `cmd/project-management/main.go`, `services/project/handler_test.go`

- [ ] **Step 1: dial.go log + doc**

In `pkg/iamclient/dial.go`, replace the false log (`:32`) with the truth:
```go
	log.Printf("%s: %s unset — no IAM permission checker; gated RPCs fail closed with Internal (#138). A service that requires authz should refuse to start.", serviceName, EnvAddr)
```
Correct the stale doc comment (`:22-25`) — drop "services keep serving without IAM as a hard dependency"; state instead that an unset addr yields a nil checker and the CALLER decides (gated services `Fatalf`; #138 makes a nil checker fail closed). Do NOT change the return values.

- [ ] **Step 2: project handler.go**

`services/project/handler.go`: change `func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }` to:
```go
// NewHandler creates a Handler wrapping the given Service. perm is required, not
// optional: a nil here is a test or a bug, and RequirePermission fails such a call
// closed with Internal (#138). cmd/project-management refuses to start on a nil checker.
func NewHandler(svc *Service, perm interceptor.PermissionChecker) *Handler {
	return &Handler{svc: svc, perm: perm}
}
```
**Delete** the `WithPermissionChecker` method. Keep the `perm` field (comment already notes fail-closed).

- [ ] **Step 3: cmd/project-management/main.go**

After the existing `iamPerm, closeIAM, err := iamclient.DialFromEnv("project-management")` + `if err != nil {…Fatalf…}` + `defer func() { _ = closeIAM() }()`, add:
```go
	if iamPerm == nil {
		log.Fatalf("project-management: startup: %s is not set; project-management cannot authorize without a permission checker", iamclient.EnvAddr)
	}
```
Replace `handler := project.NewHandler(svc)` + the `if iamPerm != nil { handler = handler.WithPermissionChecker(iamPerm) }` block with:
```go
	handler := project.NewHandler(svc, iamPerm)
```

- [ ] **Step 4: project handler_test.go**

Migrate all 20 `NewHandler(` sites per the test-migration rule (move `.WithPermissionChecker(x)` into the constructor; bare → `, nil`).

- [ ] **Step 5: gate + commit**
```bash
go test ./services/project/ ./pkg/iamclient/ -race && go vet ./... && go build ./...
gofmt -l pkg/iamclient/dial.go services/project/handler.go cmd/project-management/main.go services/project/handler_test.go
git add pkg/iamclient/dial.go services/project/handler.go cmd/project-management/main.go services/project/handler_test.go
git commit -m "fix(project): require permission checker; correct dial.go authz log (#167)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```
`go build ./...` MUST pass — it proves `cmd/project-management` uses the new signature.

---

### Task 2: budget

**Files:** Modify `services/budget/handler.go`, `cmd/budget-planning/main.go`, `services/budget/handler_test.go`

- [ ] **Step 1: budget handler.go** — same transform as Task 1 Step 2 (NewHandler required `perm`, delete `WithPermissionChecker`, comment adapted for `cmd/budget-planning`).
- [ ] **Step 2: cmd/budget-planning/main.go** — add the `if iamPerm == nil { log.Fatalf("budget-planning: startup: %s is not set; budget-planning cannot authorize without a permission checker", iamclient.EnvAddr) }`; replace `NewHandler(svc)` + `!= nil` block with `NewHandler(svc, iamPerm)`. **Also delete the stale "RequirePermission calls … are no-ops" comment** (`main.go:74-77`).
- [ ] **Step 3: budget handler_test.go** — migrate all 21 `NewHandler(` sites.
- [ ] **Step 4: gate + commit**
```bash
go test ./services/budget/ -race && go vet ./... && go build ./...
gofmt -l services/budget/handler.go cmd/budget-planning/main.go services/budget/handler_test.go
git add services/budget/handler.go cmd/budget-planning/main.go services/budget/handler_test.go
git commit -m "fix(budget): require permission checker at startup (#167)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: expense

**Files:** Modify `services/expense/handler.go`, `cmd/expense-tracking/main.go`, `services/expense/handler_test.go`

- [ ] **Step 1: expense handler.go** — NewHandler required `perm`, delete `WithPermissionChecker`, comment for `cmd/expense-tracking`.
- [ ] **Step 2: cmd/expense-tracking/main.go** — add `if iamPerm == nil { log.Fatalf("expense-tracking: startup: %s is not set; expense-tracking cannot authorize without a permission checker", iamclient.EnvAddr) }`; replace with `NewHandler(svc, iamPerm)`, drop the `!= nil` block.
- [ ] **Step 3: expense handler_test.go** — migrate all 33 `NewHandler(` sites.
- [ ] **Step 4: gate + commit**
```bash
go test ./services/expense/ -race && go vet ./... && go build ./...
gofmt -l services/expense/handler.go cmd/expense-tracking/main.go services/expense/handler_test.go
git add services/expense/handler.go cmd/expense-tracking/main.go services/expense/handler_test.go
git commit -m "fix(expense): require permission checker at startup (#167)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 4: inventory

**Files:** Modify `services/inventory/handler.go`, `cmd/inventory-management/main.go`, `services/inventory/handler_test.go`

- [ ] **Step 1: inventory handler.go** — NewHandler required `perm`, delete `WithPermissionChecker`, comment for `cmd/inventory-management`.
- [ ] **Step 2: cmd/inventory-management/main.go** — add `if iamPerm == nil { log.Fatalf("inventory-management: startup: %s is not set; inventory-management cannot authorize without a permission checker", iamclient.EnvAddr) }`; replace with `NewHandler(svc, iamPerm)`, drop the `!= nil` block. (Note: inventory `NewService(repo)` takes only repo — leave unchanged.)
- [ ] **Step 3: inventory handler_test.go** — migrate all 14 `NewHandler(` sites.
- [ ] **Step 4: gate + commit**
```bash
go test ./services/inventory/ -race && go vet ./... && go build ./...
gofmt -l services/inventory/handler.go cmd/inventory-management/main.go services/inventory/handler_test.go
git add services/inventory/handler.go cmd/inventory-management/main.go services/inventory/handler_test.go
git commit -m "fix(inventory): require permission checker at startup (#167)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:** all 4 services get required-`perm` NewHandler + setter deletion + cmd Fatalf (Tasks 1-4); dial.go log/doc fixed (Task 1); budget stale comment removed (Task 2); every test call-site migrated per service; `_NoPermissionChecker_Denies` sentinels retained. Non-goals honored (no DialFromEnv contract change, no k8s, 5 correct services untouched). ✅

**Placeholder scan:** the transform + the ledger template + exact Fatalf strings are given verbatim. "Migrate all N NewHandler sites" is a mechanical grep-and-edit, compiler-verified (build fails if any site is left on the old arity). Not a placeholder.

**Type consistency:** `NewHandler(svc *Service, perm interceptor.PermissionChecker) *Handler` identical across all 4 services; cmd passes `iamPerm` (concrete `*iamclient.PermissionChecker`, which implements `interceptor.PermissionChecker`). `interceptor` already imported in each handler.go (used by RequirePermission). No cross-service type coupling.

**Ordering:** independent per-service tasks; Task 1 first (carries shared dial.go). Each commit builds tree-wide (`go build ./...`) because the only out-of-package caller of each `NewHandler` is its own `cmd/*`, updated in the same commit. Order among 2-4 is arbitrary; listed project→budget→expense→inventory.
