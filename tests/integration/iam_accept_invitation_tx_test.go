//go:build integration

// Real-Postgres proof that AcceptInvitation is transactional (#148): a role
// grant that fails inside the tx must leave NO user row and a still-pending
// invitation, and the same token must succeed once the invitation's role
// reference is repaired. Reuses the harness in iam_tenant_isolation_test.go /
// iam_invitation_roundtrip_test.go (seedIAMTenant, seedIAMUser, seedIAMRole,
// noopTokenIssuer).
package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wegofwd2020/thittam/pkg/auth"
	"github.com/wegofwd2020/thittam/pkg/testdb"
	"github.com/wegofwd2020/thittam/services/iam"
	iamdb "github.com/wegofwd2020/thittam/services/iam/db"
)

func TestIAM_AcceptInvitation_RollsBackOnFailedGrant(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	repo := iamdb.NewPostgres(pool)
	hasher := auth.NewArgon2idHasher()
	svc := iam.NewService(repo, nil, noopTokenIssuer{}, hasher, auth.NewDualVerifier())

	tenantA := seedIAMTenant(t, pool)
	tenantB := seedIAMTenant(t, pool)
	inviterHash, err := hasher.Hash("inviter-pass")
	require.NoError(t, err)
	inviter := seedIAMUser(t, pool, tenantA, inviterHash)
	crossTenantRole := seedIAMRole(t, pool, tenantB, "cross") // in tenant B, not A
	validRole := seedIAMRole(t, pool, tenantA, "valid")       // in tenant A

	email := "invitee-" + uuid.NewString() + "@example.com"
	token := "tok-" + uuid.NewString()
	invID := seedIAMInvitationTx(t, pool, tenantA, email, crossTenantRole, inviter, token)

	// --- Failed grant: GetRoleByID(tenantA, crossTenantRole) fails inside the
	// tx, so CreateUser rolls back. ---
	_, err = svc.AcceptInvitation(ctx, token, "chosen-password")
	require.Error(t, err, "cross-tenant role grant must fail the acceptance")

	assert.Equal(t, uuid.Nil, readUserIDByEmail(t, pool, tenantA, email),
		"a rolled-back accept must leave NO user row")
	assert.Equal(t, "pending", readInvitationStatus(t, pool, invID),
		"a rolled-back accept must leave the invitation pending")

	// --- Repair the invitation's role to a valid tenant-A role, then retry the
	// SAME token: it must now succeed end to end. ---
	_, err = pool.Exec(ctx, `UPDATE invitations SET role_id = $1 WHERE id = $2`, validRole, invID)
	require.NoError(t, err, "repair invitation role")

	_, err = svc.AcceptInvitation(ctx, token, "chosen-password")
	require.NoError(t, err, "retry with the repaired role must succeed")

	newUserID := readUserIDByEmail(t, pool, tenantA, email)
	require.NotEqual(t, uuid.Nil, newUserID, "the invitee's user row must exist after success")
	assert.True(t, userHasRole(t, pool, newUserID, validRole), "the repaired role must be granted")
	assert.Equal(t, "accepted", readInvitationStatus(t, pool, invID), "the invitation must be accepted")
}

// seedIAMInvitationTx inserts a pending invitation carrying roleID and returns
// its id. Registered cleanup deletes the invitation row FIRST (LIFO) so the
// inviter user's own cleanup does not hit the invited_by FK (NO ACTION).
func seedIAMInvitationTx(t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID, email string, roleID, invitedBy uuid.UUID, token string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	invID := uuid.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO invitations (id, tenant_id, email, invited_by, token, expires_at, role_id, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending')`,
		invID, tenantID, email, invitedBy, token, time.Now().UTC().Add(7*24*time.Hour), roleID)
	require.NoError(t, err, "insert invitation")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM invitations WHERE id = $1`, invID)
	})
	return invID
}
