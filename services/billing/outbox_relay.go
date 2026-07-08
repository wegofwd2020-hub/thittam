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
