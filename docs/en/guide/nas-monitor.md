# NAS monitoring <Badge type="tip" text="1.0 preview" />

System → NAS monitoring (系统 → NAS 监控) decides where the [NAS overview](home-and-health.md#nas-overview) on the home page gets its data: the host pt-tools runs on (the local source) and any NAS you connect (for now [TrueNAS](#truenas), [Glances](#glances) and, experimentally, [Synology](#synology) and [fnOS](#fnos)). pt-tools reads these sources in the background and keeps what it reads in memory; the home page and this page show that data, so opening them does not connect to anything again.

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

[Glances](https://github.com/nicolargo/glances) is a general-purpose system monitor. On a NAS that runs Docker, such as Synology, fnOS, Unraid or OMV, run Glances from its official image and pt-tools reads its REST interface (`http://<address>:61208/api/4/…`). It connects Glances 4 and was verified on 4.5.8; Glances 5 moved its interface to `/api/5` and is not supported yet, which Test connection says. For TrueNAS, use the [TrueNAS](#truenas) source above, which reads more; Synology and fnOS can also use the experimental [Synology](#synology) and [fnOS](#fnos) sources below, which read the NAS directly.

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

## Synology <Badge type="warning" text="experimental" />

Experimental: it connects to the Web API of Synology DSM and has been tested only against a fake service written from the interface format, not on a real device, so some data may be missing; when it cannot connect, the [source list](#the-source-list) says why. A Synology NAS can also run [Glances](#glances).

pt-tools logs in to the DSM Web API (`<address>/webapi/`) with a DSM account: it first looks up the path and version of each interface, then logs in for a session and sends that session with every later request; when the session expires or is taken over, it logs in once more. The account, password and session go in the request body, so they stay out of the access logs of DSM and any reverse proxy. The requests and responses follow the format recorded by [py-synologydsm-api](https://github.com/mib1185/py-synologydsm-api), which the Synology integration of Home Assistant uses, and by the Synology integration of [Homarr](https://github.com/homarr-labs/homarr). It reads:

- Host: model (for example DS920+), DSM version, CPU cores and uptime; DSM gives no host name here, and serial numbers are not read
- CPU usage (user, system and other added up) and load (DSM reports the load multiplied by 100, and pt-tools divides it back), and the system temperature (the one in DSM's Info Center, shown in the CPU card)
- Memory: total and used, with used taken from the percentage in DSM's Resource Monitor
- Network: the combined receive and send speed DSM adds up over its interfaces (`total`)
- Storage: each volume (`/volume1`, `/volume2`…) with its state, health and capacity, the used and total size DSM reports. A state other than normal, degraded or crashed (for example repairing) is shown as DSM writes it, with no health
- Disks: name, model, whether it is a hard disk, SSD or NVMe, size, temperature and the volume it belongs to; serial numbers are not read. When the Storage Manager data cannot be read, volumes fall back to the capacity in the system information and the disks part says why
- Alerts and apps: DSM's notifications and packages (including Container Manager) are not read yet, and these two blocks on the home page say why

For a Synology source, the NAS group of the [system check](home-and-health.md#system-check) looks at whether it is connected, whether the volumes are healthy and the disk temperatures; pt-tools does not read DSM's notifications, so the alerts part is not checked.

### Preparing an account

1. In DSM, go to Control Panel → User & Group and click Create to create a user only for pt-tools, with a strong password.
2. Under User Groups, tick `administrators`. DSM lets only accounts in the administrators group read the system information and the Resource Monitor (the [Homepage documentation for its Synology widget](https://gethomepage.dev/widgets/services/diskstation/) asks for the same); with an ordinary account, Test connection says `这个账号读不到系统信息：DSM 只让管理员组（administrators）的账号读…` (this account cannot read the system information: DSM allows only the administrators group).
3. Under Permissions, tick No Access at the top so the account cannot open any shared folder; under Applications, tick Deny in the header, then tick Allow for the DSM row only, or By IP to allow only the IP address of the host pt-tools runs on.
4. In pt-tools, under Add source (添加来源), choose 群晖 (Synology), enter the address of the DSM web UI and the account and password, click Test connection (测试连接) and save when it connects. DSM listens on port 5001 for `https://` and 5000 for `http://` by default: an `http://` address without a port uses 5000; an `https://` address without a port uses 443 (the address of a reverse proxy), so add `:5001` to connect to DSM directly. DSM uses a self-signed certificate by default; trust it once the fingerprint matches.

- With an `http://` address, the account and password cross the local network unencrypted, and you must tick 我知道账号密码会以明文经过局域网，可能被截获 (I know the account and password cross the local network unencrypted and may be intercepted). When DSM redirects HTTP to HTTPS (a setting under Control Panel → Login Portal), Test connection says `…DSM 开了 HTTPS 跳转，地址改成 https://（默认端口 5001）` (DSM redirects to HTTPS; change the address to https://, port 5001 by default).
- pt-tools only reads this data and changes no DSM settings.
- When login fails (wrong user name or password, a disabled account, no permission to sign in to DSM, an expired password and so on), the source shows `登录失败` (login failed) with the reason DSM gave, and it is not retried automatically: DSM's auto block bans an IP address after failed logins. When it has been banned, the reason is `登录失败太多次，DSM 封锁了 pt-tools 所在的 IP…` (too many failed logins; DSM blocked the IP of pt-tools), and you lift the ban under Control Panel → Security → Protection in DSM.

### Two-factor authentication

When the account uses two-factor authentication, the first Test connection asks for a code: enter the 6-digit code from the authenticator app and test again. Once connected, DSM gives pt-tools a device token (it signs in with the device name `pt-tools`), which is stored encrypted when you save; later logins send it and need no code.

- A wrong or expired code says `验证码不对或过期了，重新输入` (the code is wrong or expired; enter it again); enter a new one.
- When pt-tools is removed from the trusted devices on DSM (or two-factor authentication was turned on for the account after you saved the source), the source shows `登录失败` (login failed) with a reason saying DSM wants a code, and it stops signing in: edit the source, enter a code during Test connection, and save.
- After changing the address or the account, the old device token is no longer used and you enter a code again.

## fnOS <Badge type="warning" text="experimental" />

Experimental: it connects through the WebSocket interface the fnOS web UI itself uses. fnOS has not published documentation for this interface, so this is an unofficial protocol: pt-tools has been verified on a real device running fnOS 1.2.1000, and an fnOS upgrade may stop it from reading; when it cannot connect, the [source list](#the-source-list) says why. fnOS can also run [Glances](#glances).

pt-tools connects to `<address>/websocket?type=main` on the fnOS web UI (`wss://` for an `https://` address, `ws://` for an `http://` address). The requests and responses follow the format recorded by [pyfnos](https://github.com/Timandes/pyfnos); none of its code is used:

- It first asks fnOS for the RSA public key used to log in. The account and password are encrypted with a one-time AES key, and that key is encrypted with the fnOS public key before it is sent, so only fnOS can decrypt them.
- After login, fnOS gives a token, a long-lived token and a signing key, and every later request is signed. These three are stored encrypted when you save, and later connections use them instead of the password: when the token expires, the long-lived token gets a new one, and only when neither works (for example after the device was removed on fnOS) does pt-tools log in with the password again.
- The connection stays open between reads, with a heartbeat every 30 seconds; when it drops, the next read connects again.

It reads:

- Host: host name, fnOS version, CPU model and threads, uptime
- CPU usage, I/O wait and load, and the CPU temperature (the highest of the temperatures fnOS gives; usually unavailable in a virtual machine)
- Memory: total and used. Used is the figure fnOS gives, the same as 已使用 (used) in the fnOS web UI (it leaves out the small part the system reserves); when fnOS gives no used figure, total minus available
- Network: the combined receive and send speed fnOS adds up, and the interface names (without `lo`)
- Storage: each storage space (`/vol1`, `/vol2`…) with its capacity and state. The state follows the software RAID arrays under it: degraded, rebuilding, crashed and not running count as unhealthy; syncing and reshaping are shown as they are, with no health
- Disks: name, model, whether it is a hard disk, SSD or NVMe, size, temperature and the storage space it belongs to; hot spares are not listed and serial numbers are not read; when fnOS reports a temperature of 0 (some NVMe drives do), no temperature is shown
- Apps: how many Docker containers are running and how many there are, 0 when Docker is not started or has no containers; which can be updated is not known
- Alerts: fnOS notifications are not read yet, and the alerts block on the home page says why; a degraded storage space shows in the storage block

For an fnOS source, the NAS group of the [system check](home-and-health.md#system-check) looks at whether it is connected, whether the storage spaces are healthy and the disk temperatures; pt-tools does not read fnOS notifications, so the alerts part is not checked.

### Preparing an fnOS account

1. In the user settings of the fnOS web UI, create a user only for pt-tools, with a strong password.
2. Give it administrator rights: fnOS lets only administrators read storage spaces, disks and Docker containers (in the interface notes of [pyfnos](https://github.com/Timandes/pyfnos), the storage interfaces all say they need an administrator and return error code 4352 for an ordinary account; on fnOS 1.2.1000 an ordinary account reading Docker gets error code 52429312, and the fnOS web UI itself does not show Docker to ordinary accounts). With an ordinary account, CPU, memory and network show as usual, and the storage, disks and containers blocks say `这个账号没有读…的权限：飞牛只让管理员读…` (this account may not read …: fnOS allows only administrators); when fnOS does not even let it read the host information, Test connection says `这个账号读不到飞牛的主机信息：要用管理员账号…` (this account cannot read the fnOS host information: use an administrator account), and the source cannot be saved.
3. In pt-tools, under Add source (添加来源), choose 飞牛 (fnOS), enter the address of the fnOS web UI and the account and password, click Test connection (测试连接) and save when it connects. fnOS listens on port 5667 for `https://` and 5666 for `http://` by default: an `http://` address without a port uses 5666; an `https://` address without a port uses 443 (the address of a reverse proxy), so add `:5667` to connect to fnOS directly. fnOS usually uses a self-signed certificate for https; trust it once the fingerprint matches.

- With an `http://` address, pt-tools connects over unencrypted `ws://`. The account and password are encrypted, but a man in the middle could replace the public key fnOS sends with one of its own and decrypt them, so such an address always needs 我知道账号密码会以明文经过局域网，可能被截获 (I know the account and password cross the local network unencrypted and may be intercepted) ticked, even for a draft without a password. When fnOS forces HTTPS, Test connection says `…飞牛开了「强制 HTTPS」，地址改成 https://（默认端口 5667）` (fnOS forces HTTPS; change the address to https://, port 5667 by default).
- When the address points at some other web page (a wrong port, or a reverse proxy that does not forward WebSocket), it says `这个地址没有飞牛的接口（/websocket 回了 HTTP 404）…` (this address has no fnOS interface) or `回的不是飞牛 fnOS 的数据…` (the reply is not fnOS data), and the account and password are not sent.
- pt-tools only reads this data and changes no fnOS settings; it does not log out when it disconnects, so the stored token stays usable.
- When login fails (a wrong user name or password and so on), the source shows `登录失败` (login failed) with the reason fnOS gave (`飞牛说：…（错误码 …）`, fnOS says … (error code …), with any password in it replaced by `***`), and it is not retried automatically: signing in again and again with a wrong password may get the account locked on fnOS.

### Two-factor authentication on fnOS

When the account uses two-factor authentication, the first Test connection asks for a code: enter the 6-digit code from the authenticator app and test again. During verification pt-tools asks fnOS to trust this device (the device name is `pt-tools`), and the device ID is stored with the login credentials; later connections use the token and need no code, and when it has to log in with the password again, fnOS recognises the trusted device and needs no code either.

- A wrong or expired code says `验证码不对或过期了，重新输入` (the code is wrong or expired; enter it again); enter a new one.
- When fnOS requires two-factor authentication for the account but no authenticator is bound yet, it says `飞牛要求这个账号开两步验证，它还没有绑定验证器…` (fnOS requires two-factor authentication for this account, which has no authenticator bound): log in to the fnOS web UI with the account once, bind one as prompted, and test again.
- When both the token and the trusted device no longer work (or two-factor authentication was turned on for the account after you saved the source), the source shows `登录失败` (login failed) with a reason starting `飞牛要验证码…` (fnOS wants a code), and it stops signing in: edit the source, enter a code during Test connection, and save.
- After changing the address or the account, the old token and device ID are no longer used and you enter a code again.

## The source list

One line per source (a card on a phone):

| Column  | What it shows                                                                                                                                                                                                                                                      |
| ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Source  | Name, kind and address (`pt-tools 所在的主机`, the host pt-tools runs on, for the local source); experimental kinds are tagged `实验性` (for one verified on a real device, hovering over the tag shows the version), `http://` addresses `明文连接` (unencrypted) |
| Host    | Host name, CPU model and cores, total memory, platform and version; `还没有读到数据` (nothing read yet) before the first read; the reason when the source is connected but cannot read host information (a Synology or fnOS account that is not an administrator)  |
| State   | The state and a line explaining it, plus when data was last read when there is a problem                                                                                                                                                                           |
| Actions | The switch of the local source; Reconnect (重新连接); Edit (编辑) and Delete (删除) for connected sources                                                                                                                                                          |

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

Add source (添加来源) at the top right of the list shows the fields each kind needs, for TrueNAS how to prepare a read-only API key (see [TrueNAS](#truenas)), for Glances how to run it on the NAS (see [Glances](#glances)), and for Synology and fnOS how to create an account for pt-tools (see [Synology](#synology) and [fnOS](#fnos)); experimental kinds carry `（实验性）` (experimental) in the list of kinds, and once chosen the dialog says whether the kind has been verified on a real device (with the version if it has); the notes below the list say which kinds can be connected and which have been verified on a real device.

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
