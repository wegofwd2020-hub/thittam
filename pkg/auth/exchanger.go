package auth

import (
	"context"
	"fmt"
	"sync"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// HTTPExchanger implements OIDCTokenExchanger using the real OIDC discovery
// protocol. It performs the OAuth2 authorization code exchange and verifies
// the returned ID token against the issuer's JWKS endpoint.
//
// Providers are cached by issuer URL so that the discovery HTTP round-trip
// (/.well-known/openid-configuration) only happens once per issuer.
//
// NOTE: OIDCConfig.ClientSecret is expected to be the plaintext secret.
// The DB stores an AES-256-GCM-encrypted value; the caller is responsible
// for decrypting it before constructing OIDCConfig.
type HTTPExchanger struct {
	mu        sync.Mutex
	providers map[string]*gooidc.Provider // keyed by issuer URL
}

// NewHTTPExchanger creates an exchanger with an empty provider cache.
func NewHTTPExchanger() *HTTPExchanger {
	return &HTTPExchanger{
		providers: make(map[string]*gooidc.Provider),
	}
}

// Exchange performs the authorization code → tokens flow for the given tenant
// configuration. It discovers the IdP endpoints, exchanges the code, verifies
// the ID token signature and audience, and extracts standard claims.
func (e *HTTPExchanger) Exchange(
	ctx context.Context,
	cfg *OIDCConfig,
	code, redirectURI, codeVerifier string,
) (*OIDCClaims, error) {
	provider, err := e.getOrCreateProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("%w: discover issuer %s: %v", ErrOIDCTokenExchange, cfg.IssuerURL, err)
	}

	oauth2Cfg := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  redirectURI,
		Scopes:       cfg.Scopes,
	}

	// Exchange the authorization code, optionally with a PKCE code verifier.
	var exchangeOpts []oauth2.AuthCodeOption
	if codeVerifier != "" {
		exchangeOpts = append(exchangeOpts, oauth2.VerifierOption(codeVerifier))
	}

	token, err := oauth2Cfg.Exchange(ctx, code, exchangeOpts...)
	if err != nil {
		return nil, fmt.Errorf("%w: code exchange: %v", ErrOIDCTokenExchange, err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, fmt.Errorf("%w: id_token missing from token response", ErrOIDCTokenExchange)
	}

	verifier := provider.Verifier(&gooidc.Config{ClientID: cfg.ClientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("%w: id token verification: %v", ErrOIDCTokenExchange, err)
	}

	// Extract claims using the tenant-configured claim names.
	var raw map[string]interface{}
	if err := idToken.Claims(&raw); err != nil {
		return nil, fmt.Errorf("%w: extract claims: %v", ErrOIDCClaimMapping, err)
	}

	claims := &OIDCClaims{
		Subject: idToken.Subject,
		Email:   stringClaim(raw, cfg.EmailClaim),
	}

	if cfg.DisplayNameClaim != "" {
		claims.DisplayName = stringClaim(raw, cfg.DisplayNameClaim)
	}

	if cfg.GroupsClaim != "" {
		claims.Groups = stringSliceClaim(raw, cfg.GroupsClaim)
	}

	return claims, nil
}

// getOrCreateProvider returns a cached OIDC provider or discovers a new one.
func (e *HTTPExchanger) getOrCreateProvider(ctx context.Context, issuerURL string) (*gooidc.Provider, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if p, ok := e.providers[issuerURL]; ok {
		return p, nil
	}

	p, err := gooidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, err
	}
	e.providers[issuerURL] = p
	return p, nil
}

// stringClaim extracts a string value from a raw claims map.
func stringClaim(claims map[string]interface{}, key string) string {
	v, _ := claims[key].(string)
	return v
}

// stringSliceClaim extracts a []string from a raw claims map.
// Handles both []interface{} (JSON arrays) and []string.
func stringSliceClaim(claims map[string]interface{}, key string) []string {
	v, ok := claims[key]
	if !ok {
		return nil
	}
	switch typed := v.(type) {
	case []string:
		return typed
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// Compile-time check.
var _ OIDCTokenExchanger = (*HTTPExchanger)(nil)
