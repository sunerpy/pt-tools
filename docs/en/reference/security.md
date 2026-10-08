# Data and security

This page explains where pt-tools keeps its data, what it encrypts, which addresses it connects to, and what to keep in mind when you deploy it.

## Where the data is kept

Everything is in the data folder: the folder mounted at `/app/.pt-tools` in Docker, or `~/.pt-tools` of the user running the binary. pt-tools uses no external database and no hosted service.

| File or folder | Contents                                                                    |
| -------------- | --------------------------------------------------------------------------- |
| `torrents.db`  | SQLite database: settings, sites, feeds, rules, tasks and cached statistics |
| `secret.key`   | The encryption key                                                          |
| `downloads/`   | Staged `.torrent` files                                                     |
| `logs/`        | Logs                                                                        |

## What is encrypted

These are encrypted with AES-256-GCM, using the key in `secret.key`, before they are written to the database:

- site cookies;
- notification channel credentials, such as a Telegram bot token or a OneBot access token;
- the CloakBrowser access token.

The key is generated on first start. You can also supply a base64 key in the environment variable `PT_TOOLS_SECRET_KEY`, which takes precedence over the `secret.key` file.

Earlier versions also kept a plaintext copy of each cookie in the `site_settings` table. On the first start after upgrading, pt-tools encrypts any cookie that only had the plaintext copy and then clears the plaintext; it writes no backup for this step. Backups that older upgrades left in `~/.pt-tools/backups/` (`site_settings_v8_to_v9_*.json`) contain plaintext cookies too; once the upgrade looks fine, you can delete them.

> [!WARNING]
> Site **API keys and passkeys are currently stored in the database in plain text**. Make the data folder and its backups readable only by the user running pt-tools, and keep them out of shared folders and public cloud storage.

The notification channel list never returns credentials; they are decrypted and shown only on a channel's edit page, after you have signed in, so that you can change them.

## Accounts and sign-in

- pt-tools has a single admin account. The password is stored as a salted SHA-256 digest iterated 100,000 times; there is no plain-text password in the database.
- A signed-in session is kept in the process's memory and identified by a cookie named `session` (HttpOnly, SameSite=Lax). After pt-tools restarts you sign in again.
- A session with no requests for 30 days expires. At most 256 sessions are kept; beyond that the one unused for longest is dropped. Changing the password ends every other session on other browsers and devices; the session that made the change stays signed in.
- After 10 failed sign-ins from one IP within 15 minutes (unknown usernames included), that IP cannot sign in for 15 minutes. Behind a reverse proxy pt-tools sees only the proxy's address, so failures from everyone behind it are counted together.
- Apart from the sign-in page, static files and the health check `/api/ping`, every page and API requires you to be signed in. `/api/ping` returns only the status and the version.
- The [App API](app-api.md) (`/api/app/v1/`) also accepts [API tokens](../guide/api-tokens.md). A token calls the App API with the permissions chosen when it was created; it cannot reach any other API or manage tokens. The database stores only the SHA-256 digest of each token.
- The initial account is described in [Install](../guide/install.md); change its password the first time you sign in. If you forget it, reset it with `PT_ADMIN_RESET`, as described in [Configuration](../configuration.md).

## What it connects to

| Destination                          | Why                                                                                   |
| ------------------------------------ | ------------------------------------------------------------------------------------- |
| The trackers you add                 | Fetching feeds, searching, reading statistics and checking login status               |
| The downloaders you configure        | Pushing torrents and reading tasks and free space                                     |
| The notification channels you enable | Sending notifications; Telegram receives commands by long polling                     |
| GitHub (api.github.com)              | Checking for new releases; downloading one when the binary upgrades                   |
| A CloakBrowser Manager               | Only after you set its endpoint, token and profile ID, as a fallback for login checks |

The [CloakBrowser fallback](../guide/site-login-monitoring.md#cloakbrowser-fallback) hands the site's cookies to the CloakBrowser you deploy, which then opens the site's pages.

When `HTTP_PROXY` or `HTTPS_PROXY` is set, these requests go through the proxy; `ALL_PROXY` applies only to site access and downloader connections. pt-tools has no analytics or telemetry and sends no usage data to anyone.

## Which ports it listens on

- The web interface and API: `8080` by default.
- The QQ OneBot channel: once enabled, a second port that NapCat connects to over a reverse WebSocket, for example `/onebot/v11/ws` on `0.0.0.0:6701`. When the listen address is not a loopback address (127.0.0.1, localhost) an access token is required, otherwise the channel does not start. Open the port only on your local network.

## Deployment advice

- Do not expose the web port directly to the internet. For remote access, put it behind a reverse proxy with HTTPS, or reach it over a VPN. The session cookie is not marked Secure, so over plain HTTP someone on the same network could capture it, and the same goes for API tokens.
- Back up `torrents.db` and `secret.key` together and encrypt the backup; see [Upgrades and backups](../guide/upgrade.md).
- ChatOps runs commands only from accounts linked with a binding code, and QQ and Telegram channels also check the sender against the channel's allow lists first. At present every linked account has admin rights and can pause or delete torrents and manage subscriptions, so give binding codes only to people you trust. Every command is recorded in the audit log, with arguments such as tokens and passkeys redacted before they are written.

## The risk of automated access

pt-tools reaches your sites with your own cookie, API key or passkey. Some sites treat frequent automated access as a violation of their rules. Before turning on RSS, login checks and the other features, make sure each site's rules allow it and keep the intervals reasonable. pt-tools cannot guarantee that automated access will never lead to a warning, a demotion or a ban.

## Reporting a security problem

Do not post exploit details or any credential in a public issue. You can describe the kind of problem in [GitHub Issues](https://github.com/sunerpy/pt-tools/issues), or reach the maintainers in the [Telegram group](https://t.me/+7YK2kmWIX0s1Nzdl).
