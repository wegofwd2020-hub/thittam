#!/usr/bin/env bash
# scripts/db-test-bootstrap.sh — Idempotent test DB bootstrap.
#
# Ensures the thittam_test database exists and is at migration head. No seed —
# integration tests own their own fixtures via pkg/testdb (transactional
# rollback per test, so the DB schema is the only shared state).
#
# Usage:
#   ./scripts/db-test-bootstrap.sh

set -euo pipefail

DB_URL="${THITTAM_TEST_DSN:-postgres://thittam:thittam_dev@localhost:5433/thittam_test?sslmode=disable}"

echo "==> Bootstrap test DB"
echo "    DSN: $DB_URL"

DB_NAME=thittam_test ./scripts/local-db-init.sh
make migrate-all DB_URL="$DB_URL"

echo "==> Test DB ready. Run integration tests with:"
echo "    THITTAM_TEST_DSN=\"$DB_URL\" go test ./... -tags=integration"
