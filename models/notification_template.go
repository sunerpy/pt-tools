package models

import "time"

// NotificationTemplate 是一种事件的全局消息模板（Go 模板，所有通知通道共用）。
// Title、Text 空着的一项用内置文字；某种事件没有这一行就全用内置文字。
type NotificationTemplate struct {
	EventType string    `gorm:"primaryKey;size:64" json:"event_type"`
	Title     string    `gorm:"type:text;not null;default:''" json:"title"`
	Text      string    `gorm:"type:text;not null;default:''" json:"text"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (NotificationTemplate) TableName() string { return "notification_template" }
