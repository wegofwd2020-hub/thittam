#!/usr/bin/env bash
# scripts/verify-project-rbac.sh — End-to-end verification of ADR-014 Phase 2.
#
# Drives the four gated RPCs from #43 against three pre-seeded XYZ_CBA users:
#
#   Priya Sharma  (manager)            — tenant-wide, should pass on any project
#   Vikram Reddy  (project_supervisor) — scoped to The Last Horizon only
#   Deepa Menon   (member)             — should be denied on all gated RPCs
#
# Prereqs:
#   - Local Postgres bootstrapped + seeded:    make db-bootstrap WITH_SEED=1
#   - Services running with rollout enabled:   ./scripts/dev-start.sh --with-project-rbac
#   - grpcurl installed:                       go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
#
# Exit code is 0 iff every probe matches its expected outcome.

set -uo pipefail

# Hard prereq — without grpcurl every probe silently produces no output, which
# the parser would interpret as code=OK and mis-flag pass-expected probes as
# successful. Bail loudly instead.
if ! command -v grpcurl >/dev/null 2>&1; then
  echo "ERROR: grpcurl not found on PATH."
  echo "Install with: go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest"
  echo "Then ensure \$HOME/go/bin is on your PATH."
  exit 2
fi

# ── Constants (mirror seeds/demo/xyz-cba/) ────────────────────────────────────

TENANT_ID="d0000000-0000-0000-0000-000000000001"

PRIYA_ID="d1000000-0000-0000-0000-000000000002"     # manager
VIKRAM_ID="d1000000-0000-0000-0000-000000000005"    # project_supervisor on Last Horizon
DEEPA_ID="d1000000-0000-0000-0000-000000000006"     # member

PROJECT_LAST_HORIZON="d2000000-0000-0000-0000-000000000001"
PROJECT_OTHER="d2000000-0000-0000-0000-000000000002"   # different production — Vikram unscoped

# Service addresses (local dev defaults from cmd/*/main.go).
IAM_ADDR=localhost:8086
BUDGET_ADDR=localhost:8081
EXPENSE_ADDR=localhost:8082
PROJECT_ADDR=localhost:8080
INVENTORY_ADDR=localhost:8084

# Dummy IDs for the gated RPC bodies. These don't need to refer to real rows —
# the permission check fires before the handler dereferences them, and we only
# care about distinguishing PermissionDenied from other status codes.
DUMMY_BUDGET_ID="00000000-0000-0000-0000-000000000bbb"
DUMMY_EXPENSE_ID="00000000-0000-0000-0000-000000000eee"
DUMMY_PHASE_ID="00000000-0000-0000-0000-000000000ddd"
DUMMY_ASSET_ID="00000000-0000-0000-0000-000000000aaa"

# ── Output helpers ───────────────────────────────────────────────────────────

if [ -t 1 ]; then
  GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'; BOLD='\033[1m'; RESET='\033[0m'
else
  GREEN=''; RED=''; YELLOW=''; BOLD=''; RESET=''
fi

PASS=0
FAIL=0

# probe <name> <expected_pass|expected_deny> <user_id> <project_id|-> <addr> <fq_method> <body_json>
#
# Calls grpcurl, then asserts:
#   expected_pass — code is OK or NotFound/InvalidArgument (i.e. permission was granted; handler ran)
#   expected_deny — code is PermissionDenied
probe() {
  local name=$1 expected=$2 user=$3 project=$4 addr=$5 method=$6 body=$7
  local hdrs=( -H "x-caller-id: $user" -H "x-tenant-id: $TENANT_ID" )
  if [ "$project" != "-" ]; then
    hdrs+=( -H "x-project-id: $project" )
  fi

  local out code
  out=$(grpcurl -plaintext "${hdrs[@]}" -d "$body" "$addr" "$method" 2>&1)
  local rc=$?
  if echo "$out" | grep -qE 'code = [A-Za-z]+'; then
    code=$(echo "$out" | grep -oE 'code = [A-Za-z]+' | head -1 | awk '{print $3}')
  elif [ $rc -eq 0 ]; then
    # No status code in output and grpcurl exited 0 → call genuinely succeeded.
    code=OK
  else
    # grpcurl errored without producing a gRPC status (e.g. dial failure,
    # service down). Surface as Unavailable so the assertion fails loudly.
    code=Unavailable
  fi

  local ok=false
  case "$expected" in
    pass)
      # Permission was granted iff the handler ran. Allow OK plus the
      # status codes the handler can legitimately return when given dummy
      # IDs (NotFound / InvalidArgument / FailedPrecondition).
      # Anything else — Unavailable, Internal, PermissionDenied — is a fail.
      case "$code" in
        OK|NotFound|InvalidArgument|FailedPrecondition) ok=true ;;
      esac
      ;;
    deny)
      [ "$code" = "PermissionDenied" ] && ok=true
      ;;
  esac

  if $ok; then
    printf "  ${GREEN}✓${RESET} %-45s expected=%s code=%s\n" "$name" "$expected" "$code"
    PASS=$((PASS+1))
  else
    printf "  ${RED}✗${RESET} %-45s expected=%s code=%s\n" "$name" "$expected" "$code"
    echo "      ${out}" | head -3
    FAIL=$((FAIL+1))
  fi
}

# ── Probes ───────────────────────────────────────────────────────────────────

echo -e "${BOLD}Verifying PROJECT_SCOPED_RBAC enforcement...${RESET}"
echo ""

echo -e "${BOLD}1. Tenant-wide manager (Priya) — should pass everywhere${RESET}"
probe "manager: ApproveBudget (no project)"        pass "$PRIYA_ID" - \
  "$BUDGET_ADDR"  thittam.budget.v1.BudgetService/ApproveBudget \
  "{\"id\":\"$DUMMY_BUDGET_ID\"}"

probe "manager: ApproveBudget on Last Horizon"     pass "$PRIYA_ID" "$PROJECT_LAST_HORIZON" \
  "$BUDGET_ADDR"  thittam.budget.v1.BudgetService/ApproveBudget \
  "{\"id\":\"$DUMMY_BUDGET_ID\"}"

echo ""
echo -e "${BOLD}2. project_supervisor (Vikram) on Last Horizon — pass; on other project — deny${RESET}"
probe "supervisor: ApproveExpense on Last Horizon" pass "$VIKRAM_ID" "$PROJECT_LAST_HORIZON" \
  "$EXPENSE_ADDR" thittam.expense.v1.ExpenseService/ApproveExpense \
  "{\"expense_id\":\"$DUMMY_EXPENSE_ID\"}"

probe "supervisor: ApproveExpense on other project" deny "$VIKRAM_ID" "$PROJECT_OTHER" \
  "$EXPENSE_ADDR" thittam.expense.v1.ExpenseService/ApproveExpense \
  "{\"expense_id\":\"$DUMMY_EXPENSE_ID\"}"

probe "supervisor: ApproveBudget tenant-wide"      deny "$VIKRAM_ID" - \
  "$BUDGET_ADDR"  thittam.budget.v1.BudgetService/ApproveBudget \
  "{\"id\":\"$DUMMY_BUDGET_ID\"}"

echo ""
echo -e "${BOLD}3. Tenant-wide member (Deepa) — should be denied on all gated RPCs${RESET}"
probe "member: ApproveBudget"                      deny "$DEEPA_ID" - \
  "$BUDGET_ADDR"  thittam.budget.v1.BudgetService/ApproveBudget \
  "{\"id\":\"$DUMMY_BUDGET_ID\"}"

probe "member: ApproveExpense"                     deny "$DEEPA_ID" "$PROJECT_LAST_HORIZON" \
  "$EXPENSE_ADDR" thittam.expense.v1.ExpenseService/ApproveExpense \
  "{\"expense_id\":\"$DUMMY_EXPENSE_ID\"}"

probe "member: UpdatePhaseStatus"                  deny "$DEEPA_ID" "$PROJECT_LAST_HORIZON" \
  "$PROJECT_ADDR" thittam.project.v1.ProjectService/UpdatePhaseStatus \
  "{\"phase_id\":\"$DUMMY_PHASE_ID\",\"status\":\"in_progress\"}"

probe "member: CheckOutAsset"                      deny "$DEEPA_ID" "$PROJECT_LAST_HORIZON" \
  "$INVENTORY_ADDR" thittam.inventory.v1.InventoryService/CheckOutAsset \
  "{\"asset_id\":\"$DUMMY_ASSET_ID\"}"

# ── Summary ──────────────────────────────────────────────────────────────────

echo ""
if [ "$FAIL" -eq 0 ]; then
  echo -e "${GREEN}${BOLD}All $PASS probes passed.${RESET}"
  exit 0
else
  echo -e "${RED}${BOLD}$FAIL of $((PASS+FAIL)) probes failed.${RESET}"
  echo ""
  echo "Common causes:"
  echo "  - Services not started with --with-project-rbac"
  echo "  - Vikram's project_supervisor row missing (run: make db-reset)"
  echo "  - IAM service unreachable on $IAM_ADDR"
  exit 1
fi
