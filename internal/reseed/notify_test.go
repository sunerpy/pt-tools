package reseed

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

// 推送设置：默认关闭；开着时至少一个通道、通道要存在；没给的项保留原来的；读回来是生效的值。
func TestNotifySettings(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	got, err := e.svc.Settings(ctx)
	require.NoError(t, err)
	assert.Equal(t, models.ReseedNotifyOff, got.NotifyMode)
	assert.Empty(t, got.NotifyChannelIDs)
	assert.NotNil(t, got.NotifyChannelIDs, "空列表，不是 null")
	assert.False(t, got.DailyReport)

	require.NoError(t, e.db.Create(&models.NotificationConf{ID: 1, ChannelType: "telegram", Name: "tg", Enabled: true}).Error)
	require.NoError(t, e.db.Create(&models.NotificationConf{ID: 2, ChannelType: "qq", Name: "qq", Enabled: true}).Error)
	str := func(v string) *string { return &v }
	ids := func(v ...uint) *[]uint { return &v }
	yes := true

	for _, u := range []SettingsUpdate{
		{NotifyMode: str(models.ReseedNotifyAlways)},
		{NotifyMode: str(models.ReseedNotifyChanges), NotifyChannelIDs: ids()},
		{NotifyMode: str(models.ReseedNotifyAlways), NotifyChannelIDs: ids(1, 9)},
		{NotifyMode: str("weekly"), NotifyChannelIDs: ids(1)},
	} {
		_, err = e.svc.SaveSettings(ctx, u)
		require.ErrorIs(t, err, ErrInvalid, "%+v", u)
	}

	got, err = e.svc.SaveSettings(ctx, SettingsUpdate{NotifyMode: str(models.ReseedNotifyChanges), NotifyChannelIDs: ids(2, 1, 2, 0), DailyReport: &yes})
	require.NoError(t, err)
	assert.Equal(t, models.ReseedNotifyChanges, got.NotifyMode)
	assert.Equal(t, []uint{2, 1}, got.NotifyChannelIDs, "去重、去掉 0")
	assert.True(t, got.DailyReport)

	// 旧页面保存时不带这三项：保留原来的
	got, err = e.svc.SaveSettings(ctx, SettingsUpdate{IntervalHours: 24})
	require.NoError(t, err)
	assert.Equal(t, models.ReseedNotifyChanges, got.NotifyMode)
	assert.Equal(t, []uint{2, 1}, got.NotifyChannelIDs)
	assert.True(t, got.DailyReport)

	// 关掉推送时可以不选通道；通道列表留着，下次打开不用重选
	got, err = e.svc.SaveSettings(ctx, SettingsUpdate{NotifyMode: str("")})
	require.NoError(t, err)
	assert.Equal(t, models.ReseedNotifyOff, got.NotifyMode)
	assert.Equal(t, []uint{2, 1}, got.NotifyChannelIDs)
	_, err = e.svc.SaveSettings(ctx, SettingsUpdate{NotifyMode: str(models.ReseedNotifyOff), NotifyChannelIDs: ids()})
	require.NoError(t, err)

	_, err = e.svc.SaveSettings(ctx, SettingsUpdate{NotifyMode: str(models.ReseedNotifyAlways), NotifyChannelIDs: ids(1)})
	require.NoError(t, err)
	mode, channels, err := e.svc.NotifyTarget(ctx)
	require.NoError(t, err)
	assert.Equal(t, models.ReseedNotifyAlways, mode)
	assert.Equal(t, []uint{1}, channels)

	// 不认识的旧值按关闭
	require.NoError(t, e.db.Model(&models.ReseedSetting{}).Where("id = 1").Update("notify_mode", "bogus").Error)
	mode, _, err = e.svc.NotifyTarget(ctx)
	require.NoError(t, err)
	assert.Equal(t, models.ReseedNotifyOff, mode)
}

// 「有变化」：加入了辅种、有失败，或者没跑完。
func TestRoundChanged(t *testing.T) {
	assert.False(t, RoundChanged(RunResult{Hashes: 10, Candidates: 3, Gone: 1, Skipped: map[string]int{SkipTried: 2}}, models.ReseedRunOK))
	assert.True(t, RoundChanged(RunResult{Created: 1}, models.ReseedRunOK))
	assert.True(t, RoundChanged(RunResult{Failed: 1}, models.ReseedRunOK))
	for _, status := range []string{models.ReseedRunPartial, models.ReseedRunLimited, models.ReseedRunStopped, models.ReseedRunFailed} {
		assert.True(t, RoundChanged(RunResult{}, status), status)
	}
}

// 推送的标题、正文与模板变量：正文只有计数与状态；提前停下、被停止的普通轮次写明可以接着查。
func TestNewRoundNotice(t *testing.T) {
	res := RunResult{Hashes: 1200, Candidates: 35, Created: 3, Failed: 1, Gone: 2, IYUUCalls: 3, Skipped: map[string]int{SkipTried: 20, SkipExists: 9}}
	n := NewRoundNotice(Round{ID: 12, Trigger: models.ReseedTriggerSchedule, Status: models.ReseedRunOK, Summary: res.Summary(), Took: 250 * time.Second, Result: res})
	assert.Equal(t, "辅种：第 12 轮完成（定时）", n.Title)
	assert.Equal(t, "查询 1200 个种子，找到 35 个可辅种，加入 3 个，失败 1 个，站点已删除 2 个，跳过 29 个。\n用时 4 分钟，请求 IYUU 3 次。", n.Text)
	assert.Equal(t, uint(12), n.Fields["run_id"])
	assert.Equal(t, 29, n.Fields["skipped"])
	assert.Equal(t, int64(250), n.Fields["duration_seconds"])
	assert.Equal(t, "完成", n.Fields["status_label"])
	assert.Empty(t, n.Fields["stopped"], "跑完了没有停下的原因")

	limited := RunResult{Hashes: 400, End: EndLimited, Stopped: "IYUU 限流：请求太频繁"}
	n = NewRoundNotice(Round{ID: 13, Trigger: models.ReseedTriggerManual, Status: models.ReseedRunLimited, Reason: limited.Stopped, Summary: limited.Summary(), Took: 8 * time.Second, Result: limited})
	assert.Equal(t, "辅种：第 13 轮提前停下（立即运行）", n.Title)
	assert.Contains(t, n.Text, "IYUU 限流：请求太频繁。\n用时 8 秒。")
	assert.Contains(t, n.Text, "「IYUU 辅种 → 轮次」里点「继续」")
	assert.Equal(t, "IYUU 限流：请求太频繁", n.Fields["stopped"])

	retry := RunResult{Hashes: 2, End: EndStopped, Stopped: "被停止", Retry: true}
	n = NewRoundNotice(Round{ID: 14, Trigger: models.ReseedTriggerRetry, Status: models.ReseedRunStopped, Reason: "被停止", Summary: retry.Summary(), Result: retry})
	assert.Equal(t, "辅种：第 14 轮已停止（重试）", n.Title)
	assert.NotContains(t, n.Text, "继续", "重试轮次不能继续")

	n = NewRoundNotice(Round{ID: 15, Trigger: models.ReseedTriggerContinue, Status: models.ReseedRunFailed, Reason: "读取站点列表失败。", Summary: "运行失败：读取站点列表失败。"})
	assert.Equal(t, "辅种：第 15 轮未能开始（继续）", n.Title)
	assert.Equal(t, "运行失败：读取站点列表失败。\n用时 0 秒。", n.Text, "不重复句号")
	assert.Equal(t, "读取站点列表失败。", n.Fields["stopped"])
}

// 每日战报里今天的辅种：没开或者今天没有数据时没有这一行；数字与统计同一套算法，不写跳过。
func TestDailyReportLine(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.now = dayAt(t, "2026-10-30", 21)
	today := "2026-10-30"
	line, err := e.svc.DailyReportLine(ctx, today)
	require.NoError(t, err)
	assert.Empty(t, line, "没开")

	yes := true
	_, err = e.svc.SaveSettings(ctx, SettingsUpdate{DailyReport: &yes})
	require.NoError(t, err)
	require.NoError(t, e.db.Create(&models.ReseedDailySkip{Day: today, SiteName: "hdsky", Present: 40}).Error)
	line, err = e.svc.DailyReportLine(ctx, today)
	require.NoError(t, err)
	assert.Empty(t, line, "只有跳过时不写")

	done := dayAt(t, today, 9)
	job := models.TorrentTransferJob{Kind: models.JobKindReseed, TargetDownloaderID: 1, InfoHash: "x", State: models.TransferDone, FinishedAt: &done}
	require.NoError(t, e.db.Create(&job).Error)
	rolled := dayAt(t, today, 10)
	for _, a := range []models.ReseedAttempt{
		{StartedAt: dayAt(t, today, 8), SiteName: "hdsky", Outcome: models.ReseedAttemptCreated, JobID: &job.ID},
		{StartedAt: dayAt(t, today, 8), SiteName: "hdsky", Outcome: models.ReseedAttemptCreated, JobID: func() *uint { v := uint(77); return &v }(), JobState: models.TransferRolledBack, JobFinishedAt: &rolled},
		{StartedAt: dayAt(t, today, 8), SiteName: "hdsky", Outcome: models.ReseedAttemptFailed},
		{StartedAt: dayAt(t, today, 8), SiteName: "ourbits", Outcome: models.ReseedAttemptGone},
		{StartedAt: dayAt(t, "2026-10-29", 8), SiteName: "hdsky", Outcome: models.ReseedAttemptFailed},
	} {
		a.InfoHash = "h"
		require.NoError(t, e.db.Create(&a).Error)
	}
	line, err = e.svc.DailyReportLine(ctx, today)
	require.NoError(t, err)
	assert.Equal(t, "🌱 今日辅种：建成 2 · 成功 1 · 失败 2 · 站点已删除 1", line, "失败含撤回；昨天的不算")
}
