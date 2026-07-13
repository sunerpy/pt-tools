package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func TestCompareIntHelpers(t *testing.T) {
	assert.Equal(t, -1, compareInt(1, 2))
	assert.Equal(t, 1, compareInt(3, 2))
	assert.Equal(t, 0, compareInt(2, 2))

	assert.Equal(t, -1, compareInt64(1, 2))
	assert.Equal(t, 1, compareInt64(3, 2))
	assert.Equal(t, 0, compareInt64(2, 2))

	assert.Equal(t, -1, compareFloat64(1.0, 2.0))
	assert.Equal(t, 1, compareFloat64(3.0, 2.0))
	assert.Equal(t, 0, compareFloat64(2.0, 2.0))
}

func TestCompareDownloaderTorrentItem(t *testing.T) {
	a := DownloaderTorrentItem{
		DownloaderName: "aaa", DownloaderType: "qbittorrent", Title: "Alpha",
		Progress: 10, Seeds: 1, Connections: 2, Size: 100, UploadSpeed: 5,
		DownloadSpeed: 6, AddedAt: 1000, CompletedAt: 2000, Ratio: 0.5, State: "downloading", ETA: 50,
	}
	b := DownloaderTorrentItem{
		DownloaderName: "bbb", DownloaderType: "transmission", Title: "Beta",
		Progress: 20, Seeds: 3, Connections: 4, Size: 200, UploadSpeed: 7,
		DownloadSpeed: 8, AddedAt: 3000, CompletedAt: 4000, Ratio: 1.5, State: "seeding", ETA: 60,
	}

	sortFields := []string{
		"downloader_name", "downloader_type", "title", "progress", "seeds",
		"connections", "size", "upload_speed", "download_speed", "added_at",
		"completed_at", "ratio", "state", "eta", "unknown_default",
	}
	for _, f := range sortFields {
		t.Run(f, func(t *testing.T) {
			assert.Equal(t, -1, compareDownloaderTorrentItem(a, b, f))
			assert.Equal(t, 1, compareDownloaderTorrentItem(b, a, f))
		})
	}
}

func TestDownloaderCapabilityFromRecord(t *testing.T) {
	rec := downloaderRecord{ID: 3, Name: "qb", Type: "qbittorrent"}
	cap := downloaderCapabilityFromRecord(rec)
	assert.Equal(t, uint(3), cap.DownloaderID)
	assert.Equal(t, "qb", cap.DownloaderName)
	assert.Equal(t, "qbittorrent", cap.DownloaderType)
	assert.True(t, cap.CanPause)
	assert.True(t, cap.CanResume)
	assert.True(t, cap.CanAddTorrent)
	assert.True(t, cap.CanViewTrackers)
}

func TestListEnabledDownloaderRecords(t *testing.T) {
	server, db := setupTestServer(t)

	require.NoError(t, db.Create(&models.DownloaderSetting{Name: "on1", Type: "qbittorrent", Enabled: true}).Error)
	require.NoError(t, db.Create(&models.DownloaderSetting{Name: "on2", Type: "transmission", Enabled: true}).Error)
	require.NoError(t, db.Create(&models.DownloaderSetting{Name: "off1", Type: "qbittorrent", Enabled: false}).Error)

	t.Run("all enabled", func(t *testing.T) {
		recs, err := server.listEnabledDownloaderRecords(nil)
		require.NoError(t, err)
		assert.Len(t, recs, 2)
	})

	t.Run("filter by id", func(t *testing.T) {
		var dl models.DownloaderSetting
		require.NoError(t, db.Where("name = ?", "on1").First(&dl).Error)
		recs, err := server.listEnabledDownloaderRecords(&dl.ID)
		require.NoError(t, err)
		require.Len(t, recs, 1)
		assert.Equal(t, "on1", recs[0].Name)
	})
}

func TestGetDownloaderRecordMap(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.Create(&models.DownloaderSetting{Name: "d1", Type: "qbittorrent", Enabled: true}).Error)
	require.NoError(t, db.Create(&models.DownloaderSetting{Name: "d2", Type: "qbittorrent", Enabled: false}).Error)

	m, err := server.getDownloaderRecordMap()
	require.NoError(t, err)
	assert.Len(t, m, 1)
	for _, rec := range m {
		assert.Equal(t, "d1", rec.Name)
	}
}

func TestApiDownloaderCapabilities(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.Create(&models.DownloaderSetting{Name: "d1", Type: "qbittorrent", Enabled: true}).Error)

	t.Run("GET returns capabilities", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/capabilities", nil)
		server.apiDownloaderCapabilities(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp DownloaderCapabilitiesResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Len(t, resp.Items, 1)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/capabilities", nil)
		server.apiDownloaderCapabilities(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

func TestApiDownloaderTorrents_NoManager(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.Create(&models.DownloaderSetting{Name: "d1", Type: "qbittorrent", Enabled: true}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents", nil)
		server.apiDownloaderTorrents(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("invalid downloader_id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents?downloader_id=abc", nil)
		server.apiDownloaderTorrents(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("manager not init returns 500", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents", nil)
		server.apiDownloaderTorrents(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestApiDownloaderTorrentDetail_BadInput(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.Create(&models.DownloaderSetting{Name: "d1", Type: "qbittorrent", Enabled: true}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/detail", nil)
		server.apiDownloaderTorrentDetail(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("missing params", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/detail", nil)
		server.apiDownloaderTorrentDetail(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid downloader_id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/detail?downloader_id=abc&task_id=x", nil)
		server.apiDownloaderTorrentDetail(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("downloader not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/detail?downloader_id=999&task_id=x", nil)
		server.apiDownloaderTorrentDetail(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestApiDownloaderTorrentActions_BadInput(t *testing.T) {
	server, _ := setupTestServer(t)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/batch-action", nil)
		server.apiDownloaderTorrentActions(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("invalid body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewBufferString(`{bad`))
		server.apiDownloaderTorrentActions(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("empty targets returns ok empty", func(t *testing.T) {
		body, _ := json.Marshal(BatchTorrentActionRequest{Action: "pause"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
		server.apiDownloaderTorrentActions(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("empty action", func(t *testing.T) {
		body, _ := json.Marshal(BatchTorrentActionRequest{
			Targets: []TorrentActionTarget{{DownloaderID: 1, TaskID: "x"}},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
		server.apiDownloaderTorrentActions(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("manager not init", func(t *testing.T) {
		body, _ := json.Marshal(BatchTorrentActionRequest{
			Action:  "pause",
			Targets: []TorrentActionTarget{{DownloaderID: 1, TaskID: "x"}},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
		server.apiDownloaderTorrentActions(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestApiAddDownloaderTorrent_BadInput(t *testing.T) {
	server, _ := setupTestServer(t)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/add", nil)
		server.apiAddDownloaderTorrent(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("invalid body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/add", bytes.NewBufferString(`{bad`))
		server.apiAddDownloaderTorrent(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("no source", func(t *testing.T) {
		body, _ := json.Marshal(AddDownloaderTorrentRequest{})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/add", bytes.NewReader(body))
		server.apiAddDownloaderTorrent(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("manager not init", func(t *testing.T) {
		body, _ := json.Marshal(AddDownloaderTorrentRequest{MagnetLink: "magnet:?xt=urn:btih:abc"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/add", bytes.NewReader(body))
		server.apiAddDownloaderTorrent(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestApiDownloaderTransferStats_NoManager(t *testing.T) {
	server, _ := setupTestServer(t)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/transfer-stats", nil)
		server.apiDownloaderTransferStats(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("manager not init", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/transfer-stats", nil)
		server.apiDownloaderTransferStats(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestApiDownloaderTorrentMeta_NoManager(t *testing.T) {
	server, _ := setupTestServer(t)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/meta", nil)
		server.apiDownloaderTorrentMeta(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("manager not init", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/meta", nil)
		server.apiDownloaderTorrentMeta(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
