package registration

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/wegofwd2020/thittam/pkg/vertical"
)

// SagaStatus is the lifecycle state of a registration saga.
type SagaStatus string

const (
	// SagaInProgress means the saga is actively running or paused mid-flight.
	SagaInProgress SagaStatus = "in_progress"

	// SagaCompleted means all 9 steps finished successfully.
	SagaCompleted SagaStatus = "completed"

	// SagaFailed means a critical step failed and the saga has not yet been
	// compensated. Manual intervention or a Retry call is required.
	SagaFailed SagaStatus = "failed"

	// SagaCompensating means the orchestrator is actively rolling back
	// completed steps in reverse order.
	SagaCompensating SagaStatus = "compensating"

	// SagaCompensated means all compensating transactions completed
	// successfully. The system is back to the pre-registration state.
	SagaCompensated SagaStatus = "compensated"

	// SagaCompensationFailed means one or more compensating transactions
	// failed. Manual ops intervention is required — see the ops runbook.
	SagaCompensationFailed SagaStatus = "compensation_failed"
)

// RegistrationSaga is the durable state of one tenant registration attempt.
// It is persisted to the registration_sagas table after every step transition
// so that a process crash or network partition leaves a recoverable record.
type RegistrationSaga struct {
	ID uuid.UUID `json:"id"`

	// Email is the deduplication key — only one in-progress saga per email is
	// permitted. A new Start call with the same email returns the existing saga.
	Email string `json:"email"`

	// TenantID and UserID are set as steps 1 and 2 complete.
	// uuid.Nil means the corresponding step has not yet run.
	TenantID uuid.UUID `json:"tenant_id"`
	UserID   uuid.UUID `json:"user_id"`

	Status SagaStatus `json:"status"`

	// CompletedSteps records every step that committed successfully.
	// Used by Resume to skip already-completed steps on retry.
	CompletedSteps []Step `json:"completed_steps"`

	// FailedStep is the step that caused the transition to SagaFailed.
	// 0 (zero value) means no failure.
	FailedStep    Step   `json:"failed_step,omitempty"`
	FailureReason string `json:"failure_reason,omitempty"`

	// CompensationErrors records any errors from compensating transactions.
	// Non-empty only when Status == SagaCompensationFailed.
	CompensationErrors []string `json:"compensation_errors,omitempty"`

	// Request is the original registration input. Stored so that Resume can
	// re-run failed steps without the caller re-supplying the full request.
	Request RegisterRequest `json:"request"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// stepCompleted returns true if the given step is in CompletedSteps.
func (s *RegistrationSaga) stepCompleted(step Step) bool {
	for _, completed := range s.CompletedSteps {
		if completed == step {
			return true
		}
	}
	return false
}

// markStepCompleted appends the step to CompletedSteps (idempotent).
func (s *RegistrationSaga) markStepCompleted(step Step) {
	if !s.stepCompleted(step) {
		s.CompletedSteps = append(s.CompletedSteps, step)
	}
}

// SagaStore persists and retrieves RegistrationSaga records.
// The implementation uses the platform schema (not a tenant schema).
type SagaStore interface {
	// Create inserts a new saga record. Returns an error if a saga with the
	// same ID already exists.
	Create(ctx context.Context, saga *RegistrationSaga) error

	// Update writes the current saga state. Must be called after every step
	// transition to ensure crash-recoverability.
	Update(ctx context.Context, saga *RegistrationSaga) error

	// GetByID retrieves a saga by its primary key.
	// Returns ErrSagaNotFound if not present.
	GetByID(ctx context.Context, id uuid.UUID) (*RegistrationSaga, error)

	// GetByEmail retrieves the most recent saga for an email address.
	// Used for deduplication on Start. Returns ErrSagaNotFound if absent.
	GetByEmail(ctx context.Context, email string) (*RegistrationSaga, error)
}

// Compensator executes the compensating transactions that undo completed steps
// when the saga must roll back.
//
// # Compensation mapping
//
//	Step 1 (CreateTenant)      → DeleteTenant: removes the tenant row and drops
//	                             the tenant_<uuid> schema (cascades all step 4-7 data)
//	Step 2 (CreateUser)        → DeleteUser: removes the admin user row
//	Step 3 (BindVertical)      → UnbindVertical: removes the tenant_verticals row
//	Steps 4-7 (seeders)        → No explicit compensation. The tenant schema is
//	                             dropped by DeleteTenant, which cascades all seed data.
//	Steps 8-9 (best-effort)    → No compensation. Events and emails cannot be recalled;
//	                             downstream consumers are responsible for idempotency.
//
// Compensation is executed in reverse step order: 3 → 2 → 1.
type Compensator interface {
	// DeleteTenant removes the tenant record and drops its schema.
	// Idempotent: returns nil if the tenant does not exist.
	DeleteTenant(ctx context.Context, tenantID uuid.UUID) error

	// DeleteUser removes the user record.
	// Idempotent: returns nil if the user does not exist.
	DeleteUser(ctx context.Context, tenantID, userID uuid.UUID) error

	// UnbindVertical removes the tenant–vertical association.
	// Idempotent: returns nil if the binding does not exist.
	UnbindVertical(ctx context.Context, tenantID uuid.UUID, verticalID string) error
}

// Orchestrator drives the tenant registration saga with durable state,
// idempotent retries, and compensating transactions on failure.
//
// The pipeline executes steps 1-9. After each step the saga state is
// persisted to the SagaStore. If a critical step fails (1-7), the
// orchestrator runs compensating transactions in reverse order for the
// steps that completed, leaving the system in a consistent state.
//
// Steps 8-9 (publish event, send welcome email) are best-effort: their
// failure is recorded but does not trigger compensation or mark the saga
// as failed.
type Orchestrator struct {
	pipeline    *Pipeline
	store       SagaStore
	compensator Compensator
	logger      Logger
}

// NewOrchestrator creates an Orchestrator.
func NewOrchestrator(pipeline *Pipeline, store SagaStore, compensator Compensator, logger Logger) *Orchestrator {
	return &Orchestrator{
		pipeline:    pipeline,
		store:       store,
		compensator: compensator,
		logger:      logger,
	}
}

// Start begins a new registration saga for the given request.
//
// Idempotency: if an in-progress or completed saga already exists for
// req.Email, Start returns that existing saga rather than creating a
// duplicate. This means clients can safely retry a Start call that timed
// out without creating duplicate tenants.
//
// On a critical step failure the orchestrator compensates automatically
// and returns the saga (with status SagaFailed or SagaCompensated) alongside
// the original step error.
func (o *Orchestrator) Start(ctx context.Context, req RegisterRequest) (*RegistrationSaga, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Deduplication: re-use an existing saga for this email.
	if existing, err := o.store.GetByEmail(ctx, req.Email); err == nil {
		switch existing.Status {
		case SagaCompleted:
			return existing, nil
		case SagaInProgress, SagaFailed:
			// Return the existing saga — caller should call Resume if they want
			// to retry, or check status if they just want to observe.
			return existing, fmt.Errorf("registration: saga %s already exists for %s (status: %s)",
				existing.ID, req.Email, existing.Status)
		}
	}

	saga := &RegistrationSaga{
		ID:        uuid.New(),
		Email:     req.Email,
		Status:    SagaInProgress,
		Request:   req,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	if err := o.store.Create(ctx, saga); err != nil {
		return nil, fmt.Errorf("registration: persist saga: %w", err)
	}

	return o.run(ctx, saga)
}

// Resume retries a saga that is in SagaFailed status.
// Already-completed steps are skipped. The orchestrator re-runs from the
// first uncompleted step, then compensates again if it fails.
func (o *Orchestrator) Resume(ctx context.Context, sagaID uuid.UUID) (*RegistrationSaga, error) {
	saga, err := o.store.GetByID(ctx, sagaID)
	if err != nil {
		return nil, fmt.Errorf("registration: load saga %s: %w", sagaID, err)
	}

	if saga.Status == SagaCompleted {
		return saga, nil
	}
	if saga.Status != SagaFailed {
		return nil, fmt.Errorf("registration: saga %s cannot be resumed (status: %s)", sagaID, saga.Status)
	}

	// Clear failure state and re-attempt.
	saga.Status = SagaInProgress
	saga.FailedStep = 0
	saga.FailureReason = ""
	saga.UpdatedAt = time.Now().UTC()
	if err := o.store.Update(ctx, saga); err != nil {
		return nil, fmt.Errorf("registration: update saga for resume: %w", err)
	}

	return o.run(ctx, saga)
}

// GetStatus returns the current state of a saga.
func (o *Orchestrator) GetStatus(ctx context.Context, sagaID uuid.UUID) (*RegistrationSaga, error) {
	return o.store.GetByID(ctx, sagaID)
}

// run executes pipeline steps for the saga, persisting state after each one.
// Steps already in saga.CompletedSteps are skipped (idempotent resume).
func (o *Orchestrator) run(ctx context.Context, saga *RegistrationSaga) (*RegistrationSaga, error) {
	req := saga.Request

	// We need the vertical config for the seeding steps regardless of which
	// step we are resuming from. Fetch it once upfront.
	vd, cfg, err := o.pipeline.loadVerticalConfig(ctx, req.VerticalID)
	if err != nil {
		return saga, o.failSaga(ctx, saga, 0, err)
	}

	// --- Step 1: Create Tenant ---
	if !saga.stepCompleted(StepCreateTenant) {
		slug := Slugify(req.CompanyName)
		if slug == "" {
			slug = "tenant"
		}
		if taken, _ := o.pipeline.tenants.TenantExistsBySlug(ctx, slug); taken {
			slug = slug + "-" + saga.ID.String()[:8]
		}

		if taken, err := o.pipeline.tenants.TenantExistsByNormalizedName(ctx, req.CompanyName); err != nil {
			return saga, o.failSaga(ctx, saga, StepCreateTenant, fmt.Errorf("check tenant name: %w", err))
		} else if taken {
			return saga, o.failSaga(ctx, saga, StepCreateTenant, ErrTenantNameTaken)
		}

		tenantID, err := o.pipeline.tenants.CreateTenant(ctx, req.CompanyName, slug, req.Plan)
		if err != nil {
			return saga, o.failSaga(ctx, saga, StepCreateTenant, fmt.Errorf("step 1 create tenant: %w", err))
		}
		saga.TenantID = tenantID
		saga.markStepCompleted(StepCreateTenant)
		o.persist(ctx, saga)
		o.logger.Info("saga step completed", "saga_id", saga.ID, "step", StepCreateTenant.String())
	}

	// --- Step 2: Create User ---
	if !saga.stepCompleted(StepCreateUser) {
		passwordHash, err := o.pipeline.hasher.Hash(req.Password)
		if err != nil {
			return saga, o.failAndCompensate(ctx, saga, StepCreateUser, fmt.Errorf("step 2 hash password: %w", err))
		}
		userID, err := o.pipeline.tenants.CreateUser(ctx, saga.TenantID, req.Email, req.CompanyName+" Admin", passwordHash)
		if err != nil {
			return saga, o.failAndCompensate(ctx, saga, StepCreateUser, fmt.Errorf("step 2 create user: %w", err))
		}
		saga.UserID = userID
		saga.markStepCompleted(StepCreateUser)
		o.persist(ctx, saga)
		o.logger.Info("saga step completed", "saga_id", saga.ID, "step", StepCreateUser.String())
	}

	// --- Step 3: Bind Vertical ---
	if !saga.stepCompleted(StepBindVertical) {
		if err := o.pipeline.verticals.BindTenantVertical(ctx, saga.TenantID, req.VerticalID, saga.UserID); err != nil {
			return saga, o.failAndCompensate(ctx, saga, StepBindVertical, fmt.Errorf("step 3 bind vertical: %w", err))
		}
		saga.markStepCompleted(StepBindVertical)
		o.persist(ctx, saga)
		o.logger.Info("saga step completed", "saga_id", saga.ID, "step", StepBindVertical.String())
	}

	// --- Step 4: Seed Chart of Accounts ---
	if !saga.stepCompleted(StepSeedChartOfAccounts) {
		if err := o.pipeline.ledger.SeedChartOfAccounts(ctx, saga.TenantID, cfg.DefaultChartOfAccounts); err != nil {
			return saga, o.failAndCompensate(ctx, saga, StepSeedChartOfAccounts, fmt.Errorf("step 4 seed chart of accounts: %w", err))
		}
		saga.markStepCompleted(StepSeedChartOfAccounts)
		o.persist(ctx, saga)
		o.logger.Info("saga step completed", "saga_id", saga.ID, "step", StepSeedChartOfAccounts.String())
	}

	// --- Step 5: Seed Budget Templates ---
	if !saga.stepCompleted(StepSeedBudgetTemplates) {
		if err := o.pipeline.budgets.SeedBudgetTemplates(ctx, saga.TenantID, cfg.BudgetTemplates); err != nil {
			return saga, o.failAndCompensate(ctx, saga, StepSeedBudgetTemplates, fmt.Errorf("step 5 seed budget templates: %w", err))
		}
		saga.markStepCompleted(StepSeedBudgetTemplates)
		o.persist(ctx, saga)
		o.logger.Info("saga step completed", "saga_id", saga.ID, "step", StepSeedBudgetTemplates.String())
	}

	// --- Step 6: Seed Expense Categories ---
	if !saga.stepCompleted(StepSeedExpenseCategories) {
		if err := o.pipeline.expenses.SeedExpenseCategories(ctx, saga.TenantID, cfg.ExpenseCategories); err != nil {
			return saga, o.failAndCompensate(ctx, saga, StepSeedExpenseCategories, fmt.Errorf("step 6 seed expense categories: %w", err))
		}
		saga.markStepCompleted(StepSeedExpenseCategories)
		o.persist(ctx, saga)
		o.logger.Info("saga step completed", "saga_id", saga.ID, "step", StepSeedExpenseCategories.String())
	}

	// --- Step 7: Open Accounting Period ---
	if !saga.stepCompleted(StepOpenAccountingPeriod) {
		now := time.Now().UTC()
		if err := o.pipeline.ledger.OpenAccountingPeriod(ctx, saga.TenantID, now.Year(), now.Month()); err != nil {
			return saga, o.failAndCompensate(ctx, saga, StepOpenAccountingPeriod, fmt.Errorf("step 7 open accounting period: %w", err))
		}
		saga.markStepCompleted(StepOpenAccountingPeriod)
		o.persist(ctx, saga)
		o.logger.Info("saga step completed", "saga_id", saga.ID, "step", StepOpenAccountingPeriod.String())
	}

	// --- Steps 8-9: Best-effort (failures recorded, not compensated) ---

	if !saga.stepCompleted(StepPublishEvent) {
		err := o.pipeline.events.PublishTenantCreated(ctx, TenantCreatedEvent{
			TenantID:    saga.TenantID,
			Name:        req.CompanyName,
			Slug:        Slugify(req.CompanyName),
			Plan:        req.Plan,
			VerticalID:  req.VerticalID,
			AdminEmail:  req.Email,
			AdminUserID: saga.UserID,
			OccurredAt:  time.Now().UTC(),
		})
		saga.markStepCompleted(StepPublishEvent) // mark even on failure — best-effort
		if err != nil {
			o.logger.Warn("saga step failed (best-effort, not compensating)",
				"saga_id", saga.ID, "step", StepPublishEvent.String(), "error", err.Error())
		} else {
			o.logger.Info("saga step completed", "saga_id", saga.ID, "step", StepPublishEvent.String())
		}
		o.persist(ctx, saga)
	}

	if !saga.stepCompleted(StepSendWelcomeEmail) {
		err := o.pipeline.notifier.SendWelcomeEmail(ctx, saga.TenantID, req.Email, req.CompanyName, vd.Name)
		saga.markStepCompleted(StepSendWelcomeEmail) // mark even on failure — best-effort
		if err != nil {
			o.logger.Warn("saga step failed (best-effort, not compensating)",
				"saga_id", saga.ID, "step", StepSendWelcomeEmail.String(), "error", err.Error())
		} else {
			o.logger.Info("saga step completed", "saga_id", saga.ID, "step", StepSendWelcomeEmail.String())
		}
		o.persist(ctx, saga)
	}

	saga.Status = SagaCompleted
	saga.UpdatedAt = time.Now().UTC()
	o.persist(ctx, saga)

	o.logger.Info("registration saga completed",
		"saga_id", saga.ID,
		"tenant_id", saga.TenantID,
		"email", saga.Email,
	)

	return saga, nil
}

// failAndCompensate marks the saga as failed at the given step and runs
// compensating transactions in reverse order for all completed steps.
func (o *Orchestrator) failAndCompensate(ctx context.Context, saga *RegistrationSaga, step Step, stepErr error) error {
	_ = o.failSaga(ctx, saga, step, stepErr)

	o.logger.Info("saga triggering compensation",
		"saga_id", saga.ID,
		"failed_step", step.String(),
		"completed_steps", len(saga.CompletedSteps),
	)

	saga.Status = SagaCompensating
	o.persist(ctx, saga)

	var compensationErrors []string

	// Compensate in reverse order: step 3 → 2 → 1.
	// Steps 4-7 write only to the tenant schema; schema is dropped by
	// DeleteTenant, so no explicit compensation is needed for them.
	// Steps 8-9 are best-effort and have no compensating action.
	if saga.stepCompleted(StepBindVertical) {
		if err := o.compensator.UnbindVertical(ctx, saga.TenantID, saga.Request.VerticalID); err != nil {
			o.logger.Error("compensation failed: UnbindVertical",
				"saga_id", saga.ID, "tenant_id", saga.TenantID, "error", err.Error())
			compensationErrors = append(compensationErrors, fmt.Sprintf("UnbindVertical: %v", err))
		} else {
			o.logger.Info("compensation: UnbindVertical succeeded", "saga_id", saga.ID)
		}
	}

	if saga.stepCompleted(StepCreateUser) {
		if err := o.compensator.DeleteUser(ctx, saga.TenantID, saga.UserID); err != nil {
			o.logger.Error("compensation failed: DeleteUser",
				"saga_id", saga.ID, "user_id", saga.UserID, "error", err.Error())
			compensationErrors = append(compensationErrors, fmt.Sprintf("DeleteUser: %v", err))
		} else {
			o.logger.Info("compensation: DeleteUser succeeded", "saga_id", saga.ID)
		}
	}

	if saga.stepCompleted(StepCreateTenant) {
		// DeleteTenant drops the schema and all seed data within it (steps 4-7).
		if err := o.compensator.DeleteTenant(ctx, saga.TenantID); err != nil {
			o.logger.Error("compensation failed: DeleteTenant",
				"saga_id", saga.ID, "tenant_id", saga.TenantID, "error", err.Error())
			compensationErrors = append(compensationErrors, fmt.Sprintf("DeleteTenant: %v", err))
		} else {
			o.logger.Info("compensation: DeleteTenant succeeded", "saga_id", saga.ID)
		}
	}

	if len(compensationErrors) > 0 {
		saga.Status = SagaCompensationFailed
		saga.CompensationErrors = compensationErrors
		o.logger.Error("saga compensation incomplete — manual intervention required",
			"saga_id", saga.ID, "errors", compensationErrors)
	} else {
		saga.Status = SagaCompensated
	}
	saga.UpdatedAt = time.Now().UTC()
	o.persist(ctx, saga)

	return stepErr
}

// failSaga records a critical step failure on the saga without running compensation.
// Use failAndCompensate when compensation is required.
func (o *Orchestrator) failSaga(ctx context.Context, saga *RegistrationSaga, step Step, err error) error {
	saga.Status = SagaFailed
	saga.FailedStep = step
	saga.FailureReason = err.Error()
	saga.UpdatedAt = time.Now().UTC()
	o.persist(ctx, saga)
	return err
}

// persist writes the saga state to the store. Errors are logged but not
// propagated — a state-write failure must not mask the original step error.
func (o *Orchestrator) persist(ctx context.Context, saga *RegistrationSaga) {
	saga.UpdatedAt = time.Now().UTC()
	if err := o.store.Update(ctx, saga); err != nil {
		o.logger.Error("saga state persist failed (non-fatal)",
			"saga_id", saga.ID, "status", string(saga.Status), "error", err.Error())
	}
}

// loadVerticalConfig is a helper that validates the vertical and parses its config.
// Extracted here so the orchestrator can call it directly (same logic as Pipeline.Run).
func (p *Pipeline) loadVerticalConfig(ctx context.Context, verticalID string) (VerticalDefinitionRow, *vertical.Config, error) {
	vd, err := p.verticals.GetVerticalDefinition(ctx, verticalID)
	if err != nil {
		return VerticalDefinitionRow{}, nil, fmt.Errorf("validate vertical: %w", err)
	}
	if !vd.IsActive {
		return VerticalDefinitionRow{}, nil, ErrVerticalInactive
	}
	var cfg vertical.Config
	if err := json.Unmarshal(vd.Config, &cfg); err != nil {
		return VerticalDefinitionRow{}, nil, fmt.Errorf("parse vertical config: %w", err)
	}
	return vd, &cfg, nil
}
