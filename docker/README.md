# Docker 镜像使用说明

本镜像提供 `pt-tools`，用于自动化管理PT站点的相关任务。

## 支持的架构

目前提供的镜像基于 `alpine` 构建，可在 `linux/amd64` 等常见架构上运行。

## 快速开始

### 使用 Docker CLI

```bash
docker run -d \
  --name=pt-tools \
  -v /path/to/config.toml:/app/config/config.toml:ro \
  -v /path/to/pt-data:/app/.pt-tools:rw \
  -e TZ=Asia/Shanghai \
  sunerpy/pt-tools:latest
```

### 使用 Docker Compose

```yaml
version: '3.3'
services:
  pt-tools:
    image: sunerpy/pt-tools:latest
    container_name: pt-tools
    environment:
      - TZ=Asia/Shanghai
    volumes:
      - /path/to/config.toml:/app/config/config.toml:ro
      - /path/to/pt-data:/app/.pt-tools
    restart: unless-stopped
```

## 参数说明

| 参数/挂载点 | 作用 |
|-------------|------|
| `TZ` | 设置容器时区，默认为 `Asia/Shanghai` |
| `/app/config/config.toml` | 程序配置文件，需在宿主机创建并挂载到该路径 |
| `/app/.pt-tools` | 程序工作目录，存放数据库等数据 |

## 更新镜像

```bash
docker pull sunerpy/pt-tools:latest
docker stop pt-tools
docker rm pt-tools
# 重新以相同参数启动
```

## 进入容器

```bash
docker exec -it pt-tools -- /bin/bash
```

更多使用细节请参阅项目 [README](../README.md)。
