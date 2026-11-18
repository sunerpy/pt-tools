package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 扩展认不出格式的 Cookie（BYR 现在是 auth_token/refresh_token，没有 c_secure_*）带上 verify_cookie 推过来：
// pt-tools 先拿这组 Cookie 实际登录一次，登录上了才保存；没登录上回 422 和原因，库里原来的 Cookie 不动。
// 2026-10-29 用户：「不能做一个通用适配处理吗…站点检测到哪种就推送到 pt-tools，而不是报其缺少 id 之类的」。
func TestUpdateSiteCredential_VerifyCookie(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)
	require.NoError(t, srv.store.SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, AutoStart: true,
	}))
	disabled := false
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("hdsky"), models.SiteConfig{
		Enabled: &disabled, AuthMethod: "cookie", Cookie: "old=1", APIUrl: "https://hdsky.me",
	}))

	var (
		mu     sync.Mutex
		calls  int
		seen   models.SiteConfig
		answer = &sitelogin.ProbeResult{
			Status:   sitelogin.SESSION_EXPIRED,
			RawError: fmt.Errorf("get user info: %w", v2.ErrInvalidCredentials),
		}
	)
	srv.verifyCookie = func(_ context.Context, siteName string, sc models.SiteConfig) (*sitelogin.ProbeResult, string) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		seen = sc
		assert.Equal(t, "hdsky", siteName)
		return answer, sc.Cookie
	}
	setAnswer := func(r *sitelogin.ProbeResult) {
		mu.Lock()
		defer mu.Unlock()
		answer = r
	}
	callCount := func() int {
		mu.Lock()
		defer mu.Unlock()
		return calls
	}
	verify, keep := true, true
	candidate := "auth_token=AAA; refresh_token=BBB; session_id=CCC"
	put := func(body credentialUpdateRequest) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		srv.updateSiteCredential(w, httptest.NewRequest(http.MethodPut, "/api/sites/hdsky", bytes.NewReader(raw)), models.SiteGroup("hdsky"))
		return w
	}
	stored := func() models.SiteConfig {
		t.Helper()
		sc, err := srv.store.GetSiteConf(models.SiteGroup("hdsky"))
		require.NoError(t, err)
		return sc
	}

	// 没登录上：带 keep_enabled 也好、不带也好，都不保存，回 422 写明原因，不回传 Cookie
	for _, body := range []credentialUpdateRequest{
		{Cookie: &candidate, VerifyCookie: &verify, KeepEnabled: &keep},
		{Cookie: &candidate, VerifyCookie: &verify},
	} {
		w := put(body)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "没有保存")
		assert.Contains(t, w.Body.String(), "HDSky")
		assert.Contains(t, w.Body.String(), "401 或 403")
		assert.Contains(t, w.Body.String(), "原来的 Cookie 没有改动")
		assert.NotContains(t, w.Body.String(), "AAA")
		sc := stored()
		assert.Equal(t, "old=1", sc.Cookie, "没登录上的那组没有写进去")
		require.NotNil(t, sc.Enabled)
		assert.False(t, *sc.Enabled, "没有因此启用站点")
	}
	assert.Equal(t, 2, callCount())
	assert.Equal(t, candidate, seen.Cookie, "验证用的是这次推来的 Cookie")
	assert.Equal(t, "https://hdsky.me", seen.APIUrl, "按站点配置的地址访问")

	// verify_cookie 不带 Cookie 是请求错了，不去访问站点
	w := put(credentialUpdateRequest{VerifyCookie: &verify, Passkey: credStrPtr("pk")})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Equal(t, 2, callCount())

	// 登录上了：照原来的路保存（keep_enabled 时站点保持停用）
	setAnswer(&sitelogin.ProbeResult{Status: sitelogin.OK})
	w = put(credentialUpdateRequest{Cookie: &candidate, VerifyCookie: &verify, KeepEnabled: &keep})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, candidate, stored().Cookie)
	assert.False(t, *stored().Enabled)
	assert.Equal(t, 3, callCount())

	// 不带 verify_cookie：照旧直接保存，不访问站点
	w = put(credentialUpdateRequest{Cookie: credStrPtr("c_secure_uid=1; c_secure_pass=2"), KeepEnabled: &keep})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "c_secure_uid=1; c_secure_pass=2", stored().Cookie)
	assert.Equal(t, 3, callCount())

	// 登录上了、不带 keep_enabled：保存并启用（扩展里手动点的同步）
	w = put(credentialUpdateRequest{Cookie: &candidate, VerifyCookie: &verify})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, candidate, stored().Cookie)
	assert.True(t, *stored().Enabled)
	settleCredentialReload(t, srv)
}

// 验证和登录探测共用单站锁：这个站点正在探测时回 409、不去访问站点；验证完锁放开。
func TestUpdateSiteCredential_VerifyCookieWaitsForProbeLock(t *testing.T) {
	writeWebTestSecretKey(t)
	srv := setupServer(t)
	disabled := false
	require.NoError(t, srv.store.UpsertSiteWithRSS(models.SiteGroup("hdsky"), models.SiteConfig{
		Enabled: &disabled, AuthMethod: "cookie", Cookie: "old=1", APIUrl: "https://hdsky.me",
	}))
	mon := scheduler.NewLoginReminderMonitor(scheduler.LoginReminderConfig{})
	srv.mgr.SetLoginReminderMonitor(mon)
	t.Cleanup(func() { srv.mgr.SetLoginReminderMonitor(nil) })
	calls := 0
	srv.verifyCookie = func(_ context.Context, _ string, sc models.SiteConfig) (*sitelogin.ProbeResult, string) {
		calls++
		_, free := mon.TryAcquireProbeLock("hdsky")
		assert.False(t, free, "验证期间持有这个站点的探测锁")
		return &sitelogin.ProbeResult{Status: sitelogin.NETWORK_ERROR, RawError: v2.ErrNetworkError}, sc.Cookie
	}
	verify, keep := true, true
	put := func() *httptest.ResponseRecorder {
		raw, _ := json.Marshal(credentialUpdateRequest{Cookie: credStrPtr("a=1"), VerifyCookie: &verify, KeepEnabled: &keep})
		w := httptest.NewRecorder()
		srv.updateSiteCredential(w, httptest.NewRequest(http.MethodPut, "/api/sites/hdsky", bytes.NewReader(raw)), models.SiteGroup("hdsky"))
		return w
	}

	release, ok := mon.TryAcquireProbeLock("hdsky")
	require.True(t, ok)
	w := put()
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "正在做登录检查")
	assert.Equal(t, 0, calls)
	release()

	w = put()
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "连不上 HDSky")
	assert.Equal(t, 1, calls)
	again, ok := mon.TryAcquireProbeLock("hdsky")
	require.True(t, ok, "验证完把锁放开了")
	again()
}

func TestCookieVerifyFailure(t *testing.T) {
	tests := []struct {
		name   string
		result *sitelogin.ProbeResult
		want   []string
		not    []string
	}{
		{
			name:   "回了登录页",
			result: &sitelogin.ProbeResult{Status: sitelogin.SESSION_EXPIRED, RawError: fmt.Errorf("index: %w（页面标题：登录）", v2.ErrSessionExpired)},
			want:   []string{"没有保存：用这组 Cookie 没能登录 HDSky", "页面标题：登录"},
		},
		{
			name:   "页面上没有用户名时不重复长说明",
			result: &sitelogin.ProbeResult{Status: sitelogin.PARSE_ERROR, Diagnostic: "取回的页面上没有用户名，多半不是登录后的页面：…"},
			want:   []string{"页面上没有用户名，多半没有登录"},
			not:    []string{"（"},
		},
		{
			name:   "人机验证",
			result: &sitelogin.ProbeResult{Status: sitelogin.CHALLENGE, RawError: fmt.Errorf("HTTP 403: %w", v2.ErrCloudflareChallenge)},
			want:   []string{"要求人机验证", "pt-tools 的服务器过不去"},
		},
		{
			name:   "限流",
			result: &sitelogin.ProbeResult{Status: sitelogin.RATE_LIMITED},
			want:   []string{"限制了访问频率"},
		},
		{
			name:   "没有内置定义",
			result: &sitelogin.ProbeResult{Status: sitelogin.UNSUPPORTED, Diagnostic: "站点没有内置定义"},
			want:   []string{"没法在线验证"},
		},
		{
			name:   "说不清时说没能确认，说明截断",
			result: &sitelogin.ProbeResult{Status: sitelogin.UNKNOWN, Diagnostic: strings.Repeat("长", 300)},
			want:   []string{"没能确认这组 Cookie 能不能登录 HDSky", strings.Repeat("长", 200) + "…"},
			not:    []string{strings.Repeat("长", 201)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cookieVerifyFailure("hdsky", tt.result)
			for _, want := range tt.want {
				assert.Contains(t, got, want)
			}
			for _, not := range tt.not {
				assert.NotContains(t, got, not)
			}
			assert.True(t, strings.HasSuffix(got, "pt-tools 里原来的 Cookie 没有改动"), got)
		})
	}
}

// probeSiteCookie 用这次推来的 Cookie 新建站点实例去取用户信息，不用已注册的实例（那上面是旧 Cookie）：
// 假站点只回登录页，判成没登录；请求里带的是候选 Cookie。
func TestProbeSiteCookie_UsesCandidateCookie(t *testing.T) {
	var (
		mu      sync.Mutex
		cookies []string
	)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		cookies = append(cookies, r.Header.Get("Cookie"))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>登录</title></head><body><form action="takelogin.php" method="post"></form></body></html>`))
	}))
	t.Cleanup(fake.Close)

	InitSiteRegistry(nil)
	result, _ := probeSiteCookie(context.Background(), "hdsky", models.SiteConfig{Cookie: "candidate=1", APIUrl: fake.URL})
	assert.Equal(t, sitelogin.UNKNOWN, result.Status, "站点服务没初始化时说不清，不当成登录上了")

	InitSiteRegistry(v2.NewSiteRegistry(nil))
	t.Cleanup(func() { InitSiteRegistry(nil) })
	result, _ = probeSiteCookie(context.Background(), "no-such-site", models.SiteConfig{Cookie: "candidate=1", APIUrl: fake.URL})
	assert.Equal(t, sitelogin.UNSUPPORTED, result.Status)

	result, cookie := probeSiteCookie(context.Background(), "hdsky", models.SiteConfig{Cookie: "candidate=1", APIUrl: fake.URL})
	assert.Equal(t, sitelogin.SESSION_EXPIRED, result.Status, "%+v", result)
	assert.Equal(t, "candidate=1", cookie, "不续期的站点原样返回")
	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, cookies)
	for _, c := range cookies {
		assert.Contains(t, c, "candidate=1")
	}
}
