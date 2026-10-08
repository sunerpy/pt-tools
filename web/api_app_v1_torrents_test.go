package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/web/middleware"
)

func appAsWith(h http.HandlerFunc, method, target, body string, scopes ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	req = req.WithContext(middleware.WithPrincipal(req.Context(), &middleware.Principal{Kind: middleware.KindAPIToken, ID: "1", Scopes: scopes}))
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

// 种子列表：按下载器聚合、筛选、排序、分页；下载器取不到时写进 failures
func TestAppTorrents(t *testing.T) {
	fake := &fakeDownloader{torrents: sampleTorrents()}
	srv, dlID := setupServerWithFakeDownloader(t, fake)
	w := appAs(srv.appTorrents, http.MethodGet, "/api/app/v1/torrents?sort=size&order=asc&page_size=1")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	var page AppTorrentPage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	assert.Equal(t, 2, page.Total)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "Alpha Movie", page.Items[0].Title, "体积小的在前")
	assert.Equal(t, dlID, page.Items[0].DownloaderID)
	assert.InDelta(t, 50, page.Items[0].Progress, 0.01, "进度按百分比")
	assert.Empty(t, page.Failures)

	w = appAs(srv.appTorrents, http.MethodGet, "/api/app/v1/torrents?q=beta&downloader_id="+strconv.Itoa(int(dlID)))
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.Len(t, page.Items, 1)
	assert.Equal(t, "Beta Show", page.Items[0].Title)

	for _, q := range []string{"?sort=passkey", "?order=up", "?downloader_id=x", "?downloader_id=0", "?page_size=201"} {
		assert.Equal(t, http.StatusBadRequest, appAs(srv.appTorrents, http.MethodGet, "/api/app/v1/torrents"+q).Code, q)
	}

	fake.listErr = errors.New(`获取失败: Get "http://qb:8080/api?token=abc": EOF`)
	w = appAs(srv.appTorrents, http.MethodGet, "/api/app/v1/torrents")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.Len(t, page.Failures, 1)
	assert.NotContains(t, page.Failures[0].Error, "token=abc", "错误里地址的查询串去掉")
}

// 种子操作：只认 pause、resume、delete、delete_with_files，targets 1 到 100 个，严格解析
func TestAppTorrentActions(t *testing.T) {
	fake := &fakeDownloader{torrents: sampleTorrents()}
	srv, dlID := setupServerWithFakeDownloader(t, fake)
	id := strconv.Itoa(int(dlID))
	body := `{"action":"pause","targets":[{"downloader_id":` + id + `,"task_id":"t1"},{"downloader_id":999,"task_id":"t2"}]}`
	w := appAsWith(srv.appTorrentActions, http.MethodPost, "/api/app/v1/torrents/actions", body, "app:write")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var res AppTorrentActionsResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, 1, res.Succeeded)
	assert.Equal(t, 1, res.Failed, "不存在的下载器")
	require.Len(t, res.Results, 2)

	fake.batchRemoveErr = errors.New("batch")
	fake.removeErr = errors.New("单个也失败")
	w = appAsWith(srv.appTorrentActions, http.MethodPost, "/api/app/v1/torrents/actions",
		`{"action":"delete_with_files","targets":[{"downloader_id":`+id+`,"task_id":"t1"}]}`, "app:write")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, 1, res.Failed)
	assert.Equal(t, "单个也失败", res.Results[0].Message)

	for _, bad := range []string{
		`{"action":"set_location","targets":[{"downloader_id":1,"task_id":"t1"}]}`,
		`{"action":"pause","targets":[]}`,
		`{"action":"pause","targets":[{"downloader_id":0,"task_id":"t1"}]}`,
		`{"action":"pause","targets":[{"downloader_id":1,"task_id":""}]}`,
		`{"action":"pause","targets":[{"downloader_id":1,"task_id":"t1"}],"save_path":"/x"}`,
		`{"action":"pause"} {}`,
		`not json`,
	} {
		assert.Equal(t, http.StatusBadRequest, appAsWith(srv.appTorrentActions, http.MethodPost, "/api/app/v1/torrents/actions", bad, "app:write").Code, bad)
	}
	many := `{"action":"pause","targets":[`
	for i := range 101 {
		if i > 0 {
			many += ","
		}
		many += `{"downloader_id":1,"task_id":"t` + strconv.Itoa(i) + `"}`
	}
	many += `]}`
	assert.Equal(t, http.StatusBadRequest, appAsWith(srv.appTorrentActions, http.MethodPost, "/api/app/v1/torrents/actions", many, "app:write").Code, "最多 100 个")
}
