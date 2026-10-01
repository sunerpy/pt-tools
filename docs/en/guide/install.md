# Install

This page covers installing pt-tools with Docker, as a Linux binary or as a Windows binary, then signing in for the first time and what is in the data folder.

| Option         | Systems                     | Notes                                                    |
| -------------- | --------------------------- | -------------------------------------------------------- |
| Docker Compose | Any system that runs Docker | Recommended. Upgrading is pulling a new image            |
| `docker run`   | The same                    | A single command, handy for trying it out                |
| Linux binary   | Linux amd64 and arm64       | The install script checks SHA-256; can run under systemd |
| Windows binary | Windows amd64 and arm64     | A PowerShell install script, or unpack it yourself       |

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

The image is published on Docker Hub (`sunerpy/pt-tools`) and on GHCR (`ghcr.io/sunerpy/pt-tools`, with a build attestation); both are the same image.

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

## Linux binary

### With the install script

```bash
curl -fsSL https://raw.githubusercontent.com/sunerpy/pt-tools/main/scripts/install.sh | sh
```

The script downloads the latest release and that release's `checksums.txt`, and refuses to install if the SHA-256 does not match. It installs to `~/.local/bin`; two environment variables change that:

- `TOOL_VERSION`: install a particular version, for example `TOOL_VERSION=v0.48.0`.
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

To install a particular version, set `$env:TOOL_VERSION = "v0.48.0"` first. You can also download `pt-tools-windows-amd64.exe.zip` from the [releases page](https://github.com/sunerpy/pt-tools/releases), unpack it and run:

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
