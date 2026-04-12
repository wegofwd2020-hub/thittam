package jetstream

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/wegofwd2020/thittam/pkg/events"
)

// MessageHandler is the callback invoked by Subscribe for each decoded event.
// Return nil to acknowledge the message (processed successfully).
// Return a non-nil error to negative-acknowledge it (NATS will redeliver).
// Unknown event types should be acknowledged (return nil) to remain
// forward-compatible: a consumer must never block on an event it does not
// understand.
type MessageHandler func(ctx context.Context, env *events.Envelope) error

// ackableMsg is a minimal interface over *nats.Msg used by dispatchMsg.
// Extracted so the dispatch logic can be unit-tested without a live NATS connection.
// The variadic AckOpt matches *nats.Msg's actual Ack/Nak signatures.
type ackableMsg interface {
	Ack(opts ...nats.AckOpt) error
	Nak(opts ...nats.AckOpt) error
}

// dispatchMsg decodes the raw NATS message body and calls handler.
// On a decode error the message is acked (bad payloads must not block the queue).
// On a handler error the message is nacked so NATS redelivers after recovery.
// This function is unexported but called directly by Subscribe and tested via consumer_test.go.
func dispatchMsg(msg ackableMsg, data []byte, subject, consumerName string, handler MessageHandler) {
	ctx := context.Background()

	var env events.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		slog.Error("jetstream: failed to decode envelope",
			"consumer", consumerName,
			"subject", subject,
			"error", err,
		)
		// Bad envelope — ack to avoid infinite redelivery of an unparseable message.
		_ = msg.Ack()
		return
	}

	if err := handler(ctx, &env); err != nil {
		slog.Error("jetstream: handler error — naking message",
			"consumer", consumerName,
			"event_type", env.Type,
			"event_id", env.EventID,
			"error", err,
		)
		_ = msg.Nak()
		return
	}

	if err := msg.Ack(); err != nil {
		slog.Error("jetstream: failed to ack message",
			"consumer", consumerName,
			"event_id", env.EventID,
			"error", err,
		)
	}
}

// Subscribe binds to an existing durable push consumer and dispatches messages
// to handler in a background goroutine. The consumer must already exist in NATS
// (created via `make nats-provision` / infra/nats/provision.sh).
//
// Parameters:
//
//	js         — JetStream context from nats.Conn.JetStream()
//	streamName — the NATS JetStream stream name (e.g. StreamEvents, StreamFinancial)
//	consumerName — the durable consumer name (e.g. ConsumerNotificationsEvents)
//	handler    — called for each message; nil return = Ack, error = Nak
//
// Returns the underlying *nats.Subscription so the caller can drain it on shutdown.
func Subscribe(js nats.JetStreamContext, streamName, consumerName string, handler MessageHandler) (*nats.Subscription, error) {
	sub, err := js.Subscribe(
		"", // subject is empty when binding to a named consumer
		func(msg *nats.Msg) {
			dispatchMsg(msg, msg.Data, msg.Subject, consumerName, handler)
		},
		nats.Bind(streamName, consumerName),
		nats.AckExplicit(),
		nats.ManualAck(),
	)
	if err != nil {
		return nil, fmt.Errorf("jetstream: subscribe to %s/%s: %w", streamName, consumerName, err)
	}

	slog.Info("jetstream: consumer subscribed",
		"stream", streamName,
		"consumer", consumerName,
	)
	return sub, nil
}
