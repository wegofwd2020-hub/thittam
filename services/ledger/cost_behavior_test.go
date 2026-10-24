package ledger

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetAccountCostBehavior_RejectsUnknownValue(t *testing.T) {
	called := false
	repo := &mockRepo{setCostBehaviorFn: func(context.Context, uuid.UUID, uuid.UUID, string) (*Account, error) {
		called = true
		return nil, nil
	}}
	svc := NewService(repo)

	_, err := svc.SetAccountCostBehavior(context.Background(), fixedTenantID, uuid.New(), "semi")
	require.ErrorIs(t, err, ErrCostBehaviorInvalid)
	assert.False(t, called, "invalid value must not reach the repository")
}

func TestSetAccountCostBehavior_RejectsNonExpenseAccount(t *testing.T) {
	repo := &mockRepo{getAccountByIDFn: func(_ context.Context, tenantID, id uuid.UUID) (*Account, error) {
		return &Account{ID: id, TenantID: tenantID, AccountType: "asset"}, nil
	}}
	svc := NewService(repo)

	_, err := svc.SetAccountCostBehavior(context.Background(), fixedTenantID, uuid.New(), "fixed")
	require.ErrorIs(t, err, ErrCostBehaviorNotExpense)
}

func TestSetAccountCostBehavior_ClearsWithEmptyValue(t *testing.T) {
	var got string
	repo := &mockRepo{setCostBehaviorFn: func(_ context.Context, _, id uuid.UUID, cb string) (*Account, error) {
		got = cb
		return &Account{ID: id, AccountType: "expense", CostBehavior: cb}, nil
	}}
	svc := NewService(repo)

	acct, err := svc.SetAccountCostBehavior(context.Background(), fixedTenantID, uuid.New(), "")
	require.NoError(t, err)
	assert.Equal(t, "", got)
	assert.Equal(t, "", acct.CostBehavior)
}

func TestGetMonthlyCostBehavior_RejectsBadRange(t *testing.T) {
	svc := NewService(&mockRepo{})
	ctx := context.Background()

	cases := []struct {
		name     string
		from, to string
	}{
		{"reversed", "2026-03", "2026-01"},
		{"bad month", "2026-13", "2026-14"},
		{"not a month", "Jan-2026", "2026-02"},
		{"over 36 months", "2020-01", "2026-01"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.GetMonthlyCostBehavior(ctx, fixedTenantID, c.from, c.to)
			require.ErrorIs(t, err, ErrMonthRangeInvalid)
		})
	}
}

func TestGetMonthlyCostBehavior_FillsMonthsWithoutPostings(t *testing.T) {
	repo := &mockRepo{monthlyCostBehaviorFn: func(context.Context, uuid.UUID, string, string) ([]MonthlyCostBehaviorRow, error) {
		return []MonthlyCostBehaviorRow{
			{Month: "2026-01", Fixed: decimal.NewFromInt(100), Variable: decimal.NewFromInt(40), Unclassified: decimal.Zero, Total: decimal.NewFromInt(140)},
			{Month: "2026-03", Fixed: decimal.Zero, Variable: decimal.NewFromInt(25), Unclassified: decimal.NewFromInt(5), Total: decimal.NewFromInt(30)},
		}, nil
	}}
	svc := NewService(repo)

	rows, err := svc.GetMonthlyCostBehavior(context.Background(), fixedTenantID, "2026-01", "2026-03")
	require.NoError(t, err)
	require.Len(t, rows, 3)

	assert.Equal(t, "2026-01", rows[0].Month)
	assert.True(t, rows[0].Fixed.Equal(decimal.NewFromInt(100)))

	assert.Equal(t, "2026-02", rows[1].Month, "gap month is emitted")
	assert.True(t, rows[1].Total.IsZero())
	assert.True(t, rows[1].Fixed.IsZero() && rows[1].Variable.IsZero() && rows[1].Unclassified.IsZero())

	assert.Equal(t, "2026-03", rows[2].Month)
	assert.True(t, rows[2].Total.Equal(decimal.NewFromInt(30)))
}
