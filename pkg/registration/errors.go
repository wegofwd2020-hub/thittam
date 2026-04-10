package registration

import "errors"

var (
	// ErrVerticalNotFound is returned when the requested vertical_id does not exist.
	ErrVerticalNotFound = errors.New("registration: vertical not found")

	// ErrVerticalInactive is returned when the vertical exists but is deactivated.
	ErrVerticalInactive = errors.New("registration: vertical is inactive")

	// ErrEmailTaken is returned when the email is already registered.
	ErrEmailTaken = errors.New("registration: email already registered")

	// ErrSlugTaken is returned when the generated slug collides with an existing tenant.
	ErrSlugTaken = errors.New("registration: tenant slug already taken")

	// ErrInvalidPlan is returned when the plan is not one of the allowed values.
	ErrInvalidPlan = errors.New("registration: invalid plan")

	// ErrInvalidRequest is returned when required fields are missing or malformed.
	ErrInvalidRequest = errors.New("registration: invalid request")

	// ErrSagaNotFound is returned when no saga exists for the given ID or email.
	ErrSagaNotFound = errors.New("registration: saga not found")
)
