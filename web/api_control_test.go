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
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/models"
)

// seedRSSTaskConfig 写一份全局设置和一个已启用站点下的一条 RSS 订阅。
func seedRSSTaskConfig(t *testing.T, autoStart bool) {
	t.Helper()
	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SettingsGlobal{DownloadDir: "downloads", AutoStart: autoStart}).Error)
	site := models.SiteSetting{Name: "hdsky", Enabled: true}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Create(&models.RSSSubscription{SiteID: site.ID, Name: "free", URL: "https://hdsky.me/torrentrss.php", IntervalMinutes: 10}).Error)
}

func getRSSTaskStatus(t *testing.T, srv *Server) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	srv.apiControlStatus(w, httptest.NewRequest(http.MethodGet, "/api/control/status", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var out map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return out
}

func postAutoStart(srv *Server, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	srv.apiControlAutoStart(w, httptest.NewRequest(http.MethodPost, "/api/control/auto-start", bytes.NewBufferString(body)))
	return w
}

// 程序启动后没开自动启动、也没点过启动：状态是「未启动」，并报出有几条订阅等着运行。
func TestControlStatus_ManualWhenNotStarted(t *testing.T) {
	srv := setupServer(t)
	seedRSSTaskConfig(t, false)

	st := getRSSTaskStatus(t, srv)
	assert.Equal(t, "manual", st["state"])
	assert.EqualValues(t, 0, st["jobs"])
	assert.EqualValues(t, 1, st["subscriptions"])
	assert.Equal(t, false, st["auto_start"])
	assert.NotContains(t, st, "started_at", "没有任务在运行时不给启动时间")
}

func TestControlStatus_RunningAndPaused(t *testing.T) {
	srv := setupServer(t)
	seedRSSTaskConfig(t, false)
	srv.mgr.Start(models.SiteGroup("hdsky"), models.RSSConfig{Name: "free"}, func(ctx context.Context) { <-ctx.Done() })

	st := getRSSTaskStatus(t, srv)
	assert.Equal(t, "running", st["state"])
	assert.EqualValues(t, 1, st["jobs"])
	startedAt, ok := st["started_at"].(string)
	require.True(t, ok, "运行中要给出启动时间")
	_, err := time.Parse(time.RFC3339Nano, startedAt)
	assert.NoError(t, err)

	w := httptest.NewRecorder()
	srv.apiStopAll(w, httptest.NewRequest(http.MethodPost, "/api/control/stop", nil))
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "paused", getRSSTaskStatus(t, srv)["state"])
}

func TestControlStatus_MethodNotAllowed(t *testing.T) {
	srv := setupServer(t)
	w := httptest.NewRecorder()
	srv.apiControlStatus(w, httptest.NewRequest(http.MethodPost, "/api/control/status", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestControlStatus_NoManager(t *testing.T) {
	srv := setupServer(t)
	srv.mgr = nil
	w := httptest.NewRecorder()
	srv.apiControlStatus(w, httptest.NewRequest(http.MethodGet, "/api/control/status", nil))
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// 打开自动启动只改这一项，并像在全局设置里保存一样触发重载：这里没有默认下载器，重载做完后状态报出原因。
func TestControlAutoStart_EnableTriggersReload(t *testing.T) {
	srv := setupServer(t)
	seedRSSTaskConfig(t, false)
	require.Equal(t, "manual", getRSSTaskStatus(t, srv)["state"])

	w := postAutoStart(srv, `{"enabled":true}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var st map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &st))
	assert.Equal(t, true, st["auto_start"])
	assert.Contains(t, []any{"starting", "no_downloader"}, st["state"], "打开以后要么在启动、要么已经报出没启动的原因，不能还是「未启动」")

	gs, err := srv.store.GetGlobalSettings()
	require.NoError(t, err)
	assert.True(t, gs.AutoStart)
	assert.Equal(t, "downloads", gs.DownloadDir, "其余全局设置不动")

	require.Eventually(t, func() bool {
		return getRSSTaskStatus(t, srv)["state"] == "no_downloader"
	}, 10*time.Second, 20*time.Millisecond, "配置变更引起的重载应报出没有默认下载器")

	w = postAutoStart(srv, `{"enabled":false}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	gs, err = srv.store.GetGlobalSettings()
	require.NoError(t, err)
	assert.False(t, gs.AutoStart)
}

func TestControlAutoStart_BadRequests(t *testing.T) {
	srv := setupServer(t)
	seedRSSTaskConfig(t, false)
	for _, body := range []string{``, `not-json`, `{}`, `{"enabled":"yes"}`} {
		assert.Equal(t, http.StatusBadRequest, postAutoStart(srv, body).Code, "body=%q", body)
	}
	w := httptest.NewRecorder()
	srv.apiControlAutoStart(w, httptest.NewRequest(http.MethodGet, "/api/control/auto-start", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	gs, err := srv.store.GetGlobalSettings()
	require.NoError(t, err)
	assert.False(t, gs.AutoStart, "请求不合格时不改设置")
}

// 两个新接口和启停一样挂在 s.auth 后面：只认网页登录，API 令牌（权限范围全有）也进不来。
func TestControlRoutes_SessionOnly(t *testing.T) {
	e := newAppEnv(t)
	seedRSSTaskConfig(t, false)
	all := e.token(apitoken.Scopes...)
	for _, q := range []appReq{
		{method: http.MethodGet, path: "/api/control/status"},
		{method: http.MethodPost, path: "/api/control/auto-start", body: `{"enabled":true}`},
	} {
		q.bearer = all
		w := e.do(q)
		assert.Contains(t, []int{http.StatusUnauthorized, http.StatusFound}, w.Code, "%s 令牌访问回了 %d", q.path, w.Code)
	}
	gs, err := e.srv.store.GetGlobalSettings()
	require.NoError(t, err)
	assert.False(t, gs.AutoStart, "令牌请求不能改设置")

	w := e.do(appReq{method: http.MethodGet, path: "/api/control/status", session: true})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"state":"manual"`)
}
