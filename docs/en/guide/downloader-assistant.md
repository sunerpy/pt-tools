# Downloader assistant

Downloader assistant (下载器助手), under Download (下载), tidies up the torrents already in a downloader. It finds dead torrents that a site has deleted, adds site tags to torrents that lack one, and replaces tracker addresses in bulk. A scheduled dead-torrent scan sends notifications only.

Each action starts with a scan or a preview. Nothing happens until you tick the torrents to handle and confirm. pt-tools checks every torrent again when it runs the action and skips the ones that changed after the preview; the result lists what was skipped or failed and why. Pick the downloader at the top of the page; only enabled downloaders are listed.

Tracker addresses carry your passkey. The page masks the passkey, parameters such as `credential` and long key-like path segments; replacements use the full address stored in the downloader.

## Dead torrents

When a site deletes or replaces a torrent, the downloader can no longer announce it, and the tracker answers "unregistered" or "torrent not found". After you click Scan (扫描), pt-tools reads the tracker status of every torrent and lists a torrent as dead when both hold:

- at least one tracker reports it as unregistered (unregistered, torrent not registered, 未注册) or missing (torrent not found, 种子不存在, 种子已被删除 and similar);
- no tracker is working for it.

Replies that mention the passkey, the user, the account, the ratio or a ban do not count. They point to an account problem: the torrent still exists, and deleting it would lose its seeding history.

Tick the torrents and click Delete selected (删除选中). By default the torrent is removed from the downloader and its data files stay on disk; tick Also delete data files (同时删除数据文件) to remove the files too, which cannot be undone. pt-tools reads the tracker status of each torrent again before deleting it and keeps the ones that work again.

A scan asks the downloader once per torrent, so it can take tens of seconds when there are many torrents.

## Site tags

pt-tools recognises the site of a torrent from its tracker address (see [Recognising sites by tracker](#recognising-sites-by-tracker)). Torrents whose category and tags do not contain that site are listed; tick them and click Tag the selected (给选中的…打标签) to add the site name as a tag. Existing tags are kept.

With the site tag in place, the site seeding capacity, the scope of automatic cleanup and the site filter in the downloader web UI all recognise these torrents.

## Replacing trackers

After a site moves its tracker to a new domain or you reset your passkey, the tracker address of the old torrents needs to change. Fill in the text to replace and the replacement, then click Preview (预览): the page lists the torrents whose tracker address contains the text, with the address after the change.

- The text to replace needs at least 3 characters; the first occurrence in the address is replaced.
- The new address must still be an http, https or udp address. Otherwise that item reports an error and is left unchanged.
- After you edit either field, preview again before running the replacement, so it never uses text you have not previewed.

Both qBittorrent and Transmission can edit trackers. Transmission 4.0 and later update the whole tracker list (tiers stay as they are); older versions replace each address. When a downloader cannot edit trackers, the page says the preview is all it can do.

## Scheduled scan

Turn the scan on under the Scheduled scan (定时扫描) tab and pt-tools scans every enabled downloader at the interval you set (6 to 168 hours, 24 by default). When it finds new dead torrents it sends one notification to the channels you choose; pick at least one channel to turn it on.

- It only sends notifications and never deletes a torrent.
- The same set of dead torrents is reported once. A new dead torrent triggers another notification listing all of them; a shrinking list does not.
- A channel in its quiet hours gets the notification when the quiet hours end, and failed deliveries are retried.

## Recognising sites by tracker

pt-tools first compares the tracker host with the addresses of the built-in sites, then the registrable domain: `tracker.hdsky.me` and the site address `hdsky.me` share `hdsky.me`, so the torrent belongs to HDSky. When several sites share a registrable domain, only an exact host match counts. Custom sites are not recognised.

The site seeding capacity (刷流容量上限 in the site details) follows the same rule: a torrent with no pt-tools push record and no site category or tag still counts towards a site when its main tracker belongs to that site. Sites where you added many torrents by hand will show a larger seeding total than before.

## API

| Method and path                                                   | Purpose                                                                              |
| ----------------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| `GET /api/downloader-assistant/dead?downloader_id=`               | Scan for dead torrents                                                               |
| `POST /api/downloader-assistant/dead`                             | Delete `{downloader_id, hashes, remove_data}`                                        |
| `GET /api/downloader-assistant/site-tags?downloader_id=`          | List torrents without a site tag                                                     |
| `POST /api/downloader-assistant/site-tags`                        | Add tags `{downloader_id, items: [{hash, site}]}`                                    |
| `GET /api/downloader-assistant/trackers?downloader_id=&from=&to=` | Preview a replacement; `supported` is false when the downloader cannot edit trackers |
| `POST /api/downloader-assistant/trackers`                         | Replace `{downloader_id, from, to, hashes}`                                          |
| `GET`, `PUT /api/downloader-assistant/dead-scan`                  | Read or save the scheduled scan `{enabled, interval_hours, channel_ids}`             |

All of them require a signed-in session and handle at most 2000 torrents per request. The action endpoints return `{done, skipped, failed}`; the last two list each torrent with the reason.
