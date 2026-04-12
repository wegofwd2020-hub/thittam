package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestResolver builds a Resolver with real (but mocked-dependency) providers.
// Both local and OIDC providers use the shared mocks from local_test.go / oidc_test.go.
func newTestResolver(store *mockOIDCConfigStore) *Resolver {
	local := newTestLocalProvider()
	oidc := newTestOIDCProvider()
	return NewResolver(local, oidc, store)
}

// --- Resolve ---

func TestResolve_OIDCConfigPresent_ReturnsOIDCProvider(t *testing.T) {
	t.Parallel()
	r := newTestResolver(&mockOIDCConfigStore{
		getFn: func(_ context.Context, _ uuid.UUID) (*OIDCConfig, error) {
			return testOIDCConfig(), nil
		},
	})

	provider, err := r.Resolve(context.Background(), testTenantID)
	require.NoError(t, err)
	assert.Equal(t, ProviderOIDC, provider.Type())
}

func TestResolve_OIDCConfigMissing_FallsBackToLocal(t *testing.T) {
	t.Parallel()
	r := newTestResolver(&mockOIDCConfigStore{
		getFn: func(_ context.Context, _ uuid.UUID) (*OIDCConfig, error) {
			return nil, ErrOIDCConfigMissing
		},
	})

	provider, err := r.Resolve(context.Background(), testTenantID)
	require.NoError(t, err)
	assert.Equal(t, ProviderLocal, provider.Type())
}

func TestResolve_OIDCConfigError_FallsBackToLocal(t *testing.T) {
	t.Parallel()
	// Any non-nil error (not just ErrOIDCConfigMissing) causes local fallback.
	r := newTestResolver(&mockOIDCConfigStore{
		getFn: func(_ context.Context, _ uuid.UUID) (*OIDCConfig, error) {
			return nil, errors.New("database timeout")
		},
	})

	provider, err := r.Resolve(context.Background(), testTenantID)
	require.NoError(t, err)
	assert.Equal(t, ProviderLocal, provider.Type())
}

func TestResolve_NilOIDCProvider_AlwaysLocal(t *testing.T) {
	t.Parallel()
	// When oidc is nil, Resolve must always return local — even if config exists.
	local := newTestLocalProvider()
	r := NewResolver(local, nil, &mockOIDCConfigStore{
		getFn: func(_ context.Context, _ uuid.UUID) (*OIDCConfig, error) {
			return testOIDCConfig(), nil
		},
	})

	provider, err := r.Resolve(context.Background(), testTenantID)
	require.NoError(t, err)
	assert.Equal(t, ProviderLocal, provider.Type())
}

func TestResolve_NilOIDCConfigStore_AlwaysLocal(t *testing.T) {
	t.Parallel()
	local := newTestLocalProvider()
	oidc := newTestOIDCProvider()
	r := NewResolver(local, oidc, nil)

	provider, err := r.Resolve(context.Background(), testTenantID)
	require.NoError(t, err)
	assert.Equal(t, ProviderLocal, provider.Type())
}

// --- Authenticate ---

func TestAuthenticate_WithAuthorizationCode_UsesOIDCDirect(t *testing.T) {
	t.Parallel()
	// When AuthorizationCode is present, Authenticate must use the OIDC provider
	// without consulting the config store at all.
	configStoreCalled := false
	r := newTestResolver(&mockOIDCConfigStore{
		getFn: func(_ context.Context, _ uuid.UUID) (*OIDCConfig, error) {
			configStoreCalled = true
			return testOIDCConfig(), nil
		},
	})

	result, err := r.Authenticate(context.Background(), AuthRequest{
		TenantID:          testTenantID,
		AuthorizationCode: "google-auth-code",
		RedirectURI:       "https://app.example.com/callback",
	})
	require.NoError(t, err)
	assert.Equal(t, ProviderOIDC, result.AuthMethod)
	assert.False(t, configStoreCalled, "config store must not be consulted when AuthorizationCode is present")
}

func TestAuthenticate_NoCode_OIDCConfigPresent_UsesOIDC(t *testing.T) {
	t.Parallel()
	r := newTestResolver(&mockOIDCConfigStore{
		getFn: func(_ context.Context, _ uuid.UUID) (*OIDCConfig, error) {
			return testOIDCConfig(), nil
		},
	})

	// No AuthorizationCode → falls through to Resolve, which sees OIDC config.
	// The OIDC provider's Authenticate with no code will produce a result via
	// the mock exchanger (exchange empty code → returns test claims).
	result, err := r.Authenticate(context.Background(), AuthRequest{
		TenantID: testTenantID,
		Email:    "admin@acme.com",
	})
	require.NoError(t, err)
	assert.Equal(t, ProviderOIDC, result.AuthMethod)
}

func TestAuthenticate_NoCode_NoOIDCConfig_UsesLocal(t *testing.T) {
	t.Parallel()
	r := newTestResolver(&mockOIDCConfigStore{
		getFn: func(_ context.Context, _ uuid.UUID) (*OIDCConfig, error) {
			return nil, ErrOIDCConfigMissing
		},
	})

	result, err := r.Authenticate(context.Background(), AuthRequest{
		TenantID: testTenantID,
		Email:    "admin@acme.com",
		Password: "securepass", // matches testUser()'s hash "$2a$12$securepass"
	})
	require.NoError(t, err)
	assert.Equal(t, ProviderLocal, result.AuthMethod)
}

func TestAuthenticate_NilOIDCProvider_NoCode_UsesLocal(t *testing.T) {
	t.Parallel()
	local := newTestLocalProvider()
	r := NewResolver(local, nil, nil)

	result, err := r.Authenticate(context.Background(), AuthRequest{
		TenantID: testTenantID,
		Email:    "admin@acme.com",
		Password: "securepass",
	})
	require.NoError(t, err)
	assert.Equal(t, ProviderLocal, result.AuthMethod)
}

func TestAuthenticate_WithCode_NilOIDCProvider_NoError(t *testing.T) {
	t.Parallel()
	// When OIDC provider is nil and AuthorizationCode is present, Authenticate
	// must not panic — it falls through to the local provider path via Resolve.
	local := newTestLocalProvider()
	r := NewResolver(local, nil, &mockOIDCConfigStore{
		getFn: func(_ context.Context, _ uuid.UUID) (*OIDCConfig, error) {
			return nil, ErrOIDCConfigMissing
		},
	})

	// Falls back to local since oidc is nil.
	result, err := r.Authenticate(context.Background(), AuthRequest{
		TenantID:          testTenantID,
		AuthorizationCode: "some-code",
		Email:             "admin@acme.com",
		Password:          "securepass",
	})
	require.NoError(t, err)
	assert.Equal(t, ProviderLocal, result.AuthMethod)
}
