package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// organizeTestDLs 是整理入库用的下载器：只有 ID 1 的 qb，实例是 fakeDownloader。
type organizeTestDLs struct {
	dl      *fakeDownloader
	setting models.DownloaderSetting
}

func (d organizeTestDLs) Get(_ context.Context, id uint) (downloader.Downloader, models.DownloaderSetting, error) {
	if id != d.setting.ID {
		return nil, models.DownloaderSetting{}, errors.New("下载器不存在")
	}
	return d.dl, d.setting, nil
}

func (d organizeTestDLs) ByName(ctx context.Context, name string) (downloader.Downloader, models.DownloaderSetting, error) {
	return d.Get(ctx, d.setting.ID)
}

func (d organizeTestDLs) List(context.Context) ([]models.DownloaderSetting, error) {
	return []models.DownloaderSetting{d.setting}, nil
}

type organizeTestEnv struct {
	mux     *http.ServeMux
	dl      *fakeDownloader
	dlDir   string
	library string
}

// newOrganizeServer 起一个接好整理入库服务的接口：TMDB 是本地假服务（能搜到肖申克的救赎），
// 下载器里有一个下载完的种子，保存目录 /downloads 映射到临时目录。
func newOrganizeServer(t *testing.T) *organizeTestEnv {
	t.Helper()
	srv := setupServer(t)
	srv.mgr.StopAll()
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(
		&models.MediaSetting{}, &models.MediaWordRule{}, &models.MediaOverride{}, &models.MediaCache{},
		&models.MediaLibrary{}, &models.MediaPathMap{}, &models.MediaServer{}, &models.MediaOrganizeSetting{}, &models.MediaTransferHistory{},
	))
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/3/search/movie" && strings.Contains(r.URL.Query().Get("query"), "Shawshank"):
			_, _ = w.Write([]byte(`{"results":[{"id":278,"title":"肖申克的救赎","original_title":"The Shawshank Redemption","release_date":"1994-09-23"}]}`))
		case strings.HasPrefix(r.URL.Path, "/3/search/"):
			_, _ = w.Write([]byte(`{"results":[]}`))
		case r.URL.Path == "/3/movie/278":
			_, _ = w.Write([]byte(`{"id":278,"title":"肖申克的救赎","original_title":"The Shawshank Redemption","release_date":"1994-09-23","imdb_id":"tt0111161"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(fake.Close)
	rec := recognize.New(recognize.Config{DB: db, Cipher: apiTestCipher{}, BaseURL: fake.URL + "/3", RatePerSecond: 1000})
	key := mediaTestKey
	_, err := rec.SaveSettings(context.Background(), recognize.SettingsInput{TMDBKey: &key})
	require.NoError(t, err)

	root := t.TempDir()
	e := &organizeTestEnv{dlDir: filepath.Join(root, "downloads"), library: filepath.Join(root, "movies")}
	require.NoError(t, os.MkdirAll(e.library, 0o755))
	name := "The.Shawshank.Redemption.1994.1080p.BluRay.x264-WiKi"
	require.NoError(t, os.MkdirAll(filepath.Join(e.dlDir, name), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(e.dlDir, name, name+".mkv"), []byte("video"), 0o644))
	e.dl = &fakeDownloader{
		torrents: []downloader.Torrent{{ID: "hash1", InfoHash: "hash1", Name: name, Progress: 1, SavePath: "/downloads", State: downloader.TorrentSeeding}},
		files:    []downloader.TorrentFile{{Name: name + "/" + name + ".mkv", Size: 100 << 20}},
	}
	setting := models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://qb", Enabled: true}
	require.NoError(t, db.Create(&setting).Error)
	srv.SetOrganizeService(organize.New(organize.Config{
		DB: db, Cipher: apiTestCipher{}, Recognizer: rec, Downloaders: organizeTestDLs{dl: e.dl, setting: setting},
	}))
	e.mux = http.NewServeMux()
	srv.registerOrganizeRoutes(e.mux)
	srv.sessions.put("sess-test", "admin")
	return e
}

func TestOrganizeAPI_RequiresSessionAndService(t *testing.T) {
	e := newOrganizeServer(t)
	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/media/organize/settings"},
		{http.MethodPut, "/api/media/organize/settings"},
		{http.MethodPost, "/api/media/organize/preview"},
		{http.MethodPost, "/api/media/organize"},
		{http.MethodGet, "/api/media/libraries"},
		{http.MethodPost, "/api/media/libraries"},
		{http.MethodPut, "/api/media/libraries/1"},
		{http.MethodDelete, "/api/media/libraries/1"},
		{http.MethodPost, "/api/media/libraries/check"},
		{http.MethodPost, "/api/media/libraries/template-preview"},
		{http.MethodGet, "/api/media/path-maps"},
		{http.MethodPost, "/api/media/path-maps"},
		{http.MethodPut, "/api/media/path-maps/1"},
		{http.MethodDelete, "/api/media/path-maps/1"},
		{http.MethodGet, "/api/media/servers"},
		{http.MethodPost, "/api/media/servers"},
		{http.MethodPut, "/api/media/servers/1"},
		{http.MethodDelete, "/api/media/servers/1"},
		{http.MethodPost, "/api/media/servers/test"},
		{http.MethodGet, "/api/media/history"},
		{http.MethodPost, "/api/media/history/1/retry"},
		{http.MethodDelete, "/api/media/history/1"},
	}
	for _, c := range routes {
		w := httptest.NewRecorder()
		e.mux.ServeHTTP(w, httptest.NewRequest(c.method, c.path, nil))
		assert.Equal(t, http.StatusUnauthorized, w.Code, c.method+" "+c.path)
	}

	srv := setupServer(t)
	srv.mgr.StopAll()
	bare := http.NewServeMux()
	srv.registerOrganizeRoutes(bare)
	srv.sessions.put("sess-test", "admin")
	for _, c := range routes {
		assert.Equal(t, http.StatusServiceUnavailable, serveAuthed(bare, c.method, c.path, "{}").Code, c.method+" "+c.path)
	}
}

func TestOrganizeAPI_SettingsLibrariesMapsServers(t *testing.T) {
	e := newOrganizeServer(t)
	w := serveAuthed(e.mux, http.MethodGet, "/api/media/organize/settings", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"scan_interval_min":60`)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPut, "/api/media/organize/settings", `{"auto":true}`).Code, "拼错的字段")
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPut, "/api/media/organize/settings", `{"scan_interval_min":1}`).Code)
	w = serveAuthed(e.mux, http.MethodPut, "/api/media/organize/settings", `{"auto_enabled":true,"categories":["movies"],"notify_channels":[2]}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"auto_since"`)

	w = serveAuthed(e.mux, http.MethodPost, "/api/media/libraries/template-preview", `{"kind":"tv","template":""}`)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "最后生还者 (2023)/Season 1/最后生还者 - S01E02 - 感染")
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/libraries/template-preview", `{"kind":"movie","template":"{{.Nope}}"}`)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"error"`)

	w = serveAuthed(e.mux, http.MethodPost, "/api/media/libraries", `{"name":"电影","kind":"movie","path":"`+e.library+`","enabled":true}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var lib organize.LibraryView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &lib))
	assert.Equal(t, "hardlink", lib.Mode)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, "/api/media/libraries", `{"name":"x","kind":"movie","path":"rel"}`).Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(e.mux, http.MethodPut, "/api/media/libraries/99", `{"name":"x","kind":"movie","path":"/x"}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPut, "/api/media/libraries/abc", `{}`).Code)
	w = serveAuthed(e.mux, http.MethodGet, "/api/media/libraries", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"effective_template"`)

	w = serveAuthed(e.mux, http.MethodPost, "/api/media/path-maps", `{"downloader_id":1,"downloader_prefix":"/downloads","local_prefix":"`+e.dlDir+`"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, "/api/media/path-maps", `{"downloader_id":5,"downloader_prefix":"/x","local_prefix":"/y"}`).Code)
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/libraries/check", `{"path":"`+e.library+`"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"name":"库目录可写","ok":true`)

	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Emby-Token") != "k1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"ServerName":"Emby","Version":"4.8"}`))
	}))
	t.Cleanup(emby.Close)
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/servers", `{"name":"Emby","kind":"emby","url":"`+emby.URL+`","token":"k1","enabled":true}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "k1", "Token 只写不读")
	assert.Contains(t, w.Body.String(), `"has_token":true`)
	w = serveAuthed(e.mux, http.MethodGet, "/api/media/servers", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "k1")
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/servers/test", `{"id":1,"kind":"emby","url":"`+emby.URL+`"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"version":"4.8"`)
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/servers/test", `{"kind":"emby","url":"`+emby.URL+`","token":"bad"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, "Token 不对")
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/servers/test", `{"kind":"emby","url":"http://127.0.0.1:9","token":"k1"}`)
	assert.Equal(t, http.StatusBadGateway, w.Code, "连不上")
	assert.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodDelete, "/api/media/servers/1", "").Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(e.mux, http.MethodDelete, "/api/media/servers/1", "").Code)
	assert.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodDelete, "/api/media/path-maps/1", "").Code)
	assert.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodDelete, "/api/media/libraries/"+strconv.Itoa(int(lib.ID)), "").Code)
}

func TestOrganizeAPI_PreviewOrganizeHistory(t *testing.T) {
	e := newOrganizeServer(t)
	require.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodPost, "/api/media/libraries", `{"name":"电影","kind":"movie","path":"`+e.library+`","enabled":true}`).Code)
	require.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodPut, "/api/media/organize/settings", `{"min_video_mb":1}`).Code)

	// 没有路径映射：找不到文件
	w := serveAuthed(e.mux, http.MethodPost, "/api/media/organize/preview", `{"downloader_id":1,"hash":"hash1"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "路径映射")

	require.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodPost, "/api/media/path-maps", `{"downloader_id":1,"downloader_prefix":"/downloads","local_prefix":"`+e.dlDir+`"}`).Code)
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/organize/preview", `{"downloader_id":1,"hash":"hash1"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var plan organize.Plan
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &plan))
	require.Empty(t, plan.Problem)
	require.Len(t, plan.Items, 1)
	target := filepath.Join(e.library, "肖申克的救赎 (1994)", "肖申克的救赎 (1994) - 1080p BluRay H.264.mkv")
	assert.Equal(t, target, plan.Items[0].Target)

	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, "/api/media/organize", `{"downloader_id":1,"hash":"hash1","extra":1}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, "/api/media/organize", `{"downloader_id":1}`).Code)
	e.dl.getErr = downloader.ErrTorrentNotFound
	assert.Equal(t, http.StatusNotFound, serveAuthed(e.mux, http.MethodPost, "/api/media/organize", `{"downloader_id":1,"hash":"nope"}`).Code)
	e.dl.getErr = nil

	w = serveAuthed(e.mux, http.MethodPost, "/api/media/organize", `{"downloader_id":1,"hash":"hash1"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"created":1`)
	_, err := os.Stat(target)
	require.NoError(t, err)

	w = serveAuthed(e.mux, http.MethodGet, "/api/media/history?status=done&q=Shawshank&limit=10", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var page organize.HistoryPage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.Equal(t, int64(1), page.Total)
	assert.Equal(t, "电影", page.Items[0].LibraryName)
	assert.NotContains(t, w.Body.String(), "target_file_id", "内部字段不回")
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodGet, "/api/media/history?status=bad", "").Code)

	id := strconv.Itoa(int(page.Items[0].ID))
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/history/"+id+"/retry", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"done":1`)
	assert.Equal(t, http.StatusNotFound, serveAuthed(e.mux, http.MethodPost, "/api/media/history/99/retry", "").Code)

	w = serveAuthed(e.mux, http.MethodDelete, "/api/media/history/"+id+"?files=1", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	_, err = os.Stat(target)
	assert.ErrorIs(t, err, os.ErrNotExist, "连库里的文件一起删")
	assert.Equal(t, http.StatusNotFound, serveAuthed(e.mux, http.MethodDelete, "/api/media/history/"+id, "").Code)
}
