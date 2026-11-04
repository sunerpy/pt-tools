package subscribe

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

// 探索页的列表：TMDB 的列表（本周趋势、热门、高分，电影的正在上映与即将上映，剧集的正在播出与今天播出，见 tmdb.HasList）、
// 按条件发现（见 Discover），或者搜索。
const (
	ExploreSearch   = "search"
	ExploreDiscover = "discover"
)

// ExploreItem 是探索页的一个条目，标上库里有没有、订阅过没有。
type ExploreItem struct {
	tmdb.Result
	InLibrary bool `json:"in_library"`
	// Subscribed 是这个条目有订阅（剧集是任意一季）；SubscriptionID 是其中最新的一个
	Subscribed     bool `json:"subscribed"`
	SubscriptionID uint `json:"subscription_id,omitempty"`
}

// ExplorePage 是探索页的一页。
type ExplorePage struct {
	Items      []ExploreItem `json:"items"`
	Page       int           `json:"page"`
	TotalPages int           `json:"total_pages"`
}

// Explore 取 TMDB 的一个列表（见 tmdb.HasList）或搜索结果（搜索只有一页），标上已入库（整理记录里有）与已订阅。
// 按条件发现用 Discover。
func (s *Service) Explore(ctx context.Context, kind, list string, page int, query string) (*ExplorePage, error) {
	if kind != models.MediaKindMovie && kind != models.MediaKindTV {
		return nil, fmt.Errorf("%w: 类型要选电影或剧集", ErrInvalid)
	}
	if list != ExploreSearch && !tmdb.HasList(kind, list) {
		lists := "trending、popular、top_rated、now_playing、upcoming"
		if kind == models.MediaKindTV {
			lists = "trending、popular、top_rated、on_the_air、airing_today"
		}
		return nil, fmt.Errorf("%w: 列表要选 %s 或 search", ErrInvalid, lists)
	}
	c, err := s.cfg.Recognizer.TMDB(ctx)
	if err != nil {
		return nil, err
	}
	if list != ExploreSearch {
		p, err := c.List(ctx, kind, list, page)
		if err != nil {
			return nil, err
		}
		return s.explorePage(ctx, kind, p)
	}
	q := strings.TrimSpace(query)
	if q == "" || utf8.RuneCountInString(q) > 100 {
		return nil, fmt.Errorf("%w: 搜索词要填，最多 100 个字", ErrInvalid)
	}
	results, err := c.Search(ctx, kind, q, 0)
	if err != nil {
		return nil, err
	}
	return s.explorePage(ctx, kind, &tmdb.ListPage{Results: results, Page: 1, TotalPages: 1})
}

// ExploreFilter 是按条件发现的筛选：Country 是出品国家或地区（两个大写字母，例如 CN、JP），Genres 是 TMDB 的类型编号
// （几个用 | 隔开时是其中任意一个，例如综艺是 10764|10767）。空的不限。
type ExploreFilter struct {
	Country string
	Genres  string
}

// Discover 按出品国家与类型发现 TMDB 上的电影或剧集（按热度排），标上已入库与已订阅。
func (s *Service) Discover(ctx context.Context, kind string, f ExploreFilter, page int) (*ExplorePage, error) {
	if kind != models.MediaKindMovie && kind != models.MediaKindTV {
		return nil, fmt.Errorf("%w: 类型要选电影或剧集", ErrInvalid)
	}
	f.Country, f.Genres = strings.ToUpper(strings.TrimSpace(f.Country)), strings.TrimSpace(f.Genres)
	if f.Country != "" && !countryRe.MatchString(f.Country) {
		return nil, fmt.Errorf("%w: 出品国家要写两个字母的代码，例如 CN、JP", ErrInvalid)
	}
	if f.Genres != "" && !genresRe.MatchString(f.Genres) {
		return nil, fmt.Errorf("%w: 类型要写 TMDB 的类型编号，几个用 | 隔开", ErrInvalid)
	}
	c, err := s.cfg.Recognizer.TMDB(ctx)
	if err != nil {
		return nil, err
	}
	p, err := c.Discover(ctx, kind, tmdb.DiscoverFilter{Country: f.Country, Genres: f.Genres}, page)
	if err != nil {
		return nil, err
	}
	return s.explorePage(ctx, kind, p)
}

var (
	countryRe = regexp.MustCompile(`^[A-Z]{2}$`)
	genresRe  = regexp.MustCompile(`^\d{1,6}(\|\d{1,6}){0,9}$`)
)

// explorePage 把 TMDB 的一页标上已入库与已订阅。
func (s *Service) explorePage(ctx context.Context, kind string, p *tmdb.ListPage) (*ExplorePage, error) {
	items, err := s.markExplore(ctx, kind, p.Results)
	if err != nil {
		return nil, err
	}
	return &ExplorePage{Items: items, Page: p.Page, TotalPages: p.TotalPages}, nil
}
