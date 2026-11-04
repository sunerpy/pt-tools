package models

import "time"

// 媒体服务器索引里一项的种类。
const (
	MediaServerItemMovie   = "movie"
	MediaServerItemSeries  = "series"
	MediaServerItemEpisode = "episode"
)

// MediaServerItem 是媒体服务器索引里的一项：一部电影、一部剧，或剧的一集。由同步全量写入（见 internal/media/serverindex）。
// 集的 TMDBID、IMDbID、TVDBID 是所在剧的编号（集自己的编号用不上）；LocalPath 是按服务器的路径前缀换成的 pt-tools 里的路径，
// 换不了时为空。RemovedAt 为空表示服务器里还有；不为空是同步时发现服务器里没有了（删除标记，90 天后才删行）。
type MediaServerItem struct {
	ID           uint   `gorm:"primaryKey" json:"-"`
	ServerID     uint   `gorm:"not null;uniqueIndex:idx_media_server_item,priority:1;index:idx_media_server_item_series,priority:1" json:"server_id"`
	ItemID       string `gorm:"size:64;not null;uniqueIndex:idx_media_server_item,priority:2" json:"item_id"`
	Kind         string `gorm:"size:8;not null;default:'';index:idx_media_server_item_tmdb,priority:1" json:"kind"`
	SeriesItemID string `gorm:"size:64;not null;default:'';index:idx_media_server_item_series,priority:2" json:"series_item_id"`
	TMDBID       int    `gorm:"column:tmdb_id;not null;default:0;index:idx_media_server_item_tmdb,priority:2" json:"tmdb_id"`
	IMDbID       string `gorm:"column:imdb_id;size:16;not null;default:''" json:"imdb_id"`
	TVDBID       string `gorm:"column:tvdb_id;size:16;not null;default:''" json:"tvdb_id"`
	Season       int    `gorm:"not null;default:0" json:"season"`
	Episode      int    `gorm:"not null;default:0" json:"episode"`
	// EpisodeEnd 是一个文件里的最后一集（两集合一时）；只有一集时为 0
	EpisodeEnd int        `gorm:"not null;default:0" json:"episode_end"`
	Title      string     `gorm:"size:255;not null;default:''" json:"title"`
	Year       int        `gorm:"not null;default:0" json:"year"`
	Path       string     `gorm:"size:1024;not null;default:''" json:"path"`
	LocalPath  string     `gorm:"size:1024;not null;default:''" json:"local_path"`
	AddedAt    *time.Time `json:"added_at,omitempty"`
	// SyncedAt 是这一行最后一次按服务器的数据写入的时间（新增、变了、又出现）
	SyncedAt  time.Time  `json:"synced_at"`
	RemovedAt *time.Time `gorm:"index" json:"removed_at,omitempty"`
}

func (MediaServerItem) TableName() string { return "media_server_items" }

// 待处理问题的种类与状态。
const (
	// MediaIssueServerRemoved 是 pt-tools 整理过的一项在媒体服务器里没有了（索引同步时发现）
	MediaIssueServerRemoved = "server_removed"
	// MediaIssueLibraryMissing 是整理记录在库里的文件不见了（M7 的巡检）
	MediaIssueLibraryMissing = "library_missing"

	MediaIssueOpen      = "open"
	MediaIssueResolved  = "resolved"
	MediaIssueDismissed = "dismissed"
)

// MediaIssue 是一个待处理问题。唯一键是（种类、服务器、服务器里的那一项、整理记录），不适用的列写 0 或空串：
// server_removed 按服务器里的那一项认（HistoryID 为 0），library_missing 按整理记录认（服务器两列为 0 与空串）。
// 同一个问题重复发现不会多一行：又出现了改成 resolved，之后又发生改回 open，用户忽略的（dismissed）在问题消失以前保持。
type MediaIssue struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	Kind         string     `gorm:"size:32;not null;uniqueIndex:idx_media_issue,priority:1" json:"kind"`
	Status       string     `gorm:"size:16;not null;default:'';index" json:"status"`
	ServerID     uint       `gorm:"not null;default:0;uniqueIndex:idx_media_issue,priority:2" json:"server_id"`
	ServerItemID string     `gorm:"size:64;not null;default:'';uniqueIndex:idx_media_issue,priority:3" json:"server_item_id"`
	HistoryID    uint       `gorm:"not null;default:0;uniqueIndex:idx_media_issue,priority:4" json:"history_id"`
	MediaType    string     `gorm:"size:8;not null;default:'';index:idx_media_issue_item,priority:1" json:"media_type"`
	TMDBID       int        `gorm:"column:tmdb_id;not null;default:0;index:idx_media_issue_item,priority:2" json:"tmdb_id"`
	Season       int        `gorm:"not null;default:0" json:"season"`
	Episode      int        `gorm:"not null;default:0" json:"episode"`
	Title        string     `gorm:"size:255;not null;default:''" json:"title"`
	Detail       string     `gorm:"size:1024;not null;default:''" json:"detail"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ResolvedAt   *time.Time `json:"resolved_at,omitempty"`
}

func (MediaIssue) TableName() string { return "media_issues" }
