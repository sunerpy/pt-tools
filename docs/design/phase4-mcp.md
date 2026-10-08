# MCP Server 设计

> **状态**：已实现，随 1.0.0-rc 预览版发布。用法见 [MCP 使用说明](../guide/mcp.md)。
> **代码入口**：[`internal/mcp`](https://github.com/sunerpy/pt-tools/tree/v1.0.0-rc/internal/mcp)（工具、权限、审计）、`web/mcp.go`（`/mcp` 与进程内调用 App API）、`cmd/mcp.go`（stdio 桥）
> **关联设计**：[`chatops-mcp-agent.md`](chatops-mcp-agent.md) §8

---

## 1. 概览

pt-tools 作为 [Model Context Protocol](https://modelcontextprotocol.io) Server，让外部 AI 助手与 IDE（Claude Desktop、Claude Code、Cursor、Cherry Studio 等）用标准协议调用 pt-tools 的能力。

工具本身不碰数据库与下载器：每个工具在进程内调用 [App API v1](../reference/app-api.md) 的一条路由，返回 App API 的 JSON。这样字段、脱敏（没有 Cookie、Passkey、密码、RSS 地址与下载链接）、参数校验与推送闸门都只有一份实现，MCP 与手机 App 看到的是同一套数据。

---

## 2. 为什么是 MCP

| 替代方案                   | 缺点                                                                              |
| -------------------------- | --------------------------------------------------------------------------------- |
| Agent 直接调内部 Go 函数   | 紧耦合；每换一个 Agent 框架要重写适配                                             |
| Agent 调 pt-tools REST API | API 是面向 UI 的；缺少能力描述（无 schema、无权限元数据），Agent 难以正确选择工具 |
| 直接接入某个 Agent 框架    | 与厂商绑死；用户切换 LLM 提供商代价高                                             |
| **MCP Server**             | 协议级标准；工具自描述；权限元数据；Cursor/Claude Desktop 即插即用                |

---

## 3. 选型：modelcontextprotocol/go-sdk

- 官方 Go SDK，固定在 v1.8.0（`go.mod`）。
- 用 `mcp.AddTool[In, Out]` 注册工具：参数结构体带 `jsonschema` 标签，SDK 生成输入 schema 并在调用前校验；取值固定的参数（状态、排序、媒体类型）另外写进 schema 的 `enum`。
- 工具带注解：只读工具 `readOnlyHint`，删除 `destructiveHint`，客户端据此决定提醒的强度。
- 测试用 SDK 的内存传输逐个测工具，HTTP 层用 SDK 的 streamable HTTP 客户端走真实的 mux。

---

## 4. 传输

### 4.1 streamable HTTP（`/mcp`）

- 挂在 Web 主端口的 `/mcp`，无状态（每个请求独立，不发 `Mcp-Session-Id`），回应是 JSON 而不是 SSE，反向代理不需要特别配置。
- 每个请求都要 API 令牌，所以关掉了 go-sdk 默认的 localhost DNS rebinding 保护（经 127.0.0.1 进来、Host 不是 localhost 的请求原本会被拒，同机反向代理会撞上）：rebinding 拿不到令牌。

### 4.2 stdio（`pt-tools mcp`）

- 给只支持 stdio 的客户端（Claude Desktop）。`pt-tools mcp --url <地址> --token <令牌>`（也认 `PT_TOOLS_MCP_URL`、`PT_TOOLS_MCP_TOKEN`）是一个桥：用 go-sdk 的客户端连上运行中的 pt-tools 的 `/mcp`，把那里的工具原样注册到本地 stdio 服务，调用原样转发。
- 桥不打开数据库，也不启动调度器：pt-tools 只有一个进程在写，避免第二个调度器与第二个写者。所以 stdio 也要令牌，权限和 HTTP 完全一样。

---

## 5. 工具

| 工具                       | 权限        | App API 路由               | 与最初契约的差异                                                                                                         |
| -------------------------- | ----------- | -------------------------- | ------------------------------------------------------------------------------------------------------------------------ |
| `list_tasks`               | `mcp:read`  | `GET /tasks`               | 列的是推送记录（RSS、订阅、App、MCP 推过的种子），不是 RSS 订阅配置                                                      |
| `list_downloader_torrents` | `mcp:read`  | `GET /torrents`            | 不给 `downloader_id` 时列全部下载器；多了关键字与排序                                                                    |
| `get_downloader_stats`     | `mcp:read`  | `GET /downloaders`（新增） | `downloader_id` 可选，不给时报全部；也用来查下载器编号                                                                   |
| `search_torrents`          | `mcp:read`  | `POST /search`             | 结果只有站点与种子编号，没有下载链接；`limit` 在工具里截断                                                               |
| `get_site_userinfo`        | `mcp:read`  | `GET /sites`               | 去掉了 `refresh`：给的是缓存的数据（`updated_at` 写明时间）                                                              |
| `check_updates`            | `mcp:read`  | `GET /updates`（新增）     | 无                                                                                                                       |
| `explore_media`            | `mcp:read`  | `GET /explore`             | 新增：按片名找 TMDB 编号，或者看趋势、热门                                                                               |
| `list_subscriptions`       | `mcp:read`  | `GET /subscriptions`       | 新增                                                                                                                     |
| `pause_torrent`            | `mcp:write` | `POST /torrents/actions`   | 无                                                                                                                       |
| `resume_torrent`           | `mcp:write` | `POST /torrents/actions`   | 也要 `confirm=true`                                                                                                      |
| `delete_torrent`           | `mcp:write` | `POST /torrents/actions`   | 无                                                                                                                       |
| `push_torrent`             | `mcp:write` | `POST /push`               | 给站点与种子编号，或者已知站点的下载地址（解析成站点与编号，不请求它）；不收 `torrent_b64` 与磁力链接；要 `confirm=true` |
| `add_subscription`         | `mcp:write` | `POST /subscriptions`      | 新增；要 `confirm=true`                                                                                                  |

- 写工具（`mcp:write` 的那几个）一律要 `confirm=true`，没有时不调用 App API。
- 工具清单的权威来源是 `internal/mcp.ToolNames`；「系统 → MCP 接入」页与使用说明里的清单由测试对照它。
- 不做：升级、重启这类高破坏性的工具，令牌管理工具。

---

## 6. 鉴权与审计

| 维度       | 实现                                                                                                                                         |
| ---------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| HTTP 层    | go-sdk 的 `auth.RequireBearerToken`，校验函数用 `internal/apitoken`；没有或不对的令牌 401，既没有 `mcp:read` 也没有 `mcp:write` 的令牌 403   |
| 网页会话   | 不收：`/mcp` 只认 `Authorization: Bearer`                                                                                                    |
| 工具层     | 只读工具要 `mcp:read`，写工具要 `mcp:write`（精确匹配，互不包含）                                                                            |
| 进程内调用 | 主体种类 `mcp`，`mcp:read` 当 `app:read`、`mcp:write` 当 `app:write`；不经 App API 的审计包装（审计由工具层记）                              |
| 审计       | 写工具每次调用都记进操作审计（`action_audit`）：通道 `mcp`、命令是工具名、触发用户是令牌编号；被拒的（`denied:scope`、`denied:confirm`）也记 |
| 推送来源   | 经 MCP 推送的种子记录来源是 `mcp_push`                                                                                                       |

---

## 7. 与 ChatOps / AI Agent 的边界

```
+----------------+      +---------------+      +------------------+      +------------------+
| AI 助手 / IDE  | ---> | MCP Server    | ---> | App API v1       | ---> | 服务层与下载器    |
| (Claude 等)    |      | internal/mcp  |      | （进程内调用）   |      | internal/...     |
+----------------+      +---------------+      +------------------+      +------------------+
+----------------+                                                              ^
| Telegram / QQ  | --- ChatOps ---------------------------------------------------+
+----------------+
```

- **ChatOps**：人类通过 IM 直接调用，命令在 `internal/chatops/commands/` 实现。
- **MCP**：AI 助手通过协议调用，工具在 `internal/mcp` 实现，经 App API v1 复用同一批服务。
- **AI Agent**：见 [`phase5-agent.md`](phase5-agent.md)。Agent 必须经 MCP，不直接持有数据库写权限。
