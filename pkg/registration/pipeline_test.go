package registration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// --- Mocks ---

type mockTenantStore struct {
	createTenantFn                 func(ctx context.Context, name, slug, plan string) (uuid.UUID, error)
	createUserFn                   func(ctx context.Context, tenantID uuid.UUID, email, displayName, passwordHash string) (uuid.UUID, error)
	tenantExistsBySlugFn           func(ctx context.Context, slug string) (bool, error)
	tenantExistsByNormalizedNameFn func(ctx context.Context, name string) (bool, error)
	userExistsByEmailFn            func(ctx context.Context, email string) (bool, error)
}

func (m *mockTenantStore) CreateTenant(ctx context.Context, name, slug, plan string) (uuid.UUID, error) {
	if m.createTenantFn != nil {
		return m.createTenantFn(ctx, name, slug, plan)
	}
	return uuid.New(), nil
}

func (m *mockTenantStore) CreateUser(ctx context.Context, tenantID uuid.UUID, email, displayName, passwordHash string) (uuid.UUID, error) {
	if m.createUserFn != nil {
		return m.createUserFn(ctx, tenantID, email, displayName, passwordHash)
	}
	return uuid.New(), nil
}

func (m *mockTenantStore) TenantExistsBySlug(ctx context.Context, slug string) (bool, error) {
	if m.tenantExistsBySlugFn != nil {
		return m.tenantExistsBySlugFn(ctx, slug)
	}
	return false, nil
}

func (m *mockTenantStore) TenantExistsByNormalizedName(ctx context.Context, name string) (bool, error) {
	if m.tenantExistsByNormalizedNameFn != nil {
		return m.tenantExistsByNormalizedNameFn(ctx, name)
	}
	return false, nil
}

func (m *mockTenantStore) UserExistsByEmail(ctx context.Context, email string) (bool, error) {
	if m.userExistsByEmailFn != nil {
		return m.userExistsByEmailFn(ctx, email)
	}
	return false, nil
}

type mockVerticalStore struct {
	getVerticalDefinitionFn func(ctx context.Context, id string) (VerticalDefinitionRow, error)
	bindTenantVerticalFn    func(ctx context.Context, tenantID uuid.UUID, verticalID string, registeredBy uuid.UUID) error
}

func (m *mockVerticalStore) GetVerticalDefinition(ctx context.Context, id string) (VerticalDefinitionRow, error) {
	if m.getVerticalDefinitionFn != nil {
		return m.getVerticalDefinitionFn(ctx, id)
	}
	return testVerticalDefinition(), nil
}

func (m *mockVerticalStore) BindTenantVertical(ctx context.Context, tenantID uuid.UUID, verticalID string, registeredBy uuid.UUID) error {
	if m.bindTenantVerticalFn != nil {
		return m.bindTenantVerticalFn(ctx, tenantID, verticalID, registeredBy)
	}
	return nil
}

type mockLedgerSeeder struct {
	seedCoAFn    func(ctx context.Context, tenantID uuid.UUID, accounts []vertical.ChartOfAccountEntry) error
	openPeriodFn func(ctx context.Context, tenantID uuid.UUID, year int, month time.Month) error
}

func (m *mockLedgerSeeder) SeedChartOfAccounts(ctx context.Context, tenantID uuid.UUID, accounts []vertical.ChartOfAccountEntry) error {
	if m.seedCoAFn != nil {
		return m.seedCoAFn(ctx, tenantID, accounts)
	}
	return nil
}

func (m *mockLedgerSeeder) OpenAccountingPeriod(ctx context.Context, tenantID uuid.UUID, year int, month time.Month) error {
	if m.openPeriodFn != nil {
		return m.openPeriodFn(ctx, tenantID, year, month)
	}
	return nil
}

type mockBudgetSeeder struct {
	seedFn func(ctx context.Context, tenantID uuid.UUID, templates []vertical.BudgetTemplate) error
}

func (m *mockBudgetSeeder) SeedBudgetTemplates(ctx context.Context, tenantID uuid.UUID, templates []vertical.BudgetTemplate) error {
	if m.seedFn != nil {
		return m.seedFn(ctx, tenantID, templates)
	}
	return nil
}

type mockExpenseSeeder struct {
	seedFn func(ctx context.Context, tenantID uuid.UUID, categories []vertical.ExpenseCategory) error
}

func (m *mockExpenseSeeder) SeedExpenseCategories(ctx context.Context, tenantID uuid.UUID, categories []vertical.ExpenseCategory) error {
	if m.seedFn != nil {
		return m.seedFn(ctx, tenantID, categories)
	}
	return nil
}

type mockEventPublisher struct {
	publishFn func(ctx context.Context, event TenantCreatedEvent) error
}

func (m *mockEventPublisher) PublishTenantCreated(ctx context.Context, event TenantCreatedEvent) error {
	if m.publishFn != nil {
		return m.publishFn(ctx, event)
	}
	return nil
}

type mockNotifier struct {
	sendFn func(ctx context.Context, tenantID uuid.UUID, email, companyName, verticalName string) error
}

func (m *mockNotifier) SendWelcomeEmail(ctx context.Context, tenantID uuid.UUID, email, companyName, verticalName string) error {
	if m.sendFn != nil {
		return m.sendFn(ctx, tenantID, email, companyName, verticalName)
	}
	return nil
}

type mockHasher struct{}

func (mockHasher) Hash(password string) (string, error) {
	return "$2a$12$mock_hash_" + password, nil
}

type mockLogger struct{}

func (mockLogger) Info(msg string, kv ...interface{})  {}
func (mockLogger) Warn(msg string, kv ...interface{})  {}
func (mockLogger) Error(msg string, kv ...interface{}) {}

// --- Test helpers ---

func testVerticalDefinition() VerticalDefinitionRow {
	cfg := vertical.Config{
		ID:      "software-development",
		Name:    "Software Development Services",
		Version: "1.0.0",
		EntityLabels: vertical.EntityLabels{
			Project:          "Project",
			ProjectPlural:    "Projects",
			Phase:            "Milestone",
			PhasePlural:      "Milestones",
			TeamMember:       "Team Member",
			TeamMemberPlural: "Team Members",
			RateLabel:        "Hourly Rate",
			RateUnit:         "hour",
		},
		DefaultChartOfAccounts: []vertical.ChartOfAccountEntry{
			{Code: "1000", Name: "Bank & Cash", AccountType: "asset"},
			{Code: "5000", Name: "Personnel Costs", AccountType: "expense"},
		},
		BudgetTemplates: []vertical.BudgetTemplate{
			{Name: "Fixed-Price Web App", Description: "Standard web app budget"},
		},
		ExpenseCategories: []vertical.ExpenseCategory{
			{ID: "salary", Label: "Salary / Payroll", TaxTreatment: "tds_applicable", DefaultAccountCode: "5000"},
		},
	}
	data, _ := json.Marshal(cfg)
	return VerticalDefinitionRow{
		ID:       "software-development",
		Name:     "Software Development Services",
		Version:  "1.0.0",
		Config:   data,
		IsActive: true,
	}
}

func testRequest() RegisterRequest {
	return RegisterRequest{
		CompanyName: "Acme Software Pvt. Ltd.",
		Email:       "admin@acmesoftware.com",
		Password:    "securepass123",
		VerticalID:  "software-development",
		Plan:        "professional",
	}
}

func newTestPipeline(opts ...func(*Pipeline)) *Pipeline {
	p := NewPipeline(
		&mockTenantStore{},
		&mockVerticalStore{},
		&mockLedgerSeeder{},
		&mockBudgetSeeder{},
		&mockExpenseSeeder{},
		&mockEventPublisher{},
		&mockNotifier{},
		mockHasher{},
		mockLogger{},
	)
	for _, fn := range opts {
		fn(p)
	}
	return p
}

// --- Tests ---

func TestPipeline_HappyPath(t *testing.T) {
	t.Parallel()
	p := newTestPipeline()

	result, err := p.Run(context.Background(), testRequest())
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.NotEqual(t, uuid.Nil, result.TenantID)
	assert.NotEqual(t, uuid.Nil, result.UserID)
	assert.Equal(t, "software-development", result.VerticalID)
	assert.Equal(t, "professional", result.Plan)
	assert.Len(t, result.CompletedSteps, 9)

	for _, step := range result.CompletedSteps {
		assert.Equal(t, StepCompleted, step.Status, "step %s should be completed", step.Name)
	}
}

func TestPipeline_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		req     RegisterRequest
		wantErr error
	}{
		{
			name:    "blank company name",
			req:     RegisterRequest{CompanyName: "", Email: "a@b.com", Password: "12345678", VerticalID: "x", Plan: "starter"},
			wantErr: ErrInvalidRequest,
		},
		{
			name:    "invalid email",
			req:     RegisterRequest{CompanyName: "Acme", Email: "not-an-email", Password: "12345678", VerticalID: "x", Plan: "starter"},
			wantErr: ErrInvalidRequest,
		},
		{
			name:    "short password",
			req:     RegisterRequest{CompanyName: "Acme", Email: "a@b.com", Password: "short", VerticalID: "x", Plan: "starter"},
			wantErr: ErrInvalidRequest,
		},
		{
			name:    "blank vertical_id",
			req:     RegisterRequest{CompanyName: "Acme", Email: "a@b.com", Password: "12345678", VerticalID: "", Plan: "starter"},
			wantErr: ErrInvalidRequest,
		},
		{
			name:    "invalid plan",
			req:     RegisterRequest{CompanyName: "Acme", Email: "a@b.com", Password: "12345678", VerticalID: "x", Plan: "free"},
			wantErr: ErrInvalidPlan,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := newTestPipeline()
			_, err := p.Run(context.Background(), tc.req)
			require.Error(t, err)
			assert.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestPipeline_VerticalNotFound(t *testing.T) {
	t.Parallel()
	p := newTestPipeline(func(p *Pipeline) {
		p.verticals = &mockVerticalStore{
			getVerticalDefinitionFn: func(ctx context.Context, id string) (VerticalDefinitionRow, error) {
				return VerticalDefinitionRow{}, ErrVerticalNotFound
			},
		}
	})

	_, err := p.Run(context.Background(), testRequest())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVerticalNotFound)
}

func TestPipeline_VerticalInactive(t *testing.T) {
	t.Parallel()
	p := newTestPipeline(func(p *Pipeline) {
		p.verticals = &mockVerticalStore{
			getVerticalDefinitionFn: func(ctx context.Context, id string) (VerticalDefinitionRow, error) {
				vd := testVerticalDefinition()
				vd.IsActive = false
				return vd, nil
			},
		}
	})

	_, err := p.Run(context.Background(), testRequest())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrVerticalInactive)
}

func TestPipeline_EmailTaken(t *testing.T) {
	t.Parallel()
	p := newTestPipeline(func(p *Pipeline) {
		p.tenants = &mockTenantStore{
			userExistsByEmailFn: func(ctx context.Context, email string) (bool, error) {
				return true, nil
			},
		}
	})

	_, err := p.Run(context.Background(), testRequest())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrEmailTaken)
}

func TestPipeline_TenantNameTaken(t *testing.T) {
	t.Parallel()
	p := newTestPipeline(func(p *Pipeline) {
		p.tenants = &mockTenantStore{
			tenantExistsByNormalizedNameFn: func(ctx context.Context, name string) (bool, error) {
				return true, nil
			},
		}
	})

	_, err := p.Run(context.Background(), testRequest())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTenantNameTaken)
}

func TestPipeline_CreateTenantFailure(t *testing.T) {
	t.Parallel()
	dbErr := errors.New("db connection lost")
	p := newTestPipeline(func(p *Pipeline) {
		p.tenants = &mockTenantStore{
			createTenantFn: func(ctx context.Context, name, slug, plan string) (uuid.UUID, error) {
				return uuid.Nil, dbErr
			},
		}
	})

	result, err := p.Run(context.Background(), testRequest())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "step 1")
	require.NotNil(t, result)
	assert.Len(t, result.CompletedSteps, 1)
	assert.Equal(t, StepFailed, result.CompletedSteps[0].Status)
}

func TestPipeline_SeedCoAFailure_PartialResult(t *testing.T) {
	t.Parallel()
	p := newTestPipeline(func(p *Pipeline) {
		p.ledger = &mockLedgerSeeder{
			seedCoAFn: func(ctx context.Context, tenantID uuid.UUID, accounts []vertical.ChartOfAccountEntry) error {
				return errors.New("general-ledger unavailable")
			},
		}
	})

	result, err := p.Run(context.Background(), testRequest())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "step 4")
	require.NotNil(t, result)
	// Steps 1-3 completed, step 4 failed
	assert.Len(t, result.CompletedSteps, 4)
	assert.Equal(t, StepCompleted, result.CompletedSteps[0].Status)
	assert.Equal(t, StepCompleted, result.CompletedSteps[1].Status)
	assert.Equal(t, StepCompleted, result.CompletedSteps[2].Status)
	assert.Equal(t, StepFailed, result.CompletedSteps[3].Status)
}

func TestPipeline_BestEffortStepsDoNotFail(t *testing.T) {
	t.Parallel()
	p := newTestPipeline(func(p *Pipeline) {
		p.events = &mockEventPublisher{
			publishFn: func(ctx context.Context, event TenantCreatedEvent) error {
				return errors.New("NATS down")
			},
		}
		p.notifier = &mockNotifier{
			sendFn: func(ctx context.Context, tenantID uuid.UUID, email, companyName, verticalName string) error {
				return errors.New("SMTP error")
			},
		}
	})

	result, err := p.Run(context.Background(), testRequest())
	require.NoError(t, err, "pipeline should succeed even if best-effort steps fail")
	require.NotNil(t, result)
	assert.Len(t, result.CompletedSteps, 9)

	// Steps 8 and 9 should be failed but pipeline succeeds
	assert.Equal(t, StepFailed, result.CompletedSteps[7].Status)
	assert.Equal(t, StepFailed, result.CompletedSteps[8].Status)
	// But critical steps succeeded
	for i := 0; i < 7; i++ {
		assert.Equal(t, StepCompleted, result.CompletedSteps[i].Status)
	}
}

func TestPipeline_SlugCollision(t *testing.T) {
	t.Parallel()
	p := newTestPipeline(func(p *Pipeline) {
		p.tenants = &mockTenantStore{
			tenantExistsBySlugFn: func(ctx context.Context, slug string) (bool, error) {
				// First slug is taken
				return slug == "acme-software-pvt-ltd", nil
			},
		}
	})

	result, err := p.Run(context.Background(), testRequest())
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEqual(t, uuid.Nil, result.TenantID)
}

func TestPipeline_SeedsCorrectData(t *testing.T) {
	t.Parallel()

	var seededAccounts []vertical.ChartOfAccountEntry
	var seededTemplates []vertical.BudgetTemplate
	var seededCategories []vertical.ExpenseCategory

	p := newTestPipeline(func(p *Pipeline) {
		p.ledger = &mockLedgerSeeder{
			seedCoAFn: func(ctx context.Context, tenantID uuid.UUID, accounts []vertical.ChartOfAccountEntry) error {
				seededAccounts = accounts
				return nil
			},
		}
		p.budgets = &mockBudgetSeeder{
			seedFn: func(ctx context.Context, tenantID uuid.UUID, templates []vertical.BudgetTemplate) error {
				seededTemplates = templates
				return nil
			},
		}
		p.expenses = &mockExpenseSeeder{
			seedFn: func(ctx context.Context, tenantID uuid.UUID, categories []vertical.ExpenseCategory) error {
				seededCategories = categories
				return nil
			},
		}
	})

	_, err := p.Run(context.Background(), testRequest())
	require.NoError(t, err)

	// Verify the seeded data matches the vertical config
	assert.Len(t, seededAccounts, 2)
	assert.Equal(t, "1000", seededAccounts[0].Code)
	assert.Equal(t, "5000", seededAccounts[1].Code)

	assert.Len(t, seededTemplates, 1)
	assert.Equal(t, "Fixed-Price Web App", seededTemplates[0].Name)

	assert.Len(t, seededCategories, 1)
	assert.Equal(t, "salary", seededCategories[0].ID)
}

func TestPipeline_PublishesCorrectEvent(t *testing.T) {
	t.Parallel()

	var publishedEvent TenantCreatedEvent
	p := newTestPipeline(func(p *Pipeline) {
		p.events = &mockEventPublisher{
			publishFn: func(ctx context.Context, event TenantCreatedEvent) error {
				publishedEvent = event
				return nil
			},
		}
	})

	req := testRequest()
	result, err := p.Run(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, result.TenantID, publishedEvent.TenantID)
	assert.Equal(t, "Acme Software Pvt. Ltd.", publishedEvent.Name)
	assert.Equal(t, "software-development", publishedEvent.VerticalID)
	assert.Equal(t, "professional", publishedEvent.Plan)
	assert.Equal(t, "admin@acmesoftware.com", publishedEvent.AdminEmail)
	assert.False(t, publishedEvent.OccurredAt.IsZero())
}

// --- Slug tests ---

func TestSlugify(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"Acme Software Pvt. Ltd.", "acme-software-pvt-ltd"},
		{"   WeGoFwd2020   ", "wegofwd2020"},
		{"Hello---World", "hello-world"},
		{"Special @#$ Characters!", "special-characters"},
		{"", ""},
		{"a", "a"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(fmt.Sprintf("slugify(%q)", tc.input), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, Slugify(tc.input))
		})
	}
}

// --- Request validation tests ---

func TestRegisterRequest_Validate(t *testing.T) {
	t.Parallel()

	t.Run("valid request", func(t *testing.T) {
		req := testRequest()
		assert.NoError(t, req.Validate())
	})

	t.Run("normalizes email to lowercase", func(t *testing.T) {
		req := testRequest()
		req.Email = "Admin@AcmeSoftware.COM"
		require.NoError(t, req.Validate())
		assert.Equal(t, "admin@acmesoftware.com", req.Email)
	})

	t.Run("normalizes plan to lowercase", func(t *testing.T) {
		req := testRequest()
		req.Plan = "Professional"
		require.NoError(t, req.Validate())
		assert.Equal(t, "professional", req.Plan)
	})

	t.Run("collapses internal whitespace in company name", func(t *testing.T) {
		req := testRequest()
		req.CompanyName = "  Acme\t Corp   Studios  "
		require.NoError(t, req.Validate())
		assert.Equal(t, "Acme Corp Studios", req.CompanyName)
	})
}
