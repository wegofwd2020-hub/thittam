package billing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeOutboxPublisher struct {
	published []uuid.UUID
	err       error               // fails every publish
	failFor   map[uuid.UUID]error // fails only these tenants
}

func (f *fakeOutboxPublisher) Publish(_ context.Context, _ string, tenantID uuid.UUID, _ interface{}) error {
	if e, ok := f.failFor[tenantID]; ok {
		return e
	}
	if f.err != nil {
		return f.err
	}
	f.published = append(f.published, tenantID)
	return nil
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

// A poison row — fails while a batch-mate succeeds — is dead-lettered once it
// exhausts maxOutboxAttempts.
func TestRelay_PoisonRow_DeadLetteredWhenSiblingSucceeds(t *testing.T) {
	t.Parallel()
	poison := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`), Attempts: maxOutboxAttempts}
	healthy := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`), Attempts: 1}

	var movedID uuid.UUID
	var movedMsg string
	recordedFailure := false
	repo := &mockRepo{
		claimUnsentOutboxFn:   func(_ context.Context, _ int) ([]*OutboxEvent, error) { return []*OutboxEvent{poison, healthy}, nil },
		markOutboxSentFn:      func(_ context.Context, _ uuid.UUID) error { return nil },
		recordOutboxFailureFn: func(_ context.Context, _ uuid.UUID, _ string) error { recordedFailure = true; return nil },
		moveOutboxToDeadFn:    func(_ context.Context, id uuid.UUID, msg string) error { movedID, movedMsg = id, msg; return nil },
	}
	pub := &fakeOutboxPublisher{failFor: map[uuid.UUID]error{poison.TenantID: fmt.Errorf("malformed payload")}}
	r := NewRelay(repo, pub)

	n, err := r.processBatch(context.Background(), 10)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "the healthy sibling still publishes")
	assert.Equal(t, poison.ID, movedID, "poison row must be dead-lettered")
	assert.Contains(t, movedMsg, "malformed payload")
	assert.False(t, recordedFailure, "a dead-lettered row is moved, not merely recorded")
}

// The systemic guard: when the WHOLE batch fails, it is an outage, not poison.
// Nothing is dead-lettered no matter how high attempts has climbed.
func TestRelay_TotalBatchFailure_NeverDeadLetters(t *testing.T) {
	t.Parallel()
	stale := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`), Attempts: 99}
	other := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`), Attempts: 42}

	moved := false
	var recorded []uuid.UUID
	repo := &mockRepo{
		claimUnsentOutboxFn:   func(_ context.Context, _ int) ([]*OutboxEvent, error) { return []*OutboxEvent{stale, other}, nil },
		recordOutboxFailureFn: func(_ context.Context, id uuid.UUID, _ string) error { recorded = append(recorded, id); return nil },
		moveOutboxToDeadFn:    func(_ context.Context, _ uuid.UUID, _ string) error { moved = true; return nil },
	}
	pub := &fakeOutboxPublisher{err: fmt.Errorf("nats down")}
	r := NewRelay(repo, pub)
	// Outage semantics: nothing published this batch AND no publish succeeded within
	// the broker-health window — the relay must retry, never park (#137).
	fixed := time.Now()
	r.now = func() time.Time { return fixed }
	r.lastPublishSuccess = fixed.Add(-10 * time.Minute)

	n, err := r.processBatch(context.Background(), 10)
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.False(t, moved, "a NATS outage must never park the backlog")
	assert.ElementsMatch(t, []uuid.UUID{stale.ID, other.ID}, recorded)
}

// Saturation: a batch of ONLY maxed-attempts poison rows (published==0), but the
// broker published successfully within the window — the rows are poison, not an
// outage, so dead-letter them to unblock healthy events queued behind them (#137).
func TestRelay_Saturation_DeadLettersWhenBrokerRecentlyHealthy(t *testing.T) {
	t.Parallel()
	p1 := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`), Attempts: maxOutboxAttempts}
	p2 := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`), Attempts: maxOutboxAttempts}
	moved := map[uuid.UUID]bool{}
	recorded := false
	repo := &mockRepo{
		claimUnsentOutboxFn:   func(_ context.Context, _ int) ([]*OutboxEvent, error) { return []*OutboxEvent{p1, p2}, nil },
		moveOutboxToDeadFn:    func(_ context.Context, id uuid.UUID, _ string) error { moved[id] = true; return nil },
		recordOutboxFailureFn: func(_ context.Context, _ uuid.UUID, _ string) error { recorded = true; return nil },
	}
	pub := &fakeOutboxPublisher{failFor: map[uuid.UUID]error{p1.TenantID: fmt.Errorf("poison"), p2.TenantID: fmt.Errorf("poison")}}
	r := NewRelay(repo, pub)
	fixed := time.Now()
	r.now = func() time.Time { return fixed }
	r.lastPublishSuccess = fixed.Add(-1 * time.Minute) // broker healthy recently

	n, err := r.processBatch(context.Background(), 10)
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.True(t, moved[p1.ID] && moved[p2.ID], "recently-healthy broker => poison rows dead-lettered")
	assert.False(t, recorded, "dead-lettered, not merely recorded")
}

// A successful publish advances lastPublishSuccess to now — the signal the guard reads.
func TestRelay_PublishSuccess_UpdatesLastPublishSuccess(t *testing.T) {
	t.Parallel()
	healthy := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`), Attempts: 1}
	repo := &mockRepo{
		claimUnsentOutboxFn: func(_ context.Context, _ int) ([]*OutboxEvent, error) { return []*OutboxEvent{healthy}, nil },
		markOutboxSentFn:    func(_ context.Context, _ uuid.UUID) error { return nil },
	}
	r := NewRelay(repo, &fakeOutboxPublisher{})
	fixed := time.Now()
	r.now = func() time.Time { return fixed }
	r.lastPublishSuccess = fixed.Add(-1 * time.Hour)

	_, err := r.processBatch(context.Background(), 10)
	require.NoError(t, err)
	assert.Equal(t, fixed, r.lastPublishSuccess, "a successful publish refreshes the broker-health signal")
}

// Below the cap, a failure is recorded and retried, even with a healthy sibling.
func TestRelay_UnderCap_RecordsFailureNotDead(t *testing.T) {
	t.Parallel()
	young := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`), Attempts: maxOutboxAttempts - 1}
	healthy := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`), Attempts: 1}

	moved := false
	var failedID uuid.UUID
	repo := &mockRepo{
		claimUnsentOutboxFn:   func(_ context.Context, _ int) ([]*OutboxEvent, error) { return []*OutboxEvent{young, healthy}, nil },
		markOutboxSentFn:      func(_ context.Context, _ uuid.UUID) error { return nil },
		recordOutboxFailureFn: func(_ context.Context, id uuid.UUID, _ string) error { failedID = id; return nil },
		moveOutboxToDeadFn:    func(_ context.Context, _ uuid.UUID, _ string) error { moved = true; return nil },
	}
	pub := &fakeOutboxPublisher{failFor: map[uuid.UUID]error{young.TenantID: fmt.Errorf("transient")}}
	r := NewRelay(repo, pub)

	_, err := r.processBatch(context.Background(), 10)
	require.NoError(t, err)
	assert.False(t, moved)
	assert.Equal(t, young.ID, failedID)
}

// A MoveOutboxToDead error is logged, not fatal: the tick completes.
func TestRelay_MoveToDeadError_TickContinues(t *testing.T) {
	t.Parallel()
	poison := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`), Attempts: maxOutboxAttempts}
	healthy := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`), Attempts: 1}
	repo := &mockRepo{
		claimUnsentOutboxFn: func(_ context.Context, _ int) ([]*OutboxEvent, error) { return []*OutboxEvent{poison, healthy}, nil },
		markOutboxSentFn:    func(_ context.Context, _ uuid.UUID) error { return nil },
		moveOutboxToDeadFn:  func(_ context.Context, _ uuid.UUID, _ string) error { return fmt.Errorf("db down") },
	}
	pub := &fakeOutboxPublisher{failFor: map[uuid.UUID]error{poison.TenantID: fmt.Errorf("malformed")}}
	r := NewRelay(repo, pub)

	n, err := r.processBatch(context.Background(), 10)
	require.NoError(t, err, "a failed move must not abort the batch")
	assert.Equal(t, 1, n)
}

// updateGauges sets the three outbox health gauges from repo stats. Not run
// in parallel: it asserts on package-level (global registry) gauge values.
func TestRelay_UpdateGauges_SetsFromStats(t *testing.T) {
	repo := &mockRepo{
		outboxStatsFn: func(_ context.Context) (*OutboxStats, error) {
			return &OutboxStats{Pending: 3, OldestPendingSeconds: 12.5, Dead: 2}, nil
		},
	}
	r := NewRelay(repo, &fakeOutboxPublisher{})

	r.updateGauges(context.Background())

	assert.Equal(t, float64(3), testutil.ToFloat64(outboxPending))
	assert.Equal(t, 12.5, testutil.ToFloat64(outboxOldestPending))
	assert.Equal(t, float64(2), testutil.ToFloat64(outboxDead))

	lastSuccess := testutil.ToFloat64(outboxStatsLastSuccess)
	assert.NotZero(t, lastSuccess, "last-success timestamp must be set on a successful read")
	assert.InDelta(t, float64(time.Now().Unix()), lastSuccess, 5,
		"last-success timestamp should be approximately now")
}

// A stats error is swallowed: observability must never take down delivery.
// The last-success timestamp gauge must NOT advance — that frozen value is
// exactly what BillingOutboxStatsStale alerts on.
func TestRelay_UpdateGauges_StatsErrorSwallowed(t *testing.T) {
	repo := &mockRepo{
		outboxStatsFn: func(_ context.Context) (*OutboxStats, error) { return nil, fmt.Errorf("db down") },
	}
	r := NewRelay(repo, &fakeOutboxPublisher{})

	before := testutil.ToFloat64(outboxStatsLastSuccess)

	require.NotPanics(t, func() { r.updateGauges(context.Background()) })

	assert.Equal(t, before, testutil.ToFloat64(outboxStatsLastSuccess),
		"a failed stats read must leave the last-success timestamp unchanged")
}
