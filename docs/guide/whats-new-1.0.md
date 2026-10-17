# 1.0 预览版新功能 <Badge type="tip" text="1.0 预览版" />

1.0 预览版（版本号形如 `1.0.0-rc.N`，更早的几个预览版是 `0.49.0-rc.N`）在 0.48.0 之后加入了刷流、IYUU 辅种、媒体库整理、手机 App、远程访问、AI 助手等功能。本站文档按预览版编写：标「1.0 预览版」的页面和首页标「预览版」的功能只在预览版里，0.48.0 正式版尚未包含；1.0 正式版发布后会包含全部这些功能。

预览版用于测试，不建议直接用在生产环境。安装方法见[安装预览版](./install.md#安装预览版)，升级前先备份数据目录。每个预览版的变更见 [GitHub Releases](https://github.com/sunerpy/pt-tools/releases) 中标为 Pre-release 的版本。

## 刷流与做种

- [刷流任务](./brush.md)：从站点的免费列表里按条件挑新种做种，按做种时长、分享率、上传速度等规则删除，限定同时下载数和体积；有 H&R 的种子按站点规则或站点设置里的做种要求处理。
- [IYUU 辅种](./reseed.md)：用 IYUU 找出其他站点上数据相同的种子，核对文件后加进同一台下载器，校验到 100% 才开始做种，数据只存一份。
- [转移做种](./torrent-transfer.md)：把下完的种子搬到另一台下载器继续做种，数据文件不动；校验到 100% 才从源下载器移除，也能按规则定时转移。
- [下载器助手](./downloader-assistant.md)：找出站点已删除的失效种子、给缺站点标签的种子补标签、批量替换 tracker 地址，都先预览再执行。
- [qB 兼容入口](./qbit-compat.md)：MoviePilot、autobrr、Sonarr、Radarr 这类只认 qBittorrent 的工具可以把 pt-tools 当成一台 qBittorrent，加的种子照常经过磁盘空间和站点做种容量的检查。

## 媒体库

- [订阅](./media-subscribe.md)：订阅电影或剧集的一季，定时到各站点搜索，RSS 里出现时也下载；按质量档案挑资源，剧集补齐缺的集，洗版换更好的版本，豆瓣「想看」自动建订阅。
- [整理入库](./media-library.md)：下载完的电影与剧集按命名模板硬链接进媒体库，写好 NFO 与海报，通知 Emby、Jellyfin 或 Plex 扫描；可以按二级分类放进子目录、按策略替换同名的旧版本、监控目录自动整理，刮削内容可以分别开关，简介与评分可以用豆瓣的。
- [媒体识别](./media-recognize.md)：解析种子标题，到 TMDB 找对应的电影或剧集，记下站点给的 IMDb 与豆瓣编号；识别不对时可以手动纠正或加识别词，TMDB 的接口地址与图片地址可以更换。

## 手机 App 与接口

- [手机 App](./mobile-app.md)：Android App 在外面也能看概览、种子、站点与订阅，按配对时给的权限暂停、删除、推送种子或管理订阅，也能和 AI 助手对话；从 1.0.0-rc.28 起，安装包和服务端发布在同一个 Release 里。
- [远程访问](./remote-access.md)：手机扫网页上的二维码配对，App 先试直连，连不上再经 relay 连回 pt-tools；连接端到端加密，relay 只转发。默认用内置的托管 relay，也可以自建或关掉 relay 只用直连。
- [API 令牌](./api-tokens.md)与 [App API](../reference/app-api.md)：脚本和客户端带令牌访问 `/api/app/v1/`，按权限范围读取或修改，不用保存网页的登录密码。
- [MCP](./mcp.md)：Claude、Cursor、Cherry Studio 这类 MCP 客户端里的 AI 助手可以查看种子、下载器、站点数据与订阅，在你确认以后执行改动。

## 通知与 AI 助手

- [AI 助手](./assistant.md)：接一个 OpenAI 兼容的大模型（也支持 Groq 与 Google AI Studio），在网页、手机 App 或 Telegram、QQ 里用自然语言查数据、搜索与推送种子、添加订阅；改动类操作每次都要你确认。
- [更多通知通道](./notify-channels.md)：钉钉、飞书群机器人，以及 Bark、Server 酱、ntfy；「最近的通知」列出 RSS 与后台通知，投递统计按来源写明。
- [消息模板](./notify-channels.md#消息模板)：按事件自定义通知的标题与正文，命令回复与 AI 助手的回复也可以套用。
- [自定义 Webhook](./notify-channels.md#自定义-webhook)：自定义请求方法、格式与请求体模板，内置 Discord、Slack、Gotify 等预设，可以按响应内容判断是否发送成功。

## 站点与数据

- [数据统计与每日战报](./user-stats.md)：成功获取站点数据的当天存一份快照，按今日、本周、本月看增量和走势；每日战报（默认关闭）可以每天把当天的增量发到选定的通知通道。
- [每日签到](./site-login-monitoring.md#每日签到)：在设定的时间窗里给支持签到的站点签到，也可以用 `/signin` 命令；HDArea 等站点使用各自的签到接口。
- [CookieCloud 导入](./cookiecloud.md)：从自建的 CookieCloud 取回浏览器同步的 Cookie，在本机解密，预览后写进站点，也可以定时同步；浏览器扩展可以定时同步 Cookie，并加密上传到 CookieCloud。
- [NAS 部署](./install.md#nas)：1Panel、Unraid 与飞牛 fnOS 的应用定义。
- 站点解析：CrabPT、GameGamePT、HDArea、OpenCD 的积分、分享率与时魔，以及多个站点在分享率不到 1 时的取值都已修正；没有 H&R 做种时长的站点可以在站点设置里填写做种要求。

## 可靠性与安全

- [免费资格](../configuration.md#全局设置)：要求免费的推送用这一轮读到的免费状态判断，状态未知不算免费，剩余免费时间不够也不推送；免费结束时间未知时不自动开始下载；可以设置在免费结束前提前几分钟暂停或删除没下完的种子。
- [自动删种](./auto-cleanup.md)：可以先预览这一轮会删哪些种子，页面写明上一轮的结果；有 H&R 但做种要求未知的种子会被保护。
- 凭证：站点 Cookie 只以密文存储，升级时会清除旧版本留下的明文；密钥文件损坏时不再被覆盖，密钥与已保存的密文对不上时拒绝启动。
- 升级：自升级会按同一版本的 `checksums.txt` 核对下载的文件；检查更新时，旧的预览版不再被当成新版本。
