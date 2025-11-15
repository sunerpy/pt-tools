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

	"github.com/zeebo/bencode"
)

type QbitClient struct {
	BaseURL     string
	Username    string
	Password    string
	Client      *http.Client
	RateLimiter *time.Ticker
	mu          sync.Mutex
}
type DownloadTaskInfo struct {
	Name          string
	Hash          string
	SizeLeft      int64
	DownloadSpeed int64
	ETA           time.Duration
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

func (q *QbitClient) DoRequestWithRetry(req *http.Request) (*http.Response, error) {
	<-q.RateLimiter.C
	resp, err := q.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		// 尝试重新登录
		if err := q.authenticate(); err != nil {
			return nil, fmt.Errorf("re-authentication failed: %w", err)
		}
		// 重新发起请求（复制原始请求）
		newReq := req.Clone(req.Context())
		if req.Body != nil {
			// 如果有请求体，需要支持再次读取（推荐用 bytes.Buffer）
			return nil, fmt.Errorf("cannot retry request with non-rewindable body")
		}
		return q.Client.Do(newReq)
	}
	return resp, nil
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
		sLogger().Error("登录失败")
		return fmt.Errorf("登录失败，响应: %s", string(body))
	}
	// 获取 Set-Cookie 信息
	cookies := resp.Cookies()
	for _, cookie := range cookies {
		if cookie.Name == "SID" {
			sLogger().Debug("获取到 SID")
		}
	}
	sLogger().Info("登录qbittorrent成功")
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
	if _, err = part.Write(fileData); err != nil {
		return fmt.Errorf("写入 torrent 文件数据失败: %v", err)
	}
	if category != "" {
		if err = writer.WriteField("category", category); err != nil {
			return fmt.Errorf("写入 category 失败: %v", err)
		}
	}
	if tags != "" {
		if err = writer.WriteField("tags", tags); err != nil {
			return fmt.Errorf("写入 tags 失败: %v", err)
		}
	}
	// if savepath != "" {
	// 	if err := writer.WriteField("savepath", savepath); err != nil {
	// 		return fmt.Errorf("写入 savepath 失败: %v", err)
	// 	}
	// }
	if err = writer.WriteField("skip_checking", fmt.Sprintf("%t", skipChecking)); err != nil {
		return fmt.Errorf("写入 skip_checking 失败: %v", err)
	}
	if err = writer.WriteField("paused", fmt.Sprintf("%t", paused)); err != nil {
		return fmt.Errorf("写入 paused 失败: %v", err)
	}
	if err = writer.Close(); err != nil {
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
		sLogger().Errorf("空间不足，无法添加种子，所需空间: %.2fGB，当前可用空间: %.2fGB", needSize, availableSize)
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
	resp, err := q.DoRequestWithRetry(req)
	if err != nil {
		return false, fmt.Errorf("检查请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		sLogger().Infof("种子: %s 不在qbit中,准备添加...", torrentHash)
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("检查失败，状态码: %d", resp.StatusCode)
	}
	var props QbitTorrentProperties
	if err := json.NewDecoder(resp.Body).Decode(&props); err != nil {
		return false, fmt.Errorf("解析种子信息失败: %v", err)
	}
	sLogger().Info("种子保存路径: ", props.SavePath)
	return true, nil
}

func (q *QbitClient) ProcessSingleTorrentFile(ctx context.Context, filePath, category, tags string) error {
	// 检查磁盘空间
	freeSpace, err := q.GetDiskSpace(ctx)
	if err != nil {
		return fmt.Errorf("检查磁盘空间失败: %v", err)
	}
	sLogger().Info("可用磁盘空间: ", float64(freeSpace)/(1024*1024*1024))
	// 处理单个种子文件
	err = q.processTorrentFile(ctx, filePath, category, tags)
	if err != nil {
		return fmt.Errorf("处理种子文件失败: %v", err)
	}
	sLogger().Infof("处理单个种子文件完成: %s", filePath)
	return nil
}

func (q *QbitClient) ProcessTorrentDirectory(ctx context.Context, directory string, category, tags string) error {
	// 检查磁盘空间
	freeSpace, err := q.GetDiskSpace(ctx)
	if err != nil {
		return fmt.Errorf("检查磁盘空间失败: %v", err)
	}
	sLogger().Info("可用磁盘空间: ", float64(freeSpace)/(1024*1024*1024))
	// 获取种子文件列表
	torrentFiles, err := GetTorrentFilesPath(directory)
	if err != nil {
		return fmt.Errorf("无法读取目录: %v", err)
	}
	// 处理种子文件
	for _, file := range torrentFiles {
		if err := q.processTorrentFile(ctx, file, category, tags); err != nil {
			sLogger().Error("处理种子文件失败", file, err)
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
	sLogger().Info("开始处理种子文件", filePath)
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
		if err = os.Remove(filePath); err != nil {
			return fmt.Errorf("种子已存在，但删除本地文件失败: %v", err)
		}
		sLogger().Info("种子已存在,本地文件删除成功", filePath)
		return nil
	}
	canAdd, err := q.CanAddTorrent(ctx, int64(len(torrentData)))
	if err != nil {
		return fmt.Errorf("无法判断是否可以添加种子: %v", err)
	}
	if !canAdd {
		sLogger().Error("磁盘空间不足，跳过种子", filePath)
		return nil
	}
	// 添加种子
	if err := q.AddTorrent(torrentData, category, tags); err != nil {
		return fmt.Errorf("添加种子失败: %v", err)
	}
	sLogger().Info("种子添加成功", filePath)
	return nil
}

//	func (q *QbitClient) GetActiveDownloadTasks(ctx context.Context) ([]DownloadTaskInfo, int64, error) {
//		url := fmt.Sprintf("%s/api/v2/sync/maindata", q.BaseURL)
//		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
//		if err != nil {
//			return nil, 0, err
//		}
//		resp, err := q.Client.Do(req)
//		if err != nil {
//			return nil, 0, err
//		}
//		defer resp.Body.Close()
//		var data struct {
//			ServerState struct {
//				DownloadRate int64 `json:"dl_info_speed"`
//			} `json:"server_state"`
//			Torrents map[string]struct {
//				Name          string `json:"name"`
//				AmountLeft    int64  `json:"amount_left"`
//				DownloadSpeed int64  `json:"dlspeed"`
//				State         string `json:"state"`
//			} `json:"torrents"`
//		}
//		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
//			return nil, 0, err
//		}
//		var tasks []DownloadTaskInfo
//		for hash, t := range data.Torrents {
//			if t.AmountLeft > 0 && t.DownloadSpeed > 0 && strings.HasPrefix(t.State, "downloading") {
//				eta := time.Duration(float64(t.AmountLeft)/float64(t.DownloadSpeed)) * time.Second
//				tasks = append(tasks, DownloadTaskInfo{
//					Name:          t.Name,
//					Hash:          hash,
//					SizeLeft:      t.AmountLeft,
//					DownloadSpeed: t.DownloadSpeed,
//					ETA:           eta,
//				})
//			}
//		}
//		return tasks, data.ServerState.DownloadRate, nil
//	}
func (q *QbitClient) WaitAndAddTorrentSmart(ctx context.Context, fileData []byte, category, tags string, maxDuration time.Duration, sharedStats []DownloadTaskInfo, sharedRate int64) error {
	newSize := int64(len(fileData))
	taskCount := len(sharedStats)
	// 平均速度预估（最保守按任务数 + 1）
	estimatedSpeed := float64(sharedRate) / float64(taskCount+1)
	estDuration := time.Duration(float64(newSize)/estimatedSpeed) * time.Second
	if estDuration <= maxDuration {
		sLogger().Infof("种子可以直接添加，预估下载时长: %v", estDuration)
		return q.AddTorrent(fileData, category, tags)
	}
	// 找出 ETA 最小的任务
	var soonestTask *DownloadTaskInfo
	for i := range sharedStats {
		if soonestTask == nil || sharedStats[i].ETA < soonestTask.ETA {
			soonestTask = &sharedStats[i]
		}
	}
	if soonestTask == nil {
		return fmt.Errorf("无法找到 ETA 最短的任务")
	}
	sLogger().Warnf("种子将等待任务 [%s] 完成后再添加，预计等待: %v", soonestTask.Name, soonestTask.ETA)
	// 等待最短 ETA 的任务完成
	timer := time.NewTimer(soonestTask.ETA + 10*time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("等待任务完成被取消: %v", ctx.Err())
	case <-timer.C:
	}
	// 尝试添加
	return q.AddTorrent(fileData, category, tags)
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
