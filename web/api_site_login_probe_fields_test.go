package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// M1a：登录状态接口返回下次探测、失败开始、最近成功、访问未生效与判定来源。
func TestApiSiteLoginStateList_ExposesScheduleAndSource(t *testing.T) {
	srv := newLoginMonitorServer(t)
	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", Enabled: true}).Error)
	now := time.Now().UTC().Truncate(time.Second)
	next := now.Add(3 * time.Hour)
	since := now.Add(-26 * time.Hour)
	success := now.Add(-30 * time.Hour)
	stale := now.Add(-2 * time.Hour)
	access := now.Add(-20 * 24 * time.Hour)
	visit := now.Add(-time.Hour)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName: "hdsky", BanThresholdDays: 30, RemindBeforeDays: 10, ReminderCron: "0 10,22 * * *",
		LastReminderTier: "none", ProbeMode: "auto", LastProbeStatus: "SESSION_EXPIRED",
		NextProbeAt: &next, FirstFailureAt: &since, LastSuccessAt: &success, AccessStaleSince: &stale,
		LastAccessAt: &access, LastVisitAt: &visit,
	}).Error)

	mux := http.NewServeMux()
	srv.registerLoginStateRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/sites/login-state", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "sess-test"})
	srv.sessions.put("sess-test", "admin")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var out []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Len(t, out, 1)
	row := out[0]
	assert.EqualValues(t, next.Unix(), row["next_probe_at"])
	assert.EqualValues(t, since.Unix(), row["first_failure_at"])
	assert.EqualValues(t, success.Unix(), row["last_success_at"])
	assert.EqualValues(t, stale.Unix(), row["access_stale_since"])
	assert.Equal(t, "last_visit", row["effective_source"], "a failing probe lets the newer browser visit decide")
	assert.EqualValues(t, visit.Unix(), row["effective_last_active_at"])
	for _, secret := range []string{"cookie", "cookie_encrypted", "api_key", "passkey"} {
		assert.NotContains(t, row, secret)
	}
}

func TestBuildLoginStateResponse_EffectiveSourceNoneWithoutData(t *testing.T) {
	resp := buildLoginStateResponse(models.SiteSetting{Name: "hdsky"}, models.SiteLoginState{SiteName: "hdsky"}, time.Now())
	assert.Equal(t, "none", resp.EffectiveSource)
	assert.Nil(t, resp.NextProbeAt)
	assert.Nil(t, resp.FirstFailureAt)
}

// M1a 问题 4：扩展随凭证同步上报的 last_visit_at 只前进不后退，非法值被忽略，扩展实际发送的毫秒格式也能解析。
func TestRecordExtensionVisit_OnlyMovesForward(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, db.DB.AutoMigrate(&models.SiteLoginState{}))
	prev := global.GlobalDB
	global.GlobalDB = db
	t.Cleanup(func() { global.GlobalDB = prev })

	lastVisit := func() *time.Time {
		var st models.SiteLoginState
		require.NoError(t, db.DB.Where("site_name = ?", "hdsky").First(&st).Error)
		return st.LastVisitAt
	}

	visit := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	recordExtensionVisit("hdsky", visit.Format(time.RFC3339))
	require.NotNil(t, lastVisit(), "the row is created on first report")
	assert.True(t, lastVisit().Equal(visit))

	recordExtensionVisit("hdsky", visit.Add(-24*time.Hour).Format(time.RFC3339))
	assert.True(t, lastVisit().Equal(visit), "an older visit never moves last_visit_at backwards")

	recordExtensionVisit("hdsky", "not-a-time")
	assert.True(t, lastVisit().Equal(visit), "an unparsable value is ignored")

	// 扩展存的是 Date.toISOString() 的结果，带毫秒。
	newer := visit.Add(30*time.Minute + 123*time.Millisecond)
	recordExtensionVisit("hdsky", newer.Format("2006-01-02T15:04:05.000Z"))
	assert.True(t, lastVisit().Equal(newer), "the extension's millisecond ISO format is accepted")
}

// 经 updateSiteCredential 的请求体读取 last_visit_at；值无法解析时凭证同步照常成功。
// 每个用例只发一次请求，并用 settleCredentialReload 等异步 reload 完成，避免与后续测试改写 GlobalDB 竞态。
func TestUpdateSiteCredential_RecordsLastVisit(t *testing.T) {
	visit := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	cases := []struct {
		name string
		body string
		want *time.Time
	}{
		{"valid visit is recorded", `{"cookie":"uid=1; pass=2","last_visit_at":"` + visit.Format(time.RFC3339) + `"}`, &visit},
		{"invalid visit does not fail the sync", `{"cookie":"uid=1; pass=2","last_visit_at":"not-a-time"}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writeWebTestSecretKey(t)
			srv := setupServer(t)
			db := global.GlobalDB.DB
			require.NoError(t, db.AutoMigrate(&models.SiteLoginState{}))
			require.NoError(t, srv.store.SaveGlobalSettings(models.SettingsGlobal{
				DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, AutoStart: true,
			}))
			enabled := true
			require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("hdsky"), models.SiteConfig{
				Enabled: &enabled, AuthMethod: "cookie", Cookie: "seed=1",
			}))

			w := httptest.NewRecorder()
			srv.updateSiteCredential(w, httptest.NewRequest(http.MethodPut, "/api/sites/hdsky", bytes.NewBufferString(tc.body)), models.SiteGroup("hdsky"))
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			settleCredentialReload(t, srv)

			var st models.SiteLoginState
			err := db.Where("site_name = ?", "hdsky").First(&st).Error
			if tc.want == nil {
				if err == nil {
					assert.Nil(t, st.LastVisitAt)
				}
				return
			}
			require.NoError(t, err)
			require.NotNil(t, st.LastVisitAt)
			assert.True(t, st.LastVisitAt.Equal(*tc.want))
		})
	}
}
