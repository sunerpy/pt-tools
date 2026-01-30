package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sunerpy/pt-tools/models"
)

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
