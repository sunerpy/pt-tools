package qbit

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func TestQbitGetDiskSpace_ErrorBranches(t *testing.T) {
	t.Run("non-success status", func(t *testing.T) {
		srv := failStatusServer(t, http.StatusInternalServerError)
		_, err := coverageTestClient(srv.URL, false).GetDiskSpace(context.Background())
		require.Error(t, err)
	})

	t.Run("missing server_state", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"other":1}`))
		}))
		defer srv.Close()
		_, err := coverageTestClient(srv.URL, false).GetDiskSpace(context.Background())
		require.Error(t, err)
	})

	t.Run("missing free_space", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"server_state":{"x":1}}`))
		}))
		defer srv.Close()
		_, err := coverageTestClient(srv.URL, false).GetDiskSpace(context.Background())
		require.Error(t, err)
	})

	t.Run("malformed json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("{bad"))
		}))
		defer srv.Close()
		_, err := coverageTestClient(srv.URL, false).GetDiskSpace(context.Background())
		require.Error(t, err)
	})
}

func TestQbitGetDiskInfo_Errors(t *testing.T) {
	t.Run("request error", func(t *testing.T) {
		srv := failStatusServer(t, http.StatusInternalServerError)
		_, err := coverageTestClient(srv.URL, false).GetDiskInfo()
		require.Error(t, err)
	})

	t.Run("missing server_state", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"x":1}`))
		}))
		defer srv.Close()
		_, err := coverageTestClient(srv.URL, false).GetDiskInfo()
		require.Error(t, err)
	})
}

func TestQbitGetSpeedLimit_ModeError(t *testing.T) {
	srv := failStatusServer(t, http.StatusInternalServerError)
	_, err := coverageTestClient(srv.URL, false).GetSpeedLimit()
	require.Error(t, err)
}

func TestQbitGetSpeedLimit_DownloadLimitError_C2(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/transfer/speedLimitsMode" {
			_, _ = w.Write([]byte("0"))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	_, err := coverageTestClient(srv.URL, false).GetSpeedLimit()
	require.Error(t, err)
}

func TestQbitSetSpeedLimit_DownloadPostError(t *testing.T) {
	srv := failStatusServer(t, http.StatusInternalServerError)
	err := coverageTestClient(srv.URL, false).SetSpeedLimit(downloader.SpeedLimit{})
	require.Error(t, err)
}

func TestQbitGetClientPaths_Errors(t *testing.T) {
	t.Run("status error", func(t *testing.T) {
		srv := failStatusServer(t, http.StatusInternalServerError)
		_, err := coverageTestClient(srv.URL, false).GetClientPaths()
		require.Error(t, err)
	})
	t.Run("malformed json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("{bad"))
		}))
		defer srv.Close()
		_, err := coverageTestClient(srv.URL, false).GetClientPaths()
		require.Error(t, err)
	})
}

func TestQbitGetClientLabels_Errors(t *testing.T) {
	srv := failStatusServer(t, http.StatusInternalServerError)
	_, err := coverageTestClient(srv.URL, false).GetClientLabels()
	require.Error(t, err)
}

func TestQbitGetClientVersion_Error(t *testing.T) {
	srv := failStatusServer(t, http.StatusInternalServerError)
	_, err := coverageTestClient(srv.URL, false).GetClientVersion()
	require.Error(t, err)
}

func TestQbitGetIncompletePendingBytes_Errors(t *testing.T) {
	t.Run("status error", func(t *testing.T) {
		srv := failStatusServer(t, http.StatusInternalServerError)
		_, err := coverageTestClient(srv.URL, false).GetIncompletePendingBytes(context.Background())
		require.Error(t, err)
	})
	t.Run("malformed json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("{bad"))
		}))
		defer srv.Close()
		_, err := coverageTestClient(srv.URL, false).GetIncompletePendingBytes(context.Background())
		require.Error(t, err)
	})
}

func TestQbitGetAllTorrents_StatusError(t *testing.T) {
	srv := failStatusServer(t, http.StatusInternalServerError)
	_, err := coverageTestClient(srv.URL, false).GetAllTorrents()
	require.Error(t, err)
}

func TestQbitCallPauseResumeEndpoints_LegacyFallback(t *testing.T) {
	var sawLegacy bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/torrents/pause":
			w.WriteHeader(http.StatusNotFound) // modern endpoint missing
		case "/api/v2/torrents/stop":
			sawLegacy = true
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	err := coverageTestClient(srv.URL, false).callPauseResumeEndpoints(
		[]string{"h1"}, "/api/v2/torrents/pause", "/api/v2/torrents/stop",
	)
	require.NoError(t, err)
	assert.True(t, sawLegacy, "404 on modern endpoint must fall back to legacy")
}

func TestQbitCallPauseResumeEndpoints_EmptyIDs(t *testing.T) {
	err := coverageTestClient("http://unused", false).callPauseResumeEndpoints(
		nil, "/api/v2/torrents/pause", "/api/v2/torrents/stop",
	)
	require.NoError(t, err)
}

func TestQbitCallPauseResumeEndpoints_StatusError(t *testing.T) {
	srv := failStatusServer(t, http.StatusInternalServerError)
	err := coverageTestClient(srv.URL, false).callPauseResumeEndpoints(
		[]string{"h1"}, "/api/v2/torrents/pause", "/api/v2/torrents/stop",
	)
	require.Error(t, err)
}

func TestQbitPostForm_StatusError(t *testing.T) {
	srv := failStatusServer(t, http.StatusInternalServerError)
	err := coverageTestClient(srv.URL, false).postForm("/api/v2/x", nil)
	require.Error(t, err)
}

func TestQbitGetJSON_Errors(t *testing.T) {
	t.Run("status error", func(t *testing.T) {
		srv := failStatusServer(t, http.StatusInternalServerError)
		var dst map[string]any
		err := coverageTestClient(srv.URL, false).getJSON("/api/v2/x", &dst)
		require.Error(t, err)
	})
	t.Run("malformed json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("{bad"))
		}))
		defer srv.Close()
		var dst map[string]any
		err := coverageTestClient(srv.URL, false).getJSON("/api/v2/x", &dst)
		require.Error(t, err)
	})
}

func TestQbitAddTorrentWithPath_StatusError(t *testing.T) {
	srv := failStatusServer(t, http.StatusInternalServerError)
	err := coverageTestClient(srv.URL, false).AddTorrentWithPath([]byte("data"), "cat", "tag", "/p")
	require.Error(t, err)
}

func TestQbitProcessTorrentFile_ReadError_C2(t *testing.T) {
	c := coverageTestClient("http://unused", false)
	require.Error(t, c.processTorrentFile(context.Background(), "/no/such.torrent", "", ""))
}

func TestQbitProcessTorrentFile_BadBencode(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.torrent")
	require.NoError(t, os.WriteFile(bad, []byte("not bencode"), 0o644))
	c := coverageTestClient("http://unused", false)
	require.Error(t, c.processTorrentFile(context.Background(), bad, "", ""))
}

func TestQbitProcessSingleTorrentFile_DiskSpaceError(t *testing.T) {
	srv := failStatusServer(t, http.StatusInternalServerError)
	c := coverageTestClient(srv.URL, false)
	err := c.ProcessSingleTorrentFile(context.Background(), "/no/such.torrent", "", "")
	require.Error(t, err)
}

func TestQbitDoRequestWithRetry_ClientClosed_C2(t *testing.T) {
	c := coverageTestClient("http://unused", false)
	c.client = nil
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://x", nil)
	_, err := c.doRequestWithRetry(req)
	require.Error(t, err)
}

func TestQbitEnsureTorrentStarted_NoAutoStart(t *testing.T) {
	c := coverageTestClient("http://unused", false)
	c.autoStart = false
	require.NoError(t, c.EnsureTorrentStarted("h1"))
}

func TestQbitAuthenticate_FailsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			_, _ = w.Write([]byte("Fails."))
		}
	}))
	defer srv.Close()

	c := coverageTestClient(srv.URL, false)
	err := c.Authenticate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "用户名或密码")
	assert.False(t, c.healthy)
}

func TestQbitAuthenticate_HTMLBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			_, _ = w.Write([]byte("<!DOCTYPE html><html><body>login</body></html>"))
		}
	}))
	defer srv.Close()

	c := coverageTestClient(srv.URL, false)
	err := c.Authenticate()
	require.Error(t, err)
	assert.False(t, c.healthy)
}

func TestQbitAuthenticate_StatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := coverageTestClient(srv.URL, false)
	err := c.Authenticate()
	require.Error(t, err)
	assert.False(t, c.healthy)
}

func TestQbitAuthenticate_ConnError(t *testing.T) {
	c := coverageTestClient("http://127.0.0.1:1", false)
	err := c.Authenticate()
	require.Error(t, err)
	assert.False(t, c.healthy)
}

func TestQbitEnsureTorrentStarted_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/torrents/info" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := coverageTestClient(srv.URL, false)
	c.autoStart = true
	require.Error(t, c.EnsureTorrentStarted("hash1"))
}

func TestQbitEnsureTorrentStarted_InfoStatusError(t *testing.T) {
	srv := failStatusServer(t, http.StatusInternalServerError)
	c := coverageTestClient(srv.URL, false)
	c.autoStart = true
	require.Error(t, c.EnsureTorrentStarted("hash1"))
}

func TestQbitProcessTorrentDirectory_DiskError(t *testing.T) {
	srv := failStatusServer(t, http.StatusInternalServerError)
	c := coverageTestClient(srv.URL, false)
	require.Error(t, c.ProcessTorrentDirectory(context.Background(), t.TempDir(), "", ""))
}

func TestQbitProcessTorrentDirectory_ReadDirError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":1099511627776}}`))
	}))
	defer srv.Close()
	c := coverageTestClient(srv.URL, false)
	require.Error(t, c.ProcessTorrentDirectory(context.Background(), "/no/such/dir-xyz", "", ""))
}

func TestQbitGetSpeedLimit_UploadLimitError_C2(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/transfer/speedLimitsMode":
			_, _ = w.Write([]byte("1"))
		case "/api/v2/transfer/downloadLimit":
			_, _ = w.Write([]byte("100"))
		case "/api/v2/transfer/uploadLimit":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	_, err := coverageTestClient(srv.URL, false).GetSpeedLimit()
	require.Error(t, err)
}

func TestQbitAddTorrentFileEx_LegacyStatusError(t *testing.T) {
	srv := failStatusServer(t, http.StatusInternalServerError)
	res, err := coverageTestClient(srv.URL, false).AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{})
	require.Error(t, err)
	assert.False(t, res.Success)
}

func TestQbitAddTorrentFileEx_BadTorrentHash(t *testing.T) {
	res, err := coverageTestClient("http://unused", false).AddTorrentFileEx([]byte("not-bencode"), downloader.AddTorrentOptions{})
	require.Error(t, err)
	assert.False(t, res.Success)
}

func TestQbitGetAllTorrents_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{bad"))
	}))
	defer srv.Close()
	_, err := coverageTestClient(srv.URL, false).GetAllTorrents()
	require.Error(t, err)
}

func TestQbitGetClientLabels_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{bad"))
	}))
	defer srv.Close()
	_, err := coverageTestClient(srv.URL, false).GetClientLabels()
	require.Error(t, err)
}

func TestQbitGetTorrent_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	_, err := coverageTestClient(srv.URL, false).GetTorrent("missing")
	require.ErrorIs(t, err, downloader.ErrTorrentNotFound)
}

func TestQbitProcessTorrentFile_InsufficientSpaceSkips(t *testing.T) {
	data := makeSingleFileTorrent(t, 5_000_000_000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/torrents/properties":
			w.WriteHeader(http.StatusNotFound)
		case "/api/v2/sync/maindata":
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":1}}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "big.torrent")
	require.NoError(t, os.WriteFile(path, data, 0o644))

	c := coverageTestClient(srv.URL, false)
	require.NoError(t, c.processTorrentFile(context.Background(), path, "", ""))
	_, statErr := os.Stat(path)
	require.NoError(t, statErr, "file should remain since torrent was skipped for space")
}

func TestQbitCheckTorrentExists_NotFound(t *testing.T) {
	srv := failStatusServer(t, http.StatusNotFound)
	got, err := coverageTestClient(srv.URL, false).CheckTorrentExists("h1")
	require.NoError(t, err)
	assert.False(t, got)
}

// TestQbitDoRequestWithRetry_403ReAuth drives doRequestWithRetry's 403 →
// re-authenticate → retry path for a bodyless GET. The first version fetch
// (from CheckTorrentExists) returns 403, triggering Authenticate (login Ok. +
// version probe), after which the retried request succeeds.
func TestQbitDoRequestWithRetry_403ReAuth(t *testing.T) {
	var propsCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/torrents/properties":
			propsCalls++
			if propsCalls == 1 {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			_, _ = w.Write([]byte(`{"hash":"h1"}`))
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			_, _ = w.Write([]byte("v4.6.0"))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	c := coverageTestClient(srv.URL, false)
	got, err := c.CheckTorrentExists("h1")
	require.NoError(t, err)
	assert.True(t, got)
	assert.Equal(t, 2, propsCalls, "403 must trigger one retry after re-auth")
}

// TestQbitDoRequestWithRetry_ReAuthFails covers the branch where the 403
// re-auth attempt itself fails.
func TestQbitDoRequestWithRetry_ReAuthFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/torrents/properties":
			w.WriteHeader(http.StatusForbidden)
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Fails."))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	c := coverageTestClient(srv.URL, false)
	_, err := c.CheckTorrentExists("h1")
	require.Error(t, err)
}

// TestQbitAddTorrentEx_V520OKNonJSON drives AddTorrentEx's v520 branch where the
// 200 body is neither valid result-JSON nor "Fails." → generic success.
func TestQbitAddTorrentEx_V520OKNonJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("Ok."))
	}))
	defer srv.Close()
	res, err := coverageTestClient(srv.URL, true).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{})
	require.NoError(t, err)
	assert.True(t, res.Success)
}

// TestQbitAddTorrentFileEx_V520OKNonJSON drives the analogous v520 generic
// success path for the file-based add.
func TestQbitAddTorrentFileEx_V520OKNonJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("Ok."))
	}))
	defer srv.Close()
	res, err := coverageTestClient(srv.URL, true).AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{})
	require.NoError(t, err)
	assert.True(t, res.Success)
}

// TestQbitAddTorrentFileEx_V520Conflict covers the 409 conflict branch.
func TestQbitAddTorrentFileEx_V520Conflict(t *testing.T) {
	srv := failStatusServer(t, http.StatusConflict)
	res, err := coverageTestClient(srv.URL, true).AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{})
	require.NoError(t, err)
	assert.False(t, res.Success)
}

// TestQbitAddTorrentFileEx_V520Fails covers the "Fails." body branch.
func TestQbitAddTorrentFileEx_V520FailsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("Fails."))
	}))
	defer srv.Close()
	res, err := coverageTestClient(srv.URL, true).AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{})
	require.Error(t, err)
	assert.False(t, res.Success)
}

// TestQbitAddTorrentFileEx_V520DefaultStatus covers the default-status error.
func TestQbitAddTorrentFileEx_V520DefaultStatus(t *testing.T) {
	srv := failStatusServer(t, http.StatusInternalServerError)
	res, err := coverageTestClient(srv.URL, true).AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{})
	require.Error(t, err)
	assert.False(t, res.Success)
}

// TestQbitGetAllTorrents_MapsRichFields feeds a fully-populated torrent so
// mapQbitTorrent's field extraction branches run.
func TestQbitGetSpeedLimit_DisabledMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/transfer/speedLimitsMode":
			_, _ = w.Write([]byte("0"))
		case "/api/v2/transfer/downloadLimit":
			_, _ = w.Write([]byte("0"))
		case "/api/v2/transfer/uploadLimit":
			_, _ = w.Write([]byte("0"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	lim, err := coverageTestClient(srv.URL, false).GetSpeedLimit()
	require.NoError(t, err)
	assert.False(t, lim.LimitEnabled)
}

func TestQbitGetClientVersion_ConnError(t *testing.T) {
	_, err := coverageTestClient("http://127.0.0.1:1", false).GetClientVersion()
	require.Error(t, err)
}

func TestQbitAddTorrentEx_ConnError(t *testing.T) {
	res, err := coverageTestClient("http://127.0.0.1:1", false).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{})
	require.Error(t, err)
	assert.False(t, res.Success)
}

func TestQbitProcessTorrentFile_CheckExistsError(t *testing.T) {
	data := makeSingleFileTorrent(t, 1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/torrents/properties" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "x.torrent")
	require.NoError(t, os.WriteFile(path, data, 0o644))

	c := coverageTestClient(srv.URL, false)
	require.Error(t, c.processTorrentFile(context.Background(), path, "", ""))
}

func TestQbitProcessTorrentFile_CanAddError(t *testing.T) {
	data := makeSingleFileTorrent(t, 1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/torrents/properties":
			w.WriteHeader(http.StatusNotFound)
		case "/api/v2/sync/maindata":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "x.torrent")
	require.NoError(t, os.WriteFile(path, data, 0o644))

	c := coverageTestClient(srv.URL, false)
	require.Error(t, c.processTorrentFile(context.Background(), path, "", ""))
}

func TestQbitProcessTorrentFile_AddError(t *testing.T) {
	data := makeSingleFileTorrent(t, 1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/torrents/properties":
			w.WriteHeader(http.StatusNotFound)
		case "/api/v2/sync/maindata":
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":1099511627776}}`))
		case "/api/v2/torrents/add":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "x.torrent")
	require.NoError(t, os.WriteFile(path, data, 0o644))

	c := coverageTestClient(srv.URL, false)
	require.Error(t, c.processTorrentFile(context.Background(), path, "", ""))
}

func TestQbitGetClientPaths_EmptySavePath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"save_path":""}`))
	}))
	defer srv.Close()
	paths, err := coverageTestClient(srv.URL, false).GetClientPaths()
	require.NoError(t, err)
	assert.Empty(t, paths)
}

func TestQbitAddTorrentWithPath_ConnError(t *testing.T) {
	c := coverageTestClient("http://127.0.0.1:1", false)
	require.Error(t, c.AddTorrentWithPath([]byte("data"), "", "", ""))
}

func TestQbitAddTorrentFileEx_ConnError(t *testing.T) {
	res, err := coverageTestClient("http://127.0.0.1:1", false).AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{})
	require.Error(t, err)
	assert.False(t, res.Success)
}

func TestQbitEnsureTorrentStarted_ResumeStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/torrents/info":
			_, _ = w.Write([]byte(`[{"name":"t","state":"pausedUP"}]`))
		case "/api/v2/torrents/resume":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	c := coverageTestClient(srv.URL, false)
	c.autoStart = true
	require.Error(t, c.EnsureTorrentStarted("h1"))
}

func TestQbitEnsureTorrentStarted_InfoMalformed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/torrents/info" {
			_, _ = w.Write([]byte("{bad"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := coverageTestClient(srv.URL, false)
	c.autoStart = true
	require.Error(t, c.EnsureTorrentStarted("h1"))
}

func TestQbitEnsureTorrentStarted_ConnError(t *testing.T) {
	c := coverageTestClient("http://127.0.0.1:1", false)
	c.autoStart = true
	require.Error(t, c.EnsureTorrentStarted("h1"))
}

func TestQbitGetSpeedLimit_ModeConnError(t *testing.T) {
	c := coverageTestClient("http://127.0.0.1:1", false)
	_, err := c.GetSpeedLimit()
	require.Error(t, err)
}

func TestQbitGetTorrent_GetAllError(t *testing.T) {
	srv := failStatusServer(t, http.StatusInternalServerError)
	_, err := coverageTestClient(srv.URL, false).GetTorrent("h1")
	require.Error(t, err)
}

func TestQbitComputeTorrentHash_MissingInfo(t *testing.T) {
	_, err := ComputeTorrentHash([]byte("d8:announce3:xyze"))
	require.Error(t, err)
}

func TestQbitGetTorrentFilesPath_Error(t *testing.T) {
	_, err := GetTorrentFilesPath("/no/such/dir-abc")
	require.Error(t, err)
}

func TestQbitParseQBitVersion_Overflow(t *testing.T) {
	_, _, _, ok := parseQBitVersion("99999999999999999999.1.1")
	assert.False(t, ok)
}

func TestQbitGetClientStatus_ConnError(t *testing.T) {
	_, err := coverageTestClient("http://127.0.0.1:1", false).GetClientStatus()
	require.Error(t, err)
}

func TestQbitGetIncompletePendingBytes_ConnError(t *testing.T) {
	_, err := coverageTestClient("http://127.0.0.1:1", false).GetIncompletePendingBytes(context.Background())
	require.Error(t, err)
}

func TestQbitGetAllTorrents_ConnError(t *testing.T) {
	_, err := coverageTestClient("http://127.0.0.1:1", false).GetAllTorrents()
	require.Error(t, err)
}

func TestQbitGetClientStatus_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("{bad"))
	}))
	defer srv.Close()
	_, err := coverageTestClient(srv.URL, false).GetClientStatus()
	require.Error(t, err)
}

func TestQbitGetClientStatus_MissingServerState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"x":1}`))
	}))
	defer srv.Close()
	_, err := coverageTestClient(srv.URL, false).GetClientStatus()
	require.Error(t, err)
}

func TestQbitPing_ConnError(t *testing.T) {
	ok, err := coverageTestClient("http://127.0.0.1:1", false).Ping()
	require.Error(t, err)
	assert.False(t, ok)
}

func TestQbitPing_NonSuccessStatus(t *testing.T) {
	srv := failStatusServer(t, http.StatusInternalServerError)
	ok, err := coverageTestClient(srv.URL, false).Ping()
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestQbitGetDiskSpace_ConnError(t *testing.T) {
	_, err := coverageTestClient("http://127.0.0.1:1", false).GetDiskSpace(context.Background())
	require.Error(t, err)
}

func TestQbitPostForm_ConnError(t *testing.T) {
	err := coverageTestClient("http://127.0.0.1:1", false).postForm("/api/v2/x", nil)
	require.Error(t, err)
}

func TestQbitGetJSON_ConnError(t *testing.T) {
	var dst map[string]any
	err := coverageTestClient("http://127.0.0.1:1", false).getJSON("/api/v2/x", &dst)
	require.Error(t, err)
}

func TestQbitGetClientPaths_ConnError(t *testing.T) {
	_, err := coverageTestClient("http://127.0.0.1:1", false).GetClientPaths()
	require.Error(t, err)
}

func TestQbitGetClientLabels_ConnError(t *testing.T) {
	_, err := coverageTestClient("http://127.0.0.1:1", false).GetClientLabels()
	require.Error(t, err)
}

func TestQbitCallPauseResume_ConnError(t *testing.T) {
	err := coverageTestClient("http://127.0.0.1:1", false).callPauseResumeEndpoints(
		[]string{"h1"}, "/api/v2/torrents/stop", "/api/v2/torrents/pause",
	)
	require.Error(t, err)
}

func TestQbitGetSpeedLimit_DownloadReadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/transfer/speedLimitsMode":
			_, _ = w.Write([]byte("1"))
		case "/api/v2/transfer/downloadLimit":
			w.Header().Set("Content-Length", "1000")
			_, _ = w.Write([]byte("10"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			if hj, ok := w.(http.Hijacker); ok {
				conn, _, _ := hj.Hijack()
				_ = conn.Close()
			}
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	_, err := coverageTestClient(srv.URL, false).GetSpeedLimit()
	require.Error(t, err)
}

func TestQbitAddTorrentEx_LegacyStatusError(t *testing.T) {
	srv := failStatusServer(t, http.StatusInternalServerError)
	res, err := coverageTestClient(srv.URL, false).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{})
	require.Error(t, err)
	assert.False(t, res.Success)
}

func TestQbitAddTorrentEx_V520Conflict(t *testing.T) {
	srv := failStatusServer(t, http.StatusConflict)
	res, err := coverageTestClient(srv.URL, true).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{})
	require.NoError(t, err)
	assert.False(t, res.Success)
}

func TestQbitAddTorrentEx_V520Fails_C2(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("Fails."))
	}))
	defer srv.Close()
	res, err := coverageTestClient(srv.URL, true).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{})
	require.Error(t, err)
	assert.False(t, res.Success)
}

func TestQbitAddTorrentEx_V520DefaultStatus(t *testing.T) {
	srv := failStatusServer(t, http.StatusInternalServerError)
	res, err := coverageTestClient(srv.URL, true).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{})
	require.Error(t, err)
	assert.False(t, res.Success)
}

func TestQbitPing(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v2/app/version", r.URL.Path)
			_, _ = w.Write([]byte("v4.6.5"))
		}))
		defer srv.Close()

		ok, err := coverageTestClient(srv.URL, false).Ping()
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("non-success status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		c := coverageTestClient(srv.URL, false)
		ok, err := c.Ping()
		require.NoError(t, err)
		assert.False(t, ok)
		assert.False(t, c.IsHealthy())
	})

	t.Run("network error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		url := srv.URL
		srv.Close() // server closed -> connection refused

		ok, err := coverageTestClient(url, false).Ping()
		require.Error(t, err)
		assert.False(t, ok)
	})
}

func TestQbitGetClientVersion(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("v5.0.0"))
		}))
		defer srv.Close()

		v, err := coverageTestClient(srv.URL, false).GetClientVersion()
		require.NoError(t, err)
		assert.Equal(t, "v5.0.0", v)
	})

	t.Run("error status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetClientVersion()
		require.Error(t, err)
	})
}

func TestQbitGetClientStatus(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"server_state": map[string]any{
					"up_info_speed": float64(100),
					"alltime_ul":    float64(2000),
					"dl_info_speed": float64(200),
					"alltime_dl":    float64(4000),
					"up_info_data":  float64(50),
					"dl_info_data":  float64(60),
				},
			})
		}))
		defer srv.Close()

		st, err := coverageTestClient(srv.URL, false).GetClientStatus()
		require.NoError(t, err)
		assert.Equal(t, int64(100), st.UpSpeed)
		assert.Equal(t, int64(2000), st.UpData)
		assert.Equal(t, int64(200), st.DlSpeed)
		assert.Equal(t, int64(4000), st.DlData)
		assert.Equal(t, int64(50), st.SessionUpData)
		assert.Equal(t, int64(60), st.SessionDlData)
	})

	t.Run("missing server_state", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"other":1}`))
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetClientStatus()
		require.Error(t, err)
	})

	t.Run("malformed json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`not json`))
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetClientStatus()
		require.Error(t, err)
	})

	t.Run("error status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetClientStatus()
		require.Error(t, err)
	})
}

func TestQbitGetIncompletePendingBytes(t *testing.T) {
	t.Run("aggregates active states", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v2/torrents/info", r.URL.Path)
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"state": "downloading", "amount_left": float64(1000)},
				{"state": "pausedDL", "amount_left": float64(500)},
				{"state": "uploading", "amount_left": float64(999)}, // not counted
				{"state": "stalledDL", "amount_left": float64(0)},   // zero skipped
			})
		}))
		defer srv.Close()

		got, err := coverageTestClient(srv.URL, false).GetIncompletePendingBytes(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(1500), got)
	})

	t.Run("http error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetIncompletePendingBytes(context.Background())
		require.Error(t, err)
	})

	t.Run("malformed json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{bad`))
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetIncompletePendingBytes(context.Background())
		require.Error(t, err)
	})
}

func TestQbitGetAllTorrents_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := coverageTestClient(srv.URL, false).GetAllTorrents()
	require.Error(t, err)
}

func TestQbitPauseResume_EmptyIDs(t *testing.T) {
	c := coverageTestClient("http://unused", true)
	assert.NoError(t, c.PauseTorrents(nil))
	assert.NoError(t, c.ResumeTorrents(nil))
	assert.NoError(t, c.RemoveTorrents(nil, true))
}

func TestQbitPauseResume_LegacyFallback(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		// modern endpoints 404 -> triggers legacy fallback
		if strings.HasSuffix(r.URL.Path, "/stop") || strings.HasSuffix(r.URL.Path, "/start") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := coverageTestClient(srv.URL, false)

	require.NoError(t, c.PauseTorrents([]string{"h1"}))
	require.NoError(t, c.ResumeTorrents([]string{"h1"}))
	assert.Contains(t, paths, "/api/v2/torrents/pause")
	assert.Contains(t, paths, "/api/v2/torrents/resume")
}

func TestQbitSetters_ErrorPropagation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := coverageTestClient(srv.URL, false)

	require.Error(t, c.SetTorrentCategory("h1", "movies"))
	require.Error(t, c.SetTorrentTags("h1", "tag1"))
	require.Error(t, c.SetTorrentSavePath("h1", "/new/path"))
	require.Error(t, c.RecheckTorrent("h1"))
}

func TestQbitGetTorrentFiles(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Contains(t, r.URL.Path, "/api/v2/torrents/files")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"index": float64(0), "name": "a.mkv", "size": float64(1000), "progress": float64(0.5), "priority": float64(1)},
				{"index": float64(1), "name": "b.mkv", "size": float64(2000), "progress": float64(1.0), "priority": float64(7)},
			})
		}))
		defer srv.Close()

		files, err := coverageTestClient(srv.URL, false).GetTorrentFiles("h1")
		require.NoError(t, err)
		require.Len(t, files, 2)
		assert.Equal(t, "a.mkv", files[0].Name)
		assert.Equal(t, int64(1000), files[0].Size)
		assert.Equal(t, 0.5, files[0].Progress)
		assert.Equal(t, 1, files[0].Priority)
		assert.Equal(t, 1, files[1].Index)
	})

	t.Run("error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).GetTorrentFiles("h1")
		require.Error(t, err)
	})
}

func TestQbitSetSpeedLimit_DownloadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := coverageTestClient(srv.URL, false).SetSpeedLimit(downloader.SpeedLimit{DownloadLimit: 1})
	require.Error(t, err)
}

func TestQbitEnsureTorrentStarted(t *testing.T) {
	t.Run("no autostart returns nil", func(t *testing.T) {
		c := coverageTestClient("http://unused", false)
		c.autoStart = false
		require.NoError(t, c.EnsureTorrentStarted("h1"))
	})

	t.Run("resumes paused torrent", func(t *testing.T) {
		var resumed bool
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.Contains(r.URL.Path, "/api/v2/torrents/info"):
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"name": "t", "state": "pausedDL"},
				})
			case strings.Contains(r.URL.Path, "/api/v2/torrents/resume"):
				resumed = true
				w.WriteHeader(http.StatusOK)
			default:
				w.WriteHeader(http.StatusOK)
			}
		}))
		defer srv.Close()

		c := coverageTestClient(srv.URL, false)
		c.autoStart = true
		require.NoError(t, c.EnsureTorrentStarted("h1"))
		assert.True(t, resumed)
	})

	t.Run("already running", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode([]map[string]any{{"name": "t", "state": "downloading"}})
		}))
		defer srv.Close()

		c := coverageTestClient(srv.URL, false)
		c.autoStart = true
		require.NoError(t, c.EnsureTorrentStarted("h1"))
	})

	t.Run("torrent not found", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`[]`))
		}))
		defer srv.Close()

		c := coverageTestClient(srv.URL, false)
		c.autoStart = true
		require.Error(t, c.EnsureTorrentStarted("h1"))
	})

	t.Run("info error status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		c := coverageTestClient(srv.URL, false)
		c.autoStart = true
		require.Error(t, c.EnsureTorrentStarted("h1"))
	})
}

func TestQbitWrapConnectionError(t *testing.T) {
	c := coverageTestClient("http://x", false)
	cases := []struct{ in, want string }{
		{"connection refused", "连接被拒绝"},
		{"no such host", "无法解析主机名"},
		{"timeout awaiting", "连接超时"},
		{"deadline exceeded", "连接超时"},
		{"x509 certificate signed", "SSL 证书错误"},
		{"some other error", "连接失败"},
	}
	for _, tc := range cases {
		err := c.wrapConnectionError(assertErr(tc.in))
		require.Error(t, err)
		assert.Contains(t, err.Error(), tc.want, tc.in)
	}
}

func TestQbitWrapStatusCodeError(t *testing.T) {
	c := coverageTestClient("http://x", false)
	assert.Contains(t, c.wrapStatusCodeError(http.StatusForbidden).Error(), "403")
	assert.Contains(t, c.wrapStatusCodeError(http.StatusNotFound).Error(), "404")
	assert.Contains(t, c.wrapStatusCodeError(http.StatusUnauthorized).Error(), "401")
	assert.Contains(t, c.wrapStatusCodeError(http.StatusTeapot).Error(), "418")
}

func TestQbitAddTorrentWithPath_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := coverageTestClient(srv.URL, false)

	err := c.AddTorrentWithPath(makeSingleFileTorrent(t, 1000), "", "", "")
	require.Error(t, err)
}

func TestQbitProcessTorrentFile_ReadError(t *testing.T) {
	c := coverageTestClient("http://unused", false)
	err := c.processTorrentFile(t.Context(), "/nonexistent/x.torrent", "", "")
	require.Error(t, err)
}

func TestQbitProcessTorrentDirectory(t *testing.T) {
	srv := qbitAddServer(t, false)
	defer srv.Close()
	c := coverageTestClient(srv.URL, false)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "1.torrent"), makeSingleFileTorrent(t, 1000), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "2.torrent"), makeSingleFileTorrent(t, 2000), 0o644))

	require.NoError(t, c.ProcessTorrentDirectory(t.Context(), dir, "cat", "tag"))
}

func TestQbitProcessTorrentDirectory_DiskSpaceError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := coverageTestClient(srv.URL, false)

	err := c.ProcessTorrentDirectory(t.Context(), t.TempDir(), "", "")
	require.Error(t, err)
}

func TestQbitDoRequestWithRetry_ReAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			_, _ = w.Write([]byte("v4.6.5"))
		default:
			_, _ = w.Write([]byte("ok"))
		}
	}))
	defer srv.Close()

	c := coverageTestClient(srv.URL, false)
	c.username = "admin"
	c.password = "pwd"
	c.client = &reauthDoer{inner: &standardHTTPDoer{client: &http.Client{}}}

	// GetClientVersion goes through doRequestWithRetry; first call 403 -> re-auth -> retry.
	v, err := c.GetClientVersion()
	require.NoError(t, err)
	assert.Equal(t, "v4.6.5", v)
}

func TestQbitDoRequestWithRetry_ClientClosed(t *testing.T) {
	c := coverageTestClient("http://unused", false)
	c.client = nil
	req, _ := http.NewRequest(http.MethodGet, "http://unused/x", bytes.NewReader(nil))
	_, err := c.doRequestWithRetry(req)
	require.Error(t, err)
}

func TestQbitAddTorrentFileEx_LegacyErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	res, err := coverageTestClient(srv.URL, false).AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{})
	require.Error(t, err)
	assert.False(t, res.Success)
}

func TestQbitAddTorrentFileEx_V520Fails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Fails."))
	}))
	defer srv.Close()

	res, err := coverageTestClient(srv.URL, true).AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{})
	require.Error(t, err)
	assert.False(t, res.Success)
}

func TestQbitAddTorrentFileEx_V520PlainOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Ok."))
	}))
	defer srv.Close()

	res, err := coverageTestClient(srv.URL, true).AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{})
	require.NoError(t, err)
	assert.True(t, res.Success)
}

func TestQbitAddTorrentFileEx_V520ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	res, err := coverageTestClient(srv.URL, true).AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{})
	require.Error(t, err)
	assert.False(t, res.Success)
}

func TestQbitAddTorrentEx_V520Fails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Fails."))
	}))
	defer srv.Close()

	res, err := coverageTestClient(srv.URL, true).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{})
	require.Error(t, err)
	assert.False(t, res.Success)
}

func TestQbitGetSpeedLimit_DownloadLimitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/transfer/speedLimitsMode":
			_, _ = w.Write([]byte("1"))
		case "/api/v2/transfer/downloadLimit":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	_, err := coverageTestClient(srv.URL, false).GetSpeedLimit()
	require.Error(t, err)
}

func TestQbitGetSpeedLimit_UploadLimitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/transfer/speedLimitsMode":
			_, _ = w.Write([]byte("1"))
		case "/api/v2/transfer/downloadLimit":
			_, _ = w.Write([]byte("100"))
		case "/api/v2/transfer/uploadLimit":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	_, err := coverageTestClient(srv.URL, false).GetSpeedLimit()
	require.Error(t, err)
}

func TestQbitSetSpeedLimit_UploadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/transfer/setUploadLimit" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := coverageTestClient(srv.URL, false).SetSpeedLimit(downloader.SpeedLimit{UploadLimit: 1})
	require.Error(t, err)
}

func TestQbitCheckTorrentExists(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"save_path":"/downloads"}`))
		}))
		defer srv.Close()

		got, err := coverageTestClient(srv.URL, false).CheckTorrentExists("hash1")
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("not found", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		got, err := coverageTestClient(srv.URL, false).CheckTorrentExists("hash1")
		require.NoError(t, err)
		assert.False(t, got)
	})

	t.Run("error status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).CheckTorrentExists("hash1")
		require.Error(t, err)
	})

	t.Run("malformed json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{bad`))
		}))
		defer srv.Close()

		_, err := coverageTestClient(srv.URL, false).CheckTorrentExists("hash1")
		require.Error(t, err)
	})
}
