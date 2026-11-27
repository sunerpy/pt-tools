# Install

This page covers installing pt-tools with Docker, as a Linux binary or as a Windows binary, then signing in for the first time and what is in the data folder.

| Option         | Systems                     | Notes                                                                                                           |
| -------------- | --------------------------- | --------------------------------------------------------------------------------------------------------------- |
| Docker Compose | Any system that runs Docker | Recommended. Upgrading is pulling a new image                                                                   |
| `docker run`   | The same                    | A single command, handy for trying it out                                                                       |
| Linux binary   | Linux amd64 and arm64       | The install script checks SHA-256; can run under systemd                                                        |
| Windows binary | Windows amd64 and arm64     | A PowerShell install script, or unpack it yourself                                                              |
| NAS            | 1Panel, Unraid and fnOS     | Install from the app definitions in `deploy/`; on fnOS you can also install the fnOS app package from a release |

There is no macOS binary; use Docker on macOS.

## Docker Compose (recommended)

Save this as `compose.yml`:

```yaml
services:
  pt-tools:
    image: sunerpy/pt-tools:latest
    container_name: pt-tools
    environment:
      PT_HOST: "0.0.0.0"
      PT_PORT: "8080"
      TZ: "Asia/Shanghai"
      PUID: "1000"
      PGID: "1000"
    ports:
      - "8080:8080"
    volumes:
      - ./data:/app/.pt-tools
    restart: unless-stopped
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"
```

Then start it:

```bash
docker compose up -d
```

> [!IMPORTANT]
> Keep `/app/.pt-tools` on a volume. It holds the database, your settings and the `secret.key` that encrypts site credentials; if the key is deleted or lost, saved cookies cannot be recovered.

The `logging` section caps the container's standard output log. Without it, Docker's log file keeps growing for as long as the container runs, which matters on a NAS in particular; see [Configuration](../configuration.md).

The image is published on Docker Hub (`sunerpy/pt-tools`) and on GHCR (`ghcr.io/sunerpy/pt-tools`, with a build attestation); both are the same image. If pulling from Docker Hub is slow where you are, set `registry-mirrors` in Docker's `daemon.json`; the image name stays the same.

## docker run

```bash
docker run -d \
  --name pt-tools \
  -p 8080:8080 \
  -v ~/pt-data:/app/.pt-tools \
  -e TZ=Asia/Shanghai \
  sunerpy/pt-tools:latest
```

### Common environment variables

| Variable          | Meaning                                                        | Default         |
| ----------------- | -------------------------------------------------------------- | --------------- |
| `PT_HOST`         | Address to listen on                                           | `0.0.0.0`       |
| `PT_PORT`         | Port to listen on                                              | `8080`          |
| `TZ`              | Time zone                                                      | `Asia/Shanghai` |
| `PUID`, `PGID`    | User and group IDs the program runs as inside the container    | `1000`          |
| `PT_ADMIN_USER`   | Initial admin user name                                        | `admin`         |
| `PT_ADMIN_PASS`   | Initial admin password                                         | `adminadmin`    |
| `PT_ADMIN_RESET`  | Set to `1` to reset the admin password to the two values above | Not set         |
| `HTTP_PROXY` etc. | Proxies; see [Configuration](../configuration.md)              | Not set         |

## NAS

The `deploy/` folder in the repository has Docker app definitions for 1Panel, Unraid and fnOS. The steps below install from those files directly, without an app store. These definitions map the data folder to `/app/.pt-tools` in the container; keep that folder across upgrades and reinstalls, and back up the `secret.key` inside it. On fnOS you can also install the fnOS app, which does not use Docker; see below.

The container runs as the user given by `PUID` and `PGID`, and hands the files under `/app` to that user on startup. Group ID 100, common on NAS systems, already exists in the image (the `users` group) and is used as is. This needs image 1.0.0 or later: 0.48.0 and earlier images fail to start when `PGID` is a group that already exists in the image (the log says `addgroup: gid in use`).

### 1Panel

Copy the whole `deploy/1panel/pt-tools` folder to `/opt/1panel/resource/apps/local/pt-tools` on the server, refresh the local app list in the 1Panel app store, then install pt-tools and fill in the web port and time zone. Data lives in `data/` under the app folder.

### Unraid

Download `deploy/unraid/pt-tools.xml` to `/boot/config/plugins/dockerMan/templates-user/my-pt-tools.xml` on the flash drive, click **Add Container** on the Docker page and pick pt-tools from the Template list. The template defaults to `PUID=99` and `PGID=100` (Unraid's nobody and users), with data in `/mnt/user/appdata/pt-tools`.

### fnOS

There are two ways to install on fnOS: the fnOS app (no Docker) or Docker Compose. Pick one.

#### fnOS app

From 1.2.0, every [release](https://github.com/sunerpy/pt-tools/releases/latest) includes an fnOS app package: download `pt-tools-fnos-x86.fpk` for x86 devices or `pt-tools-fnos-arm.fpk` for ARM devices (the ARM package has not been tested on a real device yet).

1. Open App Center (应用中心), click Manual install (手动安装) at the bottom left, choose a volume, then click Upload from computer (从电脑上传) and pick the fpk.
2. The package is not signed by fnOS, so App Center shows a security notice about an unverified app (未经验证应用的安全提示). Click Agree (同意) to continue.
3. The install wizard shows a few notes; click Next (下一步) to finish. A pt-tools icon appears on the desktop and opens the web interface at `http://<NAS address>:8080`.
4. Under Settings → Apps → pt-tools → Accessible files (系统设置 → 应用 → pt-tools → 可访问的文件), click Add (添加), choose the folder that holds the downloads and the media libraries (for example `video`) and give it read and write access (读写). pt-tools runs as its own app account and cannot read or write these folders until you grant access; organising into libraries fails without it.

The fnOS app sees the real paths on the NAS (for example `/vol1/1000/video/downloads`), so no folders need mounting. When qBittorrent runs on the same NAS, both see the same paths and no path mapping is needed; to try a hardlink with the library check (检查), add a mapping with the same path on both sides under Media → Media library → Path mappings (媒体 → 媒体库 → 路径映射).

- The web interface always uses port 8080. When something else holds 8080 (for example pt-tools in Docker, or qBittorrent on 8080), App Center reports that pt-tools cannot be enabled because the port is in use (无法启用 pt-tools：端口被占用). Stop the other program; to use another port, install with Docker as below.
- Data lives in the app's own folder (`/vol1/@appdata/pt-tools/.pt-tools`). Download backups under Global settings → Backup (全局设置 → 备份); they include `secret.key` by default.
- When uninstalling, choose to keep the data (保留数据) and a later install finds it again; choosing to delete everything (清除所有数据) removes the database and `secret.key`.
- Moving from the Docker install: in the Docker install, click Back up now (立即备份) under Global settings → Backup and download the backup; stop the Docker container; in the fnOS app, use Upload a backup to restore (上传备份恢复) in the Backup and restore (备份与恢复) panel; then disable and enable pt-tools once in App Center.

For upgrades, see [Upgrades and backups](upgrade.md#fnos-app).

#### Docker Compose

Create a Compose project in the fnOS Docker app using [the Compose file above](#docker-compose-recommended). Replace `./data` with a folder in a shared folder (for example `/vol1/1000/docker/pt-tools`) and set `PUID` and `PGID` to the owner of that folder (check with `ls -n`). `deploy/fnos/apps` is the Docker packaging for the third-party app center fnos-apps; the fnOS app package above is built from `deploy/fnos/native`.

## Linux binary

### With the install script

```bash
curl -fsSL https://raw.githubusercontent.com/sunerpy/pt-tools/main/scripts/install.sh | sh
```

The script downloads the latest release and that release's `checksums.txt`, and refuses to install if the SHA-256 does not match. It installs to `~/.local/bin`; two environment variables change that:

- `TOOL_VERSION`: install a particular version, for example `TOOL_VERSION=v1.0.0`.
- `TOOL_INSTALL_DIR`: the folder to install into.

Every release's notes include an install command pinned to that release, ready to copy.

### Downloading it yourself

From the [releases page](https://github.com/sunerpy/pt-tools/releases), download the archive for your architecture and `checksums.txt`, check it, then unpack it:

```bash
sha256sum -c checksums.txt --ignore-missing
tar -xzf pt-tools-linux-amd64.tar.gz
chmod +x pt-tools
./pt-tools web --host 0.0.0.0 --port 8080
```

To confirm that an archive was built by this project's release pipeline, verify its build attestation with the GitHub CLI:

```bash
gh attestation verify pt-tools-linux-amd64.tar.gz --repo sunerpy/pt-tools \
  --signer-workflow sunerpy/pt-tools/.github/workflows/release.yml \
  --deny-self-hosted-runners
```

### Running it under systemd

Create `/etc/systemd/system/pt-tools.service`, with your own user and paths:

```ini
[Unit]
Description=pt-tools
After=network.target

[Service]
Type=simple
User=your_username
WorkingDirectory=/home/your_username
ExecStart=/usr/local/bin/pt-tools web --host 0.0.0.0 --port 8080
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now pt-tools
sudo journalctl -u pt-tools -f
```

The data is kept in `~/.pt-tools` of the user the service runs as.

## Windows binary

In PowerShell:

```powershell
irm https://raw.githubusercontent.com/sunerpy/pt-tools/main/scripts/install.ps1 | iex
```

To install a particular version, set `$env:TOOL_VERSION = "v1.0.0"` first. You can also download `pt-tools-windows-amd64.exe.zip` from the [releases page](https://github.com/sunerpy/pt-tools/releases), unpack it and run:

```powershell
.\pt-tools.exe web --host 0.0.0.0 --port 8080
```

The data is kept in the `.pt-tools` folder in your user folder.

## Signing in for the first time

Open `http://your-server:8080` and sign in with the initial account: user name `admin`, password `adminadmin`. If you set `PT_ADMIN_USER` and `PT_ADMIN_PASS` before the first start, those apply instead.

Change the password straight away in Change password (修改密码). If you forget it, follow "Resetting the admin password" in [Configuration](../configuration.md).

> [!WARNING]
> Do not expose pt-tools directly to the internet. For remote access, put it behind a reverse proxy with HTTPS, or reach it over a VPN.

## The data folder

Every option uses the same layout:

```text
.pt-tools/
├── secret.key     # the credential encryption key; back it up with the database
├── torrents.db    # SQLite database: settings, task history and cached statistics
├── downloads/     # staged .torrent files
├── backups/       # configuration backups made automatically
└── logs/          # rotated logs
```

Backing up and restoring are covered in [Upgrades and backups](upgrade.md).

## Next

Once it is running, follow [Quick start](quick-start.md) to add a downloader, your sites and a first RSS feed.
