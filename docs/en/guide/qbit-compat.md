# qB-compatible entrance

The qB-compatible entrance lets tools that only work with qBittorrent, such as MoviePilot, IYUU, autobrr, Sonarr and Radarr, treat pt-tools as a qBittorrent client. The torrents they add are pushed by pt-tools to the real downloader you choose, with the usual disk space protection and site seeding capacity checks; torrent records, site tags and the audit log all stay in pt-tools.

The entrance listens on its own port and is off by default.

## Turning it on

1. Start pt-tools with `--qbit-compat-addr 0.0.0.0:8081`, or set the environment variable `PT_QBIT_COMPAT_ADDR=0.0.0.0:8081`, and restart. With Docker, use the environment variable and publish the port:

   ```yaml
   environment:
     PT_QBIT_COMPAT_ADDR: "0.0.0.0:8081"
   ports:
     - "8081:8081"
   ```

2. Under Downloads → qB-compatible entrance (下载 → qB 兼容入口), choose the downloader behind it (the default downloader when none is chosen) and turn on Full control (完全控制) if you need it. The page also shows whether the entrance is listening.
3. Under System → API tokens (系统 → API 令牌), create a token with the qB compatible (qB 兼容) permission; see [API tokens](api-tokens.md).

## Client settings

Add a qBittorrent downloader in the tool's settings:

| Setting  | Value                                                        |
| -------- | ------------------------------------------------------------ |
| Address  | `http://<pt-tools host>:8081`                                |
| Username | Anything, for example `pt-tools` (recorded in the audit log) |
| Password | An API token with the qB compatible permission (`ptt_…`)     |

- Tools that sort torrents by category, such as Sonarr and Radarr, create the category before adding torrents; the entrance accepts this, and the folder given when creating a category becomes that category's save folder.
- IYUU cross-seeding hands over download links for other sites: those sites must be enabled in pt-tools with their cookies set, so that pt-tools can download with its own site settings. pt-tools also has its own [IYUU cross-seeding](reseed.md).

## What it can do

- **Add torrents**: upload torrent files, or give a site's torrent download link.
  - Torrent files are matched to a site by their tracker; when a site is recognised, its site tag is added.
  - Download links are accepted only for enabled sites: pt-tools downloads with its own site settings and never requests the address the client gave.
  - Magnet links are not accepted.
  - Options: save folder, category, tags, add paused, rename and speed limits (the stricter of these and the site's limits applies).
- **List torrents**: the torrent list, details, files, trackers, categories and tags cover every torrent in the downloader behind the entrance. Passkeys in tracker addresses are hidden.
- **Change torrents**: pause, resume, delete (optionally with the data), change category, add and remove tags. By default only torrents the entrance added to this downloader can be changed (not ones that were already in the downloader when added, nor the same torrent re-added elsewhere after the entrance deleted it); with Full control on, every torrent in the downloader can. Torrents the client may not change are skipped: the client still sees success, and the audit log records a refusal.

It also works with Transmission behind it; torrent states, categories and tags are translated into qBittorrent's form.

The rest of qBittorrent's API (RSS, search, logs, changing preferences, adjusting speed limits and priorities, and so on) responds with 404.

## Sign-in and security

- Any username works; the password is the API token. After 5 failed sign-ins from one IP within 15 minutes, that IP cannot sign in for 15 minutes.
- A session with no requests for an hour expires; once its token is revoked or expires, the session ends immediately. After pt-tools restarts, clients sign in again on their own.
- Open this port only on your local network: over plain HTTP, someone on the same network could capture the session cookie and the token.
- Changes and failed sign-ins are recorded under ChatOps → Audit log (操作审计) with the channel qB compatible (qB 兼容).
