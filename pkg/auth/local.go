package auth

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// UserStore provides user lookup for authentication.
type UserStore interface {
	// GetUserByEmail returns the user for authentication.
	// Returns ErrInvalidCredentials if not found.
	GetUserByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*UserRecord, error)

	// GetUserByID returns the user by ID (used for refresh token validation).
	GetUserByID(ctx context.Context, tenantID, userID uuid.UUID) (*UserRecord, error)

	// CreateOIDCUser creates a JIT-provisioned user on first OIDC login.
	CreateOIDCUser(ctx context.Context, tenantID uuid.UUID, email, displayName string) (*UserRecord, error)
}

// UserRecord is the minimal user data needed for authentication.
type UserRecord struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	Email        string
	DisplayName  string
	PasswordHash string // empty for OIDC-only users
	Status       string // "active", "invited", "deactivated"
	Roles        []string
	Permissions  []string
}

// TenantStore provides tenant lookup for authentication.
type TenantStore interface {
	// GetTenantStatus returns the tenant's status.
	GetTenantStatus(ctx context.Context, tenantID uuid.UUID) (string, error)
}

// PasswordVerifier verifies passwords against hashes.
type PasswordVerifier interface {
	Verify(password, hash string) error
}

// LocalProvider implements email/password authentication against the local database.
type LocalProvider struct {
	users    UserStore
	tenants  TenantStore
	verifier PasswordVerifier
}

// NewLocalProvider creates a local authentication provider.
func NewLocalProvider(users UserStore, tenants TenantStore, verifier PasswordVerifier) *LocalProvider {
	return &LocalProvider{
		users:    users,
		tenants:  tenants,
		verifier: verifier,
	}
}

// Type returns ProviderLocal.
func (p *LocalProvider) Type() ProviderType {
	return ProviderLocal
}

// Authenticate validates email and password against the local database.
func (p *LocalProvider) Authenticate(ctx context.Context, req AuthRequest) (*AuthResult, error) {
	// Check tenant status
	status, err := p.tenants.GetTenantStatus(ctx, req.TenantID)
	if err != nil {
		return nil, fmt.Errorf("auth: check tenant: %w", err)
	}
	if status == "suspended" {
		return nil, ErrTenantSuspended
	}

	// Look up user
	user, err := p.users.GetUserByEmail(ctx, req.TenantID, req.Email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	// Check user status
	switch user.Status {
	case "deactivated":
		return nil, ErrAccountDeactivated
	case "invited":
		return nil, ErrAccountInvited
	}

	// Verify password
	if user.PasswordHash == "" {
		// OIDC-only user trying local login
		return nil, ErrInvalidCredentials
	}
	if err := p.verifier.Verify(req.Password, user.PasswordHash); err != nil {
		return nil, ErrInvalidCredentials
	}

	return &AuthResult{
		UserID:      user.ID,
		TenantID:    user.TenantID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Roles:       user.Roles,
		Permissions: user.Permissions,
		AuthMethod:  ProviderLocal,
	}, nil
}

// Compile-time check.
var _ Provider = (*LocalProvider)(nil)
