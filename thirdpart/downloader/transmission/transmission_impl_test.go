package transmission

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"

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

// TestTransmissionClientGetDiskSpace 测试获取磁盘空间
func TestTransmissionClientGetDiskSpace(t *testing.T) {
	server := createMockTransmissionServer(false, "", "")
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	space, err := client.GetDiskSpace(ctx)
	if err != nil {
		t.Fatalf("failed to get disk space: %v", err)
	}

	expectedSpace := int64(1024 * 1024 * 1024 * 100) // 100 GB
	if space != expectedSpace {
		t.Errorf("expected space %d, got %d", expectedSpace, space)
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

// TestTransmissionClientGetDiskSpaceWithError 测试当 free-space 失败时的处理
func TestTransmissionClientGetDiskSpaceWithError(t *testing.T) {
	server := createMockServerWithFreeSpaceError()
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	space, err := client.GetDiskSpace(ctx)
	// 应该不返回错误，而是返回默认的 100GB
	if err != nil {
		t.Fatalf("expected no error when free-space fails, got: %v", err)
	}

	expectedSpace := int64(100 * 1024 * 1024 * 1024) // 100 GB default
	if space != expectedSpace {
		t.Errorf("expected default space %d (100GB), got %d", expectedSpace, space)
	}
}

// TestTransmissionClientCanAddTorrentWithDiskSpaceError 测试磁盘空间检查失败时仍能添加种子
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

	// 即使 free-space 失败，也应该允许添加合理大小的种子
	canAdd, err := client.CanAddTorrent(ctx, 1024*1024*100) // 100 MB
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !canAdd {
		t.Error("expected to be able to add torrent when disk space check fails with default value")
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

// TestTransmissionClientGetDiskSpaceZero 测试零可用空间处理
func TestTransmissionClientGetDiskSpaceZero(t *testing.T) {
	server := createMockServerWithZeroFreeSpace()
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	space, err := client.GetDiskSpace(ctx)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// 当返回零空间时，应该返回默认的 100GB
	expectedSpace := int64(100 * 1024 * 1024 * 1024)
	if space != expectedSpace {
		t.Errorf("expected default space %d (100GB), got %d", expectedSpace, space)
	}
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

// TestTransmissionClientGetDiskSpaceEmptyDownloadDir 测试空下载目录处理
func TestTransmissionClientGetDiskSpaceEmptyDownloadDir(t *testing.T) {
	server := createMockServerWithEmptyDownloadDir()
	defer server.Close()

	config := NewTransmissionConfig(server.URL, "", "")
	client, err := NewTransmissionClient(config, "test")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	space, err := client.GetDiskSpace(ctx)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// 应该使用默认路径 /downloads 并返回 50GB
	expectedSpace := int64(50 * 1024 * 1024 * 1024)
	if space != expectedSpace {
		t.Errorf("expected space %d (50GB), got %d", expectedSpace, space)
	}
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
