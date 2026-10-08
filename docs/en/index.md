---
layout: home
title: "pt-tools: automation for private trackers"
titleTemplate: false
description: RSS downloads, statistics and search across sites, downloader management and ChatOps, configured in one web interface. Runs in Docker, on Linux and on Windows.

hero:
  name: pt-tools
  text: Automation for private trackers
  tagline: Download freeleech torrents from your RSS feeds, follow every site's statistics and login status, manage qBittorrent and Transmission in one place, and check on all of it from QQ or Telegram.
  actions:
    - theme: brand
      text: Install
      link: /en/guide/install
    - theme: alt
      text: Quick start
      link: /en/guide/quick-start
    - theme: alt
      text: GitHub
      link: https://github.com/sunerpy/pt-tools

home:
  facts:
    - term: Runs on
      text: A Docker image (amd64 and arm64), and binaries for Linux and Windows. On macOS, use Docker.
    - term: Your data
      text: Settings, history and site credentials stay on your own server; cookies are encrypted with a key generated on that machine.

  visual:
    desktop:
      light: /screens/home-light.webp
      dark: /screens/home-dark.webp
      width: 1440
      height: 900
      alt: The pt-tools statistics page, with each site's upload, ratio, seeding count, bonus points and class. The interface is in Chinese.
    mobile:
      light: /screens/home-mobile-light.webp
      dark: /screens/home-mobile-dark.webp
      width: 375
      height: 812
      alt: The statistics page on a phone, with the totals at the top and one card per site.

  index:
    title: What pt-tools does
    intro: Everything at a glance. Features marked Experimental ship in the release but have not been verified end to end.
    groups:
      - name: Sites and statistics
        items:
          - title: 66 built-in sites
            body: NexusPHP, mTorrent, Gazelle, HDDolby and Rousi sites, signing in with a cookie, an API key or a passkey.
            status: available
            link: /en/sites
          - title: Statistics across sites
            body: Upload, download, ratio, seeding, bonus points and progress to the next class for every site on one page, with increments and trends for today, this week and this month, also exportable as an image to share.
            status: available
            link: /en/guide/user-stats
          - title: Daily report
            body: Each day at a set time, sends every site's upload, download and bonus increments and any login problems to the channels you choose. Off by default.
            status: available
            link: /en/guide/user-stats#daily-report
          - title: Search across sites
            body: Search several sites at once, filter and sort by size, seeders and freeleech, and send results straight to a downloader.
            status: available
            link: /en/guide/what-is-pt-tools
          - title: Login status monitoring
            body: Per-site checks and reminders, so you hear about a site before an inactive account is disabled.
            status: available
            link: /en/guide/site-login-monitoring
          - title: CookieCloud import
            body: Fetch the cookies your browser syncs to a self-hosted CookieCloud server, decrypt them on this machine and write them into the sites after a preview; scheduled sync is optional.
            status: available
            link: /en/guide/cookiecloud
      - name: Automatic RSS downloads
        items:
          - title: RSS subscriptions
            body: Feeds are fetched on a schedule. A feed without filter rules downloads freeleech torrents only.
            status: available
            link: /en/guide/rss-subscription
          - title: Filter rules and TV series
            body: Match titles with keywords, wildcards or regular expressions, limited by size and by whether a torrent must be free.
            status: available
            link: /en/guide/filter-rules-tv-series
          - title: Brush tasks
            body: Pick fresh torrents from a site's free list by your conditions, seed them, and remove them by seeding time, ratio or upload speed, within limits on concurrent downloads and size. Off by default.
            status: available
            link: /en/guide/brush
          - title: Pause when freeleech ends
            body: A torrent still downloading when its free period ends is paused, so it does not count against your download.
            status: available
            link: /en/configuration
          - title: Disk protection and auto-delete
            body: Free space on the downloader is checked before every push; old torrents are removed by seeding time, ratio and other conditions, with H&R torrents kept.
            status: available
            link: /en/guide/auto-cleanup
      - name: Downloaders
        items:
          - title: qBittorrent and Transmission
            body: Add several downloaders; a torrent goes to the one bound to its RSS feed, then its site, then the default.
            status: available
            link: /en/configuration
          - title: Downloader web UI
            body: Every downloader's torrents, speeds and states in pt-tools, with bulk pause, resume and delete.
            status: available
            link: /en/guide/what-is-pt-tools
          - title: Download folders
            body: Keep several save folders per downloader and choose one when you push a torrent.
            status: available
            link: /en/configuration
          - title: Downloader assistant
            body: Find dead torrents a site has deleted, tag torrents with their site by tracker and replace tracker addresses in bulk, each previewed first.
            status: available
            link: /en/guide/downloader-assistant
          - title: Torrent transfer
            body: Move finished torrents to another downloader and keep seeding without touching the data; the source copy goes only after a 100% check, and rules can do it on a schedule.
            status: available
            link: /en/guide/torrent-transfer
          - title: IYUU cross-seeding
            body: Find torrents with the same data on other sites through IYUU, check their files and add them to the same downloader; they seed only after a 100% check.
            status: available
            link: /en/guide/reseed
      - name: Media
        items:
          - title: Subscriptions
            body: Subscribe to a film or a season, search your sites on a schedule and pick it up from RSS; quality profiles choose the release, missing episodes are filled in, upgrades swap in better releases, and a Douban wish list subscribes for you.
            status: available
            link: /en/guide/media-subscribe
          - title: Library organising
            body: Hard-link finished movies and shows into your library with a naming template, write NFO files and artwork, ask Emby, Jellyfin or Plex to scan, and send a notification.
            status: available
            link: /en/guide/media-library
          - title: Media recognition
            body: Parse torrent titles and find the movie or TV show on TMDB; IMDb and Douban IDs from sites are recorded, and wrong matches can be corrected by hand or with recognition words.
            status: available
            link: /en/guide/media-recognize
      - name: Notifications and ChatOps
        items:
          - title: Commands in QQ and Telegram
            body: Check status, tasks and torrents, pause, resume or delete torrents, add and remove RSS feeds; every command is recorded in the audit log.
            status: available
            link: /en/guide/chatops-quickstart
          - title: New-torrent notifications
            body: Push results sent to your chat app, with quiet hours, digests, retries and an hourly limit.
            status: available
            link: /en/guide/chatops-rss-notify
          - title: More notification channels
            body: WeCom, DingTalk and Feishu group bots, plus Bark, ServerChan and ntfy; they send notifications and do not take commands.
            status: available
            link: /en/guide/notify-channels
          - title: Generic webhook
            body: Sends notifications as JSON to your own system (HMAC-signed), for n8n, Zapier and the like.
            status: experimental
            link: /en/guide/chatops-quickstart
      - name: Running it
        items:
          - title: Browser extension
            body: PT Tools Helper syncs a site's cookie in one click and collects redacted pages to request support for a new site.
            status: available
            link: /en/guide/browser-extension
          - title: Upgrades and cleanup
            body: The binary upgrades itself from the web interface; logs, staged torrent files and old backups can be previewed, then removed.
            status: available
            link: /en/guide/upgrade
          - title: Proxies
            body: Reach sites and downloaders through an HTTP, HTTPS or SOCKS5 proxy, with exclusions by address.
            status: available
            link: /en/configuration

  steps:
    title: From install to the first automatic download
    items:
      - title: Install
        command: docker compose up -d
        body: Start it with Docker Compose and keep /app/.pt-tools on a volume, or run the binary directly.
      - title: Add a downloader
        body: In Downloader settings (下载器设置), add qBittorrent or Transmission and check that it connects.
      - title: Sign in to your sites
        body: Sync a cookie with the browser extension, import cookies from CookieCloud, or enter the site's API key or passkey.
      - title: Subscribe to RSS
        body: Add an RSS feed to a site and choose a downloader. Without filter rules, only freeleech torrents are downloaded.

  rules:
    columns: [Rule, Type, Matches]
    rows:
      - pattern: 2160p|4K
        kind: Regular expression
        matches: Titles containing 2160p or 4K
      - pattern: "*REMUX*"
        kind: Wildcard
        matches: Titles with REMUX anywhere
      - pattern: Black.Mirror
        kind: Keyword
        matches: Release names containing Black.Mirror
    caption: Rules are tried in order of priority and the first match wins; each rule can also limit the size and require freeleech.

  commands:
    items:
      - command: /status
        body: Speeds, disk space and active tasks
      - command: /tasks
        body: RSS tasks and how they are running
      - command: /torrents
        body: Torrents per downloader, a page at a time
      - command: /pause <hash>
        body: Pause a torrent; a hash prefix is enough
      - command: /addrss
        body: Add an RSS feed step by step
    caption: Only accounts linked with a binding code can send commands, and at present every linked account has admin rights, so give binding codes only to people you trust. The allow lists of a QQ or Telegram channel only decide who may talk to the bot, not who has admin rights; each account is limited to 10 commands a minute by default.

  shots:
    sites:
      light: /screens/sites-light.webp
      dark: /screens/sites-dark.webp
      width: 1440
      height: 900
      alt: The site list, with each site's status, sign-in method, login status and days left before the inactivity limit. The interface is in Chinese.

  deploy:
    title: Ways to run it
    intro: Every option runs the same program with the same data folder, so you can move between them.
    columns: [Option, Architectures, Where to get it, Data folder]
    rows:
      - name: Docker
        status: available
        cells:
          - amd64 and arm64
          - "sunerpy/pt-tools on Docker Hub, or ghcr.io/sunerpy/pt-tools with a build attestation"
          - Mounted at /app/.pt-tools
      - name: Linux
        status: available
        cells:
          - amd64 and arm64
          - "The release archive, or an install script that checks SHA-256"
          - ~/.pt-tools
      - name: Windows
        status: available
        cells:
          - amd64 and arm64
          - "The release archive, or a PowerShell install script"
          - .pt-tools in your user folder
    note: There is no macOS binary; use Docker on macOS. pt-tools listens on port 8080 by default.

  privacy:
    title: Where your data goes
    intro: pt-tools depends on no hosted service. These are the only things it connects to, and you configure every one of them.
    sendsLabel: Sends
    modes:
      - name: Private trackers
        sends: A cookie, API key or passkey
        detail: Fetching feeds, searching, reading statistics and checking login status only ever contact the sites you have added.
      - name: Downloaders
        sends: Torrent files and commands
        detail: Pushing torrents and reading tasks and free space only contact the qBittorrent and Transmission instances you configure.
      - name: Notification channels and GitHub
        sends: Notifications and version checks
        detail: Notifications go only to the channels you enable; checking for updates and upgrading the binary reach GitHub Releases.

  roadmap:
    title: Roadmap
    intro: Designed but not released. The design documents record contracts; they do not mean a feature is available.
    items:
      - title: MCP server
        body: Queries and confirmed actions for AI assistants over the Model Context Protocol. Only the interface contract exists today; there is no server.
        status: planned
      - title: AI agent
        body: Multi-step tasks on top of ChatOps, with every write confirmed by you.
        status: planned
      - title: Verifying WeCom and generic webhooks
        body: Both outbound channels are implemented; they leave Experimental once they have been verified end to end.
        status: planned
    notPlanned:
      title: Deliberately not built
      items:
        - A chat bot or language model inside pt-tools, or calls to a language model service
        - An AI mode that writes without asking you first
        - Upgrades, restarts or other destructive actions exposed over MCP
---

<HomeIndex />

<HomeSteps />

<SplitBlock proof="rules">

## RSS downloads that fetch only what you want

pt-tools fetches every RSS feed on a schedule and pushes the matching torrents to a downloader. A feed with no filter rules downloads freeleech torrents only, which suits building ratio; once rules are attached, only matching torrents are downloaded, which suits following a series or collecting particular releases.

Before a push it checks the downloader's free space and the site's seeding limit, and a torrent still downloading when its free period ends is paused.

[RSS subscriptions](/en/guide/rss-subscription) · [Filter rules and TV series](/en/guide/filter-rules-tv-series) · [Auto-delete and disk protection](/en/guide/auto-cleanup)

</SplitBlock>

<SplitBlock proof="screen" shot="sites" flip>

## Every site's numbers on one page

The statistics page totals each site's upload, ratio, seeding and progress to the next class; the site list shows how you sign in to each site, whether that still works, and how many days are left before the site's inactivity limit.

Cookie sites sync in one click with the browser extension, or import from CookieCloud; sites such as M-Team use an API key, and Rousi Pro uses a passkey.

[Supported sites](/en/sites) · [Cookies and API keys](/en/guide/get-cookie-apikey) · [CookieCloud import](/en/guide/cookiecloud) · [Login status and key backup](/en/guide/site-login-monitoring)

</SplitBlock>

<SplitBlock proof="commands">

## Check on it from QQ and Telegram

Send commands in a private chat over QQ (through OneBot, for example NapCat) or a Telegram bot to see status and torrents and to pause or resume them; RSS push results arrive in the same chat.

A chat account is linked first with a one-time code generated in the web interface. Every command is recorded in the audit log, which you can search in the web interface.

[ChatOps quick start](/en/guide/chatops-quickstart) · [New-torrent notifications](/en/guide/chatops-rss-notify)

</SplitBlock>

<HomeDeploy />

<HomePrivacy />

## Install

Docker Compose is the recommended way. Keep the data folder `/app/.pt-tools` on a volume: its `secret.key` is what decrypts your saved site credentials.

::: code-group

```yaml [Docker Compose]
services:
  pt-tools:
    image: sunerpy/pt-tools:latest
    container_name: pt-tools
    environment:
      TZ: Asia/Shanghai
    ports:
      - "8080:8080"
    volumes:
      - ./data:/app/.pt-tools
    restart: unless-stopped
```

```bash [Linux]
curl -fsSL https://raw.githubusercontent.com/sunerpy/pt-tools/main/scripts/install.sh | sh
```

```powershell [Windows]
irm https://raw.githubusercontent.com/sunerpy/pt-tools/main/scripts/install.ps1 | iex
```

:::

The install scripts download the latest release and check it against the same release's `checksums.txt` before unpacking. The [install guide](/en/guide/install) covers each option in full, including how to pin a version.

<HomeRoadmap />

## Community

- Bug reports and feature requests: [GitHub Issues](https://github.com/sunerpy/pt-tools/issues)
- Questions and discussion: [GitHub Discussions](https://github.com/sunerpy/pt-tools/discussions), the [Telegram group](https://t.me/+7YK2kmWIX0s1Nzdl), or QQ group 274984594
- WeChat Official Account: 六月水蓝

<QrCode src="/community/wechat-official-account.jpg" alt="QR code of the WeChat Official Account 六月水蓝" caption="Scan with WeChat to follow" />
