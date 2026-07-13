package web

import (
	"net/http"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func setupChatOpsDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	prev := global.GlobalDB
	global.GlobalDB = &models.TorrentDB{DB: db}
	t.Cleanup(func() { global.GlobalDB = prev })
	require.NoError(t, db.AutoMigrate(&models.RSSNotificationLog{}))
}

func TestListRSSNotifications(t *testing.T) {
	srv, _, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	setupChatOpsDB(t)
	now := time.Now()
	require.NoError(t, global.GlobalDB.DB.Create(&models.RSSNotificationLog{
		RSSID: 1, SiteName: "hdsky", TorrentID: "t1", NotifyKind: "new",
		NotificationConfID: 1, Result: "pending", NextRetryAt: &now,
	}).Error)
	require.NoError(t, global.GlobalDB.DB.Create(&models.RSSNotificationLog{
		RSSID: 2, SiteName: "mteam", TorrentID: "t2", NotifyKind: "new",
		NotificationConfID: 1, Result: "failed",
	}).Error)

	t.Run("list all", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodGet, "/api/chatops/rss-notifications", tok, nil)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var body struct {
			Items []models.RSSNotificationLog `json:"items"`
			Total int64                       `json:"total"`
		}
		decodeBody(t, resp, &body)
		assert.Equal(t, int64(2), body.Total)
	})

	t.Run("filter by result and rss_id", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodGet, "/api/chatops/rss-notifications?result=pending&rss_id=1&page=1&page_size=10", tok, nil)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var body struct {
			Total int64 `json:"total"`
		}
		decodeBody(t, resp, &body)
		assert.Equal(t, int64(1), body.Total)
	})
}

func TestRetryRSSNotification(t *testing.T) {
	srv, _, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	setupChatOpsDB(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.RSSNotificationLog{
		RSSID: 1, SiteName: "hdsky", TorrentID: "t1", NotifyKind: "new",
		NotificationConfID: 1, Result: "failed",
	}).Error)

	t.Run("retry existing row", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodPost, "/api/chatops/rss-notifications/1/retry", tok, nil)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var row models.RSSNotificationLog
		require.NoError(t, global.GlobalDB.DB.First(&row, 1).Error)
		assert.Equal(t, "pending", row.Result)
	})

	t.Run("retry missing row returns 404", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodPost, "/api/chatops/rss-notifications/999/retry", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

func TestCancelRSSNotification(t *testing.T) {
	srv, _, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")

	setupChatOpsDB(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.RSSNotificationLog{
		RSSID: 1, SiteName: "hdsky", TorrentID: "t1", NotifyKind: "new",
		NotificationConfID: 1, Result: "pending",
	}).Error)
	require.NoError(t, global.GlobalDB.DB.Create(&models.RSSNotificationLog{
		RSSID: 2, SiteName: "mteam", TorrentID: "t2", NotifyKind: "new",
		NotificationConfID: 1, Result: "sent",
	}).Error)

	t.Run("cancel pending row", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodPost, "/api/chatops/rss-notifications/1/cancel", tok, nil)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var row models.RSSNotificationLog
		require.NoError(t, global.GlobalDB.DB.First(&row, 1).Error)
		assert.Equal(t, "suppressed", row.Result)
	})

	t.Run("cancel non-retryable row returns 400", func(t *testing.T) {
		resp := chatopsReq(t, srv, http.MethodPost, "/api/chatops/rss-notifications/2/cancel", tok, nil)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}
