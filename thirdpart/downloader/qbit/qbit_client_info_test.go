package qbit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// TestQbitClientGetDiskSpace 测试获取磁盘空间
func TestQbitClientGetDiskSpace(t *testing.T) {
	server := createMockQbitServer()
	defer server.Close()

	config := NewQBitConfig(server.URL, "admin", "password")
	client, err := NewQbitClient(config, "test-qbit")
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

// TestQbitClientGetDiskSpaceError 测试获取磁盘空间失败
func TestQbitClientGetDiskSpaceError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Ok."))
		case "/api/v2/sync/maindata":
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	config := NewQBitConfig(server.URL, "admin", "password")
	client, err := NewQbitClient(config, "test-qbit")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	_, err = client.GetDiskSpace(ctx)
	if err == nil {
		t.Error("expected error for failed disk space request")
	}
}

// TestQbitClientGetDiskSpaceInvalidResponse 测试无效的磁盘空间响应
func TestQbitClientGetDiskSpaceInvalidResponse(t *testing.T) {
	server := createMockQbitServerWithInvalidDiskSpaceResponse()
	defer server.Close()

	config := NewQBitConfig(server.URL, "admin", "password")
	client, err := NewQbitClient(config, "test-qbit")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	_, err = client.GetDiskSpace(ctx)
	if err == nil {
		t.Error("expected error for invalid disk space response")
	}
}

func TestQbitGetDiskInfo_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":2048,"default_save_path":"/dl"}}`))
	}))
	defer srv.Close()

	info, err := coverageTestClient(srv.URL, false).GetDiskInfo()
	require.NoError(t, err)
	assert.Equal(t, int64(2048), info.FreeSpace)
	assert.Equal(t, "/dl", info.Path)
}

func TestQbitGetSpeedLimit_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/transfer/speedLimitsMode":
			_, _ = w.Write([]byte("1"))
		case "/api/v2/transfer/downloadLimit":
			_, _ = w.Write([]byte("102400"))
		case "/api/v2/transfer/uploadLimit":
			_, _ = w.Write([]byte("51200"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	lim, err := coverageTestClient(srv.URL, false).GetSpeedLimit()
	require.NoError(t, err)
	assert.Equal(t, int64(102400), lim.DownloadLimit)
	assert.Equal(t, int64(51200), lim.UploadLimit)
	assert.True(t, lim.LimitEnabled)
}

func TestQbitSetSpeedLimit_TogglesWhenModeDiffers(t *testing.T) {
	var toggled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/transfer/setDownloadLimit", "/api/v2/transfer/setUploadLimit":
			w.WriteHeader(http.StatusOK)
		case "/api/v2/transfer/speedLimitsMode":
			_, _ = w.Write([]byte("0")) // currently disabled
		case "/api/v2/transfer/downloadLimit", "/api/v2/transfer/uploadLimit":
			_, _ = w.Write([]byte("0"))
		case "/api/v2/transfer/toggleSpeedLimitsMode":
			toggled = true
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	err := coverageTestClient(srv.URL, false).SetSpeedLimit(downloader.SpeedLimit{
		DownloadLimit: 100, UploadLimit: 200, LimitEnabled: true,
	})
	require.NoError(t, err)
	assert.True(t, toggled, "toggle must fire when current mode differs from desired")
}

func TestQbitGetClientPaths_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"save_path":"/downloads"}`))
	}))
	defer srv.Close()

	paths, err := coverageTestClient(srv.URL, false).GetClientPaths()
	require.NoError(t, err)
	assert.Equal(t, []string{"/downloads"}, paths)
}

func TestQbitGetClientLabels_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"movies":{"name":"movies"},"tv":{"name":"tv"}}`))
	}))
	defer srv.Close()

	labels, err := coverageTestClient(srv.URL, false).GetClientLabels()
	require.NoError(t, err)
	assert.Len(t, labels, 2)
}

func TestQbitGetClientFreeSpace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":1073741824}}`))
	}))
	defer srv.Close()

	got, err := coverageTestClient(srv.URL, false).GetClientFreeSpace(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1073741824), got)
}

func TestQbitGetDiskInfo(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":5000,"default_save_path":"/dl"}}`))
		}))
		defer srv.Close()

		info, err := coverageTestClient(srv.URL, false).GetDiskInfo()
		require.NoError(t, err)
		assert.Equal(t, int64(5000), info.FreeSpace)
		assert.Equal(t, "/dl", info.Path)
	})

	t.Run("missing server_state", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"x":1}`))
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetDiskInfo()
		require.Error(t, err)
	})
}

func TestQbitGetSpeedLimit(t *testing.T) {
	t.Run("success enabled", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/v2/transfer/speedLimitsMode":
				_, _ = w.Write([]byte("1"))
			case "/api/v2/transfer/downloadLimit":
				_, _ = w.Write([]byte("1024"))
			case "/api/v2/transfer/uploadLimit":
				_, _ = w.Write([]byte("2048"))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		lim, err := coverageTestClient(srv.URL, false).GetSpeedLimit()
		require.NoError(t, err)
		assert.Equal(t, int64(1024), lim.DownloadLimit)
		assert.Equal(t, int64(2048), lim.UploadLimit)
		assert.True(t, lim.LimitEnabled)
	})

	t.Run("mode request error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetSpeedLimit()
		require.Error(t, err)
	})
}

func TestQbitSetSpeedLimit(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		switch r.URL.Path {
		case "/api/v2/transfer/speedLimitsMode":
			_, _ = w.Write([]byte("0")) // currently disabled
		case "/api/v2/transfer/downloadLimit":
			_, _ = w.Write([]byte("0"))
		case "/api/v2/transfer/uploadLimit":
			_, _ = w.Write([]byte("0"))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	err := coverageTestClient(srv.URL, false).SetSpeedLimit(downloader.SpeedLimit{
		DownloadLimit: 1000, UploadLimit: 2000, LimitEnabled: true,
	})
	require.NoError(t, err)
	assert.Contains(t, seen, "/api/v2/transfer/setDownloadLimit")
	assert.Contains(t, seen, "/api/v2/transfer/setUploadLimit")
	assert.Contains(t, seen, "/api/v2/transfer/toggleSpeedLimitsMode")
}

func TestQbitGetClientPaths(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v2/app/preferences", r.URL.Path)
			_, _ = w.Write([]byte(`{"save_path":"/downloads"}`))
		}))
		defer srv.Close()

		paths, err := coverageTestClient(srv.URL, false).GetClientPaths()
		require.NoError(t, err)
		assert.Equal(t, []string{"/downloads"}, paths)
	})

	t.Run("error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetClientPaths()
		require.Error(t, err)
	})
}

func TestQbitGetClientLabels(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v2/torrents/categories", r.URL.Path)
			_, _ = w.Write([]byte(`{"movies":{"name":"movies"},"tv":{"name":"tv"}}`))
		}))
		defer srv.Close()

		labels, err := coverageTestClient(srv.URL, false).GetClientLabels()
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"movies", "tv"}, labels)
	})

	t.Run("error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetClientLabels()
		require.Error(t, err)
	})
}
