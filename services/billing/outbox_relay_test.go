package billing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeOutboxPublisher struct {
	published []uuid.UUID
	err       error
}

func (f *fakeOutboxPublisher) Publish(_ context.Context, _ string, tenantID uuid.UUID, _ interface{}) error {
	f.published = append(f.published, tenantID)
	return f.err
}

func TestRelay_PublishesAndMarksSent(t *testing.T) {
	t.Parallel()
	ev := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`)}
	var marked uuid.UUID
	repo := &mockRepo{
		claimUnsentOutboxFn: func(_ context.Context, _ int) ([]*OutboxEvent, error) { return []*OutboxEvent{ev}, nil },
		markOutboxSentFn:    func(_ context.Context, id uuid.UUID) error { marked = id; return nil },
	}
	pub := &fakeOutboxPublisher{}
	r := NewRelay(repo, pub)

	n, err := r.processBatch(context.Background(), 10)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	require.Len(t, pub.published, 1)
	assert.Equal(t, ev.ID, marked)
}

func TestRelay_PublishError_RecordsFailureLeavesUnsent(t *testing.T) {
	t.Parallel()
	ev := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`)}
	var failedID uuid.UUID
	var failedMsg string
	markedSent := false
	repo := &mockRepo{
		claimUnsentOutboxFn:   func(_ context.Context, _ int) ([]*OutboxEvent, error) { return []*OutboxEvent{ev}, nil },
		markOutboxSentFn:      func(_ context.Context, _ uuid.UUID) error { markedSent = true; return nil },
		recordOutboxFailureFn: func(_ context.Context, id uuid.UUID, msg string) error { failedID, failedMsg = id, msg; return nil },
	}
	pub := &fakeOutboxPublisher{err: fmt.Errorf("nats down")}
	r := NewRelay(repo, pub)

	_, err := r.processBatch(context.Background(), 10)
	require.NoError(t, err, "a single publish failure is recorded, not fatal to the batch")
	assert.False(t, markedSent, "must NOT mark sent on publish failure")
	assert.Equal(t, ev.ID, failedID)
	assert.Contains(t, failedMsg, "nats down")
}

// TestRelay_Run_StopsOnContextCancel exercises Run's ticker setup and its
// ctx.Done() exit path. It does not wait out a real relayInterval tick (15s);
// that tick-driven branch is covered indirectly by processBatch's own tests.
func TestRelay_Run_StopsOnContextCancel(t *testing.T) {
	t.Parallel()
	repo := &mockRepo{
		claimUnsentOutboxFn: func(_ context.Context, _ int) ([]*OutboxEvent, error) { return nil, nil },
	}
	r := NewRelay(repo, &fakeOutboxPublisher{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return promptly after context cancellation")
	}
}
