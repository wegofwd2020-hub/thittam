# ChangePassword takes its subject from the token — design

**Issue:** #139, slice A. **Branch:** `fix/iam-changepassword-actor-139`, base `1147f4c`.
**Follows:** #138 (authentication), #144 (tenant boundary), #146 (role-assignment), #149 (ledger authorization and actor integrity).
**Policy table:** `docs/superpowers/specs/2026-07-22-authz-policy-table-139.md`, row `iam.ChangePassword`, decision D1.

## 1. The problem

`services/iam/handler.go:216`:

```go
func (h *Handler) ChangePassword(ctx context.Context, req *iamv1.ChangePasswordRequest) (*iamv1.ChangePasswordResponse, error) {
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id")
	}
	if err := h.svc.ChangePassword(ctx, userID, req.GetOldPassword(), req.GetNewPassword()); err != nil {
		return nil, grpcError(err)
	}
	return &iamv1.ChangePasswordResponse{}, nil
}
```

The subject comes from the **request body**. The handler never calls `CallerFromContext`. `Service.ChangePassword` then calls `repo.GetUserByID(ctx, userID)`, which carries no tenant filter, and `repo.UpdatePasswordHash(ctx, userID, hash)`, which carries none either.

So any authenticated user can change **any** user's password, in **any** tenant, provided they know that user's current password. `ChangePasswordRequest` has no `tenant_id` field, so the tenant boundary is not merely unchecked here — it is not represented at all.

This is the defect class #149 fixed on `posted_by`, `closed_by` and `voided_by`. It survived #144 because #144 scoped itself to `tenant_id`, and this message has none. It survived #146 because #146 scoped itself to role assignment.

### 1.1 What limits the impact, and what does not

Knowing the victim's current password is required, so this is not a takeover primitive. It is a **privilege-shaped** defect rather than a credential-stealing one: the caller must already hold the secret, but nothing checks that the secret is *theirs*.

Two realistic paths make it matter. A shared or default password issued at onboarding lets one holder change it for every other holder, across tenant boundaries. And a support operator who legitimately learns a password during a session can change it afterwards, from any tenant, with no record that the actor and the subject differed.

**A permission gate would not fix this.** A caller holding `user:manage` is authorized to administer users — the defect is that the handler cannot tell *which* user the caller is. That distinction is the whole of #149.

## 2. Design

One guard, reusing what #149 already built:

```go
func (h *Handler) ChangePassword(ctx context.Context, req *iamv1.ChangePasswordRequest) (*iamv1.ChangePasswordResponse, error) {
	userID, err := interceptor.ActorFromRequest(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}
	if err := h.svc.ChangePassword(ctx, userID, req.GetOldPassword(), req.GetNewPassword()); err != nil {
		return nil, grpcError(err)
	}
	return &iamv1.ChangePasswordResponse{}, nil
}
```

`ActorFromRequest` (`pkg/interceptor/actor.go`, added by #149) returns the caller's `UserID` from the verified token and never the request's, returns `uuid.Nil` on every error path, accepts an empty request field, and refuses a mismatch with `PermissionDenied` rather than silently substituting the caller.

Three consequences follow, none of which need new code:

- **The tenant boundary closes as a side effect.** The returned id is `caller.UserID`, which by construction belongs to the caller's tenant. `GetUserByID` stays unscoped and stays correct, because it is no longer reachable with a caller-supplied id.
- **The guard cannot be skipped.** It returns the value the handler must pass to the service. Delete the call and there is nothing to give `h.svc.ChangePassword`.
- **Forgery is refused, not corrected.** A request naming another user fails rather than quietly changing the caller's own password, which would misinterpret the request instead of rejecting it.

### 2.1 Rejected alternatives

**Read `CallerFromContext` directly and ignore `user_id`.** Marginally less code. A client sending another user's id would then get a silent success against its *own* password — the request is misread rather than refused, and a broken client is indistinguishable from a hostile one. #149 settled this reasoning.

**Add `tenant_id` to the request and scope the repository.** Wrong shape twice over. `GetUserByID` is deliberately unscoped because refresh-token validation has only a user id available (`services/iam/db/postgres.go:103`), and adding a caller-supplied `tenant_id` would reintroduce the #144 class of defect in order to fix an actor defect. The token already carries the tenant.

**Add an admin reset path in the same change.** Out of scope per D1. If a `user:manage` holder should be able to reset another user's password, that is a *different* RPC with different semantics — it must not require the old password, and it must be audited as an administrative act. Bundling it here would mean one RPC with two authorization models.

## 3. What does not change

- **`Service.ChangePassword` is untouched.** It already takes the actor as a `uuid.UUID` parameter. As in #149, the service layer was written correctly and the handler was the layer trusting the wire.
- **`Repository.GetUserByID` and `UpdatePasswordHash` are untouched.** Both are intentionally unscoped and have legitimate internal callers — `GetUserByID` backs refresh-token validation, and `UpdatePasswordHash` is also called by the login-path rehash at `services/iam/service.go:220`.
- **No migration. No new permission string. No proto field added, removed or renumbered.** `git diff --stat gen/` must be empty.

## 4. Proto

`user_id` is deprecated by comment, matching the convention #144 and #149 used:

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

The field is **not** removed. `proto/buf.yaml` enables the `FILE` breaking category and CI runs `buf breaking proto --against '.git#branch=main,subdir=proto'`; a comment changes neither number, name nor type, so the job passes. `ActorFromRequest` accepts the empty string, so a client that stops sending the field works immediately.

## 5. Testing

`services/iam/handler_test.go` has two `ChangePassword` tests. **Both call `context.Background()` — neither injects a caller at all**, so both will fail with `Unauthenticated` after the change. That is the correct count; a third failure means this reading was wrong.

| Test | Today | After | Repair |
|---|---|---|---|
| `TestHandler_ChangePassword_Success` (`:263`) | passes with no caller | `Unauthenticated` | give it a caller whose id equals the `UserId` it sends |
| `TestHandler_ChangePassword_InvalidUserID` (`:282`) | `InvalidArgument` for `UserId: "bad"` | `Unauthenticated` | give it a caller; it then still asserts `InvalidArgument`, from `ActorFromRequest`'s parse |

**Neither assertion changes.** The second is the one to watch: `ActorFromRequest` checks the caller *before* parsing, so a test with no caller can no longer reach the parse. This is the hazard #146 shipped and #149 documented — a guard ahead of a parse converting one status code into another. Here the conversion is intended and the fix is to supply the caller the test always should have had, not to weaken the assertion.

`services/iam` has `memberCtx(tid)`, which mints a fresh `uuid.New()` for `UserID` on every call, so no existing test can name its own caller. A `memberCtxAs(tid, uid)` variant is needed, mirroring the `callerCtxAs` that #149 added to `services/ledger`.

### 5.1 New tests

- **Forgery is refused.** Caller A sends `user_id` = B: `PermissionDenied`, and `updatePasswordHashFn` must carry a `t.Fatal` body so the test proves the guard fires before the write. `mockRepo`'s unset fn-fields return benign zero values and never panic, so an assertion on the status code alone would also pass against a handler that wrote first and denied second.
- **The subject written is the caller.** With `user_id` empty, `updatePasswordHashFn` must receive `caller.UserID`. `Service.ChangePassword(ctx, userID, old, new)` takes the id positionally, so this is the assertion that catches a wrong id being threaded through.
- **No caller is `Unauthenticated`**, proving the RPC is unreachable without the interceptor chain.

### 5.2 Proving the tests have teeth

A denial test that passes against the vulnerable code is a tautology.

Unlike #149, no signature changes here, so the parent commit still compiles against the new tests — the cheap check is available. The new tests live in the existing `services/iam/handler_test.go` rather than a new file, so the procedure is: create a scratch `git worktree` at `HEAD~1`, copy this branch's `handler_test.go` over the worktree's copy, and run `go test ./services/iam/ -run ChangePassword`.

Expected: the forgery test and the subject-is-the-caller test **fail**; the two repaired pre-existing tests **pass**, because a correct caller is now supplied and the old handler ignores it. A forgery test that passes against the vulnerable handler proves nothing and means the test is wrong.

Remove the worktree afterwards and confirm `git worktree list` shows no leftovers.

## 6. Constraints

- Security change touching `iam`: senior review, two approvals.
- **No Docker, no database.** Never `docker compose … -v` / `down` / `up` against `infra/local/` — that compose is project-scoped and `-v` deletes all local volumes. A disposable worktree is fine.
- Whole-tree `go vet ./...` is the gate. `iam.Repository` has three implementers including a hidden e2e double in `e2e/critical_path/helpers_test.go`.
- `errcheck` runs in CI; `golangci-lint` is not installed locally.
- `gofmt -l services/iam` flags `service.go` and `lifecycle_test.go` on a clean `main`. Pre-existing; CI's Lint does not gate on gofmt. Do not reformat them.
- Coverage on `services/iam` must not regress.
- `gh pr checks` before declaring the PR ready.

## 7. Out of scope

**Session revocation on password change — file as a new issue.** Nothing in the codebase revokes sessions. `Logout` revokes a single refresh token; changing a password revokes nothing. A stolen refresh token therefore keeps working for the full refresh window after the victim changes their password. Closing it needs per-user enumeration or versioning in the token store — a design change to `auth.TokenIssuer`, not a patch, and far larger than this slice.

**An administrative password reset.** See §2.1.

**The rest of iam's ungated RPCs.** `CreateUser`, `UpdateUser`, `ListUsers`, `GetUser`, `ListRoles`, `GetTenant`, `SetTenantAddress`, `Logout` and `DeactivateUser` are slice B.

**Correction to the policy table.** The table marks `iam.GetCurrentUser` 🔴 "must read the token subject". It already does — `handler.go:83` derives everything from the bearer token and never reads a request field. That row is wrong and is corrected on the table's own branch, not here.
