package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	nats "github.com/nats-io/nats.go"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/events"
	"github.com/wegofwd2020/thittam/pkg/jetstream"
	"github.com/wegofwd2020/thittam/services/expense"
)

// mockJS captures publish calls for assertion.
type mockJS struct {
	subject string
	payload []byte
	err     error
}

func (m *mockJS) Publish(subj string, data []byte, _ ...nats.PubOpt) (*nats.PubAck, error) {
	m.subject = subj
	m.payload = data
	return &nats.PubAck{}, m.err
}

var (
	fixedTenantID     = uuid.MustParse("a0000000-0000-0000-0000-000000000001")
	fixedProductionID = uuid.MustParse("b0000000-0000-0000-0000-000000000001")
	fixedExpenseID    = uuid.MustParse("c0000000-0000-0000-0000-000000000001")
	fixedUserID       = uuid.MustParse("d0000000-0000-0000-0000-000000000001")
	fixedTime         = time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC)
)

func newExpense() *expense.Expense {
	return &expense.Expense{
		ID:           fixedExpenseID,
		ProductionID: fixedProductionID,
		TenantID:     fixedTenantID,
		CategoryID:   "cat-lighting",
		Description:  "LED panel hire",
		Amount:       decimal.NewFromFloat(3500.00),
		SubmittedBy:  fixedUserID,
		CreatedAt:    fixedTime,
	}
}

func TestExpensePublisher_PublishExpenseSubmitted_Subject(t *testing.T) {
	t.Parallel()
	js := &mockJS{}
	p := &expensePublisher{pub: jetstream.NewPublisher(js)}

	require.NoError(t, p.PublishExpenseSubmitted(context.Background(), newExpense()))
	assert.Equal(t, events.SubjectExpenseSubmitted, js.subject)
}

func TestExpensePublisher_PublishExpenseSubmitted_Payload(t *testing.T) {
	t.Parallel()
	js := &mockJS{}
	p := &expensePublisher{pub: jetstream.NewPublisher(js)}

	e := newExpense()
	require.NoError(t, p.PublishExpenseSubmitted(context.Background(), e))

	var env events.Envelope
	require.NoError(t, json.Unmarshal(js.payload, &env))
	assert.Equal(t, fixedTenantID, env.TenantID)

	var pl events.ExpenseSubmittedPayload
	require.NoError(t, json.Unmarshal(env.Payload, &pl))
	assert.Equal(t, fixedExpenseID, pl.ExpenseID)
	assert.Equal(t, fixedProductionID, pl.ProductionID)
	assert.Equal(t, "cat-lighting", pl.CategoryID)
	assert.Equal(t, "3500.00", pl.Amount)
}

func TestExpensePublisher_PublishExpenseApproved_Subject(t *testing.T) {
	t.Parallel()
	js := &mockJS{}
	p := &expensePublisher{pub: jetstream.NewPublisher(js)}

	approvedAt := fixedTime.Add(2 * time.Hour)
	e := newExpense()
	e.ApprovedAt = &approvedAt

	require.NoError(t, p.PublishExpenseApproved(context.Background(), e))
	assert.Equal(t, events.SubjectExpenseApproved, js.subject)
}

func TestExpensePublisher_PublishExpenseApproved_MonthDerived(t *testing.T) {
	t.Parallel()
	js := &mockJS{}
	p := &expensePublisher{pub: jetstream.NewPublisher(js)}

	approvedAt := time.Date(2026, 4, 12, 15, 30, 0, 0, time.UTC)
	e := newExpense()
	e.ApprovedAt = &approvedAt

	require.NoError(t, p.PublishExpenseApproved(context.Background(), e))

	var env events.Envelope
	require.NoError(t, json.Unmarshal(js.payload, &env))
	var pl events.ExpenseApprovedPayload
	require.NoError(t, json.Unmarshal(env.Payload, &pl))
	// Month must be derived from ApprovedAt, not from CreatedAt.
	assert.Equal(t, "2026-04", pl.Month)
}

func TestExpensePublisher_PublishExpenseApproved_FallsBackToCreatedAt(t *testing.T) {
	t.Parallel()
	// When ApprovedAt is nil, the adapter falls back to CreatedAt.
	js := &mockJS{}
	p := &expensePublisher{pub: jetstream.NewPublisher(js)}

	e := newExpense() // ApprovedAt is nil
	require.NoError(t, p.PublishExpenseApproved(context.Background(), e))

	var env events.Envelope
	require.NoError(t, json.Unmarshal(js.payload, &env))
	var pl events.ExpenseApprovedPayload
	require.NoError(t, json.Unmarshal(env.Payload, &pl))
	assert.Equal(t, "2026-04", pl.Month) // falls back to CreatedAt month
}

func TestNatsChecker_Unhealthy(t *testing.T) {
	t.Parallel()
	checker := &natsChecker{nc: &nats.Conn{}}
	assert.Error(t, checker.CheckHealth(context.Background()))
}

func TestRequireenv_ReturnsValue(t *testing.T) {
	// t.Setenv is incompatible with t.Parallel — must run sequentially.
	t.Setenv("TEST_EXPENSE_VAR", "ok")
	assert.Equal(t, "ok", requireenv("TEST_EXPENSE_VAR"))
}
