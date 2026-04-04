package registration

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ValidPlans lists the allowed subscription plans.
var ValidPlans = []string{"starter", "professional", "enterprise"}

// Step identifies a step in the registration pipeline.
type Step int

const (
	StepCreateTenant Step = iota + 1
	StepCreateUser
	StepBindVertical
	StepSeedChartOfAccounts
	StepSeedBudgetTemplates
	StepSeedExpenseCategories
	StepOpenAccountingPeriod
	StepPublishEvent
	StepSendWelcomeEmail
)

// String returns a human-readable name for the step.
func (s Step) String() string {
	switch s {
	case StepCreateTenant:
		return "create_tenant"
	case StepCreateUser:
		return "create_user"
	case StepBindVertical:
		return "bind_vertical"
	case StepSeedChartOfAccounts:
		return "seed_chart_of_accounts"
	case StepSeedBudgetTemplates:
		return "seed_budget_templates"
	case StepSeedExpenseCategories:
		return "seed_expense_categories"
	case StepOpenAccountingPeriod:
		return "open_accounting_period"
	case StepPublishEvent:
		return "publish_event"
	case StepSendWelcomeEmail:
		return "send_welcome_email"
	default:
		return "unknown"
	}
}

// StepStatus represents the outcome of a pipeline step.
type StepStatus string

const (
	StepCompleted StepStatus = "completed"
	StepFailed    StepStatus = "failed"
	StepSkipped   StepStatus = "skipped"
)

// StepResult records the outcome of a single pipeline step.
type StepResult struct {
	Step    Step          `json:"step"`
	Name    string        `json:"name"`
	Status  StepStatus    `json:"status"`
	Error   string        `json:"error,omitempty"`
	Elapsed time.Duration `json:"elapsed_ms"`
}

// RegisterRequest contains the input for tenant registration.
type RegisterRequest struct {
	CompanyName string
	Email       string
	Password    string
	VerticalID  string
	Plan        string
}

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// Validate checks all fields and returns a descriptive error on failure.
func (r *RegisterRequest) Validate() error {
	r.CompanyName = strings.TrimSpace(r.CompanyName)
	r.Email = strings.TrimSpace(strings.ToLower(r.Email))
	r.Plan = strings.TrimSpace(strings.ToLower(r.Plan))
	r.VerticalID = strings.TrimSpace(r.VerticalID)

	if r.CompanyName == "" {
		return fmt.Errorf("%w: company_name is required", ErrInvalidRequest)
	}
	if len(r.CompanyName) < 2 || len(r.CompanyName) > 200 {
		return fmt.Errorf("%w: company_name must be 2-200 characters", ErrInvalidRequest)
	}
	if r.Email == "" {
		return fmt.Errorf("%w: email is required", ErrInvalidRequest)
	}
	if !emailRegex.MatchString(r.Email) {
		return fmt.Errorf("%w: email format is invalid", ErrInvalidRequest)
	}
	if r.Password == "" {
		return fmt.Errorf("%w: password is required", ErrInvalidRequest)
	}
	if len(r.Password) < 8 {
		return fmt.Errorf("%w: password must be at least 8 characters", ErrInvalidRequest)
	}
	if r.VerticalID == "" {
		return fmt.Errorf("%w: vertical_id is required", ErrInvalidRequest)
	}
	if !isValidPlan(r.Plan) {
		return fmt.Errorf("%w: plan must be one of: %s", ErrInvalidPlan, strings.Join(ValidPlans, ", "))
	}
	return nil
}

func isValidPlan(plan string) bool {
	for _, p := range ValidPlans {
		if p == plan {
			return true
		}
	}
	return false
}

// RegisterResult is returned after a successful (or partially successful) registration.
type RegisterResult struct {
	TenantID       uuid.UUID    `json:"tenant_id"`
	UserID         uuid.UUID    `json:"user_id"`
	VerticalID     string       `json:"vertical_id"`
	Plan           string       `json:"plan"`
	CompletedSteps []StepResult `json:"completed_steps"`
}
