# Design: billing transactional outbox for subscription.suspended (#126)

**Issue:** #126 (reliability gap surfaced by the #118 final review). **Date:** 2026-07-08
**Scope:** billing only — new outbox table + in-tx write + in-process relay in `cmd/billing`.
**Branch:** `feat/billing-outbox-126` (to be created)
**Depends on:** #118 (the billing→iam suspend event pipe this hardens; merged).

## Context

`SuspendSubscription` (`services/billing/service.go:134-158`) commits the suspend, then publishes
`subscription.suspended` **best-effort** — a crash between the commit and the NATS publish loses
the event permanently (at-most-once on the publish edge). The subscription is suspended in
billing but iam's retention clock never starts: the tenant stays `active` and is never purged.
Revenue/compliance-relevant.

**Fix = transactional outbox.** Write the event to an outbox table in the SAME transaction as the
suspend (atomic: both or neither); an in-process relay publishes unsent rows and marks them sent.

**Three facts from reconnaissance make this clean:**
1. **The consumer is already at-least-once-safe.** iam's `BillingConsumer.Handle`
   (`services/iam/billing_consumer.go:48-66`) transitions only an `active` tenant; a duplicate
   delivery finds it non-active and Acks as a no-op. So the outbox relay can deliver
   at-least-once with **no consumer change**.
2. **Publish is synchronous with a stream ack.** `pkg/jetstream/publisher.go:38-59` calls the
   blocking `js.Publish`, which returns an error unless the FINANCIAL stream durably persisted
   the message. A relay that marks a row `sent` only after `Publish` returns nil loses nothing;
   worst case is a redelivery the consumer absorbs.
3. **The atomic write is available.** Billing's `UpdateSubscription` is a single `p.db.Exec`
   (`services/billing/db/postgres.go:95-120`), but `p.db` is a `*pgxpool.Pool` supporting
   `Begin` — the suspend UPDATE + an outbox INSERT can share one `pgx.Tx` (ledger's
   `Begin`/`WithTx`/`Commit` pattern, `services/ledger/db/postgres.go:176-215`).

**No outbox/dedup/relay precedent exists** anywhere in the repo — this is net-new.

**⚠️ Migration-drift note:** billing's migrations (`001`) are already behind the live code (code
reads columns/tables/statuses no migration defines — `trial_ends_at`, `payment_methods`, etc.).
The outbox is a brand-new table, so it's unaffected, but the migration `002` author must NOT
assume `001` mirrors the live schema.

## Component 1 — migration `002_event_outbox` (`migrations/billing/`)

Next number is 002 (latest is 001). `.up.sql`:

```sql
CREATE TABLE event_outbox (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    subject    TEXT        NOT NULL,   -- e.g. thittam.billing.subscription.suspended
    tenant_id  UUID        NOT NULL,
    payload    JSONB       NOT NULL,   -- the event payload (e.g. BillingSubscriptionSuspendedPayload)
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at    TIMESTAMPTZ,            -- NULL until the relay publishes it
    attempts   INTEGER     NOT NULL DEFAULT 0,
    last_error TEXT
);

-- The relay's poll: unsent rows, oldest first.
CREATE INDEX idx_event_outbox_unsent
    ON event_outbox (created_at)
    WHERE sent_at IS NULL;
```

`.down.sql`: `DROP INDEX IF EXISTS idx_event_outbox_unsent; DROP TABLE IF EXISTS event_outbox;`

Generic (`subject` column) so future billing events reuse the same table + relay.

## Component 2 — transactional write (`services/billing`)

`SuspendSubscription` (`service.go`) no longer publishes best-effort. It builds the payload and
calls a new repo method that performs both writes atomically:

- Service builds `events.BillingSubscriptionSuspendedPayload{SubscriptionID, SuspendedAt,
  PurgeAfter}` (the shape today's `cmd/billing` adapter builds, `cmd/billing/main.go:129-140`)
  and marshals it, then calls `repo.SuspendSubscriptionWithOutbox(ctx, sub, subject, payloadJSON)`.
- `services/billing` imports `pkg/events` for `SubjectBillingSubscriptionSuspended` +
  `BillingSubscriptionSuspendedPayload`. No import cycle — the #118 cycle was `jetstream`↔`billing`;
  `pkg/events` is lower-level (no service deps).

New `*Postgres` method (ledger tx pattern):

```go
func (p *Postgres) SuspendSubscriptionWithOutbox(ctx, sub *Subscription, subject string, payload []byte) error {
    tx := p.db.Begin(ctx); defer tx.Rollback(ctx) //nolint:errcheck
    tx.Exec(ctx, `UPDATE subscriptions SET status='suspended', suspended_at=$2, updated_at=$3 WHERE tenant_id=$1`, ...)
    tx.Exec(ctx, `INSERT INTO event_outbox (subject, tenant_id, payload) VALUES ($1, $2, $3)`, subject, sub.TenantID, payload)
    return tx.Commit(ctx)
}
```

The `billing.EventPublisher` interface + `WithPublisher` (`service.go:14-29`, `events.go:5-9`) and
the `cmd/billing` adapter's suspend-publish are **removed** — the relay owns publishing.

## Component 3 — relay (`services/billing/outbox_relay.go`, goroutine in `cmd/billing`)

A `Relay` type holding a `*pgxpool.Pool` and a narrow publisher interface
(`Publish(ctx, subject string, tenantID uuid.UUID, payload interface{}) error` — satisfied by
`*jetstream.Publisher`). Ticker-driven (~15s). Each tick, in a transaction:

```sql
SELECT id, subject, tenant_id, payload FROM event_outbox
 WHERE sent_at IS NULL ORDER BY created_at
 FOR UPDATE SKIP LOCKED LIMIT 100;
```
For each row: `pub.Publish(subject, tenant_id, payload)` (synchronous stream ack) — on success
`UPDATE event_outbox SET sent_at=now() WHERE id=$1`; on error `UPDATE ... SET
attempts=attempts+1, last_error=$2 WHERE id=$1` (row stays unsent → retried next tick). `FOR
UPDATE SKIP LOCKED` makes it safe across billing replicas.

**Publish payload:** the relay unmarshals the stored `payload` JSONB and republishes via
`pub.Publish`, which mints a fresh envelope `EventID` per attempt. That's fine — consumer dedup is
**domain-state** (tenant status), not `EventID`.

**Cleanup:** every Nth tick, `DELETE FROM event_outbox WHERE sent_at IS NOT NULL AND sent_at <
now() - INTERVAL '7 days'` — keeps the table bounded without a separate job.

**Wiring** (`cmd/billing/main.go`, before `srv.Run()`): construct the relay with the existing
`pool` + `jetstream.Publisher`, start `go relay.Run(ctx)`; started only when NATS is configured
(mirrors today's `pub != nil` gate, `cmd/billing/main.go:43-70`). Graceful stop on ctx cancel.
Precedent: iam runs in-process background goroutines (impersonation-expiry ticker + NATS
consumer, `cmd/iam/main.go:188-238`).

## Component 4 — delivery semantics

At-least-once. A crash after a successful `Publish` but before the `sent_at` update →
redelivery → iam consumer finds the tenant non-`active` → no-op Ack. Publish failure (NATS down)
is transient and retried on every tick indefinitely — correct; there is no poison concept on the
publish edge (the FINANCIAL stream's DLQ is a consumer-side construct).

## Component 5 — metrics

Billing already exposes a metrics port (`cmd/billing/main.go:91`, MetricsPort 9099). Register on
the in-process registry (NOT the CronJob pushgateway path): counters
`thittam_billing_outbox_published_total`, `thittam_billing_outbox_failed_total`, and a gauge
`thittam_billing_outbox_pending` (unsent count per tick).

## Testing (billing ≥ 80%)

- **Service unit:** `SuspendSubscription` calls `SuspendSubscriptionWithOutbox` with the correct
  subject + a payload that round-trips to `BillingSubscriptionSuspendedPayload` (fake repo
  captures the args); it no longer references a publisher.
- **Relay unit:** an unsent row → `Publish` called → marked `sent`; a `Publish` error → row left
  unsent, `attempts` incremented, `last_error` set (fake publisher + fake/real repo); cleanup
  deletes only old sent rows.
- **Integration (real Postgres, `-tags=integration`):** `SuspendSubscriptionWithOutbox` writes
  BOTH rows on commit and NEITHER on a forced rollback (atomicity — the core guarantee); the
  `FOR UPDATE SKIP LOCKED` poll returns unsent rows oldest-first.

## Non-goals

- Generic `pkg/outbox` — billing-scoped now (billing is the only publisher); extract when a
  second publisher needs it. The table's `subject` column already generalizes within billing.
- Reconciliation sweep — the atomic outbox makes it unnecessary for this event.
- Changing the iam consumer, the FINANCIAL stream, or other services' best-effort publishers.
- Keeping an immediate best-effort publish alongside the outbox — outbox-only; the ≤~15s relay
  latency is negligible against 30/90/180-day retention windows and avoids double-publish churn.

## Files touched

`migrations/billing/002_event_outbox.{up,down}.sql`; `services/billing/service.go` (suspend
rewrite, drop publisher); `services/billing/db/postgres.go` (`SuspendSubscriptionWithOutbox` +
outbox read/mark/cleanup methods); `services/billing/repository.go` (interface); new
`services/billing/outbox_relay.go`; `services/billing/events.go` (remove `EventPublisher`);
`cmd/billing/main.go` (wire relay, drop the suspend-publish adapter path); plus the test files.
