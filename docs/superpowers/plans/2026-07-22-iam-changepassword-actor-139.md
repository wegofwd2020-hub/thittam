# ChangePassword Actor Integrity — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `iam.ChangePassword` take its subject from the caller's verified token instead of the request body, so a user can no longer change another user's password in another tenant.

**Architecture:** One guard swap in one handler. `uuid.Parse(req.GetUserId())` becomes `interceptor.ActorFromRequest(ctx, req.GetUserId())` — the guard #149 added, which returns the caller's `UserID` from the token, never the request's, and refuses a mismatch with `PermissionDenied`. The service and repository layers are untouched: `Service.ChangePassword` already takes the actor as a parameter.

**Tech Stack:** Go 1.25 (CI pins 1.25.12), gRPC, `google/uuid`, `stretchr/testify`.

**Spec:** `docs/superpowers/specs/2026-07-22-iam-changepassword-actor-design.md`
**Issue:** #139, slice A. **Branch:** `fix/iam-changepassword-actor-139`, base `1147f4c`.

## Global Constraints

- **Security change touching `iam`.** Senior review + 2 approvals. Every task ends green.
- **NO Docker. NO database.** Never run `docker compose … -v` / `down` / `up` against `infra/local/` — that compose is project-scoped, so `-v` deletes ALL local volumes (it once destroyed unrelated MinIO dev data). A disposable, uniquely-named `git worktree` under the session scratchpad is fine; remove it afterwards. Integration tests SKIP without `THITTAM_TEST_DSN`; leave it unset.
- **Whole-tree `go vet ./...` is the gate**, not a focused package build. `iam.Repository` has **three** implementers, including a hidden test double `iamRepo` in `e2e/critical_path/helpers_test.go`. This change widens no interface, so none should break — but vet the whole tree anyway.
- `errcheck` runs in CI; `golangci-lint` is **not installed locally**.
- `gh pr checks <n>` is the real gate. Local green is not CI green.
- **No migration. No new permission string. No proto field added, removed or renumbered. `git diff --stat gen/` must be empty.**
- **Do NOT touch** `services/iam/service.go`, `services/iam/repository.go`, or `services/iam/db/`. `Service.ChangePassword` already takes the actor as a `uuid.UUID`. `Repository.GetUserByID` is deliberately unscoped — it backs refresh-token validation, which has only a user id available (`services/iam/db/postgres.go:103`) — and `UpdatePasswordHash` has a second legitimate caller at `services/iam/service.go:220` (login-path rehash).
- **`gofmt -l services/iam`** flags `service.go` and `lifecycle_test.go` on a clean `main`. Pre-existing; CI's Lint does not gate on gofmt. **Do not reformat them.**
- Coverage on `services/iam` must not regress. **Baseline: 87.2%.**
- Structured logging via `slog`; never log a password, a token, or the `authorization` metadata value.

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `services/iam/handler_test.go` | **Modify.** Add `memberCtxAs`; repair 2 existing tests; add 3 new tests. | 1 |
| `services/iam/handler.go` | **Modify.** `ChangePassword` at `:216` — one guard swap. | 1 |
| `proto/thittam/iam/v1/iam.proto` | **Modify.** Deprecation comment on `ChangePasswordRequest.user_id` (`:256`). | 2 |

One task for the code because the handler change and its tests must land together to keep every commit green; a second, separate task for the proto comment because it is reviewable independently and touches no Go.

---

## Verified scaffolding facts

Established by reading the tree at `1147f4c`. Trust these; do not re-derive them.

- `interceptor.ActorFromRequest(ctx context.Context, reqActorID string) (uuid.UUID, error)` — `pkg/interceptor/actor.go`, added by #149. Returns `caller.UserID` on success; `uuid.Nil` + `Unauthenticated` when no caller or a nil subject; `uuid.Nil` + `InvalidArgument` on an unparseable request id; `uuid.Nil` + `PermissionDenied` on a mismatch. An **empty** `reqActorID` resolves to the caller.
- `services/iam/handler.go` **already imports** `"github.com/wegofwd2020/thittam/pkg/interceptor"` — no import change is needed.
- `ChangePassword` is at `services/iam/handler.go:216`.
- `Service.ChangePassword(ctx context.Context, userID uuid.UUID, oldPassword, newPassword string) error` — `services/iam/service.go:320`. Unchanged by this plan.
- `services/iam` `mockRepo` is declared in **`service_test.go:29`**, not `handler_test.go`. Relevant fn-fields: `getUserByIDFn func(ctx context.Context, userID uuid.UUID) (*auth.UserRecord, error)` (`:35`) and `updatePasswordHashFn func(ctx context.Context, userID uuid.UUID, hash string) error` (`:42`).
- `services/iam/handler_test.go:42` `memberCtx(tid uuid.UUID)` — **mints a fresh `uuid.New()` for `UserID` on every call**, so no existing test can name its own caller. Task 1 adds `memberCtxAs`.
- `newHandler()` (`handler_test.go:33`) and `newHandlerWithRepo(r *mockRepo)` (`:37`) both exist. `NewHandler` takes **one** argument in iam — unlike ledger, no signature changes here, so no construction site breaks.
- `handler_test.go` already imports `context`, `uuid`, `assert`, `require`, `iamv1`, `auth`, `interceptor`, `codes`, `status`. **No import change is needed.**
- The test hasher/verifier stub treats `"hashed:"+plain` as the hash; `TestHandler_ChangePassword_Success` relies on `PasswordHash: "hashed:old"` matching old password `"old"`.
- `grpcError` maps domain sentinels only. `ActorFromRequest` returns `*status.Status` errors — return them **directly**, never through `grpcError`. This matches how the existing code returns `TenantFromRequest`'s error.
- `proto/buf.yaml` enables only the `FILE` breaking category. A comment is not a breaking change.

## Traps

1. **Exactly two existing tests flip.** Both call `context.Background()` with **no caller at all**:
   - `TestHandler_ChangePassword_Success` (`:263`)
   - `TestHandler_ChangePassword_InvalidUserID` (`:282`)

   Both will return `Unauthenticated` after the change. Repair each by supplying the caller it always should have had. **Do not weaken either assertion. If a third test fails, STOP and report** — the spec's reading was wrong.

2. **`TestHandler_ChangePassword_InvalidUserID` is the subtle one.** `ActorFromRequest` checks the caller **before** parsing, so with no caller the test can no longer reach the parse. Once given a caller, it reaches the parse and still asserts `InvalidArgument`. The assertion is unchanged; only the context is. This is the hazard #146 shipped and #149 documented.

3. **`mockRepo`'s unset fn-fields do NOT all return benign zero values — `GetUserByID` returns a usable record** (`service_test.go:90`: `&auth.UserRecord{ID: userID, PasswordHash: "hashed"}`), which the test verifier then rejects with `auth.ErrInvalidCredentials`, which `grpcError` maps to `codes.Unauthenticated` (`handler.go:848`). So a denial test asserting only a status code can pass against the vulnerable handler by an unrelated route. **Every denial test in this task must install a `t.Fatal` fn on the first repository call it should never reach** — `getUserByIDFn`, not only `updatePasswordHashFn`.\n\n   The write-side statement still holds: `updatePasswordHashFn` unset returns `nil`. So a forgery test that asserts only `PermissionDenied` **would also pass against the vulnerable handler**, which parses the body and writes. The forgery test **must** install `updatePasswordHashFn` with a `t.Fatal` body.

4. **`Service.ChangePassword` takes the id positionally** among two strings. A wrong id threaded through still compiles. Only an assertion on what reaches `updatePasswordHashFn` catches it.

---

## Task 1: The actor guard and its tests

**Files:**
- Modify: `services/iam/handler_test.go` (add `memberCtxAs` after `memberCtx` at `:42-49`; repair `:263` and `:282`; append 3 new tests)
- Modify: `services/iam/handler.go:216-224`

**Interfaces:**
- Consumes: `interceptor.ActorFromRequest(ctx context.Context, reqActorID string) (uuid.UUID, error)` — already exists, added by #149.
- Produces: nothing new. `ChangePassword`'s signature is unchanged.

This task is atomic: the handler change makes the two existing tests fail, so handler and tests land in one commit.

- [ ] **Step 1: Add the `memberCtxAs` helper**

In `services/iam/handler_test.go`, immediately after the existing `memberCtx` function (ends at `:49`), add:

```go
// memberCtxAs returns a member caller in tenant tid with a specific user id.
// memberCtx mints a random UserID, so a test that asserts on the recorded
// subject must name its caller. Mirrors callerCtxAs in services/ledger (#149).
func memberCtxAs(tid, uid uuid.UUID) context.Context {
	return interceptor.WithCaller(context.Background(), interceptor.CallerInfo{
		UserID:   uid,
		TenantID: tid,
		Email:    "member@example.com",
		Roles:    []string{interceptor.RoleMember},
	})
}
```

- [ ] **Step 2: Write the three failing tests**

Append to `services/iam/handler_test.go`:

```go
// --- #139 slice A: ChangePassword actor integrity ---

// A caller may not change somebody else's password. mockRepo's unset fn-fields
// return benign zero values and never panic, so asserting only the status code
// would pass against the vulnerable handler too — updatePasswordHashFn must
// carry a t.Fatal body to prove the guard fires before the write.
func TestHandler_ChangePassword_ForgedSubjectDenied(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	callerID := uuid.New()
	victimID := uuid.New()

	h := NewHandler(newTestService(&mockRepo{
		getUserByIDFn: func(context.Context, uuid.UUID) (*auth.UserRecord, error) {
			t.Fatal("a forged subject must be refused before the user is read")
			return nil, nil
		},
		updatePasswordHashFn: func(context.Context, uuid.UUID, string) error {
			t.Fatal("a forged subject must never reach the password write")
			return nil
		},
	}))

	_, err := h.ChangePassword(memberCtxAs(tenantID, callerID), &iamv1.ChangePasswordRequest{
		UserId:      victimID.String(),
		OldPassword: "old",
		NewPassword: "newpass",
	})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// With user_id empty — the path a client takes once the field is deprecated —
// the password changed must be the caller's own.
func TestHandler_ChangePassword_UsesTheCallerAsSubject(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	callerID := uuid.New()
	var gotReadID, gotWriteID uuid.UUID

	h := NewHandler(newTestService(&mockRepo{
		getUserByIDFn: func(_ context.Context, id uuid.UUID) (*auth.UserRecord, error) {
			gotReadID = id
			return &auth.UserRecord{ID: id, PasswordHash: "hashed:old"}, nil
		},
		updatePasswordHashFn: func(_ context.Context, id uuid.UUID, _ string) error {
			gotWriteID = id
			return nil
		},
	}))

	_, err := h.ChangePassword(memberCtxAs(tenantID, callerID), &iamv1.ChangePasswordRequest{
		OldPassword: "old",
		NewPassword: "newpass",
	})
	require.NoError(t, err)
	assert.Equal(t, callerID, gotReadID, "the user read must be the caller")
	assert.Equal(t, callerID, gotWriteID, "the password written must be the caller's")
}

// Without the interceptor chain there is no caller, and the RPC must not proceed.
//
// getUserByIDFn carries the t.Fatal, not just the write fn. grpcError maps
// auth.ErrInvalidCredentials to codes.Unauthenticated (handler.go:848), and
// mockRepo's default GetUserByID returns PasswordHash "hashed", which the test
// verifier rejects — so the VULNERABLE handler also answers Unauthenticated here,
// by a completely different route. Asserting the code alone would be a tautology.
// The real requirement is that a tokenless call reaches no repository at all.
func TestHandler_ChangePassword_NoCallerUnauthenticated(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(&mockRepo{
		getUserByIDFn: func(context.Context, uuid.UUID) (*auth.UserRecord, error) {
			t.Fatal("a tokenless call must never reach the repository")
			return nil, nil
		},
		updatePasswordHashFn: func(context.Context, uuid.UUID, string) error {
			t.Fatal("a tokenless call must never reach the password write")
			return nil
		},
	}))

	_, err := h.ChangePassword(context.Background(), &iamv1.ChangePasswordRequest{
		UserId:      uuid.New().String(),
		OldPassword: "old",
		NewPassword: "newpass",
	})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}
```

- [ ] **Step 3: Run the new tests to verify they fail**

Run: `go test ./services/iam/ -run 'ChangePassword_ForgedSubjectDenied|ChangePassword_UsesTheCallerAsSubject|ChangePassword_NoCallerUnauthenticated' -v 2>&1 | tail -30`

Expected: all three **FAIL** against the current handler.
- `ForgedSubjectDenied` fires its `t.Fatal` — the vulnerable handler parses the victim's id and reads the user.
- `UsesTheCallerAsSubject` fails on `gotReadID`: with `UserId` empty, `uuid.Parse("")` errors and the handler returns `InvalidArgument`, so `require.NoError` fails.
- `NoCallerUnauthenticated` gets `codes.OK` (or a domain error), not `Unauthenticated`.

- [ ] **Step 4: Swap the guard**

In `services/iam/handler.go`, replace the body of `ChangePassword` at `:216`. Before:

```go
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id")
	}
```

After:

```go
	// The subject comes from the verified token, never the request body: this RPC
	// previously accepted any user_id, so a caller who knew another user's password
	// could change it in any tenant (#139 slice A). ActorFromRequest returns the
	// caller's own id, so Service.ChangePassword cannot be aimed at anyone else.
	userID, err := interceptor.ActorFromRequest(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}
```

Do **not** wrap the error in `grpcError` — `ActorFromRequest` already returns a `*status.Status` error. `handler.go` already imports `interceptor`; no import change.

- [ ] **Step 5: Repair the two existing tests**

`TestHandler_ChangePassword_Success` (`:263`) — give it a caller equal to the `UserId` it sends. Change the call from `context.Background()`:

```go
	tenantID := uuid.New()
	userID := uuid.New()
	h := NewHandler(newTestService(&mockRepo{
		getUserByIDFn: func(_ context.Context, id uuid.UUID) (*auth.UserRecord, error) {
			return &auth.UserRecord{ID: id, PasswordHash: "hashed:old"}, nil
		},
		updatePasswordHashFn: func(_ context.Context, _ uuid.UUID, _ string) error { return nil },
	}))

	resp, err := h.ChangePassword(memberCtxAs(tenantID, userID), &iamv1.ChangePasswordRequest{
		UserId:      userID.String(),
		OldPassword: "old",
		NewPassword: "newpass",
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
```

This now also exercises `ActorFromRequest`'s matching-actor path, which is why `UserId` stays populated rather than being blanked.

`TestHandler_ChangePassword_InvalidUserID` (`:282`) — give it a caller; keep the assertion:

```go
func TestHandler_ChangePassword_InvalidUserID(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ChangePassword(memberCtx(uuid.New()), &iamv1.ChangePasswordRequest{
		UserId: "bad", OldPassword: "old", NewPassword: "new",
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}
```

`ActorFromRequest` returns `InvalidArgument "invalid actor id"` — a different message from the old `"invalid user_id"`, same code. The test asserts only the code, so it stands unchanged.

**If any test other than these two fails, STOP and report it.** The count is two.

- [ ] **Step 6: Run the full package**

Run: `go test ./services/iam/ 2>&1 | tail -10`
Expected: PASS.

Run: `go test ./services/iam/ -run ChangePassword -v 2>&1 | grep -E "^(--- |ok|FAIL)"`
Expected: PASS for all five ChangePassword handler tests plus the `service_test.go` ones.

Run: `go vet ./...`
Expected: clean, exit 0.

Run: `go test -race ./services/iam/`
Expected: PASS.

Run: `gofmt -l services/iam/handler.go services/iam/handler_test.go`
Expected: **no output.** (Do not run gofmt over the whole package — `service.go` and `lifecycle_test.go` are pre-existing dirty.)

- [ ] **Step 7: Prove the tests have teeth**

No signature changed, so the parent commit compiles against the new tests. Copy this branch's test file onto `HEAD~1` and confirm the guard is what denies.

```bash
WT="${TMPDIR:-/tmp}/teeth-139a-$$"   # never inside the repo: a worktree there pollutes git status
rm -rf "$WT"; git worktree add --detach -q "$WT" HEAD~1
cp services/iam/handler_test.go "$WT/services/iam/handler_test.go"
(cd "$WT" && go test ./services/iam/ -run ChangePassword 2>&1 | tail -25)
git worktree remove "$WT" --force; git worktree prune
```

Expected against the **vulnerable** handler:
- `TestHandler_ChangePassword_ForgedSubjectDenied` **FAILS** (fires `t.Fatal` — the forged subject reached the repository).
- `TestHandler_ChangePassword_UsesTheCallerAsSubject` **FAILS** (`InvalidArgument` from `uuid.Parse("")`).
- `TestHandler_ChangePassword_NoCallerUnauthenticated` **FAILS** — it fires its `getUserByIDFn` `t.Fatal`, because the vulnerable handler parses the body and reads the user. It must NOT be allowed to pass by asserting `Unauthenticated` alone: `grpcError` maps `auth.ErrInvalidCredentials` to that same code (`handler.go:848`), so the vulnerable handler reaches it by a different route.
- `TestHandler_ChangePassword_Success` and `_InvalidUserID` **PASS** — the old handler ignores the caller the repairs added.

Paste the transcript into your report. Confirm `git worktree list` shows no leftovers. **If any check does not behave as described, stop and report — do not proceed.**

- [ ] **Step 8: Commit**

```bash
git add services/iam/handler.go services/iam/handler_test.go
git commit -m "fix(iam): ChangePassword takes its subject from the token (#139)

ChangePassword took user_id from the request body and never read the
caller from the context. Its repository lookup carries no tenant filter,
so any authenticated user could change any user's password in any tenant,
provided they knew that user's current password.

Knowing the current password is required, so this is not a takeover
primitive. It is privilege-shaped: the caller must already hold the
secret, and nothing checked that the secret was theirs. A shared
onboarding password, or a support operator who legitimately learned one,
turns that into a cross-tenant write with no record that the actor and
the subject differed.

A permission gate would not have fixed it. A caller holding user:manage
is authorized to administer users; the defect is that the handler could
not tell which user the caller was. ActorFromRequest, added by #149 for
the same defect class on posted_by, returns the caller's own id and
refuses a mismatch rather than silently substituting.

The tenant boundary closes as a side effect: the id now comes from the
token, so GetUserByID stays unscoped and stays correct because it is no
longer reachable with a caller-supplied id. Scoping it instead would
break refresh-token validation, which has only a user id available.

Both existing handler tests called context.Background() with no caller,
so both failed and both were repaired by supplying the caller they always
should have had. Neither assertion changed.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Task 2: Deprecate the request field

**Files:**
- Modify: `proto/thittam/iam/v1/iam.proto:255-259`

**Interfaces:**
- Consumes: nothing from Task 1.
- Produces: nothing.

- [ ] **Step 1: Add the deprecation comment**

In `proto/thittam/iam/v1/iam.proto`, `ChangePasswordRequest` at `:255`. Before:

```protobuf
message ChangePasswordRequest {
  string user_id = 1;
  string old_password = 2;
  string new_password = 3;
}
```

After:

```protobuf
message ChangePasswordRequest {
  // Deprecated: ignored. The subject is derived from the caller's verified token
  // (#139). Sending a value that differs from the authenticated caller is rejected
  // with PermissionDenied.
  string user_id = 1;
  string old_password = 2;
  string new_password = 3;
}
```

Do **not** use the `[deprecated = true]` field option — `grep -rn "deprecated = true" proto/` returns nothing tree-wide; the codebase convention is a `// Deprecated:` comment (#144, #149).

Do **not** remove the field. `proto/buf.yaml` enables the `FILE` breaking category and CI runs `buf breaking proto --against '.git#branch=main,subdir=proto'`.

- [ ] **Step 2: Verify no codegen drift**

Run: `git diff --stat gen/`
Expected: **empty.** A comment does not change generated code. **Do not run `buf generate`.**

Run: `buf lint proto`
Expected: clean. (If `buf` is not installed locally, say so in the report; CI runs it.)

- [ ] **Step 3: Verify the tree**

Run: `go test ./... -short 2>&1 | grep -v "^ok\|no test files"`
Expected: no output.

Run: `go vet ./...`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add proto/thittam/iam/v1/iam.proto
git commit -m "docs(proto): deprecate ChangePasswordRequest.user_id (#139)"
```

---

## Verification (whole branch, before PR)

- [ ] `go vet ./...` — clean.
- [ ] `go test ./... -short` — PASS.
- [ ] `go test -race ./services/iam/` — PASS.
- [ ] `go build ./cmd/...` — all ten entrypoints build.
- [ ] `git diff --stat gen/` — **empty**.
- [ ] `git diff --stat 1147f4c..HEAD -- migrations/` — **empty**.
- [ ] `grep -c 'uuid.Parse(req.GetUserId())' services/iam/handler.go` — must **not** match inside `ChangePassword`. Other RPCs legitimately parse a `user_id` for a target user (e.g. `AssignRole`); confirm with `grep -n` that the remaining matches are those, not this one.
- [ ] `grep -n 'ActorFromRequest' services/iam/handler.go` — exactly **1** occurrence, in `ChangePassword`.
- [ ] `gofmt -l services/iam/handler.go services/iam/handler_test.go` — no output.
- [ ] Coverage `services/iam` — **≥ 87.2%** (baseline). Record before and after.
- [ ] **`gh pr checks <n>` after opening the PR.** Local green is not CI green.

## What this does not fix

**Session revocation — filed as #154.** Changing a password revokes no sessions. `Logout` revokes a single refresh token; `RefreshToken` re-issues from the stored payload without re-reading the user record, so a stolen refresh token survives a password change for the full refresh window. Closing it needs per-user enumeration or a token-generation counter in `auth.TokenIssuer` — a contract change, not a patch.

**An administrative password reset.** If a `user:manage` holder should reset another user's password, that is a separate RPC: it must not require the old password and must be audited as an administrative act. Bundling it here would give one RPC two authorization models.

**The rest of iam.** `CreateUser`, `UpdateUser`, `ListUsers`, `GetUser`, `ListRoles`, `GetTenant`, `SetTenantAddress`, `Logout` and `DeactivateUser` are slice B of the policy table.
