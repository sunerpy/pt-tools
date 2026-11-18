package servarr

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

// Sonarr：追剧。series 的 id 是 TMDB 剧集编号，一部剧的每一季是一条独立的季订阅。Seerr 读的是 id、title、titleSlug、tvdbId、tags、
// monitored、seasons[].{seasonNumber, monitored, statistics.episodeFileCount, statistics.totalEpisodeCount}。

// sonarrSeason 是 series 里的一季。
type sonarrSeason struct {
	SeasonNumber int          `json:"seasonNumber"`
	Monitored    bool         `json:"monitored"`
	Statistics   *seasonStats `json:"statistics,omitempty"`
}

type seasonStats struct {
	EpisodeFileCount  int     `json:"episodeFileCount"`
	EpisodeCount      int     `json:"episodeCount"`
	TotalEpisodeCount int     `json:"totalEpisodeCount"`
	SizeOnDisk        int64   `json:"sizeOnDisk"`
	PercentOfEpisodes float64 `json:"percentOfEpisodes"`
}

type seriesStats struct {
	SeasonCount       int      `json:"seasonCount"`
	EpisodeFileCount  int      `json:"episodeFileCount"`
	EpisodeCount      int      `json:"episodeCount"`
	TotalEpisodeCount int      `json:"totalEpisodeCount"`
	SizeOnDisk        int64    `json:"sizeOnDisk"`
	ReleaseGroups     []string `json:"releaseGroups"`
	PercentOfEpisodes float64  `json:"percentOfEpisodes"`
}

// sonarrSeries 是 Sonarr 的 series。id 为 0 时不写（查找的结果里没有季订阅的）。
type sonarrSeries struct {
	ID                int            `json:"id,omitempty"`
	Title             string         `json:"title"`
	SortTitle         string         `json:"sortTitle"`
	TitleSlug         string         `json:"titleSlug"`
	TVDBID            int            `json:"tvdbId"`
	TMDBID            int            `json:"tmdbId"`
	IMDbID            string         `json:"imdbId"`
	Year              int            `json:"year"`
	Overview          string         `json:"overview"`
	Status            string         `json:"status"`
	Monitored         bool           `json:"monitored"`
	MonitorNewItems   string         `json:"monitorNewItems"`
	SeasonCount       int            `json:"seasonCount"`
	Seasons           []sonarrSeason `json:"seasons"`
	Statistics        seriesStats    `json:"statistics"`
	QualityProfileID  uint           `json:"qualityProfileId"`
	LanguageProfileID int            `json:"languageProfileId"`
	SeasonFolder      bool           `json:"seasonFolder"`
	SeriesType        string         `json:"seriesType"`
	Path              string         `json:"path"`
	RootFolderPath    string         `json:"rootFolderPath"`
	Added             string         `json:"added"`
	Tags              []uint         `json:"tags"`
	Images            []image        `json:"images"`
	RemotePoster      string         `json:"remotePoster,omitempty"`
}

// seriesMeta 是拼 series 要的 TMDB 资料：TVDB 编号、各季，以及没有订阅时的名字、年份、IMDb、海报与简介。
type seriesMeta struct {
	tvdbID        int
	seasons       []tmdb.SeasonSummary
	title, imdbID string
	year          int
	poster        string
	overview      string
}

// meta 取一部剧的 TMDB 资料：TVDB 编号（knownTVDB 大于 0 时不再取）与各季；withDetails 为真时另取名字等（没有订阅时用）。
func (s *Server) meta(ctx context.Context, tmdbID, knownTVDB int, withDetails bool) (seriesMeta, error) {
	c, err := s.tmdb(ctx)
	if err != nil {
		return seriesMeta{}, err
	}
	out := seriesMeta{tvdbID: knownTVDB}
	if out.tvdbID <= 0 || withDetails {
		x, xerr := c.Extras(ctx, tmdb.KindTV, tmdbID)
		if xerr != nil {
			return seriesMeta{}, xerr
		}
		if out.tvdbID <= 0 {
			out.tvdbID = x.TVDBID
		}
		out.imdbID = x.IMDbID
	}
	if out.seasons, err = c.Seasons(ctx, tmdbID); err != nil {
		return seriesMeta{}, err
	}
	if withDetails {
		d, derr := c.Details(ctx, tmdb.KindTV, tmdbID)
		if derr != nil {
			return seriesMeta{}, derr
		}
		out.title, out.year, out.poster, out.overview = d.Title, d.Year, d.PosterPath, d.Overview
		if d.IMDbID != "" {
			out.imdbID = d.IMDbID
		}
	}
	return out, nil
}

// representative 是 series 级的设置取哪条季订阅：在找的里编号最大的，没有在找的时编号最大的。
func representative(subs []models.MediaSubscription) models.MediaSubscription {
	var best models.MediaSubscription
	for _, sub := range subs {
		active, bestActive := sub.Status == models.MediaSubActive, best.Status == models.MediaSubActive
		if best.ID == 0 || (active && !bestActive) || (active == bestActive && sub.ID > best.ID) {
			best = sub
		}
	}
	return best
}

// seriesOf 拼一个 series：seasons 列出 TMDB 上的全部季（另加 TMDB 上没有、但有季订阅的），季订阅在找的 monitored 为真；
// episodeFileCount 是这一季整理好的集数，totalEpisodeCount 是 TMDB 上的集数（手动填了集数的季订阅用它），
// episodeCount 是已播出的集数（有季订阅时按台账，没有时按这一季的开播日期算全部或 0）。
func (s *Server) seriesOf(ctx context.Context, tmdbID int, subs []models.MediaSubscription, m seriesMeta, libs []models.MediaLibrary) (sonarrSeries, error) {
	have, err := s.deps.Subs.LibrarySeasons(ctx, tmdbID)
	if err != nil {
		return sonarrSeries{}, err
	}
	bySeason := map[int]models.MediaSubscription{}
	for _, sub := range subs {
		bySeason[sub.Season] = sub
	}
	list := slices.Clone(m.seasons)
	for _, sub := range subs {
		if !slices.ContainsFunc(list, func(ss tmdb.SeasonSummary) bool { return ss.Number == sub.Season }) {
			list = append(list, tmdb.SeasonSummary{Number: sub.Season, EpisodeCount: sub.TotalEpisodes})
		}
	}
	slices.SortStableFunc(list, func(a, b tmdb.SeasonSummary) int { return cmp.Compare(a.Number, b.Number) })
	today := s.deps.Now().Format(time.DateOnly)
	out := sonarrSeries{
		ID: tmdbID, TitleSlug: strconv.Itoa(tmdbID), TVDBID: m.tvdbID, TMDBID: tmdbID, Status: "continuing",
		LanguageProfileID: 1, SeasonFolder: true, SeriesType: "standard", Seasons: []sonarrSeason{},
		Statistics: seriesStats{ReleaseGroups: []string{}}, Tags: s.tagsOf(ctx, tagKindSeries, tmdbID), MonitorNewItems: "none",
	}
	firstYear := 0
	for _, ss := range list {
		sub, subscribed := bySeason[ss.Number]
		st := seasonStats{TotalEpisodeCount: ss.EpisodeCount, EpisodeFileCount: have[ss.Number].Episodes, SizeOnDisk: have[ss.Number].Size}
		if subscribed && sub.TotalEpisodesManual && sub.TotalEpisodes > 0 {
			st.TotalEpisodeCount = sub.TotalEpisodes
		}
		switch {
		case subscribed:
			st.EpisodeCount = min(s.deps.Subs.AiredEpisodes(ctx, &sub), st.TotalEpisodeCount)
		case ss.AirDate != "" && ss.AirDate <= today:
			st.EpisodeCount = st.TotalEpisodeCount
		}
		if st.EpisodeCount > 0 {
			st.PercentOfEpisodes = min(float64(st.EpisodeFileCount)*100/float64(st.EpisodeCount), 100)
		}
		monitored := subscribed && sub.Status == models.MediaSubActive
		out.Seasons = append(out.Seasons, sonarrSeason{SeasonNumber: ss.Number, Monitored: monitored, Statistics: &st})
		out.Monitored = out.Monitored || monitored
		if ss.Number <= 0 {
			continue
		}
		out.SeasonCount++
		out.Statistics.EpisodeFileCount += st.EpisodeFileCount
		out.Statistics.EpisodeCount += st.EpisodeCount
		out.Statistics.TotalEpisodeCount += st.TotalEpisodeCount
		out.Statistics.SizeOnDisk += st.SizeOnDisk
		if y := yearOf(ss.AirDate); y > 0 && (firstYear == 0 || y < firstYear) {
			firstYear = y
		}
	}
	out.Statistics.SeasonCount = out.SeasonCount
	if out.Statistics.EpisodeCount > 0 {
		out.Statistics.PercentOfEpisodes = min(float64(out.Statistics.EpisodeFileCount)*100/float64(out.Statistics.EpisodeCount), 100)
	}
	if len(subs) == 0 {
		out.ID = 0
		out.Title, out.IMDbID, out.Year, out.Overview = m.title, m.imdbID, m.year, m.overview
		out.Added, out.Path, out.RootFolderPath = timeText(time.Time{}), "", ""
		out.Images, out.RemotePoster = posterImages(m.poster), tmdb.PosterURL(m.poster, "")
	} else {
		rep := representative(subs)
		added := subs[0].CreatedAt
		for _, sub := range subs {
			if sub.CreatedAt.Before(added) {
				added = sub.CreatedAt
			}
		}
		out.Title, out.IMDbID, out.Added = rep.Title, rep.IMDbID, timeText(added)
		if out.IMDbID == "" {
			out.IMDbID = m.imdbID
		}
		out.QualityProfileID = servarrProfile(rep.ProfileID)
		out.RootFolderPath = rootPathOf(libs, rep.LibraryID)
		out.Path = out.RootFolderPath
		if rep.FollowNextSeason {
			out.MonitorNewItems = "all"
		}
		out.Year = firstYear
		if out.Year == 0 {
			out.Year = rep.Year
		}
		out.Images, out.RemotePoster = posterImages(rep.PosterPath), tmdb.PosterURL(rep.PosterPath, "")
	}
	out.SortTitle = strings.ToLower(out.Title)
	return out, nil
}

func yearOf(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, _ := strconv.Atoi(date[:4])
	return y
}

// groupByShow 把季订阅按 TMDB 编号分组，按每部剧最早的订阅排。
func groupByShow(subs []models.MediaSubscription) ([]int, map[int][]models.MediaSubscription) {
	var order []int
	groups := map[int][]models.MediaSubscription{}
	for _, sub := range subs {
		if _, ok := groups[sub.TMDBID]; !ok {
			order = append(order, sub.TMDBID)
		}
		groups[sub.TMDBID] = append(groups[sub.TMDBID], sub)
	}
	return order, groups
}

// listSeries 是 GET /series（带 tvdbId 时只要这部的，0 或 1 个）：有季订阅的每部剧一个 series。列全部时 TMDB 的资料总共最多等
// listBudget，取不到的剧这次不列、写日志（结果有缓存，下次再列）。
func (s *Server) listSeries(w http.ResponseWriter, r *http.Request, _ *call) {
	ctx := r.Context()
	if v := r.URL.Query().Get("tvdbId"); v != "" {
		s.seriesByTVDB(w, r, v)
		return
	}
	subs, err := s.deps.Subs.SubscriptionsOf(ctx, models.MediaKindTV, 0)
	if err != nil {
		writeSubError(w, err, "追剧")
		return
	}
	libs, err := s.libraries(ctx, models.MediaKindTV)
	if err != nil {
		writeSubError(w, err, "媒体库")
		return
	}
	order, groups := groupByShow(subs)
	bctx, cancel := context.WithTimeout(ctx, listBudget)
	defer cancel()
	out := make([]sonarrSeries, 0, len(order))
	var skipped []string
	for _, id := range order {
		m, merr := s.meta(bctx, id, 0, false)
		if merr != nil || m.tvdbID <= 0 {
			skipped = append(skipped, groups[id][0].Title)
			continue
		}
		se, serr := s.seriesOf(ctx, id, groups[id], m, libs)
		if serr != nil {
			writeSubError(w, serr, "追剧")
			return
		}
		out = append(out, se)
	}
	if len(skipped) > 0 {
		global.GetSlogger().Warnf("[Servarr] 这次没有列出 %d 部剧（TMDB 上没有 TVDB 编号或暂时取不到资料）: %s", len(skipped), strings.Join(skipped, "、"))
	}
	writeJSON(w, http.StatusOK, out)
}

// findTVDB 把 TVDB 编号换成 TMDB 剧集编号。
func (s *Server) findTVDB(ctx context.Context, tvdbID int) (int, error) {
	c, err := s.tmdb(ctx)
	if err != nil {
		return 0, err
	}
	res, err := c.FindByTVDB(ctx, tvdbID)
	if err != nil {
		return 0, err
	}
	return res.ID, nil
}

// seriesByTVDB 是 GET /series?tvdbId=X。
func (s *Server) seriesByTVDB(w http.ResponseWriter, r *http.Request, v string) {
	ctx := r.Context()
	tvdbID, err := strconv.Atoi(v)
	if err != nil || tvdbID <= 0 {
		writeJSON(w, http.StatusOK, []sonarrSeries{})
		return
	}
	tmdbID, err := s.findTVDB(ctx, tvdbID)
	if errors.Is(err, tmdb.ErrNotFound) {
		writeJSON(w, http.StatusOK, []sonarrSeries{})
		return
	}
	if err != nil {
		writeTMDBError(w, err)
		return
	}
	subs, err := s.deps.Subs.SubscriptionsOf(ctx, models.MediaKindTV, tmdbID)
	if err != nil {
		writeSubError(w, err, "追剧")
		return
	}
	if len(subs) == 0 {
		writeJSON(w, http.StatusOK, []sonarrSeries{})
		return
	}
	se, ok := s.buildSeries(w, ctx, tmdbID, subs, tvdbID)
	if ok {
		writeJSON(w, http.StatusOK, []sonarrSeries{se})
	}
}

// buildSeries 取 TMDB 的资料拼一个有季订阅的 series；出错时写回应并返回 false。
func (s *Server) buildSeries(w http.ResponseWriter, ctx context.Context, tmdbID int, subs []models.MediaSubscription, knownTVDB int) (sonarrSeries, bool) {
	libs, err := s.libraries(ctx, models.MediaKindTV)
	if err != nil {
		writeSubError(w, err, "媒体库")
		return sonarrSeries{}, false
	}
	m, err := s.meta(ctx, tmdbID, knownTVDB, false)
	if err != nil {
		writeTMDBError(w, err)
		return sonarrSeries{}, false
	}
	se, err := s.seriesOf(ctx, tmdbID, subs, m, libs)
	if err != nil {
		writeSubError(w, err, "追剧")
		return sonarrSeries{}, false
	}
	return se, true
}

// showSubs 读一部剧的季订阅；没有时写 404 并返回 false。
func (s *Server) showSubs(w http.ResponseWriter, ctx context.Context, tmdbID int) ([]models.MediaSubscription, bool) {
	subs, err := s.deps.Subs.SubscriptionsOf(ctx, models.MediaKindTV, tmdbID)
	if err != nil {
		writeSubError(w, err, "追剧")
		return nil, false
	}
	if len(subs) == 0 {
		writeError(w, http.StatusNotFound, "没有这部剧的追剧")
		return nil, false
	}
	return subs, true
}

// getSeries 是 GET /series/{id}（TMDB 编号）。
func (s *Server) getSeries(w http.ResponseWriter, r *http.Request, _ *call) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	subs, ok := s.showSubs(w, r.Context(), id)
	if !ok {
		return
	}
	if se, built := s.buildSeries(w, r.Context(), id, subs, 0); built {
		writeJSON(w, http.StatusOK, se)
	}
}

// lookupSeries 是 GET /series/lookup：term 是 tvdb:X 时回一个（有季订阅时带 id），换算不到 TMDB 时回 []；别的 term 按名字在 TMDB 上
// 搜剧集，最多 10 个（TMDB 上没有 TVDB 编号的不列：Seerr 按 TVDB 编号选）。
func (s *Server) lookupSeries(w http.ResponseWriter, r *http.Request, _ *call) {
	ctx := r.Context()
	term := strings.TrimSpace(r.URL.Query().Get("term"))
	if v, ok := strings.CutPrefix(term, "tvdb:"); ok {
		tvdbID, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || tvdbID <= 0 {
			writeJSON(w, http.StatusOK, []sonarrSeries{})
			return
		}
		tmdbID, err := s.findTVDB(ctx, tvdbID)
		if errors.Is(err, tmdb.ErrNotFound) {
			writeJSON(w, http.StatusOK, []sonarrSeries{})
			return
		}
		if err != nil {
			writeTMDBError(w, err)
			return
		}
		se, err := s.lookupOne(ctx, tmdbID, tvdbID)
		if err != nil {
			writeTMDBError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, []sonarrSeries{se})
		return
	}
	if term == "" {
		writeJSON(w, http.StatusOK, []sonarrSeries{})
		return
	}
	c, err := s.tmdb(ctx)
	if err != nil {
		writeTMDBError(w, err)
		return
	}
	res, err := c.Search(ctx, tmdb.KindTV, term, 0)
	if err != nil {
		writeTMDBError(w, err)
		return
	}
	out := []sonarrSeries{}
	for _, it := range res[:min(len(res), 10)] {
		se, lerr := s.lookupOne(ctx, it.ID, 0)
		if lerr != nil {
			writeTMDBError(w, lerr)
			return
		}
		if se.TVDBID > 0 {
			out = append(out, se)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// lookupOne 是查找的一个结果：有季订阅时是那部剧（带 id），没有时按 TMDB 填、不带 id。
func (s *Server) lookupOne(ctx context.Context, tmdbID, knownTVDB int) (sonarrSeries, error) {
	subs, err := s.deps.Subs.SubscriptionsOf(ctx, models.MediaKindTV, tmdbID)
	if err != nil {
		return sonarrSeries{}, err
	}
	libs, err := s.libraries(ctx, models.MediaKindTV)
	if err != nil {
		return sonarrSeries{}, err
	}
	m, err := s.meta(ctx, tmdbID, knownTVDB, len(subs) == 0)
	if err != nil {
		return sonarrSeries{}, err
	}
	return s.seriesOf(ctx, tmdbID, subs, m, libs)
}

// seriesInput 是 POST、PUT /series 的请求体（Seerr 发回整个 series，只读这几项）。
type seriesInput struct {
	ID               int            `json:"id"`
	TVDBID           int            `json:"tvdbId"`
	QualityProfileID uint           `json:"qualityProfileId"`
	RootFolderPath   string         `json:"rootFolderPath"`
	Monitored        *bool          `json:"monitored"`
	Seasons          []sonarrSeason `json:"seasons"`
	Tags             []uint         `json:"tags"`
	AddOptions       struct {
		SearchForMissingEpisodes *bool `json:"searchForMissingEpisodes"`
	} `json:"addOptions"`
}

// seasonPlan 是请求里一季要怎么办，记进审计。
type seasonPlan struct {
	Season int    `json:"season"`
	Action string `json:"action"` // created、resumed、paused、unchanged、skipped
	Reason string `json:"reason,omitempty"`
}

// applySeasons 按 seasons[].monitored 改一部剧的季订阅：为真的没有订阅就建（档案与媒体库用 profile、lib，来源 servarr）、没在找的打开，
// deferred 为假时这些季马上搜（已经在找的也提前到现在），为真时按定时排；为假的在找的暂停。已有季订阅的档案、媒体库、保存路径、
// 分类与标签不改。特别篇（第 0 季）不建。
func (s *Server) applySeasons(ctx context.Context, tmdbID int, subs []models.MediaSubscription, seasons []sonarrSeason, profile, lib uint, paused, deferred bool) ([]seasonPlan, error) {
	bySeason := map[int]models.MediaSubscription{}
	for _, sub := range subs {
		bySeason[sub.Season] = sub
	}
	var plans []seasonPlan
	for _, ss := range seasons {
		sub, ok := bySeason[ss.SeasonNumber]
		switch {
		case ss.SeasonNumber <= 0:
			if ss.Monitored {
				plans = append(plans, seasonPlan{Season: ss.SeasonNumber, Action: "skipped", Reason: "特别篇不建追剧"})
			}
		case ok:
			updated, err := s.deps.Subs.SetMonitored(ctx, sub.ID, ss.Monitored, !deferred)
			if err != nil {
				return plans, err
			}
			action := "unchanged"
			switch {
			case updated.Status != sub.Status && ss.Monitored:
				action = "resumed"
			case updated.Status != sub.Status:
				action = "paused"
			}
			plans = append(plans, seasonPlan{Season: ss.SeasonNumber, Action: action})
		case ss.Monitored:
			in := subscribe.SubscriptionInput{
				MediaType: models.MediaKindTV, TMDBID: tmdbID, Season: ss.SeasonNumber, ProfileID: profile, LibraryID: &lib,
				Paused: paused, Deferred: deferred,
			}
			subscribe.FollowDefaults(&in)
			_, err := s.deps.Subs.CreateSubscription(ctx, in, models.MediaSubFromServarr)
			switch {
			case err == nil:
				plans = append(plans, seasonPlan{Season: ss.SeasonNumber, Action: "created"})
			case errors.Is(err, subscribe.ErrInvalid):
				// 同时建的（已经订阅过了）或 TMDB 上没有这一季：这一季不建，写明原因
				plans = append(plans, seasonPlan{Season: ss.SeasonNumber, Action: "skipped", Reason: strings.TrimPrefix(err.Error(), subscribe.ErrInvalid.Error()+": ")})
			default:
				return plans, err
			}
		}
	}
	return plans, nil
}

// addSeries 是 POST /series：按 TVDB 编号在 TMDB 上找到剧集，monitored 为真的每一季建季订阅（档案按 qualityProfileId，媒体库按
// rootFolderPath，来源 servarr；series 的 monitored 为假时建成暂停，searchForMissingEpisodes 为假时不马上搜）。201，回 series；
// 一季都没有订阅时回 400。
func (s *Server) addSeries(w http.ResponseWriter, r *http.Request, c *call) {
	var in seriesInput
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	c.args["tvdb_id"] = in.TVDBID
	if in.TVDBID <= 0 {
		writeError(w, http.StatusBadRequest, "要指定 tvdbId")
		return
	}
	tmdbID, err := s.findTVDB(ctx, in.TVDBID)
	if errors.Is(err, tmdb.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "TMDB 上找不到 TVDB 编号是 "+strconv.Itoa(in.TVDBID)+" 的剧集")
		return
	}
	if err != nil {
		writeTMDBError(w, err)
		return
	}
	c.args["tmdb_id"] = tmdbID
	profile, err := s.ptProfile(ctx, in.QualityProfileID)
	if err != nil {
		writeSubError(w, err, "质量档案")
		return
	}
	lib, err := s.libraryOf(ctx, models.MediaKindTV, in.RootFolderPath)
	if err != nil {
		writeSubError(w, err, "媒体库")
		return
	}
	subs, err := s.deps.Subs.SubscriptionsOf(ctx, models.MediaKindTV, tmdbID)
	if err != nil {
		writeSubError(w, err, "追剧")
		return
	}
	paused := in.Monitored != nil && !*in.Monitored
	search := in.AddOptions.SearchForMissingEpisodes == nil || *in.AddOptions.SearchForMissingEpisodes
	plans, err := s.applySeasons(ctx, tmdbID, subs, in.Seasons, profile, lib, paused, !search)
	c.args["seasons"], c.args["search"] = plans, search
	if err != nil {
		writeSubError(w, err, "追剧")
		return
	}
	if subs, err = s.deps.Subs.SubscriptionsOf(ctx, models.MediaKindTV, tmdbID); err != nil {
		writeSubError(w, err, "追剧")
		return
	}
	if len(subs) == 0 {
		reasons := make([]string, 0, len(plans))
		for _, p := range plans {
			if p.Reason != "" {
				reasons = append(reasons, "第 "+strconv.Itoa(p.Season)+" 季："+p.Reason)
			}
		}
		msg := "没有可以追的季：要在 seasons 里把要追的季设成 monitored"
		if len(reasons) > 0 {
			msg = "没有建出追剧（" + strings.Join(reasons, "；") + "）"
		}
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if _, err = s.setTags(ctx, tagKindSeries, tmdbID, in.Tags); err != nil {
		writeSubError(w, err, "标签")
		return
	}
	if se, ok := s.buildSeries(w, ctx, tmdbID, subs, in.TVDBID); ok {
		writeJSON(w, http.StatusCreated, se)
	}
}

// updateSeries 是 PUT /series 与 PUT /series/{id}（id 在请求体里，是 TMDB 编号）：seasons[].monitored 为真的季没有订阅就建
// （不马上搜：Seerr 要搜时随后发 MissingEpisodeSearch）、没在找的打开，为假的在找的暂停；已有季的其他设置不改；换标签。200，回 series。
func (s *Server) updateSeries(w http.ResponseWriter, r *http.Request, c *call) {
	var in seriesInput
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	id := in.ID
	if r.PathValue("id") != "" {
		var ok bool
		if id, ok = pathID(w, r); !ok {
			return
		}
	}
	c.args["tmdb_id"] = id
	if id <= 0 {
		writeError(w, http.StatusNotFound, "没有这部剧的追剧")
		return
	}
	subs, ok := s.showSubs(w, ctx, id)
	if !ok {
		return
	}
	var profile, lib uint
	if slices.ContainsFunc(in.Seasons, func(ss sonarrSeason) bool {
		return ss.Monitored && ss.SeasonNumber > 0 && !slices.ContainsFunc(subs, func(sub models.MediaSubscription) bool { return sub.Season == ss.SeasonNumber })
	}) {
		var err error
		if profile, err = s.ptProfile(ctx, in.QualityProfileID); err != nil {
			writeSubError(w, err, "质量档案")
			return
		}
		if lib, err = s.libraryOf(ctx, models.MediaKindTV, in.RootFolderPath); err != nil {
			writeSubError(w, err, "媒体库")
			return
		}
	}
	plans, err := s.applySeasons(ctx, id, subs, in.Seasons, profile, lib, false, true)
	c.args["seasons"] = plans
	if err != nil {
		writeSubError(w, err, "追剧")
		return
	}
	if in.Tags != nil {
		if _, err = s.setTags(ctx, tagKindSeries, id, in.Tags); err != nil {
			writeSubError(w, err, "标签")
			return
		}
	}
	if subs, err = s.deps.Subs.SubscriptionsOf(ctx, models.MediaKindTV, id); err != nil {
		writeSubError(w, err, "追剧")
		return
	}
	if se, built := s.buildSeries(w, ctx, id, subs, in.TVDBID); built {
		writeJSON(w, http.StatusOK, se)
	}
}

// deleteSeries 是 DELETE /series/{id}：删这部剧的全部季订阅，忽略 deleteFiles（不删任何文件或种子）。200 {}；没有时 404。
func (s *Server) deleteSeries(w http.ResponseWriter, r *http.Request, c *call) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	c.args["tmdb_id"] = id
	subs, ok := s.showSubs(w, r.Context(), id)
	if !ok {
		return
	}
	var deleted []int
	for _, sub := range subs {
		if err := s.deps.Subs.DeleteSubscription(r.Context(), sub.ID); err != nil && !errors.Is(err, subscribe.ErrNotFound) {
			c.args["deleted_seasons"] = deleted
			writeSubError(w, err, "追剧")
			return
		}
		deleted = append(deleted, sub.Season)
	}
	c.args["deleted_seasons"] = deleted
	s.dropTags(r.Context(), tagKindSeries, id)
	writeJSON(w, http.StatusOK, map[string]any{})
}

// sonarrEpisode 是 Sonarr 的 episode。id 是季×10000+集（同一部剧里不重复）。
type sonarrEpisode struct {
	ID                       int    `json:"id"`
	SeriesID                 int    `json:"seriesId"`
	EpisodeFileID            int    `json:"episodeFileId"`
	SeasonNumber             int    `json:"seasonNumber"`
	EpisodeNumber            int    `json:"episodeNumber"`
	Title                    string `json:"title"`
	AirDate                  string `json:"airDate"`
	AirDateUtc               string `json:"airDateUtc"`
	Overview                 string `json:"overview"`
	HasFile                  bool   `json:"hasFile"`
	Monitored                bool   `json:"monitored"`
	AbsoluteEpisodeNumber    int    `json:"absoluteEpisodeNumber"`
	UnverifiedSceneNumbering bool   `json:"unverifiedSceneNumbering"`
}

// listEpisodes 是 GET /episode?seriesId=X：已订阅的季每集一项，hasFile 按台账（已入库），monitored 是季订阅在找且这一集没跳过。
func (s *Server) listEpisodes(w http.ResponseWriter, r *http.Request, _ *call) {
	id, err := strconv.Atoi(r.URL.Query().Get("seriesId"))
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "要指定 seriesId")
		return
	}
	subs, ok := s.showSubs(w, r.Context(), id)
	if !ok {
		return
	}
	slices.SortFunc(subs, func(a, b models.MediaSubscription) int { return cmp.Compare(a.Season, b.Season) })
	out := []sonarrEpisode{}
	for _, sub := range subs {
		eps, eerr := s.deps.Subs.Episodes(r.Context(), sub.ID)
		if eerr != nil {
			writeSubError(w, eerr, "追剧的分集")
			return
		}
		for _, ep := range eps {
			e := sonarrEpisode{
				ID: sub.Season*10000 + ep.Number, SeriesID: id, SeasonNumber: sub.Season, EpisodeNumber: ep.Number,
				Title: ep.Name, AirDate: ep.AirDate, HasFile: ep.State == subscribe.EpisodeLibrary,
				Monitored: sub.Status == models.MediaSubActive && ep.State != subscribe.EpisodeSkipped,
			}
			if ep.AirDate != "" {
				e.AirDateUtc = ep.AirDate + "T00:00:00Z"
			}
			out = append(out, e)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// monitorEpisodes 是 PUT /episode/monitor：pt-tools 按整季追，不支持按集开关，回 202 {}（Seerr 只在把季设成监控以后、
// 为没监控的集调它，季订阅已经在找，结果一样）。
func (s *Server) monitorEpisodes(w http.ResponseWriter, r *http.Request, c *call) {
	var in struct {
		EpisodeIDs []int `json:"episodeIds"`
		Monitored  bool  `json:"monitored"`
	}
	if !decode(w, r, &in) {
		return
	}
	c.args["episodes"], c.args["monitored"], c.args["ignored"] = len(in.EpisodeIDs), in.Monitored, true
	global.GetSlogger().Infof("[Servarr] Seerr 要按集开关监控（%d 集）：pt-tools 按整季追，不支持按集开关，忽略", len(in.EpisodeIDs))
	writeJSON(w, http.StatusAccepted, map[string]any{})
}
