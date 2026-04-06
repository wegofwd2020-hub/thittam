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
)

// --- Fixed test IDs (deterministic, never random) ---

var (
	fixedTenantID = uuid.MustParse("a1000000-0000-0000-0000-000000000001")
	fixedUserID   = uuid.MustParse("b2000000-0000-0000-0000-000000000002")
	fixedRoleID   = uuid.MustParse("c3000000-0000-0000-0000-000000000003")
	fixedInviteID = uuid.MustParse("d4000000-0000-0000-0000-000000000004")
)

// --- Mock Repository ---

type mockRepo struct {
	getUserByEmailFn       func(ctx context.Context, tenantID uuid.UUID, email string) (*auth.UserRecord, error)
	getUserByIDFn          func(ctx context.Context, userID uuid.UUID) (*auth.UserRecord, error)
	createOIDCUserFn       func(ctx context.Context, tenantID uuid.UUID, email, displayName string) (*auth.UserRecord, error)
	getTenantStatusFn      func(ctx context.Context, tenantID uuid.UUID) (string, error)
	createUserFn           func(ctx context.Context, user *User) error
	getUserFn              func(ctx context.Context, tenantID, id uuid.UUID) (*User, error)
	listUsersFn            func(ctx context.Context, tenantID uuid.UUID, status string, limit, offset int) ([]User, error)
	updateUserFn           func(ctx context.Context, user *User) error
	updatePasswordHashFn   func(ctx context.Context, userID uuid.UUID, hash string) error
	deactivateUserFn       func(ctx context.Context, tenantID, id uuid.UUID) error
	createTenantFn         func(ctx context.Context, tenant *Tenant) error
	getTenantFn            func(ctx context.Context, id uuid.UUID) (*Tenant, error)
	updateTenantStatusFn   func(ctx context.Context, id uuid.UUID, status string) error
	createRoleFn           func(ctx context.Context, role *Role) error
	getRoleFn              func(ctx context.Context, tenantID uuid.UUID, name string) (*Role, error)
	listRolesFn            func(ctx context.Context, tenantID uuid.UUID) ([]Role, error)
	assignRoleFn           func(ctx context.Context, ur *UserRole) error
	revokeRoleFn           func(ctx context.Context, userID, roleID uuid.UUID) error
	getUserPermissionsFn   func(ctx context.Context, userID uuid.UUID) ([]string, error)
	createInvitationFn     func(ctx context.Context, inv *Invitation) error
	getInvitationByTokenFn func(ctx context.Context, token string) (*Invitation, error)
	markInvitationFn       func(ctx context.Context, id uuid.UUID) error
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
func (m *mockRepo) GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]string, error) {
	if m.getUserPermissionsFn != nil {
		return m.getUserPermissionsFn(ctx, userID)
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
		getUserPermissionsFn: func(_ context.Context, _ uuid.UUID) ([]string, error) {
			return []string{"production:read", "budget:write"}, nil
		},
	})

	ok, err := svc.CheckPermission(context.Background(), fixedUserID, "budget:write")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestCheckPermission_NotFound(t *testing.T) {
	t.Parallel()
	svc := newTestService(&mockRepo{
		getUserPermissionsFn: func(_ context.Context, _ uuid.UUID) ([]string, error) {
			return []string{"production:read"}, nil
		},
	})

	ok, err := svc.CheckPermission(context.Background(), fixedUserID, "budget:approve")
	require.NoError(t, err)
	assert.False(t, ok)
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
	assert.Contains(t, seededRoles, "crew_member")
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
