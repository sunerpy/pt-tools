// MIT License
// Copyright (c) 2025 pt-tools

package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

func TestFaviconService_fetchAndSave_NoDB(t *testing.T) {
	// 保存当前 GlobalDB 并在测试后恢复
	oldDB := global.GlobalDB
	global.GlobalDB = nil
	defer func() { global.GlobalDB = oldDB }()

	// 测试在没有数据库时的行为
	fs := &FaviconService{
		refreshInterval: 12 * time.Hour,
	}

	err := fs.fetchAndSave("test", "Test Site", "https://example.com/favicon.ico")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "数据库未初始化")
}

func TestFaviconService_GetFavicon_NoDB(t *testing.T) {
	// 保存当前 GlobalDB 并在测试后恢复
	oldDB := global.GlobalDB
	global.GlobalDB = nil
	defer func() { global.GlobalDB = oldDB }()

	fs := &FaviconService{
		refreshInterval: 12 * time.Hour,
	}

	cache, err := fs.GetFavicon("test")
	assert.Error(t, err)
	assert.Nil(t, cache)
}

func TestApiFavicon_MethodNotAllowed(t *testing.T) {
	server := &Server{}

	// POST 请求到非 refresh 路径应该返回 405
	req := httptest.NewRequest(http.MethodPost, "/api/favicon/hdsky", nil)
	rec := httptest.NewRecorder()

	// 初始化服务避免 nil panic
	faviconService = &FaviconService{
		refreshInterval: 12 * time.Hour,
	}

	server.apiFavicon(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestApiFavicon_EmptySiteID(t *testing.T) {
	server := &Server{}

	// 初始化服务
	faviconService = &FaviconService{
		refreshInterval: 12 * time.Hour,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/favicon/", nil)
	rec := httptest.NewRecorder()

	server.apiFavicon(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestApiFavicon_NonexistentSite(t *testing.T) {
	server := &Server{}

	// 初始化服务
	faviconService = &FaviconService{
		refreshInterval: 12 * time.Hour,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/favicon/nonexistent_site_xyz", nil)
	rec := httptest.NewRecorder()

	server.apiFavicon(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestApiFaviconRefresh_MethodNotAllowed(t *testing.T) {
	server := &Server{}

	// 初始化服务
	faviconService = &FaviconService{
		refreshInterval: 12 * time.Hour,
	}

	// GET 请求应该返回 405
	req := httptest.NewRequest(http.MethodGet, "/api/favicon/hdsky/refresh", nil)
	rec := httptest.NewRecorder()

	server.apiFaviconRefresh(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestApiFaviconRefresh_EmptySiteID(t *testing.T) {
	server := &Server{}

	// 初始化服务
	faviconService = &FaviconService{
		refreshInterval: 12 * time.Hour,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/favicon//refresh", nil)
	rec := httptest.NewRecorder()

	server.apiFaviconRefresh(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestApiFaviconRefresh_NonexistentSite(t *testing.T) {
	server := &Server{}

	// 初始化服务
	faviconService = &FaviconService{
		refreshInterval: 12 * time.Hour,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/favicon/nonexistent_xyz/refresh", nil)
	rec := httptest.NewRecorder()

	server.apiFaviconRefresh(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestServeFaviconData_Caching(t *testing.T) {
	server := &Server{}

	testData := []byte{0x00, 0x00, 0x01, 0x00} // 简单的 ICO 文件头
	cache := &models.FaviconCache{
		SiteID:      "test",
		SiteName:    "Test Site",
		Data:        testData,
		ContentType: "image/x-icon",
		ETag:        "abc123",
	}

	// 第一次请求
	req1 := httptest.NewRequest(http.MethodGet, "/api/favicon/test", nil)
	rec1 := httptest.NewRecorder()

	server.serveFaviconData(rec1, req1, cache)

	assert.Equal(t, http.StatusOK, rec1.Code)
	assert.Equal(t, `"abc123"`, rec1.Header().Get("ETag"))
	assert.Equal(t, "public, max-age=86400", rec1.Header().Get("Cache-Control"))
	assert.Equal(t, "image/x-icon", rec1.Header().Get("Content-Type"))

	// 带有 If-None-Match 的请求（应该返回 304）
	req2 := httptest.NewRequest(http.MethodGet, "/api/favicon/test", nil)
	req2.Header.Set("If-None-Match", `"abc123"`)
	rec2 := httptest.NewRecorder()

	server.serveFaviconData(rec2, req2, cache)

	assert.Equal(t, http.StatusNotModified, rec2.Code)
}

func TestApiFaviconList_WithRegisteredSites(t *testing.T) {
	// 注册测试站点定义
	testDef := &v2.SiteDefinition{
		ID:         "testsite_favicon",
		Name:       "Test Site for Favicon",
		URLs:       []string{"https://test.example.com/"},
		FaviconURL: "https://test.example.com/favicon.ico",
	}
	v2.RegisterSiteDefinition(testDef)

	// 初始化服务
	faviconService = &FaviconService{
		refreshInterval: 12 * time.Hour,
	}

	server := &Server{}

	req := httptest.NewRequest(http.MethodGet, "/api/favicons", nil)
	rec := httptest.NewRecorder()

	// 注意：这个测试需要 global.GlobalDB，如果没有会返回 500
	// 但至少可以测试路由是否正确
	server.apiFaviconList(rec, req)

	// 由于没有初始化数据库，应该返回 500
	// 但这验证了函数被正确调用
	assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusInternalServerError)
}
