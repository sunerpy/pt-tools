package fanart

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/media/cachectl"
)

type memCache struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (c *memCache) Get(k string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[k]
	return v, ok
}

func (c *memCache) Set(k string, v []byte, _ time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[k] = v
}

const key = "projectkey0123456789"

func newServer(t *testing.T, hits *atomic.Int32) *httptest.Server {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			if r.URL.Path == "/assets/html.png" {
				w.Header().Set("Content-Type", "text/html")
			} else {
				w.Header().Set("Content-Type", "image/png")
			}
			_, _ = w.Write([]byte("PNG" + r.URL.Path))
			return
		}
		assert.Empty(t, r.URL.RawQuery, "Key 放在请求头里，不放进地址")
		if r.Header.Get("api-key") != key {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		assert.Equal(t, "personal", r.Header.Get("client-key"))
		w.Header().Set("Content-Type", "application/json")
		img := func(path, lang string, likes any) string {
			l := fmt.Sprint(likes)
			if s, ok := likes.(string); ok {
				l = strconv.Quote(s)
			}
			return `{"id":"1","url":"` + srv.URL + `/assets/` + path + `","lang":"` + lang + `","likes":` + l + `}`
		}
		switch r.URL.Path {
		case "/v3.2/movies/872585":
			_, _ = w.Write([]byte(`{"name":"Oppenheimer","hdmovielogo":[` + img("logo-en.png", "en", "3") + `,` + img("logo-zh.png", "zh", 9) + `],
				"movielogo":[` + img("old-logo.png", "en", 50) + `],"hdmovieclearart":[` + img("clearart.png", "00", 1) + `],
				"moviedisc":[{"url":"` + srv.URL + `/assets/disc.png","lang":"en","likes":"2","disc":"1","disc_type":"bluray"}],
				"moviethumb":[` + img("thumb.jpg", "en", 1) + `],"moviebanner":[` + img("banner.jpg", "en", 1) + `],
				"moviebackground":[` + img("bg.jpg", "", 4) + `],"movieposter":[{"url":"javascript:alert(1)","lang":"en","likes":"1"}]}`))
		case "/v3.2/tv/392256":
			_, _ = w.Write([]byte(`{"name":"The Last of Us","hdtvlogo":[` + img("tv-logo.png", "en", 2) + `],
				"seasonposter":[{"url":"` + srv.URL + `/assets/s1.jpg","lang":"en","likes":"5","season":"1"},
				{"url":"` + srv.URL + `/assets/sall.jpg","lang":"en","likes":"1","season":"all"},
				{"url":"` + srv.URL + `/assets/sbad.jpg","lang":"en","likes":"1","season":"x"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status":"error","error message":"Not found"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestImages(t *testing.T) {
	var hits atomic.Int32
	srv := newServer(t, &hits)
	c, err := New(Options{
		APIKey: key, ClientKey: "personal", BaseURL: srv.URL + "/v3.2", ImageHosts: []string{srv.Listener.Addr().String()},
		Cache: &memCache{m: map[string][]byte{}},
	})
	require.NoError(t, err)
	ctx := context.Background()

	m, err := c.Movie(ctx, "872585")
	require.NoError(t, err)
	logos := m["logo"]
	require.Len(t, logos, 3)
	assert.Equal(t, srv.URL+"/assets/logo-zh.png", logos[0].URL, "组内按点赞数排，字符串的点赞数也认")
	assert.Equal(t, srv.URL+"/assets/old-logo.png", logos[2].URL, "高清的组在前面")
	assert.Equal(t, "", m["clearart"][0].Lang, "00 是没有文字")
	assert.Len(t, m["discart"], 1)
	assert.Len(t, m["landscape"], 1)
	assert.Len(t, m["banner"], 1)
	assert.Len(t, m["fanart"], 1)
	assert.Empty(t, m["poster"], "不是 http(s) 的地址不要")

	tv, err := c.TV(ctx, 392256)
	require.NoError(t, err)
	require.Len(t, tv["season_poster"], 2, "季号认不出的不要")
	assert.Equal(t, 1, tv["season_poster"][0].Season)
	assert.Equal(t, -1, tv["season_poster"][1].Season, "all 记成 -1")

	none, err := c.Movie(ctx, "1")
	require.NoError(t, err)
	assert.Empty(t, none, "404 是没有图片")
	before := hits.Load()
	_, err = c.Movie(ctx, "1")
	require.NoError(t, err)
	_, err = c.Movie(ctx, "872585")
	require.NoError(t, err)
	assert.Equal(t, before, hits.Load(), "没有的与有的都读缓存")
	_, err = c.Movie(cachectl.Bypass(ctx), "872585")
	require.NoError(t, err)
	assert.Equal(t, before+1, hits.Load(), "重新刮削时不读缓存")

	b, err := c.Download(ctx, logos[0].URL)
	require.NoError(t, err)
	assert.Equal(t, "PNG/assets/logo-zh.png", string(b))
	_, err = c.Download(ctx, srv.URL+"/assets/html.png")
	assert.ErrorIs(t, err, ErrNotImage)
	_, err = c.Download(ctx, "https://evil.example/x.png")
	assert.ErrorContains(t, err, "不是 Fanart.tv 的图片地址")

	_, err = c.Movie(ctx, "../x")
	assert.Error(t, err)
	_, err = c.TV(ctx, 0)
	assert.Error(t, err)
}

func TestKeyErrors(t *testing.T) {
	var hits atomic.Int32
	srv := newServer(t, &hits)
	_, err := New(Options{APIKey: " "})
	assert.ErrorIs(t, err, ErrNoKey)
	c, err := New(Options{APIKey: "wrongwrongwrongwrong", ClientKey: "personal", BaseURL: srv.URL + "/v3.2"})
	require.NoError(t, err)
	_, err = c.Movie(context.Background(), "872585")
	assert.ErrorIs(t, err, ErrUnauthorized)
	assert.NotContains(t, err.Error(), "wrongwrong")
	assert.ErrorIs(t, c.Validate(context.Background()), ErrUnauthorized)

	good, err := New(Options{APIKey: key, ClientKey: "personal", BaseURL: srv.URL + "/v3.2"})
	require.NoError(t, err)
	assert.ErrorIs(t, good.Validate(context.Background()), ErrUnavailable, "测试用的电影没有时也报出来")

	assert.True(t, (&Client{}).allowedImage("https://assets.fanart.tv/fanart/movies/1/x.png"))
	assert.False(t, (&Client{}).allowedImage("http://assets.fanart.tv/x.png"), "默认只认 https")
	assert.False(t, (&Client{}).allowedImage("https://fanart.tv.evil.example/x.png"))
	assert.False(t, (&Client{}).allowedImage("https://user:pw@assets.fanart.tv/x.png"))
}

// 跳转：图片跳到别的域名、接口跳到别的主机都不跟（接口的请求头里有 API Key）
func TestRedirects(t *testing.T) {
	var other atomic.Int32
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		other.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("EVIL"))
	}))
	t.Cleanup(evil.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, evil.URL+r.URL.Path, http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	c, err := New(Options{APIKey: key, BaseURL: srv.URL + "/v3.2", ImageHosts: []string{srv.Listener.Addr().String()}})
	require.NoError(t, err)
	_, err = c.Download(context.Background(), srv.URL+"/assets/x.png")
	assert.ErrorContains(t, err, "跳转到了别的地址")
	_, err = c.Movie(context.Background(), "872585")
	assert.ErrorContains(t, err, "跳转到了别的地址")
	assert.Zero(t, other.Load(), "没有请求到跳转的地址")
}

// 接口从 https 跳到同一个主机的 http：不跟（请求头里有 API Key）
func TestRedirectDowngrade(t *testing.T) {
	rt := &redirectTransport{to: "http://webservice.fanart.tv/v3.2/movies/872585"}
	c, err := New(Options{APIKey: key, HTTP: &http.Client{Transport: rt}})
	require.NoError(t, err)
	_, err = c.Movie(context.Background(), "872585")
	assert.ErrorContains(t, err, "跳转到了别的地址")
	assert.Equal(t, []string{"https://webservice.fanart.tv/v3.2/movies/872585"}, rt.urls, "没有用 http 发出带 Key 的请求")
}

// redirectTransport 把第一个请求跳到 to，记下发出的每个地址。
type redirectTransport struct {
	to   string
	urls []string
}

func (r *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.urls = append(r.urls, req.URL.String())
	return &http.Response{
		StatusCode: http.StatusFound, Header: http.Header{"Location": {r.to}}, Body: io.NopCloser(strings.NewReader("")), Request: req,
	}, nil
}

type recordTransport struct{ urls []string }

func (r *recordTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.urls = append(r.urls, req.URL.String())
	return &http.Response{
		StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"image/png"}}, Body: io.NopCloser(strings.NewReader("PNG")), Request: req,
	}, nil
}

// 接口给的 http:// 图片地址换成 https:// 再下载
func TestDownloadUpgradesHTTP(t *testing.T) {
	rt := &recordTransport{}
	c, err := New(Options{APIKey: key, HTTP: &http.Client{Transport: rt}})
	require.NoError(t, err)
	b, err := c.Download(context.Background(), "http://assets.fanart.tv/fanart/movies/120/hdmovielogo/x.png")
	require.NoError(t, err)
	assert.Equal(t, "PNG", string(b))
	assert.Equal(t, []string{"https://assets.fanart.tv/fanart/movies/120/hdmovielogo/x.png"}, rt.urls)
	_, err = c.Download(context.Background(), "http://evil.example/x.png")
	assert.ErrorContains(t, err, "不是 Fanart.tv 的图片地址")
}
