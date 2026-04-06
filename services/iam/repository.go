package iam

import (
	"context"

	"github.com/google/uuid"
	"github.com/wegofwd2020/thittam/pkg/auth"
)

// Repository defines all data access required by the IAM service.
// The concrete implementation must also satisfy auth.UserStore and
// auth.TenantStore so it can be passed directly to auth.NewLocalProvider.
type Repository interface {
	// auth.UserStore — used by auth.LocalProvider and auth.OIDCProvider.
	GetUserByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*auth.UserRecord, error)
	GetUserByID(ctx context.Context, userID uuid.UUID) (*auth.UserRecord, error)
	CreateOIDCUser(ctx context.Context, tenantID uuid.UUID, email, displayName string) (*auth.UserRecord, error)

	// auth.TenantStore — used by auth.LocalProvider.
	GetTenantStatus(ctx context.Context, tenantID uuid.UUID) (string, error)

	// Users
	CreateUser(ctx context.Context, user *User) error
	GetUser(ctx context.Context, tenantID, id uuid.UUID) (*User, error)
	ListUsers(ctx context.Context, tenantID uuid.UUID, status string, limit, offset int) ([]User, error)
	UpdateUser(ctx context.Context, user *User) error
	UpdatePasswordHash(ctx context.Context, userID uuid.UUID, hash string) error
	DeactivateUser(ctx context.Context, tenantID, id uuid.UUID) error

	// Tenants
	CreateTenant(ctx context.Context, tenant *Tenant) error
	GetTenant(ctx context.Context, id uuid.UUID) (*Tenant, error)
	UpdateTenantStatus(ctx context.Context, id uuid.UUID, status string) error

	// Roles
	CreateRole(ctx context.Context, role *Role) error
	GetRole(ctx context.Context, tenantID uuid.UUID, name string) (*Role, error)
	ListRoles(ctx context.Context, tenantID uuid.UUID) ([]Role, error)
	AssignRole(ctx context.Context, ur *UserRole) error
	RevokeRole(ctx context.Context, userID, roleID uuid.UUID) error
	GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]string, error)

	// Invitations
	CreateInvitation(ctx context.Context, inv *Invitation) error
	GetInvitationByToken(ctx context.Context, token string) (*Invitation, error)
	MarkInvitationAccepted(ctx context.Context, id uuid.UUID) error
}
