package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/health"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// newHomeServer 是带两台假下载器（qb1 连得上，qb2 连不上）的服务，只挂首页与体检的接口。
func newHomeServer(t *testing.T) (*Server, *http.ServeMux, string) {
	t.Helper()
	srv := setupServer(t)
	srv.mgr.StopAll()
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(
		&models.SiteLoginState{}, &models.SiteAttendanceLog{}, &models.MediaTransferHistory{}, &models.MediaSubscription{},
		&models.MediaSubscriptionEpisode{}, &models.MediaIssue{}, &models.MediaServer{}, &models.MediaPathMap{}, &models.MediaLibrary{},
		&models.MediaServerItem{},
	))
	local := t.TempDir()
	fakes := map[string]*fakeDownloader{
		"qb1": {
			status: downloader.ClientStatus{DlSpeed: 300, UpSpeed: 100}, freSpace: 500 << 30, version: "v5.0.4", paths: []string{"/downloads"},
			torrents: []downloader.Torrent{
				{ID: "a", State: downloader.TorrentDownloading},
				{ID: "b", State: downloader.TorrentDownloading},
				{ID: "c", State: downloader.TorrentSeeding},
				{ID: "d", State: downloader.TorrentSeeding},
				{ID: "e", State: downloader.TorrentSeeding},
				{ID: "f", State: downloader.TorrentPaused},
			},
		},
		"qb2": {
			statusErr: assertErr("dial tcp: connection refused"), freeSpaceErr: assertErr("dial tcp: connection refused"),
			versionErr: assertErr("dial tcp: connection refused"), pathsErr: assertErr("dial tcp: connection refused"),
		},
	}
	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)
	dm := mgr.GetDownloaderManager()
	dm.RegisterFactory(downloader.DownloaderQBittorrent, func(_ downloader.DownloaderConfig, name string) (downloader.Downloader, error) {
		f := fakes[name]
		f.name, f.dlType = name, downloader.DownloaderQBittorrent
		return f, nil
	})
	for _, name := range []string{"qb1", "qb2"} {
		require.NoError(t, dm.RegisterConfig(name, downloader.NewGenericConfig(downloader.DownloaderQBittorrent, "http://"+name, "u", "p", true), name == "qb1"))
		row := models.DownloaderSetting{Name: name, Type: "qbittorrent", URL: "http://" + name, Enabled: true}
		require.NoError(t, db.Create(&row).Error)
		if name == "qb1" {
			require.NoError(t, db.Create(&models.MediaPathMap{DownloaderID: row.ID, DownloaderPrefix: "/downloads", LocalPrefix: local}).Error)
		}
	}
	srv.mgr = mgr
	require.NoError(t, db.Create(&models.MediaLibrary{Name: "电影", Kind: models.MediaKindMovie, Path: filepath.Join(local, "movies"), Mode: models.MediaModeCopy}).Error)
	mux := http.NewServeMux()
	srv.registerHomeRoutes(mux)
	srv.sessions.put("sess-test", "admin")
	return srv, mux, local
}

type homeBody struct {
	At          time.Time       `json:"at"`
	Calendar    json.RawMessage `json:"calendar"`
	Todo        json.RawMessage `json:"todo"`
	Resume      json.RawMessage `json:"resume"`
	Recent      json.RawMessage `json:"recent"`
	Downloaders json.RawMessage `json:"downloaders"`
	Storage     json.RawMessage `json:"storage"`
	Status      json.RawMessage `json:"status"`
}

func getHome(t *testing.T, mux *http.ServeMux, path string) homeBody {
	t.Helper()
	w := serveAuthed(mux, http.MethodGet, path, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var b homeBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &b))
	return b
}

func block[T any](t *testing.T, raw json.RawMessage) (T, string) {
	t.Helper()
	var b struct {
		Data  T      `json:"data"`
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(raw, &b), string(raw))
	return b.Data, b.Error
}

func TestHomeAPI_RequiresSession(t *testing.T) {
	_, mux, _ := newHomeServer(t)
	for _, r := range []struct{ method, path string }{
		{http.MethodGet, "/api/home"}, {http.MethodGet, "/api/system/check"}, {http.MethodPost, "/api/system/check"},
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(r.method, r.path, nil))
		assert.Equal(t, http.StatusUnauthorized, w.Code, r.path)
	}
}

func TestHomeAPI(t *testing.T) {
	srv, mux, local := newHomeServer(t)
	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.MediaTransferHistory{
		Status: models.MediaTransferFailed, FailKind: models.MediaFailPath, SourcePath: "/dl/a.mkv", TorrentName: "A.2024.1080p", Message: "找不到文件",
	}).Error)

	b := getHome(t, mux, "/api/home")
	_, calErr := block[any](t, b.Calendar)
	assert.Equal(t, "订阅服务没有启动", calErr, "没有订阅服务时只有这一块出错")
	_, recErr := block[any](t, b.Recent)
	assert.Equal(t, "整理入库服务没有启动", recErr)
	_, resErr := block[any](t, b.Resume)
	assert.Equal(t, "整理入库服务没有启动", resErr)

	todo, todoErr := block[struct {
		Groups []struct {
			Kind  string `json:"kind"`
			Label string `json:"label"`
		} `json:"groups"`
	}](t, b.Todo)
	require.Empty(t, todoErr)
	require.Len(t, todo.Groups, 1)
	assert.Equal(t, "整理失败 1", todo.Groups[0].Label)

	dls, dlErr := block[homeDownloaders](t, b.Downloaders)
	require.Empty(t, dlErr)
	require.Len(t, dls.Downloaders, 2)
	assert.Equal(t, homeDownloader{ID: dls.Downloaders[0].ID, Name: "qb1", Type: "qbittorrent", Online: true, Down: 300, Up: 100, Downloading: 2, Seeding: 3}, dls.Downloaders[0])
	assert.False(t, dls.Downloaders[1].Online)
	assert.Contains(t, dls.Downloaders[1].Error, "connection refused")
	assert.Equal(t, [4]int64{300, 100, 2, 3}, [4]int64{dls.Down, dls.Up, int64(dls.Downloading), int64(dls.Seeding)})
	require.Len(t, dls.SpeedHistory, 1, "首页取速度时也记一条采样")
	assert.Equal(t, 1, dls.SpeedHistory[0].Answered)
	assert.Equal(t, 2, dls.SpeedHistory[0].Total)

	disks, diskErr := block[[]struct {
		Kind  string `json:"kind"`
		Label string `json:"label"`
		Path  string `json:"path"`
		Total uint64 `json:"total"`
		Error string `json:"error"`
	}](t, b.Storage)
	require.Empty(t, diskErr)
	require.Len(t, disks, 3)
	assert.Equal(t, "下载盘 · qb1", disks[0].Label, "按路径映射换成本机的目录读到了总量")
	assert.Equal(t, "/downloads", disks[0].Path)
	assert.Positive(t, disks[0].Total)
	assert.Equal(t, "下载盘 · qb2", disks[1].Label)
	assert.Contains(t, disks[1].Error, "connection refused")
	assert.Equal(t, "媒体库 · 电影", disks[2].Label)
	assert.NotEmpty(t, disks[2].Error, "库目录不在时写原因")
	_ = local

	st, stErr := block[struct {
		Health *health.Report `json:"health"`
		Backup struct {
			Enabled bool `json:"enabled"`
			Late    bool `json:"late"`
			Days    int  `json:"days"`
		} `json:"backup"`
		Servers []any `json:"servers"`
	}](t, b.Status)
	require.Empty(t, stErr)
	assert.Nil(t, st.Health, "没体检过")
	assert.True(t, st.Backup.Enabled, "没有备份设置时按默认（开着）")
	assert.False(t, st.Backup.Late)
	assert.Equal(t, -1, st.Backup.Days)
	assert.Empty(t, st.Servers)

	// 30 秒内再取是缓存的；?refresh=1 重新算
	again := getHome(t, mux, "/api/home")
	assert.True(t, again.At.Equal(b.At))
	fresh := getHome(t, mux, "/api/home?refresh=1")
	assert.True(t, fresh.At.After(b.At))

	// 体检以后状态行与待处理跟着变：缓存里那份作废，不带 refresh 也重新算
	w := serveAuthed(mux, http.MethodPost, "/api/system/check", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	fresh = getHome(t, mux, "/api/home")
	st2, _ := block[struct {
		Health *struct {
			Fail int `json:"fail"`
		} `json:"health"`
	}](t, fresh.Status)
	require.NotNil(t, st2.Health)
	assert.Positive(t, st2.Health.Fail, "没有媒体识别服务、qb2 连不上")
	todo2, _ := block[struct {
		Groups []struct {
			Kind string `json:"kind"`
		} `json:"groups"`
	}](t, fresh.Todo)
	require.Len(t, todo2.Groups, 2)
	assert.Equal(t, "health", todo2.Groups[1].Kind)
	_ = srv
}

func TestSystemCheckAPI(t *testing.T) {
	_, mux, _ := newHomeServer(t)
	w := serveAuthed(mux, http.MethodGet, "/api/system/check", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"report":null}`, w.Body.String(), "没体检过")

	w = serveAuthed(mux, http.MethodPost, "/api/system/check", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var run systemCheckResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &run))
	require.NotNil(t, run.Report)
	items := map[string]health.Item{}
	for _, it := range run.Report.Items {
		items[it.Key] = it
	}
	assert.Equal(t, health.StatusFail, items["tmdb"].Status)
	assert.Equal(t, "媒体识别服务没有启动", items["tmdb"].Detail)
	assert.Equal(t, health.StatusOK, items["downloader:1"].Status, "%+v", items["downloader:1"])
	assert.Equal(t, "v5.0.4 · 剩余 500.00 GiB", items["downloader:1"].Detail)
	assert.Equal(t, health.StatusFail, items["downloader:2"].Status)
	assert.Equal(t, health.StatusOK, items["path:1"].Status)
	assert.Equal(t, health.StatusFail, items["library:1"].Status, "没有整理入库服务")
	assert.Equal(t, health.StatusSkip, items["servers"].Status)
	assert.Equal(t, health.StatusSkip, items["backup"].Status, "没做过备份")
	assert.Equal(t, health.Item{Key: "nas", Group: health.GroupNAS, Title: "NAS", Status: health.StatusSkip, Detail: "NAS 监控没有启动"}, items["nas"])
	assert.Contains(t, items, "secret")
	var disks int
	for _, it := range run.Report.Items {
		if it.Group == health.GroupDisks {
			disks++
		}
	}
	assert.Positive(t, disks)

	w = serveAuthed(mux, http.MethodGet, "/api/system/check", "")
	require.Equal(t, http.StatusOK, w.Code)
	var last systemCheckResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &last))
	require.NotNil(t, last.Report)
	assert.Equal(t, run.Report.Items, last.Report.Items)
}

// transfer-stats 带上最近的总速度采样（首页的速度柱）。
func TestTransferStatsSpeedHistory(t *testing.T) {
	srv, _, _ := newHomeServer(t)
	w := httptest.NewRecorder()
	srv.apiDownloaderTransferStats(w, httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/transfer-stats", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var resp DownloaderTransferStatsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.SpeedHistory, 1)
	assert.Equal(t, int64(300), resp.SpeedHistory[0].Down)
}
