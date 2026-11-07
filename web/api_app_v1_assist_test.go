package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/web/middleware"
)

// 追剧助手的 App 接口（追剧方案 3.2.2、3.2.3）：新建前读草稿、按还没保存的表单试搜索（只读），以及选项、媒体库、新建订阅补的字段。

const previewPath = "/api/app/v1/subscriptions/search/preview"

// previewAs 用 principal 这个令牌主体调试搜索（ctx 为空时用 Background）；validate 为真时按契约校验回应。
func previewAs(t *testing.T, srv *Server, ctx context.Context, principal, body string, validate bool) *httptest.ResponseRecorder {
	t.Helper()
	if ctx == nil {
		ctx = context.Background()
	}
	req := httptest.NewRequest(http.MethodPost, previewPath, strings.NewReader(body)).WithContext(ctx)
	req = req.WithContext(middleware.WithPrincipal(req.Context(), &middleware.Principal{Kind: middleware.KindAPIToken, ID: principal, Scopes: []string{"app:read"}}))
	w := httptest.NewRecorder()
	srv.appSearchPreview(w, req)
	if validate {
		validateAppResponse(t, req, w)
	}
	return w
}

func enableSite(t *testing.T, srv *Server, name string) {
	t.Helper()
	enabled := true
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup(name), models.SiteConfig{Enabled: &enabled, AuthMethod: "cookie", Cookie: "uid=1"}))
}

func shawshank(id, group string) v2.TorrentItem {
	return v2.TorrentItem{
		ID: id, SourceSite: "hdsky", Title: "The.Shawshank.Redemption.1994.1080p.BluRay.x264-" + group, Subtitle: "肖申克的救赎 " + group,
		SizeBytes: 10 << 30, Seeders: 50, URL: "https://hdsky.example/details.php?id=" + id, DownloadURL: "https://hdsky.example/download.php?id=" + id + "&passkey=abc",
	}
}

// 草稿：季 0 换成最新一季，写一共几季、这一季几集、自动会整理进哪个库、这一季是不是已经在追；参数不对 400，TMDB 上没有 404。
func TestAppSubscriptionDraft(t *testing.T) {
	srv, _ := newSubscribeEnv(t)
	get := func(q string) *httptest.ResponseRecorder {
		return appAs(t, srv.appSubscriptionDraft, http.MethodGet, "/api/app/v1/subscriptions/draft?"+q)
	}
	w := get("media_type=tv&tmdb_id=1001&season=0")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	var d AppSubscriptionDraft
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &d))
	assert.Equal(t, AppSubscriptionDraft{MediaType: "tv", TMDBID: 1001, Season: 1, Seasons: 1, TotalEpisodes: 2, Title: "假综艺", OriginalTitle: "假综艺", Year: 2026}, d)

	db := global.GlobalDB.DB
	lib := models.MediaLibrary{Name: "综艺", Kind: models.MediaKindTV, Path: t.TempDir(), Enabled: true}
	require.NoError(t, db.Create(&lib).Error)
	sub, err := srv.subscriber.CreateSubscription(context.Background(), subscribe.SubscriptionInput{MediaType: "tv", TMDBID: 1001, Season: 1}, models.MediaSubFromApp)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(get("media_type=tv&tmdb_id=1001").Body.Bytes(), &d))
	assert.Equal(t, lib.ID, d.AutoLibraryID)
	assert.Equal(t, sub.ID, d.SubscriptionID)

	require.NoError(t, json.Unmarshal(get("media_type=movie&tmdb_id=278&season=3").Body.Bytes(), &d))
	assert.Zero(t, d.Season, "电影没有季")
	assert.Equal(t, "肖申克的救赎", d.Title)

	for _, q := range []string{"media_type=book&tmdb_id=1", "media_type=tv&tmdb_id=abc", "media_type=tv&tmdb_id=1001&season=x", "media_type=tv&tmdb_id=1001&season=300", "media_type=tv"} {
		assert.Equal(t, http.StatusBadRequest, get(q).Code, q)
	}
	assert.Equal(t, http.StatusNotFound, get("media_type=movie&tmdb_id=999").Code)
}

// 试搜索：按表单的包含词收窄、不带种子地址、错误去掉 passkey；只读：订阅、运行记录、种子记录、台账的行数都不变。
func TestAppSearchPreview(t *testing.T) {
	writeWebTestSecretKey(t)
	fake := &entrySearchFake{items: []v2.TorrentItem{shawshank("1", "WiKi"), shawshank("2", "CHD")}}
	srv, _ := newSubscribeEnv(t, func(c *subscribe.Config) { c.Search = fake })
	enableSite(t, srv, "hdsky")
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}))
	counts := func() []int64 {
		var out []int64
		for _, m := range []any{&models.MediaSubscription{}, &models.MediaSubscriptionRun{}, &models.MediaSubscriptionTorrent{}, &models.TorrentInfo{}, &models.MediaSubscriptionEpisode{}} {
			var n int64
			require.NoError(t, db.Model(m).Count(&n).Error)
			out = append(out, n)
		}
		return out
	}
	before := counts()

	w := previewAs(t, srv, nil, "1", `{"media_type":"movie","tmdb_id":278,"profile_id":0,"sites":[],"include_keywords":"WiKi","exclude_keywords":"","include_unmatched":true}`, true)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	assert.NotContains(t, w.Body.String(), "hdsky.example", "不带种子页面与下载地址")
	var got AppSearchPreview
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "肖申克的救赎", got.Item.Title)
	assert.Nil(t, got.Subscription)
	require.NotNil(t, got.Profile)
	assert.NotEmpty(t, got.Keywords)
	require.Len(t, got.Groups, 2)
	byGroup := map[string]AppSearchPreviewGroup{}
	for _, g := range got.Groups {
		byGroup[g.Meta.Group] = g
	}
	require.NotNil(t, byGroup["WiKi"].Verdict)
	assert.Empty(t, byGroup["WiKi"].Verdict.Reject)
	assert.Equal(t, "标题不含任何包含词", byGroup["CHD"].Verdict.Reject)
	require.Len(t, byGroup["WiKi"].Members, 1)
	m := byGroup["WiKi"].Members[0]
	assert.Equal(t, "hdsky", m.Site)
	assert.Equal(t, "1", m.TorrentID)
	assert.Equal(t, "The.Shawshank.Redemption.1994.1080p.BluRay.x264-WiKi", m.Title)
	assert.Equal(t, "肖申克的救赎 WiKi", m.Subtitle)
	assert.Equal(t, int64(10<<30), m.SizeBytes)
	assert.Equal(t, 50, m.Seeders)
	require.NotNil(t, m.Verdict)
	assert.Equal(t, "1080p", byGroup["WiKi"].Meta.Resolution)
	assert.Equal(t, 2, got.Total)
	assert.Empty(t, got.Errors)
	assert.Equal(t, []string{"hdsky"}, fake.queries[0].Sites, "没指定站点时搜启用的站点")
	assert.Equal(t, before, counts(), "只读")

	// 指定的站点有没启用的：写进 errors；一个都没启用：不搜，回空结果
	n := len(fake.queries)
	w = previewAs(t, srv, nil, "1", `{"media_type":"movie","tmdb_id":278,"sites":["nope"]}`, true)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Empty(t, got.Groups)
	assert.Equal(t, []AppSearchPreviewError{{Site: "nope", Error: "站点没有启用，没有搜索"}}, got.Errors)
	assert.Equal(t, n, len(fake.queries), "一个都没启用时不搜")
	assert.Equal(t, 278, got.Item.TMDBID)

	// 参数不对 400；拼错的字段 400；TMDB 上没有 404
	for _, body := range []string{
		`{"media_type":"book","tmdb_id":278}`,
		`{"media_type":"movie","tmdb_id":0}`,
		`{"media_type":"movie","tmdb_id":278,"include_keywords":"` + strings.Repeat("a", 1025) + `"}`,
		`{"media_type":"movie","tmdb_id":278,"bogus":1}`,
		`{"media_type":"movie","tmdb_id":278,"profile_id":-1}`,
	} {
		assert.Equal(t, http.StatusBadRequest, previewAs(t, srv, nil, "1", body, true).Code, body)
	}
	assert.Equal(t, http.StatusNotFound, previewAs(t, srv, nil, "1", `{"media_type":"movie","tmdb_id":999}`, true).Code)
}

// 对不上这个条目的最多给 50 条，unmatched_count 是总数；站点的错误去掉 passkey。
func TestAppSearchPreviewUnmatchedAndErrors(t *testing.T) {
	writeWebTestSecretKey(t)
	items := []v2.TorrentItem{shawshank("1", "WiKi")}
	for i := range 60 {
		items = append(items, v2.TorrentItem{ID: fmt.Sprint(100 + i), SourceSite: "hdsky", Title: fmt.Sprintf("Other.Movie.%d.2001.1080p.BluRay-X", i), SizeBytes: 1 << 30})
	}
	fake := &errSearchFake{items: items, errs: []v2.SearchError{{Site: "hdtime", Error: `Get "https://hdtime.example/x.php?passkey=abc": EOF`}}}
	srv, _ := newSubscribeEnv(t, func(c *subscribe.Config) { c.Search = fake })
	enableSite(t, srv, "hdsky")
	enableSite(t, srv, "hdtime")
	w := previewAs(t, srv, nil, "1", `{"media_type":"movie","tmdb_id":278,"include_unmatched":true}`, true)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	var got AppSearchPreview
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Len(t, got.Unmatched, 50)
	assert.Equal(t, 60, got.UnmatchedCount)
	assert.NotEmpty(t, got.Unmatched[0].Reason)
	require.Len(t, got.Errors, 1)
	assert.Equal(t, "hdtime", got.Errors[0].Site)
	assert.False(t, got.Errors[0].TimedOut)
	assert.NotContains(t, got.Errors[0].Error, "passkey=abc")

	w = previewAs(t, srv, nil, "1", `{"media_type":"movie","tmdb_id":278}`, true)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Empty(t, got.Unmatched, "没要时不带")
	assert.Equal(t, 60, got.UnmatchedCount)
}

// errSearchFake 回固定的种子与站点错误。
type errSearchFake struct {
	items []v2.TorrentItem
	errs  []v2.SearchError
}

func (f *errSearchFake) Search(context.Context, v2.MultiSiteSearchQuery) (*v2.MultiSiteSearchResult, error) {
	return &v2.MultiSiteSearchResult{Items: f.items, Errors: f.errs}, nil
}

// 时限到了：回已经回来的结果，没回来的站点写「N 秒内没有回应」并标 timed_out（时限可注入，不真等 25 秒）。
func TestAppSearchPreviewTimeout(t *testing.T) {
	writeWebTestSecretKey(t)
	old := appPreviewTimeout
	appPreviewTimeout = 300 * time.Millisecond
	t.Cleanup(func() { appPreviewTimeout = old })
	fake := &entrySearchFake{delay: time.Minute, items: []v2.TorrentItem{shawshank("1", "WiKi")}}
	srv, _ := newSubscribeEnv(t, func(c *subscribe.Config) { c.Search = fake })
	enableSite(t, srv, "hdsky")
	start := time.Now()
	w := previewAs(t, srv, nil, "1", `{"media_type":"movie","tmdb_id":278}`, true)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Less(t, time.Since(start), 10*time.Second)
	var got AppSearchPreview
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Empty(t, got.Groups)
	require.Len(t, got.Errors, 1)
	assert.Equal(t, AppSearchPreviewError{Site: "hdsky", Error: "1 秒内没有回应", TimedOut: true}, got.Errors[0])
	assert.Positive(t, got.DurationMs)
}

// gateSearch 搜索开始时发一次 started，等 release 或 ctx 结束。
type gateSearch struct {
	started chan struct{}
	release chan struct{}
	active  atomic.Int32
	items   []v2.TorrentItem
}

func (g *gateSearch) Search(ctx context.Context, _ v2.MultiSiteSearchQuery) (*v2.MultiSiteSearchResult, error) {
	g.active.Add(1)
	defer g.active.Add(-1)
	select {
	case g.started <- struct{}{}:
	default:
	}
	select {
	case <-g.release:
		return &v2.MultiSiteSearchResult{Items: g.items}, nil
	case <-ctx.Done():
		return &v2.MultiSiteSearchResult{Errors: []v2.SearchError{{Site: "hdsky", Error: ctx.Err().Error()}}}, nil
	}
}

// 并发：同一主体同时只能跑一次（409 busy）；所有主体合计同时两次，满了等一会儿还满回 503 busy 带 Retry-After。
// TestAppSearchPreviewCancel：请求被取消（隧道里的 CANCEL）时两层名额都还回去，同一主体紧接着再发回 200。
func TestAppSearchPreviewConcurrency(t *testing.T) {
	writeWebTestSecretKey(t)
	oldWait := appPreviewWait
	appPreviewWait = 50 * time.Millisecond
	t.Cleanup(func() { appPreviewWait = oldWait })
	gate := &gateSearch{started: make(chan struct{}, 16), release: make(chan struct{}), items: []v2.TorrentItem{shawshank("1", "WiKi")}}
	srv, _ := newSubscribeEnv(t, func(c *subscribe.Config) { c.Search = gate })
	enableSite(t, srv, "hdsky")
	body := `{"media_type":"movie","tmdb_id":278}`

	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i, p := range []string{"p1", "p2"} {
		wg.Go(func() { codes[i] = previewAs(t, srv, nil, p, body, false).Code })
	}
	require.Eventually(t, func() bool { return len(appPreviewSlots) == 2 && gate.active.Load() >= 2 }, 5*time.Second, 5*time.Millisecond)

	w := previewAs(t, srv, nil, "p1", body, true)
	assert.Equal(t, http.StatusConflict, w.Code, "同一主体还有一次在跑")
	assert.Contains(t, w.Body.String(), `"busy"`)
	w = previewAs(t, srv, nil, "p3", body, true)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code, "合计两次满了")
	assert.Contains(t, w.Body.String(), `"busy"`)
	assert.NotEmpty(t, w.Header().Get("Retry-After"))

	close(gate.release)
	wg.Wait()
	assert.Equal(t, []int{http.StatusOK, http.StatusOK}, codes)
	assert.Empty(t, appPreviewSlots, "名额都还回去了")
}

func TestAppSearchPreviewCancel(t *testing.T) {
	writeWebTestSecretKey(t)
	gate2 := &gateSearch{started: make(chan struct{}, 16), release: make(chan struct{}), items: []v2.TorrentItem{shawshank("1", "WiKi")}}
	srv, _ := newSubscribeEnv(t, func(c *subscribe.Config) { c.Search = gate2 })
	enableSite(t, srv, "hdsky")
	body := `{"media_type":"movie","tmdb_id":278}`
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		previewAs(t, srv, ctx, "p1", body, false)
	}()
	<-gate2.started
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("取消以后处理函数没有返回")
	}
	close(gate2.release)
	assert.Equal(t, http.StatusOK, previewAs(t, srv, nil, "p1", body, true).Code, "取消的那次已经还了名额")
	assert.Empty(t, appPreviewSlots)
}

// 等全局名额时请求被取消（隧道里的 CANCEL）：处理函数马上返回，不写回应，主体的名额还回去，名额空出来以后同一主体再发回 200。
func TestAppSearchPreviewCancelWhileWaiting(t *testing.T) {
	writeWebTestSecretKey(t)
	oldWait := appPreviewWait
	appPreviewWait = 10 * time.Second
	t.Cleanup(func() { appPreviewWait = oldWait })
	fake := &entrySearchFake{items: []v2.TorrentItem{shawshank("1", "WiKi")}}
	srv, _ := newSubscribeEnv(t, func(c *subscribe.Config) { c.Search = fake })
	enableSite(t, srv, "hdsky")
	body := `{"media_type":"movie","tmdb_id":278}`
	for range cap(appPreviewSlots) {
		appPreviewSlots <- struct{}{}
	}
	drained := false
	drain := func() {
		if drained {
			return
		}
		drained = true
		for range cap(appPreviewSlots) {
			<-appPreviewSlots
		}
	}
	t.Cleanup(drain)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- previewAs(t, srv, ctx, "p1", body, false) }()
	require.Eventually(t, func() bool {
		appPreviewMu.Lock()
		defer appPreviewMu.Unlock()
		return appPreviewBusy[middleware.KindAPIToken+":p1"]
	}, 5*time.Second, 5*time.Millisecond, "在等名额")
	cancel()
	var w *httptest.ResponseRecorder
	select {
	case w = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("取消以后处理函数没有返回")
	}
	assert.Empty(t, w.Body.String(), "没人等回应，不写")
	assert.Empty(t, fake.queries, "没有搜")
	appPreviewMu.Lock()
	assert.Empty(t, appPreviewBusy, "主体的名额还回去了")
	appPreviewMu.Unlock()

	drain()
	assert.Equal(t, http.StatusOK, previewAs(t, srv, nil, "p1", body, true).Code)
}

// 剧集的试搜索：写这一季每一集的状态与每组覆盖哪几集（整季包标 whole）；这一季已经在追时带上订阅与锁住的版本，
// 每一集的状态照追剧台账（开着锁、还没下到的是 waiting_version）。
func TestAppSearchPreviewTV(t *testing.T) {
	writeWebTestSecretKey(t)
	fake := &entrySearchFake{items: []v2.TorrentItem{
		{ID: "11", SourceSite: "hdsky", Title: "Fake.Variety.S01E01.2026.1080p.WEB-DL.H264-CHDWEB", SizeBytes: 2 << 30, Seeders: 9},
		{ID: "12", SourceSite: "hdsky", Title: "Fake.Variety.S01.2026.1080p.WEB-DL.H264-HHWEB", SizeBytes: 4 << 30, Seeders: 5},
	}}
	srv, _ := newSubscribeEnv(t, func(c *subscribe.Config) { c.Search = fake })
	enableSite(t, srv, "hdsky")
	preview := func() AppSearchPreview {
		t.Helper()
		w := previewAs(t, srv, nil, "1", `{"media_type":"tv","tmdb_id":1001,"season":1}`, true)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var got AppSearchPreview
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		return got
	}
	states := func(got AppSearchPreview) []string {
		var out []string
		for i, e := range got.Episodes {
			assert.Equal(t, i+1, e.Number)
			out = append(out, e.State)
		}
		return out
	}

	// 还没订阅：没有 subscription，按 TMDB 两集都缺
	got := preview()
	assert.Equal(t, 1, got.Item.Season)
	assert.Nil(t, got.Subscription)
	assert.Equal(t, []string{"missing", "missing"}, states(got))
	require.Len(t, got.Episodes, 2)
	assert.Equal(t, "第1期上", got.Episodes[0].Name)
	byGroup := map[string]AppSearchPreviewGroup{}
	for _, g := range got.Groups {
		byGroup[g.Meta.Group] = g
	}
	require.Contains(t, byGroup, "CHDWEB")
	require.Contains(t, byGroup, "HHWEB")
	require.NotNil(t, byGroup["CHDWEB"].Span)
	assert.Equal(t, AppSearchPreviewSpan{Start: 1, End: 1}, *byGroup["CHDWEB"].Span)
	assert.Equal(t, []int{1}, byGroup["CHDWEB"].Covers)
	require.NotNil(t, byGroup["HHWEB"].Span)
	assert.True(t, byGroup["HHWEB"].Span.Whole)
	assert.Equal(t, []int{1, 2}, byGroup["HHWEB"].Covers, "整季包能补上这一季缺的两集")

	w := appAsWith(t, srv.appSubscriptionCreate, http.MethodPost, "/api/app/v1/subscriptions",
		`{"media_type":"tv","tmdb_id":1001,"season":1,"lock_version":true,"lock_group":"CHDWEB","lock_resolution":"1080p"}`, "app:write")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var sub AppSubscription
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sub))

	got = preview()
	require.NotNil(t, got.Subscription)
	assert.Equal(t, sub.ID, got.Subscription.ID)
	assert.Equal(t, "active", got.Subscription.Status)
	require.NotNil(t, got.Subscription.Lock)
	assert.Equal(t, AppSearchPreviewLock{Group: "CHDWEB", Resolution: "1080p"}, *got.Subscription.Lock)
	assert.Equal(t, []string{"waiting_version", "waiting_version"}, states(got), "开着锁、还没下到：等锁住的版本")
}

// 同一条目先后试两次（截止时间不同、包含词不同）：真的多站点搜索带缓存，假站点只被搜了一次。
func TestAppSearchPreviewCache(t *testing.T) {
	writeWebTestSecretKey(t)
	site := &previewSite{id: "hdsky", items: []v2.TorrentItem{shawshank("1", "WiKi"), shawshank("2", "CHD")}}
	orch := v2.NewSearchOrchestrator(v2.SearchOrchestratorConfig{})
	orch.RegisterSite(site)
	cached := v2.NewCachedSearchOrchestrator(orch, v2.SearchCacheConfig{TTL: time.Minute})
	srv, _ := newSubscribeEnv(t, func(c *subscribe.Config) { c.Search = cached })
	enableSite(t, srv, "hdsky")
	enableSite(t, srv, "hdtime")
	require.Equal(t, http.StatusOK, previewAs(t, srv, nil, "1", `{"media_type":"movie","tmdb_id":278}`, true).Code)
	first := site.searches.Load()
	require.Positive(t, first)
	w := previewAs(t, srv, nil, "1", `{"media_type":"movie","tmdb_id":278,"include_keywords":"CHD"}`, true)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, first, site.searches.Load(), "只改了包含词：命中缓存")
	var got AppSearchPreview
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got.Groups, 2)
}

// previewSite 是一个记搜索次数的假站点。
type previewSite struct {
	fakeV2Site
	id       string
	items    []v2.TorrentItem
	searches atomic.Int32
}

func (s *previewSite) ID() string   { return s.id }
func (s *previewSite) Name() string { return s.id }
func (s *previewSite) Search(context.Context, v2.SearchQuery) ([]v2.TorrentItem, error) {
	s.searches.Add(1)
	return s.items, nil
}

// /meta 的 features 里有 subscription_assist；路由表里试搜索只要 app:read。
func TestAppSubscriptionAssistFeature(t *testing.T) {
	e := newAppEnv(t)
	w := e.do(appReq{method: http.MethodGet, path: appPrefix + "/meta", bearer: e.token(apitoken.ScopeAppRead)})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var meta AppMeta
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &meta))
	assert.Contains(t, meta.Features, "subscription_assist")
	assert.True(t, appReadPosts["POST /subscriptions/search/preview"])
}

// 选项：下载器带 enabled，停用的照旧列出；各下载器的预设目录；不带账号与地址。
func TestAppMobileOptionsDownloadersAndDirectories(t *testing.T) {
	e := newAppEnv(t)
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(&models.DownloaderDirectory{}))
	on := models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:1", Username: "secret-user", Password: "secret-password", Enabled: true, IsDefault: true}
	require.NoError(t, db.Create(&on).Error)
	off := models.DownloaderSetting{Name: "tr", Type: "transmission", URL: "http://127.0.0.1:2", Enabled: true}
	require.NoError(t, db.Create(&off).Error)
	require.NoError(t, db.Model(&off).Update("enabled", false).Error)
	require.NoError(t, db.Create(&models.DownloaderDirectory{DownloaderID: on.ID, Path: "/downloads/tv", Alias: "剧集", IsDefault: true}).Error)
	require.NoError(t, db.Create(&models.DownloaderDirectory{DownloaderID: on.ID, Path: "/downloads/movies"}).Error)
	w := e.do(appReq{method: http.MethodGet, path: appPrefix + "/mobile/options", bearer: e.token(apitoken.ScopeAppRead)})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	assert.NotContains(t, w.Body.String(), "secret-")
	assert.NotContains(t, w.Body.String(), "127.0.0.1")
	var got AppMobileOptions
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.ElementsMatch(t, []AppMobileDownloaderOption{{ID: on.ID, Name: "qb", Enabled: true}, {ID: off.ID, Name: "tr", Enabled: false}}, got.Downloaders)
	assert.Equal(t, []AppMobileDirectory{
		{DownloaderID: on.ID, Path: "/downloads/tv", Alias: "剧集", IsDefault: true},
		{DownloaderID: on.ID, Path: "/downloads/movies"},
	}, got.Directories)
}

// 新建订阅可以带锁住的版本（只在用户改了锁时发）；不认识的字段照旧 400。
// 新建订阅时可以一开始就锁住版本（试搜索里点了「锁这个版本」）；拼错的字段 400。
func TestAppSubscriptionCreateLock(t *testing.T) {
	srv, _ := newSubscribeEnv(t)
	w := appAsWith(t, srv.appSubscriptionCreate, http.MethodPost, "/api/app/v1/subscriptions",
		`{"media_type":"tv","tmdb_id":1001,"lock_version":true,"lock_group":"CHDWEB","lock_resolution":"1080p"}`, "app:write")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var sub AppSubscription
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sub))
	assert.True(t, sub.LockVersion)
	assert.Equal(t, "CHDWEB", sub.LockGroup)
	assert.Equal(t, "1080p", sub.LockResolution)
	assert.Equal(t, http.StatusBadRequest, appAsWith(t, srv.appSubscriptionCreate, http.MethodPost, "/api/app/v1/subscriptions",
		`{"media_type":"tv","tmdb_id":1001,"lock_groups":"x"}`, "app:write").Code)
}
