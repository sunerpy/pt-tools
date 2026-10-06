# 下载器助手

「下载 → 下载器助手」页面整理下载器里已有的种子，提供三项操作：找出站点已删除的失效种子、给缺站点标签的种子补标签、批量替换 tracker 地址。另有一个只发通知的失效种子定时扫描。

每项都先扫描或预览，勾选要处理的种子后才执行；执行时 pt-tools 会逐个重新核对，预览之后种子有了变化的会跳过，结果里会写明跳过和失败的原因。页面上方选择要处理的下载器，只列出已启用的下载器。

tracker 地址里带着 passkey。页面上显示的地址已经把 passkey、`credential` 一类参数和路径里的长串密钥遮住，替换时用的是下载器里的完整地址。

## 失效种子

站点删除了种子或者换了种之后，下载器里的这个种子再也连不上 tracker，tracker 会回复「未注册」或「种子不存在」。点「扫描」后，pt-tools 逐个读取种子的 tracker 状态，满足以下两条的列为失效种子：

- 至少一个 tracker 回复未注册（unregistered、torrent not registered、未注册）或不存在（torrent not found、种子不存在、种子已被删除等）；
- 没有任何 tracker 在正常工作。

回复里提到 passkey、用户、账号、分享率或封禁的不算失效：那是账号的问题，种子本身还在，删掉它会丢掉做种记录。

勾选后点「删除选中 N 个」。默认只从下载器里删除种子、保留数据文件；勾上「同时删除数据文件」时连文件一起删，删除后无法恢复。删除前会逐个重新读取 tracker 状态，已经恢复正常的种子不删。

扫描要为每个种子请求一次下载器，种子多时需要几十秒。

## 补站点标签

pt-tools 按 tracker 地址认出种子属于哪个站点（见下文[按 tracker 识别站点](#按-tracker-识别站点)）。分类和标签里都没有这个站点名的种子会列出来，勾选后点「给选中的 N 个打标签」，给它们加上站点标签（站点 ID，如 `hdsky`，页面「加上的标签」一列就是它），原有标签保留。

打上站点标签后，站点做种容量、自动删种的清理范围和下载器 Web UI 的站点筛选都能认出这些种子。

## 替换 tracker

站点换了 tracker 域名，或者重置了 passkey 之后，下载器里旧种子的 tracker 地址需要改成新的。填写「原内容」和「替换成」后点「预览」，列出 tracker 地址里含有原内容的种子，以及替换后的地址。

- 原内容至少 3 个字，替换的是地址里第一次出现的原内容。
- 替换后的地址必须仍是 http、https 或 udp 地址，否则这一项报错、不修改。
- 改了原内容或替换成之后，要重新预览才能执行，防止按没有预览过的内容替换。

qBittorrent 和 Transmission 都支持修改 tracker：Transmission 4.0 起整份更新 tracker 列表（分层保持不变），更早的版本逐个替换。下载器不支持时页面会提示只能预览。

## 定时扫描

在「定时扫描」里开启后，pt-tools 按设定的间隔（6–168 小时，默认 24 小时）扫描所有已启用的下载器，发现新的失效种子时发一条通知到选定的通道。开启时至少要选一个通知通道。

- 只发通知，不会自动删除任何种子。
- 同一批失效种子只通知一次；出现新的失效种子时再发一条，列出全部。失效种子只是变少时不再通知。
- 通道处在静默时段时，通知顺延到静默结束再发；发送失败会自动重试。

## 按 tracker 识别站点

pt-tools 先按主机名比对 tracker 地址和内置站点的地址，再比对可注册域：`tracker.hdsky.me` 与站点地址 `hdsky.me` 同属 `hdsky.me`，所以认作 HDSky。多个站点共用一个可注册域时，只认主机名完全相同的。自定义站点不参与识别。

站点做种容量（站点详情里的「刷流容量上限」）也用这条规则：没有 pt-tools 推送记录、也没打站点分类或标签的种子，主 tracker 属于这个站点时同样计入它的做种总量。手动添加了很多种子的站点，做种总量会比以前大。

## 接口

| 方法与路径                                                        | 说明                                                      |
| ----------------------------------------------------------------- | --------------------------------------------------------- |
| `GET /api/downloader-assistant/dead?downloader_id=`               | 扫描失效种子                                              |
| `POST /api/downloader-assistant/dead`                             | 删除 `{downloader_id, hashes, remove_data}`               |
| `GET /api/downloader-assistant/site-tags?downloader_id=`          | 列出缺站点标签的种子                                      |
| `POST /api/downloader-assistant/site-tags`                        | 补标签 `{downloader_id, items: [{hash, site}]}`           |
| `GET /api/downloader-assistant/trackers?downloader_id=&from=&to=` | 预览替换；`supported` 为 false 表示下载器不支持修改       |
| `POST /api/downloader-assistant/trackers`                         | 执行替换 `{downloader_id, from, to, hashes}`              |
| `GET`、`PUT /api/downloader-assistant/dead-scan`                  | 读写定时扫描设置 `{enabled, interval_hours, channel_ids}` |

都要求登录，一次最多处理 2000 个种子。执行接口返回 `{done, skipped, failed}`，后两项逐个写明种子和原因。
