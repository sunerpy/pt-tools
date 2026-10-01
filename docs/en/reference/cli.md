# Command line

This page lists the commands and options of the `pt-tools` program. Day to day you only need `pt-tools web`; the others back up the key, remove old files and help with troubleshooting.

In Docker, put `docker exec pt-tools` in front of a command, for example `docker exec pt-tools pt-tools version`.

## pt-tools web

Starts the web interface and the background jobs. Running `pt-tools` with no command does the same.

```bash
pt-tools web --host 0.0.0.0 --port 8080
```

| Option   | Meaning              | Default   |
| -------- | -------------------- | --------- |
| `--host` | Address to listen on | `0.0.0.0` |
| `--port` | Port to listen on    | `8080`    |

The Docker image sets both through the environment variables `PT_HOST` and `PT_PORT`; the other variables are listed in [Install](../guide/install.md) and [Configuration](../configuration.md).

## pt-tools secret

Exports or imports the key that encrypts site cookies and notification credentials.

```bash
# Print the key to standard output as base64 text
pt-tools secret export

# Read a base64 key from standard input and write ~/.pt-tools/secret.key
pt-tools secret import < secret.b64
```

| Command  | Option    | Meaning                                                                                           |
| -------- | --------- | ------------------------------------------------------------------------------------------------- |
| `export` |           | The output is the key itself; keep it safe                                                        |
| `import` | `--force` | Replace an existing `secret.key`; without it, an existing file is left alone and the import fails |

The import checks that the key is 32 bytes long and writes the file atomically. The full backup and restore procedure is in [Upgrades and backups](../guide/upgrade.md).

## pt-tools clean

Removes old logs, staged torrent files and old backups from the data folder. By default it only previews and deletes nothing.

```bash
pt-tools clean                         # preview every category
pt-tools clean --category logs         # preview logs only
pt-tools clean --confirm               # delete
pt-tools clean --confirm --keep-backups 10
```

| Option           | Meaning                                                                                 | Default |
| ---------------- | --------------------------------------------------------------------------------------- | ------- |
| `--confirm`      | Actually delete; without it the command only previews                                   | Off     |
| `--category`     | One or more of `logs`, `staging` and `backups`, separated by commas; empty means all    | All     |
| `--keep-backups` | How many of the most recent backups the `backups` category keeps                        | `5`     |
| `--dry-run`      | Preview mode. Setting it to `false` without `--confirm` makes the command refuse to run | `true`  |

The categories:

- `logs`: rotated log backups. The log file being written is never removed.
- `staging`: `.torrent` files in `downloads/` that were pushed, are orphaned or are past their retention.
- `backups`: configuration backups made automatically; only the most recent N are kept.

`torrents.db`, `secret.key` and the log being written are never removed, whatever the options.

## pt-tools version

Prints the version, the build time and the Git commit. Include this output when you report a problem.

```bash
pt-tools version
```

## pt-tools completion

Generates a shell completion script for Bash or Zsh.

```bash
source <(pt-tools completion bash)
pt-tools completion zsh > "${fpath[1]}/_pt-tools"
```

## Less common commands

These commands do not appear in `pt-tools --help` and are for particular situations only.

| Command                    | Purpose                                                                                                                                                                       |
| -------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `pt-tools config init`     | Creates `~/.pt-tools` and its `downloads` folder ahead of time. `pt-tools web` also creates them on start                                                                     |
| `pt-tools db fix-timezone` | Corrects the 8-hour offset early versions introduced by storing freeleech end times as UTC. Preview with `--dry-run`, then run it; do not run it unless you have that problem |

`pt-tools web` migrates the database schema on start, so the database never needs initialising or upgrading by hand.
