# iam Completion Implementation Plan (#139 slice B)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close a cross-tenant read in `GetTenant`, stop `UpdateUser` from silently wiping a user's `status`, and gate the three iam write RPCs on `user:manage`.

**Architecture:** Three independent changes in `services/iam`, each small. `GetTenant` gains the `TenantFromRequest` derivation every other tenant-scoped RPC already uses. The three writes gain the existing in-process `h.requireUserManage(ctx)` helper. The `UpdateUser` SQL stops clobbering `status` with an empty string. **No interface changes, no signature changes, no sqlc regeneration** — the affected SQL is a raw inline string.

**Tech Stack:** Go 1.25, pgx/v5, grpc-go, testify, `github.com/google/uuid`.

**Spec:** `docs/superpowers/specs/2026-07-23-iam-completion-139b-design.md` (committed on this branch at `6e89400`). Read §2 for why the read RPCs are deliberately unchanged.

## Global Constraints

- **Branch:** `fix/iam-completion-139b`, already created, base `e1871c5` (`main`).
- **NO Docker, NO database.** NEVER run `docker compose` with `-v`, `down`, or `up` against `infra/local/` — that compose is project-scoped and `-v` deletes ALL local volumes; it destroyed unrelated MinIO dev data once. Use `pkg/testdb` (integration tests SKIP without `THITTAM_TEST_DSN`) or a uniquely-named throwaway container. CI's real-Postgres job is the authoritative gate. **This binds delegated subagents — state it in their instructions.**
- **Whole-tree `go vet ./...` is the completion gate for every task.** `iam.Repository` has three implementers: `db.Postgres`, `mockRepo` (`services/iam/service_test.go`), and a hidden double `iamRepo` in `e2e/critical_path/helpers_test.go` — a different tree that `go build` and package-scoped tests both miss. **This plan changes no interface, so no implementer should need editing — if one does, something is wrong; stop and report.**
- **Do NOT run `make generate-sqlc`.** Nothing in this plan touches `queries.sql`. The `UpdateUser` statement is a raw inline `const q` in `postgres.go`. Running codegen would dirty `services/billing/` with unrelated pre-existing drift (issue #160).
- **NEVER `git add -A`.** Use the scoped `git add` in each task's commit step.
- **iam must NOT use `interceptor.RequirePermission`** — it would make iam dial itself, and after #138 a nil checker makes that path return `Internal` for every gated RPC. Gate in-process with `h.requireUserManage(ctx)` (`services/iam/handler.go:241`).
- **Guard order:** tenant (`TenantFromRequest`) → permission (`requireUserManage`) → `uuid.Parse` → service call. This matches the four RPCs that already use the helper. The order is load-bearing: it is why the `_InvalidTenantID` tests do not flip and why `_InvalidID` does.
- NO migration. NO proto change (`git diff --stat gen/` must be empty). NO new permission string. NO `systemRoles` edit — therefore no D10 backfill.
- `errcheck` runs in CI; `golangci-lint` is NOT installed locally.
- `gofmt -l services/iam` flags `service.go` and `lifecycle_test.go` on a clean `main`. **Pre-existing — do not reformat them.** CI's Lint does not gate on gofmt.
- Coverage on `services/iam` must not regress. **Baseline 87.2%**; the tier floor for iam is 85%.
- Structured logging via `slog`; no PII or secrets in logs.

### The mock trap in `services/iam` (inverted vs other services)

`mockRepo`'s unset fn-fields do **NOT** uniformly return benign zero values. `getUserFn` and `getRoleByIDFn` return objects **in the caller's tenant**, so an ownership-denial test that forgets to stub them passes **vacuously**. Unset *write* fns return `nil` and never panic.

Therefore **"the repository was never reached" is proven only by a fn-field whose body calls `t.Fatal`** — never by a status code alone. `grpcError` maps several distinct errors onto the same gRPC codes.

`getUserPermissionsFn` returns `nil, nil` when unstubbed (`service_test.go:282`), so `Service.CheckPermission` answers `false` and `requireUserManage` denies. That is what makes the flip predictions below deterministic.

### Test helpers that already exist

In `services/iam/handler_test.go`:
- `platformAdminCtx()` — caller with `RolePlatformAdmin`
- `memberCtx(tid uuid.UUID)` — caller in tenant `tid`, role `member`, random `UserID`
- `memberCtxAs(tid, uid uuid.UUID)` — same, with a named `UserID`
- `newHandler()` — handler over an empty `mockRepo`
- `newHandlerWithRepo(r *mockRepo)` — handler over a supplied `mockRepo`

Relevant `mockRepo` fn-fields (all already declared in `services/iam/service_test.go`): `createUserFn`, `updateUserFn`, `getTenantFn`, `getUserPermissionsFn`.

---

### Task 1: `GetTenant` — close the cross-tenant read

Any authenticated user can currently read any tenant's record by UUID: name, slug, plan, status, full billing address, currency, and the `#92` lifecycle timestamps. The handler derives no tenant and checks no caller.

**Files:**
- Modify: `services/iam/handler.go` (the `GetTenant` handler, around line 423)
- Test: `services/iam/handler_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: no signature changes. `Service.GetTenant(ctx, id uuid.UUID) (*Tenant, error)` is unchanged.

- [ ] **Step 1: Record the coverage baseline**

```bash
go test ./services/iam/ -cover -count=1 2>&1 | tail -1
```

Expected: `coverage: 87.2% of statements`. Record the exact figure in the task report; compare at Step 7.

- [ ] **Step 2: Write the failing tests**

Add to `services/iam/handler_test.go`. The `t.Fatal` in `getTenantFn` is what gives the denial test teeth — `mockRepo`'s unset `getTenantFn` would otherwise return a usable tenant and the test could pass against the vulnerable handler.

```go
func TestHandler_GetTenant_CrossTenantDenied(t *testing.T) {
	t.Parallel()
	callerTenant := uuid.New()
	victimTenant := uuid.New()

	h := newHandlerWithRepo(&mockRepo{
		getTenantFn: func(_ context.Context, _ uuid.UUID) (*Tenant, error) {
			t.Fatal("repository reached: GetTenant must refuse a foreign tenant id before querying")
			return nil, nil
		},
	})

	_, err := h.GetTenant(memberCtx(callerTenant), &iamv1.GetTenantRequest{
		Id: victimTenant.String(),
	})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_GetTenant_OwnTenantSucceeds(t *testing.T) {
	t.Parallel()
	callerTenant := uuid.New()
	var got uuid.UUID

	h := newHandlerWithRepo(&mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			got = id
			return &Tenant{ID: id, Name: "Acme", Status: "active"}, nil
		},
	})

	out, err := h.GetTenant(memberCtx(callerTenant), &iamv1.GetTenantRequest{
		Id: callerTenant.String(),
	})

	require.NoError(t, err)
	require.Equal(t, callerTenant, got, "must query the caller's own tenant")
	require.Equal(t, callerTenant.String(), out.GetId())
}

func TestHandler_GetTenant_EmptyIDUsesCallerTenant(t *testing.T) {
	t.Parallel()
	callerTenant := uuid.New()
	var got uuid.UUID

	h := newHandlerWithRepo(&mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			got = id
			return &Tenant{ID: id, Name: "Acme", Status: "active"}, nil
		},
	})

	// TenantFromRequest falls back to the caller's tenant when the field is empty.
	_, err := h.GetTenant(memberCtx(callerTenant), &iamv1.GetTenantRequest{Id: ""})

	require.NoError(t, err)
	require.Equal(t, callerTenant, got)
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./services/iam/ -run 'GetTenant_CrossTenantDenied|GetTenant_EmptyIDUsesCallerTenant' -v`

Expected: **`TestHandler_GetTenant_CrossTenantDenied` FAILS** — the current handler parses `req.Id` and calls the repository, so the `t.Fatal` fires. `TestHandler_GetTenant_EmptyIDUsesCallerTenant` also FAILS, because `uuid.Parse("")` returns `InvalidArgument` today.

This is the teeth check for this task: the denial test must fail against the vulnerable handler. Record that it did.

- [ ] **Step 4: Fix the handler**

In `services/iam/handler.go`, replace the `GetTenant` body:

```go
func (h *Handler) GetTenant(ctx context.Context, req *iamv1.GetTenantRequest) (*iamv1.Tenant, error) {
	// The id IS the tenant id: derive it from the caller's verified token rather
	// than the request body. #144 scoped itself to fields named tenant_id and so
	// missed this one, leaving any authenticated user able to read any tenant's
	// record by UUID (#139 slice B). TenantFromRequest returns the caller's tenant
	// when the field is empty and PermissionDenied when it differs.
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	tenant, err := h.svc.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, grpcError(err)
	}
	return tenantToProto(tenant), nil
}
```

The `uuid.Parse` is deliberately gone: `TenantFromRequest` performs the parse and returns `InvalidArgument` on a malformed value, so keeping a second one would be dead code.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./services/iam/ -run GetTenant -v`
Expected: all three new tests PASS.

- [ ] **Step 6: Check for flipped pre-existing tests**

```bash
go test ./services/iam/ -count=1 2>&1 | grep -E '^\s+--- FAIL' | sort
```

**Prediction: zero flips.** No existing test in `handler_test.go` calls `GetTenant` (verify with `grep -c 'h\.GetTenant(\|\.GetTenant(ctx' services/iam/handler_test.go`). **If any test fails, STOP and report** rather than repairing it — a surprise means this reading was wrong.

- [ ] **Step 7: Run the full check**

```bash
go vet ./...
go test ./services/iam/ -race -cover -count=1
git diff --stat gen/    # must be empty
```

Expected: vet clean, tests pass, coverage ≥ the Step 1 baseline.

- [ ] **Step 8: Commit**

```bash
git add services/iam/handler.go services/iam/handler_test.go
git commit -m "fix(iam): GetTenant took its tenant from the request body (#139)

Any authenticated user could read any tenant's record by UUID -- name,
slug, plan, status, full billing address, currency, and the #92
lifecycle timestamps. The handler derived no tenant and checked no
caller.

It survived #144 for a mechanical reason: #144 scoped itself to fields
named tenant_id, and this message's field is named id. The field IS the
tenant id; only the name differs.

Derives it from the caller's verified token via TenantFromRequest, which
returns the caller's tenant when the field is empty and PermissionDenied
when it differs -- so a client asking for its own tenant keeps working
and one asking for another's is refused rather than silently redirected.

Every other GetTenant call site is service- or repository-layer and
bypasses the handler, so no internal caller is affected.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Gate the three write RPCs on `user:manage`

`CreateUser`, `UpdateUser` and `SetTenantAddress` are tenant-bounded but enforce no permission. The read RPCs are deliberately left alone — see the spec §2.1; they are AUTH by decision D3 and already tenant-scoped.

**Files:**
- Modify: `services/iam/handler.go` (`CreateUser` ~line 127, `UpdateUser` ~line 176, `SetTenantAddress` ~line 402)
- Test: `services/iam/handler_test.go`

**Interfaces:**
- Consumes: nothing from Task 1 (different handler).
- Produces: no signature changes.

- [ ] **Step 1: Write the failing denial tests**

Add to `services/iam/handler_test.go`. Each `t.Fatal` sits on the **write** fn the gated path must never reach.

```go
func TestHandler_CreateUser_RequiresUserManage(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := newHandlerWithRepo(&mockRepo{
		createUserFn: func(_ context.Context, _ *User) error {
			t.Fatal("repository reached: CreateUser must deny before writing")
			return nil
		},
	})

	_, err := h.CreateUser(memberCtx(tid), &iamv1.CreateUserRequest{
		TenantId:    tid.String(),
		Email:       "new@example.com",
		DisplayName: "New User",
		Password:    "correct-horse-battery-staple",
	})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_UpdateUser_RequiresUserManage(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := newHandlerWithRepo(&mockRepo{
		updateUserFn: func(_ context.Context, _ *User) error {
			t.Fatal("repository reached: UpdateUser must deny before writing")
			return nil
		},
	})

	_, err := h.UpdateUser(memberCtx(tid), &iamv1.UpdateUserRequest{
		TenantId:    tid.String(),
		Id:          uuid.New().String(),
		DisplayName: "Renamed",
	})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_SetTenantAddress_RequiresUserManage(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := newHandlerWithRepo(&mockRepo{
		updateTenantAddressFn: func(_ context.Context, _ *Tenant) (*Tenant, error) {
			t.Fatal("repository reached: SetTenantAddress must deny before writing")
			return nil, nil
		},
	})

	_, err := h.SetTenantAddress(memberCtx(tid), &iamv1.SetTenantAddressRequest{
		TenantId:     tid.String(),
		AddressLine1: "1 Main St",
		City:         "Chennai",
		CountryCode:  "IN",
	})

	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}
```

Note the address field is named `updateTenantAddressFn` (not `setTenantAddressFn`) and returns `(*Tenant, error)`, not `error` — `Service.SetTenantAddress` calls `s.repo.UpdateTenantAddress`. All four fn-fields used in this task are confirmed present in `services/iam/service_test.go`: `createUserFn:38`, `updateUserFn:41`, `updateTenantAddressFn:51`, `getUserPermissionsFn:67`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./services/iam/ -run RequiresUserManage -v`
Expected: all three FAIL — the handlers have no gate, so each `t.Fatal` fires.

- [ ] **Step 3: Add the gate to `CreateUser`**

In `services/iam/handler.go`, insert immediately after the existing `TenantFromRequest` block:

```go
	if err := h.requireUserManage(ctx); err != nil {
		return nil, err
	}
```

The resulting `CreateUser` opens:

```go
func (h *Handler) CreateUser(ctx context.Context, req *iamv1.CreateUserRequest) (*iamv1.User, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	if err := h.requireUserManage(ctx); err != nil {
		return nil, err
	}
	user := &User{
```

- [ ] **Step 4: Add the gate to `UpdateUser`**

Same insertion, immediately after `TenantFromRequest` and **before** the `uuid.Parse(req.GetId())`:

```go
func (h *Handler) UpdateUser(ctx context.Context, req *iamv1.UpdateUserRequest) (*iamv1.User, error) {
	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	if err := h.requireUserManage(ctx); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.GetId())
```

Placing it before the parse is deliberate and matches the four RPCs that already use the helper. It is why `TestHandler_UpdateUser_InvalidID` flips in Step 6.

- [ ] **Step 5: Add the gate to `SetTenantAddress`**

Same insertion, immediately after `TenantFromRequest`:

```go
func (h *Handler) SetTenantAddress(ctx context.Context, req *iamv1.SetTenantAddressRequest) (*iamv1.Tenant, error) {
	id, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	if err := h.requireUserManage(ctx); err != nil {
		return nil, err
	}
	t := &Tenant{
```

- [ ] **Step 6: Repair exactly five flipped tests**

`mockRepo.GetUserPermissions` returns `nil, nil` unstubbed, so `requireUserManage` denies. Run:

```bash
go test ./services/iam/ -count=1 2>&1 | grep -E '^\s+--- FAIL' | sort
```

**Prediction — exactly these five, and no others:**

| test | why it flips |
|---|---|
| `TestHandler_CreateUser_Success` | reaches the gate |
| `TestHandler_UpdateUser_Success` | reaches the gate |
| `TestHandler_UpdateUser_InvalidID` | gate precedes the `uuid.Parse` |
| `TestHandler_SetTenantAddress_Success` | reaches the gate |
| `TestHandler_SetTenantAddress_MissingCountry` | reaches the gate |

These four must **not** flip, because `TenantFromRequest` refuses them first: `TestHandler_CreateUser_InvalidTenantID`, `TestHandler_UpdateUser_InvalidTenantID`, `TestHandler_SetTenantAddress_InvalidTenantID`, `TestCreateUser_CrossTenant_Denied`.

**If the count is not exactly 5, STOP and report.** Slice C predicted 3 flips and got 5; the difference was benign but was only known to be benign because it was investigated.

Repair each by granting the caller the permission — add to that test's `mockRepo`:

```go
		getUserPermissionsFn: func(_ context.Context, _ uuid.UUID, _ *uuid.UUID) ([]string, error) {
			return []string{"user:manage"}, nil
		},
```

**Do not weaken any assertion.** `TestHandler_UpdateUser_InvalidID` must still assert `codes.InvalidArgument`, which it reaches once the caller holds the permission. `TestHandler_SetTenantAddress_MissingCountry` must still assert whatever it asserted before.

- [ ] **Step 7: Run the full check**

```bash
go vet ./...
go test ./services/iam/ -race -cover -count=1
grep -c 'h.requireUserManage(ctx)' services/iam/handler.go   # must be 7
git diff --stat gen/                                          # must be empty
```

Expected: vet clean, tests pass, coverage ≥ baseline. The helper count goes from 4 to **7** (the four pre-existing role-management RPCs plus the three added here).

- [ ] **Step 8: Commit**

```bash
git add services/iam/handler.go services/iam/handler_test.go
git commit -m "fix(iam): gate CreateUser, UpdateUser and SetTenantAddress on user:manage (#139)

Three write RPCs were tenant-bounded but enforced no permission, so any
authenticated member could create users, rename or re-status a
colleague, and rewrite the tenant's billing address.

Gated in-process via h.requireUserManage, not interceptor
.RequirePermission -- iam holding a gRPC PermissionChecker would mean
iam dialling itself, and after #138 a nil checker makes that path return
Internal for every gated RPC.

The read RPCs (GetUser, ListUsers, ListRoles, GetTenant, Logout) are
deliberately left AUTH per decision D3: user:manage is the only
user-related permission that exists and only super_admin holds it, so
gating reads on it would stop a manager listing their own team. Adding a
user:read string would require a systemRoles edit, which reaches new
tenants only and would pull in the D10 cross-schema backfill.

Guard order is tenant -> permission -> parse, matching the four RPCs
that already use the helper.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Stop `UpdateUser` wiping a user's `status`

`Postgres.UpdateUser` writes `status` unconditionally. `status` is security-critical — `pkg/auth/local.go:84-89` refuses login for `deactivated` and `invited` accounts. A client updating only a display name sends `status: ""`, which sets the column to the empty string; the login switch then matches neither case and **a deactivated account becomes loginable**.

Task 2's gate does not fix this: a legitimate `user:manage` holder editing a display name would still wipe the status.

**Files:**
- Modify: `services/iam/db/postgres.go` (the `UpdateUser` method, around line 305)
- Test: `services/iam/db/postgres_test.go` if a unit-testable seam exists; otherwise `services/iam/handler_test.go` — see Step 2.

**Interfaces:**
- Consumes: Task 2's gate exists, so any handler-level test in this task must supply a caller holding `user:manage` via `getUserPermissionsFn`.
- Produces: no signature changes. `Repository.UpdateUser(ctx, u *iam.User) error` is unchanged.

- [ ] **Step 1: Read the current implementation**

```bash
sed -n '/func (p \*Postgres) UpdateUser(/,/^}/p' services/iam/db/postgres.go
```

It is a raw inline statement, not sqlc:

```go
const q = `UPDATE users SET display_name = $2, status = $3 WHERE id = $1 AND tenant_id = $4`
```

**Do not run `make generate-sqlc`** — this statement is not in `queries.sql` and codegen would only dirty `services/billing/` (issue #160).

- [ ] **Step 2: Write the failing test**

The behaviour lives in SQL, so a `mockRepo`-based handler test cannot observe it — the mock never runs the statement. Decide the seam and say which you chose in the task report:

**Preferred — a handler-level test asserting the value passed to the repository.** This proves the handler does not *intend* to blank the status, which is the half that unit tests can reach:

```go
func TestHandler_UpdateUser_EmptyStatusIsNotAWipe(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	var got *User

	h := newHandlerWithRepo(&mockRepo{
		getUserPermissionsFn: func(_ context.Context, _ uuid.UUID, _ *uuid.UUID) ([]string, error) {
			return []string{"user:manage"}, nil
		},
		updateUserFn: func(_ context.Context, u *User) error {
			got = u
			return nil
		},
	})

	_, err := h.UpdateUser(memberCtx(tid), &iamv1.UpdateUserRequest{
		TenantId:    tid.String(),
		Id:          uuid.New().String(),
		DisplayName: "Renamed",
		// Status deliberately omitted — a profile edit must not touch it.
	})

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Empty(t, got.Status,
		"handler must pass through an empty status; the SQL decides to preserve it")
}
```

**Required — an integration test proving the column survives.** Create `services/iam/db/user_status_preserve_integration_test.go`. It **must** open with the build tag on its own line, followed by a blank line, matching every other integration test in the package:

```go
//go:build integration

package db_test
```

Read `services/iam/db/tenant_find_by_name_integration_test.go` first and copy its setup exactly. The package convention is `testdb.Open(t)` for the pool and `testdb.NewTx(t, pool)` where a rollback is wanted; `testdb.Open` skips the test when `THITTAM_TEST_DSN` is unset. Note that package's constructor caveat: `NewPostgres` accepts only a `*pgxpool.Pool`, so tests that need a transaction exercise `iamdb.New(tx)` directly. Since this test must exercise `Postgres.UpdateUser` — the wrapper, where the fixed statement lives — use the pool form and clean up the row it inserts.

The test: insert a user with `status = 'deactivated'`, call `Postgres.UpdateUser` with `Status: ""` and a changed `DisplayName`, read the row back, assert `status` is still `deactivated` **and** that `display_name` did change (proving the update ran rather than silently matching zero rows).

- [ ] **Step 3: Establish what can and cannot be proven locally**

**Integration tests carry `//go:build integration`, so they are not compiled into the default test binary at all** — `go test ./services/iam/db/` will not run or even build the new file. To compile it locally:

```bash
go vet -tags=integration ./services/iam/db/
go test -tags=integration ./services/iam/db/ -run UserStatusPreserve -v
```

Without `THITTAM_TEST_DSN` the second command SKIPs. **That is the expected local outcome. Do not set up a database, do not fabricate a failure, and do not report a SKIP as a pass.** Record in the task report that the fix's proof is deferred to CI's real-Postgres job, and that the local run was a SKIP with the tag and a build check with `go vet -tags=integration`.

The `go vet -tags=integration` run is not optional — it is the only local signal that the new file compiles. A test file excluded by a build tag can contain a compile error and every default-tag command will still pass.

Run the handler test now: `go test ./services/iam/ -run EmptyStatusIsNotAWipe -v` — expected PASS both before and after the SQL change, since it asserts handler pass-through, not SQL behaviour.

- [ ] **Step 4: Fix the SQL**

In `services/iam/db/postgres.go`:

```go
func (p *Postgres) UpdateUser(ctx context.Context, u *iam.User) error {
	// An empty status means "leave it alone", not "clear it". status is
	// security-critical: pkg/auth/local.go refuses login for 'deactivated' and
	// 'invited'. Before this guard, a client updating only a display name sent
	// status: "" and silently reactivated a deactivated account (#139 slice B).
	const q = `UPDATE users SET display_name = $2, status = COALESCE(NULLIF($3, ''), status) WHERE id = $1 AND tenant_id = $4`
	tag, err := p.db.Exec(ctx, q, u.ID, u.DisplayName, u.Status, u.TenantID)
	if err != nil {
		return fmt.Errorf("iam/db: update user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return iam.ErrUserNotFound
	}
	return nil
}
```

`NULLIF($3, '')` turns an empty string into NULL; `COALESCE(..., status)` then falls back to the existing column value. A caller that genuinely wants to change status still sends one.

Preserve the `RowsAffected() == 0 → ErrUserNotFound` check exactly — it is what makes a cross-tenant update return `NotFound` rather than silently affecting zero rows.

- [ ] **Step 5: Verify**

```bash
go vet ./...
go vet -tags=integration ./services/iam/db/     # proves the tagged file compiles
go test ./services/iam/... -race -cover -count=1
```

Expected: both vet runs clean, all default-tag tests pass, coverage ≥ baseline. The integration test does not appear in the third command's output at all — it is excluded by its build tag, not skipped.

- [ ] **Step 6: Commit**

```bash
git add services/iam/db/postgres.go services/iam/handler_test.go services/iam/db/
git commit -m "fix(iam): an empty status in UpdateUser no longer wipes the column (#139)

Postgres.UpdateUser wrote status unconditionally with no COALESCE. status
is security-critical: pkg/auth/local.go refuses login for 'deactivated'
and 'invited' accounts.

So a client updating only a display name sent status: \"\", the column
became the empty string, and the login switch matched neither case -- a
deactivated account became loginable again through an ordinary profile
edit. DeactivateUser is gated on platform_admin, the strictest control in
the service, and an ordinary field update undid it. That is the #146
shape: a strict gate rendered decorative by a sibling writing the same
column.

Gating the RPC (previous commit) does not close this half: a legitimate
user:manage holder editing a display name would still wipe the status.

NULLIF turns an empty status into NULL and COALESCE falls back to the
stored value, so an omitted status now means 'leave it alone'. A caller
that wants to change it still sends one.

Validating status against an allow-list is the better long-term shape but
is a behaviour change for clients sending unrecognised values, and the
legal set is not declared in one place today. Out of scope; NULLIF closes
the live hole without guessing at the vocabulary.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Whole-branch verification

Run after all three tasks, before opening the PR.

- [ ] **Step 1: Build and vet**

```bash
go vet ./...
go build ./cmd/...
```

- [ ] **Step 2: Full suite**

```bash
go test ./... -short -count=1
go vet -tags=integration ./services/iam/db/
go test ./services/iam/... -race -count=1
```

- [ ] **Step 3: Confirm the intended end state**

```bash
grep -c 'h.requireUserManage(ctx)' services/iam/handler.go        # 7 (4 pre-existing + 3 new)
grep -n 'TenantFromRequest(ctx, req.GetId())' services/iam/handler.go   # GetTenant
grep -c "NULLIF(\$3, '')" services/iam/db/postgres.go             # 1
grep -c 'interceptor.RequirePermission' services/iam/handler.go   # 0 — iam must never use it
```

- [ ] **Step 4: Constraints**

```bash
git diff --stat gen/          # must be empty
git diff --stat migrations/   # must be empty
git status --short            # clean; services/billing/ must NOT appear
```

- [ ] **Step 5: Coverage**

```bash
go test ./services/iam/ -cover -count=1
```

Must be ≥ 87.2% (baseline) and ≥ 85% (iam tier floor).

- [ ] **Step 6: Push and open the PR**

```bash
git push -u origin fix/iam-completion-139b
```

The PR body must state: closes #139 slice B; two live defects fixed (`GetTenant` cross-tenant read, `UpdateUser` status wipe) plus three permission gates; reads deliberately left AUTH per D3 with the reasoning; no migration, no proto change, no new permission string, **no D10 backfill**; `DeactivateUser` deliberately unchanged (D5) because changing it would widen access. Flag for senior review — security change touching `iam`, needs 2 approvals.

- [ ] **Step 7: Confirm CI**

```bash
gh pr checks <number>
```

**Local green is not CI green.** The `UpdateUser` status fix is proven only by the real-Postgres integration job — locally that test SKIPs. Do not declare the PR ready until that job passes.
