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
	// MediaSubFromApp 是经 App API（M12）建的订阅
	MediaSubFromApp = "app"
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

// MediaAdoptedSitePrefix 是追剧认领的下载器里的种子的站点名前缀（后面是下载器编号）。
const MediaAdoptedSitePrefix = "downloader:"

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
	Upgrade  bool   `gorm:"not null;default:false" json:"upgrade"`
	Source   string `gorm:"size:16;not null;default:''" json:"source"`
	DoubanID string `gorm:"column:douban_id;size:16;not null;default:''" json:"douban_id"`
	// TotalEpisodes 是这一季的集数（来自 TMDB），电影为 0
	TotalEpisodes int        `gorm:"not null;default:0" json:"total_episodes"`
	LastSearchAt  *time.Time `json:"last_search_at,omitempty"`
	NextSearchAt  *time.Time `gorm:"index" json:"next_search_at,omitempty"`
	// Message 是最近一次搜索或下载的结果说明
	Message string `gorm:"size:1024;not null;default:''" json:"message"`

	// 以下是追剧（剧集的一季）的设置，默认值让升级以前的订阅保持原来的行为。

	// ScheduleKind 是主动搜索的定时方式（MediaSchedule*）；空串沿用设置里的全局间隔
	ScheduleKind string `gorm:"size:16;not null;default:''" json:"-"`
	// ScheduleAt 是时刻（HH:MM）；按播出时间时是当地的播出时刻（TMDB 只有日期），空串按 20:00
	ScheduleAt string `gorm:"size:5;not null;default:''" json:"-"`
	// ScheduleEvery 是每隔几天（1–30）
	ScheduleEvery int `gorm:"not null;default:0" json:"-"`
	// ScheduleDays 是每周的星期几（0–6，0 是星期日）或每月的几号（1–31，短月按月末），JSON 数组
	ScheduleDays string `gorm:"type:text;not null;default:''" json:"-"`
	// AirDelayHours、AirRetryHours、AirWindowDays 用在按播出时间：播出后多久开始搜（0 = 2 小时）、
	// 没找到时隔多久再搜（0 = 6 小时）、播出后几天里集中重试（0 = 7 天）
	AirDelayHours int `gorm:"not null;default:0" json:"-"`
	AirRetryHours int `gorm:"not null;default:0" json:"-"`
	AirWindowDays int `gorm:"not null;default:0" json:"-"`
	// StartEpisode 是从第几集开始追（0 与 1 都是整季）
	StartEpisode int `gorm:"not null;default:0" json:"start_episode"`
	// TotalEpisodesManual 为真时 TotalEpisodes 以用户填的为准，不再按 TMDB 改
	TotalEpisodesManual bool `gorm:"not null;default:false" json:"total_episodes_manual"`
	// LockVersion 是版本锁：开着时只要和已有版本同一个制作组、同一个分辨率的；LockGroup、LockResolution 是锁住的版本
	LockVersion    bool   `gorm:"not null;default:false" json:"lock_version"`
	LockGroup      string `gorm:"size:64;not null;default:''" json:"lock_group"`
	LockResolution string `gorm:"size:16;not null;default:''" json:"lock_resolution"`
	// FollowNextSeason 为真时这一季完成以后自动给下一季建追剧；NextSeasonID 是已经建好的那个
	FollowNextSeason bool `gorm:"not null;default:false" json:"follow_next_season"`
	NextSeasonID     uint `gorm:"not null;default:0" json:"next_season_id"`
	// LibraryID 是整理进哪个媒体库（0 按类型自动选）
	LibraryID uint `gorm:"not null;default:0" json:"library_id"`
	// CompletedAt 是变成完成的时间
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	// LastRunID 是最近一次运行记录
	LastRunID uint `gorm:"not null;default:0" json:"last_run_id"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// 追剧主动搜索的定时方式（MediaSubscription.ScheduleKind）。空串是沿用设置里的全局间隔。
const (
	MediaScheduleAirtime   = "airtime"
	MediaScheduleDaily     = "daily"
	MediaScheduleEveryDays = "every_days"
	MediaScheduleWeekly    = "weekly"
	MediaScheduleMonthly   = "monthly"
)

// 追剧台账里一集的状态。
const (
	MediaEpisodeUpcoming = "upcoming"
	MediaEpisodeMissing  = "missing"
	// MediaEpisodeWaitingVersion 是播出很久了还没有锁住的那个版本（仍算缺的）
	MediaEpisodeWaitingVersion = "waiting_version"
	MediaEpisodeDownloading    = "downloading"
	MediaEpisodeLibrary        = "library"
	MediaEpisodeSkipped        = "skipped"
)

// 台账里一集的状态从哪里来的。
const (
	MediaEpisodeFromLibrary      = "library"
	MediaEpisodeFromSubscription = "subscription"
	MediaEpisodeFromDownloader   = "downloader"
)

// MediaSubscriptionEpisode 是追剧台账里的一集：由入库记录、订阅的种子与下载器里的种子合成，只由重建台账写；
// ManualSkip（不要这一集）与 ManualUnlock（这一集不受版本锁限制）只由用户的操作写，重建时保留。
type MediaSubscriptionEpisode struct {
	ID             uint   `gorm:"primaryKey" json:"-"`
	SubscriptionID uint   `gorm:"not null;uniqueIndex:idx_media_sub_episode" json:"subscription_id"`
	Episode        int    `gorm:"not null;uniqueIndex:idx_media_sub_episode" json:"episode"`
	AirDate        string `gorm:"size:10;not null;default:''" json:"air_date"`
	Name           string `gorm:"size:255;not null;default:''" json:"name"`
	State          string `gorm:"size:16;not null;default:''" json:"state"`
	Source         string `gorm:"size:16;not null;default:''" json:"source"`
	// Ref 是状态的出处：整理记录编号、订阅种子记录编号
	Ref               string `gorm:"size:64;not null;default:''" json:"ref"`
	VersionGroup      string `gorm:"size:64;not null;default:''" json:"version_group"`
	VersionResolution string `gorm:"size:16;not null;default:''" json:"version_resolution"`
	ManualSkip        bool   `gorm:"not null;default:false" json:"manual_skip"`
	ManualUnlock      bool   `gorm:"not null;default:false" json:"manual_unlock"`
	// WaitingSince 是开始等锁住的版本的时间；WaitingNotifiedAt 是最近一次提醒的时间
	WaitingSince      *time.Time `json:"waiting_since,omitempty"`
	WaitingNotifiedAt *time.Time `json:"-"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func (MediaSubscriptionEpisode) TableName() string { return "media_subscription_episodes" }

// 集号规则（MediaEpisodeRule.Mode）：种子标题里的集号怎么对到 TMDB 的集。
const (
	// MediaEpisodeRuleTMDB 是标题里的 E{n} 就是 TMDB 的第 n 集
	MediaEpisodeRuleTMDB = "tmdb"
	// MediaEpisodeRulePeriodPart 是标题里的 E{n}（与 Part）是第 n 期（上、下），按 TMDB 的集名找
	MediaEpisodeRulePeriodPart = "period_part"
	// MediaEpisodeRuleDate 是按标题里的日期找那天播出的集
	MediaEpisodeRuleDate = "date"
	// MediaEpisodeRuleOffset 是 TMDB 的集号 = 标题里的集号 + Offset
	MediaEpisodeRuleOffset = "offset"
)

// 集号规则的来源。
const (
	MediaRuleFromAuto   = "auto"
	MediaRuleFromManual = "manual"
	MediaRuleFromAI     = "ai"
)

// MediaEpisodeRule 是一个追剧对一个制作组（空 = 所有组）的集号规则。
type MediaEpisodeRule struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	SubscriptionID uint      `gorm:"not null;uniqueIndex:idx_media_episode_rule" json:"subscription_id"`
	Group          string    `gorm:"column:group_name;size:64;not null;default:'';uniqueIndex:idx_media_episode_rule" json:"group"`
	Mode           string    `gorm:"size:16;not null;default:''" json:"mode"`
	Offset         int       `gorm:"not null;default:0" json:"offset"`
	Source         string    `gorm:"size:16;not null;default:''" json:"source"`
	Note           string    `gorm:"size:255;not null;default:''" json:"note"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (MediaEpisodeRule) TableName() string { return "media_episode_rules" }

// 追剧运行是怎么开始的。
const (
	MediaRunCreate   = "create"
	MediaRunSchedule = "schedule"
	MediaRunRSS      = "rss"
	MediaRunManual   = "manual"
	MediaRunAI       = "ai"
)

// 追剧运行的结果。
const (
	MediaRunOK      = "ok"
	MediaRunNothing = "nothing"
	MediaRunWaiting = "waiting"
	MediaRunFailed  = "failed"
	MediaRunPartial = "partial"
	// MediaRunChanged 是手动改动的记录（改设置、跳过一集、换版本、集号规则），不是一次搜索
	MediaRunChanged = "changed"
)

// MediaSubscriptionRun 是追剧（与订阅）的一次运行：一次主动搜索、RSS 命中、建立或手动搜索。
// 每个订阅保留最近 100 条，且不超过 90 天。
type MediaSubscriptionRun struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	SubscriptionID uint       `gorm:"not null;index" json:"subscription_id"`
	Trigger        string     `gorm:"size:16;not null;default:''" json:"trigger"`
	StartedAt      time.Time  `gorm:"index" json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	Status         string     `gorm:"size:16;not null;default:''" json:"status"`
	Summary        string     `gorm:"size:1024;not null;default:''" json:"summary"`
	// Steps 是这次运行的步骤（JSON 数组，最多 16 KiB；超出时截断，StepsTruncated 为真）
	Steps          string `gorm:"type:text;not null;default:''" json:"-"`
	StepsTruncated bool   `gorm:"not null;default:false" json:"steps_truncated"`
}

func (MediaSubscriptionRun) TableName() string { return "media_subscription_runs" }

func (MediaSubscription) TableName() string { return "media_subscriptions" }

// MediaSubscriptionTorrent 是订阅下载的一个种子：同一个订阅里同一个站点的同一个种子只记一次。
// 追剧认领的下载器里别处加的种子没有站点与种子编号：SiteName 是 MediaAdoptedSitePrefix 加下载器编号，TorrentID 是 info hash。
type MediaSubscriptionTorrent struct {
	ID             uint   `gorm:"primaryKey" json:"id"`
	SubscriptionID uint   `gorm:"not null;uniqueIndex:idx_media_sub_torrent" json:"subscription_id"`
	SiteName       string `gorm:"size:64;not null;uniqueIndex:idx_media_sub_torrent" json:"site_name"`
	TorrentID      string `gorm:"size:128;not null;uniqueIndex:idx_media_sub_torrent" json:"torrent_id"`
	InfoHash       string `gorm:"size:64;not null;default:'';index" json:"info_hash"`
	Title          string `gorm:"size:512;not null;default:''" json:"title"`
	// Subtitle 是站点上的副标题：洗版时按现在的档案重算分数要用（中字写在副标题里）
	Subtitle string `gorm:"size:512;not null;default:''" json:"subtitle"`
	// Score 是推送时按当时的档案打的分（档案改过以后比较版本时重算，不用这个）
	Score int `gorm:"not null;default:0" json:"score"`
	// Episode、EpisodeEnd 是种子里的集（剧集，单集时 EpisodeEnd 为 0）；整季包 Complete 为真
	Episode    int  `gorm:"not null;default:0" json:"episode"`
	EpisodeEnd int  `gorm:"not null;default:0" json:"episode_end"`
	Complete   bool `gorm:"not null;default:false" json:"complete"`
	// CoveredEpisodes 是整季包推送以后按种子的文件列表核对出的集（JSON 数组）；为空时按 Episode、EpisodeEnd、Complete 算
	CoveredEpisodes string `gorm:"type:text;not null;default:''" json:"-"`
	SizeBytes       int64  `gorm:"not null;default:0" json:"size_bytes"`
	DownloaderID    uint   `gorm:"not null;default:0" json:"downloader_id"`
	// HasHR 是推送时知道的 H&R 要求（站点整站有，或者这个种子有）：洗版删旧种子时留着做种
	HasHR bool `gorm:"not null;default:false" json:"has_hr"`
	// Adopted 是推送时下载器里已经有这个种子（不是订阅加进去的）：洗版时不删
	Adopted   bool      `gorm:"not null;default:false" json:"adopted"`
	Status    string    `gorm:"size:16;not null;default:'';index" json:"status"`
	Message   string    `gorm:"size:1024;not null;default:''" json:"message"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
	// MediaDoubanRemoved 是建的订阅被用户删掉了（不会再建回来）
	MediaDoubanRemoved = "removed"
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
