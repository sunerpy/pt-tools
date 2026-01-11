package v2

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/sunerpy/requests"
	"go.uber.org/zap"
)

// Failover errors
var (
	ErrAllURLsFailed    = errors.New("all URLs failed")
	ErrNoURLsConfigured = errors.New("no URLs configured for site")
)

// URLFailoverConfig configures multi-URL failover behavior
type URLFailoverConfig struct {
	// BaseURLs is the list of base URLs to try in order
	BaseURLs []string
	// RetryDelay is the delay between retries on the same URL
	RetryDelay time.Duration
	// MaxRetries is the maximum number of retries per URL (0 = no retry, just try once)
	MaxRetries int
	// Timeout is the timeout for each request
	Timeout time.Duration
}

// DefaultFailoverConfig returns a default failover configuration
func DefaultFailoverConfig(baseURLs []string) URLFailoverConfig {
	return URLFailoverConfig{
		BaseURLs:   baseURLs,
		RetryDelay: 500 * time.Millisecond,
		MaxRetries: 2, // Retry up to 2 times on transient errors
		Timeout:    30 * time.Second,
	}
}

// URLFailoverManager manages multi-URL failover for site requests
type URLFailoverManager struct {
	config     URLFailoverConfig
	currentIdx int
	mu         sync.RWMutex
	logger     *zap.Logger
}

// NewURLFailoverManager creates a new URL failover manager
func NewURLFailoverManager(config URLFailoverConfig, logger *zap.Logger) *URLFailoverManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &URLFailoverManager{
		config:     config,
		currentIdx: 0,
		logger:     logger,
	}
}

// GetCurrentURL returns the currently active base URL
func (m *URLFailoverManager) GetCurrentURL() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.config.BaseURLs) == 0 {
		return ""
	}
	return m.config.BaseURLs[m.currentIdx]
}

// GetAllURLs returns all configured base URLs
func (m *URLFailoverManager) GetAllURLs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config.BaseURLs
}

// ExecuteWithFailover executes a function with automatic URL failover
// The execFunc receives the base URL and should return an error if the request fails
// Returns the error from the last attempted URL if all URLs fail
func (m *URLFailoverManager) ExecuteWithFailover(
	ctx context.Context,
	execFunc func(baseURL string) error,
) error {
	m.mu.RLock()
	urls := m.config.BaseURLs
	startIdx := m.currentIdx
	maxRetries := m.config.MaxRetries
	retryDelay := m.config.RetryDelay
	m.mu.RUnlock()

	if len(urls) == 0 {
		return ErrNoURLsConfigured
	}

	var lastErr error
	urlCount := len(urls)

	// Try each URL starting from current index
	for i := 0; i < urlCount; i++ {
		idx := (startIdx + i) % urlCount
		baseURL := urls[idx]

		// Try this URL with retries
		for retry := 0; retry <= maxRetries; retry++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if retry > 0 {
				m.logger.Debug("Retrying URL",
					zap.String("url", baseURL),
					zap.Int("retry", retry),
				)
				time.Sleep(retryDelay)
			}

			err := execFunc(baseURL)
			if err == nil {
				// Success - update current index if we switched URLs
				if idx != startIdx {
					m.mu.Lock()
					m.currentIdx = idx
					m.mu.Unlock()
					m.logger.Info("Switched to new base URL",
						zap.String("url", baseURL),
						zap.Int("index", idx),
					)
				}
				return nil
			}

			lastErr = err
			m.logger.Warn("URL request failed",
				zap.String("url", baseURL),
				zap.Int("retry", retry),
				zap.Error(err),
			)

			// Check if error is retryable
			if !isRetryableError(err) {
				break // Don't retry non-retryable errors
			}
		}

		m.logger.Debug("Moving to next URL",
			zap.String("failedURL", baseURL),
			zap.Int("nextIndex", (idx+1)%urlCount),
		)
	}

	return fmt.Errorf("%w: %v", ErrAllURLsFailed, lastErr)
}

// isRetryableError checks if an error is retryable (network errors, timeouts)
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	// Context errors are not retryable
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// Authentication errors are not retryable
	if errors.Is(err, ErrInvalidCredentials) || errors.Is(err, ErrSessionExpired) || errors.Is(err, Err2FARequired) {
		return false
	}
	// Rate limit errors are not retryable on the same URL
	if errors.Is(err, ErrRateLimited) {
		return false
	}
	// All other errors (network, timeout, etc.) are retryable
	return true
}

// FailoverOption is a functional option for FailoverHTTPClient
type FailoverOption func(*FailoverHTTPClient)

// WithUserAgent sets the User-Agent header
func WithUserAgent(ua string) FailoverOption {
	return func(c *FailoverHTTPClient) {
		c.userAgent = ua
	}
}

// WithLogger sets the logger
func WithLogger(logger *zap.Logger) FailoverOption {
	return func(c *FailoverHTTPClient) {
		c.logger = logger
	}
}

// FailoverHTTPClient is a generic HTTP client with automatic URL failover
// All site drivers can use this client for making HTTP requests
// Uses requests library instead of net/http directly
type FailoverHTTPClient struct {
	manager   *URLFailoverManager
	session   requests.Session
	userAgent string
	logger    *zap.Logger
}

// NewFailoverHTTPClient creates a new failover HTTP client
func NewFailoverHTTPClient(config URLFailoverConfig, opts ...FailoverOption) *FailoverHTTPClient {
	logger := zap.NewNop()

	session := requests.NewSession().
		WithTimeout(config.Timeout).
		WithIdleTimeout(30 * time.Second).
		WithMaxIdleConns(10).
		WithKeepAlive(false)

	c := &FailoverHTTPClient{
		manager:   NewURLFailoverManager(config, logger),
		session:   session,
		userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		logger:    logger,
	}

	for _, opt := range opts {
		opt(c)
	}

	// Update manager's logger if changed
	c.manager.logger = c.logger

	return c
}

// GetCurrentBaseURL returns the currently active base URL
func (c *FailoverHTTPClient) GetCurrentBaseURL() string {
	return c.manager.GetCurrentURL()
}

// Get performs a GET request with automatic URL failover
func (c *FailoverHTTPClient) Get(ctx context.Context, path string, headers map[string]string) (*HTTPResponse, error) {
	var resp *HTTPResponse
	err := c.manager.ExecuteWithFailover(ctx, func(baseURL string) error {
		fullURL := baseURL + path

		opts := []requests.RequestOption{
			requests.WithContext(ctx),
			requests.WithHeader("User-Agent", c.userAgent),
		}
		for k, v := range headers {
			opts = append(opts, requests.WithHeader(k, v))
		}

		r, err := requests.Get(fullURL, opts...)
		if err != nil {
			return fmt.Errorf("execute request: %w", err)
		}

		// Check for server errors that should trigger failover
		if r.StatusCode >= 500 {
			return fmt.Errorf("server error: HTTP %d", r.StatusCode)
		}

		resp = &HTTPResponse{
			StatusCode: r.StatusCode,
			Body:       r.Bytes(),
			Headers:    r.Headers,
		}
		return nil
	})

	return resp, err
}

// Post performs a POST request with automatic URL failover
func (c *FailoverHTTPClient) Post(ctx context.Context, path string, body []byte, headers map[string]string) (*HTTPResponse, error) {
	var resp *HTTPResponse

	err := c.manager.ExecuteWithFailover(ctx, func(baseURL string) error {
		fullURL := baseURL + path

		opts := []requests.RequestOption{
			requests.WithContext(ctx),
			requests.WithHeader("User-Agent", c.userAgent),
		}
		for k, v := range headers {
			opts = append(opts, requests.WithHeader(k, v))
		}

		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}

		r, err := requests.Post(fullURL, bodyReader, opts...)
		if err != nil {
			return fmt.Errorf("execute request: %w", err)
		}

		// Check for server errors that should trigger failover
		if r.StatusCode >= 500 {
			return fmt.Errorf("server error: HTTP %d", r.StatusCode)
		}

		resp = &HTTPResponse{
			StatusCode: r.StatusCode,
			Body:       r.Bytes(),
			Headers:    r.Headers,
		}
		return nil
	})

	return resp, err
}

// Do performs a custom request with automatic URL failover
// Note: The request URL should be a path (e.g., "/api/search"), not a full URL
func (c *FailoverHTTPClient) Do(ctx context.Context, method, path string, body []byte, headers map[string]string) (*HTTPResponse, error) {
	var resp *HTTPResponse

	err := c.manager.ExecuteWithFailover(ctx, func(baseURL string) error {
		fullURL := baseURL + path

		opts := []requests.RequestOption{
			requests.WithContext(ctx),
			requests.WithHeader("User-Agent", c.userAgent),
		}
		for k, v := range headers {
			opts = append(opts, requests.WithHeader(k, v))
		}

		var r *requests.Response
		var err error
		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}

		switch method {
		case http.MethodGet:
			r, err = requests.Get(fullURL, opts...)
		case http.MethodPost:
			r, err = requests.Post(fullURL, bodyReader, opts...)
		case http.MethodPut:
			r, err = requests.Put(fullURL, bodyReader, opts...)
		case http.MethodDelete:
			r, err = requests.Delete(fullURL, opts...)
		case http.MethodPatch:
			r, err = requests.Patch(fullURL, bodyReader, opts...)
		default:
			return fmt.Errorf("unsupported HTTP method: %s", method)
		}

		if err != nil {
			return fmt.Errorf("execute request: %w", err)
		}

		// Check for server errors that should trigger failover
		if r.StatusCode >= 500 {
			return fmt.Errorf("server error: HTTP %d", r.StatusCode)
		}

		resp = &HTTPResponse{
			StatusCode: r.StatusCode,
			Body:       r.Bytes(),
			Headers:    r.Headers,
		}
		return nil
	})

	return resp, err
}
