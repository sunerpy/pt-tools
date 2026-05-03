package types

// CLIConfig stores the CLI configuration
type CLIConfig struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Cookie   string `json:"cookie,omitempty"`
	Expires  int64  `json:"expires,omitempty"`
}

// MultiSiteSearchRequest represents a multi-site search request
type MultiSiteSearchRequest struct {
	Keyword      string                      `json:"keyword"`
	Category     string                      `json:"category,omitempty"`
	FreeOnly     bool                        `json:"freeOnly,omitempty"`
	Sites        []string                    `json:"sites,omitempty"`
	MinSeeders   int                         `json:"minSeeders,omitempty"`
	MaxSizeBytes int64                       `json:"maxSizeBytes,omitempty"`
	MinSizeBytes int64                       `json:"minSizeBytes,omitempty"`
	Page         int                         `json:"page,omitempty"`
	PageSize     int                         `json:"pageSize,omitempty"`
	TimeoutSecs  int                         `json:"timeoutSecs,omitempty"`
	SortBy       string                      `json:"sortBy,omitempty"`
	OrderDesc    bool                        `json:"orderDesc,omitempty"`
	SiteParams   map[string]SiteSearchParams `json:"siteParams,omitempty"`
}

// SiteSearchParams represents site-specific search parameters
type SiteSearchParams struct {
	Cat        any `json:"cat,omitempty"`
	Medium     any `json:"medium,omitempty"`
	Codec      any `json:"codec,omitempty"`
	AudioCodec any `json:"audiocodec,omitempty"`
	Standard   any `json:"standard,omitempty"`
	Team       any `json:"team,omitempty"`
	Source     any `json:"source,omitempty"`
	Incldead   any `json:"incldead,omitempty"`
	Spstate    any `json:"spstate,omitempty"`
}

// MultiSiteSearchResponse represents a multi-site search response
type MultiSiteSearchResponse struct {
	Items        []TorrentItemResponse `json:"items"`
	TotalResults int                   `json:"totalResults"`
	SiteResults  map[string]int        `json:"siteResults"`
	Errors       []SearchErrorResponse `json:"errors,omitempty"`
	DurationMs   int64                 `json:"durationMs"`
}

// TorrentItemResponse represents a torrent item in API response
type TorrentItemResponse struct {
	ID              string   `json:"id"`
	URL             string   `json:"url,omitempty"`
	Title           string   `json:"title"`
	Subtitle        string   `json:"subtitle,omitempty"`
	InfoHash        string   `json:"infoHash,omitempty"`
	Magnet          string   `json:"magnet,omitempty"`
	SizeBytes       int64    `json:"sizeBytes"`
	Seeders         int      `json:"seeders"`
	Leechers        int      `json:"leechers"`
	Snatched        int      `json:"snatched,omitempty"`
	UploadedAt      int64    `json:"uploadedAt,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	SourceSite      string   `json:"sourceSite"`
	DiscountLevel   string   `json:"discountLevel"`
	DiscountEndTime int64    `json:"discountEndTime,omitempty"`
	HasHR           bool     `json:"hasHR,omitempty"`
	DownloadURL     string   `json:"downloadUrl,omitempty"`
	Category        string   `json:"category,omitempty"`
	IsFree          bool     `json:"isFree"`
}

// SearchErrorResponse represents a search error
type SearchErrorResponse struct {
	Site  string `json:"site"`
	Error string `json:"error"`
}

// TorrentPushRequest represents a torrent push request
type TorrentPushRequest struct {
	DownloadURL   string `json:"downloadUrl"`
	TorrentID     string `json:"torrentId,omitempty"`
	DownloaderIDs []uint `json:"downloaderIds"`
	SavePath      string `json:"savePath,omitempty"`
	Category      string `json:"category,omitempty"`
	Tags          string `json:"tags,omitempty"`
	AutoStart     *bool  `json:"autoStart,omitempty"`
	TorrentTitle  string `json:"torrentTitle,omitempty"`
	SourceSite    string `json:"sourceSite,omitempty"`
	SizeBytes     int64  `json:"sizeBytes,omitempty"`
}

// TorrentPushResponse represents a torrent push response
type TorrentPushResponse struct {
	Success bool                    `json:"success"`
	Results []TorrentPushResultItem `json:"results"`
	Message string                  `json:"message,omitempty"`
}

// TorrentPushResultItem represents a single downloader's push result
type TorrentPushResultItem struct {
	DownloaderID   uint   `json:"downloaderId"`
	DownloaderName string `json:"downloaderName"`
	Success        bool   `json:"success"`
	Skipped        bool   `json:"skipped,omitempty"`
	Message        string `json:"message,omitempty"`
	TorrentHash    string `json:"torrentHash,omitempty"`
}

// BatchTorrentPushRequest represents a batch push request
type BatchTorrentPushRequest struct {
	Torrents      []TorrentPushItem `json:"torrents"`
	DownloaderIDs []uint            `json:"downloaderIds"`
	SavePath      string            `json:"savePath,omitempty"`
	Category      string            `json:"category,omitempty"`
	Tags          string            `json:"tags,omitempty"`
	AutoStart     *bool             `json:"autoStart,omitempty"`
}

// TorrentPushItem represents a single torrent in a batch push
type TorrentPushItem struct {
	DownloadURL  string `json:"downloadUrl"`
	TorrentID    string `json:"torrentId,omitempty"`
	TorrentTitle string `json:"torrentTitle,omitempty"`
	SourceSite   string `json:"sourceSite,omitempty"`
	SizeBytes    int64  `json:"sizeBytes,omitempty"`
}

// BatchTorrentPushResponse represents a batch push response
type BatchTorrentPushResponse struct {
	Success      bool                         `json:"success"`
	TotalCount   int                          `json:"totalCount"`
	SuccessCount int                          `json:"successCount"`
	SkippedCount int                          `json:"skippedCount"`
	FailedCount  int                          `json:"failedCount"`
	Results      []BatchTorrentPushResultItem `json:"results"`
}

// BatchTorrentPushResultItem represents a single torrent's batch push result
type BatchTorrentPushResultItem struct {
	TorrentTitle string                  `json:"torrentTitle"`
	SourceSite   string                  `json:"sourceSite"`
	Success      bool                    `json:"success"`
	Skipped      bool                    `json:"skipped,omitempty"`
	Message      string                  `json:"message,omitempty"`
	Results      []TorrentPushResultItem `json:"results,omitempty"`
}

// DownloaderResponse represents a downloader configuration
type DownloaderResponse struct {
	ID              uint   `json:"id"`
	Name            string `json:"name"`
	Type            string `json:"type"`
	URL             string `json:"url"`
	Username        string `json:"username"`
	Enabled         bool   `json:"enabled"`
	DefaultSavePath string `json:"defaultSavePath"`
	DefaultCategory string `json:"defaultCategory"`
	DefaultTags     string `json:"defaultTags"`
	AutoStart       bool   `json:"autoStart"`
}

// DownloaderTorrentItem represents a torrent in a downloader
type DownloaderTorrentItem struct {
	ID           string  `json:"id"`
	Hash         string  `json:"hash"`
	Name         string  `json:"name"`
	State        string  `json:"state"`
	Progress     float64 `json:"progress"`
	SizeBytes    int64   `json:"sizeBytes"`
	Downloaded   int64   `json:"downloaded"`
	Uploaded     int64   `json:"uploaded"`
	Seeders      int     `json:"seeders"`
	Leechers     int     `json:"leechers"`
	DownloaderID uint    `json:"downloaderId"`
	AddedAt      int64   `json:"addedAt,omitempty"`
	CompletedAt  int64   `json:"completedAt,omitempty"`
	Category     string  `json:"category,omitempty"`
	Tags         string  `json:"tags,omitempty"`
	SavePath     string  `json:"savePath,omitempty"`
}

// DownloaderTransferStats represents transfer statistics
type DownloaderTransferStats struct {
	DownloadSpeed int64 `json:"downloadSpeed"`
	UploadSpeed   int64 `json:"uploadSpeed"`
	Downloaded    int64 `json:"downloaded"`
	Uploaded      int64 `json:"uploaded"`
	ActiveTorrents int  `json:"activeTorrents"`
}

// SiteConfigResponse represents a site configuration
type SiteConfigResponse struct {
	Enabled           *bool  `json:"enabled"`
	AuthMethod        string `json:"auth_method"`
	Cookie            string `json:"cookie"`
	APIKey            string `json:"api_key"`
	APIUrl            string `json:"api_url"`
	Passkey           string `json:"passkey"`
	URLs              []string `json:"urls,omitempty"`
	Unavailable       bool   `json:"unavailable,omitempty"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
	IsBuiltin         bool   `json:"is_builtin"`
}

// TaskResponse represents a task in the API response
type TaskResponse struct {
	ID             uint   `json:"id"`
	SiteName       string `json:"site_name"`
	Title          string `json:"title"`
	TorrentHash    string `json:"torrent_hash"`
	IsDownloaded   bool   `json:"is_downloaded"`
	IsPushed       bool   `json:"is_pushed"`
	IsExpired      bool   `json:"is_expired"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// TaskListResponse represents a paginated task list
type TaskListResponse struct {
	Items []TaskResponse `json:"items"`
	Total int64          `json:"total"`
	Page  int            `json:"page"`
	Size  int            `json:"page_size"`
}

// LogsResponse represents log entries from the API
type LogsResponse struct {
	Lines     []string `json:"lines"`
	Path      string   `json:"path"`
	Truncated bool     `json:"truncated"`
}

// VersionResponse represents version information
type VersionResponse struct {
	Version     string `json:"version"`
	BuildTime   string `json:"buildTime"`
	CommitID    string `json:"commitId"`
	Latest      string `json:"latest,omitempty"`
	HasUpdate   bool   `json:"hasUpdate,omitempty"`
	Runtime     string `json:"runtime,omitempty"`
	GoVersion   string `json:"goVersion,omitempty"`
}

// UserInfoAggregated represents aggregated user information
type UserInfoAggregated struct {
	Sites []UserInfoSite `json:"sites"`
}

// UserInfoSite represents user info for a single site
type UserInfoSite struct {
	SiteID   string  `json:"siteId"`
	SiteName string  `json:"siteName"`
	Username string  `json:"username"`
	Level    string  `json:"level"`
	Uploaded int64   `json:"uploaded"`
	Downloaded int64 `json:"downloaded"`
	Ratio    float64 `json:"ratio"`
	SeedSize int64   `json:"seedSize"`
}

// GlobalSettings represents global configuration
type GlobalSettings struct {
	DefaultIntervalMinutes int32   `json:"default_interval_minutes"`
	DownloadDir            string  `json:"download_dir"`
	DownloadLimitEnabled   bool    `json:"download_limit_enabled"`
	DownloadSpeedLimit     int     `json:"download_speed_limit"`
	TorrentSizeGB          int     `json:"torrent_size_gb"`
	MinFreeMinutes         int     `json:"min_free_minutes"`
	AutoStart              bool    `json:"auto_start"`
	CleanupEnabled         bool    `json:"cleanup_enabled"`
	CleanupIntervalMin     int     `json:"cleanup_interval_min"`
	CleanupScope           string  `json:"cleanup_scope"`
	CleanupScopeTags       string  `json:"cleanup_scope_tags"`
	CleanupRemoveData      bool    `json:"cleanup_remove_data"`
	CleanupConditionMode   string  `json:"cleanup_condition_mode"`
	CleanupMaxSeedTimeH    int     `json:"cleanup_max_seed_time_h"`
	CleanupMinRatio        float64 `json:"cleanup_min_ratio"`
	CleanupMaxInactiveH    int     `json:"cleanup_max_inactive_h"`
	CleanupSlowSeedTimeH   int     `json:"cleanup_slow_seed_time_h"`
	CleanupSlowMaxRatio    float64 `json:"cleanup_slow_max_ratio"`
	CleanupDelFreeExpired  bool    `json:"cleanup_del_free_expired"`
	CleanupDiskProtect     bool    `json:"cleanup_disk_protect"`
	CleanupMinDiskSpaceGB  float64 `json:"cleanup_min_disk_space_gb"`
	CleanupProtectDL       bool    `json:"cleanup_protect_dl"`
	CleanupProtectHR       bool    `json:"cleanup_protect_hr"`
	CleanupMinRetainH      int     `json:"cleanup_min_retain_h"`
	CleanupProtectTags     string  `json:"cleanup_protect_tags"`
	AutoDeleteOnFreeEnd    bool    `json:"auto_delete_on_free_end"`
}

// QbitSettings represents qBittorrent configuration
type QbitSettings struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// FilterRule represents a filter rule
type FilterRule struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Pattern  string `json:"pattern"`
	Enabled  bool   `json:"enabled"`
	Priority int    `json:"priority"`
	Action   string `json:"action"`
}

// SearchSite represents a site available for searching
type SearchSite struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Enabled  bool   `json:"enabled"`
	Category string `json:"category,omitempty"`
}
