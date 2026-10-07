package iyuu

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeIYUU 按 iyuuplus-dev 客户端的约定回应：校验 token 头、表单字段与 sha1。
type fakeIYUU struct {
	t        *testing.T
	token    string
	code     int // 不为 0 时所有接口都回这个 code
	data     map[string]any
	lastForm map[string][]string
}

func (f *fakeIYUU) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		write := func(code int, msg string, data any) {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "msg": msg, "data": data})
		}
		if r.Header.Get("token") != f.token {
			write(403, "token 无效", map[string]any{})
			return
		}
		require.NoError(f.t, r.ParseForm())
		f.lastForm = r.PostForm
		if f.code == 429 {
			write(429, "请求太频繁", map[string]any{"Retry-After": 12, "X-RateLimit-Limit": 10})
			return
		}
		if f.code != 0 {
			write(f.code, "服务器错误", map[string]any{})
			return
		}
		switch r.URL.Path {
		case "/reseed/sites/index":
			write(0, "ok", map[string]any{"sites": []map[string]any{
				{"id": 1, "site": "hdsky", "nickname": "天空", "base_url": "hdsky.me", "download_page": "download.php?id={}", "is_https": 2},
			}})
		case "/reseed/sites/reportExisting":
			write(0, "ok", map[string]any{"sid_sha1": "s1"})
		case "/reseed/index/index":
			hash := r.PostForm.Get("hash")
			sum := sha1.Sum([]byte(hash))
			if r.PostForm.Get("sha1") != hex.EncodeToString(sum[:]) {
				write(400, "sha1 不符", map[string]any{})
				return
			}
			if f.data == nil {
				_, _ = w.Write([]byte(`{"code":0,"msg":"ok","data":[]}`))
				return
			}
			write(0, "ok", f.data)
		default:
			http.NotFound(w, r)
		}
	}))
}

func newClient(t *testing.T, f *fakeIYUU) *Client {
	srv := f.server()
	t.Cleanup(srv.Close)
	c := New("tok")
	c.BaseURL = srv.URL
	c.Now = func() time.Time { return time.Unix(1700000000, 0) }
	return c
}

func TestHashParamMatchesPHP(t *testing.T) {
	hash, sum := HashParam([]string{"BB", "aa"})
	assert.Equal(t, `["aa","bb"]`, hash, "排序、小写、无空格（与 PHP json_encode 一致）")
	want := sha1.Sum([]byte(`["aa","bb"]`))
	assert.Equal(t, hex.EncodeToString(want[:]), sum)
}

func TestClientSitesReportAndQuery(t *testing.T) {
	f := &fakeIYUU{t: t, token: "tok"}
	c := newClient(t, f)
	ctx := context.Background()

	sites, err := c.Sites(ctx)
	require.NoError(t, err)
	require.Len(t, sites, 1)
	assert.Equal(t, Site{SID: 1, Site: "hdsky", Nickname: "天空", BaseURL: "hdsky.me", DownloadPage: "download.php?id={}", IsHTTPS: 2}, sites[0])

	sum, err := c.ReportExisting(ctx, []int{1, 7})
	require.NoError(t, err)
	assert.Equal(t, "s1", sum)
	assert.Equal(t, []string{"1", "7"}, f.lastForm["sid_list[]"])

	got, err := c.Query(ctx, []string{"aa"}, "s1")
	require.NoError(t, err)
	assert.Empty(t, got, "没有结果时 data 是 []")
	assert.Equal(t, Version, f.lastForm["version"][0])
	assert.Equal(t, "1700000000", f.lastForm["timestamp"][0])
	assert.Equal(t, "s1", f.lastForm["sid_sha1"][0])

	f.data = map[string]any{"AA": map[string]any{"torrent": []map[string]any{
		{"sid": 1, "torrent_id": 12345, "info_hash": "CC"},
		{"sid": "2", "torrent_id": "678", "info_hash": "dd"},
	}}}
	got, err = c.Query(ctx, []string{"aa"}, "s1")
	require.NoError(t, err)
	assert.Equal(t, []Candidate{{SID: 1, TorrentID: "12345", InfoHash: "cc"}, {SID: 2, TorrentID: "678", InfoHash: "dd"}}, got["aa"])

	_, err = c.Query(ctx, make([]string, MaxBatch+1), "s1")
	assert.Error(t, err)
	empty, err := c.Query(ctx, nil, "s1")
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestClientErrors(t *testing.T) {
	ctx := context.Background()
	_, err := New("").Sites(ctx)
	assert.ErrorIs(t, err, ErrNoToken)

	f := &fakeIYUU{t: t, token: "tok", code: 429}
	c := newClient(t, f)
	_, err = c.Sites(ctx)
	var rl *RateLimitError
	require.ErrorAs(t, err, &rl)
	assert.Equal(t, 12*time.Second, rl.RetryAfter)
	assert.Contains(t, rl.Error(), "12 秒后再试")

	f.code = 500
	_, err = c.ReportExisting(ctx, []int{1})
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 500, apiErr.Code)

	bad := &fakeIYUU{t: t, token: "other"}
	c2 := newClient(t, bad)
	_, err = c2.Sites(ctx)
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 403, apiErr.Code)

	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer down.Close()
	c3 := New("tok")
	c3.BaseURL = down.URL
	_, err = c3.Sites(ctx)
	assert.ErrorContains(t, err, "服务器繁忙")

	notJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>")) }))
	defer notJSON.Close()
	c4 := New("tok")
	c4.BaseURL = notJSON.URL
	_, err = c4.Sites(ctx)
	assert.ErrorContains(t, err, "不是 JSON")
	assert.False(t, errors.Is(err, ErrNoToken))
}
