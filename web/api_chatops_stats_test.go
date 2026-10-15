package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// 近 7 天的投递按通道合计，RSS 与监控通知都算，来源分开；8 天前的不算
func TestNotifyStats(t *testing.T) {
	srv, _, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")
	setupChatOpsDB(t)
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(&models.MonitorNotificationLog{}))

	get := func() (int, NotifyStatsView, string) {
		resp := chatopsReq(t, srv, http.MethodGet, "/api/chatops/notifications/stats", tok, nil)
		if resp.StatusCode != http.StatusOK {
			defer resp.Body.Close()
			var e map[string]any
			decodeBody(t, resp, &e)
			msg, _ := e["detail"].(string)
			return resp.StatusCode, NotifyStatsView{}, msg
		}
		var view NotifyStatsView
		decodeBody(t, resp, &view)
		return resp.StatusCode, view, ""
	}

	code, view, _ := get()
	require.Equal(t, http.StatusOK, code)
	assert.NotNil(t, view.Channels, "没有记录时是空列表，不是 null")
	assert.Empty(t, view.Channels)

	now := time.Now()
	old := now.Add(-8 * 24 * time.Hour)
	rss := func(conf uint, tid, result string, at time.Time) {
		require.NoError(t, db.Create(&models.RSSNotificationLog{
			RSSID: 1, SiteName: "hdsky", TorrentID: tid, NotifyKind: "new",
			NotificationConfID: conf, Result: result, CreatedAt: at,
		}).Error)
	}
	mon := func(conf uint, src, key, result string, at time.Time) {
		require.NoError(t, db.Create(&models.MonitorNotificationLog{
			Source: src, Subject: "s", Kind: "k", EventKey: key,
			NotificationConfID: conf, Result: result, CreatedAt: at,
		}).Error)
	}
	rss(1, "t1", "sent", now)
	rss(1, "t2", "sent", now)
	rss(1, "t3", "failed", now)
	rss(1, "t4", "throttled", now)
	rss(1, "t5", "suppressed", now)
	rss(1, "t6", "sent", old)
	mon(1, "attendance", "d1", "sent", now)
	mon(1, "attendance", "d2", "pending", now)
	mon(2, "daily_report", "d1", "sent", now)
	mon(2, "login", "x", "failed", old)

	code, view, _ = get()
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, 7, view.Days)
	assert.WithinDuration(t, now.Add(-7*24*time.Hour), view.Since, time.Minute)
	require.Len(t, view.Channels, 2)
	assert.Equal(t, NotifyStatsChannel{
		ConfID: 1, Total: 7, Sent: 3, Failed: 1, Pending: 1, Skipped: 2,
		Sources: []NotifyStatsSource{{Source: "rss", Total: 5, Sent: 2}, {Source: "attendance", Total: 2, Sent: 1}},
	}, view.Channels[0])
	assert.Equal(t, NotifyStatsChannel{
		ConfID: 2, Total: 1, Sent: 1,
		Sources: []NotifyStatsSource{{Source: "daily_report", Total: 1, Sent: 1}},
	}, view.Channels[1])

	resp := chatopsReq(t, srv, http.MethodGet, "/api/chatops/notifications/stats", "", nil)
	resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "只认登录")

	require.NoError(t, db.Migrator().DropTable(&models.MonitorNotificationLog{}))
	code, _, msg := get()
	assert.Equal(t, http.StatusInternalServerError, code)
	assert.Equal(t, "读取监控通知日志失败", msg)
	require.NoError(t, db.Migrator().DropTable(&models.RSSNotificationLog{}))
	code, _, msg = get()
	assert.Equal(t, http.StatusInternalServerError, code)
	assert.Equal(t, "读取 RSS 通知日志失败", msg)

	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })
	w := httptest.NewRecorder()
	(&chatopsHandlers{deps: &ChatOpsDeps{}}).notifyStats(w, httptest.NewRequest(http.MethodGet, "/api/chatops/notifications/stats", nil))
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
