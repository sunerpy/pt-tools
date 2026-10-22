package v2

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

const (
	partialIndexPage   = `<html><body><a id="uid" href="userdetails.php?id=888">tester</a><span id="seed">7</span></body></html>`
	partialDetailsPage = `<html><body><span id="up">2.00 TB</span><span id="down">1.00 TB</span><span id="bonus">1,234.5</span></body></html>`
	partialBonusPage   = `<html><body><span id="bph">12.5</span></body></html>`
	challengePage      = `<html><head><title>Just a moment...</title></head><body><script>window._cf_chl_opt={}</script></body></html>`
	loginPage          = `<html><head><title>登录</title></head><body><form action="takelogin.php"><input name="username"><input name="password"></form></body></html>`
)

// partialDefinition 是三页的站点定义：index（用户名、ID、做种数）与 mybonus（时魔）不依赖别的页，userdetails 要用户 ID。
func partialDefinition(delayMS int) *SiteDefinition {
	return &SiteDefinition{
		ID: "testsite",
		UserInfo: &UserInfoConfig{
			RequestDelay: delayMS,
			Process: []UserInfoProcess{
				{RequestConfig: RequestConfig{URL: "/index.php", ResponseType: "document"}, Fields: []string{"id", "name", "seeding"}},
				{
					RequestConfig: RequestConfig{URL: "/userdetails.php", ResponseType: "document"},
					Assertion:     map[string]string{"id": "params.id"},
					Fields:        []string{"uploaded", "downloaded", "bonus"},
				},
				{RequestConfig: RequestConfig{URL: "/mybonus.php", ResponseType: "document"}, Fields: []string{"bonusPerHour"}},
			},
			Selectors: map[string]FieldSelector{
				"id":           {Selector: []string{"#uid"}, Attr: "href", Filters: []Filter{{Name: "querystring", Args: []any{"id"}}}},
				"name":         {Selector: []string{"#uid"}},
				"seeding":      {Selector: []string{"#seed"}},
				"uploaded":     {Selector: []string{"#up"}},
				"downloaded":   {Selector: []string{"#down"}},
				"bonus":        {Selector: []string{"#bonus"}},
				"bonusPerHour": {Selector: []string{"#bph"}},
				// 定义里写了做种体积：不另外去取做种列表
				"seedingSize": {Selector: []string{"#none"}},
			},
		},
	}
}

// partialServer 是假站点：page 决定每一页（按第几次请求）怎么回；记下每一页被请求了几次与同时最多几个请求。
type partialServer struct {
	*httptest.Server
	mu       sync.Mutex
	hits     map[string]int
	order    []string
	inFlight atomic.Int32
	maxIn    atomic.Int32
}

func newPartialServer(t *testing.T, page func(path string, n int, w http.ResponseWriter) bool) *partialServer {
	t.Helper()
	ps := &partialServer{hits: map[string]int{}}
	ps.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := ps.inFlight.Add(1)
		defer ps.inFlight.Add(-1)
		for {
			old := ps.maxIn.Load()
			if cur <= old || ps.maxIn.CompareAndSwap(old, cur) {
				break
			}
		}
		ps.mu.Lock()
		ps.hits[r.URL.Path]++
		n := ps.hits[r.URL.Path]
		ps.order = append(ps.order, r.URL.Path)
		ps.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		if page(r.URL.Path, n, w) {
			return
		}
		switch r.URL.Path {
		case "/index.php":
			_, _ = w.Write([]byte(partialIndexPage))
		case "/userdetails.php":
			_, _ = w.Write([]byte(partialDetailsPage))
		case "/mybonus.php":
			_, _ = w.Write([]byte(partialBonusPage))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(ps.Close)
	return ps
}

func (ps *partialServer) count(path string) int {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return ps.hits[path]
}

// resetConn 掐断连接（客户端看到 connection reset / EOF）。
func resetConn(w http.ResponseWriter) {
	hj, _ := w.(http.Hijacker)
	conn, _, err := hj.Hijack()
	if err != nil {
		return
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetLinger(0)
	}
	_ = conn.Close()
}

func challenge(w http.ResponseWriter) {
	w.Header().Set("Cf-Mitigated", "challenge")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(challengePage))
}

func partialDriver(t *testing.T, ps *partialServer, delayMS int) *NexusPHPDriver {
	t.Helper()
	fastUserInfoRetries(t)
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: ps.URL, Cookie: "c=1"})
	d.SetSiteDefinition(partialDefinition(delayMS))
	return d
}

// 连接被重置一次：再试一次就取到了，整次获取没有缺字段
func TestUserInfo_RetriesResetConnection(t *testing.T) {
	ps := newPartialServer(t, func(path string, n int, w http.ResponseWriter) bool {
		if path == "/userdetails.php" && n == 1 {
			resetConn(w)
			return true
		}
		return false
	})
	info, err := partialDriver(t, ps, 0).GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, ps.count("/userdetails.php"), "掐断以后再试了一次")
	assert.Equal(t, int64(2*1024*1024*1024*1024), info.Uploaded)
	assert.Equal(t, 12.5, info.BonusPerHour)
}

// 一直连不上的页：最多再试两次；用户名到手了，回取到的部分，写明缺哪些字段
func TestUserInfo_PersistentResetIsPartial(t *testing.T) {
	ps := newPartialServer(t, func(path string, _ int, w http.ResponseWriter) bool {
		if path == "/mybonus.php" {
			resetConn(w)
			return true
		}
		return false
	})
	info, err := partialDriver(t, ps, 0).GetUserInfo(context.Background())
	var partial *PartialUserInfoError
	require.ErrorAs(t, err, &partial)
	assert.Equal(t, []string{"bonusPerHour"}, partial.Missing)
	assert.ErrorIs(t, err, ErrNetworkError)
	assert.Equal(t, 1+userInfoNetRetries, ps.count("/mybonus.php"))
	assert.Equal(t, "tester", info.Username)
	assert.Equal(t, int64(2*1024*1024*1024*1024), info.Uploaded, "其余的页照常")
}

// Cloudflare 质询：隔一会儿再试一次；过了就照常，还在就只少这一页（HDSky 的 phase 2 质询让整次失败的问题）
func TestUserInfo_ChallengeRetriedOnceThenPartial(t *testing.T) {
	ps := newPartialServer(t, func(path string, n int, w http.ResponseWriter) bool {
		if path == "/userdetails.php" && n == 1 {
			challenge(w)
			return true
		}
		return false
	})
	_, err := partialDriver(t, ps, 0).GetUserInfo(context.Background())
	require.NoError(t, err, "第二次过了质询")
	assert.Equal(t, 2, ps.count("/userdetails.php"))

	ps = newPartialServer(t, func(path string, _ int, w http.ResponseWriter) bool {
		if path == "/userdetails.php" {
			challenge(w)
			return true
		}
		return false
	})
	info, err := partialDriver(t, ps, 0).GetUserInfo(context.Background())
	var partial *PartialUserInfoError
	require.ErrorAs(t, err, &partial)
	assert.ElementsMatch(t, []string{"uploaded", "downloaded", "bonus"}, partial.Missing)
	assert.ErrorIs(t, err, ErrCloudflareChallenge)
	assert.Equal(t, 2, ps.count("/userdetails.php"), "质询只再试一次")
	assert.Equal(t, "tester", info.Username)
	assert.Equal(t, 12.5, info.BonusPerHour)
}

// 几页都写了的字段（用户名在首页与详情页都有）：详情页没取到时不算缺，保存时也不拿上一次的盖掉这次取到的
func TestUserInfo_FieldFromAnotherPageNotMissing(t *testing.T) {
	ps := newPartialServer(t, func(path string, _ int, w http.ResponseWriter) bool {
		if path == "/userdetails.php" {
			challenge(w)
			return true
		}
		return false
	})
	def := partialDefinition(0)
	def.UserInfo.Process[1].Fields = append([]string{"name"}, def.UserInfo.Process[1].Fields...)
	d := partialDriver(t, ps, 0)
	d.SetSiteDefinition(def)
	info, err := d.GetUserInfo(context.Background())
	var partial *PartialUserInfoError
	require.ErrorAs(t, err, &partial)
	assert.ElementsMatch(t, []string{"uploaded", "downloaded", "bonus"}, partial.Missing, "用户名首页取到了，不算缺")
	assert.Equal(t, "tester", info.Username)

	merged, kept := mergeMissingUserInfo(info, UserInfo{Username: "old-name", Uploaded: 9, Downloaded: 3}, partial.Missing)
	assert.Equal(t, "tester", merged.Username, "这次取到的用户名不被上一次的盖掉")
	assert.ElementsMatch(t, []string{"uploaded", "downloaded", "bonus"}, kept)
}

// 取用户名的那一页过不了质询：整次失败，原因是质询（登录探测据此判为质询、可以走后备）
func TestUserInfo_ChallengeOnUsernamePageFails(t *testing.T) {
	ps := newPartialServer(t, func(path string, _ int, w http.ResponseWriter) bool {
		if path == "/index.php" {
			challenge(w)
			return true
		}
		return false
	})
	info, err := partialDriver(t, ps, 0).GetUserInfo(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCloudflareChallenge)
	var partial *PartialUserInfoError
	assert.False(t, errors.As(err, &partial))
	assert.Empty(t, info.Username)
}

// 登录失效：整次失败，不当成少一页
func TestUserInfo_SessionExpiredIsFatal(t *testing.T) {
	ps := newPartialServer(t, func(path string, _ int, w http.ResponseWriter) bool {
		if path == "/userdetails.php" {
			_, _ = w.Write([]byte(loginPage))
			return true
		}
		return false
	})
	_, err := partialDriver(t, ps, 0).GetUserInfo(context.Background())
	require.ErrorIs(t, err, ErrSessionExpired)
	assert.Equal(t, 1, ps.count("/userdetails.php"), "登录失效不重试")
}

// HTTP 出错的页：以前悄悄当成空页（统计里冒出 0），现在算缺的字段
func TestUserInfo_HTTPErrorIsPartial(t *testing.T) {
	ps := newPartialServer(t, func(path string, _ int, w http.ResponseWriter) bool {
		if path == "/userdetails.php" {
			w.WriteHeader(http.StatusBadGateway)
			return true
		}
		return false
	})
	_, err := partialDriver(t, ps, 0).GetUserInfo(context.Background())
	var partial *PartialUserInfoError
	require.ErrorAs(t, err, &partial)
	assert.ElementsMatch(t, []string{"uploaded", "downloaded", "bonus"}, partial.Missing)
	assert.Equal(t, 1+userInfoNetRetries, ps.count("/userdetails.php"), "站点出错（5xx）也再试")
	assert.Equal(t, "HTTP 502", FetchFailureReason(partial.Err))
}

// 定义里写了 RequestDelay 的站点一页一页地取（同时只有一个请求），先取不依赖别的页的；没写的并发
func TestUserInfo_RequestDelayIsSequential(t *testing.T) {
	ps := newPartialServer(t, func(string, int, http.ResponseWriter) bool { return false })
	start := time.Now()
	_, err := partialDriver(t, ps, 30).GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(1), ps.maxIn.Load(), "同时只有一个请求")
	assert.Equal(t, []string{"/index.php", "/mybonus.php", "/userdetails.php"}, ps.order)
	assert.GreaterOrEqual(t, time.Since(start), 60*time.Millisecond, "每两页之间隔 RequestDelay")

	ps = newPartialServer(t, func(string, int, http.ResponseWriter) bool {
		time.Sleep(30 * time.Millisecond)
		return false
	})
	_, err = partialDriver(t, ps, 0).GetUserInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(2), ps.maxIn.Load(), "没写 RequestDelay：第一轮的两页同时取")
}

// 剩下的时间不够时不再等着重试
func TestUserInfo_NoRetryPastDeadline(t *testing.T) {
	ps := newPartialServer(t, func(path string, _ int, w http.ResponseWriter) bool {
		if path == "/mybonus.php" {
			resetConn(w)
			return true
		}
		return false
	})
	d := partialDriver(t, ps, 0)
	userInfoRetryBase, userInfoMinAttempt = time.Second, 3*time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	_, err := d.GetUserInfo(ctx)
	var partial *PartialUserInfoError
	require.ErrorAs(t, err, &partial)
	assert.Less(t, time.Since(start), time.Second, "等了也来不及试：直接回")
	assert.Equal(t, 1, ps.count("/mybonus.php"))
}

// 一页一页地取的站点，调用方的时限在第二轮到了（生产里 SSD 的登录探测 60 秒用完）：用户名已经到手，回取到的部分，
// 没来得及取的页算缺，原因是超时；不是整次失败
func TestUserInfo_DeadlineInSequentialPhase2IsPartial(t *testing.T) {
	ps := newPartialServer(t, func(path string, _ int, w http.ResponseWriter) bool {
		if path == "/mybonus.php" {
			time.Sleep(400 * time.Millisecond)
		}
		return false
	})
	d := partialDriver(t, ps, 50)
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	info, err := d.GetUserInfo(ctx)
	var partial *PartialUserInfoError
	require.ErrorAs(t, err, &partial, "用户名到手了：不是整次失败")
	assert.ElementsMatch(t, []string{"bonusPerHour", "uploaded", "downloaded", "bonus"}, partial.Missing)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, "超时", FetchFailureReason(partial.Err))
	assert.Equal(t, "tester", info.Username)
	assert.Equal(t, 0, ps.count("/userdetails.php"), "时限到了以后不再发请求")
}

// 时限到了才回来的部分结果：保存时照样读上一次的记录补上缺的字段（不用已经过期的 ctx 去读）
func TestUserInfoService_PartialAfterDeadlineStillMerges(t *testing.T) {
	repo := NewInMemoryUserInfoRepo()
	require.NoError(t, repo.Save(context.Background(), UserInfo{Site: "ssd", Username: "tester", Uploaded: 100, Downloaded: 50, Ratio: 2}))
	service := NewUserInfoService(UserInfoServiceConfig{Repo: ctxRepo{repo}, Logger: zap.NewNop()})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	site := &MockSite{}
	site.On("ID").Return("ssd")
	site.On("GetUserInfo", ctx).Return(UserInfo{Site: "ssd", Username: "tester", Seeding: 3},
		fmt.Errorf("get user info: %w", &PartialUserInfoError{Missing: []string{"uploaded", "downloaded"}, Err: context.DeadlineExceeded}))
	service.RegisterSite(site)
	info, err := service.FetchAndSave(ctx, "ssd")
	require.NoError(t, err)
	assert.Equal(t, int64(100), info.Uploaded)
	assert.Equal(t, int64(50), info.Downloaded)
	assert.Equal(t, 3, info.Seeding)
}

// ctxRepo 和真实的数据库仓库一样：ctx 已经结束时读写都失败
type ctxRepo struct{ UserInfoRepo }

func (r ctxRepo) Get(ctx context.Context, site string) (UserInfo, error) {
	if err := ctx.Err(); err != nil {
		return UserInfo{}, err
	}
	return r.UserInfoRepo.Get(ctx, site)
}

func (r ctxRepo) Save(ctx context.Context, info UserInfo) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.UserInfoRepo.Save(ctx, info)
}

// 保存时缺的字段沿用上一次的（最近访问时间不沿用）；上传、下载量缺了又没有上一次的时不保存；只记一行 WARN
func TestUserInfoService_PartialKeepsPreviousValues(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	repo := NewInMemoryUserInfoRepo()
	service := NewUserInfoService(UserInfoServiceConfig{Repo: repo, Logger: zap.New(core)})
	ctx := context.Background()
	require.NoError(t, repo.Save(ctx, UserInfo{Site: "hdsky", Username: "tester", Uploaded: 100, Downloaded: 50, Ratio: 2, Bonus: 5, BonusPerHour: 1.5, LastAccess: 1700000000}))

	reason := fmt.Errorf("HTTP 403: %w", ErrCloudflareChallenge)
	site := &MockSite{}
	site.On("ID").Return("hdsky")
	site.On("GetUserInfo", ctx).Return(UserInfo{Site: "hdsky", Username: "tester", Seeding: 9},
		fmt.Errorf("get user info: %w", &PartialUserInfoError{Missing: []string{"uploaded", "downloaded", "bonus", "lastAccessAt"}, Err: reason})).Once()
	service.RegisterSite(site)

	info, err := service.FetchAndSave(ctx, "hdsky")
	require.NoError(t, err)
	assert.Equal(t, int64(100), info.Uploaded, "沿用上一次的")
	assert.Equal(t, int64(50), info.Downloaded)
	assert.Equal(t, 2.0, info.Ratio, "上传、下载都沿用时分享率也沿用")
	assert.Equal(t, 5.0, info.Bonus)
	assert.Zero(t, info.BonusPerHour, "这次取到的页面上没有它、也没说缺：按这次的")
	assert.Zero(t, info.LastAccess, "最近访问时间按这次访问算，不沿用")
	assert.Equal(t, 9, info.Seeding, "这次取到的照用")
	saved, err := repo.Get(ctx, "hdsky")
	require.NoError(t, err)
	assert.Equal(t, int64(100), saved.Uploaded)
	entries := logs.All()
	require.Len(t, entries, 1)
	assert.Equal(t, "Partial user info, kept previous values", entries[0].Message)
	assert.Equal(t, "Cloudflare 质询", entries[0].ContextMap()["reason"])

	// 新站点：没有上一次的，上传量又缺了：不保存，报错
	fresh := &MockSite{}
	fresh.On("ID").Return("newsite")
	fresh.On("GetUserInfo", ctx).Return(UserInfo{Site: "newsite", Username: "u"}, &PartialUserInfoError{Missing: []string{"uploaded"}, Err: reason})
	service.RegisterSite(fresh)
	_, err = service.FetchAndSave(ctx, "newsite")
	require.Error(t, err)
	_, err = repo.Get(ctx, "newsite")
	assert.ErrorIs(t, err, ErrSiteNotFound, "没保存")

	// 只缺时魔：照样保存
	minor := &MockSite{}
	minor.On("ID").Return("minor")
	minor.On("GetUserInfo", ctx).Return(UserInfo{Site: "minor", Username: "u", Uploaded: 7}, &PartialUserInfoError{Missing: []string{"bonusPerHour"}, Err: reason})
	service.RegisterSite(minor)
	info, err = service.FetchAndSave(ctx, "minor")
	require.NoError(t, err)
	assert.Equal(t, int64(7), info.Uploaded)
}

// 失败的原因分类（日志的 reason）
func TestFetchFailureReason(t *testing.T) {
	reset := fmt.Errorf("execute request: %w: %w", ErrNetworkError, errors.New("read tcp 1.2.3.4:5->6.7.8.9:443: read: connection reset by peer"))
	for _, c := range []struct {
		err  error
		want string
	}{
		{nil, ""},
		{fmt.Errorf("phase 2 failed: HTTP 403: %w", ErrCloudflareChallenge), "Cloudflare 质询"},
		{reset, "网络错误（连接被重置）"},
		{fmt.Errorf("%w: %w", ErrNetworkError, context.DeadlineExceeded), "超时"},
		{fmt.Errorf("%w: lookup x.xxx: no such host", ErrNetworkError), "网络错误（域名解析不了）"},
		{ErrSessionExpired, "登录失效"},
		{fmt.Errorf("HTTP 429: %w", ErrRateLimited), "被限流"},
		{&PartialUserInfoError{Missing: []string{"bonus"}, Err: reset}, "部分页面没取到"},
		{&HTTPStatusError{Code: 500}, "HTTP 500"},
		{errors.New("x"), "其他错误"},
	} {
		assert.Equal(t, c.want, FetchFailureReason(c.err), fmt.Sprint(c.err))
	}
	assert.True(t, isTransientNetError(reset))
	assert.False(t, isTransientNetError(fmt.Errorf("%w: %w", ErrNetworkError, &net.DNSError{Err: "no such host", Name: "x.xxx", IsNotFound: true})))
	assert.False(t, isTransientNetError(ErrCloudflareChallenge))
	assert.True(t, strings.Contains((&PartialUserInfoError{Missing: []string{"a", "b"}, Err: reset}).Error(), "a, b"))
}

// 做种列表：一时取不到（5xx、质询、网络错误）算缺做种体积、保存时沿用上一次的；站点没有这一页（404）和以前一样不管它
func TestUserInfo_SeedingStatusFailures(t *testing.T) {
	for _, c := range []struct {
		status  int
		partial bool
	}{{http.StatusBadGateway, true}, {http.StatusNotFound, false}} {
		ps := newPartialServer(t, func(path string, _ int, w http.ResponseWriter) bool {
			if path == "/getusertorrentlistajax.php" {
				w.WriteHeader(c.status)
				return true
			}
			return false
		})
		d := partialDriver(t, ps, 0)
		def := partialDefinition(0)
		delete(def.UserInfo.Selectors, "seedingSize")
		d.SetSiteDefinition(def)
		info, err := d.GetUserInfo(context.Background())
		wantHits := 1
		if c.status >= 500 {
			wantHits = 1 + userInfoNetRetries
		}
		assert.Equal(t, wantHits, ps.count("/getusertorrentlistajax.php"), c.status)
		assert.Equal(t, "tester", info.Username)
		var partial *PartialUserInfoError
		if c.partial {
			require.ErrorAs(t, err, &partial, c.status)
			assert.Equal(t, []string{"seederSize"}, partial.Missing)
		} else {
			assert.NoError(t, err, c.status)
		}
	}
}
