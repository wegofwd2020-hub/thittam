//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/testdb"
)

func TestPurgeTenant_SQL_DropsSchema_Tombstones_PreservesAudit(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool) // owner role; DDL is transactional → auto rollback
	ctx := context.Background()

	id := uuid.New()
	reqID := uuid.New()
	schema := "tenant_" + id.String()

	// Seed: a purge_eligible tenant, its schema, an approved purge request,
	// and an audit row.
	_, err := tx.Exec(ctx, `INSERT INTO tenants (id, name, slug, country_code, primary_currency_code, status, deactivated_at)
		VALUES ($1, $2, $3, 'US', 'USD', 'purge_eligible', now() - INTERVAL '200 days')`,
		id, "Doomed Studios", "slug-"+id.String()[:8])
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `CREATE SCHEMA "`+schema+`"`)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `INSERT INTO tenant_purge_requests
		(id, tenant_id, status, requested_by, request_reason, tenant_name, tenant_slug, approved_by, approved_at)
		VALUES ($1, $2, 'approved', $2, 'gdpr erasure', 'Doomed Studios', 'slug-'||$2::text, $2, now())`,
		reqID, id)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `INSERT INTO audit_log (id, tenant_id, actor_id, actor_email, action, resource_type, resource_id, occurred_at)
		VALUES (gen_random_uuid(), $1, $1, 'a@b.c', 'tenant_purged', 'tenant', $1, now())`, id)
	require.NoError(t, err)

	// Act: the exact statements PurgeTenantSchemaAndTombstone runs, in the
	// same order (claim → drop → tombstone).
	// Mirrors PurgeTenantSchemaAndTombstone ordering — keep in sync.
	claimCt, err := tx.Exec(ctx,
		`UPDATE tenant_purge_requests SET status='executed', executed_at=now() WHERE id=$1 AND status='approved'`,
		reqID)
	require.NoError(t, err)
	require.Equal(t, int64(1), claimCt.RowsAffected(), "exactly one purge request claimed")

	_, err = tx.Exec(ctx, `DROP SCHEMA IF EXISTS "`+schema+`" CASCADE`)
	require.NoError(t, err)
	ct, err := tx.Exec(ctx, `UPDATE tenants SET status='purged',
		name = 'purged-' || id::text, address_line1=NULL, address_line2=NULL, city=NULL, postal_code=NULL,
		purged_at=now() WHERE id=$1 AND status='purge_eligible'`, id)
	require.NoError(t, err)
	require.Equal(t, int64(1), ct.RowsAffected(), "exactly one tenant tombstoned")

	// Assert: schema gone.
	var n int
	require.NoError(t, tx.QueryRow(ctx,
		`SELECT count(*) FROM information_schema.schemata WHERE schema_name=$1`, schema).Scan(&n))
	assert.Equal(t, 0, n, "tenant schema must be dropped")

	// Assert: row tombstoned.
	var status, name string
	require.NoError(t, tx.QueryRow(ctx, `SELECT status, name FROM tenants WHERE id=$1`, id).Scan(&status, &name))
	assert.Equal(t, "purged", status)
	assert.Equal(t, "purged-"+id.String(), name)

	// Assert: purge request claimed (executed).
	var reqStatus string
	require.NoError(t, tx.QueryRow(ctx, `SELECT status FROM tenant_purge_requests WHERE id=$1`, reqID).Scan(&reqStatus))
	assert.Equal(t, "executed", reqStatus, "purge request must be marked executed")

	// Assert: audit row SURVIVES the drop (shared public schema).
	require.NoError(t, tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id=$1`, id).Scan(&n))
	assert.Equal(t, 1, n, "audit_log row must survive the schema drop")
}
