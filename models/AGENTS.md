# `models/` — GORM Schema, Repositories, and Migrations

## Role

`models` defines the SQLite persistence contract, schema history, seed/preset data, and small repositories. SQLite runs in WAL mode with one open connection for predictable write serialization.

## Main Files

```text
models/
├── init.go                    # TorrentDB, torrent/archive tables, AutoMigrate list
├── config_models.go           # Global/site/RSS/downloader/directory/favicon/cloak config
├── chatops_models.go          # NotificationConf, bindings, audits, bot tokens, outbox
├── rss_notification_log.go    # RSS delivery idempotency/retry log
├── site_login_state.go        # Login activity/probe/reminder state + repository
├── site_login_presets.go      # Per-site inactivity/reminder defaults
├── site_repository.go         # Site and user-info persistence helpers
├── filter_rule.go             # Filter rules and purpose
├── rss_filter_association.go  # RSS ↔ rule join table
├── rate_limit.go              # Persistent site rate limits
├── schema_version.go          # Numbered schema chain (current v10)
├── migration_state.go         # Post-migration runtime/broadcast sentinel
├── site_template.go           # Import/export site templates
└── presets.go                 # Fresh-install defaults
```

## Important Tables

| Model                                                           | Purpose                                                                             |
| --------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| `TorrentInfo`, `TorrentInfoArchive`                             | Active and archived torrent lifecycle, downloader task, pause/cleanup/peer metadata |
| `SettingsGlobal`                                                | Runtime defaults, cleanup, disk protection, peer-ratio policy                       |
| `SiteSetting`                                                   | Credentials, site/download bindings, per-site limits, Cloak profile fields          |
| `RSSSubscription`                                               | Schedule, downloader, filter, pause-on-free-end, notify mode/channels/quota         |
| `DownloaderSetting`, `DownloaderDirectory`                      | Named client instances and cached paths                                             |
| `FilterRule`, `RSSFilterAssociation`                            | Download/notify rule graph                                                          |
| `NotificationConf`, `ChannelBinding`, `ActionAudit`, `BotToken` | ChatOps configuration and authorization/audit state                                 |
| `NotificationOutbox`, `RSSNotificationLog`                      | Durable delivery retry and RSS idempotency                                          |
| `SiteLoginState`                                                | Probe sources/timestamps, reminder tier/config, consistency mode                    |
| `SchemaVersion`, `MigrationState`                               | Data schema history and post-migration runtime state                                |

## Schema Change Procedure

1. Modify or add the GORM model.
2. Add it to the `AutoMigrate` list in `models/init.go`.
3. If existing rows need a backfill, rename, encryption, or semantic transformation, increment `CurrentSchemaVersion` and add a migration in `registerMigrations()`.
4. Back up affected data before sensitive/destructive transformations.
5. Add both fresh-DB and previous-version upgrade tests.
6. Update JSON/API/frontend types if the field crosses a boundary.

`AutoMigrate` alone is not sufficient for semantic changes.

## Current Schema History

- v6: RSS notification fields/log.
- v7: filter-rule `purpose`.
- v8: notification quiet hours.
- v9: encrypted stored cookies + login-state table.
- v10: split API/cookie timestamps, probe mode, consistency status.

New installations record v10 directly; legacy databases run each missing migration.

## Data Invariants

- `TorrentInfo` uniqueness is `(site_name, torrent_id)`; hash is indexed but may be absent initially.
- `RSSNotificationLog` uniqueness is `(rss_id, site_name, torrent_id, notify_kind, notification_conf_id)`.
- Secrets belong in encrypted fields. Do not use legacy plaintext cookie/config fields as persistent authority.
- Time values used across probes/migrations should be stored in UTC; format in CST only at presentation boundaries.
- Settings fallback order is row-specific override → global setting → code default.
- Cleanup/peer monitors must query only their documented managed scope; model availability is not permission to mutate every downloader torrent.

## Tests

Use temporary SQLite databases and run the real migrations. Assert indexes/unique constraints where idempotency depends on them. Migration tests must verify data values and sentinel/version rows, not merely column existence.
