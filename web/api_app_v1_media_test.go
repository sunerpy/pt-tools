package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/web/middleware"
)

// appWithID 调带 {id} 的处理函数。
func appWithID(t testing.TB, h http.HandlerFunc, method, target, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.SetPathValue("id", id)
	req = req.WithContext(middleware.WithPrincipal(req.Context(), &middleware.Principal{Kind: middleware.KindAPIToken, ID: "1", Scopes: []string{"app:read", "app:write"}}))
	w := httptest.NewRecorder()
	h(w, req)
	validateAppResponse(t, req, w)
	return w
}

// 订阅：建、列、详情、暂停、立即搜索、删除；来源记成 app；探索标出已订阅
func TestAppSubscriptions(t *testing.T) {
	srv, _ := newSubscribeEnv(t)
	w := appAsWith(t, srv.appSubscriptionCreate, http.MethodPost, "/api/app/v1/subscriptions", `{"media_type":"movie","tmdb_id":278}`, "app:write")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	var sub AppSubscription
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sub))
	assert.Equal(t, "肖申克的救赎", sub.Title)
	assert.Equal(t, models.MediaSubFromApp, sub.Source)
	assert.Equal(t, models.MediaSubActive, sub.Status)
	id := strconv.Itoa(int(sub.ID))

	for _, c := range []struct {
		body string
		want int
	}{
		{`{"media_type":"movie","tmdb_id":278}`, http.StatusBadRequest},
		{`{"media_type":"book","tmdb_id":1}`, http.StatusBadRequest},
		{`{"media_type":"movie","tmdb_id":999}`, http.StatusNotFound},
		{`{"media_type":"movie","tmdb_id":278,"sites":["x"]}`, http.StatusBadRequest},
	} {
		assert.Equal(t, c.want, appAsWith(t, srv.appSubscriptionCreate, http.MethodPost, "/api/app/v1/subscriptions", c.body, "app:write").Code, c.body)
	}

	w = appAsWith(t, srv.appSubscriptions, http.MethodGet, "/api/app/v1/subscriptions?status=active", "", "app:read")
	require.Equal(t, http.StatusOK, w.Code)
	var list []AppSubscription
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list, 1)
	assert.Equal(t, []int{1}, list[0].Progress.Missing)

	// 订阅的消息里有推送失败时下载器给的错误：交出去时去掉下载器的地址
	require.NoError(t, global.GlobalDB.DB.Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).
		Update("message", `推送失败: Post "http://10.0.0.5:8080/api/v2/torrents/add": dial tcp 10.0.0.5:8080: connect: connection refused`).Error)
	w = appAsWith(t, srv.appSubscriptions, http.MethodGet, "/api/app/v1/subscriptions", "", "app:read")
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "10.0.0.5")
	assert.Contains(t, w.Body.String(), "connection refused")

	w = appWithID(t, srv.appSubscriptionDetail, http.MethodGet, "/api/app/v1/subscriptions/"+id, id, "")
	require.Equal(t, http.StatusOK, w.Code)
	assertNoSecrets(t, w.Body.Bytes())
	var d AppSubscriptionDetail
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &d))
	assert.Empty(t, d.Torrents)
	assert.Equal(t, http.StatusNotFound, appWithID(t, srv.appSubscriptionDetail, http.MethodGet, "/api/app/v1/subscriptions/99", "99", "").Code)
	assert.Equal(t, http.StatusBadRequest, appWithID(t, srv.appSubscriptionDetail, http.MethodGet, "/api/app/v1/subscriptions/x", "x", "").Code)

	w = appWithID(t, srv.appSubscriptionSearch, http.MethodPost, "/api/app/v1/subscriptions/"+id+"/search", id, "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "搜索服务没有启动")
	w = appWithID(t, srv.appSubscriptionSetStatus, http.MethodPost, "/api/app/v1/subscriptions/"+id+"/status", id, `{"status":"paused"}`)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"status":"paused"`)
	assert.Equal(t, http.StatusBadRequest, appWithID(t, srv.appSubscriptionSearch, http.MethodPost, "/api/app/v1/subscriptions/"+id+"/search", id, "").Code, "暂停的不搜")
	assert.Equal(t, http.StatusBadRequest, appWithID(t, srv.appSubscriptionSetStatus, http.MethodPost, "/api/app/v1/subscriptions/"+id+"/status", id, `{"status":"done"}`).Code)

	w = appAsWith(t, srv.appExplore, http.MethodGet, "/api/app/v1/explore?kind=movie", "", "app:read")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	var page AppExplorePage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.NotEmpty(t, page.Items)
	assert.True(t, page.Items[0].Subscribed)
	assert.Equal(t, http.StatusBadRequest, appAsWith(t, srv.appExplore, http.MethodGet, "/api/app/v1/explore?kind=book", "", "app:read").Code)
	assert.Equal(t, http.StatusBadRequest, appAsWith(t, srv.appExplore, http.MethodGet, "/api/app/v1/explore?kind=movie&page=0", "", "app:read").Code)

	require.Equal(t, http.StatusOK, appWithID(t, srv.appSubscriptionDelete, http.MethodDelete, "/api/app/v1/subscriptions/"+id, id, "").Code)
	assert.Equal(t, http.StatusNotFound, appWithID(t, srv.appSubscriptionDelete, http.MethodDelete, "/api/app/v1/subscriptions/"+id, id, "").Code)

	bare := setupServer(t)
	bare.mgr.StopAll()
	assert.Equal(t, http.StatusServiceUnavailable, appAsWith(t, bare.appSubscriptions, http.MethodGet, "/api/app/v1/subscriptions", "", "app:read").Code)
	assert.Equal(t, http.StatusServiceUnavailable, appAsWith(t, bare.appMediaHistory, http.MethodGet, "/api/app/v1/media/history", "", "app:read").Code)
}

// 刷流任务：名字、站点、下载器、在刷的种子数与收益
func TestAppBrushTasks(t *testing.T) {
	srv, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.BrushTask{}, &models.BrushTorrent{}, &models.BrushTorrentSample{}, &models.BrushDailyStat{}, &models.DownloaderSetting{}))
	ds := models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true}
	require.NoError(t, db.Create(&ds).Error)
	require.NoError(t, models.NewBrushRepository(db).SaveTask(&models.BrushTask{Name: "刷 hdsky", SiteName: "hdsky", DownloaderID: ds.ID, IntervalMin: 30}))
	w := appAs(t, srv.appBrushTasks, http.MethodGet, "/api/app/v1/brush/tasks")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	var tasks []AppBrushTask
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &tasks))
	require.Len(t, tasks, 1)
	assert.Equal(t, "刷 hdsky", tasks[0].Name)
	assert.Equal(t, "qb", tasks[0].Downloader)
}

// 整理历史：分页，按状态与关键字
func TestAppMediaHistory(t *testing.T) {
	srv, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.MediaTransferHistory{}, &models.MediaLibrary{}))
	srv.SetOrganizeService(organize.New(organize.Config{DB: global.GlobalDB.DB}))
	for i, title := range []string{"沙丘2", "奥本海默"} {
		require.NoError(t, db.Create(&models.MediaTransferHistory{
			SourcePath: "/dl/" + strconv.Itoa(i), TargetPath: "/lib/" + title + ".mkv", MediaType: models.MediaKindMovie, Title: title,
			Status: models.MediaTransferDone, Message: `失败: Get "https://x/a?apikey=k": EOF`,
		}).Error)
	}
	w := appAs(t, srv.appMediaHistory, http.MethodGet, "/api/app/v1/media/history?q=沙丘&page_size=10")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	assert.NotContains(t, w.Body.String(), "apikey=k")
	var page AppPage[AppMediaHistory]
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	assert.Equal(t, 1, page.Total)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "沙丘2", page.Items[0].Title)
	assert.Equal(t, http.StatusBadRequest, appAs(t, srv.appMediaHistory, http.MethodGet, "/api/app/v1/media/history?page=0").Code)
}

// TMDB 图片经 pt-tools 去取：App 不直接连 TMDB（国内常常连不上，也不泄露给 TMDB 这台手机）
func TestAppTMDBImage(t *testing.T) {
	srv := setupServer(t)
	srv.mgr.StopAll()
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(&models.MediaSetting{}, &models.MediaWordRule{}, &models.MediaOverride{}, &models.MediaCache{}))
	png := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 64))
	img := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/t/p/w342/poster.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		case "/t/p/w342/text.jpg":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(img.Close)
	call := func(size, file string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/app/v1/images/tmdb/"+size+"/"+file, nil)
		req.SetPathValue("size", size)
		req.SetPathValue("file", file)
		req = req.WithContext(middleware.WithPrincipal(req.Context(), &middleware.Principal{Kind: middleware.KindAPIToken, ID: "1", Scopes: []string{"app:read"}}))
		w := httptest.NewRecorder()
		srv.appTMDBImage(w, req)
		// 文件名带 / 的请求在契约里没有对应的路由（经 mux 的真实请求也到不了这里），只看状态码
		if !strings.Contains(file, "/") {
			validateAppResponse(t, req, w)
		}
		return w
	}
	// 没有媒体识别服务：503
	assert.Equal(t, http.StatusServiceUnavailable, call("w342", "poster.png").Code)

	rec := recognize.New(recognize.Config{DB: db, Cipher: apiTestCipher{}, BaseURL: img.URL + "/3", ImageBaseURL: img.URL + "/t/p/", RatePerSecond: 1000})
	srv.SetMediaService(rec)
	// 没有 TMDB API Key：400
	assert.Equal(t, http.StatusBadRequest, call("w342", "poster.png").Code)
	key := mediaTestKey
	_, err := rec.SaveSettings(context.Background(), recognize.SettingsInput{TMDBKey: &key})
	require.NoError(t, err)

	w := call("w342", "poster.png")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "image/png", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Cache-Control"), "max-age=")
	assert.Equal(t, png, w.Body.Bytes())

	for _, c := range []struct {
		size, file string
		want       int
	}{
		{"w342", "missing.jpg", http.StatusNotFound},
		{"w342", "text.jpg", http.StatusBadGateway},
		{"x9", "poster.png", http.StatusBadRequest},
		{"original", "poster.png", http.StatusBadRequest},
		{"h632", "poster.png", http.StatusBadRequest},
		{"w342", "../etc.png", http.StatusBadRequest},
		{"w342", "a.gif", http.StatusBadRequest},
	} {
		assert.Equal(t, c.want, call(c.size, c.file).Code, c.size+"/"+c.file)
	}

	// 同时取图的名额用完：排队超时回 503 busy，名额还回来以后照常
	oldWait := appImageWait
	appImageWait = 20 * time.Millisecond
	t.Cleanup(func() { appImageWait = oldWait })
	for range cap(appImageSlots) {
		appImageSlots <- struct{}{}
	}
	w = call("w342", "poster.png")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), `"busy"`)
	assert.Equal(t, "5", w.Header().Get("Retry-After"))
	for range cap(appImageSlots) {
		<-appImageSlots
	}
	assert.Equal(t, http.StatusOK, call("w342", "poster.png").Code)
	assert.Empty(t, appImageSlots, "取完图名额还回去了")
}
