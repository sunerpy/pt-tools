#!/usr/bin/env bash
# scripts/qa/inject-tg-cmd.sh — 注入 Telegram 入站消息到测试钩子端点
# 用法：./inject-tg-cmd.sh "<command>" [user_id] [conf_id]
# 依赖：T17 telegram adapter + qa build tag 启用的 /test/telegram/inject
set -euo pipefail

CMD="${1:-}"
USER_ID="${2:-12345}"
CONF_ID="${3:-1}"
BASE_URL="${PT_QA_BASE_URL:-http://127.0.0.1:8080}"
TIMEOUT="${PT_QA_TIMEOUT:-5}"

if [ -z "$CMD" ]; then
  echo "ERROR: command argument required" >&2
  echo "Usage: $0 \"/help\" [user_id] [conf_id]" >&2
  exit 2
fi

PAYLOAD=$(jq -n \
  --arg text "$CMD" \
  --argjson user_id "$USER_ID" \
  --argjson conf_id "$CONF_ID" \
  '{text: $text, from: {id: $user_id, username: "qa-tester"}, conf_id: $conf_id, chat_id: ($user_id | tostring)}')

RESPONSE=$(curl -sS --max-time "$TIMEOUT" \
  -X POST \
  -H "Content-Type: application/json" \
  -d "$PAYLOAD" \
  -w "\n%{http_code}" \
  "$BASE_URL/test/telegram/inject")

HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | sed '$d')

echo "$BODY"

if [ "$HTTP_CODE" != "200" ]; then
  echo "ERROR: HTTP $HTTP_CODE" >&2
  exit 1
fi
