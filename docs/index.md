---
layout: home
title: pt-tools：面向 PT 站点的自动化管理工具
titleTemplate: false
description: RSS 自动下载、多站点统计与搜索、下载器管理和 ChatOps，在一个 Web 界面中统一配置。支持 Docker、Linux 和 Windows。

hero:
  name: pt-tools
  text: PT 站点的自动化管理工具
  tagline: 按 RSS 订阅自动下载免费种子，汇总各站点的数据与登录状态，统一管理 qBittorrent 和 Transmission，并可以在 QQ 和 Telegram 里查看和控制。
  actions:
    - theme: brand
      text: 安装
      link: /guide/install
    - theme: alt
      text: 快速开始
      link: /guide/quick-start
    - theme: alt
      text: GitHub
      link: https://github.com/sunerpy/pt-tools

home:
  facts:
    - term: 运行方式
      text: Docker 镜像（amd64、arm64），以及 Linux 和 Windows 的二进制。macOS 请使用 Docker。
    - term: 数据位置
      text: 配置、任务记录和站点凭证都保存在你自己的服务器上；Cookie 用本机生成的密钥加密后存储。

  visual:
    desktop:
      light: /screens/home-light.webp
      dark: /screens/home-dark.webp
      width: 1440
      height: 900
      alt: pt-tools 的用户统计页：各站点的上传量、分享率、做种数、积分和等级。
    mobile:
      light: /screens/home-mobile-light.webp
      dark: /screens/home-mobile-dark.webp
      width: 375
      height: 812
      alt: 手机上的用户统计页：顶部是汇总指标，每个站点一张卡片。

  index:
    title: pt-tools 能做什么
    intro: 全部功能一览。标记为「实验性」的功能已经包含在正式版本中，但尚未完成端到端验证。
    groups:
      - name: 站点与数据
        items:
          - title: 66 个内置站点
            body: 覆盖 NexusPHP、mTorrent、Gazelle、HDDolby 和 Rousi 等架构，支持 Cookie、API Key 和 Passkey 三种认证方式。
            status: available
            link: /sites
          - title: 用户数据统计
            body: 在一个页面查看所有站点的上传量、下载量、分享率、做种、积分和等级进度，按今日、本周、本月看增量和走势，也可以导出为分享图。
            status: available
            link: /guide/user-stats
          - title: 每日战报
            body: 每天定时把各站当天的上传、下载、魔力增量和登录异常发到选定的通知通道，默认关闭。
            status: available
            link: /guide/user-stats#每日战报
          - title: 跨站点搜索
            body: 一次搜索多个站点，按体积、做种数和优惠筛选排序，结果可以直接推送到下载器。
            status: available
            link: /guide/what-is-pt-tools
          - title: 登录状态监控
            body: 按站点设置探测方式和提醒阈值，在长期未登录导致封号之前发出提醒。
            status: available
            link: /guide/site-login-monitoring
          - title: CookieCloud 导入
            body: 从自建的 CookieCloud 服务取回浏览器同步的 Cookie，在本机解密，预览后写进站点；也可以定时同步。
            status: available
            link: /guide/cookiecloud
      - name: RSS 自动下载
        items:
          - title: RSS 订阅
            body: 定时拉取订阅。未关联过滤规则时，只下载免费种子。
            status: available
            link: /guide/rss-subscription
          - title: 过滤规则与追剧
            body: 用关键词、通配符或正则匹配标题，可以限定体积和是否只要免费种子。
            status: available
            link: /guide/filter-rules-tv-series
          - title: 刷流任务
            body: 从站点的免费列表里按条件挑新种做种，按做种时长、分享率、上传速度等规则删除，限定同时下载数和体积，默认关闭。
            status: available
            link: /guide/brush
          - title: 免费结束自动暂停
            body: 种子的免费期结束时如果尚未下载完成，自动暂停，避免计入下载量。
            status: available
            link: /configuration
          - title: 磁盘保护与自动删种
            body: 推送前检查下载器的剩余空间；按做种时长、分享率等条件清理旧种子，并保护 H&R 种子。
            status: available
            link: /guide/auto-cleanup
      - name: 下载器
        items:
          - title: qBittorrent 与 Transmission
            body: 可以添加多个下载器，按 RSS 订阅、站点或默认下载器的顺序决定种子推送到哪一个。
            status: available
            link: /configuration
          - title: 下载器 Web UI
            body: 在 pt-tools 中查看所有下载器的任务、速度和状态，批量暂停、恢复或删除任务。
            status: available
            link: /guide/what-is-pt-tools
          - title: 下载目录
            body: 为每个下载器预设多个保存目录，推送时选择其中一个。
            status: available
            link: /configuration
          - title: 下载器助手
            body: 找出站点已删除的失效种子、按 tracker 给种子补站点标签、批量替换 tracker 地址，都是先预览再执行。
            status: available
            link: /guide/downloader-assistant
          - title: 转移做种
            body: 把下完的种子搬到另一台下载器继续做种，数据不动；校验到 100% 才从源下载器移除，也能按规则定时转移。
            status: available
            link: /guide/torrent-transfer
          - title: IYUU 辅种
            body: 用 IYUU 找出其他站点上数据相同的种子，核对文件后加进同一台下载器，校验到 100% 才开始做种。
            status: available
            link: /guide/reseed
      - name: 媒体
        items:
          - title: 整理入库
            body: 下载完的电影与剧集按模板硬链接进媒体库，写好 NFO 与海报，通知 Emby、Jellyfin、Plex 扫描，发入库通知。
            status: available
            link: /guide/media-library
          - title: 媒体识别
            body: 解析种子标题，到 TMDB 找对应的电影或剧集；记下站点给的 IMDb 与豆瓣编号，识别不对时可以手动纠正，也可以加识别词。
            status: available
            link: /guide/media-recognize
      - name: 通知与 ChatOps
        items:
          - title: QQ 与 Telegram 命令
            body: 查看运行状态、任务和种子，暂停、恢复或删除种子，添加和删除 RSS 订阅；每条命令都记入操作审计。
            status: available
            link: /guide/chatops-quickstart
          - title: RSS 上新通知
            body: 推送结果发送到聊天软件，支持静默时段、合并摘要、失败重试和每小时配额。
            status: available
            link: /guide/chatops-rss-notify
          - title: 更多通知通道
            body: 企业微信、钉钉、飞书群机器人，以及 Bark、Server 酱、ntfy；只发送通知，不接收命令。
            status: available
            link: /guide/notify-channels
          - title: 通用 Webhook
            body: 把通知以 JSON 发到自己的系统（HMAC 签名），接入 n8n、Zapier 等。
            status: experimental
            link: /guide/chatops-quickstart
      - name: 部署与维护
        items:
          - title: 浏览器扩展
            body: PT Tools Helper 可以一键同步站点 Cookie，并采集脱敏后的页面数据，用于申请新增站点。
            status: available
            link: /guide/browser-extension
          - title: 升级与清理
            body: 二进制部署可以在 Web 界面中升级；日志、暂存种子和旧备份可以先预览、再清理。
            status: available
            link: /guide/upgrade
          - title: 代理
            body: 访问站点和下载器时可以走 HTTP、HTTPS 或 SOCKS5 代理，并按地址排除。
            status: available
            link: /configuration

  steps:
    title: 从部署到第一次自动下载
    items:
      - title: 部署
        command: docker compose up -d
        body: 用 Docker Compose 启动，并持久化 /app/.pt-tools 目录；也可以直接运行二进制。
      - title: 添加下载器
        body: 在「下载器设置」中添加 qBittorrent 或 Transmission，并检查连通性。
      - title: 添加站点认证
        body: 用浏览器扩展同步 Cookie、从 CookieCloud 导入，或者在站点设置中填写 API Key、Passkey。
      - title: 订阅 RSS
        body: 为站点添加 RSS 订阅并选择下载器。未关联过滤规则时，只下载免费种子。

  rules:
    columns: [规则, 类型, 匹配的标题]
    rows:
      - pattern: 2160p|4K
        kind: 正则
        matches: 含有 2160p 或 4K 的标题
      - pattern: "*REMUX*"
        kind: 通配符
        matches: 任意位置出现 REMUX 的标题
      - pattern: 黑镜
        kind: 关键词
        matches: 标题或副标题包含「黑镜」
    caption: 规则按优先级从小到大依次匹配，命中即停止；每条规则还可以限定体积范围和是否只要免费种子。

  commands:
    items:
      - command: /status
        body: 运行状态：速度、磁盘和活跃任务数
      - command: /tasks
        body: RSS 任务列表及运行状态
      - command: /torrents
        body: 按下载器分页列出种子
      - command: /pause <hash>
        body: 暂停种子，支持 hash 前缀
      - command: /addrss
        body: 以对话方式添加 RSS 订阅
    caption: 只有用绑定码完成绑定的账号可以发送命令，目前每个绑定账号都拥有管理员权限，绑定码只发给信任的人。QQ 和 Telegram 通道里的名单只决定谁能与机器人对话，不决定管理员权限；默认每个账号每分钟最多 10 条。

  shots:
    sites:
      light: /screens/sites-light.webp
      dark: /screens/sites-dark.webp
      width: 1440
      height: 900
      alt: 站点列表：每个站点的状态、认证方式、登录状态和距离封禁阈值的剩余天数。

  deploy:
    title: 部署方式
    intro: 各种方式运行的是同一个程序，数据目录的格式相同，可以在不同方式之间迁移。
    columns: [方式, 架构, 获取方式, 数据目录]
    rows:
      - name: Docker
        status: available
        cells:
          - amd64、arm64
          - Docker Hub 的 sunerpy/pt-tools，或带构建证明的 ghcr.io/sunerpy/pt-tools
          - 挂载到 /app/.pt-tools
      - name: Linux
        status: available
        cells:
          - amd64、arm64
          - 发布页的压缩包，或会校验 SHA-256 的安装脚本
          - ~/.pt-tools
      - name: Windows
        status: available
        cells:
          - amd64、arm64
          - 发布页的压缩包，或 PowerShell 安装脚本
          - 用户目录下的 .pt-tools
    note: 不提供 macOS 二进制，macOS 请使用 Docker。默认监听 8080 端口。

  privacy:
    title: 数据去向
    intro: pt-tools 不依赖任何托管服务。下面是它会主动连接的对象，全部由你自己配置。
    sendsLabel: 发送
    modes:
      - name: PT 站点
        sends: Cookie、API Key 或 Passkey
        detail: 拉取 RSS、搜索、读取用户数据和探测登录状态时，请求只发往你添加的站点。
      - name: 下载器
        sends: 种子文件与控制指令
        detail: 推送种子、读取任务和剩余空间，只连接你配置的 qBittorrent 和 Transmission。
      - name: 通知通道与 GitHub
        sends: 通知内容与版本查询
        detail: 启用通知后，消息只发给你配置的通道；检查更新和升级二进制时访问 GitHub Releases。

  roadmap:
    title: 路线图
    intro: 正在设计、尚未发布的能力。设计文档记录的是接口契约，并不表示功能已经可用。
    items:
      - title: MCP Server
        body: 通过 MCP 协议向 AI 助手提供查询与需要确认的操作；目前只有接口契约，没有服务端实现。
        status: planned
      - title: AI Agent
        body: 在 ChatOps 之上处理需要多步判断的任务，每一步写操作都由用户确认。
        status: planned
      - title: 企业微信与通用 Webhook 验证
        body: 这两个出站通道已经实现，完成端到端验证后才会从「实验性」改为正式支持。
        status: planned
    notPlanned:
      title: 有意不做的事
      items:
        - 在 pt-tools 进程里内置聊天机器人或语言模型，或直接调用语言模型服务
        - 不经用户确认就自动执行写操作的 AI 模式
        - 通过 MCP 开放升级、重启服务这类破坏性操作
---

<HomeIndex />

<HomeSteps />

<SplitBlock proof="rules">

## RSS 自动下载，只下载需要的种子

pt-tools 定时拉取每个 RSS 订阅，把符合条件的种子推送到下载器。没有关联任何过滤规则的订阅只下载免费种子，适合刷流；关联了规则之后，只下载规则命中的种子，适合追剧或收集指定资源。

推送之前会检查下载器的剩余空间和站点的做种数量上限；免费期结束时尚未下载完成的种子会被自动暂停。

[RSS 订阅](/guide/rss-subscription) · [过滤规则与追剧](/guide/filter-rules-tv-series) · [自动删种与磁盘保护](/guide/auto-cleanup)

</SplitBlock>

<SplitBlock proof="screen" shot="sites" flip>

## 所有站点的数据，一个页面看完

用户统计页汇总每个站点的上传量、分享率、做种和等级进度；站点列表显示每个站点的认证方式、登录状态，以及距离长期未登录封禁阈值还剩多少天。

Cookie 站点可以用浏览器扩展一键同步，也可以从 CookieCloud 导入；M-Team 这类站点使用 API Key，Rousi Pro 使用 Passkey。

[支持站点](/sites) · [获取 Cookie 与 API Key](/guide/get-cookie-apikey) · [CookieCloud 导入](/guide/cookiecloud) · [登录状态与密钥备份](/guide/site-login-monitoring)

</SplitBlock>

<SplitBlock proof="commands">

## 在 QQ 和 Telegram 里查看和控制

通过 QQ（OneBot 协议，例如 NapCat）或 Telegram Bot 私聊发送命令，查看运行状态和种子，暂停或恢复任务；RSS 推送的结果也会发到聊天软件里。

聊天账号需要先用 Web 界面生成的一次性绑定码完成绑定。所有命令都记入操作审计，可以在 Web 界面中查询。

[ChatOps 快速开始](/guide/chatops-quickstart) · [RSS 上新通知](/guide/chatops-rss-notify)

</SplitBlock>

<HomeDeploy />

<HomePrivacy />

## 安装

推荐使用 Docker Compose。数据目录 `/app/.pt-tools` 必须持久化，其中的 `secret.key` 用来解密已保存的站点凭证。

::: code-group

```yaml [Docker Compose]
services:
  pt-tools:
    image: sunerpy/pt-tools:latest
    container_name: pt-tools
    environment:
      TZ: Asia/Shanghai
    ports:
      - "8080:8080"
    volumes:
      - ./data:/app/.pt-tools
    restart: unless-stopped
```

```bash [Linux]
curl -fsSL https://raw.githubusercontent.com/sunerpy/pt-tools/main/scripts/install.sh | sh
```

```powershell [Windows]
irm https://raw.githubusercontent.com/sunerpy/pt-tools/main/scripts/install.ps1 | iex
```

:::

安装脚本会下载最新版本，并按同一版本的 `checksums.txt` 校验 SHA-256 后再解压。[安装指南](/guide/install)介绍了各种方式的完整步骤，以及如何固定版本。

<HomeRoadmap />

## 社区

- 报告问题、提出功能建议：[GitHub Issues](https://github.com/sunerpy/pt-tools/issues)
- 使用交流：[GitHub Discussions](https://github.com/sunerpy/pt-tools/discussions)、[Telegram 群](https://t.me/+7YK2kmWIX0s1Nzdl)、QQ 群 274984594
- 微信公众号：六月水蓝

<QrCode src="/community/wechat-official-account.jpg" alt="微信公众号「六月水蓝」的二维码" caption="微信扫码关注" />
