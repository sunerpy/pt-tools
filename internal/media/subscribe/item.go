package subscribe

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/media/meta"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 条目详情（媒体中心 M3）：一部电影或剧集的资料（TMDB 详情与演职员）、各季、库里与做种的情况、订阅、关联的种子与推荐；
// 剧集一季每一集的状态另见 SeasonItem。都只读：不重建台账、不写运行记录。

const (
	// itemCast 是详情里最多列几位演员
	itemCast = 15
	// itemRecommendations 是最多列几个推荐
	itemRecommendations = 12
)

// ItemInfo 是条目的资料：TMDB 详情与 Extras 合在一起。
type ItemInfo struct {
	MediaType     string `json:"media_type"`
	TMDBID        int    `json:"tmdb_id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title,omitempty"`
	Year          int    `json:"year,omitempty"`
	// Date 是电影的上映日期、剧集的首播日期
	Date     string   `json:"date,omitempty"`
	Overview string   `json:"overview,omitempty"`
	Genres   []string `json:"genres"`
	// Countries 是出品国家或地区（ISO 3166-1）
	Countries []string `json:"countries"`
	// Runtime 是片长（分钟）：电影是整部，剧集是一集
	Runtime int `json:"runtime,omitempty"`
	// Status 是 TMDB 的状态原文（Returning Series、Ended、Released……）
	Status       string            `json:"status,omitempty"`
	PosterPath   string            `json:"poster_path,omitempty"`
	BackdropPath string            `json:"backdrop_path,omitempty"`
	Rating       float64           `json:"rating,omitempty"`
	Votes        int               `json:"votes,omitempty"`
	IMDbID       string            `json:"imdb_id,omitempty"`
	TVDBID       int               `json:"tvdb_id,omitempty"`
	Studios      []string          `json:"studios"`
	Directors    []string          `json:"directors"`
	Cast         []tmdb.CastMember `json:"cast"`
	// SeasonsCount、EpisodesCount 是剧集的季数与总集数（不算特别篇）
	SeasonsCount  int `json:"seasons_count,omitempty"`
	EpisodesCount int `json:"episodes_count,omitempty"`
}

// ItemSeason 是剧集的一季：Have 是这一季已入库的集数。
type ItemSeason struct {
	Number       int    `json:"n"`
	Name         string `json:"name,omitempty"`
	EpisodeCount int    `json:"episode_count"`
	AirDate      string `json:"air_date,omitempty"`
	PosterPath   string `json:"poster_path,omitempty"`
	Have         int    `json:"have"`
}

// ItemNAS 是「你的 NAS 上」：库里的情况与做种的情况。
type ItemNAS struct {
	Library ItemLibrary `json:"library"`
	Seeding ItemSeeding `json:"seeding"`
}

// ItemLibrary 是库里的情况：剧集按季写入库了几集（只列有入库的季），整理好的文件数、体积与媒体库；电影另写版本。
type ItemLibrary struct {
	Seasons   []ItemSeasonHave `json:"seasons"`
	Files     int              `json:"files"`
	Size      int64            `json:"size"`
	Libraries []string         `json:"libraries"`
	// Version 是电影最新入库的那个文件的版本（分辨率与制作组）
	Version string `json:"version,omitempty"`
	// RecordID 是最新的一条整理记录（详情页的「重新刮削」用它：刮削这条记录所在的电影或剧集）
	RecordID uint `json:"record_id,omitempty"`
}

// ItemSeasonHave 是一季入库了几集（Total 是这一季的集数）。
type ItemSeasonHave struct {
	Number int `json:"n"`
	Have   int `json:"have"`
	Total  int `json:"total"`
}

// ItemSeeding 是做种的情况：在做种的种子数，有 H&R 要求还没做够的个数与其中离做够最近的还差几秒，在做种的种子里最低的分享率。
type ItemSeeding struct {
	Count         int      `json:"count"`
	HRPending     int      `json:"hr_pending"`
	HRLeftSeconds int64    `json:"hr_left_seconds,omitempty"`
	LowestRatio   *float64 `json:"lowest_ratio,omitempty"`
}

// ItemSubscription 是这个条目的一个订阅（剧集每季一个）。Follow 是追剧（按自己的定时搜索）。
type ItemSubscription struct {
	ID     uint             `json:"id"`
	Status string           `json:"status"`
	Season int              `json:"season"`
	Follow bool             `json:"follow"`
	Lock   *MediaSearchLock `json:"lock,omitempty"`
}

// 关联的种子在下载器里的状态。
const (
	ItemTorrentSeeding     = "seeding"
	ItemTorrentDownloading = "downloading"
	ItemTorrentPaused      = "paused"
	ItemTorrentError       = "error"
	// ItemTorrentUnknown 是下载器读不到（连不上）：不知道它现在怎样
	ItemTorrentUnknown = "unknown"
)

// ItemTorrent 是和这个条目有关、还在下载器里的一个种子：推送时说明了这个条目的、订阅下载的、整理进库的。
// Deletable 与 Reason 是能不能删（M3 只看下载中与 H&R）。
type ItemTorrent struct {
	Site      string `json:"site"`
	TorrentID string `json:"torrent_id,omitempty"`
	Title     string `json:"title"`
	// Season 是剧集的季（0 是不知道）；Episodes 是种子里的集（E01–E04、整季），不知道时为空
	Season         int     `json:"season"`
	Episodes       string  `json:"episodes,omitempty"`
	Version        string  `json:"version,omitempty"`
	Size           int64   `json:"size"`
	DownloaderID   uint    `json:"downloader_id"`
	Downloader     string  `json:"downloader"`
	Hash           string  `json:"hash"`
	State          string  `json:"state"`
	Percent        float64 `json:"percent"`
	ETASeconds     int64   `json:"eta_seconds,omitempty"`
	Ratio          float64 `json:"ratio"`
	SeedingSeconds int64   `json:"seeding_seconds"`
	HR             bool    `json:"hr"`
	HRHours        int     `json:"hr_hours,omitempty"`
	Deletable      bool    `json:"deletable"`
	Reason         string  `json:"reason"`
}

// ItemDetail 是条目详情页的数据。Errors 是没取到的部分（演职员、各季、推荐、某个下载器），其余照样给。
type ItemDetail struct {
	Item            ItemInfo           `json:"item"`
	Seasons         []ItemSeason       `json:"seasons"`
	NAS             ItemNAS            `json:"nas"`
	Subscriptions   []ItemSubscription `json:"subscriptions"`
	Torrents        []ItemTorrent      `json:"torrents"`
	Recommendations []ExploreItem      `json:"recommendations"`
	Errors          []string           `json:"errors"`
}

// Item 是一部电影或剧集的详情（见 ItemDetail）。TMDB 上没有这个条目时返回 tmdb.ErrNotFound。
func (s *Service) Item(ctx context.Context, kind string, id int) (*ItemDetail, error) {
	if kind != models.MediaKindMovie && kind != models.MediaKindTV {
		return nil, fmt.Errorf("%w: 类型要选电影或剧集", ErrInvalid)
	}
	if id <= 0 {
		return nil, fmt.Errorf("%w: 要指定 TMDB 编号", ErrInvalid)
	}
	c, err := s.cfg.Recognizer.TMDB(ctx)
	if err != nil {
		return nil, err
	}
	d, err := c.Details(ctx, kind, id)
	if err != nil {
		return nil, fmt.Errorf("取 TMDB 条目失败: %w", err)
	}
	out := &ItemDetail{
		Item:    itemInfo(kind, d),
		Seasons: []ItemSeason{}, Subscriptions: []ItemSubscription{}, Torrents: []ItemTorrent{}, Recommendations: []ExploreItem{},
		Errors: []string{},
	}
	out.NAS.Library = ItemLibrary{Seasons: []ItemSeasonHave{}, Libraries: []string{}}
	if x, xerr := c.Extras(ctx, kind, id); xerr == nil {
		applyExtras(&out.Item, x)
	} else {
		out.Errors = append(out.Errors, "演职员与评分人数没取到："+xerr.Error())
	}

	var subs []models.MediaSubscription
	if err = s.cfg.DB.WithContext(ctx).Where("media_type = ? AND tmdb_id = ?", kind, id).Order("season DESC, id DESC").Find(&subs).Error; err != nil {
		return nil, fmt.Errorf("读取订阅失败: %w", err)
	}
	for i := range subs {
		sub := &subs[i]
		v := ItemSubscription{ID: sub.ID, Status: sub.Status, Season: sub.Season, Follow: isFollow(sub)}
		if g, r, ok := lockOf(sub); ok {
			v.Lock = &MediaSearchLock{Group: g, Resolution: r}
		}
		out.Subscriptions = append(out.Subscriptions, v)
	}

	hist, err := s.itemHistory(ctx, kind, id)
	if err != nil {
		return nil, err
	}
	if err = s.itemLibrary(ctx, out, hist); err != nil {
		return nil, err
	}
	if kind == models.MediaKindTV {
		s.itemSeasons(ctx, c, out, d, subs, hist)
	}

	torrents, errs, err := s.itemTorrents(ctx, kind, id, subs, hist)
	if err != nil {
		return nil, err
	}
	out.Torrents = torrents
	out.Errors = append(out.Errors, errs...)
	out.NAS.Seeding = seedingOf(torrents)

	if recs, rerr := c.Recommendations(ctx, kind, id); rerr != nil {
		out.Errors = append(out.Errors, "推荐没取到："+rerr.Error())
	} else if items, merr := s.markExplore(ctx, kind, recs[:min(len(recs), itemRecommendations)]); merr != nil {
		out.Errors = append(out.Errors, merr.Error())
	} else {
		out.Recommendations = items
	}
	return out, nil
}

func itemInfo(kind string, d *tmdb.Result) ItemInfo {
	return ItemInfo{
		MediaType: kind, TMDBID: d.ID, Title: d.Title, OriginalTitle: d.OriginalTitle, Year: d.Year, Date: d.Date, Overview: d.Overview,
		Genres: orEmpty(d.Genres), Countries: orEmpty(d.Countries), Runtime: d.Runtime, PosterPath: d.PosterPath, BackdropPath: d.BackdropPath,
		Rating: d.VoteAverage, IMDbID: d.IMDbID, Studios: []string{}, Directors: []string{}, Cast: []tmdb.CastMember{}, SeasonsCount: d.Seasons,
	}
}

func applyExtras(info *ItemInfo, x *tmdb.Extras) {
	info.Status, info.Votes, info.TVDBID = x.Status, x.VoteCount, x.TVDBID
	if info.IMDbID == "" {
		info.IMDbID = x.IMDbID
	}
	info.Studios = orEmpty(x.Studios)
	info.Directors = orEmpty(x.Directors)
	info.Cast = x.Cast[:min(len(x.Cast), itemCast)]
	if info.Cast == nil {
		info.Cast = []tmdb.CastMember{}
	}
}

// itemHistory 是这个条目整理好的记录（新的在前）。
func (s *Service) itemHistory(ctx context.Context, kind string, id int) ([]models.MediaTransferHistory, error) {
	var rows []models.MediaTransferHistory
	if err := s.cfg.DB.WithContext(ctx).
		Select("id", "library_id", "season", "episode", "episode_end", "size", "torrent_name", "source_path", "info_hash", "downloader_id", "downloader_name",
			"created_at").
		Where("status = ? AND media_type = ? AND tmdb_id = ?", models.MediaTransferDone, kind, id).
		Order("id DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取整理记录失败: %w", err)
	}
	return rows, nil
}

// historyEpisodes 是整理记录里每一季入库的集（季号 → 集号的集合）。
func historyEpisodes(hist []models.MediaTransferHistory) map[int]map[int]bool {
	out := map[int]map[int]bool{}
	for _, h := range hist {
		if h.Episode <= 0 {
			continue
		}
		if out[h.Season] == nil {
			out[h.Season] = map[int]bool{}
		}
		for n := h.Episode; n <= max(h.EpisodeEnd, h.Episode); n++ {
			out[h.Season][n] = true
		}
	}
	return out
}

// itemLibrary 填库里的情况（不含每季的集数，见 itemSeasons）。
func (s *Service) itemLibrary(ctx context.Context, out *ItemDetail, hist []models.MediaTransferHistory) error {
	lib := &out.NAS.Library
	lib.Files = len(hist)
	if len(hist) > 0 {
		lib.RecordID = hist[0].ID
	}
	ids := []uint{}
	for _, h := range hist {
		lib.Size += h.Size
		if h.LibraryID != 0 && !slices.Contains(ids, h.LibraryID) {
			ids = append(ids, h.LibraryID)
		}
	}
	if len(ids) > 0 {
		var names []string
		if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaLibrary{}).Where("id IN ?", ids).Order("id").Pluck("name", &names).Error; err != nil {
			return fmt.Errorf("读取媒体库失败: %w", err)
		}
		lib.Libraries = names
	}
	if out.Item.MediaType == models.MediaKindMovie && len(hist) > 0 {
		lib.Version = versionLabel(s.parse(ctx, historyName(hist[0]), ""))
	}
	return nil
}

// historyName 是整理记录对应的种子名（目录监控的记录没有种子名，用源文件名）。
func historyName(h models.MediaTransferHistory) string {
	if h.TorrentName != "" {
		return h.TorrentName
	}
	return filepath.Base(h.SourcePath)
}

// versionLabel 是给人看的版本：分辨率与制作组（2160p HHWEB）。
func versionLabel(m meta.Meta) string {
	return strings.TrimSpace(m.Resolution + " " + m.Group)
}

// itemSeasons 填剧集的各季与每季入库了几集。TMDB 的季列表取不到（或是空的）时按详情里的季数列出，没有名字与集数。
func (s *Service) itemSeasons(ctx context.Context, c *tmdb.Client, out *ItemDetail, d *tmdb.Result, subs []models.MediaSubscription, hist []models.MediaTransferHistory) {
	have := historyEpisodes(hist)
	list, err := c.Seasons(ctx, d.ID)
	if err != nil {
		out.Errors = append(out.Errors, "各季的列表没取到："+err.Error())
	}
	if len(list) == 0 {
		for n := 1; n <= d.Seasons; n++ {
			list = append(list, tmdb.SeasonSummary{Number: n})
		}
	}
	manual := map[int]int{}
	for _, sub := range subs {
		if sub.TotalEpisodesManual && sub.TotalEpisodes > 0 {
			manual[sub.Season] = sub.TotalEpisodes
		}
	}
	for _, ss := range list {
		total := ss.EpisodeCount
		if m := manual[ss.Number]; m > 0 {
			total = m
		}
		got := len(have[ss.Number])
		out.Seasons = append(out.Seasons, ItemSeason{
			Number: ss.Number, Name: ss.Name, EpisodeCount: total, AirDate: ss.AirDate, PosterPath: ss.PosterPath, Have: got,
		})
		if ss.Number > 0 {
			out.Item.EpisodesCount += total
		}
		if got > 0 {
			out.NAS.Library.Seasons = append(out.NAS.Library.Seasons, ItemSeasonHave{Number: ss.Number, Have: got, Total: total})
		}
	}
	// 特别篇排在最后
	slices.SortStableFunc(out.Seasons, func(a, b ItemSeason) int {
		return cmp.Compare(seasonOrder(a.Number), seasonOrder(b.Number))
	})
	if n := len(list); n > 0 && out.Item.SeasonsCount == 0 {
		out.Item.SeasonsCount = len(slices.DeleteFunc(slices.Clone(list), func(ss tmdb.SeasonSummary) bool { return ss.Number == 0 }))
	}
}

// seasonOrder 是季的排序键：第 0 季（特别篇）排在最后。
func seasonOrder(n int) int {
	if n == 0 {
		return 1 << 20
	}
	return n
}

// torrentRef 是一个关联的种子，在合并来源以后、读下载器以前。
type torrentRef struct {
	site, torrentID, title string
	season                 int
	eps                    []int
	whole                  bool
	size                   int64
	downloaderID           uint
	hash                   string
	hr                     bool
	hrHours                int
	seen                   time.Time // 关联记录里最晚记下的时间（刚记下的，缓存的下载器列表里可能还没有这个种子）
}

// itemTorrents 合并关联的种子（订阅的种子、推送记录、整理记录），按下载器里现在的样子给出状态；不在下载器里的不列。
// 下载器读不到时这些种子的状态是 unknown，第二个返回值写明哪个下载器没读到。
func (s *Service) itemTorrents(ctx context.Context, kind string, id int, subs []models.MediaSubscription, hist []models.MediaTransferHistory) ([]ItemTorrent, []string, error) {
	refs := []*torrentRef{}
	index := map[string]*torrentRef{}
	add := func(r torrentRef) {
		r.hash = strings.ToLower(strings.TrimSpace(r.hash))
		if r.hash == "" || r.downloaderID == 0 {
			return
		}
		key := fmt.Sprintf("%d|%s", r.downloaderID, r.hash)
		old, ok := index[key]
		if !ok {
			cp := r
			refs = append(refs, &cp)
			index[key] = &cp
			return
		}
		if old.site == "" {
			old.site, old.torrentID = r.site, r.torrentID
		}
		if old.title == "" {
			old.title = r.title
		}
		if old.season == 0 {
			old.season = r.season
		}
		if len(old.eps) == 0 && !old.whole {
			old.eps, old.whole = r.eps, r.whole
		}
		old.size = max(old.size, r.size)
		old.hr = old.hr || r.hr
		old.hrHours = max(old.hrHours, r.hrHours)
		if r.seen.After(old.seen) {
			old.seen = r.seen
		}
	}

	for _, sub := range subs {
		rows, err := s.torrents(ctx, sub.ID)
		if err != nil {
			return nil, nil, err
		}
		for _, t := range rows {
			site := t.SiteName
			if strings.HasPrefix(site, models.MediaAdoptedSitePrefix) {
				site = ""
			}
			r := torrentRef{
				site: site, torrentID: t.TorrentID, title: t.Title, season: sub.Season, size: t.SizeBytes,
				downloaderID: t.DownloaderID, hash: t.InfoHash, hr: t.HasHR, whole: t.Complete && t.CoveredEpisodes == "", seen: t.CreatedAt,
			}
			if site == "" {
				r.torrentID = ""
			}
			if !r.whole {
				for n := 1; n <= max(sub.TotalEpisodes, t.EpisodeEnd, t.Episode); n++ {
					if covered(t)(n) {
						r.eps = append(r.eps, n)
					}
				}
			}
			add(r)
		}
	}

	pushed, err := s.itemPushed(ctx, kind, id)
	if err != nil {
		return nil, nil, err
	}
	for _, p := range pushed {
		r := torrentRef{
			site: p.site, torrentID: p.torrentID, title: p.title, season: p.season, size: p.size, downloaderID: p.downloaderID,
			hash: p.hash, hr: p.hr, hrHours: p.hrHours, seen: p.seen,
		}
		m := s.parse(ctx, p.title, "")
		switch {
		case m.Episode > 0:
			for n := m.Episode; n <= max(m.EpisodeEnd, m.Episode); n++ {
				r.eps = append(r.eps, n)
			}
		case kind == models.MediaKindTV && m.Complete:
			r.whole = true
		}
		add(r)
	}

	// 整理记录：一个种子整理出几个文件就有几条，集号合在一起
	byHash := map[string]*torrentRef{}
	order := []string{}
	for _, h := range hist {
		if h.InfoHash == "" || h.DownloaderID == 0 {
			continue
		}
		key := fmt.Sprintf("%d|%s", h.DownloaderID, strings.ToLower(h.InfoHash))
		r, ok := byHash[key]
		if !ok {
			r = &torrentRef{title: h.TorrentName, season: h.Season, downloaderID: h.DownloaderID, hash: h.InfoHash}
			byHash[key] = r
			order = append(order, key)
		}
		r.size += h.Size
		if h.CreatedAt.After(r.seen) {
			r.seen = h.CreatedAt
		}
		for n := h.Episode; n > 0 && n <= max(h.EpisodeEnd, h.Episode); n++ {
			if !slices.Contains(r.eps, n) {
				r.eps = append(r.eps, n)
			}
		}
	}
	for _, key := range order {
		r := byHash[key]
		slices.Sort(r.eps)
		add(*r)
	}

	return s.liveTorrents(ctx, refs)
}

// pushedRef 是一条推送记录（活动表或归档表）。
type pushedRef struct {
	site, torrentID, title string
	season                 int
	size                   int64
	downloaderID           uint
	hash                   string
	hr                     bool
	hrHours                int
	seen                   time.Time // 记下与推送的时间里晚的那个
}

// itemPushed 是推送时说明了这个条目的种子记录：先活动表，再归档表（种子完成 14 天后归档，照样在下载器里做种）。
func (s *Service) itemPushed(ctx context.Context, kind string, id int) ([]pushedRef, error) {
	var live []models.TorrentInfo
	if err := s.cfg.DB.WithContext(ctx).
		Select("site_name", "torrent_id", "title", "torrent_hash", "downloader_id", "torrent_size", "has_hr", "hr_seed_time_h", "media_season",
			"push_time", "created_at").
		Where("media_type = ? AND tmdb_id = ? AND is_pushed = ?", kind, id, true).Order("id").Find(&live).Error; err != nil {
		return nil, fmt.Errorf("读取种子记录失败: %w", err)
	}
	var archived []models.TorrentInfoArchive
	if s.cfg.DB.Migrator().HasTable(&models.TorrentInfoArchive{}) {
		if err := s.cfg.DB.WithContext(ctx).
			Select("site_name", "torrent_id", "title", "torrent_hash", "downloader_id", "torrent_size", "has_hr", "hr_seed_time_h", "media_season",
				"push_time").
			Where("media_type = ? AND tmdb_id = ? AND is_pushed = ?", kind, id, true).Order("id").Find(&archived).Error; err != nil {
			return nil, fmt.Errorf("读取归档的种子记录失败: %w", err)
		}
	}
	out := make([]pushedRef, 0, len(live)+len(archived))
	deref := func(h *string, d *uint) (string, uint) {
		var hash string
		var dl uint
		if h != nil {
			hash = *h
		}
		if d != nil {
			dl = *d
		}
		return hash, dl
	}
	for _, t := range live {
		hash, dl := deref(t.TorrentHash, t.DownloaderID)
		out = append(out, pushedRef{
			site: t.SiteName, torrentID: t.TorrentID, title: t.Title, season: t.MediaSeason, size: t.TorrentSize,
			downloaderID: dl, hash: hash, hr: t.HasHR, hrHours: t.HRSeedTimeH, seen: recordSeen(t.CreatedAt, t.PushTime),
		})
	}
	for _, t := range archived {
		hash, dl := deref(t.TorrentHash, t.DownloaderID)
		out = append(out, pushedRef{
			site: t.SiteName, torrentID: t.TorrentID, title: t.Title, season: t.MediaSeason, size: t.TorrentSize,
			downloaderID: dl, hash: hash, hr: t.HasHR, hrHours: t.HRSeedTimeH, seen: recordSeen(time.Time{}, t.PushTime),
		})
	}
	return out, nil
}

// recordSeen 是种子记录记下与推送的时间里晚的那个（RSS 先记下、后推送）。
func recordSeen(created time.Time, pushed *time.Time) time.Time {
	if pushed != nil && pushed.After(created) {
		return *pushed
	}
	return created
}

// liveTorrents 按下载器里现在的样子给出关联的种子：不在下载器里的不列；读不到下载器时状态是 unknown。
func (s *Service) liveTorrents(ctx context.Context, refs []*torrentRef) ([]ItemTorrent, []string, error) {
	out := []ItemTorrent{}
	if len(refs) == 0 {
		return out, nil, nil
	}
	ids := []uint{}
	for _, r := range refs {
		if !slices.Contains(ids, r.downloaderID) {
			ids = append(ids, r.downloaderID)
		}
	}
	recent := recentHashes(refs, func(r *torrentRef) (uint, string, time.Time) { return r.downloaderID, r.hash, r.seen })
	var settings []models.DownloaderSetting
	if err := s.cfg.DB.WithContext(ctx).Select("id", "name").Where("id IN ?", ids).Find(&settings).Error; err != nil {
		return nil, nil, fmt.Errorf("读取下载器失败: %w", err)
	}
	names := map[uint]string{}
	for _, d := range settings {
		names[d.ID] = d.Name
	}
	lists := map[uint]map[string]downloader.Torrent{}
	failed := map[uint]bool{}
	var errs []string
	for _, id := range ids {
		list, err := s.downloaderTorrentsWithin(ctx, id, itemListMaxAge, recent[id])
		if err != nil {
			failed[id] = true
			name := names[id]
			if name == "" {
				name = fmt.Sprintf("编号 %d", id)
			}
			errs = append(errs, fmt.Sprintf("下载器「%s」没读到：%v", name, err))
			continue
		}
		m := make(map[string]downloader.Torrent, len(list))
		for _, t := range list {
			m[strings.ToLower(t.InfoHash)] = t
		}
		lists[id] = m
	}
	for _, r := range refs {
		v := ItemTorrent{
			Site: r.site, TorrentID: r.torrentID, Title: r.title, Season: r.season, Episodes: spanLabel(r.eps, r.whole), Size: r.size,
			DownloaderID: r.downloaderID, Downloader: names[r.downloaderID], Hash: r.hash, HR: r.hr, HRHours: r.hrHours,
		}
		if v.HR && v.HRHours == 0 && r.site != "" {
			_, def := siteHR(r.site)
			v.HRHours = ptinternal.HRSeedTimeH(r.site, def, r.size)
		}
		if failed[r.downloaderID] {
			v.State, v.Reason = ItemTorrentUnknown, "下载器没读到，不知道能不能删"
		} else {
			t, ok := lists[r.downloaderID][r.hash]
			if !ok {
				continue
			}
			v.State = liveState(t)
			v.Percent = min(max(t.Progress, 0), 1) * 100
			v.Ratio, v.SeedingSeconds = t.Ratio, t.SeedingTime
			if v.State == ItemTorrentDownloading && t.ETA > 0 {
				v.ETASeconds = t.ETA
			}
			if v.Size == 0 {
				v.Size = t.TotalSize
			}
			if v.Title == "" {
				v.Title = t.Name
			}
			v.Deletable, v.Reason = deletable(v, t.IsCompleted || t.Progress >= 1)
		}
		v.Version = versionLabel(s.parse(ctx, v.Title, ""))
		out = append(out, v)
	}
	slices.SortStableFunc(out, func(a, b ItemTorrent) int {
		return cmp.Or(cmp.Compare(seasonOrder(a.Season), seasonOrder(b.Season)), cmp.Compare(a.Episodes, b.Episodes),
			cmp.Compare(strings.ToLower(a.Site), strings.ToLower(b.Site)), cmp.Compare(a.Title, b.Title))
	})
	return out, errs, nil
}

// liveState 是下载器里种子的状态：出错、暂停、下载中、做种。
func liveState(t downloader.Torrent) string {
	switch {
	case t.State == downloader.TorrentError:
		return ItemTorrentError
	case t.State == downloader.TorrentPaused || t.State == downloader.TorrentStopped:
		return ItemTorrentPaused
	case !t.IsCompleted && t.Progress < 1:
		return ItemTorrentDownloading
	}
	return ItemTorrentSeeding
}

// deletable 是能不能删（M3 只看两条）：没下完的受保护；有 H&R 要求的做种时间要做够（要求不明的受保护）。
func deletable(v ItemTorrent, completed bool) (bool, string) {
	switch {
	case !completed:
		return false, "受保护：下载中"
	case v.HR && v.HRHours <= 0:
		return false, "受保护：有 H&R，要求不明"
	case v.HR:
		if left := int64(v.HRHours)*3600 - v.SeedingSeconds; left > 0 {
			return false, "H&R 还差 " + leftText(left)
		}
		return true, "H&R 已满足"
	}
	return true, "可以删"
}

// leftText 是还差多久：不到一天写小时（不足一小时按一小时），一天以上按天（四舍五入）。
func leftText(seconds int64) string {
	hours := (seconds + 3599) / 3600
	if hours < 24 {
		return fmt.Sprintf("%d 小时", hours)
	}
	return fmt.Sprintf("%d 天", (hours+12)/24)
}

// seedingOf 是「你的 NAS 上」做种那一行。
func seedingOf(list []ItemTorrent) ItemSeeding {
	var out ItemSeeding
	for _, t := range list {
		if t.State == ItemTorrentSeeding {
			out.Count++
			if out.LowestRatio == nil || t.Ratio < *out.LowestRatio {
				r := t.Ratio
				out.LowestRatio = &r
			}
		}
		if t.State != ItemTorrentDownloading && t.HR && t.HRHours > 0 {
			if left := int64(t.HRHours)*3600 - t.SeedingSeconds; left > 0 {
				out.HRPending++
				if out.HRLeftSeconds == 0 || left < out.HRLeftSeconds {
					out.HRLeftSeconds = left
				}
			}
		}
	}
	return out
}

// spanLabel 是种子里的集：整季，或连着的写成 E01–E04、不连着的用顿号隔开；不知道时为空。
func spanLabel(eps []int, whole bool) string {
	if whole {
		return "整季"
	}
	if len(eps) == 0 {
		return ""
	}
	eps = slices.Clone(eps)
	slices.Sort(eps)
	eps = slices.Compact(eps)
	var parts []string
	for i := 0; i < len(eps); {
		j := i
		for j+1 < len(eps) && eps[j+1] == eps[j]+1 {
			j++
		}
		if j > i {
			parts = append(parts, fmt.Sprintf("E%02d–E%02d", eps[i], eps[j]))
		} else {
			parts = append(parts, fmt.Sprintf("E%02d", eps[i]))
		}
		i = j + 1
	}
	return strings.Join(parts, "、")
}

// markExplore 给 TMDB 的结果标上已入库（整理记录里有）与已订阅（剧集是任意一季，SubscriptionID 是其中最新的一个）。
func (s *Service) markExplore(ctx context.Context, kind string, results []tmdb.Result) ([]ExploreItem, error) {
	ids := make([]int, 0, len(results))
	for _, r := range results {
		ids = append(ids, r.ID)
	}
	inLib := map[int]bool{}
	subs := map[int]uint{}
	if len(ids) > 0 {
		var lib []int
		if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).
			Where("status = ? AND media_type = ? AND tmdb_id IN ?", models.MediaTransferDone, kind, ids).Distinct().Pluck("tmdb_id", &lib).Error; err != nil {
			return nil, fmt.Errorf("读取整理记录失败: %w", err)
		}
		for _, id := range lib {
			inLib[id] = true
		}
		var rows []models.MediaSubscription
		if err := s.cfg.DB.WithContext(ctx).Select("id", "tmdb_id").Where("media_type = ? AND tmdb_id IN ?", kind, ids).Order("id").Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("读取订阅失败: %w", err)
		}
		for _, r := range rows {
			subs[r.TMDBID] = r.ID
		}
	}
	out := make([]ExploreItem, 0, len(results))
	for _, r := range results {
		out = append(out, ExploreItem{Result: r, InLibrary: inLib[r.ID], Subscribed: subs[r.ID] != 0, SubscriptionID: subs[r.ID]})
	}
	return out, nil
}

// ---- 一季的每一集 ----

// SeasonSubscription 是这一季的订阅：Airtime 为真时追剧按播出时间搜，SearchDelayHours 是播出后几小时开始搜。
type SeasonSubscription struct {
	ID               uint             `json:"id"`
	Status           string           `json:"status"`
	Follow           bool             `json:"follow"`
	Lock             *MediaSearchLock `json:"lock,omitempty"`
	Airtime          bool             `json:"airtime,omitempty"`
	SearchDelayHours int              `json:"search_delay_hours,omitempty"`
}

// SeasonEpisode 是一季里的一集：状态同按条目搜索（已入库、下载中、等版本、缺、没播；按播出时间追剧的，今天还没到播出时刻的写没播）；
// 入库的带版本与体积，下载中的带进度。
type SeasonEpisode struct {
	Episode int    `json:"episode"`
	Name    string `json:"name,omitempty"`
	AirDate string `json:"air_date,omitempty"`
	// AirAt 是按播出时间追剧时这一集的播出时刻
	AirAt      *time.Time `json:"air_at,omitempty"`
	StillPath  string     `json:"still_path,omitempty"`
	Runtime    int        `json:"runtime,omitempty"`
	State      string     `json:"state"`
	Site       string     `json:"site,omitempty"`
	Version    string     `json:"version,omitempty"`
	Size       int64      `json:"size,omitempty"`
	Percent    float64    `json:"percent,omitempty"`
	ETASeconds int64      `json:"eta_seconds,omitempty"`
}

// SeasonDetail 是剧集一季的每一集。Errors 是没取到的部分（某个下载器的进度）。
type SeasonDetail struct {
	TMDBID       int                 `json:"tmdb_id"`
	Season       int                 `json:"season"`
	Name         string              `json:"name,omitempty"`
	AirDate      string              `json:"air_date,omitempty"`
	Subscription *SeasonSubscription `json:"subscription,omitempty"`
	Episodes     []SeasonEpisode     `json:"episodes"`
	Errors       []string            `json:"errors"`
}

// SeasonItem 是剧集一季的每一集（见 SeasonDetail）。这一季订阅了时按订阅（集号规则、有效集数、在下载的种子、等版本），
// 没有时按 TMDB 上的条目拼一份（同按条目搜索，不写库）。
func (s *Service) SeasonItem(ctx context.Context, id, n int) (*SeasonDetail, error) {
	if id <= 0 {
		return nil, fmt.Errorf("%w: 要指定 TMDB 编号", ErrInvalid)
	}
	if n < 0 || n > 200 {
		return nil, fmt.Errorf("%w: 季号不对", ErrInvalid)
	}
	sub, subscribed, err := s.searchSubject(ctx, MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: id, Season: n})
	if err != nil {
		return nil, err
	}
	season, err := s.seasonFor(ctx, sub, false)
	if err != nil {
		return nil, fmt.Errorf("取 TMDB 季详情失败: %w", err)
	}
	total := len(season.Episodes)
	if subscribed && sub.TotalEpisodesManual {
		total = sub.TotalEpisodes
	} else if subscribed {
		total = max(total, sub.TotalEpisodes)
	}
	out := &SeasonDetail{TMDBID: id, Season: n, Name: season.Name, AirDate: season.AirDate, Episodes: []SeasonEpisode{}, Errors: []string{}}
	if subscribed {
		v := &SeasonSubscription{ID: sub.ID, Status: sub.Status, Follow: isFollow(sub)}
		if g, r, ok := lockOf(sub); ok {
			v.Lock = &MediaSearchLock{Group: g, Resolution: r}
		}
		if sub.ScheduleKind == models.MediaScheduleAirtime {
			v.Airtime = true
			v.SearchDelayHours = int(hoursOr(sub.AirDelayHours, defaultAirDelay) / time.Hour)
		}
		out.Subscription = v
	}
	states, err := s.episodeStates(ctx, sub, subscribed, season, total, s.mapperFor(ctx, sub, season))
	if err != nil {
		return nil, err
	}
	now := s.cfg.Now()
	recent := recentHashes(states, func(st episodeState) (uint, string, time.Time) { return st.dl.downloaderID, st.dl.hash, st.dl.seen })
	lists := map[uint]map[string]downloader.Torrent{}
	listFor := func(dl uint) map[string]downloader.Torrent {
		if m, ok := lists[dl]; ok {
			return m
		}
		m := map[string]downloader.Torrent{}
		list, lerr := s.downloaderTorrentsWithin(ctx, dl, itemListMaxAge, recent[dl])
		if lerr != nil {
			out.Errors = append(out.Errors, "下载进度没读到："+lerr.Error())
		}
		for _, t := range list {
			m[strings.ToLower(t.InfoHash)] = t
		}
		lists[dl] = m
		return m
	}
	for _, st := range states {
		e := SeasonEpisode{Episode: st.Episode, Name: st.Name, AirDate: st.AirDate, State: st.State, Site: st.Site}
		if ep := season.Episode(st.Episode); ep != nil {
			e.StillPath, e.Runtime = ep.StillPath, ep.Runtime
		}
		if out.Subscription != nil && out.Subscription.Airtime && e.AirDate != "" {
			if at, ok := airTime(sub, e.AirDate); ok {
				e.AirAt = &at
			}
		}
		e.State = beforeAir(e.State, e.AirAt, now)
		if st.lib != nil {
			e.Version = versionLabel(s.parse(ctx, historyName(*st.lib), ""))
			e.Size = st.lib.Size
		}
		if st.State == models.MediaEpisodeDownloading && st.dl.downloaderID != 0 && st.dl.hash != "" {
			if t, ok := listFor(st.dl.downloaderID)[strings.ToLower(st.dl.hash)]; ok {
				e.Percent = min(max(t.Progress, 0), 1) * 100
				if t.ETA > 0 {
					e.ETASeconds = t.ETA
				}
			}
		}
		out.Episodes = append(out.Episodes, e)
	}
	return out, nil
}
