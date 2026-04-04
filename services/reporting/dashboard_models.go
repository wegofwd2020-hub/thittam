package reporting

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ── Portfolio Overview ──

// PortfolioOverview provides a bird's-eye view of all active projects.
type PortfolioOverview struct {
	TenantID     uuid.UUID         `json:"tenant_id"`
	ProjectLabel string            `json:"project_label"` // vertical entity label
	TotalActive  int               `json:"total_active"`
	TotalBudget  decimal.Decimal   `json:"total_budget"`
	TotalActual  decimal.Decimal   `json:"total_actual"`
	HealthCounts BudgetHealthCount `json:"health_counts"`
	Projects     []ProjectSummary  `json:"projects"`
}

// BudgetHealthCount categorises projects by budget health.
type BudgetHealthCount struct {
	OnTrack  int `json:"on_track"`  // actual <= 80% of budget
	AtRisk   int `json:"at_risk"`   // actual 80-100% of budget
	OverBudget int `json:"over_budget"` // actual > budget
}

// ProjectSummary is a single row in the portfolio overview.
type ProjectSummary struct {
	ID            uuid.UUID       `json:"id"`
	Title         string          `json:"title"`
	Status        string          `json:"status"`
	CurrentPhase  string          `json:"current_phase"`
	BudgetTotal   decimal.Decimal `json:"budget_total"`
	ActualSpend   decimal.Decimal `json:"actual_spend"`
	BudgetUsedPct decimal.Decimal `json:"budget_used_pct"` // (actual/budget)*100
	Health        string          `json:"health"`           // on_track, at_risk, over_budget
	StartDate     *string         `json:"start_date,omitempty"`
}

// ── Financial Summary ──

// FinancialSummary aggregates financial data across all projects.
type FinancialSummary struct {
	TenantID        uuid.UUID          `json:"tenant_id"`
	TotalBudgeted   decimal.Decimal    `json:"total_budgeted"`
	TotalActual     decimal.Decimal    `json:"total_actual"`
	TotalCommitted  decimal.Decimal    `json:"total_committed"`
	TotalAvailable  decimal.Decimal    `json:"total_available"` // budgeted - actual - committed
	BurnRate        decimal.Decimal    `json:"burn_rate_per_month"`
	ByCategory      []CategorySpend    `json:"by_category"`
	MonthlyTrend    []MonthlySpend     `json:"monthly_trend"`
}

// CategorySpend shows spend breakdown by vertical budget category.
type CategorySpend struct {
	CategoryID   string          `json:"category_id"`
	CategoryName string          `json:"category_name"`
	Budgeted     decimal.Decimal `json:"budgeted"`
	Actual       decimal.Decimal `json:"actual"`
	Percentage   decimal.Decimal `json:"percentage"` // of total actual
}

// MonthlySpend tracks spend over time for trend analysis.
type MonthlySpend struct {
	Month  string          `json:"month"` // "2026-01", "2026-02", etc.
	Actual decimal.Decimal `json:"actual"`
	Budget decimal.Decimal `json:"budget"` // monthly allocation
}

// ── Approval Pipeline ──

// ApprovalPipeline shows pending approvals across budget and expense workflows.
type ApprovalPipeline struct {
	TenantID       uuid.UUID        `json:"tenant_id"`
	TotalPending   int              `json:"total_pending"`
	TotalAmount    decimal.Decimal  `json:"total_amount"`
	ByAge          ApprovalAgeBands `json:"by_age"`
	PendingItems   []PendingApproval `json:"pending_items"`
}

// ApprovalAgeBands categorises pending approvals by age.
type ApprovalAgeBands struct {
	Under24h  int `json:"under_24h"`
	OneToThreeDays int `json:"1_to_3_days"`
	ThreeToSevenDays int `json:"3_to_7_days"`
	OverSevenDays int `json:"over_7_days"`
}

// PendingApproval is a single item awaiting approval.
type PendingApproval struct {
	ID           uuid.UUID       `json:"id"`
	Type         string          `json:"type"` // "expense" or "budget"
	Description  string          `json:"description"`
	Amount       decimal.Decimal `json:"amount"`
	SubmittedBy  string          `json:"submitted_by"`
	SubmittedAt  time.Time       `json:"submitted_at"`
	ProjectTitle string          `json:"project_title"`
	AgeHours     int             `json:"age_hours"`
}

// ── Team Utilization ──

// TeamUtilization shows crew/team allocation across projects.
type TeamUtilization struct {
	TenantID        uuid.UUID       `json:"tenant_id"`
	TeamMemberLabel string          `json:"team_member_label"` // vertical entity label
	TotalMembers    int             `json:"total_members"`
	Assigned        int             `json:"assigned"`   // currently on a project
	Available       int             `json:"available"`  // not assigned
	UtilizationPct  decimal.Decimal `json:"utilization_pct"`
	ByProject       []ProjectTeam   `json:"by_project"`
}

// ProjectTeam shows team allocation for a single project.
type ProjectTeam struct {
	ProjectID    uuid.UUID `json:"project_id"`
	ProjectTitle string    `json:"project_title"`
	MemberCount  int       `json:"member_count"`
}

// ── Compliance Status ──

// ComplianceStatus flags overdue approvals and policy violations.
type ComplianceStatus struct {
	TenantID           uuid.UUID          `json:"tenant_id"`
	OverdueApprovals   int                `json:"overdue_approvals"`   // pending > 7 days
	PolicyViolations   int                `json:"policy_violations"`   // e.g., expenses without PO
	AuditFlags         int                `json:"audit_flags"`
	RecentViolations   []ComplianceItem   `json:"recent_violations"`
}

// ComplianceItem is a single compliance flag.
type ComplianceItem struct {
	ID          uuid.UUID `json:"id"`
	Type        string    `json:"type"`        // "overdue_approval", "missing_po", "over_limit"
	Description string    `json:"description"`
	Severity    string    `json:"severity"`    // "warning", "critical"
	DetectedAt  time.Time `json:"detected_at"`
	ProjectTitle string   `json:"project_title"`
}

// ── Vertical Breakdown ──

// VerticalBreakdown segments metrics by industry vertical (platform admin view).
type VerticalBreakdown struct {
	Verticals []VerticalMetrics `json:"verticals"`
}

// VerticalMetrics aggregates per-vertical across all tenants.
type VerticalMetrics struct {
	VerticalID    string          `json:"vertical_id"`
	VerticalName  string          `json:"vertical_name"`
	TenantCount   int             `json:"tenant_count"`
	ProjectCount  int             `json:"project_count"`
	TotalBudget   decimal.Decimal `json:"total_budget"`
	TotalActual   decimal.Decimal `json:"total_actual"`
}
