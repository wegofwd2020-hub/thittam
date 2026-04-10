package migrate_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/migrate"
)

// --- Mock runner ---

type mockRunner struct {
	// fn is called once per tenant. If nil, returns nil (success).
	fn func(ctx context.Context, dbURL string, tenantID uuid.UUID, migrationPath string) error
}

func (m *mockRunner) Run(ctx context.Context, dbURL string, tenantID uuid.UUID, migrationPath string) error {
	if m.fn != nil {
		return m.fn(ctx, dbURL, tenantID, migrationPath)
	}
	return nil
}

// --- Fixtures ---

func tenants(n int) []uuid.UUID {
	ids := make([]uuid.UUID, n)
	for i := range ids {
		ids[i] = uuid.New()
	}
	return ids
}

func opts(r migrate.Runner) migrate.Options {
	return migrate.Options{
		DBURL:       "postgres://localhost/thittam?sslmode=disable",
		Parallelism: 4,
		Runner:      r,
	}
}

// --- Tests ---

func TestMigrateAll_AllSuccess(t *testing.T) {
	t.Parallel()
	var count atomic.Int64
	runner := &mockRunner{fn: func(_ context.Context, _ string, _ uuid.UUID, _ string) error {
		count.Add(1)
		return nil
	}}

	m := migrate.New(opts(runner))
	report, err := m.MigrateAll(context.Background(), tenants(10), "migrations/project")
	require.NoError(t, err)
	assert.Equal(t, 10, report.Total)
	assert.Equal(t, 10, report.Passed)
	assert.Empty(t, report.Failures)
	assert.False(t, report.Failed())
	assert.Equal(t, int64(10), count.Load())
}

func TestMigrateAll_PartialFailure(t *testing.T) {
	t.Parallel()
	errMigration := errors.New("migration: relation already exists")
	ids := tenants(5)
	failID := ids[2]

	runner := &mockRunner{fn: func(_ context.Context, _ string, tenantID uuid.UUID, _ string) error {
		if tenantID == failID {
			return errMigration
		}
		return nil
	}}

	m := migrate.New(opts(runner))
	report, err := m.MigrateAll(context.Background(), ids, "migrations/project")
	require.NoError(t, err) // partial failure is NOT a hard error
	assert.Equal(t, 5, report.Total)
	assert.Equal(t, 4, report.Passed)
	require.Len(t, report.Failures, 1)
	assert.Equal(t, failID, report.Failures[0].TenantID)
	assert.ErrorIs(t, report.Failures[0].Err, errMigration)
	assert.True(t, report.Failed())
}

func TestMigrateAll_AllFail(t *testing.T) {
	t.Parallel()
	errBoom := errors.New("db unreachable")
	runner := &mockRunner{fn: func(_ context.Context, _ string, _ uuid.UUID, _ string) error {
		return errBoom
	}}

	m := migrate.New(opts(runner))
	report, err := m.MigrateAll(context.Background(), tenants(3), "migrations/project")
	require.NoError(t, err) // all-fail is still not a hard error
	assert.Equal(t, 3, report.Total)
	assert.Equal(t, 0, report.Passed)
	assert.Len(t, report.Failures, 3)
}

func TestMigrateAll_EmptyTenantList(t *testing.T) {
	t.Parallel()
	var called atomic.Bool
	runner := &mockRunner{fn: func(_ context.Context, _ string, _ uuid.UUID, _ string) error {
		called.Store(true)
		return nil
	}}

	m := migrate.New(opts(runner))
	report, err := m.MigrateAll(context.Background(), nil, "migrations/project")
	require.NoError(t, err)
	assert.Equal(t, 0, report.Total)
	assert.False(t, called.Load())
}

func TestMigrateAll_MissingMigrationPath(t *testing.T) {
	t.Parallel()
	m := migrate.New(opts(&mockRunner{}))
	_, err := m.MigrateAll(context.Background(), tenants(1), "")
	require.Error(t, err)
}

func TestMigrateAll_ContextCancelled(t *testing.T) {
	t.Parallel()
	runner := &mockRunner{fn: func(ctx context.Context, _ string, _ uuid.UUID, _ string) error {
		// simulate slow migration
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return nil
		}
	}}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	m := migrate.New(opts(runner))
	report, err := m.MigrateAll(ctx, tenants(4), "migrations/project")
	require.NoError(t, err)
	// Some or all tenants should have context error
	assert.True(t, report.Failed() || report.Passed < 4)
}

func TestMigrateAll_OnProgressCalledForEach(t *testing.T) {
	t.Parallel()
	ids := tenants(6)
	var progressIDs []uuid.UUID
	var mu sync.Mutex //nolint:govet

	runner := &mockRunner{}
	o := opts(runner)
	o.OnProgress = func(r migrate.Result) {
		mu.Lock()
		defer mu.Unlock()
		progressIDs = append(progressIDs, r.TenantID)
	}

	m := migrate.New(o)
	_, err := m.MigrateAll(context.Background(), ids, "migrations/project")
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, progressIDs, 6)
}

func TestMigrateAll_ResultHasDuration(t *testing.T) {
	t.Parallel()
	runner := &mockRunner{}
	m := migrate.New(opts(runner))
	report, err := m.MigrateAll(context.Background(), tenants(3), "migrations/project")
	require.NoError(t, err)
	assert.Greater(t, report.Duration, time.Duration(0))
}

func TestMigrateAll_DefaultParallelism(t *testing.T) {
	t.Parallel()
	// Parallelism=0 should default to GOMAXPROCS (capped at 8), not panic.
	o := migrate.Options{
		DBURL:       "postgres://localhost/thittam?sslmode=disable",
		Parallelism: 0, // trigger default
		Runner:      &mockRunner{},
	}
	require.NotPanics(t, func() {
		m := migrate.New(o)
		_, _ = m.MigrateAll(context.Background(), tenants(2), "migrations/project")
	})
}

func TestMigrateAll_ConcurrentExecution(t *testing.T) {
	t.Parallel()
	// Verify workers run in parallel: 4 workers × 4 tenants, each sleeping 50ms,
	// total wall time should be ≤ 200ms (not 200ms × 4 = 800ms).
	runner := &mockRunner{fn: func(_ context.Context, _ string, _ uuid.UUID, _ string) error {
		time.Sleep(50 * time.Millisecond)
		return nil
	}}

	o := migrate.Options{
		DBURL:       "postgres://localhost/thittam?sslmode=disable",
		Parallelism: 4,
		Runner:      runner,
	}
	m := migrate.New(o)
	start := time.Now()
	_, err := m.MigrateAll(context.Background(), tenants(4), "migrations/project")
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Less(t, elapsed, 300*time.Millisecond,
		"4 parallel workers × 50ms should finish in <300ms, got %s", elapsed)
}

// --- Tests: MigrateTenant ---

func TestMigrateTenant_Success(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	var capturedID uuid.UUID
	runner := &mockRunner{fn: func(_ context.Context, _ string, tenantID uuid.UUID, _ string) error {
		capturedID = tenantID
		return nil
	}}

	m := migrate.New(opts(runner))
	err := m.MigrateTenant(context.Background(), tid, "migrations/project")
	require.NoError(t, err)
	assert.Equal(t, tid, capturedID)
}

func TestMigrateTenant_RunnerError(t *testing.T) {
	t.Parallel()
	errConn := errors.New("connection refused")
	runner := &mockRunner{fn: func(_ context.Context, _ string, _ uuid.UUID, _ string) error {
		return errConn
	}}

	m := migrate.New(opts(runner))
	err := m.MigrateTenant(context.Background(), uuid.New(), "migrations/project")
	require.Error(t, err)
	assert.ErrorIs(t, err, errConn)
}

func TestMigrateTenant_MissingPath(t *testing.T) {
	t.Parallel()
	m := migrate.New(opts(&mockRunner{}))
	err := m.MigrateTenant(context.Background(), uuid.New(), "")
	require.Error(t, err)
}

// --- Tests: CLIRunner URL construction (unit, no exec) ---

func TestCLIRunner_NilTenantIDRejected(t *testing.T) {
	t.Parallel()
	r := &migrate.CLIRunner{}
	err := r.Run(context.Background(), "postgres://localhost/db", uuid.Nil, "migrations/project")
	require.Error(t, err)
}

