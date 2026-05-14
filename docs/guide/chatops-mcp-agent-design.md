# ChatOps / MCP / AI Agent 设计方向

本文档用于整理 `pt-tools` 后续在 **聊天机器人（ChatOps）**、**通知通道**、**MCP Server** 与 **AI Agent** 方向上的总体设计思路，帮助在正式实现前做方案对比。

## 1. 背景与目标

`pt-tools` 当前已经具备以下核心能力：

- RSS 任务调度与自动下载
- 下载器任务控制（暂停、恢复、删种、重校验、改保存路径）
- 种子状态查询与历史记录管理
- 站点搜索、用户信息聚合、版本检查与升级

这些能力已经足够支撑一个外部控制层。新的设计方向不是再造一套业务核心，而是在现有能力之外增加一层：

1. **ChatOps 层**：允许用户通过 QQ / 企业微信 / Telegram 等通道查看状态、执行有限控制操作、接收通知。
2. **MCP Server 层**：把 `pt-tools` 能力标准化暴露为工具接口，供外部 AI Agent 或 IDE/平台调用。
3. **AI Agent 层**：在 MCP 之上增加自然语言任务编排、问答、策略建议与安全确认。

这三层应该按照“**同一能力内核，不同接入面**”来设计，而不是分别实现三套逻辑。

---

## 2. 核心设计判断

### 2.1 不引入完整多用户业务模型

当前项目是单管理员架构，只有 `AdminUser` 和基于 session 的后台登录。现阶段不建议把系统改造成完整多租户应用。

推荐方向：

- 保持 `pt-tools` 核心仍是 **单实例、单管理员配置中心**
- 新增的是“**通道绑定**”与“**能力授权**”，而不是本地用户中心

这意味着：

- 机器人、MCP、Agent 的“使用者身份”主要来源于外部通道（QQ 号、chat_id、API token、client id）
- 系统内部只维护绑定关系、权限范围、审计记录

### 2.2 统一能力服务层，避免重复实现

无论入口是 Web、Bot、MCP 还是 AI Agent，底层都应复用同一套服务能力，例如：

- 查询任务列表
- 查询下载器实时种子
- 暂停/恢复/删种
- 推送种子到下载器
- 触发 RSS 任务控制
- 查询站点数据与版本状态

建议新增一层显式的 **Application Service / Use Case 层**，把现在散在 `web/`、`scheduler/`、`internal/` 的调用链收敛出来，为后续多入口复用做准备。

### 2.3 先做 ChatOps 兼容架构，再平滑扩展到 MCP

聊天机器人是最接近现有用户心智的入口，MCP Server 更适合作为后续标准化扩展。

因此推荐路线：

1. **Phase 1：通知 + 简单命令**
2. **Phase 2：统一命令服务层**
3. **Phase 3：MCP Server 暴露工具**
4. **Phase 4：AI Agent 编排与自然语言控制**

---

## 3. 总体分层架构

建议把未来体系拆成五层：

### 3.1 Core Domain / Existing Runtime

现有核心能力，继续保留在当前结构中：

- `scheduler/`：RSS、cleanup、free-end、peer-ratio 等调度
- `internal/`：push、过滤、公共业务逻辑、事件总线
- `thirdpart/downloader/`：下载器抽象
- `version/`：版本检查与升级
- `models/` + `core/config_store.go`：配置与持久化

### 3.2 Application Service 层（建议新增）

建议新增例如：

```text
internal/app/
  task_service.go
  torrent_service.go
  search_service.go
  stats_service.go
  version_service.go
  notification_service.go
```

职责：

- 封装跨模块调用
- 统一参数校验
- 统一结果结构
- 提供适合 Bot / MCP / Agent 复用的接口

例如：

- `ListTasks(...)`
- `ListDownloaderTorrents(...)`
- `PauseTorrent(...)`
- `DeleteTorrent(...)`
- `PushTorrent(...)`
- `CheckUpdates(...)`

### 3.3 Notification / Channel 层（建议新增）

负责把事件和响应发到外部通道：

```text
internal/channel/
  dispatcher.go
  provider.go
  onebot/
  wecom/
  telegram/
```

职责：

- 维护通道配置
- 路由通知到不同 provider
- 接收外部通道回调或消息
- 做身份绑定与权限校验

### 3.4 MCP Server 层（后续扩展）

建议新增：

```text
internal/mcpserver/
  server.go
  tools_tasks.go
  tools_torrents.go
  tools_search.go
  tools_version.go
```

职责：

- 以标准 MCP Tool 形式暴露能力
- 把 Application Service 层包装成稳定工具接口
- 输出结构化结果，便于 AI Agent 使用

### 3.5 Agent Orchestration 层（远期）

这一层不建议一开始深耦合进主业务，而建议保持可插拔：

- 可以是内置一个轻量 agent router
- 也可以由外部 AI 平台通过 MCP 调用 `pt-tools`

职责：

- 自然语言解释
- 计划分解
- 多步调用工具
- 高风险动作确认

---

## 4. 推荐的 ChatOps 设计方向

### 4.1 QQ：优先采用 OneBot 生态

综合现有生态，QQ 方向推荐：

- **NapCatQQ** 作为 QQ 协议桥
- **OneBot v11** 作为标准接入协议
- Go 侧可以：
  - 直接调用 OneBot HTTP API
  - 或通过 ZeroBot / 自定义 webhook / reverse WS 处理入站消息

推荐原因：

- 活跃度高
- 已有大量现成实践
- 支持私聊、群聊、命令式交互
- 与 MoviePilot 用户心智一致

注意事项：

- 属于社区协议桥方案，需预期 QQ 风控风险
- 适合个人或小范围自托管，不适合直接作为公共 SaaS 平台默认通道

### 4.2 企业微信：优先支持 webhook 机器人

企业微信分两种模式：

1. **群机器人 webhook**：适合通知，配置最轻
2. **企业应用 + 回调**：适合双向交互，但配置复杂，需要公网和安全校验

建议先实现：

- 企业微信群机器人 webhook
- 用于发版公告、任务通知、异常告警

后续如有明确需求再扩展企业应用模式。

### 4.3 Telegram / 其他通道：作为可选 provider

架构上不要把 QQ 设计成特例。建议统一成 provider 插件模型，未来可加：

- Telegram
- Discord
- 飞书
- 钉钉
- 通用 webhook

---

## 5. 推荐的数据模型方向

建议至少新增两类模型。

### 5.1 NotificationConf

用于描述一个通道实例：

- 名称
- 类型（qqbot_onebot / wecom_webhook / telegram ...）
- 是否启用
- 凭证信息
- 事件订阅开关
- 是否允许命令
- provider 专属扩展配置

示意：

```go
type NotificationConf struct {
    ID         uint
    Name       string
    Type       string
    Enabled    bool
    Token      string
    Secret     string
    Endpoint   string
    ExtraJSON  string
    Switches   string
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

### 5.2 ChannelBinding

用于描述“某个外部身份”如何绑定到本实例：

- 所属通道配置
- 外部身份标识（QQ `user_id`、TG `chat_id`、企微 `userid`）
- 是否验证成功
- 是否具有管理权限
- 可使用的能力范围

示意：

```go
type ChannelBinding struct {
    ID          uint
    ChannelID   uint
    BindKey     string
    Alias       string
    Verified    bool
    IsAdmin     bool
    ScopeJSON   string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

### 5.3 ActionAudit（推荐）

对于 Bot / MCP / Agent，这个表很重要，建议尽早考虑：

- 谁触发的
- 通过哪个入口触发的（web / bot / mcp / agent）
- 调用了哪个动作
- 参数摘要
- 是否成功
- 错误原因

这样后续才能做审计、追责和调试。

---

## 6. 建议的统一能力模型

为了给 Bot、MCP、Agent 共用，建议把外部可调用能力定义成有限、稳定的动作集合，而不是直接暴露内部函数细节。

推荐第一批能力：

### 6.1 只读能力

- 查询 RSS 任务列表
- 查询下载器实时种子列表
- 查询暂停种子
- 查询站点聚合统计
- 查询版本与更新状态
- 查询系统运行状态（速度、磁盘、活跃任务数）

### 6.2 有副作用能力

- 暂停种子
- 恢复种子
- 删除种子
- 删除种子及数据
- 推送种子到下载器
- 启动/停止全局调度
- 触发手动更新检查

### 6.3 高风险能力

- 升级程序
- 批量删除
- 清空历史记录
- 修改全局配置

高风险能力必须支持：

- 二次确认
- 审计记录
- 更严格的权限范围

---

## 7. 事件模型建议

当前 `internal/events/bus.go` 很轻，只定义了少量事件。为了支撑通知与 Agent 观察能力，建议扩展事件模型。

推荐事件类型：

- `TorrentPushed`
- `TorrentPushFailed`
- `TorrentPaused`
- `TorrentResumed`
- `TorrentDeleted`
- `TorrentCleaned`
- `FreeEndTriggered`
- `DiskSpaceLow`
- `VersionAvailable`
- `ConfigChanged`

建议事件结构不要只放类型，最好带结构化 payload：

```go
type Event struct {
    Type    EventType
    Source  string
    At      time.Time
    Payload json.RawMessage
}
```

或使用明确的 typed payload。

这样用途有三类：

1. 通知系统订阅并发消息
2. MCP/Agent 做状态感知
3. 后续如有需要可接入事件日志或观测系统

---

## 8. MCP Server 设计方向

### 8.1 为什么值得做 MCP Server

如果后续要引入 AI Agent，那么最干净的边界不是“让 AI 直接调内部函数”，而是让 `pt-tools` 先成为一个能力明确、边界清晰的 MCP Server。

好处：

- 和具体 AI 框架解耦
- 工具清单清晰、权限容易管控
- 易于接入 Cursor / Claude Desktop / 自建 Agent / OpenAI compatible agent frameworks
- 对外输出的是稳定工具协议，不是项目内部实现细节

### 8.2 MCP Tools 的第一批建议

推荐优先做这些工具：

- `list_tasks`
- `list_downloader_torrents`
- `get_downloader_stats`
- `search_torrents`
- `pause_torrent`
- `resume_torrent`
- `delete_torrent`
- `push_torrent`
- `get_site_userinfo`
- `check_updates`

这些工具已经几乎都能映射到现有 API 或内部逻辑。

### 8.3 MCP 设计要点

- 工具输出尽量结构化
- 错误信息明确且机器可读
- 高风险工具要求显式确认参数
- 不要在 MCP 层做复杂策略，策略交给 Agent

示例：

```json
{
  "name": "delete_torrent",
  "description": "Delete a torrent from downloader by downloader_id and task_id",
  "input_schema": {
    "type": "object",
    "properties": {
      "downloader_id": { "type": "integer" },
      "task_id": { "type": "string" },
      "remove_data": { "type": "boolean" }
    },
    "required": ["downloader_id", "task_id"]
  }
}
```

---

## 9. AI Agent 设计方向

### 9.1 Agent 不应直接成为“超级管理员”

后续若要支持 AI Agent，建议遵循：

- Agent 只调用 MCP tools
- Agent 不直接持有底层数据库写权限
- 危险动作需走确认模式

### 9.2 推荐的 Agent 能力场景

比起让 Agent 直接“全自动删种”，更适合先做这些场景：

1. **问答型助手**
   - 最近有哪些下载失败？
   - 哪些种子长时间未完成？
   - 哪些站点上传下载比异常？

2. **辅助决策**
   - 根据当前磁盘和种子状态，推荐清理候选
   - 解释为什么某条 RSS 没有推送
   - 解释免费种子没有下载的原因

3. **受控执行**
   - 帮我暂停所有某类任务
   - 帮我删除某个下载器下卡住 7 天以上的任务
   - 执行前先生成计划，再确认

### 9.3 Agent 模式建议

推荐区分两种模式：

- **Assistant 模式**：只读、分析、建议
- **Operator 模式**：允许有副作用的工具调用，但必须确认

这样可以降低一开始引入 AI 的风险。

---

## 10. 统一权限模型建议

即使不做本地多用户，也建议抽象出统一权限范围，供 Bot / MCP / Agent 共用。

可以按 capability 设计，例如：

- `tasks.read`
- `torrents.read`
- `torrents.pause`
- `torrents.resume`
- `torrents.delete`
- `torrents.delete_with_data`
- `torrents.push`
- `system.read`
- `system.control`
- `system.upgrade`

用途：

- ChannelBinding 限权
- MCP token 限权
- Agent profile 限权

这样未来从 Bot 扩展到 MCP 时不用重做权限系统。

---

## 11. 推荐的实现阶段

### Phase 1：通知通道

目标：先解决“看见系统状态”和“发版公告”。

范围：

- `NotificationConf`
- 企业微信 webhook provider
- OneBot sender
- GitHub Actions 发版公告接入
- 事件总线最小扩展

### Phase 2：ChatOps 命令

目标：先做有限命令控制。

范围：

- `ChannelBinding`
- `/bind <code>` 绑定流程
- `/status` `/tasks` `/pause` `/resume` `/delete`
- 操作审计
- 高风险二次确认

### Phase 3：统一应用服务层

目标：把 Web / Bot / 后续 MCP 的逻辑统一。

范围：

- `internal/app/*`
- 统一 DTO / result / error 模型
- 抽离 `web/` 里可复用业务逻辑

### Phase 4：MCP Server

目标：标准化工具暴露。

范围：

- MCP server 基础设施
- 首批工具集
- token/capability 授权

### Phase 5：AI Agent

目标：引入自然语言与多步编排。

范围：

- Agent profile
- 计划/确认机制
- 只读优先，逐步放开执行能力

---

## 12. 方案对比建议

你后续做实现方式对比时，可以优先比较下面几组选择。

### 12.1 QQ 方案对比

- NapCat + OneBot
- 官方 QQ Bot 平台
- 暂不支持 QQ，仅保留企业微信/Telegram

建议判断标准：

- 自托管难度
- 风控/稳定性
- 私聊能力
- 群聊能力
- 社区活跃度

### 12.2 服务边界对比

- 直接在 `web/` 上加 bot 接口
- 新增 `internal/app` 统一服务层后再接 Bot/MCP

建议判断标准：

- 短期实现成本
- 中长期复用性
- MCP 演进阻力

### 12.3 AI 集成路径对比

- Bot 直接内嵌 AI
- 先做 MCP，再接外部 Agent

建议判断标准：

- 可维护性
- 供应商耦合度
- 权限与审计清晰度

---

## 13. 最终推荐方向

如果以“**先做对、再做大**”为原则，推荐路线如下：

1. **先把通知/聊天通道抽象出来**，形成 provider 模型
2. **把核心操作抽成统一服务层**，避免 Web/Bot/MCP 各写一套
3. **先做 ChatOps，不急着做重 AI 化**
4. **MCP 作为标准化能力出口**，而不是 Bot 的附属功能
5. **AI Agent 始终建立在 MCP tools 之上**，不要直接穿透内部实现

一句话总结：

> `pt-tools` 更适合演进成“**具备 ChatOps 与 MCP 能力的 PT 运维核心**”，而不是单纯加一个聊天机器人插件。

这个方向能同时覆盖：

- 自托管用户的日常控制需求
- 群聊与通知场景
- 后续 AI 助手与外部自动化系统接入

---

## §14 Phase 1+2 实施摘要

本节汇总 Phase 1（通知通道）与 Phase 2（ChatOps 命令）已落地的关键实现，便于后续开发者快速定位各模块。

### §14.1 数据库 Schema（5 张表）

| 表名 | GORM 模型 | 用途 |
|------|-----------|------|
| `notification_conf` | `models.NotificationConf` | 通道配置（Telegram / QQ / WeCom / Webhook），`ConfigJSON` AES-GCM 加密存储 |
| `channel_binding` | `models.ChannelBinding` | 外部用户与通道实例的绑定关系，含管理员标记与白名单状态 |
| `action_audit` | `models.ActionAudit` | 命令执行审计日志，复合索引 `(conf_id, user_id, created_at DESC)` |
| `bot_token` | `models.BotToken` | bind code（明文 8 字符，5 min TTL）与 bearer token（bcrypt hash）统一存储 |
| `notification_outbox` | `models.NotificationOutbox` | 离线投递队列，`status` 取值 `pending/sent/failed/dead` |

所有表通过 GORM `AutoMigrate` 追加，旧版本数据库在升级时自动新建这 5 张表，不影响现有数据。

### §14.2 Channel Adapter（4 个）

| Adapter 类型 | 注册键 | 配置包路径 |
|-------------|--------|-----------|
| QQ OneBot（NapCat 反向 WS） | `qq_onebot` | `internal/notify/adapter/qq` |
| Telegram Bot（长轮询） | `telegram` | `internal/notify/adapter/telegram` |
| 企业微信群机器人 Webhook | `wecom_webhook` | `internal/notify/adapter/wecom` |
| 通用 HTTP Webhook | `webhook` | `internal/notify/adapter/webhook` |

每个 adapter 在 `init()` 中调用 `notify.RegisterChannel(type, factory)` 完成注册，与 `site/v2/definitions/` 侧注册模式一致。所有 adapter 实现 `notify.Channel` 接口的 7 个方法：`Type / Init / SupportsInbound / Send / OnInbound / Close / Healthy`。

### §14.3 ChatOps 命令（11 个）

| 命令 | 文件 | 说明 |
|------|------|------|
| `/bind <code>` | `commands/bind.go` | 凭 8 字符 bind code 绑定当前用户 |
| `/unbind` | `commands/unbind.go` | 解除当前会话绑定 |
| `/status` | `commands/status.go` | 查询系统运行状态（速度、磁盘、活跃任务数） |
| `/tasks` | `commands/tasks.go` | 列出 RSS 任务运行状态 |
| `/torrents` | `commands/torrents.go` | 按下载器分页列出种子 |
| `/pause <hash>` | `commands/pause.go` | 暂停指定种子 |
| `/resume <hash>` | `commands/resume.go` | 恢复指定种子 |
| `/delete <hash>` | `commands/delete.go` | 删除种子（带二次确认） |
| `/sites` | `commands/sites.go` | 列出配置的站点摘要 |
| `/version` | `commands/version.go` | 查询当前版本及最新可用版本 |
| `/help` | `commands/help.go` | 列出所有命令及用法 |

命令均通过 `internal/chatops.CommandRegistry` 注册，支持速率限制（默认 10 次/分钟/用户）与会话上下文（5 分钟 TTL）。

### §14.4 前端页面（4 个 Vue SPA）

| 路由路径 | 文件 | 功能 |
|---------|------|------|
| `/chatops/notifications` | `views/chatops/Notifications.vue` | 通道列表、新建/编辑/删除/测试通道 |
| `/chatops/notifications/:id` | `views/chatops/NotificationDetail.vue` | 单通道详情与配置 JSON 编辑 |
| `/chatops/bindings` | `views/chatops/Bindings.vue` | 绑定关系管理、生成 bind code、调整权限 |
| `/chatops/audit` | `views/chatops/AuditLog.vue` | 命令执行审计日志分页查询 |

### §14.5 关键依赖

| 依赖 | 版本 | 用途 |
|------|------|------|
| `github.com/mymmrac/telego` | v1.9.0 | Telegram Bot API SDK（长轮询） |
| `github.com/wdvxdr1123/ZeroBot` | v1.8.2 | QQ OneBot 消息框架 |
| `golang.org/x/crypto/bcrypt` | v0.51.0+ | bearer token 哈希校验 |

### §14.6 配置示例

**Telegram 通道（ConfigJSON 加密前明文）**

```json
{
  "bot_token": "123456789:AABB...",
  "allowed_users": [100000001],
  "admin_users": [100000001],
  "default_chat_id": -1001234567890,
  "polling_timeout_seconds": 10
}
```

**QQ OneBot 通道（ConfigJSON 加密前明文）**

```json
{
  "listen_addr": "0.0.0.0:8765",
  "path": "/onebot/v11/ws",
  "access_token": "your-access-token",
  "admin_qq_users": [123456789],
  "allowed_qq_users": [123456789]
}
```

**企业微信群机器人 Webhook（ConfigJSON 加密前明文）**

```json
{
  "webhook_key": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "msg_type": "markdown"
}
```

**通用 Webhook（ConfigJSON 加密前明文）**

```json
{
  "endpoint_url": "https://your-server.example.com/hook",
  "timeout_seconds": 10,
  "hmac_secret": "your-hmac-secret",
  "headers": {
    "Authorization": "Bearer your-api-key"
  }
}
```

**docker-compose 环境变量示例（密钥注入）**

```yaml
services:
  pt-tools:
    image: sunerpy/pt-tools:latest
    environment:
      PT_HOST: "0.0.0.0"
      PT_PORT: "8080"
      PT_TOOLS_SECRET_KEY: "base64-encoded-32-byte-key-here"
      TZ: "Asia/Shanghai"
    volumes:
      - ./data:/app/.pt-tools
    restart: unless-stopped
```

---

## §15 升级与回滚

### §15.1 升级路径

ChatOps Phase 1+2 新增了 5 张数据库表，但不修改任何现有表结构。升级流程如下：

1. **停止旧版本服务**
2. **替换二进制（或更新 Docker 镜像）**
3. **启动新版本**：`core.InitRuntime()` 内的 `AutoMigrate` 会自动检测并创建缺失的表

```text
旧版本（无 ChatOps 表）
  → 启动新版本
    → GORM AutoMigrate 追加 5 张表
      → 用户通过 Web UI 添加通道配置
        → 管理员执行 /bind <code> 完成绑定
          → 功能可用
```

全程无需手动执行 SQL。若部署在 SQLite 模式下，`AutoMigrate` 只添加列/建表，不删除旧数据。

### §15.2 回滚程序

如果新版本出现严重问题，需要回滚到不含 ChatOps 功能的旧版本：

**步骤 1：停止服务**

```bash
systemctl stop pt-tools
# 或 Docker
docker stop pt-tools
```

**步骤 2：删除 5 张 ChatOps 表（可选，旧版本会忽略未知表）**

```sql
-- 连接到 ~/.pt-tools/torrents.db
sqlite3 ~/.pt-tools/torrents.db <<EOF
DROP TABLE IF EXISTS notification_outbox;
DROP TABLE IF EXISTS action_audit;
DROP TABLE IF EXISTS bot_token;
DROP TABLE IF EXISTS channel_binding;
DROP TABLE IF EXISTS notification_conf;
EOF
```

> 注意：旧版本二进制实际上不会读写这 5 张表，所以即使不删除也不影响正常运行。删除仅用于彻底清理或迁移到其他存储方案。

**步骤 3：切回旧版本二进制**

```bash
cp /backup/pt-tools-old /usr/local/bin/pt-tools
systemctl start pt-tools
```

**Docker 回滚**：

```bash
docker pull sunerpy/pt-tools:v1.x.x  # 指定旧版本 tag
docker stop pt-tools && docker rm pt-tools
docker run -d --name pt-tools \
  -p 8080:8080 \
  -v ~/pt-data:/app/.pt-tools \
  -e PT_HOST=0.0.0.0 \
  sunerpy/pt-tools:v1.x.x
```

### §15.3 数据保留说明

回滚后如果重新升级到带 ChatOps 的版本，`AutoMigrate` 会重建表结构，但之前的绑定关系、通道配置等数据已丢失（如未备份）。建议在升级前备份整个数据库文件：

```bash
cp ~/.pt-tools/torrents.db ~/.pt-tools/torrents.db.bak.$(date +%Y%m%d)
```

---

## §16 NapCat 部署指南

NapCatQQ 是目前主流的 QQ 协议桥实现，通过 OneBot v11 协议与 pt-tools 通信。pt-tools 的 QQ 适配器使用**反向 WebSocket（Reverse WS）**模式，即 NapCat 主动连接到 pt-tools 监听的 HTTP 服务。

### §16.1 前置条件

- 一个可正常登录 QQ 的账号
- NapCatQQ 已安装并登录（参考 NapCat 官方文档）
- pt-tools 所在服务器与 NapCat 宿主机网络互通

### §16.2 pt-tools 通道配置

在 Web UI 中新建通道，类型选 `qq_onebot`，ConfigJSON 填写：

```json
{
  "listen_addr": "0.0.0.0:8765",
  "path": "/onebot/v11/ws",
  "access_token": "your-secret-token",
  "admin_qq_users": [123456789],
  "allowed_qq_users": [123456789, 987654321]
}
```

- `listen_addr`：pt-tools 监听反向 WS 连接的地址和端口，需在防火墙上开放
- `path`：WebSocket 路径，NapCat 侧需与此匹配
- `access_token`：双向认证令牌，NapCat 连接时会携带，留空则不校验
- `admin_qq_users`：具有管理员权限（可执行危险命令）的 QQ 号列表
- `allowed_qq_users`：白名单 QQ 号列表，不在列表内的消息会被忽略

### §16.3 NapCat 配置

在 NapCat 的配置文件（或 Web UI 的「网络」设置）中添加反向 WebSocket 连接：

```json
{
  "websocketClients": [
    {
      "enable": true,
      "url": "ws://your-pt-tools-host:8765/onebot/v11/ws",
      "messagePostFormat": "array",
      "token": "your-secret-token",
      "reconnectInterval": 5000,
      "heartInterval": 30000
    }
  ]
}
```

字段说明：

| 字段 | 值 | 说明 |
|------|----|------|
| `url` | `ws://host:port/path` | 填写 pt-tools 的监听地址，路径与 `path` 配置一致 |
| `messagePostFormat` | `array` | 固定使用 array 格式，pt-tools 仅解析此格式 |
| `token` | 同 `access_token` | 与 pt-tools 侧保持完全一致 |
| `reconnectInterval` | `5000` | 断线重连间隔（毫秒），建议 5000 |
| `heartInterval` | `30000` | 心跳间隔（毫秒），建议 30000 |

### §16.4 验证连接

NapCat 启动后，pt-tools 日志应出现类似：

```
INFO  qq_onebot adapter: websocket client connected, conf_id=1
```

在绑定的 QQ 账号私聊发送 `/help`，若收到命令列表说明连通正常。

> **风控提示**：QQ 协议桥属于第三方社区方案，存在账号被风控封禁的风险。建议使用小号或单独的机器人 QQ 账号，不要使用主力 QQ 账号。

---

## §17 Telegram bot token 申请

### §17.1 通过 BotFather 创建 bot

1. 在 Telegram 中搜索 `@BotFather`，点击「Start」
2. 发送 `/newbot` 命令
3. BotFather 会提示输入 bot 的**显示名称**（用户可见，如 `PT Tools Bot`）
4. 再输入 bot 的**用户名**（全局唯一，必须以 `bot` 结尾，如 `mypts_bot`）
5. 创建成功后，BotFather 会返回一段消息，其中包含 token，格式为：

   ```
   123456789:AABB-CCDDEEFFaabbccddeeff00112233445566
   ```

   这就是 `bot_token`，妥善保管，不要公开。

### §17.2 获取 chat_id

`default_chat_id` 是 pt-tools 主动推送消息时使用的默认目标。可以是：

- **个人私聊**：与 bot 发一条消息，然后访问 `https://api.telegram.org/bot<token>/getUpdates`，在返回的 JSON 中找到 `message.chat.id`（正整数）
- **群组**：将 bot 加入群组后同上获取，群组 chat_id 为负整数（如 `-1001234567890`）

### §17.3 设置隐私模式（可选但推荐）

默认情况下，bot 在群组中只有被 `@` 或发送命令时才会收到消息（隐私模式开启）。对于 pt-tools 的使用场景，建议保持默认（隐私模式开启），避免 bot 处理群组内所有消息带来的性能浪费。

如果需要 bot 接收群组内所有消息，可通过 BotFather 的 `/setprivacy` 命令关闭隐私模式。

### §17.4 填入 pt-tools 配置

在 Web UI 新建通道，类型选 `telegram`，将获取到的 token 和 chat_id 填入 ConfigJSON：

```json
{
  "bot_token": "123456789:AABB-CCDDEEFFaabbccddeeff...",
  "allowed_users": [100000001],
  "admin_users": [100000001],
  "default_chat_id": 100000001,
  "polling_timeout_seconds": 10
}
```

其中 `allowed_users` 和 `admin_users` 填写你自己的 Telegram 数字 ID（可通过 `@userinfobot` 查询）。

---

## §18 安全注意事项

### §18.1 PT_TOOLS_SECRET_KEY 管理

`PT_TOOLS_SECRET_KEY` 是用于 AES-256-GCM 加密通道 `ConfigJSON` 的主密钥。

**密钥生成**：

```bash
# 生成随机 32 字节密钥并 base64 编码
openssl rand -base64 32
```

**部署方式（按安全性排序）**：

1. **推荐**：通过容器 secrets 或外部 secret manager（如 Docker Secrets、AWS SSM）注入环境变量，不写入 docker-compose.yml 明文
2. **可接受**：在 `.env` 文件中设置 `PT_TOOLS_SECRET_KEY=...`，`.env` 加入 `.gitignore`，不提交到代码仓库
3. **不推荐**：直接写入 docker-compose.yml 或 systemd service 文件（任何有权读取配置文件的人都能获取密钥）

**密钥丢失后果**：若密钥丢失，已加密的 `ConfigJSON` 无法解密，通道配置将不可用。需重新添加所有通道。因此建议在安全位置（如密码管理器）备份密钥。

**密钥轮换**：当前版本不支持在线密钥轮换。若需更换密钥，需先删除所有通道配置，替换密钥，再重新添加。

### §18.2 bind code TTL 与使用限制

bind code 是 8 字符一次性验证码，用于将外部聊天账号绑定到 pt-tools。

- **TTL**：5 分钟，过期自动失效
- **单次使用**：每个 code 只能使用一次，使用后立即标记为已用
- **并发限制**：同一管理员最多同时持有 3 个未使用且未过期的活跃 code，超过限制时必须等待旧 code 过期
- **字符集设计**：排除了视觉上易混淆的字符（`0`/`O`、`1`/`l`/`I`），减少手动输入出错概率

在 Web UI 生成 code 后，应通过安全渠道（而非公开群聊）将 code 告知需要绑定的用户。

### §18.3 命令速率限制

默认每用户每分钟最多执行 10 条命令，超出后静默丢弃（不回复错误，避免信息泄露）。

部分高频或高代价命令有独立的更严格限制，例如 `/torrents` 分页查询。速率计数器基于 `(channel_type, channel_user_id, command)` 三元组，跨通道不共享配额。

当前速率限制在内存中实现，重启后计数器重置。

### §18.4 bearer token 旋转

bearer token 用于 HTTP API 的机器人鉴权（MCP Server、外部自动化脚本等）。

**旋转建议**：

- 定期（如每 90 天）通过 Web UI 撤销旧 token 并生成新 token
- 旧 token 在撤销后立即失效，依赖旧 token 的客户端需同步更新
- token 以 bcrypt hash 存储，原始 token 仅在生成时展示一次，之后无法查看，请立即保存

**泄露处理**：

1. 立即在 Web UI 中撤销泄露的 token
2. 检查 `action_audit` 表中该 token 对应的操作记录
3. 生成新 token 并更新所有使用方

### §18.5 bot token 旋转（Telegram）

如果 Telegram bot token 泄露，需立即通过 BotFather 旋转：

1. 向 `@BotFather` 发送 `/mybots`，选择对应 bot
2. 选择「API Token」→「Revoke current token」
3. BotFather 返回新 token
4. 在 pt-tools Web UI 中更新对应通道的 ConfigJSON，将 `bot_token` 替换为新值
5. 保存后通道会自动重新初始化（无需重启服务）

旧 token 在 BotFather 吊销后立即失效，使用旧 token 的所有请求会返回 `401 Unauthorized`。
