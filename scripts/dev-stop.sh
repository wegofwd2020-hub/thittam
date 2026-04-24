#!/usr/bin/env bash
# scripts/dev-stop.sh — Stop all Thittam dev services and (optionally) middleware.
#
# Usage:
#   ./scripts/dev-stop.sh              # stop services only (keep Docker containers)
#   ./scripts/dev-stop.sh --infra      # stop services AND Docker containers

set -euo pipefail

PID_FILE="/tmp/thittam-dev.pids"
INFRA_FILE="infra/local/docker-compose.infra.yml"

if [ -t 1 ]; then
  GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'
  BOLD='\033[1m'; RESET='\033[0m'
else
  GREEN=''; YELLOW=''; CYAN=''; BOLD=''; RESET=''
fi

STOP_INFRA=false
for arg in "$@"; do
  case $arg in
    --infra) STOP_INFRA=true ;;
    *)
      echo "Unknown flag: $arg"
      echo "Usage: $0 [--infra]"
      exit 1
      ;;
  esac
done

echo -e "${BOLD}Thittam — Stopping Dev Stack${RESET}"
echo ""

# ── Stop service processes ────────────────────────────────────────────────────

if [ -f "$PID_FILE" ]; then
  STOPPED=0
  MISSING=0
  while IFS= read -r pid; do
    [ -z "$pid" ] && continue
    if kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null && echo -e "  ${GREEN}✓${RESET} Stopped PID ${pid}" || true
      STOPPED=$((STOPPED + 1))
    else
      MISSING=$((MISSING + 1))
    fi
  done < "$PID_FILE"
  rm -f "$PID_FILE"
  echo ""
  echo -e "  Stopped ${STOPPED} launcher(s). ${MISSING} were already gone."
fi

# Always sweep the compiled service binaries — `go run` exec's into a cached
# binary whose PID is not the PID we recorded in PID_FILE, so killing the
# launcher above does not cascade. Also serves as fallback when PID_FILE is
# missing (e.g. process crash, or dev-start was never run in this shell).
echo ""
echo "  Sweeping service binaries..."
SERVICES=(iam project-management budget-planning expense-tracking
          general-ledger inventory-management reporting-analytics
          notifications document billing)
for svc in "${SERVICES[@]}"; do
  # Kill the `go run` launcher if still around (belt-and-braces for the PID-file path).
  pkill -f "go run ./cmd/${svc}" 2>/dev/null \
    && echo -e "  ${GREEN}✓${RESET} Killed ${svc} (go run)" || true
  # Kill the compiled binary that `go run` exec'd into. The kernel truncates
  # comm to 15 chars, so long names (e.g. inventory-management) need the
  # truncated form for `pkill -x`.
  pkill -x "${svc:0:15}" 2>/dev/null \
    && echo -e "  ${GREEN}✓${RESET} Killed ${svc} (binary)" || true
done

# ── Stop infrastructure ───────────────────────────────────────────────────────

if [ "$STOP_INFRA" = true ]; then
  REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  echo ""
  echo -e "  ${CYAN}→${RESET} Stopping Docker middleware..."
  docker compose -f "${REPO_ROOT}/${INFRA_FILE}" down
  echo -e "  ${GREEN}✓${RESET} Docker containers stopped"
else
  echo ""
  echo -e "  ${YELLOW}Docker containers still running.${RESET}"
  echo "  To stop them too:  ./scripts/dev-stop.sh --infra"
  echo "  To stop manually:  docker compose -f ${INFRA_FILE} down"
fi

echo ""
echo -e "${GREEN}Done.${RESET}"
