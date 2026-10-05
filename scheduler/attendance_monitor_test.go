package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// fakeAttendSite 是能签到的假站点：unsupported 时 SupportsAttendance 为 false。
type fakeAttendSite struct {
	result      v2.AttendResult
	err         error
	unsupported bool
	calls       atomic.Int32
}

func (s *fakeAttendSite) SupportsAttendance() bool { return !s.unsupported }

func (s *fakeAttendSite) Attend(context.Context) (v2.AttendResult, error) {
	s.calls.Add(1)
	return s.result, s.err
}

type fixedWindow struct{ start, end string }

func (w fixedWindow) AttendanceWindow() (string, string, error) { return w.start, w.end, nil }

type attendanceRig struct {
	db     *gorm.DB
	clock  *sitelogin.FakeClock
	probe  *LoginReminderMonitor
	mon    *AttendanceMonitor
	sites  map[string]*fakeAttendSite
	sender *captureSender
}

// newAttendanceRig 用 UTC 作进程时区、时间窗 08:00–10:00，随机数取区间中点，方便断言计划时刻。
func newAttendanceRig(t *testing.T, now time.Time, siteNames ...string) *attendanceRig {
	t.Helper()
	db := newReminderTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.SiteAttendanceLog{}))
	rig := &attendanceRig{db: db, clock: sitelogin.NewFakeClock(now), sites: map[string]*fakeAttendSite{}}
	rig.probe = newReminderMonitorForTest(db, now)
	rig.probe.clock = rig.clock
	for _, name := range siteNames {
		require.NoError(t, db.Create(&models.SiteSetting{Name: name, Enabled: true, AuthMethod: "cookie", Cookie: "c=1", AttendanceEnabled: true}).Error)
		rig.sites[name] = &fakeAttendSite{result: v2.AttendResult{Status: v2.AttendSigned, Message: "这是您的第 3 次签到"}}
	}
	conf := quietConf(1, "", "")
	require.NoError(t, db.Create(&conf).Error)
	rig.sender = &captureSender{}
	notifier := NewMonitorNotifier(db, rig.sender, rig.clock, zap.NewNop().Sugar())
	rig.mon = NewAttendanceMonitor(AttendanceMonitorConfig{
		DB: db,
		Sites: AttendanceSitesFunc(func(name string) (v2.AttendanceCapable, bool) {
			s, ok := rig.sites[name]
			return s, ok
		}),
		Window:   fixedWindow{"08:00", "10:00"},
		Locker:   rig.probe,
		Notifier: notifier,
		Clock:    rig.clock,
		Location: time.UTC,
		Rand:     func(n int64) int64 { return n / 2 },
	})
	return rig
}

func (r *attendanceRig) row(t *testing.T, site string) models.SiteAttendanceLog {
	t.Helper()
	row, err := models.NewSiteAttendanceRepository(r.db).GetDay(site, r.clock.Now().UTC().Format("2006-01-02"))
	require.NoError(t, err)
	return *row
}

func (r *attendanceRig) at(hhmm string) {
	now := r.clock.Now().UTC()
	target, _ := time.Parse("2006-01-02 15:04", now.Format("2006-01-02")+" "+hhmm)
	r.clock.Advance(target.Sub(now))
}

// M1c：每站每天一次，在时间窗内取时刻；当天成功后不再请求，第二天重新安排。
func TestAttendance_OncePerDayInsideWindow(t *testing.T) {
	rig := newAttendanceRig(t, time.Date(2026, 10, 5, 7, 0, 0, 0, time.UTC), "hdtime")
	ctx := context.Background()

	rig.mon.RunOnce(ctx)
	row := rig.row(t, "hdtime")
	assert.Equal(t, models.AttendancePending, row.Status)
	assert.True(t, row.ScheduledAt.Equal(time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)), "midpoint of 08:00–10:00, got %s", row.ScheduledAt)
	assert.Zero(t, rig.sites["hdtime"].calls.Load(), "nothing before the scheduled time")

	rig.at("08:59")
	rig.mon.RunOnce(ctx)
	assert.Zero(t, rig.sites["hdtime"].calls.Load())

	rig.at("09:00")
	rig.mon.RunOnce(ctx)
	assert.Equal(t, int32(1), rig.sites["hdtime"].calls.Load())
	row = rig.row(t, "hdtime")
	assert.Equal(t, models.AttendanceSigned, row.Status)
	assert.Contains(t, row.Message, "第 3 次签到")

	rig.at("09:30")
	rig.mon.RunOnce(ctx)
	rig.at("23:59")
	rig.mon.RunOnce(ctx)
	assert.Equal(t, int32(1), rig.sites["hdtime"].calls.Load(), "signed today, no more requests")

	rig.clock.Advance(7*time.Hour + time.Minute) // 次日 07:00，重新安排到 09:00
	rig.mon.RunOnce(ctx)
	rig.at("09:00")
	rig.mon.RunOnce(ctx)
	assert.Equal(t, int32(2), rig.sites["hdtime"].calls.Load(), "a new day, a new sign-in")
}

// 过了时间窗才开启（或进程启动）时，在接下来的 10 分钟内签到，不等到第二天。
func TestAttendance_AfterWindowSignsSoon(t *testing.T) {
	rig := newAttendanceRig(t, time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC), "hdtime")
	ctx := context.Background()
	rig.mon.RunOnce(ctx)
	assert.True(t, rig.row(t, "hdtime").ScheduledAt.Equal(time.Date(2026, 10, 5, 12, 5, 0, 0, time.UTC)))
	rig.at("12:05")
	rig.mon.RunOnce(ctx)
	assert.Equal(t, int32(1), rig.sites["hdtime"].calls.Load())
}

// 失败退避重试最多 2 次（10 分钟、30 分钟），第 3 次失败记为 failed，当天不再请求。
func TestAttendance_RetriesTwiceThenFails(t *testing.T) {
	rig := newAttendanceRig(t, time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC), "hdtime")
	ctx := context.Background()
	rig.sites["hdtime"].err = fmt.Errorf("attendance request: %w", v2.ErrSessionExpired)
	rig.mon.Rand = func(int64) int64 { return 0 }

	rig.mon.RunOnce(ctx) // 09:00 第 1 次
	row := rig.row(t, "hdtime")
	assert.Equal(t, 1, row.Attempts)
	require.NotNil(t, row.NextAttemptAt)
	assert.Equal(t, 10*time.Minute, row.NextAttemptAt.Sub(rig.clock.Now()))
	assert.Contains(t, row.LastError, "Cookie", "a session error is explained in plain words")

	rig.at("09:09")
	rig.mon.RunOnce(ctx)
	assert.Equal(t, int32(1), rig.sites["hdtime"].calls.Load(), "waits for the backoff")
	rig.at("09:10") // 第 2 次
	rig.mon.RunOnce(ctx)
	rig.at("09:40") // 第 3 次
	rig.mon.RunOnce(ctx)
	row = rig.row(t, "hdtime")
	assert.Equal(t, models.AttendanceFailed, row.Status)
	assert.Equal(t, 3, row.Attempts)

	rig.at("12:00")
	rig.mon.RunOnce(ctx)
	assert.Equal(t, int32(3), rig.sites["hdtime"].calls.Load(), "no more attempts after the 3rd failure")
}

// 不支持签到的站点（验证码、答题等）记为 unsupported，不发请求、不重试。
func TestAttendance_UnsupportedIsFinalWithoutRequest(t *testing.T) {
	rig := newAttendanceRig(t, time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC), "hdsky")
	rig.sites["hdsky"].unsupported = true
	rig.mon.Rand = func(int64) int64 { return 0 }
	rig.mon.RunOnce(context.Background())
	assert.Equal(t, models.AttendanceUnsupported, rig.row(t, "hdsky").Status)
	assert.Zero(t, rig.sites["hdsky"].calls.Load())
}

// 站点实例没有注册（凭证缺失等）按失败处理并说明原因。
func TestAttendance_UnregisteredSiteFails(t *testing.T) {
	rig := newAttendanceRig(t, time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC), "hdtime")
	delete(rig.sites, "hdtime")
	rig.mon.Rand = func(int64) int64 { return 0 }
	rig.mon.RunOnce(context.Background())
	row := rig.row(t, "hdtime")
	assert.Equal(t, 1, row.Attempts)
	assert.Contains(t, row.LastError, "没有注册")
}

// 与登录探测共用单站单飞锁：探测进行中这一轮跳过，锁释放后下一轮签到。
func TestAttendance_SharesSingleFlightWithProbe(t *testing.T) {
	rig := newAttendanceRig(t, time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC), "hdtime")
	rig.mon.Rand = func(int64) int64 { return 0 }
	release, ok := rig.probe.TryAcquireProbeLock("hdtime")
	require.True(t, ok)
	rig.mon.RunOnce(context.Background())
	assert.Zero(t, rig.sites["hdtime"].calls.Load(), "a probe holds the site")
	assert.Equal(t, models.AttendancePending, rig.row(t, "hdtime").Status)

	_, err := rig.mon.SignNow(context.Background(), "hdtime")
	assert.ErrorIs(t, err, ErrAttendanceBusy, "a manual sign-in reports the conflict")

	release()
	rig.mon.RunOnce(context.Background())
	assert.Equal(t, int32(1), rig.sites["hdtime"].calls.Load())
}

// 只处理启用且打开了自动签到的站点。
func TestAttendance_OnlyEnabledSites(t *testing.T) {
	rig := newAttendanceRig(t, time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC), "hdtime")
	rig.mon.Rand = func(int64) int64 { return 0 }
	require.NoError(t, rig.db.Create(&models.SiteSetting{Name: "off", Enabled: true, AuthMethod: "cookie", Cookie: "c"}).Error)
	require.NoError(t, rig.db.Create(&models.SiteSetting{Name: "disabled", Enabled: false, AuthMethod: "cookie", Cookie: "c", AttendanceEnabled: true}).Error)
	rig.sites["off"] = &fakeAttendSite{}
	rig.sites["disabled"] = &fakeAttendSite{}
	rig.mon.RunOnce(context.Background())
	assert.Equal(t, int32(1), rig.sites["hdtime"].calls.Load())
	assert.Zero(t, rig.sites["off"].calls.Load())
	assert.Zero(t, rig.sites["disabled"].calls.Load())
}

// 当天所有开启签到的站点都有了结果，才合成一条通知；重复检查不会重复发。
func TestAttendance_DailySummaryOnceWhenAllDone(t *testing.T) {
	rig := newAttendanceRig(t, time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC), "hdtime", "pthome")
	rig.mon.Rand = func(int64) int64 { return 0 }
	rig.sites["pthome"].err = errors.New("HTTP 502: Bad Gateway")
	ctx := context.Background()
	deliver := func() { rig.mon.notifier.DeliverDue(ctx) }

	rig.mon.RunOnce(ctx) // hdtime 成功，pthome 第 1 次失败
	deliver()
	assert.Zero(t, rig.sender.count.Load(), "pthome is still retrying")

	rig.at("09:10")
	rig.mon.RunOnce(ctx)
	rig.at("09:40")
	rig.mon.RunOnce(ctx) // pthome 第 3 次失败
	deliver()
	require.Equal(t, int32(1), rig.sender.count.Load())
	title, text := rig.sender.lastMessage()
	assert.Contains(t, title, "签到")
	assert.Contains(t, text, "hdtime")
	assert.Contains(t, text, "签到成功")
	assert.Contains(t, text, "pthome")
	assert.Contains(t, text, "失败")

	rig.at("10:00")
	rig.mon.RunOnce(ctx)
	deliver()
	assert.Equal(t, int32(1), rig.sender.count.Load(), "one summary per day")
}

// 手动签到：没开启自动签到的站点也能签；手动失败不占用自动重试次数。
func TestAttendance_SignNow(t *testing.T) {
	rig := newAttendanceRig(t, time.Date(2026, 10, 5, 7, 0, 0, 0, time.UTC), "hdtime")
	require.NoError(t, rig.db.Create(&models.SiteSetting{Name: "manual", Enabled: true, AuthMethod: "cookie", Cookie: "c"}).Error)
	rig.sites["manual"] = &fakeAttendSite{result: v2.AttendResult{Status: v2.AttendAlready, Message: "您今天已经签到过了"}}

	row, err := rig.mon.SignNow(context.Background(), "manual")
	require.NoError(t, err)
	assert.Equal(t, models.AttendanceAlready, row.Status)

	rig.sites["hdtime"].err = errors.New("HTTP 502: Bad Gateway")
	row, err = rig.mon.SignNow(context.Background(), "hdtime")
	require.NoError(t, err, "the attempt itself ran; its failure is in the row")
	assert.Equal(t, models.AttendancePending, row.Status, "a manual failure leaves the automatic schedule alone")
	assert.Zero(t, row.Attempts)
	assert.Contains(t, row.LastError, "502")

	_, err = rig.mon.SignNow(context.Background(), "no-such-site")
	assert.ErrorContains(t, err, "站点不存在", "a typo does not create a record")
	_, err = models.NewSiteAttendanceRepository(rig.db).GetDay("no-such-site", rig.mon.Today())
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

	rig.sites["hdtime"].err = nil
	results, err := rig.mon.SignAll(context.Background())
	require.NoError(t, err)
	require.Len(t, results, 1, "only sites with automatic sign-in enabled")
	assert.Equal(t, models.AttendanceSigned, results[0].Status)
}

func TestAttendanceMonitor_StartStopAndManager(t *testing.T) {
	rig := newAttendanceRig(t, time.Now(), "hdtime")
	rig.mon.Start()
	rig.mon.Start()
	rig.mon.Stop()
	rig.mon.Stop()

	var nilMon *AttendanceMonitor
	nilMon.Start()
	nilMon.Stop()
	nilMon.RunOnce(context.Background())
	_, err := nilMon.SignNow(context.Background(), "hdtime")
	assert.Error(t, err)

	mgr := NewManager()
	t.Cleanup(mgr.StopAll)
	assert.Nil(t, mgr.GetAttendanceMonitor())
	mgr.SetAttendanceMonitor(rig.mon)
	assert.Same(t, rig.mon, mgr.GetAttendanceMonitor())
}

// 零值配置用默认的时钟、时区、间隔与随机数；默认随机数落在 [0, n)。
func TestAttendanceMonitor_Defaults(t *testing.T) {
	m := NewAttendanceMonitor(AttendanceMonitorConfig{})
	assert.Equal(t, time.Local, m.loc)
	assert.Equal(t, attendanceTick, m.tick)
	for range 20 {
		v := m.Rand(10)
		assert.True(t, v >= 0 && v < 10)
	}
	assert.Zero(t, m.Rand(0))
	m.RunOnce(context.Background()) // 没有数据库：直接返回
	_, err := m.SignAll(context.Background())
	assert.Error(t, err)
	assert.Equal(t, time.Now().Format("2006-01-02"), m.Today())
}

// 时间窗不可用（开始不早于结束）时用 08:00–10:00。
func TestAttendance_InvalidWindowFallsBack(t *testing.T) {
	rig := newAttendanceRig(t, time.Date(2026, 10, 5, 7, 0, 0, 0, time.UTC), "hdtime")
	rig.mon.window = fixedWindow{"10:00", "09:00"}
	rig.mon.RunOnce(context.Background())
	assert.True(t, rig.row(t, "hdtime").ScheduledAt.Equal(time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)))
}

// 记录表不可用时这一轮只记告警；取消的上下文让签到中途返回。
func TestAttendance_ErrorPaths(t *testing.T) {
	rig := newAttendanceRig(t, time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC), "hdtime", "pthome")
	rig.mon.Rand = func(int64) int64 { return 0 }

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rig.mon.RunOnce(ctx)
	assert.Zero(t, rig.sites["hdtime"].calls.Load(), "a cancelled round stops before signing in")
	out, err := rig.mon.SignAll(ctx)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, out)

	require.NoError(t, rig.db.Migrator().DropTable(&models.SiteAttendanceLog{}))
	rig.mon.RunOnce(context.Background())
	_, err = rig.mon.SignNow(context.Background(), "hdtime")
	assert.Error(t, err)
	out, err = rig.mon.SignAll(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 2)
	assert.NotEmpty(t, out[0].LastError, "a site whose sign-in could not run reports why")

	require.NoError(t, rig.db.Migrator().DropTable(&models.SiteSetting{}))
	rig.mon.RunOnce(context.Background())
	_, err = rig.mon.SignAll(context.Background())
	assert.Error(t, err)
}
