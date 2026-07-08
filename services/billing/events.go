package billing

import "context"

// EventPublisher publishes billing domain events. Implemented by the cmd/billing
// composition root over pkg/jetstream (nil in tests / no-NATS deploys).
type EventPublisher interface {
	PublishSubscriptionSuspended(ctx context.Context, sub *Subscription) error
}
