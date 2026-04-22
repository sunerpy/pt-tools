# pt-tools-cli

通过 CLI 远程管理 Docker 部署的 pt-tools 实例。

## 安装

```bash
make build-cli
cp dist/pt-tools-cli /usr/local/bin/pt-tools-cli
```

## 快速开始

```bash
# 设置服务器地址并登录
pt-tools-cli --url http://localhost:9090 login admin adminadmin

# 检查连接
pt-tools-cli ping

# 搜索种子（交互模式：选择 → 选择下载器 → 推送）
pt-tools-cli search "复仇者联盟" --sites mteam

# 非交互搜索
pt-tools-cli search "Movie" --sites mteam --min-seeders 10 --no-interactive
```

## 命令参考

### 认证

| 命令 | 说明 |
|------|------|
| `login [user] [pass]` | 登录，session cookie 缓存到 `~/.pt-tools-cli/config.json` |
| `config show` | 查看当前 CLI 配置 |
| `config clear` | 清除缓存的 cookie |
| `config set-url <url>` | 设置服务器 URL |
| `config set-user <user>` | 设置默认用户名 |

### 搜索与推送（核心功能）

```bash
pt-tools-cli search <keyword> [flags]
```

| 参数 | 说明 |
|------|------|
| `--sites x,y` | 指定搜索站点（逗号分隔） |
| `--min-seeders N` | 最小做种数 |
| `--free-only` | 仅显示免费/折扣种子 |
| `--timeout 30` | 搜索超时（秒） |
| `--no-interactive` | 跳过交互选择，仅显示结果 |
| `--output json` | 以 JSON 格式输出 |
| `--output quiet` | 安静模式，仅输出摘要行 |

交互式搜索流程：
1. 关键词搜索 → 显示结果表格
2. 输入编号选择种子（逗号分隔，支持 `all`）
3. 选择目标下载器（多选）
4. 批量推送 → 显示推送结果

单独推送：
```bash
pt-tools-cli push <downloadUrl> --downloaders 1,2
```

### 站点管理

| 命令 | 说明 |
|------|------|
| `site list` | 列出所有站点 |
| `site validate <name>` | 验证站点凭证 |
| `site delete <name>` | 删除站点 |

### 任务管理

| 命令 | 说明 |
|------|------|
| `task list` | 列出任务 |
| `task start` | 启动所有任务 |
| `task stop` | 停止所有任务 |

任务列表支持 `--site`、`--q`、`--page`、`--page-size`、`--downloaded`、`--pushed` 等过滤参数。

### 下载器

| 命令 | 说明 |
|------|------|
| `downloader list` | 列出下载器 |
| `downloader stats` | 传输统计（速度、总量、活跃数） |

### 其他

| 命令 | 说明 |
|------|------|
| `userinfo` | 聚合用户信息 |
| `logs --tail 100` | 查看最近日志 |
| `version` | 显示服务器版本 |

## 环境变量

| 变量 | 说明 |
|------|------|
| `PT_TOOLS_URL` | 服务器地址（等价于 `--url`） |
| `PT_TOOLS_USER` | 默认用户名 |

```bash
export PT_TOOLS_URL=http://localhost:9090
export PT_TOOLS_USER=admin
pt-tools-cli login admin adminadmin  # --url 可省略
```
