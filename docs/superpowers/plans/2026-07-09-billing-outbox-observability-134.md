# Billing Outbox Observability + DLQ Implementation Plan (#134)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the billing outbox relay observable and give permanently-failing ("poison") events a dead-letter queue with a human-triggered replay path.

**Architecture:** A new `event_outbox_dead` table receives rows that fail publication `maxOutboxAttempts` times *while at least one batch-mate succeeded* — a systemic-failure guard that prevents a NATS outage from mass-parking the backlog. The relay sets three gauges per tick from a single `OutboxStats` query, three Prometheus alerts fire on them, and `cmd/outbox-admin` lets an operator list and replay parked events.

**Tech Stack:** Go 1.22+, pgx v5 / pgxpool, golang-migrate v4, prometheus/client_golang (`promauto`, default registry), cobra (CLI), testify, `pkg/testdb`.

**Spec:** `docs/superpowers/specs/2026-07-09-billing-outbox-observability-134-design.md`

## Global Constraints

- **Local DB safety (CLAUDE.md, non-negotiable):** Never run `docker compose … -v` / `down` / `up` against `infra/local/`. That compose is project-scoped; `-v` deletes ALL local volumes (it once destroyed unrelated MinIO dev data). For DB verification use a disposable, uniquely-named throwaway container, or `pkg/testdb` (integration tests SKIP without `THITTAM_TEST_DSN`). CI's real-Postgres job is the authoritative up/down gate. **This binds every subagent.**
- **Whole-tree vet is the gate.** Widening `billing.Repository` breaks three implementers; `go build ./services/billing/...` and focused tests will NOT catch the e2e double. Every task that touches the interface must end with `go vet ./...` over the whole tree.
- **errcheck is on in CI, not locally.** golangci-lint is not installed in this sandbox. A deferred rollback must be written `defer func() { _ = tx.Rollback(ctx) }()`, never `defer tx.Rollback(ctx)`.
- **SQL** is always parameterized. No string interpolation.
- **Logging** is `slog`, structured, no PII and no secrets. An outbox `payload` is tenant data — never log it. `last_error` and `subject` are safe.
- **Monetary values** are `NUMERIC(14,2)` / `decimal.Decimal`, never `float64`. (Nothing in this change is monetary, but the rule stands.)
- **Commits:** Conventional Commits, scope `billing` (or `infra` for the Prometheus task).
- **Coverage:** billing threshold is ≥ 75%; the package sits at 80.5% and must not regress.
- **Error wrapping style:** `fmt.Errorf("billing: <op>: %w", err)` inline. No helper.

## File Structure

| File | Responsibility |
|---|---|
| `migrations/billing/003_event_outbox_dead.{up,down}.sql` | the DLQ table; additive, touches nothing existing |
| `services/billing/models.go` | `OutboxStats` value type |
| `services/billing/errors.go` | `ErrOutboxEventNotFound` sentinel |
| `services/billing/repository.go` | four new interface methods |
| `services/billing/db/postgres.go` | their Postgres implementations (two transactional) |
| `services/billing/service_test.go` | `mockRepo` — unit-test double |
| `e2e/critical_path/billing_test.go` | `billingRepo` — e2e double |
| `services/billing/db/outbox_integration_test.go` | move/replay/stats against real Postgres |
| `services/billing/outbox_relay.go` | guarded cap, gauges, counter |
| `services/billing/outbox_relay_test.go` | the cap's behaviour, especially the systemic guard |
| `cmd/outbox-admin/{main,commands}.go` | operator CLI: `list`, `replay` |
| `infra/prometheus/prometheus.yml` | enable the `billing` scrape job |
| `infra/prometheus/alerts/billing.yaml` | three outbox alert rules |

**Execution order rationale.** Task 2 adds interface methods *and* updates all three implementers in one commit, because splitting them leaves the tree red. Task 3 depends on Task 2's methods existing. Tasks 4 and 5 are independent of each other. No task ends with a broken build.

**No registration changes needed for the migration:** `scripts/_db-common.sh` already lists `billing` in `MIGRATION_DIRS`, and the `Makefile` already runs `migrations/billing` in both `migrate-all` and `migrate-down` (added by #130). Dropping `003_*.sql` in is sufficient.

---

### Task 1: Migration 003 — `event_outbox_dead`

**Files:**
- Create: `migrations/billing/003_event_outbox_dead.up.sql`
- Create: `migrations/billing/003_event_outbox_dead.down.sql`

**Interfaces:**
- Consumes: nothing.
- Produces: table `event_outbox_dead` with columns `(id UUID PK, subject TEXT, tenant_id UUID, payload JSONB, created_at TIMESTAMPTZ, attempts INTEGER, last_error TEXT, died_at TIMESTAMPTZ)` and index `idx_event_outbox_dead_died_at`. Task 2's SQL depends on this exact column list and order.

**Constraint for this task:** SQL and file inspection only. Do **not** start Postgres, do **not** run Docker, do **not** touch `infra/local/`. CI's real-Postgres job verifies up/down.

- [ ] **Step 1: Write the up migration**

Create `migrations/billing/003_event_outbox_dead.up.sql`. Match the header-comment style of `002_event_outbox.up.sql` (filename + purpose + issue ref, then a rationale paragraph, aligned column definitions):

```sql
-- 003_event_outbox_dead.up.sql — dead-letter queue for the billing outbox (#134).
-- A row lands here when the relay fails to publish it maxOutboxAttempts times
-- while at least one batch-mate succeeded — i.e. the event is poison, not the
-- victim of a NATS outage. Rows move OUT of event_outbox so the relay's claim
-- query and its partial index never see them. Replay is human-triggered via
-- cmd/outbox-admin; nothing drains this table automatically, by design.
CREATE TABLE event_outbox_dead (
    id         UUID        PRIMARY KEY,          -- carried from event_outbox
    subject    TEXT        NOT NULL,
    tenant_id  UUID        NOT NULL,
    payload    JSONB       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,             -- original enqueue time, preserved
    attempts   INTEGER     NOT NULL,
    last_error TEXT,
    died_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Operator queries list most-recently-parked events first.
CREATE INDEX idx_event_outbox_dead_died_at ON event_outbox_dead (died_at);
```

There is deliberately no `sent_at` column: a dead row is by definition unsent.

- [ ] **Step 2: Write the down migration**

Create `migrations/billing/003_event_outbox_dead.down.sql`. Bare drops, reverse order, no header comment — matching `002_event_outbox.down.sql`:

```sql
DROP INDEX IF EXISTS idx_event_outbox_dead_died_at;
DROP TABLE IF EXISTS event_outbox_dead;
```

- [ ] **Step 3: Verify the file pair is well-formed**

Run: `ls migrations/billing/`
Expected: exactly these eight files —
```
001_create_tables.down.sql  001_create_tables.up.sql
002_event_outbox.down.sql   002_event_outbox.up.sql
003_event_outbox_dead.down.sql  003_event_outbox_dead.up.sql
```
(golang-migrate stores no content checksums, so an un-applied migration is safe to edit; but the `NNN_<name>.{up,down}.sql` pair must be exact or `migrate` will not see it.)

- [ ] **Step 4: Confirm no registration change is required**

Run: `grep -n 'billing' scripts/_db-common.sh Makefile`
Expected: `billing` already present in `MIGRATION_DIRS`, and `migrations/billing` already present in both the `migrate-all` and `migrate-down` targets with `x-migrations-table=schema_migrations_billing`. **Make no edits.** If it is somehow absent, stop and report — that contradicts #130 and needs a human.

- [ ] **Step 5: Commit**

```bash
git add migrations/billing/003_event_outbox_dead.up.sql migrations/billing/003_event_outbox_dead.down.sql
git commit -m "feat(billing): migration 003 — event_outbox_dead table (#134)"
```

---

### Task 2: Repository — dead-letter + stats methods, and all three doubles

**Files:**
- Modify: `services/billing/models.go` (append `OutboxStats` near `OutboxEvent`, ~line 143)
- Modify: `services/billing/errors.go` (append one sentinel)
- Modify: `services/billing/repository.go:45-50` (extend the `// Outbox (#126)` block)
- Modify: `services/billing/db/postgres.go` (append after `DeleteSentOutboxOlderThan`, ~line 217)
- Modify: `services/billing/service_test.go` (`mockRepo` struct ~line 40-44, methods ~line 188)
- Modify: `e2e/critical_path/billing_test.go` (`billingRepo` struct ~line 37, methods after ~line 283)
- Modify: `services/billing/db/outbox_integration_test.go` (append tests)

**Interfaces:**
- Consumes: table `event_outbox_dead` from Task 1.
- Produces, relied on by Tasks 3 and 4 — these exact signatures:
  ```go
  MoveOutboxToDead(ctx context.Context, id uuid.UUID, errMsg string) error
  OutboxStats(ctx context.Context) (*OutboxStats, error)
  ListDeadOutbox(ctx context.Context, limit int) ([]*OutboxEvent, error)
  ReplayDeadOutbox(ctx context.Context, id uuid.UUID) error
  ```
  and the type:
  ```go
  type OutboxStats struct {
      Pending              int64
      OldestPendingSeconds float64
      Dead                 int64
  }
  ```
  and the sentinel `billing.ErrOutboxEventNotFound`.

**Why one task:** adding methods to the `Repository` interface breaks `*Postgres`, `mockRepo`, and `billingRepo` simultaneously. Splitting this leaves the tree un-buildable between commits.

**Constraint for this task:** the integration tests you write here run only under `-tags integration` with `THITTAM_TEST_DSN` set, and they SKIP otherwise. You will NOT have a database. Write them, compile them, do not run them. Do **not** start Docker. Do **not** touch `infra/local/`.

- [ ] **Step 1: Add the `OutboxStats` type**

In `services/billing/models.go`, immediately after the `OutboxEvent` struct (which ends ~line 143). Note `OutboxEvent` carries no json tags; `OutboxStats` follows suit — it is an internal value, never serialized.

```go
// OutboxStats is a point-in-time snapshot of outbox health, read once per
// relay tick to drive the pending/oldest/dead gauges (#134).
type OutboxStats struct {
	Pending              int64
	OldestPendingSeconds float64
	Dead                 int64
}
```

- [ ] **Step 2: Add the sentinel**

In `services/billing/errors.go`, append inside the existing `var (...)` block, matching the `errors.New("billing: <msg>")` style:

```go
	ErrOutboxEventNotFound     = errors.New("billing: outbox event not found")
```

- [ ] **Step 3: Extend the Repository interface**

In `services/billing/repository.go`, the `// Outbox (#126)` block ends at line 50. Append:

```go
	// Outbox DLQ + observability (#134)
	MoveOutboxToDead(ctx context.Context, id uuid.UUID, errMsg string) error
	OutboxStats(ctx context.Context) (*OutboxStats, error)
	ListDeadOutbox(ctx context.Context, limit int) ([]*OutboxEvent, error)
	ReplayDeadOutbox(ctx context.Context, id uuid.UUID) error
```

- [ ] **Step 4: Run the build to watch it break in exactly three places**

Run: `go vet ./... 2>&1 | head -30`
Expected: FAIL. `*db.Postgres`, `*billing.mockRepo`, and `*billingRepo` each reported as not implementing `billing.Repository` (missing `MoveOutboxToDead`, …). If you see fewer than three distinct types named, a fourth implementer exists that this plan did not anticipate — stop and report it.

- [ ] **Step 5: Implement the four methods on `*Postgres`**

In `services/billing/db/postgres.go`, after `DeleteSentOutboxOlderThan` (~line 217). Copy the tx idiom from `SuspendSubscriptionWithOutbox` exactly: `Begin` → `defer func() { _ = tx.Rollback(ctx) }()` → `Commit`.

```go
// MoveOutboxToDead parks a poison event: copy it into event_outbox_dead and
// delete it from event_outbox, in one tx. Idempotent — if a concurrent relay
// already moved the row, zero rows are copied and the tx commits as a no-op.
func (p *Postgres) MoveOutboxToDead(ctx context.Context, id uuid.UUID, errMsg string) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("billing: begin move-to-dead tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO event_outbox_dead (id, subject, tenant_id, payload, created_at, attempts, last_error, died_at)
		SELECT id, subject, tenant_id, payload, created_at, attempts, $2, now()
		FROM event_outbox WHERE id = $1
		ON CONFLICT (id) DO NOTHING`, id, errMsg); err != nil {
		return fmt.Errorf("billing: insert dead outbox: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM event_outbox WHERE id = $1`, id); err != nil {
		return fmt.Errorf("billing: delete moved outbox: %w", err)
	}

	return tx.Commit(ctx)
}

// OutboxStats reads pending depth, oldest-pending age, and dead depth in one
// round trip. The first two subqueries ride idx_event_outbox_unsent.
func (p *Postgres) OutboxStats(ctx context.Context) (*billing.OutboxStats, error) {
	var s billing.OutboxStats
	err := p.db.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM event_outbox WHERE sent_at IS NULL),
		  (SELECT coalesce(extract(epoch FROM now() - min(created_at)), 0)::double precision
		     FROM event_outbox WHERE sent_at IS NULL),
		  (SELECT count(*) FROM event_outbox_dead)`).
		Scan(&s.Pending, &s.OldestPendingSeconds, &s.Dead)
	if err != nil {
		return nil, fmt.Errorf("billing: outbox stats: %w", err)
	}
	return &s, nil
}

// ListDeadOutbox returns parked events, most recently died first.
func (p *Postgres) ListDeadOutbox(ctx context.Context, limit int) ([]*billing.OutboxEvent, error) {
	rows, err := p.db.Query(ctx, `
		SELECT id, subject, tenant_id, payload, created_at, attempts, last_error
		FROM event_outbox_dead
		ORDER BY died_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("billing: list dead outbox: %w", err)
	}
	defer rows.Close()

	var out []*billing.OutboxEvent
	for rows.Next() {
		var e billing.OutboxEvent
		var lastErr pgtype.Text
		if err := rows.Scan(&e.ID, &e.Subject, &e.TenantID, &e.Payload, &e.CreatedAt, &e.Attempts, &lastErr); err != nil {
			return nil, fmt.Errorf("billing: scan dead outbox: %w", err)
		}
		if lastErr.Valid {
			e.LastError = &lastErr.String
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}

// ReplayDeadOutbox re-arms a parked event: move it back to event_outbox with
// attempts reset and last_error cleared, preserving id and created_at so
// age-based alerts stay honest. Unknown id → ErrOutboxEventNotFound.
func (p *Postgres) ReplayDeadOutbox(ctx context.Context, id uuid.UUID) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("billing: begin replay tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ct, err := tx.Exec(ctx, `
		INSERT INTO event_outbox (id, subject, tenant_id, payload, created_at, sent_at, attempts, last_error)
		SELECT id, subject, tenant_id, payload, created_at, NULL, 0, NULL
		FROM event_outbox_dead WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("billing: reinsert outbox: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return billing.ErrOutboxEventNotFound
	}

	if _, err := tx.Exec(ctx, `DELETE FROM event_outbox_dead WHERE id = $1`, id); err != nil {
		return fmt.Errorf("billing: delete replayed outbox: %w", err)
	}

	return tx.Commit(ctx)
}
```

Note `pgtype` is already imported in this file. `ON CONFLICT (id) DO NOTHING` makes the move safe if two relay replicas race the same row — an impossible race today (one relay per process, `SKIP LOCKED` on claim) but free to defend.

- [ ] **Step 6: Add four fn-fields and four methods to `mockRepo`**

In `services/billing/service_test.go`, append to the struct after `deleteSentOutboxOlderThanFn` (~line 44):

```go
	// Outbox DLQ (#134)
	moveOutboxToDeadFn  func(ctx context.Context, id uuid.UUID, errMsg string) error
	outboxStatsFn       func(ctx context.Context) (*OutboxStats, error)
	listDeadOutboxFn    func(ctx context.Context, limit int) ([]*OutboxEvent, error)
	replayDeadOutboxFn  func(ctx context.Context, id uuid.UUID) error
```

And the methods, after the existing outbox methods (~line 188), following the "call-fn-if-set, else zero value" idiom:

```go
func (m *mockRepo) MoveOutboxToDead(ctx context.Context, id uuid.UUID, errMsg string) error {
	if m.moveOutboxToDeadFn != nil {
		return m.moveOutboxToDeadFn(ctx, id, errMsg)
	}
	return nil
}

func (m *mockRepo) OutboxStats(ctx context.Context) (*OutboxStats, error) {
	if m.outboxStatsFn != nil {
		return m.outboxStatsFn(ctx)
	}
	return &OutboxStats{}, nil
}

func (m *mockRepo) ListDeadOutbox(ctx context.Context, limit int) ([]*OutboxEvent, error) {
	if m.listDeadOutboxFn != nil {
		return m.listDeadOutboxFn(ctx, limit)
	}
	return nil, nil
}

func (m *mockRepo) ReplayDeadOutbox(ctx context.Context, id uuid.UUID) error {
	if m.replayDeadOutboxFn != nil {
		return m.replayDeadOutboxFn(ctx, id)
	}
	return nil
}
```

`OutboxStats` defaults to a non-nil zero struct, not `nil, nil` — Task 3's relay dereferences the result, and a nil default would make every unrelated relay test panic.

- [ ] **Step 7: Add the four methods to the e2e double `billingRepo`**

In `e2e/critical_path/billing_test.go`. First add a `dead` slice to the struct (~line 37, next to `outbox`):

```go
	dead          []*billing.OutboxEvent
```

Then append the methods after `DeleteSentOutboxOlderThan` (~line 283). Mirror the existing mutex-guarded, slice-scanning style:

```go
func (r *billingRepo) MoveOutboxToDead(_ context.Context, id uuid.UUID, errMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var kept []*billing.OutboxEvent
	for _, e := range r.outbox {
		if e.ID == id {
			e.LastError = &errMsg
			r.dead = append(r.dead, e)
			continue
		}
		kept = append(kept, e)
	}
	r.outbox = kept
	return nil
}

func (r *billingRepo) OutboxStats(_ context.Context) (*billing.OutboxStats, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := &billing.OutboxStats{Dead: int64(len(r.dead))}
	var oldest time.Time
	for _, e := range r.outbox {
		if e.SentAt != nil {
			continue
		}
		s.Pending++
		if oldest.IsZero() || e.CreatedAt.Before(oldest) {
			oldest = e.CreatedAt
		}
	}
	if !oldest.IsZero() {
		s.OldestPendingSeconds = time.Since(oldest).Seconds()
	}
	return s, nil
}

func (r *billingRepo) ListDeadOutbox(_ context.Context, limit int) ([]*billing.OutboxEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit > len(r.dead) {
		limit = len(r.dead)
	}
	out := make([]*billing.OutboxEvent, limit)
	copy(out, r.dead[:limit])
	return out, nil
}

func (r *billingRepo) ReplayDeadOutbox(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, e := range r.dead {
		if e.ID == id {
			e.Attempts = 0
			e.LastError = nil
			e.SentAt = nil
			r.outbox = append(r.outbox, e)
			r.dead = append(r.dead[:i], r.dead[i+1:]...)
			return nil
		}
	}
	return billing.ErrOutboxEventNotFound
}
```

- [ ] **Step 8: Whole-tree vet — the real gate**

Run: `go vet ./...`
Expected: clean, no output. If `go build ./services/billing/...` passes but `go vet ./...` fails, you missed the e2e double. That is the exact trap this step exists to catch.

- [ ] **Step 9: Write the integration tests**

Append to `services/billing/db/outbox_integration_test.go`. The file already has `//go:build integration`, package `db_test`, and the imports you need. There are **no** `seedTenant` / `newTestSubscription` helpers — the existing test inlines its seeding, and so does this one.

**Two constraints the existing test teaches you, and you must obey:**

1. **`event_outbox` is shared and un-namespaced.** Integration tests may run against a database holding other tests' rows. The existing test therefore filters claimed events by `tenantID` and never asserts a global count. **Never assert `stats.Pending == 1`** — assert on *deltas* against a baseline captured at the start.
2. **`event_outbox.tenant_id` has no foreign key** (see migration `002` — no `REFERENCES`). Deleting the `tenants` row does *not* cascade to outbox rows. Clean up `event_outbox` and `event_outbox_dead` explicitly, or you leak rows into every later test's stats.

```go
func TestOutboxDLQ_MoveReplayAndStats(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := billingdb.NewPostgres(pool)

	// Baseline: other tests' rows may already be present.
	base, err := repo.OutboxStats(ctx)
	require.NoError(t, err)

	tenantID := uuid.New()
	_, err = pool.Exec(ctx,
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code, status)
		 VALUES ($1, $2, $3, 'US', 'USD', 'active')`,
		tenantID, "DLQ IT "+tenantID.String()[:8], "dlq-"+tenantID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM event_outbox_dead WHERE tenant_id = $1`, tenantID)
		_, _ = pool.Exec(bg, `DELETE FROM event_outbox WHERE tenant_id = $1`, tenantID)
		_, _ = pool.Exec(bg, `DELETE FROM tenants WHERE id = $1`, tenantID)
	})

	now := time.Now().UTC()
	sub := &billing.Subscription{
		ID: uuid.New(), TenantID: tenantID, Plan: "starter", Status: "active", BillingCycle: "monthly",
		CurrentPeriodStart: now, CurrentPeriodEnd: now.AddDate(0, 1, 0), CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.CreateSubscription(ctx, sub))

	sus := now
	sub.Status = "suspended"
	sub.SuspendedAt = &sus
	sub.UpdatedAt = now
	require.NoError(t, repo.SuspendSubscriptionWithOutbox(ctx, sub,
		"thittam.billing.subscription.suspended", []byte(`{"subscription_id":"x"}`)))

	// Find our row among any others.
	ev := claimMine(t, ctx, repo, tenantID)
	origCreatedAt := ev.CreatedAt

	stats, err := repo.OutboxStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, base.Pending+1, stats.Pending, "our suspend added one pending row")
	assert.Equal(t, base.Dead, stats.Dead)
	assert.GreaterOrEqual(t, stats.OldestPendingSeconds, 0.0)

	// Move: atomic, preserves id and created_at.
	require.NoError(t, repo.MoveOutboxToDead(ctx, ev.ID, "stream rejected payload"))

	stats, err = repo.OutboxStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, base.Pending, stats.Pending, "moved row must leave event_outbox")
	assert.Equal(t, base.Dead+1, stats.Dead)

	// A dead row is never claimed again.
	assert.Nil(t, tryClaimMine(t, ctx, repo, tenantID), "dead rows must not be claimable")

	dead, err := repo.ListDeadOutbox(ctx, 100)
	require.NoError(t, err)
	var mineDead *billing.OutboxEvent
	for _, d := range dead {
		if d.ID == ev.ID {
			mineDead = d
		}
	}
	require.NotNil(t, mineDead, "our event is in the DLQ")
	assert.WithinDuration(t, origCreatedAt, mineDead.CreatedAt, time.Second, "created_at preserved")
	require.NotNil(t, mineDead.LastError)
	assert.Contains(t, *mineDead.LastError, "stream rejected")

	// Replay: attempts reset, row claimable again.
	require.NoError(t, repo.ReplayDeadOutbox(ctx, ev.ID))

	stats, err = repo.OutboxStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, base.Pending+1, stats.Pending)
	assert.Equal(t, base.Dead, stats.Dead)

	replayed := claimMine(t, ctx, repo, tenantID)
	assert.Equal(t, ev.ID, replayed.ID)
	assert.Equal(t, 1, replayed.Attempts, "replay reset attempts to 0; this claim incremented it to 1")
	assert.Nil(t, replayed.LastError)
}

func TestOutboxDLQ_ReplayUnknownID(t *testing.T) {
	pool := testdb.Open(t)
	repo := billingdb.NewPostgres(pool)

	err := repo.ReplayDeadOutbox(context.Background(), uuid.New())
	assert.ErrorIs(t, err, billing.ErrOutboxEventNotFound)
}

// claimMine claims a batch and returns this tenant's event, failing if absent.
func claimMine(t *testing.T, ctx context.Context, repo *billingdb.Postgres, tenantID uuid.UUID) *billing.OutboxEvent {
	t.Helper()
	e := tryClaimMine(t, ctx, repo, tenantID)
	require.NotNil(t, e, "expected a claimable outbox row for this tenant")
	return e
}

// tryClaimMine claims a batch and returns this tenant's event, or nil.
func tryClaimMine(t *testing.T, ctx context.Context, repo *billingdb.Postgres, tenantID uuid.UUID) *billing.OutboxEvent {
	t.Helper()
	claimed, err := repo.ClaimUnsentOutbox(ctx, 100)
	require.NoError(t, err)
	for _, e := range claimed {
		if e.TenantID == tenantID {
			return e
		}
	}
	return nil
}
```

Note `tryClaimMine` claims (and so increments `attempts` on) other tests' rows as a side effect. That is exactly what the pre-existing test already does, and is harmless — `attempts` carries no meaning until a publish fails.

- [ ] **Step 10: Verify the integration tests compile (do NOT run them)**

Run: `go vet -tags=integration ./services/billing/...`
Expected: clean, no output.

Then confirm they skip without a DSN: `go test -tags=integration ./services/billing/db/ -run TestOutboxDLQ -v 2>&1 | head -5`
Expected: `SKIP` (because `THITTAM_TEST_DSN` is unset). **Do not set it. Do not start a database.** CI runs these against real Postgres.

- [ ] **Step 11: Run the existing unit tests to confirm nothing regressed**

Run: `go test ./services/billing/... -short`
Expected: PASS. The new `mockRepo` defaults must not have disturbed any existing test.

- [ ] **Step 12: Commit**

```bash
git add services/billing/models.go services/billing/errors.go services/billing/repository.go \
        services/billing/db/postgres.go services/billing/service_test.go \
        e2e/critical_path/billing_test.go services/billing/db/outbox_integration_test.go
git commit -m "feat(billing): outbox DLQ + stats repo methods (#134)"
```

---

### Task 3: Relay — guarded attempt cap and gauges

**Files:**
- Modify: `services/billing/outbox_relay.go` (metric block ~35-44, `outboxRepo` ~21-26, `Run` ~58-79, `processBatch` ~85-107)
- Modify: `services/billing/outbox_relay_test.go` (extend `fakeOutboxPublisher`, add cap tests)

**Interfaces:**
- Consumes: `MoveOutboxToDead(ctx, id, errMsg) error` and `OutboxStats(ctx) (*OutboxStats, error)` from Task 2.
- Produces: metrics `thittam_billing_outbox_pending`, `thittam_billing_outbox_oldest_pending_seconds`, `thittam_billing_outbox_dead` (gauges) and `thittam_billing_outbox_dead_lettered_total` (counter). Task 5's alert rules reference these names verbatim.

**The load-bearing idea.** `attempts` alone cannot distinguish a poison payload (one row, permanently bad) from a NATS outage (all rows, temporarily bad). The relay can: **a poison row fails while its batch-mates succeed; an outage fails the whole batch.** A row is dead-lettered only if `attempts >= maxOutboxAttempts` AND at least one sibling in the same batch published. Without that guard, `maxOutboxAttempts = 5` at a 15s tick means a **75-second** NATS outage parks the entire pending backlog — and every one of those tenants then never starts their retention clock.

**Constraint for this task:** pure Go, unit tests only. No database, no Docker.

- [ ] **Step 1: Extend `fakeOutboxPublisher` to fail selectively**

In `services/billing/outbox_relay_test.go`, replace the existing fake (lines 14-22) with one that can fail per-tenant. `Publish` receives `tenantID`, not the event id, so key on that. Keep the old `err` field working so the two existing tests are untouched.

```go
type fakeOutboxPublisher struct {
	published []uuid.UUID
	err       error                  // fails every publish
	failFor   map[uuid.UUID]error    // fails only these tenants
}

func (f *fakeOutboxPublisher) Publish(_ context.Context, _ string, tenantID uuid.UUID, _ interface{}) error {
	if e, ok := f.failFor[tenantID]; ok {
		return e
	}
	if f.err != nil {
		return f.err
	}
	f.published = append(f.published, tenantID)
	return nil
}
```

Note the reordering: a selectively-failed publish must NOT be appended to `published`. The original appended before checking `err`, which was harmless when every publish failed identically. Now it would lie.

- [ ] **Step 2: Write the three failing cap tests**

Append to `services/billing/outbox_relay_test.go`:

```go
// A poison row — fails while a batch-mate succeeds — is dead-lettered once it
// exhausts maxOutboxAttempts.
func TestRelay_PoisonRow_DeadLetteredWhenSiblingSucceeds(t *testing.T) {
	t.Parallel()
	poison := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`), Attempts: maxOutboxAttempts}
	healthy := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`), Attempts: 1}

	var movedID uuid.UUID
	var movedMsg string
	recordedFailure := false
	repo := &mockRepo{
		claimUnsentOutboxFn:   func(_ context.Context, _ int) ([]*OutboxEvent, error) { return []*OutboxEvent{poison, healthy}, nil },
		markOutboxSentFn:      func(_ context.Context, _ uuid.UUID) error { return nil },
		recordOutboxFailureFn: func(_ context.Context, _ uuid.UUID, _ string) error { recordedFailure = true; return nil },
		moveOutboxToDeadFn:    func(_ context.Context, id uuid.UUID, msg string) error { movedID, movedMsg = id, msg; return nil },
	}
	pub := &fakeOutboxPublisher{failFor: map[uuid.UUID]error{poison.TenantID: fmt.Errorf("malformed payload")}}
	r := NewRelay(repo, pub)

	n, err := r.processBatch(context.Background(), 10)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "the healthy sibling still publishes")
	assert.Equal(t, poison.ID, movedID, "poison row must be dead-lettered")
	assert.Contains(t, movedMsg, "malformed payload")
	assert.False(t, recordedFailure, "a dead-lettered row is moved, not merely recorded")
}

// The systemic guard: when the WHOLE batch fails, it is an outage, not poison.
// Nothing is dead-lettered no matter how high attempts has climbed.
func TestRelay_TotalBatchFailure_NeverDeadLetters(t *testing.T) {
	t.Parallel()
	stale := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`), Attempts: 99}
	other := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`), Attempts: 42}

	moved := false
	var recorded []uuid.UUID
	repo := &mockRepo{
		claimUnsentOutboxFn:   func(_ context.Context, _ int) ([]*OutboxEvent, error) { return []*OutboxEvent{stale, other}, nil },
		recordOutboxFailureFn: func(_ context.Context, id uuid.UUID, _ string) error { recorded = append(recorded, id); return nil },
		moveOutboxToDeadFn:    func(_ context.Context, _ uuid.UUID, _ string) error { moved = true; return nil },
	}
	pub := &fakeOutboxPublisher{err: fmt.Errorf("nats down")}
	r := NewRelay(repo, pub)

	n, err := r.processBatch(context.Background(), 10)
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.False(t, moved, "a NATS outage must never park the backlog")
	assert.ElementsMatch(t, []uuid.UUID{stale.ID, other.ID}, recorded)
}

// Below the cap, a failure is recorded and retried, even with a healthy sibling.
func TestRelay_UnderCap_RecordsFailureNotDead(t *testing.T) {
	t.Parallel()
	young := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`), Attempts: maxOutboxAttempts - 1}
	healthy := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`), Attempts: 1}

	moved := false
	var failedID uuid.UUID
	repo := &mockRepo{
		claimUnsentOutboxFn:   func(_ context.Context, _ int) ([]*OutboxEvent, error) { return []*OutboxEvent{young, healthy}, nil },
		markOutboxSentFn:      func(_ context.Context, _ uuid.UUID) error { return nil },
		recordOutboxFailureFn: func(_ context.Context, id uuid.UUID, _ string) error { failedID = id; return nil },
		moveOutboxToDeadFn:    func(_ context.Context, _ uuid.UUID, _ string) error { moved = true; return nil },
	}
	pub := &fakeOutboxPublisher{failFor: map[uuid.UUID]error{young.TenantID: fmt.Errorf("transient")}}
	r := NewRelay(repo, pub)

	_, err := r.processBatch(context.Background(), 10)
	require.NoError(t, err)
	assert.False(t, moved)
	assert.Equal(t, young.ID, failedID)
}

// A MoveOutboxToDead error is logged, not fatal: the tick completes.
func TestRelay_MoveToDeadError_TickContinues(t *testing.T) {
	t.Parallel()
	poison := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`), Attempts: maxOutboxAttempts}
	healthy := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`), Attempts: 1}
	repo := &mockRepo{
		claimUnsentOutboxFn: func(_ context.Context, _ int) ([]*OutboxEvent, error) { return []*OutboxEvent{poison, healthy}, nil },
		markOutboxSentFn:    func(_ context.Context, _ uuid.UUID) error { return nil },
		moveOutboxToDeadFn:  func(_ context.Context, _ uuid.UUID, _ string) error { return fmt.Errorf("db down") },
	}
	pub := &fakeOutboxPublisher{failFor: map[uuid.UUID]error{poison.TenantID: fmt.Errorf("malformed")}}
	r := NewRelay(repo, pub)

	n, err := r.processBatch(context.Background(), 10)
	require.NoError(t, err, "a failed move must not abort the batch")
	assert.Equal(t, 1, n)
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./services/billing/ -run 'TestRelay_(PoisonRow|TotalBatchFailure|UnderCap|MoveToDeadError)' -v`
Expected: FAIL — `undefined: maxOutboxAttempts` (compile error). That is the correct first failure.

- [ ] **Step 4: Add the cap constant and the four metrics**

In `services/billing/outbox_relay.go`, add to the `const` block (lines 28-33):

```go
	maxOutboxAttempts = 5 // dead-letter after this many failed publishes — but only
	                      // when a batch-mate succeeded; see processBatch.
```

And extend the `var (...)` metric block (lines 35-44). These use `promauto` on the default registry, which `promhttp.Handler()` on `:9099` serves — no `cmd/billing/main.go` change is needed.

```go
	outboxDeadLettered = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "thittam", Subsystem: "billing", Name: "outbox_dead_lettered_total",
		Help: "Outbox events moved to the dead-letter queue after exhausting retries.",
	})
	outboxPending = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "thittam", Subsystem: "billing", Name: "outbox_pending",
		Help: "Outbox events awaiting publication.",
	})
	outboxOldestPending = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "thittam", Subsystem: "billing", Name: "outbox_oldest_pending_seconds",
		Help: "Age of the oldest unpublished outbox event, in seconds.",
	})
	outboxDead = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "thittam", Subsystem: "billing", Name: "outbox_dead",
		Help: "Events currently parked in the outbox dead-letter queue.",
	})
```

- [ ] **Step 5: Widen the narrow `outboxRepo` interface**

In `services/billing/outbox_relay.go` (lines 21-26). The relay needs only two of Task 2's four methods; the CLI-only pair stays off this interface.

```go
type outboxRepo interface {
	ClaimUnsentOutbox(ctx context.Context, limit int) ([]*OutboxEvent, error)
	MarkOutboxSent(ctx context.Context, id uuid.UUID) error
	RecordOutboxFailure(ctx context.Context, id uuid.UUID, errMsg string) error
	DeleteSentOutboxOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
	MoveOutboxToDead(ctx context.Context, id uuid.UUID, errMsg string) error
	OutboxStats(ctx context.Context) (*OutboxStats, error)
}
```

- [ ] **Step 6: Rewrite `processBatch` with the guarded cap**

Replace `processBatch` (lines 85-107) entirely:

```go
// outboxFailure pairs a failed event with its publish error so the second pass
// can decide, with whole-batch knowledge, whether it is poison or an outage.
type outboxFailure struct {
	event *OutboxEvent
	err   error
}

// processBatch claims up to limit unsent events and publishes each. Successes
// are marked sent. Failures are held until the batch completes, then either
// dead-lettered (poison: exhausted retries while a sibling published) or
// recorded for retry (outage: nothing in the batch published). Returns the
// count published. A systemic claim error is returned; per-event failures are
// recorded and counted, not returned.
func (r *Relay) processBatch(ctx context.Context, limit int) (int, error) {
	events, err := r.repo.ClaimUnsentOutbox(ctx, limit)
	if err != nil {
		return 0, err
	}

	published := 0
	var failed []outboxFailure
	for _, e := range events {
		if perr := r.pub.Publish(ctx, e.Subject, e.TenantID, json.RawMessage(e.Payload)); perr != nil {
			outboxFailed.Inc()
			failed = append(failed, outboxFailure{event: e, err: perr})
			continue
		}
		if merr := r.repo.MarkOutboxSent(ctx, e.ID); merr != nil {
			r.log.Error("mark outbox sent", "id", e.ID, "error", merr)
			continue
		}
		outboxPublished.Inc()
		published++
	}

	// A batch in which nothing published is an outage (NATS down), not a poison
	// payload. Dead-lettering then would park the entire backlog — and every
	// parked suspension is a tenant whose retention clock never starts.
	canDeadLetter := published > 0

	for _, f := range failed {
		if canDeadLetter && f.event.Attempts >= maxOutboxAttempts {
			if merr := r.repo.MoveOutboxToDead(ctx, f.event.ID, f.err.Error()); merr != nil {
				r.log.Error("move outbox to dead", "id", f.event.ID, "error", merr)
				continue
			}
			outboxDeadLettered.Inc()
			r.log.Error("outbox event dead-lettered",
				"id", f.event.ID, "subject", f.event.Subject, "attempts", f.event.Attempts, "error", f.err)
			continue
		}
		if rerr := r.repo.RecordOutboxFailure(ctx, f.event.ID, f.err.Error()); rerr != nil {
			r.log.Error("record outbox failure", "id", f.event.ID, "error", rerr)
		}
	}

	return published, nil
}
```

Never log `f.event.Payload` — it is tenant data.

- [ ] **Step 7: Set the gauges each tick**

Add a method, and call it from `Run` right after `processBatch` (inside the `case <-ticker.C:` arm, before the cleanup block):

```go
// updateGauges refreshes outbox health metrics. A stats error is logged and
// swallowed: observability must never take down delivery.
func (r *Relay) updateGauges(ctx context.Context) {
	stats, err := r.repo.OutboxStats(ctx)
	if err != nil {
		r.log.Warn("outbox stats", "error", err)
		return
	}
	outboxPending.Set(float64(stats.Pending))
	outboxOldestPending.Set(stats.OldestPendingSeconds)
	outboxDead.Set(float64(stats.Dead))
}
```

The `Run` tick body becomes:

```go
		case <-ticker.C:
			if _, err := r.processBatch(ctx, relayBatchSize); err != nil {
				r.log.Error("outbox batch failed", "error", err)
			}
			r.updateGauges(ctx)
			if ticks++; ticks%relayCleanupEach == 0 {
				if n, err := r.repo.DeleteSentOutboxOlderThan(ctx, time.Now().UTC().Add(-relaySentTTL)); err != nil {
					r.log.Warn("outbox cleanup failed", "error", err)
				} else if n > 0 {
					r.log.Info("outbox cleanup", "deleted", n)
				}
			}
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go test ./services/billing/ -run TestRelay -v`
Expected: PASS, all seven `TestRelay_*` tests (three pre-existing, four new).

- [ ] **Step 9: Whole-tree vet and full unit suite**

Run: `go vet ./... && go test ./services/billing/... -short`
Expected: clean, PASS. (`mockRepo` already gained `moveOutboxToDeadFn` and `outboxStatsFn` in Task 2, so nothing else should break.)

- [ ] **Step 10: Check coverage did not regress**

Run: `go test ./services/billing/ -short -coverprofile=/tmp/cov.out && go tool cover -func=/tmp/cov.out | tail -1`
Expected: total ≥ 80.0% (it was 80.5% before this change; the threshold is 75%).

- [ ] **Step 11: Commit**

```bash
git add services/billing/outbox_relay.go services/billing/outbox_relay_test.go
git commit -m "feat(billing): guarded dead-letter cap + outbox gauges (#134)"
```

---

### Task 4: `cmd/outbox-admin` — operator CLI

**Files:**
- Create: `cmd/outbox-admin/main.go`
- Create: `cmd/outbox-admin/commands.go`

**Interfaces:**
- Consumes: `ListDeadOutbox(ctx, limit) ([]*OutboxEvent, error)` and `ReplayDeadOutbox(ctx, id) error` and `billing.ErrOutboxEventNotFound` from Task 2.
- Produces: a binary, no Go API. Nothing depends on it.

**Precedent:** `cmd/thittam-cli` uses cobra with one `newXxxCmd() *cobra.Command` constructor per subcommand. Follow that, not `purge-worker`'s bare `flag` (which has no subcommands). `github.com/spf13/cobra` is already a module dependency.

**DSN note.** Every Thittam binary reads the same `DATABASE_URL`; owner-vs-runtime is decided at deploy time by which credential is injected. `outbox-admin` does pure DML and must be run with the **runtime** (`thittam_app`) credential. The code cannot enforce that, so say it in the doc block.

**Constraint for this task:** no database, no Docker. Build and `--help` only.

- [ ] **Step 1: Write `main.go`**

```go
// outbox-admin inspects and drains the billing outbox dead-letter queue (#134).
//
// A dead-lettered event is one the relay failed to publish maxOutboxAttempts
// times while other events in its batch succeeded — i.e. the event itself is
// bad, not the broker. Nothing drains event_outbox_dead automatically: a human
// must understand why the event failed before re-arming it. That is the point.
//
// Replaying a suspension event matters. Until it is delivered, iam's consumer
// never starts that tenant's retention clock.
//
// Run with the RUNTIME database credential (thittam_app). This binary performs
// only DML; it does not need — and must not be given — the owner DSN that
// cmd/purge-worker requires.
//
// Exit codes: 0 success; 1 config error (missing DATABASE_URL); 2 runtime error.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	billingdb "github.com/wegofwd2020/thittam/services/billing/db"
)

func main() {
	root := &cobra.Command{
		Use:   "outbox-admin",
		Short: "Inspect and replay billing outbox dead-letter events",
	}
	root.AddCommand(newListCmd(), newReplayCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

// openRepo connects using DATABASE_URL. The caller owns the returned close func.
func openRepo(ctx context.Context) (*billingdb.Postgres, func(), error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "ERROR: DATABASE_URL is required")
		os.Exit(1)
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("ping database: %w", err)
	}
	return billingdb.NewPostgres(pool), pool.Close, nil
}

func cmdContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 60*time.Second)
}
```

- [ ] **Step 2: Write `commands.go`**

`list` prints a tab-aligned table. `replay` takes `--id` or `--all`, exactly one of them. Payload is never printed — it is tenant data. `last_error` is truncated so one enormous broker error cannot flood a terminal.

```go
package main

import (
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/wegofwd2020/thittam/services/billing"
)

const deadListLimit = 100

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List dead-lettered outbox events, most recent first",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := cmdContext()
			defer cancel()

			repo, closeFn, err := openRepo(ctx)
			if err != nil {
				return err
			}
			defer closeFn()

			dead, err := repo.ListDeadOutbox(ctx, deadListLimit)
			if err != nil {
				return err
			}
			if len(dead) == 0 {
				fmt.Println("dead-letter queue is empty")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tSUBJECT\tTENANT\tATTEMPTS\tLAST ERROR")
			for _, e := range dead {
				lastErr := ""
				if e.LastError != nil {
					lastErr = truncate(*e.LastError, 60)
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", e.ID, e.Subject, e.TenantID, e.Attempts, lastErr)
			}
			return w.Flush()
		},
	}
}

func newReplayCmd() *cobra.Command {
	var idFlag string
	var all bool

	cmd := &cobra.Command{
		Use:   "replay",
		Short: "Re-arm dead-lettered events so the relay retries them",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Exactly one of --id / --all. Both set or neither set is an error.
			if (idFlag != "") == all {
				return errors.New("specify exactly one of --id or --all")
			}

			ctx, cancel := cmdContext()
			defer cancel()

			repo, closeFn, err := openRepo(ctx)
			if err != nil {
				return err
			}
			defer closeFn()

			if idFlag != "" {
				id, perr := uuid.Parse(idFlag)
				if perr != nil {
					return fmt.Errorf("invalid --id: %w", perr)
				}
				if rerr := repo.ReplayDeadOutbox(ctx, id); rerr != nil {
					if errors.Is(rerr, billing.ErrOutboxEventNotFound) {
						return fmt.Errorf("no dead event with id %s", id)
					}
					return rerr
				}
				fmt.Printf("replayed %s\n", id)
				return nil
			}

			dead, lerr := repo.ListDeadOutbox(ctx, deadListLimit)
			if lerr != nil {
				return lerr
			}
			if len(dead) == 0 {
				fmt.Println("dead-letter queue is empty; nothing to replay")
				return nil
			}
			for _, e := range dead {
				if rerr := repo.ReplayDeadOutbox(ctx, e.ID); rerr != nil {
					return fmt.Errorf("replay %s: %w", e.ID, rerr)
				}
				fmt.Printf("replayed %s\n", e.ID)
			}
			fmt.Printf("replayed %d event(s)\n", len(dead))
			return nil
		},
	}

	cmd.Flags().StringVar(&idFlag, "id", "", "UUID of a single dead event to replay")
	cmd.Flags().BoolVar(&all, "all", false, "Replay every dead event")
	return cmd
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
```

`replay --all` is recoverable — the events simply re-enter the queue and the relay retries them — so it needs no confirmation prompt. It prints every id it moved.

- [ ] **Step 3: Build it**

Run: `go build ./cmd/outbox-admin/`
Expected: clean, no output. (Delete the resulting `outbox-admin` binary if it lands in the repo root — it is not gitignored by name.)

- [ ] **Step 4: Verify the CLI surface without a database**

Run: `go run ./cmd/outbox-admin --help`
Expected: usage listing both `list` and `replay` subcommands. No DB connection is attempted, because `openRepo` is called inside `RunE`, not at construction.

Run: `go run ./cmd/outbox-admin replay --id abc --all`
Expected: `Error: specify exactly one of --id or --all`, exit code 2. Still no DB connection — the flag check precedes `openRepo`.

- [ ] **Step 5: Whole-tree vet**

Run: `go vet ./...`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add cmd/outbox-admin/main.go cmd/outbox-admin/commands.go
git commit -m "feat(billing): cmd/outbox-admin — list and replay dead-lettered events (#134)"
```

---

### Task 5: Prometheus — enable the billing scrape, add three alerts

**Files:**
- Modify: `infra/prometheus/prometheus.yml:129-132` (the commented `billing` stub) and the port-map comment at ~53-63
- Modify: `infra/prometheus/alerts/billing.yaml:18` (stale header note) and append rules to group `thittam_billing`

**Interfaces:**
- Consumes: metric names from Task 3 — `thittam_billing_outbox_dead`, `thittam_billing_outbox_oldest_pending_seconds`, `thittam_billing_outbox_failed_total`. These must match verbatim.
- Produces: nothing Go-facing.

**Why this task exists at all.** The `billing` scrape job is currently commented out with the note "cmd/billing not yet created". `cmd/billing/main.go` exists and sets `MetricsPort: 9099`. Both counters shipped in #126 are exported and never collected. Every metric in Task 3 is likewise invisible until this lands.

**Constraint for this task:** YAML only. No Docker, no Prometheus, no `infra/local/`. Validate with `promtool` if it happens to be installed; otherwise rely on CI.

- [ ] **Step 1: Enable the scrape job**

In `infra/prometheus/prometheus.yml`, replace lines 129-132:

```yaml
  # billing: cmd/billing not yet created; add port 9099 when wired up.
  # - job_name: billing
  #   static_configs:
  #     - targets: ["host.docker.internal:9099"]
```

with, matching the `notifications` / `document` block style exactly:

```yaml
  - job_name: billing
    static_configs:
      - targets: ["host.docker.internal:9099"]
    relabel_configs:
      - target_label: service
        replacement: billing
```

Also fix the port-map comment near lines 53-63: change the `TBD → 9099 billing` entry to just `9099 billing`.

- [ ] **Step 2: Fix the stale note in the alerts file header**

In `infra/prometheus/alerts/billing.yaml`, replace line 18-19:

```yaml
  # Note: billing metrics port is 9099 (TBD — cmd/billing not yet created).
  # These alerts will become active once the billing service is wired up.
```

with:

```yaml
  # Billing metrics are served on port 9099 (see cmd/billing/main.go,
  # server.Config{MetricsPort: 9099}) and scraped as job="billing".
  #
  # Outbox relay metrics (#134), defined in services/billing/outbox_relay.go:
  #
  #   thittam_billing_outbox_published_total
  #   thittam_billing_outbox_failed_total
  #   thittam_billing_outbox_dead_lettered_total
  #   thittam_billing_outbox_pending              (gauge)
  #   thittam_billing_outbox_oldest_pending_seconds (gauge)
  #   thittam_billing_outbox_dead                 (gauge)
```

- [ ] **Step 3: Append the three alert rules**

At the end of the `thittam_billing` group's `rules:` list in `infra/prometheus/alerts/billing.yaml` (after `BillingServiceErrorRateElevated`, which ends at line 118). Match the existing shape: multiline `expr: |`, `for:`, `labels` with `severity`/`team`/`component`, `annotations` with `summary`/`description`/`runbook_url`.

```yaml
      # ── Outbox dead-letter queue non-empty ───────────────────────────────
      # A dead-lettered event is one the relay could not publish after
      # maxOutboxAttempts tries while OTHER events in its batch published
      # fine — so the event itself is bad, not the broker. Nothing drains
      # this queue automatically.
      #
      # This is a correctness alert, not hygiene. The only event on the
      # outbox today is subscription.suspended; until it is delivered, iam's
      # consumer never starts that tenant's retention clock, and the tenant
      # keeps its data past the retention window. Silently.
      - alert: BillingOutboxDeadLetter
        expr: |
          thittam_billing_outbox_dead > 0
        for: 5m
        labels:
          severity: critical
          team: platform
          component: billing
        annotations:
          summary: "{{ $value }} billing outbox event(s) dead-lettered — delivery has stopped for them"
          description: >
            {{ $value }} event(s) are parked in event_outbox_dead. Each one is
            a domain event that will never be delivered until a human replays
            it. For subscription.suspended events this means the affected
            tenant's retention clock never starts.
            Inspect with `outbox-admin list` (run with the runtime DSN), fix
            the underlying cause, then `outbox-admin replay --id <uuid>`.
          runbook_url: "https://github.com/wegofwd2020-hub/thittam_docs/blob/main/docs/developers/operations/runbook.md#billing-outbox-dead-letter"

      # ── Outbox backlog going stale ───────────────────────────────────────
      # The relay ticks every 15s and claims 100 rows, so a healthy oldest-
      # pending age stays well under a minute. 15 minutes means the relay is
      # not running, NATS is down, or publishes are failing wholesale (the
      # systemic guard deliberately does NOT dead-letter in that case, so the
      # backlog simply grows — this alert is what surfaces it).
      - alert: BillingOutboxBacklogStale
        expr: |
          thittam_billing_outbox_oldest_pending_seconds > 900
        for: 10m
        labels:
          severity: warning
          team: platform
          component: billing
        annotations:
          summary: "Oldest unpublished billing outbox event is {{ $value | humanizeDuration }} old (> 15m)"
          description: >
            The billing outbox relay has not drained its backlog. Either the
            relay goroutine is not running (check that cmd/billing started with
            NATS configured — the relay is gated on a non-nil publisher), or
            NATS is unreachable.
            Check thittam_billing_outbox_pending for depth and the billing
            service logs for "outbox batch failed".
          runbook_url: "https://github.com/wegofwd2020-hub/thittam_docs/blob/main/docs/developers/operations/runbook.md#billing-outbox-backlog"

      # ── Outbox publish failures ──────────────────────────────────────────
      # Any sustained publish failure. Transient NATS blips resolve within a
      # tick or two, so the 15m `for` avoids paging on noise while still
      # catching a broker that is down but not yet stale enough to trip
      # BillingOutboxBacklogStale.
      - alert: BillingOutboxPublishFailing
        expr: |
          increase(thittam_billing_outbox_failed_total[15m]) > 0
        for: 15m
        labels:
          severity: warning
          team: platform
          component: billing
        annotations:
          summary: "Billing outbox publishes failing ({{ $value | printf \"%.0f\" }} in 15m)"
          description: >
            The outbox relay is failing to publish events to NATS. Events are
            retried on each 15s tick and are not lost. If the whole batch is
            failing, nothing is dead-lettered by design — the backlog grows
            until NATS recovers.
            Check NATS JetStream health and the FINANCIAL stream.
          runbook_url: "https://github.com/wegofwd2020-hub/thittam_docs/blob/main/docs/developers/operations/runbook.md#billing-outbox-publish-failures"
```

- [ ] **Step 4: Validate the YAML parses**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('infra/prometheus/alerts/billing.yaml')); yaml.safe_load(open('infra/prometheus/prometheus.yml')); print('ok')"`
Expected: `ok`

If `promtool` is installed, also run `promtool check rules infra/prometheus/alerts/billing.yaml` and expect `SUCCESS: 7 rules found`. If it is not installed, skip — do not install it, and do not start Prometheus.

- [ ] **Step 5: Confirm the metric names match Task 3 exactly**

Run: `grep -oE 'thittam_billing_outbox_[a-z_]+' infra/prometheus/alerts/billing.yaml | sort -u`
Then: `grep -oE 'Name: "outbox_[a-z_]+"' services/billing/outbox_relay.go | sort -u`
Expected: every name referenced by an `expr:` (`thittam_billing_outbox_dead`, `thittam_billing_outbox_oldest_pending_seconds`, `thittam_billing_outbox_failed_total`) has a corresponding `Namespace: "thittam", Subsystem: "billing", Name: "outbox_…"` declaration. A typo here produces an alert that never fires and no compiler will tell you.

- [ ] **Step 6: Commit**

```bash
git add infra/prometheus/prometheus.yml infra/prometheus/alerts/billing.yaml
git commit -m "feat(infra): scrape billing :9099 and alert on outbox DLQ/backlog (#134)"
```

---

## Verification (whole branch, before PR)

- [ ] `go vet ./...` — clean. This is the only check that catches the e2e double.
- [ ] `go test ./... -short` — PASS.
- [ ] `go test -race ./services/billing/...` — PASS.
- [ ] `go vet -tags=integration ./services/billing/...` — clean (integration tests compile).
- [ ] `go test ./services/billing/ -short -coverprofile=/tmp/cov.out && go tool cover -func=/tmp/cov.out | tail -1` — ≥ 80.0%.
- [ ] `gofmt -l services/billing cmd/outbox-admin` — lists nothing **among files this branch touched**. Note: `models.go`, `db/postgres.go`, `service_test.go`, and `billing_test.go` have a pre-existing gofmt gap that predates this work (recorded in #126's ledger). Do not reformat them wholesale — that buries the diff. Confirm only that the lines *you added* are gofmt-clean.
- [ ] `git log --oneline` — five commits, each building green on its own.

## Deploy-time notes (not code)

- `outbox-admin` must be run with the **runtime** (`thittam_app`) credential in `DATABASE_URL`, never the owner DSN. It needs `SELECT`/`INSERT`/`DELETE` on `event_outbox` and `event_outbox_dead`. Confirm the `thittam_app` grants cover the new table — migration `003` creates it as the owner, so `thittam_app` may need an explicit `GRANT` depending on how default privileges are configured. **Check this before the alert can be actioned.**
- `BillingOutboxDeadLetter` is `severity: critical` and routes to a pager. Confirm Alertmanager routing in prod, which is configured outside this repo.
- The runbook anchors referenced by the three new alerts (`#billing-outbox-dead-letter`, `#billing-outbox-backlog`, `#billing-outbox-publish-failures`) do not exist yet in `thittam_docs`. File a docs issue.

## Known pre-existing drift, deliberately not fixed here

`infra/prometheus/alerts/billing.yaml` already contains three alerts firing on `billing_dunning_attempts_total`, `billing_subscriptions_total`, and `billing_plan_limit_rejections_total` — **none of which any service defines**. They have been harmless because nothing was scraped. Enabling the job does not make them fire (absent metrics do not alert), but it does mean the file now holds three rules that can never trigger. Out of scope; worth its own issue.
