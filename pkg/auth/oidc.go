package auth

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// OIDCConfig holds the OIDC configuration for a tenant.
type OIDCConfig struct {
	TenantID     uuid.UUID `json:"tenant_id"`
	IssuerURL    string    `json:"issuer_url"`    // e.g., "https://accounts.google.com"
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret"` // encrypted at rest
	Scopes       []string  `json:"scopes"`        // default: ["openid", "email", "profile"]

	// Claim mapping (IdP claim name → Thittam field)
	EmailClaim       string `json:"email_claim"`        // default: "email"
	DisplayNameClaim string `json:"display_name_claim"` // default: "name"
	GroupsClaim      string `json:"groups_claim"`       // optional: maps IdP groups to Thittam roles

	// Behaviour
	AutoProvision bool `json:"auto_provision"` // JIT create user on first login
	DefaultRole   string `json:"default_role"`  // role assigned to JIT-provisioned users
}

// OIDCConfigStore retrieves OIDC configuration per tenant.
type OIDCConfigStore interface {
	GetOIDCConfig(ctx context.Context, tenantID uuid.UUID) (*OIDCConfig, error)
}

// OIDCTokenExchanger handles the OAuth2 authorization code exchange.
// This is separated to allow mocking in tests without a real IdP.
type OIDCTokenExchanger interface {
	// Exchange trades an authorization code for an ID token.
	// Returns the decoded claims from the ID token.
	Exchange(ctx context.Context, cfg *OIDCConfig, code, redirectURI, codeVerifier string) (*OIDCClaims, error)
}

// OIDCClaims represents the relevant claims extracted from an ID token.
type OIDCClaims struct {
	Subject     string   // IdP's user identifier (sub claim)
	Email       string
	DisplayName string
	Groups      []string // optional; used for role mapping
}

// OIDCProvider implements OIDC-based authentication.
type OIDCProvider struct {
	configs   OIDCConfigStore
	exchanger OIDCTokenExchanger
	users     UserStore
	tenants   TenantStore
}

// NewOIDCProvider creates an OIDC authentication provider.
func NewOIDCProvider(configs OIDCConfigStore, exchanger OIDCTokenExchanger, users UserStore, tenants TenantStore) *OIDCProvider {
	return &OIDCProvider{
		configs:   configs,
		exchanger: exchanger,
		users:     users,
		tenants:   tenants,
	}
}

// Type returns ProviderOIDC.
func (p *OIDCProvider) Type() ProviderType {
	return ProviderOIDC
}

// Authenticate exchanges an authorization code for an ID token, maps claims to a
// Thittam user, and optionally JIT-provisions a new user on first login.
func (p *OIDCProvider) Authenticate(ctx context.Context, req AuthRequest) (*AuthResult, error) {
	// Check tenant status
	status, err := p.tenants.GetTenantStatus(ctx, req.TenantID)
	if err != nil {
		return nil, fmt.Errorf("auth: check tenant: %w", err)
	}
	if status == "suspended" {
		return nil, ErrTenantSuspended
	}

	// Load OIDC configuration for this tenant
	cfg, err := p.configs.GetOIDCConfig(ctx, req.TenantID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOIDCConfigMissing, err)
	}

	// Exchange authorization code for ID token claims
	claims, err := p.exchanger.Exchange(ctx, cfg, req.AuthorizationCode, req.RedirectURI, req.CodeVerifier)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOIDCTokenExchange, err)
	}

	if claims.Email == "" {
		return nil, fmt.Errorf("%w: email claim is empty", ErrOIDCClaimMapping)
	}

	// Look up existing user
	user, err := p.users.GetUserByEmail(ctx, req.TenantID, claims.Email)
	if err != nil {
		// User doesn't exist — JIT provision if enabled
		if !cfg.AutoProvision {
			return nil, ErrInvalidCredentials
		}

		displayName := claims.DisplayName
		if displayName == "" {
			displayName = claims.Email
		}

		user, err = p.users.CreateOIDCUser(ctx, req.TenantID, claims.Email, displayName)
		if err != nil {
			return nil, fmt.Errorf("auth: JIT provision user: %w", err)
		}

		return &AuthResult{
			UserID:      user.ID,
			TenantID:    user.TenantID,
			Email:       user.Email,
			DisplayName: user.DisplayName,
			Roles:       user.Roles,
			Permissions: user.Permissions,
			AuthMethod:  ProviderOIDC,
			IsNewUser:   true,
		}, nil
	}

	// Existing user
	if user.Status == "deactivated" {
		return nil, ErrAccountDeactivated
	}

	return &AuthResult{
		UserID:      user.ID,
		TenantID:    user.TenantID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Roles:       user.Roles,
		Permissions: user.Permissions,
		AuthMethod:  ProviderOIDC,
		IsNewUser:   false,
	}, nil
}

// Compile-time check.
var _ Provider = (*OIDCProvider)(nil)
