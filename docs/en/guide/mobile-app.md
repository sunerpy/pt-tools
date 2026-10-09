# Mobile app

The pt-tools mobile app shows the overview, torrents, sites and subscriptions when you are away from home, and lets you pause, delete or send torrents and manage subscriptions within the permissions chosen when pairing. The app reaches pt-tools through [remote access](remote-access.md): scan the QR code in the web UI to pair, and the app then tries the direct address first and falls back to a relay. Connections are end-to-end encrypted; a relay only forwards traffic and cannot read it.

The app runs on Android (no App Store release for iOS yet). The interface is in Chinese and English and follows the phone's language.

## Pairing

1. In the pt-tools web UI, open System → Remote access, make sure remote access is on and a direct address or a relay is set, see [Turn on remote access](remote-access.md#turn-it-on).
2. Click Add device, choose the permissions (full control or read-only) and generate the QR code. The QR code is valid for 10 minutes and works once.
3. Open the app and tap Scan QR code, or copy the link from the web UI to the phone and paste it into Pairing link. Scan with the app itself, not the phone's camera or another scanner app: the QR code carries the pairing secret, and whoever reads it can pair.
4. Enter a device name (shown in the web UI's device list) and tap Pair.

After pairing, the window in the web UI shows that the device is paired and the app opens the overview. If the pairing window has expired, was used, or had too many wrong attempts, the app asks you to generate a new QR code in the web UI.

## Connection

- The app tries the direct address from the pairing link first (on the same network, or when it is reachable from outside), then the relays in order. The current method and address are under More → Connection & device.
- The app keeps the connection only in the foreground: it disconnects in the background and reconnects when you come back. It also reconnects when the connection drops.
- When the device's permissions change (for example from full control to read-only), the app reconnects at once and shows the new permissions.
- When the device is revoked in the web UI, or the host's keys change, the app shows Pair again, which takes you back to the pairing page.
- When remote access is turned off in the web UI, the app says so and tries again a little later.

## What you can do

| Page                       | Content                                                                                                                                                  |
| -------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Overview                   | Uploaded, downloaded, ratio, bonus, seeding and unread messages across enabled sites, today's numbers, downloader speeds and free space                  |
| Torrents                   | Torrents in your downloaders, filtered by state or title; pause, resume, delete (with a confirmation, optionally with the data)                          |
| Search                     | Search all enabled sites and send a result to a downloader (with the usual disk-space and seeding-capacity checks)                                       |
| Media                      | Subscriptions (progress, missing episodes, search now, pause, delete), recently added items, and Explore (trending, popular, search, subscribe directly) |
| More → Sites               | Login status (days left before the inactivity limit), today's check-in, user data and unread messages; check in now                                      |
| More → Tasks, Brush        | Send history, and brush tasks with their active torrents and totals                                                                                      |
| More → Downloaders         | Downloader speeds, free space and versions                                                                                                               |
| More → Connection & device | Connection method and address, host, this device's name and permissions; reconnect, forget this host                                                     |

Read-only devices don't see write actions (pause, delete, send, check in, subscribe).

## Privacy

- The app only talks to your pt-tools: site icons and TMDB posters are fetched by pt-tools and sent to the phone over the encrypted connection, so the phone never contacts the sites or TMDB.
- This device's private key stays in the phone's secure storage (Android Keystore). Forget this host only removes the record on the phone; the device record stays in the web UI, where you can revoke it.
- A relay only sees connection metadata (hostId, IP addresses, times and traffic sizes), see [remote access](remote-access.md#audit-and-security).

## FAQ

**Pairing says the host has no pairing window open**: the QR code expired or was already used. Click Add device in the web UI again.

**The app keeps showing Connecting**: the phone can't reach the direct address or any relay. Check in the web UI under System → Remote access that the relay shows as connected. When you are away from home, the direct address must be reachable from outside, or set up a relay, see [Run your own relay](remote-access.md#run-your-own-relay).

**The app says the host uses a different App API level**: pt-tools changed the App API in an incompatible way. Install the new version of the app.
