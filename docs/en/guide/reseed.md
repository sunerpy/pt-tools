# IYUU cross-seeding <Badge type="tip" text="1.0 preview" />

IYUU cross-seeding (IYUU 辅种), under Download (下载), uses [IYUU](https://doc.iyuu.cn/) to find torrents on other sites that carry the same data, and adds them to the same downloader: the data is stored once and seeds on several sites. It is off by default. The page has five tabs, Statistics (统计), Settings (设置), Sites (站点), Rounds (轮次) and Records (记录); once a token is set it opens on Statistics.

## Before you turn it on

- You need an IYUU token: on the IYUU site [iyuu.cn](https://iyuu.cn/), click 开始使用 and sign in by scanning the code with WeChat to get one; see [IYUU's documentation](https://doc.iyuu.cn/) for details.
- When it is on, pt-tools sends the info hashes of the finished torrents in the participating downloaders to IYUU (`2025.iyuu.cn`).
- The token is stored on this machine, encrypted with the same key as the site cookies; the page and the API never show it, only whether it is set.

## Settings

| Setting                               | Meaning                                                                                                                                                                                |
| ------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Turn on (开启辅种)                    | Runs at the interval when on; when off, only Run now (立即运行) runs it                                                                                                                |
| IYUU token                            | Left empty, the saved token stays; Clear (清除) deletes it and turns cross-seeding off                                                                                                 |
| Interval (运行间隔)                   | 1 to 168 hours, 12 by default                                                                                                                                                          |
| Per site per day (每站每天最多)       | Attempts per site per day, 20 by default, up to 500. It counts attempts: failed downloads, failed checks and manual retries all count, and deleting a record does not give one back    |
| Downloaders (参与的下载器)            | None selected means every enabled downloader                                                                                                                                           |
| Gap between requests (两次请求至少隔) | Seconds between two IYUU requests, 3 by default, 0 for no gap, up to 300                                                                                                               |
| Requests per day (每天最多请求)       | IYUU requests per day, 0 (no limit) by default, up to 10000; at the limit the round stops early and the next day carries on from there                                                 |
| Round results (每轮结果)              | Send a notification when a round ends: never (不推送, the default), every round (每一轮都推送), or only when something changed (只在有变化时推送); see [Notifications](#notifications) |
| Channels (接收通道)                   | The notification channels for round results; at least one while notifications are on                                                                                                   |
| In the daily report (写进每日战报)    | Add a line about cross-seeding on the day the report covers; off by default                                                                                                            |

The sites that take part are set with the switches on the Sites tab.

## Sites

The Sites (站点) tab lists the sites IYUU supports and their state in pt-tools: taking part, not configured (pt-tools knows the site but it has no cookie or other credentials yet), not selected, or not supported by pt-tools. pt-tools matches a site by the host name and registrable domain of its address; see [Recognising sites by tracker](downloader-assistant.md#recognising-sites-by-tracker).

Each configured site has a switch on its row, with Turn all on (全部开启) and Turn all off (全部关闭) above the list:

- By default all are on: every site that is configured and supported by IYUU takes part, including sites configured later.
- Once you turn a site off, only the sites whose switch is on take part, and sites configured later are off until you turn them on here. Turn all on goes back to every site.
- After Turn all off no site takes part, and a round stops with no sites to cross-seed (没有可以辅种的站点).
- A site that is not configured cannot be turned on; configure it in the site list first.

## What a run does

1. Reads the finished torrents in the participating downloaders (except those in error or being checked) and sends their info hashes to IYUU in batches of 200.
2. For each torrent IYUU finds, skips it when its site is not taking part, the downloader already has it, this torrent on this site was tried before, or the site has reached today's limit.
3. Downloads the torrent file from the site (with the site's rate limit), checks that its info hash matches IYUU's, and checks that its file list is exactly the same as the original torrent's in the downloader (every file with the same path and size). A mismatch is recorded as failed and not tried again; a torrent that cannot be downloaded (the site is unavailable, for example), or whose original torrent's file list cannot be read (the downloader is briefly unreachable, for example), is also recorded as failed and tried again after 7 days. A torrent the site has deleted (IYUU's index still lists it; on download M-Team answers 種子未找到, or a NexusPHP site's details page says No torrent with this ID) is recorded as Deleted on site (站点已删除), does not count as failed, and is not tried again.
4. Adds a torrent that passes the checks to the same downloader, paused, with the original torrent's save path and the tags site ID, `pt-tools-reseed` and `pt-tools-reseed-<job number>`, which belongs to this job only (when the job ends qBittorrent deletes it; in Transmission it stays on the torrent).
5. Once the downloader checks it at 100%, the torrent starts seeding and pt-tools records it (site seeding capacity and auto-delete both see it); below 100% it is removed from the downloader with the data kept, and the original torrent is not affected.

Steps 4 and 5 use the same jobs as [Torrent transfer](torrent-transfer.md): each downloader checks one torrent at a time, and after pt-tools restarts a job continues from the step where it stopped. Cross-seeding never removes an existing torrent.

A round runs for at most 30 minutes (time spent paused does not count) and stops early when the time is up. When the token is cleared or replaced during a run, the run stops before it queries the next batch and does not send info hashes with the old token.

## Rounds

The Rounds (轮次) tab keeps one entry per round (scheduled, Run now, continued or retry) with how it started, its result, how long it took and its counts; it keeps the latest 500, for at most 180 days. Open a round to see each step (the downloaders read, the batches queried, what was added, failed, skipped, and why it stopped) and every attempt in it. A round left unfinished when pt-tools exited is marked interrupted (被打断) at the next start.

While a round runs, the page header has Pause (暂停) and Stop (停止):

- Pause: the round stops after the torrent it is downloading and waits until you click Continue (继续); time spent paused does not count towards the 30 minutes. A pause longer than 24 hours stops the round, so a forgotten pause does not hold up the scheduled rounds.
- Stop: the torrent being downloaded is interrupted, is not recorded as failed, and is tried again as usual later; cross-seeding jobs already created are added and checked as usual. With Also cancel the jobs this round created that are not in the downloader yet (同时取消这一轮建的、尚未加入下载器的任务) ticked, those jobs are cancelled when the round ends.
- Continue: a round that stopped early (stopped, out of time, rate-limited by IYUU, or unfinished when pt-tools exited) can be continued from the Rounds tab, which queries only the part it had not finished; only the latest round can be continued. The next scheduled round also carries on from there, and the round after it reaches the end starts from the beginning again.

## IYUU requests and rate limits

IYUU recommends cross-seeding once a day and rejects several runs in a short time (辅种太频繁). Every IYUU request (reading the site list, reporting sites and queries, including the site list read by the Sites tab) follows these rules:

- Two requests are at least the gap between requests apart (3 seconds by default).
- Once today's requests reach the requests per day, no more are sent; the round stops early and the next day carries on from there.
- When IYUU answers too many requests (HTTP 429), pt-tools cools down until the later of its `Retry-After` and `X-RateLimit-Reset`, for at most 24 hours. A cooldown of up to a minute is waited out before querying again; a longer one stops the round. During a cooldown no scheduled round runs, and Run now and Continue are refused with the time the cooldown ends; replacing the token ends the cooldown.

The run status on the Settings tab shows how many requests each endpoint got today, until when the cooldown lasts and the last rate limit; each round on the Rounds tab shows its own request count.

## Deleting torrents and shared data

A cross-seed and its original torrent use the same files. When auto-delete (emergency cleanup and deleting torrents still unfinished when the free period ends included), the seeding competition monitor (做种竞争度监控), brushing removal or Delete dead torrents (删除失效种子) in the downloader assistant is about to delete data, pt-tools first checks whether another torrent in the downloader still uses the same data: if one does, only the torrent is removed and the data stays; when all the torrents sharing the data are deleted together, the data is deleted as usual. pt-tools compares the paths where the torrents keep their data: the same path counts as shared (for a qBittorrent multi-file torrent without a subfolder it compares the files).

Deleting a torrent by hand, in the downloader or in a pt-tools torrent list, with its data selected for deletion deletes the files, and the torrents sharing them lose their data too.

## Records

The Records (记录) tab lists every record by the time of its last attempt, newest first, 20 per page: torrent, site and torrent ID, result, reason and time (a retry after 7 days updates the original record). Filter by result or site, or search by torrent name or info hash.

The result is the progress of the job (waiting to add, checking and so on), or cross-seeded (已辅种), withdrawn (已撤回, the check stopped short of 100% and the torrent was removed), failed (失败, the download or a check failed), cancelled (已取消), deleted on site (站点已删除, the site no longer has the torrent), or job deleted (任务已删除, the record's job was deleted some other way). Clearing finished cross-seeding jobs writes each job's result into its record first, so the result in the record stays.

- Retry (重试): records that failed, were deleted on site, withdrawn, cancelled or whose job was deleted can be retried, up to 100 at a time. A retry runs a round in the background (shown as a retry on the Rounds tab) that does not query IYUU: it downloads and checks again with the site, torrent ID and info hash in the record. It ignores the site switches and whether the torrent was tried before, but it uses the per site per day allowance and skips a torrent once the allowance is used up; it also skips a torrent whose original torrent is no longer in the downloader. Retry rounds do not change Last run (上次运行) or the scheduled interval.
- Delete (删除): records whose job has not finished cannot be deleted; up to 500 at a time. After deleting, those torrents are tried again in a later round; today's used allowance and the statistics stay as they are.
- Cancel (取消): a job still waiting to be added to the downloader can be cancelled; one already added cannot.

The summary of each round also shows as Last run (上次运行) on the Settings tab, next to Next run (下次运行), roughly when the next round is (with cross-seeding on and a token set, by the interval; once it's due, it runs at the next check).

## Statistics

The Statistics (统计) tab covers a day, week, month, quarter or year range, or a custom one:

| Range   | Covers                                                                                           |
| ------- | ------------------------------------------------------------------------------------------------ |
| Day     | Last 30 days                                                                                     |
| Week    | Last 12 weeks                                                                                    |
| Month   | Last 12 months                                                                                   |
| Quarter | Last 8 quarters                                                                                  |
| Year    | Last 5 years                                                                                     |
| Custom  | A start and end date, up to 5 years: by day up to 62 days, by week up to a year, by month beyond |

- The number band at the top: succeeded, failed (withdrawn included), deleted on site, jobs created, skipped and IYUU requests; the small bars in each cell are the last 7 periods.
- The bar chart draws the cross-seeds that succeeded in each period, and its readout shows the period's created, failed, deleted on site, skipped and IYUU requests; Details (明细) lists every period as a table, newest first.
- By site (按站点) lists each site's created, succeeded, failed, deleted on site and skipped; Why skipped (跳过的原因) separates already in the downloader, tried before, the site's limit reached today, the site not taking part, and a job already in progress; IYUU requests (IYUU 请求) shows today's, the range's and the latest request.

Created, failed, deleted on site and skipped count on the day the attempt started; succeeded and withdrawn count on the day the job finished. Periods follow the server's time zone and weeks start on Monday. Every round checks again, so the same torrent counts as skipped once in every round. On upgrading to this version, earlier cross-seeding records are added to the statistics approximately, by the time of the record; skips and IYUU requests are counted from the upgrade on. The attempt details are kept for 5 years.

## Notifications

With Round results set to every round or only when something changed, a notification goes to the chosen channels when each round ends. The title names the round, its result and how it started (for example 辅种：第 12 轮完成（定时）), and the body gives the numbers queried, found, added, failed, deleted on site and skipped, the time taken and the IYUU request count; a round that stopped early also says it can be continued. The body has only counts and status, never torrent names.

- Something changed means the round added cross-seeds, had failures, or did not finish (partly failed, stopped early, stopped, or could not start).
- A round interrupted because pt-tools exited sends nothing. Retry rounds follow the same setting.
- Each round sends once; a channel in its quiet hours gets it when they end; every delivery is recorded in the [audit log](chatops-quickstart.md#viewing-the-audit-log) (辅种结果).
- When a chosen channel is disabled or deleted, the notification is not redirected to another channel; on save, every chosen channel must still exist.

In message templates and custom webhooks this event is `reseed.round` (辅种结果); see [Message templates](notify-channels.md#message-templates) for its variables.

With In the daily report on, the [daily report](user-stats.md) gets the line 🌱 昨日辅种：建成 x · 成功 y · 失败 z · 站点已删除 w (created, succeeded, failed, deleted on site; 今日辅种 when the report covers today), counted for the day the report covers and the same way as the statistics (failed includes withdrawn; skips are left out); the line is left out on days without these. The daily report itself has to be turned on in Global settings (全局设置) first.

## Upgrading and rolling back

A few things behave differently after upgrading to this version: the sites taking part are set with the switches on the Sites tab; the daily allowance counts attempts (deleting records no longer frees allowance); a torrent being downloaded when a round is stopped, runs out of time or pt-tools exits is no longer recorded as failed; a round that stopped early is carried on by the next round; during an IYUU cooldown no scheduled round runs and manual runs are refused.

Rolling back to an older version removes these features, and cross-seeding runs the old way (by the list of site names). Older versions treat an empty site list as every site, so rolling back after Turn all off would cross-seed every site: before rolling back, turn on the sites you want on the Sites tab.

## API

| Method and path                              | Purpose                                                                                                                                                                                                                                        |
| -------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `GET`, `PUT /api/reseed/settings`            | Read or save the settings; unknown fields are rejected. Without `token` the saved one stays and `""` clears it; without `site_names`, `iyuu_min_interval_seconds`, `notify_mode`, `notify_channel_ids` or `daily_report` the saved value stays |
| `GET /api/reseed/sites`                      | IYUU sites and the matching pt-tools sites                                                                                                                                                                                                     |
| `PUT /api/reseed/sites`                      | `{"selected": true}` turns every site on, `false` turns every site off                                                                                                                                                                         |
| `PUT /api/reseed/sites/{site}`               | `{"selected": true}` turns one site on, `false` turns it off                                                                                                                                                                                   |
| `POST /api/reseed/run`                       | Start a round from the beginning in the background: 202 with the round number; 409 while a round is in progress; 429 during an IYUU cooldown or at the daily request limit                                                                     |
| `GET /api/reseed/runs`                       | Rounds, newest first (`page`, `page_size`)                                                                                                                                                                                                     |
| `GET /api/reseed/runs/{id}`                  | A round with its steps; `GET …/attempts` lists its attempts                                                                                                                                                                                    |
| `POST /api/reseed/runs/{id}/pause`, `resume` | Pause or continue the round in progress                                                                                                                                                                                                        |
| `POST /api/reseed/runs/{id}/stop`            | Stop the round in progress; with `{"cancel_pending": true}` its jobs not yet in the downloader are cancelled too                                                                                                                               |
| `POST /api/reseed/runs/{id}/continue`        | Carry on from the latest round that stopped early: 202 with the new round number                                                                                                                                                               |
| `GET /api/reseed/records`                    | Records (by the time of the last attempt, newest first) with their job states; `result`, `site`, `q`, `page`, `page_size`                                                                                                                      |
| `POST /api/reseed/records/retry`, `delete`   | `{"ids": [...]}` retries the chosen records in the background (202 with the round number) or deletes them                                                                                                                                      |
| `POST /api/reseed/records/{id}/cancel`       | Cancel the record's job while it is still waiting to be added to the downloader                                                                                                                                                                |
| `GET /api/reseed/stats`                      | Statistics: `granularity` (day, week, month, quarter, year) with `periods`, or a custom `from` and `to` (YYYY-MM-DD, both included)                                                                                                            |
| `GET`, `DELETE /api/reseed/jobs`             | Cross-seeding jobs; DELETE clears finished jobs after writing their results into the records                                                                                                                                                   |

All of them require a signed-in session.
