# Notifications Authorization (#139 slice G) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-23
**Issue:** #139 (platform-wide authorization), slice G — notifications-analytics service
**Branch:** `fix/notifications-authz-139g` off `main` (`3924668`)
**Migration:** `023`

## Goal

Bring the notifications service under #139's authorization model: gate the six
config/send RPCs on `notifications:read` / `notifications:manage`, and close the
D9 defect by self-scoping the two personal-inbox reads to the caller's own
recipient id. This is the **last gating slice** of #139.

## Context

Eight RPCs on `services/notifications/handler.go` are currently ungated. All
eight already resolve tenant from the verified token
(`interceptor.TenantFromRequest`), so tenant isolation is intact — the gap is
**permission** on config/send, and **recipient-level** scoping on the personal
inbox.

Notifications has **no perm field** today: `NewHandler(svc *Service)`. Wiring it
fail-closed (dial IAM, `log.Fatalf` if absent) is part of this slice, mirroring
ledger / reporting / document / billing.

### The three RPC groups

| RPC | Control | Rationale |
|---|---|---|
| `ListNotifications` | **self-scope**, AUTH only | personal inbox; caller sees only own |
| `GetNotification` | **self-scope**, AUTH only | personal inbox; caller sees only own |
| `GetTemplate` | `notifications:read` | tenant notification config |
| `ListTemplates` | `notifications:read` | tenant notification config |
| `CreateTemplate` | `notifications:manage` | config write |
| `UpdateTemplate` | `notifications:manage` | config write |
| `Send` | `notifications:manage` | privileged send |
| `Dispatch` | `notifications:manage` | privileged send |

### What is deliberately NOT touched

- **Event-driven delivery.** `cmd/notifications/dispatcher.go` (NATS consumer)
  calls `d.svc.Send` / `d.svc.Dispatch` at the **Service** layer — it never
  constructs or crosses a `Handler`. Gating the handler methods does not affect
  the event path. (Handler doc comment already states this contract.)
- **`UpdateNotificationLog` internal status update** (`db/postgres.go:152`,
  `UPDATE notification_log SET status = $2 WHERE id = $1`). Dispatcher-internal,
  keyed by the log row id, no RPC reaches it. Correct-unscoped, same class as
  billing's outbox writer. Left as-is; documented in the plan.

### §4.1 sibling cross-tenant scan (done at design time)

Ran the three #157 shapes against the whole service:

1. **Handler parses id with no tenant block** — none. All eight handlers call
   `TenantFromRequest` before any `uuid.Parse`.
2. **Raw SQL `WHERE id = $1` with no `tenant_id`** — only the internal
   `UpdateNotificationLog` (classified correct-unscoped above). Every
   caller-reachable read already carries `AND tenant_id = $N`.
3. **Dropped tenant (handler resolves tenantID, service passes bare id)** —
   none. `GetNotification`/`GetTemplate` thread `tenantID` through to the repo.

Result: **no cross-tenant defect** in this slice beyond the D9 recipient-scoping
gap, which is a finer-grained (intra-tenant) issue handled below. No scope
expansion.

## Design

### 1. D9 fix — self-scope the personal inbox (no migration)

`notification_log.recipient_id` is already `UUID NOT NULL`
(`migrations/notifications/002_create_notification_log.up.sql`), so the fix is
a predicate, not a schema change.

**Caller identity comes from the verified token, never the request body**, via
`interceptor.ActorFromRequest(ctx, "")` — returns `caller.UserID`,
`Unauthenticated` if the token carries no subject, and never trusts a
request-supplied id.

Thread `recipientID` as a **required parameter** (guard-by-type: call sites
cannot compile without it):

- `Service.GetNotification(ctx, tenantID, recipientID, id)`
  repo SQL gains `AND recipient_id = $3`; a cross-recipient (or cross-tenant) id
  returns `ErrNotificationNotFound` → gRPC `NotFound`. No existence oracle.
- `Service.ListNotifications(ctx, tenantID, recipientID, channel, status, limit, offset)`
  repo SQL gains `AND recipient_id = $N` (renumber `LIMIT`/`OFFSET`).

Handler wiring, both reads:

```go
recipientID, err := interceptor.ActorFromRequest(ctx, "")
if err != nil {
    return nil, err
}
```

The signature changes plus **all** call sites (service, repo interface, any
`e2e/critical_path/*_test.go` Repository double) land in **one commit**;
whole-tree `go vet ./...` is the completion gate.

### 2. Gates on the six config/send RPCs

Each gated handler opens with the billing/document pattern:

```go
if err := interceptor.RequirePermission(ctx, h.perm, "notifications:read"); err != nil {
    return nil, err
}
```

- `GetTemplate`, `ListTemplates` → `notifications:read`
- `CreateTemplate`, `UpdateTemplate`, `Send`, `Dispatch` → `notifications:manage`

The gate lands **after** each handler's existing `TenantFromRequest` block (the
tenant resolution stays first, as elsewhere).

### 3. Vocabulary + backfill — the three-halves rule

Two new permission strings: `notifications:read`, `notifications:manage`. All
three of the following move together, or the seeds drift:

1. **Migration `migrations/iam/023_seed_notifications_permissions.{up,down}.sql`**
   — idempotent `UPDATE roles` on the **shared public-schema** `roles` table
   (one UPDATE across tenants, not per-schema — `make migrate-all` and
   `cmd/iam` use no `search_path`). `down` removes the two strings.
2. **`systemRoles`** in `services/iam/service.go`.
3. **Both** seed fixtures:
   - `seeds/demo/xyz-cba/007_iam_roles.sql`
   - `seeds/template/new-tenant/001_tenant.sql`

**Grant matrix:**

| Role | `notifications:read` | `notifications:manage` |
|---|---|---|
| super_admin | ✅ | ✅ |
| manager | ✅ | ✅ |
| accountant | — | — |
| (others) | — | — |

Notifications config/send is an ops concern → super_admin + manager only; no
accountant tier (unlike billing). The vocab task ends with `grep -cF` count
checks proving the two strings appear the expected number of times in each of
the three halves.

### 4. Fail-closed wiring

Notifications currently has no perm field. Add it as a **required** constructor
parameter:

- `services/notifications/handler.go`: `NewHandler(svc *Service, perm interceptor.PermissionChecker) *Handler`, storing `h.perm` — same field type as billing (`interceptor.PermissionChecker`). `iamclient.DialFromEnv` returns `*iamclient.PermissionChecker`, which satisfies that interface.
- `cmd/notifications/main.go`: `iamPerm, closeIAM, err := iamclient.DialFromEnv("notifications")`; on error `log.Fatalf` — the service **refuses to start** without IAM. Pass `iamPerm` into `NewHandler`. Ensure `closeIAM` is deferred.
- The dispatcher path (`cmd/notifications/dispatcher.go`) uses the Service
  directly and needs **no** perm — leave it untouched.
- The two self-scoped reads take **no** permission — AUTH + recipient predicate
  only.

The `NewHandler` signature change is a compile break; the new call site in
`cmd/notifications` and any test constructing a Handler land in the same commit.

### 5. Testing

Grant-matrix, self-scope, and idempotency are proven only by
`//go:build integration` tests in the **real-Postgres CI job** — `Migration
Validate` runs against an empty DB (syntax only). `go vet -tags=integration
./...` is the only local compile signal for the integration files.

Required integration cases:

- **Self-scope:** member A's `ListNotifications` returns only A's rows; member B
  seeing A's notification via `GetNotification` → `NotFound`.
- **Grant matrix:** a role without `notifications:read` → `PermissionDenied` on
  `GetTemplate`/`ListTemplates`; without `notifications:manage` →
  `PermissionDenied` on `CreateTemplate`/`UpdateTemplate`/`Send`/`Dispatch`.
- **Granted role** (super_admin/manager) passes all six.
- **Fail-closed:** constructing the service without a perm checker is a compile
  error (guard-by-type) — no runtime test needed; the `log.Fatalf` on nil IAM
  is asserted by the wiring, not a unit test.
- **Migration idempotency:** apply `023` up twice → same grant rows; `down`
  removes both strings.
- **Event path unaffected:** a dispatcher-driven `Send` still writes a
  `notification_log` row (service-layer, ungated).

## Non-goals

- No proto changes (no new RPCs, no request-field changes — self-scope reads the
  token, not a new field).
- No change to the dispatcher / NATS consumer path.
- No accountant or per-project grant for notifications (YAGNI).
- Machine-token handling (slice I) and the #159 tenant-isolation audit (slice H)
  are out of scope.

## Coverage

Notifications floor is ≥ 75% (CLAUDE.md). The new gate + self-scope branches are
covered by the integration cases above plus existing unit tests.

## Files

- `services/notifications/handler.go` — perm field, 6 gates, 2 self-scope reads
- `services/notifications/service.go` — `recipientID` param on Get/List
- `services/notifications/db/postgres.go` — `AND recipient_id = $N` on Get/List
- `services/notifications/*` repo interface + any `e2e/critical_path` double
- `cmd/notifications/main.go` — `iamclient.DialFromEnv`, fail-closed
- `services/iam/service.go` — `systemRoles`
- `migrations/iam/023_seed_notifications_permissions.{up,down}.sql`
- `seeds/demo/xyz-cba/007_iam_roles.sql`
- `seeds/template/new-tenant/001_tenant.sql`
- integration test file(s) under `tests/integration/` (grant matrix + self-scope)
