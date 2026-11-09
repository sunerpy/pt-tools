package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func getSeedingStats(t *testing.T, server *Server) DownloaderSeedingResponse {
	t.Helper()
	w := httptest.NewRecorder()
	server.apiDownloaderSeedingStats(w, httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/seeding-stats", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp DownloaderSeedingResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

// 正在做种的只算状态是做种的；下载完了、暂停停止排队校验出错的另外算；没下载完的都不算。
func TestDownloaderSeedingStats_CountsByState(t *testing.T) {
	fake := &fakeDownloader{torrents: []downloader.Torrent{
		{ID: "s1", State: downloader.TorrentSeeding, Progress: 1, IsCompleted: true, TotalSize: 100},
		{ID: "s2", State: downloader.TorrentSeeding, Progress: 1, IsCompleted: true, TotalSize: 200},
		{ID: "p1", State: downloader.TorrentPaused, Progress: 1, IsCompleted: true, TotalSize: 40},
		{ID: "q1", State: downloader.TorrentQueued, Progress: 1, IsCompleted: true, TotalSize: 8},
		{ID: "c1", State: downloader.TorrentChecking, Progress: 1, TotalSize: 2},
		{ID: "d1", State: downloader.TorrentDownloading, Progress: 0.5, TotalSize: 1000},
		{ID: "p2", State: downloader.TorrentPaused, Progress: 0.2, TotalSize: 500},
	}}
	server, id := setupServerWithFakeDownloader(t, fake)

	resp := getSeedingStats(t, server)
	require.Len(t, resp.Downloaders, 1)
	got := resp.Downloaders[0]
	assert.Equal(t, DownloaderSeedingItem{
		DownloaderID: id, DownloaderName: "qb1", DownloaderType: "qbittorrent", Reachable: true,
		SeedingCount: 2, SeedingSize: 300, IdleCount: 3, IdleSize: 50,
	}, got)
	assert.Equal(t, 2, resp.TotalSeedingCount)
	assert.Equal(t, int64(300), resp.TotalSeedingSize)
}

// 连不上、读不到列表的那台照样列出、写明原因，合计只算读到了的；顺序和下载器设置一样。
func TestDownloaderSeedingStats_ListsUnreachable(t *testing.T) {
	server, db := setupTestServer(t)
	mgr := scheduler.NewManager()
	t.Cleanup(func() { mgr.StopAll() })
	dm := mgr.GetDownloaderManager()
	fakes := map[string]*fakeDownloader{
		"ok":     {torrents: []downloader.Torrent{{ID: "a", State: downloader.TorrentSeeding, IsCompleted: true, TotalSize: 7}}},
		"broken": {listErr: errors.New("list failed")},
	}
	dm.RegisterFactory(downloader.DownloaderQBittorrent, func(_ downloader.DownloaderConfig, name string) (downloader.Downloader, error) {
		if f, ok := fakes[name]; ok {
			return f, nil
		}
		return nil, errors.New("dial tcp: connection refused")
	})
	for _, name := range []string{"ok", "broken", "down"} {
		require.NoError(t, dm.RegisterConfig(name, downloader.NewGenericConfig(
			downloader.DownloaderQBittorrent, "http://localhost:8080", "u", "p", false,
		), false))
		require.NoError(t, db.Create(&models.DownloaderSetting{Name: name, Type: "qbittorrent", Enabled: true}).Error)
	}
	require.NoError(t, db.Create(&models.DownloaderSetting{Name: "off", Type: "qbittorrent", Enabled: false}).Error)
	server.mgr = mgr

	resp := getSeedingStats(t, server)
	require.Len(t, resp.Downloaders, 3, "停用的下载器不列")
	names := []string{resp.Downloaders[0].DownloaderName, resp.Downloaders[1].DownloaderName, resp.Downloaders[2].DownloaderName}
	assert.Equal(t, []string{"ok", "broken", "down"}, names)

	assert.True(t, resp.Downloaders[0].Reachable)
	assert.Equal(t, 1, resp.Downloaders[0].SeedingCount)
	assert.False(t, resp.Downloaders[1].Reachable)
	assert.Contains(t, resp.Downloaders[1].Error, "list failed")
	assert.False(t, resp.Downloaders[2].Reachable)
	assert.NotEmpty(t, resp.Downloaders[2].Error)
	assert.Zero(t, resp.Downloaders[2].SeedingCount)

	assert.Equal(t, 1, resp.TotalSeedingCount)
	assert.Equal(t, int64(7), resp.TotalSeedingSize)
}

func TestDownloaderSeedingStats_Errors(t *testing.T) {
	server, _ := setupTestServer(t)

	w := httptest.NewRecorder()
	server.apiDownloaderSeedingStats(w, httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/seeding-stats", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	w = httptest.NewRecorder()
	server.apiDownloaderSeedingStats(w, httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/seeding-stats", nil))
	assert.Equal(t, http.StatusInternalServerError, w.Code, "下载器管理器没初始化")
}

// 走真实的路由：要登录，登录后能取到；不会被 /api/downloader-torrents/ 的任务详情接住。
func TestDownloaderSeedingStats_ThroughMux(t *testing.T) {
	fake := &fakeDownloader{torrents: []downloader.Torrent{{ID: "a", State: downloader.TorrentSeeding, IsCompleted: true, TotalSize: 9}}}
	server, _ := setupServerWithFakeDownloader(t, fake)

	mux := http.NewServeMux()
	server.registerDownloaderHubRoutes(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	const sid = "test-session"
	server.sessions.put(sid, "admin")
	get := func(withSession bool) *http.Response {
		req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/downloader-torrents/seeding-stats", nil)
		require.NoError(t, err)
		if withSession {
			req.AddCookie(&http.Cookie{Name: "session", Value: sid})
		}
		resp, err := ts.Client().Do(req)
		require.NoError(t, err)
		return resp
	}

	resp := get(false)
	resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	resp = get(true)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out DownloaderSeedingResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.Len(t, out.Downloaders, 1)
	assert.Equal(t, int64(9), out.Downloaders[0].SeedingSize)
}
