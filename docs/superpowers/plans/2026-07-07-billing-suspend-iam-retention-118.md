# Billing-suspend → iam retention clock (#118) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development or superpowers:executing-plans. Checkbox (`- [ ]`) steps.
>
> **Verification note:** the end-to-end NATS flow needs a running+provisioned NATS (not available in this sandbox), so tests are **unit-level** (handler logic with a fake service; publish call with a fake publisher). Build/vet + those unit tests are the local gate; the live pub/sub is deploy-verified.

**Goal:** When billing suspends a lapsed subscription, iam suspends the tenant and starts the retention clock — via the existing `thittam.billing.subscription.suspended` event. Billing gets its first publisher; iam its first JetStream consumer.

**Architecture:** `SuspendSubscription` publishes (best-effort) → `FINANCIAL` stream → iam's `ConsumerIamBilling` durable → dispatch handler → `SuspendTenant(nil,nil)`. Mirrors the notifications consumer + expense publisher patterns.

## Global Constraints

- Publish is **best-effort** (`pkg/jetstream/publisher.go:34`): log on error, never roll back the committed suspend.
- At-least-once delivery, no framework dedupe → **handler must be idempotent**; only `active → suspended` is valid (no status regression); Ack unknown types; Nak (return error) only on infra failure.
- `SuspendTenant(ctx, id, nil, nil)` — **no** `freeze_reason` (that would freeze the sweeper).
- Consumers are not auto-created (`consumer.go:69`) — `provision.sh` + `make nats-provision` is a deploy prerequisite.
- Nil-publisher must be safe (existing billing tests/deploys keep working).

---

### Task 1: Billing publishes subscription.suspended

**Files:** `services/billing/service.go`, `services/billing/ports.go` (or a new `events.go`), `cmd/billing/main.go`, `services/billing/service_test.go`.

**Interfaces:**
- Produces: billing `EventPublisher` port + `Service.WithPublisher(p)`; a `subscription.suspended` publish in `SuspendSubscription`.

- [ ] **Step 1: Define the billing EventPublisher port**

In `services/billing` (new `events.go` or in `ports.go` if one exists):

```go
// EventPublisher publishes billing domain events. Implemented by the cmd/billing
// composition root over pkg/jetstream (nil in tests / no-NATS deploys).
type EventPublisher interface {
	PublishSubscriptionSuspended(ctx context.Context, sub *Subscription) error
}
```

- [ ] **Step 2: Add the publisher to Service (nil-safe option)**

In `services/billing/service.go`, add the field + chained option (mirrors iam's `WithAuditLogger`), keeping `NewService(repo)` unchanged so existing callers/tests don't break:

```go
type Service struct {
	repo      Repository
	publisher EventPublisher // optional; nil = no-op
}

func (s *Service) WithPublisher(p EventPublisher) *Service { s.publisher = p; return s }
```

- [ ] **Step 3: Publish in SuspendSubscription**

In `Service.SuspendSubscription` (`service.go:127`), after `UpdateSubscription` succeeds and before `return sub, nil`:

```go
	if s.publisher != nil {
		if err := s.publisher.PublishSubscriptionSuspended(ctx, sub); err != nil {
			// Best-effort: the suspend is committed; a failed publish must not fail the op.
			slog.Default().Warn("publish subscription.suspended failed",
				"tenant_id", tenantID, "subscription_id", sub.ID, "err", err)
		}
	}
```

Add the `log/slog` import if missing.

- [ ] **Step 4: Wire the publisher + adapter in cmd/billing/main.go**

Mirror `cmd/expense-tracking/main.go:56-71` (NATS connect → `nc.JetStream()` → `jetstream.NewPublisher(js)`) and the `expensePublisher` adapter (`:150-177`). Add to `cmd/billing/main.go`:

```go
type billingPublisher struct{ pub *jetstream.Publisher }

func (p *billingPublisher) PublishSubscriptionSuspended(ctx context.Context, sub *billing.Subscription) error {
	now := time.Now().UTC()
	suspendedAt := now
	if sub.SuspendedAt != nil {
		suspendedAt = *sub.SuspendedAt
	}
	return p.pub.Publish(ctx, events.SubjectBillingSubscriptionSuspended, sub.TenantID,
		events.BillingSubscriptionSuspendedPayload{
			SubscriptionID: sub.ID.String(),
			SuspendedAt:    suspendedAt.Format(time.RFC3339),
			PurgeAfter:     suspendedAt.AddDate(0, 0, 30).Format(time.RFC3339),
		})
}
```

and change the service build (`main.go:42`): connect NATS (guard on the NATS URL env — skip the publisher if unset so billing still boots without NATS), then
`svc := billing.NewService(repo).WithPublisher(&billingPublisher{pub: jetstream.NewPublisher(js)})`.
Confirm `Subscription`'s fields (`ID uuid.UUID`, `TenantID uuid.UUID`, `SuspendedAt *time.Time`) — adjust `.String()` / deref to the real types.

- [ ] **Step 5: Publisher unit test**

In `services/billing/service_test.go`, add a fake `EventPublisher` capturing the call; assert `SuspendSubscription` invokes `PublishSubscriptionSuspended` once with the suspended sub, and that a nil publisher path is a clean no-op (no panic). Run: `go test ./services/billing/ -run TestSuspendSubscription -v`.

- [ ] **Step 6: Commit**

```bash
git add services/billing/ cmd/billing/main.go
git commit -m "feat(billing): publish subscription.suspended on SuspendSubscription (#118)"
```

---

### Task 2: iam consumes it → SuspendTenant

**Files:** `pkg/jetstream/config.go`, `services/iam/billing_consumer.go` (new) + test, `cmd/iam/main.go`, `infra/nats/provision.sh`.

**Interfaces:**
- Consumes: `events.SubjectBillingSubscriptionSuspended`, `iam.Service.GetTenant` + `SuspendTenant`.
- Produces: `jetstream.ConsumerIamBilling`; a `MessageHandler` dispatch.

- [ ] **Step 1: Add the consumer config**

In `pkg/jetstream/config.go`, add the constant (after `:49`): `ConsumerIamBilling = "iam-billing"`, and an entry in `FinancialConsumers()`:

```go
		{
			StreamName:     StreamFinancial,
			DurableName:    ConsumerIamBilling,
			FilterSubjects: []string{"thittam.billing.subscription.suspended"},
			MaxDeliver:     MaxDeliverAttempts,
			AckWait:        AckWait,
			BackOff:        DeliveryBackOff,
			Description:    "IAM service billing consumer. Suspends the tenant + starts the retention clock on subscription suspension (#118).",
		},
```

- [ ] **Step 2: Write the dispatch handler test FIRST (TDD — it's the meat)**

Create `services/iam/billing_consumer_test.go`. Define a fake suspender capturing calls, and cover:
- active tenant + subscription.suspended → `SuspendTenant` called once; handler returns nil.
- already-suspended (GetTenant returns status=suspended) → no `SuspendTenant` call; returns nil (no-op/Ack).
- non-active (grace/deactivated) → no call; returns nil (no regression).
- unknown `env.Type` → no call; returns nil.
- GetTenant/SuspendTenant returns infra error → handler returns error (Nak).

```go
// sketch — adapt to the real handler constructor
func TestBillingConsumer_SuspendsActiveTenant(t *testing.T) {
	var suspended []uuid.UUID
	fake := &fakeSuspender{
		getTenantFn: func(_ context.Context, id uuid.UUID) (*iam.Tenant, error) {
			return &iam.Tenant{ID: id, Status: "active"}, nil
		},
		suspendFn: func(_ context.Context, id uuid.UUID, _ *time.Time, _ *string) (*iam.Tenant, error) {
			suspended = append(suspended, id); return &iam.Tenant{ID: id, Status: "suspended"}, nil
		},
	}
	h := iam.NewBillingConsumer(fake)
	env := &events.Envelope{Type: events.SubjectBillingSubscriptionSuspended, TenantID: uuid.New()}
	require.NoError(t, h.Handle(context.Background(), env))
	require.Len(t, suspended, 1)
}
```

- [ ] **Step 3: Implement the handler**

Create `services/iam/billing_consumer.go`:

```go
package iam

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/wegofwd2020/thittam/pkg/events"
)

// tenantSuspender is the iam surface the billing consumer needs.
type tenantSuspender interface {
	GetTenant(ctx context.Context, id uuid.UUID) (*Tenant, error)
	SuspendTenant(ctx context.Context, id uuid.UUID, holdUntil *time.Time, freezeReason *string) (*Tenant, error)
}

type BillingConsumer struct{ svc tenantSuspender }

func NewBillingConsumer(svc tenantSuspender) *BillingConsumer { return &BillingConsumer{svc: svc} }

// Handle is a pkg/jetstream.MessageHandler. Idempotent + at-least-once safe:
// only active → suspended; unknown types and non-active tenants Ack (return nil);
// only infra failures return an error (Nak → backoff/DLQ).
func (c *BillingConsumer) Handle(ctx context.Context, env *events.Envelope) error {
	if env.Type != events.SubjectBillingSubscriptionSuspended {
		return nil // unknown type → Ack + skip
	}
	t, err := c.svc.GetTenant(ctx, env.TenantID)
	if err != nil {
		return err // infra failure → Nak
	}
	if t.Status != "active" {
		return nil // already suspended/further along → no regression, Ack
	}
	if _, err := c.svc.SuspendTenant(ctx, env.TenantID, nil, nil); err != nil {
		return err // Nak
	}
	return nil
}
```

Confirm `Service` satisfies `tenantSuspender` (it has `GetTenant` + `SuspendTenant`; add a `var _ tenantSuspender = (*Service)(nil)` assertion). If `GetTenant` isn't on `Service`, use the repo-backed getter it exposes, or add a thin passthrough.

- [ ] **Step 4: Run the handler tests**

Run: `go test ./services/iam/ -run TestBillingConsumer -v` → all pass. `go build ./services/iam/...`.

- [ ] **Step 5: Wire the consumer into cmd/iam/main.go**

Mirror `cmd/notifications/main.go:42-101`: connect NATS (guard on the NATS URL env so iam still boots without NATS), `nc.JetStream()`,
`sub, err := jetstream.Subscribe(js, jetstream.StreamFinancial, jetstream.ConsumerIamBilling, iam.NewBillingConsumer(svc).Handle)`,
drain `sub` on shutdown (add to the existing teardown alongside the impersonation ticker/gateway), and add a NATS ping to the health check. Log clearly when NATS is unconfigured (consumer disabled).

- [ ] **Step 6: Add the provision.sh consumer block**

In `infra/nats/provision.sh`, copy the `notifications-financial` consumer block (~`:141-158`) as `iam-billing`, `--filter=thittam.billing.subscription.suspended`, `--target=push.iam-billing`. This is a deploy step (`make nats-provision`).

- [ ] **Step 7: Whole-tree vet + build + unit tests**

Run: `go vet ./... && go build ./... && go test ./services/iam/ ./services/billing/ ./pkg/jetstream/ -short`
Expected: clean; handler + publisher tests pass. (Live pub/sub is deploy-verified — no NATS locally.)

- [ ] **Step 8: Commit**

```bash
git add pkg/jetstream/config.go services/iam/billing_consumer.go services/iam/billing_consumer_test.go \
        cmd/iam/main.go infra/nats/provision.sh
git commit -m "feat(iam): consume subscription.suspended → SuspendTenant, start retention clock (#118)"
```

---

## Self-Review

**Spec coverage:**
- Billing publishes subscription.suspended (best-effort, nil-safe) → Task 1. ✅
- iam consumer on FINANCIAL/ConsumerIamBilling filtered to subject → Task 2 Steps 1,5. ✅
- Handler: SuspendTenant(nil,nil), idempotent, no-regression, Ack unknown, Nak infra → Task 2 Steps 2-3. ✅
- provision.sh consumer → Task 2 Step 6. ✅
- Unit tests (dispatch + publisher) → Task 1 Step 5, Task 2 Step 2. ✅
- Out of scope (gateway webhook, suspend audit, ops nats-provision) — noted, not built. ✅

**Type consistency:** `EventPublisher.PublishSubscriptionSuspended(ctx, *Subscription)` — same in the port (Task 1 Step 1), adapter (Step 4), and fake (Step 5). `tenantSuspender` (GetTenant + SuspendTenant) implemented by `Service` (asserted). `BillingConsumer.Handle` matches `pkg/jetstream.MessageHandler` `func(ctx, *events.Envelope) error`. Subject string `thittam.billing.subscription.suspended` identical in config filter, provision.sh, and `events.SubjectBillingSubscriptionSuspended`.

**Placeholder scan:** No TBD. Payload fields are the real ones (`SubscriptionID`/`SuspendedAt`/`PurgeAfter` strings). The "confirm Subscription field types" / "confirm Service has GetTenant" are explicit verification steps with fallbacks. NATS boilerplate references the exact template file:line rather than repeating it — the template IS the spec for that wiring.

**Interface note:** `EventPublisher` (billing) and `tenantSuspender` (iam) are new interfaces, each with a single production implementer + a test fake; `var _` assertions guard them. No existing interface widens. Whole-tree `go vet` (Task 2 Step 7) is the backstop. See [[reference_iam_repository_implementers]].
