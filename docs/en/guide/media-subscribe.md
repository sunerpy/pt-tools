# Follow and subscribe <Badge type="tip" text="1.0 preview" />

Once you subscribe to a film or follow one season of a series, pt-tools searches your sites and also picks it up from RSS: it chooses the best release for your quality profile, pushes it to a downloader, and once the download completes, [library organising](media-library.md) puts it into your media library. A series is followed: pt-tools searches when each episode airs according to TMDB, fills in the season episode by episode and keeps to one release group and resolution; every search records what it found and why it skipped a torrent. With upgrades on, it keeps looking for a better release and replaces the old one after the new one is organised. A Douban "wish list" (想看) can create subscriptions automatically.

The pages are Media → Explore (媒体 → 探索) and Media → Follow and subscribe (媒体 → 追剧与订阅); in the torrent search results, Follow (追剧) starts following a show from that torrent. Subscriptions use TMDB: released builds have a TMDB API key built in, so there is nothing to enter; builds from source need a key under [media recognition](media-recognize.md#tmdb-api-key) first.

## Before you start

1. Turn on the Subscriptions (订阅) switch in Subscription settings (订阅设置) at the bottom of the Follow and subscribe page. While it is off nothing is searched, RSS items are not checked and Douban is not fetched; the subscription list says so at the top.
2. Active searches use each site's search, the same as the Torrent search (种子搜索) page: a site that finds nothing there finds nothing for subscriptions either.
3. A subscription only counts as in the library once the files are organised: turn on automatic organising under Media → Media library (媒体 → 媒体库), see [library organising](media-library.md).

## Subscribing

On the Explore page, browse TMDB's lists (trending this week, popular and top rated; now playing and upcoming for films, on the air and airing today for series), [find by country and genre](#explore), or search by name, and click Subscribe (订阅) under a film or Follow (追剧) under a series. Titles you have already subscribed to show Subscribed (已订阅); titles already in your library show In library (已入库). For a series, pick the season; leave it empty for the latest season. The dialog shows how many seasons TMDB has and how many episodes the season has. Each season can have one subscription; the dialog says so when the season is already followed, and you follow again for another season. A series also has follow settings, see Following a series below. Click a poster or a name to open its [title details](media-detail.md) with the title's details, how many episodes are in the library and the torrents downloaded for it.

| Option                                              | Description                                                                                                                                                                                                                                                                                                                                                                       |
| --------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Media library (整理进哪个媒体库)                    | Films and series can both pick one; only enabled libraries of the same type are listed, see [Media library](#media-library) below                                                                                                                                                                                                                                                 |
| Downloader (下载器)                                 | Leave empty for the default in the settings, then pt-tools' default downloader                                                                                                                                                                                                                                                                                                    |
| Category, Tags (分类、标签)                         | Sent to the downloader with the torrent                                                                                                                                                                                                                                                                                                                                           |
| Save folder (保存目录)                              | As the downloader sees it; pick one of the downloader's preset folders or type a path; leave empty for the downloader's default folder                                                                                                                                                                                                                                            |
| Quality profile (质量档案)                          | Which profile picks the release; Use the default profile (用设置里的默认档案) follows the settings                                                                                                                                                                                                                                                                                |
| Only search these sites (只在这些站点找)            | Leave empty for all enabled sites                                                                                                                                                                                                                                                                                                                                                 |
| Title includes, Title excludes (标题包含、标题排除) | Matched against the torrent title and subtitle: with include words, a torrent needs at least one of them; any exclude word rules it out. One per line or comma-separated, case-insensitive; spaces, dots and underscores count as the same. To take only one group's releases, enter the group, for example `CHDWEB`; to require several words together, enter a run of the title |
| Upgrade (洗版)                                      | Keep looking for higher-scoring releases after the first download, see Upgrades below                                                                                                                                                                                                                                                                                             |

When you create a subscription, the downloader, category, tags and save folder are filled in from the chosen media library (or the one Choose by type picks), with a note under each field; fields you changed yourself stay when you switch libraries, see [download settings for new subscriptions](media-library.md#download-settings-for-new-subscriptions).

### Trial search

Trial search (试搜索), under Title includes (标题包含) in the dialog, searches the sites once with the quality profile, sites, title includes and excludes the form has now. The results open on the right (full screen on a phone); nothing is pushed and the subscription is not saved. It works when creating and when editing; when editing it uses the form's unsaved changes. Slow sites get at most one minute.

The results have three tabs:

- **Matches (符合条件)**: torrents the quality profile, title includes and excludes, and blocks do not rule out. A match is not necessarily downloaded: each group says which episodes it can fill, or whether those episodes are already in the library or downloading.
- **Rejected (不符合)**: with every reason, for example 标题含排除词 hdtv (the title contains the exclude word hdtv); blocked torrents say they are blocked.
- **Not this season (对不上这一季)** (for a film, 对不上这部电影): torrents whose name, year or season does not match the title, with the reason.

At the top: the search words (TMDB's names), how long it took, which episodes of the season are missing, and which sites failed. With the version lock on and a group and resolution locked in the form, groups that do not match it are marked 版本锁不收 (not taken by the version lock); with nothing locked yet, it says the first release that downloads becomes the lock.

The actions on a result only change the form; they take effect when you click Save (保存), or Follow (开始追) and Subscribe (订阅) when creating:

| Action                                                      | What it changes                                                                                                                                                                                                                                                                                                                                       |
| ----------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Copy title, Copy subtitle (复制标题、复制副标题)            | Copies the selected torrent of a group; when a group has torrents from several sites, click a row to select it                                                                                                                                                                                                                                        |
| Only / Not this group (只要「组名」、不要「组名」)          | Adds a line with the group to Title includes or Title excludes; shown when a release group was recognized                                                                                                                                                                                                                                             |
| Pick part of the title (选标题里的一段…)                    | The title is cut at spaces, dots, underscores and brackets; click the first and last block of the part you want (click a block twice to take only it) and add it to Title includes or Title excludes. When the part contains an episode number (for example `S02E03` or `第 3 集`), it warns that adding it to Title includes takes only that episode |
| Lock this version (锁这个版本)                              | Series: turns the version lock on and locks this group's release group and resolution. A new subscription is locked from the start instead of waiting for the first release that downloads                                                                                                                                                            |
| Start from this torrent when saved (保存时从这个种子开始追) | New series: pushes this torrent as soon as it is created (as in [Starting from a torrent](#starting-from-a-torrent)) and sets the first episode to the torrent's first episode; a season pack leaves the first episode unchanged. Click 不从它开始 to undo                                                                                            |

A word that is already there is not added again; when adding it would go past 1024 bytes, it is not added and you are asked to remove some words first.

After you change the form, the top of the results says the form changed (表单改过了); click Try again (再试一次) to search with the new form. Site search results are cached for 10 minutes: trying again after changing only Title includes, Title excludes or the quality profile is quick and does not search the sites again; a run with a failing site is not cached, and trying again searches the sites again.

### Explore

Find by country and genre (按国家与类型找) lists TMDB's films or series by country or region of origin (mainland China, Hong Kong, Taiwan, Japan, South Korea, the United States, the United Kingdom) and genre (animation and documentary; series also have variety, which is TMDB's reality and talk genres), sorted by popularity; either filter can be left open. TMDB lists and search results are cached for 6 hours.

The anime calendar (番剧日历) lists, day by day, the anime of the current season in the daily schedule of [Bangumi](https://bgm.tv), starting on today; the data is cached for 6 hours. When you click a title, pt-tools strips suffixes such as 第二季, 第2期 or 2nd Season from its Chinese and original names and looks the series up on TMDB: only animation counts, and its first air year may be at most one year after this season started. When exactly one result has the same Chinese or original name, it is used; when none has the same name, a single result whose name starts with the other is used. A match opens the [title details](media-detail.md), where you can follow it; when pt-tools is not sure or finds nothing, it names the titles it searched for, and Search by name (按名字搜索) lets you pick on TMDB yourself.

You can also subscribe from the bot: send `/sub <name>` and reply with a number from the list; for a series, add a season after the number, for example `2 3` subscribes to season 3 of the second title. `/follow <show>` (or `/追剧`) lists series only and uses the follow defaults. `/subs` lists your subscriptions with their IDs, progress and next search, and `/runs <id>` shows the latest runs. `/sub` and `/follow` are admin-only, see the [ChatOps quick start](chatops-quickstart.md).

The list shows each subscription's progress, the result of the latest search or download, and when the next search runs; a followed series also gets a small square per episode and its schedule and version lock. Active subscriptions can Search now (立即搜索) or Pause (暂停); paused ones have Resume (恢复); subscriptions created from Douban as pending start searching after you click Confirm (确认). Details (详情) lists every episode, the run history and the torrents downloaded. Click a subscription's name to open the title's [details page](media-detail.md). Media → Calendar (媒体 → 日历) lists by day when each subscribed episode airs, see [the subscription calendar](media-detail.md#subscription-calendar). Deleting a subscription leaves the torrents in the downloader and the files in the library alone.

## Following a series

A series subscription is followed. Series created with Follow on the Explore page or in the search results, or with `/follow` in the bot, follow the air time with the version lock on by default. Series subscriptions created earlier, and those from `/sub` or Douban, keep searching at the global interval as before; switch them to the follow settings under Edit (修改).

### Starting from a torrent

Click Follow (追剧) on a torrent in the search results: pt-tools first recognises the show and season from the torrent's title and subtitle (pick another candidate in the dialog if it guessed wrong), creates the follow and pushes that torrent right away. The start episode defaults to the torrent's episode, so earlier episodes are not wanted. The torrent is not pushed when it fails the quality profile or those episodes are already there; the reason goes into the follow's message. A torrent recognised as a different show or season is not used. When the show cannot be recognised, correct it under [media recognition](media-recognize.md) first, or search for the show on the Explore page.

### When to search

| Schedule                      | Description                                                                                                                                                                                                                                                                                                                      |
| ----------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Air time (按播出时间)         | The default. Each episode's TMDB air date plus the air time (in the time zone of the machine pt-tools runs on, 20:00 by default): the search starts 2 hours after airing, runs again every 6 hours while nothing is found, and once a day after 7 days. With no episode to wait for, TMDB is checked once a day for new episodes |
| Every so often (每隔一段时间) | Every 30 minutes to 7 days (in minutes, hours or days), counted from the last search                                                                                                                                                                                                                                             |
| Daily (每天)                  | Searches once a day at the time                                                                                                                                                                                                                                                                                                  |
| Every few days (每隔几天)     | Every 1 to 30 days at the time, counted from the day it was created                                                                                                                                                                                                                                                              |
| Weekly, monthly               | On the chosen weekdays or days of the month (a shorter month uses its last day) at the time                                                                                                                                                                                                                                      |
| Cron expression (cron 表达式) | Write the times yourself, for shows such as variety programmes that air on fixed days and hours; see below                                                                                                                                                                                                                       |
| Global interval               | The interval in Subscription settings, like films                                                                                                                                                                                                                                                                                |

A cron expression has 5 space-separated fields: `minute hour day month weekday`. Each field takes `*` (any), a number, a range such as `1-5`, a step such as `*/2`, or a list such as `8,20`. Weekday 0 and 7 are both Sunday, and `MON`–`SUN` and `JAN`–`DEC` work too. Separate several expressions with semicolons (or new lines); the earliest one due wins. When both the day and the weekday are restricted, either one matching is enough, as in crontab. The shortcuts `@daily`, `@weekly`, `@monthly` and the like are accepted. Some examples:

| Expression                 | Searches at                           |
| -------------------------- | ------------------------------------- |
| `0 20 * * *`               | 20:00 every day                       |
| `0 8,22 * * *`             | 8:00 and 22:00 every day              |
| `0 22 * * 5,6`             | 22:00 on Fridays and Saturdays        |
| `0 22 * * 5; 0 9,13 * * 6` | 22:00 Friday, 9:00 and 13:00 Saturday |
| `0 */2 * * 1-5`            | Every 2 hours on weekdays             |
| `0 9 1 * *`                | 9:00 on the 1st of every month        |

While you edit the schedule, the dialog works out the next few search times, and a wrong expression is reported with the field that is off.

How many hours after airing to start, how many hours between retries, and for how many days after airing to retry can all be changed under Edit. Scheduled searches get a few minutes of random offset, and two searches are at least 30 minutes apart. A followed season's episode list is read from TMDB again every 6 hours, so new episodes are picked up on their own.

### Version lock

With the version lock on, once the first release is downloaded its version (release group and resolution, such as `HHWEB 1080p`) is kept for the rest of the season, so one season never mixes several groups. Until that happens, a version already in the library or downloading becomes the locked one.

When you click Follow (追剧) on a search result and that torrent cannot be pushed (for example, a Cloudflare challenge on the site blocks the torrent file), its version is still locked: Search again (重新搜索) keeps looking for that version instead of switching to a higher-scoring one.

- When the locked version still has not shown up after the retry days, the episode shows Waiting for the version (等版本). By default pt-tools keeps waiting rather than switching, and notifies once through the channels in the settings (then at most every 3 days). An episode whose locked version was found on a site but whose torrent file could not be downloaded is not waiting: it shows Missing (缺) and is searched for again.
- To take another version for one episode, click Use another version for this episode (这一集换版本) on that episode in the follow details, or send `/unlock <id> <episode>` to the bot. Only that episode is released from the lock and a search runs within a minute; the others stay as they are.
- To stop waiting after a while, set Switch version after waiting this many days (版本锁等满几天改用别的版本) in Subscription settings (0 to 90; the default 0 waits forever). When an episode has been waiting for the version that many days and has been searched at least once since it started waiting, it is released from the lock as if you had clicked Use another version for this episode, and a search runs straight away; the run history says it switched by the setting (按设置改用别的版本). The locked version stays the same and the other episodes keep looking for it, so one season may end up with two versions.
- When the original release group stops releasing, change the locked version under Edit. Found in the last search (最近一次搜索找到) lists the versions that search found which can fill the missing episodes, with the episodes each one covers; choose one to lock to it. The list is empty until the next search runs; to fill it right away, click Search now (立即搜索) in the list. You can also enter the release group and resolution yourself; leaving one empty means any value for it (for example, enter only the group to take that group in any resolution). With both cleared (清空), the next successful download becomes the locked version, and the episodes already in the library stay and no longer decide the lock. A search with the new lock runs soon after you save.
- Under Edit you can also turn the version lock off. Replacing the season with a pack during an upgrade is not limited by the lock; the new version becomes the locked one.

### Episodes

The follow details show one square per episode: in library, downloading, missing (aired but not there), waiting for the version, not aired, and not followed (before the start episode, or skipped). Click an episode to see its name, air date and current version, and to Skip this episode (跳过这一集; it no longer counts as missing or holds up completion) or Want this episode again (重新要这一集).

Torrents for the show that are already in the subscription's downloader but were not pushed by it, such as ones you downloaded yourself, count too, so they are not downloaded twice. Their episodes come from the files selected in the torrent, so a torrent named after the whole season (some sites post one torrent per episode under the season's folder name) or a season pack with only a few episodes selected counts correctly; when the file list cannot be read the torrent name is used, and one naming the whole season is left out for now. Torrents downloaded by [brush tasks](brush.md) don't count, because brushing removes them by its own rules (free period ending, slow uploads). When such a torrent is deleted from the downloader, its episodes are searched again right away, as for the subscription's own torrents (see [Series](#series) below). One that is still not organised after 7 days also counts as failed and the episodes are searched again: automatic organising only handles torrents pt-tools pushed, so organise it by hand or turn on the periodic scan under [library organising](media-library.md).

Under Edit you can change the episode to start from (earlier ones are not wanted), and enter the season's episode count when TMDB's is wrong.

### Variety show episodes

Many variety shows split one issue into two TMDB episodes, while torrents on the sites number them by TMDB, write 第 1 期下 or E01 Part 2 (issue 1, part 2), or give only the air date (`20261002`, `1015期`). When pt-tools sees that a season is numbered by issue (its TMDB genre is reality or talk, or half or more of the episode names are 第几期), it converts the issue and part, or the air date, into the TMDB episode number. Anything it is unsure about is skipped, and the run history says the episode number does not match (集号对不上). When the title has an episode number and only the subtitle names the issue, the two have to agree (subtitles are often wrong); if they do not, the torrent is skipped too. The one exception: when, by TMDB's air dates, the issue in the subtitle cannot have aired yet (for example only issue 3 has aired and the subtitle says issue 5) while the episode in the title has aired, the subtitle is taken to have written the episode number as the issue, and the title's episode number is used. Once such a torrent is pushed, pt-tools records an episode-number rule for that release group, by the TMDB episode number (its source shows as Automatic, 自动), and from then on uses the title's episode numbers for that group: early in a season, when the subtitle's issue is only one issue ahead of what has aired, and late in a season, once TMDB lists the issue the subtitle names, the exception above cannot decide. Delete the rule under Episode-number rules (集号规则) if it is wrong.

When a release group's numbering is converted wrongly, give it a rule under Episode-number rules (集号规则) in the follow details: by the TMDB episode number, by issue and part, by air date, or an offset (TMDB episode = the number in the title + offset). A rule without a group applies to every group. A search under the new rule runs within a minute.

### Episode groups

Besides the default seasons, a series on TMDB can have other orderings, called episode groups: absolute numbering, common for anime (episode 25), or seasons as split on DVD or at broadcast. When the torrents on the sites number episodes in another ordering, choose that episode group of the series under Edit (修改) of the followed series; the option only shows when the series has episode groups on TMDB.

- Seasons and episodes in torrent titles and file names are first converted from the episode group to TMDB's default seasons, then matched and mapped. A title that only gives an episode number (season taken as 1) counts in the group's first part, so absolute episode 25 becomes season 2, episode 1. Active search leaves the season off the English title (torrents number episodes by the group, so TMDB's season number would find nothing).
- Progress, the calendar and the title details, as well as the file names and scraping in the library, all use TMDB's default seasons.
- An episode the group does not list is counted by TMDB's default seasons, noted in the run history and the organise record. When matching a torrent, converted episodes that are not consecutive mean the torrent is not used; when organising, converted episodes that span two seasons or are not consecutive are organised by the season and episode in the file name.
- When the episode group cannot be read (TMDB temporarily unavailable), torrents are not matched by the group in this round, and organising is retried later. When the group has been deleted on TMDB, this is reported; choose another group or none under Edit.

### Run history

Every active search, RSS match, creation and change by hand (marked Changed, 改动) records a run: how many torrents were found on which sites, which torrents were skipped and why (fails the quality profile, not the locked version, episode number does not match…), and what was pushed. Click a run in the follow details to see the steps, or send `/runs <id>` to the bot. Each subscription keeps its latest 100 runs for at most 90 days.

For an active subscription, Search now (立即搜索) sits above the run history. When the latest run did not succeed (it failed, or a torrent failed to push), the button reads Search again (重新搜索): a torrent that failed to push was not recorded, so searching again pushes it once more.

### Downloaded torrents

At the bottom of the follow details is every torrent the subscription downloaded, and under each torrent how each video file was organised and scraped: organised, failed (with the reason) or skipped, whether a failed file is being retried automatically, and whether an organised file was scraped fully or partly. A season pack works the same as a single episode, with one set of buttons per torrent:

- Organise again (重新整理): shown when a file of the torrent was not organised and is not being retried, or the torrent was marked failed after 7 days without reaching the library. It organises the torrent with the subscription's title and library (no recognition) and leaves organised files alone; a torrent marked failed goes back to waiting, and is marked in the library as soon as organising succeeds.
- Scrape again (重新刮削): shown when a file is organised. It rewrites the NFO and images of the show (or movie) with the library's current scrape settings, the same as Scrape again in the organise history.
- Don't want this torrent (不要这个种子): shown for every torrent downloaded from a site (not for torrents adopted from the downloader). It blocks the torrent, see [blocked torrents](#blocked-torrents) below.

Each of them is recorded in the run history. An old version replaced by an upgrade has neither Organise again nor Scrape again. A single file can also be retried or scraped again under Media → Organise history (媒体 → 整理历史).

### After the season is done

Once every episode to follow (from the start episode on, not skipped) is in the library, the follow is done. If TMDB adds aired episodes to the season within 30 days after that (an extra episode or a special numbered into the season), it becomes active again. With Then follow the next season (追完以后接着追下一季) on, once TMDB has an air date for the next season within a year, a follow for the next season is created with this season's settings (the version lock stays on and the locked version is decided again).

### Media library

You can pick the media library when you create or edit a subscription, for films too; the list shows only enabled libraries of the same type. With a library picked, torrents the subscription pushed, and downloader torrents counted towards it, are organised into that library instead of one chosen by type. When the library is later deleted or switched off, or its type doesn't match the item, organising fails with an explanation; pick another library in the subscription.

Choose by type (按类型自动选) saves the choice as automatic, so the library is chosen by type when organising and an animation library added later is used too. When you create a subscription it shows which library it would pick now, and says organising will fail when there is no enabled library of that type. The download settings of new subscriptions are filled in from this library too, see [download settings for new subscriptions](media-library.md#download-settings-for-new-subscriptions).

These torrents are also organised as the subscription's item instead of being recognised from the torrent name (organise history shows 按订阅的条目), so a torrent whose name is not recognised, or is recognised as another series, still ends up in this series. Files corrected by hand or organised before keep the item they were organised as. A subscription also records its item on the torrents it pushes, so they are organised the same way after the subscription is deleted.

## Finding releases

Two sources feed every subscription, and the results are picked together:

- **RSS**: items fetched by your existing [RSS subscriptions](rss-subscription.md) are also checked against your subscriptions once their details are fetched. This works independently of the RSS filter rules: a torrent that matches a subscription is pushed by the subscription.
- **Active search**: films and series on the global interval are searched once per Search interval (主动搜索的间隔), 12 hours by default and at least 6, plus up to 30 minutes of random offset so that the subscriptions do not all search at once; followed series search on their own schedule (see Following a series above). It searches by the English or original title first (with the year for films and the season for series, such as `The Last of Us S02`; a series with an [episode group](#episode-groups) gets no season), then by the Chinese title. A subscription limited to certain sites only searches those; sites under Sites skipped by active search (不参与主动搜索的站点) only contribute through RSS.

To see which releases the sites have right now and pick one yourself, use [search by title](torrent-search.md#search-by-title) on the Torrent search page: it searches and matches the same way, scores releases with the subscription's quality profile and says which episodes each one would fill in.

To see which torrents a subscription would take before you change its title includes, excludes or quality profile, click [Trial search](#trial-search) in the subscribe dialog.

A torrent matches a subscription when:

- both have an IMDb ID and the IDs are equal; when both have one, nothing else is compared;
- otherwise the Chinese or English title parsed from the torrent equals the subscription's title, original title or one of its TMDB alternative titles (ignoring spaces, punctuation and case), with recognition words applied as usual;
- for films, the years differ by one at most; when only the names are compared, the torrent's title or subtitle must state a year, since many films share a name. Anything with seasons and episodes is not a film;
- for series, the season matches; a multi-season pack containing the season also counts. Anime that only gives an episode number (such as `- 05`) counts as season 1. Years are not compared for series: variety show torrents often give the broadcast year, which differs from the year of the season on TMDB.

The chosen torrent goes through the same push as RSS: disk space protection and site seeding capacity still apply, and the torrent record shows the source Subscription (订阅). A torrent is pushed once per subscription; a failed push, including one stopped by disk space protection or site seeding capacity, is not recorded, so the next search tries again.

Downloading the torrent file is retried twice, with exponential backoff (about 2 and then 4 seconds, each retry first waiting for the site's rate limit), after a temporary network error (connection reset, timeout) or a server error (5xx). This applies to every site; for sites that first fetch a download address (M-Team, NexusPHP) only that address is fetched again. A site whose download fails twice in a row in one round is skipped for the rest of the round. A round pushes at most 10 torrents, failed ones included. When torrent files could not be downloaded, the limit was reached with episodes still missing, or a search by hand ran out of time, the next search moves up to 30 minutes later, doubling (1, 2, 4 hours) while runs keep failing; an earlier regular search time is kept. A torrent from RSS whose file could not be downloaded moves the next active search up the same way.

## Review by AI before downloading

Turn on Review with AI before downloading (下载前让 AI 复核, off by default) in the subscription settings (订阅设置), and before pushing a torrent a subscription found, pt-tools gives the model of the [AI assistant](assistant.md) the subscription's title, original title, year (for a series, the year the season started), season and alternative titles, plus the torrent's title, subtitle and parsed details (for a series, also the season and episodes it matched), and asks whether the torrent is this title (and, for a series, this season), expecting yes or no and one sentence of reasoning. Set up and enable a model in the AI assistant settings (AI 助手设置) first.

- By default only titles matched by name are reviewed (只复核按名字对上的): a torrent matched by IMDb ID, with both it and the subscription carrying one, is not asked about. Choose All (全部) to review every torrent about to be pushed.
- A torrent the model says no to is not pushed in this round; the run history records `AI 复核：不是` with the reason, and the next candidate is considered (when every season pack gets a no, missing episodes are filled one by one). A torrent it says yes to is pushed as usual, and the run history records the reason and how many tokens the review used.
- When the model fails (the assistant is not enabled, cannot be reached, or does not answer within 30 seconds), the torrent is pushed as usual, the run history records `AI 复核没做成` with the cause, and the rest of the round's candidates are not reviewed. When the answer is neither yes nor no, only that torrent is pushed without review.
- Each run asks about 10 torrents at most; when the limit is reached, the remaining candidates wait for the next search, which moves up to 30 minutes later. The verdict for a torrent in a subscription is kept for 24 hours and not asked again (it is asked again after pt-tools restarts).
- A torrent you chose yourself when [starting from a torrent](#starting-from-a-torrent) is not reviewed.

Every torrent about to be pushed waits for the model's answer and uses the AI assistant's tokens.

## Per-title download folders

Turn on Create a folder per title when downloading (下载时按条目建子目录, off by default) in the subscription settings, and torrents pushed by subscriptions go into a folder named after the title under the save path, such as `/downloads/最后生还者 (2023)`:

- The save path is the subscription's; without one it is the downloader's default save path (the default save path in qBittorrent's settings, Transmission's download-dir). When the downloader's default save path cannot be read, this push creates no folder and the run history says so.
- The folder name template is `{title} ({year})` by default and can use `{title}`, `{original_title}`, `{year}` (for a series, the year of its first broadcast), `{season}` and `{tmdb_id}`. An unknown value is left empty, empty brackets are removed, and characters not allowed in names are replaced; when the name comes out empty, no folder is created.
- The downloader keeps the torrent in that folder: qBittorrent also receives `autoTMM=false`, so these torrents are not under automatic management (with automatic management on, qBittorrent ignores the save path); Transmission follows download-dir anyway.
- The push dialog of search by title has the same Create a folder per title (按条目建子目录) switch, following this setting by default and using the same template; see [search by title](torrent-search.md#search-by-title).
- Library organising reads the content path the downloader reports and is not affected. When automatic organising is limited by save path, the scope must include these folders; see [when organising runs](media-library.md#when-organising-runs).

## Quality profiles

Add quality profiles under Quality profiles (质量档案) on the Follow and subscribe page. Each subscription uses one profile; without any profile, releases are compared by resolution and source from best to worst.

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

A series' progress follows TMDB's episode list for the season: episodes that are organised into the library, downloading, missing (aired but not in the library), waiting for the locked version, not aired yet, or not followed.

- While the season airs, missing episodes are filled in one at a time, each with its highest-scoring release.
- Once the season has finished airing, a season pack is downloaded when half or more of the episodes are missing, or when no single-episode release covers the missing ones. No season packs are taken while the season airs, since those are usually incomplete.
- When a season pack (or a torrent holding several episodes) fills missing episodes, only those episodes are downloaded: after the push, pt-tools sets the video files of episodes that are already in the library, downloading elsewhere or not followed to Do not download in the downloader (qBittorrent and Transmission), and organizing skips those files. A torrent with none of the missing episodes is not downloaded. When the downloader does not support this or the change fails, the whole torrent downloads and the run history says so. Subscriptions with upgrades on still take the whole pack, since every episode already in the library is compared with it too.
- When an episode that was set to Do not download goes missing later (the torrent downloading it elsewhere was deleted, or the episode was deleted from the library), the same torrent is downloaded again for the missing episodes only. If the torrent is still in the downloader, those files are set back to download; otherwise the torrent is added again. The push uses the current promotion and download settings. The torrent counts as organised once those episodes are organised; when it has finished downloading but nothing is organised after 10 minutes, the subscription asks library organising to organise it with the subscription's title (at most once an hour).
- A downloaded torrent counts as organised once some of its files are in the library and the rest are no longer retried automatically; when a season pack leaves some episodes unorganised, the torrent says so, and those episodes count as missing for later searches.
- A downloaded torrent that disappears from the downloader (deleted by brushing, by hand or by another tool) is marked failed once it has been missing for 10 minutes in a row. Its episodes count as missing again, a search runs right away, and the run history records it. Nothing is marked while the downloader can't be read or lists no torrents, and a torrent pushed or organised again less than 10 minutes ago waits until its record is 10 minutes old.
- A downloaded torrent that is still not organised after 7 days is marked failed; its episodes count as missing again and later searches look for them.
- The subscription is done once every episode to follow has aired and is in the library (for followed series, see After the season is done above). A film is done once it is organised, or once a media server configured under Media library already has it. A film subscription without upgrades does not download anything when the film is already in the library.

## Upgrades

With upgrades on, a subscription keeps looking for higher-scoring releases after the first download until the version in the library reaches the first resolution and source in its profile (and Remux too when the profile prefers Remux; for a series, every episode must reach it). Without a ranked resolution and source in the profile, it stops after one download.

- Releases are compared with the current versions: the files already in the library for the title (including ones the subscription did not download and [imported existing files](media-library.md#importing-an-existing-library)) and the subscription's torrents that are downloading or organised. Their scores are recalculated with the current profile, so changing the profile keeps comparisons fair; failed downloads do not count.
- Films: a higher-scoring release is downloaded when found; no new one is pushed while the previous one is still downloading.
- Series: once the whole season is in the library, only season packs are considered, and only if they improve at least one episode; episodes whose current version is better than the pack keep it.
- After the new release is organised, the older files it covers that score no higher than it (equal scores too, so the library keeps one copy) are removed from the library by the rules for [deleting history records](media-library.md#organise-history): only files confirmed to be the ones pt-tools placed, never moved files and never imported existing files (their records stay organised). The old record becomes Removed (已删除) and names the release that replaced it. For series, only the episodes the new torrent actually organised are replaced: when a season pack leaves some episodes unorganised, their old files stay, and an old file holding several episodes is replaced only when the new release has all of them. When the old and new releases organise to the same file name, the old one is removed first and the new one organised after it (if the new one then fails to organise, the old one is organised back), but only when the new one scores higher; on equal scores the old file stays. When the old one is an imported existing file it isn't removed and the new release is skipped when organising, see [importing an existing library](media-library.md#importing-an-existing-library).
- Once none of an old subscription torrent's files remain in the library, it follows the setting: keep seeding (the default), or delete it with its data. Torrents that were already in the downloader when the subscription pushed them, and torrents with H&R requirements, are kept.

## Blocked torrents

Subscriptions no longer pick a blocked torrent: both torrents from RSS and torrents found by active search skip it, and the run history says "已拉黑：原因" (blocked: reason). A block is kept per title (for a series, per season), so deleting a subscription and creating it again keeps it; a block made in keyword search applies to every subscription. For where you can block a torrent, see [torrent search](torrent-search.md#blocking).

Don't want this torrent (不要这个种子) under Downloaded torrents in the follow details:

- You can give a reason, and tick Not this torrent's release group again (以后也不要这个种子的制作组) to add the release group from the torrent name to the subscription's title exclusions (a group already there is not added twice).
- A torrent that is still downloading is marked failed with "不要这个种子：原因" (don't want this torrent: reason); its episodes count as missing again and the subscription searches again straight away. Files already organised into the library are left alone.
- You can tick Also delete this torrent from the downloader (同时从下载器删掉这个种子), and then also the downloaded files; you confirm once more before the deletion.
- A blocked torrent is marked Blocked (已拉黑), and a blocked torrent marked failed cannot be organised again (that would set it back to downloading). Unblock it first to organise it.

Blocked torrents (拉黑的种子) on the Follow and subscribe page lists every blocked torrent: the torrent name, site and ID, the title the block applies to ("所有条目", every title, for blocks made in keyword search), the reason and the time. After Unblock (移出), subscriptions pick it again.

## Douban wish list

Under Douban wish list (豆瓣想看) on the Follow and subscribe page, add a Douban user ID, the part after `douban.com/people/` in the profile address. The collection must be public: for the wish list, pt-tools only fetches Douban's public collection RSS, without logging in or calling any non-public API.

- It is fetched every 6 hours (plus up to 30 minutes of random offset), and only films and series marked "wish" (想看) are taken; books, music, and items marked watching or watched are skipped.
- Each item is looked up on TMDB by its name (and the original title Douban gives). For a series, the season written in the title is used (such as 第二季), otherwise season 1. Titles that already have a subscription are only recorded, not subscribed twice.
- With Confirm first (先确认) on, the subscriptions it creates start as Pending (待确认) and only start searching after you click Confirm in the subscription list.
- Items not found on TMDB are recorded as not found and not searched again; you can search and subscribe to them from the Explore page.
- When you delete a subscription created from Douban, the item shows Subscription deleted (订阅已删除) and is not subscribed again; when an item disappears from Douban's RSS, the subscription it created is kept.
- Failed fetches back off for 1, 2, 4… hours, up to 24 hours; after 3 failures in a row the source is marked Abnormal (异常) and one notification goes to the channels selected in the settings. Without a usable TMDB API key the whole fetch counts as failed, so no items are recorded as not found.
- Fetch now (立即拉取) fetches right away (while the source is already being fetched, it asks you to try again later); Items (条目) lists the items seen and their results. Deleting a source keeps the subscriptions it created.

## Notifications

After choosing channels in Subscription settings, a message is sent whenever a subscription downloads a release: the subscription (with the season for series), the torrent, the site, the episodes and the quality, followed by the TMDB poster address. A followed series sends Followed show updated (追剧更新) with the locked version as well; Waiting for the version (追剧等版本, with the `/unlock` command) when the locked version takes too long; and Followed show failing (追剧出错) after 3 runs in a row could not search (for example every site timed out). A Douban source that becomes abnormal also notifies once. With Today's updates (今日更新) on, the episodes airing today and the films released today are pushed every day at the chosen time, see [today's updates](media-detail.md#updates-airing-today). Notifications are delayed during the channel's quiet hours and retried automatically on failure; the text can be changed with [message templates](notify-channels.md).

## Troubleshooting

**Active searches never find anything**: try the same name on the Torrent search page first to confirm the site finds it. The Latest (最近一次) column of the subscription list shows every result; when it only says nothing matching was found (没有找到对得上的资源), the torrent names on the site probably differ from TMDB's titles, and [recognition words](media-recognize.md) can help.

**Releases are found but not downloaded**: the Latest column names the torrents that did not meet the quality profile and why (for example 分辨率 1080p 不在档案里, resolution 1080p is not in the profile). Relax the profile, or give the subscription a different one.

**Downloaded, but the subscription never completes**: a subscription waits for the files to be organised. Downloaded torrents in the follow details shows how each file was organised, with the reason for any that were not; once that is fixed (for example a path mapping added or another library picked), use Organise again. With automatic organising off, completed torrents are not organised.

**A followed series keeps showing Waiting for the version**: the locked release group has not released the episode yet, or has stopped. Use another version for this episode in the follow details releases only that episode; to change the whole season, clear the locked version under Edit; to switch automatically after some days, set Switch version after waiting this many days in Subscription settings.

**Variety show episodes do not match**: check why the run history skipped the torrents. For episode number does not match (集号对不上), give that release group an episode-number rule in the follow details (for example by issue and part). Title include words do not let such a torrent through: the episode number is checked before the title keywords.
