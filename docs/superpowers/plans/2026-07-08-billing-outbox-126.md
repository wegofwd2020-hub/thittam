# Billing transactional outbox (#126) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the billing `subscription.suspended` edge crash-safe — write the event to an outbox table in the same tx as the suspend, and publish it from an in-process relay in `cmd/billing`.

**Architecture:** `SuspendSubscription` writes the suspend + an `event_outbox` row atomically (one tx). A ticker-driven relay goroutine in `cmd/billing` claims unsent rows (`UPDATE … RETURNING` with `FOR UPDATE SKIP LOCKED` — short claim tx, safe across replicas, no tx held across the NATS publish), publishes each via the existing `*jetstream.Publisher`, and marks them sent. The iam consumer is already idempotent under at-least-once, so no consumer change.

**Tech Stack:** Go, pgx v5, golang-migrate (billing now wired into `migrate-all` by #130), `pkg/jetstream`, `pkg/events`, Prometheus (`promauto`, global registry served on billing's `:9099`), `pkg/testdb`, testify.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-08-billing-outbox-126-design.md`.
- Base is post-#130: `migrations/billing/001` is reconciled to the live schema and billing is in `migrate-all`/`migrate-down`/`MIGRATION_DIRS`; the outbox table is migration **`002`** and needs NO tooling wiring.
- Widening `billing.Repository` → update ALL implementers: `*Postgres` (`services/billing/db/postgres.go`), `mockRepo` (`services/billing/service_test.go`, reused by `handler_test.go`), and the e2e `billingRepo` (`e2e/critical_path/billing_test.go`). Whole-tree `go build ./... && go vet ./...` is the gate.
- Billing uses **raw parameterised SQL** (`p.db.Exec`/`QueryRow`), not sqlc — no `sqlc generate` for these.
- **Local DB safety (CLAUDE.md):** never `docker compose … -v` on `infra/local`; DB verification via a throwaway named container or `pkg/testdb` (integration tests SKIP without `THITTAM_TEST_DSN`); CI's real-Postgres job is the authoritative gate.
- Coverage: billing ≥ 80%. errcheck (deferred `tx.Rollback` → `_ =`). slog, no PII.
- Commits: Conventional Commits, scopes `billing` / `infra`. End every message with:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`

---

### Task 1: Migration `002_event_outbox`

**Files:**
- Create: `migrations/billing/002_event_outbox.up.sql`
- Create: `migrations/billing/002_event_outbox.down.sql`

**Interfaces:** Produces the `event_outbox` table (auto-applied — billing is already in `migrate-all` after #130).

- [ ] **Step 1: up migration** — `migrations/billing/002_event_outbox.up.sql`:

```sql
-- 002_event_outbox.up.sql — transactional outbox for billing domain events (#126).
-- Rows are written in the same tx as the domain change; an in-process relay in
-- cmd/billing publishes them and marks sent_at. Generic (subject column) so any
-- billing event can use it.
CREATE TABLE event_outbox (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    subject    TEXT        NOT NULL,
    tenant_id  UUID        NOT NULL,
    payload    JSONB       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at    TIMESTAMPTZ,
    attempts   INTEGER     NOT NULL DEFAULT 0,
    last_error TEXT
);

-- The relay's claim query: unsent rows, oldest first.
CREATE INDEX idx_event_outbox_unsent ON event_outbox (created_at) WHERE sent_at IS NULL;
```

- [ ] **Step 2: down migration** — `migrations/billing/002_event_outbox.down.sql`:

```sql
DROP INDEX IF EXISTS idx_event_outbox_unsent;
DROP TABLE IF EXISTS event_outbox;
```

- [ ] **Step 3: Verify** — inspection + (optional) a throwaway container (NEVER `infra/local -v`). The SQL is standalone (no FKs). If no scratch DB, note that CI migration-validate (billing is in its loop) exercises up+down. Confirm well-formed by inspection.

- [ ] **Step 4: Commit**

```bash
git add migrations/billing/002_event_outbox.up.sql migrations/billing/002_event_outbox.down.sql
git commit -m "feat(billing): event_outbox table for transactional publish (#126)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Repository — atomic outbox write + relay-support methods

**Files:**
- Modify: `services/billing/models.go` (add `OutboxEvent` struct)
- Modify: `services/billing/repository.go` (5 interface methods)
- Modify: `services/billing/db/postgres.go` (impls)
- Modify: `services/billing/service_test.go` (`mockRepo` fields + methods)
- Modify: `e2e/critical_path/billing_test.go` (`billingRepo` methods)

**Interfaces:**
- Produces `billing.OutboxEvent`.
- Produces `Repository` methods:
  - `SuspendSubscriptionWithOutbox(ctx, sub *Subscription, subject string, payload []byte) error` — one tx: suspend UPDATE + outbox INSERT.
  - `ClaimUnsentOutbox(ctx, limit int) ([]*OutboxEvent, error)` — atomically claim a batch (increments attempts), for the relay.
  - `MarkOutboxSent(ctx, id uuid.UUID) error`
  - `RecordOutboxFailure(ctx, id uuid.UUID, errMsg string) error`
  - `DeleteSentOutboxOlderThan(ctx, cutoff time.Time) (int64, error)`

- [ ] **Step 1: Domain struct** in `services/billing/models.go`:

```go
// OutboxEvent is a row of the transactional outbox (#126): a domain event
// persisted in the same tx as its domain change, awaiting relay publication.
type OutboxEvent struct {
	ID        uuid.UUID
	Subject   string
	TenantID  uuid.UUID
	Payload   []byte
	CreatedAt time.Time
	SentAt    *time.Time
	Attempts  int
	LastError *string
}
```

- [ ] **Step 2: Interface methods** in `services/billing/repository.go` (new `// Outbox (#126)` section):

```go
	// Outbox (#126)
	SuspendSubscriptionWithOutbox(ctx context.Context, sub *Subscription, subject string, payload []byte) error
	ClaimUnsentOutbox(ctx context.Context, limit int) ([]*OutboxEvent, error)
	MarkOutboxSent(ctx context.Context, id uuid.UUID) error
	RecordOutboxFailure(ctx context.Context, id uuid.UUID, errMsg string) error
	DeleteSentOutboxOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
```

- [ ] **Step 3: `*Postgres` impls** in `services/billing/db/postgres.go`. `SuspendSubscriptionWithOutbox` uses the ledger tx pattern; the suspend UPDATE mirrors the existing `UpdateSubscription` column set exactly (so all columns stay consistent):

```go
func (p *Postgres) SuspendSubscriptionWithOutbox(ctx context.Context, s *billing.Subscription, subject string, payload []byte) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("billing: begin suspend+outbox tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Same column set as UpdateSubscription — keep in sync.
	if _, err := tx.Exec(ctx, `
		UPDATE subscriptions
		SET plan=$2, status=$3, billing_cycle=$4, current_period_start=$5, current_period_end=$6,
		    trial_ends_at=$7, cancelled_at=$8, suspended_at=$9, razorpay_sub_id=$10, stripe_sub_id=$11,
		    updated_at=$12
		WHERE tenant_id = $1`,
		s.TenantID, s.Plan, s.Status, s.BillingCycle, s.CurrentPeriodStart, s.CurrentPeriodEnd,
		s.TrialEndsAt, s.CancelledAt, s.SuspendedAt, s.RazorpaySubID, s.StripeSubID, s.UpdatedAt,
	); err != nil {
		return fmt.Errorf("billing: suspend subscription (tx): %w", err)
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO event_outbox (subject, tenant_id, payload) VALUES ($1, $2, $3)`,
		subject, s.TenantID, payload,
	); err != nil {
		return fmt.Errorf("billing: insert outbox: %w", err)
	}

	return tx.Commit(ctx)
}

// ClaimUnsentOutbox atomically claims a batch of unsent events for the relay:
// SKIP LOCKED avoids double-claim across replicas; attempts++ counts the try.
// The claim is a single short statement — no tx is held across the NATS publish.
func (p *Postgres) ClaimUnsentOutbox(ctx context.Context, limit int) ([]*billing.OutboxEvent, error) {
	rows, err := p.db.Query(ctx, `
		UPDATE event_outbox SET attempts = attempts + 1
		WHERE id IN (
			SELECT id FROM event_outbox
			WHERE sent_at IS NULL
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		RETURNING id, subject, tenant_id, payload, created_at, sent_at, attempts, last_error`, limit)
	if err != nil {
		return nil, fmt.Errorf("billing: claim outbox: %w", err)
	}
	defer rows.Close()

	var out []*billing.OutboxEvent
	for rows.Next() {
		var e billing.OutboxEvent
		var sentAt pgtype.Timestamptz
		var lastErr pgtype.Text
		if err := rows.Scan(&e.ID, &e.Subject, &e.TenantID, &e.Payload, &e.CreatedAt, &sentAt, &e.Attempts, &lastErr); err != nil {
			return nil, fmt.Errorf("billing: scan outbox: %w", err)
		}
		if sentAt.Valid {
			e.SentAt = &sentAt.Time
		}
		if lastErr.Valid {
			e.LastError = &lastErr.String
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}

func (p *Postgres) MarkOutboxSent(ctx context.Context, id uuid.UUID) error {
	if _, err := p.db.Exec(ctx, `UPDATE event_outbox SET sent_at = now() WHERE id = $1`, id); err != nil {
		return fmt.Errorf("billing: mark outbox sent: %w", err)
	}
	return nil
}

func (p *Postgres) RecordOutboxFailure(ctx context.Context, id uuid.UUID, errMsg string) error {
	if _, err := p.db.Exec(ctx, `UPDATE event_outbox SET last_error = $2 WHERE id = $1`, id, errMsg); err != nil {
		return fmt.Errorf("billing: record outbox failure: %w", err)
	}
	return nil
}

func (p *Postgres) DeleteSentOutboxOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	ct, err := p.db.Exec(ctx, `DELETE FROM event_outbox WHERE sent_at IS NOT NULL AND sent_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("billing: delete sent outbox: %w", err)
	}
	return ct.RowsAffected(), nil
}
```

(`pgtype` is already imported in `postgres.go`.)

- [ ] **Step 4: Update the test doubles**

`mockRepo` (`services/billing/service_test.go`): add five `…Fn` fields + methods (delegating to the hook or a benign default), mirroring the existing field-pointer style. The relay-support ones can default to `nil, nil` / `nil`; `suspendSubscriptionWithOutboxFn` defaults to `nil`.

e2e `billingRepo` (`e2e/critical_path/billing_test.go`): add the five methods. `SuspendSubscriptionWithOutbox` should update its in-memory subscription map (mirror the existing `UpdateSubscription` double) and append to an in-memory outbox slice; the relay-support methods can be minimal (the e2e path doesn't run the relay). Enough to satisfy the interface.

- [ ] **Step 5: Build + vet whole tree**

Run: `go build ./... && go vet ./...`
Expected: clean (all three implementers satisfy the widened interface).

- [ ] **Step 6: Commit**

```bash
git add services/billing/models.go services/billing/repository.go services/billing/db/postgres.go \
        services/billing/service_test.go e2e/critical_path/billing_test.go
git commit -m "feat(billing): outbox repo — atomic suspend+outbox write + relay claim/mark (#126)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Service — atomic suspend, drop the best-effort publisher

**Files:**
- Modify: `services/billing/service.go` (`SuspendSubscription` rewrite; remove publisher field + `WithPublisher`)
- Delete/empty: `services/billing/events.go` (remove `EventPublisher`)
- Modify: `services/billing/service_test.go` (replace the 4 publisher tests with outbox assertions; drop `fakeEventPublisher`)

**Interfaces:**
- Consumes: `Repository.SuspendSubscriptionWithOutbox` (Task 2); `pkg/events` (`SubjectBillingSubscriptionSuspended`, `BillingSubscriptionSuspendedPayload`).
- Produces: a publisher-free `Service` whose `SuspendSubscription` writes the outbox in-tx.

- [ ] **Step 1: Write the failing service tests** in `services/billing/service_test.go`. Remove `fakeEventPublisher` (Task drops the interface) and replace `TestSuspendSubscription_PublishesEvent`/`_NilPublisherIsNoOp`/`_PublishErrorDoesNotFailOp` with outbox-oriented tests. Keep `TestSuspendSubscription_SetsSuspendedAt` but point it at the new repo method. Capture the outbox args via a `mockRepo` hook:

```go
func TestSuspendSubscription_WritesOutboxAtomically(t *testing.T) {
	t.Parallel()
	var gotSubject string
	var gotPayload []byte
	var gotStatus string
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return starterSub(), nil
		},
		suspendSubscriptionWithOutboxFn: func(_ context.Context, s *Subscription, subject string, payload []byte) error {
			gotSubject, gotPayload, gotStatus = subject, payload, s.Status
			return nil
		},
	})

	sub, err := svc.SuspendSubscription(context.Background(), fixedTenantID)
	require.NoError(t, err)
	assert.Equal(t, "suspended", sub.Status)
	require.NotNil(t, sub.SuspendedAt)
	assert.Equal(t, "suspended", gotStatus, "outbox written with the suspended subscription")
	assert.Equal(t, events.SubjectBillingSubscriptionSuspended, gotSubject)

	var p events.BillingSubscriptionSuspendedPayload
	require.NoError(t, json.Unmarshal(gotPayload, &p), "payload is a valid BillingSubscriptionSuspendedPayload")
	assert.Equal(t, sub.ID.String(), p.SubscriptionID)
	assert.NotEmpty(t, p.SuspendedAt)
	assert.NotEmpty(t, p.PurgeAfter)
}

func TestSuspendSubscription_PropagatesOutboxError(t *testing.T) {
	t.Parallel()
	svc := NewService(&mockRepo{
		getSubscriptionByTenantFn: func(_ context.Context, _ uuid.UUID) (*Subscription, error) {
			return starterSub(), nil
		},
		suspendSubscriptionWithOutboxFn: func(_ context.Context, _ *Subscription, _ string, _ []byte) error {
			return fmt.Errorf("tx failed")
		},
	})
	_, err := svc.SuspendSubscription(context.Background(), fixedTenantID)
	require.Error(t, err, "a failed atomic write must fail the op (unlike the old best-effort publish)")
}
```

(Add `encoding/json` and the `pkg/events` import to the test file.)

- [ ] **Step 2: Run to verify fail**

Run: `go test ./services/billing/ -run TestSuspendSubscription -v`
Expected: FAIL — `suspendSubscriptionWithOutboxFn`/method not yet wired into the service; `fakeEventPublisher` removal breaks the old tests.

- [ ] **Step 3: Rewrite `SuspendSubscription`** in `services/billing/service.go`:

```go
// SuspendSubscription moves a subscription to suspended and records the
// subscription.suspended event in the SAME transaction (transactional outbox,
// #126) — the relay publishes it. Atomic: a failed write fails the op.
func (s *Service) SuspendSubscription(ctx context.Context, tenantID uuid.UUID) (*Subscription, error) {
	sub, err := s.repo.GetSubscriptionByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}

	now := time.Now().UTC()
	sub.Status = "suspended"
	sub.SuspendedAt = &now
	sub.UpdatedAt = now

	payload, err := json.Marshal(events.BillingSubscriptionSuspendedPayload{
		SubscriptionID: sub.ID.String(),
		SuspendedAt:    now.Format(time.RFC3339),
		PurgeAfter:     now.AddDate(0, 0, 30).Format(time.RFC3339),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal suspend payload: %w", err)
	}

	if err := s.repo.SuspendSubscriptionWithOutbox(ctx, sub, events.SubjectBillingSubscriptionSuspended, payload); err != nil {
		return nil, fmt.Errorf("suspend subscription: %w", err)
	}
	return sub, nil
}
```

Update `service.go` imports: add `encoding/json` and `github.com/wegofwd2020/thittam/pkg/events`; remove `log/slog` if now unused (check — other methods may still use it; only remove if no remaining reference). Remove the `publisher EventPublisher` field from the `Service` struct and the `WithPublisher` method.

- [ ] **Step 4: Remove the `EventPublisher` interface**

`services/billing/events.go` is now empty of purpose — delete the file (`git rm services/billing/events.go`). (Confirm no other reference to `billing.EventPublisher` remains: `grep -rn EventPublisher services/billing cmd/billing` — the `cmd/billing` adapter is removed in Task 5; if Task 5 hasn't run yet, `cmd/billing` still references it and won't compile until Task 5. To keep the tree green after THIS task, leave the `cmd/billing` wiring for Task 5 but note the build of `./cmd/billing` will be temporarily broken between Task 3 and Task 5 — run `go build ./services/billing/...` for this task's gate, and the whole-tree build is restored at Task 5.)

> **Sequencing note for the controller:** Tasks 3 and 5 are coupled — removing `EventPublisher` (Task 3) breaks `cmd/billing/main.go`'s adapter until Task 5 rewires it. Options: (a) run Task 3's gate as `go build ./services/billing/... && go test ./services/billing/ -run TestSuspendSubscription`, accepting `./cmd/billing` is red until Task 5; or (b) fold Task 5 into Task 3. This plan keeps them separate for reviewability but the whole-tree `go build ./...` gate only holds again after Task 5. Do NOT mark the branch done until Task 5 restores it.

- [ ] **Step 5: Run to verify pass**

Run: `go test ./services/billing/ -run TestSuspendSubscription -v && go build ./services/billing/...`
Expected: PASS; `services/billing` builds. (`./cmd/billing` intentionally not built here — see the note.)

- [ ] **Step 6: Commit**

```bash
git add services/billing/service.go services/billing/service_test.go
git rm services/billing/events.go
git commit -m "feat(billing): SuspendSubscription writes outbox in-tx; drop best-effort publisher (#126)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Relay component + metrics

**Files:**
- Create: `services/billing/outbox_relay.go`
- Create: `services/billing/outbox_relay_test.go`

**Interfaces:**
- Consumes: `Repository.ClaimUnsentOutbox`/`MarkOutboxSent`/`RecordOutboxFailure`/`DeleteSentOutboxOlderThan` (Task 2).
- Produces: `Relay` with `Run(ctx)` (blocks until ctx done), driven by a ticker; publishes via a narrow `outboxPublisher` interface.

- [ ] **Step 1: Write the failing relay tests** in `services/billing/outbox_relay_test.go` (package `billing`). Use a fake repo hook set (the `mockRepo`) + a fake publisher:

```go
package billing

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeOutboxPublisher struct {
	published []uuid.UUID
	err       error
}

func (f *fakeOutboxPublisher) Publish(_ context.Context, _ string, tenantID uuid.UUID, _ interface{}) error {
	f.published = append(f.published, tenantID)
	return f.err
}

func TestRelay_PublishesAndMarksSent(t *testing.T) {
	t.Parallel()
	ev := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`)}
	var marked uuid.UUID
	repo := &mockRepo{
		claimUnsentOutboxFn: func(_ context.Context, _ int) ([]*OutboxEvent, error) { return []*OutboxEvent{ev}, nil },
		markOutboxSentFn:    func(_ context.Context, id uuid.UUID) error { marked = id; return nil },
	}
	pub := &fakeOutboxPublisher{}
	r := NewRelay(repo, pub)

	n, err := r.processBatch(context.Background(), 10)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	require.Len(t, pub.published, 1)
	assert.Equal(t, ev.ID, marked)
}

func TestRelay_PublishError_RecordsFailureLeavesUnsent(t *testing.T) {
	t.Parallel()
	ev := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`)}
	var failedID uuid.UUID
	var failedMsg string
	markedSent := false
	repo := &mockRepo{
		claimUnsentOutboxFn:   func(_ context.Context, _ int) ([]*OutboxEvent, error) { return []*OutboxEvent{ev}, nil },
		markOutboxSentFn:      func(_ context.Context, _ uuid.UUID) error { markedSent = true; return nil },
		recordOutboxFailureFn: func(_ context.Context, id uuid.UUID, msg string) error { failedID, failedMsg = id, msg; return nil },
	}
	pub := &fakeOutboxPublisher{err: fmt.Errorf("nats down")}
	r := NewRelay(repo, pub)

	_, err := r.processBatch(context.Background(), 10)
	require.NoError(t, err, "a single publish failure is recorded, not fatal to the batch")
	assert.False(t, markedSent, "must NOT mark sent on publish failure")
	assert.Equal(t, ev.ID, failedID)
	assert.Contains(t, failedMsg, "nats down")
}
```

- [ ] **Step 2: Run to verify fail**

Run: `go test ./services/billing/ -run TestRelay -v`
Expected: FAIL — `NewRelay`/`Relay.processBatch` undefined.

- [ ] **Step 3: Implement the relay** in `services/billing/outbox_relay.go`:

```go
package billing

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// outboxPublisher is the narrow publish port the relay needs (satisfied by
// *jetstream.Publisher). Kept local to avoid a pkg/jetstream import cycle.
type outboxPublisher interface {
	Publish(ctx context.Context, subject string, tenantID uuid.UUID, payload interface{}) error
}

// outboxRepo is the subset of Repository the relay uses.
type outboxRepo interface {
	ClaimUnsentOutbox(ctx context.Context, limit int) ([]*OutboxEvent, error)
	MarkOutboxSent(ctx context.Context, id uuid.UUID) error
	RecordOutboxFailure(ctx context.Context, id uuid.UUID, errMsg string) error
	DeleteSentOutboxOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

const (
	relayInterval    = 15 * time.Second
	relayBatchSize   = 100
	relayCleanupEach = 240 // every 240th tick (~1h at 15s) delete old sent rows
	relaySentTTL     = 7 * 24 * time.Hour
)

var (
	outboxPublished = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "thittam", Subsystem: "billing", Name: "outbox_published_total",
		Help: "Outbox events successfully published by the relay.",
	})
	outboxFailed = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "thittam", Subsystem: "billing", Name: "outbox_failed_total",
		Help: "Outbox publish attempts that failed (retried next tick).",
	})
)

// Relay drains the event_outbox to NATS. Run it as a goroutine in cmd/billing.
type Relay struct {
	repo outboxRepo
	pub  outboxPublisher
	log  *slog.Logger
}

func NewRelay(repo outboxRepo, pub outboxPublisher) *Relay {
	return &Relay{repo: repo, pub: pub, log: slog.Default().With("component", "outbox-relay")}
}

// Run ticks until ctx is cancelled.
func (r *Relay) Run(ctx context.Context) {
	ticker := time.NewTicker(relayInterval)
	defer ticker.Stop()
	var ticks int
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := r.processBatch(ctx, relayBatchSize); err != nil {
				r.log.Error("outbox batch failed", "error", err)
			}
			if ticks++; ticks%relayCleanupEach == 0 {
				if n, err := r.repo.DeleteSentOutboxOlderThan(ctx, time.Now().UTC().Add(-relaySentTTL)); err != nil {
					r.log.Warn("outbox cleanup failed", "error", err)
				} else if n > 0 {
					r.log.Info("outbox cleanup", "deleted", n)
				}
			}
		}
	}
}

// processBatch claims up to limit unsent events, publishes each, and marks
// sent (or records failure, leaving the row for the next tick). Returns the
// count published. A systemic claim error is returned; per-event publish
// failures are recorded and counted, not returned.
func (r *Relay) processBatch(ctx context.Context, limit int) (int, error) {
	events, err := r.repo.ClaimUnsentOutbox(ctx, limit)
	if err != nil {
		return 0, err
	}
	published := 0
	for _, e := range events {
		if perr := r.pub.Publish(ctx, e.Subject, e.TenantID, json.RawMessage(e.Payload)); perr != nil {
			outboxFailed.Inc()
			if rerr := r.repo.RecordOutboxFailure(ctx, e.ID, perr.Error()); rerr != nil {
				r.log.Error("record outbox failure", "id", e.ID, "error", rerr)
			}
			continue
		}
		if merr := r.repo.MarkOutboxSent(ctx, e.ID); merr != nil {
			r.log.Error("mark outbox sent", "id", e.ID, "error", merr)
			continue
		}
		outboxPublished.Inc()
		published++
	}
	return published, nil
}
```

Note: the relay publishes `json.RawMessage(e.Payload)` via `pub.Publish` (which wraps it in a fresh envelope). The envelope `EventID` differs per attempt — fine, the iam consumer dedups on domain state, not `EventID` (per spec §4).

- [ ] **Step 4: Add the `mockRepo` relay hooks** (if not already added in Task 2): ensure `mockRepo` has `claimUnsentOutboxFn`, `markOutboxSentFn`, `recordOutboxFailureFn`, `deleteSentOutboxOlderThanFn` fields + methods (Task 2 Step 4 adds them). Run the tests.

- [ ] **Step 5: Run to verify pass**

Run: `go test ./services/billing/ -run 'TestRelay|TestSuspendSubscription' -v && go build ./services/billing/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add services/billing/outbox_relay.go services/billing/outbox_relay_test.go
git commit -m "feat(billing): outbox relay — claim/publish/mark + cleanup + metrics (#126)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: cmd/billing wiring — start relay, drop the adapter

**Files:**
- Modify: `cmd/billing/main.go`

**Interfaces:**
- Consumes: `billing.NewRelay` (Task 4); the existing `pool` + `*jetstream.Publisher`.
- Produces: a running relay goroutine; removes the `billingPublisher` adapter + `svc.WithPublisher`.

- [ ] **Step 1: Rewire `cmd/billing/main.go`**

- Remove the `billingPublisher` adapter type + its `PublishSubscriptionSuspended` method.
- Remove the `svc = svc.WithPublisher(&billingPublisher{...})` line.
- After building `repo`, `svc`, and (if configured) `pub`, and BEFORE `srv.Run()`, start the relay only when NATS is configured (mirror the existing `pub != nil` gate). Use a cancellable context tied to process lifetime:

```go
	// --- Outbox relay (#126) ---
	// Drains event_outbox → NATS in-process. Only runs when NATS is configured
	// (same gate as the publisher); reuses the pool + publisher.
	if pub != nil {
		relayCtx, cancelRelay := context.WithCancel(ctx)
		defer cancelRelay()
		relay := billing.NewRelay(repo, pub)
		go relay.Run(relayCtx)
		log.Printf("billing: outbox relay started")
	}
```

(`repo` is `*billingdb.Postgres` which satisfies `billing.NewRelay`'s `outboxRepo`; `pub` is `*jetstream.Publisher` which satisfies `outboxPublisher`. If `NewRelay`'s param types are the exported interfaces, this compiles directly; if not, adjust `NewRelay` to accept the interfaces — it does per Task 4.)

- Remove now-unused imports (`events` may still be unused in cmd — check; the adapter used `events` + `time`; if the relay wiring doesn't, drop them). Run goimports/`go build` to confirm.

- [ ] **Step 2: Whole-tree build + vet + full billing suite**

Run: `go build ./... && go vet ./... && go test ./services/billing/... -short`
Expected: clean — `./cmd/billing` compiles again (this restores the whole-tree green that Task 3 temporarily broke); billing unit tests pass; coverage ≥ 80% (`go test ./services/billing/ -short -coverprofile=cover.out && go tool cover -func=cover.out | tail -1`).

- [ ] **Step 3: Commit**

```bash
git add cmd/billing/main.go
git commit -m "feat(billing): run outbox relay in cmd/billing; remove best-effort adapter (#126)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Integration test — atomicity + relay claim

**Files:**
- Create: `services/billing/db/outbox_integration_test.go`

**Interfaces:** Consumes the real `*Postgres` repo + a migrated `thittam_test` (billing now in migrate-all).

- [ ] **Step 1: Write the integration test** (`//go:build integration`, `pkg/testdb`, mirroring the #130 `subscription_integration_test.go` seeding style — seed a `tenants` FK parent, cleanup via cascade). Cover: (a) `SuspendSubscriptionWithOutbox` writes BOTH the suspend and an outbox row atomically; (b) `ClaimUnsentOutbox` returns it (and increments attempts); (c) `MarkOutboxSent` makes it no longer claimable. **Do NOT use `infra/local -v`** — `testdb.Open` skips without `THITTAM_TEST_DSN`.

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

	"github.com/wegofwd2020/thittam/pkg/testdb"
	"github.com/wegofwd2020/thittam/services/billing"
	billingdb "github.com/wegofwd2020/thittam/services/billing/db"
)

func TestOutbox_SuspendWritesAtomically_ThenClaimAndMark(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := billingdb.NewPostgres(pool)

	tenantID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code, status)
		 VALUES ($1, $2, $3, 'US', 'USD', 'active')`,
		tenantID, "Outbox IT "+tenantID.String()[:8], "obx-"+tenantID.String()[:8])
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM tenants WHERE id = $1`, tenantID) })

	now := time.Now().UTC()
	sub := &billing.Subscription{
		ID: uuid.New(), TenantID: tenantID, Plan: "starter", Status: "active", BillingCycle: "monthly",
		CurrentPeriodStart: now, CurrentPeriodEnd: now.AddDate(0, 1, 0), CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.CreateSubscription(ctx, sub))

	// Atomic suspend + outbox.
	sus := now
	sub.Status = "suspended"
	sub.SuspendedAt = &sus
	sub.UpdatedAt = now
	require.NoError(t, repo.SuspendSubscriptionWithOutbox(ctx, sub,
		"thittam.billing.subscription.suspended", []byte(`{"subscription_id":"x"}`)))

	// Suspend persisted...
	got, err := repo.GetSubscriptionByTenant(ctx, tenantID)
	require.NoError(t, err)
	assert.Equal(t, "suspended", got.Status)

	// ...and exactly one claimable outbox row for this tenant.
	claimed, err := repo.ClaimUnsentOutbox(ctx, 100)
	require.NoError(t, err)
	var mine *billing.OutboxEvent
	for _, e := range claimed {
		if e.TenantID == tenantID {
			mine = e
		}
	}
	require.NotNil(t, mine, "the suspend wrote a claimable outbox row")
	assert.Equal(t, 1, mine.Attempts, "claim increments attempts")

	require.NoError(t, repo.MarkOutboxSent(ctx, mine.ID))

	// After marking sent, it is no longer claimable.
	again, err := repo.ClaimUnsentOutbox(ctx, 100)
	require.NoError(t, err)
	for _, e := range again {
		assert.NotEqual(t, mine.ID, e.ID, "sent row must not be re-claimed")
	}
}
```

- [ ] **Step 2: Verify (compile only — no DB/Docker)**

Run: `go test ./services/billing/db/ -tags=integration -run TestOutbox -count=0` (compiles) and `go build ./... && go vet -tags=integration ./services/billing/db/`.
Expected: compiles/clean. SKIPs without `THITTAM_TEST_DSN`; CI's integration-tests job runs it for real.

- [ ] **Step 3: Commit**

```bash
git add services/billing/db/outbox_integration_test.go
git commit -m "test(billing): integration — atomic suspend+outbox write, claim, mark sent (#126)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:**
- Migration 002 event_outbox (+ auto-wired via #130) → Task 1. ✓
- Atomic suspend+outbox write (one tx) → Task 2 (`SuspendSubscriptionWithOutbox`) + Task 3 (service uses it). ✓
- In-process relay, ticker, claim/publish/mark, `FOR UPDATE SKIP LOCKED` (short claim tx, not held across publish) → Task 2 (`ClaimUnsentOutbox`) + Task 4 (`Relay`). ✓
- Remove best-effort publisher / `EventPublisher` / adapter → Task 3 + Task 5. ✓
- Cleanup of old sent rows → Task 4 (`DeleteSentOutboxOlderThan` + periodic tick). ✓
- Metrics via promauto/global registry (billing :9099) → Task 4. ✓
- At-least-once tolerated by the (unchanged) consumer → no consumer task; relay comment notes EventID-per-attempt is fine. ✓
- Testing: service unit (atomic write + error propagation), relay unit (publish/mark, failure-records), integration (atomicity + claim/mark) → Tasks 3/4/6. ✓
- Widen Repository across all three implementers + whole-tree vet → Task 2. ✓

**Placeholder scan:** none. The one real hazard is called out explicitly: **Tasks 3↔5 coupling** (removing `EventPublisher` breaks `cmd/billing` until Task 5) — flagged with a controller note and per-task gates scoped to `./services/billing/...`, with whole-tree green restored at Task 5. The controller MUST NOT finish the branch before Task 5.

**Type consistency:** `SuspendSubscriptionWithOutbox(ctx, *Subscription, string, []byte)` and the four relay-support methods are used identically across interface, `*Postgres`, `mockRepo`, `billingRepo`, service, and relay. `OutboxEvent` fields match the migration columns. `NewRelay(outboxRepo, outboxPublisher)` — `*billingdb.Postgres` satisfies `outboxRepo`, `*jetstream.Publisher` satisfies `outboxPublisher` (signature `Publish(ctx, string, uuid.UUID, interface{}) error` matches exactly).
