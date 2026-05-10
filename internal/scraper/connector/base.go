// Package connector provides shared HTTP client logic for Jellyfin and Emby
// media servers. Their REST APIs overlap ~95% because Jellyfin forked from Emby,
// so a single baseClient is reused by both concrete connectors.
package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

// baseClient is the shared HTTP client for Jellyfin/Emby connectors.
// It is unexported on purpose — consumers must go through the public
// JellyfinConnector / EmbyConnector wrappers.
type baseClient struct {
	BaseURL string
	APIKey  string
	Client  *http.Client
	Product string
}

// do sends an HTTP request and decodes the JSON response into out.
// The X-Emby-Token header is set automatically when APIKey is non-empty
// (both Jellyfin and Emby accept this header). 4xx/5xx responses are
// mapped onto the core sentinel errors.
func (c *baseClient) do(ctx context.Context, method, path string, body, out any) error {
	url := strings.TrimRight(c.BaseURL, "/") + path

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return core.Wrap(err, "marshal request")
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return core.Wrap(err, "build request")
	}

	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.APIKey != "" {
		req.Header.Set("X-Emby-Token", c.APIKey)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return core.Wrap(err, "http do")
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusAccepted:
	case http.StatusUnauthorized, http.StatusForbidden:
		return core.Wrap(core.ErrUnauthorized, fmt.Sprintf("%s %s: %d", method, path, resp.StatusCode))
	case http.StatusNotFound:
		return core.Wrap(core.ErrNotFound, fmt.Sprintf("%s %s", method, path))
	default:
		if resp.StatusCode >= 500 {
			return core.Wrap(core.ErrProviderDown, fmt.Sprintf("%s %s: %d", method, path, resp.StatusCode))
		}
		return fmt.Errorf("unexpected status %d for %s %s", resp.StatusCode, method, path)
	}

	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return core.Wrap(err, "decode response")
	}
	return nil
}
