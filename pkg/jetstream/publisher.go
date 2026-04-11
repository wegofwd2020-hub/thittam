package jetstream

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/wegofwd2020/thittam/pkg/events"
)

// JetStreamContext is the narrow subset of nats.JetStreamContext that Publisher uses.
// Keeping this minimal makes mocking in tests trivial.
type JetStreamContext interface {
	Publish(subj string, data []byte, opts ...nats.PubOpt) (*nats.PubAck, error)
}

// Publisher wraps a JetStreamContext and publishes events.Envelope messages.
// It is the single concrete NATS implementation shared by all services.
// Each service defines its own narrow EventPublisher interface; cmd adapters
// delegate to this Publisher to avoid import cycles.
type Publisher struct {
	js JetStreamContext
}

// NewPublisher creates a Publisher backed by the given JetStream context.
func NewPublisher(js JetStreamContext) *Publisher {
	return &Publisher{js: js}
}

// Publish serialises payload into an events.Envelope and publishes it to subject.
// Best-effort: the returned error is informational — callers should log it but
// must not roll back already-committed domain operations because of it.
// (Financial events should use the DLQ stream to recover from consumer failures,
// not from publisher failures, which are extremely rare.)
func (p *Publisher) Publish(ctx context.Context, subject string, tenantID uuid.UUID, payload interface{}) error {
	env, err := events.NewEnvelope(subject, tenantID, payload)
	if err != nil {
		return fmt.Errorf("jetstream: build envelope for %s: %w", subject, err)
	}

	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("jetstream: marshal envelope for %s: %w", subject, err)
	}

	if _, err := p.js.Publish(subject, data); err != nil {
		return fmt.Errorf("jetstream: publish %s: %w", subject, err)
	}

	slog.DebugContext(ctx, "event published",
		"subject", subject,
		"event_id", env.EventID,
		"tenant_id", tenantID,
	)
	return nil
}
