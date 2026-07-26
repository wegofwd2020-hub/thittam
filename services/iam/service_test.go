package iam

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/audit"
	"github.com/wegofwd2020/thittam/pkg/auth"
	"github.com/wegofwd2020/thittam/pkg/crypto"
)

// oidcTestKey is a deterministic 32-byte AES-256 key for OIDC service tests.
var oidcTestKey = []byte("thittam-svc-test-key-32bytes-xxx")

// --- Fixed test IDs (deterministic, never random) ---

var (
	fixedTenantID = uuid.MustParse("a1000000-0000-0000-0000-000000000001")
	fixedUserID   = uuid.MustParse("b2000000-0000-0000-0000-000000000002")
	fixedRoleID   = uuid.MustParse("c3000000-0000-0000-0000-000000000003")
	fixedInviteID = uuid.MustParse("d4000000-0000-0000-0000-000000000004")
)

// --- Mock Repository ---

type mockRepo struct {
	getUserByEmailFn                  func(ctx context.Context, tenantID uuid.UUID, email string) (*auth.UserRecord, error)
	findTenantByEmailFn               func(ctx context.Context, email string) (uuid.UUID, error)
	getUserByIDFn                     func(ctx context.Context, tenantID, userID uuid.UUID) (*auth.UserRecord, error)
	createOIDCUserFn                  func(ctx context.Context, tenantID uuid.UUID, email, displayName string) (*auth.UserRecord, error)
	getTenantStatusFn                 func(ctx context.Context, tenantID uuid.UUID) (string, error)
	createUserFn                      func(ctx context.Context, user *User) error
	getUserFn                         func(ctx context.Context, tenantID, id uuid.UUID) (*User, error)
	listUsersFn                       func(ctx context.Context, tenantID uuid.UUID, status string, limit, offset int) ([]User, error)
	updateUserFn                      func(ctx context.Context, user *User) error
	updatePasswordHashFn              func(ctx context.Context, tenantID, userID uuid.UUID, hash string) error
	deactivateUserFn                  func(ctx context.Context, tenantID, id uuid.UUID) error
	activateUserFn                    func(ctx context.Context, tenantID, id uuid.UUID) error
	createTenantFn                    func(ctx context.Context, tenant *Tenant) error
	getTenantFn                       func(ctx context.Context, id uuid.UUID) (*Tenant, error)
	findTenantByNormalizedNameFn      func(ctx context.Context, name string) (*Tenant, error)
	updateTenantStatusFn              func(ctx context.Context, id uuid.UUID, status string, holdUntil *time.Time, freezeReason *string) error
	clearTenantLegalHoldFn            func(ctx context.Context, id uuid.UUID) (*Tenant, error)
	setTenantLegalHoldFn              func(ctx context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason string) (*Tenant, error)
	countTenantsOnHoldFn              func(ctx context.Context) (int64, error)
	updateTenantAddressFn             func(ctx context.Context, t *Tenant) (*Tenant, error)
	transitionTenantStatusFn          func(ctx context.Context, id uuid.UUID, from, to string) (*Tenant, bool, error)
	listTenantsDueForLifecycleFn      func(ctx context.Context, now time.Time, limit int) ([]*Tenant, error)
	createTenantPurgeRequestFn        func(ctx context.Context, req *TenantPurgeRequest) error
	getOpenTenantPurgeRequestFn       func(ctx context.Context, tenantID uuid.UUID) (*TenantPurgeRequest, error)
	approveTenantPurgeRequestFn       func(ctx context.Context, tenantID, requestID, approverID uuid.UUID) (*TenantPurgeRequest, error)
	cancelTenantPurgeRequestFn        func(ctx context.Context, tenantID, requestID, cancellerID uuid.UUID) (*TenantPurgeRequest, error)
	listApprovedTenantPurgeRequestsFn func(ctx context.Context, limit int) ([]*TenantPurgeRequest, error)
	markTenantPurgeRequestFailedFn    func(ctx context.Context, requestID uuid.UUID, reason string) (*TenantPurgeRequest, error)
	purgeTenantSchemaAndTombstoneFn   func(ctx context.Context, tenantID, requestID uuid.UUID) error
	createRoleFn                      func(ctx context.Context, role *Role) error
	getRoleFn                         func(ctx context.Context, tenantID uuid.UUID, name string) (*Role, error)
	getRoleByIDFn                     func(ctx context.Context, tenantID, roleID uuid.UUID) (*Role, error)
	listRolesFn                       func(ctx context.Context, tenantID uuid.UUID) ([]Role, error)
	assignRoleFn                      func(ctx context.Context, ur *UserRole) error
	revokeRoleFn                      func(ctx context.Context, userID, roleID uuid.UUID) error
	getUserPermissionsFn              func(ctx context.Context, userID uuid.UUID, projectID *uuid.UUID) ([]string, error)
	createInvitationFn                func(ctx context.Context, inv *Invitation) error
	getInvitationByTokenFn            func(ctx context.Context, token string) (*Invitation, error)
	markInvitationFn                  func(ctx context.Context, id uuid.UUID) error
	upsertOIDCConfigFn                func(ctx context.Context, params OIDCConfigParams) error
	createAuditEntryFn                func(ctx context.Context, entry *AuditEntry) error
}

func (m *mockRepo) GetUserByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*auth.UserRecord, error) {
	if m.getUserByEmailFn != nil {
		return m.getUserByEmailFn(ctx, tenantID, email)
	}
	return &auth.UserRecord{ID: fixedUserID, TenantID: tenantID, Email: email, Status: "active"}, nil
}
func (m *mockRepo) FindTenantByEmail(ctx context.Context, email string) (uuid.UUID, error) {
	if m.findTenantByEmailFn != nil {
		return m.findTenantByEmailFn(ctx, email)
	}
	return fixedTenantID, nil
}
func (m *mockRepo) GetUserByID(ctx context.Context, tenantID, userID uuid.UUID) (*auth.UserRecord, error) {
	if m.getUserByIDFn != nil {
		return m.getUserByIDFn(ctx, tenantID, userID)
	}
	return &auth.UserRecord{ID: userID, PasswordHash: "hashed"}, nil // TenantID left uuid.Nil deliberately
}
func (m *mockRepo) CreateOIDCUser(ctx context.Context, tenantID uuid.UUID, email, displayName string) (*auth.UserRecord, error) {
	if m.createOIDCUserFn != nil {
		return m.createOIDCUserFn(ctx, tenantID, email, displayName)
	}
	return &auth.UserRecord{ID: uuid.New(), TenantID: tenantID, Email: email}, nil
}
func (m *mockRepo) GetTenantStatus(ctx context.Context, tenantID uuid.UUID) (string, error) {
	if m.getTenantStatusFn != nil {
		return m.getTenantStatusFn(ctx, tenantID)
	}
	return "active", nil
}
func (m *mockRepo) CreateUser(ctx context.Context, user *User) error {
	if m.createUserFn != nil {
		return m.createUserFn(ctx, user)
	}
	return nil
}
func (m *mockRepo) GetUser(ctx context.Context, tenantID, id uuid.UUID) (*User, error) {
	if m.getUserFn != nil {
		return m.getUserFn(ctx, tenantID, id)
	}
	return &User{ID: id, TenantID: tenantID, Status: "active"}, nil
}
func (m *mockRepo) ListUsers(ctx context.Context, tenantID uuid.UUID, status string, limit, offset int) ([]User, error) {
	if m.listUsersFn != nil {
		return m.listUsersFn(ctx, tenantID, status, limit, offset)
	}
	return nil, nil
}
func (m *mockRepo) UpdateUser(ctx context.Context, user *User) error {
	if m.updateUserFn != nil {
		return m.updateUserFn(ctx, user)
	}
	return nil
}
func (m *mockRepo) UpdatePasswordHash(ctx context.Context, tenantID, userID uuid.UUID, hash string) error {
	if m.updatePasswordHashFn != nil {
		return m.updatePasswordHashFn(ctx, tenantID, userID, hash)
	}
	return nil
}
func (m *mockRepo) DeactivateUser(ctx context.Context, tenantID, id uuid.UUID) error {
	if m.deactivateUserFn != nil {
		return m.deactivateUserFn(ctx, tenantID, id)
	}
	return nil
}
func (m *mockRepo) ActivateUser(ctx context.Context, tenantID, id uuid.UUID) error {
	if m.activateUserFn != nil {
		return m.activateUserFn(ctx, tenantID, id)
	}
	return nil
}
func (m *mockRepo) CreateTenant(ctx context.Context, tenant *Tenant) error {
	if m.createTenantFn != nil {
		return m.createTenantFn(ctx, tenant)
	}
	return nil
}
func (m *mockRepo) GetTenant(ctx context.Context, id uuid.UUID) (*Tenant, error) {
	if m.getTenantFn != nil {
		return m.getTenantFn(ctx, id)
	}
	return &Tenant{ID: id, Status: "active", Plan: "starter"}, nil
}
func (m *mockRepo) FindTenantByNormalizedName(ctx context.Context, name string) (*Tenant, error) {
	if m.findTenantByNormalizedNameFn != nil {
		return m.findTenantByNormalizedNameFn(ctx, name)
	}
	return nil, nil
}
func (m *mockRepo) UpdateTenantStatus(ctx context.Context, id uuid.UUID, status string, holdUntil *time.Time, freezeReason *string) error {
	if m.updateTenantStatusFn != nil {
		return m.updateTenantStatusFn(ctx, id, status, holdUntil, freezeReason)
	}
	return nil
}
func (m *mockRepo) ClearTenantLegalHold(ctx context.Context, id uuid.UUID) (*Tenant, error) {
	if m.clearTenantLegalHoldFn != nil {
		return m.clearTenantLegalHoldFn(ctx, id)
	}
	return &Tenant{ID: id, Status: TenantStatusSuspended}, nil
}
func (m *mockRepo) SetTenantLegalHold(ctx context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason string) (*Tenant, error) {
	if m.setTenantLegalHoldFn != nil {
		return m.setTenantLegalHoldFn(ctx, id, holdUntil, freezeReason)
	}
	return &Tenant{ID: id, Status: TenantStatusSuspended, FreezeReason: &freezeReason, HoldUntil: holdUntil}, nil
}
func (m *mockRepo) CountTenantsOnHold(ctx context.Context) (int64, error) {
	if m.countTenantsOnHoldFn != nil {
		return m.countTenantsOnHoldFn(ctx)
	}
	return 0, nil
}
func (m *mockRepo) UpdateTenantAddress(ctx context.Context, t *Tenant) (*Tenant, error) {
	if m.updateTenantAddressFn != nil {
		return m.updateTenantAddressFn(ctx, t)
	}
	return t, nil
}
func (m *mockRepo) TransitionTenantStatus(ctx context.Context, id uuid.UUID, from, to string) (*Tenant, bool, error) {
	if m.transitionTenantStatusFn != nil {
		return m.transitionTenantStatusFn(ctx, id, from, to)
	}
	return &Tenant{ID: id, Status: to}, true, nil
}
func (m *mockRepo) ListTenantsDueForLifecycle(ctx context.Context, now time.Time, limit int) ([]*Tenant, error) {
	if m.listTenantsDueForLifecycleFn != nil {
		return m.listTenantsDueForLifecycleFn(ctx, now, limit)
	}
	return nil, nil
}
func (m *mockRepo) CreateTenantPurgeRequest(ctx context.Context, req *TenantPurgeRequest) error {
	if m.createTenantPurgeRequestFn != nil {
		return m.createTenantPurgeRequestFn(ctx, req)
	}
	return nil
}
func (m *mockRepo) GetOpenTenantPurgeRequest(ctx context.Context, tenantID uuid.UUID) (*TenantPurgeRequest, error) {
	if m.getOpenTenantPurgeRequestFn != nil {
		return m.getOpenTenantPurgeRequestFn(ctx, tenantID)
	}
	return nil, ErrPurgeRequestNotFound
}
func (m *mockRepo) ApproveTenantPurgeRequest(ctx context.Context, tenantID, requestID, approverID uuid.UUID) (*TenantPurgeRequest, error) {
	if m.approveTenantPurgeRequestFn != nil {
		return m.approveTenantPurgeRequestFn(ctx, tenantID, requestID, approverID)
	}
	return nil, ErrPurgeRequestNotFound
}
func (m *mockRepo) CancelTenantPurgeRequest(ctx context.Context, tenantID, requestID, cancellerID uuid.UUID) (*TenantPurgeRequest, error) {
	if m.cancelTenantPurgeRequestFn != nil {
		return m.cancelTenantPurgeRequestFn(ctx, tenantID, requestID, cancellerID)
	}
	return nil, ErrPurgeRequestNotFound
}
func (m *mockRepo) ListApprovedTenantPurgeRequests(ctx context.Context, limit int) ([]*TenantPurgeRequest, error) {
	if m.listApprovedTenantPurgeRequestsFn != nil {
		return m.listApprovedTenantPurgeRequestsFn(ctx, limit)
	}
	return nil, nil
}
func (m *mockRepo) MarkTenantPurgeRequestFailed(ctx context.Context, requestID uuid.UUID, reason string) (*TenantPurgeRequest, error) {
	if m.markTenantPurgeRequestFailedFn != nil {
		return m.markTenantPurgeRequestFailedFn(ctx, requestID, reason)
	}
	return nil, ErrPurgeRequestNotFound
}
func (m *mockRepo) PurgeTenantSchemaAndTombstone(ctx context.Context, tenantID, requestID uuid.UUID) error {
	if m.purgeTenantSchemaAndTombstoneFn != nil {
		return m.purgeTenantSchemaAndTombstoneFn(ctx, tenantID, requestID)
	}
	return nil
}
func (m *mockRepo) CreateRole(ctx context.Context, role *Role) error {
	if m.createRoleFn != nil {
		return m.createRoleFn(ctx, role)
	}
	return nil
}
func (m *mockRepo) GetRole(ctx context.Context, tenantID uuid.UUID, name string) (*Role, error) {
	if m.getRoleFn != nil {
		return m.getRoleFn(ctx, tenantID, name)
	}
	return &Role{ID: fixedRoleID, TenantID: tenantID, Name: name}, nil
}
func (m *mockRepo) GetRoleByID(ctx context.Context, tenantID, roleID uuid.UUID) (*Role, error) {
	if m.getRoleByIDFn != nil {
		return m.getRoleByIDFn(ctx, tenantID, roleID)
	}
	return &Role{ID: roleID, TenantID: tenantID, Name: "member", IsSystem: true}, nil
}
func (m *mockRepo) ListRoles(ctx context.Context, tenantID uuid.UUID) ([]Role, error) {
	if m.listRolesFn != nil {
		return m.listRolesFn(ctx, tenantID)
	}
	return nil, nil
}
func (m *mockRepo) AssignRole(ctx context.Context, ur *UserRole) error {
	if m.assignRoleFn != nil {
		return m.assignRoleFn(ctx, ur)
	}
	return nil
}
func (m *mockRepo) RevokeRole(ctx context.Context, userID, roleID uuid.UUID) error {
	if m.revokeRoleFn != nil {
		return m.revokeRoleFn(ctx, userID, roleID)
	}
	return nil
}
func (m *mockRepo) GetUserPermissions(ctx context.Context, userID uuid.UUID, projectID *uuid.UUID) ([]string, error) {
	if m.getUserPermissionsFn != nil {
		return m.getUserPermissionsFn(ctx, userID, projectID)
	}
	return nil, nil
}
func (m *mockRepo) CreateInvitation(ctx context.Context, inv *Invitation) error {
	if m.createInvitationFn != nil {
		return m.createInvitationFn(ctx, inv)
	}
	return nil
}
func (m *mockRepo) GetInvitationByToken(ctx context.Context, token string) (*Invitation, error) {
	if m.getInvitationByTokenFn != nil {
		return m.getInvitationByTokenFn(ctx, token)
	}
	return nil, ErrInvitationNotFound
}
func (m *mockRepo) MarkInvitationAccepted(ctx context.Context, id uuid.UUID) error {
	if m.markInvitationFn != nil {
		return m.markInvitationFn(ctx, id)
	}
	return nil
}
func (m *mockRepo) WithTx(ctx context.Context, fn func(Repository) error) error {
	return fn(m) // no real tx; atomicity is a Postgres property (see integration test)
}
func (m *mockRepo) UpsertOIDCConfig(ctx context.Context, params OIDCConfigParams) error {
	if m.upsertOIDCConfigFn != nil {
		return m.upsertOIDCConfigFn(ctx, params)
	}
	return nil
}
func (m *mockRepo) CreateAuditEntry(ctx context.Context, entry *AuditEntry) error {
	if m.createAuditEntryFn != nil {
		return m.createAuditEntryFn(ctx, entry)
	}
	return nil
}

// --- Mock Authenticator ---

type mockAuthenticator struct {
	authenticateFn func(ctx context.Context, req auth.AuthRequest) (*auth.AuthResult, error)
}

func (m *mockAuthenticator) Authenticate(ctx context.Context, req auth.AuthRequest) (*auth.AuthResult, error) {
	if m.authenticateFn != nil {
		return m.authenticateFn(ctx, req)
	}
	return &auth.AuthResult{
		UserID:   fixedUserID,
		TenantID: req.TenantID,
		Email:    req.Email,
	}, nil
}

// --- Mock TokenIssuer ---

type mockTokenIssuer struct {
	issueFn              func(ctx context.Context, result *auth.AuthResult) (*auth.TokenPair, error)
	refreshFn            func(ctx context.Context, refreshToken string) (*auth.TokenPair, error)
	revokeFn             func(ctx context.Context, refreshToken string) error
	validateFn           func(ctx context.Context, accessToken string) (*auth.Claims, error)
	revokeAllForUserFn   func(ctx context.Context, userID uuid.UUID) error
	revokeAllForTenantFn func(ctx context.Context, tenantID uuid.UUID) error
}

func (m *mockTokenIssuer) Issue(ctx context.Context, result *auth.AuthResult) (*auth.TokenPair, error) {
	if m.issueFn != nil {
		return m.issueFn(ctx, result)
	}
	return &auth.TokenPair{AccessToken: "access", RefreshToken: "refresh", TokenType: "Bearer"}, nil
}
func (m *mockTokenIssuer) Refresh(ctx context.Context, refreshToken string) (*auth.TokenPair, error) {
	if m.refreshFn != nil {
		return m.refreshFn(ctx, refreshToken)
	}
	return &auth.TokenPair{AccessToken: "new-access", RefreshToken: "new-refresh", TokenType: "Bearer"}, nil
}
func (m *mockTokenIssuer) Revoke(ctx context.Context, refreshToken string) error {
	if m.revokeFn != nil {
		return m.revokeFn(ctx, refreshToken)
	}
	return nil
}
func (m *mockTokenIssuer) Validate(ctx context.Context, accessToken string) (*auth.Claims, error) {
	if m.validateFn != nil {
		return m.validateFn(ctx, accessToken)
	}
	return &auth.Claims{Subject: fixedUserID}, nil
}
func (m *mockTokenIssuer) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	if m.revokeAllForUserFn != nil {
		return m.revokeAllForUserFn(ctx, userID)
	}
	return nil
}
func (m *mockTokenIssuer) RevokeAllForTenant(ctx context.Context, tenantID uuid.UUID) error {
	if m.revokeAllForTenantFn != nil {
		return m.revokeAllForTenantFn(ctx, tenantID)
	}
	return nil
}

// --- Mock PasswordHasher & Verifier ---

type mockHasher struct {
	hashFn func(password string) (string, error)
}

func (m *mockHasher) Hash(password string) (string, error) {
	if m.hashFn != nil {
		return m.hashFn(password)
	}
	return "hashed:" + password, nil
}

type mockVerifier struct {
	verifyFn func(password, hash string) error
}

func (m *mockVerifier) Verify(password, hash string) error {
	if m.verifyFn != nil {
		return m.verifyFn(password, hash)
	}
	// Default: accept "hashed:<password>" pairs
	if hash == "hashed:"+password {
		return nil
	}
	return auth.ErrInvalidCredentials
}

// mockSchemaMigrator implements SchemaMigrator for unit tests.
type mockSchemaMigrator struct {
	migrateFn func(ctx context.Context, tenantID uuid.UUID) error
}

func (m *mockSchemaMigrator) MigrateTenantSchema(ctx context.Context, tenantID uuid.UUID) error {
	if m.migrateFn != nil {
		return m.migrateFn(ctx, tenantID)
	}
	return nil
}

// --- Test helpers ---

func newTestService(repo Repository) *Service {
	return NewService(repo, &mockAuthenticator{}, &mockTokenIssuer{}, &mockHasher{}, &mockVerifier{})
}

// --- Tests ---

func TestLogin_Success(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{})

	pair, err := svc.Login(context.Background(), fixedTenantID, "user@example.com", "pass")
	require.NoError(t, err)
	assert.Equal(t, "access", pair.AccessToken)
	assert.Equal(t, "Bearer", pair.TokenType)
}

func TestLogin_TenantResolvedFromEmail(t *testing.T) {
	t.Parallel()
	resolvedTenant := uuid.MustParse("d0000000-0000-0000-0000-0000000000aa")
	var seenTenant uuid.UUID
	svc := NewService(
		&mockRepo{
			findTenantByEmailFn: func(_ context.Context, _ string) (uuid.UUID, error) {
				return resolvedTenant, nil
			},
		},
		&mockAuthenticator{
			authenticateFn: func(_ context.Context, req auth.AuthRequest) (*auth.AuthResult, error) {
				seenTenant = req.TenantID
				return &auth.AuthResult{UserID: fixedUserID, TenantID: req.TenantID, Email: req.Email, AuthenticatedAt: time.Now()}, nil
			},
		},
		&mockTokenIssuer{},
		&mockHasher{},
		&mockVerifier{},
	)

	_, err := svc.Login(context.Background(), uuid.Nil, "lookup@example.com", "pw")
	require.NoError(t, err)
	assert.Equal(t, resolvedTenant, seenTenant, "Authenticate must receive the resolved tenant")
}

func TestLogin_AmbiguousEmail_ReturnsError(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		findTenantByEmailFn: func(_ context.Context, _ string) (uuid.UUID, error) {
			return uuid.Nil, ErrAmbiguousEmail
		},
	})

	_, err := svc.Login(context.Background(), uuid.Nil, "shared@example.com", "pw")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAmbiguousEmail)
}

func TestLogin_AuthError_Propagates(t *testing.T) {
	t.Parallel()
	svc := NewService(
		&mockRepo{},
		&mockAuthenticator{
			authenticateFn: func(_ context.Context, _ auth.AuthRequest) (*auth.AuthResult, error) {
				return nil, auth.ErrInvalidCredentials
			},
		},
		&mockTokenIssuer{},
		&mockHasher{},
		&mockVerifier{},
	)

	_, err := svc.Login(context.Background(), fixedTenantID, "bad@example.com", "wrong")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrInvalidCredentials)
}

func TestRefreshToken_Delegates(t *testing.T) {
	t.Parallel()
	called := false
	svc := NewService(
		&mockRepo{},
		&mockAuthenticator{},
		&mockTokenIssuer{
			refreshFn: func(_ context.Context, tok string) (*auth.TokenPair, error) {
				called = true
				assert.Equal(t, "old-refresh", tok)
				return &auth.TokenPair{AccessToken: "new"}, nil
			},
		},
		&mockHasher{},
		&mockVerifier{},
	)

	_, err := svc.RefreshToken(context.Background(), "old-refresh")
	require.NoError(t, err)
	assert.True(t, called)
}

func TestLogout_RevokesToken(t *testing.T) {
	t.Parallel()
	var revokedToken string
	svc := NewService(
		&mockRepo{},
		&mockAuthenticator{},
		&mockTokenIssuer{
			revokeFn: func(_ context.Context, tok string) error {
				revokedToken = tok
				return nil
			},
		},
		&mockHasher{},
		&mockVerifier{},
	)

	err := svc.Logout(context.Background(), "my-refresh")
	require.NoError(t, err)
	assert.Equal(t, "my-refresh", revokedToken)
}

func TestCreateUser_GeneratesIDAndHashesPassword(t *testing.T) {
	t.Parallel()
	var saved *User
	svc := newTestService(&mockRepo{
		createUserFn: func(_ context.Context, u *User) error {
			saved = u
			return nil
		},
	})

	user := &User{TenantID: fixedTenantID, Email: "new@example.com", DisplayName: "New User"}
	result, err := svc.CreateUser(context.Background(), user, "plainpass")
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, result.ID)
	assert.Equal(t, "hashed:plainpass", saved.PasswordHash)
	assert.Equal(t, "active", result.Status)
}

func TestListUsers_DefaultLimit(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	svc := newTestService(&mockRepo{
		listUsersFn: func(_ context.Context, _ uuid.UUID, _ string, limit, _ int) ([]User, error) {
			capturedLimit = limit
			return nil, nil
		},
	})

	_, err := svc.ListUsers(context.Background(), fixedTenantID, "", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 20, capturedLimit)
}

func TestListUsers_MaxLimitEnforced(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	svc := newTestService(&mockRepo{
		listUsersFn: func(_ context.Context, _ uuid.UUID, _ string, limit, _ int) ([]User, error) {
			capturedLimit = limit
			return nil, nil
		},
	})

	_, err := svc.ListUsers(context.Background(), fixedTenantID, "", 9999, 0)
	require.NoError(t, err)
	assert.Equal(t, 20, capturedLimit)
}

func TestChangePassword_WrongOldPassword(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getUserByIDFn: func(_ context.Context, _, _ uuid.UUID) (*auth.UserRecord, error) {
			return &auth.UserRecord{PasswordHash: "hashed:correct"}, nil
		},
	})

	err := svc.ChangePassword(context.Background(), fixedTenantID, fixedUserID, "wrong", "newpass")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrInvalidCredentials)
}

func TestChangePassword_Success(t *testing.T) {
	t.Parallel()
	var updatedHash string
	svc := newTestService(&mockRepo{
		getUserByIDFn: func(_ context.Context, _, _ uuid.UUID) (*auth.UserRecord, error) {
			return &auth.UserRecord{PasswordHash: "hashed:oldpass"}, nil
		},
		updatePasswordHashFn: func(_ context.Context, _, _ uuid.UUID, hash string) error {
			updatedHash = hash
			return nil
		},
	})

	err := svc.ChangePassword(context.Background(), fixedTenantID, fixedUserID, "oldpass", "newpass")
	require.NoError(t, err)
	assert.Equal(t, "hashed:newpass", updatedHash)
}

func TestChangePassword_RevokesAllSessions(t *testing.T) {
	t.Parallel()
	var revokedFor uuid.UUID
	tokens := &mockTokenIssuer{
		revokeAllForUserFn: func(_ context.Context, userID uuid.UUID) error {
			revokedFor = userID
			return nil
		},
	}
	repo := &mockRepo{
		getUserByIDFn: func(_ context.Context, _, _ uuid.UUID) (*auth.UserRecord, error) {
			return &auth.UserRecord{PasswordHash: "hashed:oldpass"}, nil
		},
		updatePasswordHashFn: func(_ context.Context, _, _ uuid.UUID, _ string) error {
			return nil
		},
	}
	svc := NewService(repo, &mockAuthenticator{}, tokens, &mockHasher{}, &mockVerifier{})

	require.NoError(t, svc.ChangePassword(context.Background(), fixedTenantID, fixedUserID, "oldpass", "newpass"))
	assert.Equal(t, fixedUserID, revokedFor, "changing a password must revoke every session")
}

func TestChangePassword_RevokeFailure_IsReported(t *testing.T) {
	t.Parallel()
	tokens := &mockTokenIssuer{
		revokeAllForUserFn: func(context.Context, uuid.UUID) error { return errors.New("redis down") },
	}
	repo := &mockRepo{
		getUserByIDFn: func(_ context.Context, _, _ uuid.UUID) (*auth.UserRecord, error) {
			return &auth.UserRecord{PasswordHash: "hashed:oldpass"}, nil
		},
		updatePasswordHashFn: func(_ context.Context, _, _ uuid.UUID, _ string) error {
			return nil
		},
	}
	svc := NewService(repo, &mockAuthenticator{}, tokens, &mockHasher{}, &mockVerifier{})

	err := svc.ChangePassword(context.Background(), fixedTenantID, fixedUserID, "oldpass", "newpass")
	require.Error(t, err, "a failed revocation must not be silently swallowed")
}

func TestCheckPermission_Found(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getUserPermissionsFn: func(_ context.Context, _ uuid.UUID, _ *uuid.UUID) ([]string, error) {
			return []string{"production:read", "budget:write"}, nil
		},
	})

	ok, err := svc.CheckPermission(context.Background(), fixedUserID, "budget:write", nil)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestCheckPermission_NotFound(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getUserPermissionsFn: func(_ context.Context, _ uuid.UUID, _ *uuid.UUID) ([]string, error) {
			return []string{"production:read"}, nil
		},
	})

	ok, err := svc.CheckPermission(context.Background(), fixedUserID, "budget:approve", nil)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestCheckPermission_ProjectScopedSeparation(t *testing.T) {
	t.Parallel()
	projectA := uuid.MustParse("aa000000-0000-0000-0000-000000000001")
	projectB := uuid.MustParse("bb000000-0000-0000-0000-000000000002")

	// Repository fixture: user has tenant-wide "production:read" via member,
	// plus "expense:approve" project-scoped to projectA via project_supervisor.
	svc := newTestService(&mockRepo{
		getUserPermissionsFn: func(_ context.Context, _ uuid.UUID, projectID *uuid.UUID) ([]string, error) {
			perms := []string{"production:read"} // tenant-wide member
			if projectID != nil && *projectID == projectA {
				perms = append(perms, "expense:approve") // project-scoped on A only
			}
			return perms, nil
		},
	})

	// Tenant-wide check sees only the tenant-wide grant.
	okTenant, err := svc.CheckPermission(context.Background(), fixedUserID, "expense:approve", nil)
	require.NoError(t, err)
	assert.False(t, okTenant)

	// Project A: project-scoped grant is visible.
	okA, err := svc.CheckPermission(context.Background(), fixedUserID, "expense:approve", &projectA)
	require.NoError(t, err)
	assert.True(t, okA)

	// Project B: same user, different project — must not leak.
	okB, err := svc.CheckPermission(context.Background(), fixedUserID, "expense:approve", &projectB)
	require.NoError(t, err)
	assert.False(t, okB)

	// Tenant-wide perm still resolves under either scope.
	okMerge, err := svc.CheckPermission(context.Background(), fixedUserID, "production:read", &projectB)
	require.NoError(t, err)
	assert.True(t, okMerge)
}

func TestAssignProjectRole_RejectsTenantWideRole(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getRoleByIDFn: func(_ context.Context, tenantID, roleID uuid.UUID) (*Role, error) {
			return &Role{ID: roleID, TenantID: tenantID, Name: "manager", IsSystem: true}, nil
		},
	})
	err := svc.AssignProjectRole(context.Background(),
		fixedTenantID, fixedUserID, fixedRoleID, uuid.New(), fixedUserID)
	require.ErrorIs(t, err, ErrRoleNotProjectScoped)
}

func TestAssignProjectRole_AcceptsProjectSupervisor(t *testing.T) {
	t.Parallel()
	var captured *UserRole
	svc := newTestService(&mockRepo{
		getRoleByIDFn: func(_ context.Context, tenantID, roleID uuid.UUID) (*Role, error) {
			return &Role{ID: roleID, TenantID: tenantID, Name: "project_supervisor", IsSystem: true}, nil
		},
		assignRoleFn: func(_ context.Context, ur *UserRole) error {
			captured = ur
			return nil
		},
	})
	projectID := uuid.New()
	err := svc.AssignProjectRole(context.Background(),
		fixedTenantID, fixedUserID, fixedRoleID, projectID, fixedUserID)
	require.NoError(t, err)
	require.NotNil(t, captured)
	require.NotNil(t, captured.ProjectID)
	assert.Equal(t, projectID, *captured.ProjectID)
}

func TestAssignProjectRole_AllowsCustomRole(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getRoleByIDFn: func(_ context.Context, tenantID, roleID uuid.UUID) (*Role, error) {
			// Custom (non-system) roles bypass the project-scope guard.
			return &Role{ID: roleID, TenantID: tenantID, Name: "auditor", IsSystem: false}, nil
		},
	})
	err := svc.AssignProjectRole(context.Background(),
		fixedTenantID, fixedUserID, fixedRoleID, uuid.New(), fixedUserID)
	require.NoError(t, err)
}

func TestAssignProjectRole_ForeignUser_Denied(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getRoleByIDFn: func(_ context.Context, tenantID, roleID uuid.UUID) (*Role, error) {
			return &Role{ID: roleID, TenantID: tenantID, Name: "project_supervisor", IsSystem: true}, nil
		},
		getUserFn: func(context.Context, uuid.UUID, uuid.UUID) (*User, error) {
			return nil, ErrUserNotFound // the target user belongs to another tenant
		},
		assignRoleFn: func(context.Context, *UserRole) error {
			t.Fatal("repository must not be written for a foreign target user")
			return nil
		},
	})
	err := svc.AssignProjectRole(context.Background(),
		fixedTenantID, uuid.New(), fixedRoleID, uuid.New(), fixedUserID)
	require.ErrorIs(t, err, ErrUserNotFound)
}

func TestCreateTenant_RequiresCountry(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{})
	_, err := svc.CreateTenant(context.Background(), &Tenant{Name: "Acme", Plan: "starter"})
	require.ErrorIs(t, err, ErrCountryRequired)
}

func TestCreateTenant_RejectsUnknownCountry(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{})
	_, err := svc.CreateTenant(context.Background(), &Tenant{
		Name: "Acme", Plan: "starter", CountryCode: "ZZ",
	})
	require.ErrorIs(t, err, ErrUnknownCountry)
}

func TestCreateTenant_DerivesCurrencyFromCountry(t *testing.T) {
	t.Parallel()
	cases := []struct {
		country, wantCurrency string
	}{
		{"IN", "INR"}, {"US", "USD"}, {"DE", "EUR"}, {"GB", "GBP"}, {"JP", "JPY"},
	}
	for _, tc := range cases {
		svc := newTestService(&mockRepo{})
		got, err := svc.CreateTenant(context.Background(), &Tenant{
			Name: "Acme " + tc.country, Plan: "starter", CountryCode: tc.country,
		})
		require.NoError(t, err)
		assert.Equal(t, tc.wantCurrency, got.PrimaryCurrencyCode, "country %s", tc.country)
	}
}

func TestCreateTenant_RespectsExplicitCurrencyOverride(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{})
	got, err := svc.CreateTenant(context.Background(), &Tenant{
		Name: "Acme PR", Plan: "starter",
		CountryCode:         "US",  // US maps to USD
		PrimaryCurrencyCode: "eur", // caller override, case-normalised
	})
	require.NoError(t, err)
	assert.Equal(t, "EUR", got.PrimaryCurrencyCode)
}

func TestCreateTenant_InvalidPlan(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{})

	_, err := svc.CreateTenant(context.Background(), &Tenant{Name: "Acme", Plan: "galaxy", CountryCode: "IN"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidPlan)
}

func TestCreateTenant_SeedsSystemRoles(t *testing.T) {
	t.Parallel()
	var seededRoles []string
	svc := newTestService(&mockRepo{
		createRoleFn: func(_ context.Context, r *Role) error {
			seededRoles = append(seededRoles, r.Name)
			return nil
		},
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: "active"}, nil
		},
	})

	_, err := svc.CreateTenant(context.Background(), &Tenant{Name: "Acme", Plan: "starter", CountryCode: "IN"})
	require.NoError(t, err)
	assert.Len(t, seededRoles, len(systemRoles))
	assert.Contains(t, seededRoles, "super_admin")
	assert.Contains(t, seededRoles, "member")
	assert.Contains(t, seededRoles, "inventory_manager")
	assert.Contains(t, seededRoles, "project_supervisor")
}

func TestSystemRoles_InventoryManagerPermissions(t *testing.T) {
	t.Parallel()
	var inv struct{ perms []string }
	for _, r := range systemRoles {
		if r.name == "inventory_manager" {
			inv.perms = r.permissions
		}
	}
	assert.ElementsMatch(t, []string{
		"inventory:read", "inventory:write", "inventory:checkout", "inventory:retire",
		"document:read",
	}, inv.perms)
}

func TestSystemRoles_ProjectSupervisorPermissions(t *testing.T) {
	t.Parallel()
	var ps []string
	for _, r := range systemRoles {
		if r.name == "project_supervisor" {
			ps = r.permissions
		}
	}
	assert.ElementsMatch(t, []string{
		"production:read",
		"budget:read",
		"expense:read", "expense:submit", "expense:approve",
		"resource:manage",
		"inventory:read", "inventory:checkout",
		"document:read", "document:write",
	}, ps)
}

func TestCheckPermission_InventoryRetire_OnlyInventoryManager(t *testing.T) {
	t.Parallel()
	// inventory_manager holds inventory:retire; no other system role does.
	for _, r := range systemRoles {
		hasRetire := false
		for _, p := range r.permissions {
			if p == "inventory:retire" {
				hasRetire = true
				break
			}
		}
		if r.name == "inventory_manager" {
			assert.True(t, hasRetire, "inventory_manager must have inventory:retire")
		} else {
			assert.False(t, hasRetire, "%s must not have inventory:retire", r.name)
		}
	}
}

func TestCreateTenant_GeneratesSlugFromName(t *testing.T) {
	t.Parallel()
	var savedTenant *Tenant
	svc := newTestService(&mockRepo{
		createTenantFn: func(_ context.Context, t *Tenant) error {
			savedTenant = t
			return nil
		},
	})

	_, err := svc.CreateTenant(context.Background(), &Tenant{Name: "Acme Software Pvt. Ltd.", Plan: "professional", CountryCode: "IN"})
	require.NoError(t, err)
	assert.Equal(t, "acme-software-pvt-ltd", savedTenant.Slug)
}

func TestCreateTenant_PreflightDuplicate_NamesUUID(t *testing.T) {
	existing := uuid.New()
	svc := newTestService(&mockRepo{
		findTenantByNormalizedNameFn: func(_ context.Context, _ string) (*Tenant, error) {
			return &Tenant{ID: existing, Name: "Acme Corp"}, nil
		},
	})

	_, err := svc.CreateTenant(context.Background(), &Tenant{
		Name: "  acme   corp ", CountryCode: "US",
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTenantNameTaken), "must wrap ErrTenantNameTaken")
	assert.Contains(t, err.Error(), existing.String(), "message must name the colliding UUID")
}

func TestCreateTenant_NormalizesName(t *testing.T) {
	var stored string
	svc := newTestService(&mockRepo{
		createTenantFn: func(_ context.Context, t *Tenant) error {
			stored = t.Name
			return nil
		},
	})

	_, err := svc.CreateTenant(context.Background(), &Tenant{
		Name: "  Acme\t Corp   Studios  ", CountryCode: "US",
	})
	require.NoError(t, err)
	assert.Equal(t, "Acme Corp Studios", stored)
}

func TestSuspendTenant_UpdatesStatus(t *testing.T) {
	t.Parallel()
	var updatedStatus string
	var gotHoldUntil *time.Time
	var gotFreezeReason *string
	svc := newTestService(&mockRepo{
		updateTenantStatusFn: func(_ context.Context, _ uuid.UUID, status string, holdUntil *time.Time, freezeReason *string) error {
			updatedStatus = status
			gotHoldUntil = holdUntil
			gotFreezeReason = freezeReason
			return nil
		},
	})

	_, err := svc.SuspendTenant(context.Background(), fixedTenantID, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "suspended", updatedStatus)
	// Bare SuspendTenant (no hold params) plumbs nil pointers — the SQL
	// COALESCE layer then preserves any existing hold on the row.
	assert.Nil(t, gotHoldUntil)
	assert.Nil(t, gotFreezeReason)
}

func TestSuspendTenant_LegalHold_PlumbsFieldsToRepo(t *testing.T) {
	t.Parallel()
	holdUntil := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	reason := "SEC litigation 2026-CV-123"

	var gotHoldUntil *time.Time
	var gotFreezeReason *string
	svc := newTestService(&mockRepo{
		updateTenantStatusFn: func(_ context.Context, _ uuid.UUID, _ string, holdUntil *time.Time, freezeReason *string) error {
			gotHoldUntil = holdUntil
			gotFreezeReason = freezeReason
			return nil
		},
	})

	_, err := svc.SuspendTenant(context.Background(), fixedTenantID, &holdUntil, &reason)
	require.NoError(t, err)
	require.NotNil(t, gotHoldUntil)
	assert.Equal(t, holdUntil, *gotHoldUntil)
	require.NotNil(t, gotFreezeReason)
	assert.Equal(t, reason, *gotFreezeReason)
}

func TestSuspendTenant_IndefiniteHold_NilHoldUntilReachesRepo(t *testing.T) {
	t.Parallel()
	// Indefinite hold: reason set, hold_until nil. The repo must receive
	// a nil holdUntil so the COALESCE keeps whatever NULL column value is
	// already there (i.e. stays NULL — pinning the tenant indefinitely).
	reason := "GDPR discovery — indefinite"

	var gotHoldUntil *time.Time
	var gotFreezeReason *string
	svc := newTestService(&mockRepo{
		updateTenantStatusFn: func(_ context.Context, _ uuid.UUID, _ string, holdUntil *time.Time, freezeReason *string) error {
			gotHoldUntil = holdUntil
			gotFreezeReason = freezeReason
			return nil
		},
	})

	_, err := svc.SuspendTenant(context.Background(), fixedTenantID, nil, &reason)
	require.NoError(t, err)
	assert.Nil(t, gotHoldUntil, "indefinite hold must plumb nil holdUntil")
	require.NotNil(t, gotFreezeReason)
	assert.Equal(t, reason, *gotFreezeReason)
}

func TestSuspendTenant_EmitsLegalHoldAuditEvent(t *testing.T) {
	// Not t.Parallel() — waits on audit flush.
	holdUntil := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	reason := "regulatory freeze"

	after := &Tenant{
		ID:           fixedTenantID,
		Name:         "Acme",
		Status:       TenantStatusSuspended,
		HoldUntil:    &holdUntil,
		FreezeReason: &reason,
	}
	before := &Tenant{ID: fixedTenantID, Name: "Acme", Status: TenantStatusActive}

	getCall := 0
	repo := &mockRepo{
		getTenantFn: func(_ context.Context, _ uuid.UUID) (*Tenant, error) {
			getCall++
			if getCall == 1 {
				return before, nil
			}
			return after, nil
		},
		updateTenantStatusFn: func(_ context.Context, _ uuid.UUID, _ string, _ *time.Time, _ *string) error {
			return nil
		},
	}

	store := &memoryAuditStore{}
	logger := audit.NewLogger(store, audit.LoggerConfig{
		BufferSize:    10,
		FlushInterval: 10 * time.Millisecond,
		BatchSize:     10,
	}, nil)
	svc := newTestService(repo).WithAuditLogger(logger)

	actorID := uuid.MustParse("a1000000-0000-0000-0000-000000000092")
	ctx := audit.WithActor(context.Background(), audit.ActorInfo{
		UserID: actorID,
		Email:  "admin@xyzcba.com",
		IP:     "10.0.0.1",
	})

	_, err := svc.SuspendTenant(ctx, fixedTenantID, &holdUntil, &reason)
	require.NoError(t, err)

	flushCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, logger.Close(flushCtx))

	events := store.snapshot()
	require.Len(t, events, 1)
	e := events[0]
	assert.Equal(t, fixedTenantID, e.TenantID)
	assert.Equal(t, actorID, e.ActorID, "actor identity from context")
	assert.Equal(t, "admin@xyzcba.com", e.ActorEmail)
	assert.Equal(t, "10.0.0.1", e.ActorIP)
	assert.Equal(t, audit.ActionLegalHoldApplied, e.Action)
	assert.Equal(t, audit.ResourceTenant, e.ResourceType)
	assert.JSONEq(t, `{"status":"active"}`, string(e.OldState))
	assert.JSONEq(t,
		`{"status":"suspended","hold_until":"2026-06-01T00:00:00Z","freeze_reason":"regulatory freeze"}`,
		string(e.NewState),
	)
}

func TestSuspendTenant_BareSuspend_EmitsNoLegalHoldAuditEvent(t *testing.T) {
	// A bare SuspendTenant (no hold params) must NOT emit the
	// legal-hold audit event. Vanilla-suspend audit is covered by the
	// generic RPC interceptor; we do not double-log here.
	repo := &mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: TenantStatusSuspended}, nil
		},
		updateTenantStatusFn: func(_ context.Context, _ uuid.UUID, _ string, _ *time.Time, _ *string) error {
			return nil
		},
	}

	store := &memoryAuditStore{}
	logger := audit.NewLogger(store, audit.LoggerConfig{
		BufferSize:    10,
		FlushInterval: 10 * time.Millisecond,
		BatchSize:     10,
	}, nil)
	svc := newTestService(repo).WithAuditLogger(logger)

	_, err := svc.SuspendTenant(context.Background(), fixedTenantID, nil, nil)
	require.NoError(t, err)

	flushCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	require.NoError(t, logger.Close(flushCtx))

	assert.Empty(t, store.snapshot(), "bare suspend must not emit legal-hold audit event")
}

func TestSuspendTenant_RevokesAllSessions(t *testing.T) {
	t.Parallel()
	var revokedFor uuid.UUID
	called := false
	tokens := &mockTokenIssuer{revokeAllForTenantFn: func(_ context.Context, tid uuid.UUID) error {
		called = true
		revokedFor = tid
		return nil
	}}
	repo := &mockRepo{
		updateTenantStatusFn: func(_ context.Context, _ uuid.UUID, _ string, _ *time.Time, _ *string) error { return nil },
		getTenantFn:          func(_ context.Context, id uuid.UUID) (*Tenant, error) { return &Tenant{ID: id}, nil },
	}
	svc := NewService(repo, &mockAuthenticator{}, tokens, &mockHasher{}, &mockVerifier{})
	_, err := svc.SuspendTenant(context.Background(), fixedTenantID, nil, nil)
	require.NoError(t, err)
	assert.True(t, called, "suspending a tenant must revoke every session in it")
	assert.Equal(t, fixedTenantID, revokedFor)
}

func TestSuspendTenant_RevokeFailure_IsReported(t *testing.T) {
	t.Parallel()
	tokens := &mockTokenIssuer{revokeAllForTenantFn: func(_ context.Context, _ uuid.UUID) error {
		return errors.New("redis down")
	}}
	repo := &mockRepo{
		updateTenantStatusFn: func(_ context.Context, _ uuid.UUID, _ string, _ *time.Time, _ *string) error { return nil },
		getTenantFn:          func(_ context.Context, id uuid.UUID) (*Tenant, error) { return &Tenant{ID: id}, nil },
	}
	svc := NewService(repo, &mockAuthenticator{}, tokens, &mockHasher{}, &mockVerifier{})
	_, err := svc.SuspendTenant(context.Background(), fixedTenantID, nil, nil)
	require.Error(t, err)
}

func TestClearTenantLegalHold_ClearsFieldsAndEmitsAudit(t *testing.T) {
	// Not t.Parallel() — waits on audit flush.
	holdUntil := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	reason := "regulatory freeze"

	before := &Tenant{
		ID:           fixedTenantID,
		Name:         "Acme",
		Status:       TenantStatusSuspended,
		HoldUntil:    &holdUntil,
		FreezeReason: &reason,
	}
	after := &Tenant{ID: fixedTenantID, Name: "Acme", Status: TenantStatusSuspended}

	repo := &mockRepo{
		getTenantFn: func(_ context.Context, _ uuid.UUID) (*Tenant, error) {
			return before, nil
		},
		clearTenantLegalHoldFn: func(_ context.Context, _ uuid.UUID) (*Tenant, error) {
			return after, nil
		},
	}

	store := &memoryAuditStore{}
	logger := audit.NewLogger(store, audit.LoggerConfig{
		BufferSize:    10,
		FlushInterval: 10 * time.Millisecond,
		BatchSize:     10,
	}, nil)
	svc := newTestService(repo).WithAuditLogger(logger)

	actorID := uuid.MustParse("a2000000-0000-0000-0000-000000000092")
	ctx := audit.WithActor(context.Background(), audit.ActorInfo{
		UserID: actorID,
		Email:  "admin@xyzcba.com",
		IP:     "10.0.0.2",
	})

	clearReason := "court order 2026-CV-456"
	got, err := svc.ClearTenantLegalHold(ctx, fixedTenantID, clearReason)
	require.NoError(t, err)
	assert.Nil(t, got.HoldUntil, "returned tenant should have cleared hold_until")
	assert.Nil(t, got.FreezeReason, "returned tenant should have cleared freeze_reason")

	flushCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, logger.Close(flushCtx))

	events := store.snapshot()
	require.Len(t, events, 1)
	e := events[0]
	assert.Equal(t, fixedTenantID, e.TenantID)
	assert.Equal(t, actorID, e.ActorID)
	assert.Equal(t, "admin@xyzcba.com", e.ActorEmail)
	assert.Equal(t, audit.ActionLegalHoldCleared, e.Action)
	assert.Equal(t, audit.ResourceTenant, e.ResourceType)
	assert.JSONEq(t,
		`{"status":"suspended","hold_until":"2026-06-01T00:00:00Z","freeze_reason":"regulatory freeze"}`,
		string(e.OldState),
	)
	assert.JSONEq(t, `{"status":"suspended"}`, string(e.NewState))
	assert.JSONEq(t, `{"reason":"court order 2026-CV-456"}`, string(e.Metadata),
		"operator-supplied rationale lives in Metadata, not state")
}

func TestClearTenantLegalHold_NoActiveHold_EmitsNoAuditEvent(t *testing.T) {
	// Idempotent no-op: tenant had no hold to begin with, so no
	// ActionLegalHoldCleared event. The generic RPC audit interceptor
	// still records the call itself.
	notHeld := &Tenant{ID: fixedTenantID, Name: "Clean", Status: TenantStatusSuspended}

	repo := &mockRepo{
		getTenantFn: func(_ context.Context, _ uuid.UUID) (*Tenant, error) {
			return notHeld, nil
		},
		clearTenantLegalHoldFn: func(_ context.Context, _ uuid.UUID) (*Tenant, error) {
			return notHeld, nil
		},
	}

	store := &memoryAuditStore{}
	logger := audit.NewLogger(store, audit.LoggerConfig{
		BufferSize:    10,
		FlushInterval: 10 * time.Millisecond,
		BatchSize:     10,
	}, nil)
	svc := newTestService(repo).WithAuditLogger(logger)

	_, err := svc.ClearTenantLegalHold(context.Background(), fixedTenantID, "routine review")
	require.NoError(t, err)

	flushCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	require.NoError(t, logger.Close(flushCtx))

	assert.Empty(t, store.snapshot(), "no audit event when tenant had no hold to clear")
}

func TestClearTenantLegalHold_EmptyReason_EmptyMetadata(t *testing.T) {
	// Reason is optional (proto3 field presence); empty string maps to
	// empty-object metadata so we always emit valid JSON.
	reason := "compliance sign-off"
	before := &Tenant{ID: fixedTenantID, Status: TenantStatusSuspended, FreezeReason: &reason}
	after := &Tenant{ID: fixedTenantID, Status: TenantStatusSuspended}

	repo := &mockRepo{
		getTenantFn: func(_ context.Context, _ uuid.UUID) (*Tenant, error) { return before, nil },
		clearTenantLegalHoldFn: func(_ context.Context, _ uuid.UUID) (*Tenant, error) {
			return after, nil
		},
	}

	store := &memoryAuditStore{}
	logger := audit.NewLogger(store, audit.LoggerConfig{
		BufferSize:    10,
		FlushInterval: 10 * time.Millisecond,
		BatchSize:     10,
	}, nil)
	svc := newTestService(repo).WithAuditLogger(logger)

	_, err := svc.ClearTenantLegalHold(context.Background(), fixedTenantID, "")
	require.NoError(t, err)

	flushCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, logger.Close(flushCtx))

	events := store.snapshot()
	require.Len(t, events, 1)
	assert.JSONEq(t, `{}`, string(events[0].Metadata))
}

func TestClearTenantLegalHold_RepoError_Propagated(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("db unavailable")
	svc := newTestService(&mockRepo{
		getTenantFn: func(_ context.Context, _ uuid.UUID) (*Tenant, error) {
			return nil, sentinel
		},
	})

	_, err := svc.ClearTenantLegalHold(context.Background(), fixedTenantID, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

func TestInviteUser_SetsTokenAndExpiry(t *testing.T) {
	t.Parallel()
	var saved *Invitation
	svc := newTestService(&mockRepo{
		createInvitationFn: func(_ context.Context, inv *Invitation) error {
			saved = inv
			return nil
		},
	})

	inv := &Invitation{
		TenantID:  fixedTenantID,
		Email:     "new@example.com",
		InvitedBy: fixedUserID,
	}
	result, err := svc.InviteUser(context.Background(), inv)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Token)
	assert.Equal(t, "pending", saved.Status)
	assert.True(t, saved.ExpiresAt.After(time.Now()))
}

func TestInviteUser_ForeignRole_Denied(t *testing.T) {
	t.Parallel()
	foreignRoleID := uuid.New()
	svc := newTestService(&mockRepo{
		getRoleByIDFn: func(context.Context, uuid.UUID, uuid.UUID) (*Role, error) {
			return nil, ErrRoleNotFound // the role belongs to another tenant
		},
		createInvitationFn: func(context.Context, *Invitation) error {
			t.Fatal("repository must not be written for a foreign role")
			return nil
		},
	})

	inv := &Invitation{
		TenantID:  fixedTenantID,
		Email:     "new@example.com",
		InvitedBy: fixedUserID,
		RoleID:    &foreignRoleID,
	}
	_, err := svc.InviteUser(context.Background(), inv)
	require.ErrorIs(t, err, ErrRoleNotFound)
}

func TestAcceptInvitation_ExpiredToken(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getInvitationByTokenFn: func(_ context.Context, _ string) (*Invitation, error) {
			return &Invitation{
				Status:    "pending",
				ExpiresAt: time.Now().UTC().Add(-1 * time.Hour), // already expired
			}, nil
		},
	})

	_, err := svc.AcceptInvitation(context.Background(), "tok", "pass")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvitationExpired)
}

func TestAcceptInvitation_AlreadyAccepted(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getInvitationByTokenFn: func(_ context.Context, _ string) (*Invitation, error) {
			return &Invitation{
				Status:    "accepted",
				ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
			}, nil
		},
	})

	_, err := svc.AcceptInvitation(context.Background(), "tok", "pass")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvitationAccepted)
}

func TestAcceptInvitation_Success(t *testing.T) {
	t.Parallel()
	roleID := fixedRoleID
	marked := false
	svc := NewService(
		&mockRepo{
			getInvitationByTokenFn: func(_ context.Context, _ string) (*Invitation, error) {
				return &Invitation{
					ID:        fixedInviteID,
					TenantID:  fixedTenantID,
					Email:     "invited@example.com",
					RoleID:    &roleID,
					Status:    "pending",
					InvitedBy: fixedUserID,
					ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
				}, nil
			},
			markInvitationFn: func(_ context.Context, id uuid.UUID) error {
				assert.Equal(t, fixedInviteID, id)
				marked = true
				return nil
			},
		},
		&mockAuthenticator{},
		&mockTokenIssuer{},
		&mockHasher{},
		&mockVerifier{},
	)

	pair, err := svc.AcceptInvitation(context.Background(), "valid-token", "newpass")
	require.NoError(t, err)
	assert.NotEmpty(t, pair.AccessToken)
	assert.True(t, marked)
}

func TestAcceptInvitation_TokenNotFound(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getInvitationByTokenFn: func(_ context.Context, _ string) (*Invitation, error) {
			return nil, errors.New("not found")
		},
	})

	_, err := svc.AcceptInvitation(context.Background(), "bad-token", "pass")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvitationNotFound)
}

// A failed role grant must fail the acceptance, not return a token to a role-less user.
func TestAcceptInvitation_RoleGrantFails_ReturnsError(t *testing.T) {
	t.Parallel()
	roleID := fixedRoleID
	svc := newTestService(&mockRepo{
		getInvitationByTokenFn: func(context.Context, string) (*Invitation, error) {
			return &Invitation{
				ID: fixedInviteID, TenantID: fixedTenantID, Email: "invited@example.com",
				RoleID: &roleID, Status: "pending", InvitedBy: fixedUserID,
				ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
			}, nil
		},
		assignRoleFn: func(context.Context, *UserRole) error { return errors.New("insert failed") },
	})
	_, err := svc.AcceptInvitation(context.Background(), "valid-token", "newpass")
	require.Error(t, err, "a failed grant must not yield a token")
	// Pin the failure to the grant. Without this the test would still pass if a later
	// step (MarkInvitationAccepted, token issue) were the one erroring.
	require.ErrorContains(t, err, "assign role")
}

// AssignRole takes four consecutive uuid.UUID, so a transposed argument compiles
// silently and the mock's defaults echo any id back as a valid in-tenant object.
// Capture the UserRole actually written and pin each field. The dangerous swap is
// userID <-> assignedBy: it would grant the invited role to the inviter.
func TestAcceptInvitation_GrantsInvitedRoleToTheNewUser(t *testing.T) {
	t.Parallel()
	roleID := fixedRoleID
	var createdUserID uuid.UUID
	var granted *UserRole
	svc := newTestService(&mockRepo{
		getInvitationByTokenFn: func(context.Context, string) (*Invitation, error) {
			return &Invitation{
				ID: fixedInviteID, TenantID: fixedTenantID, Email: "invited@example.com",
				RoleID: &roleID, Status: "pending", InvitedBy: fixedUserID,
				ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
			}, nil
		},
		createUserFn: func(_ context.Context, u *User) error {
			createdUserID = u.ID
			return nil
		},
		assignRoleFn: func(_ context.Context, ur *UserRole) error {
			granted = ur
			return nil
		},
	})
	_, err := svc.AcceptInvitation(context.Background(), "valid-token", "newpass")
	require.NoError(t, err)
	require.NotNil(t, granted, "the invited role must be granted")
	require.NotEqual(t, uuid.Nil, createdUserID)
	assert.Equal(t, createdUserID, granted.UserID, "the role goes to the new user, not the inviter")
	assert.Equal(t, fixedRoleID, granted.RoleID)
	assert.Equal(t, fixedUserID, granted.AssignedBy, "attributed to the inviter")
}

// A seven-day-old invitation may name a role that has since been deleted.
func TestAcceptInvitation_RoleGone_ReturnsError(t *testing.T) {
	t.Parallel()
	roleID := fixedRoleID
	svc := newTestService(&mockRepo{
		getInvitationByTokenFn: func(context.Context, string) (*Invitation, error) {
			return &Invitation{
				ID: fixedInviteID, TenantID: fixedTenantID, Email: "invited@example.com",
				RoleID: &roleID, Status: "pending", InvitedBy: fixedUserID,
				ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
			}, nil
		},
		getRoleByIDFn: func(context.Context, uuid.UUID, uuid.UUID) (*Role, error) {
			return nil, ErrRoleNotFound
		},
		assignRoleFn: func(context.Context, *UserRole) error {
			t.Fatal("must not assign a role that no longer exists")
			return nil
		},
	})
	_, err := svc.AcceptInvitation(context.Background(), "valid-token", "newpass")
	require.ErrorIs(t, err, ErrRoleNotFound)
}

// A failed role grant must not issue a token: create-user, assign-role, and
// mark-accepted all live inside the WithTx callback, so a role-grant failure
// must roll back the user row too and never reach tokens.Issue.
func TestAcceptInvitation_FailedGrant_IssuesNoToken(t *testing.T) {
	roleID := uuid.New()
	repo := &mockRepo{
		getInvitationByTokenFn: func(_ context.Context, _ string) (*Invitation, error) {
			return &Invitation{
				ID: uuid.New(), TenantID: uuid.New(), Email: "invitee@example.com",
				Status: "pending", RoleID: &roleID, InvitedBy: uuid.New(),
				ExpiresAt: time.Now().UTC().Add(time.Hour),
			}, nil
		},
		createUserFn: func(_ context.Context, _ *User) error { return nil },
		getRoleByIDFn: func(_ context.Context, _, _ uuid.UUID) (*Role, error) {
			return nil, ErrRoleNotFound // the grant fails inside the tx
		},
	}
	issuer := &mockTokenIssuer{
		issueFn: func(_ context.Context, _ *auth.AuthResult) (*auth.TokenPair, error) {
			t.Fatal("tokens.Issue must not be called when the role grant fails")
			return nil, nil
		},
	}
	svc := NewService(repo, &mockAuthenticator{}, issuer, &mockHasher{}, &mockVerifier{})

	_, err := svc.AcceptInvitation(context.Background(), "tok", "pw")
	require.ErrorIs(t, err, ErrRoleNotFound)
}

// --- Additional coverage tests ---

func TestGetUser_Success(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getUserFn: func(_ context.Context, tenantID, id uuid.UUID) (*User, error) {
			return &User{ID: id, TenantID: tenantID, Status: "active"}, nil
		},
	})

	u, err := svc.GetUser(context.Background(), fixedTenantID, fixedUserID)
	require.NoError(t, err)
	assert.Equal(t, fixedUserID, u.ID)
}

func TestGetUser_Error(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getUserFn: func(_ context.Context, _, _ uuid.UUID) (*User, error) {
			return nil, errors.New("not found")
		},
	})

	_, err := svc.GetUser(context.Background(), fixedTenantID, fixedUserID)
	require.Error(t, err)
}

func TestUpdateUser_Success(t *testing.T) {
	t.Parallel()
	var saved *User
	svc := newTestService(&mockRepo{
		updateUserFn: func(_ context.Context, u *User) error {
			saved = u
			return nil
		},
	})

	user := &User{ID: fixedUserID, TenantID: fixedTenantID, DisplayName: "Updated Name"}
	result, err := svc.UpdateUser(context.Background(), user)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", result.DisplayName)
	assert.Equal(t, fixedUserID, saved.ID)
}

func TestUpdateUser_RevokesSessionsOnDeactivate(t *testing.T) {
	t.Parallel()
	var revokedFor uuid.UUID
	called := false
	tokens := &mockTokenIssuer{revokeAllForUserFn: func(_ context.Context, id uuid.UUID) error {
		called = true
		revokedFor = id
		return nil
	}}
	repo := &mockRepo{updateUserFn: func(_ context.Context, _ *User) error { return nil }}
	svc := NewService(repo, &mockAuthenticator{}, tokens, &mockHasher{}, &mockVerifier{})

	_, err := svc.UpdateUser(context.Background(), &User{ID: fixedUserID, TenantID: fixedTenantID, Status: "deactivated"})
	require.NoError(t, err)
	assert.True(t, called, "deactivating via UpdateUser must revoke sessions")
	assert.Equal(t, fixedUserID, revokedFor)
}

func TestUpdateUser_NoRevokeOnActiveOrEmpty(t *testing.T) {
	t.Parallel()
	for _, st := range []string{"active", ""} {
		st := st
		t.Run("status="+st, func(t *testing.T) {
			t.Parallel()
			called := false
			tokens := &mockTokenIssuer{revokeAllForUserFn: func(_ context.Context, _ uuid.UUID) error {
				called = true
				return nil
			}}
			repo := &mockRepo{updateUserFn: func(_ context.Context, _ *User) error { return nil }}
			svc := NewService(repo, &mockAuthenticator{}, tokens, &mockHasher{}, &mockVerifier{})
			_, err := svc.UpdateUser(context.Background(), &User{ID: fixedUserID, TenantID: fixedTenantID, Status: st})
			require.NoError(t, err)
			assert.False(t, called, "profile edit / reactivation must NOT revoke")
		})
	}
}

func TestUpdateUser_RevokeFailure_IsReported(t *testing.T) {
	t.Parallel()
	tokens := &mockTokenIssuer{revokeAllForUserFn: func(_ context.Context, _ uuid.UUID) error {
		return errors.New("redis down")
	}}
	repo := &mockRepo{updateUserFn: func(_ context.Context, _ *User) error { return nil }}
	svc := NewService(repo, &mockAuthenticator{}, tokens, &mockHasher{}, &mockVerifier{})
	_, err := svc.UpdateUser(context.Background(), &User{ID: fixedUserID, TenantID: fixedTenantID, Status: "deactivated"})
	require.Error(t, err)
}

func TestDeactivateUser_Success(t *testing.T) {
	t.Parallel()
	var deactivatedID uuid.UUID
	svc := newTestService(&mockRepo{
		deactivateUserFn: func(_ context.Context, _, id uuid.UUID) error {
			deactivatedID = id
			return nil
		},
	})

	err := svc.DeactivateUser(context.Background(), fixedTenantID, fixedUserID)
	require.NoError(t, err)
	assert.Equal(t, fixedUserID, deactivatedID)
}

func TestDeactivateUser_RevokesAllSessions(t *testing.T) {
	t.Parallel()
	var revokedFor uuid.UUID
	tokens := &mockTokenIssuer{
		revokeAllForUserFn: func(_ context.Context, userID uuid.UUID) error {
			revokedFor = userID
			return nil
		},
	}
	repo := &mockRepo{
		deactivateUserFn: func(_ context.Context, _, _ uuid.UUID) error {
			return nil
		},
	}
	svc := NewService(repo, &mockAuthenticator{}, tokens, &mockHasher{}, &mockVerifier{})

	require.NoError(t, svc.DeactivateUser(context.Background(), fixedTenantID, fixedUserID))
	assert.Equal(t, fixedUserID, revokedFor, "deactivating a user must revoke every session")
}

func TestActivateUser_Success(t *testing.T) {
	t.Parallel()
	var gotID uuid.UUID
	revoked := false
	tokens := &mockTokenIssuer{revokeAllForUserFn: func(_ context.Context, _ uuid.UUID) error { revoked = true; return nil }}
	repo := &mockRepo{activateUserFn: func(_ context.Context, _, id uuid.UUID) error { gotID = id; return nil }}
	svc := NewService(repo, &mockAuthenticator{}, tokens, &mockHasher{}, &mockVerifier{})
	require.NoError(t, svc.ActivateUser(context.Background(), fixedTenantID, fixedUserID))
	assert.Equal(t, fixedUserID, gotID)
	assert.False(t, revoked, "activation must not revoke sessions")
}

func TestActivateUser_NotDeactivated_Propagated(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{activateUserFn: func(_ context.Context, _, _ uuid.UUID) error { return ErrNotDeactivated }}
	svc := NewService(repo, &mockAuthenticator{}, &mockTokenIssuer{}, &mockHasher{}, &mockVerifier{})
	err := svc.ActivateUser(context.Background(), fixedTenantID, fixedUserID)
	assert.ErrorIs(t, err, ErrNotDeactivated)
}

func TestDeactivateUser_Error(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		deactivateUserFn: func(_ context.Context, _, _ uuid.UUID) error {
			return errors.New("db error")
		},
	})

	err := svc.DeactivateUser(context.Background(), fixedTenantID, fixedUserID)
	require.Error(t, err)
}

func TestAssignRole_Success(t *testing.T) {
	t.Parallel()
	var capturedUR *UserRole
	svc := newTestService(&mockRepo{
		assignRoleFn: func(_ context.Context, ur *UserRole) error {
			capturedUR = ur
			return nil
		},
	})

	err := svc.AssignRole(context.Background(), fixedTenantID, fixedUserID, fixedRoleID, fixedUserID)
	require.NoError(t, err)
	assert.Equal(t, fixedUserID, capturedUR.UserID)
	assert.Equal(t, fixedRoleID, capturedUR.RoleID)
}

func TestAssignRole_Error(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		assignRoleFn: func(_ context.Context, _ *UserRole) error {
			return errors.New("db error")
		},
	})

	err := svc.AssignRole(context.Background(), fixedTenantID, fixedUserID, fixedRoleID, fixedUserID)
	require.Error(t, err)
}

func TestAssignRole_ForeignRole_Denied(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getRoleByIDFn: func(context.Context, uuid.UUID, uuid.UUID) (*Role, error) {
			return nil, ErrRoleNotFound // the role belongs to another tenant
		},
		assignRoleFn: func(context.Context, *UserRole) error {
			t.Fatal("repository must not be written for a foreign role")
			return nil
		},
	})
	err := svc.AssignRole(context.Background(), uuid.New(), uuid.New(), uuid.New(), uuid.New())
	require.ErrorIs(t, err, ErrRoleNotFound)
}

func TestAssignRole_ForeignUser_Denied(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getUserFn: func(context.Context, uuid.UUID, uuid.UUID) (*User, error) {
			return nil, ErrUserNotFound // the target user belongs to another tenant
		},
		assignRoleFn: func(context.Context, *UserRole) error {
			t.Fatal("repository must not be written for a foreign target user")
			return nil
		},
	})
	err := svc.AssignRole(context.Background(), uuid.New(), uuid.New(), uuid.New(), uuid.New())
	require.ErrorIs(t, err, ErrUserNotFound)
}

func TestAssignRole_InTenant_Succeeds(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	var gotTenantOnRoleLookup, gotTenantOnUserLookup uuid.UUID
	assigned := false
	svc := newTestService(&mockRepo{
		getRoleByIDFn: func(_ context.Context, tenantID, roleID uuid.UUID) (*Role, error) {
			gotTenantOnRoleLookup = tenantID
			return &Role{ID: roleID, TenantID: tenantID, Name: "member", IsSystem: true}, nil
		},
		getUserFn: func(_ context.Context, tenantID, id uuid.UUID) (*User, error) {
			gotTenantOnUserLookup = tenantID
			return &User{ID: id, TenantID: tenantID, Status: "active"}, nil
		},
		assignRoleFn: func(context.Context, *UserRole) error { assigned = true; return nil },
	})
	require.NoError(t, svc.AssignRole(context.Background(), tid, uuid.New(), uuid.New(), uuid.New()))
	assert.True(t, assigned)
	assert.Equal(t, tid, gotTenantOnRoleLookup, "the role must be looked up in the caller's tenant")
	assert.Equal(t, tid, gotTenantOnUserLookup, "the target user must be looked up in the caller's tenant")
}

func TestRevokeRole_Success(t *testing.T) {
	t.Parallel()
	var capturedRoleID uuid.UUID
	svc := newTestService(&mockRepo{
		revokeRoleFn: func(_ context.Context, _, roleID uuid.UUID) error {
			capturedRoleID = roleID
			return nil
		},
	})

	err := svc.RevokeRole(context.Background(), fixedTenantID, fixedUserID, fixedRoleID)
	require.NoError(t, err)
	assert.Equal(t, fixedRoleID, capturedRoleID)
}

func TestRevokeRole_RevokesAllSessions(t *testing.T) {
	t.Parallel()
	var revokedFor uuid.UUID
	tokens := &mockTokenIssuer{
		revokeAllForUserFn: func(_ context.Context, userID uuid.UUID) error {
			revokedFor = userID
			return nil
		},
	}
	repo := &mockRepo{
		revokeRoleFn: func(_ context.Context, _, _ uuid.UUID) error {
			return nil
		},
	}
	svc := NewService(repo, &mockAuthenticator{}, tokens, &mockHasher{}, &mockVerifier{})

	require.NoError(t, svc.RevokeRole(context.Background(), fixedTenantID, fixedUserID, fixedRoleID))
	assert.Equal(t, fixedUserID, revokedFor, "revoking a role must revoke every session")
}

func TestRevokeRole_ForeignRole_Denied(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getRoleByIDFn: func(context.Context, uuid.UUID, uuid.UUID) (*Role, error) {
			return nil, ErrRoleNotFound // the role belongs to another tenant
		},
		revokeRoleFn: func(context.Context, uuid.UUID, uuid.UUID) error {
			t.Fatal("repository must not be written for a foreign role")
			return nil
		},
	})
	err := svc.RevokeRole(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.ErrorIs(t, err, ErrRoleNotFound)
}

func TestRevokeRole_ForeignUser_Denied(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getUserFn: func(context.Context, uuid.UUID, uuid.UUID) (*User, error) {
			return nil, ErrUserNotFound // the target user belongs to another tenant
		},
		revokeRoleFn: func(context.Context, uuid.UUID, uuid.UUID) error {
			t.Fatal("repository must not be written for a foreign target user")
			return nil
		},
	})
	err := svc.RevokeRole(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.ErrorIs(t, err, ErrUserNotFound)
}

func TestRevokeRole_InTenant_Succeeds(t *testing.T) {
	t.Parallel()
	tid := uuid.New()
	var gotTenantOnRoleLookup, gotTenantOnUserLookup uuid.UUID
	revoked := false
	svc := newTestService(&mockRepo{
		getRoleByIDFn: func(_ context.Context, tenantID, roleID uuid.UUID) (*Role, error) {
			gotTenantOnRoleLookup = tenantID
			return &Role{ID: roleID, TenantID: tenantID, Name: "member", IsSystem: true}, nil
		},
		getUserFn: func(_ context.Context, tenantID, id uuid.UUID) (*User, error) {
			gotTenantOnUserLookup = tenantID
			return &User{ID: id, TenantID: tenantID, Status: "active"}, nil
		},
		revokeRoleFn: func(context.Context, uuid.UUID, uuid.UUID) error { revoked = true; return nil },
	})
	require.NoError(t, svc.RevokeRole(context.Background(), tid, uuid.New(), uuid.New()))
	assert.True(t, revoked)
	assert.Equal(t, tid, gotTenantOnRoleLookup, "the role must be looked up in the caller's tenant")
	assert.Equal(t, tid, gotTenantOnUserLookup, "the target user must be looked up in the caller's tenant")
}

func TestListRoles_Success(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		listRolesFn: func(_ context.Context, tenantID uuid.UUID) ([]Role, error) {
			return []Role{
				{ID: fixedRoleID, Name: "super_admin", TenantID: tenantID},
			}, nil
		},
	})

	roles, err := svc.ListRoles(context.Background(), fixedTenantID)
	require.NoError(t, err)
	assert.Len(t, roles, 1)
	assert.Equal(t, "super_admin", roles[0].Name)
}

func TestGetTenant_Success(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*Tenant, error) {
			return &Tenant{ID: id, Status: "active", Plan: "professional"}, nil
		},
	})

	tenant, err := svc.GetTenant(context.Background(), fixedTenantID)
	require.NoError(t, err)
	assert.Equal(t, fixedTenantID, tenant.ID)
	assert.Equal(t, "professional", tenant.Plan)
}

func TestGetTenant_Error(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getTenantFn: func(_ context.Context, _ uuid.UUID) (*Tenant, error) {
			return nil, errors.New("not found")
		},
	})

	_, err := svc.GetTenant(context.Background(), fixedTenantID)
	require.Error(t, err)
}

func TestLogin_IssueTokenError(t *testing.T) {
	t.Parallel()
	svc := NewService(
		&mockRepo{},
		&mockAuthenticator{},
		&mockTokenIssuer{
			issueFn: func(_ context.Context, _ *auth.AuthResult) (*auth.TokenPair, error) {
				return nil, errors.New("token signing failed")
			},
		},
		&mockHasher{},
		&mockVerifier{},
	)

	_, err := svc.Login(context.Background(), fixedTenantID, "user@example.com", "pass")
	require.Error(t, err)
}

func TestRefreshToken_Error(t *testing.T) {
	t.Parallel()
	svc := NewService(
		&mockRepo{},
		&mockAuthenticator{},
		&mockTokenIssuer{
			refreshFn: func(_ context.Context, _ string) (*auth.TokenPair, error) {
				return nil, auth.ErrInvalidCredentials
			},
		},
		&mockHasher{},
		&mockVerifier{},
	)

	_, err := svc.RefreshToken(context.Background(), "bad-token")
	require.Error(t, err)
}

func TestLogout_Error(t *testing.T) {
	t.Parallel()
	svc := NewService(
		&mockRepo{},
		&mockAuthenticator{},
		&mockTokenIssuer{
			revokeFn: func(_ context.Context, _ string) error {
				return errors.New("revoke failed")
			},
		},
		&mockHasher{},
		&mockVerifier{},
	)

	err := svc.Logout(context.Background(), "some-token")
	require.Error(t, err)
}

func TestCreateTenant_DefaultPlan(t *testing.T) {
	t.Parallel()
	var savedTenant *Tenant
	svc := newTestService(&mockRepo{
		createTenantFn: func(_ context.Context, t *Tenant) error {
			savedTenant = t
			return nil
		},
	})

	_, err := svc.CreateTenant(context.Background(), &Tenant{Name: "Acme", CountryCode: "IN"})
	require.NoError(t, err)
	assert.Equal(t, "starter", savedTenant.Plan)
	assert.Equal(t, "active", savedTenant.Status)
}

func TestCreateTenant_SchemaMigratorCalled(t *testing.T) {
	t.Parallel()
	var migratedTenantID uuid.UUID
	m := &mockSchemaMigrator{
		migrateFn: func(_ context.Context, id uuid.UUID) error {
			migratedTenantID = id
			return nil
		},
	}
	svc := newTestService(&mockRepo{}).WithSchemaMigrator(m)

	tenant, err := svc.CreateTenant(context.Background(), &Tenant{Name: "Acme", Plan: "starter", CountryCode: "IN"})
	require.NoError(t, err)
	// Migrator must be called with the tenant ID that was assigned during creation.
	assert.Equal(t, tenant.ID, migratedTenantID)
}

func TestCreateTenant_SchemaMigratorError_Propagates(t *testing.T) {
	t.Parallel()
	m := &mockSchemaMigrator{
		migrateFn: func(_ context.Context, _ uuid.UUID) error {
			return errors.New("migrate: schema creation failed")
		},
	}
	svc := newTestService(&mockRepo{}).WithSchemaMigrator(m)

	_, err := svc.CreateTenant(context.Background(), &Tenant{Name: "Acme", Plan: "starter", CountryCode: "IN"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "migrate schema")
}

func TestSuspendTenant_Error(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		updateTenantStatusFn: func(_ context.Context, _ uuid.UUID, _ string, _ *time.Time, _ *string) error {
			return errors.New("db error")
		},
	})

	_, err := svc.SuspendTenant(context.Background(), fixedTenantID, nil, nil)
	require.Error(t, err)
}

func TestCreateUser_HashError(t *testing.T) {
	t.Parallel()
	svc := NewService(
		&mockRepo{},
		&mockAuthenticator{},
		&mockTokenIssuer{},
		&mockHasher{
			hashFn: func(_ string) (string, error) {
				return "", errors.New("bcrypt capacity exceeded")
			},
		},
		&mockVerifier{},
	)

	_, err := svc.CreateUser(context.Background(), &User{TenantID: fixedTenantID, Email: "x@y.com"}, "pass")
	require.Error(t, err)
}

func TestUpdateUser_Error(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		updateUserFn: func(_ context.Context, _ *User) error {
			return errors.New("db error")
		},
	})

	_, err := svc.UpdateUser(context.Background(), &User{ID: fixedUserID, TenantID: fixedTenantID})
	require.Error(t, err)
}

func TestRevokeRole_Error(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		revokeRoleFn: func(_ context.Context, _, _ uuid.UUID) error {
			return errors.New("db error")
		},
	})

	err := svc.RevokeRole(context.Background(), fixedTenantID, fixedUserID, fixedRoleID)
	require.Error(t, err)
}

func TestChangePassword_GetUserError(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getUserByIDFn: func(_ context.Context, _, _ uuid.UUID) (*auth.UserRecord, error) {
			return nil, errors.New("db error")
		},
	})

	err := svc.ChangePassword(context.Background(), fixedTenantID, fixedUserID, "old", "new")
	require.Error(t, err)
}

func TestChangePassword_HashNewPasswordError(t *testing.T) {
	t.Parallel()
	svc := NewService(
		&mockRepo{
			getUserByIDFn: func(_ context.Context, _, _ uuid.UUID) (*auth.UserRecord, error) {
				return &auth.UserRecord{PasswordHash: "hashed:oldpass"}, nil
			},
		},
		&mockAuthenticator{},
		&mockTokenIssuer{},
		&mockHasher{
			hashFn: func(_ string) (string, error) {
				return "", errors.New("hash error")
			},
		},
		&mockVerifier{},
	)

	err := svc.ChangePassword(context.Background(), fixedTenantID, fixedUserID, "oldpass", "newpass")
	require.Error(t, err)
}

func TestChangePassword_UpdateHashError(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getUserByIDFn: func(_ context.Context, _, _ uuid.UUID) (*auth.UserRecord, error) {
			return &auth.UserRecord{PasswordHash: "hashed:oldpass"}, nil
		},
		updatePasswordHashFn: func(_ context.Context, _, _ uuid.UUID, _ string) error {
			return errors.New("db error")
		},
	})

	err := svc.ChangePassword(context.Background(), fixedTenantID, fixedUserID, "oldpass", "newpass")
	require.Error(t, err)
}

// rehashIfNeeded is called from a goroutine inside Login.
// These tests invoke it directly (same package) to exercise the early-return paths.

func TestRehashIfNeeded_GetUserError(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getUserByEmailFn: func(_ context.Context, _ uuid.UUID, _ string) (*auth.UserRecord, error) {
			return nil, errors.New("db error")
		},
	})
	// Must not panic; errors are swallowed (non-critical write, Rule #6).
	svc.rehashIfNeeded(fixedTenantID, "user@example.com", "pass")
}

// --- WithOIDCEncryptionKey ---

func TestWithOIDCEncryptionKey_SetsKeyAndReturnsReceiver(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{})
	returned := svc.WithOIDCEncryptionKey(oidcTestKey)
	// Must return the same receiver for chaining.
	assert.Same(t, svc, returned)
	// Key is stored internally — verify by calling SetOIDCConfig (which reads it).
	err := svc.SetOIDCConfig(context.Background(), OIDCConfigParams{
		TenantID:        fixedTenantID.String(),
		ClientSecretEnc: "plaintext-secret",
	})
	// ErrOIDCKeyNotConfigured must NOT be returned — the key was set.
	require.NotErrorIs(t, err, ErrOIDCKeyNotConfigured)
}

// --- SetOIDCConfig ---

func TestSetOIDCConfig_NoKey_ReturnsErrOIDCKeyNotConfigured(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{})
	// No WithOIDCEncryptionKey call — key is nil.
	err := svc.SetOIDCConfig(context.Background(), OIDCConfigParams{
		TenantID:        fixedTenantID.String(),
		ClientSecretEnc: "plaintext",
	})
	require.ErrorIs(t, err, ErrOIDCKeyNotConfigured)
}

func TestSetOIDCConfig_BadKeyLength_EncryptError(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{})
	// 10-byte key is invalid for AES-256 — crypto.Encrypt returns ErrInvalidKeyLength.
	svc.WithOIDCEncryptionKey([]byte("tooshort-key"))
	err := svc.SetOIDCConfig(context.Background(), OIDCConfigParams{
		TenantID:        fixedTenantID.String(),
		ClientSecretEnc: "plaintext",
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "encrypt client secret")
}

func TestSetOIDCConfig_RepoError(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		upsertOIDCConfigFn: func(_ context.Context, _ OIDCConfigParams) error {
			return errors.New("db error")
		},
	})
	svc.WithOIDCEncryptionKey(oidcTestKey)

	err := svc.SetOIDCConfig(context.Background(), OIDCConfigParams{
		TenantID:        fixedTenantID.String(),
		ClientSecretEnc: "plaintext",
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "upsert oidc config")
}

func TestSetOIDCConfig_Success_SecretIsEncrypted(t *testing.T) {
	t.Parallel()
	const plaintext = "my-oauth2-client-secret"
	var capturedParams OIDCConfigParams

	svc := newTestService(&mockRepo{
		upsertOIDCConfigFn: func(_ context.Context, p OIDCConfigParams) error {
			capturedParams = p
			return nil
		},
	})
	svc.WithOIDCEncryptionKey(oidcTestKey)

	err := svc.SetOIDCConfig(context.Background(), OIDCConfigParams{
		TenantID:        fixedTenantID.String(),
		IssuerURL:       "https://accounts.google.com",
		ClientID:        "client-id-123",
		ClientSecretEnc: plaintext,
	})
	require.NoError(t, err)

	// Repo must receive the encrypted value, not the plaintext.
	assert.NotEqual(t, plaintext, capturedParams.ClientSecretEnc,
		"repo must receive ciphertext, not plaintext")

	// The encrypted value must be decryptable back to the original.
	decrypted, err := crypto.Decrypt(oidcTestKey, capturedParams.ClientSecretEnc)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)

	// Other fields must pass through unchanged.
	assert.Equal(t, "https://accounts.google.com", capturedParams.IssuerURL)
	assert.Equal(t, "client-id-123", capturedParams.ClientID)
}

func TestRehashIfNeeded_NeedsRehash_HashFails(t *testing.T) {
	t.Parallel()
	// A bcrypt-prefixed hash triggers NeedsRehash=true so the upgrade path is taken.
	bcryptLikeHash := "$2a$10$aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	svc := NewService(
		&mockRepo{
			getUserByEmailFn: func(_ context.Context, _ uuid.UUID, _ string) (*auth.UserRecord, error) {
				return &auth.UserRecord{
					ID:           fixedUserID,
					PasswordHash: bcryptLikeHash,
				}, nil
			},
		},
		&mockAuthenticator{},
		&mockTokenIssuer{},
		&mockHasher{
			hashFn: func(_ string) (string, error) {
				return "", errors.New("hash error")
			},
		},
		&mockVerifier{},
	)
	// Must not panic; hash failure is swallowed.
	svc.rehashIfNeeded(fixedTenantID, "user@example.com", "pass")
}

func TestSetTenantAddress_DerivesCurrencyFromCountry(t *testing.T) {
	t.Parallel()
	var saved *Tenant
	svc := newTestService(&mockRepo{
		updateTenantAddressFn: func(_ context.Context, tn *Tenant) (*Tenant, error) {
			saved = tn
			return tn, nil
		},
	})

	out, err := svc.SetTenantAddress(context.Background(), &Tenant{
		ID:          fixedTenantID,
		CountryCode: "in",
	})
	require.NoError(t, err)
	assert.Equal(t, "IN", out.CountryCode)
	assert.Equal(t, "INR", out.PrimaryCurrencyCode)
	assert.Equal(t, "IN", saved.CountryCode)
}

func TestSetTenantAddress_HonoursExplicitCurrency(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		updateTenantAddressFn: func(_ context.Context, tn *Tenant) (*Tenant, error) {
			return tn, nil
		},
	})

	out, err := svc.SetTenantAddress(context.Background(), &Tenant{
		ID:                  fixedTenantID,
		CountryCode:         "US",
		PrimaryCurrencyCode: "eur",
	})
	require.NoError(t, err)
	assert.Equal(t, "EUR", out.PrimaryCurrencyCode)
}

func TestSetTenantAddress_MissingCountry(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{})
	_, err := svc.SetTenantAddress(context.Background(), &Tenant{ID: fixedTenantID})
	assert.ErrorIs(t, err, ErrCountryRequired)
}

func TestSetTenantAddress_UnknownCountry(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{})
	_, err := svc.SetTenantAddress(context.Background(), &Tenant{
		ID:          fixedTenantID,
		CountryCode: "ZZ",
	})
	assert.ErrorIs(t, err, ErrUnknownCountry)
}

func TestSetTenantAddress_RepoError(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	svc := newTestService(&mockRepo{
		updateTenantAddressFn: func(_ context.Context, _ *Tenant) (*Tenant, error) {
			return nil, boom
		},
	})
	_, err := svc.SetTenantAddress(context.Background(), &Tenant{
		ID:          fixedTenantID,
		CountryCode: "IN",
	})
	assert.ErrorIs(t, err, boom)
}

func TestGetCurrentUser_ReturnsUserAndTenant(t *testing.T) {
	t.Parallel()
	wantUser := &User{ID: fixedUserID, TenantID: fixedTenantID, Status: "active"}
	wantTenant := &Tenant{ID: fixedTenantID, Name: "Acme", CountryCode: "IN", PrimaryCurrencyCode: "INR"}
	svc := NewService(
		&mockRepo{
			getUserFn: func(_ context.Context, _, _ uuid.UUID) (*User, error) {
				return wantUser, nil
			},
			getTenantFn: func(_ context.Context, _ uuid.UUID) (*Tenant, error) {
				return wantTenant, nil
			},
		},
		&mockAuthenticator{},
		&mockTokenIssuer{
			validateFn: func(_ context.Context, _ string) (*auth.Claims, error) {
				return &auth.Claims{Subject: fixedUserID, TenantID: fixedTenantID}, nil
			},
		},
		&mockHasher{},
		&mockVerifier{},
	)

	user, tenant, err := svc.GetCurrentUser(context.Background(), "access-token")
	require.NoError(t, err)
	assert.Equal(t, wantUser, user)
	assert.Equal(t, wantTenant, tenant)
}

func TestGetCurrentUser_InvalidToken(t *testing.T) {
	t.Parallel()
	svc := NewService(
		&mockRepo{},
		&mockAuthenticator{},
		&mockTokenIssuer{
			validateFn: func(_ context.Context, _ string) (*auth.Claims, error) {
				return nil, errors.New("invalid")
			},
		},
		&mockHasher{},
		&mockVerifier{},
	)
	_, _, err := svc.GetCurrentUser(context.Background(), "bad")
	require.Error(t, err)
}

func TestGetCurrentUser_UserLookupFails(t *testing.T) {
	t.Parallel()
	svc := NewService(
		&mockRepo{
			getUserFn: func(_ context.Context, _, _ uuid.UUID) (*User, error) {
				return nil, errors.New("db down")
			},
		},
		&mockAuthenticator{},
		&mockTokenIssuer{
			validateFn: func(_ context.Context, _ string) (*auth.Claims, error) {
				return &auth.Claims{Subject: fixedUserID, TenantID: fixedTenantID}, nil
			},
		},
		&mockHasher{},
		&mockVerifier{},
	)
	_, _, err := svc.GetCurrentUser(context.Background(), "tok")
	require.Error(t, err)
}

func TestGetCurrentUser_TenantLookupFails(t *testing.T) {
	t.Parallel()
	svc := NewService(
		&mockRepo{
			getTenantFn: func(_ context.Context, _ uuid.UUID) (*Tenant, error) {
				return nil, errors.New("no tenant")
			},
		},
		&mockAuthenticator{},
		&mockTokenIssuer{
			validateFn: func(_ context.Context, _ string) (*auth.Claims, error) {
				return &auth.Claims{Subject: fixedUserID, TenantID: fixedTenantID}, nil
			},
		},
		&mockHasher{},
		&mockVerifier{},
	)
	_, _, err := svc.GetCurrentUser(context.Background(), "tok")
	require.Error(t, err)
}

// The ledger permissions encode separation of duties. This test is the only guard
// against the strings drifting from services/ledger/handler.go's own copies, so it
// asserts literals rather than constants.
func TestSystemRoles_LedgerGrants(t *testing.T) {
	t.Parallel()

	want := map[string][]string{
		"super_admin": {"ledger:read", "ledger:write", "ledger:post", "ledger:admin"},
		"accountant":  {"ledger:read", "ledger:write", "ledger:post"},
		"manager":     {"ledger:read"},
		"coordinator": {"ledger:read"},
	}
	// Roles that must hold NO ledger permission at all.
	none := []string{"member", "inventory_manager", "project_supervisor"}

	got := map[string][]string{}
	for _, r := range systemRoles {
		for _, p := range r.permissions {
			if strings.HasPrefix(p, "ledger:") {
				got[r.name] = append(got[r.name], p)
			}
		}
	}

	for role, perms := range want {
		assert.ElementsMatch(t, perms, got[role], "role %s", role)
	}
	for _, role := range none {
		assert.Empty(t, got[role], "role %s must hold no ledger permission", role)
	}

	// Separation of duties: nobody but super_admin may close a period.
	for _, r := range systemRoles {
		if r.name == "super_admin" {
			continue
		}
		assert.NotContains(t, r.permissions, "ledger:admin",
			"only super_admin may hold ledger:admin — closing a period is a separately-privileged act")
	}
}

// TestSystemRoles_ReadGrants pins the #139 slice D grant matrix on the
// systemRoles side (the half that governs every future tenant). member is
// deliberately excluded from both permissions: ListExpenses has no
// submitted_by filter, so granting member expense:read would expose every
// colleague's amounts, vendors and categories to the lowest-privilege role
// (tracked as #165). Without this test, adding either permission to member
// in service.go would flip zero other tests.
func TestSystemRoles_ReadGrants(t *testing.T) {
	t.Parallel()

	want := map[string][]string{
		"super_admin":        {"expense:read", "inventory:read"},
		"manager":            {"expense:read", "inventory:read"},
		"coordinator":        {"expense:read", "inventory:read"},
		"accountant":         {"expense:read"},
		"inventory_manager":  {"inventory:read"},
		"project_supervisor": {"expense:read", "inventory:read"},
	}
	// Roles that must hold NEITHER read permission.
	none := []string{"member"}

	got := map[string][]string{}
	for _, r := range systemRoles {
		for _, p := range r.permissions {
			if p == "expense:read" || p == "inventory:read" {
				got[r.name] = append(got[r.name], p)
			}
		}
	}

	for role, perms := range want {
		assert.ElementsMatch(t, perms, got[role], "role %s", role)
	}
	for _, role := range none {
		assert.Empty(t, got[role], "role %s must hold neither expense:read nor inventory:read", role)
	}
}

// TestSystemRoles_DocumentGrants pins the #139 slice E grant matrix on the
// systemRoles side (the half that governs every future tenant). Absence is
// as load-bearing as presence: member and inventory_manager must hold
// document:read only, never document:write or document:delete. Without this
// test, adding document:write to member in service.go would flip zero other
// tests -- the gap #168's review found for slice D.
func TestSystemRoles_DocumentGrants(t *testing.T) {
	t.Parallel()

	want := map[string][]string{
		"super_admin":        {"document:read", "document:write", "document:delete"},
		"manager":            {"document:read", "document:write", "document:delete"},
		"coordinator":        {"document:read", "document:write"},
		"accountant":         {"document:read", "document:write"},
		"project_supervisor": {"document:read", "document:write"},
		"member":             {"document:read"},
		"inventory_manager":  {"document:read"},
	}

	byName := map[string][]string{}
	for _, r := range systemRoles {
		byName[r.name] = r.permissions
	}

	for role, expected := range want {
		perms := byName[role]
		for _, p := range expected {
			assert.Contains(t, perms, p, "%s must hold %s", role, p)
		}
		// Nothing outside `expected` from the document: namespace.
		for _, p := range perms {
			if strings.HasPrefix(p, "document:") {
				assert.Contains(t, expected, p, "%s holds unexpected %s", role, p)
			}
		}
	}
}

// TestSystemRoles_BillingGrants pins the #139 slice F grant matrix on the
// systemRoles side (the half that governs every future tenant). accountant
// holding billing:read but NOT billing:manage is the load-bearing exclusion:
// without this test, adding billing:manage to accountant in service.go would
// flip zero other tests -- the same gap #168's review found for slice D.
func TestSystemRoles_BillingGrants(t *testing.T) {
	t.Parallel()

	want := map[string][]string{
		"super_admin": {"billing:read", "billing:manage"},
		"manager":     {"billing:read", "billing:manage"},
		"accountant":  {"billing:read"},
	}

	byName := map[string][]string{}
	for _, r := range systemRoles {
		byName[r.name] = r.permissions
	}

	for role, expected := range want {
		perms := byName[role]
		for _, p := range expected {
			assert.Contains(t, perms, p, "%s must hold %s", role, p)
		}
	}
	// No role outside `want` holds any billing: permission; and within
	// `want`, nothing beyond what's expected (in particular, accountant
	// must never pick up billing:manage).
	for _, r := range systemRoles {
		for _, p := range r.permissions {
			if strings.HasPrefix(p, "billing:") {
				assert.Contains(t, want[r.name], p, "%s holds unexpected %s", r.name, p)
			}
		}
	}
}
