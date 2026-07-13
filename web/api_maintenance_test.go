package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// setupMaintenanceServer 构造一个 Server，并把 $HOME 指向一个 fake home，
// 内含 ~/.pt-tools/{logs,downloads,backups} 及若干可删除文件与红线文件。
// 返回 srv 与 fake home 路径，供断言磁盘状态。
func setupMaintenanceServer(t *testing.T) (*Server, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	work := filepath.Join(home, models.WorkDir)
	for _, d := range []string{"logs", "downloads", "backups"} {
		require.NoError(t, os.MkdirAll(filepath.Join(work, d), 0o755))
	}

	// logs：base 文件（红线）+ 一个超龄轮转备份（可删除）。
	require.NoError(t, os.WriteFile(filepath.Join(work, "logs", "all.log"), []byte("base"), 0o644))
	oldBackup := filepath.Join(work, "logs", "all-2020-01-01T00-00-00.000.log.gz")
	require.NoError(t, os.WriteFile(oldBackup, []byte("old-rotated"), 0o644))
	ancient := time.Now().Add(-400 * 24 * time.Hour)
	require.NoError(t, os.Chtimes(oldBackup, ancient, ancient))

	// 红线文件（位于工作根，不在白名单三根内，任何情况下都不得删除）。
	require.NoError(t, os.WriteFile(filepath.Join(work, models.DBFile), []byte("db"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(work, "secret.key"), []byte("key"), 0o600))

	srv := setupServer(t)
	return srv, home
}

// TestAPIMaintenance_PreviewGET：GET 为预览，返回将清理项，且磁盘上不删除任何文件。
func TestAPIMaintenance_PreviewGET(t *testing.T) {
	srv, home := setupMaintenanceServer(t)
	work := filepath.Join(home, models.WorkDir)
	oldBackup := filepath.Join(work, "logs", "all-2020-01-01T00-00-00.000.log.gz")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/maintenance/clean", nil)
	srv.apiMaintenanceClean(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp cleanResultDTO
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.DryRun, "GET 必须是预览（dryRun=true）")
	assert.GreaterOrEqual(t, resp.TotalDeleted, 1, "预览应列出至少一个将清理项")

	// 关键：预览绝不删除磁盘文件。
	_, err := os.Stat(oldBackup)
	assert.NoError(t, err, "预览不得删除文件: %s", oldBackup)
	_, err = os.Stat(filepath.Join(work, "logs", "all.log"))
	assert.NoError(t, err, "base 日志必须存活")
}

// TestAPIMaintenance_ExecutePOST：POST dryRun:false 真正删除可清理项，红线文件存活。
func TestAPIMaintenance_ExecutePOST(t *testing.T) {
	srv, home := setupMaintenanceServer(t)
	work := filepath.Join(home, models.WorkDir)
	oldBackup := filepath.Join(work, "logs", "all-2020-01-01T00-00-00.000.log.gz")

	body, _ := json.Marshal(maintenanceCleanRequest{DryRun: false})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/maintenance/clean", bytes.NewReader(body))
	srv.apiMaintenanceClean(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp cleanResultDTO
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.DryRun)

	// 可清理项已被删除。
	_, err := os.Stat(oldBackup)
	assert.True(t, os.IsNotExist(err), "超龄轮转备份应被删除: %s", oldBackup)

	// 红线文件必须存活。
	_, err = os.Stat(filepath.Join(work, models.DBFile))
	assert.NoError(t, err, "torrents.db 红线文件必须存活")
	_, err = os.Stat(filepath.Join(work, "secret.key"))
	assert.NoError(t, err, "secret.key 红线文件必须存活")
	_, err = os.Stat(filepath.Join(work, "logs", "all.log"))
	assert.NoError(t, err, "base 日志（红线）必须存活")
}

// TestAPIMaintenance_Unauthorized：无 session 经 auth 中间件应返回 401。
func TestAPIMaintenance_Unauthorized(t *testing.T) {
	srv, _ := setupMaintenanceServer(t)
	h := srv.auth(srv.apiMaintenanceClean)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/maintenance/clean", nil)
	h(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "unauthorized")
}

// TestAPIMaintenance_BadBody：POST 非法 JSON 应返回 400。
func TestAPIMaintenance_BadBody(t *testing.T) {
	srv, _ := setupMaintenanceServer(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/maintenance/clean", bytes.NewBufferString(`{bad`))
	srv.apiMaintenanceClean(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestAPIMaintenance_MethodNotAllowed：非 GET/POST 应返回 405。
func TestAPIMaintenance_MethodNotAllowed(t *testing.T) {
	srv, _ := setupMaintenanceServer(t)

	for _, m := range []string{http.MethodPut, http.MethodDelete} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(m, "/api/maintenance/clean", nil)
		srv.apiMaintenanceClean(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code, "method=%s", m)
	}
}

// TestAPIMaintenance_CategoryFilter：POST categories:["logs"] 仅清理 logs 类别。
func TestAPIMaintenance_CategoryFilter(t *testing.T) {
	srv, home := setupMaintenanceServer(t)
	work := filepath.Join(home, models.WorkDir)
	oldBackup := filepath.Join(work, "logs", "all-2020-01-01T00-00-00.000.log.gz")

	body, _ := json.Marshal(maintenanceCleanRequest{
		Categories: []string{"logs"},
		DryRun:     false,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/maintenance/clean", bytes.NewReader(body))
	srv.apiMaintenanceClean(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp cleanResultDTO
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// 结果中只应出现 logs 类别。
	for _, c := range resp.Categories {
		assert.Equal(t, "logs", c.Name, "只应清理 logs 类别，出现了: %s", c.Name)
	}

	// logs 的可清理项被删除。
	_, err := os.Stat(oldBackup)
	assert.True(t, os.IsNotExist(err), "logs 超龄备份应被删除")
}
