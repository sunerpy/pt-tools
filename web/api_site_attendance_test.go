package web

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

type stubAttendSite struct {
	result v2.AttendResult
	err    error
}

func (s stubAttendSite) SupportsAttendance() bool { return true }

func (s stubAttendSite) Attend(context.Context) (v2.AttendResult, error) { return s.result, s.err }

// newAttendanceServer 建一个带签到监控的服务，并按 Serve() 的方式注册签到相关路由（含 s.auth）。
func newAttendanceServer(t *testing.T, withMonitor bool) (*Server, *http.ServeMux, *scheduler.LoginReminderMonitor) {
	t.Helper()
	writeWebTestSecretKey(t)
	srv := setupServer(t)
	// 这些接口会发布 ConfigChanged；先停掉管理器，它的事件协程收到事件就退出、不再 reload，
	// 否则延迟 200ms 的 reload 会读 GlobalDB，与后续测试改写 GlobalDB 竞态。
	srv.mgr.StopAll()
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(&models.SiteLoginState{}, &models.SiteAttendanceLog{}, &models.MigrationState{}))
	require.NoError(t, srv.store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10}))
	for _, s := range []models.SiteSetting{
		{Name: "hdtime", Enabled: true, AuthMethod: "cookie", Cookie: "c=1", IsBuiltin: true},
		{Name: "hdsky", Enabled: true, AuthMethod: "cookie", Cookie: "c=1", IsBuiltin: true},
		{Name: "qa-dynamic", Enabled: true, AuthMethod: "cookie", Cookie: "c=1"},
	} {
		require.NoError(t, db.Create(&s).Error)
	}

	probe := scheduler.NewLoginReminderMonitor(scheduler.LoginReminderConfig{DB: db})
	if withMonitor {
		att := scheduler.NewAttendanceMonitor(scheduler.AttendanceMonitorConfig{
			DB: db,
			Sites: scheduler.AttendanceSitesFunc(func(name string) (v2.AttendanceCapable, bool) {
				if name == "hdtime" {
					return stubAttendSite{result: v2.AttendResult{Status: v2.AttendSigned, Message: "这是您的第 9 次签到"}}, true
				}
				return nil, false
			}),
			Window: srv.store,
			Locker: probe,
		})
		srv.mgr.SetAttendanceMonitor(att)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/sites/", srv.auth(srv.apiSiteDetail))
	srv.registerAttendanceRoutes(mux)
	srv.sessions["sess-test"] = "admin"
	return srv, mux, probe
}

func decodeAttendance(t *testing.T, body []byte) map[string]SiteAttendanceResponse {
	t.Helper()
	var list []SiteAttendanceResponse
	require.NoError(t, json.Unmarshal(body, &list))
	out := map[string]SiteAttendanceResponse{}
	for _, item := range list {
		out[item.SiteName] = item
	}
	return out
}

// M1c：签到列表给出每个站点能否签到（不支持时带原因）、开关和当天的结果。
func TestAttendanceList_ReportsSupportAndToday(t *testing.T) {
	_, mux, _ := newAttendanceServer(t, true)
	day := time.Now().Format("2006-01-02")
	require.NoError(t, models.NewSiteAttendanceRepository(global.GlobalDB.DB).EnsureDay(models.SiteAttendanceLog{
		SiteName: "hdtime", Day: day, Status: models.AttendanceSigned, Message: "这是您的第 8 次签到", ScheduledAt: time.Now().UTC(),
	}))

	w := serveAuthed(mux, http.MethodGet, "/api/sites/attendance", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	items := decodeAttendance(t, w.Body.Bytes())

	assert.True(t, items["hdtime"].Supported)
	assert.Equal(t, models.AttendanceSigned, items["hdtime"].Status)
	assert.Equal(t, day, items["hdtime"].Day)
	assert.Contains(t, items["hdtime"].Message, "第 8 次签到")
	assert.False(t, items["hdsky"].Supported)
	assert.Contains(t, items["hdsky"].UnsupportedReason, "验证码")
	assert.Empty(t, items["hdsky"].Status, "no record today")
	assert.False(t, items["qa-dynamic"].Supported)
	assert.Contains(t, items["qa-dynamic"].UnsupportedReason, "内置定义")
}

// M1c：开关只写签到这一列；不支持签到的站点不能打开，并说明原因。
func TestAttendanceToggle(t *testing.T) {
	_, mux, _ := newAttendanceServer(t, true)
	db := global.GlobalDB.DB
	var row models.SiteSetting

	w := serveAuthed(mux, http.MethodPut, "/api/sites/hdtime/attendance", `{"enabled":true}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, db.Where("name = ?", "hdtime").First(&row).Error)
	assert.True(t, row.AttendanceEnabled)

	w = serveAuthed(mux, http.MethodPut, "/api/sites/hdsky/attendance", `{"enabled":true}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "验证码")

	w = serveAuthed(mux, http.MethodPut, "/api/sites/hdtime/attendance", `{"enabled":false}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, db.Where("name = ?", "hdtime").First(&row).Error)
	assert.False(t, row.AttendanceEnabled)

	w = serveAuthed(mux, http.MethodPut, "/api/sites/hdtime/attendance", `not json`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// M1c：手动签到经签到监控执行；探测占用该站时返回 409；监控未接线时返回 503。
func TestAttendanceSignNow(t *testing.T) {
	_, mux, probe := newAttendanceServer(t, true)

	w := serveAuthed(mux, http.MethodPost, "/api/sites/hdtime/attendance", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var item SiteAttendanceResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &item))
	assert.Equal(t, models.AttendanceSigned, item.Status)
	assert.Contains(t, item.Message, "第 9 次签到")

	release, ok := probe.TryAcquireProbeLock("hdtime")
	require.True(t, ok)
	w = serveAuthed(mux, http.MethodPost, "/api/sites/hdtime/attendance", "")
	assert.Equal(t, http.StatusConflict, w.Code)
	release()

	_, bare, _ := newAttendanceServer(t, false)
	w = serveAuthed(bare, http.MethodPost, "/api/sites/hdtime/attendance", "")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// M1c：签到时间窗默认 08:00–10:00，校验 HH:MM 且开始早于结束。
func TestAttendanceSettings(t *testing.T) {
	_, mux, _ := newAttendanceServer(t, true)

	w := serveAuthed(mux, http.MethodGet, "/api/sites/attendance/settings", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got AttendanceSettingsDTO
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, AttendanceSettingsDTO{WindowStart: "08:00", WindowEnd: "10:00"}, got)

	w = serveAuthed(mux, http.MethodPut, "/api/sites/attendance/settings", `{"window_start":"07:00","window_end":"09:30"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = serveAuthed(mux, http.MethodGet, "/api/sites/attendance/settings", "")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, AttendanceSettingsDTO{WindowStart: "07:00", WindowEnd: "09:30"}, got)

	w = serveAuthed(mux, http.MethodPut, "/api/sites/attendance/settings", `{"window_start":"10:00","window_end":"09:00"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = serveAuthed(mux, http.MethodDelete, "/api/sites/attendance/settings", "")
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// 错误路径：方法不对、站点不存在、数据库未初始化、存储失败都给出明确的状态码。
func TestAttendanceAPI_ErrorPaths(t *testing.T) {
	_, mux, _ := newAttendanceServer(t, false)

	w := serveAuthed(mux, http.MethodGet, "/api/sites/attendance", "")
	require.Equal(t, http.StatusOK, w.Code, "the list works without the monitor")
	assert.Equal(t, time.Now().Format("2006-01-02"), decodeAttendance(t, w.Body.Bytes())["hdtime"].Day)

	assert.Equal(t, http.StatusMethodNotAllowed, serveAuthed(mux, http.MethodPost, "/api/sites/attendance", "").Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(mux, http.MethodPost, "/api/sites/nope/attendance", "").Code)
	assert.Equal(t, http.StatusMethodNotAllowed, serveAuthed(mux, http.MethodPatch, "/api/sites/hdtime/attendance", "").Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPut, "/api/sites/attendance/settings", "{").Code)

	db := global.GlobalDB.DB
	require.NoError(t, db.Migrator().DropTable(&models.SiteAttendanceLog{}))
	assert.Equal(t, http.StatusInternalServerError, serveAuthed(mux, http.MethodGet, "/api/sites/attendance", "").Code)

	require.NoError(t, db.Migrator().DropTable(&models.SettingsGlobal{}))
	assert.Equal(t, http.StatusInternalServerError, serveAuthed(mux, http.MethodGet, "/api/sites/attendance/settings", "").Code)
	assert.Equal(t, http.StatusInternalServerError, serveAuthed(mux, http.MethodPut, "/api/sites/attendance/settings", `{"window_start":"07:00","window_end":"09:00"}`).Code)

	require.NoError(t, db.Migrator().DropTable(&models.SiteSetting{}))
	assert.Equal(t, http.StatusInternalServerError, serveAuthed(mux, http.MethodGet, "/api/sites/attendance", "").Code)

	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })
	assert.Equal(t, http.StatusServiceUnavailable, serveAuthed(mux, http.MethodGet, "/api/sites/attendance", "").Code)
	assert.Equal(t, http.StatusServiceUnavailable, serveAuthed(mux, http.MethodPost, "/api/sites/hdtime/attendance", "").Code)
}

// 签到监控内部出错（签到记录表不可用）时手动签到返回 500；开关写库失败也返回 500。
func TestAttendanceAPI_StoreFailures(t *testing.T) {
	_, mux, _ := newAttendanceServer(t, true)
	db := global.GlobalDB.DB
	require.NoError(t, db.Migrator().DropTable(&models.SiteAttendanceLog{}))
	assert.Equal(t, http.StatusInternalServerError, serveAuthed(mux, http.MethodPost, "/api/sites/hdtime/attendance", "").Code)

	require.NoError(t, db.Exec("CREATE TRIGGER block_attendance BEFORE UPDATE OF attendance_enabled ON site_settings BEGIN SELECT RAISE(ABORT, 'read only'); END;").Error)
	assert.Equal(t, http.StatusInternalServerError, serveAuthed(mux, http.MethodPut, "/api/sites/hdtime/attendance", `{"enabled":true}`).Code)
	assert.Nil(t, (&Server{}).attendanceMonitor())
}
