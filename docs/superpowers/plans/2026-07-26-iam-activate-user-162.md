# iam ActivateUser + UpdateUser status-bypass fix (#162) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the `UpdateUser` status bypass — add a `platform_admin`-gated `ActivateUser` RPC (guarded `deactivated → active` only, mirroring `DeactivateUser`), and make `UpdateUser` ignore `status` entirely (pure profile-edit), retiring #181's now-dead revoke-in-UpdateUser.

**Architecture:** Three tasks, each builds tree-wide. (1) Contract: proto `ActivateUser` RPC + messages + deprecate `UpdateUserRequest.status`; sqlc `ReactivateUser` conditional query. (2) Backend: `iam.Repository.ActivateUser` (+ all implementers), `Postgres.ActivateUser`, `Service.ActivateUser`, `ErrNotDeactivated`. (3) Handler `ActivateUser` + atomic `UpdateUser` status-removal (handler stops passing status AND the #181 revoke block is deleted in the same commit).

**Tech Stack:** Go 1.25, gRPC/buf, sqlc (pinned v1.26.0), pgx/v5, `pkg/interceptor` (RequireRole/platform_admin), testify.

## Global Constraints

- **`ActivateUser` gate:** `interceptor.RequireRole(ctx, interceptor.RolePlatformAdmin)` — mirror `DeactivateUser` (handler.go:207-223) exactly, incl. parsing `req.GetTenantId()` **directly** (platform admin acts cross-tenant). NO session revoke (a deactivated user has no live sessions).
- **Guarded transition:** `ReactivateUser` SQL is `... WHERE id=$1 AND tenant_id=$2 AND status='deactivated'` — only reverses a deactivation. Repo disambiguates the no-row case: user absent → `ErrUserNotFound` (NotFound); user present but not deactivated → `ErrNotDeactivated` (FailedPrecondition).
- **`UpdateUser` becomes status-free ATOMICALLY** (Task 3): the handler stops setting `Status: req.GetStatus()` AND `Service.UpdateUser`'s #181 revoke block is removed in the same commit — so there is never a commit where UpdateUser writes status without revoking.
- **Proto:** adding an RPC + messages is buf-safe (`FILE` category); `UpdateUserRequest.status` (field 4) is kept-but-deprecated (removing is breaking) — match the file's `tenant_id` "Deprecated: ignored" precedent. No HTTP annotation (gRPC-only). `buf generate proto`, revert cross-service `gen/` drift, commit only `gen/iam/`.
- **sqlc** pinned v1.26.0; `sqlc generate`, commit generated `services/iam/db/queries.sql.go`; scope `git add` to `services/iam/db/` (revert any unrelated cross-service drift). No migration (`users`/statuses already exist). sqlc does NOT validate the bare `status='deactivated'` WHERE literal — the real-Postgres Integration/Migration jobs are the authoritative gate ([[reference-sqlc-where-clause-blind-spot]]).
- **Widening `iam.Repository` (add `ActivateUser`) breaks all implementers** — `Postgres`, `mockRepo` (services/iam/service_test.go), and the **hidden e2e double `iamRepo`** (e2e/critical_path/helpers_test.go); there may be others. `go build ./...` skips other packages' `_test.go`; **only whole-tree `go vet ./...`** catches them. Grep + fix all in Task 2's commit ([[reference-iam-repository-implementers]]).
- **`platform_admin` ≠ tenant `super_admin`** — it's a separate auth domain (`platform_users`, `"platform"` scope). A tenant super_admin can never satisfy `RequireRole(platform_admin)`; that's the whole point of #162.
- Gate every Go/codegen task with `gofmt -l <touched .go files>` (empty; some iam files carry pre-existing gofmt debt on `main` — diff-vs-main to tell yours from theirs) + `go vet ./...` + `go build ./...`.
- **Security-sensitive** (iam authorization + proto + sqlc) → senior review per CLAUDE.md.
- Commits Conventional-Commits (scope `iam`), ending `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `proto/thittam/iam/v1/iam.proto` | ActivateUser rpc+messages; deprecate status | 1 |
| `gen/iam/v1/*.pb.go` | buf-regenerated | 1 |
| `services/iam/db/queries.sql` + `queries.sql.go` | `ReactivateUser` conditional query | 1 |
| `services/iam/repository.go` | `ActivateUser` interface method | 2 |
| `services/iam/db/postgres.go` | `Postgres.ActivateUser` (guarded + disambiguation) | 2 |
| `services/iam/errors.go` | `ErrNotDeactivated` | 2 |
| `services/iam/service.go` | `Service.ActivateUser`; remove #181 revoke from `UpdateUser` | 2,3 |
| `services/iam/service_test.go` | ActivateUser tests; delete 3 #181 tests; mock `activateUserFn` | 2,3 |
| `e2e/critical_path/helpers_test.go` (+ any other double) | `iamRepo.ActivateUser` stub | 2 |
| `services/iam/handler.go` | `ActivateUser` handler + grpcError arm; strip status from `UpdateUser` | 3 |
| `services/iam/handler_test.go` | ActivateUser handler tests; delete EmptyStatusIsNotAWipe | 3 |

---

### Task 1: Contract — proto ActivateUser + sqlc ReactivateUser

**Files:** Modify `proto/thittam/iam/v1/iam.proto`, `services/iam/db/queries.sql`; regenerate `gen/iam/`, `services/iam/db/queries.sql.go`.

**Interfaces:**
- Produces: `iamv1.ActivateUserRequest{TenantId, Id}` / `iamv1.ActivateUserResponse{}` + the `ActivateUser` server-interface method (served by `UnimplementedIAMServiceServer` until Task 3); `db.ReactivateUser`/`ReactivateUserParams{ID, TenantID}`.

- [ ] **Step 1: Proto — add ActivateUser, deprecate status**

In `proto/thittam/iam/v1/iam.proto`, add the rpc next to `DeactivateUser` (:45):
```proto
  rpc ActivateUser(ActivateUserRequest) returns (ActivateUserResponse);
```
Add messages near `DeactivateUserRequest`/`Response` (:254-259):
```proto
message ActivateUserRequest {
  string tenant_id = 1;
  string id = 2;
}

message ActivateUserResponse {}
```
Deprecate `UpdateUserRequest.status` (field 4) — replace `string status = 4;` with:
```proto
  // Deprecated: ignored. Account status transitions go through the platform_admin-gated
  // ActivateUser / DeactivateUser RPCs, not this profile-edit path (#162). Kept for
  // wire-compat; the server no longer reads it.
  string status = 4;
```

- [ ] **Step 2: buf generate**

Run `buf generate proto` (or `make generate-proto`). `git status`: revert any `gen/` changes OUTSIDE `gen/iam/` (`git checkout -- <those>`). Confirm `gen/iam/v1/iam.pb.go` has `ActivateUserRequest`/`ActivateUserResponse` and `gen/iam/v1/iam_grpc.pb.go` has `ActivateUser` in the server interface + `UnimplementedIAMServiceServer.ActivateUser`.

- [ ] **Step 3: sqlc — add ReactivateUser query**

In `services/iam/db/queries.sql`, add near `UpdateUserStatus` (:150):
```sql
-- name: ReactivateUser :one
UPDATE users SET status = 'active'
WHERE id = $1 AND tenant_id = $2 AND status = 'deactivated'
RETURNING *;
```

- [ ] **Step 4: sqlc generate**

Run `sqlc generate` (pinned v1.26.0, or `make generate-sqlc`). Confirm `services/iam/db/queries.sql.go` has `ReactivateUser` + `ReactivateUserParams{ID uuid.UUID, TenantID uuid.UUID}`. If other packages' generated files changed, that's a version mismatch — revert them; only `services/iam/db/` should change.

- [ ] **Step 5: Verify + commit**
```bash
buf lint proto && buf breaking proto --against '.git#branch=main,subdir=proto'
go build ./... && go vet ./... && go test ./services/iam/ -race
gofmt -l services/iam/db/
```
Tree builds (Handler embeds `UnimplementedIAMServiceServer` → `ActivateUser` served as Unimplemented; `ReactivateUser` generated-but-unused). `buf breaking` passes (additions only).
```bash
git add proto/thittam/iam/v1/iam.proto gen/iam/ services/iam/db/queries.sql services/iam/db/queries.sql.go
git commit -m "feat(iam): add ActivateUser RPC + ReactivateUser query; deprecate UpdateUser.status (#162)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Backend — Repository.ActivateUser + Service.ActivateUser + ErrNotDeactivated

**Files:** Modify `services/iam/repository.go`, `db/postgres.go`, `errors.go`, `service.go`, `service_test.go`, `e2e/critical_path/helpers_test.go` (+ any other `iam.Repository` double)

**Interfaces:**
- Consumes: Task 1's `db.ReactivateUser`/`ReactivateUserParams`, `db.GetUser`/`GetUserParams`.
- Produces: `Repository.ActivateUser(ctx, tenantID, id uuid.UUID) error`; `Service.ActivateUser(...)`; `ErrNotDeactivated`; `mockRepo.activateUserFn`.

- [ ] **Step 1: Write failing service tests**

In `services/iam/service_test.go`: add `activateUserFn func(ctx context.Context, tenantID, id uuid.UUID) error` to `mockRepo` (next to `deactivateUserFn`) + the dispatch method; add:
```go
func TestActivateUser_Success(t *testing.T) {
	t.Parallel()
	var gotID uuid.UUID
	revoked := false
	tokens := &mockTokenIssuer{revokeAllForUserFn: func(_ context.Context, _ uuid.UUID) error { revoked = true; return nil }}
	repo := &mockRepo{activateUserFn: func(_ context.Context, _, id uuid.UUID) error { gotID = id; return nil }}
	svc := NewService(repo, &mockAuthenticator{}, tokens, &mockHasher{}, &mockVerifier{})
	require.NoError(t, svc.ActivateUser(context.Background(), fixedTenantID, fixedUserID))
	assert.Equal(t, fixedUserID, gotID)
	assert.False(t, revoked, "activation must not revoke sessions")
}

func TestActivateUser_NotDeactivated_Propagated(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{activateUserFn: func(_ context.Context, _, _ uuid.UUID) error { return ErrNotDeactivated }}
	svc := NewService(repo, &mockAuthenticator{}, &mockTokenIssuer{}, &mockHasher{}, &mockVerifier{})
	err := svc.ActivateUser(context.Background(), fixedTenantID, fixedUserID)
	assert.ErrorIs(t, err, ErrNotDeactivated)
}
```
(Source the exact `NewService` arg order + `mockRepo`/`deactivateUserFn` shape from `TestDeactivateUser_RevokesAllSessions`.)

- [ ] **Step 2: Run — expect FAIL** (`ActivateUser`/`ErrNotDeactivated` undefined): `go test ./services/iam/ -run TestActivateUser`

- [ ] **Step 3: Error + interface**

`services/iam/errors.go`, add:
```go
	// ErrNotDeactivated is returned by ActivateUser when the target user is not in
	// 'deactivated' status (already active, or an unaccepted 'invited' user). ActivateUser
	// reverses a deactivation only; it does not force-activate (#162).
	ErrNotDeactivated = errors.New("iam: user is not deactivated")
```
`services/iam/repository.go`, add next to `DeactivateUser`:
```go
	ActivateUser(ctx context.Context, tenantID, id uuid.UUID) error
```

- [ ] **Step 4: Postgres.ActivateUser (guarded + disambiguation)**

In `services/iam/db/postgres.go`, mirror `DeactivateUser` (:322-335) but conditional + disambiguating:
```go
func (p *Postgres) ActivateUser(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := p.q.ReactivateUser(ctx, ReactivateUserParams{ID: id, TenantID: tenantID})
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("iam/db: activate user: %w", err)
	}
	// No row updated: user absent (wrong id/tenant) or present-but-not-deactivated.
	// Disambiguate for a correct gRPC code.
	if _, gerr := p.q.GetUser(ctx, GetUserParams{ID: id, TenantID: tenantID}); gerr != nil {
		if errors.Is(gerr, pgx.ErrNoRows) {
			return iam.ErrUserNotFound
		}
		return fmt.Errorf("iam/db: activate user: %w", gerr)
	}
	return iam.ErrNotDeactivated
}
```
(Confirm `GetUser`'s generated param struct name/fields — `GetUserParams{ID, TenantID}` — from `queries.sql.go`; adjust if the tenant-scoped fetch is named differently. Confirm `errors`/`pgx`/`fmt` are imported in postgres.go — they are, used by `DeactivateUser`.)

- [ ] **Step 5: Service.ActivateUser + find all Repository doubles**

In `services/iam/service.go`, add (mirror `DeactivateUser` minus the revoke):
```go
// ActivateUser reverses a deactivation, restoring a 'deactivated' user to 'active'.
// No session revoke — a deactivated user has no live sessions (#154 revoked them at
// deactivation); and it does not force-activate an invited/active user (ErrNotDeactivated).
func (s *Service) ActivateUser(ctx context.Context, tenantID, id uuid.UUID) error {
	if err := s.repo.ActivateUser(ctx, tenantID, id); err != nil {
		return fmt.Errorf("iam: activate user %s: %w", id, err)
	}
	return nil
}
```
Grep the WHOLE tree for `iam.Repository` implementers and add `ActivateUser` to each:
```bash
grep -rln "func.*DeactivateUser(ctx context.Context" --include=*.go
```
Known: `Postgres` (done Step 4), `mockRepo` (service_test.go — a recording stub via `activateUserFn`, Step 1), and the hidden **`iamRepo`** in `e2e/critical_path/helpers_test.go` (add a no-op `ActivateUser` stub mirroring its `DeactivateUser`). Add to every implementer found. Do NOT touch `UpdateUser` in this task.

- [ ] **Step 6: Run — expect PASS + whole-tree vet**
```bash
go test ./services/iam/ -race && go vet ./... && go build ./...
gofmt -l services/iam/repository.go services/iam/db/postgres.go services/iam/errors.go services/iam/service.go services/iam/service_test.go e2e/critical_path/helpers_test.go
```
`go vet ./...` (WHOLE TREE) MUST pass — proves every `iam.Repository` implementer (incl. the e2e double) got `ActivateUser`.

- [ ] **Step 7: Optional integration test**

If adding is cheap, add a `//go:build integration` test in `services/iam/db/` (mirror an existing iam integration test's tenant+user seeding): seed a `deactivated` user → `ActivateUser` → status `active`; an `active` user → `ErrNotDeactivated`; a missing id → `ErrUserNotFound`. This is the authoritative proof of the conditional SQL (sqlc can't validate the bare WHERE literal). Skips locally without `THITTAM_TEST_DSN`; CI's real-Postgres job runs it.

- [ ] **Step 8: Commit**
```bash
git add services/iam/repository.go services/iam/db/postgres.go services/iam/errors.go services/iam/service.go services/iam/service_test.go e2e/critical_path/helpers_test.go
# + any other implementer file found in Step 5; + the integration test if added
git commit -m "feat(iam): guarded ActivateUser (deactivated→active) backend (#162)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Handler ActivateUser + atomic UpdateUser status-removal

**Files:** Modify `services/iam/handler.go`, `handler_test.go`, `service.go` (UpdateUser only), `service_test.go` (delete 3 #181 tests)

**Interfaces:**
- Consumes: Task 1's `iamv1.ActivateUserRequest`/`Response`; Task 2's `Service.ActivateUser`, `ErrNotDeactivated`.

- [ ] **Step 1: Write failing handler tests**

In `services/iam/handler_test.go`, mirror `TestHandler_DeactivateUser_Success`/`_PermissionDenied` (:325, using `platformAdminCtx()` :21):
```go
func TestHandler_ActivateUser_Success(t *testing.T) {
	t.Parallel()
	resp, err := newHandlerWith(&mockRepo{activateUserFn: func(_ context.Context, _, _ uuid.UUID) error { return nil }}).
		ActivateUser(platformAdminCtx(), &iamv1.ActivateUserRequest{TenantId: uuid.New().String(), Id: uuid.New().String()})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestHandler_ActivateUser_PermissionDenied(t *testing.T) {
	t.Parallel()
	_, err := newHandler().ActivateUser(context.Background(), &iamv1.ActivateUserRequest{TenantId: uuid.New().String(), Id: uuid.New().String()})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_ActivateUser_NotDeactivated(t *testing.T) {
	t.Parallel()
	_, err := newHandlerWith(&mockRepo{activateUserFn: func(_ context.Context, _, _ uuid.UUID) error { return ErrNotDeactivated }}).
		ActivateUser(platformAdminCtx(), &iamv1.ActivateUserRequest{TenantId: uuid.New().String(), Id: uuid.New().String()})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}
```
(Source the exact handler-construction helper — `newHandler()` / how a handler with a custom `mockRepo` is built — from the existing `TestHandler_DeactivateUser_*` tests; adjust `newHandlerWith` to whatever the file uses.)
Also **delete** `TestHandler_UpdateUser_EmptyStatusIsNotAWipe` (:297) and, in `services/iam/service_test.go`, **delete** the three #181 tests `TestUpdateUser_RevokesSessionsOnDeactivate` / `_NoRevokeOnActiveOrEmpty` / `_RevokeFailure_IsReported`.

- [ ] **Step 2: Run — expect FAIL** (`Handler.ActivateUser` undefined / returns Unimplemented): `go test ./services/iam/ -run TestHandler_ActivateUser`

- [ ] **Step 3: Handler.ActivateUser + grpcError arm**

In `services/iam/handler.go`, add (mirror `DeactivateUser` :207-223):
```go
func (h *Handler) ActivateUser(ctx context.Context, req *iamv1.ActivateUserRequest) (*iamv1.ActivateUserResponse, error) {
	if err := interceptor.RequireRole(ctx, interceptor.RolePlatformAdmin); err != nil {
		return nil, err
	}
	tenantID, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid id")
	}
	if err := h.svc.ActivateUser(ctx, tenantID, id); err != nil {
		return nil, grpcError(err)
	}
	return &iamv1.ActivateUserResponse{}, nil
}
```
In `grpcError` (`handler.go`, a `FailedPrecondition` arm ~:803), add `errors.Is(err, ErrNotDeactivated)`.

- [ ] **Step 4: Strip status from UpdateUser (handler + service, atomically)**

In `services/iam/handler.go` `UpdateUser` (:194-199), drop the `Status` field:
```go
	user := &User{
		ID:          id,
		TenantID:    tenantID,
		DisplayName: req.GetDisplayName(),
	}
```
In `services/iam/service.go` `UpdateUser`, remove the #181 revoke block so it is just:
```go
func (s *Service) UpdateUser(ctx context.Context, user *User) (*User, error) {
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("iam: update user %s: %w", user.ID, err)
	}
	return user, nil
}
```
(Update the doc comment to note status is no longer settable here — it goes through Activate/DeactivateUser, #162.)

- [ ] **Step 5: Run — expect PASS + gate**
```bash
go test ./services/iam/ -race && go vet ./... && go build ./...
gofmt -l services/iam/handler.go services/iam/handler_test.go services/iam/service.go services/iam/service_test.go
```
All green. Confirm the remaining `TestHandler_UpdateUser_*` (Success, RequiresUserManage) still pass — `UpdateUser` still edits `display_name` and is still `user:manage`-gated; it just ignores status.

- [ ] **Step 6: Commit**
```bash
git add services/iam/handler.go services/iam/handler_test.go services/iam/service.go services/iam/service_test.go
git commit -m "fix(iam): wire ActivateUser handler; UpdateUser ignores status (closes bypass) (#162)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- Proto ActivateUser rpc+messages + deprecate status → Task 1 ✅
- sqlc ReactivateUser conditional query → Task 1 ✅
- Repository.ActivateUser + all implementers (Postgres, mockRepo, e2e double) → Task 2 ✅
- Postgres guarded + not-found/not-deactivated disambiguation → Task 2 ✅
- ErrNotDeactivated + grpcError→FailedPrecondition → Task 2/3 ✅
- Service.ActivateUser (no revoke) → Task 2 ✅
- Handler ActivateUser (platform_admin gate, mirror DeactivateUser) → Task 3 ✅
- UpdateUser status-free (handler strip + #181 revoke-block removal, atomic) + delete 4 stale tests → Task 3 ✅
- Non-goals honored (no D2, no DeactivateUser scoping change, no COALESCE simplify, no migration/REST, no #163 sweep) ✅

**Placeholder scan:** production code (proto, query, repo, service, handler, error, grpcError) is fully given. Test snippets specify behavior + the exact sibling test to copy construction from — the `newHandler`/`platformAdminCtx`/`mockRepo` fixtures live in the file the implementer reads. Not TODOs.

**Type consistency:** `ActivateUser(ctx, tenantID, id uuid.UUID) error` identical across `Repository` (Task 2), `Postgres`/`mockRepo`/`iamRepo` (Task 2), `Service` (Task 2), and the handler call site (Task 3). `iamv1.ActivateUserRequest{TenantId, Id}`/`ActivateUserResponse{}` (Task 1) consumed by the handler (Task 3). `ReactivateUserParams{ID, TenantID}` (Task 1) used in Postgres (Task 2). `ErrNotDeactivated` (Task 2) returned by Postgres, mapped in grpcError (Task 3), asserted in tests.

**Ordering:** Task 1 (contract; tree builds via UnimplementedIAMServiceServer + unused query) → Task 2 (backend; whole-tree vet for Repository widening; UpdateUser untouched) → Task 3 (handler + atomic UpdateUser status-removal so no commit writes status without revoking). Every commit builds tree-wide and passes its gate.
