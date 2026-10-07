package subscribe

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

// 探索页的列表：本周趋势、热门，或者搜索。
const ExploreSearch = "search"

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

// Explore 取 TMDB 的趋势、热门或搜索结果（搜索只有一页），标上已入库（整理记录里有）与已订阅。
func (s *Service) Explore(ctx context.Context, kind, list string, page int, query string) (*ExplorePage, error) {
	if kind != models.MediaKindMovie && kind != models.MediaKindTV {
		return nil, fmt.Errorf("%w: 类型要选电影或剧集", ErrInvalid)
	}
	c, err := s.cfg.Recognizer.TMDB(ctx)
	if err != nil {
		return nil, err
	}
	out := &ExplorePage{Page: 1, TotalPages: 1}
	var results []tmdb.Result
	switch list {
	case tmdb.ListTrending, tmdb.ListPopular:
		p, lerr := c.List(ctx, kind, list, page)
		if lerr != nil {
			return nil, lerr
		}
		results, out.Page, out.TotalPages = p.Results, p.Page, p.TotalPages
	case ExploreSearch:
		q := strings.TrimSpace(query)
		if q == "" || utf8.RuneCountInString(q) > 100 {
			return nil, fmt.Errorf("%w: 搜索词要填，最多 100 个字", ErrInvalid)
		}
		if results, err = c.Search(ctx, kind, q, 0); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%w: 列表要选 trending、popular 或 search", ErrInvalid)
	}
	ids := make([]int, 0, len(results))
	for _, r := range results {
		ids = append(ids, r.ID)
	}
	inLib := map[int]bool{}
	subs := map[int]uint{}
	if len(ids) > 0 {
		var lib []int
		if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).
			Where("status = ? AND media_type = ? AND tmdb_id IN ?", models.MediaTransferDone, kind, ids).Distinct().Pluck("tmdb_id", &lib).Error; err != nil {
			return nil, fmt.Errorf("读取整理记录失败: %w", err)
		}
		for _, id := range lib {
			inLib[id] = true
		}
		var rows []models.MediaSubscription
		if err := s.cfg.DB.WithContext(ctx).Select("id", "tmdb_id").Where("media_type = ? AND tmdb_id IN ?", kind, ids).Order("id").Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("读取订阅失败: %w", err)
		}
		for _, r := range rows {
			subs[r.TMDBID] = r.ID
		}
	}
	out.Items = make([]ExploreItem, 0, len(results))
	for _, r := range results {
		out.Items = append(out.Items, ExploreItem{Result: r, InLibrary: inLib[r.ID], Subscribed: subs[r.ID] != 0, SubscriptionID: subs[r.ID]})
	}
	return out, nil
}
