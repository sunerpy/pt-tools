package transmission

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// recordedRPCCall captures one RPC invocation for test assertions.
type recordedRPCCall struct {
	Method string
	Args   map[string]any
}

// newRecordingTransmissionServer returns a mock Transmission RPC endpoint that
// records every method call in order. This lets tests verify the expected
// torrent-add → torrent-set → torrent-start sequence that speed limits require.
func newRecordingTransmissionServer(t *testing.T) (*httptest.Server, *sync.Mutex, *[]recordedRPCCall) {
	t.Helper()
	sessionID := "test-session-id"
	mu := &sync.Mutex{}
	calls := &[]recordedRPCCall{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Transmission-Session-Id") != sessionID {
			w.Header().Set("X-Transmission-Session-Id", sessionID)
			w.WriteHeader(http.StatusConflict)
			return
		}

		// 解码为通用 map 以便同时读取 method + arguments（rpcRequest.Arguments 为 any 类型）
		var raw struct {
			Method    string         `json:"method"`
			Arguments map[string]any `json:"arguments"`
			Tag       int            `json:"tag"`
		}
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		mu.Lock()
		*calls = append(*calls, recordedRPCCall{Method: raw.Method, Args: raw.Arguments})
		mu.Unlock()

		resp := rpcResponse{Result: "success"}
		switch raw.Method {
		case "session-get":
			a := map[string]any{"download-dir": "/downloads"}
			resp.Arguments, _ = json.Marshal(a)
		case "torrent-add":
			a := torrentAddResponse{
				TorrentAdded: &torrentInfo{ID: 42, Name: "test", HashString: "hash42"},
			}
			resp.Arguments, _ = json.Marshal(a)
		case "torrent-set", "torrent-start":
			// no arguments needed in response
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	return srv, mu, calls
}

func newTransmissionTestClient(t *testing.T, srvURL string) *TransmissionClient {
	t.Helper()
	cfg := NewTransmissionConfig(srvURL, "", "")
	cli, err := NewTransmissionClient(cfg, "test-tr")
	require.NoError(t, err)
	return cli.(*TransmissionClient)
}

// rpcCallsByMethod filters the recorded calls to a single method.
func rpcCallsByMethod(calls []recordedRPCCall, method string) []recordedRPCCall {
	var out []recordedRPCCall
	for _, c := range calls {
		if c.Method == method {
			out = append(out, c)
		}
	}
	return out
}

// TestTransmissionAddTorrentFileEx_NoLimits verifies that when no speed limits
// are configured, only a single torrent-add call is made (no set/start follow-ups).
// Regression guard: ensure the new speed-limit code path doesn't break the
// simple "add and go" flow for users who don't configure any limits.
func TestTransmissionAddTorrentFileEx_NoLimits(t *testing.T) {
	srv, mu, calls := newRecordingTransmissionServer(t)
	defer srv.Close()
	cli := newTransmissionTestClient(t, srv.URL)

	_, err := cli.AddTorrentFileEx([]byte("fake-torrent"), downloader.AddTorrentOptions{})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	setCalls := rpcCallsByMethod(*calls, "torrent-set")
	startCalls := rpcCallsByMethod(*calls, "torrent-start")
	assert.Empty(t, setCalls, "no torrent-set expected when no limits")
	assert.Empty(t, startCalls, "no torrent-start expected when no limits")
}

// TestTransmissionAddTorrentFileEx_UploadLimit_AutoStart verifies that when
// the user wants auto-start + speed limit:
//  1. torrent-add is called with paused=true (forced, to allow setting limits)
//  2. torrent-set is called with uploadLimit (in KB/s) + uploadLimited=true
//  3. torrent-start is called to honor the original auto-start intent
//
// Regression guard for issue #276 per-site upload limit.
func TestTransmissionAddTorrentFileEx_UploadLimit_AutoStart(t *testing.T) {
	srv, mu, calls := newRecordingTransmissionServer(t)
	defer srv.Close()
	cli := newTransmissionTestClient(t, srv.URL)

	_, err := cli.AddTorrentFileEx([]byte("fake-torrent"), downloader.AddTorrentOptions{
		UploadSpeedLimitKBs: 512,
		AddAtPaused:         false, // user wants auto-start
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()

	addCalls := rpcCallsByMethod(*calls, "torrent-add")
	require.Len(t, addCalls, 1)
	assert.Equal(t, true, addCalls[0].Args["paused"], "torrent-add must force paused=true when limits set")

	setCalls := rpcCallsByMethod(*calls, "torrent-set")
	require.Len(t, setCalls, 1, "torrent-set should be called exactly once")
	// Transmission uses kB/s (integer) — 512 KB input = 512 KB/s
	// The implementation converts: bytes/1024 = KB. Since EffectiveUploadLimitBytes
	// returns 512*1024 = 524288 bytes, dividing by 1024 gives 512.
	assert.EqualValues(t, 512, setCalls[0].Args["uploadLimit"])
	assert.Equal(t, true, setCalls[0].Args["uploadLimited"])

	startCalls := rpcCallsByMethod(*calls, "torrent-start")
	require.Len(t, startCalls, 1, "torrent-start must be called to honor user's auto-start intent")
}

// TestTransmissionAddTorrentFileEx_UploadLimit_KeepsPaused verifies that when
// the user EXPLICITLY wanted paused+limits, no torrent-start is called (user's
// intent is preserved; they will start manually).
func TestTransmissionAddTorrentFileEx_UploadLimit_KeepsPaused(t *testing.T) {
	srv, mu, calls := newRecordingTransmissionServer(t)
	defer srv.Close()
	cli := newTransmissionTestClient(t, srv.URL)

	_, err := cli.AddTorrentFileEx([]byte("fake"), downloader.AddTorrentOptions{
		UploadSpeedLimitKBs: 256,
		AddAtPaused:         true,
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	setCalls := rpcCallsByMethod(*calls, "torrent-set")
	startCalls := rpcCallsByMethod(*calls, "torrent-start")
	assert.Len(t, setCalls, 1)
	assert.Empty(t, startCalls, "no torrent-start when user explicitly wanted paused")
}

// TestTransmissionAddTorrentFileEx_DownloadLimit verifies that download speed
// limit is sent as downloadLimit (kB/s) + downloadLimited=true.
func TestTransmissionAddTorrentFileEx_DownloadLimit(t *testing.T) {
	srv, mu, calls := newRecordingTransmissionServer(t)
	defer srv.Close()
	cli := newTransmissionTestClient(t, srv.URL)

	_, err := cli.AddTorrentFileEx([]byte("fake"), downloader.AddTorrentOptions{
		DownloadSpeedLimitKBs: 2048,
		AddAtPaused:           true,
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	setCalls := rpcCallsByMethod(*calls, "torrent-set")
	require.Len(t, setCalls, 1)
	assert.EqualValues(t, 2048, setCalls[0].Args["downloadLimit"])
	assert.Equal(t, true, setCalls[0].Args["downloadLimited"])
	_, hasUp := setCalls[0].Args["uploadLimit"]
	assert.False(t, hasUp, "uploadLimit must not be set when only download is limited")
}

// TestTransmissionAddTorrentFileEx_BothLimits verifies setting both upload +
// download in a single torrent-set call (single RPC, atomic).
func TestTransmissionAddTorrentFileEx_BothLimits(t *testing.T) {
	srv, mu, calls := newRecordingTransmissionServer(t)
	defer srv.Close()
	cli := newTransmissionTestClient(t, srv.URL)

	_, err := cli.AddTorrentFileEx([]byte("fake"), downloader.AddTorrentOptions{
		UploadSpeedLimitKBs:   100,
		DownloadSpeedLimitKBs: 500,
		AddAtPaused:           true,
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	setCalls := rpcCallsByMethod(*calls, "torrent-set")
	require.Len(t, setCalls, 1, "must be a single torrent-set call carrying both limits")
	assert.EqualValues(t, 100, setCalls[0].Args["uploadLimit"])
	assert.EqualValues(t, 500, setCalls[0].Args["downloadLimit"])
	assert.Equal(t, true, setCalls[0].Args["uploadLimited"])
	assert.Equal(t, true, setCalls[0].Args["downloadLimited"])
}

// TestTransmissionAddTorrentFileEx_LegacyMBFieldBackwardCompat ensures the
// deprecated UploadSpeedLimitMB still works via the EffectiveUploadLimitBytes
// helper. Backward-compat guard: existing code using the MB field must continue
// to apply the limit after v0.26.
func TestTransmissionAddTorrentFileEx_LegacyMBFieldBackwardCompat(t *testing.T) {
	srv, mu, calls := newRecordingTransmissionServer(t)
	defer srv.Close()
	cli := newTransmissionTestClient(t, srv.URL)

	_, err := cli.AddTorrentFileEx([]byte("fake"), downloader.AddTorrentOptions{
		UploadSpeedLimitMB: 3, // 3 MB/s = 3072 KB/s
		AddAtPaused:        true,
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	setCalls := rpcCallsByMethod(*calls, "torrent-set")
	require.Len(t, setCalls, 1)
	assert.EqualValues(t, 3072, setCalls[0].Args["uploadLimit"])
}

// TestTransmissionAddTorrentFileEx_CallOrdering verifies the exact call
// sequence torrent-add → torrent-set → torrent-start. This ordering is
// important: calling set before add would fail, and calling start before set
// would race the limit application.
func TestTransmissionAddTorrentFileEx_CallOrdering(t *testing.T) {
	srv, mu, calls := newRecordingTransmissionServer(t)
	defer srv.Close()
	cli := newTransmissionTestClient(t, srv.URL)

	_, err := cli.AddTorrentFileEx([]byte("fake"), downloader.AddTorrentOptions{
		UploadSpeedLimitKBs: 100,
		AddAtPaused:         false,
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	var rpcMethods []string
	for _, c := range *calls {
		if c.Method == "torrent-add" || c.Method == "torrent-set" || c.Method == "torrent-start" {
			rpcMethods = append(rpcMethods, c.Method)
		}
	}
	assert.Equal(t, []string{"torrent-add", "torrent-set", "torrent-start"}, rpcMethods,
		"calls must be add → set → start in order")
}

// TestTransmissionAddTorrentFileEx_NegativeLimitIgnored verifies that negative
// values are treated as unset.
func TestTransmissionAddTorrentFileEx_NegativeLimitIgnored(t *testing.T) {
	srv, mu, calls := newRecordingTransmissionServer(t)
	defer srv.Close()
	cli := newTransmissionTestClient(t, srv.URL)

	_, err := cli.AddTorrentFileEx([]byte("fake"), downloader.AddTorrentOptions{
		UploadSpeedLimitKBs: -5,
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	setCalls := rpcCallsByMethod(*calls, "torrent-set")
	assert.Empty(t, setCalls, "negative limit must be treated as unset")
}
