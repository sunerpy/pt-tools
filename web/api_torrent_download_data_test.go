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

func TestApiSiteTorrentDownload_WithOrchestrator(t *testing.T) {
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: []byte("torrentdata")})

	s := &Server{}

	t.Run("download succeeds", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/site/hdsky/torrent/42/download?title=Cool", nil)
		s.apiSiteTorrentDownload(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/x-bittorrent", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Header().Get("Content-Disposition"), "[hdsky]Cool.torrent")
		assert.Equal(t, "torrentdata", w.Body.String())
	})

	t.Run("site not found", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/site/unknown/torrent/1/download", nil)
		s.apiSiteTorrentDownload(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestApiSiteTorrentDownload_WithHash(t *testing.T) {
	withOrchestrator(t, &fakeV2Site{id: "hddolby", name: "HDDolby", hashData: []byte("hashdata")})

	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/site/hddolby/torrent/7/download?downhash=abc", nil)
	s.apiSiteTorrentDownload(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "hashdata", w.Body.String())
}

func TestApiBatchTorrentDownload_WithOrchestrator(t *testing.T) {
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: []byte("data1")})

	s := &Server{}

	t.Run("all fail returns 500", func(t *testing.T) {
		body, _ := json.Marshal(BatchDownloadRequest{Torrents: []BatchDownloadItem{
			{SiteID: "unknown", TorrentID: "1"},
		}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-download", bytes.NewReader(body))
		s.apiBatchTorrentDownload(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("partial success returns archive", func(t *testing.T) {
		body, _ := json.Marshal(BatchDownloadRequest{Torrents: []BatchDownloadItem{
			{SiteID: "hdsky", TorrentID: "1", Title: "One"},
			{SiteID: "unknown", TorrentID: "2"},
		}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-download", bytes.NewReader(body))
		s.apiBatchTorrentDownload(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/gzip", w.Header().Get("Content-Type"))
		assert.NotEmpty(t, w.Body.Bytes())
	})
}

func TestProcessTorrentPush_WithOrchestrator(t *testing.T) {
	withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", data: []byte("data")})

	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/push", nil)

	t.Run("site not found", func(t *testing.T) {
		resp := s.processTorrentPush(req, TorrentPushRequest{
			DownloadURL:   "/api/site/unknown/torrent/1/download",
			DownloaderIDs: []uint{1},
		})
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Message, "Site not found")
	})
}
