// Package v2 provides HTTP client utilities based on github.com/sunerpy/requests
package v2

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sunerpy/requests"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/utils/httpclient"
)

// HTTPClientConfig holds configuration for HTTP clients
type HTTPClientConfig struct {
	// Timeout is the request timeout
	Timeout time.Duration
	// MaxIdleConns is the maximum number of idle connections
	MaxIdleConns int
	// MaxIdleConnsPerHost is the maximum idle connections per host
	MaxIdleConnsPerHost int
	// IdleConnTimeout is the idle connection timeout
	IdleConnTimeout time.Duration
	// DisableKeepAlives disables HTTP keep-alives
	DisableKeepAlives bool
}

// DefaultHTTPClientConfig returns default HTTP client configuration
func DefaultHTTPClientConfig() HTTPClientConfig {
	return HTTPClientConfig{
		Timeout:             30 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
	}
}

// ============================================================================
// Unified HTTP Client Interface using requests library
// ============================================================================

// SiteHTTPClient provides a unified HTTP client interface for site drivers
// using the requests library instead of net/http directly
type SiteHTTPClient struct {
	session   requests.Session
	userAgent string
	proxyURL  string
	timeout   time.Duration
	idleTime  time.Duration
	maxIdle   int
	keepAlive bool
	logger    *zap.Logger
}

// SiteHTTPClientConfig holds configuration for SiteHTTPClient
type SiteHTTPClientConfig struct {
	Timeout           time.Duration
	MaxIdleConns      int
	IdleConnTimeout   time.Duration
	DisableKeepAlives bool
	ProxyURL          string
	UserAgent         string
	Logger            *zap.Logger
}

// DefaultSiteHTTPClientConfig returns default configuration
func DefaultSiteHTTPClientConfig() SiteHTTPClientConfig {
	return SiteHTTPClientConfig{
		Timeout:           30 * time.Second,
		MaxIdleConns:      100,
		IdleConnTimeout:   90 * time.Second,
		DisableKeepAlives: false,
		UserAgent:         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
	}
}

// NewSiteHTTPClient creates a new SiteHTTPClient
func NewSiteHTTPClient(config SiteHTTPClientConfig) *SiteHTTPClient {
	if config.Logger == nil {
		config.Logger = zap.NewNop()
	}

	session := requests.NewSession().
		WithTimeout(config.Timeout).
		WithIdleTimeout(config.IdleConnTimeout).
		WithMaxIdleConns(config.MaxIdleConns).
		WithKeepAlive(!config.DisableKeepAlives)

	if strings.TrimSpace(config.ProxyURL) != "" {
		session = session.WithProxy(strings.TrimSpace(config.ProxyURL))
	}

	return &SiteHTTPClient{
		session:   session,
		userAgent: config.UserAgent,
		proxyURL:  strings.TrimSpace(config.ProxyURL),
		timeout:   config.Timeout,
		idleTime:  config.IdleConnTimeout,
		maxIdle:   config.MaxIdleConns,
		keepAlive: !config.DisableKeepAlives,
		logger:    config.Logger,
	}
}

// HTTPResponse wraps the response from requests library
type HTTPResponse struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

// IsSuccess returns true if status code is 2xx
func (r *HTTPResponse) IsSuccess() bool {
	return r.StatusCode >= 200 && r.StatusCode < 300
}

// IsError returns true if status code is 4xx or 5xx
func (r *HTTPResponse) IsError() bool {
	return r.StatusCode >= 400
}

// DoRequest performs an HTTP request using the requests library
func (c *SiteHTTPClient) DoRequest(ctx context.Context, method, url string, body io.Reader, headers map[string]string) (*HTTPResponse, error) {
	var builder *requests.RequestBuilder
	switch method {
	case http.MethodGet:
		builder = requests.NewGet(url)
	case http.MethodPost:
		builder = requests.NewPost(url)
	case http.MethodPut:
		builder = requests.NewPut(url)
	case http.MethodDelete:
		builder = requests.NewDeleteBuilder(url)
	case http.MethodPatch:
		builder = requests.NewPatch(url)
	default:
		return nil, fmt.Errorf("unsupported HTTP method: %s", method)
	}

	if body != nil && method != http.MethodGet && method != http.MethodDelete {
		builder = builder.WithBody(body)
	}

	req, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("build request failed: %w", err)
	}

	req.AddHeader("User-Agent", c.userAgent)
	for k, v := range headers {
		req.AddHeader(k, v)
	}

	activeSession := c.session
	if c.proxyURL == "" {
		envProxyURL := httpclient.ResolveProxyFromEnvironment(url)
		if envProxyURL != "" {
			activeSession = requests.NewSession().
				WithTimeout(c.timeout).
				WithIdleTimeout(c.idleTime).
				WithMaxIdleConns(c.maxIdle).
				WithKeepAlive(c.keepAlive).
				WithProxy(envProxyURL)
			defer func() { _ = activeSession.Close() }()
		}
	}

	resp, err := activeSession.DoWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &HTTPResponse{
		StatusCode: resp.StatusCode,
		Body:       resp.Bytes(),
		Headers:    resp.Headers,
	}, nil
}

// Get performs a GET request
func (c *SiteHTTPClient) Get(ctx context.Context, url string, headers map[string]string) (*HTTPResponse, error) {
	return c.DoRequest(ctx, http.MethodGet, url, nil, headers)
}

// Post performs a POST request with body
func (c *SiteHTTPClient) Post(ctx context.Context, url string, body []byte, headers map[string]string) (*HTTPResponse, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	return c.DoRequest(ctx, http.MethodPost, url, reader, headers)
}

// PostJSON performs a POST request with JSON body
func (c *SiteHTTPClient) PostJSON(ctx context.Context, url string, body []byte, headers map[string]string) (*HTTPResponse, error) {
	if headers == nil {
		headers = make(map[string]string)
	}
	headers["Content-Type"] = "application/json"
	headers["Accept"] = "application/json"
	return c.Post(ctx, url, body, headers)
}

// Close closes the underlying session
func (c *SiteHTTPClient) Close() error {
	return c.session.Close()
}

type HTTPClientPool struct {
	sessions map[string]requests.Session
	mu       sync.RWMutex
	config   HTTPClientConfig
	logger   *zap.Logger
}

func NewHTTPClientPool(config HTTPClientConfig, logger *zap.Logger) *HTTPClientPool {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &HTTPClientPool{
		sessions: make(map[string]requests.Session),
		config:   config,
		logger:   logger,
	}
}

// GetSession returns a requests.Session for the given site ID
func (p *HTTPClientPool) GetSession(siteID string) requests.Session {
	p.mu.RLock()
	session, ok := p.sessions[siteID]
	p.mu.RUnlock()

	if ok {
		return session
	}

	// Create new session
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if session, ok = p.sessions[siteID]; ok {
		return session
	}

	session = requests.NewSession().
		WithTimeout(p.config.Timeout).
		WithIdleTimeout(p.config.IdleConnTimeout).
		WithMaxIdleConns(p.config.MaxIdleConns).
		WithKeepAlive(!p.config.DisableKeepAlives)

	p.sessions[siteID] = session
	p.logger.Debug("Created HTTP session", zap.String("site", siteID))
	return session
}

func (p *HTTPClientPool) CloseClient(siteID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if session, ok := p.sessions[siteID]; ok {
		_ = session.Close()
		delete(p.sessions, siteID)
	}
	p.logger.Debug("Closed HTTP client", zap.String("site", siteID))
}

func (p *HTTPClientPool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for siteID, session := range p.sessions {
		_ = session.Close()
		delete(p.sessions, siteID)
		p.logger.Debug("Closed HTTP client", zap.String("site", siteID))
	}
}

// RetryConfig holds configuration for retry logic
type RetryConfig struct {
	// MaxRetries is the maximum number of retries
	MaxRetries int
	// InitialBackoff is the initial backoff duration
	InitialBackoff time.Duration
	// MaxBackoff is the maximum backoff duration
	MaxBackoff time.Duration
	// BackoffMultiplier is the backoff multiplier
	BackoffMultiplier float64
	// Jitter adds randomness to backoff
	Jitter bool
	// RetryableStatusCodes are HTTP status codes that should trigger a retry
	RetryableStatusCodes []int
}

// DefaultRetryConfig returns default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:        3,
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        30 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            true,
		RetryableStatusCodes: []int{
			http.StatusTooManyRequests,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout,
			http.StatusBadGateway,
		},
	}
}

// RetryableHTTPClient wraps an HTTP client with retry logic
type RetryableHTTPClient struct {
	client *http.Client
	config RetryConfig
	logger *zap.Logger
}

// NewRetryableHTTPClient creates a new retryable HTTP client
func NewRetryableHTTPClient(client *http.Client, config RetryConfig, logger *zap.Logger) *RetryableHTTPClient {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RetryableHTTPClient{
		client: client,
		config: config,
		logger: logger,
	}
}

// Do executes an HTTP request with retry logic
func (c *RetryableHTTPClient) Do(req *http.Request) (*http.Response, error) {
	var lastErr error
	var lastResp *http.Response

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := c.calculateBackoff(attempt)
			c.logger.Debug("Retrying request",
				zap.Int("attempt", attempt),
				zap.Duration("backoff", backoff),
				zap.String("url", req.URL.String()),
			)

			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(backoff):
			}
		}

		// Clone request for retry (body needs to be re-readable)
		reqClone := req.Clone(req.Context())
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, fmt.Errorf("get request body: %w", err)
			}
			reqClone.Body = body
		}

		resp, err := c.client.Do(reqClone)
		if err != nil {
			lastErr = err
			c.logger.Warn("Request failed",
				zap.Int("attempt", attempt),
				zap.Error(err),
			)
			continue
		}

		// Check if we should retry based on status code
		if c.shouldRetry(resp.StatusCode) {
			lastResp = resp
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
			c.logger.Warn("Retryable status code",
				zap.Int("attempt", attempt),
				zap.Int("status", resp.StatusCode),
			)
			// Close body to allow connection reuse
			_, _ = resp.Body.Read(make([]byte, 1024))
			resp.Body.Close()
			continue
		}

		return resp, nil
	}

	if lastResp != nil {
		return lastResp, lastErr
	}
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// calculateBackoff calculates the backoff duration for a given attempt
func (c *RetryableHTTPClient) calculateBackoff(attempt int) time.Duration {
	backoff := float64(c.config.InitialBackoff) * math.Pow(c.config.BackoffMultiplier, float64(attempt-1))

	if backoff > float64(c.config.MaxBackoff) {
		backoff = float64(c.config.MaxBackoff)
	}

	if c.config.Jitter {
		// Add up to 25% jitter
		jitter := backoff * 0.25 * rand.Float64()
		backoff += jitter
	}

	return time.Duration(backoff)
}

// shouldRetry checks if the status code should trigger a retry
func (c *RetryableHTTPClient) shouldRetry(statusCode int) bool {
	return slices.Contains(c.config.RetryableStatusCodes, statusCode)
}

// SessionManager manages authentication sessions for sites
type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
	logger   *zap.Logger
}

// Session represents an authentication session
type Session struct {
	SiteID     string
	Cookie     string
	APIKey     string
	SessionID  string // For Transmission
	ExpiresAt  time.Time
	LastUsed   time.Time
	LoginCount int
	mu         sync.RWMutex
}

// NewSessionManager creates a new session manager
func NewSessionManager(logger *zap.Logger) *SessionManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SessionManager{
		sessions: make(map[string]*Session),
		logger:   logger,
	}
}

// GetSession returns the session for a site
func (m *SessionManager) GetSession(siteID string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.sessions[siteID]
	return session, ok
}

// SetSession sets the session for a site
func (m *SessionManager) SetSession(siteID string, session *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session.SiteID = siteID
	session.LastUsed = time.Now()
	m.sessions[siteID] = session
	m.logger.Debug("Session set", zap.String("site", siteID))
}

// UpdateSessionID updates the session ID (for Transmission)
func (m *SessionManager) UpdateSessionID(siteID, sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session, ok := m.sessions[siteID]; ok {
		session.mu.Lock()
		session.SessionID = sessionID
		session.LastUsed = time.Now()
		session.mu.Unlock()
		m.logger.Debug("Session ID updated", zap.String("site", siteID))
	}
}

// InvalidateSession marks a session as expired
func (m *SessionManager) InvalidateSession(siteID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session, ok := m.sessions[siteID]; ok {
		session.mu.Lock()
		session.ExpiresAt = time.Now()
		session.mu.Unlock()
		m.logger.Debug("Session invalidated", zap.String("site", siteID))
	}
}

// RemoveSession removes a session
func (m *SessionManager) RemoveSession(siteID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, siteID)
	m.logger.Debug("Session removed", zap.String("site", siteID))
}

// IsSessionValid checks if a session is still valid
func (m *SessionManager) IsSessionValid(siteID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, ok := m.sessions[siteID]
	if !ok {
		return false
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	if !session.ExpiresAt.IsZero() && time.Now().After(session.ExpiresAt) {
		return false
	}

	return session.Cookie != "" || session.APIKey != "" || session.SessionID != ""
}

// IncrementLoginCount increments the login count for a session
func (m *SessionManager) IncrementLoginCount(siteID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session, ok := m.sessions[siteID]; ok {
		session.mu.Lock()
		session.LoginCount++
		count := session.LoginCount
		session.mu.Unlock()
		return count
	}
	return 0
}

// ResetLoginCount resets the login count for a session
func (m *SessionManager) ResetLoginCount(siteID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session, ok := m.sessions[siteID]; ok {
		session.mu.Lock()
		session.LoginCount = 0
		session.mu.Unlock()
	}
}

// HandleQBittorrentAuthWithRequests handles QBittorrent 403 responses using requests library
func HandleQBittorrentAuthWithRequests(ctx context.Context, baseURL, username, password string) (string, error) {
	// QBittorrent login endpoint
	loginURL := baseURL + "/api/v2/auth/login"

	opts := []requests.RequestOption{
		requests.WithContext(ctx),
		requests.WithQueryParams(map[string]string{
			"username": username,
			"password": password,
		}),
	}

	resp, err := requests.Post(loginURL, nil, opts...)
	if err != nil {
		return "", fmt.Errorf("login request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed: HTTP %d", resp.StatusCode)
	}

	// Extract SID cookie from response
	for _, cookie := range resp.Cookies {
		if cookie.Name == "SID" {
			return cookie.Value, nil
		}
	}

	return "", fmt.Errorf("no SID cookie in response")
}

// HandleTransmissionSession handles Transmission 409 responses by extracting session ID
func HandleTransmissionSession(resp *http.Response) string {
	return resp.Header.Get("X-Transmission-Session-Id")
}

// RequestsClient provides a high-level HTTP client using requests library
type RequestsClient struct {
	session requests.Session
	config  RetryConfig
	logger  *zap.Logger
}

// RequestsResponse wraps the response from requests library
type RequestsResponse struct {
	statusCode int
	body       []byte
	headers    http.Header
}

// StatusCode returns the HTTP status code
func (r *RequestsResponse) StatusCode() int { return r.statusCode }

// Bytes returns the response body as bytes
func (r *RequestsResponse) Bytes() []byte { return r.body }

// Text returns the response body as string
func (r *RequestsResponse) Text() string { return string(r.body) }

// Headers returns the response headers
func (r *RequestsResponse) Headers() http.Header { return r.headers }

// IsSuccess returns true if status code is 2xx
func (r *RequestsResponse) IsSuccess() bool { return r.statusCode >= 200 && r.statusCode < 300 }

// IsError returns true if status code is 4xx or 5xx
func (r *RequestsResponse) IsError() bool { return r.statusCode >= 400 }

// NewRequestsClient creates a new requests-based HTTP client
func NewRequestsClient(config HTTPClientConfig, retryConfig RetryConfig, logger *zap.Logger) *RequestsClient {
	if logger == nil {
		logger = zap.NewNop()
	}

	session := requests.NewSession().
		WithTimeout(config.Timeout).
		WithIdleTimeout(config.IdleConnTimeout).
		WithMaxIdleConns(config.MaxIdleConns).
		WithKeepAlive(!config.DisableKeepAlives)

	return &RequestsClient{
		session: session,
		config:  retryConfig,
		logger:  logger,
	}
}

// Get performs a GET request with retry logic
func (c *RequestsClient) Get(ctx context.Context, url string, opts ...requests.RequestOption) (*RequestsResponse, error) {
	opts = append(opts, requests.WithContext(ctx))
	return c.doWithRetry(func() (*RequestsResponse, error) {
		resp, err := requests.Get(url, opts...)
		if err != nil {
			return nil, err
		}
		return &RequestsResponse{
			statusCode: resp.StatusCode,
			body:       resp.Bytes(),
			headers:    resp.Headers,
		}, nil
	})
}

// Post performs a POST request with retry logic
func (c *RequestsClient) Post(ctx context.Context, url string, body any, opts ...requests.RequestOption) (*RequestsResponse, error) {
	opts = append(opts, requests.WithContext(ctx))
	return c.doWithRetry(func() (*RequestsResponse, error) {
		resp, err := requests.Post(url, body, opts...)
		if err != nil {
			return nil, err
		}
		return &RequestsResponse{
			statusCode: resp.StatusCode,
			body:       resp.Bytes(),
			headers:    resp.Headers,
		}, nil
	})
}

// doWithRetry executes a request function with retry logic
func (c *RequestsClient) doWithRetry(fn func() (*RequestsResponse, error)) (*RequestsResponse, error) {
	var lastErr error
	var lastResp *RequestsResponse

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := c.calculateBackoff(attempt)
			c.logger.Debug("Retrying request", zap.Int("attempt", attempt), zap.Duration("backoff", backoff))
			time.Sleep(backoff)
		}

		resp, err := fn()
		if err != nil {
			lastErr = err
			c.logger.Warn("Request failed", zap.Int("attempt", attempt), zap.Error(err))
			continue
		}

		// Check if we should retry based on status code
		if slices.Contains(c.config.RetryableStatusCodes, resp.statusCode) {
			lastResp = resp
			lastErr = fmt.Errorf("HTTP %d: %s", resp.statusCode, http.StatusText(resp.statusCode))
			c.logger.Warn("Retryable status code", zap.Int("attempt", attempt), zap.Int("status", resp.statusCode))
			continue
		}

		return resp, nil
	}

	if lastResp != nil {
		return lastResp, lastErr
	}
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// calculateBackoff calculates the backoff duration for a given attempt
func (c *RequestsClient) calculateBackoff(attempt int) time.Duration {
	backoff := float64(c.config.InitialBackoff) * math.Pow(c.config.BackoffMultiplier, float64(attempt-1))

	if backoff > float64(c.config.MaxBackoff) {
		backoff = float64(c.config.MaxBackoff)
	}

	if c.config.Jitter {
		jitter := backoff * 0.25 * rand.Float64()
		backoff += jitter
	}

	return time.Duration(backoff)
}

// Close closes the underlying session
func (c *RequestsClient) Close() error {
	return c.session.Close()
}
