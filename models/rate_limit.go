package models

import (
	"time"
)

// SiteRateLimit 站点速率限制记录
// 每个站点一行，使用滑动窗口算法持久化请求计数
type SiteRateLimit struct {
	SiteID       string    `gorm:"primaryKey;size:64" json:"site_id"`
	WindowStart  time.Time `gorm:"not null" json:"window_start"`   // 当前窗口开始时间
	RequestCount int       `gorm:"default:0" json:"request_count"` // 当前窗口内请求数
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (SiteRateLimit) TableName() string {
	return "site_rate_limits"
}
