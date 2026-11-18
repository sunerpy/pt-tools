package models

import "time"

// MonitorNotificationLog 记录后台监控（登录提醒、战报等）的每条通知在每个通道上的投递。
//
// 决定发送时按目标通道各写一行，(source, subject, kind, event_key, notification_conf_id) 唯一，
// 同一决策重做不会重复发送。投递器按 next_retry_at 取到期的 pending 行，经 live 通道同步发送，
// 是这些通知唯一的重试者；通道处于静默时段时把 next_retry_at 顺延到静默结束。
// 时间一律以 UTC 写入，next_retry_at 的字符串比较才有序。
type MonitorNotificationLog struct {
	ID                 uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Source             string     `gorm:"size:32;not null;uniqueIndex:idx_monitor_notify_dedup,priority:1" json:"source"`
	Subject            string     `gorm:"size:64;not null;uniqueIndex:idx_monitor_notify_dedup,priority:2" json:"subject"`
	Kind               string     `gorm:"size:32;not null;uniqueIndex:idx_monitor_notify_dedup,priority:3" json:"kind"`
	EventKey           string     `gorm:"size:128;not null;uniqueIndex:idx_monitor_notify_dedup,priority:4" json:"event_key"`
	NotificationConfID uint       `gorm:"not null;uniqueIndex:idx_monitor_notify_dedup,priority:5" json:"notification_conf_id"`
	BypassQuiet        bool       `gorm:"not null;default:false" json:"bypass_quiet"`
	Result             string     `gorm:"size:16;not null;default:'pending';index:idx_monitor_notify_pending,priority:1" json:"result"`
	Attempts           int        `gorm:"not null;default:0" json:"attempts"`
	NextRetryAt        time.Time  `gorm:"index:idx_monitor_notify_pending,priority:2" json:"next_retry_at"`
	LastError          string     `gorm:"size:1024;default:''" json:"last_error,omitempty"`
	PayloadJSON        string     `gorm:"type:text" json:"payload_json,omitempty"`
	DeliveredAt        *time.Time `json:"delivered_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

const (
	MonitorNotifyPending = "pending"
	MonitorNotifySent    = "sent"
	MonitorNotifyFailed  = "failed"
)
