# Seed boosting (brush tasks)

A brush task picks fresh torrents from one site's free-torrent list, pushes them to a downloader, and removes them by rule after they have seeded for a while, so you build up upload and bonus. It runs separately from RSS subscriptions; turning a brush task on does not change how RSS works.

Brushing adds requests to the site and keeps torrents moving in and out of your downloader. Before you turn a task on, make sure the site's rules allow it, and check the site's requirements for new-torrent download speed, H&R, and seeding time.

## Create a task

Open **Download → Brush tasks** in the navigation and click **New task**. One task covers one site and one downloader:

| Setting             | Meaning                                                                                           |
| ------------------- | ------------------------------------------------------------------------------------------------- |
| Site                | Built-in sites only. Configure the site's credentials and enable it under **Sites** first         |
| Downloader          | Where torrents are pushed. An empty save path or category uses the downloader's default           |
| Extra tags          | Optional. The site name, `pt-tools-brush`, and the task tag `pt-brush-<task id>` are always added |
| Check interval      | How often a round runs. At least 5 minutes, 10 by default                                         |
| Enable after saving | New tasks are off by default; turn the task on once you have checked the settings                 |

Each round of an enabled task does three things in order: samples the task's torrents (records upload and download), removes torrents by the removal rules, and adds new torrents within the limits.

## Entry conditions

Each round requests the site's torrent list once (subject to the site's own rate limit) and checks every item locally:

- **Discount types**: "Free" and "2x free" by default; you can add 50%, 2x 50%, and others. Some sites (such as M-Team) mix normal torrents into the list; they are filtered out here as well.
- **Discount remaining**: How long the discount must still last, at least 60 minutes by default. Permanent free torrents without an end time are not limited.
- **Size**: Minimum and maximum size in GB; 0 means no limit. When a size limit is set, torrents whose size the list does not show are skipped.
- **At most N seeders** and **at least N leechers**: pick busy torrents with little competition.
- **Published within**: only take recent torrents; 0 means no limit.
- **Title includes / excludes**: one per line or comma separated, case-insensitive, matched against the title and subtitle.
- **Exclude H&R**: on by default. When the site has site-wide H&R enabled, every torrent from it counts as H&R (the same rule RSS downloads use). With the option off, only H&R torrents whose required seeding time the site definition can compute are taken: without a known requirement, the removal rules cannot tell when a torrent may go.

Matching torrents are added in order of most leechers, then fewest seeders. A torrent any brush task has pushed from this site before (including ones already removed) is never added again, so two tasks never add the same torrent; torrents already in the downloader are skipped too.

## Limits

| Limit                | Meaning                                                                 |
| -------------------- | ----------------------------------------------------------------------- |
| Concurrent downloads | Maximum number of the task's torrents still downloading, 1–50, required |
| Total size           | Maximum combined size of the task's active torrents; 0 means no limit   |
| Added per day        | Maximum combined size of torrents added today; 0 means no limit         |

Pushes go through the same entry point as RSS downloads, so the disk-space protection and per-site seeding capacity checks still apply. When either check rejects a torrent, the round stops adding and tries again next round. The round also stops when three torrent files in a row fail to download, or when a torrent reaches the downloader but its local record cannot be written (that torrent is removed again).

## Removal rules

A torrent is removed as soon as any rule matches. By default its data is deleted too (you can switch to removing the torrent from the downloader only); when another torrent still uses the data (a [cross-seed](reseed.md) of it, for example), only the torrent is removed and the data stays:

- **Seeded for N hours**: counted only for completed torrents.
- **Ratio reaches N**.
- **Average upload below N KB/s**: measured over the last M minutes (30 by default), and only for torrents that are complete, have seeded for at least M minutes, and have samples covering that period.
- **No activity for N hours**: neither upload nor download grew in that time.
- **Free period ending before download completes**: on by default. If the discount ends before the next check, the torrent is removed in this round, so nothing is downloaded after the free period at the normal rate.

H&R torrents are never removed before they reach the site's required seeding time, including when the free period ends before they finish downloading.

A download counts as complete when it is at 100% and not in a checking or error state; torrents being checked or with missing files are not complete.

Brushing only touches torrents that carry the task's tag and were added by the task. If you remove the task tag from a torrent, it is handed back to you and brushing stops removing it; torrents removed by hand or by other means are marked "no longer in the downloader". After you switch a task to another downloader, the torrents it already added are still sampled and removed in their original downloader; if that downloader is temporarily unreachable, those torrents are left alone for the round.

## Relation to auto cleanup

All brush torrents carry the `pt-tools-brush` tag. The regular **Auto cleanup** rules (seeding time, ratio, and so on) skip them and leave them to the task's own removal rules, so the two sets of rules never compete. The emergency cleanup that runs when free disk space drops below the minimum still covers every torrent, brush torrents included.

## Turning off and deleting a task

- **Turn off**: no new torrents are added. Torrents already added keep being handled by the removal rules until they are all gone.
- **Delete**: the task, its statistics, and its torrent records are deleted. The torrents stay in the downloader and are no longer managed by brushing (the regular auto cleanup rules still skip them; handle them by hand if needed).
- A task cannot be deleted or edited while a round is running; try again shortly (a round uses the settings read when it started).

## Earnings and the daily report

The top of the brush page shows active torrents, today's upload and download, and the overall ratio; the **30-day earnings** card plots the daily upload of all tasks. Earnings are the difference between two consecutive samples of each brush torrent's total upload and download in the downloader, grouped by day in the server time zone; when two samples straddle midnight, the increment is split between the two days by duration.

With the [daily report](user-stats.md#daily-report) on, a report sent on a day with brush earnings gains a "Brush today" section: total upload, download, added, and removed, plus one line per task when there is more than one.

## Migrating from Vertex

pt-tools brushing follows the common Vertex setups. The concepts map roughly like this:

| Vertex                                   | pt-tools                                                                                                                         |
| ---------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| Brush task (RSS task)                    | Brush task: one task covers one site and one downloader; torrents come from the site's free-torrent list rather than an RSS link |
| Selection rules                          | Entry conditions: discount type, discount remaining, size, seeders / leechers, age, title includes and excludes, exclude H&R     |
| Removal rules                            | Removal rules: seeding time, ratio, average upload speed, inactivity, free period ending before completion; any one rule removes |
| Downloader limits (max downloads, space) | Limits: concurrent downloads, total size, added per day; disk protection and site seeding capacity come from the global settings |
| Tags / category                          | Extra tags and category; `pt-tools-brush` and the task tag are added automatically                                               |

pt-tools does not support Vertex's custom script rules. For more complex conditions, split them into several tasks with different conditions.

## API

| Endpoint                                                         | Purpose                                                                                                              |
| ---------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| `GET` / `POST /api/brush/tasks`                                  | List tasks (with active torrents and earnings) / create a task                                                       |
| `GET` / `PUT` / `DELETE /api/brush/tasks/<id>`                   | Read, update, or delete a task                                                                                       |
| `POST /api/brush/tasks/<id>/run`                                 | Run one round now and return its result; returns 502 when the site or downloader failed and the round did not finish |
| `GET /api/brush/tasks/<id>/torrents?state=active\|removed\|gone` | The task's torrents                                                                                                  |
| `GET /api/brush/stats?days=<1–90>`                               | Daily earnings per task and in total for recent days                                                                 |

All of these require a logged-in session.
