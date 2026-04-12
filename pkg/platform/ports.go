package platform

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/wegofwd2020/thittam/pkg/audit"
)

// UserStore manages platform user persistence.
type UserStore interface {
	// CreateUser creates a new platform user.
	CreateUser(ctx context.Context, email, displayName, passwordHash string, role Role, createdBy uuid.UUID) (uuid.UUID, error)

	// GetUserByEmail returns a platform user by email.
	GetUserByEmail(ctx context.Context, email string) (*PlatformUser, error)

	// GetUserByID returns a platform user by ID.
	GetUserByID(ctx context.Context, id uuid.UUID) (*PlatformUser, error)

	// ListUsers returns all active platform users.
	ListUsers(ctx context.Context) ([]PlatformUser, error)

	// DeactivateUser deactivates a platform user.
	DeactivateUser(ctx context.Context, id uuid.UUID) error

	// UpdateLastLogin records the last login timestamp.
	UpdateLastLogin(ctx context.Context, id uuid.UUID) error
}

// TenantManager provides tenant management operations for platform admins.
type TenantManager interface {
	// ListTenants returns all tenants with summary info.
	ListTenants(ctx context.Context) ([]TenantSummary, error)

	// SuspendTenant suspends a tenant (blocks all user logins).
	SuspendTenant(ctx context.Context, tenantID uuid.UUID) error

	// ReactivateTenant reactivates a suspended tenant.
	ReactivateTenant(ctx context.Context, tenantID uuid.UUID) error

	// UpgradePlan changes a tenant's subscription plan.
	UpgradePlan(ctx context.Context, tenantID uuid.UUID, newPlan string) error
}

// ImpersonationStore persists and manages impersonation sessions.
type ImpersonationStore interface {
	// LogImpersonation records a new impersonation session and returns its ID.
	LogImpersonation(ctx context.Context, req ImpersonationRequest, expiresAt time.Time) (uuid.UUID, error)

	// RevokeSession marks an active session as ended with the given reason.
	// Returns ErrSessionNotFound if the session does not exist or is already ended.
	RevokeSession(ctx context.Context, sessionID uuid.UUID, reason RevocationReason) error

	// GetActiveSessionsForUser returns all unexpired, non-revoked sessions where
	// the given userID is the target. Used by revocation triggers (password change,
	// deactivation, MFA modification).
	GetActiveSessionsForUser(ctx context.Context, targetUserID uuid.UUID) ([]ActiveImpersonationSession, error)
}

// AuditSink records security events. Satisfied in production by *audit.Logger.
// Defined here as a narrow interface so pkg/platform does not take a hard
// dependency on the concrete audit.Logger type.
type AuditSink interface {
	LogAction(
		tenantID, actorID uuid.UUID,
		actorEmail string,
		action audit.Action,
		resourceType audit.ResourceType,
		resourceID uuid.UUID,
		oldState, newState interface{},
		metadata map[string]interface{},
	)
}

// ImpersonationNotifier sends post-session notifications to the target user.
// The implementation sends an email informing the user that their account was
// accessed by a platform administrator.
type ImpersonationNotifier interface {
	// NotifyImpersonationEnded sends a notification to the target user after an
	// impersonation session ends, regardless of the revocation reason.
	NotifyImpersonationEnded(ctx context.Context, session ActiveImpersonationSession, reason RevocationReason) error
}

// noopNotifier is used when no notifier is configured (e.g. in tests).
type noopNotifier struct{}

func (noopNotifier) NotifyImpersonationEnded(_ context.Context, _ ActiveImpersonationSession, _ RevocationReason) error {
	return nil
}

// VerticalManager provides vertical definition management for platform admins.
type VerticalManager interface {
	// ListVerticals returns all vertical definitions (active and inactive).
	ListVerticals(ctx context.Context) ([]VerticalSummary, error)

	// DeactivateVertical deactivates a vertical (blocks new registrations).
	DeactivateVertical(ctx context.Context, id string) error
}

// VerticalSummary provides an overview of a vertical definition.
type VerticalSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	IsActive    bool   `json:"is_active"`
	TenantCount int    `json:"tenant_count"`
}
