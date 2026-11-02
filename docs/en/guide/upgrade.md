# Upgrades and backups

This page covers upgrading pt-tools, backing up and restoring its data, and removing old logs and staged files.

## Upgrading

The database schema is migrated automatically on start, so an upgrade needs nothing done to the data. The first time a new version starts, pt-tools takes a backup automatically before it migrates the database (see [Backing up](#backing-up)), so there is no need to back up by hand before upgrading; for an extra copy, click Back up now (立即备份) in the web interface first.

### Docker

```bash
docker compose pull
docker compose up -d
```

With `docker run`, pull the new image, then create the container again with the same options and the same data folder:

```bash
docker pull sunerpy/pt-tools:latest
docker stop pt-tools && docker rm pt-tools
docker run -d --name pt-tools -p 8080:8080 \
  -v ~/pt-data:/app/.pt-tools -e TZ=Asia/Shanghai \
  sunerpy/pt-tools:latest
```

The upgrade button in the web interface does not work inside a container; upgrade by changing the image.

### Linux and Windows binaries

When a new release is out, the web interface says so next to the version number at the top left. From there you can read the release notes and start the upgrade: pt-tools downloads the new version, checks its SHA-256 against that release's `checksums.txt`, and only then replaces its own executable and shows "Upgrade complete, please restart" (升级完成，请重启应用). If the checksum does not match, or the release has no `checksums.txt`, the upgrade fails and the running program is left as it was.

- Under systemd, run `sudo systemctl restart pt-tools`.
- If you started it by hand, stop the process and run `pt-tools web` again.

Upgrading to a pre-release (an rc version) asks you to confirm first; unless you need it, wait for the stable release. Running the install script from [Install](install.md) again also upgrades.

## Backing up

A backup is one `pt-tools-backup-<time>-<reason>.tar.gz` file. It holds a snapshot of the database (all configuration, sites, subscriptions, rules and history) and a description file, plus `secret.key` (the key that encrypts site cookies and notification credentials) when Include the key (带上密钥) is on. The snapshot can be taken while pt-tools is running; there is no need to stop it.

Backups are taken at these times:

| Reason              | When                                                                                                      | Kept                            |
| ------------------- | --------------------------------------------------------------------------------------------------------- | ------------------------------- |
| Scheduled backup    | Once a day at the time you set (04:30 by default); if pt-tools was not running then, after its next start | The latest 7 (adjustable, 1–60) |
| Pre-upgrade backup  | The first time a new version starts, before the database is migrated                                      | The latest 3                    |
| Manual backup       | When you click Back up now (立即备份) in the web interface                                                | Never removed automatically     |
| Command-line backup | When you run `pt-tools db backup`                                                                         | Never removed automatically     |

The settings are under Global settings → Backup (全局设置 → 备份):

- **Daily scheduled backup** (每天定时备份): on by default. When a scheduled backup fails, pt-tools sends a Backup failed (备份失败) notification to every enabled notification channel, at most once a day, and shows the reason in the Backup and restore (备份与恢复) panel. The pre-upgrade backup does not depend on this switch.
- **Backup time** (备份时刻) and **Keep** (保留份数): Keep counts scheduled backups only.
- **Backup folder** (备份目录): empty means `backups` in the data folder. It must be an absolute path; when you save, pt-tools writes a test file there to check that it can. On another disk or network storage, the backups survive a failure of the data disk. In Docker, mount this folder into the container.
- **Include the key** (带上密钥): on by default. A backup that includes the key can be restored on another machine, or after the original key is lost, but anyone who gets it can decrypt the cookies and notification credentials saved in it, so keep it as carefully as the database. With the switch off, a backup holds the database only, and restoring it needs the key that was in use when it was taken.

The Backup and restore (备份与恢复) panel further down the same page lists the backups in the backup folder; from there you can download, restore or delete them, or click Back up now (立即备份).

> [!IMPORTANT]
> Back up and restore the database and the key together. If you restore the database without its original key, the saved cookies and notification credentials cannot be decrypted.

### Command-line backup

```bash
# Into the folder from the backup settings (backups in the data folder by default)
pt-tools db backup
# Into another folder, or to a given file (an existing file is never overwritten)
pt-tools db backup --output /mnt/backup/
# Without the key
pt-tools db backup --no-secret

# Docker
docker exec pt-tools pt-tools db backup
```

The command works while pt-tools is running: it opens the database read-only and changes nothing. Whether the key is included follows the backup settings unless you pass `--no-secret`.

You can also stop pt-tools and copy `torrents.db` and `secret.key` from the data folder yourself: once the service has stopped, the contents of `torrents.db-wal` have been written back to the database file.

### Backing up the key on its own

The key can also be exported as one line of base64 text and kept in a password manager:

```bash
pt-tools secret export > secret.b64
# Docker
docker exec pt-tools pt-tools secret export > secret.b64
```

The output is the key itself; keep it as you would a password. You need it to restore a backup taken with Include the key (带上密钥) off.

## Restoring

A restore has two steps: a backup is first checked and staged, and it replaces the current data **when pt-tools restarts**. Until then you can cancel it, and the current data is not touched.

- **Web interface**: under Global settings → Backup and restore (全局设置 → 备份与恢复), click Restore (恢复) on a backup, or use Upload a backup to restore (上传备份恢复) for a backup file (up to 2 GiB). Once the backup passes the checks, the top of the panel says it is staged; restart pt-tools to apply it, or click Cancel restore (取消恢复) before restarting.
- **Command line**:

  ```bash
  pt-tools db restore --input /mnt/backup/pt-tools-backup-20261102-043000-scheduled.tar.gz
  pt-tools db restore --cancel   # cancel a restore that has not been applied yet

  # Docker: the backup file must be visible inside the container, for example in the data folder
  docker exec pt-tools pt-tools db restore --input /app/.pt-tools/backups/pt-tools-backup-20261102-043000-scheduled.tar.gz
  docker restart pt-tools
  ```

On restart, before it opens the database, pt-tools moves the current `torrents.db` (and `secret.key`, when the key changes) into `backups/pre-restore-<time>/` in the data folder, puts the files from the backup in place, and decrypts the saved credentials once with the key now in use. If any step fails, it puts the original files back and starts with the original data, and the panel shows why the backup was not applied. Once you are satisfied with the restored data, you can delete the `pre-restore-<time>` folder.

The key is handled by these rules; a backup that fails them is not staged:

- When the key comes from the environment variable `PT_TOOLS_SECRET_KEY`, pt-tools keeps using it after the restore and leaves `secret.key` alone. If that key cannot decrypt the credentials saved in the backup, the restore is refused.
- The backup includes the key: after the restore pt-tools uses the key from the backup. If the original `secret.key` is lost after the backup was staged, that is fine: the key from the backup is put in place on restart.
- The backup does not include the key: the restore is allowed only if the current key can decrypt the credentials saved in the backup (or the backup holds no encrypted credentials), and `secret.key` stays as it is. Otherwise import the key from the time of the backup with `pt-tools secret import` first.

A backup taken by a newer version of pt-tools (with a newer database version) cannot be restored; upgrade pt-tools first. A backup from an older version is migrated as usual after it is put in place, and a pre-upgrade backup is taken before that migration too.

### Without the restore feature

To go back to an older version (for example from a preview release to 0.48.0), which has no restore feature: stop pt-tools, extract the pre-upgrade backup (`pt-tools-backup-<time>-pre-upgrade.tar.gz`), put its `torrents.db` and `secret.key` in the data folder (and delete `torrents.db-wal` and `torrents.db-shm` there), then start the older version.

If you only have the base64 copy of the key, write it back with `secret import` (`--force` is needed when a key file already exists):

```bash
pt-tools secret import --force < secret.b64
```

Instead of a file, you can also supply the base64 key in the environment variable `PT_TOOLS_SECRET_KEY`, which takes precedence over `secret.key`.

pt-tools checks the key at startup and refuses to start, with the reason in the log, in two cases:

- `secret.key` exists but cannot be read or is not in the expected format (64 hex characters). pt-tools does not overwrite it; put the right file back, or restore your backup with `secret import`.
- `secret.key` does not exist, yet the database already holds encrypted cookies or notification credentials. This usually means only the database was restored, or the data directory is not mounted correctly; put the original `secret.key` back and start again. If the original key is really lost, start once with the environment variable `PT_TOOLS_ACCEPT_NEW_SECRET_KEY=1`: pt-tools switches to a new key, and you then enter the site cookies and notification credentials again.

## Removing old logs and staged files

Logs, staged `.torrent` files and old configuration backups build up over time. Preview and remove them under Clean the work folder (清理工作目录) at the bottom of Rules → Auto cleanup (规则 → 自动清理), or from the command line:

```bash
# Preview (by default nothing is deleted)
pt-tools clean
# Delete; --category limits it to some of logs, staging and backups
pt-tools clean --confirm
```

The database and the key are protected and are never removed by a cleanup. The `backups` category only removes the old configuration backups left by migrations (`.json` files); backup files (`.tar.gz`) and `pre-restore-<time>` folders are never removed by it. All of the options are listed in [Command line](../reference/cli.md).

The container's own standard output log is not in the data folder; how to cap it is explained under "Log management" in [Configuration](../configuration.md).
