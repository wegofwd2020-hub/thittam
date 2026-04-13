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

# EVENTS: non-financial domain events (24-hour retention, general consumers).
# Subjects are explicitly listed because JetStream forbids subject overlap
# between streams — the financial subjects below live on their own stream
# with longer retention.
if stream_exists EVENTS; then
  echo "  [skip] EVENTS stream already exists"
else
  $NATS --server "${NATS_URL}" stream add EVENTS \
    --defaults \
    --subjects "thittam.iam.>,thittam.notifications.>,thittam.document.>,thittam.project.>,thittam.inventory.>,thittam.reporting.>" \
    --retention limits \
    --storage file \
    --replicas 1 \
    --max-age 24h \
    --max-bytes=-1 \
    --discard old \
    --dupe-window 2m \
    --description "Non-financial Thittam domain events (24-hour retention)"
  echo "  [ok] EVENTS stream created"
fi

# FINANCIAL: budget, expense, ledger events — DLQ-enabled, 7-day retention for replay
if stream_exists FINANCIAL; then
  echo "  [skip] FINANCIAL stream already exists"
else
  $NATS --server "${NATS_URL}" stream add FINANCIAL \
    --defaults \
    --subjects "thittam.budget.>,thittam.expense.>,thittam.ledger.>,thittam.billing.>" \
    --retention limits \
    --storage file \
    --replicas 1 \
    --max-age 7d \
    --max-bytes=-1 \
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
    --defaults \
    --subjects '$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.FINANCIAL.>' \
    --retention limits \
    --storage file \
    --replicas 1 \
    --max-age 7d \
    --max-bytes=-1 \
    --discard old \
    --description "Dead-letter advisories for FINANCIAL consumers (ops investigation + replay)"
  echo "  [ok] FINANCIAL_DLQ stream created"
fi

echo ""

# ── Consumers ─────────────────────────────────────────────────────────────────

echo "--- Consumers ---"

# MaxDeliver=5 with backoff: 5s, 30s, 5m, 30m
# After 5 failed deliveries NATS publishes a MaxDeliveries advisory → FINANCIAL_DLQ stream.

# NOTE on --filter: pass once per subject. Newer nats CLI does NOT split a
# single comma-separated value — it stores the literal "a.>,b.>" as a single
# subject filter that no real message will ever match, so the consumer looks
# created but every subscribe gets "subject does not match consumer".
# NOTE on --no-headers-only: newer CLI defaults headers-only to true under
# some prompt paths; force off so message bodies are delivered.

if consumer_exists FINANCIAL reporting-financial; then
  echo "  [skip] FINANCIAL/reporting-financial consumer already exists"
else
  $NATS --server "${NATS_URL}" consumer add FINANCIAL reporting-financial \
    --pull \
    --deliver all \
    --ack explicit \
    --replay=instant \
    --wait 30s \
    --max-deliver 5 \
    --backoff=linear --backoff-min=5s --backoff-max=30m --backoff-steps=4 \
    --no-headers-only \
    --filter thittam.budget.\> \
    --filter thittam.expense.\> \
    --filter thittam.ledger.\> \
    --filter thittam.billing.\> \
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
    --replay=instant \
    --wait 30s \
    --max-deliver 5 \
    --backoff=linear --backoff-min=5s --backoff-max=30m --backoff-steps=4 \
    --no-headers-only \
    --filter thittam.budget.\> \
    --filter thittam.expense.\> \
    --filter thittam.ledger.\> \
    --filter thittam.billing.\> \
    --description "Notifications service financial event consumer"
  echo "  [ok] FINANCIAL/notifications-financial consumer created"
fi

# notifications-events: non-financial domain events (project, document).
# Financial events are intentionally excluded — they are already handled by
# notifications-financial on the FINANCIAL stream, which provides DLQ protection
# and 7-day retention. Duplicating them here would cause double notifications.
if consumer_exists EVENTS notifications-events; then
  echo "  [skip] EVENTS/notifications-events consumer already exists"
else
  $NATS --server "${NATS_URL}" consumer add EVENTS notifications-events \
    --pull \
    --deliver all \
    --ack explicit \
    --replay=instant \
    --wait 30s \
    --max-deliver 5 \
    --backoff=linear --backoff-min=5s --backoff-max=30m --backoff-steps=4 \
    --no-headers-only \
    --filter thittam.project.\> \
    --filter thittam.document.\> \
    --description "Notifications service non-financial event consumer (project, document)"
  echo "  [ok] EVENTS/notifications-events consumer created"
fi

echo ""
echo "==> Provisioning complete."
echo ""
echo "Verify with:"
echo "  nats --server ${NATS_URL} stream ls"
echo "  nats --server ${NATS_URL} consumer ls FINANCIAL"
echo "  nats --server ${NATS_URL} consumer ls EVENTS"
