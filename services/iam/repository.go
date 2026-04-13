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
	GetRoleByID(ctx context.Context, tenantID, roleID uuid.UUID) (*Role, error)
	ListRoles(ctx context.Context, tenantID uuid.UUID) ([]Role, error)
	AssignRole(ctx context.Context, ur *UserRole) error
	RevokeRole(ctx context.Context, userID, roleID uuid.UUID) error
	// GetUserPermissions returns the union of permissions held by the user.
	// If projectID is nil, only tenant-wide assignments (project_id IS NULL) contribute.
	// If projectID is non-nil, tenant-wide assignments are merged with project-scoped
	// assignments for that specific project.
	GetUserPermissions(ctx context.Context, userID uuid.UUID, projectID *uuid.UUID) ([]string, error)

	// Invitations
	CreateInvitation(ctx context.Context, inv *Invitation) error
	GetInvitationByToken(ctx context.Context, token string) (*Invitation, error)
	MarkInvitationAccepted(ctx context.Context, id uuid.UUID) error

	// OIDC configuration
	// UpsertOIDCConfig creates or replaces the OIDC configuration for a tenant.
	// ClientSecretEnc in params must be pre-encrypted by the caller.
	UpsertOIDCConfig(ctx context.Context, params OIDCConfigParams) error

	// Impersonation lifecycle
	// StartImpersonation opens a new bounded-TTL impersonation session.
	StartImpersonation(ctx context.Context, params StartImpersonationParams) (*ImpersonationSession, error)
	// EndImpersonationSession marks a session as explicitly ended by setting ended_at = NOW().
	// Returns ErrImpersonationNotFound if the session does not exist.
	// Returns ErrImpersonationAlreadyEnded if ended_at is already set.
	EndImpersonationSession(ctx context.Context, sessionID uuid.UUID) error
	// ExpireImpersonationSessions sets ended_at = NOW() on all sessions whose
	// expires_at < NOW() and ended_at IS NULL. Returns the count of rows updated.
	// Called by the background expiry ticker in cmd/iam/main.go.
	ExpireImpersonationSessions(ctx context.Context) (int64, error)

	// Audit log — append-only. Rows must never be updated or deleted (Rule #7).
	// CreateAuditEntry inserts a single audit record. If the write fails the
	// caller must log the error but must not roll back the triggering operation —
	// audit failures should surface as observability alerts, not user errors.
	CreateAuditEntry(ctx context.Context, entry *AuditEntry) error
}
