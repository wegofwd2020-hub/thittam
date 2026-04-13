package critical_path_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/services/budget"
	"github.com/wegofwd2020/thittam/services/expense"
	"github.com/wegofwd2020/thittam/services/iam"
	"github.com/wegofwd2020/thittam/services/ledger"
	"github.com/wegofwd2020/thittam/services/reporting"
)

// ── Critical Path 1: Tenant Registration ──────────────────────────────────
//
// Verifies that CreateTenant:
//   - Persists the tenant with "active" status
//   - Seeds all system roles (super_admin, manager, coordinator, etc.)
//   - Applies a default plan when none is provided

func TestCriticalPath_TenantRegistration(t *testing.T) {
	t.Parallel()

	repo := newIAMRepo()
	// Stub hasher and authenticator — not exercised in this path.
	svc := iam.NewService(repo, stubAuthenticator{}, stubTokenIssuer{}, stubHasher{}, stubVerifier{})

	tenant := &iam.Tenant{
		ID:   fixedTenantID,
		Name: "XYZ Productions",
		Slug: "xyz-productions",
		// Plan deliberately omitted — service should default to "starter"
	}

	created, err := svc.CreateTenant(context.Background(), tenant)
	require.NoError(t, err)

	// Tenant is immediately active.
	assert.Equal(t, fixedTenantID, created.ID)
	assert.Equal(t, "active", created.Status)
	assert.Equal(t, "starter", created.Plan)

	// System roles must be seeded.
	roles, err := repo.ListRoles(context.Background(), fixedTenantID)
	require.NoError(t, err)
	assert.NotEmpty(t, roles, "system roles must be seeded on tenant creation")

	roleNames := make([]string, len(roles))
	for i, r := range roles {
		roleNames[i] = r.Name
	}
	assert.Contains(t, roleNames, "super_admin")
	assert.Contains(t, roleNames, "coordinator")
}

// ── Critical Path 2: Budget Lifecycle ─────────────────────────────────────
//
// Verifies draft → submitted → approved state machine:
//   - CreateBudget defaults to "draft"
//   - SubmitBudget transitions to "submitted"
//   - ApproveBudget transitions to "approved" and records approver

func TestCriticalPath_BudgetLifecycle(t *testing.T) {
	t.Parallel()

	repo := newBudgetRepo()
	svc := budget.NewService(repo)

	b := &budget.Budget{
		ID:           fixedBudgetID,
		TenantID:     fixedTenantID,
		ProductionID: fixedProductionID,
		Label:        "Main Production Budget",
		Currency:     "INR",
		TotalAmount:  decimal.NewFromInt(2000000),
		CreatedBy:    fixedUserID,
	}

	// Step 1: Create — must default to "draft".
	err := svc.CreateBudget(context.Background(), b)
	require.NoError(t, err)

	created, err := svc.GetBudget(context.Background(), fixedTenantID, fixedBudgetID)
	require.NoError(t, err)
	assert.Equal(t, "draft", created.Status)

	// Step 2: Submit.
	err = svc.SubmitBudget(context.Background(), fixedTenantID, fixedBudgetID, fixedUserID)
	require.NoError(t, err)

	submitted, err := svc.GetBudget(context.Background(), fixedTenantID, fixedBudgetID)
	require.NoError(t, err)
	assert.Equal(t, "submitted", submitted.Status)

	// Step 3: Approve.
	approverID := uuid.MustParse("a0000000-0000-0000-0000-000000000099")
	err = svc.ApproveBudget(context.Background(), fixedTenantID, fixedBudgetID, approverID)
	require.NoError(t, err)

	approved, err := svc.GetBudget(context.Background(), fixedTenantID, fixedBudgetID)
	require.NoError(t, err)
	assert.Equal(t, "approved", approved.Status)
	require.NotNil(t, approved.ApprovedBy)
	assert.Equal(t, approverID, *approved.ApprovedBy)
}

// ── Critical Path 3: Expense Submission and Approval ──────────────────────
//
// Verifies the expense lifecycle with vertical config enforcement:
//   - SubmitExpense validates category against vertical config
//   - ApproveExpense with a role that has sufficient limit succeeds
//   - Approved expense stores the approver and timestamp

func TestCriticalPath_ExpenseLifecycle(t *testing.T) {
	t.Parallel()

	repo := newExpenseRepo()
	svc := expense.NewService(repo)
	ctx := movieVerticalCtx()

	now := time.Now()
	e := &expense.Expense{
		ID:           fixedExpenseID,
		TenantID:     fixedTenantID,
		ProductionID: fixedProductionID,
		CategoryID:   "location_rental",
		Amount:       decimal.NewFromInt(150000),
		Currency:     "INR",
		SubmittedBy:  fixedUserID,
		SubmittedAt:  &now,
		Status:       "submitted",
	}

	// Step 1: Submit expense.
	err := svc.SubmitExpense(ctx, e)
	require.NoError(t, err)

	submitted, err := svc.GetExpense(ctx, fixedTenantID, fixedExpenseID)
	require.NoError(t, err)
	assert.Equal(t, "submitted", submitted.Status)

	// Step 2: Approve by coordinator (limit 500k, amount 150k — within limit).
	approverID := uuid.MustParse("a0000000-0000-0000-0000-000000000098")
	err = svc.ApproveExpense(ctx, fixedTenantID, fixedExpenseID, approverID, "coordinator")
	require.NoError(t, err)

	approved, err := svc.GetExpense(ctx, fixedTenantID, fixedExpenseID)
	require.NoError(t, err)
	assert.Equal(t, "approved", approved.Status)
	require.NotNil(t, approved.ApprovedBy)
	assert.Equal(t, approverID, *approved.ApprovedBy)
	assert.NotNil(t, approved.ApprovedAt)
}

// ── Critical Path 4: Double-Entry Ledger Posting ──────────────────────────
//
// Verifies that a journal entry can be created and posted:
//   - CreateJournalEntry persists the entry with "draft" status
//   - PostJournalEntry transitions to "posted"
//   - Debits and credits balance (total debits == total credits)

func TestCriticalPath_LedgerPosting(t *testing.T) {
	t.Parallel()

	repo := newLedgerRepo()

	// Pre-seed an open accounting period.
	repo.periods[fixedPeriodID] = &ledger.AccountingPeriod{
		ID:       fixedPeriodID,
		TenantID: fixedTenantID,
		Year:     2024,
		Month:    6,
		Status:   "open",
	}

	// Pre-seed the accounts referenced by the journal entry lines.
	repo.accounts[fixedAccountID1] = &ledger.Account{
		ID:          fixedAccountID1,
		TenantID:    fixedTenantID,
		Code:        "5200",
		Name:        "Location Expenses",
		AccountType: "expense",
		IsActive:    true,
	}
	repo.accounts[fixedAccountID2] = &ledger.Account{
		ID:          fixedAccountID2,
		TenantID:    fixedTenantID,
		Code:        "2100",
		Name:        "Accounts Payable",
		AccountType: "liability",
		IsActive:    true,
	}

	svc := ledger.NewService(repo)

	amount := decimal.NewFromInt(150000)
	je := &ledger.JournalEntry{
		ID:        fixedLedgerID,
		TenantID:  fixedTenantID,
		PeriodID:  fixedPeriodID,
		Narration: "Location rental — Day 1",
		Status:    "draft",
		Lines: []ledger.JournalLine{
			{
				ID:          uuid.MustParse("a0000000-0000-0000-0000-000000000010"),
				JournalID:   fixedLedgerID,
				AccountID:   fixedAccountID1,
				DebitAmount: amount,
				Currency:    "INR",
			},
			{
				ID:           uuid.MustParse("a0000000-0000-0000-0000-000000000011"),
				JournalID:    fixedLedgerID,
				AccountID:    fixedAccountID2,
				CreditAmount: amount,
				Currency:     "INR",
			},
		},
	}

	// Step 1: Create — must persist as "draft".
	draft, err := svc.CreateJournalEntry(context.Background(), je)
	require.NoError(t, err)
	assert.Equal(t, "draft", draft.Status)

	// Verify double-entry balance: Σ debits == Σ credits.
	var totalDebit, totalCredit decimal.Decimal
	for _, line := range draft.Lines {
		totalDebit = totalDebit.Add(line.DebitAmount)
		totalCredit = totalCredit.Add(line.CreditAmount)
	}
	assert.True(t, totalDebit.Equal(totalCredit),
		"double-entry must balance: debit %s != credit %s", totalDebit, totalCredit)

	// Step 2: Post the entry.
	posterID := uuid.MustParse("a0000000-0000-0000-0000-000000000097")
	posted, err := svc.PostJournalEntry(context.Background(), fixedTenantID, fixedLedgerID, posterID)
	require.NoError(t, err)
	assert.Equal(t, "posted", posted.Status)
}

// ── Critical Path 5: Report Generation with Vertical Context ──────────────
//
// Verifies that reporting service:
//   - Returns expense facts for a production
//   - Enriches dashboard summary with vertical-specific entity labels
//   - Computes financial figures correctly

func TestCriticalPath_ReportGeneration(t *testing.T) {
	t.Parallel()

	repo := &reportingRepo{
		expenseFacts: []reporting.ExpenseFact{
			{
				TenantID:     fixedTenantID,
				ProductionID: fixedProductionID,
				CategoryID:   "location_rental",
				Amount:       decimal.NewFromInt(150000),
			},
		},
	}
	svc := reporting.NewService(repo)
	ctx := movieVerticalCtx()

	// Step 1: Expense facts for the production.
	facts, err := svc.GetExpenseFacts(ctx, fixedTenantID, fixedProductionID)
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, "location_rental", facts[0].CategoryID)
	assert.True(t, decimal.NewFromInt(150000).Equal(facts[0].Amount))

	// Step 2: Dashboard summary enriched with vertical labels.
	summary, err := svc.GetDashboardSummary(ctx, fixedTenantID)
	require.NoError(t, err)
	// The vertical config has ProjectPlural: "Productions" — must appear on the summary.
	assert.Equal(t, "Productions", summary.ProjectLabel,
		"dashboard must use vertical entity labels, not generic ones")
	assert.Equal(t, 3, summary.ProjectCount)

	// Step 3: Report definition lookup.
	rd, err := svc.GetReportDefinition(ctx, "cost_report")
	require.NoError(t, err)
	assert.Equal(t, "Daily Cost Report", rd.Name)
}
