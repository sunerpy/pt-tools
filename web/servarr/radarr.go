package servarr

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

// Radarr：电影订阅。movie 的 id 是电影订阅的编号；Seerr 读的是 id、title、titleSlug、tmdbId、hasFile、monitored、isAvailable、tags。

// tagKindMovie、tagKindSeries 是标签挂在哪类条目上（servarr_tag_links.kind）。
const (
	tagKindMovie  = models.ServarrKindMovie
	tagKindSeries = models.ServarrKindSeries
)

// image 是海报（Seerr 只在按名字找剧时显示 remotePoster）。
type image struct {
	CoverType string `json:"coverType"`
	URL       string `json:"url"`
	RemoteURL string `json:"remoteUrl"`
}

func posterImages(path string) []image {
	u := tmdb.PosterURL(path, "")
	if u == "" {
		return []image{}
	}
	return []image{{CoverType: "poster", URL: u, RemoteURL: u}}
}

// radarrMovie 是 Radarr 的 movie。id 为 0 时不写（查找的结果里没有订阅的）。
type radarrMovie struct {
	ID                  uint    `json:"id,omitempty"`
	Title               string  `json:"title"`
	OriginalTitle       string  `json:"originalTitle"`
	SortTitle           string  `json:"sortTitle"`
	TitleSlug           string  `json:"titleSlug"`
	TMDBID              int     `json:"tmdbId"`
	IMDbID              string  `json:"imdbId"`
	Year                int     `json:"year"`
	Overview            string  `json:"overview"`
	Monitored           bool    `json:"monitored"`
	HasFile             bool    `json:"hasFile"`
	IsAvailable         bool    `json:"isAvailable"`
	Status              string  `json:"status"`
	MinimumAvailability string  `json:"minimumAvailability"`
	QualityProfileID    uint    `json:"qualityProfileId"`
	ProfileID           uint    `json:"profileId"`
	Path                string  `json:"path"`
	FolderName          string  `json:"folderName"`
	RootFolderPath      string  `json:"rootFolderPath"`
	SizeOnDisk          int64   `json:"sizeOnDisk"`
	Added               string  `json:"added"`
	Tags                []uint  `json:"tags"`
	Images              []image `json:"images"`
	RemotePoster        string  `json:"remotePoster,omitempty"`
}

// movieOf 把电影订阅写成 movie：monitored 是订阅在找，hasFile 是库里有整理好的记录。
func (s *Server) movieOf(ctx context.Context, sub models.MediaSubscription, libs []models.MediaLibrary) (radarrMovie, error) {
	lib, err := s.deps.Subs.MovieLibrary(ctx, sub.TMDBID)
	if err != nil {
		return radarrMovie{}, err
	}
	root := rootPathOf(libs, sub.LibraryID)
	m := radarrMovie{
		ID: sub.ID, Title: sub.Title, OriginalTitle: sub.OriginalTitle, SortTitle: strings.ToLower(sub.Title),
		TitleSlug: strconv.Itoa(sub.TMDBID), TMDBID: sub.TMDBID, IMDbID: sub.IMDbID, Year: sub.Year,
		Monitored: sub.Status == models.MediaSubActive, HasFile: lib.Have, IsAvailable: true, Status: "released",
		MinimumAvailability: "released", QualityProfileID: servarrProfile(sub.ProfileID), ProfileID: servarrProfile(sub.ProfileID),
		Path: lib.Dir, RootFolderPath: root, SizeOnDisk: lib.Size, Added: timeText(sub.CreatedAt),
		Tags: s.tagsOf(ctx, tagKindMovie, sub.TMDBID), Images: posterImages(sub.PosterPath),
	}
	if m.Path == "" {
		m.Path = root
	}
	m.FolderName = filepath.Base(m.Path)
	return m, nil
}

// listMovies 是 GET /movie（带 tmdbId 时只要这部的，0 或 1 个）：全部电影订阅。
func (s *Server) listMovies(w http.ResponseWriter, r *http.Request, _ *call) {
	tmdbID := 0
	if v := r.URL.Query().Get("tmdbId"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeJSON(w, http.StatusOK, []radarrMovie{})
			return
		}
		tmdbID = n
	}
	subs, err := s.deps.Subs.SubscriptionsOf(r.Context(), models.MediaKindMovie, tmdbID)
	if err != nil {
		writeSubError(w, err, "电影订阅")
		return
	}
	libs, err := s.libraries(r.Context(), models.MediaKindMovie)
	if err != nil {
		writeSubError(w, err, "媒体库")
		return
	}
	out := make([]radarrMovie, 0, len(subs))
	for _, sub := range subs {
		m, merr := s.movieOf(r.Context(), sub, libs)
		if merr != nil {
			writeSubError(w, merr, "电影订阅")
			return
		}
		out = append(out, m)
	}
	writeJSON(w, http.StatusOK, out)
}

// movieSub 读电影订阅；没有或不是电影时写 404 并返回 false。
func (s *Server) movieSub(w http.ResponseWriter, ctx context.Context, id int) (models.MediaSubscription, bool) {
	sub, err := s.deps.Subs.SubscriptionRow(ctx, uint(id)) //nolint:gosec // pathID 检查过是正数
	if errors.Is(err, subscribe.ErrNotFound) || (err == nil && sub.MediaType != models.MediaKindMovie) {
		writeError(w, http.StatusNotFound, "没有这个电影订阅")
		return sub, false
	}
	if err != nil {
		writeSubError(w, err, "电影订阅")
		return sub, false
	}
	return sub, true
}

// writeMovie 回一个电影订阅。
func (s *Server) writeMovie(w http.ResponseWriter, ctx context.Context, status int, sub models.MediaSubscription) {
	libs, err := s.libraries(ctx, models.MediaKindMovie)
	if err != nil {
		writeSubError(w, err, "媒体库")
		return
	}
	m, err := s.movieOf(ctx, sub, libs)
	if err != nil {
		writeSubError(w, err, "电影订阅")
		return
	}
	writeJSON(w, status, m)
}

// getMovie 是 GET /movie/{id}。
func (s *Server) getMovie(w http.ResponseWriter, r *http.Request, _ *call) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if sub, found := s.movieSub(w, r.Context(), id); found {
		s.writeMovie(w, r.Context(), http.StatusOK, sub)
	}
}

// lookupMovie 是 GET /movie/lookup?term=tmdb:X：有订阅时回订阅（带 id），没有时按 TMDB 详情填、不带 id，hasFile 按库里有没有。
// term 不是 tmdb: 开头、TMDB 上没有时回 []。
func (s *Server) lookupMovie(w http.ResponseWriter, r *http.Request, _ *call) {
	v, ok := strings.CutPrefix(strings.TrimSpace(r.URL.Query().Get("term")), "tmdb:")
	tmdbID, err := strconv.Atoi(strings.TrimSpace(v))
	if !ok || err != nil || tmdbID <= 0 {
		writeJSON(w, http.StatusOK, []radarrMovie{})
		return
	}
	ctx := r.Context()
	subs, err := s.deps.Subs.SubscriptionsOf(ctx, models.MediaKindMovie, tmdbID)
	if err != nil {
		writeSubError(w, err, "电影订阅")
		return
	}
	if len(subs) > 0 {
		libs, lerr := s.libraries(ctx, models.MediaKindMovie)
		if lerr != nil {
			writeSubError(w, lerr, "媒体库")
			return
		}
		m, merr := s.movieOf(ctx, subs[0], libs)
		if merr != nil {
			writeSubError(w, merr, "电影订阅")
			return
		}
		writeJSON(w, http.StatusOK, []radarrMovie{m})
		return
	}
	d, err := s.details(ctx, tmdb.KindMovie, tmdbID)
	if errors.Is(err, tmdb.ErrNotFound) {
		writeJSON(w, http.StatusOK, []radarrMovie{})
		return
	}
	if err != nil {
		writeTMDBError(w, err)
		return
	}
	lib, err := s.deps.Subs.MovieLibrary(ctx, tmdbID)
	if err != nil {
		writeSubError(w, err, "整理记录")
		return
	}
	writeJSON(w, http.StatusOK, []radarrMovie{{
		Title: d.Title, OriginalTitle: d.OriginalTitle, SortTitle: strings.ToLower(d.Title), TitleSlug: strconv.Itoa(tmdbID),
		TMDBID: tmdbID, IMDbID: d.IMDbID, Year: d.Year, Overview: d.Overview, HasFile: lib.Have, IsAvailable: true,
		Status: "released", MinimumAvailability: "released", Path: lib.Dir, FolderName: filepath.Base(lib.Dir),
		SizeOnDisk: lib.Size, Added: timeText(time.Time{}), Tags: []uint{}, Images: posterImages(d.PosterPath),
		RemotePoster: tmdb.PosterURL(d.PosterPath, ""),
	}})
}

// movieInput 是 POST、PUT /movie 的请求体（Seerr 发回整个 movie，只读这几项）。
type movieInput struct {
	ID               uint   `json:"id"`
	TMDBID           int    `json:"tmdbId"`
	QualityProfileID uint   `json:"qualityProfileId"`
	RootFolderPath   string `json:"rootFolderPath"`
	Monitored        *bool  `json:"monitored"`
	Tags             []uint `json:"tags"`
	AddOptions       struct {
		SearchForMovie *bool `json:"searchForMovie"`
	} `json:"addOptions"`
}

// addMovie 是 POST /movie：建电影订阅（来源 servarr）。档案按 qualityProfileId，媒体库按 rootFolderPath；monitored 为假时建成暂停，
// searchForMovie 为假时不马上搜。已经订阅过时回已有的那个。201，回 movie。
func (s *Server) addMovie(w http.ResponseWriter, r *http.Request, c *call) {
	var in movieInput
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	c.args["tmdb_id"] = in.TMDBID
	if in.TMDBID <= 0 {
		writeError(w, http.StatusBadRequest, "要指定 tmdbId")
		return
	}
	if subs, err := s.deps.Subs.SubscriptionsOf(ctx, models.MediaKindMovie, in.TMDBID); err != nil {
		writeSubError(w, err, "电影订阅")
		return
	} else if len(subs) > 0 {
		c.args["subscription_id"], c.args["existing"] = subs[0].ID, true
		s.writeMovie(w, ctx, http.StatusCreated, subs[0])
		return
	}
	profile, err := s.ptProfile(ctx, in.QualityProfileID)
	if err != nil {
		writeSubError(w, err, "质量档案")
		return
	}
	lib, err := s.libraryOf(ctx, models.MediaKindMovie, in.RootFolderPath)
	if err != nil {
		writeSubError(w, err, "媒体库")
		return
	}
	monitored := in.Monitored == nil || *in.Monitored
	search := in.AddOptions.SearchForMovie == nil || *in.AddOptions.SearchForMovie
	sub, err := s.deps.Subs.CreateSubscription(ctx, subscribe.SubscriptionInput{
		MediaType: models.MediaKindMovie, TMDBID: in.TMDBID, ProfileID: profile, LibraryID: &lib,
		Paused: !monitored, Deferred: !search,
	}, models.MediaSubFromServarr)
	if err != nil {
		// 同时建的：用已有的那条
		if errors.Is(err, subscribe.ErrInvalid) {
			if subs, lerr := s.deps.Subs.SubscriptionsOf(ctx, models.MediaKindMovie, in.TMDBID); lerr == nil && len(subs) > 0 {
				c.args["subscription_id"], c.args["existing"] = subs[0].ID, true
				s.writeMovie(w, ctx, http.StatusCreated, subs[0])
				return
			}
		}
		if errors.Is(err, tmdb.ErrNotFound) {
			writeError(w, http.StatusBadRequest, "TMDB 上没有编号是 "+strconv.Itoa(in.TMDBID)+" 的电影")
			return
		}
		writeSubError(w, err, "电影订阅")
		return
	}
	c.args["subscription_id"], c.args["monitored"], c.args["search"] = sub.ID, monitored, search
	if _, err = s.setTags(ctx, tagKindMovie, in.TMDBID, in.Tags); err != nil {
		writeSubError(w, err, "标签")
		return
	}
	s.writeMovie(w, ctx, http.StatusCreated, *sub)
}

// updateMovie 是 PUT /movie 与 PUT /movie/{id}（id 在请求体里）：按 monitored 打开或暂停订阅、按 qualityProfileId 改档案、换标签；
// searchForMovie 为真时马上搜。200，回更新后的 movie。
func (s *Server) updateMovie(w http.ResponseWriter, r *http.Request, c *call) {
	var in movieInput
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	id := int(in.ID) //nolint:gosec // 订阅编号不会超过 int
	if r.PathValue("id") != "" {
		var ok bool
		if id, ok = pathID(w, r); !ok {
			return
		}
	}
	c.args["subscription_id"] = id
	if id <= 0 {
		writeError(w, http.StatusNotFound, "没有这个电影订阅")
		return
	}
	sub, found := s.movieSub(w, ctx, id)
	if !found {
		return
	}
	c.args["tmdb_id"] = sub.TMDBID
	if in.QualityProfileID != 0 && in.QualityProfileID != servarrProfile(sub.ProfileID) {
		profile, err := s.ptProfile(ctx, in.QualityProfileID)
		if err != nil {
			writeSubError(w, err, "质量档案")
			return
		}
		if _, err = s.deps.Subs.SetProfile(ctx, sub.ID, profile); err != nil {
			writeSubError(w, err, "电影订阅")
			return
		}
		c.args["profile_id"] = profile
	}
	search := in.AddOptions.SearchForMovie != nil && *in.AddOptions.SearchForMovie
	if in.Monitored != nil {
		updated, err := s.deps.Subs.SetMonitored(ctx, sub.ID, *in.Monitored, search)
		if err != nil {
			writeSubError(w, err, "电影订阅")
			return
		}
		sub = *updated
		c.args["monitored"] = *in.Monitored
	} else if search && sub.Status == models.MediaSubActive {
		if err := s.deps.Subs.SearchSoon(ctx, sub.ID); err != nil {
			writeSubError(w, err, "电影订阅")
			return
		}
	}
	c.args["search"] = search
	if in.Tags != nil {
		if _, err := s.setTags(ctx, tagKindMovie, sub.TMDBID, in.Tags); err != nil {
			writeSubError(w, err, "标签")
			return
		}
	}
	if fresh, err := s.deps.Subs.SubscriptionRow(ctx, sub.ID); err == nil {
		sub = fresh
	}
	s.writeMovie(w, ctx, http.StatusOK, sub)
}

// deleteMovie 是 DELETE /movie/{id}：只删订阅，忽略 deleteFiles（不删任何文件或种子）。200 {}；已经不在时 404。
func (s *Server) deleteMovie(w http.ResponseWriter, r *http.Request, c *call) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	c.args["subscription_id"] = id
	sub, found := s.movieSub(w, r.Context(), id)
	if !found {
		return
	}
	c.args["tmdb_id"] = sub.TMDBID
	if err := s.deps.Subs.DeleteSubscription(r.Context(), sub.ID); err != nil {
		writeSubError(w, err, "电影订阅")
		return
	}
	s.dropTags(r.Context(), tagKindMovie, sub.TMDBID)
	writeJSON(w, http.StatusOK, map[string]any{})
}

// details 取 TMDB 条目的详情。
func (s *Server) details(ctx context.Context, kind string, id int) (*tmdb.Result, error) {
	c, err := s.tmdb(ctx)
	if err != nil {
		return nil, err
	}
	return c.Details(ctx, kind, id)
}

// tmdb 是 TMDB 客户端；没有接上时当作没有设置 API Key。
func (s *Server) tmdb(ctx context.Context) (*tmdb.Client, error) {
	if s.deps.TMDB == nil {
		return nil, tmdb.ErrNoKey
	}
	return s.deps.TMDB(ctx)
}

// writeTMDBError 写 TMDB 出错的回应（503：Seerr 当作失败，过后重试）。
func writeTMDBError(w http.ResponseWriter, err error) {
	if errors.Is(err, tmdb.ErrNoKey) {
		writeError(w, http.StatusServiceUnavailable, "pt-tools 还没有设置 TMDB API Key")
		return
	}
	global.GetSlogger().Warnf("[Servarr] 取 TMDB 的数据失败: %v", err)
	writeError(w, http.StatusServiceUnavailable, "暂时取不到 TMDB 的数据")
}
