package transmission

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

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
	if err == nil {
		t.Fatal("expected error when free-space fails")
	}
	if space != 0 {
		t.Errorf("expected zero space when free-space fails, got %d", space)
	}
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
	if err == nil {
		t.Fatal("expected error when free-space returns zero")
	}
	if space != 0 {
		t.Errorf("expected zero space when free-space returns zero, got %d", space)
	}
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

func TestTrPing(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"session-get": map[string]any{"download-dir": "/dl"}})
		defer srv.Close()

		ok, err := covClient(srv.URL).Ping()
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("rpc error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "failure"})
		}))
		defer srv.Close()

		ok, err := covClient(srv.URL).Ping()
		require.Error(t, err)
		assert.False(t, ok)
	})
}

func TestTrGetClientVersion(t *testing.T) {
	srv := rpcServer(t, map[string]any{"session-get": map[string]any{"version": "4.0.5"}})
	defer srv.Close()

	v, err := covClient(srv.URL).GetClientVersion()
	require.NoError(t, err)
	assert.Equal(t, "4.0.5", v)
}

func TestTrGetClientStatus(t *testing.T) {
	srv := rpcServer(t, map[string]any{"session-stats": map[string]any{
		"uploadSpeed":      100,
		"downloadSpeed":    200,
		"cumulative-stats": map[string]any{"uploadedBytes": 5000, "downloadedBytes": 9000},
		"current-stats":    map[string]any{"uploadedBytes": 50, "downloadedBytes": 90},
	}})
	defer srv.Close()

	st, err := covClient(srv.URL).GetClientStatus()
	require.NoError(t, err)
	assert.Equal(t, int64(100), st.UpSpeed)
	assert.Equal(t, int64(200), st.DlSpeed)
	assert.Equal(t, int64(5000), st.UpData)
	assert.Equal(t, int64(9000), st.DlData)
	assert.Equal(t, int64(50), st.SessionUpData)
	assert.Equal(t, int64(90), st.SessionDlData)
}

func TestTrGetClientFreeSpace(t *testing.T) {
	srv := rpcServer(t, map[string]any{
		"session-get": map[string]any{"download-dir": "/downloads"},
		"free-space":  freeSpaceResponse{Path: "/downloads", SizeBytes: 2048},
	})
	defer srv.Close()

	got, err := covClient(srv.URL).GetClientFreeSpace(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(2048), got)
}

func TestTrGetDiskSpace_DefaultDirAndZeroSpace(t *testing.T) {
	t.Run("empty download dir uses default", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{
			"session-get": map[string]any{"download-dir": ""},
			"free-space":  freeSpaceResponse{Path: "/downloads", SizeBytes: 4096},
		})
		defer srv.Close()

		got, err := covClient(srv.URL).GetDiskSpace(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(4096), got)
	})

	t.Run("zero free space is an error", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{
			"session-get": map[string]any{"download-dir": "/dl"},
			"free-space":  freeSpaceResponse{Path: "/dl", SizeBytes: 0},
		})
		defer srv.Close()

		_, err := covClient(srv.URL).GetDiskSpace(context.Background())
		require.Error(t, err)
	})
}

func TestTrGetIncompletePendingBytes(t *testing.T) {
	srv := rpcServer(t, map[string]any{"torrent-get": map[string]any{
		"torrents": []map[string]any{
			{"status": 4, "leftUntilDone": 1000},
			{"status": 0, "leftUntilDone": 500},
			{"status": 6, "leftUntilDone": 999}, // seeding, not counted
			{"status": 4, "leftUntilDone": 0},   // zero skipped
		},
	}})
	defer srv.Close()

	got, err := covClient(srv.URL).GetIncompletePendingBytes(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1500), got)
}

func TestTrGetDiskInfo(t *testing.T) {
	srv := rpcServer(t, map[string]any{
		"session-get": map[string]any{"download-dir": "/downloads"},
		"free-space":  freeSpaceResponse{Path: "/downloads", SizeBytes: 8192},
	})
	defer srv.Close()

	info, err := covClient(srv.URL).GetDiskInfo()
	require.NoError(t, err)
	assert.Equal(t, "/downloads", info.Path)
	assert.Equal(t, int64(8192), info.FreeSpace)
}

func TestTrGetDiskInfo_DefaultPath(t *testing.T) {
	srv := rpcServer(t, map[string]any{
		"session-get": map[string]any{"download-dir": ""},
		"free-space":  freeSpaceResponse{Path: "/downloads", SizeBytes: 1},
	})
	defer srv.Close()

	info, err := covClient(srv.URL).GetDiskInfo()
	require.NoError(t, err)
	assert.Equal(t, "/downloads", info.Path)
}

func TestTrGetSpeedLimit(t *testing.T) {
	srv := rpcServer(t, map[string]any{"session-get": map[string]any{
		"speed-limit-down-enabled": true,
		"speed-limit-down":         100,
		"speed-limit-up-enabled":   false,
		"speed-limit-up":           200,
	}})
	defer srv.Close()

	lim, err := covClient(srv.URL).GetSpeedLimit()
	require.NoError(t, err)
	assert.Equal(t, int64(100*1024), lim.DownloadLimit)
	assert.Equal(t, int64(200*1024), lim.UploadLimit)
	assert.True(t, lim.LimitEnabled)
}

func TestTrSetSpeedLimit(t *testing.T) {
	var sawSet bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "session-set" {
			sawSet = true
		}
		_ = json.NewEncoder(w).Encode(rpcResponse{Result: "success"})
	}))
	defer srv.Close()

	err := covClient(srv.URL).SetSpeedLimit(downloader.SpeedLimit{
		DownloadLimit: 1024 * 100, UploadLimit: 1024 * 200, LimitEnabled: true,
	})
	require.NoError(t, err)
	assert.True(t, sawSet)
}

func TestTrGetClientPaths(t *testing.T) {
	srv := rpcServer(t, map[string]any{"session-get": map[string]any{"download-dir": "/downloads"}})
	defer srv.Close()

	paths, err := covClient(srv.URL).GetClientPaths()
	require.NoError(t, err)
	assert.Equal(t, []string{"/downloads"}, paths)
}

func TestTrGetClientLabels(t *testing.T) {
	labels, err := covClient("http://unused").GetClientLabels()
	require.NoError(t, err)
	assert.Empty(t, labels)
}
