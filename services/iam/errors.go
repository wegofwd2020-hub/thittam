package iam

import "errors"

var (
	// ErrNotPlatformAdmin is returned when an admin-only RPC is called by a
	// caller that does not hold the platform_admin role.
	ErrNotPlatformAdmin = errors.New("iam: caller does not have platform_admin role")

	ErrTenantNotFound     = errors.New("iam: tenant not found")
	ErrImpersonationNotFound   = errors.New("iam: impersonation session not found")
	ErrImpersonationAlreadyEnded = errors.New("iam: impersonation session already ended")
	ErrUserNotFound       = errors.New("iam: user not found")
	ErrUserAlreadyExists  = errors.New("iam: user already exists for tenant")
	ErrRoleNotFound       = errors.New("iam: role not found")
	ErrInvitationNotFound = errors.New("iam: invitation not found")
	ErrInvitationExpired  = errors.New("iam: invitation has expired")
	ErrInvitationAccepted = errors.New("iam: invitation already accepted")
	ErrTenantSlugTaken    = errors.New("iam: tenant slug already taken")
	ErrInvalidPlan        = errors.New("iam: invalid plan; must be starter, professional, or enterprise")
)
