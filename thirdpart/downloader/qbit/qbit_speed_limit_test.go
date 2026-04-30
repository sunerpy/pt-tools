package qbit

import (
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// capturedAddRequest is the decoded multipart form submitted to
// /api/v2/torrents/add, captured for assertion.
type capturedAddRequest struct {
	Fields map[string]string
}

// newCapturingQbitServer returns an httptest server that captures the most
// recent /api/v2/torrents/add request for inspection.
func newCapturingQbitServer(t *testing.T) (*httptest.Server, *sync.Mutex, *capturedAddRequest) {
	t.Helper()
	mu := &sync.Mutex{}
	captured := &capturedAddRequest{Fields: map[string]string{}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/sync/maindata":
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":1073741824}}`))
		case "/api/v2/torrents/properties":
			w.WriteHeader(http.StatusNotFound)
		case "/api/v2/torrents/add":
			ct := r.Header.Get("Content-Type")
			_, params, _ := mime.ParseMediaType(ct)
			reader := multipart.NewReader(r.Body, params["boundary"])
			mu.Lock()
			captured.Fields = map[string]string{}
			for {
				part, err := reader.NextPart()
				if err != nil {
					break
				}
				name := part.FormName()
				if name == "torrents" {
					_ = part.Close()
					continue
				}
				data, _ := io.ReadAll(part)
				captured.Fields[name] = string(data)
				_ = part.Close()
			}
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	return srv, mu, captured
}

func newQbitTestClient(t *testing.T, srvURL string) *QbitClient {
	t.Helper()
	config := NewQBitConfig(srvURL, "admin", "pwd")
	cli, err := NewQbitClient(config, "test-qbit")
	require.NoError(t, err)
	return cli.(*QbitClient)
}

// minimal valid torrent bytes for AddTorrentFileEx (the qbit mock accepts any).
func fixtureTorrentBytes() []byte {
	return []byte("d8:announce35:http://tracker.example.com/announce4:infod6:lengthi1024e4:name8:test.txt12:piece lengthi16384e6:pieces20:01234567890123456789ee")
}

// TestQbitAddTorrentFileEx_UploadLimitKBs verifies that UploadSpeedLimitKBs
// is correctly converted to bytes/second and sent as `upLimit` multipart field.
// Regression guard for issue #276 per-site upload limit.
func TestQbitAddTorrentFileEx_UploadLimitKBs(t *testing.T) {
	srv, mu, captured := newCapturingQbitServer(t)
	defer srv.Close()
	cli := newQbitTestClient(t, srv.URL)
	defer cli.Close()

	_, err := cli.AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{
		UploadSpeedLimitKBs: 500,
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "512000", captured.Fields["upLimit"], "500 KB/s should be sent as 512000 bytes/s")
	_, hasDl := captured.Fields["dlLimit"]
	assert.False(t, hasDl, "dlLimit must not be set when only upload is limited")
}

// TestQbitAddTorrentFileEx_DownloadLimitKBs verifies that DownloadSpeedLimitKBs
// is sent as `dlLimit` in bytes/second. Regression guard for issue #276.
func TestQbitAddTorrentFileEx_DownloadLimitKBs(t *testing.T) {
	srv, mu, captured := newCapturingQbitServer(t)
	defer srv.Close()
	cli := newQbitTestClient(t, srv.URL)
	defer cli.Close()

	_, err := cli.AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{
		DownloadSpeedLimitKBs: 2048,
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "2097152", captured.Fields["dlLimit"], "2048 KB/s should be 2097152 bytes/s")
	_, hasUp := captured.Fields["upLimit"]
	assert.False(t, hasUp)
}

// TestQbitAddTorrentFileEx_BothLimits verifies both limits set simultaneously.
func TestQbitAddTorrentFileEx_BothLimits(t *testing.T) {
	srv, mu, captured := newCapturingQbitServer(t)
	defer srv.Close()
	cli := newQbitTestClient(t, srv.URL)
	defer cli.Close()

	_, err := cli.AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{
		UploadSpeedLimitKBs:   1000,
		DownloadSpeedLimitKBs: 4000,
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "1024000", captured.Fields["upLimit"])
	assert.Equal(t, "4096000", captured.Fields["dlLimit"])
}

// TestQbitAddTorrentFileEx_NoLimits verifies no speed-limit fields are sent
// when the options are at zero value (preserves backward compatibility —
// previous callers never set these fields and must not get unlimited→0 surprises).
func TestQbitAddTorrentFileEx_NoLimits(t *testing.T) {
	srv, mu, captured := newCapturingQbitServer(t)
	defer srv.Close()
	cli := newQbitTestClient(t, srv.URL)
	defer cli.Close()

	_, err := cli.AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	_, hasUp := captured.Fields["upLimit"]
	_, hasDl := captured.Fields["dlLimit"]
	assert.False(t, hasUp, "upLimit must not be sent when limit=0")
	assert.False(t, hasDl, "dlLimit must not be sent when limit=0")
}

// TestQbitAddTorrentFileEx_LegacyMBFieldBackwardCompat ensures the deprecated
// UploadSpeedLimitMB still works (converted via EffectiveUploadLimitBytes).
// Backward-compat guard: existing integrations using the MB field (before v0.26)
// must continue working.
func TestQbitAddTorrentFileEx_LegacyMBFieldBackwardCompat(t *testing.T) {
	srv, mu, captured := newCapturingQbitServer(t)
	defer srv.Close()
	cli := newQbitTestClient(t, srv.URL)
	defer cli.Close()

	_, err := cli.AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{
		UploadSpeedLimitMB: 3,
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "3145728", captured.Fields["upLimit"], "3 MB/s should be 3*1024*1024 bytes/s")
}

// TestQbitAddTorrentFileEx_KBsTakesPriorityOverMB verifies that when both fields
// are set, the new KBs field wins. Guards against a regression where the old MB
// field accidentally overrides the finer-grained new field.
func TestQbitAddTorrentFileEx_KBsTakesPriorityOverMB(t *testing.T) {
	srv, mu, captured := newCapturingQbitServer(t)
	defer srv.Close()
	cli := newQbitTestClient(t, srv.URL)
	defer cli.Close()

	_, err := cli.AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{
		UploadSpeedLimitMB:  5,   // 5 MB = 5120 KB = 5242880 bytes
		UploadSpeedLimitKBs: 100, // 100 KB = 102400 bytes
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "102400", captured.Fields["upLimit"], "KBs must win over MB")
	assert.NotEqual(t, "5242880", captured.Fields["upLimit"])
}

// TestQbitAddTorrentFileEx_NegativeLimitIgnored verifies that negative values
// are treated as unset (no field sent), preventing accidental 负数 bug.
func TestQbitAddTorrentFileEx_NegativeLimitIgnored(t *testing.T) {
	srv, mu, captured := newCapturingQbitServer(t)
	defer srv.Close()
	cli := newQbitTestClient(t, srv.URL)
	defer cli.Close()

	_, err := cli.AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{
		UploadSpeedLimitKBs:   -100,
		DownloadSpeedLimitKBs: -50,
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	_, hasUp := captured.Fields["upLimit"]
	_, hasDl := captured.Fields["dlLimit"]
	assert.False(t, hasUp)
	assert.False(t, hasDl)
}

// TestQbitAddTorrentFileEx_OtherFieldsUnaffected verifies that adding speed
// limits does not break other existing multipart fields (category, tags,
// savepath, skip_checking). Regression guard against accidental field removal.
func TestQbitAddTorrentFileEx_OtherFieldsUnaffected(t *testing.T) {
	srv, mu, captured := newCapturingQbitServer(t)
	defer srv.Close()
	cli := newQbitTestClient(t, srv.URL)
	defer cli.Close()

	_, err := cli.AddTorrentFileEx(fixtureTorrentBytes(), downloader.AddTorrentOptions{
		SavePath:            "/downloads/pt",
		Category:            "movie",
		Tags:                "hd,movie",
		UploadSpeedLimitKBs: 500,
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "/downloads/pt", captured.Fields["savepath"])
	assert.Equal(t, "movie", captured.Fields["category"])
	assert.True(t, strings.Contains(captured.Fields["tags"], "hd"))
	assert.Equal(t, "512000", captured.Fields["upLimit"])
}
