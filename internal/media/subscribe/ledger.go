package subscribe

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/media/cachectl"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 追剧台账：剧集的一季每一集现在的情况，由三种事实按优先级合成——整理记录里已入库的、订阅推送（或认领）的种子在下载的、
// 下载器里别处加的种子（对得上就认领进来，之后和订阅推送的一样看）。台账是派生的缓存，只由 rebuildLedger 写；
// 用户的「跳过这一集」「这一集换版本」只由对应的操作写，重建时保留。

// ledger 是重建出的台账。
type ledger struct {
	// total 是有效总集数，listed 是 TMDB 这一季列出的集数
	total, listed int
	season        *tmdb.Season
	rows          []models.MediaSubscriptionEpisode
	// required 是必需集（起始集以后、没有跳过的），missing 是其中已播、不在库也不在下载的（含等版本的）
	required, missing []int
	// aired 是必需集里已经播出的集数
	aired int
	// listErr 不为空时读不到下载器里的种子：这一轮不推缺的集，免得重复下载正在下载的集
	listErr string
}

func (lg *ledger) row(n int) *models.MediaSubscriptionEpisode {
	if lg == nil {
		return nil
	}
	i, ok := slices.BinarySearchFunc(lg.rows, n, func(r models.MediaSubscriptionEpisode, n int) int { return r.Episode - n })
	if !ok {
		return nil
	}
	return &lg.rows[i]
}

// requiredAired 报告必需集是不是都播出了，而且 TMDB 已经列出了全部的集（有效总集数大于列出的集数时还有没列出的）。
func (lg *ledger) requiredAired() bool {
	return lg != nil && len(lg.required) > 0 && lg.aired == len(lg.required) && lg.total <= lg.listed
}

// seasonRefresh 是追剧多久问一次 TMDB 这一季现在的集列表（不读缓存）：新的一集、播出日期常常是播出前几天才有。
const seasonRefresh = 6 * time.Hour

// seasonFor 取订阅这一季的 TMDB 集列表；fresh 为真且上次不读缓存地问过已经超过 seasonRefresh 时，绕过缓存问一次。
func (s *Service) seasonFor(ctx context.Context, sub *models.MediaSubscription, fresh bool) (*tmdb.Season, error) {
	c, err := s.cfg.Recognizer.TMDB(ctx)
	if err != nil {
		return nil, err
	}
	key := sub.ID
	now := s.cfg.Now()
	if fresh {
		s.seasonMu.Lock()
		last, ok := s.seasonAt[key]
		s.seasonMu.Unlock()
		if !ok || now.Sub(last) >= seasonRefresh {
			season, err := c.Season(cachectl.Bypass(ctx), sub.TMDBID, sub.Season)
			if err == nil {
				s.seasonMu.Lock()
				s.seasonAt[key] = now
				s.seasonMu.Unlock()
				return season, nil
			}
			// 问不到现在的就用缓存里的（下面）
		}
	}
	return c.Season(ctx, sub.TMDBID, sub.Season)
}

// genresFor 是剧集的类型编号（判断是不是综艺；TMDB 详情有缓存）。取不到时为空。
func (s *Service) genresFor(ctx context.Context, sub *models.MediaSubscription) []int {
	c, err := s.cfg.Recognizer.TMDB(ctx)
	if err != nil {
		return nil
	}
	d, err := c.Details(ctx, sub.MediaType, sub.TMDBID)
	if err != nil {
		return nil
	}
	return d.GenreIDs
}

// mapperFor 是这个追剧的集号映射（TMDB 这一季的集列表与用户、AI 设的规则）。
func (s *Service) mapperFor(ctx context.Context, sub *models.MediaSubscription, season *tmdb.Season) *episodeMapper {
	if sub.MediaType != models.MediaKindTV || season == nil {
		return nil
	}
	var rules []models.MediaEpisodeRule
	if err := s.cfg.DB.WithContext(ctx).Where("subscription_id = ?", sub.ID).Find(&rules).Error; err != nil {
		s.cfg.Logger.Warnf("[订阅] 读取集号规则失败: %v", err)
	}
	return newEpisodeMapper(newEpisodeIndex(season, s.genresFor(ctx, sub)), rules)
}

// libraryRows 是整理记录里这一季已入库的集（一集有几个文件时取最新的那条）。
func (s *Service) libraryRows(ctx context.Context, sub *models.MediaSubscription) (map[int]models.MediaTransferHistory, error) {
	var rows []models.MediaTransferHistory
	if err := s.cfg.DB.WithContext(ctx).Select("id", "episode", "episode_end", "torrent_name", "source_path").
		Where("status = ? AND media_type = ? AND tmdb_id = ? AND season = ?", models.MediaTransferDone, models.MediaKindTV, sub.TMDBID, sub.Season).
		Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取整理记录失败: %w", err)
	}
	out := map[int]models.MediaTransferHistory{}
	for _, r := range rows {
		for n := r.Episode; n > 0 && n <= max(r.EpisodeEnd, r.Episode); n++ {
			out[n] = r
		}
	}
	return out, nil
}

// covered 是订阅的一个种子里有的集：整季包推送以后核对过文件列表的按核对的，否则按记下的集数（整季是全部）。
func covered(t models.MediaSubscriptionTorrent) func(n int) bool {
	if t.CoveredEpisodes != "" {
		var eps []int
		if json.Unmarshal([]byte(t.CoveredEpisodes), &eps) == nil {
			return func(n int) bool { return slices.Contains(eps, n) }
		}
	}
	sp := spanOf(t)
	return sp.covers
}

// ---- 下载器里的种子 ----

// listCacheTTL 是下载器的种子列表缓存多久：大客户端列一次要好几秒，每个下载器每分钟最多列一次。
const listCacheTTL = time.Minute

type dlList struct {
	at       time.Time
	torrents []downloader.Torrent
	err      error
}

type dlCache struct {
	mu sync.Mutex
	m  map[uint]dlList
}

// downloaderTorrents 列出下载器里的种子（缓存 listCacheTTL）。
func (s *Service) downloaderTorrents(ctx context.Context, id uint) ([]downloader.Torrent, error) {
	now := s.cfg.Now()
	s.dls.mu.Lock()
	if l, ok := s.dls.m[id]; ok && now.Sub(l.at) < listCacheTTL {
		s.dls.mu.Unlock()
		return l.torrents, l.err
	}
	s.dls.mu.Unlock()
	if s.cfg.Downloaders == nil {
		return nil, fmt.Errorf("下载器服务没有启动")
	}
	var list []downloader.Torrent
	dl, setting, err := s.cfg.Downloaders.Get(ctx, id)
	if err == nil {
		list, err = dl.GetAllTorrents()
		if err != nil {
			err = fmt.Errorf("读不到下载器「%s」里的种子：%w", setting.Name, err)
		}
	} else {
		err = fmt.Errorf("连不上下载器：%w", err)
	}
	s.dls.mu.Lock()
	s.dls.m[id] = dlList{at: now, torrents: list, err: err}
	s.dls.mu.Unlock()
	return list, err
}

// adoptedSite 是认领的下载器里的种子记在订阅种子表里的站点名。
func adoptedSite(downloaderID uint) string {
	return models.MediaAdoptedSitePrefix + strconv.FormatUint(uint64(downloaderID), 10)
}

// adopt 把下载器里别处加的、对得上这个追剧、有还缺的必需集的种子认领进订阅的种子表（Adopted），之后和订阅推送的种子一样
// 参与入库判定、卡住检查、版本锁与媒体库路由；洗版时不删。返回认领的种子；读不到下载器时返回错误（这一轮不推缺的集）。
func (s *Service) adopt(ctx context.Context, sub *models.MediaSubscription, set Settings, mp *episodeMapper, need func(n int) bool, linked []models.MediaSubscriptionTorrent) ([]models.MediaSubscriptionTorrent, error) {
	dlID, err := s.downloaderFor(ctx, sub, set)
	if err != nil {
		return nil, err
	}
	list, err := s.downloaderTorrents(ctx, dlID)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, t := range linked {
		if t.InfoHash != "" {
			known[strings.ToLower(t.InfoHash)] = true
		}
	}
	mt := newMatcher(*sub).withMapper(mp)
	profile := s.profileFor(ctx, sub, set)
	var out []models.MediaSubscriptionTorrent
	for _, t := range list {
		hash := strings.ToLower(t.InfoHash)
		if hash == "" || known[hash] {
			continue
		}
		m := s.parse(ctx, t.Name, "")
		sp, ok, _ := mt.matchMapped(m, v2.TorrentItem{Title: t.Name})
		if !ok {
			continue
		}
		useful := false
		for n := 1; n <= max(sub.TotalEpisodes, sp.End); n++ {
			if sp.covers(n) && need(n) {
				useful = true
				break
			}
		}
		if !useful {
			continue
		}
		now := s.cfg.Now()
		row := models.MediaSubscriptionTorrent{
			SubscriptionID: sub.ID, SiteName: adoptedSite(dlID), TorrentID: hash, InfoHash: hash, Title: truncate(t.Name, 512),
			Score: profile.points(m), Episode: sp.Start, EpisodeEnd: sp.End, Complete: sp.Whole, SizeBytes: t.TotalSize,
			DownloaderID: dlID, Adopted: true, Status: models.MediaSubTorrentDownloading,
			Message: "下载器里已经有这个种子（不是订阅推送的，认领进来）", CreatedAt: now, UpdatedAt: now,
		}
		if err := s.cfg.DB.WithContext(ctx).Create(&row).Error; err != nil {
			s.cfg.Logger.Warnf("[订阅] 认领下载器里的种子 %s 失败: %v", t.Name, err)
			continue
		}
		known[hash] = true
		step(ctx, stepAdopt, "认领了下载器里已有的 %s", t.Name)
		out = append(out, row)
	}
	return out, nil
}

// ---- 重建 ----

type rebuildOpts struct {
	set Settings
	// adopt：认领下载器里的种子（只在后台刷新与评估时，订阅在找资源时）
	adopt bool
	// fresh：按 seasonRefresh 绕过缓存问 TMDB
	fresh bool
}

// rebuildLedger 重建一个追剧的台账并写回（只改变了的行）。电影没有台账，返回 nil。
func (s *Service) rebuildLedger(ctx context.Context, sub *models.MediaSubscription, opt rebuildOpts) (*ledger, error) {
	if sub.MediaType != models.MediaKindTV {
		return nil, nil
	}
	season, err := s.seasonFor(ctx, sub, opt.fresh)
	if err != nil {
		return nil, err
	}
	lg := &ledger{season: season, listed: len(season.Episodes)}
	lg.total = sub.TotalEpisodes
	if !sub.TotalEpisodesManual {
		lg.total = max(lg.listed, sub.TotalEpisodes)
		if lg.total != sub.TotalEpisodes {
			if err = s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("id = ? AND total_episodes_manual = ?", sub.ID, false).
				Update("total_episodes", lg.total).Error; err == nil {
				sub.TotalEpisodes = lg.total
			}
		}
	}
	var existing []models.MediaSubscriptionEpisode
	if err = s.cfg.DB.WithContext(ctx).Where("subscription_id = ?", sub.ID).Order("episode").Find(&existing).Error; err != nil {
		return nil, fmt.Errorf("读取追剧台账失败: %w", err)
	}
	old := map[int]models.MediaSubscriptionEpisode{}
	for _, r := range existing {
		old[r.Episode] = r
	}
	lib, err := s.libraryRows(ctx, sub)
	if err != nil {
		return nil, err
	}
	linked, err := s.torrents(ctx, sub.ID)
	if err != nil {
		return nil, err
	}
	start := max(sub.StartEpisode, 1)
	required := func(n int) bool { return n >= start && n <= lg.total && !old[n].ManualSkip }
	downloadingAt := func(n int) *models.MediaSubscriptionTorrent {
		for i := range linked {
			if linked[i].Status == models.MediaSubTorrentDownloading && covered(linked[i])(n) {
				return &linked[i]
			}
		}
		return nil
	}
	if opt.adopt && sub.Status == models.MediaSubActive && s.cfg.Downloaders != nil {
		need := func(n int) bool {
			_, in := lib[n]
			return required(n) && !in && downloadingAt(n) == nil
		}
		adopted, aerr := s.adopt(ctx, sub, opt.set, s.mapperFor(ctx, sub, season), need, linked)
		if aerr != nil {
			lg.listErr = aerr.Error()
		}
		linked = append(linked, adopted...)
	}
	today := s.cfg.Now().In(time.Local).Format(time.DateOnly)
	now := s.cfg.Now()
	_, _, locked := lockOf(sub)
	window := airWindow(sub)
	lg.rows = make([]models.MediaSubscriptionEpisode, 0, lg.total)
	for n := 1; n <= lg.total; n++ {
		r := models.MediaSubscriptionEpisode{SubscriptionID: sub.ID, Episode: n}
		if o, ok := old[n]; ok {
			r = o
		}
		r.AirDate, r.Name = "", ""
		if e := season.Episode(n); e != nil {
			r.AirDate, r.Name = e.AirDate, truncate(e.Name, 255)
		}
		r.Source, r.Ref, r.VersionGroup, r.VersionResolution = "", "", "", ""
		aired := r.AirDate != "" && r.AirDate <= today
		h, inLib := lib[n]
		t := downloadingAt(n)
		switch {
		case inLib:
			r.State, r.Source, r.Ref = models.MediaEpisodeLibrary, models.MediaEpisodeFromLibrary, strconv.FormatUint(uint64(h.ID), 10)
			title := h.TorrentName
			if title == "" {
				title = h.SourcePath
			}
			m := s.parse(ctx, title, "")
			r.VersionGroup, r.VersionResolution = truncate(m.Group, 64), m.Resolution
		case t != nil:
			r.State, r.Source, r.Ref = models.MediaEpisodeDownloading, models.MediaEpisodeFromSubscription, strconv.FormatUint(uint64(t.ID), 10)
			if t.Adopted && strings.HasPrefix(t.SiteName, models.MediaAdoptedSitePrefix) {
				r.Source = models.MediaEpisodeFromDownloader
			}
			m := s.parse(ctx, t.Title, t.Subtitle)
			r.VersionGroup, r.VersionResolution = truncate(m.Group, 64), m.Resolution
		case !required(n):
			// 起始集以前的与跳过的：已经入库或在下载的照样显示（上面），其余不追
			r.State = models.MediaEpisodeSkipped
		case aired:
			r.State = models.MediaEpisodeMissing
			if at, ok := airTime(sub, r.AirDate); ok && locked && !r.ManualUnlock && now.Sub(at) >= window {
				r.State = models.MediaEpisodeWaitingVersion
			}
		default:
			r.State = models.MediaEpisodeUpcoming
		}
		if r.State == models.MediaEpisodeWaitingVersion {
			if r.WaitingSince == nil {
				r.WaitingSince = &now
			}
		} else {
			r.WaitingSince, r.WaitingNotifiedAt = nil, nil
		}
		if required(n) {
			lg.required = append(lg.required, n)
			if aired {
				lg.aired++
			}
			if r.State == models.MediaEpisodeMissing || r.State == models.MediaEpisodeWaitingVersion {
				lg.missing = append(lg.missing, n)
			}
		}
		lg.rows = append(lg.rows, r)
	}
	if err := s.saveLedger(ctx, sub.ID, old, lg.rows, lg.total); err != nil {
		return nil, err
	}
	return lg, nil
}

// sameRow 报告台账的一行有没有变（不看更新时间与编号）。
func sameRow(a, b models.MediaSubscriptionEpisode) bool {
	tm := func(t *time.Time) int64 {
		if t == nil {
			return 0
		}
		return t.Unix()
	}
	return a.AirDate == b.AirDate && a.Name == b.Name && a.State == b.State && a.Source == b.Source && a.Ref == b.Ref &&
		a.VersionGroup == b.VersionGroup && a.VersionResolution == b.VersionResolution && a.ManualSkip == b.ManualSkip &&
		a.ManualUnlock == b.ManualUnlock && tm(a.WaitingSince) == tm(b.WaitingSince) && tm(a.WaitingNotifiedAt) == tm(b.WaitingNotifiedAt)
}

// saveLedger 写回台账：只写变了的行，删掉超出总集数的行。
func (s *Service) saveLedger(ctx context.Context, subID uint, old map[int]models.MediaSubscriptionEpisode, rows []models.MediaSubscriptionEpisode, total int) error {
	now := s.cfg.Now()
	return s.cfg.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range rows {
			r := &rows[i]
			o, ok := old[r.Episode]
			if ok && sameRow(o, *r) {
				continue
			}
			r.UpdatedAt = now
			if !ok {
				if err := tx.Create(r).Error; err != nil {
					return fmt.Errorf("保存追剧台账失败: %w", err)
				}
				continue
			}
			if err := tx.Model(&models.MediaSubscriptionEpisode{}).Where("id = ?", o.ID).Updates(map[string]any{
				"air_date": r.AirDate, "name": r.Name, "state": r.State, "source": r.Source, "ref": r.Ref,
				"version_group": r.VersionGroup, "version_resolution": r.VersionResolution,
				"waiting_since": r.WaitingSince, "waiting_notified_at": r.WaitingNotifiedAt, "updated_at": now,
			}).Error; err != nil {
				return fmt.Errorf("保存追剧台账失败: %w", err)
			}
		}
		if err := tx.Where("subscription_id = ? AND episode > ?", subID, total).Delete(&models.MediaSubscriptionEpisode{}).Error; err != nil {
			return fmt.Errorf("保存追剧台账失败: %w", err)
		}
		return nil
	})
}

// ledgerRows 读台账（没有时重建一次，不认领下载器里的种子）。
func (s *Service) ledgerRows(ctx context.Context, sub *models.MediaSubscription) (*ledger, error) {
	var rows []models.MediaSubscriptionEpisode
	if err := s.cfg.DB.WithContext(ctx).Where("subscription_id = ?", sub.ID).Order("episode").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取追剧台账失败: %w", err)
	}
	if len(rows) == 0 {
		return s.rebuildLedger(ctx, sub, rebuildOpts{})
	}
	lg := &ledger{total: sub.TotalEpisodes, listed: sub.TotalEpisodes, rows: rows}
	today := s.cfg.Now().In(time.Local).Format(time.DateOnly)
	for _, r := range rows {
		if r.State == models.MediaEpisodeSkipped {
			continue
		}
		lg.required = append(lg.required, r.Episode)
		if r.AirDate != "" && r.AirDate <= today {
			lg.aired++
		}
		if r.State == models.MediaEpisodeMissing || r.State == models.MediaEpisodeWaitingVersion {
			lg.missing = append(lg.missing, r.Episode)
		}
	}
	return lg, nil
}
