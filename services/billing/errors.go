package billing

import "errors"

var (
	ErrSubscriptionNotFound    = errors.New("billing: subscription not found")
	ErrSubscriptionAlreadyExists = errors.New("billing: tenant already has a subscription")
	ErrInvoiceNotFound         = errors.New("billing: invoice not found")
	ErrPaymentMethodNotFound   = errors.New("billing: payment method not found")
	ErrInvalidPlan             = errors.New("billing: invalid plan name")
	ErrDowngradeNotAllowed     = errors.New("billing: downgrade not allowed — remove resources first")
	ErrSubscriptionCancelled   = errors.New("billing: subscription is cancelled")
	ErrSubscriptionSuspended   = errors.New("billing: subscription is suspended")
	ErrInvoiceAlreadyPaid      = errors.New("billing: invoice is already paid")
	ErrPlanLimitExceeded       = errors.New("billing: plan limit exceeded")
	ErrNoDefaultPaymentMethod  = errors.New("billing: no default payment method on file")
	ErrOutboxEventNotFound     = errors.New("billing: outbox event not found")
)
