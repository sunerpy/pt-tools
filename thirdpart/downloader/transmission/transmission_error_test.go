package transmission

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func TestTrAuthenticate_409MissingSessionID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer srv.Close()

	c := covClient(srv.URL)
	c.sessionID = ""
	err := c.Authenticate()
	require.Error(t, err)
	assert.False(t, c.healthy)
}

func TestTrAuthenticate_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	err := covClient(srv.URL).Authenticate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "用户名或密码")
}

func TestTrAuthenticate_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	err := covClient(srv.URL).Authenticate()
	require.Error(t, err)
}

func TestTrAuthenticate_ConnectionError(t *testing.T) {
	c := covClient("http://127.0.0.1:1")
	err := c.Authenticate()
	require.Error(t, err)
	assert.False(t, c.healthy)
}

// TestTrSetters_ErrorBranches drives the error-return branch of every simple
// setter/action by pointing them at a server that always returns RPC failure.
func TestTrSetters_ErrorBranches(t *testing.T) {
	srv := failRPCServer(t)
	c := covClient(srv.URL)

	assert.Error(t, c.PauseTorrents([]string{"1"}))
	assert.Error(t, c.ResumeTorrents([]string{"1"}))
	assert.Error(t, c.RemoveTorrents([]string{"1"}, true))
	assert.Error(t, c.SetTorrentCategory("1", "cat"))
	assert.Error(t, c.SetTorrentTags("1", "a,b"))
	assert.Error(t, c.SetTorrentSavePath("1", "/x"))
	assert.Error(t, c.RecheckTorrent("1"))
}

func TestTrGetters_ErrorBranches(t *testing.T) {
	srv := failRPCServer(t)
	c := covClient(srv.URL)

	_, err := c.GetAllTorrents()
	assert.Error(t, err)
	_, err = c.GetIncompletePendingBytes(context.Background())
	assert.Error(t, err)
	_, err = c.CheckTorrentExists("h")
	assert.Error(t, err)
	_, err = c.GetTorrentFiles("1")
	assert.Error(t, err)
	_, err = c.GetTorrentTrackers("1")
	assert.Error(t, err)
	_, err = c.GetClientVersion()
	assert.Error(t, err)
	_, err = c.GetDiskSpace(context.Background())
	assert.Error(t, err)
}

func TestTrGetAllTorrents_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(rpcResponse{Result: "success", Arguments: json.RawMessage("not-json")})
	}))
	defer srv.Close()

	_, err := covClient(srv.URL).GetAllTorrents()
	require.Error(t, err)
}

func TestTrProcessSingleTorrentFile_DiskSpaceWarnContinues(t *testing.T) {
	// session-get returns failure so GetDiskSpace errors → warn branch, but the
	// subsequent processTorrentFile still runs (and fails on read path here).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "session-get" {
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "failure"})
			return
		}
		_ = json.NewEncoder(w).Encode(rpcResponse{Result: "success"})
	}))
	defer srv.Close()

	c := covClient(srv.URL)
	err := c.ProcessSingleTorrentFile(context.Background(), "/nonexistent.torrent", "", "")
	require.Error(t, err)
}

func TestTrEnsureTorrentStarted_GetError(t *testing.T) {
	srv := failRPCServer(t)
	c := covClient(srv.URL)
	c.autoStart = true
	require.Error(t, c.EnsureTorrentStarted("h1"))
}

func TestTrVerifyConnection_Errors(t *testing.T) {
	t.Run("unauthorized", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer srv.Close()
		c := covClient(srv.URL)
		require.Error(t, c.verifyConnection())
	})

	t.Run("non-200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer srv.Close()
		c := covClient(srv.URL)
		require.Error(t, c.verifyConnection())
	})

	t.Run("connection error", func(t *testing.T) {
		c := covClient("http://127.0.0.1:1")
		require.Error(t, c.verifyConnection())
	})
}

func TestTrGetClientVersion_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := covClient(srv.URL).GetClientVersion()
	require.Error(t, err)
}

func TestTrSetters(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		methods = append(methods, req.Method)
		_ = json.NewEncoder(w).Encode(rpcResponse{Result: "success"})
	}))
	defer srv.Close()
	c := covClient(srv.URL)

	require.NoError(t, c.SetTorrentCategory("1", "movies"))
	require.NoError(t, c.SetTorrentTags("1", "a,b"))
	require.NoError(t, c.SetTorrentSavePath("1", "/new"))
	require.NoError(t, c.RecheckTorrent("1"))

	assert.Contains(t, methods, "torrent-set")
	assert.Contains(t, methods, "torrent-set-location")
	assert.Contains(t, methods, "torrent-verify")
}

func TestTrEnsureTorrentStarted(t *testing.T) {
	t.Run("no autostart", func(t *testing.T) {
		c := covClient("http://unused")
		c.autoStart = false
		require.NoError(t, c.EnsureTorrentStarted("h1"))
	})

	t.Run("starts stopped torrent", func(t *testing.T) {
		var started bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req rpcRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			resp := rpcResponse{Result: "success"}
			switch req.Method {
			case "torrent-get":
				raw, _ := json.Marshal(map[string]any{
					"torrents": []map[string]any{{"id": 1, "name": "t", "hashString": "h1", "status": 0}},
				})
				resp.Arguments = raw
			case "torrent-start":
				started = true
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer srv.Close()

		c := covClient(srv.URL)
		c.autoStart = true
		require.NoError(t, c.EnsureTorrentStarted("h1"))
		assert.True(t, started)
	})

	t.Run("already running", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-get": map[string]any{
			"torrents": []map[string]any{{"id": 1, "name": "t", "hashString": "h1", "status": 4}},
		}})
		defer srv.Close()

		c := covClient(srv.URL)
		c.autoStart = true
		require.NoError(t, c.EnsureTorrentStarted("h1"))
	})

	t.Run("not found", func(t *testing.T) {
		srv := rpcServer(t, map[string]any{"torrent-get": map[string]any{"torrents": []map[string]any{}}})
		defer srv.Close()

		c := covClient(srv.URL)
		c.autoStart = true
		require.Error(t, c.EnsureTorrentStarted("h1"))
	})
}

func TestTrWrapConnectionError(t *testing.T) {
	c := covClient("http://x")
	cases := []struct{ in, want string }{
		{"connection refused", "连接被拒绝"},
		{"no such host", "无法解析主机名"},
		{"timeout", "连接超时"},
		{"deadline exceeded", "连接超时"},
		{"x509 certificate", "SSL 证书错误"},
		{"weird", "连接失败"},
	}
	for _, tc := range cases {
		err := c.wrapConnectionError(errString(tc.in))
		require.Error(t, err)
		assert.Contains(t, err.Error(), tc.want, tc.in)
	}
}

func TestTrWrapStatusCodeError(t *testing.T) {
	c := covClient("http://x")
	assert.Contains(t, c.wrapStatusCodeError(http.StatusForbidden).Error(), "403")
	assert.Contains(t, c.wrapStatusCodeError(http.StatusNotFound).Error(), "404")
	assert.Contains(t, c.wrapStatusCodeError(http.StatusTeapot).Error(), "418")
}

func TestTrProcessSingleTorrentFile(t *testing.T) {
	// AddTorrent sleeps 500ms after adding; keep a single new-torrent flow.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		resp := rpcResponse{Result: "success"}
		switch req.Method {
		case "session-get":
			resp.Arguments, _ = json.Marshal(map[string]any{"download-dir": "/dl"})
		case "free-space":
			resp.Arguments, _ = json.Marshal(freeSpaceResponse{Path: "/dl", SizeBytes: 1 << 40})
		case "torrent-get":
			resp.Arguments, _ = json.Marshal(map[string]any{"torrents": []map[string]any{}})
		case "torrent-add":
			resp.Arguments, _ = json.Marshal(torrentAddResponse{TorrentAdded: &torrentInfo{ID: 1, HashString: "hh"}})
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()
	c := covClient(srv.URL)
	c.autoStart = false

	dir := t.TempDir()
	path := filepath.Join(dir, "x.torrent")
	require.NoError(t, os.WriteFile(path, makeTorrentBytes(t), 0o644))

	require.NoError(t, c.ProcessSingleTorrentFile(context.Background(), path, "cat", "tag"))
}

func TestTrProcessTorrentFile_Errors(t *testing.T) {
	c := covClient("http://unused")
	require.Error(t, c.processTorrentFile(context.Background(), "/nonexistent.torrent", "", ""))

	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.torrent")
	require.NoError(t, os.WriteFile(bad, []byte("not bencode"), 0o644))
	require.Error(t, c.processTorrentFile(context.Background(), bad, "", ""))
}

func TestTrDoRequest_ErrorPaths(t *testing.T) {
	t.Run("client closed", func(t *testing.T) {
		c := covClient("http://unused")
		c.client = nil
		_, err := c.doRequest("session-get", nil)
		require.Error(t, err)
	})

	t.Run("non-200 status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := covClient(srv.URL).doRequest("session-get", nil)
		require.Error(t, err)
	})

	t.Run("malformed json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("{bad"))
		}))
		defer srv.Close()

		_, err := covClient(srv.URL).doRequest("session-get", nil)
		require.Error(t, err)
	})

	t.Run("409 refreshes session and retries", func(t *testing.T) {
		var calls int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if calls == 1 {
				w.Header().Set("X-Transmission-Session-Id", "new-sess")
				w.WriteHeader(http.StatusConflict)
				return
			}
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "success"})
		}))
		defer srv.Close()

		_, err := covClient(srv.URL).doRequest("session-get", nil)
		require.NoError(t, err)
		assert.Equal(t, 2, calls)
	})
}

func TestTrGetClientStatus_Errors(t *testing.T) {
	t.Run("malformed json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "success", Arguments: json.RawMessage("not json")})
		}))
		defer srv.Close()

		_, err := covClient(srv.URL).GetClientStatus()
		require.Error(t, err)
	})

	t.Run("rpc error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "failure"})
		}))
		defer srv.Close()

		_, err := covClient(srv.URL).GetClientStatus()
		require.Error(t, err)
	})
}

func TestTrGetDiskInfo_Errors(t *testing.T) {
	t.Run("session-get rpc error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "failure"})
		}))
		defer srv.Close()

		_, err := covClient(srv.URL).GetDiskInfo()
		require.Error(t, err)
	})

	t.Run("free-space rpc error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req rpcRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Method == "free-space" {
				_ = json.NewEncoder(w).Encode(rpcResponse{Result: "failure"})
				return
			}
			raw, _ := json.Marshal(map[string]any{"download-dir": "/dl"})
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "success", Arguments: raw})
		}))
		defer srv.Close()

		_, err := covClient(srv.URL).GetDiskInfo()
		require.Error(t, err)
	})
}

func TestTrGetSpeedLimit_Errors(t *testing.T) {
	t.Run("rpc error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "failure"})
		}))
		defer srv.Close()

		_, err := covClient(srv.URL).GetSpeedLimit()
		require.Error(t, err)
	})

	t.Run("malformed json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "success", Arguments: json.RawMessage("bad")})
		}))
		defer srv.Close()

		_, err := covClient(srv.URL).GetSpeedLimit()
		require.Error(t, err)
	})
}

func TestTrSetSpeedLimit_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(rpcResponse{Result: "failure"})
	}))
	defer srv.Close()

	err := covClient(srv.URL).SetSpeedLimit(downloader.SpeedLimit{})
	require.Error(t, err)
}

func TestTrGetClientPaths_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(rpcResponse{Result: "failure"})
	}))
	defer srv.Close()

	_, err := covClient(srv.URL).GetClientPaths()
	require.Error(t, err)
}
