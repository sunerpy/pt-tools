package subscribe

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/serverindex"
	"github.com/sunerpy/pt-tools/models"
)

// 首页（媒体中心 M8）的「今天播出」与「这周还要播」：同订阅日历，今天播出的每一项再加首页卡片上要写的。

// homeWeekDays 是「这周还要播」列明天起几天
const homeWeekDays = 6

// TodayItem 是首页「今天播出」的一项：日历的一项（见 CalendarEntry），另加
//   - BackdropPath：没有剧照时用的背景图（这一部的）；
//   - Version：已入库的版本（按整理记录的种子名解析，例如 2160p HHWEB）；
//   - Site、Percent、ETASeconds：下载中的站点、进度（0–100）与预计还要多久（秒，不知道时为 0）；
//   - Lock：等版本时锁住的版本（例如 HHWEB 2160p）；
//   - OpenURL：已入库、媒体服务器里有这一集（电影是这一部）时，在服务器里打开它的链接。
type TodayItem struct {
	CalendarEntry
	BackdropPath string  `json:"backdrop_path,omitempty"`
	Version      string  `json:"version,omitempty"`
	Site         string  `json:"site,omitempty"`
	Percent      float64 `json:"percent,omitempty"`
	ETASeconds   int64   `json:"eta_seconds,omitempty"`
	Lock         string  `json:"lock,omitempty"`
	OpenURL      string  `json:"open_url,omitempty"`
}

// HomeCalendar 是首页用的今天播出与这周还要播（明天起 6 天）。Errors 是没读到 TMDB 的订阅（别的照样列）；
// Subscriptions 是订阅的个数（一个都没有时首页写「还没有追剧」，不写「今天没有要播的集」）。
type HomeCalendar struct {
	Today         []TodayItem     `json:"today"`
	Week          []CalendarEntry `json:"week"`
	Errors        []string        `json:"errors"`
	Subscriptions int64           `json:"subscriptions"`
}

// HomeCalendar 列出今天播出的集与上映的电影（带上首页卡片要写的）和明天起 6 天的。
func (s *Service) HomeCalendar(ctx context.Context) (*HomeCalendar, error) {
	now := s.cfg.Now().In(time.Local)
	today := now.Format(time.DateOnly)
	end := now.AddDate(0, 0, homeWeekDays).Format(time.DateOnly)
	var subs int64
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Count(&subs).Error; err != nil {
		return nil, fmt.Errorf("读取订阅失败: %w", err)
	}
	entries, errs, err := s.calendarEntries(ctx, today, end)
	if err != nil {
		return nil, err
	}
	out := &HomeCalendar{Today: []TodayItem{}, Week: []CalendarEntry{}, Errors: errs, Subscriptions: subs}
	h := homeLookup{s: s, ctx: ctx, backdrops: map[string]string{}, seasons: map[[2]int]map[int]serverindex.SeasonHit{}}
	for _, e := range entries {
		if e.AirDate != today {
			out.Week = append(out.Week, e)
			continue
		}
		out.Today = append(out.Today, h.today(e))
	}
	return out, nil
}

// homeLookup 给今天播出的一项补上卡片要写的；同一部剧的背景图、同一季的索引只读一次。读不到的部分留空，不挡别的。
type homeLookup struct {
	s         *Service
	ctx       context.Context
	backdrops map[string]string
	seasons   map[[2]int]map[int]serverindex.SeasonHit
}

func (h *homeLookup) today(e CalendarEntry) TodayItem {
	it := TodayItem{CalendarEntry: e, BackdropPath: h.backdrop(e.MediaType, e.TMDBID)}
	sub, err := h.s.subRow(h.ctx, e.SubscriptionID)
	if err != nil {
		return it
	}
	var row models.MediaSubscriptionEpisode
	if e.MediaType == models.MediaKindTV {
		h.find(&row, "subscription_id = ? AND episode = ?", sub.ID, e.Episode)
	}
	switch e.State {
	case models.MediaEpisodeLibrary:
		it.Version = h.version(e, row)
		it.OpenURL = h.openURL(e, row)
	case models.MediaEpisodeDownloading:
		h.progress(&it, row)
	case models.MediaEpisodeWaitingVersion:
		if g, r, ok := lockOf(&sub); ok {
			it.Lock = lockText(g, r)
		}
	}
	return it
}

// find 读一行（读不到、出错时留着零值：首页这一项少写一点，不挡别的）。
func (h *homeLookup) find(out any, query string, args ...any) {
	if err := h.s.cfg.DB.WithContext(h.ctx).Where(query, args...).Limit(1).Find(out).Error; err != nil {
		h.s.cfg.Logger.Debugf("[订阅] 首页读取失败: %v", err)
	}
}

// refID 是台账里记的编号（整理记录、订阅的种子记录）；不是编号时为 0。
func refID(ref string) uint64 {
	id, _ := strconv.ParseUint(ref, 10, 64)
	return id
}

func (h *homeLookup) backdrop(kind string, id int) string {
	key := kind + ":" + strconv.Itoa(id)
	if v, ok := h.backdrops[key]; ok {
		return v
	}
	h.backdrops[key] = ""
	c, err := h.s.cfg.Recognizer.TMDB(h.ctx)
	if err != nil {
		return ""
	}
	if d, err := c.Details(h.ctx, kind, id); err == nil {
		h.backdrops[key] = d.BackdropPath
	}
	return h.backdrops[key]
}

// version 是已入库的版本：台账记着整理记录时用那一条，否则用这一集（这部电影）最新的整理记录；按种子名解析。
func (h *homeLookup) version(e CalendarEntry, row models.MediaSubscriptionEpisode) string {
	var rec models.MediaTransferHistory
	if id := refID(row.Ref); row.Source == models.MediaEpisodeFromLibrary && id > 0 {
		h.find(&rec, "id = ? AND status = ?", id, models.MediaTransferDone)
	}
	if rec.ID == 0 {
		q := h.s.cfg.DB.WithContext(h.ctx).Where("status = ? AND media_type = ? AND tmdb_id = ?", models.MediaTransferDone, e.MediaType, e.TMDBID)
		if e.MediaType == models.MediaKindTV {
			q = q.Where("season = ? AND episode > 0 AND episode <= ? AND MAX(episode_end, episode) >= ?", e.Season, e.Episode, e.Episode)
		}
		if err := q.Order("updated_at DESC, id DESC").Limit(1).Find(&rec).Error; err != nil {
			h.s.cfg.Logger.Debugf("[订阅] 首页读取整理记录失败: %v", err)
		}
	}
	if rec.ID == 0 {
		return ""
	}
	name := rec.TorrentName
	if name == "" {
		name = filepath.Base(rec.SourcePath)
	}
	return versionLabel(h.s.parse(h.ctx, name, ""))
}

// openURL 是在媒体服务器里打开这一集（电影是这一部）的链接：台账记着服务器里的一项时用它，否则按索引找。
func (h *homeLookup) openURL(e CalendarEntry, row models.MediaSubscriptionEpisode) string {
	db := h.s.cfg.DB.WithContext(h.ctx)
	if e.MediaType != models.MediaKindTV {
		list, err := serverindex.Lookup(h.ctx, db, e.MediaType, e.TMDBID)
		if err != nil {
			return ""
		}
		for _, p := range list {
			if p.ItemID != "" {
				return serverindex.OpenURL(p.Server, p.ItemID)
			}
		}
		return ""
	}
	if row.Source == models.MediaEpisodeFromServer {
		if id, item, ok := strings.Cut(row.Ref, ":"); ok {
			var srv models.MediaServer
			if h.find(&srv, "id = ? AND enabled = ?", refID(id), true); srv.ID > 0 {
				return serverindex.OpenURL(srv, item)
			}
		}
	}
	key := [2]int{e.TMDBID, e.Season}
	hits, ok := h.seasons[key]
	if !ok {
		hits, _ = serverindex.SeasonEpisodes(h.ctx, db, e.TMDBID, e.Season, false)
		h.seasons[key] = hits
	}
	if hit, ok := hits[e.Episode]; ok {
		return serverindex.OpenURL(hit.Server, hit.ItemID)
	}
	return ""
}

// progress 填下载中的站点、进度与预计时间：台账记着订阅的种子，下载器里的进度现读（10 秒以内的缓存）。
func (h *homeLookup) progress(it *TodayItem, row models.MediaSubscriptionEpisode) {
	id := refID(row.Ref)
	if id == 0 {
		return
	}
	var t models.MediaSubscriptionTorrent
	if h.find(&t, "id = ?", id); t.ID == 0 {
		return
	}
	if !strings.HasPrefix(t.SiteName, models.MediaAdoptedSitePrefix) {
		it.Site = t.SiteName
	}
	if t.DownloaderID == 0 || t.InfoHash == "" {
		return
	}
	list, err := h.s.downloaderTorrentsWithin(h.ctx, t.DownloaderID, itemListMaxAge, nil)
	if err != nil {
		return
	}
	for _, d := range list {
		if strings.EqualFold(d.InfoHash, t.InfoHash) {
			it.Percent = min(max(d.Progress, 0), 1) * 100
			if d.ETA > 0 {
				it.ETASeconds = d.ETA
			}
			return
		}
	}
}
