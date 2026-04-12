package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── fakeOIDCServer ──────────────────────────────────────────────────────────

// fakeOIDCServer is a minimal OIDC-compliant HTTP server for testing HTTPExchanger.
// It serves discovery, JWKS, and token endpoints backed by a real RSA key pair.
type fakeOIDCServer struct {
	srv            *httptest.Server
	privKey        *rsa.PrivateKey
	discoveryCalls atomic.Int32
	mu             sync.Mutex
	tokenFn        func(w http.ResponseWriter, r *http.Request)
}

// newFakeOIDCServer starts a fake OIDC server and registers t.Cleanup to stop it.
func newFakeOIDCServer(t *testing.T) *fakeOIDCServer {
	t.Helper()
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	f := &fakeOIDCServer{privKey: privKey}

	mux := http.NewServeMux()

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		f.discoveryCalls.Add(1)
		baseURL := "http://" + r.Host
		doc := map[string]any{
			"issuer":                                baseURL,
			"authorization_endpoint":                baseURL + "/authorize",
			"token_endpoint":                        baseURL + "/token",
			"jwks_uri":                              baseURL + "/jwks",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(doc)
	})

	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		keys := map[string]any{"keys": []any{f.jwk()}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(keys)
	})

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		fn := f.tokenFn
		f.mu.Unlock()
		if fn != nil {
			fn(w, r)
			return
		}
		// Default: return valid id_token for client "default-client".
		idTok := f.signedIDToken(f.srv.URL, "default-client", "default-sub", map[string]any{
			"email": "default@example.com",
		})
		resp := map[string]any{
			"access_token": "fake-access",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     idTok,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// oidcCtx injects the fake server's HTTP client so go-oidc and oauth2 use it.
func (f *fakeOIDCServer) oidcCtx(ctx context.Context) context.Context {
	return gooidc.ClientContext(ctx, f.srv.Client())
}

// jwk returns the public key as a JWK map for the /jwks endpoint.
func (f *fakeOIDCServer) jwk() map[string]any {
	n := base64.RawURLEncoding.EncodeToString(f.privKey.PublicKey.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(f.privKey.PublicKey.E)).Bytes())
	return map[string]any{
		"kty": "RSA", "alg": "RS256", "use": "sig",
		"kid": "test-key", "n": n, "e": e,
	}
}

// signedIDToken creates a valid RS256 ID token signed with the fake server's private key.
func (f *fakeOIDCServer) signedIDToken(issuer, clientID, sub string, extra map[string]any) string {
	claims := jwtlib.MapClaims{
		"iss": issuer,
		"aud": []string{clientID},
		"sub": sub,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	for k, v := range extra {
		claims[k] = v
	}
	tok := jwtlib.NewWithClaims(jwtlib.SigningMethodRS256, claims)
	tok.Header["kid"] = "test-key"
	signed, err := tok.SignedString(f.privKey)
	if err != nil {
		panic("fakeOIDCServer.signedIDToken: " + err.Error())
	}
	return signed
}

// setTokenFn sets the /token handler under a mutex to avoid data races.
func (f *fakeOIDCServer) setTokenFn(fn func(w http.ResponseWriter, r *http.Request)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokenFn = fn
}

// ─── stringClaim tests ────────────────────────────────────────────────────────

func TestStringClaim_Present(t *testing.T) {
	t.Parallel()
	claims := map[string]interface{}{"email": "user@example.com"}
	assert.Equal(t, "user@example.com", stringClaim(claims, "email"))
}

func TestStringClaim_Missing(t *testing.T) {
	t.Parallel()
	claims := map[string]interface{}{}
	assert.Equal(t, "", stringClaim(claims, "email"))
}

func TestStringClaim_NonStringValue(t *testing.T) {
	t.Parallel()
	// A non-string claim (e.g. a numeric iat) must not panic and returns "".
	claims := map[string]interface{}{"iat": 1234567890}
	assert.Equal(t, "", stringClaim(claims, "iat"))
}

// ─── stringSliceClaim tests ───────────────────────────────────────────────────

func TestStringSliceClaim_SliceOfInterface(t *testing.T) {
	t.Parallel()
	// Standard JSON-decoded []interface{} form.
	claims := map[string]interface{}{
		"groups": []interface{}{"engineering", "leads"},
	}
	got := stringSliceClaim(claims, "groups")
	assert.Equal(t, []string{"engineering", "leads"}, got)
}

func TestStringSliceClaim_SliceOfString(t *testing.T) {
	t.Parallel()
	claims := map[string]interface{}{
		"groups": []string{"engineering"},
	}
	got := stringSliceClaim(claims, "groups")
	assert.Equal(t, []string{"engineering"}, got)
}

func TestStringSliceClaim_Missing(t *testing.T) {
	t.Parallel()
	claims := map[string]interface{}{}
	assert.Nil(t, stringSliceClaim(claims, "groups"))
}

func TestStringSliceClaim_MixedTypes(t *testing.T) {
	t.Parallel()
	// Non-string items in the slice are silently dropped.
	claims := map[string]interface{}{
		"groups": []interface{}{"engineering", 42, "leads"},
	}
	got := stringSliceClaim(claims, "groups")
	assert.Equal(t, []string{"engineering", "leads"}, got)
}

func TestStringSliceClaim_WrongType(t *testing.T) {
	t.Parallel()
	// A scalar claim where a slice is expected returns nil, not a panic.
	claims := map[string]interface{}{"groups": "not-a-slice"}
	assert.Nil(t, stringSliceClaim(claims, "groups"))
}

// ─── HTTPExchanger cache tests ─────────────────────────────────────────────

func TestHTTPExchanger_NewReturnsEmptyCache(t *testing.T) {
	t.Parallel()
	e := NewHTTPExchanger()
	assert.NotNil(t, e)
	assert.Empty(t, e.providers)
}

func TestHTTPExchanger_CompileTimeCheck(t *testing.T) {
	t.Parallel()
	// Verifies the compile-time assertion at the bottom of exchanger.go holds.
	var _ OIDCTokenExchanger = (*HTTPExchanger)(nil)
}

// ─── getOrCreateProvider tests ───────────────────────────────────────────────

func TestGetOrCreateProvider_CacheHit_DiscoveryCalledOnce(t *testing.T) {
	t.Parallel()
	srv := newFakeOIDCServer(t)
	ex := NewHTTPExchanger()
	ctx := srv.oidcCtx(context.Background())

	p1, err := ex.getOrCreateProvider(ctx, srv.srv.URL)
	require.NoError(t, err)
	p2, err := ex.getOrCreateProvider(ctx, srv.srv.URL)
	require.NoError(t, err)

	assert.Same(t, p1, p2, "second call must return the cached provider pointer")
	assert.Equal(t, int32(1), srv.discoveryCalls.Load(),
		"discovery endpoint must be called exactly once for the same issuer")
}

func TestGetOrCreateProvider_DiscoveryFail_ReturnsError(t *testing.T) {
	t.Parallel()
	// Server returns HTTP 500 for the discovery endpoint — go-oidc must fail.
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(failSrv.Close)

	ex := NewHTTPExchanger()
	ctx := gooidc.ClientContext(context.Background(), failSrv.Client())
	_, err := ex.getOrCreateProvider(ctx, failSrv.URL)
	assert.Error(t, err)
}

// ─── Exchange tests ───────────────────────────────────────────────────────────

func TestExchange_Success_ReturnsClaims(t *testing.T) {
	t.Parallel()
	srv := newFakeOIDCServer(t)

	const clientID = "my-app-client"
	srv.setTokenFn(func(w http.ResponseWriter, _ *http.Request) {
		idTok := srv.signedIDToken(srv.srv.URL, clientID, "user-external-42", map[string]any{
			"email":  "alice@example.com",
			"name":   "Alice Smith",
			"groups": []any{"engineering", "leads"},
		})
		resp := map[string]any{
			"access_token": "fake-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     idTok,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	ex := NewHTTPExchanger()
	cfg := &OIDCConfig{
		IssuerURL:        srv.srv.URL,
		ClientID:         clientID,
		ClientSecret:     "client-secret",
		Scopes:           []string{"openid", "email", "profile"},
		EmailClaim:       "email",
		DisplayNameClaim: "name",
		GroupsClaim:      "groups",
	}

	claims, err := ex.Exchange(srv.oidcCtx(context.Background()), cfg, "auth-code-xyz", "https://app.example.com/callback", "")
	require.NoError(t, err)
	require.NotNil(t, claims)

	assert.Equal(t, "user-external-42", claims.Subject)
	assert.Equal(t, "alice@example.com", claims.Email)
	assert.Equal(t, "Alice Smith", claims.DisplayName)
	assert.Equal(t, []string{"engineering", "leads"}, claims.Groups)
}

func TestExchange_DiscoveryFail_ReturnsExchangeError(t *testing.T) {
	t.Parallel()
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(failSrv.Close)

	ex := NewHTTPExchanger()
	cfg := &OIDCConfig{
		IssuerURL:    failSrv.URL,
		ClientID:     "client",
		ClientSecret: "secret",
	}
	ctx := gooidc.ClientContext(context.Background(), failSrv.Client())

	_, err := ex.Exchange(ctx, cfg, "code", "https://app.example.com/callback", "")
	assert.ErrorIs(t, err, ErrOIDCTokenExchange)
}

func TestExchange_CodeExchangeFail_ReturnsExchangeError(t *testing.T) {
	t.Parallel()
	srv := newFakeOIDCServer(t)
	srv.setTokenFn(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"authorization code expired"}`))
	})

	ex := NewHTTPExchanger()
	cfg := &OIDCConfig{
		IssuerURL:    srv.srv.URL,
		ClientID:     "client",
		ClientSecret: "secret",
	}

	_, err := ex.Exchange(srv.oidcCtx(context.Background()), cfg, "expired-code", "https://app.example.com/callback", "")
	assert.ErrorIs(t, err, ErrOIDCTokenExchange)
}

func TestExchange_MissingIDToken_ReturnsExchangeError(t *testing.T) {
	t.Parallel()
	srv := newFakeOIDCServer(t)
	srv.setTokenFn(func(w http.ResponseWriter, _ *http.Request) {
		// Token response with no id_token field.
		resp := map[string]any{
			"access_token": "fake-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	ex := NewHTTPExchanger()
	cfg := &OIDCConfig{
		IssuerURL:    srv.srv.URL,
		ClientID:     "client",
		ClientSecret: "secret",
	}

	_, err := ex.Exchange(srv.oidcCtx(context.Background()), cfg, "code", "https://app.example.com/callback", "")
	assert.ErrorIs(t, err, ErrOIDCTokenExchange)
}

func TestExchange_BadIDTokenSignature_ReturnsExchangeError(t *testing.T) {
	t.Parallel()
	srv := newFakeOIDCServer(t)
	const clientID = "client"

	srv.setTokenFn(func(w http.ResponseWriter, _ *http.Request) {
		// Sign with a different key — the JWKS public key won't validate this token.
		wrongKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			http.Error(w, "keygen failed", http.StatusInternalServerError)
			return
		}
		claims := jwtlib.MapClaims{
			"iss": srv.srv.URL,
			"aud": []string{clientID},
			"sub": "sub",
			"exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(),
		}
		tok := jwtlib.NewWithClaims(jwtlib.SigningMethodRS256, claims)
		tok.Header["kid"] = "test-key" // claims to be the registered key, but isn't
		badIDTok, _ := tok.SignedString(wrongKey)

		resp := map[string]any{
			"access_token": "fake",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     badIDTok,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	ex := NewHTTPExchanger()
	cfg := &OIDCConfig{
		IssuerURL:    srv.srv.URL,
		ClientID:     clientID,
		ClientSecret: "secret",
	}

	_, err := ex.Exchange(srv.oidcCtx(context.Background()), cfg, "code", "https://app.example.com/callback", "")
	assert.ErrorIs(t, err, ErrOIDCTokenExchange)
}
