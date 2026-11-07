# Media library organising <Badge type="tip" text="1.0 preview" />

pt-tools can put finished movies and TV shows into your media library: it recognises the title, places the files in the library folder following the library's naming template (hard links by default, so no extra space is used and the torrent keeps seeding), writes NFO files and artwork, asks Emby, Jellyfin or Plex to scan, and sends a notification.

Settings live under Media → Media library (媒体 → 媒体库) and results under Media → Organise history (媒体 → 整理历史). Recognition uses TMDB: released builds have a TMDB API key built in, so there is nothing to fill in; builds from source need a key under [media recognition](media-recognize.md#tmdb-api-key) first.

## Preparing the folders

pt-tools must be able to read the download folder and write the media library. A direct install usually needs no changes; in Docker, mount both folders into the pt-tools container.

Hard links only work when the download folder and the library are on the same file system. In Docker the safest way is to mount their common parent as one volume:

```yaml
services:
  pt-tools:
    volumes:
      - /mnt/data:/data # downloads in /mnt/data/downloads, library in /mnt/data/media
```

If you mount them as two volumes, `/downloads` and `/media`, hard links fail inside the container even when both are on the same disk on the host.

When the downloader and pt-tools see different paths (for example when they run in different containers), add a mapping under Path mappings (路径映射): which folder in pt-tools the downloader's `/downloads` is. The longest matching prefix wins, compared by path segment, so `/data` does not match `/database`. When the paths are the same, no mapping is needed.

## Libraries

Each library takes one kind of title. Create one for movies and one for TV shows; to keep animation apart, add a library with Animation only (只收动画) checked (titles with TMDB's "Animation" genre go there first, and to the regular library when there is no such library). When several libraries share a type, the first one is used.

| Field             | Description                                                                                                                                                                  |
| ----------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Type              | Movie or TV; with Animation only checked it takes animation only                                                                                                             |
| Library folder    | The path as pt-tools sees it (absolute)                                                                                                                                      |
| Mode              | Hard link (default), copy, symbolic link or move, see below                                                                                                                  |
| Template          | Leave empty for the default, see [naming templates](#naming-templates)                                                                                                       |
| Scraping          | Write NFO files and images; what to write, which image language and which data sources to use, see [scraping](#scraping); turn it off to let the media server do it          |
| Overwrite         | Rewrite NFO files and images on every run; when off, existing files are left alone                                                                                           |
| Same-name files   | Whether to replace an old version pt-tools put into the library, see [files with the same name](#files-with-the-same-name)                                                   |
| Categories        | With Organise by category checked, files go into `<library folder>/<category>/…`, see [secondary categories](#secondary-categories)                                          |
| Download settings | Downloader, save folder, category and tags filled in when you create a subscription, see [download settings for new subscriptions](#download-settings-for-new-subscriptions) |

| Mode          | Description                                                                                                                              |
| ------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| Hard link     | No extra space and the torrent keeps seeding; the download folder and the library must be on the same file system                        |
| Copy          | Works across partitions; written to a temporary file first, then renamed. Uses space for a second copy, except with copy-on-write below  |
| Symbolic link | No extra space; the media server must reach the download folder by the same path, and the link breaks when the torrent's data is deleted |
| Move          | The files move into the library and the torrent can no longer seed; only paused torrents are organised                                   |

When a move cannot delete the source file (for example because the download folder is read-only), the copy already placed in the library is still recorded as organised, with the mode it actually has, copy (across file systems) or hard link, and a note that the source file is still in the download folder; delete it yourself if needed.

Copying tries copy-on-write first (reflink, on Linux when the download folder and the library are on the same file system that supports it, such as Btrfs or XFS): the library copy shares the data on disk with the source and takes no extra space until one side changes. The mode is still copy, and the organise record says 复制用了写时复制. When the file system doesn't support it, across file systems, or on other operating systems, the file is copied byte by byte as before.

Click Check (检查) while editing a library: it confirms the folder exists and is writable, and for hard links it creates a file in the pt-tools path of each path mapping and tries to hard-link it into the library, which confirms they are on the same file system. The test files are removed afterwards.

### Download settings for new subscriptions

A library can set a downloader, a save folder, a category and tags; all of them can stay empty. When you create a subscription in Follow and subscribe (追剧与订阅) and pick this library, or Choose by type (按类型自动选) picks it, these fields are filled into the subscription form, with a note under each saying which library it came from. You can change them before saving.

- They are filled in only when you create a subscription, by the web dialog and by the subscription editor of the [mobile app](mobile-app.md#following-shows-brush-tasks-and-statistics-sharing). Existing subscriptions keep their settings, and nothing is added when a subscription is saved or pushes a torrent. Subscriptions created by the bot, the Douban wish list, Servarr or the AI assistant don't get them.
- A field you changed yourself (typed, cleared or picked something else) stays when you switch libraries; Use library "X" (换成媒体库「X」的) next to it restores the library's value. Fields that came from a library follow the library when you switch, and a field the new library leaves empty is cleared.
- Editing an existing subscription fills nothing in: after you switch libraries, a line Apply "X"'s download settings (套用「X」的下载设置) appears, and the fields change only when you click it.
- A downloader that is disabled or deleted is not filled in; the push uses the default downloader from the settings, and the form says why. When you edit the library after its downloader was deleted, the downloader shows Not set (不指定) and is cleared when you save.
- The save folder is the folder as the downloader sees it, not the library folder. When qBittorrent's default torrent management mode is Automatic, torrents go to their category's folder (the default folder when there is no category) and the save folder has no effect; with [per-title download folders](media-subscribe.md#per-title-download-folders) turned on in the subscription settings, pt-tools makes qBittorrent use the save folder.
- With automatic organising or periodic scanning on, editing a library warns when these fields fall outside the [organising scope](#when-organising-runs): torrents pushed this way would not be organised when they finish.

## Naming templates

Templates use Go's text/template and produce the path inside the library, without the extension (it is added when organising). The defaults work with Emby, Jellyfin and Plex.

Default movie template:

```text
{{.Title}}{{if .Year}} ({{.Year}}){{end}}/{{.Title}}{{if .Year}} ({{.Year}}){{end}}{{if .Quality}} - {{.Quality}}{{end}}
```

Example: `Oppenheimer (2023)/Oppenheimer (2023) - 2160p BluRay HDR10 H.265.mkv`.

Default TV template:

```text
{{.Title}}{{if .Year}} ({{.Year}}){{end}}/Season {{.Season}}/{{.Title}} - {{.SeasonEpisode}}{{if .EpisodeTitle}} - {{.EpisodeTitle}}{{end}}
```

Example: `The Last of Us (2023)/Season 1/The Last of Us - S01E02 - Infected.mkv`.

The presets above the editor add the TMDB ID to the folder name the way each server expects: `[tmdbid=ID]` for Emby, `[tmdbid-ID]` for Jellyfin and `{tmdb-ID}` for Plex. While you edit, the page shows the template applied to an example, or why it does not work.

| Variable                                                                        | Description                                                                  |
| ------------------------------------------------------------------------------- | ---------------------------------------------------------------------------- |
| `.Title`, `.OriginalTitle`                                                      | Title (TMDB, in the language set under media recognition) and original title |
| `.Year`                                                                         | Year; the first air year for TV shows                                        |
| `.TMDBID`, `.IMDbID`                                                            | TMDB and IMDb IDs                                                            |
| `.Season`, `.Episode`                                                           | Season and episode                                                           |
| `.SeasonEpisode`                                                                | `S01E02`, or `S01E02-E03` when a file holds several episodes                 |
| `.EpisodeTitle`                                                                 | The episode's title (when TMDB has one)                                      |
| `.Quality`                                                                      | Resolution, source, HDR and codec together                                   |
| `.Resolution`, `.Source`, `.VideoCodec`, `.HDR`, `.Audio`, `.Group`, `.Edition` | Resolution, source, video codec, HDR, audio, release group, edition          |

`pad` adds leading zeros, so this template writes `The Last of Us/Season 01/The Last of Us S01E02.mkv`:

```text
{{.Title}}/Season {{pad .Season 2}}/{{.Title}} {{.SeasonEpisode}}
```

Characters not allowed in paths (`\ / * ? " < > |`) become spaces and colons become `-`. Each level is trimmed of spaces and dots, empty levels are dropped and each level is at most 200 bytes. The result never points outside the library folder.

## Secondary categories

With Organise by category (按二级分类放进子目录) checked on a library, each title is categorised first and then placed under `<library folder>/<category>/`, for example `Movies/华语电影/Let the Bullets Fly (2010)/…` or `TV/欧美剧/The Last of Us (2023)/Season 1/…`. The rules live under Secondary categories (二级分类) on the media library page, one list for movies and one for TV:

- The first matching rule from the top wins; when none matches, the title goes straight into the library folder.
- A rule can match on genre, original language and production country or region: all conditions must hold, and a condition with several values matches any of them. A rule with no conditions takes everything the rules above it didn't.
- A rule can also match on running time and title keywords (useful for short dramas): the running time (minutes, per episode for TV) is at most a limit, or the torrent name or the site's title contains one of the keywords. The two count as one condition with "or" between them, and the other conditions must still hold; a limit of 0 ignores the running time, and when TMDB has no running time only the keywords count.
- Original languages are ISO 639-1 codes (`zh`, `ja` and so on; TMDB records Cantonese as `cn`), countries and regions are ISO 3166-1 codes (`CN`, `US` and so on). Codes that aren't in the list can be typed in.
- Fill in common categories (填入常用分类) uses roughly the same categories as MoviePilot's defaults: animated, Chinese-language and foreign movies; for TV, Chinese animation, Japanese anime, documentaries, kids, variety, short dramas (短剧), Chinese, Western and Japanese/Korean series, and uncategorised. Short dramas are shows with episodes of at most 15 minutes or 短剧 in the title; the rule comes after the genre rules, so animation and kids' shows with short episodes go into their own categories first.

The category name is the folder name, so it can't contain `/ \ : * ? " < > |`. Changing the rules or the checkbox only affects files organised afterwards; files already in the library stay where they are. For TV, the NFO files, poster and logo go into the show folder inside the category folder. The organise preview shows which category a title goes into, which rule matched and which conditions it matched (for example 对上第 6 条「短剧」：片长 8 分钟（不超过 15 分钟）, rule 6 "短剧": 8 minutes, at most 15); when no rule matches, it says the title stays in the library folder. Categorising needs TMDB's details (a movie's production countries and running time are only there); when they can't be fetched, the title isn't organised this time and is retried later.

## Files with the same name

When a movie or an episode gets another release (say 1080p first and 2160p later) and both come out with the same file name, the old version already occupies that spot in the library. Same-name files (同名文件) on the library decides what happens:

| Option              | What it does                                                              |
| ------------------- | ------------------------------------------------------------------------- |
| Keep (default)      | Keep the old version; the new one is recorded as skipped, with the reason |
| Replace when larger | Replace only when the new file is larger than the old version             |

Only files pt-tools put into this library for the same movie (or the same episode) are replaced, confirmed by the organise record and file ID. Files you put there yourself, files that were changed, [imported existing files](#importing-an-existing-library) and moved files (the only copy) are never replaced, and nothing is replaced when the library organises by moving. The new file is placed under a temporary name next to the target first, the old file is moved to a backup name and checked to be the one the old record placed, then the new file goes into place, and the old file is deleted only after the records are saved. If any step fails the old file goes back where it was, and if pt-tools stops halfway the unfinished replacement is rolled back on the next start. The old version's subtitles and same-name NFO and thumbnail are swapped for the new ones, folder images stay, and the old record is marked removed with what replaced it. Files in the download folder are never touched. When the library's file system supports neither hard links nor no-overwrite renames (some network storage), nothing is replaced and the reason is shown.

Only size decides: the smaller old version never swaps the larger one back when it's organised again (automatically or by hand); it's recorded as skipped. To go back, delete the new version's record together with its library files under organise history, then organise the old one. When the naming template includes the quality (the default movie template does), different releases get different file names, never collide, and both stay in the library; to keep only one, drop the quality from the template or use subscription upgrades.

## Which files are organised

- Video files: mkv, mp4, m4v, avi, ts, m2ts, mts, mov, wmv, flv, rmvb, rm, webm, mpg, mpeg.
- Skipped: files in Sample, Trailers, Featurettes, Extras, SPs and similar folders, file names ending in sample or trailer, NCOP and NCED openings and endings, and files smaller than Minimum video size (视频最小体积, 50 MB by default). The Specials folder holds season 0 episodes and is organised as usual.
- ISO images: a movie's ISO is organised as one video file (named by the video file rules, with the `.iso` extension); whether it plays depends on the media server and the client. ISO images in a TV show are not organised, and each one gets a skipped record.
- Discs (BDMV and VIDEO_TS folders): a movie's disc goes into the movie folder whole, see [discs](#discs).
- Subtitles (srt, ass, ssa, sub, idx, sup, vtt) go with their video (except those in extras or sample folders) and get a language suffix from the file name: `zh-CN` simplified Chinese, `zh-TW` traditional Chinese, `zh` Chinese, `en` English, `ja` Japanese, `ko` Korean. When a torrent has several videos, subtitles are matched by file name prefix or by season and episode.
- Season and episode numbers come from each file name, with [recognition words](media-recognize.md#recognition-words) applied (episode offsets included); single-file torrents use the torrent name.

Organising picks the item in this order, and the preview and organise history say which one was used:

1. The TMDB ID filled in for a manual organise (手动指定);
2. The item these files were organised as before (沿用之前整理用的条目);
3. For torrents a subscription pushed or counted towards it: the subscription's item (按订阅的条目), without recognising;
4. The item pt-tools recorded when it pushed the torrent (按推送时说明的条目): every torrent a subscription pushes carries one, so it still applies after the subscription is deleted;
5. Otherwise recognition: the torrent name (and the site title as a subtitle when it differs), looking up the site's IMDb ID first when there is one.

When the item from a subscription or a push is no longer on TMDB (merged or deleted), recognition is used instead and the organise record says so. Seasons and episodes are still read from each file name, not taken from the subscription. When the followed series uses an [episode group](media-subscribe.md#episode-groups), the season and episode in each file name of the series are first converted from the group to TMDB's default seasons, then named and scraped; an episode the group does not list keeps the season and episode from the file name, and the organise record says so. When the group cannot be read, nothing is organised this time and it is retried later.

## Discs

A movie's Blu-ray disc (BDMV folder) or DVD (VIDEO_TS folder) goes into the movie folder whole:

- The disc folders (BDMV, CERTIFICATE, AACS, BDSVM, VIDEO_TS, AUDIO_TS) in the disc root (the folder above BDMV or VIDEO_TS) go into the movie folder the naming template gives, keeping their relative paths, for example `Oppenheimer (2023)/BDMV/STREAM/00001.m2ts`. Other files in the disc root (nfo, images, samples) stay out of the library and are listed under 没选上的文件 in the preview.
- The organise mode works as for video files: hard links, copies or symbolic links are made file by file; a move still only organises paused torrents and removes the disc folders it emptied in the download folder. When it fails half way, the files placed so far are taken back out (moved files are moved back) and the record says failed.
- Scraping writes `movie.nfo`, `poster.jpg`, `fanart.jpg` and the rest into the movie folder (Jellyfin, Emby and Kodi read `movie.nfo` there), and the media servers are asked to scan the movie folder.
- A disc has one organise record: the source is the disc root, the target is the movie folder, the size is the total of the files placed, and organise history shows 原盘 N 个文件. Deleting the record with its library files, and subscription upgrades, check each file's ID before deleting it and remove the empty folders last.
- When the movie folder already exists with other files in it (for example another version of the movie), the disc isn't placed there and the record says skipped; discs don't take part in same-name replacement. When a record was deleted but its library files kept, organising again recognises them (when each file is a hard link to, or a symbolic link pointing at, its source).
- Skipped and not organised: TV show discs, several discs in one torrent, discs without a video stream, and discs whose BDMV or VIDEO_TS sits directly in the download folder or monitored folder (the disc has no folder of its own; for a manual organise, pick the folder that contains BDMV). When the movie naming template has no folder (movies placed directly in the library folder), the record says failed: switch to a template that puts each movie in its own folder (such as the default one) and organise again.
- Discs and ISO images skipped before the upgrade aren't organised again automatically (they already have records); click Retry (重试) in organise history.

## When organising runs

| Trigger       | Description                                                                                                                                                                                                        |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Automatic     | Torrents pushed by pt-tools are organised when they finish downloading. Only torrents finished after you turn it on; missed ones are picked up every 10 minutes                                                    |
| Periodic scan | Finished torrents in the downloaders are scanned at the interval, including ones pt-tools did not push. Up to 50 per round; torrents that already have records are skipped                                         |
| Monitoring    | Videos dropped into monitored folders that are switched on (not torrents) are organised when the folder changes (real-time monitoring) and at the scan interval, see [directory monitoring](#directory-monitoring) |
| Manual        | The organise icon in the Status (状态) column of the task list (整理入库 on mobile cards); in the downloader web UI, select one finished torrent and click 整理入库, or use the right-click menu                   |
| By path       | Manual organise (手动整理) on the organise history and media library pages: enter a file or folder, preview, then organise, see [organising a file or folder by hand](#organising-a-file-or-folder-by-hand)        |

The organise settings show when the periodic scan and directory monitoring last ran and roughly when they run next (these times start over when pt-tools restarts, and the first round after start runs within a minute).

Automatic organising and periodic scans only take torrents in the scope: downloaders, categories, tags and save paths (as the downloader sees them). An empty item means no limit; a filled one must match one of its values. When subscriptions use [per-title download folders](media-subscribe.md#per-title-download-folders), the save paths you fill in must include those folders. Their automatic retries follow the current settings too: once the switch is turned off or the torrent is out of scope, they are not retried.

Manual organising shows a preview first: the recognized title, the chosen library and where each file goes. When the match is wrong, enter a TMDB ID (choose movie or TV) or pick another library of the same type, click Preview again (重新预览), then organise. The organise mode follows the library by default; under Organise mode (整理方式) in the dialog you can pick hard link, copy, symbolic link or move for this run only (the library setting stays as it is), and the preview updates right away. Moving still only organises paused torrents. A torrent organised before keeps the title it was organised with (the preview says 沿用之前整理用的条目); to change it, enter a TMDB ID or delete its records first. Copying large files can take a while; the dialog waits 20 seconds, the work continues in the background and the result appears in the history.

Each file is organised once: repeated completion events or several triggers at the same time never organise it twice. When the library already has this file (the same inode, or a symbolic link to the source), it counts as done. When another file is at the target, it is not overwritten and the record says skipped (copied and moved files cannot be told apart from someone else's, so an existing target is always skipped); when it's an older version of the same title that pt-tools placed, the library's same-name setting decides, see [files with the same name](#files-with-the-same-name). Subtitles that fail are noted in the record and added the next time the torrent is organised (a retry or another manual run).

## Directory monitoring

Videos dropped into a monitored folder (not torrents in a downloader, for example files copied by hand or downloaded by another tool) can be organised into the libraries too. Add folders (paths as pt-tools sees them) under Directory monitoring (目录监控) on the media library page; the scan interval is in the organise settings, 10 minutes by default. A folder's path can't be changed after it's added: to monitor another folder, delete it and add the new one.

- Real-time monitoring (实时监控, a switch in the organise settings, on by default): when something changes in a folder, the changed item is organised 2 minutes after its last change instead of waiting for the next scan; the scans at the interval still run and catch anything it misses. It's available on Linux and Windows only; on macOS the folders are scanned at the interval. Folders that can't be watched are scanned at the interval only, and the directory monitoring list says 实时监控不可用 with the reason: the inotify watch limit was reached (`fs.inotify.max_user_watches` on the host), the folder is on NFS, SMB or CephFS (changes written from other machines aren't reported), or a monitored folder has more than 4096 subfolders. Those folders are tried again every minute.

- Each item in the folder (one file, or one subfolder) is treated like a torrent: it's recognised by its name, its videos go into the library with the library's mode (hard link, copy, symbolic link or move), and the organise record shows 目录监控 as the downloader.
- Items changed within the last 2 minutes wait (they may still be copying). Items whose videos all have organise records aren't scanned again; when a subfolder gets more episodes later, the new ones are organised.
- Hidden files and folders, NAS system folders (`@eaDir`, `#recycle` and the like) and unfinished downloads (`.part`, `.!qB` and the like) are ignored. An item more than 6 folders deep or with more than 5000 files isn't organised at all, and organise history says why.
- Each location has one organise record, which stores the source file's ID (the same file with the same size and modification time counts as unchanged): when another file appears where one was organised before (for example a file with the same name dropped in after the old one was moved), it isn't organised while that record still tracks library files, and the record says so; delete that record and the next scan organises the new file. When the record tracks no library files (for example a failed recognition), the new file is organised.
- A monitored folder can't be a library folder, be inside one or contain one. Don't monitor a downloader's download folder either: its torrents are handled by automatic organising and the periodic scan.
- When the library moves files, pt-tools first checks that the item isn't a torrent in a downloader (moving it away would stop the torrent from seeding). If it is, the item isn't organised and the record says why; when a downloader can't be reached to check, it's tried again later. Torrent paths are converted with the path mappings before comparing; torrents whose paths pt-tools can't see (no path mapping, or a downloader on another machine) are matched by name, so a torrent with the same name as the item counts as that item.
- There's no site title or IMDb ID, only the name. When the recognition is wrong, add a correction or recognition word under media recognition, then retry from organise history.
- Delete links when torrents are removed doesn't touch files organised from monitored folders: deleting the source file in the monitored folder leaves the library file in place.

## Organising a file or folder by hand

Videos that aren't in a downloader pt-tools manages (for example ones downloaded by another client or copied from another machine) can be organised into a library by entering their path, without adding a monitored folder first. Click Manual organise (手动整理) at the top of the organise history (整理历史) or media library (媒体库) page:

1. **Enter the source path**: a file or folder as pt-tools sees it, for example the folder of one movie or one season, or a single video file. In Docker, mount the folder that holds it into the container first and enter the path inside the container. You can also click Browse (浏览): it lists the folders you can browse (libraries, monitored folders, the local side of path mappings and the download folders set on downloaders); open a folder and pick a file, or use Pick this folder (选这个文件夹). Only these folders can be browsed, and symbolic links pointing outside them aren't listed; a folder lists at most 2,000 entries, so type the path for larger ones.
2. **Preview**: click Preview (预览) or press Enter. The dialog shows the recognised title, the library it goes into, the organise mode, whether NFO files and images are written, and where each file goes in the library. When the match is wrong, choose the type (movie or TV), enter a TMDB ID and click Preview again (重新预览); you can also pick another library of the same type. Leaving the TMDB ID empty recognises the title by name.
3. **Organise**: the files go into the library folder with the organise mode (the library's by default; Organise mode (整理方式) can pick another one for this run), NFO files and images are written when the library scrapes, the media servers are asked to scan and a notification is sent, the same as for torrents. After changing the source path, preview again before organising.

**Where the links go**: into the chosen library's folder (into its category folder when Organise by category is checked), named by the library's naming template. To put them somewhere else, add a library for that folder under Media library first. Hard links need the source and the library on the same file system, see [preparing the folders](#preparing-the-folders).

Rules:

- The source path can't be a library folder, be inside one or contain one, and the path itself can't be a symbolic link: enter the path it points to.
- A path changed within the last 2 minutes isn't organised (it may still be copying), and neither is a folder more than 6 levels deep or with more than 5000 files: choose the folder of one movie or one season instead. Neither case writes an organise record.
- A single video file is organised on its own, without the subtitles next to it; to include them, choose the folder that holds them.
- When the source path is exactly a torrent in a downloader, it's organised as that torrent instead (the preview says so): the records belong to the torrent, so deleting library links on torrent removal and the periodic scan recognise it. When it only partly overlaps a torrent (inside one, or containing one), it isn't organised.
- When files are moved (the library moves files, or move was picked for this run), the source files move into the library and don't stay where they were: a path with the same name as a torrent in a downloader isn't organised, and neither is one that can't be checked because a downloader can't be reached; those are tried again later. Don't move files that are still seeding.
- You can also enter an item in a monitored folder to organise it now instead of waiting for the next scan.

Failures and retries:

- Each video gets one organise record, with the trigger shown as 手动（指定路径） (manual, by path).
- Temporary problems such as TMDB being unreachable are retried after 10 minutes, 30 minutes, 1 hour, 3 hours and 12 hours, up to 5 times; when the record has a title, both the automatic retries and Retry (重试) keep that title and the library used at the time. A run with an organise mode picked keeps that mode too; one that followed the library uses the library's current mode.
- Failed or skipped records of this kind also have Manual organise (手动整理): it opens the dialog with the path entered and the organise mode picked back then, so you can change the title, library or mode and organise again.
- Records whose source path is gone (deleted or moved away) are not retried automatically.
- The mobile app and MCP can retry these records but can't organise a new path.

When a torrent you downloaded by hand is in a downloader pt-tools manages, you can also select it in the downloader web UI and click 整理入库. To have such torrents organised automatically from now on, give them a category or tag in the downloader, add it to the scope in the organise settings (整理设置) and turn on Periodic downloader scan (定期扫描下载器).

## Importing an existing library

Movies and shows already in a library folder (ones you organised yourself, or ones another tool put there) can be imported as organise records: the files stay where they are, without moving, renaming or linking. After the import they count as in the library: subscriptions no longer download those movies or the episodes already there, title details, the calendar and the follow details show them as in the library (已入库), and upgrades compare against their versions.

In the library list on the Media library (媒体库) page, click Import existing files (导入已有文件) on the library:

1. **Choose the path**: the library folder by default; click Browse (浏览) to pick a folder inside it and import only what's under that folder. Only the library folder and folders inside it can be chosen.
2. **Scan**: click Start scan (开始扫描). Every item is looked up on TMDB, so the scan runs in the background; the dialog shows how many items are done, and the scan can be cancelled. Only one scan runs at a time, and starting another cancels the previous one. The result is kept for 30 minutes from the last time it was viewed; after that, scan again.
3. **Review**: each item shows the title it was matched to and where the match came from, how many videos it has (seasons and episodes for TV) and whether it can be imported; items that can be imported are selected. When a match is wrong or missing, click Change ID (改编号) or Enter TMDB ID (填 TMDB 编号) (the type follows the library), then select the item. The list can show only importable items, items already recorded or items that can't be imported; View videos (看视频) lists every video and why it can't be imported.
4. **Import**: click Import N items (导入 N 项). The import is queued like organising and runs one job at a time; when it's done the dialog says how many videos were recorded, how many already had organise records and were skipped, and what couldn't be imported and why. With Fill in missing NFO and images (补上缺的 NFO 与图片) ticked (only possible when the library scrapes), each title is scraped once after the import: only NFO files and images that don't exist yet are written, and existing ones stay.

How items are found:

- Movie library: every folder under the library folder is one movie (its videos are picked by the rules in [which files are organised](#which-files-are-organised), without extras and samples), and every video directly in the library folder is one movie.
- TV library: every folder under the library folder is one show. Seasons and episodes come from the file names (recognition words apply); when a file name has no season number, the season folder above it decides (`Season 1`, `S01`, `第一季`, `Specials`), and otherwise it's season 1. Videos without a recognisable episode number are listed and not imported; videos directly in the library folder aren't imported either and need to go into the show's folder.
- Folders named like a secondary category, and folders that only hold several movies or shows (groups of your own), count as category folders and are searched below, at most 4 levels deep.
- One scan lists at most 1,000 items; for more, scan subfolders. An item more than 6 levels deep or with more than 5,000 files, an item whose videos changed within the last 2 minutes (they may still be copying; NFO files and images don't count), discs (BDMV, VIDEO_TS) and ISO images in TV shows aren't imported, with the reason shown.
- Videos that already have organise records (organised by pt-tools, or imported before) are skipped, and an item whose videos all have records shows as already recorded (已有记录).

Titles are matched in this order, and the type has to match the library (a movie library item matched as a show isn't imported; change its ID):

1. Existing NFO files: `movie.nfo` and the `.nfo` next to each video for movies, `tvshow.nfo` for shows. A TMDB ID there is used directly; with only an IMDb ID, the title is recognised by that IMDb ID.
2. An ID in the folder name (the file name for a movie directly in the library folder): `[tmdbid=123]` and `[tmdbid-123]` as Emby and Jellyfin read them, `{tmdb-123}` as Plex reads it; with only an IMDb ID (`[imdbid-tt1234567]`), the title is recognised by that IMDb ID. When the ID isn't this type on TMDB, the item says so and is matched by name.
3. Recognition of the folder name, with recognition words and corrections applied as for torrent names.

Importing reads each item's videos again and writes one organise record per video with the chosen title: the mode shows as existing file (已有文件), the trigger as import (导入已有文件), the downloader column as 导入, and the source file is the library file itself. pt-tools didn't put these files there, so:

- Deleting a record doesn't delete the file by default. With Also delete the organised files in the library (同时删除库里整理出的文件) checked there's a second confirmation (batch delete with files asks too), and the file is deleted only after its file ID confirms it's still the file that was imported.
- Upgrades don't delete them: the new version is organised into the library as usual and the imported record stays as it is; with different file names, both versions stay in the library. When the new version has the same file name as an imported file (the library's existing naming matches the naming template), the new version is skipped when organising and stays in the downloader; to switch to it, delete the imported record together with its file under organise history, then organise the new version again.
- Same-name files (同名文件) never replaces them, whichever option is chosen.
- They don't need retrying: retry and batch retry skip them, with the reason shown.
- Delete library links on torrent removal leaves them alone.
- Rescrape works as usual: NFO files and images that were there before count as written by another tool and are replaced only with Overwrite existing NFO and images (覆盖已有的 NFO 与图片) on.

## Scraping

With scraping on, Scraping content (刮削内容) lets you pick NFO, poster, background, logo, clearart, banner, landscape, disc art (movies only), season posters and episode thumbnails (the last two only exist for TV) one by one. A library you haven't changed writes the NFO, poster, background, season posters and episode thumbnails. Clearart, banner, landscape and disc art only exist on Fanart.tv and are written only after you enter a Fanart.tv key under [media recognition](media-recognize.md#other-scraping-sources).

| Title | Files written                                                                                                                                                                                                                                                                                                                                                 |
| ----- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Movie | `<file name>.nfo` (with its own folder that already has a `movie.nfo`, that file instead of a second one); with its own folder, `poster.jpg`, `fanart.jpg`, `clearlogo.png`, `clearart.png`, `banner.jpg`, `landscape.jpg` and `discart.png`; directly in the library root, the same images with the file name in front, for example `<file name>-poster.jpg` |
| TV    | In the show folder: `tvshow.nfo`, `poster.jpg`, `fanart.jpg`, `clearlogo.png`, `clearart.png`, `banner.jpg`, `landscape.jpg` and `season01-poster.jpg` (`season-specials-poster.jpg` for season 0); `season.nfo` in the season folder; `<file name>.nfo` and `<file name>-thumb.jpg` for each episode                                                         |

The logo is the title artwork on a transparent background, clearart shows characters and the title on a transparent background, and landscape is a wide image with the title; Emby, Jellyfin and Kodi all read these file names. Image language (图片语言) decides which image is used:

| Image language                 | Poster                                                     | Background           | Logo and the rest                                          |
| ------------------------------ | ---------------------------------------------------------- | -------------------- | ---------------------------------------------------------- |
| Recognition language (default) | The source's default (TMDB's for the recognition language) | The source's default | Recognition language, then English, then without text      |
| Chinese, English or Japanese   | That language first, otherwise the source's default        | Without text first   | That language, then the recognition language, then English |
| Without text                   | Without text first                                         | Without text first   | Without text, then the recognition language, then English  |

For the logo or a chosen image language, pt-tools fetches TMDB's image list once more (cached for a week). If that fails, the poster and background fall back to the ones TMDB returns, no logo is written, and the organise result says why. Episode thumbnails always use the ones TMDB returns.

The NFO files use the Kodi format, read by the NFO support of Emby, Jellyfin and Plex. Besides the title, plot, genres and ratings they include the tagline, certification, studios (networks for TV), the collection a movie belongs to, a show's status, directors, writers and cast (with TMDB image links for the photos), and whichever TMDB, IMDb, TVDB, Douban and Bangumi IDs are known. Images are downloaded through the proxy set under media recognition, three at a time. When the template puts episodes directly in the library root, the show and season files are not written.

With Cast names in Chinese (演员名用中文) ticked, the first 15 cast members in the NFO use their Chinese names on TMDB (`zh-CN`, then `zh-SG`, `zh-TW` and `zh-HK`); members without a Chinese name keep the original one, and role names don't change. The names are cached for 30 days. When they can't be fetched, the original names are used and the scrape result says so.

With Fill in missing scraping weekly (每周补一次缺的刮削) ticked, every 7 days the titles in this library whose scrape result is partly scraped or scrape failed are scraped again, at most 50 titles per run, the ones scraped longest ago first. It works like Rescrape, except that files you placed or another tool wrote are never replaced, whether or not Overwrite is on. Titles that don't fit into a full organise queue are filled in next time.

Existing files are left alone by default; turn on Overwrite existing NFO and images (覆盖已有的 NFO 与图片) to rewrite them:

- Rewriting an NFO replaces only the parts pt-tools writes and keeps the rest as it is, such as the play history, date added, tags and media info a media server put there.
- Items locked in Jellyfin or Emby (`<lockdata>true</lockdata>` in the NFO) are not changed, and locked fields (`<lockedfields>`, such as the plot, genres or cast) keep their old values.
- A file whose content would not change is not rewritten, so the media server has no reason to read it again.
- Each file is checked again right before it is written: if it was edited, added or locked while the data was being fetched, it is left alone.

### Data sources

NFO files and images come from TMDB by default. Data sources (数据来源) switches individual parts to another source; when that source has no match, lacks the part or can't be reached, TMDB's is used, and the organise result and the [organise history](#organise-history) say why:

| Part     | Choices                     | Description                                                                                                                                                              |
| -------- | --------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Plot     | TMDB, Douban, Bangumi       | Plots of movies, shows and seasons. Douban and Bangumi list each season separately: `tvshow.nfo` uses season 1 and `season.nfo` uses that season's                       |
| Rating   | TMDB, Douban, Bangumi, IMDb | The main rating. Ratings from the other sources that were fetched are written too, with the main one last: Jellyfin and Emby show the last one                           |
| Episodes | TMDB, Bangumi               | Episode titles and plots (TV only). Episodes missing on TMDB are filled in from Bangumi                                                                                  |
| Images   | TMDB, Fanart.tv             | Which source's poster, background, logo and season posters come first; when the first source has no such image or its image can't be downloaded, the other one's is used |

- Douban: before writing the NFO, pt-tools looks the title up on Douban by its Chinese name, then by its original title. Among titles with a matching name, the same year wins; otherwise a title within one year is used only when it's the only one. When more than one title has the name and it's unclear which, TMDB's data is used. Shows are looked up per season, from season 2 by the year that season first aired. When a torrent pushed by pt-tools carries a Douban ID from the site, a movie is tried by that ID first and used only when its name and year match.
- Douban has no public API; pt-tools uses the two endpoints Douban's own web pages use (search and title details), doesn't log in, and doesn't go through the media recognition proxy. Douban is requested at most once every 2 seconds; search results are cached for a day and titles for a week.
- Bangumi: used only for anime (titles whose TMDB genres include Animation); other titles use TMDB's. A season is looked up by its original title and first air date, and Japanese plots and titles are skipped. pt-tools uses Bangumi's public API, needs no key, and sends at most one request a second.
- IMDb: the rating comes through OMDb and needs an OMDb key under [media recognition](media-recognize.md#other-scraping-sources). Once the key is set, the IMDb, Rotten Tomatoes and Metacritic ratings are written whatever is chosen here (looked up by IMDb ID; titles without an IMDb ID on TMDB are skipped), and OMDb's certification is used when TMDB has none.
- Fanart.tv: needs a Fanart.tv key under media recognition. Movies are looked up by TMDB ID and shows by TVDB ID (shows without a TVDB ID on TMDB use TMDB's images). Without Fanart.tv chosen, the logo comes from TMDB first and from Fanart.tv when TMDB has none.
- Lookups happen only for files that will be written: Douban and Bangumi aren't asked about NFO files that already exist and aren't overwritten. After changing the data sources, click Rescrape (重新刮削) in the organise history to update titles that are already organised.

## Subtitles

Subtitles inside a torrent go with their video (see [which files are organised](#which-files-are-organised)). When a torrent has none, pt-tools can fetch them from the site or hand the video to ChineseSubFinder; both are under Subtitles (字幕) in the library settings.

### Site subtitles

With Download subtitles from the site after organising (整理完到站点上下载字幕) ticked, pt-tools fetches subtitles from the torrent's detail page for each video newly placed in the library when:

- the torrent has no subtitle files;
- pt-tools pushed the torrent (from RSS, search, subscriptions, follow-show or the qB-compatible entrance), so it knows which torrent on which site it is;
- the site runs NexusPHP: a torrent's subtitles are listed in the Subtitles (字幕) row of its detail page as `downloadsubs.php` links. When a site has changed its detail page and has no such links, the site is treated as having no subtitles. Sites on other platforms are not asked.

What is fetched and where it goes:

- When the list has Chinese subtitles (simplified, traditional or plain Chinese), only those are fetched; otherwise all are. The language comes from the language name, the flag image and the title on the list. At most 10 subtitles are downloaded per torrent.
- When a torrent has several videos, subtitles are matched to videos by file name prefix or by season and episode. Subtitles whose title matches no video are not downloaded, and episodes organised earlier get none.
- Zip archives are unpacked and only their subtitle files are kept (names that aren't UTF-8 are read as GBK; only the file name is used, not the folders inside the archive). RAR and 7z archives are not unpacked, and the organise history says so.
- Files are named `<video name>.<language>.<extension>` like subtitles from the torrent, for example `Oppenheimer (2023) - 2160p BluRay HDR H.265.zh-CN.srt`. When there are several of one language, the second gets `.2`, the third `.3`. When a name is taken by another file, the next number is used; nothing is overwritten. A video gets at most 4.
- Requests use the site's rate limit and retries, shared with the site's other requests; the site needs its cookie set in the site settings.

Placed subtitles are recorded in the organise record, are counted in the N subtitles shown in the organise history, and are removed on [deletion](#organise-history) (checked by file ID; replaced files are kept). Problems such as an unreadable list, a failed download or an archive that won't unpack are written in the record's message and don't change the organise result. When a video is organised again, subtitles placed earlier that are still there aren't fetched again.

### ChineseSubFinder

[ChineseSubFinder](https://github.com/ChineseSubFinder/ChineseSubFinder) downloads Chinese subtitles automatically. With these settings filled in for a library, every newly organised video (except discs) is handed to it:

| Setting                                                          | Notes                                                                                                                                         |
| ---------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| ChineseSubFinder address (ChineseSubFinder 的地址)               | For example `http://192.168.1.10:19035/`; leave it empty to not use it (saving then also clears the API key)                                  |
| API key                                                          | Turn on the API key in ChineseSubFinder's Lab (实验室) settings; it is write-only and shows as set (已设置) once saved                        |
| Library folder in ChineseSubFinder (ChineseSubFinder 里的库目录) | This library's path as ChineseSubFinder sees it (often different when the two run in separate containers); leave it empty when it is the same |

Test (测试) calls ChineseSubFinder once to check the address and API key without creating a job; without a new key it uses the saved one.

- pt-tools calls ChineseSubFinder's `api/v1/add-job` endpoint with one job per video at priority 3 (ChineseSubFinder's own scans use 5). The video path is translated with the library folder in ChineseSubFinder.
- Videos are handed over after scraping: shows need scraping on, because ChineseSubFinder reads each episode's NFO.
- Failures (unreachable, wrong key, file not found on its side) are written in the record's message and don't change the organise result. When ChineseSubFinder can't be reached or the key is wrong, the rest of the batch isn't tried.
- ChineseSubFinder downloads the subtitles and places them in the library itself; they aren't recorded by pt-tools and aren't removed when a record is deleted.

## Media servers

After organising, pt-tools asks the media servers to scan the new folders. For servers with a synced index, the title details page shows whether the server has the title, with a link to play it there. Organising also works without a media server; the server then scans on its own schedule.

| Field                                        | Description                                                                                                                                                                                              |
| -------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Kind                                         | Emby, Jellyfin or Plex                                                                                                                                                                                   |
| Address                                      | The full address, such as `http://192.168.1.10:8096` (`:32400` for Plex). Redirects are not followed                                                                                                     |
| API key                                      | For Emby and Jellyfin, create one under "API keys" in the dashboard; for Plex, the X-Plex-Token. Stored encrypted and never shown again                                                                  |
| Refresh                                      | Scan only the organised folders (default), or refresh the whole library                                                                                                                                  |
| Path prefixes                                | When the media server sees different paths, fill in both prefixes, for example pt-tools' `/media` is `/data/media` in Emby                                                                               |
| Address for opening links (打开链接用的地址) | The address that "Play in Jellyfin" on the details page opens. Fill it in when the browser cannot open the address above (a container name such as `http://jellyfin:8096`); empty uses the address above |
| Index interval (索引间隔)                    | How often the [index](#index) is synced, in hours: 6 by default, at most 168; 0 turns the index off                                                                                                      |
| Continue-watching user (继续观看的用户)      | Emby and Jellyfin only: click Load users (读取用户) to read the users from the server and pick one. See [Continue watching](#continue-watching)                                                          |
| Count server episodes as in the library      | Off by default; see [Counting server episodes](#counting-server-episodes)                                                                                                                                |
| Webhook                                      | Generate the address while editing a saved server; see [Webhook](#webhook)                                                                                                                               |
| Playback notifications (播放通知)            | Pick channels to be notified when the webhook reports playback starting or stopping; see [Playback notifications](#playback-notifications)                                                               |

Click Test (测试) to check the address and key. The list shows the result of the last notification and the state of the index; a failed notification does not affect organising. With path prefixes set, a folder outside them is not sent to that server as a pt-tools path; the list says why.

When Jellyfin scans only the organised folders, scan the library in Jellyfin once after the first title is organised into an empty library. Jellyfin walks up the path to a folder it knows, and it only knows a library's folder after scanning it with something inside (scanning it while empty does not count); until then nothing is refreshed, and Jellyfin's real-time monitoring does not watch that folder either. After that scan, organised titles show up on their own.

### Index

The index records which movies, shows and episodes the server has (TMDB, IMDb and TVDB IDs and file paths). It is used for:

- the fourth line of "On your NAS" and Play in Jellyfin on the title details page, and the play button on each episode (see [title details](media-detail.md#on-your-nas));
- continue watching on the home page, where an episode is matched to its show through the index;
- [counting server episodes](#counting-server-episodes).

How syncing works:

- Each enabled server is synced in full every "index interval"; servers that were never synced are synced one minute after pt-tools starts. Sync (同步) in the list syncs at once and waits until it finishes.
- With the [webhook](#webhook) on, a server that reports a new or removed item is synced 2 minutes later; reports within those 2 minutes are merged into one sync.
- Nothing is written unless everything was read: when reading fails half way, takes over 10 minutes or exceeds 200,000 items, nothing is written, the list says why, and pt-tools tries again after 30 minutes. When items are added or removed on the server during a sync (pages may shift), only new and changed items are written and nothing is marked removed; the next sync catches up.
- Items no longer on the server are first marked removed and cleared after 90 days. When the kind or address changes, or the server's ID changes (a different server), the old index of that server is cleared before the next sync.
- File paths are turned into pt-tools paths with the path prefixes above; when that is not possible, only the server path is kept.

### Removed from the server

When a movie or an episode that pt-tools organised into the library disappears from the server (deleted, or its file is gone and the server dropped it on a scan), a sync records an issue:

- the fourth line of "On your NAS" on the title details page shows `S02E03 在服务器里被删了` (removed from the server) in the warning colour, and that episode's card shows `Jellyfin 里没有了` (no longer in Jellyfin);
- Handle (处理) opens the delete dialog for those episodes only (see [deleting a title](media-detail.md#deleting-a-title)); once the library files and organise records are deleted, the issue is marked handled. Deleting the record in organise history, or a replacement version retiring it, does the same;
- Ignore (忽略) stops the reminder for as long as the item stays away; when the server has the item again, the issue closes on its own.

The matching organise record is found by path first (the server path turned into a pt-tools path equals the record's target path); when the path cannot be turned, movies are matched by TMDB ID, and episodes by TMDB ID, season and episode. Items pt-tools did not organise are not recorded.

### Webhook

With the webhook on, the server tells pt-tools about new items, removed items and playback: new and removed items sync that server's index 2 minutes later, and playback starting or stopping sends a [playback notification](#playback-notifications). The webhook itself neither changes the index nor deletes anything.

Edit the server and click Generate address (生成地址) under Webhook. The address is shown only this once and looks like `http://<pt-tools address>/api/media/servers/1/webhook/<token>`; when the media server reaches pt-tools at a different address than the browser does (for example inside Docker), replace the first part. Regenerate address (重新生成地址) voids the old address, and Turn off (关掉) refuses everything. Only a digest of the token is stored, and logs mask it. Each server may send 60 requests a minute, with bodies of at most 1 MiB.

Enter the address in the media server:

| Server   | Where                                                                                                                                                                                                                                                                                                      |
| -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Jellyfin | Install the Webhook plugin from Dashboard → Plugins → Catalog, restart, then Add Generic Destination in its settings: Webhook Url is this address; tick Item Added, Item Deleted, Playback Start and Playback Stop, the movie, series and episode item types, and Send All Properties (no template needed) |
| Emby     | Add this address under Settings → Notifications (Webhooks) and pick the new media, deleted, playback start and playback stop events; either application/json or the form data type works                                                                                                                   |
| Plex     | Add this address under Settings → Webhooks (needs Plex Pass)                                                                                                                                                                                                                                               |

Jellyfin's Webhook plugin collects new and removed items and sends them about every minute and a half (playback starting and stopping goes out at once). Adding the minute Jellyfin itself collects changes before it scans and the 2 minutes pt-tools waits after a request arrives, an organised title reaches the index three to five minutes later. Right after installing the plugin the wait is longer: Jellyfin first runs the plugin's two scheduled tasks that send these one hour after it starts, as long as they have never run. To skip the wait, run Webhook Item Added Notifier and Webhook Item Deleted Notifier once under Dashboard → Scheduled Tasks in Jellyfin, then restart Jellyfin. The plugin keeps what it has collected in memory only: anything not yet sent when Jellyfin restarts is lost, and the scheduled sync at the index interval catches up.

The list and the dialog show when the last request arrived and its event, so you can check the server is sending. A server that is not enabled only has its requests recorded, not handled.

Jellyfin 12.2 with Webhook plugin 22.0.0.0 has been tested end to end; Emby and Plex only against their documentation and sample requests.

### Continue watching

Pick a continue-watching user in the server settings, and the [home page](home-and-health.md#continue-watching) lists the movies and episodes that user has not finished in Emby or Jellyfin (up to 12 per server); episodes show the season, episode number and title with a still, and open on the server when clicked. The server list on the media library page shows how many there are. Plex has no such option.

What counts as unfinished follows the server's own rules: by default Jellyfin only keeps the playback position of videos at least 5 minutes long, and counts under 5% watched as unplayed and over 90% as played. For short dramas whose episodes run a few minutes, lower Minimum resume duration under Dashboard → Playback → Resume in Jellyfin.

### Playback notifications

After picking playback notification channels in the server settings, one message is sent each time the webhook reports playback starting or stopping, for example:

```text
▶️ 开始播放：最后生还者 S01E01「当你迷失在黑暗中」
Jellyfin · admin · Living Room TV（Jellyfin Web）
```

Playing the same title again sends another one; when an episode's title is its file name (the server found no metadata), only the show and the episode number are written. Like other background notifications, messages are queued and go out within a minute; they wait out the channel's quiet hours and are retried on failure; the text can be changed under [message templates](notify-channels.md#message-templates), event `media.playback`. Nothing is sent without channels.

### Counting server episodes

With Count server episodes as in the library (把服务器里有的集算作已入库) on, episodes this server has count as in the library for followed shows: episodes with no organise record but present on the server show as in the library (source: media server) in the follow details and the title details, and are not downloaded. This suits titles put into the library outside pt-tools.

- Only show episodes count. Movies ignore this option: movie subscriptions without upgrades already ask the media servers whether they have the movie.
- For subscriptions with upgrades on, these episodes count as having no version (pt-tools cannot tell what the server's files are): a season pack that fits the quality profile is still downloaded and organised, and the server's own files stay, so the server ends up with two copies. Leave upgrades off for that show to avoid this.
- With the option, the server or the index turned off, these episodes count as missing again.

## Notifications

When channels are selected under Organise settings (整理设置), each run that adds files sends one message: title and year, episodes, quality, library and source torrent, ending with the TMDB poster address (Telegram usually shows it as a preview image). Messages wait for the channel's quiet hours and are retried on failure. Nothing is sent when no channel is selected.

## Organise history

There is one record per video file, showing which item it was organised as and where that item came from (the order at the end of [Which files are organised](#which-files-are-organised); records from before the upgrade don't say), where it went, how, which trigger organised it and whether it worked, plus the result of its last scrape: scraped (刮削成功), partly scraped (部分刮削: some selected items weren't written because the sources don't have them, a download failed or the Fanart.tv key is missing; or a source failed and TMDB's data was used) or scrape failed (刮削失败), with the reasons and notes on the line below.

| Status           | Description                                                                                                                                                                                                                                                              |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Done (已整理)    | The file is in the library                                                                                                                                                                                                                                               |
| Failed (失败)    | With the reason. Temporary problems such as TMDB being unreachable are retried after 10 minutes, 30 minutes, 1 hour, 3 hours and 12 hours, then no more; problems that need a settings change, such as no match or different file systems, are not retried automatically |
| Skipped (跳过)   | Another file is at the target, there's no video to organise, the disc can't be organised (see [discs](#discs)), or it's an ISO image in a TV show                                                                                                                        |
| Removed (已删除) | The files in the library have been deleted                                                                                                                                                                                                                               |

Failed and skipped records show a category: not recognised (没识别出来), library (媒体库: no enabled library, or a problem with the naming template or secondary categories), path mapping (路径映射: pt-tools can't find the file), file system (文件系统: different file systems, no permission), target already has a file (目标已有文件), source gone (源文件不在了), discs and images (原盘与镜像: discs that can't be organised, ISO images in TV shows) and other (其他). Failed and skipped records from before this version have no category and count as other. The list can be filtered by category when showing all, failed or skipped records.

After ticking some records, batch actions appear below the list:

- **Retry**: only the failed and skipped records among those ticked; several records of the same torrent (or the same monitored item or path) are organised once.
- **Rescrape**: only organised records; the same title in the same library is scraped once.
- **Delete records** (删除记录) and **delete with files** (连文件删除): after a confirmation; deleting with files follows the rules for deleting below, and moved records can only have their record deleted, which the result says.

Retry and rescrape answer once the work is queued; refresh later for the results. The result says how many records were done and why the others weren't.

By title (按条目) groups the records by movie or by season of a show (records that weren't recognised by torrent name) and shows how many records each group has, how many per status and the latest time; the status, category and keyword filters still apply. View records (看记录) lists a group's records, and Back to groups (返回分组) returns.

Records of [imported existing files](#importing-an-existing-library) show the mode as existing file (已有文件) and the trigger as import (导入已有文件); they can't be retried, and deleting them with their files asks for a second confirmation.

Records that weren't recognised have Ask AI (让 AI 认): it opens the organise dialog and asks the AI once after the preview, see [Asking the AI](#asking-the-ai).

Retry (重试) organises the whole torrent of the record again; use it after changing recognition words, corrections or libraries. A record organised by hand with an organise mode picked is retried with that mode. Failed or skipped records shown as 手动（指定路径） (manual, by path) also have Manual organise (手动整理): it opens the dialog with the path entered and the organise mode picked back then, so you can change the title, library or mode and organise again.

Rescrape (重新刮削) rewrites the NFO files and images of the record's movie or show (every episode of it organised in this library) with the library's current scraping settings; use it after changing the scraping content, data sources or image language, or after entering a new key under media recognition:

- Files pt-tools wrote while scraping and that haven't been edited since are replaced (even without Overwrite); files you placed or another tool wrote are replaced only with Overwrite existing NFO and images (覆盖已有的 NFO 与图片) on; locked NFO files are left alone; missing files are added. Files whose content is unchanged aren't rewritten; files kept because they were edited or locked are listed in the result.
- No source answers from the cache: their current data is fetched (and cached again).
- Only records whose video in the library is still the file pt-tools organised are scraped: when the video is gone or was replaced with another file, nothing is written and the record is marked scrape failed with the reason.
- It is queued like organising and runs one at a time; if it takes a while, the page answers first and you can refresh later.
- Records organised before this version don't remember which category folder they went into: when the library puts titles into category folders and the folder can't be worked out, the record isn't scraped and the result says so.

When deleting, you can check Also delete the organised files in the library (同时删除库里整理出的文件): videos and subtitles are deleted only when still confirmed to be the files pt-tools created (hard links and copies by file ID: device, inode, size and modification time; symbolic links by their target); NFO files and images only when they are the copies pt-tools wrote, and the folder's posters, backgrounds, logos and NFO files only once no video is left; empty folders go last. Replaced or edited files stay and are listed after the deletion. When a record turns skipped or failed (for example because the video in the library was replaced with another file), the subtitles and scraped files organised earlier stay recorded on it and are cleaned up by the same rules when it is deleted. Moved files are the only copy and are never deleted here. Files in the download folder are never touched.

## Missing library files

Once a day pt-tools checks that the files of organised records are still in the library (hard links and copies by the file itself, symbolic links also by their target, discs by the main file). When one is gone it records an issue:

- the first line of "On your NAS" on the title details page shows `S02E05 的文件不见了` (the file of S02E05 is missing) in the warning colour, and that episode's card shows `库里的文件不见了` (library file missing);
- Handle (处理) opens the delete dialog for those episodes only (see [deleting a title](media-detail.md#deleting-a-title)); once the organise record is deleted, the issue is marked handled;
- after Ignore (忽略) it is not mentioned again until the file comes back; when the file is back, the issue closes on its own, and so it does when the organise record is deleted or no longer organised.

The check only records issues; it deletes nothing. The first check runs 10 minutes after start-up when there has never been one, then every 24 hours, scheduled from the previous round after a restart. One round checks up to 5000 records, and the next round carries on from there. Records whose library folder is missing (for example not mounted) or whose library was deleted are skipped, so they are never taken for missing files.

## Asking the AI

When the torrent name can't be matched to a title (the organise dialog's preview says 没有识别出来, or the history shows the not recognised category), Ask AI (让 AI 认) searches TMDB for movies and shows by the torrent name (with recognition words applied and corrections ignored, at most 8 candidates) and sends them, with the torrent name and the site's title, to the model of the [AI assistant](./assistant.md), which picks one or says none fits.

- When it picks one, the type and TMDB ID are filled into the row for fixing the match, the preview is worked out for that title, and the AI's reason, the model and the token counts are shown. **You still click Organise yourself**, after checking the preview or changing the title or library.
- With Record as a correction when organising (整理时记成纠正, ticked by default), clicking Organise first saves a [correction](./media-recognize.md), so torrents with the same name are matched to this title from then on; changing the title in the row turns this off.
- When the AI says none fits, the first few candidates are listed so you can enter a TMDB ID yourself. When TMDB has no candidates, the model isn't asked.
- The AI assistant must be enabled with its endpoint, key and model set up first; otherwise the dialog says why. Each request waits at most 30 seconds.
- Only the torrent name, the site's title and each candidate's type, name, year and the first 80 characters of its overview go to the model, with long strings that look like secrets masked first. It uses the AI assistant's endpoint without tools, so it can't organise or change anything.

## Interaction with auto cleanup

Once a torrent is hard-linked into the library, deleting it with its data frees no space (the library link still holds it), so the low-space emergency cleanup does not pick hard-linked torrents. Regular cleanup rules still apply: delete the torrent when it has seeded enough and the library file stays watchable.

Delete library links when torrents are deleted (删种时一并删除入库链接) is off by default. When on, pt-tools checks every 10 minutes for torrents that are gone from the downloader and whose source files are gone too (deleted with data), deletes the hard links and symbolic links organised from them and marks the records removed; copies and moved files stay. With it on, the emergency cleanup also picks hard-linked torrents, since deleting them now frees space. Nothing is deleted while the torrent's save folder or the library folder is missing (for example not mounted) or the downloader cannot be reached.

When the [delete dialog](media-detail.md#deleting-a-title) on title details deletes a torrent with its data while its library files stay unticked, the hard links organised from it are the ones you are keeping: their records get a mark, and Delete library links when torrents are deleted leaves them alone. Organising the same source file into the library again later removes the mark.

## Troubleshooting

**Failed: the source and the library are not on the same file system**: hard links need the download folder and the library on one partition. In Docker, mount them as one volume as in [preparing the folders](#preparing-the-folders), or switch the library to copy.

**Failed: pt-tools cannot find a path**: the downloader and pt-tools see different paths; add a path mapping, then retry in the history (filter by path mapping to retry them together). Once a retry finds the files, the failed records written under the downloader's path are removed.

**No match**: preview the torrent name under [media recognition](media-recognize.md), pick a candidate or enter a TMDB ID, then click Retry (重试) in the history; or enter the TMDB ID directly when organising by hand, or [ask the AI](#asking-the-ai).

**Another file is at the target**: pt-tools does not overwrite other files. If you no longer need the file in the library, delete it and retry.

**Subscriptions download again what's already in a library I organised myself**: subscriptions only go by organise records. Click Import existing files (导入已有文件) under Media library to record the movies and shows already in the library, see [importing an existing library](#importing-an-existing-library).

**How do I organise a movie I downloaded by hand, and where do the links go**: see [organising a file or folder by hand](#organising-a-file-or-folder-by-hand).

**Site subtitles are on but nothing was downloaded**: look at the record's message in the organise history. "站点上没有这个种子的字幕" means the site has none for this torrent (or has changed its detail page). With no message at all, the torrent most likely had its own subtitles, wasn't pushed by pt-tools, or comes from a site that doesn't run NexusPHP; see [site subtitles](#site-subtitles).
