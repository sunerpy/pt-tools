package models

import "time"

// RSSNotificationLog records every (rss_id, torrent_id, notify_kind, conf_id)
// notification attempt. Acts as the durable record AND the idempotency guard
// (via the unique index idx_rss_notify_dedup).
//
// result values: 'sent' | 'failed' | 'suppressed' | 'pending' | 'throttled'
//   - 'sent'       — accepted by NotificationService.Push (note: may have
//     fallen back to outbox internally; not strictly equal to
//     "delivered to user")
//   - 'failed'     — Push returned an error
//   - 'suppressed' — overshadowed by a later 'filtered' notification for the
//     same (rss_id, site_name, torrent_id)
//   - 'pending'    — queued but not yet attempted (S3 retry worker uses this)
//   - 'throttled'  — rejected because RSS hit max_notifications_per_hour
type RSSNotificationLog struct {
	ID                  uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	RSSID               uint       `gorm:"column:rss_id;not null;uniqueIndex:idx_rss_notify_dedup,priority:1;index:idx_rss_notify_rss_throttle,priority:1" json:"rss_id"`
	SiteName            string     `gorm:"column:site_name;not null;uniqueIndex:idx_rss_notify_dedup,priority:2" json:"site_name"`
	TorrentID           string     `gorm:"column:torrent_id;not null;uniqueIndex:idx_rss_notify_dedup,priority:3" json:"torrent_id"`
	NotifyKind          string     `gorm:"column:notify_kind;not null;uniqueIndex:idx_rss_notify_dedup,priority:4" json:"notify_kind"`
	NotificationConfID  uint       `gorm:"column:notification_conf_id;not null;uniqueIndex:idx_rss_notify_dedup,priority:5" json:"notification_conf_id"`
	MatchedFilterRuleID *uint      `gorm:"column:matched_filter_rule_id" json:"matched_filter_rule_id,omitempty"`
	Result              string     `gorm:"not null;index:idx_rss_notify_pending,priority:1" json:"result"`
	Attempts            int        `gorm:"default:0" json:"attempts"`
	NextRetryAt         *time.Time `gorm:"column:next_retry_at;index:idx_rss_notify_pending,priority:2" json:"next_retry_at,omitempty"`
	LastError           string     `gorm:"default:''" json:"last_error,omitempty"`
	PayloadJSON         string     `gorm:"column:payload_json" json:"payload_json,omitempty"`
	DeliveredAt         *time.Time `gorm:"column:delivered_at" json:"delivered_at,omitempty"`
	CreatedAt           time.Time  `gorm:"index:idx_rss_notify_rss_throttle,priority:2" json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

func (RSSNotificationLog) TableName() string { return "rss_notification_log" }
