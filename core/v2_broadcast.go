package core

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

// V2BroadcastSchemaVersion is the schema version that triggers the v2 upgrade
// broadcast. Centralized here so tests reference the same constant.
const V2BroadcastSchemaVersion = 10

// V2BroadcastFreshnessWindow guards against re-broadcasting on stale rows.
// If MigrationState.CompletedAt is older than this window, the broadcast is
// suppressed even when BroadcastSent=false (e.g. the row was migrated from a
// previous v2 release line that did not yet implement the sentinel flag).
const V2BroadcastFreshnessWindow = 5 * time.Minute

// V2Broadcaster delivers the one-time "v2.0 升级完成" notification.
// Implementations typically wrap an internal/notify.Router but the interface
// is kept here to avoid a dependency cycle and to make tests trivial.
type V2Broadcaster interface {
	BroadcastV2Upgrade(ctx context.Context) error
}

// V2BroadcasterFunc adapts a plain function into a V2Broadcaster.
type V2BroadcasterFunc func(ctx context.Context) error

// BroadcastV2Upgrade satisfies V2Broadcaster.
func (f V2BroadcasterFunc) BroadcastV2Upgrade(ctx context.Context) error { return f(ctx) }

// V2BroadcastResult describes the outcome of MaybeSendV2Broadcast.
type V2BroadcastResult struct {
	Sent     bool
	Reason   string
	BcastErr error
}

// MaybeSendV2Broadcast inspects MigrationState for V2BroadcastSchemaVersion
// and dispatches the upgrade broadcast at most once. It is safe to call from
// every startup; subsequent calls become no-ops once BroadcastSent=true. The
// freshness window prevents broadcasts on databases whose v10 row was
// written long ago without the sentinel column.
//
// Errors from the broadcaster are logged but never propagated upward —
// startup must not be blocked by a transient notification failure.
func MaybeSendV2Broadcast(ctx context.Context, db *gorm.DB, broadcaster V2Broadcaster, logger *zap.SugaredLogger, now time.Time) V2BroadcastResult {
	if db == nil {
		return V2BroadcastResult{Reason: "db_nil"}
	}
	if broadcaster == nil {
		return V2BroadcastResult{Reason: "broadcaster_nil"}
	}
	state, ok := models.GetMigrationState(db, V2BroadcastSchemaVersion)
	if !ok {
		return V2BroadcastResult{Reason: "no_migration_state"}
	}
	if state.BroadcastSent {
		return V2BroadcastResult{Reason: "already_sent"}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	completed := state.CompletedAt.UTC()
	if completed.IsZero() {
		return V2BroadcastResult{Reason: "completed_at_zero"}
	}
	if now.Sub(completed) > V2BroadcastFreshnessWindow {
		if logger != nil {
			logger.Infow("v2_broadcast_skipped_stale_migration",
				"completed_at", completed,
				"age", now.Sub(completed))
		}
		_ = models.MarkBroadcastSent(db, V2BroadcastSchemaVersion)
		return V2BroadcastResult{Reason: "stale"}
	}

	bcastErr := broadcaster.BroadcastV2Upgrade(ctx)
	if bcastErr != nil {
		if logger != nil {
			logger.Warnw("v2_broadcast_failed_continuing", "err", bcastErr)
		}
		return V2BroadcastResult{Sent: false, Reason: "broadcaster_error", BcastErr: bcastErr}
	}

	if err := models.MarkBroadcastSent(db, V2BroadcastSchemaVersion); err != nil {
		if logger != nil {
			logger.Warnw("v2_broadcast_mark_sent_failed", "err", err)
		}
		return V2BroadcastResult{Sent: true, Reason: "mark_failed", BcastErr: err}
	}
	if logger != nil {
		logger.Infow("v2_broadcast_sent")
	}
	return V2BroadcastResult{Sent: true, Reason: "sent"}
}

var (
	v2BroadcasterMu sync.RWMutex
	v2Broadcaster   V2Broadcaster
)

// SetV2Broadcaster registers the broadcaster used by InitRuntime.
// Production wiring (e.g. cmd/web.go) installs a concrete implementation
// after the notify.Router is constructed; if no broadcaster is registered,
// InitRuntime skips the dispatch step silently.
func SetV2Broadcaster(b V2Broadcaster) {
	v2BroadcasterMu.Lock()
	defer v2BroadcasterMu.Unlock()
	v2Broadcaster = b
}

func currentV2Broadcaster() V2Broadcaster {
	v2BroadcasterMu.RLock()
	defer v2BroadcasterMu.RUnlock()
	return v2Broadcaster
}

// ErrNoBroadcaster is returned by tests that intentionally exercise the
// "broadcaster missing" branch.
var ErrNoBroadcaster = errors.New("v2 broadcaster not configured")
