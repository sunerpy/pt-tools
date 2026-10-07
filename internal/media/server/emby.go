package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// emby 是 Emby 与 Jellyfin（两者的接口相同，只有鉴权头不同）。
type emby struct {
	h *httpc
}

func (e *emby) Test(ctx context.Context) (Info, error) {
	var out struct {
		ServerName string `json:"ServerName"`
		Version    string `json:"Version"`
	}
	if err := e.h.do(ctx, http.MethodGet, "/System/Info", nil, nil, &out); err != nil {
		return Info{}, err
	}
	if out.ServerName == "" && out.Version == "" {
		return Info{}, fmt.Errorf("%w: /System/Info 没有服务器信息（地址是不是填成了别的服务）", ErrUnavailable)
	}
	return Info{Name: out.ServerName, Version: out.Version}, nil
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
