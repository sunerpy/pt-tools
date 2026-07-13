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

func TestApiSiteDetail_PostSaveConfig(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)

	body, _ := json.Marshal(models.SiteConfig{
		AuthMethod: "cookie",
		Cookie:     "x=1",
		APIUrl:     "https://hdsky.me",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/sites/hdsky", bytes.NewReader(body))
	srv.apiSiteDetail(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestApiSiteDetail_PostBadBody(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/sites/hdsky", bytes.NewBufferString(`{bad`))
	srv.apiSiteDetail(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestApiVersionCheck_Dispatch(t *testing.T) {
	s := &Server{}

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/version/check", nil)
		s.apiVersionCheck(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("get triggers check", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/version/check?proxy=http://127.0.0.1:1", nil)
		s.apiVersionCheck(w, req)
		assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError)
	})
}

func TestApiSiteTorrentDownload_MoreBranches(t *testing.T) {
	s := &Server{}

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/site/hdsky/torrent/1/download", nil)
		s.apiSiteTorrentDownload(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("bad path", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/site/hdsky/bad/1", nil)
		s.apiSiteTorrentDownload(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("download error surfaces 500", func(t *testing.T) {
		withOrchestrator(t, &fakeV2Site{id: "hdsky", name: "HDSky", err: assertErr("boom")})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/site/hdsky/torrent/1/download", nil)
		s.apiSiteTorrentDownload(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestApiBatchTorrentDownload_PartialAndErrors(t *testing.T) {
	s := &Server{}

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v2/torrents/batch-download", nil)
		s.apiBatchTorrentDownload(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("bad body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-download", bytes.NewBufferString(`{bad`))
		s.apiBatchTorrentDownload(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("empty torrents", func(t *testing.T) {
		body, _ := json.Marshal(BatchDownloadRequest{})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-download", bytes.NewReader(body))
		s.apiBatchTorrentDownload(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("orchestrator nil", func(t *testing.T) {
		prev := searchOrchestrator
		searchOrchestrator = nil
		t.Cleanup(func() { searchOrchestrator = prev })
		body, _ := json.Marshal(BatchDownloadRequest{Torrents: []BatchDownloadItem{{SiteID: "hdsky", TorrentID: "1"}}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-download", bytes.NewReader(body))
		s.apiBatchTorrentDownload(w, req)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})
}
