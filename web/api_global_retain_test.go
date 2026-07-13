package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// postGlobal 向 apiGlobal 发送 POST，body 为给定 map（缺失字段模拟旧前端/omitted）。
func postGlobal(t *testing.T, srv *Server, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/global", bytes.NewReader(raw))
	srv.apiGlobal(w, req)
	return w
}

// TA1：POST 漏传 4 个字段（default_enabled/retain_hours/max_retry/default_concurrency）
// 时，不得将 DB 中已有的值清零。这是核心回归用例。
func TestAPIGlobal_OmittedFieldsNotClobbered(t *testing.T) {
	srv := setupServer(t)
	// 先写入已有配置：RetainHours=24 / MaxRetry=3 / DefaultConcurrency=3 / DefaultEnabled=true
	require.NoError(t, srv.store.SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir:        t.TempDir(),
		RetainHours:        24,
		MaxRetry:           3,
		DefaultConcurrency: 3,
		DefaultEnabled:     true,
	}))

	// POST body 完全不含这 4 个字段（模拟旧前端）。
	w := postGlobal(t, srv, map[string]any{
		"default_interval_minutes": 20,
		"download_dir":             t.TempDir(),
		"torrent_size_gb":          200,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	got, err := srv.store.GetGlobalSettings()
	require.NoError(t, err)
	assert.Equal(t, 24, got.RetainHours, "RetainHours 不应被清零")
	assert.Equal(t, 3, got.MaxRetry, "MaxRetry 不应被清零")
	assert.Equal(t, int32(3), got.DefaultConcurrency, "DefaultConcurrency 不应被清零")
	assert.True(t, got.DefaultEnabled, "DefaultEnabled 不应被清零")
}

// TA2：显式提供的值应正确写入。
func TestAPIGlobal_ExplicitValuesApplied(t *testing.T) {
	srv := setupServer(t)
	require.NoError(t, srv.store.SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir:        t.TempDir(),
		RetainHours:        24,
		MaxRetry:           3,
		DefaultConcurrency: 3,
		DefaultEnabled:     true,
	}))

	w := postGlobal(t, srv, map[string]any{
		"default_interval_minutes": 20,
		"download_dir":             t.TempDir(),
		"retain_hours":             48,
		"max_retry":                5,
		"default_concurrency":      4,
		"default_enabled":          false,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	got, err := srv.store.GetGlobalSettings()
	require.NoError(t, err)
	assert.Equal(t, 48, got.RetainHours)
	assert.Equal(t, 5, got.MaxRetry)
	assert.Equal(t, int32(4), got.DefaultConcurrency)
	assert.False(t, got.DefaultEnabled)
}

// TA3：显式 0 必须被保留（指针能区分 omitted 与 explicit-0）。
// RetainHours=0 = 禁用自动 sweep，MaxRetry=0 = 不按重试删除，均为合法值。
func TestAPIGlobal_ExplicitZeroPreserved(t *testing.T) {
	srv := setupServer(t)
	require.NoError(t, srv.store.SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir:        t.TempDir(),
		RetainHours:        24,
		MaxRetry:           3,
		DefaultConcurrency: 3,
	}))

	w := postGlobal(t, srv, map[string]any{
		"default_interval_minutes": 20,
		"download_dir":             t.TempDir(),
		"retain_hours":             0,
		"max_retry":                0,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	got, err := srv.store.GetGlobalSettings()
	require.NoError(t, err)
	assert.Equal(t, 0, got.RetainHours, "显式 retain_hours=0 应被保留")
	assert.Equal(t, 0, got.MaxRetry, "显式 max_retry=0 应被保留")
	// 未传的 default_concurrency 保持原值
	assert.Equal(t, int32(3), got.DefaultConcurrency)
}

// TA4：空 DB 首次保存时，漏传的 4 字段应落合理默认（retain=24/maxRetry=3/concurrency=3）。
func TestAPIGlobal_EmptyDBFirstSaveDefaults(t *testing.T) {
	srv := setupServer(t)
	// 清空全局设置行，模拟真正的空 DB 首保存。
	require.NoError(t, global.GlobalDB.DB.Where("1 = 1").Delete(&models.SettingsGlobal{}).Error)

	w := postGlobal(t, srv, map[string]any{
		"default_interval_minutes": 20,
		"download_dir":             t.TempDir(),
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	got, err := srv.store.GetGlobalSettings()
	require.NoError(t, err)
	assert.Equal(t, 24, got.RetainHours)
	assert.Equal(t, 3, got.MaxRetry)
	assert.Equal(t, int32(3), got.DefaultConcurrency)
}

// TC1：retain_hours/max_retry/default_concurrency 保存后可 round-trip（GET 返回相同值）。
func TestAPIGlobal_RetainRoundTrip(t *testing.T) {
	srv := setupServer(t)
	require.NoError(t, srv.store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir()}))

	w := postGlobal(t, srv, map[string]any{
		"default_interval_minutes": 20,
		"download_dir":             t.TempDir(),
		"retain_hours":             36,
		"max_retry":                5,
		"default_concurrency":      4,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	getW := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/api/global", nil)
	srv.apiGlobal(getW, getReq)
	require.Equal(t, http.StatusOK, getW.Code)

	var resp models.SettingsGlobal
	require.NoError(t, json.Unmarshal(getW.Body.Bytes(), &resp))
	assert.Equal(t, 36, resp.RetainHours)
	assert.Equal(t, 5, resp.MaxRetry)
	assert.Equal(t, int32(4), resp.DefaultConcurrency)
}
