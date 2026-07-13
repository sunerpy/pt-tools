package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// ==== merged from api_torrent_push_cov2_test.go ====
func TestProcessTorrentPush_SiteNotFound(t *testing.T) {
	server, _ := setupTestServer(t)
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: []byte("x")})

	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/push", nil)
	resp := server.processTorrentPush(req, TorrentPushRequest{
		DownloadURL:   "/api/site/othersite/torrent/1/download",
		DownloaderIDs: []uint{1},
	})
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Message, "Site not found")
}

func TestProcessTorrentPush_DownloadWithHash(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}, &models.DownloaderSetting{}, &models.SiteSetting{}))
	withOrchestrator(t, &fakeV2Site{id: "hddolby", name: "HDDolby", hashData: minimalTorrentBytes(1024)})
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb1", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true,
	}).Error)

	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/push", nil)
	resp := server.processTorrentPush(req, TorrentPushRequest{
		DownloadURL:   "/api/site/hddolby/torrent/42/download?downhash=abcdef",
		DownloaderIDs: []uint{1},
		TorrentTitle:  "HashT",
	})
	// hash download succeeded; downloader connection fails -> result recorded, not successful
	require.Len(t, resp.Results, 1)
	assert.Equal(t, uint(1), resp.Results[0].DownloaderID)
}

func TestProcessTorrentPush_HashDownloadError(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}, &models.DownloaderSetting{}))
	withOrchestrator(t, &fakeV2Site{id: "hddolby", name: "HDDolby", hashErr: assertErr("hashboom")})

	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/push", nil)
	resp := server.processTorrentPush(req, TorrentPushRequest{
		DownloadURL:   "/api/site/hddolby/torrent/42/download?downhash=abc",
		DownloaderIDs: []uint{1},
	})
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Message, "Failed to download torrent")
}

func TestProcessTorrentPush_OverrideSiteAndTorrentID(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}, &models.DownloaderSetting{}))
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: minimalTorrentBytes(512)})
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb1", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true,
	}).Error)

	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/push", nil)
	resp := server.processTorrentPush(req, TorrentPushRequest{
		DownloadURL:   "/api/site/ignored/torrent/0/download",
		TorrentID:     "77",
		SourceSite:    "hdsky",
		DownloaderIDs: []uint{1},
		TorrentTitle:  "OverrideT",
	})
	require.Len(t, resp.Results, 1)
}

func TestApiTorrentPush_FullDispatch(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}, &models.DownloaderSetting{}))
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: minimalTorrentBytes(256)})
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb1", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true,
	}).Error)

	body := `{"downloadUrl":"/api/site/hdsky/torrent/1/download","downloaderIds":[1],"torrentTitle":"T"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/push", strings.NewReader(body))
	server.apiTorrentPush(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestApiTorrentBatchPush_FullDispatch(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}, &models.DownloaderSetting{}))
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: minimalTorrentBytes(256)})
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb1", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true,
	}).Error)

	body := `{"torrents":[{"downloadUrl":"/api/site/hdsky/torrent/1/download","torrentTitle":"t1"}],"downloaderIds":[1]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-push", strings.NewReader(body))
	server.apiTorrentBatchPush(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// ==== merged from api_torrent_push_cov3_test.go ====
// qbitMockServer returns an httptest server emulating the minimal qBittorrent
// API surface used by the push pipeline: login, add, properties (not-found so
// the torrent is treated as new), maindata (free space), and info.
func qbitMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			_, _ = w.Write([]byte("v4.6.0"))
		case "/api/v2/torrents/properties":
			w.WriteHeader(http.StatusNotFound)
		case "/api/v2/sync/maindata":
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":1099511627776}}`))
		case "/api/v2/torrents/add":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
}

func TestProcessTorrentPush_FullSuccessViaQbitMock(t *testing.T) {
	writeWebTestSecretKey(t)
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}, &models.DownloaderSetting{}, &models.SiteSetting{}, &models.SettingsGlobal{}))
	require.NoError(t, db.Create(&models.SettingsGlobal{DownloadDir: "downloads", CleanupDiskProtect: false}).Error)

	mock := qbitMockServer(t)
	defer mock.Close()

	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qbmock", Type: "qbittorrent", URL: mock.URL, Enabled: true, AutoStart: false,
	}).Error)

	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: minimalTorrentBytes(4096)})

	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/push", nil)
	resp := server.processTorrentPush(req, TorrentPushRequest{
		DownloadURL:   "/api/site/hdsky/torrent/1/download",
		DownloaderIDs: []uint{1},
		TorrentTitle:  "PushOK",
	})
	require.Len(t, resp.Results, 1)
	assert.True(t, resp.Results[0].Success)
	assert.True(t, resp.Success)
	_ = global.GlobalDB
}

func TestProcessBatchTorrentPush_MixedViaQbitMock(t *testing.T) {
	writeWebTestSecretKey(t)
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}, &models.DownloaderSetting{}, &models.SiteSetting{}, &models.SettingsGlobal{}))
	require.NoError(t, db.Create(&models.SettingsGlobal{DownloadDir: "downloads", CleanupDiskProtect: false}).Error)

	mock := qbitMockServer(t)
	defer mock.Close()
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qbmock", Type: "qbittorrent", URL: mock.URL, Enabled: true,
	}).Error)

	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: minimalTorrentBytes(4096)})

	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-push", nil)
	resp := server.processBatchTorrentPush(req, BatchTorrentPushRequest{
		Torrents: []TorrentPushItem{
			{DownloadURL: "/api/site/hdsky/torrent/1/download", TorrentTitle: "t1"},
		},
		DownloaderIDs: []uint{1},
	})
	assert.Equal(t, 1, resp.TotalCount)
	assert.GreaterOrEqual(t, resp.SuccessCount+resp.SkippedCount+resp.FailedCount, 1)
}

// ==== merged from api_torrent_push_cov_test.go ====
func TestParseDownloadURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantErr  bool
		wantSite string
		wantID   string
		wantHash string
	}{
		{"valid with download suffix", "/api/site/hdsky/torrent/42/download", false, "hdsky", "42", ""},
		{"valid without suffix", "/api/site/mteam/torrent/7", false, "mteam", "7", ""},
		{"with downhash", "/api/site/hddolby/torrent/9/download?downhash=abc123", false, "hddolby", "9", "abc123"},
		{"bad prefix", "/other/path", true, "", "", ""},
		{"too few parts", "/api/site/hdsky", true, "", "", ""},
		{"wrong second segment", "/api/site/hdsky/notorrent/42", true, "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDownloadURL(tt.url)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantSite, got.SiteID)
			assert.Equal(t, tt.wantID, got.TorrentID)
			assert.Equal(t, tt.wantHash, got.Downhash)
		})
	}
}

func TestApiTorrentPush_BadInput(t *testing.T) {
	s := &Server{}

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v2/torrents/push", nil)
		s.apiTorrentPush(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("invalid body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/push", bytes.NewBufferString(`{bad`))
		s.apiTorrentPush(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing download url", func(t *testing.T) {
		body, _ := json.Marshal(TorrentPushRequest{DownloaderIDs: []uint{1}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/push", bytes.NewReader(body))
		s.apiTorrentPush(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("missing downloader ids", func(t *testing.T) {
		body, _ := json.Marshal(TorrentPushRequest{DownloadURL: "/api/site/hdsky/torrent/1/download"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/push", bytes.NewReader(body))
		s.apiTorrentPush(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestApiTorrentBatchPush_BadInput(t *testing.T) {
	s := &Server{}

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v2/torrents/batch-push", nil)
		s.apiTorrentBatchPush(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("invalid body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-push", bytes.NewBufferString(`{bad`))
		s.apiTorrentBatchPush(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("empty torrents", func(t *testing.T) {
		body, _ := json.Marshal(BatchTorrentPushRequest{DownloaderIDs: []uint{1}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-push", bytes.NewReader(body))
		s.apiTorrentBatchPush(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("empty downloader ids", func(t *testing.T) {
		body, _ := json.Marshal(BatchTorrentPushRequest{Torrents: []TorrentPushItem{{DownloadURL: "/api/site/hdsky/torrent/1/download"}}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-push", bytes.NewReader(body))
		s.apiTorrentBatchPush(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestProcessTorrentPush_InvalidURL(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/push", nil)
	resp := s.processTorrentPush(req, TorrentPushRequest{DownloadURL: "/bad/url", DownloaderIDs: []uint{1}})
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Message, "Invalid download URL")
}

func TestProcessTorrentPush_OrchestratorNil(t *testing.T) {
	// Ensure orchestrator is nil for this test path.
	prev := searchOrchestrator
	searchOrchestrator = nil
	t.Cleanup(func() { searchOrchestrator = prev })

	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/push", nil)
	resp := s.processTorrentPush(req, TorrentPushRequest{
		DownloadURL:   "/api/site/hdsky/torrent/42/download",
		DownloaderIDs: []uint{1},
	})
	assert.False(t, resp.Success)
	assert.Equal(t, "Search service not initialized", resp.Message)
}

func TestProcessBatchTorrentPush_AllFail(t *testing.T) {
	prev := searchOrchestrator
	searchOrchestrator = nil
	t.Cleanup(func() { searchOrchestrator = prev })

	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-push", nil)
	resp := s.processBatchTorrentPush(req, BatchTorrentPushRequest{
		Torrents: []TorrentPushItem{
			{DownloadURL: "/api/site/hdsky/torrent/1/download", TorrentTitle: "t1"},
			{DownloadURL: "/api/site/hdsky/torrent/2/download", TorrentTitle: "t2"},
		},
		DownloaderIDs: []uint{1},
	})
	assert.Equal(t, 2, resp.TotalCount)
	assert.Equal(t, 2, resp.FailedCount)
	assert.False(t, resp.Success)
}

// ==== merged from api_torrent_push_data_test.go ====
func TestProcessTorrentPush_DownloadPath(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}, &models.DownloaderSetting{}, &models.SiteSetting{}))
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: []byte("torrentbytes")})

	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "qb1", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true,
	}).Error)

	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/push", nil)

	t.Run("download error surfaces", func(t *testing.T) {
		withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", err: assertErr("boom")})
		resp := server.processTorrentPush(req, TorrentPushRequest{
			DownloadURL:   "/api/site/hdsky/torrent/1/download",
			DownloaderIDs: []uint{1},
		})
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Message, "Failed to download torrent")
	})

	t.Run("push to unreachable downloader reports failure", func(t *testing.T) {
		withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: []byte("torrentbytes")})
		resp := server.processTorrentPush(req, TorrentPushRequest{
			DownloadURL:   "/api/site/hdsky/torrent/1/download",
			DownloaderIDs: []uint{1},
			TorrentTitle:  "T1",
		})
		// downloader instance creation/connection fails -> not successful,
		// but the result set has one entry for the target downloader.
		require.Len(t, resp.Results, 1)
		assert.Equal(t, uint(1), resp.Results[0].DownloaderID)
		assert.False(t, resp.Success)
	})
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
