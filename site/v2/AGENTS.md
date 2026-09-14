# `site/v2/` — Tracker Site Definitions and Drivers

## Role

`site/v2` normalizes heterogeneous tracker architectures behind one `Site` interface. Site-specific metadata belongs in `definitions/`; reusable protocol behavior belongs in architecture drivers.

## Architecture

```text
site/v2/
├── definitions/            # One definition per supported site + fixture tests
├── types.go                # Site, Driver, shared DTOs and errors
├── site_definition.go      # Declarative site schema/selectors/options
├── definition_registry.go  # Definition registration from init()
├── registry.go / factory.go# Metadata and authenticated Site construction
├── driver_registry.go      # Schema → driver factory
├── nexusphp_driver.go      # NexusPHP HTML/cookie
├── mtorrent_driver.go      # mTorrent JSON/API key
├── unit3d_driver.go        # Unit3D API key
├── gazelle_driver.go       # Gazelle cookie
├── hddolby_driver.go       # HDDolby hybrid API/cookie
├── http_client.go          # Retry/rate-limit/circuit-breaker request layer
├── search_orchestrator.go  # Concurrent multi-site search
├── search_cache.go         # TTL result cache
├── userinfo_service.go     # Cached user stats
└── ranker/deduper/filters/normalizer/... # Result processing
```

## Core Interfaces

```go
type Site interface {
    ID() string
    Name() string
    Kind() SiteKind
    Login(context.Context, Credentials) error
    Search(context.Context, SearchQuery) ([]TorrentItem, error)
    GetUserInfo(context.Context) (UserInfo, error)
    Download(context.Context, string) ([]byte, error)
    Close() error
}
```

`Driver[Req, Res]` owns prepare/execute/parse for search and download plus user-info fetch. Optional interfaces such as `HashDownloader`, `TorrentDetailFetcher`, and `DetailFetcherProvider` add capabilities without widening `Site`.

## Supported Schemas

| Schema     | Site kind      | Default auth      | Notes                 |
| ---------- | -------------- | ----------------- | --------------------- |
| `NexusPHP` | `SiteNexusPHP` | Cookie            | Most Chinese trackers |
| `mTorrent` | `SiteMTorrent` | API key           | M-Team API            |
| `Unit3D`   | `SiteUnit3D`   | API key           | Unit3D family         |
| `Gazelle`  | `SiteGazelle`  | Cookie            | Gazelle family        |
| `HDDolby`  | `SiteHDDolby`  | Cookie/API hybrid | Dedicated driver      |
| `Rousi`    | `SiteRousi`    | Passkey           | Dedicated behavior    |

## Adding or Updating a Site

Always load the `pt-add-site` skill first. The complete change is normally more than one file:

1. Add/update `definitions/<site>.go` and register it in `init()`.
2. Add an HTML/JSON fixture and `<site>_fixture_test.go` for selectors/parsing.
3. Prefer `SiteDefinition` fields for URLs, selectors, categories, discount mapping, login/access parsing, cookies, and auth options.
4. Update `tools/browser-extension/src/core/constants.ts` `KNOWN_SITES` and domains.
5. Run the definition test, `make check-sites`, and extension tests.
6. Update user-facing supported-site docs.

Do not edit `registry.go` to add one site. Do not add a driver branch for a selector difference that the definition model can express.

## Request and Error Rules

- All tracker requests use the shared site HTTP/driver layer. It supplies retry, rate limit, failover, and circuit breaking.
- Preserve sentinel errors (`ErrSessionExpired`, `ErrRateLimited`, `ErrCircuitOpen`, `ErrNetworkError`, etc.) so login probes and APIs classify failures correctly.
- Never log cookies, API keys, passkeys, or signed download URLs.
- Normalize external timestamps at the driver boundary; persistence/probe code expects UTC.
- `Close()` must release driver resources and remain safe after partial initialization.
- Unavailable definitions stay registered as metadata but are skipped/disabled for runtime use.

## Search and User Info

The search orchestrator runs sites concurrently, then normalizes, filters, deduplicates by info hash, and ranks results. A site error is reported per-site and does not discard successful results from others. User-info and search services support dynamic unregister/re-register when credentials change.

## Tests

Fixture tests are the primary regression guard for real tracker markup. Keep them deterministic and scrub credentials/PII. Driver tests should cover auth failure, rate limiting, circuit-open, invalid responses, download validation, and context cancellation.
