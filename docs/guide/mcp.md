# MCP <Badge type="tip" text="1.0 预览版" />

MCP（Model Context Protocol）是 AI 助手调用外部工具的协议。pt-tools 在 Web 端口上提供 MCP 服务：接上以后，Claude、Cursor、Cherry Studio 这类 MCP 客户端里的 AI 助手可以查看种子、下载器、站点数据与订阅，搜索种子，并在你确认以后暂停、删除、推送种子或添加订阅；令牌另外勾了「配置」时，还能读、改 pt-tools 的配置。

MCP 服务和 Web 界面在同一个端口上，地址是 `http://<pt-tools 所在主机>:<端口>/mcp`。「系统 → MCP 接入」页写明了这台 pt-tools 的地址、各种客户端的填法与工具清单。

## 准备令牌

在「系统 → API 令牌」新建一个令牌，见 [API 令牌](api-tokens.md)：

| 权限     | 能做什么                                                                                     |
| -------- | -------------------------------------------------------------------------------------------- |
| MCP 读取 | 调用只读工具：推送记录、下载器里的种子、下载器状态、搜索、站点数据、检查更新、找片与订阅列表 |
| MCP 操作 | 调用会改动下载器或订阅的工具：暂停、继续、删除、推送种子，添加订阅                           |
| 配置     | 另外再勾：`get_config` 读配置（要「MCP 读取」），`change_config` 改配置（要「MCP 操作」）    |

两种 MCP 权限互不包含：只选「MCP 操作」的令牌调用不了只读工具，所以通常两个都选；只想让 AI 助手看数据时只选「MCP 读取」。要让它读改 pt-tools 的配置再勾「配置」，已有的令牌不会自动有这一项。

## 客户端怎么填

下面的例子里 pt-tools 在 `192.168.1.10:8080`，令牌是 `ptt_1_xxxxxxxx`，换成你自己的。

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
- 令牌放在环境变量 `PT_TOOLS_MCP_TOKEN` 里，不要放进 `args`：命令行参数会出现在进程列表中。
- Windows 上 `command` 填 `pt-tools.exe` 的完整路径。

## 工具

| 工具                       | 权限     | 做什么                                                                               |
| -------------------------- | -------- | ------------------------------------------------------------------------------------ |
| `list_tasks`               | MCP 读取 | 推送记录：RSS、订阅、App 与 MCP 推过的种子，可以按站点、关键字筛选                   |
| `list_downloader_torrents` | MCP 读取 | 下载器里的种子，分页，可以按下载器、状态、关键字筛选与排序                           |
| `get_downloader_stats`     | MCP 读取 | 各台下载器连不连得上、上传下载速度、累计量与剩余空间                                 |
| `search_torrents`          | MCP 读取 | 在已启用的站点上搜索种子；结果带站点与种子编号，不带下载链接                         |
| `get_site_userinfo`        | MCP 读取 | 各站点的上传、计费下载量、分享率、魔力、做种数、未读消息、登录状态与签到结果         |
| `check_updates`            | MCP 读取 | 有没有新版本，只看不升级                                                             |
| `explore_media`            | MCP 读取 | 按片名在 TMDB 找电影与剧集，或者看本周趋势、热门                                     |
| `list_subscriptions`       | MCP 读取 | 订阅与进度：已播、已入库、下载中与缺的集                                             |
| `get_subscription`         | MCP 读取 | 一个订阅的详情：每一集的情况（已入库、下载中、缺、未播）与下载过的种子               |
| `recognize_media`          | MCP 读取 | 识别种子标题：解析名字、年份、季集与规格，按手动纠正与识别词到 TMDB 找条目           |
| `list_media_libraries`     | MCP 读取 | 媒体库：类型、目录、整理方式与刮削                                                   |
| `list_media_history`       | MCP 读取 | 整理历史：每个视频放进了哪里，跳过、失败的原因，可以按状态与关键字筛选               |
| `preview_media_organize`   | MCP 读取 | 预览整理下载器里的一个种子：认出的条目、媒体库与每个视频的目标位置，不改动           |
| `get_config`               | MCP 读取 | 读 pt-tools 的配置（另要「配置」权限）：不带资源名时是配置目录；密钥只显示有没有设置 |
| `get_skill`                | MCP 读取 | 内置技能：`pt-tools-config` 写明怎么读改配置，附配置目录                             |
| `pause_torrent`            | MCP 操作 | 暂停一个种子                                                                         |
| `resume_torrent`           | MCP 操作 | 继续一个暂停的种子                                                                   |
| `delete_torrent`           | MCP 操作 | 删除一个种子，可以连数据一起删                                                       |
| `push_torrent`             | MCP 操作 | 把种子推到下载器：给搜索结果里的站点与种子编号，或者已配置站点的种子下载地址         |
| `add_subscription`         | MCP 操作 | 按 TMDB 编号订阅一部电影或一季剧集                                                   |
| `set_subscription_status`  | MCP 操作 | 暂停或恢复一个订阅                                                                   |
| `search_subscription`      | MCP 操作 | 立即搜索一个订阅，搜到合适的种子就下载                                               |
| `delete_subscription`      | MCP 操作 | 删除一个订阅；下载过的种子与库里的文件不动                                           |
| `organize_media`           | MCP 操作 | 把下载完的种子整理进媒体库：按命名模板放好、刮削，通知媒体服务器扫描                 |
| `retry_media_organize`     | MCP 操作 | 重新整理一条整理记录对应的种子（多半是失败的那条）                                   |
| `change_config`            | MCP 操作 | 改 pt-tools 的配置（另要「配置」权限）：先回预览，确认以后才保存                     |

- 会改动下载器、订阅、媒体库或配置的工具，不带 `confirm=true` 调用时不执行，回一份预览：会改什么（每一项改以前、改以后）与可能的影响。客户端的 AI 把预览给你看，你同意以后它再带 `confirm=true` 调一次才执行。
- `organize_media` 与 `preview_media_organize` 的种子用 `list_downloader_torrents` 里的下载器编号与 `info_hash`；认错了条目时给 `media_type` 与 `tmdb_id`（从 `recognize_media` 或 `explore_media` 取）指定，给 `library_id` 指定媒体库。整理的规则和网页上手动整理一样，见[整理入库](media-library.md)。
- `push_torrent` 由 pt-tools 用自己的站点配置下载种子文件，照常经过磁盘空间保护与站点做种容量的检查；不接受磁力链接，也不去请求别的地址。经 MCP 推送的种子在任务列表里的来源是 `mcp_push`。
- `get_site_userinfo` 返回的下载量（`downloaded`）是站点计费的下载量：站点按免费、折扣折算以后计入分享率的数，不是实际传输量，详见[数据统计](user-stats.md#下载量的两种口径)。
- 返回的数据和 [App API](../reference/app-api.md) 一样：没有 Cookie、Passkey、密码、RSS 地址与 PT 站点的种子下载链接，下载器的错误信息里也去掉了它的地址。`check_updates` 会返回 GitHub 上公开的发布页与安装包地址。
- 参数不对（缺必填项、取值不在范围里、多出不认识的字段）时工具直接报错，不会去调用 pt-tools 的接口。

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
- 不能经配置工具改：登录密码、API 令牌、远程访问、加密密钥、AI 助手自己的设置（含语音输入）、ChatOps 绑定与管理员、qB 兼容入口。
- 能改哪些配置见 [AI 助手 · 改配置](assistant.md#改配置)；接口见 [App API · 配置](../reference/app-api.md#配置)。

## 审计与安全

- `/mcp` 只认 API 令牌，网页登录的会话用不了：没有令牌或令牌不对回 401，令牌没有 MCP 权限回 403。撤销令牌立即生效。
- 会改动下载器、订阅、媒体库或配置的工具每次执行都记在「ChatOps → 操作审计」里，因为权限不够或参数不对而被拒的也记（只回预览的不记）：通道是「MCP」，命令是工具名，触发用户是令牌编号。`change_config` 只记改了哪类配置、哪些字段，不记值。
- 在公网上访问时请用 HTTPS（例如放在反向代理之后），否则令牌会以明文经过网络。反向代理要保留 `Authorization` 请求头。
