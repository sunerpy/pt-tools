FROM 107255705363.dkr.ecr.cn-northwest-1.amazonaws.com.cn/sunerpy/golang:1.23.1 AS builder
WORKDIR /app
ARG CONFIG_FILE
COPY cmd /app/cmd
COPY config /app/config
COPY core /app/core
COPY global /app/global
COPY internal /app/internal
COPY models /app/models
COPY thirdpart /app/thirdpart
COPY vendor /app/vendor
COPY go.* /app/
COPY *.go /app/
RUN go env -w GO111MODULE=on \
    && go env -w GOPROXY=https://goproxy.cn,direct \
    && go env -w CGO_ENABLED=0 \
    && go env \
    && go build -ldflags="-s -w" -mod=vendor -o pt-tools .

# 二阶段构建
FROM 107255705363.dkr.ecr.cn-northwest-1.amazonaws.com.cn/official/alpine:3.20.3
LABEL MAINTAINER="nkuzhangshn@gmail.com"
ARG CONFIG_ENV
ENV TZ=Asia/Shanghai ENV=${CONFIG_ENV}  ENVIRONMENT=${CONFIG_ENV}
WORKDIR /app
RUN addgroup -S appgroup && adduser -S -u 1000 -G appgroup appuser && chown -R appuser:appgroup /app
USER appuser
COPY --from=builder /app/pt-tools ./
EXPOSE 12024
ENTRYPOINT ["./pt-tools","-c", "/app/config.yaml"]