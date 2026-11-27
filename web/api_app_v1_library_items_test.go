package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/web/middleware"
)

// appLibraryItems 以只读令牌调媒体库内容的接口（路径里的 id 另外给），回应按契约校验。
func appLibraryItems(t *testing.T, srv *Server, id, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/app/v1/media/libraries/"+id+"/items"+query, nil)
	req.SetPathValue("id", id)
	req = req.WithContext(middleware.WithPrincipal(req.Context(), &middleware.Principal{Kind: middleware.KindAPIToken, ID: "1", Scopes: []string{"app:read"}}))
	w := httptest.NewRecorder()
	srv.appMediaLibraryItems(w, req)
	validateAppResponse(t, req, w)
	return w
}

// 媒体库的内容：电影一部一项（导入的已有文件也算），剧集一季一项；不带路径、种子名；分页、搜索；参数不对回 400，库不存在回 404
func TestAppMediaLibraryItems(t *testing.T) {
	e := newOrganizeServer(t)
	tvDir := filepath.Join(t.TempDir(), "tv")
	require.NoError(t, os.MkdirAll(tvDir, 0o755))
	require.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodPost, "/api/media/libraries", `{"name":"电影","kind":"movie","path":`+jsonPath(e.library)+`,"enabled":true}`).Code)
	require.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodPost, "/api/media/libraries", `{"name":"剧集","kind":"tv","path":`+jsonPath(tvDir)+`,"enabled":true}`).Code)
	var libs []models.MediaLibrary
	db := global.GlobalDB.DB
	require.NoError(t, db.Order("id").Find(&libs).Error)
	require.Len(t, libs, 2)
	movies, tv := strconv.Itoa(int(libs[0].ID)), strconv.Itoa(int(libs[1].ID))
	at := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
	rows := []models.MediaTransferHistory{
		{
			LibraryID: libs[0].ID, MediaType: models.MediaKindMovie, TMDBID: 278, Title: "肖申克的救赎", Year: 1994, Mode: models.MediaModeExisting,
			Status: models.MediaTransferDone, Size: 4 << 30, SourcePath: "/medias/Mv/肖申克的救赎 (1994)/a.mkv", TargetPath: "/medias/Mv/肖申克的救赎 (1994)/a.mkv",
			TorrentName: "The.Shawshank.Redemption.1994.1080p.BluRay", CreatedAt: at,
		},
		{
			LibraryID: libs[1].ID, MediaType: models.MediaKindTV, TMDBID: 100088, Title: "最后生还者", Year: 2023, Season: 1, Episode: 1, EpisodeEnd: 2,
			Status: models.MediaTransferDone, Size: 2 << 30, SourcePath: "/downloads/tlou/e1.mkv", TargetPath: tvDir + "/e1.mkv", CreatedAt: at,
		},
		{
			LibraryID: libs[1].ID, MediaType: models.MediaKindTV, TMDBID: 100088, Title: "最后生还者", Year: 2023, Season: 1, Episode: 5,
			Status: models.MediaTransferDone, Size: 1 << 30, SourcePath: "/downloads/tlou/e5.mkv", CreatedAt: at.Add(time.Hour),
		},
	}
	require.NoError(t, db.Create(&rows).Error)

	w := appLibraryItems(t, e.srv, movies, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	assert.NotContains(t, w.Body.String(), "/medias/", "不带路径")
	assert.NotContains(t, w.Body.String(), "BluRay", "不带种子名")
	var page AppPage[AppMediaLibraryItem]
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	assert.Equal(t, 1, page.Total)
	assert.Equal(t, 1, page.Page)
	assert.Equal(t, 20, page.PageSize)
	require.Len(t, page.Items, 1)
	assert.Equal(t, AppMediaLibraryItem{
		MediaType: models.MediaKindMovie, TMDBID: 278, Title: "肖申克的救赎", Year: 1994, Episodes: []int{}, Files: 1, Size: 4 << 30,
		LatestAt: at.Unix(),
	}, page.Items[0], "导入的已有文件也算；电影的集号是空数组")

	w = appLibraryItems(t, e.srv, tv, "?page_size=1")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	assert.Equal(t, 1, page.Total)
	require.Len(t, page.Items, 1)
	assert.Equal(t, []int{1, 2, 5}, page.Items[0].Episodes)
	assert.Equal(t, 1, page.Items[0].Season)
	assert.Equal(t, 2, page.Items[0].Files)
	assert.Equal(t, int64(3<<30), page.Items[0].Size)
	assert.Equal(t, at.Add(time.Hour).Unix(), page.Items[0].LatestAt)

	w = appLibraryItems(t, e.srv, tv, "?q=%E7%94%9F%E8%BF%98&page=2&page_size=1")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	assert.Equal(t, 1, page.Total, "按片名搜到")
	assert.Empty(t, page.Items, "第 2 页没有了")
	w = appLibraryItems(t, e.srv, tv, "?q=nothing")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	assert.Zero(t, page.Total)
	assert.NotNil(t, page.Items)

	for _, c := range []struct{ id, query, code string }{
		{"abc", "", "invalid_argument"},
		{"0", "", "invalid_argument"},
		{"-1", "", "invalid_argument"},
		{movies, "?page_size=101", "invalid_argument"},
		{movies, "?page=0", "invalid_argument"},
		{movies, "?q=" + strings.Repeat("a", 201), "invalid_argument"},
	} {
		w = appLibraryItems(t, e.srv, c.id, c.query)
		assert.Equal(t, http.StatusBadRequest, w.Code, c.id+c.query)
		assert.Equal(t, c.code, errorCode(t, w), c.id+c.query)
	}
	w = appLibraryItems(t, e.srv, "999", "")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "not_found", errorCode(t, w))
}

// 没有整理入库服务时回 503
func TestAppMediaLibraryItemsUnavailable(t *testing.T) {
	srv := setupServer(t)
	srv.mgr.StopAll()
	assert.Equal(t, http.StatusServiceUnavailable, appLibraryItems(t, srv, "1", "").Code)
}

// meta 写明有媒体库内容的接口（App 据此显示「媒体库」标签）
func TestAppMetaMediaLibraryFeature(t *testing.T) {
	assert.Contains(t, appFeatures, "media_library")
}
