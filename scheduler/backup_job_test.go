package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
)

type backupJobFixture struct {
	db    *gorm.DB
	clock *sitelogin.FakeClock
	cfg   BackupJobConfig
	// sched 是 Settings 返回的设置；Run 成功或失败以后按 core.ConfigStore.RecordBackupResult 的做法记下日期
	sched  BackupSchedule
	runErr error
	runs   atomic.Int32
}

func newBackupJobFixture(t *testing.T, now time.Time) *backupJobFixture {
	t.Helper()
	db := newReminderTestDB(t)
	conf := quietConf(1, "", "")
	require.NoError(t, db.Create(&conf).Error)
	clock := sitelogin.NewFakeClock(now)
	f := &backupJobFixture{db: db, clock: clock, sched: BackupSchedule{Enabled: true, Time: "04:30", Dir: "/data/backups"}}
	f.cfg = BackupJobConfig{
		Settings: func() (BackupSchedule, error) { return f.sched, nil },
		Run: func(context.Context) error {
			f.runs.Add(1)
			f.sched.Day = clock.Now().In(time.UTC).Format("2006-01-02")
			if f.runErr == nil {
				ok := clock.Now()
				f.sched.LastOK = &ok
			}
			return f.runErr
		},
		Notifier: NewMonitorNotifier(db, &captureSender{}, clock, zap.NewNop().Sugar()),
		Clock:    clock,
		Location: time.UTC,
		Logger:   zap.NewNop().Sugar(),
	}
	return f
}

func backupRows(t *testing.T, db *gorm.DB) []models.MonitorNotificationLog {
	t.Helper()
	var rows []models.MonitorNotificationLog
	require.NoError(t, db.Where("source = ?", backupNotifySource).Order("id").Find(&rows).Error)
	return rows
}

// 到点以前不做；到点以后做一次，同一天不再做（重启以后按记下的日期也不做）；第二天再做。
func TestBackupJob_RunsOncePerDayAfterTime(t *testing.T) {
	f := newBackupJobFixture(t, time.Date(2026, 11, 2, 4, 29, 0, 0, time.UTC))
	job := NewBackupJob(f.cfg)
	job.RunOnce(context.Background())
	assert.Zero(t, f.runs.Load(), "04:30 以前不做")

	f.clock.Advance(time.Minute)
	job.RunOnce(context.Background())
	assert.EqualValues(t, 1, f.runs.Load())
	job.RunOnce(context.Background())
	NewBackupJob(f.cfg).RunOnce(context.Background())
	assert.EqualValues(t, 1, f.runs.Load(), "同一天不再做，重启也不再做")

	f.clock.Advance(24 * time.Hour)
	job.RunOnce(context.Background())
	assert.EqualValues(t, 2, f.runs.Load(), "第二天再做")
	assert.Empty(t, backupRows(t, f.db), "成功不发通知")
}

// 启动时已经过了当天的时刻、当天还没做过：补做一份。
func TestBackupJob_CatchUpAfterStart(t *testing.T) {
	f := newBackupJobFixture(t, time.Date(2026, 11, 2, 15, 0, 0, 0, time.UTC))
	f.sched.Day = "2026-11-01"
	NewBackupJob(f.cfg).RunOnce(context.Background())
	assert.EqualValues(t, 1, f.runs.Load())
}

// 关着、设置读不出、时刻无效：都不做。
func TestBackupJob_SkipsWhenDisabledOrBadSettings(t *testing.T) {
	f := newBackupJobFixture(t, time.Date(2026, 11, 2, 23, 0, 0, 0, time.UTC))
	f.sched.Enabled = false
	NewBackupJob(f.cfg).RunOnce(context.Background())
	f.sched.Enabled, f.sched.Time = true, "25:99"
	NewBackupJob(f.cfg).RunOnce(context.Background())
	cfg := f.cfg
	cfg.Settings = func() (BackupSchedule, error) { return BackupSchedule{}, errors.New("database is locked") }
	j := NewBackupJob(cfg)
	j.RunOnce(context.Background())
	assert.Zero(t, f.runs.Load())
	assert.Empty(t, j.doneDay, "读不出设置时下一轮再看")
}

// 失败：发一次 system.backup_failed（带目录、原因与上次成功的时间），同一天不再做也不再发；第二天再做。
func TestBackupJob_FailureNotifiesOnce(t *testing.T) {
	f := newBackupJobFixture(t, time.Date(2026, 11, 2, 4, 30, 0, 0, time.UTC))
	ok := time.Date(2026, 10, 31, 4, 30, 0, 0, time.UTC)
	f.sched.LastOK = &ok
	f.runErr = errors.New("创建备份目录失败: permission denied")
	job := NewBackupJob(f.cfg)
	job.RunOnce(context.Background())
	job.RunOnce(context.Background())
	NewBackupJob(f.cfg).RunOnce(context.Background())
	assert.EqualValues(t, 1, f.runs.Load())

	rows := backupRows(t, f.db)
	require.Len(t, rows, 1)
	assert.Equal(t, backupNotifyKind, rows[0].Kind)
	assert.Equal(t, "2026-11-02", rows[0].EventKey)
	var p monitorNotifyPayload
	require.NoError(t, json.Unmarshal([]byte(rows[0].PayloadJSON), &p))
	assert.Equal(t, "备份失败", p.Title)
	assert.Contains(t, p.Text, "permission denied")
	assert.Contains(t, p.Text, "/data/backups")
	assert.Contains(t, p.Text, "2026-10-31 04:30")
	require.NotNil(t, p.Event)
	assert.Equal(t, notify.EventBackupFailed, p.Event.Type)
	assert.Equal(t, "/data/backups", p.Event.Fields["dir"])
	assert.Equal(t, "创建备份目录失败: permission denied", p.Event.Fields["error"])
	assert.Equal(t, "2026-10-31 04:30", p.Event.Fields["last_ok"])

	f.clock.Advance(24 * time.Hour)
	f.sched.LastOK = nil
	job.RunOnce(context.Background())
	rows = backupRows(t, f.db)
	require.Len(t, rows, 2, "第二天失败再发一次")
	require.NoError(t, json.Unmarshal([]byte(rows[1].PayloadJSON), &p))
	assert.Contains(t, p.Text, "还没有成功做过备份")
	assert.Equal(t, "", p.Event.Fields["last_ok"])
}

// 退出时被打断：不算这天做过，也不发通知。
func TestBackupJob_CancelledRunIsNotCounted(t *testing.T) {
	f := newBackupJobFixture(t, time.Date(2026, 11, 2, 5, 0, 0, 0, time.UTC))
	ctx, cancel := context.WithCancel(context.Background())
	cfg := f.cfg
	cfg.Run = func(context.Context) error {
		cancel()
		return context.Canceled
	}
	j := NewBackupJob(cfg)
	j.RunOnce(ctx)
	assert.Empty(t, j.doneDay)
	assert.Empty(t, backupRows(t, f.db))
}

// 一次备份有时限：超时算失败。
func TestBackupJob_Timeout(t *testing.T) {
	f := newBackupJobFixture(t, time.Date(2026, 11, 2, 5, 0, 0, 0, time.UTC))
	cfg := f.cfg
	cfg.Timeout = time.Millisecond
	cfg.Run = func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	j := NewBackupJob(cfg)
	j.RunOnce(context.Background())
	assert.Equal(t, "2026-11-02", j.doneDay)
	require.Len(t, backupRows(t, f.db), 1)
}

func TestNextBackupAt(t *testing.T) {
	now := time.Date(2026, 11, 2, 3, 0, 0, 0, time.UTC)
	s := BackupSchedule{Enabled: true, Time: "04:30"}
	assert.Equal(t, time.Date(2026, 11, 2, 4, 30, 0, 0, time.UTC), NextBackupAt(s, now), "今天还没到点")
	assert.Equal(t, now.Add(2*time.Hour), NextBackupAt(s, now.Add(2*time.Hour)), "过了点还没做：马上")
	s.Day = "2026-11-02"
	assert.Equal(t, time.Date(2026, 11, 3, 4, 30, 0, 0, time.UTC), NextBackupAt(s, now), "今天做过了：明天")
	s.Enabled = false
	assert.True(t, NextBackupAt(s, now).IsZero())
	s.Enabled, s.Time = true, "bad"
	assert.True(t, NextBackupAt(s, now).IsZero())
}

// 构造时补默认值；Start/Stop 可重复调用，nil 接收者安全；Manager 换任务时停旧的，StopAll 停掉。
func TestBackupJob_LifecycleAndManager(t *testing.T) {
	j := NewBackupJob(BackupJobConfig{})
	require.NotNil(t, j.cfg.Clock)
	assert.Equal(t, backupJobTick, j.cfg.Tick)
	assert.Equal(t, backupJobTimeout, j.cfg.Timeout)
	assert.Equal(t, time.Local, j.cfg.Location)
	j.RunOnce(context.Background()) // 没有 Settings、Run：什么也不做
	j.Start()
	j.Start()
	j.Stop()
	j.Stop()
	var nilJob *BackupJob
	nilJob.Start()
	nilJob.Stop()
	nilJob.RunOnce(context.Background())

	m := &Manager{}
	first := NewBackupJob(BackupJobConfig{})
	first.Start()
	m.SetBackupJob(first)
	second := NewBackupJob(BackupJobConfig{})
	second.Start()
	m.SetBackupJob(second)
	assert.False(t, first.running, "换上新任务时停掉旧的")
	m.SetBackupJob(second)
	assert.True(t, second.running)
	m.StopAll()
	assert.False(t, second.running)
	assert.Nil(t, m.backupJob)
}
