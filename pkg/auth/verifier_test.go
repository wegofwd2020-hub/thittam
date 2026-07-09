package auth

import (
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

func TestVerifier_ValidToken(t *testing.T) {
	t.Parallel()
	key, pub := testKeyPair(t)
	v, err := NewVerifier(pub)
	require.NoError(t, err)

	claims, err := v.Verify(signRS256(t, key, time.Now().Add(time.Hour)))
	require.NoError(t, err)
	assert.Equal(t, "user@example.com", claims.Email)
	assert.Equal(t, []string{"member", "tenant_admin"}, claims.Roles)
	assert.NotEqual(t, uuid.Nil, claims.Subject)
	assert.NotEqual(t, uuid.Nil, claims.TenantID)
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
