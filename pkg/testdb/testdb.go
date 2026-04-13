// Package testdb provides shared helpers for integration tests that need a
// real PostgreSQL connection.
//
// Test DB lifecycle is owned by `make db-test-bootstrap` / `make db-test-reset`
// (and the corresponding scripts/db-test-*.sh). This package never creates,
// drops, or migrates the database — it only opens connections and hands out
// auto-rollback transactions.
//
// Usage:
//
//	//go:build integration
//
//	pool := testdb.Open(t)
//	tx := testdb.NewTx(t, pool)
//	// ... use tx as your DB handle; rollback happens automatically on cleanup
package testdb

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// EnvDSN is the env var integration tests read for the test DB connection.
const EnvDSN = "THITTAM_TEST_DSN"

// Open returns a connection pool to the test database. If THITTAM_TEST_DSN is
// unset the test is skipped — keeping `go test ./...` (no env, no DB) green for
// developers who haven't bootstrapped the test DB.
func Open(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv(EnvDSN)
	if dsn == "" {
		t.Skipf("%s not set — run 'make db-test-bootstrap' and re-run with -tags=integration", EnvDSN)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err, "open pool")
	require.NoError(t, pool.Ping(ctx), "ping db")
	t.Cleanup(pool.Close)
	return pool
}

// NewTx begins a transaction and registers an automatic rollback on test
// cleanup. The returned tx is the only DB handle the test should use, so all
// changes are isolated and torn down even on assertion failure.
//
// Caveat: sequences and trigger-side-effects on auxiliary tables are not
// rolled back if they happen outside the tx (they should never in our code).
func NewTx(t *testing.T, pool *pgxpool.Pool) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(context.Background())
	require.NoError(t, err, "begin tx")
	t.Cleanup(func() {
		// Rollback is best-effort — if the test already rolled back via an
		// expected DB error, this is a no-op.
		_ = tx.Rollback(context.Background())
	})
	return tx
}
