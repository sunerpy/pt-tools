package models

import "time"

// 订阅的状态。
const (
	// MediaSubActive 在找资源：主动搜索，RSS 里的种子也会拿来对。
	MediaSubActive = "active"
	// MediaSubPaused 暂停：不搜索，也不接 RSS。
	MediaSubPaused = "paused"
	// MediaSubPending 待确认：豆瓣想看建的、设成要先确认的订阅，确认后才开始找。
	MediaSubPending = "pending"
	// MediaSubDone 完成：电影入了库，或剧集这一季都入了库（开着洗版时要达到目标质量）。
	MediaSubDone = "done"
)

// 订阅从哪里建的。
const (
	MediaSubFromManual  = "manual"
	MediaSubFromExplore = "explore"
	MediaSubFromDouban  = "douban"
	MediaSubFromChatOps = "chatops"
)

// 质量档案里「要不要」的取值：空串不限，prefer 优先，require 必须有，avoid 尽量不要（只用于 HDR）。
const (
	MediaPrefPrefer  = "prefer"
	MediaPrefRequire = "require"
	MediaPrefAvoid   = "avoid"
)

// 订阅下载的种子的状态。
const (
	MediaSubTorrentDownloading = "downloading"
	MediaSubTorrentDone        = "done"
	MediaSubTorrentFailed      = "failed"
	// MediaSubTorrentReplaced 是洗版换成更好的版本以后，旧的那个
	MediaSubTorrentReplaced = "replaced"
)

// 洗版以后旧种子怎么办。
const (
	MediaUpgradeKeep   = "keep"
	MediaUpgradeDelete = "delete"
)

// MediaQualityProfile 是质量档案：订阅按它挑资源、打分。列表字段是 JSON 数组，按偏好从高到低排，空数组不限；
// 取值用 internal/media/meta 解析出的写法（2160p、BluRay、WEB-DL、H.265 等）。
type MediaQualityProfile struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	Name        string `gorm:"size:64;not null;default:'';uniqueIndex" json:"name"`
	Resolutions string `gorm:"type:text;not null;default:''" json:"-"`
	Sources     string `gorm:"type:text;not null;default:''" json:"-"`
	Codecs      string `gorm:"type:text;not null;default:''" json:"-"`
	// Remux、HDR、ChineseSubs、Free 是空串 / prefer / require（HDR 还有 avoid）
	Remux       string `gorm:"size:16;not null;default:''" json:"remux"`
	HDR         string `gorm:"column:hdr;size:16;not null;default:''" json:"hdr"`
	ChineseSubs string `gorm:"size:16;not null;default:''" json:"chinese_subs"`
	Free        string `gorm:"size:16;not null;default:''" json:"free"`
	// Groups 是偏好的制作组（JSON 数组）
	Groups string `gorm:"type:text;not null;default:''" json:"-"`
	// MinSizeGB、MaxSizeGB 是体积区间（0 不限）：电影按整部算，剧集按每集算
	MinSizeGB  float64   `gorm:"not null;default:0" json:"min_size_gb"`
	MaxSizeGB  float64   `gorm:"not null;default:0" json:"max_size_gb"`
	MinSeeders int       `gorm:"not null;default:0" json:"min_seeders"`
	ExcludeHR  bool      `gorm:"column:exclude_hr;not null;default:false" json:"exclude_hr"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (MediaQualityProfile) TableName() string { return "media_quality_profiles" }

// MediaSubscription 是一个订阅：一部电影，或剧集的一季。同一个条目的同一季只有一个订阅。
type MediaSubscription struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	MediaType     string `gorm:"size:8;not null;default:'';uniqueIndex:idx_media_sub" json:"media_type"`
	TMDBID        int    `gorm:"column:tmdb_id;not null;default:0;uniqueIndex:idx_media_sub" json:"tmdb_id"`
	Season        int    `gorm:"not null;default:0;uniqueIndex:idx_media_sub" json:"season"`
	Title         string `gorm:"size:255;not null;default:''" json:"title"`
	OriginalTitle string `gorm:"size:255;not null;default:''" json:"original_title"`
	Year          int    `gorm:"not null;default:0" json:"year"`
	// Aliases 是对种子标题时用的其他名字（JSON 数组：英文名与 TMDB 别名）
	Aliases    string `gorm:"type:text;not null;default:''" json:"-"`
	IMDbID     string `gorm:"column:imdb_id;size:16;not null;default:''" json:"imdb_id"`
	PosterPath string `gorm:"size:255;not null;default:''" json:"poster_path"`
	// ProfileID 为 0 时用设置里的默认档案
	ProfileID uint `gorm:"not null;default:0" json:"profile_id"`
	// Sites 是只在这些站点找（JSON 数组，空 = 所有启用的站点）
	Sites string `gorm:"type:text;not null;default:''" json:"-"`
	// DownloaderID 为 0 时用设置里的默认下载器，再没有时用 pt-tools 的默认下载器
	DownloaderID uint   `gorm:"not null;default:0" json:"downloader_id"`
	Category     string `gorm:"size:128;not null;default:''" json:"category"`
	Tags         string `gorm:"size:255;not null;default:''" json:"tags"`
	SavePath     string `gorm:"size:1024;not null;default:''" json:"save_path"`
	Status       string `gorm:"size:16;not null;default:'';index" json:"status"`
	// Upgrade 是洗版：下载过以后还接着找分数更高的，档案里排第一的分辨率与来源都达到才停
	Upgrade bool `gorm:"not null;default:false" json:"upgrade"`
	// BestScore、BestTitle 是已经下载的资源里分数最高的那个
	BestScore int    `gorm:"not null;default:0" json:"best_score"`
	BestTitle string `gorm:"size:512;not null;default:''" json:"best_title"`
	Source    string `gorm:"size:16;not null;default:''" json:"source"`
	DoubanID  string `gorm:"column:douban_id;size:16;not null;default:''" json:"douban_id"`
	// TotalEpisodes 是这一季的集数（来自 TMDB），电影为 0
	TotalEpisodes int        `gorm:"not null;default:0" json:"total_episodes"`
	LastSearchAt  *time.Time `json:"last_search_at,omitempty"`
	NextSearchAt  *time.Time `gorm:"index" json:"next_search_at,omitempty"`
	// Message 是最近一次搜索或下载的结果说明
	Message   string    `gorm:"size:1024;not null;default:''" json:"message"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (MediaSubscription) TableName() string { return "media_subscriptions" }

// MediaSubscriptionTorrent 是订阅下载的一个种子：同一个订阅里同一个站点的同一个种子只记一次。
type MediaSubscriptionTorrent struct {
	ID             uint   `gorm:"primaryKey" json:"id"`
	SubscriptionID uint   `gorm:"not null;uniqueIndex:idx_media_sub_torrent" json:"subscription_id"`
	SiteName       string `gorm:"size:64;not null;uniqueIndex:idx_media_sub_torrent" json:"site_name"`
	TorrentID      string `gorm:"size:128;not null;uniqueIndex:idx_media_sub_torrent" json:"torrent_id"`
	InfoHash       string `gorm:"size:64;not null;default:'';index" json:"info_hash"`
	Title          string `gorm:"size:512;not null;default:''" json:"title"`
	Score          int    `gorm:"not null;default:0" json:"score"`
	// Episode、EpisodeEnd 是种子里的集（剧集，单集时 EpisodeEnd 为 0）；整季包 Complete 为真
	Episode      int       `gorm:"not null;default:0" json:"episode"`
	EpisodeEnd   int       `gorm:"not null;default:0" json:"episode_end"`
	Complete     bool      `gorm:"not null;default:false" json:"complete"`
	SizeBytes    int64     `gorm:"not null;default:0" json:"size_bytes"`
	DownloaderID uint      `gorm:"not null;default:0" json:"downloader_id"`
	Status       string    `gorm:"size:16;not null;default:'';index" json:"status"`
	Message      string    `gorm:"size:1024;not null;default:''" json:"message"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (MediaSubscriptionTorrent) TableName() string { return "media_subscription_torrents" }

// MediaSubscribeSetting 是订阅的设置，只有一行（ID 为 1）。
type MediaSubscribeSetting struct {
	ID uint `gorm:"primaryKey" json:"-"`
	// Enabled 是总开关：关掉时不搜索、不接 RSS、不拉豆瓣
	Enabled bool `gorm:"not null;default:false" json:"enabled"`
	// SearchIntervalHours 是每个订阅主动搜索的间隔（最少 6 小时），每次另加最多 30 分钟的随机偏移
	SearchIntervalHours int `gorm:"not null;default:0" json:"search_interval_hours"`
	// SearchSkipSites 是不参与主动搜索的站点（JSON 数组；RSS 里的种子照样对）
	SearchSkipSites     string `gorm:"type:text;not null;default:''" json:"-"`
	DefaultProfileID    uint   `gorm:"not null;default:0" json:"default_profile_id"`
	DefaultDownloaderID uint   `gorm:"not null;default:0" json:"default_downloader_id"`
	// NotifyChannels 是下载了订阅的资源时发通知的通道（JSON 数组）
	NotifyChannels string `gorm:"type:text;not null;default:''" json:"-"`
	// UpgradeOld 是洗版以后旧种子怎么办：keep（空串也是）继续做种，delete 连数据删掉
	UpgradeOld string    `gorm:"size:16;not null;default:''" json:"upgrade_old"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (MediaSubscribeSetting) TableName() string { return "media_subscribe_settings" }

// MediaDoubanSource 是一个豆瓣用户的想看：定时拉取公开的 RSS，给「想看」的电影与剧集建订阅。
type MediaDoubanSource struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	UserID string `gorm:"size:64;not null;default:'';uniqueIndex" json:"user_id"`
	// Name 是界面上显示的名字（可以不填）
	Name    string `gorm:"size:64;not null;default:''" json:"name"`
	Enabled bool   `gorm:"not null;default:false" json:"enabled"`
	// Confirm 为真时建出的订阅先是待确认
	Confirm     bool       `gorm:"not null;default:false" json:"confirm"`
	ProfileID   uint       `gorm:"not null;default:0" json:"profile_id"`
	LastFetchAt *time.Time `json:"last_fetch_at,omitempty"`
	NextFetchAt *time.Time `gorm:"index" json:"next_fetch_at,omitempty"`
	// Failures 是连续失败的次数；到 3 次标成异常，并发一次通知（Notified 记下发过了）
	Failures  int       `gorm:"not null;default:0" json:"failures"`
	LastError string    `gorm:"size:1024;not null;default:''" json:"last_error"`
	Abnormal  bool      `gorm:"not null;default:false" json:"abnormal"`
	Notified  bool      `gorm:"not null;default:false" json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (MediaDoubanSource) TableName() string { return "media_douban_sources" }

// 豆瓣想看里条目的处理结果。
const (
	MediaDoubanSubscribed = "subscribed"
	MediaDoubanUnmatched  = "unmatched"
)

// MediaDoubanItem 是豆瓣想看里见过的条目：建过订阅的不再建（用户删掉订阅也不会再建回来），
// 没找到对应 TMDB 条目的也记下，不每次都去搜。
type MediaDoubanItem struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	SourceID       uint      `gorm:"not null;uniqueIndex:idx_media_douban_item" json:"source_id"`
	DoubanID       string    `gorm:"column:douban_id;size:16;not null;uniqueIndex:idx_media_douban_item" json:"douban_id"`
	Title          string    `gorm:"size:255;not null;default:''" json:"title"`
	Year           int       `gorm:"not null;default:0" json:"year"`
	MediaType      string    `gorm:"size:8;not null;default:''" json:"media_type"`
	TMDBID         int       `gorm:"column:tmdb_id;not null;default:0" json:"tmdb_id"`
	SubscriptionID uint      `gorm:"not null;default:0" json:"subscription_id"`
	Status         string    `gorm:"size:16;not null;default:''" json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (MediaDoubanItem) TableName() string { return "media_douban_items" }
