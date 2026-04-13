// migrate-all-tenants is the CLI entry point for the parallel tenant migration runner.
//
// It reads all active tenant IDs from the `public.tenants` table, then runs the
// requested migration path against every tenant schema concurrently.
//
// Usage:
//
//	go run ./cmd/migrate-all-tenants \
//	    -db "postgres://thittam:pass@localhost:5433/thittam?sslmode=disable" \
//	    -path migrations/project \
//	    -parallelism 8
//
// The runner does not stop on partial failure. All tenant IDs that failed are
// printed at the end with their errors. Re-run individual tenants with:
//
//	make migrate-tenant id=<uuid>
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/wegofwd2020/thittam/pkg/migrate"
)

func main() {
	dbURL := flag.String("db", "", "PostgreSQL connection URL (required)")
	migPath := flag.String("path", "migrations/project", "Migration path to apply to every tenant schema")
	parallelism := flag.Int("parallelism", 8, "Number of concurrent migration workers (max 8 recommended)")
	flag.Parse()

	if *dbURL == "" {
		fmt.Fprintln(os.Stderr, "ERROR: -db is required")
		flag.Usage()
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx := context.Background()

	// Fetch all active tenant IDs from the shared tenants table.
	tenantIDs, err := loadTenantIDs(ctx, *dbURL)
	if err != nil {
		logger.Error("failed to load tenant IDs", "error", err)
		os.Exit(1)
	}
	logger.Info("loaded tenants", "count", len(tenantIDs), "path", *migPath, "parallelism", *parallelism)

	m := migrate.New(migrate.Options{
		DBURL:       *dbURL,
		Parallelism: *parallelism,
		OnProgress: func(r migrate.Result) {
			if r.Err != nil {
				logger.Error("tenant migration failed",
					"tenant_id", r.TenantID,
					"duration_ms", r.Duration.Milliseconds(),
					"error", r.Err,
				)
			} else {
				logger.Info("tenant migrated",
					"tenant_id", r.TenantID,
					"duration_ms", r.Duration.Milliseconds(),
				)
			}
		},
	})

	start := time.Now()
	report, err := m.MigrateAll(ctx, tenantIDs, *migPath)
	if err != nil {
		logger.Error("migration runner error", "error", err)
		os.Exit(1)
	}

	logger.Info("migration run complete",
		"total", report.Total,
		"passed", report.Passed,
		"failed", len(report.Failures),
		"duration_ms", report.Duration.Milliseconds(),
	)

	if report.Failed() {
		fmt.Fprintf(os.Stderr, "\n==> %d tenant(s) failed migration:\n", len(report.Failures))
		for _, f := range report.Failures {
			fmt.Fprintf(os.Stderr, "    make migrate-tenant id=%s  # %v\n", f.TenantID, f.Err)
		}
		logger.Info("retry failed tenants using: make migrate-tenant id=<uuid>",
			"elapsed", time.Since(start))
		os.Exit(2) // exit 2 = partial failure (distinct from hard error exit 1)
	}

	logger.Info("all tenants migrated successfully", "elapsed", time.Since(start))
}

// loadTenantIDs queries the public.tenants table for all non-suspended tenant IDs.
// It does not use the tenantdb package because this runs outside any tenant context.
func loadTenantIDs(ctx context.Context, dbURL string) ([]uuid.UUID, error) {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	rows, err := db.QueryContext(ctx,
		`SELECT id FROM public.tenants WHERE status NOT IN ('cancelled', 'suspended') ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("query tenants: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan tenant id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
