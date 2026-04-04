package auth

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Resolver selects the correct Provider based on the tenant's auth configuration.
type Resolver struct {
	local      *LocalProvider
	oidc       *OIDCProvider
	oidcConfig OIDCConfigStore
}

// NewResolver creates a provider resolver.
func NewResolver(local *LocalProvider, oidc *OIDCProvider, oidcConfig OIDCConfigStore) *Resolver {
	return &Resolver{
		local:      local,
		oidc:       oidc,
		oidcConfig: oidcConfig,
	}
}

// Resolve returns the appropriate Provider for the given tenant.
// If the tenant has an OIDC configuration, returns the OIDC provider.
// Otherwise, returns the local provider.
func (r *Resolver) Resolve(ctx context.Context, tenantID uuid.UUID) (Provider, error) {
	// Check if tenant has OIDC config
	cfg, err := r.oidcConfig.GetOIDCConfig(ctx, tenantID)
	if err == nil && cfg != nil {
		return r.oidc, nil
	}

	// Default to local auth
	return r.local, nil
}

// Authenticate is a convenience method that resolves the provider and authenticates.
func (r *Resolver) Authenticate(ctx context.Context, req AuthRequest) (*AuthResult, error) {
	// If OIDC fields are present, use OIDC provider directly
	if req.AuthorizationCode != "" {
		return r.oidc.Authenticate(ctx, req)
	}

	// Otherwise resolve based on tenant config
	provider, err := r.Resolve(ctx, req.TenantID)
	if err != nil {
		return nil, fmt.Errorf("auth: resolve provider: %w", err)
	}

	return provider.Authenticate(ctx, req)
}
