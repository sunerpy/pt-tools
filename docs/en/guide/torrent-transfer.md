# Torrent transfer

Torrent transfer (转移做种), under Download (下载), moves finished torrents from one downloader to another and keeps seeding them there. The data files stay where they are. For example, hand long-seeding torrents from qBittorrent on an SSD to Transmission on a NAS.

## Transferring

Tick torrents in the Downloader web UI (下载器 Web UI), from one or more downloaders, or tick records already pushed to a downloader in the Task list (任务列表). Click Transfer to… (转移到…), pick the target downloader and click Preview (预览). For each torrent the preview shows:

- Whether it can be transferred. Otherwise the reason: not finished, already in the target downloader, already being transferred, no torrent file available, and so on.
- The source path and the path in the target downloader (see [Path mappings](#path-mappings)).
- Where the torrent file comes from: qBittorrent 4.5 and later export it directly; for older qBittorrent and for Transmission, pt-tools downloads it again from the site using its push record and checks the info hash. A torrent without a push record can only leave qBittorrent 4.5 or later.

After Create N transfer jobs (建 N 个转移任务), the jobs run in the background, one at a time:

1. Get the torrent file.
2. Add the torrent to the target downloader paused, with the mapped save path and two tags on top of its category and tags: `pt-tools-transfer` and `pt-tools-transfer-<job number>`, which belongs to this job only. This goes through the same entry point as pushing a torrent: the site seeding capacity limit still applies; the data is already on disk, so no disk space is reserved.
3. Let the target downloader check the data.
4. Once the check reaches 100%, start seeding in the target downloader, then remove the torrent from the source downloader, keeping the data files. pt-tools confirms the target is still at 100% right before it starts seeding. The pt-tools records of the torrent on the source downloader move to the target downloader.

If the check stops short of 100% (the data is not at the target path, usually a wrong path mapping) or runs out of time (30 minutes plus the time to read all the data at 20 MiB/s), the torrent is removed from the target downloader with its data kept, the torrent in the source downloader is left alone, and the job is marked Rolled back (已回滚).

Each target downloader checks one transferred torrent at a time and the rest wait, so that many checks at once do not saturate the disk. After pt-tools restarts, each job continues from the step where it stopped.

The `pt-tools-transfer-<job number>` tag marks the torrent this job added: when the target downloader holds the same torrent without that tag (for example one you added yourself, even with `pt-tools-transfer`), the job fails and neither copy is touched. When the job ends, qBittorrent deletes this tag (so it does not stay in the tag list) and `pt-tools-transfer` stays; in Transmission the label stays on the torrent.

## Jobs

Torrent transfer → Jobs (任务) lists the latest 200 jobs with their state, check progress and messages. Jobs not yet added to the target downloader can be canceled; Clear finished (清除已结束) only deletes job records and leaves the downloaders alone.

| State                        | Meaning                                                                    |
| ---------------------------- | -------------------------------------------------------------------------- |
| Waiting to export (等待导出) | Just created, no torrent file yet                                          |
| Waiting to add (等待加入)    | Torrent file ready, waiting for the target downloader                      |
| Adding (正在加入)            | Being added to the target downloader paused                                |
| Checking (校验中)            | Added paused, the target is checking the data                              |
| Checked (校验完成)           | Next: start seeding in the target and remove from the source               |
| Done (已完成)                | Seeding in the target, removed from the source                             |
| Rolled back (已回滚)         | Check below 100% or timed out; removed from the target, source left alone  |
| Failed (失败)                | Failed before the torrent was added, for example the site capacity is full |
| Canceled (已取消)            | Canceled before the torrent was added                                      |

## Path mappings

When two downloaders see the same data under different paths (for example two containers mount one disk at different folders), open Path mappings (路径映射), pick the source and target downloaders, add one line per folder, "path prefix in the source downloader → path prefix in the target downloader", and click Save (保存).

- The longest matching prefix wins, compared by whole folders: `/data` does not match `/database`.
- Windows paths (a drive letter or `\\` at the start) match regardless of case and with either `/` or `\`; Linux paths are case-sensitive.
- When one side is a Windows path and the other a Linux path, the separators in the rest of the path follow the target.
- Without a match, the target uses the same path.

## Scheduled rules

Scheduled rules (定时规则) create transfer jobs for matching torrents in the source downloader at an interval. They are off by default. With no conditions, every finished torrent matches:

- category equals, tags contain (case-insensitive);
- site: the site in the pt-tools push record, or the site recognised from the tracker when there is no record, as a site ID such as `hdsky`;
- seeding time of at least so many hours.

Each run takes the oldest torrents first and creates at most Max per run (每轮最多) jobs (10 by default, up to 100); the interval is 60 minutes by default and at least 10. Torrents already in the target downloader, torrents with a job in progress and brush torrents (tagged `pt-tools-brush`) are skipped. Run now (立即运行) runs the rule once; the outcome shows under Last run (上次运行).

## API

| Method and path                                                                                       | Purpose                                                             |
| ----------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------- |
| `POST /api/transfer/preview`                                                                          | Preview `{target_id, items: [{source_id, hash}]}`                   |
| `GET /api/transfer/jobs?status=active\|finished`                                                      | Jobs (latest 200)                                                   |
| `POST /api/transfer/jobs`                                                                             | Create jobs; same body as the preview; returns `{created, skipped}` |
| `POST /api/transfer/jobs/{id}/cancel`                                                                 | Cancel a job not yet added to the target                            |
| `DELETE /api/transfer/jobs`                                                                           | Clear finished job records                                          |
| `GET`, `PUT /api/transfer/path-maps`                                                                  | Read or replace the path mappings of a pair of downloaders          |
| `GET`, `POST /api/transfer/rules`; `GET`, `PUT`, `DELETE /api/transfer/rules/{id}`; `POST …/{id}/run` | Scheduled rules and running one now                                 |

All of them require a signed-in session. A preview or a create request takes at most 500 torrents.
