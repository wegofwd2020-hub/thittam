package jetstream

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/events"
)

// mockJS captures publishes for assertion.
type mockJS struct {
	calls []publishCall
	err   error
}

type publishCall struct {
	subject string
	data    []byte
}

func (m *mockJS) Publish(subj string, data []byte, _ ...nats.PubOpt) (*nats.PubAck, error) {
	if m.err != nil {
		return nil, m.err
	}
	m.calls = append(m.calls, publishCall{subject: subj, data: data})
	return &nats.PubAck{}, nil
}

func TestPublisher_Publish_Success(t *testing.T) {
	t.Parallel()
	js := &mockJS{}
	p := NewPublisher(js)
	tenantID := uuid.MustParse("a1000000-0000-0000-0000-000000000001")

	type TestPayload struct {
		Value string `json:"value"`
	}

	err := p.Publish(context.Background(), events.SubjectExpenseSubmitted, tenantID, TestPayload{Value: "hello"})
	require.NoError(t, err)

	require.Len(t, js.calls, 1)
	assert.Equal(t, events.SubjectExpenseSubmitted, js.calls[0].subject)

	// Verify envelope structure
	var env events.Envelope
	require.NoError(t, json.Unmarshal(js.calls[0].data, &env))
	assert.Equal(t, events.SubjectExpenseSubmitted, env.Type)
	assert.Equal(t, tenantID, env.TenantID)
	assert.NotEqual(t, uuid.Nil, env.EventID)
	assert.False(t, env.OccurredAt.IsZero())

	// Verify payload round-trips
	var payload TestPayload
	require.NoError(t, json.Unmarshal(env.Payload, &payload))
	assert.Equal(t, "hello", payload.Value)
}

func TestPublisher_Publish_JetStreamError(t *testing.T) {
	t.Parallel()
	js := &mockJS{err: nats.ErrConnectionClosed}
	p := NewPublisher(js)

	err := p.Publish(context.Background(), events.SubjectExpenseSubmitted, uuid.New(), struct{}{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "publish")
}

func TestPublisher_Publish_AllFinancialSubjects(t *testing.T) {
	t.Parallel()
	// Verify Publisher can publish to every financial subject — ensures
	// subjects are non-empty strings and envelopes serialize cleanly.
	subjects := []string{
		events.SubjectExpenseSubmitted,
		events.SubjectExpenseApproved,
		events.SubjectBudgetApproved,
		events.SubjectLedgerJournalPosted,
	}
	for _, subj := range subjects {
		subj := subj
		t.Run(subj, func(t *testing.T) {
			t.Parallel()
			js := &mockJS{}
			p := NewPublisher(js)
			err := p.Publish(context.Background(), subj, uuid.New(), struct{}{})
			require.NoError(t, err)
			assert.Len(t, js.calls, 1)
		})
	}
}

func TestPublisher_Publish_UnmarshalablePayload_ReturnsError(t *testing.T) {
	t.Parallel()
	js := &mockJS{}
	p := NewPublisher(js)

	// A channel cannot be marshaled to JSON — events.NewEnvelope must return an error.
	unmarshalable := make(chan int)
	err := p.Publish(context.Background(), events.SubjectExpenseSubmitted, uuid.New(), unmarshalable)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "build envelope")
	assert.Empty(t, js.calls, "js.Publish must not be called when envelope construction fails")
}

func TestPublisher_Publish_EventIDIsUnique(t *testing.T) {
	t.Parallel()
	js := &mockJS{}
	p := NewPublisher(js)

	for i := 0; i < 3; i++ {
		require.NoError(t, p.Publish(context.Background(), events.SubjectProjectCreated, uuid.New(), struct{}{}))
	}
	require.Len(t, js.calls, 3)

	seen := make(map[uuid.UUID]bool)
	for _, call := range js.calls {
		var env events.Envelope
		require.NoError(t, json.Unmarshal(call.data, &env))
		assert.False(t, seen[env.EventID], "duplicate event ID %s", env.EventID)
		seen[env.EventID] = true
	}
}
