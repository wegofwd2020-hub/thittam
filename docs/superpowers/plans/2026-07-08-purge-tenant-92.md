# PurgeTenant (#92 Stage 3) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Hard-delete a `purge_eligible` tenant behind two-person approval — drop `tenant_<uuid>` schema + tombstone the `iam.tenants` row — executed off the request path by a dedicated daily worker under the owner DSN.

**Architecture:** Two-person approval RPCs (`RequestTenantPurge` → `ApproveTenantPurge`, `CancelTenantPurge`) persist a `tenant_purge_requests` row; a new `cmd/purge-worker` CronJob (owner DSN) picks up `approved` requests and performs the destructive `DROP SCHEMA` + tombstone in one idempotent transaction. Audit log survives inherently (shared `public.audit_log`, keyed by `tenant_id`).

**Tech Stack:** Go 1.22+, gRPC (`buf generate`), sqlc (`sqlc generate`), pgx v5 / pgtype, golang-migrate, testify, `pkg/audit`, `pkg/interceptor`, `pkg/testdb`, Prometheus.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-08-purge-tenant-92-design.md`.
- Coverage floor **iam ≥ 85%**.
- **Widening `iam.Repository` → update all three implementers**: `*Postgres` (`services/iam/db/postgres.go`), `*mockRepo` (`services/iam/service_test.go`), e2e `*iamRepo` (`e2e/critical_path/helpers_test.go`). Only the first two have compile-time assertions, so **`go vet ./...` (whole tree)** is the gate — `go build ./services/iam/...` alone will not catch the e2e double.
- **The tombstone cannot NULL `name`** (`NOT NULL` since migration 001, part of `tenants_name_ci_unique` from #015) nor `country_code`/`primary_currency_code` (`NOT NULL` + CHECK regex, migration 014). Tombstone writes `name = 'purged-' || id::text` (unique sentinel), NULLs only `address_line1`/`address_line2`/`city`/`postal_code`, retains country/currency.
- **Purge-worker uses the OWNER DSN** — CronJob `DATABASE_URL` from `secretKeyRef key: url` (owner), NOT `runtime_url` (`thittam_app` can't `DROP SCHEMA`). This is the one component needing DDL rights.
- Schema name for DROP = `"tenant_" + id.String()` (dashes kept), interpolated (Postgres can't parameterize identifiers); safe because the value is a validated `uuid.UUID` (per `pkg/tenantdb` doc). `schemaNameFor` is unexported — replicate the one-liner in iam with a comment; do NOT export it.
- errcheck runs in CI. Deferred `tx.Rollback` needs `//nolint:errcheck` or `defer func(){ _ = tx.Rollback(ctx) }()`.
- Monetary rule N/A. slog, no PII/secrets.
- Commits: Conventional Commits, scopes `iam` / `proto` / `infra`. End every commit message with:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
- Next iam migration number = **019**. sqlc: NOT NULL timestamptz → `time.Time`; nullable → `pgtype.Timestamptz`; nullable uuid → `pgtype.UUID`; NOT NULL text → `string`; nullable text → `pgtype.Text` (verified against existing `Tenant` mapping).

---

### Task 1: Migration 019 — tenant_purge_requests + tombstone columns

**Files:**
- Create: `migrations/iam/019_tenant_purge_requests.up.sql`
- Create: `migrations/iam/019_tenant_purge_requests.down.sql`

**Interfaces:**
- Produces: table `tenant_purge_requests`, `tenants.purged_at` column, `'purged'` in `tenants_status_check`.

- [ ] **Step 1: Write the up migration**

`migrations/iam/019_tenant_purge_requests.up.sql`:

```sql
-- PurgeTenant (#92 Stage 3): two-person approval record for hard-deleting a
-- purge_eligible tenant, plus the tombstone column on tenants.

CREATE TABLE tenant_purge_requests (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      UUID        NOT NULL,
    status         TEXT        NOT NULL DEFAULT 'pending'
                               CHECK (status IN ('pending','approved','executed','failed','cancelled')),
    requested_by   UUID        NOT NULL,
    requested_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    request_reason TEXT        NOT NULL,
    approved_by    UUID,
    approved_at    TIMESTAMPTZ,
    cancelled_by   UUID,
    cancelled_at   TIMESTAMPTZ,
    executed_at    TIMESTAMPTZ,
    failure_reason TEXT,
    -- Forensic snapshot captured at request time (the tombstone overwrites the
    -- live tenant name with a sentinel).
    tenant_name    TEXT        NOT NULL,
    tenant_slug    TEXT        NOT NULL
);

-- At most one OPEN (pending|approved) request per tenant.
CREATE UNIQUE INDEX tenant_purge_requests_one_open
    ON tenant_purge_requests (tenant_id)
    WHERE status IN ('pending', 'approved');

-- Worker poll: approved requests, oldest first.
CREATE INDEX idx_tenant_purge_requests_approved
    ON tenant_purge_requests (approved_at)
    WHERE status = 'approved';

-- Tombstone marker on the retained tenants row.
ALTER TABLE tenants ADD COLUMN purged_at TIMESTAMPTZ;

-- Terminal 'purged' state.
ALTER TABLE tenants DROP CONSTRAINT tenants_status_check;
ALTER TABLE tenants
    ADD CONSTRAINT tenants_status_check
        CHECK (status IN ('active','suspended','grace','deactivated','purge_eligible','purged'));
```

- [ ] **Step 2: Write the down migration**

`migrations/iam/019_tenant_purge_requests.down.sql`:

```sql
ALTER TABLE tenants DROP CONSTRAINT tenants_status_check;
ALTER TABLE tenants
    ADD CONSTRAINT tenants_status_check
        CHECK (status IN ('active','suspended','grace','deactivated','purge_eligible'));

ALTER TABLE tenants DROP COLUMN IF EXISTS purged_at;

DROP INDEX IF EXISTS idx_tenant_purge_requests_approved;
DROP INDEX IF EXISTS tenant_purge_requests_one_open;
DROP TABLE IF EXISTS tenant_purge_requests;
```

- [ ] **Step 3: Verify up + down apply cleanly**

Run (mirrors CI "Migration Validate (up + down)"):
`make migrate-all && make migrate-down 2>&1 | tail -20` — or if a scoped target is unavailable, apply the single file pair with the migrate CLI against a scratch DB.
Expected: up creates the table/column/constraint; down reverses to the prior schema with no error. If no local DB, note it and rely on CI.

- [ ] **Step 4: Commit**

```bash
git add migrations/iam/019_tenant_purge_requests.up.sql migrations/iam/019_tenant_purge_requests.down.sql
git commit -m "feat(iam): migration 019 — tenant_purge_requests + tombstone columns (#92)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Domain model + request-record repository (CRUD)

**Files:**
- Modify: `services/iam/models.go` (new `TenantPurgeRequest` struct + status constants)
- Modify: `services/iam/db/queries.sql` (6 CRUD queries)
- Regenerate: `services/iam/db/queries.sql.go` (`sqlc generate`)
- Modify: `services/iam/repository.go` (6 interface methods)
- Modify: `services/iam/db/postgres.go` (6 impls + `dbPurgeRequestToDomain` + nullable-uuid helpers if absent)
- Modify: `services/iam/service_test.go` (`mockRepo` fields + methods)
- Modify: `e2e/critical_path/helpers_test.go` (`iamRepo` methods)

**Interfaces:**
- Produces domain `iam.TenantPurgeRequest` + `PurgeRequest*` status consts.
- Produces `Repository` methods: `CreateTenantPurgeRequest(ctx, *TenantPurgeRequest) error`; `GetOpenTenantPurgeRequest(ctx, tenantID uuid.UUID) (*TenantPurgeRequest, error)`; `ApproveTenantPurgeRequest(ctx, requestID, approverID uuid.UUID) (*TenantPurgeRequest, error)`; `CancelTenantPurgeRequest(ctx, requestID, cancellerID uuid.UUID) (*TenantPurgeRequest, error)`; `ListApprovedTenantPurgeRequests(ctx, limit int) ([]*TenantPurgeRequest, error)`; `MarkTenantPurgeRequestFailed(ctx, requestID uuid.UUID, reason string) (*TenantPurgeRequest, error)`.
- These are consumed by Task 4 (service) and Task 6 (worker).

- [ ] **Step 1: Domain model + status constants** in `services/iam/models.go`:

```go
// TenantPurgeRequest is the persisted two-person-approval record for a
// PurgeTenant operation (#92 Stage 3). One OPEN (pending|approved) request may
// exist per tenant at a time.
type TenantPurgeRequest struct {
	ID            uuid.UUID  `json:"id"`
	TenantID      uuid.UUID  `json:"tenant_id"`
	Status        string     `json:"status"` // pending|approved|executed|failed|cancelled
	RequestedBy   uuid.UUID  `json:"requested_by"`
	RequestedAt   time.Time  `json:"requested_at"`
	RequestReason string     `json:"request_reason"`
	ApprovedBy    *uuid.UUID `json:"approved_by,omitempty"`
	ApprovedAt    *time.Time `json:"approved_at,omitempty"`
	CancelledBy   *uuid.UUID `json:"cancelled_by,omitempty"`
	CancelledAt   *time.Time `json:"cancelled_at,omitempty"`
	ExecutedAt    *time.Time `json:"executed_at,omitempty"`
	FailureReason *string    `json:"failure_reason,omitempty"`
	// Forensic snapshot (tombstone overwrites the live tenant name).
	TenantName string `json:"tenant_name"`
	TenantSlug string `json:"tenant_slug"`
}

const (
	PurgeRequestPending   = "pending"
	PurgeRequestApproved  = "approved"
	PurgeRequestExecuted  = "executed"
	PurgeRequestFailed    = "failed"
	PurgeRequestCancelled = "cancelled"
)
```

- [ ] **Step 2: sqlc queries** in `services/iam/db/queries.sql`:

```sql
-- name: CreateTenantPurgeRequest :one
INSERT INTO tenant_purge_requests (
    id, tenant_id, status, requested_by, request_reason, tenant_name, tenant_slug
) VALUES ($1, $2, 'pending', $3, $4, $5, $6)
RETURNING *;

-- name: GetOpenTenantPurgeRequest :one
SELECT * FROM tenant_purge_requests
WHERE tenant_id = $1 AND status IN ('pending', 'approved')
LIMIT 1;

-- name: ApproveTenantPurgeRequest :one
UPDATE tenant_purge_requests
   SET status = 'approved', approved_by = $2, approved_at = now()
 WHERE id = $1 AND status = 'pending'
RETURNING *;

-- name: CancelTenantPurgeRequest :one
UPDATE tenant_purge_requests
   SET status = 'cancelled', cancelled_by = $2, cancelled_at = now()
 WHERE id = $1 AND status IN ('pending', 'approved')
RETURNING *;

-- name: ListApprovedTenantPurgeRequests :many
SELECT * FROM tenant_purge_requests
 WHERE status = 'approved'
 ORDER BY approved_at
 LIMIT $1;

-- name: MarkTenantPurgeRequestFailed :one
UPDATE tenant_purge_requests
   SET status = 'failed', failure_reason = $2
 WHERE id = $1
RETURNING *;
```

- [ ] **Step 3: Regenerate sqlc**

Run: `sqlc generate`
Expected: `queries.sql.go` gains a `TenantPurgeRequest` row struct and the 6 methods + param structs. No error.

- [ ] **Step 4: Interface methods** — add to `services/iam/repository.go` (in the `// Tenants` section):

```go
	// --- Tenant purge (two-person approval, #92 Stage 3) ---
	CreateTenantPurgeRequest(ctx context.Context, req *TenantPurgeRequest) error
	GetOpenTenantPurgeRequest(ctx context.Context, tenantID uuid.UUID) (*TenantPurgeRequest, error)
	ApproveTenantPurgeRequest(ctx context.Context, requestID, approverID uuid.UUID) (*TenantPurgeRequest, error)
	CancelTenantPurgeRequest(ctx context.Context, requestID, cancellerID uuid.UUID) (*TenantPurgeRequest, error)
	ListApprovedTenantPurgeRequests(ctx context.Context, limit int) ([]*TenantPurgeRequest, error)
	MarkTenantPurgeRequestFailed(ctx context.Context, requestID uuid.UUID, reason string) (*TenantPurgeRequest, error)
```

- [ ] **Step 5: `*Postgres` impls + mapper** in `services/iam/db/postgres.go`. Add the mapper and methods; mirror `CreateTenant`/`GetTenant` structure. Use the generated `*Params` structs (field names are PascalCase of columns).

```go
func dbPurgeRequestToDomain(r TenantPurgeRequest) *iam.TenantPurgeRequest {
	return &iam.TenantPurgeRequest{
		ID:            r.ID,
		TenantID:      r.TenantID,
		Status:        r.Status,
		RequestedBy:   r.RequestedBy,
		RequestedAt:   r.RequestedAt,
		RequestReason: r.RequestReason,
		ApprovedBy:    pgUUIDToPtr(r.ApprovedBy),
		ApprovedAt:    pgTimestamptzToTimePtr(r.ApprovedAt),
		CancelledBy:   pgUUIDToPtr(r.CancelledBy),
		CancelledAt:   pgTimestamptzToTimePtr(r.CancelledAt),
		ExecutedAt:    pgTimestamptzToTimePtr(r.ExecutedAt),
		FailureReason: pgTextToStringPtr(r.FailureReason),
		TenantName:    r.TenantName,
		TenantSlug:    r.TenantSlug,
	}
}

// pgUUIDToPtr / pgUUIDFromPtr: add these helpers next to the existing pgText*/
// pgTimestamptz* helpers IF they don't already exist in this file.
func pgUUIDToPtr(u pgtype.UUID) *uuid.UUID {
	if !u.Valid {
		return nil
	}
	id := uuid.UUID(u.Bytes)
	return &id
}
func pgUUIDFromPtr(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}
```

Methods (each maps `pgx.ErrNoRows` appropriately — see notes):

```go
func (p *Postgres) CreateTenantPurgeRequest(ctx context.Context, req *iam.TenantPurgeRequest) error {
	row, err := p.q.CreateTenantPurgeRequest(ctx, CreateTenantPurgeRequestParams{
		ID:            req.ID,
		TenantID:      req.TenantID,
		RequestedBy:   req.RequestedBy,
		RequestReason: req.RequestReason,
		TenantName:    req.TenantName,
		TenantSlug:    req.TenantSlug,
	})
	if err != nil {
		if isUniqueViolationOn(err, "tenant_purge_requests_one_open") {
			return iam.ErrPurgeRequestExists
		}
		return fmt.Errorf("iam/db: create purge request: %w", err)
	}
	*req = *dbPurgeRequestToDomain(row)
	return nil
}

func (p *Postgres) GetOpenTenantPurgeRequest(ctx context.Context, tenantID uuid.UUID) (*iam.TenantPurgeRequest, error) {
	row, err := p.q.GetOpenTenantPurgeRequest(ctx, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, iam.ErrPurgeRequestNotFound
		}
		return nil, fmt.Errorf("iam/db: get open purge request: %w", err)
	}
	return dbPurgeRequestToDomain(row), nil
}

func (p *Postgres) ApproveTenantPurgeRequest(ctx context.Context, requestID, approverID uuid.UUID) (*iam.TenantPurgeRequest, error) {
	row, err := p.q.ApproveTenantPurgeRequest(ctx, ApproveTenantPurgeRequestParams{
		ID:         requestID,
		ApprovedBy: pgUUIDFromPtr(&approverID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, iam.ErrPurgeRequestNotFound // no longer pending
		}
		return nil, fmt.Errorf("iam/db: approve purge request: %w", err)
	}
	return dbPurgeRequestToDomain(row), nil
}

func (p *Postgres) CancelTenantPurgeRequest(ctx context.Context, requestID, cancellerID uuid.UUID) (*iam.TenantPurgeRequest, error) {
	row, err := p.q.CancelTenantPurgeRequest(ctx, CancelTenantPurgeRequestParams{
		ID:          requestID,
		CancelledBy: pgUUIDFromPtr(&cancellerID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, iam.ErrPurgeRequestNotFound
		}
		return nil, fmt.Errorf("iam/db: cancel purge request: %w", err)
	}
	return dbPurgeRequestToDomain(row), nil
}

func (p *Postgres) ListApprovedTenantPurgeRequests(ctx context.Context, limit int) ([]*iam.TenantPurgeRequest, error) {
	rows, err := p.q.ListApprovedTenantPurgeRequests(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("iam/db: list approved purge requests: %w", err)
	}
	out := make([]*iam.TenantPurgeRequest, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbPurgeRequestToDomain(r))
	}
	return out, nil
}

func (p *Postgres) MarkTenantPurgeRequestFailed(ctx context.Context, requestID uuid.UUID, reason string) (*iam.TenantPurgeRequest, error) {
	row, err := p.q.MarkTenantPurgeRequestFailed(ctx, MarkTenantPurgeRequestFailedParams{
		ID:            requestID,
		FailureReason: pgTextFromString(reason),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, iam.ErrPurgeRequestNotFound
		}
		return nil, fmt.Errorf("iam/db: mark purge request failed: %w", err)
	}
	return dbPurgeRequestToDomain(row), nil
}
```

Note: `ListApprovedTenantPurgeRequests` param is `int32` if sqlc types `LIMIT $1` as int32 — confirm from the generated signature and cast accordingly.

- [ ] **Step 6: Add the methods to both test doubles**

In `services/iam/service_test.go` `mockRepo`: add 6 function-pointer fields (`createTenantPurgeRequestFn`, `getOpenTenantPurgeRequestFn`, `approveTenantPurgeRequestFn`, `cancelTenantPurgeRequestFn`, `listApprovedTenantPurgeRequestsFn`, `markTenantPurgeRequestFailedFn`) and 6 methods that call the hook or return a benign default (mirror the existing `clearTenantLegalHoldFn` pattern). Example:

```go
func (m *mockRepo) CreateTenantPurgeRequest(ctx context.Context, req *TenantPurgeRequest) error {
	if m.createTenantPurgeRequestFn != nil {
		return m.createTenantPurgeRequestFn(ctx, req)
	}
	return nil
}
func (m *mockRepo) GetOpenTenantPurgeRequest(ctx context.Context, tenantID uuid.UUID) (*TenantPurgeRequest, error) {
	if m.getOpenTenantPurgeRequestFn != nil {
		return m.getOpenTenantPurgeRequestFn(ctx, tenantID)
	}
	return nil, ErrPurgeRequestNotFound
}
// ...and the remaining four, each delegating to its Fn or returning a benign default.
```

In `e2e/critical_path/helpers_test.go` `iamRepo`: add 6 minimal map-backed methods (or ones returning `ErrPurgeRequestNotFound`/`nil`) so it still satisfies the widened interface. The e2e path doesn't exercise purge, so trivial stubs suffice:

```go
func (r *iamRepo) CreateTenantPurgeRequest(_ context.Context, _ *iam.TenantPurgeRequest) error { return nil }
func (r *iamRepo) GetOpenTenantPurgeRequest(_ context.Context, _ uuid.UUID) (*iam.TenantPurgeRequest, error) {
	return nil, iam.ErrPurgeRequestNotFound
}
func (r *iamRepo) ApproveTenantPurgeRequest(_ context.Context, _, _ uuid.UUID) (*iam.TenantPurgeRequest, error) {
	return nil, iam.ErrPurgeRequestNotFound
}
func (r *iamRepo) CancelTenantPurgeRequest(_ context.Context, _, _ uuid.UUID) (*iam.TenantPurgeRequest, error) {
	return nil, iam.ErrPurgeRequestNotFound
}
func (r *iamRepo) ListApprovedTenantPurgeRequests(_ context.Context, _ int) ([]*iam.TenantPurgeRequest, error) {
	return nil, nil
}
func (r *iamRepo) MarkTenantPurgeRequestFailed(_ context.Context, _ uuid.UUID, _ string) (*iam.TenantPurgeRequest, error) {
	return nil, iam.ErrPurgeRequestNotFound
}
```

(These reference `iam.ErrPurgeRequestNotFound`, added in Task 4. To keep Task 2 self-compiling, add the four purge sentinels to `services/iam/errors.go` HERE in Task 2 — see the block below — rather than deferring to Task 4.)

Add to `services/iam/errors.go` now (Task 4's service consumes them; the doubles need them to compile):

```go
	// PurgeTenant (#92 Stage 3).
	ErrTenantNotPurgeable   = errors.New("iam: tenant is not purge_eligible")
	ErrPurgeRequestExists   = errors.New("iam: an open purge request already exists for this tenant")
	ErrPurgeRequestNotFound = errors.New("iam: no open purge request for this tenant")
	ErrSelfApproval         = errors.New("iam: purge approver must differ from the requester")
```

- [ ] **Step 7: Build + vet the whole tree**

Run: `go build ./... && go vet ./...`
Expected: clean. (Confirms all three implementers satisfy the widened interface — especially the e2e `iamRepo`.)

- [ ] **Step 8: Commit**

```bash
git add services/iam/models.go services/iam/db/queries.sql services/iam/db/queries.sql.go \
        services/iam/repository.go services/iam/db/postgres.go services/iam/errors.go \
        services/iam/service_test.go e2e/critical_path/helpers_test.go
git commit -m "feat(iam): tenant_purge_requests domain model + repo CRUD (#92)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Destructive repo method — `PurgeTenantSchemaAndTombstone`

**Files:**
- Modify: `services/iam/repository.go` (1 interface method)
- Modify: `services/iam/db/postgres.go` (raw-DDL tx impl)
- Modify: `services/iam/service_test.go` (`mockRepo` field + method)
- Modify: `e2e/critical_path/helpers_test.go` (`iamRepo` stub)
- Create: `services/iam/db/tenant_purge_integration_test.go` (real-Postgres DDL test)

**Interfaces:**
- Produces `Repository.PurgeTenantSchemaAndTombstone(ctx, tenantID, requestID uuid.UUID) error` — one transaction: `DROP SCHEMA IF EXISTS "tenant_<uuid>" CASCADE` + status-guarded tombstone of the `tenants` row + mark the request `executed`. Returns `ErrTenantNotPurgeable` if the tenant is no longer `purge_eligible` (0 rows tombstoned → rollback).

- [ ] **Step 1: Interface method** in `services/iam/repository.go` (after the purge CRUD methods):

```go
	// PurgeTenantSchemaAndTombstone hard-deletes a purge_eligible tenant in one
	// transaction: DROP SCHEMA tenant_<uuid> CASCADE, tombstone the tenants row
	// (status='purged', PII nulled, name→sentinel, purged_at=now()), and mark
	// the purge request executed. Owner privileges required (DDL). Returns
	// ErrTenantNotPurgeable if the tenant left purge_eligible (status-guarded).
	PurgeTenantSchemaAndTombstone(ctx context.Context, tenantID, requestID uuid.UUID) error
```

- [ ] **Step 2: Write the failing integration test**

`services/iam/db/tenant_purge_integration_test.go` (mirror `tenant_legal_hold_integration_test.go`: `//go:build integration`, `testdb.Open`/`NewTx`, raw `tx.Exec`). Because the repo method opens its own pool transaction (incompatible with the outer test tx), this test exercises the **exact SQL the method runs** through the test tx, proving the DDL + tombstone + audit-survival semantics:

```go
//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/testdb"
)

func TestPurgeTenant_SQL_DropsSchema_Tombstones_PreservesAudit(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool) // owner role; DDL is transactional → auto rollback
	ctx := context.Background()

	id := uuid.New()
	schema := "tenant_" + id.String()

	// Seed: a purge_eligible tenant, its schema, and an audit row.
	_, err := tx.Exec(ctx, `INSERT INTO tenants (id, name, slug, country_code, primary_currency_code, status, deactivated_at)
		VALUES ($1, $2, $3, 'US', 'USD', 'purge_eligible', now() - INTERVAL '200 days')`,
		id, "Doomed Studios", "slug-"+id.String()[:8])
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `CREATE SCHEMA "`+schema+`"`)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `INSERT INTO audit_log (id, tenant_id, actor_id, actor_email, action, resource_type, resource_id, occurred_at)
		VALUES (gen_random_uuid(), $1, $1, 'a@b.c', 'tenant_purged', 'tenant', $1, now())`, id)
	require.NoError(t, err)

	// Act: the exact statements PurgeTenantSchemaAndTombstone runs.
	_, err = tx.Exec(ctx, `DROP SCHEMA IF EXISTS "`+schema+`" CASCADE`)
	require.NoError(t, err)
	ct, err := tx.Exec(ctx, `UPDATE tenants SET status='purged',
		name = 'purged-' || id::text, address_line1=NULL, address_line2=NULL, city=NULL, postal_code=NULL,
		purged_at=now() WHERE id=$1 AND status='purge_eligible'`, id)
	require.NoError(t, err)
	require.Equal(t, int64(1), ct.RowsAffected(), "exactly one tenant tombstoned")

	// Assert: schema gone.
	var n int
	require.NoError(t, tx.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.schemata WHERE schema_name=$1`, schema).Scan(&n))
	assert.Equal(t, 0, n, "tenant schema must be dropped")

	// Assert: row tombstoned.
	var status, name string
	require.NoError(t, tx.QueryRow(ctx, `SELECT status, name FROM tenants WHERE id=$1`, id).Scan(&status, &name))
	assert.Equal(t, "purged", status)
	assert.Equal(t, "purged-"+id.String(), name)

	// Assert: audit row SURVIVES the drop (shared public schema).
	require.NoError(t, tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id=$1`, id).Scan(&n))
	assert.Equal(t, 1, n, "audit_log row must survive the schema drop")
}
```

- [ ] **Step 3: Run the integration test to see it fail / skip**

Run: `go test ./services/iam/db/ -tags=integration -run TestPurgeTenant_SQL -v`
Expected: passes if a test DB is bootstrapped, or SKIPs (`THITTAM_TEST_DSN` unset). If it runs, it validates the SQL semantics up front. (This test does not call the Go method — it locks the SQL contract the method must implement.)

- [ ] **Step 4: Implement the repo method** in `services/iam/db/postgres.go` (ledger tx pattern; raw DDL):

```go
func (p *Postgres) PurgeTenantSchemaAndTombstone(ctx context.Context, tenantID, requestID uuid.UUID) error {
	// Schema name = tenant_<uuid> (dashes kept). Interpolated because Postgres
	// cannot parameterize identifiers; safe because tenantID is a validated
	// uuid.UUID (see pkg/tenantdb doc: uuid.String() yields only hex+hyphens).
	schema := "tenant_" + tenantID.String()

	tx, err := p.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("iam/db: purge begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DROP SCHEMA IF EXISTS "`+schema+`" CASCADE`); err != nil {
		return fmt.Errorf("iam/db: drop schema %s: %w", schema, err)
	}

	ct, err := tx.Exec(ctx, `UPDATE tenants SET status='purged',
		name = 'purged-' || id::text,
		address_line1=NULL, address_line2=NULL, city=NULL, postal_code=NULL,
		purged_at=now()
	 WHERE id=$1 AND status='purge_eligible'`, tenantID)
	if err != nil {
		return fmt.Errorf("iam/db: tombstone tenant %s: %w", tenantID, err)
	}
	if ct.RowsAffected() == 0 {
		// Tenant left purge_eligible since approval (race / manual change). The
		// schema drop rolls back with the tx — nothing is destroyed.
		return iam.ErrTenantNotPurgeable
	}

	if _, err := tx.Exec(ctx,
		`UPDATE tenant_purge_requests SET status='executed', executed_at=now() WHERE id=$1`, requestID); err != nil {
		return fmt.Errorf("iam/db: mark purge request executed: %w", err)
	}

	return tx.Commit(ctx)
}
```

- [ ] **Step 5: Add the method to both test doubles**

`mockRepo` (service_test.go): field `purgeTenantSchemaAndTombstoneFn func(ctx context.Context, tenantID, requestID uuid.UUID) error` + method delegating or returning `nil`.
`iamRepo` (e2e helpers_test.go):

```go
func (r *iamRepo) PurgeTenantSchemaAndTombstone(_ context.Context, _, _ uuid.UUID) error { return nil }
```

- [ ] **Step 6: Build + vet whole tree**

Run: `go build ./... && go vet ./...`
Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add services/iam/repository.go services/iam/db/postgres.go services/iam/service_test.go \
        e2e/critical_path/helpers_test.go services/iam/db/tenant_purge_integration_test.go
git commit -m "feat(iam): PurgeTenantSchemaAndTombstone destructive repo tx (#92)

DROP SCHEMA tenant_<uuid> CASCADE + status-guarded tombstone + mark request
executed, in one owner-privileged transaction. Integration test locks the SQL
contract (schema dropped, row tombstoned, audit_log survives).

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Service layer + audit actions

**Files:**
- Create: `services/iam/purge.go`
- Modify: `pkg/audit/types.go` (4 action constants)
- Modify: `services/iam/lifecycle.go` (system-actor const) — or put it in purge.go
- Test: `services/iam/purge_test.go`

**Interfaces:**
- Consumes: the Task 2/3 repo methods; `audit.ActorFromContext`, `audit.Event`, `mustMarshalStatus`-style helpers; `TenantStatusPurgeEligible`.
- Produces: `(*Service)` methods `RequestTenantPurge(ctx, tenantID uuid.UUID, reason string) (*TenantPurgeRequest, error)`; `ApproveTenantPurge(ctx, tenantID uuid.UUID, reason string) (*TenantPurgeRequest, error)`; `CancelTenantPurge(ctx, tenantID uuid.UUID, reason string) (*TenantPurgeRequest, error)`; `PurgeApprovedTenant(ctx, req *TenantPurgeRequest) error` (worker-facing).
- Produces audit consts `ActionPurgeRequested`, `ActionPurgeApproved`, `ActionPurgeCancelled`, `ActionTenantPurged`; system actor `SystemActorPurgeWorker`.

- [ ] **Step 1: Audit action constants** in `pkg/audit/types.go` (next to `ActionLegalHoldApplied`):

```go
	ActionPurgeRequested Action = "purge_requested"
	ActionPurgeApproved  Action = "purge_approved"
	ActionPurgeCancelled Action = "purge_cancelled"
	ActionTenantPurged   Action = "tenant_purged"
```

- [ ] **Step 2: Write the failing service tests** in `services/iam/purge_test.go`. Cover: request rejects non-`purge_eligible`; request duplicate → `ErrPurgeRequestExists`; happy request; approve rejects self-approval; approve rejects when tenant left `purge_eligible`; happy approve; cancel a pending request; `PurgeApprovedTenant` success emits `ActionTenantPurged`; `PurgeApprovedTenant` failure marks the request failed. Use `mockRepo` hooks + `memoryAuditStore`. Representative cases:

```go
package iam

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/audit"
)

func TestRequestTenantPurge_RejectsNonPurgeEligible(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: TenantStatusDeactivated}, nil
		},
		createTenantPurgeRequestFn: func(_ context.Context, _ *TenantPurgeRequest) error {
			t.Fatal("must not create a request for a non-purge_eligible tenant")
			return nil
		},
	}
	_, err := newTestService(repo).RequestTenantPurge(context.Background(), fixedTenantID, "gdpr erasure")
	assert.ErrorIs(t, err, ErrTenantNotPurgeable)
}

func TestApproveTenantPurge_RejectsSelfApproval(t *testing.T) {
	t.Parallel()
	requester := uuid.New()
	repo := &mockRepo{
		getOpenTenantPurgeRequestFn: func(_ context.Context, tid uuid.UUID) (*TenantPurgeRequest, error) {
			return &TenantPurgeRequest{ID: uuid.New(), TenantID: tid, Status: PurgeRequestPending, RequestedBy: requester}, nil
		},
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: TenantStatusPurgeEligible}, nil
		},
		approveTenantPurgeRequestFn: func(_ context.Context, _, _ uuid.UUID) (*TenantPurgeRequest, error) {
			t.Fatal("must not approve when approver == requester")
			return nil, nil
		},
	}
	ctx := audit.WithActor(context.Background(), audit.ActorInfo{UserID: requester, Email: "a@b.c"})
	_, err := newTestService(repo).ApproveTenantPurge(ctx, fixedTenantID, "ok")
	assert.ErrorIs(t, err, ErrSelfApproval)
}

func TestPurgeApprovedTenant_Success_EmitsAudit(t *testing.T) {
	req := &TenantPurgeRequest{ID: uuid.New(), TenantID: fixedTenantID, Status: PurgeRequestApproved, TenantName: "Doomed"}
	repo := &mockRepo{
		purgeTenantSchemaAndTombstoneFn: func(_ context.Context, _, _ uuid.UUID) error { return nil },
	}
	store := &memoryAuditStore{}
	logger := audit.NewLogger(store, audit.LoggerConfig{BufferSize: 10, FlushInterval: 10 * 1e6, BatchSize: 10}, nil)
	svc := newTestService(repo).WithAuditLogger(logger)

	err := svc.PurgeApprovedTenant(context.Background(), req)
	require.NoError(t, err)

	flushCtx, cancel := context.WithTimeout(context.Background(), 1e9)
	defer cancel()
	require.NoError(t, logger.Close(flushCtx))
	events := store.snapshot()
	require.Len(t, events, 1)
	assert.Equal(t, audit.ActionTenantPurged, events[0].Action)
	assert.Equal(t, fixedTenantID, events[0].TenantID)
}

func TestPurgeApprovedTenant_Failure_MarksFailed(t *testing.T) {
	req := &TenantPurgeRequest{ID: uuid.New(), TenantID: fixedTenantID, Status: PurgeRequestApproved}
	var markedReason string
	repo := &mockRepo{
		purgeTenantSchemaAndTombstoneFn: func(_ context.Context, _, _ uuid.UUID) error { return ErrTenantNotPurgeable },
		markTenantPurgeRequestFailedFn: func(_ context.Context, _ uuid.UUID, reason string) (*TenantPurgeRequest, error) {
			markedReason = reason
			return req, nil
		},
	}
	err := newTestService(repo).PurgeApprovedTenant(context.Background(), req)
	require.Error(t, err)
	assert.NotEmpty(t, markedReason, "failure reason must be recorded on the request")
}
```

(Add the remaining cases — duplicate-request, happy request, approve-when-not-purge_eligible, cancel — following the same shape. Note: `1e6`/`1e9` above are `time.Duration` nanoseconds; use `10*time.Millisecond` / `time.Second` with a `time` import in the real file.)

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./services/iam/ -run 'TestRequestTenantPurge|TestApproveTenantPurge|TestCancelTenantPurge|TestPurgeApprovedTenant' -v`
Expected: FAIL — methods undefined.

- [ ] **Step 4: Implement the service** in `services/iam/purge.go`:

```go
package iam

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/wegofwd2020/thittam/pkg/audit"
)

// SystemActorPurgeWorker is the audit actor for worker-executed purges (mirrors
// SystemActorRetentionSweeper).
const SystemActorPurgeWorker = "system:purge-worker"

// RequestTenantPurge opens a pending two-person purge request for a
// purge_eligible tenant (#92 Stage 3). The caller is recorded as requested_by.
func (s *Service) RequestTenantPurge(ctx context.Context, tenantID uuid.UUID, reason string) (*TenantPurgeRequest, error) {
	if reason == "" {
		return nil, fmt.Errorf("iam: request tenant purge %s: %w", tenantID, errEmptyReason)
	}
	tenant, err := s.repo.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("iam: request tenant purge %s: %w", tenantID, err)
	}
	if tenant.Status != TenantStatusPurgeEligible {
		return nil, fmt.Errorf("iam: request tenant purge %s (status %s): %w", tenantID, tenant.Status, ErrTenantNotPurgeable)
	}
	actor, _ := audit.ActorFromContext(ctx)
	req := &TenantPurgeRequest{
		ID:            uuid.New(),
		TenantID:      tenantID,
		Status:        PurgeRequestPending,
		RequestedBy:   actor.UserID,
		RequestReason: reason,
		TenantName:    tenant.Name,
		TenantSlug:    tenant.Slug,
	}
	if err := s.repo.CreateTenantPurgeRequest(ctx, req); err != nil {
		return nil, fmt.Errorf("iam: request tenant purge %s: %w", tenantID, err)
	}
	s.auditPurge(ctx, audit.ActionPurgeRequested, tenantID, actor, reason)
	return req, nil
}

// ApproveTenantPurge approves the open pending request for a tenant. The
// approver MUST differ from the requester (two-person control) and the tenant
// must still be purge_eligible. No deletion happens here — the worker executes.
func (s *Service) ApproveTenantPurge(ctx context.Context, tenantID uuid.UUID, reason string) (*TenantPurgeRequest, error) {
	open, err := s.repo.GetOpenTenantPurgeRequest(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("iam: approve tenant purge %s: %w", tenantID, err)
	}
	if open.Status != PurgeRequestPending {
		return nil, fmt.Errorf("iam: approve tenant purge %s: %w", tenantID, ErrPurgeRequestNotFound)
	}
	actor, _ := audit.ActorFromContext(ctx)
	if actor.UserID == open.RequestedBy {
		return nil, fmt.Errorf("iam: approve tenant purge %s: %w", tenantID, ErrSelfApproval)
	}
	tenant, err := s.repo.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("iam: approve tenant purge %s: %w", tenantID, err)
	}
	if tenant.Status != TenantStatusPurgeEligible {
		return nil, fmt.Errorf("iam: approve tenant purge %s (status %s): %w", tenantID, tenant.Status, ErrTenantNotPurgeable)
	}
	approved, err := s.repo.ApproveTenantPurgeRequest(ctx, open.ID, actor.UserID)
	if err != nil {
		return nil, fmt.Errorf("iam: approve tenant purge %s: %w", tenantID, err)
	}
	s.auditPurge(ctx, audit.ActionPurgeApproved, tenantID, actor, reason)
	return approved, nil
}

// CancelTenantPurge cancels the open (pending or approved) request for a tenant
// — a safety valve for the window before the worker executes.
func (s *Service) CancelTenantPurge(ctx context.Context, tenantID uuid.UUID, reason string) (*TenantPurgeRequest, error) {
	open, err := s.repo.GetOpenTenantPurgeRequest(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("iam: cancel tenant purge %s: %w", tenantID, err)
	}
	actor, _ := audit.ActorFromContext(ctx)
	cancelled, err := s.repo.CancelTenantPurgeRequest(ctx, open.ID, actor.UserID)
	if err != nil {
		return nil, fmt.Errorf("iam: cancel tenant purge %s: %w", tenantID, err)
	}
	s.auditPurge(ctx, audit.ActionPurgeCancelled, tenantID, actor, reason)
	return cancelled, nil
}

// PurgeApprovedTenant executes an approved purge (called by the purge-worker).
// On success emits ActionTenantPurged (system actor); on failure records the
// reason on the request so the next run can retry.
func (s *Service) PurgeApprovedTenant(ctx context.Context, req *TenantPurgeRequest) error {
	if err := s.repo.PurgeTenantSchemaAndTombstone(ctx, req.TenantID, req.ID); err != nil {
		if _, ferr := s.repo.MarkTenantPurgeRequestFailed(ctx, req.ID, err.Error()); ferr != nil {
			return fmt.Errorf("iam: purge %s failed (%v) and mark-failed errored: %w", req.TenantID, err, ferr)
		}
		return fmt.Errorf("iam: purge %s: %w", req.TenantID, err)
	}
	if s.audit != nil {
		s.audit.Log(audit.Event{
			TenantID:     req.TenantID,
			ActorEmail:   SystemActorPurgeWorker,
			Action:       audit.ActionTenantPurged,
			ResourceType: audit.ResourceTenant,
			ResourceID:   req.TenantID,
			OccurredAt:   time.Now().UTC(),
		})
	}
	return nil
}

// auditPurge emits a purge-lifecycle audit event with the ctx actor.
func (s *Service) auditPurge(ctx context.Context, action audit.Action, tenantID uuid.UUID, actor audit.ActorInfo, reason string) {
	if s.audit == nil {
		return
	}
	s.audit.Log(audit.Event{
		TenantID:     tenantID,
		ActorID:      actor.UserID,
		ActorEmail:   actor.Email,
		ActorIP:      actor.IP,
		Action:       action,
		ResourceType: audit.ResourceTenant,
		ResourceID:   tenantID,
		Metadata:     mustMarshalClearReason(reason), // {"reason": "..."} — reuse existing helper
		OccurredAt:   time.Now().UTC(),
	})
}
```

Note: `errEmptyReason` — if no such sentinel exists, either add one (`ErrReasonRequired`, mapped to `InvalidArgument`) or enforce empty-reason in the handler (Task 5) and drop this check. Prefer enforcing in the handler like `SetTenantRetention` does; if so, remove the `reason == ""` guard here. `mustMarshalClearReason` is the existing `{"reason":...}` helper in `lifecycle.go`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./services/iam/ -run 'TestRequestTenantPurge|TestApproveTenantPurge|TestCancelTenantPurge|TestPurgeApprovedTenant' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add services/iam/purge.go services/iam/purge_test.go pkg/audit/types.go
git commit -m "feat(iam): purge service — request/approve/cancel/execute + audit (#92)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Proto + handlers + error mapping

**Files:**
- Modify: `proto/thittam/iam/v1/iam.proto` (3 RPCs + 4 messages)
- Regenerate: `gen/iam/v1/*` (`buf generate`)
- Modify: `services/iam/handler.go` (3 handlers + `grpcError` cases)
- Test: `services/iam/handler_test.go`

**Interfaces:**
- Consumes: the Task 4 service methods + the sentinels.
- Produces: `RequestTenantPurge`/`ApproveTenantPurge`/`CancelTenantPurge` RPCs + `TenantPurgeRequest` proto message.

- [ ] **Step 1: Proto** — add to the `// --- Tenants ---` block:

```proto
rpc RequestTenantPurge(RequestTenantPurgeRequest) returns (TenantPurgeRequest);
rpc ApproveTenantPurge(ApproveTenantPurgeRequest) returns (TenantPurgeRequest);
rpc CancelTenantPurge(CancelTenantPurgeRequest) returns (TenantPurgeRequest);
```

and near the other tenant messages:

```proto
message RequestTenantPurgeRequest { string tenant_id = 1; string reason = 2; }
message ApproveTenantPurgeRequest { string tenant_id = 1; string reason = 2; }
message CancelTenantPurgeRequest  { string tenant_id = 1; string reason = 2; }

message TenantPurgeRequest {
  string id = 1;
  string tenant_id = 2;
  string status = 3;
  string requested_by = 4;
  google.protobuf.Timestamp requested_at = 5;
  string request_reason = 6;
  string approved_by = 7;
  google.protobuf.Timestamp approved_at = 8;
  google.protobuf.Timestamp executed_at = 9;
  string failure_reason = 10;
}
```

- [ ] **Step 2: Regenerate + verify additive**

Run (`buf` at `~/go/bin/buf`; pass `proto` as the target dir like the Makefile does):
`buf lint proto && buf generate && buf breaking proto --against '.git#branch=main,subdir=proto'`
Expected: no lint errors; `gen/iam/v1/` gains the messages + `IAMServiceServer` methods; `buf breaking` reports no breaking changes (additive). `go build ./...` stays green (Handler embeds `UnimplementedIAMServiceServer`).

- [ ] **Step 3: Write failing handler tests** in `services/iam/handler_test.go`: success (platform-admin) for each of the three; permission-denied (bare ctx) for one; empty-reason → `InvalidArgument`; not-purge_eligible → `FailedPrecondition` through the full chain; self-approval → `FailedPrecondition`. Example:

```go
func TestHandler_RequestTenantPurge_Success(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	h := NewHandler(newTestService(&mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: "purge_eligible", Name: "X", Slug: "x"}, nil
		},
		createTenantPurgeRequestFn: func(_ context.Context, _ *TenantPurgeRequest) error { return nil },
	}))
	resp, err := h.RequestTenantPurge(platformAdminCtx(), &iamv1.RequestTenantPurgeRequest{Id: tid.String(), Reason: "gdpr"})
	require.NoError(t, err)
	assert.Equal(t, "pending", resp.GetStatus())
}

func TestHandler_RequestTenantPurge_PermissionDenied(t *testing.T) {
	t.Parallel()
	_, err := newHandler().RequestTenantPurge(context.Background(), &iamv1.RequestTenantPurgeRequest{Id: uuid.New().String(), Reason: "x"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestHandler_RequestTenantPurge_NotEligible_FailedPrecondition(t *testing.T) {
	t.Parallel()
	h := NewHandler(newTestService(&mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: "active"}, nil
		},
	}))
	_, err := h.RequestTenantPurge(platformAdminCtx(), &iamv1.RequestTenantPurgeRequest{Id: uuid.New().String(), Reason: "x"})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}
```

Note: the request proto field for the id is `tenant_id` → generated getter `GetTenantId()`; confirm the exact getter name from the generated code and use it (`req.GetTenantId()`), not `GetId()`.

- [ ] **Step 4: Run to verify fail**

Run: `go test ./services/iam/ -run 'TestHandler_.*TenantPurge' -v`
Expected: FAIL — handler methods undefined.

- [ ] **Step 5: Implement handlers + grpcError** in `services/iam/handler.go`. Each mirrors `SetTenantRetention`:

```go
func (h *Handler) RequestTenantPurge(ctx context.Context, req *iamv1.RequestTenantPurgeRequest) (*iamv1.TenantPurgeRequest, error) {
	if err := interceptor.RequireRole(ctx, interceptor.RolePlatformAdmin); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}
	if strings.TrimSpace(req.GetReason()) == "" {
		return nil, status.Error(codes.InvalidArgument, "reason is required")
	}
	pr, err := h.svc.RequestTenantPurge(ctx, id, req.GetReason())
	if err != nil {
		return nil, grpcError(err)
	}
	return purgeRequestToProto(pr), nil
}
// ApproveTenantPurge and CancelTenantPurge follow the same shape, calling
// h.svc.ApproveTenantPurge / h.svc.CancelTenantPurge.
```

Add a `purgeRequestToProto(*TenantPurgeRequest) *iamv1.TenantPurgeRequest` mapper (near `tenantToProto`), converting uuids to strings (empty when nil), `*time.Time` to `timestamppb` (nil-safe). Extend `grpcError`:

```go
	case errors.Is(err, ErrTenantNotPurgeable),
		errors.Is(err, ErrSelfApproval):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, ErrPurgeRequestExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, ErrPurgeRequestNotFound):
		return status.Error(codes.NotFound, err.Error())
```

(If Task 4 kept an `errEmptyReason`/`ErrReasonRequired`, map it to `InvalidArgument`; otherwise the handler's empty-reason check covers it.)

- [ ] **Step 6: Run to verify pass + whole suite + coverage**

Run: `go build ./... && go vet ./... && go test ./services/iam/ -run 'TestHandler_.*TenantPurge' -v && go test ./services/iam/ -short`
Expected: all pass. Then check coverage without the covdata-breaking `-coverprofile` across `db`: `go test ./services/iam/ -short -coverprofile=cover.out && go tool cover -func=cover.out | tail -1` → ≥ 85%.

- [ ] **Step 7: Commit**

```bash
git add proto/thittam/iam/v1/iam.proto gen/iam/v1/ services/iam/handler.go services/iam/handler_test.go
git commit -m "feat(iam): PurgeTenant RPCs — request/approve/cancel handlers (#92)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: purge-worker binary + CronJob

**Files:**
- Create: `cmd/purge-worker/main.go`
- Create: `cmd/purge-worker/metrics.go`
- Create: `cmd/purge-worker/purge_test.go` (worker-loop unit test)
- Create: `infra/k8s/jobs/purge-worker-cronjob.yaml`

**Interfaces:**
- Consumes: `iamdb.NewPostgres`, `iam.NewService(...).WithAuditLogger(...)`, `svc.ListApprovedTenantPurgeRequests`? (no — that's on the repo; the worker uses the repo via the service). Add a thin service passthrough `Service.ListApprovedTenantPurgeRequests(ctx, limit)` OR call `repo` — mirror the sweeper, which calls `svc.ListTenantsDueForLifecycle`. **Add `Service.ListApprovedPurges(ctx, limit) ([]*TenantPurgeRequest, error)`** (one-line passthrough to the repo) so the worker depends only on the service, consistent with the sweeper.

- [ ] **Step 1: Service passthrough** — add to `services/iam/purge.go`:

```go
// ListApprovedPurges returns approved purge requests for the worker to execute.
func (s *Service) ListApprovedPurges(ctx context.Context, limit int) ([]*TenantPurgeRequest, error) {
	return s.repo.ListApprovedTenantPurgeRequests(ctx, limit)
}
```

Add a one-line service unit test asserting it delegates to the repo hook. Commit can be folded into this task.

- [ ] **Step 2: Worker `main.go`** in `cmd/purge-worker/main.go` — mirror `cmd/retention-sweeper/main.go` exactly for env/pool/service/audit/flush/metrics wiring, replacing the sweep loop with a purge loop:

```go
// purge-worker is the batch CLI that executes approved tenant purges (#92
// Stage 3) — the terminal, destructive step of the retention lifecycle. It
// runs as a Kubernetes CronJob (daily) UNDER THE OWNER DSN (DROP SCHEMA needs
// owner privileges; thittam_app cannot). For each approved tenant_purge_request
// it drops tenant_<uuid> and tombstones the tenants row, in one transaction.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wegofwd2020/thittam/pkg/audit"
	"github.com/wegofwd2020/thittam/services/iam"
	iamdb "github.com/wegofwd2020/thittam/services/iam/db"
)

func main() {
	batchSize := flag.Int("batch", 100, "Max approved purge requests to process per run")
	flag.Parse()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "ERROR: DATABASE_URL is required")
		os.Exit(1)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger = logger.With("binary", "purge-worker")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		logger.Error("connect to database", "error", err)
		os.Exit(2)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		logger.Error("ping database", "error", err)
		os.Exit(2)
	}

	repo := iamdb.NewPostgres(pool)
	auditLogger := audit.NewLogger(audit.NewPostgres(pool), audit.DefaultConfig(), nil)
	svc := iam.NewService(repo, nil, nil, nil, nil).WithAuditLogger(auditLogger)

	metrics := newPurgeMetrics()
	err = runPurge(ctx, svc, logger, metrics, *batchSize)

	flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if cerr := auditLogger.Close(flushCtx); cerr != nil {
		logger.Warn("audit flush failed", "error", cerr)
	}
	flushCancel()

	pushMetrics(ctx, logger, os.Getenv("PUSHGATEWAY_URL"), pushInstance(), metrics.registry)

	if err != nil {
		logger.Error("purge run failed", "error", err)
		os.Exit(2)
	}
}

// runPurge processes up to batchSize approved requests. Individual failures are
// recorded on the request (by the service) and counted, not fatal — the run
// only fails on a systemic error (e.g. the list query itself).
func runPurge(ctx context.Context, svc *iam.Service, logger *slog.Logger, m *purgeMetrics, batchSize int) error {
	reqs, err := svc.ListApprovedPurges(ctx, batchSize)
	if err != nil {
		m.runsTotal.WithLabelValues("error").Inc()
		return fmt.Errorf("list approved purges: %w", err)
	}
	for _, req := range reqs {
		if perr := svc.PurgeApprovedTenant(ctx, req); perr != nil {
			logger.Error("purge failed", "tenant_id", req.TenantID, "error", perr)
			m.failuresTotal.Inc()
			continue
		}
		logger.Info("tenant purged", "tenant_id", req.TenantID)
		m.purgedTotal.Inc()
	}
	m.runsTotal.WithLabelValues("success").Inc()
	m.lastSuccessTimestamp.SetToCurrentTime()
	return nil
}
```

- [ ] **Step 3: Worker `metrics.go`** in `cmd/purge-worker/metrics.go` — mirror the sweeper's `metrics.go` (local registry, `pushMetrics`, `pushInstance`), with purge-specific counters:

```go
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
)

type purgeMetrics struct {
	registry *prometheus.Registry

	runsTotal            *prometheus.CounterVec // outcome=success|error
	purgedTotal          prometheus.Counter
	failuresTotal        prometheus.Counter
	lastSuccessTimestamp prometheus.Gauge
}

func newPurgeMetrics() *purgeMetrics {
	reg := prometheus.NewRegistry()
	m := &purgeMetrics{
		registry: reg,
		runsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "thittam_purge_worker_runs_total",
			Help: "Total purge-worker runs, labeled by outcome.",
		}, []string{"outcome"}),
		purgedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "thittam_purge_worker_purged_total",
			Help: "Tenants hard-deleted by the purge worker.",
		}),
		failuresTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "thittam_purge_worker_failures_total",
			Help: "Approved purge requests that failed to execute.",
		}),
		lastSuccessTimestamp: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "thittam_purge_worker_last_success_timestamp_seconds",
			Help: "Unix timestamp of the most recent successful purge run.",
		}),
	}
	reg.MustRegister(m.runsTotal, m.purgedTotal, m.failuresTotal, m.lastSuccessTimestamp)
	return m
}

// pushMetrics / pushInstance: copy verbatim from cmd/retention-sweeper/metrics.go
// (identical Pushgateway push logic), changing the job name to "purge-worker".
func pushMetrics(ctx context.Context, logger *slog.Logger, url, instance string, reg *prometheus.Registry) {
	if url == "" {
		logger.Info("pushgateway URL not set — skipping metrics push")
		return
	}
	pusher := push.New(url, "purge-worker").Gatherer(reg).Grouping("instance", instance)
	if err := pusher.PushContext(ctx); err != nil {
		logger.Warn("pushgateway push failed — continuing", "error", err, "url", url, "instance", instance)
		return
	}
	logger.Info("pushed metrics to pushgateway", "url", url, "instance", instance)
}

func pushInstance() string {
	if p := os.Getenv("POD_NAME"); p != "" {
		return p
	}
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "unknown"
}
```

- [ ] **Step 4: Worker unit test** in `cmd/purge-worker/purge_test.go` — `runPurge` over a fake service is not possible (it takes `*iam.Service` concrete). Instead test the metrics registry (mirror `cmd/retention-sweeper/metrics_test.go`) and, for loop behavior, rely on the service-level `PurgeApprovedTenant` tests from Task 4. Write a metrics test:

```go
package main

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPurgeMetrics_CountersRegistered(t *testing.T) {
	m := newPurgeMetrics()
	m.purgedTotal.Inc()
	m.failuresTotal.Inc()
	m.runsTotal.WithLabelValues("success").Inc()
	if got := testutil.CollectAndCount(m.registry); got == 0 {
		t.Fatal("expected metrics registered on the registry")
	}
	if err := testutil.CollectAndCompare(m.purgedTotal, strings.NewReader(`
# HELP thittam_purge_worker_purged_total Tenants hard-deleted by the purge worker.
# TYPE thittam_purge_worker_purged_total counter
thittam_purge_worker_purged_total 1
`)); err != nil {
		t.Fatalf("purged_total mismatch: %v", err)
	}
}
```

- [ ] **Step 5: CronJob manifest** in `infra/k8s/jobs/purge-worker-cronjob.yaml` — copy `retention-sweeper-cronjob.yaml` and change: `name: purge-worker`, image `.../purge-worker:latest`, args `-batch 100`, schedule `"45 2 * * *"` (after the sweeper's 02:15), and **CRITICALLY** the DSN key:

```yaml
              env:
                - name: DATABASE_URL
                  valueFrom:
                    secretKeyRef:
                      name: thittam-db
                      key: url          # OWNER DSN — purge needs DROP SCHEMA; runtime_url (thittam_app) cannot.
```

Add a comment block at the top of the file explaining the owner-DSN requirement (the one difference from the sweeper). Keep `concurrencyPolicy: Forbid`, `restartPolicy: Never`, resource limits, and the ServiceAccount (named `purge-worker`).

- [ ] **Step 6: Build, vet, test**

Run: `go build ./... && go vet ./... && go test ./cmd/purge-worker/ -v && go test ./services/iam/ -short`
Expected: worker builds; metrics test passes; iam suite green.

- [ ] **Step 7: Commit**

```bash
git add cmd/purge-worker/ infra/k8s/jobs/purge-worker-cronjob.yaml services/iam/purge.go services/iam/purge_test.go
git commit -m "feat(iam): purge-worker CronJob executes approved purges (#92)

Owner-DSN daily worker: polls approved tenant_purge_requests, DROPs the tenant
schema + tombstones the row via the service. Mirrors retention-sweeper.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- Migration (table + one-open index + purged_at + 'purged' CHECK) → Task 1. ✓
- Two-person approval record + proposer≠approver → Task 2 (record) + Task 4 (`ApproveTenantPurge` self-approval guard). ✓
- Cancel safety valve → Task 4 `CancelTenantPurge` + Task 5 handler. ✓
- Destructive DROP SCHEMA + tombstone (sentinel name, nulled address, kept country/currency) in one owner tx, status-guarded/idempotent → Task 3. ✓
- Audit-log survival → Task 3 integration test asserts it. ✓
- Worker (daily CronJob, owner DSN, retryable) → Task 6. ✓
- 4 audit actions + system actor → Task 4. ✓
- Error mapping (FailedPrecondition/AlreadyExists/NotFound/InvalidArgument) → Task 5 `grpcError`. ✓
- Three-implementer widening + whole-tree vet → Tasks 2, 3 (both add to all doubles; Steps run `go vet ./...`). ✓
- Testing (service unit, handler, worker metrics, DDL integration) → Tasks 3–6. ✓
- Non-goals (no auto-purge, no restore, no list RPC, no sweeper/#123 change) → honored. ✓

**Placeholder scan:** The only deferred specifics are sqlc-generated `*Params` field names and the exact `LIMIT` int width — both resolved by running `sqlc generate` and reading the signatures (Task 2 Step 3). Flagged inline, not left vague. The `errEmptyReason` decision is called out with a concrete resolution (enforce in handler; drop the service check).

**Type consistency:** `TenantPurgeRequest` domain struct fields are used identically across mapper, service, handler proto-mapper, and tests. Repo method signatures match interface ↔ Postgres ↔ mockRepo ↔ iamRepo ↔ service calls. `PurgeApprovedTenant(ctx, *TenantPurgeRequest)` and `ListApprovedPurges(ctx, int)` are the only service methods the worker calls. Audit action consts are referenced by the exact names defined in Task 4.

**Cross-task risk note for the reviewer/controller:** Task 2 intentionally adds the four purge sentinels to `errors.go` (not Task 4) so the e2e `iamRepo` stubs compile; Task 4 must NOT re-declare them. The `grpcError` cases are added in Task 5. The worker cannot unit-test its loop against a fake (service is concrete `*iam.Service`), so loop correctness rests on Task 4's `PurgeApprovedTenant` tests — the final review should confirm that coverage is real.
