#!/usr/bin/env bash
# scripts/qa/inject-qq-cmd.sh — 注入 QQ OneBot 入站消息
# 用法：./inject-qq-cmd.sh "<command>" [user_id] [conf_id]
set -euo pipefail

CMD="${1:-}"
USER_ID="${2:-67890}"
CONF_ID="${3:-1}"
BASE_URL="${PT_QA_BASE_URL:-http://127.0.0.1:8080}"
TIMEOUT="${PT_QA_TIMEOUT:-5}"

if [ -z "$CMD" ]; then
  echo "ERROR: command argument required" >&2
  echo "Usage: $0 \"/status\" [user_id] [conf_id]" >&2
  exit 2
fi

PAYLOAD=$(jq -n \
  --arg text "$CMD" \
  --argjson user_id "$USER_ID" \
  --argjson conf_id "$CONF_ID" \
  '{text: $text, from: {id: $user_id, username: "qa-qq-tester"}, conf_id: $conf_id, chat_id: ($user_id | tostring)}')

RESPONSE=$(curl -sS --max-time "$TIMEOUT" \
  -X POST \
  -H "Content-Type: application/json" \
  -d "$PAYLOAD" \
  -w "\n%{http_code}" \
  "$BASE_URL/test/qq/inject")

HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | sed '$d')

echo "$BODY"

if [ "$HTTP_CODE" != "200" ]; then
  echo "ERROR: HTTP $HTTP_CODE" >&2
  exit 1
fi
