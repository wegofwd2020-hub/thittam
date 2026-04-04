package registration

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// TenantStore handles tenant and user persistence (steps 1-3).
type TenantStore interface {
	// CreateTenant creates a new tenant and returns its UUID.
	CreateTenant(ctx context.Context, name, slug, plan string) (uuid.UUID, error)

	// CreateUser creates the first admin user for a tenant and returns its UUID.
	CreateUser(ctx context.Context, tenantID uuid.UUID, email, displayName, passwordHash string) (uuid.UUID, error)

	// TenantExistsBySlug returns true if a tenant with this slug already exists.
	TenantExistsBySlug(ctx context.Context, slug string) (bool, error)

	// UserExistsByEmail returns true if any user with this email exists.
	UserExistsByEmail(ctx context.Context, email string) (bool, error)
}

// VerticalStore validates and binds verticals (step 3).
type VerticalStore interface {
	// GetVerticalDefinition returns the vertical definition or an error.
	// Returns the raw config JSONB alongside metadata.
	GetVerticalDefinition(ctx context.Context, id string) (VerticalDefinitionRow, error)

	// BindTenantVertical associates a tenant with a vertical definition.
	// Idempotent: ON CONFLICT DO NOTHING.
	BindTenantVertical(ctx context.Context, tenantID uuid.UUID, verticalID string, registeredBy uuid.UUID) error
}

// VerticalDefinitionRow is the data returned when looking up a vertical definition.
type VerticalDefinitionRow struct {
	ID          string
	Name        string
	Version     string
	Description string
	Config      json.RawMessage
	IsActive    bool
}

// LedgerSeeder seeds chart of accounts and opens accounting periods (steps 4, 7).
// Will be implemented as a gRPC client to the general-ledger service.
type LedgerSeeder interface {
	// SeedChartOfAccounts creates the initial chart of accounts for a tenant.
	// Must be idempotent: calling twice with the same tenantID is a no-op.
	SeedChartOfAccounts(ctx context.Context, tenantID uuid.UUID, accounts []vertical.ChartOfAccountEntry) error

	// OpenAccountingPeriod creates the first accounting period (current month).
	// Must be idempotent.
	OpenAccountingPeriod(ctx context.Context, tenantID uuid.UUID, year int, month time.Month) error
}

// BudgetSeeder seeds budget templates (step 5).
// Will be implemented as a gRPC client to the budget-planning service.
type BudgetSeeder interface {
	// SeedBudgetTemplates stores the vertical's default budget templates for a tenant.
	// Must be idempotent.
	SeedBudgetTemplates(ctx context.Context, tenantID uuid.UUID, templates []vertical.BudgetTemplate) error
}

// ExpenseSeeder seeds expense categories (step 6).
// Will be implemented as a gRPC client to the expense-tracking service.
type ExpenseSeeder interface {
	// SeedExpenseCategories stores the vertical's expense categories for a tenant.
	// Must be idempotent.
	SeedExpenseCategories(ctx context.Context, tenantID uuid.UUID, categories []vertical.ExpenseCategory) error
}

// EventPublisher publishes domain events (step 8).
// Will be implemented using NATS JetStream.
type EventPublisher interface {
	// PublishTenantCreated publishes the tenant.created event.
	PublishTenantCreated(ctx context.Context, event TenantCreatedEvent) error
}

// Notifier sends notifications (step 9).
// Will be implemented as a gRPC client to the notifications service.
type Notifier interface {
	// SendWelcomeEmail sends the vertical-appropriate welcome email.
	SendWelcomeEmail(ctx context.Context, tenantID uuid.UUID, email, companyName, verticalName string) error
}

// PasswordHasher hashes passwords. Callers provide a bcrypt (or argon2) implementation.
type PasswordHasher interface {
	Hash(password string) (string, error)
}
