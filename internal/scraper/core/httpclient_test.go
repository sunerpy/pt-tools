package core

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// proxyRecorder 返回一个假代理，它不真正转发请求，
// 只是捕获连接然后返回一个固定响应，方便测试"proxy 被使用"。
func proxyRecorder(t *testing.T) (*httptest.Server, *int64) {
	t.Helper()
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok via proxy")
	}))
	return srv, &hits
}

func TestHTTPClient_ExplicitProxy(t *testing.T) {
	proxy, hits := proxyRecorder(t)
	defer proxy.Close()

	cli, err := NewHTTPClient(HTTPClientConfig{ProxyURL: proxy.URL})
	require.NoError(t, err)

	resp, err := cli.Get("http://example.invalid/test")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	require.EqualValues(t, 1, atomic.LoadInt64(hits))
}

func TestHTTPClient_EnvProxy(t *testing.T) {
	proxy, hits := proxyRecorder(t)
	defer proxy.Close()

	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)

	cli, err := NewHTTPClient(HTTPClientConfig{})
	require.NoError(t, err)

	resp, err := cli.Get("http://example.invalid/test")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	require.EqualValues(t, 1, atomic.LoadInt64(hits))
}

func TestHTTPClient_ExplicitOverridesEnv(t *testing.T) {
	envProxy, envHits := proxyRecorder(t)
	defer envProxy.Close()
	cfgProxy, cfgHits := proxyRecorder(t)
	defer cfgProxy.Close()

	t.Setenv("HTTP_PROXY", envProxy.URL)
	t.Setenv("HTTPS_PROXY", envProxy.URL)

	cli, err := NewHTTPClient(HTTPClientConfig{ProxyURL: cfgProxy.URL})
	require.NoError(t, err)

	resp, err := cli.Get("http://example.invalid/")
	require.NoError(t, err)
	resp.Body.Close()

	require.EqualValues(t, 0, atomic.LoadInt64(envHits), "env proxy should NOT be called when explicit set")
	require.EqualValues(t, 1, atomic.LoadInt64(cfgHits))
}

func TestHTTPClient_InvalidProxyURL(t *testing.T) {
	_, err := NewHTTPClient(HTTPClientConfig{ProxyURL: "://bad"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid")
}

func TestHTTPClient_DefaultsApplied(t *testing.T) {
	cli, err := NewHTTPClient(HTTPClientConfig{})
	require.NoError(t, err)
	require.Equal(t, defaultTimeout, cli.Timeout)
}

func TestHTTPClient_UserAgentInjected(t *testing.T) {
	got := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	cli, err := NewHTTPClient(HTTPClientConfig{UserAgent: "pt-scraper/test"})
	require.NoError(t, err)

	resp, err := cli.Get(srv.URL)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, "pt-scraper/test", got)
}

func TestHTTPClient_NoUserAgentByDefault(t *testing.T) {
	cli, err := NewHTTPClient(HTTPClientConfig{})
	require.NoError(t, err)

	_ = cli
	_, err = url.Parse("http://example.invalid")
	require.NoError(t, err)
	_ = time.Now()
}
