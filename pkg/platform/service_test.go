package platform

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
	getByIDFn func(ctx context.Context, id uuid.UUID) (*PlatformUser, error)
}

func (m *mockUserStore) CreateUser(ctx context.Context, email, displayName, passwordHash string, role Role, createdBy uuid.UUID) (uuid.UUID, error) {
	return uuid.New(), nil
}
func (m *mockUserStore) GetUserByEmail(ctx context.Context, email string) (*PlatformUser, error) {
	return nil, nil
}
func (m *mockUserStore) GetUserByID(ctx context.Context, id uuid.UUID) (*PlatformUser, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return testPlatformAdmin(), nil
}
func (m *mockUserStore) ListUsers(ctx context.Context) ([]PlatformUser, error)        { return nil, nil }
func (m *mockUserStore) DeactivateUser(ctx context.Context, id uuid.UUID) error        { return nil }
func (m *mockUserStore) UpdateLastLogin(ctx context.Context, id uuid.UUID) error       { return nil }

type mockImpersonationStore struct{}

func (m *mockImpersonationStore) LogImpersonation(ctx context.Context, req ImpersonationRequest, expiresAt interface{}) (uuid.UUID, error) {
	return uuid.New(), nil
}

type mockTenantManager struct{}

func (m *mockTenantManager) ListTenants(ctx context.Context) ([]TenantSummary, error) { return nil, nil }
func (m *mockTenantManager) SuspendTenant(ctx context.Context, id uuid.UUID) error    { return nil }
func (m *mockTenantManager) ReactivateTenant(ctx context.Context, id uuid.UUID) error { return nil }
func (m *mockTenantManager) UpgradePlan(ctx context.Context, id uuid.UUID, plan string) error {
	return nil
}

type mockVerticalManager struct{}

func (m *mockVerticalManager) ListVerticals(ctx context.Context) ([]VerticalSummary, error) {
	return nil, nil
}
func (m *mockVerticalManager) DeactivateVertical(ctx context.Context, id string) error { return nil }

type mockLogger struct{}

func (mockLogger) Info(msg string, kv ...interface{})  {}
func (mockLogger) Warn(msg string, kv ...interface{})  {}
func (mockLogger) Error(msg string, kv ...interface{}) {}

// --- Helpers ---

var (
	testPlatformUserID = uuid.MustParse("f0000000-0000-0000-0000-000000000001")
	testTenantID       = uuid.MustParse("d0000000-0000-0000-0000-000000000001")
	testImpersonateID  = uuid.MustParse("d1000000-0000-0000-0000-000000000003")
)

func testPlatformAdmin() *PlatformUser {
	return &PlatformUser{
		ID:          testPlatformUserID,
		Email:       "admin@wegofwd2020.com",
		DisplayName: "Platform Admin",
		Role:        RoleAdmin,
		MFAEnabled:  true,
		Status:      "active",
	}
}

func newTestService(opts ...func(*Service)) *Service {
	s := NewService(
		&mockUserStore{},
		&mockTenantManager{},
		&mockImpersonationStore{},
		&mockVerticalManager{},
		mockLogger{},
	)
	for _, fn := range opts {
		fn(s)
	}
	return s
}

// --- Tests ---

func TestImpersonate_HappyPath(t *testing.T) {
	t.Parallel()
	s := newTestService()

	session, err := s.Impersonate(context.Background(), ImpersonationRequest{
		PlatformUserID: testPlatformUserID,
		TenantID:       testTenantID,
		UserID:         testImpersonateID,
		Reason:         "Support ticket #1234",
		IPAddress:      "10.0.0.1",
	})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, session.ID)
	assert.False(t, session.ExpiresAt.IsZero())
}

func TestImpersonate_ReasonRequired(t *testing.T) {
	t.Parallel()
	s := newTestService()

	_, err := s.Impersonate(context.Background(), ImpersonationRequest{
		PlatformUserID: testPlatformUserID,
		TenantID:       testTenantID,
		UserID:         testImpersonateID,
		Reason:         "",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrReasonRequired)
}

func TestImpersonate_SupportRoleDenied(t *testing.T) {
	t.Parallel()
	s := newTestService(func(s *Service) {
		s.users = &mockUserStore{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*PlatformUser, error) {
				u := testPlatformAdmin()
				u.Role = RoleSupport
				return u, nil
			},
		}
	})

	_, err := s.Impersonate(context.Background(), ImpersonationRequest{
		PlatformUserID: testPlatformUserID,
		TenantID:       testTenantID,
		UserID:         testImpersonateID,
		Reason:         "Support ticket #1234",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrImpersonationDenied)
}

func TestImpersonate_MFARequired(t *testing.T) {
	t.Parallel()
	s := newTestService(func(s *Service) {
		s.users = &mockUserStore{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*PlatformUser, error) {
				u := testPlatformAdmin()
				u.MFAEnabled = false
				return u, nil
			},
		}
	})

	_, err := s.Impersonate(context.Background(), ImpersonationRequest{
		PlatformUserID: testPlatformUserID,
		TenantID:       testTenantID,
		UserID:         testImpersonateID,
		Reason:         "Support ticket #1234",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMFARequired)
}

func TestImpersonate_UserNotFound(t *testing.T) {
	t.Parallel()
	s := newTestService(func(s *Service) {
		s.users = &mockUserStore{
			getByIDFn: func(ctx context.Context, id uuid.UUID) (*PlatformUser, error) {
				return nil, errors.New("not found")
			},
		}
	})

	_, err := s.Impersonate(context.Background(), ImpersonationRequest{
		PlatformUserID: testPlatformUserID,
		TenantID:       testTenantID,
		UserID:         testImpersonateID,
		Reason:         "Support ticket #1234",
	})
	require.Error(t, err)
}

func TestCheckAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		role     Role
		required Role
		wantErr  bool
	}{
		{"owner can do anything", RoleOwner, RoleOwner, false},
		{"owner can do admin tasks", RoleOwner, RoleAdmin, false},
		{"admin can do admin tasks", RoleAdmin, RoleAdmin, false},
		{"admin can do support tasks", RoleAdmin, RoleSupport, false},
		{"support can do support tasks", RoleSupport, RoleSupport, false},
		{"support cannot do admin tasks", RoleSupport, RoleAdmin, true},
		{"admin cannot do owner tasks", RoleAdmin, RoleOwner, true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			user := &PlatformUser{Role: tc.role}
			err := CheckAccess(user, tc.required)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSeedPlatformOwner(t *testing.T) {
	t.Parallel()
	s := newTestService()

	id, err := s.SeedPlatformOwner(context.Background(), "cto@wegofwd2020.com", "CTO", "$2a$12$hash")
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, id)
}
