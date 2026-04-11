package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Deterministic fixture IDs for test isolation (CODING_RULES §9).
var (
	fixtureUserID   = uuid.MustParse("a1000000-0000-0000-0000-000000000001")
	fixtureTenantID = uuid.MustParse("b1000000-0000-0000-0000-000000000001")
)

// testIssuer creates a JWTIssuer backed by miniredis for use in tests.
// Uses a freshly generated 2048-bit RSA key and short TTLs to keep tests fast.
func testIssuer(t *testing.T) (*JWTIssuer, *miniredis.Miniredis) {
	t.Helper()

	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privKey),
	})

	issuer, err := NewJWTIssuer(keyPEM, rdb, JWTConfig{
		AccessTTL:  5 * time.Second,
		RefreshTTL: 10 * time.Second,
	})
	require.NoError(t, err)
	return issuer, mr
}

func fixtureAuthResult() *AuthResult {
	return &AuthResult{
		UserID:          fixtureUserID,
		TenantID:        fixtureTenantID,
		Email:           "user@example.com",
		Roles:           []string{"line_producer"},
		Permissions:     []string{"budget:read", "expense:submit"},
		AuthMethod:      ProviderLocal,
		AuthenticatedAt: time.Now().UTC(),
	}
}

// --- Issue ---

func TestJWTIssuer_Issue_ReturnsTokenPair(t *testing.T) {
	t.Parallel()
	issuer, _ := testIssuer(t)

	pair, err := issuer.Issue(context.Background(), fixtureAuthResult())
	require.NoError(t, err)

	assert.NotEmpty(t, pair.AccessToken)
	assert.NotEmpty(t, pair.RefreshToken)
	assert.Equal(t, "Bearer", pair.TokenType)
	assert.Equal(t, 5, pair.ExpiresIn) // 5s TTL
	assert.False(t, pair.ExpiresAt.IsZero())
}

func TestJWTIssuer_Issue_RefreshTokensAreDifferent(t *testing.T) {
	t.Parallel()
	issuer, _ := testIssuer(t)
	ctx := context.Background()
	result := fixtureAuthResult()

	p1, err := issuer.Issue(ctx, result)
	require.NoError(t, err)
	p2, err := issuer.Issue(ctx, result)
	require.NoError(t, err)

	assert.NotEqual(t, p1.RefreshToken, p2.RefreshToken)
}

// --- Validate ---

func TestJWTIssuer_Validate_ValidToken(t *testing.T) {
	t.Parallel()
	issuer, _ := testIssuer(t)
	ctx := context.Background()

	pair, err := issuer.Issue(ctx, fixtureAuthResult())
	require.NoError(t, err)

	claims, err := issuer.Validate(ctx, pair.AccessToken)
	require.NoError(t, err)

	assert.Equal(t, fixtureUserID, claims.Subject)
	assert.Equal(t, fixtureTenantID, claims.TenantID)
	assert.Equal(t, "user@example.com", claims.Email)
	assert.Equal(t, []string{"line_producer"}, claims.Roles)
	assert.Equal(t, []string{"budget:read", "expense:submit"}, claims.Permissions)
	assert.Equal(t, ProviderLocal, claims.AuthMethod)
}

func TestJWTIssuer_Validate_ExpiredToken(t *testing.T) {
	t.Parallel()
	// Use a separate issuer with a negative access TTL so the token is
	// already expired at issue time. This avoids any real-clock dependency.
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privKey),
	})
	expiredIssuer, err := NewJWTIssuer(keyPEM, rdb, JWTConfig{
		AccessTTL:  -time.Second, // already expired at issue time
		RefreshTTL: 10 * time.Second,
	})
	require.NoError(t, err)

	pair, err := expiredIssuer.Issue(context.Background(), fixtureAuthResult())
	require.NoError(t, err)

	_, err = expiredIssuer.Validate(context.Background(), pair.AccessToken)
	assert.ErrorIs(t, err, ErrTokenExpired)
}

func TestJWTIssuer_Validate_InvalidToken(t *testing.T) {
	t.Parallel()
	issuer, _ := testIssuer(t)

	_, err := issuer.Validate(context.Background(), "not.a.jwt")
	assert.ErrorIs(t, err, ErrTokenInvalid)
}

func TestJWTIssuer_Validate_TamperedToken(t *testing.T) {
	t.Parallel()
	issuer, _ := testIssuer(t)
	ctx := context.Background()

	pair, err := issuer.Issue(ctx, fixtureAuthResult())
	require.NoError(t, err)

	// Replace several bytes in the middle of the signature (3rd JWT segment)
	// to ensure the RSA verification fails regardless of base64 edge cases.
	parts := strings.Split(pair.AccessToken, ".")
	require.Len(t, parts, 3, "expected 3 JWT segments")
	sig := []byte(parts[2])
	// Flip bytes near the middle of the signature.
	mid := len(sig) / 2
	for i := mid; i < mid+4 && i < len(sig); i++ {
		sig[i] ^= 0xFF
	}
	parts[2] = string(sig)
	tampered := strings.Join(parts, ".")

	_, err = issuer.Validate(ctx, tampered)
	assert.ErrorIs(t, err, ErrTokenInvalid)
}

// --- Refresh ---

func TestJWTIssuer_Refresh_IssuasNewPair(t *testing.T) {
	t.Parallel()
	issuer, _ := testIssuer(t)
	ctx := context.Background()

	original, err := issuer.Issue(ctx, fixtureAuthResult())
	require.NoError(t, err)

	refreshed, err := issuer.Refresh(ctx, original.RefreshToken)
	require.NoError(t, err)

	assert.NotEmpty(t, refreshed.AccessToken)
	assert.NotEmpty(t, refreshed.RefreshToken)
	// Refresh tokens must always be unique (random). Access tokens may be equal
	// if both are issued within the same second (JWT uses second-precision timestamps
	// and RS256 is deterministic for the same input).
	assert.NotEqual(t, original.RefreshToken, refreshed.RefreshToken)
}

func TestJWTIssuer_Refresh_TokenIsConsumedOnUse(t *testing.T) {
	t.Parallel()
	issuer, _ := testIssuer(t)
	ctx := context.Background()

	pair, err := issuer.Issue(ctx, fixtureAuthResult())
	require.NoError(t, err)

	// First refresh: should succeed.
	_, err = issuer.Refresh(ctx, pair.RefreshToken)
	require.NoError(t, err)

	// Second refresh with the same token: must fail — token is single-use.
	_, err = issuer.Refresh(ctx, pair.RefreshToken)
	assert.ErrorIs(t, err, ErrRefreshTokenNotFound)
}

func TestJWTIssuer_Refresh_PreservesUserClaims(t *testing.T) {
	t.Parallel()
	issuer, _ := testIssuer(t)
	ctx := context.Background()

	original, err := issuer.Issue(ctx, fixtureAuthResult())
	require.NoError(t, err)

	refreshed, err := issuer.Refresh(ctx, original.RefreshToken)
	require.NoError(t, err)

	claims, err := issuer.Validate(ctx, refreshed.AccessToken)
	require.NoError(t, err)

	assert.Equal(t, fixtureUserID, claims.Subject)
	assert.Equal(t, fixtureTenantID, claims.TenantID)
	assert.Equal(t, "user@example.com", claims.Email)
}

func TestJWTIssuer_Refresh_UnknownToken(t *testing.T) {
	t.Parallel()
	issuer, _ := testIssuer(t)

	_, err := issuer.Refresh(context.Background(), "does-not-exist")
	assert.ErrorIs(t, err, ErrRefreshTokenNotFound)
}

// --- Revoke ---

func TestJWTIssuer_Revoke_PreventsRefresh(t *testing.T) {
	t.Parallel()
	issuer, _ := testIssuer(t)
	ctx := context.Background()

	pair, err := issuer.Issue(ctx, fixtureAuthResult())
	require.NoError(t, err)

	require.NoError(t, issuer.Revoke(ctx, pair.RefreshToken))

	_, err = issuer.Refresh(ctx, pair.RefreshToken)
	assert.ErrorIs(t, err, ErrRefreshTokenNotFound)
}

func TestJWTIssuer_Revoke_UnknownToken(t *testing.T) {
	t.Parallel()
	issuer, _ := testIssuer(t)

	err := issuer.Revoke(context.Background(), "ghost-token")
	assert.ErrorIs(t, err, ErrRefreshTokenNotFound)
}

// --- NewJWTIssuer construction ---

func TestNewJWTIssuer_EmptyKeyError(t *testing.T) {
	t.Parallel()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	_, err = NewJWTIssuer(nil, rdb, JWTConfig{})
	assert.Error(t, err)
}

func TestNewJWTIssuer_InvalidPEMError(t *testing.T) {
	t.Parallel()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	_, err = NewJWTIssuer([]byte("not-a-pem-block"), rdb, JWTConfig{})
	assert.Error(t, err)
}

func TestNewJWTIssuer_PKCS8Key(t *testing.T) {
	t.Parallel()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	require.NoError(t, err)

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8Bytes,
	})

	issuer, err := NewJWTIssuer(keyPEM, rdb, JWTConfig{})
	require.NoError(t, err)
	assert.NotNil(t, issuer)
}
