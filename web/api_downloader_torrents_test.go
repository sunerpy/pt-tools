package web

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
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

// ==== merged from api_dl_cloak_cov3_test.go ====
func TestApiAddDownloaderTorrent_ListEnabledAndGetDownloaderError(t *testing.T) {
	fake := &fakeDownloader{freSpace: 1 << 40}
	server, _ := setupServerWithFakeDownloader(t, fake)
	gs, err := server.store.GetGlobalSettings()
	require.NoError(t, err)
	require.NoError(t, server.store.SaveGlobalSettings(gs))
	require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
		Where("1 = 1").Update("cleanup_disk_protect", false).Error)

	// A second enabled downloader present in DB but NOT registered in the manager,
	// exercising the GetDownloader error branch.
	require.NoError(t, global.GlobalDB.DB.Create(&models.DownloaderSetting{
		Name: "phantom", Type: "qbittorrent", URL: "http://127.0.0.1:2", Enabled: true,
	}).Error)

	body, _ := json.Marshal(AddDownloaderTorrentRequest{
		MagnetLink: "magnet:?xt=urn:btih:abc",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/add", bytes.NewReader(body))
	server.apiAddDownloaderTorrent(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp AddDownloaderTorrentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.GreaterOrEqual(t, resp.SuccessCount+resp.FailedCount, 1)
}

func TestApiCloakTest_TokenFromStoreOnly(t *testing.T) {
	srv, store, cleanup := newCloakTestServer(t)
	defer cleanup()

	mock := newCloakManagerMock(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","version":"2.0.0"}`))
	})
	defer mock.Close()

	require.NoError(t, store.SaveCloakConfig(mock.URL, "stored-tok", false))

	// Provide endpoint in request but NO token -> token loaded from store.
	body := map[string]any{"endpoint": mock.URL}
	w := httptest.NewRecorder()
	srv.apiCloakTest(w, cloakAuthedReq(http.MethodPost, "/api/cloak/test", body))
	require.Equal(t, http.StatusOK, w.Code)
	var out map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Equal(t, "success", out["category"])
}

// ==== merged from api_downloader_torrents_cov2_test.go ====
func TestApiDownloaderTorrentActions_MoreBranches(t *testing.T) {
	fake := &fakeDownloader{torrents: sampleTorrents()}
	server, _ := setupServerWithFakeDownloader(t, fake)

	t.Run("bad body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewBufferString(`{bad`))
		server.apiDownloaderTorrentActions(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("empty targets", func(t *testing.T) {
		body, _ := json.Marshal(BatchTorrentActionRequest{Action: "pause"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
		server.apiDownloaderTorrentActions(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("empty action", func(t *testing.T) {
		body, _ := json.Marshal(BatchTorrentActionRequest{Targets: []TorrentActionTarget{{DownloaderID: 1, TaskID: "t1"}}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
		server.apiDownloaderTorrentActions(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("unsupported action", func(t *testing.T) {
		body, _ := json.Marshal(BatchTorrentActionRequest{
			Action: "bogus", Targets: []TorrentActionTarget{{DownloaderID: 1, TaskID: "t1"}},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
		server.apiDownloaderTorrentActions(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("set_location without path fails per target", func(t *testing.T) {
		body, _ := json.Marshal(BatchTorrentActionRequest{
			Action: "set_location", Targets: []TorrentActionTarget{{DownloaderID: 1, TaskID: "t1"}},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
		server.apiDownloaderTorrentActions(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp BatchTorrentActionResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp.FailedCount)
	})

	t.Run("set_location with path succeeds", func(t *testing.T) {
		body, _ := json.Marshal(BatchTorrentActionRequest{
			Action: "set_location", SavePath: "/downloads",
			Targets: []TorrentActionTarget{{DownloaderID: 1, TaskID: "t1"}},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
		server.apiDownloaderTorrentActions(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp BatchTorrentActionResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp.SuccessCount)
	})
}

func TestApiDownloaderTorrentActions_BatchError(t *testing.T) {
	fake := &fakeDownloader{torrents: sampleTorrents(), pauseErr: assertErr("pausefail")}
	server, _ := setupServerWithFakeDownloader(t, fake)

	body, _ := json.Marshal(BatchTorrentActionRequest{
		Action: "pause", Targets: []TorrentActionTarget{{DownloaderID: 1, TaskID: "t1"}},
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
	server.apiDownloaderTorrentActions(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp BatchTorrentActionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.FailedCount)
}

func TestApiDownloaderTorrentDetail_ErrorBranches(t *testing.T) {
	server, _ := setupServerWithFakeDownloader(t, &fakeDownloader{getErr: assertErr("nope")})

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

	t.Run("invalid downloader id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/detail?downloader_id=abc&task_id=t1", nil)
		server.apiDownloaderTorrentDetail(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("downloader not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/detail?downloader_id=999&task_id=t1", nil)
		server.apiDownloaderTorrentDetail(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("get torrent error", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/detail?downloader_id=1&task_id=t1", nil)
		server.apiDownloaderTorrentDetail(w, req)
		assert.Equal(t, http.StatusBadGateway, w.Code)
	})
}

func TestApiAddDownloaderTorrent_TorrentFileSuccess(t *testing.T) {
	fake := &fakeDownloader{
		addResult: downloader.AddTorrentResult{Success: true, ID: "new1", Hash: "h1", Message: "ok"},
		freSpace:  1 << 40,
	}
	server, _ := setupServerWithFakeDownloader(t, fake)
	require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
		Where("1 = 1").Update("cleanup_disk_protect", false).Error)

	req := AddDownloaderTorrentRequest{
		DownloaderIDs: []uint{1},
		TorrentBase64: base64.StdEncoding.EncodeToString(minimalTorrentBytes(1024)),
		AddPaused:     true,
		SavePath:      "/dl",
		Category:      "movie",
		Tags:          "hd",
	}
	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/add", bytes.NewReader(body))
	server.apiAddDownloaderTorrent(w, r)
	require.Equal(t, http.StatusOK, w.Code)
	var resp AddDownloaderTorrentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.SuccessCount)
}

func TestApiAddDownloaderTorrent_AddErrorReleasesBudget(t *testing.T) {
	fake := &fakeDownloader{addErr: assertErr("addfail"), freSpace: 1 << 40}
	server, _ := setupServerWithFakeDownloader(t, fake)
	require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
		Where("1 = 1").Update("cleanup_disk_protect", false).Error)

	body, _ := json.Marshal(AddDownloaderTorrentRequest{
		DownloaderIDs: []uint{1},
		MagnetLink:    "magnet:?xt=urn:btih:abc",
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/add", bytes.NewReader(body))
	server.apiAddDownloaderTorrent(w, r)
	require.Equal(t, http.StatusOK, w.Code)
	var resp AddDownloaderTorrentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.FailedCount)
}

// ==== merged from api_downloader_torrents_cov3_test.go ====
func TestApiDownloaderTorrents_ErrorBranches(t *testing.T) {
	t.Run("method not allowed", func(t *testing.T) {
		server, _ := setupTestServer(t)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents", nil)
		server.apiDownloaderTorrents(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("invalid downloader_id", func(t *testing.T) {
		server, _ := setupServerWithFakeDownloader(t, &fakeDownloader{})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents?downloader_id=abc", nil)
		server.apiDownloaderTorrents(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("no manager", func(t *testing.T) {
		server, _ := setupTestServer(t)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents", nil)
		server.apiDownloaderTorrents(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("list error surfaces empty then ok", func(t *testing.T) {
		fake := &fakeDownloader{listErr: assertErr("listfail")}
		server, _ := setupServerWithFakeDownloader(t, fake)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents", nil)
		server.apiDownloaderTorrents(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp DownloaderTorrentsResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 0, resp.Total)
	})

	t.Run("filter by downloader_id with data", func(t *testing.T) {
		fake := &fakeDownloader{torrents: sampleTorrents()}
		server, id := setupServerWithFakeDownloader(t, fake)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents?downloader_id=1&page_size=0", nil)
		require.Equal(t, uint(1), id)
		server.apiDownloaderTorrents(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestApiAddDownloaderTorrent_DiskProtectRejectsMagnet(t *testing.T) {
	fake := &fakeDownloader{freSpace: 1 << 40}
	server, _ := setupServerWithFakeDownloader(t, fake)
	require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
		Where("1 = 1").Updates(map[string]any{
		"cleanup_disk_protect":      true,
		"cleanup_min_disk_space_gb": 10.0,
	}).Error)

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
	assert.Equal(t, 1, resp.FailedCount)
}

func TestApiAddDownloaderTorrent_UnknownDownloaderInList(t *testing.T) {
	fake := &fakeDownloader{
		freSpace: 1 << 40,
	}
	server, _ := setupServerWithFakeDownloader(t, fake)
	require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
		Where("1 = 1").Update("cleanup_disk_protect", false).Error)

	body, _ := json.Marshal(AddDownloaderTorrentRequest{
		DownloaderIDs: []uint{1, 999},
		MagnetLink:    "magnet:?xt=urn:btih:abc",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/add", bytes.NewReader(body))
	server.apiAddDownloaderTorrent(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

// ==== merged from api_downloader_torrents_cov4_test.go ====
func TestDownloaderTorrentSubHandlers_NoManager(t *testing.T) {
	server, _ := setupTestServer(t)

	handlers := []struct {
		name string
		fn   func(http.ResponseWriter, *http.Request)
		mth  string
	}{
		{"torrents", server.apiDownloaderTorrents, http.MethodGet},
		{"meta", server.apiDownloaderTorrentMeta, http.MethodGet},
		{"transfer-stats", server.apiDownloaderTransferStats, http.MethodGet},
	}
	for _, h := range handlers {
		t.Run(h.name+" no manager", func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(h.mth, "/api/x", nil)
			h.fn(w, req)
			assert.Equal(t, http.StatusInternalServerError, w.Code)
		})
	}
}

func TestDownloaderTorrentSubHandlers_MethodNotAllowed(t *testing.T) {
	server, _ := setupServerWithFakeDownloader(t, &fakeDownloader{})

	handlers := []func(http.ResponseWriter, *http.Request){
		server.apiDownloaderTorrentMeta,
		server.apiDownloaderTransferStats,
	}
	for _, fn := range handlers {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/x", nil)
		fn(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	}
}

func TestDownloaderTorrentMeta_ListErrSkips(t *testing.T) {
	fake := &fakeDownloader{listErr: assertErr("boom")}
	server, _ := setupServerWithFakeDownloader(t, fake)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/meta", nil)
	server.apiDownloaderTorrentMeta(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestDownloaderTransferStats_WithStatus(t *testing.T) {
	fake := &fakeDownloader{
		status:   downloader.ClientStatus{UpSpeed: 5, DlSpeed: 6, UpData: 70, DlData: 80},
		freSpace: 4096,
	}
	server, _ := setupServerWithFakeDownloader(t, fake)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/transfer-stats", nil)
	server.apiDownloaderTransferStats(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// ==== merged from api_downloader_torrents_cov5_test.go ====
func TestApiDownloaderTorrentActions_BatchFailsPerTargetFallback(t *testing.T) {
	fake := &fakeDownloader{
		torrents:      sampleTorrents(),
		batchPauseErr: assertErr("batchfail"),
	}
	server, _ := setupServerWithFakeDownloader(t, fake)

	body, _ := json.Marshal(BatchTorrentActionRequest{
		Action:  "pause",
		Targets: []TorrentActionTarget{{DownloaderID: 1, TaskID: "t1"}},
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
	server.apiDownloaderTorrentActions(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp BatchTorrentActionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.SuccessCount)
}

func TestApiDownloaderTorrentActions_PerTargetError(t *testing.T) {
	fake := &fakeDownloader{
		torrents:      sampleTorrents(),
		batchPauseErr: assertErr("batchfail"),
		pauseErr:      assertErr("perfail"),
	}
	server, _ := setupServerWithFakeDownloader(t, fake)

	body, _ := json.Marshal(BatchTorrentActionRequest{
		Action:  "pause",
		Targets: []TorrentActionTarget{{DownloaderID: 1, TaskID: "t1"}},
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
	server.apiDownloaderTorrentActions(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp BatchTorrentActionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.FailedCount)
}

func TestApiDownloaderTorrentActions_RecheckError(t *testing.T) {
	fake := &fakeDownloader{torrents: sampleTorrents()}
	server, _ := setupServerWithFakeDownloader(t, fake)

	body, _ := json.Marshal(BatchTorrentActionRequest{
		Action:  "recheck",
		Targets: []TorrentActionTarget{{DownloaderID: 1, TaskID: "t1"}},
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
	server.apiDownloaderTorrentActions(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp BatchTorrentActionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.SuccessCount)
}

// ==== merged from api_downloader_torrents_cov_test.go ====
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

// ==== merged from api_downloader_torrents_data_test.go ====
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

// ==== merged from api_downloader_torrents_err_test.go ====
func TestApiDownloaderTorrentDetail_GetError(t *testing.T) {
	fake := &fakeDownloader{getErr: errors.New("boom")}
	server, _ := setupServerWithFakeDownloader(t, fake)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/detail?downloader_id=1&task_id=t1", nil)
	server.apiDownloaderTorrentDetail(w, req)
	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestApiAddDownloaderTorrent_AddFailure(t *testing.T) {
	fake := &fakeDownloader{
		addResult: downloader.AddTorrentResult{Success: false, Message: "rejected"},
		addErr:    errors.New("add failed"),
		freSpace:  1 << 40,
	}
	server, _ := setupServerWithFakeDownloader(t, fake)
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
	assert.Equal(t, 1, resp.FailedCount)
}

func TestApiAddDownloaderTorrent_ResultNotSuccess(t *testing.T) {
	fake := &fakeDownloader{
		addResult: downloader.AddTorrentResult{Success: false, Message: "dup"},
		freSpace:  1 << 40,
	}
	server, _ := setupServerWithFakeDownloader(t, fake)
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
	assert.Equal(t, 1, resp.FailedCount)
}

func TestApiDownloaderTorrentActions_ListFallbackSingle(t *testing.T) {
	fake := &fakeDownloader{
		torrents:  sampleTorrents(),
		pauseErr:  errors.New("batch pause failed"),
		resumeErr: nil,
	}
	server, _ := setupServerWithFakeDownloader(t, fake)

	body, _ := json.Marshal(BatchTorrentActionRequest{
		Action:  "pause",
		Targets: []TorrentActionTarget{{DownloaderID: 1, TaskID: "t1"}},
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
	server.apiDownloaderTorrentActions(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp BatchTorrentActionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.FailedCount)
}

// ==== merged from api_extra_cov_test.go ====
func TestProcessTorrentPush_ValidTorrentData(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}, &models.DownloaderSetting{}, &models.SiteSetting{}, &models.SettingsGlobal{}))
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: minimalTorrentBytes(1024)})

	require.NoError(t, db.Create(&models.DownloaderSetting{Name: "qb1", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true, AutoStart: true}).Error)
	require.NoError(t, db.Model(&models.SettingsGlobal{}).Where("1=1").Update("cleanup_disk_protect", false).Error)

	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/push", nil)
	resp := server.processTorrentPush(req, TorrentPushRequest{
		DownloadURL:   "/api/site/hdsky/torrent/1/download",
		DownloaderIDs: []uint{1},
		TorrentTitle:  "T1",
	})
	require.Len(t, resp.Results, 1)
	assert.Equal(t, uint(1), resp.Results[0].DownloaderID)
}

func TestApiFavicon_FetchThroughDefinition(t *testing.T) {
	server := setupFaviconServer(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47, 1, 2, 3})
	}))
	defer ts.Close()

	require.NoError(t, faviconService.fetchAndSave("hdsky", "HDSky", ts.URL))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/favicon/hdsky", nil)
	server.apiFavicon(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Body.Bytes())
}

func TestApiDeleteTasks_Errors(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}))

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/tasks/batch-delete", nil)
		server.apiDeleteTasks(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("invalid body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/tasks/batch-delete", bytes.NewBufferString(`{bad`))
		server.apiDeleteTasks(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("no rows returns zero", func(t *testing.T) {
		body, _ := json.Marshal(DeleteTasksRequest{IDs: []uint{999}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/tasks/batch-delete", bytes.NewReader(body))
		server.apiDeleteTasks(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp DeleteTasksResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 0, resp.Success)
	})
}

func TestApiDownloaderRouter_ApplyToSites(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}, &models.DownloaderSetting{}, &models.RSSSubscription{}))
	require.NoError(t, db.Create(&models.DownloaderSetting{Name: "qb1", Type: "qbittorrent", Enabled: true}).Error)

	body, _ := json.Marshal(ApplyDownloaderToSitesRequest{SiteIDs: nil})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/apply-to-sites", bytes.NewReader(body))
	server.apiDownloaderRouter(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestApiDownloaderTorrentActions_SetLocation(t *testing.T) {
	fake := &fakeDownloader{torrents: sampleTorrents()}
	server, _ := setupServerWithFakeDownloader(t, fake)

	t.Run("set_location missing path fails", func(t *testing.T) {
		body, _ := json.Marshal(BatchTorrentActionRequest{
			Action:  "set_location",
			Targets: []TorrentActionTarget{{DownloaderID: 1, TaskID: "t1"}},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
		server.apiDownloaderTorrentActions(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp BatchTorrentActionResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp.FailedCount)
	})

	t.Run("set_location with path succeeds", func(t *testing.T) {
		body, _ := json.Marshal(BatchTorrentActionRequest{
			Action:   "set_location",
			SavePath: "/downloads",
			Targets:  []TorrentActionTarget{{DownloaderID: 1, TaskID: "t1"}},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
		server.apiDownloaderTorrentActions(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp BatchTorrentActionResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp.SuccessCount)
	})

	t.Run("unknown action", func(t *testing.T) {
		body, _ := json.Marshal(BatchTorrentActionRequest{
			Action:  "explode",
			Targets: []TorrentActionTarget{{DownloaderID: 1, TaskID: "t1"}},
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body))
		server.apiDownloaderTorrentActions(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}
