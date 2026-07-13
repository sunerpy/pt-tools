package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

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
