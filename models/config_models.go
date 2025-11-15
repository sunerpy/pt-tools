package models

import (
	"time"
)

// Admin 用户（单用户登录）
type AdminUser struct {
	ID           uint   `gorm:"primaryKey"`
	Username     string `gorm:"uniqueIndex;size:64"`
	PasswordHash string `gorm:"size:255"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// 全局设置
type SettingsGlobal struct {
	ID                     uint      `gorm:"primaryKey" json:"id"`
	DefaultIntervalMinutes int32     `json:"default_interval_minutes"`
	DefaultEnabled         bool      `json:"default_enabled"`
	DownloadDir            string    `gorm:"not null" json:"download_dir"`
	DownloadLimitEnabled   bool      `json:"download_limit_enabled"`
	DownloadSpeedLimit     int       `json:"download_speed_limit"`
	TorrentSizeGB          int       `json:"torrent_size_gb"`
	AutoStart              bool      `json:"auto_start"`
	RetainHours            int       `json:"retain_hours" gorm:"default:24"`
	MaxRetry               int       `json:"max_retry" gorm:"default:3"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// qBittorrent 设置
type QbitSettings struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Enabled   bool      `json:"enabled"`
	URL       string    `gorm:"not null" json:"url"`
	User      string    `gorm:"not null" json:"user"`
	Password  string    `gorm:"not null" json:"password"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// 站点设置
type SiteSetting struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Name       string    `gorm:"index;size:32" json:"name"`
	Enabled    bool      `json:"enabled"`
	AuthMethod string    `gorm:"size:16;not null" json:"auth_method"`
	Cookie     string    `gorm:"size:1024" json:"cookie"`
	APIKey     string    `gorm:"size:512" json:"api_key"`
	APIUrl     string    `gorm:"size:512" json:"api_url"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// RSS 订阅
type RSSSubscription struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	SiteID          uint      `gorm:"index" json:"site_id"`
	Name            string    `gorm:"size:128;not null" json:"name"`
	URL             string    `gorm:"size:1024;not null" json:"url"`
	Category        string    `gorm:"size:128" json:"category"`
	Tag             string    `gorm:"size:128" json:"tag"`
	IntervalMinutes int32     `gorm:"check:interval_minutes >= 1" json:"interval_minutes"`
	DownloadSubPath string    `gorm:"size:256" json:"download_sub_path"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Runtime-friendly structures for API usage and scheduler/config load
type RSSConfig struct {
	Name            string `json:"name"`
	URL             string `json:"url"`
	Category        string `json:"category"`
	Tag             string `json:"tag"`
	IntervalMinutes int32  `json:"interval_minutes"`
	DownloadSubPath string `json:"download_sub_path"`
}
type SiteConfig struct {
	Enabled    *bool       `json:"enabled"`
	AuthMethod string      `json:"auth_method"`
	Cookie     string      `json:"cookie"`
	APIKey     string      `json:"api_key"`
	APIUrl     string      `json:"api_url"`
	RSS        []RSSConfig `json:"rss"`
}
type Config struct {
	Global SettingsGlobal           `json:"global"`
	Qbit   QbitSettings             `json:"qbit"`
	Sites  map[SiteGroup]SiteConfig `json:"sites"`
}
