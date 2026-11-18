#!/usr/bin/env bash
# 对 wrangler dev 跑 relay 的一致性测试（Go 黑盒，internal/remote/relaytest）：三种限额各起一个实例，
# 再用真正的主机端经它走一遍配对与会话。本地与 .github/workflows/relay.yml 都用这个脚本。
#
#   relay/cloudflare/scripts/conformance.sh        # 在仓库根或 relay/cloudflare 里跑都行
set -euo pipefail

here="$(cd "$(dirname "$0")/.." && pwd)"
root="$(cd "$here/../.." && pwd)"
state="$(mktemp -d)"
pids=()

cleanup() {
  for p in "${pids[@]}"; do
    # setsid 起的进程组：连 workerd 一起停
    kill -- "-$p" 2>/dev/null || true
  done
  rm -rf "$state"
}
trap cleanup EXIT

export NO_PROXY="127.0.0.1,localhost${NO_PROXY:+,$NO_PROXY}"
export no_proxy="$NO_PROXY"

start() {
  local name="$1" port="$2" inspector="$3"
  shift 3
  # 不套子 shell：$! 就是 setsid 起的进程组组长，清理时整组停掉（连 workerd）
  setsid pnpm --dir "$here" exec wrangler dev --ip 127.0.0.1 --port "$port" --inspector-port "$inspector" \
    --show-interactive-dev-session=false --persist-to "$state/$name" "$@" >"$state/$name.log" 2>&1 </dev/null &
  pids+=("$!")
}

start main 18761 19229 --var MAX_STREAMS_PER_HOST:4 --var MAX_CONN_PER_IP_PER_MIN:-1 --var DAILY_BYTES_PER_HOST:0
start limits 18762 19230 --var MAX_STREAMS_PER_HOST:16 --var MAX_CONN_PER_IP_PER_MIN:8 --var DAILY_BYTES_PER_HOST:200000
start disabled 18763 19231 --var RELAY_DISABLED:true

for port in 18761 18762 18763; do
  if ! timeout 120 sh -c "until curl -sf --noproxy '*' http://127.0.0.1:$port/healthz >/dev/null; do sleep 1; done"; then
    echo "wrangler dev（端口 $port）没有起来：" >&2
    cat "$state"/*.log >&2
    exit 1
  fi
done

cd "$root"
run() {
  env "$@" go test ./internal/remote/relaytest -run 'TestExternalRelay' -count=1 -v
}
run RELAY_CONFORMANCE_URL=ws://127.0.0.1:18761 RELAY_CONFORMANCE_PROFILE=main RELAY_CONFORMANCE_MAX_STREAMS=4 RELAY_CONFORMANCE_INTEROP=1
run RELAY_CONFORMANCE_URL=ws://127.0.0.1:18762 RELAY_CONFORMANCE_PROFILE=limits RELAY_CONFORMANCE_MAX_CONN_PER_IP=8 RELAY_CONFORMANCE_DAILY_BYTES=200000
run RELAY_CONFORMANCE_URL=ws://127.0.0.1:18763 RELAY_CONFORMANCE_PROFILE=disabled
echo "relay 一致性测试（wrangler dev）通过"
