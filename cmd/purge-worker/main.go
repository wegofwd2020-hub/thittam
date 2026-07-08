// purge-worker is the batch CLI that executes approved tenant purges (#92
// Stage 3) — the terminal, destructive step of the retention lifecycle. It
// runs as a Kubernetes CronJob (daily) UNDER THE OWNER DSN (DROP SCHEMA needs
// owner privileges; thittam_app cannot). For each approved tenant_purge_request
// it drops tenant_<uuid> and tombstones the tenants row, in one transaction.
//
// Usage:
//
//	purge-worker -batch 100
//
// Exit codes:
//
//	0 — run complete (including a clean empty pass; per-request failures are
//	    logged and counted but do not fail the run)
//	1 — configuration error (missing DATABASE_URL)
//	2 — unrecoverable runtime error (DB down, the list query itself failing)
//
// Metrics are emitted the same way as retention-sweeper (#92 Stage 5):
// structured slog lines plus Prometheus metrics pushed to Pushgateway,
// controlled by the PUSHGATEWAY_URL environment variable.
package main

import (
	"context"
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
	batchSize := flag.Int("batch", 100, "Max approved purge requests to process per run")
	flag.Parse()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "ERROR: DATABASE_URL is required")
		os.Exit(1)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger = logger.With("binary", "purge-worker")

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

	metrics := newPurgeMetrics()
	err = runPurge(ctx, svc, logger, metrics, *batchSize)

	flushCtx, flushCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if cerr := auditLogger.Close(flushCtx); cerr != nil {
		logger.Warn("audit flush failed", "error", cerr)
	}
	flushCancel()

	// Fresh bounded context — mirrors the audit-flush pattern above. Reusing
	// the 30-minute run ctx would let a nearly-exhausted deadline skip the
	// metrics push even though the run itself completed.
	pushCtx, pushCancel := context.WithTimeout(context.Background(), 10*time.Second)
	pushMetrics(pushCtx, logger, os.Getenv("PUSHGATEWAY_URL"), pushInstance(), metrics.registry)
	pushCancel()

	if err != nil {
		logger.Error("purge run failed", "error", err)
		os.Exit(2)
	}
}

// runPurge processes up to batchSize approved requests. Individual failures are
// recorded on the request (by the service) and counted, not fatal — the run
// only fails on a systemic error (e.g. the list query itself).
func runPurge(ctx context.Context, svc *iam.Service, logger *slog.Logger, m *purgeMetrics, batchSize int) error {
	reqs, err := svc.ListApprovedPurges(ctx, batchSize)
	if err != nil {
		m.runsTotal.WithLabelValues("error").Inc()
		return fmt.Errorf("list approved purges: %w", err)
	}
	for _, req := range reqs {
		if perr := svc.PurgeApprovedTenant(ctx, req); perr != nil {
			logger.Error("purge failed", "tenant_id", req.TenantID, "error", perr)
			m.failuresTotal.Inc()
			continue
		}
		logger.Info("tenant purged", "tenant_id", req.TenantID)
		m.purgedTotal.Inc()
	}
	m.runsTotal.WithLabelValues("success").Inc()
	m.lastSuccessTimestamp.SetToCurrentTime()
	return nil
}
