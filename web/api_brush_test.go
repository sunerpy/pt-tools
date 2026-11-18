package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// newBrushServer 按 Serve() 的方式注册刷流路由（含 s.auth），带一个站点、一个下载器。
func newBrushServer(t *testing.T) (*Server, *http.ServeMux, models.DownloaderSetting) {
	t.Helper()
	srv := setupServer(t)
	srv.mgr.StopAll()
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(&models.BrushTask{}, &models.BrushTorrent{}, &models.BrushTorrentSample{}, &models.BrushDailyStat{}, &models.BrushRun{}))
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", Enabled: true, AuthMethod: "cookie", Cookie: "c=1", IsBuiltin: true}).Error)
	ds := models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true}
	require.NoError(t, db.Create(&ds).Error)
	mux := http.NewServeMux()
	srv.registerBrushRoutes(mux)
	srv.sessions.put("sess-test", "admin")
	return srv, mux, ds
}

func brushBody(name string, dlID uint, mod func(map[string]any)) string {
	m := map[string]any{
		"name": name, "enabled": false, "site_name": "hdsky", "downloader_id": dlID, "interval_min": 10,
		"max_downloading": 3, "exclude_hr": true, "remove_free_expired_incomplete": true, "remove_with_data": true,
		"discounts": "free, 2xfree", "remove_ratio": 3,
	}
	if mod != nil {
		mod(m)
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func TestBrushAPI_RequiresSession(t *testing.T) {
	_, mux, _ := newBrushServer(t)
	for _, path := range []string{"/api/brush/tasks", "/api/brush/tasks/1", "/api/brush/tasks/1/runs", "/api/brush/stats"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		assert.Contains(t, []int{http.StatusUnauthorized, http.StatusFound}, w.Code, path)
	}
}

// 新建时关掉的开关原样保存：删种不删文件、不排除 H&R、免费到期不删。
func TestBrushAPI_CreateKeepsSwitchesOff(t *testing.T) {
	_, mux, ds := newBrushServer(t)
	w := serveAuthed(mux, http.MethodPost, "/api/brush/tasks", brushBody("keep files", ds.ID, func(m map[string]any) {
		m["exclude_hr"], m["remove_free_expired_incomplete"], m["remove_with_data"] = false, false, false
	}))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created BrushTaskView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.False(t, created.ExcludeHR)
	assert.False(t, created.RemoveFreeExpiredIncomplete)
	assert.False(t, created.RemoveWithData, "删种时不删文件")
}

func TestBrushAPI_CRUD(t *testing.T) {
	_, mux, ds := newBrushServer(t)

	w := serveAuthed(mux, http.MethodPost, "/api/brush/tasks", brushBody("hdsky 刷流", ds.ID, nil))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created BrushTaskView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.NotZero(t, created.ID)
	assert.Equal(t, "FREE,2XFREE", created.Discounts, "优惠类型规整成大写")
	assert.Equal(t, "qb", created.DownloaderName)
	assert.True(t, created.SiteEnabled)
	assert.False(t, created.Enabled, "默认关闭")
	assert.Equal(t, 30, created.RemoveLowSpeedWindowMin, "低速窗口缺省 30 分钟")

	w = serveAuthed(mux, http.MethodPost, "/api/brush/tasks", brushBody("hdsky 刷流", ds.ID, nil))
	assert.Equal(t, http.StatusConflict, w.Code, "同名任务")

	w = serveAuthed(mux, http.MethodGet, "/api/brush/tasks", "")
	require.Equal(t, http.StatusOK, w.Code)
	var list []BrushTaskView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list, 1)

	path := fmt.Sprintf("/api/brush/tasks/%d", created.ID)
	w = serveAuthed(mux, http.MethodPut, path, brushBody("改了名", ds.ID, func(m map[string]any) { m["enabled"] = true; m["max_downloading"] = 5 }))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var updated BrushTaskView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	assert.Equal(t, "改了名", updated.Name)
	assert.True(t, updated.Enabled)
	assert.Equal(t, 5, updated.MaxDownloading)

	w = serveAuthed(mux, http.MethodGet, path, "")
	require.Equal(t, http.StatusOK, w.Code)

	w = serveAuthed(mux, http.MethodDelete, path, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = serveAuthed(mux, http.MethodGet, path, "")
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = serveAuthed(mux, http.MethodDelete, path, "")
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = serveAuthed(mux, http.MethodPut, path, brushBody("x", ds.ID, nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestBrushAPI_Validation(t *testing.T) {
	_, mux, ds := newBrushServer(t)
	cases := map[string]func(map[string]any){
		"任务名称不能为空":     func(m map[string]any) { m["name"] = "  " },
		"请选择站点":        func(m map[string]any) { m["site_name"] = "" },
		"没有内置定义":       func(m map[string]any) { m["site_name"] = "qa-dynamic" },
		"还没有配置":        func(m map[string]any) { m["site_name"] = "ourbits" },
		"请选择下载器":       func(m map[string]any) { m["downloader_id"] = 0 },
		"下载器已经不存在":     func(m map[string]any) { m["downloader_id"] = 99 },
		"检查间隔":         func(m map[string]any) { m["interval_min"] = 2 },
		"同时下载数":        func(m map[string]any) { m["max_downloading"] = 0 },
		"不认识的优惠类型":     func(m map[string]any) { m["discounts"] = "FREE,HALF" },
		"不能是负数":        func(m map[string]any) { m["remove_ratio"] = -1 },
		"最小体积不能大于最大体积": func(m map[string]any) { m["min_size_gb"] = 10; m["max_size_gb"] = 5 },
		"时间窗":          func(m map[string]any) { m["remove_low_speed_kbs"] = 50; m["remove_low_speed_window_min"] = 2 },
	}
	for want, mod := range cases {
		w := serveAuthed(mux, http.MethodPost, "/api/brush/tasks", brushBody("t", ds.ID, mod))
		assert.Equal(t, http.StatusBadRequest, w.Code, want)
		assert.Contains(t, w.Body.String(), want)
	}
	w := serveAuthed(mux, http.MethodPost, "/api/brush/tasks", "{")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	for _, path := range []string{"/api/brush/tasks/abc", "/api/brush/tasks/0"} {
		w = serveAuthed(mux, http.MethodGet, path, "")
		assert.Equal(t, http.StatusBadRequest, w.Code, path)
	}
	w = serveAuthed(mux, http.MethodGet, "/api/brush/tasks/1/unknown", "")
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = serveAuthed(mux, http.MethodPatch, "/api/brush/tasks", "")
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// 分区只对 M-Team：取值规整成小写、不认识的拒收；别的站点（包括从 M-Team 改成别的站点）一律存成空。
func TestBrushAPI_Section(t *testing.T) {
	_, mux, ds := newBrushServer(t)
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{Name: "mteam", Enabled: true, AuthMethod: "api_key", APIKey: "k", IsBuiltin: true}).Error)
	create := func(name, site, section string) (int, BrushTaskView, string) {
		w := serveAuthed(mux, http.MethodPost, "/api/brush/tasks", brushBody(name, ds.ID, func(m map[string]any) {
			m["site_name"], m["section"] = site, section
		}))
		var v BrushTaskView
		_ = json.Unmarshal(w.Body.Bytes(), &v)
		return w.Code, v, w.Body.String()
	}

	code, adult, body := create("成人区", "mteam", " ADULT ")
	require.Equal(t, http.StatusCreated, code, body)
	assert.Equal(t, "adult", adult.Section)

	code, both, body := create("两个都读", "mteam", "all")
	require.Equal(t, http.StatusCreated, code, body)
	assert.Equal(t, "all", both.Section)

	code, old, body := create("老任务", "mteam", "")
	require.Equal(t, http.StatusCreated, code, body)
	assert.Empty(t, old.Section, "空值等于综合区")

	code, _, body = create("不认识", "mteam", "music")
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Contains(t, body, "不认识的分区 music")

	code, other, body := create("hdsky", "hdsky", "adult")
	require.Equal(t, http.StatusCreated, code, body)
	assert.Empty(t, other.Section, "别的站点存成空")

	w := serveAuthed(mux, http.MethodPut, fmt.Sprintf("/api/brush/tasks/%d", both.ID), brushBody("两个都读", ds.ID, func(m map[string]any) {
		m["site_name"], m["section"] = "hdsky", "all"
	}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var moved BrushTaskView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &moved))
	assert.Equal(t, "hdsky", moved.SiteName)
	assert.Empty(t, moved.Section, "改成别的站点时清空")
}

type brushStubSite struct{ items []v2.TorrentItem }

func (s brushStubSite) ID() string { return "hdsky" }

func (s brushStubSite) Name() string { return "HDSky" }

func (s brushStubSite) Kind() v2.SiteKind { return v2.SiteNexusPHP }

func (s brushStubSite) Login(context.Context, v2.Credentials) error { return nil }

func (s brushStubSite) GetUserInfo(context.Context) (v2.UserInfo, error) { return v2.UserInfo{}, nil }

func (s brushStubSite) Download(context.Context, string) ([]byte, error) {
	return nil, fmt.Errorf("no")
}

func (s brushStubSite) Close() error { return nil }

func (s brushStubSite) Search(context.Context, v2.SearchQuery) ([]v2.TorrentItem, error) {
	return s.items, nil
}

type brushListDL struct {
	downloader.Downloader
	torrents []downloader.Torrent
}

func (d brushListDL) GetAllTorrents() ([]downloader.Torrent, error) { return d.torrents, nil }

func TestBrushAPI_RunTorrentsAndStats(t *testing.T) {
	srv, mux, ds := newBrushServer(t)
	w := serveAuthed(mux, http.MethodPost, "/api/brush/tasks", brushBody("t", ds.ID, func(m map[string]any) { m["enabled"] = true }))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var task BrushTaskView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &task))
	runPath := fmt.Sprintf("/api/brush/tasks/%d/run", task.ID)

	w = serveAuthed(mux, http.MethodPost, runPath, "")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code, "没有接线监控")

	repo := models.NewBrushRepository(global.GlobalDB.DB)
	now := time.Now()
	day := now.Format("2006-01-02")
	bt := &models.BrushTorrent{
		TaskID: task.ID, InfoHash: "h1", SiteName: "hdsky", TorrentID: "1", Title: "T1",
		SizeBytes: 100, AddedAt: now, State: models.BrushTorrentActive, Progress: 0.5,
	}
	require.NoError(t, repo.RecordAdded(bt, day))
	require.NoError(t, repo.RecordSample(bt, models.BrushTorrentSample{At: now, Uploaded: 300, Downloaded: 50}, 0.5, 6, 0, time.Local))

	mon := scheduler.NewBrushMonitor(scheduler.BrushMonitorConfig{
		DB:    global.GlobalDB.DB,
		Sites: scheduler.BrushSitesFunc(func(string) (v2.Site, bool) { return brushStubSite{}, true }),
		Downloaders: scheduler.BrushDownloadersFunc(func(uint) (downloader.Downloader, string, error) {
			return brushListDL{torrents: []downloader.Torrent{{ID: "x", InfoHash: "h1", Tags: models.BrushTaskTag(task.ID), Progress: 0.5, TotalUploaded: 300, TotalDownloaded: 50}}}, "qb", nil
		}),
	})
	srv.mgr.SetBrushMonitor(mon)

	w = serveAuthed(mux, http.MethodPost, runPath, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var run BrushRunResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &run))
	assert.Empty(t, run.Error)
	assert.Equal(t, 1, run.Result.Sampled)
	w = serveAuthed(mux, http.MethodGet, runPath, "")
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	w = serveAuthed(mux, http.MethodPost, "/api/brush/tasks/999/run", "")
	assert.Equal(t, http.StatusNotFound, w.Code)

	w = serveAuthed(mux, http.MethodGet, fmt.Sprintf("/api/brush/tasks/%d/torrents?state=active", task.ID), "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var page struct {
		Items []models.BrushTorrent `json:"items"`
		Total int64                 `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	assert.EqualValues(t, 1, page.Total)
	assert.Equal(t, "T1", page.Items[0].Title)
	w = serveAuthed(mux, http.MethodGet, fmt.Sprintf("/api/brush/tasks/%d/torrents?state=bad", task.ID), "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w = serveAuthed(mux, http.MethodGet, "/api/brush/tasks/999/torrents", "")
	assert.Equal(t, http.StatusNotFound, w.Code)

	w = serveAuthed(mux, http.MethodGet, "/api/brush/tasks", "")
	var list []BrushTaskView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list, 1)
	assert.Equal(t, 1, list[0].ActiveCount)
	assert.Equal(t, 1, list[0].DownloadingCount)
	assert.EqualValues(t, 300, list[0].Today.Uploaded)
	assert.EqualValues(t, 300, list[0].Total.Uploaded)

	w = serveAuthed(mux, http.MethodGet, "/api/brush/stats?days=7", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var stats BrushStatsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &stats))
	require.Len(t, stats.Dates, 7)
	assert.Equal(t, day, stats.To)
	assert.EqualValues(t, 300, stats.Totals[6].Uploaded)
	require.Len(t, stats.Tasks, 1)
	assert.Equal(t, 1, stats.Tasks[0].Series[6].Added)
	for _, q := range []string{"?days=0", "?days=91", "?days=x"} {
		w = serveAuthed(mux, http.MethodGet, "/api/brush/stats"+q, "")
		assert.Equal(t, http.StatusBadRequest, w.Code, q)
	}
	w = serveAuthed(mux, http.MethodPost, "/api/brush/stats", "")
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	// 运行中删除、修改任务被拒（409）
	require.NoError(t, mon.WithTaskLock(task.ID, func() error {
		w = serveAuthed(mux, http.MethodDelete, fmt.Sprintf("/api/brush/tasks/%d", task.ID), "")
		assert.Equal(t, http.StatusConflict, w.Code)
		w = serveAuthed(mux, http.MethodPost, runPath, "")
		assert.Equal(t, http.StatusConflict, w.Code)
		w = serveAuthed(mux, http.MethodPut, fmt.Sprintf("/api/brush/tasks/%d", task.ID), brushBody("t", ds.ID, nil))
		assert.Equal(t, http.StatusConflict, w.Code)
		return nil
	}))

	// 这一轮没跑完（下载器不可用）：502，原因在正文里
	srv.mgr.SetBrushMonitor(scheduler.NewBrushMonitor(scheduler.BrushMonitorConfig{
		DB:    global.GlobalDB.DB,
		Sites: scheduler.BrushSitesFunc(func(string) (v2.Site, bool) { return brushStubSite{}, true }),
		Downloaders: scheduler.BrushDownloadersFunc(func(uint) (downloader.Downloader, string, error) {
			return nil, "", fmt.Errorf("connection refused")
		}),
	}))
	w = serveAuthed(mux, http.MethodPost, runPath, "")
	assert.Equal(t, http.StatusBadGateway, w.Code)
	assert.Contains(t, w.Body.String(), "下载器不可用")
}

// 运行记录：「立即运行」记成手动，新的在前，带步骤；可以分页；参数不对回 400，任务不存在回 404。
func TestBrushAPI_Runs(t *testing.T) {
	srv, mux, ds := newBrushServer(t)
	w := serveAuthed(mux, http.MethodPost, "/api/brush/tasks", brushBody("t", ds.ID, func(m map[string]any) { m["enabled"] = true }))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var task BrushTaskView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &task))
	runPath := fmt.Sprintf("/api/brush/tasks/%d/run", task.ID)
	runsPath := fmt.Sprintf("/api/brush/tasks/%d/runs", task.ID)

	w = serveAuthed(mux, http.MethodGet, runsPath, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"items":[],"total":0}`, w.Body.String(), "还没运行过")

	srv.mgr.SetBrushMonitor(scheduler.NewBrushMonitor(scheduler.BrushMonitorConfig{
		DB:    global.GlobalDB.DB,
		Sites: scheduler.BrushSitesFunc(func(string) (v2.Site, bool) { return brushStubSite{}, true }),
		Downloaders: scheduler.BrushDownloadersFunc(func(uint) (downloader.Downloader, string, error) {
			return brushListDL{}, "qb", nil
		}),
	}))
	w = serveAuthed(mux, http.MethodPost, runPath, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	srv.mgr.SetBrushMonitor(scheduler.NewBrushMonitor(scheduler.BrushMonitorConfig{
		DB:    global.GlobalDB.DB,
		Sites: scheduler.BrushSitesFunc(func(string) (v2.Site, bool) { return brushStubSite{}, true }),
		Downloaders: scheduler.BrushDownloadersFunc(func(uint) (downloader.Downloader, string, error) {
			return nil, "", fmt.Errorf("connection refused")
		}),
	}))
	w = serveAuthed(mux, http.MethodPost, runPath, "")
	require.Equal(t, http.StatusBadGateway, w.Code)

	w = serveAuthed(mux, http.MethodGet, runsPath, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var page BrushRunPage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.EqualValues(t, 2, page.Total)
	require.Len(t, page.Items, 2)
	latest, first := page.Items[0], page.Items[1]
	assert.Greater(t, latest.ID, first.ID, "新的在前")
	assert.Equal(t, models.BrushRunManual, latest.Trigger)
	assert.Equal(t, models.BrushRunFailed, latest.Status)
	assert.Contains(t, latest.Error, "connection refused")
	require.NotEmpty(t, latest.Steps)
	assert.Equal(t, "error", latest.Steps[len(latest.Steps)-1].Kind)
	assert.Equal(t, models.BrushRunOK, first.Status)
	assert.NotNil(t, first.FinishedAt)
	assert.NotEmpty(t, first.Summary)
	// 原始 JSON 里步骤是数组、steps 字符串不外露
	var raw struct {
		Items []map[string]any `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.IsType(t, []any{}, raw.Items[0]["steps"])
	assert.Contains(t, raw.Items[0], "steps_truncated")

	w = serveAuthed(mux, http.MethodGet, runsPath+"?limit=1&offset=1", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	page = BrushRunPage{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	assert.EqualValues(t, 2, page.Total)
	require.Len(t, page.Items, 1)
	assert.Equal(t, first.ID, page.Items[0].ID)

	for _, q := range []string{"?limit=0", "?limit=101", "?limit=x", "?offset=-1", "?offset=x"} {
		w = serveAuthed(mux, http.MethodGet, runsPath+q, "")
		assert.Equal(t, http.StatusBadRequest, w.Code, q)
	}
	w = serveAuthed(mux, http.MethodGet, "/api/brush/tasks/999/runs", "")
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = serveAuthed(mux, http.MethodPost, runsPath, "")
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	// 删任务连带删它的运行记录
	w = serveAuthed(mux, http.MethodDelete, fmt.Sprintf("/api/brush/tasks/%d", task.ID), "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var n int64
	require.NoError(t, global.GlobalDB.DB.Model(&models.BrushRun{}).Count(&n).Error)
	assert.Zero(t, n)
}

// 开启的任务要求站点与下载器都已启用；关闭的任务可以先存成草稿。读库失败回 500。
func TestBrushAPI_EnabledRequiresEnabledDeps(t *testing.T) {
	_, mux, ds := newBrushServer(t)
	db := global.GlobalDB.DB
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdtime", Enabled: false, AuthMethod: "cookie", IsBuiltin: true}).Error)
	w := serveAuthed(mux, http.MethodPost, "/api/brush/tasks", brushBody("draft", ds.ID, func(m map[string]any) { m["site_name"] = "hdtime" }))
	assert.Equal(t, http.StatusCreated, w.Code, "关闭的任务可以选未启用的站点")
	w = serveAuthed(mux, http.MethodPost, "/api/brush/tasks", brushBody("on", ds.ID, func(m map[string]any) { m["site_name"] = "hdtime"; m["enabled"] = true }))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "未启用")

	require.NoError(t, db.Model(&models.DownloaderSetting{}).Where("id = ?", ds.ID).Update("enabled", false).Error)
	w = serveAuthed(mux, http.MethodPost, "/api/brush/tasks", brushBody("on2", ds.ID, func(m map[string]any) { m["enabled"] = true }))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "下载器未启用")

	require.NoError(t, db.Migrator().DropTable(&models.DownloaderSetting{}))
	w = serveAuthed(mux, http.MethodPost, "/api/brush/tasks", brushBody("x", ds.ID, nil))
	assert.Equal(t, http.StatusInternalServerError, w.Code, "读库失败不是参数错误")
}

// 2026-10-28 用户：刷流任务页要能按天、周、月、季度、年看统计，「30 天收益」也能切换范围。
// 分段按进程时区，周从周一开始；最后一段是当前这一段，只算到今天。
func TestBuildBrushStats_Granularity(t *testing.T) {
	newBrushServer(t)
	db := global.GlobalDB.DB
	repo := models.NewBrushRepository(db)
	require.NoError(t, db.Create(&models.BrushTask{Name: "a", SiteName: "hdsky", DownloaderID: 1}).Error)
	var task models.BrushTask
	require.NoError(t, db.First(&task).Error)
	for _, st := range []models.BrushDailyStat{
		{TaskID: task.ID, Day: "2025-12-31", Uploaded: 1},
		{TaskID: task.ID, Day: "2026-06-30", Uploaded: 2},
		{TaskID: task.ID, Day: "2026-07-01", Uploaded: 4},
		{TaskID: task.ID, Day: "2026-09-30", Uploaded: 8, Downloaded: 1},
		{TaskID: task.ID, Day: "2026-10-01", Uploaded: 16, Added: 1},
		{TaskID: task.ID, Day: "2026-10-25", Uploaded: 32},
		{TaskID: task.ID, Day: "2026-10-26", Uploaded: 64, Removed: 2},
		{TaskID: task.ID, Day: "2026-10-29", Uploaded: 128, Downloaded: 3},
	} {
		require.NoError(t, db.Create(&st).Error)
	}
	// 2026-10-29 是周四
	now := time.Date(2026, 10, 29, 15, 0, 0, 0, time.Local)
	type seg struct {
		date, end string
		up        int64
	}
	cases := []struct {
		gran    string
		periods int
		want    []seg
		days    int
	}{
		{BrushGranDay, 3, []seg{{"2026-10-27", "2026-10-27", 0}, {"2026-10-28", "2026-10-28", 0}, {"2026-10-29", "2026-10-29", 128}}, 3},
		{BrushGranWeek, 2, []seg{{"2026-10-19", "2026-10-25", 32}, {"2026-10-26", "2026-11-01", 192}}, 11},
		{BrushGranMonth, 2, []seg{{"2026-09-01", "2026-09-30", 8}, {"2026-10-01", "2026-10-31", 240}}, 59},
		{BrushGranQuarter, 2, []seg{{"2026-04-01", "2026-06-30", 2}, {"2026-07-01", "2026-09-30", 12}}, 0},
		{BrushGranYear, 2, []seg{{"2025-01-01", "2025-12-31", 1}, {"2026-01-01", "2026-12-31", 254}}, 0},
	}
	cases[3].periods = 3
	cases[3].want = append(cases[3].want, seg{"2026-10-01", "2026-12-31", 240})
	for _, tc := range cases {
		t.Run(tc.gran, func(t *testing.T) {
			resp, err := buildBrushStats(repo, now, tc.gran, tc.periods)
			require.NoError(t, err)
			assert.Equal(t, tc.gran, resp.Granularity)
			assert.Equal(t, tc.periods, resp.Periods)
			assert.Equal(t, "2026-10-29", resp.To, "只算到今天")
			require.Len(t, resp.Totals, len(tc.want))
			for i, w := range tc.want {
				assert.Equal(t, w.date, resp.Totals[i].Date, "段 %d 的第一天", i)
				assert.Equal(t, w.end, resp.Totals[i].End, "段 %d 的最后一天", i)
				assert.Equal(t, w.up, resp.Totals[i].Uploaded, "段 %d 的上传", i)
				assert.Equal(t, w.date, resp.Dates[i])
			}
			require.Len(t, resp.Tasks, 1)
			assert.Equal(t, resp.Totals, resp.Tasks[0].Series, "只有一个任务时与合计相同")
			if tc.days > 0 {
				assert.Equal(t, tc.days, resp.Days)
			}
			assert.Equal(t, tc.want[0].date, resp.From)
		})
	}
	// 周日也属于这一周（周一开始）
	sunday := time.Date(2026, 11, 1, 9, 0, 0, 0, time.Local)
	resp, err := buildBrushStats(repo, sunday, BrushGranWeek, 1)
	require.NoError(t, err)
	assert.Equal(t, "2026-10-26", resp.Totals[0].Date)
	assert.EqualValues(t, 192, resp.Totals[0].Uploaded)
	assert.EqualValues(t, 3, resp.Totals[0].Downloaded)
	assert.Equal(t, 2, resp.Totals[0].Removed)
}

func TestBrushStatsAPI_GranularityParams(t *testing.T) {
	_, mux, _ := newBrushServer(t)
	w := serveAuthed(mux, http.MethodGet, "/api/brush/stats?granularity=month&periods=6", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var stats BrushStatsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &stats))
	assert.Equal(t, "month", stats.Granularity)
	require.Len(t, stats.Totals, 6)
	assert.True(t, strings.HasSuffix(stats.Totals[5].Date, "-01"), "每月的第一天")

	w = serveAuthed(mux, http.MethodGet, "/api/brush/stats?granularity=year", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &stats))
	assert.Equal(t, 5, stats.Periods, "年默认 5 段")

	w = serveAuthed(mux, http.MethodGet, "/api/brush/stats", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &stats))
	assert.Equal(t, "day", stats.Granularity)
	assert.Equal(t, 30, stats.Periods, "默认还是最近 30 天")

	for _, q := range []string{"?granularity=hour", "?granularity=week&periods=0", "?granularity=week&periods=105", "?granularity=year&periods=x"} {
		w = serveAuthed(mux, http.MethodGet, "/api/brush/stats"+q, "")
		assert.Equal(t, http.StatusBadRequest, w.Code, q)
	}
}
