# Title details and the subscription calendar <Badge type="tip" text="1.0 preview" />

The title details page puts a film or series as TMDB describes it next to where it stands on your side: how many episodes are in the library, which torrents are seeding, the subscription, whether the media server has it, whether each episode is in the library, and which torrents were downloaded for it; when you no longer want a title, you delete its library files, torrents and subscription from here. The subscription calendar lists, day by day, when each episode of a subscribed series airs and when a subscribed film is released, and whether it is in the library yet; once a day it can also push today's updates to your notification channels.

Both use TMDB data. Released builds have a TMDB API key built in, so there is nothing to fill in; if you build from source, enter a key under [media recognition](media-recognize.md#tmdb-api-key) first.

## Opening title details

You can open title details from:

- Media → Explore (媒体 → 探索): click a poster or a name.
- Media → Follow and subscribe (媒体 → 追剧与订阅): click a subscription's name; for a series it opens the subscribed season.
- Media → Organise history (媒体 → 整理历史): click the recognised name in the Title (条目) column.
- Download → Torrent search (下载 → 种子搜索), when searching by title: Title details › (条目详情 ›) in the side panel, or the name on the title card on a phone (see [torrent search](torrent-search.md#search-by-title)).
- Media → Calendar (媒体 → 日历): click an entry.
- Recommendations (推荐) on a title details page: click a poster to open that title.

The breadcrumb at the top says which page you came from, for example 媒体 / 追剧与订阅 / 长夜将尽; click its middle part to go back there. When TMDB has no such title, or TMDB could not be read, the page says why and offers to retry.

## Details and actions

The top of the page is a backdrop image with the poster, the name, the original title and year (seasons and episodes for a series, the running time for a film), genres, countries of origin, the episode running time, the status (returning series, ended, released and so on), the TMDB rating and number of votes, the overview, and the directors and leading cast. On the desktop there are links to IMDb and TVDB as well.

| Action                                                      | What it does                                                                                                                                                                                                                                                   |
| ----------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Find releases (搜资源)                                      | Opens torrent search and searches every site for this title, with the season you are looking at for a series                                                                                                                                                   |
| Follow (追剧), Subscribe (订阅)                             | Without a subscription, opens the subscription dialog (a series follows the season you are looking at). With one, it is a menu: open the follow details, pause or resume, and Follow season N (追第 N 季) when the season you are looking at is not subscribed |
| Play in Jellyfin (在 Jellyfin 播放)                         | Only when a media server has this title (the label names the server): opens the title on the server in a new tab. With several servers, the first one that has it                                                                                              |
| Rescrape (重新刮削)                                         | Only when the title is in the library: rewrites its NFO files and images with the library's current scraping settings, the same as Rescrape in [organise history](media-library.md#organise-history)                                                           |
| Delete (删除)                                               | Only when the title has library files, related torrents or a subscription: opens the [delete dialog](#deleting-a-title) for the whole film, or for the season you are looking at                                                                               |
| More → Open on TMDB (更多 → 在 TMDB 打开)                   | Opens the TMDB page in a new tab                                                                                                                                                                                                                               |
| More → Copy TMDB ID (更多 → 复制 TMDB 编号)                 | Copies the TMDB ID to the clipboard                                                                                                                                                                                                                            |
| More → Organise into this title (更多 → 手动整理到这个条目) | Opens [organising by hand](media-library.md#organising-a-file-or-folder-by-hand) with this title already filled in; enter the source path and organise                                                                                                         |

On a phone, Rescrape and Delete are in the More menu too, and More sits at the top right of the backdrop; Play in Jellyfin is at the bottom of the On your NAS card.

## On your NAS

On your NAS (你的 NAS 上) has four lines about this title on your side (the fourth needs a [media server](media-library.md#media-servers) with a synced [index](media-library.md#index)):

- **Library**: for a series, how many episodes of each season are in the library, for example `库里 S01 12/12 · S02 6/12`, with the libraries, the number of files and their size below; for a film, the version (resolution and release group) of the file in the library. When nothing is in the library it says 库里没有 (not in the library). When the daily check finds library files missing, the line below names them in the warning colour (`S02E05 的文件不见了`, the file of S02E05 is missing; several read `S02E05 等 2 个文件不见了`), followed by Handle (处理) and Ignore (忽略); see [missing library files](media-library.md#missing-library-files).
- **Seeding**: how many torrents are seeding. When some have an H&R requirement that is not met yet, it says how many and how long until the closest one is met (for example `1 个 H&R 还差 2 天`, one H&R torrent needs 2 more days); otherwise it shows the lowest ratio.
- **Subscription**: the subscription's status and season (`追剧中 · 第 2 季`, following season 2; `已暂停`, paused; `待确认`, waiting for confirmation; `已完成`, done), with the locked version below, or a note that the version lock is off; without a subscription it says 没有追剧 (not followed) or 没有订阅 (not subscribed).
- **Media server**: whether the server has this title, and for a series how many episodes (`Jellyfin 已入库 17 集`, 17 episodes in Jellyfin), with the time of the last index sync below. When episodes pt-tools organised are gone from the server, it names them in the warning colour (`S02E03 在服务器里被删了`, removed from the server; several read `S02E03 等 3 集`), followed by Handle (处理) and Ignore (忽略): Handle opens the [delete dialog](#deleting-a-title) for those episodes only; once ignored they are not mentioned again for as long as they stay away (see [removed from the server](media-library.md#removed-from-the-server)). With several servers, a server with such issues comes first, then one that has the title.

On the desktop this block sits to the right of the backdrop, and moves under it when the window is narrow; on a phone it is a white card under the backdrop.

## Seasons and episodes

Only series have this block.

- **Season tabs**: each season shows how many of its episodes are in the library and how many it has (`6/12`); with [counting server episodes](media-library.md#counting-server-episodes) on, episodes the server has count too. Specials come last. The page opens the season given in the link; without one, the subscribed season; without a subscription, the latest season that has episodes. Switching seasons updates the address, so reloading or sharing the link keeps the season.
- **Legend**: how many episodes of the season are in each state.
- **Episode cards**: the still, the episode number and name, the air date (today's episodes say 今天, today; a show followed by air time shows the air time too; the last episode says 季终, season finale; 播出日期未定 means TMDB has no date yet), the state and a note:

| State                          | Note                                                                                                                                                                                                                                                                                                                                                                                                                       |
| ------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `已入库` (in library)          | The version and size of the file in the library; `库里的文件不见了` (library file missing) when the file is gone from the library, `Jellyfin 里没有了` (no longer in Jellyfin) when the media server lost it, both with Handle (处理) and Ignore (忽略) at the top right; `Jellyfin 里有` (in Jellyfin) for episodes with no organise record that are [counted from the server](media-library.md#counting-server-episodes) |
| `下载中` (downloading)         | Progress and the time left, with a progress bar                                                                                                                                                                                                                                                                                                                                                                            |
| `等版本` (waiting for version) | The locked version; See versions (看版本) opens search by title for this season to compare the versions on the sites                                                                                                                                                                                                                                                                                                       |
| `缺` (missing)                 | Aired, not in the library, with how many days ago it aired; Find releases (搜资源) opens search by title for this season                                                                                                                                                                                                                                                                                                   |
| `还没播` (not aired)           | On the next episode: how many hours after airing the search starts (for a show followed by air time); an episode airing later today counts as not aired until its air time                                                                                                                                                                                                                                                 |
| `不追` (skipped)               | Episodes before the start episode, and skipped ones                                                                                                                                                                                                                                                                                                                                                                        |

Episodes in the library that the media server has show a play button on the still; click it, or the card, to open the episode on the server.

The states are the same as the episode squares in the follow details (see [follow and subscribe](media-subscribe.md#episodes)); for a season that is not subscribed they come from the episodes on TMDB and what is in the library, and no subscription is created. A run of library episodes at the start of the season collapses into one row (`E01–E20 · 20 集都已入库`, all 20 in library); Expand (展开) lists them and Collapse (收起) folds them again. On a phone the row appears once more than 3 episodes qualify; on the desktop, only for seasons with more than 24 episodes.

## Torrents for this season

The list shows the torrents that belong to this title and are still in a downloader: ones pushed with this title attached (by a subscription or by search by title), ones a subscription downloaded, and ones organised into the library. Torrents that are no longer in a downloader are not listed. For a series only the season you are looking at is shown (torrents whose season is unknown are listed too); for a film the block is called 这部片的种子 (torrents for this film).

Each row shows the site; the season and episodes, version and size; the downloader; the state (seeding, downloading, paused, error, or 没读到 when the downloader could not be reached); the ratio and seeding time (progress while downloading); and whether the torrent can be deleted. That is judged by the [auto cleanup protection rules](auto-cleanup.md#protection-rules); both places use the same rules. The heading lists the rules that are on (for example `删除时按这几条保护：下载中、最短保种 24 小时、H&R、保留与保护标签`, protected on deletion by: downloading, minimum keep time 24 hours, H&R, keep and protected tags), and hovering over a protected cell shows the full reason.

| It says                                                  | Meaning                                                                     |
| -------------------------------------------------------- | --------------------------------------------------------------------------- |
| 可以删 (can be deleted)                                  | No protection rule applies                                                  |
| H&R 已满足 (H&R met)                                     | It has an H&R requirement and has seeded long enough                        |
| H&R 还差 2 天 (H&R: 2 days left)                         | It has an H&R requirement and needs to seed longer                          |
| 受保护：H&R 要求不明 (protected: H&R, unknown terms)     | The site marks it H&R without saying how long to seed                       |
| 受保护：下载中 (protected: downloading)                  | Downloading or being checked (when Protect downloading torrents is on)      |
| 受保护：加进来不到 24 小时 (protected: added under 24 h) | The minimum keep time has not passed yet                                    |
| 受保护：标签 pt-tools-keep (protected: tag)              | It has the keep tag or a protected tag                                      |
| 受保护：读不到保护规则 (protected: rules not read)       | The global settings could not be read, so every torrent counts as protected |
| 下载器没读到，不知道能不能删 (downloader not read)       | The downloader could not be read                                            |

The More (更多) menu at the end of each row has:

- **Open in downloader (在下载器里打开)**: opens Download → Downloader web UI (下载 → 下载器 Web UI) with this downloader selected and the list filtered to this torrent.
- **Delete this torrent (删除这个种子)**: opens the [delete dialog](#deleting-a-title) with only this torrent ticked, the library files unticked and the subscription kept.
- **Don't want this torrent (不要这个种子)**: blocks the torrent for this title only. Subscriptions no longer pick it, and the torrent in the downloader is left alone (see [blocking](torrent-search.md#blocking)). Torrents whose ID on the site is unknown cannot be blocked and do not have this item.

On a phone, tap a row and these items open from the bottom of the screen.

## Deleting a title

Delete a film, or a season of a series, that you no longer want from its title details: Delete (删除) in the backdrop (in the More menu on a phone) covers the whole film or the season you are looking at; Delete this torrent on a torrent row and Handle (处理) on an issue open the same dialog. The dialog lists what would be touched, you tick what to delete, and the line at the bottom follows your ticks with what is about to happen (for example `将删除 6 个库文件、1 个种子（连数据），暂停追剧`, delete 6 library files and 1 torrent with its data, pause following). Nothing is deleted until you click Delete (删除).

**Delete library files (删除库里的文件)**: the files pt-tools organised into the library and their organise records, ticked by default. The note says how many files, their size and the organise mode; See files (看文件) lists each one. The rules are those of deleting a record together with its files in [organise history](media-library.md#organise-history):

- Only files confirmed to be the ones that were organised are deleted; files that were replaced or changed stay and only the record goes. When a file is already gone from the library, only the record is deleted.
- Moved files are the only copy and are not deleted here; the dialog says so.
- [Imported existing files](media-library.md#importing-an-existing-library) were in the library before the import, and deleting them leaves no copy: they are deleted only when you also tick Also delete files that were there before the import (也删导入前就有的文件).
- Deleting hard links, symbolic links and copies does not touch the data of the torrents.

**Torrents**: torrents that belong to this season (or film) and are still in a downloader, one row per torrent per downloader. The same torrent on two downloaders is two rows, judged separately.

- Torrents that can be deleted are ticked; on the right choose Delete with data (连数据一起删, the default) or Torrent only (只删种子). When another torrent on the same downloader that is not being deleted uses the same data, only the torrent is deleted and the data stays; the row names that torrent, and the choice cannot be changed.
- Protected torrents (see the protection rules above) show the reason and are not ticked; to delete one, tick Delete anyway (仍然删除) on its right. Each protected torrent needs its own tick.
- A torrent that also holds episodes you did not select (for example a season pack when you delete a few episodes) is not ticked, and the row says which episodes.

**Subscription (订阅)**: shown when this season (or film) has a subscription.

| Option                         | Description                                                                                                                                                                             |
| ------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Pause following (暂停追剧)     | The default. Without pausing, the ledger would count the deleted episodes as missing and download them again; a finished film subscription never searches again, so it is left as it is |
| Delete subscription (删除订阅) | Stop looking for this season                                                                                                                                                            |
| Keep following (保留，继续追)  | Delete only the files and torrents you selected; the subscription carries on                                                                                                            |

For subscriptions that are not followed (films, and series without following) the options read 暂停订阅 (pause subscription) and 保留订阅 (keep subscription). When there is nothing to pause, because the subscription is already paused, still waiting for confirmation (created from Douban) or a finished film subscription, the pause option says why (for example `不用暂停：已经暂停了`, no need to pause: already paused) and the dialog keeps the subscription by default.

After you click Delete, pt-tools handles the subscription first, then deletes the torrents downloader by downloader, then the library files. Every step gets its own result: a tick when it worked, the reason when it did not; a step that failed does not undo the steps already done. Some details:

- Torrent deletion does not trust what the dialog showed: pt-tools reads the downloader again and works out protection and shared data again. A torrent that became protected after the dialog opened is still skipped, and the result says so; a torrent that is already gone from the downloader counts as done.
- When a torrent is deleted with its data and its library files are not ticked, the hard links organised from it are what you are keeping: Delete library links when torrents are deleted (删种时一并删除入库链接) leaves them alone (see [interaction with auto cleanup](media-library.md#interaction-with-auto-cleanup)).
- Once the library files and organise records are deleted, the `library file missing` and `removed from the server` issues of those episodes are marked handled. Opened from Handle, the dialog selects only the episodes with issues, and the top of the dialog names the issues being handled.
- Deletion takes turns with auto cleanup and brush deletion, so they never delete the same torrent at once; every deletion is written to the log (`[媒体库] 网页上删除 …`).
- Deleting is only on the web; the mobile app, MCP and the bot do not have it.

## Cast and recommendations

**Cast (演职员)**: directors first, then the cast, with photos, names and characters. On the desktop, All N (全部 N 位) expands the list onto several rows and Collapse (收起) folds it again; on a phone the row scrolls sideways.

**Recommendations (推荐)**: up to 12 films or series that TMDB recommends for this title, tagged `已入库` (in library) or `已订阅` (subscribed); click a poster to open that title.

When the cast, the season list, the recommendations or a downloader could not be read, the rest of the page is shown as usual and a note at the top says which parts are missing; Retry (重试) reads them again. The cast and recommendations blocks are hidden when they are empty.

## Subscription calendar

Media → Calendar (媒体 → 日历) lists, day by day, when each episode of a subscribed series airs and when a subscribed film is released. Paused subscriptions are not listed, and neither are subscriptions created from Douban that are waiting for confirmation. Episodes are placed by their air date on TMDB, so an episode without one cannot be placed; films use their release date on TMDB.

Each entry shows the name, the season and episode (`S02E03`, or 上映 for a film's release) and the state, the same as the episode cards on title details (in the month view, the colour of the dot). For a show followed by air time, the week view and the phone list show the air time, and the month view shows it when you hover over the entry; an episode airing later today is `还没播` (not aired) until then.

- **Month view**: up to 3 entries per day; the rest show as 还有 N 项 (N more), which opens that week in the week view.
- **Week view**: every entry of each day, with posters and episode names.
- **Phone**: only the days that have entries, with today marked 今天.

Switch between month and week at the top right, use the arrows to move to the previous or next period, and Today (今天) to come back to today; the address keeps the view and the date. Click an entry to open its title details (for a series, at that season). When TMDB could not be read for a subscription, a note at the top names it, and the other entries are listed as usual.

## Updates airing today

Turn on Today's updates (今日更新) in Subscription settings (订阅设置) at the bottom of Media → Follow and subscribe, and every day pt-tools pushes the episodes airing today and the films released today to your notification channels:

| Option                     | Description                                                                                          |
| -------------------------- | ---------------------------------------------------------------------------------------------------- |
| Today's updates (今日更新) | The switch, off by default                                                                           |
| Push at (推送时刻)         | 09:00 by default, in the server's time zone, in 15-minute steps                                      |
| Push to (推送到)           | The notification channels to push to; when none is chosen, the ones under Notifications (通知) above |

At that time it sends a message titled 📺 今日更新 2026-11-03 that lists every episode airing today (the subscription name and year, the season, the episode number and name, and the air time for a show followed by air time) and every film released today, each with its state (in library, downloading, not aired and so on).

- Nothing is sent when no episode of your subscriptions airs and no film is released today.
- Each channel gets at most one message a day, also across restarts; when pt-tools starts after the push time, it still sends that day's message. When TMDB cannot be read, it tries again 10 minutes later.
- Nothing is pushed while the main Subscriptions (订阅) switch is off.
- Messages wait for the channel's quiet hours to end and are retried automatically on failure; the text can be changed with [message templates](notify-channels.md#message-templates), where the event is `media.calendar.today`.

## Bot and daily report

- Send `/today` (or `/今日`) to the bot for the episodes of your subscriptions airing today and the films released today, and whether each is in the library. It lists up to 30 entries; the rest are in the calendar on the web. Every linked account can use it, see [ChatOps quick start](chatops-quickstart.md).
- The [daily report](user-stats.md#daily-report) gets a line `📺 昨日追剧：播出 N 集，入库 M 集` (yesterday: N episodes aired, M in library; 今日 instead of 昨日 when the report covers today): the episodes of your subscriptions that aired on the day the report covers, and how many of them are in the library now. The line is left out while the main Subscriptions switch is off, or when nothing aired that day.
