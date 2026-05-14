#!/usr/bin/env bash
# scripts/qa/qq-onebot-status.sh — 校验 QQ /status 回复
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESPONSE=$("$SCRIPT_DIR/inject-qq-cmd.sh" "/status")

if echo "$RESPONSE" | grep -qiE "running|version|ok"; then
  echo "PASS: /status reply matches expected pattern"
  exit 0
fi

echo "FAIL: /status reply did not match" >&2
echo "Response: $RESPONSE" >&2
exit 1
