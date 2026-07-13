package qbit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

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

func TestQbitAddTorrentWithPath_MinimalNoOptionalFields(t *testing.T) {
	var form map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		form = r.MultipartForm.Value
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := coverageTestClient(srv.URL, false)
	c.autoStart = false
	require.NoError(t, c.AddTorrentWithPath([]byte("data"), "", "", ""))
	_, hasCat := form["category"]
	_, hasTags := form["tags"]
	_, hasSave := form["savepath"]
	assert.False(t, hasCat)
	assert.False(t, hasTags)
	assert.False(t, hasSave)
	assert.Equal(t, "true", form["paused"][0])
}

func TestQbitConfigGetURL_SchemeAndTrim(t *testing.T) {
	assert.Equal(t, "http://host:8080", (&QBitConfig{URL: "  host:8080/ "}).GetURL())
	assert.Equal(t, "https://q.example", (&QBitConfig{URL: "https://q.example"}).GetURL())
	assert.Equal(t, "", (&QBitConfig{URL: ""}).GetURL())
}

func TestQbitConfigValidate_Branches(t *testing.T) {
	assert.Error(t, (&QBitConfig{URL: ""}).Validate())
	assert.Error(t, (&QBitConfig{URL: "://bad"}).Validate())
	assert.Error(t, (&QBitConfig{URL: "ftp://host"}).Validate())
	assert.Error(t, (&QBitConfig{URL: "http://u:p@host"}).Validate())
	assert.Error(t, (&QBitConfig{URL: "http://host#frag"}).Validate())
	assert.NoError(t, (&QBitConfig{URL: "http://host:8080"}).Validate())
}

func TestQbitEnter_SLoggerNotNil(t *testing.T) {
	assert.NotNil(t, sLogger())
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

func TestQbitSetSpeedLimit_DownloadPostError(t *testing.T) {
	srv := failStatusServer(t, http.StatusInternalServerError)
	err := coverageTestClient(srv.URL, false).SetSpeedLimit(downloader.SpeedLimit{})
	require.Error(t, err)
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

func TestQbitGetClientLabels_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"movies":{"name":"movies"},"tv":{"name":"tv"}}`))
	}))
	defer srv.Close()

	labels, err := coverageTestClient(srv.URL, false).GetClientLabels()
	require.NoError(t, err)
	assert.Len(t, labels, 2)
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

func TestQbitAddTorrentWithPath_Success(t *testing.T) {
	var savePath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		savePath = r.FormValue("savepath")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := coverageTestClient(srv.URL, false)
	c.autoStart = true
	require.NoError(t, c.AddTorrentWithPath([]byte("data"), "cat", "tag", "/custom"))
	assert.Equal(t, "/custom", savePath)
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

func TestQbitProcessTorrentFile_ExistingRemovesLocal_C2(t *testing.T) {
	data := makeSingleFileTorrent(t, 1024)
	hash, err := ComputeTorrentHash(data)
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/torrents/properties" {
			// exists → return 200 with the matching hash props
			_, _ = w.Write([]byte(`{"hash":"` + hash + `"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "x.torrent")
	require.NoError(t, os.WriteFile(path, data, 0o644))

	c := coverageTestClient(srv.URL, false)
	require.NoError(t, c.processTorrentFile(context.Background(), path, "", ""))
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "existing torrent's local file must be removed")
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

func TestQbitAddTorrentEx_LegacySuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Ok."))
	}))
	defer srv.Close()

	res, err := coverageTestClient(srv.URL, false).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{
		SavePath: "/dl", Category: "cat", Tags: "a,b",
	})
	require.NoError(t, err)
	assert.True(t, res.Success)
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

func TestQbitProcessTorrentDirectory_Success(t *testing.T) {
	data := makeSingleFileTorrent(t, 1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/sync/maindata":
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":1099511627776}}`))
		case "/api/v2/torrents/properties":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.torrent"), data, 0o644))

	c := coverageTestClient(srv.URL, false)
	require.NoError(t, c.ProcessTorrentDirectory(context.Background(), dir, "cat", "tag"))
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

func TestQbitAddTorrentFileEx_V520JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success_count":1,"pending_count":0,"failure_count":0,"added_torrent_ids":["zzz"]}`))
	}))
	defer srv.Close()

	res, err := coverageTestClient(srv.URL, true).AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{})
	require.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, "zzz", res.Hash)
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

func TestQbitGetClientVersion_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("v4.6.5"))
	}))
	defer srv.Close()
	v, err := coverageTestClient(srv.URL, false).GetClientVersion()
	require.NoError(t, err)
	assert.Equal(t, "v4.6.5", v)
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

func TestQbitCheckTorrentExists_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"hash":"h1","name":"t"}`))
	}))
	defer srv.Close()
	got, err := coverageTestClient(srv.URL, false).CheckTorrentExists("h1")
	require.NoError(t, err)
	assert.True(t, got)
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

func TestQbitGetTorrentsBy_UnfilteredReturnsAll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"hash":"h1","name":"t1","state":"downloading"}]`))
	}))
	defer srv.Close()
	got, err := coverageTestClient(srv.URL, false).GetTorrentsBy(downloader.TorrentFilter{})
	require.NoError(t, err)
	require.Len(t, got, 1)
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

func TestQbitComputeTorrentSize_SingleAndMulti(t *testing.T) {
	single := makeSingleFileTorrent(t, 4096)
	sz, err := ComputeTorrentSize(single)
	require.NoError(t, err)
	assert.Equal(t, int64(4096), sz)

	multi := makeMultiFileTorrent(t, 100, 200, 300)
	msz, err := ComputeTorrentSize(multi)
	require.NoError(t, err)
	assert.Equal(t, int64(600), msz)

	_, err = ComputeTorrentSize([]byte("not bencode"))
	require.Error(t, err)
}

func TestQbitComputeTorrentHashWithPath(t *testing.T) {
	data := makeSingleFileTorrent(t, 1024)
	dir := t.TempDir()
	p := filepath.Join(dir, "x.torrent")
	require.NoError(t, os.WriteFile(p, data, 0o644))

	h, err := ComputeTorrentHashWithPath(p)
	require.NoError(t, err)
	assert.NotEmpty(t, h)

	_, err = ComputeTorrentHashWithPath(filepath.Join(dir, "missing.torrent"))
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

func TestQbitAddTorrentEx_V520EmptyBodyGenericSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	res, err := coverageTestClient(srv.URL, true).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{})
	require.NoError(t, err)
	assert.True(t, res.Success)
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

func TestQbitProcessSingleTorrentFile_Success(t *testing.T) {
	data := makeSingleFileTorrent(t, 1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/sync/maindata":
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":1099511627776}}`))
		case "/api/v2/torrents/properties":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "x.torrent")
	require.NoError(t, os.WriteFile(path, data, 0o644))

	require.NoError(t, coverageTestClient(srv.URL, false).ProcessSingleTorrentFile(context.Background(), path, "cat", "tag"))
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

func TestQbitAddTorrentEx_LegacyStatusError(t *testing.T) {
	srv := failStatusServer(t, http.StatusInternalServerError)
	res, err := coverageTestClient(srv.URL, false).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{})
	require.Error(t, err)
	assert.False(t, res.Success)
}

func TestQbitAddTorrentEx_V520JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"success_count":1,"pending_count":0,"failure_count":0,"added_torrent_ids":["abc"]}`))
	}))
	defer srv.Close()

	res, err := coverageTestClient(srv.URL, true).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{})
	require.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, "abc", res.Hash)
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

// TestQbitAddTorrentFileEx_AllOptions exercises writeAddTorrentOptions's
// SavePath/Category/Tags/limits/AdvanceOptions branches through a real add.
func TestQbitAddTorrentFileEx_AllOptions(t *testing.T) {
	var form map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		form = r.MultipartForm.Value
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	res, err := coverageTestClient(srv.URL, false).AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{
		SavePath:              "/dl",
		Category:              "movies",
		Tags:                  "hd,x265",
		AddAtPaused:           true,
		UploadSpeedLimitKBs:   100,
		DownloadSpeedLimitKBs: 200,
		AdvanceOptions: map[string]any{
			"sequentialDownload": true,
			"contentLayout":      "Original",
		},
	})
	require.NoError(t, err)
	assert.True(t, res.Success)
	assert.Equal(t, "/dl", form["savepath"][0])
	assert.Equal(t, "movies", form["category"][0])
	assert.Equal(t, "hd,x265", form["tags"][0])
	assert.Contains(t, form, "upLimit")
	assert.Contains(t, form, "dlLimit")
	assert.Equal(t, "Original", form["contentLayout"][0])
}
