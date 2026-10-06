package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
)

func createBrushTask(t *testing.T, mux *http.ServeMux, name string, dlID uint) string {
	t.Helper()
	w := serveAuthed(mux, http.MethodPost, "/api/brush/tasks", brushBody(name, dlID, nil))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created BrushTaskView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	return fmt.Sprintf("/api/brush/tasks/%d", created.ID)
}

// 数据库没初始化时三个入口都回 503。
func TestBrushAPI_NoDatabase(t *testing.T) {
	_, mux, _ := newBrushServer(t)
	saved := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = saved })
	for _, path := range []string{"/api/brush/tasks", "/api/brush/tasks/1", "/api/brush/stats"} {
		w := serveAuthed(mux, http.MethodGet, path, "")
		assert.Equal(t, http.StatusServiceUnavailable, w.Code, path)
	}
}

// 读库失败一律回 500，不当成参数错误或「不存在」。
func TestBrushAPI_DatabaseErrors(t *testing.T) {
	_, mux, ds := newBrushServer(t)
	db := global.GlobalDB.DB
	path := createBrushTask(t, mux, "t", ds.ID)
	expect500 := func(method, p, body string) {
		t.Helper()
		w := serveAuthed(mux, method, p, body)
		assert.Equal(t, http.StatusInternalServerError, w.Code, "%s %s: %s", method, p, w.Body.String())
	}

	require.NoError(t, db.Migrator().DropTable(&models.BrushTorrent{}))
	expect500(http.MethodGet, "/api/brush/tasks", "")
	expect500(http.MethodGet, path, "")
	expect500(http.MethodPut, path, brushBody("t", ds.ID, nil))
	expect500(http.MethodGet, path+"/torrents", "")
	expect500(http.MethodDelete, path, "")
	require.NoError(t, db.AutoMigrate(&models.BrushTorrent{}))

	require.NoError(t, db.Migrator().DropTable(&models.BrushDailyStat{}))
	expect500(http.MethodGet, "/api/brush/tasks", "")
	expect500(http.MethodGet, "/api/brush/stats", "")
	require.NoError(t, db.AutoMigrate(&models.BrushDailyStat{}))

	require.NoError(t, db.Migrator().RenameTable("downloader_settings", "downloader_settings_off"))
	expect500(http.MethodGet, "/api/brush/tasks", "")
	expect500(http.MethodPost, "/api/brush/tasks", brushBody("t3", ds.ID, nil))
	require.NoError(t, db.Migrator().RenameTable("downloader_settings_off", "downloader_settings"))

	require.NoError(t, db.Migrator().RenameTable("site_settings", "site_settings_off"))
	expect500(http.MethodGet, "/api/brush/tasks", "")
	expect500(http.MethodPost, "/api/brush/tasks", brushBody("t3", ds.ID, nil))
	require.NoError(t, db.Migrator().RenameTable("site_settings_off", "site_settings"))

	require.NoError(t, db.Migrator().DropTable(&models.BrushTask{}))
	expect500(http.MethodGet, "/api/brush/tasks", "")
	expect500(http.MethodGet, "/api/brush/stats", "")
	expect500(http.MethodGet, path, "")
	expect500(http.MethodPost, "/api/brush/tasks", brushBody("t4", ds.ID, nil))
}

// 修改：请求体错误回 400，改成别的任务的名字回 409；不支持的方法回 405。
func TestBrushAPI_UpdateErrorsAndMethods(t *testing.T) {
	srv, mux, ds := newBrushServer(t)
	path := createBrushTask(t, mux, "a", ds.ID)
	createBrushTask(t, mux, "b", ds.ID)

	w := serveAuthed(mux, http.MethodPut, path, "{")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w = serveAuthed(mux, http.MethodPut, path, brushBody("b", ds.ID, nil))
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "已经有同名的刷流任务")
	w = serveAuthed(mux, http.MethodPatch, path, "")
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	w = serveAuthed(mux, http.MethodPost, path+"/torrents", "")
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	srv.mgr = nil // 没有调度器时修改照常保存
	w = serveAuthed(mux, http.MethodPut, path, brushBody("a2", ds.ID, nil))
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

// 立即运行：运行中途任务被删回 404，刷流服务已停止回 503。
func TestBrushAPI_RunNotFoundAndStopped(t *testing.T) {
	srv, mux, ds := newBrushServer(t)
	path := createBrushTask(t, mux, "a", ds.ID)

	// 监控用另一套空库：接口先查到了任务，运行时却读不到（等同于两步之间被删）
	other, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := other.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, other.DB.AutoMigrate(&models.BrushTask{}, &models.BrushTorrent{}, &models.BrushTorrentSample{}, &models.BrushDailyStat{}))
	mon := scheduler.NewBrushMonitor(scheduler.BrushMonitorConfig{DB: other.DB})
	srv.mgr.SetBrushMonitor(mon)
	w := serveAuthed(mux, http.MethodPost, path+"/run", "")
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	mon.Stop()
	w = serveAuthed(mux, http.MethodPost, path+"/run", "")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
}

func TestBrushAPI_MoreValidation(t *testing.T) {
	_, mux, ds := newBrushServer(t)
	cases := map[string]func(map[string]any){
		"任务名称不能超过 64 个字":  func(m map[string]any) { m["name"] = strings.Repeat("刷", 65) },
		"做种人数上限不能是负数":     func(m map[string]any) { m["max_seeders"] = -1 },
		"关键词不能超过 1000 个字": func(m map[string]any) { m["include_keywords"] = strings.Repeat("a", 1001) },
		"标签、分类或保存路径太长":    func(m map[string]any) { m["tags"] = strings.Repeat("t", 201) },
	}
	for want, mod := range cases {
		w := serveAuthed(mux, http.MethodPost, "/api/brush/tasks", brushBody("t", ds.ID, mod))
		assert.Equal(t, http.StatusBadRequest, w.Code, want)
		assert.Contains(t, w.Body.String(), want)
	}
	w := serveAuthed(mux, http.MethodPost, "/api/brush/tasks", brushBody("t", ds.ID, func(m map[string]any) { m["discounts"] = "free,, 2xfree," }))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created BrushTaskView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.Equal(t, "FREE,2XFREE", created.Discounts, "空项忽略")
}
