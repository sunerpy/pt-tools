package qbit

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zeebo/bencode"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// QbitClient qBittorrent 客户端实现
type QbitClient struct {
	name         string
	baseURL      string
	username     string
	password     string
	autoStart    bool
	client       requestDoer
	mu           sync.Mutex
	healthy      bool
	lastActivity time.Time
}

type requestDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// QbitTorrentProperties qBittorrent 种子属性
type QbitTorrentProperties struct {
	SavePath string `json:"save_path"`
}

// 确保 QbitClient 实现 Downloader 接口
var _ downloader.Downloader = (*QbitClient)(nil)

// NewQbitClient 创建新的 qBittorrent 客户端
func NewQbitClient(config downloader.DownloaderConfig, name string) (downloader.Downloader, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	client := &QbitClient{
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
func (q *QbitClient) GetType() downloader.DownloaderType {
	return downloader.DownloaderQBittorrent
}

// GetName 获取下载器实例名称
func (q *QbitClient) GetName() string {
	return q.name
}

// IsHealthy 检查下载器是否健康可用
func (q *QbitClient) IsHealthy() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.healthy
}

// Close 关闭下载器连接
func (q *QbitClient) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.healthy = false
	if closer, ok := q.client.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
	return nil
}

// Authenticate 认证连接到 qBittorrent
func (q *QbitClient) Authenticate() error {
	return q.AuthenticateWithContext(context.Background())
}

// AuthenticateWithContext 带 context 的认证连接到 qBittorrent
func (q *QbitClient) AuthenticateWithContext(ctx context.Context) error {
	loginURL := fmt.Sprintf("%s/api/v2/auth/login", q.baseURL)
	data := url.Values{}
	data.Set("username", q.username)
	data.Set("password", q.password)

	req, err := http.NewRequestWithContext(ctx, "POST", loginURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return fmt.Errorf("创建登录请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", q.baseURL)

	resp, err := q.client.Do(req)
	if err != nil {
		q.healthy = false
		return q.wrapConnectionError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		q.healthy = false
		return q.wrapStatusCodeError(resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		q.healthy = false
		return fmt.Errorf("读取响应失败: %w", err)
	}

	if string(body) != "Ok." {
		q.healthy = false
		if string(body) == "Fails." {
			return fmt.Errorf("用户名或密码错误")
		}
		return fmt.Errorf("登录失败，服务器响应: %s", string(body))
	}

	q.healthy = true
	q.lastActivity = time.Now()
	sLogger().Info("Successfully logged in to qBittorrent")
	return nil
}

func (q *QbitClient) wrapConnectionError(err error) error {
	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "connection refused"):
		return fmt.Errorf("连接被拒绝，请检查: 1) qBittorrent 是否正在运行 2) WebUI 是否已启用 3) 端口是否正确 (原始错误: %w)", err)
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

func (q *QbitClient) wrapStatusCodeError(statusCode int) error {
	switch statusCode {
	case http.StatusForbidden:
		return fmt.Errorf("访问被禁止(403)，可能原因: 1) IP 被封禁（登录失败次数过多） 2) 需要在 qBittorrent 设置中添加 IP 白名单")
	case http.StatusNotFound:
		return fmt.Errorf("API 路径不存在(404)，请检查: 1) URL 是否正确 2) qBittorrent 版本是否过旧")
	case http.StatusUnauthorized:
		return fmt.Errorf("认证失败(401)，用户名或密码错误")
	default:
		return fmt.Errorf("登录失败，HTTP 状态码: %d", statusCode)
	}
}

// doRequestWithRetry 执行请求并在需要时重试
func (q *QbitClient) doRequestWithRetry(req *http.Request) (*http.Response, error) {
	if q.client == nil {
		return nil, fmt.Errorf("client is closed")
	}
	resp, err := q.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		if authErr := q.Authenticate(); authErr != nil {
			return nil, fmt.Errorf("re-authentication failed: %w", authErr)
		}
		newReq := req.Clone(req.Context())
		if req.Body != nil {
			return nil, fmt.Errorf("cannot retry request with non-rewindable body")
		}
		resp, err = q.client.Do(newReq)
		if err != nil {
			return nil, err
		}
	}

	if resp.StatusCode == http.StatusOK {
		q.mu.Lock()
		q.lastActivity = time.Now()
		q.mu.Unlock()
	}
	return resp, nil
}

// GetDiskSpace 获取可用磁盘空间
func (q *QbitClient) GetDiskSpace(ctx context.Context) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	diskURL := fmt.Sprintf("%s/api/v2/sync/maindata", q.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", diskURL, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create disk space request: %w", err)
	}

	resp, err := q.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("disk space request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("disk space request failed with status code: %d", resp.StatusCode)
	}

	var responseData map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&responseData); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	serverState, ok := responseData["server_state"].(map[string]any)
	if !ok {
		return 0, fmt.Errorf("unable to get server_state info")
	}

	freeSpace, ok := serverState["free_space_on_disk"].(float64)
	if !ok {
		return 0, fmt.Errorf("unable to get disk space info")
	}

	return int64(freeSpace), nil
}

// CanAddTorrent 检查是否可以添加指定大小的种子
func (q *QbitClient) CanAddTorrent(ctx context.Context, fileSize int64) (bool, error) {
	freeSpace, err := q.GetDiskSpace(ctx)
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

// AddTorrent 添加种子到 qBittorrent
func (q *QbitClient) AddTorrent(fileData []byte, category, tags string) error {
	return q.AddTorrentWithPath(fileData, category, tags, "")
}

// AddTorrentWithPath 添加种子到 qBittorrent 并指定下载路径
func (q *QbitClient) AddTorrentWithPath(fileData []byte, category, tags, downloadPath string) error {
	skipChecking := false
	paused := !q.autoStart // autoStart=true 时 paused=false，autoStart=false 时 paused=true

	// Debug logging for troubleshooting
	sLogger().Infof("[qBittorrent] AddTorrentWithPath called: category=%s, tags=%s, downloadPath=%s, autoStart=%v, paused=%v",
		category, tags, downloadPath, q.autoStart, paused)

	q.mu.Lock()
	defer q.mu.Unlock()

	uploadURL := fmt.Sprintf("%s/api/v2/torrents/add", q.baseURL)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("torrents", "upload.torrent")
	if err != nil {
		return fmt.Errorf("failed to create torrent form file: %w", err)
	}
	if _, err = part.Write(fileData); err != nil {
		return fmt.Errorf("failed to write torrent file data: %w", err)
	}

	if category != "" {
		if err = writer.WriteField("category", category); err != nil {
			return fmt.Errorf("failed to write category: %w", err)
		}
	}
	if tags != "" {
		if err = writer.WriteField("tags", tags); err != nil {
			return fmt.Errorf("failed to write tags: %w", err)
		}
	}
	// 设置自定义下载路径（savepath）
	if downloadPath != "" {
		sLogger().Infof("[qBittorrent] Setting savepath to: %s", downloadPath)
		if err = writer.WriteField("savepath", downloadPath); err != nil {
			return fmt.Errorf("failed to write savepath: %w", err)
		}
	}
	if err = writer.WriteField("skip_checking", fmt.Sprintf("%t", skipChecking)); err != nil {
		return fmt.Errorf("failed to write skip_checking: %w", err)
	}
	// 同时发送 paused 和 stopped 参数以兼容不同版本的 qBittorrent
	// qBittorrent < 5.1.0 使用 'paused' 参数
	if err = writer.WriteField("paused", fmt.Sprintf("%t", paused)); err != nil {
		return fmt.Errorf("failed to write paused: %w", err)
	}
	// qBittorrent 5.1.0+ 使用 'stopped' 参数
	if err = writer.WriteField("stopped", fmt.Sprintf("%t", paused)); err != nil {
		return fmt.Errorf("failed to write stopped: %w", err)
	}
	if err = writer.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", uploadURL, body)
	if err != nil {
		return fmt.Errorf("failed to create upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed with status code: %d, response: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// CheckTorrentExists 检查种子是否存在
func (q *QbitClient) CheckTorrentExists(torrentHash string) (bool, error) {
	return q.CheckTorrentExistsWithContext(context.Background(), torrentHash)
}

// CheckTorrentExistsWithContext 带 context 检查种子是否存在
func (q *QbitClient) CheckTorrentExistsWithContext(ctx context.Context, torrentHash string) (bool, error) {
	propertiesURL := fmt.Sprintf("%s/api/v2/torrents/properties?hash=%s", q.baseURL, torrentHash)
	req, err := http.NewRequestWithContext(ctx, "GET", propertiesURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create check request: %w", err)
	}

	resp, err := q.doRequestWithRetry(req)
	if err != nil {
		return false, fmt.Errorf("check request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		sLogger().Infof("Torrent %s not in qBittorrent, preparing to add...", torrentHash)
		return false, nil
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("check failed with status code: %d", resp.StatusCode)
	}

	var props QbitTorrentProperties
	if err := json.NewDecoder(resp.Body).Decode(&props); err != nil {
		return false, fmt.Errorf("failed to parse torrent info: %w", err)
	}

	sLogger().Info("Torrent save path: ", props.SavePath)
	return true, nil
}

// ProcessSingleTorrentFile 处理单个种子文件
func (q *QbitClient) ProcessSingleTorrentFile(ctx context.Context, filePath, category, tags string) error {
	freeSpace, err := q.GetDiskSpace(ctx)
	if err != nil {
		return fmt.Errorf("failed to check disk space: %w", err)
	}
	sLogger().Info("Available disk space: ", float64(freeSpace)/(1024*1024*1024))

	err = q.processTorrentFile(ctx, filePath, category, tags)
	if err != nil {
		return fmt.Errorf("failed to process torrent file: %w", err)
	}

	sLogger().Infof("Processed single torrent file: %s", filePath)
	return nil
}

func (q *QbitClient) processTorrentFile(ctx context.Context, filePath, category, tags string) error {
	sLogger().Info("Processing torrent file: ", filePath)

	torrentData, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("unable to read torrent file: %w", err)
	}

	torrentHash, err := ComputeTorrentHash(torrentData)
	if err != nil {
		return fmt.Errorf("unable to compute torrent hash: %w", err)
	}

	exists, err := q.CheckTorrentExists(torrentHash)
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

	canAdd, err := q.CanAddTorrent(ctx, int64(len(torrentData)))
	if err != nil {
		return fmt.Errorf("unable to determine if torrent can be added: %w", err)
	}

	if !canAdd {
		sLogger().Error("Insufficient disk space, skipping torrent: ", filePath)
		return nil
	}

	if err := q.AddTorrent(torrentData, category, tags); err != nil {
		return fmt.Errorf("failed to add torrent: %w", err)
	}

	sLogger().Info("Torrent added successfully: ", filePath)
	return nil
}

// ComputeTorrentHash 计算种子的 SHA1 哈希值
func ComputeTorrentHash(data []byte) (string, error) {
	reader := bytes.NewReader(data)
	var torrent map[string]any
	err := bencode.NewDecoder(reader).Decode(&torrent)
	if err != nil {
		return "", fmt.Errorf("failed to decode torrent file: %w", err)
	}

	info, ok := torrent["info"]
	if !ok {
		return "", fmt.Errorf("info section not found in torrent file")
	}

	infoEncoded, err := bencode.EncodeString(info)
	if err != nil {
		return "", fmt.Errorf("failed to encode info dictionary: %w", err)
	}

	hash := sha1.Sum([]byte(infoEncoded))
	return hex.EncodeToString(hash[:]), nil
}

// ComputeTorrentHashWithPath 从文件路径计算种子哈希
func ComputeTorrentHashWithPath(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("unable to read torrent file: %w", err)
	}
	return ComputeTorrentHash(data)
}

// GetTorrentFilesPath 获取目录中所有种子文件
func GetTorrentFilesPath(directory string) ([]string, error) {
	files, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("unable to read directory: %w", err)
	}

	var torrentFiles []string
	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".torrent" {
			torrentFiles = append(torrentFiles, filepath.Join(directory, file.Name()))
		}
	}
	return torrentFiles, nil
}

// NewQbitClientForTesting 创建用于测试的 qBittorrent 客户端
// 允许注入自定义 HTTP 客户端
func NewQbitClientForTesting(httpClient *http.Client, baseURL string) *QbitClient {
	return &QbitClient{
		name:         "test-client",
		baseURL:      baseURL,
		client:       &standardHTTPDoer{client: httpClient},
		healthy:      true,
		lastActivity: time.Now(),
	}
}

type standardHTTPDoer struct {
	client *http.Client
}

func (d *standardHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	return d.client.Do(req)
}

// ProcessTorrentDirectory 处理目录中的所有种子文件
func (q *QbitClient) ProcessTorrentDirectory(ctx context.Context, directory, category, tags string) error {
	freeSpace, err := q.GetDiskSpace(ctx)
	if err != nil {
		return fmt.Errorf("failed to check disk space: %w", err)
	}
	sLogger().Info("Available disk space: ", float64(freeSpace)/(1024*1024*1024))

	torrentFiles, err := GetTorrentFilesPath(directory)
	if err != nil {
		return fmt.Errorf("unable to read directory: %w", err)
	}

	for _, file := range torrentFiles {
		if err := q.processTorrentFile(ctx, file, category, tags); err != nil {
			sLogger().Error("Failed to process torrent file: ", file, err)
		}
	}
	return nil
}

// EnsureTorrentStarted 确保种子已启动（如果配置了自动启动）
func (q *QbitClient) EnsureTorrentStarted(torrentHash string) error {
	// 如果没有配置自动启动，直接返回
	if !q.autoStart {
		return nil
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// 获取种子信息
	infoURL := fmt.Sprintf("%s/api/v2/torrents/info?hashes=%s", q.baseURL, torrentHash)
	req, err := http.NewRequestWithContext(context.Background(), "GET", infoURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create info request: %w", err)
	}

	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("info request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("info request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var torrents []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&torrents); err != nil {
		return fmt.Errorf("failed to parse torrent info: %w", err)
	}

	if len(torrents) == 0 {
		return fmt.Errorf("torrent %s not found", torrentHash)
	}

	torrent := torrents[0]
	name, _ := torrent["name"].(string)
	state, _ := torrent["state"].(string)

	// 检查种子状态，pausedDL 和 pausedUP 表示暂停状态
	if state == "pausedDL" || state == "pausedUP" {
		sLogger().Infof("Torrent %s is paused (state: %s), resuming it...", name, state)

		// 恢复种子
		resumeURL := fmt.Sprintf("%s/api/v2/torrents/resume", q.baseURL)
		data := url.Values{}
		data.Set("hashes", torrentHash)

		resumeReq, err := http.NewRequestWithContext(context.Background(), "POST", resumeURL, bytes.NewBufferString(data.Encode()))
		if err != nil {
			return fmt.Errorf("failed to create resume request: %w", err)
		}

		resumeReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resumeResp, err := q.client.Do(resumeReq)
		if err != nil {
			return fmt.Errorf("resume request failed: %w", err)
		}
		defer resumeResp.Body.Close()

		if resumeResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resumeResp.Body)
			return fmt.Errorf("resume request failed with status %d: %s", resumeResp.StatusCode, string(body))
		}

		sLogger().Infof("Torrent %s resumed successfully", name)
	} else {
		sLogger().Debugf("Torrent %s is already running (state: %s)", name, state)
	}

	return nil
}

// ============ 新接口实现 ============

// Ping 检查下载器连接是否正常
func (q *QbitClient) Ping() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	versionURL := fmt.Sprintf("%s/api/v2/app/version", q.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", versionURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create ping request: %w", err)
	}

	resp, err := q.doRequestWithRetry(req)
	if err != nil {
		q.healthy = false
		return false, fmt.Errorf("ping request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		q.healthy = false
		return false, nil
	}

	q.healthy = true
	q.lastActivity = time.Now()
	return true, nil
}

// GetClientVersion 获取下载器版本
func (q *QbitClient) GetClientVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	versionURL := fmt.Sprintf("%s/api/v2/app/version", q.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", versionURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create version request: %w", err)
	}

	resp, err := q.doRequestWithRetry(req)
	if err != nil {
		return "", fmt.Errorf("version request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("version request failed with status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read version response: %w", err)
	}

	return string(body), nil
}

// GetClientStatus 获取下载器状态
func (q *QbitClient) GetClientStatus() (downloader.ClientStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mainDataURL := fmt.Sprintf("%s/api/v2/sync/maindata", q.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", mainDataURL, nil)
	if err != nil {
		return downloader.ClientStatus{}, fmt.Errorf("failed to create status request: %w", err)
	}

	resp, err := q.doRequestWithRetry(req)
	if err != nil {
		return downloader.ClientStatus{}, fmt.Errorf("status request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return downloader.ClientStatus{}, fmt.Errorf("status request failed with status code: %d", resp.StatusCode)
	}

	var responseData map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&responseData); err != nil {
		return downloader.ClientStatus{}, fmt.Errorf("failed to parse response: %w", err)
	}

	serverState, ok := responseData["server_state"].(map[string]any)
	if !ok {
		return downloader.ClientStatus{}, fmt.Errorf("unable to get server_state info")
	}

	status := downloader.ClientStatus{}
	if upSpeed, ok := serverState["up_info_speed"].(float64); ok {
		status.UpSpeed = int64(upSpeed)
	}
	if upData, ok := serverState["alltime_ul"].(float64); ok { //nolint:misspell // qBittorrent API field name
		status.UpData = int64(upData)
	}
	if dlSpeed, ok := serverState["dl_info_speed"].(float64); ok {
		status.DlSpeed = int64(dlSpeed)
	}
	if dlData, ok := serverState["alltime_dl"].(float64); ok { //nolint:misspell // qBittorrent API field name
		status.DlData = int64(dlData)
	}

	return status, nil
}

// GetClientFreeSpace 获取下载器所在磁盘的可用空间
func (q *QbitClient) GetClientFreeSpace(ctx context.Context) (int64, error) {
	return q.GetDiskSpace(ctx)
}

// GetAllTorrents 获取所有种子列表
func (q *QbitClient) GetAllTorrents() ([]downloader.Torrent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	torrentsURL := fmt.Sprintf("%s/api/v2/torrents/info", q.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", torrentsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create torrents request: %w", err)
	}

	resp, err := q.doRequestWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("torrents request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("torrents request failed with status code: %d", resp.StatusCode)
	}

	var qbitTorrents []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&qbitTorrents); err != nil {
		return nil, fmt.Errorf("failed to parse torrents response: %w", err)
	}

	torrents := make([]downloader.Torrent, 0, len(qbitTorrents))
	for _, qt := range qbitTorrents {
		torrents = append(torrents, q.mapQbitTorrent(qt))
	}

	return torrents, nil
}

// mapQbitTorrent 将 qBittorrent 种子信息映射到通用 Torrent 结构
func (q *QbitClient) mapQbitTorrent(qt map[string]any) downloader.Torrent {
	t := downloader.Torrent{
		ClientID: q.name,
	}

	if hash, ok := qt["hash"].(string); ok {
		t.ID = hash
		t.InfoHash = hash
	}
	if name, ok := qt["name"].(string); ok {
		t.Name = name
	}
	if progress, ok := qt["progress"].(float64); ok {
		t.Progress = progress
		t.IsCompleted = progress >= 1.0
	}
	if ratio, ok := qt["ratio"].(float64); ok {
		t.Ratio = ratio
	}
	if addedOn, ok := qt["added_on"].(float64); ok {
		t.DateAdded = int64(addedOn)
	}
	if savePath, ok := qt["save_path"].(string); ok {
		t.SavePath = savePath
	}
	if category, ok := qt["category"].(string); ok {
		t.Category = category
		t.Label = category
	}
	if tags, ok := qt["tags"].(string); ok {
		t.Tags = tags
	}
	if state, ok := qt["state"].(string); ok {
		t.State = q.mapQbitState(state)
	}
	if size, ok := qt["size"].(float64); ok {
		t.TotalSize = int64(size)
	}
	if upSpeed, ok := qt["upspeed"].(float64); ok {
		t.UploadSpeed = int64(upSpeed)
	}
	if dlSpeed, ok := qt["dlspeed"].(float64); ok {
		t.DownloadSpeed = int64(dlSpeed)
	}
	if uploaded, ok := qt["uploaded"].(float64); ok {
		t.TotalUploaded = int64(uploaded)
	}
	if downloaded, ok := qt["downloaded"].(float64); ok {
		t.TotalDownloaded = int64(downloaded)
	}
	if eta, ok := qt["eta"].(float64); ok {
		t.ETA = int64(eta)
	}
	if seedingTime, ok := qt["seeding_time"].(float64); ok {
		t.SeedingTime = int64(seedingTime)
	}
	if tracker, ok := qt["tracker"].(string); ok {
		t.Tracker = tracker
	}
	if completionOn, ok := qt["completion_on"].(float64); ok {
		t.CompletionOn = int64(completionOn)
	}
	if numSeeds, ok := qt["num_seeds"].(float64); ok {
		t.NumSeeds = int(numSeeds)
	}
	if numLeechs, ok := qt["num_leechs"].(float64); ok {
		t.NumPeers = int(numLeechs)
	}
	if availability, ok := qt["availability"].(float64); ok {
		t.Availability = availability
	}
	if contentPath, ok := qt["content_path"].(string); ok {
		t.ContentPath = contentPath
	}

	t.Raw = qt
	return t
}

func (q *QbitClient) postForm(endpoint string, data url.Values) error {
	req, err := http.NewRequestWithContext(context.Background(), "POST", fmt.Sprintf("%s%s", q.baseURL, endpoint), bytes.NewBufferString(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request for %s: %w", endpoint, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed for %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed for %s with status %d: %s", endpoint, resp.StatusCode, string(body))
	}

	return nil
}

func (q *QbitClient) getJSON(endpoint string, dst any) error {
	req, err := http.NewRequestWithContext(context.Background(), "GET", fmt.Sprintf("%s%s", q.baseURL, endpoint), nil)
	if err != nil {
		return fmt.Errorf("failed to create request for %s: %w", endpoint, err)
	}

	resp, err := q.doRequestWithRetry(req)
	if err != nil {
		return fmt.Errorf("request failed for %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed for %s with status %d: %s", endpoint, resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("failed to decode response for %s: %w", endpoint, err)
	}

	return nil
}

func (q *QbitClient) callPauseResumeEndpoints(ids []string, modernEndpoint, legacyEndpoint string) error {
	hashes := strings.Join(ids, "|")
	if hashes == "" {
		return nil
	}

	data := url.Values{}
	data.Set("hashes", hashes)

	req, err := http.NewRequestWithContext(context.Background(), "POST", fmt.Sprintf("%s%s", q.baseURL, modernEndpoint), bytes.NewBufferString(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request for %s: %w", modernEndpoint, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed for %s: %w", modernEndpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return q.postForm(legacyEndpoint, data)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed for %s with status %d: %s", modernEndpoint, resp.StatusCode, string(body))
	}

	return nil
}

// mapQbitState 将 qBittorrent 状态映射到通用状态
func (q *QbitClient) mapQbitState(state string) downloader.TorrentState {
	switch state {
	case "downloading", "forcedDL", "metaDL", "stalledDL":
		return downloader.TorrentDownloading
	case "uploading", "forcedUP", "stalledUP":
		return downloader.TorrentSeeding
	case "pausedDL", "pausedUP":
		return downloader.TorrentPaused
	case "queuedDL", "queuedUP":
		return downloader.TorrentQueued
	case "checkingDL", "checkingUP", "checkingResumeData":
		return downloader.TorrentChecking
	case "error", "missingFiles":
		return downloader.TorrentError
	default:
		return downloader.TorrentUnknown
	}
}

// GetTorrentsBy 根据过滤条件获取种子列表
func (q *QbitClient) GetTorrentsBy(filter downloader.TorrentFilter) ([]downloader.Torrent, error) {
	allTorrents, err := q.GetAllTorrents()
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
	for _, t := range allTorrents {
		// 按 ID 过滤
		if len(idSet) > 0 && !idSet[t.ID] {
			continue
		}
		// 按哈希过滤
		if len(hashSet) > 0 && !hashSet[t.InfoHash] {
			continue
		}
		// 按完成状态过滤
		if filter.Complete != nil && t.IsCompleted != *filter.Complete {
			continue
		}
		// 按状态过滤
		if filter.State != nil && t.State != *filter.State {
			continue
		}
		filtered = append(filtered, t)
	}

	return filtered, nil
}

// GetTorrent 获取单个种子信息
func (q *QbitClient) GetTorrent(id string) (downloader.Torrent, error) {
	filter := downloader.TorrentFilter{
		Hashes: []string{id},
	}
	torrents, err := q.GetTorrentsBy(filter)
	if err != nil {
		return downloader.Torrent{}, err
	}
	if len(torrents) == 0 {
		return downloader.Torrent{}, downloader.ErrTorrentNotFound
	}
	return torrents[0], nil
}

// AddTorrentEx 添加种子到下载器（新接口）
func (q *QbitClient) AddTorrentEx(torrentURL string, opt downloader.AddTorrentOptions) (downloader.AddTorrentResult, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	uploadURL := fmt.Sprintf("%s/api/v2/torrents/add", q.baseURL)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 添加 URL
	if err := writer.WriteField("urls", torrentURL); err != nil {
		return downloader.AddTorrentResult{Success: false, Message: err.Error()}, err
	}

	// 写入选项
	if err := q.writeAddTorrentOptions(writer, opt); err != nil {
		return downloader.AddTorrentResult{Success: false, Message: err.Error()}, err
	}

	if err := writer.Close(); err != nil {
		return downloader.AddTorrentResult{Success: false, Message: err.Error()}, err
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", uploadURL, body)
	if err != nil {
		return downloader.AddTorrentResult{Success: false, Message: err.Error()}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := q.client.Do(req)
	if err != nil {
		return downloader.AddTorrentResult{Success: false, Message: err.Error()}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return downloader.AddTorrentResult{
			Success: false,
			Message: fmt.Sprintf("upload failed with status code: %d, response: %s", resp.StatusCode, string(respBody)),
		}, fmt.Errorf("upload failed with status code: %d", resp.StatusCode)
	}

	return downloader.AddTorrentResult{Success: true, Message: "Torrent added successfully"}, nil
}

// AddTorrentFileEx 添加种子文件到下载器（新接口）
func (q *QbitClient) AddTorrentFileEx(fileData []byte, opt downloader.AddTorrentOptions) (downloader.AddTorrentResult, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// 计算种子哈希
	torrentHash, err := ComputeTorrentHash(fileData)
	if err != nil {
		return downloader.AddTorrentResult{Success: false, Message: err.Error()}, err
	}

	uploadURL := fmt.Sprintf("%s/api/v2/torrents/add", q.baseURL)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 添加种子文件
	part, err := writer.CreateFormFile("torrents", "upload.torrent")
	if err != nil {
		return downloader.AddTorrentResult{Success: false, Message: err.Error()}, err
	}
	if _, err = part.Write(fileData); err != nil {
		return downloader.AddTorrentResult{Success: false, Message: err.Error()}, err
	}

	// 写入选项
	if writeErr := q.writeAddTorrentOptions(writer, opt); writeErr != nil {
		return downloader.AddTorrentResult{Success: false, Message: writeErr.Error()}, writeErr
	}

	if closeErr := writer.Close(); closeErr != nil {
		return downloader.AddTorrentResult{Success: false, Message: closeErr.Error()}, closeErr
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", uploadURL, body)
	if err != nil {
		return downloader.AddTorrentResult{Success: false, Message: err.Error()}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := q.client.Do(req)
	if err != nil {
		return downloader.AddTorrentResult{Success: false, Message: err.Error()}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return downloader.AddTorrentResult{
			Success: false,
			Message: fmt.Sprintf("upload failed with status code: %d, response: %s", resp.StatusCode, string(respBody)),
		}, fmt.Errorf("upload failed with status code: %d", resp.StatusCode)
	}

	return downloader.AddTorrentResult{
		Success: true,
		Message: "Torrent added successfully",
		Hash:    torrentHash,
	}, nil
}

// writeAddTorrentOptions 写入添加种子的选项到 multipart writer
func (q *QbitClient) writeAddTorrentOptions(writer *multipart.Writer, opt downloader.AddTorrentOptions) error {
	// 设置暂停状态 - 同时设置 paused 和 stopped 以兼容不同版本
	pausedStr := fmt.Sprintf("%t", opt.AddAtPaused)
	if err := writer.WriteField("paused", pausedStr); err != nil {
		return fmt.Errorf("failed to write paused: %w", err)
	}
	// qBittorrent 5.1+ 使用 stopped 参数
	if err := writer.WriteField("stopped", pausedStr); err != nil {
		return fmt.Errorf("failed to write stopped: %w", err)
	}

	// 设置保存路径
	if opt.SavePath != "" {
		if err := writer.WriteField("savepath", opt.SavePath); err != nil {
			return fmt.Errorf("failed to write savepath: %w", err)
		}
	}

	// 设置分类
	if opt.Category != "" {
		if err := writer.WriteField("category", opt.Category); err != nil {
			return fmt.Errorf("failed to write category: %w", err)
		}
	}

	// 设置标签
	if opt.Tags != "" {
		if err := writer.WriteField("tags", opt.Tags); err != nil {
			return fmt.Errorf("failed to write tags: %w", err)
		}
	}

	// 设置上传速度限制
	if opt.UploadSpeedLimitMB > 0 {
		limitBytes := opt.UploadSpeedLimitMB * 1024 * 1024
		if err := writer.WriteField("upLimit", fmt.Sprintf("%d", limitBytes)); err != nil {
			return fmt.Errorf("failed to write upLimit: %w", err)
		}
	}

	// 设置高级选项
	for key, value := range opt.AdvanceOptions {
		if boolVal, ok := value.(bool); ok && boolVal {
			if err := writer.WriteField(key, "true"); err != nil {
				return fmt.Errorf("failed to write %s: %w", key, err)
			}
		} else if strVal, ok := value.(string); ok {
			if err := writer.WriteField(key, strVal); err != nil {
				return fmt.Errorf("failed to write %s: %w", key, err)
			}
		}
	}

	// 默认不跳过校验
	if err := writer.WriteField("skip_checking", "false"); err != nil {
		return fmt.Errorf("failed to write skip_checking: %w", err)
	}

	return nil
}

// PauseTorrent 暂停种子
// qBittorrent 5.0+ 使用 /api/v2/torrents/stop，旧版本使用 /api/v2/torrents/pause
func (q *QbitClient) PauseTorrent(id string) error {
	return q.PauseTorrents([]string{id})
}

// ResumeTorrent 恢复种子
// qBittorrent 5.0+ 使用 /api/v2/torrents/start，旧版本使用 /api/v2/torrents/resume
func (q *QbitClient) ResumeTorrent(id string) error {
	return q.ResumeTorrents([]string{id})
}

// RemoveTorrent 删除种子
func (q *QbitClient) RemoveTorrent(id string, removeData bool) error {
	return q.RemoveTorrents([]string{id}, removeData)
}

// PauseTorrents 批量暂停种子
func (q *QbitClient) PauseTorrents(ids []string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.callPauseResumeEndpoints(ids, "/api/v2/torrents/stop", "/api/v2/torrents/pause")
}

// ResumeTorrents 批量恢复种子
func (q *QbitClient) ResumeTorrents(ids []string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.callPauseResumeEndpoints(ids, "/api/v2/torrents/start", "/api/v2/torrents/resume")
}

// RemoveTorrents 批量删除种子
func (q *QbitClient) RemoveTorrents(ids []string, removeData bool) error {
	hashes := strings.Join(ids, "|")
	if hashes == "" {
		return nil
	}

	data := url.Values{}
	data.Set("hashes", hashes)
	data.Set("deleteFiles", fmt.Sprintf("%t", removeData))

	q.mu.Lock()
	defer q.mu.Unlock()
	return q.postForm("/api/v2/torrents/delete", data)
}

// SetTorrentCategory 设置种子分类
func (q *QbitClient) SetTorrentCategory(id, category string) error {
	data := url.Values{}
	data.Set("hashes", id)
	data.Set("category", category)

	q.mu.Lock()
	defer q.mu.Unlock()
	return q.postForm("/api/v2/torrents/setCategory", data)
}

// SetTorrentTags 设置种子标签
func (q *QbitClient) SetTorrentTags(id, tags string) error {
	data := url.Values{}
	data.Set("hashes", id)
	data.Set("tags", tags)

	q.mu.Lock()
	defer q.mu.Unlock()
	return q.postForm("/api/v2/torrents/addTags", data)
}

// SetTorrentSavePath 设置种子保存路径
func (q *QbitClient) SetTorrentSavePath(id, path string) error {
	data := url.Values{}
	data.Set("hashes", id)
	data.Set("location", path)

	q.mu.Lock()
	defer q.mu.Unlock()
	return q.postForm("/api/v2/torrents/setLocation", data)
}

// RecheckTorrent 重新校验种子
func (q *QbitClient) RecheckTorrent(id string) error {
	data := url.Values{}
	data.Set("hashes", id)

	q.mu.Lock()
	defer q.mu.Unlock()
	return q.postForm("/api/v2/torrents/recheck", data)
}

// GetTorrentFiles 获取种子文件列表
func (q *QbitClient) GetTorrentFiles(id string) ([]downloader.TorrentFile, error) {
	var qFiles []map[string]any
	if err := q.getJSON(fmt.Sprintf("/api/v2/torrents/files?hash=%s", url.QueryEscape(id)), &qFiles); err != nil {
		return nil, err
	}

	files := make([]downloader.TorrentFile, 0, len(qFiles))
	for _, item := range qFiles {
		file := downloader.TorrentFile{}
		if index, ok := item["index"].(float64); ok {
			file.Index = int(index)
		}
		if name, ok := item["name"].(string); ok {
			file.Name = name
		}
		if size, ok := item["size"].(float64); ok {
			file.Size = int64(size)
		}
		if progress, ok := item["progress"].(float64); ok {
			file.Progress = progress
		}
		if priority, ok := item["priority"].(float64); ok {
			file.Priority = int(priority)
		}
		files = append(files, file)
	}

	return files, nil
}

// GetTorrentTrackers 获取种子 Tracker 列表
func (q *QbitClient) GetTorrentTrackers(id string) ([]downloader.TorrentTracker, error) {
	var qTrackers []map[string]any
	if err := q.getJSON(fmt.Sprintf("/api/v2/torrents/trackers?hash=%s", url.QueryEscape(id)), &qTrackers); err != nil {
		return nil, err
	}

	trackers := make([]downloader.TorrentTracker, 0, len(qTrackers))
	for _, item := range qTrackers {
		tracker := downloader.TorrentTracker{}
		if trackerURL, ok := item["url"].(string); ok {
			tracker.URL = trackerURL
		}
		if status, ok := item["status"].(float64); ok {
			tracker.Status = int(status)
		}
		if peers, ok := item["num_peers"].(float64); ok {
			tracker.Peers = int(peers)
		}
		if seeds, ok := item["num_seeds"].(float64); ok {
			tracker.Seeds = int(seeds)
		}
		if leeches, ok := item["num_leeches"].(float64); ok {
			tracker.Leeches = int(leeches)
		}
		if msg, ok := item["msg"].(string); ok {
			tracker.Message = msg
		} else if message, ok := item["message"].(string); ok {
			tracker.Message = message
		}
		trackers = append(trackers, tracker)
	}

	return trackers, nil
}

// GetDiskInfo 获取磁盘信息
func (q *QbitClient) GetDiskInfo() (downloader.DiskInfo, error) {
	var responseData map[string]any
	if err := q.getJSON("/api/v2/sync/maindata", &responseData); err != nil {
		return downloader.DiskInfo{}, err
	}

	serverState, ok := responseData["server_state"].(map[string]any)
	if !ok {
		return downloader.DiskInfo{}, fmt.Errorf("unable to get server_state info")
	}

	diskInfo := downloader.DiskInfo{}
	if freeSpace, ok := serverState["free_space_on_disk"].(float64); ok {
		diskInfo.FreeSpace = int64(freeSpace)
	}
	if savePath, ok := serverState["default_save_path"].(string); ok {
		diskInfo.Path = savePath
	}

	return diskInfo, nil
}

// GetSpeedLimit 获取全局速度限制
func (q *QbitClient) GetSpeedLimit() (downloader.SpeedLimit, error) {
	req, err := http.NewRequestWithContext(context.Background(), "GET", fmt.Sprintf("%s/api/v2/transfer/speedLimitsMode", q.baseURL), nil)
	if err != nil {
		return downloader.SpeedLimit{}, fmt.Errorf("failed to create speed mode request: %w", err)
	}
	resp, err := q.doRequestWithRetry(req)
	if err != nil {
		return downloader.SpeedLimit{}, fmt.Errorf("speed mode request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return downloader.SpeedLimit{}, fmt.Errorf("speed mode request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return downloader.SpeedLimit{}, fmt.Errorf("failed to read speed mode response: %w", err)
	}
	mode := strings.TrimSpace(string(body))

	reqDl, err := http.NewRequestWithContext(context.Background(), "GET", fmt.Sprintf("%s/api/v2/transfer/downloadLimit", q.baseURL), nil)
	if err != nil {
		return downloader.SpeedLimit{}, fmt.Errorf("failed to create download limit request: %w", err)
	}
	respDl, err := q.doRequestWithRetry(reqDl)
	if err != nil {
		return downloader.SpeedLimit{}, fmt.Errorf("download limit request failed: %w", err)
	}
	defer respDl.Body.Close()
	if respDl.StatusCode != http.StatusOK {
		dlBody, _ := io.ReadAll(respDl.Body)
		return downloader.SpeedLimit{}, fmt.Errorf("download limit request failed with status %d: %s", respDl.StatusCode, string(dlBody))
	}
	dlBody, err := io.ReadAll(respDl.Body)
	if err != nil {
		return downloader.SpeedLimit{}, fmt.Errorf("failed to read download limit response: %w", err)
	}

	reqUl, err := http.NewRequestWithContext(context.Background(), "GET", fmt.Sprintf("%s/api/v2/transfer/uploadLimit", q.baseURL), nil)
	if err != nil {
		return downloader.SpeedLimit{}, fmt.Errorf("failed to create upload limit request: %w", err)
	}
	respUl, err := q.doRequestWithRetry(reqUl)
	if err != nil {
		return downloader.SpeedLimit{}, fmt.Errorf("upload limit request failed: %w", err)
	}
	defer respUl.Body.Close()
	if respUl.StatusCode != http.StatusOK {
		ulBody, _ := io.ReadAll(respUl.Body)
		return downloader.SpeedLimit{}, fmt.Errorf("upload limit request failed with status %d: %s", respUl.StatusCode, string(ulBody))
	}
	ulBody, err := io.ReadAll(respUl.Body)
	if err != nil {
		return downloader.SpeedLimit{}, fmt.Errorf("failed to read upload limit response: %w", err)
	}

	limit := downloader.SpeedLimit{}
	_, _ = fmt.Sscanf(strings.TrimSpace(string(dlBody)), "%d", &limit.DownloadLimit)
	_, _ = fmt.Sscanf(strings.TrimSpace(string(ulBody)), "%d", &limit.UploadLimit)
	limit.LimitEnabled = mode != "0"

	return limit, nil
}

// SetSpeedLimit 设置全局速度限制
func (q *QbitClient) SetSpeedLimit(limit downloader.SpeedLimit) error {
	dataDl := url.Values{}
	dataDl.Set("limit", fmt.Sprintf("%d", limit.DownloadLimit))
	if err := q.postForm("/api/v2/transfer/setDownloadLimit", dataDl); err != nil {
		return err
	}

	dataUl := url.Values{}
	dataUl.Set("limit", fmt.Sprintf("%d", limit.UploadLimit))
	if err := q.postForm("/api/v2/transfer/setUploadLimit", dataUl); err != nil {
		return err
	}

	if current, err := q.GetSpeedLimit(); err == nil && current.LimitEnabled != limit.LimitEnabled {
		if err := q.postForm("/api/v2/transfer/toggleSpeedLimitsMode", url.Values{}); err != nil {
			return err
		}
	}

	return nil
}

// GetClientPaths 获取下载器配置的保存路径列表
func (q *QbitClient) GetClientPaths() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	prefsURL := fmt.Sprintf("%s/api/v2/app/preferences", q.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", prefsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create preferences request: %w", err)
	}

	resp, err := q.doRequestWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("preferences request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("preferences request failed with status code: %d", resp.StatusCode)
	}

	var prefs map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&prefs); err != nil {
		return nil, fmt.Errorf("failed to parse preferences: %w", err)
	}

	var paths []string
	if savePath, ok := prefs["save_path"].(string); ok && savePath != "" {
		paths = append(paths, savePath)
	}

	return paths, nil
}

// GetClientLabels 获取下载器配置的标签列表
func (q *QbitClient) GetClientLabels() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	categoriesURL := fmt.Sprintf("%s/api/v2/torrents/categories", q.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", categoriesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create categories request: %w", err)
	}

	resp, err := q.doRequestWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("categories request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("categories request failed with status code: %d", resp.StatusCode)
	}

	var categories map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&categories); err != nil {
		return nil, fmt.Errorf("failed to parse categories: %w", err)
	}

	labels := make([]string, 0, len(categories))
	for name := range categories {
		labels = append(labels, name)
	}

	return labels, nil
}
