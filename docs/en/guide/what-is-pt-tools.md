# What is pt-tools

This page explains what pt-tools is for, how it works and what each page of its web interface does, so you can decide whether it fits your setup.

pt-tools is a self-hosted manager for private trackers (PT sites). It runs on your own server, NAS or computer and brings your trackers, downloaders and chat notifications into one web interface: it fetches RSS feeds on a schedule and pushes the matching torrents to a downloader, collects every site's statistics and login status, and can be checked and controlled from QQ or Telegram.

> [!NOTE]
> The web interface is in Chinese. These pages give each label in Chinese next to its English name, for example Downloader settings (下载器设置).

## How it works

pt-tools is a single program: the web interface and the background jobs run in the same process, and the data is kept in a local SQLite database. It depends on no hosted service.

- **Sites**: every site you add is accessed with your own cookie, API key or passkey. 66 sites are built in; see [Supported sites](../sites.md).
- **RSS subscriptions**: feeds are fetched at the interval you set, filtered by your rules or by freeleech status, and pushed to a downloader.
- **Downloaders**: qBittorrent and Transmission, as many as you like. Free space and the site's seeding limit are checked before every push.
- **Notifications and ChatOps**: push results and system events can be sent to a chat app, and linked chat accounts can send commands.

## The pages of the interface

The navigation on the left is grouped by purpose.

| Group            | Pages                                                                                                                                            | What they are for                                                                                                |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------- |
| Overview (概览)  | Statistics (用户统计), Data export (数据导出)                                                                                                    | Upload, ratio, seeding and class progress for every site; exporting them as an image                             |
| Sites (站点)     | Site list (站点列表), Supported sites (已支持站点)                                                                                               | Adding and configuring sites and seeing their login status; browsing the built-in sites                          |
| Downloads (下载) | Torrent search (种子搜索), Task list (任务列表), Paused torrents (暂停任务), Downloader web UI (下载器 Web UI), Downloader settings (下载器设置) | Searching sites and pushing torrents; the RSS push history; managing torrents and downloaders                    |
| Rules (规则)     | Filter rules (过滤规则), Auto cleanup (自动清理)                                                                                                 | Writing RSS filter rules; auto-delete, disk protection and cleaning the work folder                              |
| ChatOps          | Notifications (消息通知), ChatOps binding (ChatOps 绑定), Audit log (操作审计), RSS notification log (RSS 通知日志)                              | Setting up notification channels; linking chat accounts; command history and delivery results                    |
| System (系统)    | Global settings (全局设置), CloakBrowser, Logs (运行日志), Change password (修改密码)                                                            | Intervals, download policy and other global settings; an optional fallback for login checks; logs; your password |

On a phone the same pages are shown as cards, with the most used ones in a bottom bar.

## Who it is for

- People who build ratio or follow series on several trackers and want downloads to happen by rule.
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
