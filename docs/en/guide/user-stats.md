# Statistics and daily report

The User Stats page summarizes upload, download, ratio, bonus and seeding data across your sites. pt-tools stores one data snapshot per site per day and uses it to compute daily, weekly and monthly increments, draw trends on the page, and optionally send each day's increments to a notification channel (the daily report).

## Where the data comes from

- Every time pt-tools successfully fetches a site's user data, it updates the site's record and writes that day's snapshot; fetching several times a day keeps only the last one.
- For sites whose probe mode is "Auto", the login-state probe fetches user data roughly every 6 hours, which also refreshes the snapshot (see [Site login state and key backup](site-login-monitoring.md)).
- Sites in "Manual" or "Disabled" probe mode only update when you sign in to the Web UI or click "Sync"; pt-tools does not visit them on a schedule.
- Snapshots are grouped by day in the server time zone (`TZ` in Docker) and kept for 400 days; older ones are removed automatically once a day.

## How increments are computed

- A day's increment = that day's snapshot − the previous snapshot. If some days have no snapshot, the increment covers all of them together; nothing is interpolated.
- A period increment (today, this week, this month) = the latest snapshot in the period − the most recent snapshot before the period starts; if there is none before the period, the earliest snapshot inside it is used. A site with a single snapshot has no increment.
- When a site's numbers go backwards (the tracker reset its stats, or you spent bonus), that value counts as 0 and is left out of totals.

## Where to look

| Place                                | What it shows                                                                                                                           |
| ------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------- |
| User Stats → toolbar "Period"        | Today, this week (last 7 days), this month (last 30 days)                                                                               |
| User Stats → KPI "Total upload"      | The upload increment for the selected period; the bars are the daily upload increments of the last 7 days                               |
| User Stats → "Upload breakdown" card | The 6 sites with the largest upload increment in the period; ranked by current total upload until comparable snapshots exist            |
| Site list (phone)                    | 8 bars on each site card: daily upload increments over the last 8 days                                                                  |
| Site detail → "Site stats" card      | The 30-day upload increment and its daily trend                                                                                         |
| User Stats → Export                  | The share image can include the period increments ("Include period increments" switch and period choice); masking options are unchanged |

Right after installation there is no history yet; increments appear once a second snapshot exists, usually the next day.

## Daily report

Turn it on under "Daily report" in System Settings, then set the send time (default `22:00`, server time zone) and the receiving channels. At least one notification channel is required when it is enabled; it is never sent to all channels by default.

Each day at the send time, plus a fixed per-installation offset of 1–10 minutes (no later than 23:59), pt-tools sends one report for the day:

- upload, download and bonus increments per site and in total, listing up to 10 sites by upload;
- sites whose login state is abnormal (session expired, key error, challenged, network error and so on);
- today's check-in results (how many sites signed in, were already signed in, or failed);
- sites whose numbers went backwards and were counted as 0.

Each channel receives it at most once per day, including across restarts. If a channel is in its quiet hours, the report waits until the quiet hours end; failed deliveries are retried after 1, 5 and 30 minutes.

## API

| Endpoint                                                | Description                                                   |
| ------------------------------------------------------- | ------------------------------------------------------------- |
| `GET /api/v2/userinfo/history?site=<site>&days=<1–400>` | Daily data and increments of one site over recent days        |
| `GET /api/v2/userinfo/summary?range=today\|7d\|30d`     | Increments and totals of enabled sites over the period        |
| `GET /api/v2/userinfo/trends?days=<1–60>`               | Daily increments and totals of enabled sites over recent days |
| `GET` / `PUT /api/v2/userinfo/daily-report`             | Read or update the daily report settings                      |

All of them require a signed-in session.
