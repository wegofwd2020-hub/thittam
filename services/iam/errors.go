package iam

import "errors"

var (
	ErrTenantNotFound     = errors.New("iam: tenant not found")
	ErrUserNotFound       = errors.New("iam: user not found")
	ErrUserAlreadyExists  = errors.New("iam: user already exists for tenant")
	ErrRoleNotFound       = errors.New("iam: role not found")
	ErrInvitationNotFound = errors.New("iam: invitation not found")
	ErrInvitationExpired  = errors.New("iam: invitation has expired")
	ErrInvitationAccepted = errors.New("iam: invitation already accepted")
	ErrTenantSlugTaken    = errors.New("iam: tenant slug already taken")
	ErrInvalidPlan        = errors.New("iam: invalid plan; must be starter, professional, or enterprise")
)
