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
type UserRole struct {
	UserID     uuid.UUID `json:"user_id"`
	RoleID     uuid.UUID `json:"role_id"`
	AssignedBy uuid.UUID `json:"assigned_by"`
	AssignedAt time.Time `json:"assigned_at"`
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
