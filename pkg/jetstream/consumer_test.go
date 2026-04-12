package jetstream

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	nats "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wegofwd2020/thittam/pkg/events"
)

// stubAckableMsg records whether Ack or Nak was called.
type stubAckableMsg struct {
	acked bool
	naked bool
	data  []byte
}

func (m *stubAckableMsg) Ack(_ ...nats.AckOpt) error { m.acked = true; return nil }
func (m *stubAckableMsg) Nak(_ ...nats.AckOpt) error { m.naked = true; return nil }

// validEnvelope builds a well-formed envelope payload.
func validEnvelope(t *testing.T) []byte {
	t.Helper()
	env := events.Envelope{
		EventID:    uuid.MustParse("e0000000-0000-0000-0000-000000000001"),
		Type:       events.SubjectExpenseApproved,
		TenantID:   uuid.MustParse("a0000000-0000-0000-0000-000000000001"),
		OccurredAt: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
		Payload:    json.RawMessage(`{"key":"value"}`),
	}
	data, err := json.Marshal(env)
	require.NoError(t, err)
	return data
}

// ── dispatchMsg tests ─────────────────────────────────────────────────────────

func TestDispatchMsg_ValidEnvelope_HandlerSuccess_Acks(t *testing.T) {
	t.Parallel()
	msg := &stubAckableMsg{data: validEnvelope(t)}
	var received *events.Envelope

	dispatchMsg(msg, msg.data, events.SubjectExpenseApproved, "test-consumer",
		func(_ context.Context, env *events.Envelope) error {
			received = env
			return nil
		},
	)

	assert.True(t, msg.acked, "successful dispatch must Ack the message")
	assert.False(t, msg.naked)
	require.NotNil(t, received)
	assert.Equal(t, events.SubjectExpenseApproved, received.Type)
}

func TestDispatchMsg_BadJSON_AcksWithoutCallingHandler(t *testing.T) {
	t.Parallel()
	msg := &stubAckableMsg{}
	handlerCalled := false

	dispatchMsg(msg, []byte("not-json"), "thittam.expense.approved", "test-consumer",
		func(_ context.Context, _ *events.Envelope) error {
			handlerCalled = true
			return nil
		},
	)

	assert.True(t, msg.acked, "bad envelope must be acked to prevent infinite redelivery")
	assert.False(t, msg.naked)
	assert.False(t, handlerCalled, "handler must not be called when envelope is unparseable")
}

func TestDispatchMsg_HandlerError_Naks(t *testing.T) {
	t.Parallel()
	msg := &stubAckableMsg{data: validEnvelope(t)}

	dispatchMsg(msg, msg.data, events.SubjectExpenseApproved, "test-consumer",
		func(_ context.Context, _ *events.Envelope) error {
			return errors.New("db connection refused")
		},
	)

	assert.False(t, msg.acked)
	assert.True(t, msg.naked, "handler error must Nak so NATS redelivers after recovery")
}

func TestDispatchMsg_AckError_IsLoggedNotReturned(t *testing.T) {
	t.Parallel()
	// Even when msg.Ack() fails (e.g. connection lost after handler succeeded),
	// dispatchMsg must not panic — the error is logged and swallowed.
	ackErr := &errAckableMsg{ackErr: errors.New("ack failed: connection lost")}

	dispatchMsg(ackErr, validEnvelope(t), events.SubjectExpenseApproved, "test-consumer",
		func(_ context.Context, _ *events.Envelope) error { return nil },
	)
	// No panic — test passes if we reach here.
	assert.True(t, ackErr.ackCalled)
}

// errAckableMsg returns an error from Ack to exercise the ack-failure log path.
type errAckableMsg struct {
	ackCalled bool
	ackErr    error
}

func (m *errAckableMsg) Ack(_ ...nats.AckOpt) error { m.ackCalled = true; return m.ackErr }
func (m *errAckableMsg) Nak(_ ...nats.AckOpt) error { return nil }

func TestDispatchMsg_PassesEnvelopeFieldsToHandler(t *testing.T) {
	t.Parallel()
	fixedTenantID := uuid.MustParse("b0000000-0000-0000-0000-000000000001")
	fixedEventID := uuid.MustParse("e1000000-0000-0000-0000-000000000002")
	occurredAt := time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC)

	env := events.Envelope{
		EventID:    fixedEventID,
		Type:       events.SubjectBudgetApproved,
		TenantID:   fixedTenantID,
		OccurredAt: occurredAt,
		Payload:    json.RawMessage(`{}`),
	}
	data, err := json.Marshal(env)
	require.NoError(t, err)

	msg := &stubAckableMsg{}
	var got *events.Envelope
	dispatchMsg(msg, data, events.SubjectBudgetApproved, "test-consumer",
		func(_ context.Context, e *events.Envelope) error {
			got = e
			return nil
		},
	)

	require.NotNil(t, got)
	assert.Equal(t, fixedEventID, got.EventID)
	assert.Equal(t, fixedTenantID, got.TenantID)
	assert.Equal(t, events.SubjectBudgetApproved, got.Type)
	assert.Equal(t, occurredAt.UTC(), got.OccurredAt.UTC())
}
