//go:build integration

package audit_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuditLog_AppRole_AppendOnly proves the DB-level append-only enforcement:
// the least-privilege thittam_app role can INSERT/SELECT audit_log but NOT
// UPDATE/DELETE it (the REVOKE in scripts/db-grant-app-role.sql). Requires the
// thittam_app DSN in THITTAM_TEST_APP_DSN (provisioned in CI); skips locally
// where the role can't be created (CREATE ROLE needs superuser).
func TestAuditLog_AppRole_AppendOnly(t *testing.T) {
	appDSN := os.Getenv("THITTAM_TEST_APP_DSN")
	if appDSN == "" {
		t.Skip("THITTAM_TEST_APP_DSN not set — thittam_app role not provisioned (CI-only)")
	}
	ctx := context.Background()

	appPool, err := pgxpool.New(ctx, appDSN)
	require.NoError(t, err)
	defer appPool.Close()

	tenant := uuid.New()
	// Cleanup via the OWNER DSN — thittam_app can't DELETE audit_log (that's the point).
	// Cleanup via the owner DSN (THITTAM_TEST_OWNER_DSN) — thittam_app can't
	// DELETE audit_log (that's the point). cleanupAuditLog is defined in
	// postgres_integration_test.go (same package).
	t.Cleanup(func() { cleanupAuditLog(ctx, appPool, tenant) })

	// INSERT allowed.
	_, err = appPool.Exec(ctx, `
		INSERT INTO audit_log (tenant_id, actor_id, actor_email, action, resource_type, resource_id)
		VALUES ($1, $2, 'system:test', 'status_changed', 'tenant', $1)`,
		tenant, uuid.Nil)
	require.NoError(t, err, "thittam_app must be able to INSERT audit_log")

	// SELECT allowed.
	var n int
	require.NoError(t, appPool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE tenant_id = $1`, tenant).Scan(&n))
	assert.Equal(t, 1, n)

	// UPDATE denied (42501 insufficient_privilege).
	_, err = appPool.Exec(ctx, `UPDATE audit_log SET action = 'tampered' WHERE tenant_id = $1`, tenant)
	assertInsufficientPrivilege(t, err, "UPDATE audit_log")

	// DELETE denied.
	_, err = appPool.Exec(ctx, `DELETE FROM audit_log WHERE tenant_id = $1`, tenant)
	assertInsufficientPrivilege(t, err, "DELETE audit_log")
}

func assertInsufficientPrivilege(t *testing.T, err error, op string) {
	t.Helper()
	require.Error(t, err, "%s must be denied for thittam_app", op)
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr, "%s: expected pgconn.PgError, got %T", op, err)
	assert.Equal(t, "42501", pgErr.Code, "%s: expected insufficient_privilege (42501), got %s", op, pgErr.Code)
}
