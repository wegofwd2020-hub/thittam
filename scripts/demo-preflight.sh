#!/usr/bin/env bash
# scripts/demo-preflight.sh — sanity-check the local dev stack 15 minutes
# before a demo. Validates that every piece a typical "happy-path" demo
# touches is up, responsive, and has data loaded. Exits 0 only if every
# required check passes; non-zero on the first blocker.
#
# Usage:
#   ./scripts/demo-preflight.sh                   # check everything
#   ./scripts/demo-preflight.sh --verbose         # print raw responses
#   DEMO_SKIP_LOGIN=1 ./scripts/demo-preflight.sh # skip login (if auth is flaky)
#
# Optional checks are tagged [soft] — they emit a warning but do NOT fail
# the run. Required checks are unlabelled.
#
# Exit codes:
#   0   — every required check passed
#   1   — at least one required check failed (see the first ❌)
#   2   — the script itself couldn't start (missing curl / jq)

set -uo pipefail    # NB: no -e. We want to continue past a failing check
                     # to give the operator the full picture before exiting.

# ── Configuration ─────────────────────────────────────────────────────────────

DB_URL="${DATABASE_URL:-postgres://thittam:thittam_dev@localhost:5433/thittam?sslmode=disable}"
REDIS_HOST_PORT="${REDIS_URL:-localhost:6380}"
NATS_URL="${NATS_URL:-nats://localhost:4222}"

IAM_HTTP="${IAM_HTTP:-http://localhost:8086}"
PROJECT_HTTP="${PROJECT_HTTP:-http://localhost:8080}"
BUDGET_HTTP="${BUDGET_HTTP:-http://localhost:8081}"
EXPENSE_HTTP="${EXPENSE_HTTP:-http://localhost:8082}"
LEDGER_HTTP="${LEDGER_HTTP:-http://localhost:8083}"
INVENTORY_HTTP="${INVENTORY_HTTP:-http://localhost:8084}"
REPORTING_HTTP="${REPORTING_HTTP:-http://localhost:8085}"
NOTIFICATIONS_HTTP="${NOTIFICATIONS_HTTP:-http://localhost:8087}"
DOCUMENT_HTTP="${DOCUMENT_HTTP:-http://localhost:8088}"

# grpc-gateway REST ports (what the web client actually calls)
IAM_GW="${IAM_GW:-http://localhost:9086}"
PROJECT_GW="${PROJECT_GW:-http://localhost:9080}"
BUDGET_GW="${BUDGET_GW:-http://localhost:9081}"

WEB_URL="${WEB_URL:-http://localhost:3100}"

CBA_EMAIL="${DEMO_CBA_EMAIL:-rajesh.kumar@xyzcba.com}"
CBA_PASSWORD="${DEMO_CBA_PASSWORD:-demo1234}"
CONSTRUCTION_EMAIL="${DEMO_CONSTRUCTION_EMAIL:-miles.sullivan@xyzconstruction.com}"
CONSTRUCTION_PASSWORD="${DEMO_CONSTRUCTION_PASSWORD:-demo1234}"

VERBOSE=0
[[ "${1:-}" == "--verbose" ]] && VERBOSE=1

# ── Output helpers ────────────────────────────────────────────────────────────

C_RESET='\033[0m'
C_GREEN='\033[0;32m'
C_RED='\033[0;31m'
C_YELLOW='\033[0;33m'
C_BLUE='\033[0;34m'
C_DIM='\033[0;90m'

failures=0
warnings=0

pass() { printf "  ${C_GREEN}✅${C_RESET} %s\n" "$1"; }
fail() { printf "  ${C_RED}❌${C_RESET} %s\n" "$1"; failures=$((failures + 1)); }
warn() { printf "  ${C_YELLOW}⚠️ ${C_RESET} %s ${C_DIM}(soft check)${C_RESET}\n" "$1"; warnings=$((warnings + 1)); }
info() { printf "  ${C_DIM}%s${C_RESET}\n" "$1"; }
section() { printf "\n${C_BLUE}━━━ %s ━━━${C_RESET}\n" "$1"; }

# ── Tool prerequisites ────────────────────────────────────────────────────────

section "Prerequisites"
for tool in curl jq psql nc; do
  if command -v "$tool" >/dev/null 2>&1; then
    pass "$tool installed"
  else
    fail "$tool not installed"
    # Prereqs are fatal — bail early with a distinct code.
    exit 2
  fi
done

# ── Infra ─────────────────────────────────────────────────────────────────────

section "Infrastructure"

# Postgres
if psql "$DB_URL" -c 'SELECT 1;' >/dev/null 2>&1; then
  pass "Postgres reachable at $DB_URL"
else
  fail "Postgres NOT reachable — is Docker up? Try: make db-bootstrap"
fi

# Redis — simple TCP port check
if nc -z ${REDIS_HOST_PORT/:/ } 2>/dev/null; then
  pass "Redis reachable at $REDIS_HOST_PORT"
else
  fail "Redis NOT reachable — is Docker up?"
fi

# NATS
if nc -z ${NATS_URL#nats://} 2>/dev/null | head -c 0; then : ; fi
NATS_HOST_PORT="${NATS_URL#nats://}"
if nc -z ${NATS_HOST_PORT/:/ } 2>/dev/null; then
  pass "NATS reachable at $NATS_URL"
else
  fail "NATS NOT reachable — is Docker up?"
fi

# ── Services (liveness) ───────────────────────────────────────────────────────

check_readyz() {
  local name="$1" url="$2" soft="${3:-}"
  local code
  code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 "$url/readyz" 2>/dev/null || echo "000")
  if [[ "$code" == "200" ]]; then
    pass "$name /readyz → 200"
  elif [[ -n "$soft" ]]; then
    warn "$name /readyz → $code (optional for the happy-path demo)"
  else
    fail "$name /readyz → $code at $url"
  fi
}

section "Services — liveness"
check_readyz "iam"                 "$IAM_HTTP"
check_readyz "project-management"  "$PROJECT_HTTP"
check_readyz "budget-planning"     "$BUDGET_HTTP"
check_readyz "expense-tracking"    "$EXPENSE_HTTP"     soft
check_readyz "general-ledger"      "$LEDGER_HTTP"      soft
check_readyz "inventory-management" "$INVENTORY_HTTP"  soft
check_readyz "reporting-analytics"  "$REPORTING_HTTP"  soft
check_readyz "notifications"       "$NOTIFICATIONS_HTTP" soft
check_readyz "document"            "$DOCUMENT_HTTP"    soft

# ── REST gateways (what the UI actually calls) ───────────────────────────────

check_gateway() {
  local name="$1" url="$2" probe="$3"
  local code
  code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 "$url$probe" 2>/dev/null || echo "000")
  # 401 is the expected "I'm alive but you need auth" response on these probes.
  if [[ "$code" == "200" || "$code" == "401" ]]; then
    pass "$name gateway reachable at $url ($probe → $code)"
  else
    fail "$name gateway NOT reachable at $url ($probe → $code)"
  fi
}

section "REST gateways"
check_gateway "iam"                "$IAM_GW"     "/api/v1/auth/whoami"
check_gateway "project-management" "$PROJECT_GW" "/api/v1/productions"
check_gateway "budget-planning"    "$BUDGET_GW"  "/api/v1/budgets"

# ── Web dev server ────────────────────────────────────────────────────────────

section "Web"

code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3 "$WEB_URL" 2>/dev/null || echo "000")
if [[ "$code" == "200" || "$code" == "307" || "$code" == "302" ]]; then
  pass "Web dev server responding at $WEB_URL (HTTP $code)"
else
  fail "Web dev server not responding at $WEB_URL (HTTP $code) — is npm run dev running?"
fi

# ── Tenant logins (the critical path) ─────────────────────────────────────────

login_probe() {
  local label="$1" email="$2" password="$3"
  local body resp code

  body=$(jq -cn --arg e "$email" --arg p "$password" '{email:$e, password:$p}')
  resp=$(curl -s -w "\n%{http_code}" --max-time 5 \
    -H "Content-Type: application/json" \
    -X POST "$IAM_GW/api/v1/auth/login" \
    -d "$body" 2>/dev/null || echo -e "\n000")
  code="${resp##*$'\n'}"
  body="${resp%$'\n'*}"

  if [[ "$code" == "200" ]] && echo "$body" | jq -e '.access_token' >/dev/null 2>&1; then
    pass "$label login succeeded ($email)"
    if [[ "$VERBOSE" == "1" ]]; then
      info "tenant_id=$(echo "$body" | jq -r '.tenant_id // .user.tenant_id // "?"')"
    fi
  else
    fail "$label login failed ($email → HTTP $code)"
    [[ "$VERBOSE" == "1" ]] && info "response: $body"
  fi
}

section "Tenant logins"
if [[ "${DEMO_SKIP_LOGIN:-}" == "1" ]]; then
  warn "DEMO_SKIP_LOGIN=1 — skipping login checks"
else
  login_probe "XYZ_CBA"          "$CBA_EMAIL"          "$CBA_PASSWORD"
  login_probe "XYZ Construction" "$CONSTRUCTION_EMAIL" "$CONSTRUCTION_PASSWORD"
fi

# ── Seed data sanity ──────────────────────────────────────────────────────────

section "Seed data"

count_tenants=$(psql "$DB_URL" -tAc "SELECT count(*) FROM tenants;" 2>/dev/null || echo "?")
if [[ "$count_tenants" =~ ^[0-9]+$ ]] && [[ "$count_tenants" -ge 2 ]]; then
  pass "tenants table has $count_tenants rows (≥2 expected)"
else
  fail "tenants table has $count_tenants rows — expected at least 2 (XYZ_CBA + XYZ Construction). Run: make seed && make seed-construction"
fi

count_productions=$(psql "$DB_URL" -tAc \
  "SELECT count(*) FROM productions;" 2>/dev/null || echo "?")
if [[ "$count_productions" =~ ^[0-9]+$ ]] && [[ "$count_productions" -ge 10 ]]; then
  pass "productions table has $count_productions rows across tenants"
else
  warn "productions table has $count_productions rows — expected ≥10 from the two demo seeds"
fi

# The "money shot" — Great Lakes Brewery is seeded 8% over on Materials;
# if the seed is missing or stale the over-budget demo loses its punch.
gl_exists=$(psql "$DB_URL" -tAc \
  "SELECT count(*) FROM productions WHERE title = 'Great Lakes Brewery Expansion';" 2>/dev/null || echo "0")
if [[ "$gl_exists" == "1" ]]; then
  pass "Great Lakes Brewery Expansion seeded (over-budget demo)"
else
  warn "Great Lakes Brewery Expansion not found — the over-budget demo beat won't land"
fi

# ── Summary ───────────────────────────────────────────────────────────────────

section "Summary"

if [[ "$failures" -eq 0 ]]; then
  if [[ "$warnings" -eq 0 ]]; then
    printf "${C_GREEN}All checks passed.${C_RESET} Safe to demo.\n"
  else
    printf "${C_GREEN}Required checks passed${C_RESET} with ${C_YELLOW}$warnings soft warning(s)${C_RESET}. Review before demo.\n"
  fi
  exit 0
else
  printf "${C_RED}$failures required check(s) failed.${C_RESET} Fix before demo.\n"
  exit 1
fi
