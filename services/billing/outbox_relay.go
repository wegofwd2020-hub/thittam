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
	MoveOutboxToDead(ctx context.Context, id uuid.UUID, errMsg string) error
	OutboxStats(ctx context.Context) (*OutboxStats, error)
}

const (
	relayInterval    = 15 * time.Second
	relayBatchSize   = 100
	relayCleanupEach = 240 // every 240th tick (~1h at 15s) delete old sent rows
	relaySentTTL     = 7 * 24 * time.Hour

	maxOutboxAttempts = 5 // dead-letter after this many failed publishes — but only
	// when a batch-mate succeeded; see processBatch.
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
			r.updateGauges(ctx)
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
