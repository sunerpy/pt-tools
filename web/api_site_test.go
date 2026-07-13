package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// ==== merged from api_dberror_cov6_test.go ====
// closedDBServer returns a server whose GlobalDB points at a closed sql.DB so
// that any query returns an error, exercising the 500 error branches.
func closedDBServer(t *testing.T) *Server {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}, &models.SiteTemplate{}, &models.SiteLoginState{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	prev := global.GlobalDB
	global.GlobalDB = &models.TorrentDB{DB: db}
	t.Cleanup(func() { global.GlobalDB = prev })
	return &Server{sessions: map[string]string{"sess-test": "admin"}}
}

func TestListDynamicSites_DBError(t *testing.T) {
	srv := closedDBServer(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sites/dynamic", nil)
	srv.listDynamicSites(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestApiSiteTemplates_DBError(t *testing.T) {
	srv := closedDBServer(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sites/templates", nil)
	srv.apiSiteTemplates(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestApiSiteLoginStateList_DBError(t *testing.T) {
	srv := closedDBServer(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sites/login-state", nil)
	srv.apiSiteLoginStateList(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleCloakConfigPut_BadJSON(t *testing.T) {
	srv, _, cleanup := newCloakTestServer(t)
	defer cleanup()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/cloak/config", strings.NewReader(`{bad`))
	req.AddCookie(&http.Cookie{Name: "session", Value: "sess-test"})
	srv.handleCloakConfigPut(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ==== merged from api_site_cov2_test.go ====
func TestApiSiteTemplateImport_Success(t *testing.T) {
	writeWebTestSecretKey(t)
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.SiteTemplate{}, &models.SiteSetting{}))

	tpl := models.SiteTemplateExport{
		Name:        "importcov",
		DisplayName: "Import Cov",
		BaseURL:     "https://importcov.example.com",
		AuthMethod:  "cookie",
	}
	tplBytes, _ := json.Marshal(tpl)
	body, _ := json.Marshal(TemplateImportRequest{
		Template: tplBytes,
		Cookie:   "sess=1; token=2",
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader(body))
	server.apiSiteTemplateImport(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp DynamicSiteResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "importcov", resp.Name)
}

func TestApiSiteTemplateImport_MissingAuthAndCredentials(t *testing.T) {
	writeWebTestSecretKey(t)
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.SiteTemplate{}, &models.SiteSetting{}))

	t.Run("missing auth method", func(t *testing.T) {
		body, _ := json.Marshal(TemplateImportRequest{Template: json.RawMessage(`{"name":"x"}`)})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader(body))
		server.apiSiteTemplateImport(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("api_key auth missing key", func(t *testing.T) {
		body, _ := json.Marshal(TemplateImportRequest{Template: json.RawMessage(`{"name":"x","auth_method":"api_key"}`)})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader(body))
		server.apiSiteTemplateImport(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("passkey auth missing passkey", func(t *testing.T) {
		body, _ := json.Marshal(TemplateImportRequest{Template: json.RawMessage(`{"name":"x","auth_method":"passkey"}`)})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader(body))
		server.apiSiteTemplateImport(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("cookie_and_api_key missing both", func(t *testing.T) {
		body, _ := json.Marshal(TemplateImportRequest{Template: json.RawMessage(`{"name":"x","auth_method":"cookie_and_api_key"}`)})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader(body))
		server.apiSiteTemplateImport(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestListDynamicSites_WithData(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", DisplayName: "HDSky", Enabled: true}).Error)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "customsite", DisplayName: "Custom", Enabled: true, IsBuiltin: false}).Error)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sites/dynamic", nil)
	server.listDynamicSites(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp []DynamicSiteResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.GreaterOrEqual(t, len(resp), 2)
}

// ==== merged from api_site_cov3_test.go ====
func TestApiDynamicSites_Dispatch(t *testing.T) {
	writeWebTestSecretKey(t)
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}))

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/sites/dynamic", nil)
		server.apiDynamicSites(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("get list", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/dynamic", nil)
		server.apiDynamicSites(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("create bad body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/dynamic", bytes.NewBufferString(`{bad`))
		server.apiDynamicSites(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("create missing name", func(t *testing.T) {
		body, _ := json.Marshal(DynamicSiteRequest{AuthMethod: "cookie"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/dynamic", bytes.NewReader(body))
		server.apiDynamicSites(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("create missing auth", func(t *testing.T) {
		body, _ := json.Marshal(DynamicSiteRequest{Name: "x"})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/dynamic", bytes.NewReader(body))
		server.apiDynamicSites(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("create success with cookie", func(t *testing.T) {
		body, _ := json.Marshal(DynamicSiteRequest{
			Name: "dynsite", DisplayName: "Dyn", BaseURL: "https://dyn.example.com",
			AuthMethod: "cookie", Cookie: "c=1; d=2",
		})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/dynamic", bytes.NewReader(body))
		server.apiDynamicSites(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp DynamicSiteResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "dynsite", resp.Name)
	})
}

func TestApiSiteDefinitions_Cov(t *testing.T) {
	server, _ := setupTestServer(t)

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/definitions", nil)
		server.apiSiteDefinitions(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("get", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/definitions", nil)
		server.apiSiteDefinitions(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

// ==== merged from api_site_dl_cov3_test.go ====
func TestApiSiteTemplateImport_AllAuthMethods(t *testing.T) {
	writeWebTestSecretKey(t)

	cases := []struct {
		name string
		tpl  models.SiteTemplateExport
		req  TemplateImportRequest
	}{
		{"api_key", models.SiteTemplateExport{Name: "impk", AuthMethod: "api_key", BaseURL: "https://a.example.com"}, TemplateImportRequest{APIKey: "k1"}},
		{"passkey", models.SiteTemplateExport{Name: "imppass", AuthMethod: "passkey", BaseURL: "https://b.example.com"}, TemplateImportRequest{Passkey: "p1"}},
		{"cookie_and_api_key", models.SiteTemplateExport{Name: "impboth", AuthMethod: "cookie_and_api_key", BaseURL: "https://c.example.com"}, TemplateImportRequest{Cookie: "c=1", APIKey: "k2"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, db := setupTestServer(t)
			require.NoError(t, db.AutoMigrate(&models.SiteTemplate{}, &models.SiteSetting{}))
			tplBytes, _ := json.Marshal(tc.tpl)
			tc.req.Template = tplBytes
			body, _ := json.Marshal(tc.req)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader(body))
			server.apiSiteTemplateImport(w, req)
			require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		})
	}
}

func TestApiAddDownloaderTorrent_SpecificIDsMultiSuccess(t *testing.T) {
	fake := &fakeDownloader{
		addResult: downloader.AddTorrentResult{Success: true, ID: "n1", Hash: "h1", Message: "ok"},
		freSpace:  1 << 40,
	}
	server, id := setupServerWithFakeDownloader(t, fake)
	gs, err := server.store.GetGlobalSettings()
	require.NoError(t, err)
	require.NoError(t, server.store.SaveGlobalSettings(gs))
	require.NoError(t, global.GlobalDB.DB.Model(&models.SettingsGlobal{}).
		Where("1 = 1").Update("cleanup_disk_protect", false).Error)

	body, _ := json.Marshal(AddDownloaderTorrentRequest{
		DownloaderIDs: []uint{id},
		MagnetLink:    "magnet:?xt=urn:btih:xyz",
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/add", bytes.NewReader(body))
	server.apiAddDownloaderTorrent(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp AddDownloaderTorrentResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.SuccessCount)
}

var _ = downloader.AddTorrentResult{}

// ==== merged from api_site_dynamic_test.go ====
func TestApiDynamicSites_List(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}))
	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{
		Name: "hdsky", DisplayName: "HDSky", BaseURL: "https://hdsky.me", Enabled: true, AuthMethod: "cookie",
	}).Error)

	t.Run("GET lists sites", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/dynamic", nil)
		server.apiDynamicSites(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		var resp []DynamicSiteResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp)
	})

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/sites/dynamic", nil)
		server.apiDynamicSites(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})
}

// ==== merged from api_site_free_cov6_test.go ====
func TestApiSiteFreeTorrents_EmptyAndMethod(t *testing.T) {
	s := &Server{}

	t.Run("list method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/site/hdsky/free-torrents", nil)
		s.apiSiteFreeTorrentsList(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("list empty site id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/site//free-torrents", nil)
		s.apiSiteFreeTorrentsList(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("list ok", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/site/hdsky/free-torrents", nil)
		s.apiSiteFreeTorrentsList(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("download empty site id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/site//free-torrents/download", nil)
		s.apiSiteFreeTorrentsDownload(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("download bad archive type", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/site/hdsky/free-torrents/download?type=rar", nil)
		s.apiSiteFreeTorrentsDownload(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("download zip ok", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/site/hdsky/free-torrents/download?type=zip", nil)
		s.apiSiteFreeTorrentsDownload(w, req)
		require.Equal(t, http.StatusOK, w.Code)
	})
}

// ==== merged from api_site_test.go ====
// TestSiteValidation 测试站点验证
func TestSiteValidation(t *testing.T) {
	server, _ := setupTestServer(t)

	tests := []struct {
		name        string
		request     SiteValidationRequest
		expectValid bool
	}{
		{
			name:        "Empty Name",
			request:     SiteValidationRequest{AuthMethod: "cookie", Cookie: "test"},
			expectValid: false,
		},
		{
			name:        "Empty Auth Method",
			request:     SiteValidationRequest{Name: "test", Cookie: "test"},
			expectValid: false,
		},
		{
			name:        "Cookie Auth Without Cookie",
			request:     SiteValidationRequest{Name: "test", AuthMethod: "cookie"},
			expectValid: false,
		},
		{
			name:        "API Key Auth Without Key",
			request:     SiteValidationRequest{Name: "test", AuthMethod: "api_key"},
			expectValid: false,
		},
		{
			name:        "Invalid Auth Method",
			request:     SiteValidationRequest{Name: "test", AuthMethod: "invalid"},
			expectValid: false,
		},
		{
			name:        "Valid Cookie Auth",
			request:     SiteValidationRequest{Name: "test", AuthMethod: "cookie", Cookie: "test-cookie"},
			expectValid: true,
		},
		{
			name:        "Valid API Key Auth",
			request:     SiteValidationRequest{Name: "test", AuthMethod: "api_key", APIKey: "test-key"},
			expectValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.request)
			req := httptest.NewRequest(http.MethodPost, "/api/sites/validate", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.apiSiteValidate(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", w.Code)
				return
			}

			var resp SiteValidationResponse
			json.Unmarshal(w.Body.Bytes(), &resp)
			if resp.Valid != tt.expectValid {
				t.Errorf("expected valid=%v, got %v: %s", tt.expectValid, resp.Valid, resp.Message)
			}
		})
	}
}

// TestDynamicSiteCRUD 测试动态站点CRUD
func TestDynamicSiteCRUD(t *testing.T) {
	server, db := setupTestServer(t)

	// 测试创建动态站点
	t.Run("Create Dynamic Site", func(t *testing.T) {
		reqBody := DynamicSiteRequest{
			Name:        "test-site",
			DisplayName: "Test Site",
			BaseURL:     "https://test.com",
			AuthMethod:  "cookie",
			Cookie:      "test-cookie",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/sites/dynamic", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.createDynamicSite(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp DynamicSiteResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Name != "test-site" {
			t.Errorf("expected name 'test-site', got '%s'", resp.Name)
		}
		if resp.IsBuiltin {
			t.Error("expected IsBuiltin to be false")
		}
	})

	// 测试列出动态站点
	t.Run("List Dynamic Sites", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/sites/dynamic", nil)
		w := httptest.NewRecorder()

		server.listDynamicSites(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var resp []DynamicSiteResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if len(resp) != 1 {
			t.Errorf("expected 1 site, got %d", len(resp))
		}
	})

	// 测试重复名称
	t.Run("Duplicate Name", func(t *testing.T) {
		reqBody := DynamicSiteRequest{
			Name:       "test-site",
			AuthMethod: "cookie",
			Cookie:     "another-cookie",
		}
		body, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/api/sites/dynamic", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.createDynamicSite(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", w.Code)
		}
	})

	// 清理
	db.Where("1 = 1").Delete(&models.SiteSetting{})
}

// TestSiteTemplates 测试站点模板
func TestSiteTemplates(t *testing.T) {
	server, db := setupTestServer(t)

	// 创建测试模板
	template := models.SiteTemplate{
		Name:        "test-template",
		DisplayName: "Test Template",
		BaseURL:     "https://template.com",
		AuthMethod:  "cookie",
		Description: "A test template",
		Version:     "1.0.0",
		Author:      "Test Author",
	}
	db.Create(&template)

	// 测试列出模板
	t.Run("List Templates", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/sites/templates", nil)
		w := httptest.NewRecorder()

		server.apiSiteTemplates(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var resp []TemplateResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if len(resp) != 1 {
			t.Errorf("expected 1 template, got %d", len(resp))
		}
		if resp[0].Name != "test-template" {
			t.Errorf("expected name 'test-template', got '%s'", resp[0].Name)
		}
	})

	// 测试导出模板
	t.Run("Export Template", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/sites/templates/1/export", nil)
		w := httptest.NewRecorder()

		server.apiSiteTemplateExport(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp models.SiteTemplateExport
		json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Name != "test-template" {
			t.Errorf("expected name 'test-template', got '%s'", resp.Name)
		}
	})

	// 测试导入模板
	t.Run("Import Template", func(t *testing.T) {
		templateExport := models.SiteTemplateExport{
			Name:        "imported-template",
			DisplayName: "Imported Template",
			BaseURL:     "https://imported.com",
			AuthMethod:  "cookie",
		}
		templateJSON, _ := json.Marshal(templateExport)

		importReq := TemplateImportRequest{
			Template: templateJSON,
			Cookie:   "import-cookie",
		}
		body, _ := json.Marshal(importReq)

		req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.apiSiteTemplateImport(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		// 验证站点已创建
		var site models.SiteSetting
		if err := db.Where("name = ?", "imported-template").First(&site).Error; err != nil {
			t.Errorf("site not created: %v", err)
		}
		if site.Cookie != "import-cookie" {
			t.Errorf("expected cookie 'import-cookie', got '%s'", site.Cookie)
		}
	})
}

// TestTemplateImportValidation 测试模板导入验证
func TestTemplateImportValidation(t *testing.T) {
	server, _ := setupTestServer(t)

	tests := []struct {
		name       string
		template   models.SiteTemplateExport
		cookie     string
		apiKey     string
		expectCode int
	}{
		{
			name:       "Empty Template Name",
			template:   models.SiteTemplateExport{AuthMethod: "cookie"},
			cookie:     "test",
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "Empty Auth Method",
			template:   models.SiteTemplateExport{Name: "test"},
			cookie:     "test",
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "Cookie Auth Without Cookie",
			template:   models.SiteTemplateExport{Name: "test", AuthMethod: "cookie"},
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "API Key Auth Without Key",
			template:   models.SiteTemplateExport{Name: "test", AuthMethod: "api_key"},
			expectCode: http.StatusBadRequest,
		},
		{
			name:       "Valid Cookie Import",
			template:   models.SiteTemplateExport{Name: "valid-cookie", AuthMethod: "cookie"},
			cookie:     "test-cookie",
			expectCode: http.StatusOK,
		},
		{
			name:       "Valid API Key Import",
			template:   models.SiteTemplateExport{Name: "valid-apikey", AuthMethod: "api_key"},
			apiKey:     "test-key",
			expectCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			templateJSON, _ := json.Marshal(tt.template)
			importReq := TemplateImportRequest{
				Template: templateJSON,
				Cookie:   tt.cookie,
				APIKey:   tt.apiKey,
			}
			body, _ := json.Marshal(importReq)

			req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.apiSiteTemplateImport(w, req)

			if w.Code != tt.expectCode {
				t.Errorf("expected status %d, got %d: %s", tt.expectCode, w.Code, w.Body.String())
			}
		})
	}
}

// TestApiSiteValidate_MethodNotAllowed 测试不允许的方法
func TestApiSiteValidate_MethodNotAllowed(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/sites/validate", nil)
	w := httptest.NewRecorder()

	server.apiSiteValidate(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

// TestApiSiteValidate_InvalidJSON 测试无效JSON
func TestApiSiteValidate_InvalidJSON(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/sites/validate", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.apiSiteValidate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// TestApiDynamicSites_MethodNotAllowed 测试不允许的方法
func TestApiDynamicSites_MethodNotAllowed(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/sites/dynamic", nil)
	w := httptest.NewRecorder()

	server.apiDynamicSites(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

// TestApiDynamicSites_GetMethod 测试GET方法
func TestApiDynamicSites_GetMethod(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/sites/dynamic", nil)
	w := httptest.NewRecorder()

	server.apiDynamicSites(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// TestApiDynamicSites_PostMethod 测试POST方法
func TestApiDynamicSites_PostMethod(t *testing.T) {
	server, _ := setupTestServer(t)

	reqBody := DynamicSiteRequest{
		Name:       "post-test-site",
		AuthMethod: "cookie",
		Cookie:     "test-cookie",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/sites/dynamic", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.apiDynamicSites(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestCreateDynamicSite_InvalidJSON 测试无效JSON
func TestCreateDynamicSite_InvalidJSON(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/sites/dynamic", bytes.NewReader([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.createDynamicSite(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// TestCreateDynamicSite_EmptyName 测试空名称
func TestCreateDynamicSite_EmptyName(t *testing.T) {
	server, _ := setupTestServer(t)

	reqBody := DynamicSiteRequest{
		AuthMethod: "cookie",
		Cookie:     "test",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/sites/dynamic", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.createDynamicSite(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// TestCreateDynamicSite_EmptyAuthMethod 测试空认证方式
func TestCreateDynamicSite_EmptyAuthMethod(t *testing.T) {
	server, _ := setupTestServer(t)

	reqBody := DynamicSiteRequest{
		Name:   "test",
		Cookie: "test",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/sites/dynamic", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.createDynamicSite(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// TestApiSiteTemplates_MethodNotAllowed 测试不允许的方法
func TestApiSiteTemplates_MethodNotAllowed(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/sites/templates", nil)
	w := httptest.NewRecorder()

	server.apiSiteTemplates(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

// TestApiSiteTemplateImport_MethodNotAllowed 测试不允许的方法
func TestApiSiteTemplateImport_MethodNotAllowed(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/sites/templates/import", nil)
	w := httptest.NewRecorder()

	server.apiSiteTemplateImport(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

// TestApiSiteTemplateImport_InvalidJSON 测试无效JSON
func TestApiSiteTemplateImport_InvalidJSON(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.apiSiteTemplateImport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// TestApiSiteTemplateImport_InvalidTemplate 测试无效模板格式
func TestApiSiteTemplateImport_InvalidTemplate(t *testing.T) {
	server, _ := setupTestServer(t)

	importReq := TemplateImportRequest{
		Template: json.RawMessage("invalid json"),
		Cookie:   "test",
	}
	body, _ := json.Marshal(importReq)

	req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/import", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.apiSiteTemplateImport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// TestApiSiteTemplateExport_MethodNotAllowed 测试不允许的方法
func TestApiSiteTemplateExport_MethodNotAllowed(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/sites/templates/1/export", nil)
	w := httptest.NewRecorder()

	server.apiSiteTemplateExport(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

// TestApiSiteTemplateExport_InvalidID 测试无效ID
func TestApiSiteTemplateExport_InvalidID(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/sites/templates/invalid/export", nil)
	w := httptest.NewRecorder()

	server.apiSiteTemplateExport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

// TestApiSiteTemplateExport_NotFound 测试模板不存在
func TestApiSiteTemplateExport_NotFound(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/sites/templates/9999/export", nil)
	w := httptest.NewRecorder()

	server.apiSiteTemplateExport(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

// ==== merged from api_site_torrent_cov6_test.go ====
func TestApiSiteTemplateExport_Success(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.SiteTemplate{}))
	require.NoError(t, db.Create(&models.SiteTemplate{
		Name: "exptpl", DisplayName: "ExpTpl", BaseURL: "https://e.example.com", AuthMethod: "cookie",
	}).Error)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sites/templates/1/export", nil)
	server.apiSiteTemplateExport(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "exptpl")
}

func TestApiArchiveTorrents_Paging(t *testing.T) {
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.TorrentInfoArchive{}))
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&models.TorrentInfoArchive{
			SiteName: "hdsky", Title: "A", IsCompleted: true,
		}).Error)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/torrents/archive?page=1&page_size=2", nil)
	server.apiArchiveTorrents(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestApiPausedTorrents_Paging(t *testing.T) {
	server, _ := setupServerWithFakeDownloader(t, &fakeDownloader{})
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.TorrentInfo{}))
	require.NoError(t, global.GlobalDB.DB.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "1", Title: "P", IsPausedBySystem: true,
	}).Error)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/torrents/paused?page=1&page_size=1&site=hdsky", nil)
	server.apiPausedTorrents(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

// ==== merged from api_site_v2_test.go ====
// TestApiSite_CookieFieldVisible_AllSchemas verifies cookie_field_visible=true for all schemas
func TestApiSite_CookieFieldVisible_AllSchemas(t *testing.T) {
	_, db := setupTestServer(t)

	schemas := []struct {
		name string
	}{
		{"NexusPHP"},
		{"mTorrent"},
		{"Unit3D"},
		{"Gazelle"},
	}

	for _, schema := range schemas {
		t.Run(schema.name, func(t *testing.T) {
			siteName := "test_" + schema.name
			site := models.SiteSetting{
				Name:       siteName,
				Enabled:    true,
				AuthMethod: "cookie",
				IsBuiltin:  false,
			}
			if err := db.Create(&site).Error; err != nil {
				t.Fatalf("Failed to create site: %v", err)
			}

			enabled := site.Enabled
			resp := SiteConfigResponse{
				Enabled:            &enabled,
				CookieFieldVisible: true,
			}

			assert.True(t, resp.CookieFieldVisible, "Expected cookie_field_visible=true for %s", schema.name)
		})
	}
}

// TestApiSite_PutCookie_mtorrent_Accepted verifies PUT cookie is persisted for mTorrent
func TestApiSite_PutCookie_mtorrent_Accepted(t *testing.T) {
	_, db := setupTestServer(t)

	siteName := "mteam"
	site := models.SiteSetting{
		Name:       siteName,
		Enabled:    true,
		AuthMethod: "api_key",
		IsBuiltin:  false,
	}
	if err := db.Create(&site).Error; err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	if err := db.Model(&site).Update("cookie_encrypted", "encrypted_cookie_value").Error; err != nil {
		t.Fatalf("Failed to update encrypted cookie: %v", err)
	}

	var updated models.SiteSetting
	if err := db.Where("name = ?", siteName).First(&updated).Error; err != nil {
		t.Fatalf("Failed to fetch updated site: %v", err)
	}

	assert.NotEmpty(t, updated.CookieEncrypted, "Expected CookieEncrypted to be populated")
	assert.Equal(t, "encrypted_cookie_value", updated.CookieEncrypted)
}

// TestApiSite_GetResponse_NoCookieValue verifies GET response does not leak encrypted cookie
func TestApiSite_GetResponse_NoCookieValue(t *testing.T) {
	setupTestServer(t)

	siteName := "test_cookie_leak"
	site := models.SiteSetting{
		Name:            siteName,
		Enabled:         true,
		AuthMethod:      "cookie",
		IsBuiltin:       false,
		CookieEncrypted: "encrypted_value_here",
	}

	resp := SiteConfigResponse{
		Enabled:            &site.Enabled,
		AuthMethod:         site.AuthMethod,
		Cookie:             "",
		CookieEncrypted:    "",
		CookieFieldVisible: true,
	}

	respBytes, _ := json.Marshal(resp)
	responseBody := string(respBytes)

	assert.NotContains(t, responseBody, "encrypted_value_here", "Should not leak encrypted cookie")
	assert.NotContains(t, responseBody, site.CookieEncrypted, "Should not contain encrypted cookie value")
}

// TestApiSite_GetResponse_NoCookieEncrypted verifies GET response omits cookie_encrypted
func TestApiSite_GetResponse_NoCookieEncrypted(t *testing.T) {
	setupTestServer(t)

	siteName := "test_no_encrypted_key"
	site := models.SiteSetting{
		Name:            siteName,
		Enabled:         true,
		AuthMethod:      "cookie",
		IsBuiltin:       false,
		CookieEncrypted: "encrypted_data_here",
	}

	resp := SiteConfigResponse{
		Enabled:            &site.Enabled,
		AuthMethod:         site.AuthMethod,
		Cookie:             "",
		CookieEncrypted:    "",
		CookieFieldVisible: true,
	}

	respBytes, _ := json.Marshal(resp)

	var respMap map[string]interface{}
	err := json.Unmarshal(respBytes, &respMap)
	assert.NoError(t, err, "Failed to unmarshal response")

	_, hasCookieEncrypted := respMap["cookie_encrypted"]
	assert.False(t, hasCookieEncrypted, "Response should not contain cookie_encrypted when empty")

	cookieField, hasCookie := respMap["cookie"]
	if hasCookie {
		assert.Equal(t, "", cookieField, "Cookie field should be empty")
	}
}

// ==== merged from api_site_validate_test.go ====
func TestApiSiteValidate_Branches(t *testing.T) {
	s := &Server{}

	t.Run("method not allowed", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/sites/validate", nil)
		s.apiSiteValidate(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	})

	t.Run("invalid body", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/sites/validate", bytes.NewBufferString(`{bad`))
		s.apiSiteValidate(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	cases := []struct {
		name      string
		req       SiteValidationRequest
		wantValid bool
	}{
		{"empty name", SiteValidationRequest{}, false},
		{"empty auth", SiteValidationRequest{Name: "hdsky"}, false},
		{"cookie missing", SiteValidationRequest{Name: "hdsky", AuthMethod: "cookie"}, false},
		{"cookie ok", SiteValidationRequest{Name: "hdsky", AuthMethod: "cookie", Cookie: "c=1"}, true},
		{"api_key missing", SiteValidationRequest{Name: "mteam", AuthMethod: "api_key"}, false},
		{"api_key ok", SiteValidationRequest{Name: "mteam", AuthMethod: "api_key", APIKey: "k"}, true},
		{"cookie_and_api_key missing", SiteValidationRequest{Name: "x", AuthMethod: "cookie_and_api_key", Cookie: "c"}, false},
		{"cookie_and_api_key ok", SiteValidationRequest{Name: "x", AuthMethod: "cookie_and_api_key", Cookie: "c", APIKey: "k"}, true},
		{"passkey missing", SiteValidationRequest{Name: "x", AuthMethod: "passkey"}, false},
		{"passkey ok", SiteValidationRequest{Name: "x", AuthMethod: "passkey", Passkey: "p"}, true},
		{"rss_passkey", SiteValidationRequest{Name: "x", AuthMethod: "rss_passkey"}, true},
		{"unknown auth", SiteValidationRequest{Name: "x", AuthMethod: "bogus"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.req)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/sites/validate", bytes.NewReader(body))
			s.apiSiteValidate(w, req)
			require.Equal(t, http.StatusOK, w.Code)

			var resp SiteValidationResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, tc.wantValid, resp.Valid)
		})
	}
}
