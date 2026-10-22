# More notification channels

Besides [QQ](chatops-qq-napcat.md) and [Telegram](chatops-telegram.md), which also take commands, pt-tools can push notifications to these outbound-only channels: WeCom group bots, DingTalk group bots, Feishu (Lark) group bots, Bark, ServerChan (Server 酱) and ntfy. They only send; you cannot send commands from them.

DingTalk, Feishu, Bark, ServerChan and ntfy, [message templates](#message-templates) and the [custom webhook](#custom-webhook) settings were added in the 1.0 preview; the WeCom group bot and the generic webhook were already in 0.48.0 (experimental).

In ChatOps → Notifications (消息通知), click Add channel (添加通道), choose the type, and enter a name and the settings. For Bark or ntfy on your own server, enter the server address and credentials in this step: the channel is enabled as soon as it is created, and with the address left empty it first sends to the default public server. After creating it, click Send test message (发送测试消息) to check that it arrives; you can change the settings later in the channel's settings. Credentials are stored encrypted like other channels and are not shown in the list.

Settings are checked when you save. If one is malformed (for example, a webhook address that is not the service's address, or an ntfy topic containing `.`), nothing is saved and the message names the setting.

When sending:

- A response that is not the service's success response (for example, when the address points to another site) counts as a failed delivery and is retried under the usual notification retry rules.
- Redirects are not followed: a redirect counts as a failed delivery, so enter the address it points to.
- Local and private addresses (`127.0.0.1`, `192.168.x.x` and so on) are not used unless you allow them, and link-local addresses (including the cloud metadata address `169.254.169.254`) are never used.
- Error messages leave out webhook addresses, keys, tokens and passwords.

The Recent delivery stats (最近投递统计) card on the Notifications page shows sent / total per channel over the last 7 days. RSS notifications and background notifications (keep-alive reminders, check-ins, daily reports, dead torrents, subscriptions and library imports, remote access) all count; channel tests and command replies do not. The small text under each channel gives the count per source and says whether the rest failed, are waiting (held for quiet hours or for a retry), were canceled or replaced (canceled in the RSS notification log, or replaced by a later notification for the same torrent), or went over the RSS hourly limit. An RSS notification that cannot be sent right away goes to the retry queue and still counts as sent; the small text gives how many went to the retry queue and how many of those were sent later, are still retrying, or were given up.

The Recent notifications (最近的通知) card on the same page lists the latest 8, RSS and background notifications alike: the channel, the source, the notification's title (the torrent name for RSS notifications, after the site), the result and the number of attempts. This is the built-in title: with a message template, the title you receive follows the template, and RSS notifications combined into one message are still listed one by one here. The full RSS list, retries and cancels are in the RSS notification log (RSS 通知日志); a background notification that fails is retried after 1, 5 and 30 minutes.

## WeCom group bot

| Setting     | Meaning                                                                                           |
| ----------- | ------------------------------------------------------------------------------------------------- |
| Webhook Key | The part after `key=` in the bot address `https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=…` |

Messages are sent as Markdown. Error codes from WeCom (an invalid key, too many messages and so on) count as failed deliveries and are retried under the usual notification retry rules.

## DingTalk group bot

| Setting                   | Meaning                                                                                                               |
| ------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| Webhook address           | The full address copied from the bot settings; only `https://oapi.dingtalk.com/robot/send?access_token=…` is accepted |
| Signing secret (optional) | The secret starting with `SEC` when the security setting is signing (加签)                                            |
| Message format            | Markdown (default) or plain text                                                                                      |

DingTalk requires at least one security setting per bot: with signing, enter the secret here; with custom keywords, every message must contain one of the keywords (for example in the notification content), or DingTalk rejects it; with an IP allowlist, add the public IP pt-tools sends from.

## Feishu (Lark) group bot

| Setting                   | Meaning                                                                                                                                     |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| Webhook address           | The address copied from the bot settings; only `https://open.feishu.cn/open-apis/bot/v2/hook/…` is accepted (`open.larksuite.com` for Lark) |
| Signing secret (optional) | Enter it when signature verification (签名校验) is on                                                                                       |

The signature uses the current time; Feishu rejects messages when the clock of the machine running pt-tools is more than an hour off.

## Bark

| Setting                                | Meaning                                                                                                            |
| -------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| Device Key                             | The part of the push address in the Bark app, `https://api.day.app/<Device Key>/…`                                 |
| Server address (optional)              | `https://api.day.app` by default; your own address for a self-hosted bark-server                                   |
| Allow private addresses (允许内网地址) | Turn on when your server is on this machine or your LAN (such as `192.168.x.x` or Docker on a NAS); off by default |
| Group, sound (optional)                | The group and sound of the notification in Bark                                                                    |

## ServerChan

| Setting | Meaning                                                                                                      |
| ------- | ------------------------------------------------------------------------------------------------------------ |
| SendKey | Both the Turbo `SCT…` keys and the ServerChan³ `sctp…` keys work; pt-tools picks the address from the format |

ServerChan titles are limited to 32 characters; a longer title is shortened and the full title goes on the first line of the message.

## ntfy

| Setting                                          | Meaning                                                                 |
| ------------------------------------------------ | ----------------------------------------------------------------------- |
| Topic                                            | Letters, digits, `-` and `_`, up to 64 characters                       |
| Server address (optional)                        | `https://ntfy.sh` by default; your own address for a self-hosted ntfy   |
| Allow private addresses (允许内网地址)           | Turn on when your server is on this machine or your LAN; off by default |
| Access token / user name and password (optional) | One of them when the server has access control on                       |
| Priority (optional)                              | 1 lowest, 5 highest, 0 for the server default                           |

Topics on the public `ntfy.sh` server have no password, and anyone who knows the name can subscribe: pick a name that is hard to guess, or use your own server with access control.

## Message templates

You can write your own title and body for each kind of notification, and every notification channel (QQ, Telegram, the group bots, Bark and the others) sends it that way. In ChatOps → Message templates (消息模板), pick an event, fill in Title template (标题模板) or Body template (正文模板), and click Save (保存). A field left empty uses the built-in text; Restore default (恢复默认) goes back to the built-in wording. Replies to ChatOps commands and the messages the AI assistant sends in a channel can be changed too; see [Command and AI assistant replies](#command-and-ai-assistant-replies) below.

Templates use Go template syntax: a variable is written like `.site` inside two pairs of curly braces and is replaced by its value; `if` is a condition and `range` loops over a list. Variables (变量) on the page lists every variable the event has, and clicking one inserts it at the cursor; Preview (预览（示例数据）) renders the template with sample data as you type; Built-in (内置的样子) shows what is sent when you change nothing.

Examples: a single line for an RSS item that matches a filter rule, and an attendance summary that lists only the sites that failed.

```text
{{.site}} | {{.torrent_title}} | {{.size}}{{if .free}} | free, {{.free_remaining}} left{{end}}
```

```text
{{.day}} sign-in: {{.signed}} done, {{.failed}} failed
{{range .items}}{{if eq .status "failed"}}· {{.site}}: {{.message}}
{{end}}{{end}}
```

Every event has these variables: `event` (event type), `event_name` (event name), `time` (local time such as `2026-10-12 08:00:00`), `timestamp` (Unix seconds), `title` and `text` (the built-in title and body, which your template can reuse). The main variables of each event:

| Event                                                     | Main variables                                                                                                                 |
| --------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `rss.new` new RSS item                                    | `site`, `rss`, `torrent_title`, `torrent_id`, `link`, `published`                                                              |
| `rss.matched` RSS item matched a filter rule              | the same, plus `subtitle`, `size`, `size_bytes`, `free`, `free_end`, `free_remaining`, `rule`, `seeders`, `leechers`, `has_hr` |
| `rss.digest` combined RSS notification                    | `count`, `items` (each with `title`, `site`, `torrent_title`, `torrent_id`)                                                    |
| `subscribe.downloaded` subscription download              | `subscription`, `media_type`, `year`, `season`, `episodes`, `torrent_title`, `site`, `quality`, `poster`                       |
| `media.followshow.found` followed show updated            | `subscription`, `season`, `episodes`, `torrent_title`, `site`, `quality`, `version`, `poster`                                  |
| `media.followshow.waiting` followed show waiting          | `subscription`, `subscription_id`, `season`, `episodes`, `version`, `days`                                                     |
| `media.followshow.failed` followed show failing           | `subscription`, `season`, `fails`, `error`                                                                                     |
| `subscribe.douban_failed` Douban wish list fetch failed   | `source`, `fails`, `error`                                                                                                     |
| `media.organized` organized into the library              | `name`, `media_type`, `episodes`, `files`, `quality`, `library`, `torrent`, `poster`                                           |
| `site.login_expiring` site account about to be disabled   | `site`, `tier`, `days_remaining`, `last_active`, `days_since_active`, `access_stale`                                           |
| `site.probe_failed` site probe failed                     | `site`, `status`, `status_label`, `since`, `advice`, `diagnosis`                                                               |
| `site.probe_recovered` site probe recovered               | `site`, `probed_at`                                                                                                            |
| `site.login_test` login reminder test                     | `site`, `tier`, `days_remaining`, `last_active`                                                                                |
| `attendance.daily` sign-in results                        | `day`, `signed`, `failed`, `items` (each with `site`, `status`, `status_label`, `message`)                                     |
| `report.daily` daily report                               | `day`, `uploaded`, `downloaded`, `uploaded_bytes`, `downloaded_bytes`, `bonus`, `items` (per site)                             |
| `downloader.dead_torrents` dead torrents                  | `downloader`, `count`, `size`, `size_bytes`, `items` (each with `name`, `size`, `size_bytes`)                                  |
| `downloader.free_end_pause_failed` free-end pause failing | `site`, `torrent_title`, `downloader`, `free_end`, `failures`, `error`                                                         |
| `remote.paired` new device paired                         | `device`, `permission`                                                                                                         |
| `test` channel test                                       | `channel`                                                                                                                      |

The daily report's variables cover the totals and the per-site figures; the login problems, sign-in and brushing parts are only in `text`. Lists hold at most 100 entries; the number left out is in `items_more`.

Functions: the comparisons `eq`, `ne`, `lt`, `le`, `gt` and `ge` (integers and decimals compare directly), `default` (a fallback for empty values), `truncate` (cuts to a number of characters and adds "…"), `contains`, `hasPrefix`, `hasSuffix`, `replace`, `upper`, `lower`, `trim`, `bytes` (writes a byte count as `15.00 GB`), `date` (formats a time with a Go layout), `printf` and `json` (writes a JSON literal). Examples:

```text
{{if gt .seeders 10}}popular{{end}}
{{.rule | default "none"}}
{{.text | truncate 100}}
{{if contains "2160p" .torrent_title}}4K{{end}}
{{bytes .size_bytes}}
{{date "01-02 15:04" .timestamp}}
{{printf "%.1f" .bonus}}
```

Saving renders the template once with sample data: a misspelled variable or a syntax error is not saved, and the message says which line is wrong. So that templates cannot slow notifications down, there are a few limits: `range` only loops over list variables, at most two levels deep, and a two-level loop can only hold a few simple actions; a template can declare up to 64 variables; a template can be up to 8 KB; a rendered title up to 1 KB and a body up to 16 KB; `printf` widths and precisions go up to 256, without the `*` and `[n]` forms; a function's arguments together, and its result, can be up to 64 KB, and all the function calls in one render can handle up to 4 MB in total. If rendering still fails when sending, or produces nothing, that notification goes out with the built-in text, so a broken template never stops notifications.

Telegram messages are sent as plain text, so HTML or Markdown in a template shows up as written.

### Command and AI assistant replies

What pt-tools replies when you send a command in Telegram, QQ and the other channels, and the messages the AI assistant sends there, can be rewritten with templates as well. They are in the ChatOps command replies (ChatOps 命令回复) and AI assistant replies (AI 助手回复) groups on the Message templates page and have a body template only. A reply goes back only to the channel the message came from, so a webhook never receives it; one template applies to every binding, whatever its reply language.

| Event                          | Used for                                                                                                                 | Main variables                                                                                                                                                                    |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `reply.status` status overview | `/status`                                                                                                                | `tasks`, `running`, `items` (each downloader: `name`, `type`, `healthy`)                                                                                                          |
| `reply.torrents` torrent list  | `/torrents <downloader>`                                                                                                 | `downloader`, `total`, `items` (up to 20: `name`, `state`, `progress`, `size`, `size_bytes`), `items_more`                                                                        |
| `reply.sites` site list        | `/sites`                                                                                                                 | `count`, `items` (`name`, `status`, `enabled`)                                                                                                                                    |
| `reply.site_info` user info    | `/sites <site>`                                                                                                          | `site`, `user`, `uploaded`, `downloaded`, `uploaded_bytes`, `downloaded_bytes`, `ratio`, `bonus`, `class`                                                                         |
| `reply.tasks` RSS jobs         | `/tasks`                                                                                                                 | `count`, `running`, `items` (`site`, `rss`, `state`)                                                                                                                              |
| `reply.signin` sign-in         | `/signin`                                                                                                                | `signed`, `failed`, `items` (`site`, `status`, `status_label`, `message`)                                                                                                         |
| `reply.version` version        | `/version`                                                                                                               | `version`, `build`, `commit`, `update_checked`, `has_update`, `latest`                                                                                                            |
| `reply.help` command list      | `/help`                                                                                                                  | `items` (`name`, `description`, `admin`)                                                                                                                                          |
| `reply.subs` subscriptions     | `/subs`                                                                                                                  | `count`, `items` (up to 20: `id`, `title`, `status`, `progress`, `next_search`), `items_more`                                                                                     |
| `reply.other` other replies    | everything else: results of the other commands, failed commands, and the not bound / not allowed / unknown command hints | `command` (the command name without `/`), `ok` (whether it worked)                                                                                                                |
| `assistant.reply` AI assistant | messages from the AI assistant                                                                                           | `kind` (`answer`, `notice`, `confirm` for an action to confirm, `timeout` for a confirmation that timed out, `status` for the immediate reply to a message), `summary`, `expires` |

The common variable `text` is the built-in reply, and `event`, `time` and the rest work as usual; replies have no title, so `title` is empty. When a command does not work (bad arguments, a service that is not available, an error), the reply uses `reply.other` with `ok` set to false, including the query commands above.

Examples: mark the replies that did not work, list only the downloaders `/status` cannot reach, and put "🤖" in front of the assistant's answers while leaving its other messages alone.

```text
{{if not .ok}}❌ {{end}}{{.text}}
```

```text
{{.running}}/{{.tasks}} RSS jobs running{{range .items}}{{if not .healthy}}
⚠️ {{.name}} is unreachable{{end}}{{end}}
```

```text
{{if eq .kind "answer"}}🤖 {{end}}{{.text}}
```

A long reply is sent as several messages: up to 3,500 characters each and at most 4 of them, with a note when the rest is left out. The assistant's answer, too, goes through the template before it is split, so a header or footer the template adds appears once. The body of a command or assistant reply can be up to 64 KB; the other limits are the same as for notifications, and a reply whose template fails or renders empty is sent with the built-in text.

## Custom webhook

The generic Webhook channel sends notifications to your own services, such as n8n, Home Assistant, Discord, Slack, Gotify or PushPlus. In the channel's settings, the "Request" part lets you change:

- Method: POST (default), PUT, PATCH or GET. GET has no body.
- Body format: JSON (default), plain text, form, or custom (put `Content-Type` in the headers). When the headers contain `Content-Type`, the header wins.
- Body template: the same syntax and variables as [message templates](#message-templates), plus `channel` (the channel name). The title and text are the ones after message templates. To put text into JSON, use the `json` function, which adds the quotes and escapes:

  ```text
  {"content": {{json .title}}, "site": {{json .site}}}
  ```

- Presets: Discord, Slack, Gotify, PushPlus, Home Assistant, n8n (every variable flattened into one JSON object), plain text and form. A preset only fills in a starting point that you can edit, and shows what that service's address usually looks like. For PushPlus, replace "你的 token" in the body with your own token.
- Address: the path and query can contain a template too; the host name cannot, and the rendered address must keep the scheme and host written in the template. For example:

  ```text
  https://example.com/hook?site={{urlquery .site}}
  ```

With an empty body template, the channel sends the fixed JSON: `event_type` (always `notification`), `title`, `text`, `link`, `payload`, and `event` (the event `type`, its `time`, and all of its variables in `fields`). `event` is new in this version and the other fields are unchanged: receivers that read fields are not affected, and receivers that compare the whole body need updating.

A webhook receives every kind of notification, so saving renders the templates once with the sample data of each event: a misspelled variable, or a JSON body that does not render to valid JSON, is not saved, and the message names the event and the line. Variables that only some events have are empty or 0 in the others. "Preview with sample data" lets you pick an event and see the rendered request without sending it.

With an HMAC secret, the `X-PT-Signature` header is the Base64-encoded HMAC-SHA256 of the body actually sent. Only a 2xx response counts as delivered; any other status is retried under the notification retry rules. Redirects are not followed: a redirect counts as a failure, so enter the final address. At most the first 64 KB of a response is read.

Many services answer 200 even on errors and put the error in the response. For those, fill in the success condition: after a 2xx it is rendered against the response, and only the result `true` counts as delivered; `false` or an empty result is a failure, and any other text is used as the failure reason (tokens, header values and the HMAC secret we sent are replaced with `***` if the response echoes them). Variables: `status` (the status code), `body` (the raw response, up to the first 64 KB) and `json` (the parsed response when it is JSON; fields read as zero values otherwise). The PushPlus, Gotify and Slack presets include a success condition. The editor lets you paste a response to see whether it counts as success. Example:

```text
{{if eq .json.code 0}}true{{else}}{{.json.msg}}{{end}}
```

This channel is meant for your own services and may reach loopback and private addresses. Error messages and logs leave out the query part of the address and the body.
