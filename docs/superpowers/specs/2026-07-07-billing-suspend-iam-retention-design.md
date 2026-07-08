# Design: billing subscription-suspend → iam retention clock (#118)

**Issue:** #118 (carved from #90). **Date:** 2026-07-07
**Scope:** billing (first event publisher) + iam (first event consumer) + NATS provisioning.
**Branch:** `feat/billing-suspend-iam-retention-118`

## Context

When a tenant's billing lapses, iam should suspend the tenant and start the retention
clock (`suspended_at = now()` → sweeper cascades suspended→grace→deactivated→purge_eligible,
#92). Today nothing connects billing to iam: billing's `SuspendSubscription` only flips its
own subscription row; iam never hears about it.

**Refinement of the issue's wording:** the issue says "on `invoice.payment_failed`", but the
correct point to start the *retention* clock is when the subscription is actually
**suspended** (post-dunning, Day 14) — not on a first failed charge. Billing already has that
point (`SuspendSubscription`, `services/billing/service.go:127`) and already defines the event
type `SubjectBillingSubscriptionSuspended = "thittam.billing.subscription.suspended"`
(`pkg/events/events.go:41`) + `BillingSubscriptionSuspendedPayload` (`:206`) — it just never
publishes it. So #118 reuses the existing subject/payload; no new event type, no webhook build.

**This builds both halves of a new event pipe** — billing has no publisher wired
(`cmd/billing/main.go:39-40`: "No NATS publisher… reserved for a future PR"), and iam has **no
event consumer at all** (pure gRPC server). Model: the notifications consumer
(`cmd/notifications/main.go` + `dispatcher.go`) using the shared `pkg/jetstream` helpers.

## Component 1 — billing publishes subscription.suspended

**Publisher wiring** — `cmd/billing/main.go`: connect NATS (`nats.Connect`), `nc.JetStream()`,
`jetstream.NewPublisher(js)` (mirror `cmd/expense-tracking/main.go:56-71`). Define a narrow
billing `EventPublisher` interface in `services/billing` + a `billingPublisher` adapter in
`cmd/billing/main.go` (avoids import cycle; mirror `expensePublisher`). Inject into
`billing.NewService(repo, publisher)`.

**Publish call** — in `Service.SuspendSubscription` (`service.go:127`), after
`UpdateSubscription` succeeds, publish **best-effort** (log-on-error, never roll back the
committed suspend — the publisher contract, `pkg/jetstream/publisher.go:34-37`):

```go
if s.publisher != nil {
    payload := events.BillingSubscriptionSuspendedPayload{
        SubscriptionID: sub.ID, SuspendedAt: now, PurgeAfter: /* now + retention grace */,
    }
    if err := s.publisher.Publish(ctx, events.SubjectBillingSubscriptionSuspended, tenantID, payload); err != nil {
        s.logger.Warn("publish subscription.suspended failed", "tenant", tenantID, "err", err)
    }
}
```

(Confirm the exact `BillingSubscriptionSuspendedPayload` fields during implementation; set
what's available. `PurgeAfter` is informational for consumers — iam derives its own clock from
`suspended_at`, so an approximate value is fine.) Because `thittam.billing.>` is in
`FinancialSubjects`, this routes to the `FINANCIAL` stream (DLQ-protected).

**Nil-publisher safe:** existing billing tests / deployments without a publisher keep working
(the guard), so this is additive.

## Component 2 — iam consumes it and suspends the tenant

**Consumer wiring** — `cmd/iam/main.go` (iam's first NATS): connect NATS, `nc.JetStream()`,
`jetstream.Subscribe(js, jetstream.StreamFinancial, jetstream.ConsumerIamBilling, dispatch)`,
drain the subscription on shutdown (alongside the existing impersonation-ticker/gateway
teardown), and add a NATS health check. Guard the whole block on NATS being configured (env)
so iam still boots without NATS in minimal/test setups.

**Consumer config** — `pkg/jetstream/config.go`: add `ConsumerIamBilling` durable name
constant + a `ConsumerConfig` in `FinancialConsumers()` bound to `StreamFinancial`, filtered to
`thittam.billing.subscription.suspended`.

**Dispatch handler** — new `services/iam/billing_consumer.go` (or a `cmd/iam` dispatcher
mirroring `cmd/notifications/dispatcher.go`): a `MessageHandler` that switches on `env.Type`:
- `SubjectBillingSubscriptionSuspended` → unmarshal payload → `svc.SuspendTenant(ctx, env.TenantID, nil, nil)`.
  - `nil, nil` = suspend + start clock, **no legal hold** (a `freeze_reason` would *freeze* the
    sweeper — the opposite of what we want).
- Unknown type → return `nil` (Ack + skip).

**Idempotency (at-least-once):** the framework does not dedupe; safety rests on handler
idempotency. `SuspendTenant` sets `suspended_at` only on first entry to suspended (COALESCE),
so a redelivered event does not reset the clock. The handler returns `nil` (Ack) on
"already suspended" and only returns an error (Nak → backoff/DLQ) on genuine infra failure
(DB down). To avoid a status-regression edge (a tenant already in grace/deactivated getting
pulled back to suspended), the handler should **no-op when the tenant is not `active`** — only
`active → suspended` is a valid billing-driven transition; log+Ack otherwise.

## Component 3 — ops provisioning

`infra/nats/provision.sh`: add a `consumer add FINANCIAL iam-billing` block (copy the
`notifications-financial` block ~`:141-158`), `--filter=thittam.billing.subscription.suspended`,
`--target=push.iam-billing`. Consumers are not auto-created (`consumer.go:69`), so
`make nats-provision` must run before iam binds. Document in the PR that this is a deploy step.

## Component 4 — tests

- **iam dispatch test** (mirror `cmd/notifications/dispatcher_test.go` / `pkg/jetstream/consumer_test.go`):
  - subscription.suspended for an `active` tenant → calls `SuspendTenant` once; returns nil (Ack).
  - redelivery / already-suspended tenant → no second effective suspend; returns nil (Ack).
  - non-active tenant (grace/deactivated) → no-op, Ack (no status regression).
  - unknown event type → Ack, no call.
  - service error (DB down) → returns error (Nak).
  Use a mock/fake iam service exposing `SuspendTenant`.
- **billing publisher test** (mirror `cmd/*/publisher_test.go`): `SuspendSubscription` publishes
  `subscription.suspended` with the right subject/tenant; nil-publisher path is a clean no-op.

## Acceptance criteria

- [ ] `SuspendSubscription` publishes `subscription.suspended` (best-effort; nil-publisher safe).
- [ ] iam runs a JetStream consumer on `StreamFinancial`/`ConsumerIamBilling` filtered to the subject.
- [ ] Handler suspends the tenant via `SuspendTenant(nil,nil)` (clock starts, no hold); idempotent; no-op on non-active tenants; Ack unknown types; Nak infra failures.
- [ ] `provision.sh` adds the iam-billing consumer.
- [ ] Unit tests: dispatch (idempotency, no-regression, unknown, error) + billing publisher.
- [ ] `go vet ./...` clean; builds; iam/billing unit tests pass.

## Out of scope / follow-up

- Full gateway webhook (`billing/handler.go:288` stub) + signature verification — separate; the
  dunning `SuspendSubscription` path is the trigger here.
- Auditing the billing-driven suspend (SuspendTenant only audits legal-hold today) — note as a
  gap; the sweeper's subsequent transitions are audited (#92/#121).
- **Ops:** running `make nats-provision` (adds the consumer) is a deploy prerequisite.
- **#119** (SetTenantRetention override), **PurgeTenant** — separate PRs.

## Files touched

- `pkg/events/events.go` (reuse existing subject/payload — likely no change; verify fields)
- `services/billing/service.go` (publisher field + publish in SuspendSubscription), a billing `EventPublisher` interface
- `cmd/billing/main.go` (NATS + publisher wiring + adapter)
- `pkg/jetstream/config.go` (ConsumerIamBilling + FinancialConsumers entry)
- `cmd/iam/main.go` (NATS consumer wiring + drain + health)
- `services/iam/billing_consumer.go` (new dispatch handler)
- `infra/nats/provision.sh` (consumer block)
- tests: iam dispatch + billing publisher

Review: cross-service (billing→iam) + new consumer infra → senior review.
