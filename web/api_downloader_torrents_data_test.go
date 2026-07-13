package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// setupServerWithFakeDownloader creates a server whose scheduler.Manager has a
// registered fake downloader named "qb1" wired through the real
// DownloaderManager factory/config path.
func setupServerWithFakeDownloader(t *testing.T, fake *fakeDownloader) (*Server, uint) {
	t.Helper()
	server, db := setupTestServer(t)

	mgr := scheduler.NewManager()
	t.Cleanup(func() { mgr.StopAll() })
	dm := mgr.GetDownloaderManager()
	dm.RegisterFactory(downloader.DownloaderQBittorrent, func(_ downloader.DownloaderConfig, name string) (downloader.Downloader, error) {
		fake.name = name
		fake.dlType = downloader.DownloaderQBittorrent
		return fake, nil
	})
	require.NoError(t, dm.RegisterConfig("qb1", downloader.NewGenericConfig(
		downloader.DownloaderQBittorrent, "http://localhost:8080", "u", "p", true,
	), true))

	server.mgr = mgr

	dl := models.DownloaderSetting{Name: "qb1", Type: "qbittorrent", Enabled: true}
	require.NoError(t, db.Create(&dl).Error)
	return server, dl.ID
}

func sampleTorrents() []downloader.Torrent {
	return []downloader.Torrent{
		{
			ID: "t1", InfoHash: "hash1", Name: "Alpha Movie", Progress: 0.5,
			State: downloader.TorrentDownloading, TotalSize: 1000, Category: "movie",
			Tags: "hd,foreign", DateAdded: 100, Ratio: 0.5,
		},
		{
			ID: "t2", InfoHash: "hash2", Name: "Beta Show", Progress: 1.0,
			State: downloader.TorrentSeeding, TotalSize: 2000, Category: "tv",
			Tags: "sd", DateAdded: 200, Ratio: 2.0,
		},
	}
}

func TestApiDownloaderTorrents_WithData(t *testing.T) {
	fake := &fakeDownloader{torrents: sampleTorrents()}
	server, _ := setupServerWithFakeDownloader(t, fake)

	t.Run("list all", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents", nil)
		server.apiDownloaderTorrents(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp DownloaderTorrentsResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 2, resp.Total)
	})

	t.Run("search filter", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents?search=alpha", nil)
		server.apiDownloaderTorrents(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp DownloaderTorrentsResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, 1, resp.Total)
		assert.Equal(t, "Alpha Movie", resp.Items[0].Title)
	})

	t.Run("state filter", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents?state=seeding", nil)
		server.apiDownloaderTorrents(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp DownloaderTorrentsResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp.Total)
	})

	t.Run("category filter", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents?category=tv", nil)
		server.apiDownloaderTorrents(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp DownloaderTorrentsResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp.Total)
	})

	t.Run("tag filter and sort asc", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents?tag=hd&sort_by=title&sort_order=asc&page_size=1&page=1", nil)
		server.apiDownloaderTorrents(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp DownloaderTorrentsResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp.Total)
	})
}

func TestApiDownloaderTorrentDetail_WithData(t *testing.T) {
	fake := &fakeDownloader{
		torrents: sampleTorrents(),
		files:    []downloader.TorrentFile{{Index: 0, Name: "file.mkv", Size: 999, Progress: 0.5, Priority: 1}},
		trackers: []downloader.TorrentTracker{{URL: "http://tr", Status: 2, Peers: 3, Seeds: 4, Leeches: 1}},
	}
	server, dlID := setupServerWithFakeDownloader(t, fake)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/detail?downloader_id=1&task_id=t1", nil)
	// path-based downloader id is via query; dlID must be 1
	require.Equal(t, uint(1), dlID)
	server.apiDownloaderTorrentDetail(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp TorrentDetailResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "t1", resp.Torrent.TaskID)
	assert.Len(t, resp.Files, 1)
	assert.Len(t, resp.Trackers, 1)
}

func TestApiDownloaderTorrentActions_WithData(t *testing.T) {
	fake := &fakeDownloader{torrents: sampleTorrents()}
	server, _ := setupServerWithFakeDownloader(t, fake)

	tests := []string{"pause", "resume", "delete", "delete_with_files", "recheck"}
	for _, action := range tests {
		t.Run(action, func(t *testing.T) {
			body, _ := json.Marshal(BatchTorrentActionRequest{
				Action:  action,
				Targets: []TorrentActionTarget{{DownloaderID: 1, TaskID: "t1"}},
			})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
			server.apiDownloaderTorrentActions(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp BatchTorrentActionResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.GreaterOrEqual(t, resp.SuccessCount+resp.FailedCount, 1)
		})
	}

	t.Run("unknown downloader id fails", func(t *testing.T) {
		body, _ := json.Marshal(BatchTorrentActionRequest{
			Action:  "pause",
			Targets: []TorrentActionTarget{{DownloaderID: 999, TaskID: "x"}},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
		server.apiDownloaderTorrentActions(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp BatchTorrentActionResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp.FailedCount)
	})
}

func TestApiDownloaderTransferStats_WithData(t *testing.T) {
	fake := &fakeDownloader{
		status:   downloader.ClientStatus{UpSpeed: 10, DlSpeed: 20, UpData: 100, DlData: 200},
		freSpace: 5000,
	}
	server, _ := setupServerWithFakeDownloader(t, fake)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/transfer-stats", nil)
	server.apiDownloaderTransferStats(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp DownloaderTransferStatsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, int64(10), resp.TotalUploadSpeed)
	assert.Equal(t, int64(5000), resp.TotalFreeSpace)
	assert.Len(t, resp.Downloaders, 1)
}

func TestApiDownloaderTorrentMeta_WithData(t *testing.T) {
	fake := &fakeDownloader{torrents: sampleTorrents()}
	server, _ := setupServerWithFakeDownloader(t, fake)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/meta", nil)
	server.apiDownloaderTorrentMeta(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp DownloaderTorrentMetaResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp.Categories, "movie")
	assert.Contains(t, resp.Categories, "tv")
	assert.Contains(t, resp.Tags, "hd")
}

func TestApiAddDownloaderTorrent_WithData(t *testing.T) {
	fake := &fakeDownloader{
		addResult: downloader.AddTorrentResult{Success: true, ID: "new1", Hash: "h1", Message: "ok"},
		freSpace:  1 << 40,
	}
	server, _ := setupServerWithFakeDownloader(t, fake)

	t.Run("magnet add succeeds with disk protect off", func(t *testing.T) {
		gs, err := server.store.GetGlobalSettings()
		require.NoError(t, err)
		require.NoError(t, server.store.SaveGlobalSettings(gs))
		require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
			Where("1 = 1").Update("cleanup_disk_protect", false).Error)

		body, _ := json.Marshal(AddDownloaderTorrentRequest{
			DownloaderIDs: []uint{1},
			MagnetLink:    "magnet:?xt=urn:btih:abc",
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/add", bytes.NewReader(body))
		server.apiAddDownloaderTorrent(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp AddDownloaderTorrentResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp.SuccessCount)
	})

	t.Run("no downloaders matched", func(t *testing.T) {
		body, _ := json.Marshal(AddDownloaderTorrentRequest{
			DownloaderIDs: []uint{999},
			MagnetLink:    "magnet:?xt=urn:btih:abc",
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/add", bytes.NewReader(body))
		server.apiAddDownloaderTorrent(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid base64", func(t *testing.T) {
		body, _ := json.Marshal(AddDownloaderTorrentRequest{
			DownloaderIDs: []uint{1},
			TorrentBase64: "!!!not-base64!!!",
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/add", bytes.NewReader(body))
		server.apiAddDownloaderTorrent(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	_ = global.GlobalDB
	_ = core.NewConfigStore
}
