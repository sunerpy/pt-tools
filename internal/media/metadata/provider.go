package metadata

import "context"

// Query 是要找的条目：识别出的 TMDB 条目（编号、中文名、原名、年份、上映或首播日期、是不是动画），
// 以及站点给的豆瓣编号（pt-tools 推送的电影才有）。别的来源按它找同一部。
type Query struct {
	Kind string
	// IDs 是已知的编号：tmdb、imdb，剧集取到 TMDB 详情以后还有 tvdb；站点给了豆瓣编号时有 douban
	IDs           map[string]string
	Title         string
	OriginalTitle string
	Year          int
	Premiered     string
	// Anime 表示 TMDB 类型里有「动画」：只有动画才去 Bangumi 找
	Anime bool
}

// SeasonRef 是要找的一季：季号，以及 TMDB 上这一季的首播日期（豆瓣、Bangumi 一季是一个条目，按它找同一季）。
type SeasonRef struct {
	Number    int
	Premiered string
	// Episodes 表示要这一季每一集的标题与简介（只要简介时不去取每一集）
	Episodes bool
}

// ArtRequest 是要的图片：种类、哪几季的季海报与截图、海报与 Logo 优先的语言（见 LangAuto）、识别语言（例如 zh-CN）。
type ArtRequest struct {
	Kinds     []string
	Seasons   []int
	ImageLang string
	RecogLang string
}

// Provider 是一个来源。下面几个接口按能力实现，一个来源可以实现其中几个。
type Provider interface {
	ID() string
}

// ItemSource 提供电影或整部剧集的信息（豆瓣与 Bangumi 用第 1 季的条目）。没找到时返回 nil、nil。
type ItemSource interface {
	Provider
	Item(ctx context.Context, q Query) (*Item, error)
}

// SeasonSource 提供剧集一季（含每一集）的信息。没找到时返回 nil、nil。
type SeasonSource interface {
	Provider
	// Season 取一季。找到了这一季、每一集取不到时可以同时返回这一季与错误（简介照样用）。
	Season(ctx context.Context, q Query, s SeasonRef) (*Season, error)
}

// ArtSource 提供图片。取到了一部分时可以同时返回图片与错误（例如 TMDB 的图片列表取不到，详情里的海报还在）。
type ArtSource interface {
	Provider
	Art(ctx context.Context, q Query, req ArtRequest) ([]Artwork, error)
	Download(ctx context.Context, a Artwork) ([]byte, error)
}
