# More notification channels

Besides [QQ](chatops-qq-napcat.md) and [Telegram](chatops-telegram.md), which also take commands, pt-tools can push notifications to these outbound-only channels: WeCom group bots, DingTalk group bots, Feishu (Lark) group bots, Bark, ServerChan (Server 酱) and ntfy. They only send; you cannot send commands from them.

In ChatOps → Notifications (消息通知), click Add channel (添加通道), choose the type, enter a name and the required field, then fill in the rest in the channel's settings and click Send test message (发送测试消息) to check that it arrives. Credentials are stored encrypted like other channels and are not shown in the list.

## WeCom group bot

| Setting     | Meaning                                                                                           |
| ----------- | ------------------------------------------------------------------------------------------------- |
| Webhook Key | The part after `key=` in the bot address `https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=…` |

Messages are sent as Markdown. Error codes from WeCom (an invalid key, too many messages and so on) count as failed deliveries and are retried under the usual notification retry rules.

## DingTalk group bot

| Setting                   | Meaning                                                                                              |
| ------------------------- | ---------------------------------------------------------------------------------------------------- |
| Webhook address           | The full address copied from the bot settings, `https://oapi.dingtalk.com/robot/send?access_token=…` |
| Signing secret (optional) | The secret starting with `SEC` when the security setting is signing (加签)                           |
| Message format            | Markdown (default) or plain text                                                                     |

DingTalk requires at least one security setting per bot: with signing, enter the secret here; with custom keywords, every message must contain one of the keywords (for example in the notification content), or DingTalk rejects it; with an IP allowlist, add the public IP pt-tools sends from.

## Feishu (Lark) group bot

| Setting                   | Meaning                                                                                                                    |
| ------------------------- | -------------------------------------------------------------------------------------------------------------------------- |
| Webhook address           | The address copied from the bot settings, `https://open.feishu.cn/open-apis/bot/v2/hook/…` (`open.larksuite.com` for Lark) |
| Signing secret (optional) | Enter it when signature verification (签名校验) is on                                                                      |

The signature uses the current time; Feishu rejects messages when the clock of the machine running pt-tools is more than an hour off.

## Bark

| Setting                   | Meaning                                                                            |
| ------------------------- | ---------------------------------------------------------------------------------- |
| Device Key                | The part of the push address in the Bark app, `https://api.day.app/<Device Key>/…` |
| Server address (optional) | `https://api.day.app` by default; your own address for a self-hosted bark-server   |
| Group, sound (optional)   | The group and sound of the notification in Bark                                    |

## ServerChan

| Setting | Meaning                                                                                                      |
| ------- | ------------------------------------------------------------------------------------------------------------ |
| SendKey | Both the Turbo `SCT…` keys and the ServerChan³ `sctp…` keys work; pt-tools picks the address from the format |

ServerChan titles are limited to 32 characters; a longer title is shortened and the full title goes on the first line of the message.

## ntfy

| Setting                                          | Meaning                                                               |
| ------------------------------------------------ | --------------------------------------------------------------------- |
| Topic                                            | Letters, digits, `-` and `_`, up to 64 characters                     |
| Server address (optional)                        | `https://ntfy.sh` by default; your own address for a self-hosted ntfy |
| Access token / user name and password (optional) | One of them when the server has access control on                     |
| Priority (optional)                              | 1 lowest, 5 highest, 0 for the server default                         |

Topics on the public `ntfy.sh` server have no password, and anyone who knows the name can subscribe: pick a name that is hard to guess, or use your own server with access control.
