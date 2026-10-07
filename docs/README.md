# pt-tools 文档

[返回项目首页](../README.md) · 文档站：[firlab.app/pt-tools](https://firlab.app/pt-tools/)（中文）· [English](https://firlab.app/pt-tools/en/)

本目录是文档站的内容源：中文页在 `docs/`，英文页在 `docs/en/` 的同一路径下，同一份文件在 GitHub 上也能直接阅读。这个文件本身是 GitHub 上的目录索引，不发布到文档站。

## 开始使用

- [pt-tools 是什么](guide/what-is-pt-tools.md)：工作方式、界面各页的用途与适用场景。
- [安装](guide/install.md)：Docker、Linux 与 Windows 的安装方式、环境变量和首次登录。
- [快速开始](guide/quick-start.md)：添加下载器、站点和 RSS 订阅，确认第一次自动下载。
- [配置说明](configuration.md)：环境变量、代理、全局设置、下载器、持久化、日志和备份。
- [升级与备份](guide/upgrade.md)：各安装方式的升级、备份与恢复。
- [Docker 运行示例](../examples/docker-run.md)、[Docker Compose 示例](../examples/docker-compose.yml)、[二进制运行示例](../examples/binary-run.md)。

## 站点与认证

- [支持站点](sites.md)：当前 66 个内置站点、认证方式与适配备注。
- [获取 Cookie / API Key](guide/get-cookie-apikey.md)：浏览器扩展同步与手动获取认证信息。
- [浏览器扩展](guide/browser-extension.md)：PT Tools Helper 的安装、Cookie 同步与站点采集。
- [站点登录状态与密钥备份](guide/site-login-monitoring.md)：登录状态监控、提醒策略、`secret.key` 备份与合规边界。
- [CookieCloud 导入](guide/cookiecloud.md)：从自建的 CookieCloud 服务取回 Cookie，在本机解密后写进站点，可选定时同步。
- [请求新增站点](guide/request-new-site.md)：采集、脱敏并提交新站点适配资料。

## RSS 与下载

- [RSS 订阅配置](guide/rss-subscription.md)：订阅地址、选项、下载模式与故障排查。
- [过滤规则与追剧](guide/filter-rules-tv-series.md)：关键词、通配符、正则和组合规则。
- [自动删种与磁盘保护](guide/auto-cleanup.md)：清理范围、H&R 保护、磁盘门禁和工作目录清理。
- [刷流任务](guide/brush.md)：从站点的免费列表挑种做种、按规则删种，限额与收益统计，以及从 Vertex 迁移。
- [数据统计与每日战报](guide/user-stats.md)：每日快照、增量口径、走势图与每日战报。
- [下载器助手](guide/downloader-assistant.md)：失效种子、补站点标签、替换 tracker 与失效种子定时扫描。
- [转移做种](guide/torrent-transfer.md)：把种子搬到另一台下载器继续做种，路径映射与定时规则。
- [IYUU 辅种](guide/reseed.md)：用 IYUU 找出其他站点上数据相同的种子，核对后加进同一台下载器。

## ChatOps 与通知

- [ChatOps 快速开始](guide/chatops-quickstart.md)：通道选择、命令清单与配置入口。
- [QQ OneBot（NapCat）](guide/chatops-qq-napcat.md)：反向 WebSocket、绑定与排障。
- [Telegram Bot](guide/chatops-telegram.md)：BotFather、代理、绑定与排障。
- [RSS 上新通知](guide/chatops-rss-notify.md)：通知模式、静默时段、digest、重试和配额。

## 参考

- [常见问题](faq.md)：下载器、认证、RSS、免费暂停与数据库排障。
- [命令行](reference/cli.md)：`web`、`secret`、`clean` 等子命令与参数。
- [数据与安全](reference/security.md)：数据位置、加密范围、外连、监听端口与部署建议。
- [参与开发](reference/developers.md)：从源码构建、提交改动与维护本文档站。

## 开发与设计（仅中文）

- [开发指南](development.md)：固定工具链、构建门禁、代码规范、站点定义与 fixture 测试。
- [品牌标识](brand.md)：标志文件、变体选择、最小尺寸与留白规则。
- [ChatOps / MCP / Agent 架构](design/chatops-mcp-agent.md)：能力分层、权限模型与当前实现边界。
- [MCP Server 接口契约](design/phase4-mcp.md)：未来 MCP 工具、传输与安全边界。
- [AI Agent 设计](design/phase5-agent.md)：未来 Agent 模式、接入路径与非目标。

> [!NOTE]
> `docs/design/` 记录设计契约和后续演进方向，不表示相应能力已经对外发布。当前用户可用能力以文档站、Web UI 与 Release 说明为准。

---

## 维护文档站

文档站的文字都在本目录；VitePress 配置、主题、组件和部署在 [sunerpy/firlab](https://github.com/sunerpy/firlab) 的 `pt-tools/` 下（那里的 `README.md` 说明归属与一次性配置）。站点没有单独的域名，随 firlab.app 一起由 GitHub Pages 发布在 `/pt-tools/` 路径下。

### 目录与站点路径

下表的站点路径都相对于 `https://firlab.app/pt-tools/`，例如 `/en/` 就是 `https://firlab.app/pt-tools/en/`。

| 本目录中的路径                                                                                        | 站点路径                      | 说明                                                     |
| ----------------------------------------------------------------------------------------------------- | ----------------------------- | -------------------------------------------------------- |
| `index.md`、`en/index.md`                                                                             | `/`、`/en/`                   | 首页，文字在 frontmatter 里，见下文「首页数据」          |
| `guide/`、`reference/`、`configuration.md`、`faq.md`、`sites.md`                                      | `/guide/*`、`/reference/*` 等 | 用户文档；`en/` 下同一路径是英文版，两种语言必须同时存在 |
| `development.md`、`design/chatops-mcp-agent.md`、`design/phase4-mcp.md`、`design/phase5-agent.md`     | `/development`、`/design/*`   | 只有中文；英文站同一路径是同步脚本生成的指引页（不收录） |
| `public/screens/`                                                                                     | `/screens/*.webp`             | 首页截图，见下文「截图」                                 |
| `guide/images/`                                                                                       | 随页面发布                    | 页面引用的图片                                           |
| `README.md`、`brand.md`、`design/webui-board-spec.md`、`guide/chatops-mcp-agent-design.md`、`readme/` | 不发布                        | 仓库内资料；`readme/` 是根目录 README 的英文版           |

`web/frontend/public/logo.svg` 也会同步过去，作为站点图标。

### 一次修改怎么上线

1. 改动本目录的 PR 会触发 `.github/workflows/docs-site.yml`：检出公开的 firlab 仓库，把本目录同步进去，构建一次站点并运行 firlab 的 `pt-tools/scripts/check-dist.sh`。死链、未注册的组件、只有一种语言的页面、禁用词、不合格的首页数据，或指向 `/pt-tools/` 以外的站内链接都会让这一步失败。它不读取任何密钥，fork 的 PR 也能运行。
2. 合并到 `main` 后，`.github/workflows/publish-docs-site.yml` 运行 firlab 的 `pt-tools/scripts/sync-pt-tools-docs.sh`，把结果以 `docs(pt-tools): sync from pt-tools@<sha>` 提交到 firlab 的 `main`。这一步需要仓库密钥 `FIRLAB_DOCS_TOKEN`（见 `.github/README-secrets.md`）。
3. firlab 的 `deploy.yml` 构建主站和本站点，检查后把本站点放进主站的 `pt-tools/` 目录，一起部署到 GitHub Pages。

每个页面的页脚都写着内容来自 pt-tools 的哪个提交。

### 本地预览

```bash
git clone https://github.com/sunerpy/firlab ../firlab    # 只需一次
../firlab/pt-tools/scripts/sync-pt-tools-docs.sh "$PWD"
cd ../firlab/pt-tools && pnpm install --frozen-lockfile && pnpm dev    # http://localhost:5173/pt-tools/
```

每次修改后重新运行同步脚本。它会在页面缺少另一种语言的版本、使用了站点未注册的组件或使用了禁用词时停下并给出原因；从未提交的工作区同步时，页脚的提交号带 `-dirty`。提交前另外运行 `make docs-check`，它检查本目录内所有相对链接和锚点。

### 写作约定

- **两种语言同时更新**：中文页与英文页章节相同、顺序相同。只有中文的页面仅限开发指南和设计文档。
- **链接用相对路径**：页面之间写 `../faq.md` 这样的 `.md` 相对链接，GitHub 和文档站都能打开；英文页链接只有中文的页面时，要跳出 `en/`（如 `../../development.md`）。
- **界面文字以 Web UI 为准**：写之前对照 `web/frontend/src/config/navigation.ts`、路由和页面里的实际文字，不凭记忆写。Web UI 只有中文，英文页写成「English name (中文)」，如 `Downloader settings (下载器设置)`。
- **行为以代码为准**：默认值、选项和限制要能在代码里找到出处；与代码不符时改文档，不要沿用旧说法。
- **规范书面语**：中文页不用口语（还没、没能、免得、搭的、咋、啥、搞定、折腾），英文页不用 just、gonna、stuff。中文与英文、数字之间留空格。
- **用户页不写内部实现**：不出现 `UnifiedPTSite`、`ConfigStore`、`DiskBudget`、`MessageChain` 之类的代码名和 `§` 符号；`reference/developers.md` 例外。同步脚本会拒绝这些词。
- **未发布的能力单独说明**：实验性、开发中和计划中的功能用 `<StatusTag>` 标出，不能写成已经可用。
- **不出现真实凭证**：文字和截图里不能有真实域名下的 Cookie、Passkey、Token 或下载器地址。

### 可用组件

页面只能使用下面这些组件，其他标签会让同步失败：

| 组件                                                                               | 用途                                          |
| ---------------------------------------------------------------------------------- | --------------------------------------------- |
| `<StatusTag status="available \| experimental \| building \| planned" />`          | 发布状态，以文字显示                          |
| `<ScreenFigure src dark? width height alt caption? />`                             | 截图；`dark` 是同一画面的深色主题版本         |
| `<Badge>`                                                                          | VitePress 自带的标记                          |
| `HomeIndex`、`HomeSteps`、`SplitBlock`、`HomeDeploy`、`HomePrivacy`、`HomeRoadmap` | 仅用于首页，渲染 `home:` frontmatter 中的数据 |

### 首页数据

两个首页的文字都在 frontmatter 里：VitePress 的 `hero:`（名称、标题、副标题、按钮）和一段由组件渲染的 `home:`。构建时会按 firlab 中的 `pt-tools/src/.vitepress/theme/data/home-schema.ts` 检查 `home:`，缺少字段或多出未知字段都会失败。

| 键         | 内容                                                                                        |
| ---------- | ------------------------------------------------------------------------------------------- |
| `facts`    | 副标题下的事实行：`term`、`text`（至少两条）                                                |
| `visual`   | hero 右侧的截图：`desktop`，可选 `mobile`；每张有 `light`、`dark`、`width`、`height`、`alt` |
| `index`    | 功能一览：`title`、`intro`、`groups[].items[]`（`title`、`body`、`status`、`link`）         |
| `steps`    | 上手步骤：`title`、`items[]`（`title`、`body`，可选 `command`，至少两步）                   |
| `rules`    | RSS 分屏里的过滤规则示例：`columns`（三列）、`rows[]`（`pattern`、`kind`、`matches`）       |
| `commands` | ChatOps 分屏里的命令示例：`items[]`（`command`、`body`，至少两条）                          |
| `shots`    | `<SplitBlock proof="screen" shot="…">` 引用的截图，键名即 `shot` 的值                       |
| `deploy`   | 部署方式表：`columns`，`rows[]`（`name`、`status`、`cells`，比 `columns` 少一项）           |
| `privacy`  | 数据去向：`sendsLabel`，`modes[]`（`name`、`sends`、`detail`）                              |
| `roadmap`  | 路线图：`items[]`（`title`、`body`、`status`），`notPlanned`                                |

`status` 取值为 `available`、`experimental`、`building` 或 `planned`。

### 截图

截图是 `public/screens/` 和 `guide/images/chatops/` 下的 WebP 文件，来自真实的 Web UI 加载验收假数据（`web/frontend/scripts/board-fixtures.mjs`）后的画面，不会出现真实账号或地址：

```bash
make build-frontend
go build -o /tmp/pt-tools .
HOME=$(mktemp -d) /tmp/pt-tools web --host 127.0.0.1 --port 18310 &
node web/frontend/scripts/docs-screens.mjs http://127.0.0.1:18310
```

脚本用 Chrome 的 CDP 截取桌面 1440 × 900 的用户统计页和站点列表、手机 375 × 812 的用户统计页，明暗主题各一张，写回 `public/screens/`；再用明亮主题截取 ChatOps 指南的 8 张配图（通道列表、添加通道、Telegram 与 QQ 的凭证、绑定列表与生成绑定码、操作审计、RSS 通知日志），写回 `guide/images/chatops/`。凭证里的 Token 和用户 ID 是脚本里的示例值；截图里的版本号取 `.release-please-manifest.json` 的当前版本。仓库根目录的 README 与 `readme/README.en.md` 用的也是 `public/screens/` 这几张。Chrome 路径可用 `PT_CHROME` 指定。改了截图尺寸要同步修改两个首页中的 `width` 和 `height`；提交前逐张检查图片。界面的文字或布局变化后重新截图。
