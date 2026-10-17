# Media library organising <Badge type="tip" text="1.0 preview" />

pt-tools can put finished movies and TV shows into your media library: it recognises the title, places the files in the library folder following the library's naming template (hard links by default, so no extra space is used and the torrent keeps seeding), writes NFO files and artwork, asks Emby, Jellyfin or Plex to scan, and sends a notification.

Settings live under Media → Media library (媒体 → 媒体库) and results under Media → Organise history (媒体 → 整理历史). Recognition uses TMDB, so fill in the TMDB API key under [media recognition](media-recognize.md) first.

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

| Field           | Description                                                                                                                                     |
| --------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| Type            | Movie or TV; with Animation only checked it takes animation only                                                                                |
| Library folder  | The path as pt-tools sees it (absolute)                                                                                                         |
| Mode            | Hard link (default), copy, symbolic link or move, see below                                                                                     |
| Template        | Leave empty for the default, see [naming templates](#naming-templates)                                                                          |
| Scraping        | Write NFO files and images; what to write and which image language to use, see [scraping](#scraping); turn it off to let the media server do it |
| Overwrite       | Rewrite NFO files and images on every run; when off, existing files are left alone                                                              |
| Same-name files | Whether to replace an old version pt-tools put into the library, see [files with the same name](#files-with-the-same-name)                      |
| Categories      | With Organise by category checked, files go into `<library folder>/<category>/…`, see [secondary categories](#secondary-categories)             |

| Mode          | Description                                                                                                                              |
| ------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| Hard link     | No extra space and the torrent keeps seeding; the download folder and the library must be on the same file system                        |
| Copy          | Uses space for a second copy and works across partitions; written to a temporary file first, then renamed                                |
| Symbolic link | No extra space; the media server must reach the download folder by the same path, and the link breaks when the torrent's data is deleted |
| Move          | The files move into the library and the torrent can no longer seed; only paused torrents are organised                                   |

When a move cannot delete the source file (for example because the download folder is read-only), the copy already placed in the library is still recorded as organised, with the mode it actually has, copy (across file systems) or hard link, and a note that the source file is still in the download folder; delete it yourself if needed.

Click Check (检查) while editing a library: it confirms the folder exists and is writable, and for hard links it creates a file in the pt-tools path of each path mapping and tries to hard-link it into the library, which confirms they are on the same file system. The test files are removed afterwards.

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
- Original languages are ISO 639-1 codes (`zh`, `ja` and so on; TMDB records Cantonese as `cn`), countries and regions are ISO 3166-1 codes (`CN`, `US` and so on). Codes that aren't in the list can be typed in.
- Fill in common categories (填入常用分类) uses the same categories as MoviePilot's defaults: animated, Chinese-language and foreign movies; for TV, Chinese animation, Japanese anime, documentaries, kids, variety, Chinese, Western and Japanese/Korean series, and uncategorised.

The category name is the folder name, so it can't contain `/ \ : * ? " < > |`. Changing the rules or the checkbox only affects files organised afterwards; files already in the library stay where they are. For TV, the NFO files, poster and logo go into the show folder inside the category folder. The organise preview shows which category a title goes into. Categorising needs TMDB's details (a movie's production countries are only there); when they can't be fetched, the title isn't organised this time and is retried later.

## Files with the same name

When a movie or an episode gets another release (say 1080p first and 2160p later) and both come out with the same file name, the old version already occupies that spot in the library. Same-name files (同名文件) on the library decides what happens:

| Option              | What it does                                                              |
| ------------------- | ------------------------------------------------------------------------- |
| Keep (default)      | Keep the old version; the new one is recorded as skipped, with the reason |
| Replace when larger | Replace only when the new file is larger than the old version             |

Only files pt-tools put into this library for the same movie (or the same episode) are replaced, confirmed by the organise record and file ID. Files you put there yourself, files that were changed, and moved files (the only copy) are never replaced, and nothing is replaced when the library organises by moving. The new file is placed under a temporary name next to the target first, the old file is moved to a backup name and checked to be the one the old record placed, then the new file goes into place, and the old file is deleted only after the records are saved. If any step fails the old file goes back where it was, and if pt-tools stops halfway the unfinished replacement is rolled back on the next start. The old version's subtitles and same-name NFO and thumbnail are swapped for the new ones, folder images stay, and the old record is marked removed with what replaced it. Files in the download folder are never touched. When the library's file system supports neither hard links nor no-overwrite renames (some network storage), nothing is replaced and the reason is shown.

Only size decides: the smaller old version never swaps the larger one back when it's organised again (automatically or by hand); it's recorded as skipped. To go back, delete the new version's record together with its library files under organise history, then organise the old one. When the naming template includes the quality (the default movie template does), different releases get different file names, never collide, and both stay in the library; to keep only one, drop the quality from the template or use subscription upgrades.

## Which files are organised

- Video files: mkv, mp4, m4v, avi, ts, m2ts, mts, mov, wmv, flv, rmvb, rm, webm, mpg, mpeg.
- Skipped: files in Sample, Trailers, Featurettes, Extras, SPs and similar folders, file names ending in sample or trailer, NCOP and NCED openings and endings, and files smaller than Minimum video size (视频最小体积, 50 MB by default). The Specials folder holds season 0 episodes and is organised as usual.
- Discs (BDMV and VIDEO_TS folders) and ISO images are not organised.
- Subtitles (srt, ass, ssa, sub, idx, sup, vtt) go with their video (except those in extras or sample folders) and get a language suffix from the file name: `zh-CN` simplified Chinese, `zh-TW` traditional Chinese, `zh` Chinese, `en` English, `ja` Japanese, `ko` Korean. When a torrent has several videos, subtitles are matched by file name prefix or by season and episode.
- Season and episode numbers come from each file name, with [recognition words](media-recognize.md#recognition-words) applied (episode offsets included); single-file torrents use the torrent name.

Recognition uses the torrent name (and the site title as a subtitle when it differs), looking up the site's IMDb ID first when there is one.

## When organising runs

| Trigger       | Description                                                                                                                                                                                      |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Automatic     | Torrents pushed by pt-tools are organised when they finish downloading. Only torrents finished after you turn it on; missed ones are picked up every 10 minutes                                  |
| Periodic scan | Finished torrents in the downloaders are scanned at the interval, including ones pt-tools did not push. Up to 50 per round; torrents that already have records are skipped                       |
| Monitoring    | Monitored folders that are switched on are scanned at the interval, and videos dropped there (not torrents) are organised, see [directory monitoring](#directory-monitoring)                     |
| Manual        | The organise icon in the Status (状态) column of the task list (整理入库 on mobile cards); in the downloader web UI, select one finished torrent and click 整理入库, or use the right-click menu |

The organise settings show when the periodic scan and directory monitoring last ran and roughly when they run next (these times start over when pt-tools restarts, and the first round after start runs within a minute).

Automatic organising and periodic scans only take torrents in the scope: downloaders, categories, tags and save paths (as the downloader sees them). An empty item means no limit; a filled one must match one of its values. Their automatic retries follow the current settings too: once the switch is turned off or the torrent is out of scope, they are not retried.

Manual organising shows a preview first: the recognized title, the chosen library and where each file goes. When the match is wrong, enter a TMDB ID (choose movie or TV) or pick another library of the same type, click Preview again (重新预览), then organise. A torrent organised before keeps the title it was organised with (the preview says 沿用之前整理用的条目); to change it, enter a TMDB ID or delete its records first. Copying large files can take a while; the dialog waits 20 seconds, the work continues in the background and the result appears in the history.

Each file is organised once: repeated completion events or several triggers at the same time never organise it twice. When the library already has this file (the same inode, or a symbolic link to the source), it counts as done. When another file is at the target, it is not overwritten and the record says skipped (copied and moved files cannot be told apart from someone else's, so an existing target is always skipped); when it's an older version of the same title that pt-tools placed, the library's same-name setting decides, see [files with the same name](#files-with-the-same-name). Subtitles that fail are noted in the record and added the next time the torrent is organised (a retry or another manual run).

## Directory monitoring

Videos dropped into a monitored folder (not torrents in a downloader, for example files copied by hand or downloaded by another tool) can be organised into the libraries too. Add folders (paths as pt-tools sees them) under Directory monitoring (目录监控) on the media library page; the scan interval is in the organise settings, 10 minutes by default. A folder's path can't be changed after it's added: to monitor another folder, delete it and add the new one.

- Each item in the folder (one file, or one subfolder) is treated like a torrent: it's recognised by its name, its videos go into the library with the library's mode (hard link, copy, symbolic link or move), and the organise record shows 目录监控 as the downloader.
- Items changed within the last 2 minutes wait (they may still be copying). Items whose videos all have organise records aren't scanned again; when a subfolder gets more episodes later, the new ones are organised.
- Hidden files and folders, NAS system folders (`@eaDir`, `#recycle` and the like) and unfinished downloads (`.part`, `.!qB` and the like) are ignored. An item more than 6 folders deep or with more than 5000 files isn't organised at all, and organise history says why.
- Each location has one organise record, which stores the source file's ID (the same file with the same size and modification time counts as unchanged): when another file appears where one was organised before (for example a file with the same name dropped in after the old one was moved), it isn't organised while that record still tracks library files, and the record says so; delete that record and the next scan organises the new file. When the record tracks no library files (for example a failed recognition), the new file is organised.
- A monitored folder can't be a library folder, be inside one or contain one. Don't monitor a downloader's download folder either: its torrents are handled by automatic organising and the periodic scan.
- When the library moves files, pt-tools first checks that the item isn't a torrent in a downloader (moving it away would stop the torrent from seeding). If it is, the item isn't organised and the record says why; when a downloader can't be reached to check, it's tried again later. Torrent paths are converted with the path mappings before comparing; torrents whose paths pt-tools can't see (no path mapping, or a downloader on another machine) are matched by name, so a torrent with the same name as the item counts as that item.
- There's no site title or IMDb ID, only the name. When the recognition is wrong, add a correction or recognition word under media recognition, then retry from organise history.
- Delete links when torrents are removed doesn't touch files organised from monitored folders: deleting the source file in the monitored folder leaves the library file in place.

## Scraping

With scraping on, Scraping content (刮削内容) lets you pick NFO, poster, background, logo, season posters and episode thumbnails one by one (the last two only exist for TV). A library you haven't changed writes everything except the logo.

| Title | Files written                                                                                                                                                                                                                                          |
| ----- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Movie | `<file name>.nfo`; with its own folder, `poster.jpg`, `fanart.jpg` and `clearlogo.png`; directly in the library root, `<file name>-poster.jpg`, `<file name>-fanart.jpg` and `<file name>-clearlogo.png`                                               |
| TV    | In the show folder: `tvshow.nfo`, `poster.jpg`, `fanart.jpg`, `clearlogo.png` and `season01-poster.jpg` (`season-specials-poster.jpg` for season 0); `season.nfo` in the season folder; `<file name>.nfo` and `<file name>-thumb.jpg` for each episode |

The logo is the title artwork on a transparent background; Emby, Jellyfin and Kodi all read `clearlogo.png`. Image language (图片语言) decides which image is used:

| Image language                 | Poster                                              | Background           | Logo                                                       |
| ------------------------------ | --------------------------------------------------- | -------------------- | ---------------------------------------------------------- |
| Recognition language (default) | The one TMDB returns for the recognition language   | The one TMDB returns | Recognition language, then English, then without text      |
| Chinese, English or Japanese   | That language first, otherwise the one TMDB returns | Without text first   | That language, then the recognition language, then English |
| Without text                   | Without text first                                  | Without text first   | Without text, then the recognition language, then English  |

For the logo or a chosen image language, pt-tools fetches the title's image list once more (cached for a week). If that fails, the poster and background fall back to the ones TMDB returns, no logo is written, and the organize result says why. Season posters and episode thumbnails always use the ones TMDB returns.

The NFO files use the Kodi format, read by the NFO support of Emby, Jellyfin and Plex, and include the TMDB and IMDb IDs. Images are downloaded from TMDB through the proxy set under media recognition. When the template puts episodes directly in the library root, the show and season files are not written.

Plot and rating (简介与评分) come from TMDB by default. With Douban (豆瓣) selected, pt-tools looks the title up on Douban by its Chinese name before writing the NFO, then by its original title. Among titles with a matching name, the same year wins; otherwise a title within one year is used only when it's the only one. When more than one title has the name and it's unclear which, TMDB's data is used.

- For a movie, the plot is replaced with Douban's, Douban's rating becomes the main one (TMDB's is kept), and the Douban ID is added. When a torrent pushed by pt-tools carries a Douban ID from the site, that ID is tried first and used only when its name and year match.
- Douban lists each season of a series separately: `tvshow.nfo` uses season 1, and `season.nfo` uses that season's plot. Episode NFO files still come from TMDB.
- Douban has no public API; pt-tools uses the two endpoints Douban's own web pages use (search and title details), doesn't log in, and doesn't go through the media recognition proxy. When nothing is found or Douban can't be reached, TMDB's data is used; a manual organise says so in its result, and background organising writes it to the log.
- Douban is requested at most once every 2 seconds; search results are cached for a day and titles for a week. Lookups happen only for NFO files that will be written: existing ones that aren't overwritten aren't looked up, and they're replaced after turning on Overwrite existing NFO and images (覆盖已有的 NFO 与图片) and organising again.

## Media servers

After organising, pt-tools asks the media servers to scan the new folders. Organising also works without a media server; the server then scans on its own schedule.

| Field         | Description                                                                                                                             |
| ------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| Kind          | Emby, Jellyfin or Plex                                                                                                                  |
| Address       | The full address, such as `http://192.168.1.10:8096` (`:32400` for Plex). Redirects are not followed                                    |
| API key       | For Emby and Jellyfin, create one under "API keys" in the dashboard; for Plex, the X-Plex-Token. Stored encrypted and never shown again |
| Refresh       | Scan only the organised folders (default), or refresh the whole library                                                                 |
| Path prefixes | When the media server sees different paths, fill in both prefixes, for example pt-tools' `/media` is `/data/media` in Emby              |

Click Test (测试) to check the address and key. The list shows the result of the last notification; a failed notification does not affect organising. With path prefixes set, a folder outside them is not sent to that server as a pt-tools path; the list says why.

## Notifications

When channels are selected under Organise settings (整理设置), each run that adds files sends one message: title and year, episodes, quality, library and source torrent, ending with the TMDB poster address (Telegram usually shows it as a preview image). Messages wait for the channel's quiet hours and are retried on failure. Nothing is sent when no channel is selected.

## Organise history

There is one record per video file, showing where it went, how, which trigger organised it and whether it worked.

| Status           | Description                                                                                                                                                                                                                                                              |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Done (已整理)    | The file is in the library                                                                                                                                                                                                                                               |
| Failed (失败)    | With the reason. Temporary problems such as TMDB being unreachable are retried after 10 minutes, 30 minutes, 1 hour, 3 hours and 12 hours, then no more; problems that need a settings change, such as no match or different file systems, are not retried automatically |
| Skipped (跳过)   | Another file is at the target, or the torrent is a disc or has no video to organise                                                                                                                                                                                      |
| Removed (已删除) | The files in the library have been deleted                                                                                                                                                                                                                               |

Retry (重试) organises the whole torrent of the record again; use it after changing recognition words, corrections or libraries. When deleting, you can check Also delete the organised files in the library (同时删除库里整理出的文件): videos and subtitles are deleted only when still confirmed to be the files pt-tools created (hard links and copies by file ID: device, inode, size and modification time; symbolic links by their target); NFO files and images only when they are the copies pt-tools wrote, and the folder's posters, backgrounds, logos and NFO files only once no video is left; empty folders go last. Replaced or edited files stay and are listed after the deletion. When a record turns skipped or failed (for example because the video in the library was replaced with another file), the subtitles and scraped files organised earlier stay recorded on it and are cleaned up by the same rules when it is deleted. Moved files are the only copy and are never deleted here. Files in the download folder are never touched.

## Interaction with auto cleanup

Once a torrent is hard-linked into the library, deleting it with its data frees no space (the library link still holds it), so the low-space emergency cleanup does not pick hard-linked torrents. Regular cleanup rules still apply: delete the torrent when it has seeded enough and the library file stays watchable.

Delete library links when torrents are deleted (删种时一并删除入库链接) is off by default. When on, pt-tools checks every 10 minutes for torrents that are gone from the downloader and whose source files are gone too (deleted with data), deletes the hard links and symbolic links organised from them and marks the records removed; copies and moved files stay. With it on, the emergency cleanup also picks hard-linked torrents, since deleting them now frees space. Nothing is deleted while the torrent's save folder or the library folder is missing (for example not mounted) or the downloader cannot be reached.

## Troubleshooting

**Failed: the source and the library are not on the same file system**: hard links need the download folder and the library on one partition. In Docker, mount them as one volume as in [preparing the folders](#preparing-the-folders), or switch the library to copy.

**Failed: pt-tools cannot find a path**: the downloader and pt-tools see different paths; add a path mapping.

**No match**: preview the torrent name under [media recognition](media-recognize.md), pick a candidate or enter a TMDB ID, then click Retry (重试) in the history; or enter the TMDB ID directly when organising by hand.

**Another file is at the target**: pt-tools does not overwrite other files. If you no longer need the file in the library, delete it and retry.
