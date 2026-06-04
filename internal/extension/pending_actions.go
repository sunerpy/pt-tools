// Package extension implements the pending-actions queue for browser
// extension polling. Backend pushes "open this site" requests; extension
// polls every 30s, executes, acks.
package extension

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// PendingActionType enumerates supported action kinds for the extension.
type PendingActionType string

const (
	// ActionOpenTab instructs the extension to open the target URL in a
	// background tab so the user can refresh login / cookies.
	ActionOpenTab PendingActionType = "open_tab"
)

// DefaultTTL is the time-to-live for an action before it expires (R-EC10).
const DefaultTTL = 24 * time.Hour

// PendingAction is a unit of work that the backend queues for the browser
// extension to execute on its next poll. Cookies and credentials never
// appear here — only a target URL plus a human-readable reason.
type PendingAction struct {
	ID        uint              `gorm:"primaryKey" json:"id"`
	Type      PendingActionType `gorm:"size:32;not null" json:"type"`
	TargetURL string            `gorm:"size:1024;not null" json:"target_url"`
	SiteName  string            `gorm:"size:64;index" json:"site_name"`
	Reason    string            `gorm:"size:256" json:"reason"`
	CreatedAt time.Time         `gorm:"index" json:"created_at"`
	AckedAt   *time.Time        `json:"acked_at,omitempty"`
	ExpiresAt time.Time         `gorm:"index" json:"expires_at"`
}

// TableName pins the GORM table name so it remains stable even if the
// package path changes.
func (PendingAction) TableName() string {
	return "extension_pending_actions"
}

// Enqueue inserts a new pending action. CreatedAt and ExpiresAt are
// populated automatically when zero (24h TTL per R-EC10).
func Enqueue(db *gorm.DB, action PendingAction) error {
	if db == nil {
		return errors.New("db is nil")
	}
	if action.Type == "" {
		return errors.New("action type is required")
	}
	if action.TargetURL == "" {
		return errors.New("target_url is required")
	}
	now := time.Now().UTC()
	if action.CreatedAt.IsZero() {
		action.CreatedAt = now
	}
	if action.ExpiresAt.IsZero() {
		action.ExpiresAt = action.CreatedAt.Add(DefaultTTL)
	}
	if err := db.Create(&action).Error; err != nil {
		return fmt.Errorf("enqueue pending action: %w", err)
	}
	return nil
}

// ListPending returns unacked, unexpired actions whose CreatedAt is strictly
// greater than sinceUnix. sinceUnix=0 means "all unacked + unexpired".
// Results are ordered by CreatedAt ascending so the extension processes
// oldest-first.
func ListPending(db *gorm.DB, sinceUnix int64) ([]PendingAction, error) {
	if db == nil {
		return nil, errors.New("db is nil")
	}
	now := time.Now().UTC()
	q := db.Model(&PendingAction{}).
		Where("acked_at IS NULL").
		Where("expires_at > ?", now)
	if sinceUnix > 0 {
		q = q.Where("created_at > ?", time.Unix(sinceUnix, 0).UTC())
	}
	var out []PendingAction
	if err := q.Order("created_at ASC, id ASC").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("list pending actions: %w", err)
	}
	return out, nil
}

// Ack marks an action as acknowledged. Calling Ack twice on the same
// actionID is a no-op (idempotent) — second call returns nil so callers
// can retry safely.
func Ack(db *gorm.DB, actionID uint) error {
	if db == nil {
		return errors.New("db is nil")
	}
	if actionID == 0 {
		return errors.New("action id is required")
	}
	var existing PendingAction
	if err := db.Where("id = ?", actionID).First(&existing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("ack: action %d not found", actionID)
		}
		return fmt.Errorf("ack: load action %d: %w", actionID, err)
	}
	if existing.AckedAt != nil {
		// Already acked — idempotent.
		return nil
	}
	now := time.Now().UTC()
	if err := db.Model(&PendingAction{}).
		Where("id = ? AND acked_at IS NULL", actionID).
		Update("acked_at", now).Error; err != nil {
		return fmt.Errorf("ack: update action %d: %w", actionID, err)
	}
	return nil
}

// AutoMigrate creates or updates the pending_actions table. Safe to call
// multiple times.
func AutoMigrate(db *gorm.DB) error {
	if db == nil {
		return errors.New("db is nil")
	}
	if err := db.AutoMigrate(&PendingAction{}); err != nil {
		return fmt.Errorf("auto migrate pending actions: %w", err)
	}
	return nil
}

// PurgeExpired deletes actions whose ExpiresAt is on or before now.
// Returns the number of rows deleted. Safe to call repeatedly.
func PurgeExpired(db *gorm.DB, now time.Time) (int64, error) {
	if db == nil {
		return 0, errors.New("db is nil")
	}
	res := db.Where("expires_at <= ?", now.UTC()).Delete(&PendingAction{})
	if res.Error != nil {
		return 0, fmt.Errorf("purge expired pending actions: %w", res.Error)
	}
	return res.RowsAffected, nil
}
