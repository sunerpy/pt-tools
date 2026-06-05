# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.34.0] - 2026-05-28

### Bug Fixes

- **downloader**: RSS 推送遵循站点-下载器绑定，不再静默回退到 is_default
- **scheduler**: 磁盘预留独立于 CleanupEnabled 周期归零，避免单调累加

### Dependencies (Frontend)

- **pnpm**: Bump sass from 1.99.0 to 1.100.0 in /web/frontend ([#364](https://github.com/sunerpy/pt-tools/issues/364)) ([#364](https://github.com/sunerpy/pt-tools/pull/364))
  Bumps [sass](https://github.com/sass/dart-sass) from 1.99.0 to 1.100.0. - [Release notes](https://github.com/sass/dart-sass/releases) - [Changelog](https://github.com/sass/dart-sass/blob/main/CHANGELOG.md) - [Commits](https://github.com/sass/dart-sass/compare/1.99.0...1.100.0)

        ---
        updated-dependencies:
        - dependency-name: sass
         dependency-version: 1.100.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump dompurify from 3.4.2 to 3.4.5 in /web/frontend ([#368](https://github.com/sunerpy/pt-tools/issues/368)) ([#368](https://github.com/sunerpy/pt-tools/pull/368))
  Bumps [dompurify](https://github.com/cure53/DOMPurify) from 3.4.2 to 3.4.5. - [Release notes](https://github.com/cure53/DOMPurify/releases) - [Commits](https://github.com/cure53/DOMPurify/compare/3.4.2...3.4.5)

        ---
        updated-dependencies:
        - dependency-name: dompurify
         dependency-version: 3.4.5
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump vue-tsc from 3.3.0 to 3.3.2 in /web/frontend ([#369](https://github.com/sunerpy/pt-tools/issues/369)) ([#369](https://github.com/sunerpy/pt-tools/pull/369))
  Bumps [vue-tsc](https://github.com/vuejs/language-tools/tree/HEAD/packages/tsc) from 3.3.0 to 3.3.2. - [Release notes](https://github.com/vuejs/language-tools/releases) - [Changelog](https://github.com/vuejs/language-tools/blob/master/CHANGELOG.md) - [Commits](https://github.com/vuejs/language-tools/commits/v3.3.2/packages/tsc)

        ---
        updated-dependencies:
        - dependency-name: vue-tsc
         dependency-version: 3.3.2
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump oxlint from 1.65.0 to 1.66.0 in /web/frontend ([#367](https://github.com/sunerpy/pt-tools/issues/367)) ([#367](https://github.com/sunerpy/pt-tools/pull/367))
  Bumps [oxlint](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxlint) from 1.65.0 to 1.66.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxlint/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxlint_v1.66.0/npm/oxlint)

        ---
        updated-dependencies:
        - dependency-name: oxlint
         dependency-version: 1.66.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump vite from 8.0.13 to 8.0.14 in /web/frontend ([#370](https://github.com/sunerpy/pt-tools/issues/370)) ([#370](https://github.com/sunerpy/pt-tools/pull/370))
  Bumps [vite](https://github.com/vitejs/vite/tree/HEAD/packages/vite) from 8.0.13 to 8.0.14. - [Release notes](https://github.com/vitejs/vite/releases) - [Changelog](https://github.com/vitejs/vite/blob/main/packages/vite/CHANGELOG.md) - [Commits](https://github.com/vitejs/vite/commits/v8.0.14/packages/vite)

        ---
        updated-dependencies:
        - dependency-name: vite
         dependency-version: 8.0.14
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump vitest from 4.1.6 to 4.1.7 in /web/frontend ([#372](https://github.com/sunerpy/pt-tools/issues/372)) ([#372](https://github.com/sunerpy/pt-tools/pull/372))
  Bumps [vitest](https://github.com/vitest-dev/vitest/tree/HEAD/packages/vitest) from 4.1.6 to 4.1.7. - [Release notes](https://github.com/vitest-dev/vitest/releases) - [Changelog](https://github.com/vitest-dev/vitest/blob/main/docs/releases.md) - [Commits](https://github.com/vitest-dev/vitest/commits/v4.1.7/packages/vitest)

        ---
        updated-dependencies:
        - dependency-name: vitest
         dependency-version: 4.1.7
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump @types/node from 25.9.0 to 25.9.1 in /web/frontend ([#371](https://github.com/sunerpy/pt-tools/issues/371)) ([#371](https://github.com/sunerpy/pt-tools/pull/371))
  Bumps [@types/node](https://github.com/DefinitelyTyped/DefinitelyTyped/tree/HEAD/types/node) from 25.9.0 to 25.9.1. - [Release notes](https://github.com/DefinitelyTyped/DefinitelyTyped/releases) - [Commits](https://github.com/DefinitelyTyped/DefinitelyTyped/commits/HEAD/types/node)

        ---
        updated-dependencies:
        - dependency-name: "@types/node"
         dependency-version: 25.9.1
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

### Features

- **sites**: 新增 2 个 NexusPHP 站点适配（ourbits / mua）
  新增站点（均为 NexusPHP + Cookie 鉴权）：

        - OurBits (ourbits.club) — Issue #329
         - 标准 10 列 NexusPHP 表格
         - 详情页 h1 嵌入 free 折扣 + onmouseover 含剩余时间
         - 用户信息：标准 userdetails.php?id 路径
        - Mua (mua.xloli.cc, 二次元站点) — Issue #339
         - 9 列变体（无'进度'列）
         - userdetails URL 使用 uuid= 而非 id=，因此 id 字段从
         'td.rowhead:contains(用户ID/UID) + td' 提取整数 UID
         - 副标题在 td.embedded > span（无 optiontag/tag class）

        每站包含完整定义 + fixture 测试（搜索/详情/用户信息 + 无密钥校验）。
        此外两站点都被加入 real_html_validation_test.go 的 TestRealHTML_*
        测试列表，本地放置 issue 附件 ZIP 解压到 /tmp/site-zips/<id>/ 后
        即可对真实站点 HTML 跑回归。本次实现已用 issue 附件验证：

        - OurBits: 真实搜索 100 行解析 OK；详情 SizeMB=88709.1 / Discount=FREE；
         userinfo 全字段（id/name/bonus/seeding/leeching/uploaded/downloaded/ratio/
         levelName/joinTime/trueUploaded/trueDownloaded）解析非空
        - Mua: 真实搜索 100 行解析 OK；详情 SizeMB=1402.9 / Discount=FREE；
         userinfo 全字段（含 uuid 路径下的 UID 提取）解析非空

        浏览器扩展 KNOWN_SITES 同步更新（pt-sites.ts 中两域名已存在）。
        docs/sites.md 已适配站点数从 41 → 43，NexusPHP 系列从 37 → 39。

### Styling

- **changelog**: 应用 oxfmt 自动格式化
  CI Format Check 触发 oxfmt 重排已有 CHANGELOG 行；本次修复仅是格式调整，无内容变更。

## [0.33.0] - 2026-05-24

### Bug Fixes

- **ci**: Edge 商店 InReview 阻塞时软失败而非硬失败
  实验发现：Edge Add-ons Public API 不允许在前一次 submission 处于
  InReview 状态时上传新 draft package。0.2.2 实验中验证：

        - 0.2.0 / 0.2.1 失败：都是"InProgress → Failed"，~10s（验证管线后期失败）
        - 0.2.2 失败：直接"Failed"，<1s（同步前置条件检查失败）

        时间差 + null errorCode/errors 是"product 已被锁定，无法接受新 submission"
        的特征签名。Public API 没有公开端点查询 submission 列表（/overview/submissions
        是内部 Cookie 鉴权 endpoint），因此无法预检。

        修复：Wait for upload processing 步骤改为：
        - 失败 < 5s 且 errorCode/errors 都为 null
         → 视为前置条件失败（很可能是 InReview 阻塞）
         → 输出 ::warning:: + 操作指引（手动 workflow_dispatch 重试）
         → exit 0，软失败
        - 其它失败（慢速 / 有 errorCode / 有 errors）
         → 真实包错误，硬失败 exit 1

        Publish submission + Wait for publish processing 两步增加
        `if: env.UPLOAD_SOFT_FAIL != 'true'` 跳过保护。

        操作指引：前一次 submission 进入 InExtensionStore (Live) 后，
        `gh workflow run extension-publish.yml --ref main` 手动重试发布。

        Release artifact (zip + GitHub Release) 始终生成，软失败不影响。

- **qbit**: 检测到 HTML 响应时拒绝认证，避免错误 URL 静默通过

### CI/CD

- **extension-publish**: 放宽 InReview 软失败判定为基于响应 message
  Edge API 在已有 InReview 的提交时，会以 'An error occurred while performing the operation'+空 errorCode/errors 形式失败。原本 <5s 的时间窗口判定不够稳健，实测有 31s 才返回 Failed 的情况，会被误判为硬失败。改成基于响应 message 字面识别，时间窗口去掉。

### Features

- **extension**: API 错误分类与超时控制
- 新增 PtToolsApiError 类型化错误码：network_unreachable / timeout
  / auth_required / client_error / server_error / invalid_response
  / ping_failed / unknown - 错误码以 "[code] message" 前缀编码，可穿越 chrome.runtime 消息总线
  保留分类信息（messages.ts 的 Error→string 序列化只保留 .message）- friendlyErrorMessage() 把错误码映射到 i18n key，server/client 错误
  附带 HTTP 状态详情 - fetchWithTimeout: AbortController + 默认 10s 超时
  TypeError → network_unreachable
  AbortError → timeout - classifyHttpResponse: 401/0/opaqueredirect → auth_required
  4xx → client_error，5xx → server_error - api-client.ts: - 所有 fetch 改用 fetchWithTimeout - 4xx/5xx 通过 classifyHttpResponse 抛 PtToolsApiError - 移除硬编码中文 AUTH_REQUIRED_MESSAGE - ping() 不再静默吞掉错误（旧版 catch{} 返回 false 让用户看不到原因），
  现在抛 PtToolsApiError，由调用方决定如何处理
- **extension**: SYNC_COOKIES 区分 test-vs-save，加 chrome.notifications 自动同步反馈
- onMessage("SYNC_COOKIES") 新行为：- 提供用户名密码 = 显式 "测试连接"，ping 失败抛错，弹窗看到具体原因 - 仅 baseUrl = 静默 "保存设置"，ping 失败仍保存 URL 但 connected=false，
  弹窗显示 warning（设置已保存但未连通）- 修复旧 bug：ping 失败时弹窗显示绿色「连接成功」（旧 ping 永远 boolean）- maybeAutoSyncByDomain() 在弹窗关着时通过 chrome.notifications 推送：- 成功：「{site} Cookie 自动同步成功」- 失败：「{site} 同步失败：{friendly reason}」priority 2 - 失败原因经 friendlyErrorMessage() 翻译为可读中文 - 之前 logger.warn 是后台静默日志，用户完全看不见
- **extension**: 用 toast 替代单行 feedback；按操作分类显示成功/失败/警告
- 新增 useToast composable + ToastStack 组件 - 4 种 severity：success/error/warning/info，各自配色 + 图标 - 自动消失：success/info 3.5s，warning 6s，error 8s - 点击立即关闭，多条堆叠 - 进出场动画 - App.vue 重写所有 7 个 handler：- 单点同步成功 → success（含站点名）- 同步失败 → error，错误码经 friendlyErrorMessage() 翻译 - 自动同步开关 → info - 测试连接 → 服务器在线 success；离线 warning（提示设置已保存）- 保存设置（无凭据）→ ping 通 success，ping 不通 warning（不再误显绿色）- 批量同步分四档：全部成功 success / 部分成功 warning / 全部失败 error / 无可同步 warning - 失败站点列表显示站点名（之前只显示 siteId）- 删除孤儿组件 CookieSync.vue（零引用）
- **extension**: 新增 i18n 错误分类 key 与 notifications 权限
- manifest 增加 "notifications" 权限以支持 chrome.notifications.create()
  自动同步运行时弹窗关闭，依赖 OS 通知反馈 - 新增 i18n key（zh-CN + en-US 双语）：- feedback.syncSuccess（带站点名占位符）- feedback.autoSyncEnabled / autoSyncDisabled - feedback.settingsSavedNotConnected - feedback.batchAllSynced / batchPartial / batchAllFailed - error.networkUnreachable / timeout / authRequired - error.clientError（占位符填 HTTP 状态详情）- error.serverError / pingFailed / invalidResponseFromPtTools - notification.autoSyncSuccess / autoSyncSuccessBody - notification.autoSyncFailure / autoSyncFailureBody - 修复历史漏译（中文 locale 中夹杂英文）：- error.noActiveTab "No active tab" → "未找到活动标签页" - error.invalidResponse "Invalid response" → "响应数据无效" - error.ptToolsLoginFailed "pt-tools login failed" → "pt-tools 登录失败，请检查用户名密码" - 版本 0.2.2 → 0.2.3

## [ext-v0.2.2] - 2026-05-23

### Documentation

- **extension**: 新增 PRIVACY.md 用于 Edge 商店隐私策略 URL
  Microsoft Edge 商店 0.2.1 提交被拒，API 返回仅
  "An error occurred while performing the operation"。通过
  Partner Center 页面诊断定位到隐私表未填。

        隐私策略 URL 为隐私表必填项。本文档详细阐述：
        - 数据读取/传输/不传输范围（仅同步到用户自部署 pt-tools）
        - 自动脱敏规则（Cookie 值 / 邮箱 / IP / API Key 等）
        - 所有权限（storage / activeTab / scripting / cookies / tabs）逆向用途
        - 未使用远程代码声明
        - 开源审计指引与联系方式

        提交后 URL 会预在 Edge 商店扩展页面公开展示。

### Miscellaneous

- **extension**: 发布 0.2.2 验证 API 发布路径
  0.2.1 经 Partner Center Web UI 提交后已 InReview，stuck draft 槽位
  被消费。本次纯版本号 bump（无任何代码改动）作为判别实验：

        - 通过 ext-v0.2.2 tag 触发 extension-publish workflow
        - workflow 仅走 Public Add-ons API（非 Web UI）
        - 若 publish-edge job 全绿、商店 Submission 进入 InReview：
         hypothesis (a) stuck-draft 已消除，未来 API path 可持续使用
        - 若仍报 status=Failed errorCode=null：
         hypothesis (c) 后端流水线差异占主导，需进一步排查 zip 内容
         （locale/manifest 归一化）或申请 Microsoft 工单查内部错误

        不含任何业务变更，与 0.2.1 zip 字节差异仅 manifest version 字段。

## [ext-v0.2.1] - 2026-05-23

### Bug Fixes

- **ci**: Edge 商店发布从 Location 头提取 OperationID
- 微软 Add-ons API 在 202 响应的 Location 头返回 OperationID（不在 body）- 之前用 grep -oP body 抓 "id" 字段始终为空，env 变量 $operation_id 是空字符串 - 状态轮询 URL 拼成 /operations/ 拿不到合法响应，20 次循环全部 Unknown - 但 if/elif 没有 else 分支，循环跑完后步骤继续执行 - 最终 publish 调用看似 202 成功，实际微软那边没有可发布的 draft，submission stuck - 表现：CI 全绿但商店仍是旧版本（0.1.0）

      修复：
      - 改用 curl -D 把 headers 写到独立文件，从 Location 行提取 OperationID
      - 上传/发布两步均用大写 $UPLOAD_OPERATION_ID / $PUBLISH_OPERATION_ID 通过 GITHUB_ENV 传递
      - 30 次轮询结束仍未 Succeeded 必须 fail（不再静默继续）
      - 新增 publish 后的状态轮询，确认 submission 真正进入审核
      - 全步骤 set -euo pipefail，header 与 body 完整打印便于排障

      参考：
      https://learn.microsoft.com/en-us/microsoft-edge/extensions-chromium/publish/api/using-addons-api

- **extension**: 修复 SpringSunday Cookie 检测使用 SPRINGID
- SSD（springsunday.net）已迁移到 PHP session 鉴权 - session_name=SPRINGID 是站点唯一鉴权 cookie - 不再下发传统 NexusPHP 的 c_secure_uid / c_secure_pass / c_secure_tracker_ssl 三件套 - 插件之前硬编码要求三件套，导致已登录用户被判定为 missing - 表现：扩展弹窗显示"❌ Cookie 缺失"，无法同步到 pt-tools

      修复：
      - KNOWN_SITES 中 SpringSunday 的 cookieNames 从三件套改为 [SPRINGID]
      - 版本 0.2.0 → 0.2.1（package.json + src/manifest.ts）

### Dependencies (Go)

- **go**: Bump golang.org/x/net 0.53.0 -> 0.55.0 修复 5 个 HTML 解析 CVE
  govulncheck 报告 site/v2/nexusphp_driver.go 通过 goquery.NewDocumentFromReader
  触发 golang.org/x/net/html 的 5 个漏洞：

        - GO-2026-5028 解析任意 HTML 时的 DoS
        - GO-2026-5027 foreign content 中元素处理错误
        - GO-2026-5025 foreign content 中命名空间元素处理错误
        - 另 2 个 x/net/html 同源 advisory

        修复仅升级到 v0.55.0；go mod tidy 顺手把已被 web 子包直接导入的
        RomiChan/websocket 与 tidwall/gjson 从 indirect 提到 direct（不影响构建）。

        go.mod 自带 vendor 目录非 git 跟踪，本次只更新 go.mod / go.sum。

### Styling

- **changelog**: Oxfmt 自动格式化历史 v0.24.0 条目
  CI 的 Format Check 因 oxfmt 输出与仓库版本漂移而失败。漂移点是 v0.24.0
  （OpenCD/PTT 适配）一个嵌套列表的 markdown 转义：oxfmt 把多层 "\_" / "\*"
  归一化为反斜杠转义。仅文档格式调整，不改任何内容。

## [0.32.1] - 2026-05-22

### Bug Fixes

- **scheduler**: 修复部分种子免费期结束后未自动暂停
- updateAllMonitoredProgress / updateAllPushedTasksProgress 不再仅凭
  progress=1.0 标记 is_completed；新增 isTorrentTrulyCompleted 同时校验
  下载器状态（仅 Seeding/Queued 视为完成），避免 pausedDL/missingFiles/
  checkingResumeData 等噪音状态被误判，导致种子被永久排除出免费期监控。- 移除完成路径上的 CancelTorrent，让定时器继续生效，便于后续真实状态
  确认。- periodicCheck 在处理已过期种子之外，新增 rescheduleMissingFutureTorrents
  补预约：扫描 free_end_time>now 但不在 pendingTasks 中的种子并重新调度，
  覆盖初始 Schedule 漏调用 / 进程重启丢失定时器的场景。- ScheduleTorrent 在跳过调度时输出 Debug 日志（缺 FreeEndTime 或
  DownloaderTaskID 各自独立提示），成功调度时升级到 Info 级别，便于
  运维 grep "跳过调度" 诊断。- 测试：新增 4 组真实路径回归——pausedDL/stoppedDL/error/missingFiles/
  checkingResumeData 在 progress=1.0 不应被标记完成；
  uploading/stalledUP/forcedUP 应被标记完成；
  periodicCheck 必须补预约错过初始 Schedule 的未来过期种子；
  ScheduleTorrent 在缺前置条件时不进入 pendingTasks。- 更新 mock qBit 服务器：progress=1.0 时返回 uploading/stalledUP，
  贴合真实 qBit 语义。

### Build

- **release**: 公告流程支持自定义 release-notes 文件并隐藏依赖噪音
- telegram-release-announce.yml 在读取 release.body 之前先尝试加载
  .github/release-notes/<tag>.md（带或不带 v 前缀均可）；存在则覆盖
  release.body 作为 TG 公告输入。修复 v0.32.0 公告被 release-please
  squash 进来的 6 个 dependabot PR body 污染、commit body 被截断的问题。- release-please-config.json 增加 changelog-sections，将 chore / docs /
  style / refactor / test / build / ci / deps / deps(go) / deps(pnpm)
  全部 hidden=true，仅在公告与 release body 中保留 feat / fix / perf /
  revert，避免 dependabot 升级污染 release notes。CHANGELOG.md 仍由
  git-cliff 单独完整生成。- 新增 .github/release-notes/README.md 说明覆盖文件命名与发版流程，
  以及预置 v0.32.1.md 作为本次发版的精简公告（≤ 400 字）。

### Documentation

- **release-notes**: 合并 v0.32.0 公告至 v0.32.1，统一对外发布
  v0.32.0 的 TG 公告由于 release-please 自动 body 被 dependabot 6 个 deps PR 污染、commit body 被截断，未能符合精简要求。本次发版的 TG 公告改为合并版，覆盖两次 release 的所有用户可见改动：

        - 磁盘保护并发 race + 日志拆分（Issue #299）
        - 5 站点 UserInfo 字段同步（Issue #332）
        - GTKPW Cloudflare RSS UA
        - 哈希计算失败提示
        - 自动暂停误标完成 (v0.32.1 新修)

        字数 ≈ 365 中文字符，符合 ≤400 限制；通过 .github/release-notes/v0.32.1.md 走新引入的 TG 公告覆盖路径。

## [0.32.0] - 2026-05-22

### Bug Fixes

- **core**: 修复磁盘保护 race、站点字段同步、RSS Cloudflare 与下载器 URL 等用户反馈问题
- 磁盘保护 (Issue #299 race 修复) - 推送成功后保留预留，由 cleanup_monitor 周期 Reset 归还，避免 qBit 可见性窗口内并发 worker 重复借用空间 - cleanup_monitor.runOnce 的 Reset 包入 PushMutex，避免与 worker 临界区交错 - DB 缺失 TorrentSize 时回退到 ComputeTorrentSize 解析种子文件 - 日志拆分输出 qBit可用 / 下载中待占用 / 本进程预留 / 有效空间，解释空间不足原因 - 手动添加 API 加入磁盘保护，magnet/URL 在保护启用时拒绝放行 - 默认全局配置启用 CleanupDiskProtect 与 50GB 保底 - Transmission GetDiskSpace 改为 fail-closed - 新增 TestDiskProtect_RealRSSPath_SuccessKeepsReservation 端到端 race 回归

      - 站点字段同步 (Issue #332)
       - 基于 issue 用户上传的真实 DOM 重写 nicept/gtkpw/gamegamept 的 trueUploaded/trueDownloaded 选择器，从 传输/傳送 row 内 实际上传量/實際上傳量 strong 标签提取
       - 撤回 seedingSize 显式选择器，恢复 driver 自动 FetchSeedingStatus ajax 兜底
       - bonusPerHour 改用 #outer / #info_block 容器 + 紧凑正则，避免误匹配
       - pttime 时魔从 userdetails.php 内联 (xx魔力/小时) 解析，无需 mybonus.php
       - mteam 增加 trueUploaded/trueDownloaded JSON 字段映射
       - 各站 fixture 测试更新为真实 DOM 形态

      - 种子下载与同步稳定性
       - 下载响应先校验为合法 torrent 再落盘，错误信息附带 status/size/preview
       - mTorrent 详情接口增加 1.2s 本站频控与业务错误码识别
       - UserInfoService 拒绝空用户名，保存使用独立 10s 超时

      - RSS 抓取
       - fetchRSSFeed 改用浏览器 UA + 30s 超时，修复 Cloudflare-fronted 站点 (gtkpw) TLS reset

      - 下载器配置
       - qbit/transmission URL 自动补全 http:// 前缀，拒绝非 http(s)/带 userinfo/带 fragment 的输入

      - 推送日志降噪
       - 推送目标下载器未启用时改为 Warn，提示用户切换或启用，避免 Error 刷屏

### Dependencies (Frontend)

- **pnpm**: Bump oxlint from 1.64.0 to 1.65.0 in /web/frontend ([#343](https://github.com/sunerpy/pt-tools/issues/343)) ([#343](https://github.com/sunerpy/pt-tools/pull/343))
  Bumps [oxlint](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxlint) from 1.64.0 to 1.65.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxlint/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxlint_v1.65.0/npm/oxlint)

        ---
        updated-dependencies:
        - dependency-name: oxlint
         dependency-version: 1.65.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump vue-tsc from 3.2.8 to 3.3.0 in /web/frontend ([#346](https://github.com/sunerpy/pt-tools/issues/346)) ([#346](https://github.com/sunerpy/pt-tools/pull/346))
  Bumps [vue-tsc](https://github.com/vuejs/language-tools/tree/HEAD/packages/tsc) from 3.2.8 to 3.3.0. - [Release notes](https://github.com/vuejs/language-tools/releases) - [Changelog](https://github.com/vuejs/language-tools/blob/master/CHANGELOG.md) - [Commits](https://github.com/vuejs/language-tools/commits/v3.3.0/packages/tsc)

        ---
        updated-dependencies:
        - dependency-name: vue-tsc
         dependency-version: 3.3.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump vite from 8.0.12 to 8.0.13 in /web/frontend ([#347](https://github.com/sunerpy/pt-tools/issues/347)) ([#347](https://github.com/sunerpy/pt-tools/pull/347))
  Bumps [vite](https://github.com/vitejs/vite/tree/HEAD/packages/vite) from 8.0.12 to 8.0.13. - [Release notes](https://github.com/vitejs/vite/releases) - [Changelog](https://github.com/vitejs/vite/blob/main/packages/vite/CHANGELOG.md) - [Commits](https://github.com/vitejs/vite/commits/v8.0.13/packages/vite)

        ---
        updated-dependencies:
        - dependency-name: vite
         dependency-version: 8.0.13
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump vue-router from 5.0.6 to 5.0.7 in /web/frontend ([#344](https://github.com/sunerpy/pt-tools/issues/344)) ([#344](https://github.com/sunerpy/pt-tools/pull/344))
  Bumps [vue-router](https://github.com/vuejs/router) from 5.0.6 to 5.0.7. - [Release notes](https://github.com/vuejs/router/releases) - [Commits](https://github.com/vuejs/router/compare/v5.0.6...v5.0.7)

        ---
        updated-dependencies:
        - dependency-name: vue-router
         dependency-version: 5.0.7
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump @vitejs/plugin-vue from 6.0.6 to 6.0.7 in /web/frontend ([#348](https://github.com/sunerpy/pt-tools/issues/348)) ([#348](https://github.com/sunerpy/pt-tools/pull/348))
  Bumps [@vitejs/plugin-vue](https://github.com/vitejs/vite-plugin-vue/tree/HEAD/packages/plugin-vue) from 6.0.6 to 6.0.7. - [Release notes](https://github.com/vitejs/vite-plugin-vue/releases) - [Changelog](https://github.com/vitejs/vite-plugin-vue/blob/main/packages/plugin-vue/CHANGELOG.md) - [Commits](https://github.com/vitejs/vite-plugin-vue/commits/plugin-vue@6.0.7/packages/plugin-vue)

        ---
        updated-dependencies:
        - dependency-name: "@vitejs/plugin-vue"
         dependency-version: 6.0.7
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump oxfmt from 0.49.0 to 0.50.0 in /web/frontend ([#349](https://github.com/sunerpy/pt-tools/issues/349)) ([#349](https://github.com/sunerpy/pt-tools/pull/349))
  Bumps [oxfmt](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxfmt) from 0.49.0 to 0.50.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxfmt/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxfmt_v0.50.0/npm/oxfmt)

        ---
        updated-dependencies:
        - dependency-name: oxfmt
         dependency-version: 0.50.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump @types/node from 25.6.0 to 25.9.0 in /web/frontend ([#350](https://github.com/sunerpy/pt-tools/issues/350)) ([#350](https://github.com/sunerpy/pt-tools/pull/350))
  Bumps [@types/node](https://github.com/DefinitelyTyped/DefinitelyTyped/tree/HEAD/types/node) from 25.6.0 to 25.9.0. - [Release notes](https://github.com/DefinitelyTyped/DefinitelyTyped/releases) - [Commits](https://github.com/DefinitelyTyped/DefinitelyTyped/commits/HEAD/types/node)

        ---
        updated-dependencies:
        - dependency-name: "@types/node"
         dependency-version: 25.9.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

### Features

- **ui**: 优化侧栏菜单激活逻辑，支持详情页与子路由映射
- 新增 routeNameToMenuIndex 映射表，解决路由 name 与菜单 index 不一致问题 - 改进 activeMenu 计算逻辑，避免首屏闪烁并正确匹配子菜单项

## [0.31.3] - 2026-05-17

### Bug Fixes

- **chatops**: 5 个用户反馈 bug + 表单/权限/UX 改进 (v0.31.3) ([#341](https://github.com/sunerpy/pt-tools/issues/341)) ([#341](https://github.com/sunerpy/pt-tools/pull/341))

* fix(chatops/telegram): allowed_users 用户可以使用非管理员命令

      之前 Telegram adapter 在 permitted() 里硬性要求"任何斜杠命令必须 admin"，
      导致 allowed_users 的普通用户连 /help / /version 这种纯查询都被回复
      "只有管理员才有权限执行此命令。"，与设计意图不符，且与 QQ adapter 行为不
      一致 (QQ 同时接受 admin + allowed 进入命令链)。

      修复：

      - adapter 层只检查白名单（admin OR allowed），任意名单内用户都能进入命令链
      - 命令级别的 AdminOnly 检查由 MessageChain.Process 在 binding.PtAdmin 上
       细粒度执行，回复 "需管理员权限"（已存在的逻辑）
      - AdminOnly 标记的 4 个命令仍然限管理员: /unbind /delete /pause /resume
      - 其余 7 个命令（/help /status /version /tasks /sites /torrents /bind）
       对所有白名单用户开放
      - 删除 adminOnlyMessage 常量，permitted/denyReason/replyDenied 移除
       isCommand 参数

      文档同步：chatops-telegram.md 字段语义说明表更新 allowed_users 权限范围。

      测试：新增 TestPermitted_AdmitsBothListsForAllMessages、
      TestPermitted_EmptyLists、TestDenyReason_AlwaysNotInWhitelist；
      将 TestTelegramAdapter_PermissionGate_NonAdminCommand 改写为
      TestTelegramAdapter_PermissionGate_AllowedUserCanSendCommands 验证新语义。

      * fix(web): API 路径 auth 失败时返 401 JSON 而非 302 重定向，扩展同步 Cookie 不再误报 405

      之前 auth middleware 对未登录请求一律 302 跳 /login，浏览器扩展 fetch
      默认 follow 重定向 → GET /login → /login 仅接受 POST → 405 Method Not
      Allowed。用户看到 "HTTP 405" 完全不知道是 session 问题。

      - web/server.go: auth middleware 检测 r.URL.Path 前缀 /api/，API 请求返
       401 + JSON {error:"unauthorized", message:"..."}；SPA 路径保持 302
       让浏览器用户透明跳转到 /login
      - tools/browser-extension/src/modules/sync/api-client.ts: fetch 加
       redirect:"manual"，对 401/opaqueredirect 显式抛 "未登录或会话已过期"
       错误，让用户知道要先在扩展里登录 pt-tools；getSites/ping 同步加固
      - web/server_test.go: 新增 TestAuth_APIPathReturns401_NotRedirect 与
       TestAuth_SPAPathReturns302_StillRedirects 双向覆盖

      * fix(chatops/chain): 非命令消息回复 /help 提示，不再 silent drop

      之前用户发非 / 开头的普通文本（"你好" 等）被 silent ignore，没回复，
      用户以为 bot 挂了。改为：
      - 回复："🤖 我只识别命令消息（以 / 开头）。\n\n发送 /help 查看支持的命令清单。"
      - audit result 由 user_message_ignored 改为 user_message_hinted 区分

      unknown-command 分支（/ 开头但命令不存在）的"命令未知，发送 /help 查看"
      逻辑保持不变。测试更新：
      - TestProcess_FreeText_Bound_Ignored 改名为 TestProcess_FreeText_Bound_RepliesWithHelpHint
      - 现在验证回复包含 /help 和命令消息相关内容

      * fix(chatops/web): Telegram 表单 admin_users/allowed_users/default_chat_id 双向绑定

      - textarea 直接 v-model 到 conf.admin_users（[]int64），数组在 input 显示
       为空或乱码；保存时把 textarea 字符串当数组发，后端解析失败导致编辑保存
       不生效
      - 引入 tgForm 独立 reactive 持有 *_text 字段：加载时数组 → 逗号分隔文本，
       保存时文本 → 数字数组（parseUserIdList 容错空白和非数字）
      - default_chat_id 同理：number 显示为字符串，保存时含字母（@username）
       保留字符串供 Telegram 公开频道使用，纯数字回转 number
      - unpackNotificationResponse 此前仅 QQ 数组字段透传，Telegram []int64 与
       数字 default_chat_id 被静默丢弃；扩充 arrayFields/numericFields 使
       Telegram 配置完整流到表单
      - QQ 既有 helper 重命名为通用 userIdListToText/parseUserIdList，TG/QQ 共用
      - 提示文本明确格式（含示例）与权限范围

      * fix(chatops/web): Telegram 表单 v-model 绑定到 tgForm 而不是 conf

      01bb4f0 子 agent 把 *_text 字段写在 tgForm reactive 里，但 template
      v-model 写成 (conf as any).admin_users_text — conf 上没这字段，强转
      读到 undefined，textarea 永远空白。

      3 处 v-model 改为 tgForm.{admin_users,allowed_users,default_chat_id}_text，
      与 loadDetail / saveCredentials 中的 tgForm.* 引用对齐。

      * chore: oxfmt 同步 CHANGELOG.md / chatops-telegram.md / api-client.ts

## [0.31.2] - 2026-05-16

### Bug Fixes

- **chatops**: 修复 4 个通知通道 bug + 配置热重载 (v0.31.2) ([#340](https://github.com/sunerpy/pt-tools/issues/340)) ([#340](https://github.com/sunerpy/pt-tools/pull/340))

* fix(chatops/notify): 测试消息不再走 outbox fallback，失败直接报错给前端

      之前 TestConf → Push → manager.Send 超时/失败时 fallback 到 Enqueue（写
      notification_outbox 等异步重试），返回 nil 给 handler，前端 toast 显示
      "测试消息发送成功"——但消息其实没送达，用户被误导。

      测试消息的语义是"立即验证通道工作"，与业务通知（应当 fallback retry）
      不同。新增 PushSync：仅同步 manager.Send + 5s 超时，错误原样返回，**不写
      outbox**。TestConf 改用 PushSync。

      前端 NotificationDetail.vue 收到错误后展示明确失败 toast 含原因；后端
      testNotification handler 在错误响应里给出操作提示（检查网络 / proxy_url /
      NapCat WS）。

      业务通知（free-end / 上新 RSS / chatops 命令回复等）继续走原 Push（含
      outbox fallback），不受影响。

      * fix(chatops/notify): UpdateConf 合并配置而非整体覆盖，避免编辑时丢失字段

      之前 UpdateConf 把前端发的 partial config_json 整体加密覆盖到 DB，结果用户
      在 Web UI 编辑某一项时（比如只填 proxy_url），其他原有字段（admin_users /
      default_chat_id 等）全部丢失。

      新行为：UpdateConf 接到 config_json 时先解密 DB 现有值，json.Unmarshal 成
      map，与 partial 新值 merge（新值覆盖、缺失键保留），再加密回写。

      前端 loadDetail 同步修复：把 GET 返回的 config_json 对象 flatten 到表单
      绑定的 conf 上，避免编辑表单 v-model 显示为空。

      单测覆盖 partial update 不丢字段、partial 同 key 覆盖两个场景。

      * feat(chatops/notify): 通道配置变更后自动热重载，无需重启服务

      之前 UpdateConf / CreateConf / DeleteConf 修改 DB 后，运行中的通道实例
      继续使用启动时的旧配置（如 proxy_url、bot_token），用户必须手动重启服务
      才能让新配置生效。

      - NotificationService 在 Create/Update/DeleteConf 成功后 publish
       events.ConfigChanged{Source: "notification"}
      - cmd/web.go 在 bootstrapChatOps 后启动订阅 goroutine，收到该事件时
       重建 bs.channels：先逐个 Close 旧实例（带 chatopsShutdownPerStep
       超时），再调 initEnabledChannels 重新加载所有 enabled=true 的 conf
       并 Init，最后重新注册 RSS callback action handler 与 liveManager
      - 全量重建匹配 core/config_store.go 现有模式，避免逐通道 dirty 状态
       追踪；通道 Init 是轻量操作（TG 仅打开新 HTTPS client）
      - internal/app/notification_service_test.go 新增 4 个测试覆盖
       Create/Update/Delete 触发事件、空 UpdateConf 不触发事件

      实测：编辑 telegram conf 的 proxy_url 后 1ms 内通道重建完成，下一次
      SendMessage 自动通过新代理发送，无需重启进程。

      * fix(chatops/telegram): default_chat_id / admin_users 容错 string 与 int 混合输入

      之前 Config 用强类型 int64 / []int64，用户在 Web UI 表单填字符串
      （@channelusername、加引号的数字 "8576996727"、混合数组等）会让整个
      channel init 失败：

       ChatOps 通道初始化失败 conf_id=1 type=telegram: telegram: 解析
       config_json 失败: json: cannot unmarshal string into Go struct field
       Config.default_chat_id of type int64

      按 Telegram Bot API 官方语义，chat_id 同时支持 Integer 与 String
      (@channelusername)。改 Config 字段为 json.RawMessage，访问通过 helper
      （DefaultChatIDInt / DefaultChatIDUsername / AdminUsersList /
      AllowedUsersList）做容错解析：

      - 数字 → ID
      - 加引号的数字字符串 → 解析为 ID
      - @ 前缀字符串 → Username（@ 缺失自动补）
      - 混合数组中坏条目跳过、不再 abort init

      resolveChatID 返回 telego.ChatID 而非 int64，让 username 形式也能正常发
      （频道公告等用例需要）。inbound.go 鉴权改用 cfg.AdminUsersList() /
      cfg.AllowedUsersList()。

      新增单测覆盖 int / 加引号 int / @username / mixed array / bad entry 五种
      输入。

      * fix(test): rss_retry_worker_test 协调 mockNow 与 row CreatedAt 避免时区差异 fail

      之前测试用 real time.Now() 创建 row 但用 mock 2026-05-16 12:00 UTC 作为 worker now，
      两者时差导致 query 'next_retry_at <= mock_now' 在 CI（UTC）和某些时区本地都找不到
      row (past = real_now - 1min > mock_now)。

      改用一致 mockNow 作为整个测试的时间基准：row.CreatedAt / past / w.now() 全部锚定
      mockNow，确保 query 总能命中。

## [ext-v0.2.0] - 2026-05-16

### Bug Fixes

- **ci/extension-publish**: 通过 env 注入 secrets 防止 $ 字符被 shell 展开 ([#336](https://github.com/sunerpy/pt-tools/issues/336)) ([#336](https://github.com/sunerpy/pt-tools/pull/336))
  含 $ 字符的 EDGE_API_KEY 在 ${{ secrets.EDGE_API_KEY }} 模板展开后直接进入
      shell 命令字符串，bash 把 $BzUHf30Hi8G3N 当作变量替换为空，导致 API 鉴权
      401。改为通过 env: 注入 + shell 用 "$VAR" 引用，避免任何展开。

### Miscellaneous

- **extension**: 版本 0.1.0 → 0.2.0 准备首次 Edge 商店 API 发布 ([#335](https://github.com/sunerpy/pt-tools/issues/335)) ([#335](https://github.com/sunerpy/pt-tools/pull/335))

* chore(extension): 版本 0.1.0 → 0.2.0 准备首次 Edge 商店 API 发布

      * chore: oxfmt 同步 CHANGELOG.md (release-please 提交后)

## [0.31.1] - 2026-05-16

### Bug Fixes

- 修复 v0.31.0 发版公告链路 + 添加 CRX 构建 (v0.31.1) ([#333](https://github.com/sunerpy/pt-tools/issues/333)) ([#333](https://github.com/sunerpy/pt-tools/pull/333))

* docs(chatops): 移除不存在的占位截图引用

      botfather-create-bot.png / telegram-getupdates.png / napcat-* 系列 5 张
      图实际未捕获（属于用户侧 QQ / TG 客户端或 NapCat 容器 WebUI），文档里
      "请自行截图"的占位 UX 较差。改为：
      - 删除 ![...](path.png) 占位与对应"截图位置..."blockquote
      - 关键步骤补一行简洁文字提示（仅 getUpdates 那处加 from.id/chat.id 提示）
      - 周围编号步骤已充分描述操作，无需额外说明

      不影响已存在的 chatops-*.png（telegram-detail / bindings-* / notifications-list /
      qq-detail / add-channel-dialog / audit-stats / rss-notifications）。

      * fix(ci/tg-release): workflow 缺少 actions/checkout 导致脚本路径找不到

      * fix(ci/tg-release): 修复多 H2 / 缩进 bullet / PR 链接重复导致公告内容丢失

      v0.31.0 公告（msg_id 170）只显示了"…还有 3 项已省略"，关键内容全没。
      排查发现三个独立 bug：

      1. 缩进代码块启发式（4+ 空格 → 包成 code span）误把 release-please /
       git-cliff 的缩进 bullet 转成超长代码行，触发后续截断。删除该启发式。
      2. release body 多个 H2（`## What's Changed` 后跟 `## [0.31.0] - YYYY-MM-DD`），
       后者被渲染成"📌 [0.31.0] - ..."噪音。新增 _VERSION_TAG_H2 正则跳过版本
       标签 H2。
      3. release body 含重复的 PR 链接（[#330](issues) + [#330](pull) 两个），
       _preprocess_body 添加去重 regex。

      附加增强：
      - 缩进 bullet 三级渲染：0-1 空格 → •，2-5 → ◦，6+ → ▸
      - 缩进 bullet 不再继承 conventional commits emoji（顶层独占）
      - 智能截断：先丢 ▸ 层，再丢 ◦ 层，最后才硬截断 • 边界
      - 字节预算改为字符预算（TG 限制是 4096 UTF-16 chars 而非字节），
       中文场景下避免误判 over-budget
      - 剥离 release-please 追加的 Installation/Docker Images 等冗余章节

      实测：v0.31.0 真实 body 渲染后 3411 chars / 4704 bytes，无截断，
      40 个嵌套 bullet 全部可见，msg_id 172 已成功发送并 pin。

      * feat(ci/release): 每次 release 额外签名构建 .crx 浏览器扩展产物

      附在 release 资产里供用户直接安装到 Chrome（无需 unpacked 模式）。

      - release-please.yml: 在 zip 构建后追加 crx 构建（用 pnpm dlx crx + 解码
       CRX_PRIVATE_KEY 私钥）；签名后上传至 release assets，构建步骤受 secret
       存在性 guard 保护，缺失时安静 skip
      - release-please.yml: RELEASE_NOTES 模板增加 Browser Extension 区块，
       列出 zip / crx 两种下载格式
      - README-secrets.md: 文档化 CRX_PRIVATE_KEY 一次性设置流程（openssl 生成
       + base64 编码 + 妥善保管）
      - README.md: 浏览器扩展安装小节补充 .crx 直装说明

      注意：私钥必须 across releases 稳定，否则用户失去 auto-update 路径。

      * chore: oxfmt 同步 .github/README-secrets.md / CHANGELOG.md / docs/guide/chatops-telegram.md

## [0.31.0] - 2026-05-16

### Features

- ChatOps 机器人 + RSS 上新通知子系统 (v0.31.0) ([#330](https://github.com/sunerpy/pt-tools/issues/330)) ([#330](https://github.com/sunerpy/pt-tools/pull/330))

* feat(events): 扩展事件总线添加 Payload 字段以支持结构化负载

      * feat(crypto): 添加 AES-GCM 加密与 bind code 生成器

      - 新增 internal/crypto/aes_gcm.go：AES-256-GCM 对称加密
       - Encrypt/Decrypt round-trip，防篡改检测
       - 支持环境变量 PT_TOOLS_SECRET_KEY（base64 32 字节）
       - 无 env 时自动生成 ~/.pt-tools/secret.key（0600 权限）

      - 新增 internal/chatops/bindcode.go：8 字符无歧义绑定码生成器
       - 字符集：23456789ABCDEFGHJKMNPQRSTUVWXYZ（31 字符，排除 0/O/1/l/I）
       - 使用 crypto/rand + rejection sampling（无模偏差）
       - 2^40 ≈ 10^12 熵，足以满足 5 分钟 TTL 场景

      - 全部单测 TDD 流程：RED → GREEN → REFACTOR
      - 测试覆盖：round-trip、篡改检测、secret 文件生成、长度、唯一性、字符集约束

      * feat(scheduler): 暴露 ListJobs() 公开方法供 ChatOps 查询

      - 新增 JobStatus 类型导出（SiteName、RSSName、Running、StartedAt）
      - 实现 Manager.ListJobs() 返回当前运行中的所有 jobs
      - 为 job 结构体添加 startedAt 字段记录启动时间
      - 单测覆盖空状态、单job、并发安全场景（-race）

      * feat(models): 新增 ChatOps 五张表(notification_conf 等)，支持 additive 迁移

      - 新增 NotificationConf / ChannelBinding / ActionAudit / BotToken / NotificationOutbox 5 个 GORM model
      - 每个 model 实现 TableName() 强制单数表名
      - ChannelBinding 复合索引 (notification_conf_id, channel_user_id)
      - ActionAudit 复合索引 (notification_conf_id, channel_user_id, created_at DESC)
      - NotificationOutbox 复合索引 (status, next_retry_at) 用于 outbox worker 扫描
      - AutoMigrate 列表追加 5 个新类型，不 bump CurrentSchemaVersion
      - 单测覆盖 AutoMigrate / TableName / RoundTrip / 三类索引存在性 / Outbox 四态

      * feat(events): 添加 ChatOps 类型化事件常量与 payload 结构体

      - 定义 11 个 EventType 常量（torrent.added/completed/failed、free.ending_soon、free.ended、disk.low、cleanup.triggered、site.login_expired、site.scraped_daily、notification.delivered、notification.failed）
      - 为每个事件提供对应的 typed payload struct，使用 snake_case JSON tag
      - 配合 T1 的 PublishWithPayload 使用，为 chatops/MCP 提供机器可读的事件 schema
      - 不修改任何现有事件常量与发布站点，纯追加

      * feat(web): 添加 bearer token 鉴权中间件支持 bot 与 MCP 调用

      * feat(app): 引入 Application Service 层骨架(torrent/notification/binding/audit/task/site)

      - 新增 internal/app 包，封装 6 个应用服务（T10-T14）：
       - TorrentService：基于 downloader.DownloaderManager 的种子分页/暂停/恢复/删除
       - NotificationService：notification_conf CRUD + Push（同步 5s 超时回退 outbox）
       - BindingService：bind_code 签发/消费/吊销 + max 3 active cap + 并发安全
       - AuditService：action_audit 写入 + 递归 redact 敏感字段 + 90d/500k 清理
       - TaskService：封装 scheduler.Manager.ListJobs（Start/Stop 待 T32 接线）
       - SiteService：基于 ConfigStore + UserInfoRepo 的站点列表与用户信息

      - 测试：38 个单测全绿（含 -race），覆盖加密落库/race/redact/分页/接口契约
      - 接口边界：通过 mock-friendly 接口（NotifyManager/JobLister/UserInfoSource 等）
       与底层 manager/store 解耦，便于上层 chatops 命令与 web API 编程对接

## [0.30.1] - 2026-05-14

### Bug Fixes

- **web**: 修正 favicon 全站刷新行为并增加已支持站点视图
  修复用户反馈的"只配置了 2 个站点但日志显示刷新全部 40+ 站点"的问题。

        后端：
        - web/api_favicon.go: refreshExpiredFavicons 改为只迭代 SiteSetting
         中已启用 (Enabled=true) 的站点，新增 loadEnabledSiteIDsLower 辅助
        - web/api_favicon.go: apiFavicon 单图标 GET 支持 ?nofetch=1 query，
         避免"已支持站点"页面对未启用站点触发外部抓取
        - web/api_favicon.go: apiFaviconList 同样按启用状态过滤
        - web/api_site.go: 新增 GET /api/sites/definitions handler 暴露
         SiteDefinitionRegistry 的精简 DTO（id/name/aka/schema/urls/
         faviconUrl/authMethod/hrEnabled/unavailable*）
        - web/server.go: 注册 /api/sites/definitions 路由

        前端：
        - views/SupportedSites.vue: 新增"已支持站点"页面，卡片网格 + 搜索 +
         按 Schema 筛选 + 不可用状态徽章；favicon 用 noFetch=true 避免
         对未启用站点发起后端抓取
        - components/SiteAvatar.vue: 新增 noFetch prop，对应后端 ?nofetch=1
        - router/index.ts: 新增 /supported-sites 路由
        - App.vue: 侧栏菜单"站点与RSS"下方新增"已支持站点"入口
        - views/UserInfoDashboard.vue: 站点统计卡片操作区新增"已支持站点" CTA
        - api/index.ts: 新增 SupportedSiteDefinition 类型 + sitesApi.listDefinitions()

## [0.30.0] - 2026-05-14

### Features

- **sites**: 新增 2 个 NexusPHP 站点适配
  新增站点（均为 NexusPHP + Cookie 鉴权）：

        - GTKPW (pt.gtkpw.xyz / pt.gtk.pw / pt.gtk.xyz / t.myaltbox.com) — Issue #323
        - NicePT (好趣, www.nicept.net, 繁体界面) — Issue #321

        每站包含完整定义 + fixture 测试（搜索 / 详情 / 用户信息 + 无密钥校验）：

        - GTKPW: 4 个镜像域名同时注册，标准简体 NexusPHP 选择器
        - NicePT: 繁体优先选择器（傳送 / 上傳量 / 下載量 / 等級 / 用戶ID / 加入日期）
         + 简体 fallback；HasHR 解析；基本資訊 / 基本信息 size 解析；带 H&R 标志

        浏览器扩展 KNOWN_SITES + pt-sites.ts 同步更新（新增 pt.gtk.xyz、t.myaltbox.com）。
        docs/sites.md 已适配站点数从 39 → 41，NexusPHP 系列从 35 → 37。

### Styling

- **changelog**: 应用 oxfmt 格式化

## [0.29.0] - 2026-05-14

### Dependencies (Frontend)

- **pnpm**: Bump oxfmt from 0.47.0 to 0.49.0 in /web/frontend ([#311](https://github.com/sunerpy/pt-tools/issues/311)) ([#311](https://github.com/sunerpy/pt-tools/pull/311))
  Bumps [oxfmt](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxfmt) from 0.47.0 to 0.49.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxfmt/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxfmt_v0.49.0/npm/oxfmt)

        ---
        updated-dependencies:
        - dependency-name: oxfmt
         dependency-version: 0.49.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump vitest from 4.1.5 to 4.1.6 in /web/frontend ([#312](https://github.com/sunerpy/pt-tools/issues/312)) ([#312](https://github.com/sunerpy/pt-tools/pull/312))
  Bumps [vitest](https://github.com/vitest-dev/vitest/tree/HEAD/packages/vitest) from 4.1.5 to 4.1.6. - [Release notes](https://github.com/vitest-dev/vitest/releases) - [Commits](https://github.com/vitest-dev/vitest/commits/v4.1.6/packages/vitest)

        ---
        updated-dependencies:
        - dependency-name: vitest
         dependency-version: 4.1.6
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump vue from 3.5.33 to 3.5.34 in /web/frontend ([#314](https://github.com/sunerpy/pt-tools/issues/314)) ([#314](https://github.com/sunerpy/pt-tools/pull/314))
  Bumps [vue](https://github.com/vuejs/core) from 3.5.33 to 3.5.34. - [Release notes](https://github.com/vuejs/core/releases) - [Changelog](https://github.com/vuejs/core/blob/main/CHANGELOG.md) - [Commits](https://github.com/vuejs/core/compare/v3.5.33...v3.5.34)

        ---
        updated-dependencies:
        - dependency-name: vue
         dependency-version: 3.5.34
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump vite from 8.0.10 to 8.0.12 in /web/frontend ([#313](https://github.com/sunerpy/pt-tools/issues/313)) ([#313](https://github.com/sunerpy/pt-tools/pull/313))
  Bumps [vite](https://github.com/vitejs/vite/tree/HEAD/packages/vite) from 8.0.10 to 8.0.12. - [Release notes](https://github.com/vitejs/vite/releases) - [Changelog](https://github.com/vitejs/vite/blob/main/packages/vite/CHANGELOG.md) - [Commits](https://github.com/vitejs/vite/commits/v8.0.12/packages/vite)

        ---
        updated-dependencies:
        - dependency-name: vite
         dependency-version: 8.0.12
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump element-plus from 2.13.7 to 2.14.0 in /web/frontend ([#315](https://github.com/sunerpy/pt-tools/issues/315)) ([#315](https://github.com/sunerpy/pt-tools/pull/315))
  Bumps [element-plus](https://github.com/element-plus/element-plus) from 2.13.7 to 2.14.0. - [Release notes](https://github.com/element-plus/element-plus/releases) - [Changelog](https://github.com/element-plus/element-plus/blob/dev/CHANGELOG.en-US.md) - [Commits](https://github.com/element-plus/element-plus/compare/2.13.7...2.14.0)

        ---
        updated-dependencies:
        - dependency-name: element-plus
         dependency-version: 2.14.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump oxlint from 1.62.0 to 1.64.0 in /web/frontend ([#317](https://github.com/sunerpy/pt-tools/issues/317)) ([#317](https://github.com/sunerpy/pt-tools/pull/317))
  Bumps [oxlint](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxlint) from 1.62.0 to 1.64.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxlint/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxlint_v1.64.0/npm/oxlint)

        ---
        updated-dependencies:
        - dependency-name: oxlint
         dependency-version: 1.64.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

### Dependencies (Go)

- **go**: Bump golang.org/x/sys from 0.43.0 to 0.44.0 ([#309](https://github.com/sunerpy/pt-tools/issues/309)) ([#309](https://github.com/sunerpy/pt-tools/pull/309))
  Bumps [golang.org/x/sys](https://github.com/golang/sys) from 0.43.0 to 0.44.0. - [Commits](https://github.com/golang/sys/compare/v0.43.0...v0.44.0)

        ---
        updated-dependencies:
        - dependency-name: golang.org/x/sys
         dependency-version: 0.44.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

- **go**: Bump golang.org/x/text from 0.36.0 to 0.37.0 ([#310](https://github.com/sunerpy/pt-tools/issues/310)) ([#310](https://github.com/sunerpy/pt-tools/pull/310))
  Bumps [golang.org/x/text](https://github.com/golang/text) from 0.36.0 to 0.37.0. - [Release notes](https://github.com/golang/text/releases) - [Commits](https://github.com/golang/text/compare/v0.36.0...v0.37.0)

        ---
        updated-dependencies:
        - dependency-name: golang.org/x/text
         dependency-version: 0.37.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

### Features

- **sites**: 新增 3 个 NexusPHP 站点适配
  新增站点（均为 NexusPHP + Cookie 鉴权）：

        - DuckBoobee (鸭鸭, duckboobee.org) — Issue #319
        - LongPT (龙PT, longpt.org) — Issue #318
        - HDVideo (HD视频, hdvideo.top) — Issue #307

        每站包含完整定义 + fixture 测试（搜索 / 详情 / 用户信息 + 无密钥校验），
        浏览器扩展 KNOWN_SITES 同步更新（pt-sites.ts 域名已提前存在）。

        docs/sites.md 已适配站点数从 36 → 39，NexusPHP 系列从 32 → 35。

### Styling

- **changelog**: 应用 oxfmt 格式化

## [0.28.1] - 2026-05-10

### Bug Fixes

- **push**: 修复磁盘保护漏洞 - 推送前考虑种子大小、待下载累计与并发预留 (Issue #299) ([#304](https://github.com/sunerpy/pt-tools/issues/304)) ([#304](https://github.com/sunerpy/pt-tools/pull/304))

* fix(push): 修复磁盘保护漏洞 - 推送前考虑种子大小、待下载累计与并发预留 (Issue #299)

      根因诊断 —— 旧实现 freeGB < threshold 存在三个 race：
      1. 不扣除即将推送种子自身大小（连推 5 个 100GB 种子时各自看到相同 freeGB）
      2. qBit 默认 preallocate_all=false，新推送种子直到下载完成才反映在
       free_space_on_disk —— 多 RSS worker 并发推送基于过时数据通过检查
      3. GetClientFreeSpace 失败时 fail-open（"继续推送"）

      修复策略 = 三层减法 + 全局互斥锁：
       effective_free = client_free - in_flight_pending - pre_reserved
       gate = effective_free - thisTorrentSize >= threshold

      代码变更：
      - internal/disk_budget.go: 修复 Release 的 Add+CAS race（用 CompareAndSwap 循环
       保证不下溢）；新增 PushMutex() 全局互斥锁；新增 EffectiveFreeBytes 字节版本；
       Reset 用 GetSloggerSafe 避免单测 panic
      - internal/common.go: 推送前 PushMutex 上锁 → 查询 free + pending → 减 reserved
       → 减种子大小 → 比较阈值；通过则 Reserve；失败 Release；fail-closed 行为
      - internal/push.go: 同上同步策略；从 .torrent 元数据用 ComputeTorrentSize 推算
       种子内容大小
      - thirdpart/downloader/interface.go: Downloader 接口新增 GetIncompletePendingBytes；
       Torrent struct 新增 AmountLeft 字段
      - thirdpart/downloader/qbit/qbit_impl.go: 实现 GetIncompletePendingBytes 聚合
       10 种 active 状态的 amount_left；新增 ComputeTorrentSize 解析 .torrent 内容大小
      - thirdpart/downloader/transmission/transmission_impl.go: 实现同名方法聚合
       status ∈ {0,1,2,3,4} 的 leftUntilDone；GetAllTorrents 加 leftUntilDone 字段
      - mocks/downloader_mock.go + manager_test.go: 同步新接口方法

      测试覆盖（全部 race-free 通过）：
      - internal/disk_budget_test.go: 12 个测试覆盖 Reserve/Release/Reset/EffectiveFree
       / 全局单例 / PushMutex 串行化 / 高并发计数收敛 / RaceConditionGate 业务剧本
      - internal/disk_protect_test.go: 7 个测试覆盖
       RejectsWhenSizeExceedsEffectiveFree / AllowsWhenSizeFits /
       PendingDownloadsSubtracted / ReleaseOnPushFailure / FailClosedOnFreeSpaceError /
       PendingErrorTreatedAsZero / ConcurrentPushesSerializeAndReject
      - thirdpart/downloader/qbit/qbit_disk_protect_test.go: 8 个测试覆盖
       ComputeTorrentSize 单/多文件/空/非法 + GetIncompletePendingBytes 聚合规则

      * test(disk-protect): 修复 lint 问题（empty critical section + unnecessary conversions）

      - disk_budget_test.go: PushMutex 测试加占位以避免 SA2001 误判
      - disk_protect_test.go: 移除冗余的 int64() 转换（gb / free / pending 已是 int64）

## [0.28.0] - 2026-05-10

### Features

- **sites**: 新增 5 个 NexusPHP 站点适配 ([#303](https://github.com/sunerpy/pt-tools/issues/303)) ([#303](https://github.com/sunerpy/pt-tools/pull/303))

* feat(sites): 新增 5 个 NexusPHP 站点适配

      新增站点（均为 NexusPHP + Cookie 鉴权）：

      - CrabPT (蟹黄堡, crabpt.vip) — Issue #302
      - GameGamePT (GGPT, gamegamept.com) — Issue #301
      - Dubhe (天枢, dubhe.site) — Issue #300
      - PTCafe (咖啡, ptcafe.club) — Issue #296
      - CyanBug (大青虫, cyanbug.net) — Issue #295

      每站包含完整定义 + fixture 测试（搜索 / 详情 / 用户信息 + 无密钥校验），
      浏览器扩展 KNOWN_SITES 同步更新（pt-sites.ts 域名已提前存在）。

      docs/sites.md 已适配站点数从 31 → 36，NexusPHP 系列从 27 → 32。

      * chore(deps): 升级 Go 1.26.2 → 1.26.3 修复 stdlib 漏洞

      govulncheck 报告 Go 1.26.2 stdlib 4 个漏洞，1.26.3 已修复。

      - go.mod: go 1.26.2 → 1.26.3
      - Makefile: BUILD_IMAGE golang:1.26.2 → 1.26.3
      - Dockerfile: ARG BUILD_IMAGE 默认值同步

      CI go-security 在 1.26.2 下 fail（此为 v0.28.0-beta.1 之外的新发现），
      CHANGELOG 历史记录 (1.26.1→1.26.2 的迁移说明) 不动。

## [0.28.0-beta.1] - 2026-05-06

### Bug Fixes

- **qbit**: 兼容 qBittorrent v5.2.0+ 接口变更
  解决用户升级 qBit 到 5.2.0 后看到「下载器状态 204」的问题。qBit v5.2.0 的
  PR #21349 (https://github.com/qbittorrent/qBittorrent/pull/21349) 将大量
  无响应体的 endpoint 从 200 改为 204，且 /api/v2/torrents/add 改用新的
  JSON 响应格式（含 202 pending/409 duplicate 等新状态码），pt-tools 原有
  ~18 处 resp.StatusCode != http.StatusOK 严格检查会把 204 误判为失败。

        实现方案：版本探测 + 分支处理，完全兼容旧版 qBit (4.x/5.0/5.1)

        1) QbitClient 新增 appVersion/isV520Plus/versionMu 字段，登录成功后调
         /api/v2/app/version 探测版本，semver 比较 >=5.2.0 设置 isV520Plus。
         探测失败（网络错误/500/非法版本串）静默回退 legacy 模式，WARN 日志提示。

        2) AuthenticateWithContext 破解循环依赖：登录阶段版本未知，同时接受
         200 和 204。200 仅将 body == Fails. 视为凭证错误（容忍新版可能的空 body
         或非 Ok. 响应），204 直接当成功；认证成功后再做版本探测。401/403
         继续走 wrapStatusCodeError 原有映射。

        3) 新增 isSuccessStatus(code) helper：5.2.0+ 接受 2xx，旧版仅 200。
         替换所有非 Authenticate 路径的严格检查（20+ 处），保留 404 回退
         (pause/resume legacy 端点) 与 403 重新认证重试。

        4) AddTorrentEx / AddTorrentFileEx 新增 5.2.0+ 分支：
         - 200/202/204: 尝试解析新 JSON {success_count, pending_count,
         failure_count, added_torrent_ids}，解析失败回退 Ok./Fails. 字符串
         - 409 Conflict: 返回 Success=false，消息「种子已存在或添加失败」
         legacy 分支保持原有 200 + Ok. 字符串解析逻辑。

        5) parseQBitVersion(raw) 辅助函数，正则容错解析 v5.2.0/5.2.0/v5.2.0-rc1/
         qBittorrent v5.2.0 等格式。

        测试覆盖（新增 11 test + 11 sub-test，全部 PASS）：
        - TestAuthenticate_200LegacyOkBody/200FailsBody/204LoginSuccess/401InvalidCreds
        - TestDetectVersion_520Plus/Legacy/Fallback
        - TestParseQBitVersion (9 sub-cases)
        - TestIsSuccessStatus (2 sub-cases)
        - TestAddTorrentFileEx_202PendingJSON/409Duplicate

        验证：
        - go test ./... 21 个包全通过，qbit 包覆盖率 33.6%
        - make fmt + make lint-go 0 issues
        - 不变动：interface.go、transmission、http_doer.go、manager.go

        对旧版 qBit (4.x/5.0/5.1) 完全无副作用：版本探测识别为 legacy 时，
        isSuccessStatus 只接受 200，行为与修改前一致。对 qBit 5.2.0+ 用户，
        登录、pause/resume、add 等操作不再被 204 误判为失败。

### CI/CD

- 新增预发版发布工作流 (beta/rc/alpha)
  新增独立的预发版发布通道，支持在正式版之前发布 beta/rc/alpha 进行
  真实环境测试，测试通过后再合入正式版。

        变更内容：

        1. 新增 .github/workflows/release-prerelease.yml
         - 触发条件：push tag 匹配 v*-beta.N / v*-rc.N / v*-alpha.N
         - 触发时自动从 tag 中解析 channel（beta/rc/alpha）
         - 构建跨平台二进制 (linux/windows × amd64/arm64)
         - 构建 Docker 镜像并推送 :<version> + :<channel>（不动 :latest）
         - 发布 GitHub Release 带 prerelease=true 标志
         - 打包浏览器扩展 zip 一并发布
         - Release notes 头部警示「不建议生产环境使用」

        2. 修改 .github/workflows/release-assets.yml
         - tag 过滤器从 v* 收窄为 v[0-9]+.[0-9]+.[0-9]+
         - 防止预发版误触发两个 workflow 重复发布
         - 与 release-prerelease.yml 正则完全互斥

        3. 新增 make build-prerelease-docker TAG=... CHANNEL=... target
         - 与 build-remote-docker 的区别：不打 :latest，改打 :<channel>
         - 参数校验：TAG/CHANNEL 缺失时给出用法提示并 exit 1

        4. 修复 web/server.go 函数间缺失空行（gofumpt 历史遗留格式问题）

        使用方法：
         git tag v0.28.0-beta.1 <commit>
         git push origin v0.28.0-beta.1
         # 自动触发 release-prerelease.yml，发布预发版
         # 用户可通过 docker pull sunerpy/pt-tools:beta 测试
         # 测试通过后，走现有 release-please PR 发正式版 v0.28.0

        防呆设计：
        - 两个 workflow 的 tag 正则严格互斥
        - Makefile 参数校验防止误调用
        - channel 解析失败时 workflow 立即 exit 1

        验证：
        - YAML 语法合法
        - Tag 正则覆盖测试：v0.28.0-beta.1/rc.2/alpha.5 通过，v0.28.0 和 v1.2.3-dev 拒绝
        - Makefile dry-run 只打 :<version> + :<channel> 两个 tag，无 :latest
        - make fmt + make lint-go 0 issues

### Dependencies (Frontend)

- **pnpm**: Bump @vueuse/core from 14.2.1 to 14.3.0 in /web/frontend ([#286](https://github.com/sunerpy/pt-tools/issues/286)) ([#286](https://github.com/sunerpy/pt-tools/pull/286))
  Bumps [@vueuse/core](https://github.com/vueuse/vueuse/tree/HEAD/packages/core) from 14.2.1 to 14.3.0. - [Release notes](https://github.com/vueuse/vueuse/releases) - [Commits](https://github.com/vueuse/vueuse/commits/v14.3.0/packages/core)

        ---
        updated-dependencies:
        - dependency-name: "@vueuse/core"
         dependency-version: 14.3.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump vue-tsc from 3.2.7 to 3.2.8 in /web/frontend ([#289](https://github.com/sunerpy/pt-tools/issues/289)) ([#289](https://github.com/sunerpy/pt-tools/pull/289))
  Bumps [vue-tsc](https://github.com/vuejs/language-tools/tree/HEAD/packages/tsc) from 3.2.7 to 3.2.8. - [Release notes](https://github.com/vuejs/language-tools/releases) - [Changelog](https://github.com/vuejs/language-tools/blob/master/CHANGELOG.md) - [Commits](https://github.com/vuejs/language-tools/commits/v3.2.8/packages/tsc)

        ---
        updated-dependencies:
        - dependency-name: vue-tsc
         dependency-version: 3.2.8
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump dompurify from 3.4.1 to 3.4.2 in /web/frontend ([#287](https://github.com/sunerpy/pt-tools/issues/287)) ([#287](https://github.com/sunerpy/pt-tools/pull/287))
  Bumps [dompurify](https://github.com/cure53/DOMPurify) from 3.4.1 to 3.4.2. - [Release notes](https://github.com/cure53/DOMPurify/releases) - [Commits](https://github.com/cure53/DOMPurify/compare/3.4.1...3.4.2)

        ---
        updated-dependencies:
        - dependency-name: dompurify
         dependency-version: 3.4.2
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

### Dependencies (Go)

- **go**: Bump go.uber.org/zap from 1.27.1 to 1.28.0 ([#285](https://github.com/sunerpy/pt-tools/issues/285)) ([#285](https://github.com/sunerpy/pt-tools/pull/285))
  Bumps [go.uber.org/zap](https://github.com/uber-go/zap) from 1.27.1 to 1.28.0. - [Release notes](https://github.com/uber-go/zap/releases) - [Changelog](https://github.com/uber-go/zap/blob/master/CHANGELOG.md) - [Commits](https://github.com/uber-go/zap/compare/v1.27.1...v1.28.0)

        ---
        updated-dependencies:
        - dependency-name: go.uber.org/zap
         dependency-version: 1.28.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

### Features

- **version**: 预发版识别与 UI 突出显示
  用户在版本更新检测和自升级流程中需要直观区分预发版（beta/rc/alpha）与
  正式版，并在升级到预发版前给出明确警示，避免误升级到测试版本。

        实现方案：后端识别 + 前端 delta-ui 风格的多维度视觉差异化。

        后端变更 (version/checker.go + web/api_version.go):

        1) ReleaseInfo DTO 新增 Prerelease / PrereleaseLabel 字段，透传 GitHub API
         的 prerelease 标记和从 tag 解析出的通道名 (beta/rc/alpha)。
        2) 新增 extractPrereleaseLabel 辅助：从 semver 预发版段解析通道名，
         白名单只认 beta/rc/alpha 三类，dev/snapshot 等非标准标签返回空。
        3) filterNewReleases 增加 includePrerelease 参数：默认 false 保持向后
         兼容；双重信号识别预发版 (r.Prerelease || tag 后缀)，防止发版时
         漏勾 GitHub prerelease 勾选导致的漏判。
        4) CheckOptions 增加 IncludePrerelease；缓存 key 同时考虑此字段，
         避免用户切换开关时拿到过期缓存。
        5) /api/version/check 接受新查询参数 include_prerelease=true。

        前端变更 (Vue 3 + Element Plus):

        1) TS 类型 ReleaseInfo 同步添加 prerelease / prerelease_label 字段；
         versionApi.checkUpdate 接受 includePrerelease 选项。
        2) Pinia store 新增 showPrerelease 开关 (localStorage 持久化) +
         hasPrereleaseUpdate / onlyPrereleaseUpdates computed + setShowPrerelease
         action (切换后自动强制刷新)。
        3) VersionChecker.vue 多维度视觉差异化：
         - 右上角按钮：仅预发版更新时按钮色改为 warning，红点改用 danger 色
         - 弹层顶部新增「含预发版」开关 + 悬浮帮助提示
         - 更新列表头部 tag 区分「仅发现预发版」/「发现新版本（含预发版）」/「发现新版本」
         - 存在预发版时显示 el-alert 警示「非必要请等待正式版」
         - 预发版卡片整体用 warning 色边框/背景 + 顶部 3px 渐变条
         - 版本号徽章在预发版使用 warning 色
         - 版本号旁新增大写通道徽章 BETA/RC/ALPHA
         - 升级按钮对预发版文案改为「升级到预发版」并使用 warning 色
         - 点击预发版升级先弹 ElMessageBox 确认对话框，文案强调测试风险
        4) 样式参考 delta-ui 设计语言：pill 徽章、柔和层叠阴影、color-mix 混色、
         顶部渐变强调条，保持与现有 pt-tools 设计 token (pt-color-warning) 一致。

        测试覆盖 (5 个新增用例全部通过):
        - TestFilterNewReleases_IncludePrerelease: 开关语义 + 字段填充 + 排序
        - TestFilterNewReleases_TagSuffixDetection: 漏勾 prerelease 时的 tag 后缀识别
        - TestExtractPrereleaseLabel: 9 个输入覆盖白名单 + 大小写 + dot 分段

### Styling

- 应用 gofumpt v0.10.0 列对齐格式化
  CI 执行 go install mvdan.cc/gofumpt@latest 拉到 v0.10.0，而本地之前是 v0.9.2，
  新版会对相邻的单行方法签名做列对齐。整理 17 个文件的格式差异。

        仅格式变动，无逻辑改动。

## [0.27.0] - 2026-05-02

### Features

- **site**: 新增 Farmm (pt.0ff.cc) 站点适配
  实现 Issue #279 站点请求。Farmm 是 CHD/scenetorrents 模板衍生的 NexusPHP
  站点，需处理两个非标准结构：

        1. 详情页 size 内联于「基本信息」行
         标准 NexusPHP 详情页有独立的 td.rowhead:contains("大小") 行，此站点将
         size 与类型/地区/分辨率等字段全部塞进「基本信息」cell 内，用 <b> 标签
         分隔。SizeRegex 从该 cell 文本中抽取数值与单位。

        2. 用户详情页「传输」行 HDSky 风格
         分享率/上传量/下载量三个字段打包在同一 td.rowfollow 中。选择器用正则
         提取每个字段，关键点：用 (?:^|[^实]) 前缀锚定 上传量，避免误匹配
         实际上传量（此坑在 hdsky.go 也有同样处理）。

        其他字段：
        - 9 列搜索表格（无隐藏列），td:nth-child(1..9) 分别对应类型/标题/评论数/
         时间/大小/种子数/下载数/完成数/发布者
        - 详情页 discount 在 h1#top 的 font.free/twoupfree/halfdown 结构内，
         无 domTT tooltip 所以无法获取 end_time
        - 此站点无 H&R 标记（列表页和详情页均无 hitandrun/hit_run.gif）
        - 浏览器扩展域名 pt.0ff.cc 已在 pt-sites.ts（无需修改）
        - constants.ts KNOWN_SITES 新增 pt0ffcc 条目
        - docs/sites.md 已适配站点从 30 增至 31（NexusPHP 系列 27 个）

        测试（全部通过）：
        - Search: 2 行 fixture，验证 id/title/discount/seeders/leechers/snatched
        - Detail: FREE discount、9.07 GB size、noHR
        - UserInfo: IndexPage（登录用户 info_block 的 seeding/leeching）+
         UserdetailsPage（被查看用户的 name/level）+ 4 个 regex 驱动字段
         必定非空（防止 上传量/实际上传量 混淆）
        - 全量回归: go test ./... 21 个包全通过，make lint 0 issues，
         vue-tsc 0 errors，extension 一致性检查通过 (31 个站点)

## [0.26.1] - 2026-05-02

### Bug Fixes

- **ci**: 修复站点请求 Issue 附件检查遗漏 comment 场景

## [0.26.0] - 2026-04-30

### Features

- **downloader**: 支持按站点配置种子上传/下载限速
  实现 issue #276：为每个 PT 站点配置独立的种子上传和下载速度限制，
  推送种子到下载器时自动应用。

        设计：
        - 上传和下载限速完全对称（用户可以分别配置或只配一项）
        - qBittorrent: 在 /api/v2/torrents/add 时通过 upLimit/dlLimit 原子设置
         （qBit 4.1+ 原生支持）
        - Transmission: torrent-add 不支持 limit 字段，实现 add-paused →
         torrent-set → torrent-start 的 3 步流程；若用户原本要求 paused
         则跳过最后一步
        - 全局 DownloadSpeedLimit 不受影响（它是 pt-tools 内部计算 "能否在免费期
         内下完" 的参数，与本次 per-torrent 限速语义正交）

        实现：
        - models.SiteSetting + SiteConfig: 新增 UploadLimitKBs + DownloadLimitKBs 字段
        - downloader.AddTorrentOptions:
         * 新增 UploadSpeedLimitKBs + DownloadSpeedLimitKBs（KB/s，更细粒度）
         * 原 UploadSpeedLimitMB 标记为 Deprecated 但保留向后兼容
         * 新增 EffectiveUploadLimitBytes() / EffectiveDownloadLimitBytes() 辅助方法
         统一返回 bytes/s，KBs 字段优先于 MB
        - qbit.AddTorrentFileEx: 改用 EffectiveXxxLimitBytes，同时支持 upLimit 和 dlLimit
        - transmission.AddTorrentFileEx: 限速时自动 paused → set → start
        - internal/push.go: 新增 applySiteSpeedLimits() 从 SiteSetting 查询并填入 opts
        - core/config_store.go: 三处 SiteConfig 构造 + UpsertSiteWithRSS 持久化新字段
        - web/server.go: SiteConfigResponse 新增两个字段
        - 前端: SiteConfig 类型 + SiteDetail.vue 新增两个 el-input-number

        测试（全部通过，覆盖所有关键场景）：
        - thirdpart/downloader/speed_limit_test.go (2 testcase)：
         * EffectiveUploadLimitBytes 优先级链（KBs > MB）+ 单位换算
         * EffectiveDownloadLimitBytes 单位换算
        - thirdpart/downloader/qbit/qbit_speed_limit_test.go (8 testcase)：
         * UploadLimitKBs / DownloadLimitKBs / 双限速同时设置
         * 零值不发字段（向后兼容）
         * 负值忽略
         * 遗留 MB 字段向后兼容
         * KBs 优先于 MB
         * 其他字段不受影响（回归守卫）
        - thirdpart/downloader/transmission/transmission_speed_limit_test.go (8 testcase)：
         * 无限速：单次 torrent-add 调用
         * AutoStart + 限速：完整 add→set→start 三步链路 + 正确 KB/s 单位
         * 用户明确要求 paused：不调用 torrent-start
         * 下载限速 / 双限速 / MB 字段兼容 / 调用顺序 / 负值
        - internal/push_speed_limit_test.go (7 testcase)：
         * 站点配置正确读取并传递
         * 零值站点正确传递
         * 未知站点 / 空 siteID / nil opts / nil DB 全部安全 no-op
         * 站点配置更新立即生效（无缓存）

        全量测试：go test ./... 21 个包全通过，lint 0 warnings，vue-tsc 0 errors。

        兼容性：GORM AutoMigrate 自动添加新列，所有新字段零值默认，升级后旧
        行为不变；遗留 UploadSpeedLimitMB 字段继续生效。

## [0.25.1] - 2026-04-29

### Bug Fixes

- **filter**: 修复关联过滤规则后仍自动下载非匹配免费种子的问题
  用户反馈："虽然设置了过滤规则，但实际上还是只要免费的就下载，感觉还是 free 和
  filter 是 or 的关系，不是 and 的关系"。

        根因：auto_free 模式下，即使 RSS 关联了过滤规则，若种子未匹配规则，仍会走免费通道
        兜底下载。此 OR 语义违反"设置过滤规则 = 精准下载"的用户直觉。

        修复（Plan A 智能模式语义）：
        - auto_free + RSS 关联了规则 → 免费通道自动关闭，仅下载匹配规则的种子（精准模式）
        - auto_free + RSS 无关联规则 → 免费通道开启，自动下载免费种子（保留 v0.25 行为）
        - filter_only / free_only → 保持原语义不变

        实现：
        - filter.Service 新增 hasAssociatedRules 内部方法，判断 RSS 是否关联任何规则
        - Decide 决策树新增"免费通道门控"：有规则时禁用免费通道兜底
        - buildDecisionReason 增加 hasRules 参数，跳过原因说明为 RSS 关联规则的精准模式
        - evaluateTestDecision（规则测试 UI）同步新语义，测试中总是视为"有规则"

        测试：
        - 新增 TestDecide_PlanA_UserReportedBug 永久回归守卫，确保问题不再出现
        - 新增 TestDecide_AutoFreeMode_NoRules_KeepsFreeChannel 守护无规则的旧行为
        - 更新 TestDecide_AutoFreeMode_CombinedChannels 预期：非匹配免费 → 拒绝
        - 重命名 *_FallsBackToFreeChannel 为 *_RejectsUnderPlanA，反映新预期

        UI 更新：
        - 过滤规则页警告条：明确"关联规则 = 精准下载，不再附带自动下免费"
        - 全局设置：auto_free 更名为"智能模式"，增加 v0.26.0 行为变更提示
        - 站点详情 RSS 编辑：下载模式标签同步更新并解释智能模式行为

## [0.25.0] - 2026-04-29

### Features

- **filter**: 新增 FilterMode 与过滤规则大小约束，修复全局大小被绕过问题
  修复全局 TorrentSizeGB 被过滤规则绕过的 bug：过去 shouldDownload = filter || free 是 OR 逻辑，
  只要规则命中就会绕过全局大小限制。现在全局大小是所有通道的硬上限。

        - FilterRule 新增 MinSizeGB/MaxSizeGB 字段，规则可进一步收紧大小范围（不能突破全局上限）
        - 新增 FilterMode (auto_free/filter_only/free_only)，支持 3 种下载策略：
         * auto_free（默认）: 免费通道 + 过滤规则通道，兼容旧行为
         * filter_only: 仅匹配过滤规则的种子才下载
         * free_only: 仅免费种子自动下载，忽略过滤规则
        - FilterMode 支持 RSS 级别覆盖全局默认（GetEffectiveFilterMode 实现 RSS > Global > Default 优先级）
        - filter.Service 新增 Decide/DecideWithoutRules 方法，统一决策树：
         全局大小硬上限 → 过滤规则通道 → 免费通道
        - internal/common.go 两条 RSS 工作路径（Unified + legacy）统一改用 Decide
        - 规则测试 UI 新增完整决策模拟：种子大小/免费状态/全局上限/模式，输出 决策结果/原因/下载通道
        - 全局设置增加"默认下载模式"单选组
        - RSS 订阅编辑增加"下载模式"选择器（空值=跟随全局）
        - 新增测试 internal/filter/decide_test.go 覆盖 30+ 场景（含 bug 回归守卫）
        - 新增测试 models/filter_rule_size_test.go 覆盖 MatchesSize 边界和 FilterMode 优先级
        - TorrentMetadata 接口新增 GetSizeBytes 方法供 Decide 获取种子大小

## [0.24.0] - 2026-04-29

### Features

- **site**: 新增 OpenCD 和 PTT 站点适配
- 新增 site/v2/definitions/opencd.go 适配 open.cd (繁体 NexusPHP)
  _ 使用 div.title + td.rowtitle 替代标准 h1 + td.rowhead
  _ 支持 plugin\*details.php 链接格式
  - 完整 UserInfo / Search / DetailParser 配置 + fixture 测试 - 新增 site/v2/definitions/pttime.go 适配 www.pttime.org (PTT-NP 分支)
  - 处理 font.promotion 替代 img.pro\*_ 的非标准折扣标记
    _ span.category 替代 img[alt] 的分类标记
    _ 处理 info_block 隐藏列的 nth-child 索引偏移
    _ 处理 "上传:" / "下载:" 无 "量" 后缀的 userinfo 标签 \* 完整 fixture 测试覆盖 Search/Detail/UserInfo - 浏览器扩展 constants.ts 注册 opencd 和 pttime 至 KNOWN_SITES - docs/sites.md 更新适配站点列表至 30 个 - Closes #233 #250

## [0.23.0] - 2026-04-29

### Dependencies (Frontend)

- **pnpm**: Bump oxfmt from 0.43.0 to 0.45.0 in /web/frontend ([#243](https://github.com/sunerpy/pt-tools/issues/243)) ([#243](https://github.com/sunerpy/pt-tools/pull/243))
  Bumps [oxfmt](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxfmt) from 0.43.0 to 0.45.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxfmt/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxfmt_v0.45.0/npm/oxfmt)

        ---
        updated-dependencies:
        - dependency-name: oxfmt
         dependency-version: 0.45.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump vue from 3.5.31 to 3.5.32 in /web/frontend ([#244](https://github.com/sunerpy/pt-tools/issues/244)) ([#244](https://github.com/sunerpy/pt-tools/pull/244))
  Bumps [vue](https://github.com/vuejs/core) from 3.5.31 to 3.5.32. - [Release notes](https://github.com/vuejs/core/releases) - [Changelog](https://github.com/vuejs/core/blob/main/CHANGELOG.md) - [Commits](https://github.com/vuejs/core/compare/v3.5.31...v3.5.32)

        ---
        updated-dependencies:
        - dependency-name: vue
         dependency-version: 3.5.32
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump vite from 8.0.5 to 8.0.8 in /web/frontend ([#245](https://github.com/sunerpy/pt-tools/issues/245)) ([#245](https://github.com/sunerpy/pt-tools/pull/245))
  Bumps [vite](https://github.com/vitejs/vite/tree/HEAD/packages/vite) from 8.0.5 to 8.0.8. - [Release notes](https://github.com/vitejs/vite/releases) - [Changelog](https://github.com/vitejs/vite/blob/main/packages/vite/CHANGELOG.md) - [Commits](https://github.com/vitejs/vite/commits/v8.0.8/packages/vite)

        ---
        updated-dependencies:
        - dependency-name: vite
         dependency-version: 8.0.8
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump @vitejs/plugin-vue from 6.0.5 to 6.0.6 in /web/frontend ([#246](https://github.com/sunerpy/pt-tools/issues/246)) ([#246](https://github.com/sunerpy/pt-tools/pull/246))
  Bumps [@vitejs/plugin-vue](https://github.com/vitejs/vite-plugin-vue/tree/HEAD/packages/plugin-vue) from 6.0.5 to 6.0.6. - [Release notes](https://github.com/vitejs/vite-plugin-vue/releases) - [Changelog](https://github.com/vitejs/vite-plugin-vue/blob/main/packages/plugin-vue/CHANGELOG.md) - [Commits](https://github.com/vitejs/vite-plugin-vue/commits/plugin-vue@6.0.6/packages/plugin-vue)

        ---
        updated-dependencies:
        - dependency-name: "@vitejs/plugin-vue"
         dependency-version: 6.0.6
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump element-plus from 2.13.6 to 2.13.7 in /web/frontend ([#247](https://github.com/sunerpy/pt-tools/issues/247)) ([#247](https://github.com/sunerpy/pt-tools/pull/247))
  Bumps [element-plus](https://github.com/element-plus/element-plus) from 2.13.6 to 2.13.7. - [Release notes](https://github.com/element-plus/element-plus/releases) - [Changelog](https://github.com/element-plus/element-plus/blob/dev/CHANGELOG.en-US.md) - [Commits](https://github.com/element-plus/element-plus/compare/2.13.6...2.13.7)

        ---
        updated-dependencies:
        - dependency-name: element-plus
         dependency-version: 2.13.7
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump oxlint from 1.58.0 to 1.60.0 in /web/frontend ([#248](https://github.com/sunerpy/pt-tools/issues/248)) ([#248](https://github.com/sunerpy/pt-tools/pull/248))
  Bumps [oxlint](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxlint) from 1.58.0 to 1.60.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxlint/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxlint_v1.60.0/npm/oxlint)

        ---
        updated-dependencies:
        - dependency-name: oxlint
         dependency-version: 1.60.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump @types/node from 25.5.2 to 25.6.0 in /web/frontend ([#249](https://github.com/sunerpy/pt-tools/issues/249)) ([#249](https://github.com/sunerpy/pt-tools/pull/249))
  Bumps [@types/node](https://github.com/DefinitelyTyped/DefinitelyTyped/tree/HEAD/types/node) from 25.5.2 to 25.6.0. - [Release notes](https://github.com/DefinitelyTyped/DefinitelyTyped/releases) - [Commits](https://github.com/DefinitelyTyped/DefinitelyTyped/commits/HEAD/types/node)

        ---
        updated-dependencies:
        - dependency-name: "@types/node"
         dependency-version: 25.6.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump typescript from 6.0.2 to 6.0.3 in /web/frontend ([#251](https://github.com/sunerpy/pt-tools/issues/251)) ([#251](https://github.com/sunerpy/pt-tools/pull/251))
  Bumps [typescript](https://github.com/microsoft/TypeScript) from 6.0.2 to 6.0.3. - [Release notes](https://github.com/microsoft/TypeScript/releases) - [Commits](https://github.com/microsoft/TypeScript/compare/v6.0.2...v6.0.3)

        ---
        updated-dependencies:
        - dependency-name: typescript
         dependency-version: 6.0.3
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump oxlint from 1.60.0 to 1.61.0 in /web/frontend ([#253](https://github.com/sunerpy/pt-tools/issues/253)) ([#253](https://github.com/sunerpy/pt-tools/pull/253))
  Bumps [oxlint](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxlint) from 1.60.0 to 1.61.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxlint/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxlint_v1.61.0/npm/oxlint)

        ---
        updated-dependencies:
        - dependency-name: oxlint
         dependency-version: 1.61.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump dompurify from 3.3.3 to 3.4.0 in /web/frontend ([#256](https://github.com/sunerpy/pt-tools/issues/256)) ([#256](https://github.com/sunerpy/pt-tools/pull/256))
  Bumps [dompurify](https://github.com/cure53/DOMPurify) from 3.3.3 to 3.4.0. - [Release notes](https://github.com/cure53/DOMPurify/releases) - [Commits](https://github.com/cure53/DOMPurify/compare/3.3.3...3.4.0)

        ---
        updated-dependencies:
        - dependency-name: dompurify
         dependency-version: 3.4.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump vitest from 4.1.2 to 4.1.4 in /web/frontend ([#257](https://github.com/sunerpy/pt-tools/issues/257)) ([#257](https://github.com/sunerpy/pt-tools/pull/257))
  Bumps [vitest](https://github.com/vitest-dev/vitest/tree/HEAD/packages/vitest) from 4.1.2 to 4.1.4. - [Release notes](https://github.com/vitest-dev/vitest/releases) - [Commits](https://github.com/vitest-dev/vitest/commits/v4.1.4/packages/vitest)

        ---
        updated-dependencies:
        - dependency-name: vitest
         dependency-version: 4.1.4
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump vue-tsc from 3.2.6 to 3.2.7 in /web/frontend ([#254](https://github.com/sunerpy/pt-tools/issues/254)) ([#254](https://github.com/sunerpy/pt-tools/pull/254))
  Bumps [vue-tsc](https://github.com/vuejs/language-tools/tree/HEAD/packages/tsc) from 3.2.6 to 3.2.7. - [Release notes](https://github.com/vuejs/language-tools/releases) - [Changelog](https://github.com/vuejs/language-tools/blob/master/CHANGELOG.md) - [Commits](https://github.com/vuejs/language-tools/commits/v3.2.7/packages/tsc)

        ---
        updated-dependencies:
        - dependency-name: vue-tsc
         dependency-version: 3.2.7
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump vite from 8.0.8 to 8.0.9 in /web/frontend ([#252](https://github.com/sunerpy/pt-tools/issues/252)) ([#252](https://github.com/sunerpy/pt-tools/pull/252))
  Bumps [vite](https://github.com/vitejs/vite/tree/HEAD/packages/vite) from 8.0.8 to 8.0.9. - [Release notes](https://github.com/vitejs/vite/releases) - [Changelog](https://github.com/vitejs/vite/blob/main/packages/vite/CHANGELOG.md) - [Commits](https://github.com/vitejs/vite/commits/v8.0.9/packages/vite)

        ---
        updated-dependencies:
        - dependency-name: vite
         dependency-version: 8.0.9
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump oxfmt from 0.45.0 to 0.46.0 in /web/frontend ([#255](https://github.com/sunerpy/pt-tools/issues/255)) ([#255](https://github.com/sunerpy/pt-tools/pull/255))
  Bumps [oxfmt](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxfmt) from 0.45.0 to 0.46.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxfmt/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxfmt_v0.46.0/npm/oxfmt)

        ---
        updated-dependencies:
        - dependency-name: oxfmt
         dependency-version: 0.46.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump oxlint from 1.61.0 to 1.62.0 in /web/frontend ([#261](https://github.com/sunerpy/pt-tools/issues/261)) ([#261](https://github.com/sunerpy/pt-tools/pull/261))
  Bumps [oxlint](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxlint) from 1.61.0 to 1.62.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxlint/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxlint_v1.62.0/npm/oxlint)

        ---
        updated-dependencies:
        - dependency-name: oxlint
         dependency-version: 1.62.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump dompurify from 3.4.0 to 3.4.1 in /web/frontend ([#262](https://github.com/sunerpy/pt-tools/issues/262)) ([#262](https://github.com/sunerpy/pt-tools/pull/262))
  Bumps [dompurify](https://github.com/cure53/DOMPurify) from 3.4.0 to 3.4.1. - [Release notes](https://github.com/cure53/DOMPurify/releases) - [Commits](https://github.com/cure53/DOMPurify/compare/3.4.0...3.4.1)

        ---
        updated-dependencies:
        - dependency-name: dompurify
         dependency-version: 3.4.1
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump vue from 3.5.32 to 3.5.33 in /web/frontend ([#264](https://github.com/sunerpy/pt-tools/issues/264)) ([#264](https://github.com/sunerpy/pt-tools/pull/264))
  Bumps [vue](https://github.com/vuejs/core) from 3.5.32 to 3.5.33. - [Release notes](https://github.com/vuejs/core/releases) - [Changelog](https://github.com/vuejs/core/blob/main/CHANGELOG.md) - [Commits](https://github.com/vuejs/core/compare/v3.5.32...v3.5.33)

        ---
        updated-dependencies:
        - dependency-name: vue
         dependency-version: 3.5.33
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump vitest from 4.1.4 to 4.1.5 in /web/frontend ([#266](https://github.com/sunerpy/pt-tools/issues/266)) ([#266](https://github.com/sunerpy/pt-tools/pull/266))
  Bumps [vitest](https://github.com/vitest-dev/vitest/tree/HEAD/packages/vitest) from 4.1.4 to 4.1.5. - [Release notes](https://github.com/vitest-dev/vitest/releases) - [Commits](https://github.com/vitest-dev/vitest/commits/v4.1.5/packages/vitest)

        ---
        updated-dependencies:
        - dependency-name: vitest
         dependency-version: 4.1.5
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump oxfmt from 0.46.0 to 0.47.0 in /web/frontend ([#263](https://github.com/sunerpy/pt-tools/issues/263)) ([#263](https://github.com/sunerpy/pt-tools/pull/263))
  Bumps [oxfmt](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxfmt) from 0.46.0 to 0.47.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxfmt/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxfmt_v0.47.0/npm/oxfmt)

        ---
        updated-dependencies:
        - dependency-name: oxfmt
         dependency-version: 0.47.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump vue-router from 5.0.4 to 5.0.6 in /web/frontend ([#265](https://github.com/sunerpy/pt-tools/issues/265)) ([#265](https://github.com/sunerpy/pt-tools/pull/265))
  Bumps [vue-router](https://github.com/vuejs/router) from 5.0.4 to 5.0.6. - [Release notes](https://github.com/vuejs/router/releases) - [Commits](https://github.com/vuejs/router/compare/v5.0.4...v5.0.6)

        ---
        updated-dependencies:
        - dependency-name: vue-router
         dependency-version: 5.0.6
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump vite from 8.0.9 to 8.0.10 in /web/frontend ([#267](https://github.com/sunerpy/pt-tools/issues/267)) ([#267](https://github.com/sunerpy/pt-tools/pull/267))
  Bumps [vite](https://github.com/vitejs/vite/tree/HEAD/packages/vite) from 8.0.9 to 8.0.10. - [Release notes](https://github.com/vitejs/vite/releases) - [Changelog](https://github.com/vitejs/vite/blob/main/packages/vite/CHANGELOG.md) - [Commits](https://github.com/vitejs/vite/commits/v8.0.10/packages/vite)

        ---
        updated-dependencies:
        - dependency-name: vite
         dependency-version: 8.0.10
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

### Dependencies (Go)

- **go**: Bump golang.org/x/text from 0.35.0 to 0.36.0 ([#241](https://github.com/sunerpy/pt-tools/issues/241)) ([#241](https://github.com/sunerpy/pt-tools/pull/241))
  Bumps [golang.org/x/text](https://github.com/golang/text) from 0.35.0 to 0.36.0. - [Release notes](https://github.com/golang/text/releases) - [Commits](https://github.com/golang/text/compare/v0.35.0...v0.36.0)

        ---
        updated-dependencies:
        - dependency-name: golang.org/x/text
         dependency-version: 0.36.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

- **go**: Bump golang.org/x/sys from 0.42.0 to 0.43.0 ([#242](https://github.com/sunerpy/pt-tools/issues/242)) ([#242](https://github.com/sunerpy/pt-tools/pull/242))
  Bumps [golang.org/x/sys](https://github.com/golang/sys) from 0.42.0 to 0.43.0. - [Commits](https://github.com/golang/sys/compare/v0.42.0...v0.43.0)

        ---
        updated-dependencies:
        - dependency-name: golang.org/x/sys
         dependency-version: 0.43.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

### Features

- **scheduler**: 新增做种竞争度监控功能
- 新增 PeerRatioMonitor 监控做种中种子的 S/L 比值 - 通过 Tracker API 获取 Swarm 级别的做种/下载用户数 - 超过阈值时自动暂停或删除（可配置），避免无效做种占用资源 - 仅管理 DB 内已追踪的种子，遵循 scope isolation 原则 - cleanup_monitor 增加互斥检查，避免对已暂停种子重复处理 - TorrentInfo 新增 seeders/leechers 字段持久化 Tracker 数据 - AutoCleanup 前端新增配置卡片：启用开关、S/L 阈值、检查间隔、删除/暂停模式 - 更新 docs/sites.md 已适配站点列表至 28 个

## [0.22.4] - 2026-04-11

### Bug Fixes

- **ci**: 修复 stale issue 自动关闭缺少7天宽限期和用户回复检测
- 关闭前检查 stale-close 警告是否已过 CLOSE_GRACE_DAYS(7天) - 警告发出后如有非 Bot 用户回复则跳过关闭
- **api**: 修复下载器目录接口返回格式与前端不匹配
- all-directories 接口返回数组而非映射导致前端无法按下载器ID索引目录 - 改为返回 map[downloaderID][]DownloaderDirectoryResponse - 修复所有站点推送种子和RSS订阅配置时选择不了下载目录的问题

## [0.22.3] - 2026-04-08

### Bug Fixes

- **build**: 同步 Makefile Go 镜像版本至 1.26.2
- Makefile BUILD_IMAGE 从 golang:1.26.1 升级至 golang:1.26.2 - 此为 Docker 构建失败的根因：Makefile 通过 --build-arg 覆盖了 Dockerfile 的默认值

## [0.22.2] - 2026-04-08

### Bug Fixes

- **docker**: 同步 Dockerfile Go 版本至 1.26.2
- BUILD_IMAGE 从 golang:1.26.1 升级至 golang:1.26.2 - 与 go.mod 版本保持一致，修复 Docker 构建失败

### CI/CD

- Bump actions/upload-artifact from 6 to 7 ([#144](https://github.com/sunerpy/pt-tools/issues/144)) ([#144](https://github.com/sunerpy/pt-tools/pull/144))
  Bumps [actions/upload-artifact](https://github.com/actions/upload-artifact) from 6 to 7. - [Release notes](https://github.com/actions/upload-artifact/releases) - [Commits](https://github.com/actions/upload-artifact/compare/v6...v7)

        ---
        updated-dependencies:
        - dependency-name: actions/upload-artifact
         dependency-version: '7'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

- Bump actions/github-script from 7 to 8 ([#142](https://github.com/sunerpy/pt-tools/issues/142)) ([#142](https://github.com/sunerpy/pt-tools/pull/142))
  Bumps [actions/github-script](https://github.com/actions/github-script) from 7 to 8. - [Release notes](https://github.com/actions/github-script/releases) - [Commits](https://github.com/actions/github-script/compare/v7...v8)

        ---
        updated-dependencies:
        - dependency-name: actions/github-script
         dependency-version: '8'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

## [0.22.1] - 2026-04-08

### Bug Fixes

- **deps**: 升级 Go 至 1.26.2 修复 crypto/x509 漏洞 (GO-2026-4947)
- go.mod 升级 1.26.1 → 1.26.2 - 恢复 ci.yml 中 govulncheck 的阻断逻辑

### CI/CD

- Bump actions/download-artifact from 7 to 8
  Bumps [actions/download-artifact](https://github.com/actions/download-artifact) from 7 to 8. - [Release notes](https://github.com/actions/download-artifact/releases) - [Commits](https://github.com/actions/download-artifact/compare/v7...v8)

        ---
        updated-dependencies:
        - dependency-name: actions/download-artifact
         dependency-version: '8'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

- **security**: Govulncheck 使用 stable 版本 Go 运行
- go-security job 从 go-version-file 切换为 go-version: stable - 确保 govulncheck 始终使用最新 patch 版修复标准库漏洞
- **security**: 将 govulncheck 降级为非阻断警告
- 标准库 crypto/x509 漏洞 (GO-2026-4947) 需等待 Go 1.26.2 修复 - govulncheck 设为 continue-on-error 避免阻断 CI - ci-success 中 go-security 失败改为 warning 而非 hard fail

### Dependencies (Frontend)

- **pnpm**: Bump vite from 7.3.1 to 8.0.5 in /web/frontend
  Bumps [vite](https://github.com/vitejs/vite/tree/HEAD/packages/vite) from 7.3.1 to 8.0.5. - [Release notes](https://github.com/vitejs/vite/releases) - [Changelog](https://github.com/vitejs/vite/blob/main/packages/vite/CHANGELOG.md) - [Commits](https://github.com/vitejs/vite/commits/v8.0.5/packages/vite)

        ---
        updated-dependencies:
        - dependency-name: vite
         dependency-version: 8.0.5
         dependency-type: direct:development
         update-type: version-update:semver-major
        ...

- **pnpm**: 升级 typescript 至 6.0.2 并适配废弃选项
- 升级 typescript 5.9.3 → 6.0.2 - tsconfig.app.json 增加 ignoreDeprecations: "6.0" 适配 baseUrl 废弃警告

## [0.22.0] - 2026-04-07

### CI/CD

- **workflow**: 增加新站点请求缺失附件自动关闭工作流
- 每日扫描未上传 ZIP 附件的站点请求 Issue - 提醒超过 14 天未补充附件时发送 7 天倒计时警告 - 警告后仍无附件则自动关闭并标记 stale-closed

### Dependencies (Frontend)

- **pnpm**: Bump @vitejs/plugin-vue from 6.0.4 to 6.0.5 in /web/frontend ([#185](https://github.com/sunerpy/pt-tools/issues/185)) ([#185](https://github.com/sunerpy/pt-tools/pull/185))
  Bumps [@vitejs/plugin-vue](https://github.com/vitejs/vite-plugin-vue/tree/HEAD/packages/plugin-vue) from 6.0.4 to 6.0.5. - [Release notes](https://github.com/vitejs/vite-plugin-vue/releases) - [Changelog](https://github.com/vitejs/vite-plugin-vue/blob/main/packages/plugin-vue/CHANGELOG.md) - [Commits](https://github.com/vitejs/vite-plugin-vue/commits/plugin-vue@6.0.5/packages/plugin-vue)

        ---
        updated-dependencies:
        - dependency-name: "@vitejs/plugin-vue"
         dependency-version: 6.0.5
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump oxfmt from 0.36.0 to 0.41.0 in /web/frontend ([#188](https://github.com/sunerpy/pt-tools/issues/188)) ([#188](https://github.com/sunerpy/pt-tools/pull/188))
  Bumps [oxfmt](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxfmt) from 0.36.0 to 0.41.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxfmt/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxfmt_v0.41.0/npm/oxfmt)

        ---
        updated-dependencies:
        - dependency-name: oxfmt
         dependency-version: 0.41.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump dompurify from 3.3.2 to 3.3.3 in /web/frontend ([#189](https://github.com/sunerpy/pt-tools/issues/189)) ([#189](https://github.com/sunerpy/pt-tools/pull/189))
  Bumps [dompurify](https://github.com/cure53/DOMPurify) from 3.3.2 to 3.3.3. - [Release notes](https://github.com/cure53/DOMPurify/releases) - [Commits](https://github.com/cure53/DOMPurify/compare/3.3.2...3.3.3)

        ---
        updated-dependencies:
        - dependency-name: dompurify
         dependency-version: 3.3.3
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump @types/node from 25.4.0 to 25.5.0 in /web/frontend ([#187](https://github.com/sunerpy/pt-tools/issues/187)) ([#187](https://github.com/sunerpy/pt-tools/pull/187))
  Bumps [@types/node](https://github.com/DefinitelyTyped/DefinitelyTyped/tree/HEAD/types/node) from 25.4.0 to 25.5.0. - [Release notes](https://github.com/DefinitelyTyped/DefinitelyTyped/releases) - [Commits](https://github.com/DefinitelyTyped/DefinitelyTyped/commits/HEAD/types/node)

        ---
        updated-dependencies:
        - dependency-name: "@types/node"
         dependency-version: 25.5.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump vitest from 4.0.18 to 4.1.0 in /web/frontend ([#190](https://github.com/sunerpy/pt-tools/issues/190)) ([#190](https://github.com/sunerpy/pt-tools/pull/190))
  Bumps [vitest](https://github.com/vitest-dev/vitest/tree/HEAD/packages/vitest) from 4.0.18 to 4.1.0. - [Release notes](https://github.com/vitest-dev/vitest/releases) - [Commits](https://github.com/vitest-dev/vitest/commits/v4.1.0/packages/vitest)

        ---
        updated-dependencies:
        - dependency-name: vitest
         dependency-version: 4.1.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump sass from 1.97.3 to 1.98.0 in /web/frontend ([#191](https://github.com/sunerpy/pt-tools/issues/191)) ([#191](https://github.com/sunerpy/pt-tools/pull/191))
  Bumps [sass](https://github.com/sass/dart-sass) from 1.97.3 to 1.98.0. - [Release notes](https://github.com/sass/dart-sass/releases) - [Changelog](https://github.com/sass/dart-sass/blob/main/CHANGELOG.md) - [Commits](https://github.com/sass/dart-sass/compare/1.97.3...1.98.0)

        ---
        updated-dependencies:
        - dependency-name: sass
         dependency-version: 1.98.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump oxlint from 1.51.0 to 1.56.0 in /web/frontend ([#192](https://github.com/sunerpy/pt-tools/issues/192)) ([#192](https://github.com/sunerpy/pt-tools/pull/192))
  Bumps [oxlint](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxlint) from 1.51.0 to 1.56.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxlint/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxlint_v1.56.0/npm/oxlint)

        ---
        updated-dependencies:
        - dependency-name: oxlint
         dependency-version: 1.56.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump element-plus from 2.13.5 to 2.13.6 in /web/frontend ([#198](https://github.com/sunerpy/pt-tools/issues/198)) ([#198](https://github.com/sunerpy/pt-tools/pull/198))
  Bumps [element-plus](https://github.com/element-plus/element-plus) from 2.13.5 to 2.13.6. - [Release notes](https://github.com/element-plus/element-plus/releases) - [Changelog](https://github.com/element-plus/element-plus/blob/dev/CHANGELOG.en-US.md) - [Commits](https://github.com/element-plus/element-plus/compare/2.13.5...2.13.6)

        ---
        updated-dependencies:
        - dependency-name: element-plus
         dependency-version: 2.13.6
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump vitest from 4.1.0 to 4.1.1 in /web/frontend ([#200](https://github.com/sunerpy/pt-tools/issues/200)) ([#200](https://github.com/sunerpy/pt-tools/pull/200))
  Bumps [vitest](https://github.com/vitest-dev/vitest/tree/HEAD/packages/vitest) from 4.1.0 to 4.1.1. - [Release notes](https://github.com/vitest-dev/vitest/releases) - [Commits](https://github.com/vitest-dev/vitest/commits/v4.1.1/packages/vitest)

        ---
        updated-dependencies:
        - dependency-name: vitest
         dependency-version: 4.1.1
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump vue-router from 5.0.3 to 5.0.4 in /web/frontend ([#201](https://github.com/sunerpy/pt-tools/issues/201)) ([#201](https://github.com/sunerpy/pt-tools/pull/201))
  Bumps [vue-router](https://github.com/vuejs/router) from 5.0.3 to 5.0.4. - [Release notes](https://github.com/vuejs/router/releases) - [Commits](https://github.com/vuejs/router/compare/v5.0.3...v5.0.4)

        ---
        updated-dependencies:
        - dependency-name: vue-router
         dependency-version: 5.0.4
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump marked from 17.0.4 to 17.0.5 in /web/frontend ([#202](https://github.com/sunerpy/pt-tools/issues/202)) ([#202](https://github.com/sunerpy/pt-tools/pull/202))
  Bumps [marked](https://github.com/markedjs/marked) from 17.0.4 to 17.0.5. - [Release notes](https://github.com/markedjs/marked/releases) - [Commits](https://github.com/markedjs/marked/compare/v17.0.4...v17.0.5)

        ---
        updated-dependencies:
        - dependency-name: marked
         dependency-version: 17.0.5
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump vue-tsc from 3.2.5 to 3.2.6 in /web/frontend ([#203](https://github.com/sunerpy/pt-tools/issues/203)) ([#203](https://github.com/sunerpy/pt-tools/pull/203))
  Bumps [vue-tsc](https://github.com/vuejs/language-tools/tree/HEAD/packages/tsc) from 3.2.5 to 3.2.6. - [Release notes](https://github.com/vuejs/language-tools/releases) - [Changelog](https://github.com/vuejs/language-tools/blob/master/CHANGELOG.md) - [Commits](https://github.com/vuejs/language-tools/commits/v3.2.6/packages/tsc)

        ---
        updated-dependencies:
        - dependency-name: vue-tsc
         dependency-version: 3.2.6
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump oxlint from 1.56.0 to 1.57.0 in /web/frontend ([#208](https://github.com/sunerpy/pt-tools/issues/208)) ([#208](https://github.com/sunerpy/pt-tools/pull/208))
  Bumps [oxlint](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxlint) from 1.56.0 to 1.57.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxlint/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxlint_v1.57.0/npm/oxlint)

        ---
        updated-dependencies:
        - dependency-name: oxlint
         dependency-version: 1.57.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump @vue/tsconfig from 0.9.0 to 0.9.1 in /web/frontend ([#213](https://github.com/sunerpy/pt-tools/issues/213)) ([#213](https://github.com/sunerpy/pt-tools/pull/213))
  Bumps [@vue/tsconfig](https://github.com/vuejs/tsconfig) from 0.9.0 to 0.9.1. - [Release notes](https://github.com/vuejs/tsconfig/releases) - [Commits](https://github.com/vuejs/tsconfig/compare/v0.9.0...v0.9.1)

        ---
        updated-dependencies:
        - dependency-name: "@vue/tsconfig"
         dependency-version: 0.9.1
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump oxfmt from 0.41.0 to 0.42.0 in /web/frontend ([#209](https://github.com/sunerpy/pt-tools/issues/209)) ([#209](https://github.com/sunerpy/pt-tools/pull/209))
  Bumps [oxfmt](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxfmt) from 0.41.0 to 0.42.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxfmt/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxfmt_v0.42.0/npm/oxfmt)

        ---
        updated-dependencies:
        - dependency-name: oxfmt
         dependency-version: 0.42.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump vue from 3.5.30 to 3.5.31 in /web/frontend ([#211](https://github.com/sunerpy/pt-tools/issues/211)) ([#211](https://github.com/sunerpy/pt-tools/pull/211))
  Bumps [vue](https://github.com/vuejs/core) from 3.5.30 to 3.5.31. - [Release notes](https://github.com/vuejs/core/releases) - [Changelog](https://github.com/vuejs/core/blob/main/CHANGELOG.md) - [Commits](https://github.com/vuejs/core/compare/v3.5.30...v3.5.31)

        ---
        updated-dependencies:
        - dependency-name: vue
         dependency-version: 3.5.31
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump vitest from 4.1.1 to 4.1.2 in /web/frontend ([#212](https://github.com/sunerpy/pt-tools/issues/212)) ([#212](https://github.com/sunerpy/pt-tools/pull/212))
  Bumps [vitest](https://github.com/vitest-dev/vitest/tree/HEAD/packages/vitest) from 4.1.1 to 4.1.2. - [Release notes](https://github.com/vitest-dev/vitest/releases) - [Commits](https://github.com/vitest-dev/vitest/commits/v4.1.2/packages/vitest)

        ---
        updated-dependencies:
        - dependency-name: vitest
         dependency-version: 4.1.2
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump sass from 1.98.0 to 1.99.0 in /web/frontend ([#217](https://github.com/sunerpy/pt-tools/issues/217)) ([#217](https://github.com/sunerpy/pt-tools/pull/217))
  Bumps [sass](https://github.com/sass/dart-sass) from 1.98.0 to 1.99.0. - [Release notes](https://github.com/sass/dart-sass/releases) - [Changelog](https://github.com/sass/dart-sass/blob/main/CHANGELOG.md) - [Commits](https://github.com/sass/dart-sass/compare/1.98.0...1.99.0)

        ---
        updated-dependencies:
        - dependency-name: sass
         dependency-version: 1.99.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump marked from 17.0.5 to 17.0.6 in /web/frontend ([#218](https://github.com/sunerpy/pt-tools/issues/218)) ([#218](https://github.com/sunerpy/pt-tools/pull/218))
  Bumps [marked](https://github.com/markedjs/marked) from 17.0.5 to 17.0.6. - [Release notes](https://github.com/markedjs/marked/releases) - [Commits](https://github.com/markedjs/marked/compare/v17.0.5...v17.0.6)

        ---
        updated-dependencies:
        - dependency-name: marked
         dependency-version: 17.0.6
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump oxfmt from 0.42.0 to 0.43.0 in /web/frontend ([#221](https://github.com/sunerpy/pt-tools/issues/221)) ([#221](https://github.com/sunerpy/pt-tools/pull/221))
  Bumps [oxfmt](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxfmt) from 0.42.0 to 0.43.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxfmt/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxfmt_v0.43.0/npm/oxfmt)

        ---
        updated-dependencies:
        - dependency-name: oxfmt
         dependency-version: 0.43.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump oxlint from 1.57.0 to 1.58.0 in /web/frontend ([#219](https://github.com/sunerpy/pt-tools/issues/219)) ([#219](https://github.com/sunerpy/pt-tools/pull/219))
  Bumps [oxlint](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxlint) from 1.57.0 to 1.58.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxlint/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxlint_v1.58.0/npm/oxlint)

        ---
        updated-dependencies:
        - dependency-name: oxlint
         dependency-version: 1.58.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump @types/node from 25.5.0 to 25.5.2 in /web/frontend ([#220](https://github.com/sunerpy/pt-tools/issues/220)) ([#220](https://github.com/sunerpy/pt-tools/pull/220))
  Bumps [@types/node](https://github.com/DefinitelyTyped/DefinitelyTyped/tree/HEAD/types/node) from 25.5.0 to 25.5.2. - [Release notes](https://github.com/DefinitelyTyped/DefinitelyTyped/releases) - [Commits](https://github.com/DefinitelyTyped/DefinitelyTyped/commits/HEAD/types/node)

        ---
        updated-dependencies:
        - dependency-name: "@types/node"
         dependency-version: 25.5.2
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

### Dependencies (Go)

- **go**: Bump golang.org/x/text from 0.34.0 to 0.35.0 ([#183](https://github.com/sunerpy/pt-tools/issues/183)) ([#183](https://github.com/sunerpy/pt-tools/pull/183))
  Bumps [golang.org/x/text](https://github.com/golang/text) from 0.34.0 to 0.35.0. - [Release notes](https://github.com/golang/text/releases) - [Commits](https://github.com/golang/text/compare/v0.34.0...v0.35.0)

        ---
        updated-dependencies:
        - dependency-name: golang.org/x/text
         dependency-version: 0.35.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

- **go**: Bump github.com/fatih/color from 1.18.0 to 1.19.0 ([#197](https://github.com/sunerpy/pt-tools/issues/197)) ([#197](https://github.com/sunerpy/pt-tools/pull/197))
  Bumps [github.com/fatih/color](https://github.com/fatih/color) from 1.18.0 to 1.19.0. - [Release notes](https://github.com/fatih/color/releases) - [Commits](https://github.com/fatih/color/compare/v1.18.0...v1.19.0)

        ---
        updated-dependencies:
        - dependency-name: github.com/fatih/color
         dependency-version: 1.19.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

- **go**: Bump github.com/PuerkitoBio/goquery from 1.11.0 to 1.12.0 ([#184](https://github.com/sunerpy/pt-tools/issues/184)) ([#184](https://github.com/sunerpy/pt-tools/pull/184))
  Bumps [github.com/PuerkitoBio/goquery](https://github.com/PuerkitoBio/goquery) from 1.11.0 to 1.12.0. - [Release notes](https://github.com/PuerkitoBio/goquery/releases) - [Commits](https://github.com/PuerkitoBio/goquery/compare/v1.11.0...v1.12.0)

        ---
        updated-dependencies:
        - dependency-name: github.com/PuerkitoBio/goquery
         dependency-version: 1.12.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

### Features

- **site**: 新增 13 个站点定义及 fixture 测试
- audiences.me ([#164](https://github.com/sunerpy/pt-tools/issues/164)), byr.pt ([#196](https://github.com/sunerpy/pt-tools/issues/196)), carpt.net ([#214](https://github.com/sunerpy/pt-tools/issues/214)) - cc.mypt.cc ([#162](https://github.com/sunerpy/pt-tools/issues/162)), hdhome.org ([#175](https://github.com/sunerpy/pt-tools/issues/175)), hdtime.org ([#205](https://github.com/sunerpy/pt-tools/issues/205)) - hxpt.org ([#206](https://github.com/sunerpy/pt-tools/issues/206)), lemonhd.net ([#195](https://github.com/sunerpy/pt-tools/issues/195)), ptlover.cc ([#160](https://github.com/sunerpy/pt-tools/issues/160)) - ptskit.org ([#207](https://github.com/sunerpy/pt-tools/issues/207)), raingfh.top ([#128](https://github.com/sunerpy/pt-tools/issues/128)), tmpt.top ([#182](https://github.com/sunerpy/pt-tools/issues/182)) - ubits.club ([#193](https://github.com/sunerpy/pt-tools/issues/193)) - 全部基于用户提交的 ZIP 采集包分析 HTML 结构实现 - 每个站点包含完整的 Search/Detail/UserInfo fixture 测试
- **extension**: 同步 13 个新增站点到浏览器扩展 KNOWN_SITES

## [0.21.0] - 2026-03-13

### Features

- **rss**: 增加非免费跳过种子的定时重检与手动清理功能
- 跳过的非免费种子在 6 小时后允许重新检测免费状态 - 仅对当前 RSS 中仍存在的种子生效，不产生额外请求 - 新增 POST /api/tasks/batch-delete 接口用于批量删除未推送记录 - 任务列表页面增加多选框和批量删除按钮，已推送记录不可选中 - 新增重检逻辑和批量删除接口的单元测试

## [0.20.1] - 2026-03-12

### Bug Fixes

- 修复 mTorrent 单种置顶全站免费判断问题
- MTorrent 全站免费优先级提高
- 修复 mallSingleFree 活动时间判断运算符优先级问题
  || 和 && 混用缺少括号导致条件被解析为 A || (B && C) || D，
  活动未开始时只要 now < endDate 即被标记为免费，添加括号与 promotion 判断保持一致。

### Build

- **go**: Bump Go version from 1.26.0 to 1.26.1

### Dependencies (Frontend)

- **pnpm**: Bump marked from 17.0.3 to 17.0.4 in /web/frontend ([#172](https://github.com/sunerpy/pt-tools/issues/172)) ([#172](https://github.com/sunerpy/pt-tools/pull/172))
  Bumps [marked](https://github.com/markedjs/marked) from 17.0.3 to 17.0.4. - [Release notes](https://github.com/markedjs/marked/releases) - [Commits](https://github.com/markedjs/marked/compare/v17.0.3...v17.0.4)

        ---
        updated-dependencies:
        - dependency-name: marked
         dependency-version: 17.0.4
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump dompurify from 3.3.1 to 3.3.2 in /web/frontend ([#173](https://github.com/sunerpy/pt-tools/issues/173)) ([#173](https://github.com/sunerpy/pt-tools/pull/173))
  Bumps [dompurify](https://github.com/cure53/DOMPurify) from 3.3.1 to 3.3.2. - [Release notes](https://github.com/cure53/DOMPurify/releases) - [Commits](https://github.com/cure53/DOMPurify/compare/3.3.1...3.3.2)

        ---
        updated-dependencies:
        - dependency-name: dompurify
         dependency-version: 3.3.2
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump @types/node from 25.3.3 to 25.4.0 in /web/frontend ([#174](https://github.com/sunerpy/pt-tools/issues/174)) ([#174](https://github.com/sunerpy/pt-tools/pull/174))
  Bumps [@types/node](https://github.com/DefinitelyTyped/DefinitelyTyped/tree/HEAD/types/node) from 25.3.3 to 25.4.0. - [Release notes](https://github.com/DefinitelyTyped/DefinitelyTyped/releases) - [Commits](https://github.com/DefinitelyTyped/DefinitelyTyped/commits/HEAD/types/node)

        ---
        updated-dependencies:
        - dependency-name: "@types/node"
         dependency-version: 25.4.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump element-plus from 2.13.3 to 2.13.5 in /web/frontend ([#171](https://github.com/sunerpy/pt-tools/issues/171)) ([#171](https://github.com/sunerpy/pt-tools/pull/171))
  Bumps [element-plus](https://github.com/element-plus/element-plus) from 2.13.3 to 2.13.5. - [Release notes](https://github.com/element-plus/element-plus/releases) - [Changelog](https://github.com/element-plus/element-plus/blob/dev/CHANGELOG.en-US.md) - [Commits](https://github.com/element-plus/element-plus/compare/2.13.3...2.13.5)

        ---
        updated-dependencies:
        - dependency-name: element-plus
         dependency-version: 2.13.5
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump vue from 3.5.28 to 3.5.30 in /web/frontend ([#169](https://github.com/sunerpy/pt-tools/issues/169)) ([#169](https://github.com/sunerpy/pt-tools/pull/169))
  Bumps [vue](https://github.com/vuejs/core) from 3.5.28 to 3.5.30. - [Release notes](https://github.com/vuejs/core/releases) - [Changelog](https://github.com/vuejs/core/blob/main/CHANGELOG.md) - [Commits](https://github.com/vuejs/core/compare/v3.5.28...v3.5.30)

        ---
        updated-dependencies:
        - dependency-name: vue
         dependency-version: 3.5.30
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

### Dependencies (Go)

- **go**: Bump golang.org/x/sys from 0.41.0 to 0.42.0 ([#166](https://github.com/sunerpy/pt-tools/issues/166)) ([#166](https://github.com/sunerpy/pt-tools/pull/166))
  Bumps [golang.org/x/sys](https://github.com/golang/sys) from 0.41.0 to 0.42.0. - [Commits](https://github.com/golang/sys/compare/v0.41.0...v0.42.0)

        ---
        updated-dependencies:
        - dependency-name: golang.org/x/sys
         dependency-version: 0.42.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

- **go**: Bump golang.org/x/time from 0.14.0 to 0.15.0 ([#170](https://github.com/sunerpy/pt-tools/issues/170)) ([#170](https://github.com/sunerpy/pt-tools/pull/170))
  Bumps [golang.org/x/time](https://github.com/golang/time) from 0.14.0 to 0.15.0. - [Commits](https://github.com/golang/time/compare/v0.14.0...v0.15.0)

        ---
        updated-dependencies:
        - dependency-name: golang.org/x/time
         dependency-version: 0.15.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

- **go**: Bump golang.org/x/sync from 0.19.0 to 0.20.0 ([#168](https://github.com/sunerpy/pt-tools/issues/168)) ([#168](https://github.com/sunerpy/pt-tools/pull/168))
  Bumps [golang.org/x/sync](https://github.com/golang/sync) from 0.19.0 to 0.20.0. - [Commits](https://github.com/golang/sync/compare/v0.19.0...v0.20.0)

        ---
        updated-dependencies:
        - dependency-name: golang.org/x/sync
         dependency-version: 0.20.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

### Testing

- 补充 mallSingleFree 折扣分支用例

## [0.20.0] - 2026-03-05

### Bug Fixes

- **extension**: 同步新增站点到浏览器扩展 KNOWN_SITES

### Features

- **site**: 增加多个站点定义并优化现有站点数据抓取
- 新增 BTSchool、52PT、HDFans、垃圾堆、1PTBar、SoulVoice 六个站点定义及 fixture 测试 - 优化 AGSVPT 和 XingYunGe 上传量/下载量/分享率抓取，改用 regex 从合并单元格提取 - 放宽站点 ID 格式限制，允许数字开头以支持 52pt、1ptba 等站点 - 新增 real HTML 验证测试框架用于真实页面选择器校验

## [0.19.2] - 2026-03-05

### Bug Fixes

- **site**: 修复新站点详情抓取与跨站点推送问题

### Dependencies (Frontend)

- **pnpm**: Bump @vue/tsconfig from 0.8.1 to 0.9.0 in /web/frontend ([#146](https://github.com/sunerpy/pt-tools/issues/146)) ([#146](https://github.com/sunerpy/pt-tools/pull/146))
  Bumps [@vue/tsconfig](https://github.com/vuejs/tsconfig) from 0.8.1 to 0.9.0. - [Release notes](https://github.com/vuejs/tsconfig/releases) - [Commits](https://github.com/vuejs/tsconfig/compare/v0.8.1...v0.9.0)

        ---
        updated-dependencies:
        - dependency-name: "@vue/tsconfig"
         dependency-version: 0.9.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump oxlint from 1.50.0 to 1.51.0 in /web/frontend ([#145](https://github.com/sunerpy/pt-tools/issues/145)) ([#145](https://github.com/sunerpy/pt-tools/pull/145))
  Bumps [oxlint](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxlint) from 1.50.0 to 1.51.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxlint/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxlint_v1.51.0/npm/oxlint)

        ---
        updated-dependencies:
        - dependency-name: oxlint
         dependency-version: 1.51.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump element-plus from 2.13.2 to 2.13.3 in /web/frontend ([#150](https://github.com/sunerpy/pt-tools/issues/150)) ([#150](https://github.com/sunerpy/pt-tools/pull/150))
  Bumps [element-plus](https://github.com/element-plus/element-plus) from 2.13.2 to 2.13.3. - [Release notes](https://github.com/element-plus/element-plus/releases) - [Changelog](https://github.com/element-plus/element-plus/blob/dev/CHANGELOG.en-US.md) - [Commits](https://github.com/element-plus/element-plus/compare/2.13.2...2.13.3)

        ---
        updated-dependencies:
        - dependency-name: element-plus
         dependency-version: 2.13.3
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump oxfmt from 0.35.0 to 0.36.0 in /web/frontend ([#149](https://github.com/sunerpy/pt-tools/issues/149)) ([#149](https://github.com/sunerpy/pt-tools/pull/149))
  Bumps [oxfmt](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxfmt) from 0.35.0 to 0.36.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxfmt/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxfmt_v0.36.0/npm/oxfmt)

        ---
        updated-dependencies:
        - dependency-name: oxfmt
         dependency-version: 0.36.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump @types/node from 25.3.0 to 25.3.3 in /web/frontend ([#148](https://github.com/sunerpy/pt-tools/issues/148)) ([#148](https://github.com/sunerpy/pt-tools/pull/148))
  Bumps [@types/node](https://github.com/DefinitelyTyped/DefinitelyTyped/tree/HEAD/types/node) from 25.3.0 to 25.3.3. - [Release notes](https://github.com/DefinitelyTyped/DefinitelyTyped/releases) - [Commits](https://github.com/DefinitelyTyped/DefinitelyTyped/commits/HEAD/types/node)

        ---
        updated-dependencies:
        - dependency-name: "@types/node"
         dependency-version: 25.3.3
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

### Documentation

- **extension**: Update install guide with Edge Add-ons store link
- **extension**: Fix Edge Add-ons store URL with extension ID

## [0.19.1] - 2026-03-02

### Bug Fixes

- **extension**: 优化 Issue 创建流程增加 ZIP 上传醒目提示

## [0.19.0] - 2026-03-02

### Bug Fixes

- **extension**: Add AGSVPT, XingYunGe, MooKo to KNOWN_SITES

### Features

- **site**: 新增 AGSVPT、XingYunGe、MooKo 站点适配及 HR 规则引擎
- 新增 AGSVPT (NexusPHP) 站点定义及 fixture 测试 - 新增 XingYunGe (NexusPHP) 站点定义及 fixture 测试 - 新增 MooKo (Gazelle) 站点定义及 fixture 测试 - SiteDefinition 新增 HRCalcSeedTime 函数字段支持站点自定义 HR 计算逻辑 - 内置 NewSizeTieredHRCalc 工厂函数处理按体积分档的 HR 规则 - CalcHRSeedTimeH 实现三层优先级链: 自定义函数 > 分档规则 > 固定值 - RSS 入库时按种子实际大小计算精确的 HR 做种时间 - cleanup monitor fallback 路径同步使用 CalcHRSeedTimeH 按种子计算

## [0.18.1] - 2026-03-02

### Bug Fixes

- **docker**: 升级构建镜像 Go 版本至 1.26.0 ([#133](https://github.com/sunerpy/pt-tools/pull/133))

## [0.18.0] - 2026-03-02

### Dependencies (Frontend)

- **pnpm**: Bump oxlint from 1.48.0 to 1.50.0 in /web/frontend ([#121](https://github.com/sunerpy/pt-tools/issues/121)) ([#121](https://github.com/sunerpy/pt-tools/pull/121))
  Bumps [oxlint](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxlint) from 1.48.0 to 1.50.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxlint/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxlint_v1.50.0/npm/oxlint)

        ---
        updated-dependencies:
        - dependency-name: oxlint
         dependency-version: 1.50.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump vue-router from 5.0.2 to 5.0.3 in /web/frontend ([#124](https://github.com/sunerpy/pt-tools/issues/124)) ([#124](https://github.com/sunerpy/pt-tools/pull/124))
  Bumps [vue-router](https://github.com/vuejs/router) from 5.0.2 to 5.0.3. - [Release notes](https://github.com/vuejs/router/releases) - [Commits](https://github.com/vuejs/router/compare/v5.0.2...v5.0.3)

        ---
        updated-dependencies:
        - dependency-name: vue-router
         dependency-version: 5.0.3
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump vue-tsc from 3.2.2 to 3.2.5 in /web/frontend ([#125](https://github.com/sunerpy/pt-tools/issues/125)) ([#125](https://github.com/sunerpy/pt-tools/pull/125))
  Bumps [vue-tsc](https://github.com/vuejs/language-tools/tree/HEAD/packages/tsc) from 3.2.2 to 3.2.5. - [Release notes](https://github.com/vuejs/language-tools/releases) - [Changelog](https://github.com/vuejs/language-tools/blob/master/CHANGELOG.md) - [Commits](https://github.com/vuejs/language-tools/commits/v3.2.5/packages/tsc)

        ---
        updated-dependencies:
        - dependency-name: vue-tsc
         dependency-version: 3.2.5
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump @types/node from 25.2.3 to 25.3.0 in /web/frontend ([#126](https://github.com/sunerpy/pt-tools/issues/126)) ([#126](https://github.com/sunerpy/pt-tools/pull/126))
  Bumps [@types/node](https://github.com/DefinitelyTyped/DefinitelyTyped/tree/HEAD/types/node) from 25.2.3 to 25.3.0. - [Release notes](https://github.com/DefinitelyTyped/DefinitelyTyped/releases) - [Commits](https://github.com/DefinitelyTyped/DefinitelyTyped/commits/HEAD/types/node)

        ---
        updated-dependencies:
        - dependency-name: "@types/node"
         dependency-version: 25.3.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump oxfmt from 0.33.0 to 0.35.0 in /web/frontend ([#123](https://github.com/sunerpy/pt-tools/issues/123)) ([#123](https://github.com/sunerpy/pt-tools/pull/123))
  Bumps [oxfmt](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxfmt) from 0.33.0 to 0.35.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxfmt/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxfmt_v0.35.0/npm/oxfmt)

        ---
        updated-dependencies:
        - dependency-name: oxfmt
         dependency-version: 0.35.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump marked from 17.0.2 to 17.0.3 in /web/frontend ([#122](https://github.com/sunerpy/pt-tools/issues/122)) ([#122](https://github.com/sunerpy/pt-tools/pull/122))
  Bumps [marked](https://github.com/markedjs/marked) from 17.0.2 to 17.0.3. - [Release notes](https://github.com/markedjs/marked/releases) - [Commits](https://github.com/markedjs/marked/compare/v17.0.2...v17.0.3)

        ---
        updated-dependencies:
        - dependency-name: marked
         dependency-version: 17.0.3
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

### Features

- **downloader-hub**: 新增混合下载器管理中心 ([#131](https://github.com/sunerpy/pt-tools/pull/131))

* feat(ui): 新增混合下载器 Web UI 页面

      包含侧栏统计、任务列表、虚拟滚动、详情面板、右键菜单、列管理等功能

      * feat(downloader): 新增下载器停止状态支持

      * refactor(internal): 提取磁盘预算计算为独立模块

      * feat(api): 新增混合下载器种子列表与详情接口

      * build(deps): 升级 Go 至 1.26 并更新依赖

      * feat(ui): 注册下载器中心路由并优化前端沉浸模式

      - server.go: 注册 Downloader Hub 全部 API 路由

      - App.vue: 统一引号风格为双引号

      - api/index.ts: 拆分长行提升可读性

      - app-layout.css: 修复多余 box-shadow 语法错误，完善沉浸模式全屏布局

      Ultraworked with [Sisyphus](https://github.com/code-yeongyu/oh-my-opencode)

## [0.17.0] - 2026-02-21

### Features

- **ui**: 站点管理与首页增加浏览器扩展引导提示
- 站点管理页禁用的「新增站点」按钮增加 Popover 悬浮提示和 Alert 横幅 - 首页 Dashboard 顶部增加可关闭的扩展推荐横幅 - 引导用户通过浏览器扩展快速适配新站点，提供下载和文档链接 - 移除不再使用的 addSite 函数避免构建报错

## [0.16.0] - 2026-02-21

### Documentation

- **guide**: 重构新站点请求指南为扩展优先
- 新增方式一：浏览器扩展自动采集（安装、一键采集、导出提交完整步骤）- 原有手动步骤降级为方式二 - 增加两种方式对比表

### Features

- **cleanup**: 支持免费期结束自动删除未完成种子
  新增全局设置「免费结束自动删除」，开启后免费期结束时未下载完成的种子
  将自动从下载器删除（含数据文件），无需手动操作。默认关闭。

        - SettingsGlobal 新增 AutoDeleteOnFreeEnd 字段
        - FreeEndMonitor 新增自动删除分支，仅作用于免费期结束未完成的种子
        - 系统设置页面新增「免费结束管理」区块含开关和警告提示
        - 暂停任务页面新增自动删除快捷开关（含悬浮提示）
        - README 补充功能说明

        Ultraworked with [Sisyphus](https://github.com/code-yeongyu/oh-my-opencode)

### Performance

- **cleanup**: 优化磁盘紧急清理策略
- 紧急清理目标增加缓冲区（阈值 20% 或至少 10GB），避免清理后立即再次触底 - 新增 DiskSpaceLow 事件，推送检测空间不足时通知清理监控立即执行 - CleanupMonitor 订阅事件总线，收到信号后 3 秒去抖再立即清理 - 仅在自动删种启用时才发送磁盘空间不足信号

      Ultraworked with [Sisyphus](https://github.com/code-yeongyu/oh-my-opencode)

## [0.15.0] - 2026-02-19

### Bug Fixes

- **build**: 修复站点一致性检查脚本引号匹配
- check-sites.ts 中扩展站点 ID 提取正则兼容双引号和单引号
- **api**: 修复搜索站点校验在测试环境空指针问题
- getEnabledSiteIDs 增加 store 空值检查避免测试中 panic
- **test**: 适配登录接口 JSON 响应变更
- 登录测试预期状态码从 302 改为 200 以匹配 JSON 请求返回 JSON 响应的行为

### CI/CD

- Bump actions/upload-artifact from 4 to 6 ([#53](https://github.com/sunerpy/pt-tools/issues/53)) ([#53](https://github.com/sunerpy/pt-tools/pull/53))
  Bumps [actions/upload-artifact](https://github.com/actions/upload-artifact) from 4 to 6. - [Release notes](https://github.com/actions/upload-artifact/releases) - [Commits](https://github.com/actions/upload-artifact/compare/v4...v6)

        ---
        updated-dependencies:
        - dependency-name: actions/upload-artifact
         dependency-version: '6'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

### Dependencies (Frontend)

- **pnpm**: Bump vue-router from 4.6.4 to 5.0.2 in /web/frontend ([#57](https://github.com/sunerpy/pt-tools/issues/57)) ([#57](https://github.com/sunerpy/pt-tools/pull/57))
  Bumps [vue-router](https://github.com/vuejs/router) from 4.6.4 to 5.0.2. - [Release notes](https://github.com/vuejs/router/releases) - [Commits](https://github.com/vuejs/router/compare/v4.6.4...v5.0.2)

        ---
        updated-dependencies:
        - dependency-name: vue-router
         dependency-version: 5.0.2
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

### Features

- **extension**: 增加 PT Tools Helper 浏览器扩展及配套设施
- 新增 Chrome/Edge 浏览器扩展 (tools/browser-extension) - 支持 Cookie 自动同步、批量同步、一键采集站点数据 - 内置 337 个 PT 站点域名识别库，支持中英文界面 - 后端新增 PUT /api/sites/{name} 凭据更新和 /api/ping 健康检查 - 后端增加 CORS 支持、JSON 登录响应、搜索前站点启用校验 - 前端搜索前刷新可用站点列表防止搜索禁用站点 - 新增图标生成脚本和站点一致性检查脚本 - 新增扩展构建发布 CI 流程 (ext-v\* tag 触发 Edge Add-ons 发布) - 更新文档：Cookie 配置优先推荐浏览器扩展同步方式

## [0.14.0] - 2026-02-17

### Dependencies (Frontend)

- **pnpm**: Bump @types/node from 25.2.2 to 25.2.3 in /web/frontend ([#100](https://github.com/sunerpy/pt-tools/issues/100)) ([#100](https://github.com/sunerpy/pt-tools/pull/100))
  Bumps [@types/node](https://github.com/DefinitelyTyped/DefinitelyTyped/tree/HEAD/types/node) from 25.2.2 to 25.2.3. - [Release notes](https://github.com/DefinitelyTyped/DefinitelyTyped/releases) - [Commits](https://github.com/DefinitelyTyped/DefinitelyTyped/commits/HEAD/types/node)

        ---
        updated-dependencies:
        - dependency-name: "@types/node"
         dependency-version: 25.2.3
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump @vueuse/core from 14.2.0 to 14.2.1 in /web/frontend ([#101](https://github.com/sunerpy/pt-tools/issues/101)) ([#101](https://github.com/sunerpy/pt-tools/pull/101))
  Bumps [@vueuse/core](https://github.com/vueuse/vueuse/tree/HEAD/packages/core) from 14.2.0 to 14.2.1. - [Release notes](https://github.com/vueuse/vueuse/releases) - [Commits](https://github.com/vueuse/vueuse/commits/v14.2.1/packages/core)

        ---
        updated-dependencies:
        - dependency-name: "@vueuse/core"
         dependency-version: 14.2.1
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump oxlint from 1.43.0 to 1.48.0 in /web/frontend ([#102](https://github.com/sunerpy/pt-tools/issues/102)) ([#102](https://github.com/sunerpy/pt-tools/pull/102))
  Bumps [oxlint](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxlint) from 1.43.0 to 1.48.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxlint/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxlint_v1.48.0/npm/oxlint)

        ---
        updated-dependencies:
        - dependency-name: oxlint
         dependency-version: 1.48.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump marked from 17.0.1 to 17.0.2 in /web/frontend ([#103](https://github.com/sunerpy/pt-tools/issues/103)) ([#103](https://github.com/sunerpy/pt-tools/pull/103))
  Bumps [marked](https://github.com/markedjs/marked) from 17.0.1 to 17.0.2. - [Release notes](https://github.com/markedjs/marked/releases) - [Commits](https://github.com/markedjs/marked/compare/v17.0.1...v17.0.2)

        ---
        updated-dependencies:
        - dependency-name: marked
         dependency-version: 17.0.2
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump oxfmt from 0.28.0 to 0.33.0 in /web/frontend ([#104](https://github.com/sunerpy/pt-tools/issues/104)) ([#104](https://github.com/sunerpy/pt-tools/pull/104))
  Bumps [oxfmt](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxfmt) from 0.28.0 to 0.33.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxfmt/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxfmt_v0.33.0/npm/oxfmt)

        ---
        updated-dependencies:
        - dependency-name: oxfmt
         dependency-version: 0.33.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

### Features

- **cleanup**: 磁盘空间保护与自动删种优化 ([#105](https://github.com/sunerpy/pt-tools/issues/105)) ([#105](https://github.com/sunerpy/pt-tools/pull/105))
- RSS 推送前增加磁盘空间预检查，空间不足时拒绝推送并短路剩余种子 - 手动推送入口同步增加空间预检查 - 修复 SaveGlobalSettings 更新分支丢失 Cleanup/MaxRetry 等字段的问题 - 修复 MaxRetry=0 时所有种子被误判为超过重试次数的问题 - 修复 CanbeFinished 单位换算错误导致免费期判断失效的问题 - 新增最短免费时间阈值(MinFreeMinutes)，跳过免费剩余时间不足的种子 - 自动删种预设方案选择后保留选中状态，页面加载时反向匹配预设 - 自动删种检查增加运行状态日志，缩短启动延迟 - NexusPHP 站点(hdsky/novahd)降低默认请求速率，减少频率限制误判 - CleanupDiskProtect 默认值改为 true - 新增自动删种功能文档，更新配置文档和 FAQ

## [0.13.0] - 2026-02-12

### Dependencies (Frontend)

- **pnpm**: Bump vue from 3.5.27 to 3.5.28 in /web/frontend ([#93](https://github.com/sunerpy/pt-tools/issues/93)) ([#93](https://github.com/sunerpy/pt-tools/pull/93))
  Bumps [vue](https://github.com/vuejs/core) from 3.5.27 to 3.5.28. - [Release notes](https://github.com/vuejs/core/releases) - [Changelog](https://github.com/vuejs/core/blob/main/CHANGELOG.md) - [Commits](https://github.com/vuejs/core/compare/v3.5.27...v3.5.28)

        ---
        updated-dependencies:
        - dependency-name: vue
         dependency-version: 3.5.28
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump @types/node from 25.2.0 to 25.2.2 in /web/frontend ([#96](https://github.com/sunerpy/pt-tools/issues/96)) ([#96](https://github.com/sunerpy/pt-tools/pull/96))
  Bumps [@types/node](https://github.com/DefinitelyTyped/DefinitelyTyped/tree/HEAD/types/node) from 25.2.0 to 25.2.2. - [Release notes](https://github.com/DefinitelyTyped/DefinitelyTyped/releases) - [Commits](https://github.com/DefinitelyTyped/DefinitelyTyped/commits/HEAD/types/node)

        ---
        updated-dependencies:
        - dependency-name: "@types/node"
         dependency-version: 25.2.2
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

### Dependencies (Go)

- **go**: Bump golang.org/x/sys from 0.40.0 to 0.41.0 ([#95](https://github.com/sunerpy/pt-tools/issues/95)) ([#95](https://github.com/sunerpy/pt-tools/pull/95))
  Bumps [golang.org/x/sys](https://github.com/golang/sys) from 0.40.0 to 0.41.0. - [Commits](https://github.com/golang/sys/compare/v0.40.0...v0.41.0)

        ---
        updated-dependencies:
        - dependency-name: golang.org/x/sys
         dependency-version: 0.41.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

- **go**: Bump golang.org/x/text from 0.33.0 to 0.34.0 ([#94](https://github.com/sunerpy/pt-tools/issues/94)) ([#94](https://github.com/sunerpy/pt-tools/pull/94))
  Bumps [golang.org/x/text](https://github.com/golang/text) from 0.33.0 to 0.34.0. - [Release notes](https://github.com/golang/text/releases) - [Commits](https://github.com/golang/text/compare/v0.33.0...v0.34.0)

        ---
        updated-dependencies:
        - dependency-name: golang.org/x/text
         dependency-version: 0.34.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

### Features

- **multi**: 多站点优化与代理支持 ([#97](https://github.com/sunerpy/pt-tools/issues/97)) ([#97](https://github.com/sunerpy/pt-tools/pull/97))
- 增加 ALL_PROXY 支持并统一 HTTP 客户端代理与连接池 - 迁移站点验证器从 net/http 到 requests 库 - 删除无调用者的死代码和冗余依赖 - 新增代理配置文档和使用说明 - 更新站点列表和 docker-compose 配置 - 修复多站点免费解析失败与种子过期误判(HDDolby/RousiPro/SpringSunday) - 修复 RSS 任务统计计数与站点列表 Passkey 缺失 - 美化主题配色系统并重命名高辨识为极光配色

### Miscellaneous

- **config**: 调整 release-please 版本策略
- bump-patch-for-minor-pre-major 改为 false - feat 类型在 v0.x 阶段触发 minor 版本升级

## [0.12.6] - 2026-02-08

### Bug Fixes

- **frontend**: 修复日志页面加载卡顿问题

## [0.12.5] - 2026-02-07

### Bug Fixes

- **release**: 将构建发布流程收敛到 release-please 内部
- 在同一工作流内串联 release-please、build-and-release、update-changelog - 避免跨工作流触发造成重复触发或遗漏 - 确保 release PR 合并后按单链路完成 tag 后构建发布

## [0.12.4] - 2026-02-07

### Bug Fixes

- **release**: 回滚 release-please 标题模式与 component 配置 ([#86](https://github.com/sunerpy/pt-tools/issues/86)) ([#86](https://github.com/sunerpy/pt-tools/pull/86))
- 恢复 pull-request-title-pattern 为 chore: release - 恢复 pull-request-header 为 ## Release - 移除根包 component 配置，回到此前可稳定触发发布的单包模式
- **release**: 拆分 release-please 与 tag 构建流程 ([#88](https://github.com/sunerpy/pt-tools/issues/88)) ([#88](https://github.com/sunerpy/pt-tools/pull/88))
- release-please 工作流仅负责创建 Release PR 与更新 changelog - 新增 release-assets 工作流，仅在 v\* tag 或手动触发时构建并发布资产 - 避免普通 main 提交在未确认 tag 发布前触发 Build and Release
- **release**: 拆分 release-please 与 tag 构建流程 ([#87](https://github.com/sunerpy/pt-tools/pull/87))
- release-please 工作流仅负责创建 Release PR 与更新 changelog - 新增 release-assets 工作流，仅在 v\* tag 或手动触发时构建并发布资产 - 避免普通 main 提交在未确认 tag 发布前触发 Build and Release

## [0.12.3] - 2026-02-07

### Features

- **site**: 增加站点定义 CI 校验体系与 RSS 免费下载说明 ([#84](https://github.com/sunerpy/pt-tools/issues/84)) ([#84](https://github.com/sunerpy/pt-tools/pull/84))
- 新增 SiteDefinition.Validate() 校验方法及完整单元测试 - RegisterSiteDefinition() 增加重复 ID 检测 - 新增 FixtureSuite 框架，全部 6 个内置站点迁移至 fixture 测试 - 清空 legacy 白名单，所有站点通过动态注册表驱动测试 - 更新 docs/development.md 增加测试指南 - README/RSS 指南/过滤规则指南增加警告：默认仅下载免费种子 - 前端 RSS 页面和过滤规则页面增加 warning 级别提醒横幅
- **site**: 增加站点定义 CI 校验体系与 RSS 免费下载说明 ([#85](https://github.com/sunerpy/pt-tools/pull/85))
- 新增 SiteDefinition.Validate() 校验方法及完整单元测试 - RegisterSiteDefinition() 增加重复 ID 检测 - 新增 FixtureSuite 框架，全部 6 个内置站点迁移至 fixture 测试 - 清空 legacy 白名单，所有站点通过动态注册表驱动测试 - 更新 docs/development.md 增加测试指南 - README/RSS 指南/过滤规则指南增加警告：默认仅下载免费种子 - 前端 RSS 页面和过滤规则页面增加 warning 级别提醒横幅

### Miscellaneous

- **ci**: 调整 GitHub Actions 分支触发规则并更新 release-please 配置 ([#83](https://github.com/sunerpy/pt-tools/issues/83)) ([#83](https://github.com/sunerpy/pt-tools/pull/83))

* chore(ci): 调整 GitHub Actions 分支触发规则并更新 release-please 配置

      - 限制 CI 触发分支为 main
      - 更新 release-please 标题模板并指定组件名

      * chore(build): 更新 Go 版本至 1.25.7

      - 同步 Dockerfile 和 Makefile 中的构建镜像版本
      - 更新 go.mod 文件中的 Go 模块版本要求

      * chore(ci): 简化 Go 构建测试工作流并使用 go.mod 指定版本

      - 使用 go.mod 文件指定 Go 版本以确保一致性

      * refactor(site): 抽离时间参数以支持测试断言

      - 新增 parseMTorrentDiscountWithPromotionAt 方法用于注入时间
      - 固定测试时间避免随机性影响断言结果

## [0.12.2] - 2026-02-05

### Bug Fixes

- **rss**: 修复种子大小限制独立于限速开关生效 ([#81](https://github.com/sunerpy/pt-tools/issues/81)) ([#81](https://github.com/sunerpy/pt-tools/pull/81))
- TorrentSizeGB 设置现在即使未启用下载限速也会生效 - 先检查种子大小限制，再检查限速时间
- **rss**: 修复种子大小限制独立于限速开关生效 ([#81](https://github.com/sunerpy/pt-tools/issues/81)) ([#82](https://github.com/sunerpy/pt-tools/pull/82))
- TorrentSizeGB 设置现在即使未启用下载限速也会生效

## [0.12.1] - 2026-02-05

### Bug Fixes

- **rss**: Allow longer intervals and stabilize release-please config ([#78](https://github.com/sunerpy/pt-tools/issues/78)) ([#78](https://github.com/sunerpy/pt-tools/pull/78))
- **release**: Remove manifest schema for release-please ([#79](https://github.com/sunerpy/pt-tools/issues/79)) ([#79](https://github.com/sunerpy/pt-tools/pull/79))
- **release**: Remove manifest schema for release-please ([#80](https://github.com/sunerpy/pt-tools/pull/80))

## [0.12.0] - 2026-02-05

### Features

- **site**: 简化新增站点设计并修复 mTorrent 优惠时间判断
- 移除硬编码的站点常量，改为从 v2 Registry 动态获取 - 新增 APIUrls 字段支持 API 站点 URL 列表轮换 - 修复 mTorrent 活动优惠结束时间判断问题 ([#75](https://github.com/sunerpy/pt-tools/issues/75)) - 更新前端使用 is_builtin 字段替代硬编码站点列表 - 扩展 CI 触发分支包含 dev - 更新 README 添加数据截图分享功能介绍
- **site**: 简化新增站点设计并修复 mTorrent 优惠时间判断 ([#76](https://github.com/sunerpy/pt-tools/pull/76))
- 移除硬编码的站点常量，改为从 v2 Registry 动态获取 - 新增 APIUrls 字段支持 API 站点 URL 列表轮换 - 修复 mTorrent 活动优惠结束时间判断问题 ([#75](https://github.com/sunerpy/pt-tools/issues/75)) - 更新前端使用 is_builtin 字段替代硬编码站点列表 - 扩展 CI 触发分支包含 dev - 更新 README 添加数据截图分享功能介绍
- **site**: 简化新增站点设计并修复 mTorrent 优惠时间判断 ([#77](https://github.com/sunerpy/pt-tools/pull/77))
- 移除硬编码的站点常量，改为从 v2 Registry 动态获取 - 新增 APIUrls 字段支持 API 站点 URL 列表轮换 - 修复 mTorrent 活动优惠结束时间判断问题 ([#75](https://github.com/sunerpy/pt-tools/issues/75)) - 更新前端使用 is_builtin 字段替代硬编码站点列表 - 扩展 CI 触发分支包含 dev - 更新 README 添加数据截图分享功能介绍

## [0.11.0] - 2026-02-05

### CI/CD

- 修复 Release Please 生成文件的格式化问题
- 在 update-changelog job 中格式化所有 Release Please 生成的文件 - 包括 .release-please-manifest.json 和 release-please-config.json - 移除冗余注释
- 修复 Release Please 生成文件的格式化问题 ([#70](https://github.com/sunerpy/pt-tools/pull/70))
- 在 update-changelog job 中格式化所有 Release Please 生成的文件

### Features

- **docker**: 增加 ARM64 架构支持
- **docker**: 增加 ARM64 架构支持 ([#73](https://github.com/sunerpy/pt-tools/pull/73))
- 增加 ARM64 架构支持 (c6d7ad4), closes #72 ([#74](https://github.com/sunerpy/pt-tools/pull/74))

### Miscellaneous

- **main**: Release 0.11.0

## [0.10.2] - 2026-02-04

### Bug Fixes

- 修复下载器地址不通时 web 无法访问的问题 ([#66](https://github.com/sunerpy/pt-tools/issues/66))
- 修复下载器地址不通时 web 无法访问的问题 ([#66](https://github.com/sunerpy/pt-tools/issues/66)) ([#68](https://github.com/sunerpy/pt-tools/pull/68))
- 将下载器健康检查改为 goroutine 异步执行，不阻塞启动 - 健康状态并行加载，互不阻塞

### CI/CD

- 使用 Release Please 自动化版本发布
  替换手动 tag 发布流程为 Release Please 自动化发布:

        - 添加 release-please.yml: 基于 Conventional Commits 自动创建 Release PR
        - 添加 release-please-config.json: 配置版本规则和 changelog 分类
        - 添加 .release-please-manifest.json: 跟踪当前版本 (v0.10.1)
        - 删除 release.yml: 旧的手动 tag 触发发布
        - 删除 changelog.yml: 旧的手动 changelog 更新

### Miscellaneous

- **main**: Release 0.10.2

## [0.10.1] - 2026-02-03

### Bug Fixes

- 修复低版本数据迁移导致的不兼容问题
- 强制同步站点认证方式与默认URL，确保旧数据正确迁移 - 新增 defaultAPIUrlForSite 函数统一设置 MTeam API URL - 支持旧版密码格式兼容（明文/SHA256）自动升级为新格式
- **frontend**: 修复 SiteList 组件 TypeScript 类型错误
  MessageBoxData 类型不能直接解构 value 属性
- 修复低版本数据迁移导致的不兼容问题 ([#64](https://github.com/sunerpy/pt-tools/pull/64))
- 强制同步站点认证方式与默认URL，确保旧数据正确迁移 - 新增 defaultAPIUrlForSite 函数统一设置 MTeam API URL - 支持旧版密码格式自动升级为新格式
- 修复数据库锁、事务超时及 MTeam 促销规则解析问题
- 移除全局信号量，解决前端页面加载慢的问题 - 移除事务中的 HTTP 调用，避免 context deadline exceeded 错误 - RSS 无关联过滤规则时不再匹配全局规则 - MTeam GetTorrentDetail 正确解析 promotionRule 促销折扣

### Dependencies (Frontend)

- **pnpm**: Bump oxfmt from 0.27.0 to 0.28.0 in /web/frontend ([#54](https://github.com/sunerpy/pt-tools/issues/54)) ([#54](https://github.com/sunerpy/pt-tools/pull/54))
  Bumps [oxfmt](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxfmt) from 0.27.0 to 0.28.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxfmt/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxfmt_v0.28.0/npm/oxfmt)

        ---
        updated-dependencies:
        - dependency-name: oxfmt
         dependency-version: 0.28.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump vitest from 4.0.17 to 4.0.18 in /web/frontend ([#55](https://github.com/sunerpy/pt-tools/issues/55)) ([#55](https://github.com/sunerpy/pt-tools/pull/55))
  Bumps [vitest](https://github.com/vitest-dev/vitest/tree/HEAD/packages/vitest) from 4.0.17 to 4.0.18. - [Release notes](https://github.com/vitest-dev/vitest/releases) - [Commits](https://github.com/vitest-dev/vitest/commits/v4.0.18/packages/vitest)

        ---
        updated-dependencies:
        - dependency-name: vitest
         dependency-version: 4.0.18
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump element-plus from 2.13.1 to 2.13.2 in /web/frontend ([#58](https://github.com/sunerpy/pt-tools/issues/58)) ([#58](https://github.com/sunerpy/pt-tools/pull/58))
  Bumps [element-plus](https://github.com/element-plus/element-plus) from 2.13.1 to 2.13.2. - [Release notes](https://github.com/element-plus/element-plus/releases) - [Changelog](https://github.com/element-plus/element-plus/blob/dev/CHANGELOG.en-US.md) - [Commits](https://github.com/element-plus/element-plus/compare/2.13.1...2.13.2)

        ---
        updated-dependencies:
        - dependency-name: element-plus
         dependency-version: 2.13.2
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump @vueuse/core from 14.1.0 to 14.2.0 in /web/frontend ([#60](https://github.com/sunerpy/pt-tools/issues/60)) ([#60](https://github.com/sunerpy/pt-tools/pull/60))
  Bumps [@vueuse/core](https://github.com/vueuse/vueuse/tree/HEAD/packages/core) from 14.1.0 to 14.2.0. - [Release notes](https://github.com/vueuse/vueuse/releases) - [Commits](https://github.com/vueuse/vueuse/commits/v14.2.0/packages/core)

        ---
        updated-dependencies:
        - dependency-name: "@vueuse/core"
         dependency-version: 14.2.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump oxlint from 1.42.0 to 1.43.0 in /web/frontend ([#56](https://github.com/sunerpy/pt-tools/issues/56)) ([#56](https://github.com/sunerpy/pt-tools/pull/56))
  Bumps [oxlint](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxlint) from 1.42.0 to 1.43.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxlint/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxlint_v1.43.0/npm/oxlint)

        ---
        updated-dependencies:
        - dependency-name: oxlint
         dependency-version: 1.43.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump @types/node from 25.0.7 to 25.2.0 in /web/frontend ([#59](https://github.com/sunerpy/pt-tools/issues/59)) ([#59](https://github.com/sunerpy/pt-tools/pull/59))
  Bumps [@types/node](https://github.com/DefinitelyTyped/DefinitelyTyped/tree/HEAD/types/node) from 25.0.7 to 25.2.0. - [Release notes](https://github.com/DefinitelyTyped/DefinitelyTyped/releases) - [Commits](https://github.com/DefinitelyTyped/DefinitelyTyped/commits/HEAD/types/node)

        ---
        updated-dependencies:
        - dependency-name: "@types/node"
         dependency-version: 25.2.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump sass from 1.97.2 to 1.97.3 in /web/frontend ([#62](https://github.com/sunerpy/pt-tools/issues/62)) ([#62](https://github.com/sunerpy/pt-tools/pull/62))
  Bumps [sass](https://github.com/sass/dart-sass) from 1.97.2 to 1.97.3. - [Release notes](https://github.com/sass/dart-sass/releases) - [Changelog](https://github.com/sass/dart-sass/blob/main/CHANGELOG.md) - [Commits](https://github.com/sass/dart-sass/compare/1.97.2...1.97.3)

        ---
        updated-dependencies:
        - dependency-name: sass
         dependency-version: 1.97.3
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump @vitejs/plugin-vue from 6.0.3 to 6.0.4 in /web/frontend ([#61](https://github.com/sunerpy/pt-tools/issues/61)) ([#61](https://github.com/sunerpy/pt-tools/pull/61))
  Bumps [@vitejs/plugin-vue](https://github.com/vitejs/vite-plugin-vue/tree/HEAD/packages/plugin-vue) from 6.0.3 to 6.0.4. - [Release notes](https://github.com/vitejs/vite-plugin-vue/releases) - [Changelog](https://github.com/vitejs/vite-plugin-vue/blob/main/packages/plugin-vue/CHANGELOG.md) - [Commits](https://github.com/vitejs/vite-plugin-vue/commits/plugin-vue@6.0.4/packages/plugin-vue)

        ---
        updated-dependencies:
        - dependency-name: "@vitejs/plugin-vue"
         dependency-version: 6.0.4
         dependency-type: direct:development
         update-type: version-update:semver-patch
        ...

## [0.10.0] - 2026-02-02

### Features

- **export**: 导出图片显示用户等级信息
- Canvas 导出和 HTML 预览均显示站点等级 - 等级显示在用户名右侧，使用紫色标识
- **site**: 新增 RousiPro 站点支持
- 新增 RousiPro (rousipro) 站点支持 - 修复 NovaHD 免费种子检测问题 (Issue #50)
- **site**: 新增 RousiPro 站点支持 ([#52](https://github.com/sunerpy/pt-tools/pull/52))
- 新增 RousiPro (rousipro) 站点支持 - 修复 NovaHD 免费种子检测问题 (Issue #50)

## [0.9.1] - 2026-01-31

### Bug Fixes

- **site**: 修复站点404错误
- 站点验证改为从 Registry 动态获取，支持所有已注册站点
- **site**: 修复站点404错误 ([#49](https://github.com/sunerpy/pt-tools/pull/49))
- 站点验证改为从 Registry 动态获取，支持所有已注册站点

## [0.9.0] - 2026-01-31

### Features

- **site**: 新增 NovaHD 站点支持 + 修复图片分享功能
  NovaHD 站点支持: - 新增 NovaHD 站点定义，包含 9 个等级要求 - 自定义 DetailParser 配置用于解析优惠和结束时间
- **site**: 新增 NovaHD 站点支持 + 修复图片分享功能 ([#48](https://github.com/sunerpy/pt-tools/pull/48))
- 新增 NovaHD 站点定义，包含 9 个等级要求 - 自定义 DetailParser 配置用于解析优惠和结束时间

      - 修复 HTTP 环境下剪贴板 API 不可用导致的错误
      - 优化分享率颜色对比度，在绿色主题下更易辨识
      - 站点卡片显示入站日期和时长

## [0.8.0] - 2026-01-31

### Features

- **site**: HDDolby 两步验证支持 + 解析逻辑优化
  主要变更：- feat(hddolby): 新增 HDDolbyDriver 支持两步验证（Cookie + 详情页解析）- feat(ratelimit): 实现 SQLite 持久化速率限制器，重启后状态不丢失 - refactor(parser): 统一 NexusPHP 详情页解析配置到 SiteDefinition - feat(discount): 搜索结果页支持可配置的 DiscountMapping - docs: 更新开发指南，添加持久化限流和解析配置说明

        技术细节：
        - 新增 models/rate_limit.go (SiteRateLimit 数据模型)
        - 新增 site/v2/persistent_rate_limiter.go (滑动窗口限速器)
        - 新增 site/v2/hddolby_driver.go (HDDolby 专用驱动)
        - 删除冗余的 site/parser/ 和 site/mocks/ 目录
        - SiteDefinition 新增 DetailParser 和 DiscountMapping 配置

- **site**: HDDolby 两步验证支持 + 支持分享站点数据截图 ([#47](https://github.com/sunerpy/pt-tools/pull/47))
- feat(hddolby): 新增 HDDolby 支持两步验证（Cookie + 详情页解析）- feat: 支持用户统计页面导出分享数据截图，支持模糊站点logo、名称、用户名等自定义项 - refactor(parser): 统一 NexusPHP 详情页解析配置到 SiteDefinition - feat(discount): 搜索结果页支持可配置的 DiscountMapping - docs: 更新开发指南，添加持久化限流和解析配置说明

## [0.7.0] - 2026-01-31

### CI/CD

- Add dependabot auto-merge workflow with safety checks
- Add dependabot auto-merge workflow with safety checks ([#45](https://github.com/sunerpy/pt-tools/pull/45))
- Bump actions/setup-go from 5 to 6
  Bumps [actions/setup-go](https://github.com/actions/setup-go) from 5 to 6. - [Release notes](https://github.com/actions/setup-go/releases) - [Commits](https://github.com/actions/setup-go/compare/v5...v6)

        ---
        updated-dependencies:
        - dependency-name: actions/setup-go
         dependency-version: '6'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

- Bump actions/setup-node from 4 to 6
  Bumps [actions/setup-node](https://github.com/actions/setup-node) from 4 to 6. - [Release notes](https://github.com/actions/setup-node/releases) - [Commits](https://github.com/actions/setup-node/compare/v4...v6)

        ---
        updated-dependencies:
        - dependency-name: actions/setup-node
         dependency-version: '6'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

- Bump actions/download-artifact from 4 to 7
  Bumps [actions/download-artifact](https://github.com/actions/download-artifact) from 4 to 7. - [Release notes](https://github.com/actions/download-artifact/releases) - [Commits](https://github.com/actions/download-artifact/compare/v4...v7)

        ---
        updated-dependencies:
        - dependency-name: actions/download-artifact
         dependency-version: '7'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

- Bump actions/cache from 4 to 5
  Bumps [actions/cache](https://github.com/actions/cache) from 4 to 5. - [Release notes](https://github.com/actions/cache/releases) - [Changelog](https://github.com/actions/cache/blob/main/RELEASES.md) - [Commits](https://github.com/actions/cache/compare/v4...v5)

        ---
        updated-dependencies:
        - dependency-name: actions/cache
         dependency-version: '5'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

- Bump actions/checkout from 4 to 6
  Bumps [actions/checkout](https://github.com/actions/checkout) from 4 to 6. - [Release notes](https://github.com/actions/checkout/releases) - [Changelog](https://github.com/actions/checkout/blob/main/CHANGELOG.md) - [Commits](https://github.com/actions/checkout/compare/v4...v6)

        ---
        updated-dependencies:
        - dependency-name: actions/checkout
         dependency-version: '6'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

### Dependencies (Frontend)

- **pnpm**: Bump vue from 3.5.26 to 3.5.27 in /web/frontend
  Bumps [vue](https://github.com/vuejs/core) from 3.5.26 to 3.5.27. - [Release notes](https://github.com/vuejs/core/releases) - [Changelog](https://github.com/vuejs/core/blob/main/CHANGELOG.md) - [Commits](https://github.com/vuejs/core/compare/v3.5.26...v3.5.27)

        ---
        updated-dependencies:
        - dependency-name: vue
         dependency-version: 3.5.27
         dependency-type: direct:production
         update-type: version-update:semver-patch
        ...

- **pnpm**: Bump oxlint from 1.39.0 to 1.42.0 in /web/frontend
  Bumps [oxlint](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxlint) from 1.39.0 to 1.42.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxlint/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxlint_v1.42.0/npm/oxlint)

        ---
        updated-dependencies:
        - dependency-name: oxlint
         dependency-version: 1.42.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump oxfmt from 0.24.0 to 0.27.0 in /web/frontend ([#37](https://github.com/sunerpy/pt-tools/issues/37)) ([#37](https://github.com/sunerpy/pt-tools/pull/37))
  Bumps [oxfmt](https://github.com/oxc-project/oxc/tree/HEAD/npm/oxfmt) from 0.24.0 to 0.27.0. - [Release notes](https://github.com/oxc-project/oxc/releases) - [Changelog](https://github.com/oxc-project/oxc/blob/main/npm/oxfmt/CHANGELOG.md) - [Commits](https://github.com/oxc-project/oxc/commits/oxfmt_v0.27.0/npm/oxfmt)

        ---
        updated-dependencies:
        - dependency-name: oxfmt
         dependency-version: 0.27.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

### Features

- **frontend**: 新增用户数据导出分享功能
- 新增 UserDataExport.vue 组件，支持 Canvas 渲染生成分享图片 - 支持导出总上传/下载、分享率、魔力值、做种数等汇总统计 - 支持展示各站点详情：用户名、上传下载量、魔力值、入站时长 - 隐私保护：支持模糊用户名、站点名和站点图标（马赛克效果）- 6 种预设主题配色 + 自定义颜色选择器 - 支持下载 PNG 图片和复制到剪贴板 - 可自由选择要展示的站点
- **frontend**: 新增用户数据导出分享功能 ([#46](https://github.com/sunerpy/pt-tools/pull/46))
- 新增 UserDataExport.vue 组件，支持 Canvas 渲染生成分享图片 - 支持导出总上传/下载、分享率、魔力值、做种数等汇总统计 - 支持展示各站点详情：用户名、上传下载量、魔力值、入站时长 - 隐私保护：支持模糊用户名、站点名和站点图标（马赛克效果）- 支持下载 PNG 图片和复制到剪贴板 - 可自由选择要展示的站点

## [0.6.0] - 2026-01-30

### Bug Fixes

- Downloader sync bugs - RSS subscription sync + auto-enable on set default
- BatchUpdateSiteDownloader now also updates associated RSS subscriptions' downloader_id - setDefaultDownloader automatically enables the downloader when set as default

      Both fixes include corresponding tests.

### Features

- 站点配置统一化与下载器增强
- 简化新增站点配置，修复未配置qbit下载器的错误，增加tg交流群 ([#42](https://github.com/sunerpy/pt-tools/pull/42))
- 修复未配置qbittorrent下载器时跳过站点的问题 - 统一站点配置源，简化新站点添加流程，新增站点只需创建 definitions/.go 文件 - 前端禁用不可用站点并同步数据库状态 - 新增tg交流群

### Miscellaneous

- **build**: 更新 Go 版本至 1.25.6
- 统一构建环境中的 Go 版本 - 确保与 Docker 构建镜像版本一致

## [0.5.0] - 2026-01-24

### Features

- **web**: 增加版本一键自动升级功能
- 新增运行时环境检测与升级状态接口 - 实现 Web 界面触发的二进制自动升级流程 - 支持下载进度跟踪与取消操作 - 前端集成升级控制与状态展示逻辑

## [0.4.2] - 2026-01-24

### Features

- **downloader**: 增强下载器连接检查与错误提示
- 优化 qBittorrent 和 Transmission 的连接错误处理 - 添加详细的中文错误信息和日志记录 - 前端校验下载器表单必填字段并高亮显示状态

### Miscellaneous

- **build**: 切换 changelog 格式化工具至 oxfmt
- 移除 dprint 相关配置与使用 - 使用 oxfmt 替代 dprint 进行 markdown 格式化

## [0.4.1] - 2026-01-24

### Bug Fixes

- **downloader**: 去除 URL 尾部斜杠并优化下载器检查逻辑
- 为 qBittorrent 和 Transmission 的 GetURL 方法添加去除尾斜杠处理 - 改进 downloaderHealthCheck 接口实现，支持真正的连接测试 - 增强错误提示信息，区分不同类型下载器的健康状态

### Miscellaneous

- **frontend**: 使用oxc oxfmt 和 oxlint 并更新 Makefile
- 更改前端格式化工具为 oxfmt，调整 CI 中的格式检查步骤 - 引入 .oxfmtrc.json 配置文件并移除旧的 dprint 配置 - 更新 tsconfig.json、vite.config.ts 及多个 Vue 文件中的语法（主要是添加分号）- 调整 cliff.toml 以支持提交正文内容显示 - 添加 pre-commit 钩子配置支持 fmt 和 lint 命令
- **build**: 优化 release.yml 中的文件重命名逻辑以支持 Windows 可执行文件
- 区分处理 `.exe` 文件和非 `.exe` 文件 - 确保 Windows 平台下保留可执行文件扩展名 - 统一压缩前的临时目录结构操作
- **build**: 调整二进制打包方式以支持 latest 版本下载
- 移除文件名中的版本标签 - 更新 release workflow 中的下载链接为 latest 地址 - 便于用户通过固定链接获取最新构建产物
- **build**: 增强发布工作流中的标签验证与变量引用安全性
- 添加输入标签格式校验，确保符合语义化版本规范 - 优化构建与打包命令中的变量传递方式

## [0.4.0] - 2026-01-19

### Features

- **version**: 增加版本检查功能支持检测 GitHub 新版本并提供更新提醒
- 新增 version/checker 包实现 GitHub Releases 检查逻辑 - 支持语义化版本解析和比较 - 提供 API 接口 /api/version 和 /api/version/check - 前端集成版本检查组件和状态管理 - 支持通过代理获取更新及版本忽略功能

## [0.3.5] - 2026-01-18

### Bug Fixes

- **site**: 修复 HDDolby 种子选择器并优化时间解析逻辑
- 新增对 HDDolby 站点种子列表各项属性的选择器定义 - 改进 NexusPHP 驱动，支持从 onmouseover 属性中提取折扣结束时间 - 添加针对不同站点的折扣时间解析测试用例

## [0.3.4] - 2026-01-18

### Bug Fixes

- **scheduler**: 修复 Manager 事件监听器导致的数据竞态
- 添加 stopped 标志和 eventCancel 用于优雅关闭事件监听 goroutine - StopAll() 现在会设置 stopped=true 并调用 eventCancel() 终止监听 - 事件监听 goroutine 检查 stopped 标志，防止访问已关闭资源 - rss 命令执行后清理 scheduler Manager - 添加 defer m.StopAll() 确保后台 goroutine 正确退出

### Features

- **scheduler**: 优化免费结束监控器的并发处理逻辑
- 防止独立定时器与周期检查协程重复处理相同任务 - 提升系统在某些场景下的稳定性与数据一致性

### Miscellaneous

- **build**: 调整 Makefile 和格式化配置以支持 dprint 工具
- **build**: 优化 CI 流程
- 更新 README 中的 Go 版本标识 - 调整覆盖率上传 artifact 命名规则 - 优化 CI 成功检查逻辑，明确依赖任务结果判断
- **build**: 更新 golangci-lint 安装方式并升级 pnpm 版本
- 使用 go install 替代 GitHub Action 安装 golangci-lint - 将 pnpm 版本从 9 升级至 10
- **ci**: 增加前端构建任务并优化 CI 流程
- 新增独立的 frontend-build job 处理前端构建和检查 - 前端产物通过 artifact 在各 job 间传递 - 移除原有的 frontend-checks job

### Testing

- **site**: 增加测试中的错误处理
- 在多个测试函数中添加缺失的 return 语句以避免继续执行无效逻辑 - 修正部分测试断言和条件判断顺序，确保测试更稳定可靠

## [0.3.2] - 2026-01-17

### Bug Fixes

- **scheduler**: 修复种子被下载器删除后的状态处理逻辑
- 检测到种子不存在时自动标记任务为完成并清空下载器任务ID - 更新前端任务列表显示“已删除”状态 - 优化日志提示信息，区分不同错误原因

## [0.3.0] - 2026-01-17

### CI/CD

- Bump actions/upload-artifact from 4 to 6
  Bumps [actions/upload-artifact](https://github.com/actions/upload-artifact) from 4 to 6. - [Release notes](https://github.com/actions/upload-artifact/releases) - [Commits](https://github.com/actions/upload-artifact/compare/v4...v6)

        ---
        updated-dependencies:
        - dependency-name: actions/upload-artifact
         dependency-version: '6'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

- Bump actions/setup-node from 4 to 6
  Bumps [actions/setup-node](https://github.com/actions/setup-node) from 4 to 6. - [Release notes](https://github.com/actions/setup-node/releases) - [Commits](https://github.com/actions/setup-node/compare/v4...v6)

        ---
        updated-dependencies:
        - dependency-name: actions/setup-node
         dependency-version: '6'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

- Bump actions/setup-go from 4 to 6
  Bumps [actions/setup-go](https://github.com/actions/setup-go) from 4 to 6. - [Release notes](https://github.com/actions/setup-go/releases) - [Commits](https://github.com/actions/setup-go/compare/v4...v6)

        ---
        updated-dependencies:
        - dependency-name: actions/setup-go
         dependency-version: '6'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

- Bump actions/checkout from 4 to 6
  Bumps [actions/checkout](https://github.com/actions/checkout) from 4 to 6. - [Release notes](https://github.com/actions/checkout/releases) - [Changelog](https://github.com/actions/checkout/blob/main/CHANGELOG.md) - [Commits](https://github.com/actions/checkout/compare/v4...v6)

        ---
        updated-dependencies:
        - dependency-name: actions/checkout
         dependency-version: '6'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

- Bump actions/download-artifact from 4 to 7
  Bumps [actions/download-artifact](https://github.com/actions/download-artifact) from 4 to 7. - [Release notes](https://github.com/actions/download-artifact/releases) - [Commits](https://github.com/actions/download-artifact/compare/v4...v7)

        ---
        updated-dependencies:
        - dependency-name: actions/download-artifact
         dependency-version: '7'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

- Bump docker/setup-buildx-action from 2 to 3
  Bumps [docker/setup-buildx-action](https://github.com/docker/setup-buildx-action) from 2 to 3. - [Release notes](https://github.com/docker/setup-buildx-action/releases) - [Commits](https://github.com/docker/setup-buildx-action/compare/v2...v3)

        ---
        updated-dependencies:
        - dependency-name: docker/setup-buildx-action
         dependency-version: '3'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

- Bump docker/login-action from 2 to 3
  Bumps [docker/login-action](https://github.com/docker/login-action) from 2 to 3. - [Release notes](https://github.com/docker/login-action/releases) - [Commits](https://github.com/docker/login-action/compare/v2...v3)

        ---
        updated-dependencies:
        - dependency-name: docker/login-action
         dependency-version: '3'
         dependency-type: direct:production
         update-type: version-update:semver-major
        ...

### Dependencies (Frontend)

- **pnpm**: Bump globals from 16.5.0 to 17.0.0 in /web/frontend
  Bumps [globals](https://github.com/sindresorhus/globals) from 16.5.0 to 17.0.0. - [Release notes](https://github.com/sindresorhus/globals/releases) - [Commits](https://github.com/sindresorhus/globals/compare/v16.5.0...v17.0.0)

        ---
        updated-dependencies:
        - dependency-name: globals
         dependency-version: 17.0.0
         dependency-type: direct:development
         update-type: version-update:semver-major
        ...

- **pnpm**: Bump @types/node from 24.10.7 to 25.0.7 in /web/frontend
  Bumps [@types/node](https://github.com/DefinitelyTyped/DefinitelyTyped/tree/HEAD/types/node) from 24.10.7 to 25.0.7. - [Release notes](https://github.com/DefinitelyTyped/DefinitelyTyped/releases) - [Commits](https://github.com/DefinitelyTyped/DefinitelyTyped/commits/HEAD/types/node)

        ---
        updated-dependencies:
        - dependency-name: "@types/node"
         dependency-version: 25.0.7
         dependency-type: direct:development
         update-type: version-update:semver-major
        ...

- **pnpm**: Bump @typescript-eslint/parser in /web/frontend
  Bumps [@typescript-eslint/parser](https://github.com/typescript-eslint/typescript-eslint/tree/HEAD/packages/parser) from 8.52.0 to 8.53.0. - [Release notes](https://github.com/typescript-eslint/typescript-eslint/releases) - [Changelog](https://github.com/typescript-eslint/typescript-eslint/blob/main/packages/parser/CHANGELOG.md) - [Commits](https://github.com/typescript-eslint/typescript-eslint/commits/v8.53.0/packages/parser)

        ---
        updated-dependencies:
        - dependency-name: "@typescript-eslint/parser"
         dependency-version: 8.53.0
         dependency-type: direct:development
         update-type: version-update:semver-minor
        ...

- **pnpm**: Bump vitest from 3.2.4 to 4.0.17 in /web/frontend
  Bumps [vitest](https://github.com/vitest-dev/vitest/tree/HEAD/packages/vitest) from 3.2.4 to 4.0.17. - [Release notes](https://github.com/vitest-dev/vitest/releases) - [Commits](https://github.com/vitest-dev/vitest/commits/v4.0.17/packages/vitest)

        ---
        updated-dependencies:
        - dependency-name: vitest
         dependency-version: 4.0.17
         dependency-type: direct:development
         update-type: version-update:semver-major
        ...

### Dependencies (Go)

- **go**: Bump golang.org/x/text from 0.32.0 to 0.33.0
  Bumps [golang.org/x/text](https://github.com/golang/text) from 0.32.0 to 0.33.0. - [Release notes](https://github.com/golang/text/releases) - [Commits](https://github.com/golang/text/compare/v0.32.0...v0.33.0)

        ---
        updated-dependencies:
        - dependency-name: golang.org/x/text
         dependency-version: 0.33.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

- **go**: Bump golang.org/x/text from 0.32.0 to 0.33.0
  Bumps [golang.org/x/text](https://github.com/golang/text) from 0.32.0 to 0.33.0. - [Release notes](https://github.com/golang/text/releases) - [Commits](https://github.com/golang/text/compare/v0.32.0...v0.33.0)

        ---
        updated-dependencies:
        - dependency-name: golang.org/x/text
         dependency-version: 0.33.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

### Features

- **task**: 实现免费种子到期自动暂停功能
- 新增 `DownloaderInfo` 结构体及 `GetDownloaderForRSS` 方法，支持获取下载器状态 - 优化任务监控机制，确保免费时间结束时未完成的任务能自动切换至暂停状态

### Miscellaneous

- **frontend**: 更新 pnpm 锁定文件中的依赖版本和 libc 支持
- **build**: 调整 GitHub Actions 触发条件为仅标签推送时更新 CHANGELOG.md
- 移除对 main 分支的监听限制 - 改为只在 v\* 标签推送时触发工作流

## [0.2.0] - 2026-01-11

### Bug Fixes

- 修复构建错误
- 修复前端模板构建错误
- 修复构建错误

### Dependencies (Go)

- **go**: Bump golang.org/x/sys from 0.39.0 to 0.40.0
  Bumps [golang.org/x/sys](https://github.com/golang/sys) from 0.39.0 to 0.40.0. - [Commits](https://github.com/golang/sys/compare/v0.39.0...v0.40.0)

        ---
        updated-dependencies:
        - dependency-name: golang.org/x/sys
         dependency-version: 0.40.0
         dependency-type: direct:production
         update-type: version-update:semver-minor
        ...

### Features

- 支持多下载器(qbittorrent,transmission),支持规则过滤，多站点搜索与用户信息聚合，统一接口与文档设计
- 引入 UnifiedPTSite 统一接口 - 新增多站点用户信息聚合（上传/下载/做种）与并发搜索（去重、排序）- 新增 Transmission 下载器支持 - Web 端支持动态站点搜索 - 增强配置管理：ConfigStore.SyncSites、RSS 过滤规则、运行时日志级别 - 废弃旧泛型实现，统一错误处理
- 支持多下载器(qbittorrent,transmission),支持规则过滤，多站点搜索与用户信息聚合，统一接口与文档设计 ([#4](https://github.com/sunerpy/pt-tools/pull/4))

## [0.1.6] - 2025-12-25

### Bug Fixes

- **frontend**: 修复 RSS 相关空值问题
- 修复添加 RSS 时可能出现的空数组访问问题 - 改进重复 RSS URL 检查逻辑，避免空值错误

## [0.1.5] - 2025-12-22

### Bug Fixes

- **internal**: 优化种子过期处理逻辑

## [0.1.4] - 2025-12-22

### Bug Fixes

- **api**: 修复页面添加rss订阅时响应较慢和添加失败的问题
- 前端修正 RSS ID 类型从 string 为 number，保持前后端一致 - 增加前端本地去重判断，提升用户体验 - 所有 RSS 相关操作增加详细日志记录

### Features

- **release**: 优化发布流程,优化release页面内容展示
  使用标准化的 changelog 生成与 Docker 镜像推送逻辑，并修复文件打包路径问题。

## [0.1.3] - 2025-12-16

### Bug Fixes

- **internal**: 站点初始化失败时返回错误并跳过该站点
  修复了 MTEAM、HDSKY 和 CMCT 站点实现中 NewXxxImpl 函数的错误处理逻辑

### Build

- **workflow**: 添加前端构建步骤到release工作流

### Documentation

- **readme**: 更新 README 文档内容与结构
- 补充支持站点列表及认证方式说明 - 完善 Web 配置页面各模块的参数说明

### Features

- **web**: 使用vue3改写前端页面
  在 Dockerfile 中新增前端构建阶段，使用vue3改写页面。

## [0.1.1] - 2025-11-22

### Bug Fixes

- 修复windows下种子保存路径识别错误的问题

## [0.1.0] - 2025-11-17

### Documentation

- **docker**: 移除 Docker 时区设置说明

### Features

- 重构配置系统，新增 Web 管理界面与多项功能优化
- 改用 SQLite 存储配置，移除 viper 依赖 - 新增 Web 管理界面及静态资源，支持密码重置与任务分页 - 引入 TorrentInfo.IsFree、重试计数、错误记录等字段，优化任务列表展示 - 统一工作目录常量，增强站点配置校验，简化 Docker 单目录挂载 - 调整 UI 样式，移除废弃配置与命令，更新 Go 1.25.2 与文档

## [0.0.17] - 2025-07-02

### Bug Fixes

- **docker**: 为添加的用户设置 HOME 环境变量
- 在创建用户时使用 -h 参数指定 HOME 目录 - 解决了pt-tools工作目录错误的问题

## [0.0.16] - 2025-07-02

### Features

- **docker**: 添加环境变量配置并优化容器启动脚本
- 在 README.md 中添加环境变量配置说明，包括 PUID、PGID 和 TZ - 修改 docker-entrypoint.sh，优化 /app 目录权限设置，忽略只读挂载目录的错误

## [0.0.15] - 2025-07-02

### Documentation

- **docker**: 更新 README 中的容器交互命令
- 将 docker exec 命令中的 /bin/bash 改为 /bin/sh
- 更新 README.md 中的项目描述

### Features

- **docker**: 优化 Docker 构建和运行时环境
- 添加 gosu 工具，用于在非 root 用户下运行应用 - 在构建过程中添加 ca-certificates、dpkg 和 gnupg 依赖 - 通过环境变量设置 PUID 和 PGUID，默认为 1000 - 将用户创建和权限设置移至初始化脚本中 - 修改启动命令，使用 gosu 切换到目标用户运行应用 - 优化 Makefile 中的构建命令

## [0.0.14] - 2025-07-02

### Features

- **docker**: 添加 Docker 支持并优化配置流程
- 新增 Dockerfile 和 docker-entrypoint.sh 文件，实现 Docker 化部署 - 更新 Makefile，添加 HTTP_PROXY 等代理变量支持 - 修改 README.md，增加 Docker 部署说明 - 重构 config_init.go，优化配置目录初始化逻辑 - 更新 hooks.go，添加对下载目录的检查和初始化 - 调整 root.go，延迟配置文件加载到程序运行时 - 修改 viper.go，增加对默认配置文件路径的支持

## [0.0.13] - 2025-07-02

### Features

- **run**: 添加程序互斥锁功能
- 实现了 acquireLockOrExit 函数来创建和加锁锁文件 - 在 runCmdFunc 中添加了锁文件的创建和释放逻辑

### Refactor

- **run**: 重构互斥锁实现，支持跨平台
- 移除原有直接使用 unix.Flock 的实现 - 新增 utils 包下的 Locker 接口和具体实现 - 实现了 Unix 和 Windows 平台的锁机制 - 优化了错误处理和资源释放

## [0.0.12] - 2025-07-01

### Features

- **site**: 添加种子信息缓存并优化下载流程
- 新增 bigcache 作为种子信息缓存，提高重复请求的处理效率 - 优化下载工作器中的日志输出，提高错误信息的可读性 - 在下载路径中使用清理后的标题，避免特殊字符导致的文件名错误 - 允许 Collector 重新访问已爬取的 URL

## [0.0.11] - 2025-07-01

### Build

- 更新 Go 依赖至最新版本
- 将 Go 语言版本从 1.23.1 升级到 1.24.3 - 更新多个依赖库至最新版本 - 修复非免费种子误下载的问题
- 更新 Go 依赖至最新版本
- 将 Go 语言版本从 1.23.1 升级到 1.24.3 - 更新多个依赖库至最新版本 - 修复非免费种子误下载的问题

## [0.0.10] - 2025-07-01

### Features

- **internal**: 优化种子处理逻辑并添加过期检查
- 新增 processSingleTorrent 函数，用于独立处理每个种子文件 - 添加种子过期检查逻辑，标记并删除过期种子 - 优化已推送种子的处理流程，避免重复推送
- **internal**: 优化种子处理逻辑并添加过期检查
- 新增 processSingleTorrent 函数，用于独立处理每个种子文件 - 添加种子过期检查逻辑，标记并删除过期种子 - 优化已推送种子的处理流程，避免重复推送

## [0.0.9] - 2025-04-06

### Build

- **ci**: 升级 GitHub Actions 依赖版本
- 将 actions/checkout 从 v3 升级到 v4 - 将 actions/upload-artifact 从 v3 升级到 v4 - 将 actions/download-artifact 从 v3 升级到 v4

## [0.0.8] - 2025-04-06

### Features

- **qbit**: 添加请求自动重试机制并处理禁止访问错误

## [0.0.7] - 2024-12-06

### Refactor

- **rss**: 优化 RSS 任务执行间隔和日志处理
- 新增 getInterval 函数，用于获取 RSS 任务的执行间隔 - 使用全局配置中的默认间隔作为备用 - 优化日志记录，将 Fatal 改为 Error，避免程序意外退出 - 添加信号量控制，确保数据库事务的原子性 - 更新 go.mod 和 go.sum，添加 golang.org/x/sync 依赖

## [0.0.6] - 2024-12-05

### Features

- **log**: 重构日志系统并优化输出格式
- 重构了全局日志初始化和访问方式 - 优化了日志输出格式，增加了更多详细信息 - 调整了日志级别和输出方式 - 修复了一些日志相关的错误处理

## [0.0.5] - 2024-12-05

### Features

- **cmd**: 改进多个子命令描述和逻辑
- 为 `config` 命令更新了描述，简化并增强了帮助信息 - 为 `config init` 添加示例和详细说明 - 增强 `db` 命令，添加 `PersistentPreRun` 以确保配置检查 - 修改 `db init` 和 `db backup` 的描述及运行逻辑，增加用户提示和错误处理 - 改进 `task` 和 `task list` 命令的描述，补充示例，完善输出信息 - 改进配置和日志初始化流程的错误处理

## [0.0.4] - 2024-12-05

### Bug Fixes

- 禁用CGO编译
- 禁用CGO,以支持二进制文件独立运行

## [0.0.3] - 2024-12-04

### Documentation

- **README**: 更新文档快速部署和使用 pt-tools
- 新增一键部署脚本说明 - 添加下载最新 Release 的详细步骤 - 补充快速开始部分，包括初始化配置和运行方法 - 更新 GitHub 仓库链接 - 修正许可证链接
- **README**: 更新 pt-tools 安装命令

### Features

- **site**: 添加对 CMCT 站点的支持
- 新增 CMCT 站点的配置和解析逻辑 - 实现 CMCT 站点的 RSS 订阅和种子下载功能 - 优化站点配置结构，支持更多站点类型 - 重构部分代码以提高可扩展性和可维护性 - 修改release压缩包内的二进制文件名统一为pt-tools - 在全局配置中增加 torrent_size_gb 选项，用于设置默认的下载种子大小限制 - 更新站点配置初始化和处理逻辑

## [0.0.2] - 2024-12-04

### Features

- 添加自动下载安装脚本并优化相关功能
- 新增 download.sh 脚本，实现自动检测平台并下载安装最新版本 pt-tools - 优化 Makefile 中的 upx-binaries 目标，增加对 windows-\*.exe 文件的判断 - 修复 MTTorrentDetail.CanbeFinished 方法，增加对 DiscountEndTime 为空的判断 - 优化 CanbeFinished 方法错误日志，增加 tid 信息

## [0.0.1] - 2024-12-04

### CI/CD

- **release**: 重构 GitHub Actions 工作流
- 更新工作流名称和步骤，增加 Docker 镜像构建和推送 - 移除不必要的环境变量和条件判断 - 简化二进制文件构建和打包流程 - 更新 Dockerfile，增加配置文件路径和调整ENTRYPOINT - 重构 Makefile，支持多平台构建和 UPX 压缩 - 更新 README，优化命令行用法说明 - 在 README.md 中新增配置说明部分，详细介绍配置文件结构和示例 - 新增 config.toml 文件，提供默认配置示例 - 更新 config.go 和 zap.go，调整配置结构和默认日志配置 - 在 Dockerfile 中添加构建环境和基础镜像的参数 - 实现本地和远程构建的逻辑区分 - 优化 Makefile 中的构建命令 - 添加 upx-binaries 目标，使用 UPX 压缩二进制文件 - 增加 package-binaries 目标，将二进制文件打包成 tar.gz 或 zip 格式 - 优化 build-binaries 目标，增加对不同操作系统和架构的支持 - 合并构建、压缩和打包二进制文件的步骤 - 添加 TAG 变量以支持自定义版本标签 - 在 Dockerfile 和 Makefile 中添加构建参数，用于设置版本信息 - 更新 Go 构建命令，将版本信息编译到可执行文件中 - 重构配置文件，增加全局配置和站点配置结构 - 新增 version 命令，用于显示版本信息

---

_Generated by [git-cliff](https://github.com/orhun/git-cliff)_
