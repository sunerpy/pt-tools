package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/sitelogin"
)

const (
	backupJobTick         = time.Minute
	backupJobStartupDelay = time.Minute
	backupJobTimeout      = 30 * time.Minute
	backupNotifySource    = "backup"
	backupNotifyKind      = "failed"
)

// BackupSchedule 是定时备份的设置与状态（core.ConfigStore.BackupSettings 的那几项）。
type BackupSchedule struct {
	Enabled bool
	// Time 是每天备份的时刻（HH:MM，Location 时区）
	Time string
	// Day 是最近一次做过定时备份（不论成败）的日期：同一天不再做，重启也不再做
	Day string
	// Dir 是现在生效的备份目录（失败通知里写给人看）
	Dir string
	// LastOK 是最近一次成功备份的时间，没有时为 nil
	LastOK *time.Time
}

// BackupJobConfig 是 BackupJob 的依赖。
type BackupJobConfig struct {
	Settings func() (BackupSchedule, error)
	// Run 做一份定时备份并记下结果与做过的日期（core.ConfigStore.RunBackup，原因 scheduled）。
	Run func(ctx context.Context) error
	// Notifier 为空或不可用时失败只记日志。
	Notifier *MonitorNotifier
	Clock    sitelogin.Clock
	Logger   *zap.SugaredLogger
	Tick     time.Duration
	// Location 是计算日期与备份时刻的时区，默认进程时区。
	Location *time.Location
	// Timeout 是一次备份的时限，默认 30 分钟。
	Timeout time.Duration
}

// BackupJob 每分钟检查一次：到了设定时刻、当天还没做过定时备份就做一份（启动时错过了当天的时刻也补做）。
// 失败时经 MonitorNotifier 发 system.backup_failed，同一天只发一次。
type BackupJob struct {
	cfg BackupJobConfig

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	doneDay string // 当天已经做过，不用每分钟读设置
}

// NewBackupJob 构造定时备份任务，不启动；调用 Start 开始调度。
func NewBackupJob(cfg BackupJobConfig) *BackupJob {
	if cfg.Clock == nil {
		cfg.Clock = sitelogin.NewRealClock()
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop().Sugar()
	}
	if cfg.Tick <= 0 {
		cfg.Tick = backupJobTick
	}
	if cfg.Location == nil {
		cfg.Location = time.Local
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = backupJobTimeout
	}
	return &BackupJob{cfg: cfg}
}

// Start 启动调度循环；重复调用无效果。
func (j *BackupJob) Start() {
	if j == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.running {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	j.cancel = cancel
	j.running = true
	j.wg.Go(func() { j.loop(ctx) })
}

// Stop 停止调度循环并等它退出（正在做的备份随 ctx 取消）。
func (j *BackupJob) Stop() {
	if j == nil {
		return
	}
	j.mu.Lock()
	if !j.running {
		j.mu.Unlock()
		return
	}
	j.running = false
	cancel := j.cancel
	j.mu.Unlock()
	cancel()
	j.wg.Wait()
}

func (j *BackupJob) loop(ctx context.Context) {
	// 启动后先等一会儿：刚启动时下载器重连、站点同步都在跑，备份晚一分钟没关系
	select {
	case <-ctx.Done():
		return
	case <-time.After(backupJobStartupDelay):
		j.RunOnce(ctx)
	}
	ticker := time.NewTicker(j.cfg.Tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			j.RunOnce(ctx)
		}
	}
}

// RunOnce 处理一轮：开着、到了时刻、当天还没做过时做一份定时备份；失败时发通知。
func (j *BackupJob) RunOnce(ctx context.Context) {
	if j == nil || j.cfg.Settings == nil || j.cfg.Run == nil {
		return
	}
	now := j.cfg.Clock.Now().In(j.cfg.Location)
	today := now.Format("2006-01-02")
	j.mu.Lock()
	done := j.doneDay == today
	j.mu.Unlock()
	if done {
		return
	}
	sched, err := j.cfg.Settings()
	if err != nil {
		j.cfg.Logger.Warnf("读取备份设置失败: %v", err)
		return
	}
	if !sched.Enabled {
		return
	}
	if sched.Day == today {
		j.markDone(today)
		return
	}
	due, err := backupDue(now, sched.Time)
	if err != nil {
		j.cfg.Logger.Warnf("定时备份的时刻 %q 无效: %v", sched.Time, err)
		return
	}
	if now.Before(due) {
		return
	}

	runCtx, cancel := context.WithTimeout(ctx, j.cfg.Timeout)
	runErr := j.cfg.Run(runCtx)
	cancel()
	if ctx.Err() != nil {
		// 退出时被打断：不算这天做过，也不发失败通知；下次启动补做
		return
	}
	// 失败也算这天做过：失败原因已经记下、通知发一次，第二天再做；想马上重试可以点「立即备份」
	j.markDone(today)
	if runErr == nil {
		j.cfg.Logger.Info("定时备份完成")
		return
	}
	j.cfg.Logger.Errorf("定时备份失败: %v", runErr)
	j.notifyFailure(ctx, now, today, sched, runErr)
}

// backupDue 是 now 那天的备份时刻（now 所在时区的墙上时间，夏令时切换日也不偏）。
func backupDue(now time.Time, hhmm string) (time.Time, error) {
	at, err := time.Parse("15:04", hhmm)
	if err != nil {
		return time.Time{}, err
	}
	y, m, d := now.Date()
	return time.Date(y, m, d, at.Hour(), at.Minute(), 0, 0, now.Location()), nil
}

// NextBackupAt 是下一次定时备份大约什么时候，判断与 BackupJob.RunOnce 相同（now 用任务的时区）：
// 关着或时刻无效时是零值；当天还没做、时刻已过时是 now（下一轮检查就做，最多一分钟）。
func NextBackupAt(s BackupSchedule, now time.Time) time.Time {
	if !s.Enabled {
		return time.Time{}
	}
	due, err := backupDue(now, s.Time)
	if err != nil {
		return time.Time{}
	}
	switch {
	case s.Day == now.Format("2006-01-02"):
		return due.AddDate(0, 0, 1)
	case now.Before(due):
		return due
	default:
		return now
	}
}

func (j *BackupJob) markDone(day string) {
	j.mu.Lock()
	j.doneDay = day
	j.mu.Unlock()
}

func (j *BackupJob) notifyFailure(ctx context.Context, now time.Time, today string, sched BackupSchedule, runErr error) {
	if !j.cfg.Notifier.Enabled() {
		return
	}
	lastOK := ""
	if sched.LastOK != nil {
		lastOK = sched.LastOK.In(j.cfg.Location).Format("2006-01-02 15:04")
	}
	text := fmt.Sprintf("今天的定时备份没做成：%v。\n备份目录：%s", runErr, sched.Dir)
	if lastOK != "" {
		text += "\n上一次成功的备份：" + lastOK
	} else {
		text += "\n还没有成功做过备份。"
	}
	text += "\n请到「全局设置 → 备份」检查备份目录，或者点「立即备份」再试一次。"
	if _, err := j.cfg.Notifier.Enqueue(ctx, MonitorNotifyEntry{
		Source:   backupNotifySource,
		Subject:  "*",
		Kind:     backupNotifyKind,
		EventKey: today,
		Title:    "备份失败",
		Text:     text,
		Event: notify.NewEvent(notify.EventBackupFailed, now, map[string]any{
			"dir": sched.Dir, "error": runErr.Error(), "last_ok": lastOK,
		}),
	}); err != nil {
		j.cfg.Logger.Warnf("写入备份失败通知失败: %v", err)
	}
}
