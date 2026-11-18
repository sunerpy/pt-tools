package tmdb

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

// SeasonsTTL 是剧集各季列表的缓存时间：新的一季宣布了要能看到，比详情的一周短。
const SeasonsTTL = 24 * time.Hour

// Recommendations 取 TMDB 按这个条目推荐的电影或剧集（第 1 页，同类），缓存 ListTTL。
func (c *Client) Recommendations(ctx context.Context, kind string, id int) ([]Result, error) {
	if err := checkKind(kind); err != nil {
		return nil, err
	}
	if id <= 0 {
		return nil, ErrNotFound
	}
	q := url.Values{"language": {c.language}, "page": {"1"}}
	var out []Result
	err := c.cached(ctx, fmt.Sprintf("recommendations:%s:%d:%s", kind, id, c.language), ListTTL, &out, func() (any, error) {
		var raw struct {
			Results []rawItem `json:"results"`
		}
		if err := c.get(ctx, fmt.Sprintf("/%s/%d/recommendations", kind, id), q, &raw); err != nil {
			return nil, err
		}
		res := make([]Result, 0, len(raw.Results))
		for _, r := range raw.Results {
			res = append(res, r.result(kind))
		}
		return res, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SeasonSummary 是剧集详情里列的一季：季号（0 是特别篇）、名字、集数、首播日期与季海报。
type SeasonSummary struct {
	Number       int    `json:"season_number"`
	Name         string `json:"name,omitempty"`
	EpisodeCount int    `json:"episode_count"`
	AirDate      string `json:"air_date,omitempty"`
	PosterPath   string `json:"poster_path,omitempty"`
}

// Seasons 取剧集的各季（按 TMDB 给的顺序），缓存 SeasonsTTL。格式不对的海报路径去掉。
func (c *Client) Seasons(ctx context.Context, tvID int) ([]SeasonSummary, error) {
	if tvID <= 0 {
		return nil, ErrNotFound
	}
	var out []SeasonSummary
	err := c.cached(ctx, fmt.Sprintf("seasons:%d:%s", tvID, c.language), SeasonsTTL, &out, func() (any, error) {
		var raw struct {
			Seasons []SeasonSummary `json:"seasons"`
		}
		if err := c.get(ctx, fmt.Sprintf("/tv/%d", tvID), url.Values{"language": {c.language}}, &raw); err != nil {
			return nil, err
		}
		res := make([]SeasonSummary, 0, len(raw.Seasons))
		for _, s := range raw.Seasons {
			if !imagePathRe.MatchString(s.PosterPath) {
				s.PosterPath = ""
			}
			res = append(res, s)
		}
		return res, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
