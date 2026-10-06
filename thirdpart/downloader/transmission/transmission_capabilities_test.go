package transmission

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// fakeTrackerRPC 模拟 torrent-get 返回的 tracker 字段，记录 torrent-set 的参数。
type fakeTrackerRPC struct {
	torrent map[string]any
	sets    []map[string]any
}

func (f *fakeTrackerRPC) server(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Transmission-Session-Id") != "sid" {
			w.Header().Set("X-Transmission-Session-Id", "sid")
			w.WriteHeader(http.StatusConflict)
			return
		}
		var req struct {
			Method    string         `json:"method"`
			Arguments map[string]any `json:"arguments"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		resp := map[string]any{"result": "success", "arguments": map[string]any{}}
		switch req.Method {
		case "torrent-get":
			torrents := []any{}
			if f.torrent != nil {
				torrents = append(torrents, f.torrent)
			}
			resp["arguments"] = map[string]any{"torrents": torrents}
		case "torrent-set":
			f.sets = append(f.sets, req.Arguments)
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func newCapabilityClient(t *testing.T, url string) *TransmissionClient {
	t.Helper()
	dl, err := NewTransmissionClient(NewTransmissionConfig(url, "", ""), "tr")
	require.NoError(t, err)
	return dl.(*TransmissionClient)
}

// Transmission 4.0 起用 trackerList：整份列表替换，分层（空行分隔）保持不变。
func TestTransmissionEditTracker_TrackerList(t *testing.T) {
	f := &fakeTrackerRPC{torrent: map[string]any{
		"id": 7, "hashString": "abc",
		"trackerList": "https://a/announce\n\nhttps://old/announce?passkey=1\n",
		"trackers":    []any{map[string]any{"id": 0, "announce": "https://a/announce"}, map[string]any{"id": 1, "announce": "https://old/announce?passkey=1"}},
	}}
	srv := f.server(t)
	defer srv.Close()
	c := newCapabilityClient(t, srv.URL)

	require.NoError(t, c.EditTracker(context.Background(), "abc", "https://old/announce?passkey=1", "https://new/announce?passkey=1"))
	require.Len(t, f.sets, 1)
	assert.Equal(t, "https://a/announce\n\nhttps://new/announce?passkey=1", f.sets[0]["trackerList"])
	assert.Equal(t, []any{float64(7)}, f.sets[0]["ids"])

	err := c.EditTracker(context.Background(), "abc", "https://missing/announce", "https://new/announce")
	assert.ErrorIs(t, err, downloader.ErrTrackerNotFound)
	assert.Len(t, f.sets, 1, "找不到原地址时不改")
}

// Transmission 3.x 没有 trackerList：用 trackerReplace（tracker id + 新地址）。
func TestTransmissionEditTracker_LegacyReplace(t *testing.T) {
	f := &fakeTrackerRPC{torrent: map[string]any{
		"id": 3, "hashString": "abc",
		"trackers": []any{map[string]any{"id": 5, "announce": "https://old/announce"}},
	}}
	srv := f.server(t)
	defer srv.Close()
	c := newCapabilityClient(t, srv.URL)

	require.NoError(t, c.EditTracker(context.Background(), "abc", "https://old/announce", "https://new/announce"))
	require.Len(t, f.sets, 1)
	assert.Equal(t, []any{float64(5), "https://new/announce"}, f.sets[0]["trackerReplace"])

	f.torrent = nil
	err := c.EditTracker(context.Background(), "abc", "https://old/announce", "https://new/announce")
	assert.ErrorIs(t, err, downloader.ErrTorrentNotFound)
}

func TestTransmissionTrackerReaders(t *testing.T) {
	stats := []any{map[string]any{"announce": "https://t.example/announce", "lastAnnounceResult": "Unregistered torrent", "lastAnnounceSucceeded": false}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Transmission-Session-Id") != "sid" {
			w.Header().Set("X-Transmission-Session-Id", "sid")
			w.WriteHeader(http.StatusConflict)
			return
		}
		var req struct {
			Method    string         `json:"method"`
			Arguments map[string]any `json:"arguments"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		args := map[string]any{}
		if req.Method == "torrent-get" {
			args["torrents"] = []any{
				map[string]any{"id": 1, "hashString": "AAAA", "trackerStats": stats},
				map[string]any{"id": 2, "hashString": "bbbb", "trackerStats": []any{}},
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": "success", "arguments": args})
	}))
	defer srv.Close()
	c := newCapabilityClient(t, srv.URL)

	var bulk downloader.BulkTrackerReader = c
	all, err := bulk.GetAllTorrentTrackers(context.Background())
	require.NoError(t, err)
	require.Len(t, all, 2)
	require.Len(t, all["aaaa"], 1, "键是小写 hash")
	assert.Equal(t, 4, all["aaaa"][0].Status)
	assert.Equal(t, "Unregistered torrent", all["aaaa"][0].Message)

	var one downloader.TrackerReader = c
	trs, err := one.GetTorrentTrackersContext(context.Background(), "1")
	require.NoError(t, err)
	assert.NotEmpty(t, trs)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.GetAllTorrentTrackers(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

// 修改 tracker 的两次 RPC 都随 ctx 取消：下载器不响应时不会一直占着请求。
func TestTransmissionEditTracker_HonorsContext(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Transmission-Session-Id") != "sid" {
			w.Header().Set("X-Transmission-Session-Id", "sid")
			w.WriteHeader(http.StatusConflict)
			return
		}
		var req struct {
			Method string `json:"method"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		if req.Method != "torrent-get" {
			_ = json.NewEncoder(w).Encode(map[string]any{"result": "success", "arguments": map[string]any{}})
			return
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	defer close(release)
	c := newCapabilityClient(t, srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := c.EditTracker(ctx, "abc", "https://old/announce", "https://new/announce")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 3*time.Second)
}
