package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testKeyPair(t *testing.T) (*rsa.PrivateKey, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	return key, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

func signRS256(t *testing.T, key *rsa.PrivateKey, exp time.Time) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, &jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.New().String(),
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		TenantID: uuid.New().String(),
		Email:    "user@example.com",
		Roles:    []string{"member", "tenant_admin"},
	})
	s, err := tok.SignedString(key)
	require.NoError(t, err)
	return s
}

// TestVerifier_ValidToken asserts every field the claims mapping produces.
// It builds the token directly (rather than via signRS256) so it can pin
// exact expected values for Subject/TenantID/IssuedAt/ExpiresAt instead of
// merely checking non-zero — a mapping that silently dropped a field (e.g.
// Permissions) would still pass a partial assertion.
func TestVerifier_ValidToken(t *testing.T) {
	t.Parallel()
	key, pub := testKeyPair(t)
	v, err := NewVerifier(pub)
	require.NoError(t, err)

	subject := uuid.New()
	tenantID := uuid.New()
	issuedAt := time.Now().Add(-time.Minute).Truncate(time.Second)
	expiresAt := time.Now().Add(time.Hour).Truncate(time.Second)

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, &jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject.String(),
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
		TenantID:    tenantID.String(),
		Email:       "user@example.com",
		Roles:       []string{"member", "tenant_admin"},
		Permissions: []string{"budget:read", "budget:approve"},
		AuthMethod:  ProviderLocal,
	})
	s, err := tok.SignedString(key)
	require.NoError(t, err)

	claims, err := v.Verify(s)
	require.NoError(t, err)
	assert.Equal(t, subject, claims.Subject)
	assert.Equal(t, tenantID, claims.TenantID)
	assert.Equal(t, "user@example.com", claims.Email)
	assert.Equal(t, []string{"member", "tenant_admin"}, claims.Roles)
	assert.Equal(t, []string{"budget:read", "budget:approve"}, claims.Permissions)
	assert.Equal(t, ProviderLocal, claims.AuthMethod)
	assert.True(t, issuedAt.Equal(claims.IssuedAt), "IssuedAt: want %v, got %v", issuedAt, claims.IssuedAt)
	assert.True(t, expiresAt.Equal(claims.ExpiresAt), "ExpiresAt: want %v, got %v", expiresAt, claims.ExpiresAt)
}

func TestVerifier_Expired(t *testing.T) {
	t.Parallel()
	key, pub := testKeyPair(t)
	v, _ := NewVerifier(pub)
	_, err := v.Verify(signRS256(t, key, time.Now().Add(-time.Minute)))
	assert.ErrorIs(t, err, ErrTokenExpired)
}

func TestVerifier_WrongKey(t *testing.T) {
	t.Parallel()
	attacker, _ := testKeyPair(t)
	_, pub := testKeyPair(t)
	v, _ := NewVerifier(pub)
	_, err := v.Verify(signRS256(t, attacker, time.Now().Add(time.Hour)))
	assert.ErrorIs(t, err, ErrTokenInvalid)
}

// A token with alg:none must never be accepted. jwt/v5 refuses to sign with
// "none" unless handed the sentinel key, which is exactly how an attacker
// crafts one.
//
// NOTE: this test (and TestVerifier_HMACConfusion_Rejected below) would
// still pass with the `t.Method.(*jwt.SigningMethodRSA)` type assertion
// removed from Verify's keyfunc. golang-jwt/v5 independently refuses both
// forgeries downstream of the keyfunc: SigningMethodNone.Verify only
// succeeds when handed the jwt.UnsafeAllowNoneSignatureType sentinel key,
// and this keyfunc always returns v.publicKey (an *rsa.PublicKey) — never
// that sentinel — so alg:none fails regardless of the guard. Likewise
// SigningMethodHMAC.Verify (hmac.go) type-asserts the supplied key as
// []byte; handed an *rsa.PublicKey it returns ErrInvalidKeyType before ever
// comparing a MAC, again independent of the guard. So neither test is a
// regression guard for that assertion — they exercise defense in depth
// already present in the library. Do not read a pass here as proof the
// guard line works; if you remove the guard, these two tests stay green.
func TestVerifier_AlgNone_Rejected(t *testing.T) {
	t.Parallel()
	_, pub := testKeyPair(t)
	v, _ := NewVerifier(pub)

	tok := jwt.NewWithClaims(jwt.SigningMethodNone, &jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.New().String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		TenantID: uuid.New().String(),
	})
	s, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	_, verr := v.Verify(s)
	assert.ErrorIs(t, verr, ErrTokenInvalid)
}

// Algorithm confusion: sign with HMAC using the PUBLIC key bytes as the shared
// secret. The public key is not secret, so a verifier that trusts the token's
// own `alg` header would accept this. This is the classic RS256->HS256 attack.
func TestVerifier_HMACConfusion_Rejected(t *testing.T) {
	t.Parallel()
	_, pub := testKeyPair(t)
	v, _ := NewVerifier(pub)

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, &jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.New().String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		TenantID: uuid.New().String(),
		Roles:    []string{"platform_admin"},
	})
	s, err := tok.SignedString(pub) // the public key, as an HMAC secret
	require.NoError(t, err)

	_, verr := v.Verify(s)
	assert.ErrorIs(t, verr, ErrTokenInvalid)
}

func TestVerifier_Garbage(t *testing.T) {
	t.Parallel()
	_, pub := testKeyPair(t)
	v, _ := NewVerifier(pub)
	for _, s := range []string{"", "not-a-token", "a.b.c"} {
		_, err := v.Verify(s)
		assert.ErrorIs(t, err, ErrTokenInvalid, "input %q", s)
	}
}

func TestNewVerifier_BadPEM(t *testing.T) {
	t.Parallel()
	for _, pem := range [][]byte{nil, []byte(""), []byte("-----BEGIN PUBLIC KEY-----\ngarbage\n-----END PUBLIC KEY-----")} {
		_, err := NewVerifier(pem)
		assert.Error(t, err)
	}
}

// A PRIVATE key PEM must be rejected: services must never be handed one, and
// silently accepting it would hide a catastrophic misconfiguration.
func TestNewVerifier_RejectsPrivateKeyPEM(t *testing.T) {
	t.Parallel()
	key, _ := testKeyPair(t)
	der := x509.MarshalPKCS1PrivateKey(key)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
	_, err := NewVerifier(privPEM)
	assert.Error(t, err)
}

// A PKCS#8 "PRIVATE KEY" PEM must be rejected too. NewVerifier's private-key
// guard was previously exercised only via the PKCS#1 "RSA PRIVATE KEY" form;
// PKCS#8 is a distinct, equally common encoding and has its own block type
// ("PRIVATE KEY"), which falls through the type switch's default case.
func TestNewVerifier_RejectsPKCS8PrivateKeyPEM(t *testing.T) {
	t.Parallel()
	key, _ := testKeyPair(t)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	_, err = NewVerifier(privPEM)
	assert.Error(t, err)
}

// A PKIX public key that isn't RSA (e.g. ECDSA) must be rejected with an
// error identifying it as non-RSA, and must not panic on the type assertion
// in the "PUBLIC KEY" branch of NewVerifier.
func TestNewVerifier_RejectsNonRSAPKIXKey(t *testing.T) {
	t.Parallel()
	assert.NotPanics(t, func() {
		ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		der, err := x509.MarshalPKIXPublicKey(&ecKey.PublicKey)
		require.NoError(t, err)
		ecPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})

		v, err := NewVerifier(ecPEM)
		require.Error(t, err)
		assert.Nil(t, v)
		assert.Contains(t, err.Error(), "not RSA")
	})
}

// A token with no exp claim at all must never be accepted. jwt.WithExpirationRequired
// only rejects the absence of exp during parsing; this test confirms that
// path actually rejects it (rather than, say, defaulting to zero-value time
// and appearing "already expired" or, worse, "never expires") and that the
// rejection surfaces as the same ErrTokenInvalid used elsewhere — not a
// distinct error an attacker could use as an oracle to learn "this token has
// no exp" versus "this token is otherwise malformed".
func TestVerifier_NoExpClaim(t *testing.T) {
	t.Parallel()
	key, pub := testKeyPair(t)
	v, err := NewVerifier(pub)
	require.NoError(t, err)

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": uuid.New().String(),
		"tid": uuid.New().String(),
		"iat": float64(time.Now().Unix()),
		// exp intentionally omitted.
	})
	s, err := tok.SignedString(key)
	require.NoError(t, err)

	_, verr := v.Verify(s)
	assert.ErrorIs(t, verr, ErrTokenInvalid)
	assert.NotErrorIs(t, verr, ErrTokenExpired)
}

// TestVerifier_NoIssuedAt is the regression test for the nil-pointer panic
// fixed in Verify's claims mapping: c.IssuedAt is a *jwt.NumericDate and iat
// is not a required claim (only exp is, via jwt.WithExpirationRequired()),
// so a validly-signed token that omits iat must verify successfully with a
// zero-value IssuedAt rather than panicking on `c.IssuedAt.Time`.
//
// Confirmed manually: reverting the nil guard in verifier.go and running
// this test panics with "runtime error: invalid memory address or nil
// pointer dereference" inside Verify; with the guard restored it passes.
func TestVerifier_NoIssuedAt(t *testing.T) {
	t.Parallel()
	key, pub := testKeyPair(t)
	v, err := NewVerifier(pub)
	require.NoError(t, err)

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, &jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.New().String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			// IssuedAt intentionally left nil.
		},
		TenantID: uuid.New().String(),
	})
	s, err := tok.SignedString(key)
	require.NoError(t, err)

	claims, err := v.Verify(s)
	require.NoError(t, err)
	assert.True(t, claims.IssuedAt.IsZero(), "IssuedAt should be the zero time when iat is absent, got %v", claims.IssuedAt)
}

// TestVerifier_FutureNotBefore pins the actual behaviour of a not-yet-valid
// token rather than assuming it: golang-jwt/v5 validates nbf whenever the
// claim is present (even though this verifier never requires it), so a
// token with a future nbf is rejected — surfaced as the same ErrTokenInvalid
// as any other validation failure, not a distinct "not yet valid" signal.
func TestVerifier_FutureNotBefore(t *testing.T) {
	t.Parallel()
	key, pub := testKeyPair(t)
	v, err := NewVerifier(pub)
	require.NoError(t, err)

	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub": uuid.New().String(),
		"tid": uuid.New().String(),
		"iat": float64(now.Unix()),
		"exp": float64(now.Add(2 * time.Hour).Unix()),
		"nbf": float64(now.Add(time.Hour).Unix()), // not valid for another hour
	})
	s, err := tok.SignedString(key)
	require.NoError(t, err)

	_, verr := v.Verify(s)
	assert.ErrorIs(t, verr, ErrTokenInvalid, "observed behaviour: golang-jwt/v5 rejects a future nbf as ErrTokenNotValidYet, mapped here to ErrTokenInvalid")
}
