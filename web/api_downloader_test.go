package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// setupTestServer 创建测试服务器
func setupTestServer(t *testing.T) (*Server, *gorm.DB) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}

	// 迁移表
	db.AutoMigrate(
		&models.DownloaderSetting{},
		&models.SiteTemplate{},
		&models.AdminUser{},
		&models.SettingsGlobal{},
		&models.QbitSettings{},
		&models.SiteSetting{},
	)

	// 设置全局DB
	global.GlobalDB = &models.TorrentDB{DB: db}

	// 初始化logger（如果未初始化）
	if global.GlobalLogger == nil {
		zapLogger, _ := zap.NewDevelopment()
		global.GlobalLogger = zapLogger
	}

	store := core.NewConfigStore(global.GlobalDB)
	server := NewServer(store, nil)

	return server, db
}

// TestDownloaderCRUD 测试下载器CRUD操作
func TestDownloaderCRUD(t *testing.T) {
	server, db := setupTestServer(t)

	// 测试创建下载器
	t.Run("Create Downloader", func(t *testing.T) {
		reqBody := DownloaderRequest{
			Name:      "test-qbit",
			Type:      "qbittorrent",
			URL:       "http://localhost:8080",
			Username:  "admin",
			Password:  "password",
			IsDefault: true,
			Enabled:   true,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.createDownloader(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp DownloaderResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Name != "test-qbit" {
			t.Errorf("expected name 'test-qbit', got '%s'", resp.Name)
		}
		if resp.Type != "qbittorrent" {
			t.Errorf("expected type 'qbittorrent', got '%s'", resp.Type)
		}
	})

	// 测试列出下载器
	t.Run("List Downloaders", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders", nil)
		w := httptest.NewRecorder()

		server.listDownloaders(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var resp []DownloaderResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if len(resp) != 1 {
			t.Errorf("expected 1 downloader, got %d", len(resp))
		}
	})

	// 测试获取下载器详情
	t.Run("Get Downloader", func(t *testing.T) {
		var dl models.DownloaderSetting
		db.First(&dl)

		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/1", nil)
		w := httptest.NewRecorder()

		server.getDownloader(w, req, dl.ID)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var resp DownloaderResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Name != "test-qbit" {
			t.Errorf("expected name 'test-qbit', got '%s'", resp.Name)
		}
	})

	// 测试更新下载器
	t.Run("Update Downloader", func(t *testing.T) {
		var dl models.DownloaderSetting
		db.First(&dl)

		reqBody := DownloaderRequest{
			Name:      "updated-qbit",
			Type:      "qbittorrent",
			URL:       "http://localhost:9090",
			IsDefault: true, // 保持默认状态，因为是唯一的下载器
			Enabled:   true, // 默认下载器不能禁用
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/1", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.updateDownloader(w, req, dl.ID)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp DownloaderResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Name != "updated-qbit" {
			t.Errorf("expected name 'updated-qbit', got '%s'", resp.Name)
		}
		if resp.URL != "http://localhost:9090" {
			t.Errorf("expected URL 'http://localhost:9090', got '%s'", resp.URL)
		}
	})

	// 测试删除下载器
	t.Run("Delete Downloader", func(t *testing.T) {
		var dl models.DownloaderSetting
		db.First(&dl)

		req := httptest.NewRequest(http.MethodDelete, "/api/downloaders/1", nil)
		w := httptest.NewRecorder()

		server.deleteDownloader(w, req, dl.ID)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		// 验证已删除
		var count int64
		db.Model(&models.DownloaderSetting{}).Count(&count)
		if count != 0 {
			t.Errorf("expected 0 downloaders, got %d", count)
		}
	})
}

// TestDownloaderValidation 测试下载器验证
func TestDownloaderValidation(t *testing.T) {
	server, _ := setupTestServer(t)

	tests := []struct {
		name       string
		request    DownloaderRequest
		expectCode int
	}{
		{
			name:       "Empty Name",
			request:    DownloaderRequest{Type: "qbittorrent", URL: "http://localhost"},
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "Empty Type",
			request:    DownloaderRequest{Name: "test", URL: "http://localhost"},
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "Invalid Type",
			request:    DownloaderRequest{Name: "test", Type: "invalid", URL: "http://localhost"},
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "Empty URL",
			request:    DownloaderRequest{Name: "test", Type: "qbittorrent"},
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "Valid Request",
			request:    DownloaderRequest{Name: "test", Type: "qbittorrent", URL: "http://localhost"},
			expectCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.createDownloader(w, req)

			if w.Code != tt.expectCode {
				t.Errorf("expected status %d, got %d: %s", tt.expectCode, w.Code, w.Body.String())
			}
		})
	}
}

// TestDownloaderDefaultHandling 测试默认下载器处理
func TestDownloaderDefaultHandling(t *testing.T) {
	server, db := setupTestServer(t)

	// 创建第一个默认下载器
	dl1 := DownloaderRequest{
		Name:      "dl-1",
		Type:      "qbittorrent",
		URL:       "http://localhost:8080",
		IsDefault: true,
		Enabled:   true,
	}
	body1, _ := json.Marshal(dl1)
	req1 := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	server.createDownloader(w1, req1)

	// 创建第二个默认下载器
	dl2 := DownloaderRequest{
		Name:      "dl-2",
		Type:      "transmission",
		URL:       "http://localhost:9091",
		IsDefault: true,
		Enabled:   true,
	}
	body2, _ := json.Marshal(dl2)
	req2 := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	server.createDownloader(w2, req2)

	// 验证只有一个默认下载器
	var defaultCount int64
	db.Model(&models.DownloaderSetting{}).Where("is_default = ?", true).Count(&defaultCount)
	if defaultCount != 1 {
		t.Errorf("expected 1 default downloader, got %d", defaultCount)
	}

	// 验证第二个是默认的
	var dl models.DownloaderSetting
	db.Where("is_default = ?", true).First(&dl)
	if dl.Name != "dl-2" {
		t.Errorf("expected dl-2 to be default, got %s", dl.Name)
	}
}

// TestSetDefaultDownloader 测试设置默认下载器
func TestSetDefaultDownloader(t *testing.T) {
	server, db := setupTestServer(t)

	// 创建两个下载器
	dl1 := DownloaderRequest{
		Name:      "dl-1",
		Type:      "qbittorrent",
		URL:       "http://localhost:8080",
		IsDefault: true,
		Enabled:   true,
	}
	body1, _ := json.Marshal(dl1)
	req1 := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	server.createDownloader(w1, req1)

	dl2 := DownloaderRequest{
		Name:      "dl-2",
		Type:      "transmission",
		URL:       "http://localhost:9091",
		IsDefault: false,
		Enabled:   true,
	}
	body2, _ := json.Marshal(dl2)
	req2 := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	server.createDownloader(w2, req2)

	// 获取第二个下载器的ID
	var dlRecord models.DownloaderSetting
	db.Where("name = ?", "dl-2").First(&dlRecord)

	// 测试设置第二个为默认
	t.Run("Set Default", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/downloaders/2/set-default", nil)
		w := httptest.NewRecorder()

		server.setDefaultDownloader(w, req, "2")

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		// 验证只有一个默认下载器
		var defaultCount int64
		db.Model(&models.DownloaderSetting{}).Where("is_default = ?", true).Count(&defaultCount)
		if defaultCount != 1 {
			t.Errorf("expected 1 default downloader, got %d", defaultCount)
		}

		// 验证dl-2是默认的
		var dl models.DownloaderSetting
		db.Where("is_default = ?", true).First(&dl)
		if dl.Name != "dl-2" {
			t.Errorf("expected dl-2 to be default, got %s", dl.Name)
		}
	})
}

// TestCannotRemoveOnlyDefault 测试不能移除唯一默认下载器的默认状态
func TestCannotRemoveOnlyDefault(t *testing.T) {
	server, db := setupTestServer(t)

	// 创建一个默认下载器
	dl := DownloaderRequest{
		Name:      "only-dl",
		Type:      "qbittorrent",
		URL:       "http://localhost:8080",
		IsDefault: true,
		Enabled:   true,
	}
	body, _ := json.Marshal(dl)
	req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.createDownloader(w, req)

	// 获取下载器ID
	var dlRecord models.DownloaderSetting
	db.First(&dlRecord)

	// 尝试取消默认状态
	updateReq := DownloaderRequest{
		Name:      "only-dl",
		Type:      "qbittorrent",
		URL:       "http://localhost:8080",
		IsDefault: false, // 尝试取消默认
		Enabled:   true,
	}
	updateBody, _ := json.Marshal(updateReq)
	req2 := httptest.NewRequest(http.MethodPut, "/api/downloaders/1", bytes.NewReader(updateBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()

	server.updateDownloader(w2, req2, dlRecord.ID)

	// 应该返回错误
	if w2.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w2.Code)
	}
}

// TestCannotDeleteDefaultWithOthers 测试有其他下载器时不能删除默认下载器
func TestCannotDeleteDefaultWithOthers(t *testing.T) {
	server, db := setupTestServer(t)

	// 创建两个下载器
	dl1 := DownloaderRequest{
		Name:      "default-dl",
		Type:      "qbittorrent",
		URL:       "http://localhost:8080",
		IsDefault: true,
		Enabled:   true,
	}
	body1, _ := json.Marshal(dl1)
	req1 := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	server.createDownloader(w1, req1)

	dl2 := DownloaderRequest{
		Name:      "other-dl",
		Type:      "transmission",
		URL:       "http://localhost:9091",
		IsDefault: false,
		Enabled:   true,
	}
	body2, _ := json.Marshal(dl2)
	req2 := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	server.createDownloader(w2, req2)

	// 获取默认下载器ID
	var defaultDl models.DownloaderSetting
	db.Where("is_default = ?", true).First(&defaultDl)

	// 尝试删除默认下载器
	req3 := httptest.NewRequest(http.MethodDelete, "/api/downloaders/1", nil)
	w3 := httptest.NewRecorder()

	server.deleteDownloader(w3, req3, defaultDl.ID)

	// 应该返回错误
	if w3.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", w3.Code, w3.Body.String())
	}
}

// TestDownloaderAutoStart 测试下载器 auto_start 字段
func TestDownloaderAutoStart(t *testing.T) {
	server, db := setupTestServer(t)

	// 测试创建带 auto_start=true 的下载器
	t.Run("Create with AutoStart true", func(t *testing.T) {
		reqBody := DownloaderRequest{
			Name:      "auto-start-dl",
			Type:      "qbittorrent",
			URL:       "http://localhost:8080",
			Username:  "admin",
			Password:  "password",
			IsDefault: true,
			Enabled:   true,
			AutoStart: true,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.createDownloader(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp DownloaderResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if !resp.AutoStart {
			t.Error("expected auto_start to be true")
		}

		// 验证数据库中的值
		var dl models.DownloaderSetting
		db.First(&dl)
		if !dl.AutoStart {
			t.Error("expected auto_start in DB to be true")
		}
	})

	// 测试创建带 auto_start=false 的下载器
	t.Run("Create with AutoStart false", func(t *testing.T) {
		reqBody := DownloaderRequest{
			Name:      "no-auto-start-dl",
			Type:      "transmission",
			URL:       "http://localhost:9091",
			IsDefault: false,
			Enabled:   true,
			AutoStart: false,
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.createDownloader(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp DownloaderResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.AutoStart {
			t.Error("expected auto_start to be false")
		}
	})

	// 测试更新 auto_start 字段
	t.Run("Update AutoStart", func(t *testing.T) {
		var dl models.DownloaderSetting
		db.Where("name = ?", "auto-start-dl").First(&dl)

		reqBody := DownloaderRequest{
			Name:      "auto-start-dl",
			Type:      "qbittorrent",
			URL:       "http://localhost:8080",
			IsDefault: true,
			Enabled:   true,
			AutoStart: false, // 从 true 改为 false
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPut, "/api/downloaders/1", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.updateDownloader(w, req, dl.ID)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp DownloaderResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.AutoStart {
			t.Error("expected auto_start to be false after update")
		}

		// 验证数据库中的值
		db.First(&dl, dl.ID)
		if dl.AutoStart {
			t.Error("expected auto_start in DB to be false after update")
		}
	})

	// 测试列表返回 auto_start 字段
	t.Run("List includes AutoStart", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/downloaders", nil)
		w := httptest.NewRecorder()

		server.listDownloaders(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var resp []DownloaderResponse
		json.Unmarshal(w.Body.Bytes(), &resp)

		// 找到 auto-start-dl 并验证 auto_start 字段
		found := false
		for _, dl := range resp {
			if dl.Name == "auto-start-dl" {
				found = true
				// 之前更新为 false
				if dl.AutoStart {
					t.Error("expected auto_start to be false in list")
				}
			}
		}
		if !found {
			t.Error("auto-start-dl not found in list")
		}
	})

	// 测试获取详情返回 auto_start 字段
	t.Run("Get includes AutoStart", func(t *testing.T) {
		var dl models.DownloaderSetting
		db.Where("name = ?", "auto-start-dl").First(&dl)

		req := httptest.NewRequest(http.MethodGet, "/api/downloaders/1", nil)
		w := httptest.NewRecorder()

		server.getDownloader(w, req, dl.ID)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var resp DownloaderResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		// 之前更新为 false
		if resp.AutoStart {
			t.Error("expected auto_start to be false in get response")
		}
	})
}

func TestSetDefaultDownloaderAutoEnable(t *testing.T) {
	server, db := setupTestServer(t)

	db.Create(&models.DownloaderSetting{
		Name:      "disabled-dl",
		Type:      "qbittorrent",
		URL:       "http://localhost:8080",
		IsDefault: false,
		Enabled:   false,
	})

	var dl models.DownloaderSetting
	db.First(&dl)

	req := httptest.NewRequest(http.MethodPost, "/api/downloaders/1/set-default", nil)
	w := httptest.NewRecorder()

	server.setDefaultDownloader(w, req, "1")

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	db.First(&dl, dl.ID)
	if !dl.IsDefault {
		t.Error("expected is_default to be true")
	}
	if !dl.Enabled {
		t.Error("expected enabled to be true after setting as default")
	}

	var resp DownloaderResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.Enabled {
		t.Error("expected enabled in response to be true")
	}
}

// TestDownloaderAutoStartDefault 测试 auto_start 默认值
func TestDownloaderAutoStartDefault(t *testing.T) {
	server, db := setupTestServer(t)

	// 创建不指定 auto_start 的下载器（应该默认为 false）
	reqBody := DownloaderRequest{
		Name:      "default-auto-start-dl",
		Type:      "qbittorrent",
		URL:       "http://localhost:8080",
		IsDefault: true,
		Enabled:   true,
		// 不指定 AutoStart
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/downloaders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.createDownloader(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp DownloaderResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.AutoStart {
		t.Error("expected auto_start to default to false")
	}

	// 验证数据库中的值
	var dl models.DownloaderSetting
	db.First(&dl)
	if dl.AutoStart {
		t.Error("expected auto_start in DB to default to false")
	}
}
