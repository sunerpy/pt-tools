# NAS monitoring <Badge type="tip" text="1.0 preview" />

System → NAS monitoring (系统 → NAS 监控) decides where the [NAS overview](home-and-health.md#nas-overview) on the home page gets its data: the host pt-tools runs on (the local source) and any NAS you connect. pt-tools reads these sources in the background and keeps what it reads in memory; the home page and this page show that data, so opening them does not connect to anything again.

## The local source

The local source reads `/proc` and `/sys` on the host pt-tools runs on: CPU usage, load, I/O wait and CPU temperature (when the host has a temperature sensor), memory, uptime, and the host name, CPU model and cores and kernel version.

- Linux only. On Windows and macOS its line says `本机来源只支持 Linux` (the local source supports Linux only) and the switch cannot be turned on.
- When pt-tools runs in Docker, it reads the CPU, memory and load of the Docker host.
- It cannot read network interfaces, storage pools, disks or alerts. The network card on the home page becomes the downloader speed, and storage becomes the disks of the download folders and media libraries.
- It is on by default. Turned off, it is no longer read, and the home page shows an empty card when no other source can be shown.

## The source list

One line per source (a card on a phone):

| Column  | What it shows                                                                                                                                                                         |
| ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Source  | Name, kind and address (`pt-tools 所在的主机`, the host pt-tools runs on, for the local source); experimental kinds are tagged `实验性`, `http://` addresses `明文连接` (unencrypted) |
| Host    | Host name, CPU model and cores, total memory, platform and version; `还没有读到数据` (nothing read yet) before the first read                                                         |
| State   | The state and a line explaining it, plus when data was last read when there is a problem                                                                                              |
| Actions | The switch of the local source; Reconnect (重新连接); Edit (编辑) and Delete (删除) for connected sources                                                                             |

| State                                  | Line below                                                                                                                            |
| -------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| `已连上` (connected)                   | How long ago it was updated (`40 秒前更新`)                                                                                           |
| `正在连` (connecting)                  | Shown once data arrives                                                                                                               |
| `连不上` (cannot connect)              | The error and when it will try again (`30 秒后再试`, retry in 30 seconds)                                                             |
| `登录失败` (login failed)              | The reason; it tries again only after you change the settings or click Reconnect                                                      |
| `证书不受信任` (untrusted certificate) | It disconnected without sending credentials: edit the source, test the connection and confirm the certificate                         |
| `证书换了` (certificate changed)       | The certificate differs from the trusted one and it disconnected without sending credentials: edit the source and confirm the new one |
| `已停用` (disabled)                    | The local source is off, or Enabled (启用) is not ticked for a connected source                                                       |
| `用不了` (unavailable)                 | This version or platform cannot use this kind of source                                                                               |

When a source cannot be reached it tries again after 30 seconds, then 1, 2 and 5 minutes. Login failures and certificate problems are not retried automatically, so the NAS does not lock the account; it tries again after you change the settings or click Reconnect. Reconnect tries once right away instead of waiting, and the result appears in a message.

When this version can connect some kind of NAS, Add source (添加来源) appears at the top right of the list and the notes below say which kinds; until then the list has only the local source and the notes say `这个版本还不能接入 NAS，只有本机来源。` (this version cannot connect a NAS yet, only the local source).

## What the home page shows

Choose a source under 首页的 NAS 概览显示 (shown in the NAS overview on the home page):

- **自动** (automatic, the default): the first usable source, or the local one when there is none.
- **本机** (local) or a particular source: always that one. When the chosen source is disabled or unavailable, the home page shows another one for now, and the line below the list says which.

The choice is saved right away without a restart, and the home page switches the next time it fetches data. Deleting the source the home page shows switches it back to automatic.

## How often it reads and what it keeps

- Each source is read every 30 seconds for CPU, memory and network, and every 5 minutes for storage pools, disks, alerts and apps.
- The last 30 minutes are kept in memory only (they are the bars on the home page); after pt-tools restarts it starts reading again.
- Keys and login credentials of connected sources are stored encrypted in the database and never shown on the page. When editing, leaving the key empty keeps it; after changing the address or the account you must enter it again, and the old key is never sent to the new address.
- For a source with a self-signed certificate, you check the certificate fingerprint during Test connection (测试连接) and trust it, and from then on only that certificate is accepted; when the certificate changes, the source stops and waits for you to confirm it, without sending credentials to the server with the new certificate.

The App API, MCP and the mobile app cannot read NAS data yet, and the AI assistant's configuration tools cannot change the NAS monitoring settings.
