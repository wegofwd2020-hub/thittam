# Load & Chaos Tests — Thittam

Load tests for the double-entry ledger, concurrent expense approvals, budget
lifecycle, and report generation. Chaos tests verify graceful degradation under
partial infrastructure failure.

---

## Prerequisites

```bash
# Install k6
curl -fsSL https://github.com/grafana/k6/releases/download/v0.51.0/k6-v0.51.0-linux-amd64.tar.gz \
  | tar -xz --strip-components=1 -C /usr/local/bin k6-v0.51.0-linux-amd64/k6

# Install psql (for ledger consistency check)
sudo apt-get install -y postgresql-client
```

---

## Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `BASE_URL` | Kong API Gateway root | `http://localhost:8000` |
| `TOKEN` | JWT access token (load-test user) | `eyJhb...` |
| `TENANT_ID` | Pre-provisioned load-test tenant UUID | `a1b2c3d4-...` |
| `PRODUCTION_ID` | Pre-existing production UUID | `b2c3d4e5-...` |
| `DEBIT_ACCOUNT_ID` | Account UUID for journal debit lines | `00000000-...` |
| `CREDIT_ACCOUNT_ID` | Account UUID for journal credit lines | `00000000-...` |
| `PERIOD_ID` | Open accounting period UUID | `00000000-...` |
| `DB_URL` | Direct Postgres URL (for ledger verifier) | `postgres://...` |

Create a `.env.load-test` file (gitignored) for local runs:

```bash
export BASE_URL=http://localhost:8000
export TOKEN=eyJhb...
export TENANT_ID=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
export PRODUCTION_ID=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
export DEBIT_ACCOUNT_ID=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
export CREDIT_ACCOUNT_ID=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
export PERIOD_ID=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
export DB_URL=postgres://thittam:thittam_dev@localhost:5433/thittam?sslmode=disable
```

---

## Running Individual Scenarios

```bash
source .env.load-test

# 100 concurrent expense submissions (~4 min)
k6 run load-tests/scenarios/expense-submissions.js

# 50 concurrent budget approvals across tenants (~4 min)
k6 run load-tests/scenarios/budget-approvals.js

# 1 000 ledger entries/min sustained for 10 minutes
k6 run load-tests/scenarios/ledger-throughput.js

# Report generation while write loads are active (~5 min)
k6 run load-tests/scenarios/report-under-load.js

# Full suite — all scenarios concurrently (~10 min)
k6 run load-tests/scenarios/full-suite.js
```

After the ledger throughput or full suite test, verify consistency:

```bash
./load-tests/scripts/verify-ledger-balance.sh
# Exit 0 = all entries balance. Exit 1 = double-entry invariant violated.
```

---

## Acceptance Criteria

| Scenario | Metric | Threshold |
|----------|--------|-----------|
| Expense submissions | p95 latency | < 500 ms |
| Expense submissions | p99 latency | < 1 000 ms |
| Budget approvals | p95 latency | < 400 ms |
| Budget approvals | p99 latency | < 800 ms |
| Ledger throughput | p95 latency | < 300 ms |
| Ledger throughput | p99 latency | < 600 ms |
| Ledger throughput | Throughput | ≥ 900 entries/min |
| Report generation | p95 latency | < 2 000 ms |
| Report generation | p99 latency | < 5 000 ms |
| All scenarios | Error rate | < 0.1% |
| Ledger (financial) | Error rate | < 0.01% |
| Post-load | Ledger balance | 0 imbalanced entries |

---

## Chaos Test Scenarios

### 1. general-ledger returns 503 for 30 seconds

```bash
kubectl apply -f infra/chaos/ledger-503.yaml
# Run expense approvals concurrently and observe behaviour
sleep 30
kubectl delete -f infra/chaos/ledger-503.yaml
```

**Expected:** Expense approvals degrade with 503 during the window. No 5xx
before or after. NATS DLQ has 0 messages after recovery.

### 2. NATS connection lost for 60 seconds

```bash
kubectl apply -f infra/chaos/nats-partition.yaml
# Run expense/budget/ledger operations concurrently
sleep 60
kubectl delete -f infra/chaos/nats-partition.yaml
# Wait 10s for reconnect, then check:
# nats stream info FINANCIAL_DLQ  (should be 0 messages)
```

**Expected:** HTTP 5xx rate < 0.1% during partition (NATS failure must not
surface as HTTP 500 to clients). All events delivered after reconnect.

### 3. PostgreSQL replica lag of 5 seconds

```bash
# Istio delay injection on read services:
kubectl apply -f infra/chaos/postgres-replica-lag.yaml
# Run read-path requests concurrently
sleep 120
kubectl delete -f infra/chaos/postgres-replica-lag.yaml

# Or simulate actual TCP lag on the replica pod:
kubectl exec -n thittam deploy/postgres-replica -- \
  tc qdisc add dev eth0 root netem delay 5000ms
# To remove:
kubectl exec -n thittam deploy/postgres-replica -- \
  tc qdisc del dev eth0 root
```

**Expected:** Read responses ≤ p99 6 000 ms (5 s lag + 1 s service).
Write operations unaffected (p99 < 500 ms).

---

## CI Schedule

Load tests run automatically every **Thursday at 20:30 UTC (02:00 IST Friday)**.

Results are uploaded as workflow artifacts (retained 90 days). Failures trigger
a Slack alert to `#platform-ops`.

To run manually: GitHub Actions → "Load & Chaos Tests" → "Run workflow".
