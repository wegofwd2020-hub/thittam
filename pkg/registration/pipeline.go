package registration

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// Logger defines structured logging for the registration package.
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
}

// Pipeline orchestrates the 9-step tenant registration flow.
type Pipeline struct {
	tenants   TenantStore
	verticals VerticalStore
	ledger    LedgerSeeder
	budgets   BudgetSeeder
	expenses  ExpenseSeeder
	events    EventPublisher
	notifier  Notifier
	hasher    PasswordHasher
	logger    Logger
}

// NewPipeline creates a Pipeline wired with all required dependencies.
func NewPipeline(
	tenants TenantStore,
	verticals VerticalStore,
	ledger LedgerSeeder,
	budgets BudgetSeeder,
	expenses ExpenseSeeder,
	events EventPublisher,
	notifier Notifier,
	hasher PasswordHasher,
	logger Logger,
) *Pipeline {
	return &Pipeline{
		tenants:   tenants,
		verticals: verticals,
		ledger:    ledger,
		budgets:   budgets,
		expenses:  expenses,
		events:    events,
		notifier:  notifier,
		hasher:    hasher,
		logger:    logger,
	}
}

// Run executes the full registration pipeline.
//
// Steps 1-3 are transactional (tenant + user + vertical binding).
// Steps 4-7 are idempotent remote calls (seeders).
// Steps 8-9 are best-effort (event + email).
func (p *Pipeline) Run(ctx context.Context, req RegisterRequest) (*RegisterResult, error) {
	// --- Validation ---
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Validate vertical exists and is active
	vd, err := p.verticals.GetVerticalDefinition(ctx, req.VerticalID)
	if err != nil {
		return nil, fmt.Errorf("validate vertical: %w", err)
	}
	if !vd.IsActive {
		return nil, ErrVerticalInactive
	}

	// Parse vertical config to access CoA, templates, categories
	var cfg vertical.Config
	if err := json.Unmarshal(vd.Config, &cfg); err != nil {
		return nil, fmt.Errorf("parse vertical config: %w", err)
	}

	// Check email uniqueness
	emailTaken, err := p.tenants.UserExistsByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("check email: %w", err)
	}
	if emailTaken {
		return nil, ErrEmailTaken
	}

	// Check tenant-name uniqueness (normalized). Unlike slug (which silently
	// auto-suffixes on collision), a duplicate name is a clean rejection.
	nameTaken, err := p.tenants.TenantExistsByNormalizedName(ctx, req.CompanyName)
	if err != nil {
		return nil, fmt.Errorf("check tenant name: %w", err)
	}
	if nameTaken {
		return nil, ErrTenantNameTaken
	}

	// Generate slug
	slug := Slugify(req.CompanyName)
	if slug == "" {
		slug = "tenant"
	}
	slugTaken, err := p.tenants.TenantExistsBySlug(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("check slug: %w", err)
	}
	if slugTaken {
		// Append short UUID suffix for collision
		slug = slug + "-" + uuid.New().String()[:8]
	}

	// Hash password
	passwordHash, err := p.hasher.Hash(req.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	result := &RegisterResult{
		VerticalID:     req.VerticalID,
		Plan:           req.Plan,
		CompletedSteps: make([]StepResult, 0, 9),
	}

	// --- Steps 1-3: Transactional (tenant + user + vertical binding) ---

	// Step 1: Create Tenant
	start := time.Now()
	tenantID, err := p.tenants.CreateTenant(ctx, req.CompanyName, slug, req.Plan, req.Country, req.PrimaryCurrency)
	result.CompletedSteps = append(result.CompletedSteps, stepResult(StepCreateTenant, start, err))
	if err != nil {
		return result, fmt.Errorf("step 1 create tenant: %w", err)
	}
	result.TenantID = tenantID

	p.logger.Info("tenant created",
		"tenant_id", tenantID.String(),
		"slug", slug,
	)

	// Step 2: Create first User (super_admin)
	start = time.Now()
	userID, err := p.tenants.CreateUser(ctx, tenantID, req.Email, req.CompanyName+" Admin", passwordHash)
	result.CompletedSteps = append(result.CompletedSteps, stepResult(StepCreateUser, start, err))
	if err != nil {
		return result, fmt.Errorf("step 2 create user: %w", err)
	}
	result.UserID = userID

	p.logger.Info("admin user created",
		"tenant_id", tenantID.String(),
		"user_id", userID.String(),
		"email", req.Email,
	)

	// Step 3: Bind tenant to vertical
	start = time.Now()
	err = p.verticals.BindTenantVertical(ctx, tenantID, req.VerticalID, userID)
	result.CompletedSteps = append(result.CompletedSteps, stepResult(StepBindVertical, start, err))
	if err != nil {
		return result, fmt.Errorf("step 3 bind vertical: %w", err)
	}

	p.logger.Info("vertical bound",
		"tenant_id", tenantID.String(),
		"vertical_id", req.VerticalID,
	)

	// --- Steps 4-7: Idempotent remote calls ---

	// Step 4: Seed Chart of Accounts
	start = time.Now()
	err = p.ledger.SeedChartOfAccounts(ctx, tenantID, cfg.DefaultChartOfAccounts)
	result.CompletedSteps = append(result.CompletedSteps, stepResult(StepSeedChartOfAccounts, start, err))
	if err != nil {
		p.logger.Error("step 4 seed CoA failed (retryable)",
			"tenant_id", tenantID.String(),
			"error", err.Error(),
		)
		return result, fmt.Errorf("step 4 seed chart of accounts: %w", err)
	}

	// Step 5: Seed Budget Templates
	start = time.Now()
	err = p.budgets.SeedBudgetTemplates(ctx, tenantID, cfg.BudgetTemplates)
	result.CompletedSteps = append(result.CompletedSteps, stepResult(StepSeedBudgetTemplates, start, err))
	if err != nil {
		p.logger.Error("step 5 seed budget templates failed (retryable)",
			"tenant_id", tenantID.String(),
			"error", err.Error(),
		)
		return result, fmt.Errorf("step 5 seed budget templates: %w", err)
	}

	// Step 6: Seed Expense Categories
	start = time.Now()
	err = p.expenses.SeedExpenseCategories(ctx, tenantID, cfg.ExpenseCategories)
	result.CompletedSteps = append(result.CompletedSteps, stepResult(StepSeedExpenseCategories, start, err))
	if err != nil {
		p.logger.Error("step 6 seed expense categories failed (retryable)",
			"tenant_id", tenantID.String(),
			"error", err.Error(),
		)
		return result, fmt.Errorf("step 6 seed expense categories: %w", err)
	}

	// Step 7: Open first Accounting Period
	start = time.Now()
	now := time.Now()
	err = p.ledger.OpenAccountingPeriod(ctx, tenantID, now.Year(), now.Month())
	result.CompletedSteps = append(result.CompletedSteps, stepResult(StepOpenAccountingPeriod, start, err))
	if err != nil {
		p.logger.Error("step 7 open accounting period failed (retryable)",
			"tenant_id", tenantID.String(),
			"error", err.Error(),
		)
		return result, fmt.Errorf("step 7 open accounting period: %w", err)
	}

	// --- Steps 8-9: Best-effort ---

	// Step 8: Publish tenant.created event
	start = time.Now()
	err = p.events.PublishTenantCreated(ctx, TenantCreatedEvent{
		TenantID:    tenantID,
		Name:        req.CompanyName,
		Slug:        slug,
		Plan:        req.Plan,
		VerticalID:  req.VerticalID,
		AdminEmail:  req.Email,
		AdminUserID: userID,
		OccurredAt:  time.Now(),
	})
	result.CompletedSteps = append(result.CompletedSteps, stepResult(StepPublishEvent, start, err))
	if err != nil {
		p.logger.Warn("step 8 publish event failed (best-effort)",
			"tenant_id", tenantID.String(),
			"error", err.Error(),
		)
		// Do not return error — best effort
	}

	// Step 9: Send welcome email
	start = time.Now()
	err = p.notifier.SendWelcomeEmail(ctx, tenantID, req.Email, req.CompanyName, vd.Name)
	result.CompletedSteps = append(result.CompletedSteps, stepResult(StepSendWelcomeEmail, start, err))
	if err != nil {
		p.logger.Warn("step 9 send welcome email failed (best-effort)",
			"tenant_id", tenantID.String(),
			"error", err.Error(),
		)
		// Do not return error — best effort
	}

	p.logger.Info("registration pipeline completed",
		"tenant_id", tenantID.String(),
		"vertical_id", req.VerticalID,
		"plan", req.Plan,
	)

	return result, nil
}

func stepResult(step Step, start time.Time, err error) StepResult {
	sr := StepResult{
		Step:    step,
		Name:    step.String(),
		Elapsed: time.Since(start),
	}
	if err != nil {
		sr.Status = StepFailed
		sr.Error = err.Error()
	} else {
		sr.Status = StepCompleted
	}
	return sr
}
