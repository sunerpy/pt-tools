// Package v2 provides a generic site architecture using the Driver pattern.
// This package implements type-safe site drivers for different PT site architectures
// including NexusPHP, Unit3D, Gazelle, and mTorrent (M-Team).
package v2

import (
	"context"
	"errors"
	"time"

	"github.com/sunerpy/pt-tools/utils"
)

// CSTLocation is the China Standard Time timezone (UTC+8).
// Re-exported from utils for convenience within this package.
var CSTLocation = utils.CSTLocation

// ParseTimeInCST parses a time string in CST timezone.
// Re-exported from utils for convenience within this package.
func ParseTimeInCST(layout, value string) (time.Time, error) {
	return utils.ParseTimeInCST(layout, value)
}

// Common errors for site operations
var (
	ErrSiteNotFound       = errors.New("site not found")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrSessionExpired     = errors.New("session expired")
	ErrAuthFailed         = errors.New("authentication failed: please check cookie or 2FA settings")
	Err2FARequired        = ErrAuthFailed // Alias for backward compatibility
	ErrRateLimited        = errors.New("rate limited")
	ErrParseError         = errors.New("failed to parse response")
	ErrNetworkError       = errors.New("network error")
	ErrNotImplemented     = errors.New("not implemented")
)

// SiteKind represents the type of PT site architecture
type SiteKind string

const (
	// SiteNexusPHP represents NexusPHP-based sites (e.g., HDSky, CHDBits)
	SiteNexusPHP SiteKind = "nexusphp"
	// SiteUnit3D represents Unit3D-based sites
	SiteUnit3D SiteKind = "unit3d"
	// SiteGazelle represents Gazelle-based sites (e.g., What.CD clones)
	SiteGazelle SiteKind = "gazelle"
	// SiteMTorrent represents M-Team's custom API
	SiteMTorrent SiteKind = "mtorrent"
	// SiteHDDolby represents HDDolby's REST API
	SiteHDDolby SiteKind = "hddolby"
	// SiteRousi represents Rousi-based sites using passkey auth
	SiteRousi SiteKind = "rousi"
)

// AuthMethod represents the authentication method for a site
type AuthMethod string

const (
	// AuthMethodCookie uses browser cookie for authentication
	AuthMethodCookie AuthMethod = "cookie"
	// AuthMethodAPIKey uses API key for authentication
	AuthMethodAPIKey AuthMethod = "api_key"
	// AuthMethodCookieAndAPIKey uses both cookie and API key
	AuthMethodCookieAndAPIKey AuthMethod = "cookie_and_api_key"
	// AuthMethodPasskey uses passkey for RSS/download authentication
	AuthMethodPasskey AuthMethod = "passkey"
)

// IsValid checks if the auth method is a known valid value
func (a AuthMethod) IsValid() bool {
	switch a {
	case AuthMethodCookie, AuthMethodAPIKey, AuthMethodCookieAndAPIKey, AuthMethodPasskey:
		return true
	default:
		return false
	}
}

// String returns the string representation of the auth method
func (a AuthMethod) String() string {
	return string(a)
}

// Schema represents the site architecture/schema type
type Schema string

const (
	// SchemaNexusPHP is the NexusPHP architecture (most common for Chinese PT sites)
	SchemaNexusPHP Schema = "NexusPHP"
	// SchemaMTorrent is M-Team's custom API architecture
	SchemaMTorrent Schema = "mTorrent"
	// SchemaUnit3D is the Unit3D architecture
	SchemaUnit3D Schema = "Unit3D"
	// SchemaGazelle is the Gazelle architecture
	SchemaGazelle Schema = "Gazelle"
	// SchemaHDDolby is HDDolby's custom REST API architecture
	SchemaHDDolby Schema = "HDDolby"
	// SchemaRousi is RousiPro's custom architecture
	SchemaRousi Schema = "Rousi"
)

// IsValid checks if the schema is a known valid value
func (s Schema) IsValid() bool {
	switch s {
	case SchemaNexusPHP, SchemaMTorrent, SchemaUnit3D, SchemaGazelle, SchemaHDDolby, SchemaRousi:
		return true
	default:
		return false
	}
}

// String returns the string representation of the schema
func (s Schema) String() string {
	return string(s)
}

// ToSiteKind converts a Schema to corresponding SiteKind
func (s Schema) ToSiteKind() SiteKind {
	switch s {
	case SchemaNexusPHP:
		return SiteNexusPHP
	case SchemaMTorrent:
		return SiteMTorrent
	case SchemaUnit3D:
		return SiteUnit3D
	case SchemaGazelle:
		return SiteGazelle
	case SchemaHDDolby:
		return SiteHDDolby
	case SchemaRousi:
		return SiteRousi
	default:
		return SiteNexusPHP // Default fallback
	}
}

// DefaultAuthMethod returns the default auth method for this schema
func (s Schema) DefaultAuthMethod() AuthMethod {
	switch s {
	case SchemaMTorrent, SchemaUnit3D:
		return AuthMethodAPIKey
	case SchemaRousi:
		return AuthMethodPasskey
	default:
		return AuthMethodCookie
	}
}

// DiscountLevel represents the discount level of a torrent
type DiscountLevel string

const (
	// DiscountNone represents no discount (normal download/upload counting)
	DiscountNone DiscountLevel = "NONE"
	// DiscountFree represents free download (0% download counting)
	DiscountFree DiscountLevel = "FREE"
	// Discount2xFree represents 2x free (0% download, 2x upload counting)
	Discount2xFree DiscountLevel = "2XFREE"
	// DiscountPercent50 represents 50% download counting
	DiscountPercent50 DiscountLevel = "PERCENT_50"
	// DiscountPercent30 represents 30% download counting
	DiscountPercent30 DiscountLevel = "PERCENT_30"
	// DiscountPercent70 represents 70% download counting
	DiscountPercent70 DiscountLevel = "PERCENT_70"
	// Discount2xUp represents 2x upload counting
	Discount2xUp DiscountLevel = "2XUP"
	// Discount2x50 represents 2x upload and 50% download counting
	Discount2x50 DiscountLevel = "2X50"
)

// FreeDiscountLevels contains all discount levels that result in free download
var FreeDiscountLevels = []DiscountLevel{
	DiscountFree,
	Discount2xFree,
}

// IsFreeTorrent checks if the given discount level results in free download
func IsFreeTorrent(level DiscountLevel) bool {
	for _, freeLevel := range FreeDiscountLevels {
		if level == freeLevel {
			return true
		}
	}
	return false
}

// GetDownloadRatio returns the download counting ratio (0.0 = free, 1.0 = normal)
func (d DiscountLevel) GetDownloadRatio() float64 {
	switch d {
	case DiscountFree, Discount2xFree:
		return 0.0
	case DiscountPercent30:
		return 0.3
	case DiscountPercent50, Discount2x50:
		return 0.5
	case DiscountPercent70:
		return 0.7
	default:
		return 1.0
	}
}

// GetUploadRatio returns the upload counting ratio (1.0 = normal, 2.0 = double)
func (d DiscountLevel) GetUploadRatio() float64 {
	switch d {
	case Discount2xFree, Discount2xUp, Discount2x50:
		return 2.0
	default:
		return 1.0
	}
}

// Credentials holds authentication information for a site
type Credentials struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Cookie   string `json:"cookie,omitempty"`
	APIKey   string `json:"apiKey,omitempty"`
}

// SearchQuery represents a search request to a PT site
type SearchQuery struct {
	// Keyword is the search term
	Keyword string `json:"keyword"`
	// Category filters by torrent category
	Category string `json:"category,omitempty"`
	// FreeOnly filters to only show free torrents
	FreeOnly bool `json:"freeOnly,omitempty"`
	// Page is the page number (1-indexed)
	Page int `json:"page,omitempty"`
	// PageSize is the number of results per page
	PageSize int `json:"pageSize,omitempty"`
	// SortBy specifies the sort field
	SortBy string `json:"sortBy,omitempty"`
	// OrderDesc specifies descending order when true
	OrderDesc bool `json:"orderDesc,omitempty"`
}

// Validate validates the search query
func (q *SearchQuery) Validate() error {
	if q.Page < 0 {
		return errors.New("page must be non-negative")
	}
	if q.PageSize < 0 {
		return errors.New("pageSize must be non-negative")
	}
	return nil
}

// TorrentItem represents a torrent search result
type TorrentItem struct {
	// ID is the site-specific torrent identifier
	ID string `json:"id"`
	// URL is the torrent detail page URL
	URL string `json:"url,omitempty"`
	// Title is the torrent title
	Title string `json:"title"`
	// Subtitle is the torrent subtitle (副标题)
	Subtitle string `json:"subtitle,omitempty"`
	// InfoHash is the torrent info hash (if available)
	InfoHash string `json:"infoHash,omitempty"`
	// Magnet is the magnet link (if available)
	Magnet string `json:"magnet,omitempty"`
	// SizeBytes is the torrent size in bytes
	SizeBytes int64 `json:"sizeBytes"`
	// Seeders is the number of seeders
	Seeders int `json:"seeders"`
	// Leechers is the number of leechers
	Leechers int `json:"leechers"`
	// Snatched is the number of completed downloads
	Snatched int `json:"snatched,omitempty"`
	// UploadedAt is the upload timestamp (Unix seconds)
	UploadedAt int64 `json:"uploadedAt,omitempty"`
	// Tags are the torrent tags/labels
	Tags []string `json:"tags,omitempty"`
	// SourceSite is the site this torrent came from
	SourceSite string `json:"sourceSite"`
	// DiscountLevel is the current discount level
	DiscountLevel DiscountLevel `json:"discountLevel"`
	// DiscountEndTime is when the discount expires
	DiscountEndTime time.Time `json:"discountEndTime,omitempty"`
	// HasHR indicates if the torrent has H&R requirements
	HasHR bool `json:"hasHR,omitempty"`
	// DownloadURL is the direct download URL
	DownloadURL string `json:"downloadUrl,omitempty"`
	// Category is the torrent category
	Category string `json:"category,omitempty"`
}

// IsFree returns true if the torrent is currently free
func (t *TorrentItem) IsFree() bool {
	return IsFreeTorrent(t.DiscountLevel)
}

// IsDiscountActive returns true if the discount is still active
func (t *TorrentItem) IsDiscountActive() bool {
	if t.DiscountLevel == DiscountNone {
		return false
	}
	if t.DiscountEndTime.IsZero() {
		return true // No end time means permanent discount
	}
	return time.Now().Before(t.DiscountEndTime)
}

// CanbeFinished checks if the torrent can be downloaded within the free period
// enabled: whether download limit is enabled
// speedLimit: download speed limit in MB/s
// sizeLimitGB: maximum torrent size in GB
func (t *TorrentItem) CanbeFinished(enabled bool, speedLimit, sizeLimitGB int) bool {
	sizeMB := float64(t.SizeBytes) / 1024 / 1024
	if sizeLimitGB > 0 && sizeMB >= float64(sizeLimitGB*1024) {
		return false
	}
	if enabled && speedLimit > 0 {
		if t.DiscountEndTime.IsZero() {
			return true
		}
		duration := time.Until(t.DiscountEndTime)
		if duration <= 0 {
			return false
		}
		canDownloadMB := duration.Seconds() * float64(speedLimit)
		return canDownloadMB >= sizeMB
	}
	return true
}

// GetFreeEndTime returns the discount end time
func (t *TorrentItem) GetFreeEndTime() *time.Time {
	if t.DiscountEndTime.IsZero() {
		return nil
	}
	return &t.DiscountEndTime
}

// GetFreeLevel returns the discount level as string
func (t *TorrentItem) GetFreeLevel() string {
	return string(t.DiscountLevel)
}

// GetName returns the torrent title
func (t *TorrentItem) GetName() string {
	return t.Title
}

// GetSubTitle returns the torrent tags as subtitle
func (t *TorrentItem) GetSubTitle() string {
	if len(t.Tags) == 0 {
		return ""
	}
	result := ""
	for i, tag := range t.Tags {
		if i > 0 {
			result += " "
		}
		result += tag
	}
	return result
}

// UserInfo represents user information from a PT site
type UserInfo struct {
	// Site is the site identifier
	Site string `json:"site"`
	// Username is the user's username
	Username string `json:"username"`
	// UserID is the site-specific user ID
	UserID string `json:"userId"`
	// Uploaded is the total uploaded bytes
	Uploaded int64 `json:"uploaded"`
	// Downloaded is the total downloaded bytes
	Downloaded int64 `json:"downloaded"`
	// Ratio is the upload/download ratio
	Ratio float64 `json:"ratio"`
	// Bonus is the bonus points
	Bonus float64 `json:"bonus"`
	// Seeding is the number of torrents being seeded
	Seeding int `json:"seeding"`
	// Leeching is the number of torrents being downloaded
	Leeching int `json:"leeching"`
	// Rank is the user's rank/class
	Rank string `json:"rank"`
	// JoinDate is when the user joined (Unix seconds)
	JoinDate int64 `json:"joinDate,omitempty"`
	// LastAccess is the last access time (Unix seconds)
	LastAccess int64 `json:"lastAccess,omitempty"`
	// LastUpdate is when this info was last updated (Unix seconds)
	LastUpdate int64 `json:"lastUpdate"`
	// NextLevel contains level progression info (optional)
	NextLevel *LevelProgress `json:"nextLevel,omitempty"`

	// Extended fields (populated by sites that support them)
	// LevelName is the user's level/rank name
	LevelName string `json:"levelName,omitempty"`
	// LevelID is the numeric level ID
	LevelID int `json:"levelId,omitempty"`
	// BonusPerHour is the bonus points earned per hour (时魔)
	BonusPerHour float64 `json:"bonusPerHour,omitempty"`
	// SeedingBonus is the seeding bonus points (做种积分)
	SeedingBonus float64 `json:"seedingBonus,omitempty"`
	// SeedingBonusPerHour is the seeding bonus per hour
	SeedingBonusPerHour float64 `json:"seedingBonusPerHour,omitempty"`
	// UnreadMessageCount is the number of unread messages
	UnreadMessageCount int `json:"unreadMessageCount,omitempty"`
	// TotalMessageCount is the total number of messages
	TotalMessageCount int `json:"totalMessageCount,omitempty"`
	// SeederCount is the number of torrents being seeded (from peer statistics)
	SeederCount int `json:"seederCount,omitempty"`
	// SeederSize is the total size of seeding torrents (bytes)
	SeederSize int64 `json:"seederSize,omitempty"`
	// LeecherCount is the number of torrents being downloaded
	LeecherCount int `json:"leecherCount,omitempty"`
	// LeecherSize is the total size of leeching torrents (bytes)
	LeecherSize int64 `json:"leecherSize,omitempty"`
	// HnRUnsatisfied is the number of unsatisfied H&R
	HnRUnsatisfied int `json:"hnrUnsatisfied,omitempty"`
	// HnRPreWarning is the number of H&R pre-warnings
	HnRPreWarning int `json:"hnrPreWarning,omitempty"`
	// TrueUploaded is the true uploaded bytes (some sites track separately)
	TrueUploaded int64 `json:"trueUploaded,omitempty"`
	// TrueDownloaded is the true downloaded bytes
	TrueDownloaded int64 `json:"trueDownloaded,omitempty"`
	// Uploads is the number of torrents uploaded by user
	Uploads int `json:"uploads,omitempty"`
}

// LevelProgress represents progress towards the next user level
type LevelProgress struct {
	// CurrentLevel is the user's current level/rank
	CurrentLevel string `json:"currentLevel"`
	// NextLevel is the next level to achieve
	NextLevel string `json:"nextLevel"`
	// UploadNeeded is the additional upload needed (bytes)
	UploadNeeded int64 `json:"uploadNeeded,omitempty"`
	// DownloadNeeded is the additional download needed (bytes)
	DownloadNeeded int64 `json:"downloadNeeded,omitempty"`
	// RatioNeeded is the additional ratio needed
	RatioNeeded float64 `json:"ratioNeeded,omitempty"`
	// TimeNeeded is the additional time needed
	TimeNeeded time.Duration `json:"timeNeeded,omitempty"`
	// ProgressPercent is the overall progress percentage (0-100)
	ProgressPercent float64 `json:"progressPercent"`
}

// AggregatedStats represents aggregated statistics across all sites
type AggregatedStats struct {
	// TotalUploaded is the sum of all uploaded bytes
	TotalUploaded int64 `json:"totalUploaded"`
	// TotalDownloaded is the sum of all downloaded bytes
	TotalDownloaded int64 `json:"totalDownloaded"`
	// AverageRatio is the average ratio across all sites
	AverageRatio float64 `json:"averageRatio"`
	// TotalSeeding is the sum of all seeding torrents
	TotalSeeding int `json:"totalSeeding"`
	// TotalLeeching is the sum of all leeching torrents
	TotalLeeching int `json:"totalLeeching"`
	// TotalBonus is the sum of all bonus points
	TotalBonus float64 `json:"totalBonus"`
	// SiteCount is the number of sites included
	SiteCount int `json:"siteCount"`
	// LastUpdate is when this was last calculated (Unix seconds)
	LastUpdate int64 `json:"lastUpdate"`
	// PerSiteStats contains individual site statistics
	PerSiteStats []UserInfo `json:"perSiteStats"`
	// TotalBonusPerHour is the sum of all bonus per hour
	TotalBonusPerHour float64 `json:"totalBonusPerHour,omitempty"`
	// TotalSeedingBonus is the sum of all seeding bonus points
	TotalSeedingBonus float64 `json:"totalSeedingBonus,omitempty"`
	// TotalUnreadMessages is the sum of all unread messages
	TotalUnreadMessages int `json:"totalUnreadMessages,omitempty"`
	// TotalSeederSize is the sum of all seeding sizes
	TotalSeederSize int64 `json:"totalSeederSize,omitempty"`
	// TotalLeecherSize is the sum of all leeching sizes
	TotalLeecherSize int64 `json:"totalLeecherSize,omitempty"`
}

// PeerStatistics represents user's seeding/leeching statistics
type PeerStatistics struct {
	// SeederCount is the number of torrents being seeded
	SeederCount int `json:"seederCount"`
	// SeederSize is the total size of seeding torrents (bytes)
	SeederSize int64 `json:"seederSize"`
	// LeecherCount is the number of torrents being downloaded
	LeecherCount int `json:"leecherCount"`
	// LeecherSize is the total size of leeching torrents (bytes)
	LeecherSize int64 `json:"leecherSize"`
}

// Extended info errors
var (
	ErrBonusInfoUnavailable    = errors.New("bonus info unavailable")
	ErrMessageCountUnavailable = errors.New("message count unavailable")
	ErrPeerStatsUnavailable    = errors.New("peer statistics unavailable")
)

// Site is the core interface for interacting with a PT site
type Site interface {
	// ID returns the unique site identifier
	ID() string
	// Name returns the human-readable site name
	Name() string
	// Kind returns the site architecture type
	Kind() SiteKind
	// Login authenticates with the site
	Login(ctx context.Context, creds Credentials) error
	// Search searches for torrents
	Search(ctx context.Context, query SearchQuery) ([]TorrentItem, error)
	// GetUserInfo fetches the current user's information
	GetUserInfo(ctx context.Context) (UserInfo, error)
	// Download downloads a torrent file by ID
	Download(ctx context.Context, torrentID string) ([]byte, error)
	// Close releases any resources held by the site
	Close() error
}

// Driver defines how to interact with a specific type of site architecture.
// Req is the type of request payload (e.g., url.Values for forms, struct for JSON).
// Res is the raw response type (e.g., *goquery.Document, JSON struct).
type Driver[Req any, Res any] interface {
	// PrepareSearch converts a standard SearchQuery to site-specific request
	PrepareSearch(query SearchQuery) (Req, error)
	// Execute performs the actual network call
	Execute(ctx context.Context, req Req) (Res, error)
	// ParseSearch converts the raw response into standard TorrentItem list
	ParseSearch(res Res) ([]TorrentItem, error)
	// GetUserInfo fetches complete user information (including extended info if supported)
	// This method is responsible for making all necessary API calls
	GetUserInfo(ctx context.Context) (UserInfo, error)
	// PrepareDownload prepares request for downloading a torrent
	PrepareDownload(torrentID string) (Req, error)
	// ParseDownload extracts torrent file data from response
	ParseDownload(res Res) ([]byte, error)
}

// HashDownloader is an optional interface for sites that require a hash for download
type HashDownloader interface {
	DownloadWithHash(ctx context.Context, torrentID, hash string) ([]byte, error)
}

// TorrentDetailFetcher is an optional interface for drivers that can fetch torrent details
// from RSS item metadata. This is used by RSS processing to get discount info, size, etc.
// Drivers that implement this interface can be used with GetTorrentDetails in unified_site.go.
type TorrentDetailFetcher interface {
	// GetTorrentDetail fetches detailed torrent info given an RSS item's GUID and link.
	// The guid is typically the torrent ID, and link is the detail page URL.
	// Returns nil TorrentItem if the torrent is not found (without error).
	GetTorrentDetail(ctx context.Context, guid, link, title string) (*TorrentItem, error)
}

// DetailFetcherProvider is an optional interface for Site implementations that can provide
// a TorrentDetailFetcher. This allows unified_site.go to delegate detail fetching to drivers.
type DetailFetcherProvider interface {
	// GetDetailFetcher returns a TorrentDetailFetcher if the site supports it.
	// Returns nil if the site doesn't support detail fetching (e.g., NexusPHP uses HTML scraping).
	GetDetailFetcher() TorrentDetailFetcher
}
