// retention-sweeper is the batch CLI entry point for the tenant retention
// state machine introduced in #92. It runs as a Kubernetes CronJob (daily)
// and advances tenants through the lifecycle defined in
// thittam_docs/docs/developers/operations/tenant-onboarding.md §1.3a:
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
// Metrics are emitted in two ways (#92 Stage 5):
//   - Structured slog lines, for Loki-based queries and human-readable
//     run summaries.
//   - Prometheus metrics pushed to Pushgateway, for Grafana dashboards
//     and alerting. Push target is controlled by the PUSHGATEWAY_URL
//     environment variable; unset means "skip push" (local-dev runs
//     don't need a Pushgateway deployed).
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

	"github.com/wegofwd2020/thittam/pkg/audit"
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

	auditLogger := audit.NewLogger(audit.NewPostgres(pool), audit.DefaultConfig(), nil)
	svc := iam.NewService(repo, nil, nil, nil, nil).WithAuditLogger(auditLogger)

	metrics := newSweeperMetrics()
	pushURL := os.Getenv("PUSHGATEWAY_URL")
	instance := pushInstance()

	err = runSweep(ctx, svc, logger, metrics, *batchSize, *maxBatches)

	// Flush buffered audit events before we report/push. The Logger is async
	// (5s flush); Close drains it. Use a fresh bounded context — the sweep ctx
	// may be near its 30-min deadline.
	flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if cerr := auditLogger.Close(flushCtx); cerr != nil {
		logger.Warn("audit flush failed", "error", cerr)
	}
	flushCancel()

	// Push metrics unconditionally — even on failure, so the error
	// counter actually surfaces in Prometheus. A silent error-exit
	// with no push is invisible on the dashboard.
	pushMetrics(ctx, logger, pushURL, instance, metrics.registry)

	if err != nil {
		logger.Error("sweep failed", "error", err)
		os.Exit(2)
	}
}

// runSweep pages through tenants due for lifecycle transition and advances
// each one. Summary metrics are logged at the end of the pass and also
// materialized on the Prometheus registry attached to `m` for push.
func runSweep(
	ctx context.Context,
	svc *iam.Service,
	logger *slog.Logger,
	m *sweeperMetrics,
	batchSize int,
	maxBatches int,
) error {
	now := time.Now().UTC()
	start := time.Now()

	transitions := map[string]int{} // from→to counter (for slog summary)
	var seen, failed int
	var oldestPending time.Duration

	// Query the legal-hold gauge once at the start of the run. Cheap
	// (indexed predicate on a tiny row set) and gives the dashboard a
	// "tenants currently on hold" data point even on runs that had
	// no other work to do.
	if onHold, err := svc.CountTenantsOnHold(ctx); err != nil {
		// Log and continue — the hold gauge is observability data, not
		// a correctness dependency. A stale or missing value is better
		// than refusing to run the sweep.
		logger.Warn("count tenants on hold failed — gauge will stay 0", "error", err)
	} else {
		m.tenantsOnHold.Set(float64(onHold))
	}

	for batch := 0; batch < maxBatches; batch++ {
		due, err := svc.ListTenantsDueForLifecycle(ctx, now, batchSize)
		if err != nil {
			m.runsTotal.WithLabelValues("error").Inc()
			m.durationSeconds.Set(time.Since(start).Seconds())
			return fmt.Errorf("list due tenants: %w", err)
		}
		if len(due) == 0 {
			break
		}

		for _, t := range due {
			seen++
			m.tenantsSeenTotal.Inc()
			if age := pendingAge(t, now); age > oldestPending {
				oldestPending = age
			}

			trans, err := svc.AdvanceTenantLifecycle(ctx, t.ID, now)
			if err != nil {
				failed++
				// Label failures by the status the tenant was in when the
				// sweeper tried to advance it, and the status it would
				// have moved to. Lets alerts target a specific transition.
				if nextStatus, ok := nextStatusFor(t.Status); ok {
					m.failuresTotal.WithLabelValues(t.Status, nextStatus).Inc()
				} else {
					m.failuresTotal.WithLabelValues(t.Status, "unknown").Inc()
				}
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
			m.transitionsTotal.WithLabelValues(trans.FromStatus, trans.ToStatus).Inc()
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

	duration := time.Since(start)
	m.durationSeconds.Set(duration.Seconds())
	m.oldestPendingSeconds.Set(oldestPending.Seconds())

	// Summary metrics — structured so Loki/Grafana can extract them.
	attrs := []any{
		"retention_sweep_seen", seen,
		"retention_sweep_failed", failed,
		"retention_sweep_duration_ms", duration.Milliseconds(),
		"retention_oldest_pending_seconds", int(oldestPending.Seconds()),
	}
	for key, n := range transitions {
		attrs = append(attrs, "retention_transitions_"+key, n)
	}
	logger.Info("retention sweep complete", attrs...)

	if failed > 0 {
		m.runsTotal.WithLabelValues("error").Inc()
		return errors.New("one or more tenants failed to advance (see earlier logs)")
	}
	m.runsTotal.WithLabelValues("success").Inc()
	m.lastSuccessTimestamp.SetToCurrentTime()
	return nil
}

// nextStatusFor maps a tenant's current lifecycle status to the status
// it would transition to. Used only for labeling failures — so alerts
// can say "grace→deactivated transitions are flaky". Returns ("", false)
// for terminal or unexpected states.
func nextStatusFor(from string) (string, bool) {
	switch from {
	case iam.TenantStatusSuspended:
		return iam.TenantStatusGrace, true
	case iam.TenantStatusGrace:
		return iam.TenantStatusDeactivated, true
	case iam.TenantStatusDeactivated:
		return iam.TenantStatusPurgeEligible, true
	}
	return "", false
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
