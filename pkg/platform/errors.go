package platform

import "errors"

var (
	// ErrNotPlatformUser is returned when a non-platform JWT is used on platform endpoints.
	ErrNotPlatformUser = errors.New("platform: not a platform user")

	// ErrInsufficientRole is returned when the platform user lacks the required role.
	ErrInsufficientRole = errors.New("platform: insufficient role for this action")

	// ErrMFARequired is returned when MFA is not enabled/verified for a platform action.
	ErrMFARequired = errors.New("platform: MFA verification required")

	// ErrImpersonationDenied is returned when the user cannot impersonate (wrong role or tenant).
	ErrImpersonationDenied = errors.New("platform: impersonation denied")

	// ErrReasonRequired is returned when an impersonation request lacks a reason.
	ErrReasonRequired = errors.New("platform: impersonation reason is required")

	// ErrPlatformUserNotFound is returned when the platform user doesn't exist.
	ErrPlatformUserNotFound = errors.New("platform: user not found")
)
