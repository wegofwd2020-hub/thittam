#!/usr/bin/env bash
# Bulk-updates docs/<old-path>/ → docs/<new-path>/ across Thittam source,
# Prometheus alerts, workflows, and seed READMEs. Use when reorganising
# thittam_docs' folder layout so we don't hand-edit 13 runbook URLs and
# scattered code/config refs.
#
# Usage:
#   scripts/update-docs-refs.sh <old-path> <new-path>
#
# Example (the Apr 2026 reorg that moved content under docs/developers/):
#   scripts/update-docs-refs.sh "docs/operations" "docs/developers/operations"
#   scripts/update-docs-refs.sh "docs/architecture" "docs/developers/architecture"
#
# After running, verify with `git diff` and revert any refs that point at
# thittam-local docs (e.g., thittam/docs/operations/nats-dlq.md) rather
# than cross-repo thittam_docs paths.

set -euo pipefail

if [ $# -ne 2 ]; then
  echo "usage: $0 <old-path> <new-path>" >&2
  echo "example: $0 docs/operations docs/developers/operations" >&2
  exit 2
fi

OLD="$1"
NEW="$2"

# File globs we touch. Extend as needed.
mapfile -t FILES < <(git ls-files \
  '*.md' '*.go' '*.yml' '*.yaml' '*.sh' 'Makefile' \
  ':!:vendor/**' ':!:node_modules/**')

changed=0
for f in "${FILES[@]}"; do
  if grep -q "${OLD}/" "$f" 2>/dev/null; then
    sed -i "s|${OLD}/|${NEW}/|g" "$f"
    changed=$((changed + 1))
    echo "  updated: $f"
  fi
done

echo ""
echo "Updated $changed file(s)."
echo "Run 'git diff' and revert any rewrites that point at thittam's own"
echo "docs/ (not thittam_docs/) — they did not move."
