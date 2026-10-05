package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// fakeUserInfo 代替 UserInfoService：记录 FetchAndSave 的调用；block 非空时阻塞到被关闭或 ctx 结束。
type fakeUserInfo struct {
	mu      sync.Mutex
	info    v2.UserInfo
	err     error
	ids     []string
	calls   atomic.Int32
	block   chan struct{}
	entered chan struct{}
}

func (f *fakeUserInfo) FetchAndSave(ctx context.Context, siteID string) (v2.UserInfo, error) {
	f.calls.Add(1)
	f.mu.Lock()
	f.ids = append(f.ids, siteID)
	info, err := f.info, f.err
	f.mu.Unlock()
	if f.entered != nil {
		select {
		case f.entered <- struct{}{}:
		default:
		}
	}
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return v2.UserInfo{}, ctx.Err()
		}
	}
	return info, err
}

// namedSite 是注册进真实 UserInfoService 的假站点，ID 与站点配置名一致。
type namedSite struct {
	fakeReminderSite
	id    string
	calls atomic.Int32
}

func (s *namedSite) ID() string { return s.id }

func (s *namedSite) GetUserInfo(context.Context) (v2.UserInfo, error) {
	s.calls.Add(1)
	return s.info, s.err
}

type failingSaveRepo struct {
	v2.UserInfoRepo
}

func (failingSaveRepo) Save(context.Context, v2.UserInfo) error { return errors.New("disk full") }

func okUserInfo(now time.Time) v2.UserInfo {
	return v2.UserInfo{Site: "hdsky", Username: "tester", Uploaded: 42, LastAccess: now.Add(-time.Hour).Unix()}
}

// M1b：有 UserInfo 依赖时探测走 FetchAndSave（与搜索共用的实例和限速器），不再经 resolver 新建站点实例。
func TestProbe_UsesUserInfoServiceInsteadOfResolver(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(newReminderTestDB(t), now)
	createSite(t, m, cookieSite("hdsky"))
	resolver := &countingResolver{def: nexusDef(), site: &countingSite{}}
	m.resolver = resolver
	fetch := &fakeUserInfo{info: okUserInfo(now)}
	m.userInfo = fetch

	m.RunProbeOnce(context.Background())

	assert.Equal(t, int32(1), fetch.calls.Load())
	assert.Equal(t, []string{"hdsky"}, fetch.ids, "the site is fetched by its configured name")
	assert.Zero(t, resolver.calls.Load(), "no extra site instance is created")
	st := loadState(t, m, "hdsky")
	assert.Equal(t, "OK", st.LastProbeStatus)
	require.NotNil(t, st.LastAccessAt)
	assert.Equal(t, now.Add(-time.Hour).Unix(), st.LastAccessAt.Unix())
}

// M1b：成功探测顺带刷新用户统计（真实 UserInfoService + 内存仓库）。
func TestProbe_UserInfoServiceRefreshesStatistics(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(newReminderTestDB(t), now)
	createSite(t, m, cookieSite("hdsky"))
	svc := v2.NewUserInfoService(v2.UserInfoServiceConfig{Repo: v2.NewInMemoryUserInfoRepo()})
	site := &namedSite{id: "hdsky"}
	site.info = okUserInfo(now)
	svc.RegisterSite(site)
	m.userInfo = svc

	m.RunProbeOnce(context.Background())

	assert.Equal(t, int32(1), site.calls.Load())
	got, err := svc.GetUserInfo(context.Background(), "hdsky")
	require.NoError(t, err)
	assert.Equal(t, int64(42), got.Uploaded, "the probe saved the statistics it fetched")
	assert.Equal(t, "OK", loadState(t, m, "hdsky").LastProbeStatus)
}

// M1b：站点没有注册到 UserInfoService（凭证缺失或创建实例失败）记为未配置凭证，不计失败。
func TestProbe_UnregisteredSiteIsNotConfigured(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(newReminderTestDB(t), now)
	createSite(t, m, cookieSite("hdsky"))
	m.userInfo = v2.NewUserInfoService(v2.UserInfoServiceConfig{Repo: v2.NewInMemoryUserInfoRepo()})

	m.RunProbeOnce(context.Background())

	st := loadState(t, m, "hdsky")
	assert.Equal(t, "NOT_CONFIGURED", st.LastProbeStatus)
	assert.Zero(t, st.ConsecutiveProbeFailures)
	assert.Nil(t, st.FirstFailureAt)
}

// M1b：取回的用户信息没有用户名时记为解析失败。
func TestProbe_EmptyUsernameIsParseError(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(newReminderTestDB(t), now)
	createSite(t, m, cookieSite("hdsky"))
	svc := v2.NewUserInfoService(v2.UserInfoServiceConfig{Repo: v2.NewInMemoryUserInfoRepo()})
	site := &namedSite{id: "hdsky"}
	site.info = v2.UserInfo{Site: "hdsky", LastAccess: now.Add(-time.Hour).Unix()}
	svc.RegisterSite(site)
	m.userInfo = svc

	m.RunProbeOnce(context.Background())

	st := loadState(t, m, "hdsky")
	assert.Equal(t, "PARSE_ERROR", st.LastProbeStatus)
	assert.Equal(t, 1, st.ConsecutiveProbeFailures)
}

// M1b：统计记录保存失败时按取回的数据判断站点状态，不算探测失败。
func TestProbe_PersistFailureJudgedByReturnedInfo(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(newReminderTestDB(t), now)
	createSite(t, m, cookieSite("hdsky"))
	svc := v2.NewUserInfoService(v2.UserInfoServiceConfig{Repo: failingSaveRepo{UserInfoRepo: v2.NewInMemoryUserInfoRepo()}})
	site := &namedSite{id: "hdsky"}
	site.info = okUserInfo(now)
	svc.RegisterSite(site)
	m.userInfo = svc

	m.RunProbeOnce(context.Background())

	st := loadState(t, m, "hdsky")
	assert.Equal(t, "OK", st.LastProbeStatus)
	assert.Zero(t, st.ConsecutiveProbeFailures)
	require.NotNil(t, st.LastAccessAt)
}

// M1b：没有 UserInfoService（仓库初始化失败）时退回 resolver 新建实例的旧路径。
func TestProbe_WithoutUserInfoServiceUsesResolver(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(newReminderTestDB(t), now)
	createSite(t, m, cookieSite("hdsky"))
	site := &countingSite{}
	site.info = okUserInfo(now)
	resolver := &countingResolver{def: nexusDef(), site: site}
	m.resolver = resolver

	m.RunProbeOnce(context.Background())

	assert.Equal(t, int32(1), resolver.calls.Load())
	assert.Equal(t, int32(1), site.calls.Load())
	assert.Equal(t, "OK", loadState(t, m, "hdsky").LastProbeStatus)
}

// M1b 竞态一：批量循环读到的是补齐凭证之前的配置，取锁后重读到新凭证，按新凭证探测，不判为未配置凭证。
func TestProbe_CredentialCompletedAfterBatchReadUsesNewCredential(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(newReminderTestDB(t), now)
	stale := models.SiteSetting{Name: "hdsky", Enabled: true, AuthMethod: "cookie"}
	createSite(t, m, stale)
	svc := v2.NewUserInfoService(v2.UserInfoServiceConfig{Repo: v2.NewInMemoryUserInfoRepo()})
	m.userInfo = svc

	// 循环读完配置之后：凭证写库 → 刷新注册（这里直接注册新实例）。
	require.NoError(t, m.db.Model(&models.SiteSetting{}).Where("name = ?", "hdsky").Update("cookie", "uid=1; pass=2").Error)
	site := &namedSite{id: "hdsky"}
	site.info = okUserInfo(now)
	svc.RegisterSite(site)

	release, ok := m.TryAcquireProbeLock("hdsky")
	require.True(t, ok)
	m.probeSiteInternal(context.Background(), stale, false)
	release()

	assert.Equal(t, "OK", loadState(t, m, "hdsky").LastProbeStatus)
	assert.Equal(t, int32(1), site.calls.Load())
}

// M1b 竞态二：探测进行中更新了凭证（写库 → 刷新注册 → RequestProbe），这次探测结束后该站仍到期，下一轮再探一次。
func TestRunDueProbes_CredentialUpdateDuringProbeProbesAgain(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(newReminderTestDB(t), now)
	createSite(t, m, cookieSite("hdsky"))
	due := now.Add(-time.Minute)
	require.NoError(t, m.db.Create(&models.SiteLoginState{
		SiteName: "hdsky", BanThresholdDays: 30, RemindBeforeDays: 10, ReminderCron: "0 10,22 * * *",
		LastReminderTier: "none", ProbeMode: "auto", ProbeJitterSeconds: 60, NextProbeAt: &due,
	}).Error)
	fetch := &fakeUserInfo{info: okUserInfo(now), block: make(chan struct{}), entered: make(chan struct{}, 1)}
	m.userInfo = fetch

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.RunDueProbes(context.Background())
	}()
	select {
	case <-fetch.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the probe never started")
	}
	advanceClock(m, time.Second)
	require.NoError(t, m.RequestProbe("hdsky"))
	close(fetch.block)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the probe never finished")
	}
	assert.Equal(t, int32(1), fetch.calls.Load())

	m.RunDueProbes(context.Background())
	assert.Equal(t, int32(2), fetch.calls.Load(), "a request made during the probe triggers one more probe")

	advanceClock(m, time.Minute)
	m.RunDueProbes(context.Background())
	assert.Equal(t, int32(2), fetch.calls.Load(), "the request is consumed by the second probe")
}

// M1b：RequestProbe 用监控自己的时钟写 probe_requested_at，缺行时先建行；auto 模式下一轮循环即探测。
func TestRequestProbe_SchedulesImmediateProbe(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(newReminderTestDB(t), now)
	createSite(t, m, cookieSite("hdsky"))
	fetch := &fakeUserInfo{info: okUserInfo(now)}
	m.userInfo = fetch

	require.NoError(t, m.RequestProbe("hdsky"))
	st := loadState(t, m, "hdsky")
	require.NotNil(t, st.ProbeRequestedAt)
	assert.True(t, st.ProbeRequestedAt.Equal(now))

	m.RunDueProbes(context.Background())
	assert.Equal(t, int32(1), fetch.calls.Load(), "a fresh row with a pending request is probed at once")
	m.RunDueProbes(context.Background())
	assert.Equal(t, int32(1), fetch.calls.Load())

	var nilMonitor *LoginReminderMonitor
	assert.NoError(t, nilMonitor.RequestProbe("hdsky"))
}

// 预算为 0 时用默认的 150 秒和 60 秒，经 UserInfoService 的探测照常完成。
func TestProbe_UserInfoPathWithDefaultBudgets(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(newReminderTestDB(t), now)
	m.probeBudget = 0
	m.primaryTimeout = 0
	createSite(t, m, cookieSite("hdsky"))
	m.userInfo = &fakeUserInfo{info: okUserInfo(now)}
	m.RunProbeOnce(context.Background())
	assert.Equal(t, "OK", loadState(t, m, "hdsky").LastProbeStatus)
}

// 登录状态表不可用时 RequestProbe 返回错误，调用方据此记告警。
func TestRequestProbe_ReportsStateErrors(t *testing.T) {
	db := newReminderTestDB(t)
	require.NoError(t, db.Migrator().DropTable(&models.SiteLoginState{}))
	m := newReminderMonitorForTest(db, time.Now())
	assert.Error(t, m.RequestProbe("hdsky"))
}
