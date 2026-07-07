//go:build integration

// Full-lifecycle audit integration test for #92 Task 2. Drives a tenant
// through all three sweeper transitions (suspended → grace → deactivated →
// purge_eligible) via AdvanceTenantLifecycle and confirms one status_changed
// audit row is durably written per transition.
//
// This test intentionally uses the pool directly instead of testdb.NewTx:
// the audit.Logger flushes asynchronously through its own Store on the pool,
// so it cannot participate in — or see uncommitted writes inside — a test
// transaction. Tenant status updates and the resulting audit rows must share
// the same committed connection pool. Cleanup is manual (t.Cleanup deletes
// both rows by tenant_id) since there's no transaction to roll back.
package db_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/audit"
	"github.com/wegofwd2020/thittam/pkg/testdb"
	iam "github.com/wegofwd2020/thittam/services/iam"
	iamdb "github.com/wegofwd2020/thittam/services/iam/db"
)

// ownerDeleteAuditLog deletes a tenant's audit_log rows via the owner DSN when
// the suite runs as thittam_app (THITTAM_TEST_OWNER_DSN set; thittam_app can't
// DELETE audit_log); else via the given pool (local single-role runs).
func ownerDeleteAuditLog(ctx context.Context, pool *pgxpool.Pool, tenant uuid.UUID) {
	if dsn := os.Getenv("THITTAM_TEST_OWNER_DSN"); dsn != "" {
		if op, err := pgxpool.New(ctx, dsn); err == nil {
			defer op.Close()
			_, _ = op.Exec(ctx, `DELETE FROM audit_log WHERE tenant_id = $1`, tenant)
			return
		}
	}
	_, _ = pool.Exec(ctx, `DELETE FROM audit_log WHERE tenant_id = $1`, tenant)
}

// lifecycleThresholds returns the three "now" values that drive
// AdvanceTenantLifecycle through suspended→grace→deactivated→purge_eligible,
// one transition per call. Durations are services/iam/lifecycle.go's:
//
//	SuspensionGracePeriod       = 30 * 24h  (suspended → grace, from suspended_at)
//	GraceToDeactivatedDuration  = 90 * 24h  (grace → deactivated, from suspended_at)
//	DeactivatedToPurgeDuration  = 180 * 24h (deactivated → purge_eligible, from deactivated_at)
//
// deactivated_at is stamped by the DB's own now() at transition time (see
// TransitionTenantStatus's SQL CASE), not by the "now" argument passed to
// AdvanceTenantLifecycle — so it lands at approximately `base` (the test
// runs in well under a second). The third offset (base+272d) clears the
// 180-day deactivated→purge_eligible window measured from that real
// deactivated_at with comfortable margin.
func lifecycleThresholds(base time.Time) [3]time.Time {
	return [3]time.Time{
		base.Add(31 * 24 * time.Hour),  // suspended → grace (needs >= 30d since suspended_at)
		base.Add(91 * 24 * time.Hour),  // grace → deactivated (needs >= 90d since suspended_at)
		base.Add(272 * 24 * time.Hour), // deactivated → purge_eligible (needs >= 180d since deactivated_at)
	}
}

func TestTenantLifecycle_EmitsAuditPerTransition(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	tenantID := uuid.New()
	t.Cleanup(func() {
		ownerDeleteAuditLog(ctx, pool, tenantID) // audit_log: owner only under the split
		_, _ = pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
	})

	// Seed a suspended tenant; suspended_at drives the first transition.
	base := time.Now().UTC()
	_, err := pool.Exec(ctx, `
		INSERT INTO tenants (id, name, slug, country_code, primary_currency_code, status, suspended_at)
		VALUES ($1, $2, $3, 'US', 'USD', 'suspended', $4)`,
		tenantID, "Lifecycle Audit Co", "slug-"+tenantID.String()[:8], base.Add(-1*time.Hour))
	require.NoError(t, err)

	auditStore := audit.NewPostgres(pool)
	logger := audit.NewLogger(auditStore, audit.DefaultConfig(), nil)
	svc := iam.NewService(iamdb.NewPostgres(pool), nil, nil, nil, nil).WithAuditLogger(logger)

	// Advance through each stage; each call performs at most one conditional
	// transition, so one call per threshold advances exactly one stage.
	for _, now := range lifecycleThresholds(base) {
		transition, err := svc.AdvanceTenantLifecycle(ctx, tenantID, now)
		require.NoError(t, err)
		require.NotNil(t, transition, "expected a transition at now=%s", now)
	}

	require.NoError(t, logger.Close(ctx)) // flush async events before asserting

	// Final status is purge_eligible.
	var status string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM tenants WHERE id = $1`, tenantID).Scan(&status))
	assert.Equal(t, "purge_eligible", status)

	// One status_changed audit row per transition, actor = system:retention-sweeper.
	sc := audit.ActionStatusChanged
	events, err := auditStore.Query(ctx, audit.QueryFilter{TenantID: tenantID, Action: &sc, Limit: 100})
	require.NoError(t, err)
	assert.Len(t, events, 3)
	for _, e := range events {
		assert.Equal(t, "system:retention-sweeper", e.ActorEmail)
		assert.Equal(t, audit.ResourceTenant, e.ResourceType)
	}
}
