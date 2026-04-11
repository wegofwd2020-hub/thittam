package db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/wegofwd2020/thittam/services/reporting"
)

// Postgres implements reporting.Repository backed by PostgreSQL.
type Postgres struct {
	q  *Queries
	db *pgxpool.Pool
}

// NewPostgres creates a Postgres-backed reporting repository.
func NewPostgres(db *pgxpool.Pool) *Postgres {
	return &Postgres{q: New(db), db: db}
}

// Compile-time interface check.
var _ reporting.Repository = (*Postgres)(nil)

// --- Expense Facts ---

func (p *Postgres) GetExpenseFacts(ctx context.Context, tenantID, productionID uuid.UUID) ([]reporting.ExpenseFact, error) {
	const sql = `SELECT expense_id, production_id, tenant_id, budget_line_id, category_id, amount, approved_at
		FROM expense_facts WHERE tenant_id = $1 AND production_id = $2`

	rows, err := p.db.Query(ctx, sql, tenantID, productionID)
	if err != nil {
		return nil, fmt.Errorf("reporting: get expense facts: %w", err)
	}
	defer rows.Close()

	var result []reporting.ExpenseFact
	for rows.Next() {
		var (
			expenseID    uuid.UUID
			prodID       uuid.UUID
			tenID        uuid.UUID
			budgetLineID pgtype.UUID
			categoryID   string
			amount       pgtype.Numeric
			approvedAt   pgtype.Timestamptz
		)
		if err := rows.Scan(&expenseID, &prodID, &tenID, &budgetLineID, &categoryID, &amount, &approvedAt); err != nil {
			return nil, fmt.Errorf("reporting: scan expense fact: %w", err)
		}
		f := reporting.ExpenseFact{
			ExpenseID:    expenseID,
			ProductionID: prodID,
			TenantID:     tenID,
			CategoryID:   categoryID,
			Amount:       decimalFromPgNumeric(amount),
		}
		if budgetLineID.Valid {
			uid := uuid.UUID(budgetLineID.Bytes)
			f.BudgetLineID = &uid
		}
		if approvedAt.Valid {
			t := approvedAt.Time
			f.ApprovedAt = &t
		}
		result = append(result, f)
	}
	return result, rows.Err()
}

// --- Budget Facts ---

func (p *Postgres) GetBudgetFacts(ctx context.Context, tenantID, productionID uuid.UUID) ([]reporting.BudgetFact, error) {
	const sql = `SELECT budget_id, production_id, tenant_id, total_budgeted, total_actual, total_committed
		FROM budget_facts WHERE tenant_id = $1 AND production_id = $2`

	rows, err := p.db.Query(ctx, sql, tenantID, productionID)
	if err != nil {
		return nil, fmt.Errorf("reporting: get budget facts: %w", err)
	}
	defer rows.Close()

	var result []reporting.BudgetFact
	for rows.Next() {
		var (
			budgetID       uuid.UUID
			prodID         uuid.UUID
			tenID          uuid.UUID
			totalBudgeted  pgtype.Numeric
			totalActual    pgtype.Numeric
			totalCommitted pgtype.Numeric
		)
		if err := rows.Scan(&budgetID, &prodID, &tenID, &totalBudgeted, &totalActual, &totalCommitted); err != nil {
			return nil, fmt.Errorf("reporting: scan budget fact: %w", err)
		}
		result = append(result, reporting.BudgetFact{
			BudgetID:       budgetID,
			ProductionID:   prodID,
			TenantID:       tenID,
			TotalBudgeted:  decimalFromPgNumeric(totalBudgeted),
			TotalActual:    decimalFromPgNumeric(totalActual),
			TotalCommitted: decimalFromPgNumeric(totalCommitted),
		})
	}
	return result, rows.Err()
}

// --- Dashboard Summary ---

func (p *Postgres) GetDashboardSummary(ctx context.Context, tenantID uuid.UUID) (*reporting.DashboardSummary, error) {
	stats, err := p.q.GetTenantDashboardStats(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("reporting: get dashboard stats: %w", err)
	}

	const aggSQL = `SELECT
		COALESCE(SUM(budget_total), 0),
		COALESCE(SUM(actual_spend), 0),
		COALESCE(SUM(committed),    0)
		FROM portfolio_snapshots WHERE tenant_id = $1`

	var totalBudgetedN, totalActualN, totalCommittedN pgtype.Numeric
	if err := p.db.QueryRow(ctx, aggSQL, tenantID).Scan(&totalBudgetedN, &totalActualN, &totalCommittedN); err != nil {
		return nil, fmt.Errorf("reporting: get portfolio totals: %w", err)
	}

	budgeted := decimalFromPgNumeric(totalBudgetedN)
	actual := decimalFromPgNumeric(totalActualN)
	committed := decimalFromPgNumeric(totalCommittedN)

	return &reporting.DashboardSummary{
		TenantID:       tenantID,
		ProjectCount:   int(stats.ProductionCount),
		TotalBudgeted:  budgeted,
		TotalActual:    actual,
		TotalCommitted: committed,
		Variance:       budgeted.Sub(actual),
	}, nil
}

// --- Portfolio Overview ---

func (p *Postgres) GetPortfolioOverview(ctx context.Context, tenantID uuid.UUID) (*reporting.PortfolioOverview, error) {
	const sql = `SELECT project_id, title, status, current_phase, budget_total, actual_spend, committed, start_date
		FROM portfolio_snapshots WHERE tenant_id = $1 AND status = 'active' ORDER BY title`

	rows, err := p.db.Query(ctx, sql, tenantID)
	if err != nil {
		return nil, fmt.Errorf("reporting: get portfolio snapshots: %w", err)
	}
	defer rows.Close()

	var projects []reporting.ProjectSummary
	var totalBudget, totalActual decimal.Decimal
	health := reporting.BudgetHealthCount{}

	for rows.Next() {
		var (
			projectID    uuid.UUID
			title        string
			status       string
			currentPhase pgtype.Text
			budgetTotal  pgtype.Numeric
			actualSpend  pgtype.Numeric
			committed    pgtype.Numeric
			startDate    pgtype.Date
		)
		if err := rows.Scan(&projectID, &title, &status, &currentPhase, &budgetTotal, &actualSpend, &committed, &startDate); err != nil {
			return nil, fmt.Errorf("reporting: scan portfolio snapshot: %w", err)
		}

		budget := decimalFromPgNumeric(budgetTotal)
		actual := decimalFromPgNumeric(actualSpend)

		var pct decimal.Decimal
		if budget.IsPositive() {
			pct = actual.Div(budget).Mul(decimal.NewFromInt(100))
		}

		h := classifyHealth(pct)
		switch h {
		case "on_track":
			health.OnTrack++
		case "at_risk":
			health.AtRisk++
		case "over_budget":
			health.OverBudget++
		}

		totalBudget = totalBudget.Add(budget)
		totalActual = totalActual.Add(actual)

		ps := reporting.ProjectSummary{
			ID:            projectID,
			Title:         title,
			Status:        status,
			BudgetTotal:   budget,
			ActualSpend:   actual,
			BudgetUsedPct: pct,
			Health:        h,
		}
		if currentPhase.Valid {
			ps.CurrentPhase = currentPhase.String
		}
		if startDate.Valid {
			s := startDate.Time.Format(time.DateOnly)
			ps.StartDate = &s
		}
		projects = append(projects, ps)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reporting: portfolio rows: %w", err)
	}

	return &reporting.PortfolioOverview{
		TenantID:     tenantID,
		TotalActive:  len(projects),
		TotalBudget:  totalBudget,
		TotalActual:  totalActual,
		HealthCounts: health,
		Projects:     projects,
	}, nil
}

// --- Financial Summary ---

func (p *Postgres) GetFinancialSummary(ctx context.Context, tenantID uuid.UUID) (*reporting.FinancialSummary, error) {
	const aggSQL = `SELECT
		COALESCE(SUM(budget_total), 0),
		COALESCE(SUM(actual_spend), 0),
		COALESCE(SUM(committed),    0)
		FROM portfolio_snapshots WHERE tenant_id = $1`

	var totalBudgetedN, totalActualN, totalCommittedN pgtype.Numeric
	if err := p.db.QueryRow(ctx, aggSQL, tenantID).Scan(&totalBudgetedN, &totalActualN, &totalCommittedN); err != nil {
		return nil, fmt.Errorf("reporting: financial totals: %w", err)
	}
	totalBudgeted := decimalFromPgNumeric(totalBudgetedN)
	totalActual := decimalFromPgNumeric(totalActualN)
	totalCommitted := decimalFromPgNumeric(totalCommittedN)
	totalAvailable := totalBudgeted.Sub(totalActual).Sub(totalCommitted)

	const catSQL = `SELECT category_id, category_name, budgeted, actual
		FROM category_spend WHERE tenant_id = $1 ORDER BY actual DESC`

	catRows, err := p.db.Query(ctx, catSQL, tenantID)
	if err != nil {
		return nil, fmt.Errorf("reporting: category spend: %w", err)
	}
	defer catRows.Close()

	var byCategory []reporting.CategorySpend
	for catRows.Next() {
		var (
			categoryID   string
			categoryName string
			budgeted     pgtype.Numeric
			actual       pgtype.Numeric
		)
		if err := catRows.Scan(&categoryID, &categoryName, &budgeted, &actual); err != nil {
			return nil, fmt.Errorf("reporting: scan category spend: %w", err)
		}
		act := decimalFromPgNumeric(actual)
		var pct decimal.Decimal
		if totalActual.IsPositive() {
			pct = act.Div(totalActual).Mul(decimal.NewFromInt(100))
		}
		byCategory = append(byCategory, reporting.CategorySpend{
			CategoryID:   categoryID,
			CategoryName: categoryName,
			Budgeted:     decimalFromPgNumeric(budgeted),
			Actual:       act,
			Percentage:   pct,
		})
	}
	if err := catRows.Err(); err != nil {
		return nil, fmt.Errorf("reporting: category rows: %w", err)
	}

	const monthSQL = `SELECT month, actual, budget FROM monthly_spend
		WHERE tenant_id = $1 ORDER BY month DESC LIMIT 12`

	mRows, err := p.db.Query(ctx, monthSQL, tenantID)
	if err != nil {
		return nil, fmt.Errorf("reporting: monthly spend: %w", err)
	}
	defer mRows.Close()

	var monthly []reporting.MonthlySpend
	for mRows.Next() {
		var (
			month  string
			actual pgtype.Numeric
			budget pgtype.Numeric
		)
		if err := mRows.Scan(&month, &actual, &budget); err != nil {
			return nil, fmt.Errorf("reporting: scan monthly spend: %w", err)
		}
		monthly = append(monthly, reporting.MonthlySpend{
			Month:  month,
			Actual: decimalFromPgNumeric(actual),
			Budget: decimalFromPgNumeric(budget),
		})
	}
	if err := mRows.Err(); err != nil {
		return nil, fmt.Errorf("reporting: monthly rows: %w", err)
	}

	// Burn rate = average monthly actual over the most recent 3 months.
	var burnRate decimal.Decimal
	if n := len(monthly); n > 0 {
		window := n
		if window > 3 {
			window = 3
		}
		var sum decimal.Decimal
		for i := 0; i < window; i++ {
			sum = sum.Add(monthly[i].Actual)
		}
		burnRate = sum.Div(decimal.NewFromInt(int64(window)))
	}

	return &reporting.FinancialSummary{
		TenantID:       tenantID,
		TotalBudgeted:  totalBudgeted,
		TotalActual:    totalActual,
		TotalCommitted: totalCommitted,
		TotalAvailable: totalAvailable,
		BurnRate:       burnRate,
		ByCategory:     byCategory,
		MonthlyTrend:   monthly,
	}, nil
}

// --- Approval Pipeline ---

func (p *Postgres) GetApprovalPipeline(ctx context.Context, tenantID uuid.UUID) (*reporting.ApprovalPipeline, error) {
	const sql = `SELECT id, item_type, description, amount, submitted_by, submitted_at, project_title
		FROM pending_approvals WHERE tenant_id = $1 ORDER BY submitted_at`

	rows, err := p.db.Query(ctx, sql, tenantID)
	if err != nil {
		return nil, fmt.Errorf("reporting: get pending approvals: %w", err)
	}
	defer rows.Close()

	var items []reporting.PendingApproval
	var totalAmount decimal.Decimal
	bands := reporting.ApprovalAgeBands{}
	now := time.Now()

	for rows.Next() {
		var (
			id           uuid.UUID
			itemType     string
			description  string
			amount       pgtype.Numeric
			submittedBy  string
			submittedAt  time.Time
			projectTitle pgtype.Text
		)
		if err := rows.Scan(&id, &itemType, &description, &amount, &submittedBy, &submittedAt, &projectTitle); err != nil {
			return nil, fmt.Errorf("reporting: scan pending approval: %w", err)
		}

		amt := decimalFromPgNumeric(amount)
		totalAmount = totalAmount.Add(amt)
		ageHours := int(now.Sub(submittedAt).Hours())
		switch {
		case ageHours < 24:
			bands.Under24h++
		case ageHours < 72:
			bands.OneToThreeDays++
		case ageHours < 168:
			bands.ThreeToSevenDays++
		default:
			bands.OverSevenDays++
		}

		projTitle := ""
		if projectTitle.Valid {
			projTitle = projectTitle.String
		}
		items = append(items, reporting.PendingApproval{
			ID:           id,
			Type:         itemType,
			Description:  description,
			Amount:       amt,
			SubmittedBy:  submittedBy,
			SubmittedAt:  submittedAt,
			ProjectTitle: projTitle,
			AgeHours:     ageHours,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reporting: approval rows: %w", err)
	}

	return &reporting.ApprovalPipeline{
		TenantID:     tenantID,
		TotalPending: len(items),
		TotalAmount:  totalAmount,
		ByAge:        bands,
		PendingItems: items,
	}, nil
}

// --- Team Utilization ---

func (p *Postgres) GetTeamUtilization(ctx context.Context, tenantID uuid.UUID) (*reporting.TeamUtilization, error) {
	var totalMembers int64
	if err := p.db.QueryRow(ctx, "SELECT COUNT(*) FROM crew_members WHERE tenant_id = $1", tenantID).Scan(&totalMembers); err != nil {
		return nil, fmt.Errorf("reporting: count crew members: %w", err)
	}

	const sql = `SELECT project_id, project_title, member_count
		FROM team_assignments WHERE tenant_id = $1 ORDER BY member_count DESC`

	rows, err := p.db.Query(ctx, sql, tenantID)
	if err != nil {
		return nil, fmt.Errorf("reporting: get team assignments: %w", err)
	}
	defer rows.Close()

	var byProject []reporting.ProjectTeam
	var totalAssigned int64
	for rows.Next() {
		var (
			projectID    uuid.UUID
			projectTitle string
			memberCount  int32
		)
		if err := rows.Scan(&projectID, &projectTitle, &memberCount); err != nil {
			return nil, fmt.Errorf("reporting: scan team assignment: %w", err)
		}
		totalAssigned += int64(memberCount)
		byProject = append(byProject, reporting.ProjectTeam{
			ProjectID:    projectID,
			ProjectTitle: projectTitle,
			MemberCount:  int(memberCount),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reporting: team rows: %w", err)
	}

	assigned := int(totalAssigned)
	available := int(totalMembers) - assigned
	if available < 0 {
		available = 0
	}

	var utilizationPct decimal.Decimal
	if totalMembers > 0 {
		utilizationPct = decimal.NewFromInt(totalAssigned).
			Div(decimal.NewFromInt(totalMembers)).
			Mul(decimal.NewFromInt(100))
	}

	return &reporting.TeamUtilization{
		TenantID:       tenantID,
		TotalMembers:   int(totalMembers),
		Assigned:       assigned,
		Available:      available,
		UtilizationPct: utilizationPct,
		ByProject:      byProject,
	}, nil
}

// --- Compliance Status ---

func (p *Postgres) GetComplianceStatus(ctx context.Context, tenantID uuid.UUID) (*reporting.ComplianceStatus, error) {
	const sql = `SELECT id, flag_type, description, severity, project_title, detected_at
		FROM compliance_flags WHERE tenant_id = $1 ORDER BY detected_at DESC LIMIT 50`

	rows, err := p.db.Query(ctx, sql, tenantID)
	if err != nil {
		return nil, fmt.Errorf("reporting: get compliance flags: %w", err)
	}
	defer rows.Close()

	var items []reporting.ComplianceItem
	var overdueApprovals, policyViolations, auditFlags int

	for rows.Next() {
		var (
			id           uuid.UUID
			flagType     string
			description  string
			severity     string
			projectTitle pgtype.Text
			detectedAt   time.Time
		)
		if err := rows.Scan(&id, &flagType, &description, &severity, &projectTitle, &detectedAt); err != nil {
			return nil, fmt.Errorf("reporting: scan compliance flag: %w", err)
		}
		switch flagType {
		case "overdue_approval":
			overdueApprovals++
		case "missing_po", "over_limit":
			policyViolations++
		default:
			auditFlags++
		}
		projTitle := ""
		if projectTitle.Valid {
			projTitle = projectTitle.String
		}
		items = append(items, reporting.ComplianceItem{
			ID:           id,
			Type:         flagType,
			Description:  description,
			Severity:     severity,
			DetectedAt:   detectedAt,
			ProjectTitle: projTitle,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reporting: compliance rows: %w", err)
	}

	return &reporting.ComplianceStatus{
		TenantID:         tenantID,
		OverdueApprovals: overdueApprovals,
		PolicyViolations: policyViolations,
		AuditFlags:       auditFlags,
		RecentViolations: items,
	}, nil
}

// --- helpers ---

// classifyHealth categorises budget usage percentage into a health string.
func classifyHealth(pct decimal.Decimal) string {
	switch {
	case pct.GreaterThan(decimal.NewFromInt(100)):
		return "over_budget"
	case pct.GreaterThan(decimal.NewFromInt(80)):
		return "at_risk"
	default:
		return "on_track"
	}
}

func decimalFromPgNumeric(n pgtype.Numeric) decimal.Decimal {
	if !n.Valid || n.NaN || n.Int == nil {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(n.Int, n.Exp)
}
