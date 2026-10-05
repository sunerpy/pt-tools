package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// fakeFallback 是后备通道：block 为 true 时一直等到 ctx 结束。
type fakeFallback struct {
	result *sitelogin.ProbeResult
	block  bool
	calls  atomic.Int32
}

func (f *fakeFallback) Name() string { return "cloak" }

func (f *fakeFallback) FetchUserInfo(ctx context.Context, _ *v2.SiteDefinition, _ v2.Site, _ sitelogin.Clock) (*sitelogin.ProbeResult, error) {
	f.calls.Add(1)
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return f.result, nil
}

type fakeFallbackProvider struct {
	transport sitelogin.Transport
	calls     atomic.Int32
	lastSite  string
	lastDef   *v2.SiteDefinition
}

func (p *fakeFallbackProvider) FallbackFor(_ context.Context, setting models.SiteSetting, def *v2.SiteDefinition) sitelogin.Transport {
	p.calls.Add(1)
	p.lastSite = setting.Name
	p.lastDef = def
	return p.transport
}

func challengedUserInfo() *fakeUserInfo {
	return &fakeUserInfo{err: errors.New("get user info: cloudflare challenge page")}
}

// M1d：主通道被拦截时用后备探测；后备返回 OK 时替换结果，登录时间按 cloak 来源写进 Cookie 登录时间。
func TestProbe_FallbackReplacesChallengedPrimary(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(newReminderTestDB(t), now)
	createSite(t, m, cookieSite("hdsky"))
	m.userInfo = challengedUserInfo()
	access := now.Add(-time.Hour)
	login := now.Add(-2 * time.Hour)
	fb := &fakeFallback{result: &sitelogin.ProbeResult{Status: sitelogin.OK, Source: sitelogin.ProbeSourceCloak, LastAccessAt: &access, LastLoginAt: &login}}
	provider := &fakeFallbackProvider{transport: fb}
	m.fallback = provider

	m.RunProbeOnce(context.Background())

	assert.Equal(t, int32(1), fb.calls.Load())
	assert.Equal(t, "hdsky", provider.lastSite)
	require.NotNil(t, provider.lastDef)
	st := loadState(t, m, "hdsky")
	assert.Equal(t, "OK", st.LastProbeStatus)
	require.NotNil(t, st.CookieLastLoginAt)
	assert.True(t, st.CookieLastLoginAt.Equal(login))
	require.NotNil(t, st.LastAccessAt)
	assert.Zero(t, st.ConsecutiveProbeFailures)
}

// 后备也没成功时保留主通道的状态，后备的说明附在错误信息后面。
func TestProbe_FallbackFailureKeepsPrimaryStatus(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(newReminderTestDB(t), now)
	createSite(t, m, cookieSite("hdsky"))
	m.userInfo = challengedUserInfo()
	m.fallback = &fakeFallbackProvider{transport: &fakeFallback{result: &sitelogin.ProbeResult{Status: sitelogin.NOT_APPLICABLE, Diagnostic: "cookie not configured"}}}

	m.RunProbeOnce(context.Background())

	st := loadState(t, m, "hdsky")
	assert.Equal(t, "CHALLENGE", st.LastProbeStatus, "NOT_APPLICABLE never becomes the site status")
	assert.Contains(t, st.LastProbeError, "cloudflare")
	assert.Contains(t, st.LastProbeError, "CloakBrowser 后备未成功")
	assert.Contains(t, st.LastProbeError, "cookie not configured")
	assert.Equal(t, 1, st.ConsecutiveProbeFailures)
}

// 凭证类失败与正常结果都不走后备；提供者返回 nil（配置不全等）时同样不走。
func TestProbe_FallbackOnlyForEligibleStatuses(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(newReminderTestDB(t), now)
	createSite(t, m, cookieSite("hdsky"))
	fb := &fakeFallback{result: &sitelogin.ProbeResult{Status: sitelogin.OK}}
	m.fallback = &fakeFallbackProvider{transport: fb}

	m.userInfo = &fakeUserInfo{err: wrapErr(v2.ErrSessionExpired)}
	m.RunProbeOnce(context.Background())
	assert.Equal(t, "SESSION_EXPIRED", loadState(t, m, "hdsky").LastProbeStatus)

	m.userInfo = &fakeUserInfo{info: okUserInfo(now)}
	m.RunProbeOnce(context.Background())
	assert.Equal(t, "OK", loadState(t, m, "hdsky").LastProbeStatus)
	assert.Zero(t, fb.calls.Load(), "the fallback is only for blocked or unreachable sites")

	m.userInfo = challengedUserInfo()
	m.fallback = &fakeFallbackProvider{transport: nil}
	m.RunProbeOnce(context.Background())
	assert.Equal(t, "CHALLENGE", loadState(t, m, "hdsky").LastProbeStatus)
	assert.NotContains(t, loadState(t, m, "hdsky").LastProbeError, "CloakBrowser", "no fallback, no fallback note")
}

// 手动探测带后备时也在单次预算内返回：后备一直不返回时按预算截断，状态保持主通道的结果。
func TestProbe_ManualProbeWithFallbackHonoursBudget(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(newReminderTestDB(t), now)
	m.probeBudget = 300 * time.Millisecond
	m.primaryTimeout = 100 * time.Millisecond
	createSite(t, m, cookieSite("hdsky"))
	m.userInfo = challengedUserInfo()
	fb := &fakeFallback{block: true}
	m.fallback = &fakeFallbackProvider{transport: fb}

	start := time.Now()
	require.True(t, m.RunProbeOnceForSite(context.Background(), "hdsky"))
	assert.Less(t, time.Since(start), 2*time.Second, "the whole probe stays within its budget")
	assert.Equal(t, int32(1), fb.calls.Load())
	st := loadState(t, m, "hdsky")
	assert.Equal(t, "CHALLENGE", st.LastProbeStatus)
	assert.Contains(t, st.LastProbeError, "context deadline exceeded")
}

// 没有 UserInfoService 时（resolver 路径）同样会问提供者要后备。
func TestProbe_ResolverPathAlsoUsesFallback(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(newReminderTestDB(t), now)
	createSite(t, m, cookieSite("hdsky"))
	site := &countingSite{}
	site.err = errors.New("cloudflare challenge")
	m.resolver = &countingResolver{def: nexusDef(), site: site}
	access := now.Add(-time.Hour)
	fb := &fakeFallback{result: &sitelogin.ProbeResult{Status: sitelogin.OK, Source: sitelogin.ProbeSourceCloak, LastAccessAt: &access}}
	m.fallback = &fakeFallbackProvider{transport: fb}

	m.RunProbeOnce(context.Background())
	assert.Equal(t, int32(1), fb.calls.Load())
	assert.Equal(t, "OK", loadState(t, m, "hdsky").LastProbeStatus)
}

func wrapErr(err error) error { return errors.Join(errors.New("fetch user info from hdsky"), err) }
