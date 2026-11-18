package cmd

import (
	"context"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/backup"
	"github.com/sunerpy/pt-tools/scheduler"
)

// wireBackupJob 构造并启动定时备份（默认每天 04:30）：失败通知与登录提醒共用通知投递器（notifier 为 nil 时只记日志）。
func wireBackupJob(mgr *scheduler.Manager, store *core.ConfigStore, notifier *scheduler.MonitorNotifier) *scheduler.BackupJob {
	if mgr == nil || store == nil || global.GlobalDB == nil || global.GlobalDB.DB == nil {
		global.GetSlogger().Warn("定时备份跳过初始化：配置或数据库未就绪")
		return nil
	}
	job := scheduler.NewBackupJob(scheduler.BackupJobConfig{
		Settings: backupSchedule(store),
		Run: func(ctx context.Context) error {
			_, err := store.RunBackup(ctx, backup.ReasonScheduled)
			return err
		},
		Notifier: notifier,
		Logger:   global.GetSlogger(),
	})
	mgr.SetBackupJob(job)
	job.Start()
	global.GetSlogger().Info("定时备份任务已启动")
	return job
}

// backupSchedule 把备份设置换成定时任务要的那几项。
func backupSchedule(store *core.ConfigStore) func() (scheduler.BackupSchedule, error) {
	return func() (scheduler.BackupSchedule, error) {
		s, err := store.BackupSettings()
		if err != nil {
			return scheduler.BackupSchedule{}, err
		}
		out := scheduler.BackupSchedule{Enabled: s.Enabled, Time: s.Time, Day: s.ScheduledDay, Dir: s.BackupDir()}
		if s.Last != nil {
			out.LastOK = s.Last.LastOKAt
		}
		return out, nil
	}
}
