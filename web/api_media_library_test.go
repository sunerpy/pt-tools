package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/media/csf"
	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/internal/media/transfer"
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
	srv     *Server
	rec     *recognize.Service
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
		&models.MediaLibrary{}, &models.MediaPathMap{}, &models.MediaServer{}, &models.MediaOrganizeSetting{}, &models.MediaTransferHistory{}, &models.MediaReplaceJournal{}, &models.MediaWatchDir{},
		&models.DownloaderDirectory{}, &models.MediaServerItem{}, &models.MediaIssue{},
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
	e := &organizeTestEnv{srv: srv, rec: rec, dlDir: filepath.Join(root, "downloads"), library: filepath.Join(root, "movies")}
	require.NoError(t, os.MkdirAll(e.library, 0o755))
	name := "The.Shawshank.Redemption.1994.1080p.BluRay.x264-WiKi"
	require.NoError(t, os.MkdirAll(filepath.Join(e.dlDir, name), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(e.dlDir, name, name+".mkv"), []byte("video"), 0o644))
	e.dl = &fakeDownloader{
		torrents: []downloader.Torrent{{ID: "hash1", InfoHash: "hash1", Name: name, Progress: 1, SavePath: "/downloads", State: downloader.TorrentSeeding}},
		files:    []downloader.TorrentFile{{Name: name + "/" + name + ".mkv", Size: 100 << 20, Progress: 1, Priority: 1}},
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
		{http.MethodGet, "/api/media/organize/classify"},
		{http.MethodPut, "/api/media/organize/classify"},
		{http.MethodPost, "/api/media/organize/preview"},
		{http.MethodPost, "/api/media/organize"},
		{http.MethodGet, "/api/media/libraries"},
		{http.MethodPost, "/api/media/libraries"},
		{http.MethodPut, "/api/media/libraries/1"},
		{http.MethodDelete, "/api/media/libraries/1"},
		{http.MethodPost, "/api/media/libraries/check"},
		{http.MethodPost, "/api/media/libraries/csf-test"},
		{http.MethodPost, "/api/media/libraries/template-preview"},
		{http.MethodGet, "/api/media/path-maps"},
		{http.MethodPost, "/api/media/path-maps"},
		{http.MethodPut, "/api/media/path-maps/1"},
		{http.MethodDelete, "/api/media/path-maps/1"},
		{http.MethodGet, "/api/media/watch-dirs"},
		{http.MethodPost, "/api/media/watch-dirs"},
		{http.MethodPut, "/api/media/watch-dirs/1"},
		{http.MethodDelete, "/api/media/watch-dirs/1"},
		{http.MethodGet, "/api/media/servers"},
		{http.MethodPost, "/api/media/servers"},
		{http.MethodPut, "/api/media/servers/1"},
		{http.MethodDelete, "/api/media/servers/1"},
		{http.MethodPost, "/api/media/servers/test"},
		{http.MethodGet, "/api/media/history"},
		{http.MethodPost, "/api/media/history/batch"},
		{http.MethodGet, "/api/media/fs/roots"},
		{http.MethodGet, "/api/media/organize/ai-status"},
		{http.MethodPost, "/api/media/organize/ai-recognize"},
		{http.MethodPost, "/api/media/history/1/ai-recognize"},
		{http.MethodGet, "/api/media/fs/list?path=/"},
		{http.MethodPost, "/api/media/history/1/retry"},
		{http.MethodPost, "/api/media/history/1/scrape"},
		{http.MethodDelete, "/api/media/history/1"},
		{http.MethodPost, "/api/media/libraries/1/import/scan"},
		{http.MethodGet, "/api/media/import/scans/0123456789abcdef"},
		{http.MethodDelete, "/api/media/import/scans/0123456789abcdef"},
		{http.MethodPut, "/api/media/import/scans/0123456789abcdef/item"},
		{http.MethodPost, "/api/media/import/scans/0123456789abcdef/import"},
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

func TestOrganizeAPI_WatchDirs(t *testing.T) {
	e := newOrganizeServer(t)
	dir := filepath.Join(t.TempDir(), "incoming")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	w := serveAuthed(e.mux, http.MethodPost, "/api/media/watch-dirs", `{"path":`+jsonPath(dir)+`,"enabled":true}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var d models.MediaWatchDir
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &d))
	assert.True(t, d.Enabled)
	w = serveAuthed(e.mux, http.MethodGet, "/api/media/watch-dirs", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"enabled":true`)
	id := strconv.FormatUint(uint64(d.ID), 10)
	w = serveAuthed(e.mux, http.MethodPut, "/api/media/watch-dirs/"+id, `{"path":`+jsonPath(dir)+`,"enabled":false}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"enabled":false`)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, "/api/media/watch-dirs", `{"path":"rel"}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, "/api/media/watch-dirs", `{"dir":"/x"}`).Code, "拼错的字段")
	assert.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodDelete, "/api/media/watch-dirs/"+id, "").Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(e.mux, http.MethodDelete, "/api/media/watch-dirs/"+id, "").Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPut, "/api/media/watch-dirs/x", `{"path":`+jsonPath(dir)+`}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodDelete, "/api/media/watch-dirs/x", "").Code)
}

func TestOrganizeAPI_Classify(t *testing.T) {
	e := newOrganizeServer(t)
	w := serveAuthed(e.mux, http.MethodGet, "/api/media/organize/classify", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"rules":{"movie":[],"tv":[]}`, "没设过时两类都是空的")
	assert.Contains(t, w.Body.String(), `"name":"华语电影"`, "带上常用的预设")

	w = serveAuthed(e.mux, http.MethodPut, "/api/media/organize/classify", `{"movie":[{"name":"华语电影","languages":["ZH"]}],"tv":[{"name":"美剧","countries":["us"]}]}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"rules":{"movie":[{"name":"华语电影","languages":["zh"]}],"tv":[{"name":"美剧","countries":["US"]}]}`)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPut, "/api/media/organize/classify", `{"movie":[{"name":"a/b"}]}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPut, "/api/media/organize/classify", `{"films":[]}`).Code, "拼错的字段")

	w = serveAuthed(e.mux, http.MethodPost, "/api/media/libraries", `{"name":"电影","kind":"movie","path":`+jsonPath(e.library)+`,"classify_dir":true,"enabled":true}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"classify_dir":true`)
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

	w = serveAuthed(e.mux, http.MethodPost, "/api/media/libraries", `{"name":"电影","kind":"movie","path":`+jsonPath(e.library)+`,"enabled":true}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var lib organize.LibraryView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &lib))
	assert.Equal(t, "hardlink", lib.Mode)
	assert.Contains(t, w.Body.String(), `"scrape_items":["nfo","poster","fanart","season_poster","thumb"]`, "没选过时是默认的那几项")
	assert.Contains(t, w.Body.String(), `"scrape_image_lang":""`)
	w = serveAuthed(e.mux, http.MethodPut, "/api/media/libraries/"+strconv.FormatUint(uint64(lib.ID), 10),
		`{"name":"电影","kind":"movie","path":`+jsonPath(e.library)+`,"scrape":true,"scrape_items":["logo","poster"],"scrape_image_lang":"none","enabled":true}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"scrape_items":["poster","logo"]`)
	assert.Contains(t, w.Body.String(), `"scrape_image_lang":"none"`)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPut, "/api/media/libraries/"+strconv.FormatUint(uint64(lib.ID), 10),
		`{"name":"电影","kind":"movie","path":`+jsonPath(e.library)+`,"scrape":true,"scrape_items":[]}`).Code, "开着刮削时一项都没选")
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, "/api/media/libraries", `{"name":"x","kind":"movie","path":"rel"}`).Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(e.mux, http.MethodPut, "/api/media/libraries/99", `{"name":"x","kind":"movie","path":"/x"}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPut, "/api/media/libraries/abc", `{}`).Code)
	w = serveAuthed(e.mux, http.MethodGet, "/api/media/libraries", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"effective_template"`)

	w = serveAuthed(e.mux, http.MethodPost, "/api/media/path-maps", `{"downloader_id":1,"downloader_prefix":"/downloads","local_prefix":`+jsonPath(e.dlDir)+`}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, "/api/media/path-maps", `{"downloader_id":5,"downloader_prefix":"/x","local_prefix":"/y"}`).Code)
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/libraries/check", `{"path":`+jsonPath(e.library)+`}`)
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

// 媒体库的字幕设置：ChineseSubFinder 的 Key 只写不读；测试按钮按状态码分出 Key 不对与连不上。
func TestOrganizeAPI_LibraryCSF(t *testing.T) {
	e := newOrganizeServer(t)
	csfSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer csf-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"AccessToken Error"}`))
			return
		}
		_, _ = w.Write([]byte(`{"message":"job_id is empty"}`))
	}))
	t.Cleanup(csfSrv.Close)
	w := serveAuthed(e.mux, http.MethodPost, "/api/media/libraries", `{"name":"电影","kind":"movie","path":`+jsonPath(e.library)+
		`,"site_subtitles":true,"csf_url":"`+csfSrv.URL+`","csf_key":"csf-secret","csf_path":"/csf/movies","enabled":true}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "csf-secret", "Key 只写不读")
	assert.Contains(t, w.Body.String(), `"has_csf_key":true`)
	assert.Contains(t, w.Body.String(), `"site_subtitles":true`)
	assert.Contains(t, w.Body.String(), `"csf_path":"/csf/movies"`)
	var lib organize.LibraryView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &lib))
	w = serveAuthed(e.mux, http.MethodGet, "/api/media/libraries", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "csf-secret")

	id := strconv.FormatUint(uint64(lib.ID), 10)
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/libraries/csf-test", `{"id":`+id+`,"url":"`+csfSrv.URL+`"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"ok":true}`, w.Body.String())
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/libraries/csf-test", `{"url":"`+csfSrv.URL+`","key":"bad"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, "Key 不对")
	assert.Contains(t, w.Body.String(), csf.ErrUnauthorized.Error())
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/libraries/csf-test", `{"url":"http://127.0.0.1:9","key":"k"}`)
	assert.Equal(t, http.StatusBadGateway, w.Code, "连不上")
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/libraries/csf-test", `{"url":"csf.lan","key":"k"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, "地址不对")
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/libraries/csf-test", `{"id":99,"url":"`+csfSrv.URL+`"}`)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/libraries", `{"name":"剧集","kind":"tv","path":`+jsonPath(e.library+"-tv")+`,"csf_url":"`+csfSrv.URL+`"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, "填了地址没有 Key")
}

func TestOrganizeAPI_PreviewOrganizeHistory(t *testing.T) {
	e := newOrganizeServer(t)
	require.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodPost, "/api/media/libraries", `{"name":"电影","kind":"movie","path":`+jsonPath(e.library)+`,"enabled":true}`).Code)
	require.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodPut, "/api/media/organize/settings", `{"min_video_mb":1}`).Code)

	// 没有路径映射：找不到文件
	w := serveAuthed(e.mux, http.MethodPost, "/api/media/organize/preview", `{"downloader_id":1,"hash":"hash1"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "路径映射")

	require.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodPost, "/api/media/path-maps", `{"downloader_id":1,"downloader_prefix":"/downloads","local_prefix":`+jsonPath(e.dlDir)+`}`).Code)
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

	// 重新刮削：媒体库关着刮削时回 400；打开以后写 NFO
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/history/"+id+"/scrape", "")
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "关着刮削")
	lib := strconv.Itoa(int(page.Items[0].LibraryID))
	require.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodPut, "/api/media/libraries/"+lib,
		`{"name":"电影","kind":"movie","path":`+jsonPath(e.library)+`,"enabled":true,"scrape":true,"scrape_items":["nfo"]}`).Code)
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/history/"+id+"/scrape", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"records":1`)
	assert.Contains(t, w.Body.String(), `"written":1`)
	_, err = os.Stat(strings.TrimSuffix(target, ".mkv") + ".nfo")
	require.NoError(t, err)
	w = serveAuthed(e.mux, http.MethodGet, "/api/media/history?status=done", "")
	assert.Contains(t, w.Body.String(), `"scrape_status":"ok"`)
	assert.Equal(t, http.StatusNotFound, serveAuthed(e.mux, http.MethodPost, "/api/media/history/99/scrape", "").Code)

	w = serveAuthed(e.mux, http.MethodDelete, "/api/media/history/"+id+"?files=1", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "kept", "都删掉了，没有留着的文件")
	_, err = os.Stat(target)
	assert.ErrorIs(t, err, os.ErrNotExist, "连库里的文件一起删")
	assert.Equal(t, http.StatusNotFound, serveAuthed(e.mux, http.MethodDelete, "/api/media/history/"+id, "").Code)
}

// 浏览文件：根目录、一层目录；根目录以外的回 403，不存在的 404，相对路径 400
func TestOrganizeAPI_FS(t *testing.T) {
	e := newOrganizeServer(t)
	require.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodPost, "/api/media/libraries", `{"name":"电影","kind":"movie","path":`+jsonPath(e.library)+`,"enabled":true}`).Code)
	require.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodPost, "/api/media/path-maps", `{"downloader_id":1,"downloader_prefix":"/downloads","local_prefix":`+jsonPath(e.dlDir)+`}`).Code)
	w := serveAuthed(e.mux, http.MethodGet, "/api/media/fs/roots", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var roots struct {
		Roots []organize.FSRoot `json:"roots"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &roots))
	require.Len(t, roots.Roots, 2)

	w = serveAuthed(e.mux, http.MethodGet, "/api/media/fs/list?path="+url.QueryEscape(e.dlDir), "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var list organize.FSList
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Entries, 1)
	assert.True(t, list.Entries[0].Dir)
	assert.Equal(t, http.StatusForbidden, serveAuthed(e.mux, http.MethodGet, "/api/media/fs/list?path="+url.QueryEscape(filepath.Dir(e.dlDir)), "").Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(e.mux, http.MethodGet, "/api/media/fs/list?path="+url.QueryEscape(filepath.Join(e.dlDir, "nope")), "").Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodGet, "/api/media/fs/list?path=rel", "").Code)
}

// 整理历史的分类筛选、按条目分组与批量操作
func TestOrganizeAPI_HistoryGroupsAndBatch(t *testing.T) {
	e := newOrganizeServer(t)
	require.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodPost, "/api/media/libraries", `{"name":"电影","kind":"movie","path":`+jsonPath(e.library)+`,"enabled":true}`).Code)
	require.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodPost, "/api/media/path-maps", `{"downloader_id":1,"downloader_prefix":"/downloads","local_prefix":`+jsonPath(e.dlDir)+`}`).Code)
	w := serveAuthed(e.mux, http.MethodPost, "/api/media/organize", `{"downloader_id":1,"hash":"hash1"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"created":1`)
	failed := models.MediaTransferHistory{TorrentName: "Unknown.2020", SourcePath: "/downloads/Unknown.2020", Status: models.MediaTransferFailed, FailKind: models.MediaFailRecognize}
	require.NoError(t, global.GlobalDB.DB.Create(&failed).Error)

	w = serveAuthed(e.mux, http.MethodGet, "/api/media/history?kind=recognize", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var page organize.HistoryPage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.Len(t, page.Items, 1)
	assert.Equal(t, failed.ID, page.Items[0].ID)
	assert.Contains(t, w.Body.String(), `"fail_kind":"recognize"`)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodGet, "/api/media/history?kind=bad", "").Code)

	w = serveAuthed(e.mux, http.MethodGet, "/api/media/history?group=item", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var groups organize.HistoryGroupPage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &groups))
	require.Equal(t, int64(2), groups.Total)
	var movie organize.HistoryGroup
	for _, g := range groups.Groups {
		if g.TMDBID > 0 {
			movie = g
		}
	}
	require.Equal(t, "movie", movie.MediaType)
	assert.Equal(t, 1, movie.Done)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodGet, "/api/media/history?group=site", "").Code)

	w = serveAuthed(e.mux, http.MethodGet, "/api/media/history?media_type=movie&tmdb_id="+strconv.Itoa(movie.TMDBID)+"&season=0", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.Len(t, page.Items, 1)
	done := page.Items[0].ID
	w = serveAuthed(e.mux, http.MethodGet, "/api/media/history?torrent=Unknown.2020", "")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.Len(t, page.Items, 1)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodGet, "/api/media/history?tmdb_id=5", "").Code, "按条目看要写类型")

	body := `{"ids":[` + strconv.Itoa(int(done)) + `,` + strconv.Itoa(int(failed.ID)) + `],"action":"delete"}`
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/history/batch", body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var res organize.BatchResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, 2, res.OK)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, "/api/media/history/batch", `{"ids":[1],"action":"burn"}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, "/api/media/history/batch", `{"ids":[],"action":"delete"}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, "/api/media/history/batch", `{"ids":[1],"action":"delete","x":1}`).Code)
}

// 手动整理时选整理方式：网页会话的预览与整理按选的方式（这里媒体库是硬链接、选了复制），整理历史写明选的方式；
// 认不得的整理方式回 400
func TestOrganizeAPI_ChosenMode(t *testing.T) {
	e := newOrganizeServer(t)
	require.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodPost, "/api/media/libraries", `{"name":"电影","kind":"movie","path":`+jsonPath(e.library)+`,"enabled":true}`).Code)
	require.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodPut, "/api/media/organize/settings", `{"min_video_mb":1}`).Code)
	in := filepath.Join(t.TempDir(), "incoming")
	require.NoError(t, os.MkdirAll(in, 0o755))
	src := filepath.Join(in, "The.Shawshank.Redemption.1994.1080p.BluRay.x264-WiKi.mkv")
	require.NoError(t, os.WriteFile(src, make([]byte, 2<<20), 0o644))
	old := time.Now().Add(-10 * time.Minute)
	require.NoError(t, os.Chtimes(src, old, old))
	body := `{"source_path":` + jsonPath(src) + `,"mode":"copy"}`

	w := serveAuthed(e.mux, http.MethodPost, "/api/media/organize/preview", body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var plan struct {
		Mode    string `json:"mode"`
		Problem string `json:"problem"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &plan))
	require.Empty(t, plan.Problem)
	assert.Equal(t, "copy", plan.Mode)

	for _, route := range []string{"/api/media/organize/preview", "/api/media/organize"} {
		w = serveAuthed(e.mux, http.MethodPost, route, `{"source_path":`+jsonPath(src)+`,"mode":"rename"}`)
		assert.Equal(t, http.StatusBadRequest, w.Code, route+" "+w.Body.String())
		assert.Contains(t, w.Body.String(), "整理方式", route)
	}

	w = serveAuthed(e.mux, http.MethodPost, "/api/media/organize", body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"created":1`)

	w = serveAuthed(e.mux, http.MethodGet, "/api/media/history?q=Shawshank", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var page struct {
		Items []struct {
			Mode       string `json:"mode"`
			ChosenMode string `json:"chosen_mode"`
			TargetPath string `json:"target_path"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.Len(t, page.Items, 1)
	assert.Equal(t, "copy", page.Items[0].Mode)
	assert.Equal(t, "copy", page.Items[0].ChosenMode)
	si, err := os.Stat(src)
	require.NoError(t, err)
	ti, err := os.Stat(page.Items[0].TargetPath)
	require.NoError(t, err)
	assert.False(t, os.SameFile(si, ti), "复制出的是另一个文件，不是媒体库的硬链接")
}

// 手动指定一个文件整理：网页会话的预览与整理两个路由收 source_path；整理历史带上当时填的路径，重试照常；
// 路径不对、来源不止一种时回 400
func TestOrganizeAPI_SourcePath(t *testing.T) {
	e := newOrganizeServer(t)
	require.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodPost, "/api/media/libraries", `{"name":"电影","kind":"movie","path":`+jsonPath(e.library)+`,"enabled":true}`).Code)
	require.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodPut, "/api/media/organize/settings", `{"min_video_mb":1}`).Code)
	in := filepath.Join(t.TempDir(), "incoming")
	require.NoError(t, os.MkdirAll(in, 0o755))
	src := filepath.Join(in, "The.Shawshank.Redemption.1994.1080p.BluRay.x264-WiKi.mkv")
	require.NoError(t, os.WriteFile(src, make([]byte, 2<<20), 0o644))
	// 最近 2 分钟里还在改动的不整理（可能还在复制）
	old := time.Now().Add(-10 * time.Minute)
	require.NoError(t, os.Chtimes(src, old, old))
	body := `{"source_path":` + jsonPath(src) + `}`

	w := serveAuthed(e.mux, http.MethodPost, "/api/media/organize/preview", body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var plan struct {
		SourcePath     string `json:"source_path"`
		DownloaderID   uint   `json:"downloader_id"`
		DownloaderName string `json:"downloader_name"`
		Problem        string `json:"problem"`
		Items          []struct {
			Target string `json:"target"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &plan))
	require.Empty(t, plan.Problem)
	assert.Equal(t, src, plan.SourcePath)
	assert.Zero(t, plan.DownloaderID)
	assert.Equal(t, "指定路径", plan.DownloaderName)
	require.Len(t, plan.Items, 1)
	target := filepath.Join(e.library, "肖申克的救赎 (1994)", "肖申克的救赎 (1994) - 1080p BluRay H.264.mkv")
	assert.Equal(t, target, plan.Items[0].Target)
	_, err := os.Stat(target)
	assert.ErrorIs(t, err, os.ErrNotExist, "预览不动文件")

	missing := `{"source_path":` + jsonPath(filepath.Join(in, "nope.mkv")) + `}`
	for _, bad := range []string{
		`{"source_path":"rel"}`,
		missing,
		`{"downloader_id":1,"hash":"hash1","source_path":` + jsonPath(src) + `}`,
		`{"source_path":` + jsonPath(e.library) + `}`,
	} {
		for _, route := range []string{"/api/media/organize/preview", "/api/media/organize"} {
			w = serveAuthed(e.mux, http.MethodPost, route, bad)
			assert.Equal(t, http.StatusBadRequest, w.Code, route+" "+bad+" "+w.Body.String())
		}
	}
	assert.Contains(t, serveAuthed(e.mux, http.MethodPost, "/api/media/organize/preview", missing).Body.String(), "找不到")

	w = serveAuthed(e.mux, http.MethodPost, "/api/media/organize", body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"created":1`)
	_, err = os.Stat(target)
	require.NoError(t, err)

	w = serveAuthed(e.mux, http.MethodGet, "/api/media/history?q=Shawshank", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var page struct {
		Items []struct {
			ID             uint   `json:"id"`
			ManualPath     string `json:"manual_path"`
			DownloaderName string `json:"downloader_name"`
			Trigger        string `json:"trigger"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.Len(t, page.Items, 1)
	assert.Equal(t, src, page.Items[0].ManualPath, "带上当时填的路径")
	assert.Equal(t, "指定路径", page.Items[0].DownloaderName)
	assert.Equal(t, "manual", page.Items[0].Trigger)

	w = serveAuthed(e.mux, http.MethodPost, "/api/media/history/"+strconv.Itoa(int(page.Items[0].ID))+"/retry", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"done":1`, "已经在库里")
}

// 导入已有文件：扫描（后台进行）、看进度与结果、改条目、导入；导入的记录删除时要另外确认才连文件删
func TestOrganizeAPI_Import(t *testing.T) {
	e := newOrganizeServer(t)
	w := serveAuthed(e.mux, http.MethodPost, "/api/media/libraries", `{"name":"电影","kind":"movie","path":`+jsonPath(e.library)+`,"enabled":true}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var lib organize.LibraryView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &lib))
	dir := filepath.Join(e.library, "The Shawshank Redemption (1994)")
	video := filepath.Join(dir, "The.Shawshank.Redemption.1994.mkv")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	f, err := os.Create(video)
	require.NoError(t, err)
	require.NoError(t, f.Truncate(60<<20))
	require.NoError(t, f.Close())
	old := time.Now().Add(-time.Hour)
	for _, p := range []string{video, dir} {
		require.NoError(t, os.Chtimes(p, old, old))
	}
	scanURL := "/api/media/libraries/" + strconv.Itoa(int(lib.ID)) + "/import/scan"

	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, scanURL, `{"path":"/elsewhere"}`).Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(e.mux, http.MethodPost, "/api/media/libraries/99/import/scan", `{}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, scanURL, `{"x":1}`).Code)
	w = serveAuthed(e.mux, http.MethodPost, scanURL, `{"path":""}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var sc organize.ImportScan
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sc))
	scanPath := "/api/media/import/scans/" + sc.ID
	require.Eventually(t, func() bool {
		got := serveAuthed(e.mux, http.MethodGet, scanPath+"?brief=1", "")
		return got.Code == http.StatusOK && strings.Contains(got.Body.String(), `"status":"done"`)
	}, 10*time.Second, 10*time.Millisecond)
	w = serveAuthed(e.mux, http.MethodGet, scanPath+"?brief=1", "")
	assert.NotContains(t, w.Body.String(), `"items"`)
	w = serveAuthed(e.mux, http.MethodGet, scanPath, "")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sc))
	require.Len(t, sc.Items, 1)
	require.NotNil(t, sc.Items[0].Match, sc.Items[0].Problem)
	assert.Equal(t, 278, sc.Items[0].Match.TMDBID)

	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPut, scanPath+"/item", `{"path":`+jsonPath(dir)+`,"tmdb_id":0}`).Code)
	w = serveAuthed(e.mux, http.MethodPut, scanPath+"/item", `{"path":`+jsonPath(dir)+`,"tmdb_id":278}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"source":"manual"`)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, scanPath+"/import", `{"paths":[]}`).Code)
	w = serveAuthed(e.mux, http.MethodPost, scanPath+"/import", `{"paths":[`+jsonPath(dir)+`],"scrape":false}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"created":1`)

	w = serveAuthed(e.mux, http.MethodGet, "/api/media/history", "")
	var page organize.HistoryPage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.Len(t, page.Items, 1)
	row := page.Items[0]
	assert.Equal(t, models.MediaModeExisting, row.Mode)
	assert.Equal(t, models.MediaTriggerImport, row.Trigger)
	id := strconv.Itoa(int(row.ID))
	w = serveAuthed(e.mux, http.MethodDelete, "/api/media/history/"+id+"?files=1", "")
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "导入时就在库里的文件")
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/history/batch", `{"ids":[`+id+`],"action":"delete_files"}`)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"ok":false`)
	assert.FileExists(t, video)
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/history/batch", `{"ids":[`+id+`],"action":"delete_files","allow_existing":true}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"ok":1`)
	assert.NoFileExists(t, video, "确认以后连文件删除")

	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPut, scanPath+"/item", `{"x":1}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, scanPath+"/import", `{"x":1}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, "/api/media/libraries/abc/import/scan", `{}`).Code)
	require.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodDelete, scanPath, "").Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(e.mux, http.MethodGet, scanPath, "").Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(e.mux, http.MethodDelete, scanPath, "").Code)
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/media/import/scans/xyz"},
		{http.MethodDelete, "/api/media/import/scans/xyz"},
		{http.MethodPut, "/api/media/import/scans/xyz/item"},
		{http.MethodPost, "/api/media/import/scans/xyz/import"},
		{http.MethodPut, scanPath + "/item"},
		{http.MethodPost, scanPath + "/import"},
	} {
		body := `{"path":"/x","tmdb_id":278,"paths":["/x"]}`
		if strings.HasSuffix(c.path, "/item") {
			body = `{"path":"/x","tmdb_id":278}`
		} else if strings.HasSuffix(c.path, "/import") {
			body = `{"paths":["/x"]}`
		}
		assert.Equal(t, http.StatusNotFound, serveAuthed(e.mux, c.method, c.path, body).Code, c.method+" "+c.path)
	}
}

// 删除单条导入的记录：带 allow_existing=1 才连文件删
func TestOrganizeAPI_DeleteImportedWithFiles(t *testing.T) {
	e := newOrganizeServer(t)
	video := filepath.Join(e.library, "a.mkv")
	require.NoError(t, os.WriteFile(video, []byte("video"), 0o644))
	fileID, err := transfer.FileID(video)
	require.NoError(t, err)
	row := models.MediaTransferHistory{
		SourcePath: video, TargetPath: video, Status: models.MediaTransferDone, Mode: models.MediaModeExisting, Trigger: models.MediaTriggerImport,
		TargetFileID: fileID, InfoHash: "import:0123456789abcdef",
	}
	require.NoError(t, global.GlobalDB.DB.Create(&row).Error)
	id := strconv.Itoa(int(row.ID))
	w := serveAuthed(e.mux, http.MethodDelete, "/api/media/history/"+id+"?files=1&allow_existing=1", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NoFileExists(t, video)
}
