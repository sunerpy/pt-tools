#!/usr/bin/env bash
# 在托管 relay 的服务器上部署或升级 relay 容器（Docker）。.github/workflows/relay-deploy.yml 经 ssh 把它送过去执行
# （ssh host bash -s -- <参数> < scripts/deploy-relay.sh），也可以在服务器上直接跑。
#
#   deploy-relay.sh --image sunerpy/pt-tools:v1.0.0-rc.19 --public-url wss://relay.example.com \
#     [--port 8443] [--name pt-tools-relay] [--drain-grace 5s] [--client-ip-header X-Real-IP] \
#     [--max-connections N] [--daily-bytes-per-host N] [--expect-version v1.0.0-rc.19] [--ready-timeout 60] [--no-pull]
#
# --no-pull 用本机已有的镜像（自己构建的、或者离线的服务器）。
# 步骤：拉新镜像 → 停旧容器（docker stop：relay 先不接新连接，等 drain-grace，再以 1012 关掉所有连接，pt-tools 几秒内重连）
# → 用同一个名字起新容器（PT_MODE=relay，只听 127.0.0.1:<端口>，前面的反向代理做 TLS 并把 WebSocket 转过来）
# → 等 /ready 回 200、/healthz 的版本与预期一致 → 删掉旧容器。新容器起不来时删掉它、把旧容器原样起回来，以失败退出。
set -euo pipefail

image="" public_url="" port=8443 name=pt-tools-relay drain_grace=5s client_ip_header=""
max_connections="" daily_bytes="" expect_version="-" ready_timeout=60 pull=1

die() {
  echo "deploy-relay: $*" >&2
  exit 1
}

while [ $# -gt 0 ]; do
  case "$1" in
  --image) image=${2:?}; shift 2 ;;
  --public-url) public_url=${2:?}; shift 2 ;;
  --port) port=${2:?}; shift 2 ;;
  --name) name=${2:?}; shift 2 ;;
  --drain-grace) drain_grace=${2:?}; shift 2 ;;
  --client-ip-header) client_ip_header=${2:?}; shift 2 ;;
  --max-connections) max_connections=${2:?}; shift 2 ;;
  --daily-bytes-per-host) daily_bytes=${2:?}; shift 2 ;;
  --expect-version) expect_version=${2?}; shift 2 ;;
  --ready-timeout) ready_timeout=${2:?}; shift 2 ;;
  --no-pull) pull=0; shift ;;
  -h | --help) sed -n '2,15p' "$0"; exit 0 ;;
  *) die "不认识的参数 $1" ;;
  esac
done

[ -n "$image" ] || die "要用 --image 给出镜像，例如 sunerpy/pt-tools:v1.0.0-rc.19"
case "$public_url" in
ws://* | wss://*) ;;
*) die "--public-url 要以 ws:// 或 wss:// 开头，是 App 与 pt-tools 里填的地址" ;;
esac
[[ "$port" =~ ^[0-9]+$ ]] && [ "$port" -ge 1 ] && [ "$port" -le 65535 ] || die "--port 不对：$port"
[[ "$name" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] || die "--name 不对：$name"
[[ "$drain_grace" =~ ^[0-9]+s$ ]] || die "--drain-grace 写成秒数，例如 5s"
[[ "$ready_timeout" =~ ^[0-9]+$ ]] || die "--ready-timeout 是秒数"
# 不给 --expect-version 时按镜像标签核对版本（latest 之类没有版本含义的标签不核对）
if [ "$expect_version" = "-" ]; then
  tag=${image##*:}
  case "$tag" in v[0-9]*) expect_version=$tag ;; *) expect_version="" ;; esac
fi
command -v docker >/dev/null || die "这台机器上没有 docker"
command -v curl >/dev/null || die "这台机器上没有 curl"

grace_s=${drain_grace%s}
stop_timeout=$((grace_s + 10))
previous="$name-previous"

run_args=(
  -d --name "$name" --restart unless-stopped --stop-timeout "$stop_timeout"
  -p "127.0.0.1:$port:8443"
  -e PT_MODE=relay -e "PT_TOOLS_RELAY_PUBLIC_URL=$public_url" -e "PT_TOOLS_RELAY_DRAIN_GRACE=$drain_grace"
)
[ -z "$client_ip_header" ] || run_args+=(-e "PT_TOOLS_RELAY_CLIENT_IP_HEADER=$client_ip_header")
[ -z "$max_connections" ] || run_args+=(-e "PT_TOOLS_RELAY_MAX_CONNECTIONS=$max_connections")
[ -z "$daily_bytes" ] || run_args+=(-e "PT_TOOLS_RELAY_DAILY_BYTES_PER_HOST=$daily_bytes")

# wait_ready 等 127.0.0.1:<端口> 的 /ready 回 200（最多 ready_timeout 秒）
wait_ready() {
  local deadline=$((SECONDS + ready_timeout))
  until curl -fsS --noproxy '*' -o /dev/null "http://127.0.0.1:$port/ready" 2>/dev/null; do
    if [ "$SECONDS" -ge "$deadline" ]; then
      return 1
    fi
    # 退出了，或者已经被重启策略拉起来过（起来就退出的容器会在 Running 与重启之间来回跳）
    if [ "$(docker inspect -f '{{.State.Running}} {{.RestartCount}}' "$name" 2>/dev/null)" != "true 0" ]; then
      return 1
    fi
    sleep 1
  done
}

rollback() {
  echo "deploy-relay: 新容器没有就绪，日志：" >&2
  docker logs --tail 30 "$name" >&2 || true
  docker rm -f "$name" >/dev/null 2>&1 || true
  if docker inspect "$previous" >/dev/null 2>&1; then
    docker rename "$previous" "$name"
    docker start "$name" >/dev/null
    if wait_ready; then
      echo "deploy-relay: 已经换回原来的容器（$(docker inspect -f '{{.Config.Image}}' "$name")）" >&2
    else
      echo "deploy-relay: 原来的容器也没有就绪，请登录服务器检查" >&2
    fi
  fi
  exit 1
}

if [ "$pull" = 1 ]; then
  echo "deploy-relay: 拉取 $image"
  docker pull -q "$image" >/dev/null
fi
docker image inspect "$image" >/dev/null 2>&1 || die "本机没有镜像 $image"


docker rm -f "$previous" >/dev/null 2>&1 || true
if docker inspect "$name" >/dev/null 2>&1; then
  echo "deploy-relay: 停止旧容器（$(docker inspect -f '{{.Config.Image}}' "$name")，排空 $drain_grace）"
  docker stop -t "$stop_timeout" "$name" >/dev/null
  docker rename "$name" "$previous"
fi

echo "deploy-relay: 启动新容器"
docker run "${run_args[@]}" "$image" >/dev/null || rollback
wait_ready || rollback

if [ -n "$expect_version" ]; then
  version=$(curl -fsS --noproxy '*' "http://127.0.0.1:$port/healthz" | sed -n 's/.*"version":"\([^"]*\)".*/\1/p')
  if [ "$version" != "$expect_version" ]; then
    echo "deploy-relay: /healthz 的版本是 ${version:-（读不到）}，不是 $expect_version" >&2
    rollback
  fi
fi

docker rm "$previous" >/dev/null 2>&1 || true
echo "deploy-relay: 完成：$name 运行 $image，听 127.0.0.1:$port，对外地址 $public_url"
