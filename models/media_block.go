package models

import "time"

// MediaTorrentBlock 是拉黑的一个种子（站点上的种子编号）：订阅不再选它，按条目搜索把它标成已拉黑。
// 按条目存，不按订阅编号：删了订阅再建，拉黑仍然有效。TMDBID 为 0（MediaType 为空、Season 为 0）的对所有条目生效
// （关键字搜索里拉黑的）。同一个种子对同一个条目只有一行。
type MediaTorrentBlock struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	SiteName  string `gorm:"size:64;not null;uniqueIndex:idx_media_torrent_block" json:"site_name"`
	TorrentID string `gorm:"size:128;not null;uniqueIndex:idx_media_torrent_block" json:"torrent_id"`
	MediaType string `gorm:"size:8;not null;default:'';uniqueIndex:idx_media_torrent_block" json:"media_type"`
	TMDBID    int    `gorm:"column:tmdb_id;not null;default:0;uniqueIndex:idx_media_torrent_block;index" json:"tmdb_id"`
	// Season 是剧集的季；电影与对所有条目生效的是 0
	Season int `gorm:"not null;default:0;uniqueIndex:idx_media_torrent_block" json:"season"`
	// MediaTitle 是拉黑时条目的名字（列表里写作用于哪个条目）
	MediaTitle string `gorm:"size:255;not null;default:''" json:"media_title"`
	// Title 是拉黑时种子的标题（站点上删了以后列表里还看得出是哪个）
	Title string `gorm:"size:512;not null;default:''" json:"title"`
	// Reason 是用户写的原因，可以为空
	Reason    string    `gorm:"size:255;not null;default:''" json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

func (MediaTorrentBlock) TableName() string { return "media_torrent_blocks" }
