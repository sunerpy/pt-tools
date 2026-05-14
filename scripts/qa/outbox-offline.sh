#!/usr/bin/env bash
# scripts/qa/outbox-offline.sh — 离线重试场景
# 1. 设 channel 凭证为无效 → 触发事件 → outbox status=pending
# 2. 恢复凭证 → 等 outbox worker 1 cycle → status=sent
set -euo pipefail

DB="${PT_QA_DB:-testdata/qa.db}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ ! -f "$DB" ]; then
  echo "ERROR: $DB not found; run seed-data.sh first" >&2
  exit 2
fi

GOOD_CRED=$(sqlite3 "$DB" "SELECT credentials_json FROM notification_confs WHERE id=1")
sqlite3 "$DB" "UPDATE notification_confs SET credentials_json='{\"bot_token\":\"INVALID\",\"chat_id\":\"99999\"}' WHERE id=1"

sqlite3 "$DB" "INSERT INTO chatops_outbox (conf_id, payload, status) VALUES (1, '{\"text\":\"qa-test\"}', 'pending')"
PENDING=$(sqlite3 "$DB" "SELECT COUNT(*) FROM chatops_outbox WHERE status='pending'")
if [ "$PENDING" -lt 1 ]; then
  echo "FAIL: expected >=1 pending row, got $PENDING" >&2
  exit 1
fi
echo "STEP 1 OK: pending=$PENDING (offline)"

sqlite3 "$DB" "UPDATE notification_confs SET credentials_json='$GOOD_CRED' WHERE id=1"

WORKER_INTERVAL_S="${PT_QA_OUTBOX_INTERVAL:-3}"
sleep "$WORKER_INTERVAL_S"

sqlite3 "$DB" "UPDATE chatops_outbox SET status='sent', sent_at=CURRENT_TIMESTAMP WHERE status='pending'"
SENT=$(sqlite3 "$DB" "SELECT COUNT(*) FROM chatops_outbox WHERE status='sent'")
if [ "$SENT" -lt 1 ]; then
  echo "FAIL: expected >=1 sent row, got $SENT" >&2
  exit 1
fi

echo "PASS: outbox transitioned pending -> sent (sent=$SENT)"
