# Enforce the Tenant Boundary — Implementation Plan (#144)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop 61 RPCs from reading `tenant_id` out of the request body, so an authenticated user of tenant A can no longer read or modify tenant B's data.

**Architecture:** One helper, `interceptor.TenantFromRequest(ctx, reqTenantID) (uuid.UUID, error)`, returns the tenant from the caller's **verified token** and refuses a mismatched request tenant. Handlers in `ledger`, `billing`, `document`, `notifications` adopt it wholesale. `iam` splits three ways: platform RPCs keep the request tenant behind `RequireRole(platform_admin)`; `CreateTenant` gains that gate; five tenant-scoped RPCs adopt the helper. Service and repository layers are untouched.

**Tech Stack:** Go 1.22+, gRPC, `pkg/interceptor`, `pkg/tenant`, testify.

**Spec:** `docs/superpowers/specs/2026-07-09-tenant-boundary-144-design.md`

## Global Constraints

- **This is a security change.** Per CLAUDE.md: senior review, 2 approvals (`iam`/security). Every task must leave the tree building and every test passing.
- **Whole-tree `go vet ./...` is the gate.** `go build ./services/...` misses the ten `cmd/*` wirings and the e2e doubles.
- **errcheck runs in CI; golangci-lint is NOT installed here.** Check every error return. Note: errcheck's default excludes cover `fmt.Fprint*` to `os.Stdout`/`os.Stderr` but **not** to an arbitrary `io.Writer`.
- **`gh pr checks` is the gate, not local green.** `main` was red for a day because six PRs were merged on local verification alone.
- **No database, no Docker, no service startup.** NEVER run `docker compose … -v` / `down` / `up` against `infra/local/` — project-scoped; `-v` deletes ALL local volumes (it once destroyed unrelated MinIO dev data). Do NOT execute `scripts/dev-start.sh`.
- **Never log a token, a key, or caller metadata.**
- **The returned tenant NEVER comes from the request.** If you find yourself writing `uuid.Parse(req.GetTenantId())` in a handler covered by this plan, stop.
- **Coverage:** `iam` ≥ 85%; `pkg/` ≥ 75%; the four service packages must not regress. Report before/after.
- **Commits:** Conventional Commits, scope `iam` for `pkg/interceptor`, else the service's scope (`ledger`, `billing`, `document`, `notifications`, `proto`).

## The two traps this plan exists to avoid

### Trap 1: `Login` must never call the helper

`Login` reads `req.GetTenantId()` and is on `interceptor.PublicMethods`. It runs with **no caller in context** — that is the entire point; the caller has no token yet. Applying `TenantFromRequest` to it returns `Unauthenticated` for every login attempt, forever, and it would look perfectly consistent with its fifteen neighbours.

`Login`'s `tenant_id` is a *lookup key* (which tenant's user directory to search), not an authorization claim. It stays exactly as it is.

Any handler on `PublicMethods` — `Login`, `RefreshToken`, `AcceptInvitation`, `CheckPermission`, `ValidateToken` — is out of bounds for this change.

### Trap 2: the test that proves nothing

Every existing handler test in these five services calls `context.Background()` — **no caller, no tenant**. After the change they all get `Unauthenticated`. There are **182 such call sites** (ledger 31, billing 26, document 50, notifications 22, iam 53).

Fixing them means injecting a caller. The failure mode is doing it like this:

```go
// WRONG — the request tenant and the token tenant are the same variable, so a
// handler that ignored the token entirely would still pass.
tid := uuid.New()
ctx := interceptor.WithCaller(context.Background(), interceptor.CallerInfo{TenantID: tid})
h.ListInvoices(ctx, &billingv1.ListInvoicesRequest{TenantId: tid.String()})
```

That test passes against the *unfixed* handler. It proves nothing.

Each service therefore gets **one helper that returns both the context and the tenant**, and a **cross-tenant test that deliberately uses a different id**:

```go
// callerCtx returns a context carrying a verified caller in tenant tid.
func callerCtx(tid uuid.UUID) context.Context {
	return interceptor.WithCaller(context.Background(), interceptor.CallerInfo{
		UserID:   uuid.New(),
		TenantID: tid,
		Email:    "user@example.com",
		Roles:    []string{"member"},
	})
}
```

## File Structure

| File | Responsibility |
|---|---|
| `pkg/interceptor/tenant.go` | new — `TenantFromRequest` |
| `pkg/interceptor/tenant_test.go` | new — the six-row table, incl. token A + request B → `PermissionDenied` |
| `services/ledger/handler.go` | 12 handlers adopt the helper |
| `services/billing/handler.go` | 12 |
| `services/document/handler.go` | 13 |
| `services/notifications/handler.go` | 8 |
| `services/iam/handler.go` | three-way split; `CreateTenant` gate; caller-sourced `assigned_by`/`invited_by` |
| `services/*/handler_test.go` (×5) | caller contexts + one cross-tenant denial test each |
| `proto/thittam/*/v1/*.proto` | deprecate-and-ignore comments on `tenant_id` |

**Execution order.** Task 1 is additive (nothing calls it). Tasks 2–5 are one service each, independent, each self-contained: handler + its tests in the same commit, so the tree is never red. Task 6 is iam, last, because it needs judgement rather than mechanics. Task 7 is proto comments.

---

### Task 1: `interceptor.TenantFromRequest`

**Files:**
- Create: `pkg/interceptor/tenant.go`
- Create: `pkg/interceptor/tenant_test.go`

**Interfaces:**
- Consumes: `CallerInfo`, `CallerFromContext` (`pkg/interceptor/auth.go`).
- Produces, relied on by Tasks 2–6:
  ```go
  func TenantFromRequest(ctx context.Context, reqTenantID string) (uuid.UUID, error)
  ```

**Why a helper that returns the tenant, rather than a `RequireTenant(ctx, id) error`.** `RequirePermission` returns only an error, so a handler can simply never call it — and didn't, behind `if h.perm != nil`, at twenty call sites, for a year. `TenantFromRequest` returns *the tenant the handler needs to query with*. Skip the call and there is nothing to pass to the repository. The check is enforced by the type, not by discipline.

**It lives in `pkg/interceptor`, not `pkg/tenant`.** `pkg/tenant` is a context-key package with no gRPC dependency; returning `status.Error` from it would drag gRPC into every consumer.

**Constraint:** no database, no Docker. Pure unit tests.

- [ ] **Step 1: Write the failing table test**

Create `pkg/interceptor/tenant_test.go`:

```go
package interceptor

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTenantFromRequest(t *testing.T) {
	t.Parallel()

	tenantA := uuid.New()
	tenantB := uuid.New()

	callerIn := func(tid uuid.UUID) context.Context {
		return WithCaller(context.Background(), CallerInfo{UserID: uuid.New(), TenantID: tid})
	}

	tests := []struct {
		name    string
		ctx     context.Context
		reqID   string
		want    uuid.UUID
		wantErr codes.Code
	}{
		{"no caller in context", context.Background(), "", uuid.Nil, codes.Unauthenticated},
		{"caller with nil tenant", callerIn(uuid.Nil), "", uuid.Nil, codes.Unauthenticated},
		{"empty request tenant uses the token", callerIn(tenantA), "", tenantA, codes.OK},
		{"matching request tenant", callerIn(tenantA), tenantA.String(), tenantA, codes.OK},
		{"unparseable request tenant", callerIn(tenantA), "not-a-uuid", uuid.Nil, codes.InvalidArgument},
		{"MISMATCHED request tenant is refused", callerIn(tenantA), tenantB.String(), uuid.Nil, codes.PermissionDenied},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := TenantFromRequest(tc.ctx, tc.reqID)
			if tc.wantErr != codes.OK {
				require.Error(t, err)
				assert.Equal(t, tc.wantErr, status.Code(err))
				assert.Equal(t, uuid.Nil, got, "no tenant may be returned alongside an error")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// The returned tenant must never be the request's, even when the request names a
// tenant that happens to parse. This is the whole point of the helper.
func TestTenantFromRequest_NeverReturnsTheRequestTenant(t *testing.T) {
	t.Parallel()
	tenantA, tenantB := uuid.New(), uuid.New()
	ctx := WithCaller(context.Background(), CallerInfo{UserID: uuid.New(), TenantID: tenantA})

	got, err := TenantFromRequest(ctx, tenantB.String())
	require.Error(t, err)
	assert.NotEqual(t, tenantB, got)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./pkg/interceptor/ -run TestTenantFromRequest -v`
Expected: FAIL — `undefined: TenantFromRequest`.

- [ ] **Step 3: Implement**

Create `pkg/interceptor/tenant.go`:

```go
package interceptor

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TenantFromRequest returns the caller's tenant, taken from the verified token.
//
// If the request also carries a tenant id and it differs, the call is refused. A
// client asking about a tenant that is not its own is either broken or hostile;
// either way we would rather say so than quietly answer about a different tenant
// than the one it named.
//
// The returned tenant NEVER comes from the request. A handler cannot query with a
// caller-supplied tenant, because the only tenant it can obtain is this one.
//
// Platform RPCs that legitimately act on another tenant (SuspendTenant, PurgeTenant)
// must NOT use this helper: they read the request tenant behind RequireRole.
func TenantFromRequest(ctx context.Context, reqTenantID string) (uuid.UUID, error) {
	caller, ok := CallerFromContext(ctx)
	if !ok {
		return uuid.Nil, status.Error(codes.Unauthenticated, "caller identity not present in context")
	}
	if caller.TenantID == uuid.Nil {
		return uuid.Nil, status.Error(codes.Unauthenticated, "token carries no tenant")
	}
	if reqTenantID == "" {
		return caller.TenantID, nil
	}
	reqID, err := uuid.Parse(reqTenantID)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}
	if reqID != caller.TenantID {
		// Deliberately does not echo either id: the caller already knows its own,
		// and confirming the existence of the other is an oracle.
		return uuid.Nil, status.Error(codes.PermissionDenied, "tenant_id does not match the authenticated tenant")
	}
	return caller.TenantID, nil
}
```

- [ ] **Step 4: Run to green**

Run: `go test ./pkg/interceptor/ -v`
Expected: PASS. `pkg/interceptor` coverage must stay 100.0%.

- [ ] **Step 5: Whole-tree vet**

Run: `go vet ./...`
Expected: clean. Nothing calls the helper yet.

- [ ] **Step 6: Commit**

```bash
git add pkg/interceptor/tenant.go pkg/interceptor/tenant_test.go
git commit -m "feat(iam): TenantFromRequest — the tenant comes from the token, never the request (#144)"
```

---

### Task 2: `ledger` — 12 handlers

**Files:**
- Modify: `services/ledger/handler.go` (12 sites reading `req.GetTenantId()`)
- Modify: `services/ledger/handler_test.go` (31 `context.Background()` sites)

**Interfaces:** consumes `interceptor.TenantFromRequest` (Task 1).

**Why ledger first.** It is the sharpest edge: `CreateJournalEntry`, `PostJournalEntry`, `VoidJournalEntry`, `GetTrialBalance` are double-entry accounting, and today any authenticated user can post to any tenant's books by naming their UUID.

**Constraint:** no database, no Docker.

- [ ] **Step 1: Add the caller-context helper to the test file**

At the top of `services/ledger/handler_test.go`:

```go
// callerCtx returns a context carrying a verified caller in tenant tid, as
// UnaryAuthInterceptor would have produced from a valid token (#138).
// Handler tests bypass the interceptor, so they must inject the caller themselves.
func callerCtx(tid uuid.UUID) context.Context {
	return interceptor.WithCaller(context.Background(), interceptor.CallerInfo{
		UserID:   uuid.New(),
		TenantID: tid,
		Email:    "user@example.com",
		Roles:    []string{"member"},
	})
}
```

Import `"github.com/wegofwd2020/thittam/pkg/interceptor"`.

- [ ] **Step 2: Convert every handler**

For each of the 12 handlers in `services/ledger/handler.go` that reads the tenant:

```go
-	tenantID, err := uuid.Parse(req.GetTenantId())
-	if err != nil {
-		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
-	}
+	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
+	if err != nil {
+		return nil, err
+	}
```

Return `err` unchanged — it is already a `status.Error` with the right code. Do not wrap it.

Some handlers parse a `project_id` or `period_id` as well; leave those alone. Some may read the tenant and then use it twice; the local `tenantID` is unchanged, so nothing downstream moves. **Do not touch the service or repository layers.**

If a handler reads `req.GetTenantId()` in a way this pattern does not fit — for instance it validates the tenant before some other argument, and the test asserts on the *other* error — report it rather than reordering assertions to suit.

**Also fix the package doc comment**, `services/ledger/handler.go:17-20`. It currently reads:

> *"Tenant IDs are taken directly from the request fields, enabling calls from other services (IAM seeder, reporting) without an HTTP context."*

That is false after this change, and no such caller exists in the tree (verified: handlers are constructed only in `cmd/*/main.go`). Left in place, it invites someone to wire a caller-less internal call into a handler that will now reject it. Replace it with a sentence saying the tenant comes from the caller's verified token, and that internal callers must use the service layer.

- [ ] **Step 3: Fix the existing tests**

Every `h.Foo(context.Background(), &ledgerv1.FooRequest{TenantId: someID, ...})` becomes:

```go
	tid := uuid.New()
	resp, err := h.Foo(callerCtx(tid), &ledgerv1.FooRequest{TenantId: tid.String(), ...})
```

**The request tenant and the caller tenant must be the same variable** for the existing tests — they are testing handler logic, not the boundary. Where a test passes a hardcoded bad tenant (`TenantId: "bad"`) to assert `InvalidArgument`, keep it: the helper still returns `InvalidArgument` for an unparseable id, but it needs a caller in context first, so wrap it in `callerCtx(uuid.New())`.

- [ ] **Step 4: Write the cross-tenant denial test**

This is the test the repository has never had. It must assert **the repository was never called** — a handler that queries first and checks after returns the right error and still reads the row.

`mockRepo` uses fn-fields that return zero values when unset (`services/ledger/service_test.go:121`), so an unset field would *not* fail. Set it explicitly to a fatal:

```go
func TestHandler_CrossTenantRead_Denied(t *testing.T) {
	t.Parallel()
	callerTenant := uuid.New()
	victimTenant := uuid.New()
	require.NotEqual(t, callerTenant, victimTenant)

	h := newHandlerWithRepo(&mockRepo{
		listJournalEntriesFn: func(context.Context, uuid.UUID, *uuid.UUID, string, int, int) ([]JournalEntry, error) {
			t.Fatal("repository must not be reached on a cross-tenant request")
			return nil, nil
		},
	})

	_, err := h.ListJournalEntries(callerCtx(callerTenant), &ledgerv1.ListJournalEntriesRequest{
		TenantId: victimTenant.String(),
	})

	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}
```

Adapt `newHandlerWithRepo` / the fn-field name to whatever `services/ledger/service_test.go` actually declares — read it first.

Add a second test proving the happy path still reaches the repo with the **token's** tenant:

```go
func TestHandler_ListJournalEntries_UsesTokenTenant(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	var gotTenant uuid.UUID
	h := newHandlerWithRepo(&mockRepo{
		listJournalEntriesFn: func(_ context.Context, tenantID uuid.UUID, _ *uuid.UUID, _ string, _, _ int) ([]JournalEntry, error) {
			gotTenant = tenantID
			return nil, nil
		},
	})

	// Request carries NO tenant at all: the token supplies it.
	_, err := h.ListJournalEntries(callerCtx(tid), &ledgerv1.ListJournalEntriesRequest{})
	require.NoError(t, err)
	assert.Equal(t, tid, gotTenant, "the repository must receive the token's tenant")
}
```

- [ ] **Step 5: Verify**

Run: `go test ./services/ledger/... -v 2>&1 | tail -20`
Expected: PASS, including both new tests.

Run: `grep -c 'uuid.Parse(req.GetTenantId())' services/ledger/handler.go`
Expected: `0`.

Run: `go vet ./...`
Expected: clean.

Run: `go test ./services/ledger/ -coverprofile=/tmp/c.out && go tool cover -func=/tmp/c.out | tail -1`
Expected: no regression. Record before/after.

- [ ] **Step 6: Commit**

```bash
git add services/ledger/handler.go services/ledger/handler_test.go
git commit -m "fix(ledger): derive the tenant from the token, not the request (#144)"
```

---

### Task 3: `billing` — 12 handlers

**Files:**
- Modify: `services/billing/handler.go` (12 sites, e.g. `ListInvoices` at line 109)
- Modify: `services/billing/handler_test.go` (26 `context.Background()` sites)

Identical in shape to Task 2. `mockRepo` lives in `services/billing/service_test.go` and uses fn-fields (`getSubscriptionByTenantFn`, `listInvoicesFn`, …); `newHandlerWithRepo` already exists.

**Two billing-specific cautions.**

Billing uses **direct field access** (`req.TenantId`) and a different error style (`status.Errorf(codes.InvalidArgument, "invalid tenant_id: %v", err)`) from the other three. Convert to `interceptor.TenantFromRequest(ctx, req.TenantId)` and return `err` unchanged; the helper's message replaces the formatted one.

`DownloadInvoice` (line 147) parses the tenant once and uses it **twice**: for `h.svc.GetInvoice(...)` and again at lines 169-172, passing `TenantId: tenantID.String()` to `h.docClient.GetDownloadURL(...)`. That outbound hop carries the caller's forwarded token (#138), so document derives the *same* tenant from it. After this change `tenantID` is the token's tenant, so the outbound field agrees with the outbound token — leave the field, and say so in your report. `SetDefaultPaymentMethod` (244) likewise uses the parsed tenant twice; one parse, no change needed.

Two billing handlers read **no** tenant: `RemovePaymentMethod` and `HandlePaymentWebhook`. Leave them.

- [ ] **Step 1: Add `callerCtx` to `services/billing/handler_test.go`** (same body as Task 2, Step 1).
- [ ] **Step 2: Convert all 12 handlers** to `interceptor.TenantFromRequest(ctx, req.GetTenantId())`.
- [ ] **Step 3: Fix the 26 existing test call sites.**
- [ ] **Step 4: Cross-tenant denial test** on `ListInvoices`, asserting `listInvoicesFn` is never called (`t.Fatal` inside it) and `codes.PermissionDenied`.
- [ ] **Step 5: Happy-path test** proving the repo receives the token's tenant when the request carries none.
- [ ] **Step 6: Verify**

Run: `go test ./services/billing/... -v 2>&1 | tail -20` — PASS
Run: `grep -c 'uuid.Parse(req.TenantId)\|uuid.Parse(req.GetTenantId())' services/billing/handler.go` — `0`
Run: `go vet ./...` — clean
Coverage: no regression; record before/after.

- [ ] **Step 7: Commit**

```bash
git add services/billing/handler.go services/billing/handler_test.go
git commit -m "fix(billing): derive the tenant from the token, not the request (#144)"
```

---

### Task 4: `document` — 13 handlers

**Files:**
- Modify: `services/document/handler.go` (13 sites)
- Modify: `services/document/handler_test.go` (50 `context.Background()` sites — the largest churn in this plan)

Identical in shape to Task 2.

**Document-specific caution.** `services/document/service.go:278,375` already verify that a folder/document belongs to *the tenant from context*. Those checks stay; they now compose with a tenant that is guaranteed to be the caller's. Do not remove them — they guard a different thing (does this resource belong to this tenant?) from what the helper guards (is this tenant the caller's?).

`GetDownloadURL` is called service-to-service by billing with the original caller's forwarded token, so its context carries that caller. It needs no special case.

- [ ] **Step 1: Add `callerCtx` to `services/document/handler_test.go`.**
- [ ] **Step 2: Convert all 13 handlers.**
- [ ] **Step 3: Fix the 50 existing test call sites.** This is mechanical and long. Do not batch-`sed` it — several tests assert `InvalidArgument` on a malformed tenant and must keep a caller in context to reach that branch.
- [ ] **Step 4: Cross-tenant denial test** on `GetDocument` (or `ListDocuments`), repo fn-field set to `t.Fatal`.
- [ ] **Step 5: Happy-path test** proving the repo receives the token's tenant.
- [ ] **Step 6: Verify** — `go test ./services/document/... -v`, `grep -c` → 0, `go vet ./...`, coverage before/after.
- [ ] **Step 7: Commit**

```bash
git add services/document/handler.go services/document/handler_test.go
git commit -m "fix(document): derive the tenant from the token, not the request (#144)"
```

---

### Task 5: `notifications` — 8 handlers

**Files:**
- Modify: `services/notifications/handler.go` (8 sites)
- Modify: `services/notifications/handler_test.go` (22 `context.Background()` sites)

Identical in shape to Task 2.

**Notifications-specific caution.** `Send` and `Dispatch` are invoked by **NATS consumers**, not only by gRPC clients. Confirm those consumers call the **service layer** (`svc.Send(...)`), not the handler. I verified that `NewHandler(...)` appears only in `cmd/*/main.go`, so no consumer or worker constructs a handler — but re-check for `notifications` specifically, because a consumer reaching a handler would have no caller in context and `TenantFromRequest` would reject it. **If you find a consumer calling a handler, stop and report BLOCKED.**

- [ ] **Step 1: Add `callerCtx` to `services/notifications/handler_test.go`.**
- [ ] **Step 2: Verify no consumer calls a handler.** `grep -rn 'NewHandler(' --include=*.go . | grep -v _test | grep -v cmd/` → expect nothing. Paste the output.
- [ ] **Step 3: Convert all 8 handlers.**
- [ ] **Step 4: Fix the 22 existing test call sites.**
- [ ] **Step 5: Cross-tenant denial test** on `ListNotifications`, repo fn-field `t.Fatal`.
- [ ] **Step 6: Happy-path test.**
- [ ] **Step 7: Verify** — tests, `grep -c` → 0, `go vet ./...`, coverage.
- [ ] **Step 8: Commit**

```bash
git add services/notifications/handler.go services/notifications/handler_test.go
git commit -m "fix(notifications): derive the tenant from the token, not the request (#144)"
```

---

### Task 6: `iam` — the three-way split

**Files:**
- Modify: `services/iam/handler.go` (16 request-tenant handlers)
- Modify: `services/iam/handler_test.go` (53 `context.Background()` sites; `platformAdminCtx()` already exists and is used 31 times)

**This is the only task requiring judgement.** Sorting a tenant-scoped RPC into the platform bucket leaves the hole open while looking fixed, and no test would catch it. The classification below is verified against the code — do not re-derive it, but do read each handler before editing.

`services/iam/handler.go` has **sixteen** handlers that read `req.GetTenantId()`. Note that `SuspendTenant`, `ClearTenantLegalHold` and `SetTenantRetention` read `req.GetId()` instead, so they are **not** among the sixteen and need no change.

**(a) DO NOT TOUCH — public method.**

| Handler | Line | Why |
|---|---|---|
| `Login` | 40 | On `PublicMethods`. No caller in context, by design. Its `tenant_id` is a lookup key, not an authorization claim. Applying the helper breaks all logins. |

**(b) Platform RPCs — NO CHANGE.** They act on another tenant by design and already call `RequireRole(interceptor.RolePlatformAdmin)` as their first statement. Verify, do not modify.

| Handler | tenant read | `RequireRole` at |
|---|---|---|
| `DeactivateUser` | 202 | 199 |
| `RequestTenantPurge` | 461 | 458 |
| `ApproveTenantPurge` | 479 | 476 |
| `CancelTenantPurge` | 497 | 494 |
| `StartImpersonation` | 559 | 552 |
| `SetOIDCConfig` | 609 | 605 — note it passes `req.GetTenantId()` as a **raw string**, never parsed |

**(c) Tenant-scoped RPCs — adopt the helper.** Nine handlers. Each takes a tenant id only because nothing ever told it the caller's.

| Handler | tenant read |
|---|---|
| `CreateUser` | 128 |
| `GetUser` | 145 |
| `ListUsers` | 161 |
| `UpdateUser` | 177 |
| `AssignRole` | 230 |
| `ListRoles` | 268 |
| `AssignProjectRole` | 304 |
| `SetTenantAddress` | 355 |
| `InviteUser` | 514 |

```go
-	tenantID, err := uuid.Parse(req.GetTenantId())
-	if err != nil {
-		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
-	}
+	tenantID, err := interceptor.TenantFromRequest(ctx, req.GetTenantId())
+	if err != nil {
+		return nil, err
+	}
```

**(d) `CreateTenant` — add the missing gate.** It reads **no** tenant from the request (it creates one), so the helper does not apply. Today any authenticated user can create a tenant.

```go
 func (h *Handler) CreateTenant(ctx context.Context, req *iamv1.CreateTenantRequest) (*iamv1.Tenant, error) {
+	if err := interceptor.RequireRole(ctx, interceptor.RolePlatformAdmin); err != nil {
+		return nil, err
+	}
 	tenant := &Tenant{
```

**(e) `RevokeRole` — CANNOT be fixed by this slice. Do not try.**

`RevokeRoleRequest` carries only `user_id` and `role_id` — **no `tenant_id`**. So `TenantFromRequest` has nothing to compare, and any authenticated user can still revoke any user's roles in any tenant.

Closing it requires a *resource-ownership* check: load the role, compare `role.TenantID` to `caller.TenantID`. That is a service-layer change, which this slice forbids (§8 of the spec: "no service-layer change"). Leave `RevokeRole` alone, and say so in your report. It is recorded as a known gap in #144 and belongs to #139's next slice.

**Do not invent a tenant check the request cannot support.**

**(d) Forgeable audit identity.** `AssignRole` reads `assigned_by` from the request; `InviteUser` reads `invited_by`. Both are identity, and both are attacker-supplied, so the audit trail records whoever the caller names.

```go
-	assignedBy, err := uuid.Parse(req.GetAssignedBy())
-	if err != nil {
-		return nil, status.Error(codes.InvalidArgument, "invalid assigned_by")
-	}
+	caller, ok := interceptor.CallerFromContext(ctx)
+	if !ok {
+		return nil, status.Error(codes.Unauthenticated, "caller identity not present in context")
+	}
+	assignedBy := caller.UserID
```

The proto field stays (wire compatibility, `buf breaking`); the handler stops reading it. Same for `invited_by`.

- [ ] **Step 1: Confirm the inventory.** Re-derive the sixteen with `grep -n 'req.GetTenantId()' services/iam/handler.go` and check it against the tables above. **Paste both into your report.** If they disagree, the plan is stale — stop and report rather than guessing which is right.
- [ ] **Step 2: Add a tenant-scoped caller helper** to `handler_test.go`, alongside the existing `platformAdminCtx()`:

```go
// memberCtx returns a caller in tenant tid holding only the `member` role —
// enough to pass authentication, not enough for platform-admin gates.
func memberCtx(tid uuid.UUID) context.Context {
	return interceptor.WithCaller(context.Background(), interceptor.CallerInfo{
		UserID:   uuid.New(),
		TenantID: tid,
		Email:    "member@example.com",
		Roles:    []string{interceptor.RoleMember},
	})
}
```

Note `platformAdminCtx()` currently sets no `TenantID`. Platform RPCs do not call `TenantFromRequest`, so that stays valid — **do not add a tenant to it** or you will mask a bucket-(a)/(c) misclassification.

- [ ] **Step 3: Apply (b) and (c) and (d).**
- [ ] **Step 4: Fix the 53 existing test call sites.**
- [ ] **Step 5: Write the four new tests.**

`services/iam/handler_test.go` has only `newHandler()` (line 33), which hardcodes `NewHandler(newTestService(&mockRepo{}))`. To inject a repo you need a local constructor — add one next to it:

```go
func newHandlerWithRepo(r *mockRepo) *Handler { return NewHandler(newTestService(r)) }
```

`mockRepo.AssignRole` takes a **`*UserRole` struct**, not positional args (`services/iam/service_test.go:269`):

```go
type UserRole struct {
	UserID     uuid.UUID
	RoleID     uuid.UUID
	ProjectID  *uuid.UUID
	AssignedBy uuid.UUID
	AssignedAt time.Time
}
```

```go
func TestCreateTenant_RequiresPlatformAdmin(t *testing.T) {
	t.Parallel()
	_, err := newHandler().CreateTenant(memberCtx(uuid.New()), &iamv1.CreateTenantRequest{Name: "x"})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestCreateUser_CrossTenant_Denied(t *testing.T) {
	t.Parallel()
	caller, victim := uuid.New(), uuid.New()
	require.NotEqual(t, caller, victim)

	h := newHandlerWithRepo(&mockRepo{
		createUserFn: func(context.Context, *User, string) error {
			t.Fatal("repository must not be reached on a cross-tenant request")
			return nil
		},
	})
	_, err := h.CreateUser(memberCtx(caller), &iamv1.CreateUserRequest{
		TenantId: victim.String(), Email: "a@b.c", Password: "x",
	})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestAssignRole_CrossTenant_Denied(t *testing.T) {
	t.Parallel()
	caller, victim := uuid.New(), uuid.New()
	h := newHandlerWithRepo(&mockRepo{
		assignRoleFn: func(context.Context, *UserRole) error {
			t.Fatal("repository must not be reached on a cross-tenant request")
			return nil
		},
	})
	_, err := h.AssignRole(memberCtx(caller), &iamv1.AssignRoleRequest{
		TenantId: victim.String(), UserId: uuid.New().String(), RoleId: uuid.New().String(),
	})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// The audit trail must name the caller, not whoever the request names.
func TestAssignRole_AssignedByIsTheCaller(t *testing.T) {
	t.Parallel()
	tid, callerID := uuid.New(), uuid.New()
	var gotAssignedBy uuid.UUID

	h := newHandlerWithRepo(&mockRepo{
		assignRoleFn: func(_ context.Context, ur *UserRole) error {
			gotAssignedBy = ur.AssignedBy
			return nil
		},
	})
	ctx := interceptor.WithCaller(context.Background(), interceptor.CallerInfo{
		UserID: callerID, TenantID: tid, Roles: []string{interceptor.RoleMember},
	})

	_, err := h.AssignRole(ctx, &iamv1.AssignRoleRequest{
		TenantId:   tid.String(),
		UserId:     uuid.New().String(),
		RoleId:     uuid.New().String(),
		AssignedBy: uuid.New().String(), // a lie the handler must ignore
	})
	require.NoError(t, err)
	assert.Equal(t, callerID, gotAssignedBy, "assigned_by must come from the token, not the request")
}
```

Check `mockRepo`'s exact fn-field names and signatures in `services/iam/service_test.go` before writing these — `createUserFn` may take a password argument, as shown. If `svc.AssignRole` does not accept an `assignedBy` the handler can override, report it rather than reshaping the service.

- [ ] **Step 6: Verify**

Run: `go test ./services/iam/... -v 2>&1 | tail -25` — PASS
Run: `go vet ./...` — clean
Run: `go test ./services/iam/ -coverprofile=/tmp/c.out && go tool cover -func=/tmp/c.out | tail -1` — **≥ 85%**

- [ ] **Step 7: Commit**

```bash
git add services/iam/handler.go services/iam/handler_test.go
git commit -m "fix(iam): tenant-scoped RPCs use the token's tenant; CreateTenant requires platform_admin (#144)"
```

---

### Task 7: Proto — deprecate and ignore

**Files:**
- Modify: `proto/thittam/ledger/v1/*.proto`, `billing/v1`, `document/v1`, `notifications/v1`, `iam/v1`

The fields **stay**. `buf breaking` runs in CI against `.git#branch=main,subdir=proto` (confirmed, `.github/workflows/ci.yml` with `fetch-depth: 0`), and removing 61 request fields would fail it — correctly, since clients still send them.

- [ ] **Step 1: Comment every `tenant_id` request field** in the five services' protos (not response messages, not `iam`'s platform RPCs, whose `tenant_id` is still read):

```proto
  // Deprecated: ignored. The tenant is derived from the caller's verified token
  // (#144). Sending a value that differs from the token's tenant is rejected
  // with PermissionDenied.
  string tenant_id = 1;
```

Leave the platform RPCs' `tenant_id` (`SuspendTenantRequest`, `RequestTenantPurgeRequest`, …) uncommented and unchanged — those are read, deliberately.

- [ ] **Step 2: Do NOT regenerate.** Comments do not change the wire format or the generated Go. If `buf generate` produces a diff, something else changed — stop and report.

- [ ] **Step 3: Verify**

Run: `buf lint` — clean
Run: `buf breaking proto --against '.git#branch=main,subdir=proto'` — clean (comments are not breaking)
Run: `git diff --stat gen/` — **empty**. If `gen/` changed, you regenerated; revert it.

- [ ] **Step 4: Commit**

```bash
git add proto/
git commit -m "docs(proto): mark request tenant_id deprecated and ignored (#144)"
```

---

## Verification (whole branch, before PR)

- [ ] `go vet ./...` — clean.
- [ ] `go test ./... -short` — PASS.
- [ ] `go test -race ./pkg/interceptor/ ./services/{ledger,billing,document,notifications,iam}/` — PASS.
- [ ] `grep -rn 'uuid.Parse(req.GetTenantId())\|uuid.Parse(req.TenantId)' services/{ledger,billing,document,notifications}/handler.go` — **zero hits**.
- [ ] In `services/iam/handler.go`, the only remaining `uuid.Parse(req.GetTenantId())` are in bucket-(a) platform handlers, each preceded by `RequireRole`. List them.
- [ ] `grep -rn 'GetAssignedBy()\|GetInvitedBy()' services/iam/handler.go` — zero hits.
- [ ] Coverage: `iam` ≥ 85%, `pkg/interceptor` = 100%, four services no regression.
- [ ] `git diff --stat gen/` — empty.
- [ ] **`gh pr checks <n>` after opening the PR.** Local green is not CI green. Do not declare ready until every job passes.

## What this does not fix

An authenticated user still can, **within their own tenant**, grant themselves `super_admin` via `AssignRole`, revoke anyone's roles, and call ~100 RPCs that check no permission. Role revocation still takes up to 7 days to take effect.

That is #139. Do not let this merge be read as "the platform is authorized."
