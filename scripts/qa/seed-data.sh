#!/usr/bin/env bash
# scripts/qa/seed-data.sh — sqlite3 测试夹具填充
#
# 准备：1 NotificationConf (telegram, mock token) + N fake torrents
# 用法：./seed-data.sh   （或 count=10000 ./seed-data.sh）
# 退出码：0=成功，非 0=失败
set -euo pipefail

DB="${PT_QA_DB:-testdata/qa.db}"
COUNT="${count:-5000}"

mkdir -p "$(dirname "$DB")"
rm -f "$DB"

sqlite3 "$DB" <<SQL
CREATE TABLE notification_confs (
  id INTEGER PRIMARY KEY,
  channel_type TEXT NOT NULL,
  name TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  credentials_json TEXT NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE channel_bindings (
  id INTEGER PRIMARY KEY,
  conf_id INTEGER NOT NULL,
  channel_type TEXT NOT NULL,
  channel_user_id TEXT NOT NULL,
  reply_lang TEXT NOT NULL DEFAULT 'zh',
  pt_admin INTEGER NOT NULL DEFAULT 0,
  allowed INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE bind_codes (
  id INTEGER PRIMARY KEY,
  code TEXT NOT NULL UNIQUE,
  conf_id INTEGER NOT NULL,
  expires_at TIMESTAMP NOT NULL,
  used_at TIMESTAMP
);
CREATE TABLE chatops_outbox (
  id INTEGER PRIMARY KEY,
  conf_id INTEGER NOT NULL,
  payload TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  attempts INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  sent_at TIMESTAMP
);
CREATE TABLE fake_torrents (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  size_bytes INTEGER NOT NULL,
  downloader TEXT NOT NULL DEFAULT 'qb1'
);

INSERT INTO notification_confs (id, channel_type, name, credentials_json)
VALUES (1, 'telegram', 'qa-telegram', '{"bot_token":"mock-12345:ABC","chat_id":"99999"}');
SQL

awk -v n="$COUNT" 'BEGIN{
  print "BEGIN;"
  for (i = 1; i <= n; i++) {
    printf "INSERT INTO fake_torrents (name, size_bytes, downloader) VALUES ('\''qa-torrent-%06d.mkv'\'', %d, '\''qb1'\'');\n", i, (i*1024*1024)
  }
  print "COMMIT;"
}' | sqlite3 "$DB"

ROWS=$(sqlite3 "$DB" "SELECT COUNT(*) FROM fake_torrents")
CONFS=$(sqlite3 "$DB" "SELECT COUNT(*) FROM notification_confs")

echo "OK: db=$DB confs=$CONFS torrents=$ROWS"
test "$CONFS" = "1"
test "$ROWS" = "$COUNT"
