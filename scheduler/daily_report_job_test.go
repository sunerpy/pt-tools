package scheduler

import (
	"context"
	"encoding/json"
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
