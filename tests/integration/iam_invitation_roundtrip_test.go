//go:build integration

// First real-Postgres coverage of the invitation flow (#185). Before the fix,
// GetInvitationByToken/AcceptInvitation referenced a nonexistent accepted_at
// column and CreateInvitation dropped role_id and had no ON CONFLICT target, so
// this whole path errored against a live database and was only ever exercised
// against in-memory doubles. Reuses the harness in iam_tenant_isolation_test.go
// (seedIAMTenant, seedIAMUser, noopTokenIssuer).
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

func TestIAM_InvitationRoundTrip(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := iamdb.NewPostgres(pool)
	hasher := auth.NewArgon2idHasher()
	svc := iam.NewService(repo, nil, noopTokenIssuer{}, hasher, auth.NewDualVerifier())

	tenant := seedIAMTenant(t, pool)
	inviterHash, err := hasher.Hash("inviter-pass")
	require.NoError(t, err)
	inviter := seedIAMUser(t, pool, tenant, inviterHash)
	role := seedIAMRole(t, pool, tenant, "coordinator")

	inviteeEmail := "invitee-" + uuid.NewString() + "@example.com"

	// --- InviteUser: proves the CreateInvitation INSERT (incl. role_id) and the
	// ON CONFLICT target both work against real Postgres. ---
	inv := &iam.Invitation{TenantID: tenant, Email: inviteeEmail, RoleID: &role, InvitedBy: inviter}
	created, err := svc.InviteUser(ctx, inv)
	require.NoError(t, err, "InviteUser must succeed against real Postgres")
	require.NotEmpty(t, created.Token, "invitation must carry a token")
	firstToken := created.Token

	// --- GetInvitationByToken: proves the SELECT works and role_id persisted. ---
	loaded, err := repo.GetInvitationByToken(ctx, firstToken)
	require.NoError(t, err, "GetInvitationByToken must find the pending invitation")
	require.NotNil(t, loaded.RoleID, "role_id must have been persisted")
	assert.Equal(t, role, *loaded.RoleID, "persisted role must match the invited role")
	assert.Equal(t, "pending", loaded.Status)

	// --- AcceptInvitation: proves the UPDATE works and the invited role is
	// actually granted to the new user in one flow. ---
	_, err = svc.AcceptInvitation(ctx, firstToken, "invitee-chosen-password")
	require.NoError(t, err, "AcceptInvitation must succeed")

	newUserID := readUserIDByEmail(t, pool, tenant, inviteeEmail)
	require.NotEqual(t, uuid.Nil, newUserID, "the invitee's user row must exist")
	assert.True(t, userHasRole(t, pool, newUserID, role), "the invited role must be granted to the new user")
	assert.Equal(t, "accepted", readInvitationStatus(t, pool, created.ID), "the invitation must be marked accepted")

	// --- Re-invite upsert: proves ON CONFLICT (tenant_id,email) against the new
	// UNIQUE constraint — a second invite for the same address updates the one
	// row with a fresh token rather than erroring or inserting a duplicate. ---
	reinv := &iam.Invitation{TenantID: tenant, Email: inviteeEmail, RoleID: &role, InvitedBy: inviter}
	reCreated, err := svc.InviteUser(ctx, reinv)
	require.NoError(t, err, "re-invite must upsert, not error")
	assert.NotEqual(t, firstToken, reCreated.Token, "re-invite must refresh the token")
	assert.Equal(t, 1, countInvitations(t, pool, tenant, inviteeEmail), "re-invite must keep exactly one row")
}

// seedIAMRole inserts a role in tenantID and returns its id. Cleaned up by the
// tenant's ON DELETE CASCADE (roles.tenant_id), plus an explicit delete.
func seedIAMRole(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID, name string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	roleID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO roles (id, tenant_id, name, permissions) VALUES ($1, $2, $3, '{}')`,
		roleID, tenantID, name+"-"+roleID.String())
	require.NoError(t, err, "insert role")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM roles WHERE id = $1`, roleID)
	})
	return roleID
}

func readUserIDByEmail(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID, email string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE tenant_id = $1 AND email = $2`, tenantID, email).Scan(&id)
	if err != nil {
		return uuid.Nil
	}
	return id
}

func userHasRole(t *testing.T, pool *pgxpool.Pool, userID, roleID uuid.UUID) bool {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM user_roles WHERE user_id = $1 AND role_id = $2`, userID, roleID).Scan(&n))
	return n == 1
}

func readInvitationStatus(t *testing.T, pool *pgxpool.Pool, invID uuid.UUID) string {
	t.Helper()
	var status string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT status FROM invitations WHERE id = $1`, invID).Scan(&status))
	return status
}

func countInvitations(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID, email string) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM invitations WHERE tenant_id = $1 AND email = $2`, tenantID, email).Scan(&n))
	return n
}
