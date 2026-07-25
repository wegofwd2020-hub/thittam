#!/usr/bin/env bash
# Verifies Kong routes the login flow end to end (#60 Phase B). Requires the
# stack up first:
#   make infra-up-full && make db-bootstrap WITH_SEED=1 && make dev-start
set -euo pipefail

KONG_URL="${KONG_URL:-http://localhost:8500}"
EMAIL="${LOGIN_EMAIL:-rajesh.kumar@xyzcba.com}"
PASSWORD="${LOGIN_PASSWORD:-demo1234}"
TENANT="${TENANT_ID:-d0000000-0000-0000-0000-000000000001}"

echo "==> POST ${KONG_URL}/api/v1/auth/login  (via Kong → iam grpc-gateway)"
# -f: fail (non-zero) on any non-2xx — e.g. a missing Kong route returns 404.
resp=$(curl -sf -X POST "${KONG_URL}/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -H "X-Tenant-ID: ${TENANT}" \
  -d "{\"email\":\"${EMAIL}\",\"password\":\"${PASSWORD}\"}")

if echo "${resp}" | grep -q '"access_token"'; then
  echo "OK: Kong routed login to iam; access_token present in the TokenPair."
else
  echo "FAIL: login succeeded (2xx) but no access_token in response:"
  echo "${resp}"
  exit 1
fi
