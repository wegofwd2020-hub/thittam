# Required permission checker for project/budget/expense/inventory (#167) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-29
**Issue:** #167 (4 services start without a permission checker; dial.go log is false) — from #139 slice D review
**Branch:** `fix/require-perm-checker-167` off `main`
**Migration:** none · **Proto:** none · **sqlc:** none · **k8s:** none

## Goal

`project`, `budget`, `expense`, `inventory` construct their handler with an OPTIONAL
`WithPermissionChecker` setter and start regardless of whether IAM was reachable. Since #138 a
nil checker makes `interceptor.RequirePermission` fail **closed** (`codes.Internal`), so these
services don't "run without authz" — they serve `Internal` on every gated RPC, a silent
misconfiguration. Convert all 4 to the already-established ledger/reporting convention: the
checker is a **required constructor param** (omitting it is a build error) and `cmd/*` **refuses
to start** (`log.Fatalf`) when the dial yields no checker. Also correct the false `dial.go` log.

## Context (grounding facts, `main` @ 24b19c2)

- **The convention already exists in 5 services** — ledger, reporting, billing, document,
  notifications — each `NewHandler(svc *Service, perm interceptor.PermissionChecker) *Handler`
  (no setter), and each `cmd/*` does the two-step check on `iamclient.DialFromEnv`:
  ```go
  iamPerm, closeIAM, err := iamclient.DialFromEnv("<svc>")
  if err != nil { log.Fatalf("<svc>: startup: dial IAM: %v", err) }
  defer func() { _ = closeIAM() }()
  if iamPerm == nil { log.Fatalf("<svc>: startup: %s is not set; <svc> cannot authorize without a permission checker", iamclient.EnvAddr) }
  handler := <pkg>.NewHandler(svc, iamPerm)
  ```
  The `services/ledger/handler.go:39-53` NewHandler comment already names the 4 broken services and
  the intended fix.
- **`DialFromEnv` contract** (`pkg/iamclient/dial.go:19-54`) returns `(*PermissionChecker, func() error, error)`:
  unset `IAM_SERVICE_ADDR` → `(nil, noop-close, nil)`; dial failure → `(nil, noop-close, err)`;
  ok → `(checker, conn.Close, nil)`. **Behavior stays as-is** — the 4 cmd/* just add the missing
  `iamPerm == nil` Fatalf (like the other 5). No "noop checker" type exists; the "no-op" language
  in the doc comment is stale.
- **The false log** (`dial.go:32`): `"%s: %s unset — IAM permission checks DISABLED (handlers run
  without authz)"` — false since #138 (gated RPCs return `Internal`, they don't "run without authz").
  The doc comment (`dial.go:22-25`, "rollback-safe default … services keep serving without IAM")
  is likewise stale.
- **`RequirePermission` nil branch** (`pkg/interceptor/permission.go:68-73`): `checker == nil` →
  `status.Error(codes.Internal, "permission checker unavailable")`. Confirmed fail-closed.
- **The 4 to fix** — `services/{project,budget,expense,inventory}/handler.go`: identical
  `NewHandler(svc) *Handler` + `WithPermissionChecker(p) *Handler` setter; `perm` defaults nil.
  `cmd/{project-management,budget-planning,expense-tracking,inventory-management}/main.go`: each has
  `if iamPerm != nil { handler = handler.WithPermissionChecker(iamPerm) }` and starts regardless.
  `cmd/budget-planning/main.go:74-77` also carries a stale "RequirePermission calls … are no-ops"
  comment.
- **Test construction sites** (all in-package `_test.go`, NO hidden e2e/integration doubles —
  grep confirmed only cmd/* qualify `<pkg>.NewHandler`): project 20, budget 21, expense 33,
  inventory 14. Each service already defines `allowAllPerm{}`/`denyPerm{}` fakes. The
  `_NoPermissionChecker_Denies` sentinel tests (one per service) pass `nil` and assert the gated
  RPC returns `Internal` — they keep working with `NewHandler(…, nil)`.
- **`IAM_SERVICE_ADDR` is in the shared ConfigMap** (`infra/k8s/config/configmap.yaml:85`, used by
  all via `envFrom`) — refusing to start breaks no deploy; the other 5 services already `Fatalf` on
  this in prod. No k8s change, no init-container ordering needed.

## Design

For **each** of the 4 services (mechanical, mirror ledger):

### `services/<svc>/handler.go`
- Change `func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }` →
  `func NewHandler(svc *Service, perm interceptor.PermissionChecker) *Handler { return &Handler{svc: svc, perm: perm} }`.
- **Delete** the `WithPermissionChecker` method. Update the `perm` field comment to match ledger's
  ("required; a nil here is a test/bug and RequirePermission fails closed").

### `cmd/<svc>/main.go`
- After the existing `DialFromEnv` + `if err != nil { Fatalf }` + `defer closeIAM()`, add:
  `if iamPerm == nil { log.Fatalf("<svc>: startup: %s is not set; <svc> cannot authorize without a permission checker", iamclient.EnvAddr) }`.
- Replace `handler := <pkg>.NewHandler(svc)` + `if iamPerm != nil { handler = handler.WithPermissionChecker(iamPerm) }`
  with `handler := <pkg>.NewHandler(svc, iamPerm)`.
- (`budget` only) delete the stale "no-ops" comment.

### `services/<svc>/handler_test.go`
- Every `NewHandler(NewService(&mockRepo{…})).WithPermissionChecker(x)` → `NewHandler(NewService(&mockRepo{…}), x)`.
- Every bare `NewHandler(NewService(&mockRepo{…}))` (no chain) → `NewHandler(NewService(&mockRepo{…}), nil)`
  (keeps the `_NoPermissionChecker_Denies` sentinel semantics).

### `pkg/iamclient/dial.go` (shared, once)
- Fix the log line (`:32`) to the truth, e.g.:
  `log.Printf("%s: %s unset — no IAM permission checker; gated RPCs will fail closed with Internal (#138). Services requiring authz should refuse to start.", serviceName, EnvAddr)`.
- Correct the stale doc comment (`:22-25`) — remove "services keep serving without IAM"; state that
  the caller decides (the 6+ gated services `Fatalf`).

## Testing

- Each service's existing handler tests compile + pass under the new signature (the transform is
  purely moving the checker into the constructor). The `_NoPermissionChecker_Denies` test per service
  still asserts `codes.Internal` for a `nil`-checker handler — keep it, it now documents the required-
  param's nil case explicitly.
- No new tests needed for the mechanical conversion; the compile-time requirement (build error if a
  caller omits `perm`) is the new guarantee, verified by `go build ./...`.
- Gates per task: `go test ./services/<svc>/... -race`; `go vet ./...`; `go build ./...` (catches the
  `cmd/<svc>` site); `gofmt -l` touched files. No proto/sqlc/migration → no codegen gates.

## Non-goals

- No change to `DialFromEnv`'s return contract (still `nil` for unset addr — callers Fatalf).
- No k8s / deploy-manifest / init-container change (addr already in the shared ConfigMap).
- Not touching the 5 already-correct services.
- No change to the permission logic, gates, or which RPCs are gated.

## Review weight

Security-hardening (startup authz guarantee) across 4 services → senior per CLAUDE.md. Mechanical +
low-risk (mirrors a proven convention); whole-branch review on the most capable model to confirm all
4 converted consistently + the log/comment corrections.
