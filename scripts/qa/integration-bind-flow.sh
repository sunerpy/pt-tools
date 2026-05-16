#!/usr/bin/env bash
# scripts/qa/integration-bind-flow.sh — 端到端 Web UI + bot 流程
# 同 bind-flow.sh，但显式从 /api/chatops/bindings/issue-code 走 Web 调用并校验列表 API
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DB="${PT_QA_DB:-testdata/qa.db}"
BASE_URL="${PT_QA_BASE_URL:-http://127.0.0.1:8080}"
COOKIE="${PT_QA_SESSION_COOKIE:-}"
TIMEOUT="${PT_QA_TIMEOUT:-5}"

if [ ! -f "$DB" ]; then
  echo "ERROR: $DB not found; run seed-data.sh first" >&2
  exit 2
fi

BEFORE=$(curl -sS --max-time "$TIMEOUT" \
  ${COOKIE:+-H "Cookie: $COOKIE"} \
  "$BASE_URL/api/chatops/bindings" | jq -r 'length // 0')

"$SCRIPT_DIR/bind-flow.sh"

AFTER=$(curl -sS --max-time "$TIMEOUT" \
  ${COOKIE:+-H "Cookie: $COOKIE"} \
  "$BASE_URL/api/chatops/bindings" | jq -r 'length // 0')

if [ "$AFTER" -le "$BEFORE" ]; then
  echo "FAIL: bindings list did not grow ($BEFORE -> $AFTER)" >&2
  exit 1
fi

echo "PASS: integration bind flow ($BEFORE -> $AFTER bindings)"
