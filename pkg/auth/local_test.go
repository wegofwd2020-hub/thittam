package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

type mockUserStore struct {
	getByEmailFn func(ctx context.Context, tenantID uuid.UUID, email string) (*UserRecord, error)
	getByIDFn    func(ctx context.Context, tenantID, userID uuid.UUID) (*UserRecord, error)
	createOIDCFn func(ctx context.Context, tenantID uuid.UUID, email, displayName string) (*UserRecord, error)
}

func (m *mockUserStore) GetUserByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*UserRecord, error) {
	if m.getByEmailFn != nil {
		return m.getByEmailFn(ctx, tenantID, email)
	}
	return testUser(), nil
}

func (m *mockUserStore) GetUserByID(ctx context.Context, tenantID, userID uuid.UUID) (*UserRecord, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, tenantID, userID)
	}
	return testUser(), nil
}

func (m *mockUserStore) CreateOIDCUser(ctx context.Context, tenantID uuid.UUID, email, displayName string) (*UserRecord, error) {
	if m.createOIDCFn != nil {
		return m.createOIDCFn(ctx, tenantID, email, displayName)
	}
	u := testUser()
	u.Email = email
	u.DisplayName = displayName
	u.PasswordHash = ""
	return u, nil
}

type mockTenantStore struct {
	statusFn func(ctx context.Context, tenantID uuid.UUID) (string, error)
}

func (m *mockTenantStore) GetTenantStatus(ctx context.Context, tenantID uuid.UUID) (string, error) {
	if m.statusFn != nil {
		return m.statusFn(ctx, tenantID)
	}
	return "active", nil
}

type mockVerifier struct {
	verifyFn func(password, hash string) error
}

func (m *mockVerifier) Verify(password, hash string) error {
	if m.verifyFn != nil {
		return m.verifyFn(password, hash)
	}
	// Simple mock: password matches if hash ends with password
	if hash == "$2a$12$"+password {
		return nil
	}
	return errors.New("mismatch")
}

// --- Test helpers ---

var (
	testTenantID = uuid.MustParse("10000000-0000-0000-0000-000000000001")
	testUserID   = uuid.MustParse("50000000-0000-0000-0000-000000000001")
)

func testUser() *UserRecord {
	return &UserRecord{
		ID:           testUserID,
		TenantID:     testTenantID,
		Email:        "admin@acme.com",
		DisplayName:  "Admin User",
		PasswordHash: "$2a$12$securepass",
		Status:       "active",
		Roles:        []string{"super_admin"},
		Permissions:  []string{"production:read", "budget:write"},
	}
}

func newTestLocalProvider(opts ...func(*LocalProvider)) *LocalProvider {
	p := NewLocalProvider(
		&mockUserStore{},
		&mockTenantStore{},
		&mockVerifier{},
	)
	for _, fn := range opts {
		fn(p)
	}
	return p
}

// --- Tests ---

func TestLocalProvider_Type(t *testing.T) {
	p := newTestLocalProvider()
	assert.Equal(t, ProviderLocal, p.Type())
}

func TestLocalProvider_HappyPath(t *testing.T) {
	t.Parallel()
	p := newTestLocalProvider()

	result, err := p.Authenticate(context.Background(), AuthRequest{
		TenantID: testTenantID,
		Email:    "admin@acme.com",
		Password: "securepass",
	})
	require.NoError(t, err)
	assert.Equal(t, testUserID, result.UserID)
	assert.Equal(t, testTenantID, result.TenantID)
	assert.Equal(t, "admin@acme.com", result.Email)
	assert.Equal(t, ProviderLocal, result.AuthMethod)
	assert.Equal(t, []string{"super_admin"}, result.Roles)
}

func TestLocalProvider_WrongPassword(t *testing.T) {
	t.Parallel()
	p := newTestLocalProvider()

	_, err := p.Authenticate(context.Background(), AuthRequest{
		TenantID: testTenantID,
		Email:    "admin@acme.com",
		Password: "wrongpassword",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestLocalProvider_UserNotFound(t *testing.T) {
	t.Parallel()
	p := newTestLocalProvider(func(lp *LocalProvider) {
		lp.users = &mockUserStore{
			getByEmailFn: func(ctx context.Context, tenantID uuid.UUID, email string) (*UserRecord, error) {
				return nil, errors.New("not found")
			},
		}
	})

	_, err := p.Authenticate(context.Background(), AuthRequest{
		TenantID: testTenantID,
		Email:    "nobody@acme.com",
		Password: "securepass",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestLocalProvider_DeactivatedUser(t *testing.T) {
	t.Parallel()
	p := newTestLocalProvider(func(lp *LocalProvider) {
		lp.users = &mockUserStore{
			getByEmailFn: func(ctx context.Context, tenantID uuid.UUID, email string) (*UserRecord, error) {
				u := testUser()
				u.Status = "deactivated"
				return u, nil
			},
		}
	})

	_, err := p.Authenticate(context.Background(), AuthRequest{
		TenantID: testTenantID,
		Email:    "admin@acme.com",
		Password: "securepass",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAccountDeactivated)
}

func TestLocalProvider_TenantSuspended(t *testing.T) {
	t.Parallel()
	p := newTestLocalProvider(func(lp *LocalProvider) {
		lp.tenants = &mockTenantStore{
			statusFn: func(ctx context.Context, tenantID uuid.UUID) (string, error) {
				return "suspended", nil
			},
		}
	})

	_, err := p.Authenticate(context.Background(), AuthRequest{
		TenantID: testTenantID,
		Email:    "admin@acme.com",
		Password: "securepass",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTenantSuspended)
}

func TestLocalProvider_TenantInactiveStatuses(t *testing.T) {
	for _, st := range []string{"grace", "deactivated", "purge_eligible"} {
		st := st
		t.Run(st, func(t *testing.T) {
			t.Parallel()
			p := newTestLocalProvider(func(lp *LocalProvider) {
				lp.tenants = &mockTenantStore{
					statusFn: func(ctx context.Context, tenantID uuid.UUID) (string, error) {
						return st, nil
					},
				}
			})

			_, err := p.Authenticate(context.Background(), AuthRequest{
				TenantID: testTenantID,
				Email:    "admin@acme.com",
				Password: "securepass",
			})
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrTenantInactive)
		})
	}
}

func TestLocalProvider_OIDCOnlyUser(t *testing.T) {
	t.Parallel()
	p := newTestLocalProvider(func(lp *LocalProvider) {
		lp.users = &mockUserStore{
			getByEmailFn: func(ctx context.Context, tenantID uuid.UUID, email string) (*UserRecord, error) {
				u := testUser()
				u.PasswordHash = "" // OIDC-only user
				return u, nil
			},
		}
	})

	_, err := p.Authenticate(context.Background(), AuthRequest{
		TenantID: testTenantID,
		Email:    "admin@acme.com",
		Password: "securepass",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestLocalProvider_InvitedUser(t *testing.T) {
	t.Parallel()
	p := newTestLocalProvider(func(lp *LocalProvider) {
		lp.users = &mockUserStore{
			getByEmailFn: func(ctx context.Context, tenantID uuid.UUID, email string) (*UserRecord, error) {
				u := testUser()
				u.Status = "invited"
				return u, nil
			},
		}
	})

	_, err := p.Authenticate(context.Background(), AuthRequest{
		TenantID: testTenantID,
		Email:    "admin@acme.com",
		Password: "securepass",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAccountInvited)
}
