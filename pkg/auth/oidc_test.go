package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- OIDC mocks ---

type mockOIDCConfigStore struct {
	getFn func(ctx context.Context, tenantID uuid.UUID) (*OIDCConfig, error)
}

func (m *mockOIDCConfigStore) GetOIDCConfig(ctx context.Context, tenantID uuid.UUID) (*OIDCConfig, error) {
	if m.getFn != nil {
		return m.getFn(ctx, tenantID)
	}
	return testOIDCConfig(), nil
}

type mockExchanger struct {
	exchangeFn func(ctx context.Context, cfg *OIDCConfig, code, redirectURI, codeVerifier string) (*OIDCClaims, error)
}

func (m *mockExchanger) Exchange(ctx context.Context, cfg *OIDCConfig, code, redirectURI, codeVerifier string) (*OIDCClaims, error) {
	if m.exchangeFn != nil {
		return m.exchangeFn(ctx, cfg, code, redirectURI, codeVerifier)
	}
	return &OIDCClaims{
		Subject:     "google-uid-123",
		Email:       "admin@acme.com",
		DisplayName: "Admin User",
		Groups:      []string{"engineering"},
	}, nil
}

func testOIDCConfig() *OIDCConfig {
	return &OIDCConfig{
		TenantID:         testTenantID,
		IssuerURL:        "https://accounts.google.com",
		ClientID:         "client-id-123",
		ClientSecret:     "client-secret",
		Scopes:           []string{"openid", "email", "profile"},
		EmailClaim:       "email",
		DisplayNameClaim: "name",
		AutoProvision:    true,
		DefaultRole:      "member",
	}
}

func newTestOIDCProvider(opts ...func(*OIDCProvider)) *OIDCProvider {
	p := NewOIDCProvider(
		&mockOIDCConfigStore{},
		&mockExchanger{},
		&mockUserStore{},
		&mockTenantStore{},
	)
	for _, fn := range opts {
		fn(p)
	}
	return p
}

// --- Tests ---

func TestOIDCProvider_Type(t *testing.T) {
	p := newTestOIDCProvider()
	assert.Equal(t, ProviderOIDC, p.Type())
}

func TestOIDCProvider_HappyPath_ExistingUser(t *testing.T) {
	t.Parallel()
	p := newTestOIDCProvider()

	result, err := p.Authenticate(context.Background(), AuthRequest{
		TenantID:          testTenantID,
		AuthorizationCode: "auth-code-123",
		RedirectURI:       "https://app.thittam.io/callback",
	})
	require.NoError(t, err)
	assert.Equal(t, testUserID, result.UserID)
	assert.Equal(t, ProviderOIDC, result.AuthMethod)
	assert.False(t, result.IsNewUser)
}

func TestOIDCProvider_JITProvision_NewUser(t *testing.T) {
	t.Parallel()

	newUserID := uuid.New()
	p := newTestOIDCProvider(func(op *OIDCProvider) {
		op.users = &mockUserStore{
			getByEmailFn: func(ctx context.Context, tenantID uuid.UUID, email string) (*UserRecord, error) {
				return nil, errors.New("not found")
			},
			createOIDCFn: func(ctx context.Context, tenantID uuid.UUID, email, displayName string) (*UserRecord, error) {
				return &UserRecord{
					ID:          newUserID,
					TenantID:    tenantID,
					Email:       email,
					DisplayName: displayName,
					Status:      "active",
					Roles:       []string{"member"},
				}, nil
			},
		}
	})

	result, err := p.Authenticate(context.Background(), AuthRequest{
		TenantID:          testTenantID,
		AuthorizationCode: "auth-code-123",
		RedirectURI:       "https://app.thittam.io/callback",
	})
	require.NoError(t, err)
	assert.Equal(t, newUserID, result.UserID)
	assert.True(t, result.IsNewUser)
	assert.Equal(t, ProviderOIDC, result.AuthMethod)
}

func TestOIDCProvider_JITDisabled_UserNotFound(t *testing.T) {
	t.Parallel()
	p := newTestOIDCProvider(func(op *OIDCProvider) {
		op.configs = &mockOIDCConfigStore{
			getFn: func(ctx context.Context, tenantID uuid.UUID) (*OIDCConfig, error) {
				cfg := testOIDCConfig()
				cfg.AutoProvision = false
				return cfg, nil
			},
		}
		op.users = &mockUserStore{
			getByEmailFn: func(ctx context.Context, tenantID uuid.UUID, email string) (*UserRecord, error) {
				return nil, errors.New("not found")
			},
		}
	})

	_, err := p.Authenticate(context.Background(), AuthRequest{
		TenantID:          testTenantID,
		AuthorizationCode: "auth-code-123",
		RedirectURI:       "https://app.thittam.io/callback",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestOIDCProvider_ConfigMissing(t *testing.T) {
	t.Parallel()
	p := newTestOIDCProvider(func(op *OIDCProvider) {
		op.configs = &mockOIDCConfigStore{
			getFn: func(ctx context.Context, tenantID uuid.UUID) (*OIDCConfig, error) {
				return nil, errors.New("no config")
			},
		}
	})

	_, err := p.Authenticate(context.Background(), AuthRequest{
		TenantID:          testTenantID,
		AuthorizationCode: "auth-code-123",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrOIDCConfigMissing)
}

func TestOIDCProvider_TokenExchangeFails(t *testing.T) {
	t.Parallel()
	p := newTestOIDCProvider(func(op *OIDCProvider) {
		op.exchanger = &mockExchanger{
			exchangeFn: func(ctx context.Context, cfg *OIDCConfig, code, redirectURI, codeVerifier string) (*OIDCClaims, error) {
				return nil, errors.New("idp error")
			},
		}
	})

	_, err := p.Authenticate(context.Background(), AuthRequest{
		TenantID:          testTenantID,
		AuthorizationCode: "bad-code",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrOIDCTokenExchange)
}

func TestOIDCProvider_MissingEmailClaim(t *testing.T) {
	t.Parallel()
	p := newTestOIDCProvider(func(op *OIDCProvider) {
		op.exchanger = &mockExchanger{
			exchangeFn: func(ctx context.Context, cfg *OIDCConfig, code, redirectURI, codeVerifier string) (*OIDCClaims, error) {
				return &OIDCClaims{
					Subject: "uid-123",
					Email:   "", // missing
				}, nil
			},
		}
	})

	_, err := p.Authenticate(context.Background(), AuthRequest{
		TenantID:          testTenantID,
		AuthorizationCode: "auth-code-123",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrOIDCClaimMapping)
}

func TestOIDCProvider_TenantSuspended(t *testing.T) {
	t.Parallel()
	p := newTestOIDCProvider(func(op *OIDCProvider) {
		op.tenants = &mockTenantStore{
			statusFn: func(ctx context.Context, tenantID uuid.UUID) (string, error) {
				return "suspended", nil
			},
		}
	})

	_, err := p.Authenticate(context.Background(), AuthRequest{
		TenantID:          testTenantID,
		AuthorizationCode: "auth-code-123",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTenantSuspended)
}

func TestOIDCProvider_TenantInactiveStatuses(t *testing.T) {
	for _, st := range []string{"grace", "deactivated", "purge_eligible"} {
		st := st
		t.Run(st, func(t *testing.T) {
			t.Parallel()
			p := newTestOIDCProvider(func(op *OIDCProvider) {
				op.tenants = &mockTenantStore{
					statusFn: func(ctx context.Context, tenantID uuid.UUID) (string, error) {
						return st, nil
					},
				}
			})

			_, err := p.Authenticate(context.Background(), AuthRequest{
				TenantID:          testTenantID,
				AuthorizationCode: "auth-code-123",
			})
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrTenantInactive)
		})
	}
}

func TestOIDCProvider_DeactivatedUser(t *testing.T) {
	t.Parallel()
	p := newTestOIDCProvider(func(op *OIDCProvider) {
		op.users = &mockUserStore{
			getByEmailFn: func(ctx context.Context, tenantID uuid.UUID, email string) (*UserRecord, error) {
				u := testUser()
				u.Status = "deactivated"
				return u, nil
			},
		}
	})

	_, err := p.Authenticate(context.Background(), AuthRequest{
		TenantID:          testTenantID,
		AuthorizationCode: "auth-code-123",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAccountDeactivated)
}
