# What is pt-tools

This page explains what pt-tools is for, how it works and what each page of its web interface does, so you can decide whether it fits your setup.

pt-tools is a self-hosted manager for private trackers (PT sites). It runs on your own server, NAS or computer and brings your trackers, downloaders and chat notifications into one web interface: it fetches RSS feeds on a schedule and pushes the matching torrents to a downloader, boosts, cross-seeds and transfers torrents, files finished films and shows into your media library, collects every site's statistics and login status, and can be checked and controlled from the mobile app, QQ or Telegram.

> [!NOTE]
> The web interface is in Chinese. These pages give each label in Chinese next to its English name, for example Downloader settings (下载器设置).

## How it works

pt-tools is a single program: the web interface and the background jobs run in the same process, and the data is kept in a local SQLite database. It depends on no hosted service.

- **Sites**: every site you add is accessed with your own cookie, API key or passkey. 66 sites are built in; see [Supported sites](../sites.md).
- **RSS subscriptions**: feeds are fetched at the interval you set, filtered by your rules or by freeleech status, and pushed to a downloader.
- **Downloaders**: qBittorrent and Transmission, as many as you like. Free space and the site's seeding limit are checked before every push.
- **Seeding**: brush tasks pick new torrents to seed by your conditions and delete them by rules; IYUU cross-seeding seeds the same data on several sites; torrent transfer moves torrents to another downloader.
- **Media library**: subscribe to films and series; finished downloads are recognized, placed into the library by templates, and Emby, Jellyfin or Plex is told to scan.
- **Notifications and ChatOps**: push results and system events can be sent to a chat app, and linked chat accounts can send commands or talk to the AI assistant.
- **Mobile app and APIs**: the mobile app connects back to pt-tools through remote access; scripts, MCP clients and other tools connect with API tokens or the qB-compatible entrance.

Seed boosting, cross-seeding, torrent transfer, the media library, the mobile app and the AI assistant are among the features added in the 1.0 preview and are not in the 0.48.0 stable release yet; see [What's new in the 1.0 preview](whats-new-1.0.md).

## The pages of the interface

The navigation on the left is grouped by purpose.

| Group            | Pages                                                                                                                                                                                                                                                                                                          | What they are for                                                                                                                                             |
| ---------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Overview (概览)  | Statistics (用户统计), Data export (数据导出), AI assistant (AI 助手)                                                                                                                                                                                                                                          | Upload, ratio, seeding and class progress for every site; exporting them as an image; asking about data and acting in plain language                          |
| Sites (站点)     | Site list (站点列表), Supported sites (已支持站点), CookieCloud import (CookieCloud 导入)                                                                                                                                                                                                                      | Adding and configuring sites and seeing their login status; browsing the built-in sites; importing cookies from CookieCloud                                   |
| Downloads (下载) | Torrent search (种子搜索), Task list (任务列表), Paused torrents (暂停任务), Brush tasks (刷流任务), Downloader web UI (下载器 Web UI), Downloader settings (下载器设置), Downloader assistant (下载器助手), Torrent transfer (转移做种), IYUU cross-seeding (IYUU 辅种), qB-compatible entrance (qB 兼容入口) | Searching sites and pushing torrents; the RSS push history; boosting, cross-seeding and transferring torrents; managing torrents and downloaders              |
| Media (媒体)     | Explore (探索), Subscriptions (订阅), Library (媒体库), Organising history (整理历史), Media recognition (媒体识别)                                                                                                                                                                                            | Subscribing to films and series; library and organising settings; organising results; correcting recognition                                                  |
| Rules (规则)     | Filter rules (过滤规则), Auto cleanup (自动清理)                                                                                                                                                                                                                                                               | Writing RSS filter rules; auto-delete, disk protection and cleaning the work folder                                                                           |
| ChatOps          | Notifications (消息通知), Message templates (消息模板), ChatOps binding (ChatOps 绑定), Audit log (操作审计), RSS notification log (RSS 通知日志)                                                                                                                                                              | Setting up notification channels and templates; linking chat accounts; command history and delivery results                                                   |
| System (系统)    | Global settings (全局设置), CloakBrowser, API tokens (API 令牌), MCP (MCP 接入), Remote access (远程访问), AI assistant settings (AI 助手设置), Logs (运行日志), Change password (修改密码)                                                                                                                    | Intervals, download policy and other global settings; the login check fallback; tokens, MCP and the mobile app; the AI assistant's model; logs; your password |

On a phone the same pages are shown as cards, with the most used ones in a bottom bar.

## Who it is for

- People who build ratio or follow series on several trackers and want downloads to happen by rule.
- People who want the same data seeded on several sites, or finished films and shows filed into a media library automatically.
- People who want every site's numbers on one page and a reminder before an inactive account is disabled.
- People already running qBittorrent or Transmission who want to manage several downloaders together.

## Before you start

- pt-tools reaches your sites with your own credentials. Before turning on automatic access, make sure each site's rules allow it, and keep the request intervals reasonable. pt-tools cannot guarantee that automated access will never lead to a warning or a ban.
- The `secret.key` file in the data folder encrypts your site cookies. Without it, saved cookies cannot be recovered, so back it up together with the database; see [Upgrades and backups](upgrade.md).
- There is no macOS binary; run pt-tools in Docker on macOS.

## Next

- [Install](install.md)
- [Quick start](quick-start.md)
- [Questions and troubleshooting](../faq.md)
