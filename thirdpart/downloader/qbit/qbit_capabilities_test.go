package qbit

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeebo/bencode"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func testTorrentFile(t *testing.T) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, bencode.NewEncoder(&buf).Encode(map[string]any{
		"announce": "https://tracker.example.org/announce.php?passkey=x",
		"info":     map[string]any{"name": "a", "length": 10, "piece length": 16384, "pieces": "01234567890123456789"},
	}))
	h, err := ComputeTorrentHash(buf.Bytes())
	require.NoError(t, err)
	return buf.Bytes(), h
}

func TestQbitExportTorrent(t *testing.T) {
	data, hash := testTorrentFile(t)
	var gotHash string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v2/torrents/export", r.URL.Path)
		gotHash = r.URL.Query().Get("hash")
		switch gotHash {
		case hash:
			w.Header().Set("Content-Type", "application/x-bittorrent")
			_, _ = w.Write(data)
		case "other":
			_, _ = w.Write(data) // 拿到的不是要的那个种子
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := coverageTestClient(srv.URL, false)
	c.appVersion = "v4.6.2"
	got, err := c.ExportTorrent(context.Background(), hash)
	require.NoError(t, err)
	assert.Equal(t, data, got)
	assert.Equal(t, hash, gotHash)

	_, err = c.ExportTorrent(context.Background(), "other")
	assert.ErrorContains(t, err, "与请求的种子不符")
	_, err = c.ExportTorrent(context.Background(), "missing")
	assert.ErrorIs(t, err, downloader.ErrTorrentNotFound)

	old := coverageTestClient(srv.URL, false)
	old.appVersion = "v4.4.5"
	_, err = old.ExportTorrent(context.Background(), hash)
	assert.ErrorIs(t, err, downloader.ErrCapabilityUnsupported, "4.5 以前没有 torrents/export")
}

func TestQbitEditTracker(t *testing.T) {
	var form map[string]string
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v2/torrents/editTracker", r.URL.Path)
		require.NoError(t, r.ParseForm())
		form = map[string]string{"hash": r.Form.Get("hash"), "origUrl": r.Form.Get("origUrl"), "newUrl": r.Form.Get("newUrl")}
		w.WriteHeader(status)
	}))
	defer srv.Close()

	c := coverageTestClient(srv.URL, false)
	require.NoError(t, c.EditTracker(context.Background(), "abc", "https://old/announce", "https://new/announce"))
	assert.Equal(t, map[string]string{"hash": "abc", "origUrl": "https://old/announce", "newUrl": "https://new/announce"}, form)

	status = http.StatusConflict
	err := c.EditTracker(context.Background(), "abc", "https://old/announce", "https://new/announce")
	assert.ErrorIs(t, err, downloader.ErrTrackerNotFound)

	assert.Error(t, c.EditTracker(context.Background(), "abc", "", "https://new/announce"), "地址不能为空")
}

func TestQbitGetTorrentTrackersContext(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("hash") == "slow" {
			<-block
			return
		}
		_, _ = w.Write([]byte(`[{"url":"https://t.example/announce","status":4,"msg":"Unregistered torrent"}]`))
	}))
	defer srv.Close()
	defer close(block)

	c := coverageTestClient(srv.URL, false)
	var r downloader.TrackerReader = c
	trs, err := r.GetTorrentTrackersContext(context.Background(), "abc")
	require.NoError(t, err)
	require.Len(t, trs, 1)
	assert.Equal(t, "Unregistered torrent", trs[0].Message)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = c.GetTorrentTrackersContext(ctx, "slow")
	assert.ErrorIs(t, err, context.DeadlineExceeded, "ctx 到期时底层请求一起取消")
}
