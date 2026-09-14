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

## 14. 当前实现状态

本文最初用于规划 ChatOps、MCP 与 Agent 的演进路径。当前仓库状态如下：

| 层级           | 状态                              | 当前入口                                                    |
| -------------- | --------------------------------- | ----------------------------------------------------------- |
| 通知与 ChatOps | 已实现                            | Web UI、QQ OneBot、Telegram、企业微信 Webhook、通用 Webhook |
| 应用服务层     | 已实现并持续收敛                  | `internal/app/`                                             |
| MCP Server     | 仅保留工具契约，尚无可运行 Server | [`phase4-mcp.md`](phase4-mcp.md)                            |
| AI Agent       | 设计阶段，不承诺交付日期          | [`phase5-agent.md`](phase5-agent.md)                        |

ChatOps 的安装与运维说明已拆分到用户指南，避免设计文档重复维护配置步骤：

- [ChatOps 快速开始](../guide/chatops-quickstart.md)
- [QQ OneBot（NapCat）配置](../guide/chatops-qq-napcat.md)
- [Telegram Bot 配置](../guide/chatops-telegram.md)
- [RSS 上新通知](../guide/chatops-rss-notify.md)

> [!IMPORTANT]
> `internal/mcp/contract.go` 只冻结未来工具名称和风险分级，不代表 MCP transport、鉴权或运行时已经上线。对外可用能力以根 README、Web UI 和 Release 说明为准。
