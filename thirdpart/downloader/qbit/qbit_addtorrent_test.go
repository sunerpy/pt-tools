package qbit

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

func TestQbitAddTorrentEx_V520EmptyBodyGenericSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	res, err := coverageTestClient(srv.URL, true).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{})
	require.NoError(t, err)
	assert.True(t, res.Success)
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

func TestQbitAddTorrentEx(t *testing.T) {
	t.Run("legacy success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v2/torrents/add", r.URL.Path)
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		res, err := coverageTestClient(srv.URL, false).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{})
		require.NoError(t, err)
		assert.True(t, res.Success)
	})

	t.Run("legacy error status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		res, err := coverageTestClient(srv.URL, false).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{})
		require.Error(t, err)
		assert.False(t, res.Success)
	})

	t.Run("v520 json result", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success_count":     1,
				"failure_count":     0,
				"added_torrent_ids": []string{"newhash"},
			})
		}))
		defer srv.Close()

		res, err := coverageTestClient(srv.URL, true).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{
			SavePath: "/dl", Category: "cat", Tags: "t", AddAtPaused: true,
		})
		require.NoError(t, err)
		assert.True(t, res.Success)
		assert.Equal(t, "newhash", res.Hash)
	})

	t.Run("v520 conflict", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
		}))
		defer srv.Close()

		res, err := coverageTestClient(srv.URL, true).AddTorrentEx("magnet:?x", downloader.AddTorrentOptions{})
		require.NoError(t, err)
		assert.False(t, res.Success)
	})
}

func TestQbitAddTorrentWithPath(t *testing.T) {
	srv := qbitAddServer(t, false)
	defer srv.Close()
	c := coverageTestClient(srv.URL, false)
	c.autoStart = true

	data := makeSingleFileTorrent(t, 1000)
	require.NoError(t, c.AddTorrentWithPath(data, "movies", "hd", "/custom/path"))
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

func TestQbitProcessTorrentFile_InvalidTorrent(t *testing.T) {
	c := coverageTestClient("http://unused", false)
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.torrent")
	require.NoError(t, os.WriteFile(path, []byte("not bencode"), 0o644))

	err := c.processTorrentFile(t.Context(), path, "", "")
	require.Error(t, err)
}
