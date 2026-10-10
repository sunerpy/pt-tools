#!/usr/bin/env bash
# 在托管 relay 的服务器上部署或升级 relay 容器（Docker）。.github/workflows/relay-deploy.yml 经 ssh 把它送过去执行
# （ssh host 'bash -s -- <参数>' < scripts/deploy-relay.sh），也可以在服务器上直接跑。
#
#   deploy-relay.sh --image sunerpy/pt-tools@sha256:… --public-url wss://relay.example.com --expect-version v1.0.0-rc.19 \
#     [--port 8443] [--name pt-tools-relay] [--drain-grace 5s] [--client-ip-header X-Real-IP] \
#     [--max-connections N] [--daily-bytes-per-host N] [--ready-timeout 60] [--no-pull] [--keep-previous]
#   deploy-relay.sh --finalize [--name pt-tools-relay]   # 外面检查通过：删掉留着的旧容器
#   deploy-relay.sh --rollback [--name pt-tools-relay] [--port 8443] [--ready-timeout 60]   # 外面检查没过：换回旧容器
#
# 部署：拉镜像 → 停旧容器（docker stop：relay 先不接新连接，等 drain-grace，再以 1012 关掉所有连接，pt-tools 几秒内重连）并改名成 <name>-previous
# → 用原来的名字起新容器（PT_MODE=relay，只听 127.0.0.1:<端口>，前面的反向代理做 TLS 并把 WebSocket 转过来）→ 等 /ready 回 200、/healthz 的版本对。
# 这一段里任何一步失败、或者脚本被中断（包括 ssh 断开），都删掉新容器、把旧容器原样起回来，以失败退出。
# 成功以后默认删掉旧容器；--keep-previous 时留着（停着的），等调用方从外面检查完再 --finalize 或 --rollback。
# 上一次部署没有收尾（还有 <name>-previous）时拒绝再部署，先 --finalize 或 --rollback。
# --no-pull 用本机已有的镜像（自己构建的、或者离线的服务器）。
set -euo pipefail

mode=deploy image="" public_url="" port=8443 name=pt-tools-relay drain_grace=5s client_ip_header=""
max_connections="" daily_bytes="" expect_version="" ready_timeout=60 pull=1 keep_previous=0

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
  --keep-previous) keep_previous=1; shift ;;
  --finalize) mode=finalize; shift ;;
  --rollback) mode=rollback; shift ;;
  -h | --help) sed -n '2,16p' "${BASH_SOURCE[0]}"; exit 0 ;;
  *) die "不认识的参数 $1" ;;
  esac
done

[[ "$name" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] || die "--name 不对：$name"
[[ "$port" =~ ^[0-9]+$ ]] && [ "$port" -ge 1 ] && [ "$port" -le 65535 ] || die "--port 不对：$port"
[[ "$ready_timeout" =~ ^[0-9]+$ ]] || die "--ready-timeout 是秒数"
command -v docker >/dev/null || die "这台机器上没有 docker"
command -v curl >/dev/null || die "这台机器上没有 curl"
previous="$name-previous"

exists() { docker inspect "$1" >/dev/null 2>&1; }

# wait_ready 等 127.0.0.1:<端口> 的 /ready 回 200（最多 ready_timeout 秒）；容器退出了、或者被重启策略拉起来过就算失败
wait_ready() {
  local deadline=$((SECONDS + ready_timeout))
  until curl -fsS --noproxy '*' -o /dev/null "http://127.0.0.1:$port/ready" 2>/dev/null; do
    if [ "$SECONDS" -ge "$deadline" ]; then
      return 1
    fi
    if [ "$(docker inspect -f '{{.State.Running}} {{.RestartCount}}' "$name" 2>/dev/null)" != "true 0" ]; then
      return 1
    fi
    sleep 1
  done
}

# restore 删掉（新的）<name>，把 <name>-previous 改回原名起来；就绪时返回 0
restore() {
  docker rm -f "$name" >/dev/null 2>&1 || true
  exists "$previous" || return 1
  docker rename "$previous" "$name"
  docker start "$name" >/dev/null
  wait_ready
}

case "$mode" in
finalize)
  if exists "$previous"; then
    docker rm "$previous" >/dev/null
    echo "deploy-relay: 删掉了旧容器 $previous"
  else
    echo "deploy-relay: 没有要删的旧容器"
  fi
  exit 0
  ;;
rollback)
  exists "$previous" || die "没有 $previous，换不回去"
  if restore; then
    echo "deploy-relay: 已经换回旧容器（$(docker inspect -f '{{.Config.Image}}' "$name")）"
    exit 0
  fi
  die "旧容器起来了但没有就绪，请登录服务器检查"
  ;;
esac

[ -n "$image" ] || die "要用 --image 给出镜像，例如 sunerpy/pt-tools:v1.0.0-rc.19 或 sunerpy/pt-tools@sha256:…"
case "$public_url" in
ws://* | wss://*) ;;
*) die "--public-url 要以 ws:// 或 wss:// 开头，是 App 与 pt-tools 里填的地址" ;;
esac
[[ "$drain_grace" =~ ^[0-9]+s$ ]] || die "--drain-grace 写成秒数，例如 5s"
# 不给 --expect-version 时按镜像标签核对版本（digest 写法、latest 之类没有版本含义的标签不核对）
if [ -z "$expect_version" ] && [[ "$image" != *@* ]]; then
  tag=${image##*:}
  case "$tag" in v[0-9]*) expect_version=$tag ;; esac
fi
if exists "$previous"; then
  die "上一次部署没有收尾（还有 $previous）：先用 --finalize（新容器没问题）或 --rollback（换回旧容器）"
fi

grace_s=${drain_grace%s}
stop_timeout=$((grace_s + 10))
run_args=(
  -d --name "$name" --restart unless-stopped --stop-timeout "$stop_timeout"
  -p "127.0.0.1:$port:8443"
  -e PT_MODE=relay -e "PT_TOOLS_RELAY_PUBLIC_URL=$public_url" -e "PT_TOOLS_RELAY_DRAIN_GRACE=$drain_grace"
)
[ -z "$client_ip_header" ] || run_args+=(-e "PT_TOOLS_RELAY_CLIENT_IP_HEADER=$client_ip_header")
[ -z "$max_connections" ] || run_args+=(-e "PT_TOOLS_RELAY_MAX_CONNECTIONS=$max_connections")
[ -z "$daily_bytes" ] || run_args+=(-e "PT_TOOLS_RELAY_DAILY_BYTES_PER_HOST=$daily_bytes")

if [ "$pull" = 1 ]; then
  echo "deploy-relay: 拉取 $image"
  docker pull -q "$image" >/dev/null
fi
docker image inspect "$image" >/dev/null 2>&1 || die "本机没有镜像 $image"

# 从停旧容器到新容器核对完：出错、被中断都换回旧容器。stage 记着走到哪一步，回滚按它来：
#   1 = 正在停旧容器（它还叫 <name>）：把它重新起来；
#   2 = 旧容器已经改名成 <name>-previous（新容器可能已经叫 <name>）：删掉新的，把旧的改回原名起来。
stage=0
abort() {
  local rc=$?
  trap - ERR HUP INT TERM EXIT
  case "$stage" in
  0) exit "$rc" ;;
  1)
    echo "deploy-relay: 停旧容器时中断了，把它重新起来" >&2
    docker start "$name" >/dev/null 2>&1 || true
    if wait_ready; then
      echo "deploy-relay: 旧容器已经重新起来（$(docker inspect -f '{{.Config.Image}}' "$name")）" >&2
    else
      echo "deploy-relay: 旧容器没有就绪，请登录服务器检查" >&2
    fi
    exit 1
    ;;
  esac
  echo "deploy-relay: 部署没有完成，日志：" >&2
  docker logs --tail 30 "$name" 2>&1 | sed 's/^/  /' >&2 || true
  if restore; then
    echo "deploy-relay: 已经换回原来的容器（$(docker inspect -f '{{.Config.Image}}' "$name")）" >&2
  elif exists "$name"; then
    echo "deploy-relay: 原来的容器起来了但没有就绪，请登录服务器检查" >&2
  else
    echo "deploy-relay: 这是第一次部署，没有可以换回的容器" >&2
  fi
  exit 1
}
trap abort ERR HUP INT TERM EXIT

if exists "$name"; then
  echo "deploy-relay: 停止旧容器（$(docker inspect -f '{{.Config.Image}}' "$name")，排空 $drain_grace）"
  stage=1
  docker stop -t "$stop_timeout" "$name" >/dev/null
  docker rename "$name" "$previous"
fi
stage=2

echo "deploy-relay: 启动新容器"
docker run "${run_args[@]}" "$image" >/dev/null
if ! wait_ready; then
  echo "deploy-relay: 新容器没有就绪" >&2
  false
fi
if [ -n "$expect_version" ]; then
  health=$(curl -fsS --noproxy '*' "http://127.0.0.1:$port/healthz")
  version=$(printf '%s' "$health" | sed -n 's/.*"version":"\([^"]*\)".*/\1/p')
  if [ "$version" != "$expect_version" ]; then
    echo "deploy-relay: /healthz 的版本是 ${version:-（读不到）}，不是 $expect_version" >&2
    false
  fi
fi

stage=0
trap - ERR HUP INT TERM EXIT
if [ "$keep_previous" = 1 ]; then
  if exists "$previous"; then
    echo "deploy-relay: 旧容器留作 $previous（停着），外面检查完以后 --finalize 或 --rollback"
  fi
else
  docker rm "$previous" >/dev/null 2>&1 || true
fi
echo "deploy-relay: 完成：$name 运行 $image，听 127.0.0.1:$port，对外地址 $public_url"
