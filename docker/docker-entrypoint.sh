#!/bin/sh
export LANG=en_US.utf-8
##########################################################################
#*    @File    :   docker-entrypoint.sh
#*    @Time    :   2025/07/02 14:08:27
#*    @Author  :   sunerpy
#*    @Version :   1.0
#*    @Contact :   sunerpy<nkuzhangshn@gmail.com>
#*    @Desc    :   None
#*    @Use     :   ~/workspace/ProdDir/pt-tools/docker/docker-entrypoint.sh

set -o nounset # 禁止引用未定义的变量
set -e         # 遇到错误就退出
#set -o errexit

LOGFILE=/tmp/docker-entrypoint.sh_$(date +%Y%m%d).log
touch "${LOGFILE}"
date >"${LOGFILE}"

logger() {
    TIMESTAMP=[$(date +'%Y-%m-%d %H:%M:%S')]
    case "$1" in
    debug)
        echo "$TIMESTAMP [DEBUG] $2" >>"${LOGFILE}"
        ;;
    info)
        echo "$2"
        echo "$TIMESTAMP [INFO]  $2" >>"${LOGFILE}"
        ;;
    warn)
        echo "$TIMESTAMP [WARN]  $2" >>"${LOGFILE}"
        ;;
    error)
        echo "$TIMESTAMP [ERROR] $2" | tee -a "${LOGFILE}"
        exit 1
        ;;
    *)
        echo "$TIMESTAMP Parameters wrong $2" | tee -a "${LOGFILE}"
        exit 1
        ;;
    esac
}

suCmd() {
    osuser=$1
    cmd=$2
    su - "${osuser}" -c "${cmd}"
}
checkEnv() {
    :
}
# 设置默认 UID 和 GID（从环境变量读取）
PUID=${PUID:-1000}
PGID=${PGID:-1000}

# 镜像里还没有这个 GID / UID 时建一个具名的组与用户；已经被占用时直接沿用（例如 Unraid、群晖常用的
# PGID=100 在 Alpine 里就是 users 组 —— 以前这里 addgroup 报「gid in use」，set -e 让容器直接退出）。
# 运行时按数字 ID 切换，不依赖名字，所以建不成具名的组或用户也不影响启动。
if ! getent group "$PGID" >/dev/null; then
    addgroup -g "$PGID" appgroup 2>/dev/null || true
fi
if ! getent passwd "$PUID" >/dev/null; then
    adduser -u "$PUID" -G "$(getent group "$PGID" | cut -d: -f1)" -h "$HOME" -D appuser 2>/dev/null || true
fi

# 修改/app 权限 忽略挂载的只读目录报错
chown -R "$PUID":"$PGID" /app 2>/dev/null || true

mainRunServer() {
    # if [ "$1" = 'pt-tools' ] && [ "$(id -u)" = '0' ]; then
    #     find . \! -user appuser -exec chown appuser '{}' +
    #     exec gosu django "$0" "$@"
    # fi
    # exec "$@" run -m persistent
    # 以目标用户运行应用（使用 exec 切换，避免启动残留 PID 1）
    HOST=${PT_HOST:-0.0.0.0}
    PORT=${PT_PORT:-8080}
    # PT_MODE=relay：运行远程访问的 relay（只做转发，不读写 /app/.pt-tools）；参数从 PT_TOOLS_RELAY_* 环境变量读，
    # 例如 PT_TOOLS_RELAY_PUBLIC_URL=wss://relay.example.com、PT_TOOLS_RELAY_LISTEN=0.0.0.0:8443
    if [ "${PT_MODE:-web}" = "relay" ]; then
        exec gosu "$PUID:$PGID" "$@" relay serve
    fi
    exec gosu "$PUID:$PGID" "$@" web --host "$HOST" --port "$PORT"
}
if [ "$#" -ne 1 ] && [ "$#" -ne 0 ]; then
    logger error "参数个数有误，请检查"
fi
alias cp='cp'
alias rm='rm'
alias mv='mv'

mainRunServer "$@"
