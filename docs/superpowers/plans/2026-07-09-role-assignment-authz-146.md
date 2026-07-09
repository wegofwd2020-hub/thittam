# Role Assignment Authorization — Implementation Plan (#146)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop any authenticated member from granting themselves `super_admin`, and stop role management from crossing the tenant boundary.

**Architecture:** Three layers, three tasks. The **service layer** gains ownership checks — the role and the target user must belong to the caller's tenant. The **handler layer** gains a `user:manage` gate, answered in-process by `h.svc.CheckPermission` (iam must not dial itself). **`AcceptInvitation`** stops discarding its role-assignment error and routes through the guarded service method.

**Tech Stack:** Go 1.22+, gRPC, testify. No migration, no proto change, no repository-interface change.

**Spec:** `docs/superpowers/specs/2026-07-09-role-assignment-authz-146-design.md`

## Global Constraints

- **This is a security change.** Per CLAUDE.md: senior review, 2 approvals (`iam`/security). Every task leaves the tree building and every test passing.
- **iam must NOT use `interceptor.RequirePermission`.** It would dial itself over gRPC to answer a question its own repository holds, and after #138 a nil checker makes `RequirePermission` return `codes.Internal`. Use `h.svc.CheckPermission(ctx, caller.UserID, permUserManage, nil)`.
- **Whole-tree `go vet ./...` is the gate.**
- **errcheck runs in CI; golangci-lint is not installed here.** Check every error return. Its default excludes cover `fmt.Fprint*` to `os.Stdout`/`os.Stderr` but not an arbitrary `io.Writer`.
- **`gh pr checks` is the real gate, not local green.** `main` was red for a day because six PRs merged on local verification alone.
- **No database, no Docker.** NEVER run `docker compose … -v` / `down` / `up` against `infra/local/` — project-scoped; `-v` deletes ALL local volumes (it once destroyed unrelated MinIO dev data). A scratch `git worktree` under `/tmp` is fine; remove it after.
- **Never log a token, a key, or caller metadata.**
- **Coverage:** `services/iam` ≥ 85% (baseline **86.7%**). Record before/after.
- **No interface change.** `GetRoleByID`, `GetUser`, `GetUserPermissions` already exist on all three `iam.Repository` implementers (`*Postgres`, `mockRepo` in `service_test.go`, `iamRepo` in `e2e/critical_path/helpers_test.go`).

## Two traps

### Trap 1: the mock's defaults make denial tests pass vacuously

`mockRepo`'s unset fn-fields return objects **in the caller's tenant**:

```go
func (m *mockRepo) GetRoleByID(ctx, tenantID, roleID) (*Role, error) {
	if m.getRoleByIDFn != nil { return m.getRoleByIDFn(ctx, tenantID, roleID) }
	return &Role{ID: roleID, TenantID: tenantID, Name: "member", IsSystem: true}, nil
}
func (m *mockRepo) GetUser(ctx, tenantID, id) (*User, error) {
	if m.getUserFn != nil { return m.getUserFn(ctx, tenantID, id) }
	return &User{ID: id, TenantID: tenantID, Status: "active"}, nil
}
```

A foreign-role denial test that forgets to stub `getRoleByIDFn` gets a **valid in-tenant role** and the ownership check passes. The test goes green having never executed the denial path.

Every ownership-denial test must **explicitly** stub the fn-field to return `ErrRoleNotFound` / `ErrUserNotFound`, and must additionally assert the write was never reached by installing an `assignRoleFn`/`revokeRoleFn` whose body calls `t.Fatal`.

### Trap 2: the gate flips seven existing tests, and `RevokeRole` has no caller at all

`getUserPermissionsFn` unset returns `nil, nil` — no permissions — so `CheckPermission` returns `false`. Every existing "Success" test for the four gated RPCs will start returning `PermissionDenied`:

| Test | file:line | why |
|---|---|---|
| `TestHandler_AssignRole_Success` | `handler_test.go:283` | bare `newHandler()`, no perms |
| `TestHandler_AssignProjectRole_Success` | `:411` | stubs only `getRoleByIDFn` |
| `TestHandler_RevokeRole_Success` | `:315` | **`context.Background()` — no caller at all** |
| `TestHandler_InviteUser_Success` | `:944` | bare `newHandler()` |
| `TestAssignRole_AssignedByIsTheCaller` | `:1226` | stubs only `assignRoleFn` |
| `TestAssignProjectRole_AssignedByIsTheCaller` | `:1255` | stubs role + assign |
| `TestInviteUser_InvitedByIsTheCaller` | `:1292` | stubs only `createInvitationFn` |

`TestHandler_AssignRole_Success` currently asserts a bare `member` successfully assigns an arbitrary role — **it encodes the vulnerability as intended behaviour.** It must flip to `PermissionDenied`, and a *new* happy-path test must grant the permission.

`TestHandler_RevokeRole_Success` and `TestHandler_RevokeRole_InvalidUserID` pass `context.Background()`. Once `RevokeRole` reads a caller they hit the `!ok` branch. See Task 2's guard ordering, which keeps `InvalidUserID` on the parse path.

## Guard ordering — get this right or four more tests flip

The `InvalidTenantID` / `InvalidUserID` / `InvalidArgs` tests assert `codes.InvalidArgument`. If the permission check runs before the UUID parses, they all become `PermissionDenied`.

**Order for every gated RPC:**

1. `interceptor.TenantFromRequest(ctx, req.GetTenantId())` — where the request carries a tenant. (This already denies a cross-tenant request with `PermissionDenied`, per #144.)
2. Parse the UUID arguments → `InvalidArgument`.
3. `interceptor.CallerFromContext(ctx)` → `Unauthenticated` if absent.
4. **`h.requireUserManage(ctx)`** → `PermissionDenied`.
5. Call the service.

`RevokeRole` has no request tenant, so it skips (1) and takes the tenant from the caller in (3).

## File Structure

| File | Responsibility |
|---|---|
| `services/iam/service.go` | `permUserManage` const; ownership checks in `AssignRole`, `AssignProjectRole`, `RevokeRole`, `InviteUser`; `AcceptInvitation` routed through `s.AssignRole` |
| `services/iam/handler.go` | Task 1 touches it once (`RevokeRole`'s call site, for the signature change). Task 2 adds `requireUserManage` and gates the four RPCs. |
| `services/iam/service_test.go` | ownership tests; `Service.RevokeRole`'s two test call sites gain a tenant |
| `services/iam/handler_test.go` | flip seven tests; add authority + ownership denial tests. **Does not import `errors` today** — Task 2's `PermissionLookupFails_Internal` test needs it added. |

Verified: `Service.RevokeRole` has exactly **three** call sites — `handler.go:262`, `service_test.go:1438`, `service_test.go:1636`. (`handler_test.go:317,327` call the *handler's* `RevokeRole`, not the service's, and are unaffected by the signature change.) `newHandlerWithRepo(*mockRepo) *Handler` already exists at `handler_test.go:37`. The fixtures `fixedTenantID`/`fixedUserID`/`fixedRoleID`/`fixedInviteID` exist at `service_test.go:23-26`, and `service_test.go` already imports `errors`.

**Execution order.** Task 1 is service-layer only and leaves every existing test green, because the mock's defaults return in-tenant objects. Task 2 adds the gate and does the test churn. Task 3 is `AcceptInvitation`. No task ends red.

---

### Task 1: Service-layer ownership — the role and the target user must belong to the tenant

**Files:**
- Modify: `services/iam/service.go` (`AssignRole` 316-328, `AssignProjectRole` 380-399, `RevokeRole` 330-336, `InviteUser` 625-640)
- Modify: `services/iam/service_test.go` (`TestRevokeRole_Success` :1438, `TestRevokeRole_Error` :1636 — signature change)

**Interfaces:**
- Consumes: `repo.GetRoleByID(ctx, tenantID, roleID)` → `ErrRoleNotFound` for a foreign role (its SQL is `WHERE tenant_id = $1 AND id = $2`); `repo.GetUser(ctx, tenantID, userID)` → `ErrUserNotFound` for a foreign user (fetches by PK, then post-filters `row.TenantID != tenantID`).
- Produces, relied on by Tasks 2 and 3:
  ```go
  const permUserManage = "user:manage"
  func (s *Service) AssignRole(ctx, tenantID, userID, roleID, assignedBy uuid.UUID) error
  func (s *Service) AssignProjectRole(ctx, tenantID, userID, roleID, projectID, assignedBy uuid.UUID) error
  func (s *Service) RevokeRole(ctx, tenantID, userID, roleID uuid.UUID) error   // tenantID is NEW
  ```

**Why the service layer.** `Service.AssignRole` currently accepts `tenantID` and **never references it**; `repo.AssignRole` is a bare `INSERT INTO user_roles` with no tenant column. #144 taught the handler to derive the tenant from the verified token and then handed it to a function that discards it. The check belongs where the operands are used.

**Constraint:** no database, no Docker.

- [ ] **Step 1: Add the permission constant**

At the top of `services/iam/service.go`, next to `systemRoles`:

```go
// permUserManage gates role management. Only super_admin holds it (see systemRoles),
// and no RPC creates roles — CreateRole exists only at the repository layer, called by
// seedSystemRoles. So "holds user:manage" is exactly "is a super_admin of this tenant".
//
// That equivalence is contingent: a future custom-role feature could mint a role carrying
// user:manage without carrying everything else, reopening escalation. There is no
// "you may only grant permissions you hold" subset rule anywhere. See the spec, §2 and §8.
const permUserManage = "user:manage"
```

Use it in `systemRoles`' `super_admin` entry, replacing the literal `"user:manage"`, so the two can never drift.

- [ ] **Step 2: Write the failing ownership tests**

Append to `services/iam/service_test.go`. **Each denial test must stub the fn-field explicitly** — the mock's default returns an in-tenant object, so an unstubbed test passes without ever running the denial path. And each must prove the write was never reached.

```go
func TestAssignRole_ForeignRole_Denied(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getRoleByIDFn: func(context.Context, uuid.UUID, uuid.UUID) (*Role, error) {
			return nil, ErrRoleNotFound // the role belongs to another tenant
		},
		assignRoleFn: func(context.Context, *UserRole) error {
			t.Fatal("repository must not be written for a foreign role")
			return nil
		},
	})
	err := svc.AssignRole(context.Background(), uuid.New(), uuid.New(), uuid.New(), uuid.New())
	require.ErrorIs(t, err, ErrRoleNotFound)
}

func TestAssignRole_ForeignUser_Denied(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getUserFn: func(context.Context, uuid.UUID, uuid.UUID) (*User, error) {
			return nil, ErrUserNotFound // the target user belongs to another tenant
		},
		assignRoleFn: func(context.Context, *UserRole) error {
			t.Fatal("repository must not be written for a foreign target user")
			return nil
		},
	})
	err := svc.AssignRole(context.Background(), uuid.New(), uuid.New(), uuid.New(), uuid.New())
	require.ErrorIs(t, err, ErrUserNotFound)
}

func TestAssignRole_InTenant_Succeeds(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	var gotTenantOnRoleLookup, gotTenantOnUserLookup uuid.UUID
	assigned := false
	svc := newTestService(&mockRepo{
		getRoleByIDFn: func(_ context.Context, tenantID, roleID uuid.UUID) (*Role, error) {
			gotTenantOnRoleLookup = tenantID
			return &Role{ID: roleID, TenantID: tenantID, Name: "member", IsSystem: true}, nil
		},
		getUserFn: func(_ context.Context, tenantID, id uuid.UUID) (*User, error) {
			gotTenantOnUserLookup = tenantID
			return &User{ID: id, TenantID: tenantID, Status: "active"}, nil
		},
		assignRoleFn: func(context.Context, *UserRole) error { assigned = true; return nil },
	})
	require.NoError(t, svc.AssignRole(context.Background(), tid, uuid.New(), uuid.New(), uuid.New()))
	assert.True(t, assigned)
	assert.Equal(t, tid, gotTenantOnRoleLookup, "the role must be looked up in the caller's tenant")
	assert.Equal(t, tid, gotTenantOnUserLookup, "the target user must be looked up in the caller's tenant")
}
```

Write the same three shapes for `RevokeRole` (`TestRevokeRole_ForeignRole_Denied`, `..._ForeignUser_Denied`, with `revokeRoleFn` as the `t.Fatal`), one for `AssignProjectRole`'s **target user** (its role lookup already exists), and one for `InviteUser` (`TestInviteUser_ForeignRole_Denied` — stub `getRoleByIDFn` to `ErrRoleNotFound`, `createInvitationFn` to `t.Fatal`).

- [ ] **Step 3: Run them and watch them fail**

Run: `go test ./services/iam/ -run 'ForeignRole|ForeignUser|InTenant' -v`
Expected: the `Foreign*` tests FAIL (no check exists, so `assignRoleFn`'s `t.Fatal` fires), and `RevokeRole`'s fail to compile (signature). Record the real output.

- [ ] **Step 4: Add the ownership checks**

`AssignRole` — replace the body:

```go
// AssignRole grants a role to a user. Both the role and the target user must belong to
// tenantID: repo.AssignRole is a bare INSERT with no tenant column, so this is the only
// place the boundary can be enforced.
func (s *Service) AssignRole(ctx context.Context, tenantID, userID, roleID, assignedBy uuid.UUID) error {
	if _, err := s.repo.GetRoleByID(ctx, tenantID, roleID); err != nil {
		return err // ErrRoleNotFound for a role in another tenant
	}
	if _, err := s.repo.GetUser(ctx, tenantID, userID); err != nil {
		return err // ErrUserNotFound for a user in another tenant
	}
	ur := &UserRole{
		UserID:     userID,
		RoleID:     roleID,
		AssignedBy: assignedBy,
		AssignedAt: time.Now().UTC(),
	}
	if err := s.repo.AssignRole(ctx, ur); err != nil {
		return fmt.Errorf("iam: assign role %s to user %s: %w", roleID, userID, err)
	}
	return nil
}
```

Return the sentinel **unwrapped** so `errors.Is` and `grpcError` see it: `ErrRoleNotFound` and `ErrUserNotFound` both map to `codes.NotFound`, which is also the right answer for an attacker — it does not confirm the foreign role or user exists.

`AssignProjectRole` already calls `GetRoleByID(ctx, tenantID, roleID)`. Add the target-user check after the project-scope gate, before building the `UserRole`.

`RevokeRole` — **gains `tenantID`**:

```go
// RevokeRole removes a role from a user. RevokeRoleRequest carries no tenant_id, so the
// handler supplies the caller's tenant from the verified token.
func (s *Service) RevokeRole(ctx context.Context, tenantID, userID, roleID uuid.UUID) error {
	if _, err := s.repo.GetRoleByID(ctx, tenantID, roleID); err != nil {
		return err
	}
	if _, err := s.repo.GetUser(ctx, tenantID, userID); err != nil {
		return err
	}
	if err := s.repo.RevokeRole(ctx, userID, roleID); err != nil {
		return fmt.Errorf("iam: revoke role %s from user %s: %w", roleID, userID, err)
	}
	return nil
}
```

`InviteUser` — validate the invitation's role belongs to the tenant, before creating it:

```go
func (s *Service) InviteUser(ctx context.Context, inv *Invitation) (*Invitation, error) {
	if inv.RoleID != nil {
		// The invitation's role is assigned at accept time, when there is no caller to
		// authorize. Validate it here, where there is one.
		if _, err := s.repo.GetRoleByID(ctx, inv.TenantID, *inv.RoleID); err != nil {
			return nil, err
		}
	}
	if inv.ID == uuid.Nil {
		inv.ID = uuid.New()
	}
	...
```

- [ ] **Step 5: Fix `RevokeRole`'s three call sites**

Exactly three, all in `services/iam/`:
- `services/iam/handler.go:262` — `h.svc.RevokeRole(ctx, userID, roleID)`. For now pass `interceptor.CallerFromContext(ctx)`'s `TenantID`; Task 2 restructures this handler properly. If the caller is absent, return `Unauthenticated` **before** the service call.
- `services/iam/service_test.go:1438` (`TestRevokeRole_Success`) and `:1636` (`TestRevokeRole_Error`) — add a `tenantID` argument.

Run `grep -rn '\.RevokeRole(' --include=*.go .` and confirm no fourth site. Nothing outside `services/iam/` calls it — verified: no `cmd/`, `e2e/`, seed, or NATS-consumer caller.

- [ ] **Step 6: Run to green**

Run: `go test ./services/iam/ -v 2>&1 | tail -25`
Expected: PASS. Every pre-existing handler test still passes — the mock's `getRoleByIDFn`/`getUserFn` defaults return in-tenant objects, so the new checks are satisfied.

Run: `go vet ./...` — clean.
Run: `go test ./services/iam/ -short -coverprofile=/tmp/c1.out && go tool cover -func=/tmp/c1.out | tail -1` — ≥ 85%. Record.

- [ ] **Step 7: Commit**

```bash
git add services/iam/service.go services/iam/service_test.go services/iam/handler.go
git commit -m "fix(iam): role assignment stays inside the caller's tenant (#146)"
```

---

### Task 2: The `user:manage` gate

**Files:**
- Modify: `services/iam/handler.go` (`AssignRole` 229-251, `RevokeRole` 253-266, `AssignProjectRole` 304-330, `InviteUser` 518-545)
- Modify: `services/iam/handler_test.go` (seven flips + new denial tests)

**Interfaces:** consumes `permUserManage` and `Service.CheckPermission` (Task 1 / existing).

**Why in-process.** The four permission-gated services hold an `interceptor.PermissionChecker` from `iamclient.DialFromEnv` and call `interceptor.RequirePermission`. iam must not copy that: it would dial itself, and after #138 a nil checker makes `RequirePermission` return `codes.Internal` for every gated RPC. `Handler`'s only field is `svc *Service`, and `Service.CheckPermission(ctx, userID, permission, nil)` needs just a user id.

**`grpcError` has no `PermissionDenied` route for a domain sentinel** — produce the status in the handler.

**Constraint:** no database, no Docker.

- [ ] **Step 1: Add the guard helper**

In `services/iam/handler.go`. Note there are currently **no non-RPC methods** with a `(h *Handler)` receiver; this is the first.

```go
// requireUserManage authorizes a role-management RPC.
//
// iam answers this from its own repository rather than dialling itself: the four services
// that use interceptor.RequirePermission hold a gRPC PermissionChecker, and iam holding one
// would mean iam→iam. After #138 a nil checker makes RequirePermission return Internal.
//
// Fails closed on every path: no caller, a repository error, and a negative answer all deny.
func (h *Handler) requireUserManage(ctx context.Context) error {
	caller, ok := interceptor.CallerFromContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "caller identity not present in context")
	}
	allowed, err := h.svc.CheckPermission(ctx, caller.UserID, permUserManage, nil)
	if err != nil {
		// A failed permission lookup is a misconfiguration, not an authorization decision.
		return status.Error(codes.Internal, "permission check failed")
	}
	if !allowed {
		return status.Errorf(codes.PermissionDenied, "requires permission %s", permUserManage)
	}
	return nil
}
```

Do **not** echo the caller's permissions in the error — the caller knows what they hold, and listing them is an oracle.

- [ ] **Step 2: Gate the four RPCs, preserving guard order**

The order matters. `InvalidTenantID` / `InvalidUserID` / `InvalidArgs` tests assert `codes.InvalidArgument`; a permission check before the parses turns them into `PermissionDenied`.

For `AssignRole`, `AssignProjectRole`, `InviteUser`:

```go
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())   // 1. #144's boundary
	if err != nil { return nil, err }
	userID, err := uuid.Parse(req.GetUserId())                              // 2. parses
	if err != nil { return nil, status.Error(codes.InvalidArgument, "invalid user_id") }
	// ... remaining parses ...
	caller, ok := interceptor.CallerFromContext(ctx)                        // 3. caller
	if !ok { return nil, status.Error(codes.Unauthenticated, "caller identity not present in context") }
+	if err := h.requireUserManage(ctx); err != nil {                        // 4. authority
+		return nil, err
+	}
	assignedBy := caller.UserID
	if err := h.svc.AssignRole(ctx, tenantID, userID, roleID, assignedBy); err != nil { ... }
```

`RevokeRole` has **no request tenant**. Parse first (so `InvalidUserID` stays `InvalidArgument`), then caller, then gate, then take the tenant from the caller:

```go
func (h *Handler) RevokeRole(ctx context.Context, req *iamv1.RevokeRoleRequest) (*iamv1.RevokeRoleResponse, error) {
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user_id")
	}
	roleID, err := uuid.Parse(req.GetRoleId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid role_id")
	}
	caller, ok := interceptor.CallerFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "caller identity not present in context")
	}
	if err := h.requireUserManage(ctx); err != nil {
		return nil, err
	}
	if err := h.svc.RevokeRole(ctx, caller.TenantID, userID, roleID); err != nil {
		return nil, grpcError(err)
	}
	return &iamv1.RevokeRoleResponse{}, nil
}
```

- [ ] **Step 3: Add a test helper that grants the permission**

There is no `superAdminCtx`. `memberCtx(tid)` supplies identity and tenant; the permission comes from the mock. Add to `handler_test.go`:

```go
// grantUserManage returns a mockRepo option granting the caller user:manage, as a
// super_admin would hold. The gate calls CheckPermission → repo.GetUserPermissions.
func grantUserManage() func(context.Context, uuid.UUID, *uuid.UUID) ([]string, error) {
	return func(context.Context, uuid.UUID, *uuid.UUID) ([]string, error) {
		return []string{"user:manage"}, nil
	}
}
```

The exact fn-field signature is `func(ctx context.Context, userID uuid.UUID, projectID *uuid.UUID) ([]string, error)` (`service_test.go:66`). The gate passes `nil` for `projectID`.

- [ ] **Step 4: Flip the seven tests**

`TestHandler_AssignRole_Success` currently asserts a bare `member` can assign an arbitrary role — **it encodes the vulnerability**. Split it:

```go
// A bare member may not assign roles. Before #146 this test asserted the opposite.
func TestHandler_AssignRole_MemberDenied(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := newHandlerWithRepo(&mockRepo{
		assignRoleFn: func(context.Context, *UserRole) error {
			t.Fatal("repository must not be reached without user:manage")
			return nil
		},
	})
	_, err := h.AssignRole(memberCtx(tid), &iamv1.AssignRoleRequest{
		TenantId: tid.String(), UserId: uuid.New().String(), RoleId: uuid.New().String(),
	})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_AssignRole_WithUserManage_Succeeds(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := newHandlerWithRepo(&mockRepo{getUserPermissionsFn: grantUserManage()})
	resp, err := h.AssignRole(memberCtx(tid), &iamv1.AssignRoleRequest{
		TenantId: tid.String(), UserId: uuid.New().String(), RoleId: uuid.New().String(),
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}
```

Then repair the other six by adding `getUserPermissionsFn: grantUserManage()` to their `mockRepo`:
`TestHandler_AssignProjectRole_Success` (:411), `TestHandler_RevokeRole_Success` (:315), `TestHandler_InviteUser_Success` (:944), `TestAssignRole_AssignedByIsTheCaller` (:1226), `TestAssignProjectRole_AssignedByIsTheCaller` (:1255), `TestInviteUser_InvitedByIsTheCaller` (:1292).

**`TestHandler_RevokeRole_Success` and `TestHandler_RevokeRole_InvalidUserID` use `context.Background()`** — no caller. `_Success` needs `memberCtx(tid)` plus the permission. `_InvalidUserID` passes a malformed `user_id`, and the parse now precedes the caller check, so it keeps asserting `InvalidArgument` with no context change. **Verify that, don't assume it.**

- [ ] **Step 5: Add the denial tests**

One per gated RPC — a member is denied and the repository is never reached:

```go
func TestHandler_RevokeRole_MemberDenied(t *testing.T) {
	t.Parallel()
	h := newHandlerWithRepo(&mockRepo{
		revokeRoleFn: func(context.Context, uuid.UUID, uuid.UUID) error {
			t.Fatal("repository must not be reached without user:manage")
			return nil
		},
	})
	_, err := h.RevokeRole(memberCtx(uuid.New()), &iamv1.RevokeRoleRequest{
		UserId: uuid.New().String(), RoleId: uuid.New().String(),
	})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}
```

Same for `AssignProjectRole` and `InviteUser`. Plus:

```go
// No caller at all → Unauthenticated, never a uuid.Nil actor.
func TestHandler_RevokeRole_NoCaller_Unauthenticated(t *testing.T) {
	t.Parallel()
	_, err := newHandler().RevokeRole(context.Background(), &iamv1.RevokeRoleRequest{
		UserId: uuid.New().String(), RoleId: uuid.New().String(),
	})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// A repository failure during the permission lookup is a misconfiguration, not a grant.
// NOTE: handler_test.go does not import "errors" today — add it.
func TestHandler_AssignRole_PermissionLookupFails_Internal(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := newHandlerWithRepo(&mockRepo{
		getUserPermissionsFn: func(context.Context, uuid.UUID, *uuid.UUID) ([]string, error) {
			return nil, errors.New("db down")
		},
		assignRoleFn: func(context.Context, *UserRole) error {
			t.Fatal("repository must not be written when the permission check errored")
			return nil
		},
	})
	_, err := h.AssignRole(memberCtx(tid), &iamv1.AssignRoleRequest{
		TenantId: tid.String(), UserId: uuid.New().String(), RoleId: uuid.New().String(),
	})
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}
```

- [ ] **Step 6: Prove the new tests have teeth**

Create a scratch worktree at the pre-change commit, drop in the current `handler_test.go`, and run only the new denial tests against the ungated handler. Each must **fail** there.

```bash
git worktree add --detach /tmp/t146-verify <parent-sha>
cp services/iam/handler_test.go /tmp/t146-verify/services/iam/handler_test.go
(cd /tmp/t146-verify && go test ./services/iam/ -run 'MemberDenied|NoCaller|PermissionLookupFails' -v 2>&1 | tail -20)
git worktree remove --force /tmp/t146-verify
```

Paste the verbatim output. A denial test that passes against the ungated handler is a tautology.

- [ ] **Step 7: Verify**

Run: `go test ./services/iam/... -v 2>&1 | tail -25` — PASS
Run: `go vet ./...` — clean
Run: `go test ./services/iam/ -short -coverprofile=/tmp/c2.out && go tool cover -func=/tmp/c2.out | tail -1` — ≥ 85%
Run: `grep -c 'RequirePermission' services/iam/handler.go` — **0**. iam must not use it.

- [ ] **Step 8: Commit**

```bash
git add services/iam/handler.go services/iam/handler_test.go
git commit -m "fix(iam): role management requires user:manage (#146)"
```

---

### Task 3: `AcceptInvitation` — stop discarding the grant, and re-validate the role

**Files:**
- Modify: `services/iam/service.go` (`AcceptInvitation` 644-696)
- Modify: `services/iam/service_test.go`

**Interfaces:** consumes `Service.AssignRole` (Task 1, now guarded).

**Why.** `AcceptInvitation` is on `PublicMethods` — the invitee has no token, so it cannot be gated. Its privileged decision lives upstream at `InviteUser`, which Task 1 taught to validate the role and Task 2 gated on `user:manage`. Two problems remain here:

```go
if inv.RoleID != nil {
	_ = s.repo.AssignRole(ctx, &UserRole{...})   // error discarded
}
```

The error is **discarded** — a failed grant produces a role-less user and a successful-looking response with a valid token. And it calls `s.repo.AssignRole` **directly**, bypassing Task 1's ownership check. An invitation may be seven days old; its role may have been deleted since.

**Constraint:** no database, no Docker. `newTestService` already wires a token issuer, hasher and verifier, so `AcceptInvitation` runs fully in a unit test.

- [ ] **Step 1: Write the failing tests**

```go
// A failed role grant must fail the acceptance, not return a token to a role-less user.
func TestAcceptInvitation_RoleGrantFails_ReturnsError(t *testing.T) {
	t.Parallel()
	roleID := fixedRoleID
	svc := newTestService(&mockRepo{
		getInvitationByTokenFn: func(context.Context, string) (*Invitation, error) {
			return &Invitation{
				ID: fixedInviteID, TenantID: fixedTenantID, Email: "invited@example.com",
				RoleID: &roleID, Status: "pending", InvitedBy: fixedUserID,
				ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
			}, nil
		},
		assignRoleFn: func(context.Context, *UserRole) error { return errors.New("insert failed") },
	})
	_, err := svc.AcceptInvitation(context.Background(), "valid-token", "newpass")
	require.Error(t, err, "a failed grant must not yield a token")
}

// A seven-day-old invitation may name a role that has since been deleted.
func TestAcceptInvitation_RoleGone_ReturnsError(t *testing.T) {
	t.Parallel()
	roleID := fixedRoleID
	svc := newTestService(&mockRepo{
		getInvitationByTokenFn: func(context.Context, string) (*Invitation, error) {
			return &Invitation{
				ID: fixedInviteID, TenantID: fixedTenantID, Email: "invited@example.com",
				RoleID: &roleID, Status: "pending", InvitedBy: fixedUserID,
				ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
			}, nil
		},
		getRoleByIDFn: func(context.Context, uuid.UUID, uuid.UUID) (*Role, error) {
			return nil, ErrRoleNotFound
		},
		assignRoleFn: func(context.Context, *UserRole) error {
			t.Fatal("must not assign a role that no longer exists")
			return nil
		},
	})
	_, err := svc.AcceptInvitation(context.Background(), "valid-token", "newpass")
	require.ErrorIs(t, err, ErrRoleNotFound)
}
```

- [ ] **Step 2: Run and watch them fail**

Run: `go test ./services/iam/ -run 'TestAcceptInvitation_Role' -v`
Expected: both FAIL — the first because the discarded error yields `NoError`, the second because `t.Fatal` fires (nothing looks the role up).

- [ ] **Step 3: Route through the guarded service method**

```go
	// Assign the pre-selected role if the invitation carried one. Route through
	// s.AssignRole, not s.repo.AssignRole: the invitation may be seven days old and its
	// role may have been deleted, and s.AssignRole re-checks that both the role and the
	// new user belong to the invitation's tenant. A failed grant fails the acceptance —
	// a role-less user holding a valid token is worse than a rejected invitation.
	if inv.RoleID != nil {
		if err := s.AssignRole(ctx, inv.TenantID, user.ID, *inv.RoleID, inv.InvitedBy); err != nil {
			return nil, err
		}
	}
```

Return the error unwrapped so `ErrRoleNotFound` reaches `grpcError` → `codes.NotFound`.

- [ ] **Step 4: Confirm the existing tests still pass**

`TestAcceptInvitation_Success` (`service_test.go:1281`) carries a `RoleID` and stubs neither `getRoleByIDFn` nor `getUserFn` — the mock's defaults return in-tenant objects, so the new lookups succeed and it stays green. **Verify, don't assume.** `TestHandler_AcceptInvitation_Success` (`handler_test.go:982`) has a nil `RoleID` and skips the path entirely.

- [ ] **Step 5: Verify**

Run: `go test ./services/iam/... -v 2>&1 | tail -25` — PASS
Run: `go vet ./...` — clean
Run: `grep -n '_ = s.repo.AssignRole' services/iam/service.go` — **no output**
Coverage ≥ 85%. Record.

- [ ] **Step 6: Commit**

```bash
git add services/iam/service.go services/iam/service_test.go
git commit -m "fix(iam): AcceptInvitation propagates a failed role grant and re-validates the role (#146)"
```

---

## Verification (whole branch, before PR)

- [ ] `go vet ./...` — clean.
- [ ] `go test ./... -short` — PASS.
- [ ] `go test -race ./services/iam/` — PASS.
- [ ] `grep -c 'RequirePermission' services/iam/handler.go` — **0**.
- [ ] `grep -n '_ = s.repo.AssignRole' services/iam/service.go` — no output.
- [ ] `grep -rn '\.RevokeRole(' --include=*.go .` — three sites, all in `services/iam/`.
- [ ] Coverage `services/iam` ≥ 85% (baseline 86.7%).
- [ ] `gofmt -l services/iam` — nothing among files this branch touched.
- [ ] `git diff --stat gen/` — empty. No proto change.
- [ ] **`gh pr checks <n>` after opening the PR.** Local green is not CI green.

## What this does not fix

`super_admin` may still grant `super_admin` — that is administration.

There is **no subset rule** ("you may only grant permissions you hold"). Unnecessary while `user:manage` implies `super_admin`; the wrong shape once custom roles exist.

`GetUserPermissions`' SQL has no `tenant_id` filter. Correct today because a user's `user_roles` only reference their own tenant's roles — an invariant this change enforces at the write path but which the read path still assumes.

`iam.CheckPermission` (the RPC) remains on `PublicMethods` with an unscoped `user_id`. ~100 RPCs platform-wide still enforce no permission check. Both stay in #139.
