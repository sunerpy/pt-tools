# `thirdpart/downloader/` — Torrent Client Integrations

## Role

This package normalizes qBittorrent and Transmission and manages named instances, factories, defaults, site bindings, health, and reconnection.

## Layout

```text
thirdpart/downloader/
├── interface.go       # Downloader contract, DTOs, errors, options
├── manager.go         # Factory/config/instance lifecycle and DB sync
├── http_doer.go       # Injectable HTTP execution
├── qbit/              # qBittorrent Web API, including 5.2 add response
└── transmission/      # Transmission RPC
```

## Interface Areas

Implementations must cover:

- Connection: `Authenticate`, `Ping`, client version/status, `Close`.
- Capacity: `GetClientFreeSpace`, `GetIncompletePendingBytes`, `GetDiskInfo`.
- Query: all/filter/one torrent, files, trackers, paths, labels.
- Add: `AddTorrentEx`, `AddTorrentFileEx` with `AddTorrentOptions`.
- Mutation: pause/resume/remove (single and batch), category/tags/path, recheck.
- Global speed limits and health/name/type metadata.

Compatibility methods (`AddTorrent`, `AddTorrentWithPath`, `GetDiskSpace`) still exist but new callers must not use them.

## Manager Lifecycle

`DownloaderManager` stores factories and configs separately from lazy live instances. `GetDownloader` reuses a healthy instance, pings an unhealthy one, closes/recreates if necessary, and uses bounded exponential retry. `SyncFromDB` removes disabled/deleted instances, recreates changed configs, and updates the default while closing replaced clients.

Production factory registration currently happens in `scheduler.Manager.initDownloaderManager`. A new downloader requires a factory there plus Web/model/frontend support.

## `AddTorrentOptions`

Use `AddAtPaused` (inverse of legacy `AutoStart`), `SavePath`, `Category`, `Tags`, and byte/KB-based speed-limit helpers. qBittorrent 5.1+ uses `stopped`; the implementation sends both compatible pause fields. qBittorrent 5.2 may return structured add results; preserve older status handling too.

## Disk Safety Contract

`GetIncompletePendingBytes` is not optional bookkeeping. The push layer uses it to prevent several active torrents from all passing the same stale OS-free-space check. Implementations should include all active incomplete states and tolerate a bad individual item without aborting the aggregate.

Downloader methods alone do not authorize a push. New Web/RSS callers must use the internal push service so disk reservation and site-capacity gates run.

## Adding an Implementation

1. Add `<type>/config.go`, implementation, and tests.
2. Implement the entire current `Downloader` interface and close owned resources.
3. Add a `DownloaderType`, factory registration, DB/UI choices, and API capability mapping.
4. Cover auth, connection failures, context behavior, version differences, add options/results, pending-byte calculation, batch actions, and error sentinel mapping.

Avoid package-specific type assertions in callers; expose genuinely shared behavior through the interface or an explicit capability response.
