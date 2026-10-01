# Configuration

This page describes every pt-tools setting in detail: environment variables, global settings, downloaders and RSS subscriptions.

## Environment variables

Environment variables set how pt-tools runs, which suits Docker deployments.

| Variable         | Meaning                              | Default         | Example            |
| ---------------- | ------------------------------------ | --------------- | ------------------ |
| `PT_HOST`        | Address the web interface listens on | `0.0.0.0`       | `127.0.0.1`        |
| `PT_PORT`        | Port the web interface listens on    | `8080`          | `8888`             |
| `PT_ADMIN_USER`  | Admin user name                      | `admin`         | `myadmin`          |
| `PT_ADMIN_PASS`  | Admin password                       | `adminadmin`    | `MySecurePass123`  |
| `PT_ADMIN_RESET` | Resets the admin password            | -               | `1` (on)           |
| `PUID`           | User ID inside the container         | `1000`          | `1001`             |
| `PGID`           | Group ID inside the container        | `1000`          | `1001`             |
| `TZ`             | Time zone                            | `Asia/Shanghai` | `America/New_York` |

### Using environment variables

**On the Docker command line**:

```bash
docker run -d \
  -e PT_HOST=0.0.0.0 \
  -e PT_PORT=8080 \
  -e PT_ADMIN_USER=admin \
  -e PT_ADMIN_PASS=your_password \
  -e TZ=Asia/Shanghai \
  sunerpy/pt-tools:latest
```

**Docker Compose**:

```yaml
environment:
  PT_HOST: "0.0.0.0"
  PT_PORT: "8080"
  PT_ADMIN_USER: "admin"
  PT_ADMIN_PASS: "your_password"
  TZ: "Asia/Shanghai"
```

### Resetting the admin password

If you forget the password, reset it with environment variables:

```bash
docker run -d \
  -e PT_ADMIN_RESET=1 \
  -e PT_ADMIN_USER=admin \
  -e PT_ADMIN_PASS='your_new_password' \
  -v ~/pt-data:/app/.pt-tools \
  -p 8080:8080 \
  sunerpy/pt-tools:latest
```

> After the reset, remove the `PT_ADMIN_RESET` environment variable and start the container again.

## Proxy settings

pt-tools reads the standard proxy environment variables, whether it runs in Docker, under systemd or as a binary you start yourself.

| Variable                      | Meaning                                              | Example                    |
| ----------------------------- | ---------------------------------------------------- | -------------------------- |
| `HTTP_PROXY` / `http_proxy`   | Proxy for HTTP requests                              | `http://127.0.0.1:7890`    |
| `HTTPS_PROXY` / `https_proxy` | Proxy for HTTPS requests                             | `http://127.0.0.1:7890`    |
| `ALL_PROXY` / `all_proxy`     | General proxy, used as a fallback                    | `socks5://127.0.0.1:1080`  |
| `NO_PROXY` / `no_proxy`       | Addresses that bypass the proxy, separated by commas | `localhost,127.0.0.1,.lan` |

Notes:

- Set both `HTTP_PROXY` and `HTTPS_PROXY`.
- When neither `HTTP_PROXY` nor `HTTPS_PROXY` is set, site access and downloader connections fall back to `ALL_PROXY`. Fetching RSS feeds and the Telegram channel currently read only `HTTP_PROXY`/`HTTPS_PROXY`, so set those two when they need a proxy (a Telegram channel can also have its own proxy URL).
- `NO_PROXY` is useful for local addresses: it keeps requests to local services and the LAN off the proxy.

### Proxy examples

**On the Docker command line**:

```bash
docker run -d \
  --name pt-tools \
  -p 8080:8080 \
  -v ~/pt-data:/app/.pt-tools \
  -e HTTP_PROXY=http://127.0.0.1:7890 \
  -e HTTPS_PROXY=http://127.0.0.1:7890 \
  -e ALL_PROXY=socks5://127.0.0.1:1080 \
  -e NO_PROXY=localhost,127.0.0.1,.lan \
  sunerpy/pt-tools:latest
```

**Docker Compose**:

```yaml
environment:
  HTTP_PROXY: "http://127.0.0.1:7890"
  HTTPS_PROXY: "http://127.0.0.1:7890"
  ALL_PROXY: "socks5://127.0.0.1:1080"
  NO_PROXY: "localhost,127.0.0.1,.lan"
```

## Global settings

These options are on the System → Global settings (系统 → 全局设置) page of the web interface:

| Setting                                                     | Meaning                                                            | Default     | Suggested           |
| ----------------------------------------------------------- | ------------------------------------------------------------------ | ----------- | ------------------- |
| **Default interval in minutes (默认间隔（分钟）)**          | The default interval for RSS tasks                                 | 10          | 15-30 minutes       |
| **Torrent file folder (种子下载目录)**                      | The folder where `.torrent` files are saved                        | `downloads` | As you need         |
| **Download speed check (启用下载限速判断)**                 | Used to decide whether a torrent can finish within its free period | Off         | On if you need it   |
| **Estimated download speed in MB/s (预估下载速度（MB/s）)** | Your actual average download speed                                 | -           | Your real bandwidth |
| **Maximum torrent size in GB (最大种子大小（GB）)**         | Torrents larger than this are skipped                              | No limit    | 50-100 GB           |
| **Minimum free time in minutes (最短免费时间（分钟）)**     | Torrents with less freeleech time left than this are skipped       | 30          | 20-60 minutes       |
| **Start tasks automatically (自动启动任务)**                | Whether RSS tasks start when the program starts                    | No          | Yes                 |

### Download speed and the free period

With the download speed check on, pt-tools uses this formula to decide whether a torrent can finish within its free period:

```
estimated finish time = torrent size / estimated download speed
if estimated finish time > freeleech time left, skip the torrent
```

### Minimum free time

Even without the download speed check, pt-tools checks how much freeleech time a torrent has left. When that is less than the Minimum free time (最短免费时间) threshold, 30 minutes by default, the torrent is skipped. This keeps a torrent from being paused soon after it starts, which would waste download quota and bandwidth.

Set it to 0 to turn the check off.

### When freeleech ends

| Setting                                                                     | Meaning                                                                                                    | Default | Suggested         |
| --------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- | ------- | ----------------- |
| **Delete when freeleech ends (免费结束自动删除)**                           | When the free period ends, unfinished torrents are deleted with their data; when off, they are only paused | Off     | On if you need it |
| **Act early, minutes before freeleech ends (免费期结束前提前处理（分钟）)** | Pauses or deletes unfinished torrents N minutes early, to absorb deletion delay and reporting lag          | 0       | 5 minutes or more |

Acting early adds a safety margin: the pause or deletion runs at `freeleech end time - N minutes`. That absorbs the delay of the deletion itself and the lag before the downloader reports to the tracker, so that no non-free download is counted after the free period has ended.

- The range is `[0, 60]` minutes; a value outside it is clamped.
- `0` acts exactly when freeleech ends. This is the earlier behaviour, so existing setups are unchanged.
- The periodic fallback check runs every 5 minutes, so `≥5` minutes is recommended. Each torrent's own timer is accurate to the second, so the main path is not limited to that interval.
- The margin only moves the freeleech-end action; retries keep their own schedule.

## Downloaders

### Basic settings

pt-tools supports these downloaders:

| Downloader       | Supported versions | Recommendation |
| ---------------- | ------------------ | -------------- |
| **qBittorrent**  | 4.x+               | Recommended    |
| **Transmission** | 3.x+               | Supported      |

**Fields** of the Add downloader (添加下载器) dialog under Downloads → Downloader settings (下载 → 下载器设置):

| Field                              | Meaning                                             | Example                        |
| ---------------------------------- | --------------------------------------------------- | ------------------------------ |
| **Name (名称)**                    | A name that identifies the downloader               | `Home qBit`, `NAS main`        |
| **Type (类型)**                    | The kind of downloader                              | `qBittorrent` / `Transmission` |
| **Address (地址)**                 | Web UI address                                      | `http://192.168.1.10:8080`     |
| **User name (用户名)**             | Sign-in user name                                   | `admin`                        |
| **Password (密码)**                | Sign-in password                                    | `password`                     |
| **Set as default (设为默认)**      | Whether this is the default downloader              | Yes/No                         |
| **Start automatically (自动开始)** | Whether a pushed torrent starts downloading at once | Yes/No                         |

### Download folders

Each downloader can have several download folders:

| Field                                    | Meaning                                     | Example                  |
| ---------------------------------------- | ------------------------------------------- | ------------------------ |
| **Path (路径)**                          | The folder's actual path                    | `/data/downloads/movies` |
| **Alias (别名)**                         | The name shown for the folder               | `Movies`                 |
| **Set as default folder (设为默认目录)** | Whether this is the default download folder | Yes/No                   |

**Keep in mind**:

- The path must be valid on the machine the downloader runs on.
- In Docker, make sure the path is mapped correctly.
- Aliases make it quicker to choose a folder when you push a torrent.

**Example folders**:

```
Path: /data/downloads/movies    Alias: Movies    Default: No
Path: /data/downloads/tv        Alias: TV        Default: Yes
Path: /data/downloads/anime     Alias: Anime     Default: No
Path: /data/downloads/music     Alias: Music     Default: No
```

## RSS subscriptions

RSS subscriptions are the core of pt-tools. A feed is added on the RSS subscriptions (RSS 订阅) tab of a site's page, with these fields:

| Field                                            | Meaning                                      | Required | Example                     |
| ------------------------------------------------ | -------------------------------------------- | -------- | --------------------------- |
| **Name (名称)**                                  | A name that identifies the feed              | Yes      | `HDSky movies`, `MT series` |
| **Link (链接)**                                  | The RSS feed URL                             | Yes      | `https://site.com/rss?...`  |
| **Category (分类)**                              | The category in the downloader               | No       | `PT-Auto`                   |
| **Tags (标签)**                                  | Task tags, to keep things organised          | No       | `hdsky,movie`               |
| **Check interval in minutes (检查间隔（分钟）)** | How often the feed is fetched                | Yes      | 5-1440 minutes              |
| **Downloader (下载器)**                          | The downloader to use                        | No       | The default when not set    |
| **Download path (下载路径)**                     | The download folder to use                   | No       | The default when not set    |
| **Filter rules (过滤规则)**                      | Filter rules attached to the feed            | No       | Several can be chosen       |
| **Pause when freeleech ends (免费结束暂停)**     | Pauses the torrent when its free period ends | No       | Yes/No                      |

### Suggested RSS settings

| Situation                   | Suggested interval | Pause when freeleech ends |
| --------------------------- | ------------------ | ------------------------- |
| **Everyday ratio building** | 15-30 minutes      | Yes                       |
| **Following a series**      | 10-15 minutes      | Depends                   |
| **Grabbing new releases**   | 5-10 minutes       | No                        |

The full RSS guide is in [RSS subscriptions](guide/rss-subscription.md).

## Where data is kept

pt-tools keeps all of its data in the data folder:

| Item              | Docker path      | Local path    | Notes                     |
| ----------------- | ---------------- | ------------- | ------------------------- |
| **Data folder**   | `/app/.pt-tools` | `~/.pt-tools` | The root of all the data  |
| **Database**      | `torrents.db`    | `torrents.db` | The SQLite database file  |
| **Torrent files** | `downloads/`     | `downloads/`  | Downloaded .torrent files |
| **Log files**     | `logs/`          | `logs/`       | Logs                      |

## Log management

pt-tools' logs come in two parts, which are managed separately.

### 1. Application log files (`~/.pt-tools/logs/`)

The application rotates these itself with [lumberjack](https://github.com/natefinch/lumberjack), so they need no manual cleanup:

| Item              | Default | Meaning                                             |
| ----------------- | ------- | --------------------------------------------------- |
| Maximum file size | 10 MB   | A larger file is rotated into a time-stamped backup |
| Days kept         | 30 days | Older backups are removed at the next rotation      |
| Backups kept      | 10      | Each log stream keeps at most 10 old backups        |
| Compression       | On      | Old backups are stored compressed with gzip         |

The logs are split by level into `all.log`, `info.log`, `error.log` and `debug.log`. Together the four are capped at about 440 MB, so they do not grow without limit.

> An extra cleanup runs on start and removes backups beyond the retention policy. It covers the case where frequent restarts keep each file below the rotation size, which would otherwise leave old backups behind.

Two environment variables change this behaviour at start-up:

| Variable               | Default | Meaning                                                                                                                                                                          |
| ---------------------- | ------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `PT_TOOLS_LOG_LEVEL`   | `info`  | Log level: `debug`/`info`/`warn`/`error`                                                                                                                                         |
| `PT_TOOLS_LOG_CONSOLE` | `true`  | Whether logs also go to the console (stdout). On by default, so you can read them with `docker logs` or in a NAS console; set it to `false` to turn stdout output off completely |

### 2. The container's standard output log (important for Docker)

> [!WARNING]
> **Docker log growth**: the application also writes its logs to stdout, so that `docker logs` and NAS consoles can show them, and Docker's `json-file` log driver has **no size limit by default**. Over a long run this log can reach several GB (8 GB has been seen). It is kept in `/var/lib/docker/containers/<id>/<id>-json.log`, is **not controlled by the application's lumberjack**, and has to be capped on the Docker side.

Console output is on by default (`PT_TOOLS_LOG_CONSOLE=true`) so that logs can be read straight from the container console of a NAS such as fnOS or Synology. **So always** cap the container's log; otherwise Docker keeps every line of stdout. If you really do not need console logs, turn them off with `PT_TOOLS_LOG_CONSOLE=false`.

Choose the setting that matches how you deploy:

**Docker Compose (recommended, works on every platform)**: add a `logging` block under the service.

```yaml
services:
  pt-tools:
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"
```

**`docker run`**: add these options.

```bash
docker run -d \
  --name pt-tools \
  --log-opt max-size=10m \
  --log-opt max-file=3 \
  ... \
  sunerpy/pt-tools:latest
```

**fnOS, Synology DSM Container Manager, Unraid and Portainer**:
For a container created in a graphical interface, most versions **do not expose the log driver setting**. There are two ways to deal with that: (1) **for the long term (recommended)**, deploy with **Docker Compose** instead and add `logging` as shown above (a container already created in the GUI can be recreated with Compose over SSH on the NAS; the data folder is persisted, so nothing is lost); (2) **as an immediate fix**, clear the accumulated container log by hand (see "Clearing an accumulated container log" below). Changing `daemon.json` for a single container is not recommended.

**Linux, as the global default** (applies to every container created afterwards): edit `/etc/docker/daemon.json`.

```json
{
  "log-driver": "json-file",
  "log-opts": { "max-size": "10m", "max-file": "3" }
}
```

Then run `sudo systemctl restart docker`. ⚠️ The setting **only applies to containers created after the change**; an existing container has to be recreated to pick it up.

**Clearing an accumulated container log**: `docker logs` cannot empty it. There are two ways:

- Truncate the log file (the container keeps running):

  ```bash
  truncate -s 0 $(docker inspect --format='{{.LogPath}}' pt-tools)
  ```

- Or recreate the container (`docker rm`, then create it again; the data folder is persisted, so nothing is lost).

### Backing up data

**What to back up**: `torrents.db` (all settings and history) and `secret.key` (the key that encrypts site cookies and notification credentials). Back them up and restore them as one unit: a database restored without its original key leaves every saved cookie and notification credential unreadable.

The database runs in WAL mode, so the latest writes may still be in `torrents.db-wal`. Stop pt-tools before copying:

```bash
# Docker (data folder mounted at ./data)
docker compose stop
tar -czf pt-tools-backup.tar.gz -C ./data torrents.db secret.key
docker compose start

# Local install (systemd)
sudo systemctl stop pt-tools
tar -czf pt-tools-backup.tar.gz -C ~/.pt-tools torrents.db secret.key
sudo systemctl start pt-tools
```

**Restoring**: stop pt-tools, put the backed-up `torrents.db` and `secret.key` back in the data folder over the existing files, then start it. If you kept the key only as base64 text, write it back with `pt-tools secret import --force`. The full steps are in [Upgrades and backups](guide/upgrade.md).

## Example configurations

### A complete Docker Compose file

```yaml
services:
  pt-tools:
    image: sunerpy/pt-tools:latest
    container_name: pt-tools
    environment:
      PT_HOST: "0.0.0.0"
      PT_PORT: "8080"
      PT_ADMIN_USER: "admin"
      PT_ADMIN_PASS: "your_secure_password"
      TZ: "Asia/Shanghai"
      PUID: "1000"
      PGID: "1000"
    ports:
      - "8080:8080"
    volumes:
      - ./data:/app/.pt-tools
      - /path/to/downloads:/downloads # optional: map a download folder
    restart: unless-stopped
```

### Typical setups

**Setup 1: ratio building only**

- RSS interval: 15-30 minutes
- Download freeleech torrents only
- Turn on Pause when freeleech ends (免费结束暂停)
- Set Maximum torrent size (最大种子大小（GB）)

**Setup 2: following series as well as building ratio**

- RSS interval: 10 minutes
- Create a filter rule for the series that allows torrents that are not free
- Create a general rule that downloads freeleech only
- Turn on Pause when freeleech ends

**Setup 3: several sites**

- Configure RSS separately for each site
- Configure several downloaders (at home and at the office, for example)
- Set different download folders
- Use tags to tell the sources apart

---

Related pages:

- [RSS subscriptions](guide/rss-subscription.md)
- [Cookies and API keys](guide/get-cookie-apikey.md)
- [Filter rules and TV series](guide/filter-rules-tv-series.md)
