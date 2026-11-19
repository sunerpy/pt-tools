# Feishu app bot

This page explains how to connect a Feishu (Lark) **custom app bot** to pt-tools: message the bot in a private chat or @ it in a group to send ChatOps commands and talk to the AI assistant, and receive background notifications such as the daily report, sign-ins and series tracking in Feishu. pt-tools can create the app for you when you **scan a QR code** (next section), or you can create it by hand on the Feishu Open Platform (from section 1 on).

pt-tools receives events over the **long connection** of the official Feishu Go SDK: pt-tools connects out to the Feishu Open Platform, so no public address or open port is needed. Deployment is the same as with Telegram's long polling.

> [!NOTE]
> This page is about the Feishu app bot (`飞书应用机器人`, channel type `feishu_app`), which receives commands. The [Feishu group bot](notify-channels.md#feishu-lark-group-bot) in More notification channels is a custom bot in a group that only sends notifications and cannot receive commands; the two are configured separately.

---

## Creating the bot with a QR code (recommended)

Scan one QR code with Feishu and pt-tools creates the app bot in your organization, with the three permissions and the Receive messages v2.0 event already filled in. **There is no subscription mode to change and no version to publish**: the channel connects to Feishu as soon as it is created.

1. Open the pt-tools Web UI, go to ChatOps → Notifications (`消息通知`), click **Add channel** (`添加通道`) and select **Feishu app bot** (`飞书应用机器人`) as the channel type.
2. Keep the creation method at **Scan to create (recommended)** (`扫码一键创建（推荐）`), change the channel name if you like (`飞书应用机器人` by default), and click **Generate QR code** (`生成二维码`).
3. Scan the code with a Feishu account **in the organization that pt-tools should serve** (on a computer you can also click **Open in browser** (`在浏览器里打开`)) and confirm on Feishu's confirmation page. You can change the app name there.
4. After you confirm, pt-tools creates the channel and opens its detail page, and the channel list shows **Connected** (`已连接`). Send `/help` to the bot in a private chat to try it; see section 8.

What is filled in is the same as what sections 3 and 5 set up by hand: the permissions `im:message:send_as_bot`, `im:message.p2p_msg:readonly` and `im:message.group_at_msg:readonly`, and the event Receive messages v2.0 (`接收消息 v2.0`, `im.message.receive_v1`).

- **The person who scans becomes the administrator and is linked at once**: Administrator open_id (`管理员 open_id`) is set to their open_id, so there is no need to message the bot for an open_id or to generate a binding code, and commands work right away. To let someone else use the bot, add their open_id to the lists in the channel detail (see section 7) and bind them. If Feishu does not return the scanner's open_id, the channel is still created, with no administrator: get the open_id as in section 7, enter it, and bind it on the bindings page.
- The app is created **in the scanner's organization**. With the wrong account, or when that organization's platform policy refuses it, the channel shows Error (`异常`) and getting the tenant_access_token fails with `21001` in the run logs (see section 12); scan again with the right account.
- If the scanner uses Lark, the channel's platform is set to Lark (`Lark 国际版`); this has not been tested on Lark (see section 12).
- One app serves one pt-tools instance (see section 12). With several instances, scan once on each.
- pt-tools stores the App ID and App Secret itself, encrypted like the ones entered by hand, and shows them only in the channel detail after login.
- A QR code is valid for 10 minutes, and there is only one at a time: generating a new one or closing the dialog voids the previous one, unless you had already confirmed it in Feishu and pt-tools had already received the app at that moment; the channel is then created as usual and the page tells you. If closing the dialog fails to reach pt-tools to cancel (a network error), the page says so as well. While the previous one is still saving its channel, **Generate QR code** asks you to wait a few seconds. Do not scan a voided code: if it is scanned and confirmed, Feishu may still create an app. First check the channel list for a newly created channel; if there is none, delete the extra app in the [Feishu developer console](https://open.feishu.cn/app). Apps created by scanning are all named pt-tools by default, so tell them apart by App ID: an App ID that appears in a channel's details is in use, so do not delete it. Scan the code only from your own screen and do not send screenshots of it to others.
- If the organization requires an administrator to approve apps, the confirmation page says so and the app may not be created; in that case create it by hand as described below and ask an administrator to approve it.
- pt-tools sends the requests that create the app, and they follow only the `HTTPS_PROXY` and `NO_PROXY` environment variables: the channel does not exist yet, so its proxy URL cannot apply (see section 11).
- Scan to create is only in the web interface after you sign in: the mobile app and MCP do not have it, and when the AI assistant changes the configuration it can only create the channel from an App ID and App Secret.

The steps from section 1 on create the app by hand.

---

## 1. Before you start

- A Feishu organization account that can create a **custom app** (`企业自建应用`) on the [Feishu Open Platform](https://open.feishu.cn/app). The long connection only works for custom apps.
- Publishing an app needs approval from an administrator of the organization. For personal use you can create a separate test organization in which you are the administrator, so publishing does not wait for anyone else.
- The machine running pt-tools can reach the Feishu Open Platform at `open.feishu.cn` (`open.larksuite.com` for Lark), which it uses to get the long connection address, obtain access tokens and send messages, and the long connection address Feishu hands out (a host under `feishu.cn` or `larksuite.com`). If it needs a proxy, see section 11.

---

## 2. Creating the app and enabling the bot

1. Open the [Feishu Open Platform](https://open.feishu.cn/app), click Create custom app (`创建企业自建应用`), and enter a name (such as `pt-tools`) and a description.
2. Under Add features (`添加应用能力`), add Bot (`机器人`).
3. Under Credentials & Basic Info (`凭证与基础信息`), find the **App ID** (starting with `cli_`) and the **App Secret**. You need both in the next step.

> [!WARNING]
> The App Secret is the password of the app. Do not post it in public groups or screenshots, and do not commit it to a code repository. If it leaks, reset it under Credentials & Basic Info.

---

## 3. Granting permissions

Under Permissions & Scopes (`权限管理`), search for and enable these three:

| Permission                         | Name                                  | Required | Used for                                    |
| ---------------------------------- | ------------------------------------- | :------: | ------------------------------------------- |
| `im:message.p2p_msg:readonly`      | Read private messages sent to the bot |   Yes    | Sending commands in a private chat          |
| `im:message.group_at_msg:readonly` | Receive group messages that @ the bot |    No    | Sending commands by @-ing the bot in groups |
| `im:message:send_as_bot`           | Send messages as the app              |   Yes    | Replying to commands, background notices    |

pt-tools needs no contacts permission: it calls no contacts API and does not read the sender's name. When a group message @-mentions other people, the "@name" text in it is passed as is to commands and the AI assistant. A broader permission such as reading all group messages does no harm: pt-tools ignores every group message that does not @ this bot.

---

## 4. Creating the channel in pt-tools

Open the pt-tools Web UI, go to ChatOps → Notifications (`消息通知`) and click **Add channel** (`添加通道`):

1. Select **Feishu app bot** (`飞书应用机器人`) as the channel type and the creation method **Fill in App ID and App Secret** (`填写 App ID 与 App Secret`), enter a channel name, the App ID and the App Secret, keep the platform at Feishu (`飞书`, or choose Lark (`Lark 国际版`)), and click **Create channel** (`创建通道`).
2. The channel connects to Feishu as soon as it is created. Watch its status in the channel list: **Connected** (`已连接`) means the long connection is up, which the next step needs.
3. Open the channel detail and complete the fields below under **Credentials & connection** (`凭证与连接`), then click **Save credentials** (`保存凭证`). The channel reconnects with the new settings after saving.

| Field                                         | Value                    | Notes                                                                                                                                      |
| --------------------------------------------- | ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------ |
| App ID                                        | `cli_a1b2c3d4e5f6`       | The App ID under Credentials & Basic Info                                                                                                  |
| App Secret                                    | The app's App Secret     | Stored encrypted; shown only in the channel detail after login                                                                             |
| Platform (`平台`)                             | Feishu / Lark            | Feishu by default                                                                                                                          |
| Administrator open_id (`管理员 open_id`)      | open_ids, `ou_…`         | Separate several with commas or new lines; notifications go to the first one when no default chat ID is set                                |
| Allowed users' open_id (`允许的用户 open_id`) | open_ids, `ou_…`         | They can also talk to the bot                                                                                                              |
| Default chat ID (`默认会话 ID（可选）`)       | `oc_…`, `ou_…` or `on_…` | Where notifications such as the daily report go: `oc_` is a group (or a private chat), `ou_` a person's open_id, `on_` a person's union_id |
| Proxy URL (`代理 URL（可选）`)                | `http://127.0.0.1:1080`  | Used by this channel only, see section 11                                                                                                  |

- Both lists only accept open_ids (starting with `ou_`). A user_id, union_id or anything else is rejected on saving, with the offending entry named. To find your open_id, see section 7.
- **Only people in one of the two lists can talk to the bot**; with both lists empty, no message is handled. When someone outside the lists messages the bot privately, they get a reply with their open_id (at most one every 10 minutes per person); in groups the bot stays silent.
- A notification without a recipient goes to the first of: the default chat ID → the first administrator open_id → the first account linked on this channel. When none exists, the notification cannot be sent, and the channel section of ChatOps bindings (`ChatOps 绑定`) says why.

---

## 5. Subscribing to events over the long connection

Back on the app page of the Feishu Open Platform, open **Events & Callbacks** (`事件与回调`) → **Event configuration** (`事件配置`):

1. Set the subscription mode to **Receive events through persistent connection** (`使用长连接接收事件`) and save. Feishu checks for an online client when saving, so the pt-tools channel must show Connected at this point, or saving fails.
2. Click Add events (`添加事件`), then search for and add **Receive messages v2.0** (`接收消息 v2.0`, `im.message.receive_v1`).

Subscribe to this one event only. pt-tools does not handle other events: it answers them with a failure, Feishu pushes them again, and the run logs show a `not found handler` error (receiving commands is not affected). The exceptions are message read, the bot being added to or removed from a group, and a user's private chat with the bot being created for the first time: pt-tools accepts those and does nothing, without an error.

---

## 6. Publishing a version

Changes to permissions and events take effect only after a version is published:

1. Open **App Release** (`应用发布`) → **Version Management & Release** (`版本管理与发布`), click Create version (`创建版本`), and enter a version number and release notes.
2. Under Availability (`可用范围`), add the people (or departments) who will use the bot. People outside the availability cannot find the bot, and notifications pt-tools sends to them fail (error code 230013).
3. Submit for release and wait for an administrator to approve it. In a test organization, approve it yourself.

Publish a new version whenever you change permissions or event subscriptions later.

---

## 7. Getting an open_id

An open_id is a user's ID within this app and starts with `ou_`. **The same person has a different open_id in each app**, so the lists and bindings have to be set up again after switching to another app.

The easiest way: search for the bot in Feishu and send it a private message (such as `/help`). While you are not in the lists, the bot replies (in Chinese) that you are not in its lists, followed by your open_id:

```
你不在这个 pt-tools 机器人的名单里，发来的消息没有处理。
你的 open_id 是 ou_7f3a9c2d1e…
请 pt-tools 的管理员在通道设置里把它填进「管理员 open_id」或「允许的用户 open_id」。
```

Enter this open_id in Administrator open_id in the channel detail and save. You can also look it up in the API Explorer of the Open Platform (see [How to obtain an Open ID](https://open.feishu.cn/document/uAjLw4CM/ugTN1YjL4UTN24CO1UjN/trouble-shooting/how-to-obtain-openid)).

---

## 8. Binding and testing commands

### Sending a test message

Click **Send test** (`发测试消息`) on the channel card in the list, or **Send test message** (`发送测试消息`) under Connectivity test (`连通性测试`) in the channel detail. When the test message from the bot arrives in Feishu, sending works.

### Binding

The lists only decide who can talk to the bot; **running commands also requires binding**. Open ChatOps → ChatOps bindings (`ChatOps 绑定`):

- The Channels section at the top lists each account in the Feishu channel's lists and whether it is linked. Click **Link** (`绑定`) next to an account in the lists that is not linked, and it can use commands once you confirm.
- Or click **Generate binding code** (`生成绑定码`), choose the Feishu channel, get an 8-character code, and send `/bind <code>` to the bot in a **private chat**. The bot confirms the binding.

Currently **every linked account has administrator rights** and can pause or delete torrents and manage RSS subscriptions. Only give binding codes to people you trust.

### Testing commands

Send `/help` to the bot in a private chat; it replies with the full command list. Then send `/status` to check a read operation. For the command list, see [ChatOps quick start](chatops-quickstart.md#built-in-commands).

---

## 9. Using the bot in groups

1. For an app created by hand, enable the `im:message.group_at_msg:readonly` permission from section 3 and publish a version; an app created with a QR code already has it, so skip this step.
2. Add the bot to the group: group settings → Group bots (`群机器人`) → Add bot (`添加机器人`), then choose this app.
3. **@ the bot** followed by the command, such as `@pt-tools /status`. pt-tools ignores messages that do not @ the bot, including ones that only @ other people or other bots.

In a group, the sender also has to be in the lists and linked. Replies go to the group where every member can see them, binding codes included, so **link in a private chat**. To send notifications such as the daily report to a group, enter the group's chat ID (starting with `oc_`) as the default chat ID. You can take the chat ID from the run logs: when someone in the lists @-mentions the bot in the group, the `收到消息` (message received) line in the run logs has `chat_id=oc_…`, which is that group's chat ID.

---

## 10. The AI assistant and confirmations

After enabling Notification channels (`通知渠道（ChatOps）`) under System → AI assistant settings (`AI 助手设置`), text that does not start with `/` goes to the AI assistant (in groups, still @ the bot). For changes such as pausing or deleting torrents, the assistant first asks for confirmation; reply `确认` (confirm), or `确定`, `是`, `好的`, `ok` or `yes`, to run it, or anything else to cancel. Only the person who started the conversation can confirm; replies from other group members do not count. See [AI assistant](assistant.md#in-notification-channels).

Feishu has no card buttons here: confirmations are text replies, and new RSS notifications have no Download now / Ignore buttons (the same as QQ).

---

## <a id="11-proxy"></a>11. Proxy

Networks in mainland China can usually reach `open.feishu.cn` directly. If a proxy is needed, there are two ways:

- **Environment variables**: without a channel proxy, the Feishu app bot follows only `HTTPS_PROXY` and `NO_PROXY` (every Feishu address is https, so `HTTP_PROXY` has no effect). The proxy in the environment variable also has to be `http://` or `socks5://`; with a scheme such as `https://` the long connection cannot connect (sending messages still works). If the deployment sets `HTTPS_PROXY` for other sites and Feishu should connect directly, add `feishu.cn` (`larksuite.com` for Lark) to `NO_PROXY`.
- **Proxy URL of the channel**: enter it in Proxy URL in the channel detail, and only this channel uses it. `http://` and `socks5://` are supported, with the port included (such as `socks5://192.168.1.2:1080`); the long connection does not support proxies with the `https://` scheme.

---

## 12. FAQ

### Q: The bot does not react to my messages

Check in this order:

1. Is the channel status Connected (ChatOps → Notifications)?
2. For an app created by hand: is the subscription mode under Events & Callbacks set to the persistent connection, and is Receive messages v2.0 added?
3. For an app created by hand: after changing permissions and events, did you **publish a version** and get it approved?
4. Are you within the app's availability?
5. Private chats need `im:message.p2p_msg:readonly`, groups need `im:message.group_at_msg:readonly`, and in groups you must @ the bot.
6. Is your open_id in the channel's lists, and is your account linked before you run commands? (The channel section of ChatOps bindings shows the last interaction and its result.)

---

### Q: Saving the persistent connection mode fails

Feishu checks for an online client when saving this setting. Make sure the channel shows Connected in pt-tools, then save again on the Open Platform.

---

### Q: The channel status shows Error (`异常`)

Search the run logs (`运行日志`) for `飞书应用机器人`. Common causes:

- A wrong App ID or App Secret: the log has `长连接被飞书拒绝` (the long connection was rejected by Feishu) and `长连接出错` (a long connection error), followed by `1000040345: app_id or app_secret is invalid`; a test message fails with `code:10014` (`app secret invalid`). Correct it and save the credentials, and the channel reconnects at once.
- Getting the tenant_access_token fails with `21001` (`Access denied due to platform policy`): Feishu's platform policy refuses this app, most likely because it was created with the wrong account or organisation. Create the app again with an account in the organisation that pt-tools should serve.
- The bot feature of the app is not enabled: the status also shows Error while the bot info cannot be read. In the meantime pt-tools cannot tell whether a group message @-mentions the bot, so it asks Feishu to push group messages that @-mention anyone again later (per Feishu's docs at most 4 more pushes per message, each logging an Info line `还没取到机器人信息`) and handles them once the bot info is read; private chats work as usual.
- The same Feishu app is configured on two channels: only the one that started first connects, and the other shows Error, see the next question.

---

### Q: Can two pt-tools instances, or two channels, share one app?

**No**. The long connection works in cluster mode: when an app has several connections, each message goes to one of them at random, so the others miss it, which looks like messages being dropped at random. Use one app for one channel of one pt-tools instance; with several machines, create one app per machine.

---

### Q: After changing another notification setting, replies come a little later

Changing any notification channel reconnects all channels once:

- The message being handled at that moment is cancelled (part of it may already have run, and it is not retried); messages still waiting in line are handled after the reconnect, each exactly once. If the message being handled cannot stop right away (for example a slow downloader), the reconnect waits for it up to 10 seconds; after that it reconnects anyway, and that person's next messages still wait for it to finish while other people's messages go ahead.
- Several messages from the same person are handled in the order they were sent; a message still on its way at the moment of the reconnect is pushed again by Feishu later and is handled after the messages already handled by then.
- According to the Feishu documentation, messages sent during those few seconds are usually pushed again about 15 seconds later, and pt-tools handles them as usual without running them twice.

---

### Q: Notifications fail with error code 230013 or 230002

- **230013**: the recipient is outside the app's availability. Add them under Version Management & Release and publish a new version.
- **230002**: the bot is not in this group. Add the bot to the group, or change the default chat ID.

Other error codes and their causes appear in Recent notifications (`最近的通知`) and in the action audit.

---

### Q: The bot replies that a message was sent N minutes ago and was not run

According to the Feishu documentation, a message that gets no answer is pushed again after about 15 seconds, 5 minutes, 1 hour and 6 hours, so messages sent while pt-tools is offline can arrive after it comes back. Measured: with pt-tools stopped for 2 minutes, a message sent in that time arrived about 5 minutes after it was sent and was handled as usual. Messages that arrive more than 30 minutes after they were sent are not run, so that an old `/delete` does not take effect once pt-tools is back online. Send it again if you still need it.

---

### Q: Is Lark supported?

Yes. For an app created by hand, create it on the [Lark Open Platform](https://open.larksuite.com/app) with the same steps and choose Lark (`Lark 国际版`) as the platform when creating the channel; when the bot is created with a QR code and the person who scans uses Lark, the channel's platform is set to Lark automatically. This part follows the official documentation and **has not been tested** on Lark.
