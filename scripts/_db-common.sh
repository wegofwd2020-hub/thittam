#!/usr/bin/env bash
# scripts/_db-common.sh — shared helpers for DB lifecycle scripts.
# Sourced from db-bootstrap.sh, db-reset.sh, dev-start.sh, etc.

# All migration directories that the app expects to be at head.
MIGRATION_DIRS=(
  shared
  iam
  project
  budget
  expense
  ledger
  inventory
  document
  notifications
  billing
)

# expected_version <dir>
# Echoes the highest migration sequence number in migrations/<dir>/.
# Returns 0 if no migrations exist (empty directory).
expected_version() {
  local dir="$1"
  local max=0
  shopt -s nullglob
  for f in "migrations/$dir"/*.up.sql; do
    local n
    n=$(basename "$f" | sed -E 's/^([0-9]+)_.*/\1/')
    # Strip leading zeros without invoking arithmetic on octal.
    n=$((10#$n))
    if [ "$n" -gt "$max" ]; then max=$n; fi
  done
  shopt -u nullglob
  echo "$max"
}

# applied_version <dir> <db_url>
# Echoes the migrate-applied version, or 0 if nothing applied.
# Honours the per-directory schema_migrations_<dir> table convention used
# by `make migrate-all`.
applied_version() {
  local dir="$1"
  local url="$2"
  local out
  out=$(migrate -path "migrations/$dir" \
                -database "${url}&x-migrations-table=schema_migrations_${dir}" \
                version 2>&1 || true)
  # `migrate version` prints "<version>" or "no migration" or "error: ...".
  if echo "$out" | grep -qE "no migration"; then
    echo 0
  elif echo "$out" | grep -qiE "error|unable"; then
    # Bubble the error up via stderr so callers can decide.
    echo "$out" >&2
    return 1
  else
    # Accept lines like "12" or "12 (dirty)".
    echo "$out" | awk '{print $1}'
  fi
}

# check_db_reachable <db_url>
# Returns 0 if a SELECT 1 succeeds, 1 otherwise.
check_db_reachable() {
  local url="$1"
  PGCONNECT_TIMEOUT=2 psql "$url" -tc "SELECT 1" >/dev/null 2>&1
}

# check_migration_head <db_url>
# Verifies every migration directory is at expected head. Prints one line per
# divergent directory and returns non-zero if any drift is found.
check_migration_head() {
  local url="$1"
  local drift=0
  for dir in "${MIGRATION_DIRS[@]}"; do
    local exp app
    exp=$(expected_version "$dir")
    app=$(applied_version "$dir" "$url") || return 2
    if [ "$exp" != "$app" ]; then
      echo "  ✗ migrations/$dir: applied=$app expected=$exp"
      drift=1
    fi
  done
  return $drift
}
