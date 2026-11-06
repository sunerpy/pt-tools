# NAS monitoring <Badge type="tip" text="1.0 preview" />

System → NAS monitoring (系统 → NAS 监控) decides where the [NAS overview](home-and-health.md#nas-overview) on the home page gets its data: the host pt-tools runs on (the local source) and any NAS you connect (for now [TrueNAS](#truenas) and [Glances](#glances)). pt-tools reads these sources in the background and keeps what it reads in memory; the home page and this page show that data, so opening them does not connect to anything again.

## The local source

The local source reads `/proc` and `/sys` on the host pt-tools runs on: CPU usage, load, I/O wait and CPU temperature (when the host has a temperature sensor), memory, uptime, and the host name, CPU model and cores and kernel version.

- Linux only. On Windows and macOS its line says `本机来源只支持 Linux` (the local source supports Linux only) and the switch cannot be turned on.
- When pt-tools runs in Docker, it reads the CPU, memory and load of the Docker host.
- It cannot read network interfaces, storage pools, disks or alerts. The network card on the home page becomes the downloader speed, and storage becomes the disks of the download folders and media libraries.
- It is on by default. Turned off, it is no longer read, and the home page shows an empty card when no other source can be shown.

## TrueNAS

Connects TrueNAS 25.04 or later. pt-tools uses a read-only API key on its JSON-RPC interface (`wss://<address>/api/current`) over one connection that stays open, and logs in once per connection. It reads:

- Host: host name, version, CPU model and cores, total memory, uptime and load
- CPU usage and temperature, and memory; a virtual machine usually has no CPU temperature, and the CPU card on the home page then says `读不到温度` (no temperature)
- Network: the combined receive and send speed of the physical interfaces that are up
- Storage pools: state, health and capacity. Capacity is the used and available space of the pool's root dataset, the space you can actually store after parity; a pool without a readable root dataset falls back to its raw size, parity included
- Disks: model, whether it is a hard disk, SSD or NVMe, size, temperature and the pool it belongs to; serial numbers are not read
- Alerts: the alerts not dismissed on TrueNAS, with the common kinds written in Chinese and the rest in TrueNAS's own wording
- Apps: how many are running, how many there are and how many can be updated

On the first connection it fills the home page bars with the last 30 minutes of CPU, memory and network that TrueNAS recorded.

### Preparing a read-only API key

1. On TrueNAS, go to Credentials → Users and click Add to create a user only for pt-tools: turn on Disable Password, choose `truenas_readonly_administrators` (read-only administrators) under Auxiliary Groups, leave SMB User unticked and give it no sudo.
2. In the user menu at the top right of TrueNAS, click My API Keys → Add, choose this user under Username, save and copy the key. TrueNAS shows it only once; if you missed it, edit the key and click Reset for a new one.
3. In pt-tools, under Add source (添加来源), choose TrueNAS, enter the https address of the TrueNAS web UI (for example `https://192.168.1.10`, with the port if you changed it), paste the key and click Test connection (测试连接). TrueNAS uses a self-signed certificate by default: once the SHA-256 fingerprint matches the one on TrueNAS, click 信任这张证书并再测一次 (trust this certificate and test again), and save when it connects.

- Only `https://` addresses are accepted. TrueNAS revokes an API key that was used over http, and pt-tools never sends the key over http.
- A read-only administrator can read this data but cannot change settings: pt-tools changes nothing on TrueNAS and does not dismiss its alerts.
- When the key is deleted or revoked on TrueNAS, the source shows `登录失败` (login failed). Create a new key and edit the source to enter it.
- On a version older than 25.04, Test connection says `这台 TrueNAS 没有 /api/current 接口（HTTP 404）：要 TrueNAS 25.04 或以后的版本` (this TrueNAS has no /api/current interface; it needs TrueNAS 25.04 or later).

## Glances

[Glances](https://github.com/nicolargo/glances) is a general-purpose system monitor. On a NAS that runs Docker, such as Synology, fnOS, Unraid or OMV, run Glances from its official image and pt-tools reads its REST interface (`http://<address>:61208/api/4/…`). It connects Glances 4 and was verified on 4.5.8; Glances 5 moved its interface to `/api/5` and is not supported yet, which Test connection says. For TrueNAS, use the [TrueNAS](#truenas) source above, which reads more.

It reads:

- Host: host name, system and version, CPU model and logical cores, uptime and load
- CPU usage, I/O wait and temperature (when the host has a temperature sensor), and memory
- Network: the combined receive and send speed of the physical interfaces that are up (names such as `eth…`, `en…` or `wl…`); bridges, bonds, VLANs and Docker's virtual interfaces are left out, so the same traffic is not counted twice. When no interface has such a name (for example `ixl0` on FreeBSD), every interface except the virtual ones counts
- Storage: the file systems of the folders mounted into the Glances container, by used and total size; a disk mounted in several places counts once, and ZFS is added up per pool. Glances gives no health, so the home page shows no health for these pools
- Disks: temperature only. NVMe disks are named `NVMe 1`, `NVMe 2`; SATA disks need the `drivetemp` module loaded on the NAS. Glances gives no model or size
- Alerts: Glances gives no alerts of the NAS itself. With Glances's raid plugin turned on, a software RAID (mdadm) array that is not running or is degraded becomes a critical alert (see [Software RAID](#software-raid)); with it off, the alerts block on the home page says why
- Apps: how many Docker containers are running and how many there are; which can be updated is unknown

pt-tools does not read Glances's history: the home page bars start when it connects, instead of being filled with the last 30 minutes as for TrueNAS.

For a Glances source, the NAS group of the [system check](home-and-health.md#system-check) looks at whether it is connected, the disk temperatures and the software RAID arrays; Glances gives no storage pool health, so that part is not checked.

### Running Glances on the NAS

Create a container from the official image; in compose:

```yaml
services:
  glances:
    image: nicolargo/glances:4.5.8-full
    restart: unless-stopped
    network_mode: host # sees the NAS's interfaces, and the host name is the NAS's
    pid: host
    environment:
      - GLANCES_OPT=-w
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro # reads the containers
      - /etc/os-release:/etc/os-release:ro # shows the NAS's own system name
      - /volume1:/volume1:ro # the storage folders to watch, mounted read-only
```

- Use a 4.x version tag, not `latest`: `latest` will become Glances 5 later.
- Use the host network. On the default bridge network Glances sees only its own container's interface; pt-tools recognises this and the network line says `Glances 的容器没用 host 网络…` (the Glances container does not use the host network) instead of showing the container's own traffic as the NAS's.
- On Synology, create a project in Container Manager and paste the above; the storage folders are `/volume1`, `/volume2`. On fnOS, Unraid and OMV, use their own Docker pages and your own storage folders.
- In pt-tools, under Add source (添加来源), choose Glances, enter `http://<NAS address>:61208` (an `http://` address without a port uses 61208), click Test connection (测试连接), and save when it connects.

### Authentication

Glances has no authentication by default, so anyone on the local network can read this data. To turn it on (the way the Glances documentation describes):

1. Create the password file inside the container: `docker exec -it glances glances -s --password`, enter the password twice, answer `Yes` when asked whether to save it, then press Ctrl+C. The user name is `glances` by default.
2. Copy the password file out: `docker cp glances:/root/.config/glances/glances.pwd ./glances.pwd`.
3. In compose, mount `./glances.pwd:/root/.config/glances/glances.pwd:ro`, change `GLANCES_OPT` to `-w --password`, and recreate the container.
4. In pt-tools, enter the user name (`glances`, also used when left empty) and the password. With an `http://` address you must tick 我知道账号密码会以明文经过局域网，可能被截获 (I know the user name and password cross the local network unencrypted and may be intercepted); to avoid sending the password unencrypted, put a reverse proxy with https in front of Glances and use an `https://` address.

With a wrong user name or password the source shows `登录失败` (login failed) and is not retried automatically.

### Software RAID

Glances's raid plugin is off by default. To watch software RAID (mdadm) arrays:

1. Copy the configuration file out of the image: `docker cp glances:/etc/glances/glances.conf ./glances.conf`.
2. In its `[raid]` section, change `disable=True` to `disable=False`.
3. In compose, mount `./glances.conf:/etc/glances/glances.conf:ro` and recreate the container.

An array that is not running, or that has fewer disks in use than it should (degraded), becomes a critical alert on the home page and in the system check; RAID 0 has no redundancy and is never degraded. With the plugin on but no arrays, the alerts block says `Glances 没有发现软 RAID（mdadm）阵列…` (Glances found no software RAID arrays). RAID built into ZFS or Btrfs is not covered here.

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

Add source (添加来源) at the top right of the list shows the fields each kind needs, for TrueNAS how to prepare a read-only API key (see [TrueNAS](#truenas)), and for Glances how to run it on the NAS (see [Glances](#glances)); the notes below the list say which kinds can be connected.

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

The NAS group of the [system check](home-and-health.md#system-check) looks at the connected sources: whether they are connected, whether the storage pools are healthy, whether there are critical alerts and whether the disks run hot. It uses only the data already read here and does not connect to the NAS again.

The App API, MCP and the mobile app cannot read NAS data yet, and the AI assistant's configuration tools cannot change the NAS monitoring settings.
