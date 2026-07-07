# Postgres audit Store + retention-sweeper audit wiring (#92) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the missing DB-backed `audit.Store`, wire it into `cmd/retention-sweeper` so lifecycle transitions are persisted to `audit_log`, and prove it with integration tests. Closes #92's "sweeper emits no audit" gap.

**Architecture:** New `pkg/audit.Postgres` (hand-written parameterized pgx; `sqlc` does not cover the audit migrations) implements the existing 3-method `Store` interface against the existing `audit_log` table. The sweeper constructs it, wraps it in the existing async `audit.Logger`, chains `WithAuditLogger`, and flushes before exit. `services/iam/lifecycle.go`'s emit site is unchanged (already present, becomes live with a non-nil logger).

**Tech Stack:** Go 1.22+, pgx/v5 + pgxpool, PostgreSQL, testify, `pkg/testdb`.

## Global Constraints

- Audit table is **append-only** — Store does INSERT + SELECT only, never UPDATE/DELETE.
- SQL parameterized via pgx placeholders — no string interpolation of values (dynamic WHERE builds the *clause* with positional `$N`, never inlines values).
- iam coverage ≥85%; audit package coverage ≥75%.
- Integration tests: `//go:build integration`, skip when `THITTAM_TEST_DSN` unset.
- The async `audit.Logger` (buffered, 5s flush) MUST be `Close`d before the process exits or events are lost.

## Test DB (sandbox)

`make db-test-bootstrap` HANGS here (sudo/askpass) — do NOT run it. `thittam_test` has migrations through 018 applied; the `audit_log` table exists (migration `audit/001`). Integration DSN:
`postgres://thittam:thittam_dev@localhost:5433/thittam_test?sslmode=disable`
Run: `THITTAM_TEST_DSN="<dsn>" go test -tags=integration ./pkg/audit/... ./services/iam/db/... -v`

---

### Task 1: `pkg/audit/postgres.go` Store + its integration test

**Files:**
- Create: `pkg/audit/postgres.go`
- Test: `pkg/audit/postgres_integration_test.go`

**Interfaces:**
- Produces: `audit.NewPostgres(pool *pgxpool.Pool) *Postgres` implementing `audit.Store` (`Insert`, `InsertBatch`, `Query`).
- Consumes: existing `audit.Event`, `audit.QueryFilter`, `audit.Action`, `audit.ResourceType` (`pkg/audit/types.go`), the `audit_log` table (`migrations/audit/001_create_audit_log.up.sql`).

- [ ] **Step 1: Write the Store**

Create `pkg/audit/postgres.go` (package `audit`):

```go
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres is the append-only audit_log Store backed by a pgx pool.
// It performs INSERT and SELECT only — never UPDATE or DELETE.
type Postgres struct{ pool *pgxpool.Pool }

// NewPostgres returns a Postgres audit Store over the given pool.
func NewPostgres(pool *pgxpool.Pool) *Postgres { return &Postgres{pool: pool} }

const insertAuditSQL = `
INSERT INTO audit_log
	(tenant_id, actor_id, actor_email, actor_ip, action, resource_type,
	 resource_id, production_id, old_state, new_state, metadata, occurred_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11, COALESCE($12, now()))`

// insertArgs maps an Event to the 12 positional args for insertAuditSQL.
// Zero/empty optional fields become SQL NULL (id omitted → DB default;
// occurred_at zero → COALESCE picks now()).
func insertArgs(e Event) []any {
	return []any{
		e.TenantID, e.ActorID, e.ActorEmail, nullStr(e.ActorIP),
		string(e.Action), string(e.ResourceType), e.ResourceID,
		nullUUID(e.ProductionID), nullJSON(e.OldState), nullJSON(e.NewState),
		nullJSON(e.Metadata), nullTime(e.OccurredAt),
	}
}

func (p *Postgres) Insert(ctx context.Context, e Event) error {
	if _, err := p.pool.Exec(ctx, insertAuditSQL, insertArgs(e)...); err != nil {
		return fmt.Errorf("audit/db: insert: %w", err)
	}
	return nil
}

func (p *Postgres) InsertBatch(ctx context.Context, events []Event) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("audit/db: begin batch: %w", err)
	}
	defer tx.Rollback(ctx) // no-op after a successful Commit
	for _, e := range events {
		if _, err := tx.Exec(ctx, insertAuditSQL, insertArgs(e)...); err != nil {
			return fmt.Errorf("audit/db: batch insert: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (p *Postgres) Query(ctx context.Context, f QueryFilter) ([]Event, error) {
	args := []any{f.TenantID}
	where := "tenant_id = $1"
	add := func(v any, col string) {
		args = append(args, v)
		where += fmt.Sprintf(" AND %s = $%d", col, len(args))
	}
	if f.ActorID != nil {
		add(*f.ActorID, "actor_id")
	}
	if f.ResourceType != nil {
		add(string(*f.ResourceType), "resource_type")
	}
	if f.ResourceID != nil {
		add(*f.ResourceID, "resource_id")
	}
	if f.Action != nil {
		add(string(*f.Action), "action")
	}
	if f.From != nil {
		args = append(args, *f.From)
		where += fmt.Sprintf(" AND occurred_at >= $%d", len(args))
	}
	if f.To != nil {
		args = append(args, *f.To)
		where += fmt.Sprintf(" AND occurred_at <= $%d", len(args))
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	args = append(args, limit, f.Offset)
	sql := fmt.Sprintf(`
		SELECT id, tenant_id, actor_id, actor_email, actor_ip, action, resource_type,
		       resource_id, production_id, old_state, new_state, metadata, occurred_at
		FROM audit_log WHERE %s
		ORDER BY occurred_at DESC LIMIT $%d OFFSET $%d`,
		where, len(args)-1, len(args))

	rows, err := p.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("audit/db: query: %w", err)
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var (
			e            Event
			actorIP      *string
			productionID *uuid.UUID
			oldState     []byte
			newState     []byte
			metadata     []byte
		)
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.ActorID, &e.ActorEmail, &actorIP, &e.Action,
			&e.ResourceType, &e.ResourceID, &productionID, &oldState, &newState,
			&metadata, &e.OccurredAt,
		); err != nil {
			return nil, fmt.Errorf("audit/db: scan: %w", err)
		}
		if actorIP != nil {
			e.ActorIP = *actorIP
		}
		e.ProductionID = productionID
		e.OldState = json.RawMessage(oldState)
		e.NewState = json.RawMessage(newState)
		e.Metadata = json.RawMessage(metadata)
		out = append(out, e)
	}
	return out, rows.Err()
}

// --- null helpers: return nil (→ SQL NULL) for zero values ---

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func nullUUID(u *uuid.UUID) any {
	if u == nil {
		return nil
	}
	return *u
}
func nullJSON(j json.RawMessage) any {
	if len(j) == 0 {
		return nil
	}
	return []byte(j)
}
func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

var _ Store = (*Postgres)(nil)

// (strings import retained if a later helper needs it; remove if unused.)
var _ = strings.TrimSpace
```

Note: drop the `strings` import + the `var _ = strings.TrimSpace` line if the implementer doesn't use `strings` — do not leave an unused import (build fails). It's included only as a hint; prefer removing both.

- [ ] **Step 2: Verify it compiles + satisfies the interface**

Run: `go build ./pkg/audit/...`
Expected: clean. The `var _ Store = (*Postgres)(nil)` assertion fails to compile if any method signature is wrong.

- [ ] **Step 3: Write the integration test (failing until Step 1 present — here, verifying behavior)**

Create `pkg/audit/postgres_integration_test.go`:

```go
//go:build integration

package audit_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/audit"
	"github.com/wegofwd2020/thittam/pkg/testdb"
)

func TestPostgresAudit_RoundTrip(t *testing.T) {
	pool := testdb.Open(t)
	store := audit.NewPostgres(pool)
	tenant := uuid.New()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_log WHERE tenant_id = $1`, tenant)
	})

	e := audit.Event{
		TenantID:     tenant,
		ActorID:      uuid.New(),
		ActorEmail:   "system:test",
		Action:       audit.ActionStatusChanged,
		ResourceType: audit.ResourceTenant,
		ResourceID:   tenant,
		OldState:     json.RawMessage(`{"status":"suspended"}`),
		NewState:     json.RawMessage(`{"status":"grace"}`),
		OccurredAt:   time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, store.Insert(context.Background(), e))

	got, err := store.Query(context.Background(), audit.QueryFilter{TenantID: tenant})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, e.ActorEmail, got[0].ActorEmail)
	assert.Equal(t, audit.ActionStatusChanged, got[0].Action)
	assert.JSONEq(t, `{"status":"grace"}`, string(got[0].NewState))
	assert.Empty(t, got[0].ActorIP)      // NULL → zero
	assert.Nil(t, got[0].ProductionID)   // NULL → nil
	assert.NotEqual(t, uuid.Nil, got[0].ID) // DB default assigned
}

func TestPostgresAudit_BatchAndFilter(t *testing.T) {
	pool := testdb.Open(t)
	store := audit.NewPostgres(pool)
	tenant := uuid.New()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_log WHERE tenant_id = $1`, tenant)
	})

	base := time.Now().UTC()
	mk := func(a audit.Action, at time.Time) audit.Event {
		return audit.Event{TenantID: tenant, ActorID: uuid.New(), ActorEmail: "system:test",
			Action: a, ResourceType: audit.ResourceTenant, ResourceID: tenant, OccurredAt: at}
	}
	require.NoError(t, store.InsertBatch(context.Background(), []audit.Event{
		mk(audit.ActionStatusChanged, base.Add(-2*time.Hour)),
		mk(audit.ActionStatusChanged, base.Add(-1*time.Hour)),
		mk(audit.ActionLegalHoldApplied, base),
	}))

	// filter by action
	sc := audit.ActionStatusChanged
	got, err := store.Query(context.Background(), audit.QueryFilter{TenantID: tenant, Action: &sc})
	require.NoError(t, err)
	assert.Len(t, got, 2)
	// DESC order
	assert.True(t, got[0].OccurredAt.After(got[1].OccurredAt))

	// limit/offset
	page, err := store.Query(context.Background(), audit.QueryFilter{TenantID: tenant, Limit: 1, Offset: 1})
	require.NoError(t, err)
	assert.Len(t, page, 1)
}
```

- [ ] **Step 4: Run the integration test**

Run: `THITTAM_TEST_DSN="postgres://thittam:thittam_dev@localhost:5433/thittam_test?sslmode=disable" go test -tags=integration ./pkg/audit/ -run TestPostgresAudit -v`
Expected: PASS (both tests).

- [ ] **Step 5: Vet + existing audit unit tests still pass**

Run: `go vet ./pkg/audit/... && go test ./pkg/audit/ -short`
Expected: clean; existing `logger_test`/`interceptor_test` still green.

- [ ] **Step 6: Commit**

```bash
git add pkg/audit/postgres.go pkg/audit/postgres_integration_test.go
git commit -m "feat(audit): Postgres append-only Store for audit_log (#92)"
```

---

### Task 2: Wire the sweeper + full-lifecycle audit integration test

**Files:**
- Modify: `cmd/retention-sweeper/main.go:76-77` (construct logger, flush before exit)
- Test: `services/iam/db/tenant_lifecycle_audit_integration_test.go` (new)

**Interfaces:**
- Consumes: `audit.NewPostgres` (Task 1), `audit.NewLogger`/`DefaultConfig`/`Logger.Close` (`pkg/audit/logger.go:77,67,165`), `iam.Service.WithAuditLogger` (`services/iam/lifecycle.go:62`), `iam.Service.AdvanceTenantLifecycle(ctx, id, now) (*iam.LifecycleTransition, error)` (`services/iam/lifecycle.go:79` — confirm exact name/return with `grep -n "func.*AdvanceTenantLifecycle" services/iam/lifecycle.go`).

- [ ] **Step 1: Wire the audit logger into the sweeper**

In `cmd/retention-sweeper/main.go`, replace lines 76-77:

```go
	repo := iamdb.NewPostgres(pool)

	auditLogger := audit.NewLogger(audit.NewPostgres(pool), audit.DefaultConfig(), nil)
	svc := iam.NewService(repo, nil, nil, nil, nil).WithAuditLogger(auditLogger)
```

Add the import `"github.com/wegofwd2020/thittam/pkg/audit"`.

Then, after `runSweep` returns and BEFORE `pushMetrics` (currently :83-88), flush the
async logger so audit rows are durable before the run reports success:

```go
	err = runSweep(ctx, svc, logger, metrics, *batchSize, *maxBatches)

	// Flush buffered audit events before we report/push. The Logger is async
	// (5s flush); Close drains it. Use a fresh bounded context — the sweep ctx
	// may be near its 30-min deadline.
	flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if cerr := auditLogger.Close(flushCtx); cerr != nil {
		logger.Warn("audit flush failed", "error", cerr)
	}
	flushCancel()

	pushMetrics(ctx, logger, pushURL, instance, metrics.registry)
```

- [ ] **Step 2: Build the sweeper**

Run: `go build ./cmd/retention-sweeper/...`
Expected: clean.

- [ ] **Step 3: Write the full-lifecycle audit integration test**

Create `services/iam/db/tenant_lifecycle_audit_integration_test.go`. IMPORTANT: use the
**pool directly** (not `testdb.NewTx`) with `t.Cleanup` — the async audit Logger flushes
through its own store on the pool and cannot participate in a test transaction; tenant
transitions and audit writes must share the committed pool.

```go
//go:build integration

package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/audit"
	iam "github.com/wegofwd2020/thittam/services/iam"
	iamdb "github.com/wegofwd2020/thittam/services/iam/db"
	"github.com/wegofwd2020/thittam/pkg/testdb"
)

func TestTenantLifecycle_EmitsAuditPerTransition(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	tenantID := uuid.New()
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM audit_log WHERE tenant_id = $1`, tenantID)
		_, _ = pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
	})

	// Seed a suspended tenant; suspended_at drives the first transition.
	base := time.Now().UTC()
	_, err := pool.Exec(ctx, `
		INSERT INTO tenants (id, name, slug, country_code, primary_currency_code, status, suspended_at)
		VALUES ($1, $2, $3, 'US', 'USD', 'suspended', $4)`,
		tenantID, "Lifecycle Audit Co", "slug-"+tenantID.String()[:8], base.Add(-1*time.Hour))
	require.NoError(t, err)

	auditStore := audit.NewPostgres(pool)
	logger := audit.NewLogger(auditStore, audit.DefaultConfig(), nil)
	svc := iam.NewService(iamdb.NewPostgres(pool), nil, nil, nil, nil).WithAuditLogger(logger)

	// Advance through each stage by passing a "now" past each threshold. Confirm the
	// exact durations with `grep -n "Duration\|GracePeriod" services/iam/lifecycle.go`
	// and set the offsets accordingly (suspended→grace ~30d, →deactivated ~90d from
	// suspended_at, →purge_eligible ~180d from deactivated_at). Call AdvanceTenantLifecycle
	// once per threshold; each call performs one conditional transition.
	for _, now := range lifecycleThresholds(base) { // helper: returns the 3 "now" values
		_, err := svc.AdvanceTenantLifecycle(ctx, tenantID, now)
		require.NoError(t, err)
	}

	require.NoError(t, logger.Close(ctx)) // flush async events before asserting

	// Final status is purge_eligible.
	var status string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM tenants WHERE id = $1`, tenantID).Scan(&status))
	assert.Equal(t, "purge_eligible", status)

	// One status_changed audit row per transition, actor = system:retention-sweeper.
	sc := audit.ActionStatusChanged
	events, err := auditStore.Query(ctx, audit.QueryFilter{TenantID: tenantID, Action: &sc, Limit: 100})
	require.NoError(t, err)
	assert.Len(t, events, 3)
	for _, e := range events {
		assert.Equal(t, "system:retention-sweeper", e.ActorEmail)
		assert.Equal(t, audit.ResourceTenant, e.ResourceType)
	}
}
```

Implementation notes for the test author:
- Write the `lifecycleThresholds(base)` helper (or inline the 3 `now` values) using the
  real durations from `services/iam/lifecycle.go` (grep `SuspensionGracePeriod`,
  `GraceToDeactivatedDuration`, `DeactivatedToPurgeDuration`). Because `deactivated_at`
  is stamped when the deactivated transition fires, the third `now` must be past
  `(second now) + DeactivatedToPurgeDuration` — e.g. `base+31d`, `base+91d`, `base+272d`.
  Verify against `nextLifecycleStatus` (`lifecycle.go:154`) so each call advances exactly
  one stage.
- If `AdvanceTenantLifecycle` returns a nil transition when not yet due, adjust the `now`
  offsets until each call transitions; assert `require.NoError` (nil transition is not an
  error).

- [ ] **Step 4: Run the lifecycle audit test**

Run: `THITTAM_TEST_DSN="postgres://thittam:thittam_dev@localhost:5433/thittam_test?sslmode=disable" go test -tags=integration ./services/iam/db/ -run TestTenantLifecycle_EmitsAuditPerTransition -v`
Expected: PASS — final status `purge_eligible`, 3 `status_changed` audit rows.

- [ ] **Step 5: Whole-tree vet + affected suites + coverage**

Run:
```
go vet ./...
go test ./cmd/retention-sweeper/... ./services/iam/... ./pkg/audit/... -short
THITTAM_TEST_DSN="postgres://thittam:thittam_dev@localhost:5433/thittam_test?sslmode=disable" go test -tags=integration ./pkg/audit/... ./services/iam/db/... 2>&1 | tail
```
Expected: vet clean; unit suites pass; integration suites pass.

- [ ] **Step 6: Commit**

```bash
git add cmd/retention-sweeper/main.go services/iam/db/tenant_lifecycle_audit_integration_test.go
git commit -m "feat(audit): wire audit logger into retention sweeper + lifecycle audit test (#92)"
```

---

## Self-Review

**Spec coverage:**
- `pkg/audit.Postgres` Store (Insert/InsertBatch/Query, parameterized, append-only) → Task 1 Step 1. ✅
- Nullable columns handled → Task 1 null helpers + round-trip test asserts NULL→zero. ✅
- Sweeper constructs store+logger, WithAuditLogger, flush before exit → Task 2 Step 1. ✅
- Store integration test (round-trip, batch, nulls, filters) → Task 1 Step 3. ✅
- Full-lifecycle audit test (transitions + 1 row/stage) → Task 2 Step 3. ✅
- vet + coverage → Task 2 Step 5. ✅
- Out of scope (cmd/iam persistence, PurgeTenant, #118/#119) — not in plan. ✅

**Type consistency:** `NewPostgres(pool) *Postgres` implements `Store` (guarded by `var _ Store = (*Postgres)(nil)`). `insertAuditSQL`/`insertArgs` shared by Insert + InsertBatch (DRY). Query builds positional placeholders only. Sweeper uses `audit.NewLogger(audit.NewPostgres(pool), audit.DefaultConfig(), nil)` + `.WithAuditLogger` + `Close`. Test asserts actor `system:retention-sweeper` (matches `services/iam/lifecycle.go:32`) and action `status_changed`.

**Placeholder scan:** No TBD/TODO in delivered code. The `lifecycleThresholds` helper and the exact day-offsets are explicitly delegated to the implementer with a grep-backed rule (confirm durations from `lifecycle.go`) because they depend on the precise constants — this is a verification step, not vague hand-waving. The `strings`-import hint line is explicitly flagged for removal to avoid an unused import.

**Interface note:** No public interface widens here (new type implementing an existing interface), so no cross-tree caller breakage — but Task 2 Step 5 still runs whole-tree `go vet ./...` as standard diligence. See [[reference_iam_repository_implementers]].
