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
