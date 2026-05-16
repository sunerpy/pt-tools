#!/usr/bin/env bash
# scripts/qa/run-all-task-scenarios.sh — 编排
# 顺序执行 task-{1..33}-*.sh，结果汇总到 final-f3-tasks.txt
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
EVIDENCE_DIR="${PT_QA_EVIDENCE_DIR:-.sisyphus/evidence}"
OUT="$EVIDENCE_DIR/final-f3-tasks.txt"

mkdir -p "$EVIDENCE_DIR"
: > "$OUT"

TOTAL=0
PASSED=0
FAILED=0

for n in $(seq 1 33); do
  for script in "$SCRIPT_DIR"/task-${n}-*.sh; do
    [ -f "$script" ] || continue
    TOTAL=$((TOTAL + 1))
    name=$(basename "$script")
    if "$script" >>"$OUT" 2>&1; then
      echo "[PASS] $name" | tee -a "$OUT"
      PASSED=$((PASSED + 1))
    else
      echo "[FAIL] $name" | tee -a "$OUT"
      FAILED=$((FAILED + 1))
    fi
  done
done

echo "" | tee -a "$OUT"
echo "Summary: total=$TOTAL passed=$PASSED failed=$FAILED" | tee -a "$OUT"

if [ "$TOTAL" -eq 0 ]; then
  echo "WARN: no task-N-*.sh scripts found (each task implementer must add their own)"
  exit 0
fi

if [ "$FAILED" -gt 0 ]; then
  exit 1
fi

echo "All $TOTAL task scenarios PASSED"
