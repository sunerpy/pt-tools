#!/usr/bin/env bash
# scripts/qa/telegram-poll-help.sh — 校验 /help 回复含 11 命令
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESPONSE=$("$SCRIPT_DIR/inject-tg-cmd.sh" "/help")

CMDS=(help status torrents search push pause resume delete bind unbind mute)
MISSING=0
for cmd in "${CMDS[@]}"; do
  if ! echo "$RESPONSE" | grep -q "/$cmd"; then
    echo "MISSING: /$cmd" >&2
    MISSING=$((MISSING + 1))
  fi
done

if [ "$MISSING" -gt 0 ]; then
  echo "FAIL: $MISSING commands missing in /help reply" >&2
  exit 1
fi

echo "PASS: /help reply contains all 11 commands"
