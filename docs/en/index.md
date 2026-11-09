---
layout: home
markdownStyles: false
title: "pt-tools: automation for private trackers"
titleTemplate: false
description: RSS downloads, seed boosting, IYUU cross-seeding, following shows and organising your media library, statistics across sites and downloader management, in the web interface and the mobile app, with control from QQ and Telegram or through an AI assistant. Runs in Docker, on Linux and on Windows.

hero:
  name: pt-tools
  text: Automation for private trackers
  tagline: Download, brush and cross-seed with IYUU, follow shows into your media library, and keep every site's statistics and your qBittorrent and Transmission in one place, from the web, the app, QQ or Telegram.
  actions:
    - theme: brand
      text: Install
      link: /en/guide/install
    - theme: alt
      text: Quick start
      link: /en/guide/quick-start
    - theme: alt
      text: New in 1.0
      link: /en/guide/whats-new-1.0

home:
  facts:
    - term: Runs on
      text: Docker, Linux, Windows; amd64 and arm64
    - term: Your data
      text: On your own server; cookies encrypted
    - term: Docs version
      text: The 1.0 preview; stable is still 0.48.0

  stages:
    steps:
      - Follow a show, locked to one release
      - Each new episode is filed into the TV library
    screens:
      - src: /screens/media-follow-light.webp
        dark: /screens/media-follow-dark.webp
        width: 1280
        height: 1060
        alt: The title page of the series 长夜将尽 (The Long Night). The button reads 追剧中 · 第 2 季 (following season 2), and the 你的 NAS 上 (on your NAS) panel lists 12 episodes of season 1 and 6 of season 2 in the library, 4 torrents seeding, and the season 2 follow locked to the HHWEB 2160p release. The interface is in Chinese.
      - src: /screens/media-history-light.webp
        dark: /screens/media-history-dark.webp
        width: 1280
        height: 1060
        alt: The organise history (整理历史). The top entry is 长夜将尽 S02E07, matched by its subscription; The.Long.Night.S02E07.2160p.WEB-DL.H265-HHWEB.mkv from the torrent was hard-linked to /media/剧集/长夜将尽 (2025)/Season 02/长夜将尽 - S02E07 - 失踪者.mkv when the download finished, the scrape succeeded, and the entry is marked organised. Below it is the entry for episode 6. The interface is in Chinese.
    caption: Demo data. 长夜将尽 (The Long Night) is a made-up series, and its poster and backdrop are generated. The two pictures are the title page and the organise history, both in the web interface's narrow layout. A followed show is searched on your sites as each episode airs, keeping to one release; when the download finishes, it is hard-linked into the library by your naming template, with NFO files and artwork.

  capabilities:
    title: What pt-tools does
    intro: Features marked Preview ship in the 1.0 preview releases and are not in the 0.48.0 stable release yet; see What's new in the 1.0 preview. Each card's title opens its guide.
    items:
      - icon: folder-open
        title: Following shows and filing them
        wide: true
        status: preview
        body: Subscribe to a film or follow a season. Each episode is searched on your sites as it airs, keeping to one release, and upgrades swap in better ones; finished downloads are hard-linked into your library by a naming template, with NFO files and artwork, and Emby, Jellyfin or Plex is asked to scan. One page per title shows the episodes in your library and whether the torrents seeding can be deleted, and the calendar lists the updates day by day.
        evidence:
          kind: code
          text: The Long Night (2025)/Season 02/The Long Night - S02E07 - The Missing.mkv
        link: /en/guide/media-subscribe
      - icon: globe
        title: Sites and statistics
        status: available
        body: 67 built-in sites. The statistics page totals each site's upload, ratio, seeding and progress to the next class; you can search across sites and hear about a site before an inactive account is disabled.
        evidence:
          kind: chips
          items: [Cookie, API key, Passkey]
        link: /en/sites
      - icon: rss
        title: Automatic RSS downloads
        status: available
        body: Feeds are fetched on a schedule, and a feed without filter rules downloads freeleech torrents only. Rules can limit the size, and a torrent still downloading when its free period ends is paused or deleted, as its subscription chooses.
        evidence:
          kind: chips
          items: [Keywords, Wildcards, Regular expressions]
        link: /en/guide/rss-subscription
      - icon: trending-up
        title: Brushing and cross-seeding
        status: preview
        body: Brush tasks pick fresh freeleech torrents from a site's list by your conditions and remove them by seeding time, ratio and upload speed. IYUU cross-seeding adds torrents with the same data on other sites to the same downloader, and they seed only after a 100% check.
        evidence:
          kind: code
          text: "list 50, matching 4, added 2"
        link: /en/guide/brush
      - icon: server
        title: Downloaders
        status: available
        body: Add several qBittorrent and Transmission instances; a torrent goes to the one bound to its RSS feed, then its site, then the default. Free space and the site's seeding limit are checked before every push, and auto-delete keeps H&R torrents.
        evidence:
          kind: chips
          items: [qBittorrent, Transmission]
        link: /en/configuration
      - icon: smartphone
        title: Mobile app and remote access
        status: preview
        body: The Android app shows the overview, torrents, sites and subscriptions away from home. Pair it by scanning a QR code; it tries a direct connection first and falls back to a relay. The connection is end-to-end encrypted, no port has to be opened to the internet, and you can run the relay yourself.
        evidence:
          kind: code
          text: pt-tools relay serve
        link: /en/guide/mobile-app
      - icon: sparkles
        title: AI assistant and MCP
        wide: true
        status: preview
        body: Connect an OpenAI-compatible model and ask in plain language, in the web interface, the mobile app, Telegram or QQ, to look up data, search and push torrents and manage subscriptions; MCP clients such as Claude and Cursor can connect too. Every change waits for your confirmation.
        evidence:
          kind: chips
          items:
            [DeepSeek, Qwen, Kimi, SiliconFlow, Google AI Studio, Groq, OpenRouter, Ollama, OpenAI]
        link: /en/guide/assistant
      - icon: bell
        title: QQ, Telegram and notifications
        status: available
        body: Check status and torrents from QQ or Telegram, pause, resume or delete torrents and add RSS feeds, with every command recorded in the audit log. New-torrent notifications support quiet hours, digests and an hourly limit.
        evidence:
          kind: chips
          items: [/status, /tasks, /torrents, /addrss]
        link: /en/guide/chatops-quickstart
      - icon: house
        title: Home page and NAS monitoring
        status: preview
        body: The home page puts today's episodes, what needs attention, downloaders and storage in one place. The NAS overview reads CPU, memory, network and storage from the local host, TrueNAS or Glances, with Synology and fnOS as experimental sources.
        evidence:
          kind: chips
          items: [Local, TrueNAS, Glances, Synology, fnOS]
        link: /en/guide/home-and-health
      - icon: archive
        title: Upgrades and backups
        status: preview
        body: The database is backed up every day, and once more before the first start on a new version; you can back up, download and restore from the web interface. Binary installs also upgrade from the web interface.
        evidence:
          kind: code
          text: pt-tools db backup
        link: /en/guide/upgrade#backing-up

  showcases:
    - title: Brush tasks
      intro: Pick fresh freeleech torrents from a site's list by your conditions, seed them, and remove them by seeding time, ratio and upload speed. Each run records what it read and why each torrent was skipped.
      screen:
        src: /screens/brush-light.webp
        dark: /screens/brush-dark.webp
        width: 1764
        height: 980
        alt: The brush tasks page (刷流任务). Along the top are 11 torrents seeding, 87.7 GB uploaded and 29.82 GB downloaded today, and a total ratio of 2.97. Below are two tasks, HDSky 免费刷流 and OurBits 刷流, each with its downloader, torrents in progress, today's upload and download, total upload and last run. Bottom left is a bar chart of the last 30 days' earnings, bottom right a note on how brushing works. The interface is in Chinese.
      caption: Demo data. The brush tasks page in the web interface.
      points:
        - title: Freeleech only by default
          body: Only free and 2x free torrents are taken and H&R is excluded; when a site has site-wide H&R, every torrent from it counts as H&R.
        - title: The same checks as RSS
          body: Disk reserve and the site's seeding capacity are checked before a torrent is added, and removal only touches torrents the task added.
        - title: Earnings by period
          body: Upload and download by day, week, month, quarter or year, with one bar per day for the last 30 days.
      guide:
        text: Brush tasks
        link: /en/guide/brush
    - title: Mobile app
      intro: Once paired by QR code, the Android app shows the overview, torrents, sites and subscriptions away from home, and pauses, deletes and pushes torrents or manages subscriptions within the permissions given at pairing.
      screen:
        src: /screens/app-overview-en-light.webp
        dark: /screens/app-overview-en-dark.webp
        width: 780
        height: 1688
        alt: "The app's overview page: 10.5 TiB uploaded, 2.0 TiB downloaded (billed), a ratio of 5.25, 4,567.8 bonus points, 8 torrents seeding at 1.0 TiB and 2 unread messages; 1.0 GiB uploaded today; the downloaders seed 8 torrents at 1.0 TiB, 3 on qb-nas and 5 on tr-nas."
      caption: Demo data. The app's Overview page.
      points:
        - title: No open port
          body: The app tries a direct connection first and falls back to a relay; the connection is end-to-end encrypted, and the relay only forwards data it cannot read.
        - title: Permissions per device
          body: Choose Full control or Read only when pairing, then change the permission or revoke the device at any time.
        - title: Released with the server
          body: The app's installers ship in the same GitHub release as the server.
      guide:
        text: Mobile app
        link: /en/guide/mobile-app
    - title: AI assistant
      intro: Connect an OpenAI-compatible model and ask pt-tools in plain language which episode a show is up to or how much you uploaded today, or have it search and push torrents and add subscriptions.
      screen:
        src: /screens/assistant-light.webp
        dark: /screens/assistant-dark.webp
        width: 1152
        height: 822
        alt: A conversation with the AI assistant. Asked which episode of season 2 of 长夜将尽 the follow is up to and whether tonight's episode 7 has downloaded, the assistant called 2 tools and answered that episode 7 has downloaded and been hard-linked into the TV library, episodes 1 to 6 were already there, and episode 8 has not aired yet. Below is the message box, with the model deepseek-v4-flash selected. The interface is in Chinese.
      caption: Demo data. The assistant called two tools, for the follow and the organise history.
      points:
        - title: Changes wait for you
          body: Pushes, deletions and subscription changes first show a confirmation card with each value before and after and the possible effects; nothing runs until you confirm.
        - title: Credentials stay out
          body: Cookies, passkeys and passwords are never sent to the model, and the data its tools return is redacted first.
        - title: Ask from anywhere
          body: Chat in the web interface or the mobile app, or message the assistant through the Telegram and QQ bots.
      guide:
        text: AI assistant
        link: /en/guide/assistant

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
    guide:
      text: Install guide
      link: /en/guide/install

  privacy:
    title: Where your data goes
    intro: The core features of pt-tools depend on no hosted service. These are the things it connects to; apart from update checks on GitHub, each one is contacted only after you enable or configure it.
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
      - name: AI model provider
        sends: The conversation and the data its tools return
        detail: Contacted only once the AI assistant is enabled, and only the provider you chose; cookies, passkeys and passwords are never given to the model.
      - name: Remote access relay
        sends: The app's encrypted connection
        detail: Contacted only once remote access is turned on. With no relay set, the built-in hosted relay is used; it only forwards end-to-end encrypted data and cannot read it. You can run your own relay or connect directly only.
      - name: Media and cross-seeding services
        sends: Titles, TMDB IDs and torrent info hashes
        detail: TMDB and Douban's public data are contacted when you use the media features; with IYUU cross-seeding on, the info hashes of finished torrents in your downloaders are sent to IYUU for lookup; CookieCloud only ever means your own server.

  faq:
    title: Questions
    items:
      - q: How do I get the features marked Preview?
        a: They are in the 1.0 preview releases (v1.0.0-rc.N), not in the 0.48.0 stable release yet. Install a preview release as the install guide describes; previews are for testing, so back up the data folder before you upgrade.
        guide:
          text: Installing a preview release
          link: /en/guide/install#installing-a-preview-release
      - q: Will the AI make changes on its own?
        a: No. The built-in AI assistant asks you to confirm every change before it runs. An MCP write tool first returns only a preview; once you agree in your client, the client's AI calls it again with `confirm=true` to run it, and a token without the MCP write scope cannot use these tools at all.
        guide:
          text: MCP tools
          link: /en/guide/mcp#tools
      - q: Can MCP upgrade or restart pt-tools?
        a: No. No MCP tool upgrades, restarts or otherwise interrupts the service, and the update check only reports new versions. Changing the configuration needs the token's separate Config scope, and it too returns a preview first and saves only after you confirm.
        guide:
          text: MCP
          link: /en/guide/mcp
      - q: Does it work with MoviePilot, Sonarr or Seerr?
        a: Yes, through two compatible entrances in the 1.0 preview. The qB-compatible entrance lets tools that only talk to qBittorrent treat pt-tools as one, and the torrents they add still pass the disk space and site seeding capacity checks. The Servarr-compatible entrance lets Seerr treat pt-tools as Radarr and Sonarr, turning requested films and series into subscriptions and follows.
        guide:
          text: qB-compatible entrance
          link: /en/guide/qbit-compat
      - q: Do I have to open a port to reach home from my phone?
        a: No. The app tries a direct connection first and falls back to a relay; the connection is end-to-end encrypted, and the relay only forwards data it cannot read. With no relay set, the built-in hosted relay is used; you can also run your own or connect directly only.
        guide:
          text: Remote access
          link: /en/guide/remote-access
      - q: My site is not in the list. What can I do?
        a: Collect redacted pages with the browser extension and request the new site by following the steps; no code is needed.
        guide:
          text: Request a new site
          link: /en/guide/request-new-site

  closer:
    title: Install pt-tools
    body: A Docker image, and binaries for Linux and Windows, under the MIT License. Report problems and suggest features in [GitHub Issues](https://github.com/sunerpy/pt-tools/issues).
    actions:
      - text: Install
        link: /en/guide/install
        theme: brand
        icon: download
      - text: Quick start
        link: /en/guide/quick-start
        theme: alt
        icon: arrow-right
---

<HomeCapabilities />

<HomeShowcases />

<HomeDeploy>

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

The install scripts download the latest release and check it against the same release's `checksums.txt` before unpacking.

The commands above install the 0.48.0 stable release. To use the features marked Preview, install a preview release as described in [Installing a preview release](/en/guide/install#installing-a-preview-release); [What's new in the 1.0 preview](/en/guide/whats-new-1.0) lists what it adds.

</HomeDeploy>

<HomePrivacy />

<HomeFaq />

<HomeCloser>

- Bug reports and feature requests: [GitHub Issues](https://github.com/sunerpy/pt-tools/issues)
- Questions and discussion: [GitHub Discussions](https://github.com/sunerpy/pt-tools/discussions), the [Telegram group](https://t.me/+7YK2kmWIX0s1Nzdl), or QQ group 274984594
- WeChat Official Account: 六月水蓝

<QrCode src="/community/wechat-official-account.jpg" alt="QR code of the WeChat Official Account 六月水蓝" caption="Scan with WeChat to follow" />

</HomeCloser>
