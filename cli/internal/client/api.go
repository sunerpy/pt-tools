package client

import (
	"context"
	"net/url"
	"strconv"

	"github.com/sunerpy/pt-tools/cli/internal/types"
)

// Search performs a multi-site search.
func (c *Client) Search(ctx context.Context, req *types.MultiSiteSearchRequest) (*types.MultiSiteSearchResponse, error) {
	var result types.MultiSiteSearchResponse
	if err := c.Do(ctx, "POST", "/api/v2/search/multi", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SearchSites returns the list of sites available for searching.
func (c *Client) SearchSites(ctx context.Context) ([]types.SearchSite, error) {
	var result []types.SearchSite
	if err := c.Do(ctx, "GET", "/api/v2/search/sites", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// PushTorrent pushes a single torrent to downloaders.
func (c *Client) PushTorrent(ctx context.Context, req *types.TorrentPushRequest) (*types.TorrentPushResponse, error) {
	var result types.TorrentPushResponse
	if err := c.Do(ctx, "POST", "/api/v2/torrents/push", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// BatchPushTorrents pushes multiple torrents to downloaders.
func (c *Client) BatchPushTorrents(ctx context.Context, req *types.BatchTorrentPushRequest) (*types.BatchTorrentPushResponse, error) {
	var result types.BatchTorrentPushResponse
	if err := c.Do(ctx, "POST", "/api/v2/torrents/batch-push", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListDownloaders returns all configured downloaders.
func (c *Client) ListDownloaders(ctx context.Context) ([]types.DownloaderResponse, error) {
	var result []types.DownloaderResponse
	if err := c.Do(ctx, "GET", "/api/downloaders", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListDownloaderTorrents returns torrents across all downloaders.
func (c *Client) ListDownloaderTorrents(ctx context.Context, search string, state string, downloaderID uint) ([]types.DownloaderTorrentItem, error) {
	v := url.Values{}
	if search != "" {
		v.Set("q", search)
	}
	if state != "" {
		v.Set("state", state)
	}
	if downloaderID > 0 {
		v.Set("downloaderId", strconv.Itoa(int(downloaderID)))
	}

	path := "/api/downloader-torrents"
	if len(v) > 0 {
		path += "?" + v.Encode()
	}

	var result []types.DownloaderTorrentItem
	if err := c.Do(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// DownloaderStats returns transfer statistics.
func (c *Client) DownloaderStats(ctx context.Context) (*types.DownloaderTransferStats, error) {
	var result types.DownloaderTransferStats
	if err := c.Do(ctx, "GET", "/api/downloader-torrents/transfer-stats", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListSites returns all configured sites.
func (c *Client) ListSites(ctx context.Context) (map[string]types.SiteConfigResponse, error) {
	var result map[string]types.SiteConfigResponse
	if err := c.Do(ctx, "GET", "/api/sites", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetSite returns a single site's configuration.
func (c *Client) GetSite(ctx context.Context, name string) (*types.SiteConfigResponse, error) {
	var result types.SiteConfigResponse
	if err := c.Do(ctx, "GET", "/api/sites/"+name, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteSite deletes a site.
func (c *Client) DeleteSite(ctx context.Context, name string) error {
	return c.Do(ctx, "DELETE", "/api/sites?name="+url.QueryEscape(name), nil, nil)
}

// ValidateSite validates a site's credentials.
func (c *Client) ValidateSite(ctx context.Context, name string) error {
	return c.Do(ctx, "POST", "/api/sites/validate", map[string]string{"name": name}, nil)
}

// ListTasks returns paginated tasks.
func (c *Client) ListTasks(ctx context.Context, site, q, sort string, downloaded, pushed, expired bool, page, pageSize int) (*types.TaskListResponse, error) {
	v := url.Values{}
	if site != "" {
		v.Set("site", site)
	}
	if q != "" {
		v.Set("q", q)
	}
	if sort != "" {
		v.Set("sort", sort)
	}
	if downloaded {
		v.Set("downloaded", "1")
	}
	if pushed {
		v.Set("pushed", "1")
	}
	if expired {
		v.Set("expired", "1")
	}
	if page > 0 {
		v.Set("page", strconv.Itoa(page))
	}
	if pageSize > 0 {
		v.Set("page_size", strconv.Itoa(pageSize))
	}

	path := "/api/tasks"
	if len(v) > 0 {
		path += "?" + v.Encode()
	}

	var result types.TaskListResponse
	if err := c.Do(ctx, "GET", path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// StartAllTasks starts all tasks.
func (c *Client) StartAllTasks(ctx context.Context) error {
	return c.Do(ctx, "POST", "/api/control/start", nil, nil)
}

// StopAllTasks stops all tasks.
func (c *Client) StopAllTasks(ctx context.Context) error {
	return c.Do(ctx, "POST", "/api/control/stop", nil, nil)
}

// GetLogs returns recent log lines.
func (c *Client) GetLogs(ctx context.Context) (*types.LogsResponse, error) {
	var result types.LogsResponse
	if err := c.Do(ctx, "GET", "/api/logs", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetVersion returns server version information.
func (c *Client) GetVersion(ctx context.Context) (*types.VersionResponse, error) {
	var result types.VersionResponse
	if err := c.Do(ctx, "GET", "/api/version", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetUserInfoAggregated returns aggregated user info.
func (c *Client) GetUserInfoAggregated(ctx context.Context) (*types.UserInfoAggregated, error) {
	var result types.UserInfoAggregated
	if err := c.Do(ctx, "GET", "/api/v2/userinfo/aggregated", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetGlobalSettings returns global configuration.
func (c *Client) GetGlobalSettings(ctx context.Context) (*types.GlobalSettings, error) {
	var result types.GlobalSettings
	if err := c.Do(ctx, "GET", "/api/global", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SaveGlobalSettings saves global configuration.
func (c *Client) SaveGlobalSettings(ctx context.Context, settings *types.GlobalSettings) error {
	return c.Do(ctx, "POST", "/api/global", settings, nil)
}

// GetQbitSettings returns qBittorrent settings.
func (c *Client) GetQbitSettings(ctx context.Context) (*types.QbitSettings, error) {
	var result types.QbitSettings
	if err := c.Do(ctx, "GET", "/api/qbit", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SaveQbitSettings saves qBittorrent settings.
func (c *Client) SaveQbitSettings(ctx context.Context, settings *types.QbitSettings) error {
	return c.Do(ctx, "POST", "/api/qbit", settings, nil)
}

// ListFilterRules returns filter rules.
func (c *Client) ListFilterRules(ctx context.Context) ([]types.FilterRule, error) {
	var result []types.FilterRule
	if err := c.Do(ctx, "GET", "/api/filter-rules", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}
