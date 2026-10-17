# What's new in the 1.0 preview <Badge type="tip" text="1.0 preview" />

The 1.0 preview (versions such as `1.0.0-rc.N`) adds seed boosting (brush tasks), IYUU cross-seeding, media library organising, the mobile app, remote access, the AI assistant and more on top of 0.48.0. This site documents the preview: pages marked "1.0 preview" and features marked "Preview" on the home page are only in the preview releases, not in the 0.48.0 stable release; the 1.0 stable release will include all of them.

The preview releases are for testing and are not recommended for production yet. See [Installing a preview release](./install.md#installing-a-preview-release) to install one, and back up the data folder before upgrading. The changes in each preview are listed under the Pre-release versions on [GitHub Releases](https://github.com/sunerpy/pt-tools/releases).

## Brushing and seeding

- [Seed boosting](./brush.md): pick new torrents from a site's free list by your conditions, delete them by seeding time, ratio, upload speed and other rules, and cap concurrent downloads and size; torrents with H&R follow the site's rules or the seeding requirement set in the site settings.
- [IYUU cross-seeding](./reseed.md): find torrents with the same data on other sites through IYUU, check the files, add them to the same downloader and start seeding only after verification reaches 100%; the data is stored once.
- [Torrent transfer](./torrent-transfer.md): move finished torrents to another downloader to keep seeding without touching the data files; a torrent leaves the source downloader only after verification reaches 100%, and transfers can run on a schedule by rules.
- [Downloader assistant](./downloader-assistant.md): find torrents the site has deleted, add missing site labels, and replace tracker addresses in bulk, each previewed before it runs.
- [qB-compatible entrance](./qbit-compat.md): tools that only talk to qBittorrent, such as MoviePilot, autobrr, Sonarr and Radarr, can treat pt-tools as one; the torrents they add still pass the disk space and site seeding capacity checks.

## Media library

- [Subscriptions](./media-subscribe.md): subscribe to a movie or one season of a series; pt-tools searches the sites on a schedule and also downloads from RSS, picks releases by quality profile, fills in missing episodes, upgrades to better versions, and turns Douban "want to watch" items into subscriptions.
- [Media library organising](./media-library.md): finished movies and series are hard-linked into the library by naming templates, with NFO files and posters, and Emby, Jellyfin or Plex is told to scan; second-level categories, replacing older versions with the same name, watched folders, per-item scraping switches and Douban plots and ratings are available.
- [Media recognition](./media-recognize.md): parse torrent titles and find the movie or series on TMDB, keep the IMDb and Douban IDs the site provides, correct wrong matches or add recognition words; the TMDB API and image addresses can be changed.

## Mobile app and APIs

- [Mobile app](./mobile-app.md): the Android app shows the overview, torrents, sites and subscriptions away from home, pauses, deletes and pushes torrents or manages subscriptions within the permissions given at pairing, and talks to the AI assistant; its packages are published in the same Release as each version.
- [Remote access](./remote-access.md): pair by scanning the QR code in the web UI; the app tries a direct connection first and falls back to a relay. The connection is end-to-end encrypted and the relay only forwards. The built-in hosted relay is used by default; you can run your own relay or turn relays off and connect directly only.
- [API tokens](./api-tokens.md) and the [App API](../reference/app-api.md): scripts and clients reach `/api/app/v1/` with a token, reading or changing data within its scopes, without storing the web password.
- [MCP](./mcp.md): AI assistants in MCP clients such as Claude, Cursor and Cherry Studio can look at torrents, downloaders, site data and subscriptions, and make changes after you confirm them.

## Notifications and the AI assistant

- [AI assistant](./assistant.md): connect an OpenAI-compatible model (Groq and Google AI Studio are supported too) and ask in plain language in the web UI, the mobile app, Telegram or QQ to look up data, search and push torrents, or add subscriptions; every change needs your confirmation.
- [More notification channels](./notify-channels.md): DingTalk and Feishu group bots, Bark, ServerChan and ntfy; "Recent notifications" lists RSS and background notifications, and delivery statistics show each source.
- [Message templates](./notify-channels.md#message-templates): customize the title and body of each notification event; command replies and AI assistant replies can use templates too.
- [Custom webhook](./notify-channels.md#custom-webhook): set the request method, format and body template, start from presets such as Discord, Slack and Gotify, and judge success by the response content.

## Sites and data

- [Statistics and daily report](./user-stats.md): a daily snapshot of every site gives increments and trends for today, this week and this month, and the day's increments can be sent to a notification channel each day.
- [Daily sign-in](./site-login-monitoring.md#daily-sign-in): sign in to sites that support it within a time window, or with the `/signin` command; sites such as HDArea use their own sign-in endpoints.
- [CookieCloud import](./cookiecloud.md): fetch the cookies your browser synced to your own CookieCloud server, decrypt them locally and write them to sites after a preview, or sync on a schedule; the browser extension can sync cookies on a schedule and upload them to CookieCloud encrypted.
- Site parsing: bonus, ratio and bonus per hour for CrabPT, GameGamePT, HDArea and OpenCD, and the ratio of many sites when it is below 1, are fixed; sites without an H&R seeding time in their rules can have one set in the site settings.

## Reliability and security

- [Free status](../configuration.md#global-settings): pushes that require free torrents use the free status read in the same run; an unknown status does not count as free, and a torrent without enough free time left is not pushed. A torrent whose free end time is unknown is not started automatically, and unfinished torrents can be paused or deleted a few minutes before the free period ends.
- [Auto-delete](./auto-cleanup.md): preview which torrents a run would delete, and see the result of the last run on the page; torrents with H&R whose seeding requirement is unknown are protected.
- Credentials: site cookies are stored only encrypted, and plaintext left by older versions is removed on upgrade; a damaged key file is no longer overwritten, and pt-tools refuses to start when the key does not match the stored ciphertext.
- Upgrades: self-upgrade checks the downloaded file against the same version's `checksums.txt`; update checks no longer report older previews as new versions.
