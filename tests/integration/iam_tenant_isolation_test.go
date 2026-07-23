//go:build integration

// Integration test for the iam tenant-isolation guard-by-type fix (#139
// slice H, task 3): GetUserByID and UpdatePasswordHash previously took no
// tenantID at all — GetUserByID's sqlc query (`GetUser`) had no tenant
// predicate, and UpdatePasswordHash's query was `:exec`, so a cross-tenant
// call would silently update zero rows and return nil (success) instead of
// an error. This test exercises the real Postgres repository
// (services/iam/db), not a double, so the actual `WHERE id=$1 AND
// tenant_id=$2` predicates — and, critically, UpdateUserPasswordHash's
// `:execrows` RowsAffected check — are the thing under test. Task 3b (the
// tenant purge pair) is appended to this file separately.
//
// Uses pkg/testdb (SKIPs without THITTAM_TEST_DSN); seeds rows directly via
// the pool and cleans them up in t.Cleanup, following the pattern in
// tests/integration/ledger_tenant_isolation_test.go.
package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/auth"
	"github.com/wegofwd2020/thittam/pkg/testdb"
	"github.com/wegofwd2020/thittam/services/iam"
	iamdb "github.com/wegofwd2020/thittam/services/iam/db"
)

// TestIAM_TenantIsolation_ChangePasswordDenied is the failing (pre-fix) /
// passing (post-fix) test for task 3a. Before the fix:
//   - GetUserByID's underlying query fetched by id alone, so tenant A could
//     read tenant B's user record (email, password hash, roles) just by
//     knowing B's user id.
//   - UpdatePasswordHash's underlying query was `:exec` with no tenant
//     predicate at all — tenant A calling ChangePassword with tenant B's user
//     id would silently affect zero rows and return nil, masking the
//     cross-tenant attempt as success.
func TestIAM_TenantIsolation_ChangePasswordDenied(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := iamdb.NewPostgres(pool)

	tenantA := seedIAMTenant(t, pool)
	tenantB := seedIAMTenant(t, pool)

	const victimOldPassword = "victim-old-pass"
	hasher := auth.NewArgon2idHasher()
	victimHash, err := hasher.Hash(victimOldPassword)
	require.NoError(t, err, "hash victim's original password")

	victim := seedIAMUser(t, pool, tenantB, victimHash)

	// --- GetUserByID: tenant A must never be able to read tenant B's user
	// record by id. ---
	_, err = repo.GetUserByID(ctx, tenantA, victim)
	assert.ErrorIs(t, err, iam.ErrUserNotFound, "cross-tenant GetUserByID must be refused, not silently resolved by id alone")

	// --- UpdatePasswordHash: the direct proof against the silent-:exec bug.
	// Called directly (bypassing the Service.ChangePassword read-then-write
	// flow) so the SQL predicate itself — not GetUserByID's earlier refusal —
	// is what's under test. Under the pre-fix `:exec` query this call would
	// silently affect zero rows and return nil (success). ---
	err = repo.UpdatePasswordHash(ctx, tenantA, victim, "attacker-supplied-hash")
	assert.ErrorIs(t, err, iam.ErrUserNotFound, "cross-tenant UpdatePasswordHash must be refused, not a silent no-op")

	stillOld := readPasswordHash(t, pool, victim)
	assert.Equal(t, victimHash, stillOld, "a refused cross-tenant password update must not mutate the victim's hash")

	// --- Full Service.ChangePassword flow: tenant A changing tenant B's
	// password, even with the correct old plaintext password, must be
	// refused at the tenant boundary — not at the password-verification
	// step. ---
	svc := iam.NewService(repo, nil, nil, hasher, auth.NewDualVerifier())
	err = svc.ChangePassword(ctx, tenantA, victim, victimOldPassword, "new-password-from-attacker")
	assert.ErrorIs(t, err, iam.ErrUserNotFound, "cross-tenant ChangePassword must be refused regardless of whether the old password is known")

	stillOldAfterService := readPasswordHash(t, pool, victim)
	assert.Equal(t, victimHash, stillOldAfterService, "a refused cross-tenant ChangePassword must not mutate the victim's hash")

	// Positive control: same-tenant ChangePassword succeeds end to end,
	// proving the fix didn't just break the predicate for everyone.
	require.NoError(t, svc.ChangePassword(ctx, tenantB, victim, victimOldPassword, "new-password-from-victim"))
	updated, err := repo.GetUserByID(ctx, tenantB, victim)
	require.NoError(t, err)
	require.NoError(t, auth.NewDualVerifier().Verify("new-password-from-victim", updated.PasswordHash), "same-tenant ChangePassword must actually take effect")
}

// TestIAM_TenantIsolation_PurgeApprovalDenied is the failing (pre-fix) /
// passing (post-fix) test for task 3b. Before the fix, ApproveTenantPurgeRequest
// and CancelTenantPurgeRequest took no tenantID at all — their underlying
// queries matched by request id (and status) only, so tenant A supplying
// tenant B's open purge request id would silently approve/cancel B's request.
// Exercises the real Postgres repository directly (not the Service, whose
// GetOpenTenantPurgeRequest(ctx, tenantID) call already scopes the request id
// it hands to Approve/Cancel) so the repo-level predicate itself is under test.
func TestIAM_TenantIsolation_PurgeApprovalDenied(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := iamdb.NewPostgres(pool)

	tenantA := uuid.New()
	tenantB := uuid.New()

	requestA := seedIAMPurgeRequest(t, pool, tenantA)
	requestB := seedIAMPurgeRequest(t, pool, tenantB)

	// --- ApproveTenantPurgeRequest: tenant A approving tenant B's open
	// request id must fail, and B's request must stay pending. ---
	_, err := repo.ApproveTenantPurgeRequest(ctx, tenantA, requestB, uuid.New())
	assert.ErrorIs(t, err, iam.ErrPurgeRequestNotFound, "cross-tenant approve must be refused, not silently resolved by request id alone")
	assert.Equal(t, "pending", readPurgeRequestStatus(t, pool, requestB), "a refused cross-tenant approve must not mutate the victim tenant's request")

	// --- CancelTenantPurgeRequest: same check, using tenant A's own request
	// as the victim this time to prove the isolation isn't accidentally
	// symmetric with the first case. ---
	_, err = repo.CancelTenantPurgeRequest(ctx, tenantB, requestA, uuid.New())
	assert.ErrorIs(t, err, iam.ErrPurgeRequestNotFound, "cross-tenant cancel must be refused")
	assert.Equal(t, "pending", readPurgeRequestStatus(t, pool, requestA), "a refused cross-tenant cancel must not mutate the victim tenant's request")

	// Positive control: same-tenant approve then cancel succeeds end to end,
	// proving the fix didn't just break the predicate for everyone.
	approved, err := repo.ApproveTenantPurgeRequest(ctx, tenantB, requestB, uuid.New())
	require.NoError(t, err)
	assert.Equal(t, "approved", approved.Status)
	cancelled, err := repo.CancelTenantPurgeRequest(ctx, tenantB, requestB, uuid.New())
	require.NoError(t, err)
	assert.Equal(t, "cancelled", cancelled.Status)
}

// seedIAMPurgeRequest inserts a pending tenant_purge_requests row for
// tenantID, registering cleanup. Returns the request id.
//
// tenant_purge_requests.tenant_id carries no FK (migrations/iam/019), so this
// does not require a corresponding tenants row.
func seedIAMPurgeRequest(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	requestID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO tenant_purge_requests (id, tenant_id, status, requested_by, request_reason, tenant_name, tenant_slug)
		 VALUES ($1, $2, 'pending', $3, 'tenant isolation test', 'Tenant Isolation Test Tenant', $4)`,
		requestID, tenantID, uuid.New(), "iso-test-"+requestID.String())
	require.NoError(t, err, "insert tenant_purge_request")

	t.Cleanup(func() {
		ctx := context.Background()
		_, err := pool.Exec(ctx, `DELETE FROM tenant_purge_requests WHERE id = $1`, requestID)
		assert.NoError(t, err, "cleanup: delete tenant_purge_requests")
	})

	return requestID
}

// readPurgeRequestStatus reads a purge request's status column directly,
// bypassing the repository, so the assertion is independent of the code
// under test.
func readPurgeRequestStatus(t *testing.T, pool *pgxpool.Pool, requestID uuid.UUID) string {
	t.Helper()
	ctx := context.Background()

	var status string
	err := pool.QueryRow(ctx, `SELECT status FROM tenant_purge_requests WHERE id = $1`, requestID).Scan(&status)
	require.NoError(t, err, "read tenant_purge_request status")
	return status
}

// seedIAMTenant inserts a minimal tenant row, registering cleanup. Returns
// the tenant id.
func seedIAMTenant(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	tenantID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO tenants (id, name, slug) VALUES ($1, 'Tenant Isolation Test Tenant', $2)`,
		tenantID, "iso-test-"+tenantID.String())
	require.NoError(t, err, "insert tenant")

	t.Cleanup(func() {
		ctx := context.Background()
		_, err := pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID)
		assert.NoError(t, err, "cleanup: delete tenants")
	})

	return tenantID
}

// seedIAMUser inserts a user row for tenantID with the given password hash,
// registering cleanup. Returns the user id.
func seedIAMUser(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID, passwordHash string) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	userID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, tenant_id, email, display_name, password_hash, status)
		 VALUES ($1, $2, $3, 'Tenant Isolation Test User', $4, 'active')`,
		userID, tenantID, "iso-test-"+userID.String()+"@example.com", passwordHash)
	require.NoError(t, err, "insert user")

	t.Cleanup(func() {
		ctx := context.Background()
		_, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
		assert.NoError(t, err, "cleanup: delete users")
	})

	return userID
}

// readPasswordHash reads a user's password_hash column directly, bypassing
// the repository, so the assertion is independent of the code under test.
func readPasswordHash(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) string {
	t.Helper()
	ctx := context.Background()

	var hash string
	err := pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&hash)
	require.NoError(t, err, "read password_hash")
	return hash
}
