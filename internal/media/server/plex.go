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

type plex struct {
	h *httpc
}

type plexSection struct {
	Key      string `json:"key"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Location []struct {
		Path string `json:"path"`
	} `json:"Location"`
}

func (p *plex) sections(ctx context.Context) ([]plexSection, error) {
	var out struct {
		MediaContainer struct {
			Directory []plexSection `json:"Directory"`
		} `json:"MediaContainer"`
	}
	if err := p.h.do(ctx, http.MethodGet, "/library/sections", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.MediaContainer.Directory, nil
}

func (p *plex) Test(ctx context.Context) (Info, error) {
	var root struct {
		MediaContainer struct {
			FriendlyName string `json:"friendlyName"`
			Version      string `json:"version"`
		} `json:"MediaContainer"`
	}
	if err := p.h.do(ctx, http.MethodGet, "/", nil, nil, &root); err != nil {
		return Info{}, err
	}
	if root.MediaContainer.Version == "" {
		return Info{}, fmt.Errorf("%w: 没有读到 Plex 的版本（地址是不是填成了别的服务）", ErrUnavailable)
	}
	secs, err := p.sections(ctx)
	if err != nil {
		return Info{}, err
	}
	return Info{Name: root.MediaContainer.FriendlyName, Version: root.MediaContainer.Version, Libraries: len(secs)}, nil
}

// RefreshPaths 找到包含每个目录的分区，按路径刷新（只扫这个目录）。
func (p *plex) RefreshPaths(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	secs, err := p.sections(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, path := range paths {
		key := ""
		best := -1
		for _, s := range secs {
			for _, loc := range s.Location {
				if within(loc.Path, path) && len(loc.Path) > best {
					key, best = s.Key, len(loc.Path)
				}
			}
		}
		if key == "" {
			errs = append(errs, fmt.Errorf("%w: %s", ErrNoLibrary, path))
			continue
		}
		if err := p.h.do(ctx, http.MethodGet, "/library/sections/"+url.PathEscape(key)+"/refresh", url.Values{"path": {path}}, nil, nil); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (p *plex) RefreshAll(ctx context.Context) error {
	secs, err := p.sections(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, s := range secs {
		if err := p.h.do(ctx, http.MethodGet, "/library/sections/"+url.PathEscape(s.Key)+"/refresh", nil, nil, nil); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Exists 在电影或剧集分区里按名字找，再比对 Guid 里的 tmdb:// 与 imdb:// 编号（新版 Plex 代理才有）。
func (p *plex) Exists(ctx context.Context, q Query) (bool, error) {
	if strings.TrimSpace(q.Title) == "" {
		return false, errors.New("查询是否入库需要名称")
	}
	want := "movie"
	if q.Kind == "tv" {
		want = "show"
	}
	secs, err := p.sections(ctx)
	if err != nil {
		return false, err
	}
	var ids []string
	if q.TMDBID > 0 {
		ids = append(ids, "tmdb://"+strconv.Itoa(q.TMDBID))
	}
	if q.IMDbID != "" {
		ids = append(ids, "imdb://"+strings.ToLower(q.IMDbID))
	}
	for _, s := range secs {
		if s.Type != want {
			continue
		}
		var out struct {
			MediaContainer struct {
				Metadata []struct {
					Guid []struct {
						ID string `json:"id"`
					} `json:"Guid"`
				} `json:"Metadata"`
			} `json:"MediaContainer"`
		}
		v := url.Values{"title": {q.Title}, "includeGuids": {"1"}}
		if err := p.h.do(ctx, http.MethodGet, "/library/sections/"+url.PathEscape(s.Key)+"/all", v, nil, &out); err != nil {
			return false, err
		}
		for _, m := range out.MediaContainer.Metadata {
			for _, g := range m.Guid {
				for _, id := range ids {
					if strings.EqualFold(g.ID, id) {
						return true, nil
					}
				}
			}
		}
	}
	return false, nil
}
