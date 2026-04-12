// Package platform implements platform-level administration for Thittam.
// Platform accounts are separate from tenant users and operate under a
// distinct JWT scope ("platform") with their own role hierarchy.
package platform

import (
	"time"

	"github.com/google/uuid"
)

// Role defines the platform administration roles.
type Role string

const (
	RoleOwner   Role = "platform_owner"
	RoleAdmin   Role = "platform_admin"
	RoleSupport Role = "platform_support"
)

// PlatformUser represents a platform-level administrator.
type PlatformUser struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	Role         Role      `json:"role"`
	MFAEnabled   bool      `json:"mfa_enabled"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
}

// ImpersonationRequest contains the data for an impersonation session.
type ImpersonationRequest struct {
	PlatformUserID uuid.UUID `json:"platform_user_id"`
	TenantID       uuid.UUID `json:"tenant_id"`
	UserID         uuid.UUID `json:"user_id"`     // tenant user to impersonate
	Reason         string    `json:"reason"`       // required (e.g., "Support ticket #1234")
	IPAddress      string    `json:"ip_address"`
}

// MaxImpersonationDuration is the hard ceiling on any single impersonation session.
// Re-authentication is required to start a new session after this limit.
const MaxImpersonationDuration = 30 * time.Minute

// RevocationReason identifies why an impersonation session was ended.
type RevocationReason string

const (
	// RevocationManual means the platform admin explicitly ended the session.
	RevocationManual RevocationReason = "manual"
	// RevocationExpired means the session reached MaxImpersonationDuration.
	RevocationExpired RevocationReason = "expired"
	// RevocationPasswordChange means the target user changed their password.
	RevocationPasswordChange RevocationReason = "target_password_changed"
	// RevocationDeactivated means the target user was deactivated.
	RevocationDeactivated RevocationReason = "target_user_deactivated"
	// RevocationMFAChange means the target user modified their MFA configuration.
	RevocationMFAChange RevocationReason = "target_mfa_changed"
)

// ImpersonationSession is the result of a successful impersonation request.
type ImpersonationSession struct {
	ID        uuid.UUID `json:"id"`
	TenantJWT string    `json:"tenant_jwt"` // set by handler layer; empty from Service
	StartedAt time.Time `json:"started_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ActiveImpersonationSession represents an ongoing session as stored in the
// ImpersonationStore. Used for revocation trigger queries.
type ActiveImpersonationSession struct {
	ID             uuid.UUID `json:"id"`
	PlatformUserID uuid.UUID `json:"platform_user_id"`
	TenantID       uuid.UUID `json:"tenant_id"`
	TargetUserID   uuid.UUID `json:"target_user_id"`
	Reason         string    `json:"reason"`
	StartedAt      time.Time `json:"started_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// blockedDuringImpersonation is the set of action names that an impersonator
// must never be allowed to perform on behalf of the target user.
// Checked by IsActionBlocked.
var blockedDuringImpersonation = map[string]bool{
	// Identity mutations — must only be done by the real account owner.
	"ChangePassword":  true,
	"UpdateMFA":       true,
	"DisableMFA":      true,
	"EnableMFA":       true,
	"RevokeAllTokens": true,
	// Billing — financial scope changes require real-user consent.
	"CreateSubscription":  true,
	"UpgradeSubscription": true,
	"CancelSubscription":  true,
	"AddPaymentMethod":    true,
	"RemovePaymentMethod": true,
	// Platform meta — an impersonator cannot elevate their own access.
	"AssignRole": true,
	"RevokeRole": true,
}

// PlatformClaims represents the JWT payload for platform tokens.
type PlatformClaims struct {
	Subject    uuid.UUID `json:"sub"`
	Email      string    `json:"email"`
	Role       Role      `json:"role"`
	Scope      string    `json:"scope"` // always "platform"
	IssuedAt   time.Time `json:"iat"`
	ExpiresAt  time.Time `json:"exp"`
}

// TenantSummary provides an overview of a tenant for the platform admin dashboard.
type TenantSummary struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	Slug       string    `json:"slug"`
	Plan       string    `json:"plan"`
	VerticalID string    `json:"vertical_id"`
	Status     string    `json:"status"`
	UserCount  int       `json:"user_count"`
	CreatedAt  time.Time `json:"created_at"`
}
