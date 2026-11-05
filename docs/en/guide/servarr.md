# Servarr-compatible entrance <Badge type="tip" text="1.0 preview" />

The Servarr-compatible entrance lets [Seerr](https://github.com/seerr-team/seerr) use pt-tools as its Radarr and Sonarr: a movie requested in Seerr becomes a pt-tools movie subscription, and each season of a series request becomes a followed season. Once pt-tools searches, pushes and organises into the library, Seerr's scans see the state of those subscriptions and the files in the library. The subscriptions pick releases by quality profile and go through the disk space and site seeding capacity checks as usual; see [Follow and subscribe](media-subscribe.md).

The entrance is on the pt-tools web port (no extra port), at `/servarr/radarr` and `/servarr/sonarr`, and is reached with an API token that has the Servarr compatible (Servarr 兼容) permission.

It is compatible with one fixed version of Seerr: commit `dca1af6a` (2026-11-01) of [seerr-team/seerr](https://github.com/seerr-team/seerr). A later Seerr that changes how it calls Radarr or Sonarr may not work.

## Before you start

1. Under System → API tokens (系统 → API 令牌), create a token with only the Servarr compatible (Servarr 兼容) permission and copy it; see [API tokens](api-tokens.md).
2. Turn on the Subscriptions (订阅) switch in Subscription settings (订阅设置) at the bottom of Media → Follow and subscribe (媒体 → 追剧与订阅). While it is off, Seerr's requests still become subscriptions, but nothing searches for them.
3. To choose which library a request goes into, create the libraries under Media → Media library (媒体 → 媒体库) first. It works without any library too: pick the `auto` root folder.

## Seerr settings

In Seerr's Settings → Services, add one Radarr server and one Sonarr server (Add Radarr Server, Add Sonarr Server):

| Setting                | Radarr                                                                                  | Sonarr            |
| ---------------------- | --------------------------------------------------------------------------------------- | ----------------- |
| Hostname or IP Address | The pt-tools host                                                                       | Same              |
| Port                   | The pt-tools web port (8080 by default)                                                 | Same              |
| Use SSL                | On when Seerr reaches pt-tools through an HTTPS reverse proxy                           | Same              |
| API Key                | A token with the Servarr compatible permission (starts with `ptt_`)                     | Same              |
| URL Base               | `/servarr/radarr`                                                                       | `/servarr/sonarr` |
| Default Server         | On: Seerr sends requests only to the default server                                     | Same              |
| Enable Scan            | Recommended: Seerr reads the subscriptions regularly and shows Processing and Available | Same              |

Then click Test, and Seerr loads the quality profiles and root folders:

- **Quality Profile**: Default profile (默认档案) is the default quality profile in Subscription settings; the others are the pt-tools [quality profiles](media-subscribe.md#quality-profiles). The Sonarr Anime Quality Profile comes from the same list.
- **Root Folder**: `auto` picks the library by type, the same as leaving the library empty when you create a subscription in the web UI; the others are the folders of the enabled libraries of that type (movie libraries for Radarr, TV libraries for Sonarr). The same goes for Anime Root Folder.
- **Language Profile**: not needed. pt-tools reports Sonarr 4, so Seerr does not ask for a language profile.
- **Tags** and **Tag Requests**: they work. The tags are stored only in pt-tools and returned to Seerr; they do not affect subscriptions.
- **Enable Automatic Search**: when on, a new subscription searches right away (within a minute when the Subscriptions switch is on). When off, it searches on its schedule: movies at the search interval in Subscription settings, followed seasons by air time (the first search at most a day later).

Minimum Availability, Series Type, Season Folders and Monitor New Seasons have no effect: when to search and how files are named follow the pt-tools subscription and library. To move on to the next season once a season is complete, edit that followed season in pt-tools and turn on Then follow the next season (追完以后接着追下一季).

pt-tools keeps a single subscription per movie, with no separate 4K one: don't set it up as both a regular server and a 4K server (one with 4K Server checked).

## After a Seerr request

- **Movies**: one movie subscription is created with the source Seerr, using the quality profile and library chosen in Seerr. When pt-tools already has a subscription for the movie, none is added: one that isn't searching (paused or pending) is turned on and switched to the quality profile chosen in Seerr, and it searches right away when Enable Automatic Search is on. When the movie is already organised into the library, Seerr treats it as available and no subscription is created.
- **Series**: each requested season becomes a followed season with the source Seerr and the default settings for followed seasons (search by air time, version lock on). When the series already has followed seasons, a newly requested season uses the quality profile and root folder pt-tools currently reports to Seerr for the series (taken from the searching season with the highest ID, or from the season with the highest ID when none is searching), not the ones chosen in this Seerr request; existing seasons only change state, and keep their quality profile, library, save path, category and download tags. Season 0 (specials) is not followed.
- **State**: in Seerr's scans, a searching subscription with no files is Processing; once files are organised into the library it is Available or Partially Available by file count (each season compares the episodes organised for it with the episode count on TMDB). Only files pt-tools organised into the library count; Seerr also checks Jellyfin and Plex on its own. If you pause a subscription that has no files yet in pt-tools, Seerr's next scan treats it as no longer being processed and marks its request declined (this is how Seerr works).
- **Deleting**: removing an item from Radarr or Sonarr in Seerr deletes only the pt-tools subscription (for a series, every season of it), never any file or torrent, and ignores Seerr's request to delete files. To delete the files as well, delete the title from its [title details](media-detail.md#deleting-a-title) page in pt-tools.

## Not supported

- Monitoring single episodes: pt-tools follows whole seasons. Seerr calls this only for unmonitored episodes after it monitors a season, so the result is the same. To skip one episode, click it in the followed season's details and choose Skip this episode (跳过这一集).
- Download progress: Seerr shows no progress; the download queue is always empty.
- Series without a TVDB ID on TMDB: Seerr does not send them to Sonarr (a Seerr limitation), and pt-tools does not list such series to Seerr.
- Other Radarr and Sonarr endpoints (calendar, history, import, settings and so on) return 404; unknown commands return 400.

## Audit and security

- Writes (creating, changing and deleting subscriptions and tags, and commands) are recorded under ChatOps → Audit log (操作审计) with the channel Servarr compatible (Servarr 兼容) and the token ID as the user, including which seasons were requested and whether they were created or changed. The refresh-downloads command Seerr sends every minute is not recorded.
- Seerr puts the token in the `apikey` query parameter (the `X-Api-Key` header also works). The pt-tools access log records only the path, not the query; when Seerr and pt-tools are on different machines across another network, go through an HTTPS reverse proxy.
- After 10 wrong keys from one address within a minute, requests from it are refused for 5 minutes. A revoked token stops working at once.
- The entrance accepts only tokens, never the web login; a token with only the Servarr compatible permission can reach only `/servarr/`, not the App API or anything else in the web interface.
