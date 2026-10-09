<div align="center">

<img src="../../web/frontend/public/logo.png" alt="pt-tools" width="128" />

# pt-tools

### Private tracker automation: RSS downloads, check-in, account keep-alive, seed boosting and statistics

[![CI](https://github.com/sunerpy/pt-tools/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/sunerpy/pt-tools/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/sunerpy/pt-tools)](https://github.com/sunerpy/pt-tools/releases)
[![Docker Pulls](https://img.shields.io/docker/pulls/sunerpy/pt-tools)](https://hub.docker.com/r/sunerpy/pt-tools)
[![Codecov](https://codecov.io/gh/sunerpy/pt-tools/branch/main/graph/badge.svg)](https://codecov.io/gh/sunerpy/pt-tools)
[![License](https://img.shields.io/badge/license-MIT-green)](../../LICENSE)

[Documentation](https://firlab.app/pt-tools/en/) · [Features](#features) · [Install](#install) · [Quick start](#quick-start) · [Usage](#usage) · [Docs](#docs) · [Development](#development)

[简体中文](../../README.md) · [**English**](./README.en.md)

</div>

---

Download freeleech torrents from your RSS feeds, check in daily and get reminded before an inactive account is disabled, boost your ratio with rule-based seeding, follow every site's statistics with a daily report, manage qBittorrent and Transmission in one place, and check on all of it from QQ or Telegram. It supports 66 sites out of the box and runs in Docker, on Linux and on Windows. The full documentation is at **[firlab.app/pt-tools/en](https://firlab.app/pt-tools/en/)**.

The web interface is in Chinese; the screenshots below show it with demo data.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="../public/screens/home-dark.webp" />
  <img src="../public/screens/home-light.webp" alt="The pt-tools user statistics page: upload, ratio, seeding, bonus points and class for every site" width="1440" />
</picture>

<details>
<summary>The site list and the phone layout</summary>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="../public/screens/sites-dark.webp" />
  <img src="../public/screens/sites-light.webp" alt="The site list: sign-in method, login checks, RSS feeds and inactivity warnings for each site" width="1440" />
</picture>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="../public/screens/home-mobile-dark.webp" />
  <img src="../public/screens/home-mobile-light.webp" alt="The user statistics page on a phone: totals at the top, then one card per site" width="300" />
</picture>

</details>

## Features

- **Sites and statistics**: 66 built-in sites on NexusPHP, mTorrent, Gazelle, HDDolby, Rousi and other platforms, signed in with a cookie, an API key or a passkey. One page shows upload, download, ratio, seeding, bonus points and class progress for every site; search several sites at once and push results straight to a downloader; get a reminder before an inactive account is disabled, and turn on daily check-in per site. A daily snapshot of every site gives daily, weekly and monthly increments and an optional daily report; see [Statistics and the daily report](https://firlab.app/pt-tools/en/guide/user-stats).
- **RSS downloads**: feeds are fetched on a schedule, and filter rules match titles with keywords, wildcards or regular expressions to follow TV series. A torrent still downloading when its free period ends is paused.
- **Seed boosting (brush)**: pick fresh torrents from a site's free list by discount, size, seeders and leechers, remove them by seeding time, ratio or upload speed, and cap concurrent downloads and size; earnings are tracked per day and included in the daily report. See [Brush tasks](https://firlab.app/pt-tools/en/guide/brush).
- **Downloaders**: add several qBittorrent and Transmission instances; a torrent goes to the one bound to its RSS feed, then its site, then the default. Free space is checked before every push, and old torrents are removed by seeding time, ratio and other conditions, with H&R torrents kept.
- **Notifications and ChatOps**: from QQ (OneBot) or Telegram, check status, pause or delete torrents and add or remove RSS feeds, with every command in the audit log. New-torrent notifications support quiet hours, digests, retries and an hourly limit. WeCom and generic webhooks are experimental outbound channels.
- **Browser extension**: [PT Tools Helper](../../tools/browser-extension/README.md) syncs a site's cookie in one click and collects redacted pages to request support for a new site.
- **Deployment and upkeep**: a Docker image (amd64 and arm64) and binaries for Linux and Windows; the binary upgrades itself from the web interface; logs, staged torrent files and old backups can be previewed, then removed; sites and downloaders can be reached through an HTTP, HTTPS or SOCKS5 proxy.

> [!WARNING]
> An RSS feed without any filter rule downloads **freeleech torrents only**. To follow a series or download particular non-free torrents, create a filter rule and turn off "free only" (仅免费) where needed.

## Install

### Docker Compose (recommended)

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

Save it as `compose.yml` and run `docker compose up -d`. The image is published on Docker Hub (`sunerpy/pt-tools`) and GHCR (`ghcr.io/sunerpy/pt-tools`), for amd64 and arm64.

> [!IMPORTANT]
> Keep `/app/.pt-tools` on a volume. It holds the database, the settings and the `secret.key` that encrypts site credentials; if the key is lost, saved cookies cannot be recovered.

### Install script

```bash
# Linux
curl -fsSL https://raw.githubusercontent.com/sunerpy/pt-tools/main/scripts/install.sh | sh
```

```powershell
# Windows PowerShell
irm https://raw.githubusercontent.com/sunerpy/pt-tools/main/scripts/install.ps1 | iex
```

The script downloads the latest archive together with the same release's `checksums.txt` and refuses to install when the SHA-256 does not match. Set `TOOL_VERSION` for a particular version (for example `TOOL_VERSION=v0.48.0`) and, on Linux, `TOOL_INSTALL_DIR` for the install folder (default `~/.local/bin`).

### Prebuilt binaries

Download from [Releases](https://github.com/sunerpy/pt-tools/releases). Every release includes `checksums.txt` and build provenance attestations; each release's notes show how to check them with `gh attestation verify`:

| OS      | Architecture | File                             |
| ------- | ------------ | -------------------------------- |
| Linux   | amd64        | `pt-tools-linux-amd64.tar.gz`    |
| Linux   | arm64        | `pt-tools-linux-arm64.tar.gz`    |
| Windows | amd64        | `pt-tools-windows-amd64.exe.zip` |
| Windows | arm64        | `pt-tools-windows-arm64.exe.zip` |

Unpack it and run `pt-tools web --host 0.0.0.0 --port 8080`. There is no macOS binary; on macOS, use Docker. For a systemd service, Windows and more Docker examples, see [Install](https://firlab.app/pt-tools/en/guide/install), the [binary guide](../../examples/binary-run.md) and the [Docker examples](../../examples/docker-run.md).

### Build from source

With Go `1.26.9`, Node.js `25.2.0` and pnpm `10.25.0`, run `make build-local`; see the [development guide](../development.md) (in Chinese).

## Quick start

1. Open <http://localhost:8080>, sign in with the initial account `admin` / `adminadmin`, and change the password straight away.
2. In Downloader settings (下载器设置), add qBittorrent or Transmission and check that it connects.
3. Sign in to your sites: sync cookies with [PT Tools Helper](../../tools/browser-extension/README.md), and enter API keys or passkeys as described in [Cookies and API keys](https://firlab.app/pt-tools/en/guide/get-cookie-apikey).
4. Add an RSS feed and choose its downloader, save folder and what happens when freeleech ends. Without filter rules it downloads freeleech torrents only.
5. Back up `secret.key`, and make sure the data folder is backed up elsewhere.

The [quick start](https://firlab.app/pt-tools/en/guide/quick-start) on the documentation site walks through each step.

## Usage

- **Web interface**: sites, RSS feeds, filter rules, downloaders, auto-delete and notifications are all set up in the web interface.
- **ChatOps**: QQ and Telegram accept `/help`, `/status`, `/tasks`, `/sites`, `/torrents`, `/pause`, `/resume`, `/delete`, `/addrss`, `/delrss`, `/bind` and `/unbind`. A channel's two user lists decide who can talk to the bot; every account linked with a binding code can run the management commands, so give binding codes only to people you trust. See the [ChatOps quick start](https://firlab.app/pt-tools/en/guide/chatops-quickstart).
- **Command line**: `pt-tools web` starts the service, `pt-tools secret` exports and imports the encryption key, and `pt-tools clean` previews and removes rotated logs, staged torrent files and old backups; see the [command line](https://firlab.app/pt-tools/en/reference/cli) reference.

## Docs

The documentation site **[firlab.app/pt-tools/en](https://firlab.app/pt-tools/en/)** ([中文](https://firlab.app/pt-tools/)) is built from this repository's `docs/` folder.

| Page                                                                                      | What it covers                                        |
| ----------------------------------------------------------------------------------------- | ----------------------------------------------------- |
| [Install](https://firlab.app/pt-tools/en/guide/install)                                   | Docker, binaries, systemd and the first sign-in       |
| [Configuration](https://firlab.app/pt-tools/en/configuration)                             | Environment variables, proxies, downloaders, storage  |
| [RSS subscriptions](https://firlab.app/pt-tools/en/guide/rss-subscription)                | Feeds, downloader choice, what happens when free ends |
| [Filter rules and TV series](https://firlab.app/pt-tools/en/guide/filter-rules-tv-series) | Keyword, wildcard and regular-expression rules        |
| [Auto-delete and disk protection](https://firlab.app/pt-tools/en/guide/auto-cleanup)      | Cleanup conditions, H&R protection, free-space checks |
| [Login monitoring](https://firlab.app/pt-tools/en/guide/site-login-monitoring)            | Login checks, inactivity reminders, key backups       |
| [ChatOps](https://firlab.app/pt-tools/en/guide/chatops-quickstart)                        | QQ, Telegram, binding and new-torrent notifications   |
| [Upgrades and backups](https://firlab.app/pt-tools/en/guide/upgrade)                      | Upgrading, backing up and restoring each install      |
| [Data and security](https://firlab.app/pt-tools/en/reference/security)                    | Where data lives, encryption, outbound connections    |
| [Supported sites](https://firlab.app/pt-tools/en/sites)                                   | The 66 built-in sites and how each signs in           |
| [Requesting a new site](https://firlab.app/pt-tools/en/guide/request-new-site)            | Collecting and submitting redacted site data          |
| [FAQ](https://firlab.app/pt-tools/en/faq)                                                 | Sign-in, RSS, downloader and database problems        |

Features, fixes and upgrade notes for each version are in [Releases](https://github.com/sunerpy/pt-tools/releases) and the [CHANGELOG](../../CHANGELOG.md) (in Chinese).

## Development

The repository pins Go `1.26.9`, Node.js `25.2.0` and pnpm `10.25.0`. After cloning, run the full local gate:

```bash
make check
```

Common targets:

```bash
make fmt             # format Go and the frontend
make lint            # Go and frontend lint plus type checks
make test            # frontend build and tests, Go race tests
make docs-check      # relative links and anchors in the repository's Markdown
make build-local     # build the frontend and a binary for this platform
make build-extension # check the site list and package the browser extension
```

Before contributing, read the [development guide](../development.md) (in Chinese) and `AGENTS.md` at the repository root. Commits and pull request titles follow Conventional Commits; a new site is added only as a definition under `site/v2/definitions/`, with fixture tests. The writing rules and publishing flow for the documentation site are in [docs/README.md](../README.md) (in Chinese).

## Community

- [GitHub Issues](https://github.com/sunerpy/pt-tools/issues)
- [GitHub Discussions](https://github.com/sunerpy/pt-tools/discussions)
- [Telegram](https://t.me/+7YK2kmWIX0s1Nzdl)
- QQ group: `274984594`
- WeChat Official Account: 六月水蓝 (scan the QR code below with WeChat to follow)

<img src="../public/community/wechat-official-account.jpg" alt="QR code of the WeChat Official Account 六月水蓝" width="160">

## License

[MIT License](../../LICENSE)

---

**Disclaimer**: this tool is for learning and research. Follow each site's rules, and judge the risks of automated access for yourself.
