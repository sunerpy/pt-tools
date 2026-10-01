# Quick start

This page walks through the first setup in order: sign in, add a downloader, sign in to your sites, subscribe to an RSS feed and confirm the first automatic download. If pt-tools is not installed yet, start with [Install](install.md).

## 1. Sign in and change the password

Open `http://your-server:8080`, sign in with the initial account (see "Signing in for the first time" in [Install](install.md)), then change the password under System → Change password (系统 → 修改密码).

## 2. Add a downloader

1. Open Downloads → Downloader settings (下载 → 下载器设置) and choose Add downloader (添加下载器).
2. Choose qBittorrent or Transmission and enter its web UI address, user name and password.
3. After saving, the Connectivity (连通性) column of the list shows whether pt-tools can reach it; the check button on the row runs the check again.
4. With more than one downloader, click the star to make your usual one the default.

Download folders, disk protection and the other options are described in [Configuration](../configuration.md).

## 3. Sign in to your sites

Open Sites → Site list (站点 → 站点列表), choose a site and fill in its credentials on the Credentials (凭据) tab. Sites sign in in different ways:

- **Cookie**: most NexusPHP sites. The [PT Tools Helper browser extension](browser-extension.md) syncs the cookie in one click once you are signed in to the site.
- **API key**: M-Team, for example.
- **Passkey**: Rousi Pro, for example.

Where to find them is explained in [Cookies and API keys](get-cookie-apikey.md). After saving, the Status (状态) column of the site list shows whether the site can be reached.

## 4. Subscribe to RSS

1. On the site's page, open the RSS subscriptions (RSS 订阅) tab and add a feed.
2. Enter a name and the RSS link and choose a downloader; you can also choose a download folder, a check interval and filter rules.
3. After saving, the feed is fetched at that interval in the background.

> [!WARNING]
> A feed with no filter rules attached **downloads freeleech torrents only**. To follow a series or download torrents that are not free, create a filter rule first and turn off its Free only (仅免费) option if needed.

How to find a site's RSS link and what each option does is covered in [RSS subscriptions](rss-subscription.md).

## 5. Add filter rules if you need them

Open Rules → Filter rules (规则 → 过滤规则), choose Add rule (添加规则), enter a keyword, wildcard or regular expression, then attach the rule to the RSS feed. The syntax and examples for TV series are in [Filter rules and TV series](filter-rules-tv-series.md). Downloading freeleech only needs no rules.

## 6. Confirm the first automatic download

- Downloads → Task list (任务列表) lists every torrent that was pushed, with its freeleech end time and push status.
- Downloads → Downloader web UI (下载器 Web UI) shows each torrent's progress in the downloader itself.
- When a push fails, or nothing matched, System → Logs (运行日志) records the reason.

## 7. Back up the key

Copy `secret.key` from the data folder, together with the database, to another device. Without the key, saved cookies cannot be decrypted. Backing up and restoring are covered in [Upgrades and backups](upgrade.md).

## What next

- Get notifications and send commands from a chat app: [ChatOps quick start](chatops-quickstart.md)
- Remove old torrents automatically when disk space runs low: [Auto-delete and disk protection](auto-cleanup.md)
- Get a reminder before an inactive account is disabled: [Login status and key backup](site-login-monitoring.md)
