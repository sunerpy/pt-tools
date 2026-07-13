package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
