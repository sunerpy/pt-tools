package tmdb

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// 探索页用的列表。
const (
	// ListTrending 是本周趋势
	ListTrending = "trending"
	// ListPopular 是热门
	ListPopular = "popular"
	// ListTopRated 是高分
	ListTopRated = "top_rated"
	// ListNowPlaying、ListUpcoming 是电影的正在上映、即将上映
	ListNowPlaying = "now_playing"
	ListUpcoming   = "upcoming"
	// ListOnTheAir、ListAiringToday 是剧集的正在播出（最近一周有新的一集）、今天播出
	ListOnTheAir    = "on_the_air"
	ListAiringToday = "airing_today"
	// ListTTL 是列表的缓存时间
	ListTTL = 6 * time.Hour
	// maxListPages 是最多翻到第几页
	maxListPages = 20
)

// localZone 是服务器时区的名字：TZ 写了时区名时是它（例如 Asia/Shanghai），没写时是 Local（测试里替换）。
var localZone = func() string { return time.Local.String() }

// ListPage 是列表的一页。
type ListPage struct {
	Results    []Result `json:"results"`
	Page       int      `json:"page"`
	TotalPages int      `json:"total_pages"`
}

// listPath 是列表在 TMDB 上的路径；kind 没有这个列表时为空。
func listPath(kind, list string) string {
	switch {
	case list == ListTrending:
		return "/trending/" + kind + "/week"
	case list == ListPopular || list == ListTopRated:
		return "/" + kind + "/" + list
	case kind == KindMovie && (list == ListNowPlaying || list == ListUpcoming):
		return "/movie/" + list
	case kind == KindTV && (list == ListOnTheAir || list == ListAiringToday):
		return "/tv/" + list
	}
	return ""
}

// HasList 报告电影或剧集有没有这个列表。
func HasList(kind, list string) bool {
	return checkKind(kind) == nil && listPath(kind, list) != ""
}

// List 取电影或剧集的一个列表（见 HasList）。page 从 1 开始，最多到第 20 页。今天播出按服务器的时区算
// （TZ 写了时区名时；没写时是 TMDB 默认的时区）。
func (c *Client) List(ctx context.Context, kind, list string, page int) (*ListPage, error) {
	if err := checkKind(kind); err != nil {
		return nil, err
	}
	path := listPath(kind, list)
	if path == "" {
		return nil, fmt.Errorf("列表无效: %q", list)
	}
	q := url.Values{"language": {c.language}, "page": {strconv.Itoa(min(max(page, 1), maxListPages))}}
	if tz := localZone(); list == ListAiringToday && tz != "Local" && !strings.HasPrefix(tz, "/") {
		q.Set("timezone", tz)
	}
	return c.listPage(ctx, kind, "list:"+kind+":"+list+":"+q.Encode(), path, q)
}

// DiscoverFilter 是按条件发现的筛选：Country 是出品国家或地区（ISO 3166-1 的两个大写字母，例如 CN），Genres 是
// TMDB 的类型编号（几个用 | 隔开时是其中任意一个，例如综艺的 10764|10767）。空的不限。
type DiscoverFilter struct {
	Country string
	Genres  string
}

var (
	countryRe = regexp.MustCompile(`^[A-Z]{2}$`)
	genresRe  = regexp.MustCompile(`^\d{1,6}(\|\d{1,6}){0,9}$`)
)

// Discover 按出品国家与类型发现电影或剧集（按热度排，不含成人内容）。page 从 1 开始，最多到第 20 页。
func (c *Client) Discover(ctx context.Context, kind string, f DiscoverFilter, page int) (*ListPage, error) {
	if err := checkKind(kind); err != nil {
		return nil, err
	}
	if f.Country != "" && !countryRe.MatchString(f.Country) {
		return nil, fmt.Errorf("出品国家无效: %q", f.Country)
	}
	if f.Genres != "" && !genresRe.MatchString(f.Genres) {
		return nil, fmt.Errorf("类型无效: %q", f.Genres)
	}
	q := url.Values{
		"language": {c.language}, "page": {strconv.Itoa(min(max(page, 1), maxListPages))},
		"sort_by": {"popularity.desc"}, "include_adult": {"false"},
	}
	if kind == KindMovie {
		q.Set("include_video", "false")
	}
	if f.Country != "" {
		q.Set("with_origin_country", f.Country)
	}
	if f.Genres != "" {
		q.Set("with_genres", f.Genres)
	}
	return c.listPage(ctx, kind, "discover:"+kind+":"+q.Encode(), "/discover/"+kind, q)
}

// listPage 取一页列表（缓存 ListTTL）。
func (c *Client) listPage(ctx context.Context, kind, key, path string, q url.Values) (*ListPage, error) {
	var out ListPage
	err := c.cached(ctx, key, ListTTL, &out, func() (any, error) {
		var raw struct {
			Page       int       `json:"page"`
			TotalPages int       `json:"total_pages"`
			Results    []rawItem `json:"results"`
		}
		if err := c.get(ctx, path, q, &raw); err != nil {
			return nil, err
		}
		res := ListPage{Page: raw.Page, TotalPages: min(raw.TotalPages, maxListPages), Results: make([]Result, 0, len(raw.Results))}
		for _, r := range raw.Results {
			res.Results = append(res.Results, r.result(kind))
		}
		return res, nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
