package validator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/sunerpy/requests"
)

// Feature: downloader-site-extensibility, Property 6: Site Validation Result Completeness
// Test that validation returns all required information
func TestProperty6_ValidationResultCompleteness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.MaxSize = 50

	properties := gopter.NewProperties(parameters)

	// Property: Validation result always contains required fields
	properties.Property("validation result contains all required fields", prop.ForAll(
		func(name, displayName, baseURL string, authMethod int, cookie, apiKey, apiURL string) bool {
			req := &ValidationRequest{
				Name:        name,
				DisplayName: displayName,
				BaseURL:     baseURL,
				Cookie:      cookie,
				APIKey:      apiKey,
				APIURL:      apiURL,
			}

			// Set auth method
			if authMethod%2 == 0 {
				req.AuthMethod = AuthMethodCookie
			} else {
				req.AuthMethod = AuthMethodAPIKey
			}

			validator := NewSiteValidator()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			result := validator.Validate(ctx, req)

			// Result must always have these fields set
			if result == nil {
				return false
			}

			// Message must not be empty
			if result.Message == "" {
				return false
			}

			// ResponseTime must be positive
			if result.ResponseTime < 0 {
				return false
			}

			// Errors must be initialized (not nil)
			if result.Errors == nil {
				return false
			}

			// If success is true, there should be no errors
			if result.Success && len(result.Errors) > 0 {
				return false
			}

			// If there are validation errors, success should be false
			if len(result.Errors) > 0 && result.Success {
				return false
			}

			return true
		},
		gen.AlphaString(),  // name
		gen.AlphaString(),  // displayName
		gen.AlphaString(),  // baseURL
		gen.IntRange(0, 1), // authMethod
		gen.AlphaString(),  // cookie
		gen.AlphaString(),  // apiKey
		gen.AlphaString(),  // apiURL
	))

	// Property: Missing required fields always produce errors
	properties.Property("missing required fields produce validation errors", prop.ForAll(
		func(hasName, hasBaseURL, hasAuthMethod bool) bool {
			req := &ValidationRequest{}

			if hasName {
				req.Name = "test-site"
			}
			if hasBaseURL {
				req.BaseURL = "https://example.com"
			}
			if hasAuthMethod {
				req.AuthMethod = AuthMethodCookie
				req.Cookie = "test-cookie"
			}

			validator := NewSiteValidator()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			result := validator.Validate(ctx, req)

			// If any required field is missing, validation should fail
			allFieldsPresent := hasName && hasBaseURL && hasAuthMethod
			if !allFieldsPresent {
				// Should have errors
				return !result.Success && len(result.Errors) > 0
			}

			// All fields present - may still fail due to connection test
			return true
		},
		gen.Bool(), // hasName
		gen.Bool(), // hasBaseURL
		gen.Bool(), // hasAuthMethod
	))

	// Property: Auth method validation is consistent
	properties.Property("auth method validation is consistent", prop.ForAll(
		func(authMethodIdx int, hasCookie, hasAPIKey, hasAPIURL bool) bool {
			req := &ValidationRequest{
				Name:    "test-site",
				BaseURL: "https://example.com",
			}

			if authMethodIdx%2 == 0 {
				req.AuthMethod = AuthMethodCookie
			} else {
				req.AuthMethod = AuthMethodAPIKey
			}

			if hasCookie {
				req.Cookie = "test-cookie"
			}
			if hasAPIKey {
				req.APIKey = "test-api-key"
			}
			if hasAPIURL {
				req.APIURL = "https://api.example.com"
			}

			validator := NewSiteValidator()
			errors := validator.validateAuthFields(req)

			// Cookie auth requires cookie
			if req.AuthMethod == AuthMethodCookie {
				if !hasCookie {
					// Should have cookie error
					hasCookieError := false
					for _, e := range errors {
						if e.Field == "cookie" {
							hasCookieError = true
							break
						}
					}
					return hasCookieError
				}
				return len(errors) == 0
			}

			// API key auth requires api_key and api_url
			if req.AuthMethod == AuthMethodAPIKey {
				expectedErrors := 0
				if !hasAPIKey {
					expectedErrors++
				}
				if !hasAPIURL {
					expectedErrors++
				}
				return len(errors) == expectedErrors
			}

			return true
		},
		gen.IntRange(0, 1), // authMethodIdx
		gen.Bool(),         // hasCookie
		gen.Bool(),         // hasAPIKey
		gen.Bool(),         // hasAPIURL
	))

	// Property: Validation errors have field and message
	properties.Property("validation errors have field and message", prop.ForAll(
		func(name, baseURL string) bool {
			req := &ValidationRequest{
				Name:       name,
				BaseURL:    baseURL,
				AuthMethod: AuthMethodCookie,
				// Missing cookie
			}

			validator := NewSiteValidator()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			result := validator.Validate(ctx, req)

			// All errors must have field and message
			for _, err := range result.Errors {
				if err.Field == "" || err.Message == "" {
					return false
				}
			}

			return true
		},
		gen.AlphaString(), // name
		gen.AlphaString(), // baseURL
	))

	properties.TestingRun(t)
}

// TestValidatorWithMockServer tests validation with a mock HTTP server
func TestValidatorWithMockServer(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for cookie
		cookie := r.Header.Get("Cookie")
		if cookie == "valid-cookie" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
			return
		}

		// Check for API key
		apiKey := r.Header.Get("x-api-key")
		if apiKey == "valid-api-key" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
			return
		}

		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	tests := []struct {
		name        string
		req         *ValidationRequest
		wantSuccess bool
	}{
		{
			name: "valid cookie auth",
			req: &ValidationRequest{
				Name:       "test-site",
				BaseURL:    server.URL,
				AuthMethod: AuthMethodCookie,
				Cookie:     "valid-cookie",
			},
			wantSuccess: true,
		},
		{
			name: "invalid cookie auth",
			req: &ValidationRequest{
				Name:       "test-site",
				BaseURL:    server.URL,
				AuthMethod: AuthMethodCookie,
				Cookie:     "invalid-cookie",
			},
			wantSuccess: false,
		},
		{
			name: "valid api key auth",
			req: &ValidationRequest{
				Name:       "test-site",
				BaseURL:    server.URL,
				AuthMethod: AuthMethodAPIKey,
				APIKey:     "valid-api-key",
				APIURL:     server.URL,
			},
			wantSuccess: true,
		},
		{
			name: "missing required fields",
			req: &ValidationRequest{
				AuthMethod: AuthMethodCookie,
			},
			wantSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewSiteValidator()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			result := validator.Validate(ctx, tt.req)

			if result.Success != tt.wantSuccess {
				t.Errorf("Validate() success = %v, want %v, errors: %v", result.Success, tt.wantSuccess, result.Errors)
			}

			// Verify result completeness
			if result.Message == "" {
				t.Error("Validate() result.Message should not be empty")
			}
			if result.Errors == nil {
				t.Error("Validate() result.Errors should not be nil")
			}
		})
	}
}

// TestValidateConfig tests the ValidateConfig method
func TestValidateConfig(t *testing.T) {
	validator := NewSiteValidator()

	tests := []struct {
		name       string
		config     *mockDynamicSiteConfig
		wantErrors int
	}{
		{
			name: "valid cookie config",
			config: &mockDynamicSiteConfig{
				name:       "test",
				baseURL:    "https://example.com",
				authMethod: AuthMethodCookie,
				cookie:     "test-cookie",
			},
			wantErrors: 0,
		},
		{
			name: "missing cookie",
			config: &mockDynamicSiteConfig{
				name:       "test",
				baseURL:    "https://example.com",
				authMethod: AuthMethodCookie,
			},
			wantErrors: 1,
		},
		{
			name: "valid api key config",
			config: &mockDynamicSiteConfig{
				name:       "test",
				baseURL:    "https://example.com",
				authMethod: AuthMethodAPIKey,
				apiKey:     "test-key",
				apiURL:     "https://api.example.com",
			},
			wantErrors: 0,
		},
		{
			name: "missing api key and url",
			config: &mockDynamicSiteConfig{
				name:       "test",
				baseURL:    "https://example.com",
				authMethod: AuthMethodAPIKey,
			},
			wantErrors: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.ValidateConfig(tt.config)
			if len(errors) != tt.wantErrors {
				t.Errorf("ValidateConfig() got %d errors, want %d, errors: %v", len(errors), tt.wantErrors, errors)
			}
		})
	}
}

// mockDynamicSiteConfig implements site.DynamicSiteConfig for testing
type mockDynamicSiteConfig struct {
	name           string
	displayName    string
	baseURL        string
	authMethod     AuthMethod
	cookie         string
	apiKey         string
	apiURL         string
	enabled        bool
	downloaderName string
}

func (m *mockDynamicSiteConfig) GetName() string           { return m.name }
func (m *mockDynamicSiteConfig) GetDisplayName() string    { return m.displayName }
func (m *mockDynamicSiteConfig) GetBaseURL() string        { return m.baseURL }
func (m *mockDynamicSiteConfig) GetAuthMethod() AuthMethod { return m.authMethod }
func (m *mockDynamicSiteConfig) GetCookie() string         { return m.cookie }
func (m *mockDynamicSiteConfig) GetAPIKey() string         { return m.apiKey }
func (m *mockDynamicSiteConfig) GetAPIURL() string         { return m.apiURL }
func (m *mockDynamicSiteConfig) IsEnabled() bool           { return m.enabled }
func (m *mockDynamicSiteConfig) GetDownloaderName() string { return m.downloaderName }
func (m *mockDynamicSiteConfig) Validate() error           { return nil }

func TestWithTimeout(t *testing.T) {
	timeout := 10 * time.Second
	opt := WithTimeout(timeout)

	validator := NewSiteValidator()
	opt(validator)

	if validator.timeout != timeout {
		t.Errorf("WithTimeout() timeout = %v, want %v", validator.timeout, timeout)
	}
}

func TestWithSession(t *testing.T) {
	customSession := requests.NewSession().WithTimeout(5 * time.Second)
	defer func() { _ = customSession.Close() }()

	opt := WithSession(customSession)

	validator := NewSiteValidator()
	opt(validator)

	if validator.session != customSession {
		t.Error("WithSession() did not set the custom session")
	}
}

func TestNewSiteValidatorWithOptions(t *testing.T) {
	timeout := 15 * time.Second
	customSession := requests.NewSession().WithTimeout(20 * time.Second)
	defer func() { _ = customSession.Close() }()

	v1 := NewSiteValidatorWithOptions(WithTimeout(timeout))
	if v1.timeout != timeout {
		t.Errorf("NewSiteValidatorWithOptions() with timeout = %v, want %v", v1.timeout, timeout)
	}

	v2 := NewSiteValidatorWithOptions(
		WithTimeout(timeout),
		WithSession(customSession),
	)
	if v2.session != customSession {
		t.Error("NewSiteValidatorWithOptions() did not set custom session")
	}

	v3 := NewSiteValidatorWithOptions()
	if v3 == nil {
		t.Fatal("NewSiteValidatorWithOptions() should not return nil")
		return
	}
	if v3.timeout != 60*time.Second {
		t.Errorf("NewSiteValidatorWithOptions() default timeout = %v, want %v", v3.timeout, 60*time.Second)
	}
}

// TestFetchFreeTorrentsPreview 测试获取免费种子预览
func TestFetchFreeTorrentsPreview(t *testing.T) {
	// 创建模拟RSS服务器
	rssContent := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test RSS Feed</title>
    <item>
      <title>Torrent 1</title>
      <guid>12345</guid>
      <link>https://example.com/torrent/12345</link>
    </item>
    <item>
      <title>Torrent 2</title>
      <guid>67890</guid>
      <link>https://example.com/torrent/67890</link>
    </item>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(rssContent))
	}))
	defer server.Close()

	validator := NewSiteValidator()
	ctx := context.Background()

	req := &ValidationRequest{
		Name:       "test-site",
		BaseURL:    server.URL,
		AuthMethod: AuthMethodCookie,
		Cookie:     "test-cookie",
		RSSURL:     server.URL,
	}

	torrents, total, err := validator.fetchFreeTorrentsPreview(ctx, req)
	if err != nil {
		t.Fatalf("fetchFreeTorrentsPreview() error = %v", err)
	}

	if total != 2 {
		t.Errorf("fetchFreeTorrentsPreview() total = %d, want 2", total)
	}

	if len(torrents) != 2 {
		t.Errorf("fetchFreeTorrentsPreview() got %d torrents, want 2", len(torrents))
	}

	if torrents[0].Title != "Torrent 1" {
		t.Errorf("fetchFreeTorrentsPreview() first torrent title = %s, want 'Torrent 1'", torrents[0].Title)
	}
}

// TestFetchFreeTorrentsPreviewWithAPIKey 测试使用API Key获取RSS
func TestFetchFreeTorrentsPreviewWithAPIKey(t *testing.T) {
	rssContent := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test RSS Feed</title>
    <item>
      <title>API Torrent</title>
      <guid>11111</guid>
    </item>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("x-api-key")
		if apiKey != "test-api-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(rssContent))
	}))
	defer server.Close()

	validator := NewSiteValidator()
	ctx := context.Background()

	req := &ValidationRequest{
		Name:       "test-site",
		BaseURL:    server.URL,
		AuthMethod: AuthMethodAPIKey,
		APIKey:     "test-api-key",
		APIURL:     server.URL,
		RSSURL:     server.URL,
	}

	torrents, total, err := validator.fetchFreeTorrentsPreview(ctx, req)
	if err != nil {
		t.Fatalf("fetchFreeTorrentsPreview() error = %v", err)
	}

	if total != 1 {
		t.Errorf("fetchFreeTorrentsPreview() total = %d, want 1", total)
	}

	if len(torrents) != 1 {
		t.Errorf("fetchFreeTorrentsPreview() got %d torrents, want 1", len(torrents))
	}
}

// TestFetchFreeTorrentsPreviewError 测试RSS获取失败
func TestFetchFreeTorrentsPreviewError(t *testing.T) {
	// 创建返回错误的服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	validator := NewSiteValidator()
	ctx := context.Background()

	req := &ValidationRequest{
		Name:       "test-site",
		BaseURL:    server.URL,
		AuthMethod: AuthMethodCookie,
		Cookie:     "test-cookie",
		RSSURL:     server.URL,
	}

	_, _, err := validator.fetchFreeTorrentsPreview(ctx, req)
	if err == nil {
		t.Error("fetchFreeTorrentsPreview() should return error for 500 status")
	}
}

// TestFetchFreeTorrentsPreviewInvalidRSS 测试无效RSS解析
func TestFetchFreeTorrentsPreviewInvalidRSS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid rss content"))
	}))
	defer server.Close()

	validator := NewSiteValidator()
	ctx := context.Background()

	req := &ValidationRequest{
		Name:       "test-site",
		BaseURL:    server.URL,
		AuthMethod: AuthMethodCookie,
		Cookie:     "test-cookie",
		RSSURL:     server.URL,
	}

	_, _, err := validator.fetchFreeTorrentsPreview(ctx, req)
	if err == nil {
		t.Error("fetchFreeTorrentsPreview() should return error for invalid RSS")
	}
}

// TestFetchFreeTorrentsPreviewMaxItems 测试最大返回数量限制
func TestFetchFreeTorrentsPreviewMaxItems(t *testing.T) {
	// 创建包含15个项目的RSS
	rssContent := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test RSS Feed</title>
    <item><title>Torrent 1</title><guid>1</guid></item>
    <item><title>Torrent 2</title><guid>2</guid></item>
    <item><title>Torrent 3</title><guid>3</guid></item>
    <item><title>Torrent 4</title><guid>4</guid></item>
    <item><title>Torrent 5</title><guid>5</guid></item>
    <item><title>Torrent 6</title><guid>6</guid></item>
    <item><title>Torrent 7</title><guid>7</guid></item>
    <item><title>Torrent 8</title><guid>8</guid></item>
    <item><title>Torrent 9</title><guid>9</guid></item>
    <item><title>Torrent 10</title><guid>10</guid></item>
    <item><title>Torrent 11</title><guid>11</guid></item>
    <item><title>Torrent 12</title><guid>12</guid></item>
    <item><title>Torrent 13</title><guid>13</guid></item>
    <item><title>Torrent 14</title><guid>14</guid></item>
    <item><title>Torrent 15</title><guid>15</guid></item>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(rssContent))
	}))
	defer server.Close()

	validator := NewSiteValidator()
	ctx := context.Background()

	req := &ValidationRequest{
		Name:       "test-site",
		BaseURL:    server.URL,
		AuthMethod: AuthMethodCookie,
		Cookie:     "test-cookie",
		RSSURL:     server.URL,
	}

	torrents, total, err := validator.fetchFreeTorrentsPreview(ctx, req)
	if err != nil {
		t.Fatalf("fetchFreeTorrentsPreview() error = %v", err)
	}

	// 总数应该是15
	if total != 15 {
		t.Errorf("fetchFreeTorrentsPreview() total = %d, want 15", total)
	}

	// 但返回的预览应该最多10个
	if len(torrents) > 10 {
		t.Errorf("fetchFreeTorrentsPreview() returned %d torrents, should be max 10", len(torrents))
	}
}

// TestValidateURLFormat 测试URL格式验证
func TestValidateURLFormat(t *testing.T) {
	validator := NewSiteValidator()

	tests := []struct {
		name       string
		req        *ValidationRequest
		wantErrors int
	}{
		{
			name: "valid URLs",
			req: &ValidationRequest{
				BaseURL: "https://example.com",
				APIURL:  "https://api.example.com",
				RSSURL:  "https://example.com/rss",
			},
			wantErrors: 0,
		},
		{
			name: "empty URLs",
			req: &ValidationRequest{
				BaseURL: "",
				APIURL:  "",
				RSSURL:  "",
			},
			wantErrors: 0, // 空URL不会产生格式错误
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.validateURLFormat(tt.req)
			if len(errors) != tt.wantErrors {
				t.Errorf("validateURLFormat() got %d errors, want %d", len(errors), tt.wantErrors)
			}
		})
	}
}

// TestValidateRequiredFields 测试必填字段验证
func TestValidateRequiredFields(t *testing.T) {
	validator := NewSiteValidator()

	tests := []struct {
		name       string
		req        *ValidationRequest
		wantErrors int
	}{
		{
			name: "all fields present",
			req: &ValidationRequest{
				Name:       "test",
				BaseURL:    "https://example.com",
				AuthMethod: AuthMethodCookie,
			},
			wantErrors: 0,
		},
		{
			name: "missing name",
			req: &ValidationRequest{
				BaseURL:    "https://example.com",
				AuthMethod: AuthMethodCookie,
			},
			wantErrors: 1,
		},
		{
			name: "missing base_url",
			req: &ValidationRequest{
				Name:       "test",
				AuthMethod: AuthMethodCookie,
			},
			wantErrors: 1,
		},
		{
			name: "missing auth_method",
			req: &ValidationRequest{
				Name:    "test",
				BaseURL: "https://example.com",
			},
			wantErrors: 1,
		},
		{
			name:       "all fields missing",
			req:        &ValidationRequest{},
			wantErrors: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.validateRequiredFields(tt.req)
			if len(errors) != tt.wantErrors {
				t.Errorf("validateRequiredFields() got %d errors, want %d, errors: %v", len(errors), tt.wantErrors, errors)
			}
		})
	}
}

// TestValidateAuthFields 测试认证字段验证
func TestValidateAuthFields(t *testing.T) {
	validator := NewSiteValidator()

	tests := []struct {
		name       string
		req        *ValidationRequest
		wantErrors int
	}{
		{
			name: "valid cookie auth",
			req: &ValidationRequest{
				AuthMethod: AuthMethodCookie,
				Cookie:     "test-cookie",
			},
			wantErrors: 0,
		},
		{
			name: "cookie auth without cookie",
			req: &ValidationRequest{
				AuthMethod: AuthMethodCookie,
			},
			wantErrors: 1,
		},
		{
			name: "valid api key auth",
			req: &ValidationRequest{
				AuthMethod: AuthMethodAPIKey,
				APIKey:     "test-key",
				APIURL:     "https://api.example.com",
			},
			wantErrors: 0,
		},
		{
			name: "api key auth without key",
			req: &ValidationRequest{
				AuthMethod: AuthMethodAPIKey,
				APIURL:     "https://api.example.com",
			},
			wantErrors: 1,
		},
		{
			name: "api key auth without url",
			req: &ValidationRequest{
				AuthMethod: AuthMethodAPIKey,
				APIKey:     "test-key",
			},
			wantErrors: 1,
		},
		{
			name: "api key auth without both",
			req: &ValidationRequest{
				AuthMethod: AuthMethodAPIKey,
			},
			wantErrors: 2,
		},
		{
			name: "unsupported auth method",
			req: &ValidationRequest{
				AuthMethod: "unsupported",
			},
			wantErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.validateAuthFields(tt.req)
			if len(errors) != tt.wantErrors {
				t.Errorf("validateAuthFields() got %d errors, want %d, errors: %v", len(errors), tt.wantErrors, errors)
			}
		})
	}
}

// TestTestConnection 测试连接测试
func TestTestConnection(t *testing.T) {
	// 创建成功的服务器
	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer successServer.Close()

	// 创建401服务器
	authFailServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer authFailServer.Close()

	// 创建500服务器
	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer errorServer.Close()

	validator := NewSiteValidator()
	ctx := context.Background()

	tests := []struct {
		name    string
		req     *ValidationRequest
		wantErr bool
	}{
		{
			name: "successful cookie connection",
			req: &ValidationRequest{
				BaseURL:    successServer.URL,
				AuthMethod: AuthMethodCookie,
				Cookie:     "test-cookie",
			},
			wantErr: false,
		},
		{
			name: "successful api key connection",
			req: &ValidationRequest{
				AuthMethod: AuthMethodAPIKey,
				APIKey:     "test-key",
				APIURL:     successServer.URL,
			},
			wantErr: false,
		},
		{
			name: "auth failure",
			req: &ValidationRequest{
				BaseURL:    authFailServer.URL,
				AuthMethod: AuthMethodCookie,
				Cookie:     "test-cookie",
			},
			wantErr: true,
		},
		{
			name: "server error",
			req: &ValidationRequest{
				BaseURL:    errorServer.URL,
				AuthMethod: AuthMethodCookie,
				Cookie:     "test-cookie",
			},
			wantErr: true,
		},
		{
			name: "unsupported auth method",
			req: &ValidationRequest{
				BaseURL:    successServer.URL,
				AuthMethod: "unsupported",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.testConnection(ctx, tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("testConnection() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateWithRSSURL 测试带RSS URL的验证
func TestValidateWithRSSURL(t *testing.T) {
	// 创建成功的服务器
	rssContent := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test RSS</title>
    <item><title>Test</title><guid>1</guid></item>
  </channel>
</rss>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rss" {
			w.Header().Set("Content-Type", "application/rss+xml")
			w.Write([]byte(rssContent))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	validator := NewSiteValidator()
	ctx := context.Background()

	req := &ValidationRequest{
		Name:       "test-site",
		BaseURL:    server.URL,
		AuthMethod: AuthMethodCookie,
		Cookie:     "test-cookie",
		RSSURL:     server.URL + "/rss",
	}

	result := validator.Validate(ctx, req)

	if !result.Success {
		t.Errorf("Validate() success = false, errors: %v", result.Errors)
	}

	if result.TotalTorrents != 1 {
		t.Errorf("Validate() TotalTorrents = %d, want 1", result.TotalTorrents)
	}

	if len(result.FreeTorrents) != 1 {
		t.Errorf("Validate() FreeTorrents count = %d, want 1", len(result.FreeTorrents))
	}
}

// TestValidateWithRSSError 测试RSS获取失败的验证
func TestValidateWithRSSError(t *testing.T) {
	// 创建服务器 - 主页成功，RSS失败
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rss" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	validator := NewSiteValidator()
	ctx := context.Background()

	req := &ValidationRequest{
		Name:       "test-site",
		BaseURL:    server.URL,
		AuthMethod: AuthMethodCookie,
		Cookie:     "test-cookie",
		RSSURL:     server.URL + "/rss",
	}

	result := validator.Validate(ctx, req)

	// 验证应该部分成功（连接成功但RSS失败）
	if result.Success {
		t.Error("Validate() should not be fully successful when RSS fails")
	}

	// 应该有RSS错误
	hasRSSError := false
	for _, err := range result.Errors {
		if err.Field == "rss" {
			hasRSSError = true
			break
		}
	}
	if !hasRSSError {
		t.Error("Validate() should have RSS error")
	}
}

// TestValidateConfigWithMissingName 测试缺少名称的配置验证
func TestValidateConfigWithMissingName(t *testing.T) {
	validator := NewSiteValidator()

	config := &mockDynamicSiteConfig{
		name:       "",
		baseURL:    "https://example.com",
		authMethod: AuthMethodCookie,
		cookie:     "test-cookie",
	}

	errors := validator.ValidateConfig(config)

	if len(errors) != 1 {
		t.Errorf("ValidateConfig() got %d errors, want 1", len(errors))
	}

	if errors[0].Field != "name" {
		t.Errorf("ValidateConfig() error field = %s, want 'name'", errors[0].Field)
	}
}

// TestValidateConfigWithMissingBaseURL 测试缺少BaseURL的配置验证
func TestValidateConfigWithMissingBaseURL(t *testing.T) {
	validator := NewSiteValidator()

	config := &mockDynamicSiteConfig{
		name:       "test",
		baseURL:    "",
		authMethod: AuthMethodCookie,
		cookie:     "test-cookie",
	}

	errors := validator.ValidateConfig(config)

	if len(errors) != 1 {
		t.Errorf("ValidateConfig() got %d errors, want 1", len(errors))
	}

	if errors[0].Field != "base_url" {
		t.Errorf("ValidateConfig() error field = %s, want 'base_url'", errors[0].Field)
	}
}

// TestValidateConfigWithUnsupportedAuthMethod 测试不支持的认证方式
func TestValidateConfigWithUnsupportedAuthMethod(t *testing.T) {
	validator := NewSiteValidator()

	config := &mockDynamicSiteConfig{
		name:       "test",
		baseURL:    "https://example.com",
		authMethod: "unsupported",
	}

	errors := validator.ValidateConfig(config)

	hasAuthMethodError := false
	for _, err := range errors {
		if err.Field == "auth_method" {
			hasAuthMethodError = true
			break
		}
	}

	if !hasAuthMethodError {
		t.Error("ValidateConfig() should have auth_method error for unsupported method")
	}
}

func TestNewSiteValidator(t *testing.T) {
	validator := NewSiteValidator()

	if validator == nil {
		t.Fatal("NewSiteValidator() should not return nil")
		return
	}

	if validator.session == nil {
		t.Error("NewSiteValidator() session should not be nil")
	}

	if validator.timeout != 60*time.Second {
		t.Errorf("NewSiteValidator() timeout = %v, want %v", validator.timeout, 60*time.Second)
	}
}
