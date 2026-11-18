package web

import (
	"context"
	"encoding/json"
	"fmt"
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
	require.NoError(t, db.AutoMigrate(&models.SiteLoginState{}, &models.SiteAttendanceLog{}, &models.SiteAttendanceEvent{}, &models.MigrationState{}))
	require.NoError(t, srv.store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10}))
	for _, s := range []models.SiteSetting{
		{Name: "hdtime", Enabled: true, AuthMethod: "cookie", Cookie: "c=1", IsBuiltin: true},
		{Name: "hdsky", Enabled: true, AuthMethod: "cookie", Cookie: "c=1", IsBuiltin: true},
		{Name: "u2dmhy", Enabled: true, AuthMethod: "cookie", Cookie: "c=1", IsBuiltin: true},
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
	srv.sessions.put("sess-test", "admin")
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
	// HDSky 签到要识别验证码：有 AI 时能签，列表里写明要过哪一关
	assert.True(t, items["hdsky"].Supported)
	assert.Equal(t, "hdsky_captcha", items["hdsky"].Challenge)
	assert.Empty(t, items["hdsky"].Status, "no record today")
	assert.Empty(t, items["hdtime"].Challenge)
	assert.False(t, items["u2dmhy"].Supported)
	assert.Contains(t, items["u2dmhy"].UnsupportedReason, "暂不支持")
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

	w = serveAuthed(mux, http.MethodPut, "/api/sites/u2dmhy/attendance", `{"enabled":true}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "暂不支持")

	// 要过验证码的站点可以打开（签到时要 AI）
	w = serveAuthed(mux, http.MethodPut, "/api/sites/hdsky/attendance", `{"enabled":true}`)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

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
	assert.Equal(t, AttendanceSettingsDTO{WindowStart: "08:00", WindowEnd: "10:00", NotifyEnabled: true, NotifyChannelIDs: []uint{}}, got)

	w = serveAuthed(mux, http.MethodPut, "/api/sites/attendance/settings", `{"window_start":"07:00","window_end":"09:30"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = serveAuthed(mux, http.MethodGet, "/api/sites/attendance/settings", "")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, AttendanceSettingsDTO{WindowStart: "07:00", WindowEnd: "09:30", NotifyEnabled: true, NotifyChannelIDs: []uint{}}, got)

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

// AI 签到的开关和看图模型跟时间窗一起读写；只发时间窗（旧版前端）时不改 AI 两项；模型名不能有空白。
func TestAttendanceSettings_AI(t *testing.T) {
	_, mux, _ := newAttendanceServer(t, true)

	w := serveAuthed(mux, http.MethodPut, "/api/sites/attendance/settings",
		`{"window_start":"08:00","window_end":"10:00","ai_enabled":true,"vision_model":" gemini-x "}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got AttendanceSettingsDTO
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.True(t, got.AIEnabled)
	assert.Equal(t, "gemini-x", got.VisionModel)
	assert.False(t, got.AssistantReady, "没有 AI 助手服务")

	w = serveAuthed(mux, http.MethodPut, "/api/sites/attendance/settings", `{"window_start":"07:00","window_end":"09:00"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = serveAuthed(mux, http.MethodGet, "/api/sites/attendance/settings", "")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "07:00", got.WindowStart)
	assert.True(t, got.AIEnabled, "只发时间窗时不改 AI 签到")
	assert.Equal(t, "gemini-x", got.VisionModel)

	w = serveAuthed(mux, http.MethodPut, "/api/sites/attendance/settings",
		`{"window_start":"07:00","window_end":"09:00","vision_model":"bad model"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w = serveAuthed(mux, http.MethodPut, "/api/sites/attendance/settings", `{"window_start":"07:00","window_end":"09:00","ai_enabled":false}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.False(t, got.AIEnabled)
	assert.Equal(t, "gemini-x", got.VisionModel, "只关开关时模型保留")

	// 没有 AI 助手服务时「测试看图」回 503
	assert.Equal(t, http.StatusServiceUnavailable, serveAuthed(mux, http.MethodPost, "/api/sites/attendance/ai-test", `{"vision_model":""}`).Code)
}

// 签到结果推送：默认开、所有通道；可以关掉或只选几个通道；选了不存在的通道回 400；旧版前端只发时间窗时不改推送。
func TestAttendanceSettings_Notify(t *testing.T) {
	_, mux, _ := newAttendanceServer(t, true)
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.NotificationConf{}))
	conf := models.NotificationConf{ChannelType: "telegram", Name: "tg", Enabled: true}
	require.NoError(t, global.GlobalDB.DB.Create(&conf).Error)
	get := func() AttendanceSettingsDTO {
		t.Helper()
		w := serveAuthed(mux, http.MethodGet, "/api/sites/attendance/settings", "")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var got AttendanceSettingsDTO
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		return got
	}
	got := get()
	assert.True(t, got.NotifyEnabled, "默认推送（以前一直推给所有启用的通道）")
	assert.Empty(t, got.NotifyChannelIDs)

	body := fmt.Sprintf(`{"window_start":"08:00","window_end":"10:00","notify_enabled":true,"notify_channel_ids":[%d]}`, conf.ID)
	w := serveAuthed(mux, http.MethodPut, "/api/sites/attendance/settings", body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, []uint{conf.ID}, get().NotifyChannelIDs)

	w = serveAuthed(mux, http.MethodPut, "/api/sites/attendance/settings", `{"window_start":"08:00","window_end":"10:00"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	got = get()
	assert.True(t, got.NotifyEnabled, "旧版前端只发时间窗：推送不变")
	assert.Equal(t, []uint{conf.ID}, got.NotifyChannelIDs)

	w = serveAuthed(mux, http.MethodPut, "/api/sites/attendance/settings", `{"window_start":"08:00","window_end":"10:00","notify_enabled":false}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.False(t, get().NotifyEnabled)

	w = serveAuthed(mux, http.MethodPut, "/api/sites/attendance/settings", `{"window_start":"08:00","window_end":"10:00","notify_channel_ids":[9999]}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "不存在")
}

type blockingAttendSite struct{ release chan struct{} }

func (s blockingAttendSite) SupportsAttendance() bool { return true }

func (s blockingAttendSite) Attend(ctx context.Context) (v2.AttendResult, error) {
	select {
	case <-s.release:
		return v2.AttendResult{Status: v2.AttendSigned, Message: "签到成功"}, nil
	case <-ctx.Done():
		return v2.AttendResult{}, ctx.Err()
	}
}

// 一键签到：POST 马上回 202，在后台签开启了自动签到的站点；跑着时再 POST 回 409；GET 看进度，做完后列表里有结果。
func TestAttendanceRun(t *testing.T) {
	srv, mux, probe := newAttendanceServer(t, false)
	db := global.GlobalDB.DB
	require.NoError(t, db.Model(&models.SiteSetting{}).Where("name = ?", "hdtime").Update("attendance_enabled", true).Error)
	site := blockingAttendSite{release: make(chan struct{})}
	srv.mgr.SetAttendanceMonitor(scheduler.NewAttendanceMonitor(scheduler.AttendanceMonitorConfig{
		DB:     db,
		Sites:  scheduler.AttendanceSitesFunc(func(string) (v2.AttendanceCapable, bool) { return site, true }),
		Window: srv.store,
		Locker: probe,
	}))

	w := serveAuthed(mux, http.MethodPost, "/api/sites/attendance/run", "")
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	var st AttendanceRunResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &st))
	assert.True(t, st.Running)

	w = serveAuthed(mux, http.MethodPost, "/api/sites/attendance/run", "")
	assert.Equal(t, http.StatusConflict, w.Code, "同一时间只跑一轮")

	close(site.release)
	require.Eventually(t, func() bool {
		rw := serveAuthed(mux, http.MethodGet, "/api/sites/attendance/run", "")
		var cur AttendanceRunResponse
		_ = json.Unmarshal(rw.Body.Bytes(), &cur)
		return !cur.Running && cur.FinishedAt != nil && cur.Done == 1 && cur.Total == 1
	}, 5*time.Second, 10*time.Millisecond)

	w = serveAuthed(mux, http.MethodGet, "/api/sites/attendance", "")
	items := decodeAttendance(t, w.Body.Bytes())
	assert.Equal(t, models.AttendanceSigned, items["hdtime"].Status)
	assert.Empty(t, items["hdsky"].Status, "没开自动签到的站点一键签到不签")

	_, bare, _ := newAttendanceServer(t, false)
	w = serveAuthed(bare, http.MethodPost, "/api/sites/attendance/run", "")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// 签到记录：最近 days 天（默认 14，最多 60），今天在前；更早的不返回；days 不合法回 400。
func TestAttendanceHistory(t *testing.T) {
	_, mux, _ := newAttendanceServer(t, true)
	repo := models.NewSiteAttendanceRepository(global.GlobalDB.DB)
	now := time.Now()
	for _, r := range []struct {
		site   string
		ago    int
		status string
	}{{"hdtime", 0, models.AttendanceSigned}, {"hdtime", 3, models.AttendanceFailed}, {"hdsky", 3, models.AttendanceAlready}, {"hdtime", 20, models.AttendanceSigned}} {
		require.NoError(t, repo.EnsureDay(models.SiteAttendanceLog{
			SiteName: r.site, Day: now.AddDate(0, 0, -r.ago).Format("2006-01-02"), Status: r.status, ScheduledAt: now.UTC(),
		}))
	}

	w := serveAuthed(mux, http.MethodGet, "/api/sites/attendance/history?days=7", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got AttendanceHistoryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got.Days, 7)
	assert.Equal(t, now.Format("2006-01-02"), got.Days[0], "今天在前")
	require.Len(t, got.Rows, 3, "20 天前的不在 7 天里")
	assert.Equal(t, now.Format("2006-01-02"), got.Rows[0].Day)

	w = serveAuthed(mux, http.MethodGet, "/api/sites/attendance/history", "")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Len(t, got.Days, 14)
	assert.Len(t, got.Rows, 3)

	w = serveAuthed(mux, http.MethodGet, "/api/sites/attendance/history?days=999", "")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Len(t, got.Days, 60)
	assert.Len(t, got.Rows, 4)

	w = serveAuthed(mux, http.MethodGet, "/api/sites/attendance/history?days=abc", "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w = serveAuthed(mux, http.MethodPost, "/api/sites/attendance/history", "")
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// 签到详情：某天每个站点的结果、直接签还是借助 AI、AI 调用与 token，以及每次尝试；不给日期时是今天。
func TestAttendanceEvents(t *testing.T) {
	_, mux, _ := newAttendanceServer(t, true)
	repo := models.NewSiteAttendanceRepository(global.GlobalDB.DB)
	day := "2026-10-18"
	at := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	require.NoError(t, repo.EnsureDay(models.SiteAttendanceLog{SiteName: "hdtime", Day: day, Status: models.AttendanceSigned, Attempts: 2, ScheduledAt: at}))
	require.NoError(t, repo.EnsureDay(models.SiteAttendanceLog{SiteName: "hdsky", Day: day, Status: models.AttendanceFailed, Attempts: 1, ScheduledAt: at, LastError: "验证码没认对"}))
	require.NoError(t, repo.EnsureDay(models.SiteAttendanceLog{SiteName: "u2dmhy", Day: day, Status: models.AttendancePending, ScheduledAt: at}))
	for _, ev := range []models.SiteAttendanceEvent{
		{SiteName: "hdtime", Day: day, Trigger: models.AttendanceTriggerAuto, Result: models.AttendanceResultRetry, Method: models.AttendanceMethodDirect, Error: "超时", StartedAt: at, FinishedAt: at.Add(time.Second)},
		{SiteName: "hdtime", Day: day, Trigger: models.AttendanceTriggerAuto, Result: models.AttendanceResultSigned, Method: models.AttendanceMethodDirect, Message: "第 9 次", StartedAt: at.Add(10 * time.Minute), FinishedAt: at.Add(10 * time.Minute)},
		{SiteName: "hdsky", Day: day, Trigger: models.AttendanceTriggerManual, Result: models.AttendanceResultFailed, Method: models.AttendanceMethodAICaptcha, AIModel: "vision-x", AICalls: 3, PromptTokens: 300, CompletionTokens: 9, DurationMs: 4200, Error: "验证码没认对", StartedAt: at, FinishedAt: at.Add(4 * time.Second)},
	} {
		require.NoError(t, repo.AddEvent(ev))
	}

	w := serveAuthed(mux, http.MethodGet, "/api/sites/attendance/events?day="+day, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got AttendanceDayResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, day, got.Day)
	assert.Equal(t, AttendanceDaySummary{Sites: 3, Signed: 1, Failed: 1, Pending: 1, AISites: 1, AICalls: 3, PromptTokens: 300, CompletionTokens: 9}, got.Summary)
	require.Len(t, got.Sites, 3)
	assert.Equal(t, []string{"hdsky", "hdtime", "u2dmhy"}, []string{got.Sites[0].SiteName, got.Sites[1].SiteName, got.Sites[2].SiteName})
	hdsky := got.Sites[0]
	assert.Equal(t, models.AttendanceMethodAICaptcha, hdsky.Method)
	assert.Equal(t, "vision-x", hdsky.AIModel)
	assert.EqualValues(t, 300, hdsky.PromptTokens)
	require.Len(t, hdsky.Events, 1)
	assert.Equal(t, models.AttendanceTriggerManual, hdsky.Events[0].Trigger)
	assert.EqualValues(t, 4200, hdsky.Events[0].DurationMs)
	hdtime := got.Sites[1]
	require.Len(t, hdtime.Events, 2, "每次尝试一行，按时间排")
	assert.Equal(t, models.AttendanceResultRetry, hdtime.Events[0].Result)
	assert.Equal(t, models.AttendanceResultSigned, hdtime.Events[1].Result)
	assert.Equal(t, at.Unix(), hdtime.Events[0].StartedAt)
	// 没有尝试记录的站点：方式按站点的签到方式（u2 直接签到）
	assert.Equal(t, models.AttendanceMethodDirect, got.Sites[2].Method)
	assert.Empty(t, got.Sites[2].Events)

	// 不给日期：今天；今天什么也没有时是空的
	w = serveAuthed(mux, http.MethodGet, "/api/sites/attendance/events", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, got.Today, got.Day)
	assert.Empty(t, got.Sites)

	w = serveAuthed(mux, http.MethodGet, "/api/sites/attendance/events?day=10-18", "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w = serveAuthed(mux, http.MethodPost, "/api/sites/attendance/events", "")
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}
