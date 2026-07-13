package transmission

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

// TestTransmissionClientImplementsDownloader 验证 TransmissionClient 实现 Downloader 接口
func TestTransmissionClientImplementsDownloader(t *testing.T) {
	var _ downloader.Downloader = (*TransmissionClient)(nil)
}

// TestTransmissionConfigImplementsDownloaderConfig 验证 TransmissionConfig 实现 DownloaderConfig 接口
func TestTransmissionConfigImplementsDownloaderConfig(t *testing.T) {
	var _ downloader.DownloaderConfig = (*TransmissionConfig)(nil)
}

// TestTransmissionConfigGetters 测试配置 getter 方法
func TestTransmissionConfigGetters(t *testing.T) {
	config := NewTransmissionConfig("http://localhost:9091", "admin", "password")

	if config.GetType() != downloader.DownloaderTransmission {
		t.Errorf("expected type %s, got %s", downloader.DownloaderTransmission, config.GetType())
	}
	if config.GetURL() != "http://localhost:9091" {
		t.Errorf("expected URL 'http://localhost:9091', got '%s'", config.GetURL())
	}
	if config.GetUsername() != "admin" {
		t.Errorf("expected username 'admin', got '%s'", config.GetUsername())
	}
	if config.GetPassword() != "password" {
		t.Errorf("expected password 'password', got '%s'", config.GetPassword())
	}
}

// TestTransmissionConfigValidation 测试配置验证
func TestTransmissionConfigValidation(t *testing.T) {
	tests := []struct {
		name      string
		config    *TransmissionConfig
		expectErr bool
	}{
		{
			name:      "valid config",
			config:    NewTransmissionConfig("http://localhost:9091", "admin", "password"),
			expectErr: false,
		},
		{
			name:      "empty URL",
			config:    NewTransmissionConfig("", "admin", "password"),
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

// createMockTransmissionServer 创建模拟的 Transmission 服务器
func createMockTransmissionServer(authRequired bool, validUser, validPass string) *httptest.Server {
	sessionID := "test-session-id"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 检查 session ID
		if r.Header.Get("X-Transmission-Session-Id") != sessionID {
			w.Header().Set("X-Transmission-Session-Id", sessionID)
			w.WriteHeader(http.StatusConflict)
			return
		}

		// 检查认证
		if authRequired {
			user, pass, ok := r.BasicAuth()
			if !ok || user != validUser || pass != validPass {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}

		// 解析请求
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var resp rpcResponse
		resp.Result = "success"

		switch req.Method {
		case "session-get":
			args := map[string]any{
				"download-dir": "/downloads",
			}
			resp.Arguments, _ = json.Marshal(args)
		case "free-space":
			args := freeSpaceResponse{
				Path:      "/downloads",
				SizeBytes: 1024 * 1024 * 1024 * 100, // 100 GB
			}
			resp.Arguments, _ = json.Marshal(args)
		case "torrent-add":
			args := torrentAddResponse{
				TorrentAdded: &torrentInfo{
					ID:         1,
					Name:       "test-torrent",
					HashString: "abc123",
				},
			}
			resp.Arguments, _ = json.Marshal(args)
		case "torrent-get":
			args := torrentGetResponse{
				Torrents: []torrentStatus{
					{ID: 1, Name: "existing-torrent", HashString: "existing_hash", Status: 4},
				},
			}
			resp.Arguments, _ = json.Marshal(args)
		}

		json.NewEncoder(w).Encode(resp)
	}))
}

// TestProperty10_AuthenticationErrorDescriptiveness 属性测试：认证错误描述性
// Feature: downloader-site-extensibility, Property 10: Authentication Error Descriptiveness
// 对于任何无效凭据，认证应返回描述性错误
func TestProperty10_AuthenticationErrorDescriptiveness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// 属性：无效凭据应返回包含有意义信息的错误
	properties.Property("invalid credentials return descriptive error", prop.ForAll(
		func(wrongUser, wrongPass string) bool {
			server := createMockTransmissionServer(true, "correct_user", "correct_pass")
			defer server.Close()

			config := NewTransmissionConfig(server.URL, wrongUser, wrongPass)
			_, err := NewTransmissionClient(config, "test")

			if err == nil {
				// 如果碰巧生成了正确的凭据，跳过
				if wrongUser == "correct_user" && wrongPass == "correct_pass" {
					return true
				}
				return false
			}

			// 错误消息应该是描述性的
			errMsg := err.Error()
			return strings.Contains(errMsg, "authentication") ||
				strings.Contains(errMsg, "unauthorized") ||
				strings.Contains(errMsg, "invalid") ||
				strings.Contains(errMsg, "failed")
		},
		gen.AlphaString(),
		gen.AlphaString(),
	))

	properties.TestingRun(t)
}

// TestNewTransmissionClient 测试创建 Transmission 客户端
func TestNewTransmissionClient(t *testing.T) {
	server := createMockTransmissionServer(false, "", "")
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test-transmission")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	if client.GetType() != downloader.DownloaderTransmission {
		t.Errorf("expected type %s, got %s", downloader.DownloaderTransmission, client.GetType())
	}
	if client.GetName() != "test-transmission" {
		t.Errorf("expected name 'test-transmission', got '%s'", client.GetName())
	}
	if !client.IsHealthy() {
		t.Error("expected client to be healthy after successful authentication")
	}
}

// TestTransmissionClientCanAddTorrent 测试检查是否可以添加种子
func TestTransmissionClientCanAddTorrent(t *testing.T) {
	server := createMockTransmissionServer(false, "", "")
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
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

// TestTransmissionClientCheckTorrentExists 测试检查种子是否存在
func TestTransmissionClientCheckTorrentExists(t *testing.T) {
	server := createMockTransmissionServer(false, "", "")
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
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

// TestTransmissionClientClose 测试关闭客户端
func TestTransmissionClientClose(t *testing.T) {
	server := createMockTransmissionServer(false, "", "")
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
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

// TestTransmissionAuthenticationFailure 测试认证失败
func TestTransmissionAuthenticationFailure(t *testing.T) {
	server := createMockTransmissionServer(true, "admin", "secret")
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "wrong", "credentials")
	_, err := NewTransmissionClient(config, "test")

	if err == nil {
		t.Error("expected authentication to fail")
	}

	// 验证错误消息是描述性的
	errMsg := err.Error()
	if !strings.Contains(errMsg, "authentication") && !strings.Contains(errMsg, "invalid") {
		t.Errorf("expected descriptive error message, got: %s", errMsg)
	}
}

// createMockServerWithTorrents 创建带有指定种子的模拟服务器
func createMockServerWithTorrents(existingHashes []string) *httptest.Server {
	sessionID := "test-session-id"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 检查 session ID
		if r.Header.Get("X-Transmission-Session-Id") != sessionID {
			w.Header().Set("X-Transmission-Session-Id", sessionID)
			w.WriteHeader(http.StatusConflict)
			return
		}

		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var resp rpcResponse
		resp.Result = "success"

		switch req.Method {
		case "session-get":
			args := map[string]any{"download-dir": "/downloads"}
			resp.Arguments, _ = json.Marshal(args)
		case "torrent-get":
			torrents := make([]torrentStatus, 0)
			for i, hash := range existingHashes {
				torrents = append(torrents, torrentStatus{
					ID:         i + 1,
					Name:       "torrent-" + hash[:8],
					HashString: hash,
					Status:     4, // downloading
				})
			}
			args := torrentGetResponse{Torrents: torrents}
			resp.Arguments, _ = json.Marshal(args)
		}

		json.NewEncoder(w).Encode(resp)
	}))
}

// createMockServerWithFreeSpaceError 创建一个 free-space 请求会失败的模拟服务器
func createMockServerWithFreeSpaceError() *httptest.Server {
	sessionID := "test-session-id"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 检查 session ID
		if r.Header.Get("X-Transmission-Session-Id") != sessionID {
			w.Header().Set("X-Transmission-Session-Id", sessionID)
			w.WriteHeader(http.StatusConflict)
			return
		}

		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var resp rpcResponse

		switch req.Method {
		case "session-get":
			resp.Result = "success"
			args := map[string]any{"download-dir": "/nonexistent/path"}
			resp.Arguments, _ = json.Marshal(args)
		case "free-space":
			// 模拟 "No such file or directory" 错误
			resp.Result = "No such file or directory"
			resp.Arguments = nil
		default:
			resp.Result = "success"
		}

		json.NewEncoder(w).Encode(resp)
	}))
}

// TestTransmissionClientCanAddTorrentWithDiskSpaceError 测试磁盘空间检查失败时 fail-closed
func TestTransmissionClientCanAddTorrentWithDiskSpaceError(t *testing.T) {
	server := createMockServerWithFreeSpaceError()
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	canAdd, err := client.CanAddTorrent(ctx, 1024*1024*100) // 100 MB
	if err == nil {
		t.Fatal("expected error when disk space check fails")
	}
	if canAdd {
		t.Error("expected cannot add torrent when disk space check fails")
	}
}

// TestProperty9_TorrentHashExistenceCheckConsistency 属性测试：种子哈希存在检查一致性
// Feature: downloader-site-extensibility, Property 9: Torrent Hash Existence Check Consistency
// 对于任何种子哈希，CheckTorrentExists 应返回一致的结果
func TestProperty9_TorrentHashExistenceCheckConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	// 属性：对于存在的哈希，多次检查应返回一致的 true
	properties.Property("existing hash returns consistent true", prop.ForAll(
		func(hash string) bool {
			if len(hash) < 8 {
				hash = hash + "00000000" // 确保哈希足够长
			}
			server := createMockServerWithTorrents([]string{hash})
			defer server.Close()

			config := NewTransmissionConfig(server.URL, "", "")
			client, err := NewTransmissionClient(config, "test")
			if err != nil {
				return false
			}
			defer client.Close()

			// 多次检查应返回一致结果
			result1, err1 := client.CheckTorrentExists(hash)
			result2, err2 := client.CheckTorrentExists(hash)

			return err1 == nil && err2 == nil && result1 == result2 && result1 == true
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	// 属性：对于不存在的哈希，检查应返回 false
	properties.Property("non-existing hash returns false", prop.ForAll(
		func(existingHash, queryHash string) bool {
			if len(existingHash) < 8 {
				existingHash = existingHash + "00000000"
			}
			if len(queryHash) < 8 {
				queryHash = queryHash + "11111111"
			}
			// 确保查询的哈希与存在的不同
			if existingHash == queryHash {
				return true // 跳过相同的情况
			}

			server := createMockServerWithTorrents([]string{existingHash})
			defer server.Close()

			config := NewTransmissionConfig(server.URL, "", "")
			client, err := NewTransmissionClient(config, "test")
			if err != nil {
				return false
			}
			defer client.Close()

			exists, err := client.CheckTorrentExists(queryHash)
			return err == nil && !exists
		},
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }),
	))

	properties.TestingRun(t)
}

// TestTransmissionClientAddTorrent 测试添加种子
func TestTransmissionClientAddTorrent(t *testing.T) {
	server := createMockTransmissionServer(false, "", "")
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// 测试添加种子
	torrentData := []byte("d8:announce35:http://tracker.example.com/announce4:infod6:lengthi12345e4:name8:test.txt12:piece lengthi16384e6:pieces20:01234567890123456789ee")
	err = client.AddTorrent(torrentData, "movies", "free")
	if err != nil {
		t.Fatalf("failed to add torrent: %v", err)
	}
}

// TestTransmissionClientAddTorrentWithLabels 测试添加带标签的种子
func TestTransmissionClientAddTorrentWithLabels(t *testing.T) {
	server := createMockTransmissionServer(false, "", "")
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// 测试添加带标签的种子
	torrentData := []byte("d8:announce35:http://tracker.example.com/announce4:infod6:lengthi12345e4:name8:test.txt12:piece lengthi16384e6:pieces20:01234567890123456789ee")
	err = client.AddTorrent(torrentData, "", "tag1,tag2")
	if err != nil {
		t.Fatalf("failed to add torrent with tags: %v", err)
	}
}

// TestTransmissionClientInvalidConfig 测试无效配置
func TestTransmissionClientInvalidConfig(t *testing.T) {
	config := NewTransmissionConfig("", "", "")
	_, err := NewTransmissionClient(config, "test")
	if err == nil {
		t.Error("expected error for invalid config")
	}
}

// createMockServerWithDuplicateTorrent 创建返回重复种子的模拟服务器
func createMockServerWithDuplicateTorrent() *httptest.Server {
	sessionID := "test-session-id"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Transmission-Session-Id") != sessionID {
			w.Header().Set("X-Transmission-Session-Id", sessionID)
			w.WriteHeader(http.StatusConflict)
			return
		}

		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var resp rpcResponse
		resp.Result = "success"

		switch req.Method {
		case "session-get":
			args := map[string]any{"download-dir": "/downloads"}
			resp.Arguments, _ = json.Marshal(args)
		case "torrent-add":
			// 返回重复种子
			args := torrentAddResponse{
				TorrentDuplicate: &torrentInfo{
					ID:         1,
					Name:       "duplicate-torrent",
					HashString: "abc123",
				},
			}
			resp.Arguments, _ = json.Marshal(args)
		}

		json.NewEncoder(w).Encode(resp)
	}))
}

// TestTransmissionClientAddDuplicateTorrent 测试添加重复种子
func TestTransmissionClientAddDuplicateTorrent(t *testing.T) {
	server := createMockServerWithDuplicateTorrent()
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// 添加重复种子应该不返回错误
	torrentData := []byte("d8:announce35:http://tracker.example.com/announce4:infod6:lengthi12345e4:name8:test.txt12:piece lengthi16384e6:pieces20:01234567890123456789ee")
	err = client.AddTorrent(torrentData, "", "")
	if err != nil {
		t.Fatalf("expected no error for duplicate torrent, got: %v", err)
	}
}

// createMockServerWithZeroFreeSpace 创建返回零可用空间的模拟服务器
func createMockServerWithZeroFreeSpace() *httptest.Server {
	sessionID := "test-session-id"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Transmission-Session-Id") != sessionID {
			w.Header().Set("X-Transmission-Session-Id", sessionID)
			w.WriteHeader(http.StatusConflict)
			return
		}

		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var resp rpcResponse
		resp.Result = "success"

		switch req.Method {
		case "session-get":
			args := map[string]any{"download-dir": "/downloads"}
			resp.Arguments, _ = json.Marshal(args)
		case "free-space":
			args := freeSpaceResponse{
				Path:      "/downloads",
				SizeBytes: 0, // 零可用空间
			}
			resp.Arguments, _ = json.Marshal(args)
		}

		json.NewEncoder(w).Encode(resp)
	}))
}

// createMockServerWithEmptyDownloadDir 创建没有下载目录的模拟服务器
func createMockServerWithEmptyDownloadDir() *httptest.Server {
	sessionID := "test-session-id"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Transmission-Session-Id") != sessionID {
			w.Header().Set("X-Transmission-Session-Id", sessionID)
			w.WriteHeader(http.StatusConflict)
			return
		}

		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var resp rpcResponse
		resp.Result = "success"

		switch req.Method {
		case "session-get":
			// 返回空的下载目录
			args := map[string]any{"download-dir": ""}
			resp.Arguments, _ = json.Marshal(args)
		case "free-space":
			args := freeSpaceResponse{
				Path:      "/downloads",
				SizeBytes: 50 * 1024 * 1024 * 1024, // 50 GB
			}
			resp.Arguments, _ = json.Marshal(args)
		}

		json.NewEncoder(w).Encode(resp)
	}))
}

// TestTransmissionClientWithAuth 测试带认证的客户端
func TestTransmissionClientWithAuth(t *testing.T) {
	server := createMockTransmissionServer(true, "admin", "secret")
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "admin", "secret")
	client, err := NewTransmissionClient(config, "test")
	if err != nil {
		t.Fatalf("failed to create client with auth: %v", err)
	}
	defer client.Close()

	if !client.IsHealthy() {
		t.Error("expected client to be healthy")
	}
}

// TestTransmissionClientProcessSingleTorrentFile 测试处理单个种子文件
func TestTransmissionClientProcessSingleTorrentFile(t *testing.T) {
	server := createMockTransmissionServer(false, "", "")
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// 创建临时种子文件
	dir := t.TempDir()
	torrentData := []byte("d8:announce35:http://tracker.example.com/announce4:infod6:lengthi1024e4:name8:test.txt12:piece lengthi16384e6:pieces20:01234567890123456789ee")
	torrentPath := dir + "/test.torrent"
	if writeErr := writeTestFile(torrentPath, torrentData); writeErr != nil {
		t.Fatalf("failed to write torrent file: %v", writeErr)
	}

	ctx := context.Background()
	err = client.ProcessSingleTorrentFile(ctx, torrentPath, "test-cat", "test-tag")
	if err != nil {
		t.Errorf("ProcessSingleTorrentFile failed: %v", err)
	}
}

// TestTransmissionClientDoRequest 测试 doRequest 方法
func TestTransmissionClientDoRequest(t *testing.T) {
	server := createMockTransmissionServer(false, "", "")
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	transClient := client.(*TransmissionClient)
	resp, err := transClient.doRequest("session-get", nil)
	if err != nil {
		t.Fatalf("doRequest failed: %v", err)
	}
	if resp.Result != "success" {
		t.Errorf("expected result 'success', got '%s'", resp.Result)
	}
}

// TestTransmissionClientCreateRequest 测试 createRequest 方法
func TestTransmissionClientCreateRequest(t *testing.T) {
	server := createMockTransmissionServer(false, "", "")
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	transClient := client.(*TransmissionClient)
	req, err := transClient.createRequest("session-get", nil)
	if err != nil {
		t.Fatalf("createRequest failed: %v", err)
	}
	if req.Method != "POST" {
		t.Errorf("expected method POST, got %s", req.Method)
	}
}

// TestTransmissionClientCreateRequestWithContext 测试带 context 的 createRequest
func TestTransmissionClientCreateRequestWithContext(t *testing.T) {
	server := createMockTransmissionServer(false, "", "")
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	transClient := client.(*TransmissionClient)
	ctx := context.Background()
	req, err := transClient.createRequestWithContext(ctx, "session-get", nil)
	if err != nil {
		t.Fatalf("createRequestWithContext failed: %v", err)
	}
	if req.Context() != ctx {
		t.Error("expected request to have the provided context")
	}
}

// TestTransmissionClientVerifyConnection 测试 verifyConnection 方法
func TestTransmissionClientVerifyConnection(t *testing.T) {
	server := createMockTransmissionServer(false, "", "")
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	transClient := client.(*TransmissionClient)
	err = transClient.verifyConnection()
	if err != nil {
		t.Errorf("verifyConnection failed: %v", err)
	}
}

// writeTestFile 辅助函数写入测试文件
func writeTestFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

// createMockServerWithSessionIDExpiry 创建会话 ID 过期的模拟服务器
func createMockServerWithSessionIDExpiry() *httptest.Server {
	sessionID := "initial-session-id"
	newSessionID := "new-session-id"
	requestCount := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		currentSessionID := r.Header.Get("X-Transmission-Session-Id")

		// 第一次请求返回 409 获取初始 session ID
		if currentSessionID == "" {
			w.Header().Set("X-Transmission-Session-Id", sessionID)
			w.WriteHeader(http.StatusConflict)
			return
		}

		// 模拟 session ID 过期：第三次请求返回 409
		if requestCount == 3 && currentSessionID == sessionID {
			w.Header().Set("X-Transmission-Session-Id", newSessionID)
			w.WriteHeader(http.StatusConflict)
			return
		}

		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var resp rpcResponse
		resp.Result = "success"

		switch req.Method {
		case "session-get":
			args := map[string]any{"download-dir": "/downloads"}
			resp.Arguments, _ = json.Marshal(args)
		case "free-space":
			args := freeSpaceResponse{Path: "/downloads", SizeBytes: 100 * 1024 * 1024 * 1024}
			resp.Arguments, _ = json.Marshal(args)
		}

		json.NewEncoder(w).Encode(resp)
	}))
}

// TestTransmissionClientDoRequestWithSessionExpiry 测试会话过期重试
func TestTransmissionClientDoRequestWithSessionExpiry(t *testing.T) {
	server := createMockServerWithSessionIDExpiry()
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	transClient := client.(*TransmissionClient)

	// 多次调用应该能处理 session ID 过期
	for i := 0; i < 5; i++ {
		_, err := transClient.doRequest("session-get", nil)
		if err != nil {
			t.Errorf("doRequest failed on iteration %d: %v", i, err)
		}
	}
}

// createMockServerWithHTTPError 创建返回 HTTP 错误的模拟服务器
func createMockServerWithHTTPError(statusCode int) *httptest.Server {
	sessionID := "test-session-id"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Transmission-Session-Id") != sessionID {
			w.Header().Set("X-Transmission-Session-Id", sessionID)
			w.WriteHeader(http.StatusConflict)
			return
		}

		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// session-get 成功，其他请求返回错误
		if req.Method == "session-get" {
			resp := rpcResponse{Result: "success"}
			args := map[string]any{"download-dir": "/downloads"}
			resp.Arguments, _ = json.Marshal(args)
			json.NewEncoder(w).Encode(resp)
			return
		}

		w.WriteHeader(statusCode)
		w.Write([]byte("error response"))
	}))
}

// TestTransmissionClientDoRequestHTTPError 测试 HTTP 错误处理
func TestTransmissionClientDoRequestHTTPError(t *testing.T) {
	server := createMockServerWithHTTPError(http.StatusInternalServerError)
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	transClient := client.(*TransmissionClient)
	_, err = transClient.doRequest("torrent-add", nil)
	if err == nil {
		t.Error("expected error for HTTP 500")
	}
}

// createMockServerWithRPCError 创建返回 RPC 错误的模拟服务器
func createMockServerWithRPCError() *httptest.Server {
	sessionID := "test-session-id"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Transmission-Session-Id") != sessionID {
			w.Header().Set("X-Transmission-Session-Id", sessionID)
			w.WriteHeader(http.StatusConflict)
			return
		}

		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// session-get 成功
		if req.Method == "session-get" {
			resp := rpcResponse{Result: "success"}
			args := map[string]any{"download-dir": "/downloads"}
			resp.Arguments, _ = json.Marshal(args)
			json.NewEncoder(w).Encode(resp)
			return
		}

		// 其他请求返回 RPC 错误
		resp := rpcResponse{Result: "some error occurred"}
		json.NewEncoder(w).Encode(resp)
	}))
}

// TestTransmissionClientDoRequestRPCError 测试 RPC 错误处理
func TestTransmissionClientDoRequestRPCError(t *testing.T) {
	server := createMockServerWithRPCError()
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	transClient := client.(*TransmissionClient)
	_, err = transClient.doRequest("torrent-add", nil)
	if err == nil {
		t.Error("expected error for RPC error")
	}
	if !strings.Contains(err.Error(), "RPC error") {
		t.Errorf("expected RPC error message, got: %v", err)
	}
}

// TestTransmissionClientProcessTorrentFile 测试 processTorrentFile 方法
func TestTransmissionClientProcessTorrentFile(t *testing.T) {
	server := createMockTransmissionServer(false, "", "")
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	transClient := client.(*TransmissionClient)

	// 创建临时种子文件
	dir := t.TempDir()
	torrentData := []byte("d8:announce35:http://tracker.example.com/announce4:infod6:lengthi1024e4:name8:test.txt12:piece lengthi16384e6:pieces20:01234567890123456789ee")
	torrentPath := dir + "/test.torrent"
	if writeErr := writeTestFile(torrentPath, torrentData); writeErr != nil {
		t.Fatalf("failed to write torrent file: %v", writeErr)
	}

	ctx := context.Background()
	err = transClient.processTorrentFile(ctx, torrentPath, "test-cat", "test-tag")
	if err != nil {
		t.Errorf("processTorrentFile failed: %v", err)
	}
}

// TestTransmissionClientProcessTorrentFileNotFound 测试处理不存在的种子文件
func TestTransmissionClientProcessTorrentFileNotFound(t *testing.T) {
	server := createMockTransmissionServer(false, "", "")
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	transClient := client.(*TransmissionClient)

	ctx := context.Background()
	err = transClient.processTorrentFile(ctx, "/nonexistent/path/test.torrent", "test-cat", "test-tag")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

// createMockServerWithExistingTorrent 创建返回种子已存在的模拟服务器
func createMockServerWithExistingTorrent(existingHash string) *httptest.Server {
	sessionID := "test-session-id"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Transmission-Session-Id") != sessionID {
			w.Header().Set("X-Transmission-Session-Id", sessionID)
			w.WriteHeader(http.StatusConflict)
			return
		}

		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var resp rpcResponse
		resp.Result = "success"

		switch req.Method {
		case "session-get":
			args := map[string]any{"download-dir": "/downloads"}
			resp.Arguments, _ = json.Marshal(args)
		case "free-space":
			args := freeSpaceResponse{Path: "/downloads", SizeBytes: 100 * 1024 * 1024 * 1024}
			resp.Arguments, _ = json.Marshal(args)
		case "torrent-get":
			args := torrentGetResponse{
				Torrents: []torrentStatus{
					{ID: 1, Name: "existing", HashString: existingHash, Status: 4},
				},
			}
			resp.Arguments, _ = json.Marshal(args)
		}

		json.NewEncoder(w).Encode(resp)
	}))
}

// TestTransmissionClientProcessTorrentFileExisting 测试处理已存在的种子
func TestTransmissionClientProcessTorrentFileExisting(t *testing.T) {
	// 创建临时种子文件
	dir := t.TempDir()
	torrentData := []byte("d8:announce35:http://tracker.example.com/announce4:infod6:lengthi1024e4:name8:test.txt12:piece lengthi16384e6:pieces20:01234567890123456789ee")
	torrentPath := dir + "/test.torrent"
	if writeErr := writeTestFile(torrentPath, torrentData); writeErr != nil {
		t.Fatalf("failed to write torrent file: %v", writeErr)
	}

	// 使用 qbit 包计算实际的哈希
	actualHash, err := qbit.ComputeTorrentHash(torrentData)
	if err != nil {
		t.Fatalf("failed to compute hash: %v", err)
	}

	// 创建一个完整的 mock 服务器，处理所有需要的请求
	sessionID := "test-session-id"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Transmission-Session-Id") != sessionID {
			w.Header().Set("X-Transmission-Session-Id", sessionID)
			w.WriteHeader(http.StatusConflict)
			return
		}

		var req rpcRequest
		if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var resp rpcResponse
		resp.Result = "success"

		switch req.Method {
		case "session-get":
			args := map[string]any{"download-dir": "/downloads"}
			resp.Arguments, _ = json.Marshal(args)
		case "free-space":
			args := freeSpaceResponse{Path: "/downloads", SizeBytes: 100 * 1024 * 1024 * 1024}
			resp.Arguments, _ = json.Marshal(args)
		case "torrent-get":
			// 返回已存在的种子，使用实际计算的哈希
			args := torrentGetResponse{
				Torrents: []torrentStatus{
					{ID: 1, Name: "existing", HashString: actualHash, Status: 4},
				},
			}
			resp.Arguments, _ = json.Marshal(args)
		}

		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	transClient := client.(*TransmissionClient)

	ctx := context.Background()
	err = transClient.processTorrentFile(ctx, torrentPath, "test-cat", "test-tag")
	// 应该成功（种子已存在，删除本地文件）
	if err != nil {
		t.Errorf("processTorrentFile failed: %v", err)
	}
}

// TestTransmissionClientAuthenticateUnauthorized 测试认证失败（401）
func TestTransmissionClientAuthenticateUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionID := "test-session-id"
		if r.Header.Get("X-Transmission-Session-Id") != sessionID {
			w.Header().Set("X-Transmission-Session-Id", sessionID)
			w.WriteHeader(http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "wrong", "credentials")
	_, err := NewTransmissionClient(config, "test")
	if err == nil {
		t.Error("expected authentication error")
	}
}

// TestTransmissionClientAuthenticateOtherError 测试认证失败（其他状态码）
func TestTransmissionClientAuthenticateOtherError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionID := "test-session-id"
		if r.Header.Get("X-Transmission-Session-Id") != sessionID {
			w.Header().Set("X-Transmission-Session-Id", sessionID)
			w.WriteHeader(http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	_, err := NewTransmissionClient(config, "test")
	if err == nil {
		t.Error("expected authentication error")
	}
}

// TestTransmissionClientVerifyConnectionUnauthorized 测试验证连接失败（401）
func TestTransmissionClientVerifyConnectionUnauthorized(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		sessionID := "test-session-id"
		if r.Header.Get("X-Transmission-Session-Id") != sessionID {
			w.Header().Set("X-Transmission-Session-Id", sessionID)
			w.WriteHeader(http.StatusConflict)
			return
		}
		// 第二次请求返回 401
		if requestCount > 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	_, err := NewTransmissionClient(config, "test")
	if err == nil {
		t.Error("expected verification error")
	}
}

// TestTransmissionClientVerifyConnectionOtherError 测试验证连接失败（其他状态码）
func TestTransmissionClientVerifyConnectionOtherError(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		sessionID := "test-session-id"
		if r.Header.Get("X-Transmission-Session-Id") != sessionID {
			w.Header().Set("X-Transmission-Session-Id", sessionID)
			w.WriteHeader(http.StatusConflict)
			return
		}
		// 第二次请求返回 500
		if requestCount > 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	_, err := NewTransmissionClient(config, "test")
	if err == nil {
		t.Error("expected verification error")
	}
}

// TestProperty1_AutoStartPausedParameterConsistency_Transmission 属性测试：Auto-start Paused Parameter Consistency (Transmission)
// Property 1: 对于任何 auto_start 配置值，添加种子时发送到下载器 API 的 paused 参数应等于 !auto_start
// Validates: Requirements 1.3, 1.4, 1.5
func TestProperty1_AutoStartPausedParameterConsistency_Transmission(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("paused parameter equals !auto_start for Transmission", prop.ForAll(
		func(autoStart bool) bool {
			var capturedPaused bool
			sessionID := "test-session-id"

			// 创建捕获 paused 参数的模拟服务器
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Transmission-Session-Id") != sessionID {
					w.Header().Set("X-Transmission-Session-Id", sessionID)
					w.WriteHeader(http.StatusConflict)
					return
				}

				var req rpcRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}

				var resp rpcResponse
				resp.Result = "success"

				switch req.Method {
				case "session-get":
					args := map[string]any{"download-dir": "/downloads"}
					resp.Arguments, _ = json.Marshal(args)
				case "torrent-add":
					// 捕获 paused 参数
					if args, ok := req.Arguments.(map[string]any); ok {
						if paused, ok := args["paused"].(bool); ok {
							capturedPaused = paused
						}
					}
					addResp := torrentAddResponse{
						TorrentAdded: &torrentInfo{ID: 1, Name: "test", HashString: "abc123"},
					}
					resp.Arguments, _ = json.Marshal(addResp)
				}

				json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()

			// 使用指定的 autoStart 值创建配置
			config := NewTransmissionConfigWithAutoStart(server.URL, "", "", autoStart)
			client, err := NewTransmissionClient(config, "test-transmission")
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

			// 验证 paused 参数：autoStart=true 时 paused=false，autoStart=false 时 paused=true
			expectedPaused := !autoStart
			return capturedPaused == expectedPaused
		},
		gen.Bool(),
	))

	properties.TestingRun(t)
}

// TestTransmissionConfigAutoStart 测试 auto_start 配置
func TestTransmissionConfigAutoStart(t *testing.T) {
	tests := []struct {
		name      string
		autoStart bool
	}{
		{"auto_start true", true},
		{"auto_start false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewTransmissionConfigWithAutoStart("http://localhost:9091", "", "", tt.autoStart)
			if config.GetAutoStart() != tt.autoStart {
				t.Errorf("expected auto_start %v, got %v", tt.autoStart, config.GetAutoStart())
			}
		})
	}
}

// TestTransmissionConfigDefaultAutoStart 测试默认 auto_start 值
func TestTransmissionConfigDefaultAutoStart(t *testing.T) {
	config := NewTransmissionConfig("http://localhost:9091", "", "")
	if config.GetAutoStart() != false {
		t.Errorf("expected default auto_start to be false, got %v", config.GetAutoStart())
	}
}

// TestTransmissionClientAddTorrentWithPath 测试添加种子并指定下载路径
func TestTransmissionClientAddTorrentWithPath(t *testing.T) {
	var capturedDownloadDir string
	sessionID := "test-session-id"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Transmission-Session-Id") != sessionID {
			w.Header().Set("X-Transmission-Session-Id", sessionID)
			w.WriteHeader(http.StatusConflict)
			return
		}

		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var resp rpcResponse
		resp.Result = "success"

		switch req.Method {
		case "session-get":
			args := map[string]any{"download-dir": "/downloads"}
			resp.Arguments, _ = json.Marshal(args)
		case "torrent-add":
			// 捕获 download-dir 参数
			if args, ok := req.Arguments.(map[string]any); ok {
				if downloadDir, ok := args["download-dir"].(string); ok {
					capturedDownloadDir = downloadDir
				}
			}
			addResp := torrentAddResponse{
				TorrentAdded: &torrentInfo{ID: 1, Name: "test", HashString: "abc123"},
			}
			resp.Arguments, _ = json.Marshal(addResp)
		}

		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	torrentData := []byte("test-torrent-data")

	// 测试带下载路径
	err = client.AddTorrentWithPath(torrentData, "test-category", "test-tag", "/custom/download/path")
	if err != nil {
		t.Errorf("failed to add torrent with path: %v", err)
	}
	if capturedDownloadDir != "/custom/download/path" {
		t.Errorf("expected download-dir '/custom/download/path', got '%s'", capturedDownloadDir)
	}
}

// TestTransmissionClientAddTorrentWithEmptyPath 测试添加种子不指定下载路径
func TestTransmissionClientAddTorrentWithEmptyPath(t *testing.T) {
	var capturedDownloadDir string
	var downloadDirSet bool
	sessionID := "test-session-id"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Transmission-Session-Id") != sessionID {
			w.Header().Set("X-Transmission-Session-Id", sessionID)
			w.WriteHeader(http.StatusConflict)
			return
		}

		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var resp rpcResponse
		resp.Result = "success"

		switch req.Method {
		case "session-get":
			args := map[string]any{"download-dir": "/downloads"}
			resp.Arguments, _ = json.Marshal(args)
		case "torrent-add":
			// 检查 download-dir 参数是否设置
			if args, ok := req.Arguments.(map[string]any); ok {
				if downloadDir, ok := args["download-dir"].(string); ok {
					capturedDownloadDir = downloadDir
					downloadDirSet = downloadDir != ""
				}
			}
			addResp := torrentAddResponse{
				TorrentAdded: &torrentInfo{ID: 1, Name: "test", HashString: "abc123"},
			}
			resp.Arguments, _ = json.Marshal(addResp)
		}

		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	torrentData := []byte("test-torrent-data")

	// 测试不带下载路径
	err = client.AddTorrentWithPath(torrentData, "test-category", "test-tag", "")
	if err != nil {
		t.Errorf("failed to add torrent without path: %v", err)
	}
	if downloadDirSet {
		t.Errorf("expected download-dir not to be set, but got '%s'", capturedDownloadDir)
	}
}

// TestProperty_DownloadPathParameter_Transmission 属性测试：Download Path Parameter (Transmission)
// Property: 当 downloadPath 非空时，download-dir 参数应该被设置为该值；当为空时，不应设置 download-dir
// Validates: Requirements 2.2, 2.8
func TestProperty_DownloadPathParameter_Transmission(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	properties := gopter.NewProperties(parameters)

	properties.Property("download-dir parameter is set correctly for Transmission", prop.ForAll(
		func(downloadPath string) bool {
			var capturedDownloadDir string
			var downloadDirFieldExists bool
			sessionID := "test-session-id"

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Transmission-Session-Id") != sessionID {
					w.Header().Set("X-Transmission-Session-Id", sessionID)
					w.WriteHeader(http.StatusConflict)
					return
				}

				var req rpcRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}

				var resp rpcResponse
				resp.Result = "success"

				switch req.Method {
				case "session-get":
					args := map[string]any{"download-dir": "/downloads"}
					resp.Arguments, _ = json.Marshal(args)
				case "torrent-add":
					if args, ok := req.Arguments.(map[string]any); ok {
						if downloadDir, ok := args["download-dir"].(string); ok {
							capturedDownloadDir = downloadDir
							downloadDirFieldExists = downloadDir != ""
						}
					}
					addResp := torrentAddResponse{
						TorrentAdded: &torrentInfo{ID: 1, Name: "test", HashString: "abc123"},
					}
					resp.Arguments, _ = json.Marshal(addResp)
				}

				json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()

			config := NewTransmissionConfig(server.URL, "", "")
			client, err := NewTransmissionClient(config, "test")
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

			// 验证：非空路径应该设置 download-dir，空路径不应设置
			if downloadPath != "" {
				return downloadDirFieldExists && capturedDownloadDir == downloadPath
			}
			return !downloadDirFieldExists
		},
		gen.OneConstOf("", "/downloads/movies", "/data/tv", "/mnt/storage/torrents"),
	))

	properties.TestingRun(t)
}

// TestAddTorrentDelegatesToAddTorrentWithPath_Transmission 测试 AddTorrent 委托给 AddTorrentWithPath
func TestAddTorrentDelegatesToAddTorrentWithPath_Transmission(t *testing.T) {
	var capturedDownloadDir string
	sessionID := "test-session-id"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Transmission-Session-Id") != sessionID {
			w.Header().Set("X-Transmission-Session-Id", sessionID)
			w.WriteHeader(http.StatusConflict)
			return
		}

		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var resp rpcResponse
		resp.Result = "success"

		switch req.Method {
		case "session-get":
			args := map[string]any{"download-dir": "/downloads"}
			resp.Arguments, _ = json.Marshal(args)
		case "torrent-add":
			if args, ok := req.Arguments.(map[string]any); ok {
				if downloadDir, ok := args["download-dir"].(string); ok {
					capturedDownloadDir = downloadDir
				}
			}
			addResp := torrentAddResponse{
				TorrentAdded: &torrentInfo{ID: 1, Name: "test", HashString: "abc123"},
			}
			resp.Arguments, _ = json.Marshal(addResp)
		}

		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
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

	// 验证 download-dir 未设置
	if capturedDownloadDir != "" {
		t.Errorf("expected download-dir to be empty when using AddTorrent, got '%s'", capturedDownloadDir)
	}
}

func TestTransmissionConfigURLTrailingSlash(t *testing.T) {
	tests := []struct {
		inputURL    string
		expectedURL string
	}{
		{"http://localhost:9091/", "http://localhost:9091"},
		{"http://localhost:9091", "http://localhost:9091"},
		{"http://192.168.1.100:9091/", "http://192.168.1.100:9091"},
		{"https://transmission.example.com/", "https://transmission.example.com"},
	}

	for _, tt := range tests {
		config := NewTransmissionConfig(tt.inputURL, "admin", "password")
		if config.GetURL() != tt.expectedURL {
			t.Errorf("GetURL(%q) = %q, want %q", tt.inputURL, config.GetURL(), tt.expectedURL)
		}
	}
}

// failRPCServer responds to every RPC method with {"result":"failure"} so that
// doRequest returns an error, exercising each caller's error-return branch.
func failRPCServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(rpcResponse{Result: "failure"})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// covClient builds a TransmissionClient pointed at the given test server with a
// preset sessionID so doRequest skips the 409 handshake — keeping per-method
// tests fast and hermetic (no real network, no retry sleeps).
func covClient(baseURL string) *TransmissionClient {
	return &TransmissionClient{
		name:      "test-tr",
		baseURL:   baseURL,
		client:    downloader.NewRequestsHTTPDoer(baseURL, 5_000_000_000),
		sessionID: "sess-1",
		healthy:   true,
	}
}

// rpcServer serves a handler keyed by RPC method, wrapping results in the
// standard {"result":"success","arguments":...} envelope.
func rpcServer(t *testing.T, handlers map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		args, ok := handlers[req.Method]
		if !ok {
			// default empty success
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "success"})
			return
		}
		raw, _ := json.Marshal(args)
		_ = json.NewEncoder(w).Encode(rpcResponse{Result: "success", Arguments: raw})
	}))
}

type errString string

func (e errString) Error() string { return string(e) }

func makeTorrentBytes(t *testing.T) []byte {
	t.Helper()
	return []byte("d8:announce35:http://tracker.example.com/announce4:infod6:lengthi1024e4:name8:test.txt12:piece lengthi16384e6:pieces20:01234567890123456789ee")
}

func TestTrAuthenticate_409Handshake(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("X-Transmission-Session-Id", "sess-xyz")
			w.WriteHeader(http.StatusConflict)
			return
		}
		_ = json.NewEncoder(w).Encode(rpcResponse{Result: "success"})
	}))
	defer srv.Close()

	c := covClient(srv.URL)
	c.sessionID = ""
	require.NoError(t, c.Authenticate())
	assert.True(t, c.healthy)
	assert.Equal(t, "sess-xyz", c.sessionID)
}

func TestTrAuthenticate_Success200(t *testing.T) {
	srv := rpcServer(t, map[string]any{"session-get": map[string]any{"version": "4.0"}})
	defer srv.Close()

	c := covClient(srv.URL)
	require.NoError(t, c.Authenticate())
	assert.True(t, c.healthy)
}

func TestTrAddTorrentWithPath_NewAndDuplicate(t *testing.T) {
	t.Run("added path set", func(t *testing.T) {
		var sawDir string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req rpcRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			resp := rpcResponse{Result: "success"}
			switch req.Method {
			case "torrent-add":
				if m, ok := req.Arguments.(map[string]any); ok {
					if dd, ok2 := m["download-dir"].(string); ok2 {
						sawDir = dd
					}
				}
				resp.Arguments, _ = json.Marshal(torrentAddResponse{TorrentAdded: &torrentInfo{ID: 1, Name: "n", HashString: "hh"}})
			case "torrent-get":
				resp.Arguments, _ = json.Marshal(map[string]any{"torrents": []map[string]any{{"id": 1, "hashString": "hh", "status": 4}}})
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer srv.Close()

		c := covClient(srv.URL)
		c.autoStart = false
		require.NoError(t, c.AddTorrentWithPath([]byte("data"), "cat", "tag", "/custom"))
		assert.Equal(t, "/custom", sawDir)
	})

	t.Run("duplicate", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-add": torrentAddResponse{
			TorrentDuplicate: &torrentInfo{ID: 2, Name: "dup", HashString: "dd"},
		}, "torrent-get": map[string]any{"torrents": []map[string]any{{"id": 2, "hashString": "dd", "status": 4}}}})
		defer srv.Close()

		c := covClient(srv.URL)
		c.autoStart = false
		require.NoError(t, c.AddTorrentWithPath([]byte("data"), "", "", ""))
	})

	t.Run("rpc error", func(t *testing.T) {
		srv := failRPCServer(t)
		require.Error(t, covClient(srv.URL).AddTorrentWithPath([]byte("data"), "", "", ""))
	})
}

func TestTrProcessTorrentFile_ExistingRemovesLocal(t *testing.T) {
	data := makeTorrentBytes(t)
	hash, err := qbit.ComputeTorrentHash(data)
	require.NoError(t, err)

	srv := rpcServer(t, map[string]any{
		"session-get": map[string]any{"download-dir": "/dl"},
		"free-space":  freeSpaceResponse{Path: "/dl", SizeBytes: 1 << 40},
		"torrent-get": map[string]any{"torrents": []map[string]any{{"id": 1, "hashString": hash}}},
	})
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "x.torrent")
	require.NoError(t, os.WriteFile(path, data, 0o644))

	c := covClient(srv.URL)
	require.NoError(t, c.processTorrentFile(context.Background(), path, "", ""))
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "existing torrent's local file must be removed")
}

func TestTrGetAllTorrentsAndMapping(t *testing.T) {
	srv := rpcServer(t, map[string]any{"torrent-get": map[string]any{
		"torrents": []map[string]any{
			{
				"id": 1, "name": "t1", "hashString": "h1", "status": 6,
				"percentDone": 1.0, "rateDownload": 10, "rateUpload": 20,
				"totalSize": 1024, "leftUntilDone": 0, "uploadedEver": 2048,
				"downloadedEver": 4096, "uploadRatio": 2.0, "addedDate": 1600000000,
				"downloadDir": "/dl", "labels": []string{"movies", "hd"},
				"eta": -1, "secondsSeeding": 3600, "doneDate": 1600001000,
				"peersSendingToUs": 3, "peersGettingFromUs": 5, "peersConnected": 8,
				"desiredAvailable": 1.5,
				"trackers":         []map[string]any{{"announce": "http://tr1"}},
			},
			{"id": 2, "name": "t2", "hashString": "h2", "status": 4, "percentDone": 0.5, "peersConnected": 2},
		},
	}})
	defer srv.Close()

	torrents, err := covClient(srv.URL).GetAllTorrents()
	require.NoError(t, err)
	require.Len(t, torrents, 2)

	first := torrents[0]
	assert.Equal(t, "1", first.ID)
	assert.Equal(t, "h1", first.InfoHash)
	assert.Equal(t, "t1", first.Name)
	assert.True(t, first.IsCompleted)
	assert.Equal(t, downloader.TorrentSeeding, first.State)
	assert.Equal(t, int64(1024), first.TotalSize)
	assert.Equal(t, "movies,hd", first.Tags)
	assert.Equal(t, "movies", first.Category)
	assert.Equal(t, "http://tr1", first.Tracker)
	assert.Equal(t, 5, first.NumSeeds)
	assert.Equal(t, 3, first.NumPeers)
	assert.Equal(t, filepath.Join("/dl", "t1"), first.ContentPath)
	assert.Equal(t, "test-tr", first.ClientID)

	// second torrent: NumPeers falls back to peersConnected
	assert.Equal(t, 2, torrents[1].NumPeers)
	assert.Equal(t, downloader.TorrentDownloading, torrents[1].State)
}

func TestTrMapTransmissionState(t *testing.T) {
	c := covClient("http://x")
	cases := map[int]downloader.TorrentState{
		0:  downloader.TorrentStopped,
		1:  downloader.TorrentChecking,
		2:  downloader.TorrentChecking,
		3:  downloader.TorrentQueued,
		4:  downloader.TorrentDownloading,
		5:  downloader.TorrentQueued,
		6:  downloader.TorrentSeeding,
		99: downloader.TorrentUnknown,
	}
	for status, want := range cases {
		assert.Equal(t, want, c.mapTransmissionState(status), "status=%d", status)
	}
}

func TestTrGetTorrentsByAndGetTorrent(t *testing.T) {
	body := map[string]any{"torrents": []map[string]any{
		{"id": 1, "name": "a", "hashString": "h1", "status": 6, "percentDone": 1.0},
		{"id": 2, "name": "b", "hashString": "h2", "status": 4, "percentDone": 0.3},
	}}
	srv := rpcServer(t, map[string]any{"torrent-get": body})
	defer srv.Close()
	c := covClient(srv.URL)

	all, err := c.GetTorrentsBy(downloader.TorrentFilter{})
	require.NoError(t, err)
	assert.Len(t, all, 2)

	byHash, err := c.GetTorrentsBy(downloader.TorrentFilter{Hashes: []string{"h2"}})
	require.NoError(t, err)
	require.Len(t, byHash, 1)
	assert.Equal(t, "h2", byHash[0].InfoHash)

	byID, err := c.GetTorrentsBy(downloader.TorrentFilter{IDs: []string{"1"}})
	require.NoError(t, err)
	require.Len(t, byID, 1)

	done := true
	byComplete, err := c.GetTorrentsBy(downloader.TorrentFilter{Complete: &done})
	require.NoError(t, err)
	require.Len(t, byComplete, 1)

	state := downloader.TorrentDownloading
	byState, err := c.GetTorrentsBy(downloader.TorrentFilter{State: &state})
	require.NoError(t, err)
	require.Len(t, byState, 1)

	tor, err := c.GetTorrent("h1")
	require.NoError(t, err)
	assert.Equal(t, "h1", tor.InfoHash)

	_, err = c.GetTorrent("missing")
	require.ErrorIs(t, err, downloader.ErrTorrentNotFound)
}

func TestTrCheckTorrentExists(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-get": map[string]any{
			"torrents": []map[string]any{{"id": 1, "name": "a", "hashString": "abc"}},
		}})
		defer srv.Close()

		got, err := covClient(srv.URL).CheckTorrentExists("abc")
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("not found", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-get": map[string]any{"torrents": []map[string]any{}}})
		defer srv.Close()

		got, err := covClient(srv.URL).CheckTorrentExists("abc")
		require.NoError(t, err)
		assert.False(t, got)
	})
}

func TestTrAddTorrentEx(t *testing.T) {
	t.Run("added", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-add": torrentAddResponse{
			TorrentAdded: &torrentInfo{ID: 7, Name: "n", HashString: "hh"},
		}})
		defer srv.Close()

		res, err := covClient(srv.URL).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{
			SavePath: "/dl", Category: "cat", Tags: "tag",
		})
		require.NoError(t, err)
		assert.True(t, res.Success)
		assert.Equal(t, "7", res.ID)
		assert.Equal(t, "hh", res.Hash)
	})

	t.Run("duplicate", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-add": torrentAddResponse{
			TorrentDuplicate: &torrentInfo{ID: 3, Name: "dup", HashString: "dd"},
		}})
		defer srv.Close()

		res, err := covClient(srv.URL).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{})
		require.NoError(t, err)
		assert.True(t, res.Success)
		assert.Equal(t, "dd", res.Hash)
	})

	t.Run("rpc error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "duplicate torrent"})
		}))
		defer srv.Close()

		res, err := covClient(srv.URL).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{})
		require.Error(t, err)
		assert.False(t, res.Success)
	})
}

func TestTrAddTorrentFileEx(t *testing.T) {
	t.Run("added no limits", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-add": torrentAddResponse{
			TorrentAdded: &torrentInfo{ID: 9, Name: "n", HashString: "hh"},
		}})
		defer srv.Close()

		res, err := covClient(srv.URL).AddTorrentFileEx([]byte("data"), downloader.AddTorrentOptions{
			SavePath: "/dl", Category: "cat", Tags: "tag",
		})
		require.NoError(t, err)
		assert.True(t, res.Success)
		assert.Equal(t, "hh", res.Hash)
	})

	t.Run("with speed limits triggers torrent-set and start", func(t *testing.T) {
		var sawSet, sawStart bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req rpcRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			resp := rpcResponse{Result: "success"}
			switch req.Method {
			case "torrent-add":
				raw, _ := json.Marshal(torrentAddResponse{TorrentAdded: &torrentInfo{ID: 11, HashString: "hh"}})
				resp.Arguments = raw
			case "torrent-set":
				sawSet = true
			case "torrent-start":
				sawStart = true
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer srv.Close()

		res, err := covClient(srv.URL).AddTorrentFileEx([]byte("data"), downloader.AddTorrentOptions{
			UploadSpeedLimitKBs:   500,
			DownloadSpeedLimitKBs: 1000,
			AddAtPaused:           false,
		})
		require.NoError(t, err)
		assert.True(t, res.Success)
		assert.True(t, sawSet, "torrent-set should be called for speed limits")
		assert.True(t, sawStart, "torrent-start should be called when not paused")
	})

	t.Run("duplicate", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-add": torrentAddResponse{
			TorrentDuplicate: &torrentInfo{ID: 5, HashString: "dd"},
		}})
		defer srv.Close()

		res, err := covClient(srv.URL).AddTorrentFileEx([]byte("data"), downloader.AddTorrentOptions{})
		require.NoError(t, err)
		assert.True(t, res.Success)
		assert.Equal(t, "dd", res.Hash)
	})

	t.Run("rpc error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "invalid"})
		}))
		defer srv.Close()

		res, err := covClient(srv.URL).AddTorrentFileEx([]byte("data"), downloader.AddTorrentOptions{})
		require.Error(t, err)
		assert.False(t, res.Success)
	})
}

func TestTrPauseResumeRemove(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		methods = append(methods, req.Method)
		_ = json.NewEncoder(w).Encode(rpcResponse{Result: "success"})
	}))
	defer srv.Close()
	c := covClient(srv.URL)

	require.NoError(t, c.PauseTorrent("1"))
	require.NoError(t, c.ResumeTorrent("2"))
	require.NoError(t, c.RemoveTorrent("3", true))
	require.NoError(t, c.PauseTorrents([]string{"1", "abc"}))
	require.NoError(t, c.ResumeTorrents([]string{"2"}))
	require.NoError(t, c.RemoveTorrents([]string{"3"}, false))

	assert.Contains(t, methods, "torrent-stop")
	assert.Contains(t, methods, "torrent-start")
	assert.Contains(t, methods, "torrent-remove")
}

func TestTrPauseResume_EmptyIDs(t *testing.T) {
	c := covClient("http://unused")
	assert.NoError(t, c.PauseTorrents(nil))
	assert.NoError(t, c.ResumeTorrents(nil))
	assert.NoError(t, c.RemoveTorrents(nil, true))
	assert.NoError(t, c.SetTorrentCategory(" ", "x"))
	assert.NoError(t, c.SetTorrentTags(" ", "x"))
	assert.NoError(t, c.SetTorrentSavePath(" ", "x"))
	assert.NoError(t, c.RecheckTorrent(" "))
}

func TestTrGetTorrentFiles(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-get": map[string]any{
			"torrents": []map[string]any{{
				"files": []map[string]any{
					{"name": "a.mkv", "length": 1000, "bytesCompleted": 500},
					{"name": "b.mkv", "length": 2000, "bytesCompleted": 2000},
				},
				"fileStats": []map[string]any{
					{"wanted": true, "priority": 1},
					{"wanted": false, "priority": 0},
				},
			}},
		}})
		defer srv.Close()

		files, err := covClient(srv.URL).GetTorrentFiles("1")
		require.NoError(t, err)
		require.Len(t, files, 2)
		assert.Equal(t, "a.mkv", files[0].Name)
		assert.Equal(t, int64(1000), files[0].Size)
		assert.Equal(t, 0.5, files[0].Progress)
		assert.Equal(t, 6, files[0].Priority) // wanted + priority>0
		assert.Equal(t, 0, files[1].Priority) // not wanted
	})

	t.Run("no torrents", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-get": map[string]any{"torrents": []map[string]any{}}})
		defer srv.Close()

		_, err := covClient(srv.URL).GetTorrentFiles("1")
		require.ErrorIs(t, err, downloader.ErrTorrentNotFound)
	})
}

func TestTrGetTorrentTrackers(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-get": map[string]any{
			"torrents": []map[string]any{{
				"trackerStats": []map[string]any{
					{
						"announce": "http://tr1", "leecherCount": 3, "seederCount": 5,
						"downloadCount": 10, "announceState": 3, "lastAnnounceSucceeded": true,
					},
					{
						"host": "http://tr2", "lastAnnounceResult": "failed", "lastAnnounceSucceeded": false,
					},
				},
			}},
		}})
		defer srv.Close()

		trackers, err := covClient(srv.URL).GetTorrentTrackers("1")
		require.NoError(t, err)
		require.Len(t, trackers, 2)
		assert.Equal(t, "http://tr1", trackers[0].URL)
		assert.Equal(t, 5, trackers[0].Seeds)
		assert.Equal(t, 3, trackers[0].Leeches)
		assert.Equal(t, "http://tr2", trackers[1].URL) // falls back to host
		assert.Equal(t, 4, trackers[1].Status)         // failed announce
	})

	t.Run("no torrents", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-get": map[string]any{"torrents": []map[string]any{}}})
		defer srv.Close()

		_, err := covClient(srv.URL).GetTorrentTrackers("1")
		require.ErrorIs(t, err, downloader.ErrTorrentNotFound)
	})
}

func TestTrNormalizeAndSplitLabels(t *testing.T) {
	ids := normalizeTransmissionIDs([]string{"1", " 2 ", "", "abc"})
	require.Len(t, ids, 3)
	assert.Equal(t, 1, ids[0])
	assert.Equal(t, 2, ids[1])
	assert.Equal(t, "abc", ids[2])

	assert.Nil(t, splitLabels(""))
	assert.Equal(t, []string{"a", "b"}, splitLabels("a, b ,"))
}

func TestTrMapTransmissionTrackerStatus(t *testing.T) {
	assert.Equal(t, 4, mapTransmissionTrackerStatus(transmissionTrackerStat{
		LastAnnounceSucceeded: false, LastAnnounceResult: "err",
	}))
	assert.Equal(t, 2, mapTransmissionTrackerStatus(transmissionTrackerStat{
		LastAnnounceSucceeded: true, AnnounceState: 3,
	}))
	assert.Equal(t, 3, mapTransmissionTrackerStatus(transmissionTrackerStat{
		LastAnnounceSucceeded: true, AnnounceState: 1,
	}))
	assert.Equal(t, 1, mapTransmissionTrackerStatus(transmissionTrackerStat{
		LastAnnounceSucceeded: true, AnnounceState: 0,
	}))
}

func TestTrMapTransmissionPriority(t *testing.T) {
	assert.Equal(t, 0, mapTransmissionPriority(false, 5))
	assert.Equal(t, 6, mapTransmissionPriority(true, 1))
	assert.Equal(t, 1, mapTransmissionPriority(true, 0))
	assert.Equal(t, 1, mapTransmissionPriority(true, -1))
}
