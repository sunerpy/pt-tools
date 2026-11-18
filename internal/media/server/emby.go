package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/models"
)

// emby 是 Emby 与 Jellyfin（两者的接口相同，只有鉴权头不同）。
type emby struct {
	h *httpc
}

func (e *emby) Test(ctx context.Context) (Info, error) {
	var out struct {
		ServerName string `json:"ServerName"`
		Version    string `json:"Version"`
		ID         string `json:"Id"`
	}
	if err := e.h.do(ctx, http.MethodGet, "/System/Info", nil, nil, &out); err != nil {
		return Info{}, err
	}
	if out.ServerName == "" && out.Version == "" {
		return Info{}, fmt.Errorf("%w: /System/Info 没有服务器信息（地址是不是填成了别的服务）", ErrUnavailable)
	}
	return Info{Name: out.ServerName, Version: out.Version, ID: out.ID}, nil
}

type embyUpdate struct {
	Path       string `json:"Path"`
	UpdateType string `json:"UpdateType"`
}

func (e *emby) RefreshPaths(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	ups := make([]embyUpdate, 0, len(paths))
	for _, p := range paths {
		ups = append(ups, embyUpdate{Path: p, UpdateType: "Created"})
	}
	return e.h.do(ctx, http.MethodPost, "/Library/Media/Updated", nil, map[string]any{"Updates": ups}, nil)
}

func (e *emby) RefreshAll(ctx context.Context) error {
	return e.h.do(ctx, http.MethodPost, "/Library/Refresh", nil, nil, nil)
}

// Exists 按名字搜，再比对条目的 ProviderIds（Jellyfin 没有按编号过滤的参数，Emby 与它用同一种查法）。
func (e *emby) Exists(ctx context.Context, q Query) (bool, error) {
	if strings.TrimSpace(q.Title) == "" {
		return false, errors.New("查询是否入库需要名称")
	}
	typ := "Movie"
	if q.Kind == "tv" {
		typ = "Series"
	}
	var out struct {
		Items []struct {
			ProviderIDs map[string]string `json:"ProviderIds"`
		} `json:"Items"`
	}
	v := url.Values{
		"Recursive": {"true"}, "IncludeItemTypes": {typ}, "SearchTerm": {q.Title},
		"Fields": {"ProviderIds"}, "Limit": {"50"},
	}
	if err := e.h.do(ctx, http.MethodGet, "/Items", v, nil, &out); err != nil {
		return false, err
	}
	for _, it := range out.Items {
		for k, val := range it.ProviderIDs {
			switch strings.ToLower(k) {
			case "tmdb":
				if q.TMDBID > 0 && strings.TrimSpace(val) == strconv.Itoa(q.TMDBID) {
					return true, nil
				}
			case "imdb":
				if q.IMDbID != "" && strings.EqualFold(strings.TrimSpace(val), q.IMDbID) {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// embyItem 是 /Items 与继续观看里的一项（SeriesId、ParentIndexNumber、IndexNumber、ProductionYear、LocationType 是默认就有的字段）。
type embyItem struct {
	ID                string            `json:"Id"`
	Name              string            `json:"Name"`
	Type              string            `json:"Type"`
	SeriesID          string            `json:"SeriesId"`
	SeriesName        string            `json:"SeriesName"`
	ParentIndexNumber *int              `json:"ParentIndexNumber"`
	IndexNumber       *int              `json:"IndexNumber"`
	IndexNumberEnd    *int              `json:"IndexNumberEnd"`
	ProductionYear    int               `json:"ProductionYear"`
	Path              string            `json:"Path"`
	DateCreated       string            `json:"DateCreated"`
	LocationType      string            `json:"LocationType"`
	ProviderIDs       map[string]string `json:"ProviderIds"`
	RunTimeTicks      int64             `json:"RunTimeTicks"`
	UserData          *struct {
		PlayedPercentage      float64 `json:"PlayedPercentage"`
		PlaybackPositionTicks int64   `json:"PlaybackPositionTicks"`
	} `json:"UserData"`
}

func intOr(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// item 换成索引的一项；不是电影、剧、集的，与只是元数据里有、没有文件的（Virtual）不要。
func (it embyItem) item() (Item, bool) {
	out := Item{ID: it.ID, Title: it.Name, Year: it.ProductionYear, Path: it.Path, SeriesID: it.SeriesID}
	switch it.Type {
	case "Movie":
		out.Kind = models.MediaServerItemMovie
	case "Series":
		out.Kind = models.MediaServerItemSeries
	case "Episode":
		out.Kind = models.MediaServerItemEpisode
		out.Season, out.Episode = intOr(it.ParentIndexNumber), intOr(it.IndexNumber)
		if end := intOr(it.IndexNumberEnd); end > out.Episode {
			out.EpisodeEnd = end
		}
	default:
		return Item{}, false
	}
	if it.ID == "" || strings.EqualFold(it.LocationType, "Virtual") {
		return Item{}, false
	}
	if out.Kind != models.MediaServerItemEpisode {
		out.SeriesID = ""
	}
	out.TMDBID, out.IMDbID, out.TVDBID = providerIDs(it.ProviderIDs)
	if t, err := time.Parse(time.RFC3339Nano, it.DateCreated); err == nil {
		out.AddedAt = t.UTC()
	}
	return out, true
}

// ListItems 按入库时间从早到晚分页列出电影、剧与集（新加的排在最后，列的过程中加进来的不会让前面的错位）；
// 缺集占位（IsMissing、Virtual）不列。
func (e *emby) ListItems(ctx context.Context, cursor string) (ItemPage, error) {
	pc, err := parseCursor(cursor)
	if err != nil {
		return ItemPage{}, err
	}
	v := url.Values{
		"Recursive": {"true"}, "IncludeItemTypes": {"Movie,Series,Episode"}, "Fields": {"ProviderIds,Path,DateCreated"},
		"IsMissing": {"false"}, "ExcludeLocationTypes": {"Virtual"}, "SortBy": {"DateCreated,SortName"}, "SortOrder": {"Ascending"},
		"EnableImages": {"false"}, "EnableUserData": {"false"},
		"StartIndex": {strconv.Itoa(pc.start)}, "Limit": {strconv.Itoa(itemPageSize)},
	}
	var out struct {
		Items            []embyItem `json:"Items"`
		TotalRecordCount int        `json:"TotalRecordCount"`
	}
	if err := e.h.do(ctx, http.MethodGet, "/Items", v, nil, &out); err != nil {
		return ItemPage{}, err
	}
	page := ItemPage{Items: make([]Item, 0, len(out.Items))}
	for _, it := range out.Items {
		if got, ok := it.item(); ok {
			page.Items = append(page.Items, got)
		}
	}
	page.Next, page.Changed = pc.next(len(out.Items), out.TotalRecordCount)
	return page, nil
}

func (e *emby) Users(ctx context.Context) ([]User, error) {
	var out []struct {
		ID   string `json:"Id"`
		Name string `json:"Name"`
	}
	if err := e.h.do(ctx, http.MethodGet, "/Users", nil, nil, &out); err != nil {
		return nil, err
	}
	users := make([]User, 0, len(out))
	for _, u := range out {
		if u.ID != "" {
			users = append(users, User{ID: u.ID, Name: u.Name})
		}
	}
	return users, nil
}

// Resume 是 /Users/{id}/Items/Resume（Jellyfin 新版本另有 /UserItems/Resume，旧路径仍保留）。
func (e *emby) Resume(ctx context.Context, userID string, limit int) ([]ResumeItem, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("%w: 没有选用户", ErrBadConfig)
	}
	v := url.Values{
		"Limit": {strconv.Itoa(limit)}, "Fields": {"ProviderIds"}, "MediaTypes": {"Video"},
		"EnableImages": {"false"}, "EnableUserData": {"true"},
	}
	var out struct {
		Items []embyItem `json:"Items"`
	}
	if err := e.h.do(ctx, http.MethodGet, "/Users/"+url.PathEscape(userID)+"/Items/Resume", v, nil, &out); err != nil {
		return nil, err
	}
	items := make([]ResumeItem, 0, len(out.Items))
	for _, it := range out.Items {
		r := ResumeItem{ID: it.ID, Title: it.Name, Year: it.ProductionYear}
		switch it.Type {
		case "Movie":
			r.Kind = models.MediaServerItemMovie
		case "Episode":
			r.Kind = models.MediaServerItemEpisode
			r.SeriesID, r.SeriesName = it.SeriesID, it.SeriesName
			r.Season, r.Episode = intOr(it.ParentIndexNumber), intOr(it.IndexNumber)
		default:
			continue
		}
		r.TMDBID, r.IMDbID, _ = providerIDs(it.ProviderIDs)
		if it.UserData != nil {
			r.Percent = it.UserData.PlayedPercentage
			if it.RunTimeTicks > 0 && it.UserData.PlaybackPositionTicks < it.RunTimeTicks {
				r.RemainingSeconds = (it.RunTimeTicks - it.UserData.PlaybackPositionTicks) / 10_000_000
			}
		}
		items = append(items, r)
	}
	return items, nil
}
