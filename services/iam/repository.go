package iam

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/wegofwd2020/thittam/pkg/auth"
)

// Repository defines all data access required by the IAM service.
// The concrete implementation must also satisfy auth.UserStore and
// auth.TenantStore so it can be passed directly to auth.NewLocalProvider.
type Repository interface {
	// auth.UserStore — used by auth.LocalProvider and auth.OIDCProvider.
	GetUserByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*auth.UserRecord, error)
	// FindTenantByEmail returns the tenant ID for the user with this email,
	// scanning across all tenants. Used by Login when the caller doesn't
	// know which tenant they belong to (e.g. the public login form). Returns
	// ErrUserNotFound if no user matches, or an ambiguity error if multiple
	// tenants have a user with the same email — in that case the caller MUST
	// supply tenant_id explicitly.
	FindTenantByEmail(ctx context.Context, email string) (uuid.UUID, error)
	GetUserByID(ctx context.Context, tenantID, userID uuid.UUID) (*auth.UserRecord, error)
	CreateOIDCUser(ctx context.Context, tenantID uuid.UUID, email, displayName string) (*auth.UserRecord, error)

	// auth.TenantStore — used by auth.LocalProvider.
	GetTenantStatus(ctx context.Context, tenantID uuid.UUID) (string, error)

	// Users
	CreateUser(ctx context.Context, user *User) error
	GetUser(ctx context.Context, tenantID, id uuid.UUID) (*User, error)
	ListUsers(ctx context.Context, tenantID uuid.UUID, status string, limit, offset int) ([]User, error)
	UpdateUser(ctx context.Context, user *User) error
	UpdatePasswordHash(ctx context.Context, tenantID, userID uuid.UUID, hash string) error
	DeactivateUser(ctx context.Context, tenantID, id uuid.UUID) error

	// Tenants
	CreateTenant(ctx context.Context, tenant *Tenant) error
	GetTenant(ctx context.Context, id uuid.UUID) (*Tenant, error)
	// FindTenantByNormalizedName returns the tenant whose name matches the
	// given name under case-insensitive, trimmed, internal-whitespace-collapsed
	// comparison (the tenants_name_ci_unique index). Returns (nil, nil) when no
	// tenant matches. Used by CreateTenant's pre-flight duplicate check (#89).
	FindTenantByNormalizedName(ctx context.Context, name string) (*Tenant, error)
	// UpdateTenantStatus sets the tenant's status and optionally its
	// legal-hold fields (#92 Stage 4). A nil holdUntil / freezeReason
	// preserves the existing column value — the COALESCE semantics
	// protect active holds from being cleared by a bare SuspendTenant
	// retry. Clearing a hold is a separate admin operation
	// (ClearTenantLegalHold).
	UpdateTenantStatus(ctx context.Context, id uuid.UUID, status string, holdUntil *time.Time, freezeReason *string) error
	// ClearTenantLegalHold resets hold_until and freeze_reason to NULL
	// on the tenant row and returns the updated tenant. Status is
	// unchanged. Idempotent: calling on a tenant with no active hold
	// succeeds and returns the row unmodified (#92 Stage 4 pair).
	ClearTenantLegalHold(ctx context.Context, id uuid.UUID) (*Tenant, error)
	// SetTenantLegalHold sets the tenant's hold_until and freeze_reason to the
	// given values and returns the updated row. Status, suspended_at, and
	// deactivated_at are NOT touched — this applies a hold without regressing
	// the retention clock (#119). freezeReason is written verbatim (the caller
	// guarantees it is non-empty); a nil holdUntil writes NULL (indefinite
	// hold). Returns ErrTenantNotFound if the row is missing.
	SetTenantLegalHold(ctx context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason string) (*Tenant, error)
	// UpdateTenantAddress replaces the address + country + currency fields
	// on an existing tenant. Other fields (name, plan, status) are not
	// touched. Returns ErrTenantNotFound if the row is missing.
	UpdateTenantAddress(ctx context.Context, tenant *Tenant) (*Tenant, error)
	// TransitionTenantStatus performs a conditional status update guarded
	// by the current status — `UPDATE ... WHERE status = from`. Returns
	// (tenant, true) on successful transition, or (nil, false, nil) when
	// the current status did not match `from` (idempotency: another
	// worker already advanced the tenant). Used by the retention
	// sweeper (#92).
	TransitionTenantStatus(ctx context.Context, id uuid.UUID, from, to string) (*Tenant, bool, error)
	// ListTenantsDueForLifecycle returns tenants in a transient lifecycle
	// state whose next automatic transition is due at or before `now`.
	// Limit caps the number of rows; callers typically page by re-running
	// with the same `now` until an empty batch is returned (#92).
	ListTenantsDueForLifecycle(ctx context.Context, now time.Time, limit int) ([]*Tenant, error)
	// CountTenantsOnHold returns the number of tenants whose
	// freeze_reason is set — i.e. currently on legal hold regardless
	// of their lifecycle status. Used by the retention sweeper to
	// export a gauge per run (#92 Stage 5).
	CountTenantsOnHold(ctx context.Context) (int64, error)

	// --- Tenant purge (two-person approval, #92 Stage 3) ---
	CreateTenantPurgeRequest(ctx context.Context, req *TenantPurgeRequest) error
	GetOpenTenantPurgeRequest(ctx context.Context, tenantID uuid.UUID) (*TenantPurgeRequest, error)
	ApproveTenantPurgeRequest(ctx context.Context, requestID, approverID uuid.UUID) (*TenantPurgeRequest, error)
	CancelTenantPurgeRequest(ctx context.Context, requestID, cancellerID uuid.UUID) (*TenantPurgeRequest, error)
	ListApprovedTenantPurgeRequests(ctx context.Context, limit int) ([]*TenantPurgeRequest, error)
	MarkTenantPurgeRequestFailed(ctx context.Context, requestID uuid.UUID, reason string) (*TenantPurgeRequest, error)
	// PurgeTenantSchemaAndTombstone hard-deletes a purge_eligible tenant in one
	// transaction: DROP SCHEMA tenant_<uuid> CASCADE, tombstone the tenants row
	// (status='purged', PII nulled, name→sentinel, purged_at=now()), and mark
	// the purge request executed. Owner privileges required (DDL). Returns
	// ErrTenantNotPurgeable if the tenant left purge_eligible (status-guarded).
	PurgeTenantSchemaAndTombstone(ctx context.Context, tenantID, requestID uuid.UUID) error

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
