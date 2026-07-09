# Role assignment requires authority and stays inside the tenant

**Status:** approved (design), 2026-07-09
**Issue:** [#146](https://github.com/wegofwd2020-hub/thittam/issues/146) — slice 2 of [#139](https://github.com/wegofwd2020-hub/thittam/issues/139)
**Follows:** [#138](https://github.com/wegofwd2020-hub/thittam/issues/138) (fail-closed authentication), [#144](https://github.com/wegofwd2020-hub/thittam/issues/144) (tenant boundary)

## 1. Problem

`AssignRole` performs **no authorization check**. Any authenticated member of a tenant can
grant themselves `super_admin` in that tenant:

```go
func (h *Handler) AssignRole(ctx context.Context, req *iamv1.AssignRoleRequest) (*iamv1.AssignRoleResponse, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	...
	assignedBy := caller.UserID
	if err := h.svc.AssignRole(ctx, tenantID, userID, roleID, assignedBy); err != nil {
```

Nothing asks whether the caller may grant that role. Nothing asks whether the role, or the
target user, belongs to the caller's tenant.

### It is three defects wearing one name

**(a) No authority check.** A bare `member` can call `AssignRole` and become `super_admin`.

**(b) `Service.AssignRole` discards its `tenantID`.**

```go
func (s *Service) AssignRole(ctx context.Context, tenantID, userID, roleID, assignedBy uuid.UUID) error {
	ur := &UserRole{UserID: userID, RoleID: roleID, AssignedBy: assignedBy, AssignedAt: time.Now().UTC()}
	if err := s.repo.AssignRole(ctx, ur); err != nil {
```

The parameter is never referenced. `repo.AssignRole` is a bare `INSERT INTO user_roles`
keyed on `user_id`/`role_id` with no tenant column. So `AssignRole` accepts a **foreign-tenant
`role_id`** and a **foreign-tenant `user_id`**. #144 taught the handler to derive the tenant
from the verified token, and then handed it to a function that throws it away.

`AssignProjectRole` validates the role's tenant *incidentally* — `GetRoleByID(ctx, tenantID,
roleID)` is tenant-scoped SQL, and it needed the role's name for the project-scope gate. It
still accepts a foreign target user. `RevokeRole` takes no tenant at all and validates neither.

**(c) `InviteUser` is a parallel, ungated grant path.** It sets `inv.RoleID` from the request
and validates nothing:

```go
func (s *Service) InviteUser(ctx context.Context, inv *Invitation) (*Invitation, error) {
	// generates a token, sets status/expiry, inserts. No role check.
```

`AcceptInvitation` — on `PublicMethods`, **no caller in context** — then assigns it, calling
`repo.AssignRole` directly and **discarding the error**:

```go
if inv.RoleID != nil {
	_ = s.repo.AssignRole(ctx, &UserRole{UserID: user.ID, RoleID: *inv.RoleID, AssignedBy: inv.InvitedBy, ...})
}
```

Invite yourself at a second address with `role_id = <super_admin>`, accept, and you are
`super_admin`. **Gating `AssignRole` alone would be theatre** — it closes the front door and
leaves this one open.

### Reachability

These RPCs carry no `google.api.http` annotation, so grpc-gateway never routes them: they are
not reachable from a browser. They are reachable over the gRPC port by any authenticated
caller — every service pod, and anything else inside the cluster. Post-#138 that requires a
valid token; post-#144 it requires the caller's own tenant. It does not require any privilege.

## 2. Why `user:manage` is the right gate

`super_admin` is the **only** seeded role holding `user:manage`:

| role | has `user:manage` |
|---|---|
| `super_admin` | **yes** |
| `manager`, `coordinator`, `accountant`, `member`, `inventory_manager`, `project_supervisor` | no |

And **no RPC creates roles.** `CreateRole` exists only at the repository layer, called solely
by `seedSystemRoles` at tenant creation. A tenant cannot mint a role, so the seven system
roles are the entire universe of assignable roles.

Therefore "holds `user:manage`" is exactly "is a `super_admin` of this tenant", and the gate
stops the attack: a `member` is refused before any assignment happens. A `super_admin`
granting `super_admin` to a colleague is ordinary administration, not escalation.

**This reasoning is contingent on there being no custom roles.** If a future feature lets a
tenant define one, a role carrying `user:manage` without carrying everything else would
reopen escalation. There is **no** "you may only grant roles whose permissions are a subset of
your own" check anywhere in the codebase. Recorded in §8.

## 3. Mechanism: iam authorizes in-process

The four services that gate on permissions (`project`, `budget`, `expense`, `inventory`) hold
an `interceptor.PermissionChecker`, wired from `iamclient.DialFromEnv`, and call
`interceptor.RequirePermission`. **iam must not copy that.** It would be dialling itself over
gRPC to ask a question it can answer from its own repository — and after #138, wiring a nil
checker makes `RequirePermission` return `codes.Internal` for every gated RPC.

`Handler` holds `svc *Service`, and `Service.CheckPermission` already exists:

```go
func (s *Service) CheckPermission(ctx context.Context, userID uuid.UUID, permission string, projectID *uuid.UUID) (bool, error)
```

It needs only a user id — the SQL joins `user_roles → roles` keyed on `ur.user_id`. So iam
authorizes with a direct call, no network hop:

```go
// requireUserManage authorizes a role-management RPC. iam answers this from its own
// repository rather than dialling itself; see §3 of the design.
func (h *Handler) requireUserManage(ctx context.Context) error {
	caller, ok := interceptor.CallerFromContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "caller identity not present in context")
	}
	allowed, err := h.svc.CheckPermission(ctx, caller.UserID, permUserManage, nil)
	if err != nil {
		return status.Error(codes.Internal, "permission check failed")
	}
	if !allowed {
		return status.Error(codes.PermissionDenied, "requires permission user:manage")
	}
	return nil
}
```

Fails closed on every path: no caller, a repository error, or a negative answer all deny.

## 4. Ownership: the role and the target user must belong to the caller's tenant

Authorizing the *caller* says nothing about the *operands*. A tenant-A `super_admin` passing
tenant-B's `role_id` and `user_id` would still succeed. So the service layer validates both.

The tools exist and are already tenant-scoped:

- `GetRoleByID(ctx, tenantID, roleID)` — `WHERE tenant_id = $1 AND id = $2`; a foreign role
  returns `ErrRoleNotFound`.
- `GetUser(ctx, tenantID, userID)` — fetches by primary key, then post-filters
  `row.TenantID != tenantID` → `ErrUserNotFound`.

Both surface as `codes.NotFound`, which is also the right answer for an attacker: it does not
confirm that the foreign role or user exists.

| RPC | authority | role's tenant | target user's tenant |
|---|---|---|---|
| `AssignRole` | add | **add** | **add** |
| `AssignProjectRole` | add | already (incidental) | **add** |
| `RevokeRole` | add | **add** | **add** |
| `InviteUser` | add | **add** (the invitation's `role_id`) | n/a — the user does not exist yet |

`Service.AssignRole` stops ignoring its `tenantID`. `Service.RevokeRole` gains one — its
request has no `tenant_id` field, so the handler passes `TenantFromRequest(ctx, "")`, which
returns the caller's tenant from the verified token. **No proto change**, so `buf breaking`
stays clean.

## 5. `AcceptInvitation`

It runs with **no caller** (it is on `PublicMethods` — the invitee has no token yet), so it
cannot be gated. The privileged decision is upstream, at `InviteUser`, which is now gated and
validates the role's tenant. That is where it belongs: whoever creates the invitation decides
the role, and they must hold `user:manage`.

Two fixes here, both small:

- **Stop discarding the error.** `_ = s.repo.AssignRole(...)` means a failed grant produces a
  user with no role and a successful-looking response. Propagate it.
- **Re-validate the role at accept time.** The invitation may be seven days old; the role could
  have been deleted, or the tenant purged. `GetRoleByID(ctx, inv.TenantID, *inv.RoleID)` before
  assigning. If it is gone, fail the acceptance rather than silently creating a role-less user.

## 6. What this does not change

`super_admin` may still grant `super_admin`. That is administration.

The gate authorizes the caller; it does not constrain *which* role may be granted. Adding a
subset rule ("you may only grant permissions you hold") is defensible but unnecessary while
`user:manage` implies `super_admin`, and it would be the wrong shape once custom roles exist —
by then the rule needs to be about the role's permissions, not the caller's role name. Deferred.

## 7. Testing

The suite currently **encodes the vulnerability as intended behaviour**:

```go
func TestHandler_AssignRole_Success(t *testing.T) {
	resp, err := newHandler().AssignRole(memberCtx(tid), &iamv1.AssignRoleRequest{...})
	require.NoError(t, err)
```

`memberCtx` holds only `RoleMember`. That test asserts a bare member can assign an arbitrary
role. It must flip to `PermissionDenied`, and the happy path must be re-established with a
caller whose `getUserPermissionsFn` returns `user:manage`.

`RevokeRole`'s happy path uses `context.Background()` — no caller at all — and expects success.

New tests, per RPC:

- **a member is denied** (`PermissionDenied`), and **the repository is never reached** — install
  an fn-field whose body calls `t.Fatal`. The mocks' unset fn-fields return benign defaults and
  do not panic, so the assertion requires the closure.
- **a `user:manage` holder succeeds.**
- **a foreign `role_id` is refused** (`NotFound`), repository write never reached.
- **a foreign `user_id` is refused** (`NotFound`), repository write never reached.
- `InviteUser` with a foreign `role_id` → `NotFound`.
- `AcceptInvitation` propagates a failed role assignment rather than returning a token.
- **No caller in context → `Unauthenticated`**, not a `uuid.Nil` actor.

Each must be shown to fail against the pre-change handler. Coverage: `iam` ≥ 85%.

## 8. Follow-ups, recorded not fixed

- **`GetUserPermissions`'s SQL has no `tenant_id` filter.** It joins `user_roles → roles` on
  `ur.user_id` alone. Correct today because a user's `user_roles` only reference their own
  tenant's roles — an invariant this change now enforces at the write path, but which the read
  path still assumes rather than checks.
- **No subset rule.** Nothing prevents a future custom-role feature from reopening escalation.
- **`iam.CheckPermission` (the RPC) is on `PublicMethods`** and takes a `user_id` with no tenant
  field, so a caller in tenant A can probe whether a user in tenant B holds a permission. Same
  class as the holes closed here, but it needs the RPC to leave the allowlist first — possible
  now that #138's `ForwardAuthUnaryClientInterceptor` gives every internal caller a token.
  Tracked in #139.
- ~100 RPCs across the platform still enforce no permission check. This slice closes role
  management only.

## 9. Blast radius

`services/iam/handler.go`, `services/iam/service.go`, and their tests. `Service.RevokeRole`
gains a `tenantID` parameter — a service-layer signature change, so every caller must be found
(`grep -rn 'RevokeRole'`). No migration. No proto change. No repository change.

Per CLAUDE.md this is `iam`/security: senior review, two approvals.
