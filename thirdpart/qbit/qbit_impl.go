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
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/global"
	"github.com/zeebo/bencode"
	"go.uber.org/zap"
)

type QbitClient struct {
	BaseURL     string
	Username    string
	Password    string
	Client      *http.Client
	RateLimiter *time.Ticker
	mu          sync.Mutex
}

func NewQbitClient(baseURL, username, password string, rateLimit time.Duration) (*QbitClient, error) {
	jar, _ := cookiejar.New(nil)
	client := &QbitClient{
		BaseURL:     baseURL,
		Username:    username,
		Password:    password,
		Client:      &http.Client{Jar: jar},
		RateLimiter: time.NewTicker(rateLimit),
	}
	if err := client.authenticate(); err != nil {
		return nil, err
	}
	return client, nil
}

func (q *QbitClient) authenticate() error {
	loginURL := fmt.Sprintf("%s/api/v2/auth/login", q.BaseURL)
	data := url.Values{}
	data.Set("username", q.Username)
	data.Set("password", q.Password)
	req, err := http.NewRequest("POST", loginURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return fmt.Errorf("创建登录请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", q.BaseURL)
	resp, err := q.Client.Do(req)
	if err != nil {
		return fmt.Errorf("登录请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("登录失败，状态码: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %v", err)
	}
	if string(body) != "Ok." {
		global.GlobalLogger.Error("登录失败")
		return fmt.Errorf("登录失败，响应: %s", string(body))
	}
	// 获取 Set-Cookie 信息
	cookies := resp.Cookies()
	for _, cookie := range cookies {
		if cookie.Name == "SID" {
			global.GlobalLogger.Debug("获取到 SID")
		}
	}
	global.GlobalLogger.Info("登录qbittorrent成功")
	return nil
}

func (q *QbitClient) GetDiskSpace(c context.Context) (int64, error) {
	// 构建请求 URL
	ctx, cancel := context.WithTimeout(c, 30*time.Second)
	defer cancel()
	diskURL := fmt.Sprintf("%s/api/v2/sync/maindata", q.BaseURL)
	// 使用 ctx 创建请求
	req, err := http.NewRequestWithContext(ctx, "GET", diskURL, nil)
	if err != nil {
		return 0, fmt.Errorf("创建磁盘空间请求失败: %v", err)
	}
	// 发送 HTTP 请求
	resp, err := q.Client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("磁盘空间请求失败: %v", err)
	}
	defer resp.Body.Close()
	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("磁盘空间请求失败，状态码: %d", resp.StatusCode)
	}
	// 解析响应
	var responseData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&responseData); err != nil {
		return 0, fmt.Errorf("解析响应失败: %v", err)
	}
	// 获取磁盘空间信息
	serverState, ok := responseData["server_state"].(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("无法获取 server_state 信息")
	}
	freeSpace, ok := serverState["free_space_on_disk"].(float64)
	if !ok {
		return 0, fmt.Errorf("无法获取磁盘空间信息")
	}
	return int64(freeSpace), nil
}

func (q *QbitClient) GetLastAddedTorrentTime() (time.Time, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	syncURL := fmt.Sprintf("%s/api/v2/sync/maindata", q.BaseURL)
	req, err := http.NewRequest("GET", syncURL, nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("创建同步请求失败: %v", err)
	}
	resp, err := q.Client.Do(req)
	if err != nil {
		return time.Time{}, fmt.Errorf("同步请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return time.Time{}, fmt.Errorf("同步请求失败，状态码: %d", resp.StatusCode)
	}
	var responseData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&responseData); err != nil {
		return time.Time{}, fmt.Errorf("解析响应失败: %v", err)
	}
	torrents, ok := responseData["torrents"].(map[string]interface{})
	if !ok {
		return time.Time{}, fmt.Errorf("无法获取种子信息")
	}
	var lastAdded time.Time
	for _, torrent := range torrents {
		torrentInfo := torrent.(map[string]interface{})
		addedOn, ok := torrentInfo["added_on"].(float64)
		if !ok {
			continue
		}
		torrentTime := time.Unix(int64(addedOn), 0)
		if torrentTime.After(lastAdded) {
			lastAdded = torrentTime
		}
	}
	return lastAdded, nil
}

func (q *QbitClient) AddTorrent(fileData []byte, category, tags string) error {
	skipChecking := false
	paused := true
	<-q.RateLimiter.C // Rate limiting
	q.mu.Lock()
	defer q.mu.Unlock()
	uploadURL := fmt.Sprintf("%s/api/v2/torrents/add", q.BaseURL)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("torrents", "upload.torrent")
	if err != nil {
		return fmt.Errorf("创建 torrent 文件表单失败: %v", err)
	}
	if _, err := part.Write(fileData); err != nil {
		return fmt.Errorf("写入 torrent 文件数据失败: %v", err)
	}
	if category != "" {
		if err := writer.WriteField("category", category); err != nil {
			return fmt.Errorf("写入 category 失败: %v", err)
		}
	}
	if tags != "" {
		if err := writer.WriteField("tags", tags); err != nil {
			return fmt.Errorf("写入 tags 失败: %v", err)
		}
	}
	// if savepath != "" {
	// 	if err := writer.WriteField("savepath", savepath); err != nil {
	// 		return fmt.Errorf("写入 savepath 失败: %v", err)
	// 	}
	// }
	if err := writer.WriteField("skip_checking", fmt.Sprintf("%t", skipChecking)); err != nil {
		return fmt.Errorf("写入 skip_checking 失败: %v", err)
	}
	if err := writer.WriteField("paused", fmt.Sprintf("%t", paused)); err != nil {
		return fmt.Errorf("写入 paused 失败: %v", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("关闭 multipart writer 失败: %v", err)
	}
	req, err := http.NewRequest("POST", uploadURL, body)
	if err != nil {
		return fmt.Errorf("创建上传请求失败: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := q.Client.Do(req)
	if err != nil {
		return fmt.Errorf("上传请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("上传失败，状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (q *QbitClient) CanAddTorrent(ctx context.Context, fileSize int64) (bool, error) {
	freeSpace, err := q.GetDiskSpace(ctx)
	if err != nil {
		return false, err
	}
	if fileSize > freeSpace {
		availableSize := float64(freeSpace) / (1024 * 1024 * 1024)
		needSize := float64(fileSize) / (1024 * 1024 * 1024)
		global.GlobalLogger.Error("空间不足，无法添加种子", zap.Float64("needSizeGB", needSize), zap.Float64("availableSizeGB", availableSize))
		return false, nil
	}
	return true, nil
}

// CheckTorrentExists 检查种子是否存在
func (q *QbitClient) CheckTorrentExists(torrentHash string) (bool, error) {
	propertiesURL := fmt.Sprintf("%s/api/v2/torrents/properties?hash=%s", q.BaseURL, torrentHash)
	req, err := http.NewRequest("GET", propertiesURL, nil)
	if err != nil {
		return false, fmt.Errorf("创建检查请求失败: %v", err)
	}
	resp, err := q.Client.Do(req)
	if err != nil {
		return false, fmt.Errorf("检查请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		global.GlobalLogger.Info("种子不在qbit中,准备添加...", zap.String("torrentHash", torrentHash))
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("检查失败，状态码: %d", resp.StatusCode)
	}
	var props QbitTorrentProperties
	if err := json.NewDecoder(resp.Body).Decode(&props); err != nil {
		return false, fmt.Errorf("解析种子信息失败: %v", err)
	}
	global.GlobalLogger.Info("种子信息", zap.String("savePath", props.SavePath))
	return true, nil
}

func (q *QbitClient) ProcessSingleTorrentFile(ctx context.Context, filePath, category, tags string) error {
	// 检查磁盘空间
	freeSpace, err := q.GetDiskSpace(ctx)
	if err != nil {
		return fmt.Errorf("检查磁盘空间失败: %v", err)
	}
	global.GlobalLogger.Info("可用磁盘空间", zap.Float64("freeSpaceGB", float64(freeSpace)/(1024*1024*1024)))
	// 处理单个种子文件
	err = q.processTorrentFile(ctx, filePath, category, tags)
	if err != nil {
		return fmt.Errorf("处理种子文件失败: %v", err)
	}
	global.GlobalLogger.Info("处理单个种子文件完成", zap.String("filePath", filePath))
	return nil
}

func (q *QbitClient) ProcessTorrentDirectory(ctx context.Context, directory string, category, tags string) error {
	// 检查磁盘空间
	freeSpace, err := q.GetDiskSpace(ctx)
	if err != nil {
		return fmt.Errorf("检查磁盘空间失败: %v", err)
	}
	global.GlobalLogger.Info("可用磁盘空间", zap.Float64("freeSpaceGB", float64(freeSpace)/(1024*1024*1024)))
	// 获取种子文件列表
	torrentFiles, err := GetTorrentFilesPath(directory)
	if err != nil {
		return fmt.Errorf("无法读取目录: %v", err)
	}
	// 处理种子文件
	for _, file := range torrentFiles {
		if err := q.processTorrentFile(ctx, file, category, tags); err != nil {
			global.GlobalLogger.Error("处理种子文件失败", zap.String("file", file), zap.Error(err))
		}
	}
	return nil
}

// 获取目录中所有种子文件
func GetTorrentFilesPath(directory string) ([]string, error) {
	files, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("无法读取目录: %v", err)
	}
	var torrentFiles []string
	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".torrent" {
			torrentFiles = append(torrentFiles, filepath.Join(directory, file.Name()))
		}
	}
	return torrentFiles, nil
}

func (q *QbitClient) processTorrentFile(ctx context.Context, filePath, category, tags string) error {
	global.GlobalLogger.Info("开始处理种子文件", zap.String("filePath", filePath))
	torrentData, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("无法读取种子文件: %v", err)
	}
	torrentHash, err := ComputeTorrentHash(torrentData)
	if err != nil {
		return fmt.Errorf("无法计算种子哈希: %v", err)
	}
	exists, err := q.CheckTorrentExists(torrentHash)
	if err != nil {
		return fmt.Errorf("检查种子失败: %v", err)
	}
	if exists {
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("种子已存在，但删除本地文件失败: %v", err)
		}
		global.GlobalLogger.Info("种子已存在,本地文件删除成功", zap.String("filePath", filePath))
		return nil
	}
	canAdd, err := q.CanAddTorrent(ctx, int64(len(torrentData)))
	if err != nil {
		return fmt.Errorf("无法判断是否可以添加种子: %v", err)
	}
	if !canAdd {
		global.GlobalLogger.Error("磁盘空间不足，跳过种子", zap.String("filePath", filePath))
		return nil
	}
	// 添加种子
	if err := q.AddTorrent(torrentData, category, tags); err != nil {
		return fmt.Errorf("添加种子失败: %v", err)
	}
	global.GlobalLogger.Info("种子添加成功", zap.String("filePath", filePath))
	return nil
}

// ComputeTorrentHash 计算种子的 SHA1 哈希值
func ComputeTorrentHash(data []byte) (string, error) {
	reader := bytes.NewReader(data)
	var torrent map[string]interface{}
	err := bencode.NewDecoder(reader).Decode(&torrent)
	if err != nil {
		return "", fmt.Errorf("failed to decode torrent file: %v", err)
	}
	info, ok := torrent["info"]
	if !ok {
		return "", fmt.Errorf("info section not found in torrent file")
	}
	infoEncoded, err := bencode.EncodeString(info)
	if err != nil {
		return "", fmt.Errorf("failed to encode info dictionary: %v", err)
	}
	hash := sha1.Sum([]byte(infoEncoded))
	return hex.EncodeToString(hash[:]), nil
}

func ComputeTorrentHashWithPath(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("无法读取种子文件: %v", err)
	}
	return ComputeTorrentHash(data)
}
