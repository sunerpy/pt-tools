package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/web/middleware"
)

// appWithID 调带 {id} 的处理函数。
func appWithID(h http.HandlerFunc, method, target, id, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.SetPathValue("id", id)
	req = req.WithContext(middleware.WithPrincipal(req.Context(), &middleware.Principal{Kind: middleware.KindAPIToken, ID: "1", Scopes: []string{"app:read", "app:write"}}))
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

// 订阅：建、列、详情、暂停、立即搜索、删除；来源记成 app；探索标出已订阅
func TestAppSubscriptions(t *testing.T) {
	srv, _ := newSubscribeEnv(t)
	w := appAsWith(srv.appSubscriptionCreate, http.MethodPost, "/api/app/v1/subscriptions", `{"media_type":"movie","tmdb_id":278}`, "app:write")
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
		assert.Equal(t, c.want, appAsWith(srv.appSubscriptionCreate, http.MethodPost, "/api/app/v1/subscriptions", c.body, "app:write").Code, c.body)
	}

	w = appAsWith(srv.appSubscriptions, http.MethodGet, "/api/app/v1/subscriptions?status=active", "", "app:read")
	require.Equal(t, http.StatusOK, w.Code)
	var list []AppSubscription
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list, 1)
	assert.Equal(t, []int{1}, list[0].Progress.Missing)

	// 订阅的消息里有推送失败时下载器给的错误：交出去时去掉下载器的地址
	require.NoError(t, global.GlobalDB.DB.Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).
		Update("message", `推送失败: Post "http://10.0.0.5:8080/api/v2/torrents/add": dial tcp 10.0.0.5:8080: connect: connection refused`).Error)
	w = appAsWith(srv.appSubscriptions, http.MethodGet, "/api/app/v1/subscriptions", "", "app:read")
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "10.0.0.5")
	assert.Contains(t, w.Body.String(), "connection refused")

	w = appWithID(srv.appSubscriptionDetail, http.MethodGet, "/api/app/v1/subscriptions/"+id, id, "")
	require.Equal(t, http.StatusOK, w.Code)
	assertNoSecrets(t, w.Body.Bytes())
	var d AppSubscriptionDetail
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &d))
	assert.Empty(t, d.Torrents)
	assert.Equal(t, http.StatusNotFound, appWithID(srv.appSubscriptionDetail, http.MethodGet, "/x", "99", "").Code)
	assert.Equal(t, http.StatusBadRequest, appWithID(srv.appSubscriptionDetail, http.MethodGet, "/x", "x", "").Code)

	w = appWithID(srv.appSubscriptionSearch, http.MethodPost, "/x", id, "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "搜索服务没有启动")
	w = appWithID(srv.appSubscriptionSetStatus, http.MethodPost, "/x", id, `{"status":"paused"}`)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"status":"paused"`)
	assert.Equal(t, http.StatusBadRequest, appWithID(srv.appSubscriptionSearch, http.MethodPost, "/x", id, "").Code, "暂停的不搜")
	assert.Equal(t, http.StatusBadRequest, appWithID(srv.appSubscriptionSetStatus, http.MethodPost, "/x", id, `{"status":"done"}`).Code)

	w = appAsWith(srv.appExplore, http.MethodGet, "/api/app/v1/explore?kind=movie", "", "app:read")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	var page AppExplorePage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.NotEmpty(t, page.Items)
	assert.True(t, page.Items[0].Subscribed)
	assert.Equal(t, http.StatusBadRequest, appAsWith(srv.appExplore, http.MethodGet, "/api/app/v1/explore?kind=book", "", "app:read").Code)
	assert.Equal(t, http.StatusBadRequest, appAsWith(srv.appExplore, http.MethodGet, "/api/app/v1/explore?kind=movie&page=0", "", "app:read").Code)

	require.Equal(t, http.StatusOK, appWithID(srv.appSubscriptionDelete, http.MethodDelete, "/x", id, "").Code)
	assert.Equal(t, http.StatusNotFound, appWithID(srv.appSubscriptionDelete, http.MethodDelete, "/x", id, "").Code)

	bare := setupServer(t)
	bare.mgr.StopAll()
	assert.Equal(t, http.StatusServiceUnavailable, appAsWith(bare.appSubscriptions, http.MethodGet, "/x", "", "app:read").Code)
	assert.Equal(t, http.StatusServiceUnavailable, appAsWith(bare.appMediaHistory, http.MethodGet, "/x", "", "app:read").Code)
}

// 刷流任务：名字、站点、下载器、在刷的种子数与收益
func TestAppBrushTasks(t *testing.T) {
	srv, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.BrushTask{}, &models.BrushTorrent{}, &models.BrushTorrentSample{}, &models.BrushDailyStat{}, &models.DownloaderSetting{}))
	ds := models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true}
	require.NoError(t, db.Create(&ds).Error)
	require.NoError(t, models.NewBrushRepository(db).SaveTask(&models.BrushTask{Name: "刷 hdsky", SiteName: "hdsky", DownloaderID: ds.ID, IntervalMin: 30}))
	w := appAs(srv.appBrushTasks, http.MethodGet, "/api/app/v1/brush/tasks")
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
	w := appAs(srv.appMediaHistory, http.MethodGet, "/api/app/v1/media/history?q=沙丘&page_size=10")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	assert.NotContains(t, w.Body.String(), "apikey=k")
	var page AppPage[AppMediaHistory]
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	assert.Equal(t, 1, page.Total)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "沙丘2", page.Items[0].Title)
	assert.Equal(t, http.StatusBadRequest, appAs(srv.appMediaHistory, http.MethodGet, "/api/app/v1/media/history?page=0").Code)
}
