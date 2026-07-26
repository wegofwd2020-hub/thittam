# iam grpcError sentinel mapping (#163) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Map the 5 unmapped iam sentinels in `grpcError` to correct gRPC codes so client input errors stop surfacing as `codes.Internal`.

**Architecture:** One function edit (`services/iam/handler.go` `grpcError`) + two test spots. No proto/sqlc/migration.

**Tech Stack:** Go 1.25, gRPC status codes, testify.

## Global Constraints

- Mappings: `ErrCountryRequired`/`ErrUnknownCountry`/`ErrAmbiguousEmail` → `InvalidArgument`; `ErrPurgeRequestNotApproved` → `FailedPrecondition`; `ErrNotPlatformAdmin` → `PermissionDenied`. All via `errors.Is` (must match wrapped forms like `fmt.Errorf("%w: %q", ErrUnknownCountry, ...)`).
- Do NOT change the `default → Internal` behavior for genuinely-internal errors; only add the 5 named cases.
- No cross-service changes (iam only; the other-services sweep is a separate follow-up).
- Gate: `go test ./services/iam/... -race && go vet ./... && go build ./...` + `gofmt -l` touched files. (services/iam/handler.go carries pre-existing gofmt debt on `main` — add none new; diff-vs-main if flagged.)
- Security-adjacent (iam) → senior review per CLAUDE.md.
- Commit Conventional-Commits (scope `iam`), ending `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

---

## File Structure

| File | Responsibility |
|---|---|
| `services/iam/handler.go` | 5 new `grpcError` cases |
| `services/iam/handler_test.go` | table cases + fix `SetTenantAddress_MissingCountry` |

---

### Task 1: Map the 5 sentinels + tests

**Files:** Modify `services/iam/handler.go`, `services/iam/handler_test.go`

- [ ] **Step 1: Write/adjust failing tests**

In `services/iam/handler_test.go`, add 5 cases (+ a wrapped case) to the `grpcError` table test (the `cases := []struct{ err error; wantCode codes.Code }{...}` block, ~:1496):
```go
		{ErrCountryRequired, codes.InvalidArgument},
		{ErrUnknownCountry, codes.InvalidArgument},
		{ErrAmbiguousEmail, codes.InvalidArgument},
		{ErrPurgeRequestNotApproved, codes.FailedPrecondition},
		{ErrNotPlatformAdmin, codes.PermissionDenied},
		{fmt.Errorf("wrapped: %w", ErrUnknownCountry), codes.InvalidArgument},
```
(Confirm `fmt` is imported in handler_test.go; if not, add it.)
And update `TestHandler_SetTenantAddress_MissingCountry` (~:879) — assert `InvalidArgument` and drop the stale comment:
```go
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
```

- [ ] **Step 2: Run — expect FAIL** (unmapped sentinels still hit Internal): `go test ./services/iam/ -run 'TestGrpcError|TestHandler_SetTenantAddress_MissingCountry'`
(Use the actual name of the table test — grep `func Test.*[Gg]rpc` in handler_test.go; it iterates `status.Code(grpcError(tc.err))`.)

- [ ] **Step 3: Add the mappings**

In `services/iam/handler.go` `grpcError`:
- Add `errors.Is(err, ErrCountryRequired)`, `errors.Is(err, ErrUnknownCountry)`, `errors.Is(err, ErrAmbiguousEmail)` to the existing `InvalidArgument` case (currently `ErrInvalidPlan, ErrRoleNotProjectScoped, ErrHoldUntilInPast` at ~:798-801).
- Add `errors.Is(err, ErrPurgeRequestNotApproved)` to a `FailedPrecondition` case (~:803-810).
- Add `errors.Is(err, ErrNotPlatformAdmin)` to the `PermissionDenied` case (~:828-831, with `auth.ErrTenantSuspended`/etc).

- [ ] **Step 4: Run — expect PASS + gate**
```bash
go test ./services/iam/ -race && go vet ./... && go build ./...
gofmt -l services/iam/handler.go services/iam/handler_test.go
```
All green; the whole iam suite passes (no other test asserted `Internal` for these sentinels — confirm the only change is the intended ones).

- [ ] **Step 5: Commit**
```bash
git add services/iam/handler.go services/iam/handler_test.go
git commit -m "fix(iam): map country/ambiguous-email/purge/platform-admin sentinels to correct gRPC codes (#163)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:** all 5 sentinels mapped (Step 3) + table-tested incl. wrapped form + dead `ErrNotPlatformAdmin` (Step 1); `SetTenantAddress_MissingCountry` asserts `InvalidArgument` (Step 1). Non-goals honored (no cross-service, no message change, default→Internal preserved). ✅

**Placeholder scan:** full mappings + full test cases given. The "confirm the table test's actual func name / fmt import" notes are compiler-checked.

**Type consistency:** each sentinel exists in `services/iam/errors.go` (audited); `errors.Is` matches wrapped forms; codes are `codes.InvalidArgument`/`FailedPrecondition`/`PermissionDenied`.

**Single task:** one function + its tests — a reviewer gates it as a unit; no split point.
