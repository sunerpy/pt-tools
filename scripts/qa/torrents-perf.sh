#!/usr/bin/env bash
# scripts/qa/torrents-perf.sh — 性能基准 /torrents
# 用法：./torrents-perf.sh [count]
# 目标：elapsed_ms < 2000
set -euo pipefail

COUNT="${1:-5000}"
THRESHOLD_MS="${PT_QA_PERF_MS:-2000}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

count="$COUNT" "$SCRIPT_DIR/seed-data.sh" >/dev/null

START_NS=$(date +%s%N)
"$SCRIPT_DIR/inject-tg-cmd.sh" "/torrents qb1" >/dev/null
END_NS=$(date +%s%N)

ELAPSED_MS=$(( (END_NS - START_NS) / 1000000 ))
echo "elapsed_ms=$ELAPSED_MS threshold_ms=$THRESHOLD_MS count=$COUNT"

if [ "$ELAPSED_MS" -ge "$THRESHOLD_MS" ]; then
  echo "FAIL: $ELAPSED_MS ms >= $THRESHOLD_MS ms" >&2
  exit 1
fi

echo "PASS < ${THRESHOLD_MS}ms"
