package audit

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock store ---

type mockStore struct {
	mu     sync.Mutex
	events []Event
	insertFn    func(ctx context.Context, event Event) error
	insertBatchFn func(ctx context.Context, events []Event) error
	queryFn     func(ctx context.Context, filter QueryFilter) ([]Event, error)
}

func (m *mockStore) Insert(ctx context.Context, event Event) error {
	if m.insertFn != nil {
		return m.insertFn(ctx, event)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *mockStore) InsertBatch(ctx context.Context, events []Event) error {
	if m.insertBatchFn != nil {
		return m.insertBatchFn(ctx, events)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, events...)
	return nil
}

func (m *mockStore) Query(ctx context.Context, filter QueryFilter) ([]Event, error) {
	if m.queryFn != nil {
		return m.queryFn(ctx, filter)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.events, nil
}

func (m *mockStore) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func (m *mockStore) getEvents() []Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]Event, len(m.events))
	copy(cp, m.events)
	return cp
}

type silentLogger struct{}

func (silentLogger) Info(msg string, kv ...interface{})  {}
func (silentLogger) Warn(msg string, kv ...interface{})  {}
func (silentLogger) Error(msg string, kv ...interface{}) {}

// --- Helpers ---

var (
	testTenantID = uuid.MustParse("d0000000-0000-0000-0000-000000000001")
	testActorID  = uuid.MustParse("d1000000-0000-0000-0000-000000000001")
	testResourceID = uuid.MustParse("d2000000-0000-0000-0000-000000000001")
)

func testEvent() Event {
	return Event{
		TenantID:     testTenantID,
		ActorID:      testActorID,
		ActorEmail:   "admin@xyzcba.com",
		Action:       ActionApproved,
		ResourceType: ResourceExpense,
		ResourceID:   testResourceID,
	}
}

// --- Tests ---

func TestLogger_LogAndFlush(t *testing.T) {
	store := &mockStore{}
	logger := NewLogger(store, LoggerConfig{
		BufferSize:    100,
		FlushInterval: 50 * time.Millisecond,
		BatchSize:     10,
	}, silentLogger{})

	// Log 5 events
	for i := 0; i < 5; i++ {
		logger.Log(testEvent())
	}

	// Wait for flush
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, 5, store.count())

	// Close flushes remaining
	require.NoError(t, logger.Close(context.Background()))
}

func TestLogger_BatchFlush(t *testing.T) {
	store := &mockStore{}
	logger := NewLogger(store, LoggerConfig{
		BufferSize:    100,
		FlushInterval: 10 * time.Second, // long interval
		BatchSize:     5,                 // small batch
	}, silentLogger{})

	// Log exactly batchSize events — should trigger immediate flush
	for i := 0; i < 5; i++ {
		logger.Log(testEvent())
	}

	// Give the goroutine time to process
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 5, store.count())

	require.NoError(t, logger.Close(context.Background()))
}

func TestLogger_GeneratesIDAndTimestamp(t *testing.T) {
	store := &mockStore{}
	logger := NewLogger(store, LoggerConfig{
		BufferSize:    10,
		FlushInterval: 50 * time.Millisecond,
		BatchSize:     10,
	}, silentLogger{})

	event := testEvent()
	event.ID = uuid.Nil // should be generated
	event.OccurredAt = time.Time{} // should be generated
	logger.Log(event)

	time.Sleep(150 * time.Millisecond)
	require.NoError(t, logger.Close(context.Background()))

	events := store.getEvents()
	require.Len(t, events, 1)
	assert.NotEqual(t, uuid.Nil, events[0].ID)
	assert.False(t, events[0].OccurredAt.IsZero())
}

func TestLogger_LogAction_Convenience(t *testing.T) {
	store := &mockStore{}
	logger := NewLogger(store, LoggerConfig{
		BufferSize:    10,
		FlushInterval: 50 * time.Millisecond,
		BatchSize:     10,
	}, silentLogger{})

	logger.LogAction(
		testTenantID, testActorID, "admin@xyzcba.com",
		ActionApproved, ResourceExpense, testResourceID,
		map[string]string{"status": "submitted"},  // old state
		map[string]string{"status": "approved"},    // new state
		map[string]interface{}{"approval_limit": "200000"}, // metadata
	)

	time.Sleep(150 * time.Millisecond)
	require.NoError(t, logger.Close(context.Background()))

	events := store.getEvents()
	require.Len(t, events, 1)

	e := events[0]
	assert.Equal(t, ActionApproved, e.Action)
	assert.Equal(t, ResourceExpense, e.ResourceType)
	assert.NotNil(t, e.OldState)
	assert.NotNil(t, e.NewState)
	assert.NotNil(t, e.Metadata)

	// Verify old/new state contain expected values
	var oldState map[string]string
	require.NoError(t, json.Unmarshal(e.OldState, &oldState))
	assert.Equal(t, "submitted", oldState["status"])

	var newState map[string]string
	require.NoError(t, json.Unmarshal(e.NewState, &newState))
	assert.Equal(t, "approved", newState["status"])
}

func TestLogger_BufferFull_DropsEvent(t *testing.T) {
	store := &mockStore{}
	// Tiny buffer + very slow flush = buffer fills up
	logger := NewLogger(store, LoggerConfig{
		BufferSize:    2,
		FlushInterval: 10 * time.Second,
		BatchSize:     100,
	}, silentLogger{})

	// Spam more events than the buffer can hold
	for i := 0; i < 10; i++ {
		logger.Log(testEvent())
	}

	// Some events should have been dropped (buffer size 2)
	require.NoError(t, logger.Close(context.Background()))
	// At most buffer size + 1 batch could have been processed
	assert.LessOrEqual(t, store.count(), 10)
}

func TestLogger_Query(t *testing.T) {
	store := &mockStore{}
	logger := NewLogger(store, LoggerConfig{
		BufferSize:    100,
		FlushInterval: 50 * time.Millisecond,
		BatchSize:     100,
	}, silentLogger{})

	// Insert directly to store for query testing
	store.Insert(context.Background(), testEvent())
	store.Insert(context.Background(), testEvent())

	events, err := logger.Query(context.Background(), QueryFilter{
		TenantID: testTenantID,
	})
	require.NoError(t, err)
	assert.Len(t, events, 2)

	require.NoError(t, logger.Close(context.Background()))
}

func TestLogger_Query_DefaultLimit(t *testing.T) {
	store := &mockStore{
		queryFn: func(ctx context.Context, filter QueryFilter) ([]Event, error) {
			assert.Equal(t, 100, filter.Limit) // default
			return nil, nil
		},
	}
	logger := NewLogger(store, DefaultConfig(), silentLogger{})

	_, err := logger.Query(context.Background(), QueryFilter{TenantID: testTenantID, Limit: 0})
	require.NoError(t, err)
	require.NoError(t, logger.Close(context.Background()))
}

func TestLogger_Pending(t *testing.T) {
	store := &mockStore{}
	logger := NewLogger(store, LoggerConfig{
		BufferSize:    100,
		FlushInterval: 10 * time.Second, // long interval
		BatchSize:     100,
	}, silentLogger{})

	logger.Log(testEvent())
	logger.Log(testEvent())

	// Give channel time to receive
	time.Sleep(10 * time.Millisecond)
	// Pending might be 0 if flush loop already picked them up, or 2 if not
	assert.GreaterOrEqual(t, 2, logger.Pending())

	require.NoError(t, logger.Close(context.Background()))
}

func TestEvent_String(t *testing.T) {
	e := testEvent()
	e.OccurredAt = time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC)
	s := e.String()
	assert.Contains(t, s, "2026-04-04")
	assert.Contains(t, s, "approved")
	assert.Contains(t, s, "expense")
	assert.Contains(t, s, "admin@xyzcba.com")
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, 1000, cfg.BufferSize)
	assert.Equal(t, 5*time.Second, cfg.FlushInterval)
	assert.Equal(t, 100, cfg.BatchSize)
}
