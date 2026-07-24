# Retire Impersonation (#139 §5, decision D8) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-24
**Issue:** #139 §5 (impersonation does not impersonate); executes ruling **D8**; **closes #156**
**Branch:** `chore/retire-impersonation-139-5` off `main` (`4fb58f7`)
**Migration:** none (the table is deliberately retained — see §5)

## Goal

Remove the impersonation feature. #139 §5 offers two choices — implement act-as in the token, or remove the feature — and ruling D8 chose removal. This executes it: both RPCs return `codes.Unimplemented`, every implementation behind them is deleted, and the second (unwired) implementation in `pkg/platform` goes with it.

## Context — what is actually there

#139 §5 states the problem exactly:

> `StartImpersonation` writes an `impersonation_session` row and an audit entry. It mints **no token**, sets no `act`/`impersonator` claim, and `CallerInfo` has no impersonation field. Subsequent requests carry the platform admin's own identity. So the audit log records a session the request path knows nothing about.

Verified against the tree, and it is worse than one broken feature — **there are three disconnected pieces**:

| piece | state |
|---|---|
| `services/iam` Start/EndImpersonation | **Wired**: gRPC → service → Postgres → a 1-minute expiry ticker in `cmd/iam/main.go`. Gated `RequireRole(platform_admin)`. Writes to `platform_impersonation_log` + an audit entry. Mints no token. |
| `pkg/platform` `Service.Impersonate` / `EndImpersonation` / `RevokeOn*` / `IsActionBlocked` | **Entirely unwired** — zero non-test importers of `pkg/platform` in the module. Carries a stale comment claiming "tenant JWT issuance happens at the handler layer", for a handler that was never built. |
| `web` `ImpersonationDialog` + the tenants page | **Mock only** — hardcoded arrays, `console.log` instead of an API call. |

The two backend implementations disagree on the session cap (`pkg/platform` 30 min vs `services/iam` 4 h), and the UI quotes **30 minutes** — so the only user-facing text describes the implementation that was never wired. That divergence is the clearest evidence these evolved independently and were never reconciled.

**Nothing breaks by removing it.** Confirmed absent: any `act`/`impersonator`/actor field in `pkg/auth.Claims`, `jwtClaims`, or `interceptor.CallerInfo`; any `google.api.http` annotation on either RPC (not gateway/Kong exposed); any reference in `infra/` or `.github/`; any in-tree caller besides tests; any e2e scenario (only interface stubs for compilation).

### Why removal rather than act-as

Implementing act-as properly means a new token claim, a `CallerInfo` field, propagation through every service, enforcement of the blocked-action list at the interceptor layer, and session revocation on password/MFA change — a substantial feature. Nothing consumes impersonation today (no wired UI, no external exposure, no caller), so that work would be building a privileged capability on speculation. D8's reasoning stands.

### What this closes

**#156** — `StartImpersonation` takes the acting admin's identity from the request body (`req.GetPlatformUserId()` → `AuditEntry.ActorID`) and self-reports `ip_address`, while `EndImpersonation` correctly uses `caller.UserID`. It is platform-admin-to-platform-admin attribution forgery: admin A can open a session recorded as admin B. A handler returning `Unimplemented` forges nothing, so #156 closes with this change rather than needing its own fix.

## Design

### 1. The two RPCs → `Unimplemented`, never deleted

`services/iam/handler.go` — replace both handler bodies with `codes.Unimplemented` and a `// Deprecated:` doc comment; add matching `// Deprecated:` comments above both `rpc` lines in `proto/thittam/iam/v1/iam.proto`.

**The RPCs are not deleted.** `proto/buf.yaml` enables the `FILE` breaking category and CI runs `buf breaking --against main`, so removing an RPC fails CI. This is the same retirement shape used for `ValidateToken` in #139 slice I. Proto edits are comment-only → no `buf generate` (it cannot run locally without BSR; `gen/` comment drift is pre-existing and accepted).

### 2. `services/iam` — delete everything behind the handlers

Once the handlers return `Unimplemented`, all of this is dead and goes:

- `Service.StartImpersonation` / `Service.EndImpersonation`, `StartImpersonationParams`, `maxImpersonationDuration`, and the two `AuditEntry` constructions.
- `Repository` interface entries and their `Postgres` implementations: `StartImpersonation`, `EndImpersonationSession`, `ExpireImpersonationSessions`.
- `impersonationSessionToProto`, and any `ErrImpersonation*` sentinel that loses its last reference (check `grpcError`'s mapping before deleting).
- Doubles: `mockRepo` (`services/iam/service_test.go`) and the `iamRepo` stubs in `e2e/critical_path/helpers_test.go`.

The `Repository` interface shrinks, so **every implementer must drop the methods in the same commit** — whole-tree `go vet ./...` is the gate that catches the hidden e2e double.

### 3. `cmd/iam` — remove the expiry ticker

`cmd/iam/main.go` runs a 1-minute `time.NewTicker` goroutine calling `repo.ExpireImpersonationSessions`. With no way to create a session, it sweeps nothing. Remove the goroutine and its ticker.

### 4. `pkg/platform` — remove its impersonation surface

Delete `Impersonate`, `EndImpersonation`, `RevokeOnPasswordChange`, `RevokeOnDeactivation`, `RevokeOnMFAChange`, `RevokeAllForUser`, `IsActionBlocked`, `blockedDuringImpersonation`, `MaxImpersonationDuration`, the `ImpersonationStore` interface, impersonation-only types, and the ~15 tests covering them (`service_test.go`).

**Keep** the package's non-impersonation parts: `CheckAccess`, `SeedPlatformOwner`, and their tests.

`blockedDuringImpersonation` is the sharpest item here. It lists `ChangePassword`, `AssignRole`, `RemovePaymentMethod` and nine more as "blocked during impersonation", and `IsActionBlocked` has **no production caller** — it reads like an enforced control and enforces nothing. That is the guard-that-returns-a-verdict-and-gets-skipped shape this repo has been bitten by before ([[feedback-enforce-guards-by-type]]).

### 5. The table is retained, deliberately

`platform_impersonation_log` (created in `migrations/iam/004`, `ended_at` added by `010`) **stays**. It holds historical records of sessions that really were opened; dropping it needs a migration and destroys audit history. It simply stops receiving rows. No migration is written in this slice.

Audit entries already in `audit_log` likewise stay.

### 6. Web UI

Delete `web/src/components/platform/impersonation-dialog.tsx` and remove its call site in `web/src/app/(platform)/tenants/page.tsx` — the `Impersonate` control, the `handleImpersonateConfirm` handler, and the dialog's state. It is a `console.log` stub over hardcoded data, so nothing regresses; leaving a visible "Impersonate User" button for a retired backend would be a standing false promise.

## Testing

- **iam handler tests** — the 7 existing impersonation tests are replaced by two asserting `codes.Unimplemented` for `StartImpersonation` and `EndImpersonation`. Note the existing tests only assert role-gating and error-code mapping; none asserts audit-entry provenance, so nothing of value is lost.
- **iam service tests** — the 7 tests for the deleted service methods are removed with them.
- **`pkg/platform`** — impersonation tests deleted; the remaining `CheckAccess` / `SeedPlatformOwner` tests must still pass.
- **Whole-tree `go vet ./...`** — the completion gate for the shrunken `Repository` interface.
- **`buf lint proto`** must pass and `buf breaking` (CI) must stay green — the RPC declarations remain.
- **Web** — whatever `web/`'s build/lint runs in CI must still pass after the component is removed (the plan grounds the exact command).
- Coverage floor: iam ≥ 85%. Deleting tested-but-dead code alongside its tests should not move it materially; confirm rather than assume.

## Non-goals

- **No act-as implementation.** Explicitly rejected by D8; revisit only with a real consumer.
- **No migration**, no table drop, no deletion of historical audit rows.
- **No change to `pkg/platform`'s non-impersonation surface** (`CheckAccess`, `SeedPlatformOwner`).
- No proto message deletion — `StartImpersonationRequest` / `ImpersonationSession` etc. stay declared alongside their RPCs (same buf constraint).
- The policy table's `StartImpersonation` row (`docs/superpowers/specs/2026-07-22-authz-policy-table-139.md`) currently reads ✅ "gate is right; the feature is broken — see D8"; updating it to record the retirement is a one-line docs edit included here so the table does not outlive the decision.

## Review weight

Touches `iam` → senior engineer + 2 approvals per CLAUDE.md. The diff is mostly deletion, but it removes a privileged capability, so the whole-branch review runs on the most capable model.
