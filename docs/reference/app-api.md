# App API

App API 是给手机 App 和脚本用的接口，地址都以 `/api/app/v1` 开头。它只开放 App 要用的功能：回应里没有站点 Cookie、API Key、Passkey、密码、RSS 地址与下载链接，错误信息里的网址也去掉了查询串。

## 认证

每个请求带上 `Authorization: Bearer <令牌>`。令牌在「系统 → API 令牌」新建，见 [API 令牌](../guide/api-tokens.md)。浏览器里已经登录的网页会话也能调用这些接口，拥有全部权限。

```bash
curl -H "Authorization: Bearer $PTT_TOKEN" https://pt-tools.example.com/api/app/v1/overview
```

- 没带令牌，或者令牌不对、已过期、已撤销：回 401。
- 令牌没有接口要的权限：回 403。下文的「读取」是权限 `app:read`，「操作」是 `app:write`。
- 令牌只能调用 App API：网页的其他接口（设置、站点、下载器、通知通道、令牌管理等）不认令牌，没有登录时回 401。

## 约定

- 请求体与回应都是 JSON。请求体最多 1 MiB；有接口不认识的字段时回 400，不会忽略。
- 不带请求体的写接口（签到、立即搜索、删除订阅）可以不带请求体，或者带 `{}`；带别的内容回 400。
- 时间是 Unix 时间戳（秒）。大小、上传量与下载量的单位是字节，速度的单位是字节每秒，进度是 0 到 100 的百分数。
- 分页的接口接受查询参数 `page`（从 1 开始）与 `page_size`（默认 20，上限见各接口），回应是 `{"items": [...], "total": 总数, "page": 页码, "page_size": 每页条数}`。
- 兼容性：`GET /meta` 返回的 `remote_api_level` 现在是 1，只在有不兼容的改动时加一。新增接口或字段不算不兼容，客户端应忽略不认识的字段。

## 错误

出错时回应 `{"error": "代码", "message": "说明"}`。`message` 是写给人看的中文说明，客户端按 `error` 判断。

| `error`                                                         | 状态码 | 含义                                               |
| --------------------------------------------------------------- | ------ | -------------------------------------------------- |
| `invalid_body`                                                  | 400    | 请求体不是合法的 JSON，或者有不认识的字段          |
| `invalid_argument`                                              | 400    | 参数不对，`message` 写明是哪一个                   |
| `unauthorized`                                                  | 401    | 没有带有效的令牌                                   |
| `forbidden`                                                     | 403    | 令牌没有这个权限                                   |
| `not_found`                                                     | 404    | 接口、站点或订阅不存在                             |
| `method_not_allowed`                                            | 405    | 接口存在，但不接受这个方法；`Allow` 头写明接受哪些 |
| `busy`                                                          | 409    | 同一件事正在进行，例如这个站点正在签到             |
| `rate_limited`                                                  | 429    | TMDB 限流，稍后再试                                |
| `internal`                                                      | 500    | pt-tools 内部出错                                  |
| `search_failed`、`download_failed`、`attend_failed`、`upstream` | 502    | 搜索、下载种子文件、签到或访问 TMDB 失败           |
| `unavailable`                                                   | 503    | 对应的服务没有启动                                 |

## 接口一览

| 方法   | 路径                         | 权限 | 用途                                         |
| ------ | ---------------------------- | ---- | -------------------------------------------- |
| GET    | `/meta`                      | 读取 | 版本、兼容级别、功能组与调用者自己的权限     |
| GET    | `/overview`                  | 读取 | 已启用站点的合计与今天的增量                 |
| GET    | `/sites`                     | 读取 | 站点、登录状态、今天的签到与站点上的用户数据 |
| POST   | `/sites/{site}/attend`       | 操作 | 立即签到                                     |
| GET    | `/favicon/{site}`            | 读取 | 站点图标                                     |
| GET    | `/torrents`                  | 读取 | 下载器里的种子                               |
| GET    | `/downloaders`               | 读取 | 下载器与它们现在的速度、剩余空间             |
| POST   | `/torrents/actions`          | 操作 | 暂停、继续、删除种子                         |
| POST   | `/search`                    | 读取 | 多站搜索                                     |
| POST   | `/push`                      | 操作 | 把搜索到的种子推送到下载器                   |
| GET    | `/tasks`                     | 读取 | 任务列表（推送记录）                         |
| GET    | `/brush/tasks`               | 读取 | 刷流任务                                     |
| GET    | `/media/history`             | 读取 | 整理历史                                     |
| GET    | `/subscriptions`             | 读取 | 订阅列表                                     |
| GET    | `/subscriptions/{id}`        | 读取 | 订阅详情：每一集与下载过的种子               |
| POST   | `/subscriptions`             | 操作 | 新建订阅                                     |
| POST   | `/subscriptions/{id}/status` | 操作 | 暂停或恢复订阅                               |
| POST   | `/subscriptions/{id}/search` | 操作 | 立即搜索                                     |
| DELETE | `/subscriptions/{id}`        | 操作 | 删除订阅                                     |
| GET    | `/explore`                   | 读取 | 探索：TMDB 的热门、流行与搜索                |
| GET    | `/updates`                   | 读取 | 有没有新版本                                 |

路径都省略了 `/api/app/v1` 前缀。搜索用 POST 传条件，但只要「读取」权限。令牌调用「操作」接口（包括因为权限不够被拒的）都记在「ChatOps → 操作审计」里，通道是「API 令牌」；推送被拦下、批量动作有失败时，即使回应是 200 也记为出错。

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
    "updates"
  ],
  "principal": { "kind": "api_token", "name": "我的手机", "scopes": ["app:read", "app:write"] }
}
```

`principal.kind` 是 `api_token`（令牌）或 `session`（网页登录）；`features` 是这个版本提供的功能组，较早的版本没有 `downloaders` 与 `updates`；`scopes` 是调用者的权限，客户端可以据此决定显示哪些操作。

### GET /updates

有没有新版本：`current_version`、`has_update`、`new_releases`（每项有 `version`、`name`、`published_at`、`url`、`changelog`，预览版另有 `prerelease: true`）、`checked_at`。带 `include_prerelease=1` 时也报预览版。结果缓存一段时间，不会每次都去问 GitHub；只看不升级。查不了、又没有上次的结果时回 502（`upstream`）。

## 概览与站点

### GET /overview

- `totals` 是已启用站点的合计：`uploaded`、`downloaded`、`ratio`、`seeding`、`leeching`、`bonus`、`bonus_per_hour`、`seeding_size`、`site_count`、`unread_messages`。
- `today` 是今天的增量：`from`、`to`、`uploaded`、`downloaded`、`bonus`，`sites` 是各站点的增量。口径与[数据统计](../guide/user-stats.md)相同。增量算不出来时 `today.error` 写明原因，这时的 0 不代表今天没有流量。
- `updated_at` 是站点数据最近一次更新的时间。

### GET /sites

回应是 `{"items": [...], "user_error": "…"}`：`items` 每个站点一项；站点上的用户数据没读到时 `user_error` 写明原因，各项都没有 `user`。

| 字段                              | 内容                                                                                                                                                                                                                   |
| --------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `name`、`display_name`、`enabled` | 站点标识、显示名与是否启用                                                                                                                                                                                             |
| `login`                           | 登录状态：`tier` 是提醒档位（`none`、`30d`、`14d`、`7d`、`3d`、`banned-imminent`，没有访问记录时是 `unknown`），`days_remaining` 是离封号阈值还剩几天，另有 `last_active_at`、`last_probe_at`、`last_probe_status`     |
| `attendance`                      | 今天的签到：`supported`、`enabled`、`day`、`status`（`pending`、`signed`、`already`、`failed`、`unsupported`）、`message`、`last_attempt_at`                                                                           |
| `user`                            | 站点上的用户数据，没有同步过时没有这一项：`username`、`level`、`uploaded`、`downloaded`、`ratio`、`bonus`、`bonus_per_hour`、`seeding`、`leeching`、`seeding_size`、`unread_messages`、`hnr_unsatisfied`、`updated_at` |

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

这一组接口对应网页上的「媒体」，用法见[订阅](../guide/media-subscribe.md)与[整理入库](../guide/media-library.md)。新建订阅与探索要用到 TMDB，没有填写 API Key 时回 400，见[填写 TMDB API Key](../guide/media-recognize.md#填写-tmdb-api-key)。

### GET /media/history

整理历史，最近更新的在前。参数 `status`（`done`、`failed`、`skipped`、`removed`）、`q`（关键字）、`page`、`page_size`（最多 100）。每项有 `id`、`media_type`、`tmdb_id`、`title`、`year`、`season`、`episode`、`episode_end`、`torrent_name`、`library`、`target_path`、`mode`、`size`、`status`、`message`、`created_at`。

### GET /subscriptions

参数 `status`（`active`、`paused`、`pending`、`done`）与 `q`（关键字）。每项有：

- `id`、`media_type`（`movie` 或 `tv`）、`tmdb_id`、`season`、`title`、`original_title`、`year`、`total_episodes`；
- `poster_path`：TMDB 的海报路径，图片地址是 `https://image.tmdb.org/t/p/w342` 接上这个路径（`w342` 可以换成 TMDB 支持的其他尺寸）；
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
