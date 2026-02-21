package models

import (
	"time"
)

// 间隔时间常量
const (
	MinIntervalMinutes     int32 = 5    // 最小间隔时间（分钟）
	DefaultIntervalMinutes int32 = 10   // 默认间隔时间（分钟）
	MaxIntervalMinutes     int32 = 1440 // 最大间隔时间（分钟）
)

// 并发数常量
const (
	MinConcurrency     int32 = 1  // 最小并发数
	DefaultConcurrency int32 = 3  // 默认并发数
	MaxConcurrency     int32 = 10 // 最大并发数
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
	ID                     uint   `gorm:"primaryKey" json:"id"`
	DefaultIntervalMinutes int32  `json:"default_interval_minutes" gorm:"default:10"` // 默认 RSS 执行间隔（分钟）
	DefaultConcurrency     int32  `json:"default_concurrency" gorm:"default:3"`       // 默认并发数
	DefaultEnabled         bool   `json:"default_enabled"`
	DownloadDir            string `gorm:"not null" json:"download_dir"`
	DownloadLimitEnabled   bool   `json:"download_limit_enabled"`
	DownloadSpeedLimit     int    `json:"download_speed_limit"`
	TorrentSizeGB          int    `json:"torrent_size_gb"`
	MinFreeMinutes         int    `json:"min_free_minutes" gorm:"default:30"`
	AutoStart              bool   `json:"auto_start"`
	RetainHours            int    `json:"retain_hours" gorm:"default:24"`
	MaxRetry               int    `json:"max_retry" gorm:"default:3"`

	// 自动删种
	CleanupEnabled        bool    `json:"cleanup_enabled" gorm:"default:false"`
	CleanupIntervalMin    int     `json:"cleanup_interval_min" gorm:"default:30"`
	CleanupScope          string  `json:"cleanup_scope" gorm:"size:16;default:'database'"`
	CleanupScopeTags      string  `json:"cleanup_scope_tags" gorm:"size:256"`
	CleanupRemoveData     bool    `json:"cleanup_remove_data" gorm:"default:true"`
	CleanupConditionMode  string  `json:"cleanup_condition_mode" gorm:"size:8;default:'or'"`
	CleanupMaxSeedTimeH   int     `json:"cleanup_max_seed_time_h" gorm:"default:0"`
	CleanupMinRatio       float64 `json:"cleanup_min_ratio" gorm:"default:0"`
	CleanupMaxInactiveH   int     `json:"cleanup_max_inactive_h" gorm:"default:0"`
	CleanupSlowSeedTimeH  int     `json:"cleanup_slow_seed_time_h" gorm:"default:0"`
	CleanupSlowMaxRatio   float64 `json:"cleanup_slow_max_ratio" gorm:"default:0"`
	CleanupDelFreeExpired bool    `json:"cleanup_del_free_expired" gorm:"default:true"`
	CleanupDiskProtect    bool    `json:"cleanup_disk_protect" gorm:"default:true"`
	CleanupMinDiskSpaceGB float64 `json:"cleanup_min_disk_space_gb" gorm:"default:50"`
	CleanupProtectDL      bool    `json:"cleanup_protect_dl" gorm:"default:false"`
	CleanupProtectHR      bool    `json:"cleanup_protect_hr" gorm:"default:true"`
	CleanupMinRetainH     int     `json:"cleanup_min_retain_h" gorm:"default:24"`
	CleanupProtectTags    string  `json:"cleanup_protect_tags" gorm:"size:256"`

	// 免费结束自动删除
	AutoDeleteOnFreeEnd bool `json:"auto_delete_on_free_end" gorm:"default:false"` // 免费期结束时自动删除未完成的种子及数据

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GetEffectiveIntervalMinutes 获取有效的间隔时间（带默认值和边界检查）
func (s *SettingsGlobal) GetEffectiveIntervalMinutes() int32 {
	if s.DefaultIntervalMinutes <= 0 {
		return DefaultIntervalMinutes
	}
	if s.DefaultIntervalMinutes < MinIntervalMinutes {
		return MinIntervalMinutes
	}
	if s.DefaultIntervalMinutes > MaxIntervalMinutes {
		return MaxIntervalMinutes
	}
	return s.DefaultIntervalMinutes
}

// GetEffectiveConcurrency 获取有效的并发数（带默认值和边界检查）
func (s *SettingsGlobal) GetEffectiveConcurrency() int32 {
	if s.DefaultConcurrency <= 0 {
		return DefaultConcurrency
	}
	if s.DefaultConcurrency < MinConcurrency {
		return MinConcurrency
	}
	if s.DefaultConcurrency > MaxConcurrency {
		return MaxConcurrency
	}
	return s.DefaultConcurrency
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

// SiteSetting 站点设置（统一表，合并原 DynamicSiteSetting）
type SiteSetting struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"uniqueIndex;size:64;not null" json:"name"`
	DisplayName  string    `gorm:"size:128" json:"display_name"`
	BaseURL      string    `gorm:"size:512" json:"base_url"`
	Enabled      bool      `json:"enabled"`
	AuthMethod   string    `gorm:"size:16;not null" json:"auth_method"`
	Cookie       string    `gorm:"size:2048" json:"cookie,omitempty"`
	APIKey       string    `gorm:"size:512" json:"api_key,omitempty"`
	APIUrl       string    `gorm:"size:512" json:"api_url,omitempty"`
	APIUrls      string    `gorm:"size:2048" json:"api_urls,omitempty"`
	Passkey      string    `gorm:"size:512" json:"passkey,omitempty"`
	DownloaderID *uint     `gorm:"index" json:"downloader_id,omitempty"`
	ParserConfig string    `gorm:"type:text" json:"parser_config,omitempty"`
	IsBuiltin    bool      `json:"is_builtin"`
	TemplateID   *uint     `gorm:"index" json:"template_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// RSS 订阅
type RSSSubscription struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	SiteID          uint      `gorm:"index" json:"site_id"`
	Name            string    `gorm:"size:128;not null" json:"name"`
	URL             string    `gorm:"size:1024;not null" json:"url"`
	Category        string    `gorm:"size:128" json:"category"`
	Tag             string    `gorm:"size:128" json:"tag"`
	IntervalMinutes int32     `gorm:"check:interval_minutes >= 1" json:"interval_minutes"` // RSS 执行间隔（分钟），0 表示使用全局设置
	Concurrency     int32     `json:"concurrency"`                                         // 并发数，0 表示使用全局设置
	DownloadSubPath string    `gorm:"size:256" json:"download_sub_path"`
	DownloadPath    string    `gorm:"size:512" json:"download_path"` // 下载器中下载任务的目标下载路径（可选）
	DownloaderID    *uint     `gorm:"index" json:"downloader_id"`    // 指定下载器，nil 表示使用默认下载器
	IsExample       bool      `json:"is_example"`                    // 是否为示例配置，示例配置不会被执行
	PauseOnFreeEnd  bool      `gorm:"default:false" json:"pause_on_free_end"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Runtime-friendly structures for API usage and scheduler/config load
type RSSConfig struct {
	ID              uint   `json:"id"`
	Name            string `json:"name"`
	URL             string `json:"url"`
	Category        string `json:"category"`
	Tag             string `json:"tag"`
	IntervalMinutes int32  `json:"interval_minutes"` // RSS 执行间隔（分钟），0 表示使用全局设置
	Concurrency     int32  `json:"concurrency"`      // 并发数，0 表示使用全局设置
	DownloadSubPath string `json:"download_sub_path"`
	DownloadPath    string `json:"download_path"`   // 下载器中下载任务的目标下载路径（可选）
	DownloaderID    *uint  `json:"downloader_id"`   // 指定下载器，nil 表示使用默认下载器
	FilterRuleIDs   []uint `json:"filter_rule_ids"` // 关联的过滤规则 ID 列表
	IsExample       bool   `json:"is_example"`      // 是否为示例配置，示例配置不会被执行
	PauseOnFreeEnd  bool   `json:"pause_on_free_end"`
}

// ShouldSkip 判断是否应该跳过此 RSS 配置
// 示例配置或 URL 为空的配置应该被跳过
func (r *RSSConfig) ShouldSkip() bool {
	return r.IsExample || r.URL == ""
}

// GetEffectiveIntervalMinutes 获取有效的间隔时间
// 优先级：RSS 配置 > 全局配置 > 默认值
func (r *RSSConfig) GetEffectiveIntervalMinutes(globalSettings *SettingsGlobal) int32 {
	// RSS 配置优先
	if r.IntervalMinutes > 0 {
		if r.IntervalMinutes < MinIntervalMinutes {
			return MinIntervalMinutes
		}
		if r.IntervalMinutes > MaxIntervalMinutes {
			return MaxIntervalMinutes
		}
		return r.IntervalMinutes
	}
	// 使用全局配置
	if globalSettings != nil {
		return globalSettings.GetEffectiveIntervalMinutes()
	}
	// 默认值
	return DefaultIntervalMinutes
}

// GetEffectiveConcurrency 获取有效的并发数
// 优先级：RSS 配置 > 全局配置 > 默认值
func (r *RSSConfig) GetEffectiveConcurrency(globalSettings *SettingsGlobal) int32 {
	// RSS 配置优先
	if r.Concurrency > 0 {
		if r.Concurrency < MinConcurrency {
			return MinConcurrency
		}
		if r.Concurrency > MaxConcurrency {
			return MaxConcurrency
		}
		return r.Concurrency
	}
	// 使用全局配置
	if globalSettings != nil {
		return globalSettings.GetEffectiveConcurrency()
	}
	// 默认值
	return DefaultConcurrency
}

// GetEffectiveDownloadPath 获取有效的下载路径
// 如果 RSS 配置了 DownloadPath 则使用，否则返回空字符串（使用下载器默认路径）
func (r *RSSConfig) GetEffectiveDownloadPath() string {
	return r.DownloadPath
}

// HasCustomDownloadPath 检查是否配置了自定义下载路径
func (r *RSSConfig) HasCustomDownloadPath() bool {
	return r.DownloadPath != ""
}

type SiteConfig struct {
	Enabled    *bool       `json:"enabled"`
	AuthMethod string      `json:"auth_method"`
	Cookie     string      `json:"cookie"`
	APIKey     string      `json:"api_key"`
	APIUrl     string      `json:"api_url"`
	Passkey    string      `json:"passkey"`
	RSS        []RSSConfig `json:"rss"`
}
type Config struct {
	Global SettingsGlobal           `json:"global"`
	Qbit   QbitSettings             `json:"qbit"`
	Sites  map[SiteGroup]SiteConfig `json:"sites"`
}

// FaviconCache 站点图标缓存（存储在数据库中）
type FaviconCache struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	SiteID      string    `gorm:"uniqueIndex;size:64;not null" json:"site_id"` // 站点标识，如 "hdsky", "mteam"
	SiteName    string    `gorm:"size:128" json:"site_name"`                   // 站点名称
	FaviconURL  string    `gorm:"size:512" json:"favicon_url"`                 // 原始 favicon URL
	Data        []byte    `gorm:"type:blob" json:"-"`                          // 图标二进制数据
	ContentType string    `gorm:"size:64" json:"content_type"`                 // MIME 类型
	ETag        string    `gorm:"size:128" json:"etag"`                        // 用于缓存验证
	LastFetched time.Time `json:"last_fetched"`                                // 最后获取时间
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
