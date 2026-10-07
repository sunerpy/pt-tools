package models

import "time"

// ReseedTag 是辅种加进下载器时带上的标签：恢复、回滚、收尾前确认目标里的种子带着它，证明是辅种加的。
const ReseedTag = "pt-tools-reseed"

// 转移任务的种类：转移做种（从源移除）与辅种（源与目标是同一台下载器，不移除任何种子）。
const (
	JobKindTransfer = ""
	JobKindReseed   = "reseed"
)

// JobOwnerTag 是这一种任务加进下载器时带上的归属标签。
func JobOwnerTag(kind string) string {
	if kind == JobKindReseed {
		return ReseedTag
	}
	return TransferTag
}

// ReseedSetting 是 IYUU 辅种的设置（只有一行，ID 为 1）。默认关闭；列的默认值一律是零值。
type ReseedSetting struct {
	ID      uint `gorm:"primaryKey" json:"id"`
	Enabled bool `gorm:"not null;default:false" json:"enabled"`
	// TokenEncrypted 是加密后的 IYUU token；接口只返回是否已设置。
	TokenEncrypted string `gorm:"type:text;default:''" json:"-"`
	// IntervalHours 是定时运行的间隔（0 时按 ReseedDefaultIntervalHours）。
	IntervalHours int `gorm:"not null;default:0" json:"interval_hours"`
	// DownloaderIDs 是参与辅种的下载器（JSON 数组，空表示全部启用的下载器）。
	DownloaderIDs string `gorm:"type:text;default:''" json:"-"`
	// SiteNames 是允许辅种的站点（JSON 数组，空表示全部已配置、IYUU 也支持的站点）。
	SiteNames string `gorm:"type:text;default:''" json:"-"`
	// MaxPerSitePerDay 是每个站点每天最多加几个辅种（0 时按 ReseedDefaultMaxPerSitePerDay）。
	MaxPerSitePerDay int `gorm:"not null;default:0" json:"max_per_site_per_day"`
	// SidSha1 是 IYUU 的 reportExisting 返回的站点哈希，SidKey 是当时报告的站点 sid 列表；7 天内站点没变时复用。
	SidSha1   string     `gorm:"size:128;default:''" json:"-"`
	SidKey    string     `gorm:"type:text;default:''" json:"-"`
	SidAt     *time.Time `json:"-"`
	LastRunAt *time.Time `json:"last_run_at,omitempty"`
	// LastResult 是上一轮的一句话结果。
	LastResult string    `gorm:"type:text;default:''" json:"last_result"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (ReseedSetting) TableName() string { return "reseed_settings" }

// 辅种设置的默认值与范围。
const (
	ReseedDefaultIntervalHours    = 12
	ReseedMinIntervalHours        = 1
	ReseedMaxIntervalHours        = 168
	ReseedDefaultMaxPerSitePerDay = 20
	ReseedMaxPerSitePerDay        = 500
	// ReseedSidTTL 是 sid_sha1 的有效期。
	ReseedSidTTL = 7 * 24 * time.Hour
	// ReseedRetryAfter 是暂时失败（下载不到种子等）之后多久再试。
	ReseedRetryAfter = 7 * 24 * time.Hour
)

// 辅种记录的结果。
const (
	// ReseedQueued 是已经建成辅种任务，结果看任务的状态。
	ReseedQueued = "queued"
	// ReseedFailed 是下载种子、核对 info hash 或文件列表失败；同一对不再尝试。
	ReseedFailed = "failed"
)

// ReseedRecord 记下对某个站点某个种子（info hash）的一次辅种尝试，同一对只尝试一次。
type ReseedRecord struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	InfoHash string `gorm:"size:64;not null;uniqueIndex:idx_reseed_hash_site" json:"info_hash"`
	SiteName string `gorm:"size:64;not null;uniqueIndex:idx_reseed_hash_site;index" json:"site_name"`
	// TorrentID 是站内种子 ID；SourceHash 是下载器里那个数据相同的种子。
	TorrentID    string `gorm:"size:128;default:''" json:"torrent_id"`
	SourceHash   string `gorm:"size:64;default:''" json:"source_hash"`
	DownloaderID uint   `gorm:"not null;default:0" json:"downloader_id"`
	Name         string `gorm:"size:512;default:''" json:"name"`
	State        string `gorm:"size:32;not null;index" json:"state"`
	Message      string `gorm:"size:1024;default:''" json:"message"`
	// Retryable 表示这次失败是暂时的（站点不可用、下载种子失败），过了 ReseedRetryAfter 可以再试；核对没通过的不再试。
	Retryable bool      `gorm:"not null;default:false" json:"retryable"`
	JobID     *uint     `gorm:"index" json:"job_id,omitempty"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
	UpdatedAt time.Time `gorm:"index" json:"updated_at"`
}

// TableName 指定表名。
func (ReseedRecord) TableName() string { return "reseed_records" }
