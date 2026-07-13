package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/extension"
	"github.com/sunerpy/pt-tools/models"
)

func TestHandleExtensionActionAck_AlreadyAcked(t *testing.T) {
	srv, cleanup := newExtensionActionTestServer(t)
	defer cleanup()

	require.NoError(t, extension.Enqueue(global.GlobalDB.DB, extension.PendingAction{
		Type: extension.ActionOpenTab, TargetURL: "https://hdsky.me/", SiteName: "hdsky",
	}))

	w1 := httptest.NewRecorder()
	srv.handleExtensionActionAck(w1, authedRequest(http.MethodPost, "/api/extension/actions/1/ack", nil), 1)
	require.Equal(t, http.StatusOK, w1.Code)
	assert.Contains(t, w1.Body.String(), "acked")

	w2 := httptest.NewRecorder()
	srv.handleExtensionActionAck(w2, authedRequest(http.MethodPost, "/api/extension/actions/1/ack", nil), 1)
	require.Equal(t, http.StatusOK, w2.Code)
	assert.Contains(t, w2.Body.String(), "already_acked")
}

func TestHandleExtensionActionAck_NotFound(t *testing.T) {
	srv, cleanup := newExtensionActionTestServer(t)
	defer cleanup()

	w := httptest.NewRecorder()
	srv.handleExtensionActionAck(w, authedRequest(http.MethodPost, "/api/extension/actions/999/ack", nil), 999)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestApiArchiveTorrents_Paths(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfoArchive{}))
	require.NoError(t, db.Create(&models.TorrentInfoArchive{
		SiteName: "hdsky", Title: "Archived One", IsCompleted: true,
	}).Error)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/archive", nil)
		server.apiArchiveTorrents(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("list with site filter and paging", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/torrents/archive?site=hdsky&page=1&page_size=10", nil)
		server.apiArchiveTorrents(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestApiTorrentManagementRouter_Dispatch(t *testing.T) {
	server, _ := setupServerWithFakeDownloader(t, &fakeDownloader{})
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.TorrentInfo{}, &models.TorrentInfoArchive{}))

	t.Run("paused route", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/torrents/paused", nil)
		server.apiTorrentManagementRouter(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("archive route", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/torrents/archive", nil)
		server.apiTorrentManagementRouter(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("resume route", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/5/resume", nil)
		server.apiTorrentManagementRouter(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("unknown route", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/torrents/bogus", nil)
		server.apiTorrentManagementRouter(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
