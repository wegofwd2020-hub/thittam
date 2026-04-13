// Package critical_path_test contains critical-path end-to-end tests that run
// on every PR. They wire real service implementations through in-process mock
// repositories — no database, no Docker, no network — targeting ≤5 min total.
//
// These tests verify the five most important flows end-to-end:
//  1. Tenant registration (IAM)
//  2. Budget creation → approval lifecycle
//  3. Expense submission → approval lifecycle
//  4. Double-entry ledger posting
//  5. Report generation with vertical context
package critical_path_test

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/wegofwd2020/thittam/pkg/auth"
	"github.com/wegofwd2020/thittam/pkg/vertical"
	"github.com/wegofwd2020/thittam/services/budget"
	"github.com/wegofwd2020/thittam/services/expense"
	"github.com/wegofwd2020/thittam/services/iam"
	"github.com/wegofwd2020/thittam/services/ledger"
	"github.com/wegofwd2020/thittam/services/reporting"
)

// ── Deterministic fixture IDs ──────────────────────────────────────────────

var (
	fixedTenantID    = uuid.MustParse("a0000000-0000-0000-0000-000000000001")
	fixedUserID      = uuid.MustParse("a0000000-0000-0000-0000-000000000002")
	fixedBudgetID    = uuid.MustParse("a0000000-0000-0000-0000-000000000003")
	fixedExpenseID   = uuid.MustParse("a0000000-0000-0000-0000-000000000004")
	fixedLedgerID    = uuid.MustParse("a0000000-0000-0000-0000-000000000005")
	fixedProductionID = uuid.MustParse("a0000000-0000-0000-0000-000000000006")
	fixedPeriodID    = uuid.MustParse("a0000000-0000-0000-0000-000000000007")
	fixedAccountID1  = uuid.MustParse("a0000000-0000-0000-0000-000000000008")
	fixedAccountID2  = uuid.MustParse("a0000000-0000-0000-0000-000000000009")
)

// movieVerticalCtx returns a context pre-loaded with the movie-production vertical config.
func movieVerticalCtx() context.Context {
	cfg := &vertical.Config{
		ID:      "movie-production",
		Name:    "Movie & Film Production",
		Version: "1.0.0",
		EntityLabels: vertical.EntityLabels{
			Project:          "Production",
			ProjectPlural:    "Productions",
			TeamMemberPlural: "Crew Members",
		},
		BudgetCategories: []vertical.BudgetCategory{
			{ID: "above_the_line", Label: "Above the Line", DefaultAccountCode: "5100"},
		},
		ExpenseCategories: []vertical.ExpenseCategory{
			{ID: "location_rental", Label: "Location Rental", TaxTreatment: "input_gst", RequiresPO: false},
		},
		ApprovalWorkflow: vertical.ApprovalWorkflow{
			Limits: []vertical.ApprovalLimit{
				{Role: "line_producer", MaxAmount: decimal.NewFromInt(500000)},
			},
			DualApprovalAbove: decimal.NewFromInt(1000000),
		},
		ReportDefinitions: []vertical.ReportDefinition{
			{ID: "cost_report", Name: "Daily Cost Report"},
		},
	}
	return vertical.WithConfig(context.Background(), cfg)
}

// ── IAM service stubs ─────────────────────────────────────────────────────

type stubHasher struct{}

func (stubHasher) Hash(_ string) (string, error) { return "$stub$hash", nil }

type stubAuthenticator struct{}

func (stubAuthenticator) Authenticate(_ context.Context, _ auth.AuthRequest) (*auth.AuthResult, error) {
	return nil, nil
}

type stubTokenIssuer struct{}

func (stubTokenIssuer) Issue(_ context.Context, _ *auth.AuthResult) (*auth.TokenPair, error) {
	return &auth.TokenPair{}, nil
}
func (stubTokenIssuer) Refresh(_ context.Context, _ string) (*auth.TokenPair, error) {
	return &auth.TokenPair{}, nil
}
func (stubTokenIssuer) Revoke(_ context.Context, _ string) error   { return nil }
func (stubTokenIssuer) Validate(_ context.Context, _ string) (*auth.Claims, error) {
	return &auth.Claims{}, nil
}

type stubVerifier struct{}

func (stubVerifier) Verify(_, _ string) error { return nil }

// ── IAM mock repository ────────────────────────────────────────────────────

type iamRepo struct {
	tenants map[uuid.UUID]*iam.Tenant
	users   map[uuid.UUID]*iam.User
	roles   []*iam.Role
}

func newIAMRepo() *iamRepo {
	return &iamRepo{
		tenants: make(map[uuid.UUID]*iam.Tenant),
		users:   make(map[uuid.UUID]*iam.User),
	}
}

// auth.UserStore
func (r *iamRepo) GetUserByEmail(_ context.Context, _ uuid.UUID, _ string) (*auth.UserRecord, error) {
	return nil, nil
}
func (r *iamRepo) GetUserByID(_ context.Context, _ uuid.UUID) (*auth.UserRecord, error) {
	return nil, nil
}
func (r *iamRepo) CreateOIDCUser(_ context.Context, _ uuid.UUID, _, _ string) (*auth.UserRecord, error) {
	return nil, nil
}

// auth.TenantStore
func (r *iamRepo) GetTenantStatus(_ context.Context, id uuid.UUID) (string, error) {
	if t, ok := r.tenants[id]; ok {
		return t.Status, nil
	}
	return "", iam.ErrTenantNotFound
}

// Users
func (r *iamRepo) CreateUser(_ context.Context, u *iam.User) error {
	r.users[u.ID] = u
	return nil
}
func (r *iamRepo) GetUser(_ context.Context, _, id uuid.UUID) (*iam.User, error) {
	if u, ok := r.users[id]; ok {
		return u, nil
	}
	return nil, iam.ErrUserNotFound
}
func (r *iamRepo) ListUsers(_ context.Context, _ uuid.UUID, _ string, _, _ int) ([]iam.User, error) {
	return nil, nil
}
func (r *iamRepo) UpdateUser(_ context.Context, u *iam.User) error {
	r.users[u.ID] = u
	return nil
}
func (r *iamRepo) UpdatePasswordHash(_ context.Context, _ uuid.UUID, _ string) error { return nil }
func (r *iamRepo) DeactivateUser(_ context.Context, _, _ uuid.UUID) error              { return nil }

// Tenants
func (r *iamRepo) CreateTenant(_ context.Context, t *iam.Tenant) error {
	r.tenants[t.ID] = t
	return nil
}
func (r *iamRepo) GetTenant(_ context.Context, id uuid.UUID) (*iam.Tenant, error) {
	if t, ok := r.tenants[id]; ok {
		return t, nil
	}
	return nil, iam.ErrTenantNotFound
}
func (r *iamRepo) UpdateTenantStatus(_ context.Context, id uuid.UUID, status string) error {
	if t, ok := r.tenants[id]; ok {
		t.Status = status
		return nil
	}
	return iam.ErrTenantNotFound
}

// Roles
func (r *iamRepo) CreateRole(_ context.Context, role *iam.Role) error {
	r.roles = append(r.roles, role)
	return nil
}
func (r *iamRepo) GetRole(_ context.Context, _ uuid.UUID, name string) (*iam.Role, error) {
	for _, ro := range r.roles {
		if ro.Name == name {
			return ro, nil
		}
	}
	return nil, iam.ErrRoleNotFound
}
func (r *iamRepo) GetRoleByID(_ context.Context, _, roleID uuid.UUID) (*iam.Role, error) {
	for _, ro := range r.roles {
		if ro.ID == roleID {
			return ro, nil
		}
	}
	return nil, iam.ErrRoleNotFound
}
func (r *iamRepo) ListRoles(_ context.Context, _ uuid.UUID) ([]iam.Role, error) {
	out := make([]iam.Role, len(r.roles))
	for i, ro := range r.roles {
		out[i] = *ro
	}
	return out, nil
}
func (r *iamRepo) AssignRole(_ context.Context, _ *iam.UserRole) error   { return nil }
func (r *iamRepo) RevokeRole(_ context.Context, _, _ uuid.UUID) error    { return nil }
func (r *iamRepo) GetUserPermissions(_ context.Context, _ uuid.UUID, _ *uuid.UUID) ([]string, error) {
	return nil, nil
}

// Invitations
func (r *iamRepo) CreateInvitation(_ context.Context, _ *iam.Invitation) error { return nil }
func (r *iamRepo) GetInvitationByToken(_ context.Context, _ string) (*iam.Invitation, error) {
	return nil, nil
}
func (r *iamRepo) MarkInvitationAccepted(_ context.Context, _ uuid.UUID) error { return nil }
func (r *iamRepo) UpsertOIDCConfig(_ context.Context, _ iam.OIDCConfigParams) error {
	return nil
}
func (r *iamRepo) StartImpersonation(_ context.Context, p iam.StartImpersonationParams) (*iam.ImpersonationSession, error) {
	return &iam.ImpersonationSession{
		ID:               uuid.New(),
		PlatformUserID:   p.PlatformUserID,
		TenantID:         p.TenantID,
		ImpersonatedUser: p.ImpersonatedUser,
		Reason:           p.Reason,
	}, nil
}
func (r *iamRepo) EndImpersonationSession(_ context.Context, _ uuid.UUID) error { return nil }
func (r *iamRepo) ExpireImpersonationSessions(_ context.Context) (int64, error) { return 0, nil }
func (r *iamRepo) CreateAuditEntry(_ context.Context, _ *iam.AuditEntry) error  { return nil }

// ── Budget mock repository ─────────────────────────────────────────────────

type budgetRepo struct {
	budgets   map[uuid.UUID]*budget.Budget
	lineItems map[uuid.UUID]*budget.BudgetLineItem
}

func newBudgetRepo() *budgetRepo {
	return &budgetRepo{
		budgets:   make(map[uuid.UUID]*budget.Budget),
		lineItems: make(map[uuid.UUID]*budget.BudgetLineItem),
	}
}

func (r *budgetRepo) CreateBudget(_ context.Context, b *budget.Budget) error {
	r.budgets[b.ID] = b
	return nil
}
func (r *budgetRepo) GetBudget(_ context.Context, _, id uuid.UUID) (*budget.Budget, error) {
	if b, ok := r.budgets[id]; ok {
		return b, nil
	}
	return nil, budget.ErrBudgetNotFound
}
func (r *budgetRepo) ListBudgets(_ context.Context, _, _ uuid.UUID, _ string, _, _ int) ([]budget.Budget, error) {
	return nil, nil
}
func (r *budgetRepo) UpdateBudgetStatus(_ context.Context, id uuid.UUID, status string, approvedBy *uuid.UUID) error {
	if b, ok := r.budgets[id]; ok {
		b.Status = status
		b.ApprovedBy = approvedBy
		return nil
	}
	return budget.ErrBudgetNotFound
}
func (r *budgetRepo) CreateLineItem(_ context.Context, li *budget.BudgetLineItem) error {
	r.lineItems[li.ID] = li
	return nil
}
func (r *budgetRepo) GetLineItem(_ context.Context, id uuid.UUID) (*budget.BudgetLineItem, error) {
	if li, ok := r.lineItems[id]; ok {
		return li, nil
	}
	return nil, budget.ErrLineItemNotFound
}
func (r *budgetRepo) ListLineItems(_ context.Context, _ uuid.UUID, _, _ int) ([]budget.BudgetLineItem, error) {
	return nil, nil
}
func (r *budgetRepo) UpdateLineItemActuals(_ context.Context, _ uuid.UUID, _, _ decimal.Decimal) error {
	return nil
}
func (r *budgetRepo) CheckLineAvailability(_ context.Context, _ uuid.UUID) (decimal.Decimal, error) {
	return decimal.NewFromInt(500000), nil
}

// ── Expense mock repository ────────────────────────────────────────────────

type expenseRepo struct {
	expenses map[uuid.UUID]*expense.Expense
}

func newExpenseRepo() *expenseRepo {
	return &expenseRepo{expenses: make(map[uuid.UUID]*expense.Expense)}
}

func (r *expenseRepo) CreateExpense(_ context.Context, e *expense.Expense) error {
	r.expenses[e.ID] = e
	return nil
}
func (r *expenseRepo) GetExpense(_ context.Context, _, id uuid.UUID) (*expense.Expense, error) {
	if e, ok := r.expenses[id]; ok {
		return e, nil
	}
	return nil, expense.ErrExpenseNotFound
}
func (r *expenseRepo) ListExpenses(_ context.Context, _, _ uuid.UUID, _ string, _, _ int) ([]expense.Expense, error) {
	return nil, nil
}
func (r *expenseRepo) UpdateExpense(_ context.Context, e *expense.Expense) error {
	r.expenses[e.ID] = e
	return nil
}
func (r *expenseRepo) CreatePurchaseOrder(_ context.Context, _ *expense.PurchaseOrder) error {
	return nil
}
func (r *expenseRepo) GetPurchaseOrder(_ context.Context, _, _ uuid.UUID) (*expense.PurchaseOrder, error) {
	return nil, nil
}
func (r *expenseRepo) ListPurchaseOrders(_ context.Context, _, _ uuid.UUID, _ string, _, _ int) ([]expense.PurchaseOrder, error) {
	return nil, nil
}
func (r *expenseRepo) UpdatePurchaseOrder(_ context.Context, _ *expense.PurchaseOrder) error {
	return nil
}
func (r *expenseRepo) CreatePettyCashAdvance(_ context.Context, _ *expense.PettyCashAdvance) error {
	return nil
}
func (r *expenseRepo) GetPettyCashAdvance(_ context.Context, _, _ uuid.UUID) (*expense.PettyCashAdvance, error) {
	return nil, nil
}
func (r *expenseRepo) ListPettyCashAdvances(_ context.Context, _, _ uuid.UUID, _ string, _, _ int) ([]expense.PettyCashAdvance, error) {
	return nil, nil
}
func (r *expenseRepo) UpdatePettyCashAdvance(_ context.Context, _ *expense.PettyCashAdvance) error {
	return nil
}

// ── Ledger mock repository ─────────────────────────────────────────────────

type ledgerRepo struct {
	accounts map[uuid.UUID]*ledger.Account
	periods  map[uuid.UUID]*ledger.AccountingPeriod
	entries  map[uuid.UUID]*ledger.JournalEntry
	seq      int
}

func newLedgerRepo() *ledgerRepo {
	return &ledgerRepo{
		accounts: make(map[uuid.UUID]*ledger.Account),
		periods:  make(map[uuid.UUID]*ledger.AccountingPeriod),
		entries:  make(map[uuid.UUID]*ledger.JournalEntry),
	}
}

func (r *ledgerRepo) CreateAccount(_ context.Context, a *ledger.Account) error {
	r.accounts[a.ID] = a
	return nil
}
func (r *ledgerRepo) GetAccountByID(_ context.Context, _, id uuid.UUID) (*ledger.Account, error) {
	if a, ok := r.accounts[id]; ok {
		return a, nil
	}
	return nil, ledger.ErrAccountNotFound
}
func (r *ledgerRepo) GetAccountByCode(_ context.Context, _ uuid.UUID, code string) (*ledger.Account, error) {
	for _, a := range r.accounts {
		if a.Code == code {
			return a, nil
		}
	}
	return nil, ledger.ErrAccountNotFound
}
func (r *ledgerRepo) ListAccounts(_ context.Context, _ uuid.UUID, _, _ int) ([]ledger.Account, error) {
	return nil, nil
}
func (r *ledgerRepo) CreatePeriod(_ context.Context, p *ledger.AccountingPeriod) error {
	r.periods[p.ID] = p
	return nil
}
func (r *ledgerRepo) GetPeriodByID(_ context.Context, _, id uuid.UUID) (*ledger.AccountingPeriod, error) {
	if p, ok := r.periods[id]; ok {
		return p, nil
	}
	return nil, ledger.ErrPeriodNotFound
}
func (r *ledgerRepo) GetOpenPeriod(_ context.Context, _ uuid.UUID, _, _ int) (*ledger.AccountingPeriod, error) {
	for _, p := range r.periods {
		if p.Status == "open" {
			return p, nil
		}
	}
	return nil, ledger.ErrPeriodNotFound
}
func (r *ledgerRepo) ClosePeriod(_ context.Context, _, _, _ uuid.UUID, _ time.Time) error {
	return nil
}
func (r *ledgerRepo) AllocateEntryNumber(_ context.Context, _ uuid.UUID, year int) (string, error) {
	r.seq++
	return fmt.Sprintf("JE-%d-%05d", year, r.seq), nil
}
func (r *ledgerRepo) CreateJournalEntry(_ context.Context, je *ledger.JournalEntry) error {
	r.entries[je.ID] = je
	return nil
}
func (r *ledgerRepo) GetJournalEntry(_ context.Context, _, id uuid.UUID) (*ledger.JournalEntry, error) {
	if je, ok := r.entries[id]; ok {
		return je, nil
	}
	return nil, ledger.ErrJournalNotFound
}
func (r *ledgerRepo) ListJournalEntries(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _ string, _, _ int) ([]ledger.JournalEntry, error) {
	return nil, nil
}
func (r *ledgerRepo) UpdateJournalStatus(_ context.Context, id uuid.UUID, status string, _ uuid.UUID, _ time.Time) error {
	if je, ok := r.entries[id]; ok {
		je.Status = status
		return nil
	}
	return ledger.ErrJournalNotFound
}
func (r *ledgerRepo) GetTrialBalance(_ context.Context, _ uuid.UUID, _ time.Time) ([]ledger.TrialBalanceEntry, error) {
	return nil, nil
}

// ── Reporting mock repository ──────────────────────────────────────────────

type reportingRepo struct {
	expenseFacts []reporting.ExpenseFact
	budgetFacts  []reporting.BudgetFact
}

func (r *reportingRepo) GetExpenseFacts(_ context.Context, _, _ uuid.UUID) ([]reporting.ExpenseFact, error) {
	return r.expenseFacts, nil
}
func (r *reportingRepo) GetBudgetFacts(_ context.Context, _, _ uuid.UUID) ([]reporting.BudgetFact, error) {
	return r.budgetFacts, nil
}
func (r *reportingRepo) GetDashboardSummary(_ context.Context, tid uuid.UUID) (*reporting.DashboardSummary, error) {
	return &reporting.DashboardSummary{
		TenantID:      tid,
		ProjectCount:  3,
		TotalBudgeted: decimal.NewFromInt(2000000),
		TotalActual:   decimal.NewFromInt(1200000),
	}, nil
}
func (r *reportingRepo) GetPortfolioOverview(_ context.Context, tid uuid.UUID) (*reporting.PortfolioOverview, error) {
	return &reporting.PortfolioOverview{TenantID: tid}, nil
}
func (r *reportingRepo) GetFinancialSummary(_ context.Context, tid uuid.UUID) (*reporting.FinancialSummary, error) {
	return &reporting.FinancialSummary{TenantID: tid}, nil
}
func (r *reportingRepo) GetApprovalPipeline(_ context.Context, tid uuid.UUID) (*reporting.ApprovalPipeline, error) {
	return &reporting.ApprovalPipeline{TenantID: tid}, nil
}
func (r *reportingRepo) GetTeamUtilization(_ context.Context, tid uuid.UUID) (*reporting.TeamUtilization, error) {
	return &reporting.TeamUtilization{TenantID: tid}, nil
}
func (r *reportingRepo) GetComplianceStatus(_ context.Context, tid uuid.UUID) (*reporting.ComplianceStatus, error) {
	return &reporting.ComplianceStatus{TenantID: tid}, nil
}
