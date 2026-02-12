package validator

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/sunerpy/requests"

	"github.com/sunerpy/pt-tools/utils/httpclient"
)

// AuthMethod 认证方式
type AuthMethod string

const (
	// AuthMethodCookie Cookie 认证
	AuthMethodCookie AuthMethod = "cookie"
	// AuthMethodAPIKey API Key 认证
	AuthMethodAPIKey AuthMethod = "apikey"
)

// TorrentPreview 种子预览信息
type TorrentPreview struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// SiteBasicInfo 站点基本信息
type SiteBasicInfo struct {
	Name        string     `json:"name"`
	DisplayName string     `json:"display_name"`
	BaseURL     string     `json:"base_url"`
	AuthMethod  AuthMethod `json:"auth_method"`
}

// DynamicSiteConfig 动态站点配置接口
type DynamicSiteConfig interface {
	GetName() string
	GetBaseURL() string
	GetAuthMethod() AuthMethod
	GetCookie() string
	GetAPIKey() string
	GetAPIURL() string
}

// SiteValidator 站点验证器
// 用于验证新站点配置是否有效
type SiteValidator struct {
	session requests.Session
	timeout time.Duration
}

func NewSiteValidator() *SiteValidator {
	session := requests.NewSession().WithTimeout(30 * time.Second)
	return &SiteValidator{
		session: session,
		timeout: 60 * time.Second,
	}
}

type ValidatorOption func(*SiteValidator)

func WithTimeout(d time.Duration) ValidatorOption {
	return func(v *SiteValidator) {
		v.timeout = d
		v.session = v.session.WithTimeout(d)
	}
}

func WithSession(session requests.Session) ValidatorOption {
	return func(v *SiteValidator) {
		v.session = session
	}
}

func NewSiteValidatorWithOptions(opts ...ValidatorOption) *SiteValidator {
	v := NewSiteValidator()
	for _, opt := range opts {
		opt(v)
	}
	return v
}

func (v *SiteValidator) sessionWithProxy(targetURL string) requests.Session {
	if proxyURL := httpclient.ResolveProxyFromEnvironment(targetURL); proxyURL != "" {
		return v.session.Clone().(requests.Session).WithProxy(proxyURL)
	}
	return v.session
}

type ValidationRequest struct {
	Name        string     `json:"name"`
	DisplayName string     `json:"display_name"`
	BaseURL     string     `json:"base_url"`
	AuthMethod  AuthMethod `json:"auth_method"`
	Cookie      string     `json:"cookie,omitempty"`
	APIKey      string     `json:"api_key,omitempty"`
	APIURL      string     `json:"api_url,omitempty"`
	RSSURL      string     `json:"rss_url,omitempty"` // 用于验证的RSS URL
}

// ValidationError 验证错误
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationResult 验证结果
type ValidationResult struct {
	Success       bool              `json:"success"`
	Message       string            `json:"message"`
	Errors        []ValidationError `json:"errors,omitempty"`
	FreeTorrents  []TorrentPreview  `json:"free_torrents,omitempty"`
	TotalTorrents int               `json:"total_torrents"`
	SiteInfo      *SiteBasicInfo    `json:"site_info,omitempty"`
	ResponseTime  time.Duration     `json:"response_time"`
}

// Validate 验证站点配置
func (v *SiteValidator) Validate(ctx context.Context, req *ValidationRequest) *ValidationResult {
	startTime := time.Now()
	result := &ValidationResult{
		Success: false,
		Errors:  make([]ValidationError, 0),
	}

	// 1. 验证必填字段
	if err := v.validateRequiredFields(req); err != nil {
		result.Errors = append(result.Errors, err...)
		result.Message = "配置验证失败：缺少必填字段"
		result.ResponseTime = time.Since(startTime)
		return result
	}

	// 2. 验证URL格式
	if err := v.validateURLFormat(req); err != nil {
		result.Errors = append(result.Errors, err...)
		result.Message = "配置验证失败：URL格式无效"
		result.ResponseTime = time.Since(startTime)
		return result
	}

	// 3. 验证认证方式对应的字段
	if err := v.validateAuthFields(req); err != nil {
		result.Errors = append(result.Errors, err...)
		result.Message = "配置验证失败：认证信息不完整"
		result.ResponseTime = time.Since(startTime)
		return result
	}

	// 4. 测试连接
	if err := v.testConnection(ctx, req); err != nil {
		result.Errors = append(result.Errors, ValidationError{
			Field:   "connection",
			Message: err.Error(),
		})
		result.Message = "连接测试失败"
		result.ResponseTime = time.Since(startTime)
		return result
	}

	// 5. 如果提供了RSS URL，尝试获取免费种子预览
	if req.RSSURL != "" {
		torrents, total, err := v.fetchFreeTorrentsPreview(ctx, req)
		if err != nil {
			result.Errors = append(result.Errors, ValidationError{
				Field:   "rss",
				Message: fmt.Sprintf("RSS获取失败: %v", err),
			})
		} else {
			result.FreeTorrents = torrents
			result.TotalTorrents = total
		}
	}

	// 设置站点信息
	result.SiteInfo = &SiteBasicInfo{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		BaseURL:     req.BaseURL,
		AuthMethod:  req.AuthMethod,
	}

	result.Success = len(result.Errors) == 0
	if result.Success {
		result.Message = "站点配置验证成功"
	} else {
		result.Message = "站点配置验证部分失败"
	}
	result.ResponseTime = time.Since(startTime)

	return result
}

// validateRequiredFields 验证必填字段
func (v *SiteValidator) validateRequiredFields(req *ValidationRequest) []ValidationError {
	var errors []ValidationError

	if req.Name == "" {
		errors = append(errors, ValidationError{
			Field:   "name",
			Message: "站点名称不能为空",
		})
	}

	if req.BaseURL == "" {
		errors = append(errors, ValidationError{
			Field:   "base_url",
			Message: "站点URL不能为空",
		})
	}

	if req.AuthMethod == "" {
		errors = append(errors, ValidationError{
			Field:   "auth_method",
			Message: "认证方式不能为空",
		})
	}

	return errors
}

// validateURLFormat 验证URL格式
func (v *SiteValidator) validateURLFormat(req *ValidationRequest) []ValidationError {
	var errors []ValidationError

	if req.BaseURL != "" {
		if _, err := url.Parse(req.BaseURL); err != nil {
			errors = append(errors, ValidationError{
				Field:   "base_url",
				Message: fmt.Sprintf("无效的URL格式: %v", err),
			})
		}
	}

	if req.APIURL != "" {
		if _, err := url.Parse(req.APIURL); err != nil {
			errors = append(errors, ValidationError{
				Field:   "api_url",
				Message: fmt.Sprintf("无效的API URL格式: %v", err),
			})
		}
	}

	if req.RSSURL != "" {
		if _, err := url.Parse(req.RSSURL); err != nil {
			errors = append(errors, ValidationError{
				Field:   "rss_url",
				Message: fmt.Sprintf("无效的RSS URL格式: %v", err),
			})
		}
	}

	return errors
}

// validateAuthFields 验证认证字段
func (v *SiteValidator) validateAuthFields(req *ValidationRequest) []ValidationError {
	var errors []ValidationError

	switch req.AuthMethod {
	case AuthMethodCookie:
		if req.Cookie == "" {
			errors = append(errors, ValidationError{
				Field:   "cookie",
				Message: "Cookie认证方式需要提供Cookie",
			})
		}
	case AuthMethodAPIKey:
		if req.APIKey == "" {
			errors = append(errors, ValidationError{
				Field:   "api_key",
				Message: "API Key认证方式需要提供API Key",
			})
		}
		if req.APIURL == "" {
			errors = append(errors, ValidationError{
				Field:   "api_url",
				Message: "API Key认证方式需要提供API URL",
			})
		}
	default:
		errors = append(errors, ValidationError{
			Field:   "auth_method",
			Message: fmt.Sprintf("不支持的认证方式: %s", req.AuthMethod),
		})
	}

	return errors
}

// testConnection 测试连接
func (v *SiteValidator) testConnection(ctx context.Context, req *ValidationRequest) error {
	var testURL string
	var headers map[string]string

	switch req.AuthMethod {
	case AuthMethodCookie:
		testURL = req.BaseURL
		headers = map[string]string{
			"Cookie":     req.Cookie,
			"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		}
	case AuthMethodAPIKey:
		testURL = req.APIURL
		headers = map[string]string{
			"x-api-key":  req.APIKey,
			"User-Agent": "Mozilla/5.0",
		}
	default:
		return fmt.Errorf("unsupported auth method: %s", req.AuthMethod)
	}

	session := v.sessionWithProxy(testURL)

	r, err := requests.NewGet(testURL).Build()
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	for k, val := range headers {
		r.AddHeader(k, val)
	}

	resp, err := session.DoWithContext(ctx, r)
	if err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("认证失败，状态码: %d", resp.StatusCode)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("请求失败，状态码: %d", resp.StatusCode)
	}

	return nil
}

// fetchFreeTorrentsPreview 获取免费种子预览
func (v *SiteValidator) fetchFreeTorrentsPreview(ctx context.Context, req *ValidationRequest) ([]TorrentPreview, int, error) {
	session := v.sessionWithProxy(req.RSSURL)

	r, err := requests.NewGet(req.RSSURL).Build()
	if err != nil {
		return nil, 0, err
	}

	switch req.AuthMethod {
	case AuthMethodCookie:
		r.AddHeader("Cookie", req.Cookie)
	case AuthMethodAPIKey:
		r.AddHeader("x-api-key", req.APIKey)
	}
	r.AddHeader("User-Agent", "Mozilla/5.0")

	resp, err := session.DoWithContext(ctx, r)
	if err != nil {
		return nil, 0, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("RSS请求失败，状态码: %d", resp.StatusCode)
	}

	fp := gofeed.NewParser()
	feed, err := fp.Parse(bytes.NewReader(resp.Bytes()))
	if err != nil {
		return nil, 0, fmt.Errorf("RSS解析失败: %w", err)
	}

	var previews []TorrentPreview
	maxPreviews := 10
	for i, item := range feed.Items {
		if i >= maxPreviews {
			break
		}
		previews = append(previews, TorrentPreview{
			ID:    item.GUID,
			Title: item.Title,
		})
	}

	return previews, len(feed.Items), nil
}

// ValidateConfig 验证DynamicSiteConfig
func (v *SiteValidator) ValidateConfig(config DynamicSiteConfig) []ValidationError {
	var errors []ValidationError

	if config.GetName() == "" {
		errors = append(errors, ValidationError{
			Field:   "name",
			Message: "站点名称不能为空",
		})
	}

	if config.GetBaseURL() == "" {
		errors = append(errors, ValidationError{
			Field:   "base_url",
			Message: "站点URL不能为空",
		})
	}

	switch config.GetAuthMethod() {
	case AuthMethodCookie:
		if config.GetCookie() == "" {
			errors = append(errors, ValidationError{
				Field:   "cookie",
				Message: "Cookie认证方式需要提供Cookie",
			})
		}
	case AuthMethodAPIKey:
		if config.GetAPIKey() == "" {
			errors = append(errors, ValidationError{
				Field:   "api_key",
				Message: "API Key认证方式需要提供API Key",
			})
		}
		if config.GetAPIURL() == "" {
			errors = append(errors, ValidationError{
				Field:   "api_url",
				Message: "API Key认证方式需要提供API URL",
			})
		}
	default:
		errors = append(errors, ValidationError{
			Field:   "auth_method",
			Message: fmt.Sprintf("不支持的认证方式: %s", config.GetAuthMethod()),
		})
	}

	return errors
}
