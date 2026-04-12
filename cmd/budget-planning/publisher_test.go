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
	"github.com/wegofwd2020/thittam/services/budget"
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
	fixedBudgetID     = uuid.MustParse("c0000000-0000-0000-0000-000000000001")
)

func TestBudgetPublisher_PublishBudgetApproved_Subject(t *testing.T) {
	t.Parallel()
	js := &mockJS{}
	p := &budgetPublisher{pub: jetstream.NewPublisher(js)}

	b := &budget.Budget{
		ID:           fixedBudgetID,
		ProductionID: fixedProductionID,
		TenantID:     fixedTenantID,
		TotalAmount:  decimal.NewFromFloat(150000.00),
	}

	require.NoError(t, p.PublishBudgetApproved(context.Background(), b))
	assert.Equal(t, events.SubjectBudgetApproved, js.subject)
}

func TestBudgetPublisher_PublishBudgetApproved_Payload(t *testing.T) {
	t.Parallel()
	js := &mockJS{}
	p := &budgetPublisher{pub: jetstream.NewPublisher(js)}

	b := &budget.Budget{
		ID:           fixedBudgetID,
		ProductionID: fixedProductionID,
		TenantID:     fixedTenantID,
		TotalAmount:  decimal.NewFromFloat(99500.50),
	}

	require.NoError(t, p.PublishBudgetApproved(context.Background(), b))

	var env events.Envelope
	require.NoError(t, json.Unmarshal(js.payload, &env))
	assert.Equal(t, fixedTenantID, env.TenantID)

	var pl events.BudgetApprovedPayload
	require.NoError(t, json.Unmarshal(env.Payload, &pl))
	assert.Equal(t, fixedBudgetID, pl.BudgetID)
	assert.Equal(t, fixedProductionID, pl.ProductionID)
	assert.Equal(t, "99500.50", pl.TotalBudgeted)
}

func TestBudgetPublisher_PublishBudgetApproved_PropagatesPublishError(t *testing.T) {
	t.Parallel()
	js := &mockJS{err: assert.AnError}
	p := &budgetPublisher{pub: jetstream.NewPublisher(js)}

	err := p.PublishBudgetApproved(context.Background(), &budget.Budget{
		TotalAmount: decimal.NewFromInt(1000),
	})
	assert.Error(t, err)
}

// TestNatsChecker_Unhealthy verifies the checker reports an error for a nil/closed connection.
// A connected-path test requires a real NATS server and belongs in integration tests.
func TestNatsChecker_Unhealthy(t *testing.T) {
	t.Parallel()
	// A zero-value *nats.Conn has IsConnected() == false.
	checker := &natsChecker{nc: &nats.Conn{}}
	assert.Error(t, checker.CheckHealth(context.Background()))
}

// TestDbChecker_HealthCheck_Propagates exercises the nil-pool panic boundary.
// A connected-path test requires a real PostgreSQL pool (integration test).
func TestRequireenv_ReturnsValue(t *testing.T) {
	// t.Setenv is incompatible with t.Parallel — must run sequentially.
	t.Setenv("TEST_BUDGET_VAR", "hello")
	assert.Equal(t, "hello", requireenv("TEST_BUDGET_VAR"))
}

// Verify the publisher adapter builds correctly with fixed timestamps.
func TestBudgetPublisher_EnvelopeHasOccurredAt(t *testing.T) {
	t.Parallel()
	js := &mockJS{}
	p := &budgetPublisher{pub: jetstream.NewPublisher(js)}

	before := time.Now().UTC()
	require.NoError(t, p.PublishBudgetApproved(context.Background(), &budget.Budget{
		TotalAmount: decimal.NewFromInt(500),
	}))
	after := time.Now().UTC()

	var env events.Envelope
	require.NoError(t, json.Unmarshal(js.payload, &env))
	assert.True(t, !env.OccurredAt.Before(before) && !env.OccurredAt.After(after),
		"OccurredAt should be set to approximately now")
}
