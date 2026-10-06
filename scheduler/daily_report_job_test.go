package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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

type dailyReportFixture struct {
	db     *gorm.DB
	repo   *v2.DBUserInfoRepo
	clock  *sitelogin.FakeClock
	sender *captureSender
	cfg    DailyReportJobConfig
	// settings 可以在用例里改
	enabled  bool
	at       string
	channels []uint
}

func newDailyReportFixture(t *testing.T, now time.Time, confs ...models.NotificationConf) *dailyReportFixture {
	t.Helper()
	db := newReminderTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.SiteAttendanceLog{}))
	for i := range confs {
		require.NoError(t, db.Create(&confs[i]).Error)
	}
	repo, err := v2.NewDBUserInfoRepo(db)
	require.NoError(t, err)
	clock := sitelogin.NewFakeClock(now)
	repo.SetClock(clock.Now, time.UTC)
	sender := &captureSender{}
	f := &dailyReportFixture{db: db, repo: repo, clock: clock, sender: sender, enabled: true, at: "22:00", channels: []uint{1}}
	f.cfg = DailyReportJobConfig{
		DB: db,
		Settings: func() (bool, string, []uint, error) {
			return f.enabled, f.at, f.channels, nil
		},
		Offset:   func() (time.Duration, error) { return 5 * time.Minute, nil },
		History:  repo,
		Notifier: NewMonitorNotifier(db, sender, clock, zap.NewNop().Sugar()),
		Clock:    clock,
		Location: time.UTC,
		Logger:   zap.NewNop().Sugar(),
	}
	return f
}

func (f *dailyReportFixture) saveAt(t *testing.T, at time.Time, site string, up, down int64, bonus float64) {
	t.Helper()
	f.repo.SetClock(func() time.Time { return at }, time.UTC)
	require.NoError(t, f.repo.Save(context.Background(), v2.UserInfo{Site: site, Username: "u", Uploaded: up, Downloaded: down, Bonus: bonus}))
	f.repo.SetClock(f.clock.Now, time.UTC)
}

func reportRows(t *testing.T, db *gorm.DB) []models.MonitorNotificationLog {
	t.Helper()
	var rows []models.MonitorNotificationLog
	require.NoError(t, db.Where("source = ?", dailyReportSource).Order("notification_conf_id").Find(&rows).Error)
	return rows
}

func reportText(t *testing.T, row models.MonitorNotificationLog) string {
	t.Helper()
	var p monitorNotifyPayload
	require.NoError(t, json.Unmarshal([]byte(row.PayloadJSON), &p))
	return p.Title + "\n" + p.Text
}

// 设定时刻加偏移之前不发；之后每个通道只发一次；重启（新实例）也不重发。
func TestDailyReport_SendsOncePerChannelAfterDueTime(t *testing.T) {
	f := newDailyReportFixture(t, time.Date(2026, 10, 6, 22, 4, 0, 0, time.UTC), quietConf(1, "", ""), quietConf(2, "", ""))
	f.channels = []uint{1, 2}
	job := NewDailyReportJob(f.cfg)

	job.RunOnce(context.Background())
	assert.Empty(t, reportRows(t, f.db), "22:00 + 5 分钟偏移之前不发")

	f.clock.Advance(2 * time.Minute)
	job.RunOnce(context.Background())
	rows := reportRows(t, f.db)
	require.Len(t, rows, 2, "每个通道一行")
	assert.Equal(t, "2026-10-06", rows[0].EventKey)

	job.RunOnce(context.Background())
	NewDailyReportJob(f.cfg).RunOnce(context.Background())
	assert.Len(t, reportRows(t, f.db), 2, "同一天不重发，重启也不重发")

	f.clock.Advance(24 * time.Hour)
	job.RunOnce(context.Background())
	assert.Len(t, reportRows(t, f.db), 4, "第二天再发一次")
}

func TestDailyReport_DisabledOrWithoutChannelsDoesNothing(t *testing.T) {
	f := newDailyReportFixture(t, time.Date(2026, 10, 6, 23, 0, 0, 0, time.UTC), quietConf(1, "", ""))
	f.enabled = false
	NewDailyReportJob(f.cfg).RunOnce(context.Background())
	assert.Empty(t, reportRows(t, f.db))

	f.enabled = true
	f.channels = nil
	NewDailyReportJob(f.cfg).RunOnce(context.Background())
	assert.Empty(t, reportRows(t, f.db), "开启但没选通道不发（不沿用空列表即全部通道）")
}

// 通道在自己的静默时段里：这一行顺延到静默结束。
func TestDailyReport_QuietChannelDeferred(t *testing.T) {
	now := time.Date(2026, 10, 6, 22, 10, 0, 0, time.UTC)
	f := newDailyReportFixture(t, now, quietConf(1, "", ""), quietConf(2, "22:00", "08:00"))
	f.channels = []uint{1, 2}
	NewDailyReportJob(f.cfg).RunOnce(context.Background())

	rows := reportRows(t, f.db)
	require.Len(t, rows, 2)
	assert.False(t, rows[0].NextRetryAt.After(now), "非静默通道立即发")
	assert.True(t, rows[1].NextRetryAt.After(now), "静默通道顺延到静默结束")
}

// 战报内容：当天合计与各站增量、登录状态异常的站点、签到结果、数据回退的站点。
func TestDailyReport_Content(t *testing.T) {
	now := time.Date(2026, 10, 6, 22, 30, 0, 0, time.UTC)
	f := newDailyReportFixture(t, now, quietConf(1, "", ""))
	yesterday := now.Add(-24 * time.Hour)
	gib := int64(1 << 30)
	f.saveAt(t, yesterday, "hdsky", 10*gib, 2*gib, 1000)
	f.saveAt(t, now.Add(-time.Hour), "hdsky", 15*gib, 3*gib, 1500)
	f.saveAt(t, yesterday, "pter", 8*gib, gib, 500)
	f.saveAt(t, now.Add(-time.Hour), "pter", 6*gib, gib, 600) // 上传回退
	require.NoError(t, f.db.Create(&models.SiteLoginState{SiteName: "pter", LastProbeStatus: "SESSION_EXPIRED"}).Error)
	require.NoError(t, f.db.Create(&models.SiteLoginState{SiteName: "hdsky", LastProbeStatus: "OK"}).Error)
	for _, a := range []models.SiteAttendanceLog{
		{SiteName: "hdsky", Day: "2026-10-06", Status: models.AttendanceSigned},
		{SiteName: "pter", Day: "2026-10-06", Status: models.AttendanceFailed},
	} {
		require.NoError(t, f.db.Create(&a).Error)
	}

	NewDailyReportJob(f.cfg).RunOnce(context.Background())

	rows := reportRows(t, f.db)
	require.Len(t, rows, 1)
	text := reportText(t, rows[0])
	assert.Contains(t, text, "2026-10-06")
	assert.Contains(t, text, "上传 5.00 GiB", "当天合计只算 hdsky 的上传（pter 回退不计）")
	assert.Contains(t, text, "HDSky")
	assert.Contains(t, text, "会话过期")
	assert.Contains(t, text, "签到")
	assert.Contains(t, text, "成功 1")
	assert.Contains(t, text, "失败 1")
	assert.Contains(t, text, "回退")
}

func TestDailyReport_NoDataStillReports(t *testing.T) {
	f := newDailyReportFixture(t, time.Date(2026, 10, 6, 22, 30, 0, 0, time.UTC), quietConf(1, "", ""))
	NewDailyReportJob(f.cfg).RunOnce(context.Background())
	rows := reportRows(t, f.db)
	require.Len(t, rows, 1)
	assert.Contains(t, reportText(t, rows[0]), "还没有")
}

// 设定时刻加偏移跨过午夜时按当天 23:59 发，不会漏掉这一天。
func TestDailyReport_DueTimeCappedBeforeMidnight(t *testing.T) {
	f := newDailyReportFixture(t, time.Date(2026, 10, 6, 23, 59, 0, 0, time.UTC), quietConf(1, "", ""))
	f.at = "23:58"
	NewDailyReportJob(f.cfg).RunOnce(context.Background())
	assert.Len(t, reportRows(t, f.db), 1)
}

// 快照只保留 400 天：每天清理一次，关闭战报时也清。
func TestDailyReport_PrunesOldSnapshots(t *testing.T) {
	now := time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)
	f := newDailyReportFixture(t, now, quietConf(1, "", ""))
	f.enabled = false
	f.saveAt(t, now.AddDate(0, 0, -401), "hdsky", 1, 1, 1)
	f.saveAt(t, now.AddDate(0, 0, -10), "hdsky", 2, 2, 2)

	NewDailyReportJob(f.cfg).RunOnce(context.Background())

	left, err := f.repo.ListSnapshots(context.Background(), "hdsky", "2000-01-01", "2100-01-01")
	require.NoError(t, err)
	require.Len(t, left, 1)
	assert.Equal(t, now.AddDate(0, 0, -10).Format("2006-01-02"), left[0].Date)
}

// 发送时刻按当地墙上时间算：夏令时开始那天（少一小时）22:00 仍是 22:00，不会被推到 23:00。
func TestDailyReport_DueTimeUsesWallClockOnDSTDay(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	f := newDailyReportFixture(t, time.Date(2026, 3, 8, 22, 4, 0, 0, ny), quietConf(1, "", ""))
	f.cfg.Location = ny
	f.repo.SetClock(f.clock.Now, ny)
	job := NewDailyReportJob(f.cfg)

	job.RunOnce(context.Background())
	assert.Empty(t, reportRows(t, f.db), "22:00 + 5 分钟之前不发")
	f.clock.Advance(2 * time.Minute)
	job.RunOnce(context.Background())
	rows := reportRows(t, f.db)
	require.Len(t, rows, 1, "当地 22:06 已经过了发送时刻")
	assert.Equal(t, "2026-03-08", rows[0].EventKey)
}

// 偏移读不出来（首次分配时写库失败）时这一轮不发，不当成 0 偏移提前发。
func TestDailyReport_OffsetErrorSkipsRound(t *testing.T) {
	f := newDailyReportFixture(t, time.Date(2026, 10, 6, 22, 1, 0, 0, time.UTC), quietConf(1, "", ""))
	offsetErr := errors.New("database is locked")
	f.cfg.Offset = func() (time.Duration, error) {
		if offsetErr != nil {
			return 0, offsetErr
		}
		return time.Minute, nil
	}
	job := NewDailyReportJob(f.cfg)

	job.RunOnce(context.Background())
	assert.Empty(t, reportRows(t, f.db), "偏移读取失败：本轮不发")
	offsetErr = nil
	f.clock.Advance(time.Minute)
	job.RunOnce(context.Background())
	assert.Len(t, reportRows(t, f.db), 1)
}

// pruneFailOnce 第一次清理返回错误，之后照常。
type pruneFailOnce struct {
	v2.UserInfoHistoryRepo
	calls int
}

func (p *pruneFailOnce) PruneSnapshots(ctx context.Context, before string) (int64, error) {
	p.calls++
	if p.calls == 1 {
		return 0, errors.New("disk I/O error")
	}
	return p.UserInfoHistoryRepo.PruneSnapshots(ctx, before)
}

// 清理失败时同一天下一轮再试，成功之后当天不再清理。
func TestDailyReport_PruneRetriesAfterFailure(t *testing.T) {
	f := newDailyReportFixture(t, time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC), quietConf(1, "", ""))
	f.enabled = false
	repo := &pruneFailOnce{UserInfoHistoryRepo: f.repo}
	f.cfg.History = repo
	job := NewDailyReportJob(f.cfg)

	job.RunOnce(context.Background())
	job.RunOnce(context.Background())
	job.RunOnce(context.Background())
	assert.Equal(t, 2, repo.calls, "失败后重试一次，成功后当天不再清理")
}

// 战报只算已启用的站点：已禁用站点的增量、残留的登录异常与签到结果都不出现；
// 探测模式为「禁用」的站点，残留的探测状态也不算登录异常。
func TestDailyReport_OnlyEnabledSites(t *testing.T) {
	now := time.Date(2026, 10, 6, 22, 30, 0, 0, time.UTC)
	f := newDailyReportFixture(t, now, quietConf(1, "", ""))
	f.cfg.EnabledSites = func() (map[string]bool, error) {
		return map[string]bool{"hdsky": true, "audiences": true}, nil
	}
	gib := int64(1 << 30)
	yesterday := now.Add(-24 * time.Hour)
	f.saveAt(t, yesterday, "hdsky", 10*gib, 2*gib, 1000)
	f.saveAt(t, now.Add(-time.Hour), "hdsky", 15*gib, 3*gib, 1500)
	f.saveAt(t, yesterday, "pterclub", 8*gib, gib, 500)
	f.saveAt(t, now.Add(-time.Hour), "pterclub", 20*gib, gib, 600)
	require.NoError(t, f.db.Create(&models.SiteLoginState{SiteName: "pterclub", LastProbeStatus: "SESSION_EXPIRED"}).Error)
	require.NoError(t, f.db.Create(&models.SiteLoginState{SiteName: "audiences", LastProbeStatus: "NETWORK_ERROR", ProbeMode: ProbeModeDisabled}).Error)
	require.NoError(t, f.db.Create(&models.SiteAttendanceLog{SiteName: "pterclub", Day: "2026-10-06", Status: models.AttendanceFailed}).Error)
	require.NoError(t, f.db.Create(&models.SiteAttendanceLog{SiteName: "hdsky", Day: "2026-10-06", Status: models.AttendanceSigned}).Error)

	NewDailyReportJob(f.cfg).RunOnce(context.Background())

	rows := reportRows(t, f.db)
	require.Len(t, rows, 1)
	text := reportText(t, rows[0])
	assert.Contains(t, text, "上传 5.00 GiB", "合计只算已启用的 hdsky")
	assert.NotContains(t, text, "PTerClub")
	assert.NotContains(t, text, "pterclub")
	assert.NotContains(t, text, "登录状态异常", "已禁用站点与探测模式为禁用的站点都不算")
	assert.Contains(t, text, "成功 1 · 已签 0 · 失败 0")
}

// 站点读不出来时这一轮不发，免得把已禁用的站点算进去。
func TestDailyReport_EnabledSitesErrorSkipsRound(t *testing.T) {
	f := newDailyReportFixture(t, time.Date(2026, 10, 6, 22, 30, 0, 0, time.UTC), quietConf(1, "", ""))
	f.cfg.EnabledSites = func() (map[string]bool, error) { return nil, errors.New("no such table") }
	NewDailyReportJob(f.cfg).RunOnce(context.Background())
	assert.Empty(t, reportRows(t, f.db))
}

// 只有今天一份快照、算不出增量时，说明还没有可比的数据，不报一排 0。
func TestDailyReport_NoBaselineSaysSo(t *testing.T) {
	now := time.Date(2026, 10, 6, 22, 30, 0, 0, time.UTC)
	f := newDailyReportFixture(t, now, quietConf(1, "", ""))
	f.saveAt(t, now.Add(-time.Hour), "hdsky", 15<<30, 3<<30, 1500)

	NewDailyReportJob(f.cfg).RunOnce(context.Background())

	rows := reportRows(t, f.db)
	require.Len(t, rows, 1)
	text := reportText(t, rows[0])
	assert.NotContains(t, text, "今日合计")
	assert.Contains(t, text, "暂无可比")
}

// 构造时补默认值；Start/Stop 可重复调用，nil 接收者安全。
func TestDailyReport_LifecycleAndDefaults(t *testing.T) {
	j := NewDailyReportJob(DailyReportJobConfig{})
	require.NotNil(t, j.cfg.Clock)
	require.NotNil(t, j.cfg.Logger)
	assert.Equal(t, dailyReportTick, j.cfg.Tick)
	assert.Equal(t, time.Local, j.cfg.Location)
	j.RunOnce(context.Background()) // 没有 History：什么也不做

	j.Start()
	j.Start()
	j.Stop()
	j.Stop()

	var nilJob *DailyReportJob
	nilJob.Start()
	nilJob.Stop()
	nilJob.RunOnce(context.Background())
	assert.Zero(t, DailyReportRand(0))
	assert.Less(t, DailyReportRand(10), int64(10))
}

// 设置读不出、时刻无效、算不出增量、写不进日志：这一轮都不发，也不记成「今天已发」。
func TestDailyReport_ErrorsSkipRound(t *testing.T) {
	now := time.Date(2026, 10, 6, 23, 0, 0, 0, time.UTC)
	f := newDailyReportFixture(t, now, models.NotificationConf{ChannelType: "webhook", Name: "w", Enabled: true})

	f.cfg.Settings = func() (bool, string, []uint, error) { return false, "", nil, errors.New("db locked") }
	NewDailyReportJob(f.cfg).RunOnce(context.Background())
	assert.Empty(t, reportRows(t, f.db))

	f.cfg.Settings = func() (bool, string, []uint, error) { return true, "25:99", []uint{1}, nil }
	NewDailyReportJob(f.cfg).RunOnce(context.Background())
	assert.Empty(t, reportRows(t, f.db))

	f.cfg.Settings = func() (bool, string, []uint, error) { return true, "21:00", []uint{1}, nil }
	f.cfg.History = stubHistory{today: "2026-10-06", listErr: errors.New("disk I/O error")}
	j := NewDailyReportJob(f.cfg)
	j.RunOnce(context.Background())
	assert.Empty(t, reportRows(t, f.db))
	assert.Empty(t, j.doneDay)

	f.cfg.History = f.repo
	require.NoError(t, f.db.Migrator().DropTable(&models.MonitorNotificationLog{}))
	j = NewDailyReportJob(f.cfg)
	j.RunOnce(context.Background())
	assert.Empty(t, j.doneDay, "写日志失败：下一轮重试")
}

// 读不出登录状态或签到结果时这两段省略，战报照发。
func TestDailyReport_MissingSectionsAreOmitted(t *testing.T) {
	now := time.Date(2026, 10, 6, 23, 0, 0, 0, time.UTC)
	f := newDailyReportFixture(t, now, models.NotificationConf{ChannelType: "webhook", Name: "w", Enabled: true})
	require.NoError(t, f.db.Migrator().DropTable(&models.SiteLoginState{}))
	require.NoError(t, f.db.Migrator().DropTable(&models.SiteAttendanceLog{}))
	NewDailyReportJob(f.cfg).RunOnce(context.Background())
	rows := reportRows(t, f.db)
	require.Len(t, rows, 1)
	text := reportText(t, rows[0])
	assert.NotContains(t, text, "登录状态异常")
	assert.NotContains(t, text, "今日签到")
}

type stubHistory struct {
	today   string
	listErr error
}

func (s stubHistory) ListSnapshots(context.Context, string, string, string) ([]v2.UserInfoDailySnapshot, error) {
	return nil, s.listErr
}

func (s stubHistory) SnapshotBaselines(context.Context, string) (map[string]v2.UserInfoDailySnapshot, error) {
	return nil, nil
}
func (s stubHistory) PruneSnapshots(context.Context, string) (int64, error) { return 0, nil }
func (s stubHistory) Today() string                                         { return s.today }

func TestManager_DailyReportJobReplacedAndStopped(t *testing.T) {
	m := &Manager{}
	first := NewDailyReportJob(DailyReportJobConfig{})
	first.Start()
	m.SetDailyReportJob(first)
	second := NewDailyReportJob(DailyReportJobConfig{})
	second.Start()
	m.SetDailyReportJob(second)
	assert.False(t, first.running, "换上新任务时停掉旧的")
	m.SetDailyReportJob(second)
	assert.True(t, second.running, "同一个任务不重复停")
	m.StopAll()
	assert.False(t, second.running)
	assert.Nil(t, m.dailyReportJob)
}

// 当天有刷流收益时战报加一节：合计，多个任务时逐个列出。
func TestDailyReport_IncludesBrush(t *testing.T) {
	now := time.Date(2026, 10, 6, 23, 0, 0, 0, time.UTC)
	f := newDailyReportFixture(t, now, models.NotificationConf{ChannelType: "webhook", Name: "w", Enabled: true})
	require.NoError(t, f.db.AutoMigrate(&models.BrushTask{}, &models.BrushDailyStat{}))
	require.NoError(t, f.db.Create(&models.BrushTask{ID: 1, Name: "馒头刷流", SiteName: "mteam", DownloaderID: 1}).Error)
	require.NoError(t, f.db.Create(&models.BrushTask{ID: 2, Name: "天空刷流", SiteName: "hdsky", DownloaderID: 1}).Error)
	gib := int64(1 << 30)
	require.NoError(t, f.db.Create(&models.BrushDailyStat{TaskID: 1, Day: "2026-10-06", Uploaded: 30 * gib, Downloaded: 5 * gib, Added: 3, Removed: 1}).Error)
	require.NoError(t, f.db.Create(&models.BrushDailyStat{TaskID: 2, Day: "2026-10-06", Uploaded: 10 * gib, Downloaded: 2 * gib, Added: 1}).Error)
	require.NoError(t, f.db.Create(&models.BrushDailyStat{TaskID: 2, Day: "2026-10-05", Uploaded: 99 * gib}).Error)

	NewDailyReportJob(f.cfg).RunOnce(context.Background())
	rows := reportRows(t, f.db)
	require.Len(t, rows, 1)
	text := reportText(t, rows[0])
	assert.Contains(t, text, "🚀 今日刷流：上传 40.00 GiB · 下载 7.00 GiB · 加入 4 · 删除 1")
	assert.Contains(t, text, "· 馒头刷流：上传 30.00 GiB")
	assert.Contains(t, text, "· 天空刷流：上传 10.00 GiB")
	assert.Less(t, strings.Index(text, "馒头刷流"), strings.Index(text, "天空刷流"), "按上传排序")
}
