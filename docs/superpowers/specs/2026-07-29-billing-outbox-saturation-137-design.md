# Billing outbox: broker-health dead-letter guard + DLQ follow-ups (#137) — Design

**Status:** approved design, pre-plan
**Date:** 2026-07-29
**Issue:** #137 (outbox guard defeated at batch saturation; DLQ follow-ups) — from #134/PR #136 review
**Branch:** `fix/billing-outbox-saturation-137` off `main`
**Migration:** none · **Proto:** none · **sqlc:** none

## Goal

`processBatch` dead-letters a maxed-attempts poison row only when `published > 0` — an
**in-batch** broker-health signal. When the `relayBatchSize` (100) oldest unsent rows are all
poison, `published == 0` every tick, the guard declines, the same 100 re-claim forever
(`ClaimUnsentOutbox` orders by `created_at`), and healthy events queued behind them **starve** —
while the critical `BillingOutboxDeadLetter` alert never fires. Replace the in-batch signal with
an **out-of-batch** one: dead-letter when the broker is demonstrably healthy, where "healthy" =
a publish succeeded within a recent window. Plus the §2/§3 follow-ups.

## Context (grounding facts, `main` @ 390b3e1)

- **`processBatch`** (`services/billing/outbox_relay.go:114-164`): claims a batch, publishes each,
  holds failures, then `canDeadLetter := published > 0` (line 145); for each held failure at
  `Attempts >= maxOutboxAttempts` (5, const at :36) it `MoveOutboxToDead`s, else
  `RecordOutboxFailure`s. `Run` (:83-105) ticks every `relayInterval` (15s), calls `processBatch`
  then `updateGauges`.
- **`ClaimUnsentOutbox`** (`db/postgres.go:159-195`): `UPDATE … SET attempts = attempts + 1 …
  ORDER BY created_at … FOR UPDATE SKIP LOCKED LIMIT $1` — increments attempts on claim; oldest-
  first, so the poison 100 are permanently the claimed set.
- **No existing publish-recency signal.** The `..._outbox_stats_last_success_timestamp_seconds`
  gauge is fed by `OutboxStats` succeeding (a Postgres read — same DB as the claim, unrelated to
  NATS health) and is a write-only promauto gauge (no in-process getter). The `Relay` struct
  (:71-80) has only `repo`, `pub`, `log` — **no clock, no publish-time field.** Tests
  (`outbox_relay_test.go`) use real wall-clock; there is no `now` seam.
- **`maxOutboxAttempts = 5`; `relayInterval = 15s`** → a fresh poison row needs 5 claims = 75s of
  ticks to reach `Attempts >= 5`. The recent-window must exceed that.
- **§2:** `infra/prometheus/alerts/billing.yaml` has 4 rules (`BillingDunningFailureSpike`,
  `BillingSuspendedSubscriptionsHigh`, `BillingPlanLimitRejectionsHigh`,
  `BillingServiceErrorRateElevated`) referencing metrics that grep-confirms **exist nowhere** in
  the tree (`billing_dunning_attempts_total`, `billing_subscriptions_total`,
  `billing_plan_limit_rejections_total`, `grpc_server_handled_total`). A header block (:21-29)
  documents them dormant. Since #136 enabled the billing scrape they are live-but-unfirable.
- **§3 minors:** (a) `e2e/critical_path/billing_test.go` `billingRepo.ListDeadOutbox` (:326-335)
  returns insertion order; real query is `ORDER BY died_at DESC` (:266). (b)
  `outbox_relay_test.go` `TestRelay_MoveToDeadError_TickContinues` (:172-187) sets no
  `recordOutboxFailureFn`, so it can't prove the move-error path doesn't fall through to
  `RecordOutboxFailure`. (c) `ReplayDeadOutbox` raw unique-violation vs sentinel — "unreachable
  while one-table-only invariant holds" (skip). (d) `outbox-admin` "capped at N" wording —
  cosmetic (skip).
- **§4:** a `GRANT` on `event_outbox_dead` for `thittam_app` — covered by the broad
  `scripts/db-grant-app-role.sql` (`ALL TABLES IN SCHEMA public`, re-run after migrations). Pure
  deploy/runbook concern → out of code scope, belongs with #123.

## Design

### §1 — out-of-batch broker-health guard (`services/billing/outbox_relay.go`)

Add to the `Relay` struct + `NewRelay`:
```go
type Relay struct {
	repo outboxRepo
	pub  outboxPublisher
	log  *slog.Logger

	// now is a clock seam for tests; defaults to time.Now.
	now func() time.Time
	// lastPublishSuccess is the wall-clock of the most recent successful NATS
	// publish. It is the out-of-batch broker-health signal: a maxed-attempts row
	// is dead-lettered only when the broker is demonstrably healthy (a publish
	// succeeded within brokerHealthyWindow), distinguishing a poison payload from
	// a NATS outage in which nothing publishes (#137). Zero value at startup =
	// "no recent success" = do not dead-letter yet (conservative).
	lastPublishSuccess time.Time
}

func NewRelay(repo outboxRepo, pub outboxPublisher) *Relay {
	return &Relay{repo: repo, pub: pub, log: slog.Default().With("component", "outbox-relay"), now: time.Now}
}
```
New const:
```go
	// brokerHealthyWindow bounds how recently a publish must have succeeded for the
	// relay to treat the broker as healthy and dead-letter a maxed poison row.
	// Must exceed maxOutboxAttempts*relayInterval (75s) so a fresh poison row can
	// reach maxOutboxAttempts while the last pre-saturation success is still in
	// window. Trade-off: during a genuine NATS outage lasting between 75s and this
	// window, a row that only now reaches maxOutboxAttempts is dead-lettered rather
	// than retried — but such a row is replayable via outbox-admin, and a poison
	// row's earlier failures already occurred while the broker was healthy.
	brokerHealthyWindow = 5 * time.Minute
```
In `processBatch`, set the signal on each successful publish (after `outboxPublished.Inc(); published++`):
```go
		r.lastPublishSuccess = r.now()
```
Replace the guard (line 145):
```go
	// A batch in which nothing published could be an outage (NATS down) OR a batch
	// of the oldest rows all being poison. Distinguish them by out-of-batch broker
	// health: if a publish succeeded within brokerHealthyWindow, the broker is up
	// and these failures are the rows' fault → dead-letter; otherwise assume outage
	// and retry (never park the whole backlog during a real NATS outage). (#137)
	brokerHealthy := published > 0 || r.now().Sub(r.lastPublishSuccess) < brokerHealthyWindow
```
and use `brokerHealthy` where `canDeadLetter` was (`if brokerHealthy && f.event.Attempts >= maxOutboxAttempts`). `Relay` is single-goroutine (`Run` loops sequentially) → `lastPublishSuccess` needs no mutex.

**Residual (documented, out of required scope):** the pure worst case — the oldest 100 rows are
poison with *zero* interleaved healthy events, sustained past `brokerHealthyWindow` — still
starves, because no publish ever succeeds to refresh the window. The issue rates this implausible
("≥100 simultaneous poison rows, implausible with one event type today"). Not addressed here.

### §2 — delete the 4 dormant alert rules (`infra/prometheus/alerts/billing.yaml`)

Remove `BillingDunningFailureSpike`, `BillingSuspendedSubscriptionsHigh`,
`BillingPlanLimitRejectionsHigh`, `BillingServiceErrorRateElevated` and the `KNOWN DORMANT RULES`
header block. Keep every rule whose metric this tree actually emits (the outbox rules:
`BillingOutboxBacklogStale`, `BillingOutboxDeadLetter`, `BillingOutboxStatsStale`, etc.). A rule
that cannot fire reads as health; re-add each with its metric when that metric is wired.

### §3 — the two valuable test-hardening minors

- **(a)** `e2e/critical_path/billing_test.go` `billingRepo.ListDeadOutbox`: sort the returned slice
  by `died_at DESC` (mirror the real query) so an ordering regression can't pass e2e. (The mock
  appends in `MoveOutboxToDead`; either track a `diedAt` per entry and sort, or reverse insertion
  order as a proxy — prefer sorting by a recorded `died_at` for fidelity.)
- **(b)** `outbox_relay_test.go` `TestRelay_MoveToDeadError_TickContinues`: set
  `recordOutboxFailureFn` to a spy that fails the test if called (`t.Fatalf`), pinning that a
  `MoveOutboxToDead` error does NOT fall through to `RecordOutboxFailure` (double-record).

Skip §3(c) (unreachable per the one-table invariant) and §3(d) (cosmetic wording).

## Testing

- **§1** (`outbox_relay_test.go`, using the new `now` seam — set `r.now` to a controllable clock):
  - `TestRelay_Saturation_DeadLettersWhenBrokerRecentlyHealthy`: a batch of only maxed-attempts
    poison rows, `published == 0`, but `lastPublishSuccess` set to `now - 1min` (within window) →
    all dead-lettered (proves the fix — the saturation case now parks poison).
  - `TestRelay_Outage_DoesNotDeadLetter`: maxed poison rows, `published == 0`, `lastPublishSuccess`
    zero or `now - 10min` (outside window) → `RecordOutboxFailure`, NOT `MoveOutboxToDead` (proves
    a real outage still retries, never parks).
  - Keep `TestRelay_PoisonRow_DeadLetteredWhenSiblingSucceeds` (published>0 path still works).
  - Revise `TestRelay_TotalBatchFailure_NeverDeadLetters` → it currently asserts the *bug*; update
    it to the outage semantics (stale/zero `lastPublishSuccess` → never dead-letter) — this is the
    same assertion re-framed, now with an explicit stale clock.
  - A publish-success sets `lastPublishSuccess`: assert via a follow-up all-poison batch that then
    dead-letters (or expose it through behavior only — no getter needed).
- **§3**: the two hardened tests above.
- Gates: `go test ./services/billing/... -race`; `go vet ./...`; `go build ./...`; `gofmt -l`
  touched files. `promtool check rules infra/prometheus/alerts/billing.yaml` if available (CI runs
  it); otherwise YAML validity via the existing alerts test. No proto/sqlc/migration.

## Non-goals

- The pathological sustained-oldest-only-poison case (§1 residual) — implausible per the issue.
- No hard-attempts backstop (the window guard was chosen over window+ceiling).
- Not implementing the 4 dormant metrics (deleting the rules instead).
- §3(c) sentinel translation + §3(d) wording — skipped (unreachable / cosmetic).
- §4 `GRANT` — deploy runbook, covered by the broad grant script; belongs with #123.
- No change to `ClaimUnsentOutbox` ordering, batch size, interval, or the DLQ schema.

## Review weight

Billing reliability (money-event delivery + dead-letter correctness) → the concurrency/clock
reasoning matters. Senior per CLAUDE.md. Whole-branch review on the most capable model, with
attention to the broker-healthy guard's outage-vs-poison semantics and the new clock seam.
