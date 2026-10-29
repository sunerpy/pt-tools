package v2

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"weak"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
 * BYR 登录后是 auth_token（JWT，15 分钟）、refresh_token、session_id：auth_token 过期后站点在下一个请求里换发一对新的，
 * refresh_token 也换（2026-10-29 实测）。pt-tools 以前一直发存着的那组、不理 Set-Cookie，续过一次以后就拿旧令牌掉线了。
 */

func TestMergeSessionCookies(t *testing.T) {
	now := time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)
	names := []string{"auth_token", "refresh_token", "session_id"}
	header := func(lines ...string) http.Header {
		h := http.Header{}
		for _, l := range lines {
			h.Add("Set-Cookie", l)
		}
		return h
	}
	cookie := "auth_token=A0; refresh_token=R0; session_id=S; theme=dark"

	got, changed := mergeSessionCookies(cookie, header(
		"auth_token=A1; Path=/; Domain=.byr.pt; Max-Age=2592000; HttpOnly; Secure",
		"refresh_token=R1; Path=/; Domain=.byr.pt; Expires=Sat, 28 Nov 2026 08:00:00 GMT; HttpOnly",
		"session_id=S; Path=/; Max-Age=2592000",
		"tracker=x; Path=/",
	), names, now)
	assert.True(t, changed)
	assert.Equal(t, "auth_token=A1; refresh_token=R1; session_id=S; theme=dark", got, "只换列着的、顺序不变，别的 Cookie 原样")

	_, changed = mergeSessionCookies(cookie, header("session_id=S; Path=/"), names, now)
	assert.False(t, changed, "值没变不算换")

	got, changed = mergeSessionCookies("session_id=S", header("auth_token=A1; Path=/"), names, now)
	assert.True(t, changed)
	assert.Equal(t, "session_id=S; auth_token=A1", got, "原来没有的接在后面")

	for _, del := range []string{
		"auth_token=; Path=/",
		"auth_token=deleted; Expires=Thu, 01 Jan 1970 00:00:01 GMT; Max-Age=0",
		"auth_token=gone; Max-Age=-1",
		"auth_token=old; Expires=Wed, 28 Oct 2026 08:00:00 GMT",
	} {
		_, changed = mergeSessionCookies(cookie, header(del), names, now)
		assert.False(t, changed, "站点删 Cookie 不照做：%s", del)
	}
}

// firstValues 按 PHP 读 $_COOKIE 的方式取每个名字的第一个值：请求里同名 Cookie 出现两次时站点认的是前一个。
// （SiteHTTPClient 不走代理时共用一个 Session，它的 Cookie jar 会把站点设过的 Cookie 接在请求头后面。）
func firstValues(header string) string {
	seen := map[string]bool{}
	var out []string
	for _, part := range strings.Split(header, ";") {
		name, _, _ := strings.Cut(strings.TrimSpace(part), "=")
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, strings.TrimSpace(part))
	}
	return strings.Join(out, "; ")
}

// fakeRenewingSite 第一次请求换发 A1/R1；记下每个请求带的 Cookie（按站点认的那样取）和同时在处理的请求数。
type fakeRenewingSite struct {
	mu       sync.Mutex
	cookies  []string
	inFlight atomic.Int32
	maxSeen  atomic.Int32
	renewed  bool
}

func (f *fakeRenewingSite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n := f.inFlight.Add(1)
	defer f.inFlight.Add(-1)
	for {
		m := f.maxSeen.Load()
		if n <= m || f.maxSeen.CompareAndSwap(m, n) {
			break
		}
	}
	time.Sleep(20 * time.Millisecond)
	f.mu.Lock()
	f.cookies = append(f.cookies, firstValues(r.Header.Get("Cookie")))
	first := !f.renewed
	f.renewed = true
	f.mu.Unlock()
	if first {
		w.Header().Add("Set-Cookie", "auth_token=A1; Path=/; Max-Age=2592000; HttpOnly")
		w.Header().Add("Set-Cookie", "refresh_token=R1; Path=/; Max-Age=2592000; HttpOnly")
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<html><head><title>首页</title></head><body><a href="userdetails.php?id=1">me</a></body></html>`))
}

const (
	cookieA0 = "auth_token=A0; refresh_token=R0; session_id=S"
	cookieA1 = "auth_token=A1; refresh_token=R1; session_id=S"
)

// resetSessionPool 清空按 Cookie 共用的会话：每个测试从没有会话开始，测完也清掉，免得测试之间串会话。
func resetSessionPool(t *testing.T) {
	t.Helper()
	wipe := func() {
		sessionPool.mu.Lock()
		defer sessionPool.mu.Unlock()
		sessionPool.m = map[string]weak.Pointer[sessionCookies]{}
	}
	wipe()
	t.Cleanup(wipe)
}

func renewingDriverWith(t *testing.T, baseURL, cookie string) *NexusPHPDriver {
	t.Helper()
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: baseURL, Cookie: cookie, SiteID: "byrpt"})
	d.SetSiteDefinition(&SiteDefinition{ID: "byrpt", SessionCookies: []string{"auth_token", "refresh_token", "session_id"}})
	return d
}

func renewingDriver(t *testing.T, baseURL string) *NexusPHPDriver {
	t.Helper()
	return renewingDriverWith(t, baseURL, cookieA0)
}

func TestNexusPHPDriver_RenewingSession(t *testing.T) {
	fake := &fakeRenewingSite{}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)

	resetSessionPool(t)
	type saved struct {
		chain   []string
		renewed string
	}
	var (
		mu    sync.Mutex
		saves []saved
	)
	SetSessionCookieSink(func(siteID string, chain []string, renewed string) (bool, error) {
		assert.Equal(t, "byrpt", siteID)
		mu.Lock()
		defer mu.Unlock()
		saves = append(saves, saved{chain, renewed})
		return true, nil
	})
	t.Cleanup(func() { SetSessionCookieSink(nil) })

	d := renewingDriver(t, srv.URL)
	ctx := context.Background()
	_, err := d.Execute(ctx, NexusPHPRequest{Path: "/index.php"})
	require.NoError(t, err)
	_, err = d.Execute(ctx, NexusPHPRequest{Path: "/userdetails.php"})
	require.NoError(t, err)

	fake.mu.Lock()
	require.Len(t, fake.cookies, 2)
	assert.Equal(t, "auth_token=A0; refresh_token=R0; session_id=S", fake.cookies[0])
	assert.Equal(t, "auth_token=A1; refresh_token=R1; session_id=S", fake.cookies[1], "第二个请求用换发后的令牌")
	fake.mu.Unlock()
	mu.Lock()
	assert.Equal(t, []saved{{[]string{cookieA0, cookieA1}, cookieA1}}, saves, "交给 sink 的是会话用过的那些和新的")
	mu.Unlock()
	cookie, ok := d.CurrentCookie()
	assert.True(t, ok)
	assert.Equal(t, "auth_token=A1; refresh_token=R1; session_id=S", cookie)

	// 站点套上 BaseSite 也取得到现在用的那组；没有续期会话的站点取不到
	site := NewBaseSite(d, BaseSiteConfig{ID: "byrpt", Name: "BYRPT", Kind: SiteNexusPHP})
	got, ok := CurrentSessionCookie(site)
	assert.True(t, ok)
	assert.Equal(t, cookie, got)
	plain := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: srv.URL, Cookie: "c_secure_uid=1"})
	_, ok = CurrentSessionCookie(NewBaseSite(plain, BaseSiteConfig{ID: "hdsky"}))
	assert.False(t, ok)
}

// 同一实例的请求一个接一个发：两个请求不会拿同一个旧 refresh_token 各续一次。没有续期会话的站点照旧并发。
func TestNexusPHPDriver_RenewingSessionSerializes(t *testing.T) {
	resetSessionPool(t)
	fake := &fakeRenewingSite{}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)

	d := renewingDriver(t, srv.URL)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			_, err := d.Execute(context.Background(), NexusPHPRequest{Path: "/index.php"})
			assert.NoError(t, err)
		})
	}
	wg.Wait()
	assert.Equal(t, int32(1), fake.maxSeen.Load())
	fake.mu.Lock()
	for _, c := range fake.cookies[1:] {
		assert.True(t, strings.Contains(c, "refresh_token=R1"), "第一个之后都用换发后的：%s", c)
	}
	fake.mu.Unlock()
}

// 设置里的 Cookie 已经被别的换掉（存的方法回 false）：内存里照样用新的，下一次交给 sink 的还是这个会话用过的全部。
func TestNexusPHPDriver_RenewingSessionNotStored(t *testing.T) {
	resetSessionPool(t)
	var chains [][]string
	SetSessionCookieSink(func(_ string, chain []string, _ string) (bool, error) {
		chains = append(chains, chain)
		return false, nil
	})
	t.Cleanup(func() { SetSessionCookieSink(nil) })
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := int(n.Add(1))
		w.Header().Add("Set-Cookie", "auth_token=A"+strings.Repeat("x", n)+"; Path=/")
		_, _ = w.Write([]byte(`<html><body><a href="userdetails.php?id=1">me</a></body></html>`))
	}))
	t.Cleanup(srv.Close)
	d := renewingDriver(t, srv.URL)
	for range 2 {
		_, err := d.Execute(context.Background(), NexusPHPRequest{Path: "/index.php"})
		require.NoError(t, err)
	}
	assert.Equal(t, [][]string{
		{cookieA0, "auth_token=Ax; refresh_token=R0; session_id=S"},
		{cookieA0, "auth_token=Ax; refresh_token=R0; session_id=S", "auth_token=Axx; refresh_token=R0; session_id=S"},
	}, chains)
	cookie, _ := d.CurrentCookie()
	assert.Equal(t, "auth_token=Axx; refresh_token=R0; session_id=S", cookie)
}

// 下载种子（站点自己的域名）也排队、用换发后的 Cookie；外域链接不带。
func TestNexusPHPDriver_RenewingSessionDownload(t *testing.T) {
	resetSessionPool(t)
	fake := &fakeRenewingSite{}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	other := &fakeRenewingSite{renewed: true}
	external := httptest.NewServer(other)
	t.Cleanup(external.Close)
	d := renewingDriver(t, srv.URL)
	_, err := d.Execute(context.Background(), NexusPHPRequest{Path: "/index.php"})
	require.NoError(t, err)
	_, err = d.fetchTorrentOnce(context.Background(), srv.URL+"/download.php?id=1", map[string]string{"Accept": "*/*"}, true)
	require.NoError(t, err)
	// 外域：换成 localhost 访问另一个服务，Cookie jar 按主机名分开，不会把站点的 Cookie 带过去
	externalURL := strings.Replace(external.URL, "127.0.0.1", "localhost", 1)
	_, err = d.fetchTorrentOnce(context.Background(), externalURL+"/download.php?id=2", map[string]string{"Accept": "*/*"}, false)
	require.NoError(t, err)
	fake.mu.Lock()
	require.Len(t, fake.cookies, 2)
	assert.Equal(t, "auth_token=A1; refresh_token=R1; session_id=S", fake.cookies[1])
	fake.mu.Unlock()
	other.mu.Lock()
	defer other.mu.Unlock()
	require.Len(t, other.cookies, 1)
	assert.Empty(t, other.cookies[0], "外域链接不带 Cookie")
}

// 一个站点同时有好几个实例（用户信息与搜索共用的、登录探测和 RSS 取详情时每次新建的）：拿着同一串会话里任一组 Cookie 建的实例
// 共用一个会话，排同一个队、用同一组令牌。各管各的话，一个实例续了期，别的实例还拿旧的 refresh_token，站点只认最新的就都掉线。
// 谁都没用过的 Cookie（扩展刚推来的、验证时的候选）另起一个会话，在用的那个不受影响。
func TestNexusPHPDriver_RenewingSessionSharedAcrossInstances(t *testing.T) {
	resetSessionPool(t)
	fake := &fakeRenewingSite{}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	ctx := context.Background()

	main := renewingDriver(t, srv.URL)
	_, err := main.Execute(ctx, NexusPHPRequest{Path: "/index.php"})
	require.NoError(t, err)

	// 换发之前从设置里读的旧 Cookie 建的实例（RSS 取详情）、存回去以后读的新 Cookie 建的实例（登录探测）：都接进同一个会话
	stale := renewingDriverWith(t, srv.URL, cookieA0)
	fresh := renewingDriverWith(t, srv.URL, cookieA1)
	assert.Same(t, main.session, stale.session)
	assert.Same(t, main.session, fresh.session)
	got, _ := stale.CurrentCookie()
	assert.Equal(t, cookieA1, got, "旧 Cookie 建的实例也用换发后的令牌")

	// 扩展刚推来、谁都没用过的一组：另起会话，不把在用的会话换掉
	pushed := "auth_token=P0; refresh_token=PR0; session_id=S2"
	other := renewingDriverWith(t, srv.URL, pushed)
	assert.NotSame(t, main.session, other.session)

	var wg sync.WaitGroup
	for _, d := range []*NexusPHPDriver{main, stale, fresh, main, stale, fresh} {
		wg.Go(func() {
			_, execErr := d.Execute(ctx, NexusPHPRequest{Path: "/index.php"})
			assert.NoError(t, execErr)
		})
	}
	wg.Wait()
	_, err = other.Execute(ctx, NexusPHPRequest{Path: "/index.php"})
	require.NoError(t, err)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Len(t, fake.cookies, 8)
	assert.Equal(t, cookieA0, fake.cookies[0])
	for _, c := range fake.cookies[1:7] {
		assert.Equal(t, cookieA1, c, "共用会话的实例都发换发后的那组")
	}
	assert.Equal(t, pushed, fake.cookies[7], "新推来的那组照原样发")
	got, _ = main.CurrentCookie()
	assert.Equal(t, cookieA1, got)
}

// 共用的会话之间也排队：几个实例同时发请求，站点那边同一时间只有一个。
func TestNexusPHPDriver_RenewingSessionSerializesAcrossInstances(t *testing.T) {
	resetSessionPool(t)
	fake := &fakeRenewingSite{}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	drivers := []*NexusPHPDriver{renewingDriver(t, srv.URL), renewingDriver(t, srv.URL), renewingDriver(t, srv.URL)}
	var wg sync.WaitGroup
	for i := range 6 {
		d := drivers[i%len(drivers)]
		wg.Go(func() {
			_, err := d.Execute(context.Background(), NexusPHPRequest{Path: "/index.php"})
			assert.NoError(t, err)
		})
	}
	wg.Wait()
	assert.Equal(t, int32(1), fake.maxSeen.Load())
}

// 会话只记最近 sessionHistoryLimit 组：更早的那组再拿来建实例就另起一个会话，最近的照样接进来。
func TestSessionCookiesHistoryLimit(t *testing.T) {
	resetSessionPool(t)
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Add("Set-Cookie", fmt.Sprintf("auth_token=T%d; Path=/", n.Add(1)))
		_, _ = w.Write([]byte(`<html><body><a href="userdetails.php?id=1">me</a></body></html>`))
	}))
	t.Cleanup(srv.Close)
	d := renewingDriver(t, srv.URL)
	for range sessionHistoryLimit + 6 {
		_, err := d.Execute(context.Background(), NexusPHPRequest{Path: "/index.php"})
		require.NoError(t, err)
	}
	d.session.mu.Lock()
	assert.Len(t, d.session.history, sessionHistoryLimit)
	d.session.mu.Unlock()
	latest, _ := d.CurrentCookie()
	assert.Same(t, d.session, joinSessionCookies("byrpt", d.session.names, latest))
	assert.NotSame(t, d.session, joinSessionCookies("byrpt", d.session.names, cookieA0), "忘掉的那组另起会话")
}

//go:noinline
func renewedThenDropped(t *testing.T) {
	s := joinSessionCookies("byrpt", []string{"auth_token"}, cookieA0)
	s.mu.Lock()
	s.cookie = cookieA1
	s.remember(cookieA1)
	s.mu.Unlock()
}

// 会话只由站点实例持有：实例都没了，会话回收，再拿那组 Cookie 建实例是新会话（不会接到早已不用的那个上）。
func TestSessionCookiesReleasedWithInstances(t *testing.T) {
	resetSessionPool(t)
	renewedThenDropped(t)
	runtime.GC()
	s := joinSessionCookies("byrpt", []string{"auth_token"}, cookieA0)
	assert.Equal(t, cookieA0, s.current(), "原来那个会话已经回收")
}
