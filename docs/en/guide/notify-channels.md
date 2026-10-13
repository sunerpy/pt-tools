# More notification channels

Besides [QQ](chatops-qq-napcat.md) and [Telegram](chatops-telegram.md), which also take commands, pt-tools can push notifications to these outbound-only channels: WeCom group bots, DingTalk group bots, Feishu (Lark) group bots, Bark, ServerChan (Server 酱) and ntfy. They only send; you cannot send commands from them.

In ChatOps → Notifications (消息通知), click Add channel (添加通道), choose the type, and enter a name and the settings. For Bark or ntfy on your own server, enter the server address and credentials in this step: the channel is enabled as soon as it is created, and with the address left empty it first sends to the default public server. After creating it, click Send test message (发送测试消息) to check that it arrives; you can change the settings later in the channel's settings. Credentials are stored encrypted like other channels and are not shown in the list.

Settings are checked when you save. If one is malformed (for example, a webhook address that is not the service's address, or an ntfy topic containing `.`), nothing is saved and the message names the setting.

When sending:

- A response that is not the service's success response (for example, when the address points to another site) counts as a failed delivery and is retried under the usual notification retry rules.
- Redirects are not followed: a redirect counts as a failed delivery, so enter the address it points to.
- Local and private addresses (`127.0.0.1`, `192.168.x.x` and so on) are not used unless you allow them, and link-local addresses (including the cloud metadata address `169.254.169.254`) are never used.
- Error messages leave out webhook addresses, keys, tokens and passwords.

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

You can write your own title and body for each kind of notification, and every notification channel (QQ, Telegram, the group bots, Bark and the others) sends it that way. In ChatOps → Message templates (消息模板), pick an event, fill in Title template (标题模板) or Body template (正文模板), and click Save (保存). A field left empty uses the built-in text; Restore default (恢复默认) goes back to the built-in wording.

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

| Event                                                   | Main variables                                                                                                                 |
| ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `rss.new` new RSS item                                  | `site`, `rss`, `torrent_title`, `torrent_id`, `link`, `published`                                                              |
| `rss.matched` RSS item matched a filter rule            | the same, plus `subtitle`, `size`, `size_bytes`, `free`, `free_end`, `free_remaining`, `rule`, `seeders`, `leechers`, `has_hr` |
| `rss.digest` combined RSS notification                  | `count`, `items` (each with `title`, `site`, `torrent_title`, `torrent_id`)                                                    |
| `subscribe.downloaded` subscription download            | `subscription`, `media_type`, `year`, `season`, `episodes`, `torrent_title`, `site`, `quality`, `poster`                       |
| `subscribe.douban_failed` Douban wish list fetch failed | `source`, `fails`, `error`                                                                                                     |
| `media.organized` organized into the library            | `name`, `media_type`, `episodes`, `files`, `quality`, `library`, `torrent`, `poster`                                           |
| `site.login_expiring` site account about to be disabled | `site`, `tier`, `days_remaining`, `last_active`, `days_since_active`, `access_stale`                                           |
| `site.probe_failed` site probe failed                   | `site`, `status`, `status_label`, `since`, `advice`, `diagnosis`                                                               |
| `site.probe_recovered` site probe recovered             | `site`, `probed_at`                                                                                                            |
| `site.login_test` login reminder test                   | `site`, `tier`, `days_remaining`, `last_active`                                                                                |
| `attendance.daily` sign-in results                      | `day`, `signed`, `failed`, `items` (each with `site`, `status`, `status_label`, `message`)                                     |
| `report.daily` daily report                             | `day`, `uploaded`, `downloaded`, `uploaded_bytes`, `downloaded_bytes`, `bonus`, `items` (per site)                             |
| `downloader.dead_torrents` dead torrents                | `downloader`, `count`, `size`, `size_bytes`, `items` (each with `name`, `size`, `size_bytes`)                                  |
| `remote.paired` new device paired                       | `device`, `permission`                                                                                                         |
| `test` channel test                                     | `channel`                                                                                                                      |

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

Saving renders the template once with sample data: a misspelled variable or a syntax error is not saved, and the message says which line is wrong. So that templates cannot slow notifications down, there are a few limits: `range` only loops over list variables, at most two levels deep; a template can be up to 8 KB; a rendered title up to 1 KB and a body up to 16 KB; `printf` widths and precisions go up to 256, without the `*` and `[n]` forms; a function's arguments together, and its result, can be up to 64 KB, and all the function calls in one render can handle up to 4 MB in total. If rendering still fails when sending, or produces nothing, that notification goes out with the built-in text, so a broken template never stops notifications.

Templates change notifications only, not the replies to ChatOps commands or from the AI assistant. Telegram messages are sent as plain text, so HTML or Markdown in a template shows up as written.
