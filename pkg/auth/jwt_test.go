package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	jwtlib "github.com/golang-jwt/jwt/v5"
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

// testIssuerWithClient is testIssuer but also returns the *redis.Client
// backing the issuer, so a test can install a redis.Hook on it (needed for
// the #154 TOCTOU regression test below). Does not replace testIssuer —
// other tests depend on that helper's signature.
func testIssuerWithClient(t *testing.T) (*JWTIssuer, *miniredis.Miniredis, *redis.Client) {
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
	return issuer, mr, rdb
}

// midRefreshRevokeHook is a redis.Hook that, the first time it observes a GET
// against a specific "iam:usergen:<uid>" key, bumps that same key (INCR)
// immediately after the real GET completes and before control returns to the
// caller. This reproduces the #154 TOCTOU window: a RevokeAllForUser landing
// exactly between Refresh's compare-read and the generation re-read that used
// to happen inside Issue. It fires at most once (guarded by `fired`) so it
// models a single revocation event, not a storm of them.
type midRefreshRevokeHook struct {
	rdb   *redis.Client
	key   string
	fired atomic.Bool
}

func (h *midRefreshRevokeHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h *midRefreshRevokeHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		err := next(ctx, cmd)
		if cmd.Name() == "get" {
			if args := cmd.Args(); len(args) >= 2 {
				if key, ok := args[1].(string); ok && key == h.key {
					if h.fired.CompareAndSwap(false, true) {
						h.rdb.Incr(ctx, h.key)
					}
				}
			}
		}
		return err
	}
}

func (h *midRefreshRevokeHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

// TestJWTIssuer_Refresh_TOCTOU_RevokeMidRefresh reproduces #154's TOCTOU: a
// RevokeAllForUser INCR landing between Refresh's compare-read of the
// generation counter and the re-issue. Before the fix, Refresh delegated to
// the public Issue, which re-read the counter and embedded the POST-revoke
// generation into the freshly issued refresh token — permanently un-revoking
// the session the revoke was meant to kill. After the fix, Refresh carries
// forward the generation it already validated, so the new token still embeds
// the STALE (pre-revoke) generation and the very next refresh must be
// rejected with ErrSessionRevoked.
//
// Verified this test fails against the pre-fix code (Refresh calling the
// public Issue, which re-reads the counter): the second Refresh below
// succeeded instead of returning ErrSessionRevoked. See fix-report.md.
func TestJWTIssuer_Refresh_TOCTOU_RevokeMidRefresh(t *testing.T) {
	issuer, _, rdb := testIssuerWithClient(t)
	ctx := context.Background()

	// Issue the original token BEFORE installing the hook — Issue also reads
	// the generation counter, and we only want the hook to fire on the read
	// that happens inside the upcoming Refresh call, not this one.
	original, err := issuer.Issue(ctx, fixtureAuthResult())
	require.NoError(t, err)

	hook := &midRefreshRevokeHook{
		rdb: rdb,
		key: usergenKeyPrefix + fixtureUserID.String(),
	}
	rdb.AddHook(hook)

	// This Refresh's internal compare-read observes generation 0 (matching the
	// token) and is allowed to proceed; the hook then bumps the live counter
	// to 1 immediately afterward, simulating a RevokeAllForUser that lands in
	// the TOCTOU window. Refresh itself must still succeed — the token being
	// refreshed right now was valid at the moment it was validated.
	refreshed, err := issuer.Refresh(ctx, original.RefreshToken)
	require.NoError(t, err, "the in-flight refresh must complete using the generation it already validated")
	require.True(t, hook.fired.Load(), "test setup bug: the race hook never observed the expected GET")

	// The critical assertion: the token minted by that refresh must NOT
	// survive against the now-bumped counter. If it embedded the post-revoke
	// generation (the pre-fix bug), this next refresh would succeed instead.
	_, err = issuer.Refresh(ctx, refreshed.RefreshToken)
	assert.ErrorIs(t, err, ErrSessionRevoked,
		"a refresh token minted during the TOCTOU window must not survive the revocation that raced it")
}

func fixtureAuthResult() *AuthResult {
	return &AuthResult{
		UserID:          fixtureUserID,
		TenantID:        fixtureTenantID,
		Email:           "user@example.com",
		Roles:           []string{"coordinator"},
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
	assert.Equal(t, []string{"coordinator"}, claims.Roles)
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

func TestJWTIssuer_Validate_MissingIat_NoPanic(t *testing.T) {
	issuer, _ := testIssuer(t)

	// A validly-signed token that omits iat (only exp is parser-required).
	claims := &jwtClaims{
		RegisteredClaims: jwtlib.RegisteredClaims{
			Subject:   fixtureUserID.String(),
			ExpiresAt: jwtlib.NewNumericDate(time.Now().Add(time.Hour)),
			// IssuedAt deliberately omitted.
		},
		TenantID: fixtureTenantID.String(),
	}
	tokenStr, err := jwtlib.NewWithClaims(jwtlib.SigningMethodRS256, claims).SignedString(issuer.privateKey)
	require.NoError(t, err)

	got, err := issuer.Validate(context.Background(), tokenStr)
	require.NoError(t, err)
	assert.True(t, got.IssuedAt.IsZero(), "missing iat must map to zero time, not panic")
	assert.False(t, got.ExpiresAt.IsZero())
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

func TestJWTIssuer_Validate_WrongSigningMethod_ReturnsInvalid(t *testing.T) {
	t.Parallel()
	issuer, _ := testIssuer(t)

	// Create an HMAC-signed token — Validate's key function rejects non-RSA algorithms.
	hmacClaims := jwtlib.MapClaims{
		"sub": fixtureUserID.String(),
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	hmacToken := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, hmacClaims)
	tokenStr, err := hmacToken.SignedString([]byte("hmac-secret"))
	require.NoError(t, err)

	_, err = issuer.Validate(context.Background(), tokenStr)
	assert.ErrorIs(t, err, ErrTokenInvalid)
}

func TestJWTIssuer_Issue_RedisSetError_ReturnsError(t *testing.T) {
	t.Parallel()
	issuer, mr := testIssuer(t)
	mr.Close() // close before Issue so Redis Set fails

	_, err := issuer.Issue(context.Background(), fixtureAuthResult())
	assert.Error(t, err, "Issue must fail when Redis Set returns an error")
}

func TestJWTIssuer_Refresh_RedisPipelineError_ReturnsError(t *testing.T) {
	t.Parallel()
	issuer, mr := testIssuer(t)
	ctx := context.Background()

	// Issue a valid pair first so the token exists in Redis.
	pair, err := issuer.Issue(ctx, fixtureAuthResult())
	require.NoError(t, err)

	// Close miniredis so the Pipelined GET+DEL fails with a connection error.
	mr.Close()

	_, err = issuer.Refresh(ctx, pair.RefreshToken)
	assert.Error(t, err)
	// Must NOT be ErrRefreshTokenNotFound — the token existed; this is an infrastructure error.
	assert.NotErrorIs(t, err, ErrRefreshTokenNotFound)
}

func TestNewJWTIssuer_UnsupportedPEMType_ReturnsError(t *testing.T) {
	t.Parallel()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	// "EC PRIVATE KEY" is not handled — must hit the default case and return an error.
	ecPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: []byte("fake-ec-key-bytes"),
	})
	_, err = NewJWTIssuer(ecPEM, rdb, JWTConfig{})
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

func TestJWTIssuer_RevokeAllForUser_RejectsPriorRefreshToken(t *testing.T) {
	iss, _ := testIssuer(t)
	ctx := context.Background()

	pair, err := iss.Issue(ctx, &AuthResult{UserID: fixtureUserID, TenantID: fixtureTenantID, Email: "u@example.com"})
	require.NoError(t, err)

	require.NoError(t, iss.RevokeAllForUser(ctx, fixtureUserID))

	_, err = iss.Refresh(ctx, pair.RefreshToken)
	require.Error(t, err, "a refresh token issued before revoke-all must be rejected")
}

// A token issued AFTER the revocation must work — revoke-all must not wedge
// the user out permanently.
func TestJWTIssuer_RevokeAllForUser_NewTokenStillWorks(t *testing.T) {
	iss, _ := testIssuer(t)
	ctx := context.Background()

	require.NoError(t, iss.RevokeAllForUser(ctx, fixtureUserID))

	pair, err := iss.Issue(ctx, &AuthResult{UserID: fixtureUserID, TenantID: fixtureTenantID, Email: "u@example.com"})
	require.NoError(t, err)
	_, err = iss.Refresh(ctx, pair.RefreshToken)
	require.NoError(t, err)
}

// THE test that pins `!=` over `<`. With the counter key deleted (TTL expiry,
// flush, cold-replica failover) a stale token must still be rejected. Written
// against `<` this fails, because 1 < 0 is false and the token is accepted.
func TestJWTIssuer_RevokeAllForUser_CounterResetStillRejects(t *testing.T) {
	iss, mr := testIssuer(t)
	ctx := context.Background()

	// Establish a nonzero baseline generation first. Without this, the token
	// issued below would carry generation 0 — identical to what a missing key
	// reads as after the Del below — and the assertion would pass (or fail)
	// identically under `!=` and `<`, defeating the point of this test. A
	// nonzero embedded generation is required to actually distinguish the two
	// operators (see the `1 < 0` arithmetic below).
	require.NoError(t, iss.RevokeAllForUser(ctx, fixtureUserID))

	pair, err := iss.Issue(ctx, &AuthResult{UserID: fixtureUserID, TenantID: fixtureTenantID, Email: "u@example.com"})
	require.NoError(t, err)
	require.NoError(t, iss.RevokeAllForUser(ctx, fixtureUserID))

	// Simulate the counter disappearing while the refresh token is still alive.
	mr.Del(usergenKeyPrefix + fixtureUserID.String())

	_, err = iss.Refresh(ctx, pair.RefreshToken)
	require.Error(t, err, "a reset counter must not resurrect a revoked session")
}

// Revoking one user must not touch another's sessions.
func TestJWTIssuer_RevokeAllForUser_ScopedToUser(t *testing.T) {
	iss, _ := testIssuer(t)
	ctx := context.Background()
	other := uuid.MustParse("a1000000-0000-0000-0000-0000000000ff")

	pair, err := iss.Issue(ctx, &AuthResult{UserID: other, TenantID: fixtureTenantID, Email: "o@example.com"})
	require.NoError(t, err)

	require.NoError(t, iss.RevokeAllForUser(ctx, fixtureUserID))

	_, err = iss.Refresh(ctx, pair.RefreshToken)
	require.NoError(t, err, "revoking one user must not revoke another's sessions")
}

// --- RevokeAllForTenant (#182) ---

// RevokeAllForTenant rejects a refresh token minted before the bump.
func TestJWTIssuer_RevokeAllForTenant_RejectsPriorRefreshToken(t *testing.T) {
	iss, _ := testIssuer(t)
	ctx := context.Background()

	pair, err := iss.Issue(ctx, &AuthResult{UserID: fixtureUserID, TenantID: fixtureTenantID, Email: "u@example.com"})
	require.NoError(t, err)

	require.NoError(t, iss.RevokeAllForTenant(ctx, fixtureTenantID))

	_, err = iss.Refresh(ctx, pair.RefreshToken)
	require.Error(t, err, "a refresh token issued before tenant revoke-all must be rejected")
	assert.ErrorIs(t, err, ErrSessionRevoked)
}

// A token issued AFTER the tenant revocation must work — revoke-all must not
// wedge the tenant out permanently.
func TestJWTIssuer_RevokeAllForTenant_NewTokenStillWorks(t *testing.T) {
	iss, _ := testIssuer(t)
	ctx := context.Background()

	require.NoError(t, iss.RevokeAllForTenant(ctx, fixtureTenantID))

	pair, err := iss.Issue(ctx, &AuthResult{UserID: fixtureUserID, TenantID: fixtureTenantID, Email: "u@example.com"})
	require.NoError(t, err)
	_, err = iss.Refresh(ctx, pair.RefreshToken)
	require.NoError(t, err)
}

// THE test that pins `!=` over `<` on the tenant dimension. With the counter
// key deleted (TTL expiry, flush, cold-replica failover) a stale token must
// still be rejected.
func TestJWTIssuer_RevokeAllForTenant_CounterResetStillRejects(t *testing.T) {
	iss, mr := testIssuer(t)
	ctx := context.Background()

	// Establish a nonzero baseline generation first — see the analogous usergen
	// test for why this is required to distinguish `!=` from `<`.
	require.NoError(t, iss.RevokeAllForTenant(ctx, fixtureTenantID))

	pair, err := iss.Issue(ctx, &AuthResult{UserID: fixtureUserID, TenantID: fixtureTenantID, Email: "u@example.com"})
	require.NoError(t, err)
	require.NoError(t, iss.RevokeAllForTenant(ctx, fixtureTenantID))

	// Simulate the counter disappearing while the refresh token is still alive.
	mr.Del(tenantgenKeyPrefix + fixtureTenantID.String())

	_, err = iss.Refresh(ctx, pair.RefreshToken)
	require.Error(t, err, "a reset tenant counter must not resurrect a revoked session")
	assert.ErrorIs(t, err, ErrSessionRevoked)
}

// Revoking one tenant must not touch another's sessions.
func TestJWTIssuer_RevokeAllForTenant_ScopedToTenant(t *testing.T) {
	iss, _ := testIssuer(t)
	ctx := context.Background()
	otherTenant := uuid.MustParse("b1000000-0000-0000-0000-0000000000ff")

	pair, err := iss.Issue(ctx, &AuthResult{UserID: fixtureUserID, TenantID: otherTenant, Email: "u@example.com"})
	require.NoError(t, err)

	require.NoError(t, iss.RevokeAllForTenant(ctx, fixtureTenantID))

	_, err = iss.Refresh(ctx, pair.RefreshToken)
	require.NoError(t, err, "revoking one tenant must not revoke another tenant's sessions")
}

// TestJWTIssuer_Refresh_TOCTOU_RevokeTenantMidRefresh mirrors
// TestJWTIssuer_Refresh_TOCTOU_RevokeMidRefresh but on the tenant dimension: a
// RevokeAllForTenant INCR landing between Refresh's compare-read of the tenant
// generation counter and the re-issue must not be baked into the freshly
// issued token.
func TestJWTIssuer_Refresh_TOCTOU_RevokeTenantMidRefresh(t *testing.T) {
	issuer, _, rdb := testIssuerWithClient(t)
	ctx := context.Background()

	original, err := issuer.Issue(ctx, fixtureAuthResult())
	require.NoError(t, err)

	hook := &midRefreshRevokeHook{
		rdb: rdb,
		key: tenantgenKeyPrefix + fixtureTenantID.String(),
	}
	rdb.AddHook(hook)

	refreshed, err := issuer.Refresh(ctx, original.RefreshToken)
	require.NoError(t, err, "the in-flight refresh must complete using the generation it already validated")
	require.True(t, hook.fired.Load(), "test setup bug: the race hook never observed the expected GET")

	_, err = issuer.Refresh(ctx, refreshed.RefreshToken)
	assert.ErrorIs(t, err, ErrSessionRevoked,
		"a refresh token minted during the TOCTOU window must not survive the tenant revocation that raced it")
}

// Back-compat: a refresh payload minted before this change deserializes with
// TenantGeneration == 0 (Go's zero value for a missing JSON field). A
// never-bumped tenant's counter also reads 0, so the comparison passes — no
// forced logout on deploy.
func TestJWTIssuer_Refresh_PreTenantgenPayloadStillWorks(t *testing.T) {
	iss, _ := testIssuer(t)
	ctx := context.Background()

	// Simulate a refresh token minted by pre-#182 code: marshal a JSON payload
	// with no "tenant_generation" field at all, and store it directly under the
	// refresh key exactly as issueRefreshToken would.
	token, err := generateToken()
	require.NoError(t, err)

	legacyPayload := struct {
		UserID      uuid.UUID    `json:"user_id"`
		TenantID    uuid.UUID    `json:"tenant_id"`
		Email       string       `json:"email"`
		Roles       []string     `json:"roles"`
		Permissions []string     `json:"permissions"`
		AuthMethod  ProviderType `json:"auth_method"`
		Generation  int64        `json:"generation"`
	}{
		UserID:     fixtureUserID,
		TenantID:   fixtureTenantID,
		Email:      "u@example.com",
		AuthMethod: ProviderLocal,
		Generation: 0,
	}
	data, err := json.Marshal(legacyPayload)
	require.NoError(t, err)
	require.NoError(t, iss.rdb.Set(ctx, refreshKeyPrefix+token, data, 10*time.Second).Err())

	_, err = iss.Refresh(ctx, token)
	require.NoError(t, err, "a pre-#182 refresh payload (no tenant_generation field) must still refresh against a never-bumped tenant")
}
