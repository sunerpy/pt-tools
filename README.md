# pt-tools

`pt-tools` 是一个专为 PT 站点设计的自动化工具，通过 RSS 订阅自动下载免费种子并推送到 qBittorrent，帮助用户快速提升上传量。

## 功能特性

- **RSS 自动订阅**：自动解析 RSS 订阅，下载免费种子
- **多站点支持**：支持 M-Team、HDSky、CMCT 等站点
- **qBittorrent 集成**：自动推送种子到 qBittorrent
- **Web 管理界面**：通过 Web 页面统一管理配置
- **Docker 部署**：支持 Docker 一键部署

## 支持的站点

| 站点 | 认证方式 | 状态 |
|------|----------|------|
| M-Team | API Key | ✅ |
| HDSky | Cookie | ✅ |
| CMCT | Cookie | ✅ |

## 快速开始

### Docker 部署（推荐）

镜像地址： [Docker镜像](https://hub.docker.com/r/sunerpy/pt-tools)

> Docker镜像由github action 自动构建，如需手动构建，可参考[Makefile](Makefile) 中的`build-remote-docker`阶段由源码自行构建

查看示例：

- [examples/docker-run.md](examples/docker-run.md)：单容器运行、环境变量说明与数据持久化挂载（推荐）
- [examples/docker-compose.yml](examples/docker-compose.yml)：使用 Compose 编排，持久化数据库与下载目录（推荐）

```bash
docker run -d \
  --name pt-tools \
  -p 8080:8080 \
  -v ~/pt-data:/app/.pt-tools \
  -e PT_HOST=0.0.0.0 \
  -e PT_PORT=8080 \
  sunerpy/pt-tools:latest
```

### Docker Compose

```yaml
version: "3.8"
services:
  pt-tools:
    image: sunerpy/pt-tools:latest
    container_name: pt-tools
    environment:
      PT_HOST: "0.0.0.0"
      PT_PORT: "8080"
    ports:
      - "8080:8080"
    volumes:
      - ./data:/app/.pt-tools
    restart: unless-stopped
```

启动后访问 `http://localhost:8080` 进入 Web 管理界面。

**默认登录账号**：`admin` / `adminadmin`

## 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `PT_HOST` | Web 监听地址 | `0.0.0.0` |
| `PT_PORT` | Web 监听端口 | `8080` |
| `PT_ADMIN_RESET` | 重置管理员密码（设为 `1` 启用） | - |
| `PT_ADMIN_USER` | 管理员用户名 | `admin` |
| `PT_ADMIN_PASS` | 管理员密码 | `adminadmin` |
| `PUID` | 容器用户 ID | `1000` |
| `PGID` | 容器组 ID | `1000` |
| `TZ` | 时区 | `Asia/Shanghai` |

## Web 配置说明

### 全局设置

| 页面显示名称 | 说明 |
|-------------|------|
| 默认间隔(分钟) | RSS 任务的默认执行间隔，最小值为 5 分钟 |
| 种子下载目录 | 保存 `.torrent` 种子文件的目录。支持绝对路径或相对路径（相对于 `~/.pt-tools`）。**注意：此目录用于保存种子文件，并非 qBittorrent 中实际下载数据的保存路径** |
| 启用限速 | 是否启用下载速度限制，用于判断种子是否能在免费期内完成下载 |
| 下载限速(MB/s) | 预估的下载速度，用于计算种子是否能在免费期内完成 |
| 最大种子大小(GB) | 限制下载种子的最大体积，超过此大小的种子将被跳过 |
| 自动启动任务 | 开启后，程序启动时会自动运行所有已启用站点的 RSS 任务；关闭则需要手动点击"启动任务"按钮 |

### qBittorrent 设置

| 页面显示名称 | 说明 |
|-------------|------|
| 启用 | 是否启用 qBittorrent 客户端集成 |
| URL | qBittorrent WebUI 地址，如 `http://192.168.1.10:8080` |
| 用户 | qBittorrent 登录用户名 |
| 密码 | qBittorrent 登录密码 |

### 站点设置

| 页面显示名称 | 说明 |
|-------------|------|
| 启用 | 是否启用该站点的 RSS 任务 |
| 认证方式 | 站点的认证方式，M-Team 使用 `api_key`，HDSky/CMCT 使用 `cookie`（只读，由系统自动设置） |
| Cookie | Cookie 认证方式时必填，从浏览器开发者工具中获取 |
| API Key | API 认证方式时必填，从 M-Team 个人设置中获取 |
| API Url | API 地址（只读，由系统自动设置） |

### RSS 订阅

每个站点可配置多个 RSS 订阅：

| 页面显示名称 | 说明 |
|-------------|------|
| 名称 | RSS 订阅的标识名称，如 `CMCT电视剧` |
| 链接 | RSS 订阅地址，需以 `http://` 或 `https://` 开头 |
| 分类 | qBittorrent 中的分类标签，如 `Tv`、`Mv` |
| 标签 | 任务标签，用于区分不同来源，如 `CMCT`、`HDSKY` |
| 间隔(分钟) | 该 RSS 的执行间隔，范围 5-1440 分钟 |

## 数据持久化

所有数据存储在 `/app/.pt-tools` 目录：

- `torrents.db`：SQLite 数据库，存储配置和任务记录
- `downloads/`：种子文件下载目录

## 重置管理员密码

```bash
docker run -d \
  -e PT_ADMIN_RESET=1 \
  -e PT_ADMIN_USER=admin \
  -e PT_ADMIN_PASS='新密码' \
  -v ~/pt-data:/app/.pt-tools \
  -p 8080:8080 \
  sunerpy/pt-tools:latest
```

> 重置完成后移除 `PT_ADMIN_RESET` 环境变量

## 从源码构建

```bash
git clone https://github.com/sunerpy/pt-tools.git
cd pt-tools
go build -o pt-tools .
```

## 二进制下载

前往 [Releases 页面](https://github.com/sunerpy/pt-tools/releases) 下载适合你系统的预编译二进制文件：

| 系统 | 架构 | 文件名 |
|------|------|--------|
| Linux | amd64 | `pt-tools-linux-amd64.tar.gz` |
| Linux | arm64 | `pt-tools-linux-arm64.tar.gz` |
| Windows | amd64 | `pt-tools-windows-amd64.exe.zip` |
| Windows | arm64 | `pt-tools-windows-arm64.exe.zip` |

### Linux 运行

```bash
# 下载并解压
wget https://github.com/sunerpy/pt-tools/releases/latest/download/pt-tools-linux-amd64.tar.gz
tar -xzf pt-tools-linux-amd64.tar.gz

# 运行（默认启动 Web 服务）
./pt-tools web

# 或指定监听地址和端口
./pt-tools web --host 0.0.0.0 --port 8080
```

### Windows 运行

1. 下载 `pt-tools-windows-amd64.exe.zip` 并解压
2. 在命令行中执行：

```cmd
pt-tools.exe web --host 0.0.0.0 --port 8080
```

启动后访问 `http://localhost:8080` 进入 Web 管理界面。

## 许可证

[MIT License](LICENSE)
