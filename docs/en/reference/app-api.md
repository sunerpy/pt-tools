# App API <Badge type="tip" text="1.0 preview" />

The App API is the interface for the mobile app and for scripts; every address starts with `/api/app/v1`. It exposes only what an app needs: responses never contain site cookies, API keys, passkeys, passwords, RSS addresses or download links, and addresses inside error messages have their query strings removed. The [configuration endpoints](#configuration) are the exception: their field names follow the web endpoints (so keys such as `password` and `cookie` appear), secret values are replaced by `<已设置>` or an empty string, and they need the extra Config permission.

## Authentication

Send `Authorization: Bearer <token>` with every request. Tokens are created under System → API tokens (系统 → API 令牌); see [API tokens](../guide/api-tokens.md). A browser that is signed in to the web interface can also call these endpoints, with every permission.

```bash
curl -H "Authorization: Bearer $PTT_TOKEN" https://pt-tools.example.com/api/app/v1/overview
```

- No token, or a token that is wrong, expired or revoked: 401.
- A token without the permission the endpoint needs: 403. Below, Read is the permission `app:read`, Operate is `app:write` and Config is `app:config` (the configuration endpoints also need it).
- Tokens work only for the App API: the rest of the web interface (settings, sites, downloaders, notification channels, token management and so on) does not accept them and responds with 401 without a sign-in.
- A device paired through [remote access](../guide/remote-access.md) calls these endpoints inside an encrypted tunnel, without a token; its permissions are the ones chosen when it was paired, Full control (Read and Operate) or Read only (Read), neither of which includes Config. The tunnel carries only the App API; the rest of the web interface does not exist there.

## Conventions

- Request bodies and responses are JSON. A request body is limited to 1 MiB; a field the endpoint does not know is rejected with 400 rather than ignored.
- Operate endpoints without a body (sign in, search now, delete a subscription) accept no body or `{}`; anything else gets 400.
- Times are Unix timestamps in seconds. Sizes and uploaded or downloaded amounts are in bytes, speeds in bytes per second, and progress is a percentage from 0 to 100.
- Paged endpoints accept the query parameters `page` (starting at 1) and `page_size` (20 by default; each endpoint states its maximum) and respond with `{"items": [...], "total": count, "page": page, "page_size": size}`.
- Compatibility: `remote_api_level` in `GET /meta` is currently 1 and goes up only for incompatible changes. New endpoints or fields are not incompatible changes, so clients should ignore fields they do not know.
- Contract: the machine-readable description is [app-api-v1.yaml](https://github.com/sunerpy/pt-tools/blob/v1.0.0-rc/docs/reference/app-api-v1.yaml) (OpenAPI 3.0) in the repository. pt-tools' tests check every endpoint's real responses against it, and the mobile app's Dart models are generated from it.

## Errors

An error responds with `{"error": "code", "message": "explanation"}`. `message` is a human-readable explanation in Chinese; clients should act on `error`.

| `error`                                                         | Status   | Meaning                                                                                                                           |
| --------------------------------------------------------------- | -------- | --------------------------------------------------------------------------------------------------------------------------------- |
| `invalid_body`                                                  | 400      | The body is not valid JSON, or has a field that is unknown                                                                        |
| `invalid_argument`                                              | 400      | A parameter is wrong; `message` says which                                                                                        |
| `unauthorized`                                                  | 401      | No valid token                                                                                                                    |
| `forbidden`                                                     | 403      | The token lacks this permission                                                                                                   |
| `not_found`                                                     | 404      | The endpoint, site or subscription does not exist                                                                                 |
| `method_not_allowed`                                            | 405      | The endpoint exists but not with this method; the `Allow` header lists the accepted ones                                          |
| `busy`                                                          | 409, 503 | The same thing is in progress (409, such as signing in to a site); too many image requests (503, with `Retry-After`)              |
| `conflict`                                                      | 409      | When changing configuration, the endpoint that saves it reported a conflict (such as a brush task running a round)                |
| `rate_limited`                                                  | 429      | TMDB is rate limiting; try again later                                                                                            |
| `internal`                                                      | 500      | An internal error in pt-tools                                                                                                     |
| `search_failed`, `download_failed`, `attend_failed`, `upstream` | 502      | Searching, downloading the torrent file, signing in or reaching TMDB failed, or the endpoint saving a configuration change failed |
| `unavailable`                                                   | 503      | The service behind the endpoint is not running                                                                                    |
| `assistant_disabled`, `decided`                                 | 409      | The AI assistant is not enabled; this pending action has already been decided                                                     |
| `too_many`                                                      | 429      | The AI assistant is answering too many questions at once                                                                          |

## Endpoints

| Method | Path                                          | Permission       | Purpose                                                                       |
| ------ | --------------------------------------------- | ---------------- | ----------------------------------------------------------------------------- |
| GET    | `/meta`                                       | Read             | Version, compatibility level, feature groups and the caller's own permissions |
| GET    | `/overview`                                   | Read             | Totals across enabled sites and today's changes                               |
| GET    | `/sites`                                      | Read             | Sites, login status, today's sign-in and your statistics on each site         |
| POST   | `/sites/{site}/attend`                        | Operate          | Sign in now                                                                   |
| GET    | `/favicon/{site}`                             | Read             | Site icon                                                                     |
| GET    | `/torrents`                                   | Read             | Torrents in the downloaders                                                   |
| GET    | `/downloaders`                                | Read             | Downloaders with their current speed and free space                           |
| POST   | `/torrents/actions`                           | Operate          | Pause, resume and delete torrents                                             |
| POST   | `/search`                                     | Read             | Multi-site search                                                             |
| POST   | `/push`                                       | Operate          | Push a search result to a downloader                                          |
| GET    | `/tasks`                                      | Read             | Task list (push records)                                                      |
| GET    | `/brush/tasks`                                | Read             | Brush tasks                                                                   |
| GET    | `/media/history`                              | Read             | Organise history                                                              |
| POST   | `/media/history/{id}/retry`                   | Operate          | Organise the torrent of one history record again                              |
| POST   | `/media/history/{id}/scrape`                  | Operate          | Scrape the movie or show of one history record again                          |
| GET    | `/media/libraries`                            | Read             | Media libraries                                                               |
| POST   | `/media/recognize`                            | Read             | Recognise a torrent title                                                     |
| POST   | `/media/organize/preview`                     | Read             | Preview organising a torrent in a downloader                                  |
| POST   | `/media/organize`                             | Operate          | Organise a torrent in a downloader into a media library                       |
| GET    | `/subscriptions`                              | Read             | Subscriptions                                                                 |
| GET    | `/subscriptions/{id}`                         | Read             | A subscription with its episodes and downloaded torrents                      |
| POST   | `/subscriptions`                              | Operate          | Create a subscription                                                         |
| POST   | `/subscriptions/{id}/status`                  | Operate          | Pause or resume a subscription                                                |
| POST   | `/subscriptions/{id}/search`                  | Operate          | Search now                                                                    |
| PUT    | `/subscriptions/{id}`                         | Operate          | Change a subscription's settings (follow schedule, version lock and so on)    |
| DELETE | `/subscriptions/{id}`                         | Operate          | Delete a subscription                                                         |
| GET    | `/subscriptions/{id}/episodes`                | Read             | Each episode of a followed show                                               |
| POST   | `/subscriptions/{id}/episodes/{n}/skip`       | Operate          | Skip an episode, or want it again                                             |
| POST   | `/subscriptions/{id}/episodes/{n}/unlock`     | Operate          | Let one episode use another version (outside the version lock)                |
| GET    | `/subscriptions/{id}/runs`                    | Read             | A subscription's run history                                                  |
| POST   | `/subscriptions/{id}/torrents/{tid}/organize` | Operate          | Organise one torrent the subscription downloaded again                        |
| POST   | `/subscriptions/{id}/torrents/{tid}/scrape`   | Operate          | Scrape one torrent the subscription downloaded again                          |
| POST   | `/subscriptions/schedule/preview`             | Read             | When a schedule will search next                                              |
| GET    | `/subscriptions/{id}/episode-rules`           | Read             | Episode-number rules of a followed show                                       |
| PUT    | `/subscriptions/{id}/episode-rules`           | Operate          | Set the episode-number rule for a release group                               |
| DELETE | `/subscriptions/{id}/episode-rules`           | Operate          | Delete the episode-number rule for a release group                            |
| GET    | `/explore`                                    | Read             | Explore: TMDB trending, popular and search                                    |
| GET    | `/images/tmdb/{size}/{file}`                  | Read             | A TMDB image (poster), fetched by pt-tools                                    |
| GET    | `/updates`                                    | Read             | Whether a newer release exists                                                |
| GET    | `/assistant/status`                           | Read             | Whether the AI assistant is on, its model, and whether you can confirm        |
| GET    | `/assistant/conversations`                    | Read             | Your AI assistant conversations                                               |
| POST   | `/assistant/conversations`                    | Read             | Create a conversation, optionally sending the first message                   |
| GET    | `/assistant/conversations/{id}`               | Read             | A conversation's messages and pending actions                                 |
| DELETE | `/assistant/conversations/{id}`               | Read             | Delete a conversation                                                         |
| POST   | `/assistant/conversations/{id}/messages`      | Read             | Send a message; the answer is generated in the background                     |
| POST   | `/assistant/conversations/{id}/cancel`        | Read             | Stop the answer                                                               |
| GET    | `/assistant/conversations/{id}/events`        | Read             | Long-poll a conversation's progress                                           |
| POST   | `/assistant/confirmations/{id}`               | Operate          | Confirm or deny an action                                                     |
| GET    | `/templates`                                  | Read             | Message templates: events, variables and the current templates                |
| POST   | `/templates/preview`                          | Read             | Render a template with sample data                                            |
| PUT    | `/templates/{type}`                           | Operate          | Save the template for an event                                                |
| DELETE | `/templates/{type}`                           | Operate          | Restore the default                                                           |
| GET    | `/config`                                     | Config + Read    | Configuration catalog: what can be read and changed, operations and fields    |
| GET    | `/config/{resource}`                          | Config + Read    | Current values of one kind of configuration (secrets masked)                  |
| POST   | `/config/{resource}/preview`                  | Config + Read    | Preview a change: before, after and impact                                    |
| POST   | `/config/{resource}/changes`                  | Config + Operate | Save a change                                                                 |

Paths leave out the `/api/app/v1` prefix. Search, recognition, the organise preview, the template preview and the configuration preview send their content with POST but need only Read. Every call a token makes to an Operate endpoint, including calls refused for lack of permission, is recorded under ChatOps → Audit log (操作审计) with the channel API token (API 令牌); a push that was stopped and batch actions with failures are recorded as errors even though the response is 200.

## Basics

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
  "principal": { "kind": "api_token", "name": "My phone", "scopes": ["app:read", "app:write"] }
}
```

`principal.kind` is `api_token` (a token), `session` (a web sign-in) or `remote_device` (a device paired through remote access); `features` lists the feature groups of this version (earlier versions have no `downloaders`, `updates`, `assistant` or `templates`); `scopes` are the caller's permissions, so a client can decide which actions to show.

### GET /updates

Whether a newer release exists: `current_version`, `has_update`, `new_releases` (each with `version`, `name`, `published_at`, `url` and `changelog`, plus `prerelease: true` for previews) and `checked_at`. With `include_prerelease=1` preview releases count too. The result is cached for a while instead of asking GitHub every time; it never upgrades. When it cannot check and has no earlier result it returns 502 (`upstream`).

## Overview and sites

### GET /overview

- `totals` adds up the enabled sites: `uploaded`, `downloaded`, `ratio`, `seeding`, `leeching`, `bonus`, `bonus_per_hour`, `seeding_size`, `site_count`, `unread_messages`.
- `today` holds today's changes: `from`, `to`, `uploaded`, `downloaded`, `bonus`, with per-site changes in `sites`. They are counted the same way as in [Statistics](../guide/user-stats.md). When today's changes cannot be computed, `today.error` gives the reason, and the zeros do not mean there was no traffic today.
- `updated_at` is when the site statistics were last updated.
- Every `downloaded` here is the billed download: what the site counts toward your ratio after free and discount promotions, not the amount actually transferred. The `today` changes come from the daily snapshots, which keep only the billed figure. See [Two kinds of download figures](../guide/user-stats.md#two-kinds-of-download-figures).

### GET /sites

The response is `{"items": [...], "user_error": "…"}`: `items` has one item per site; when your statistics on the sites cannot be read, `user_error` gives the reason and no item has `user`.

| Field                             | Content                                                                                                                                                                                                                                                                                       |
| --------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `name`, `display_name`, `enabled` | The site's identifier, display name and whether it is enabled                                                                                                                                                                                                                                 |
| `login`                           | Login status: `tier` is the reminder level (`none`, `30d`, `14d`, `7d`, `3d`, `banned-imminent`, or `unknown` when there is no access record), `days_remaining` is the number of days left before the site's inactivity limit, plus `last_active_at`, `last_probe_at` and `last_probe_status` |
| `attendance`                      | Today's sign-in: `supported`, `enabled`, `day`, `status` (`pending`, `signed`, `already`, `failed`, `unsupported`), `message`, `last_attempt_at`                                                                                                                                              |
| `user`                            | Your statistics on the site, absent until they have been fetched: `username`, `level`, `uploaded`, `downloaded`, `ratio`, `bonus`, `bonus_per_hour`, `seeding`, `leeching`, `seeding_size`, `unread_messages`, `hnr_unsatisfied`, `updated_at`                                                |

The `downloaded` in `user` is also the billed download, not the amount actually transferred.

### POST /sites/{site}/attend

Signs in to the site now and responds with the same object as `attendance` in `/sites`. If a sign-in for the site is already running, it responds with 409.

### GET /favicon/{site}

The site icon shown in the web interface. The response is an image, not JSON.

## Downloaders

### GET /downloaders

The enabled downloaders: `id`, `name`, `type` (`qbittorrent` or `transmission`), `default`, `reachable` (status or free space could be read this time), `error` (why not), `version`, `upload_speed`, `download_speed`, `uploaded`, `downloaded` (the downloader's own totals) and `free_space`. No downloader address, username or password.

### GET /torrents

| Parameter           | Description                                                                                                                                                                                     |
| ------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `downloader_id`     | Only this downloader                                                                                                                                                                            |
| `state`             | `downloading`, `seeding`, `paused`, `stopped`, `queued`, `checking` or `error`                                                                                                                  |
| `q`                 | A keyword in the title                                                                                                                                                                          |
| `sort`, `order`     | The sort field (`added_at`, `completed_at`, `title`, `progress`, `size`, `ratio`, `state`, `upload_speed`, `download_speed`, `eta`, `seeds`) and `asc` or `desc`; newest added first by default |
| `page`, `page_size` | `page_size` is at most 200                                                                                                                                                                      |

Each item has `downloader_id`, `downloader`, `task_id`, `info_hash`, `title`, `progress`, `size`, `state`, `ratio`, `seeds`, `peers`, `upload_speed`, `download_speed`, `eta` (estimated seconds left, -1 when it cannot be estimated), `added_at`, `completed_at`, `category`, `tags` and `save_path`. `failures` lists the downloaders whose torrents could not be read (`downloader_id`, `downloader`, `error`); when it is not empty, the list is incomplete.

### POST /torrents/actions

```json
{ "action": "pause", "targets": [{ "downloader_id": 1, "task_id": "task_id from /torrents" }] }
```

`action` is `pause`, `resume`, `delete` (removes the torrent and keeps its data) or `delete_with_files` (removes the data too); `targets` holds 1 to 100 torrents. The response has `succeeded`, `failed` and per-torrent `results` (`downloader_id`, `task_id`, `success`, `message`).

## Search and push

### POST /search

| Field         | Description                                                                                                                                      |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| `keyword`     | Required, up to 100 characters                                                                                                                   |
| `sites`       | Search only these sites, up to 100; when empty, every enabled site is searched, and when none of the listed sites is enabled nothing is searched |
| `category`    | Category                                                                                                                                         |
| `free_only`   | Free torrents only                                                                                                                               |
| `min_seeders` | Minimum number of seeders                                                                                                                        |

The response:

- `items` are the results. Each has `site`, `torrent_id`, `title`, `subtitle`, `info_hash`, `size`, `seeders`, `leechers`, `snatched`, `uploaded_at`, `category`, `tags`, `discount`, `discount_end_at`, `free`, `has_hr`, `imdb_id` and `douban_id`. There are no detail-page or download links; push with `POST /push`.
- `sites` is the number of results per site, `errors` lists the sites that failed (`site`, `error`), and `duration_ms` is how long the search took. A search waits at most 30 seconds.

### POST /push

Pushes one torrent to a downloader: pt-tools downloads the torrent file from the site, then applies the same disk space protection and site seeding capacity checks as an RSS push.

| Field                                    | Description                                                                                                                            |
| ---------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| `site`, `torrent_id`                     | Required, taken from a search result                                                                                                   |
| `downloader_id`                          | The downloader to push to; when empty, the default downloader, or the first enabled one if there is no default                         |
| `title`, `category`, `tags`, `save_path` | Optional: the title for the record (the name in the torrent file when empty), and the category, tags and save folder in the downloader |

The response has `success`, `skipped` (the downloader already has this torrent), `message`, `info_hash`, `downloader_id` and `downloader`. When disk space protection or the site seeding capacity stops the push, `success` is `false` and `message` gives the reason, still with status 200; when the torrent file cannot be downloaded, the status is 502. If the torrent was found by a search shortly before, its size, H&R, free status and free expiry from the search result are recorded with the task (H&R protection and free-expiry cleanup use them).

## Task list

### GET /tasks

Push records, newest first.

| Parameter           | Description                 |
| ------------------- | --------------------------- |
| `site`              | Only this site              |
| `q`                 | A keyword in the title      |
| `pushed`            | `1` for pushed records only |
| `page`, `page_size` | `page_size` is at most 100  |

Each item has `id`, `site`, `torrent_id`, `title`, `size`, `category`, `tags`, `free`, `free_level`, `free_end_at`, `has_hr`, `pushed`, `pushed_at`, `downloader`, `progress`, `completed`, `completed_at`, `source`, `error` and `created_at`. `source` says where the record came from: RSS is `free_download` or `filter_rule`; the others are `manual_push` (pushed from the web interface), `app_push` (pushed from the app), `brush`, `subscription`, `transfer` (torrent transfer) and `reseed`.

## Brush

### GET /brush/tasks

One item per brush task: `id`, `name`, `enabled`, `site`, `site_enabled`, `downloader`, `active` (torrents being brushed), `downloading` (of those, the ones still downloading), `active_size`, and the results for `today` and `total` (`uploaded`, `downloaded`, `added`, `removed`).

## Media

These endpoints match Media (媒体) in the web interface; see [Follow and subscribe](../guide/media-subscribe.md) and [Media library organising](../guide/media-library.md). Creating subscriptions and Explore use TMDB; without a usable API key they respond with 400, see [TMDB API key](../guide/media-recognize.md#tmdb-api-key).

### GET /media/history

Organise history, most recently updated first. Parameters: `status` (`done`, `failed`, `skipped`, `removed`), `q` (keyword), `page` and `page_size` (at most 100). Each item has `id`, `media_type`, `tmdb_id`, `title`, `year`, `season`, `episode`, `episode_end`, `torrent_name`, `library`, `target_path`, `mode`, `size`, `status`, `message` and `created_at`.

### POST /media/history/{id}/retry

Organises the torrent of this record again (usually a failed one); an entry and library chosen by hand earlier are kept. Responds like `POST /media/organize`. A missing record gets 404.

### POST /media/history/{id}/scrape

Scrapes the movie or show of this record again (every history record of the same title in this library), writing the NFO and images the library's current scrape settings ask for; locked fields stay as they are. The response has `records` (history records scraped), `written` (files written), `messages` and `queued` (still running in the background; check the history later). It responds with 400 when the library has scraping turned off or the record is not organised, and 404 when the record is missing.

### GET /media/libraries

One item per library: `id`, `name`, `kind` (`movie` or `tv`), `anime` (anime only), `path` (the library folder as pt-tools sees it), `mode` (organise mode; empty means the default from the organise settings), `scrape`, `classify_dir` (put files into sub-category folders), `overwrite_mode` (empty never replaces an old version, `larger` replaces it with a larger file) and `enabled`.

### POST /media/recognize

`{"title": "…", "subtitle": "…", "imdb_id": "tt…"}`: give at least one of the torrent title and subtitle (up to 512 characters each); `imdb_id` is the IMDb ID the site gives, optional. Recognition follows the rules of [Media recognition](../guide/media-recognize.md) (corrections, recognition words, IMDb ID, then a search by name) and changes nothing. Responds with:

- `summary`: one readable line (name, year, season and episode); `meta`: what the title parsed into, such as `name_cn`, `name_en`, `year`, `type`, `season`, `episode`, `episode_end`, `resolution`, `source`, `video_codec`, `hdr`, `audio` and `group`;
- `match`: the TMDB entry (`id`, `media_type`, `title`, `original_title`, `year`, `overview`, `poster_path`, `vote_average`, `imdb_id`), absent without a reliable match; `source`: `override` (corrected by hand), `imdb`, `search` or `none`; `score`;
- `candidates`: up to 5 candidates (with `score`); `message`: a note (for example that without a usable TMDB API key only the title was parsed); `error`: why TMDB failed.

### POST /media/organize/preview

`{"downloader_id": 1, "hash": "…"}`: a torrent in a downloader; use `info_hash` from `GET /torrents` as `hash` (`task_id` when there is no `info_hash`). Add `media_type` and `tmdb_id` together to choose the TMDB entry, and `library_id` to choose the library; without them pt-tools recognises the torrent and picks the library by type. Only the plan is computed; nothing changes. Responds with:

- `downloader_id`, `downloader`, `hash`, `name` (the torrent name), `match` (as above), `library` (`id`, `name`), `category` (the sub-category) and `mode`;
- `items`: one per video, with `rel` (path inside the torrent), `size`, `target` (where it goes in the library), `season`, `episode`, `episode_end`, `status` (`pending` to organise, `done` already in the library, `exists` the target is taken by another file, `replace` replaces an older version pt-tools put there, `failed` cannot be organised) and `message`;
- `skipped`: how many files are left out (samples, too small, not video); `problem`: when set, the whole torrent cannot be organised (for example the files are not found and a path mapping is needed).

### POST /media/organize

Same parameters as the preview. Organises the torrent the library's way (hard link, copy or move), scrapes and asks the media servers to scan, like organising by hand on the web page. Responds with `plan` (as in the preview), `created` (files organised this time), `done`, `skipped`, `failed`, `messages` and `queued` (still running in the background; check the history later). When everything fails the response is still 200 and the audit log records an error. Organising waits at most 60 seconds; after that it responds with 504 (`timeout`) and keeps going in the background.

### GET /subscriptions

Parameters: `status` (`active`, `paused`, `pending`, `done`) and `q` (keyword). Each item has:

- `id`, `media_type` (`movie` or `tv`), `tmdb_id`, `season`, `title`, `original_title`, `year`, `total_episodes`;
- `poster_path`: the TMDB poster path; fetch the image with `GET /images/tmdb/w342/<the path without its leading />` (`w342` can also be one of the other sizes listed below), see below;
- `status`, `upgrade` (whether to upgrade quality), `source` (`manual`, `explore`, `douban`, `chatops`, `app`), `message`, `last_search_at`, `next_search_at`, `created_at`;
- where to look and where to push: `profile_id` (the quality profile; 0 uses the default from the settings), `sites` (search only these sites; empty means all), `downloader_id` (0 uses the one from the settings), `category`, `tags` (comma-separated) and `save_path` (empty uses the downloader's default);
- `progress`: `total`, `required` (episodes to follow: from the start episode on, not skipped), `aired`, `in_library`, `downloading`, `waiting` (episodes waiting for the locked version, also counted in `missing`), `skipped` (episodes not followed) and `missing` (the missing episode numbers);
- the follow settings of a TV show: `schedule` (see below), `lock_version` (the version lock), `lock_group` and `lock_resolution` (the locked release group and resolution, empty until decided), `start_episode` (the episode to start from; 0 and 1 mean the whole season), `follow_next_season` (follow the next season when this one is done), `next_season_id` (the subscription created for the next season), `library_id` (the media library to organise into; 0 picks one by type) and `completed_at` (when it was done).

`required`, `waiting`, `skipped`, the follow settings and the where-to-look fields were added in the 1.0 preview. Earlier versions leave them out, so the app has to cope with responses without them.

#### Schedule

`schedule` is when to search actively:

| Field              | Description                                                                                                                                                                                                                                                        |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `kind`             | Empty uses the global search interval; `airtime` follows the air time; `interval` every so often; `daily` every day; `every_days` every few days; `weekly` on weekdays; `monthly` on days of the month; `cron` by a cron expression                                |
| `at`               | The time `HH:MM` in the time zone of the machine pt-tools runs on. With `airtime` it is when the episode airs (TMDB only has the date); empty means 20:00, also for the other kinds                                                                                |
| `every`            | The number of days for `every_days`, 1 to 30                                                                                                                                                                                                                       |
| `days`             | The weekdays for `weekly` (0 to 6, 0 is Sunday) or the days of the month for `monthly` (1 to 31; a shorter month uses its last day)                                                                                                                                |
| `air_delay_hours`  | With `airtime`, how many hours after airing to start searching (0 to 72; 0 means 2 hours)                                                                                                                                                                          |
| `air_retry_hours`  | With `airtime`, how many hours to wait before searching again when nothing was found (0 to 48; 0 means 6 hours)                                                                                                                                                    |
| `air_window_days`  | With `airtime`, for how many days after airing to retry at that interval (0 to 60; 0 means 7 days); after that a missing episode is searched once a day                                                                                                            |
| `interval_minutes` | With `interval`, the minutes between searches, 30 to 10080 (7 days), counted from the last search                                                                                                                                                                  |
| `cron`             | With `cron`, the expression: 5 fields (minute hour day month weekday); separate several with semicolons and the earliest one due wins. See [Subscriptions and following](../guide/media-subscribe.md#when-to-search) for the syntax. The normalised form is stored |

`airtime` is for TV only. A new TV subscription without a schedule follows the air time. Scheduled searches get a few minutes of random offset, and two searches are at least 30 minutes apart.

### POST /subscriptions/schedule/preview

Shows when a schedule will search next (read-only, needs Read). The request is `{"schedule": {...}, "media_type": "tv", "subscription_id": 1, "count": 5}`: `schedule` is required; with `subscription_id`, that subscription's type and creation time are used (every few days counts from the day it was created); `count` is how many times to list (1 to 10, 5 by default). The response is `{"next": [Unix seconds…], "note": "…"}` without the random offset. A schedule that follows the air time has no times, and `note` explains how it is worked out. A wrong schedule gets 400 with a `message` saying what is wrong (for example which field of the cron expression).

### GET /subscriptions/{id}

The subscription's own fields, plus:

- `episodes`: each episode of a TV season, with `number`, `name`, `air_date`, `state` (`library`, `downloading`, `missing`, `waiting_version` (waiting for the locked version), `upcoming` or `skipped` (not followed)), `source` (where the state comes from: `library`, `subscription` for a torrent the subscription pushed, `downloader` for a torrent already in the downloader), `version` (the current version: release group and resolution), `manual_skip` (skipped by hand) and `manual_unlock` (this episode is outside the version lock);
- `torrents`: the torrents it downloaded, with `id` (the `tid` of the two endpoints below), `site`, `torrent_id`, `title`, `status` (`downloading`, `done`, `failed`, `replaced`), `episode`, `episode_end`, `complete`, `size`, `message`, `created_at`, and:
  - `files`: how the torrent was organised into the library, one item per video file: `id` (the history record, usable with `/media/history/{id}/…`), `name` (the file name), `episode`, `episode_end`, `status` (`done`, `failed`, `skipped`, `removed`), `message` (why it was not organised), `scrape_status` (`ok`, `partial`, `failed`; empty when never scraped), `scrape_note`, `next_retry_at` (when it is retried automatically) and `retrying` (the organise queue is retrying it now); an empty array before the torrent is organised;
  - `can_organize`: it can be organised again (a file that was not organised and is not being retried, or the torrent was marked failed and has no history yet); `can_rescrape`: it can be scraped again (a file is organised). Both are `false` for an old version replaced by an upgrade.

### POST /subscriptions

| Field                           | Description                                                                                               |
| ------------------------------- | --------------------------------------------------------------------------------------------------------- |
| `media_type`                    | Required, `movie` or `tv`                                                                                 |
| `tmdb_id`                       | Required, the TMDB ID                                                                                     |
| `season`                        | The season number for TV; the latest season when empty                                                    |
| `profile_id`                    | The quality profile; the default from the settings when empty                                             |
| `upgrade`                       | Whether to upgrade quality                                                                                |
| `sites`                         | Search only these sites; all sites when empty. They must be configured and enabled; unknown names get 400 |
| `downloader_id`                 | The downloader to push to; the one from the settings when empty. An unknown ID gets 400                   |
| `category`, `tags`, `save_path` | The category, tags (comma-separated) and save directory for pushes; the downloader's defaults when empty  |

A TV subscription follows the show and can also carry:

| Field                | Description                                                                                                                                                                                                                                                                                                                                                                                                                      |
| -------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `schedule`           | The schedule (see above); follows the air time when empty                                                                                                                                                                                                                                                                                                                                                                        |
| `lock_version`       | The version lock: once the first episode is chosen, only the same release group and resolution (it waits rather than switching to another version); on when empty                                                                                                                                                                                                                                                                |
| `start_episode`      | The episode to start from; earlier episodes are not wanted                                                                                                                                                                                                                                                                                                                                                                       |
| `follow_next_season` | Follow the next season when this one is done                                                                                                                                                                                                                                                                                                                                                                                     |
| `library_id`         | The media library to organise into; picked by type when empty or 0                                                                                                                                                                                                                                                                                                                                                               |
| `from_torrent`       | Start following from a torrent on a site: `site`, `torrent_id` and an optional `title` (used as the keyword to search that site only when the cached search results do not have the torrent). The start episode defaults to the torrent's episode, and the torrent is pushed right after creation (not when it fails the quality profile or those episodes are already there; the reason goes into the subscription's `message`) |

The response is the new subscription, with the source `app`. It responds with 400 when the title is already subscribed and 404 when TMDB does not have it. With `from_torrent` it fetches the torrent from the site and pushes it, so the response takes longer.

### PUT /subscriptions/{id}

Changes a subscription's settings. Only the fields given change (a field left out or given as `null` stays as it is); the title and season cannot. It takes `profile_id`, `upgrade`, `sites` (an empty array means all sites; `null` leaves it unchanged), `downloader_id`, `category`, `tags`, `save_path` and, for a followed show, `schedule`, `lock_version`, `lock_group`, `lock_resolution`, `start_episode`, `follow_next_season`, `library_id` and `total_episodes` (the number of episodes in the season: used when greater than 0; 0 goes back to TMDB's count). Giving empty strings for both `lock_group` and `lock_resolution` clears the locked version, which is then decided again by the next download.

When the schedule, version lock, start episode or episode count changes, every episode is worked out again under the new settings, a search runs soon, and the run history records the change. The response is the updated subscription.

### GET /subscriptions/{id}/episodes

Each episode of a followed show (the same as `episodes` in the subscription detail). A movie has no episodes and responds with 400.

### POST /subscriptions/{id}/episodes/{n}/skip

`{"skip": true}` skips episode n (not followed: it is not missing and does not hold up completion); `{"skip": false}` wants it again. The response is every episode after the change.

### POST /subscriptions/{id}/episodes/{n}/unlock

`{"unlock": true}` lets episode n use another version (for when the locked version does not turn up; pt-tools searches once right away); `{"unlock": false}` puts it back under the lock. The response is every episode after the change.

### GET /subscriptions/{id}/runs

The subscription's run history, newest first, with the parameters `page` and `page_size` (1 to 100). Each active search, RSS match, creation and change by hand records one run: `id`, `trigger` (`create`, `schedule`, `rss`, `manual`, `ai`), `started_at`, `finished_at`, `status` (`ok`, `nothing`, `waiting`, `failed`, `partial`; a change by hand is `changed`), `summary`, `steps` (each step's `kind` and `text`; `kind` is `search`, `reject`, `lock`, `push`, `fail`, `adopt`, `wait` or `info`) and `steps_truncated` (too many steps; the rest were not recorded). Each subscription keeps its latest 100 runs for at most 90 days. Downloader addresses are removed from the text.

### POST /subscriptions/{id}/torrents/{tid}/organize

Organises one torrent the subscription downloaded again (`tid` is the `id` in the detail's `torrents`; a season pack works the same as a single episode, one torrent at a time). It uses the subscription's title and library instead of recognising the name, and leaves files already organised alone. A torrent marked failed goes back to waiting for organising (the 7 days start again), and is marked in the library as soon as organising succeeds. Responds like `POST /media/organize` and records a run. A torrent of another subscription gets 404; a torrent without a recorded downloader or info hash, or an old version replaced by an upgrade (its files were removed from the library on purpose), gets 400.

### POST /subscriptions/{id}/torrents/{tid}/scrape

Scrapes the movie or show this torrent was organised into again. Responds like `POST /media/history/{id}/scrape` and records a run. It responds with 400 when no file of the torrent is organised yet (organise it again first), and for an old version replaced by an upgrade.

### GET, PUT and DELETE /subscriptions/{id}/episode-rules

The episode-number rules of a followed show: how the episode number in a torrent title maps to the TMDB episode. There is one rule per release group; an empty `group` applies to every group. `mode` is `tmdb` (the TMDB episode number), `period_part` (by issue and part; for example "issue 1, part 2" of a variety show is TMDB episode 2), `date` (by air date) or `offset` (TMDB episode = the number in the title + `offset`).

- GET: every rule, each with `group`, `mode`, `offset`, `source` (`manual`, `ai` or `auto`), `note` and `updated_at`;
- PUT: `{"group": "XX", "mode": "offset", "offset": 1, "note": "why"}` replaces the group's rule if it has one;
- DELETE: the parameter `group` (leave it out to delete the rule for every group); responds with 404 when there is no such rule.

All three respond with every rule after the change. After a rule changes, a search under the new rule runs soon.

### POST /subscriptions/{id}/status

`{"status": "paused"}` pauses and `{"status": "active"}` resumes; a resumed subscription searches again soon. The response is the updated subscription.

### POST /subscriptions/{id}/search

Searches once now and responds with `{"message": "explanation"}`. A paused subscription responds with 400.

### DELETE /subscriptions/{id}

Deletes the subscription and responds with `{"ok": true}`. Torrents in the downloader and files in the media library stay where they are.

### GET /explore

| Parameter | Description                                                                |
| --------- | -------------------------------------------------------------------------- |
| `kind`    | Required, `movie` or `tv`                                                  |
| `list`    | `trending` (the default), `popular`, or `search` (searches for `q`)        |
| `q`       | The search words for `list=search`, up to 100 characters                   |
| `page`    | Starting at 1; trending and popular go up to 20 pages, search has one page |

The response has `items`, `page` and `total_pages`. Each item has `id` (the TMDB ID), `media_type`, `title`, `original_title`, `year`, `overview`, `poster_path`, `vote_average`, `in_library`, `subscribed` and `subscription_id`.

### GET /images/tmdb/{size}/{file}

A TMDB image: `file` is `poster_path` without its leading `/`, and `size` is one of the poster sizes `w92`, `w154`, `w185`, `w342`, `w500` and `w780` (originals aren't offered, they can be several MB). pt-tools fetches it with the image address and proxy set under Media recognition and responds with the image and a one-day cache header, so clients never contact TMDB directly. Fetching images doesn't need a TMDB API key. It responds with 404 when TMDB has no such image, and with 502 (`upstream`) when the image can't be fetched. pt-tools fetches at most 4 images at a time; a request that waits longer than 10 seconds gets 503 (`busy`, with `Retry-After`).

## AI assistant

These endpoints match the web page AI assistant; see [AI assistant](../guide/assistant.md). Its settings (which model, the API key) can only be changed on the web, not here. Conversations are kept per caller: each token and each paired device sees only the conversations it created, and a signed-in web session sees the web conversations. Chatting needs only Read; when the model wants to change something, only a caller with Operate can confirm it (for a Read-only caller the model does not get those tools). Times are RFC 3339 strings.

### GET /assistant/status

Responds with `enabled` (turned on, with an endpoint and a model configured), `model`, `can_confirm` (whether the caller can confirm actions) and `chatops`. It never contains the API key or the endpoint. On a host without these endpoints, `features` in `GET /meta` has no `assistant`.

### GET /assistant/conversations

The caller's own conversations, most recently updated first, at most 100. Each has `id`, `title`, `entry` (where it was created: `web`, `app` or `chatops`), `running`, `created_at` and `updated_at`.

### POST /assistant/conversations

`{"title": "…", "message": "…"}`, both optional; with `message` you can also give `model` and `effort` (as when sending a message). Creating only a conversation responds with 201; with `message` it also sends that message and responds with 202, where `run` has the run's `run_id` and the stored user message. If the AI assistant is not enabled it responds with 409 (`assistant_disabled`) and leaves no empty conversation behind.

### GET /assistant/conversations/{id}

The conversation's own fields, plus:

- `messages`: the latest 200 messages, oldest first. Each has `id`, `run_id`, `role` (`user`, `assistant`, `tool` or `notice`), `content` and `created_at`; assistant messages carry `tool_calls` (`id`, `name`, `arguments`, `write`) and, when the model returned its reasoning, `reasoning` (redacted); tool results carry `tool_name`, `tool_call_id` and `is_error`. User and assistant messages carry the `model` and `effort` of that turn (absent: the model from the settings, no effort given); user messages sent on the web with attachments have `attachments` (`id`, `name`, `kind` (`image` or `text`), `mime`, `size`; the contents are not available through the App API). A `notice` is pt-tools' own note (stopped, failed, step limit reached, the model does not take the reasoning effort, earlier messages were compacted into a summary).
- `confirmations`: pending actions with `id`, `run_id`, `tool_call_id`, `tool`, `summary` (one readable sentence), `arguments`, `status` (`pending`, `approved`, `denied` or `expired`), `decided_by`, `decided_at`, `expires_at` and `created_at`, plus the write tool's preview: `changes` (each item's `label`, `before` and `after`; `secret` is true for secrets, which never show their value) and `impact` (the possible impact). Match them to the calls in the messages by `run_id` and `tool_call_id`.
- `run_id`, `partial` and `partial_reasoning`: the run in progress and the answer and reasoning of its current step so far; `last_seq`: read events from here.
- `usage`: context usage, `context` (about how many tokens the next request carries), `estimated` (estimated from the text), `window` (the context window), `window_source` (`setting`, `provider`, `table` or `default`), `auto_compact`, `compact_percent`, `input` and `output` (the input and output tokens the provider reported for this conversation) and `compacted` (it was compacted). See [Context and compaction](../guide/assistant.md#context-and-compaction).

### POST /assistant/conversations/{id}/messages

`{"text": "…"}`, up to 4000 characters. Optional: `model` (the model for this turn, no spaces, at most 128 characters; the model from the settings when absent) and `effort` (`none`, `minimal`, `low`, `medium`, `high` or `xhigh`, sent to the model service as `reasoning_effort`; none when absent). When the model service rejects the request because of the effort, pt-tools retries once without it and adds a notice to the conversation. Responds with 202, the run's `run_id` and the stored user message. If the conversation is already answering it responds with 409 (`busy`). The App API takes no attachments (they can only be sent on the web).

### POST /assistant/conversations/{id}/cancel

Stops the answer in progress and responds with `{"ok": true}`; does nothing when nothing is running. An action waiting for confirmation is voided and does not run.

### GET /assistant/conversations/{id}/events

Long polling: `after` is the last sequence number you read (`last_seq` from the conversation, or `last` from the previous page), and `wait` is how many seconds to wait when there are no events (default and maximum 25). Responds with `events`, `reset`, `last` and `running`. When `reset` is true, events in between were lost (or pt-tools restarted): read the conversation again and continue from its `last_seq`.

Every event has `seq`, `type` and `at`, and depending on its type:

| `type`         | Contents                                                                    |
| -------------- | --------------------------------------------------------------------------- |
| `run_started`  | `run_id`                                                                    |
| `delta`        | `text`: a piece of the answer; with `replace` true, the whole answer so far |
| `message`      | `message`: a stored message (fields as in the conversation)                 |
| `tool_started` | `tool`: the tool call that started                                          |
| `confirmation` | `confirmation`: a new pending action, or its decision                       |
| `run_finished` | `status` (`done`, `canceled` or `failed`), with `error` on failure          |

These are the only types. The web event stream also carries reasoning deltas (`reasoning`), usage (`usage`) and compaction starting (`compacting`); the App API leaves them out (installed older apps do not know the types). `last` still moves past them; the reasoning is in the message's `reasoning` once the answer is stored, and the usage is in the conversation's `usage`.

### POST /assistant/confirmations/{id}

`{"approve": true}` confirms, `{"approve": false}` denies. Needs Operate. Responds with the decided action; one that already has a decision (decided elsewhere first, timed out or voided) gets 409 (`decided`). Confirming and denying are recorded in the audit log; the executed action gets its own entry with the channel AI assistant and the user `token:<token id>` or `device:<device id>`.

## Message templates

These endpoints match ChatOps → Message templates (消息模板) on the web; the syntax is described in [Message templates](../guide/notify-channels.md#message-templates). A template applies to every channel and changes every notification and reply sent after it is saved, so saving and restoring the default need Operate, and only callers with Operate get the template source; changes made by tokens and devices are recorded in the audit log. Anything that looks like a credential is masked in the sample values, the built-in text and preview results. On a host without these endpoints, `features` in `GET /meta` has no `templates`.

### GET /templates

Responds with `groups`, `common_vars` and `events`:

- `groups`: the event groups. `key` is `notify` (notifications), `reply` (ChatOps command replies) or `assistant` (AI assistant replies), with a `name` and a `desc`.
- `common_vars`: the variables every event has. A variable has a `key`, a `type` (`string`, `int`, `float`, `bool` or `list`), a `desc` and a `sample` (the sample value written as text); a list has no sample, and the fields of each item are in `item`.
- `events`: one entry per event with `type`, `group`, `name`, `desc`, `no_title` (body only), `vars`, `sample_title` and `sample_text` (what is sent when nothing is changed), `title` and `text` (the current templates; an empty one uses the built-in text; only callers with Operate get them, a read-only caller gets them empty and can tell from `customized` whether there is one), `customized` and `updated_at` (when it was customized, as a Unix timestamp in seconds).

### POST /templates/preview

`{"type": "rss.matched", "title": "…", "text": "…"}` renders the template with the event's sample data and saves nothing. Responds with `{"title": "…", "text": "…"}` (`title` is empty for a body-only event). A template that is not right gets 400 (`invalid_argument`) with a `message` naming the line and the problem; an unknown event gets 404.

### PUT /templates/{type}

`{"title": "…", "text": "…"}` is checked the same way as a preview before it is saved and gets 400 when it is not right; leaving both empty restores the default. A title for a body-only event also gets 400. Responds with the event as saved (the same fields as an entry of `events`).

### DELETE /templates/{type}

Restores the built-in text and responds with `{"ok": true}`; an unknown event gets 404.

## Configuration

These endpoints read and change pt-tools' configuration through the configuration gateway, the same one behind MCP's `get_config` and `change_config` and the AI assistant's [configuration changes](../guide/assistant.md#changing-the-configuration). A change goes to the endpoint the web settings use, so validation, encryption and hot reload are the same. All of them need Config (`app:config`); reading and previewing also need Read, saving needs Operate. A web login has every permission; paired devices have no Config and get 403.

Not available here: the login password, API tokens, remote access, the encryption key, the AI assistant's own settings (including voice input), ChatOps bindings and admins, and the qB-compatible entrance.

### GET /config

The catalog: `resources` lists what can be read and changed, each with `name`, `title`, `description`, `kind` (`object` is one set of settings, `list` a collection), `id_field` (the list items' ID field), `ops`, `fields` (writable fields; `config.<key>` means every field starting with it), `required` (needed when adding), `secrets` (secret fields) and `hints` (field notes). It also has `excluded` (what cannot be changed here) and `secret_placeholder` (`<已设置>`).

### GET /config/{resource}

The current values: `resource` and `kind`; `item` for objects, `items` and `id_field` for lists. The query parameter `id` returns one list item (404 when there is none). Field names are the web endpoints' ones; secrets read as `<已设置>` (set) or an empty string, and user info and query strings of addresses in other fields are masked too.

### POST /config/{resource}/preview

```json
{ "op": "update", "id": "2", "values": { "enabled": false } }
```

`op` is `set` (objects), `add`, `update` or `delete` (lists; update and delete need `id`); `values` holds only the fields to change. Numbers and switches use the current types; numbers written as strings are converted to match the current value. A new value changes a secret, giving back `<已设置>` keeps it and an empty string clears it. Responds with `resource`, `op`, `id`, `summary` (one sentence), `changes` (each item's `label`, `field`, `before`, `after` and `secret`), `impact` and `applied` (false). Unknown fields, values equal to the current ones and missing required fields get 400 (`invalid_argument`) with the writable fields in `message`; an unknown resource or ID gets 404.

### POST /config/{resource}/changes

The same body as the preview. The change is worked out again from the current state (changes made elsewhere after the preview are taken into account) and saved; the response is like the preview with `applied` true, `item` as it is now (absent after a deletion) and a new item's ID in `id`. When the saving endpoint refuses, its reason comes back as it is: 400 for validation, 409 (`conflict`) for a conflict, 503 when the service is not running. Saves by tokens and devices are recorded in the audit log with the resource, operation, ID and the changed field names, not their values.
