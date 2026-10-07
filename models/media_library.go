package models

import "time"

// 媒体库的种类。
const (
	MediaKindMovie = "movie"
	MediaKindTV    = "tv"
)

// 整理方式。空串当作硬链接。
const (
	MediaModeHardlink = "hardlink"
	MediaModeCopy     = "copy"
	MediaModeSymlink  = "symlink"
	MediaModeMove     = "move"
)

// MediaLibrary 是一个媒体库：一类条目（电影或剧集，可以只收动画）整理到哪个目录、怎么命名、怎么整理、要不要刮削。
// 列的默认值一律是零值（GORM 新建时会把零值换成 default）。
type MediaLibrary struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Name string `gorm:"size:64;not null;default:'';uniqueIndex" json:"name"`
	Kind string `gorm:"size:8;not null;default:''" json:"kind"`
	// Anime 为真时只收 TMDB 类型里有「动画」的条目；同类型里没有这种库时，动画也进普通的库
	Anime bool `gorm:"not null;default:false" json:"anime"`
	// Path 是 pt-tools 里看到的库目录（绝对路径）
	Path string `gorm:"size:1024;not null;default:''" json:"path"`
	// Template 是命名模板（text/template，不写扩展名）；空串用默认模板
	Template        string    `gorm:"size:1024;not null;default:''" json:"template"`
	Mode            string    `gorm:"size:16;not null;default:''" json:"mode"`
	Scrape          bool      `gorm:"not null;default:false" json:"scrape"`
	ScrapeOverwrite bool      `gorm:"not null;default:false" json:"scrape_overwrite"`
	Enabled         bool      `gorm:"not null;default:false" json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (MediaLibrary) TableName() string { return "media_libraries" }

// MediaPathMap 把下载器里看到的路径换成 pt-tools 里看到的路径（pt-tools 在容器里时两边的挂载点不同）。
// 换算时取最长的匹配前缀；没有匹配时路径不变。
type MediaPathMap struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	DownloaderID     uint      `gorm:"not null;uniqueIndex:idx_media_path_map" json:"downloader_id"`
	DownloaderPrefix string    `gorm:"size:512;not null;uniqueIndex:idx_media_path_map" json:"downloader_prefix"`
	LocalPrefix      string    `gorm:"size:512;not null;default:''" json:"local_prefix"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (MediaPathMap) TableName() string { return "media_path_maps" }

// 媒体服务器的种类与刷新方式。
const (
	MediaServerEmby     = "emby"
	MediaServerJellyfin = "jellyfin"
	MediaServerPlex     = "plex"

	// MediaRefreshPath 只通知整理到的目录（空串也是这个）；MediaRefreshLibrary 刷新整个媒体库。
	MediaRefreshPath    = "path"
	MediaRefreshLibrary = "library"
)

// MediaServer 是整理完后要通知的媒体服务器。Token（Emby、Jellyfin 的 API Key 或 Plex 的 Token）加密保存、只写不读。
// LocalPrefix → ServerPrefix 把 pt-tools 里的路径换成媒体服务器里看到的路径，两边一样时都留空。
type MediaServer struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	Name           string     `gorm:"size:64;not null;default:'';uniqueIndex" json:"name"`
	Kind           string     `gorm:"size:16;not null;default:''" json:"kind"`
	URL            string     `gorm:"size:512;not null;default:''" json:"url"`
	TokenEncrypted string     `gorm:"type:text;not null;default:''" json:"-"`
	Enabled        bool       `gorm:"not null;default:false" json:"enabled"`
	RefreshMode    string     `gorm:"size:16;not null;default:''" json:"refresh_mode"`
	LocalPrefix    string     `gorm:"size:512;not null;default:''" json:"local_prefix"`
	ServerPrefix   string     `gorm:"size:512;not null;default:''" json:"server_prefix"`
	LastError      string     `gorm:"size:1024;not null;default:''" json:"last_error"`
	LastRefreshAt  *time.Time `json:"last_refresh_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (MediaServer) TableName() string { return "media_servers" }

// MediaOrganizeSetting 是整理入库的设置（只有一行，ID 为 1）。
type MediaOrganizeSetting struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// AutoEnabled：pt-tools 推送的种子下载完成后自动整理。AutoSince 是打开的时间，只整理之后完成的种子
	AutoEnabled bool       `gorm:"not null;default:false" json:"auto_enabled"`
	AutoSince   *time.Time `json:"auto_since,omitempty"`
	// ScanEnabled：按范围定期扫描下载器里已完成的种子（不是 pt-tools 推送的也整理）
	ScanEnabled     bool `gorm:"not null;default:false" json:"scan_enabled"`
	ScanIntervalMin int  `gorm:"not null;default:0" json:"scan_interval_min"`
	// 范围：下载器 ID（JSON 数组）、分类与标签（逗号分隔）、保存路径前缀（换行分隔）；空的一项不限制
	ScopeDownloaders string `gorm:"type:text;not null;default:''" json:"-"`
	ScopeCategories  string `gorm:"size:1024;not null;default:''" json:"-"`
	ScopeTags        string `gorm:"size:1024;not null;default:''" json:"-"`
	ScopeSavePaths   string `gorm:"type:text;not null;default:''" json:"-"`
	// MinVideoMB 是视频文件的最小体积（MB），更小的当作样片跳过；0 用默认值
	MinVideoMB int `gorm:"not null;default:0" json:"min_video_mb"`
	// NotifyChannels 是入库通知发往的通道 ID（JSON 数组）；空的不通知
	NotifyChannels string `gorm:"type:text;not null;default:''" json:"-"`
	// DeleteLinksOnRemove：种子连数据一起删掉后，把从它整理出的硬链接与软链接也删掉（复制的文件不删）
	DeleteLinksOnRemove bool      `gorm:"not null;default:false" json:"delete_links_on_remove"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func (MediaOrganizeSetting) TableName() string { return "media_organize_settings" }

// 整理记录的状态与触发方式。
const (
	MediaTransferDone    = "done"
	MediaTransferFailed  = "failed"
	MediaTransferSkipped = "skipped"
	// MediaTransferRemoved 是库里的文件已经删掉的记录（删除记录时连文件一起删，或种子连数据删掉后清理了入库链接）
	MediaTransferRemoved = "removed"

	MediaTriggerAuto   = "auto"
	MediaTriggerScan   = "scan"
	MediaTriggerManual = "manual"
)

// MediaTransferHistory 是一个视频文件的整理记录。源文件（pt-tools 里的路径）唯一：同一个文件只整理一次，
// 同一事件重复到达或几种触发方式碰到一起都不会重复整理。字幕跟着视频一起整理，目标路径记在 Extras 里。
type MediaTransferHistory struct {
	ID             uint   `gorm:"primaryKey" json:"id"`
	DownloaderID   uint   `gorm:"not null;default:0;index" json:"downloader_id"`
	DownloaderName string `gorm:"size:64;not null;default:''" json:"downloader_name"`
	InfoHash       string `gorm:"size:64;not null;default:'';index" json:"info_hash"`
	TaskID         string `gorm:"size:128;not null;default:''" json:"task_id"`
	TorrentName    string `gorm:"size:512;not null;default:''" json:"torrent_name"`
	LibraryID      uint   `gorm:"not null;default:0;index" json:"library_id"`
	SourcePath     string `gorm:"size:2048;not null;uniqueIndex" json:"source_path"`
	// SaveRoot 是整理时种子保存目录在 pt-tools 里的路径：清理入库链接前确认它还在（下载目录没挂载时不清理）
	SaveRoot   string `gorm:"size:2048;not null;default:''" json:"-"`
	TargetPath string `gorm:"size:2048;not null;default:''" json:"target_path"`
	// Extras 是一起整理的字幕与刮削写的文件（JSON 数组，带文件编号）：删除库里的文件时只删这些
	Extras     string `gorm:"type:text;not null;default:''" json:"-"`
	Mode       string `gorm:"size:16;not null;default:''" json:"mode"`
	MediaType  string `gorm:"size:8;not null;default:''" json:"media_type"`
	TMDBID     int    `gorm:"column:tmdb_id;not null;default:0" json:"tmdb_id"`
	Title      string `gorm:"size:255;not null;default:''" json:"title"`
	Year       int    `gorm:"not null;default:0" json:"year"`
	Season     int    `gorm:"not null;default:0" json:"season"`
	Episode    int    `gorm:"not null;default:0" json:"episode"`
	EpisodeEnd int    `gorm:"not null;default:0" json:"episode_end"`
	Size       int64  `gorm:"not null;default:0" json:"size"`
	Status     string `gorm:"size:16;not null;default:'';index" json:"status"`
	Message    string `gorm:"size:1024;not null;default:''" json:"message"`
	Attempts   int    `gorm:"not null;default:0" json:"attempts"`
	// NextRetryAt 是失败后下次自动重试的时间；为空的不自动重试（例如没识别出来，要先纠正）
	NextRetryAt *time.Time `gorm:"index" json:"next_retry_at,omitempty"`
	// TargetFileID 是整理出的文件的设备号与 inode（Windows 是卷序列号与文件索引），删除库里的文件前用它确认还是这个文件
	TargetFileID string    `gorm:"size:64;not null;default:''" json:"-"`
	Trigger      string    `gorm:"size:16;not null;default:''" json:"trigger"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (MediaTransferHistory) TableName() string { return "media_transfer_histories" }
