#!/usr/bin/env bash
# scripts/db-test-reset.sh — Destructive: drop and recreate thittam_test.
#
# Use when integration test schema drift makes a clean rebuild simpler than
# fixing manually. Never touches the dev `thittam` database.
#
# Usage:
#   ./scripts/db-test-reset.sh

set -euo pipefail

DB_URL="${THITTAM_TEST_DSN:-postgres://thittam:thittam_dev@localhost:5433/thittam_test?sslmode=disable}"

echo "==> Reset test DB (DESTRUCTIVE — thittam_test only)"

DB_NAME=thittam_test DROP_ROLE=false ./scripts/local-db-drop.sh
DB_NAME=thittam_test ./scripts/local-db-init.sh
make migrate-all DB_URL="$DB_URL"

echo "==> Test DB rebuilt at migration head."
