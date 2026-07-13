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
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

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

func TestTrConfigGetURL_SchemeAndTrim(t *testing.T) {
	c := &TransmissionConfig{URL: "  host:9091/  "}
	assert.Equal(t, "http://host:9091", c.GetURL())

	c2 := &TransmissionConfig{URL: "https://tr.example:9091"}
	assert.Equal(t, "https://tr.example:9091", c2.GetURL())

	c3 := &TransmissionConfig{URL: ""}
	assert.Equal(t, "", c3.GetURL())
}

func TestTrConfigValidate_Branches(t *testing.T) {
	assert.Error(t, (&TransmissionConfig{URL: ""}).Validate())
	assert.Error(t, (&TransmissionConfig{URL: "://bad"}).Validate())
	assert.Error(t, (&TransmissionConfig{URL: "ftp://host"}).Validate())
	assert.Error(t, (&TransmissionConfig{URL: "http://user:pass@host"}).Validate())
	assert.Error(t, (&TransmissionConfig{URL: "http://host#frag"}).Validate())
	assert.NoError(t, (&TransmissionConfig{URL: "http://host:9091"}).Validate())
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

func TestTrAuthenticate_Success200(t *testing.T) {
	srv := rpcServer(t, map[string]any{"session-get": map[string]any{"version": "4.0"}})
	defer srv.Close()

	c := covClient(srv.URL)
	require.NoError(t, c.Authenticate())
	assert.True(t, c.healthy)
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

func TestTrEnter_SLoggerNotNil(t *testing.T) {
	assert.NotNil(t, sLogger())
}

var _ = downloader.SpeedLimit{}
