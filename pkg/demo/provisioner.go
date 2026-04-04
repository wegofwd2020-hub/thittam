package demo

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// TenantProvisioner creates the demo tenant and all associated data.
// It wraps the registration pipeline and adds demo-specific seeding.
type TenantProvisioner interface {
	// CreateDemoTenant creates a tenant flagged as is_demo=true.
	CreateDemoTenant(ctx context.Context, name, slug, plan string) (uuid.UUID, error)

	// CreateDemoUser creates a user in the demo tenant.
	CreateDemoUser(ctx context.Context, tenantID uuid.UUID, name, email, role, passwordHash string) (uuid.UUID, error)

	// BindVertical binds the tenant to a vertical.
	BindVertical(ctx context.Context, tenantID uuid.UUID, verticalID string, adminID uuid.UUID) error

	// SeedDemoProjects creates sample projects with budgets and expenses.
	SeedDemoProjects(ctx context.Context, tenantID uuid.UUID, projects []DemoProject) error

	// DeleteDemoTenant removes a demo tenant and all associated data.
	DeleteDemoTenant(ctx context.Context, tenantID uuid.UUID) error

	// ListDemoTenants returns all tenants where is_demo=true.
	ListDemoTenants(ctx context.Context) ([]DemoTenantInfo, error)
}

// DemoTenantInfo is a summary row for the demo tenant list.
type DemoTenantInfo struct {
	TenantID   uuid.UUID `json:"tenant_id"`
	Name       string    `json:"name"`
	VerticalID string    `json:"vertical_id"`
	Plan       string    `json:"plan"`
	UserCount  int       `json:"user_count"`
	CreatedAt  time.Time `json:"created_at"`
}

// PasswordHasher hashes passwords for demo users.
type PasswordHasher interface {
	Hash(password string) (string, error)
}

// Logger for the demo provisioner.
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
}

// Provisioner orchestrates the full demo tenant creation.
type Provisioner struct {
	store  TenantProvisioner
	hasher PasswordHasher
	logger Logger
}

// NewProvisioner creates a demo provisioner.
func NewProvisioner(store TenantProvisioner, hasher PasswordHasher, logger Logger) *Provisioner {
	return &Provisioner{store: store, hasher: hasher, logger: logger}
}

// Provision creates a demo tenant from a confirmed preview.
func (p *Provisioner) Provision(ctx context.Context, preview *DemoPreview) (*ProvisionResult, error) {
	// 1. Create demo tenant
	tenantID, err := p.store.CreateDemoTenant(ctx, preview.Tenant.Name, preview.Tenant.Slug, preview.Tenant.Plan)
	if err != nil {
		return nil, fmt.Errorf("demo: create tenant: %w", err)
	}

	p.logger.Info("demo tenant created",
		"tenant_id", tenantID.String(),
		"vertical_id", preview.VerticalID,
	)

	// 2. Create users
	var adminID uuid.UUID
	for i, user := range preview.Users {
		hash, err := p.hasher.Hash(user.TempPassword)
		if err != nil {
			return nil, fmt.Errorf("demo: hash password for %s: %w", user.Email, err)
		}

		userID, err := p.store.CreateDemoUser(ctx, tenantID, user.Name, user.Email, user.Role, hash)
		if err != nil {
			return nil, fmt.Errorf("demo: create user %s: %w", user.Email, err)
		}

		if i == 0 {
			adminID = userID // first user is admin
		}
	}

	p.logger.Info("demo users created",
		"tenant_id", tenantID.String(),
		"count", len(preview.Users),
	)

	// 3. Bind vertical
	if err := p.store.BindVertical(ctx, tenantID, preview.VerticalID, adminID); err != nil {
		return nil, fmt.Errorf("demo: bind vertical: %w", err)
	}

	// 4. Seed demo projects
	if err := p.store.SeedDemoProjects(ctx, tenantID, preview.Projects); err != nil {
		p.logger.Error("demo: seed projects failed (non-fatal)",
			"tenant_id", tenantID.String(),
			"error", err.Error(),
		)
		// Non-fatal — tenant is usable without sample projects
	}

	p.logger.Info("demo provisioning complete",
		"tenant_id", tenantID.String(),
		"vertical_id", preview.VerticalID,
		"users", len(preview.Users),
		"projects", len(preview.Projects),
	)

	return &ProvisionResult{
		TenantID:      tenantID,
		AdminEmail:    preview.Users[0].Email,
		AdminPassword: preview.Users[0].TempPassword,
		UserCount:     len(preview.Users),
		ProjectCount:  len(preview.Projects),
		CreatedAt:     time.Now(),
	}, nil
}

// ListDemoTenants returns all demo tenants.
func (p *Provisioner) ListDemoTenants(ctx context.Context) ([]DemoTenantInfo, error) {
	return p.store.ListDemoTenants(ctx)
}

// Teardown removes a demo tenant and all its data.
func (p *Provisioner) Teardown(ctx context.Context, tenantID uuid.UUID) error {
	if err := p.store.DeleteDemoTenant(ctx, tenantID); err != nil {
		return fmt.Errorf("demo: teardown tenant %s: %w", tenantID, err)
	}

	p.logger.Info("demo tenant removed",
		"tenant_id", tenantID.String(),
	)
	return nil
}
