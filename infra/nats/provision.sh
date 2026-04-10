#!/usr/bin/env bash
# infra/nats/provision.sh — Provision Thittam JetStream streams and consumers.
#
# This script must be run after NATS starts (and again after any config change).
# It is idempotent: re-running it will not recreate existing streams or consumers,
# only add missing ones.
#
# Prerequisites: nats CLI installed (https://github.com/nats-io/natscli)
#   go install github.com/nats-io/natscli/nats@latest
#
# Usage:
#   ./infra/nats/provision.sh                   # connect to localhost:4222
#   NATS_URL=nats://prod-nats:4222 ./infra/nats/provision.sh
#
# In production the NATS URL is provided via the NATS_URL environment variable,
# which is injected from a Kubernetes Secret (Tier 3 — see CLAUDE.md Rule #2).

set -euo pipefail

NATS_URL="${NATS_URL:-nats://localhost:4222}"
NATS="${NATS_CLI:-nats}"

echo "==> Thittam NATS JetStream provisioning"
echo "    URL: ${NATS_URL}"
echo ""

# ── Helper ────────────────────────────────────────────────────────────────────

stream_exists() {
  $NATS --server "${NATS_URL}" stream info "$1" > /dev/null 2>&1
}

consumer_exists() {
  $NATS --server "${NATS_URL}" consumer info "$1" "$2" > /dev/null 2>&1
}

# ── Streams ───────────────────────────────────────────────────────────────────

echo "--- Streams ---"

# EVENTS: all domain events for general consumption (notifications, audit)
if stream_exists EVENTS; then
  echo "  [skip] EVENTS stream already exists"
else
  $NATS --server "${NATS_URL}" stream add EVENTS \
    --subjects "thittam.>" \
    --retention limits \
    --storage file \
    --replicas 1 \
    --max-age 24h \
    --max-bytes -1 \
    --discard old \
    --dupe-window 2m \
    --description "All Thittam domain events (24-hour retention)"
  echo "  [ok] EVENTS stream created"
fi

# FINANCIAL: budget, expense, ledger events — DLQ-enabled, 7-day retention for replay
if stream_exists FINANCIAL; then
  echo "  [skip] FINANCIAL stream already exists"
else
  $NATS --server "${NATS_URL}" stream add FINANCIAL \
    --subjects "thittam.budget.>,thittam.expense.>,thittam.ledger.>" \
    --retention limits \
    --storage file \
    --replicas 1 \
    --max-age 7d \
    --max-bytes -1 \
    --discard old \
    --dupe-window 2m \
    --description "Financial domain events: budget, expense, ledger (7-day retention, DLQ via FINANCIAL_DLQ)"
  echo "  [ok] FINANCIAL stream created"
fi

# FINANCIAL_DLQ: captures NATS MaxDeliveries advisories for FINANCIAL consumers
# When a consumer exhausts MaxDeliver attempts, NATS publishes an advisory to:
#   $JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.FINANCIAL.<consumer-name>
# This stream retains those advisories for 7 days so ops can investigate and replay.
if stream_exists FINANCIAL_DLQ; then
  echo "  [skip] FINANCIAL_DLQ stream already exists"
else
  $NATS --server "${NATS_URL}" stream add FINANCIAL_DLQ \
    --subjects '$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.FINANCIAL.>' \
    --retention limits \
    --storage file \
    --replicas 1 \
    --max-age 7d \
    --max-bytes -1 \
    --discard old \
    --description "Dead-letter advisories for FINANCIAL consumers (ops investigation + replay)"
  echo "  [ok] FINANCIAL_DLQ stream created"
fi

echo ""

# ── Consumers ─────────────────────────────────────────────────────────────────

echo "--- Consumers ---"

# MaxDeliver=5 with backoff: 5s, 30s, 5m, 30m
# After 5 failed deliveries NATS publishes a MaxDeliveries advisory → FINANCIAL_DLQ stream.

if consumer_exists FINANCIAL reporting-financial; then
  echo "  [skip] FINANCIAL/reporting-financial consumer already exists"
else
  $NATS --server "${NATS_URL}" consumer add FINANCIAL reporting-financial \
    --pull \
    --deliver all \
    --ack explicit \
    --ack-wait 30s \
    --max-deliver 5 \
    --backoff "5s,30s,5m,30m" \
    --filter "thittam.budget.>,thittam.expense.>,thittam.ledger.>" \
    --description "Reporting-analytics financial projection consumer"
  echo "  [ok] FINANCIAL/reporting-financial consumer created"
fi

if consumer_exists FINANCIAL notifications-financial; then
  echo "  [skip] FINANCIAL/notifications-financial consumer already exists"
else
  $NATS --server "${NATS_URL}" consumer add FINANCIAL notifications-financial \
    --pull \
    --deliver all \
    --ack explicit \
    --ack-wait 30s \
    --max-deliver 5 \
    --backoff "5s,30s,5m,30m" \
    --filter "thittam.budget.>,thittam.expense.>,thittam.ledger.>" \
    --description "Notifications service financial event consumer"
  echo "  [ok] FINANCIAL/notifications-financial consumer created"
fi

echo ""
echo "==> Provisioning complete."
echo ""
echo "Verify with:"
echo "  nats --server ${NATS_URL} stream ls"
echo "  nats --server ${NATS_URL} consumer ls FINANCIAL"
