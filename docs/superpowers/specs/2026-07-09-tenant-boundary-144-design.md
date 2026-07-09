# Enforce the tenant boundary (#144)

**Status:** approved (design), 2026-07-09
**Issue:** [#144](https://github.com/wegofwd2020-hub/thittam/issues/144) — slice 1 of [#139](https://github.com/wegofwd2020-hub/thittam/issues/139)
**Follows:** [#138](https://github.com/wegofwd2020-hub/thittam/issues/138) (fail-closed authentication)

## 1. Problem

Any authenticated user can read or modify **any other tenant's data** by naming that
tenant's UUID in the request body.

`services/ledger/handler.go:274`:

```go
tenantID, err := uuid.Parse(req.GetTenantId())
if err != nil {
    return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
}
```

That value flows into `WHERE tenant_id = $1`. The caller's verified tenant — sitting in
context since #138 — is never consulted. The same pattern appears in
`services/billing/handler.go:109` (`ListInvoices`), and throughout `document` and
`notifications`.

**No handler anywhere compares `caller.TenantID` against a request tenant.**
`grep -rn 'caller.TenantID' services/` returns nothing outside tests.

| Tenant sourced from | Services | RPCs |
|---|---|---|
| **request body** — reachable | ledger 12, billing 12, document 13, notifications 8, iam 16 | **61** |
| verified token context — immune | project 7, budget 7, expense 10, inventory 5, reporting 3 | 32 |

### Why exactly those five services

`ledger`, `billing`, `document`, `notifications` and `iam` are precisely the five services
that **had no caller interceptor before #138**. They read the tenant from the request
because nothing put a tenant in their context. The request field was never a design
choice; it was the only thing available.

#138 gave them a verified tenant in context for the first time, and nobody changed the
handlers. The cross-tenant vulnerability and the missing-interceptor vulnerability are the
same bug, observed one layer apart. #138 fixed the cause and left the symptom.

## 2. Scope

In scope:

- A single helper that yields the caller's tenant and refuses a mismatched request tenant.
- The 45 non-iam handlers adopt it.
- iam splits three ways (§5).
- `AssignRole.assigned_by` and `InviteUser.invited_by` come from the caller, not the request.
- Proto `tenant_id` fields marked deprecated-and-ignored.
- The repository's first cross-tenant tests.

Out of scope, and remaining in #139:

- **`AssignRole` still will not check whether the caller may grant that role.** A tenant
  member can still grant themselves `super_admin` *within their own tenant*.
- `RevokeRole` still lets any member revoke anyone's roles.
- The ~100 RPCs that enforce no permission check.
- Role-revocation latency: `RequireRole` reads a login-time snapshot of the `roles` claim,
  and `Refresh` re-issues from a Redis `refreshPayload` **without re-querying the
  database**, so a revoked role survives for up to the 7-day refresh window — not the
  15-minute access TTL. (`RequirePermission` is exempt: it queries iam live.)

This spec closes the **tenant** boundary. It does not close the **privilege** boundary,
and must not be read as doing so.

## 3. Verified, and downgraded

An earlier reading of the model suggested a tenant could create a role named
`platform_admin` and thereby satisfy `RequireRole(RolePlatformAdmin)`, since `RequireRole`
is a string match on token role names and `roles.name` is only `UNIQUE (tenant_id, name)`.

`CreateRole` exists only at the repository layer (`services/iam/db/postgres.go:547`). **No
RPC exposes it**; roles are seeded at tenant creation. The escalation requires direct
database access. Real, but not reachable through the API. Not addressed here.

## 4. The primitive

```go
// TenantFromRequest returns the caller's tenant, taken from the verified token.
//
// If the request also carries a tenant id and it differs, the call is refused. A
// client asking about a tenant that is not its own is either broken or hostile;
// either way we would rather say so than quietly answer about a different tenant
// than the one it named.
//
// The returned tenant NEVER comes from the request. A handler cannot query with a
// caller-supplied tenant, because the only tenant it can obtain is this one.
func TenantFromRequest(ctx context.Context, reqTenantID string) (uuid.UUID, error)
```

| Condition | Result |
|---|---|
| no caller in context | `codes.Unauthenticated` — unreachable after #138; kept as a belt |
| caller's `tid` is `uuid.Nil` | `codes.Unauthenticated`, "token carries no tenant" |
| `reqTenantID == ""` | caller's tenant |
| `reqTenantID` unparseable | `codes.InvalidArgument` |
| parsed ≠ caller's tenant | **`codes.PermissionDenied`** |
| parsed == caller's tenant | caller's tenant |

**The safety property is structural, not disciplinary.** The helper *returns the tenant the
handler needs*, so skipping the check leaves the handler with no tenant to query with.

Contrast `RequirePermission`, which returned only an error: a handler could omit the call
entirely and still function. It was omitted — behind `if h.perm != nil` — at twenty call
sites, and nobody noticed for a year. A check you can forget is a check that will be
forgotten. A value you cannot obtain without checking is not.

**Location: `pkg/interceptor`, not `pkg/tenant`.** `pkg/tenant` is a context-key package
with no gRPC dependency. Returning `status.Error` from it would drag gRPC into every
consumer. `pkg/interceptor` already owns authorization semantics and already imports
`grpc/status`.

## 5. Handler changes

### The 45 non-iam handlers

`ledger` (12), `billing` (12), `document` (13), `notifications` (8):

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

Service and repository layers are untouched — they already take a `tenantID` parameter.
This converges these four services onto the pattern `project`, `budget`, `expense`,
`inventory` and `reporting` already use.

### iam, three ways

iam's sixteen request-tenant handlers are not one case with exceptions. They are three
kinds of RPC that happen to share a field.

**(a) Platform RPCs — keep the request tenant, no change.** Acting on another tenant is the
entire point. All ten already call `RequireRole(RolePlatformAdmin)`:
`SuspendTenant`, `ClearTenantLegalHold`, `SetTenantRetention`, `RequestTenantPurge`,
`ApproveTenantPurge`, `CancelTenantPurge`, `DeactivateUser`, `SetOIDCConfig`,
`StartImpersonation`, `EndImpersonation`.

**(b) `CreateTenant` — platform-only, gated by nothing.** Gains
`RequireRole(RolePlatformAdmin)`. Today any authenticated user can create tenants.

**(c) Tenant-scoped RPCs — adopt the helper.** `CreateUser`, `InviteUser`, `UpdateUser`,
`AssignRole`, `RevokeRole`. They take a tenant id only because nothing ever told them the
caller's. After this they can act only inside the caller's tenant. `CreateUser` can
currently inject a user into an arbitrary tenant.

### Forgeable audit fields

`AssignRole` reads `assigned_by` from the request; `InviteUser` reads `invited_by`. Both
are identity, not data, and both are attacker-supplied — so the audit trail lies about who
did it. They must come from `caller.UserID`.

The proto fields stay (wire compatibility); the handlers stop reading them.

## 6. Proto

The `tenant_id` request fields remain. `buf breaking` runs against `main` in CI, and
removing 61 request fields would fail it — correctly, since clients still send them.

Each gains a comment:

```proto
// Deprecated: ignored. The tenant is derived from the caller's verified token
// (#144). Sending a value that differs from the token's tenant is rejected
// with PermissionDenied.
string tenant_id = 1;
```

A follow-up removes them once no client sends them.

## 7. Testing

**This repository contains no cross-tenant test.** `grep` for cross-tenant / isolation /
tenantB across `*_test.go` finds only a *within*-tenant project-scope leak test
(`services/iam/service_test.go:684`). That absence is why this survived a year.

- **Table test on `TenantFromRequest`**, covering all six rows of §4. The row that matters:
  token tenant A, request tenant B → `PermissionDenied`.
- **One cross-tenant handler test per affected service** — ledger, billing, document,
  notifications, iam. A caller whose token says tenant A, a request naming tenant B:
  assert `PermissionDenied` **and that the repository was never called**.

  The second clause is the point. Asserting only the error would pass against a handler
  that queries first and checks afterwards — which returns the right error and still reads
  the row.
- **`CreateTenant` denies a caller without `platform_admin`.**
- **`AssignRole` records `caller.UserID` as `assigned_by`**, ignoring a request value that
  names someone else.

Coverage: `iam` ≥ 85%; `pkg/` ≥ 75%. Neither may regress.

## 8. Blast radius

61 handlers across five services; one new helper; proto comments. **No migration. No
service-layer change. No repository change.**

`pkg/interceptor` is linked by all ten services. Per CLAUDE.md: senior review, two
approvals (`iam` / security).

The change is mechanical in shape but not in judgement: each iam handler must be sorted
into (a), (b) or (c) by hand, and putting a tenant-scoped RPC in bucket (a) would leave the
hole open while looking fixed.

## 9. What this does not fix

After this, an authenticated user cannot touch another tenant. They can still, **within
their own tenant**, grant themselves `super_admin` via `AssignRole`, revoke anyone's roles,
and call roughly a hundred RPCs that check no permission at all.

That is #139. Do not read this merge as "the platform is authorized."
