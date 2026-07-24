package auth

import "errors"

var (
	// ErrInvalidCredentials is returned when email/password authentication fails.
	ErrInvalidCredentials = errors.New("auth: invalid email or password")

	// ErrAccountDeactivated is returned when a deactivated user attempts to login.
	ErrAccountDeactivated = errors.New("auth: account is deactivated")

	// ErrAccountInvited is returned when a user hasn't accepted their invitation yet.
	ErrAccountInvited = errors.New("auth: account is pending invitation acceptance")

	// ErrTenantSuspended is returned when the tenant is suspended.
	ErrTenantSuspended = errors.New("auth: tenant is suspended")

	// ErrTokenExpired is returned when a JWT has expired.
	ErrTokenExpired = errors.New("auth: token has expired")

	// ErrTokenInvalid is returned when a JWT is malformed or has an invalid signature.
	ErrTokenInvalid = errors.New("auth: token is invalid")

	// ErrRefreshTokenNotFound is returned when the refresh token doesn't exist in the store.
	ErrRefreshTokenNotFound = errors.New("auth: refresh token not found")

	// ErrSessionRevoked is returned when a refresh token was issued before a
	// revoke-all (password change, deactivation, role revocation).
	ErrSessionRevoked = errors.New("auth: session revoked")

	// ErrOIDCConfigMissing is returned when a tenant has no OIDC configuration.
	ErrOIDCConfigMissing = errors.New("auth: OIDC configuration not found for tenant")

	// ErrOIDCTokenExchange is returned when the authorization code exchange fails.
	ErrOIDCTokenExchange = errors.New("auth: OIDC token exchange failed")

	// ErrOIDCClaimMapping is returned when required claims are missing from the ID token.
	ErrOIDCClaimMapping = errors.New("auth: required claims missing from ID token")
)
