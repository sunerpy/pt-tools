package store

import (
	"time"

	"gorm.io/gorm"
)

// MediaLibraryConfig 媒体库配置（一个库对应一个路径 + connector 关联）
type MediaLibraryConfig struct {
	ID          uint           `gorm:"primaryKey"              json:"id"`
	Name        string         `gorm:"uniqueIndex;size:128"    json:"name"`
	Type        string         `gorm:"size:32"                 json:"type"` // "movie"/"tv"/"mixed"
	Path        string         `gorm:"size:1024"               json:"path"`
	Enabled     bool           `gorm:"default:true"            json:"enabled"`
	ProviderIDs string         `gorm:"size:512"                json:"provider_ids"` // CSV: "tmdb,douban"
	ConnectorID *uint          `gorm:"index"                   json:"connector_id,omitempty"`
	ScanCron    string         `gorm:"size:64"                 json:"scan_cron"`     // "0 */6 * * *"
	AutoScrape  bool           `gorm:"default:false"           json:"auto_scrape"`   // 订阅 TorrentCompleted 触发
	NfoDialect  string         `gorm:"size:32;default:universal" json:"nfo_dialect"` // kodi/jellyfin/emby/universal
	LastScanAt  *time.Time     `                               json:"last_scan_at,omitempty"`
	CreatedAt   time.Time      `                               json:"created_at"`
	UpdatedAt   time.Time      `                               json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index"                   json:"-"`
}

func (MediaLibraryConfig) TableName() string { return "media_library_configs" }

// ProviderCredential 数据源凭证（TMDB/豆瓣 API key 等；LLM provider 的 key 也存这里）
type ProviderCredential struct {
	ID          uint           `gorm:"primaryKey"          json:"id"`
	Provider    string         `gorm:"uniqueIndex;size:64" json:"provider"` // "tmdb"/"douban"/"openai"/"kimi"/...
	DisplayName string         `gorm:"size:128"            json:"display_name"`
	APIKey      string         `gorm:"size:512"            json:"-"`          // 不暴露给前端
	BearerToken string         `gorm:"size:2048"           json:"-"`          // TMDB v4 bearer
	BaseURL     string         `gorm:"size:512"            json:"base_url"`   // 自定义 endpoint
	ModelName   string         `gorm:"size:128"            json:"model_name"` // LLM 用
	ProxyURL    string         `gorm:"size:256"            json:"proxy_url"`
	ExtraConfig string         `gorm:"type:text"           json:"extra_config"` // JSON 冗余参数
	Priority    int            `gorm:"default:50"          json:"priority"`
	Enabled     bool           `gorm:"default:true"        json:"enabled"`
	CreatedAt   time.Time      `                           json:"created_at"`
	UpdatedAt   time.Time      `                           json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index"               json:"-"`
}

func (ProviderCredential) TableName() string { return "provider_credentials" }

// ConnectorConfig 媒体服务器配置（Jellyfin/Emby）
type ConnectorConfig struct {
	ID           uint           `gorm:"primaryKey"          json:"id"`
	Type         string         `gorm:"size:32"             json:"type"` // "jellyfin"/"emby"
	Name         string         `gorm:"uniqueIndex;size:128" json:"name"`
	BaseURL      string         `gorm:"size:512"            json:"base_url"`
	APIKey       string         `gorm:"size:512"            json:"-"`
	AutoDetected bool           `gorm:"default:false"       json:"auto_detected"` // 从 ProductName 自动探测
	Enabled      bool           `gorm:"default:true"        json:"enabled"`
	LastPingAt   *time.Time     `                           json:"last_ping_at,omitempty"`
	LastPingOK   bool           `gorm:"default:false"       json:"last_ping_ok"`
	CreatedAt    time.Time      `                           json:"created_at"`
	UpdatedAt    time.Time      `                           json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index"               json:"-"`
}

func (ConnectorConfig) TableName() string { return "connector_configs" }

// ScrapeTask 刮削任务（队列持久化）
type ScrapeTask struct {
	ID           uint       `gorm:"primaryKey"              json:"id"`
	LibraryID    *uint      `gorm:"index"                   json:"library_id,omitempty"`
	TaskType     string     `gorm:"size:32;index"           json:"task_type"` // "movie"/"tv"/"episode"/"bulk"
	MediaPath    string     `gorm:"size:1024;index"         json:"media_path"`
	State        string     `gorm:"size:32;index"           json:"state"`         // pending/running/success/failed/retrying/canceled
	CurrentStage string     `gorm:"size:64"                 json:"current_stage"` // searching/fetching/fusing/writing_nfo/downloading_art/refreshing_server/done
	Progress     float64    `                               json:"progress"`
	RetryCount   int        `                               json:"retry_count"`
	MaxRetries   int        `gorm:"default:3"               json:"max_retries"`
	NextRetryAt  *time.Time `gorm:"index"                   json:"next_retry_at,omitempty"`
	LastError    string     `gorm:"type:text"               json:"last_error"`
	RequestData  string     `gorm:"type:text"               json:"request_data"` // JSON 原始请求
	StartedAt    *time.Time `                               json:"started_at,omitempty"`
	CompletedAt  *time.Time `                               json:"completed_at,omitempty"`
	CreatedAt    time.Time  `gorm:"index"                   json:"created_at"`
	UpdatedAt    time.Time  `                               json:"updated_at"`
}

func (ScrapeTask) TableName() string { return "scrape_tasks" }

// ScrapeResult 刮削结果（一个 Task 可能有 1+ 结果，例如 TV show 多 episode）
type ScrapeResult struct {
	ID          uint      `gorm:"primaryKey"              json:"id"`
	TaskID      uint      `gorm:"index"                   json:"task_id"`
	LibraryID   *uint     `gorm:"index"                   json:"library_id,omitempty"`
	MediaType   string    `gorm:"size:32"                 json:"media_type"` // movie/tv/season/episode
	Title       string    `gorm:"size:256"                json:"title"`
	Year        int       `                               json:"year"`
	FilePath    string    `gorm:"size:1024;index"         json:"file_path"`
	NfoPath     string    `gorm:"size:1024"               json:"nfo_path"`
	PosterPath  string    `gorm:"size:1024"               json:"poster_path"`
	FanartPath  string    `gorm:"size:1024"               json:"fanart_path"`
	UnifiedData string    `gorm:"type:text"               json:"unified_data"` // JSON serialized UnifiedMediaInfo
	Providers   string    `gorm:"size:256"                json:"providers"`    // "tmdb,douban,llm"
	Warnings    string    `gorm:"type:text"               json:"warnings"`     // JSON array
	ScrapedAt   time.Time `gorm:"index"                   json:"scraped_at"`
	CreatedAt   time.Time `                               json:"created_at"`
	UpdatedAt   time.Time `                               json:"updated_at"`
}

func (ScrapeResult) TableName() string { return "scrape_results" }

// ScraperOverride 用户手动覆盖的字段
type ScraperOverride struct {
	ID        uint      `gorm:"primaryKey"           json:"id"`
	ResultID  uint      `gorm:"index:idx_field,unique;index" json:"result_id"`
	FieldName string    `gorm:"size:64;index:idx_field,unique" json:"field_name"` // "title"/"plot"/"poster_url"
	Value     string    `gorm:"type:text"            json:"value"`
	UpdatedAt time.Time `                            json:"updated_at"`
}

func (ScraperOverride) TableName() string { return "scraper_overrides" }

// AllModels 返回所有需要 migration 的模型（用于 AutoMigrate）
func AllModels() []any {
	return []any{
		&MediaLibraryConfig{},
		&ProviderCredential{},
		&ConnectorConfig{},
		&ScrapeTask{},
		&ScrapeResult{},
		&ScraperOverride{},
	}
}
