package transmission

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

// TransmissionClient Transmission 客户端实现
type TransmissionClient struct {
	name         string
	baseURL      string
	username     string
	password     string
	autoStart    bool
	client       downloader.HTTPDoer
	sessionID    string
	mu           sync.Mutex
	healthy      bool
	lastActivity time.Time
}

// 确保 TransmissionClient 实现 Downloader 接口
var _ downloader.Downloader = (*TransmissionClient)(nil)

// Transmission RPC 请求/响应结构
type rpcRequest struct {
	Method    string `json:"method"`
	Arguments any    `json:"arguments,omitempty"`
	Tag       int    `json:"tag,omitempty"`
}

type rpcResponse struct {
	Result    string          `json:"result"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Tag       int             `json:"tag,omitempty"`
}

type sessionStatsResponse struct {
	DownloadDir string `json:"download-dir"`
}

type freeSpaceArgs struct {
	Path string `json:"path"`
}

type freeSpaceResponse struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size-bytes"`
}

type torrentAddArgs struct {
	Metainfo    string   `json:"metainfo"` // base64 encoded torrent file
	DownloadDir string   `json:"download-dir,omitempty"`
	Paused      bool     `json:"paused"`
	Labels      []string `json:"labels,omitempty"`
}

type torrentAddResponse struct {
	TorrentAdded     *torrentInfo `json:"torrent-added,omitempty"`
	TorrentDuplicate *torrentInfo `json:"torrent-duplicate,omitempty"`
}

type torrentStartArgs struct {
	IDs []any `json:"ids"`
}

type torrentInfo struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	HashString string `json:"hashString"`
}

type torrentGetArgs struct {
	IDs    []any    `json:"ids,omitempty"`
	Fields []string `json:"fields"`
}

type torrentGetResponse struct {
	Torrents []torrentStatus `json:"torrents"`
}

type torrentStatus struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	HashString string `json:"hashString"`
	Status     int    `json:"status"` // 0=stopped, 1=check wait, 2=check, 3=download wait, 4=download, 5=seed wait, 6=seed
}

// NewTransmissionClient 创建新的 Transmission 客户端
func NewTransmissionClient(config downloader.DownloaderConfig, name string) (downloader.Downloader, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	client := &TransmissionClient{
		name:      name,
		baseURL:   config.GetURL(),
		username:  config.GetUsername(),
		password:  config.GetPassword(),
		autoStart: config.GetAutoStart(),
		client:    downloader.NewRequestsHTTPDoer(config.GetURL(), 30*time.Second),
		healthy:   false,
	}

	if err := client.Authenticate(); err != nil {
		return nil, err
	}

	return client, nil
}

// GetType 获取下载器类型
func (t *TransmissionClient) GetType() downloader.DownloaderType {
	return downloader.DownloaderTransmission
}

// GetName 获取下载器实例名称
func (t *TransmissionClient) GetName() string {
	return t.name
}

// IsHealthy 检查下载器是否健康可用
func (t *TransmissionClient) IsHealthy() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.healthy
}

// Close 关闭下载器连接
func (t *TransmissionClient) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.healthy = false
	if closer, ok := t.client.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
	return nil
}

// Authenticate 认证连接到 Transmission
func (t *TransmissionClient) Authenticate() error {
	// Transmission 使用 session ID 进行认证
	// 首次请求会返回 409 和 X-Transmission-Session-Id header
	req, err := t.createRequest("session-get", nil)
	if err != nil {
		return fmt.Errorf("创建认证请求失败: %w", err)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		t.healthy = false
		return t.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	// 409 表示需要获取 session ID
	if resp.StatusCode == http.StatusConflict {
		t.sessionID = resp.Header.Get("X-Transmission-Session-Id")
		if t.sessionID == "" {
			t.healthy = false
			return fmt.Errorf("无法获取 Session ID，请检查 Transmission 版本")
		}
		// 使用新的 session ID 重试
		return t.verifyConnection()
	}

	if resp.StatusCode == http.StatusUnauthorized {
		t.healthy = false
		return fmt.Errorf("用户名或密码错误")
	}

	if resp.StatusCode != http.StatusOK {
		t.healthy = false
		return t.wrapStatusCodeError(resp.StatusCode)
	}

	t.healthy = true
	t.lastActivity = time.Now()
	sLogger().Info("Successfully connected to Transmission")
	return nil
}

func (t *TransmissionClient) wrapConnectionError(err error) error {
	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "connection refused"):
		return fmt.Errorf("连接被拒绝，请检查: 1) Transmission 是否正在运行 2) RPC 是否已启用 3) 端口是否正确(默认9091) (原始错误: %w)", err)
	case strings.Contains(errStr, "no such host"):
		return fmt.Errorf("无法解析主机名，请检查 URL 地址是否正确 (原始错误: %w)", err)
	case strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded"):
		return fmt.Errorf("连接超时，请检查: 1) 网络是否可达 2) 防火墙设置 3) URL 地址是否正确 (原始错误: %w)", err)
	case strings.Contains(errStr, "certificate"):
		return fmt.Errorf("SSL 证书错误，如使用自签名证书请检查配置 (原始错误: %w)", err)
	default:
		return fmt.Errorf("连接失败: %w", err)
	}
}

func (t *TransmissionClient) wrapStatusCodeError(statusCode int) error {
	switch statusCode {
	case http.StatusForbidden:
		return fmt.Errorf("访问被禁止(403)，请检查 Transmission 的 RPC 白名单设置")
	case http.StatusNotFound:
		return fmt.Errorf("RPC 路径不存在(404)，请检查 URL 是否正确(通常为 http://host:9091/transmission/rpc)")
	default:
		return fmt.Errorf("认证失败，HTTP 状态码: %d", statusCode)
	}
}

// verifyConnection 验证连接
func (t *TransmissionClient) verifyConnection() error {
	req, err := t.createRequest("session-get", nil)
	if err != nil {
		return err
	}

	resp, err := t.client.Do(req)
	if err != nil {
		t.healthy = false
		return fmt.Errorf("verification request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		t.healthy = false
		return fmt.Errorf("authentication failed: invalid username or password")
	}

	if resp.StatusCode != http.StatusOK {
		t.healthy = false
		return fmt.Errorf("verification failed with status code: %d", resp.StatusCode)
	}

	t.healthy = true
	t.lastActivity = time.Now()
	sLogger().Info("Successfully authenticated with Transmission")
	return nil
}

// createRequest 创建 RPC 请求
func (t *TransmissionClient) createRequest(method string, args any) (*http.Request, error) {
	return t.createRequestWithContext(context.Background(), method, args)
}

// createRequestWithContext 带 context 的创建 RPC 请求
func (t *TransmissionClient) createRequestWithContext(ctx context.Context, method string, args any) (*http.Request, error) {
	rpcReq := rpcRequest{
		Method:    method,
		Arguments: args,
	}

	body, err := json.Marshal(rpcReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	rpcURL := fmt.Sprintf("%s/transmission/rpc", t.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", rpcURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if t.sessionID != "" {
		req.Header.Set("X-Transmission-Session-Id", t.sessionID)
	}
	if t.username != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(t.username + ":" + t.password))
		req.Header.Set("Authorization", "Basic "+auth)
	}

	return req, nil
}

// doRequest 执行 RPC 请求
func (t *TransmissionClient) doRequest(method string, args any) (*rpcResponse, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.client == nil {
		return nil, fmt.Errorf("client is closed")
	}

	req, err := t.createRequest(method, args)
	if err != nil {
		return nil, err
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// 处理 session ID 过期
	if resp.StatusCode == http.StatusConflict {
		t.sessionID = resp.Header.Get("X-Transmission-Session-Id")
		// 重试请求
		req, err = t.createRequest(method, args)
		if err != nil {
			return nil, err
		}
		resp, err = t.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("retry request failed: %w", err)
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var rpcResp rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if rpcResp.Result != "success" {
		return nil, fmt.Errorf("RPC error: %s", rpcResp.Result)
	}

	t.lastActivity = time.Now()

	return &rpcResp, nil
}

// GetDiskSpace 获取可用磁盘空间
func (t *TransmissionClient) GetDiskSpace(ctx context.Context) (int64, error) {
	// 首先获取下载目录
	resp, err := t.doRequest("session-get", nil)
	if err != nil {
		return 0, fmt.Errorf("failed to get session info: %w", err)
	}

	var sessionStats sessionStatsResponse
	if unmarshalErr := json.Unmarshal(resp.Arguments, &sessionStats); unmarshalErr != nil {
		return 0, fmt.Errorf("failed to parse session info: %w", unmarshalErr)
	}

	downloadDir := sessionStats.DownloadDir
	if downloadDir == "" {
		// 如果没有配置下载目录，使用默认路径
		downloadDir = "/downloads"
		sLogger().Warnf("No download directory configured in Transmission, using default: %s", downloadDir)
	}

	// 获取下载目录的可用空间
	freeSpaceResp, err := t.doRequest("free-space", freeSpaceArgs{Path: downloadDir})
	if err != nil {
		// 如果获取磁盘空间失败，记录警告但返回一个大值以允许继续
		// 这样可以避免因为路径问题导致无法添加种子
		sLogger().Warnf("Failed to get free space for path %s: %v, assuming sufficient space", downloadDir, err)
		// 返回 100GB 作为默认值，让种子可以继续添加
		return 100 * 1024 * 1024 * 1024, nil
	}

	var freeSpace freeSpaceResponse
	if err := json.Unmarshal(freeSpaceResp.Arguments, &freeSpace); err != nil {
		return 0, fmt.Errorf("failed to parse free space: %w", err)
	}

	// 如果返回的空间为 0 或负数，可能是路径问题
	if freeSpace.SizeBytes <= 0 {
		sLogger().Warnf("Free space returned %d bytes for path %s, assuming sufficient space", freeSpace.SizeBytes, downloadDir)
		return 100 * 1024 * 1024 * 1024, nil
	}

	return freeSpace.SizeBytes, nil
}

// CanAddTorrent 检查是否可以添加指定大小的种子
func (t *TransmissionClient) CanAddTorrent(ctx context.Context, fileSize int64) (bool, error) {
	freeSpace, err := t.GetDiskSpace(ctx)
	if err != nil {
		return false, err
	}

	if fileSize > freeSpace {
		availableSize := float64(freeSpace) / (1024 * 1024 * 1024)
		needSize := float64(fileSize) / (1024 * 1024 * 1024)
		sLogger().Errorf("Insufficient space, need: %.2fGB, available: %.2fGB", needSize, availableSize)
		return false, nil
	}
	return true, nil
}

// AddTorrent 添加种子到 Transmission
func (t *TransmissionClient) AddTorrent(fileData []byte, category, tags string) error {
	return t.AddTorrentWithPath(fileData, category, tags, "")
}

// AddTorrentWithPath 添加种子到 Transmission 并指定下载路径
func (t *TransmissionClient) AddTorrentWithPath(fileData []byte, category, tags, downloadPath string) error {
	// Transmission 使用 base64 编码的种子文件
	metainfo := base64.StdEncoding.EncodeToString(fileData)

	args := torrentAddArgs{
		Metainfo: metainfo,
		Paused:   !t.autoStart, // autoStart=true 时 Paused=false，autoStart=false 时 Paused=true
	}

	// 设置自定义下载路径（download-dir）
	if downloadPath != "" {
		args.DownloadDir = downloadPath
		sLogger().Infof("[Transmission] 设置下载路径: %s", downloadPath)
	} else {
		sLogger().Info("[Transmission] 未指定下载路径，使用默认路径")
	}

	// Transmission 使用 labels 代替 category/tags
	if tags != "" {
		args.Labels = []string{tags}
	}
	if category != "" {
		args.Labels = append(args.Labels, category)
	}

	resp, err := t.doRequest("torrent-add", args)
	if err != nil {
		return fmt.Errorf("failed to add torrent: %w", err)
	}

	var addResp torrentAddResponse
	if err := json.Unmarshal(resp.Arguments, &addResp); err != nil {
		return fmt.Errorf("failed to parse add response: %w", err)
	}

	if addResp.TorrentDuplicate != nil {
		sLogger().Infof("Torrent already exists: %s", addResp.TorrentDuplicate.Name)
		// 对于已存在的种子，也检查并确保启动状态
		if err := t.EnsureTorrentStarted(addResp.TorrentDuplicate.HashString); err != nil {
			sLogger().Warnf("Failed to ensure torrent started %s: %v", addResp.TorrentDuplicate.Name, err)
		}
		return nil
	}

	if addResp.TorrentAdded != nil {
		sLogger().Infof("Torrent added: %s", addResp.TorrentAdded.Name)

		// 添加种子后，等待一小段时间让transmission处理，然后检查启动状态
		time.Sleep(500 * time.Millisecond)
		if err := t.EnsureTorrentStarted(addResp.TorrentAdded.HashString); err != nil {
			sLogger().Warnf("Failed to ensure torrent started %s: %v", addResp.TorrentAdded.Name, err)
		}
	}

	return nil
}

// CheckTorrentExists 检查种子是否存在
func (t *TransmissionClient) CheckTorrentExists(torrentHash string) (bool, error) {
	args := torrentGetArgs{
		IDs:    []any{torrentHash},
		Fields: []string{"id", "name", "hashString"},
	}

	resp, err := t.doRequest("torrent-get", args)
	if err != nil {
		return false, fmt.Errorf("failed to get torrent: %w", err)
	}

	var getResp torrentGetResponse
	if err := json.Unmarshal(resp.Arguments, &getResp); err != nil {
		return false, fmt.Errorf("failed to parse get response: %w", err)
	}

	// 检查是否找到匹配的种子
	for _, torrent := range getResp.Torrents {
		if torrent.HashString == torrentHash {
			sLogger().Infof("Torrent exists: %s", torrent.Name)
			return true, nil
		}
	}

	sLogger().Infof("Torrent %s not found in Transmission", torrentHash)
	return false, nil
}

// EnsureTorrentStarted 确保种子已启动（如果配置了自动启动）
// Deprecated: 此方法已不在接口中，保留仅为内部使用
func (t *TransmissionClient) EnsureTorrentStarted(torrentHash string) error {
	// 如果没有配置自动启动，直接返回
	if !t.autoStart {
		return nil
	}

	// 获取种子状态
	args := torrentGetArgs{
		IDs:    []any{torrentHash},
		Fields: []string{"id", "name", "hashString", "status"},
	}

	resp, err := t.doRequest("torrent-get", args)
	if err != nil {
		return fmt.Errorf("failed to get torrent status: %w", err)
	}

	var getResp torrentGetResponse
	if err := json.Unmarshal(resp.Arguments, &getResp); err != nil {
		return fmt.Errorf("failed to parse torrent status: %w", err)
	}

	// 检查种子是否存在
	if len(getResp.Torrents) == 0 {
		return fmt.Errorf("torrent %s not found", torrentHash)
	}

	torrent := getResp.Torrents[0]

	// 检查种子状态，0=stopped 表示暂停状态
	if torrent.Status == 0 {
		sLogger().Infof("Torrent %s is stopped, starting it...", torrent.Name)

		// 启动种子
		startArgs := torrentStartArgs{
			IDs: []any{torrent.ID},
		}

		_, err := t.doRequest("torrent-start", startArgs)
		if err != nil {
			return fmt.Errorf("failed to start torrent %s: %w", torrent.Name, err)
		}

		sLogger().Infof("Torrent %s started successfully", torrent.Name)
	} else {
		sLogger().Debugf("Torrent %s is already running (status: %d)", torrent.Name, torrent.Status)
	}

	return nil
}

// ============ 新接口实现 ============

// Ping 检查下载器连接是否正常
func (t *TransmissionClient) Ping() (bool, error) {
	resp, err := t.doRequest("session-get", nil)
	if err != nil {
		t.healthy = false
		return false, err
	}
	if resp.Result != "success" {
		t.healthy = false
		return false, nil
	}
	t.mu.Lock()
	t.healthy = true
	t.lastActivity = time.Now()
	t.mu.Unlock()
	return true, nil
}

// GetClientVersion 获取下载器版本
func (t *TransmissionClient) GetClientVersion() (string, error) {
	resp, err := t.doRequest("session-get", nil)
	if err != nil {
		return "", fmt.Errorf("failed to get session info: %w", err)
	}

	var sessionInfo struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(resp.Arguments, &sessionInfo); err != nil {
		return "", fmt.Errorf("failed to parse session info: %w", err)
	}

	return sessionInfo.Version, nil
}

// GetClientStatus 获取下载器状态
func (t *TransmissionClient) GetClientStatus() (downloader.ClientStatus, error) {
	resp, err := t.doRequest("session-stats", nil)
	if err != nil {
		return downloader.ClientStatus{}, fmt.Errorf("failed to get session stats: %w", err)
	}

	var stats struct {
		UploadSpeed     int64 `json:"uploadSpeed"`
		DownloadSpeed   int64 `json:"downloadSpeed"`
		CumulativeStats struct {
			UploadedBytes   int64 `json:"uploadedBytes"`
			DownloadedBytes int64 `json:"downloadedBytes"`
		} `json:"cumulative-stats"`
	}
	if err := json.Unmarshal(resp.Arguments, &stats); err != nil {
		return downloader.ClientStatus{}, fmt.Errorf("failed to parse session stats: %w", err)
	}

	return downloader.ClientStatus{
		UpSpeed: stats.UploadSpeed,
		DlSpeed: stats.DownloadSpeed,
		UpData:  stats.CumulativeStats.UploadedBytes,
		DlData:  stats.CumulativeStats.DownloadedBytes,
	}, nil
}

// GetClientFreeSpace 获取下载器所在磁盘的可用空间
func (t *TransmissionClient) GetClientFreeSpace(ctx context.Context) (int64, error) {
	return t.GetDiskSpace(ctx)
}

// torrentFullInfo 完整的种子信息结构
type torrentFullInfo struct {
	ID             int      `json:"id"`
	Name           string   `json:"name"`
	HashString     string   `json:"hashString"`
	Status         int      `json:"status"`
	PercentDone    float64  `json:"percentDone"`
	RateDownload   int64    `json:"rateDownload"`
	RateUpload     int64    `json:"rateUpload"`
	TotalSize      int64    `json:"totalSize"`
	UploadedEver   int64    `json:"uploadedEver"`
	DownloadedEver int64    `json:"downloadedEver"`
	UploadRatio    float64  `json:"uploadRatio"`
	AddedDate      int64    `json:"addedDate"`
	DownloadDir    string   `json:"downloadDir"`
	Labels         []string `json:"labels"`
}

// GetAllTorrents 获取所有种子列表
func (t *TransmissionClient) GetAllTorrents() ([]downloader.Torrent, error) {
	args := torrentGetArgs{
		Fields: []string{
			"id", "name", "hashString", "status", "percentDone",
			"rateDownload", "rateUpload", "totalSize", "uploadedEver",
			"downloadedEver", "uploadRatio", "addedDate", "downloadDir", "labels",
		},
	}

	resp, err := t.doRequest("torrent-get", args)
	if err != nil {
		return nil, fmt.Errorf("failed to get torrents: %w", err)
	}

	var getResp struct {
		Torrents []torrentFullInfo `json:"torrents"`
	}
	if err := json.Unmarshal(resp.Arguments, &getResp); err != nil {
		return nil, fmt.Errorf("failed to parse torrents: %w", err)
	}

	torrents := make([]downloader.Torrent, 0, len(getResp.Torrents))
	for _, tt := range getResp.Torrents {
		torrents = append(torrents, t.mapTransmissionTorrent(tt))
	}

	return torrents, nil
}

// mapTransmissionTorrent 将 Transmission 种子信息映射到通用 Torrent 结构
func (t *TransmissionClient) mapTransmissionTorrent(tt torrentFullInfo) downloader.Torrent {
	torrent := downloader.Torrent{
		ID:              fmt.Sprintf("%d", tt.ID),
		InfoHash:        tt.HashString,
		Name:            tt.Name,
		Progress:        tt.PercentDone,
		IsCompleted:     tt.PercentDone >= 1.0,
		Ratio:           tt.UploadRatio,
		DateAdded:       tt.AddedDate,
		SavePath:        tt.DownloadDir,
		State:           t.mapTransmissionState(tt.Status),
		TotalSize:       tt.TotalSize,
		UploadSpeed:     tt.RateUpload,
		DownloadSpeed:   tt.RateDownload,
		TotalUploaded:   tt.UploadedEver,
		TotalDownloaded: tt.DownloadedEver,
		ClientID:        t.name,
		Raw:             tt,
	}

	// 使用第一个 label 作为标签
	if len(tt.Labels) > 0 {
		torrent.Label = tt.Labels[0]
	}

	return torrent
}

// mapTransmissionState 将 Transmission 状态映射到通用状态
// Transmission 状态: 0=stopped, 1=check wait, 2=check, 3=download wait, 4=download, 5=seed wait, 6=seed
func (t *TransmissionClient) mapTransmissionState(status int) downloader.TorrentState {
	switch status {
	case 0:
		return downloader.TorrentPaused
	case 1, 2:
		return downloader.TorrentChecking
	case 3:
		return downloader.TorrentQueued
	case 4:
		return downloader.TorrentDownloading
	case 5:
		return downloader.TorrentQueued
	case 6:
		return downloader.TorrentSeeding
	default:
		return downloader.TorrentUnknown
	}
}

// GetTorrentsBy 根据过滤条件获取种子列表
func (t *TransmissionClient) GetTorrentsBy(filter downloader.TorrentFilter) ([]downloader.Torrent, error) {
	allTorrents, err := t.GetAllTorrents()
	if err != nil {
		return nil, err
	}

	// 如果没有过滤条件，返回所有种子
	if len(filter.IDs) == 0 && len(filter.Hashes) == 0 && filter.Complete == nil && filter.State == nil {
		return allTorrents, nil
	}

	// 构建过滤集合
	idSet := make(map[string]bool)
	for _, id := range filter.IDs {
		idSet[id] = true
	}
	hashSet := make(map[string]bool)
	for _, hash := range filter.Hashes {
		hashSet[hash] = true
	}

	// 过滤种子
	var filtered []downloader.Torrent
	for _, torrent := range allTorrents {
		// 按 ID 过滤
		if len(idSet) > 0 && !idSet[torrent.ID] {
			continue
		}
		// 按哈希过滤
		if len(hashSet) > 0 && !hashSet[torrent.InfoHash] {
			continue
		}
		// 按完成状态过滤
		if filter.Complete != nil && torrent.IsCompleted != *filter.Complete {
			continue
		}
		// 按状态过滤
		if filter.State != nil && torrent.State != *filter.State {
			continue
		}
		filtered = append(filtered, torrent)
	}

	return filtered, nil
}

// GetTorrent 获取单个种子信息
func (t *TransmissionClient) GetTorrent(id string) (downloader.Torrent, error) {
	filter := downloader.TorrentFilter{
		Hashes: []string{id},
	}
	torrents, err := t.GetTorrentsBy(filter)
	if err != nil {
		return downloader.Torrent{}, err
	}
	if len(torrents) == 0 {
		return downloader.Torrent{}, downloader.ErrTorrentNotFound
	}
	return torrents[0], nil
}

// AddTorrentEx 添加种子到下载器（新接口）
func (t *TransmissionClient) AddTorrentEx(torrentURL string, opt downloader.AddTorrentOptions) (downloader.AddTorrentResult, error) {
	args := map[string]any{
		"filename": torrentURL,
		"paused":   opt.AddAtPaused,
	}

	if opt.SavePath != "" {
		args["download-dir"] = opt.SavePath
	}

	// Transmission 使用 labels 代替 category/tags
	var labels []string
	if opt.Category != "" {
		labels = append(labels, opt.Category)
	}
	if opt.Tags != "" {
		labels = append(labels, opt.Tags)
	}
	if len(labels) > 0 {
		args["labels"] = labels
	}

	resp, err := t.doRequest("torrent-add", args)
	if err != nil {
		return downloader.AddTorrentResult{Success: false, Message: err.Error()}, err
	}

	var addResp torrentAddResponse
	if err := json.Unmarshal(resp.Arguments, &addResp); err != nil {
		return downloader.AddTorrentResult{Success: false, Message: err.Error()}, err
	}

	if addResp.TorrentDuplicate != nil {
		return downloader.AddTorrentResult{
			Success: true,
			Message: "Torrent already exists",
			ID:      fmt.Sprintf("%d", addResp.TorrentDuplicate.ID),
			Hash:    addResp.TorrentDuplicate.HashString,
		}, nil
	}

	if addResp.TorrentAdded != nil {
		return downloader.AddTorrentResult{
			Success: true,
			Message: "Torrent added successfully",
			ID:      fmt.Sprintf("%d", addResp.TorrentAdded.ID),
			Hash:    addResp.TorrentAdded.HashString,
		}, nil
	}

	return downloader.AddTorrentResult{Success: true, Message: "Torrent added"}, nil
}

// AddTorrentFileEx 添加种子文件到下载器（新接口）
func (t *TransmissionClient) AddTorrentFileEx(fileData []byte, opt downloader.AddTorrentOptions) (downloader.AddTorrentResult, error) {
	// Transmission 使用 base64 编码的种子文件
	metainfo := base64.StdEncoding.EncodeToString(fileData)

	args := map[string]any{
		"metainfo": metainfo,
		"paused":   opt.AddAtPaused,
	}

	if opt.SavePath != "" {
		args["download-dir"] = opt.SavePath
	}

	// Transmission 使用 labels 代替 category/tags
	var labels []string
	if opt.Category != "" {
		labels = append(labels, opt.Category)
	}
	if opt.Tags != "" {
		labels = append(labels, opt.Tags)
	}
	if len(labels) > 0 {
		args["labels"] = labels
	}

	resp, err := t.doRequest("torrent-add", args)
	if err != nil {
		return downloader.AddTorrentResult{Success: false, Message: err.Error()}, err
	}

	var addResp torrentAddResponse
	if err := json.Unmarshal(resp.Arguments, &addResp); err != nil {
		return downloader.AddTorrentResult{Success: false, Message: err.Error()}, err
	}

	if addResp.TorrentDuplicate != nil {
		return downloader.AddTorrentResult{
			Success: true,
			Message: "Torrent already exists",
			ID:      fmt.Sprintf("%d", addResp.TorrentDuplicate.ID),
			Hash:    addResp.TorrentDuplicate.HashString,
		}, nil
	}

	if addResp.TorrentAdded != nil {
		return downloader.AddTorrentResult{
			Success: true,
			Message: "Torrent added successfully",
			ID:      fmt.Sprintf("%d", addResp.TorrentAdded.ID),
			Hash:    addResp.TorrentAdded.HashString,
		}, nil
	}

	return downloader.AddTorrentResult{Success: true, Message: "Torrent added"}, nil
}

// PauseTorrent 暂停种子
func (t *TransmissionClient) PauseTorrent(id string) error {
	args := map[string]any{
		"ids": []any{id},
	}

	_, err := t.doRequest("torrent-stop", args)
	if err != nil {
		return fmt.Errorf("failed to pause torrent: %w", err)
	}

	return nil
}

// ResumeTorrent 恢复种子
func (t *TransmissionClient) ResumeTorrent(id string) error {
	args := map[string]any{
		"ids": []any{id},
	}

	_, err := t.doRequest("torrent-start", args)
	if err != nil {
		return fmt.Errorf("failed to resume torrent: %w", err)
	}

	return nil
}

// RemoveTorrent 删除种子
func (t *TransmissionClient) RemoveTorrent(id string, removeData bool) error {
	args := map[string]any{
		"ids":               []any{id},
		"delete-local-data": removeData,
	}

	_, err := t.doRequest("torrent-remove", args)
	if err != nil {
		return fmt.Errorf("failed to remove torrent: %w", err)
	}

	return nil
}

// GetClientPaths 获取下载器配置的保存路径列表
func (t *TransmissionClient) GetClientPaths() ([]string, error) {
	resp, err := t.doRequest("session-get", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get session info: %w", err)
	}

	var sessionInfo struct {
		DownloadDir string `json:"download-dir"`
	}
	if err := json.Unmarshal(resp.Arguments, &sessionInfo); err != nil {
		return nil, fmt.Errorf("failed to parse session info: %w", err)
	}

	var paths []string
	if sessionInfo.DownloadDir != "" {
		paths = append(paths, sessionInfo.DownloadDir)
	}

	return paths, nil
}

// GetClientLabels 获取下载器配置的标签列表
// Transmission 没有预定义的标签/分类系统，返回空列表
func (t *TransmissionClient) GetClientLabels() ([]string, error) {
	// Transmission 不像 qBittorrent 有预定义的分类
	// 标签是在添加种子时动态创建的
	return []string{}, nil
}

// ProcessSingleTorrentFile 处理单个种子文件
func (t *TransmissionClient) ProcessSingleTorrentFile(ctx context.Context, filePath, category, tags string) error {
	freeSpace, err := t.GetDiskSpace(ctx)
	if err != nil {
		// 磁盘空间检查失败时记录警告但继续处理
		sLogger().Warnf("Failed to check disk space: %v, continuing anyway", err)
	} else {
		sLogger().Info("Available disk space: ", float64(freeSpace)/(1024*1024*1024))
	}

	err = t.processTorrentFile(ctx, filePath, category, tags)
	if err != nil {
		return fmt.Errorf("failed to process torrent file: %w", err)
	}

	sLogger().Infof("Processed single torrent file: %s", filePath)
	return nil
}

func (t *TransmissionClient) processTorrentFile(ctx context.Context, filePath, category, tags string) error {
	sLogger().Info("Processing torrent file: ", filePath)

	torrentData, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("unable to read torrent file: %w", err)
	}

	torrentHash, err := qbit.ComputeTorrentHash(torrentData)
	if err != nil {
		return fmt.Errorf("unable to compute torrent hash: %w", err)
	}

	exists, err := t.CheckTorrentExists(torrentHash)
	if err != nil {
		return fmt.Errorf("failed to check torrent: %w", err)
	}

	if exists {
		if err = os.Remove(filePath); err != nil {
			return fmt.Errorf("torrent exists but failed to delete local file: %w", err)
		}
		sLogger().Info("Torrent exists, local file deleted: ", filePath)
		return nil
	}

	// 磁盘空间检查 - 失败时继续尝试添加
	canAdd, err := t.CanAddTorrent(ctx, int64(len(torrentData)))
	if err != nil {
		sLogger().Warnf("Unable to check disk space: %v, attempting to add anyway", err)
		canAdd = true // 假设可以添加
	}

	if !canAdd {
		sLogger().Error("Insufficient disk space, skipping torrent: ", filePath)
		return nil
	}

	if err := t.AddTorrent(torrentData, category, tags); err != nil {
		return fmt.Errorf("failed to add torrent: %w", err)
	}

	sLogger().Info("Torrent added successfully: ", filePath)
	return nil
}
