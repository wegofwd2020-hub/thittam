package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubVault stands in for a real Vault server. It records login calls and
// returns configurable responses for the secret read path.
type stubVault struct {
	loginCalls  int
	loginToken  string
	loginTTL    int
	loginStatus int

	secretPath   string
	secretValue  string
	secretStatus int
}

func (s *stubVault) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.URL.Path {
	case "/v1/auth/approle/login":
		s.loginCalls++
		if s.loginStatus != 0 && s.loginStatus != http.StatusOK {
			w.WriteHeader(s.loginStatus)
			json.NewEncoder(w).Encode(vaultErrorResponse{Errors: []string{"permission denied"}})
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(vaultLoginResponse{
			Auth: struct {
				ClientToken   string `json:"client_token"`
				LeaseDuration int    `json:"lease_duration"`
			}{
				ClientToken:   s.loginToken,
				LeaseDuration: s.loginTTL,
			},
		})

	case "/v1/" + "secret/data/" + s.secretPath:
		if r.Header.Get("X-Vault-Token") == "" {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(vaultErrorResponse{Errors: []string{"missing token"}})
			return
		}
		if s.secretStatus != 0 && s.secretStatus != http.StatusOK {
			w.WriteHeader(s.secretStatus)
			json.NewEncoder(w).Encode(vaultErrorResponse{Errors: []string{"not found"}})
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(vaultSecretResponse{
			Data: struct {
				Data map[string]string `json:"data"`
			}{
				Data: map[string]string{"value": s.secretValue},
			},
		})

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func newVaultSource(t *testing.T, stub *stubVault) (*VaultSource, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(stub)
	t.Cleanup(srv.Close)
	src := NewVaultSource(VaultConfig{
		Address:  srv.URL,
		Mount:    "secret",
		RoleID:   "test-role-id",
		SecretID: "test-secret-id",
	})
	return src, srv
}

func TestVaultSource_GetSecret_ReturnsValue(t *testing.T) {
	t.Parallel()

	stub := &stubVault{
		loginToken:  "s.test-token",
		loginTTL:    3600,
		secretPath:  "iam/jwt-private-key",
		secretValue: "-----BEGIN RSA PRIVATE KEY-----\nMIIEow==\n-----END RSA PRIVATE KEY-----",
	}
	src, _ := newVaultSource(t, stub)

	got, err := src.GetSecret(context.Background(), "iam/jwt-private-key")

	require.NoError(t, err)
	assert.Equal(t, stub.secretValue, string(got))
	assert.Equal(t, 1, stub.loginCalls, "should login exactly once")
}

func TestVaultSource_GetSecret_CachesToken(t *testing.T) {
	t.Parallel()

	stub := &stubVault{
		loginToken:  "s.cached-token",
		loginTTL:    3600,
		secretPath:  "iam/jwt-private-key",
		secretValue: "test-key",
	}
	src, _ := newVaultSource(t, stub)

	_, err := src.GetSecret(context.Background(), "iam/jwt-private-key")
	require.NoError(t, err)
	_, err = src.GetSecret(context.Background(), "iam/jwt-private-key")
	require.NoError(t, err)

	assert.Equal(t, 1, stub.loginCalls, "second call must reuse cached token, not re-login")
}

func TestVaultSource_GetSecret_RefreshesExpiredToken(t *testing.T) {
	t.Parallel()

	stub := &stubVault{
		loginToken:  "s.short-lived",
		loginTTL:    3600,
		secretPath:  "iam/jwt-private-key",
		secretValue: "test-key",
	}
	src, _ := newVaultSource(t, stub)

	// Prime the cache.
	_, err := src.GetSecret(context.Background(), "iam/jwt-private-key")
	require.NoError(t, err)
	assert.Equal(t, 1, stub.loginCalls)

	// Simulate an expired token by backdating the expiry.
	src.mu.Lock()
	src.tokenExpiry = time.Now().Add(-1 * time.Minute)
	src.mu.Unlock()

	_, err = src.GetSecret(context.Background(), "iam/jwt-private-key")
	require.NoError(t, err)
	assert.Equal(t, 2, stub.loginCalls, "expired token must trigger a new login")
}

func TestVaultSource_GetSecret_ReturnsErrSecretNotFound(t *testing.T) {
	t.Parallel()

	stub := &stubVault{
		loginToken:   "s.test-token",
		loginTTL:     3600,
		secretPath:   "iam/jwt-private-key",
		secretStatus: http.StatusNotFound,
	}
	src, _ := newVaultSource(t, stub)

	_, err := src.GetSecret(context.Background(), "iam/jwt-private-key")

	assert.ErrorIs(t, err, ErrSecretNotFound)
}

func TestVaultSource_GetSecret_ReturnsErrorOnLoginFailure(t *testing.T) {
	t.Parallel()

	stub := &stubVault{
		loginStatus: http.StatusForbidden,
	}
	src, _ := newVaultSource(t, stub)

	_, err := src.GetSecret(context.Background(), "iam/jwt-private-key")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "vault auth")
}

func TestVaultSource_CheckHealth_SucceedsWhenVaultReachable(t *testing.T) {
	t.Parallel()

	stub := &stubVault{
		loginToken: "s.health-token",
		loginTTL:   3600,
	}
	src, _ := newVaultSource(t, stub)

	err := src.CheckHealth(context.Background())
	assert.NoError(t, err)
}

func TestVaultSource_CheckHealth_FailsWhenVaultUnreachable(t *testing.T) {
	t.Parallel()

	// Point at a non-existent server.
	src := NewVaultSource(VaultConfig{
		Address:  "http://127.0.0.1:19999",
		Mount:    "secret",
		RoleID:   "r",
		SecretID: "s",
	})

	err := src.CheckHealth(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vault")
}

func TestVaultSource_GetSecret_MissingValueKey(t *testing.T) {
	t.Parallel()

	// Vault responds OK but the KV entry has no "value" key.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/approle/login":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(vaultLoginResponse{
				Auth: struct {
					ClientToken   string `json:"client_token"`
					LeaseDuration int    `json:"lease_duration"`
				}{ClientToken: "tok", LeaseDuration: 3600},
			})
		default:
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(vaultSecretResponse{
				Data: struct {
					Data map[string]string `json:"data"`
				}{Data: map[string]string{"other_key": "oops"}},
			})
		}
	}))
	t.Cleanup(srv.Close)

	src := NewVaultSource(VaultConfig{Address: srv.URL, Mount: "secret", RoleID: "r", SecretID: "s"})
	_, err := src.GetSecret(context.Background(), "iam/jwt-private-key")

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrSecretNotFound))
}
