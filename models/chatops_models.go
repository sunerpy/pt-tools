package models

import "time"

// NotificationConf 通知通道配置（Telegram / QQ / Webhook / WeCom 等）。
// ConfigJSON 由 internal/crypto AES-GCM 加密后存入，本层仅原样存取字符串。
type NotificationConf struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	ChannelType string `gorm:"size:32;not null;index:idx_notification_conf_type" json:"channel_type"`
	Name        string `gorm:"size:128;not null" json:"name"`
	ConfigJSON  string `gorm:"type:text" json:"-"`
	Enabled     bool   `gorm:"default:true;index:idx_notification_conf_enabled" json:"enabled"`
	// QuietHoursStart / QuietHoursEnd 控制静默时段，格式 "HH:MM"；空字符串表示无静默。
	// 若 start > end（如 "22:00"–"08:00"）表示窗口跨越午夜。
	QuietHoursStart string    `gorm:"column:quiet_hours_start;default:''" json:"quiet_hours_start"`
	QuietHoursEnd   string    `gorm:"column:quiet_hours_end;default:''" json:"quiet_hours_end"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// TableName 强制单数表名，避免 GORM 默认复数化与项目命名风格不一致。
func (NotificationConf) TableName() string { return "notification_conf" }

// ChannelBinding 渠道用户与 NotificationConf 的绑定关系。
// PtAdmin 标记该用户是否为管理员；Allowed 表示是否在白名单内。
type ChannelBinding struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	NotificationConfID uint       `gorm:"not null;index:idx_channel_binding_conf_user,priority:1" json:"notification_conf_id"`
	ChannelType        string     `gorm:"size:32;not null;index:idx_channel_binding_type" json:"channel_type"`
	ChannelUserID      string     `gorm:"size:128;not null;index:idx_channel_binding_conf_user,priority:2" json:"channel_user_id"`
	PtAdmin            bool       `gorm:"default:false" json:"pt_admin"`
	Allowed            bool       `gorm:"default:false" json:"allowed"`
	Label              string     `gorm:"size:128" json:"label"`
	ReplyLang          string     `gorm:"size:16;default:''" json:"reply_lang"`
	CreatedAt          time.Time  `json:"created_at"`
	LastActiveAt       *time.Time `json:"last_active_at"`
}

func (ChannelBinding) TableName() string { return "channel_binding" }

// ActionAudit 命令执行审计日志，Result 取值如 "ok" / "denied" / "error"。
// 复合索引 (notification_conf_id, channel_user_id, created_at) 用于按用户分页倒序查询。
type ActionAudit struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	NotificationConfID uint      `gorm:"not null;index:idx_action_audit_conf_user_time,priority:1" json:"notification_conf_id"`
	ChannelType        string    `gorm:"size:32;not null" json:"channel_type"`
	ChannelUserID      string    `gorm:"size:128;not null;index:idx_action_audit_conf_user_time,priority:2" json:"channel_user_id"`
	Command            string    `gorm:"size:64;not null;index:idx_action_audit_command" json:"command"`
	ArgsJSON           string    `gorm:"type:text" json:"args_json"`
	Result             string    `gorm:"size:32;not null" json:"result"`
	LatencyMs          int64     `gorm:"default:0" json:"latency_ms"`
	CreatedAt          time.Time `gorm:"index:idx_action_audit_conf_user_time,priority:3,sort:desc" json:"created_at"`
}

func (ActionAudit) TableName() string { return "action_audit" }

// BotToken 鉴权用 token / 8-char bind code 的持久化记录。
// Kind 取值如 "bind_code" / "bearer"；CodeOrTokenHash 为 bcrypt hash。
type BotToken struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	Kind            string     `gorm:"size:16;not null;index:idx_bot_token_kind" json:"kind"`
	CodeOrTokenHash string     `gorm:"size:255;not null;index:idx_bot_token_hash" json:"-"`
	Scope           string     `gorm:"size:255;default:''" json:"scope"`
	ExpiresAt       *time.Time `gorm:"index:idx_bot_token_expires" json:"expires_at"`
	UsedAt          *time.Time `json:"used_at"`
	CreatedBy       string     `gorm:"size:64" json:"created_by"`
	CreatedAt       time.Time  `json:"created_at"`
}

func (BotToken) TableName() string { return "bot_token" }

// NotificationOutbox 离线投递队列。Status 取值：pending / sent / failed / dead。
// 复合索引 (status, next_retry_at) 用于 worker 高效扫描待发送记录。
type NotificationOutbox struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	NotificationConfID uint       `gorm:"not null;index:idx_notification_outbox_conf" json:"notification_conf_id"`
	PayloadJSON        string     `gorm:"type:text;not null" json:"payload_json"`
	Status             string     `gorm:"size:16;not null;default:'pending';index:idx_notification_outbox_status_retry,priority:1" json:"status"`
	RetryCount         int        `gorm:"default:0" json:"retry_count"`
	NextRetryAt        time.Time  `gorm:"index:idx_notification_outbox_status_retry,priority:2" json:"next_retry_at"`
	CreatedAt          time.Time  `json:"created_at"`
	SentAt             *time.Time `json:"sent_at"`
	ErrorMsg           string     `gorm:"size:1024;default:''" json:"error_msg"`
}

func (NotificationOutbox) TableName() string { return "notification_outbox" }
