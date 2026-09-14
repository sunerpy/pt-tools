# `cmd/` — Cobra Commands and Production Wiring

## Role

`cmd` is both the CLI surface and the composition root. Keep business logic in `core`, `internal/app`, `scheduler`, `site/v2`, or `web`; command files should validate flags, wire dependencies, start/stop services, and present results.

## Current Layout

```text
cmd/
├── root.go                    # Root command; defaults to web; side-effect registries
├── web.go                     # Full production bootstrap and graceful shutdown
├── login_reminder_wiring.go   # Login monitor/router/resolver wiring
├── rss.go                     # Unified RSS single/persistent helpers
├── run.go                     # Legacy hidden implementation; not registered on root
├── clean.go                   # Safe maintenance cleanup, preview by default
├── secret.go                  # AES key export/import
├── config*.go                 # Hidden legacy config init surface
├── db*.go                     # Hidden/deprecated DB helpers and timezone repair
├── task*.go                   # Task listing command
├── version.go                 # Build metadata
├── completion.go              # Bash/Zsh completion
├── hooks.go                   # Shared pre-run config checks
├── web_qa.go                  # `qa` build-tag injection endpoints
└── web_noop_qa.go             # Production no-op QA hook
```

## Root and Registration

`rootCmd.Run` delegates to `webCmd.Run`, so `pt-tools` and `pt-tools web` have the same runtime path. `root.go` imports site definitions and the webhook adapter for registration; `web.go` imports QQ, Telegram, and WeCom adapters.

When adding a command:

1. Create `cmd/<name>.go` and a package-level `*cobra.Command`.
2. Attach it to `rootCmd` or its real parent in `init()`.
3. Use `RunE` for errors; avoid `os.Exit` below the outer command boundary.
4. Put testable work in a helper accepting explicit I/O/dependencies.
5. Add targeted tests and verify it appears under the intended parent.

Do not assume a command exists because a `cobra.Command` variable exists. `runCmd` is intentionally not registered.

## Web Bootstrap Order

`webCmd.Run` currently performs:

1. Old-binary cleanup and `core.InitRuntime()`.
2. Site registry creation and `ConfigStore.SyncSites`.
3. Scheduler/downloader/free-end/cleanup/peer monitor setup.
4. User-info service and cached search orchestrator registration.
5. ChatOps bootstrap: app services, command services, live channels, outbox, sessions.
6. RSS notifier/digest/retry/callback setup and channel reloader.
7. Login reminder monitor wiring.
8. HTTP server construction, async auto-start reload, signal handler, version checker, `Serve`.

Preserve these properties:

- Auto-start `mgr.Reload` is asynchronous because downloader creation can retry for minutes.
- A malformed or offline channel is non-fatal to the rest of the service.
- Enabled channel changes are reloaded without restarting the whole process.
- Shutdown is bounded and runs dependency cleanup before HTTP shutdown.
- Test-only routes exist only through `qa` build-tag hooks.

## Command Reference

| Command                                    | Status and behavior                                                                      |
| ------------------------------------------ | ---------------------------------------------------------------------------------------- |
| `pt-tools`, `pt-tools web [--host --port]` | Primary runtime                                                                          |
| `pt-tools clean`                           | Preview-only unless `--confirm`; whitelist is logs/staging/backups                       |
| `pt-tools secret export`                   | Emits the base64 AES key to stdout; output is sensitive                                  |
| `pt-tools secret import [--force]`         | Reads base64 key from stdin and atomically writes `secret.key`; see format warning below |
| `pt-tools task list`                       | Lists task/torrent records                                                               |
| `pt-tools version`                         | Prints version/build/commit                                                              |
| `pt-tools completion bash                  | zsh`                                                                                     | Shell completion |
| `pt-tools config ...`, `pt-tools db ...`   | Hidden/deprecated compatibility surfaces                                                 |

`rss.go` contains active helpers used by scheduler/tests but is not a standalone `rss` Cobra command. New code must call the `*Unified` helpers.

**Known key-format sharp edge:** the loader expects hex text in `secret.key`, while `runSecretImport` currently writes the decoded raw 32 bytes. Do not describe import/export as a verified restore path until code and round-trip tests agree on one format.

## Cleanup Safety

`clean` operates only inside recognized `~/.pt-tools` subtrees. The effective rule is simple: only `--confirm` performs deletion. `--dry-run=false` without `--confirm` is rejected. Never weaken redline protection for the DB, key file, or live base logs.

## Tests

- Use `setupCmdTest(t)` for a temporary SQLite-backed runtime.
- Prefer helpers with injected writers/config over asserting global stdout.
- Restore `global.GlobalDB`, logger state, registries, and env variables with `t.Cleanup`.
- Lifecycle changes should cover startup non-blocking behavior and graceful shutdown.
