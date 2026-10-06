package v2

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

// 按站点定义取用户信息时，每一页都连不上：报网络错误（探测据此判为 NETWORK_ERROR、可走后备），
// 而不是拿一份空的用户信息回去、被当成「没有用户名」的解析失败。
func TestNexusPHPDefinitionUserInfo_AllPagesUnreachableIsNetworkError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	closed := "http://" + ln.Addr().String()
	require.NoError(t, ln.Close())

	def, ok := GetDefinitionRegistry().Get("hdsky")
	require.True(t, ok)
	require.NotNil(t, def.UserInfo, "hdsky 走站点定义的用户信息流程")
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: closed, Cookie: "c=1"})
	d.SetSiteDefinition(def)

	_, err = d.GetUserInfo(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNetworkError)
}

// 依赖页（这里是 userdetails）连接被掐断：只是少了这一页的字段，已取到的用户名与 ID 照样返回，不报错。
func TestNexusPHPDefinitionUserInfo_DependentPageFailureKeepsPartialData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "userdetails") {
			hj, ok := w.(http.Hijacker)
			require.True(t, ok)
			conn, _, err := hj.Hijack()
			require.NoError(t, err)
			_ = conn.Close()
			return
		}
		_, _ = w.Write([]byte(`<html><body><a id="uid" href="userdetails.php?id=888">tester</a></body></html>`))
	}))
	defer server.Close()

	def := &SiteDefinition{
		ID: "testsite",
		UserInfo: &UserInfoConfig{
			Process: []UserInfoProcess{
				{RequestConfig: RequestConfig{URL: "/index.php", ResponseType: "document"}, Fields: []string{"id", "name"}},
				{
					RequestConfig: RequestConfig{URL: "/userdetails.php", ResponseType: "document"},
					Assertion:     map[string]string{"id": "params.id"},
					Fields:        []string{"uploaded"},
				},
			},
			Selectors: map[string]FieldSelector{
				"id":       {Selector: []string{"#uid"}, Attr: "href", Filters: []Filter{{Name: "querystring", Args: []any{"id"}}}},
				"name":     {Selector: []string{"#uid"}},
				"uploaded": {Selector: []string{"#up"}},
			},
		},
	}
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
	d.SetSiteDefinition(def)

	info, err := d.GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "tester", info.Username)
	assert.Equal(t, "888", info.UserID)
	assert.Zero(t, info.Uploaded, "掐断的那一页没有数据")

	pe := partialFetchError{err: ErrNetworkError}
	assert.Equal(t, ErrNetworkError.Error(), pe.Error())
	assert.ErrorIs(t, pe, ErrNetworkError)
}
