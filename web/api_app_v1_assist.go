package web

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/web/middleware"
)

// 追剧助手（追剧方案 F2）：编辑页新建订阅前读草稿，按还没保存的表单试搜索。都只读。

// AppSubscriptionDraft 是 GET /subscriptions/draft 的回应：新建订阅前编辑页要显示的条目。
// Season 是这一季（请求的 0 换成最新一季，电影是 0）；Seasons、TotalEpisodes 是一共几季与这一季几集（电影都是 0）；
// AutoLibraryID 是媒体库选「自动」时整理会选到的库（没有能用的库时是 0）；SubscriptionID 是这一季已有的订阅（没有时是 0）。
type AppSubscriptionDraft struct {
	MediaType      string `json:"media_type"`
	TMDBID         int    `json:"tmdb_id"`
	Season         int    `json:"season"`
	Seasons        int    `json:"seasons"`
	TotalEpisodes  int    `json:"total_episodes"`
	Title          string `json:"title"`
	OriginalTitle  string `json:"original_title"`
	Year           int    `json:"year"`
	PosterPath     string `json:"poster_path"`
	Anime          bool   `json:"anime"`
	AutoLibraryID  uint   `json:"auto_library_id"`
	SubscriptionID uint   `json:"subscription_id"`
}

func (s *Server) appSubscriptionDraft(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.appSubscribeService(w)
	if !ok {
		return
	}
	q := r.URL.Query()
	id, err := strconv.Atoi(q.Get("tmdb_id"))
	if err != nil || id <= 0 {
		appError(w, http.StatusBadRequest, "invalid_argument", "tmdb_id 要是正整数")
		return
	}
	season := 0
	if v := q.Get("season"); v != "" {
		if season, err = strconv.Atoi(v); err != nil {
			appError(w, http.StatusBadRequest, "invalid_argument", "season 要是整数")
			return
		}
	}
	d, err := svc.Draft(r.Context(), q.Get("media_type"), id, season)
	if err != nil {
		appSubscribeError(w, appRedactedErr{err})
		return
	}
	appJSON(w, AppSubscriptionDraft{
		MediaType: d.MediaType, TMDBID: d.TMDBID, Season: d.Season, Seasons: d.Seasons, TotalEpisodes: d.TotalEpisodes,
		Title: d.Title, OriginalTitle: d.OriginalTitle, Year: d.Year, PosterPath: d.PosterPath, Anime: d.Anime,
		AutoLibraryID: d.AutoLibraryID, SubscriptionID: d.SubscriptionID,
	})
}

// appRedactedErr 让写进回应的错误去掉密钥与网络地址，errors.Is 照旧认得原来的错误。
type appRedactedErr struct{ err error }

func (e appRedactedErr) Error() string { return appRedactAddr(e.err.Error()) }
func (e appRedactedErr) Unwrap() error { return e.err }

// AppSearchPreviewInput 是 POST /subscriptions/search/preview：按编辑页还没保存的表单试搜索（只读）。
// ProfileID 为 0 时用设置里的默认档案（和保存以后一样）；Sites 为空时搜全部启用的站点；
// IncludeKeywords、ExcludeKeywords 是表单里的原样文字（每行一个词，写法同订阅）。
type AppSearchPreviewInput struct {
	MediaType        string   `json:"media_type"`
	TMDBID           int      `json:"tmdb_id"`
	Season           int      `json:"season"`
	ProfileID        uint     `json:"profile_id"`
	Sites            []string `json:"sites"`
	IncludeKeywords  string   `json:"include_keywords"`
	ExcludeKeywords  string   `json:"exclude_keywords"`
	IncludeUnmatched bool     `json:"include_unmatched"`
}

// AppSearchPreview 是试搜索的回应（同网页的按条目搜索，种子不带页面与下载地址）。
type AppSearchPreview struct {
	Item AppSearchPreviewItem `json:"item"`
	// Subscription 是这一季（电影是这部）已有的订阅，没有时不带
	Subscription *AppSearchPreviewSubscription `json:"subscription,omitempty"`
	// Profile 是这次打分用的档案；编号为 0 时是没有档案，按分辨率与来源比
	Profile *AppSearchPreviewProfile `json:"profile,omitempty"`
	// Keywords 是这次在站点上搜的词（按 TMDB 上的名字与订阅的别名）
	Keywords []string                  `json:"keywords"`
	Episodes []AppSearchPreviewEpisode `json:"episodes"`
	Movie    *AppSearchPreviewMovie    `json:"movie,omitempty"`
	// Groups 是对上这个条目的结果，同一资源一组，排好了序
	Groups       []AppSearchPreviewGroup `json:"groups"`
	Total        int                     `json:"total"`
	BlockedCount int                     `json:"blocked_count"`
	// UnmatchedCount 是对不上这个条目的结果数；Unmatched 只在请求 include_unmatched 时有，最多 appPreviewUnmatchedMax 条
	UnmatchedCount int                         `json:"unmatched_count"`
	Unmatched      []AppSearchPreviewUnmatched `json:"unmatched"`
	Errors         []AppSearchPreviewError     `json:"errors"`
	// DurationMs 是这次试搜索用了多久（毫秒，从收到请求算起）
	DurationMs int64 `json:"duration_ms"`
}

// AppSearchPreviewItem 是搜的条目：Year 是剧集这一季的年份，Seasons 是一共几季，TotalEpisodes 是这一季几集。
type AppSearchPreviewItem struct {
	MediaType     string `json:"media_type"`
	TMDBID        int    `json:"tmdb_id"`
	Season        int    `json:"season"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	Year          int    `json:"year"`
	PosterPath    string `json:"poster_path"`
	Seasons       int    `json:"seasons"`
	TotalEpisodes int    `json:"total_episodes"`
}

// AppSearchPreviewSubscription 是已有的订阅：Lock 是锁住的版本，没开版本锁或还没锁定时不带。
type AppSearchPreviewSubscription struct {
	ID     uint                  `json:"id"`
	Status string                `json:"status"`
	Lock   *AppSearchPreviewLock `json:"lock,omitempty"`
}

// AppSearchPreviewLock 是锁住的制作组与分辨率，认不出的那一项为空。
type AppSearchPreviewLock struct {
	Group      string `json:"group"`
	Resolution string `json:"resolution"`
}

// AppSearchPreviewProfile 是打分用的质量档案。
type AppSearchPreviewProfile struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// AppSearchPreviewEpisode 是一集现在的情况：library 已入库、downloading 在下载（Site 是来源站点）、
// waiting_version 等锁住的版本、missing 缺、upcoming 还没播。
type AppSearchPreviewEpisode struct {
	Number  int    `json:"number"`
	State   string `json:"state"`
	Site    string `json:"site,omitempty"`
	AirDate string `json:"air_date,omitempty"`
	Name    string `json:"name,omitempty"`
}

// AppSearchPreviewMovie 是电影现在的情况：library、downloading 或 missing。
type AppSearchPreviewMovie struct {
	State string `json:"state"`
	Site  string `json:"site,omitempty"`
}

// AppSearchPreviewVerdict 是按表单的结果：Reject 为空时符合，不为空时是全部原因；Score 是 0–100 的分。
type AppSearchPreviewVerdict struct {
	Score  int    `json:"score"`
	Reject string `json:"reject"`
}

// AppSearchPreviewSpan 是种子里的集：Whole 为真时是整季，否则是 [Start, End]。
type AppSearchPreviewSpan struct {
	Start int  `json:"start"`
	End   int  `json:"end"`
	Whole bool `json:"whole"`
}

// AppSearchPreviewGroup 是同一个资源：Members 的第一个是首选，Meta 与 Verdict 是它的。
// Covers 是能补上的集（缺的与等版本的）；Downloading 是这一组里正在下载的集；InLibrary 是这一组的集都已入库（电影是库里有）；
// Blocked 是成员都拉黑了（排在最后）。Span 剧集才有。
type AppSearchPreviewGroup struct {
	GroupID     string                   `json:"group_id"`
	Meta        AppMediaMeta             `json:"meta"`
	Verdict     *AppSearchPreviewVerdict `json:"verdict,omitempty"`
	Span        *AppSearchPreviewSpan    `json:"span,omitempty"`
	Covers      []int                    `json:"covers"`
	Downloading []int                    `json:"downloading"`
	InLibrary   bool                     `json:"in_library"`
	Blocked     bool                     `json:"blocked"`
	Members     []AppSearchPreviewMember `json:"members"`
}

// AppSearchPreviewMember 是一个站点上的一个种子。Discount 是站点的优惠原样；FreeLeftSeconds 是免费还剩几秒（不知道时为 0）；
// HRHours 是 H&R 要做种多少小时（不知道时为 0）；ByIMDb 是按 IMDb 编号对上的；Blocked 是拉黑了。
type AppSearchPreviewMember struct {
	Site            string                   `json:"site"`
	TorrentID       string                   `json:"torrent_id"`
	Title           string                   `json:"title"`
	Subtitle        string                   `json:"subtitle"`
	SizeBytes       int64                    `json:"size_bytes"`
	Seeders         int                      `json:"seeders"`
	Leechers        int                      `json:"leechers"`
	Snatched        int                      `json:"snatched"`
	UploadedAt      int64                    `json:"uploaded_at"`
	Discount        string                   `json:"discount"`
	Free            bool                     `json:"free"`
	FreeLeftSeconds int64                    `json:"free_left_seconds"`
	HR              bool                     `json:"hr"`
	HRHours         int                      `json:"hr_hours"`
	Category        string                   `json:"category"`
	ByIMDb          bool                     `json:"by_imdb"`
	Blocked         bool                     `json:"blocked"`
	Verdict         *AppSearchPreviewVerdict `json:"verdict,omitempty"`
}

// AppSearchPreviewUnmatched 是对不上这个条目的种子与原因。
type AppSearchPreviewUnmatched struct {
	Site      string `json:"site"`
	TorrentID string `json:"torrent_id"`
	Title     string `json:"title"`
	Subtitle  string `json:"subtitle"`
	Reason    string `json:"reason"`
}

// AppSearchPreviewError 是一个站点（Site 为空时是一次搜索）的错误；TimedOut 为真时是到了时限还没有回应。
type AppSearchPreviewError struct {
	Site     string `json:"site"`
	Error    string `json:"error"`
	TimedOut bool   `json:"timed_out"`
}

// appPreviewUnmatchedMax 是试搜索最多给多少条对不上的种子（unmatched_count 照样是总数）。
const appPreviewUnmatchedMax = 50

var (
	// appPreviewTimeout 是站点搜索最晚到收到请求以后多久（TMDB 的几次读也算在里面）；到点没回来的站点写进 errors
	appPreviewTimeout = 25 * time.Second
	// 所有主体合计同时最多这么多次试搜索，满了等 appPreviewWait 还满回 503
	appPreviewSlots = make(chan struct{}, 2)
	appPreviewWait  = 2 * time.Second
	// appPreviewBusy 是正在试搜索的主体：同一主体同时只能跑一次
	appPreviewMu   sync.Mutex
	appPreviewBusy = map[string]bool{}
)

func (s *Server) appSearchPreview(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	svc, ok := s.appSubscribeService(w)
	if !ok {
		return
	}
	var body AppSearchPreviewInput
	if !appDecode(w, r, &body) {
		return
	}
	if len(body.Sites) > appMaxSites {
		appError(w, http.StatusBadRequest, "invalid_argument", "sites 最多 100 个")
		return
	}
	in := subscribe.MediaSearchInput{
		MediaType: body.MediaType, TMDBID: body.TMDBID, Season: body.Season, Sites: body.Sites, IncludeUnmatched: body.IncludeUnmatched,
		Draft: &subscribe.MediaSearchDraft{ProfileID: body.ProfileID, IncludeKeywords: body.IncludeKeywords, ExcludeKeywords: body.ExcludeKeywords},
	}
	if err := in.Validate(); err != nil {
		appSubscribeError(w, err)
		return
	}
	requested := in.Sites
	in.Sites = filterEnabledSites(in.Sites, s.getEnabledSiteIDs())
	var skipped []AppSearchPreviewError
	for _, e := range skippedSites(requested, in.Sites) {
		skipped = append(skipped, AppSearchPreviewError{Site: e.Site, Error: e.Error})
	}
	// 指定的站点都没有启用：不搜（空的站点列表会被当成搜所有站点）
	if len(requested) > 0 && len(in.Sites) == 0 {
		out := appEmptyPreview(in)
		out.Errors = append(out.Errors, skipped...)
		out.DurationMs = time.Since(start).Milliseconds()
		appJSON(w, out)
		return
	}
	release, ok := appPreviewAcquire(w, r)
	if !ok {
		return
	}
	defer release()
	in.Deadline = start.Add(appPreviewTimeout)
	got, err := svc.SearchMedia(r.Context(), in)
	if err != nil {
		// 客户端取消了（断开、隧道里的 CANCEL）：没人等回应
		if r.Context().Err() != nil {
			return
		}
		appSubscribeError(w, appRedactedErr{err})
		return
	}
	out := appPreviewOf(got)
	out.Errors = append(out.Errors, skipped...)
	out.DurationMs = time.Since(start).Milliseconds()
	appJSON(w, out)
}

// appPreviewAcquire 占一次试搜索的名额：同一主体还有一次在跑时马上回 409；所有主体合计 cap(appPreviewSlots) 次，
// 满了等 appPreviewWait，还满回 503 并带 Retry-After。返回的函数还回名额；第二个返回值为假时已经回了错误（或请求已经取消）。
func appPreviewAcquire(w http.ResponseWriter, r *http.Request) (func(), bool) {
	key := ""
	if p := middleware.PrincipalFrom(r.Context()); p != nil {
		key = p.Kind + ":" + p.ID
	}
	appPreviewMu.Lock()
	if appPreviewBusy[key] {
		appPreviewMu.Unlock()
		appError(w, http.StatusConflict, "busy", "上一次试搜索还没结束")
		return nil, false
	}
	appPreviewBusy[key] = true
	appPreviewMu.Unlock()
	done := func() {
		appPreviewMu.Lock()
		delete(appPreviewBusy, key)
		appPreviewMu.Unlock()
	}
	wait := time.NewTimer(appPreviewWait)
	defer wait.Stop()
	select {
	case appPreviewSlots <- struct{}{}:
		return func() {
			<-appPreviewSlots
			done()
		}, true
	case <-wait.C:
		done()
		w.Header().Set("Retry-After", "5")
		appError(w, http.StatusServiceUnavailable, "busy", "同时试搜索的太多，稍后再试")
		return nil, false
	case <-r.Context().Done():
		done()
		return nil, false
	}
}

// appEmptyPreview 是没搜时的回应：只有请求的条目。
func appEmptyPreview(in subscribe.MediaSearchInput) AppSearchPreview {
	return AppSearchPreview{
		Item:     AppSearchPreviewItem{MediaType: in.MediaType, TMDBID: in.TMDBID, Season: in.Season},
		Keywords: []string{}, Episodes: []AppSearchPreviewEpisode{}, Groups: []AppSearchPreviewGroup{},
		Unmatched: []AppSearchPreviewUnmatched{}, Errors: []AppSearchPreviewError{},
	}
}

// appPreviewOf 把按条目搜索的结果换成 App 的回应：不带种子的页面与下载地址、info hash；站点错误去掉密钥与地址，
// 到点没回来的写成「N 秒内没有回应」。
func appPreviewOf(res *subscribe.MediaSearchResult) AppSearchPreview {
	it := res.Item
	out := AppSearchPreview{
		Item: AppSearchPreviewItem{
			MediaType: it.MediaType, TMDBID: it.TMDBID, Season: it.Season, Title: it.Title, OriginalTitle: it.OriginalTitle,
			Year: it.Year, PosterPath: it.PosterPath, Seasons: it.Seasons, TotalEpisodes: it.TotalEpisodes,
		},
		Keywords: append([]string{}, res.Keywords...), Episodes: []AppSearchPreviewEpisode{}, Groups: []AppSearchPreviewGroup{},
		Total: res.Total, BlockedCount: res.BlockedCount, UnmatchedCount: res.UnmatchedCount,
		Unmatched: []AppSearchPreviewUnmatched{}, Errors: []AppSearchPreviewError{},
	}
	if sub := res.Subscription; sub != nil {
		out.Subscription = &AppSearchPreviewSubscription{ID: sub.ID, Status: sub.Status}
		if l := sub.Lock; l != nil {
			out.Subscription.Lock = &AppSearchPreviewLock{Group: l.Group, Resolution: l.Resolution}
		}
	}
	if p := res.Profile; p != nil {
		out.Profile = &AppSearchPreviewProfile{ID: p.ID, Name: p.Name}
	}
	for _, e := range res.Episodes {
		out.Episodes = append(out.Episodes, AppSearchPreviewEpisode{Number: e.Episode, State: e.State, Site: e.Site, AirDate: e.AirDate, Name: e.Name})
	}
	if m := res.Movie; m != nil {
		out.Movie = &AppSearchPreviewMovie{State: m.State, Site: m.Site}
	}
	for _, g := range res.Groups {
		group := AppSearchPreviewGroup{
			GroupID: g.GroupID, Meta: appMetaOf(g.Meta), Verdict: appVerdictOf(g.Verdict), Covers: append([]int{}, g.Covers...),
			Downloading: append([]int{}, g.Downloading...), InLibrary: g.InLibrary, Blocked: g.Blocked, Members: []AppSearchPreviewMember{},
		}
		if sp := g.Span; sp != nil {
			group.Span = &AppSearchPreviewSpan{Start: sp.Start, End: sp.End, Whole: sp.Whole}
		}
		for _, m := range g.Members {
			group.Members = append(group.Members, AppSearchPreviewMember{
				Site: m.Site, TorrentID: m.TorrentID, Title: m.Title, Subtitle: m.Subtitle, SizeBytes: m.SizeBytes, Seeders: m.Seeders,
				Leechers: m.Leechers, Snatched: m.Snatched, UploadedAt: m.UploadedAt, Discount: m.Discount, Free: m.Free,
				FreeLeftSeconds: m.FreeLeftSeconds, HR: m.HR, HRHours: m.HRHours, Category: m.Category, ByIMDb: m.ByIMDb,
				Blocked: m.Block != nil, Verdict: appVerdictOf(m.Verdict),
			})
		}
		out.Groups = append(out.Groups, group)
	}
	for i, m := range res.Unmatched {
		if i == appPreviewUnmatchedMax {
			break
		}
		out.Unmatched = append(out.Unmatched, AppSearchPreviewUnmatched{Site: m.Site, TorrentID: m.TorrentID, Title: m.Title, Subtitle: m.Subtitle, Reason: m.Reason})
	}
	for _, e := range res.Errors {
		msg := appRedactAddr(e.Error)
		if e.TimedOut {
			msg = fmt.Sprintf("%d 秒内没有回应", int((appPreviewTimeout+time.Second-1)/time.Second))
		}
		out.Errors = append(out.Errors, AppSearchPreviewError{Site: e.Site, Error: msg, TimedOut: e.TimedOut})
	}
	return out
}

func appVerdictOf(v *subscribe.MediaSearchVerdict) *AppSearchPreviewVerdict {
	if v == nil {
		return nil
	}
	return &AppSearchPreviewVerdict{Score: v.Score, Reject: v.Reject}
}
