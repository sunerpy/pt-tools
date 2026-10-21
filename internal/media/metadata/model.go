// Package metadata 是刮削用的元数据：与来源无关的电影、剧集、季、集与图片，各个来源（TMDB、豆瓣、Bangumi、
// Fanart.tv、OMDb）的适配，以及按媒体库的设置把几个来源的数据合在一起（每一项记下来自哪个来源）。
// 识别与整理仍以 TMDB 的条目为准：这里只决定 NFO 与图片里写什么。
package metadata

import (
	"strings"
)

// 来源。
const (
	SourceTMDB    = "tmdb"
	SourceDouban  = "douban"
	SourceBangumi = "bangumi"
	SourceFanart  = "fanart"
	SourceOMDb    = "omdb"
)

// SourceName 是来源给人看的名字。
func SourceName(id string) string {
	switch id {
	case SourceTMDB:
		return "TMDB"
	case SourceDouban:
		return "豆瓣"
	case SourceBangumi:
		return "Bangumi"
	case SourceFanart:
		return "Fanart.tv"
	case SourceOMDb:
		return "OMDb"
	}
	return id
}

// 条目的种类（和 tmdb.KindMovie、tmdb.KindTV 一样）。
const (
	KindMovie = "movie"
	KindTV    = "tv"
)

// 编号的种类：NFO 里 <uniqueid type="…"> 的 type。
const (
	IDTMDB    = "tmdb"
	IDIMDb    = "imdb"
	IDTVDB    = "tvdb"
	IDDouban  = "douban"
	IDBangumi = "bangumi"
)

// 评分的名称：NFO 里 <rating name="…"> 的 name（Kodi 的写法）。Jellyfin 把名字里带 tomato 的当成影评人评分，
// 其余的都当成大众评分、按顺序覆盖，最后一个生效（见 Item.Ratings）。
const (
	RatingTMDB           = "themoviedb"
	RatingIMDb           = "imdb"
	RatingDouban         = "douban"
	RatingBangumi        = "bangumi"
	RatingRottenTomatoes = "tomatometerallcritics"
	RatingMetacritic     = "metacritic"
)

// Rating 是一个来源的评分。
type Rating struct {
	Name  string
	Value float64
	// Max 是满分：10，烂番茄与 Metacritic 是 100
	Max   int
	Votes int
}

// Person 是一位演员：名字、饰演的角色、排第几、头像地址（完整的 URL，没有时为空）。
type Person struct {
	Name  string
	Role  string
	Order int
	Thumb string
}

// Collection 是电影所属的系列（TMDB 的 collection）。
type Collection struct {
	TMDBID int
	Name   string
}

// 剧集的状态（Kodi 与 Jellyfin 认的写法）。
const (
	StatusContinuing = "Continuing"
	StatusEnded      = "Ended"
)

// Item 是一部电影或一部剧集。
type Item struct {
	Kind          string
	IDs           map[string]string
	Title         string
	OriginalTitle string
	Year          int
	// Premiered 是上映或首播日期（yyyy-mm-dd）
	Premiered string
	Plot      string
	Tagline   string
	// Runtime 是片长（分钟）：电影是整部，剧集是一集
	Runtime   int
	Genres    []string
	Countries []string
	// Studios 是出品公司（剧集是播出的电视网）
	Studios []string
	// Certification 是分级（例如 PG-13、TV-MA）
	Certification string
	// Status 是剧集的状态（StatusContinuing、StatusEnded），电影为空
	Status string
	// Ratings 里主要的评分排在最后：Jellyfin 与 Emby 按顺序读，最后一个生效
	Ratings    []Rating
	Directors  []string
	Writers    []string
	Cast       []Person
	Collection *Collection
}

// Season 是剧集的一季。
type Season struct {
	Number    int
	Title     string
	Plot      string
	Premiered string
	IDs       map[string]string
	Episodes  []Episode
	// From 是每一项（plot）来自哪个来源；没有记的是 TMDB 的
	From map[string]string
}

// Episode 返回第 n 集；没有时返回 nil。
func (s *Season) Episode(n int) *Episode {
	if s == nil {
		return nil
	}
	for i := range s.Episodes {
		if s.Episodes[i].Number == n {
			return &s.Episodes[i]
		}
	}
	return nil
}

// Episode 是剧集的一集。
type Episode struct {
	Season    int
	Number    int
	IDs       map[string]string
	Title     string
	Plot      string
	Aired     string
	Runtime   int
	Ratings   []Rating
	Directors []string
	Writers   []string
	// From 是每一项（title、plot）来自哪个来源；没有记的是 TMDB 的
	From map[string]string
}

// 图片的种类。
const (
	ArtPoster    = "poster"
	ArtFanart    = "fanart"
	ArtLogo      = "logo"
	ArtClearart  = "clearart"
	ArtBanner    = "banner"
	ArtLandscape = "landscape"
	ArtDisc      = "discart"
	// ArtSeasonPoster 是季海报（Season 是季号）
	ArtSeasonPoster = "season_poster"
	// ArtThumb 是一集的截图（Season、Episode 是季号与集号）
	ArtThumb = "thumb"
)

// Artwork 是一个来源的一张图片。Lang 是图上文字的语言（ISO 639-1），空串是没有文字。
type Artwork struct {
	Source  string
	Kind    string
	Season  int
	Episode int
	Lang    string
	// Likes 是评分或点赞数，同一来源同一种图片里按它从高到低排
	Likes float64
	// Default 表示这是来源自己按识别语言挑的那张（TMDB 详情里的 poster_path、backdrop_path，季详情里的海报，集的截图）
	Default bool
	// Ref 是来源下载这张图用的引用（TMDB 是 /xxx.jpg 这样的路径，Fanart.tv 是图片地址）
	Ref string
}

// ID 返回条目的一种编号；没有时为空串。
func (it *Item) ID(kind string) string {
	if it == nil || it.IDs == nil {
		return ""
	}
	return it.IDs[kind]
}

// setID 记下一种编号（空的不记）。
func setID(ids map[string]string, kind, v string) map[string]string {
	if v = strings.TrimSpace(v); v == "" {
		return ids
	}
	if ids == nil {
		ids = map[string]string{}
	}
	ids[kind] = v
	return ids
}

// mergeIDs 把 add 里有、ids 里没有的编号补进 ids。
func mergeIDs(ids, add map[string]string) map[string]string {
	for k, v := range add {
		if _, ok := ids[k]; !ok {
			ids = setID(ids, k, v)
		}
	}
	return ids
}
