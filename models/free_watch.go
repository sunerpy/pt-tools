package models

import "time"

// 等免费的状态。
const (
	// FreeWatchWatching 在等：后台定时读种子现在的优惠，免费了就推送
	FreeWatchWatching = "watching"
	// FreeWatchPushed 已推送（或下载器里已经有这个种子）
	FreeWatchPushed = "pushed"
	// FreeWatchExpired 等到期了还没免费
	FreeWatchExpired = "expired"
	// FreeWatchCanceled 用户取消了
	FreeWatchCanceled = "canceled"
	// FreeWatchFailed 连续几次读不到种子的优惠，不再等
	FreeWatchFailed = "failed"
)

// FreeWatch 是一个「等免费再下」：记下要推送的下载器与路径，免费了才推。同一个种子同一时间只有一条在等（watching）。
// 列的默认值一律是零值（GORM 新建时会把零值换成 default）。
type FreeWatch struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	SiteName  string `gorm:"size:64;not null;index:idx_free_watch_torrent" json:"site_name"`
	TorrentID string `gorm:"size:128;not null;index:idx_free_watch_torrent" json:"torrent_id"`
	Title     string `gorm:"size:512;not null;default:''" json:"title"`
	Subtitle  string `gorm:"size:512;not null;default:''" json:"subtitle"`
	SizeBytes int64  `gorm:"not null;default:0" json:"size_bytes"`
	// HasHR、HRSeedTimeH 是建立时知道的 H&R 要求：推送时写进种子记录，自动清理的 H&R 保护要用
	HasHR       bool `gorm:"not null;default:false" json:"has_hr"`
	HRSeedTimeH int  `gorm:"not null;default:0" json:"hr_seed_time_h"`
	// 推送到哪里，同推送弹窗
	DownloaderID uint   `gorm:"not null;default:0" json:"downloader_id"`
	SavePath     string `gorm:"size:1024;not null;default:''" json:"save_path"`
	Category     string `gorm:"size:128;not null;default:''" json:"category"`
	Tags         string `gorm:"size:255;not null;default:''" json:"tags"`
	// MediaType、TMDBID、Season 是按条目搜索里建的带上的条目：推送时写进种子记录，整理入库按它整理；没有时为空与 0
	MediaType string `gorm:"size:8;not null;default:''" json:"media_type"`
	TMDBID    int    `gorm:"column:tmdb_id;not null;default:0" json:"tmdb_id"`
	Season    int    `gorm:"not null;default:0" json:"season"`
	Status    string `gorm:"size:16;not null;default:'';index" json:"status"`
	// NextCheckAt 是下一次读优惠的时间（只对在等的有意义）
	NextCheckAt *time.Time `gorm:"index" json:"next_check_at,omitempty"`
	LastCheckAt *time.Time `json:"last_check_at,omitempty"`
	// CheckCount 是读过几次；Failures 是连续读不到的次数（读到一次就清零）
	CheckCount int       `gorm:"not null;default:0" json:"check_count"`
	Failures   int       `gorm:"not null;default:0" json:"failures"`
	ExpiresAt  time.Time `gorm:"not null" json:"expires_at"`
	// InfoHash 是推送成功以后的种子哈希
	InfoHash string `gorm:"size:64;not null;default:''" json:"info_hash"`
	// Message 是最近一次检查的结果说明
	Message   string    `gorm:"size:1024;not null;default:''" json:"message"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (FreeWatch) TableName() string { return "free_watches" }
