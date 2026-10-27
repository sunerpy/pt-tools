# Statistics and daily report

The User Stats page summarizes upload, download, ratio, bonus and seeding data across your sites. pt-tools stores a snapshot for a site on each day its data is fetched successfully, and uses the snapshots to compute daily, weekly and monthly increments, draw trends on the page, and optionally send each day's increments to a notification channel (the daily report, off by default).

The snapshots, increments, trends and daily report were added in the 1.0 preview; the Statistics (用户统计) page itself was already in 0.48.0.

## Where the data comes from

- Every time pt-tools successfully fetches a site's user data, it updates the site's record and writes that day's snapshot; fetching several times a day keeps only the last one.
- For sites whose probe mode is "Auto", the login-state probe fetches user data roughly every 6 hours, which also refreshes the snapshot (see [Site login state and key backup](site-login-monitoring.md)).
- Sites in "Manual" or "Disabled" probe mode only update when you sign in to the Web UI or click "Sync"; pt-tools does not visit them on a schedule.
- Fetching one site's data takes several pages (the index, user details, bonus, the seeding list). Where the site definition sets a request delay (most NexusPHP sites), pt-tools fetches the pages one at a time with that gap, so that several simultaneous requests do not draw a Cloudflare challenge. When a page's connection is reset, times out or the site errors (5xx), it retries twice with exponential backoff (about 2 and then 4 seconds); on a Cloudflare challenge it retries once after a few seconds. One site takes at most 90 seconds (on a slow link a single page can take 20 to 30 seconds); pages still unfetched by then count as not fetched.
- When a page still fails but the username came through, the fetch counts as successful: the items that were not fetched keep their previous values (the last-access time is taken from this visit), and the log records one line, "Partial user info, kept previous values", with the reason and the items kept. When upload or download is missing and there is no earlier record, nothing is saved this time. Without a username, or with an expired login, the fetch fails, and the log records one "Failed to fetch user info" line per site whose `reason` says whether it was a Cloudflare challenge, a network error (connection reset), a timeout or an expired login.

## How increments are computed

- A day's increment = that day's snapshot − the previous snapshot. If some days have no snapshot, the increment covers all of them together; nothing is interpolated.
- A period increment (today, this week, this month) = the latest snapshot in the period − the most recent snapshot before the period starts; if there is none before the period, the earliest snapshot inside it is used. A site with a single snapshot has no increment.
- When a site's numbers go backwards (the tracker reset its stats, or you spent bonus), that value counts as 0 and is left out of totals.

## Two kinds of download figures

- **Billed download (下载量（计费）)**: the download a site counts toward your ratio after free, discount and other promotions. Every download figure on the User Stats page, in site details, in the daily report, in the ChatOps `/sites <site>` reply, in the mobile app, in the App API and in MCP is this one, and each of them says so (计费 in the Web UI, `Downloaded (billed)` in the ChatOps reply).
- **Actual download (实际下载)**: the amount actually transferred, before promotions, which some sites (M-Team, for example) report separately. It appears together with the actual upload in the "Actual transfer" (实际传输) column of the User Stats site table (hidden by default; turn it on under "Columns" (列设置)) and in the "Actual upload / download" (实际上传 / 下载) row of the "Site stats" (站点统计) card in site details. Sites that do not report it have no such figures.
- Downloading a free torrent increases the actual download while the billed download stays the same; that is expected. To check whether a download was billed, look at the billed download and the ratio.
- Daily snapshots keep only the billed figure: increments, trends and the daily report all use the billed download, and there is no history of the actual download.
- With some upload and no billed download the ratio is infinite (the site shows 无限, Inf. or ∞), and every site shows ∞. The average ratio leaves it out, and ratio requirements for a class count as met. The App API and MCP give an infinite ratio as 9999999.

## Where to look

| Place                                | What it shows                                                                                                                                                     |
| ------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| User Stats → toolbar "Period"        | Today, this week (last 7 days), this month (last 30 days)                                                                                                         |
| User Stats → KPI "Total upload"      | The upload increment for the selected period; the bars are the daily upload increments of the last 7 days                                                         |
| User Stats → "Upload breakdown" card | The 6 sites with the largest upload increment in the period; ranked by current total upload until comparable snapshots exist                                      |
| Site list (phone)                    | 8 bars on each site card: daily upload increments over the last 8 days                                                                                            |
| Site detail → "Site stats" card      | The 30-day upload increment and its daily trend                                                                                                                   |
| Data export (数据导出)               | The share image can include the period increments ("Include period increments" switch and period choice); see [Exporting a share image](#exporting-a-share-image) |

Right after installation there is no history yet; increments appear once a second snapshot exists, usually the next day.

## Exporting a share image

Overview → Data export (概览 → 数据导出) in the left navigation, or Export share (导出分享) on the User Stats toolbar, draws your statistics as an image to share.

- Styles: Bento (便当, the default: tiles of different sizes, the most detail), Paper (素笺: paper background, thin rules and large serif figures), Night (夜幕: dark glass cards, with a ring showing upload against billed download), Wrapped (回顾: one large figure per sentence, the three sites with the most upload on a podium) and Receipt (小票: a thermal receipt with a red stamp and a barcode). Each style has two or three color schemes. The style and color scheme you pick are remembered in this browser.
- The preview is the exported image. Download image (下载图片) saves a PNG at twice the size (1280 pixels wide); Copy image (复制图片) puts it on the clipboard, which needs pt-tools opened over HTTPS or on localhost. On a phone the preview is scaled down to fit the screen; the exported image is the same.
- Masking: site names, site icons and usernames are all masked by default; each can be turned off on its own. A name keeps only its first character followed by a bar; an icon becomes a 4×4 block of its colors. The original text is not in the image and cannot be recovered.
- The per-site details start with the 10 sites with the most upload ticked; tick the sites you want, and with none ticked the image has no per-site details. Sites are sorted by upload. The period increments can be today, this week or this month, and are left out until comparable snapshots exist.
- The ratio on the image is the average of the sites' ratios, the same figure as Average ratio (平均分享率) on the User Stats page.

## Daily report

Turn it on under "Daily report" in System Settings, then set the send time (default `22:00`, server time zone) and the receiving channels. At least one notification channel is required when it is enabled; it is never sent to all channels by default.

Each day at the send time, plus a fixed per-installation offset of up to 10 minutes (picked at random the first time it is needed and then kept, no later than 23:59), pt-tools sends one report for the day:

- upload, billed download and bonus increments per site and in total, listing up to 10 sites by upload;
- sites whose login state is abnormal (session expired, key error, challenged, network error and so on);
- today's check-in results (how many sites signed in, were already signed in, or failed);
- sites whose numbers went backwards and were counted as 0.

Only enabled sites are counted. A site whose probe mode is "Disabled" is no longer probed, so its last probe status is not reported as a login problem. When there is no snapshot from before today and nothing can be compared yet, the report says so instead of listing zeros.

Each channel receives it at most once per day, including across restarts. If a channel is in its quiet hours, the report waits until the quiet hours end; failed deliveries are retried after 1, 5 and 30 minutes. If pt-tools starts after the send time but before the day ends, it still sends that day's report; a day on which it was not running at any point between the send time and midnight is skipped, and the next day's report only covers the next day.

## API

| Endpoint                                                | Description                                                   |
| ------------------------------------------------------- | ------------------------------------------------------------- |
| `GET /api/v2/userinfo/history?site=<site>&days=<1–400>` | Daily data and increments of one site over recent days        |
| `GET /api/v2/userinfo/summary?range=today\|7d\|30d`     | Increments and totals of enabled sites over the period        |
| `GET /api/v2/userinfo/trends?days=<1–60>`               | Daily increments and totals of enabled sites over recent days |
| `GET` / `PUT /api/v2/userinfo/daily-report`             | Read or update the daily report settings                      |

All of them require a signed-in session.
