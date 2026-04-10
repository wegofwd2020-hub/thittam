package billing

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines data access for the billing service.
// All implementations must use parameterised queries (sqlc or pgx named params).
type Repository interface {
	// Subscriptions
	CreateSubscription(ctx context.Context, s *Subscription) error
	GetSubscriptionByTenant(ctx context.Context, tenantID uuid.UUID) (*Subscription, error)
	UpdateSubscription(ctx context.Context, s *Subscription) error

	// Invoices
	CreateInvoice(ctx context.Context, inv *Invoice) error
	GetInvoice(ctx context.Context, tenantID, id uuid.UUID) (*Invoice, error)
	ListInvoices(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]Invoice, error)
	UpdateInvoice(ctx context.Context, inv *Invoice) error
	// NextInvoiceSeq returns the next available invoice sequence number for the
	// given year. Implementations must use a transactional counter or sequence.
	NextInvoiceSeq(ctx context.Context, year int) (int, error)

	// Payment Methods
	CreatePaymentMethod(ctx context.Context, pm *PaymentMethod) error
	GetPaymentMethod(ctx context.Context, id uuid.UUID) (*PaymentMethod, error)
	ListPaymentMethods(ctx context.Context, tenantID uuid.UUID) ([]PaymentMethod, error)
	UpdatePaymentMethod(ctx context.Context, pm *PaymentMethod) error
	DeletePaymentMethod(ctx context.Context, id uuid.UUID) error
	// ClearDefaultPaymentMethods unsets is_default for all payment methods of a
	// tenant before setting a new default.
	ClearDefaultPaymentMethods(ctx context.Context, tenantID uuid.UUID) error

	// Usage
	CreateUsageRecord(ctx context.Context, u *UsageRecord) error
	LatestUsageRecord(ctx context.Context, tenantID uuid.UUID) (*UsageRecord, error)

	// Dunning
	CreateDunningAttempt(ctx context.Context, d *DunningAttempt) error
	ListDunningAttempts(ctx context.Context, invoiceID uuid.UUID) ([]DunningAttempt, error)
}
