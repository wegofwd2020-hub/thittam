# Close the UpdateUser status bypass: dedicated ActivateUser RPC (#162) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-26
**Issue:** #162 (`UpdateUser` bypasses `DeactivateUser`'s `platform_admin` gate) — split from #139 slice B
**Branch:** `fix/iam-activate-user-162` off `main`
**Migration:** none · **Proto:** add `ActivateUser` RPC + messages (buf-safe) · **sqlc:** one new conditional query

## Goal

`DeactivateUser` is gated on `platform_admin` (a structurally separate auth domain), but
`UpdateUser` writes the same `status` column gated only on `user:manage` (a per-tenant
`super_admin`). So a tenant `super_admin` can deactivate/reactivate a user via the
profile-edit path — reproducing or undoing a platform admin's action without holding
`platform_admin`. **Option 1 (issue-preferred):** route status transitions exclusively
through dedicated privileged RPCs — add `ActivateUser` (mirroring `DeactivateUser`), and make
`UpdateUser` ignore `status` entirely, leaving it a pure profile-edit (`display_name`).

## Context (grounding facts, `main` @ 5ca9f92)

- **`UpdateUser`** — handler (`services/iam/handler.go:182-205`) gates on `requireUserManage`
  (= `CheckPermission(user:manage)`, DB-backed = per-tenant `super_admin`) and sets
  `Status: req.GetStatus()`. Service (`service.go:324-339`) currently has the **#181** revoke
  block (`if user.Status != "" && != "active" { RevokeAllForUser }`). Proto
  `UpdateUserRequest.status` = **field 4** (`iam.proto:244-252`).
- **`DeactivateUser` — the gate/shape to mirror** — handler (`handler.go:207-223`):
  `interceptor.RequireRole(ctx, interceptor.RolePlatformAdmin)`, parses `req.GetTenantId()`
  **directly** (not `TenantFromRequest` — a platform admin acts cross-tenant, so tenant comes
  from the request; mirror this). Service (`service.go:341-352`) calls `repo.DeactivateUser`
  then `RevokeAllForUser` (#154). Proto: `rpc DeactivateUser(DeactivateUserRequest) returns
  (DeactivateUserResponse)`; `DeactivateUserRequest{tenant_id, id}` / `DeactivateUserResponse{}`;
  **no HTTP annotation** (gRPC-only). Repo (`db/postgres.go:322-335`) calls sqlc
  `UpdateUserStatus` (`UPDATE users SET status=$2 WHERE id=$1 AND tenant_id=$3 RETURNING *`).
- **`platform_admin` is NOT a tenant role** — separate `platform_users` table (migration 004),
  `"platform"` JWT scope, `pkg/platform/types.go` `Role` type. `RequireRole(RolePlatformAdmin)`
  (`pkg/interceptor/auth.go:70-86`, `RolePlatformAdmin="platform_admin"`) checks the verified JWT
  claim set; a tenant `super_admin` can never satisfy it. Correct gate for `ActivateUser`.
- **No `ActivateUser` exists** (grep-confirmed). Reactivation happens only via the
  `UpdateUser{status:active}` bypass this issue closes.
- **User statuses** (`users_status_check`, migration 002): `active, invited, deactivated`.
- **`grpcError`** (`handler.go:781-839`) has a `FailedPrecondition` arm (:803-810); `ErrUserNotFound`
  → `NotFound` (:783-787).
- **Widening `iam.Repository`** (adding `ActivateUser`) breaks all implementers — Postgres,
  `mockRepo`, and the **hidden e2e double `iamRepo`** (`e2e/critical_path/helpers_test.go`) —
  [[reference-iam-repository-implementers]]. `go build` misses the e2e double; **whole-tree
  `go vet ./...`** catches it.
- **buf/sqlc:** adding an RPC + messages is buf-safe (`FILE` category flags only removals/renumbers);
  `buf generate proto`. sqlc: add one query to `services/iam/db/queries.sql`, `sqlc generate`
  (pinned v1.26.0), commit generated. No migration.

## Design

### 1. Proto (`proto/thittam/iam/v1/iam.proto`) — add ActivateUser, deprecate UpdateUser.status

Add next to `DeactivateUser` (rpc line + messages):
```proto
  rpc ActivateUser(ActivateUserRequest) returns (ActivateUserResponse);
```
```proto
message ActivateUserRequest {
  string tenant_id = 1;
  string id = 2;
}

message ActivateUserResponse {}
```
Deprecate `UpdateUserRequest.status` (keep field 4 — removing is buf-breaking; match the file's
existing `tenant_id` "Deprecated: ignored" precedent):
```proto
  // Deprecated: ignored. Account status transitions go through the platform_admin-gated
  // ActivateUser / DeactivateUser RPCs, not this profile-edit path (#162). Kept for
  // wire-compat; the server no longer reads it.
  string status = 4;
```
`buf generate proto` (target `proto`; revert cross-service `gen/` drift; commit only `gen/iam/`).
No HTTP annotation (gRPC-only, mirroring DeactivateUser).

### 2. sqlc — guarded reactivation query (`services/iam/db/queries.sql`)

Add a conditional query (only `deactivated → active`):
```sql
-- name: ReactivateUser :one
UPDATE users SET status = 'active'
WHERE id = $1 AND tenant_id = $2 AND status = 'deactivated'
RETURNING *;
```
`sqlc generate` → `ReactivateUser`/`ReactivateUserParams{ID, TenantID}`. Commit generated
`queries.sql.go`. (No migration — `users` + status values already exist.)

### 3. Repo (`services/iam/repository.go`, `db/postgres.go`)

Interface (`repository.go`, next to `DeactivateUser`):
```go
	ActivateUser(ctx context.Context, tenantID, id uuid.UUID) error
```
`Postgres.ActivateUser` — conditional update; disambiguate not-found vs not-deactivated on the
error path (the conditional `RETURNING` yields no row for either):
```go
func (p *Postgres) ActivateUser(ctx context.Context, tenantID, id uuid.UUID) error {
	_, err := p.q.ReactivateUser(ctx, ReactivateUserParams{ID: id, TenantID: tenantID})
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("iam/db: activate user: %w", err)
	}
	// No row updated: the user is either absent (wrong id/tenant) or not deactivated.
	// Disambiguate for a correct gRPC code (NotFound vs FailedPrecondition).
	if _, gerr := p.q.GetUser(ctx, GetUserParams{ID: id, TenantID: tenantID}); gerr != nil {
		if errors.Is(gerr, pgx.ErrNoRows) {
			return iam.ErrUserNotFound
		}
		return fmt.Errorf("iam/db: activate user: %w", gerr)
	}
	return iam.ErrNotDeactivated
}
```
(Confirm the exact `GetUser`/`GetUserByID` sqlc query name + params for the tenant-scoped fetch;
mirror whatever `GetUserByID` uses. `:one` queries return `pgx.ErrNoRows` on no match.)

### 4. Errors (`services/iam/errors.go`) + grpcError

Add:
```go
	// ErrNotDeactivated is returned by ActivateUser when the target user is not in
	// 'deactivated' status (e.g. already active, or an unaccepted 'invited' user) —
	// ActivateUser reverses a deactivation only, it does not force-activate (#162).
	ErrNotDeactivated = errors.New("iam: user is not deactivated")
```
`grpcError` (`handler.go`, FailedPrecondition arm ~:803): add `errors.Is(err, ErrNotDeactivated)`.

### 5. Service (`services/iam/service.go`)

Add `ActivateUser` (mirror `DeactivateUser`; **no** session revoke — a deactivated user has no
live sessions, they were revoked at deactivation):
```go
// ActivateUser reverses a deactivation, restoring a 'deactivated' user to 'active'.
// It does not revoke sessions (a deactivated user has none — #154 revoked them) and does
// not force-activate an 'invited' or already-active user (repo returns ErrNotDeactivated).
func (s *Service) ActivateUser(ctx context.Context, tenantID, id uuid.UUID) error {
	if err := s.repo.ActivateUser(ctx, tenantID, id); err != nil {
		return fmt.Errorf("iam: activate user %s: %w", id, err)
	}
	return nil
}
```
**Remove the #181 revoke block** from `UpdateUser` (dead once the handler stops passing status):
```go
func (s *Service) UpdateUser(ctx context.Context, user *User) (*User, error) {
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("iam: update user %s: %w", user.ID, err)
	}
	return user, nil
}
```

### 6. Handler (`services/iam/handler.go`)

Add `ActivateUser` (mirror `DeactivateUser` — RequireRole platform_admin, parse `req.GetTenantId()`
directly + `req.GetId()`, call svc, return `ActivateUserResponse{}`):
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
**`UpdateUser` handler — stop reading status:** drop `Status: req.GetStatus()` from the `User{}`
literal (build `User{ID, TenantID, DisplayName}` only). The `user:manage` gate + rest unchanged.

### 7. Tests

- **Handler** (`handler_test.go`): `TestHandler_ActivateUser_Success` (platformAdminCtx → success),
  `_PermissionDenied` (no platform_admin caller → `codes.PermissionDenied`), `_NotDeactivated`
  (repo returns `ErrNotDeactivated` → `codes.FailedPrecondition`), `_InvalidID`/`_InvalidTenant` —
  mirror the `TestHandler_DeactivateUser_*` pattern + `platformAdminCtx()` (`handler_test.go:21`).
  **Delete** `TestHandler_UpdateUser_EmptyStatusIsNotAWipe` (handler no longer touches status).
  Keep/adjust `TestHandler_UpdateUser_Success` (drop the `Status` field from the request or leave
  it — it's ignored now; assert display_name still updates) and `_RequiresUserManage`.
- **Service** (`service_test.go`): `TestActivateUser_Success` (repo `activateUserFn` called with
  the id, no revoke — assert `revokeAllForUserFn` NOT called), `TestActivateUser_NotDeactivated_Propagated`
  (repo returns `ErrNotDeactivated` → surfaced). **Delete the three #181 tests**
  `TestUpdateUser_RevokesSessionsOnDeactivate` / `_NoRevokeOnActiveOrEmpty` / `_RevokeFailure_IsReported`.
  Add `activateUserFn` to `mockTokenIssuer`? No — to `mockRepo` (next to `deactivateUserFn`).
- **e2e/integration doubles:** add `ActivateUser` to the hidden `iamRepo`
  (`e2e/critical_path/helpers_test.go`) and any other `iam.Repository` double (whole-tree `go vet`
  finds them) — no-op stub is fine.
- **Repo integration** (optional, `//go:build integration`): a real-Postgres test that
  `ActivateUser` on a deactivated user → active, and on an active/invited user → `ErrNotDeactivated`,
  and on a missing user → `ErrUserNotFound` (proves the conditional SQL + disambiguation — the
  sqlc-WHERE-blind-spot class, [[reference-sqlc-where-clause-blind-spot]]). Mirror an existing iam
  integration test's tenant/user seeding.
- **Gates:** `buf lint` + `buf breaking`; `sqlc generate` clean (Codegen Freshness);
  `go test ./services/iam/... ./pkg/interceptor/... -race`; **whole-tree `go vet ./...`**
  (Repository widening); `go build ./...`; `gofmt -l` on touched Go files. Real-Postgres
  Integration + Migration Validate run in CI.

## Non-goals

- **D2 self-service profile editing** — Option 1 unblocks it (UpdateUser is now status-free), but
  adding a "caller may edit themselves" carve-out is a separate change.
- No change to `DeactivateUser`'s direct `req.GetTenantId()` parsing (mirror it; platform-admin
  cross-tenant is intentional).
- No simplification of the repo `UpdateUser` `COALESCE(NULLIF(...))` guard or its integration test
  (kept as defense-in-depth; harmless once status is always empty from the handler).
- No `ActivateUser` session revoke; no migration; no HTTP/REST route for ActivateUser.
- No sweep of other `Internal`-defaulting iam sentinels (that's #163).

## Review weight

`iam` **authorization** (privilege separation of a security-critical column) + proto + sqlc →
security-sensitive; senior engineer required per CLAUDE.md. Standard 2 approvals + senior.
Whole-branch review on the most capable model.
