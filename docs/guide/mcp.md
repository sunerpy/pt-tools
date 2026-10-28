# MCP <Badge type="tip" text="1.0 预览版" />

MCP（Model Context Protocol）是 AI 助手调用外部工具的协议。pt-tools 在 Web 端口上提供 MCP 服务：接上以后，Claude、Cursor、Cherry Studio 这类 MCP 客户端里的 AI 助手可以查看种子、下载器、站点数据与订阅，搜索种子，并在你确认以后暂停、删除、推送种子，建订阅、追剧（改定时、版本锁、每一集、集号规则），重新整理与刮削媒体库；令牌另外勾了「配置」时，还能读、改 pt-tools 的配置。

MCP 服务和 Web 界面在同一个端口上，默认地址是 `http://<pt-tools 所在主机>:<端口>/mcp`。「系统 → MCP 接入」页可以打开、关闭 MCP 接入和换路径，并写明了这台 pt-tools 现在的地址、各种客户端的填法与工具清单。

## 打开 MCP 接入

「系统 → MCP 接入」的「设置」里：

- MCP 接入：打开以后 AI 客户端才连得上；关着时 MCP 的地址回 404，并写明「MCP 接入没有打开」。新装的 pt-tools 默认关着；从更早的版本升级上来、已经有带 MCP 权限的令牌时，第一次启动记成开着，已经接好的客户端不会断。
- 路径：默认 `/mcp`，可以换成别的（例如反向代理只转发某个前缀时用 `/agent/mcp`）。只能有字母、数字与 `. _ ~ -`，用 `/` 分级；不能放在 `/api`、`/assets`、`/static`、`/login` 下面，也不能和 pt-tools 已有的地址撞上。换了以后旧的 `/mcp` 回 404（不说新地址在哪），客户端里的地址要跟着改。
- 保存后马上生效，不用重启。内置的 [AI 助手](assistant.md) 在 pt-tools 里面调用同一套工具，不经这个地址，不受开关影响。
- 开关与路径只能在网页上改：API 令牌与 `change_config` 都改不了。

## 准备令牌

在「系统 → API 令牌」新建一个令牌（「MCP 接入」页没有能用的令牌时有「新建一个」的入口，权限已经选好），见 [API 令牌](api-tokens.md)：

| 权限     | 能做什么                                                                                     |
| -------- | -------------------------------------------------------------------------------------------- |
| MCP 读取 | 调用只读工具：推送记录、下载器里的种子、下载器状态、搜索、站点数据、检查更新、找片与订阅列表 |
| MCP 操作 | 调用会改动下载器、订阅或媒体库的工具：暂停、继续、删除、推送种子，管理订阅与追剧，整理与刮削 |
| 配置     | 另外再勾：`get_config` 读配置（要「MCP 读取」），`change_config` 改配置（要「MCP 操作」）    |

两种 MCP 权限互不包含：只选「MCP 操作」的令牌调用不了只读工具，所以通常两个都选；只想让 AI 助手看数据时只选「MCP 读取」。要让它读改 pt-tools 的配置再勾「配置」，已有的令牌不会自动有这一项。

## 客户端怎么填

下面的例子里 pt-tools 在 `192.168.1.10:8080`、路径是默认的 `/mcp`，令牌是 `ptt_1_xxxxxxxx`，换成你自己的（「MCP 接入」页的填法已经带上这台 pt-tools 现在的地址）。

### 支持 HTTP 的客户端

Cursor、Cherry Studio、VS Code 这类客户端直接填地址与请求头，传输方式选 Streamable HTTP。各客户端的配置位置不同，字段大致如下：

```json
{
  "mcpServers": {
    "pt-tools": {
      "url": "http://192.168.1.10:8080/mcp",
      "headers": { "Authorization": "Bearer ptt_1_xxxxxxxx" }
    }
  }
}
```

Claude Code 用命令添加：

```bash
claude mcp add --transport http pt-tools http://192.168.1.10:8080/mcp --header "Authorization: Bearer ptt_1_xxxxxxxx"
```

### 只支持 stdio 的客户端

Claude Desktop 这类客户端只会启动一个本地程序，经标准输入输出与它对话。在客户端那台机器上放一份 pt-tools 的程序文件（不需要运行 Web 服务），用 `pt-tools mcp` 转发给运行中的 pt-tools：

```json
{
  "mcpServers": {
    "pt-tools": {
      "command": "/usr/local/bin/pt-tools",
      "args": ["mcp", "--url", "http://192.168.1.10:8080"],
      "env": { "PT_TOOLS_MCP_TOKEN": "ptt_1_xxxxxxxx" }
    }
  }
}
```

- `pt-tools mcp` 不打开数据库，也不启动调度器，只把运行中的 pt-tools 的工具原样转发。参数见[命令行](../reference/cli.md#pt-tools-mcp)。
- `--url` 只给主机与端口时转发到 `/mcp`；换了路径时给完整地址，例如 `http://192.168.1.10:8080/agent/mcp`。
- 令牌放在环境变量 `PT_TOOLS_MCP_TOKEN` 里，不要放进 `args`：命令行参数会出现在进程列表中。
- Windows 上 `command` 填 `pt-tools.exe` 的完整路径。

## 工具

| 工具                              | 权限     | 做什么                                                                               |
| --------------------------------- | -------- | ------------------------------------------------------------------------------------ |
| `list_tasks`                      | MCP 读取 | 推送记录：RSS、订阅、App 与 MCP 推过的种子，可以按站点、关键字筛选                   |
| `list_downloader_torrents`        | MCP 读取 | 下载器里的种子，分页，可以按下载器、状态、关键字筛选与排序                           |
| `get_downloader_stats`            | MCP 读取 | 各台下载器连不连得上、上传下载速度、累计量与剩余空间                                 |
| `search_torrents`                 | MCP 读取 | 在已启用的站点上搜索种子；结果带站点与种子编号，不带下载链接                         |
| `get_site_userinfo`               | MCP 读取 | 各站点的上传、计费下载量、分享率、魔力、做种数、未读消息、登录状态与签到结果         |
| `check_updates`                   | MCP 读取 | 有没有新版本，只看不升级                                                             |
| `explore_media`                   | MCP 读取 | 按片名在 TMDB 找电影与剧集，或者看本周趋势、热门                                     |
| `list_subscriptions`              | MCP 读取 | 订阅与进度：已播、已入库、下载中与缺的集                                             |
| `get_subscription`                | MCP 读取 | 一个订阅的详情：设置、每一集的情况与下载过的种子（每个文件整理、刮削得怎样）         |
| `list_subscription_runs`          | MCP 读取 | 订阅的运行记录：每次搜到了什么、哪些种子为什么没要、推送了哪个、哪个失败了           |
| `get_episode_rules`               | MCP 读取 | 追剧的集号规则：标题里的集号怎么对到 TMDB 的集（综艺按期号、播出日期）               |
| `preview_schedule`                | MCP 读取 | 看一个定时（间隔、每天、每周、每月、cron）接下来几次什么时候搜，不保存               |
| `recognize_media`                 | MCP 读取 | 识别种子标题：解析名字、年份、季集与规格，按手动纠正与识别词到 TMDB 找条目           |
| `list_media_libraries`            | MCP 读取 | 媒体库：类型、目录、整理方式与刮削                                                   |
| `list_media_history`              | MCP 读取 | 整理历史：每个视频放进了哪里，跳过、失败的原因，可以按状态与关键字筛选               |
| `preview_media_organize`          | MCP 读取 | 预览整理下载器里的一个种子：认出的条目、媒体库与每个视频的目标位置，不改动           |
| `get_config`                      | MCP 读取 | 读 pt-tools 的配置（另要「配置」权限）：不带资源名时是配置目录；密钥只显示有没有设置 |
| `get_skill`                       | MCP 读取 | 内置技能：`pt-tools-config` 写明怎么读改配置（附配置目录），`pt-tools-follow` 讲追剧 |
| `pause_torrent`                   | MCP 操作 | 暂停一个种子                                                                         |
| `resume_torrent`                  | MCP 操作 | 继续一个暂停的种子                                                                   |
| `delete_torrent`                  | MCP 操作 | 删除一个种子，可以连数据一起删                                                       |
| `push_torrent`                    | MCP 操作 | 把种子推到下载器：给搜索结果里的站点与种子编号，或者已配置站点的种子下载地址         |
| `add_subscription`                | MCP 操作 | 按 TMDB 编号订阅一部电影或追一季剧集，可以带定时、版本锁、起始集、从哪个种子开始     |
| `update_subscription`             | MCP 操作 | 改订阅的设置：定时、版本锁、起始集、站点、下载器、媒体库、集数等，只改给了的字段     |
| `set_episode_skip`                | MCP 操作 | 跳过追剧的一集（不追），或者重新要它                                                 |
| `unlock_episode`                  | MCP 操作 | 让一集不受版本锁限制、换别的版本（马上搜一次），或者恢复                             |
| `save_episode_rule`               | MCP 操作 | 给一个制作组设集号规则（按第几期与上下、按播出日期、集号偏移）                       |
| `delete_episode_rule`             | MCP 操作 | 删掉一个制作组的集号规则                                                             |
| `reorganize_subscription_torrent` | MCP 操作 | 重新整理订阅下载的一个种子（整季包与单集一样），按订阅的条目与媒体库                 |
| `rescrape_subscription_torrent`   | MCP 操作 | 重新刮削订阅下载的一个种子整理进的剧集或电影                                         |
| `set_subscription_status`         | MCP 操作 | 暂停或恢复一个订阅                                                                   |
| `search_subscription`             | MCP 操作 | 立即搜索一个订阅，搜到合适的种子就下载                                               |
| `delete_subscription`             | MCP 操作 | 删除一个订阅；下载过的种子与库里的文件不动                                           |
| `organize_media`                  | MCP 操作 | 把下载完的种子整理进媒体库：按命名模板放好、刮削，通知媒体服务器扫描                 |
| `retry_media_organize`            | MCP 操作 | 重新整理一条整理记录对应的种子（多半是失败的那条）                                   |
| `rescrape_media`                  | MCP 操作 | 重新刮削一条整理记录所在的电影或剧集                                                 |
| `change_config`                   | MCP 操作 | 改 pt-tools 的配置（另要「配置」权限）：先回预览，确认以后才保存                     |
| `list_brush_tasks`                | MCP 读取 | 刷流任务、状态与收益                                                                 |
| `get_brush_task`                  | MCP 读取 | 刷流配置                                                                             |
| `list_brush_runs`                 | MCP 读取 | 最近运行记录与步骤                                                                   |
| `change_brush_task`               | MCP 操作 | 先预览再确认新增、编辑、启停或删除                                                   |

- 会改动下载器、订阅、媒体库或配置的工具，不带 `confirm=true` 调用时不执行，回一份预览：会改什么（每一项改以前、改以后）与可能的影响。客户端的 AI 把预览给你看，你同意以后它再带 `confirm=true` 调一次才执行。
- `organize_media` 与 `preview_media_organize` 的种子用 `list_downloader_torrents` 里的下载器编号与 `info_hash`；认错了条目时给 `media_type` 与 `tmdb_id`（从 `recognize_media` 或 `explore_media` 取）指定，给 `library_id` 指定媒体库。整理的规则和网页上手动整理一样，见[整理入库](media-library.md)。
- `push_torrent` 由 pt-tools 用自己的站点配置下载种子文件，照常经过磁盘空间保护与站点做种容量的检查；不接受磁力链接，也不去请求别的地址。经 MCP 推送的种子在任务列表里的来源是 `mcp_push`。
- `get_site_userinfo` 返回的下载量（`downloaded`）是站点计费的下载量：站点按免费、折扣折算以后计入分享率的数，不是实际传输量，详见[数据统计](user-stats.md#下载量的两种口径)。
- 返回的数据和 [App API](../reference/app-api.md) 一样：没有 Cookie、Passkey、密码、RSS 地址与 PT 站点的种子下载链接，下载器的错误信息里也去掉了它的地址。`check_updates` 会返回 GitHub 上公开的发布页与安装包地址。
- 参数不对（缺必填项、取值不在范围里、多出不认识的字段）时工具直接报错，不会去调用 pt-tools 的接口。

## 追剧

让 AI 帮你追剧时，它先读内置技能 `pt-tools-follow`（`get_skill`），里面写明步骤、定时与 cron 的写法、版本锁、综艺的集号与失败以后怎么重试。常见的几种说法：

- 「追现在就出发第 4 季，每周五晚上 10 点和周六上午各搜一次」：`explore_media` 找到 TMDB 编号，`add_subscription` 带上 `schedule`（`{"kind":"cron","cron":"0 22 * * 5; 0 10 * * 6"}`），预览给你确认。
- 「改成每 3 小时搜一次」：`update_subscription` 改 `schedule`（`{"kind":"interval","interval_minutes":180}`），预览里列出接下来几次搜索的时间。
- 「第 5 集一直等不到 HHWEB 的版本」：`unlock_episode` 只放开这一集。
- 「为什么没下载」：`list_subscription_runs` 看每次搜索的经过；集号对不上的用 `save_episode_rule` 设规则，推送失败的用 `search_subscription` 重新搜，下载了没入库的用 `reorganize_subscription_torrent` 重新整理。

定时的写法见[追剧与订阅 · 什么时候搜索](media-subscribe.md#什么时候搜索)。订阅的总开关、全局搜索间隔、默认质量档案与下载器、豆瓣想看在「改配置」的 `subscribe`、`quality_profiles`、`douban_sources` 里。

## 改配置

令牌勾了「配置」以后，AI 能用这三个工具读改 pt-tools 的配置：

- `get_skill`：先读内置技能 `pt-tools-config`，里面写明步骤、规则、例子，以及配置目录（每一类配置能做的操作、能改的字段、新增时必填的字段、密钥字段与字段说明）。
- `get_config`：不带参数是配置目录；带 `resource`（例如 `downloaders`）是这一类配置的现状，列表类可以再带 `id` 只看一项。
- `change_config`：`resource`、`op`（对象类用 `set`；列表类用 `add`、`update`、`delete`）、`id`（改与删要给）、`values`（只写要改的字段）。不带 `confirm=true` 时回预览，带了才保存，保存以后回改完的样子。

```json
{ "resource": "downloaders", "op": "update", "id": "2", "values": { "enabled": false } }
```

- 改动交给网页上保存时用的同一个接口：校验、加密与热重载都和在网页上改一样，接口的报错原样回给 AI。
- 密钥只写不读：密码、Cookie、API Key、Passkey、令牌、RSS 地址，以及通知通道配置里的令牌与地址，读出来是 `<已设置>` 或空串。改的时候给新值才改，交回 `<已设置>` 表示不改，给空串表示清除。
- 不能经配置工具改：登录密码、API 令牌、远程访问、加密密钥、AI 助手自己的设置（含语音输入）、ChatOps 绑定与管理员、qB 兼容入口、MCP 接入（开关与地址）。
- 能改哪些配置见 [AI 助手 · 改配置](assistant.md#改配置)；接口见 [App API · 配置](../reference/app-api.md#配置)。

## 审计与安全

- MCP 的地址只认 API 令牌，网页登录的会话用不了：没有令牌或令牌不对回 401，令牌没有 MCP 权限回 403。撤销令牌立即生效。不用的时候在「MCP 接入」里关掉，地址就回 404。
- 会改动下载器、订阅、媒体库或配置的工具每次执行都记在「ChatOps → 操作审计」里，因为权限不够或参数不对而被拒的也记（只回预览的不记）：通道是「MCP」，命令是工具名，触发用户是令牌编号。`change_config` 只记改了哪类配置、哪些字段，不记值。
- 在公网上访问时请用 HTTPS（例如放在反向代理之后），否则令牌会以明文经过网络。反向代理要保留 `Authorization` 请求头。
