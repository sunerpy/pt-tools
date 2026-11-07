package subscribe

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 试搜索（追剧方案 3.2.1、3.2.3）：按还没保存的表单（质量档案、标题包含、标题排除）对种子，不写库；站点搜索到点就停，
// 已经回来的照常出结果。

// memberVerdict 是某个站点上某个种子的打分结果。
func memberVerdict(t *testing.T, res *MediaSearchResult, site, id string) *MediaSearchVerdict {
	t.Helper()
	for _, g := range res.Groups {
		for _, m := range g.Members {
			if m.Site == site && m.TorrentID == id {
				return m.Verdict
			}
		}
	}
	t.Fatalf("没有 %s:%s 的结果", site, id)
	return nil
}

// 包含词收窄结果、排除词写出原因；已经订阅时表单里的词换掉订阅自己的，订阅不变；不给草稿时照旧按订阅。
func TestSearchMediaDraftKeywords(t *testing.T) {
	e := newEnv(t)
	e.search.set(e.tlouItems()...)
	base := MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2}

	in := base
	in.Draft = &MediaSearchDraft{IncludeKeywords: "CHDWEB"}
	res := e.searchMedia(in)
	assert.Nil(t, res.Subscription)
	assert.Empty(t, memberVerdict(t, res, "hdsky", "1").Reject, "标题含 CHDWEB")
	assert.Equal(t, "标题不含任何包含词", memberVerdict(t, res, "pttime", "3").Reject)
	assert.Equal(t, "标题不含任何包含词", memberVerdict(t, res, "hdsky", "4").Reject)

	in.Draft = &MediaSearchDraft{ExcludeKeywords: "  ADWeb  "}
	res = e.searchMedia(in)
	assert.Equal(t, "标题含排除词 adweb", memberVerdict(t, res, "pttime", "3").Reject)
	assert.Empty(t, memberVerdict(t, res, "hdsky", "4").Reject)

	exclude := "ADWeb"
	sub := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, ExcludeKeywords: &exclude})
	before := e.subRow(sub.ID)
	in.Draft = &MediaSearchDraft{}
	res = e.searchMedia(in)
	require.NotNil(t, res.Subscription)
	assert.Equal(t, sub.ID, res.Subscription.ID)
	assert.Empty(t, memberVerdict(t, res, "pttime", "3").Reject, "表单里去掉了排除词")
	after := e.subRow(sub.ID)
	assert.Equal(t, "ADWeb", after.ExcludeKeywords, "订阅不变")
	assert.Equal(t, before.UpdatedAt, after.UpdatedAt)

	res = e.searchMedia(base)
	assert.Equal(t, "标题含排除词 adweb", memberVerdict(t, res, "pttime", "3").Reject, "不给草稿时按订阅")
}

// 表单的档案是 0 时按设置里的默认档案（和保存这张表单以后一样），不按订阅自己的；给了档案按它；档案不存在时报错。
func TestSearchMediaDraftProfile(t *testing.T) {
	e := newEnv(t)
	a, err := e.svc.SaveProfile(e.ctx, 0, ProfileInput{Name: "只要 4K", Resolutions: []string{"2160p"}})
	require.NoError(t, err)
	b, err := e.svc.SaveProfile(e.ctx, 0, ProfileInput{Name: "只要 1080p", Resolutions: []string{"1080p"}})
	require.NoError(t, err)
	sub := e.sub(SubscriptionInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, ProfileID: a.ID})
	e.enable(func(s *Settings) { s.DefaultProfileID = b.ID })
	e.search.set(e.tlouItems()...)
	in := MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Draft: &MediaSearchDraft{}}

	res := e.searchMedia(in)
	require.NotNil(t, res.Profile)
	assert.Equal(t, b.ID, res.Profile.ID)
	assert.Empty(t, memberVerdict(t, res, "pttime", "3").Reject)
	assert.Contains(t, memberVerdict(t, res, "hdsky", "1").Reject, "2160p")

	in.Draft = &MediaSearchDraft{ProfileID: a.ID}
	res = e.searchMedia(in)
	assert.Equal(t, a.ID, res.Profile.ID)
	assert.Empty(t, memberVerdict(t, res, "hdsky", "1").Reject)

	in.Draft = &MediaSearchDraft{ProfileID: 999}
	_, err = e.svc.SearchMedia(e.ctx, in)
	assert.ErrorIs(t, err, ErrInvalid, "档案不存在")
	assert.Equal(t, a.ID, e.subRow(sub.ID).ProfileID, "订阅的档案没变")
}

// 草稿与顶层的档案不能同时给；包含词、排除词各最长 1024 字节（去掉首尾空白以后，同订阅）。
func TestSearchMediaDraftInvalid(t *testing.T) {
	e := newEnv(t)
	e.search.set(e.tlouItems()...)
	long := strings.Repeat("a", maxKeywordsLen+1)
	for _, in := range []MediaSearchInput{
		{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, ProfileID: 3, Draft: &MediaSearchDraft{}},
		{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, ProfileID: noProfile, Draft: &MediaSearchDraft{}},
		{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Draft: &MediaSearchDraft{IncludeKeywords: long}},
		{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Draft: &MediaSearchDraft{ExcludeKeywords: long}},
	} {
		_, err := e.svc.SearchMedia(e.ctx, in)
		assert.ErrorIs(t, err, ErrInvalid, "%+v", in.Draft)
	}
	ok := strings.Repeat("a", maxKeywordsLen)
	_, err := e.svc.SearchMedia(e.ctx, MediaSearchInput{
		MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2,
		Draft: &MediaSearchDraft{IncludeKeywords: "  " + ok + "  ", ExcludeKeywords: ok},
	})
	assert.NoError(t, err, "1024 字节以内照常")
	assert.Empty(t, e.gotPushes())
}

// timedSite 是一个假站点：delay 以后回 items；wait 为真时一直等到 ctx 结束才回错误（站点没有回应）。
// started 不为空时第一次搜索开始时关掉它。
type timedSite struct {
	v2.Site
	id       string
	items    []v2.TorrentItem
	delay    time.Duration
	wait     bool
	searches atomic.Int32
	started  chan struct{}
	once     sync.Once
}

func (s *timedSite) ID() string        { return s.id }
func (s *timedSite) Name() string      { return s.id }
func (s *timedSite) Kind() v2.SiteKind { return v2.SiteNexusPHP }
func (s *timedSite) Close() error      { return nil }

func (s *timedSite) Search(ctx context.Context, _ v2.SearchQuery) ([]v2.TorrentItem, error) {
	s.searches.Add(1)
	s.once.Do(func() {
		if s.started != nil {
			close(s.started)
		}
	})
	if s.wait {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	out := make([]v2.TorrentItem, len(s.items))
	copy(out, s.items)
	for i := range out {
		out[i].SourceSite = s.id
	}
	return out, nil
}

// withSites 让服务用真的多站点搜索（带缓存）搜这几个假站点。
func (e *env) withSites(sites ...v2.Site) *v2.CachedSearchOrchestrator {
	orch := v2.NewSearchOrchestrator(v2.SearchOrchestratorConfig{})
	for _, s := range sites {
		orch.RegisterSite(s)
	}
	cached := v2.NewCachedSearchOrchestrator(orch, v2.SearchCacheConfig{TTL: time.Minute})
	e.svc.cfg.Search = cached
	return cached
}

// 到点返回已经回来的：快的站点照常成组，黑名单与每集情况都有；慢的站点写进 errors 并标 timed_out。
func TestSearchMediaDeadlinePartial(t *testing.T) {
	e := newEnv(t)
	items := e.tlouItems()
	fast := &timedSite{id: "hdsky", items: []v2.TorrentItem{items[0], items[3]}}
	slow := &timedSite{id: "ttg", wait: true}
	e.withSites(fast, slow)
	require.NoError(t, e.db.Create(&models.MediaTorrentBlock{SiteName: "hdsky", TorrentID: "4", MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Reason: "画质差"}).Error)

	start := time.Now()
	res, err := e.svc.SearchMedia(e.ctx, MediaSearchInput{
		MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Draft: &MediaSearchDraft{}, Deadline: time.Now().Add(500 * time.Millisecond),
	})
	require.NoError(t, err)
	assert.Less(t, time.Since(start), 10*time.Second, "到点就回")
	require.Len(t, res.Groups, 2)
	assert.Equal(t, 1, res.BlockedCount, "黑名单照常标出")
	assert.Len(t, res.Episodes, 3, "每集情况照常")
	require.Len(t, res.Errors, 1)
	assert.Equal(t, "ttg", res.Errors[0].Site)
	assert.True(t, res.Errors[0].TimedOut)
	assert.Contains(t, res.Errors[0].Error, "context deadline exceeded")
}

// 没设截止时间（零值）：不会让站点搜索马上到期，晚一点回来的站点照样有结果。
func TestSearchMediaNoDeadline(t *testing.T) {
	e := newEnv(t)
	items := e.tlouItems()
	late := &timedSite{id: "hdsky", items: []v2.TorrentItem{items[0]}, delay: 50 * time.Millisecond}
	e.withSites(late)
	res := e.searchMedia(MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	assert.Empty(t, res.Errors)
	require.Len(t, res.Groups, 1)
	assert.Equal(t, "hdsky", res.Groups[0].Members[0].Site)
}

// 调用方取消（隧道里的 CANCEL）：很快返回取消错误，不等站点。
func TestSearchMediaCallerCancelled(t *testing.T) {
	e := newEnv(t)
	slow := &timedSite{id: "ttg", wait: true, started: make(chan struct{})}
	e.withSites(slow)
	ctx, cancel := context.WithCancel(e.ctx)
	go func() {
		<-slow.started
		cancel()
	}()
	start := time.Now()
	_, err := e.svc.SearchMedia(ctx, MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Less(t, time.Since(start), 10*time.Second)
}

// 只改包含词、排除词或档案再试：截止时间不同也命中搜索缓存，不再去站点搜；有站点没回来的那次不进缓存，再试会重新搜。
func TestSearchMediaDraftUsesCache(t *testing.T) {
	e := newEnv(t)
	items := e.tlouItems()
	site := &timedSite{id: "hdsky", items: []v2.TorrentItem{items[0], items[3]}}
	e.withSites(site)
	in := MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Draft: &MediaSearchDraft{}, Deadline: time.Now().Add(time.Minute)}
	e.searchMedia(in)
	first := site.searches.Load()
	require.Positive(t, first)

	in.Draft = &MediaSearchDraft{IncludeKeywords: "CHDWEB"}
	in.Deadline = time.Now().Add(30 * time.Second)
	res := e.searchMedia(in)
	assert.Equal(t, first, site.searches.Load(), "命中缓存，没有再搜")
	assert.Equal(t, "标题不含任何包含词", memberVerdict(t, res, "hdsky", "4").Reject, "包含词照样生效")

	e2 := newEnv(t)
	ok := &timedSite{id: "hdsky", items: []v2.TorrentItem{items[0]}}
	slow := &timedSite{id: "ttg", wait: true}
	e2.withSites(ok, slow)
	in2 := MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Draft: &MediaSearchDraft{}, Deadline: time.Now().Add(300 * time.Millisecond)}
	e2.searchMedia(in2)
	n := ok.searches.Load()
	require.Positive(t, n)
	in2.Deadline = time.Now().Add(300 * time.Millisecond)
	e2.searchMedia(in2)
	assert.Greater(t, ok.searches.Load(), n, "上次有站点没回来，没进缓存，重新搜")
}

// 站点按名字排好序再搜：调用方给的顺序不同（网页与 App 按启用的站点表取，顺序不定）也命中同一条搜索缓存。
func TestSearchMediaSitesOrderUsesCache(t *testing.T) {
	e := newEnv(t)
	items := e.tlouItems()
	a := &timedSite{id: "hdsky", items: []v2.TorrentItem{items[0]}}
	b := &timedSite{id: "ttg", items: []v2.TorrentItem{items[3]}}
	e.withSites(a, b)
	in := MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Sites: []string{"ttg", "hdsky"}}
	e.searchMedia(in)
	first := a.searches.Load() + b.searches.Load()
	require.Positive(t, first)
	in.Sites = []string{"hdsky", " ttg ", "hdsky"}
	res := e.searchMedia(in)
	assert.Equal(t, first, a.searches.Load()+b.searches.Load(), "顺序不同、有重复与空白：命中缓存")
	assert.Len(t, res.Groups, 2)
}

// Validate 与 SearchMedia 开头的检查一样，不搜索。
func TestMediaSearchInputValidate(t *testing.T) {
	ok := MediaSearchInput{MediaType: models.MediaKindTV, TMDBID: 1, Season: 0, Sites: []string{"b", "a"}}
	require.NoError(t, ok.Validate())
	assert.Equal(t, []string{"b", "a"}, ok.Sites, "不改调用方的请求")
	for _, in := range []MediaSearchInput{
		{MediaType: "book", TMDBID: 1},
		{MediaType: models.MediaKindMovie},
		{MediaType: models.MediaKindTV, TMDBID: 1, Season: 201},
		{MediaType: models.MediaKindTV, TMDBID: 1, Draft: &MediaSearchDraft{ExcludeKeywords: strings.Repeat("x", maxKeywordsLen+1)}},
		{MediaType: models.MediaKindTV, TMDBID: 1, Sites: []string{strings.Repeat("s", 65)}},
	} {
		assert.ErrorIs(t, in.Validate(), ErrInvalid, "%+v", in)
	}
}
