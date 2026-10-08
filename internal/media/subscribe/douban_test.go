package subscribe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

func TestParseDouban(t *testing.T) {
	body, err := os.ReadFile("testdata/douban.xml")
	require.NoError(t, err)
	wishes, err := parseDouban(body)
	require.NoError(t, err)
	assert.Equal(t, []doubanWish{
		{ID: "35575567", Name: "沙丘2", Original: "Dune: Part Two"},
		{ID: "36000001", Name: "最后生还者 第二季", Original: "The Last of Us Season 2"},
		{ID: "99999999", Name: "不存在的电影 Relaxin'", Original: "Relaxin' Nowhere"},
	}, wishes, "只要「想看」的电影与剧集：在看、看过、读书的都不要")

	_, err = parseDouban([]byte("<html>blocked</html"))
	require.Error(t, err)
}

// fakeDouban 回 testdata/douban.xml；fail 为真时回 503。
func fakeDouban(t *testing.T, fail *atomic.Bool) *httptest.Server {
	body, err := os.ReadFile("testdata/douban.xml")
	require.NoError(t, err)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if r.URL.Path != "/feed/people/qa/interests" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDoubanPullCreatesSubscriptions(t *testing.T) {
	e := newEnv(t)
	var fail atomic.Bool
	e.svc.cfg.DoubanBase = fakeDouban(t, &fail).URL
	e.enable(nil)
	p, err := e.svc.SaveProfile(e.ctx, 0, ProfileInput{Name: "4K"})
	require.NoError(t, err)

	_, err = e.svc.SaveDoubanSource(e.ctx, 0, DoubanSourceInput{UserID: "qa/../x"})
	require.ErrorIs(t, err, ErrInvalid)
	src, err := e.svc.SaveDoubanSource(e.ctx, 0, DoubanSourceInput{UserID: "qa", Name: "QA", Enabled: true, Confirm: true, ProfileID: p.ID})
	require.NoError(t, err)
	_, err = e.svc.SaveDoubanSource(e.ctx, 0, DoubanSourceInput{UserID: "qa"})
	require.ErrorIs(t, err, ErrInvalid, "同一个用户只加一次")

	e.svc.Tick(e.ctx)
	subs, err := e.svc.Subscriptions(e.ctx, SubscriptionQuery{})
	require.NoError(t, err)
	require.Len(t, subs, 2)
	tv, movie := subs[0], subs[1]
	assert.Equal(t, 693134, movie.TMDBID)
	assert.Equal(t, models.MediaSubPending, movie.Status, "来源设成要先确认")
	assert.Equal(t, models.MediaSubFromDouban, movie.Source)
	assert.Equal(t, "35575567", movie.DoubanID)
	assert.Equal(t, p.ID, movie.ProfileID)
	assert.Equal(t, 100088, tv.TMDBID)
	assert.Equal(t, 2, tv.Season, "标题里的「第二季」")

	items, err := e.svc.DoubanItems(e.ctx, src.ID)
	require.NoError(t, err)
	require.Len(t, items, 3)
	assert.Equal(t, models.MediaDoubanUnmatched, items[0].Status, "TMDB 上找不到的记下，不每次都搜")
	views, err := e.svc.DoubanSources(e.ctx)
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, 2, views[0].Subscribed)
	assert.Equal(t, 1, views[0].Unmatched)
	require.NotNil(t, views[0].NextFetchAt)
	assert.Equal(t, e.Now().Add(6*time.Hour), *views[0].NextFetchAt)

	// 用户删掉订阅以后，再拉也不会建回来
	require.NoError(t, e.svc.DeleteSubscription(e.ctx, movie.ID))
	n, err := e.svc.FetchDouban(e.ctx, src.ID)
	require.NoError(t, err)
	assert.Zero(t, n)
	subs, err = e.svc.Subscriptions(e.ctx, SubscriptionQuery{})
	require.NoError(t, err)
	assert.Len(t, subs, 1)

	// 删除来源：建过的订阅留着
	require.NoError(t, e.svc.DeleteDoubanSource(e.ctx, src.ID))
	subs, err = e.svc.Subscriptions(e.ctx, SubscriptionQuery{})
	require.NoError(t, err)
	assert.Len(t, subs, 1)
}

// 已经订阅过的条目只记下，不重复建
func TestDoubanExistingSubscription(t *testing.T) {
	e := newEnv(t)
	var fail atomic.Bool
	e.svc.cfg.DoubanBase = fakeDouban(t, &fail).URL
	movie := e.sub(SubscriptionInput{MediaType: models.MediaKindMovie, TMDBID: 693134})
	src, err := e.svc.SaveDoubanSource(e.ctx, 0, DoubanSourceInput{UserID: "qa", Enabled: true})
	require.NoError(t, err)
	n, err := e.svc.FetchDouban(e.ctx, src.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "只建了剧集")
	items, err := e.svc.DoubanItems(e.ctx, src.ID)
	require.NoError(t, err)
	var linked uint
	for _, it := range items {
		if it.DoubanID == "35575567" {
			linked = it.SubscriptionID
		}
	}
	assert.Equal(t, movie.ID, linked)
	assert.Equal(t, models.MediaSubActive, e.subRow(movie.ID).Status)
}

// 拉取失败按 1、2、4 小时退避；连续 3 次失败标成异常、发一次通知；之后成功时恢复
func TestDoubanBackoffAndAlert(t *testing.T) {
	e := newEnv(t)
	var fail atomic.Bool
	fail.Store(true)
	e.svc.cfg.DoubanBase = fakeDouban(t, &fail).URL
	e.enable(func(s *Settings) { s.NotifyChannels = []uint{2} })
	src, err := e.svc.SaveDoubanSource(e.ctx, 0, DoubanSourceInput{UserID: "qa", Name: "QA", Enabled: true})
	require.NoError(t, err)
	for i, want := range []time.Duration{time.Hour, 2 * time.Hour, 4 * time.Hour, 8 * time.Hour} {
		e.svc.doubanDue(e.ctx)
		row, rerr := e.svc.doubanRow(e.ctx, src.ID)
		require.NoError(t, rerr)
		assert.Equal(t, i+1, row.Failures)
		assert.Contains(t, row.LastError, "HTTP 503")
		require.NotNil(t, row.NextFetchAt)
		assert.Equal(t, e.Now().Add(want), *row.NextFetchAt, "第 %d 次失败", i+1)
		assert.Equal(t, i+1 >= 3, row.Abnormal)
		e.svc.doubanDue(e.ctx)
		again, aerr := e.svc.doubanRow(e.ctx, src.ID)
		require.NoError(t, aerr)
		assert.Equal(t, i+1, again.Failures, "退避期内不拉")
		e.advance(want)
	}
	require.Len(t, e.notices, 1, "异常只通知一次")
	assert.Equal(t, NoticeDoubanAbnormal, e.notices[0].Kind)
	assert.Contains(t, e.notices[0].Body, "QA：连续 3 次")

	fail.Store(false)
	e.svc.doubanDue(e.ctx)
	row, err := e.svc.doubanRow(e.ctx, src.ID)
	require.NoError(t, err)
	assert.Zero(t, row.Failures)
	assert.False(t, row.Abnormal)
	assert.Empty(t, row.LastError)
}

// 没填 TMDB API Key 时整次算失败，条目不记成没找到
func TestDoubanNeedsTMDB(t *testing.T) {
	e := newEnv(t)
	var fail atomic.Bool
	e.svc.cfg.DoubanBase = fakeDouban(t, &fail).URL
	src, err := e.svc.SaveDoubanSource(e.ctx, 0, DoubanSourceInput{UserID: "qa", Enabled: true})
	require.NoError(t, err)
	e.svc.cfg.Recognizer = noKeyRecognizer{e.svc.cfg.Recognizer}
	_, err = e.svc.FetchDouban(e.ctx, src.ID)
	require.ErrorIs(t, err, tmdb.ErrNoKey)
	items, err := e.svc.DoubanItems(e.ctx, src.ID)
	require.NoError(t, err)
	assert.Empty(t, items)
	row, err := e.svc.doubanRow(e.ctx, src.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, row.Failures)
}

type noKeyRecognizer struct{ Recognizer }

func (noKeyRecognizer) TMDB(context.Context) (*tmdb.Client, error) { return nil, tmdb.ErrNoKey }

// 豆瓣跳到别的主机时不跟过去、记成失败；同一个主机里的跳转照常跟
func TestDoubanRedirects(t *testing.T) {
	e := newEnv(t)
	var hit atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit.Store(true) }))
	t.Cleanup(other.Close)
	body, err := os.ReadFile("testdata/douban.xml")
	require.NoError(t, err)
	douban := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed/people/away/interests":
			http.Redirect(w, r, other.URL+"/internal", http.StatusFound)
		case "/feed/people/moved/interests":
			http.Redirect(w, r, "/feed/people/qa/interests", http.StatusMovedPermanently)
		case "/feed/people/qa/interests":
			_, _ = w.Write(body)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(douban.Close)
	e.svc.cfg.DoubanBase = douban.URL

	away, err := e.svc.SaveDoubanSource(e.ctx, 0, DoubanSourceInput{UserID: "away", Enabled: true})
	require.NoError(t, err)
	_, err = e.svc.FetchDouban(e.ctx, away.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "跳转到了别的地址")
	assert.False(t, hit.Load(), "没有请求跳转的地址")

	moved, err := e.svc.SaveDoubanSource(e.ctx, 0, DoubanSourceInput{UserID: "moved", Enabled: true})
	require.NoError(t, err)
	_, err = e.svc.FetchDouban(e.ctx, moved.ID)
	require.NoError(t, err)
}

// 同一个来源同时只拉一次：正在拉时「立即拉取」写明正在拉，定时拉取跳过它，被挡住的不算失败
func TestDoubanPullIsExclusivePerSource(t *testing.T) {
	e := newEnv(t)
	body, err := os.ReadFile("testdata/douban.xml")
	require.NoError(t, err)
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		<-release
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	e.svc.cfg.DoubanBase = srv.URL
	src, err := e.svc.SaveDoubanSource(e.ctx, 0, DoubanSourceInput{UserID: "qa", Enabled: true})
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		_, ferr := e.svc.FetchDouban(e.ctx, src.ID)
		done <- ferr
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("第一次拉取没有开始")
	}
	_, err = e.svc.FetchDouban(e.ctx, src.ID)
	require.ErrorIs(t, err, ErrBusy)
	e.svc.doubanDue(e.ctx)
	close(release)
	select {
	case err = <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("第一次拉取没有结束")
	}
	assert.Len(t, started, 0, "被挡住的两次都没有去请求豆瓣")
	row, err := e.svc.doubanRow(e.ctx, src.ID)
	require.NoError(t, err)
	assert.Zero(t, row.Failures, "被挡住的不算失败")
}
