//go:build integration

// Integration test for migration 020 (#139 slice D). The migration grants
// expense:read and inventory:read to existing system roles. These tests prove
// the properties the migration depends on: each UPDATE appends the permission
// idempotently, member is excluded from expense:read, and the down migration
// removes inventory:read from the roles it granted it to while leaving
// inventory_manager's pre-existing grant intact.
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

// backfillExpenseRead is the exact statement from
// migrations/iam/020_seed_read_permissions.up.sql. Keep the two in sync.
const backfillExpenseRead = `
UPDATE roles
SET permissions = array_append(permissions, 'expense:read')
WHERE is_system = true
  AND name IN ('super_admin', 'manager', 'coordinator', 'accountant', 'project_supervisor')
  AND NOT ('expense:read' = ANY (permissions))`

// backfillInventoryRead is the exact statement from
// migrations/iam/020_seed_read_permissions.up.sql. Keep the two in sync.
const backfillInventoryRead = `
UPDATE roles
SET permissions = array_append(permissions, 'inventory:read')
WHERE is_system = true
  AND name IN ('super_admin', 'manager', 'coordinator', 'project_supervisor')
  AND NOT ('inventory:read' = ANY (permissions))`

// backfillInventoryReadDown is the exact statement from
// migrations/iam/020_seed_read_permissions.down.sql. Keep the two in sync.
const backfillInventoryReadDown = `
UPDATE roles
SET permissions = array_remove(permissions, 'inventory:read')
WHERE is_system = true
  AND name <> 'inventory_manager'`

func TestMigration020_GrantsExpenseReadIdempotently(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	tx := testdb.NewTx(t, pool)

	tenantID := uuid.New()
	_, err := tx.Exec(ctx,
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'IN', 'INR')`,
		tenantID, "Backfill Test "+tenantID.String(), "backfill-"+tenantID.String())
	require.NoError(t, err)

	// A system role as it exists BEFORE the migration: no expense:read.
	_, err = tx.Exec(ctx,
		`INSERT INTO roles (id, tenant_id, name, permissions, is_system)
		 VALUES ($1, $2, 'manager', $3, true)`,
		uuid.New(), tenantID, []string{"budget:read", "expense:approve"})
	require.NoError(t, err)

	// First application appends it.
	_, err = tx.Exec(ctx, backfillExpenseRead)
	require.NoError(t, err)

	var perms []string
	require.NoError(t, tx.QueryRow(ctx,
		`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = 'manager'`,
		tenantID).Scan(&perms))
	require.Contains(t, perms, "expense:read")
	require.Equal(t, 1, countOccurrences(perms, "expense:read"))

	// Second application is a no-op — this is the property that matters,
	// because the statement runs in more than one schema context.
	_, err = tx.Exec(ctx, backfillExpenseRead)
	require.NoError(t, err)

	require.NoError(t, tx.QueryRow(ctx,
		`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = 'manager'`,
		tenantID).Scan(&perms))
	require.Equal(t, 1, countOccurrences(perms, "expense:read"),
		"re-running the migration must not duplicate the permission")
}

func TestMigration020_LeavesMemberAlone(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	tx := testdb.NewTx(t, pool)

	tenantID := uuid.New()
	_, err := tx.Exec(ctx,
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'IN', 'INR')`,
		tenantID, "Member Test "+tenantID.String(), "member-"+tenantID.String())
	require.NoError(t, err)

	_, err = tx.Exec(ctx,
		`INSERT INTO roles (id, tenant_id, name, permissions, is_system)
		 VALUES ($1, $2, 'member', $3, true)`,
		uuid.New(), tenantID, []string{"production:read", "expense:submit"})
	require.NoError(t, err)

	_, err = tx.Exec(ctx, backfillExpenseRead)
	require.NoError(t, err)

	var perms []string
	require.NoError(t, tx.QueryRow(ctx,
		`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = 'member'`,
		tenantID).Scan(&perms))
	require.NotContains(t, perms, "expense:read",
		"member is deliberately excluded: ListExpenses has no submitted_by filter")
}

func TestMigration020_GrantsInventoryReadIdempotently(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	tx := testdb.NewTx(t, pool)

	tenantID := uuid.New()
	_, err := tx.Exec(ctx,
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'IN', 'INR')`,
		tenantID, "Backfill Test "+tenantID.String(), "backfill-"+tenantID.String())
	require.NoError(t, err)

	// A system role as it exists BEFORE the migration: no inventory:read.
	_, err = tx.Exec(ctx,
		`INSERT INTO roles (id, tenant_id, name, permissions, is_system)
		 VALUES ($1, $2, 'manager', $3, true)`,
		uuid.New(), tenantID, []string{"budget:read", "inventory:checkout"})
	require.NoError(t, err)

	// First application appends it.
	_, err = tx.Exec(ctx, backfillInventoryRead)
	require.NoError(t, err)

	var perms []string
	require.NoError(t, tx.QueryRow(ctx,
		`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = 'manager'`,
		tenantID).Scan(&perms))
	require.Contains(t, perms, "inventory:read")
	require.Equal(t, 1, countOccurrences(perms, "inventory:read"))

	// Second application is a no-op — this is the property that matters,
	// because the statement runs in more than one schema context.
	_, err = tx.Exec(ctx, backfillInventoryRead)
	require.NoError(t, err)

	require.NoError(t, tx.QueryRow(ctx,
		`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = 'manager'`,
		tenantID).Scan(&perms))
	require.Equal(t, 1, countOccurrences(perms, "inventory:read"),
		"re-running the migration must not duplicate the permission")
}

func TestMigration020_DownLeavesInventoryManagerAlone(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	tx := testdb.NewTx(t, pool)

	tenantID := uuid.New()
	_, err := tx.Exec(ctx,
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'IN', 'INR')`,
		tenantID, "Down Test "+tenantID.String(), "down-"+tenantID.String())
	require.NoError(t, err)

	// inventory_manager held inventory:read BEFORE this migration — the down
	// statement must not touch it.
	_, err = tx.Exec(ctx,
		`INSERT INTO roles (id, tenant_id, name, permissions, is_system)
		 VALUES ($1, $2, 'inventory_manager', $3, true)`,
		uuid.New(), tenantID, []string{"inventory:read", "inventory:write", "inventory:checkout", "inventory:retire"})
	require.NoError(t, err)

	// manager only received inventory:read from this migration — the down
	// statement must remove it.
	_, err = tx.Exec(ctx,
		`INSERT INTO roles (id, tenant_id, name, permissions, is_system)
		 VALUES ($1, $2, 'manager', $3, true)`,
		uuid.New(), tenantID, []string{"budget:read", "inventory:read", "inventory:checkout"})
	require.NoError(t, err)

	_, err = tx.Exec(ctx, backfillInventoryReadDown)
	require.NoError(t, err)

	var managerPerms, inventoryManagerPerms []string
	require.NoError(t, tx.QueryRow(ctx,
		`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = 'manager'`,
		tenantID).Scan(&managerPerms))
	require.NoError(t, tx.QueryRow(ctx,
		`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = 'inventory_manager'`,
		tenantID).Scan(&inventoryManagerPerms))

	require.NotContains(t, managerPerms, "inventory:read",
		"down migration must remove inventory:read from roles it granted")
	require.Contains(t, inventoryManagerPerms, "inventory:read",
		"down migration must not revoke a grant it never issued")
}

func countOccurrences(xs []string, want string) int {
	n := 0
	for _, x := range xs {
		if x == want {
			n++
		}
	}
	return n
}
