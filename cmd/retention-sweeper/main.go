// retention-sweeper is the batch CLI entry point for the tenant retention
// state machine introduced in #92. It runs as a Kubernetes CronJob (daily)
// and advances tenants through the lifecycle defined in
// thittam_docs/docs/operations/tenant-onboarding.md §1.3a:
//
//	suspended    → grace           after 30 days
//	grace        → deactivated     after 90 days (measured from suspended_at)
//	deactivated  → purge_eligible  after 180 days
//
// The sweeper never hard-deletes data. Tenants flagged purge_eligible are
// surfaced to the platform admin UI; actual schema drop requires the
// (forthcoming, #92 Stage 3) PurgeTenant RPC under two-person approval.
//
// Usage:
//
//	retention-sweeper -batch 500 -max-batches 10
//
// Exit codes:
//   0 — sweep complete (including a clean empty pass)
//   1 — configuration error (missing DATABASE_URL, bad flags)
//   2 — unrecoverable runtime error (DB down, repeated transition failure)
//
// Metrics are emitted as structured slog lines so ops can build queries
// against Loki without a Pushgateway (tracked for follow-up).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wegofwd2020/thittam/services/iam"
	iamdb "github.com/wegofwd2020/thittam/services/iam/db"
)

func main() {
	batchSize := flag.Int("batch", 500, "Max tenants to advance per iteration")
	maxBatches := flag.Int("max-batches", 100, "Safety cap on iteration count")
	flag.Parse()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "ERROR: DATABASE_URL is required")
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger = logger.With("binary", "retention-sweeper")

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
	svc := iam.NewService(repo, nil, nil, nil, nil)

	if err := runSweep(ctx, svc, logger, *batchSize, *maxBatches); err != nil {
		logger.Error("sweep failed", "error", err)
		os.Exit(2)
	}
}

// runSweep pages through tenants due for lifecycle transition and advances
// each one. Summary metrics are logged at the end of the pass.
func runSweep(
	ctx context.Context,
	svc *iam.Service,
	logger *slog.Logger,
	batchSize int,
	maxBatches int,
) error {
	now := time.Now().UTC()
	start := time.Now()

	transitions := map[string]int{} // from→to counter
	var seen, failed int
	var oldestPending time.Duration

	for batch := 0; batch < maxBatches; batch++ {
		due, err := svc.ListTenantsDueForLifecycle(ctx, now, batchSize)
		if err != nil {
			return fmt.Errorf("list due tenants: %w", err)
		}
		if len(due) == 0 {
			break
		}

		for _, t := range due {
			seen++
			if age := pendingAge(t, now); age > oldestPending {
				oldestPending = age
			}

			trans, err := svc.AdvanceTenantLifecycle(ctx, t.ID, now)
			if err != nil {
				failed++
				logger.Error("advance lifecycle",
					"tenant_id", t.ID,
					"status", t.Status,
					"error", err,
				)
				continue
			}
			if trans == nil {
				// Raced or clock not yet due — benign, just means the
				// list query and AdvanceTenantLifecycle saw slightly
				// different snapshots.
				continue
			}
			key := trans.FromStatus + "→" + trans.ToStatus
			transitions[key]++
			logger.Info("tenant advanced",
				"tenant_id", trans.TenantID,
				"tenant_name", trans.TenantName,
				"from", trans.FromStatus,
				"to", trans.ToStatus,
			)
		}

		// The batch may have had tenants that all lost their race. Break
		// when list returned fewer than batchSize to avoid a tight spin.
		if len(due) < batchSize {
			break
		}
	}

	// Summary metrics — structured so Loki/Grafana can extract them.
	attrs := []any{
		"retention_sweep_seen", seen,
		"retention_sweep_failed", failed,
		"retention_sweep_duration_ms", time.Since(start).Milliseconds(),
		"retention_oldest_pending_seconds", int(oldestPending.Seconds()),
	}
	for key, n := range transitions {
		attrs = append(attrs, "retention_transitions_"+key, n)
	}
	logger.Info("retention sweep complete", attrs...)

	if failed > 0 {
		return errors.New("one or more tenants failed to advance (see earlier logs)")
	}
	return nil
}

// pendingAge reports how overdue the tenant is for its next transition.
// Used to surface the oldest_pending metric — a cheap proxy for "is the
// sweeper keeping up with its backlog?"
func pendingAge(t *iam.Tenant, now time.Time) time.Duration {
	switch t.Status {
	case iam.TenantStatusSuspended:
		if t.SuspendedAt == nil {
			return 0
		}
		return now.Sub(*t.SuspendedAt) - iam.SuspensionGracePeriod
	case iam.TenantStatusGrace:
		if t.SuspendedAt == nil {
			return 0
		}
		return now.Sub(*t.SuspendedAt) - iam.GraceToDeactivatedDuration
	case iam.TenantStatusDeactivated:
		if t.DeactivatedAt == nil {
			return 0
		}
		return now.Sub(*t.DeactivatedAt) - iam.DeactivatedToPurgeDuration
	}
	return 0
}
