package web

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingSite 记下同时在下载的请求数的峰值。
type countingSite struct {
	plainV2Site
	inflight atomic.Int32
	peak     atomic.Int32
	calls    atomic.Int32
}

func (c *countingSite) Download(context.Context, string) ([]byte, error) {
	c.calls.Add(1)
	n := c.inflight.Add(1)
	defer c.inflight.Add(-1)
	for {
		p := c.peak.Load()
		if n <= p || c.peak.CompareAndSwap(p, n) {
			break
		}
	}
	time.Sleep(5 * time.Millisecond) // 让请求有重叠，才看得出同时有几个
	return []byte("d"), nil
}

func batchBody(t *testing.T, items []BatchDownloadItem) *bytes.Reader {
	t.Helper()
	body, err := json.Marshal(BatchDownloadRequest{Torrents: items})
	require.NoError(t, err)
	return bytes.NewReader(body)
}

func tarEntries(t *testing.T, body []byte) []string {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(body))
	require.NoError(t, err)
	tr := tar.NewReader(gz)
	var names []string
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return names
		}
		require.NoError(t, err)
		names = append(names, h.Name)
	}
}

// 一次打包的条目数有上限：原来不设限，提交几千个种子 ID 就同时打出几千个站点请求。
func TestApiBatchTorrentDownload_RejectsTooManyItems(t *testing.T) {
	site := &countingSite{plainV2Site: plainV2Site{id: "hdsky"}}
	withOrchestrator(t, site)
	items := make([]BatchDownloadItem, 201)
	for i := range items {
		items[i] = BatchDownloadItem{SiteID: "hdsky", TorrentID: "1"}
	}

	w := httptest.NewRecorder()
	(&Server{}).apiBatchTorrentDownload(w, httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-download", batchBody(t, items)))

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Zero(t, site.calls.Load(), "超限时一个都不下载")
}

// 请求体有上限。
func TestApiBatchTorrentDownload_RejectsOversizedBody(t *testing.T) {
	site := &countingSite{plainV2Site: plainV2Site{id: "hdsky"}}
	withOrchestrator(t, site)
	items := []BatchDownloadItem{{SiteID: "hdsky", TorrentID: "1", Title: strings.Repeat("x", 2<<20)}}

	w := httptest.NewRecorder()
	(&Server{}).apiBatchTorrentDownload(w, httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-download", batchBody(t, items)))

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Zero(t, site.calls.Load())
}

// 同时下载的数量固定：原来每项一个 goroutine，全部同时发出。
func TestApiBatchTorrentDownload_BoundedConcurrency(t *testing.T) {
	site := &countingSite{plainV2Site: plainV2Site{id: "hdsky"}}
	withOrchestrator(t, site)
	items := make([]BatchDownloadItem, 64)
	for i := range items {
		items[i] = BatchDownloadItem{SiteID: "hdsky", TorrentID: "1"}
	}

	w := httptest.NewRecorder()
	(&Server{}).apiBatchTorrentDownload(w, httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-download", batchBody(t, items)))

	require.Equal(t, http.StatusOK, w.Code)
	assert.EqualValues(t, 64, site.calls.Load())
	assert.LessOrEqual(t, site.peak.Load(), int32(8))
}

// 打包总字节数有上限：超出的条目不进归档，其余照常打包。
func TestApiBatchTorrentDownload_TotalBytesCap(t *testing.T) {
	prev := batchDownloadMaxBytes
	batchDownloadMaxBytes = 5
	t.Cleanup(func() { batchDownloadMaxBytes = prev })
	withOrchestrator(t, &plainV2Site{id: "hdsky", data: []byte("abc")})

	w := httptest.NewRecorder()
	(&Server{}).apiBatchTorrentDownload(w, httptest.NewRequest(http.MethodPost, "/api/v2/torrents/batch-download", batchBody(t, []BatchDownloadItem{
		{SiteID: "hdsky", TorrentID: "1"},
		{SiteID: "hdsky", TorrentID: "2"},
	})))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, tarEntries(t, w.Body.Bytes()), 1, "第二个 3 字节的种子超出 5 字节上限")
}
