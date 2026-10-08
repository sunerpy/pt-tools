# Subscriptions

Once you subscribe to a film or one season of a series, pt-tools searches your sites on a schedule and also picks it up from RSS: it chooses the best release for your quality profile, pushes it to a downloader, and once the download completes, [library organising](media-library.md) puts it into your media library. For series it fills in missing episodes from TMDB's episode list; with upgrades on, it keeps looking for a better release and replaces the old one after the new one is organised. A Douban "wish list" (想看) can create subscriptions automatically.

The pages are Media → Explore (媒体 → 探索) and Media → Subscriptions (媒体 → 订阅). Subscriptions use TMDB, so enter a TMDB API key under [media recognition](media-recognize.md) first.

## Before you start

1. Turn on the Subscriptions (订阅) switch in Subscription settings (订阅设置) at the bottom of the Subscriptions page. While it is off nothing is searched, RSS items are not checked and Douban is not fetched; the subscription list says so at the top.
2. Active searches use each site's search, the same as the Torrent search (种子搜索) page: a site that finds nothing there finds nothing for subscriptions either.
3. A subscription only counts as in the library once the files are organised: turn on automatic organising under Media → Media library (媒体 → 媒体库), see [library organising](media-library.md).

## Subscribing

On the Explore page, browse TMDB's trending-this-week and popular lists, or search by name, and click Subscribe (订阅) under a title. Titles you have already subscribed to show Subscribed (已订阅); titles already in your library show In library (已入库). For a series, pick the season; leave it empty for the latest season. Each season can have one subscription; subscribe again for another season.

| Option                                   | Description                                                                                        |
| ---------------------------------------- | -------------------------------------------------------------------------------------------------- |
| Quality profile (质量档案)               | Which profile picks the release; Use the default profile (用设置里的默认档案) follows the settings |
| Only search these sites (只在这些站点找) | Leave empty for all enabled sites                                                                  |
| Downloader (下载器)                      | Leave empty for the default in the settings, then pt-tools' default downloader                     |
| Category, Tags (分类、标签)              | Sent to the downloader with the torrent                                                            |
| Save folder (保存目录)                   | As the downloader sees it; leave empty for the downloader's default folder                         |
| Upgrade (洗版)                           | Keep looking for higher-scoring releases after the first download, see Upgrades below              |

You can also subscribe from the bot: send `/sub <name>` and reply with a number from the list; for a series, add a season after the number, for example `2 3` subscribes to season 3 of the second title. `/subs` lists your subscriptions and their progress. `/sub` is admin-only, see the [ChatOps quick start](chatops-quickstart.md).

The Subscriptions list shows each subscription's progress, the result of the latest search or download, and when the next search runs. Active subscriptions can Search now (立即搜索) or Pause (暂停); paused ones have Resume (恢复); subscriptions created from Douban as pending start searching after you click Confirm (确认). Details (详情) lists every episode of the season and the torrents downloaded for it. Deleting a subscription leaves the torrents in the downloader and the files in the library alone.

## Finding releases

Two sources feed every subscription, and the results are picked together:

- **RSS**: items fetched by your existing [RSS subscriptions](rss-subscription.md) are also checked against your subscriptions once their details are fetched. This works independently of the RSS filter rules: a torrent that matches a subscription is pushed by the subscription.
- **Active search**: each subscription is searched once per Search interval (主动搜索的间隔), 12 hours by default and at least 6, plus up to 30 minutes of random offset so that the subscriptions do not all search at once. It searches by the English or original title first (with the year for films and the season for series, such as `The Last of Us S02`), then by the Chinese title. A subscription limited to certain sites only searches those; sites under Sites skipped by active search (不参与主动搜索的站点) only contribute through RSS.

A torrent matches a subscription when:

- both have an IMDb ID and the IDs are equal; when both have one, nothing else is compared;
- otherwise the Chinese or English title parsed from the torrent equals the subscription's title, original title or one of its TMDB alternative titles (ignoring spaces, punctuation and case), with recognition words applied as usual;
- for films, the years differ by one at most; when only the names are compared, the torrent's title or subtitle must state a year, since many films share a name. Anything with seasons and episodes is not a film;
- for series, the season matches; a multi-season pack containing the season also counts. Anime that only gives an episode number (such as `- 05`) counts as season 1.

The chosen torrent goes through the same push as RSS: disk space protection and site seeding capacity still apply, and the torrent record shows the source Subscription (订阅). A torrent is pushed once per subscription; a failed push, including one stopped by disk space protection or site seeding capacity, is not recorded, so the next search tries again.

## Quality profiles

Add quality profiles under Quality profiles (质量档案) on the Subscriptions page. Each subscription uses one profile; without any profile, releases are compared by resolution and source from best to worst.

| Item                                 | Description                                                                                                                       |
| ------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------- |
| Resolutions, sources, codecs         | Ranked in the order you pick them, earlier scores higher; once picked, anything else is rejected; none picked means no limit      |
| Remux, HDR                           | Any, prefer, require or avoid (不限、优先、必须有、不要)                                                                          |
| Chinese subtitles, free (中字、免费) | Any, prefer or require; Chinese subtitles are detected from the title and subtitle                                                |
| Preferred groups (偏好的制作组)      | A bonus, not a requirement                                                                                                        |
| Size                                 | Minimum and maximum in GB, 0 for no limit; series are measured per episode, so a season pack is divided by the number of episodes |
| Minimum seeders (最少做种数)         | Only for searched torrents: seeder counts mean little for a torrent freshly published to RSS                                      |
| No H&R (不要 H&R)                    | Rejects torrents with H&R requirements, including sites where every torrent has H&R                                               |

Scoring: the requirements above are checked first, then releases are compared by resolution, then source, then codec; a better value in an earlier item always wins, whatever the later items are. Releases equal in all three get a small bonus each for Remux, HDR, Chinese subtitles (when set to prefer) and preferred groups. On equal scores, free torrents come first when the profile prefers free, then more seeders, then the smaller size.

## Series

A series' progress follows TMDB's episode list for the season: episodes that are organised into the library, downloading, missing (aired but not in the library) or not aired yet.

- While the season airs, missing episodes are filled in one at a time, each with its highest-scoring release.
- Once the season has finished airing, a season pack is downloaded when half or more of the episodes are missing, or when no single-episode release covers the missing ones. No season packs are taken while the season airs, since those are usually incomplete.
- A downloaded torrent counts as organised once some of its files are in the library and the rest are no longer retried automatically; when a season pack leaves some episodes unorganised, the torrent says so, and those episodes count as missing for later searches.
- A downloaded torrent that is still not organised after 7 days is marked failed; its episodes count as missing again and later searches look for them.
- The subscription is done once the whole season has aired and every episode is in the library. A film is done once it is organised, or once a media server configured under Media library already has it. A film subscription without upgrades does not download anything when the film is already in the library.

## Upgrades

With upgrades on, a subscription keeps looking for higher-scoring releases after the first download until the version in the library reaches the first resolution and source in its profile (and Remux too when the profile prefers Remux; for a series, every episode must reach it). Without a ranked resolution and source in the profile, it stops after one download.

- Releases are compared with the current versions: the files already in the library for the title (including ones the subscription did not download) and the subscription's torrents that are downloading or organised. Their scores are recalculated with the current profile, so changing the profile keeps comparisons fair; failed downloads do not count.
- Films: a higher-scoring release is downloaded when found; no new one is pushed while the previous one is still downloading.
- Series: once the whole season is in the library, only season packs are considered, and only if they improve at least one episode; episodes whose current version is better than the pack keep it.
- After the new release is organised, the older files it covers that score no higher than it (equal scores too, so the library keeps one copy) are removed from the library by the rules for [deleting history records](media-library.md#organise-history): only files confirmed to be the ones pt-tools placed, and never moved files. The old record becomes Removed (已删除) and names the release that replaced it. For series, only the episodes the new torrent actually organised are replaced: when a season pack leaves some episodes unorganised, their old files stay, and an old file holding several episodes is replaced only when the new release has all of them. When the old and new releases organise to the same file name, the old one is removed first and the new one organised after it, but only when the new one scores higher; on equal scores the old file stays.
- Once none of an old subscription torrent's files remain in the library, it follows the setting: keep seeding (the default), or delete it with its data. Torrents that were already in the downloader when the subscription pushed them, and torrents with H&R requirements, are kept.

## Douban wish list

Under Douban wish list (豆瓣想看) on the Subscriptions page, add a Douban user ID, the part after `douban.com/people/` in the profile address. The collection must be public: pt-tools only fetches Douban's public collection RSS, without logging in or calling any non-public API.

- It is fetched every 6 hours (plus up to 30 minutes of random offset), and only films and series marked "wish" (想看) are taken; books, music, and items marked watching or watched are skipped.
- Each item is looked up on TMDB by its name (and the original title Douban gives). For a series, the season written in the title is used (such as 第二季), otherwise season 1. Titles that already have a subscription are only recorded, not subscribed twice.
- With Confirm first (先确认) on, the subscriptions it creates start as Pending (待确认) and only start searching after you click Confirm in the subscription list.
- Items not found on TMDB are recorded as not found and not searched again; you can search and subscribe to them from the Explore page.
- When you delete a subscription created from Douban, that item is not subscribed again; when an item disappears from Douban's RSS, the subscription it created is kept.
- Failed fetches back off for 1, 2, 4… hours, up to 24 hours; after 3 failures in a row the source is marked Abnormal (异常) and one notification goes to the channels selected in the settings. Without a TMDB API key the whole fetch counts as failed, so no items are recorded as not found.
- Fetch now (立即拉取) fetches right away (while the source is already being fetched, it asks you to try again later); Items (条目) lists the items seen and their results. Deleting a source keeps the subscriptions it created.

## Notifications

After choosing channels in Subscription settings, a message is sent whenever a subscription downloads a release: the subscription (with the season for series), the torrent, the site, the episodes and the quality, followed by the TMDB poster address. A Douban source that becomes abnormal also notifies once. Notifications are delayed during the channel's quiet hours and retried automatically on failure.

## Troubleshooting

**Active searches never find anything**: try the same name on the Torrent search page first to confirm the site finds it. The Latest (最近一次) column of the subscription list shows every result; when it only says nothing matching was found (没有找到对得上的资源), the torrent names on the site probably differ from TMDB's titles, and [recognition words](media-recognize.md) can help.

**Releases are found but not downloaded**: the Latest column names the torrents that did not meet the quality profile and why (for example 分辨率 1080p 不在档案里, resolution 1080p is not in the profile). Relax the profile, or give the subscription a different one.

**Downloaded, but the subscription never completes**: a subscription waits for the files to be organised. Check Media → Organise history (媒体 → 整理历史) for the torrent; with automatic organising off, completed torrents are not organised.
