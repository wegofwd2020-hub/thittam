package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	// DefaultAccessTTL is how long an access token is valid.
	DefaultAccessTTL = 15 * time.Minute

	// DefaultRefreshTTL is how long a refresh token is valid.
	DefaultRefreshTTL = 7 * 24 * time.Hour

	// refreshKeyPrefix is the Redis key namespace for refresh tokens.
	refreshKeyPrefix = "iam:refresh:"

	// usergenKeyPrefix namespaces the per-user token generation counter.
	// Bumping a user's generation invalidates every refresh token issued
	// before the bump (#154).
	usergenKeyPrefix = "iam:usergen:"

	// tenantgenKeyPrefix namespaces the per-tenant token generation counter.
	// Bumping a tenant's generation invalidates every refresh token issued to any
	// of its users before the bump (#182).
	tenantgenKeyPrefix = "iam:tenantgen:"
)

// jwtClaims is the JWT payload — maps 1-to-1 with auth.Claims but uses the
// registered "sub" claim name for the user ID so standard JWT tools work.
type jwtClaims struct {
	jwt.RegisteredClaims

	TenantID    string       `json:"tid"`
	Email       string       `json:"email"`
	Roles       []string     `json:"roles"`
	Permissions []string     `json:"perms"`
	AuthMethod  ProviderType `json:"auth_method"`
}

// refreshPayload is stored in Redis under each refresh token key.
// It carries enough to re-issue an access token without a database round-trip.
type refreshPayload struct {
	UserID      uuid.UUID    `json:"user_id"`
	TenantID    uuid.UUID    `json:"tenant_id"`
	Email       string       `json:"email"`
	Roles       []string     `json:"roles"`
	Permissions []string     `json:"permissions"`
	AuthMethod  ProviderType `json:"auth_method"`

	// Generation is the user's token generation at issue time. Refresh compares
	// it against the live counter and rejects on ANY difference (#154).
	Generation int64 `json:"generation"`

	// TenantGeneration is the tenant's token generation at issue time; Refresh
	// rejects on ANY difference, revoking every session in a suspended tenant (#182).
	TenantGeneration int64 `json:"tenant_generation"`
}

// generations is the {user, tenant} token-generation pair embedded at issue time
// and re-validated on refresh. Refresh carries the already-validated pair forward
// (never re-reading) — the #154 TOCTOU fix, now across both dimensions.
type generations struct {
	User   int64
	Tenant int64
}

// JWTIssuer implements TokenIssuer using:
//   - RS256 JWTs for access tokens (signed with an RSA private key)
//   - 32-byte random hex tokens stored in Redis for refresh tokens
type JWTIssuer struct {
	privateKey *rsa.PrivateKey
	rdb        redis.Cmdable
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// JWTConfig holds tunable parameters for JWTIssuer.
type JWTConfig struct {
	// AccessTTL is the access token lifetime. Defaults to DefaultAccessTTL.
	AccessTTL time.Duration
	// RefreshTTL is the refresh token lifetime. Defaults to DefaultRefreshTTL.
	RefreshTTL time.Duration
}

// NewJWTIssuer creates a JWTIssuer from an RSA private key (PEM-encoded) and
// a Redis client. The key bytes are consumed here and never stored — the caller
// should zero them after this call.
//
// Accepts PKCS#1 (BEGIN RSA PRIVATE KEY) and PKCS#8 (BEGIN PRIVATE KEY) PEM blocks.
func NewJWTIssuer(privateKeyPEM []byte, rdb redis.Cmdable, cfg JWTConfig) (*JWTIssuer, error) {
	if len(privateKeyPEM) == 0 {
		return nil, errors.New("auth: jwt: private key PEM is empty")
	}

	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return nil, errors.New("auth: jwt: failed to decode PEM block")
	}

	var privateKey *rsa.PrivateKey
	switch block.Type {
	case "RSA PRIVATE KEY": // PKCS#1
		var err error
		privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("auth: jwt: parse PKCS#1 key: %w", err)
		}
	case "PRIVATE KEY": // PKCS#8
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("auth: jwt: parse PKCS#8 key: %w", err)
		}
		var ok bool
		privateKey, ok = key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("auth: jwt: PKCS#8 key is not RSA")
		}
	default:
		return nil, fmt.Errorf("auth: jwt: unsupported PEM block type %q", block.Type)
	}

	if cfg.AccessTTL == 0 {
		cfg.AccessTTL = DefaultAccessTTL
	}
	if cfg.RefreshTTL == 0 {
		cfg.RefreshTTL = DefaultRefreshTTL
	}

	return &JWTIssuer{
		privateKey: privateKey,
		rdb:        rdb,
		accessTTL:  cfg.AccessTTL,
		refreshTTL: cfg.RefreshTTL,
	}, nil
}

// Issue creates a new TokenPair for the given AuthResult.
// The access token is an RS256 JWT. The refresh token is a 32-byte random hex
// string stored in Redis.
func (j *JWTIssuer) Issue(ctx context.Context, result *AuthResult) (*TokenPair, error) {
	userGen, err := j.currentGeneration(ctx, result.UserID)
	if err != nil {
		return nil, err
	}
	tenantGen, err := j.currentTenantGeneration(ctx, result.TenantID)
	if err != nil {
		return nil, err
	}
	return j.issueWithGeneration(ctx, result, generations{User: userGen, Tenant: tenantGen})
}

// issueWithGeneration is Issue's body with the generation pair supplied by the
// caller instead of read fresh. Refresh uses this to embed the generations it
// already validated against, so a revocation landing mid-refresh cannot be
// baked into the new token (#154 TOCTOU: a bare Issue call re-reads the
// counters, reopening the window between Refresh's compare-read and re-issue).
func (j *JWTIssuer) issueWithGeneration(ctx context.Context, result *AuthResult, gens generations) (*TokenPair, error) {
	now := time.Now().UTC()
	accessExp := now.Add(j.accessTTL)

	claims := jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   result.UserID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(accessExp),
		},
		TenantID:    result.TenantID.String(),
		Email:       result.Email,
		Roles:       result.Roles,
		Permissions: result.Permissions,
		AuthMethod:  result.AuthMethod,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	accessToken, err := token.SignedString(j.privateKey)
	if err != nil {
		return nil, fmt.Errorf("auth: jwt: sign access token: %w", err)
	}

	refreshToken, err := j.issueRefreshToken(ctx, &refreshPayload{
		UserID:           result.UserID,
		TenantID:         result.TenantID,
		Email:            result.Email,
		Roles:            result.Roles,
		Permissions:      result.Permissions,
		AuthMethod:       result.AuthMethod,
		Generation:       gens.User,
		TenantGeneration: gens.Tenant,
	})
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(j.accessTTL.Seconds()),
		ExpiresAt:    accessExp,
	}, nil
}

// Refresh validates the refresh token, revokes it, and issues a new TokenPair.
// Refresh tokens are single-use — a new one is issued on every refresh.
func (j *JWTIssuer) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	payload, err := j.consumeRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, err
	}

	userGen, err := j.currentGeneration(ctx, payload.UserID)
	if err != nil {
		return nil, err
	}
	tenantGen, err := j.currentTenantGeneration(ctx, payload.TenantID)
	if err != nil {
		return nil, err
	}
	// Deliberately `!=`, not `<`, on BOTH dimensions (see #154). The counter key
	// can vanish (TTL, flush, failover) and a missing key reads as 0; under `<`
	// a stale token with a higher generation would be ACCEPTED, un-revoking
	// sessions exactly when the store is least healthy. Any divergence fails
	// closed.
	if payload.Generation != userGen || payload.TenantGeneration != tenantGen {
		return nil, ErrSessionRevoked
	}

	// Must call issueWithGeneration with the pair already validated above, NOT
	// Issue. Issue re-reads the live counters; if a RevokeAllForUser or
	// RevokeAllForTenant INCR lands between the compare-read above and that
	// re-read, the re-read would win the race and the new token would carry the
	// POST-revoke generation forever — silently defeating revocation for
	// whoever holds this refresh token (#154 TOCTOU, reproduced). Carrying the
	// pair forward closes the window. Do not "simplify" this back to
	// j.Issue(ctx, ...).
	return j.issueWithGeneration(ctx, &AuthResult{
		UserID:          payload.UserID,
		TenantID:        payload.TenantID,
		Email:           payload.Email,
		Roles:           payload.Roles,
		Permissions:     payload.Permissions,
		AuthMethod:      payload.AuthMethod,
		AuthenticatedAt: time.Now().UTC(),
	}, generations{User: userGen, Tenant: tenantGen})
}

// Revoke deletes the refresh token from Redis, invalidating the session.
func (j *JWTIssuer) Revoke(ctx context.Context, refreshToken string) error {
	key := refreshKeyPrefix + refreshToken
	deleted, err := j.rdb.Del(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("auth: jwt: revoke refresh token: %w", err)
	}
	if deleted == 0 {
		return ErrRefreshTokenNotFound
	}
	return nil
}

// RevokeAllForUser invalidates every outstanding refresh token for a user by
// bumping their generation counter. O(1) regardless of session count.
//
// Access tokens already issued remain valid until they expire (accessTTL) —
// this bounds a compromised session at the refresh boundary, not instantly.
func (j *JWTIssuer) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	key := usergenKeyPrefix + userID.String()
	// EXPIRE alongside INCR is pure housekeeping — it bounds how long a stale
	// counter lingers in Redis, not a correctness mechanism (see the `!=`
	// comparison in Refresh, which fails closed regardless of whether the key
	// is present). It is set here only, never refreshed by Issue/Refresh, so a
	// user revoked long ago who logs in again near the TTL boundary could see
	// one spurious forced re-login later when the counter expires and a new
	// baseline generation is established. Rare, fails closed, not worth an
	// extra write on every issue to close.
	if _, err := j.rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Incr(ctx, key)
		pipe.Expire(ctx, key, j.refreshTTL*2)
		return nil
	}); err != nil {
		return fmt.Errorf("auth: jwt: revoke all sessions: %w", err)
	}
	return nil
}

// RevokeAllForTenant invalidates every outstanding refresh token for every user in
// a tenant by bumping the tenant generation counter. O(1) regardless of user count
// (suspension, #182). Access tokens remain valid until they expire (accessTTL).
func (j *JWTIssuer) RevokeAllForTenant(ctx context.Context, tenantID uuid.UUID) error {
	key := tenantgenKeyPrefix + tenantID.String()
	if _, err := j.rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Incr(ctx, key)
		pipe.Expire(ctx, key, j.refreshTTL*2)
		return nil
	}); err != nil {
		return fmt.Errorf("auth: jwt: revoke all tenant sessions: %w", err)
	}
	return nil
}

// Validate parses and verifies an access token, returning its claims.
// Returns ErrTokenExpired or ErrTokenInvalid on failure.
func (j *JWTIssuer) Validate(ctx context.Context, accessToken string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(
		accessToken,
		&jwtClaims{},
		func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("auth: jwt: unexpected signing method %v", t.Header["alg"])
			}
			return &j.privateKey.PublicKey, nil
		},
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrTokenInvalid
	}

	c, ok := token.Claims.(*jwtClaims)
	if !ok || !token.Valid {
		return nil, ErrTokenInvalid
	}

	userID, err := uuid.Parse(c.Subject)
	if err != nil {
		return nil, ErrTokenInvalid
	}
	tenantID, err := uuid.Parse(c.TenantID)
	if err != nil {
		return nil, ErrTokenInvalid
	}

	var issuedAt, expiresAt time.Time
	if c.IssuedAt != nil {
		issuedAt = c.IssuedAt.Time
	}
	if c.ExpiresAt != nil {
		expiresAt = c.ExpiresAt.Time
	}

	return &Claims{
		Subject:     userID,
		TenantID:    tenantID,
		Email:       c.Email,
		Roles:       c.Roles,
		Permissions: c.Permissions,
		AuthMethod:  c.AuthMethod,
		IssuedAt:    issuedAt,
		ExpiresAt:   expiresAt,
	}, nil
}

// --- helpers ---

// issueRefreshToken generates a random token, stores the payload in Redis, and
// returns the token string.
func (j *JWTIssuer) issueRefreshToken(ctx context.Context, payload *refreshPayload) (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", fmt.Errorf("auth: jwt: generate refresh token: %w", err)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("auth: jwt: marshal refresh payload: %w", err)
	}

	key := refreshKeyPrefix + token
	if err := j.rdb.Set(ctx, key, data, j.refreshTTL).Err(); err != nil {
		return "", fmt.Errorf("auth: jwt: store refresh token: %w", err)
	}

	return token, nil
}

// consumeRefreshToken atomically reads and deletes the refresh token from Redis.
// Returns ErrRefreshTokenNotFound if the token does not exist or has expired.
func (j *JWTIssuer) consumeRefreshToken(ctx context.Context, token string) (*refreshPayload, error) {
	key := refreshKeyPrefix + token

	// GET then DEL inside a pipeline for atomicity on the happy path.
	// A race between two concurrent refreshes with the same token is benign:
	// the second will get ErrRefreshTokenNotFound and the client must re-login.
	var getCmd *redis.StringCmd
	_, err := j.rdb.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		getCmd = pipe.Get(ctx, key)
		pipe.Del(ctx, key)
		return nil
	})
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("auth: jwt: consume refresh token: %w", err)
	}

	data, err := getCmd.Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrRefreshTokenNotFound
		}
		return nil, fmt.Errorf("auth: jwt: read refresh payload: %w", err)
	}

	var payload refreshPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("auth: jwt: unmarshal refresh payload: %w", err)
	}

	return &payload, nil
}

// currentGeneration returns the user's token generation, 0 if never bumped.
func (j *JWTIssuer) currentGeneration(ctx context.Context, userID uuid.UUID) (int64, error) {
	gen, err := j.rdb.Get(ctx, usergenKeyPrefix+userID.String()).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("auth: jwt: read token generation: %w", err)
	}
	return gen, nil
}

// currentTenantGeneration returns the tenant's token generation, 0 if never bumped.
func (j *JWTIssuer) currentTenantGeneration(ctx context.Context, tenantID uuid.UUID) (int64, error) {
	gen, err := j.rdb.Get(ctx, tenantgenKeyPrefix+tenantID.String()).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("auth: jwt: read tenant generation: %w", err)
	}
	return gen, nil
}

// generateToken produces a 32-byte cryptographically random hex token.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

// Compile-time interface check.
var _ TokenIssuer = (*JWTIssuer)(nil)
