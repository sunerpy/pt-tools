#!/usr/bin/env bash
# scripts/qa/bind-flow.sh — 端到端 /bind 流程
# 1. POST /api/chatops/bindings/issue-code 拿 code
# 2. inject-tg-cmd "/bind <code>"
# 3. SELECT channel_binding 行 +1 且 allowed=1 pt_admin=1
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

BEFORE=$(sqlite3 "$DB" "SELECT COUNT(*) FROM channel_bindings")

ISSUE_RESPONSE=$(curl -sS --max-time "$TIMEOUT" \
  -X POST \
  -H "Content-Type: application/json" \
  ${COOKIE:+-H "Cookie: $COOKIE"} \
  -d '{"conf_id":1,"reply_lang":"zh","pt_admin":true}' \
  "$BASE_URL/api/chatops/bindings/issue-code")

CODE=$(echo "$ISSUE_RESPONSE" | jq -r '.code // empty')
if [ -z "$CODE" ]; then
  echo "FAIL: could not issue bind code" >&2
  echo "Response: $ISSUE_RESPONSE" >&2
  exit 1
fi

"$SCRIPT_DIR/inject-tg-cmd.sh" "/bind $CODE" >/dev/null

AFTER=$(sqlite3 "$DB" "SELECT COUNT(*) FROM channel_bindings")
ALLOWED=$(sqlite3 "$DB" "SELECT allowed FROM channel_bindings ORDER BY id DESC LIMIT 1")
ADMIN=$(sqlite3 "$DB" "SELECT pt_admin FROM channel_bindings ORDER BY id DESC LIMIT 1")

DIFF=$((AFTER - BEFORE))
if [ "$DIFF" -ne 1 ]; then
  echo "FAIL: expected +1 binding row, got +$DIFF" >&2
  exit 1
fi

if [ "$ALLOWED" != "1" ] || [ "$ADMIN" != "1" ]; then
  echo "FAIL: allowed=$ALLOWED pt_admin=$ADMIN (expected 1/1)" >&2
  exit 1
fi

echo "PASS: bind flow created allowed=1 pt_admin=1 binding"
