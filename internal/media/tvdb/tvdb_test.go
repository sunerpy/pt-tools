package tvdb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
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

func newCache() *memCache { return &memCache{m: map[string][]byte{}} }

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

const (
	testKey   = "11111111-2222-3333-4444-555555555555"
	testPIN   = "PIN0042"
	testToken = "token-0001"
)

// fakeTVDB 是假的 TheTVDB：登录只认 testKey（要 PIN 时还要 testPIN）；pages 是第 N 页的响应（没有的页 404）。
type fakeTVDB struct {
	t       *testing.T
	srv     *httptest.Server
	mu      sync.Mutex
	needPIN bool
	logins  []map[string]any
	token   string
	pages   map[int]string
	// basePages 是不带语言的列表的第 N 页（没有的页 404）
	basePages map[int]string
	requests  []string
	// reject 为真时数据请求一律 401（令牌失效）
	reject int
	// loginStatus 不为 0 时登录回这个状态
	loginStatus int
	// dataStatus 不为 0 时数据请求回这个状态
	dataStatus int
	authOnImg  int
}

func newFake(t *testing.T) *fakeTVDB {
	f := &fakeTVDB{t: t, token: testToken, pages: map[int]string{}, basePages: map[int]string{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeTVDB) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.URL.Path == "/v4/login":
		var in map[string]any
		b, _ := io.ReadAll(r.Body)
		require.NoError(f.t, json.Unmarshal(b, &in))
		f.logins = append(f.logins, in)
		if f.loginStatus != 0 {
			w.WriteHeader(f.loginStatus)
			return
		}
		pin, hasPIN := in["pin"]
		if in["apikey"] != testKey || (f.needPIN && pin != testPIN) || (hasPIN && pin == "") {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"status":"failure","message":"InvalidAPIKey"}`))
			return
		}
		_, _ = fmt.Fprintf(w, `{"status":"success","data":{"token":%q}}`, f.token)
	case strings.HasPrefix(r.URL.Path, "/banners/"):
		if r.Header.Get("Authorization") != "" {
			f.authOnImg++
		}
		if strings.HasSuffix(r.URL.Path, ".html") {
			w.Header().Set("Content-Type", "text/html")
		} else {
			w.Header().Set("Content-Type", "image/jpeg")
		}
		_, _ = w.Write([]byte("JPG" + r.URL.Path))
	default:
		f.requests = append(f.requests, r.URL.RequestURI())
		if f.reject > 0 || r.Header.Get("Authorization") != "Bearer "+f.token {
			if f.reject > 0 {
				f.reject--
			}
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if f.dataStatus != 0 {
			w.WriteHeader(f.dataStatus)
			return
		}
		if r.URL.Path == "/v4/series/81189" {
			_, _ = w.Write([]byte(`{"status":"success","data":{"id":81189,"name":"Breaking Bad"}}`))
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if r.URL.Path == "/v4/series/423432/episodes/official" {
			body, ok := f.basePages[page]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(body))
			return
		}
		body, ok := f.pages[page]
		if !ok || !strings.HasPrefix(r.URL.Path, "/v4/series/423432/episodes/official/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}
}

func (f *fakeTVDB) client(t *testing.T, o Options) *Client {
	t.Helper()
	if o.APIKey == "" {
		o.APIKey = testKey
	}
	o.BaseURL = f.srv.URL + "/v4"
	if o.ImageHosts == nil {
		o.ImageHosts = []string{strings.TrimPrefix(f.srv.URL, "http://")}
	}
	if o.Limiter == nil {
		o.Limiter = NewLimiter(1000)
	}
	c, err := New(o)
	require.NoError(t, err)
	return c
}

func (f *fakeTVDB) stats() (logins int, requests []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.logins), append([]string(nil), f.requests...)
}

// page 是一页的响应：data.series 带原语言与这几集；links 不为空时顶层带 links（next 为空串时写 null）。
func page(links *string, eps ...string) string {
	body := `{"status":"success","data":{"series":{"id":423432,"originalLanguage":"zho","episodes":[` + strings.Join(eps, ",") + `]}}`
	if links != nil {
		next := "null"
		if *links != "" {
			next = strconv.Quote(*links)
		}
		body += `,"links":{"prev":null,"self":"x","next":` + next + `,"total_items":4,"page_size":2}`
	}
	return body + "}"
}

func ep(id, season, number int, name string) string {
	return fmt.Sprintf(`{"id":%d,"seasonNumber":%d,"number":%d,"name":%q,"overview":"简介%d","aired":"2026-10-0%d","image":"/banners/v4/episode/%d/screencap/a.jpg","nameTranslations":["zho","eng"],"overviewTranslations":["ZHO"]}`,
		id, season, number, name, number, number, id)
}

func strp(s string) *string { return &s }

func TestNew(t *testing.T) {
	_, err := New(Options{APIKey: "  "})
	require.ErrorIs(t, err, ErrNoKey)
	c, err := New(Options{APIKey: testKey})
	require.NoError(t, err)
	assert.Equal(t, DefaultBaseURL, c.base)
}

// 没有 PIN 时登录请求里不带 pin（不是 user-supported 的 Key 要整个去掉）；有 PIN 时带上。
func TestLoginBody(t *testing.T) {
	f := newFake(t)
	f.pages[0] = page(nil)
	_, err := f.client(t, Options{}).Episodes(context.Background(), 423432, "zho")
	require.NoError(t, err)
	_, err = f.client(t, Options{PIN: " " + testPIN + " "}).Episodes(context.Background(), 423432, "zho")
	require.NoError(t, err)
	require.Len(t, f.logins, 2)
	assert.Equal(t, map[string]any{"apikey": testKey}, f.logins[0])
	assert.Equal(t, map[string]any{"apikey": testKey, "pin": testPIN}, f.logins[1])
}

func TestLoginErrors(t *testing.T) {
	for status, want := range map[int]error{
		http.StatusUnauthorized: ErrUnauthorized, http.StatusForbidden: ErrUnauthorized,
		http.StatusTooManyRequests: ErrRateLimited, http.StatusBadGateway: ErrUnavailable,
	} {
		f := newFake(t)
		f.loginStatus = status
		_, err := f.client(t, Options{PIN: testPIN}).Episodes(context.Background(), 423432, "zho")
		require.ErrorIs(t, err, want, status)
		assert.NotContains(t, err.Error(), testKey)
		assert.NotContains(t, err.Error(), testPIN)
	}
	// 要 PIN 的 Key 没填 PIN
	f := newFake(t)
	f.needPIN = true
	err := f.client(t, Options{}).Validate(context.Background())
	require.ErrorIs(t, err, ErrUnauthorized)
	assert.NoError(t, f.client(t, Options{PIN: testPIN}).Validate(context.Background()))
}

// 顶层带 links：按 links.next 翻页（不跟它给的地址），next 为 null 时停。
func TestEpisodesPagesWithLinks(t *testing.T) {
	f := newFake(t)
	f.pages[0] = page(strp("https://elsewhere.example/v4/series/423432/episodes/official/zho?page=1"), ep(9001, 1, 1, "第一集的名字"), ep(9002, 1, 2, "二"))
	f.pages[1] = page(strp(""), ep(9003, 1, 3, "三"))
	f.pages[2] = page(nil, ep(9004, 1, 4, "不该取到"))
	got, err := f.client(t, Options{}).Episodes(context.Background(), 423432, "zho")
	require.NoError(t, err)
	assert.Equal(t, "zho", got.OriginalLanguage)
	require.Len(t, got.Episodes, 3)
	e := got.Episodes[0]
	assert.Equal(t, Episode{
		ID: 9001, Season: 1, Number: 1, Name: "第一集的名字", Overview: "简介1", Aired: "2026-10-01",
		Image: "https://artworks.thetvdb.com/banners/v4/episode/9001/screencap/a.jpg", NameLangs: []string{"zho", "eng"}, OverviewLangs: []string{"zho"},
	}, e)
	_, reqs := f.stats()
	assert.Equal(t, []string{"/v4/series/423432/episodes/official/zho?page=0", "/v4/series/423432/episodes/official/zho?page=1"}, reqs)
}

// 顶层没有 links：这一页有没见过的集就接着翻；一页里全是见过的、或者没有集时停。
func TestEpisodesPagesWithoutLinks(t *testing.T) {
	f := newFake(t)
	f.pages[0] = page(nil, ep(9001, 1, 1, "一"), ep(9002, 1, 2, "二"))
	f.pages[1] = page(nil, ep(9003, 1, 3, "三"))
	f.pages[2] = page(nil, ep(9003, 1, 3, "三"))
	got, err := f.client(t, Options{}).Episodes(context.Background(), 423432, "zho")
	require.NoError(t, err)
	assert.Len(t, got.Episodes, 3)
	_, reqs := f.stats()
	assert.Len(t, reqs, 3, "第 2 页没有新的集，停")

	g := newFake(t)
	g.pages[0] = page(nil, ep(9001, 1, 1, "一"))
	g.pages[1] = page(nil)
	got, err = g.client(t, Options{}).Episodes(context.Background(), 423432, "zho")
	require.NoError(t, err)
	assert.Len(t, got.Episodes, 1)
	_, reqs = g.stats()
	assert.Len(t, reqs, 2, "空页停")
}

// 带语言的与不带语言的列表第 0 页都 404 是没有这部（空结果，也缓存）；后面的页 404 就停。
func TestEpisodesNotFound(t *testing.T) {
	f := newFake(t)
	cache := newCache()
	c := f.client(t, Options{Cache: cache})
	got, err := c.Episodes(context.Background(), 777, "zho")
	require.NoError(t, err)
	assert.Empty(t, got.Episodes)
	got, err = c.Episodes(context.Background(), 777, "zho")
	require.NoError(t, err)
	assert.Empty(t, got.Episodes)
	_, reqs := f.stats()
	assert.Equal(t, []string{"/v4/series/777/episodes/official/zho?page=0", "/v4/series/777/episodes/official?page=0"}, reqs, "没有这部也缓存")

	g := newFake(t)
	g.pages[0] = page(nil, ep(9001, 1, 1, "一"))
	got, err = g.client(t, Options{}).Episodes(context.Background(), 423432, "zho")
	require.NoError(t, err)
	assert.Len(t, got.Episodes, 1)
}

// 带语言的列表回 404（剧集明明在，thetvdb/v4-api#324）：改取不带语言的列表，标成 Untranslated。
func TestEpisodesUntranslatedFallback(t *testing.T) {
	f := newFake(t)
	f.basePages[0] = `{"data":{"series":{"id":423432,"originalLanguage":"jpn"},"episodes":[` + ep(9001, 1, 1, "第一話") + `]},"links":{"next":null}}`
	got, err := f.client(t, Options{Cache: newCache()}).Episodes(context.Background(), 423432, "zho")
	require.NoError(t, err)
	assert.True(t, got.Untranslated)
	assert.Equal(t, "jpn", got.OriginalLanguage)
	require.Len(t, got.Episodes, 1)
	assert.Equal(t, "https://artworks.thetvdb.com/banners/v4/episode/9001/screencap/a.jpg", got.Episodes[0].Image)
}

func TestEpisodesTooMany(t *testing.T) {
	f := newFake(t)
	for i := range MaxPages + 1 {
		f.pages[i] = page(strp("more"), ep(9000+i, 1, i+1, "集"))
	}
	_, err := f.client(t, Options{}).Episodes(context.Background(), 423432, "zho")
	require.ErrorIs(t, err, ErrTooMany)
	_, reqs := f.stats()
	assert.Len(t, reqs, MaxPages)
}

func TestEpisodesArgs(t *testing.T) {
	c, err := New(Options{APIKey: testKey})
	require.NoError(t, err)
	_, err = c.Episodes(context.Background(), 0, "zho")
	require.Error(t, err)
	for _, bad := range []string{"", "zh", "ZHO", "zho/../x", "zh-CN"} {
		_, err = c.Episodes(context.Background(), 1, bad)
		require.Error(t, err, bad)
	}
}

// 每页缓存；显式重新刮削（cachectl.Bypass）时不读缓存，取到的照样写进缓存。
func TestEpisodesCache(t *testing.T) {
	f := newFake(t)
	f.pages[0] = page(nil, ep(9001, 1, 1, "一"))
	cache := newCache()
	c := f.client(t, Options{Cache: cache})
	_, err := c.Episodes(context.Background(), 423432, "zho")
	require.NoError(t, err)
	_, err = c.Episodes(context.Background(), 423432, "zho")
	require.NoError(t, err)
	_, reqs := f.stats()
	assert.Len(t, reqs, 2, "第 0 页与空的第 1 页各一次")
	assert.Contains(t, cache.m, "tvdb:v1:episodes:423432:official:zho:0")

	f.mu.Lock()
	f.pages[0] = page(nil, ep(9001, 1, 1, "改过的"))
	f.mu.Unlock()
	got, err := c.Episodes(cachectl.Bypass(context.Background()), 423432, "zho")
	require.NoError(t, err)
	assert.Equal(t, "改过的", got.Episodes[0].Name)
	got, err = c.Episodes(context.Background(), 423432, "zho")
	require.NoError(t, err)
	assert.Equal(t, "改过的", got.Episodes[0].Name, "新的写进了缓存")
}

// 令牌在同一个令牌表里共用；数据请求 401 时重新登录一次再试，还是 401 就是 ErrUnauthorized。
func TestTokenReuseAndRelogin(t *testing.T) {
	f := newFake(t)
	f.pages[0] = page(nil)
	tokens := NewTokens()
	for range 2 {
		_, err := f.client(t, Options{Tokens: tokens}).Episodes(context.Background(), 423432, "zho")
		require.NoError(t, err)
	}
	logins, _ := f.stats()
	assert.Equal(t, 1, logins, "两个客户端共用令牌")

	f.mu.Lock()
	f.reject = 1
	f.mu.Unlock()
	_, err := f.client(t, Options{Tokens: tokens}).Episodes(context.Background(), 423432, "zho")
	require.NoError(t, err)
	logins, _ = f.stats()
	assert.Equal(t, 2, logins, "401 以后重新登录一次")

	f.mu.Lock()
	f.reject = 100
	f.mu.Unlock()
	_, err = f.client(t, Options{Tokens: tokens}).Episodes(context.Background(), 423432, "zho")
	require.ErrorIs(t, err, ErrUnauthorized)
	logins, _ = f.stats()
	assert.Equal(t, 3, logins, "只重新登录一次")
	assert.NotContains(t, err.Error(), testToken)

	// 令牌过期：重新登录
	now := time.Now()
	g := newFake(t)
	g.pages[0] = page(nil)
	tk := NewTokens()
	clock := func() time.Time { return now }
	_, err = g.client(t, Options{Tokens: tk, Now: clock}).Episodes(context.Background(), 423432, "zho")
	require.NoError(t, err)
	now = now.Add(26 * 24 * time.Hour)
	_, err = g.client(t, Options{Tokens: tk, Now: clock}).Episodes(cachectl.Bypass(context.Background()), 423432, "zho")
	require.NoError(t, err)
	logins, _ = g.stats()
	assert.Equal(t, 2, logins)
}

func TestDataErrors(t *testing.T) {
	for status, want := range map[int]error{http.StatusTooManyRequests: ErrRateLimited, http.StatusInternalServerError: ErrUnavailable} {
		f := newFake(t)
		f.dataStatus = status
		_, err := f.client(t, Options{}).Episodes(context.Background(), 423432, "zho")
		require.ErrorIs(t, err, want)
	}
}

// 接口请求跳到别的主机时不跟（请求头里有令牌）。
func TestNoCrossOriginRedirect(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("跟到了别的主机：%s", r.URL)
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v4/login" {
			_, _ = w.Write([]byte(`{"data":{"token":"t"}}`))
			return
		}
		http.Redirect(w, r, other.URL+"/steal", http.StatusFound)
	}))
	defer srv.Close()
	c, err := New(Options{APIKey: testKey, BaseURL: srv.URL + "/v4", Limiter: NewLimiter(1000)})
	require.NoError(t, err)
	_, err = c.Episodes(context.Background(), 1, "zho")
	require.ErrorIs(t, err, ErrUnavailable)
}

func TestImageURL(t *testing.T) {
	c, err := New(Options{APIKey: testKey})
	require.NoError(t, err)
	assert.Equal(t, "", c.ImageURL(" "))
	assert.Equal(t, "https://artworks.thetvdb.com/banners/a.jpg", c.ImageURL("/banners/a.jpg"))
	assert.Equal(t, "https://artworks.thetvdb.com/banners/a.jpg", c.ImageURL("http://artworks.thetvdb.com/banners/a.jpg"))
	assert.True(t, c.allowedImage("https://artworks.thetvdb.com/banners/a.jpg"))
	assert.True(t, c.allowedImage("https://thetvdb.com/a.jpg"))
	for _, bad := range []string{"http://artworks.thetvdb.com/a.jpg", "https://thetvdb.com.evil.example/a.jpg", "https://u:p@artworks.thetvdb.com/a.jpg", "ftp://artworks.thetvdb.com/a.jpg", "https://example.com/a.jpg"} {
		assert.False(t, c.allowedImage(bad), bad)
	}
}

// 下载图片不带令牌；只认允许的主机与 image/*。
func TestDownload(t *testing.T) {
	f := newFake(t)
	f.pages[0] = page(nil)
	c := f.client(t, Options{})
	_, err := c.Episodes(context.Background(), 423432, "zho")
	require.NoError(t, err)
	b, err := c.Download(context.Background(), f.srv.URL+"/banners/v4/episode/1/a.jpg")
	require.NoError(t, err)
	assert.Equal(t, "JPG/banners/v4/episode/1/a.jpg", string(b))
	assert.Equal(t, 0, f.authOnImg, "图片请求不带令牌")
	_, err = c.Download(context.Background(), f.srv.URL+"/banners/x.html")
	require.ErrorIs(t, err, ErrNotImage)
	_, err = c.Download(context.Background(), "https://example.com/a.jpg")
	require.Error(t, err)
	_, err = c.Download(context.Background(), "/banners/a.jpg")
	require.Error(t, err, "补成 artworks.thetvdb.com 以后不在允许的主机里")
}

// Validate 不用内存里的令牌，重新登录一次。
func TestValidate(t *testing.T) {
	f := newFake(t)
	f.pages[0] = page(nil)
	tokens := NewTokens()
	_, err := f.client(t, Options{Tokens: tokens}).Episodes(context.Background(), 423432, "zho")
	require.NoError(t, err)
	require.NoError(t, f.client(t, Options{Tokens: tokens}).Validate(context.Background()))
	logins, reqs := f.stats()
	assert.Equal(t, 2, logins)
	assert.Equal(t, "/v4/series/81189", reqs[len(reqs)-1])
	err = f.client(t, Options{APIKey: "bad-key-000", Tokens: tokens}).Validate(context.Background())
	require.ErrorIs(t, err, ErrUnauthorized)
}
