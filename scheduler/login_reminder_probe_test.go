package scheduler

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// countingSite 记录 GetUserInfo 的调用次数；block 非空时阻塞到被关闭或 ctx 结束，entered 用于通知「已进入」。
type countingSite struct {
	fakeReminderSite
	calls   atomic.Int32
	block   chan struct{}
	entered chan struct{}
}

func (s *countingSite) GetUserInfo(ctx context.Context) (v2.UserInfo, error) {
	s.calls.Add(1)
	if s.entered != nil {
		select {
		case s.entered <- struct{}{}:
		default:
		}
	}
	if s.block != nil {
		select {
		case <-s.block:
		case <-ctx.Done():
			return v2.UserInfo{}, ctx.Err()
		}
	}
	return s.info, s.err
}

type countingResolver struct {
	def   *v2.SiteDefinition
	site  v2.Site
	err   error
	calls atomic.Int32
}

func (r *countingResolver) Resolve(models.SiteSetting) (*v2.SiteDefinition, v2.Site, error) {
	r.calls.Add(1)
	return r.def, r.site, r.err
}

func nexusDef() *v2.SiteDefinition {
	return &v2.SiteDefinition{ID: "hdsky", Schema: v2.SchemaNexusPHP}
}

func createSite(t *testing.T, m *LoginReminderMonitor, setting models.SiteSetting) {
	t.Helper()
	require.NoError(t, m.db.Create(&setting).Error)
}

func loadState(t *testing.T, m *LoginReminderMonitor, name string) models.SiteLoginState {
	t.Helper()
	var st models.SiteLoginState
	require.NoError(t, m.db.Where("site_name = ?", name).First(&st).Error)
	return st
}

func cookieSite(name string) models.SiteSetting {
	return models.SiteSetting{Name: name, Enabled: true, AuthMethod: "cookie", Cookie: "c=1"}
}

// 问题 1：非会话类失败持续满 24 小时恰好提醒一次（按真实调用顺序：探测 → 提醒循环 → 投递）。
func TestProbeFailureNotice_NonSessionFailureNotifiesOnceAfter24h(t *testing.T) {
	db := newReminderTestDB(t)
	t0 := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(db, t0)
	createSite(t, m, cookieSite("HDSKY"))
	m.resolver = &fakeReminderResolver{def: nexusDef(), site: &fakeReminderSite{err: fmt.Errorf("dial: %w", context.DeadlineExceeded)}}
	rec := attachCaptureNotifier(t, m, db)

	for range 6 { // 0h .. 30h，每 6 小时一次
		m.RunProbeOnce(context.Background())
		m.RunReminderOnce(context.Background())
		m.notifier.DeliverDue(context.Background())
		if m.clock.Now().Sub(t0) < 24*time.Hour {
			assert.Equal(t, int32(0), rec.count.Load(), "no notice before the failure lasted 24h")
		}
		advanceClock(m, 6*time.Hour)
	}
	assert.Equal(t, int32(1), rec.count.Load(), "a network failure lasting 30h is notified exactly once")
	title, text := rec.lastMessage()
	assert.Contains(t, title, "探测失败")
	assert.Contains(t, text, "网络错误")
}

// 问题 2：凭证类失败立即提醒，之后 24 小时内不重复，满 24 小时再提醒一次。
func TestProbeFailureNotice_CredentialFailureOncePer24h(t *testing.T) {
	db := newReminderTestDB(t)
	t0 := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(db, t0)
	createSite(t, m, cookieSite("HDSKY"))
	m.resolver = &fakeReminderResolver{def: nexusDef(), site: &fakeReminderSite{err: fmt.Errorf("x: %w", v2.ErrSessionExpired)}}
	rec := attachCaptureNotifier(t, m, db)

	for range 4 { // 0h, 6h, 12h, 18h
		m.RunProbeOnce(context.Background())
		m.RunReminderOnce(context.Background())
		m.notifier.DeliverDue(context.Background())
		advanceClock(m, 6*time.Hour)
	}
	assert.Equal(t, int32(1), rec.count.Load(), "expired session is notified once within 24h")

	m.RunProbeOnce(context.Background()) // 24h
	m.RunReminderOnce(context.Background())
	m.notifier.DeliverDue(context.Background())
	assert.Equal(t, int32(2), rec.count.Load(), "still expired after 24h → one more notice")
	_, text := rec.lastMessage()
	assert.Contains(t, text, "会话已过期")
	assert.Contains(t, text, "重新同步 Cookie")
}

func TestProbeFailureNotice_RecoveryNotifiedOnce(t *testing.T) {
	db := newReminderTestDB(t)
	t0 := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(db, t0)
	createSite(t, m, cookieSite("HDSKY"))
	site := &fakeReminderSite{err: fmt.Errorf("x: %w", v2.ErrSessionExpired)}
	m.resolver = &fakeReminderResolver{def: nexusDef(), site: site}
	rec := attachCaptureNotifier(t, m, db)

	m.RunProbeOnce(context.Background())
	m.RunReminderOnce(context.Background())
	m.notifier.DeliverDue(context.Background())
	require.Equal(t, int32(1), rec.count.Load())

	site.err = nil
	site.info = v2.UserInfo{LastAccess: t0.Unix()}
	advanceClock(m, time.Hour)
	m.RunProbeOnce(context.Background())
	for range 3 {
		m.RunReminderOnce(context.Background())
		m.notifier.DeliverDue(context.Background())
	}
	assert.Equal(t, int32(2), rec.count.Load(), "recovery is notified exactly once")
	title, _ := rec.lastMessage()
	assert.Contains(t, title, "探测已恢复")
	st := loadState(t, m, "HDSKY")
	assert.Nil(t, st.LastFailureNotifiedAt)
	assert.Nil(t, st.FirstFailureAt)
	require.NotNil(t, st.LastSuccessAt)
}

func TestProbeFailureNotice_NoRecoveryWithoutPriorFailureNotice(t *testing.T) {
	db := newReminderTestDB(t)
	t0 := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(db, t0)
	createSite(t, m, cookieSite("HDSKY"))
	site := &fakeReminderSite{err: fmt.Errorf("dial: %w", v2.ErrNetworkError)}
	m.resolver = &fakeReminderResolver{def: nexusDef(), site: site}
	rec := attachCaptureNotifier(t, m, db)

	m.RunProbeOnce(context.Background()) // short network blip, never notified
	m.RunReminderOnce(context.Background())
	site.err = nil
	site.info = v2.UserInfo{LastAccess: t0.Unix()}
	advanceClock(m, time.Hour)
	m.RunProbeOnce(context.Background())
	m.RunReminderOnce(context.Background())
	m.notifier.DeliverDue(context.Background())
	assert.Equal(t, int32(0), rec.count.Load(), "no failure notice was sent, so there is nothing to recover from")
}

// 问题 3：首轮按抖动分散，重启不补探，退避表逐行生效。
func TestRunDueProbes_FirstRoundIsSpreadAndNotImmediate(t *testing.T) {
	db := newReminderTestDB(t)
	t0 := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(db, t0)
	site := &countingSite{fakeReminderSite: fakeReminderSite{info: v2.UserInfo{LastAccess: t0.Unix()}}}
	m.resolver = &fakeReminderResolver{def: nexusDef(), site: site}
	for i := range 20 {
		createSite(t, m, cookieSite(fmt.Sprintf("S%02d", i)))
	}

	m.RunDueProbes(context.Background())
	assert.Equal(t, int32(0), site.calls.Load(), "upgraded rows without NextProbeAt are scheduled, not probed at once")

	distinct := map[int64]struct{}{}
	for i := range 20 {
		st := loadState(t, m, fmt.Sprintf("S%02d", i))
		require.NotNil(t, st.NextProbeAt)
		assert.True(t, st.NextProbeAt.After(t0), "first probe is in the future")
		assert.False(t, st.NextProbeAt.After(t0.Add(6*time.Hour)), "first round fits in one 6h interval")
		assert.Positive(t, st.ProbeJitterSeconds)
		distinct[st.NextProbeAt.Unix()] = struct{}{}
	}
	assert.Greater(t, len(distinct), 1, "first probes are spread out, not aligned")

	advanceClock(m, 6*time.Hour)
	m.RunDueProbes(context.Background())
	assert.Equal(t, int32(20), site.calls.Load(), "every site is probed once within the first interval")
}

func TestRunDueProbes_RestartDoesNotReprobe(t *testing.T) {
	db := newReminderTestDB(t)
	t0 := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(db, t0)
	createSite(t, m, cookieSite("HDSKY"))
	site := &countingSite{fakeReminderSite: fakeReminderSite{info: v2.UserInfo{LastAccess: t0.Unix()}}}
	m.resolver = &fakeReminderResolver{def: nexusDef(), site: site}

	m.RunProbeOnce(context.Background())
	require.Equal(t, int32(1), site.calls.Load())

	// 模拟重启：新的监控实例，同一个数据库，一小时后。
	restarted := newReminderMonitorForTest(db, t0.Add(time.Hour))
	restarted.resolver = m.resolver
	restarted.RunDueProbes(context.Background())
	assert.Equal(t, int32(1), site.calls.Load(), "a restart must not re-probe a site that is not due")

	advanceClock(restarted, 6*time.Hour)
	restarted.RunDueProbes(context.Background())
	assert.Equal(t, int32(2), site.calls.Load(), "the site is probed again once its NextProbeAt passes")
}

func TestRunDueProbes_SkipsManualAndDisabled(t *testing.T) {
	db := newReminderTestDB(t)
	t0 := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(db, t0)
	site := &countingSite{}
	m.resolver = &fakeReminderResolver{def: nexusDef(), site: site}
	past := t0.Add(-time.Hour)
	for _, mode := range []string{ProbeModeManual, ProbeModeDisabled} {
		name := "S-" + mode
		createSite(t, m, cookieSite(name))
		require.NoError(t, db.Create(&models.SiteLoginState{SiteName: name, ProbeMode: mode, NextProbeAt: &past, ProbeRequestedAt: &t0}).Error)
	}
	m.RunDueProbes(context.Background())
	assert.Equal(t, int32(0), site.calls.Load(), "only auto mode is probed on schedule or on request")
}

func TestRunDueProbes_ProbeRequestTriggersImmediateProbeOnce(t *testing.T) {
	db := newReminderTestDB(t)
	t0 := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(db, t0)
	createSite(t, m, cookieSite("HDSKY"))
	site := &countingSite{fakeReminderSite: fakeReminderSite{info: v2.UserInfo{LastAccess: t0.Unix()}}}
	m.resolver = &fakeReminderResolver{def: nexusDef(), site: site}
	future := t0.Add(5 * time.Hour)
	started := t0.Add(-2 * time.Hour)
	requested := t0.Add(-time.Minute)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName: "HDSKY", ProbeMode: ProbeModeAuto, ProbeJitterSeconds: 60,
		NextProbeAt: &future, LastProbeStartedAt: &started, ProbeRequestedAt: &requested,
	}).Error)

	m.RunDueProbes(context.Background())
	assert.Equal(t, int32(1), site.calls.Load(), "a request newer than the last start is honoured right away")
	m.RunDueProbes(context.Background())
	assert.Equal(t, int32(1), site.calls.Load(), "the request is consumed by the probe that started after it")
}

func TestNextProbeDelay_Table(t *testing.T) {
	m := newReminderMonitorForTest(newReminderTestDB(t), time.Now())
	cases := []struct {
		status   sitelogin.ProbeStatus
		failures int
		min, max time.Duration
	}{
		{sitelogin.OK, 0, 324 * time.Minute, 396 * time.Minute}, // 6h ± 10%
		{sitelogin.SESSION_EXPIRED, 1, 24 * time.Hour, 24 * time.Hour},
		{sitelogin.KEY_ERROR, 3, 24 * time.Hour, 24 * time.Hour},
		{sitelogin.NETWORK_ERROR, 1, time.Hour, time.Hour},
		{sitelogin.CHALLENGE, 2, 2 * time.Hour, 2 * time.Hour},
		{sitelogin.RATE_LIMITED, 3, 4 * time.Hour, 4 * time.Hour},
		{sitelogin.PARSE_ERROR, 4, 6 * time.Hour, 6 * time.Hour},
		{sitelogin.UNKNOWN, 9, 6 * time.Hour, 6 * time.Hour},
		{sitelogin.NOT_CONFIGURED, 0, 6 * time.Hour, 6 * time.Hour},
		{sitelogin.UNSUPPORTED, 0, 24 * time.Hour, 24 * time.Hour},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s-%d", tc.status, tc.failures), func(t *testing.T) {
			for range 20 {
				d := m.nextProbeDelay(tc.status, tc.failures)
				assert.GreaterOrEqual(t, d, tc.min)
				assert.LessOrEqual(t, d, tc.max)
			}
		})
	}
}

func TestRecordProbeResult_SchedulesByBackoff(t *testing.T) {
	db := newReminderTestDB(t)
	t0 := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(db, t0)
	createSite(t, m, cookieSite("HDSKY"))
	m.resolver = &fakeReminderResolver{def: nexusDef(), site: &fakeReminderSite{err: fmt.Errorf("dial: %w", v2.ErrNetworkError)}}

	for i, want := range []time.Duration{time.Hour, 2 * time.Hour, 4 * time.Hour, 6 * time.Hour} {
		m.RunProbeOnce(context.Background())
		st := loadState(t, m, "HDSKY")
		require.NotNil(t, st.NextProbeAt)
		assert.Equal(t, i+1, st.ConsecutiveProbeFailures)
		assert.Equal(t, want, st.NextProbeAt.Sub(m.clock.Now()), "failure %d", i+1)
		require.NotNil(t, st.FirstFailureAt)
		assert.True(t, st.FirstFailureAt.Equal(t0), "FirstFailureAt marks the start of the streak")
		advanceClock(m, want)
	}
}

// 问题 4（写入侧）：扩展上报的较新访问在探测失败时让剩余天数回升，不再误报封号。
func TestReminder_VisitWhileFailingSuppressesFalseBanWarning(t *testing.T) {
	db := newReminderTestDB(t)
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(db, now)
	createSite(t, m, models.SiteSetting{Name: "HDSKY", Enabled: true})
	access := now.Add(-28 * 24 * time.Hour) // 2 days left by site data
	visit := now.Add(-24 * time.Hour)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName: "HDSKY", BanThresholdDays: 30, RemindBeforeDays: 10, ReminderCron: "* * * * *",
		LastReminderTier: tierNone, LastAccessAt: &access, LastVisitAt: &visit, LastProbeStatus: "SESSION_EXPIRED",
	}).Error)
	rec := attachCaptureNotifier(t, m, db)
	m.evaluateReminder(context.Background(), models.SiteSetting{Name: "HDSKY"}, now)
	m.notifier.DeliverDue(context.Background())
	assert.Equal(t, int32(0), rec.count.Load(), "29 days remain counting the browser visit, so no ban warning")
}

// 问题 5：探测进行中修改配置，探测结束后配置仍是新值；提醒循环写入也不会被探测回滚。
func TestProbe_ConcurrentConfigAndReminderWritesSurvive(t *testing.T) {
	db := newReminderTestDB(t)
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(db, now)
	createSite(t, m, cookieSite("HDSKY"))
	old := now.Add(-25 * 24 * time.Hour)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName: "HDSKY", BanThresholdDays: 30, RemindBeforeDays: 10, ReminderCron: "* * * * *",
		LastReminderTier: tierNone, ProbeMode: ProbeModeAuto, LastAccessAt: &old, LastProbeStatus: "OK",
	}).Error)
	site := &countingSite{
		fakeReminderSite: fakeReminderSite{info: v2.UserInfo{LastAccess: now.Unix()}},
		block:            make(chan struct{}),
		entered:          make(chan struct{}, 1),
	}
	m.resolver = &fakeReminderResolver{def: nexusDef(), site: site}

	done := make(chan struct{})
	go func() { m.RunProbeOnceForSite(context.Background(), "HDSKY"); close(done) }()
	<-site.entered
	require.NoError(t, models.NewSiteLoginStateRepository(db).UpsertLoginState("HDSKY", map[string]any{"ProbeMode": "disabled", "RemindBeforeDays": 12}))
	m.evaluateReminder(context.Background(), models.SiteSetting{Name: "HDSKY"}, now) // 7d tier → writes reminder columns
	close(site.block)
	<-done

	st := loadState(t, m, "HDSKY")
	assert.Equal(t, "disabled", st.ProbeMode, "a config change made during the probe is not reverted")
	assert.Equal(t, 12, st.RemindBeforeDays)
	require.NotNil(t, st.LastReminderSentAt, "the reminder loop's write is not reverted by the probe")
	assert.NotEqual(t, tierNone, st.LastReminderTier)
	require.NotNil(t, st.LastAccessAt)
	assert.Equal(t, now.Unix(), st.LastAccessAt.Unix(), "the probe's own columns are written")
}

// 问题 6：缺凭证不发请求、动态站点不调用 Resolve、Resolve 出错可见。
func TestProbe_PrecheckMissingCredential(t *testing.T) {
	cases := []struct {
		name    string
		setting models.SiteSetting
		diag    string
	}{
		{"cookie", models.SiteSetting{Name: "A", Enabled: true, AuthMethod: "cookie"}, "Cookie"},
		{"api_key", models.SiteSetting{Name: "B", Enabled: true, AuthMethod: "api_key", Cookie: "c=1"}, "API Key"},
		{"cookie_and_api_key", models.SiteSetting{Name: "C", Enabled: true, AuthMethod: "cookie_and_api_key", APIKey: "k"}, "Cookie"},
		{"passkey", models.SiteSetting{Name: "D", Enabled: true, AuthMethod: "passkey"}, "Passkey"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newReminderTestDB(t)
			m := newReminderMonitorForTest(db, time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))
			createSite(t, m, tc.setting)
			site := &countingSite{}
			resolver := &countingResolver{def: nexusDef(), site: site}
			m.resolver = resolver
			m.RunProbeOnce(context.Background())
			assert.Equal(t, int32(0), site.calls.Load(), "no request without credentials")
			assert.Equal(t, int32(0), resolver.calls.Load())
			st := loadState(t, m, tc.setting.Name)
			assert.Equal(t, "NOT_CONFIGURED", st.LastProbeStatus)
			assert.Contains(t, st.LastProbeError, tc.diag)
			assert.Equal(t, 0, st.ConsecutiveProbeFailures, "missing credentials are not counted as failures")
			assert.Nil(t, st.FirstFailureAt)
		})
	}
}

func TestProbe_PrecheckPassesWithCredentials(t *testing.T) {
	ok := []models.SiteSetting{
		{AuthMethod: "cookie", CookieEncrypted: "enc"},
		{AuthMethod: "api_key", APIKey: "k"},
		{AuthMethod: "cookie_and_api_key", Cookie: "c", APIKey: "k"},
		{AuthMethod: "passkey", Passkey: "p"},
		{AuthMethod: ""},
	}
	for _, s := range ok {
		assert.Empty(t, missingCredential(s), "auth=%q", s.AuthMethod)
	}
}

func TestProbe_DynamicSiteUnsupported(t *testing.T) {
	db := newReminderTestDB(t)
	m := newReminderMonitorForTest(db, time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))
	m.lookupDefinition = func(string) (*v2.SiteDefinition, bool) { return nil, false }
	createSite(t, m, cookieSite("mydynamic"))
	resolver := &countingResolver{def: nexusDef(), site: &countingSite{}}
	m.resolver = resolver
	m.RunProbeOnce(context.Background())
	assert.Equal(t, int32(0), resolver.calls.Load(), "dynamic sites are not resolved")
	st := loadState(t, m, "mydynamic")
	assert.Equal(t, "UNSUPPORTED", st.LastProbeStatus)
	assert.Equal(t, 0, st.ConsecutiveProbeFailures)
	require.NotNil(t, st.NextProbeAt)
	assert.Equal(t, 24*time.Hour, st.NextProbeAt.Sub(m.clock.Now()))
}

// 取锁后重读配置：循环开头读到的是旧配置（没有凭证），按新配置执行。
func TestProbe_RereadsConfigAfterLock(t *testing.T) {
	db := newReminderTestDB(t)
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(db, now)
	createSite(t, m, cookieSite("HDSKY")) // DB already has the cookie
	site := &countingSite{fakeReminderSite: fakeReminderSite{info: v2.UserInfo{LastAccess: now.Unix()}}}
	m.resolver = &fakeReminderResolver{def: nexusDef(), site: site}

	stale := models.SiteSetting{Name: "HDSKY", Enabled: true, AuthMethod: "cookie"} // snapshot without cookie
	m.probeSiteInternal(context.Background(), stale, false)
	assert.Equal(t, int32(1), site.calls.Load())
	st := loadState(t, m, "HDSKY")
	assert.Equal(t, "OK", st.LastProbeStatus, "the fresh row, not the stale snapshot, decides the pre-check")
	require.NotNil(t, st.LastProbeStartedAt)
	assert.False(t, st.LastProbeStartedAt.After(*st.LastProbeAt))
}

// 问题 7：一个卡住的站点在预算内返回，循环继续处理下一个站点。
func TestProbe_HangingSiteHonoursBudget(t *testing.T) {
	db := newReminderTestDB(t)
	m := newReminderMonitorForTest(db, time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC))
	m.primaryTimeout = 100 * time.Millisecond
	m.probeBudget = 300 * time.Millisecond
	createSite(t, m, cookieSite("A"))
	createSite(t, m, cookieSite("B"))
	site := &countingSite{block: make(chan struct{})}
	defer close(site.block)
	m.resolver = &fakeReminderResolver{def: nexusDef(), site: site}

	done := make(chan struct{})
	go func() { m.RunProbeOnce(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a hanging site must not block the probe loop")
	}
	assert.Equal(t, int32(2), site.calls.Load(), "both sites were attempted")
	for _, name := range []string{"A", "B"} {
		assert.Equal(t, "NETWORK_ERROR", loadState(t, m, name).LastProbeStatus)
	}
}

func TestProbe_AccessStaleSinceTracksNonAdvancingAccess(t *testing.T) {
	db := newReminderTestDB(t)
	t0 := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(db, t0)
	createSite(t, m, cookieSite("HDSKY"))
	access := t0.Add(-72 * time.Hour)
	site := &fakeReminderSite{info: v2.UserInfo{LastAccess: access.Unix()}}
	m.resolver = &fakeReminderResolver{def: nexusDef(), site: site}

	m.RunProbeOnce(context.Background())
	assert.Nil(t, loadState(t, m, "HDSKY").AccessStaleSince, "first observation has nothing to compare with")

	advanceClock(m, 6*time.Hour)
	m.RunProbeOnce(context.Background())
	st := loadState(t, m, "HDSKY")
	require.NotNil(t, st.AccessStaleSince, "same last_access older than 48h → access not advancing")
	staleSince := *st.AccessStaleSince

	advanceClock(m, 6*time.Hour)
	m.RunProbeOnce(context.Background())
	assert.True(t, loadState(t, m, "HDSKY").AccessStaleSince.Equal(staleSince), "the flag keeps its first detection time")

	site.info.LastAccess = m.clock.Now().Unix()
	advanceClock(m, time.Minute)
	m.RunProbeOnce(context.Background())
	assert.Nil(t, loadState(t, m, "HDSKY").AccessStaleSince, "advancing last_access clears the flag")
}

// 站点返回的最近访问时间倒退（比上次还早）不算前进，「访问未生效」不被清除。
func TestProbe_AccessStaleSinceKeptWhenAccessGoesBackwards(t *testing.T) {
	db := newReminderTestDB(t)
	t0 := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(db, t0)
	createSite(t, m, cookieSite("HDSKY"))
	access := t0.Add(-72 * time.Hour)
	site := &fakeReminderSite{info: v2.UserInfo{LastAccess: access.Unix()}}
	m.resolver = &fakeReminderResolver{def: nexusDef(), site: site}

	m.RunProbeOnce(context.Background())
	advanceClock(m, 6*time.Hour)
	m.RunProbeOnce(context.Background())
	st := loadState(t, m, "HDSKY")
	require.NotNil(t, st.AccessStaleSince)
	staleSince := *st.AccessStaleSince

	site.info.LastAccess = access.Add(-time.Hour).Unix()
	advanceClock(m, 6*time.Hour)
	m.RunProbeOnce(context.Background())
	st = loadState(t, m, "HDSKY")
	require.NotNil(t, st.AccessStaleSince, "an older last_access is not progress")
	assert.True(t, st.AccessStaleSince.Equal(staleSince))
}

func TestReminder_AccessStaleHintInText(t *testing.T) {
	db := newReminderTestDB(t)
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	m := newReminderMonitorForTest(db, now)
	createSite(t, m, models.SiteSetting{Name: "HDSKY", Enabled: true})
	access := now.Add(-25 * 24 * time.Hour)
	stale := now.Add(-time.Hour)
	require.NoError(t, db.Create(&models.SiteLoginState{
		SiteName: "HDSKY", BanThresholdDays: 30, RemindBeforeDays: 10, ReminderCron: "* * * * *",
		LastReminderTier: tierNone, LastAccessAt: &access, LastProbeStatus: "OK", AccessStaleSince: &stale,
	}).Error)
	rec := attachCaptureNotifier(t, m, db)
	m.evaluateReminder(context.Background(), models.SiteSetting{Name: "HDSKY"}, now)
	m.notifier.DeliverDue(context.Background())
	require.Equal(t, int32(1), rec.count.Load())
	_, text := rec.lastMessage()
	assert.Contains(t, text, "自动访问对该站无效")
	assert.Contains(t, text, "站点最近访问")
}

func TestEffectiveActiveSource_ReportsVisit(t *testing.T) {
	now := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	access := now.Add(-10 * 24 * time.Hour)
	visit := now.Add(-time.Hour)
	st := &models.SiteLoginState{LastAccessAt: &access, LastVisitAt: &visit, LastProbeStatus: "CHALLENGE"}
	eff := EffectiveLastActive(st, now)
	assert.Equal(t, "last_visit", EffectiveActiveSource(st, eff))
	assert.Equal(t, "浏览器访问", activitySourceLabel(EffectiveActiveSource(st, eff)))
	st.LastProbeStatus = "OK"
	eff = EffectiveLastActive(st, now)
	assert.Equal(t, "last_access", EffectiveActiveSource(st, eff))
}

func TestFailureNoticeText_TruncatesDiagnostic(t *testing.T) {
	long := strings.Repeat("错", 500)
	assert.Equal(t, 201, len([]rune(truncateRunes(long, 200))))
	assert.Equal(t, "短", truncateRunes("短", 200))
}
