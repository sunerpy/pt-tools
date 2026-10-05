package web

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

type countingUserInfo struct{ calls atomic.Int32 }

func (c *countingUserInfo) FetchAndSave(context.Context, string) (v2.UserInfo, error) {
	c.calls.Add(1)
	return v2.UserInfo{Site: "hdsky", Username: "tester", LastAccess: 1700000000}, nil
}

// newProbeRequestServer 建一个带登录监控的服务，并按 Serve() 的方式注册 /api/sites/ 路由（含 s.auth）。
func newProbeRequestServer(t *testing.T) (*Server, *http.ServeMux, *scheduler.LoginReminderMonitor, *countingUserInfo) {
	t.Helper()
	writeWebTestSecretKey(t)
	srv := setupServer(t)
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(&models.SiteLoginState{}, &models.MigrationState{}))
	require.NoError(t, srv.store.SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, AutoStart: true,
	}))
	enabled := true
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("hdsky"), models.SiteConfig{
		Enabled: &enabled, AuthMethod: "cookie", Cookie: "seed=1",
	}))

	fetch := &countingUserInfo{}
	mon := scheduler.NewLoginReminderMonitor(scheduler.LoginReminderConfig{
		DB:       db,
		UserInfo: fetch,
		DefinitionLookup: func(name string) (*v2.SiteDefinition, bool) {
			return &v2.SiteDefinition{ID: name, Schema: v2.SchemaNexusPHP}, true
		},
	})
	srv.mgr.SetLoginReminderMonitor(mon)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/sites/", srv.auth(srv.apiSiteDetail))
	srv.sessions["sess-test"] = "admin"
	return srv, mux, mon, fetch
}

func serveAuthed(mux *http.ServeMux, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: "sess-test"})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// M1b：经真实路由更新凭证（含扩展同步）后请求一次探测，下一轮循环即探测该站。
func TestCredentialUpdateThroughMux_RequestsProbe(t *testing.T) {
	srv, mux, mon, fetch := newProbeRequestServer(t)

	w := serveAuthed(mux, http.MethodPut, "/api/sites/hdsky", `{"cookie":"uid=1; pass=2"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	settleCredentialReload(t, srv)

	var st models.SiteLoginState
	require.NoError(t, global.GlobalDB.DB.Where("site_name = ?", "hdsky").First(&st).Error)
	require.NotNil(t, st.ProbeRequestedAt, "the credential update asked for a probe")

	mon.RunDueProbes(context.Background())
	assert.Equal(t, int32(1), fetch.calls.Load())
	mon.RunDueProbes(context.Background())
	assert.Equal(t, int32(1), fetch.calls.Load(), "one request, one probe")
}

// M1b：经真实路由保存站点配置后同样请求一次探测。
func TestSiteConfigSaveThroughMux_RequestsProbe(t *testing.T) {
	srv, mux, mon, fetch := newProbeRequestServer(t)

	w := serveAuthed(mux, http.MethodPost, "/api/sites/hdsky", `{"enabled":true,"auth_method":"cookie","cookie":"uid=1; pass=3"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	settleCredentialReload(t, srv)

	var st models.SiteLoginState
	require.NoError(t, global.GlobalDB.DB.Where("site_name = ?", "hdsky").First(&st).Error)
	require.NotNil(t, st.ProbeRequestedAt, "saving the site config asked for a probe")

	mon.RunDueProbes(context.Background())
	assert.Equal(t, int32(1), fetch.calls.Load())
}
