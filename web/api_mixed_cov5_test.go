package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func TestApiResumeTorrent_DownloaderErrors(t *testing.T) {
	t.Run("resume torrent fails", func(t *testing.T) {
		fake := &fakeDownloader{resumeErr: assertErr("resumefail")}
		server, _ := setupServerWithFakeDownloader(t, fake)
		require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.TorrentInfo{}))
		ti := models.TorrentInfo{
			SiteName: "s", TorrentID: "r1", IsPausedBySystem: true,
			DownloaderTaskID: "task-r1", DownloaderName: "qb1",
		}
		require.NoError(t, global.GlobalDB.DB.Create(&ti).Error)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/1/resume", nil)
		server.apiResumeTorrent(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("downloader not found in manager", func(t *testing.T) {
		server, _ := setupServerWithFakeDownloader(t, &fakeDownloader{})
		require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.TorrentInfo{}))
		ti := models.TorrentInfo{
			SiteName: "s", TorrentID: "r2", IsPausedBySystem: true,
			DownloaderTaskID: "task-r2", DownloaderName: "no-such-dl",
		}
		require.NoError(t, global.GlobalDB.DB.Create(&ti).Error)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/torrents/1/resume", nil)
		server.apiResumeTorrent(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestApiUserInfoSiteDetail_DeleteFail(t *testing.T) {
	svc := setupTestUserInfoService()
	InitUserInfoService(svc)
	t.Cleanup(func() { InitUserInfoService(nil) })

	s := &Server{}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v2/userinfo/sites/site1", nil)
	s.apiUserInfoSiteDetail(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestApiSiteLoginStateList_WithStoredStates(t *testing.T) {
	srv, cleanup := newSiteLoginTestServer(t)
	defer cleanup()

	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", DisplayName: "HDSky", Enabled: true}).Error)
	la := time.Now().Add(-time.Hour)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName: "hdsky", BanThresholdDays: 30, RemindBeforeDays: 10,
		ReminderCron: "0 10 * * *", LastReminderTier: "none", ProbeMode: "auto",
		LastAccessAt: &la,
	}).Error)

	w := httptest.NewRecorder()
	srv.apiSiteLoginStateList(w, authedRequest(http.MethodGet, "/api/sites/login-state", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var out []SiteLoginStateResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Len(t, out, 1)
}

var (
	_ = context.Background
	_ = bytes.NewReader
)
