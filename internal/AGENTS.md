# `internal/` — Business Logic and Runtime Subsystems

## Role

`internal` contains the RSS pipeline and the application subsystems that sit between Web/CLI entrypoints, tracker drivers, the DB, and downloaders.

## Map

```text
internal/
├── common.go / unified_site.go / site_factory.go # Unified RSS pipeline
├── push.go / disk_budget.go / site_capacity.go   # Downloader mutation safety
├── filter/                                        # FilterService + matchers
├── app/                                           # Application services and RSS notify flow
├── chatops/                                       # Inbound command pipeline
├── notify/                                        # Notification channels/router/outbox
├── sitelogin/                                     # Probe dispatch/status classification
├── cloakdriver/                                   # CloakBrowser Manager/CDP fallback
├── maintenance/                                   # Safe filesystem cleanup
├── crypto/                                        # AES-GCM key handling
├── events/                                        # In-process pub/sub
├── extension/                                     # Browser-extension pending actions
└── mcp/                                           # Future interface contract only
```

## Unified Site Contract

New RSS code uses:

```go
type UnifiedPTSite interface {
    GetTorrentDetails(*gofeed.Item) (*v2.TorrentItem, error)
    IsEnabled() bool
    DownloadTorrent(url, title, downloadDir string) (string, error)
    MaxRetries() int
    RetryDelay() time.Duration
    SendTorrentToDownloader(context.Context, models.RSSConfig) error
    Context() context.Context
    SiteGroup() models.SiteGroup
}
```

`PTSiteInter[T]` and non-`Unified` functions remain only for compatibility. Do not extend them.

## RSS Flow

```text
FetchAndDownloadFreeRSSUnified
  → fetch + parse feed
  → bounded worker pool from effective RSS/global concurrency
  → site/v2 detail lookup and normalization
  → FilterService download/notify decisions
  → durable TorrentInfo/RSSNotificationLog updates
  → .torrent staging/download
  → downloader selection: RSS → site → default
  → disk budget + site capacity gates
  → AddTorrentFileEx
  → free-end scheduling and event/notification publication
```

The RSS notifier is injected through `internal.SetRSSNotifier` to avoid an import cycle with `internal/app`. Keep that boundary small instead of importing `app` into the core RSS package.

## Push Invariants

When disk protection is enabled:

- Effective free bytes = downloader free bytes − incomplete pending bytes − `DiskBudget.Reserved()`.
- `PushMutex` covers space read, decision, reservation, site-capacity check, and downloader add.
- Failure to read free space is a rejection, not permission to proceed.
- A torrent that would cross the reserve threshold is skipped while later feed items may continue.
- A failed add releases its reservation.
- Site `SeedingCapacityGB` is another fail-closed mutation gate.

Do not call a downloader add method from new RSS/Web paths merely because the interface permits it. Route mutations through the safe push service.

## Filter Semantics

`filter.FilterService` combines keyword/glob, wildcard, and regex matchers with persisted rules. Rules now have a `purpose` (`download`, `notify`, `both`); do not treat a notification-only match as download authorization. RSS association and site scope remain part of the decision.

## Events

`events.Publish` is intentionally non-blocking and drops to a full subscriber buffer. `ConfigChanged` is a reload signal, not a complete state delta—consumers reload authoritative state from DB. `DiskSpaceLow` wakes cleanup. Typed payload events in `events/types.go` cover torrent, free-period, cleanup, login, and notification lifecycle events.

## Login and Cloak Boundaries

`internal/sitelogin` classifies probe results (`OK`, expired, rate limited, network, challenge, parse, key, unknown). Scheduler owns persistence/reminder timing. Cloak drivers are a browser/CDP fallback for challenge-prone probes; they must always stop launched profiles and normalize timestamps to UTC.

## Maintenance Boundary

`maintenance.Cleaner` is whitelist-based. It may manage rotated logs, staged torrent files, and old backups only. Preserve path containment, symlink/redline checks, and dry-run behavior.

## MCP Status

`internal/mcp/contract.go` enumerates future tools and JSON Schemas. It has no transport, authentication server, or tool dispatcher. Keep docs and code explicit about this distinction.

## Local Guides

- [`chatops/AGENTS.md`](chatops/AGENTS.md)
- [`notify/AGENTS.md`](notify/AGENTS.md)
