package platform

import (
	"context"

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
