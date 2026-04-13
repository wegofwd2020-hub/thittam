#!/usr/bin/env bash
# scripts/dev-start.sh — Start the full Thittam local development stack.
#
# Startup order:
#   1. Prerequisite check
#   2. Docker middleware (Redis, NATS, MinIO)
#   3. NATS JetStream stream + consumer provisioning
#   4. Database verification (reachable + at migration head — read-only check)
#   5. IAM service     (all other services validate tokens against IAM)
#   6. Core services   (started in parallel after IAM is ready)
#   7. Auxiliary services (notifications, document, billing)
#
# DB lifecycle is decoupled — this script never mutates the database.
# Use these targets explicitly:
#   make db-bootstrap               # idempotent: ensure DB exists + migrate
#   make db-bootstrap WITH_SEED=1   # also load XYZ_CBA seed
#   make db-reset                   # destructive: drop + rebuild + seed
#
# Usage:
#   ./scripts/dev-start.sh              # normal start (DB must already be bootstrapped)
#   ./scripts/dev-start.sh --svc-only   # skip infra check; start services only
#
# Logs: logs/<service>.log  (created in repo root; gitignored)
# PIDs: /tmp/thittam-dev.pids
#
# Stop all services: ./scripts/dev-stop.sh
#         or:        Ctrl+C in this terminal

set -euo pipefail

# ── Configuration ─────────────────────────────────────────────────────────────

DB_URL="${DATABASE_URL:-postgres://thittam:thittam_dev@localhost:5433/thittam?sslmode=disable}"
REDIS_URL="${REDIS_URL:-localhost:6380}"
NATS_URL="${NATS_URL:-nats://localhost:4222}"
IAM_KEY_DIR="${IAM_KEY_DIR:-./keys}"
INFRA_FILE="infra/local/docker-compose.infra.yml"
PID_FILE="/tmp/thittam-dev.pids"
LOG_DIR="logs"

# Parse flags
SVC_ONLY=false
for arg in "$@"; do
  case $arg in
    --svc-only) SVC_ONLY=true ;;
    --fresh|--no-seed)
      echo "ERROR: '$arg' is no longer supported by dev-start.sh."
      echo "DB lifecycle is now separate. Use:"
      echo "  make db-reset       # destructive rebuild + seed"
      echo "  make db-bootstrap   # idempotent ensure-and-migrate"
      exit 1
      ;;
    *)
      echo "Unknown flag: $arg"
      echo "Usage: $0 [--svc-only]"
      exit 1
      ;;
  esac
done

# Source shared DB helpers (check_db_reachable, check_migration_head).
# shellcheck disable=SC1091
source ./scripts/_db-common.sh

# ── Colours ───────────────────────────────────────────────────────────────────

if [ -t 1 ]; then
  RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
  CYAN='\033[0;36m'; BOLD='\033[1m'; RESET='\033[0m'
else
  RED=''; GREEN=''; YELLOW=''; CYAN=''; BOLD=''; RESET=''
fi

header()  { echo -e "\n${BOLD}${CYAN}══ $1 ══${RESET}"; }
ok()      { echo -e "  ${GREEN}✓${RESET} $1"; }
warn()    { echo -e "  ${YELLOW}⚠${RESET}  $1"; }
fail()    { echo -e "  ${RED}✗${RESET} $1"; }
step()    { echo -e "  ${CYAN}→${RESET} $1"; }

# ── Cleanup on exit ───────────────────────────────────────────────────────────

PIDS=()

cleanup() {
  echo ""
  header "Shutting down services"
  if [ ${#PIDS[@]} -gt 0 ]; then
    for pid in "${PIDS[@]}"; do
      if kill -0 "$pid" 2>/dev/null; then
        kill "$pid" 2>/dev/null && ok "Stopped PID $pid" || true
      fi
    done
  fi
  # Also kill anything recorded in the PID file (handles re-runs)
  if [ -f "$PID_FILE" ]; then
    while IFS= read -r pid; do
      if kill -0 "$pid" 2>/dev/null; then
        kill "$pid" 2>/dev/null || true
      fi
    done < "$PID_FILE"
    rm -f "$PID_FILE"
  fi
  echo ""
}

trap cleanup EXIT INT TERM

# ── Helper: wait for HTTP endpoint ───────────────────────────────────────────

wait_http() {
  local name="$1" url="$2" retries="${3:-30}" interval="${4:-2}"
  local i=0
  while [ $i -lt $retries ]; do
    if curl -sf "$url" > /dev/null 2>&1; then
      ok "$name is ready"
      return 0
    fi
    i=$((i + 1))
    printf "    waiting for %s (%d/%d)...\r" "$name" "$i" "$retries"
    sleep "$interval"
  done
  fail "$name did not become ready after $((retries * interval))s"
  return 1
}

# ── Helper: start a service ───────────────────────────────────────────────────
#
# start_svc <name> <cmd_path> [extra_env_key=val ...]
# Sets DATABASE_URL, REDIS_URL, NATS_URL, IAM_KEY_DIR automatically.
# Extra env pairs (KEY=value) are passed after the cmd path.

start_svc() {
  local name="$1"; shift
  local cmd_path="$1"; shift
  local log_file="$LOG_DIR/${name}.log"

  # Build env — start from the current environment, add/override standard vars
  local env_overrides=(
    "DATABASE_URL=${DB_URL}"
    "REDIS_URL=${REDIS_URL}"
    "NATS_URL=${NATS_URL}"
    "IAM_KEY_DIR=${IAM_KEY_DIR}"
  )
  # Append any caller-supplied extras
  for extra in "$@"; do
    env_overrides+=("$extra")
  done

  step "Starting ${name}  →  ${log_file}"
  env "${env_overrides[@]}" go run "./${cmd_path}" >> "$log_file" 2>&1 &
  local pid=$!
  PIDS+=("$pid")
  echo "$pid" >> "$PID_FILE"
  echo -e "    PID ${pid}"
}

# ── 0. Resolve repo root ──────────────────────────────────────────────────────

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

echo -e "${BOLD}Thittam — Local Dev Stack${RESET}"
echo "  Repo:  ${REPO_ROOT}"
echo "  DB:    ${DB_URL}"
echo "  Redis: ${REDIS_URL}"
echo "  NATS:  ${NATS_URL}"
[ "$SVC_ONLY" = true ] && echo -e "  ${YELLOW}Mode: --svc-only (skip infra checks)${RESET}"

mkdir -p "$LOG_DIR"
> "$PID_FILE"   # truncate on fresh start

# ── 1. Prerequisite check ─────────────────────────────────────────────────────

header "Prerequisites"

check_cmd() {
  if command -v "$1" &>/dev/null; then
    ok "$1"
  else
    fail "$1 not found — $2"
    return 1
  fi
}

PREREQ_OK=true
check_cmd docker    "install Docker Desktop"                         || PREREQ_OK=false
check_cmd go        "install Go 1.22+ from https://go.dev/dl"        || PREREQ_OK=false
check_cmd migrate   "go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest" || PREREQ_OK=false
check_cmd psql      "sudo apt install postgresql-client"             || PREREQ_OK=false
check_cmd nats      "go install github.com/nats-io/natscli/nats@latest" || PREREQ_OK=false

if [ "$PREREQ_OK" = false ]; then
  fail "One or more prerequisites are missing. Install them and retry."
  exit 1
fi

# Warn about missing tmuxinator (optional — only needed for 'make run-all')
command -v tmuxinator &>/dev/null || warn "tmuxinator not found (not needed for this script — only for 'make run-all')"

# ── 2. Dev keys ───────────────────────────────────────────────────────────────

header "Dev Keys"

if [ -f "keys/jwt_private.pem" ] && [ -f "keys/oidc_encryption.key" ]; then
  ok "keys/jwt_private.pem exists"
  ok "keys/oidc_encryption.key exists"
else
  step "Generating dev keys via 'make dev-keys'..."
  make dev-keys
  ok "Keys generated"
fi

# ── 3. Infrastructure ─────────────────────────────────────────────────────────

if [ "$SVC_ONLY" = false ]; then

  header "Infrastructure (Docker)"

  step "Starting Redis, NATS, MinIO..."
  docker compose -f "$INFRA_FILE" up -d --quiet-pull 2>&1 | grep -v "^$" || true

  wait_http "Redis"     "http://localhost:6380" 15 2 || \
    { warn "Redis health endpoint not available — checking via redis-cli..."; \
      redis-cli -p 6380 ping | grep -q PONG && ok "Redis is up" || { fail "Redis did not start"; exit 1; }; }

  wait_http "NATS"      "http://localhost:8222/healthz" 15 2
  wait_http "MinIO"     "http://localhost:9000/minio/health/live" 20 2

  ok "All middleware containers healthy"

  # ── 4. NATS JetStream provisioning ──────────────────────────────────────────

  header "NATS JetStream"
  step "Provisioning streams and consumers..."
  ./infra/nats/provision.sh
  ok "NATS streams provisioned"

  # ── 5. Database (read-only verification) ────────────────────────────────────
  #
  # dev-start never mutates the database. We verify the DB is reachable and
  # at expected migration head; if not, we tell the developer exactly which
  # command to run, then exit.

  header "Database (verification only)"

  if ! check_db_reachable "$DB_URL"; then
    fail "Cannot connect to $DB_URL"
    echo ""
    echo "  Run this once to bootstrap the dev database:"
    echo -e "    ${BOLD}make db-bootstrap WITH_SEED=1${RESET}"
    exit 1
  fi
  ok "Database reachable"

  step "Checking migration head..."
  if ! check_migration_head "$DB_URL"; then
    fail "Pending migrations detected (drift listed above)"
    echo ""
    echo "  Apply them with:"
    echo -e "    ${BOLD}make db-bootstrap${RESET}"
    exit 1
  fi
  ok "All migration directories at head"

fi  # end of SVC_ONLY skip block

# ── 8. Services ───────────────────────────────────────────────────────────────

header "Starting Services"
echo ""
echo -e "  Service logs are written to ${CYAN}${LOG_DIR}/<name>.log${RESET}"
echo ""

# IAM must start first — every other service validates tokens against it.
# Wait for its /readyz before launching the rest.

start_svc "iam" "cmd/iam"
step "Waiting for IAM to be ready (port 9096)..."
wait_http "IAM (/readyz)" "http://localhost:9096/readyz" 40 2

# Core transaction services — start in parallel.
# These do not depend on each other at startup time.

start_svc "project-management"  "cmd/project-management"
start_svc "budget-planning"     "cmd/budget-planning"
start_svc "expense-tracking"    "cmd/expense-tracking"
start_svc "general-ledger"      "cmd/general-ledger"
start_svc "inventory-management" "cmd/inventory-management"

# Wait for the core group to be healthy before starting reporting
# (reporting-analytics makes read-only gRPC calls to project, budget, expense, ledger).

step "Waiting for core services..."
wait_http "project-management  (/healthz)" "http://localhost:9090/healthz" 30 2
wait_http "budget-planning     (/healthz)" "http://localhost:9091/healthz" 30 2
wait_http "expense-tracking    (/healthz)" "http://localhost:9092/healthz" 30 2
wait_http "general-ledger      (/healthz)" "http://localhost:9093/healthz" 30 2
wait_http "inventory-management(/healthz)" "http://localhost:9094/healthz" 30 2

# Auxiliary services — depend on core services being up.

start_svc "reporting-analytics" "cmd/reporting-analytics"
start_svc "notifications"       "cmd/notifications"
start_svc "document"            "cmd/document"
start_svc "billing"             "cmd/billing" \
  "DOCUMENT_SERVICE_ADDR=localhost:8088"

step "Waiting for auxiliary services..."
wait_http "reporting-analytics(/healthz)" "http://localhost:9095/healthz" 30 2
wait_http "notifications       (/healthz)" "http://localhost:9097/healthz" 30 2
wait_http "document            (/healthz)" "http://localhost:9098/healthz" 30 2
wait_http "billing             (/healthz)" "http://localhost:9099/healthz" 30 2

# ── Ready ─────────────────────────────────────────────────────────────────────

echo ""
echo -e "${BOLD}${GREEN}══════════════════════════════════════════${RESET}"
echo -e "${BOLD}${GREEN}  Thittam is ready for testing            ${RESET}"
echo -e "${BOLD}${GREEN}══════════════════════════════════════════${RESET}"
echo ""
echo -e "  ${BOLD}Service endpoints (gRPC):${RESET}"
echo "    project-management   :8080     budget-planning      :8081"
echo "    expense-tracking     :8082     general-ledger       :8083"
echo "    inventory-management :8084     reporting-analytics  :8085"
echo "    iam                  :8086     notifications        :8087"
echo "    document             :8088     billing              :8089"
echo ""
echo -e "  ${BOLD}Metrics / health:${RESET}"
echo "    http://localhost:9090-9099/healthz    (one per service)"
echo "    http://localhost:9300                 (Prometheus UI)"
echo ""
echo -e "  ${BOLD}Infrastructure:${RESET}"
echo "    MinIO console:  http://localhost:9001   (thittam / thittam_dev)"
echo "    NATS monitor:   http://localhost:8222"
echo "    Redis:          localhost:6380"
echo ""
echo -e "  ${BOLD}Demo tenant:${RESET}  d0000000-0000-0000-0000-000000000001  (XYZ_CBA Productions)"
echo ""
echo -e "  ${BOLD}Demo accounts${RESET} (password: demo1234):"
echo "    rajesh.kumar@xyzcba.com    super_admin"
echo "    priya.sharma@xyzcba.com    manager"
echo "    arun.nair@xyzcba.com       coordinator"
echo "    meena.iyer@xyzcba.com      accountant"
echo "    vikram.reddy@xyzcba.com    project_supervisor"
echo "    deepa.menon@xyzcba.com     member"
echo ""
echo -e "  ${BOLD}Quick login:${RESET}"
echo "    TOKEN=\$(curl -s -X POST http://localhost:8086/auth/login \\"
echo "      -H 'Content-Type: application/json' \\"
echo "      -H 'X-Tenant-ID: d0000000-0000-0000-0000-000000000001' \\"
echo "      -d '{\"email\":\"rajesh.kumar@xyzcba.com\",\"password\":\"demo1234\"}' \\"
echo "      | jq -r '.token')"
echo ""
echo -e "  ${CYAN}Logs:${RESET}  tail -f logs/iam.log  (or any other service name)"
echo -e "  ${CYAN}Stop:${RESET}  Ctrl+C  or  ./scripts/dev-stop.sh"
echo ""

# ── Keep running ──────────────────────────────────────────────────────────────
# Block until Ctrl+C or a service exits unexpectedly.

echo -e "  ${YELLOW}All services running. Press Ctrl+C to stop.${RESET}"
echo ""

# Watch for any service process dying unexpectedly.
while true; do
  for pid in "${PIDS[@]}"; do
    if ! kill -0 "$pid" 2>/dev/null; then
      fail "A service process (PID ${pid}) exited unexpectedly."
      warn "Check the log in ${LOG_DIR}/ for details."
      # Don't exit immediately — let the operator inspect; they can Ctrl+C.
    fi
  done
  sleep 5
done
