# 安装

本页介绍用 Docker、Linux 二进制和 Windows 二进制安装 pt-tools 的步骤，以及首次登录和数据目录的说明。

| 方式           | 适用系统                 | 说明                                            |
| -------------- | ------------------------ | ----------------------------------------------- |
| Docker Compose | 任何能运行 Docker 的系统 | 推荐。升级只需要拉取新镜像                      |
| `docker run`   | 同上                     | 单条命令，适合临时试用                          |
| Linux 二进制   | Linux amd64、arm64       | 安装脚本会校验 SHA-256；可以注册为 systemd 服务 |
| Windows 二进制 | Windows amd64、arm64     | PowerShell 安装脚本或手动解压                   |

不提供 macOS 二进制，macOS 请使用 Docker。

## Docker Compose（推荐）

把下面的内容保存为 `compose.yml`：

```yaml
services:
  pt-tools:
    image: sunerpy/pt-tools:latest
    container_name: pt-tools
    environment:
      PT_HOST: "0.0.0.0"
      PT_PORT: "8080"
      TZ: "Asia/Shanghai"
    ports:
      - "8080:8080"
    volumes:
      - ./data:/app/.pt-tools
    restart: unless-stopped
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"
```

然后启动：

```bash
docker compose up -d
```

> [!IMPORTANT]
> 必须持久化 `/app/.pt-tools`。这个目录包含数据库、配置和用于加密站点凭证的 `secret.key`；删除或丢失密钥后，已保存的 Cookie 无法恢复。

`logging` 一节限制容器标准输出日志的大小。不设置时，长时间运行会让 Docker 的日志文件持续增长，NAS 用户尤其需要注意，详见[配置说明](../configuration.md)。

镜像同时发布在 Docker Hub（`sunerpy/pt-tools`）和 GHCR（`ghcr.io/sunerpy/pt-tools`，附带构建证明），两者内容相同。

## docker run

```bash
docker run -d \
  --name pt-tools \
  -p 8080:8080 \
  -v ~/pt-data:/app/.pt-tools \
  -e TZ=Asia/Shanghai \
  sunerpy/pt-tools:latest
```

### 常用环境变量

| 变量             | 说明                                    | 默认值          |
| ---------------- | --------------------------------------- | --------------- |
| `PT_HOST`        | 监听地址                                | `0.0.0.0`       |
| `PT_PORT`        | 监听端口                                | `8080`          |
| `TZ`             | 时区                                    | `Asia/Shanghai` |
| `PUID`、`PGID`   | 容器内运行程序的用户 ID 和组 ID         | `1000`          |
| `PT_ADMIN_USER`  | 初始管理员用户名                        | `admin`         |
| `PT_ADMIN_PASS`  | 初始管理员密码                          | `adminadmin`    |
| `PT_ADMIN_RESET` | 设为 `1` 时按上面两个变量重置管理员密码 | 未设置          |
| `HTTP_PROXY` 等  | 代理，见[配置说明](../configuration.md) | 未设置          |

## Linux 二进制

### 用安装脚本

```bash
curl -fsSL https://raw.githubusercontent.com/sunerpy/pt-tools/main/scripts/install.sh | sh
```

脚本会下载最新版本的压缩包和同一版本的 `checksums.txt`，SHA-256 不一致时拒绝安装。默认装到 `~/.local/bin`，可以用环境变量调整：

- `TOOL_VERSION`：安装指定版本，例如 `TOOL_VERSION=v0.48.0`。
- `TOOL_INSTALL_DIR`：安装目录。

每个版本的发布说明里都有固定到该版本的安装命令，可以直接复制使用。

### 手动下载

从[发布页面](https://github.com/sunerpy/pt-tools/releases)下载对应架构的压缩包和 `checksums.txt`，先校验再解压：

```bash
sha256sum -c checksums.txt --ignore-missing
tar -xzf pt-tools-linux-amd64.tar.gz
chmod +x pt-tools
./pt-tools web --host 0.0.0.0 --port 8080
```

如果需要确认压缩包由本项目的发布流水线构建，可以用 GitHub CLI 校验构建证明：

```bash
gh attestation verify pt-tools-linux-amd64.tar.gz --repo sunerpy/pt-tools \
  --signer-workflow sunerpy/pt-tools/.github/workflows/release.yml \
  --deny-self-hosted-runners
```

### 注册为 systemd 服务

创建 `/etc/systemd/system/pt-tools.service`，把 `User` 和路径换成你自己的：

```ini
[Unit]
Description=pt-tools
After=network.target

[Service]
Type=simple
User=your_username
WorkingDirectory=/home/your_username
ExecStart=/usr/local/bin/pt-tools web --host 0.0.0.0 --port 8080
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now pt-tools
sudo journalctl -u pt-tools -f
```

数据保存在运行用户的 `~/.pt-tools` 目录下。

## Windows 二进制

在 PowerShell 中运行：

```powershell
irm https://raw.githubusercontent.com/sunerpy/pt-tools/main/scripts/install.ps1 | iex
```

需要安装指定版本时，先设置 `$env:TOOL_VERSION = "v0.48.0"` 再运行。也可以从[发布页面](https://github.com/sunerpy/pt-tools/releases)下载 `pt-tools-windows-amd64.exe.zip`，解压后运行：

```powershell
.\pt-tools.exe web --host 0.0.0.0 --port 8080
```

数据保存在用户目录下的 `.pt-tools` 文件夹中。

## 首次登录

打开 `http://服务器地址:8080`，使用初始账号登录：用户名 `admin`，密码 `adminadmin`。如果启动时设置了 `PT_ADMIN_USER` 和 `PT_ADMIN_PASS`，以这两个变量为准。

登录后请立即在「修改密码」中更换密码。忘记密码时，按[配置说明](../configuration.md)中「重置管理员密码」一节操作。

> [!WARNING]
> 不要把 pt-tools 直接暴露在公网上。需要远程访问时，请放在启用 HTTPS 的反向代理之后，或者通过 VPN 访问。

## 数据目录

所有方式的数据目录结构相同：

```text
.pt-tools/
├── secret.key     # 凭证加密密钥，必须与数据库一起备份
├── torrents.db    # SQLite 数据库：配置、任务记录和用户数据缓存
├── downloads/     # 暂存的 .torrent 文件
├── backups/       # 自动生成的历史配置备份
└── logs/          # 轮转日志
```

备份和恢复的方法见[升级与备份](upgrade.md)。

## 下一步

安装完成后，按[快速开始](quick-start.md)添加下载器、站点和第一个 RSS 订阅。
