#!/usr/bin/env bash
# scripts/qa/webhook-event.sh — 启动 httptest receiver，触发 EvtTorrentAdded，校验 webhook POST 200
set -euo pipefail

PORT="${PT_QA_WEBHOOK_PORT:-19999}"
LOG_FILE=$(mktemp -t qa-webhook.XXXXXX.log)
trap 'rm -f "$LOG_FILE"; jobs -p | xargs -r kill 2>/dev/null || true' EXIT

go run - "$PORT" "$LOG_FILE" <<'GO' &
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

func main() {
	port := os.Args[1]
	logPath := os.Args[2]
	http.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f, _ := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		defer f.Close()
		fmt.Fprintf(f, "%s\n", body)
		w.WriteHeader(http.StatusOK)
	})
	_ = http.ListenAndServe(":"+port, nil)
}
GO
RECV_PID=$!

sleep 1

BASE_URL="${PT_QA_BASE_URL:-http://127.0.0.1:8080}"
TIMEOUT="${PT_QA_TIMEOUT:-5}"

PAYLOAD=$(jq -n --arg url "http://127.0.0.1:$PORT/webhook" \
  '{event_type: "torrent_added", target_url: $url, torrent: {name: "qa-test.mkv", size: 1024}}')

curl -sS --max-time "$TIMEOUT" \
  -X POST \
  -H "Content-Type: application/json" \
  -d "$PAYLOAD" \
  "$BASE_URL/test/telegram/inject" >/dev/null || true

sleep 2

if [ -s "$LOG_FILE" ] && grep -q "torrent_added" "$LOG_FILE"; then
  echo "PASS: webhook receiver got POST"
  exit 0
fi

echo "FAIL: webhook receiver did not receive POST" >&2
exit 1
