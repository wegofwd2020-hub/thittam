package iam

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/wegofwd2020/thittam/pkg/events"
)

// tenantSuspender is the iam surface the billing consumer needs. Satisfied by
// *Service; kept as a narrow interface so the handler can be unit-tested
// without a database.
type tenantSuspender interface {
	GetTenant(ctx context.Context, id uuid.UUID) (*Tenant, error)
	SuspendTenant(ctx context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason *string) (*Tenant, error)
}

var _ tenantSuspender = (*Service)(nil)

// BillingConsumer dispatches billing domain events onto iam.Service. It is
// iam's first JetStream consumer (#118): billing suspends a subscription for
// non-payment, and iam mirrors that by suspending the tenant, which starts
// the retention clock (active → suspended → grace → deactivated → purge_eligible).
type BillingConsumer struct {
	svc tenantSuspender
}

// NewBillingConsumer builds a BillingConsumer over the given tenant surface.
func NewBillingConsumer(svc tenantSuspender) *BillingConsumer {
	return &BillingConsumer{svc: svc}
}

// Handle is a pkg/jetstream.MessageHandler. It is idempotent and safe under
// NATS's at-least-once delivery:
//   - unknown event types are Acked (returns nil) — forward-compatible.
//   - a tenant that is not currently active is left alone and Acked (returns
//     nil) — this is a no-regression guard: a tenant already suspended,
//     in grace, deactivated, or purge-eligible must not be reset backwards
//     by a redelivered or out-of-order event.
//   - infra failures (GetTenant/SuspendTenant) return an error, which Naks
//     the message so NATS retries with backoff.
//
// The hold passed to SuspendTenant is always nil/nil: billing suspension is
// not a legal hold, and a non-nil freezeReason would freeze the retention
// sweeper, which defeats the purpose of starting the retention clock.
func (c *BillingConsumer) Handle(ctx context.Context, env *events.Envelope) error {
	if env.Type != events.SubjectBillingSubscriptionSuspended {
		return nil // unknown/irrelevant type — Ack + skip
	}

	tenant, err := c.svc.GetTenant(ctx, env.TenantID)
	if err != nil {
		return err // infra failure — Nak
	}

	if tenant.Status != TenantStatusActive {
		return nil // already suspended or further along — no regression, Ack
	}

	if _, err := c.svc.SuspendTenant(ctx, env.TenantID, nil, nil); err != nil {
		return err // infra failure — Nak
	}
	return nil
}
