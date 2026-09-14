# `scheduler/` — RSS Jobs and Runtime Monitors

## Role

The scheduler owns long-lived goroutines that reconcile DB configuration with RSS jobs and downloader state.

## Layout

```text
scheduler/
├── manager.go                 # Job lifecycle, downloader sync, config-event reload
├── free_end_monitor.go        # Progress/archive/free-period pause handling
├── cleanup_monitor.go         # Policy and low-disk cleanup
├── peer_ratio_monitor.go      # High seed/leecher competition pause/delete
├── login_reminder_monitor.go  # Site activity probes and reminder tiers
└── cron.go                    # Reminder cron parsing/matching helpers
```

## Manager

`NewManager()` creates the downloader manager and subscribes to `events.ConfigChanged`. Reload signals are debounced by 200 ms, then authoritative config is reloaded from SQLite.

```go
mgr := scheduler.NewManager()
mgr.InitFreeEndMonitor() // also downloader, cleanup, peer-ratio monitors
mgr.Reload(cfg)
mgr.StopAll()
```

Jobs are keyed by `site|rssName`. `Reload` cancels old jobs, waits up to 30 seconds for current work, re-syncs downloader factories/config, verifies the default downloader, and starts enabled RSS jobs. It must return safely when DB/config/auto-start/default downloader is unavailable.

## Monitors

### Free-end

- Loads incomplete tracked torrents with `PauseOnFreeEnd` and a downloader task ID.
- Uses individual timers plus periodic sweeps so restart/missed timers recover.
- Applies configured advance minutes, bounded to one hour.
- Checks true completion before pausing; updates progress and archives old records.
- Registers its scheduling callback through `internal.RegisterTorrentScheduler`.

### Cleanup

- Responds to interval checks and `DiskSpaceLow` wakeups.
- Scope can be managed DB torrents, tag-selected torrents, or explicit `all`; do not silently broaden it.
- Applies retention, download-state, H&R/protected tags, seeding, activity, and free-expiry guards.
- Resets `DiskBudget` under `PushMutex` to avoid racing an in-flight reservation.
- Never delete data merely because disk space is low unless cleanup is enabled and policy permits it.

### Peer ratio

- Considers only completed, managed torrents currently seeding.
- Reads tracker seed/leecher counts and pauses or removes only above configured S/L threshold.
- Records the action/reason so cleanup does not immediately conflict with it.

### Login reminder

- Is wired explicitly by `cmd/login_reminder_wiring.go`, then stored on `Manager` for API access and shutdown.
- Uses one shared per-site single-flight map across cron, REST, and extension triggers; busy manual probes surface conflict instead of duplicating I/O.
- Automatic probes honor `auto/manual/disabled`; manual probes may bypass mode restrictions.
- Tracker I/O goes through site/v2 or Cloak probe drivers.
- A post-migration silence window prevents reminder storms.

## Concurrency Rules

- `Start`/`Stop` operations must remain idempotent.
- Every goroutine needs a cancellation path and must be included in the relevant wait group.
- Do not hold manager/monitor mutexes across network calls or long waits.
- Do not replace DB reload with event payload state; event delivery is best-effort.
- Preserve downloader `Close` ownership when recreating or removing instances.

## Tests

Use fake clocks/resolvers/downloaders and in-memory SQLite. Test restart recovery, repeated start/stop, cancellation, managed-scope boundaries, single-flight conflicts, and failure behavior—not only happy-path tick execution.
