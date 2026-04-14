// Package iam implements the identity and access management service.
package iam

import (
	"time"

	"github.com/google/uuid"
)

// User represents a tenant-scoped user account.
type User struct {
	ID           uuid.UUID `json:"id"`
	TenantID     uuid.UUID `json:"tenant_id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	PasswordHash string    `json:"-"`
	Status       string    `json:"status"` // active, invited, deactivated
	CreatedAt    time.Time `json:"created_at"`
}

// Tenant represents a top-level tenant account.
type Tenant struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Plan      string    `json:"plan"`   // starter, professional, enterprise
	Status    string    `json:"status"` // active, suspended, deactivated
	IsDemo    bool      `json:"is_demo"`
	CreatedAt time.Time `json:"created_at"`

	// Company location (#61). AddressLine2 is optional; the rest are
	// required at registration time but existing demo tenants were
	// backfilled to IN / INR in migration 014.
	AddressLine1        string `json:"address_line1,omitempty"`
	AddressLine2        string `json:"address_line2,omitempty"`
	City                string `json:"city,omitempty"`
	CountryCode         string `json:"country_code"`          // ISO 3166-1 alpha-2
	PostalCode          string `json:"postal_code,omitempty"`
	PrimaryCurrencyCode string `json:"primary_currency_code"` // ISO 4217
}

// Role is a named collection of permissions scoped to a tenant.
type Role struct {
	ID          uuid.UUID `json:"id"`
	TenantID    uuid.UUID `json:"tenant_id"`
	Name        string    `json:"name"`
	Permissions []string  `json:"permissions"`
	IsSystem    bool      `json:"is_system"` // system roles are seeded at tenant creation
}

// UserRole records that a user holds a role within their tenant.
// A nil ProjectID means the assignment is tenant-wide. A non-nil ProjectID
// scopes the role to a single production (ADR-014 Phase 2).
type UserRole struct {
	UserID     uuid.UUID  `json:"user_id"`
	RoleID     uuid.UUID  `json:"role_id"`
	ProjectID  *uuid.UUID `json:"project_id,omitempty"`
	AssignedBy uuid.UUID  `json:"assigned_by"`
	AssignedAt time.Time  `json:"assigned_at"`
}

// OIDCConfigParams carries the fields for creating or replacing a tenant's OIDC
// configuration. ClientSecretEnc must already be AES-256-GCM encrypted before
// being passed to the repository — the service layer is responsible for this.
type OIDCConfigParams struct {
	TenantID         string
	IssuerURL        string
	ClientID         string
	ClientSecretEnc  string   // AES-256-GCM ciphertext (base64url)
	Scopes           []string // defaults to ["openid","email","profile"] if empty
	EmailClaim       string   // defaults to "email"
	DisplayNameClaim string   // defaults to "name"
	GroupsClaim      string   // optional
	AutoProvision    bool
	DefaultRole      string // defaults to "member"
}

// ImpersonationSession records a platform admin actively impersonating a tenant user.
// Sessions are bounded by ExpiresAt; EndedAt is set when the session is explicitly
// terminated (via EndImpersonation) or cleaned up by the background expiry ticker.
type ImpersonationSession struct {
	ID               uuid.UUID  `json:"id"`
	PlatformUserID   uuid.UUID  `json:"platform_user_id"`
	TenantID         uuid.UUID  `json:"tenant_id"`
	ImpersonatedUser uuid.UUID  `json:"impersonated_user"`
	Reason           string     `json:"reason"`
	StartedAt        time.Time  `json:"started_at"`
	ExpiresAt        time.Time  `json:"expires_at"`
	EndedAt          *time.Time `json:"ended_at,omitempty"`
	IPAddress        string     `json:"ip_address,omitempty"`
}

// StartImpersonationParams carries the fields required to open an impersonation session.
// The Duration is capped at 4 hours by the service layer regardless of what the caller
// requests.
type StartImpersonationParams struct {
	PlatformUserID   uuid.UUID
	TenantID         uuid.UUID
	ImpersonatedUser uuid.UUID
	Reason           string
	Duration         time.Duration // desired TTL; service caps at maxImpersonationDuration
	IPAddress        string
}

// AuditEntry is an append-only record of a security-relevant action.
// Fields follow Rule #7: actor_id, action, target_type, target_id,
// timestamp, old_state, new_state. Rows must never be updated or deleted.
type AuditEntry struct {
	ID         uuid.UUID `json:"id"`
	ActorID    uuid.UUID `json:"actor_id"`    // user who performed the action
	Action     string    `json:"action"`      // e.g. "impersonation.start", "impersonation.end"
	TargetType string    `json:"target_type"` // e.g. "impersonation_session", "user"
	TargetID   uuid.UUID `json:"target_id"`
	OldState   string    `json:"old_state,omitempty"`
	NewState   string    `json:"new_state,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	TenantID   uuid.UUID `json:"tenant_id"` // tenant context for the action
	IPAddress  string    `json:"ip_address,omitempty"`
}

// Invitation is a pending email invite for a new user.
type Invitation struct {
	ID        uuid.UUID  `json:"id"`
	TenantID  uuid.UUID  `json:"tenant_id"`
	Email     string     `json:"email"`
	RoleID    *uuid.UUID `json:"role_id,omitempty"` // optional pre-assigned role
	Token     string     `json:"-"`                 // secure random token, never serialised
	Status    string     `json:"status"`            // pending, accepted, expired
	InvitedBy uuid.UUID  `json:"invited_by"`
	ExpiresAt time.Time  `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
}
