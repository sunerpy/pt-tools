# scripts/qa/ — ChatOps QA 脚本套件

`pt-tools` ChatOps + MCP Bot 设计计划 T34 交付物。配套 Final Wave F3 端到端
QA 验证，依赖 `T17/T18/T22/T24/T25/T32` 实现的测试钩子端点。

## 依赖

仅以下命令行工具：`bash` (>= 4)、`sqlite3`、`curl`、`jq`、`tmux`、`go`。
**禁止**引入新依赖。所有脚本自包含，不访问外部网络。

## 测试钩子端点（仅 `qa` build tag 启用）

```bash
go build -tags qa -o dist/pt-tools-qa .   # 启用 /test/telegram/inject、/test/qq/inject
make build-local                          # 生产构建：无此端点（请求返回 404）
```

测试钩子端点：
- `POST /test/telegram/inject` — body `{"text":"...","from":{"id":12345},"conf_id":1}`，注入入站消息直入 chain
- `POST /test/qq/inject` — 同上，QQ 入站

绕过权限直接调 `internal/chatops.MessageChain.Process(ctx, notify.InboundMessage)`，
仅供 QA 脚本使用，**生产构建不含**。

## 共享配置（环境变量）

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PT_QA_BASE_URL` | `http://127.0.0.1:8080` | pt-tools 服务地址 |
| `PT_QA_DB` | `testdata/qa.db` | sqlite3 测试夹具数据库 |
| `PT_QA_SESSION_COOKIE` | `` | 已登录的 session cookie（用于调 `/api/chatops/*`）|
| `PT_QA_TIMEOUT` | `5` | curl 超时（秒）|

## 脚本清单（13 个）

每个脚本第一行 `#!/usr/bin/env bash` + `set -euo pipefail`，退出码非零即失败。
详见各文件内联注释。

### 1. `seed-data.sh`
**用途**：sqlite3 INSERT 准备测试夹具
**输入**：可选 `count=5000` 环境变量控制 fake torrents 数量
**输出**：`testdata/qa.db` 含 `1 NotificationConf (telegram, mock token)` + N 条 fake torrents
**预期**：[TBD] 退出码 0；`sqlite3 $PT_QA_DB "SELECT COUNT(*) FROM notification_confs"` = 1

### 2. `inject-tg-cmd.sh`
**用途**：注入 Telegram 入站消息
**用法**：`./inject-tg-cmd.sh "/help"` 或 `./inject-tg-cmd.sh "/torrents qb1" 67890`（可选 user_id）
**预期**：[TBD] HTTP 200；stdout 为响应 JSON

### 3. `inject-qq-cmd.sh`
**用途**：注入 QQ 入站消息（OneBot 协议模拟）
**用法**：`./inject-qq-cmd.sh "/status"`
**预期**：[TBD] HTTP 200；stdout 为响应 JSON

### 4. `telegram-poll-help.sh`
**用途**：发送 `/help` 后校验回复含 11 命令清单
**预期**：[TBD] 回复正文匹配 `/(help|status|torrents|search|push|pause|resume|delete|bind|unbind|mute)/g` ≥ 11 次

### 5. `qq-onebot-status.sh`
**用途**：QQ `/status` 回复校验
**预期**：[TBD] 回复含 `running` / `version` 字样

### 6. `webhook-event.sh`
**用途**：启动 httptest receiver，触发 `EvtTorrentAdded`，校验 webhook POST 200
**预期**：[TBD] receiver 收到 1 条 POST，body JSON 含 `event_type=torrent_added`

### 7. `bind-flow.sh`
**用途**：完整绑定流程
**步骤**：
  1. `POST /api/chatops/bindings/issue-code` 拿 code
  2. `inject-tg-cmd.sh "/bind <code>"`
  3. `sqlite3` SELECT `channel_binding` 行 +1，`allowed=1` `pt_admin=1`
**预期**：[TBD] DB 行数增 1 且字段正确

### 8. `outbox-offline.sh`
**用途**：离线重试场景
**步骤**：
  1. 设 channel 凭证为无效 → 触发事件 → outbox 行 `status=pending`
  2. 恢复凭证 → 等 outbox worker 1 cycle → `status=sent`
**预期**：[TBD] 状态从 pending → sent

### 9. `torrents-perf.sh`
**用途**：性能基准
**用法**：`./torrents-perf.sh 5000`
**步骤**：
  1. `seed-data.sh` count=5000
  2. time `inject-tg-cmd.sh "/torrents qb1"`
**预期**：[TBD] elapsed_ms < 2000；输出含 `PASS < 2000ms`

### 10. `run-all-task-scenarios.sh`
**用途**：编排，顺序执行 task-{1..33}-*.sh
**输出**：`final-f3-tasks.txt` 汇总每个 task 的 PASS/FAIL
**预期**：[TBD] 全部 task 退出码 0

### 11. `integration-bind-flow.sh`
**用途**：端到端 Web UI + bot 流程（Web 调 `/api/chatops/bindings/issue-code` + bot `/bind <code>`）
**预期**：[TBD] 退出码 0；DB 行 +1

### 12. `integration-torrents-perf.sh`
**用途**：完整 chain 链路性能（不绕过 service / downloader）
**预期**：[TBD] elapsed_ms < 2000

### 13. `edge-cases.sh`
**用途**：边界用例集合
**场景**（每个独立函数）：
  1. rate-limit 触发（>10 次/分钟）→ 第 11 次 reply 含 `rate limit`
  2. 不存在的 downloader → reply 含 `not found`
  3. 表单空提交 → reply 含 `invalid argument`
  4. 长种子名（>200 字符）→ reply 截断且不崩溃
  5. 重连后会话清空 → 二次 `/torrents` 不复用旧分页
**预期**：[TBD] 5 场景全部退出码 0

## 运行示例

```bash
# 启动 QA 构建
go build -tags qa -o dist/pt-tools-qa .
./dist/pt-tools-qa web --host 127.0.0.1 --port 8080 &

# 准备夹具
./scripts/qa/seed-data.sh

# 单脚本
./scripts/qa/telegram-poll-help.sh

# 全量
./scripts/qa/run-all-task-scenarios.sh
```

## 验证

```bash
# 语法检查
for f in scripts/qa/*.sh; do bash -n "$f" || exit 1; done

# 端到端
./scripts/qa/seed-data.sh
./scripts/qa/telegram-poll-help.sh
./scripts/qa/bind-flow.sh
./scripts/qa/torrents-perf.sh 5000

# 生产构建无测试钩子
make build-local
./dist/pt-tools web --host 127.0.0.1 --port 8081 &
curl -s -o /dev/null -w "%{http_code}\n" -X POST http://127.0.0.1:8081/test/telegram/inject -d '{}'
# 期望：404
```
