#!/usr/bin/env bash
# scripts/db-bootstrap.sh — Idempotent dev DB bootstrap.
#
# Ensures the dev database exists, runs any pending migrations, and (with
# --with-seed) loads the demo seed. Safe to run multiple times — never
# destroys data.
#
# Usage:
#   ./scripts/db-bootstrap.sh                  # init + migrate
#   ./scripts/db-bootstrap.sh --with-seed      # init + migrate + seed
#
# Run this once per machine, and again after pulling new migrations.
# For a clean slate use db-reset.sh instead.

set -euo pipefail

WITH_SEED=false
for arg in "$@"; do
  case $arg in
    --with-seed) WITH_SEED=true ;;
    *) echo "Unknown flag: $arg"; echo "Usage: $0 [--with-seed]"; exit 1 ;;
  esac
done

DB_URL="${DATABASE_URL:-postgres://thittam:thittam_dev@localhost:5433/thittam?sslmode=disable}"

echo "==> Bootstrap dev DB"
echo "    DB_URL: $DB_URL"

echo "--> Ensuring role + database exist..."
./scripts/local-db-init.sh

echo "--> Applying migrations (no-op if already at head)..."
make migrate-all DB_URL="$DB_URL"

if [ "$WITH_SEED" = true ]; then
  echo "--> Loading XYZ_CBA demo seed..."
  make seed DB_URL="$DB_URL"
fi

echo "==> Bootstrap complete."
