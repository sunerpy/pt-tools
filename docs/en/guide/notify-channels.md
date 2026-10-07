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
