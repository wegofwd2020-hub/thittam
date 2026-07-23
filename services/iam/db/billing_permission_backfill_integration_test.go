//go:build integration

// Integration test for migration 022 (#139 slice F). The migration grants
// billing:read to three system roles and billing:manage to two of them.
// These tests prove the properties the migration depends on: each UPDATE
// appends its permission idempotently, and the name IN (...) lists are
// enforced -- in particular accountant, which is in billing:read's grant
// set but NOT billing:manage's, must never pick up billing:manage.
//
// migrations/iam runs against the public schema via `make migrate-all` AND
// against every new tenant_<uuid> at CreateTenant, so a non-idempotent
// statement would duplicate entries.

package db_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/testdb"
)

// backfillBillingRead is the exact statement from
// migrations/iam/022_seed_billing_permissions.up.sql. Keep the two in sync.
const backfillBillingRead = `
UPDATE roles SET permissions = array_append(permissions, 'billing:read')
WHERE is_system = true
  AND name IN ('super_admin', 'manager', 'accountant')
  AND NOT ('billing:read' = ANY (permissions))`

// backfillBillingManage is the exact statement from
// migrations/iam/022_seed_billing_permissions.up.sql. Keep the two in sync.
const backfillBillingManage = `
UPDATE roles SET permissions = array_append(permissions, 'billing:manage')
WHERE is_system = true
  AND name IN ('super_admin', 'manager')
  AND NOT ('billing:manage' = ANY (permissions))`

func TestMigration022_GrantsBillingReadIdempotently(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	tx := testdb.NewTx(t, pool)

	tenantID := uuid.New()
	_, err := tx.Exec(ctx,
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'IN', 'INR')`,
		tenantID, "Backfill Test "+tenantID.String(), "backfill-"+tenantID.String())
	require.NoError(t, err)

	// A system role as it exists BEFORE the migration: no billing:read.
	_, err = tx.Exec(ctx,
		`INSERT INTO roles (id, tenant_id, name, permissions, is_system)
		 VALUES ($1, $2, 'accountant', $3, true)`,
		uuid.New(), tenantID, []string{"expense:read"})
	require.NoError(t, err)

	// First application appends it.
	_, err = tx.Exec(ctx, backfillBillingRead)
	require.NoError(t, err)

	var perms []string
	require.NoError(t, tx.QueryRow(ctx,
		`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = 'accountant'`,
		tenantID).Scan(&perms))
	require.Contains(t, perms, "billing:read")
	require.Equal(t, 1, countOccurrences(perms, "billing:read"))

	// Second application is a no-op -- this is the property that matters,
	// because the statement runs in more than one schema context.
	_, err = tx.Exec(ctx, backfillBillingRead)
	require.NoError(t, err)

	require.NoError(t, tx.QueryRow(ctx,
		`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = 'accountant'`,
		tenantID).Scan(&perms))
	require.Equal(t, 1, countOccurrences(perms, "billing:read"),
		"re-running the migration must not duplicate the permission")
}

func TestMigration022_BillingManageNotGrantedToAccountant(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	tx := testdb.NewTx(t, pool)

	tenantID := uuid.New()
	_, err := tx.Exec(ctx,
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'IN', 'INR')`,
		tenantID, "Accountant Test "+tenantID.String(), "accountant-"+tenantID.String())
	require.NoError(t, err)

	// accountant is in billing:read's grant set but NOT billing:manage's.
	_, err = tx.Exec(ctx,
		`INSERT INTO roles (id, tenant_id, name, permissions, is_system)
		 VALUES ($1, $2, 'accountant', $3, true)`,
		uuid.New(), tenantID, []string{"expense:read"})
	require.NoError(t, err)

	// Apply both statements, exactly as the migration does.
	_, err = tx.Exec(ctx, backfillBillingRead)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, backfillBillingManage)
	require.NoError(t, err)

	var perms []string
	require.NoError(t, tx.QueryRow(ctx,
		`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = 'accountant'`,
		tenantID).Scan(&perms))
	require.Contains(t, perms, "billing:read",
		"accountant must receive billing:read")
	require.NotContains(t, perms, "billing:manage",
		"accountant is outside billing:manage's name IN (...) list -- the list must be enforced, not just the array_append idempotency")
}
