//go:build integration

// Integration test for the user_roles project-scope trigger added in
// migration 012 (ADR-014 Phase 2 / issue #39). Verifies that:
//   - project_supervisor MUST carry a project_id
//   - tenant-wide system roles MUST NOT carry a project_id
//   - other combinations are accepted
//
// Each test wraps its work in a transaction (testdb.NewTx) so the trigger's
// RAISE EXCEPTION rolls back cleanly without polluting the test DB.

package db_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/testdb"
)

// seedRow inserts a tenant + user + role and returns their IDs. Runs inside
// the supplied tx so it disappears on rollback.
func seedRow(t *testing.T, tx pgx.Tx, roleName string) (tenantID, userID, roleID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	tenantID = uuid.New()
	userID = uuid.New()
	roleID = uuid.New()

	_, err := tx.Exec(ctx,
		`INSERT INTO tenants (id, name, slug, country_code, primary_currency_code) VALUES ($1, $2, $3, 'US', 'USD')`,
		tenantID, "test-tenant-"+tenantID.String()[:8], "test-"+tenantID.String()[:8])
	require.NoError(t, err, "insert tenant")

	_, err = tx.Exec(ctx,
		`INSERT INTO users (id, tenant_id, email, display_name, password_hash)
		 VALUES ($1, $2, $3, $4, 'x')`,
		userID, tenantID, userID.String()+"@test.local", "Test User")
	require.NoError(t, err, "insert user")

	_, err = tx.Exec(ctx,
		`INSERT INTO roles (id, tenant_id, name, permissions, is_system)
		 VALUES ($1, $2, $3, '{}', true)`,
		roleID, tenantID, roleName)
	require.NoError(t, err, "insert role")

	return tenantID, userID, roleID
}

func assertCheckViolation(t *testing.T, err error, mustContain string) {
	t.Helper()
	require.Error(t, err)
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr, "expected pgconn.PgError, got %T", err)
	// The trigger raises with ERRCODE = 'check_violation' (23514).
	assert.Equal(t, "23514", pgErr.Code, "expected check_violation, got %s", pgErr.Code)
	assert.Contains(t, pgErr.Message, mustContain)
}

func TestUserRolesScope_ProjectSupervisorRequiresProjectID(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	_, userID, roleID := seedRow(t, tx, "project_supervisor")

	_, err := tx.Exec(context.Background(),
		`INSERT INTO user_roles (user_id, role_id, project_id, assigned_by)
		 VALUES ($1, $2, NULL, $1)`,
		userID, roleID)
	assertCheckViolation(t, err, "project_supervisor")
}

func TestUserRolesScope_ProjectSupervisorWithProjectIDAccepted(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	_, userID, roleID := seedRow(t, tx, "project_supervisor")
	projectID := uuid.New()

	_, err := tx.Exec(context.Background(),
		`INSERT INTO user_roles (user_id, role_id, project_id, assigned_by)
		 VALUES ($1, $2, $3, $1)`,
		userID, roleID, projectID)
	require.NoError(t, err)
}

func TestUserRolesScope_TenantWideRoleRejectsProjectID(t *testing.T) {
	cases := []string{"super_admin", "manager", "coordinator", "accountant", "inventory_manager", "member"}
	for _, roleName := range cases {
		roleName := roleName
		t.Run(roleName, func(t *testing.T) {
			pool := testdb.Open(t)
			tx := testdb.NewTx(t, pool)
			_, userID, roleID := seedRow(t, tx, roleName)
			projectID := uuid.New()

			_, err := tx.Exec(context.Background(),
				`INSERT INTO user_roles (user_id, role_id, project_id, assigned_by)
				 VALUES ($1, $2, $3, $1)`,
				userID, roleID, projectID)
			assertCheckViolation(t, err, roleName)
		})
	}
}

func TestUserRolesScope_TenantWideRoleAcceptsNullProjectID(t *testing.T) {
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	_, userID, roleID := seedRow(t, tx, "manager")

	_, err := tx.Exec(context.Background(),
		`INSERT INTO user_roles (user_id, role_id, project_id, assigned_by)
		 VALUES ($1, $2, NULL, $1)`,
		userID, roleID)
	require.NoError(t, err)
}

func TestUserRolesScope_CustomRoleUnconstrained(t *testing.T) {
	// Custom (is_system=false) roles bypass the trigger contract.
	pool := testdb.Open(t)
	tx := testdb.NewTx(t, pool)
	tenantID := uuid.New()
	userID := uuid.New()
	roleID := uuid.New()

	ctx := context.Background()
	_, err := tx.Exec(ctx, `INSERT INTO tenants (id, name, slug, country_code, primary_currency_code) VALUES ($1, $2, $3, 'US', 'USD')`,
		tenantID, "custom-"+tenantID.String()[:8], "cust-"+tenantID.String()[:8])
	require.NoError(t, err)
	_, err = tx.Exec(ctx,
		`INSERT INTO users (id, tenant_id, email, display_name, password_hash)
		 VALUES ($1, $2, $3, 'u', 'x')`,
		userID, tenantID, userID.String()+"@test.local")
	require.NoError(t, err)
	_, err = tx.Exec(ctx,
		`INSERT INTO roles (id, tenant_id, name, permissions, is_system)
		 VALUES ($1, $2, 'auditor', '{}', false)`,
		roleID, tenantID)
	require.NoError(t, err)

	// Both NULL and non-NULL project_id should be accepted for custom roles.
	_, err = tx.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id, project_id, assigned_by)
		 VALUES ($1, $2, NULL, $1)`,
		userID, roleID)
	require.NoError(t, err, "custom role with NULL project_id")

	_, err = tx.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id, project_id, assigned_by)
		 VALUES ($1, $2, $3, $1)`,
		userID, roleID, uuid.New())
	require.NoError(t, err, "custom role with non-NULL project_id")
}
