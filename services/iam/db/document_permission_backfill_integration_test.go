//go:build integration

// Integration test for migration 021 (#139 slice E). The migration grants
// document:read to all seven system roles, document:write to five of them,
// and document:delete to two. These tests prove the properties the
// migration depends on: each UPDATE appends its permission idempotently,
// the name IN (...) lists are enforced (a role outside a grant set does not
// receive it), and member -- the lowest-privilege role -- ends up with
// document:read only, never write or delete.
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

// backfillDocumentRead is the exact statement from
// migrations/iam/021_seed_document_permissions.up.sql. Keep the two in sync.
const backfillDocumentRead = `
UPDATE roles SET permissions = array_append(permissions, 'document:read')
WHERE is_system = true
  AND name IN ('super_admin', 'manager', 'coordinator', 'accountant', 'member', 'inventory_manager', 'project_supervisor')
  AND NOT ('document:read' = ANY (permissions))`

// backfillDocumentWrite is the exact statement from
// migrations/iam/021_seed_document_permissions.up.sql. Keep the two in sync.
const backfillDocumentWrite = `
UPDATE roles SET permissions = array_append(permissions, 'document:write')
WHERE is_system = true
  AND name IN ('super_admin', 'manager', 'coordinator', 'accountant', 'project_supervisor')
  AND NOT ('document:write' = ANY (permissions))`

// backfillDocumentDelete is the exact statement from
// migrations/iam/021_seed_document_permissions.up.sql. Keep the two in sync.
const backfillDocumentDelete = `
UPDATE roles SET permissions = array_append(permissions, 'document:delete')
WHERE is_system = true
  AND name IN ('super_admin', 'manager')
  AND NOT ('document:delete' = ANY (permissions))`

func TestMigration021_GrantsDocumentWriteIdempotently(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	tx := testdb.NewTx(t, pool)

	tenantID := uuid.New()
	_, err := tx.Exec(ctx,
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'IN', 'INR')`,
		tenantID, "Backfill Test "+tenantID.String(), "backfill-"+tenantID.String())
	require.NoError(t, err)

	// A system role as it exists BEFORE the migration: no document:write.
	_, err = tx.Exec(ctx,
		`INSERT INTO roles (id, tenant_id, name, permissions, is_system)
		 VALUES ($1, $2, 'manager', $3, true)`,
		uuid.New(), tenantID, []string{"budget:read", "expense:approve"})
	require.NoError(t, err)

	// First application appends it.
	_, err = tx.Exec(ctx, backfillDocumentWrite)
	require.NoError(t, err)

	var perms []string
	require.NoError(t, tx.QueryRow(ctx,
		`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = 'manager'`,
		tenantID).Scan(&perms))
	require.Contains(t, perms, "document:write")
	require.Equal(t, 1, countOccurrences(perms, "document:write"))

	// Second application is a no-op — this is the property that matters,
	// because the statement runs in more than one schema context.
	_, err = tx.Exec(ctx, backfillDocumentWrite)
	require.NoError(t, err)

	require.NoError(t, tx.QueryRow(ctx,
		`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = 'manager'`,
		tenantID).Scan(&perms))
	require.Equal(t, 1, countOccurrences(perms, "document:write"),
		"re-running the migration must not duplicate the permission")
}

func TestMigration021_DocumentDeleteNotGrantedOutsideItsList(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	tx := testdb.NewTx(t, pool)

	tenantID := uuid.New()
	_, err := tx.Exec(ctx,
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code)
		 VALUES ($1, $2, $3, 'IN', 'INR')`,
		tenantID, "Coordinator Test "+tenantID.String(), "coordinator-"+tenantID.String())
	require.NoError(t, err)

	// coordinator is in document:write's grant set but NOT document:delete's.
	_, err = tx.Exec(ctx,
		`INSERT INTO roles (id, tenant_id, name, permissions, is_system)
		 VALUES ($1, $2, 'coordinator', $3, true)`,
		uuid.New(), tenantID, []string{"production:read"})
	require.NoError(t, err)

	_, err = tx.Exec(ctx, backfillDocumentDelete)
	require.NoError(t, err)

	var perms []string
	require.NoError(t, tx.QueryRow(ctx,
		`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = 'coordinator'`,
		tenantID).Scan(&perms))
	require.NotContains(t, perms, "document:delete",
		"coordinator is outside document:delete's name IN (...) list — the list must be enforced, not just the array_append idempotency")
}

func TestMigration021_MemberGetsReadOnlyNeverWriteOrDelete(t *testing.T) {
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

	// Apply all three statements, exactly as the migration does.
	_, err = tx.Exec(ctx, backfillDocumentRead)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, backfillDocumentWrite)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, backfillDocumentDelete)
	require.NoError(t, err)

	var perms []string
	require.NoError(t, tx.QueryRow(ctx,
		`SELECT permissions FROM roles WHERE tenant_id = $1 AND name = 'member'`,
		tenantID).Scan(&perms))
	require.Contains(t, perms, "document:read",
		"member must receive document:read — billing.DownloadInvoice forwards the caller's token to document.GetDownloadURL")
	require.NotContains(t, perms, "document:write",
		"member must not receive document:write — it is outside member's grant set")
	require.NotContains(t, perms, "document:delete",
		"member must not receive document:delete — it is outside member's grant set")
}
