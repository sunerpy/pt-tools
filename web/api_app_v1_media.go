package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

// App API v1 的刷流、整理历史、订阅与探索。

// ---- 刷流 ----

// AppBrushStat 是一段时间的刷流收益。
type AppBrushStat struct {
	Uploaded   int64 `json:"uploaded"`
	Downloaded int64 `json:"downloaded"`
	Added      int   `json:"added"`
	Removed    int   `json:"removed"`
}

// AppBrushTask 是 GET /brush/tasks 的一项。
type AppBrushTask struct {
	ID          uint         `json:"id"`
	Name        string       `json:"name"`
	Enabled     bool         `json:"enabled"`
	Site        string       `json:"site"`
	SiteEnabled bool         `json:"site_enabled"`
	Downloader  string       `json:"downloader"`
	Active      int          `json:"active"`
	Downloading int          `json:"downloading"`
	ActiveSize  int64        `json:"active_size"`
	Today       AppBrushStat `json:"today"`
	Total       AppBrushStat `json:"total"`
}

func (s *Server) appBrushTasks(w http.ResponseWriter, r *http.Request) {
	if global.GlobalDB == nil {
		appError(w, http.StatusServiceUnavailable, "unavailable", "数据库没有初始化")
		return
	}
	tasks, err := models.NewBrushRepository(global.GlobalDB.DB).ListTasks()
	if err != nil {
		appError(w, http.StatusInternalServerError, "internal", "读取刷流任务失败: "+err.Error())
		return
	}
	views, err := brushTaskViews(tasks)
	if err != nil {
		appError(w, http.StatusInternalServerError, "internal", "读取刷流任务失败: "+err.Error())
		return
	}
	out := make([]AppBrushTask, 0, len(views))
	for _, v := range views {
		out = append(out, AppBrushTask{
			ID: v.ID, Name: v.Name, Enabled: v.Enabled, Site: v.SiteName, SiteEnabled: v.SiteEnabled, Downloader: v.DownloaderName,
			Active: v.ActiveCount, Downloading: v.DownloadingCount, ActiveSize: v.ActiveSizeBytes,
			Today: AppBrushStat(v.Today), Total: AppBrushStat(v.Total),
		})
	}
	appJSON(w, out)
}

// ---- 整理历史 ----

// AppMediaHistory 是 GET /media/history 的一项。
type AppMediaHistory struct {
	ID          uint   `json:"id"`
	MediaType   string `json:"media_type"`
	TMDBID      int    `json:"tmdb_id"`
	Title       string `json:"title"`
	Year        int    `json:"year,omitempty"`
	Season      int    `json:"season,omitempty"`
	Episode     int    `json:"episode,omitempty"`
	EpisodeEnd  int    `json:"episode_end,omitempty"`
	TorrentName string `json:"torrent_name"`
	Library     string `json:"library,omitempty"`
	TargetPath  string `json:"target_path,omitempty"`
	Mode        string `json:"mode"`
	Size        int64  `json:"size"`
	Status      string `json:"status"`
	Message     string `json:"message,omitempty"`
	CreatedAt   int64  `json:"created_at"`
}

func (s *Server) appMediaHistory(w http.ResponseWriter, r *http.Request) {
	if s.organizer == nil {
		appError(w, http.StatusServiceUnavailable, "unavailable", "整理入库没有启动")
		return
	}
	page, size, ok := appPaging(r, 100)
	if !ok {
		appError(w, http.StatusBadRequest, "invalid_argument", "page 从 1 开始，page_size 在 1 到 100 之间")
		return
	}
	q := r.URL.Query()
	kw := strings.TrimSpace(q.Get("q"))
	if len(kw) > 200 {
		appError(w, http.StatusBadRequest, "invalid_argument", "关键字太长")
		return
	}
	res, err := s.organizer.History(r.Context(), organize.HistoryQuery{Status: q.Get("status"), Keyword: kw, Limit: size, Offset: (page - 1) * size})
	if err != nil {
		if errors.Is(err, organize.ErrInvalid) {
			appError(w, http.StatusBadRequest, "invalid_argument", err.Error())
			return
		}
		appError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	out := AppPage[AppMediaHistory]{Items: make([]AppMediaHistory, 0, len(res.Items)), Total: int(res.Total), Page: page, PageSize: size}
	for _, h := range res.Items {
		out.Items = append(out.Items, AppMediaHistory{
			ID: h.ID, MediaType: h.MediaType, TMDBID: h.TMDBID, Title: h.Title, Year: h.Year, Season: h.Season, Episode: h.Episode,
			EpisodeEnd: h.EpisodeEnd, TorrentName: h.TorrentName, Library: h.LibraryName, TargetPath: h.TargetPath, Mode: h.Mode,
			Size: h.Size, Status: h.Status, Message: appRedact(h.Message), CreatedAt: h.CreatedAt.Unix(),
		})
	}
	appJSON(w, out)
}

// ---- 订阅 ----

// AppProgress 是订阅的进度。
type AppProgress struct {
	Total       int   `json:"total"`
	Aired       int   `json:"aired"`
	InLibrary   int   `json:"in_library"`
	Downloading int   `json:"downloading"`
	Missing     []int `json:"missing"`
}

// AppSubscription 是一个订阅。
type AppSubscription struct {
	ID            uint        `json:"id"`
	MediaType     string      `json:"media_type"`
	TMDBID        int         `json:"tmdb_id"`
	Season        int         `json:"season"`
	Title         string      `json:"title"`
	OriginalTitle string      `json:"original_title,omitempty"`
	Year          int         `json:"year,omitempty"`
	PosterPath    string      `json:"poster_path,omitempty"`
	Status        string      `json:"status"`
	Upgrade       bool        `json:"upgrade"`
	Source        string      `json:"source"`
	TotalEpisodes int         `json:"total_episodes,omitempty"`
	Progress      AppProgress `json:"progress"`
	Message       string      `json:"message,omitempty"`
	LastSearchAt  *int64      `json:"last_search_at,omitempty"`
	NextSearchAt  *int64      `json:"next_search_at,omitempty"`
	CreatedAt     int64       `json:"created_at"`
}

// AppEpisode 是剧集一集的情况：library、downloading、missing 或 upcoming。
type AppEpisode struct {
	Number  int    `json:"number"`
	Name    string `json:"name,omitempty"`
	AirDate string `json:"air_date,omitempty"`
	State   string `json:"state"`
}

// AppSubscriptionTorrent 是订阅下载过的一个种子。
type AppSubscriptionTorrent struct {
	Site       string `json:"site"`
	TorrentID  string `json:"torrent_id"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	Episode    int    `json:"episode,omitempty"`
	EpisodeEnd int    `json:"episode_end,omitempty"`
	Complete   bool   `json:"complete"`
	Size       int64  `json:"size"`
	Message    string `json:"message,omitempty"`
	CreatedAt  int64  `json:"created_at"`
}

// AppSubscriptionDetail 是 GET /subscriptions/{id}：订阅、每一集与下载过的种子。
type AppSubscriptionDetail struct {
	AppSubscription
	Episodes []AppEpisode             `json:"episodes"`
	Torrents []AppSubscriptionTorrent `json:"torrents"`
}

func appSubscription(v subscribe.SubscriptionView) AppSubscription {
	sub := v.MediaSubscription
	out := AppSubscription{
		ID: sub.ID, MediaType: sub.MediaType, TMDBID: sub.TMDBID, Season: sub.Season, Title: sub.Title, OriginalTitle: sub.OriginalTitle,
		Year: sub.Year, PosterPath: sub.PosterPath, Status: sub.Status, Upgrade: sub.Upgrade, Source: sub.Source,
		TotalEpisodes: sub.TotalEpisodes, Message: appRedact(sub.Message), LastSearchAt: unixPtr(sub.LastSearchAt),
		NextSearchAt: unixPtr(sub.NextSearchAt), CreatedAt: sub.CreatedAt.Unix(), Progress: AppProgress{Missing: []int{}},
	}
	if p := v.Progress; p != nil {
		out.Progress = AppProgress{Total: p.Total, Aired: p.Aired, InLibrary: p.InLibrary, Downloading: p.Downloading, Missing: append([]int{}, p.Missing...)}
	}
	return out
}

func (s *Server) appSubscribeService(w http.ResponseWriter) (*subscribe.Service, bool) {
	if s.subscriber == nil {
		appError(w, http.StatusServiceUnavailable, "unavailable", "订阅没有启动")
		return nil, false
	}
	return s.subscriber, true
}

// appSubscribeError 写订阅的错误：参数不对 400，找不到 404，TMDB 的错误按种类回。
func appSubscribeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, subscribe.ErrInvalid), errors.Is(err, tmdb.ErrNoKey), errors.Is(err, tmdb.ErrUnauthorized):
		appError(w, http.StatusBadRequest, "invalid_argument", err.Error())
	case errors.Is(err, subscribe.ErrNotFound), errors.Is(err, tmdb.ErrNotFound):
		appError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, subscribe.ErrBusy):
		appError(w, http.StatusConflict, "busy", err.Error())
	case errors.Is(err, tmdb.ErrRateLimited):
		appError(w, http.StatusTooManyRequests, "rate_limited", err.Error())
	case errors.Is(err, tmdb.ErrUnavailable):
		appError(w, http.StatusBadGateway, "upstream", err.Error())
	default:
		appError(w, http.StatusInternalServerError, "internal", err.Error())
	}
}

func appID(w http.ResponseWriter, r *http.Request) (uint, bool) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		appError(w, http.StatusBadRequest, "invalid_argument", "编号不对")
		return 0, false
	}
	return uint(id), true
}

func (s *Server) appSubscriptions(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.appSubscribeService(w)
	if !ok {
		return
	}
	q := r.URL.Query()
	list, err := svc.Subscriptions(r.Context(), subscribe.SubscriptionQuery{Status: q.Get("status"), Keyword: q.Get("q")})
	if err != nil {
		appSubscribeError(w, err)
		return
	}
	out := make([]AppSubscription, 0, len(list))
	for _, v := range list {
		out = append(out, appSubscription(v))
	}
	appJSON(w, out)
}

func (s *Server) appSubscriptionDetail(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.appSubscribeService(w)
	if !ok {
		return
	}
	id, ok := appID(w, r)
	if !ok {
		return
	}
	d, err := svc.Subscription(r.Context(), id)
	if err != nil {
		appSubscribeError(w, err)
		return
	}
	out := AppSubscriptionDetail{AppSubscription: appSubscription(d.SubscriptionView), Episodes: []AppEpisode{}, Torrents: []AppSubscriptionTorrent{}}
	if d.Progress != nil {
		for _, e := range d.Progress.Episodes {
			out.Episodes = append(out.Episodes, AppEpisode{Number: e.Number, Name: e.Name, AirDate: e.AirDate, State: e.State})
		}
	}
	for _, t := range d.TorrentList {
		out.Torrents = append(out.Torrents, AppSubscriptionTorrent{
			Site: t.SiteName, TorrentID: t.TorrentID, Title: t.Title, Status: t.Status, Episode: t.Episode, EpisodeEnd: t.EpisodeEnd,
			Complete: t.Complete, Size: t.SizeBytes, Message: appRedact(t.Message), CreatedAt: t.CreatedAt.Unix(),
		})
	}
	appJSON(w, out)
}

// AppSubscriptionCreate 是 POST /subscriptions：订阅一部电影或剧集的一季（Season 为 0 时订最新一季）。
// ProfileID 为 0 时用设置里的默认质量档案。
type AppSubscriptionCreate struct {
	MediaType string `json:"media_type"`
	TMDBID    int    `json:"tmdb_id"`
	Season    int    `json:"season"`
	ProfileID uint   `json:"profile_id"`
	Upgrade   bool   `json:"upgrade"`
}

func (s *Server) appSubscriptionCreate(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.appSubscribeService(w)
	if !ok {
		return
	}
	var in AppSubscriptionCreate
	if !appDecode(w, r, &in) {
		return
	}
	if in.MediaType != models.MediaKindMovie && in.MediaType != models.MediaKindTV {
		appError(w, http.StatusBadRequest, "invalid_argument", "media_type 要选 movie 或 tv")
		return
	}
	sub, err := svc.CreateSubscription(r.Context(), subscribe.SubscriptionInput{
		MediaType: in.MediaType, TMDBID: in.TMDBID, Season: in.Season, ProfileID: in.ProfileID, Upgrade: in.Upgrade,
	}, models.MediaSubFromApp)
	if err != nil {
		appSubscribeError(w, err)
		return
	}
	appJSON(w, appSubscription(subscribe.SubscriptionView{MediaSubscription: *sub}))
}

// AppSubscriptionStatus 是 POST /subscriptions/{id}/status：active 开始（或接着）找，paused 暂停。
type AppSubscriptionStatus struct {
	Status string `json:"status"`
}

func (s *Server) appSubscriptionSetStatus(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.appSubscribeService(w)
	if !ok {
		return
	}
	id, ok := appID(w, r)
	if !ok {
		return
	}
	var in AppSubscriptionStatus
	if !appDecode(w, r, &in) {
		return
	}
	sub, err := svc.SetStatus(r.Context(), id, in.Status)
	if err != nil {
		appSubscribeError(w, err)
		return
	}
	appJSON(w, appSubscription(subscribe.SubscriptionView{MediaSubscription: *sub}))
}

func (s *Server) appSubscriptionSearch(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.appSubscribeService(w)
	if !ok {
		return
	}
	id, ok := appID(w, r)
	if !ok {
		return
	}
	msg, err := svc.SearchNow(r.Context(), id)
	if err != nil {
		appSubscribeError(w, err)
		return
	}
	appJSON(w, map[string]string{"message": appRedact(msg)})
}

func (s *Server) appSubscriptionDelete(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.appSubscribeService(w)
	if !ok {
		return
	}
	id, ok := appID(w, r)
	if !ok {
		return
	}
	if err := svc.DeleteSubscription(r.Context(), id); err != nil {
		appSubscribeError(w, err)
		return
	}
	appJSON(w, map[string]bool{"ok": true})
}

// ---- 探索 ----

// AppExploreItem 是 TMDB 的一个条目，标上已入库、已订阅。
type AppExploreItem struct {
	ID             int     `json:"id"`
	MediaType      string  `json:"media_type"`
	Title          string  `json:"title"`
	OriginalTitle  string  `json:"original_title,omitempty"`
	Year           int     `json:"year,omitempty"`
	Overview       string  `json:"overview,omitempty"`
	PosterPath     string  `json:"poster_path,omitempty"`
	VoteAverage    float64 `json:"vote_average,omitempty"`
	InLibrary      bool    `json:"in_library"`
	Subscribed     bool    `json:"subscribed"`
	SubscriptionID uint    `json:"subscription_id,omitempty"`
}

// AppExplorePage 是 GET /explore 的回应。
type AppExplorePage struct {
	Items      []AppExploreItem `json:"items"`
	Page       int              `json:"page"`
	TotalPages int              `json:"total_pages"`
}

func (s *Server) appExplore(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.appSubscribeService(w)
	if !ok {
		return
	}
	q := r.URL.Query()
	page := 1
	if v := q.Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			appError(w, http.StatusBadRequest, "invalid_argument", "page 从 1 开始")
			return
		}
		page = n
	}
	list := q.Get("list")
	if list == "" {
		list = tmdb.ListTrending
	}
	res, err := svc.Explore(r.Context(), q.Get("kind"), list, page, q.Get("q"))
	if err != nil {
		appSubscribeError(w, err)
		return
	}
	out := AppExplorePage{Items: make([]AppExploreItem, 0, len(res.Items)), Page: res.Page, TotalPages: res.TotalPages}
	for _, it := range res.Items {
		out.Items = append(out.Items, AppExploreItem{
			ID: it.ID, MediaType: it.MediaType, Title: it.Title, OriginalTitle: it.OriginalTitle, Year: it.Year, Overview: it.Overview,
			PosterPath: it.PosterPath, VoteAverage: it.VoteAverage, InLibrary: it.InLibrary, Subscribed: it.Subscribed, SubscriptionID: it.SubscriptionID,
		})
	}
	appJSON(w, out)
}
