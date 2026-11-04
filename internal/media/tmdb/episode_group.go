package tmdb

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

// 剧集组（TMDB 的 episode groups）：同一部剧的另一种排法，例如动画的绝对集号、DVD 的分季、播出平台的分季。
// 列表 GET /tv/{id}/episode_groups，详情 GET /tv/episode_group/{id}：详情里按 order 排的几组，每组里按 order 排的集，
// 每集记着它在默认排法里的季（season_number）与集（episode_number）。

// EpisodeGroupTTL 是剧集组的缓存时间。
const EpisodeGroupTTL = 24 * time.Hour

// 剧集组的类型（type）。
const (
	EpisodeGroupAirDate    = 1
	EpisodeGroupAbsolute   = 2
	EpisodeGroupDVD        = 3
	EpisodeGroupDigital    = 4
	EpisodeGroupStoryArc   = 5
	EpisodeGroupProduction = 6
	EpisodeGroupTV         = 7
)

// episodeGroupIDRe 是剧集组编号的写法（24 位十六进制）。
var episodeGroupIDRe = regexp.MustCompile(`^[0-9a-f]{24}$`)

// ValidEpisodeGroupID 报告 id 是不是剧集组编号的写法。
func ValidEpisodeGroupID(id string) bool { return episodeGroupIDRe.MatchString(id) }

// EpisodeGroupSummary 是一部剧的一个剧集组（列表里的一项）。Network 是这种排法所属的电视台或平台（没有时为空）。
type EpisodeGroupSummary struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         int    `json:"type"`
	Description  string `json:"description,omitempty"`
	GroupCount   int    `json:"group_count"`
	EpisodeCount int    `json:"episode_count"`
	Network      string `json:"network,omitempty"`
}

// EpisodeGroupEpisode 是剧集组里的一集：Season、Episode 是它在默认排法里的季与集。
type EpisodeGroupEpisode struct {
	Season  int    `json:"season_number"`
	Episode int    `json:"episode_number"`
	Order   int    `json:"order"`
	Name    string `json:"name,omitempty"`
	AirDate string `json:"air_date,omitempty"`
}

// EpisodeGroupPart 是剧集组里的一组（相当于一季）：Episodes 按 Order 排好。
type EpisodeGroupPart struct {
	ID       string                `json:"id"`
	Name     string                `json:"name"`
	Order    int                   `json:"order"`
	Episodes []EpisodeGroupEpisode `json:"episodes"`
}

// EpisodeGroup 是剧集组的详情：Groups 按 Order 排好。
type EpisodeGroup struct {
	EpisodeGroupSummary
	Groups []EpisodeGroupPart `json:"groups"`
}

type rawEpisodeGroup struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         int    `json:"type"`
	Description  string `json:"description"`
	GroupCount   int    `json:"group_count"`
	EpisodeCount int    `json:"episode_count"`
	Network      *struct {
		Name string `json:"name"`
	} `json:"network"`
}

func (r rawEpisodeGroup) summary() EpisodeGroupSummary {
	s := EpisodeGroupSummary{
		ID: r.ID, Name: strings.TrimSpace(r.Name), Type: r.Type, Description: strings.TrimSpace(r.Description),
		GroupCount: r.GroupCount, EpisodeCount: r.EpisodeCount,
	}
	if r.Network != nil {
		s.Network = strings.TrimSpace(r.Network.Name)
	}
	return s
}

// EpisodeGroups 列出一部剧的剧集组（缓存 24 小时）；没有时是空的。
func (c *Client) EpisodeGroups(ctx context.Context, tvID int) ([]EpisodeGroupSummary, error) {
	if tvID <= 0 {
		return nil, ErrNotFound
	}
	var out []EpisodeGroupSummary
	q := url.Values{"language": {c.language}}
	err := c.cached(ctx, fmt.Sprintf("epgroups:%d:%s", tvID, c.language), EpisodeGroupTTL, &out, func() (any, error) {
		var raw struct {
			Results []rawEpisodeGroup `json:"results"`
		}
		if err := c.get(ctx, fmt.Sprintf("/tv/%d/episode_groups", tvID), q, &raw); err != nil {
			return nil, err
		}
		list := make([]EpisodeGroupSummary, 0, len(raw.Results))
		for _, r := range raw.Results {
			if ValidEpisodeGroupID(r.ID) {
				list = append(list, r.summary())
			}
		}
		return list, nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// EpisodeGroup 取一个剧集组的详情（缓存 24 小时）：组按 order 排、组里的集按 order 排（order 一样时按原来的先后）。
// 编号不是剧集组编号的写法时是 ErrNotFound（不发请求）。
func (c *Client) EpisodeGroup(ctx context.Context, id string) (*EpisodeGroup, error) {
	if !ValidEpisodeGroupID(id) {
		return nil, ErrNotFound
	}
	var out EpisodeGroup
	q := url.Values{"language": {c.language}}
	err := c.cached(ctx, "epgroup:"+id+":"+c.language, EpisodeGroupTTL, &out, func() (any, error) {
		var raw struct {
			rawEpisodeGroup
			Groups []EpisodeGroupPart `json:"groups"`
		}
		if err := c.get(ctx, "/tv/episode_group/"+id, q, &raw); err != nil {
			return nil, err
		}
		g := EpisodeGroup{EpisodeGroupSummary: raw.summary(), Groups: raw.Groups}
		slices.SortStableFunc(g.Groups, func(a, b EpisodeGroupPart) int { return a.Order - b.Order })
		for i := range g.Groups {
			slices.SortStableFunc(g.Groups[i].Episodes, func(a, b EpisodeGroupEpisode) int { return a.Order - b.Order })
		}
		return g, nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
