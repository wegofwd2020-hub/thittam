# Design: Postgres audit Store + retention-sweeper audit wiring (#92 gap 1)

**Issue:** #92 (tenant retention lifecycle) — closes the "sweeper emits no audit events" gap.
**Date:** 2026-07-07
**Scope:** `pkg/audit` (new Store) + `cmd/retention-sweeper` (wiring) + integration tests
**Branch:** `feat/audit-store-sweeper-92`

## Context

The retention sweeper (`cmd/retention-sweeper`) advances tenants through the
lifecycle (`suspended → grace → deactivated → purge_eligible`) via
`iam.Service.AdvanceTenantLifecycle`. That method already emits an audit event on
each transition (`services/iam/lifecycle.go:113` — action `status_changed`, actor
`system:retention-sweeper`, resource `tenant`), **but only when a non-nil audit
logger is set**. The sweeper builds the service as
`iam.NewService(repo, nil, nil, nil, nil)` (`cmd/retention-sweeper/main.go:77`) with
no `WithAuditLogger`, so every automated transition is silently un-audited —
violating #92's acceptance criteria and the platform's append-only-audit rule.

**Root blocker:** there is **no DB-backed `audit.Store` implementation anywhere** in
the repo. The `Store` interface (`pkg/audit/logger.go:16-25`: `Insert`,
`InsertBatch`, `Query`), the `Event`/`QueryFilter` types (`pkg/audit/types.go`), the
`audit_log` table + migration (`migrations/audit/001_create_audit_log.up.sql`), and
the async `Logger` (`pkg/audit/logger.go`) all exist — but the only `Store`
implementations are test doubles (`mockStore`, `memoryAuditStore`). No binary
persists audit today. So this work first builds the missing Postgres Store, then
wires it into the sweeper.

`sqlc` does not cover the `audit` migrations (no `audit` entry in `sqlc.yaml`), so the
Store uses hand-written **parameterized** pgx queries (no sqlc, no string interpolation).

## Component 1 — `pkg/audit/postgres.go` (the Postgres Store)

```go
type Postgres struct{ pool *pgxpool.Pool }

func NewPostgres(pool *pgxpool.Pool) *Postgres { return &Postgres{pool: pool} }
```

Implements the three `Store` methods against `audit_log`. Append-only — INSERT and
SELECT only, never UPDATE/DELETE.

**`Insert(ctx, Event) error`** — parameterized INSERT. Column handling:
- Omit `id` from the column list → DB default `gen_random_uuid()` assigns it.
- Omit `occurred_at` when `event.OccurredAt.IsZero()` → DB default `now()`; otherwise pass it.
  (In practice the lifecycle emit always sets `OccurredAt`, so it will be passed.)
- `actor_ip`: pass `NULL` when `ActorIP == ""` (nullable column).
- `production_id`: pass `NULL` when `ProductionID == nil`.
- `old_state` / `new_state` / `metadata`: pass `NULL` when the `json.RawMessage` is nil/empty; otherwise the raw JSON.

Sketch:

```go
func (p *Postgres) Insert(ctx context.Context, e audit.Event) error {
	_, err := p.pool.Exec(ctx, `
		INSERT INTO audit_log
			(tenant_id, actor_id, actor_email, actor_ip, action, resource_type,
			 resource_id, production_id, old_state, new_state, metadata, occurred_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11, COALESCE($12, now()))`,
		e.TenantID, e.ActorID, e.ActorEmail, nullStr(e.ActorIP),
		string(e.Action), string(e.ResourceType), e.ResourceID,
		nullUUID(e.ProductionID), nullJSON(e.OldState), nullJSON(e.NewState),
		nullJSON(e.Metadata), nullTime(e.OccurredAt))
	if err != nil {
		return fmt.Errorf("audit/db: insert: %w", err)
	}
	return nil
}
```

Small null helpers (`nullStr`, `nullUUID`, `nullJSON`, `nullTime`) return `any` —
`nil` for the zero value, else the value — so pgx binds SQL NULL. (`id` omitted →
DB default; `occurred_at` via `COALESCE($12, now())` so a zero time still gets `now()`.)

**`InsertBatch(ctx, []Event) error`** — the interface documents "a single
transaction". Wrap in a tx:

```go
func (p *Postgres) InsertBatch(ctx context.Context, events []audit.Event) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("audit/db: begin batch: %w", err)
	}
	defer tx.Rollback(ctx) // no-op after Commit
	for _, e := range events {
		if _, err := tx.Exec(ctx, `INSERT INTO audit_log (...) VALUES (...)`, ...); err != nil {
			return fmt.Errorf("audit/db: batch insert: %w", err)
		}
	}
	return tx.Commit(ctx)
}
```

Factor the INSERT SQL + arg-building into a shared unexported helper used by both
`Insert` and `InsertBatch` (DRY — one column list, one arg builder).

**`Query(ctx, QueryFilter) ([]Event, error)`** — build a parameterized WHERE from the
non-nil filter fields (`TenantID` always; optional `ActorID`, `ResourceType`,
`ResourceID`, `Action`, `From`, `To`), `ORDER BY occurred_at DESC`, `LIMIT`/`OFFSET`.
`Limit` defaults to 100 when `<= 0`. Scan rows back into `audit.Event` (JSONB columns
→ `json.RawMessage`, nullable columns → zero values). Use a small arg-appending
builder (`args = append(args, v); clause += fmt.Sprintf(" AND x = $%d", len(args))`) —
placeholders are positional, never interpolated values.

## Component 2 — wire the sweeper

`cmd/retention-sweeper/main.go`, at the construction site (:76-77), replace the
nil-audit build:

```go
repo := iamdb.NewPostgres(pool)
auditLogger := audit.NewLogger(audit.NewPostgres(pool), audit.DefaultConfig(), nil)
defer func() {
	// Logger is async (buffered, 5s flush) — flush before exit or events are lost.
	if err := auditLogger.Close(context.Background()); err != nil {
		logger.Warn("audit flush on shutdown failed", "err", err)
	}
}()
svc := iam.NewService(repo, nil, nil, nil, nil).WithAuditLogger(auditLogger)
```

Placement notes:
- The `defer auditLogger.Close(...)` must run **before** the process exits and after
  the sweep completes. If the metrics push (`main.go:~83`) happens in `main` after
  `runSweep`, ensure Close runs before the process returns — a `defer` in `main`
  (or an explicit `Close` right after `runSweep` returns, before the push) both work;
  prefer an explicit flush right after the sweep so audit rows are durable before the
  run is reported successful.
- Close takes a fresh `context.Background()` (the sweep's 30-min ctx may be near
  expiry; the flush needs its own budget). Bound it with a short timeout
  (`context.WithTimeout(context.Background(), 10*time.Second)`).
- `NewLogger`'s third arg (`ServiceLogger`) is `nil` → the package's
  `defaultServiceLogger` (the sweeper's `*slog.Logger` does not satisfy the
  `Info/Warn/Error(msg, ...interface{})` interface, so don't pass it).

No change to `services/iam/lifecycle.go` — the emit is already there and becomes live
once the logger is non-nil.

## Component 3 — tests

**Store unit/integration** — new `pkg/audit/postgres_integration_test.go`
(`//go:build integration`, `pkg/testdb`, writes to the shared `audit_log` table):
- Insert → Query round-trips a full event (all fields set) and asserts equality.
- Insert with empty `ActorIP` / nil `ProductionID` / nil JSONB → row stored with NULLs,
  Query returns the zero values.
- `InsertBatch` of N events → all present.
- `Query` filters: by `Action`, by `From/To` time window, `Limit`/`Offset` paging,
  `ORDER BY occurred_at DESC`.
- Each test scopes to a unique `TenantID` (random uuid) + `t.Cleanup` DELETE by that
  tenant, so tests don't collide in the shared table. (The app role can't DELETE in
  prod, but the test DSN is the owner — cleanup is fine in tests.)

**Full-lifecycle audit integration** — new
`services/iam/db/tenant_lifecycle_audit_integration_test.go` (`//go:build integration`):
- Seed a tenant in `suspended` status with `suspended_at` far in the past (mirror the
  `insertSuspendedTenant` helper in `tenant_legal_hold_integration_test.go`).
- Build a `Service` wired with a real `audit.NewPostgres(pool)` logger (and `Close` it
  to flush).
- Call `AdvanceTenantLifecycle` repeatedly (advancing "now" past each threshold, or
  seeding timestamps so each call advances one stage) through
  `suspended→grace→deactivated→purge_eligible`.
- Assert the tenant's final status is `purge_eligible` AND `audit_log` contains a
  `status_changed` row per transition for that tenant (Query by tenant+action, or a
  direct SELECT), each with actor `system:retention-sweeper` and old/new state JSON.

## Acceptance criteria

- [ ] `pkg/audit.Postgres` implements `Store` (Insert/InsertBatch/Query) against `audit_log`, parameterized, append-only.
- [ ] Nullable columns (`actor_ip`, `production_id`, JSONB states) stored/read correctly.
- [ ] Sweeper constructs `audit.NewPostgres` + `NewLogger`, chains `WithAuditLogger`, and flushes (`Close`) before exit.
- [ ] Store integration test: round-trip, batch, nulls, filters.
- [ ] Full-lifecycle integration test: transitions + one `status_changed` audit row per stage.
- [ ] `go vet ./...` clean; affected-package coverage healthy (iam ≥85%, audit ≥75%).

## Out of scope (follow-up)

- Wiring `cmd/iam` (the request-path server) to persist audit — it also passes no
  logger today; a broader change (and it already has a gRPC audit interceptor path).
  Note as a separate ticket; not built here.
- `PurgeTenant` RPC and the full-lifecycle billing trigger (#118) / override RPC (#119).

## Files touched

- `pkg/audit/postgres.go` (new Store)
- `pkg/audit/postgres_integration_test.go` (new)
- `cmd/retention-sweeper/main.go` (wire logger + flush)
- `services/iam/db/tenant_lifecycle_audit_integration_test.go` (new)

Review: audit/tenancy change → senior review advisable (append-only integrity, no PII in audit rows).
