package v2

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 传输层失败统一带 ErrNetworkError，原错误链保留。
func TestSiteHTTPClient_TransportErrorIsNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		require.True(t, ok)
		conn, _, err := hj.Hijack()
		require.NoError(t, err)
		_ = conn.Close()
	}))
	defer srv.Close()

	c := NewSiteHTTPClient(SiteHTTPClientConfig{Timeout: 5 * time.Second})
	_, err := c.Get(context.Background(), srv.URL, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNetworkError)
	var uerr *url.Error
	assert.True(t, errors.As(err, &uerr), "仍能取到 *url.Error")
}

func tooManyRequestsServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"slow down"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// 各驱动把 HTTP 429 映射成 ErrRateLimited，登录探测据此判为限流并可走后备，而不是 UNKNOWN。
func TestDrivers_429IsRateLimited(t *testing.T) {
	srv := tooManyRequestsServer(t)
	ctx := context.Background()

	_, err := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: srv.URL, Cookie: "c=1"}).
		Execute(ctx, NexusPHPRequest{Path: "/index.php", Method: "GET"})
	assert.ErrorIs(t, err, ErrRateLimited, "nexusphp")

	_, err = NewMTorrentDriver(MTorrentDriverConfig{BaseURL: srv.URL, APIKey: "k"}).
		Execute(ctx, MTorrentRequest{Endpoint: "/api/member/profile", Method: "POST"})
	assert.ErrorIs(t, err, ErrRateLimited, "mtorrent")

	_, err = NewUnit3DDriver(Unit3DDriverConfig{BaseURL: srv.URL, APIKey: "k"}).
		Execute(ctx, Unit3DRequest{Endpoint: "/api/user", Method: "GET"})
	assert.ErrorIs(t, err, ErrRateLimited, "unit3d")

	_, err = NewGazelleDriver(GazelleDriverConfig{BaseURL: srv.URL, APIKey: "k"}).
		Execute(ctx, GazelleRequest{Action: "index"})
	assert.ErrorIs(t, err, ErrRateLimited, "gazelle")

	_, err = NewHDDolbyDriver(HDDolbyDriverConfig{BaseURL: srv.URL, APIURL: srv.URL, APIKey: "k"}).
		Execute(ctx, HDDolbyRequest{Endpoint: "/api/v1/user", Method: "POST"})
	assert.ErrorIs(t, err, ErrRateLimited, "hddolby")
}
