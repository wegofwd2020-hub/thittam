#!/usr/bin/env bash
# Creates the thittam role and database on the local system PostgreSQL.
#
# System PostgreSQL (Ubuntu) runs on port 5433 by default.
# This script is safe to re-run — it skips steps already done.
#
# Usage:
#   ./scripts/local-db-init.sh            # uses default port 5433
#   PG_PORT=5432 ./scripts/local-db-init.sh  # override if needed
set -euo pipefail

PG_PORT=${PG_PORT:-5433}
DB_NAME=${DB_NAME:-thittam}
DB_USER=${DB_USER:-thittam}
DB_PASS=${DB_PASS:-thittam_dev}
# Least-privilege runtime role (#120). Non-owner; receives only explicit grants
# via scripts/db-grant-app-role.sql. CREATE ROLE needs superuser, hence this
# privileged (sudo -u postgres) path.
DB_APP_USER=${DB_APP_USER:-thittam_app}
DB_APP_PASS=${DB_APP_PASS:-thittam_app_dev}

echo "==> System PostgreSQL on port $PG_PORT"

echo "--> Creating role '$DB_USER'..."
sudo -u postgres psql -p "$PG_PORT" -tc \
  "SELECT 1 FROM pg_roles WHERE rolname='$DB_USER'" | grep -q 1 \
  && echo "    Role already exists — skipping." \
  || sudo -u postgres psql -p "$PG_PORT" -c \
       "CREATE ROLE $DB_USER LOGIN PASSWORD '$DB_PASS';" \
  && echo "    Role created."

echo "--> Creating database '$DB_NAME'..."
sudo -u postgres psql -p "$PG_PORT" -tc \
  "SELECT 1 FROM pg_database WHERE datname='$DB_NAME'" | grep -q 1 \
  && echo "    Database already exists — skipping." \
  || sudo -u postgres psql -p "$PG_PORT" -c \
       "CREATE DATABASE $DB_NAME OWNER $DB_USER;" \
  && echo "    Database created."

echo "--> Creating app role '$DB_APP_USER'..."
sudo -u postgres psql -p "$PG_PORT" -tc \
  "SELECT 1 FROM pg_roles WHERE rolname='$DB_APP_USER'" | grep -q 1 \
  && echo "    App role already exists — skipping." \
  || sudo -u postgres psql -p "$PG_PORT" -c \
       "CREATE ROLE $DB_APP_USER LOGIN PASSWORD '$DB_APP_PASS';" \
  && echo "    App role created."

echo ""
echo "==> Done. Connection string:"
echo "    postgres://$DB_USER:$DB_PASS@localhost:$PG_PORT/$DB_NAME?sslmode=disable"
echo "    (app role: postgres://$DB_APP_USER:$DB_APP_PASS@localhost:$PG_PORT/$DB_NAME?sslmode=disable)"
