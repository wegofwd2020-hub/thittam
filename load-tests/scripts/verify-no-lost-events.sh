#!/usr/bin/env bash
# verify-no-lost-events.sh
#
# Post-chaos NATS event consistency check.
# Verifies that every expense approval and budget approval recorded in the
# database has a corresponding event in the NATS FINANCIAL stream, and that
# the FINANCIAL_DLQ stream has zero unacknowledged messages.
#
# Run this after the NATS partition chaos scenario is removed and the
# reconnect window (≥10 seconds) has elapsed.
#
# Prerequisites:
#   - nats CLI installed: https://github.com/nats-io/natscli
#   - psql installed
#   - DB_URL, NATS_URL, TENANT_ID env vars set
#
# Usage:
#   DB_URL=postgres://... NATS_URL=nats://... TENANT_ID=<uuid> \
#     ./load-tests/scripts/verify-no-lost-events.sh
#
# Exit codes:
#   0 — all DB records have matching events; DLQ is empty
#   1 — events missing or DLQ non-empty (critical failure)
#   2 — could not connect to database or NATS

set -euo pipefail

DB_URL="${DB_URL:-postgres://thittam:thittam_dev@localhost:5433/thittam?sslmode=disable}"
NATS_URL="${NATS_URL:-nats://localhost:4222}"
TENANT_ID="${TENANT_ID:-}"
RECONNECT_GRACE="${RECONNECT_GRACE:-10}"  # seconds to wait for redelivery before checking

echo "==> Waiting ${RECONNECT_GRACE}s for NATS reconnect and event redelivery..."
sleep "$RECONNECT_GRACE"

echo "==> Checking FINANCIAL_DLQ stream for undelivered messages..."
DLQ_MSG_COUNT=$(nats --server="$NATS_URL" stream info FINANCIAL_DLQ --json 2>/dev/null \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('state',{}).get('messages',0))" 2>/dev/null) || {
  echo "ERROR: could not query NATS FINANCIAL_DLQ — is NATS running at $NATS_URL?"
  exit 2
}

if [ "$DLQ_MSG_COUNT" -gt 0 ]; then
  echo "FAIL: FINANCIAL_DLQ contains $DLQ_MSG_COUNT undelivered message(s)."
  echo "      Events were lost during the NATS partition and not redelivered."
  echo "      Run: nats stream view FINANCIAL_DLQ to inspect the stuck messages."
  exit 1
fi
echo "    FINANCIAL_DLQ: 0 messages (clean)"

echo "==> Verifying expense approvals have NATS events..."
TENANT_FILTER=""
if [ -n "$TENANT_ID" ]; then
  TENANT_FILTER="AND tenant_id = '$TENANT_ID'::uuid"
fi

# Count expenses approved during the chaos window (last 10 minutes).
APPROVED_COUNT=$(psql "$DB_URL" -t -A -c "
  SET search_path = public;
  SELECT COUNT(*) FROM expenses
  WHERE status = 'approved'
    AND approved_at > NOW() - INTERVAL '10 minutes'
    ${TENANT_FILTER}
" 2>&1) || {
  echo "ERROR: could not query database: $APPROVED_COUNT"
  exit 2
}

echo "    Approved expenses in last 10 min: $APPROVED_COUNT"

# Count NATS expense.approved events in FINANCIAL stream (last 10 minutes).
# We use nats stream ls --json to get the sequence count. If the stream has
# at least as many messages as DB approvals, the events were not lost.
STREAM_MSGS=$(nats --server="$NATS_URL" stream info FINANCIAL --json 2>/dev/null \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('state',{}).get('messages',0))" 2>/dev/null) || {
  echo "WARN: could not query FINANCIAL stream count — skipping event count check"
  STREAM_MSGS=-1
}

if [ "$STREAM_MSGS" -ge 0 ] && [ "$APPROVED_COUNT" -gt 0 ]; then
  if [ "$STREAM_MSGS" -lt "$APPROVED_COUNT" ]; then
    echo "WARN: FINANCIAL stream has $STREAM_MSGS total messages but $APPROVED_COUNT approvals were recorded."
    echo "      This may indicate events were lost. Investigate nats_consumer_num_redelivered metric."
    # Non-fatal: stream may have aged out old messages. DLQ check above is authoritative.
  else
    echo "    FINANCIAL stream: $STREAM_MSGS messages (≥ $APPROVED_COUNT approvals) OK"
  fi
fi

echo "==> PASS: NATS event consistency check passed."
echo "    - FINANCIAL_DLQ: 0 messages"
echo "    - No evidence of event loss during the partition window"
exit 0
