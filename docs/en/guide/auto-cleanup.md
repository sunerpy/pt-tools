# Auto-delete and disk protection

pt-tools can delete torrents automatically. This manages disk space over long periods of ratio building and keeps the disk from filling up.

## Overview

Auto-delete checks the torrents in your downloaders on a schedule and, following the rules you set, deletes the torrents that meet them, together with their data files, to free disk space.

**Key features**:

- **Safety first**: by default only the torrents pt-tools pushed itself are managed; torrents you added by hand are left alone
- **H&R protection**: torrents with an H&R requirement are recognised and not deleted before they have seeded long enough
- **Flexible conditions**: seeding time, ratio, inactivity and other conditions, combined with AND or OR
- **A floor for disk space**: disk space protection (on by default) blocks new pushes when space runs low and clears existing torrents in order of priority
- **Push blocking**: RSS downloads and manual pushes stop automatically when free space falls below the threshold, so the disk does not fill up
- **Strict scope**: cleanup always respects the management scope; even an emergency cleanup never deletes torrents outside it
- **Safe with several downloaders**: each downloader is managed on its own, with torrents matched exactly through database records

## Settings

Configure it on the Rules → Auto cleanup (规则 → 自动清理) page of the web interface. The other options appear once you turn on Enable auto-delete (启用自动删种) in the Auto-delete (自动删种) card.

### Basic settings

| Setting                                                | Default    | Meaning                                                        |
| ------------------------------------------------------ | ---------- | -------------------------------------------------------------- |
| **Enable auto-delete (启用自动删种)**                  | Off        | The main switch                                                |
| **Check interval (检查间隔（分钟）)**                  | 30 minutes | How often the cleanup runs; at least 5 minutes                 |
| **Delete the data files too (删除时连数据文件一起删)** | On         | Whether a deleted torrent's downloaded data is deleted as well |

When another torrent in the downloader still uses the data (a torrent added by [IYUU cross-seeding](reseed.md) uses the same files as the original torrent, for example), only the torrent is deleted and the data stays, even with Delete the data files too turned on, so that the other torrent keeps its data. pt-tools compares the paths where the torrents keep their data: the same path counts as shared (for a qBittorrent multi-file torrent without a subfolder it compares the files, and only a common file counts). When all the torrents sharing the data are deleted together, the data is deleted as usual.

### Management scope

Three modes decide which torrents auto-delete manages:

| Mode                                                                | Meaning                                                                                              | Safety                                                          |
| ------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- | --------------------------------------------------------------- |
| **Torrents pushed by pt-tools only (仅本应用推送的种子)** (default) | Matched exactly through database records: only the torrents pt-tools pushed and recorded are managed | ⭐⭐⭐ Safest                                                   |
| **Match by tag (按标签匹配)**                                       | Every torrent in the downloader that has the given tags                                              | ⚠ Torrents you added by hand with the same tags are managed too |
| **All torrents in the downloader (下载器中所有种子)**               | Every torrent in the downloader                                                                      | ⚠⚠ Includes torrents pt-tools did not add                       |

> **Recommendation**: always use Torrents pushed by pt-tools only. Use the other modes only once you fully understand the risk.

**The scope guarantee**: whichever scope you choose, auto-delete, emergency cleanup included, **strictly respects** it and never deletes torrents outside it. With Torrents pushed by pt-tools only, for example, even an emergency cleanup triggered by a nearly full disk will never delete a torrent you added to the downloader by hand.

### Deletion conditions

A torrent that meets the configured conditions is marked for deletion.

| Condition                                                                                 | Meaning                                                                               | Example  |
| ----------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------- | -------- |
| **Seeding time over (做种时间超过（小时）)**                                              | Can be deleted after seeding for N hours                                              | 72 hours |
| **Ratio reached (分享率达到)**                                                            | Can be deleted once the ratio is ≥ N                                                  | 2.0      |
| **Inactive for over (不活跃超过（小时）)**                                                | Can be deleted once it has seeded for over N hours and is not uploading at the moment | 48 hours |
| **Delete torrents still unfinished when freeleech ends (免费到期仍未下完的种子自动删除)** | Torrents whose free period has ended before they finished downloading                 | On/Off   |

**Condition logic (条件关系)**:

- **OR mode (任一满足即删（OR）)**, the default: a torrent is deleted when any one condition is met. Good for freeing space quickly.
- **AND mode (全部满足才删（AND）)**: every configured condition has to be met at the same time. More conservative, for getting the most out of each torrent.

Delete torrents still unfinished when freeleech ends is checked on its own, whatever the condition logic: when it is on, a torrent whose free period has passed before it finished is deleted even in AND mode.

> A condition set to 0 is ignored. With the seeding time at 72 hours and the ratio at 0, for example, only the seeding time counts.

### Disk space protection

| Setting                                             | Default | Meaning                                            |
| --------------------------------------------------- | ------- | -------------------------------------------------- |
| **Enable disk space protection (启用磁盘空间保护)** | On      |                                                    |
| **Minimum free space (最低剩余空间（GB）)**         | 50 GB   | Protection starts when free space falls below this |

Disk space protection works in **two ways**:

1. **Push blocking** (at the entrance): when an RSS download or a manual push finds that the downloader's free space is below the threshold, the push is skipped and no more torrents are added to the downloader. As soon as a batch finds that space is short, it stops processing the remaining torrents.
2. **Emergency cleanup** (existing torrents): when the scheduled auto-delete check finds free space below the threshold, it force-deletes torrents within the management scope in order of a combined score, highest first, even if they do not meet the deletion conditions above. Each of these raises the score:
   - paused
   - a high ratio
   - a long seeding time
   - inactive (not uploading at the moment)
   - a large size (frees the most space)

> **Note**: emergency cleanup strictly respects the management scope. If the deletable torrents within the scope cannot free enough space, nothing outside the scope is deleted; push blocking then keeps new torrents out, so the disk fills no further.

Emergency cleanup deletes extra torrents only when deleting the data files is turned on: removing a task from the downloader alone frees no disk space. When working out how much space is still missing, it counts the torrents the deletion conditions already selected, so it does not delete more than needed. Deleting one of several torrents that share data frees no space: when all of them can be deleted, emergency cleanup picks them together and counts the space once; when one of them is protected or outside the managed scope, it picks none of them.

### Protection rules

These torrents are not deleted, even when they meet the deletion conditions:

| Rule                                                | Default  | Meaning                                                                                        |
| --------------------------------------------------- | -------- | ---------------------------------------------------------------------------------------------- |
| **Protect downloading torrents (保护下载中的种子)** | Off      | Torrents that are downloading or being checked are not deleted                                 |
| **Protect H&R torrents (保护 H&R 种子)**            | On       | Torrents that have not yet met the site's H&R seeding time are not deleted                     |
| **Minimum keep time (最短保种时间（小时）)**        | 24 hours | Torrents added less than N hours ago are not deleted                                           |
| **Protected tags (保护标签)**                       | Empty    | Torrents with these tags are not deleted (type a tag and press Enter; add as many as you like) |

## How it runs

### The RSS push flow (at the entrance)

```
1. The scheduled RSS task fires and reads the feed for its list of torrents
2. The .torrent files are downloaded to the local disk
3. For each torrent file waiting to be pushed:
   a. check whether it has expired, was already pushed or already exists
   b. check disk space (when protection is on):
      - enough space → push the torrent to the downloader
      - not enough space → skip this torrent and stop processing the rest
```

### The auto-delete flow (existing torrents)

```
1. Go through every configured downloader
2. Get the downloader's list of torrents
3. Keep the "manageable" torrents according to the management scope (strictly)
4. Leave out protected torrents (downloading / H&R / protected tags / minimum keep time)
5. Evaluate the deletion conditions (in AND or OR mode)
6. Check disk space (when protection is on):
   - enough space → delete only the torrents that meet the conditions
   - not enough space → force-delete within the scope by score until space recovers
7. Delete, update the database and write the log
```

## H&R protection

H&R (Hit and Run) is a rule on some private trackers: after downloading a torrent you have to seed it for a certain time, or you are penalised.

**How it works**:

1. When pt-tools downloads a torrent, it records the H&R flag and the required seeding time that the site returns
2. Auto-delete checks the H&R information in the database. For sites whose built-in definition declares H&R rules, the required seeding time is worked out from those rules even when a torrent has no H&R flag of its own
3. A torrent with an H&R requirement that has not yet seeded long enough is protected and not deleted
4. Sites and torrents without an H&R requirement are not affected

> **Note**: not every site provides H&R information. When a site returns no H&R flag and its built-in definition has no H&R rules either, pt-tools cannot recognise H&R torrents. Set a reasonable minimum keep time as an extra safeguard.

## Several downloaders

With several downloaders configured:

- each downloader is **handled separately**, without affecting the others
- with Torrents pushed by pt-tools only, each torrent is matched exactly to its downloader through the `downloader_name` field in the database
- a torrent of downloader A is never treated as one of downloader B

## Example configurations

### Scenario 1: everyday ratio building (recommended)

A conservative setup that suits most people:

| Setting                                                                  | Value                            |
| ------------------------------------------------------------------------ | -------------------------------- |
| Management scope (管理范围)                                              | Torrents pushed by pt-tools only |
| Condition logic (条件关系)                                               | OR                               |
| Seeding time (做种时间超过（小时）)                                      | 72 hours                         |
| Ratio (分享率达到)                                                       | 2.0                              |
| Delete when unfinished at freeleech end (免费到期仍未下完的种子自动删除) | On                               |
| Protect H&R (保护 H&R 种子)                                              | On                               |
| Minimum keep time (最短保种时间（小时）)                                 | 24 hours                         |
| Disk space protection (启用磁盘空间保护)                                 | On, at least 50 GB               |

### Scenario 2: little disk space

Keeping the disk from filling up comes first:

| Setting                            | Value                            |
| ---------------------------------- | -------------------------------- |
| Management scope                   | Torrents pushed by pt-tools only |
| Condition logic                    | OR                               |
| Seeding time                       | 48 hours                         |
| Ratio                              | 1.0                              |
| Inactive time (不活跃超过（小时）) | 24 hours                         |
| Disk space protection              | On, at least 50 GB               |

### Scenario 3: aiming for a high ratio

Torrents are deleted only once they have been put to full use:

| Setting               | Value                            |
| --------------------- | -------------------------------- |
| Management scope      | Torrents pushed by pt-tools only |
| Condition logic       | AND                              |
| Seeding time          | 168 hours (7 days)               |
| Ratio                 | 3.0                              |
| Protect H&R           | On                               |
| Minimum keep time     | 48 hours                         |
| Disk space protection | On, at least 100 GB              |

> In AND mode, a torrent is deleted only when it has seeded for more than 7 days **and** reached a ratio of 3.0.
> Combine it with disk space protection and a generous free-space floor, so that a high ratio target does not let torrents pile up until the disk is full.

## Staged torrent file cleanup (intermediate .torrent files)

> **Not to be confused**: this section is about the intermediate `.torrent` files in pt-tools' local **staging folder**, **not** the seeding tasks in qBittorrent that auto-delete above manages. The two are completely separate cleanup mechanisms.

### What is cleaned

After RSS downloads a torrent, pt-tools first writes the `.torrent` file to a local staging folder (`{torrent file folder}/{tag}/`, with file names like `siteName-torrentID.torrent`), then pushes it to the downloader. Over a long run, when some torrents keep failing to push, are skipped, or belong to an RSS subscription that is no longer processed, these staged files pile up and take disk space ([issue #450](https://github.com/sunerpy/pt-tools/issues/450)).

The cleanup described here deals **only with these locally staged `.torrent` files** and never touches any data in the downloader.

### When it runs

It runs once at the end of every RSS push round for each site, and **does not need auto-delete's main switch**. It runs whether the round processed every torrent, stopped part-way because of disk protection, or skipped some torrents for being too large (a `defer` in the code guarantees this).

### How files are judged

Each of the site's `.torrent` files in the staging folder is checked in turn and deleted when any of these applies:

| Condition                                                                     | Meaning                                                  |
| ----------------------------------------------------------------------------- | -------------------------------------------------------- |
| No matching record in the database                                            | An orphaned file; deleted                                |
| Marked in the database as pushed successfully                                 | A leftover copy after the push; deleted                  |
| The retry count has reached the maximum number of retries                     | Considered unrecoverable; deleted                        |
| Not pushed, and the file was last modified longer ago than the retention time | Stuck waiting to be pushed; treated as stale and deleted |
| Not pushed, and the file is recent (still within the retention time)          | Still in the normal retry window; kept                   |

**Why the file's mtime and not the last check time in the database**: every RSS round refreshes the torrents' last check time in the database. Judging staleness by that field would mean that a torrent which is "seen every round but never pushed" never goes stale, and that was the root cause of the files piling up. With the file system's modification time instead, old files that have been piling up for a long time are recognised and removed in the next round.

### Settings

It reuses the existing **Staged torrent retention in hours (暂存种子保留时长（小时）)** setting under Global settings → Staged torrent cleanup (全局设置 → 暂存种子清理) (`RetainHours`, 24 hours by default) as the threshold for stale files, so nothing else needs to be set.

- With the retention time at `0`, staged file cleanup is **off**, which keeps the behaviour of earlier versions.
- With any other value, a file last modified before "now minus the retention time" is treated as stale.

### Limits and safety

- Staged file cleanup **only deletes local intermediate `.torrent` files** and never deletes data that is downloaded or seeding in the downloader. Even when a file is marked as pushed in the database, only this local staged copy is deleted; the task in the downloader is not affected.
- **Known limitation**: staged file cleanup runs as part of the RSS push flow. When a site's RSS subscription has been deleted or disabled, no task triggers a cleanup of the `.torrent` files left in its folder any more. Delete them by hand, or add the subscription again to trigger one cleanup.

### Skipping oversized torrents

Issue #450 also fixed "an oversized torrent that trips disk protection blocks the pushes after it": an oversized torrent is now skipped and the remaining torrents are still processed (the reason is recorded in the torrent's `last_error`), so the whole round is no longer interrupted. The skipped torrent's `.torrent` stays in the staging folder to be retried once disk space recovers, and is eventually removed by the same mtime staleness rule.

## Cleaning the whole work folder (.pt-tools)

> **How this differs from the two sections above**: auto-delete above manages **seeding tasks** in the downloader; staged file cleanup runs in a single site's RSS push flow and only handles the tag folders of **current subscriptions**. The work folder cleanup in this section is a **separate, on-demand** mechanism: in one go it clears the **logs, staged torrents and old backups** that pile up in the whole `~/.pt-tools` work folder, and it scans **every** tag subfolder of `downloads`, whether or not an RSS subscription still uses it. That closes the gap where folders left behind by a deleted subscription could not be reached.

### What is cleaned

The work folder cleanup works only inside **three fixed allow-listed folders** of the `~/.pt-tools` work folder:

| Category  | Folder                  | What is cleaned                                                                                                                              |
| --------- | ----------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| `logs`    | `~/.pt-tools/logs`      | Rotated log **backups** (`all-*.log`, `debug-*.log.gz` and so on), with the same retention as log rotation (number of backups and days)      |
| `staging` | `~/.pt-tools/downloads` | `.torrent` files that are no longer needed, in **every** tag subfolder (pushed, orphaned, at the maximum retries or past the retention time) |
| `backups` | `~/.pt-tools/backups`   | Old configuration backups `*.json`: the **most recent N** by modification time are kept (5 by default) and the rest deleted                  |

The `staging` category judges files by the same rules as staged file cleanup (it reuses `RetainHours` and `MaxRetry`; with `RetainHours<=0` it uses the default of 24 hours instead, and the category stays on). The difference is that the work folder cleanup scans **every** tag subfolder under the allow-listed root `~/.pt-tools/downloads`, not only those of current RSS subscriptions.

### Red lines: files that are never deleted

These files are **hard-coded red lines**: whatever the category and whatever the options, they are **never** deleted:

- **The database**: `torrents.db` and its `torrents.db-wal` and `torrents.db-shm`
- **The encryption key**: `secret.key` (losing it makes every site cookie impossible to decrypt)
- **The log files being written**: `all.log`, `debug.log`, `info.log` and `error.log` (only the rotated backups are deleted, never the files being written)

There is also a **folder red line**: if you set Torrent file folder (种子下载目录, `DownloadDir`) in Global settings (全局设置) to an absolute path **outside** `~/.pt-tools` (a mounted data disk, for example), the work folder cleanup **never touches that folder**. The scan root of `staging` is **fixed** at `~/.pt-tools/downloads`; the `DownloadDir` setting is never read to widen the cleanup.

### Two ways to run it

#### Command line: `pt-tools clean`

| Option           | Default | Meaning                                                                                              |
| ---------------- | ------- | ---------------------------------------------------------------------------------------------------- |
| `--dry-run`      | `true`  | Preview mode: lists the files that **would be deleted** without deleting them                        |
| `--confirm`      | `false` | Confirms a real deletion (destructive; **without it the command only previews and deletes nothing**) |
| `--category`     | Empty   | Limits the cleanup to some of `logs,staging,backups`, separated by commas; empty means all three     |
| `--keep-backups` | `5`     | How many of the most recent backups the `backups` category keeps                                     |

```bash
# Preview what would be cleaned (the default; safe, deletes nothing)
pt-tools clean

# Preview the logs category only
pt-tools clean --category logs

# Delete for real (destructive; needs an explicit confirmation)
pt-tools clean --confirm

# Delete for real and keep the 10 most recent backups
pt-tools clean --confirm --keep-backups 10
```

> If you ask for a real deletion explicitly (`--dry-run=false`) **without** `--confirm`, the command refuses to run and asks for `--confirm`, so that nothing is deleted by mistake.

#### Web interface: Clean the work folder on `/cleanup`

Use the Clean the work folder (清理工作目录) card, marked `.pt-tools`, at the bottom of the Rules → Auto cleanup (规则 → 自动清理) page:

1. Tick the **items to clean (清理项)**: Log files (日志文件), Staged torrent files (暂存种子文件) and Old configuration backups (旧配置备份); all of them are ticked by default
2. Set Backups to keep (保留最近备份数量): 5 by default; `0` also keeps 5
3. Click **Preview (预览可清理项)** → a table shows how many files each category **could delete** and how much **space it would free**, and marks protected or skipped items (**nothing is deleted** at this point; the preview always covers all three categories and assumes that 5 backups are kept)
4. Click **Clean now (立即清理)** → a confirmation dialog appears, stating that protected items such as the database and the key are not deleted and that the operation cannot be undone → the files are deleted only after you confirm, and the result is shown

### Safety measures

The work folder cleanup runs in the safest way by default, and every deletion has to pass several checks:

- **Nothing is deleted by default**: the CLI defaults to `--dry-run` (preview only); the web interface puts the preview first: you can preview first, and a real deletion needs an explicit click on Clean now and a second confirmation.
- **Allow-list check**: every candidate path is first resolved with `EvalSymlinks`, then checked with `filepath.Rel` to make sure it is still inside one of the three allow-listed roots. Anything that escapes (through `..` or an absolute path) is refused and logged, never deleted.
- **Symlink protection for the roots themselves**: the three allow-listed roots are resolved first as well. If a root (such as `downloads`) is a symlink to an external disk and resolves outside `~/.pt-tools`, that **whole category is refused**, a warning is logged and nothing under it is deleted.
- **A second red-line check**: even if a candidate happens to land inside the allow-list, its file name is checked once more against the red lines (`torrents.db`/wal/shm, `secret.key`, the log files being written) and refused on a match.
- **One refused file does not stop the rest**: a file refused because of a red line or an escape is only recorded as skipped (跳过); the cleanup of the other files carries on.

---

Related pages:

- [Configuration](../configuration.md)
- [RSS subscriptions](rss-subscription.md)
- [Questions and troubleshooting](../faq.md)
