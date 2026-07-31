#!/usr/bin/env bash
# Record demo fixtures from a live, seeded local stack.
#
# Captures only the five-page slice: login, productions (+detail), budgets
# (+detail). Everything else in the web tier calls services that expose no
# grpc-gateway — see mambakkam-net/docs/DESIGN_thittam_demo.md.
#
# Usage:
#   make db-bootstrap WITH_SEED=1
#   make dev-start
#   ./scripts/capture-demo-fixtures.sh
set -euo pipefail

IAM="${IAM_URL:-http://localhost:9086}"
PROJECT="${PROJECT_URL:-http://localhost:9080}"
BUDGET="${BUDGET_URL:-http://localhost:9081}"
EMAIL="${DEMO_EMAIL:-rajesh.kumar@xyzcba.com}"
PASSWORD="${DEMO_PASSWORD:-demo1234}"
OUT="web/src/demo/fixtures.generated.json"

command -v jq >/dev/null || { echo "FATAL: jq is required"; exit 2; }

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
: > "$work/pairs.jsonl"

# record <key> <base> <path> [json-body]
# Fails the whole run on any non-2xx: a demo built on error bodies is worse
# than no demo.
record() {
  local key="$1" base="$2" path="$3" body="${4:-}"
  local code out
  out="$work/body.json"

  if [[ -n "$body" ]]; then
    code=$(curl -sS -o "$out" -w '%{http_code}' \
      -X POST "$base$path" \
      -H 'Content-Type: application/json' \
      -H "Authorization: Bearer ${TOKEN:-}" \
      -d "$body")
  else
    code=$(curl -sS -o "$out" -w '%{http_code}' \
      "$base$path" -H "Authorization: Bearer ${TOKEN:-}")
  fi

  if [[ "$code" != 2* ]]; then
    echo "FATAL: $key -> HTTP $code" >&2
    head -c 400 "$out" >&2; echo >&2
    exit 1
  fi

  jq -c --arg k "$key" '{key: $k, value: .}' "$out" >> "$work/pairs.jsonl"
  echo "  ok  $key"
}

echo "==> login"
login_body=$(jq -nc --arg e "$EMAIL" --arg p "$PASSWORD" \
  '{email: $e, password: $p}')
record "POST /api/v1/auth/login" "$IAM" "/api/v1/auth/login" "$login_body"
TOKEN=$(jq -r '.value.access_token' < <(tail -1 "$work/pairs.jsonl"))
[[ "$TOKEN" != "null" && -n "$TOKEN" ]] || { echo "FATAL: no access_token"; exit 1; }

echo "==> config"
record "GET /api/v1/config/entity-labels" "$PROJECT" "/api/v1/config/entity-labels"
record "GET /api/v1/config/phase-types"   "$PROJECT" "/api/v1/config/phase-types"

# /api/v1/config/budget-categories is deliberately NOT captured. The budget
# service implements GetBudgetCategories, but the RPC carries no
# google.api.http annotation (proto/thittam/budget/v1/budget.proto:40), so it
# never reaches grpc-gateway — the route returns 404 against a live stack, and
# this script aborts the whole run on any non-2xx. The same is true of
# budget-templates.
#
# Nothing in the demo slice needs it: web/src/lib/api/budgets.ts exports
# getBudgetCategories() and use-budgets.ts:232 wraps it as useBudgetCategories(),
# but no page in the five-page slice calls that hook. The real app absorbs the
# failure silently, which is why reading the client code made the endpoint look
# in scope.
#
# Do not re-add this line without first adding the http annotation and
# regenerating the gateway.

echo "==> productions"
record "GET /api/v1/productions" "$PROJECT" "/api/v1/productions"
prod_ids=$(jq -r 'select(.key == "GET /api/v1/productions")
  | .value.productions[]?.id' "$work/pairs.jsonl")
[[ -n "$prod_ids" ]] || { echo "FATAL: productions list is empty — is the seed loaded?"; exit 1; }

for id in $prod_ids; do
  record "GET /api/v1/productions/$id" "$PROJECT" "/api/v1/productions/$id"
  record "GET /api/v1/productions/$id/phases" "$PROJECT" "/api/v1/productions/$id/phases"
  record "GET /api/v1/productions/$id/crew"   "$PROJECT" "/api/v1/productions/$id/crew"
done

echo "==> budgets (per production — no tenant-wide list endpoint exists)"
# The web tier fans out one budgets query per production; there is NO bare
# GET /api/v1/budgets (see web/src/app/(dashboard)/budgets/page.tsx:47 and
# listBudgets(), which sends ?production_id=). Mirror that here.
for pid in $prod_ids; do
  record "GET /api/v1/budgets?production_id=$pid" "$BUDGET" "/api/v1/budgets?production_id=$pid"
done
budget_ids=$(jq -r 'select(.key | startswith("GET /api/v1/budgets?production_id="))
  | .value.budgets[]?.id' "$work/pairs.jsonl" | sort -u)
[[ -n "$budget_ids" ]] || { echo "FATAL: no budgets across any production — is the seed loaded?"; exit 1; }

for id in $budget_ids; do
  record "GET /api/v1/budgets/$id" "$BUDGET" "/api/v1/budgets/$id"
  record "GET /api/v1/budgets/$id/line-items" "$BUDGET" "/api/v1/budgets/$id/line-items"
done

echo "==> writing $OUT"
jq -s --arg email "$EMAIL" '
  {
    _meta: {
      capturedAt: (now | todate),
      tenant: "xyz-cba",
      demoEmail: $email
    },
    responses: (map({(.key): .value}) | add)
  }
' "$work/pairs.jsonl" > "$OUT"

echo
echo "Captured $(jq '.responses | length' "$OUT") responses."
echo "REVIEW BEFORE COMMITTING — an endpoint can return 200 with an empty list."
jq -r '.responses | to_entries[]
  | "\(.key)\t\(.value | if type == "object" then
      (to_entries | map(select(.value | type == "array"))
        | map("\(.key)=\(.value | length)") | join(",")) else "" end)"' "$OUT"
