package qbit

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sunerpy/pt-tools/global"
	"github.com/zeebo/bencode"
	"go.uber.org/zap"
)

type rb struct{ b *bytes.Buffer }

func nopBody(s string) *rb               { return &rb{bytes.NewBufferString(s)} }
func (r *rb) Read(p []byte) (int, error) { return r.b.Read(p) }
func (r *rb) Close() error               { return nil }
func TestComputeTorrentHash_ErrorOnInvalid(t *testing.T) {
	if _, err := ComputeTorrentHash([]byte("not-bencode")); err == nil {
		t.Fatalf("expected error for invalid bencode")
	}
}

func TestComputeTorrentHashWithPath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "dummy.torrent")
	var buf bytes.Buffer
	torrent := map[string]interface{}{"info": map[string]interface{}{"name": "abc"}}
	require.NoError(t, bencode.NewEncoder(&buf).Encode(torrent))
	data := buf.Bytes()
	if _, err := ComputeTorrentHashWithPath(filepath.Join(dir, "missing.torrent")); err == nil {
		t.Fatalf("expected error for missing file")
	}
	require.NoError(t, os.WriteFile(p, data, 0o644))
	_, err := ComputeTorrentHashWithPath(p)
	require.NoError(t, err)
}

type transportLoginOK struct{}

func (t *transportLoginOK) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/api/v2/auth/login" {
		return &http.Response{StatusCode: http.StatusOK, Body: nopBody("Ok."), Header: make(http.Header)}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
}

func TestNewQbitClient_AuthOK(t *testing.T) {
	global.InitLogger(zap.NewNop())
	c := &http.Client{Transport: &transportLoginOK{}}
	q := &QbitClient{BaseURL: "http://example", Username: "u", Password: "p", Client: c}
	require.NoError(t, q.authenticate())
}

func TestNewQbitClient_AuthFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Bad"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := &http.Client{}
	q := &QbitClient{BaseURL: srv.URL, Username: "u", Password: "p", Client: c}
	err := q.authenticate()
	require.Error(t, err)
}

func TestDoRequestWithRetry_403WithoutBody(t *testing.T) {
	loginOK := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			loginOK = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
			return
		}
		if !loginOK {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	c, err := NewQbitClient(srv.URL, "u", "p", time.Second)
	require.NoError(t, err)
	req, _ := http.NewRequest("GET", srv.URL+"/api/v2/sync/maindata", nil)
	resp, err := c.DoRequestWithRetry(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

type transportRetry403 struct{}

func (t *transportRetry403) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/api/v2/auth/login" {
		return &http.Response{StatusCode: http.StatusOK, Body: nopBody("Ok."), Header: make(http.Header)}, nil
	}
	if req.URL.Path == "/api/v2/torrents/properties" {
		return &http.Response{StatusCode: http.StatusForbidden, Body: http.NoBody, Header: make(http.Header)}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
}

func TestDoRequestWithRetry_403_NoBody(t *testing.T) {
	global.InitLogger(zap.NewNop())
	c := &QbitClient{Client: &http.Client{Transport: &transportRetry403{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer c.RateLimiter.Stop()
	req, _ := http.NewRequest("GET", c.BaseURL+"/api/v2/torrents/properties", nil)
	resp, err := c.DoRequestWithRetry(req)
	require.NoError(t, err)
	require.NotNil(t, resp)
}

type transportRetry403WithBody struct{}

func (t *transportRetry403WithBody) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/api/v2/auth/login" {
		return &http.Response{StatusCode: http.StatusOK, Body: nopBody("Ok."), Header: make(http.Header)}, nil
	}
	if req.URL.Path == "/api/v2/torrents/add" {
		return &http.Response{StatusCode: http.StatusForbidden, Body: http.NoBody, Header: make(http.Header)}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
}

func TestDoRequestWithRetry_403_WithBody(t *testing.T) {
	global.InitLogger(zap.NewNop())
	c := &QbitClient{Client: &http.Client{Transport: &transportRetry403WithBody{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer c.RateLimiter.Stop()
	body := bytes.NewBufferString("data")
	req, _ := http.NewRequest("POST", c.BaseURL+"/api/v2/torrents/add", body)
	resp, err := c.DoRequestWithRetry(req)
	require.Error(t, err)
	require.Nil(t, resp)
}

type transportProps struct{}

func (t *transportProps) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.Path {
	case "/api/v2/torrents/properties":
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	case "/api/v2/auth/login":
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	default:
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	}
}

func TestCheckTorrentExists_404(t *testing.T) {
	global.InitLogger(zap.NewNop())
	c := &QbitClient{Client: &http.Client{Transport: &transportProps404{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer c.RateLimiter.Stop()
	ok, err := c.CheckTorrentExists("abc")
	require.NoError(t, err)
	require.False(t, ok)
}

type transportProps404 struct{}

func (t *transportProps404) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/api/v2/torrents/properties" {
		return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: make(http.Header)}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
}

type transportProps200AddOK struct{}

func (t *transportProps200AddOK) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.Path {
	case "/api/v2/auth/login":
		return &http.Response{StatusCode: http.StatusOK, Body: nopBody("Ok."), Header: make(http.Header)}, nil
	case "/api/v2/sync/maindata":
		return &http.Response{StatusCode: http.StatusOK, Body: nopBody(`{"server_state":{"free_space_on_disk":10000000}}`), Header: make(http.Header)}, nil
	case "/api/v2/torrents/properties":
		return &http.Response{StatusCode: http.StatusOK, Body: nopBody(`{"save_path":"/tmp"}`), Header: make(http.Header)}, nil
	case "/api/v2/torrents/add":
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
}

type transportProps404AddOK struct{}

func (t *transportProps404AddOK) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.Path {
	case "/api/v2/auth/login":
		return &http.Response{StatusCode: http.StatusOK, Body: nopBody("Ok."), Header: make(http.Header)}, nil
	case "/api/v2/sync/maindata":
		return &http.Response{StatusCode: http.StatusOK, Body: nopBody(`{"server_state":{"free_space_on_disk":10000000}}`), Header: make(http.Header)}, nil
	case "/api/v2/torrents/properties":
		return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: make(http.Header)}, nil
	case "/api/v2/torrents/add":
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
}

func TestQbitClient_ProcessSingleTorrentFile_Flows(t *testing.T) {
	c := &QbitClient{Client: &http.Client{Transport: &transportProps200AddOK{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer c.RateLimiter.Stop()
	var buf bytes.Buffer
	_ = bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": "abc"}})
	p := filepath.Join(t.TempDir(), "x.torrent")
	require.NoError(t, os.WriteFile(p, buf.Bytes(), 0o644))
	require.NoError(t, c.ProcessSingleTorrentFile(context.Background(), p, "cat", "tag"))
	if _, err := os.Stat(p); err == nil {
		t.Fatalf("expected deleted for exists path")
	}
	c2 := &QbitClient{Client: &http.Client{Transport: &transportProps404AddOK{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer c2.RateLimiter.Stop()
	p2 := filepath.Join(t.TempDir(), "y.torrent")
	require.NoError(t, os.WriteFile(p2, buf.Bytes(), 0o644))
	require.NoError(t, c2.ProcessSingleTorrentFile(context.Background(), p2, "cat", "tag"))
	if _, err := os.Stat(p2); err != nil {
		t.Fatalf("expected file kept after add: %v", err)
	}
}

func TestQbitClient_WaitAndAddTorrentSmart(t *testing.T) {
	c := &QbitClient{Client: &http.Client{Transport: &transportProps404AddOK{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer c.RateLimiter.Stop()
	data := []byte("d4:infoe")
	stats := []DownloadTaskInfo{{Name: "t1", ETA: 1 * time.Second}}
	require.NoError(t, c.WaitAndAddTorrentSmart(context.Background(), data, "cat", "tag", 10*time.Second, stats, 1024))
}

func TestComputeTorrentHash_ErrorCases(t *testing.T) {
	var buf bytes.Buffer
	_ = bencode.NewEncoder(&buf).Encode(map[string]any{"noinfo": 1})
	_, err := ComputeTorrentHash(buf.Bytes())
	assert.Error(t, err)
}

type transportNetErr struct{}

func (t *transportNetErr) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/api/v2/auth/login" {
		return &http.Response{StatusCode: http.StatusOK, Body: nopBody("Ok."), Header: make(http.Header)}, nil
	}
	if req.URL.Path == "/api/v2/torrents/add" {
		return nil, assert.AnError
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
}

func TestAddTorrent_NetworkError(t *testing.T) {
	global.InitLogger(zap.NewNop())
	c := &QbitClient{Client: &http.Client{Transport: &transportNetErr{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer c.RateLimiter.Stop()
	err := c.AddTorrent([]byte("d4:infoe"), "cat", "tag")
	require.Error(t, err)
}

type transportAdd500 struct{}

func (t *transportAdd500) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/api/v2/auth/login" {
		return &http.Response{StatusCode: http.StatusOK, Body: nopBody("Ok."), Header: make(http.Header)}, nil
	}
	if req.URL.Path == "/api/v2/torrents/add" {
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: nopBody("failed"), Header: make(http.Header)}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
}

func TestAddTorrent_StatusNotOK(t *testing.T) {
	global.InitLogger(zap.NewNop())
	c := &QbitClient{Client: &http.Client{Transport: &transportAdd500{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer c.RateLimiter.Stop()
	err := c.AddTorrent([]byte("d4:infoe"), "cat", "tag")
	require.Error(t, err)
}

type transportAddOK struct{}

func (t *transportAddOK) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/api/v2/auth/login" {
		return &http.Response{StatusCode: http.StatusOK, Body: nopBody("Ok."), Header: make(http.Header)}, nil
	}
	if req.URL.Path == "/api/v2/torrents/add" {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
}

func TestAddTorrent_OK(t *testing.T) {
	global.InitLogger(zap.NewNop())
	c := &QbitClient{Client: &http.Client{Transport: &transportAddOK{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer c.RateLimiter.Stop()
	require.NoError(t, c.AddTorrent([]byte("d4:infoe"), "cat", "tag"))
}

func TestCanAddTorrent_InsufficientSpace(t *testing.T) {
	// use server to return small free space
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
			return
		}
		if r.URL.Path == "/api/v2/sync/maindata" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"server_state":{"free_space_on_disk":10}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	q, err := NewQbitClient(srv.URL, "u", "p", time.Millisecond)
	require.NoError(t, err)
	ok, err := q.CanAddTorrent(context.Background(), 1024*1024)
	require.NoError(t, err)
	require.False(t, ok)
}

type transportSync struct{}

func (t *transportSync) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.Path {
	case "/api/v2/sync/maindata":
		return &http.Response{StatusCode: http.StatusOK, Body: nopBody(`{"server_state":{"free_space_on_disk":1048576}, "torrents":{"h":{"added_on":1730000000}}}`), Header: make(http.Header)}, nil
	case "/api/v2/auth/login":
		return &http.Response{StatusCode: http.StatusOK, Body: nopBody("Ok."), Header: make(http.Header)}, nil
	default:
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	}
}

func TestGetDiskSpace_OK(t *testing.T) {
	c := &QbitClient{Client: &http.Client{Transport: &transportSync{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer c.RateLimiter.Stop()
	sp, err := c.GetDiskSpace(context.Background())
	require.NoError(t, err)
	require.True(t, sp > 0)
}

func TestGetLastAddedTorrentTime_OK(t *testing.T) {
	c := &QbitClient{Client: &http.Client{Transport: &transportSync{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer c.RateLimiter.Stop()
	tm, err := c.GetLastAddedTorrentTime()
	require.NoError(t, err)
	require.False(t, tm.IsZero())
}

type transportCheck500 struct{}

func (t *transportCheck500) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/api/v2/torrents/properties" {
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: http.NoBody, Header: make(http.Header)}, nil
	}
	if req.URL.Path == "/api/v2/auth/login" {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
}

func TestCheckTorrentExists_StatusError(t *testing.T) {
	global.InitLogger(zap.NewNop())
	c := &QbitClient{Client: &http.Client{Transport: &transportCheck500{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer c.RateLimiter.Stop()
	_, err := c.CheckTorrentExists("abc")
	require.Error(t, err)
}

type transportCheckDecodeFail struct{}

func (t *transportCheckDecodeFail) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/api/v2/torrents/properties" {
		return &http.Response{StatusCode: http.StatusOK, Body: nopBody("not-json"), Header: make(http.Header)}, nil
	}
	if req.URL.Path == "/api/v2/auth/login" {
		return &http.Response{StatusCode: http.StatusOK, Body: nopBody("Ok."), Header: make(http.Header)}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
}

func TestCheckTorrentExists_DecodeError(t *testing.T) {
	global.InitLogger(zap.NewNop())
	c := &QbitClient{Client: &http.Client{Transport: &transportCheckDecodeFail{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer c.RateLimiter.Stop()
	_, err := c.CheckTorrentExists("abc")
	require.Error(t, err)
}

type authOKTransport struct{}

func (t *authOKTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/api/v2/auth/login" {
		return &http.Response{StatusCode: http.StatusOK, Body: nopBody("Ok."), Header: make(http.Header)}, nil
	}
	if req.URL.Path == "/api/v2/torrents/properties" {
		return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: make(http.Header)}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
}

func TestQbitClient_CheckAndProcess(t *testing.T) {
	global.InitLogger(zap.NewNop())
	c := &QbitClient{Client: &http.Client{Transport: &authOKTransport{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer c.RateLimiter.Stop()
	ok, err := c.CheckTorrentExists("abc")
	require.NoError(t, err)
	assert.False(t, ok)
	c2 := &QbitClient{Client: &http.Client{Transport: &transport500Props{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer c2.RateLimiter.Stop()
	_, err = c2.CanAddTorrent(context.Background(), 1024)
	assert.Error(t, err)
}

type transport500Props struct{}

func (t *transport500Props) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/api/v2/auth/login" {
		return &http.Response{StatusCode: http.StatusOK, Body: nopBody("Ok."), Header: make(http.Header)}, nil
	}
	if req.URL.Path == "/api/v2/torrents/properties" {
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: http.NoBody, Header: make(http.Header)}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
}

func TestDoRequestWithRetry_403_Immediate(t *testing.T) {
	global.InitLogger(zap.NewNop())
	c := &QbitClient{Client: &http.Client{}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer c.RateLimiter.Stop()
	shared := []DownloadTaskInfo{{Name: "a", ETA: time.Millisecond * 10}}
	err := c.WaitAndAddTorrentSmart(context.Background(), []byte("data"), "cat", "tag", time.Second, shared, 1024*1024)
	require.Error(t, err)
}

func TestGetTorrentFilesPath_OnlyTorrent(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.torrent")
	p2 := filepath.Join(dir, "b.txt")
	require.NoError(t, os.WriteFile(p1, []byte("d4:infoe"), 0o644))
	require.NoError(t, os.WriteFile(p2, []byte("x"), 0o644))
	files, err := GetTorrentFilesPath(dir)
	require.NoError(t, err)
	require.Equal(t, 1, len(files))
	require.Equal(t, p1, files[0])
}

func TestProcessTorrentDirectory_Basic(t *testing.T) {
	c := &QbitClient{Client: &http.Client{Transport: &transportProps200AddOK{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer c.RateLimiter.Stop()
	dir := t.TempDir()
	var buf bytes.Buffer
	_ = bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": "abc"}})
	p := filepath.Join(dir, "x.torrent")
	require.NoError(t, os.WriteFile(p, buf.Bytes(), 0o644))
	require.NoError(t, c.ProcessTorrentDirectory(context.Background(), dir, "cat", "tag"))
}

func TestProcessTorrentDirectory_ReadDirError(t *testing.T) {
	c := &QbitClient{Client: &http.Client{Transport: &transportProps200AddOK{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer c.RateLimiter.Stop()
	// pass non-existent directory
	err := c.ProcessTorrentDirectory(context.Background(), filepath.Join(t.TempDir(), "noexist"), "cat", "tag")
	require.Error(t, err)
}

type transportLowSpace struct{}

func (t *transportLowSpace) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.Path {
	case "/api/v2/sync/maindata":
		return &http.Response{StatusCode: http.StatusOK, Body: nopBody(`{"server_state":{"free_space_on_disk":10}}`), Header: make(http.Header)}, nil
	case "/api/v2/torrents/properties":
		return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: make(http.Header)}, nil
	case "/api/v2/auth/login":
		return &http.Response{StatusCode: http.StatusOK, Body: nopBody("Ok."), Header: make(http.Header)}, nil
	default:
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	}
}

func TestProcessSingleTorrentFile_LowSpaceSkipsAdd(t *testing.T) {
	c := &QbitClient{Client: &http.Client{Transport: &transportLowSpace{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example"}
	defer c.RateLimiter.Stop()
	var buf bytes.Buffer
	_ = bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": "abc"}})
	p := filepath.Join(t.TempDir(), "z.torrent")
	data := make([]byte, 1024)
	copy(data, buf.Bytes())
	require.NoError(t, os.WriteFile(p, data, 0o644))
	require.NoError(t, c.ProcessSingleTorrentFile(context.Background(), p, "cat", "tag"))
}

type transportAuthFail struct{}

func (t *transportAuthFail) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/api/v2/auth/login" {
		return &http.Response{StatusCode: http.StatusOK, Body: nopBody("Bad"), Header: make(http.Header)}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
}

func TestAuthenticate_BodyNotOk_Fails(t *testing.T) {
	c := &QbitClient{Client: &http.Client{Transport: &transportAuthFail{}}, RateLimiter: time.NewTicker(time.Millisecond), BaseURL: "http://example", Username: "u", Password: "p"}
	defer c.RateLimiter.Stop()
	err := c.authenticate()
	require.Error(t, err)
}
