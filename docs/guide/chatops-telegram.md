# Telegram Bot 配置指南

本文介绍如何通过 **Telegram Bot API（长轮询模式）** 将 Telegram 接入 pt-tools，实现私聊命令控制和系统通知推送。

---

## 1. 前置条件

- 一个 Telegram 账号
- **国内大陆用户**：需要代理才能访问 `api.telegram.org`。pt-tools 支持两种代理方式（见第 6 节）。
  - 读取系统环境变量 `HTTPS_PROXY`；
  - 在通道配置里填写 `proxy_url`，每个通道可以使用不同的代理。

---

## 2. 用 BotFather 创建 bot

1. 在 Telegram 搜索 `@BotFather`，点「Start」或直接发消息
2. 发送 `/newbot`
3. BotFather 提示输入 **bot 显示名称**（随便起，如 `My PT Tools Bot`）
4. 再输入 **bot 用户名**（全局唯一，必须以 `bot` 或 `Bot` 结尾，如 `mypts_bot`）
5. 创建成功后，BotFather 会返回类似这样的信息：

   ```
   Done! Congratulations on your new bot. You will find it at t.me/mypts_bot.
   Use this token to access the HTTP API:
   123456789:ABCdefGHIjklMNOpqrstUVWxyz
   ```

   冒号后面那一大串就是 `bot_token`，格式是 `数字ID:大小写字母数字串`，把它保存好。

> [!WARNING]
> bot_token 等同于 bot 的密码。不要把它发在公开频道或截图里，不要提交到代码仓库。

---

## 3. 获取你自己的 chat_id

`chat_id` 是 pt-tools 主动推送消息时需要的目标 ID。个人私聊的 chat_id 就等于你的 Telegram 数字 user_id。

**步骤：**

1. 先给你刚创建的 bot **发一条任意消息**（如 `hello`），激活会话
2. 在浏览器访问（注意替换 `<TOKEN>`）：

   ```
   https://api.telegram.org/bot<TOKEN>/getUpdates
   ```

3. 返回的 JSON 里找 `"from"` 或 `"chat"` 字段下的 `"id"`：

   ```json
   {
     "ok": true,
     "result": [
       {
         "message": {
           "from": {
             "id": 123456789,
             "first_name": "Alice",
             ...
           },
           "chat": {
             "id": 123456789,
             ...
           },
           "text": "hello"
         }
       }
     ]
   }
   ```

   这里的 `123456789` 就是你的 user_id，也是私聊的 `chat_id`。

> 💡 在返回的 JSON 中找 `from.id` 或 `chat.id`，那个数字就是你的 Telegram user_id。

> **没有 result？** 说明 bot 尚未收到任何消息，先去给 bot 发一条 `hello` 再访问 getUpdates。

---

## 4. Web UI 创建 Telegram 通道

打开 pt-tools Web UI，进入 ChatOps → 消息通知，点「**添加通道**」：

1. 通道类型选择 **Telegram**，填写通道名称和 Bot Token，点「**创建通道**」。
2. 在通道列表中点击刚创建的通道，进入通道详情。
3. 在「**凭证与连接**」中填写下面的字段，点「**保存凭证**」。保存后通道会立即用新配置重连。

![添加通知通道对话框](images/chatops/chatops-add-channel-dialog.webp)

> 「添加通道」对话框：选好通道类型，填通道名称和 Bot Token，点「创建通道」

### 字段语义说明

| 字段              | 作用                                                                       |
| ----------------- | -------------------------------------------------------------------------- |
| `allowed_users`   | 允许与 bot 对话的 TG user_id 列表                                          |
| `admin_users`     | 同样允许与 bot 对话；测试消息在没有填写 `default_chat_id` 时发给其中第一个 |
| `default_chat_id` | 主动推送的目标：pt-tools 主动发通知时投递到此 chat_id                      |

- **入站消息**：只有出现在 `allowed_users` 或 `admin_users` 里的 user_id 能与 bot 对话，其他人会收到一条拒绝消息（`denied:not_in_whitelist`）。**两者都留空时，任何人的消息都会被拒绝**；出站推送不受影响。
- **命令权限**：能对话之后，还要用绑定码完成绑定才能执行命令。目前**每个完成绑定的账号都拥有管理员权限**，可以执行 `/pause`、`/resume`、`/delete`、`/addrss`、`/delrss` 和 `/unbind`，与它在 `allowed_users` 还是 `admin_users` 里无关。所以绑定码只发给你信任的人。

**单人自用场景**：`admin_users = 你的 user_id`，`default_chat_id = 你的 user_id`，`allowed_users` 留空，然后用绑定码绑定自己的账号。

**多人共享场景**：把需要与 bot 对话的成员的 user_id 加入 `allowed_users`，只给需要发命令的成员发绑定码；没有绑定的成员发命令时，会收到「请先 /bind <绑定码> 完成绑定」的回复。

### 凭证字段

| 字段                      | 填写值                   | 说明                                          |
| ------------------------- | ------------------------ | --------------------------------------------- |
| Bot Token                 | `123456789:ABCdef...`    | 从 BotFather 拿到的 token                     |
| 允许用户（allowed_users） | 留空或其他成员的 user_id | 逗号分隔，见上方「字段语义说明」              |
| 管理员用户（admin_users） | `你的user_id`            | 逗号分隔，见上方「字段语义说明」              |
| 默认 Chat ID              | `你的user_id`            | 数字 user_id，或公开频道的 `@channelusername` |
| 代理 URL（可选）          | `http://127.0.0.1:1080`  | 如果需要代理才能访问 TG，填这里（见第 6 节）  |

长轮询超时固定为 30 秒，不需要设置。

![pt-tools Telegram 通道凭证配置](images/chatops/chatops-telegram-detail.webp)

> Web UI → Telegram 通道详情 → 凭证与连接，展示所有字段包括代理 URL

保存后通道状态显示「运行中」，表示通道已经启动；能否连上 Telegram 服务器，以下一节的测试消息为准。

---

## 5. 验证、绑定与测试命令

### 发送测试消息

在通道列表中点通道卡片上的「**发测试消息**」，或在通道详情的「连通性测试」中点「**发送测试消息**」。

如果你的 Telegram 里收到了一条来自 bot 的测试消息，说明出站推送正常。

### 生成绑定码并绑定

在 Web UI 打开 ChatOps → ChatOps 绑定，点「**生成绑定码**」：

- 选择刚配置的 Telegram 通道
- 有效期选 5 分钟（默认）
- 点「生成」

![生成绑定码对话框](images/chatops/chatops-bindings-dialog.webp)

> 绑定码生成后出现在「待绑定」列表，8 字符，不含 0/O/1/I/L 等容易混淆的字符

复制生成的 8 字符绑定码（如 `A3F7KP2M`），在 Telegram 私聊给 bot 发：

```
/bind A3F7KP2M
```

bot 回复「绑定成功」即表示完成。绑定完成后可以在「已绑定用户」列表看到你的账号。

> 绑定属于生成绑定码时选的那个通道：绑定码只能发给这个通道的 bot，发给别的 bot 会提示绑定失败；同一个账号要在多个通道上发命令，需要在每个通道上分别绑定。

![绑定管理列表](images/chatops/chatops-bindings-list.webp)

> 绑定成功后，账号出现在「已绑定用户」列表，显示通道类型、用户 ID（部分隐藏）和管理员标记

### 测试命令

发送 `/help`，bot 应该回复完整的命令列表。再发一条 `/status` 验证读取操作。

---

## 6. 代理配置详解

Telegram Bot API 的地址是 `api.telegram.org`，国内大陆网络无法直连。有两种代理方式：

### 方式一：环境变量（全局）

在 pt-tools 的启动环境里设置：

```bash
# 适用于 HTTP 代理（clash / v2ray 等工具的 HTTP 模式）
export HTTPS_PROXY=http://127.0.0.1:1080
```

或者在 Docker Compose 里：

```yaml
environment:
  HTTPS_PROXY: "http://127.0.0.1:1080"
```

这种方式会影响 pt-tools 发出的**所有** HTTPS 请求（包括站点访问、版本检查等），不只是 Telegram。

### 方式二：通道级 proxy_url（推荐）

在 Telegram 通道详情的「凭证与连接」里填写代理 URL（`proxy_url`），只有这个通道走代理，不影响其他通道和站点请求。

支持的代理格式：

| 代理类型           | 格式示例                                  |
| ------------------ | ----------------------------------------- |
| HTTP 代理          | `http://127.0.0.1:1080`                   |
| HTTPS 代理         | `https://proxy.example.com:8443`          |
| SOCKS5 代理        | `socks5://127.0.0.1:7890`                 |
| 带认证的 HTTP 代理 | `http://user:pass@proxy.example.com:3128` |

**常见本地代理工具对应地址：**

| 工具         | 默认 HTTP 端口                   | 默认 SOCKS5 端口          |
| ------------ | -------------------------------- | ------------------------- |
| Clash        | 7890                             | 7891（或 HTTP 也是 7890） |
| V2Ray / Xray | 1087（HTTP）                     | 1080（SOCKS5）            |
| Shadowsocks  | 1087（如果开启了本地 HTTP 代理） | 1080                      |

如果不确定端口，在代理工具的界面里查看「允许局域网连接」或「LAN 连接」部分即可。

---

## 7. 常见问题 FAQ

### Q: getUpdates 返回空 result（`"result": []`）

**原因**：bot 没有收到过任何消息，getUpdates 自然返回空。

**解决**：在 Telegram 里给 bot 发一条消息（任何内容都行），然后再刷新 getUpdates。

---

### Q: 通道状态显示「异常」，或测试消息发不出去

**原因一**：网络不通到 `api.telegram.org`，需要配置代理（见第 6 节）。

**检查**：在 pt-tools 所在机器上测试：

```bash
curl -x http://127.0.0.1:1080 https://api.telegram.org/bot<TOKEN>/getMe
```

如果代理正常，会返回 bot 的基本信息 JSON。

**原因二**：bot_token 填错了，多了空格或少了字符。注意 token 格式是 `数字:字母数字串`，冒号两侧没有空格。

---

### Q: 推送消息超时或报 send_message timeout

**原因**：pt-tools 到 Telegram API 服务器的网络不通，或代理配置有误。

**解决**：

1. 确认 `proxy_url` 填写正确
2. 在 pt-tools 所在机器手动测试代理是否可以访问 `api.telegram.org`
3. 如果用环境变量代理，注意环境变量要在启动 pt-tools 之前就设置好

---

### Q: bot_token 解密失败，日志出现「illegal base64 at input byte 0」

**原因**：ChatOps 正式发布（v0.31.0）之前的测试构建存在重复解密的问题，正式版本已修复。

**解决**：升级到最新版本，然后删除并重新创建 Telegram 通道，重新填入 token（旧的加密存储数据已损坏，必须重建）。

---

### Q: 两台 pt-tools 能共用同一个 bot 吗？

**不能**。Telegram 的长轮询机制要求同一时刻只有一个进程在轮询同一个 bot。如果两台机器用同一个 bot token，会相互抢占轮询，消息分发会变得随机，大部分命令不会得到回复。

如果你有多台机器，每台用一个独立的 bot（分别找 BotFather 申请不同的 bot token）。

---

### Q: 想在群组里用 bot，而不是私聊

1. 把 bot 拉进群组
2. 在群里发一条消息（或 `@bot` 发命令，取决于 bot 的隐私模式设置）
3. 访问 getUpdates，找到 `chat.id`（群组的 chat_id 是负整数，如 `-1001234567890`）
4. 把 `default_chat_id` 填为群的 chat_id，`allowed_users` 里填需要与 bot 对话的群成员的 user_id

注意：隐私模式开启时，bot 在群组里只能看到发给它的命令（以 `/` 开头或 `@botname` 提及）；关闭隐私模式才能看到所有消息。pt-tools 的使用场景推荐保持隐私模式开启（默认）。
