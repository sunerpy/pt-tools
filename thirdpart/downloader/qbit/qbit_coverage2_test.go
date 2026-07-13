package qbit

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

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

func TestQbitAddTorrentWithPath(t *testing.T) {
	srv := qbitAddServer(t, false)
	defer srv.Close()
	c := coverageTestClient(srv.URL, false)
	c.autoStart = true

	data := makeSingleFileTorrent(t, 1000)
	require.NoError(t, c.AddTorrentWithPath(data, "movies", "hd", "/custom/path"))
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

func TestQbitAddTorrentFileEx_WithAdvanceOptions(t *testing.T) {
	srv, mu, captured := newCapturingQbitServer(t)
	defer srv.Close()
	c := coverageTestClient(srv.URL, false)

	res, err := c.AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{
		SavePath:    "/dl",
		Category:    "cat",
		Tags:        "tag",
		AddAtPaused: true,
		AdvanceOptions: map[string]any{
			"sequentialDownload": true,
			"contentLayout":      "Original",
			"ignoredFlag":        false, // false bool skipped
		},
	})
	require.NoError(t, err)
	assert.True(t, res.Success)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "/dl", captured.Fields["savepath"])
	assert.Equal(t, "cat", captured.Fields["category"])
	assert.Equal(t, "tag", captured.Fields["tags"])
	assert.Equal(t, "true", captured.Fields["paused"])
	assert.Equal(t, "true", captured.Fields["sequentialDownload"])
	assert.Equal(t, "Original", captured.Fields["contentLayout"])
	_, hasIgnored := captured.Fields["ignoredFlag"]
	assert.False(t, hasIgnored)
}

func TestQbitProcessTorrentFile_NewTorrent(t *testing.T) {
	srv := qbitAddServer(t, false)
	defer srv.Close()
	c := coverageTestClient(srv.URL, false)

	dir := t.TempDir()
	path := filepath.Join(dir, "a.torrent")
	require.NoError(t, os.WriteFile(path, makeSingleFileTorrent(t, 1000), 0o644))

	require.NoError(t, c.ProcessSingleTorrentFile(t.Context(), path, "cat", "tag"))
}

func TestQbitProcessTorrentFile_ExistingRemovesLocal(t *testing.T) {
	srv := qbitAddServer(t, true)
	defer srv.Close()
	c := coverageTestClient(srv.URL, false)

	dir := t.TempDir()
	path := filepath.Join(dir, "b.torrent")
	require.NoError(t, os.WriteFile(path, makeSingleFileTorrent(t, 1000), 0o644))

	require.NoError(t, c.processTorrentFile(t.Context(), path, "", ""))
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "existing torrent should delete local file")
}

func TestQbitProcessTorrentFile_ReadError(t *testing.T) {
	c := coverageTestClient("http://unused", false)
	err := c.processTorrentFile(t.Context(), "/nonexistent/x.torrent", "", "")
	require.Error(t, err)
}

func TestQbitProcessTorrentFile_InvalidTorrent(t *testing.T) {
	c := coverageTestClient("http://unused", false)
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.torrent")
	require.NoError(t, os.WriteFile(path, []byte("not bencode"), 0o644))

	err := c.processTorrentFile(t.Context(), path, "", "")
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
