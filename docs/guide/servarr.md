# Servarr 兼容入口 <Badge type="tip" text="1.0 预览版" />

Servarr 兼容入口让 [Seerr](https://github.com/seerr-team/seerr) 把 pt-tools 当成 Radarr 与 Sonarr 用：在 Seerr 里请求的电影建成 pt-tools 的电影订阅，剧集请求的每一季建成一条追剧；pt-tools 搜索、推送、整理入库以后，Seerr 扫描时看到的是这些订阅的状态与库里的文件。订阅照常按质量档案挑资源、经过磁盘空间与站点做种容量的检查，见[追剧与订阅](media-subscribe.md)。

入口在 pt-tools 网页的端口上（不用另开端口），地址是 `/servarr/radarr` 与 `/servarr/sonarr`，凭有「Servarr 兼容」权限的 API 令牌访问。

兼容的是 Seerr 的固定版本：[seerr-team/seerr](https://github.com/seerr-team/seerr) 的提交 `dca1af6a`（2026-11-01）。之后 Seerr 改了调用 Radarr、Sonarr 的方式时可能接不上。

## 准备

1. 在「系统 → API 令牌」新建一个令牌，权限只勾「Servarr 兼容」，复制下来，见 [API 令牌](api-tokens.md)。
2. 在「媒体 → 追剧与订阅」最下面的「订阅设置」里打开「订阅」总开关。关着时 Seerr 的请求照样建成订阅，但不会去搜索。
3. 要指定整理进哪个媒体库时，先在「媒体 → 媒体库」建好媒体库。没有媒体库也能接上：根目录选 `auto`。

## Seerr 里怎么填

在 Seerr 的「Settings → Services」里分别加一台 Radarr 与一台 Sonarr（Add Radarr Server、Add Sonarr Server）：

| 项目                   | Radarr                                               | Sonarr            |
| ---------------------- | ---------------------------------------------------- | ----------------- |
| Hostname or IP Address | pt-tools 所在的主机                                  | 同左              |
| Port                   | pt-tools 网页的端口（默认 8080）                     | 同左              |
| Use SSL                | pt-tools 前面有 HTTPS 反向代理、走它时勾上           | 同左              |
| API Key                | 有「Servarr 兼容」权限的令牌（`ptt_` 开头）          | 同左              |
| URL Base               | `/servarr/radarr`                                    | `/servarr/sonarr` |
| Default Server         | 勾上：Seerr 只把请求发给默认的服务器                 | 同左              |
| Enable Scan            | 建议勾上：Seerr 定时读订阅，显示「处理中」与「可用」 | 同左              |

填好以后点「Test」，Seerr 读出质量档案与根目录：

- **Quality Profile**：「默认档案」是「订阅设置」里的默认质量档案；其余是 pt-tools 的[质量档案](media-subscribe.md#质量档案)。Sonarr 的 Anime Quality Profile 也从这里选。
- **Root Folder**：`auto` 是按类型自动选媒体库，和网页上新建订阅时不选媒体库一样；其余是这种类型的、启用的媒体库的目录（Radarr 列电影库，Sonarr 列剧集库）。Anime Root Folder 同样。
- **Language Profile**：不用选，pt-tools 报的是 Sonarr 4，Seerr 不再要语言档案。
- **Tags** 与 **Tag Requests**：可以用。标签只存在 pt-tools 里、回给 Seerr，不影响订阅。
- **Enable Automatic Search**：勾着时新建的订阅马上搜（总开关开着时一分钟内）；不勾时按定时搜，电影按「订阅设置」里的搜索间隔，追剧按播出时间（第一次最晚一天以后）。

Minimum Availability、Series Type、Season Folders 与 Monitor New Seasons 不起作用：什么时候搜、文件怎么命名按 pt-tools 的订阅与媒体库；要追完一季接着追下一季，在 pt-tools 里「修改」这条追剧，打开「追完以后接着追下一季」。

pt-tools 里一部电影只有一条订阅，不分 4K 与普通：不要把它同时设成普通服务器与 4K 服务器（勾了 4K Server 的那台）。

## Seerr 请求以后

- **电影**：建一条电影订阅，来源写「Seerr」，质量档案与媒体库按 Seerr 里选的。pt-tools 里已经有这部电影的订阅时不另建：没在找的（暂停、待确认）打开，质量档案换成 Seerr 里选的；勾了 Enable Automatic Search 时马上搜一次。库里已经整理好了这部电影时，Seerr 直接当作已有，不建订阅。
- **剧集**：请求的每一季各建一条追剧，来源写「Seerr」，按追剧的默认设置（按播出时间搜、开着版本锁）。这部剧已经有追剧时，再请求别的季，新的季用这部剧现在报给 Seerr 的质量档案与根目录（取在找的季里编号最大的那条；都没在找时取编号最大的那条），不用 Seerr 这次请求里选的；已有的季只改状态，质量档案、媒体库、保存目录、分类与下载标签都不动。第 0 季（特别篇）不建追剧。
- **状态**：Seerr 扫描时，在找的订阅没有文件是「处理中」；整理进库以后按文件数算「可用」或「部分可用」（剧集的每一季按这一季整理好的集数与 TMDB 上的集数比）。只认 pt-tools 整理进库的文件；Seerr 另外也会按 Jellyfin、Plex 里有没有判断。在 pt-tools 里暂停一条没有文件的订阅，Seerr 下次扫描时当作不再处理，会把它的请求标成拒绝（这是 Seerr 的做法）。
- **删除**：在 Seerr 里把条目从 Radarr、Sonarr 删掉，只删 pt-tools 里的订阅（剧集是这部剧的全部季），不删任何文件与种子，Seerr 发来的「同时删文件」也不理会。要连文件一起删，到 pt-tools 的[条目详情](media-detail.md#删除一部片)里删。

## 不支持的

- 按集开关监控：pt-tools 按整季追，Seerr 只在把季设成监控以后为没监控的集调这个接口，结果一样。要不追某一集，在追剧详情里点这一集「跳过这一集」。
- 下载进度：Seerr 里不显示进度，下载队列总是空的。
- TMDB 上没有 TVDB 编号的剧：Seerr 不会把它发给 Sonarr（这是 Seerr 的限制），pt-tools 也不把这类剧列给 Seerr。
- Radarr、Sonarr 的其他接口（日历、历史、导入、改设置等）回 404，未知的命令回 400。

## 审计与安全

- 写操作（新建、修改、删除订阅与标签，发命令）都记在「ChatOps → 操作审计」里，通道是「Servarr 兼容」，触发用户是令牌编号，写明请求了哪些季、建了还是改了。Seerr 每分钟发一次的刷新下载命令不记。
- Seerr 把令牌放在地址的查询参数 `apikey` 里（也认请求头 `X-Api-Key`）。pt-tools 的访问日志只记路径，不记查询参数；Seerr 与 pt-tools 不在同一台机器、中间经过别的网络时，走 HTTPS 反向代理。
- 同一个地址 1 分钟内 Key 错 10 次，之后 5 分钟内直接拒绝。撤销令牌以后马上就连不上了。
- 这个入口只认令牌，不认网页的登录；只勾了「Servarr 兼容」的令牌只能访问 `/servarr/`，访问不了 App 接口与网页的其他功能。
