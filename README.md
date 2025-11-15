# pt-tools

`pt-tools` 是一个专为PT站点设计的命令行工具，通过rss订阅链接帮助用户自动化处理与 PT 站点相关的任务，支持通过rss订阅来下载免费种子，帮助新用户以及老用户提高上传量，快速通过考核。

## 功能特性

* **灵活的运行模式**：支持单次执行和持续监控模式。
* **Shell 补全支持**：提供 Bash 和 Zsh 的补全功能。
* **Web 配置管理**：通过 Web 页面统一管理与保存配置。
* **数据库管理**：支持数据库初始化、备份和查询等操作。

---

## 安装方法

### 推荐：使用 Docker 运行（不推荐二进制运行）

查看示例：

- [examples/docker-run.md](examples/docker-run.md)：单容器运行、环境变量说明与数据持久化挂载（推荐）
- [examples/docker-compose.yml](examples/docker-compose.yml)：使用 Compose 编排，持久化数据库与下载目录（推荐）

默认 Web 启动监听：`PT_HOST=0.0.0.0`、`PT_PORT=8080`，外部可通过端口映射访问。

### 从源码构建

1. 克隆代码库：

   ```bash
   git clone https://github.com/sunerpy/pt-tools.git
   cd pt-tools
   ```

2. 构建二进制文件：

   ```bash
   go build -o pt-tools .
   ```

3. 将二进制文件移动到系统 `PATH`：

   ```bash
   mv pt-tools /usr/local/bin/pt-tools
   ```

### 一键部署

提供了一个自动化脚本，帮助用户快速下载和安装 `pt-tools`。

```bash
curl -fsSL https://raw.githubusercontent.com/sunerpy/pt-tools/main/scripts/download.sh | bash
```

### 下载最新 Release

1. 前往 [pt-tools Release 页面](https://github.com/sunerpy/pt-tools/releases)，下载适合你系统的最新版本二进制文件。
2. 解压下载的压缩包：

   ```bash
   tar -xvzf pt-tools-linux-amd64.tar.gz
   ```

   或：

   ```bash
   unzip pt-tools-windows-amd64.exe.zip
   ```

3. 将解压后的二进制文件移动到系统 `PATH`：

   ```bash
   mv pt-tools /usr/local/bin/
   ```

4. 验证安装：

   ```bash
   pt-tools version
   ```


## 快速开始

### 初始化配置（Web）

  首次运行进入 Web 页面，根据提示完成数据库与全局配置初始化。
  默认登录账号：`admin` / `adminadmin`

#### 重置管理员密码（忘记密码）

推荐通过环境变量在启动时重置：

```bash
docker run -d \
  -e PT_HOST=0.0.0.0 -e PT_PORT=8080 \
  -e PT_ADMIN_RESET=1 \
  -e PT_ADMIN_USER=admin \
  -e PT_ADMIN_PASS='你的新密码' \
  -v ~/pt-data:/app/.pt-tools \
  -p 8080:8080 \
  --name pt-tools sunerpy/pt-tools:latest
```

说明：
- 仅当 `PT_ADMIN_RESET=1` 时执行一次重置；完成后建议移除 `PT_ADMIN_RESET/PT_ADMIN_PASS`，避免下次重启再次重置。
- 若未设置 `PT_ADMIN_USER/PT_ADMIN_PASS`，默认使用 `admin/adminadmin`。

---

通过以上简单步骤，你可以快速配置并运行 `pt-tools`。

---

## 使用说明

### 可用命令

- `web`：启动 Web 管理界面（默认）
- `completion`：生成 Bash/Zsh 补全脚本
- `help`：显示帮助信息

### 全局选项

- `-h, --help`：显示命令的帮助信息

---

## Web 页面配置项说明

通过 Web 页面进行配置，保存后立即生效并持久化到数据库。

- 全局设置（`/api/global`）
  - `download_dir`：下载根目录（默认 `~/.pt-tools/downloads`）
  - `default_interval_minutes`：默认任务间隔（分钟）
  - `default_enabled`：默认是否启用各站点任务
  - `download_limit_enabled`：启用限速
  - `download_speed_limit`：下载限速（MB/s）
  - `torrent_size_gb`：最大种子大小（GB）
  - `auto_start`：自动启动任务（开：启动时自动运行；关：需手动启动）
  - `retain_hours`：本地 `.torrent` 保留时长（小时），超过未推送则清理
  - `max_retry`：推送失败最大重试次数，超过后不再重试并记录错误

- qBittorrent 设置（`/api/qbit`）
  - `enabled`：是否启用 qBit 客户端
  - `url`：qBittorrent WebUI 地址（如 `http://192.168.1.10:8080`）
  - `user`：登录用户名
  - `password`：登录密码

- 站点与 RSS（`/api/sites`、`/api/sites/{name}`）
  - 站点设置：
    - `enabled`：是否启用该站点任务
    - `auth_method`：认证方式（`api_key` 或 `cookie`）
    - `api_key` / `api_url`：当 `auth_method=api_key` 时必填
    - `cookie`：当 `auth_method=cookie` 时必填
  - RSS 订阅：
    - `name`：订阅名
    - `url`：RSS 链接
    - `category`：分类（如 `Tv`/`Mv` 等）
    - `tag`：标签（用于任务列表区分来源）
    - `interval_minutes`：执行间隔（分钟）
    - `download_sub_path`：下载子目录（相对 `download_dir`，例如 `mteam/avs`）

---

---

### 注意事项

- 通过 Web 页面进行配置，保存后立即生效并持久化到数据库。

### 配置说明（Web 页面）

#### 全局设置

| 配置项                     | 类型   | 默认值          | 描述                                                 |
| -------------------------- | ------ | --------------- | ---------------------------------------------------- |
| `default_interval`       | 字符串 | `"5m"`        | 默认的任务间隔时间，格式支持 `"5m"`、`"10m"`等。 |
| `default_enabled`        | 布尔值 | `true`        | 默认是否启用所有站点任务。                           |
| `download_dir`           | 字符串 | `"downloads"` | 种子文件下载的默认目录。                             |
| `download_limit_enabled` | 布尔值 | `true`        | 是否启用下载速度限制。                               |
| `download_speed_limit`   | 整数   | `20`          | 下载速度限制，单位为 MB/s。                          |
| torrent_size_gb            | 整数   |                 | 限制下载种子的最大大小，单位GB                       |

#### qBittorrent 设置

| 配置项       | 类型   | 默认值                        | 描述                             |
| ------------ | ------ | ----------------------------- | -------------------------------- |
| `enabled`  | 布尔值 | `true`                      | 是否启用 qBittorrent 客户端。    |
| `url`      | 字符串 | `"http://xxx.xxx.xxx:8080"` | qBittorrent Web UI 的 URL 地址。 |
| `user`     | 字符串 | `"admin"`                   | qBittorrent 登录用户名。         |
| `password` | 字符串 | `"adminadmin"`              | qBittorrent 登录密码。           |

#### 站点与 RSS 设置

##### 站点通用设置

| 配置项          | 类型   | 默认值                                       | 描述                                          |
| --------------- | ------ | -------------------------------------------- | --------------------------------------------- |
| `enabled`     | 布尔值 | `false`                                    | 是否启用该站点的任务。                        |
| `auth_method` | 字符串 | `"api_key"`或 `"cookie"`                 | 认证方式，可选 `"api_key"`或 `"cookie"`。 |
| `api_key`     | 字符串 | 必填（如果 `auth_method`为 `"api_key"`） | API 密钥。                                    |
| `api_url`     | 字符串 | 必填（如果 `auth_method`为 `"api_key"`） | API 地址。                                    |
| `cookie`      | 字符串 | 必填（如果 `auth_method`为 `"cookie"`）  | Cookie 值。                                   |

##### RSS 设置

每个站点的 RSS 配置通过 `[[sites.<站点名>.rss]]` 定义，支持多个 RSS 配置。

| 配置项                | 类型   | 默认值 | 描述                                                         |
| --------------------- | ------ | ------ | ------------------------------------------------------------ |
| `name`              | 字符串 | 必填   | RSS 订阅的名称。                                             |
| `url`               | 字符串 | 必填   | RSS 订阅链接地址。                                           |
| `category`          | 字符串 | 必填   | RSS 订阅分类，用于标记种子类型（例如：`"Tv"`、`"Mv"`）。 |
| `tag`               | 字符串 | 必填   | 为任务添加的标记，用于区分不同任务来源。                     |
| `interval_minutes`  | 整数   | 必填   | 任务执行间隔时间，单位为分钟。                               |
| `download_sub_path` | 字符串 | 必填   | 下载的种子文件存储的子目录路径（相对于 `download_dir`）。  |

---

## 数据持久化与示例引用

- 配置与任务记录统一保存在 SQLite（`~/.pt-tools/torrents.db`）
- 下载目录默认 `~/.pt-tools/downloads`，可通过 Web 全局设置修改
- 示例文档：
  - `examples/docker-run.md`：容器运行（单目录挂载到 `/app/.pt-tools`）
  - `examples/docker-compose.yml`：Compose 编排（单目录挂载到 `/app/.pt-tools`）

---

## 高级使用

### 启用 Shell 补全

#### Bash

1. 生成 Bash 补全脚本：

   ```bash
   pt-tools completion bash > /etc/bash_completion.d/pt-tools
   ```

2. 重新加载 Shell 或直接加载补全脚本：

   ```bash
   source /etc/bash_completion.d/pt-tools
   ```

#### Zsh

1. 确保已启用补全功能：

   ```bash
   echo "autoload -U compinit; compinit" >> ~/.zshrc
   ```

2. 生成 Zsh 补全脚本：

   ```bash
   pt-tools completion zsh > "${fpath[1]}/_pt-tools"
   ```

3. 启动新 Shell 会话。

---

## 贡献

欢迎贡献代码！请通过 [GitHub 仓库](https://github.com/sunerpy/pt-tools) 提交问题或拉取请求。

---

## 许可证

本项目基于 [MIT 许可证](https://github.com/sunerpy/pt-tools?tab=MIT-1-ov-file) 进行许可。
