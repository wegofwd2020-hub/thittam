# Billing outbox saturation guard (#137) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the outbox dead-letter guard's in-batch broker-health signal (`published > 0`) with an out-of-batch one (a publish succeeded within `brokerHealthyWindow`), so an all-poison oldest batch stops starving healthy events; plus delete 4 unfirable alert rules and harden two tests.

**Architecture:** Task 1 is the substantive fix in `services/billing/outbox_relay.go` (add a `now` clock seam + `lastPublishSuccess` field, set it on publish, change the guard) with deterministic-clock tests. Task 2 bundles the independent cleanups: delete the dormant alerts (yaml) + two test-hardening minors.

**Tech Stack:** Go 1.25, `services/billing`, prometheus alert yaml, testify.

## Global Constraints

- **The guard:** `brokerHealthy := published > 0 || r.now().Sub(r.lastPublishSuccess) < brokerHealthyWindow`; dead-letter a held failure iff `brokerHealthy && f.event.Attempts >= maxOutboxAttempts`, else `RecordOutboxFailure`. During a genuine outage (no recent publish) the relay must NEVER dead-letter (retry). `lastPublishSuccess` set to `r.now()` on each successful publish; zero-value at startup = not-recently-healthy = don't dead-letter (conservative).
- **`brokerHealthyWindow = 5 * time.Minute`** — must exceed `maxOutboxAttempts*relayInterval` (75s). Documented trade-off: a 75s–window outage can park a row that reaches maxAttempts then (replayable via outbox-admin).
- **Clock seam:** add `now func() time.Time` to `Relay`, default `time.Now` in `NewRelay`; tests set `r.now` directly. `Relay` is single-goroutine (`Run` sequential) → `lastPublishSuccess` needs no mutex.
- **§2:** delete ONLY the 4 dormant rules + the `KNOWN DORMANT RULES` header; keep every rule whose metric the tree emits (all the `BillingOutbox*` rules).
- No proto/sqlc/migration/schema change; no change to `ClaimUnsentOutbox` ordering, batch size, or interval. §3(c)/(d)/§4 are out of scope (see spec non-goals).
- Gate each task: `go test ./services/billing/... -race && go vet ./... && go build ./...` + `gofmt -l <touched .go>` (billing files may carry pre-existing gofmt debt on main — diff-vs-main, add none new). Task 2's yaml: keep it promtool-valid (CI's rules check).
- **Security/reliability-sensitive** (money-event delivery) → senior review per CLAUDE.md.
- Commits Conventional-Commits (scope `billing`), ending `Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>`.

---

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `services/billing/outbox_relay.go` | `now` seam, `lastPublishSuccess`, `brokerHealthyWindow`, guard | 1 |
| `services/billing/outbox_relay_test.go` | saturation/outage tests + `TestRelay_MoveToDeadError` spy | 1,2 |
| `infra/prometheus/alerts/billing.yaml` | delete 4 dormant rules + header | 2 |
| `e2e/critical_path/billing_test.go` | `ListDeadOutbox` mock sorts `died_at DESC` | 2 |

---

### Task 1: Out-of-batch broker-health dead-letter guard

**Files:** Modify `services/billing/outbox_relay.go`, `services/billing/outbox_relay_test.go`

- [ ] **Step 1: Write failing tests (deterministic clock)**

In `services/billing/outbox_relay_test.go`, add (mirror `TestRelay_PoisonRow_DeadLetteredWhenSiblingSucceeds` for the mockRepo/pub setup; `mockRepo` lives in `service_test.go`, `fakeOutboxPublisher` in this file):
```go
func TestRelay_Saturation_DeadLettersWhenBrokerRecentlyHealthy(t *testing.T) {
	t.Parallel()
	// A batch of ONLY maxed-attempts poison rows — published==0 — but the broker
	// published successfully 1 minute ago (within the window). The rows are poison,
	// not an outage: dead-letter them so healthy events behind them can be reached.
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
	assert.Equal(t, 0, n)
	assert.True(t, moved[p1.ID] && moved[p2.ID], "recently-healthy broker => poison rows dead-lettered")
	assert.False(t, recorded, "dead-lettered, not merely recorded")
}

func TestRelay_Outage_DoesNotDeadLetter(t *testing.T) {
	t.Parallel()
	// Same all-poison batch, but no publish has succeeded within the window (outage):
	// must NOT dead-letter — parking the backlog during a NATS outage is the failure
	// this guard exists to avoid.
	p1 := &OutboxEvent{ID: uuid.New(), Subject: "s", TenantID: uuid.New(), Payload: []byte(`{}`), Attempts: maxOutboxAttempts}
	movedCalled := false
	recorded := map[uuid.UUID]bool{}
	repo := &mockRepo{
		claimUnsentOutboxFn:   func(_ context.Context, _ int) ([]*OutboxEvent, error) { return []*OutboxEvent{p1}, nil },
		moveOutboxToDeadFn:    func(_ context.Context, _ uuid.UUID, _ string) error { movedCalled = true; return nil },
		recordOutboxFailureFn: func(_ context.Context, id uuid.UUID, _ string) error { recorded[id] = true; return nil },
	}
	pub := &fakeOutboxPublisher{failFor: map[uuid.UUID]error{p1.TenantID: fmt.Errorf("nats down")}}
	r := NewRelay(repo, pub)
	fixed := time.Now()
	r.now = func() time.Time { return fixed }
	r.lastPublishSuccess = fixed.Add(-10 * time.Minute) // outside window => treat as outage

	n, err := r.processBatch(context.Background(), 10)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.False(t, movedCalled, "outage must never dead-letter")
	assert.True(t, recorded[p1.ID], "outage => record failure for retry")
}

func TestRelay_PublishSuccess_UpdatesLastPublishSuccess(t *testing.T) {
	t.Parallel()
	// A successful publish advances lastPublishSuccess to now (the signal the guard reads).
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
```
Also revise the existing `TestRelay_TotalBatchFailure_NeverDeadLetters` — it currently asserts the *bug* (all-fail never dead-letters) with the ambient wall-clock. Make it explicit outage semantics: set `r.now` fixed and `r.lastPublishSuccess = fixed.Add(-10*time.Minute)` (or leave zero), keeping the assertion "never dead-letters" — now correct as the *outage* case, not the bug.

- [ ] **Step 2: Run — expect FAIL** (`r.now`/`lastPublishSuccess` fields don't exist; saturation test expects dead-letter that today's `published>0` guard won't do): `go test ./services/billing/ -run 'TestRelay_Saturation|TestRelay_Outage|TestRelay_PublishSuccess_Updates'`

- [ ] **Step 3: Implement**

In `services/billing/outbox_relay.go`: add the `now func() time.Time` + `lastPublishSuccess time.Time` fields to `Relay` (with the doc comment from the spec); default `now: time.Now` in `NewRelay`; add the `brokerHealthyWindow = 5 * time.Minute` const (with the spec's comment) near `maxOutboxAttempts`. In `processBatch`, after `outboxPublished.Inc(); published++` add `r.lastPublishSuccess = r.now()`. Replace `canDeadLetter := published > 0` with:
```go
	brokerHealthy := published > 0 || r.now().Sub(r.lastPublishSuccess) < brokerHealthyWindow
```
and change the guard use to `if brokerHealthy && f.event.Attempts >= maxOutboxAttempts {`.

- [ ] **Step 4: Run — expect PASS + gate**
```bash
go test ./services/billing/ -race && go vet ./... && go build ./...
gofmt -l services/billing/outbox_relay.go services/billing/outbox_relay_test.go
```
All green; the pre-existing poison-row/under-cap/move-error tests still pass.

- [ ] **Step 5: Commit**
```bash
git add services/billing/outbox_relay.go services/billing/outbox_relay_test.go
git commit -m "fix(billing): dead-letter poison on out-of-batch broker health, not in-batch (#137)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Delete dormant alerts + test-hardening minors

**Files:** Modify `infra/prometheus/alerts/billing.yaml`, `e2e/critical_path/billing_test.go`, `services/billing/outbox_relay_test.go`

- [ ] **Step 1: Delete the 4 dormant alert rules**

In `infra/prometheus/alerts/billing.yaml`, remove the `BillingDunningFailureSpike`,
`BillingSuspendedSubscriptionsHigh`, `BillingPlanLimitRejectionsHigh`,
`BillingServiceErrorRateElevated` rule blocks and the `KNOWN DORMANT RULES` header comment
(:21-29). Keep all `BillingOutbox*` rules and any other rule whose metric this tree emits. Ensure
the file stays valid YAML / promtool-parseable (no dangling anchors, group still non-empty).

- [ ] **Step 2: Harden `TestRelay_MoveToDeadError_TickContinues`**

In `services/billing/outbox_relay_test.go`, add a `recordOutboxFailureFn` to that test's `mockRepo`
that fails the test if invoked, pinning that a `MoveOutboxToDead` error does NOT fall through to
`RecordOutboxFailure`:
```go
		recordOutboxFailureFn: func(_ context.Context, _ uuid.UUID, _ string) error {
			t.Fatal("MoveOutboxToDead failure must not fall through to RecordOutboxFailure")
			return nil
		},
```

- [ ] **Step 3: Fix the e2e `ListDeadOutbox` mock ordering**

In `e2e/critical_path/billing_test.go`, make `billingRepo.ListDeadOutbox` return most-recently-died
first (mirror the real `ORDER BY died_at DESC`). Record a `diedAt time.Time` per entry in
`MoveOutboxToDead` (or, if the fake has no clock, reverse the insertion slice as a documented
proxy) and return the slice sorted `died_at DESC` before applying `limit`.

- [ ] **Step 4: Run — expect PASS + gate**
```bash
go test ./services/billing/ ./e2e/critical_path/ -race 2>&1 | tail -20   # e2e may skip without infra; compile is what matters
go vet ./... && go build ./...
gofmt -l e2e/critical_path/billing_test.go services/billing/outbox_relay_test.go
```
(e2e tests may `//go:build`-gate or skip without infra; the point is they compile + the billing unit tests pass. If a `promtool`/alerts-rules test exists, run it.)

- [ ] **Step 5: Commit**
```bash
git add infra/prometheus/alerts/billing.yaml e2e/critical_path/billing_test.go services/billing/outbox_relay_test.go
git commit -m "chore(billing): delete unfirable alert rules; harden DLQ ordering + no-double-record tests (#137)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage:** §1 out-of-batch guard (field+seam+window+guard) + tests (saturation dead-letters, outage doesn't, publish updates signal, revised total-batch-failure) → Task 1 ✅. §2 delete 4 dormant alerts → Task 2 ✅. §3(a) e2e ordering + §3(b) no-double-record pin → Task 2 ✅. Non-goals honored (no hard-cap, no metrics impl, §3c/d + §4 out) ✅.

**Placeholder scan:** full guard code + struct + tests given; the e2e ordering step describes the exact transform (record diedAt / sort DESC). Not placeholders.

**Type consistency:** `now func() time.Time` + `lastPublishSuccess time.Time` on `Relay`; `brokerHealthyWindow` a `time.Duration` const; guard reads `r.now()`/`r.lastPublishSuccess`. Tests set `r.now`/`r.lastPublishSuccess` directly (same package). `mockRepo` `*Fn` fields match existing (claimUnsentOutboxFn/moveOutboxToDeadFn/recordOutboxFailureFn/markOutboxSentFn).

**Ordering:** Task 1 (the guard — self-contained in outbox_relay.go + its test) → Task 2 (independent yaml + test-only cleanups). Each commit builds + passes its gate.
