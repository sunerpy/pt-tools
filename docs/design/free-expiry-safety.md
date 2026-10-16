# 免费资格与到期处理全局安全方案

> **状态**：已实施（`v1.0.0-rc.31` 起，完成评审后的补充见 [B.3](#b3-完成评审后的补充)）— 2026-10-16 对照 rc.30 代码核对，修正见[附录 B](#附录-b实施前核对与修正2026-10-16)；以附录 B 为准。  
> **基线**：`v1.0.0-rc.30`（`998314c20a6f78e83587282ff49162854506a149`）  
> **日期**：2026-10-16  
> **受众**：pt-tools 维护者、实现者与评审者

## 摘要

本方案解决免费种子在等待磁盘、跨 RSS 周期、切换促销状态或暂停失败后产生计费下载量的问题。结论是：**修复必须落在全局资格模型、统一推送门禁和免费结束监控中，不能只给 M-Team 增加一个判断**；M-Team 另需在适配层收紧基础优惠、`promotionRule` 与 `mallSingleFree` 的解析。

核心规则如下：

1. “当前免费”“免费到某时”“明确永久免费”“状态未知”必须是不同状态；缺字段或解析失败不得解释为永久免费。
2. 需要免费的任务在加入下载器前必须持有本次流程产生的新鲜资格快照；历史数据库字段不能单独作为放行依据。
3. 免费结束暂停是端到端契约。只要入口承诺暂停，就必须保存来源、结束时间和下载器任务编号，并由统一推送服务登记监控。
4. 免费结束时间未知时，系统不能承诺准时暂停；开启暂停的自动任务应保持待处理或暂停加入，而不是自动开始。
5. M-Team 是高频暴露点，不是唯一受影响站点。所有站点驱动均输出同一种规范状态，站点差异只留在适配层。

## 1. 背景与现状

### 1.1 用户可见症状

典型时序如下：

```text
免费种子进入暂存目录
→ 磁盘保护拒绝推送，种子文件保留
→ 后续 RSS 周期继续扫描并尝试推送
→ 免费期结束或促销信息读取失败
→ 磁盘恢复后种子被加入下载器
→ 产生站点计费下载量
```

另一类症状不经过暂存等待：种子在免费期内正常加入，但系统到免费结束点才发出暂停请求；下载器与 tracker 的汇报存在延迟，最后一段流量可能在促销结束后结算。

### 1.2 当前链路的关键缺口

基线代码存在以下独立缺口：

- `TorrentItem.IsFree()` 只看优惠级别，不表达结束时间可信度；零时间同时代表永久、缺失和解析失败。
- M-Team 时间解析错误被忽略，基础优惠即使结束时间已过仍保留原优惠级别。
- RSS 重新获取详情时，已有 `TorrentInfo` 没有用本轮结果完整刷新 `FreeLevel`、`FreeEndTime`、`DownloadSource` 与 `FilterRuleID`。
- 暂存推送主要依赖历史 `GetExpired()`，不要求本轮成功验证资格，也不检查当前 `IsSkipped`。
- 暂存目录由 RSS 标签派生；同站点多条 RSS 使用相同或空标签时会共享目录。文件和种子记录没有稳定的 `rss_id` 归属，另一条 RSS 可能使用自己的下载器、路径和暂停开关接管推送。
- 传统 RSS 自动路径会登记免费结束监控，但通知按钮、App、MCP、AI 助手和媒体订阅等共享推送入口没有统一携带和执行该契约。
- `FreeEndAdvanceMinutes` 默认是 `0`，即到点才暂停，无法覆盖 tracker 汇报延迟。
- 现有测试覆盖了解析函数、磁盘保护和监控器单元行为，但缺少“M-Team 免费 → 磁盘拒绝 → 促销结束 → 空间恢复”的跨周期入口测试。

### 1.3 下载量口径

M-Team 同时返回两种数据：

- `downloaded`：计入分享率的下载量；
- `trueDownloaded`：实际传输量。

免费种子增加 `trueDownloaded` 属正常行为。故障判定必须以 `downloaded` 增长、分享率变化或站点明细为准，不能只看“真实数据”。日志和页面应始终并列标明两种口径。

## 2. 范围决策：全局实现，站点适配

### 2.1 必须全局实现的部分

下列语义与入口、站点无关，应只有一份实现：

- 规范化免费资格状态及其持久化；
- 来源策略：是否要求免费；
- 资格新鲜度与最终推送门禁；
- 暂存任务的 RSS 归属；
- 推送成功后的监控登记；
- 到期提前量、重试、对账和告警；
- 所有推送入口的一致行为；
- 计费下载量与实际传输量的展示口径。

### 2.2 留在站点适配层的部分

站点驱动只负责把原始响应转换成规范快照：

- M-Team：合并基础优惠、`promotionRule`、`mallSingleFree`，校验各自状态和时间；
- NexusPHP：解析优惠图标、剩余时间或明确的长期优惠；
- Unit3D、Gazelle、HDDolby、Rousi：按各自 API 输出同一结构；
- 无法证明永久或结束时间的，统一输出 `unknown`，不得自行猜测。

### 2.3 不应全局强制的部分

不是所有推送都必须免费。过滤规则允许非免费、用户明确手动推送或媒体订阅未要求免费时，系统可以放行。全局门禁必须读取“入口策略”，而不是简单写成 `if !isFree { reject }`。

## 3. 目标与非目标

### 3.1 目标

- 需要免费的任务不会使用过期、未知或陈旧资格加入下载器。
- 磁盘拒绝后重新推送前必须再次读取站点详情。
- 开启免费结束暂停的任务，要么获得可执行的结束时间并登记监控，要么不自动开始。
- 同站点多条 RSS 之间不串用下载器、保存路径、过滤来源或暂停开关。
- 所有入口通过同一个推送服务保存元数据并登记监控。
- 监控失败可观察、持续重试，并能与下载器真实状态对账。
- 现有明确允许非免费推送的能力保持可用。

### 3.2 非目标

- 不推测站点未公开的促销规则。
- 不保证外部 tracker 零延迟；通过提前量降低风险，并如实记录实际暂停时刻。
- 不在本方案中重做自动删种和按路径保种策略；两者可复用本文的统一策略/归属字段，但独立实施。
- 不把“实际传输量增长”定义成缺陷。

## 4. 领域模型

### 4.1 资格快照

新增规范对象，名称可在实现时按项目风格调整：

```go
type FreeValidity string

const (
    FreeNotFree  FreeValidity = "not_free"
    FreeUntil    FreeValidity = "free_until"
    FreePermanent FreeValidity = "free_permanent"
    FreeUnknown  FreeValidity = "unknown"
)

type PromotionSnapshot struct {
    Level      DiscountLevel
    Validity   FreeValidity
    EndsAt     *time.Time
    VerifiedAt time.Time
    Source     string
}
```

不变量：

- `free_until` 必须有可解析且晚于 `VerifiedAt` 的 `EndsAt`；
- `free_permanent` 必须来自站点明确语义，不能由空时间推导；
- 非空时间解析失败得到 `unknown` 和错误原因；
- `unknown` 不是免费放行凭据；
- `EndsAt` 统一按绝对时间存储，展示时再转换时区。

### 4.2 入口策略

```go
type AdmissionPolicy struct {
    RequireFree       bool
    PauseOnFreeEnd    bool
    SafetyMargin      time.Duration
    SourceRSSID       *uint
    SourceKind        string
}
```

策略来源：

| 入口                                         | `RequireFree`                                   |
| -------------------------------------------- | ----------------------------------------------- |
| RSS 免费通道、仅免费模式、要求免费的过滤规则 | `true`                                          |
| 不要求免费的过滤规则                         | `false`                                         |
| 刷流任务                                     | 由允许优惠集合决定，并在推送前再次验证所选优惠  |
| App、MCP、AI 助手、手动推送                  | 默认 `false`；调用方明确选择“仅免费”时为 `true` |
| 媒体订阅                                     | 由订阅的免费偏好决定                            |

### 4.3 持久化字段

在 `TorrentInfo` 增加或等价表达：

```text
source_rss_id             可空，产生暂存任务的 RSS
require_free              是否以免费为入场前提
free_validity             not_free/free_until/free_permanent/unknown
free_verified_at          最近一次成功验证时间
free_status_source        base/promotion_rule/mall_single_free/adapter
pause_on_free_end         现有字段，继续使用
```

`FreeLevel`、`FreeEndTime` 与 `IsFree` 暂时保留兼容，但由规范快照统一写入，不再分别拼装。所有更新使用明确列集合，避免零值或旧结构体把新状态覆盖回去。

## 5. 统一处理流程

### 5.1 RSS 获取与暂存

1. 拉取 RSS 条目。
2. 用站点详情接口获取本轮实时快照。
3. 根据过滤规则生成 `AdmissionPolicy`。
4. 若 `RequireFree=true`：
   - `free_until`：检查剩余时间、预计下载时长和安全提前量；
   - `free_permanent`：允许；
   - `not_free`：跳过；
   - `unknown`：保持待处理，不自动开始，并记录原因。
5. 原子写入完整快照、策略、`source_rss_id` 和暂存文件信息。
6. 只有本 RSS 本轮验证通过的候选才能进入推送。

`remaining <= 0` 必须直接判定过期；不能只拦截 `0 < remaining < MinFreeMinutes`。

### 5.2 延迟重推

磁盘、站点容量或下载器故障导致未推送时：

- 保留暂存文件，不增加“种子自身失败”的重试计数；
- 下个周期先按 `site + torrent_id` 重新取详情；
- 资格验证失败、条目消失或请求失败时不得沿用旧快照推送；
- 只有新的快照通过同一策略后才重试下载器操作；
- 超过暂存保留期按现有安全清理规则删除本地 `.torrent`。

### 5.3 最终推送门禁

`AddTorrentFileEx` 前执行最后一道纯函数判定：

```text
policy 不要求免费
  → 放行，不伪造免费承诺
policy 要求免费
  → 快照必须来自本次尝试且未超出 freshness TTL
  → free_until: EndsAt - now > SafetyMargin
  → free_permanent: 放行
  → not_free / unknown / 过期: 拒绝
```

门禁位于共享推送服务中，传统 RSS 的目录扫描不能绕过。磁盘检查之后等待过久时，应在真正调用下载器前再次比较时间；跨过安全边界则释放磁盘预留并拒绝。

### 5.4 推送成功与监控登记

共享 `PushTorrentRequest` 携带策略和快照。推送成功后在同一业务流程内：

1. 写入 `is_pushed`、下载器 ID/名称和任务 ID；
2. 保存最终资格快照；
3. 若 `PauseOnFreeEnd=true` 且为 `free_until`，登记监控；
4. 若承诺暂停但状态为 `unknown`，不得以运行状态加入；
5. 登记失败不把结果伪装成完整成功，返回“已加入但监控登记失败”的降级状态并立即触发对账。

RSS 通知按钮必须继承对应 RSS 的 `PauseOnFreeEnd` 和 `SourceRSSID`。其他入口只有显式请求暂停时才登记，避免把某条 RSS 设置误当成站点全局设置。

### 5.5 免费结束监控

监控器继续采用精确定时器加周期对账，但收紧以下规则：

- 新安装的建议提前量为 5 分钟；已有明确配置保持不变，值为 0 时页面持续提示 tracker 汇报风险；
- 查询条件使用规范状态，不以 `IsFree` 和空时间的组合猜测；
- 到期前先读取下载器任务，未完成则暂停或按用户设置删除；
- 下载器请求失败持续指数退避，周期对账不得因达到短期重试次数而永久放弃；
- 连续失败达到告警阈值后写入可见状态并发送通知；
- 成功调用暂停后再次读取任务，确认已进入暂停/停止状态；未确认则继续重试；
- 记录计划暂停时间、实际请求时间、确认时间和偏差。

## 6. M-Team 适配要求

M-Team 解析按以下顺序产出一个快照，而不是分别写多个松散字段：

1. 解析基础 `discount` 和 `discountEndTime`；
2. 判断 `promotionRule` 是否已开始且未结束；
3. 判断 `mallSingleFree.status == ONGOING` 且当前时间落在有效区间；
4. 选择当前实际生效且下载优惠更好的规则；同级优惠选择有效期更晚者；
5. 非空时间解析失败返回 `unknown`，并保留脱敏后的字段名和错误类型用于诊断；
6. 基础结束时间已过时，基础优惠不能继续标记为当前免费；
7. `promotionRule` 无结束时间只有在 API 明确表达长期有效时才可输出 `free_permanent`；
8. 详情接口返回 `FREE` 但没有任何可信结束/永久信息时输出 `unknown`。

测试夹具至少覆盖：基础免费、2X 免费、基础优惠过期、促销覆盖、商城进行中/已结束、缺失时间、非法时间、API 字段组合变化。夹具必须脱敏，不提交 API Key、签名下载地址或真实用户信息。

## 7. 多 RSS 归属与并发

### 7.1 暂存隔离

暂存目录改为稳定的 RSS 身份，例如：

```text
<download-base>/rss-<rss_id>/<site>-<torrent_id>.torrent
```

用户标签只用于下载器标签，不再承担内部队列隔离。兼容旧目录时只读迁移：能唯一关联一条 RSS 才接管；无法唯一判断的文件保持不推送并列入诊断。

### 7.2 种子所有权

`TorrentInfo` 仍可维持 `site_name + torrent_id` 唯一，但首次进入待推送状态时原子确定 `source_rss_id`。其他 RSS 看见同一种子时：

- 已有活动所有者：不覆盖下载器、路径、暂停和资格策略；
- 原所有者被删除/停用：通过显式接管事务重新验证后转移；
- 多来源策略冲突：默认取更保守策略，且记录冲突供用户查看。

## 8. API、界面与可观察性

### 8.1 API

推送结果增加机器可判定状态：

```text
pushed
skipped_duplicate
held_free_unknown
rejected_not_free
rejected_free_expired
rejected_free_stale
pushed_monitor_degraded
```

不要只返回模糊的 `success=false`。列表接口输出来源 RSS、免费状态、验证时间、结束时间、计划暂停时间和监控状态。

### 8.2 Web UI

RSS 页面显示：

- “要求免费”与“免费结束暂停”是两项不同设置；
- 当前提前量及值为 0 的风险；
- 暂存任务因何等待；
- 结束时间未知时明确写“无法保证到期暂停，未自动开始”。

任务详情显示：

- 优惠级别和资格来源；
- 最近验证时间；
- 计划/实际暂停时间；
- `downloaded` 与 `trueDownloaded` 的口径说明。

### 8.3 日志与指标

每条日志使用 `site`、`torrent_id`、`info_hash`、`source_kind`、`source_rss_id` 关联，但不得打印 RSS 密钥、API Key 或签名地址。至少增加：

```text
free_eligibility_rejected_total{reason,site}
free_end_schedule_total{site}
free_end_pause_delay_seconds{site}
free_end_pause_failure_total{site,stage}
deferred_push_revalidated_total{site,result}
```

## 9. 数据迁移与兼容

- 新字段通过模型迁移创建；涉及既有数据语义转换时增加编号迁移。
- `FreeEndTime != NULL` 且 `IsFree=true` 的旧记录可回填为 `free_until`。
- `IsFree=true` 且 `FreeEndTime=NULL` 的旧记录回填为 `unknown`，不得回填为永久免费；运行中任务由后台对账重新查询站点。
- 已经完成或用户手动管理的任务不因迁移自动删除、恢复或改路径。
- 无法推断 `source_rss_id` 的旧暂存文件不自动推送。
- 不批量执行 `db fix-timezone`；只有证据表明记录来自旧错误解析时才使用受控迁移。
- 明确允许非免费推送的现有过滤规则保持行为不变。

## 10. 实施阶段

### 阶段 A：先固定回归

在不改生产逻辑前增加失败测试：

- M-Team 缺失/非法/已过期结束时间；
- 已有记录从无结束时间刷新为有结束时间；
- 已有记录从免费变成非免费；
- 磁盘拒绝后促销结束，再恢复空间；
- 两条同站点、同标签 RSS 的暂停配置不串用；
- 通知按钮继承 RSS 暂停策略；
- 暂停请求成功但状态未改变。

### 阶段 B：规范状态与 M-Team 解析

主要改动范围：

```text
site/v2/types.go
site/v2/mtorrent_driver.go
site/v2/definitions/mteam_fixture_test.go
models/init.go
models/schema_version.go
```

先完成纯函数、解析和迁移，保持下载器行为不变。

### 阶段 C：RSS 归属和最终门禁

主要改动范围：

```text
internal/common.go
internal/unified_site.go
internal/push.go
core/config_store.go
models/config_models.go
```

让每个暂存项具备 RSS 所有者和新鲜快照，并将最终资格门禁下沉到共享推送服务。

### 阶段 D：统一监控登记和对账

主要改动范围：

```text
scheduler/free_end_monitor.go
scheduler/manager.go
internal/app/rss_callback_actions.go
internal/media/subscribe/evaluate.go
web/api_app_v1_actions.go
web/api_torrent_push.go
```

消除入口差异，增加暂停确认、持续重试和降级结果。

### 阶段 E：界面、文档和发布观测

更新 RSS、全局设置、任务详情和暂停任务页面；补充中英文用户文档。先在预览版观察至少一个完整促销窗口，再进入稳定版。

## 11. 验证矩阵

| 场景                                            | 期望                         |
| ----------------------------------------------- | ---------------------------- |
| 明确非免费，策略要求免费                        | 不下载 `.torrent` 或不推送   |
| 明确非免费，规则允许非免费                      | 正常推送，不登记免费暂停     |
| 限时免费且时间充足                              | 推送并登记提前暂停           |
| 限时免费已过期                                  | 最终门禁拒绝                 |
| 免费状态未知且开启暂停                          | 不自动开始，状态可见         |
| 磁盘不足后仍免费                                | 重新验证后才重推             |
| 磁盘不足后变非免费                              | 永不调用下载器添加接口       |
| 多 RSS 共站点、同标签                           | 各自只处理自己的暂存项       |
| 进程重启                                        | 从数据库重建已知结束时间任务 |
| 暂停接口暂时失败                                | 持续重试并告警，不静默放弃   |
| M-Team `trueDownloaded` 增长、`downloaded` 不变 | 标记为正常实际传输           |

最低自动验证：

```bash
CGO_ENABLED=1 go test ./site/v2 ./internal ./scheduler ./web -count=1
CGO_ENABLED=1 go test -race ./internal ./scheduler -count=1
pnpm --dir web/frontend test
make fmt-check
make lint
make docs-check
```

合入前再执行项目完整 `make test` 和 `make build`。

## 12. 发布、回滚与验收

### 12.1 发布策略

1. 首个预览版默认只启用新解析、诊断和最终拒绝，不自动改动已有任务。
2. 展示 `unknown` 数量和受影响站点，确认没有大面积误判。
3. 下一预览版启用统一监控登记和 RSS 归属。
4. 新安装默认提前 5 分钟；已有安装保留当前值并显示风险提示。
5. 收集 M-Team 至少一次限时优惠结束前后的 `downloaded`、暂停时间与日志证据。

### 12.2 回滚

- 新字段只增不删，旧二进制可忽略；迁移不得重写旧核心字段到不可逆格式。
- 新门禁提供单一兼容开关，仅用于紧急回滚；开关状态必须有高风险告警，不能长期作为默认。
- 回滚不恢复已暂停任务，避免自动产生下载量；由用户显式恢复。

### 12.3 完成标准

以下条件全部满足才算完成：

- 全部推送入口共享最终资格门禁和监控登记；
- M-Team 缺失、非法和过期时间均不再被当作永久免费；
- 已有记录能刷新资格和结束时间；
- 磁盘等待跨周期回归通过；
- 多 RSS 不串配置；
- 到期暂停有计划、执行、确认和失败证据；
- “数据量”与“真实数据”在 API、页面和文档中口径一致；
- 目标测试、race、前端测试、格式、lint、文档检查、完整测试和构建均通过。

## 附录 A：后续实施 Goal 提示词

```text
你正在 /config/workspace/ProdDir/pt-tools 实施“免费资格与到期处理全局安全方案”。先阅读 AGENTS.md、子目录 AGENTS.md、docs/design/free-expiry-safety.md；若 .codegraph 可用，先执行 codegraph status，状态不可用时不要自行初始化。开始前记录当前分支、HEAD、origin/v1.0.0-rc 的最新 SHA、工作区和 worktree 状态；以执行时最新预览分支为基线，保留所有无关改动，不提交、不推送，除非用户另行授权。

目标：修复需要免费的任务在磁盘等待、跨 RSS 周期、促销变化、入口切换或暂停失败后产生计费下载量的问题。实现必须是全局能力，M-Team 只在站点适配层增加基础 discount、promotionRule、mallSingleFree 的严格解析。

硬性语义：
1. 区分 not_free、free_until、free_permanent、unknown。缺失时间和解析失败不得当作永久免费；非空时间解析失败必须可观察。
2. 为每次推送生成 AdmissionPolicy，至少含 RequireFree、PauseOnFreeEnd、SafetyMargin、SourceRSSID、SourceKind。允许非免费的过滤规则和明确手动推送不得被一刀切拦截。
3. RequireFree=true 时，AddTorrentFileEx 前必须持有本次尝试的新鲜资格快照；free_until 需 EndsAt-now>SafetyMargin，unknown/not_free/过期/陈旧一律拒绝。磁盘检查后若等待跨过边界，释放预留并拒绝。
4. RSS 磁盘拒绝后的下一次推送必须重新读详情；条目消失或详情失败不得沿用旧资格。
5. 已有 TorrentInfo 每轮完整刷新免费级别、有效性、结束时间、验证时间、来源和过滤规则，不能用旧结构体回写旧值。
6. 暂存项绑定 rss_id；相同站点、相同或空标签的多 RSS 不得互相处理文件，也不得覆盖下载器、路径或暂停开关。
7. 将策略和快照加入统一 PushTorrentRequest。所有承诺免费暂停的入口都由 PushTorrentToDownloader 在写入 downloader_task_id 后统一登记监控；RSS 通知按钮必须继承对应 RSS 设置。
8. 开启暂停但结束时间 unknown 时不得自动开始。监控采用精确定时器加周期对账，暂停后读取下载器确认；暂时失败持续退避重试并告警，不静默放弃。
9. M-Team：基础优惠过期不得继续有效；promotionRule/mallSingleFree 必须校验状态和起止时间；只有 API 明确表示长期有效才能产出 free_permanent。
10. 不把 trueDownloaded 增长当作计费缺陷；用户统计、日志和页面明确区分 downloaded 与 trueDownloaded。

按 TDD 实施：先增加能稳定失败且失败原因正确的回归测试，再做最小实现。必测：M-Team 缺失/非法/过期时间；第一次无结束时间后刷新成功；免费转非免费；磁盘拒绝→优惠结束→空间恢复且下载器 Add 调用为 0；同站两条 RSS 同标签但暂停配置不同；RSS 通知按钮继承策略；暂停接口成功但状态未改变；明确允许非免费仍可推送；重启重建监控。

优先复用现有接口和 SQLite 数据源，不新建平行配置文件。涉及 schema 时同时更新 AutoMigrate 和必要的编号迁移；旧 IsFree=true 且 FreeEndTime=NULL 回填 unknown，不推断永久，不自动删除或恢复任务。日志不得输出 API Key、RSS 密钥、Cookie、Passkey 或签名下载地址。

完成实现后运行：CGO_ENABLED=1 go test ./site/v2 ./internal ./scheduler ./web -count=1；CGO_ENABLED=1 go test -race ./internal ./scheduler -count=1；pnpm --dir web/frontend test；make fmt-check；make lint；make docs-check；make test；make build。失败必须修复，不得删除、跳过或弱化测试。最终报告冻结的 base/HEAD、修改文件、行为变化、迁移兼容、实际执行的每项验证、未验证项和剩余风险；不要声称仅靠单元测试证明 M-Team 线上计费已修复，线上结论需要一个真实促销窗口的脱敏证据。
```

## 附录 B：实施前核对与修正（2026-10-16）

对照 `998314c2` 逐条核对后，正文的判断大体属实；下面是遗漏、不准确之处与实施时的取舍，冲突时以本附录为准。

### B.1 现状补充

1. **RSS 暂存推送不经过共享推送服务**：`internal/common.go` 的 `processSingleTorrentWithDownloader` 自己做磁盘保护、站点容量和 `AddTorrentFileEx`，并自己登记监控；`PushTorrentToDownloader` 既没有资格门禁也不登记监控。要做到 §5.3「不能绕过」与「统一登记」，RSS 暂存推送必须改走 `PushTorrentToDownloader`。
2. **暂存目录跨站点共用**：目录只由标签决定（`utils.SubPathFromTag`），不同站点同标签（含空标签）共用一个目录；`filterTorrentFilesBySite` 在目录里没有本站文件时退回处理全部文件，随后按「本站 + 哈希」查不到记录，把别的站点的暂存种子当孤立文件删掉。
3. **已有记录刷新**：upsert 写回的是旧结构体里的 `free_level`、`free_end_time`、`download_source`、`filter_rule_id`、`tag`，而 `is_free`、`is_skipped` 是本轮的，会出现 `is_free=1` 配旧结束时间的组合。暂存推送只看 `GetExpired()`（结束时间为空且优惠级别非 NONE 时永不过期，半价也算），不看 `is_skipped`、`is_free`。
4. **M-Team**：只要 `promotionRule.discount` 非空，`mallSingleFree` 就完全不看；促销、商城时间解析失败会变成「已开始、无结束」；商城单种免费固定返回 FREE，不和基础 2XFREE 比较；`promotionRule` 的作用范围（`categories`、`teams`）完全没有解析，只覆盖部分分类的促销会让所有种子都算免费。用户信息的 `definitions/mteam.go` 选择器只是声明，实际解析在 `mtorrent_driver.go`。
   第三方对 500 个真实 `/api/torrent/search` 样本的实测（[autopt `docs/sites/mteam/api-empirical.md`](https://github.com/Well2333/autopt/blob/main/docs/sites/mteam/api-empirical.md)）表明：`promotionRule` 的结构是 `{id, categories, teams, discount, startTime, endTime, operator}`，**没有状态字段**，`categories`/`teams` 为空表示全站、非空时须包含本种的 `category`/`team`，生效条件是 `startTime ≤ now ≤ endTime`；全站限免期间多数种子的 `discount` 仍是 PERCENT_50/NORMAL，免费只来自 `promotionRule`；`mallSingleFree` 是 `{status, endDate, …}`，`status == ONGOING` 表示进行中。
5. **没有结束时间的站点**：Gazelle（MooKo）接口没有结束时间；NexusPHP 长期免费不显示结束时间；HDDolby 详情来自最近 100 条的缓存，查不到时返回「非免费」；Unit3D 没有详情接口（目前没有站点使用）。Rousi 详情 404 返回 `(nil, nil)`，RSS 工作协程会空指针崩溃。
6. **入口**：通知按钮不取详情、不带来源与元数据，且通知发出时 RSS 记录还没写入；App/MCP/AI 助手、网页手动推送、订阅、刷流、qB 兼容入口、转移做种都不登记监控。`/api/downloader-torrents/add` 直接调用下载器（不写种子记录、不承诺暂停），不在本方案范围。刷流把半价等非免费优惠的结束时间也写进 `free_end_time`。
7. **监控器**：内层重试共用 10 秒上下文，实际只试一次；`retry_count` 与推送失败共用；暂停调用成功后不回读，标成已暂停后不再复查；到期处理用登记时的结构体副本，不重读数据库（免费延长或关掉暂停后仍会触发）；没有告警。
8. **提前量**：明确设成 0 与默认 0 无法区分；把 gorm 默认值改成 5 会让首次写入时的明确 0 变成 5（零值字段走默认值）。
9. **下载量口径**：`downloaded`、`trueDownloaded` 只有账号级的当前值，每日快照不存 `trueDownloaded`，也没有按种子的数据；页面「真实数据」列默认隐藏，且只在实际上传量与上传量不同时显示。
10. 主服务没有指标系统（Prometheus/expvar），§8.3 的指标没有落点。架构版本当前是 v11（`models/AGENTS.md` 写的 v10 已过时）。
11. **暂存文件的其他自动处理**：每条 RSS 的推送循环都会跑保留期清理（`sweepStagingDir`），清理只按「站点 + 哈希」查记录，不看是哪条 RSS 的。
12. **同一种子多处加入**：`TorrentInfo` 按「站点 + 种子编号」唯一，第二次推到另一个下载器会覆盖第一次的下载器与任务编号，第一处的到期监控就丢了。
13. **HDDolby 的详情来自缓存**：最近 100 条搜索结果，最多 5 分钟刷新一次；在调用时盖上「现在」作为验证时间，并不等于重新看过站点。
14. **归档表**（`TorrentInfoArchive`）复制种子记录的列，新字段也要跟着复制。

### B.2 方案修正

1. **一次发布全部语义**：§10、§12.1 的「首个预览版只启用解析与拒绝、下一预览版再启用监控与归属」改为同一版实现全部硬性语义；不自动改动已有任务（迁移只回填状态，不暂停、不恢复、不删除）。
2. **不做兼容开关**：§12.2 的单一兼容开关去掉，一个能关掉门禁的开关等于给计费缺陷留了入口。新字段只增不删，但换回旧二进制**不是**安全回滚：旧代码把「结束时间为空且有优惠级别」当成永不过期。回滚前先停掉所有会自动新增下载任务的生产者：RSS（关掉全局自动启动只挡住 RSS 任务）、刷流任务、媒体订阅都要停用，经 qB 兼容入口推送的外部自动化（如 MoviePilot）也要暂停；或者直接停用所有相关站点，并确认这些生产者确实推不出任务。再确认下载器里正在下载的免费种子已暂停或完成，最后换二进制。
3. **暂存隔离用归属而不是改目录**：§7.1 的 `rss-<rss_id>` 目录改为：目录布局不变（维护清理与已有暂存文件照常可用），`TorrentInfo.source_rss_id` 记录所有者。所有自动处理暂存文件的路径——推送、已推送/已存在/过期/超重试的删除、保留期清理、失败清理、接管——都只处理所有者是本 RSS 的文件；去掉跨站点回退；没有记录的本站文件仍按孤立文件清理；没有所有者的旧记录只有在本站点只有一条启用 RSS 使用这个目录时才接管，否则保留不推送并写明原因。
4. **所有权**：所有者在 RSS 首次决定下载（进入暂存）时用条件更新确定（`WHERE source_rss_id IS NULL OR source_rss_id = 本 RSS OR 原所有者已删除/停用`，看影响行数），不先读后写；所有者在且启用时，其他 RSS 不改它的策略字段（是否跳过、来源、过滤规则、是否要求免费、暂停、标签），也不再暂存；所有者被删除或停用时，下一条决定下载的 RSS 用同样的条件更新接管并重新验证。资格快照是种子本身的事实，任何一条 RSS 读到都可以刷新。
5. **新鲜快照**：`RequireFree=true` 的推送必须带本次尝试的开始时间（编排层给出，RSS 是这一轮的开始），没有就按陈旧拒绝；快照的 `VerifiedAt` 是底层数据**实际取得**的时间（驱动给出，HDDolby 是缓存刷新的时间），必须不早于本次尝试的开始，且不超过 15 分钟。RSS 推送阶段对不满足的暂存项按 `site + torrent_id` 重新取详情（用记录里不含密钥的种子编号；HDDolby 缓存早于本次尝试时先刷新），条目消失、详情失败或数据仍旧，一律不推送。`SafetyMargin = max(5 分钟, 提前量)`；`MinFreeMinutes` 的检查改为剩余时间小于它（含已过期）即不下载。
6. **拒绝只针对要求免费**：`RequireFree=true` 时，unknown 不推送（`held_free_unknown`），暂存文件保留到保留期、期间每轮重新验证；not_free 标记跳过并删除暂存文件（之后再变免费时，下一轮 RSS 重新判断并重新下载，见 B.3）；免费已结束的标记过期并删除。`RequireFree=false`（不要求免费的过滤规则、通知按钮、手动推送）照常推送，不做这些拒绝和删除；其中开启了免费结束暂停的：free_until 登记监控，unknown 以暂停状态加入（加入前决定），not_free 不登记。
7. **承诺暂停的推送先暂停加入、记账成功再开始**：承诺免费结束暂停的推送一律以暂停状态加入；`downloader_task_id`、策略、快照与暂停计划在同一条 UPDATE 写入成功后，下载器设成自动开始的才发开始命令。写库失败时任务保持暂停，返回 `pushed_monitor_degraded`；RSS 的暂存文件只在全部成功后才删，下一轮发现下载器里已有、且记录上留着本次加入的标记时，重新过门禁、补记账，再按设置开始。下载器里原本就有（不是本次加入的）只记账、登记监控，不改它的运行状态。内存定时器只是加速，周期对账从库里重建。
8. **通知按钮**：继承通知日志里那条 RSS 的 `PauseOnFreeEnd` 与 `SourceRSSID`；按钮是用户明确操作，`RequireFree=false`；按钮处理时自己取一次详情得到快照，取不到就是 unknown（开启暂停时以暂停状态加入）。
9. **各入口策略**：每个新增下载任务的入口都显式构造 `AdmissionPolicy`。App/MCP/AI 助手/网页手动推送/qB 兼容入口：`RequireFree=false`、不承诺暂停（本次不新增「仅免费」「到期暂停」请求参数）；订阅：免费偏好为 require 时 `RequireFree=true`；刷流：允许的优惠全是免费级别时 `RequireFree=true`，快照来自本次刚取到的搜索结果；转移做种：复用已有数据，`RequireFree=false`、不承诺暂停；下载器页面直接加任务（`/api/downloader-torrents/add`，磁力或任意种子文件，没有站点与种子编号、不写种子记录）：按明确手动操作构造 `RequireFree=false` 的策略并过同一个门禁函数，不承诺暂停。
10. **监控器**：处理前重读记录（不再满足条件就取消，结束时间变晚就重新预约）；暂停后回读下载器，状态为 paused 或 stopped 才算确认；暂停与回读失败按 30 秒起、10 分钟封顶的指数退避一直重试，计数用单独的列，不和推送重试共用；连续失败 3 次起在任务上显示并向所有启用的通知通道发一次告警；记录计划暂停时间、请求时间与确认时间。
11. **提前量默认值**：新安装在写入默认全局设置时设 5 分钟；已有安装保持原值，页面在值为 0 时提示汇报延迟风险。
12. **可观察性**：§8.3 的指标改为结构化日志字段（`site`、`torrent_id`、`source_kind`、`source_rss_id`、`reason`）加上任务上的持久状态（免费状态、验证时间、来源、错误、暂停计划/请求/确认时间、失败次数），不新建指标系统。
13. **没有结束时间的站点的后果**：按「缺失时间不当永久」，MooKo 的免费、NexusPHP 不显示结束时间的长期免费、HDDolby 缓存里找不到的种子，在要求免费的通道里都不会推送，页面写明原因；需要时可用不要求免费的过滤规则放行。HDDolby 缓存未命中改为详情失败，Rousi 404 改为返回错误，RSS 工作协程对空详情做检查。
14. **下载量口径**：不新增按种子的下载量；用户统计页把「上传量/下载量（计费）」与「实际传输」的口径写明，「真实数据」在站点提供时显示；文档写明免费种子只增加实际传输量属正常。
15. **同一种子只允许一处受监控的加入**：种子已经在某个下载器里、且正受免费结束监控时，再推到另一个下载器会被拒绝（`rejected_active_placement`），不覆盖原来的下载器与任务编号；推到同一个下载器按重复处理。
16. **归档**：归档表增加同样的新列并在归档时复制。
17. **下载量口径的范围**：用户统计页、手机 App、每日战报、MCP/App 接口输出与日志里出现账号下载量的地方都标明是「下载量（计费）」还是「实际下载」；每日快照只存计费口径，历史趋势写明「只有计费口径」。

### B.3 完成评审后的补充

1. **暂存文件按文件名对应记录**：暂存文件名是 `<站点>-<种子编号>.torrent`，推送与清理按它取记录并核对哈希。同一个哈希还可能有别的入口留下、编号不同、策略更宽的记录，不改用它们。文件名里的编号没有记录时按孤立文件清理；文件与记录的哈希对不上（站点更新过种子文件、或下载后哈希没写进记录）时删除暂存文件，下一轮重新下载。文件名里没有编号的（兼容入口）才按哈希找，对上不止一条就不推。
2. **计划暂停时刻已到的不开始**：承诺到期暂停的推送（含补记账），计划暂停时刻（结束时间减提前量）已到或一分钟内就到时只暂停加入、不开始，由监控器确认暂停；内存定时器在开始以后才登记，到期暂停不会抢在开始之前执行、又被开始撤掉。
3. **硬性语义 5 的范围**：RSS 每轮对订阅里的每个条目读详情并整套刷新资格，已跳过的记录也按这一轮重新判断（取代「不免费的 6 小时后重新检查」）。已推送的记录不再推送，资格照样每轮刷新，下载策略（来源、过滤规则、是否要求免费）保持推送时的值；受到期监控的还按新的结束时间改计划暂停。自动删种的「免费到期未完成删除」（`CleanupDelFreeExpired`）原来按推送时写下的 `is_free`、`free_end_time` 判断，和当前资格共用字段：刷新后，免费延长就等新的结束时间；原来免费、读到「不免费」的记下 `free_ended_at`（不晚于读到的时间，再变回免费时清掉），删种按「`is_free = 1` 且结束时间已过，或 `free_ended_at` 已过」判断（按时刻比较）。代价是每轮对站点的详情请求数约等于订阅的条目数。
