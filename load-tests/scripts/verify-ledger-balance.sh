#!/usr/bin/env bash
# verify-ledger-balance.sh
#
# Post-load-test ledger consistency check.
# Queries the general-ledger database and asserts that every posted
# journal entry has equal total debit and total credit.
#
# A mismatch means the double-entry invariant was violated under load —
# this is a critical failure and must block the load test result.
#
# Usage:
#   DB_URL=postgres://... ./load-tests/scripts/verify-ledger-balance.sh
#   DB_URL=postgres://... TENANT_ID=<uuid> ./load-tests/scripts/verify-ledger-balance.sh
#
# Exit codes:
#   0 — all entries balance
#   1 — one or more entries do not balance (critical failure)
#   2 — could not connect to the database

set -euo pipefail

DB_URL="${DB_URL:-postgres://thittam:thittam_dev@localhost:5433/thittam?sslmode=disable}"
TENANT_ID="${TENANT_ID:-}"

echo "==> Verifying ledger balance post-load-test..."

# Query: find any posted journal entry where sum(debit) != sum(credit)
# Scoped to the load-test tenant if TENANT_ID is set.
TENANT_FILTER=""
if [ -n "$TENANT_ID" ]; then
  TENANT_FILTER="AND je.tenant_id = '$TENANT_ID'::uuid"
  echo "    Scoped to tenant: $TENANT_ID"
fi

SQL=$(cat <<EOF
SET search_path = public;
SELECT
  je.id,
  je.entry_number,
  SUM(jel.debit)  AS total_debit,
  SUM(jel.credit) AS total_credit,
  SUM(jel.debit) - SUM(jel.credit) AS imbalance
FROM journal_entries je
JOIN journal_entry_lines jel ON jel.journal_entry_id = je.id
WHERE je.status = 'posted'
  ${TENANT_FILTER}
GROUP BY je.id, je.entry_number
HAVING ABS(SUM(jel.debit) - SUM(jel.credit)) > 0.001
ORDER BY je.entry_number;
EOF
)

RESULT=$(psql "$DB_URL" -t -A -c "$SQL" 2>&1) || {
  echo "ERROR: could not connect to database: $RESULT"
  exit 2
}

if [ -z "$RESULT" ]; then
  echo "==> PASS: all posted journal entries balance. Double-entry invariant holds."
  exit 0
else
  echo "FAIL: imbalanced journal entries found:"
  echo "$RESULT"
  echo ""
  echo "CRITICAL: the double-entry invariant was violated under load."
  echo "Review the journal_entry_lines table for the above entry IDs."
  exit 1
fi
