#!/usr/bin/env bash
# scripts/db-reset.sh — Destructive: drop dev DB and rebuild from scratch.
#
# Drops the dev database, recreates it, applies all migrations, and loads the
# XYZ_CBA demo seed. Use only when you want a clean slate — all dev data
# (tenants, users, transactions, audit logs) is destroyed.
#
# Usage:
#   ./scripts/db-reset.sh

set -euo pipefail

DB_URL="${DATABASE_URL:-postgres://thittam:thittam_dev@localhost:5433/thittam?sslmode=disable}"

echo "==> Reset dev DB (DESTRUCTIVE)"
echo "    DB_URL: $DB_URL"

# Keep the role around — other DBs (e.g. thittam_test) may depend on it.
DROP_ROLE=false ./scripts/local-db-drop.sh
./scripts/local-db-init.sh
make migrate-all DB_URL="$DB_URL"
make seed DB_URL="$DB_URL"

echo "==> Reset complete — fresh dev DB with demo seed."
