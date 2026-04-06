#!/usr/bin/env bash
# Drops the thittam database and role from the local system PostgreSQL.
# Use before re-running local-db-init.sh for a completely clean slate.
#
# Usage:
#   ./scripts/local-db-drop.sh            # uses default port 5433
#   PG_PORT=5432 ./scripts/local-db-drop.sh
set -euo pipefail

PG_PORT=${PG_PORT:-5433}
DB_NAME=thittam
DB_USER=thittam

echo "==> Dropping database '$DB_NAME' on port $PG_PORT..."
sudo -u postgres psql -p "$PG_PORT" -c \
  "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='$DB_NAME';" \
  > /dev/null 2>&1 || true

sudo -u postgres psql -p "$PG_PORT" -c "DROP DATABASE IF EXISTS $DB_NAME;"
sudo -u postgres psql -p "$PG_PORT" -c "DROP ROLE IF EXISTS $DB_USER;"

echo "==> Dropped. Run 'make db-init' to recreate."
