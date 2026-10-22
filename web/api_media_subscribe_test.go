package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

// newSubscribeServer 起一个接好订阅服务的接口：TMDB 是本地假服务（肖申克的救赎、趋势列表）。没有搜索与推送：立即搜索回「搜索服务没有启动」。
func newSubscribeServer(t *testing.T, opts ...func(*subscribe.Config)) *http.ServeMux {
	t.Helper()
	_, mux := newSubscribeEnv(t, opts...)
	return mux
}

// newSubscribeEnv 同 newSubscribeServer，另外把服务本身也交出来（App API 的测试直接调处理函数）。
func newSubscribeEnv(t *testing.T, opts ...func(*subscribe.Config)) (*Server, *http.ServeMux) {
	t.Helper()
	srv := setupServer(t)
	srv.mgr.StopAll()
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(
		&models.MediaSetting{}, &models.MediaWordRule{}, &models.MediaOverride{}, &models.MediaCache{}, &models.MediaTransferHistory{},
		&models.MediaQualityProfile{}, &models.MediaSubscription{}, &models.MediaSubscriptionTorrent{}, &models.MediaSubscribeSetting{},
		&models.MediaDoubanSource{}, &models.MediaDoubanItem{},
		&models.MediaSubscriptionEpisode{}, &models.MediaEpisodeRule{}, &models.MediaSubscriptionRun{}, &models.MediaLibrary{},
	))
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/3/movie/278" && r.URL.Query().Get("language") == "en-US":
			_, _ = w.Write([]byte(`{"id":278,"title":"The Shawshank Redemption","alternative_titles":{"titles":[]}}`))
		case r.URL.Path == "/3/movie/278":
			_, _ = w.Write([]byte(`{"id":278,"title":"肖申克的救赎","original_title":"The Shawshank Redemption","release_date":"1994-09-23","imdb_id":"tt0111161","poster_path":"/s.jpg"}`))
		case r.URL.Path == "/3/trending/movie/week":
			_, _ = w.Write([]byte(`{"page":1,"total_pages":1,"results":[{"id":278,"title":"肖申克的救赎","release_date":"1994-09-23"}]}`))
		case r.URL.Path == "/3/tv/1001" && r.URL.Query().Get("language") == "en-US":
			_, _ = w.Write([]byte(`{"id":1001,"name":"Fake Variety","alternative_titles":{"results":[]}}`))
		case r.URL.Path == "/3/tv/1001":
			_, _ = w.Write([]byte(`{"id":1001,"name":"假综艺","original_name":"假综艺","first_air_date":"2026-01-01","number_of_seasons":1,"genres":[{"id":10764,"name":"真人秀"}]}`))
		case r.URL.Path == "/3/tv/1001/season/1":
			_, _ = w.Write([]byte(`{"season_number":1,"air_date":"2026-01-01","episodes":[{"episode_number":1,"name":"第1期上","air_date":"2026-01-01"},{"episode_number":2,"name":"第1期下","air_date":"2026-01-01"}]}`))
		case r.URL.Path == "/3/search/movie" && strings.Contains(r.URL.Query().Get("query"), "肖申克"):
			_, _ = w.Write([]byte(`{"results":[{"id":278,"title":"肖申克的救赎","release_date":"1994-09-23"}]}`))
		case strings.HasPrefix(r.URL.Path, "/3/search/"):
			_, _ = w.Write([]byte(`{"results":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(fake.Close)
	rec := recognize.New(recognize.Config{DB: db, Cipher: apiTestCipher{}, BaseURL: fake.URL + "/3", RatePerSecond: 1000})
	key := mediaTestKey
	_, err := rec.SaveSettings(context.Background(), recognize.SettingsInput{TMDBKey: &key})
	require.NoError(t, err)
	cfg := subscribe.Config{DB: db, Recognizer: rec}
	for _, o := range opts {
		o(&cfg)
	}
	srv.SetSubscribeService(subscribe.New(cfg))
	mux := http.NewServeMux()
	srv.registerSubscribeRoutes(mux)
	srv.sessions.put("sess-test", "admin")
	return srv, mux
}

var subscribeRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/media/subscribe/settings"},
	{http.MethodPut, "/api/media/subscribe/settings"},
	{http.MethodGet, "/api/media/quality-profiles"},
	{http.MethodPost, "/api/media/quality-profiles"},
	{http.MethodPut, "/api/media/quality-profiles/1"},
	{http.MethodDelete, "/api/media/quality-profiles/1"},
	{http.MethodGet, "/api/media/subscriptions"},
	{http.MethodPost, "/api/media/subscriptions"},
	{http.MethodGet, "/api/media/subscriptions/1"},
	{http.MethodPut, "/api/media/subscriptions/1"},
	{http.MethodDelete, "/api/media/subscriptions/1"},
	{http.MethodPost, "/api/media/subscriptions/1/status"},
	{http.MethodPost, "/api/media/subscriptions/1/search"},
	{http.MethodGet, "/api/media/subscriptions/1/episodes"},
	{http.MethodPost, "/api/media/subscriptions/1/episodes/1/skip"},
	{http.MethodPost, "/api/media/subscriptions/1/episodes/1/unlock"},
	{http.MethodGet, "/api/media/subscriptions/1/runs"},
	{http.MethodPost, "/api/media/subscriptions/schedule-preview"},
	{http.MethodPost, "/api/media/subscriptions/1/torrents/1/organize"},
	{http.MethodPost, "/api/media/subscriptions/1/torrents/1/scrape"},
	{http.MethodGet, "/api/media/subscriptions/1/episode-rules"},
	{http.MethodPut, "/api/media/subscriptions/1/episode-rules"},
	{http.MethodDelete, "/api/media/subscriptions/1/episode-rules"},
	{http.MethodGet, "/api/media/douban-sources"},
	{http.MethodPost, "/api/media/douban-sources"},
	{http.MethodPut, "/api/media/douban-sources/1"},
	{http.MethodDelete, "/api/media/douban-sources/1"},
	{http.MethodPost, "/api/media/douban-sources/1/fetch"},
	{http.MethodGet, "/api/media/douban-sources/1/items"},
	{http.MethodGet, "/api/media/explore"},
}

func TestSubscribeAPI_RequiresSessionAndService(t *testing.T) {
	mux := newSubscribeServer(t)
	for _, c := range subscribeRoutes {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(c.method, c.path, nil))
		assert.Equal(t, http.StatusUnauthorized, w.Code, c.method+" "+c.path)
	}
	srv := setupServer(t)
	srv.mgr.StopAll()
	bare := http.NewServeMux()
	srv.registerSubscribeRoutes(bare)
	srv.sessions.put("sess-test", "admin")
	for _, c := range subscribeRoutes {
		assert.Equal(t, http.StatusServiceUnavailable, serveAuthed(bare, c.method, c.path, "{}").Code, c.method+" "+c.path)
	}
}

func TestSubscribeAPI_Flow(t *testing.T) {
	mux := newSubscribeServer(t)
	w := serveAuthed(mux, http.MethodGet, "/api/media/subscribe/settings", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"search_interval_hours":12`)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPut, "/api/media/subscribe/settings", `{"enable":true}`).Code, "拼错的字段")
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPut, "/api/media/subscribe/settings", `{"search_interval_hours":1}`).Code)
	w = serveAuthed(mux, http.MethodPut, "/api/media/subscribe/settings", `{"enabled":true,"search_interval_hours":24,"search_skip_sites":["mteam"],"notify_channels":[1],"upgrade_old":"keep"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"search_skip_sites":["mteam"]`)

	w = serveAuthed(mux, http.MethodGet, "/api/media/quality-profiles", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"resolutions":["4320p","2160p"`)
	w = serveAuthed(mux, http.MethodPost, "/api/media/quality-profiles", `{"name":"4K","resolutions":["2160p"],"hdr":"prefer"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var prof subscribe.Profile
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &prof))
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/quality-profiles", `{"name":"x","resolutions":["8K"]}`).Code)
	pid := strconv.Itoa(int(prof.ID))
	assert.Equal(t, http.StatusOK, serveAuthed(mux, http.MethodPut, "/api/media/quality-profiles/"+pid, `{"name":"4K HDR","resolutions":["2160p"]}`).Code)

	w = serveAuthed(mux, http.MethodPost, "/api/media/subscriptions", `{"media_type":"movie","tmdb_id":278,"profile_id":`+pid+`,"sites":["hdsky"]}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var sub models.MediaSubscription
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sub))
	assert.Equal(t, "肖申克的救赎", sub.Title)
	assert.NotContains(t, w.Body.String(), "aliases", "内部字段不回")
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/subscriptions", `{"media_type":"movie","tmdb_id":278}`).Code, "订阅过了")
	assert.Equal(t, http.StatusNotFound, serveAuthed(mux, http.MethodPost, "/api/media/subscriptions", `{"media_type":"movie","tmdb_id":999}`).Code, "TMDB 上没有")
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodDelete, "/api/media/quality-profiles/"+pid, "").Code, "订阅在用")
	sid := strconv.Itoa(int(sub.ID))

	w = serveAuthed(mux, http.MethodGet, "/api/media/subscriptions?status=active&q=肖申克", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"sites":["hdsky"]`)
	assert.Contains(t, w.Body.String(), `"missing":[1]`)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodGet, "/api/media/subscriptions?status=bad", "").Code)
	w = serveAuthed(mux, http.MethodGet, "/api/media/subscriptions/"+sid, "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"torrent_list":[]`)
	assert.Equal(t, http.StatusNotFound, serveAuthed(mux, http.MethodGet, "/api/media/subscriptions/99", "").Code)
	w = serveAuthed(mux, http.MethodPut, "/api/media/subscriptions/"+sid, `{"upgrade":true,"category":"movies"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"upgrade":true`)
	w = serveAuthed(mux, http.MethodPost, "/api/media/subscriptions/"+sid+"/search", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "搜索服务没有启动")
	w = serveAuthed(mux, http.MethodPost, "/api/media/subscriptions/"+sid+"/status", `{"status":"paused"}`)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"status":"paused"`)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/subscriptions/"+sid+"/search", "").Code, "暂停的不搜")
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/subscriptions/"+sid+"/status", `{"status":"done"}`).Code)

	w = serveAuthed(mux, http.MethodGet, "/api/media/explore?kind=movie&list=trending&page=1", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"subscribed":true`)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodGet, "/api/media/explore?kind=movie&list=trending&page=x", "").Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodGet, "/api/media/explore?kind=book&list=trending", "").Code)

	w = serveAuthed(mux, http.MethodPost, "/api/media/douban-sources", `{"user_id":"qa","enabled":false}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var src models.MediaDoubanSource
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &src))
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/douban-sources", `{"user_id":"a b"}`).Code)
	w = serveAuthed(mux, http.MethodGet, "/api/media/douban-sources", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"user_id":"qa"`)
	assert.NotContains(t, w.Body.String(), "notified", "内部字段不回")
	did := strconv.Itoa(int(src.ID))
	w = serveAuthed(mux, http.MethodGet, "/api/media/douban-sources/"+did+"/items", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "[]", strings.TrimSpace(w.Body.String()))
	assert.Equal(t, http.StatusOK, serveAuthed(mux, http.MethodDelete, "/api/media/douban-sources/"+did, "").Code)

	assert.Equal(t, http.StatusOK, serveAuthed(mux, http.MethodDelete, "/api/media/subscriptions/"+sid, "").Code)
	assert.Equal(t, http.StatusOK, serveAuthed(mux, http.MethodDelete, "/api/media/quality-profiles/"+pid, "").Code)
}

// 豆瓣立即拉取：建订阅；来源不存在 404；路径里的编号不对 400
func TestSubscribeAPI_DoubanFetch(t *testing.T) {
	feed := `<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>qa 的收藏</title>
<item><title>想看肖申克的救赎</title><link>https://movie.douban.com/subject/1292052/</link>
<description><![CDATA[<img src="x.jpg" title="The Shawshank Redemption" />]]></description></item></channel></rss>`
	douban := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/feed/people/qa/interests" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(feed))
	}))
	t.Cleanup(douban.Close)
	mux := newSubscribeServer(t, func(c *subscribe.Config) { c.DoubanBase = douban.URL })
	w := serveAuthed(mux, http.MethodPost, "/api/media/douban-sources", `{"user_id":"qa","enabled":true,"confirm":true}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var src models.MediaDoubanSource
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &src))
	did := strconv.Itoa(int(src.ID))
	w = serveAuthed(mux, http.MethodPost, "/api/media/douban-sources/"+did+"/fetch", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"created":1}`, w.Body.String())
	w = serveAuthed(mux, http.MethodGet, "/api/media/douban-sources/"+did+"/items", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"status":"subscribed"`)

	for _, c := range []struct {
		method, path, body string
		want               int
	}{
		{http.MethodPost, "/api/media/douban-sources/99/fetch", "", http.StatusNotFound},
		{http.MethodPost, "/api/media/douban-sources/x/fetch", "", http.StatusBadRequest},
		{http.MethodGet, "/api/media/douban-sources/99/items", "", http.StatusNotFound},
		{http.MethodGet, "/api/media/douban-sources/x/items", "", http.StatusBadRequest},
		{http.MethodDelete, "/api/media/douban-sources/99", "", http.StatusNotFound},
		{http.MethodDelete, "/api/media/douban-sources/x", "", http.StatusBadRequest},
		{http.MethodPut, "/api/media/douban-sources/x", "{}", http.StatusBadRequest},
		{http.MethodPut, "/api/media/subscriptions/x", "{}", http.StatusBadRequest},
		{http.MethodPut, "/api/media/subscriptions/99", "{}", http.StatusNotFound},
		{http.MethodPut, "/api/media/subscriptions/99", `{"bogus":1}`, http.StatusBadRequest},
		{http.MethodDelete, "/api/media/subscriptions/x", "", http.StatusBadRequest},
		{http.MethodDelete, "/api/media/subscriptions/99", "", http.StatusNotFound},
		{http.MethodPost, "/api/media/subscriptions/x/status", `{"status":"paused"}`, http.StatusBadRequest},
		{http.MethodPost, "/api/media/subscriptions/x/search", "", http.StatusBadRequest},
		{http.MethodDelete, "/api/media/quality-profiles/x", "", http.StatusBadRequest},
		{http.MethodPut, "/api/media/quality-profiles/x", "{}", http.StatusBadRequest},
	} {
		assert.Equal(t, c.want, serveAuthed(mux, c.method, c.path, c.body).Code, c.method+" "+c.path)
	}
}

// 订阅接口的错误码：参数不对 400、找不到 404、正在处理 409、TMDB 限流 429、超时 504、TMDB 不能访问 502、其他 500
func TestWriteSubscribeError(t *testing.T) {
	for _, c := range []struct {
		err  error
		want int
	}{
		{fmt.Errorf("%w: x", subscribe.ErrInvalid), http.StatusBadRequest},
		{tmdb.ErrNoKey, http.StatusBadRequest},
		{tmdb.ErrUnauthorized, http.StatusBadRequest},
		{subscribe.ErrNotFound, http.StatusNotFound},
		{tmdb.ErrNotFound, http.StatusNotFound},
		{fmt.Errorf("%w：正在拉取", subscribe.ErrBusy), http.StatusConflict},
		{tmdb.ErrRateLimited, http.StatusTooManyRequests},
		{context.DeadlineExceeded, http.StatusGatewayTimeout},
		{tmdb.ErrUnavailable, http.StatusBadGateway},
		{errors.New("boom"), http.StatusInternalServerError},
	} {
		w := httptest.NewRecorder()
		writeSubscribeError(w, c.err)
		assert.Equal(t, c.want, w.Code, c.err.Error())
	}
}

func TestSubscribeAPI_Follow(t *testing.T) {
	mux := newSubscribeServer(t)
	w := serveAuthed(mux, http.MethodPost, "/api/media/subscriptions", `{"media_type":"tv","tmdb_id":1001,"season":1}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var sub struct {
		ID          uint `json:"id"`
		LockVersion bool `json:"lock_version"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sub))
	assert.True(t, sub.LockVersion, "新建的剧集按追剧的默认设置")
	id := strconv.Itoa(int(sub.ID))
	w = serveAuthed(mux, http.MethodGet, "/api/media/subscriptions/"+id, "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"schedule":{"kind":"airtime"}`)
	assert.Contains(t, w.Body.String(), `"required":2`)

	w = serveAuthed(mux, http.MethodGet, "/api/media/subscriptions/"+id+"/episodes", "")
	require.Equal(t, http.StatusOK, w.Code)
	var eps []subscribe.EpisodeState
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &eps))
	require.Len(t, eps, 2)
	assert.Equal(t, "第1期上", eps[0].Name)

	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/subscriptions/"+id+"/episodes/1/skip", `{}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/subscriptions/"+id+"/episodes/x/skip", `{"skip":true}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/subscriptions/"+id+"/episodes/9/skip", `{"skip":true}`).Code, "这一季没有第 9 集")
	w = serveAuthed(mux, http.MethodPost, "/api/media/subscriptions/"+id+"/episodes/1/skip", `{"skip":true}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"state":"skipped"`)
	w = serveAuthed(mux, http.MethodPost, "/api/media/subscriptions/"+id+"/episodes/2/unlock", `{"unlock":true}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"manual_unlock":true`)

	w = serveAuthed(mux, http.MethodGet, "/api/media/subscriptions/"+id+"/runs?limit=2", "")
	require.Equal(t, http.StatusOK, w.Code)
	var runs subscribe.RunPage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &runs))
	assert.Equal(t, int64(3), runs.Total, "建立、跳过、换版本各一条")
	assert.Len(t, runs.Items, 2)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodGet, "/api/media/subscriptions/"+id+"/runs?limit=x", "").Code)

	w = serveAuthed(mux, http.MethodPut, "/api/media/subscriptions/"+id+"/episode-rules", `{"group":"XX","mode":"offset","offset":1}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"group":"XX"`)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPut, "/api/media/subscriptions/"+id+"/episode-rules", `{"mode":"guess"}`).Code)
	w = serveAuthed(mux, http.MethodDelete, "/api/media/subscriptions/"+id+"/episode-rules?group=XX", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "[]\n", w.Body.String())
	assert.Equal(t, http.StatusNotFound, serveAuthed(mux, http.MethodDelete, "/api/media/subscriptions/"+id+"/episode-rules?group=XX", "").Code)

	// 改定时：每周六 21:00
	w = serveAuthed(mux, http.MethodPut, "/api/media/subscriptions/"+id, `{"schedule":{"kind":"weekly","days":[6],"at":"21:00"}}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = serveAuthed(mux, http.MethodGet, "/api/media/subscriptions/"+id, "")
	assert.Contains(t, w.Body.String(), `"schedule":{"kind":"weekly","at":"21:00","days":[6]}`)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPut, "/api/media/subscriptions/"+id, `{"schedule":{"kind":"hourly"}}`).Code)

	// 电影没有追剧的设置
	w = serveAuthed(mux, http.MethodPost, "/api/media/subscriptions", `{"media_type":"movie","tmdb_id":278}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"lock_version":false`)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodGet, "/api/media/subscriptions/2/episodes", "").Code, "电影没有分集")
}

// 网页的定时预览：几次搜索的时间；按播出时间的不算，写明原因；表达式不对回 400
func TestSubscribeAPI_SchedulePreview(t *testing.T) {
	mux := newSubscribeServer(t)
	w := serveAuthed(mux, http.MethodPost, "/api/media/subscriptions/schedule-preview", `{"schedule":{"kind":"cron","cron":"0 20 * * 5"},"media_type":"tv","count":3}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var v SchedulePreviewView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &v))
	require.Len(t, v.Next, 3)
	for i, next := range v.Next {
		local := next.In(time.Local)
		assert.Equal(t, time.Friday, local.Weekday(), i)
		assert.Equal(t, 20, local.Hour(), i)
		if i > 0 {
			assert.Equal(t, 7*24*time.Hour, next.Sub(v.Next[i-1]).Round(time.Hour), i)
		}
	}
	w = serveAuthed(mux, http.MethodPost, "/api/media/subscriptions/schedule-preview", `{"schedule":{"kind":"interval","interval_minutes":90}}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	v = SchedulePreviewView{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &v))
	require.Len(t, v.Next, 5, "默认 5 次")
	assert.Equal(t, 90*time.Minute, v.Next[1].Sub(v.Next[0]))
	w = serveAuthed(mux, http.MethodPost, "/api/media/subscriptions/schedule-preview", `{"schedule":{"kind":"airtime"},"media_type":"tv"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"next":[]`)
	assert.Contains(t, w.Body.String(), "播出时间")
	for _, body := range []string{
		`{"schedule":{"kind":"cron","cron":"0 25 * * *"}}`,
		`{"schedule":{"kind":"interval","interval_minutes":1}}`,
		`{"schedule":{"kind":"cron","cron":"0 20 * * *"},"x":1}`,
		`{"schedule":{"kind":"daily","at":"7:00"},"subscription_id":99}`,
	} {
		code := serveAuthed(mux, http.MethodPost, "/api/media/subscriptions/schedule-preview", body).Code
		assert.Contains(t, []int{http.StatusBadRequest, http.StatusNotFound}, code, body)
	}
}
