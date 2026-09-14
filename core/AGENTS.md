# `core/` — Runtime Initialization and ConfigStore

## Role

`core` owns one-time process initialization and the SQLite-backed configuration boundary. It does not own site/network behavior.

## Layout

```text
core/
├── init.go             # InitRuntime: logger → DB → migrations → broadcast hook
├── config_store.go     # DB configuration reads/writes and credential boundary
├── rss_validator.go    # RSS configuration validation
├── v2_broadcast.go     # Idempotent post-migration notification contract
├── testutil.go         # Temporary DB helpers
├── enter.go            # Safe logger helper
└── migration/
    ├── backup.go       # Migration table JSON backup
    └── v1_to_v2.go     # Legacy file/v1 configuration migration
```

## `InitRuntime`

`InitRuntime()` is guarded by `sync.Once` and is normally called by `cmd/web.go` (plus command tests/legacy helpers). It:

1. Applies early log env overrides and prunes old rotated logs.
2. Installs the global Zap logger.
3. Opens `~/.pt-tools/torrents.db`, enables WAL and SQLite safety settings.
4. Runs GORM `AutoMigrate` and numbered schema migrations through `models.SchemaManager`.
5. Runs the older file/v1-to-v2 configuration migration if needed.
6. Attempts the optional one-time v2 broadcast without blocking startup on delivery failure.

Do not reset `once` or call initialization repeatedly in production code.

## ConfigStore Contract

```go
store := core.NewConfigStore(global.GlobalDB)

cfg, err := store.Load()
global, err := store.GetGlobalOnly()
sites, err := store.ListSites()

err = store.SaveGlobalSettings(settings)
err = store.SaveGlobalSettingsWithPatch(settings, patch)
siteID, err := store.UpsertSite(group, siteConfig)
err = store.ReplaceSiteRSS(siteID, rss)
err = store.SyncSites(registeredSites)
```

Rules:

- SQLite is authoritative; do not introduce another in-memory/file config owner.
- Some global fields have pointer-based patch semantics. `nil` means retain current value, not write a zero.
- Mutations that affect runtime behavior must publish `events.ConfigChanged` after a successful commit.
- Reads should assemble a consistent runtime `models.Config`; callers should not manually join config tables.
- Site sync adds registered definitions and disables removed definitions; it does not justify deleting user rows casually.

## Credential Boundary

- Persist cookie material in `SiteSetting.CookieEncrypted`.
- `EncryptCookie`/`DecryptCookie` validate that a 32-byte key is available.
- The environment form is base64 `PT_TOOLS_SECRET_KEY`; the runtime file form is hex text at `~/.pt-tools/secret.key`. `cmd/secret.go` currently imports raw bytes, a known mismatch that must be fixed before claiming restore compatibility.
- Missing/corrupt keys return `KEY_ERROR` and log only once. Never silently replace a missing key when decrypting existing data.
- Never include plaintext cookie/channel configuration in API responses or log fields.

## Two Migration Systems

Do not conflate them:

- `models/schema_version.go` is the live numbered DB schema chain (currently v10). Use it for transformations of existing DB data.
- `core/migration/v1_to_v2.go` imports legacy configuration layouts into the DB-backed runtime.
- `AutoMigrate` handles table/column existence but not semantic backfills, encryption, or destructive transformations.
- Migration steps that touch sensitive data must preserve backup hooks and idempotency.

## Testing

Use `NewTempDBDir(t.TempDir())` for integration-style config tests. Test new writes for round-trip behavior, event publication, partial-update retention, and missing-key failure paths. Schema transformations also need upgrade tests starting from the previous version, not only fresh-DB tests.
