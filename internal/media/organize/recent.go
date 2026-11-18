package organize

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/media/serverindex"
	"github.com/sunerpy/pt-tools/models"
)

// 首页（媒体中心 M8）的「最近入库」：已整理的记录按条目与季合并，连了媒体服务器时并进服务器里新加、不是 pt-tools 整理的项。

const (
	// recentLimit 是最近入库最多几项
	recentLimit = 12
	// recentScan 是找最近入库时最多看几条整理记录（同一季的几集合成一项）
	recentScan = 500
)

// 最近入库的一项从哪来（RecentItem.Source）。
const (
	RecentFromLibrary = "pt-tools"
	RecentFromServer  = "server"
)

// RecentItem 是最近入库的一项：一部电影，或剧的一季（同一季的几集合成一项，写最新那一集）。Quality 按种子名解析
// （Remux 或分辨率；服务器里新加的没有）；PosterPath 从 TMDB 取，取不到时为空。
type RecentItem struct {
	MediaType  string    `json:"media_type"`
	TMDBID     int       `json:"tmdb_id"`
	Title      string    `json:"title"`
	Season     int       `json:"season,omitempty"`
	Episode    int       `json:"episode,omitempty"`
	PosterPath string    `json:"poster_path,omitempty"`
	Quality    string    `json:"quality,omitempty"`
	Source     string    `json:"source"`
	At         time.Time `json:"at"`
}

// Recent 是首页的最近入库：最新在前，最多 12 项；TodayCount 是今天整理进库的记录数。
type Recent struct {
	TodayCount int64        `json:"today_count"`
	Items      []RecentItem `json:"items"`
}

type recentKey struct {
	kind   string
	tmdbID int
	season int
}

// Recent 列出最近入库的电影与剧（见 Recent）。媒体服务器的索引读不到时只列整理记录。
func (s *Service) Recent(ctx context.Context) (*Recent, error) {
	db := s.cfg.DB.WithContext(ctx)
	now := s.cfg.Now().In(time.Local)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	out := &Recent{Items: []RecentItem{}}
	if err := db.Model(&models.MediaTransferHistory{}).Where("status = ? AND julianday(created_at) >= julianday(?)", models.MediaTransferDone, day).
		Count(&out.TodayCount).Error; err != nil {
		return nil, fmt.Errorf("读取整理记录失败: %w", err)
	}
	var rows []models.MediaTransferHistory
	if err := db.Where("status = ? AND tmdb_id > 0 AND media_type IN ?", models.MediaTransferDone, []string{models.MediaKindMovie, models.MediaKindTV}).
		Order("created_at DESC, id DESC").Limit(recentScan).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取整理记录失败: %w", err)
	}
	seen := map[recentKey]bool{}
	var items []RecentItem
	for _, r := range rows {
		key := recentKey{r.MediaType, r.TMDBID, r.Season}
		if r.MediaType != models.MediaKindTV {
			key.season = 0
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		name := r.TorrentName
		if name == "" {
			name = filepath.Base(r.SourcePath)
		}
		it := RecentItem{MediaType: r.MediaType, TMDBID: r.TMDBID, Title: r.Title, Quality: qualityOf(meta.Parse(name, "")), Source: RecentFromLibrary, At: r.CreatedAt}
		if r.MediaType == models.MediaKindTV {
			it.Season, it.Episode = r.Season, max(r.EpisodeEnd, r.Episode)
		}
		items = append(items, it)
		if len(items) >= recentLimit {
			break
		}
	}
	added, err := serverindex.RecentAdded(ctx, s.cfg.DB, recentLimit)
	if err != nil {
		s.cfg.Logger.Debugf("[整理入库] 读取媒体服务器的索引失败（最近入库只列整理记录）: %v", err)
	}
	for _, a := range added {
		key := recentKey{a.MediaType, a.TMDBID, a.Season}
		if a.MediaType != models.MediaKindTV {
			key.season = 0
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		it := RecentItem{MediaType: a.MediaType, TMDBID: a.TMDBID, Title: a.Title, Source: RecentFromServer, At: a.AddedAt}
		if a.MediaType == models.MediaKindTV {
			it.Season, it.Episode = a.Season, a.Episode
		}
		items = append(items, it)
	}
	slices.SortStableFunc(items, func(a, b RecentItem) int { return cmp.Compare(b.At.UnixNano(), a.At.UnixNano()) })
	if len(items) > recentLimit {
		items = items[:recentLimit]
	}
	s.recentPosters(ctx, items)
	out.Items = append(out.Items, items...)
	return out, nil
}

// recentPosters 从 TMDB 补上海报（标题没记下时也补上）；TMDB 读不到时留空。
func (s *Service) recentPosters(ctx context.Context, items []RecentItem) {
	if len(items) == 0 || s.cfg.Recognizer == nil {
		return
	}
	c, err := s.cfg.Recognizer.TMDB(ctx)
	if err != nil {
		return
	}
	for i := range items {
		it := &items[i]
		d, err := c.Details(ctx, it.MediaType, it.TMDBID)
		if err != nil {
			continue
		}
		it.PosterPath = d.PosterPath
		if it.Title == "" {
			it.Title = d.Title
		}
	}
}

// qualityOf 是最近入库海报角上写的质量：Remux，或者分辨率（2160p、1080p…）；认不出时为空。
func qualityOf(m meta.Meta) string {
	if m.Remux {
		return "Remux"
	}
	return m.Resolution
}
