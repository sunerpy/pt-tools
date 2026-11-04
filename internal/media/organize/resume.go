package organize

import (
	"context"
	"fmt"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/server"
	"github.com/sunerpy/pt-tools/internal/media/serverindex"
	"github.com/sunerpy/pt-tools/models"
)

const (
	// resumeLimit 是每台服务器最多取几项没看完的
	resumeLimit = 12
	// resumeTimeout 是取一台服务器的继续观看最多等多久
	resumeTimeout = 10 * time.Second
)

// ResumeItem 是继续观看的一项（首页的继续观看卡）：剧集的 TMDB 编号是剧的（按索引里的剧找），图从 TMDB 取
// （剧集用这一集的剧照，电影用背景图）；没同步索引或 TMDB 取不到时为空。
type ResumeItem struct {
	ServerID         uint    `json:"server_id"`
	ItemID           string  `json:"item_id"`
	MediaType        string  `json:"media_type"`
	TMDBID           int     `json:"tmdb_id,omitempty"`
	Title            string  `json:"title"`
	Season           int     `json:"season,omitempty"`
	Episode          int     `json:"episode,omitempty"`
	EpisodeName      string  `json:"episode_name,omitempty"`
	StillPath        string  `json:"still_path,omitempty"`
	BackdropPath     string  `json:"backdrop_path,omitempty"`
	PosterPath       string  `json:"poster_path,omitempty"`
	Percent          float64 `json:"percent"`
	RemainingSeconds int64   `json:"remaining_seconds,omitempty"`
	OpenURL          string  `json:"open_url,omitempty"`
}

// ResumeGroup 是一台服务器的继续观看：选了用户的 Emby、Jellyfin 才有。Error 是这台没取到的原因（别的服务器照样给）。
type ResumeGroup struct {
	ServerID uint         `json:"server_id"`
	Name     string       `json:"name"`
	Kind     string       `json:"kind"`
	User     string       `json:"user"`
	Items    []ResumeItem `json:"items"`
	Error    string       `json:"error,omitempty"`
}

// Resume 是各台服务器的继续观看（启用的、选了用户的 Emby 与 Jellyfin），每台最多 12 项，最近看的在前。
func (s *Service) Resume(ctx context.Context) ([]ResumeGroup, error) {
	var servers []models.MediaServer
	if err := s.cfg.DB.WithContext(ctx).Where("enabled = ? AND resume_user_id <> '' AND kind <> ?", true, models.MediaServerPlex).
		Order("id").Find(&servers).Error; err != nil {
		return nil, fmt.Errorf("读取媒体服务器失败: %w", err)
	}
	out := make([]ResumeGroup, 0, len(servers))
	for _, srv := range servers {
		g := ResumeGroup{ServerID: srv.ID, Name: srv.Name, Kind: srv.Kind, User: srv.ResumeUserName, Items: []ResumeItem{}}
		if g.User == "" {
			g.User = srv.ResumeUserID
		}
		items, err := s.resumeOf(ctx, srv)
		if err != nil {
			g.Error = err.Error()
		} else {
			g.Items = items
		}
		out = append(out, g)
	}
	return out, nil
}

func (s *Service) resumeOf(ctx context.Context, srv models.MediaServer) ([]ResumeItem, error) {
	c, err := s.serverClient(srv)
	if err != nil {
		return nil, err
	}
	rctx, cancel := context.WithTimeout(ctx, resumeTimeout)
	defer cancel()
	list, err := c.Resume(rctx, srv.ResumeUserID, resumeLimit)
	if err != nil {
		return nil, err
	}
	out := make([]ResumeItem, 0, len(list))
	for _, it := range list {
		out = append(out, s.resumeItem(ctx, srv, it))
	}
	return out, nil
}

// resumeItem 补上 TMDB 编号、图与打开链接。
func (s *Service) resumeItem(ctx context.Context, srv models.MediaServer, it server.ResumeItem) ResumeItem {
	r := ResumeItem{
		ServerID: srv.ID, ItemID: it.ID, Title: it.Title, Percent: it.Percent, RemainingSeconds: it.RemainingSeconds,
		OpenURL: serverindex.OpenURL(srv, it.ID),
	}
	kind := models.MediaKindMovie
	if it.Kind == models.MediaServerItemEpisode {
		kind = models.MediaKindTV
		r.Title, r.EpisodeName, r.Season, r.Episode = it.SeriesName, it.Title, it.Season, it.Episode
		if id, err := serverindex.SeriesTMDB(ctx, s.cfg.DB, srv.ID, it.SeriesID); err == nil {
			r.TMDBID = id
		}
	} else {
		r.TMDBID = it.TMDBID
	}
	r.MediaType = kind
	if r.TMDBID <= 0 {
		return r
	}
	c, err := s.cfg.Recognizer.TMDB(ctx)
	if err != nil {
		return r
	}
	if d, err := c.Details(ctx, kind, r.TMDBID); err == nil {
		r.PosterPath, r.BackdropPath = d.PosterPath, d.BackdropPath
		if r.Title == "" {
			r.Title = d.Title
		}
	}
	if kind == models.MediaKindTV && r.Episode > 0 {
		if season, err := c.Season(ctx, r.TMDBID, r.Season); err == nil {
			if e := season.Episode(r.Episode); e != nil {
				r.StillPath = e.StillPath
			}
		}
	}
	return r
}
