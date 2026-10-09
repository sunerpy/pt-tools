# Remote access

Remote access lets the pt-tools mobile app show your torrents, site statistics and subscriptions while you are away from home, and, with the permissions you grant, pause, delete and push torrents or manage subscriptions. You pair a phone by scanning a QR code in the web interface; from then on the app reaches this pt-tools through a direct address or through a relay. The connection is end-to-end encrypted: a relay only forwards it and cannot read it.

Remote access is off by default. It is separate from [API tokens](api-tokens.md): tokens are for scripts and MCP clients, and you keep their plain text yourself; a paired device connects with keys created during pairing, needs no token, and does not require you to open the pt-tools port to the internet.

## How it works

The app has two ways back to pt-tools. Pairing puts both into the QR code; the app tries the direct address first and falls back to a relay:

| Way    | When to use it                                                                                                                                  |
| ------ | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| Direct | The phone and pt-tools are on the same local network, or pt-tools is reachable from outside (behind a reverse proxy)                            |
| Relay  | The phone is away and pt-tools sits on a home network: pt-tools connects out to the relay, so does the app, and the relay forwards between them |

- Both ways carry the same encrypted session (the Noise protocol), so the traffic is encrypted even over plain HTTP on a local network.
- A device can call only the [App API](../reference/app-api.md): settings, sites, downloaders, notification channels, token management and the rest of the web interface do not exist inside the encrypted tunnel.
- The protocol is specified in [the remote access protocol](https://github.com/sunerpy/pt-tools/blob/main/docs/design/remote-access.md) (Chinese).

## Turn it on

Under System → Remote access (系统 → 远程访问):

1. Turn on Remote access (远程访问).
2. Under Direct address (直连地址), enter the address the app uses to reach this pt-tools, for example `http://192.168.1.10:8080`. Behind a reverse proxy, enter the proxy's address, with a sub-path if there is one, for example `https://example.com/pt-tools`. Leave it empty and the app connects only through a relay.
3. Under Relay addresses (relay 地址), enter one `wss://` address per line, up to 4. Builds that include a hosted relay show Use the hosted relay (填入托管 relay). If you only use the app at home, you can leave this empty.
4. Save.

The first time you turn it on, pt-tools creates this host's keys and stores them in the database, encrypted with the AES key in `secret.key`. The Status panel on the right shows the hostId, the number of open connections and the state of each relay; when a relay is disconnected its line says why, and pt-tools keeps reconnecting with backoff.

Behind a reverse proxy, the proxy has to forward WebSocket connections (the `Upgrade` and `Connection` request headers). The direct entrance is at `/remote/v1/stream`.

## Add a device

1. Under Devices (设备), click Add device (添加设备).
2. Choose the permission: Full control (完全控制) can view everything and also pause, delete and push torrents, sign in to sites and manage subscriptions; Read only (只读) can only view.
3. Direct address for this pairing (这次的直连地址) defaults to the direct address in the settings, or, if that is empty, to the address in your browser's address bar. It goes only into this QR code.
4. Click Create QR code (生成二维码) and scan it with the app.

- The QR code works once and for 10 minutes, and stops working when you close the window. Only one QR code is valid at a time; creating a new one cancels the old one.
- Five failed pairing requests (a wrong pairing secret, or a malformed request from the app) also cancel the QR code.
- A QR code is a temporary key: whoever has it can pair a device within 10 minutes. Do not share it or post a screenshot of it in a chat.
- When pairing succeeds, the window shows the device's name, the device appears under Devices, and every enabled notification channel receives a New device paired (新设备已配对) message.

## Manage devices

The device list shows each device's permission, whether it is online (directly or through a relay) and when it last connected.

- Rename (改名): changes only the name shown; the device stays connected.
- Change the permission, Make read only (改成只读) or Give full control (改成完全控制): the device's current connections close at once, and when the app reconnects it has the new permission.
- Revoke (撤销): the device disconnects at once and can never connect again; pair it again to use it. A revoked device stays in the list until you delete its record (删除记录).

## Relays

A relay only forwards traffic. When pt-tools connects to a relay it proves its hostId with a signature made with the host key, so no other machine can pose as this host, and the relay cannot decrypt anything between the app and pt-tools.

What a relay can see: this host's hostId, the IP addresses of the app and of pt-tools, when they connect and how much traffic they exchange. If that matters to you, use only the direct address, or run your own relay.

## Rotate the host keys

Rotate host keys (轮换主机密钥), under Status, replaces the host keys and revokes every device: they remember the old host key and can no longer connect, so they need to be paired again. Use it only if you suspect the keys have leaked, for example when a backup of the database and `secret.key` got out.

## Audit and security

- A device's write actions (pausing, deleting and pushing torrents, signing in to sites, managing subscriptions) are recorded under ChatOps → Audit log (操作审计) with the channel Remote device (远程设备) and the device number as the user; actions refused for lack of permission are recorded too. Pairing results are recorded as well, with the command `remote:pair`.
- Turning remote access off disconnects every device at once, the direct entrance responds with 404 and the relays are disconnected. Devices and host keys are kept, so they work again when you turn it back on.
- The direct entrance accepts at most 30 handshakes per IP address per minute. Behind a reverse proxy pt-tools sees only the proxy's address, so all devices share that limit.
- Remote access settings and device management accept only a signed-in browser session; API tokens cannot change them.

## Common questions

**The app says the device is not paired or was revoked**: the device was revoked, or the host keys were rotated. Create a new QR code in the web interface and pair again.

**The direct address does not work**: check that the phone can open the direct address; behind a reverse proxy, check that the proxy forwards WebSocket connections. When the direct address fails, the app tries the relays.

**A relay stays disconnected**: read the reason on its line. Relay authentication failed (relay 认证没有通过) means the address the relay checks the signature against differs from the address you entered (for example because traffic passes through another forwarder); use the relay's official public address.
