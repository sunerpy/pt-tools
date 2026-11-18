package outbound

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/notify"
)

// 测试服务器都在 127.0.0.1 上：要么打开 AllowPrivate，要么用 Divert 把主机名改连过去。
func TestPostJSON(t *testing.T) {
	var ct, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct, auth = r.Header.Get("Content-Type"), r.Header.Get("Authorization")
		if r.URL.Path == "/bad" {
			// 回显请求里的密钥：错误里只能有状态码
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"bad key SECRET-KEY"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	c := NewClient(Options{})

	resp, err := c.PostJSON(context.Background(), Request{
		URL: srv.URL + "/ok", Payload: map[string]string{"a": "b"},
		Header: map[string]string{"Authorization": "Bearer x"}, AllowPrivate: true,
	})
	require.NoError(t, err)
	assert.Equal(t, Response{Status: 200, Body: []byte(`{"ok":true}`)}, resp)
	assert.Equal(t, "application/json", ct)
	assert.Equal(t, "Bearer x", auth)

	resp, err = c.PostJSON(context.Background(), Request{URL: srv.URL + "/bad", AllowPrivate: true})
	var he *HTTPError
	require.ErrorAs(t, err, &he)
	assert.Equal(t, 400, he.Status)
	assert.EqualError(t, err, "HTTP 400", "错误里不带响应体")
	assert.Contains(t, string(resp.Body), "SECRET-KEY", "响应体照样返回，给适配器挑字段")
}

func TestPostJSONErrorsCarryNoURL(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	addr := srv.URL
	srv.Close()
	c := NewClient(Options{})
	_, err := c.PostJSON(context.Background(), Request{URL: addr + "/hook?access_token=SECRET-TOKEN", AllowPrivate: true})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "SECRET-TOKEN", "连接失败的错误里不带地址")

	_, err = c.PostJSON(context.Background(), Request{URL: "http://[::1/hook?access_token=SECRET-TOKEN"})
	assert.EqualError(t, err, "请求地址格式不对")

	_, err = c.PostJSON(context.Background(), Request{URL: "https://x", Payload: func() {}})
	assert.ErrorContains(t, err, "序列化请求失败")
}

func TestPostJSONDoesNotFollowRedirects(t *testing.T) {
	hit := false
	mux := http.NewServeMux()
	mux.HandleFunc("/from", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/to", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/to", func(http.ResponseWriter, *http.Request) { hit = true })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	_, err := NewClient(Options{}).PostJSON(context.Background(), Request{URL: srv.URL + "/from", AllowPrivate: true})
	var he *HTTPError
	require.ErrorAs(t, err, &he)
	assert.Equal(t, http.StatusTemporaryRedirect, he.Status)
	assert.Contains(t, err.Error(), "不跟随跳转")
	assert.False(t, hit, "跳转目标没有收到请求")
}

func TestPostJSONBodyLimit(t *testing.T) {
	size := MaxBody
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", size)))
	}))
	defer srv.Close()
	c := NewClient(Options{})
	resp, err := c.PostJSON(context.Background(), Request{URL: srv.URL, AllowPrivate: true})
	require.NoError(t, err)
	assert.Len(t, resp.Body, MaxBody)

	size = MaxBody + 1
	resp, err = c.PostJSON(context.Background(), Request{URL: srv.URL, AllowPrivate: true})
	require.ErrorIs(t, err, ErrTooLarge)
	assert.Nil(t, resp.Body)
}

// 慢服务器：ctx 到期时请求被取消，服务端看到连接断开，不会在超时之后才送达。
func TestPostJSONHonorsContext(t *testing.T) {
	canceled := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		select {
		case <-r.Context().Done():
			close(canceled)
		case <-time.After(5 * time.Second):
			_, _ = w.Write([]byte(`{"late":true}`))
		}
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := NewClient(Options{}).PostJSON(ctx, Request{URL: srv.URL, Payload: map[string]string{"a": "b"}, AllowPrivate: true})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 2*time.Second)
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("服务端没有看到请求被取消")
	}
}

func TestPostJSONAddressPolicy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := NewClient(Options{})
	_, err := c.PostJSON(context.Background(), Request{URL: srv.URL})
	var ae *AddrError
	require.ErrorAs(t, err, &ae, "不允许内网时不连 127.0.0.1")
	assert.True(t, ae.Private)
	assert.NotEmpty(t, PrivateHint(err))

	// localhost 解析出来也是本机：连接时按解析出的 IP 拦下
	local := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)
	_, err = c.PostJSON(context.Background(), Request{URL: local})
	require.ErrorAs(t, err, &ae)

	_, err = c.PostJSON(context.Background(), Request{URL: srv.URL, AllowPrivate: true})
	require.NoError(t, err)
	assert.Empty(t, PrivateHint(err))
	assert.Empty(t, PrivateHint(errors.New("x")))
}

// Divert：主机名请求改连到本地 TLS 收件端（不校验证书），IP 地址的请求照常连、照常检查。
func TestDivert(t *testing.T) {
	var host, path string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, path = r.Host, r.URL.Path
		_, _ = w.Write([]byte(`{"errcode":0}`))
	}))
	defer srv.Close()
	c := NewClient(Options{Divert: srv.Listener.Addr().String()})
	_, err := c.PostJSON(context.Background(), Request{URL: "https://oapi.dingtalk.com/robot/send?access_token=t"})
	require.NoError(t, err)
	assert.Equal(t, "oapi.dingtalk.com", host)
	assert.Equal(t, "/robot/send", path)

	_, err = c.PostJSON(context.Background(), Request{URL: srv.URL})
	var ae *AddrError
	require.ErrorAs(t, err, &ae, "IP 地址不改连，照常检查")

	assert.True(t, diverted("example.com:443"))
	assert.False(t, diverted("localhost:80"))
	assert.False(t, diverted("[::1]:80"))
	assert.False(t, diverted("bad"))
}

func TestCheckIP(t *testing.T) {
	cases := []struct {
		ip      string
		public  bool // 不允许内网时能连
		private bool // 允许内网时能连
	}{
		{"8.8.8.8", true, true},
		{"198.18.0.1", true, true}, // Clash fake-ip 用的网段，要能连
		{"2001:4860:4860::8888", true, true},
		{"127.0.0.1", false, true},
		{"::1", false, true},
		{"::ffff:127.0.0.1", false, true},
		{"10.1.2.3", false, true},
		{"172.16.0.1", false, true},
		{"192.168.1.10", false, true},
		{"100.100.100.200", false, true}, // 运营商级 NAT 网段（阿里云元数据也在这里）
		{"fd00:ec2::254", false, true},
		{"169.254.169.254", false, false},    // 云厂商元数据
		{"64:ff9b::a9fe:a9fe", false, false}, // NAT64 里的 169.254.169.254
		{"fe80::1%eth0", false, false},
		{"0.0.0.0", false, false},
		{"224.0.0.1", false, false},
		{"255.255.255.255", false, false},
		{"::", false, false},
		{"ff02::1", false, false},
	}
	for _, tc := range cases {
		ip := netip.MustParseAddr(tc.ip)
		assert.Equal(t, tc.public, CheckIP(ip, false) == nil, "%s 不允许内网", tc.ip)
		assert.Equal(t, tc.private, CheckIP(ip, true) == nil, "%s 允许内网", tc.ip)
	}
	assert.Error(t, CheckIP(netip.Addr{}, true), "空地址")

	var ae *AddrError
	require.ErrorAs(t, CheckIP(netip.MustParseAddr("169.254.169.254"), true), &ae)
	assert.False(t, ae.Private)
	assert.Contains(t, ae.Error(), "链路本地")
}

func TestCheckHostAndServerURL(t *testing.T) {
	for _, h := range []string{"localhost", "LOCALHOST.", "nas.localhost", "127.0.0.1", "192.168.1.2"} {
		assert.Error(t, CheckHost(h, false), h)
		assert.NoError(t, CheckHost(h, true), h)
	}
	assert.NoError(t, CheckHost("example.com", false), "主机名保存时不查 DNS")
	assert.Error(t, CheckHost("169.254.169.254", true))

	_, err := ServerURL("http://192.168.1.2:8080", "Bark 服务器地址", false)
	assert.ErrorContains(t, err, "允许内网地址")
	s, err := ServerURL("http://192.168.1.2:8080/", "Bark 服务器地址", true)
	require.NoError(t, err)
	assert.Equal(t, "http://192.168.1.2:8080", s)
	_, err = ServerURL("http://169.254.169.254", "Bark 服务器地址", true)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "允许内网地址", "链路本地地址打开也不能用，不给这个提示")
	_, err = ServerURL("ftp://x", "Bark 服务器地址", true)
	assert.Error(t, err)
}

func TestHTTPError(t *testing.T) {
	assert.Equal(t, "HTTP 502", (&HTTPError{Status: 502}).Error())
	assert.Contains(t, (&HTTPError{Status: 302}).Error(), "不跟随跳转")
}

func TestClean(t *testing.T) {
	assert.Equal(t, "bad key ***, auth ***", Clean(" bad key dev-key-1,\n auth Bearer-tk ", "dev-key-1", "Bearer-tk", ""))
	assert.Equal(t, "ab ok", Clean("ab ok", "ab"), "太短的不当密钥替换")
	long := Clean(strings.Repeat("长", 200))
	assert.Equal(t, 121, len([]rune(long)))
	assert.True(t, strings.HasSuffix(long, "…"))
}

func TestHelpers(t *testing.T) {
	assert.Equal(t, "标题\n正文\nhttps://x", Text(notify.Notification{Title: " 标题 ", Text: "正文", Link: "https://x"}))
	assert.Equal(t, "正文", Text(notify.Notification{Text: "正文"}))
	u, err := HTTPURL(" https://ntfy.example/ ", "地址")
	require.NoError(t, err)
	assert.Equal(t, "https://ntfy.example", u)
	for _, bad := range []string{"ftp://x", "https://u:p@x", "https://x/#a", "https://x/?a=1", "https://x?", "x"} {
		_, err := HTTPURL(bad, "地址")
		assert.Error(t, err, bad)
	}
	var dst struct{ A string }
	require.NoError(t, Config("", &dst, "t"))
	assert.Error(t, Config("{", &dst, "t"))
	assert.Same(t, Default(), Use(nil))
	c := NewClient(Options{})
	assert.Same(t, c, Use(c))
	assert.Empty(t, qaDivert(), "正式构建里没有改连")
}
