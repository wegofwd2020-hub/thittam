# Billing outbox observability + dead-letter queue (#134)

**Status:** approved (design), 2026-07-09
**Issue:** [#134](https://github.com/wegofwd2020-hub/thittam/issues/134)
**Follow-up from:** #126 (billing transactional outbox), `docs/superpowers/specs/2026-07-08-billing-outbox-126-design.md`

## 1. Problem

The outbox relay (`services/billing/outbox_relay.go`) retries a failing publish every
15s tick forever. `attempts` grows unbounded, `last_error` is written, and nothing
surfaces the row. Cleanup only deletes `sent_at IS NOT NULL`, so a permanently-failing
("poison") event sits unsent indefinitely with no signal.

Two gaps compound each other:

1. **No signal.** The relay exports `thittam_billing_outbox_published_total` and
   `thittam_billing_outbox_failed_total`, but no gauge for depth or age, and no alert.
2. **Nothing is scraped.** `infra/prometheus/prometheus.yml` still has the `billing`
   job commented out with the note "cmd/billing not yet created". `cmd/billing/main.go`
   exists and sets `MetricsPort: 9099`. Both counters shipped in #126 are exported into
   the void. The stale note is repeated in `infra/prometheus/alerts/billing.yaml:18`.

Consequence: a stuck `thittam.billing.subscription.suspended` event means iam's consumer
(#118) never starts that tenant's retention clock. The tenant keeps its data past the
retention window, silently and indefinitely.

## 2. Scope

In scope:

- Migration `003` adding an `event_outbox_dead` table.
- Attempt cap with a systemic-failure guard, moving poison rows to the DLQ.
- Three new gauges, one new counter.
- Three Prometheus alert rules.
- Enabling the `billing` scrape job.
- `cmd/outbox-admin` — an operator CLI to list and replay dead events.

Out of scope:

- Automatic replay from the DLQ (deliberate: see §4).
- Changes to `ClaimUnsentOutbox`, its partial index, or any SQL shipped in #126.
- Changes to the iam consumer, which is already at-least-once safe.

## 3. Delivery semantics — the property this changes

Today the outbox is **eventually-delivering**: an event cannot be lost, only delayed,
because the relay retries forever. Capping attempts makes delivery
**best-effort-then-parked**. A dead-lettered event is not delivered until a human
replays it.

This is an accepted trade, but it makes the exit path load-bearing. Two consequences
are designed around rather than tolerated:

- `BillingOutboxDeadLetter` is a **correctness** alert, not a hygiene one. A parked
  suspension means a tenant's retention clock never starts. It pages.
- The cap must never fire during a NATS outage, which is the failure mode #126
  identified as realistic. See §5.

## 4. Replay is human-triggered

A DLQ nobody can drain is a slower silence. Replay is a deliberate operator act —
someone must decide *why* the event failed before re-arming it — but the dangerous
part, the move, is code with a test rather than hand-written SQL against production.

Rejected: automatic timed retry from the DLQ. If the failure is permanent it retries
forever anyway, which is where we started, having paid for a table.

## 5. Cap semantics — poison vs outage

`attempts` alone cannot distinguish a poison payload (one row, permanently bad) from a
NATS outage (all rows, temporarily bad). Under a naive cap the two are indistinguishable
and the outage is catastrophic: every unsent event is claimed each tick, every one
fails, every `attempts` climbs in lockstep. At a cap of 5 with a 15s tick, a
**75-second** outage parks the entire pending backlog at once.

The relay can tell them apart. **A poison row fails while its batch-mates succeed; an
outage fails the whole batch.**

Rule: a row is dead-lettered only when

```
event.Attempts >= maxOutboxAttempts   AND   at least one sibling in the same batch published
```

`maxOutboxAttempts = 5`. If the whole batch failed, the tick records `last_error` and
increments `outbox_failed_total` as today, and dead-letters nothing regardless of
`attempts`.

Accepted cost: a lone poison row with no other traffic is never dead-lettered, because
its batch is always a total failure. `BillingOutboxBacklogStale` fires on it regardless
(§8), so it is visible, which is the primary ask of #134.

Note `attempts` is incremented **at claim time** by `ClaimUnsentOutbox`
(`attempts = attempts + 1` inside the `UPDATE ... RETURNING`), not on failure.
`RecordOutboxFailure` writes only `last_error`. So the relay already holds the
post-increment count on the returned `*OutboxEvent` — the cap needs no extra query.

## 6. Schema — migration `003_event_outbox_dead`

Purely additive. `event_outbox`, `idx_event_outbox_unsent`, and all five #126 repo
methods are untouched. Dead rows move *out* of the hot table, so the claim query and its
partial index never learn the DLQ exists.

```sql
-- 003_event_outbox_dead.up.sql
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
CREATE INDEX idx_event_outbox_dead_died_at ON event_outbox_dead (died_at);
```

No `sent_at` column: a dead row is by definition unsent. `id` and `created_at` carry
over so an operator can correlate a parked event with its original.

`003_event_outbox_dead.down.sql` is `DROP TABLE IF EXISTS event_outbox_dead;`. Unlike
migration `019`'s enum narrowing, this down is unconditionally safe — nothing references
the table.

## 7. Repository — four new methods

Widening `Repository` breaks three implementers (see §11), so the three gauge reads are
folded into **one** stats call rather than three separate counts.

```go
// services/billing/repository.go — appended to the Outbox block
MoveOutboxToDead(ctx context.Context, id uuid.UUID, errMsg string) error
OutboxStats(ctx context.Context) (*OutboxStats, error)
ListDeadOutbox(ctx context.Context, limit int) ([]*OutboxEvent, error)
ReplayDeadOutbox(ctx context.Context, id uuid.UUID) error
```

```go
// services/billing/models.go
type OutboxStats struct {
    Pending              int64
    OldestPendingSeconds float64
    Dead                 int64
}
```

**`MoveOutboxToDead`** — one transaction (`Begin` / `defer _ = tx.Rollback(ctx)` /
`Commit`, matching the ledger pattern):

```sql
INSERT INTO event_outbox_dead (id, subject, tenant_id, payload, created_at, attempts, last_error, died_at)
SELECT id, subject, tenant_id, payload, created_at, attempts, $2, now()
  FROM event_outbox WHERE id = $1;
DELETE FROM event_outbox WHERE id = $1;
```

Zero rows inserted means another relay instance already moved it. Benign — commit and
return nil. Idempotent by construction.

**`OutboxStats`** — a single round trip; the first two subqueries ride the existing
partial index:

```sql
SELECT
  (SELECT count(*) FROM event_outbox WHERE sent_at IS NULL),
  (SELECT coalesce(extract(epoch FROM now() - min(created_at)), 0)
     FROM event_outbox WHERE sent_at IS NULL),
  (SELECT count(*) FROM event_outbox_dead);
```

**`ListDeadOutbox`** — `SELECT ... FROM event_outbox_dead ORDER BY died_at DESC LIMIT $1`.

**`ReplayDeadOutbox`** — the mirror transaction. `attempts` resets to 0, `sent_at` and
`last_error` are NULL, `created_at` is preserved so age-based alerts stay honest:

```sql
INSERT INTO event_outbox (id, subject, tenant_id, payload, created_at, sent_at, attempts, last_error)
SELECT id, subject, tenant_id, payload, created_at, NULL, 0, NULL
  FROM event_outbox_dead WHERE id = $1;
DELETE FROM event_outbox_dead WHERE id = $1;
```

Zero rows moved → `ErrOutboxEventNotFound`, so the CLI can report an unknown id rather
than silently succeeding.

## 8. Relay changes

`processBatch` collects successes and failures, then applies the guarded cap:

```go
canDeadLetter := published > 0   // §5 systemic guard
for _, f := range failed {
    if canDeadLetter && f.event.Attempts >= maxOutboxAttempts {
        if err := r.repo.MoveOutboxToDead(ctx, f.event.ID, f.err.Error()); err != nil {
            r.log.Error("move outbox to dead", "id", f.event.ID, "error", err)
            continue
        }
        outboxDeadLettered.Inc()
        r.log.Error("outbox event dead-lettered",
            "id", f.event.ID, "subject", f.event.Subject, "attempts", f.event.Attempts)
        continue
    }
    if err := r.repo.RecordOutboxFailure(ctx, f.event.ID, f.err.Error()); err != nil {
        r.log.Error("record outbox failure", "id", f.event.ID, "error", err)
    }
}
```

`const maxOutboxAttempts = 5`.

`Run` calls `OutboxStats` once per tick, after `processBatch`, and sets the gauges. A
stats error is logged at warn and does not abort the tick — observability must never
take down delivery.

The narrow `outboxRepo` interface in `outbox_relay.go` gains `MoveOutboxToDead` and
`OutboxStats` (not the two CLI-only methods).

### Metrics

All via `promauto` on the default registry, alongside the two existing counters, so
`promhttp.Handler()` on `:9099` serves them with no `cmd/billing/main.go` change.

| Metric | Type | Purpose |
|---|---|---|
| `thittam_billing_outbox_pending` | gauge | #134 ask 2 — unsent depth |
| `thittam_billing_outbox_oldest_pending_seconds` | gauge | outage / stalled-relay signal |
| `thittam_billing_outbox_dead` | gauge | current DLQ depth |
| `thittam_billing_outbox_dead_lettered_total` | counter | *that* something died — survives an operator draining the gauge to 0 |

## 9. Scrape + alerts

**Prerequisite.** Enable the job in `infra/prometheus/prometheus.yml`, replacing the
commented stub, matching the `notifications`/`document` block style:

```yaml
  - job_name: billing
    static_configs:
      - targets: ["host.docker.internal:9099"]
    relabel_configs:
      - target_label: service
        replacement: billing
```

Also correct the stale "cmd/billing not yet created" notes in `prometheus.yml` and in
the `billing.yaml` header comment (line 18).

**Rules** appended to group `thittam_billing` in `infra/prometheus/alerts/billing.yaml`,
following the `retention-sweeper.yaml` precedent (gauge threshold + `for:` dwell,
`severity`/`team`/`component` labels, `summary`/`description`/`runbook_url` annotations):

| Alert | Expr | For | Severity |
|---|---|---|---|
| `BillingOutboxDeadLetter` | `thittam_billing_outbox_dead > 0` | 5m | **critical** |
| `BillingOutboxBacklogStale` | `thittam_billing_outbox_oldest_pending_seconds > 900` | 10m | warning |
| `BillingOutboxPublishFailing` | `increase(thittam_billing_outbox_failed_total[15m]) > 0` | 15m | warning |

`BillingOutboxDeadLetter` is `critical` per §3: a parked event means a tenant's
retention clock never starts. Its description must state that, and point at
`outbox-admin list`.

## 10. `cmd/outbox-admin`

A small operator binary in the shape of `cmd/purge-worker`. Human-invoked; no CronJob
manifest, no k8s object.

```
outbox-admin list                    # id, subject, tenant, attempts, died_at, last_error
outbox-admin replay --id <uuid>
outbox-admin replay --all
```

Reads `DATABASE_URL`. This is the **runtime** DSN (`thittam_app`) — pure DML on billing
tables, no DDL. It deliberately does *not* need the owner role that `cmd/purge-worker`
requires for `DROP SCHEMA`.

`replay --all` is recoverable (the events simply re-enter the queue) and so needs no
two-person guard, but it prints the count and the ids it moved.

Logging is `slog`, structured, no payload contents — a payload is tenant data.
`last_error` is safe to print; `payload` is not.

## 11. Blast radius

Adding to `Repository` breaks three implementers. All three must gain the four methods:

1. `*Postgres` — `services/billing/db/postgres.go` (compile assertion at line 32)
2. `mockRepo` — `services/billing/service_test.go` (fn-field pattern), reused by
   `handler_test.go` and `outbox_relay_test.go`
3. `billingRepo` — `e2e/critical_path/billing_test.go` (e2e double)

A whole-tree `go vet ./...` is the gate; `go build` and focused tests miss the e2e
double. This exact trap was recorded for `iam.Repository`.

## 12. Testing

**Unit** (`services/billing/outbox_relay_test.go`; extend `fakeOutboxPublisher` to fail
selectively by event id):

- one event fails at `attempts = 5` while a sibling publishes → `MoveOutboxToDead`,
  `outbox_dead_lettered_total` incremented
- **every** event fails, one at `attempts = 99` → nothing dead-lettered,
  `RecordOutboxFailure` only. This is the systemic guard, and the most important test
  in the change.
- one fails at `attempts = 4` with a sibling publishing → `RecordOutboxFailure`, not dead
- `MoveOutboxToDead` returning an error → logged, tick continues, no panic
- `OutboxStats` returning an error → tick still publishes

**Integration** (`services/billing/db/outbox_integration_test.go`, `//go:build integration`,
`testdb.Open(t)`):

- move is atomic, preserves `id` and `created_at`, removes the row from `event_outbox`
- a moved row is no longer claimable by `ClaimUnsentOutbox`
- replay restores with `attempts = 0` and the row *is* claimable again
- replay of an unknown id → `ErrOutboxEventNotFound`
- `OutboxStats` returns correct `(pending, oldest, dead)` triples

**DB verification constraint.** Per `CLAUDE.md`: never run `docker compose … -v`/`down`/`up`
against `infra/local/`. Use a disposable uniquely-named throwaway container or `pkg/testdb`
(which SKIPs without `THITTAM_TEST_DSN`). CI's real-Postgres job is the authoritative
up/down gate.

Coverage: billing threshold is ≥ 75%; the branch currently sits at 80.5% and must not regress.

## 13. Files

| File | Change |
|---|---|
| `migrations/billing/003_event_outbox_dead.{up,down}.sql` | new |
| `services/billing/models.go` | `OutboxStats` struct |
| `services/billing/repository.go` | +4 interface methods |
| `services/billing/db/postgres.go` | +4 implementations (2 transactional) |
| `services/billing/errors.go` | `ErrOutboxEventNotFound` |
| `services/billing/outbox_relay.go` | guarded cap, 3 gauges, 1 counter, widened `outboxRepo` |
| `services/billing/outbox_relay_test.go` | selective-failure fake + cap tests |
| `services/billing/service_test.go` | `mockRepo` +4 fn-fields |
| `e2e/critical_path/billing_test.go` | `billingRepo` +4 methods |
| `services/billing/db/outbox_integration_test.go` | move/replay/stats tests |
| `cmd/outbox-admin/main.go` | new operator CLI |
| `infra/prometheus/prometheus.yml` | enable `billing` job, fix stale note |
| `infra/prometheus/alerts/billing.yaml` | 3 rules, fix stale header note |

## 14. Open follow-ups (not this change)

- **Batch saturation defeats the guard.** §5's accepted cost is stated for a *lone*
  poison row. It generalises: `ClaimUnsentOutbox` orders by `created_at`, so if the
  100 oldest unsent rows are all poison, every batch is entirely poison, `published`
  is 0, and the guard correctly declines to dead-letter — forever. The same rows are
  re-claimed each tick and newer healthy events behind them are never reached. Only
  `BillingOutboxBacklogStale` (warning) surfaces this; `BillingOutboxDeadLetter`
  never fires, because nothing is ever parked. Requires ≥ `relayBatchSize` simultaneous
  poison rows, implausible with one event type on the outbox today. A fix would need
  the guard to consider evidence of broker health beyond the current batch — e.g. a
  recent successful publish anywhere, rather than a successful sibling.
- `event_outbox_dead` has no retention/cleanup. Intentional: the table is expected to
  hold zero rows, and silently deleting an undelivered event is precisely the failure
  this change exists to prevent. Revisit only if a real DLQ backlog ever accumulates.
- Relay shutdown is still not awaited (`pool.Close` may race an in-flight batch; logged
  only, at-least-once absorbs it). Carried over from #126, unchanged here.
- `billing.yaml` alerts reference `billing_dunning_attempts_total`,
  `billing_subscriptions_total`, and `billing_plan_limit_rejections_total`, none of which
  any service defines today. Pre-existing drift, surfaced by enabling the scrape job.
  Worth its own issue; out of scope here.
