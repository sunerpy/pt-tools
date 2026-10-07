# IYUU cross-seeding

IYUU cross-seeding (IYUU 辅种), under Download (下载), uses [IYUU](https://doc.iyuu.cn/) to find torrents on other sites that carry the same data, and adds them to the same downloader: the data is stored once and seeds on several sites. It is off by default.

## Before you turn it on

- You need an IYUU token; see [IYUU's documentation](https://doc.iyuu.cn/) for how to get one.
- When it is on, pt-tools sends the info hashes of the finished torrents in the participating downloaders to IYUU (`2025.iyuu.cn`).
- The token is stored on this machine, encrypted with the same key as the site cookies; the page and the API never show it, only whether it is set.

## Settings

| Setting                         | Meaning                                                                                   |
| ------------------------------- | ----------------------------------------------------------------------------------------- |
| Turn on (开启辅种)              | Runs at the interval when on; when off, only Run now (立即运行) runs it                   |
| IYUU token                      | Left empty, the saved token stays; Clear (清除) deletes it and turns cross-seeding off    |
| Interval (运行间隔)             | 1 to 168 hours, 12 by default                                                             |
| Per site per day (每站每天最多) | Attempts per site per day, failed downloads and checks included; 20 by default, up to 500 |
| Downloaders (参与的下载器)      | None selected means every enabled downloader                                              |
| Sites (参与的站点)              | None selected means every site that is configured in pt-tools and supported by IYUU       |

The Sites (站点) tab lists the sites IYUU supports and their state in pt-tools: taking part, not configured (pt-tools knows the site but it has no cookie or other credentials yet), not selected, or not supported by pt-tools. pt-tools matches a site by the host name and registrable domain of its address; see [Recognising sites by tracker](downloader-assistant.md#recognising-sites-by-tracker).

## What a run does

1. Reads the finished torrents in the participating downloaders (except those in error or being checked) and sends their info hashes to IYUU in batches of 200.
2. For each torrent IYUU finds, skips it when its site is not taking part, the downloader already has it, this torrent on this site was tried before, or the site has reached today's limit.
3. Downloads the torrent file from the site (with the site's rate limit), checks that its info hash matches IYUU's, and checks that its file list is exactly the same as the original torrent's in the downloader (every file with the same path and size). A mismatch is recorded as failed and not tried again; a torrent that cannot be downloaded (the site is unavailable, for example) is also recorded as failed and tried again after 7 days.
4. Adds a torrent that passes the checks to the same downloader, paused, with the original torrent's save path and the tags site ID, `pt-tools-reseed` and `pt-tools-reseed-<job number>`, which belongs to this job only and is removed when the job ends.
5. Once the downloader checks it at 100%, the torrent starts seeding and pt-tools records it (site seeding capacity and auto-delete both see it); below 100% it is removed from the downloader with the data kept, and the original torrent is not affected.

Steps 4 and 5 use the same jobs as [Torrent transfer](torrent-transfer.md): each downloader checks one torrent at a time, and after pt-tools restarts a job continues from the step where it stopped. Cross-seeding never removes an existing torrent.

When IYUU rate-limits a request, pt-tools waits once if the wait is at most a minute, otherwise it stops the run and tries again next time. When the token is cleared or replaced during a run, the run stops before it queries the next batch and does not send info hashes with the old token.

## Deleting torrents and shared data

A cross-seed and its original torrent use the same files. When auto-delete (emergency cleanup and deleting torrents still unfinished when the free period ends included), the seeding competition monitor (做种竞争度监控), brushing removal or Delete dead torrents (删除失效种子) in the downloader assistant is about to delete data, pt-tools first checks whether another torrent in the downloader still uses the same data: if one does, only the torrent is removed and the data stays; when all the torrents sharing the data are deleted together, the data is deleted as usual. pt-tools compares the paths where the torrents keep their data: the same path counts as shared.

Deleting a torrent by hand, in the downloader or in a pt-tools torrent list, with its data selected for deletion deletes the files, and the torrents sharing them lose their data too.

## Records

The Records (记录) tab lists the latest 200 records by the time of their last attempt: torrent, site and torrent ID, result, reason and time (a retry after 7 days updates the original record). The result is the progress of the job (waiting to add, checking and so on), or cross-seeded (已辅种), withdrawn (已撤回, the check stopped short of 100% and the torrent was removed), or failed (失败, the download or a check failed). Clearing finished cross-seeding jobs writes each job's result into its record first, so the result in the record stays; a record whose job was deleted some other way shows Job deleted (任务已删除). The summary of each run shows as Last run (上次运行) at the bottom of the Settings tab.

## API

| Method and path                   | Purpose                                                                                                                                                    |
| --------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET`, `PUT /api/reseed/settings` | Read or save the settings: everything but `token` is saved as a whole and unknown fields are rejected; without `token` the saved one stays, `""` clears it |
| `GET /api/reseed/sites`           | IYUU sites and the matching pt-tools sites                                                                                                                 |
| `POST /api/reseed/run`            | Start a run in the background; 202, or 409 while a run is in progress                                                                                      |
| `GET /api/reseed/records`         | The latest 200 records (by the time of the last attempt) with their job states                                                                             |
| `GET`, `DELETE /api/reseed/jobs`  | Cross-seeding jobs; DELETE clears finished jobs after writing their results into the records                                                               |

All of them require a signed-in session.
