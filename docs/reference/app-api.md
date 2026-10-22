# App API <Badge type="tip" text="1.0 预览版" />

App API 是给手机 App 和脚本用的接口，地址都以 `/api/app/v1` 开头。它只开放 App 要用的功能：回应里没有站点 Cookie、API Key、Passkey、密码、RSS 地址与下载链接，错误信息里的网址也去掉了查询串。[配置接口](#配置)是例外：字段名照网页的接口（会有 `password`、`cookie` 这些键），密钥的值换成 `<已设置>` 或空串，要另外的「配置」权限。

## 认证

每个请求带上 `Authorization: Bearer <令牌>`。令牌在「系统 → API 令牌」新建，见 [API 令牌](../guide/api-tokens.md)。浏览器里已经登录的网页会话也能调用这些接口，拥有全部权限。

```bash
curl -H "Authorization: Bearer $PTT_TOKEN" https://pt-tools.example.com/api/app/v1/overview
```

- 没带令牌，或者令牌不对、已过期、已撤销：回 401。
- 令牌没有接口要的权限：回 403。下文的「读取」是权限 `app:read`，「操作」是 `app:write`，「配置」是 `app:config`（配置接口另外要它）。
- 令牌只能调用 App API：网页的其他接口（设置、站点、下载器、通知通道、令牌管理等）不认令牌，没有登录时回 401。
- 经[远程访问](../guide/remote-access.md)配对的设备在加密的隧道里调用这些接口，不带令牌；权限是配对时选的「完全控制」（读取与操作）或「只读」（读取），都没有「配置」。隧道里只有 App API，网页的其他接口在那里不存在。

## 约定

- 请求体与回应都是 JSON。请求体最多 1 MiB；有接口不认识的字段时回 400，不会忽略。
- 不带请求体的写接口（签到、立即搜索、删除订阅）可以不带请求体，或者带 `{}`；带别的内容回 400。
- 时间是 Unix 时间戳（秒）。大小、上传量与下载量的单位是字节，速度的单位是字节每秒，进度是 0 到 100 的百分数。
- 分页的接口接受查询参数 `page`（从 1 开始）与 `page_size`（默认 20，上限见各接口），回应是 `{"items": [...], "total": 总数, "page": 页码, "page_size": 每页条数}`。
- 兼容性：`GET /meta` 返回的 `remote_api_level` 现在是 1，只在有不兼容的改动时加一。新增接口或字段不算不兼容，客户端应忽略不认识的字段。
- 契约：机器可读的描述是仓库里的 [app-api-v1.yaml](https://github.com/sunerpy/pt-tools/blob/v1.0.0-rc/docs/reference/app-api-v1.yaml)（OpenAPI 3.0）。pt-tools 的测试用它核对每个接口的真实回应，手机 App 的 Dart 模型也由它生成。

## 错误

出错时回应 `{"error": "代码", "message": "说明"}`。`message` 是写给人看的中文说明，客户端按 `error` 判断。

| `error`                                                         | 状态码   | 含义                                                                                     |
| --------------------------------------------------------------- | -------- | ---------------------------------------------------------------------------------------- |
| `invalid_body`                                                  | 400      | 请求体不是合法的 JSON，或者有不认识的字段                                                |
| `invalid_argument`                                              | 400      | 参数不对，`message` 写明是哪一个                                                         |
| `unauthorized`                                                  | 401      | 没有带有效的令牌                                                                         |
| `forbidden`                                                     | 403      | 令牌没有这个权限                                                                         |
| `not_found`                                                     | 404      | 接口、站点或订阅不存在                                                                   |
| `method_not_allowed`                                            | 405      | 接口存在，但不接受这个方法；`Allow` 头写明接受哪些                                       |
| `busy`                                                          | 409、503 | 同一件事正在进行（409，例如这个站点正在签到）；取图片的请求太多（503，带 `Retry-After`） |
| `conflict`                                                      | 409      | 改配置时保存它的接口回了冲突（例如刷流任务正在运行一轮），稍后再试                       |
| `rate_limited`                                                  | 429      | TMDB 限流，稍后再试                                                                      |
| `internal`                                                      | 500      | pt-tools 内部出错                                                                        |
| `search_failed`、`download_failed`、`attend_failed`、`upstream` | 502      | 搜索、下载种子文件、签到、访问 TMDB 失败，或者改配置时保存它的接口出错                   |
| `unavailable`                                                   | 503      | 对应的服务没有启动                                                                       |
| `assistant_disabled`、`decided`                                 | 409      | AI 助手没有启用；这个待确认操作已经有了结论                                              |
| `too_many`                                                      | 429      | AI 助手同时在回答的问题太多                                                              |

## 接口一览

| 方法   | 路径                                     | 权限        | 用途                                         |
| ------ | ---------------------------------------- | ----------- | -------------------------------------------- |
| GET    | `/meta`                                  | 读取        | 版本、兼容级别、功能组与调用者自己的权限     |
| GET    | `/overview`                              | 读取        | 已启用站点的合计与今天的增量                 |
| GET    | `/sites`                                 | 读取        | 站点、登录状态、今天的签到与站点上的用户数据 |
| POST   | `/sites/{site}/attend`                   | 操作        | 立即签到                                     |
| GET    | `/favicon/{site}`                        | 读取        | 站点图标                                     |
| GET    | `/torrents`                              | 读取        | 下载器里的种子                               |
| GET    | `/downloaders`                           | 读取        | 下载器与它们现在的速度、剩余空间             |
| POST   | `/torrents/actions`                      | 操作        | 暂停、继续、删除种子                         |
| POST   | `/search`                                | 读取        | 多站搜索                                     |
| POST   | `/push`                                  | 操作        | 把搜索到的种子推送到下载器                   |
| GET    | `/tasks`                                 | 读取        | 任务列表（推送记录）                         |
| GET    | `/brush/tasks`                           | 读取        | 刷流任务                                     |
| GET    | `/media/history`                         | 读取        | 整理历史                                     |
| POST   | `/media/history/{id}/retry`              | 操作        | 重新整理一条整理记录对应的种子               |
| GET    | `/media/libraries`                       | 读取        | 媒体库                                       |
| POST   | `/media/recognize`                       | 读取        | 识别一个种子标题                             |
| POST   | `/media/organize/preview`                | 读取        | 预览整理下载器里的一个种子                   |
| POST   | `/media/organize`                        | 操作        | 把下载器里的一个种子整理进媒体库             |
| GET    | `/subscriptions`                         | 读取        | 订阅列表                                     |
| GET    | `/subscriptions/{id}`                    | 读取        | 订阅详情：每一集与下载过的种子               |
| POST   | `/subscriptions`                         | 操作        | 新建订阅                                     |
| POST   | `/subscriptions/{id}/status`             | 操作        | 暂停或恢复订阅                               |
| POST   | `/subscriptions/{id}/search`             | 操作        | 立即搜索                                     |
| DELETE | `/subscriptions/{id}`                    | 操作        | 删除订阅                                     |
| GET    | `/explore`                               | 读取        | 探索：TMDB 的热门、流行与搜索                |
| GET    | `/images/tmdb/{size}/{file}`             | 读取        | TMDB 图片（海报），由 pt-tools 去取          |
| GET    | `/updates`                               | 读取        | 有没有新版本                                 |
| GET    | `/assistant/status`                      | 读取        | AI 助手启用没有、用的模型、能不能确认        |
| GET    | `/assistant/conversations`               | 读取        | AI 助手的会话                                |
| POST   | `/assistant/conversations`               | 读取        | 新建会话，可以接着发第一条消息               |
| GET    | `/assistant/conversations/{id}`          | 读取        | 一个会话的消息与待确认操作                   |
| DELETE | `/assistant/conversations/{id}`          | 读取        | 删除会话                                     |
| POST   | `/assistant/conversations/{id}/messages` | 读取        | 发消息，在后台开始回答                       |
| POST   | `/assistant/conversations/{id}/cancel`   | 读取        | 停止回答                                     |
| GET    | `/assistant/conversations/{id}/events`   | 读取        | 长轮询会话的进度                             |
| POST   | `/assistant/confirmations/{id}`          | 操作        | 确认或拒绝一个改动类操作                     |
| GET    | `/templates`                             | 读取        | 消息模板：事件、变量与现在的模板             |
| POST   | `/templates/preview`                     | 读取        | 用示例数据渲染一份模板                       |
| PUT    | `/templates/{type}`                      | 操作        | 保存一种事件的模板                           |
| DELETE | `/templates/{type}`                      | 操作        | 恢复默认                                     |
| GET    | `/config`                                | 配置 + 读取 | 配置目录：能读改的各类配置、操作与字段       |
| GET    | `/config/{resource}`                     | 配置 + 读取 | 一类配置的现状（密钥遮住）                   |
| POST   | `/config/{resource}/preview`             | 配置 + 读取 | 预览一次改动：改以前、改以后与影响           |
| POST   | `/config/{resource}/changes`             | 配置 + 操作 | 保存一次改动                                 |

路径都省略了 `/api/app/v1` 前缀。搜索、识别、整理预览、模板预览与配置预览用 POST 传内容，但只要「读取」权限。令牌调用「操作」接口（包括因为权限不够被拒的）都记在「ChatOps → 操作审计」里，通道是「API 令牌」；推送被拦下、批量动作有失败时，即使回应是 200 也记为出错。

## 基本信息

### GET /meta

```json
{
  "name": "pt-tools",
  "version": "v1.0.0",
  "remote_api_level": 1,
  "features": [
    "overview",
    "sites",
    "torrents",
    "downloaders",
    "tasks",
    "search",
    "push",
    "attendance",
    "brush",
    "media",
    "subscriptions",
    "updates",
    "assistant",
    "templates"
  ],
  "principal": { "kind": "api_token", "name": "我的手机", "scopes": ["app:read", "app:write"] }
}
```

`principal.kind` 是 `api_token`（令牌）、`session`（网页登录）或 `remote_device`（经远程访问配对的设备）；`features` 是这个版本提供的功能组，较早的版本没有 `downloaders`、`updates`、`assistant` 与 `templates`；`scopes` 是调用者的权限，客户端可以据此决定显示哪些操作。

### GET /updates

有没有新版本：`current_version`、`has_update`、`new_releases`（每项有 `version`、`name`、`published_at`、`url`、`changelog`，预览版另有 `prerelease: true`）、`checked_at`。带 `include_prerelease=1` 时也报预览版。结果缓存一段时间，不会每次都去问 GitHub；只看不升级。查不了、又没有上次的结果时回 502（`upstream`）。

## 概览与站点

### GET /overview

- `totals` 是已启用站点的合计：`uploaded`、`downloaded`、`ratio`、`seeding`、`leeching`、`bonus`、`bonus_per_hour`、`seeding_size`、`site_count`、`unread_messages`。
- `today` 是今天的增量：`from`、`to`、`uploaded`、`downloaded`、`bonus`，`sites` 是各站点的增量。口径与[数据统计](../guide/user-stats.md)相同。增量算不出来时 `today.error` 写明原因，这时的 0 不代表今天没有流量。
- `updated_at` 是站点数据最近一次更新的时间。
- 这里的 `downloaded` 都是站点计费的下载量：站点按免费、折扣折算以后计入分享率的数，不是实际传输量。`today` 的增量来自每日快照，快照只存计费口径。见[下载量的两种口径](../guide/user-stats.md#下载量的两种口径)。

### GET /sites

回应是 `{"items": [...], "user_error": "…"}`：`items` 每个站点一项；站点上的用户数据没读到时 `user_error` 写明原因，各项都没有 `user`。

| 字段                              | 内容                                                                                                                                                                                                                   |
| --------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `name`、`display_name`、`enabled` | 站点标识、显示名与是否启用                                                                                                                                                                                             |
| `login`                           | 登录状态：`tier` 是提醒档位（`none`、`30d`、`14d`、`7d`、`3d`、`banned-imminent`，没有访问记录时是 `unknown`），`days_remaining` 是离封号阈值还剩几天，另有 `last_active_at`、`last_probe_at`、`last_probe_status`     |
| `attendance`                      | 今天的签到：`supported`、`enabled`、`day`、`status`（`pending`、`signed`、`already`、`failed`、`unsupported`）、`message`、`last_attempt_at`                                                                           |
| `user`                            | 站点上的用户数据，没有同步过时没有这一项：`username`、`level`、`uploaded`、`downloaded`、`ratio`、`bonus`、`bonus_per_hour`、`seeding`、`leeching`、`seeding_size`、`unread_messages`、`hnr_unsatisfied`、`updated_at` |

`user` 里的 `downloaded` 同样是计费下载量，不是实际传输量。

### POST /sites/{site}/attend

立即签到，回应与 `/sites` 里的 `attendance` 相同。这个站点正在签到时回 409。

### GET /favicon/{site}

站点图标，与网页里显示的相同。回应是图片，不是 JSON。

## 下载器

### GET /downloaders

启用的下载器：`id`、`name`、`type`（`qbittorrent` 或 `transmission`）、`default`、`reachable`（这一次读到了状态或剩余空间）、`error`（读不到时的原因）、`version`、`upload_speed`、`download_speed`、`uploaded`、`downloaded`（下载器自己记的累计量）、`free_space`。没有下载器的地址、账号与密码。

### GET /torrents

| 参数                | 说明                                                                                                                                                                            |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `downloader_id`     | 只看这一台下载器                                                                                                                                                                |
| `state`             | `downloading`、`seeding`、`paused`、`stopped`、`queued`、`checking` 或 `error`                                                                                                  |
| `q`                 | 标题里的关键字                                                                                                                                                                  |
| `sort`、`order`     | 排序字段（`added_at`、`completed_at`、`title`、`progress`、`size`、`ratio`、`state`、`upload_speed`、`download_speed`、`eta`、`seeds`）与 `asc`、`desc`；默认按添加时间从新到旧 |
| `page`、`page_size` | `page_size` 最多 200                                                                                                                                                            |

每项有 `downloader_id`、`downloader`、`task_id`、`info_hash`、`title`、`progress`、`size`、`state`、`ratio`、`seeds`、`peers`、`upload_speed`、`download_speed`、`eta`（预计剩余秒数，-1 表示无法估计）、`added_at`、`completed_at`、`category`、`tags`、`save_path`。`failures` 列出没有取到种子的下载器（`downloader_id`、`downloader`、`error`）；它不为空时列表不完整。

### POST /torrents/actions

```json
{ "action": "pause", "targets": [{ "downloader_id": 1, "task_id": "取自 /torrents 的 task_id" }] }
```

`action` 是 `pause`（暂停）、`resume`（继续）、`delete`（删除种子，保留数据）或 `delete_with_files`（连数据一起删除）；`targets` 有 1 到 100 个。回应有 `succeeded`、`failed` 与每个种子的 `results`（`downloader_id`、`task_id`、`success`、`message`）。

## 搜索与推送

### POST /search

| 字段          | 说明                                                                        |
| ------------- | --------------------------------------------------------------------------- |
| `keyword`     | 必填，最多 100 个字                                                         |
| `sites`       | 只搜这些站点，最多 100 个；不填时搜所有启用的站点，列出的站点都没启用时不搜 |
| `category`    | 分类                                                                        |
| `free_only`   | 只要免费的                                                                  |
| `min_seeders` | 做种人数下限                                                                |

回应：

- `items` 是结果，每项有 `site`、`torrent_id`、`title`、`subtitle`、`info_hash`、`size`、`seeders`、`leechers`、`snatched`、`uploaded_at`、`category`、`tags`、`discount`、`discount_end_at`、`free`、`has_hr`、`imdb_id`、`douban_id`。没有详情页与下载链接，推送用 `POST /push`。
- `sites` 是各站点的结果数，`errors` 是出错的站点（`site`、`error`），`duration_ms` 是耗时。一次搜索最多等 30 秒。

### POST /push

把一个种子推送到下载器：pt-tools 从站点下载种子文件，再和 RSS 推送一样经过磁盘空间保护与站点做种容量的检查。

| 字段                                     | 说明                                                                           |
| ---------------------------------------- | ------------------------------------------------------------------------------ |
| `site`、`torrent_id`                     | 必填，取自搜索结果                                                             |
| `downloader_id`                          | 推送到哪台下载器；不填时用默认下载器，没有默认下载器时用第一台启用的           |
| `title`、`category`、`tags`、`save_path` | 可选：记录里的标题（不填时用种子文件里的名字），下载器里的分类、标签与保存目录 |

回应有 `success`、`skipped`（下载器里已经有这个种子）、`message`、`info_hash`、`downloader_id`、`downloader`。被磁盘空间保护或站点做种容量拦下时 `success` 是 `false`、`message` 写明原因，状态码仍是 200；种子文件下载失败时回 502。推送前搜索过这个种子时，搜索结果里的体积、H&R、免费与免费到期会一并记进任务记录（H&R 保护与免费到期清理要用）。

## 任务列表

### GET /tasks

推送记录，按新到旧排列。

| 参数                | 说明                  |
| ------------------- | --------------------- |
| `site`              | 只看这个站点          |
| `q`                 | 标题里的关键字        |
| `pushed`            | 填 `1` 时只看已推送的 |
| `page`、`page_size` | `page_size` 最多 100  |

每项有 `id`、`site`、`torrent_id`、`title`、`size`、`category`、`tags`、`free`、`free_level`、`free_end_at`、`has_hr`、`pushed`、`pushed_at`、`downloader`、`progress`、`completed`、`completed_at`、`source`、`error`、`created_at`。`source` 是记录的来源：RSS 是 `free_download` 或 `filter_rule`，其余有 `manual_push`（网页推送）、`app_push`（App 推送）、`brush`（刷流）、`subscription`（订阅）、`transfer`（转移做种）、`reseed`（辅种）。

## 刷流

### GET /brush/tasks

每个刷流任务一项：`id`、`name`、`enabled`、`site`、`site_enabled`、`downloader`、`active`（在刷的种子数）、`downloading`（其中还在下载的）、`active_size`，以及 `today` 与 `total` 的收益（`uploaded`、`downloaded`、`added`、`removed`）。

## 媒体

这一组接口对应网页上的「媒体」，用法见[订阅](../guide/media-subscribe.md)与[整理入库](../guide/media-library.md)。新建订阅与探索要用到 TMDB，没有能用的 API Key 时回 400，见 [TMDB API Key](../guide/media-recognize.md#tmdb-api-key)。

### GET /media/history

整理历史，最近更新的在前。参数 `status`（`done`、`failed`、`skipped`、`removed`）、`q`（关键字）、`page`、`page_size`（最多 100）。每项有 `id`、`media_type`、`tmdb_id`、`title`、`year`、`season`、`episode`、`episode_end`、`torrent_name`、`library`、`target_path`、`mode`、`size`、`status`、`message`、`created_at`。

### POST /media/history/{id}/retry

重新整理这条记录对应的种子（多半是失败的那条），手动整理时指定过的条目与媒体库照旧。回应同 `POST /media/organize`。记录不存在时回 404。

### GET /media/libraries

每个媒体库一项：`id`、`name`、`kind`（`movie` 或 `tv`）、`anime`（只收动画）、`path`（pt-tools 里看到的库目录）、`mode`（整理方式，空的是整理设置里的默认方式）、`scrape`（刮削）、`classify_dir`（按二级分类放进子目录）、`overwrite_mode`（空的不替换旧版本，`larger` 新文件更大时替换）、`enabled`。

### POST /media/recognize

`{"title": "…", "subtitle": "…", "imdb_id": "tt…"}`：种子标题与副标题至少给一个（各最多 512 个字），`imdb_id` 是站点给的 IMDb 编号，可选。按[媒体识别](../guide/media-recognize.md)的规则（手动纠正、识别词、IMDb 编号、按名字搜索）识别，不改动任何东西。回应：

- `summary`：给人看的一句话（名字、年份、季集）；`meta`：从标题解析出来的 `name_cn`、`name_en`、`year`、`type`、`season`、`episode`、`episode_end`、`resolution`、`source`、`video_codec`、`hdr`、`audio`、`group` 等；
- `match`：TMDB 上对应的条目（`id`、`media_type`、`title`、`original_title`、`year`、`overview`、`poster_path`、`vote_average`、`imdb_id`），没有可靠的匹配时没有；`source`：`override`（手动纠正过）、`imdb`、`search` 或 `none`；`score`；
- `candidates`：最多 5 个候选（带 `score`）；`message`：提示（例如没有能用的 TMDB API Key，只做了标题解析）；`error`：TMDB 出错的原因。

### POST /media/organize/preview

`{"downloader_id": 1, "hash": "…"}`：下载器里的一个种子，`hash` 用 `GET /torrents` 的 `info_hash`（没有时用 `task_id`）。可以加 `media_type` 与 `tmdb_id`（一起给）指定 TMDB 条目，加 `library_id` 指定媒体库；不给时自动识别、按类型选库。只算整理计划，不改动任何东西。回应：

- `downloader_id`、`downloader`、`hash`、`name`（种子名）、`match`（同上）、`library`（`id`、`name`）、`category`（二级分类）、`mode`；
- `items`：每个视频一项，`rel`（种子里的相对路径）、`size`、`target`（放进库里的位置）、`season`、`episode`、`episode_end`、`status`（`pending` 要整理、`done` 库里已经有了、`exists` 目标被别的文件占着、`replace` 换掉 pt-tools 之前放进库的旧版本、`failed` 整理不了）、`message`；
- `skipped`：不整理的文件（样片、太小的、不是视频的）有几个；`problem`：不为空时整个种子整理不了（例如找不到文件、要设路径映射）。

### POST /media/organize

参数同预览。按媒体库的方式（硬链接、复制或移动）整理，刮削，通知媒体服务器扫描，和网页上的手动整理一样。回应 `plan`（同预览）、`created`（这次整理出的文件数）、`done`、`skipped`、`failed`、`messages`、`queued`（还在后台进行，稍后看整理历史）。全部失败时回应仍是 200，操作审计里记为出错。一次整理最多等 60 秒，超时回 504（`timeout`），整理照样在后台进行。

### GET /subscriptions

参数 `status`（`active`、`paused`、`pending`、`done`）与 `q`（关键字）。每项有：

- `id`、`media_type`（`movie` 或 `tv`）、`tmdb_id`、`season`、`title`、`original_title`、`year`、`total_episodes`；
- `poster_path`：TMDB 的海报路径，经 `GET /images/tmdb/w342/<去掉开头 / 的路径>` 取图（`w342` 也可以换成下文列出的其他尺寸），见下文；
- `status`、`upgrade`（是否洗版）、`source`（`manual`、`explore`、`douban`、`chatops`、`app`）、`message`、`last_search_at`、`next_search_at`、`created_at`；
- `progress`：`total`、`aired`、`in_library`、`downloading` 与 `missing`（缺的集号）。

### GET /subscriptions/{id}

订阅本身的字段，加上：

- `episodes`：剧集的每一集，`number`、`name`、`air_date`、`state`（`library`、`downloading`、`missing` 或 `upcoming`）；
- `torrents`：下载过的种子，`site`、`torrent_id`、`title`、`status`（`downloading`、`done`、`failed`、`replaced`）、`episode`、`episode_end`、`complete`、`size`、`message`、`created_at`。

### POST /subscriptions

| 字段         | 说明                                   |
| ------------ | -------------------------------------- |
| `media_type` | 必填，`movie` 或 `tv`                  |
| `tmdb_id`    | 必填，TMDB 编号                        |
| `season`     | 剧集的季号；不填时订最新一季           |
| `profile_id` | 质量档案；不填时用设置里的默认质量档案 |
| `upgrade`    | 是否洗版                               |

回应是新建的订阅，来源记为 `app`。已经订阅过时回 400，TMDB 上找不到时回 404。

### POST /subscriptions/{id}/status

`{"status": "paused"}` 暂停，`{"status": "active"}` 恢复，恢复后会尽快搜索一次。回应是更新后的订阅。

### POST /subscriptions/{id}/search

立即搜索一次，回应 `{"message": "说明"}`。暂停的订阅回 400。

### DELETE /subscriptions/{id}

删除订阅，回应 `{"ok": true}`。下载器里的种子与媒体库里的文件不动。

### GET /explore

| 参数   | 说明                                                                  |
| ------ | --------------------------------------------------------------------- |
| `kind` | 必填，`movie` 或 `tv`                                                 |
| `list` | `trending`（热门，默认）、`popular`（流行）或 `search`（按 `q` 搜索） |
| `q`    | `list=search` 时的搜索词，最多 100 个字                               |
| `page` | 从 1 开始，热门与流行最多 20 页；搜索只有一页                         |

回应有 `items`、`page`、`total_pages`。每项有 `id`（TMDB 编号）、`media_type`、`title`、`original_title`、`year`、`overview`、`poster_path`、`vote_average`、`in_library`（已入库）、`subscribed` 与 `subscription_id`。

### GET /images/tmdb/{size}/{file}

TMDB 的图片：`file` 是 `poster_path` 去掉开头的 `/`，`size` 是海报尺寸 `w92`、`w154`、`w185`、`w342`、`w500`、`w780` 之一（不提供原图，原图可能有好几 MB）。pt-tools 按「媒体识别」里设置的图片地址与代理去取，回应是图片，带一天的缓存头；客户端因此不用直接连 TMDB（国内常常连不上）。取图片不需要 TMDB API Key。TMDB 上没有这张图时回 404，取不到时回 502（`upstream`）。pt-tools 同时最多取 4 张图，排队超过 10 秒回 503（`busy`，带 `Retry-After`）。

## AI 助手

这一组接口对应网页上的「AI 助手」，用法见 [AI 助手](../guide/assistant.md)。设置（接哪家大模型、API Key）只能在网页上改，这里没有。会话按调用者分开：每个令牌、每台配对的设备只看得到自己建的会话，网页登录看到的是网页上的会话。对话本身只要「读取」权限；模型要做改动类操作时，只有带「操作」权限的调用者能确认（只读的调用者，模型拿不到这些工具）。时间是 RFC 3339 格式的字符串。

### GET /assistant/status

回应 `enabled`（启用了，而且接口地址与模型都配好了）、`model`、`can_confirm`（调用者能不能确认改动类操作）、`chatops`。不含 API Key 与接口地址。主机的版本没有这一组接口时，`GET /meta` 的 `features` 里没有 `assistant`。

### GET /assistant/conversations

调用者自己的会话，最近更新的在前，最多 100 个。每项有 `id`、`title`、`entry`（在哪里建的：`web`、`app`、`chatops`）、`running`、`created_at`、`updated_at`。

### POST /assistant/conversations

`{"title": "…", "message": "…"}`，两项都可选；带了 `message` 时还可以带 `model` 与 `effort`（同发消息）。只建会话时回 201；带了 `message` 时接着发这一条，回 202，`run` 里有这次运行的 `run_id` 与落库的用户消息。没有启用 AI 助手时回 409（`assistant_disabled`），这时也不留下空会话。

### GET /assistant/conversations/{id}

会话本身的字段，加上：

- `messages`：最近 200 条消息，按先后排。每条有 `id`、`run_id`、`role`（`user`、`assistant`、`tool`、`notice`）、`content`、`created_at`；助手的消息带 `tool_calls`（`id`、`name`、`arguments`、`write`），模型给了推理内容时带 `reasoning`（思考过程，脱敏以后的）；工具结果带 `tool_name`、`tool_call_id`、`is_error`。用户消息与助手消息带这一轮的 `model` 与 `effort`（没有时是设置里的模型、不指定推理等级）；在网页上发的、带了附件的用户消息有 `attachments`（`id`、`name`、`kind`（`image` 或 `text`）、`mime`、`size`，内容不经 App API 给）。`notice` 是 pt-tools 自己的提示（已停止、出错、到了步数上限、模型不认推理等级、前面的对话压缩成了摘要）。
- `confirmations`：待确认操作，`id`、`run_id`、`tool_call_id`、`tool`、`summary`（给人看的一句话）、`arguments`、`status`（`pending`、`approved`、`denied`、`expired`）、`decided_by`、`decided_at`、`expires_at`、`created_at`，以及写工具的预览：`changes`（每一项的 `label`、`before`、`after`，密钥 `secret` 为真、两边不写原文）与 `impact`（可能的影响）。按 `run_id` 与 `tool_call_id` 对上消息里的调用。
- `run_id`、`partial`、`partial_reasoning`：进行中的运行与它这一步到现在的回答文字、推理内容；`last_seq`：从这里接着读事件。
- `usage`：上下文用量，`context`（下一次请求大约要带的 token）、`estimated`（按字数估算的）、`window`（上下文窗口）、`window_source`（`setting`、`provider`、`table`、`default`）、`auto_compact`、`compact_percent`、`input` 与 `output`（本会话服务商报的输入、输出 token 合计）、`compacted`（压缩过）。见[上下文与压缩](../guide/assistant.md#上下文与压缩)。

### POST /assistant/conversations/{id}/messages

`{"text": "…"}`，最多 4000 个字。可选 `model`（这一轮用的模型，不能有空格，最长 128 个字符；不给时用设置里的模型）与 `effort`（推理等级 `none`、`minimal`、`low`、`medium`、`high`、`xhigh`，发给模型服务的是 `reasoning_effort`；不给时不指定）。模型服务因为推理等级拒绝请求时，pt-tools 去掉它重试一次，会话里多一条提示。回 202，带这次运行的 `run_id` 与落库的用户消息。同一个会话正在回答时回 409（`busy`）。App API 不收附件（附件只能在网页上发）。

### POST /assistant/conversations/{id}/cancel

停止会话进行中的回答，回应 `{"ok": true}`；没有在回答时什么也不做。正在等确认的操作作废，不执行。

### GET /assistant/conversations/{id}/events

长轮询：`after` 是上次读到的序号（会话详情的 `last_seq`，或者上一页的 `last`），`wait` 是没有事件时最多等几秒（默认与上限都是 25）。回应 `events`、`reset`、`last`、`running`。`reset` 为真时中间的事件丢了（或者 pt-tools 重启过），重新读会话，从它的 `last_seq` 接着读。

每个事件有 `seq`、`type`、`at`，按种类带：

| `type`         | 内容                                                       |
| -------------- | ---------------------------------------------------------- |
| `run_started`  | `run_id`                                                   |
| `delta`        | `text`：回答的一段文字；`replace` 为真时是到现在的全部文字 |
| `message`      | `message`：落库的一条消息（字段同会话详情）                |
| `tool_started` | `tool`：开始执行的工具调用                                 |
| `confirmation` | `confirmation`：建好的待确认操作，或者有了结论             |
| `run_finished` | `status`（`done`、`canceled`、`failed`），失败时带 `error` |

只有这几种。网页的事件流里还有推理内容的增量（`reasoning`）、用量（`usage`）与开始压缩（`compacting`），App API 不给（装着旧版本的 App 认不得新的种类）：跳过它们时 `last` 照样往后走，推理内容在回答落库以后见消息的 `reasoning`，用量见会话详情的 `usage`。

### POST /assistant/confirmations/{id}

`{"approve": true}` 确认执行，`{"approve": false}` 拒绝。要「操作」权限。回应是有了结论的待确认操作；已经有结论的（别处先点了、已经超时或作废）回 409（`decided`）。确认与拒绝都记在操作审计里；执行的操作另记一条，通道是「AI 助手」，触发用户是 `token:<令牌编号>` 或 `device:<设备编号>`。

## 消息模板

这一组接口对应网页上的「ChatOps → 消息模板」，写法见[消息模板](../guide/notify-channels.md#消息模板)。模板对所有通道生效，改了就影响之后发出的每一条通知与回复，所以保存与恢复默认要「操作」权限，模板的原文也只给有「操作」权限的调用者；令牌与设备的改动都记在操作审计里。示例值、内置文字的样子与预览的结果里像凭证的内容会被遮住。主机的版本没有这一组接口时，`GET /meta` 的 `features` 里没有 `templates`。

### GET /templates

回应 `groups`、`common_vars`、`events`：

- `groups`：事件的组，`key` 是 `notify`（通知）、`reply`（ChatOps 命令回复）或 `assistant`（AI 助手回复），另有 `name`、`desc`。
- `common_vars`：每种事件都有的变量。变量有 `key`、`type`（`string`、`int`、`float`、`bool`、`list`）、`desc`、`sample`（示例值写成的文字）；列表没有示例，每一项的字段在 `item` 里。
- `events`：每种事件一项，`type`、`group`、`name`、`desc`、`no_title`（只有正文）、`vars`、`sample_title` 与 `sample_text`（不改时发出去的样子）、`title` 与 `text`（现在的模板，空着的一项用内置文字；只给有「操作」权限的调用者，只读的调用者拿到的是空的，看 `customized` 知道有没有自定义）、`customized`、`updated_at`（自定义的时间，秒级时间戳）。

### POST /templates/preview

`{"type": "rss.matched", "title": "…", "text": "…"}`，用这种事件的示例数据渲染，不保存。回应 `{"title": "…", "text": "…"}`（只有正文的事件 `title` 是空的）。模板写得不对时回 400（`invalid_argument`），`message` 写明第几行哪里不对；没有这种事件时回 404。

### PUT /templates/{type}

`{"title": "…", "text": "…"}`，保存前按预览一样校验，不对时回 400；两项都空等于恢复默认。只有正文的事件填了标题也回 400。回应是保存以后的这种事件（字段同 `events` 的一项）。

### DELETE /templates/{type}

恢复成内置文字，回应 `{"ok": true}`；没有这种事件时回 404。

## 配置

这一组接口经配置网关读改 pt-tools 的配置，和 MCP 的 `get_config`、`change_config` 以及 AI 助手的[改配置](../guide/assistant.md#改配置)是同一套。改动交给网页上保存时用的同一个接口，校验、加密与热重载都一样。都要「配置」权限（`app:config`），另外读与预览要「读取」，保存要「操作」。网页登录有全部权限；配对的设备没有「配置」，调用回 403。

不能经这里改的：登录密码、API 令牌、远程访问、加密密钥、AI 助手自己的设置（含语音输入）、ChatOps 绑定与管理员、qB 兼容入口。

### GET /config

配置目录：`resources` 是能读改的各类配置，每项有 `name`、`title`、`description`、`kind`（`object` 是一份设置，`list` 是一组）、`id_field`（列表项的编号字段）、`ops`、`fields`（能改的字段；`config.<键>` 表示以它开头的字段都能改）、`required`（新增时必填）、`secrets`（密钥字段）、`hints`（字段说明）。另有 `excluded`（不能经这里改的）与 `secret_placeholder`（`<已设置>`）。

### GET /config/{resource}

一类配置的现状：`resource`、`kind`；对象类是 `item`，列表类是 `items` 与 `id_field`。查询参数 `id` 只看列表里的一项（没有时回 404）。字段名与网页的接口一样；密钥的值是 `<已设置>`（有值）或空串，读出来就是原文的字段里地址的用户信息与查询串也遮掉了。

### POST /config/{resource}/preview

```json
{ "op": "update", "id": "2", "values": { "enabled": false } }
```

`op` 是 `set`（对象类）、`add`、`update` 或 `delete`（列表类，改与删要给 `id`）；`values` 只写要改的字段。数字、开关照现状的类型写；字符串写的数字按现状换成数字。密钥给新值才改，交回 `<已设置>` 表示不改，给空串表示清除。回应 `resource`、`op`、`id`、`summary`（一句话）、`changes`（每一项的 `label`、`field`、`before`、`after`、`secret`）、`impact`、`applied`（为假）。字段不认识、和现在一样、必填的没给时回 400（`invalid_argument`），`message` 写明能改的字段；资源或编号不存在时回 404。

### POST /config/{resource}/changes

同预览的请求体。按现状重新算一遍再保存（预览以后别处改过的照新的算），回应同预览，`applied` 为真，`item` 是改完的样子（删除时没有），新增出来的编号在 `id`。保存它的接口拒绝时照原样回原因：校验不过 400，冲突 409（`conflict`），服务没有启动 503。令牌与设备的保存记在操作审计里，参数记资源、操作、编号与改了哪些字段，不记值。
