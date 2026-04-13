package iam

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	getUserByEmailFn              func(ctx context.Context, tenantID uuid.UUID, email string) (*auth.UserRecord, error)
	getUserByIDFn                 func(ctx context.Context, userID uuid.UUID) (*auth.UserRecord, error)
	createOIDCUserFn              func(ctx context.Context, tenantID uuid.UUID, email, displayName string) (*auth.UserRecord, error)
	getTenantStatusFn             func(ctx context.Context, tenantID uuid.UUID) (string, error)
	createUserFn                  func(ctx context.Context, user *User) error
	getUserFn                     func(ctx context.Context, tenantID, id uuid.UUID) (*User, error)
	listUsersFn                   func(ctx context.Context, tenantID uuid.UUID, status string, limit, offset int) ([]User, error)
	updateUserFn                  func(ctx context.Context, user *User) error
	updatePasswordHashFn          func(ctx context.Context, userID uuid.UUID, hash string) error
	deactivateUserFn              func(ctx context.Context, tenantID, id uuid.UUID) error
	createTenantFn                func(ctx context.Context, tenant *Tenant) error
	getTenantFn                   func(ctx context.Context, id uuid.UUID) (*Tenant, error)
	updateTenantStatusFn          func(ctx context.Context, id uuid.UUID, status string) error
	createRoleFn                  func(ctx context.Context, role *Role) error
	getRoleFn                     func(ctx context.Context, tenantID uuid.UUID, name string) (*Role, error)
	getRoleByIDFn                 func(ctx context.Context, tenantID, roleID uuid.UUID) (*Role, error)
	listRolesFn                   func(ctx context.Context, tenantID uuid.UUID) ([]Role, error)
	assignRoleFn                  func(ctx context.Context, ur *UserRole) error
	revokeRoleFn                  func(ctx context.Context, userID, roleID uuid.UUID) error
	getUserPermissionsFn          func(ctx context.Context, userID uuid.UUID, projectID *uuid.UUID) ([]string, error)
	createInvitationFn            func(ctx context.Context, inv *Invitation) error
	getInvitationByTokenFn        func(ctx context.Context, token string) (*Invitation, error)
	markInvitationFn              func(ctx context.Context, id uuid.UUID) error
	upsertOIDCConfigFn            func(ctx context.Context, params OIDCConfigParams) error
	startImpersonationFn          func(ctx context.Context, params StartImpersonationParams) (*ImpersonationSession, error)
	endImpersonationSessionFn     func(ctx context.Context, sessionID uuid.UUID) error
	expireImpersonationSessionsFn func(ctx context.Context) (int64, error)
	createAuditEntryFn            func(ctx context.Context, entry *AuditEntry) error
}

func (m *mockRepo) GetUserByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*auth.UserRecord, error) {
	if m.getUserByEmailFn != nil {
		return m.getUserByEmailFn(ctx, tenantID, email)
	}
	return &auth.UserRecord{ID: fixedUserID, TenantID: tenantID, Email: email, Status: "active"}, nil
}
func (m *mockRepo) GetUserByID(ctx context.Context, userID uuid.UUID) (*auth.UserRecord, error) {
	if m.getUserByIDFn != nil {
		return m.getUserByIDFn(ctx, userID)
	}
	return &auth.UserRecord{ID: userID, PasswordHash: "hashed"}, nil
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
func (m *mockRepo) UpdatePasswordHash(ctx context.Context, userID uuid.UUID, hash string) error {
	if m.updatePasswordHashFn != nil {
		return m.updatePasswordHashFn(ctx, userID, hash)
	}
	return nil
}
func (m *mockRepo) DeactivateUser(ctx context.Context, tenantID, id uuid.UUID) error {
	if m.deactivateUserFn != nil {
		return m.deactivateUserFn(ctx, tenantID, id)
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
func (m *mockRepo) UpdateTenantStatus(ctx context.Context, id uuid.UUID, status string) error {
	if m.updateTenantStatusFn != nil {
		return m.updateTenantStatusFn(ctx, id, status)
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
func (m *mockRepo) UpsertOIDCConfig(ctx context.Context, params OIDCConfigParams) error {
	if m.upsertOIDCConfigFn != nil {
		return m.upsertOIDCConfigFn(ctx, params)
	}
	return nil
}
func (m *mockRepo) StartImpersonation(ctx context.Context, params StartImpersonationParams) (*ImpersonationSession, error) {
	if m.startImpersonationFn != nil {
		return m.startImpersonationFn(ctx, params)
	}
	return &ImpersonationSession{
		ID:               uuid.MustParse("e5000000-0000-0000-0000-000000000005"),
		PlatformUserID:   params.PlatformUserID,
		TenantID:         params.TenantID,
		ImpersonatedUser: params.ImpersonatedUser,
		Reason:           params.Reason,
	}, nil
}
func (m *mockRepo) EndImpersonationSession(ctx context.Context, sessionID uuid.UUID) error {
	if m.endImpersonationSessionFn != nil {
		return m.endImpersonationSessionFn(ctx, sessionID)
	}
	return nil
}
func (m *mockRepo) ExpireImpersonationSessions(ctx context.Context) (int64, error) {
	if m.expireImpersonationSessionsFn != nil {
		return m.expireImpersonationSessionsFn(ctx)
	}
	return 0, nil
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
	issueFn   func(ctx context.Context, result *auth.AuthResult) (*auth.TokenPair, error)
	refreshFn func(ctx context.Context, refreshToken string) (*auth.TokenPair, error)
	revokeFn  func(ctx context.Context, refreshToken string) error
	validateFn func(ctx context.Context, accessToken string) (*auth.Claims, error)
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
		getUserByIDFn: func(_ context.Context, _ uuid.UUID) (*auth.UserRecord, error) {
			return &auth.UserRecord{PasswordHash: "hashed:correct"}, nil
		},
	})

	err := svc.ChangePassword(context.Background(), fixedUserID, "wrong", "newpass")
	require.Error(t, err)
	assert.ErrorIs(t, err, auth.ErrInvalidCredentials)
}

func TestChangePassword_Success(t *testing.T) {
	t.Parallel()
	var updatedHash string
	svc := newTestService(&mockRepo{
		getUserByIDFn: func(_ context.Context, _ uuid.UUID) (*auth.UserRecord, error) {
			return &auth.UserRecord{PasswordHash: "hashed:oldpass"}, nil
		},
		updatePasswordHashFn: func(_ context.Context, _ uuid.UUID, hash string) error {
			updatedHash = hash
			return nil
		},
	})

	err := svc.ChangePassword(context.Background(), fixedUserID, "oldpass", "newpass")
	require.NoError(t, err)
	assert.Equal(t, "hashed:newpass", updatedHash)
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

func TestCreateTenant_InvalidPlan(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{})

	_, err := svc.CreateTenant(context.Background(), &Tenant{Name: "Acme", Plan: "galaxy"})
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

	_, err := svc.CreateTenant(context.Background(), &Tenant{Name: "Acme", Plan: "starter"})
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
		"expense:submit", "expense:approve",
		"resource:manage",
		"inventory:checkout",
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

	_, err := svc.CreateTenant(context.Background(), &Tenant{Name: "Acme Software Pvt. Ltd.", Plan: "professional"})
	require.NoError(t, err)
	assert.Equal(t, "acme-software-pvt-ltd", savedTenant.Slug)
}

func TestSuspendTenant_UpdatesStatus(t *testing.T) {
	t.Parallel()
	var updatedStatus string
	svc := newTestService(&mockRepo{
		updateTenantStatusFn: func(_ context.Context, _ uuid.UUID, status string) error {
			updatedStatus = status
			return nil
		},
	})

	_, err := svc.SuspendTenant(context.Background(), fixedTenantID)
	require.NoError(t, err)
	assert.Equal(t, "suspended", updatedStatus)
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

func TestRevokeRole_Success(t *testing.T) {
	t.Parallel()
	var capturedRoleID uuid.UUID
	svc := newTestService(&mockRepo{
		revokeRoleFn: func(_ context.Context, _, roleID uuid.UUID) error {
			capturedRoleID = roleID
			return nil
		},
	})

	err := svc.RevokeRole(context.Background(), fixedUserID, fixedRoleID)
	require.NoError(t, err)
	assert.Equal(t, fixedRoleID, capturedRoleID)
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

	_, err := svc.CreateTenant(context.Background(), &Tenant{Name: "Acme"})
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

	tenant, err := svc.CreateTenant(context.Background(), &Tenant{Name: "Acme", Plan: "starter"})
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

	_, err := svc.CreateTenant(context.Background(), &Tenant{Name: "Acme", Plan: "starter"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "migrate schema")
}

func TestSuspendTenant_Error(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		updateTenantStatusFn: func(_ context.Context, _ uuid.UUID, _ string) error {
			return errors.New("db error")
		},
	})

	_, err := svc.SuspendTenant(context.Background(), fixedTenantID)
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

	err := svc.RevokeRole(context.Background(), fixedUserID, fixedRoleID)
	require.Error(t, err)
}

func TestChangePassword_GetUserError(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getUserByIDFn: func(_ context.Context, _ uuid.UUID) (*auth.UserRecord, error) {
			return nil, errors.New("db error")
		},
	})

	err := svc.ChangePassword(context.Background(), fixedUserID, "old", "new")
	require.Error(t, err)
}

func TestChangePassword_HashNewPasswordError(t *testing.T) {
	t.Parallel()
	svc := NewService(
		&mockRepo{
			getUserByIDFn: func(_ context.Context, _ uuid.UUID) (*auth.UserRecord, error) {
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

	err := svc.ChangePassword(context.Background(), fixedUserID, "oldpass", "newpass")
	require.Error(t, err)
}

func TestChangePassword_UpdateHashError(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getUserByIDFn: func(_ context.Context, _ uuid.UUID) (*auth.UserRecord, error) {
			return &auth.UserRecord{PasswordHash: "hashed:oldpass"}, nil
		},
		updatePasswordHashFn: func(_ context.Context, _ uuid.UUID, _ string) error {
			return errors.New("db error")
		},
	})

	err := svc.ChangePassword(context.Background(), fixedUserID, "oldpass", "newpass")
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

// --- Impersonation service tests ---

var fixedSessionID = uuid.MustParse("e5000000-0000-0000-0000-000000000005")

func TestStartImpersonation_Success(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{})
	params := StartImpersonationParams{
		PlatformUserID:   fixedUserID,
		TenantID:         fixedTenantID,
		ImpersonatedUser: uuid.MustParse("f6000000-0000-0000-0000-000000000006"),
		Reason:           "Customer support ticket #12345",
		Duration:         30 * time.Minute,
		IPAddress:        "10.0.0.1",
	}
	session, err := svc.StartImpersonation(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, params.Reason, session.Reason)
	assert.Equal(t, params.TenantID, session.TenantID)
}

func TestStartImpersonation_DurationCappedAt4Hours(t *testing.T) {
	t.Parallel()
	var capturedParams StartImpersonationParams
	svc := newTestService(&mockRepo{
		startImpersonationFn: func(_ context.Context, p StartImpersonationParams) (*ImpersonationSession, error) {
			capturedParams = p
			return &ImpersonationSession{ID: fixedSessionID, Reason: p.Reason}, nil
		},
	})

	_, err := svc.StartImpersonation(context.Background(), StartImpersonationParams{
		PlatformUserID:   fixedUserID,
		TenantID:         fixedTenantID,
		ImpersonatedUser: fixedUserID,
		Reason:           "test",
		Duration:         24 * time.Hour, // exceeds maxImpersonationDuration
	})
	require.NoError(t, err)
	assert.Equal(t, maxImpersonationDuration, capturedParams.Duration)
}

func TestStartImpersonation_ZeroDuration_UsesMax(t *testing.T) {
	t.Parallel()
	var capturedParams StartImpersonationParams
	svc := newTestService(&mockRepo{
		startImpersonationFn: func(_ context.Context, p StartImpersonationParams) (*ImpersonationSession, error) {
			capturedParams = p
			return &ImpersonationSession{ID: fixedSessionID}, nil
		},
	})

	_, err := svc.StartImpersonation(context.Background(), StartImpersonationParams{
		PlatformUserID: fixedUserID, TenantID: fixedTenantID,
		ImpersonatedUser: fixedUserID, Reason: "test", Duration: 0,
	})
	require.NoError(t, err)
	assert.Equal(t, maxImpersonationDuration, capturedParams.Duration)
}

func TestStartImpersonation_RepoError(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		startImpersonationFn: func(_ context.Context, _ StartImpersonationParams) (*ImpersonationSession, error) {
			return nil, errors.New("db error")
		},
	})

	_, err := svc.StartImpersonation(context.Background(), StartImpersonationParams{
		PlatformUserID: fixedUserID, TenantID: fixedTenantID,
		ImpersonatedUser: fixedUserID, Reason: "test", Duration: time.Hour,
	})
	require.Error(t, err)
}

func TestEndImpersonation_Success(t *testing.T) {
	t.Parallel()
	var endedID uuid.UUID
	svc := newTestService(&mockRepo{
		endImpersonationSessionFn: func(_ context.Context, id uuid.UUID) error {
			endedID = id
			return nil
		},
	})

	err := svc.EndImpersonation(context.Background(), fixedSessionID, fixedUserID)
	require.NoError(t, err)
	assert.Equal(t, fixedSessionID, endedID)
}

func TestEndImpersonation_NotFound(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		endImpersonationSessionFn: func(_ context.Context, _ uuid.UUID) error {
			return ErrImpersonationNotFound
		},
	})

	err := svc.EndImpersonation(context.Background(), fixedSessionID, fixedUserID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrImpersonationNotFound)
}

func TestEndImpersonation_AlreadyEnded(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		endImpersonationSessionFn: func(_ context.Context, _ uuid.UUID) error {
			return ErrImpersonationAlreadyEnded
		},
	})

	err := svc.EndImpersonation(context.Background(), fixedSessionID, fixedUserID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrImpersonationAlreadyEnded)
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
