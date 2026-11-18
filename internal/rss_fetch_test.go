package internal

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 照着示例填、忘了换的占位地址：不去请求，直接说清楚
func TestFetchRSSFeed_PlaceholderNotRequested(t *testing.T) {
	for _, raw := range []string{
		"https://hdsky.xxx/torrentrss.php?xxx",
		"https://springxxx.xxx",
		"https://rss.m-team.xxx/api/rss/xxx",
	} {
		_, err := FetchRSSFeed(context.Background(), "", raw)
		require.Error(t, err, raw)
		assert.ErrorIs(t, err, ErrRSSPlaceholder, raw)
		assert.Contains(t, err.Error(), "占位地址", raw)
		assert.Contains(t, err.Error(), "站点 RSS 页", raw)
	}
}

func TestFetchRSSFeed_BadScheme(t *testing.T) {
	for _, raw := range []string{"", "ftp://hdsky.me/rss", "hdsky.me/torrentrss.php", "://x"} {
		_, err := FetchRSSFeed(context.Background(), "", raw)
		require.Error(t, err, raw)
		assert.Contains(t, err.Error(), "http:// 或 https://", raw)
	}
}

func rssServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchRSSFeed_StatusAndContentErrors(t *testing.T) {
	cases := []struct {
		name string
		h    http.HandlerFunc
		want []string
	}{
		{"cloudflare 质询", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cf-Mitigated", "challenge")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("<html>Just a moment...</html>"))
		}, []string{"HTTP 403", "Cloudflare"}},
		{"passkey 失效", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}, []string{"HTTP 403", "passkey"}},
		{"地址不对", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}, []string{"HTTP 404", "重新复制链接"}},
		{"太频繁", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}, []string{"HTTP 429", "间隔调大"}},
		{"站点出错", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}, []string{"HTTP 502", "站点那边出错"}},
		{"回的是登录页", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("<!DOCTYPE html><html><body>login</body></html>"))
		}, []string{"回的是网页不是 RSS"}},
		{"不是 XML", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("not xml"))
		}, []string{"解析 RSS 失败"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := rssServer(t, tc.h)
			_, err := FetchRSSFeed(context.Background(), "", srv.URL+"/torrentrss.php?passkey=TOPSECRET")
			require.Error(t, err)
			for _, w := range tc.want {
				assert.Contains(t, err.Error(), w)
			}
			assert.NotContains(t, err.Error(), "TOPSECRET")
		})
	}
}

func TestFetchRSSFeed_OK(t *testing.T) {
	var hits atomic.Int32
	srv := rssServer(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		assert.Contains(t, r.Header.Get("User-Agent"), "Mozilla/5.0")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>t</title>` +
			`<item><title>A</title><guid>1</guid><enclosure url="http://e/1" length="1073741824" type="application/x-bittorrent"/></item>` +
			`</channel></rss>`))
	})
	feed, err := FetchRSSFeed(context.Background(), "", srv.URL)
	require.NoError(t, err)
	require.Len(t, feed.Items, 1)
	assert.Equal(t, int64(1<<30), rssEnclosureSize(feed.Items[0]))
	assert.Equal(t, int32(1), hits.Load())
}

// 传输层的错误分类（不连真实网络）：DNS 查不到、拒绝连接、被断开、超时
func TestRSSTransportError(t *testing.T) {
	wrap := func(inner error) error {
		return &url.Error{Op: "Get", URL: "https://hdsky.me/torrentrss.php?passkey=TOPSECRET", Err: &net.OpError{Op: "dial", Net: "tcp", Err: inner}}
	}
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"dns 查不到", wrap(&net.DNSError{Err: "no such host", Name: "hdsky.me", IsNotFound: true}), "找不到域名 hdsky.me"},
		{"dns 暂时失败", wrap(&net.DNSError{Err: "server misbehaving", Name: "hdsky.me", IsTemporary: true}), "解析域名 hdsky.me 失败"},
		{"拒绝连接", wrap(os.NewSyscallError("connect", syscall.ECONNREFUSED)), "连接被拒绝"},
		{"被断开", wrap(os.NewSyscallError("read", syscall.ECONNRESET)), "connection reset"},
		{"超时", context.DeadlineExceeded, "超时"},
		{"别的", errors.New("boom"), "请求 hdsky.me 失败"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := rssTransportError("hdsky.me", tc.err)
			assert.Contains(t, err.Error(), tc.want)
			assert.NotContains(t, err.Error(), "TOPSECRET")
			assert.ErrorIs(t, err, tc.err, "原错误链保留")
		})
	}
}
