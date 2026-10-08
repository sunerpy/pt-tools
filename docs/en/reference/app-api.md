# App API

The App API is the interface for the mobile app and for scripts; every address starts with `/api/app/v1`. It exposes only what an app needs: responses never contain site cookies, API keys, passkeys, passwords, RSS addresses or download links, and addresses inside error messages have their query strings removed.

## Authentication

Send `Authorization: Bearer <token>` with every request. Tokens are created under System → API tokens (系统 → API 令牌); see [API tokens](../guide/api-tokens.md). A browser that is signed in to the web interface can also call these endpoints, with every permission.

```bash
curl -H "Authorization: Bearer $PTT_TOKEN" https://pt-tools.example.com/api/app/v1/overview
```

- No token, or a token that is wrong, expired or revoked: 401.
- A token without the permission the endpoint needs: 403. Below, Read is the permission `app:read` and Operate is `app:write`.
- Tokens work only for the App API: the rest of the web interface (settings, sites, downloaders, notification channels, token management and so on) does not accept them and responds with 401 without a sign-in.

## Conventions

- Request bodies and responses are JSON. A request body is limited to 1 MiB; a field the endpoint does not know is rejected with 400 rather than ignored.
- Operate endpoints without a body (sign in, search now, delete a subscription) accept no body or `{}`; anything else gets 400.
- Times are Unix timestamps in seconds. Sizes and uploaded or downloaded amounts are in bytes, speeds in bytes per second, and progress is a percentage from 0 to 100.
- Paged endpoints accept the query parameters `page` (starting at 1) and `page_size` (20 by default; each endpoint states its maximum) and respond with `{"items": [...], "total": count, "page": page, "page_size": size}`.
- Compatibility: `remote_api_level` in `GET /meta` is currently 1 and goes up only for incompatible changes. New endpoints or fields are not incompatible changes, so clients should ignore fields they do not know.

## Errors

An error responds with `{"error": "code", "message": "explanation"}`. `message` is a human-readable explanation in Chinese; clients should act on `error`.

| `error`                                                         | Status | Meaning                                                                                  |
| --------------------------------------------------------------- | ------ | ---------------------------------------------------------------------------------------- |
| `invalid_body`                                                  | 400    | The body is not valid JSON, or has a field that is unknown                               |
| `invalid_argument`                                              | 400    | A parameter is wrong; `message` says which                                               |
| `unauthorized`                                                  | 401    | No valid token                                                                           |
| `forbidden`                                                     | 403    | The token lacks this permission                                                          |
| `not_found`                                                     | 404    | The endpoint, site or subscription does not exist                                        |
| `method_not_allowed`                                            | 405    | The endpoint exists but not with this method; the `Allow` header lists the accepted ones |
| `busy`                                                          | 409    | The same thing is in progress, such as signing in to a site                              |
| `rate_limited`                                                  | 429    | TMDB is rate limiting; try again later                                                   |
| `internal`                                                      | 500    | An internal error in pt-tools                                                            |
| `search_failed`, `download_failed`, `attend_failed`, `upstream` | 502    | Searching, downloading the torrent file, signing in or reaching TMDB failed              |
| `unavailable`                                                   | 503    | The service behind the endpoint is not running                                           |

## Endpoints

| Method | Path                         | Permission | Purpose                                                                       |
| ------ | ---------------------------- | ---------- | ----------------------------------------------------------------------------- |
| GET    | `/meta`                      | Read       | Version, compatibility level, feature groups and the caller's own permissions |
| GET    | `/overview`                  | Read       | Totals across enabled sites and today's changes                               |
| GET    | `/sites`                     | Read       | Sites, login status, today's sign-in and your statistics on each site         |
| POST   | `/sites/{site}/attend`       | Operate    | Sign in now                                                                   |
| GET    | `/favicon/{site}`            | Read       | Site icon                                                                     |
| GET    | `/torrents`                  | Read       | Torrents in the downloaders                                                   |
| GET    | `/downloaders`               | Read       | Downloaders with their current speed and free space                           |
| POST   | `/torrents/actions`          | Operate    | Pause, resume and delete torrents                                             |
| POST   | `/search`                    | Read       | Multi-site search                                                             |
| POST   | `/push`                      | Operate    | Push a search result to a downloader                                          |
| GET    | `/tasks`                     | Read       | Task list (push records)                                                      |
| GET    | `/brush/tasks`               | Read       | Brush tasks                                                                   |
| GET    | `/media/history`             | Read       | Organise history                                                              |
| GET    | `/subscriptions`             | Read       | Subscriptions                                                                 |
| GET    | `/subscriptions/{id}`        | Read       | A subscription with its episodes and downloaded torrents                      |
| POST   | `/subscriptions`             | Operate    | Create a subscription                                                         |
| POST   | `/subscriptions/{id}/status` | Operate    | Pause or resume a subscription                                                |
| POST   | `/subscriptions/{id}/search` | Operate    | Search now                                                                    |
| DELETE | `/subscriptions/{id}`        | Operate    | Delete a subscription                                                         |
| GET    | `/explore`                   | Read       | Explore: TMDB trending, popular and search                                    |
| GET    | `/updates`                   | Read       | Whether a newer release exists                                                |

Paths leave out the `/api/app/v1` prefix. Search sends its conditions with POST but needs only Read. Every call a token makes to an Operate endpoint, including calls refused for lack of permission, is recorded under ChatOps → Audit log (操作审计) with the channel API token (API 令牌); a push that was stopped and batch actions with failures are recorded as errors even though the response is 200.

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
    "updates"
  ],
  "principal": { "kind": "api_token", "name": "My phone", "scopes": ["app:read", "app:write"] }
}
```

`principal.kind` is `api_token` (a token) or `session` (a web sign-in); `features` lists the feature groups of this version (earlier versions have no `downloaders` and `updates`); `scopes` are the caller's permissions, so a client can decide which actions to show.

### GET /updates

Whether a newer release exists: `current_version`, `has_update`, `new_releases` (each with `version`, `name`, `published_at`, `url` and `changelog`, plus `prerelease: true` for previews) and `checked_at`. With `include_prerelease=1` preview releases count too. The result is cached for a while instead of asking GitHub every time; it never upgrades. When it cannot check and has no earlier result it returns 502 (`upstream`).

## Overview and sites

### GET /overview

- `totals` adds up the enabled sites: `uploaded`, `downloaded`, `ratio`, `seeding`, `leeching`, `bonus`, `bonus_per_hour`, `seeding_size`, `site_count`, `unread_messages`.
- `today` holds today's changes: `from`, `to`, `uploaded`, `downloaded`, `bonus`, with per-site changes in `sites`. They are counted the same way as in [Statistics](../guide/user-stats.md). When today's changes cannot be computed, `today.error` gives the reason, and the zeros do not mean there was no traffic today.
- `updated_at` is when the site statistics were last updated.

### GET /sites

The response is `{"items": [...], "user_error": "…"}`: `items` has one item per site; when your statistics on the sites cannot be read, `user_error` gives the reason and no item has `user`.

| Field                             | Content                                                                                                                                                                                                                                                                                       |
| --------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `name`, `display_name`, `enabled` | The site's identifier, display name and whether it is enabled                                                                                                                                                                                                                                 |
| `login`                           | Login status: `tier` is the reminder level (`none`, `30d`, `14d`, `7d`, `3d`, `banned-imminent`, or `unknown` when there is no access record), `days_remaining` is the number of days left before the site's inactivity limit, plus `last_active_at`, `last_probe_at` and `last_probe_status` |
| `attendance`                      | Today's sign-in: `supported`, `enabled`, `day`, `status` (`pending`, `signed`, `already`, `failed`, `unsupported`), `message`, `last_attempt_at`                                                                                                                                              |
| `user`                            | Your statistics on the site, absent until they have been fetched: `username`, `level`, `uploaded`, `downloaded`, `ratio`, `bonus`, `bonus_per_hour`, `seeding`, `leeching`, `seeding_size`, `unread_messages`, `hnr_unsatisfied`, `updated_at`                                                |

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

These endpoints match Media (媒体) in the web interface; see [Subscriptions](../guide/media-subscribe.md) and [Media library organising](../guide/media-library.md). Creating subscriptions and Explore use TMDB; without an API key they respond with 400, see [Enter a TMDB API key](../guide/media-recognize.md#enter-a-tmdb-api-key).

### GET /media/history

Organise history, most recently updated first. Parameters: `status` (`done`, `failed`, `skipped`, `removed`), `q` (keyword), `page` and `page_size` (at most 100). Each item has `id`, `media_type`, `tmdb_id`, `title`, `year`, `season`, `episode`, `episode_end`, `torrent_name`, `library`, `target_path`, `mode`, `size`, `status`, `message` and `created_at`.

### GET /subscriptions

Parameters: `status` (`active`, `paused`, `pending`, `done`) and `q` (keyword). Each item has:

- `id`, `media_type` (`movie` or `tv`), `tmdb_id`, `season`, `title`, `original_title`, `year`, `total_episodes`;
- `poster_path`: the TMDB poster path; the image address is `https://image.tmdb.org/t/p/w342` followed by this path (`w342` can be any other size TMDB offers);
- `status`, `upgrade` (whether to upgrade quality), `source` (`manual`, `explore`, `douban`, `chatops`, `app`), `message`, `last_search_at`, `next_search_at`, `created_at`;
- `progress`: `total`, `aired`, `in_library`, `downloading` and `missing` (the missing episode numbers).

### GET /subscriptions/{id}

The subscription's own fields, plus:

- `episodes`: each episode of a TV season, with `number`, `name`, `air_date` and `state` (`library`, `downloading`, `missing` or `upcoming`);
- `torrents`: the torrents it downloaded, with `site`, `torrent_id`, `title`, `status` (`downloading`, `done`, `failed`, `replaced`), `episode`, `episode_end`, `complete`, `size`, `message` and `created_at`.

### POST /subscriptions

| Field        | Description                                                   |
| ------------ | ------------------------------------------------------------- |
| `media_type` | Required, `movie` or `tv`                                     |
| `tmdb_id`    | Required, the TMDB ID                                         |
| `season`     | The season number for TV; the latest season when empty        |
| `profile_id` | The quality profile; the default from the settings when empty |
| `upgrade`    | Whether to upgrade quality                                    |

The response is the new subscription, with the source `app`. It responds with 400 when the title is already subscribed and 404 when TMDB does not have it.

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
