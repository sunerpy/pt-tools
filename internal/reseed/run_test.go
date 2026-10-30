package reseed

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func (e *env) attempts() []models.ReseedAttempt {
	e.t.Helper()
	var rows []models.ReseedAttempt
	require.NoError(e.t, e.db.Order("id").Find(&rows).Error)
	return rows
}

// 建成任务时，尝试（created + 任务编号）、辅种记录与任务在同一个事务里提交；回调写不进去时任务也不存在。
func TestAttemptCreatedIsAtomicWithJob(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.enable(nil)
	good := e.candidate("71", map[string]int64{"a.mkv": 100, "sub/b.srt": 1})
	e.setResults(map[string]any{"sid": 1, "torrent_id": 71, "info_hash": good})

	// 写辅种记录时出错（模拟磁盘问题）：任务一起回滚，这次记失败、可以重试
	boom := e.db.Callback().Create().Before("gorm:create").Register("test:fail-record", func(tx *gorm.DB) {
		if tx.Statement.Table == "reseed_records" && tx.Statement.Dest != nil {
			if rec, ok := tx.Statement.Dest.(*models.ReseedRecord); ok && rec.State == models.ReseedQueued {
				_ = tx.AddError(errors.New("磁盘满了"))
			}
		}
	})
	require.NoError(t, boom)
	res, err := e.svc.Run(ctx, RunOptions{})
	require.NoError(t, err)
	assert.Zero(t, res.Created)
	assert.Equal(t, 1, res.Failed)
	var jobs int64
	require.NoError(t, e.db.Model(&models.TorrentTransferJob{}).Count(&jobs).Error)
	assert.Zero(t, jobs, "回调出错时任务也不存在")
	atts := e.attempts()
	require.Len(t, atts, 1)
	assert.Equal(t, models.ReseedAttemptFailed, atts[0].Outcome)
	assert.Nil(t, atts[0].JobID)
	var rec models.ReseedRecord
	require.NoError(t, e.db.First(&rec).Error)
	assert.True(t, rec.Retryable)
	assert.Contains(t, rec.Message, "建辅种任务失败")
	require.NoError(t, e.db.Callback().Create().Remove("test:fail-record"))

	// 正常建成：三样一起写上
	require.NoError(t, e.db.Where("1 = 1").Delete(&models.ReseedRecord{}).Error)
	res, err = e.svc.Run(ctx, RunOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
	var job models.TorrentTransferJob
	require.NoError(t, e.db.First(&job).Error)
	atts = e.attempts()
	require.Len(t, atts, 2)
	assert.Equal(t, models.ReseedAttemptCreated, atts[1].Outcome)
	require.NotNil(t, atts[1].JobID)
	assert.Equal(t, job.ID, *atts[1].JobID)
	var queued models.ReseedRecord
	require.NoError(t, e.db.First(&queued).Error)
	assert.Equal(t, models.ReseedQueued, queued.State)
	assert.Equal(t, job.ID, *queued.JobID)
}

// 种子下载、核对过以后才发现已有进行中的任务：尝试记为 active，占名额，算跳过、不算失败。
func TestAttemptActiveJob(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.enable(func(u *SettingsUpdate) { u.MaxPerSitePerDay = 1 })
	good := e.candidate("72", map[string]int64{"a.mkv": 100, "sub/b.srt": 1})
	other := e.candidate("73", map[string]int64{"a.mkv": 100, "sub/b.srt": 1})
	e.setResults(map[string]any{"sid": 1, "torrent_id": 72, "info_hash": good}, map[string]any{"sid": 1, "torrent_id": 73, "info_hash": other})
	require.NoError(t, e.db.Create(&models.TorrentTransferJob{Kind: models.JobKindReseed, InfoHash: good, State: models.TransferChecking, TargetDownloaderID: 1}).Error)

	res, err := e.svc.Run(ctx, RunOptions{})
	require.NoError(t, err)
	assert.Zero(t, res.Failed)
	assert.Equal(t, 1, res.Skipped[SkipActive])
	assert.Equal(t, 1, res.Skipped[SkipDailyLimit], "active 也占了今天的名额")
	atts := e.attempts()
	require.Len(t, atts, 1)
	assert.Equal(t, models.ReseedAttemptActive, atts[0].Outcome)
	var n int64
	require.NoError(t, e.db.Model(&models.ReseedRecord{}).Count(&n).Error)
	assert.Zero(t, n, "已有进行中的任务时不记失败")
}

// 每天的名额按尝试行算：删除记录（下一轮会再试）不退名额，被打断的尝试也占名额。
func TestDailyLimitCountsAttempts(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.enable(func(u *SettingsUpdate) { u.MaxPerSitePerDay = 2 })
	bad := e.candidate("74", map[string]int64{"a.mkv": 100})
	e.setResults(map[string]any{"sid": 1, "torrent_id": 74, "info_hash": bad})
	res, err := e.svc.Run(ctx, RunOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, res.Failed)

	require.NoError(t, e.db.Where("1 = 1").Delete(&models.ReseedRecord{}).Error)
	res, err = e.svc.Run(ctx, RunOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, res.Failed, "删除记录以后再试一次")

	require.NoError(t, e.db.Where("1 = 1").Delete(&models.ReseedRecord{}).Error)
	res, err = e.svc.Run(ctx, RunOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Skipped[SkipDailyLimit], "今天已经试了两次：删除记录不退名额")
	assert.Len(t, e.attempts(), 2)

	e.now = e.now.Add(24 * time.Hour)
	res, err = e.svc.Run(ctx, RunOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Failed, "第二天重新计数")
}

// 停止打断了正在下载的种子：尝试记为 interrupted，不改记录（不记失败），以后照常再试；名额照占。
func TestAttemptInterruptedByStop(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	good := e.candidate("75", map[string]int64{"a.mkv": 100, "sub/b.srt": 1})
	e.setResults(map[string]any{"sid": 1, "torrent_id": 75, "info_hash": good})
	ctx, cancel := context.WithCancelCause(context.Background())
	e.site.onDownload = func(context.Context) error {
		cancel(ErrRunStopped)
		return context.Canceled
	}
	res, err := e.svc.Run(ctx, RunOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Interrupted)
	assert.Zero(t, res.Failed)
	assert.Equal(t, EndStopped, res.End)
	assert.Equal(t, ErrRunStopped.Error(), res.Stopped)
	atts := e.attempts()
	require.Len(t, atts, 1)
	assert.Equal(t, models.ReseedAttemptInterrupted, atts[0].Outcome)
	var n int64
	require.NoError(t, e.db.Model(&models.ReseedRecord{}).Count(&n).Error)
	assert.Zero(t, n, "被打断的不记进辅种记录")
	today, err := e.svc.todayCounts(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, today["hdsky"], "被打断的也占名额")

	e.site.onDownload = nil
	res, err = e.svc.Run(context.Background(), RunOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created, "以后照常再试")
}

// addSources 往下载器里加 n 个已经下完的种子（hash 按顺序排），返回排好序的 hash。
func (e *env) addSources(n int) []string {
	out := make([]string, 0, n)
	for i := range n {
		h := fmt.Sprintf("%040x", i+1)
		e.dl.torrents = append(e.dl.torrents, downloader.Torrent{ID: h, InfoHash: h, Name: "T", Progress: 1, IsCompleted: true, State: downloader.TorrentSeeding})
		out = append(out, h)
	}
	return out
}

// 一轮分批查询：停在第 2 批中间时，游标是第 1 批的最后一个 hash；接着查只查后面的种子（第 2 批整批再查一次）；
// 第 1 批还没处理完就停了时没有游标，继续时从原来的起点开始。
func TestRunCursorAndAfter(t *testing.T) {
	e := newEnv(t)
	e.enable(nil)
	hashes := e.addSources(450) // 加上原来的 1 个，451 个：3 批
	all := append([]string{e.srcHash}, hashes...)
	sort.Strings(all)
	// 第 2 批里放一个候选：下载时停止这一轮
	target := all[300]
	cand := e.candidate("81", map[string]int64{"a.mkv": 100})
	e.iyuu.results[target] = map[string]any{"torrent": []map[string]any{{"sid": 1, "torrent_id": 81, "info_hash": cand}}}
	e.dl.files[target] = []downloader.TorrentFile{{Name: "x/a.mkv", Size: 100}}

	ctx, cancel := context.WithCancelCause(context.Background())
	e.site.onDownload = func(context.Context) error {
		cancel(ErrRunStopped)
		return context.Canceled
	}
	res, err := e.svc.Run(ctx, RunOptions{})
	require.NoError(t, err)
	assert.Equal(t, 451, res.Hashes)
	assert.Equal(t, 3, res.Batches)
	assert.Equal(t, 1, res.BatchesDone)
	assert.Equal(t, all[199], res.Cursor, "游标是第 1 批的最后一个")
	assert.Equal(t, EndStopped, res.End)

	e.site.onDownload = nil
	queries := e.iyuu.queries
	res, err = e.svc.Run(context.Background(), RunOptions{After: res.Cursor})
	require.NoError(t, err)
	assert.Equal(t, 251, res.Hashes, "只查第 1 批之后的")
	assert.Equal(t, 2, e.iyuu.queries-queries)
	assert.Equal(t, 0, res.Created, "文件不一致")
	assert.Equal(t, 1, res.Failed, "被打断的那一个再试了一次")

	// 第 1 批还没处理完就停了：没有游标
	e2 := newEnv(t)
	e2.enable(nil)
	c2 := e2.candidate("82", map[string]int64{"a.mkv": 100, "sub/b.srt": 1})
	e2.setResults(map[string]any{"sid": 1, "torrent_id": 82, "info_hash": c2})
	ctx2, cancel2 := context.WithCancelCause(context.Background())
	e2.site.onDownload = func(context.Context) error {
		cancel2(ErrRunStopped)
		return context.Canceled
	}
	res, err = e2.svc.Run(ctx2, RunOptions{})
	require.NoError(t, err)
	assert.Zero(t, res.BatchesDone)
	assert.Empty(t, res.Cursor)
}

// 轮次记录：写开始、补结果；继续只对最近一个提前停下的、不是重试的轮次；启动时补记被打断的；旧的会被清理。
func TestRunsRecords(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	run, err := e.svc.StartRun(ctx, models.ReseedTriggerManual, "", nil)
	require.NoError(t, err)
	res := RunResult{Hashes: 10, Created: 2, Failed: 1, Skipped: map[string]int{SkipExists: 4}, Cursor: "c1", End: EndStopped, Stopped: "这一轮被停止", log: &runLog{}}
	res.log.add(stepAdd, "加入 %s", "Movie")
	require.NoError(t, e.svc.FinishRun(ctx, run.ID, res, models.ReseedRunStopped, "这一轮被停止", 90*time.Second))
	d, err := e.svc.GetRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ReseedRunStopped, d.Status)
	assert.Equal(t, 4, d.SkippedDetail[SkipExists])
	assert.Equal(t, 4, d.Skipped)
	assert.Equal(t, 90, d.PausedSeconds)
	assert.True(t, d.Continuable)
	require.Len(t, d.Steps, 1)
	assert.Contains(t, d.Steps[0].Text, "加入 Movie")
	after, err := e.svc.CanContinue(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, "c1", after)

	// 重试轮次不影响继续
	retry, err := e.svc.StartRun(ctx, models.ReseedTriggerRetry, "", nil)
	require.NoError(t, err)
	require.NoError(t, e.svc.FinishRun(ctx, retry.ID, RunResult{Retry: true}, models.ReseedRunStopped, "", 0))
	after, from, err := e.svc.ContinuePoint(ctx)
	require.NoError(t, err)
	assert.Equal(t, "c1", after)
	assert.Equal(t, run.ID, *from)
	items, total, err := e.svc.Runs(ctx, 1, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	assert.Equal(t, retry.ID, items[0].ID, "新的在前")
	assert.False(t, items[0].Continuable, "重试轮次不能继续")
	assert.True(t, items[1].Continuable)

	// 跑完的轮次之后：不再接着查
	okRun, err := e.svc.StartRun(ctx, models.ReseedTriggerContinue, "c1", &run.ID)
	require.NoError(t, err)
	require.NoError(t, e.svc.FinishRun(ctx, okRun.ID, RunResult{}, models.ReseedRunOK, "", 0))
	after, from, err = e.svc.ContinuePoint(ctx)
	require.NoError(t, err)
	assert.Empty(t, after)
	assert.Nil(t, from)
	_, err = e.svc.CanContinue(ctx, run.ID)
	assert.ErrorIs(t, err, ErrNotContinuable)
	_, err = e.svc.CanContinue(ctx, 999)
	assert.ErrorIs(t, err, ErrRunNotFound)
	_, err = e.svc.GetRun(ctx, 999)
	assert.ErrorIs(t, err, ErrRunNotFound)

	// 尝试列表带上任务现在的状态
	job := models.TorrentTransferJob{Kind: models.JobKindReseed, InfoHash: "h", State: models.TransferChecking, TargetDownloaderID: 1}
	require.NoError(t, e.db.Create(&job).Error)
	gone := uint(12345)
	require.NoError(t, e.db.Create(&[]models.ReseedAttempt{
		{RunID: &okRun.ID, StartedAt: e.now, SiteName: "hdsky", InfoHash: "h", Outcome: models.ReseedAttemptCreated, JobID: &job.ID},
		{RunID: &okRun.ID, StartedAt: e.now, SiteName: "hdsky", InfoHash: "g", Outcome: models.ReseedAttemptCreated, JobID: &gone},
	}).Error)
	atts, total, err := e.svc.RunAttempts(ctx, okRun.ID, 1, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	assert.Equal(t, models.TransferChecking, atts[0].JobState)
	assert.True(t, atts[1].JobMissing)
	_, _, err = e.svc.RunAttempts(ctx, 999, 1, 10)
	assert.ErrorIs(t, err, ErrRunNotFound)
	waiting, err := e.svc.RunJobsWaiting(ctx, okRun.ID)
	require.NoError(t, err)
	assert.Empty(t, waiting, "校验中的任务不能取消")

	// 启动时补记：早于进程启动、还写着进行中或暂停的轮次与 running 的尝试
	stale, err := e.svc.StartRun(ctx, models.ReseedTriggerSchedule, "", nil)
	require.NoError(t, err)
	require.NoError(t, e.svc.SetRunPaused(ctx, stale.ID, true, e.now, 0))
	require.NoError(t, e.db.Create(&models.ReseedAttempt{RunID: &stale.ID, StartedAt: e.now, SiteName: "hdsky", InfoHash: "r", Outcome: models.ReseedAttemptRunning}).Error)
	runs, nAtt, err := e.svc.MarkInterrupted(ctx, e.now.Add(time.Minute))
	require.NoError(t, err)
	assert.EqualValues(t, 1, runs)
	assert.EqualValues(t, 1, nAtt)
	d, err = e.svc.GetRun(ctx, stale.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ReseedRunInterrupted, d.Status)
	assert.True(t, d.Continuable, "被打断的最近一轮可以继续")

	// 清理：超过 180 天的轮次、超过 5 年的尝试
	require.NoError(t, e.db.Model(&models.ReseedRun{}).Where("id = ?", run.ID).Update("started_at", e.now.Add(-200*24*time.Hour)).Error)
	require.NoError(t, e.db.Model(&models.ReseedAttempt{}).Where("info_hash = ?", "g").Update("started_at", e.now.Add(-6*366*24*time.Hour)).Error)
	require.NoError(t, e.svc.PruneRuns(ctx))
	_, err = e.svc.GetRun(ctx, run.ID)
	assert.ErrorIs(t, err, ErrRunNotFound)
	var left []models.ReseedAttempt
	require.NoError(t, e.db.Find(&left).Error)
	for _, a := range left {
		assert.NotEqual(t, "g", a.InfoHash)
	}
}
