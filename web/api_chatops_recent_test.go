package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/models"
)

// 最近的通知：RSS 通知与后台通知合在一起按时间倒序，标题取发出去的消息，结果按真实情况写
func TestNotifyRecent(t *testing.T) {
	srv, _, store, cleanup := newTestChatOpsServer(t)
	defer cleanup()
	tok := store.registerValidToken("chatops:*")
	setupChatOpsDB(t)
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(&models.MonitorNotificationLog{}))

	type body struct {
		Items []RecentNotification `json:"items"`
	}
	get := func(query string) (int, body, string) {
		resp := chatopsReq(t, srv, http.MethodGet, "/api/chatops/notifications/recent"+query, tok, nil)
		if resp.StatusCode != http.StatusOK {
			var e map[string]any
			decodeBody(t, resp, &e)
			msg, _ := e["detail"].(string)
			return resp.StatusCode, body{}, msg
		}
		var b body
		decodeBody(t, resp, &b)
		return resp.StatusCode, b, ""
	}

	code, b, _ := get("")
	require.Equal(t, http.StatusOK, code)
	assert.NotNil(t, b.Items, "没有记录时是空列表，不是 null")
	assert.Empty(t, b.Items)

	base := time.Now().Add(-time.Hour)
	at := func(min int) time.Time { return base.Add(time.Duration(min) * time.Minute) }
	rss := func(tid, result string, min int, delivered bool, payload string) {
		row := models.RSSNotificationLog{
			RSSID: 1, SiteName: "hdsky", TorrentID: tid, NotifyKind: "filtered",
			NotificationConfID: 1, Result: result, Attempts: 1, PayloadJSON: payload, CreatedAt: at(min),
		}
		if delivered {
			d := at(min)
			row.DeliveredAt = &d
		}
		require.NoError(t, db.Create(&row).Error)
	}
	mon := func(src, kind, result string, min int, payload string) {
		require.NoError(t, db.Create(&models.MonitorNotificationLog{
			Source: src, Subject: "s", Kind: kind, EventKey: kind + strings.Repeat("x", min),
			NotificationConfID: 2, Result: result, Attempts: 2, PayloadJSON: payload, CreatedAt: at(min),
		}).Error)
	}
	rss("t1", "sent", 1, true, `{"Title":"Some.Movie.2026","Text":"..."}`)
	rss("t2", "suppressed", 3, true, `{"Title":"点了忽略的"}`)
	rss("t3", "suppressed", 5, false, `{"Title":"取消的"}`)
	rss("t4", "throttled", 7, false, "")
	mon("attendance", "daily", "sent", 2, `{"title":"签到汇总","text":"..."}`)
	mon("login", "failure", "pending", 6, `{"title":"`+strings.Repeat("长", 250)+`"}`)

	code, b, _ = get("")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, b.Items, 6)
	type brief struct{ src, title, result string }
	var got []brief
	for _, it := range b.Items {
		got = append(got, brief{it.Source, it.Title, it.Result})
	}
	assert.Equal(t, []brief{
		{"rss", "", "throttled"},
		{"login", strings.Repeat("长", 200) + "…", "pending"},
		{"rss", "取消的", "canceled"},
		{"rss", "点了忽略的", "sent"},
		{"attendance", "签到汇总", "sent"},
		{"rss", "Some.Movie.2026", "sent"},
	}, got)
	first := b.Items[0]
	assert.Equal(t, "hdsky", first.Site)
	assert.Equal(t, "t4", first.TorrentID)
	assert.Equal(t, "filtered", first.Kind)
	assert.Equal(t, uint(1), first.ConfID)
	assert.Equal(t, "failure", b.Items[1].Kind)
	assert.Equal(t, 2, b.Items[1].Attempts)
	assert.Empty(t, b.Items[1].Site)
	assert.Empty(t, b.Items[1].Error)

	// 失败过的带上原因（遮掉凭证）：每日战报为什么没收到，在这张小表里就看得到
	require.NoError(t, db.Create(&models.MonitorNotificationLog{
		Source: "daily_report", Subject: "*", Kind: "daily", EventKey: "2026-10-28", NotificationConfID: 2,
		Result: "pending", Attempts: 1, PayloadJSON: `{"title":"每日战报"}`, CreatedAt: at(9),
		LastError: `Post "https://api.telegram.org/bot1234567890:AAH_kx-Yz9Q8wE7rT6yU5iO4pA3sD2fG1hJ/sendMessage": i/o timeout`,
	}).Error)
	code, b, _ = get("?limit=1")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, b.Items, 1)
	assert.Equal(t, "daily_report", b.Items[0].Source)
	assert.Contains(t, b.Items[0].Error, "i/o timeout")
	assert.NotContains(t, b.Items[0].Error, "AAH_kx")
	require.NoError(t, db.Where("source = ?", "daily_report").Delete(&models.MonitorNotificationLog{}).Error)

	code, b, _ = get("?limit=2")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, b.Items, 2)
	assert.Equal(t, "throttled", b.Items[0].Result)

	for _, bad := range []string{"?limit=0", "?limit=51", "?limit=x"} {
		badCode, _, msg := get(bad)
		assert.Equal(t, http.StatusBadRequest, badCode, bad)
		assert.Equal(t, "limit 要在 1 到 50 之间", msg)
	}

	require.NoError(t, db.Migrator().DropTable(&models.MonitorNotificationLog{}))
	code, _, msg := get("")
	assert.Equal(t, http.StatusInternalServerError, code)
	assert.Equal(t, "读取监控通知日志失败", msg)
	require.NoError(t, db.Migrator().DropTable(&models.RSSNotificationLog{}))
	code, _, msg = get("")
	assert.Equal(t, http.StatusInternalServerError, code)
	assert.Equal(t, "读取 RSS 通知日志失败", msg)

	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })
	w := httptest.NewRecorder()
	(&chatopsHandlers{deps: &ChatOpsDeps{}}).notifyRecent(w, httptest.NewRequest(http.MethodGet, "/api/chatops/notifications/recent", nil))
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestPayloadTitle(t *testing.T) {
	assert.Empty(t, payloadTitle(""))
	assert.Empty(t, payloadTitle("not json"))
	assert.Equal(t, "a", payloadTitle(`{"Title":"a"}`))
	assert.Equal(t, "b", payloadTitle(`{"title":"b"}`))
}

// 和投递统计一样挂在生产的 s.auth 后面：只认网页登录
func TestNotifyRecent_SessionOnly(t *testing.T) {
	e := newAppEnv(t)
	e.srv.chatopsDeps = &ChatOpsDeps{}
	e.handler = e.srv.buildHandler()
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.RSSNotificationLog{}, &models.MonitorNotificationLog{}))
	all := e.token(apitoken.Scopes...)
	w := e.do(appReq{method: http.MethodGet, path: "/api/chatops/notifications/recent", bearer: all})
	assert.Contains(t, []int{http.StatusUnauthorized, http.StatusFound}, w.Code, "令牌访问回了 %d", w.Code)
	w = e.do(appReq{method: http.MethodGet, path: "/api/chatops/notifications/recent", session: true})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"items":[]}`, w.Body.String())
}
