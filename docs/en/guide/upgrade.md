# Upgrades and backups

This page covers upgrading pt-tools, backing up and restoring its data, and removing old logs and staged files.

## Upgrading

The database schema is migrated automatically on start, so an upgrade needs nothing done to the data. Back up the data folder first, as described below.

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

When a new release is out, the web interface says so next to the version number at the top left. From there you can read the release notes and start the upgrade: pt-tools downloads the new version, replaces its own executable and then shows "Upgrade complete, please restart" (升级完成，请重启应用).

- Under systemd, run `sudo systemctl restart pt-tools`.
- If you started it by hand, stop the process and run `pt-tools web` again.

Upgrading to a pre-release (an rc version) asks you to confirm first; unless you need it, wait for the stable release. Running the install script from [Install](install.md) again also upgrades.

## Backing up

In the data folder (the folder mounted at `/app/.pt-tools` in Docker, or `~/.pt-tools` of the user running the binary), these are what to back up:

| File                                            | Contents                                                        |
| ----------------------------------------------- | --------------------------------------------------------------- |
| `torrents.db` (and its `-wal` and `-shm` files) | All settings, sites, feeds, rules and history                   |
| `secret.key`                                    | The key that encrypts site cookies and notification credentials |

> [!IMPORTANT]
> Back up and restore the database and the key together. Restoring the database without its original key leaves every saved cookie and notification credential unreadable.

The database runs in WAL mode, so the latest writes may still be in `torrents.db-wal`. The safest way is to stop pt-tools before copying:

```bash
# Docker
docker compose stop
tar -czf pt-tools-backup.tar.gz -C ./data torrents.db secret.key
docker compose start

# Binary under systemd
sudo systemctl stop pt-tools
tar -czf pt-tools-backup.tar.gz -C ~/.pt-tools torrents.db secret.key
sudo systemctl start pt-tools
```

Once the service has stopped, the WAL has been written back to the database file, so copying `torrents.db` and `secret.key` is enough.

### Backing up the key on its own

The key can also be exported as one line of base64 text, for example to keep in a password manager:

```bash
pt-tools secret export > secret.b64
# Docker
docker exec pt-tools pt-tools secret export > secret.b64
```

That output is the key itself; keep it as you would a password.

## Restoring

1. Stop pt-tools.
2. Put the backed-up `torrents.db` and `secret.key` back in the data folder, replacing the files there.
3. Start pt-tools.

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

The database and the key are protected and are never removed by a cleanup. All of the options are listed in [Command line](../reference/cli.md).

The container's own standard output log is not in the data folder; how to cap it is explained under "Log management" in [Configuration](../configuration.md).
