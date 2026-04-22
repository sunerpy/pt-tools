package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sunerpy/pt-tools/cli/internal/config"
)

// Login authenticates with the pt-tools server and caches the session cookie.
func (c *Client) Login(username, password string) error {
	body := map[string]string{
		"username": username,
		"password": password,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal login request: %w", err)
	}

	req, err := http.NewRequest("POST", c.baseURL+"/login", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read body first for error messages
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		msg := string(respBody)
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("login failed: %s", msg)
	}

	// Extract session cookie
	var sessionCookie string
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "session" {
			sessionCookie = cookie.Value
			break
		}
	}

	if sessionCookie == "" {
		return fmt.Errorf("no session cookie returned from server")
	}

	// Calculate cookie expiry (assume 24 hours)
	expires := time.Now().Add(24 * time.Hour).Unix()

	// Save to config
	c.cfg.Cookie = sessionCookie
	c.cfg.Expires = expires
	c.cfg.Username = username

	if err := config.Save(c.cfg); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	return nil
}

// Ping checks server health and authentication status.
func (c *Client) Ping() error {
	resp, err := c.httpClient.Get(c.baseURL + "/api/ping")
	if err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ping failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// IsAuthenticated checks if the client has a valid session.
func (c *Client) IsAuthenticated() bool {
	return config.IsCookieValid(c.cfg)
}

// Logout clears the cached session cookie.
func (c *Client) Logout() error {
	c.cfg.Cookie = ""
	c.cfg.Expires = 0
	return config.Save(c.cfg)
}
