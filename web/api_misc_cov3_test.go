package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

func TestApiDownloaderTorrentDetail_FilesTrackersErrFallback(t *testing.T) {
	fake := &fakeDownloader{
		torrents:   sampleTorrents(),
		filesErr:   assertErr("filesfail"),
		trackerErr: assertErr("trackerfail"),
	}
	server, _ := setupServerWithFakeDownloader(t, fake)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/detail?downloader_id=1&task_id=t1", nil)
	server.apiDownloaderTorrentDetail(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp TorrentDetailResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.Files)
	assert.Empty(t, resp.Trackers)
}

func TestApiDownloaderTorrentDetail_DownloaderConnFail(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.Create(&models.DownloaderSetting{
		Name: "unreach", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true,
	}).Error)
	mgr := scheduler.NewManager()
	t.Cleanup(func() { mgr.StopAll() })
	server.mgr = mgr

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/downloader-torrents/detail?downloader_id=1&task_id=t1", nil)
	server.apiDownloaderTorrentDetail(w, req)
	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestApiSiteLoginStateVisit_NilDB(t *testing.T) {
	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })

	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/visit", nil)
	s.apiSiteLoginStateVisit(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestApiSiteLoginStateVisit_BadTimestamp(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	body := []byte(`{"site_name":"hdsky","last_visit_at":"not-a-time"}`)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/visit", bytes.NewReader(body))
	srv.apiSiteLoginStateVisit(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

var _ = context.Background
