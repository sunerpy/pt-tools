package qbit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

var (
	_ downloader.TorrentExporter   = (*QbitClient)(nil)
	_ downloader.TrackerEditor     = (*QbitClient)(nil)
	_ downloader.TrackerReader     = (*QbitClient)(nil)
	_ downloader.TorrentTagRemover = (*QbitClient)(nil)
	_ downloader.CategoryCreator   = (*QbitClient)(nil)
)

// RemoveTorrentTags 从这些种子上去掉标签（tags 逗号分隔；为空时去掉全部），别的种子不动。
func (q *QbitClient) RemoveTorrentTags(ids []string, tags string) error {
	data := url.Values{}
	data.Set("hashes", strings.Join(ids, "|"))
	data.Set("tags", tags)
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.postForm("/api/v2/torrents/removeTags", data)
}

// CreateCategory 建分类。qB 在分类已经有了（或名字不合法）时回 409：已经有了是常见情况，不算失败。
func (q *QbitClient) CreateCategory(name, savePath string) error {
	data := url.Values{}
	data.Set("category", name)
	data.Set("savePath", savePath)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.baseURL+"/api/v2/torrents/createCategory", strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	q.mu.Lock()
	defer q.mu.Unlock()
	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("建分类失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return nil
	}
	if !q.isSuccessStatus(resp.StatusCode) {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("建分类失败: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// GetTorrentTrackersContext 读取种子的 tracker 列表；请求受 ctx 约束，ctx 取消时一起取消。
func (q *QbitClient) GetTorrentTrackersContext(ctx context.Context, id string) ([]downloader.TorrentTracker, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, q.baseURL+"/api/v2/torrents/trackers?hash="+url.QueryEscape(id), nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	resp, err := q.doRequestWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("读取 tracker 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w：%s", downloader.ErrTorrentNotFound, id)
	}
	if !q.isSuccessStatus(resp.StatusCode) {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("读取 tracker 失败: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var items []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("解析 tracker 失败: %w", err)
	}
	return parseQbitTrackers(items), nil
}

// maxExportedTorrentBytes 是导出种子文件的大小上限；PT 种子文件通常只有几十 KB 到几 MB。
const maxExportedTorrentBytes = 32 << 20

// ExportTorrent 经 torrents/export 导出种子文件（qBittorrent 4.5 起）。导出内容会核对 info hash，
// 与请求的种子不符时报错，避免把别的种子当成它。
func (q *QbitClient) ExportTorrent(ctx context.Context, hash string) ([]byte, error) {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return nil, fmt.Errorf("种子 hash 不能为空")
	}
	q.versionMu.RLock()
	version := q.appVersion
	q.versionMu.RUnlock()
	if major, minor, _, ok := parseQBitVersion(version); ok && (major < 4 || (major == 4 && minor < 5)) {
		return nil, fmt.Errorf("%w：导出种子需要 qBittorrent 4.5 或更新的版本（当前 %s）", downloader.ErrCapabilityUnsupported, version)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, q.baseURL+"/api/v2/torrents/export?hash="+url.QueryEscape(hash), nil)
	if err != nil {
		return nil, fmt.Errorf("创建导出请求失败: %w", err)
	}
	resp, err := q.doRequestWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("导出种子失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w：%s（qBittorrent 4.5 以前的版本也会这样回应）", downloader.ErrTorrentNotFound, hash)
	}
	if !q.isSuccessStatus(resp.StatusCode) {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("导出种子失败: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxExportedTorrentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("读取导出的种子失败: %w", err)
	}
	if len(data) > maxExportedTorrentBytes {
		return nil, fmt.Errorf("导出的种子文件超过 %d MB", maxExportedTorrentBytes>>20)
	}
	got, err := ComputeTorrentHash(data)
	if err != nil {
		return nil, fmt.Errorf("导出的内容不是种子文件: %w", err)
	}
	if !strings.EqualFold(got, hash) {
		return nil, fmt.Errorf("导出的种子（%s）与请求的种子不符（%s）", got, hash)
	}
	return data, nil
}

// EditTracker 经 torrents/editTracker 把种子的 oldURL 改成 newURL。
// qBittorrent 在原地址不存在或新地址已在种子里时回 409，统一报 ErrTrackerNotFound。
func (q *QbitClient) EditTracker(ctx context.Context, hash, oldURL, newURL string) error {
	hash, oldURL, newURL = strings.TrimSpace(hash), strings.TrimSpace(oldURL), strings.TrimSpace(newURL)
	if hash == "" || oldURL == "" || newURL == "" {
		return fmt.Errorf("种子 hash、原地址和新地址都不能为空")
	}
	form := url.Values{"hash": {hash}, "origUrl": {oldURL}, "newUrl": {newURL}}
	resp, err := q.postFormContext(ctx, "/api/v2/torrents/editTracker", form)
	if err != nil {
		return fmt.Errorf("修改 tracker 失败: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusConflict:
		return fmt.Errorf("%w（或新地址已经在种子里）", downloader.ErrTrackerNotFound)
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%w：%s", downloader.ErrTorrentNotFound, hash)
	case !q.isSuccessStatus(resp.StatusCode):
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("修改 tracker 失败: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// postFormContext 发一个受 ctx 约束的表单 POST；会话过期（403）时重新登录后重发一次。
// 调用方负责关闭返回的 Body。
func (q *QbitClient) postFormContext(ctx context.Context, endpoint string, form url.Values) (*http.Response, error) {
	if q.client == nil {
		return nil, fmt.Errorf("client is closed")
	}
	send := func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.baseURL+endpoint, strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return q.client.Do(req)
	}
	resp, err := send()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		if authErr := q.AuthenticateWithContext(ctx); authErr != nil {
			return nil, fmt.Errorf("re-authentication failed: %w", authErr)
		}
		if resp, err = send(); err != nil {
			return nil, err
		}
	}
	if q.isSuccessStatus(resp.StatusCode) {
		q.mu.Lock()
		q.lastActivity = time.Now()
		q.mu.Unlock()
	}
	return resp, nil
}
