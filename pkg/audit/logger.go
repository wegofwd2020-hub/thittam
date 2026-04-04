package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Store persists audit events. Implementations must be append-only —
// no UPDATE or DELETE operations.
type Store interface {
	// Insert persists a single audit event.
	Insert(ctx context.Context, event Event) error

	// InsertBatch persists multiple events in a single transaction.
	InsertBatch(ctx context.Context, events []Event) error

	// Query returns audit events matching the filter.
	Query(ctx context.Context, filter QueryFilter) ([]Event, error)
}

// ServiceLogger defines structured logging for the audit package.
type ServiceLogger interface {
	Info(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
}

type defaultServiceLogger struct{}

func (defaultServiceLogger) Info(msg string, kv ...interface{})  { log.Printf("[INFO] audit: %s %v", msg, kv) }
func (defaultServiceLogger) Warn(msg string, kv ...interface{})  { log.Printf("[WARN] audit: %s %v", msg, kv) }
func (defaultServiceLogger) Error(msg string, kv ...interface{}) { log.Printf("[ERROR] audit: %s %v", msg, kv) }

// Logger is the primary audit logging interface. Services call Log() to
// record audit events. Events are buffered and flushed asynchronously
// to avoid blocking the request path.
type Logger struct {
	store   Store
	svcLog  ServiceLogger
	ch      chan Event
	wg      sync.WaitGroup
	done    chan struct{}
}

// LoggerConfig configures the audit Logger.
type LoggerConfig struct {
	// BufferSize is the channel buffer size. Events are dropped if full.
	// Default: 1000.
	BufferSize int

	// FlushInterval is how often buffered events are flushed to the store.
	// Default: 5 seconds.
	FlushInterval time.Duration

	// BatchSize is the maximum events per batch flush.
	// Default: 100.
	BatchSize int
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() LoggerConfig {
	return LoggerConfig{
		BufferSize:    1000,
		FlushInterval: 5 * time.Second,
		BatchSize:     100,
	}
}

// NewLogger creates an audit Logger with a buffered async writer.
// Call Close() to flush remaining events on shutdown.
func NewLogger(store Store, cfg LoggerConfig, svcLog ServiceLogger) *Logger {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 1000
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 5 * time.Second
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if svcLog == nil {
		svcLog = defaultServiceLogger{}
	}

	l := &Logger{
		store:  store,
		svcLog: svcLog,
		ch:     make(chan Event, cfg.BufferSize),
		done:   make(chan struct{}),
	}

	l.wg.Add(1)
	go l.flushLoop(cfg.FlushInterval, cfg.BatchSize)

	return l
}

// Log records an audit event. Non-blocking — if the buffer is full, the
// event is dropped and a warning is logged.
func (l *Logger) Log(event Event) {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now()
	}

	select {
	case l.ch <- event:
	default:
		l.svcLog.Warn("audit buffer full, event dropped",
			"action", string(event.Action),
			"resource_type", string(event.ResourceType),
			"resource_id", event.ResourceID.String(),
		)
	}
}

// LogAction is a convenience method that builds and logs an Event.
func (l *Logger) LogAction(
	tenantID, actorID uuid.UUID,
	actorEmail string,
	action Action,
	resourceType ResourceType,
	resourceID uuid.UUID,
	oldState, newState interface{},
	metadata map[string]interface{},
) {
	event := Event{
		TenantID:     tenantID,
		ActorID:      actorID,
		ActorEmail:   actorEmail,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
	}

	if oldState != nil {
		if data, err := json.Marshal(oldState); err == nil {
			event.OldState = data
		}
	}
	if newState != nil {
		if data, err := json.Marshal(newState); err == nil {
			event.NewState = data
		}
	}
	if metadata != nil {
		if data, err := json.Marshal(metadata); err == nil {
			event.Metadata = data
		}
	}

	l.Log(event)
}

// Close flushes remaining events and stops the background writer.
// Blocks until all events are flushed or the context is cancelled.
func (l *Logger) Close(ctx context.Context) error {
	close(l.done)
	l.wg.Wait()

	// Drain any remaining events in the channel
	return l.drainChannel(ctx)
}

// flushLoop runs in a goroutine, batching and flushing events periodically.
func (l *Logger) flushLoop(interval time.Duration, batchSize int) {
	defer l.wg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	batch := make([]Event, 0, batchSize)

	for {
		select {
		case event := <-l.ch:
			batch = append(batch, event)
			if len(batch) >= batchSize {
				l.flushBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				l.flushBatch(batch)
				batch = batch[:0]
			}

		case <-l.done:
			// Flush remaining batch before exit
			if len(batch) > 0 {
				l.flushBatch(batch)
			}
			return
		}
	}
}

// flushBatch writes a batch of events to the store.
func (l *Logger) flushBatch(batch []Event) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := l.store.InsertBatch(ctx, batch); err != nil {
		l.svcLog.Error("failed to flush audit batch",
			"count", len(batch),
			"error", err.Error(),
		)
	}
}

// drainChannel flushes any events remaining in the channel after Close().
func (l *Logger) drainChannel(ctx context.Context) error {
	var remaining []Event
	for {
		select {
		case event := <-l.ch:
			remaining = append(remaining, event)
		default:
			if len(remaining) > 0 {
				return l.store.InsertBatch(ctx, remaining)
			}
			return nil
		}
	}
}

// Query returns audit events matching the filter. This is a synchronous
// read — not buffered.
func (l *Logger) Query(ctx context.Context, filter QueryFilter) ([]Event, error) {
	if filter.Limit <= 0 || filter.Limit > 1000 {
		filter.Limit = 100
	}
	return l.store.Query(ctx, filter)
}

// Pending returns the number of events in the buffer waiting to be flushed.
func (l *Logger) Pending() int {
	return len(l.ch)
}

// String returns a human-readable summary for debugging.
func (e Event) String() string {
	return fmt.Sprintf("[%s] %s %s/%s by %s",
		e.OccurredAt.Format(time.RFC3339),
		e.Action, e.ResourceType, e.ResourceID, e.ActorEmail,
	)
}
