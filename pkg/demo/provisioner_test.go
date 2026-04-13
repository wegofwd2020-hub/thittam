package demo

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

type mockProvisioner struct {
	createTenantFn func(ctx context.Context, name, slug, plan string) (uuid.UUID, error)
	createUserFn   func(ctx context.Context, tenantID uuid.UUID, name, email, role, hash string) (uuid.UUID, error)
	bindFn         func(ctx context.Context, tenantID uuid.UUID, verticalID string, adminID uuid.UUID) error
	seedFn         func(ctx context.Context, tenantID uuid.UUID, projects []DemoProject) error
	deleteFn       func(ctx context.Context, tenantID uuid.UUID) error
	listFn         func(ctx context.Context) ([]DemoTenantInfo, error)
}

func (m *mockProvisioner) CreateDemoTenant(ctx context.Context, name, slug, plan string) (uuid.UUID, error) {
	if m.createTenantFn != nil {
		return m.createTenantFn(ctx, name, slug, plan)
	}
	return uuid.New(), nil
}
func (m *mockProvisioner) CreateDemoUser(ctx context.Context, tenantID uuid.UUID, name, email, role, hash string) (uuid.UUID, error) {
	if m.createUserFn != nil {
		return m.createUserFn(ctx, tenantID, name, email, role, hash)
	}
	return uuid.New(), nil
}
func (m *mockProvisioner) BindVertical(ctx context.Context, tenantID uuid.UUID, verticalID string, adminID uuid.UUID) error {
	if m.bindFn != nil {
		return m.bindFn(ctx, tenantID, verticalID, adminID)
	}
	return nil
}
func (m *mockProvisioner) SeedDemoProjects(ctx context.Context, tenantID uuid.UUID, projects []DemoProject) error {
	if m.seedFn != nil {
		return m.seedFn(ctx, tenantID, projects)
	}
	return nil
}
func (m *mockProvisioner) DeleteDemoTenant(ctx context.Context, tenantID uuid.UUID) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, tenantID)
	}
	return nil
}
func (m *mockProvisioner) ListDemoTenants(ctx context.Context) ([]DemoTenantInfo, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
}

type mockHasher struct{}
func (mockHasher) Hash(password string) (string, error) {
	return "$2a$12$mock_" + password, nil
}

type mockLogger struct{}
func (mockLogger) Info(msg string, kv ...interface{})  {}
func (mockLogger) Error(msg string, kv ...interface{}) {}

// --- Tests ---

func testPreview() *DemoPreview {
	return &DemoPreview{
		PreviewToken: "test-token",
		Tenant:       DemoTenant{Name: "Test Co", Slug: "test-co", Plan: "professional"},
		VerticalID:   "movie-production",
		VerticalName: "Movie Production",
		Users: []DemoUser{
			{Name: "Admin", Email: "admin@test-co.demo", Role: "super_admin", TempPassword: "demo1234"},
			{Name: "Producer", Email: "producer@test-co.demo", Role: "coordinator", TempPassword: "demo1234"},
		},
		Projects: []DemoProject{
			{Title: "Test Film", Phase: "production"},
		},
	}
}

func TestProvision_HappyPath(t *testing.T) {
	t.Parallel()
	p := NewProvisioner(&mockProvisioner{}, mockHasher{}, mockLogger{})

	result, err := p.Provision(context.Background(), testPreview())
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, result.TenantID)
	assert.Equal(t, "admin@test-co.demo", result.AdminEmail)
	assert.Equal(t, "demo1234", result.AdminPassword)
	assert.Equal(t, 2, result.UserCount)
	assert.Equal(t, 1, result.ProjectCount)
	assert.False(t, result.CreatedAt.IsZero())
}

func TestProvision_TenantCreationFails(t *testing.T) {
	t.Parallel()
	p := NewProvisioner(&mockProvisioner{
		createTenantFn: func(ctx context.Context, name, slug, plan string) (uuid.UUID, error) {
			return uuid.Nil, errors.New("db error")
		},
	}, mockHasher{}, mockLogger{})

	_, err := p.Provision(context.Background(), testPreview())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create tenant")
}

func TestProvision_UserCreationFails(t *testing.T) {
	t.Parallel()
	p := NewProvisioner(&mockProvisioner{
		createUserFn: func(ctx context.Context, tenantID uuid.UUID, name, email, role, hash string) (uuid.UUID, error) {
			return uuid.Nil, errors.New("user error")
		},
	}, mockHasher{}, mockLogger{})

	_, err := p.Provision(context.Background(), testPreview())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create user")
}

func TestProvision_SeedProjectsFailsNonFatal(t *testing.T) {
	t.Parallel()
	p := NewProvisioner(&mockProvisioner{
		seedFn: func(ctx context.Context, tenantID uuid.UUID, projects []DemoProject) error {
			return errors.New("seed error")
		},
	}, mockHasher{}, mockLogger{})

	// Should succeed even if seeding fails
	result, err := p.Provision(context.Background(), testPreview())
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, result.TenantID)
}

func TestTeardown(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	var deletedID uuid.UUID

	p := NewProvisioner(&mockProvisioner{
		deleteFn: func(ctx context.Context, id uuid.UUID) error {
			deletedID = id
			return nil
		},
	}, mockHasher{}, mockLogger{})

	err := p.Teardown(context.Background(), tenantID)
	require.NoError(t, err)
	assert.Equal(t, tenantID, deletedID)
}

func TestTeardown_Error(t *testing.T) {
	t.Parallel()
	p := NewProvisioner(&mockProvisioner{
		deleteFn: func(ctx context.Context, id uuid.UUID) error {
			return errors.New("cannot delete")
		},
	}, mockHasher{}, mockLogger{})

	err := p.Teardown(context.Background(), uuid.New())
	require.Error(t, err)
}

func TestListDemoTenants(t *testing.T) {
	t.Parallel()
	p := NewProvisioner(&mockProvisioner{
		listFn: func(ctx context.Context) ([]DemoTenantInfo, error) {
			return []DemoTenantInfo{
				{TenantID: uuid.New(), Name: "Demo 1"},
				{TenantID: uuid.New(), Name: "Demo 2"},
			}, nil
		},
	}, mockHasher{}, mockLogger{})

	list, err := p.ListDemoTenants(context.Background())
	require.NoError(t, err)
	assert.Len(t, list, 2)
}
