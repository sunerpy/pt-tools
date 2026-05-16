# RSS 上新通知

[返回首页](../../README.md) · [ChatOps 快速开始](chatops-quickstart.md)

pt-tools 在 RSS 自动下载之外，还可以把"上新"事件通过 **QQ 私聊** 或 **Telegram 私聊** 实时推给你。
本指南帮你快速打开这个开关，并把通知质量调到最舒服的状态。

---

## 目录

- [简介](#简介)
- [快速上手（5 步）](#快速上手5-步)
- [两种通知模式：all vs filtered](#两种通知模式all-vs-filtered)
- [静默时段](#静默时段)
- [digest 合并](#digest-合并)
- [失败重试](#失败重试)
- [Telegram 内联按钮](#telegram-内联按钮)
- [每小时配额](#每小时配额)
- [DB schema 速查](#db-schema-速查)
- [故障排查 FAQ](#故障排查-faq)

---

## 简介

**RSS 上新通知**是一个独立于"自动下载"的事件流：

| 路径                          | 触发时机                          | 用途                                                 |
| ----------------------------- | --------------------------------- | ---------------------------------------------------- |
| **不通知**                    | -                                 | 关闭此 RSS 的通知                                    |
| **全部新种（简略）**          | RSS feed 拉到一条新条目时立即触发 | 任何新种都通知我，仅含标题/链接，不消耗站点详情请求  |
| **只通知匹配的（详细）**      | 详情页拉完、过滤规则命中时触发    | 只通知"我真的要追的剧 / 蓝光原盘"，需要详情才能判断  |
| **全部都通知 + 匹配的给详细** | 上面两路都启用（合并展示）        | 既不漏种，又能在命中规则时看到完整信息，避免重复通知 |

> 通知和自动下载完全独立：你可以"只接收通知不下载"、"只下载不通知"、"两个都开"。

通知投递走的是 ChatOps 通道（NotificationConf）。
所以使用本功能前，**必须先有一个可用的 ChatOps 通道**，详见：

- [QQ OneBot (NapCat) 配置](chatops-qq-napcat.md)
- [Telegram Bot 配置](chatops-telegram.md)

---

## 快速上手（5 步）

### 第 1 步：准备 ChatOps 通道

按 [快速开始 → ChatOps](chatops-quickstart.md) 创建至少一个 NotificationConf（QQ 或 Telegram）。

确认在 Web 界面 → "ChatOps 管理" 页面看到通道状态为 **健康**。

### 第 2 步：在 RSS 订阅页打开通知

进入 Web 界面 → "RSS 订阅"，新建或编辑一条订阅，找到下方"通知设置"分区：

- **通知模式**：选"全部新种（简略）" / "只通知匹配的（详细）" / "全部都通知 + 匹配的给详细"，留空表示关闭
- **通知通道**：勾选第 1 步创建的 NotificationConf
- **每小时配额**：默认 100，超额会落 `throttled` 状态（见下方"每小时配额"）

保存即可生效——下次 RSS 调度执行时新种会触发通知。

### 第 3 步（可选）：配置过滤规则用作通知触发器

如果你选了 `filtered` 或 `both`，那么需要至少一条 `purpose` 为 `notify` 或 `both` 的过滤规则。

进入 "过滤规则" 页面，新建规则：

- **purpose（用途）**：
  - `download`：仅用于自动下载（默认）
  - `notify`：仅用于通知 → 命中后触发 filtered 通知
  - `both`：两者都触发

> **注意**：`purpose` 是 Sprint 2 引入的字段。如果你用的是更早的版本升级上来，旧规则默认是 `download`，要手动改。

### 第 4 步（可选）：配置静默时段 / 配额

在 ChatOps 通道（NotificationConf）编辑页：

- **静默时段开始 / 结束**：`HH:MM` 格式，例如 `23:30` → `07:30`
- 跨午夜支持，留空两个字段表示无静默
- 静默期内的通知会被自动**延迟到结束时刻**再投递（不是丢弃）

在 RSS 订阅编辑页：

- **每小时最大通知数**：默认 100；置 0 等同关闭限流

### 第 5 步：等通知到达 / 排查

下次 RSS 调度（默认每 5 分钟）会拉到新种并触发。

打开 Web 界面 → "RSS 通知日志"，可以看到每条通知的：

- `result`：sent / failed / suppressed / pending / throttled
- `attempts`：投递尝试次数
- `last_error`：最近一次错误（用于排查）
- `payload_json`：实际发送的标题 / 正文

---

## 四种通知模式详解

### 不通知（默认）

- **说明**：关闭此 RSS 的通知功能
- **适用**：只需要自动下载，不需要新种通知

### 全部新种（简略）

RSS 任何新条目都触发，包含 RSS feed 自带的标题和链接，**不会拉详情页**，所以信息很简略。

- **触发时机**：RSS feed 拉到 `<item>` 那一瞬间
- **数据源**：仅 feed 字段（title、link、pubDate）
- **不消耗站点请求**：不会拉详情页，节省请求额度
- **适用**：刷流玩家 / 喜欢"任何上新都看一眼"

通知正文示例：

```
🆕 [hdsky] Test.Movie.2026.1080p

📅 2026-05-16 12:00
🔗 https://hdsky.me/details.php?id=12345
```

### 只通知匹配的（详细）

拉完详情后按规则匹配，**只有命中规则的种子才触发通知**，但消息含完整详情（大小/免费/规则名）。

- **触发时机**：详情页拉完、`FilterService.MatchRulesWithInput` 命中规则之后
- **数据源**：详情页（容量、分辨率、免费状态、标签等）
- **会消耗站点请求**：和正常自动下载链路共享
- **适用**：追剧 / 挑蓝光 / 选种（需要详情才能判断）

通知正文示例：

```
🎯 [hdsky] Test.Movie.2026.1080p

📦 12.34 GB
🆓 免费 (剩余 23h45min)
📌 匹配规则：4K 蓝光原盘
🔗 https://hdsky.me/download.php?id=12345
```

### 全部都通知 + 匹配的给详细

同时启用上面两路。同一条种子如果两路都命中，pt-tools 会自动合并：

- 先入队"全部新种"那条（简略）
- 详情拉完后命中规则 → 这条简略的会被改为 `suppressed`（不发出去），改发"只通知匹配的"那条详细的
- 结果：同一种子永远只收到 1 条通知；命中规则的那种是详细版本，没命中的是简略版本

适合"想看全部新种，但希望命中关键词的能看到完整信息"的场景。

`(rss_id, site_name, torrent_id, notify_kind, conf_id)` 上有唯一索引保证幂等：
**同一个种子的同一类通知不会被发两遍**。

---

## 静默时段

适合"白天工作、晚上睡觉"的场景。

- 字段：`NotificationConf.quiet_hours_start`、`quiet_hours_end`
- 格式：`HH:MM`（24 小时制），全空表示无静默
- **跨午夜支持**：`23:30` → `07:30` 表示晚 11:30 到次日早 7:30 都安静
- **不丢消息**：静默期内，通知行写入数据库（result=`pending`），retry worker 会等到静默结束后再投递

> 静默判断是**按通道**做的：同一条通知投往多个通道时，只有处于静默的通道延迟，其他通道照常发送。

---

## digest 合并

短时间内大量上新会触发"digest 合并"：

- **窗口**：30 秒
- **阈值**：5 条
- **行为**：满足阈值或窗口结束时，把多条合并成一条带编号的摘要发给你

实现见 `internal/notify/digest.go`。

如果你不想合并（每条都即时单发），目前没有 UI 开关，但可以把 RSS 调度间隔调短到 1 分钟以下、或把每小时配额拉到很低让 digest 跑不满 5 条阈值。

---

## 失败重试

`internal/app/rss_retry_worker.go` 周期性扫表：

- 扫描 `rss_notification_log` 中 `result='pending'` 且 `next_retry_at <= now` 的行
- 复用 `payload_json`，重新调用 `NotificationService.Push`
- 失败按指数退避重排：**5s → 10s → 20s → 40s → 80s**
- 累计 `attempts >= 5` 仍失败 → 标记 `result='failed'`、`last_error` 留下最后一次错误

`failed` 行不会被自动重试。如需重投：

- 在 "RSS 通知日志" 页点"重试"按钮（手动一次）
- 或在 SQL 里把 `result` 改回 `pending`、`next_retry_at` 改成 `now`

---

## Telegram 内联按钮

filtered 通知（仅 Telegram）会附带两个按钮：

- **立即下载**：触发 push 链路把这条种子发到默认下载器（或 RSS 配置指定的下载器）
- **忽略**：把这条 log 行标记为 `suppressed`，不再重试

按钮点击后：

1. 服务端调用 `RSSCallbackActions.OnRSSDownload` / `OnRSSIgnore`
2. 完成后 `EditMessageReplyMarkup` 清空消息上的按钮（防止重复点击）
3. Telegram 顶部弹出 toast 提示"已加入下载队列 #123" / "已忽略 #123" / 错误信息

> **权限**：必须在 NotificationConf 的 `allowed_users` 或 `admin_users` 列表里才能点。
> 不在白名单的用户点按钮会收到"您没有权限执行此操作"的红色 alert。

> QQ 通道目前不支持内联按钮（OneBot 协议无对应 message segment）。filtered 通知到 QQ 时只有正文。

---

## 每小时配额

`RSSSubscription.max_notifications_per_hour`（默认 **100**）保护你不被一个大批量上新刷屏。

- **判定窗口**：滚动 1 小时（按 `created_at`）
- **统计口径**：sent + failed + pending（throttled / suppressed 不计）
- **超限行为**：直接落 `result='throttled'` 写一条日志、**不**调 push、**不**进 retry

如果想关闭限流：把字段设为 `0`（迁移会留 100 兜底，要主动改）。

---

## DB schema 速查

表：`rss_notification_log`

| 字段                        | 含义                                                       |
| --------------------------- | ---------------------------------------------------------- |
| `id`                        | 主键                                                       |
| `rss_id`                    | RSSSubscription.id                                         |
| `site_name`                 | 站点 ID（如 `hdsky`、`mteam`）                             |
| `torrent_id`                | 站点内种子 ID                                              |
| `notify_kind`               | `all` / `filtered`                                         |
| `notification_conf_id`      | 用哪个通道发                                               |
| `matched_filter_rule_id`    | filtered 模式下命中的规则 ID（可空）                       |
| `result`                    | `sent` / `failed` / `suppressed` / `pending` / `throttled` |
| `attempts`                  | 已尝试投递次数                                             |
| `next_retry_at`             | 下次允许重投的时间                                         |
| `last_error`                | 最近一次失败的错误文本                                     |
| `payload_json`              | 实际推送的 title + text                                    |
| `delivered_at`              | 第一次成功投递时间                                         |
| `created_at` / `updated_at` | 时间戳                                                     |

唯一索引：`(rss_id, site_name, torrent_id, notify_kind, notification_conf_id)`，保证幂等。

部分索引：`(result='pending', next_retry_at)`，加速 retry worker 扫描。

---

## 故障排查 FAQ

### Q1：完全收不到通知怎么排查？

按下面顺序检查：

1. **RSS 订阅** `notify_mode` 是否非空？
2. **RSS 订阅** `notify_conf_ids` 是否包含一个**已启用**的 NotificationConf？
3. 该 NotificationConf 的 **静默时段**是否包含当前时间？
4. **每小时配额** `max_notifications_per_hour` 是否已超？
5. 打开"RSS 通知日志"页，最近是否有 `result='throttled'` 或 `'pending'` 的行？
6. 通道本身是否健康（ChatOps 管理页显示绿色 / 健康）？

### Q2：日志里很多 `throttled`，正常吗？

热门站点上新峰值很容易刷爆 100/h 的默认配额。

- 把 `max_notifications_per_hour` 调高（比如 500）
- 或改成 `filtered` 模式，配合精细的过滤规则把噪声压低

### Q3：重试一直 `failed`，怎么修？

看 `last_error`：

- `bot was kicked / blocked` → 你把 bot 拉黑了 / 把它踢出群了，去 Telegram 重新启动 bot
- `connection refused` / `i/o timeout` → 通道 channel.Send 直接错（NapCat ws 半死、TG 网络抖动）
- `rate limit exceeded` → Telegram 的服务器端限流，等一会儿再重试

修好底层问题后，把日志行 `result` 改回 `pending`、`next_retry_at` 改成 `now`，retry worker 会重新捡起来。

### Q4：filtered 模式没触发？

- 确认 `FilterRule.purpose IN ('notify', 'both')`
- 确认规则 pattern 真的匹配上了种子标题（在 RSS 订阅详情页用 "测试规则" 试一下）
- 确认 RSS 订阅关联了这条规则（"过滤规则"分区）

### Q5：all 模式的同一条种子重复发了好几遍？

不会发生。`(rss_id, site_name, torrent_id, notify_kind, conf_id)` 唯一索引保证写入幂等。
如果你看到重复，多半是 **两个不同的 RSSSubscription 都订阅了同一个 feed**，每个独立计数。

### Q6：怎么清空所有通知日志？

```sql
DELETE FROM rss_notification_log;
```

无外键约束，安全。日志表只是审计 + 重试用，删光不影响下载。

### Q7：QQ 通道也能收 filtered 通知吗？

可以，正文一样。但 QQ 没有内联按钮，所以不会带"立即下载 / 忽略"。

---

## 参考链接

- [ChatOps 快速开始](chatops-quickstart.md)
- [QQ OneBot (NapCat) 配置](chatops-qq-napcat.md)
- [Telegram Bot 配置](chatops-telegram.md)
- [RSS 订阅配置指南](rss-subscription.md)
- [过滤规则与追剧指南](filter-rules-tv-series.md)
