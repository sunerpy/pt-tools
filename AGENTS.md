# AGENTS.md — pt-tools Codebase Guide

**Refreshed:** 2026-09-15
**Baseline:** `b0b4d97` (`v0.47.2`)
**Worktree:** `docs/project-foundation`

## Overview

`pt-tools` is a Private Tracker automation service with a Cobra CLI, an authenticated Web UI, RSS download automation, multi-site search and statistics, downloader management, ChatOps/notification channels, login-expiry monitoring, and a companion browser extension.

**Stack:** Go 1.26.7 | Cobra | stdlib `http.ServeMux` | GORM + SQLite/WAL | Zap | Vue 3 + Vite + Element Plus + Pinia | pnpm

## Repository Map

```text
pt-tools/
├── cmd/                    # Cobra commands and production dependency wiring
├── config/                 # Zap/lumberjack configuration
├── core/                   # Runtime initialization, ConfigStore, legacy migration
├── global/                 # Logger and DB singletons; keep usage bounded
├── internal/
│   ├── app/                # Application services (ChatOps, notification, RSS callbacks)
│   ├── chatops/            # Command registry, permission chain, sessions, rate limits
│   ├── cloakdriver/        # CloakBrowser Manager + CDP login-probe fallback
│   ├── crypto/             # AES-GCM key lifecycle and credential encryption
│   ├── events/             # Non-blocking in-process event bus
│   ├── extension/          # Browser-extension pending actions
│   ├── filter/             # RSS filter matcher chain
│   ├── maintenance/        # Safe logs/staging/backups cleanup
│   ├── mcp/                # Interface-only future MCP contract; no server runtime
│   ├── notify/             # Channel registry/router/outbox/digest/adapters
│   └── sitelogin/          # Login probe classification and dispatch
├── models/                 # GORM schema, schema migrations, presets, repositories
├── scheduler/              # RSS jobs and free-end/cleanup/peer/login monitors
├── site/v2/                # Site definitions, drivers, search and user-info services
├── thirdpart/downloader/   # qBittorrent/Transmission interface and implementations
├── tools/browser-extension/# MV3 companion extension
├── utils/                  # Paths, time, HTTP and locking helpers
├── version/                # Update checker and binary self-replacement
├── web/                    # HTTP server and JSON APIs
└── web/frontend/           # Vue SPA
```

## Where to Work

| Task                         | Primary location                                             | Required follow-through                                                                                               |
| ---------------------------- | ------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------- |
| Add a PT site                | `site/v2/definitions/`                                       | Load the `pt-add-site` skill; add fixture coverage, update extension `KNOWN_SITES`, run `make check-sites`            |
| Add a Cobra command          | `cmd/<name>.go`                                              | Register with the intended parent in `init()` and add `cmd/*_test.go`                                                 |
| Add an HTTP endpoint         | `web/api_<feature>.go`                                       | Register in `Server.Serve()` or a dedicated `register*Routes` helper; wrap protected routes with `s.auth`             |
| Add a ChatOps command        | `internal/chatops/commands/`                                 | Register a `chatops.CommandSpec`; use injected services and preserve audit/permission flow                            |
| Add a notification channel   | `internal/notify/adapter/<type>/`                            | Implement `notify.Channel`, register its factory, wire production side-effect import, test inbound/outbound lifecycle |
| Change runtime config        | `core/config_store.go`, `models/config_models.go`            | Preserve partial-update semantics, encryption, and `events.ConfigChanged` publication                                 |
| Change DB schema             | `models/` + `models/schema_version.go`                       | Add `AutoMigrate` entry and a numbered migration when existing data needs transformation                              |
| Change RSS pipeline          | `internal/common.go`, `internal/push.go`                     | Preserve downloader-selection, disk-budget, site-capacity, idempotency, and notification gates                        |
| Change scheduler behavior    | `scheduler/`                                                 | Preserve managed-torrent boundaries, cancellation, monitor idempotency, and event debouncing                          |
| Change site login monitoring | `internal/sitelogin/`, `scheduler/login_reminder_monitor.go` | Keep all I/O behind site/v2 or Cloak drivers and keep the shared per-site single-flight gate                          |
| Change browser helper        | `tools/browser-extension/`                                   | Run extension typecheck/tests plus `make check-sites`                                                                 |
| Change future MCP design     | `internal/mcp/contract.go`, `docs/design/phase4-mcp.md`      | Do not claim a transport/server exists; the current package is contract-only                                          |

## Runtime Wiring

```text
main.go
  → cmd.Execute()
  → root command defaults to cmd.webCmd
  → core.InitRuntime()
      logger → SQLite/WAL → AutoMigrate → numbered schema migrations
      → legacy v1 config migration → optional one-time v2 broadcast
  → SiteRegistry + ConfigStore.SyncSites
  → scheduler.NewManager() + downloader/monitor initialization
  → UserInfoService + CachedSearchOrchestrator
  → ChatOps/notify bootstrap + RSS digest/retry/callback wiring
  → LoginReminderMonitor
  → web.NewServer(...).Serve(addr)
```

Important startup properties:

- Site definitions and notification adapters are registered by side-effect imports from `cmd/`.
- Scheduler auto-start reload is dispatched asynchronously; an unreachable downloader must not delay HTTP listen.
- A bad notification channel is logged and skipped rather than taking down the Web service.
- SIGINT/SIGTERM shuts down channel adapters, outbox/session workers, then the HTTP server with bounded timeouts.

## Core Contracts

### Configuration and secrets

- SQLite is the source of truth. Use `core.ConfigStore`; do not resurrect `GlobalCfg` or a parallel config file.
- Site cookies are stored in `SiteSetting.CookieEncrypted`; plaintext exists only at trusted boundaries through `ConfigStore.EncryptCookie`/`DecryptCookie`.
- The AES-256 key comes from base64 `PT_TOOLS_SECRET_KEY` or hex text in `~/.pt-tools/secret.key`. The current `secret import` implementation writes raw bytes instead of the loader's hex format; do not rely on restore until that mismatch is fixed and round-trip tested.
- Config mutations publish `events.ConfigChanged`; consumers must tolerate dropped duplicate events and reload from DB.

### Site and RSS execution

- New code uses `internal.UnifiedPTSite`; the generic `PTSiteInter[T]` path is compatibility-only.
- Site definitions register through `site/v2/definitions/*` and `SiteDefinitionRegistry`; do not hand-edit the runtime site registry for one site.
- Downloader selection order is RSS binding → site binding → default downloader.
- Requests to tracker sites go through the site/v2 HTTP/driver layer so rate limiting, retries, circuit breaking, and error classification stay consistent.

### Push safety

A push is not just `AddTorrentFileEx`. Preserve the complete gate:

```text
client free space
  - active incomplete bytes reported by downloader
  - process-local DiskBudget reservation
  - current torrent size
  >= configured reserve threshold
```

`internal.PushMutex()` serializes check + reserve + push. The site seeding-capacity gate runs before the downloader mutation. Disk-space reads fail closed when protection is enabled. Failed pushes release reservations; cleanup resets stale reservations under the same mutex.

### Notifications and ChatOps

- `internal/chatops.MessageChain` owns binding, permission, parsing, session, rate-limit, admin, audit, and reply sequencing. Do not call command handlers around this chain for inbound user traffic.
- `internal/notify.Registry` creates channels; `Router` fans out with dedupe/timeouts; `NotificationService` and `OutboxWorker` provide immediate delivery plus durable retry.
- RSS notifications are idempotent through `RSSNotificationLog`, honor quiet hours and per-hour quotas, and may be combined by `DigestBuffer`.
- Notification configuration contains secrets and is encrypted at persistence boundaries. List DTOs redact `ConfigJSON`; the authenticated detail editor intentionally decrypts it. Do not broaden that disclosure or log it.

## Commands and Validation

```bash
# Fast targeted Go test
CGO_ENABLED=1 go test ./path/to/pkg -count=1

# Project gates
make toolchain-check
make fmt-check
make lint
make test                 # frontend build + Go race tests
make build                # real frontend + production binary
make coverage-gate        # race tests + filtered 90% project gate
make check                # toolchain + format + lint + test + build

# Frontend
pnpm --dir web/frontend test
pnpm --dir web/frontend build

# Browser extension
make check-sites
pnpm --dir tools/browser-extension typecheck
pnpm --dir tools/browser-extension test
make build-extension
```

`make embed-placeholder` is for lint/security compilation only. A binary built with placeholder assets is not shippable.

## Style and Testing

- Imports: stdlib, third-party, then `github.com/sunerpy/pt-tools/...`; `goimports -local github.com/sunerpy/pt-tools` enforces grouping.
- Format Go with `gofumpt -extra`; format repository text/frontend with `oxfmt`.
- Wrap errors with context (`fmt.Errorf("...: %w", err)`); Chinese user-facing errors are allowed.
- Use `testify/assert` and `require`, table tests, `t.TempDir()`, `httptest`, and package-local fakes.
- Keep tests hermetic. Live Chrome/CDP and self-upgrade/GitHub behavior are isolated from the project coverage gate.
- Tests that replace globals must restore them with `t.Cleanup`.

## Never Do This

| Avoid                                                          | Use instead                                                    |
| -------------------------------------------------------------- | -------------------------------------------------------------- |
| `GlobalCfg` or new file-backed runtime config                  | `core.ConfigStore` + SQLite                                    |
| New `PTSiteInter[T]` code                                      | `UnifiedPTSite` / site/v2 `Site`                               |
| `AddTorrent`, `AddTorrentWithPath`, `GetDiskSpace`             | `AddTorrentEx`, `AddTorrentFileEx`, `GetClientFreeSpace`       |
| Direct downloader push that skips disk/capacity checks         | `internal.PushTorrentToDownloader` or the unified RSS pipeline |
| Direct plaintext reads/writes of stored cookie/channel secrets | ConfigStore/crypto boundary methods                            |
| Editing `site/v2/registry.go` for one site                     | A definition + fixture + extension mapping                     |
| Raw tracker HTTP in schedulers/login monitors                  | site/v2 or Cloak driver abstractions                           |
| Destructive cleanup without a preview/confirmation gate        | `maintenance.Cleaner` / `pt-tools clean --confirm`             |
| Treating `internal/mcp` as implemented MCP runtime             | Describe it as an interface-only future contract               |
| `0755` literals or ignored errors                              | `0o755`; return or log errors                                  |

## Subdirectory Guides

- [`cmd/AGENTS.md`](cmd/AGENTS.md)
- [`core/AGENTS.md`](core/AGENTS.md)
- [`internal/AGENTS.md`](internal/AGENTS.md)
- [`internal/chatops/AGENTS.md`](internal/chatops/AGENTS.md)
- [`internal/notify/AGENTS.md`](internal/notify/AGENTS.md)
- [`models/AGENTS.md`](models/AGENTS.md)
- [`scheduler/AGENTS.md`](scheduler/AGENTS.md)
- [`site/v2/AGENTS.md`](site/v2/AGENTS.md)
- [`thirdpart/downloader/AGENTS.md`](thirdpart/downloader/AGENTS.md)
- [`tools/browser-extension/AGENTS.md`](tools/browser-extension/AGENTS.md)
- [`web/AGENTS.md`](web/AGENTS.md)
