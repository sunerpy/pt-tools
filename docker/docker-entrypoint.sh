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
touch ${LOGFILE}
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
    # 必要的配置文件检查
    ls /app/config -al
    cat /app/config/config.toml
    if [ ! -f "/app/config/config.toml" ]; then
        logger error "❌ 配置文件 /app/config/config.toml 不存在，请通过挂载 config.toml 传入配置。"
    fi
}

mainRunServer() {
    checkEnv

    # if [ "$1" = 'pt-tools' ] && [ "$(id -u)" = '0' ]; then
    #     find . \! -user appuser -exec chown appuser '{}' +
    #     exec gosu django "$0" "$@"
    # fi
    exec "$@" -c /app/config/config.toml run -m persistent
}
if [ "$#" -ne 1 ] && [ "$#" -ne 0 ]; then
    logger error "参数个数有误，请检查"
fi
alias cp='cp'
alias rm='rm'
alias mv='mv'

mainRunServer "$@"
