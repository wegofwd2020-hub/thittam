# Retire Impersonation (#139 §5, D8) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Retire the impersonation feature — both iam RPCs return `codes.Unimplemented`, everything behind them is deleted, `pkg/platform`'s separate unwired implementation goes with it, and the dead web UI is removed. Executes ruling D8; closes #156.

**Architecture:** Three independent tasks. Task 1 is the wired iam backend (handlers + everything behind them + the `cmd/iam` ticker, which must land in the same commit or the build breaks). Task 2 is `pkg/platform`'s separate, entirely unwired implementation. Task 3 is the web UI plus a docs correction. No migration; the table and existing audit rows are retained.

**Tech Stack:** Go 1.25, gRPC, buf, sqlc, Next.js/TypeScript (`web/`).

## Global Constraints

- **NEVER delete an RPC.** `proto/buf.yaml` enables the `FILE` breaking category and CI runs `buf breaking --against main`; removing an RPC fails CI. Retirement = handler returns `codes.Unimplemented` + `// Deprecated:` comments. The `rpc` lines at `proto/thittam/iam/v1/iam.proto:92-93` and the four impersonation messages (`:449-474`) all STAY declared.
- **Proto edits are comment-only → do NOT run `buf generate`.** It cannot run locally (`google/api/annotations.proto` unresolvable without BSR) and `gen/` comment drift is pre-existing.
- **NEVER hand-edit sqlc-generated files.** CI now has a **Codegen Freshness (sqlc)** job (added by #160) that regenerates and fails on any diff. `services/iam` is still in `sqlc.yaml` and `platform_impersonation_log` still exists, so `services/iam/db/models.go`'s `PlatformImpersonationLog` (line 26) **stays generated — leave it alone**. If any impersonation SQL lives in `services/iam/db/queries.sql`, delete it *there* and run `sqlc generate`; then verify `sqlc generate && git add -A && git diff --cached --exit-code` is clean before committing.
- **The `cmd/iam` ticker must be deleted in the same commit as the `Repository` method** it calls (`ExpireImpersonationSessions`), or `go build ./...` fails.
- **The table is retained.** No migration. `platform_impersonation_log` (`migrations/iam/004`, `ended_at` from `010`) and all existing `audit_log` rows stay — they record sessions that really happened. The table simply stops receiving rows.
- **`pkg/platform`'s `CheckAccess` and `SeedPlatformOwner` must keep working** — they are not impersonation. `CheckAccess` (`service.go:245-265`) depends only on `PlatformUser`/`Role`/`RoleOwner|Admin|Support` + `ErrNotPlatformUser`/`ErrInsufficientRole`. `SeedPlatformOwner` (`:267-281`) touches only `s.users` and `s.logger`.
- **Out of scope, do not touch:** `pkg/audit/types.go`'s `ResourceImpersonationSession` (a generic exported enum value), `pkg/registration/db/models.go`'s generated duplicate, and `pkg/platform`'s `tenants`/`verticals` fields (non-impersonation, and already called by nothing).
- **Completion gate:** whole-tree `go vet ./...` — it is what catches the hidden `e2e/critical_path` double when the `Repository` interface shrinks.
- Touches `iam` → senior engineer + 2 approvals. Coverage floor iam ≥ 85%.

---

## Task 1: iam backend — RPCs to Unimplemented, delete everything behind them

**Files:**
- Modify: `proto/thittam/iam/v1/iam.proto:91-93` (comments only)
- Modify: `services/iam/handler.go` — `StartImpersonation` (:641-675), `EndImpersonation` (:677-690), delete `impersonationSessionToProto` (:802-817), edit `grpcError` (:838, :840-842)
- Modify: `services/iam/service.go` — delete `:803-807` (header + `maxImpersonationDuration`), `:813-842`, `:844-866`
- Modify: `services/iam/models.go` — delete `ImpersonationSession` (:93-106), `StartImpersonationParams` (:108-118)
- Modify: `services/iam/repository.go` — delete the three entries + comments (:124-134)
- Modify: `services/iam/db/postgres.go` — delete `:816-907` (section header + three impls)
- Modify: `services/iam/errors.go` — delete `ErrImpersonationNotFound` (:16), `ErrImpersonationAlreadyEnded` (:17)
- Modify: `cmd/iam/main.go` — delete the ticker goroutine (:184-201)
- Modify: `services/iam/service_test.go` — `mockRepo` fn fields (:72-74), methods (:312-335); delete 7 tests (:2027-2141 incl. section comment + `fixedSessionID`)
- Modify: `services/iam/handler_test.go` — replace 7 tests (:1448-1528)
- Modify: `e2e/critical_path/helpers_test.go` — delete `iamRepo` stubs (:321-331)

**Interfaces:**
- Produces: `iam.Repository` **loses** `StartImpersonation`, `EndImpersonationSession`, `ExpireImpersonationSessions`. Every implementer must drop them in this commit: `*Postgres`, `mockRepo`, `iamRepo`. (Verified complete — no other implementer exists tree-wide.)

- [ ] **Step 1: Write the failing tests**

`services/iam/handler_test.go` — delete the 7 tests at `:1448-1528` (including the `// --- StartImpersonation ---` and `// --- EndImpersonation ---` section comments) and replace with:

```go
// --- Impersonation (retired, #139 §5 / D8) ---

func TestHandler_StartImpersonation_Retired(t *testing.T) {
	t.Parallel()
	_, err := newHandler().StartImpersonation(platformAdminCtx(), &iamv1.StartImpersonationRequest{
		PlatformUserId: uuid.New().String(),
		TenantId:       uuid.New().String(),
		Reason:         "support",
	})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}

func TestHandler_EndImpersonation_Retired(t *testing.T) {
	t.Parallel()
	_, err := newHandler().EndImpersonation(platformAdminCtx(), &iamv1.EndImpersonationRequest{
		SessionId: uuid.New().String(),
	})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}
```

Note: these use `platformAdminCtx()` (a legitimate caller) deliberately — the point is that even a properly authorised platform admin gets `Unimplemented`, proving the feature is gone rather than merely gated.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./services/iam/ -run 'Impersonation_Retired' -v`
Expected: FAIL — the current handlers do real work and return a session / empty response, not `Unimplemented`.

- [ ] **Step 3: Gut both handlers**

`services/iam/handler.go` — replace the bodies of `StartImpersonation` (:641-675) and `EndImpersonation` (:677-690) entirely:

```go
// StartImpersonation is retired.
//
// Deprecated: the feature never impersonated anything — it minted no token and
// set no act/impersonator claim, so subsequent requests carried the platform
// admin's own identity while the audit log recorded a session the request path
// knew nothing about (#139 §5, decision D8). It also took its actor from the
// request body rather than the verified caller (#156). Left declared because
// proto/buf.yaml uses the FILE breaking category — removing an RPC fails CI.
func (h *Handler) StartImpersonation(context.Context, *iamv1.StartImpersonationRequest) (*iamv1.ImpersonationSession, error) {
	return nil, status.Error(codes.Unimplemented, "impersonation is retired (#139)")
}

// EndImpersonation is retired. See StartImpersonation.
//
// Deprecated: retired with the rest of the impersonation feature (#139 §5, D8).
func (h *Handler) EndImpersonation(context.Context, *iamv1.EndImpersonationRequest) (*iamv1.EndImpersonationResponse, error) {
	return nil, status.Error(codes.Unimplemented, "impersonation is retired (#139)")
}
```

- [ ] **Step 4: Delete the service, repository, and Postgres layers**

- `services/iam/service.go` — delete `:803-807` (the `// --- Impersonation ---` header and `maxImpersonationDuration`), `:813-842` (`Service.StartImpersonation`), `:844-866` (`Service.EndImpersonation`).
- `services/iam/repository.go` — delete `:124-134` (the `// Impersonation lifecycle` comment block and all three method declarations).
- `services/iam/db/postgres.go` — delete `:816-907` (the `// --- iam.Repository: Impersonation lifecycle ---` header plus `StartImpersonation`, `EndImpersonationSession`, `ExpireImpersonationSessions`).
- `services/iam/models.go` — delete `ImpersonationSession` (:93-106) and `StartImpersonationParams` (:108-118).
- `services/iam/handler.go` — delete `impersonationSessionToProto` (:802-817); its only caller was the old `StartImpersonation` body.

**Check before deleting SQL:** if any of the three Postgres methods used a sqlc query rather than raw SQL, remove that query from `services/iam/db/queries.sql` and run `sqlc generate` — do NOT hand-edit `queries.sql.go` or `models.go`. `PlatformImpersonationLog` in `db/models.go:26` stays regardless (the table still exists).

- [ ] **Step 5: Remove the sentinels and their grpcError mapping**

`services/iam/errors.go` — delete `ErrImpersonationNotFound` (:16) and `ErrImpersonationAlreadyEnded` (:17).

`services/iam/handler.go` `grpcError` — two different edits, do not conflate:
- `:838` — `ErrImpersonationNotFound` shares a multi-sentinel `NotFound` case with `ErrUserNotFound`, `ErrTenantNotFound`, `ErrRoleNotFound`, `ErrInvitationNotFound`. Remove **only** the `errors.Is(err, ErrImpersonationNotFound),` line; the case and its four other sentinels stay.
- `:840-842` — `ErrImpersonationAlreadyEnded` has its own dedicated `FailedPrecondition` case. Delete the whole case.

- [ ] **Step 6: Delete the cmd/iam expiry ticker**

`cmd/iam/main.go` — delete `:184-201` in full: the `// --- Background: impersonation session expiry ticker ---` comment and the entire `go func() { ... }()` block. It calls `repo.ExpireImpersonationSessions`, so it **must** go in this commit or the build breaks. The block sits between `handler := iam.NewHandler(svc)` (:182) and the NATS billing-consumer section (:203); deleting it leaves those cleanly adjacent.

- [ ] **Step 7: Update the two doubles**

`services/iam/service_test.go` `mockRepo` — delete the fn fields at `:72-74` (`startImpersonationFn`, `endImpersonationSessionFn`, `expireImpersonationSessionsFn`) and the three methods at `:312-335`.

`e2e/critical_path/helpers_test.go` `iamRepo` — delete the three stubs at `:321-331`.

- [ ] **Step 8: Delete the 7 service tests**

`services/iam/service_test.go` — delete `:2027-2141`: the section comment (:2027), the `fixedSessionID` var (:2029, impersonation-only), and all seven tests (`TestStartImpersonation_Success`, `_DurationCappedAt4Hours`, `_ZeroDuration_UsesMax`, `_RepoError`, `TestEndImpersonation_Success`, `_NotFound`, `_AlreadyEnded`).

- [ ] **Step 9: Add the proto deprecation comments**

`proto/thittam/iam/v1/iam.proto` — above the two `rpc` lines (:92-93), replacing/extending the `// --- Impersonation (platform_admin only) ---` comment at :91:

```proto
  // --- Impersonation (RETIRED, #139 §5) ---
  // Deprecated: both RPCs return Unimplemented. The feature minted no token and
  // set no act/impersonator claim, so it never actually impersonated anyone.
  // Not deleted: buf's FILE breaking category makes RPC removal a breaking change.
  rpc StartImpersonation(StartImpersonationRequest) returns (ImpersonationSession);
  rpc EndImpersonation(EndImpersonationRequest) returns (EndImpersonationResponse);
```

Comment-only — do not run `buf generate`. The four impersonation messages (`:449-474`) stay declared and untouched.

- [ ] **Step 10: Run the full gate**

Run:
```bash
go build ./... && go vet ./... && \
go test ./services/iam/... ./e2e/... && \
sqlc generate && git add -A && git diff --cached --exit-code && \
buf lint proto
```
Expected: all clean. `go vet ./...` is what proves every `Repository` implementer dropped the three methods. The `sqlc` line is the #160 freshness gate — it must report no drift.

- [ ] **Step 11: Commit**

```bash
git add proto services/iam cmd/iam e2e/critical_path/helpers_test.go
git commit -m "feat(iam)!: retire impersonation — both RPCs return Unimplemented (#139 §5)

The feature minted no token and set no act/impersonator claim, so subsequent
requests carried the platform admin's own identity while the audit log recorded
a session the request path knew nothing about. It also took its actor from the
request body rather than the verified caller, making the audit entry forgeable
by any platform admin (#156).

Executes decision D8: strip the implementation, return Unimplemented, deprecate
by comment. The RPC declarations and their messages are retained because
proto/buf.yaml uses the FILE breaking category.

platform_impersonation_log and existing audit rows are deliberately kept — they
record sessions that really happened. The table simply stops receiving rows.

Closes #156."
```

---

## Task 2: pkg/platform — delete its separate, unwired impersonation surface

`pkg/platform` has **zero non-test importers anywhere in the module** (verified: `grep -rn '"github.com/wegofwd2020/thittam/pkg/platform"' --include=*.go .` → no matches). This is a second, complete implementation of the same feature that was never wired to anything, and it disagrees with the real one on the session cap (30 min vs 4 h).

**Files:**
- Modify: `pkg/platform/service.go`, `types.go`, `ports.go`, `errors.go`, `service_test.go`

**Interfaces:**
- Produces: `NewService` **loses** its `impersonations ImpersonationStore` parameter. Its only caller is `newTestService` in `service_test.go:182-194`, which must be updated in this commit.

- [ ] **Step 1: Delete the impersonation methods**

`pkg/platform/service.go` — delete:
- `WithNotifier` (:55-60)
- `Impersonate` (:62-123)
- `EndImpersonation` (:125-130)
- `revokeSession` (:132-164, private, impersonation-only)
- `RevokeOnPasswordChange` (:166-171), `RevokeOnDeactivation` (:173-177), `RevokeOnMFAChange` (:179-183)
- `revokeAllForUser` (:185-235, private, used only by the three above)
- `IsActionBlocked` (:237-242)

**KEEP** `CheckAccess` (:245-265) and `SeedPlatformOwner` (:267-281).

- [ ] **Step 2: Shrink the Service struct and NewService**

`pkg/platform/service.go:20-46` — drop the `impersonations` and `notifier` fields and the `impersonations` parameter:

```go
// Service implements platform administration operations.
type Service struct {
	users    UserStore
	tenants  TenantManager
	verticals VerticalManager
	logger   Logger
	auditLog AuditSink
}

// NewService creates a platform administration service.
func NewService(
	users UserStore,
	tenants TenantManager,
	verticals VerticalManager,
	logger Logger,
) *Service {
	return &Service{
		users:     users,
		tenants:   tenants,
		verticals: verticals,
		logger:    logger,
	}
}
```

(`tenants`/`verticals` are non-impersonation and stay, even though no method currently calls them — that is a separate pre-existing gap, out of scope.)

- [ ] **Step 3: Delete impersonation types, ports, and errors**

`pkg/platform/types.go` — delete `ImpersonationRequest` (:33-40), `MaxImpersonationDuration` (:42-44), `RevocationReason` + its consts (:46-60), `ImpersonationSession` (:62-68), `ActiveImpersonationSession` (:70-80), `blockedDuringImpersonation` (:82-101).

`pkg/platform/ports.go` — delete `ImpersonationStore` (:47-60), `ImpersonationNotifier` (:77-84), `noopNotifier` (:86-91).

`pkg/platform/errors.go` — delete `ErrImpersonationDenied` (:16), `ErrReasonRequired` (:19), `ErrSessionNotFound` (:25), `ErrSessionExpired` (:28), `ErrActionBlockedDuringImpersonation` (:32). **KEEP** `ErrNotPlatformUser` (:7) and `ErrInsufficientRole` (:10) — `CheckAccess` needs them.

- [ ] **Step 4: Delete the impersonation tests and their scaffolding**

`pkg/platform/service_test.go` — delete these 18 test functions: `TestImpersonate_HappyPath` (:214-228), `_ReasonRequired` (:230-242), `_SupportRoleDenied` (:244-264), `_MFARequired` (:266-286), `_UserNotFound` (:288-305), `_SessionDuration` (:351-369), `TestEndImpersonation_HappyPath` (:373-394), `_SessionNotFound` (:396-403), `TestRevokeOnPasswordChange_RevokesActiveSessions` (:407-431), `TestRevokeOnDeactivation_...` (:433-446), `TestRevokeOnMFAChange_...` (:448-461), `TestRevokeOnPasswordChange_NoActiveSessions` (:463-469), `TestImpersonate_EmitsAuditEvent` (:473-488), `TestEndImpersonation_EmitsAuditEvent` (:490-508), `TestWithNotifier_IsCalledOnPasswordChangeRevocation` (:545-573), `TestRevokeAllForUser_GetSessionsError_ReturnsError` (:577-585), `TestRevokeAllForUser_PartialRevokeFailure_ContinuesBestEffort` (:587-622), `TestIsActionBlocked` (:662-695).

Also delete their impersonation-only scaffolding: `mockImpersonationStore` + `newMockImpersonationStore` (:38-98), `captureNotifier` (:514-543), `mockAuditSink`/`auditEvent` (:122-161), `newTestServiceWithStore` (:198-210), `testImpersonateID` (:168), `mockTenantManager` (:100-107), `mockVerticalManager` (:109-114) — **but only if the KEEP tests do not use them**; verify each with a grep before removing (`mockTenantManager`/`mockVerticalManager` may be needed by `newTestService`'s `NewService(...)` call).

**KEEP** these 4 tests: `TestCheckAccess` (:307-338), `TestSeedPlatformOwner` (:340-347), `TestCheckAccess_UnknownUserRole_ReturnsNotPlatformUser` (:626-631), `TestSeedPlatformOwner_StoreError_ReturnsWrappedError` (:635-653) — plus their scaffolding: `mockUserStore` (:18-36), `mockLogger` (:116-120), `testPlatformAdmin()` (:171-180), `testPlatformUserID`/`testTenantID` (:166-167), `failingCreateUserStore` (:656-660), and `newTestService` (:182-194) **updated** for the new 4-arg `NewService`.

- [ ] **Step 5: Run the gate**

Run: `go build ./... && go vet ./... && go test ./pkg/platform/... -v 2>&1 | tail -20`
Expected: clean; the 4 kept tests pass. If `go vet` reports an unused import in any edited file, remove it.

- [ ] **Step 6: Commit**

```bash
git add pkg/platform
git commit -m "chore(platform): delete the unwired impersonation implementation (#139 §5)

pkg/platform held a second, complete impersonation implementation with zero
non-test importers anywhere in the module — it disagreed with the wired one on
the session cap (30m vs 4h) and carried a comment claiming tenant JWT issuance
happened at a handler layer that was never built.

Its blockedDuringImpersonation map listed ChangePassword, AssignRole and ten
other actions as blocked, while IsActionBlocked had no production caller: a
control that read as enforced and enforced nothing.

CheckAccess and SeedPlatformOwner are unaffected."
```

---

## Task 3: web UI + policy-table correction

**Files:**
- Delete: `web/src/components/platform/impersonation-dialog.tsx`
- Modify: `web/src/app/(platform)/tenants/page.tsx`
- Modify: `docs/superpowers/specs/2026-07-22-authz-policy-table-139.md`

Confirmed single-consumer: `grep -rln "impersonation-dialog\|ImpersonationDialog" web` → exactly the component and the tenants page.

- [ ] **Step 1: Delete the component**

```bash
git rm web/src/components/platform/impersonation-dialog.tsx
```

- [ ] **Step 2: Remove every reference from the tenants page**

`web/src/app/(platform)/tenants/page.tsx` — remove, in this order (work bottom-up so line numbers stay valid):
- `:329-341` — the `{/* Impersonation dialog */}` comment and the `<ImpersonationDialog ... />` render.
- `:294-302` — the `{/* Impersonate */}` comment and the Impersonate button.
- `:143` — reword the header copy that reads "View, manage, and impersonate tenant accounts across the platform." to drop "and impersonate" (prose, but it advertises the feature).
- `:124-133` — the `handleImpersonateConfirm` function.
- `:109` — the `impersonateTenant` / `setImpersonateTenant` state.
- `:94-100` — the `mockUsers` array and its `// Mock users for impersonation dialog` comment (used only by the dialog).
- `:7` — the `ImpersonationDialog` import.
- `:4` — drop `UserCog` from the combined lucide import; it is used **only** by the deleted button at :301. Leave the other icons (`Search`, `ArrowUpCircle`, `Pause`, `Play`) alone.

- [ ] **Step 3: Verify the web build**

Run from `web/`: `npm run lint && npm run build`
Expected: both pass. **Note:** `.github/workflows/ci.yml` does not touch `web/` at all, and `ui-e2e.yml` runs only the unauthenticated Playwright smoke project — so **CI will not catch a broken build here.** Running these two commands locally is the only gate; do not skip them.

- [ ] **Step 4: Correct the policy table**

`docs/superpowers/specs/2026-07-22-authz-policy-table-139.md` — the `StartImpersonation` row currently reads ✅ "gate is right; the feature is broken — see D8". Update it (and the `EndImpersonation` row) to record that D8 executed and both RPCs now return `Unimplemented`, so the table does not outlive the decision.

- [ ] **Step 5: Commit**

```bash
git add web docs/superpowers/specs/2026-07-22-authz-policy-table-139.md
git commit -m "chore(web): remove the dead impersonation dialog (#139 §5)

The dialog was never wired to a backend — hardcoded mock users and a console.log
in place of an API call — and quoted a 30-minute session cap that came from the
unwired pkg/platform implementation, not the 4-hour one that actually ran.
With impersonation retired it would advertise a feature returning Unimplemented.

Also records D8's execution in the #139 policy table."
```

---

## Self-Review

- **Spec coverage:** RPCs → Unimplemented + deprecated, never deleted (T1 S3/S9) ✓; service/repo/postgres/models/sentinels/converter deleted (T1 S4/S5) ✓; `cmd/iam` ticker (T1 S6) ✓; doubles (T1 S7) ✓; tests replaced/removed (T1 S1/S8) ✓; `pkg/platform` impersonation surface incl. the block-list map, keeping `CheckAccess`/`SeedPlatformOwner` (T2) ✓; web UI (T3 S1-S3) ✓; table + audit rows retained (Global Constraints) ✓; policy-table correction (T3 S4) ✓; closes #156 (T1 commit message) ✓.
- **Placeholder scan:** every deletion names an exact file and line range from the grounding scan. The two judgment points are called out explicitly rather than left vague: which `pkg/platform` test scaffolding the KEEP tests still need (T2 S4, "verify each with a grep"), and whether any impersonation SQL lives in `queries.sql` (T1 S4).
- **Type consistency:** `iam.Repository` loses exactly three methods, and all three implementers (`*Postgres`, `mockRepo`, `iamRepo`) are updated in Task 1's single commit. `platform.NewService` drops one parameter, and its only caller (`newTestService`) is updated in Task 2's.
- **Ordering:** the `cmd/iam` ticker is in Task 1, not Task 3, because it calls a method Task 1 deletes — splitting them would leave a non-building commit.
- **New constraint honoured:** #160's Codegen Freshness gate means generated files are never hand-edited; Task 1's gate runs `sqlc generate` and asserts no drift.
