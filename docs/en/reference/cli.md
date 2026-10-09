# Command line

This page lists the commands and options of the `pt-tools` program. Day to day you only need `pt-tools web`; the others back up the key, remove old files and help with troubleshooting.

In Docker, put `docker exec pt-tools` in front of a command, for example `docker exec pt-tools pt-tools version`.

## pt-tools web

Starts the web interface and the background jobs. Running `pt-tools` with no command does the same.

```bash
pt-tools web --host 0.0.0.0 --port 8080
```

| Option               | Meaning                                                                                                                                                                                              | Default   |
| -------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------- |
| `--host`             | Address to listen on                                                                                                                                                                                 | `0.0.0.0` |
| `--port`             | Port to listen on                                                                                                                                                                                    | `8080`    |
| `--qbit-compat-addr` | Listen address of the [qB-compatible entrance](../guide/qbit-compat.md), such as `0.0.0.0:8081`; when empty, the environment variable `PT_QBIT_COMPAT_ADDR` is used, and without either it stays off | Off       |

The Docker image sets both through the environment variables `PT_HOST` and `PT_PORT`; the other variables are listed in [Install](../guide/install.md) and [Configuration](../configuration.md).

## pt-tools mcp

Connects the [MCP](../guide/mcp.md) tools of a running pt-tools to standard input and output, for MCP clients that only speak stdio, such as Claude Desktop. It connects to the pt-tools given by `--url` and forwards its tools as they are. It opens no database and starts no scheduler, so it can run on the client's machine.

```bash
PT_TOOLS_MCP_TOKEN=ptt_1_xxxxxxxx pt-tools mcp --url http://192.168.1.10:8080
```

| Option    | Meaning                                                                                                                                                                            | Default                 |
| --------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------- |
| `--url`   | Address of pt-tools; `/mcp` is appended when only the host and port are given. When empty, the environment variable `PT_TOOLS_MCP_URL` is used                                     | `http://127.0.0.1:8080` |
| `--token` | API token with MCP read (MCP 读取) or MCP write (MCP 操作). When empty, the environment variable `PT_TOOLS_MCP_TOKEN` is used; on the command line it shows up in the process list | None                    |

Once connected it prints one line to standard error; standard output carries only the MCP protocol.

## pt-tools relay serve

Runs a relay for [remote access](../guide/remote-access.md): the mobile app and pt-tools both connect to it and it forwards their encrypted connections without being able to read them. Run it yourself on a VPS or NAS with a public address. It opens no database and does not read or write `~/.pt-tools`.

```bash
pt-tools relay serve --listen 0.0.0.0:8443 --public-url wss://relay.example.com
```

| Option                      | Meaning                                                                                                                                                                     | Default        |
| --------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------- |
| `--public-url`              | The relay's public address (`ws://` or `wss://`), the one you enter in the app and in pt-tools; hosts sign against it, so it must match the address actually used. Required | None           |
| `--listen`                  | Listen address                                                                                                                                                              | `0.0.0.0:8443` |
| `--max-streams-per-host`    | How many phone connections one pt-tools may have at once; extra ones are closed with 4429                                                                                   | `16`           |
| `--daily-bytes-per-host`    | Bytes forwarded per pt-tools per day (reset at 00:00 UTC), both directions together; over the limit its phone connections close until the reset. `0` means no limit         | `0`            |
| `--max-conn-per-ip-per-min` | New connections per IP address per minute; a negative value means no limit                                                                                                  | `30`           |
| `--client-ip-header`        | Take the client IP from this request header (behind a reverse proxy, for example `X-Real-IP`); set it only when every request comes through the proxy                       | None           |
| `--tls-cert`, `--tls-key`   | TLS certificate and key files; without them the relay speaks plain WebSocket and leaves TLS to a reverse proxy                                                              | None           |
| `--disabled`                | Pause the service: every connection is closed with 4503                                                                                                                     | off            |

Each option can also come from an environment variable: `PT_TOOLS_RELAY_LISTEN`, `PT_TOOLS_RELAY_PUBLIC_URL`, `PT_TOOLS_RELAY_MAX_STREAMS_PER_HOST`, `PT_TOOLS_RELAY_DAILY_BYTES_PER_HOST`, `PT_TOOLS_RELAY_MAX_CONN_PER_IP_PER_MIN`, `PT_TOOLS_RELAY_CLIENT_IP_HEADER`, `PT_TOOLS_RELAY_TLS_CERT`, `PT_TOOLS_RELAY_TLS_KEY` and `PT_TOOLS_RELAY_DISABLED=true`. The Docker image runs it when `PT_MODE=relay` is set. `GET /healthz` returns the status and the version.

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
