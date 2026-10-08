package tmdb

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// 探索页用的列表。
const (
	// ListTrending 是本周趋势
	ListTrending = "trending"
	// ListPopular 是热门
	ListPopular = "popular"
	// ListTTL 是列表的缓存时间
	ListTTL = 6 * time.Hour
	// maxListPages 是最多翻到第几页
	maxListPages = 20
)

// ListPage 是列表的一页。
type ListPage struct {
	Results    []Result `json:"results"`
	Page       int      `json:"page"`
	TotalPages int      `json:"total_pages"`
}

// List 取电影或剧集的本周趋势或热门列表。page 从 1 开始，最多到第 20 页。
func (c *Client) List(ctx context.Context, kind, list string, page int) (*ListPage, error) {
	if err := checkKind(kind); err != nil {
		return nil, err
	}
	var path string
	switch list {
	case ListTrending:
		path = "/trending/" + kind + "/week"
	case ListPopular:
		path = "/" + kind + "/popular"
	default:
		return nil, fmt.Errorf("列表无效: %q", list)
	}
	page = min(max(page, 1), maxListPages)
	q := url.Values{"language": {c.language}, "page": {strconv.Itoa(page)}}
	var out ListPage
	err := c.cached(ctx, "list:"+kind+":"+list+":"+q.Encode(), ListTTL, &out, func() (any, error) {
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
