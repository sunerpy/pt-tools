package models

import "time"

// Servarr 兼容入口（媒体中心 M10）的标签：Seerr 给请求打的标签（例如「1-admin」）。只用来回给 Seerr，不影响订阅。

// 标签挂在哪种条目上。
const (
	// ServarrKindMovie 的 ExternalID 是电影订阅的编号（Radarr 的 movie id）
	ServarrKindMovie = "movie"
	// ServarrKindSeries 的 ExternalID 是剧集的 TMDB 编号（Sonarr 的 series id）
	ServarrKindSeries = "series"
)

// ServarrTag 是一个标签（Radarr 与 Sonarr 共用编号）。
type ServarrTag struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Label     string    `gorm:"size:128;not null;default:'';uniqueIndex" json:"label"`
	CreatedAt time.Time `json:"created_at"`
}

func (ServarrTag) TableName() string { return "servarr_tags" }

// ServarrTagLink 是一个条目挂着的一个标签。
type ServarrTagLink struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	Kind       string `gorm:"size:8;not null;default:'';uniqueIndex:idx_servarr_tag_link" json:"kind"`
	ExternalID uint   `gorm:"not null;default:0;uniqueIndex:idx_servarr_tag_link" json:"external_id"`
	TagID      uint   `gorm:"not null;default:0;uniqueIndex:idx_servarr_tag_link;index" json:"tag_id"`
}

func (ServarrTagLink) TableName() string { return "servarr_tag_links" }
