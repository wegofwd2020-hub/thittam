// Package auth implements dual-path authentication for Thittam.
// Path A: local database (email/password + bcrypt).
// Path B: external identity provider (OIDC).
// Both paths produce identical AuthResults that the TokenIssuer converts to JWTs.
package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ProviderType identifies the authentication method.
type ProviderType string

const (
	ProviderLocal ProviderType = "local"
	ProviderOIDC  ProviderType = "oidc"
)

// Provider authenticates users via a specific method.
type Provider interface {
	// Authenticate validates credentials and returns user identity.
	Authenticate(ctx context.Context, req AuthRequest) (*AuthResult, error)

	// Type returns the provider type.
	Type() ProviderType
}

// AuthRequest carries the input for authentication.
// Only the fields relevant to the provider type are populated.
type AuthRequest struct {
	// Common
	TenantID uuid.UUID

	// Local auth
	Email    string
	Password string

	// OIDC auth
	AuthorizationCode string
	RedirectURI       string
	CodeVerifier      string // PKCE
}

// AuthResult is the output of successful authentication.
// The TokenIssuer uses this to produce a JWT.
type AuthResult struct {
	UserID       uuid.UUID
	TenantID     uuid.UUID
	Email        string
	DisplayName  string
	Roles        []string
	Permissions  []string
	AuthMethod   ProviderType
	IsNewUser    bool      // true if JIT-provisioned (OIDC first login)
	AuthenticatedAt time.Time
}
