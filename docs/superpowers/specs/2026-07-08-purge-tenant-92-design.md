# Design: PurgeTenant — two-person hard-delete of purge_eligible tenants (#92 Stage 3)

**Issue:** #92 (Stage 3, terminal step of the retention lifecycle). **Date:** 2026-07-08
**Scope:** iam service — two-person approval RPCs + a new purge-worker binary + one migration.
**Branch:** `feat/purge-tenant-92` (to be created)
**Depends on:** #118 (retention clock) + #119 (operator hold) — both merged. This consumes the `purge_eligible` terminal state they lead to.

## Context

A tenant reaches `purge_eligible` 180 days after `deactivated_at` (`services/iam/lifecycle.go:16-30`).
Today that status is a **true dead-end**: the retention sweeper surfaces it but never acts
(`cmd/retention-sweeper/main.go:10-12` — "actual schema drop requires the (forthcoming, #92
Stage 3) PurgeTenant RPC under two-person approval"). `nextLifecycleStatus` has no
`purge_eligible` case (`lifecycle.go:180-181`). PurgeTenant is the missing consumer.

A tenant is **two-homed** in Postgres: a per-tenant schema `tenant_<uuid>` (all business data)
and a row in the shared `public.tenants` table (`+` users/roles). Purging means dropping the
schema and retiring the row.

**Three findings from reconnaissance shape this design:**

1. **Preserving the audit log is free.** `audit_log` lives in shared `public`, keyed by a plain
   `tenant_id` UUID column (not an FK into the tenant schema); `pkg/audit` always writes via a
   non-tenant-scoped pool (`pkg/audit/postgres.go:38-60`). A `DROP SCHEMA tenant_<uuid>` cannot
   touch it. No special handling required.

2. **The destructive op needs OWNER privileges.** Schema creation runs implicitly via
   golang-migrate over the owner (`thittam`) DSN. `DROP SCHEMA` requires ownership; the
   least-privilege runtime role `thittam_app` (#120/#122) has only `USAGE` on `public` + table
   DML and **cannot drop schemas** (nor `UPDATE`/`DELETE` `audit_log`,
   `scripts/db-grant-app-role.sql:25`). #123 plans to repoint service pods to `thittam_app`.
   **Therefore the purge must not run inside a service-pod gRPC handler** — it runs in a
   dedicated worker under the owner DSN.

3. **No maker-checker precedent exists.** Expense "dual approval" is only a monetary threshold
   that rejects (`services/expense/service.go:87-90`); budget is single-approver. The persisted
   request record + `proposer ≠ approver` enforcement is entirely net-new. The
   `RequireRole(RolePlatformAdmin)` gate + `audit.ActorFromContext(ctx)` identity read
   (`pkg/interceptor/auth.go:107-111`, `services/iam/handler.go:388`) is the template.

## Architecture

```
RequestTenantPurge (admin A) ─▶ tenant_purge_requests row: pending
ApproveTenantPurge (admin B, B≠A) ─▶ status=approved            [NO delete here]
CancelTenantPurge (any admin) ─▶ status=cancelled              [safety valve, pre-execution]

cmd/purge-worker  (K8s CronJob, daily, OWNER DSN):
  for each request status=approved:
    tx: re-verify tenant.status='purge_eligible'  (status-guarded; else → failed)
        DROP SCHEMA IF EXISTS "tenant_<uuid>" CASCADE
        UPDATE tenants SET status='purged',
               name = 'purged-' || id::text,           -- sentinel: name is NOT NULL + ci-unique
               address_line1=NULL, address_line2=NULL, city=NULL, postal_code=NULL,
               purged_at=now()
                WHERE id=$1 AND status='purge_eligible'
        UPDATE tenant_purge_requests SET status='executed', executed_at=now()
    on error → status='failed', failure_reason  (retryable: all steps idempotent)
```

**Tenant row fate = tombstone.** After purge the `tenants` row is kept with `status='purged'`,
`purged_at` set, `slug` + `id` + lifecycle timestamps retained. PII erasure: the nullable address
columns (`address_line1`, `address_line2`, `city`, `postal_code`) are set NULL, and the company
`name` is overwritten with a per-tenant sentinel `'purged-' || id` — **`name` cannot be NULLed**
(`NOT NULL` since migration 001, and part of the `tenants_name_ci_unique` index from #015; the
id-derived sentinel satisfies both). `country_code`/`primary_currency_code` are `NOT NULL` with
CHECK regexes (migration 014) and are non-identifying, so they are retained. This prevents id
resurrection, keeps `audit_log.tenant_id` resolvable to a row, and reserves the slug. `'purged'`
is added to the `tenants_status_check`. The real PII (all business data) is destroyed with the
schema; the request record's `tenant_name`/`tenant_slug` snapshot preserves the forensic name.

**Worker cadence = daily** CronJob, mirroring `cmd/retention-sweeper` (tenants sit in
`purge_eligible` for a bounded window, so daily is ample and keeps the destructive job
low-frequency and auditable).

## Component 1 — migration `019_tenant_purge_requests` (`migrations/iam/`)

Next number is 019 (latest is 018). `.up.sql`:

- **`CREATE TABLE tenant_purge_requests`**: `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`,
  `tenant_id UUID NOT NULL`, `status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN
  ('pending','approved','executed','failed','cancelled'))`, `requested_by UUID NOT NULL`,
  `requested_at TIMESTAMPTZ NOT NULL DEFAULT now()`, `request_reason TEXT NOT NULL`,
  `approved_by UUID`, `approved_at TIMESTAMPTZ`, `cancelled_by UUID`, `cancelled_at TIMESTAMPTZ`,
  `executed_at TIMESTAMPTZ`, `failure_reason TEXT`, and forensic snapshots
  `tenant_name TEXT NOT NULL`, `tenant_slug TEXT NOT NULL` (captured at request time because the
  tombstone nulls the live name).
- **Partial unique index** enforcing at most one OPEN request per tenant:
  `CREATE UNIQUE INDEX tenant_purge_requests_one_open ON tenant_purge_requests (tenant_id)
   WHERE status IN ('pending','approved');`
- Index `(status)` for the worker poll: `WHERE status = 'approved'`.
- **`ALTER TABLE tenants ADD COLUMN purged_at TIMESTAMPTZ;`**
- **Broaden the status CHECK** to add `'purged'`: drop `tenants_status_check`, re-add with
  `('active','suspended','grace','deactivated','purge_eligible','purged')`.
- **No change to `name`'s NOT NULL / ci-unique** — the tombstone writes a sentinel value, not
  NULL (see Architecture), so the migration doesn't relax those constraints.

`.down.sql`: drop the table + its indexes, drop `tenants.purged_at`, restore the prior CHECK
(without `'purged'`). Follows the reversible `IF EXISTS` pattern of `016/017_*.down.sql`.

## Component 2 — proto (`proto/thittam/iam/v1/iam.proto`, Tenants block)

Additive — three RPCs keyed by `tenant_id` (the one-open-request invariant makes a request id
unnecessary for callers):

```proto
rpc RequestTenantPurge(RequestTenantPurgeRequest) returns (TenantPurgeRequest);
rpc ApproveTenantPurge(ApproveTenantPurgeRequest) returns (TenantPurgeRequest);
rpc CancelTenantPurge(CancelTenantPurgeRequest) returns (TenantPurgeRequest);

message RequestTenantPurgeRequest { string tenant_id = 1; string reason = 2; }
message ApproveTenantPurgeRequest { string tenant_id = 1; string reason = 2; }
message CancelTenantPurgeRequest  { string tenant_id = 1; string reason = 2; }

message TenantPurgeRequest {
  string id = 1;
  string tenant_id = 2;
  string status = 3;               // pending|approved|executed|failed|cancelled
  string requested_by = 4;
  google.protobuf.Timestamp requested_at = 5;
  string request_reason = 6;
  string approved_by = 7;          // empty until approved
  google.protobuf.Timestamp approved_at = 8;
  google.protobuf.Timestamp executed_at = 9;
  string failure_reason = 10;
}
```

gRPC-only (no `google.api.http`), matching the sibling tenant-admin RPCs. `buf breaking` clean
(purely additive). Regenerate with `buf generate`.

## Component 3 — service + handlers (`services/iam/purge.go` new; `handler.go`)

All three handlers: `RequireRole(ctx, interceptor.RolePlatformAdmin)` → parse UUID →
delegate → `grpcError` → return the request proto. Service methods:

- **`RequestTenantPurge(ctx, tenantID, reason)`**: reason required (`InvalidArgument` if empty).
  `GetTenant`; must be `status='purge_eligible'` → else `ErrTenantNotPurgeable`
  (`FailedPrecondition`). Insert a `pending` row with `requested_by = actor.UserID` and
  `tenant_name`/`tenant_slug` snapshot. The partial unique index makes a concurrent/duplicate
  open request fail → mapped to `ErrPurgeRequestExists` (`AlreadyExists`). Audit
  `ActionPurgeRequested`.
- **`ApproveTenantPurge(ctx, tenantID, reason)`**: load the open (`pending`) request for the
  tenant → `ErrPurgeRequestNotFound` (`NotFound`) if none. **`actor.UserID != request.RequestedBy`**
  else `ErrSelfApproval` (`FailedPrecondition`). Re-verify tenant still `purge_eligible`
  (defense against a state change since request) → `ErrTenantNotPurgeable` if not. Set
  `status='approved'`, `approved_by`, `approved_at`. Audit `ActionPurgeApproved`. **No delete.**
- **`CancelTenantPurge(ctx, tenantID, reason)`**: any platform-admin. Load the open
  (`pending` OR `approved`) request → `ErrPurgeRequestNotFound` if none. Set `status='cancelled'`,
  `cancelled_by`, `cancelled_at`. Audit `ActionPurgeCancelled`. Closes the window between
  approval and worker execution.

Actor identity from `audit.ActorFromContext(ctx)` (populated by the caller interceptor).

## Component 4 — purge-worker (`cmd/purge-worker/`, new binary)

Mirrors `cmd/retention-sweeper/` structure (batch CLI, K8s CronJob, Prometheus metrics, its own
`main.go` + `metrics.go`). **Runs under the OWNER DSN** (`DATABASE_URL` = `thittam`, not
`thittam_app`) — deploy note below.

`runPurge(ctx, svc, now)`:
- Poll approved requests (new repo method `ListApprovedPurgeRequests(ctx, limit)`), batched like
  the sweeper.
- For each, `PurgeTenant(ctx, req)` in **one pgx transaction** (ledger `Begin`/`WithTx`/`Commit`
  pattern, `services/ledger/db/postgres.go`):
  1. status-guarded re-read: tenant must be `purge_eligible` — if not, mark request `failed`
     (`failure_reason="tenant no longer purge_eligible"`) and continue (a cancel or manual change
     raced us).
  2. `DROP SCHEMA IF EXISTS "tenant_<uuid>" CASCADE` via raw `p.db.Exec` (schema name =
     `"tenant_" + id.String()`, interpolated — Postgres can't parameterize identifiers; safe per
     the validated-UUID rationale in `pkg/tenantdb/tenantdb.go:7-21`). `IF EXISTS` makes re-runs
     idempotent.
  3. `UPDATE tenants SET status='purged', name=NULL, address_line1=NULL, … , purged_at=now()
     WHERE id=$1 AND status='purge_eligible'` (status-guard prevents double-purge).
  4. `UPDATE tenant_purge_requests SET status='executed', executed_at=now() WHERE id=$1`.
- On any error: roll back, mark the request `failed` with `failure_reason` (a follow-up run
  retries safely — every step is idempotent/guarded). Emit `ActionTenantPurged` audit with actor
  `system:purge-worker` (new system-actor const, mirroring `SystemActorRetentionSweeper`,
  `lifecycle.go:35`). Prometheus counters: purged / failed / skipped.

## Component 5 — repository (`repository.go` interface + `db/postgres.go` + `db/queries.sql`)

New methods (widening `iam.Repository` — update all three implementers per
`[[reference_iam_repository_implementers]]`: `*Postgres`, `*mockRepo`, e2e `*iamRepo`; gate with
whole-tree `go vet ./...`):
- `CreateTenantPurgeRequest(ctx, *TenantPurgeRequest) error`
- `GetOpenTenantPurgeRequest(ctx, tenantID) (*TenantPurgeRequest, error)` (status in
  pending/approved)
- `UpdateTenantPurgeRequestStatus(ctx, id, from, to string, ...) (*TenantPurgeRequest, bool, error)`
  (status-guarded, like `TransitionTenantStatus`)
- `ListApprovedPurgeRequests(ctx, limit int) ([]*TenantPurgeRequest, error)`
- `PurgeTenantSchemaAndTombstone(ctx, tenantID) error` — the raw-DDL + tombstone tx (owner-only).

sqlc for the CRUD; raw `p.db.Exec` + `Begin`/`WithTx`/`Commit` for `PurgeTenantSchemaAndTombstone`
(sqlc can't express `DROP SCHEMA <dynamic>`).

## Component 6 — audit actions (`pkg/audit/types.go`)

Four new `Action` constants: `ActionPurgeRequested`, `ActionPurgeApproved`,
`ActionPurgeCancelled`, `ActionTenantPurged`. Distinct actions give a clean forensic trail
(request → approve → purge, or → cancel). `ResourceTenant` reused.

## Error handling

| Condition | Sentinel | gRPC code |
|---|---|---|
| bad UUID / empty reason | — | `InvalidArgument` |
| tenant not `purge_eligible` | `ErrTenantNotPurgeable` | `FailedPrecondition` |
| open request already exists | `ErrPurgeRequestExists` | `AlreadyExists` |
| no open request to approve/cancel | `ErrPurgeRequestNotFound` | `NotFound` |
| approver == requester | `ErrSelfApproval` | `FailedPrecondition` |
| not platform-admin | (RequireRole) | `PermissionDenied` |

Mapped in `grpcError` (`handler.go`), alongside the existing sentinels.

## Testing (iam ≥ 85%)

- **Service unit** (`purge_test.go`): request rejects non-`purge_eligible`; duplicate open request
  rejected; approve rejects self-approval (approver==requester); approve rejects when tenant left
  `purge_eligible`; cancel a pending and an approved request; happy request→approve; audit events
  emitted with expected actions/actors.
- **Worker unit**: status-guarded tombstone UPDATE; idempotent re-run (schema already gone,
  request already executed); failure path marks `failed` with reason; skips when tenant no longer
  `purge_eligible`.
- **Integration** (real Postgres, `-tags=integration`): full request→approve→`PurgeTenantSchemaAndTombstone`
  — create a throwaway `tenant_<uuid>` schema, run the purge, assert the schema is gone, the
  `tenants` row is tombstoned (`status='purged'`, `name` NULL, `purged_at` set), and a prior
  `audit_log` row for that tenant **still exists** (survives the drop). Unknown-id and
  already-purged idempotency cases.

## Non-goals

- Auto-purge without human approval (two-person gate is the point).
- Restoring a purged tenant (irreversible by design).
- A list/read RPC for pending requests (the admin UI can query; keyed-by-tenant_id RPCs suffice
  for MVP — add later if needed).
- Changing the retention sweeper or the #123 role repoint (the worker just needs the owner DSN).

## Files touched

Migration `migrations/iam/019_tenant_purge_requests.{up,down}.sql`; proto
`proto/thittam/iam/v1/iam.proto` (+`buf generate` → `gen/iam/v1/`); `services/iam/purge.go` (new);
`services/iam/handler.go`; `services/iam/repository.go`; `services/iam/db/queries.sql`
(+`sqlc generate`); `services/iam/db/postgres.go`; `services/iam/errors.go`;
`pkg/audit/types.go`; `cmd/purge-worker/{main,metrics}.go` (new); e2e `iamRepo` double;
K8s CronJob manifest under `infra/k8s/`; plus the three test files.

## Ops / deploy

The purge-worker CronJob must be wired to the **owner** DSN — its `DATABASE_URL` reads
`secretKeyRef: name=thittam-db, key=url` (the owner URL), **NOT `key=runtime_url`** (which the
retention-sweeper uses — the sweeper only does status UPDATEs, so `thittam_app` sufficed; the
purge-worker needs DDL/DROP, which `thittam_app` cannot do). This is the mirror of the #123
role-split note: service pods → `thittam_app`, the purge-worker (like the migrator) → owner.
Call this out in the deploy checklist.

**Note — `pkg/tenantdb.schemaNameFor` is unexported.** The worker's `DROP SCHEMA` needs the same
`"tenant_" + id.String()` name; replicate that one-liner in the iam repo method (with a comment
citing the `pkg/tenantdb` validated-UUID safety rationale) rather than exporting the helper — keeps
the change contained to iam.
