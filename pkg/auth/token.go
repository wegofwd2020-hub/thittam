package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// TokenPair contains an access token and a refresh token.
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"` // always "Bearer"
	ExpiresIn    int       `json:"expires_in"` // seconds
	ExpiresAt    time.Time `json:"expires_at"`
}

// Claims represents the JWT payload. Identical regardless of auth method.
type Claims struct {
	Subject     uuid.UUID    `json:"sub"`
	TenantID    uuid.UUID    `json:"tid"`
	Email       string       `json:"email"`
	Roles       []string     `json:"roles"`
	Permissions []string     `json:"perms"`
	AuthMethod  ProviderType `json:"auth_method"`
	IssuedAt    time.Time    `json:"iat"`
	ExpiresAt   time.Time    `json:"exp"`
}

// TokenIssuer creates and manages JWT token pairs.
type TokenIssuer interface {
	// Issue creates a new TokenPair from an AuthResult.
	Issue(ctx context.Context, result *AuthResult) (*TokenPair, error)

	// Refresh validates a refresh token and issues a new TokenPair.
	Refresh(ctx context.Context, refreshToken string) (*TokenPair, error)

	// Revoke invalidates a refresh token (logout).
	Revoke(ctx context.Context, refreshToken string) error

	// RevokeAllForUser invalidates every outstanding refresh token for a user
	// (password change, deactivation, role revocation — #154).
	RevokeAllForUser(ctx context.Context, userID uuid.UUID) error

	// RevokeAllForTenant invalidates every outstanding refresh token for every user
	// in a tenant (tenant suspension — #182).
	RevokeAllForTenant(ctx context.Context, tenantID uuid.UUID) error

	// Validate parses and validates an access token, returning its claims.
	Validate(ctx context.Context, accessToken string) (*Claims, error)
}
