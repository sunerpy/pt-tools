# 更多通知通道

除了能收发命令的 [QQ](chatops-qq-napcat.md) 与 [Telegram](chatops-telegram.md)，pt-tools 还能把通知推送到下面这些只出站的通道：企业微信群机器人、钉钉群机器人、飞书群机器人、Bark、Server 酱和 ntfy。它们只发不收，不能在里面发命令。

在「ChatOps → 消息通知」里点「添加通道」，选类型、填名称和各项设置。用自建服务器的 Bark、ntfy 要在这一步就填好服务器地址和鉴权：通道创建后立即启用，留空就会先发到默认的公共服务器。创建后点「发送测试消息」确认能收到，之后可以在通道的「设置」里修改。凭据和其他通道一样加密保存，列表里不显示。

保存时会检查各项设置，格式不对（比如 Webhook 地址不是对应服务的地址、ntfy 的 topic 含有 `.`）时不会保存，并写明哪一项不对。

发送时：

- 服务器返回的不是对应服务的成功响应（比如地址填成了别的网站）时算发送失败，按通知的重试规则再试。
- 不跟随跳转：服务器要求跳转时算发送失败，请填写跳转后的地址。
- 默认不连本机和内网地址（`127.0.0.1`、`192.168.x.x` 等），链路本地地址（含云服务器的元数据地址 `169.254.169.254`）一律不连。
- 错误信息里不带 Webhook 地址、Key、Token 和密码。

## 企业微信群机器人

| 项目        | 说明                                                                                       |
| ----------- | ------------------------------------------------------------------------------------------ |
| Webhook Key | 群机器人地址 `https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=…` 里 `key=` 后面那一段 |

消息按 Markdown 发送。企业微信返回的错误码（key 无效、发送太频繁等）会记为发送失败，按通知的重试规则再试。

## 钉钉群机器人

| 项目             | 说明                                                                                            |
| ---------------- | ----------------------------------------------------------------------------------------------- |
| Webhook 地址     | 机器人设置里复制的完整地址，只能是 `https://oapi.dingtalk.com/robot/send?access_token=…` 的形式 |
| 加签密钥（可选） | 安全设置选了「加签」时填 `SEC` 开头的密钥                                                       |
| 消息格式         | Markdown（默认）或纯文本                                                                        |

钉钉要求每个机器人至少开一种安全设置：选「加签」时把密钥填进来；选「自定义关键词」时，消息里要含有其中一个关键词（可以把通道名称或关键词写进通知内容），否则钉钉会拒收；选「IP 地址（段）」时填 pt-tools 出口的公网 IP。

## 飞书群机器人

| 项目             | 说明                                                                                                            |
| ---------------- | --------------------------------------------------------------------------------------------------------------- |
| Webhook 地址     | 机器人设置里复制的地址，只能是 `https://open.feishu.cn/open-apis/bot/v2/hook/…`（Lark 是 `open.larksuite.com`） |
| 签名密钥（可选） | 安全设置开了「签名校验」时填                                                                                    |

签名用到当前时间，pt-tools 所在机器的时间差超过一小时时飞书会拒收。

## Bark

| 项目               | 说明                                                                      |
| ------------------ | ------------------------------------------------------------------------- |
| Device Key         | Bark App 里推送地址 `https://api.day.app/<Device Key>/…` 中的那一段       |
| 服务器地址（可选） | 默认 `https://api.day.app`；自建 bark-server 时填自己的地址               |
| 允许内网地址       | 自建服务器在本机或内网（如 `192.168.x.x`、NAS 上的 Docker）时打开，默认关 |
| 分组、铃声（可选） | 通知在 Bark 里的分组与铃声                                                |

## Server 酱

| 项目    | 说明                                                                         |
| ------- | ---------------------------------------------------------------------------- |
| SendKey | Turbo 版的 `SCT…` 与 Server 酱³ 的 `sctp…` 都可以，pt-tools 按格式选发送地址 |

Server 酱的标题最多 32 个字，超出时截短，完整标题放在正文第一行。

## ntfy

| 项目                              | 说明                                             |
| --------------------------------- | ------------------------------------------------ |
| Topic                             | 字母、数字、`-` 和 `_`，最多 64 个字符           |
| 服务器地址（可选）                | 默认 `https://ntfy.sh`；自建 ntfy 时填自己的地址 |
| 允许内网地址                      | 自建服务器在本机或内网时打开，默认关             |
| Access Token / 用户名密码（可选） | 服务器开了访问控制时填其中一种                   |
| 优先级（可选）                    | 1 最低、5 最高，0 用服务器的默认值               |

公共服务器 `ntfy.sh` 上的 topic 没有密码，知道名字的人都能订阅：起一个不好猜的名字，或者用自建服务器并开启访问控制。

## 消息模板

每种通知的标题和正文都可以改成自己的写法，所有通知通道（QQ、Telegram、群机器人、Bark 等）都按它发。在「ChatOps → 消息模板」里选一种事件，填「标题模板」或「正文模板」，点「保存」。空着的一项用内置文字；点「恢复默认」回到内置的写法。

模板用 Go 模板的写法：变量写成 `.site` 这样，放在两对花括号里，换成变量的值；`if` 是条件，`range` 遍历列表。页面上的「变量」列出这种事件能用的全部变量，点一下插到光标处；「预览（示例数据）」按示例数据实时渲染；「内置的样子」是不改时发出去的内容。

例：RSS 命中过滤规则时只发一行；签到结果只列没签上的站点。

```text
{{.site}}｜{{.torrent_title}}｜{{.size}}{{if .free}}｜免费，还剩 {{.free_remaining}}{{end}}
```

```text
{{.day}} 签到：成功 {{.signed}}，失败 {{.failed}}
{{range .items}}{{if eq .status "failed"}}· {{.site}}：{{.message}}
{{end}}{{end}}
```

每种事件都有的变量：`event`（事件类型）、`event_name`（事件名称）、`time`（本地时间，形如 `2026-10-12 08:00:00`）、`timestamp`（秒级时间戳）、`title`、`text`（内置的标题与正文，可以在自己的模板里接着用）。各事件的主要变量：

| 事件                                       | 主要变量                                                                                                                   |
| ------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------- |
| `rss.new` RSS 新种子                       | `site`、`rss`、`torrent_title`、`torrent_id`、`link`、`published`                                                          |
| `rss.matched` RSS 命中过滤规则             | 同上，再加 `subtitle`、`size`、`size_bytes`、`free`、`free_end`、`free_remaining`、`rule`、`seeders`、`leechers`、`has_hr` |
| `rss.digest` RSS 合并通知                  | `count`，`items`（每项 `title`、`site`、`torrent_title`、`torrent_id`）                                                    |
| `subscribe.downloaded` 订阅下载            | `subscription`、`media_type`、`year`、`season`、`episodes`、`torrent_title`、`site`、`quality`、`poster`                   |
| `subscribe.douban_failed` 豆瓣想看拉取失败 | `source`、`fails`、`error`                                                                                                 |
| `media.organized` 入库                     | `name`、`media_type`、`episodes`、`files`、`quality`、`library`、`torrent`、`poster`                                       |
| `site.login_expiring` 站点即将被封号       | `site`、`tier`、`days_remaining`、`last_active`、`days_since_active`、`access_stale`                                       |
| `site.probe_failed` 站点探测失败           | `site`、`status`、`status_label`、`since`、`advice`、`diagnosis`                                                           |
| `site.probe_recovered` 站点探测恢复        | `site`、`probed_at`                                                                                                        |
| `site.login_test` 登录提醒测试             | `site`、`tier`、`days_remaining`、`last_active`                                                                            |
| `attendance.daily` 签到结果                | `day`、`signed`、`failed`，`items`（每项 `site`、`status`、`status_label`、`message`）                                     |
| `report.daily` 每日战报                    | `day`、`uploaded`、`downloaded`、`uploaded_bytes`、`downloaded_bytes`、`bonus`，`items`（各站点）                          |
| `downloader.dead_torrents` 失效种子        | `downloader`、`count`、`size`、`size_bytes`，`items`（每项 `name`、`size`、`size_bytes`）                                  |
| `remote.paired` 新设备配对                 | `device`、`permission`                                                                                                     |
| `test` 通道测试                            | `channel`                                                                                                                  |

每日战报的变量只有合计与各站点；登录异常、签到、刷流几段在 `text` 里。列表最多 100 项，省略的条数在 `items_more` 里。

能用的函数：比较 `eq`、`ne`、`lt`、`le`、`gt`、`ge`（整数与小数可以直接比），`default`（为空时用默认值），`truncate`（按字符截断，截掉了补「…」），`contains`、`hasPrefix`、`hasSuffix`，`replace`、`upper`、`lower`、`trim`，`bytes`（字节数写成 `15.00 GB`），`date`（按 Go 的版式写时间），`printf`，`json`（写成 JSON 字面量）。写法举例：

```text
{{if gt .seeders 10}}热门{{end}}
{{.rule | default "无"}}
{{.text | truncate 100}}
{{if contains "2160p" .torrent_title}}4K{{end}}
{{bytes .size_bytes}}
{{date "01-02 15:04" .timestamp}}
{{printf "%.1f" .bonus}}
```

保存时会用示例数据渲染一遍：变量名写错、模板语法不对时不会保存，并写明第几行哪里不对。为了不拖慢通知，有几条限制：`range` 只能遍历列表变量，最多嵌套两层，两层循环里只能放几个简单的动作；一份模板最多声明 64 个变量；模板最长 8 KB；渲染出来的标题最长 1 KB、正文最长 16 KB；`printf` 的宽度与精度最大 256，不支持 `*` 与 `[n]` 的写法；一个函数的参数合计与结果都最长 64 KB，一次渲染里所有函数处理的内容合计最多 4 MB。发送时万一渲染出错或结果是空的，这条通知照内置文字发，不会因为模板写坏而发不出去。

模板只改通知，不改 ChatOps 命令和 AI 助手的回复。Telegram 按纯文本发，模板里写的 HTML、Markdown 标记会原样显示。
