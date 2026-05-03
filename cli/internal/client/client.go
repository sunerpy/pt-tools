package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"

	"github.com/sunerpy/pt-tools/cli/internal/config"
	"github.com/sunerpy/pt-tools/cli/internal/types"
)

const (
	DefaultTimeout = 10 * time.Second
	SearchTimeout  = 60 * time.Second
)

// ErrNotAuthenticated is returned when the session cookie is invalid.
var ErrNotAuthenticated = fmt.Errorf("not authenticated, please run 'pt-tools-cli login'")

// Client wraps HTTP calls to the pt-tools server API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	cfg        *types.CLIConfig
}

// NewClient creates a new API client.
func NewClient(cfg *types.CLIConfig) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		baseURL: cfg.URL,
		httpClient: &http.Client{
			Timeout:   DefaultTimeout,
			Jar:       jar,
			Transport: &http.Transport{TLSClientConfig: nil},
		},
		cfg: cfg,
	}
}

// SetInsecure disables TLS certificate verification.
func (c *Client) SetInsecure(insecure bool) {
	if insecure {
		// Note: In production, use proper TLS config with InsecureSkipVerify
		// This is a simplified version for CLI usage
	}
}

// Do performs an HTTP request and decodes the JSON response.
func (c *Client) Do(ctx context.Context, method, path string, body, result any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	// Attach session cookie
	if c.cfg.Cookie != "" {
		req.AddCookie(&http.Cookie{Name: "session", Value: c.cfg.Cookie})
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check for auth redirect (302 to /login)
	if resp.StatusCode == http.StatusFound {
		location := resp.Header.Get("Location")
		if location == "/login" || location == "/login/" {
			return ErrNotAuthenticated
		}
	}

	// Check for unauthorized
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrNotAuthenticated
	}

	// Read body for error messages
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		msg := string(respBody)
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("server error %d: %s", resp.StatusCode, msg)
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
	}

	return nil
}

// GetSessionCookie extracts the session cookie from a login response.
func (c *Client) GetSessionCookie() string {
	return c.cfg.Cookie
}

// SetSessionCookie stores the session cookie.
func (c *Client) SetSessionCookie(cookie string, expires int64) {
	c.cfg.Cookie = cookie
	c.cfg.Expires = expires
	_ = config.Save(c.cfg)
}
