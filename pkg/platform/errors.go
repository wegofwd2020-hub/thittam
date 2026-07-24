package platform

import "errors"

var (
	// ErrNotPlatformUser is returned when a non-platform JWT is used on platform endpoints.
	ErrNotPlatformUser = errors.New("platform: not a platform user")

	// ErrInsufficientRole is returned when the platform user lacks the required role.
	ErrInsufficientRole = errors.New("platform: insufficient role for this action")
)
