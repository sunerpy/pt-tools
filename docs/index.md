---
layout: home
markdownStyles: false
title: pt-tools：面向 PT 站点的自动化管理工具
titleTemplate: false
description: RSS 自动下载、刷流、IYUU 辅种、追剧与媒体库整理、多站点统计与下载器管理，在网页和手机 App 里统一操作，也可以在 QQ、Telegram 里查看和控制，或者交给 AI 助手。支持 Docker、Linux 和 Windows。

hero:
  name: pt-tools
  text: PT 站点的<wbr>自动化<wbr>管理工具
  tagline: 自动下载、刷流与 IYUU 辅种，追剧并整理进媒体库；汇总各站点的数据与登录状态，管理 qBittorrent 与 Transmission，在网页、手机 App、QQ 和 Telegram 里查看和控制。
  actions:
    - theme: brand
      text: 安装
      link: /guide/install
    - theme: alt
      text: 快速开始
      link: /guide/quick-start
    - theme: alt
      text: 1.0 预览版新功能
      link: /guide/whats-new-1.0

home:
  facts:
    - term: 运行方式
      text: Docker、Linux、Windows，amd64 与 arm64
    - term: 数据位置
      text: 存在你自己的服务器上，Cookie 加密保存
    - term: 文档版本
      text: 按 1.0 预览版编写，正式版仍是 0.48.0

  stages:
    steps:
      - 订阅一部剧，锁定一个版本
      - 新的一集下载完成，自动整理进剧集库
    screens:
      - src: /screens/media-follow-light.webp
        dark: /screens/media-follow-dark.webp
        width: 1280
        height: 1060
        alt: 剧集「长夜将尽」的条目详情页。按钮显示「追剧中 · 第 2 季」，下面「你的 NAS 上」一栏写着库里第 1 季 12 集、第 2 季 6 集，做种 4 个种子，追剧中 · 第 2 季、版本锁 HHWEB 2160p。
      - src: /screens/media-history-light.webp
        dark: /screens/media-history-dark.webp
        width: 1280
        height: 1060
        alt: 整理历史页。最上面一条是长夜将尽 S02E07：按订阅的条目，种子里的 The.Long.Night.S02E07.2160p.WEB-DL.H265-HHWEB.mkv 硬链接到 /media/剧集/长夜将尽 (2025)/Season 02/长夜将尽 - S02E07 - 失踪者.mkv，下载完成后自动整理，刮削成功，状态已整理；下面是第 6 集的记录。
    caption: 演示数据：「长夜将尽」是编的剧，海报和背景图是生成的。两张图依次是条目详情页与整理历史，都是网页的窄屏布局。追剧按播出时间去各站点搜同一个版本，下载完成后按模板硬链接进媒体库，写好 NFO 与海报。

  capabilities:
    title: pt-tools 能做什么
    intro: 标「预览版」的功能在 1.0 预览版里，尚未包含在正式版 0.48.0 中，新增内容见「1.0 预览版新功能」。点卡片的标题看对应的文档。
    items:
      - icon: folder-open
        title: 追剧与整理入库
        wide: true
        status: preview
        body: 订阅电影或追一季剧集，按播出时间到各站点去搜，只要同一个版本，洗版换更好的；下载完按模板硬链接进媒体库，写好 NFO 与海报，通知 Emby、Jellyfin、Plex 扫描。条目详情一页看库里有几集、在做种的种子能不能删，日历按天列出更新。
        evidence:
          kind: code
          text: 长夜将尽 (2025)/Season 02/长夜将尽 - S02E07 - 失踪者.mkv
        link: /guide/media-subscribe
      - icon: globe
        title: 站点与数据
        status: available
        body: 内置 67 个站点；用户统计页汇总每个站点的上传、分享率、做种与等级进度，跨站点搜索，长期没登录、快要封号之前提醒。
        evidence:
          kind: chips
          items: [Cookie, API Key, Passkey]
        link: /sites
      - icon: rss
        title: RSS 自动下载
        status: available
        body: 定时拉取订阅，没关联过滤规则时只下载免费种子；规则可以限定体积，免费期结束时尚未下完的按订阅的选择暂停或删除。
        evidence:
          kind: chips
          items: [关键词, 通配符, 正则]
        link: /guide/rss-subscription
      - icon: trending-up
        title: 刷流与辅种
        status: preview
        body: 刷流任务从站点的种子列表里按条件挑免费新种，按做种时长、分享率与上传速度删除；IYUU 辅种把别的站点上数据相同的种子加进同一台下载器，校验到 100% 才做种。
        evidence:
          kind: code
          text: 列表 50 个，符合条件 4 个，加入 2 个
        link: /guide/brush
      - icon: server
        title: 下载器
        status: available
        body: 添加多台 qBittorrent 和 Transmission，按 RSS 订阅、站点或默认下载器的顺序决定推给哪一台；推送前检查剩余空间与站点做种上限，自动删种时保护 H&R 种子。
        evidence:
          kind: chips
          items: [qBittorrent, Transmission]
        link: /configuration
      - icon: smartphone
        title: 手机 App 与远程访问
        status: preview
        body: Android App 在外面也能看概览、种子、站点与订阅。扫码配对，先试直连，连不上经 relay 连回 pt-tools；连接端到端加密，不用把端口开放到公网，relay 也可以自己部署。
        evidence:
          kind: code
          text: pt-tools relay serve
        link: /guide/mobile-app
      - icon: sparkles
        title: AI 助手与 MCP
        wide: true
        status: preview
        body: 接一个 OpenAI 兼容的大模型，在网页、手机 App 或 Telegram、QQ 里用自然语言查数据、搜索与推送种子、管理订阅；Claude、Cursor 这类 MCP 客户端也能接进来。改动类操作每次都要你确认。
        evidence:
          kind: chips
          items:
            [DeepSeek, 通义千问, Kimi, 硅基流动, Google AI Studio, Groq, OpenRouter, Ollama, OpenAI]
        link: /guide/assistant
      - icon: bell
        title: QQ、Telegram 与通知
        status: available
        body: 在 QQ、Telegram 里查看运行状态和种子，暂停、恢复或删除种子，添加 RSS 订阅，每条命令都记入操作审计；RSS 上新推送支持静默时段、合并摘要和每小时配额。
        evidence:
          kind: chips
          items: [/status, /tasks, /torrents, /addrss]
        link: /guide/chatops-quickstart
      - icon: house
        title: 首页与 NAS 监控
        status: preview
        body: 首页把今天播出的集、要处理的事、下载器与存储放在一起；NAS 概览读本机、TrueNAS、Glances 的 CPU、内存、网络与存储，群晖与飞牛是实验性的来源。
        evidence:
          kind: chips
          items: [本机, TrueNAS, Glances, 群晖, 飞牛]
        link: /guide/home-and-health
      - icon: archive
        title: 升级与备份
        status: preview
        body: 每天定时备份数据库，换成新版本第一次启动时先自动备份一次；可以在网页上立即备份、下载与恢复。二进制部署可以在网页里升级。
        evidence:
          kind: code
          text: pt-tools db backup
        link: /guide/upgrade#备份

  showcases:
    - title: 刷流任务
      intro: 从站点的种子列表里按条件挑免费新种做种，按做种时长、分享率、上传速度这些规则删除。每一轮读到了什么、为什么没要都记在运行记录里。
      screen:
        src: /screens/brush-light.webp
        dark: /screens/brush-dark.webp
        width: 1764
        height: 980
        alt: 刷流任务页。顶部是在做种子 11、今日上传 87.7 GB、今日下载 29.82 GB、累计分享率 2.97；下面列出 HDSky 免费刷流与 OurBits 刷流两个任务，各自的下载器、在做、今日上传与下载、累计上传和上次运行；左下是最近 30 天收益的柱状图，右下是「刷流怎么工作」的说明。
      caption: 演示数据。网页上的「刷流任务」页。
      points:
        - title: 默认只要免费
          body: 默认只收免费与 2x 免费、排除 H&R；站点开了整站 H&R 时，整站的种子都按 H&R 处理。
        - title: 和 RSS 一样的检查
          body: 加种之前同样检查磁盘预留与站点做种容量，删种只动这个任务加的种子。
        - title: 收益按周期看
          body: 按日、周、月、季、年看上传与下载，最近 30 天每天一根柱子。
      guide:
        text: 刷流任务
        link: /guide/brush
    - title: 手机 App
      intro: Android App 扫码配对以后，在外面也能看概览、种子、站点与订阅，按配对时给的权限暂停、删除、推送种子或管理订阅。
      screen:
        src: /screens/app-overview-light.webp
        dark: /screens/app-overview-dark.webp
        width: 780
        height: 1688
        alt: 手机 App 的概览页：上传 10.5 TiB、下载（计费）2.0 TiB、分享率 5.25、魔力 4,567.8、做种 8 个 1.0 TiB、站内信未读 2；今天上传 1.0 GiB；下载器做种 8 个 1.0 TiB，qb-nas 3 个、tr-nas 5 个。
      caption: 演示数据。App 的「概览」页。
      points:
        - title: 不用开放端口
          body: 先试直连，连不上经 relay 连回 pt-tools；连接端到端加密，relay 只转发、看不到内容。
        - title: 权限按设备给
          body: 配对时选「完全控制」或「只读」，之后可以随时改权限或撤销这台设备。
        - title: 跟着服务端一起发布
          body: 安装包和服务端发布在同一个 GitHub Release 里。
      guide:
        text: 手机 App
        link: /guide/mobile-app
    - title: AI 助手
      intro: 接一个 OpenAI 兼容的大模型，用自然语言问 pt-tools：追剧到哪一集了、今天上传了多少，让它搜索与推送种子、建订阅。
      screen:
        src: /screens/assistant-light.webp
        dark: /screens/assistant-dark.webp
        width: 1152
        height: 822
        alt: AI 助手的对话。问「长夜将尽第 2 季追到哪一集了？今晚更新的第 7 集下好了吗？」，助手调用了 2 个工具，回答第 7 集已经下好并硬链接进剧集库，第 1 到 6 集早已入库，第 8 集尚未播出。下面是输入框，选着模型 deepseek-v4-flash。
      caption: 演示数据。助手调用了两个工具，查追剧和整理历史。
      points:
        - title: 改动先确认
          body: 推送、删除、改订阅这类操作先弹出确认，列出每一项改以前、改以后与可能的影响，你点了才执行。
        - title: 凭证不交给模型
          body: Cookie、Passkey 与密码不会发给模型，工具查到的数据先脱敏。
        - title: 哪里都能问
          body: 网页、手机 App 都能对话，Telegram 与 QQ 的机器人也能把消息转给助手。
      guide:
        text: AI 助手
        link: /guide/assistant

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
    guide:
      text: 安装指南
      link: /guide/install

  privacy:
    title: 数据去向
    intro: pt-tools 的核心功能不依赖托管服务。下面是它会主动连接的对象；除了检查更新时访问 GitHub，其余都在你启用或配置以后才连接。
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
      - name: AI 模型服务
        sends: 对话内容与工具查到的数据
        detail: 启用 AI 助手以后才连接，只发给你选的服务商；Cookie、Passkey 与密码不会交给模型。
      - name: 远程访问的 relay
        sends: 加密后的 App 连接
        detail: 打开远程访问以后才连接。relay 留空时用内置的托管 relay，它只转发端到端加密的数据，看不到内容；也可以自建 relay 或只用直连。
      - name: 媒体与辅种服务
        sends: 片名、TMDB 编号与种子的 info hash
        detail: 用到媒体功能时连接 TMDB 与豆瓣的公开数据；开启 IYUU 辅种后，把下载器里已下完种子的 info hash 发给 IYUU 查询；CookieCloud 只连接你自建的服务。

  faq:
    title: 常见问题
    items:
      - q: 标「预览版」的功能怎么用上？
        a: 这些功能在 1.0 预览版（v1.0.0-rc.N）里，尚未包含在正式版 0.48.0 中。按「安装预览版」的步骤安装；预览版用于测试，升级前先备份数据目录。
        guide:
          text: 安装预览版
          link: /guide/install#安装预览版
      - q: AI 会不会自己执行写操作？
        a: 不会。内置 AI 助手的改动类操作每次都弹出确认，你点了才执行。MCP 的写工具第一次调用只回一份预览，你在客户端里同意以后，客户端的 AI 带上 `confirm=true` 再调一次才执行；令牌没有 MCP 写入权限时，这些工具都用不了。
        guide:
          text: MCP 的工具
          link: /guide/mcp#工具
      - q: MCP 能升级或重启 pt-tools 吗？
        a: 不能。MCP 的工具里没有升级、重启这类会中断服务的操作，新版本只能查看、不会升级。改配置要令牌另外勾上「配置」权限，也是先回预览、确认以后才保存。
        guide:
          text: MCP 接入
          link: /guide/mcp
      - q: 能和 MoviePilot、Sonarr、Seerr 一起用吗？
        a: 能，两个兼容入口都在 1.0 预览版里。qB 兼容入口让只认 qBittorrent 的工具把 pt-tools 当成一台 qBittorrent，加的种子照常经过磁盘空间和站点做种容量的检查；Servarr 兼容入口让 Seerr 把 pt-tools 当成 Radarr 与 Sonarr，请求的电影与剧集建成订阅和追剧。
        guide:
          text: qB 兼容入口
          link: /guide/qbit-compat
      - q: 手机在外面连回家里，要开放端口吗？
        a: 不用。App 先试直连，连不上再经 relay 连回 pt-tools；连接端到端加密，relay 只转发、看不到内容。relay 留空时用内置的托管 relay，也可以自己部署，或者只用直连。
        guide:
          text: 远程访问
          link: /guide/remote-access
      - q: 我用的站点不在支持列表里怎么办？
        a: 用浏览器扩展采集脱敏后的页面数据，按步骤提交新增站点的请求，不需要写代码。
        guide:
          text: 请求新增站点支持
          link: /guide/request-new-site

  closer:
    title: 安装 pt-tools
    body: Docker 镜像，以及 Linux 和 Windows 的二进制，采用 MIT 许可证。问题和功能建议请提交到 [GitHub Issues](https://github.com/sunerpy/pt-tools/issues)。
    actions:
      - text: 安装
        link: /guide/install
        theme: brand
        icon: download
      - text: 快速开始
        link: /guide/quick-start
        theme: alt
        icon: arrow-right
---

<HomeCapabilities />

<HomeShowcases />

<HomeDeploy>

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

安装脚本会下载最新版本，并按同一版本的 `checksums.txt` 校验 SHA-256 后再解压。

上面的命令安装的是正式版 0.48.0。要使用标「预览版」的功能，请按[安装预览版](/guide/install#安装预览版)里的步骤安装；新增内容见 [1.0 预览版新功能](/guide/whats-new-1.0)。

</HomeDeploy>

<HomePrivacy />

<HomeFaq />

<HomeCloser>

- 报告问题、提出功能建议：[GitHub Issues](https://github.com/sunerpy/pt-tools/issues)
- 使用交流：[GitHub Discussions](https://github.com/sunerpy/pt-tools/discussions)、[Telegram 群](https://t.me/+7YK2kmWIX0s1Nzdl)、QQ 群 274984594
- 微信公众号：六月水蓝

<QrCode src="/community/wechat-official-account.jpg" alt="微信公众号「六月水蓝」的二维码" caption="微信扫码关注" />

</HomeCloser>
