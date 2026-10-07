# Media library organising

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

| Field          | Description                                                                                                           |
| -------------- | --------------------------------------------------------------------------------------------------------------------- |
| Type           | Movie or TV; with Animation only checked it takes animation only                                                      |
| Library folder | The path as pt-tools sees it (absolute)                                                                               |
| Mode           | Hard link (default), copy, symbolic link or move, see below                                                           |
| Template       | Leave empty for the default, see [naming templates](#naming-templates)                                                |
| Scraping       | Write NFO files, poster, background, season posters and episode thumbnails; turn it off to let the media server do it |
| Overwrite      | Rewrite NFO files and images on every run; when off, existing files are left alone                                    |

| Mode          | Description                                                                                                                              |
| ------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| Hard link     | No extra space and the torrent keeps seeding; the download folder and the library must be on the same file system                        |
| Copy          | Uses space for a second copy and works across partitions; written to a temporary file first, then renamed                                |
| Symbolic link | No extra space; the media server must reach the download folder by the same path, and the link breaks when the torrent's data is deleted |
| Move          | The files move into the library and the torrent can no longer seed; only paused torrents are organised                                   |

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
| Manual        | The organise icon in the Status (状态) column of the task list (整理入库 on mobile cards); in the downloader web UI, select one finished torrent and click 整理入库, or use the right-click menu |

Automatic organising and periodic scans only take torrents in the scope: downloaders, categories, tags and save paths (as the downloader sees them). An empty item means no limit; a filled one must match one of its values.

Manual organising shows a preview first: the recognized title, the chosen library and where each file goes. When the match is wrong, enter a TMDB ID (choose movie or TV) or pick another library of the same type, click Preview again (重新预览), then organise. A torrent organised before keeps the title it was organised with (the preview says 沿用之前整理用的条目); to change it, enter a TMDB ID or delete its records first. Copying large files can take a while; the dialog waits 20 seconds, the work continues in the background and the result appears in the history.

Each file is organised once: repeated completion events or several triggers at the same time never organise it twice. When the library already has this file (the same inode, or a symbolic link to the source), it counts as done. When another file is at the target, it is not overwritten and the record says skipped (copied and moved files cannot be told apart from someone else's, so an existing target is always skipped). Subtitles that fail are noted in the record and added the next time the torrent is organised (a retry or another manual run).

## Scraping

| Title | Files written                                                                                                                                                                                                                         |
| ----- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Movie | `<file name>.nfo`; with its own folder, `poster.jpg` and `fanart.jpg`; directly in the library root, `<file name>-poster.jpg` and `<file name>-fanart.jpg`                                                                            |
| TV    | In the show folder: `tvshow.nfo`, `poster.jpg`, `fanart.jpg` and `season01-poster.jpg` (`season-specials-poster.jpg` for season 0); `season.nfo` in the season folder; `<file name>.nfo` and `<file name>-thumb.jpg` for each episode |

The NFO files use the Kodi format, read by the NFO support of Emby, Jellyfin and Plex, and include the TMDB and IMDb IDs. Images are downloaded from TMDB through the proxy set under media recognition. When the template puts episodes directly in the library root, the show and season files are not written.

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

Retry (重试) organises the whole torrent of the record again; use it after changing recognition words, corrections or libraries. When deleting, you can check Also delete the organised files in the library (同时删除库里整理出的文件): videos and subtitles are deleted only when still confirmed to be the files pt-tools created (hard links and copies by file ID: device, inode, size and modification time; symbolic links by their target); NFO files and images only when they are the copies pt-tools wrote, and the folder's posters, backgrounds and NFO files only once no video is left; empty folders go last. Replaced or edited files stay and are listed after the deletion. When a record turns skipped or failed (for example because the video in the library was replaced with another file), the subtitles and scraped files organised earlier stay recorded on it and are cleaned up by the same rules when it is deleted. Moved files are the only copy and are never deleted here. Files in the download folder are never touched.

## Interaction with auto cleanup

Once a torrent is hard-linked into the library, deleting it with its data frees no space (the library link still holds it), so the low-space emergency cleanup does not pick hard-linked torrents. Regular cleanup rules still apply: delete the torrent when it has seeded enough and the library file stays watchable.

Delete library links when torrents are deleted (删种时一并删除入库链接) is off by default. When on, pt-tools checks every 10 minutes for torrents that are gone from the downloader and whose source files are gone too (deleted with data), deletes the hard links and symbolic links organised from them and marks the records removed; copies and moved files stay. With it on, the emergency cleanup also picks hard-linked torrents, since deleting them now frees space. Nothing is deleted while the torrent's save folder or the library folder is missing (for example not mounted) or the downloader cannot be reached.

## Troubleshooting

**Failed: the source and the library are not on the same file system**: hard links need the download folder and the library on one partition. In Docker, mount them as one volume as in [preparing the folders](#preparing-the-folders), or switch the library to copy.

**Failed: pt-tools cannot find a path**: the downloader and pt-tools see different paths; add a path mapping.

**No match**: preview the torrent name under [media recognition](media-recognize.md), pick a candidate or enter a TMDB ID, then click Retry (重试) in the history; or enter the TMDB ID directly when organising by hand.

**Another file is at the target**: pt-tools does not overwrite other files. If you no longer need the file in the library, delete it and retry.
