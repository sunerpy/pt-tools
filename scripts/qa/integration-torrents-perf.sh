#!/usr/bin/env bash
# scripts/qa/integration-torrents-perf.sh — 完整 chain 链路性能（不绕过 service / downloader）
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COUNT="${1:-5000}"
THRESHOLD_MS="${PT_QA_PERF_MS:-2000}"

count="$COUNT" "$SCRIPT_DIR/seed-data.sh" >/dev/null

ITERATIONS="${PT_QA_PERF_ITERATIONS:-3}"
TOTAL_MS=0

for i in $(seq 1 "$ITERATIONS"); do
  START_NS=$(date +%s%N)
  "$SCRIPT_DIR/inject-tg-cmd.sh" "/torrents qb1" >/dev/null
  END_NS=$(date +%s%N)
  ELAPSED_MS=$(( (END_NS - START_NS) / 1000000 ))
  TOTAL_MS=$((TOTAL_MS + ELAPSED_MS))
  echo "  iter $i: ${ELAPSED_MS}ms"
done

AVG_MS=$((TOTAL_MS / ITERATIONS))
echo "avg_ms=$AVG_MS threshold_ms=$THRESHOLD_MS iterations=$ITERATIONS count=$COUNT"

if [ "$AVG_MS" -ge "$THRESHOLD_MS" ]; then
  echo "FAIL: avg $AVG_MS ms >= $THRESHOLD_MS ms" >&2
  exit 1
fi

echo "PASS < ${THRESHOLD_MS}ms (avg over $ITERATIONS iterations)"
