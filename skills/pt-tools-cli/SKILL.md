---
name: pt-tools-cli
description: Manage pt-tools Docker deployment via CLI — RSS monitoring, multi-site search, user stats, task management, and downloader orchestration. For PT (Private Tracker) automation enthusiasts who run pt-tools on their home server or NAS.
version: 1.0.0
author: Hermes Agent
license: MIT
tags: ["PT", "Private-Tracker", "RSS", "Docker", "Automation", "Torrent"]
hermes:
  tags: ["PT", "RSS", "Docker", "Automation", "Torrent"]
  trigger_conditions:
    - "pt-tools"
    - "pt tools"
    - "PT站"
    - "Private Tracker"
    - "RSS订阅"
    - "种子搜索"
setup_instructions:
  prompt: "首次使用 pt-tools-cli skill，请先配置三个环境变量：PT_TOOLS_URL（服务地址）、PT_TOOLS_USER（用户名）、PT_TOOLS_PASSWORD（密码）。执行以下命令配置后告诉我完成情况：\n\n```bash\n# 追加到 ~/.zshrc（请将值替换为真实信息）\necho 'export PT_TOOLS_URL=\"http://你的服务器地址:8080\"' >> ~/.zshrc\necho 'export PT_TOOLS_USER=\"你的用户名\"' >> ~/.zshrc\necho 'export PT_TOOLS_PASSWORD=\"你的密码\"' >> ~/.zshrc\n\nsource ~/.zshrc\necho \"配置完成，PT_TOOLS_URL=$PT_TOOLS_URL\"\n```"
---

> ⚠️ **首次配置必读**：使用本 skill 前，必须在 `~/.zshrc` 中设置以下三个环境变量（否则无法运行）：
>
> ```bash
> echo 'export PT_TOOLS_URL="http://你的服务器地址:8080"' >> ~/.zshrc
> echo 'export PT_TOOLS_USER="admin"' >> ~/.zshrc
> echo 'export PT_TOOLS_PASSWORD="你的密码"' >> ~/.zshrc
> source ~/.zshrc
> ```
>
> 配置完成后告诉我，我会帮你验证连接是否正常。

# pt-tools-cli — PT站点管理CLI

`pt-tools-cli` 是 [pt-tools](https://github.com/sunerpy/pt-tools) Docker 实例的远程管理 CLI，通过 Web API 与服务端通信。适用于在 NAS/服务器上部署 pt-tools 的用户。

## 项目信息

| 项目 | 信息 |
|------|------|
| GitHub | `github.com/sunerpy/pt-tools` |
| CLI 二进制 | `/Users/colin/workspace/pt-tools/dist/pt-tools-cli` |
| 数据目录 | `~/pt-data` |
| 默认服务端口 | `8080` |
| 默认账号 | `admin` / `adminadmin` |
| 部署方式 | Docker（推荐）/ 二进制 |

## 环境变量配置

**首次使用前，请在 shell 配置文件（`~/.zshrc`）中设置以下三个环境变量：**

```bash
# 追加到 ~/.zshrc
echo 'export PT_TOOLS_URL="http://localhost:8080"' >> ~/.zshrc
echo 'export PT_TOOLS_USER="admin"' >> ~/.zshrc
echo 'export PT_TOOLS_PASSWORD="adminadmin"' >> ~/.zshrc

# 使配置生效
source ~/.zshrc
```

> ⚠️ **首次配置时提示用户填写**：如果 `PT_TOOLS_URL` 等变量未设置，Agent 应主动询问用户真实的服务器地址、用户名和密码。

| 变量 | 说明 | 示例 |
|------|------|------|
| `PT_TOOLS_URL` | pt-tools 服务地址（必填） | `http://192.168.1.100:8080` |
| `PT_TOOLS_USER` | 登录用户名（必填） | `admin` |
| `PT_TOOLS_PASSWORD` | 登录密码（必填） | `adminadmin` |

> 所有三个变量均为必填。如果调用时发现未设置，应主动告知用户配置方法并停止执行，避免硬编码或猜测值。

### 登录认证

**读取环境变量自动化登录：**

```bash
pt-tools-cli --url "$PT_TOOLS_URL" login "$PT_TOOLS_USER" "$PT_TOOLS_PASSWORD"
```

> 登录后认证信息会缓存到 `~/.config/pt-tools-cli/`，无需每次输入密码。

| 登录模式 | 命令 | 适用场景 |
|------|------|------|
| 交互式 | `pt-tools-cli login` | 首次手动登录 |
| 非交互式 | `pt-tools-cli login "$PT_TOOLS_USER" "$PT_TOOLS_PASSWORD"` | 脚本自动化 |

### 基础命令速查

```bash
pt-tools-cli --url http://localhost:8080 ping          # 检查服务状态
pt-tools-cli userinfo                                   # 聚合用户信息
pt-tools-cli userinfo --sites                           # 各站点详细统计
pt-tools-cli search "电影名"                             # 多站点搜索
pt-tools-cli search "电影名" --free-only --min-seeders 5  # 只搜免费+高做种
pt-tools-cli task list                                  # 查看任务列表
pt-tools-cli task list --downloaded --page 1            # 已完成任务
pt-tools-cli task list --expired                        # 已过期任务
pt-tools-cli task start                                 # 启动所有任务
pt-tools-cli task stop                                  # 停止所有任务
pt-tools-cli site list                                  # 查看已配置站点
pt-tools-cli downloader list                             # 查看下载器
pt-tools-cli logs --lines 50                            # 查看最近日志
pt-tools-cli version                                    # 查看服务端版本
```

## 详细命令说明

### 1. 登录认证

```bash
pt-tools-cli login [username] [password]
```

- 不带参数：交互式输入用户名和密码
- 带一个参数：只输入用户名，密码交互式输入
- 带两个参数：完全非交互（适合脚本）

> 登录后认证信息会缓存到本地配置文件（`~/.config/pt-tools-cli/`），无需每次输入密码。

### 2. 站点搜索

```bash
pt-tools-cli search "<关键词>" [flags]
```

| Flag | 说明 | 示例 |
|------|------|------|
| `--sites` | 指定站点（逗号分隔） | `--sites HDFans,MT` |
| `--min-seeders` | 最少做种人数 | `--min-seeders 10` |
| `--free-only` | 只显示免费种子 | `--free-only` |
| `--timeout` | 超时秒数（默认30） | `--timeout 60` |
| `--no-interactive` | 非交互式（适合脚本） | `--no-interactive` |
| `--output` | 输出格式：`json`/`quiet` | `--output json` |

**使用场景示例：**
```bash
# 追剧：搜索免费高做种的最新剧集
pt-tools-cli search "电视剧2026" --free-only --min-seeders 20

# 非交互式脚本调用
pt-tools-cli search "电影" --no-interactive --output json | jq '.results[] | .title'
```

### 3. 用户信息

```bash
pt-tools-cli userinfo [flags]
```

| Flag | 说明 |
|------|------|
| `--sites` | 显示各站点详细分解（否则只显示汇总） |

```bash
# 汇总信息（上传量、下载量、做种量）
pt-tools-cli userinfo

# 各站点详细表格
pt-tools-cli userinfo --sites
```

### 4. 任务管理

```bash
pt-tools-cli task list [flags]
pt-tools-cli task start
pt-tools-cli task stop
```

| Flag | 说明 | 示例 |
|------|------|------|
| `--site` | 按站点过滤 | `--site HDFans` |
| `--q` | 关键词搜索 | `--q 电影名` |
| `--downloaded` | 只看已下载 | `--downloaded` |
| `--pushed` | 只看已推送 | `--pushed` |
| `--expired` | 只看已过期（免费结束） | `--expired` |
| `--page` | 页码（默认1） | `--page 2` |
| `--page-size` | 每页数量（默认20，最大500） | `--page-size 100` |
| `--sort` | 排序方式 | `--sort created_at_asc` |

```bash
# 查看所有任务
pt-tools-cli task list

# 查看已过期任务（需要清理时）
pt-tools-cli task list --expired

# 按站点筛选
pt-tools-cli task list --site MT --page 1 --page-size 50

# 清理已下载完成的任务
pt-tools-cli task list --downloaded --page-size 200
```

### 5. 站点管理

```bash
pt-tools-cli site list
```

> 查看当前已配置的 PT 站点列表及状态。

### 6. 下载器管理

```bash
pt-tools-cli downloader list
```

> 查看已配置的 qBittorrent / Transmission 下载器实例。

### 7. 推送种子到下载器

```bash
pt-tools-cli push <downloadUrl> --downloaders <downloader_id>
```

**重要**：`push` 接收的是种子的完整 `downloadUrl`（如 `/api/site/mteam/torrent/1102489/download`），不是种子 ID。

**获取下载器 ID**（见上方"下载器管理"）：
```bash
pt-tools-cli downloader list
# 输出：│  1 │ 主下载器 │ qbittorrent │ ...
```

**推送示例**（以 search 结果的第 8 条为例）：
```bash
# 搜索并保存结果
pt-tools-cli search "创：战纪3" --no-interactive --output json > /tmp/results.json

# 提取第 N 条的 downloadUrl（downloadUrl 在 JSON 中完整呈现）
# 假设第 8 条的 downloadUrl 为 /api/site/mteam/torrent/1102489/download

# 推送到下载器（downloaders 使用数字 ID）
pt-tools-cli push /api/site/mteam/torrent/1102489/download --downloaders 1

# 可选：指定分类或标签
pt-tools-cli push /api/site/mteam/torrent/1102489/download --downloaders 1 --category "电影" --tags "2026,4K"
```

> `downloadUrl` 格式：`/api/site/<站点名>/torrent/<种子ID>/download`，搜索结果 JSON 中每个 item 的 `downloadUrl` 字段即为其完整路径。

### 8. 日志查看

```bash
pt-tools-cli logs [flags]
```

| Flag | 说明 | 默认值 |
|------|------|--------|
| `--lines` | 行数 | `50` |
| `--level` | 日志级别：`info`/`warn`/`error`/`debug` | `info` |

```bash
# 最近50行 info 日志
pt-tools-cli logs

# 最近100行 warn 及以上
pt-tools-cli logs --lines 100 --level warn
```

### 9. 服务健康检查

```bash
pt-tools-cli ping
```

> 检查服务端是否可达、认证是否有效。

### 10. 版本信息

```bash
pt-tools-cli version
```

> 查看 pt-tools 服务端版本号和构建信息。

## 与 Docker 部署的 pt-tools 协同工作

pt-tools-cli 是 pt-tools Docker 容器的管理客户端。典型架构：

```
┌─────────────────────────────────────────────┐
│  你本地 Mac/终端                             │
│  $ pt-tools-cli search "电影"                │
└──────────────┬──────────────────────────────┘
               │  HTTP API (port 8080)
               ▼
┌─────────────────────────────────────────────┐
│  Docker 容器 (pt-tools)                      │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  │
│  │ RSS订阅   │  │ 搜索爬虫  │  │ Web界面  │  │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  │
│       │            │            │          │
│       └────────────┴────────────┘          │
│                    │                      │
│            ┌───────┴───────┐              │
│            │  SQLite DB     │              │
│            │  ~/pt-data    │              │
│            └───────────────┘              │
└─────────────────────────────────────────────┘
               │  BT/下载协议
               ▼
┌─────────────────────────────────────────────┐
│  qBittorrent / Transmission                 │
│  (下载器，常在 NAS 上)                       │
└─────────────────────────────────────────────┘
```

## 配置文件

认证信息缓存位置：`~/.config/pt-tools-cli/config.json`

```json
{
  "url": "http://localhost:8080",
  "username": "admin",
  "cookie": "cached_session_cookie_here"
}
```

## 常见问题排查

### "Not authenticated" 错误
```bash
# 先登录
pt-tools-cli login admin adminadmin

# 或设置环境变量
export PT_TOOLS_URL="http://your-server:8080"
```

### 搜索返回 0 结果
- 检查站点是否已添加且启用：`pt-tools-cli site list`
- 确认 API Key 或 Cookie 有效：`pt-tools-cli ping`
- 查看服务端日志：`pt-tools-cli logs --level warn`

### Docker 部署升级
```bash
docker pull sunerpy/pt-tools:latest
docker restart pt-tools
```

## 自动化脚本示例

### 每日自动搜索追剧

```bash
#!/bin/bash
# 搜索并自动推送最新剧集
# 依赖环境变量：PT_TOOLS_URL, PT_TOOLS_USER, PT_TOOLS_PASSWORD

# 登录
pt-tools-cli --url "$PT_TOOLS_URL" login "$PT_TOOLS_USER" "$PT_TOOLS_PASSWORD"

# 搜索免费高清剧集
pt-tools-cli search "2026电视剧" --free-only --min-seeders 10 --no-interactive --output json > /tmp/search.json

# 从 JSON 中提取 downloadUrl 并推送（示例循环）
cat /tmp/search.json | jq -r '.items[] | "\(.id)|\(.title)|\(.downloadUrl)"' | while IFS='|' read -r id title url; do
  echo "推送: $title"
  pt-tools-cli push "$url" --downloaders 1
done
```

### 定期清理过期任务

```bash
#!/bin/bash
# 清理已过期任务
# 依赖环境变量：PT_TOOLS_URL, PT_TOOLS_USER, PT_TOOLS_PASSWORD

# 登录
pt-tools-cli --url "$PT_TOOLS_URL" login "$PT_TOOLS_USER" "$PT_TOOLS_PASSWORD"

expired=$(pt-tools-cli task list --expired --no-interactive --output json 2>/dev/null)
count=$(echo "$expired" | jq '.total')
echo "发现 $count 个过期任务"
pt-tools-cli task list --expired
```
