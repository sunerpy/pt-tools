# Torrent search

Download → Torrent search (下载 → 种子搜索) searches your sites for torrents and pushes the one you pick to a downloader. There are two ways to search:

- **Keyword search**: type a keyword and press Enter. The results are a table, as in 0.48.0.
- **Search by title**: while typing, pick a film or series from the drop-down. pt-tools searches every site for that title, scores each result against a quality profile, merges copies of the same release into one card and, for a series, says which episodes a release would fill in.

Search by title, the parsed tags in the results and Filters (筛选), [blocking](#blocking) torrents you do not want and [waiting for free](#waiting-for-free) before downloading were added in the 1.0 preview.

## Keyword search

Type a keyword and press Enter or click Search (搜索). With no sites selected every enabled site is searched; the row below the box narrows it to a few. Sites that support categories each get a button that filters by the site's own categories. A search waits at most 30 seconds; sites that did not answer are listed under Sites without results (没返回的站点) and do not hold up the others.

| Toolbar                                        | What it does                                                                                                                                                         |
| ---------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| All, Films, Series, Anime, Music (全部、电影…) | Filters by the torrent's category; torrents whose site gives no category are counted separately and can be shown as well                                             |
| Free only (仅免费)                             | Only free torrents                                                                                                                                                   |
| Filters (筛选)                                 | Filters by the resolution, source, release group and so on parsed from the title, see [Filters](#filters)                                                            |
| Sort (排序)                                    | By seeders, leechers, snatches, size, upload time or site                                                                                                            |
| Columns (列设置)                               | Which columns the table shows                                                                                                                                        |
| Save search (保存搜索)                         | Saves the keyword, sites, sort order, category and Free only; saved searches are listed under Saved searches (已保存的搜索), and one click runs a saved search again |
| Recent searches (最近搜索)                     | The button to the right of the box lists the last 8 searches; click one to run it again                                                                              |

Under each title the row shows what was parsed from it: resolution, source, platform, codec, HDR, release group and Chinese subtitles, next to the site's category and H&R. Push to downloader (推送到下载器) asks for the downloader, save path, category and tags; the push goes through the usual disk space protection and site seeding capacity checks. For a series torrent, Follow (追剧) starts following the show from that torrent (see [follow and subscribe](media-subscribe.md#starting-from-a-torrent)). The More (更多) menu at the end of each row has Wait for free (等免费) and Don't want (不要), see [waiting for free](#waiting-for-free) and [blocking](#blocking) below.

## Search by title

### Picking a title

Type at least 2 characters and pause for half a second: the drop-down lists the matching series and films on TMDB (the first 5 of each), with the year, the type and whether you already subscribe to it or have it in the library. Click one, or select it with the arrow keys and press Enter: the title becomes a token in the box and the search by title starts. Pressing Enter without picking a suggestion runs a keyword search as before.

The Find releases (搜资源) button on a card in Media → Explore (媒体 → 探索) opens this page with that title. Search by title uses TMDB: released builds have a TMDB API key built in; builds from source need a key under [media recognition](media-recognize.md#tmdb-api-key) first.

The × on the token removes the title and returns to keyword search, with the box showing the keyword of the last keyword search. Typing in the box while a title is selected filters the results by torrent name; it does not search again.

### What is searched and what counts as a match

The search is the same as a subscription's active search: first the English or original name (films with the year, series with the season, such as `The Last of Us S02`), then the Chinese name; when you already subscribe to this season, the subscription's own name and aliases are used. Every enabled site is searched, waiting at most 60 seconds.

Torrents are matched to the title with the subscription rules (name or IMDb ID, year, season, see [finding releases](media-subscribe.md#finding-releases)). Torrents that do not match are hidden; the last line of the results says how many there are, and clicking it lists them with the reason. When the subscription has Title includes or Title excludes, torrents that fail them count as not fitting the profile.

Search by title only reads: it creates no subscription, pushes nothing, writes no run history and changes no subscription settings.

### The side panel

The side panel shows the title: poster, year and type, and for a subscription that it is being followed and the locked version; Title details › (条目详情 ›) opens the title's [details page](media-detail.md).

- **Season**: a series can switch season; the latest season is the default.
- **Episode cells**: one cell per episode, coloured by where the episode stands: in the library (green), downloading (blue), waiting for the locked version (primary colour), missing (orange), not aired yet (grey). The sentence below says which episodes are missing and which are downloading. Click a cell to show only results that contain that episode; click it again to clear.
- **Profile**: the quality profile of this season's subscription, or the default profile from the settings when there is no subscription or it has no profile. Pick another one or Ignore profile (不按档案); the choice only applies to this search and does not change the subscription.
- **Filters**: see [Filters](#filters).

In a window narrower than 1280 pixels the side panel folds into a row above the results with a Filters (筛选) button that opens the same content from the right.

### Result cards

Above the results: how many results there are, how many cards they were merged into and how many fit the profile; when sites did not answer, how many, with each site's reason on hover. Sort by profile score (the default), seeders, size or upload time; without a profile, the default is seeders.

One card is one release:

- **Profile score, top left**: the profile's score scaled to 0–100. A torrent whose resolution, source and codec are each the profile's first choice, with every preferred Remux, HDR, Chinese subtitles and preferred release group the profile asks for, scores 100. One that fails the profile shows Not a fit (不符合); without a profile it shows Ignore profile (不按档案).
- **Top right**: the episodes this release would fill in, for example "能补上 E08" (fills E08); "E07 正在下载（HDSky）" (E07 downloading from HDSky) when those episodes are already downloading; the reason when it does not fit the profile, for example "不符合：分辨率 1080p 不在档案里" (not a fit: resolution 1080p is not in the profile); for a film, whether it is already downloading or in the library. When the subscription has a version lock and this card is a different version, a second line says "不是锁定的版本（HHWEB）" (not the locked version).
- **Under the title**: the site's subtitle, size, upload time and the parsed tags.
- **Sites**: one row per site carrying this torrent, with seeders, the discount (time left for free torrents) and the H&R requirement. The first row is the preferred one, in the same order a subscription picks releases (see the scoring in [quality profiles](media-subscribe.md#quality-profiles)). Other sites are folded into "另有 N 个站点" (N more sites); open it to push from any of them.

Cards whose episodes are all in the library are folded; click one to open it.

Pushing opens the same push dialog as keyword search. The push carries the title, so when the download completes, library organising uses that title instead of recognising the torrent name (see [library organising](media-library.md#which-files-are-organised)). Torrents that do not fit the profile have a Push anyway (仍然推送) button and can be pushed as well. The dialog adds Create a folder per title (按条目建子目录), following the switch in the subscription settings by default: when on, the torrent goes into a folder named after the title under the save path (each downloader's default save path when none is given), see [per-title download folders](media-subscribe.md#per-title-download-folders); when a downloader's default save path cannot be read, the torrent is pushed to the original save path and the result says so.

The bottom right of each card has Wait for free (等免费) and Don't want (不要), see [waiting for free](#waiting-for-free) and [blocking](#blocking) below.

### Merging copies of a release

Merge same release (合并同一资源), at the right of the search bar, is on by default: the same release on different sites becomes one card. When sites give an InfoHash, torrents with the same InfoHash are merged; otherwise torrents with the same size (to the byte), resolution, release group and episode range are merged. A torrent without an InfoHash that looks like several releases with different InfoHashes gets a card of its own and is merged into none of them. With the switch off, each torrent on each site is its own card.

## Filters

Filters (筛选) is in the toolbar for keyword search and in the side panel for search by title. Both filter on what was parsed from the torrent titles:

| Group                   | Options                                                             |
| ----------------------- | ------------------------------------------------------------------- |
| Resolution              | 2160p, 1080p…, highest first                                        |
| Source                  | WEB-DL, BluRay, Remux…                                              |
| Platform                | Streaming platforms such as Netflix or Tencent Video (腾讯视频)     |
| HDR                     | HDR10, Dolby Vision (杜比视界)…                                     |
| Release group           | Most torrents first                                                 |
| Subtitles and discounts | Chinese subtitles, free, no H&R                                     |
| Codec, episodes, site   | Under More filters (更多筛选) in the side panel for search by title |

Several options in one group are combined with "or", groups with "and". The number next to an option is how many results remain with the other groups as they are now selected: ticking an option does not turn the other options in its own group into 0. Options with 0 cannot be ticked. A group with no options (for example when no result names a platform) is not shown. In keyword search, Filters applies to the results left after the category and Free only filters, and a new search clears it.

## Blocking

Click Don't want (不要) to block a torrent you do not want (a fake, out-of-sync audio, poor picture), optionally with a reason. Subscriptions no longer pick a blocked torrent: both torrents from RSS and torrents found by active search skip it, and the run log says "已拉黑：原因" (blocked: reason). A block is keyed on the site and the torrent ID, so the same release uploaded again elsewhere is not blocked; deleting a subscription and creating it again keeps the block.

| Where you click                                                                        | What is blocked                                                        | Which subscriptions it affects                                                            |
| -------------------------------------------------------------------------------------- | ---------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| Don't want (不要) on a card in search by title                                         | Every torrent on the card (the same release on several sites together) | This title only (for a series, this season)                                               |
| More → Don't want (更多 → 不要) on a keyword search row                                | This torrent                                                           | Every subscription                                                                        |
| Don't want this torrent (不要这个种子) under Torrents downloaded in a follow's details | This torrent                                                           | The subscription's title, see [follow and subscribe](media-subscribe.md#blocked-torrents) |

With Don't want on a card in search by title, when this season has a subscription you can also tick Not this release group again (以后也不要这个制作组), which adds the preferred torrent's release group to the subscription's title exclusions. Blocked torrents are hidden by default and the last line of the results says "已拉黑 N 条" (N blocked), with the title and the reason when there is only one. Click "已拉黑 N 条" to show them greyed out at the end; each site row has Unblock (移出黑名单), and a card whose torrents are all blocked also has Unblock all (全部移出黑名单) at the bottom right.

Torrents blocked in keyword search stay in the results with Blocked (已拉黑) under the title, and the More menu offers Unblock (移出黑名单) instead. Every blocked torrent is listed under Blocked torrents (拉黑的种子) on the Follow and subscribe (追剧与订阅) page, with the title it applies to, the reason and the time; you can unblock it there too.

## Waiting for free

For a torrent that is not free yet that you want to download once it is: click Wait for free (等免费) at the bottom right of a card in search by title (it uses the card's first torrent on a site that supports waiting), or More → Wait for free (更多 → 等免费) on a keyword search row. This opens the push dialog in wait-for-free mode: pick one downloader, fill in the save path, category and tags (as for a push), and choose how long to wait (1–30 days, 7 by default). There is no Start after adding (添加后立即开始) switch.

Every 5 minutes, a background task looks at the torrents that are due and reads the current discount from each torrent's details page:

- Free, with more free time left than Act early, minutes before freeleech ends (免费期结束前提前处理, see [configuration](../configuration.md#when-freeleech-ends); at least 5 minutes; permanent free torrents skip this check): pushed to the chosen downloader, through the usual disk space protection and site seeding capacity checks, and paused if it has not finished when the free period ends. A "等到免费了，已推送" (free now, pushed) notification is sent. A torrent that is already free when you create the wait is pushed straight away.
- Not free yet, not enough free time left, no known end of the free period, or the push was stopped by disk space protection or site seeding capacity: checked again in 30 minutes.
- The discount could not be read, the torrent file could not be downloaded or the push failed: retried after 1, 2, 4 and 4 hours; after 5 times in a row it stops waiting and sends a notification with the last reason.
- Not free by the time the wait ends: it stops waiting and sends a "等免费到期了" (wait for free ended) notification.

Only sites whose torrent details can be read support waiting; on other sites the button is greyed out. At most 200 torrents can be waiting at a time, and the same torrent can only have one wait. Notifications go to every enabled notification channel; for the events and their variables see [message templates](notify-channels.md#message-templates).

Download → Wait for free (下载 → 等免费) lists every wait: which downloader it goes to, its state (waiting, pushed, ended, stopped, cancelled), what the last check found, when it is checked next and how many times it was checked. A waiting one can be given a new end time (改到期; later than now, at most 30 days from now) or cancelled (取消). The page lists the latest 500.

## Phone

On a phone, search by title puts the season, the profile and a Filters (筛选) button in a row under the box. The title card shows the poster and the episode cells (tap the name for the title details), and the result cards show the same content as on the desktop, stacked, with Wait for free (等免费) and Don't want (不要) as the two halves at the bottom of each card. Filters open from the bottom; the selection only applies after tapping "Show N results" (看 N 条结果), and closing the panel without it changes nothing. The discount group on the phone also has 50%. When sites did not answer, a line of its own under the summary says so; tap it for each site's reason.

## Troubleshooting

**No suggestions while typing**: when TMDB is not set up or cannot be reached, no drop-down opens and Enter runs a keyword search as usual. Builds from source need a TMDB API key under [media recognition](media-recognize.md#tmdb-api-key) first.

**Search by title finds few results**: the last line of the results, "对不上这个条目的 N 条" (N do not match this title), counts the torrents that did not match; open it for the reasons. When the torrent names differ from the names on TMDB, add a recognition word under [media recognition](media-recognize.md). A title you do not subscribe to is searched with the names on TMDB only; a keyword search shows how the sites name its torrents.

**The profile score is 0 or says Not a fit**: the top right of the card says why. Try another profile or Ignore profile; neither changes the subscription.

**A wait for free never pushes**: on Download → Wait for free, the wait's note says what the last check found (for example "还没免费（现在是 50%）", not free yet, 50% now) or why the push did not happen. When disk space protection stopped the push, it is pushed at the next check once there is space.
