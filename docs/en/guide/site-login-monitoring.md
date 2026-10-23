# Login status and key backup

pt-tools periodically visits each site's profile with the cookie, API key or passkey you configured, reads the `last_access` / `last_login` information the site provides, and works out how many days remain before the site's inactivity limit. When an account gets close to the limit, it sends a reminder through the notification channels you have configured; [daily sign-in](#daily-sign-in) can also be turned on per site. Cookies can be synced with PT Tools Helper.

## How to use it

1. Enable the site under Sites → Site list (站点 → 站点列表) and fill in its credentials. For a site that signs in with a cookie, you can install [PT Tools Helper](browser-extension.md) and sync the cookie in one click once you are signed in to the site in your browser; API keys and passkeys are entered in the web UI.
2. The Days left (剩余天数) column of the site list shows how many days remain before the site's inactivity limit; a negative number means the limit has passed. The Effective activity (判定活跃) column, hidden by default and turned on under Column settings (列设置), is the activity time the reminders are based on; the rules are under [Effective activity](#effective-activity). Hover over Days left or Effective activity to see what the activity time is based on, when the next probe is due and since when probes have been failing; hover over Status (状态) to see the result of the last probe that did not succeed. A site with no probe result yet (never probed, or login probing does not apply to it) shows a grey Not probed (未探测).
3. Click the Keep-alive settings (保号配置) button on the row to set, per site, the inactivity limit, how many days ahead to remind, the reminder cron, the notification channels and the probe mode.
4. The Probe now (立即探测) button on the row probes the login status at once, and Test reminder (测试提醒) sends a test message to the notification channels. Probe enabled sites (探测启用站点) above the list probes every enabled site once, at most 3 at a time.
5. When a reminder arrives, visit the site as usual. If the cookie has expired, sync it again, then click Probe now to confirm the status has been refreshed.

The Keep-alive rules (保号规则) card on the site's detail page also shows the probe result, what the activity time is based on, the next probe, since when probes have been failing, and the access status.

> [!NOTE]
> Probing refreshes `last_access` (recent activity) on most sites. A few sites decide whether to remove an account by `last_login` (an actual sign-in) or by seeding activity; on those you still need to sign in by hand from time to time, whatever the days-left figure says.

## Per-site settings

The fields of the Keep-alive settings dialog:

| Field                                   | Default         | Meaning                                                                                                     |
| --------------------------------------- | --------------- | ----------------------------------------------------------------------------------------------------------- |
| Inactivity limit in days (封号判定天数) | `30`            | After this many days without activity, the site may disable the account                                     |
| Days ahead to remind (提前提醒天数)     | `10`            | How many days before the limit the reminders start                                                          |
| Reminder cron (提醒 cron)               | `0 10,22 * * *` | A standard 5-field cron (minute, hour, day, month, weekday); by default every day at `10:00` and `22:00`    |
| Notification channels (通知通道)        | Empty           | When set, reminders go only to the selected channels that are enabled; when empty, to every enabled channel |
| Probe mode (探测模式)                   | Auto            | Auto: follows the scheduled probes; Manual: only when you click Probe now; Disabled: never probes           |

## Scheduled probes

Enabled sites whose probe mode is Auto are probed in the background:

- Once a minute, pt-tools checks which sites are due and probes them one after another.
- Each site has a fixed random offset between 1 second and 6 hours. Sites that already have probe results but no next probe scheduled (for example after upgrading from an older version) get their first probe at that offset, so the first round is spread over 6 hours. Sites that have never been probed and have no activity data at all (newly added sites, a freshly upgraded database) are probed within 5 minutes, so you do not wait hours to see their last activity. A restart does not probe every site again; sites that are not due keep waiting.
- After you update a site's credentials (including a cookie synced by the browser extension) or save its settings, the site is probed within a minute instead of waiting for the next scheduled probe (except for sites whose probe mode is Manual or Disabled).
- Probes share the site connection and rate limit with user statistics and search; a successful probe also refreshes the site's upload, download, bonus and other figures in user statistics.
- After each probe, the next one is scheduled by the result:

| Result of this probe                                                                         | Next probe                                                                                                                                       |
| -------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| OK (正常)                                                                                    | In about 6 hours (plus or minus 10%)                                                                                                             |
| Session expired (会话已过期), key error (密钥错误)                                           | In 24 hours, so an invalid credential does not keep hitting the site                                                                             |
| Blocked by anti-bot (被反爬拦截), network error, rate limited, parse failure, unknown status | 1, 2 and 4 hours after the 1st, 2nd and 3rd failure in a row, then every 6 hours                                                                 |
| Credentials missing (未配置凭证)                                                             | No request to the site; once the credentials are added the site is probed within a minute, otherwise pt-tools checks again locally after 6 hours |
| Probing not supported (暂不支持探测)                                                         | No request to the site; checked again after 24 hours                                                                                             |

Probe now is not bound by this table; after it runs, the next probe is rescheduled by its result.

A single probe takes at most 150 seconds, of which the requests to the site take at most 90 seconds (the same per-site limit as a user info sync). A timeout is recorded as a failure and does not hold up the other sites.

Two checks run before a probe, without sending any request:

- When the site has no built-in definition (it is not in [Supported sites](../sites.md), such as a dynamic site added through the API or a template import), the status is recorded as Probing not supported (暂不支持探测).
- When a credential the sign-in method needs is missing, the status is recorded as Credentials missing (未配置凭证): cookie sign-in needs a cookie, API key sign-in needs an API key, cookie plus API key needs both, and passkey sign-in needs a passkey.

These two statuses send no request to the site, do not count as consecutive failures and do not trigger failure reminders.

## CloakBrowser fallback

A probe normally sends a direct request to the site, carrying the cookie, API key or passkey that the site's sign-in method needs. When a site sits behind Cloudflare or a similar anti-bot service, that request may not reach the profile page. With CloakBrowser configured, such probes try again by opening the site's profile page in CloakBrowser.

- Under System (系统) → CloakBrowser, fill in CloakBrowser-Manager endpoint (CloakBrowser-Manager 端点), Auth Token and Profile ID. The fallback is used only when all three are set. You deploy CloakBrowser-Manager (`cloakhq/cloakbrowser-manager`) yourself and create the profile there. Test connection (测试连接) checks the endpoint and token only, not the profile ID.
- The fallback runs only when the direct request ends as Blocked by anti-bot (被反爬拦截), network error or rate limited. Credential problems such as an expired session or a key error do not use it. A Cloudflare challenge page from the site is recorded as Blocked by anti-bot, not as a credential failure.
- When the fallback finds the session OK, its result is used. When the fallback does not succeed either, the direct request's result is kept and the reason the fallback failed is appended to the failure reason, which the message after Probe now (立即探测) shows.
- The fallback uses only the site's cookie: a site without a cookie (for example, one with only an API key or a passkey) gets no fallback. It opens the same site address as the direct request and loads the site's cookies into the browser:

| Site framework | Page opened                     | Requirement                                                                   |
| -------------- | ------------------------------- | ----------------------------------------------------------------------------- |
| NexusPHP       | `/userdetails.php?id=<user ID>` | The site has been probed successfully at least once, so its user ID is known  |
| Unit3D         | `/users/<username>`             | The site has been probed successfully at least once, so its username is known |
| Gazelle        | `/user.php?id=<user ID>`        | The site has been probed successfully at least once, so its user ID is known  |
| M-Team         | `/profile` on the web site      | No other requirement                                                          |
| HDDolby, Rousi | No fallback                     | —                                                                             |

- All sites share this one profile, so only one fallback probe runs at a time and the others wait. Waiting and the fallback both count toward the 150-second limit of a single probe.
- Changes to these three settings take effect at the next probe, without a restart.

## Effective activity

Effective activity is the time the days left and the inactivity reminders are based on:

- When the last probe was OK, it is the last-access time the site returns (`last_access`); when the site does not provide one, the last sign-in time the site returns.
- When the last probe did not succeed (or no probe has run yet), the site data may be out of date, so pt-tools compares it with the visit time recorded by the browser extension and uses the later of the two. While probing is broken, normal browsing of the site therefore keeps the days left from going down.
- The browser extension reports a visit when you open one of the site's pages, and includes the time when it syncs the cookie. This time only moves forward; an earlier report never replaces a later one.

What the time is based on is shown as Site last access (站点最近访问), Site last sign-in (站点最近登录) or Browser visit (浏览器访问).

Rousi Pro updates its last-active time only when you visit the website in a browser. Reading the profile and signing in with the API key do not refresh it (checked in 2026-10), and a sign-in does not count as activity either. So sign in to Rousi Pro in a browser now and then. After the visit, the next probe, or the probe before the next reminder, reads the new time.

When the probe is OK but the last-access time the site returns is the same as last time and already more than 48 hours old, the automatic probe is not refreshing that site's access time. The site list then marks Access not taking effect (访问未生效) next to Days left, and the inactivity reminder adds a sentence saying so. Sign in to such a site by hand; the mark disappears once the site's last-access time moves forward again.

## Reminders

### Inactivity reminders

Reminders start once the days left are no more than Days ahead to remind:

- They are sent at the times of the Reminder cron, at most once per cron slot.
- When the days left drop into a more urgent band than the previous reminder (14, 7, 3 and 1 days), a reminder goes out at once instead of waiting for the cron.
- Before a reminder goes out, a site in automatic probe mode is probed once more and judged again if its last probe succeeded more than 30 minutes ago (or it has never been probed), so a reminder based on a result from hours ago does not reach you right after you signed in to the site in a browser. When the last probe failed (expired credentials, rate limiting and so on), or the site is being probed or signed in at that moment, there is no extra probe and the reminder uses the existing result.
- The reminder gives the last activity time, what it is based on and the days left.

### Probe failure and recovery reminders

| Situation                                                                       | When the reminder is sent                                                                                                    |
| ------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| Session expired, key error                                                      | At the next check after it is found (checks run every minute); while it is not fixed, at most once every 24 hours after that |
| Blocked by anti-bot, network error, rate limited, parse failure, unknown status | Once the failures have lasted 24 hours since the first one; at most once every 24 hours after that                           |
| Credentials missing, probing not supported                                      | Never                                                                                                                        |
| Back to normal                                                                  | Once, for a site that had a failure reminder, when its probe is OK again                                                     |

### Channels and quiet hours

- Reminders go to the channels selected in Keep-alive settings that are enabled; when none are selected, to every channel enabled at the time the reminder is created.
- Quiet hours apply per channel and are set in the same place as for RSS notifications; see [quiet hours in RSS notifications](chatops-rss-notify.md#quiet-hours). A channel in its quiet hours waits until they end; the other channels deliver as usual.
- Inactivity reminders with 1 day or less left (including past the limit) and Test reminder ignore quiet hours.
- A failed delivery is retried after 1, 5 and 30 minutes; after the 4th failure it is given up. A retry that falls in quiet hours also waits until they end.
- Reminders are sent through the channels ChatOps already has connected; QQ and Telegram do not open a second connection. When the notification service has not started (the log shows a warning), reminders are recorded but not sent.

## Daily sign-in

Daily sign-in was added in the 1.0 preview. pt-tools can sign in to a site once a day (the sign-in plugin of NexusPHP sites, by default a request to `attendance.php`; sites such as HDArea, PTerClub and Rousi Pro use their own sign-in endpoints). It is off by default. Its switches, results and settings are all on the Sign-in (自动签到) page under Sites in the side navigation.

- Turning it on: turn on the Auto sign-in (自动签到) switch of the site in the table on the Sign-in page (Keep-alive settings (保号配置) on the site's row in the site list has it too). Sites that do not support it cannot be turned on; the table says why.
- Timing: each day the site is signed in at a random moment within the window under Sign-in settings (签到设置) on the Sign-in page, `08:00`–`10:00` by default, in the server's time zone (`TZ` in Docker). If the window has already passed when you turn it on or when pt-tools starts, the sign-in happens within the next 10 minutes.
- Results: once the sign-in succeeds, or the site says you already signed in today, no more requests are made that day. A failure is retried after 10 and then 30 minutes; if the third attempt fails too, the day is recorded as a failure (签到失败) and the next day starts afresh. Every site's result for every day is kept; Sign-in history (签到记录) shows the last 14 days, one cell per day.
- Sign-in details: under Sign-in history is one day's Sign-in details (签到详情), today by default. Click a date in the history's header to switch to that day, or step back and forward, or pick it from the drop-down. The top line sums the day up (how many sites, how many succeeded and failed; how many sites needed AI, how many AI calls and how many input and output tokens they used). Each site has a row: result, method (direct, AI captcha, AI quiz), attempts, AI usage, time and a note (the site's message on success, the reason on failure, and when the next retry is due). Expand a row to see each attempt that day: scheduled or manual, result, duration and reason. Attempts are kept for 90 days; days before this feature only have the day's result.
- Sign-in shares the site connection and rate limit with login probes, and a site is never probed and signed in at the same time.
- Notifications: Push sign-in results (推送签到结果) under Sign-in settings is on by default: once every site with sign-in turned on has a result for the day, one summary is sent (how many were signed in, which failed and why). With nothing selected under Send to (推送到) it goes to every enabled notification channel, or pick a few; a channel in its quiet hours waits until they end. With it turned off nothing is sent, and the history is still kept.
- Sign in to all (一键签到): the button at the top right of the Sign-in page signs in once to every site with automatic sign-in turned on that has no result today. It runs in the background and the page shows the progress; a site that needs a captcha can take a few minutes, so you can leave the page.
- Signing in by hand: Sign in now (立即签到) on a row of the Sign-in page, Sign in now (the calendar icon) on the site's row in the site list, Sign in now on the Keep-alive rules (保号规则) card of the site's detail page, and the ChatOps command `/signin [site]` (without a site, it signs in every site with automatic sign-in turned on that has no result today). Signing in by hand does not require automatic sign-in to be turned on, and a failure does not use up the automatic retries.

Signing in to HDSky, OpenCD, PTCHDBits and 52PT requires a captcha or a quiz answer and uses AI; see the next section. Rousi Pro signs in with the personal API key entered in the Passkey field of its site settings; the key needs the Sign in (执行签到) permission (`attendance:claim`). Without it the sign-in fails and the reason says so. These built-in sites do not support automatic sign-in:

| Site            | Reason                                                                                                                                          |
| --------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| M-Team          | It has no sign-in; keep the account alive by signing in with a browser; the site forbids third-party tools from calling its keep-alive endpoint |
| SpringSunday    | It has no sign-in; the site has no sign-in page                                                                                                 |
| U2              | Signing in requires picking an anime title from an animated image captcha                                                                       |
| BTSCHOOL        | Signing in works differently from usual NexusPHP sites                                                                                          |
| HD Dolby, MooKo | Not NexusPHP sites; sign-in is not supported yet                                                                                                |

Signing in adds one request to the site every day; make sure the site's rules allow automatic sign-in before turning it on.

### Captchas and quiz questions with AI

HDSky and OpenCD ask for a captcha when you sign in, and PTCHDBits and 52PT ask a multiple-choice question. After you turn on Use AI to sign in (用 AI 签到) under Sign-in settings (签到设置) on the Sign-in (自动签到) page, the model of the [AI assistant](assistant.md) handles these sites. It is off by default.

- Requirement: fill in the endpoint, API key and model under AI assistant settings (AI 助手设置) and turn the assistant on.
- Reading a captcha needs a model that can see images. Choose it under Model for captchas (识别验证码用的模型), for example Gemini, gpt-4o-mini or Llama 4; when none is chosen, the assistant's model is used. DeepSeek models cannot see images. Test vision (测试看图) draws a 6-character captcha on the spot, asks the model to read it, and says whether it got it right.
- Quiz questions use the assistant's model: the question and the options go to the model, and the option it picks is submitted. A wrong answer still counts as signed in by the site, with the reward its rules give (usually smaller), and the sign-in record says the AI answered wrongly (AI 答错了).
- When a captcha is read wrongly, a new captcha is fetched and read again, up to 3 per sign-in. If it is still wrong, or the model gives no answer, the sign-in fails and is retried after 10 and then 30 minutes.
- When Use AI to sign in is off, or the AI assistant is off or not set up, the day's sign-in for these sites is recorded as a failure with a reason that says how to turn it on, and is not retried.
- Captcha images and questions are sent to the provider set under AI assistant settings; cookies and passkeys are never sent.

## Backing up the encryption key

`~/.pt-tools/secret.key` encrypts site cookies and other credentials with AES-256-GCM. If the key is lost, the stored credentials cannot be decrypted and have to be synced or entered again.

Export a backup:

```bash
pt-tools secret export > ~/secret.key.backup
chmod 600 ~/secret.key.backup
```

Restore a backup:

```bash
pt-tools secret import --force < ~/secret.key.backup
```

> [!CAUTION]
> A key backup is as sensitive as the key itself. Keep it in a password manager, on encrypted media or in a controlled backup system; never commit it to a repository or store it unencrypted on cloud storage.

### Persistence in Docker

A container deployment must mount `/app/.pt-tools` on the host or on a managed volume. The folder holds `secret.key`, the database, the logs and the staged torrent files; before recreating the container, make sure the mount is in place and a backup is available.

```yaml
volumes:
  - ./data:/app/.pt-tools
```

Deleting `./data` loses the database and the key together. Back up the key and the database as one consistent unit.

## Limits and compliance

This feature periodically visits a site's pages or profile API with your own cookie, API key or passkey. Before turning it on, make sure the site's rules allow automated access, and choose how often to probe according to the risk you are willing to take:

- Some sites may treat frequent scripted access as a violation.
- The login status fields come from the site and may be missing, delayed or mean something different.
- pt-tools does not guarantee that this feature prevents an account from being disabled, and accepts no responsibility for warnings, demotions or bans caused by automated access.

How to obtain and sync credentials is described in [Cookies and API keys](get-cookie-apikey.md).
