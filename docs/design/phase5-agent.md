# Phase 5 AI Agent 设计文档

**版本**：草稿 v0.1  
**状态**：设计中，不承诺交付日期  
**依赖**：Phase 4 MCP Server 稳定上线后评估

---

## 1. 背景：为何需要 Agent，以及何时引入

pt-tools 当前已经覆盖了"通知"（Phase 1）、"ChatOps 命令"（Phase 2）和"应用服务层"（Phase 3）。
Phase 4 MCP 提供了标准化工具接口，让外部程序可以通过 JSON-RPC 调用 pt-tools 的核心能力。

到了这个阶段，我们可以做什么还不够、或者不方便做的事情？

**ChatOps 命令的局限性**：

- 命令是一对一的：用户输入 `/pause <id>`，系统执行一次动作。
- 没有上下文推理能力：无法理解"把磁盘占用超过 80% 且分享率低于 0.3 的种子全部暂停"这类自然语言指令。
- 没有多步编排：无法自动拆解"先查状态、再过滤、再确认、再执行"的复合操作。

**MCP 工具是 Agent 的天然接入点**：

Phase 4 将所有写操作（暂停、删除、推送 RSS、管理通知通道）都抽象为 MCP 工具，每个工具都有精确的输入输出 schema 和权限作用域。这恰好是 AI Agent 需要的接口形态，任何支持 MCP 协议的 Agent 框架都能直接接入，不需要 pt-tools 重新开放专门的"AI 接口"。

**何时引入**：Phase 4 MCP 上线并稳定运行一段时间、积累足够多的实际使用数据之后再评估。具体触发条件见第 6 节。

---

## 2. Assistant 模式 vs Operator 模式

引入 AI Agent 时，最容易犯的错误是把所有能力一次打开。pt-tools 的操作涉及种子删除、通道配置修改、RSS 推送等有副作用的动作，一旦 Agent 理解偏差就可能造成不可逆的损失。

因此建议明确区分两种运行模式，并作为 Agent 接入时的第一道配置项。

### Assistant 模式（只读 + 分析）

Agent 只能调用读操作工具，不触发任何有副作用的 MCP 工具。

适用场景：

- 查询当前所有下载任务的状态，给出"卡住超过 7 天"的候选列表
- 分析某站点的分享率趋势，给出建议的做种时长
- 解释为什么某条 RSS 订阅没有触发下载（过滤规则命中哪条）
- 统计近 30 天磁盘使用情况，预测何时需要清理

在 Assistant 模式下，Agent 可以输出自然语言分析报告，也可以输出结构化的"执行计划草稿"，但不自动提交执行。

### Operator 模式（受控写操作）

Agent 可以调用有副作用的工具，但每一步写操作之前必须经过确认环节。

确认方式可以是：

- ChatOps 频道发消息给绑定用户，等待回复 `/confirm <token>` 或 `/abort`
- MCP 客户端本身提供的"工具调用确认 UI"（如果客户端支持）

Operator 模式需要额外的权限 scope（`agent.operator`），与 Assistant 模式的 `agent.readonly` 区分。这样可以按需开放，默认新接入的 Agent 处于 Assistant 模式。

两种模式都不允许 Agent 绕过 MCP 直接访问 pt-tools 内部数据库或文件系统。

---

## 3. 接入路径：外部 Agent 通过 MCP 调用 pt-tools

pt-tools 不内嵌 LLM，也不集成特定的 Agent 框架。这是一个主动选择，理由如下：

1. **避免供应商绑定**：不同用户有不同的 LLM 偏好和访问条件，内嵌某个 SDK 会强制依赖特定服务。
2. **关注点分离**：pt-tools 擅长 PT 站点操作和数据管理，AI 推理交给专门的 Agent 框架处理更合适。
3. **MCP 已经是标准接口**：Phase 4 完成后，任何支持 MCP client 协议的 Agent 均可直接接入，无需额外适配。

推荐的接入路径如下：

```
用户自然语言输入
        │
        ▼
  外部 Agent 框架
  (Claude / GPT / Gemini / 本地模型 等)
        │
        │  MCP JSON-RPC over stdio / HTTP-SSE
        ▼
  pt-tools MCP Server (Phase 4)
        │
        │  调用 MCP 工具
        ▼
  pt-tools 应用服务层 (internal/app/*)
        │
        ▼
  数据库 / 下载器 / 通知通道
```

Agent 框架持有 MCP token（由 pt-tools Phase 4 的 token 管理体系颁发，带 scope 限制）。每次工具调用都会经过 MCP Server 的鉴权和审计流程，与 ChatOps 命令使用同一套权限模型。

这意味着：

- 用户不需要在 pt-tools 里配置 LLM API Key
- 用户可以自由选择 Agent 框架（包括完全本地运行的方案）
- pt-tools 的安全边界在 MCP Server 层，不依赖外部 Agent 框架的安全实现

---

## 4. 自然语言场景示例

以下场景展示了外部 Agent 通过 MCP 实现的典型交互流程，对应 pt-tools 已有的 11 条 ChatOps 命令能力。

### 场景 1：列出当前种子

用户输入："帮我看看现在有哪些正在下的种子，按进度排序"

Agent 执行路径：
1. 调用 `mcp_torrent_list`（指定 downloader，不带过滤）
2. 对返回结果按 `progress` 字段排序
3. 格式化为自然语言或结构化列表返回给用户

等价命令：`/torrents`

### 场景 2：暂停指定种子

用户输入："把 HDSky 里那几个下了超过 7 天还没完成的种子暂停掉"

Agent 执行路径（Operator 模式）：
1. 调用 `mcp_torrent_list`，筛选 `add_time < now - 7d AND progress < 1.0`
2. 生成执行计划，向用户发送"发现 3 个符合条件的种子，确认暂停？"
3. 收到确认后，对每个种子调用 `mcp_torrent_pause`
4. 报告执行结果

等价命令：`/pause <id>`

### 场景 3：推送 RSS

用户输入："检查一下 MTeam 的 free RSS，把今天新出的种子推下来"

Agent 执行路径：
1. 调用 `mcp_site_list` 确认 MTeam 站点存在且配置正常
2. 调用 `mcp_rss_trigger`（触发指定站点的 RSS 拉取）
3. 等待任务状态或直接返回"已触发，后台处理中"

等价命令：`/rss`

### 场景 4：查询站点信息

用户输入："我在 HDSky 和 MTeam 的上传量分别是多少？分享率怎么样？"

Agent 执行路径：
1. 调用 `mcp_site_info`，分别传入两个站点名称
2. 从返回的 `UserInfo` 结构提取 `uploaded`、`ratio` 等字段
3. 组合成比较性的自然语言回答

等价命令：`/sites`、`/userinfo`

### 场景 5：删除种子

用户输入："帮我把那些做种比超过 5 且已经做了超过 90 天的种子删掉，数据也删"

Agent 执行路径（Operator 模式，高危动作）：
1. 调用 `mcp_torrent_list`，筛选 `ratio > 5 AND seeded_days > 90`
2. 生成详细的删除计划（列出所有候选种子名称、大小、当前比）
3. 向用户发送确认请求，要求明确回复（不接受模糊的"好的"）
4. 收到明确确认后，调用 `mcp_torrent_delete`（`remove_data=true`）
5. 记录完整审计日志

等价命令：`/delete <id>`

以上场景都不要求 pt-tools 有任何变更，Agent 框架只需要一个有效的 MCP token 和对应的工具 schema 文档。

---

## 5. 安全边界

AI Agent 引入了新的攻击面，需要在设计阶段就确立清晰的安全边界，而不是等到出问题再补。

### Agent 必须经过 MCP，不能绕过

所有 Agent 调用的操作都必须通过 MCP Server 的鉴权流程。pt-tools 不会为 Agent 单独开一个"内部直连"接口，也不会允许 Agent 持有数据库写权限。即使将来考虑进程内 Agent，也要走同一套接口契约。

### 权限最小化

Agent 在颁发 MCP token 时，scope 按需申请：

- 只读分析场景：`agent.readonly`（等价于现有 `*.read` 类 scope 的合集）
- 受控写操作场景：需要额外的 `agent.operator` scope，且此 scope 默认不授予
- 高危操作（删除、修改通知通道）：需要 `agent.operator` 之外还需要对应的细粒度 scope（`torrents.delete`、`notify.write` 等）

### 高危动作强制确认

以下操作类别在 Operator 模式下不允许静默执行，必须经过用户确认环节：

- 删除种子（含是否同时删除数据）
- 修改或删除通知通道配置
- 停止正在运行的 RSS 任务
- 删除或重置任何持久化配置

确认 token 有 TTL（建议 5 分钟），超时自动作废，不允许重用。

### 审计全链路

每一次 Agent 调用的 MCP 工具都必须写入审计日志，包括：

- 发起调用的 token 标识（脱敏后的 ID，不含明文 token）
- 调用的工具名称和参数（参数中的敏感字段由 redact 逻辑处理）
- 执行结果（成功/失败/被中止）
- 时间戳

这套审计日志与 ChatOps 命令审计使用同一个 `ActionAudit` 表，不需要额外的存储设计。

### 不向 Agent 暴露以下内容

- 明文 Cookie、Passkey、API Key
- 数据库连接字符串或文件路径
- 系统级操作（文件删除、进程管理）
- pt-tools 内部未经 MCP 封装的任何私有接口

---

## 6. Phase 5 触发条件

Phase 5 是"评估阶段"，不是"规划阶段"。引入 AI Agent 的前提是 Phase 4 MCP 已经足够稳定，有真实用户在使用，并且 MCP 工具的设计经过了足够的实际验证。

**进入评估的参考条件**（满足以下任意两条）：

- Phase 4 MCP Server 已正式上线，且连续运行 3 个月以上没有重大 breaking change
- 有至少 50 名活跃用户通过 MCP 接口调用过工具（不限于 Agent 场景）
- 社区有人主动实现了 pt-tools MCP 的 Agent 接入案例，并分享了实际使用体验
- ChatOps 命令中有超过 30% 的操作来自重复性的查询+执行模式（说明自动化需求真实存在）

以上条件不是硬性门槛，也不承诺满足条件后一定实施。最终决策取决于维护成本、社区需求和技术风险的综合评估。

**不设定时间表的原因**：

AI Agent 框架和 MCP 协议都在快速演进，现在承诺一个时间点只会产生沉没成本压力。等 Phase 4 稳定后再做具体规划，信息量更充分，决策质量更高。

---

## 7. 不做的事

明确不做什么，和明确要做什么同样重要。

### 不内嵌 Chatbot 或 LLM

pt-tools 不会在主进程里启动一个聊天机器人或加载本地语言模型。这会引入巨大的资源开销和依赖复杂度，对自托管用户不友好，对维护者也是负担。

如果用户想要对话体验，他们可以用自己选择的 Agent 框架连接 MCP。

### 不直接调用 LLM API

pt-tools 代码库中不会出现任何 LLM 供应商的 SDK 依赖。这包括但不限于任何商业或开源语言模型服务的客户端库。

原因：

- 引入特定 SDK 等于强制所有用户间接依赖该服务
- 不同地区的用户对不同服务的访问条件差异很大
- pt-tools 不应该承担 AI 推理的职责

### 不做 Auto-pilot 模式

不设计任何"Agent 无需用户确认自动执行写操作"的模式，哪怕是低风险的操作也不例外。

自动化执行是 RSS 订阅和定时任务的职责（已在 Phase 1/2 实现）。AI Agent 的价值在于处理需要上下文判断的复杂场景，而不是替代已有的规则引擎。

### 不替代 ChatOps 命令

Agent 是 ChatOps 的补充，不是替代。精确的单步操作（`/pause <id>`）仍然是最可靠的方式。Agent 负责处理需要多步推理、条件过滤或自然语言描述的场景。

### 不提供官方 Agent 实现

pt-tools 官方只维护 MCP Server 和工具 schema 文档，不提供官方的 Agent 配置包或 prompt 模板。社区成员可以自行探索并分享最佳实践。

---

*本文档描述的是设计方向和原则，不是实现规范。具体实现细节将在进入 Phase 5 评估阶段后另行设计。*
