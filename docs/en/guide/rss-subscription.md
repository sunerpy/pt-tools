# RSS subscriptions

This page explains how to find a site's RSS link, add a subscription in pt-tools, and what each option does.

## What an RSS subscription does

> [!WARNING]
> **Only freeleech torrents are downloaded by default.** When a subscription has no filter rules attached and its download mode is the default Smart (智能), it **downloads freeleech torrents only** and never downloads torrents that are not free. If all you want is to build ratio, add the subscription without any filter rules. Only when you want to follow a series or download particular releases even when they are not free do you need to create a rule under Filter rules (过滤规则) and turn off its Free only (仅免费) option. See [Filter rules and TV series](filter-rules-tv-series.md).

RSS (Really Simple Syndication) is a feed format for publishing new content. Private trackers use it to publish their latest torrents.

With an RSS subscription, pt-tools can:

1. Check the latest torrents a site publishes, on a schedule
2. Decide whether to download each one from the download mode and the filter rules (title keywords, size, freeleech status and so on)
3. Push the torrents that qualify to a downloader (qBittorrent, Transmission)

This is the core of fully automatic ratio building and of following series.

## Getting the RSS link

How you get the RSS link depends on the software the site runs.

### NexusPHP sites (such as HDSky and HDDolby)

These sites usually authenticate with a cookie, and the RSS link may contain your `passkey`. Never share your RSS link.

**Steps:**

1. Sign in to the site and open the **Torrents** page or the **home page**.
2. Look for the RSS icon or a "subscribe" link, usually at the top right of the page, next to the messages icon.
3. Choose the categories you want (such as TV series), the resolution and other conditions, and the number of items per page, then generate the RSS link.
4. Copy the generated link (usually the address meant for BitTorrent clients that support RSS).
5. The link usually looks like this:
   `https://site.com/torrentrss.php?rows=10&linktype=dl&passkey=YOUR_PASSKEY`

**Parameters:**

- `rows`: the number of torrents returned (10 to 50 is a good range)
- `linktype`: the link type; it normally has to include `dl` so the feed carries direct download links
- `passkey`: your personal key; **never share it**

### mTorrent sites (such as M-Team)

These newer sites usually use an API key or a dedicated RSS generator page.

**Steps (M-Team as the example):**

1. Sign in to the site and open the **user center** or **personal settings**.
2. Find the **API / RSS** settings.
3. Generate a new API key, if one is needed.
4. In the RSS generator, choose the categories you need (such as Movie and TV).
5. Copy the generated link.
6. An example of the format:
   `https://rss.m-team.cc/api/rss/fetch?dl=1&pageSize=10&sign=XXXX&t=XXX&tkeys=ttitle&uid=XXX`

## Adding a subscription in pt-tools

1. Open Sites → Site list (站点 → 站点列表) and click the Site settings and RSS (站点配置与 RSS 订阅) button on the site's row to open the site's page.
2. Switch to the RSS subscriptions (RSS 订阅) tab and click Add RSS (添加 RSS).
3. Enter a name and the link, set the options below as needed, then save.

After saving, the subscription runs in the background at its check interval. Pushed torrents appear under Downloads → Task list (下载 → 任务列表); when nothing is pushed, System → Logs (系统 → 运行日志) records why.

## Subscription options

| Option                                    | Meaning                                                                                                                             | Default            |
| ----------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- | ------------------ |
| **Name (名称)**                           | A name to recognise the subscription by                                                                                             | Required           |
| **Check interval (检查间隔)**             | How often the feed is fetched, in minutes, from 5 to 1440                                                                           | `10`               |
| **Link (链接)**                           | The site's RSS address                                                                                                              | Required           |
| **Category (分类)**                       | The category set when the torrent is pushed to the downloader                                                                       | Empty              |
| **Tag (标签)**                            | The tag added when the torrent is pushed to the downloader                                                                          | Empty              |
| **Downloader (下载器)**                   | Which downloader to push to; a downloader that is not enabled cannot be chosen                                                      | Default downloader |
| **Download folder (下载路径)**            | One of the downloader's preset folders, or a path you type after switching to Custom (自定义); empty means the downloader's default | Empty              |
| **Filter rules (过滤规则)**               | The filter rules attached to the subscription; you can choose several                                                               | None               |
| **Download mode (下载模式)**              | Follow global, Smart, Filter rules only or Free only; see below                                                                     | Follow global      |
| **Pause at freeleech end (免费结束暂停)** | Pauses the torrent automatically if it has not finished when its freeleech period ends; see Freeleech eligibility and pausing below | Off                |
| **Notification mode (通知模式)**          | Whether to send a notification for new torrents; see [New-torrent notifications](chatops-rss-notify.md)                             | No notifications   |

### Download modes

| Mode                               | Behaviour                                                                                                                                                                          |
| ---------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Smart (智能)** (recommended)     | Without filter rules, downloads freeleech torrents only; once rules are attached, downloads only the torrents that match them and no longer downloads unmatched freeleech torrents |
| **Filter rules only (仅过滤规则)** | Downloads only torrents that match a filter rule; a subscription with no rules attached downloads nothing                                                                          |
| **Free only (仅免费)**             | Downloads freeleech torrents only and ignores filter rules; suited to pure ratio building                                                                                          |
| **Follow global (跟随全局)**       | Uses the Default download mode (默认下载模式) under System → Global settings (系统 → 全局设置), which is Smart unless you change it                                                |

A freeleech torrent is pushed only if it can also finish within its freeleech period; how that is judged is explained under the speed limit and minimum freeleech time settings in [Configuration](../configuration.md). The maximum and minimum torrent sizes in the global settings apply in every mode.

## Freeleech eligibility and pausing

The freeleech path (free torrents in Smart and Free only modes) and filter rules with Free only (仅免费) on require freeleech. Before such a torrent is pushed to the downloader, pt-tools must hold freeleech information read in the current round:

- **Free until a given time**: pushed only if more than the safety margin is left. The safety margin is Act early, minutes before freeleech ends (免费期结束前提前处理) in the global settings, and at least 5 minutes.
- **End time unknown**: the site marks the torrent free but gives no end time, or the end time cannot be parsed. This is not treated as permanently free and the torrent is not pushed. The torrent file stays in the staging directory and is read again next round; once an end time is read, it is pushed. If you do want such torrents, attach a filter rule with Free only off.
- **Not free, or free has ended**: not pushed, and the staged torrent file is deleted.

A torrent that was not pushed last round (for example because the disk was full) has its site details read again before this round's push. If the item is no longer in the RSS feed or the details cannot be read, it is not pushed and last round's information is not reused. Filter rules with Free only off, Download now (立即下载) in a notification and manual pushes do not require freeleech; they push torrents that are not free as well.

Every round reads the site details of every item in the feed, and torrents skipped earlier are decided again: if one becomes free later, or you change the filter rules or size limits, it is downloaded in the next round. Torrents already pushed are not pushed again, but when freeleech is shortened, extended or cancelled, Pause at freeleech end and the auto-cleanup option Delete torrents still unfinished when freeleech ends (免费到期仍未下完的种子自动删除) both follow the newly read end time. More items mean more requests to the site; for sites that are strict about request rates, such as M-Team, do not set the interval too short.

Subscriptions of the same site that share a tag share a staging directory. Each torrent belongs to the subscription that first decided to download it; only that subscription pushes or cleans it up, with its own downloader, download path and Pause at freeleech end setting. Other subscriptions leave it alone.

For subscriptions with Pause at freeleech end on:

- The torrent is added to the downloader paused, and starts only after its task ID and the end-of-freeleech pause are recorded (it stays paused if the downloader is set not to start automatically).
- If the site gave no freeleech end time, the torrent is only added paused and is not started automatically.
- If the planned pause time (the freeleech end time minus the Act early minutes) has already passed or is less than a minute away, the torrent is also only added paused and is not started.
- Download now (立即下载) in a notification uses this subscription's downloader, path and pause setting.
- At pause time pt-tools reads the record again: if freeleech was extended, it waits for the new time. After pausing it reads the torrent back from the downloader, and only a paused state counts as success.
- A failed pause is retried indefinitely (starting at 30 seconds and doubling, at most every 10 minutes). After 3 failures in a row, one Free-end pause failing (免费结束暂停失败) alert is sent to every enabled notification channel.

In the task list, the Free end (免费结束) column shows End time unknown (结束时间未知) or Permanently free (长期免费); hovering shows when the information was read and when the pause is planned. Tasks needing attention (需要关注的任务) lists torrents held back because the end time is unknown and torrents whose pause keeps failing.

### Filter rules

Filter rules are created under Rules → Filter rules (规则 → 过滤规则) and then attached to a subscription:

- **Matching**: a keyword, a wildcard or a regular expression, matched against the title, the tags or both.
- **Size range**: a minimum and a maximum size in GB, which cannot exceed the global maximum.
- **Free only**: when on, the rule lets through freeleech torrents only.
- **Purpose**: the rule controls downloading, new-torrent notifications, or both.

The syntax and examples for following a series are in [Filter rules and TV series](filter-rules-tv-series.md).

## Good practice

1. **Choose a sensible interval.** Do not set a very short interval (under 5 minutes, for example): it puts load on the site's server and can get your account banned. `10` to `30` minutes is a good range.
2. **Test before you automate.** Before attaching rules, try them with Test match (测试匹配) on the filter rules page, which fetches live data from a subscription; after turning the subscription on, watch the logs to confirm the results are what you expect.
3. **Use the site's own filters.** If the site's RSS generator lets you choose categories (films only, for example), filter there when you generate the link, so less has to be processed locally.
4. **Keep categories apart.** Give the torrents downloaded through RSS their own downloader category, which makes it easier to automate a media library later (Jellyfin, for example).
5. **Protect your passkey.** When you share a configuration or a screenshot, always hide the `passkey` or `apikey` parameter of the RSS link.

## Troubleshooting

### 1. Fetching the feed fails (HTTP 403/401)

- **Cause**: the credentials have expired or the passkey is wrong.
- **Fix**: check that the passkey in the link is correct; for a site that needs a cookie, make sure pt-tools has a valid cookie for it.

### 2. The feed is fetched but nothing is downloaded

- **Cause**: the link itself returns no items, or every item was turned away by the download mode and the filter rules.
- **Fix**:
  - Open the RSS link in a browser and check that it returns XML.
  - Check whether the `rows` parameter is too small.
  - Check the download mode: in Smart mode with rules attached, only torrents that match the rules are downloaded.
  - Look in the logs for the reason each torrent was skipped.

### 3. The downloader cannot add the torrent

- **Cause**: the downloader cannot be reached, or the folder does not exist or is not writable.
- **Fix**: check the downloader's connectivity; make sure the download folder exists on the machine the downloader runs on and is writable.

### 4. The promotion status is not recognised

- **Cause**: some sites' feeds do not carry promotion information, or pt-tools does not yet handle the site's particular fields.
- **Fix**: update pt-tools to the latest version, or rely on reading the torrent's details page, where the site supports it.
