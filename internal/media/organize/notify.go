package organize

import (
	"context" //nolint:gosec // 只用来给目录监控的一项取个短标识，不涉及安全
	"fmt"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/internal/media/transfer"
	"github.com/sunerpy/pt-tools/internal/notify"
)

// notify 发一条入库通知（设置里选了通道时）。返回给结果看的错误；成功时为空。
func (s *Service) notify(ctx context.Context, p *Plan, created []*PlanItem) string {
	if s.cfg.Notify == nil {
		return ""
	}
	settings, err := s.Settings(ctx)
	if err != nil || len(settings.NotifyChannels) == 0 {
		return ""
	}
	n := Notice{Key: fmt.Sprintf("%s:%d", p.Hash, created[0].HistoryID), ChannelIDs: settings.NotifyChannels}
	n.Title, n.Text = noticeText(p, created)
	n.Event = noticeEvent(s.cfg.Now(), p, created)
	if err := s.cfg.Notify(ctx, n); err != nil {
		return "入库通知没有发出：" + err.Error()
	}
	return ""
}

// noticeText 是入库通知的标题与正文：条目、季集、画质、媒体库、来源种子，最后是海报地址。
// noticeEvent 是入库通知的事件变量，和 noticeText 用同样的数据。
func noticeEvent(at time.Time, p *Plan, created []*PlanItem) *notify.Event {
	m := p.Match
	name := m.Title
	if m.Year > 0 {
		name = fmt.Sprintf("%s (%d)", m.Title, m.Year)
	}
	episodes := ""
	if m.MediaType == tmdb.KindTV {
		eps := make([]string, 0, len(created))
		for _, it := range created {
			eps = append(eps, transfer.SeasonEpisode(it.Season, it.Episode, it.EpisodeEnd))
		}
		episodes = strings.Join(eps, "、")
	}
	return notify.NewEvent(notify.EventMediaOrganized, at, map[string]any{
		"name": name, "media_type": m.MediaType, "episodes": episodes, "files": len(created),
		"quality": vars(m, created[0].meta, p.meta).Quality, "library": p.Library.Name, "torrent": p.Name,
		"poster": p.posterURL(m.PosterPath),
	})
}

// posterURL 是通知里的海报地址：用这次整理的 TMDB 客户端的图片地址（用户换了图片地址时跟着换），没有客户端时用官方地址。
func (p *Plan) posterURL(path string) string {
	if p.tmdbClient != nil {
		return p.tmdbClient.PosterURL(path, "w500")
	}
	return tmdb.PosterURL(path, "w500")
}

func noticeText(p *Plan, created []*PlanItem) (string, string) {
	m := p.Match
	name := m.Title
	if m.Year > 0 {
		name = fmt.Sprintf("%s (%d)", m.Title, m.Year)
	}
	var lines []string
	if m.MediaType == tmdb.KindTV {
		var eps []string
		for _, it := range created {
			eps = append(eps, transfer.SeasonEpisode(it.Season, it.Episode, it.EpisodeEnd))
		}
		if len(eps) > 6 {
			eps = append(eps[:3], "…", eps[len(eps)-1])
		}
		lines = append(lines, fmt.Sprintf("剧集 %s（%d 个文件）", strings.Join(eps, "、"), len(created)))
	} else {
		lines = append(lines, "电影")
	}
	if q := vars(m, created[0].meta, p.meta).Quality; q != "" {
		lines = append(lines, "画质："+q)
	}
	if p.WatchDirID > 0 {
		lines = append(lines, "媒体库："+p.Library.Name, "来自目录监控："+p.Name)
	} else {
		lines = append(lines, "媒体库："+p.Library.Name, "种子："+p.Name)
	}
	if poster := p.posterURL(m.PosterPath); poster != "" {
		lines = append(lines, "海报："+poster)
	}
	return "入库：" + name, strings.Join(lines, "\n")
}
