package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestApiUserInfoSiteDetail_PostWithStore(t *testing.T) {
	writeWebTestSecretKey(t)
	svc := setupTestUserInfoService()
	InitUserInfoService(svc)
	t.Cleanup(func() { InitUserInfoService(nil) })

	srv := newLoginMonitorServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/userinfo/sites/site1", nil)
	srv.apiUserInfoSiteDetail(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "site1")
}

func TestApiDeleteTasks_MixedPushed(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfo{}))

	require.NoError(t, db.Create(&models.TorrentInfo{SiteName: "s", TorrentID: "a"}).Error)
	require.NoError(t, db.Create(&models.TorrentInfo{SiteName: "s", TorrentID: "b"}).Error)

	body, _ := json.Marshal(DeleteTasksRequest{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tasks/batch-delete", bytes.NewReader(body))
	server.apiDeleteTasks(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp DeleteTasksResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.Success)
}

func TestApiDeletePausedTorrents_NoDownloaderInfo(t *testing.T) {
	server, _ := setupServerWithFakeDownloader(t, &fakeDownloader{})
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.TorrentInfo{}))

	ti := models.TorrentInfo{SiteName: "s", TorrentID: "p1", IsPausedBySystem: true}
	require.NoError(t, global.GlobalDB.DB.Create(&ti).Error)

	body, _ := json.Marshal(DeletePausedRequest{IDs: []uint{ti.ID}})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/torrents/delete-paused", bytes.NewReader(body))
	server.apiDeletePausedTorrents(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp DeletePausedResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.Success)
}

func TestApiSiteLoginStateVisit_UnknownSiteInits(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	body, _ := json.Marshal(SiteVisitReportRequest{SiteName: "brandnew", LastVisitAt: "2026-01-01T00:00:00Z"})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/sites/login-state/visit", bytes.NewReader(body))
	srv.apiSiteLoginStateVisit(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}
