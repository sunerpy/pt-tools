# 参数区分构建环境与基础镜像来源
ARG BUILD_IMAGE=golang:1.26.10
ARG BASE_IMAGE=alpine:3.20.3
ARG NODE_IMAGE=node:25.2.0-alpine
ARG BUILD_ENV=local

# 阶段0：前端构建阶段
FROM ${NODE_IMAGE} AS frontend-builder
WORKDIR /app/web/frontend
RUN npm install -g pnpm@10.25.0
COPY web/frontend/package.json web/frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/frontend/ ./
RUN pnpm build

# 阶段1：Go 构建阶段
FROM ${BUILD_IMAGE} AS builder
WORKDIR /app

# 构建参数
ARG BUILD_ENV
ARG TAG=unknown
ARG BUILD_TIME=unknown
ARG COMMIT_ID=unknown
# 托管 relay 的地址（docker.yml 从 secret PT_TOOLS_RELAY_URL 传进来；地址随二进制公开，build-arg 会出现在镜像历史里），没给时为空
ARG RELAY_URL=""
ARG HTTP_PROXY HTTPS_PROXY NO_PROXY
ARG TARGETARCH
ARG GOPROXY

# 依赖图由仓库提交的 go.mod/go.sum 锁定。构建期间禁止 go mod tidy，
# 避免上游最新版本改变已打 tag 的依赖图。
COPY go.* /app/
RUN --mount=type=bind,source=.,target=/context,ro set -eux; \
  if [ ! -f /context/dist/pt-tools-linux-${TARGETARCH} ]; then \
    if [ -n "${GOPROXY}" ]; then \
      GOPROXY="${GOPROXY}" go mod download; \
    elif [ "${BUILD_ENV}" = "local" ]; then \
      GOPROXY=https://goproxy.cn,direct go mod download; \
    else \
      go mod download; \
    fi; \
  fi
# 拷贝项目代码
COPY cmd /app/cmd
COPY config /app/config
COPY core /app/core
COPY global /app/global
COPY internal /app/internal
COPY models /app/models
COPY scheduler /app/scheduler
COPY web /app/web
COPY site /app/site
COPY thirdpart /app/thirdpart
COPY utils /app/utils
COPY version /app/version
COPY *.go /app/

# 从前端构建阶段拷贝构建产物
COPY --from=frontend-builder /app/web/static/dist /app/web/static/dist

# 构建或接受外部二进制，并统一执行 upx 压缩。
# 内置的 TMDB API Key 用 BuildKit secret 传（docker.yml 从 secret PT_TOOLS_TMDB_KEY 给 id=tmdb_key），不用 build-arg：
# build-arg 会写进镜像历史与 provenance。没给时为空，用户要自己填。编译这一步关掉 -x，免得 Key 打进构建日志。
RUN --mount=type=bind,source=.,target=/context,ro --mount=type=secret,id=tmdb_key set -eux; \
  if [ -f /context/dist/pt-tools-linux-${TARGETARCH} ]; then \
    echo "Using provided binary from dist for ${TARGETARCH}"; \
    cp /context/dist/pt-tools-linux-${TARGETARCH} /app/pt-tools && chmod +x /app/pt-tools; \
  else \
    go env -w CGO_ENABLED=0; \
    go env; \
    set +x; \
    TMDB_KEY=""; \
    if [ -f /run/secrets/tmdb_key ]; then TMDB_KEY=$(cat /run/secrets/tmdb_key); fi; \
    go build -ldflags="-s -w \
      -X github.com/sunerpy/pt-tools/version.Version=${TAG} \
      -X github.com/sunerpy/pt-tools/version.BuildTime=${BUILD_TIME} \
      -X github.com/sunerpy/pt-tools/version.CommitID=${COMMIT_ID} \
      -X github.com/sunerpy/pt-tools/internal/remote.DefaultRelayURL=${RELAY_URL} \
      -X github.com/sunerpy/pt-tools/internal/media/tmdb.BuiltinKey=${TMDB_KEY}" \
      -mod=readonly -o pt-tools; \
    set -x; \
  fi; \
  if command -v apt-get >/dev/null 2>&1; then apt-get update && apt-get install -y --no-install-recommends upx-ucl || true; fi; \
  if command -v apk >/dev/null 2>&1; then apk add --no-cache upx || true; fi; \
  if command -v yum >/dev/null 2>&1; then yum install -y upx || true; fi; \
  upx -9 /app/pt-tools || true

# 阶段2：运行阶段
FROM ${BASE_IMAGE}
LABEL MAINTAINER="nkuzhangshn@gmail.com"
ARG HTTP_PROXY HTTPS_PROXY NO_PROXY
WORKDIR /app
ENV PUID=1000 PGID=1000
ENV TZ=Asia/Shanghai PATH=$PATH:/app/bin HOME=/app
ENV PT_HOST=0.0.0.0 PT_PORT=8080

# 从构建阶段拷贝二进制文件
COPY --from=builder /app/pt-tools /app/bin/pt-tools
COPY --chown=1000:1000 --chmod=755 docker/docker-entrypoint.sh /app/bin/

# 创建 Docker 环境标记文件（用于运行时检测是否在容器中）
RUN echo -n "pt-tools-docker-build" > /app/.pt-tools-docker

ENV GOSU_VERSION=1.17
RUN set -eux; \
	\
	apk add --no-cache tzdata ca-certificates; \
	\
	apk add --no-cache --virtual .gosu-deps \
	dpkg \
	gnupg \
	; \
	\
	dpkgArch="$(dpkg --print-architecture | awk -F- '{ print $NF }')"; \
	wget -O /usr/local/bin/gosu "https://github.com/tianon/gosu/releases/download/$GOSU_VERSION/gosu-$dpkgArch"; \
	wget -O /usr/local/bin/gosu.asc "https://github.com/tianon/gosu/releases/download/$GOSU_VERSION/gosu-$dpkgArch.asc"; \
	\
	# verify the signature
	export GNUPGHOME="$(mktemp -d)"; \
	gpg --batch --keyserver hkps://keys.openpgp.org --recv-keys B42F6819007F00F88E364FD4036A9C25BF357DD4; \
	gpg --batch --verify /usr/local/bin/gosu.asc /usr/local/bin/gosu; \
	gpgconf --kill all; \
	rm -rf "$GNUPGHOME" /usr/local/bin/gosu.asc; \
	\
	# clean up fetch dependencies
	apk del --no-network .gosu-deps; \
	\
	chmod +x /usr/local/bin/gosu; \
	# verify that the binary works
	gosu --version; \
	gosu nobody true

# 创建用户 转移到初始化脚本中
# USER appuser


# 设置启动命令
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["pt-tools"]
