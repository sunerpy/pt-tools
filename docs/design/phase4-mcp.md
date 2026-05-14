# Phase 4 — MCP Server 接口契约

> **状态**：Design — 仅契约与文档，**不含运行时实现**。  
> **代码入口**：[`internal/mcp/contract.go`](../../internal/mcp/contract.go)（零外部依赖）  
> **关联设计**：[`docs/guide/chatops-mcp-agent-design.md`](../guide/chatops-mcp-agent-design.md) §8

---

## 1. Overview

Phase 4 把 pt-tools 暴露为一个 [Model Context Protocol](https://modelcontextprotocol.io) Server，让外部 AI Agent / IDE / 工具（Claude Desktop、Cursor、Continue、自建 Agent 等）通过标准协议调用 pt-tools 的能力，而无需关心 pt-tools 的内部实现细节。

本期（Phase 1+2）**不实现** MCP server；仅交付：

1. `internal/mcp/contract.go` — 工具清单类型 + 10 个工具完整 JSON Schema
2. 本设计文档 — 选型、传输、鉴权、启动条件

实际的 MCP server 启动延后到 Phase 4。Phase 4 触发条件见 §7。

---

## 2. 为什么是 MCP

| 替代方案 | 缺点 |
|---------|------|
| Agent 直接调内部 Go 函数 | 紧耦合；每换一个 Agent 框架要重写适配 |
| Agent 调 pt-tools REST API | API 是面向 UI 的；缺少能力描述（无 schema、无权限元数据），Agent 难以正确选择工具 |
| 直接接入某个 Agent 框架 | 与厂商绑死；用户切换 LLM 提供商代价高 |
| **MCP Server** | 协议级标准；工具自描述；权限元数据；Cursor/Claude Desktop 即插即用 |

MCP 还提供 **transport-agnostic** 的优势：同一份工具实现可同时通过本地 stdio（IDE 内嵌）和远端 HTTP（多用户共享）暴露。

---

## 3. 选型：modelcontextprotocol/go-sdk

- **仓库**：<https://github.com/modelcontextprotocol/go-sdk>
- **维护方**：MCP 官方（@modelcontextprotocol 组织）
- **理由**：
  - 官方实现，协议演进同步无延迟
  - 同时支持 stdio 与 streamable-HTTP 两种 transport
  - 提供 `mcp.AddTool(server, &mcp.Tool{...}, handler)` 注册范式，与本契约的 `Tool` 结构能 1:1 映射
  - Go module 干净，不引入 cgo
- **注意**：本期 `internal/mcp/contract.go` **不引入** 该依赖。Phase 4 启动时再 `go get`。

> 若 Phase 4 启动时 `go-sdk` 仍处于 v0.x 早期阶段，可考虑社区方案 `mark3labs/mcp-go` 作为兜底；评估时再决定。

---

## 4. Transports

Phase 4 同时支持两种 transport：

### 4.1 stdio（首选，本地）

- **场景**：用户在自己的 IDE / 桌面 LLM 客户端（Claude Desktop、Cursor）里把 pt-tools 当本地 MCP server 启动。
- **启动**：`pt-tools mcp` 子命令（继承 cobra 框架，与 `pt-tools web` 平行）。
- **生命周期**：进程随 client 启动/退出，无需 daemon。
- **鉴权**：进程间共享 fd，**默认信任**调用方（与 Unix 哲学一致）。

### 4.2 streamable-HTTP（远端 / 多用户）

- **场景**：与 Web 服务同进程暴露，挂在 `/mcp` 路径下，给远端 Agent 调用。
- **复用**：与 `web/server.go` 共享 `http.ServeMux` 与中间件链。
- **鉴权**：必须经 T9 bearer token 中间件。

实现切换由 cobra flag 决定：

```bash
pt-tools mcp --transport stdio                       # 默认；本地
pt-tools mcp --transport http --addr 0.0.0.0:8081    # 远端
```

---

## 5. Tool Inventory（10 个工具）

> 以下为 `internal/mcp/contract.go` 中 `ContractTools` 的权威 schema 摘要。完整 `map[string]any` 形式见源码。  
> 工具与 [`internal/chatops/commands/`](../../internal/chatops/commands) 的命令一一对应（少数为 web 独有）。

### 5.1 `list_tasks`（read-only）

列举 RSS 订阅任务。

```json
{
  "type": "object",
  "properties": {
    "site_id":       { "type": "string", "description": "Optional site identifier filter" },
    "enabled_only":  { "type": "boolean", "description": "Only enabled tasks" }
  },
  "required": [],
  "additionalProperties": false
}
```

映射：`internal/chatops/commands/tasks.go`。

### 5.2 `list_downloader_torrents`（read-only）

分页列举下载器中的种子。

```json
{
  "type": "object",
  "properties": {
    "downloader_id": { "type": "integer" },
    "page":          { "type": "integer", "minimum": 1, "default": 1 },
    "page_size":     { "type": "integer", "minimum": 1, "maximum": 200, "default": 50 },
    "state_filter":  { "type": "string", "enum": ["all","downloading","seeding","paused","completed","error"], "default": "all" }
  },
  "required": ["downloader_id"],
  "additionalProperties": false
}
```

映射：`internal/chatops/commands/torrents.go` + web `/api/downloaders/{id}/torrents`。

### 5.3 `get_downloader_stats`（read-only）

下载器聚合统计（种子数、活跃数、速度、剩余磁盘）。

```json
{
  "type": "object",
  "properties": { "downloader_id": { "type": "integer" } },
  "required": ["downloader_id"],
  "additionalProperties": false
}
```

映射：`internal/chatops/commands/status.go`。

### 5.4 `search_torrents`（read-only，跨站点）

```json
{
  "type": "object",
  "properties": {
    "keyword":   { "type": "string", "minLength": 1 },
    "sites":     { "type": "array", "items": { "type": "string" } },
    "free_only": { "type": "boolean", "default": false },
    "limit":     { "type": "integer", "minimum": 1, "maximum": 100, "default": 20 }
  },
  "required": ["keyword"],
  "additionalProperties": false
}
```

映射：web `/api/search`（无 chatops 命令；仅 MCP / web）。

### 5.5 `pause_torrent`（**HIGH-RISK**，confirm required）

```json
{
  "type": "object",
  "properties": {
    "downloader_id": { "type": "integer" },
    "task_id":       { "type": "string" },
    "confirm":       { "type": "boolean", "const": true }
  },
  "required": ["downloader_id", "task_id", "confirm"],
  "additionalProperties": false
}
```

映射：`internal/chatops/commands/pause.go`。

### 5.6 `resume_torrent`（low-risk mutation）

```json
{
  "type": "object",
  "properties": {
    "downloader_id": { "type": "integer" },
    "task_id":       { "type": "string" }
  },
  "required": ["downloader_id", "task_id"],
  "additionalProperties": false
}
```

映射：`internal/chatops/commands/resume.go`。

### 5.7 `delete_torrent`（**HIGH-RISK**，confirm + remove_data）

```json
{
  "type": "object",
  "properties": {
    "downloader_id": { "type": "integer" },
    "task_id":       { "type": "string" },
    "remove_data":   { "type": "boolean", "default": false },
    "confirm":       { "type": "boolean", "const": true }
  },
  "required": ["downloader_id", "task_id", "confirm"],
  "additionalProperties": false
}
```

映射：`internal/chatops/commands/delete.go`。

### 5.8 `push_torrent`（mutation：新增种子）

支持 URL 推送（站点直链 + passkey）或 base64 .torrent 内容。

```json
{
  "type": "object",
  "properties": {
    "downloader_id": { "type": "integer" },
    "torrent_url":   { "type": "string", "format": "uri" },
    "torrent_b64":   { "type": "string" },
    "category":      { "type": "string" },
    "save_path":     { "type": "string" },
    "paused":        { "type": "boolean", "default": false }
  },
  "required": ["downloader_id"],
  "additionalProperties": false
}
```

> `torrent_url` 与 `torrent_b64` 互斥；server 实现需校验。

### 5.9 `get_site_userinfo`（read-only）

```json
{
  "type": "object",
  "properties": {
    "site_id": { "type": "string" },
    "refresh": { "type": "boolean", "default": false }
  },
  "required": ["site_id"],
  "additionalProperties": false
}
```

映射：`internal/chatops/commands/sites.go`。

### 5.10 `check_updates`（read-only）

```json
{
  "type": "object",
  "properties": {
    "include_prerelease": { "type": "boolean", "default": false }
  },
  "required": [],
  "additionalProperties": false
}
```

映射：`internal/chatops/commands/version.go`。**绝不**触发自升级（自升级是 web 操作，不该由 Agent 自动决定）。

---

## 6. 鉴权模型

| 维度 | 策略 |
|------|------|
| 复用 T9 bearer token 中间件 | streamable-HTTP transport 必须带 `Authorization: Bearer <token>` |
| stdio transport | 默认信任本地调用方；可选 `--require-token` flag 强制 |
| Capability scopes | Token 元数据中的 `scopes` 字段控制可调用工具集合 |
| 高危工具 | `pause_torrent` / `delete_torrent` 要求 token 持有 `mutate:torrents` scope |
| Auditing | 每次工具调用写入 `chatops_audit_logs`（与 chatops 共用），记录 token、tool、args 哈希、result code |

Token 与 scope 的具体定义在 T9（`web/middleware_bearer.go`）已落地；MCP server 仅复用。

---

## 7. Phase 4 启动条件

Phase 4 不设硬性时间表。建议触发条件（任一即可）：

1. Phase 1+2 上线 ≥ 1 个月，无 P0 故障
2. 至少 3 名外部用户主动询问 MCP / Agent 集成
3. ChatOps 命令日均调用 ≥ 100 次（说明用户接受“通过对话操作 pt-tools”的形态）

启动后预计工作量 ≈ 5 个工作日（参考 `go-sdk` 示例 + 现有 chatops `services.go` 抽象）。

---

## 8. 与 ChatOps / AI Agent 的边界

```
+----------------+      +---------------+      +------------------+
| External Agent | ---> | MCP Server    | ---> | Application      |
| (Claude/Cursor)|      | (Phase 4)     |      | Service Layer    |
+----------------+      +---------------+      | internal/app/    |
                                               +------------------+
+----------------+              ^
| Telegram / QQ  | --- ChatOps -+  (also reuses internal/app)
+----------------+
```

- **ChatOps（Phase 1+2）**：人类通过 IM 直接调用，命令在 `internal/chatops/commands/` 实现。
- **MCP（Phase 4）**：AI Agent 通过协议调用，工具在未来 `internal/mcp/server.go` 实现，**复用** `internal/app/` 服务层。
- **AI Agent（Phase 5）**：见 [`phase5-agent.md`](phase5-agent.md)。Agent 必须经 MCP，**不**直接持有 DB 写权限。

---

## 9. FAQ

**Q: 为什么不复用 `web/api_*` 端点直接给 Agent？**  
A: REST 不携带工具描述、参数 schema、危险标记，Agent 不知如何选择；MCP 协议这些都是必填字段。

**Q: ChatOps 和 MCP 是否会发散？**  
A: 不会。两者都在 `internal/app/` 服务层之上，命令/工具定义在不同 package，但调用同一组业务逻辑。

**Q: 高危工具如何审计？**  
A: 工具调用流经 `web/middleware_audit.go` → `audit.Service.Record(...)`，与 ChatOps、Web UI 共用一张表。

---

## 10. 不做的事

- ❌ Phase 1+2 不引入 `modelcontextprotocol/go-sdk` 依赖（grep 验证 0 行）
- ❌ 不实现 server lifecycle / handler / transport
- ❌ 不暴露 `start_upgrade` / `restart_service` 类工具（高破坏性，应由人工触发）
- ❌ 不暴露 token 管理工具（`create_token` 必须 web UI 走管理员鉴权）
