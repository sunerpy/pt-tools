package qbit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// TestQbitClientImplementsDownloader 验证 QbitClient 实现 Downloader 接口
func TestQbitClientImplementsDownloader(t *testing.T) {
	var _ downloader.Downloader = (*QbitClient)(nil)
}

// TestQBitConfigImplementsDownloaderConfig 验证 QBitConfig 实现 DownloaderConfig 接口
func TestQBitConfigImplementsDownloaderConfig(t *testing.T) {
	var _ downloader.DownloaderConfig = (*QBitConfig)(nil)
}

// TestQBitConfigGetters 测试配置 getter 方法
func TestQBitConfigGetters(t *testing.T) {
	config := NewQBitConfig("http://localhost:8080", "admin", "password")

	if config.GetType() != downloader.DownloaderQBittorrent {
		t.Errorf("expected type %s, got %s", downloader.DownloaderQBittorrent, config.GetType())
	}
	if config.GetURL() != "http://localhost:8080" {
		t.Errorf("expected URL 'http://localhost:8080', got '%s'", config.GetURL())
	}
	if config.GetUsername() != "admin" {
		t.Errorf("expected username 'admin', got '%s'", config.GetUsername())
	}
	if config.GetPassword() != "password" {
		t.Errorf("expected password 'password', got '%s'", config.GetPassword())
	}
}

// TestQBitConfigValidation 测试配置验证
func TestQBitConfigValidation(t *testing.T) {
	tests := []struct {
		name      string
		config    *QBitConfig
		expectErr bool
	}{
		{
			name:      "valid config",
			config:    NewQBitConfig("http://localhost:8080", "admin", "password"),
			expectErr: false,
		},
		{
			name:      "empty URL",
			config:    NewQBitConfig("", "admin", "password"),
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

// createMockQbitServer 创建模拟的 qBittorrent 服务器
func createMockQbitServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Ok."))
		case "/api/v2/sync/maindata":
			response := map[string]any{
				"server_state": map[string]any{
					"free_space_on_disk": float64(1024 * 1024 * 1024 * 100), // 100 GB
				},
			}
			json.NewEncoder(w).Encode(response)
		case "/api/v2/torrents/properties":
			hash := r.URL.Query().Get("hash")
			if hash == "existing_hash" {
				response := QbitTorrentProperties{SavePath: "/downloads"}
				json.NewEncoder(w).Encode(response)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		case "/api/v2/torrents/add":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// TestNewQbitClient 测试创建 qBittorrent 客户端
func TestNewQbitClient(t *testing.T) {
	server := createMockQbitServer()
	defer server.Close()

	config := NewQBitConfig(server.URL, "admin", "password")
	client, err := NewQbitClient(config, "test-qbit")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	if client.GetType() != downloader.DownloaderQBittorrent {
		t.Errorf("expected type %s, got %s", downloader.DownloaderQBittorrent, client.GetType())
	}
	if client.GetName() != "test-qbit" {
		t.Errorf("expected name 'test-qbit', got '%s'", client.GetName())
	}
	if !client.IsHealthy() {
		t.Error("expected client to be healthy after successful authentication")
	}
}

// TestQbitClientCanAddTorrent 测试检查是否可以添加种子
func TestQbitClientCanAddTorrent(t *testing.T) {
	server := createMockQbitServer()
	defer server.Close()

	config := NewQBitConfig(server.URL, "admin", "password")
	client, err := NewQbitClient(config, "test-qbit")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	// 测试可以添加小文件
	canAdd, err := client.CanAddTorrent(ctx, 1024*1024*100) // 100 MB
	if err != nil {
		t.Fatalf("failed to check if can add torrent: %v", err)
	}
	if !canAdd {
		t.Error("expected to be able to add small torrent")
	}

	// 测试不能添加超大文件
	canAdd, err = client.CanAddTorrent(ctx, 1024*1024*1024*200) // 200 GB
	if err != nil {
		t.Fatalf("failed to check if can add torrent: %v", err)
	}
	if canAdd {
		t.Error("expected not to be able to add large torrent")
	}
}

// TestQbitClientCheckTorrentExists 测试检查种子是否存在
func TestQbitClientCheckTorrentExists(t *testing.T) {
	server := createMockQbitServer()
	defer server.Close()

	config := NewQBitConfig(server.URL, "admin", "password")
	client, err := NewQbitClient(config, "test-qbit")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// 测试存在的种子
	exists, err := client.CheckTorrentExists("existing_hash")
	if err != nil {
		t.Fatalf("failed to check torrent exists: %v", err)
	}
	if !exists {
		t.Error("expected torrent to exist")
	}

	// 测试不存在的种子
	exists, err = client.CheckTorrentExists("non_existing_hash")
	if err != nil {
		t.Fatalf("failed to check torrent exists: %v", err)
	}
	if exists {
		t.Error("expected torrent not to exist")
	}
}

// TestQbitClientClose 测试关闭客户端
func TestQbitClientClose(t *testing.T) {
	server := createMockQbitServer()
	defer server.Close()

	config := NewQBitConfig(server.URL, "admin", "password")
	client, err := NewQbitClient(config, "test-qbit")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if !client.IsHealthy() {
		t.Error("expected client to be healthy before close")
	}

	err = client.Close()
	if err != nil {
		t.Errorf("failed to close client: %v", err)
	}

	if client.IsHealthy() {
		t.Error("expected client not to be healthy after close")
	}
}

// TestQbitClientAddTorrent 测试添加种子
func TestQbitClientAddTorrent(t *testing.T) {
	server := createMockQbitServer()
	defer server.Close()

	config := NewQBitConfig(server.URL, "admin", "password")
	client, err := NewQbitClient(config, "test-qbit")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// 创建一个简单的种子文件数据
	torrentData := []byte("d8:announce35:http://tracker.example.com/announce4:infod6:lengthi1024e4:name8:test.txt12:piece lengthi16384e6:pieces20:01234567890123456789ee")

	err = client.AddTorrent(torrentData, "test-category", "test-tag")
	if err != nil {
		t.Errorf("failed to add torrent: %v", err)
	}
}

// TestQbitClientAddTorrentWithEmptyCategory 测试添加种子（无分类）
func TestQbitClientAddTorrentWithEmptyCategory(t *testing.T) {
	server := createMockQbitServer()
	defer server.Close()

	config := NewQBitConfig(server.URL, "admin", "password")
	client, err := NewQbitClient(config, "test-qbit")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	torrentData := []byte("d8:announce35:http://tracker.example.com/announce4:infod6:lengthi1024e4:name8:test.txt12:piece lengthi16384e6:pieces20:01234567890123456789ee")

	err = client.AddTorrent(torrentData, "", "")
	if err != nil {
		t.Errorf("failed to add torrent without category: %v", err)
	}
}

// TestQbitClientAuthenticateFailed 测试认证失败
func TestQbitClientAuthenticateFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Fails."))
		}
	}))
	defer server.Close()

	config := NewQBitConfig(server.URL, "wrong", "credentials")
	_, err := NewQbitClient(config, "test-qbit")
	if err == nil {
		t.Error("expected error for failed authentication")
	}
}

// TestQbitClientCheckTorrentExistsWithContext 测试带 context 检查种子
func TestQbitClientCheckTorrentExistsWithContext(t *testing.T) {
	server := createMockQbitServer()
	defer server.Close()

	config := NewQBitConfig(server.URL, "admin", "password")
	client, err := NewQbitClient(config, "test-qbit")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// Type assert to access QbitClient-specific method
	qbitClient, ok := client.(*QbitClient)
	if !ok {
		t.Fatal("expected client to be *QbitClient")
	}

	ctx := context.Background()
	exists, err := qbitClient.CheckTorrentExistsWithContext(ctx, "existing_hash")
	if err != nil {
		t.Fatalf("failed to check torrent exists: %v", err)
	}
	if !exists {
		t.Error("expected torrent to exist")
	}
}

// TestQbitClientReauthenticate 测试重新认证
func TestQbitClientReauthenticate(t *testing.T) {
	authCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			authCount++
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Ok."))
		case "/api/v2/sync/maindata":
			response := map[string]any{
				"server_state": map[string]any{
					"free_space_on_disk": float64(1024 * 1024 * 1024 * 100),
				},
			}
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer server.Close()

	config := NewQBitConfig(server.URL, "admin", "password")
	client, err := NewQbitClient(config, "test-qbit")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// 第一次认证
	if authCount != 1 {
		t.Errorf("expected 1 auth call, got %d", authCount)
	}

	// 再次认证
	err = client.Authenticate()
	if err != nil {
		t.Errorf("failed to reauthenticate: %v", err)
	}
	if authCount != 2 {
		t.Errorf("expected 2 auth calls, got %d", authCount)
	}
}

// TestQbitClientInvalidConfig 测试无效配置
func TestQbitClientInvalidConfig(t *testing.T) {
	config := NewQBitConfig("", "", "")
	_, err := NewQbitClient(config, "test")
	if err == nil {
		t.Error("expected error for invalid config")
	}
}

// createMockQbitServerWithAuthFailure 创建认证失败的模拟服务器
func createMockQbitServerWithAuthFailure() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Fails.")) // 认证失败
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	}))
}

// TestQbitClientAuthenticationFailure 测试认证失败
func TestQbitClientAuthenticationFailure(t *testing.T) {
	server := createMockQbitServerWithAuthFailure()
	defer server.Close()

	config := NewQBitConfig(server.URL, "admin", "wrong_password")
	_, err := NewQbitClient(config, "test-qbit")
	if err == nil {
		t.Error("expected authentication to fail")
	}
}

// TestComputeTorrentHash 测试计算种子哈希
func TestComputeTorrentHash(t *testing.T) {
	// 有效的种子数据
	torrentData := []byte("d8:announce35:http://tracker.example.com/announce4:infod6:lengthi12345e4:name8:test.txt12:piece lengthi16384e6:pieces20:01234567890123456789ee")
	hash, err := ComputeTorrentHash(torrentData)
	if err != nil {
		t.Fatalf("failed to compute hash: %v", err)
	}
	if hash == "" {
		t.Error("expected non-empty hash")
	}
	if len(hash) != 40 { // SHA1 哈希是 40 个十六进制字符
		t.Errorf("expected hash length 40, got %d", len(hash))
	}
}

// TestComputeTorrentHashInvalidData 测试无效种子数据
func TestComputeTorrentHashInvalidData(t *testing.T) {
	// 无效的种子数据
	invalidData := []byte("not a valid torrent")
	_, err := ComputeTorrentHash(invalidData)
	if err == nil {
		t.Error("expected error for invalid torrent data")
	}
}

// TestComputeTorrentHashNoInfo 测试没有 info 部分的种子
func TestComputeTorrentHashNoInfo(t *testing.T) {
	// 没有 info 部分的种子数据
	noInfoData := []byte("d8:announce35:http://tracker.example.com/announcee")
	_, err := ComputeTorrentHash(noInfoData)
	if err == nil {
		t.Error("expected error for torrent without info section")
	}
}

// TestNewQbitClientForTesting 测试创建测试用客户端
func TestNewQbitClientForTesting(t *testing.T) {
	httpClient := &http.Client{}
	client := NewQbitClientForTesting(httpClient, "http://localhost:8080")

	if client.GetName() != "test-client" {
		t.Errorf("expected name 'test-client', got '%s'", client.GetName())
	}
	if !client.IsHealthy() {
		t.Error("expected client to be healthy")
	}
	if client.baseURL != "http://localhost:8080" {
		t.Errorf("expected baseURL 'http://localhost:8080', got '%s'", client.baseURL)
	}
}

// createMockQbitServerWithInvalidDiskSpaceResponse 创建返回无效磁盘空间响应的模拟服务器
func createMockQbitServerWithInvalidDiskSpaceResponse() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Ok."))
		case "/api/v2/sync/maindata":
			// 返回没有 server_state 的响应
			response := map[string]any{
				"other_field": "value",
			}
			json.NewEncoder(w).Encode(response)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// TestDoRequestWithRetry_Forbidden 测试 403 重试
func TestDoRequestWithRetry_Forbidden(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Ok."))
		case "/api/v2/test":
			callCount++
			if callCount == 1 {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		}
	}))
	defer server.Close()

	config := NewQBitConfig(server.URL, "admin", "password")
	client, err := NewQbitClient(config, "test-qbit")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	qbitClient := client.(*QbitClient)
	req, _ := http.NewRequest("GET", server.URL+"/api/v2/test", nil)
	resp, err := qbitClient.doRequestWithRetry(req)
	if err != nil {
		t.Fatalf("doRequestWithRetry failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

// TestProcessSingleTorrentFile 测试处理单个种子文件
func TestProcessSingleTorrentFile(t *testing.T) {
	server := createMockQbitServer()
	defer server.Close()

	config := NewQBitConfig(server.URL, "admin", "password")
	client, err := NewQbitClient(config, "test-qbit")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// 创建临时种子文件
	dir := t.TempDir()
	torrentData := []byte("d8:announce35:http://tracker.example.com/announce4:infod6:lengthi1024e4:name8:test.txt12:piece lengthi16384e6:pieces20:01234567890123456789ee")
	torrentPath := dir + "/test.torrent"
	if writeErr := writeFile(torrentPath, torrentData); writeErr != nil {
		t.Fatalf("failed to write torrent file: %v", writeErr)
	}

	ctx := context.Background()
	err = client.ProcessSingleTorrentFile(ctx, torrentPath, "test-cat", "test-tag")
	if err != nil {
		t.Errorf("ProcessSingleTorrentFile failed: %v", err)
	}
}

// TestProcessTorrentDirectory 测试处理种子目录
func TestProcessTorrentDirectory(t *testing.T) {
	server := createMockQbitServer()
	defer server.Close()

	config := NewQBitConfig(server.URL, "admin", "password")
	client, err := NewQbitClient(config, "test-qbit")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	qbitClient := client.(*QbitClient)

	// 创建临时目录和种子文件
	dir := t.TempDir()
	torrentData := []byte("d8:announce35:http://tracker.example.com/announce4:infod6:lengthi1024e4:name8:test.txt12:piece lengthi16384e6:pieces20:01234567890123456789ee")
	if writeErr := writeFile(dir+"/test1.torrent", torrentData); writeErr != nil {
		t.Fatalf("failed to write torrent file: %v", writeErr)
	}
	if writeErr := writeFile(dir+"/test2.torrent", torrentData); writeErr != nil {
		t.Fatalf("failed to write torrent file: %v", writeErr)
	}

	ctx := context.Background()
	err = qbitClient.ProcessTorrentDirectory(ctx, dir, "test-cat", "test-tag")
	if err != nil {
		t.Errorf("ProcessTorrentDirectory failed: %v", err)
	}
}

// TestGetTorrentFilesPath 测试获取种子文件路径
func TestGetTorrentFilesPath(t *testing.T) {
	dir := t.TempDir()

	// 创建一些文件
	writeFile(dir+"/test1.torrent", []byte("data"))
	writeFile(dir+"/test2.torrent", []byte("data"))
	writeFile(dir+"/other.txt", []byte("data"))

	files, err := GetTorrentFilesPath(dir)
	if err != nil {
		t.Fatalf("GetTorrentFilesPath failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("expected 2 torrent files, got %d", len(files))
	}
}

// TestGetTorrentFilesPath_EmptyDir 测试空目录
func TestGetTorrentFilesPath_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	files, err := GetTorrentFilesPath(dir)
	if err != nil {
		t.Fatalf("GetTorrentFilesPath failed: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("expected 0 torrent files, got %d", len(files))
	}
}

// TestGetTorrentFilesPath_InvalidDir 测试无效目录
func TestGetTorrentFilesPath_InvalidDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping test that may have different behavior when running as root")
	}
	_, err := GetTorrentFilesPath("/nonexistent/path")
	if err == nil {
		t.Error("expected error for invalid directory")
	}
}

// TestComputeTorrentHashWithPath 测试从文件路径计算哈希
func TestComputeTorrentHashWithPath(t *testing.T) {
	dir := t.TempDir()
	torrentData := []byte("d8:announce35:http://tracker.example.com/announce4:infod6:lengthi1024e4:name8:test.txt12:piece lengthi16384e6:pieces20:01234567890123456789ee")
	torrentPath := dir + "/test.torrent"
	if err := writeFile(torrentPath, torrentData); err != nil {
		t.Fatalf("failed to write torrent file: %v", err)
	}

	hash, err := ComputeTorrentHashWithPath(torrentPath)
	if err != nil {
		t.Fatalf("ComputeTorrentHashWithPath failed: %v", err)
	}

	if hash == "" {
		t.Error("expected non-empty hash")
	}
}

// TestComputeTorrentHashWithPath_InvalidPath 测试无效路径
func TestComputeTorrentHashWithPath_InvalidPath(t *testing.T) {
	_, err := ComputeTorrentHashWithPath("/nonexistent/path.torrent")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

// TestAuthenticateWithContext 测试带 context 的认证
func TestAuthenticateWithContext(t *testing.T) {
	server := createMockQbitServer()
	defer server.Close()

	config := NewQBitConfig(server.URL, "admin", "password")
	client, err := NewQbitClient(config, "test-qbit")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	qbitClient := client.(*QbitClient)
	ctx := context.Background()
	err = qbitClient.AuthenticateWithContext(ctx)
	if err != nil {
		t.Errorf("AuthenticateWithContext failed: %v", err)
	}
}

// writeFile 辅助函数写入文件
func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

// TestProperty1_AutoStartPausedParameterConsistency_QBit 属性测试：Auto-start Paused Parameter Consistency (qBittorrent)
// Property 1: 对于任何 auto_start 配置值，添加种子时发送到下载器 API 的 paused 参数应等于 !auto_start
// Validates: Requirements 1.2, 1.4, 1.5
func TestProperty1_AutoStartPausedParameterConsistency_QBit(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("paused parameter equals !auto_start for qBittorrent", prop.ForAll(
		func(autoStart bool) bool {
			var capturedPaused string

			// 创建捕获 paused 参数的模拟服务器
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v2/auth/login":
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("Ok."))
				case "/api/v2/torrents/add":
					r.ParseMultipartForm(10 << 20)
					capturedPaused = r.FormValue("paused")
					w.WriteHeader(http.StatusOK)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			// 使用指定的 autoStart 值创建配置
			config := NewQBitConfigWithAutoStart(server.URL, "admin", "password", autoStart)
			client, err := NewQbitClient(config, "test-qbit")
			if err != nil {
				t.Logf("Failed to create client: %v", err)
				return false
			}
			defer client.Close()

			// 添加种子
			torrentData := []byte("test-torrent-data")
			err = client.AddTorrent(torrentData, "test-cat", "test-tag")
			if err != nil {
				t.Logf("Failed to add torrent: %v", err)
				return false
			}

			// 验证 paused 参数
			expectedPaused := "true"
			if autoStart {
				expectedPaused = "false"
			}

			return capturedPaused == expectedPaused
		},
		gen.Bool(),
	))

	properties.TestingRun(t)
}

// TestQBitConfigAutoStart 测试 auto_start 配置
func TestQBitConfigAutoStart(t *testing.T) {
	tests := []struct {
		name      string
		autoStart bool
	}{
		{"auto_start true", true},
		{"auto_start false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewQBitConfigWithAutoStart("http://localhost:8080", "admin", "password", tt.autoStart)
			if config.GetAutoStart() != tt.autoStart {
				t.Errorf("expected auto_start %v, got %v", tt.autoStart, config.GetAutoStart())
			}
		})
	}
}

// TestQBitConfigDefaultAutoStart 测试默认 auto_start 值
func TestQBitConfigDefaultAutoStart(t *testing.T) {
	config := NewQBitConfig("http://localhost:8080", "admin", "password")
	if config.GetAutoStart() != false {
		t.Errorf("expected default auto_start to be false, got %v", config.GetAutoStart())
	}
}

// TestQbitClientAddTorrentWithPath 测试添加种子并指定下载路径
func TestQbitClientAddTorrentWithPath(t *testing.T) {
	var capturedSavePath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Ok."))
		case "/api/v2/torrents/add":
			r.ParseMultipartForm(10 << 20)
			capturedSavePath = r.FormValue("savepath")
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := NewQBitConfig(server.URL, "admin", "password")
	client, err := NewQbitClient(config, "test-qbit")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	torrentData := []byte("d8:announce35:http://tracker.example.com/announce4:infod6:lengthi1024e4:name8:test.txt12:piece lengthi16384e6:pieces20:01234567890123456789ee")

	// 测试带下载路径
	err = client.AddTorrentWithPath(torrentData, "test-category", "test-tag", "/custom/download/path")
	if err != nil {
		t.Errorf("failed to add torrent with path: %v", err)
	}
	if capturedSavePath != "/custom/download/path" {
		t.Errorf("expected savepath '/custom/download/path', got '%s'", capturedSavePath)
	}
}

// TestQbitClientAddTorrentWithEmptyPath 测试添加种子不指定下载路径
func TestQbitClientAddTorrentWithEmptyPath(t *testing.T) {
	var capturedSavePath string
	var savepathSet bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Ok."))
		case "/api/v2/torrents/add":
			r.ParseMultipartForm(10 << 20)
			capturedSavePath = r.FormValue("savepath")
			savepathSet = capturedSavePath != ""
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := NewQBitConfig(server.URL, "admin", "password")
	client, err := NewQbitClient(config, "test-qbit")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	torrentData := []byte("d8:announce35:http://tracker.example.com/announce4:infod6:lengthi1024e4:name8:test.txt12:piece lengthi16384e6:pieces20:01234567890123456789ee")

	// 测试不带下载路径
	err = client.AddTorrentWithPath(torrentData, "test-category", "test-tag", "")
	if err != nil {
		t.Errorf("failed to add torrent without path: %v", err)
	}
	if savepathSet {
		t.Errorf("expected savepath not to be set, but got '%s'", capturedSavePath)
	}
}

// TestProperty_DownloadPathParameter_QBit 属性测试：Download Path Parameter (qBittorrent)
// Property: 当 downloadPath 非空时，savepath 参数应该被设置为该值；当为空时，不应设置 savepath
// Validates: Requirements 2.2, 2.7
func TestProperty_DownloadPathParameter_QBit(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("savepath parameter is set correctly for qBittorrent", prop.ForAll(
		func(downloadPath string) bool {
			var capturedSavePath string
			var savepathFieldExists bool

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v2/auth/login":
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("Ok."))
				case "/api/v2/torrents/add":
					r.ParseMultipartForm(10 << 20)
					capturedSavePath = r.FormValue("savepath")
					savepathFieldExists = capturedSavePath != ""
					w.WriteHeader(http.StatusOK)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			config := NewQBitConfig(server.URL, "admin", "password")
			client, err := NewQbitClient(config, "test-qbit")
			if err != nil {
				t.Logf("Failed to create client: %v", err)
				return false
			}
			defer client.Close()

			torrentData := []byte("test-torrent-data")
			err = client.AddTorrentWithPath(torrentData, "cat", "tag", downloadPath)
			if err != nil {
				t.Logf("Failed to add torrent: %v", err)
				return false
			}

			// 验证：非空路径应该设置 savepath，空路径不应设置
			if downloadPath != "" {
				return savepathFieldExists && capturedSavePath == downloadPath
			}
			return !savepathFieldExists
		},
		gen.OneConstOf("", "/downloads/movies", "/data/tv", "D:\\Downloads"),
	))

	properties.TestingRun(t)
}

// TestAddTorrentDelegatesToAddTorrentWithPath 测试 AddTorrent 委托给 AddTorrentWithPath
func TestAddTorrentDelegatesToAddTorrentWithPath(t *testing.T) {
	var capturedSavePath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Ok."))
		case "/api/v2/torrents/add":
			r.ParseMultipartForm(10 << 20)
			capturedSavePath = r.FormValue("savepath")
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := NewQBitConfig(server.URL, "admin", "password")
	client, err := NewQbitClient(config, "test-qbit")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	torrentData := []byte("test-torrent-data")

	// 使用 AddTorrent（不带路径）
	err = client.AddTorrent(torrentData, "cat", "tag")
	if err != nil {
		t.Errorf("failed to add torrent: %v", err)
	}

	// 验证 savepath 未设置
	if capturedSavePath != "" {
		t.Errorf("expected savepath to be empty when using AddTorrent, got '%s'", capturedSavePath)
	}
}

func TestQBitConfigURLTrailingSlash(t *testing.T) {
	tests := []struct {
		inputURL    string
		expectedURL string
	}{
		{"http://localhost:8080/", "http://localhost:8080"},
		{"http://localhost:8080", "http://localhost:8080"},
		{"http://192.168.1.100:8080/", "http://192.168.1.100:8080"},
		{"https://qbit.example.com/", "https://qbit.example.com"},
	}

	for _, tt := range tests {
		config := NewQBitConfig(tt.inputURL, "admin", "password")
		if config.GetURL() != tt.expectedURL {
			t.Errorf("GetURL(%q) = %q, want %q", tt.inputURL, config.GetURL(), tt.expectedURL)
		}
	}
}

// failStatusServer responds to every request with the given status code so
// callers hit their non-success error branch.
func failStatusServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// coverageTestClient builds a QbitClient pointed at the given test-server URL,
// bypassing NewQbitClient's network Authenticate() so per-method tests stay
// fast and hermetic. version==5.2.0+ toggles the v520 success-status path.
func coverageTestClient(baseURL string, v520 bool) *QbitClient {
	c := &QbitClient{
		name:    "test-qbit",
		baseURL: baseURL,
		client:  &standardHTTPDoer{client: &http.Client{}},
		healthy: true,
	}
	c.versionMu.Lock()
	c.isV520Plus = v520
	if v520 {
		c.appVersion = "v5.2.0"
	}
	c.versionMu.Unlock()
	return c
}

// qbitAddServer serves auth + add + properties(404) so file-processing flows
// can exercise CheckTorrentExists -> CanAddTorrent -> AddTorrent end-to-end.
func qbitAddServer(t *testing.T, existing bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case r.URL.Path == "/api/v2/sync/maindata":
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":107374182400}}`))
		case strings.HasPrefix(r.URL.Path, "/api/v2/torrents/properties"):
			if existing {
				_, _ = w.Write([]byte(`{"save_path":"/downloads"}`))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		case r.URL.Path == "/api/v2/torrents/add":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// assertErr is a tiny error helper carrying the given message.
type assertErr string

func (e assertErr) Error() string { return string(e) }

// reauthDoer forces the first request to return 403, then delegates to the
// wrapped client so doRequestWithRetry's re-auth branch is exercised.
type reauthDoer struct {
	inner   *standardHTTPDoer
	tripped atomic.Bool
}

func (d *reauthDoer) Do(req *http.Request) (*http.Response, error) {
	if !d.tripped.Swap(true) {
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Body:       http.NoBody,
			Header:     make(http.Header),
			Request:    req,
		}, nil
	}
	return d.inner.Do(req)
}

func TestQbitParseQBitVersion_Branches(t *testing.T) {
	maj, min, patch, ok := parseQBitVersion("v5.2.1")
	require.True(t, ok)
	assert.Equal(t, 5, maj)
	assert.Equal(t, 2, min)
	assert.Equal(t, 1, patch)

	_, _, _, ok = parseQBitVersion("not-a-version")
	assert.False(t, ok)
}

func TestQbitAuthenticate_204NoContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(http.StatusNoContent)
		case "/api/v2/app/version":
			_, _ = w.Write([]byte("v5.2.0"))
		}
	}))
	defer srv.Close()

	c := coverageTestClient(srv.URL, false)
	c.username = "admin"
	c.password = "pw"
	require.NoError(t, c.Authenticate())
	assert.True(t, c.healthy)
}

func TestQbitAuthenticate_200ThenVersionDetected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			_, _ = w.Write([]byte("v5.2.1"))
		}
	}))
	defer srv.Close()

	c := coverageTestClient(srv.URL, false)
	require.NoError(t, c.Authenticate())
	assert.True(t, c.healthy)
	c.versionMu.RLock()
	defer c.versionMu.RUnlock()
	assert.True(t, c.isV520Plus)
}

func TestQbitAuthenticate_200ButVersionHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			_, _ = w.Write([]byte("<!DOCTYPE html><html>x</html>"))
		}
	}))
	defer srv.Close()

	c := coverageTestClient(srv.URL, false)
	err := c.Authenticate()
	require.Error(t, err)
}

func TestQbitDetectVersion_Branches(t *testing.T) {
	t.Run("legacy version", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("v4.6.0"))
		}))
		defer srv.Close()
		c := coverageTestClient(srv.URL, false)
		require.NoError(t, c.detectVersion(context.Background()))
		c.versionMu.RLock()
		defer c.versionMu.RUnlock()
		assert.False(t, c.isV520Plus)
	})

	t.Run("empty version body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("   "))
		}))
		defer srv.Close()
		require.NoError(t, coverageTestClient(srv.URL, false).detectVersion(context.Background()))
	})

	t.Run("invalid version", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("garbage"))
		}))
		defer srv.Close()
		require.NoError(t, coverageTestClient(srv.URL, false).detectVersion(context.Background()))
	})

	t.Run("non-2xx status", func(t *testing.T) {
		srv := failStatusServer(t, http.StatusInternalServerError)
		require.NoError(t, coverageTestClient(srv.URL, false).detectVersion(context.Background()))
	})

	t.Run("html returns wrong-server error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("<!DOCTYPE html><html>x</html>"))
		}))
		defer srv.Close()
		require.Error(t, coverageTestClient(srv.URL, false).detectVersion(context.Background()))
	})

	t.Run("connection error is non-fatal", func(t *testing.T) {
		require.NoError(t, coverageTestClient("http://127.0.0.1:1", false).detectVersion(context.Background()))
	})
}

func TestQbitEnsureTorrentStarted_ResumesPaused(t *testing.T) {
	var resumed bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[{"name":"t","state":"pausedDL"}]`))
		case "/api/v2/torrents/resume":
			resumed = true
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	c := coverageTestClient(srv.URL, false)
	c.autoStart = true
	require.NoError(t, c.EnsureTorrentStarted("hash1"))
	assert.True(t, resumed, "paused torrent must be resumed under autoStart")
}

func TestQbitEnsureTorrentStarted_AlreadyRunning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/torrents/info" {
			_, _ = w.Write([]byte(`[{"name":"t","state":"downloading"}]`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := coverageTestClient(srv.URL, false)
	c.autoStart = true
	require.NoError(t, c.EnsureTorrentStarted("hash1"))
}

func TestQbitGetClientVersion_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("v4.6.5"))
	}))
	defer srv.Close()
	v, err := coverageTestClient(srv.URL, false).GetClientVersion()
	require.NoError(t, err)
	assert.Equal(t, "v4.6.5", v)
}

func TestQbitCheckTorrentExists_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"hash":"h1","name":"t"}`))
	}))
	defer srv.Close()
	got, err := coverageTestClient(srv.URL, false).CheckTorrentExists("h1")
	require.NoError(t, err)
	assert.True(t, got)
}

func TestQbitGetTorrentsBy_UnfilteredReturnsAll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"hash":"h1","name":"t1","state":"downloading"}]`))
	}))
	defer srv.Close()
	got, err := coverageTestClient(srv.URL, false).GetTorrentsBy(downloader.TorrentFilter{})
	require.NoError(t, err)
	require.Len(t, got, 1)
}

func TestQbitGetAllTorrents_MapsRichFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{
			"hash":"h1","name":"t1","state":"uploading","progress":1.0,
			"dlspeed":10,"upspeed":20,"size":1024,"downloaded":2048,"uploaded":4096,
			"ratio":2.0,"added_on":1600000000,"save_path":"/dl","category":"movies",
			"tags":"a,b","num_seeds":5,"num_leechs":3,"tracker":"http://tr1","eta":100,
			"completion_on":1600001000
		}]`))
	}))
	defer srv.Close()

	torrents, err := coverageTestClient(srv.URL, false).GetAllTorrents()
	require.NoError(t, err)
	require.Len(t, torrents, 1)
	assert.Equal(t, "h1", torrents[0].InfoHash)
	assert.Equal(t, downloader.TorrentSeeding, torrents[0].State)
}

func TestQbitGetAllTorrentsAndMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"hash": "h1", "name": "torrent-one", "progress": float64(1.0),
				"ratio": float64(2.5), "added_on": float64(1600000000),
				"save_path": "/downloads", "category": "movies", "tags": "hd,new",
				"state": "uploading", "size": float64(1024), "amount_left": float64(0),
				"upspeed": float64(10), "dlspeed": float64(20), "uploaded": float64(2048),
				"downloaded": float64(4096), "eta": float64(0), "seeding_time": float64(3600),
				"tracker": "http://t.example", "completion_on": float64(1600001000),
				"num_seeds": float64(5), "num_leechs": float64(3), "availability": float64(1.5),
				"content_path": "/downloads/torrent-one",
			},
			{"hash": "h2", "name": "torrent-two", "progress": float64(0.5), "state": "downloading"},
		})
	}))
	defer srv.Close()

	torrents, err := coverageTestClient(srv.URL, false).GetAllTorrents()
	require.NoError(t, err)
	require.Len(t, torrents, 2)

	first := torrents[0]
	assert.Equal(t, "h1", first.ID)
	assert.Equal(t, "h1", first.InfoHash)
	assert.Equal(t, "torrent-one", first.Name)
	assert.True(t, first.IsCompleted)
	assert.Equal(t, 2.5, first.Ratio)
	assert.Equal(t, "/downloads", first.SavePath)
	assert.Equal(t, "movies", first.Category)
	assert.Equal(t, "movies", first.Label)
	assert.Equal(t, "hd,new", first.Tags)
	assert.Equal(t, downloader.TorrentSeeding, first.State)
	assert.Equal(t, int64(1024), first.TotalSize)
	assert.Equal(t, int64(10), first.UploadSpeed)
	assert.Equal(t, int64(20), first.DownloadSpeed)
	assert.Equal(t, int64(2048), first.TotalUploaded)
	assert.Equal(t, int64(4096), first.TotalDownloaded)
	assert.Equal(t, int64(3600), first.SeedingTime)
	assert.Equal(t, "http://t.example", first.Tracker)
	assert.Equal(t, 5, first.NumSeeds)
	assert.Equal(t, 3, first.NumPeers)
	assert.Equal(t, "test-qbit", first.ClientID)

	assert.False(t, torrents[1].IsCompleted)
	assert.Equal(t, downloader.TorrentDownloading, torrents[1].State)
}

func TestQbitMapQbitState(t *testing.T) {
	c := coverageTestClient("http://x", false)
	cases := map[string]downloader.TorrentState{
		"downloading":        downloader.TorrentDownloading,
		"forcedDL":           downloader.TorrentDownloading,
		"uploading":          downloader.TorrentSeeding,
		"forcedUP":           downloader.TorrentSeeding,
		"pausedDL":           downloader.TorrentPaused,
		"pausedUP":           downloader.TorrentPaused,
		"stoppedDL":          downloader.TorrentStopped,
		"stoppedUP":          downloader.TorrentStopped,
		"queuedDL":           downloader.TorrentQueued,
		"queuedUP":           downloader.TorrentQueued,
		"checkingDL":         downloader.TorrentChecking,
		"checkingResumeData": downloader.TorrentChecking,
		"error":              downloader.TorrentError,
		"missingFiles":       downloader.TorrentError,
		"somethingElse":      downloader.TorrentUnknown,
	}
	for state, want := range cases {
		assert.Equal(t, want, c.mapQbitState(state), state)
	}
}

func TestQbitGetTorrentsByAndGetTorrent(t *testing.T) {
	body := []map[string]any{
		{"hash": "h1", "name": "a", "progress": float64(1.0), "state": "uploading"},
		{"hash": "h2", "name": "b", "progress": float64(0.3), "state": "downloading"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer srv.Close()
	c := coverageTestClient(srv.URL, false)

	t.Run("no filter returns all", func(t *testing.T) {
		got, err := c.GetTorrentsBy(downloader.TorrentFilter{})
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})

	t.Run("filter by hash", func(t *testing.T) {
		got, err := c.GetTorrentsBy(downloader.TorrentFilter{Hashes: []string{"h2"}})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "h2", got[0].InfoHash)
	})

	t.Run("filter by id", func(t *testing.T) {
		got, err := c.GetTorrentsBy(downloader.TorrentFilter{IDs: []string{"h1"}})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "h1", got[0].ID)
	})

	t.Run("filter by complete", func(t *testing.T) {
		done := true
		got, err := c.GetTorrentsBy(downloader.TorrentFilter{Complete: &done})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.True(t, got[0].IsCompleted)
	})

	t.Run("filter by state", func(t *testing.T) {
		state := downloader.TorrentDownloading
		got, err := c.GetTorrentsBy(downloader.TorrentFilter{State: &state})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, downloader.TorrentDownloading, got[0].State)
	})

	t.Run("GetTorrent found", func(t *testing.T) {
		got, err := c.GetTorrent("h1")
		require.NoError(t, err)
		assert.Equal(t, "h1", got.InfoHash)
	})

	t.Run("GetTorrent not found", func(t *testing.T) {
		_, err := c.GetTorrent("missing")
		require.ErrorIs(t, err, downloader.ErrTorrentNotFound)
	})
}

func TestQbitPauseResumeRemove(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := coverageTestClient(srv.URL, true)

	require.NoError(t, c.PauseTorrent("h1"))
	require.NoError(t, c.ResumeTorrent("h1"))
	require.NoError(t, c.RemoveTorrent("h1", true))
	require.NoError(t, c.PauseTorrents([]string{"h1", "h2"}))
	require.NoError(t, c.ResumeTorrents([]string{"h1", "h2"}))
	require.NoError(t, c.RemoveTorrents([]string{"h1"}, false))

	assert.Contains(t, paths, "/api/v2/torrents/stop")
	assert.Contains(t, paths, "/api/v2/torrents/start")
	assert.Contains(t, paths, "/api/v2/torrents/delete")
}

func TestQbitSetters(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := coverageTestClient(srv.URL, false)

	require.NoError(t, c.SetTorrentCategory("h1", "movies"))
	require.NoError(t, c.SetTorrentTags("h1", "tag1"))
	require.NoError(t, c.SetTorrentSavePath("h1", "/new/path"))
	require.NoError(t, c.RecheckTorrent("h1"))

	assert.Contains(t, seen, "/api/v2/torrents/setCategory")
	assert.Contains(t, seen, "/api/v2/torrents/addTags")
	assert.Contains(t, seen, "/api/v2/torrents/setLocation")
	assert.Contains(t, seen, "/api/v2/torrents/recheck")
}

func TestQbitGetTorrentTrackers(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Contains(t, r.URL.Path, "/api/v2/torrents/trackers")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"url": "http://tracker1", "status": float64(2), "num_peers": float64(10),
					"num_seeds": float64(5), "num_leeches": float64(3), "msg": "working",
				},
				{"url": "http://tracker2", "message": "alt-message-field"},
			})
		}))
		defer srv.Close()

		trackers, err := coverageTestClient(srv.URL, false).GetTorrentTrackers("h1")
		require.NoError(t, err)
		require.Len(t, trackers, 2)
		assert.Equal(t, "http://tracker1", trackers[0].URL)
		assert.Equal(t, 2, trackers[0].Status)
		assert.Equal(t, 10, trackers[0].Peers)
		assert.Equal(t, 5, trackers[0].Seeds)
		assert.Equal(t, 3, trackers[0].Leeches)
		assert.Equal(t, "working", trackers[0].Message)
		assert.Equal(t, "alt-message-field", trackers[1].Message)
	})

	t.Run("error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetTorrentTrackers("h1")
		require.Error(t, err)
	})
}
