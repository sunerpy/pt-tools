# 参数区分构建环境与基础镜像来源
ARG BUILD_IMAGE=golang:1.24.3
ARG BASE_IMAGE=alpine:3.20.3
ARG BUILD_ENV=local

# 阶段1：构建阶段
FROM ${BUILD_IMAGE} AS builder
WORKDIR /app

# 构建参数
ARG BUILD_ENV
ARG CONFIG_FILE
ARG TAG=unknown
ARG BUILD_TIME=unknown
ARG COMMIT_ID=unknown
ARG HTTP_PROXY HTTPS_PROXY NO_PROXY

# 仅在远程构建时执行 go mod vendor
COPY go.* /app/
RUN if [ "$BUILD_ENV" = "remote" ]; then \
	go env -w GO111MODULE=on \
	&& go mod tidy && go mod vendor; \
	fi
# 拷贝项目代码
COPY cmd /app/cmd
COPY config /app/config
COPY core /app/core
COPY global /app/global
COPY internal /app/internal
COPY models /app/models
COPY site /app/site
COPY thirdpart /app/thirdpart
COPY utils /app/utils
COPY vendor /app/vendor
COPY version /app/version
COPY *.go /app/
COPY dist /app/dist

# 构建二进制文件
RUN if [ -f /app/dist/pt-tools-linux-amd64 ]; then \
	echo "Binary already exists. Skipping build and moving the file."; \
	mv /app/dist/pt-tools-linux-amd64 /app/pt-tools && chmod +x /app/pt-tools; \
	else \
	if [ "$BUILD_ENV" = "local" ]; then \
	go env -w GOPROXY=https://goproxy.cn,direct; \
	fi && \
	go env -w CGO_ENABLED=0 && \
	go env && \
	go mod tidy && \
	go mod vendor && \
	go build -ldflags="-s -w \
	-X github.com/sunerpy/pt-tools/version.Version=${TAG} \
	-X github.com/sunerpy/pt-tools/version.BuildTime=${BUILD_TIME} \
	-X github.com/sunerpy/pt-tools/version.CommitID=${COMMIT_ID}" \
	-mod=vendor -o pt-tools; \
	fi

# 拷贝配置文件
# COPY "config/${CONFIG_FILE}" /app/config.toml

# 阶段2：运行阶段
FROM ${BASE_IMAGE}
LABEL MAINTAINER="nkuzhangshn@gmail.com"
ARG HTTP_PROXY HTTPS_PROXY NO_PROXY
WORKDIR /app
ENV PUID=1000 PGUID=1000
ENV TZ=Asia/Shanghai PATH=$PATH:/app/bin HOME=/app

# 从构建阶段拷贝二进制文件
COPY --from=builder /app/pt-tools /app/bin/pt-tools
COPY --chown=1000:1000 --chmod=755 docker/docker-entrypoint.sh /app/bin/
ENV GOSU_VERSION 1.17
RUN set -eux; \
	\
	apk add --no-cache --virtual .gosu-deps \
	ca-certificates \
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
