package connector

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

// JellyfinConnector implements core.MediaServerConnector for Jellyfin servers.
// It reuses baseClient which handles shared HTTP logic (both Jellyfin and Emby
// APIs are ~95% compatible due to Jellyfin forking from Emby).
type JellyfinConnector struct {
	base baseClient
}

// NewJellyfinConnector constructs a Jellyfin connector.
// baseURL: server address (with scheme, e.g., "http://localhost:8096")
// apiKey: X-Emby-Token header value (empty for public endpoints)
// client: *http.Client to use (nil defaults to http.DefaultClient)
func NewJellyfinConnector(baseURL, apiKey string, client *http.Client) *JellyfinConnector {
	if client == nil {
		client = http.DefaultClient
	}
	return &JellyfinConnector{
		base: baseClient{
			BaseURL: baseURL,
			APIKey:  apiKey,
			Client:  client,
			Product: "Jellyfin Server",
		},
	}
}

// Name returns the connector type identifier.
func (c *JellyfinConnector) Name() string {
	return "jellyfin"
}

// Ping tests connectivity via GET /System/Info/Public (no auth required).
// Returns server metadata (product name, version, server ID).
func (c *JellyfinConnector) Ping(ctx context.Context) (*core.ServerInfo, error) {
	var info PublicInfo
	if err := c.base.do(ctx, "GET", "/System/Info/Public", nil, &info); err != nil {
		return nil, err
	}
	return &core.ServerInfo{
		Product:  info.ProductName,
		Version:  info.Version,
		ServerID: info.Id,
		Name:     info.ServerName,
	}, nil
}

// Authenticate verifies API key validity via GET /System/Info (requires auth).
// Returns server metadata if authentication succeeds.
func (c *JellyfinConnector) Authenticate(ctx context.Context) (*core.ServerInfo, error) {
	var info PublicInfo
	if err := c.base.do(ctx, "GET", "/System/Info", nil, &info); err != nil {
		return nil, err
	}
	return &core.ServerInfo{
		Product:  info.ProductName,
		Version:  info.Version,
		ServerID: info.Id,
		Name:     info.ServerName,
	}, nil
}

// ListLibraries retrieves all media libraries via GET /Library/MediaFolders.
func (c *JellyfinConnector) ListLibraries(ctx context.Context) ([]core.Library, error) {
	var resp MediaFoldersResponse
	if err := c.base.do(ctx, "GET", "/Library/MediaFolders", nil, &resp); err != nil {
		return nil, err
	}
	out := make([]core.Library, 0, len(resp.Items))
	for _, item := range resp.Items {
		paths := []string{}
		if item.Path != "" {
			paths = append(paths, item.Path)
		}
		out = append(out, core.Library{
			ID:             item.Id,
			Name:           item.Name,
			CollectionType: item.CollectionType,
			Paths:          paths,
		})
	}
	return out, nil
}

// RefreshLibrary triggers a library scan via POST.
// If libraryID is empty, performs a global refresh of all libraries.
// If libraryID is specified, refreshes that specific library with recursive flag.
func (c *JellyfinConnector) RefreshLibrary(ctx context.Context, libraryID string) error {
	path := "/Library/Refresh"
	if libraryID != "" {
		path = fmt.Sprintf("/Items/%s/Refresh?MetadataRefreshMode=Default&Recursive=true", libraryID)
	}
	return c.base.do(ctx, "POST", path, nil, nil)
}

// ScanStatus returns the current library refresh/scan status.
// Queries GET /ScheduledTasks and finds the "RefreshLibrary" task.
func (c *JellyfinConnector) ScanStatus(ctx context.Context) (*core.ScanStatus, error) {
	var tasks []ScheduledTask
	if err := c.base.do(ctx, "GET", "/ScheduledTasks", nil, &tasks); err != nil {
		return nil, err
	}
	for _, t := range tasks {
		if t.Key == "RefreshLibrary" {
			return &core.ScanStatus{
				Running:  strings.EqualFold(t.State, "Running"),
				Percent:  t.CurrentProgressPercentage,
				TaskName: t.Name,
			}, nil
		}
	}
	// If no refresh task found, report idle status
	return &core.ScanStatus{Running: false}, nil
}
