# NATS JetStream Dead-Letter Strategy — Financial Events

> **Audience:** On-call engineers, platform SRE  
> **Updated:** 2026-04-10  
> **Related:** `pkg/jetstream/config.go`, `infra/nats/provision.sh`, `infra/prometheus/alerts/nats-dlq.yml`

---

## Why this document exists

Financial events (`thittam.budget.*`, `thittam.expense.*`, `thittam.ledger.*`) flow
over NATS JetStream. If a consumer fails to process a financial event, silent message
loss means:

- The reporting-analytics service holds stale projection data (wrong dashboard totals).
- The general ledger is out of sync with the expense record.
- No alert fires — the system _looks_ healthy but isn't.

This document defines how failed financial event deliveries are captured, alerted on,
and replayed by ops.

---

## Stream topology

```
Publishers                     JetStream Streams                  Consumers
───────────                    ─────────────────                  ─────────
budget-planning  ──────────▶  FINANCIAL                ──────▶  reporting-financial
expense-tracking ──────────▶    thittam.budget.>                 notifications-financial
general-ledger   ──────────▶    thittam.expense.>
                                thittam.ledger.>
                                  MaxDeliver=5
                                  AckWait=30s
                                  BackOff=[5s,30s,5m,30m]
                                       │
                                       │ after 5th failed delivery
                                       ▼
                               FINANCIAL_DLQ                ──▶  Prometheus alert
                                 $JS.EVENT.ADVISORY.               (FinancialDLQNonEmpty)
                                 CONSUMER.MAX_DELIVERIES.
                                 FINANCIAL.*
                                  MaxAge=7d
```

---

## Dead-letter strategy

### How NATS dead-letter queuing works

NATS JetStream does not have a first-class dead-letter queue concept. Instead:

1. Each consumer has `MaxDeliver=5` and an exponential `BackOff` schedule.
2. When all delivery attempts are exhausted, NATS publishes a
   **MaxDeliveries advisory** to the subject:
   ```
   $JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.FINANCIAL.<consumer-name>
   ```
3. The `FINANCIAL_DLQ` stream captures these advisories (7-day retention).
4. The advisory body contains the `StreamSeq` of the original message,
   which is used to replay it (see Replay procedure below).

### Delivery schedule

| Attempt | Wait before retry | Cumulative wait |
|---------|-------------------|-----------------|
| 1       | immediate         | 0               |
| 2       | 5 s               | 5 s             |
| 3       | 30 s              | 35 s            |
| 4       | 5 min             | ~5.5 min        |
| 5       | 30 min            | ~35.5 min       |
| —       | **DLQ advisory**  | ~35.5 min       |

After ~35 minutes of failed delivery attempts, an advisory is placed in
`FINANCIAL_DLQ` and the `FinancialDLQNonEmpty` Prometheus alert fires.

### Consumer ack policy

- **AckWait:** 30 s — matches `ProjectionLagSLA` in `services/reporting/consumer.go`.
  If the consumer fails to ack within 30 s, the delivery counts as failed.
- **AckPolicy:** explicit — the consumer must ack each message individually.
- Domain errors (bad JSON, unknown event type) are **acked** to avoid redelivery
  loops. Infrastructure errors (DB unavailable) are **nacked** so the message
  is retried per the backoff schedule.

---

## Provisioning

Run once after NATS starts, and again after any config change. Idempotent.

```bash
# Local development
./infra/nats/provision.sh

# Production (URL injected from Kubernetes Secret)
NATS_URL=nats://prod-nats-svc:4222 ./infra/nats/provision.sh
```

Verify provisioning succeeded:

```bash
nats stream ls
nats consumer ls FINANCIAL
nats stream info FINANCIAL_DLQ
```

---

## Prometheus alerts

Three alerts are defined in `infra/prometheus/alerts/nats-dlq.yml`:

| Alert | Severity | Condition |
|-------|----------|-----------|
| `FinancialDLQNonEmpty` | critical | `FINANCIAL_DLQ` stream has ≥ 1 message |
| `FinancialConsumerHighRedeliveries` | warning | redelivery rate > 1/s for 5 min |
| `FinancialConsumerAckTimeout` | warning | any ack-wait exceeded on FINANCIAL |

---

## Ops runbook: investigating a DLQ alert

When `FinancialDLQNonEmpty` fires:

### Step 1 — Identify the failed messages

```bash
# List messages in FINANCIAL_DLQ
nats stream view FINANCIAL_DLQ

# Get details of the first DLQ advisory
nats stream get FINANCIAL_DLQ 1
```

The advisory body is JSON. Extract the key fields:

```json
{
  "stream": "FINANCIAL",
  "consumer": "reporting-financial",
  "consumer_seq": 42,
  "stream_seq": 137,        ← sequence number of the original message
  "deliveries": 5
}
```

### Step 2 — Read the original failed message

Using the `stream_seq` from the advisory:

```bash
nats stream get FINANCIAL 137
```

This shows the original event envelope. Decode the `data` field (base64 → JSON)
to read the event type, tenant ID, and payload.

### Step 3 — Diagnose the root cause

Check the consumer's service logs around the time of the first delivery attempt:

```bash
kubectl logs -n thittam deploy/reporting-analytics --since=1h | grep "event_id"
kubectl logs -n thittam deploy/notifications --since=1h | grep "event_id"
```

Common causes:

| Symptom | Likely cause | Action |
|---------|-------------|--------|
| `upsert expense_fact failed: connection refused` | DB was down | Verify DB is healthy; message will replay automatically once consumer reconnects |
| `unmarshal envelope: ...` | Bad JSON in message | Fix the publishing service; the message cannot be replayed as-is |
| `skip: bad amount` | Money field was `null` or non-string | Fix the publishing service; ack the advisory to clear the alert |
| Consumer pod crash-looping | OOM, panic, config error | Fix the pod; restart the consumer |

### Step 4 — Replay the original message

If the consumer is now healthy and the original message should be re-processed:

```bash
# Option A: re-publish the original message body
# (extract data from step 2 and re-publish to the original subject)
nats stream get FINANCIAL 137 --raw | \
  jq -r '.data' | base64 -d | \
  nats pub thittam.expense.approved

# Option B: reset the consumer delivery count to allow re-delivery
# WARNING: this replays from stream_seq, which may replay other messages too.
# Use only if the consumer has been idle and you want it to reprocess.
nats consumer edit FINANCIAL reporting-financial --max-deliver 5
```

### Step 5 — Clear the DLQ advisory

Once the root cause is resolved and the original message has been processed
(either by re-publishing or by re-delivery), delete the advisory from the DLQ:

```bash
# Delete a specific advisory by sequence number
nats stream rmm FINANCIAL_DLQ 1

# Delete all advisories (use with caution — only after all are resolved)
nats stream purge FINANCIAL_DLQ
```

### Step 6 — Verify the alert clears

```bash
nats stream info FINANCIAL_DLQ   # msgs: 0
```

The `FinancialDLQNonEmpty` alert resolves automatically once the stream is empty.

---

## Configuration reference

All stream and consumer configuration lives in `pkg/jetstream/config.go`.
`pkg/jetstream/config_test.go` enforces invariants (monotonic backoff, advisory
subject, 7-day retention, MaxDeliver within recommended range) so CI will fail
if the configuration is accidentally weakened.

To change the delivery policy, update `config.go` and re-run `provision.sh`.

---

## Linked diagrams

- **Event Schemas diagram** (Architecture Diagram #10): see
  `thittam_docs/docs/architecture/10-event-schemas.md`
- **Sequence Diagrams** (Architecture Diagram #7): the expense approval flow
  shows where `thittam.expense.approved` is published and consumed.
