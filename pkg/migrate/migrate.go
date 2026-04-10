// Package migrate provides a parallel, fault-tolerant migration runner for
// Thittam's tenant-per-schema PostgreSQL model.
//
// # Problem
//
// Thittam isolates each tenant in a dedicated PostgreSQL schema
// (tenant_<uuid>). When a new migration is released, it must be applied to
// every tenant schema. Serially, 1000 tenants × 30 s/migration = 8+ hours.
//
// # Solution
//
// ParallelMigrator runs migrations concurrently with a configurable worker
// pool. Failed tenants are collected into a Report and do not stop the
// remaining work. The caller inspects Report.Failures and retries or alerts.
//
// # Usage
//
//	m := migrate.New(migrate.Options{
//	    DBURL:       "postgres://thittam:pass@localhost:5432/thittam?sslmode=disable",
//	    Parallelism: 8,
//	    OnProgress:  func(r migrate.Result) { log.Printf("migrated %s: %v", r.TenantID, r.Err) },
//	})
//	report, err := m.MigrateAll(ctx, tenantIDs, "migrations/project")
//	if err != nil {
//	    // hard error (context cancelled, runner misconfiguration)
//	}
//	if len(report.Failures) > 0 {
//	    // partial failure — some tenants were not migrated
//	}
package migrate

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Runner abstracts the low-level migration execution so the parallelism logic
// can be tested without a real database or the migrate CLI.
type Runner interface {
	// Run applies the migrations at migrationPath to the schema for tenantID.
	// It must be safe to call concurrently from multiple goroutines.
	Run(ctx context.Context, dbURL string, tenantID uuid.UUID, migrationPath string) error
}

// Result holds the outcome of migrating one tenant schema.
type Result struct {
	TenantID  uuid.UUID
	Err       error         // nil on success
	Duration  time.Duration
}

// Report is the aggregate result returned by MigrateAll.
type Report struct {
	Total    int
	Passed   int
	Failures []Result
	Duration time.Duration
}

// Failed reports whether any tenants failed to migrate.
func (r *Report) Failed() bool { return len(r.Failures) > 0 }

// Options configures the ParallelMigrator.
type Options struct {
	// DBURL is the base PostgreSQL connection string. The runner appends
	// the tenant search_path. Required.
	DBURL string

	// Parallelism is the number of concurrent migration workers.
	// Defaults to runtime.GOMAXPROCS(0) (i.e. number of available CPUs).
	// Recommended ceiling: 8 — beyond this Postgres connection overhead
	// starts to dominate.
	Parallelism int

	// OnProgress is called after each tenant migration completes (success or
	// failure). It is called from worker goroutines; implementations must be
	// goroutine-safe. May be nil.
	OnProgress func(Result)

	// Runner is the migration executor. Defaults to CLIRunner (delegates to
	// the `migrate` CLI). Override for testing.
	Runner Runner
}

// ParallelMigrator runs tenant schema migrations concurrently with partial
// failure recovery.
type ParallelMigrator struct {
	opts Options
}

// New creates a ParallelMigrator with the given options.
// Panics if DBURL is empty (programming error).
func New(opts Options) *ParallelMigrator {
	if opts.DBURL == "" {
		panic("migrate: DBURL is required")
	}
	if opts.Parallelism <= 0 {
		opts.Parallelism = runtime.GOMAXPROCS(0)
		if opts.Parallelism > 8 {
			opts.Parallelism = 8 // cap at 8 to avoid Postgres connection exhaustion
		}
	}
	if opts.Runner == nil {
		opts.Runner = &CLIRunner{}
	}
	return &ParallelMigrator{opts: opts}
}

// MigrateAll runs migrations for all tenants concurrently. It returns a Report
// summarising successes and failures. A non-nil error is only returned for hard
// failures (context cancelled before any work, invalid arguments). Partial
// failures are expressed via Report.Failures, not the error return.
func (m *ParallelMigrator) MigrateAll(ctx context.Context, tenantIDs []uuid.UUID, migrationPath string) (*Report, error) {
	if migrationPath == "" {
		return nil, fmt.Errorf("migrate: migrationPath is required")
	}
	if len(tenantIDs) == 0 {
		return &Report{}, nil
	}

	start := time.Now()
	jobs := make(chan uuid.UUID, len(tenantIDs))
	results := make(chan Result, len(tenantIDs))

	// Seed jobs
	for _, id := range tenantIDs {
		jobs <- id
	}
	close(jobs)

	// Launch worker pool
	var wg sync.WaitGroup
	for i := 0; i < m.opts.Parallelism; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for tenantID := range jobs {
				if err := ctx.Err(); err != nil {
					results <- Result{TenantID: tenantID, Err: err}
					continue
				}
				t := time.Now()
				err := m.opts.Runner.Run(ctx, m.opts.DBURL, tenantID, migrationPath)
				r := Result{TenantID: tenantID, Err: err, Duration: time.Since(t)}
				results <- r
				if m.opts.OnProgress != nil {
					m.opts.OnProgress(r)
				}
			}
		}()
	}

	// Wait for all workers, then close results
	go func() {
		wg.Wait()
		close(results)
	}()

	report := &Report{Total: len(tenantIDs)}
	for r := range results {
		if r.Err != nil {
			report.Failures = append(report.Failures, r)
		} else {
			report.Passed++
		}
	}
	report.Duration = time.Since(start)
	return report, nil
}

// MigrateTenant runs migrations for a single tenant. It is a thin wrapper over
// Runner.Run, provided for the `make migrate-tenant id=<uuid>` target.
func (m *ParallelMigrator) MigrateTenant(ctx context.Context, tenantID uuid.UUID, migrationPath string) error {
	if migrationPath == "" {
		return fmt.Errorf("migrate: migrationPath is required")
	}
	return m.opts.Runner.Run(ctx, m.opts.DBURL, tenantID, migrationPath)
}

// --- CLIRunner ---

// CLIRunner is the production Runner implementation. It delegates to the
// `migrate` CLI (github.com/golang-migrate/migrate) which must be on $PATH.
//
// The tenant schema is addressed by appending &search_path=tenant_<uuid> to
// the database URL. This follows the same pattern used by the tenantdb package
// to set search_path per connection.
type CLIRunner struct{}

// Run calls `migrate -path <migrationPath> -database <tenantURL> up`.
// It uses exec.CommandContext so the process is cancelled when ctx expires.
func (r *CLIRunner) Run(ctx context.Context, dbURL string, tenantID uuid.UUID, migrationPath string) error {
	if tenantID == uuid.Nil {
		return fmt.Errorf("migrate: tenant ID must be non-nil")
	}
	// Append the tenant schema as search_path. uuid.UUID.String() produces
	// only hex digits and hyphens — no injection risk.
	tenantURL := fmt.Sprintf("%s&search_path=tenant_%s,public&x-migrations-table=schema_migrations_%s",
		dbURL, tenantID.String(), tenantID.String())

	//nolint:gosec // migrationPath comes from a trusted caller (Makefile / internal code), not user input.
	cmd := exec.CommandContext(ctx, "migrate",
		"-path", migrationPath,
		"-database", tenantURL,
		"up",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("migrate tenant %s: %w\noutput: %s", tenantID, err, out)
	}
	return nil
}
