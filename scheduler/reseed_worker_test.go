package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/iyuu"
	"github.com/sunerpy/pt-tools/internal/reseed"
	"github.com/sunerpy/pt-tools/internal/transfer"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

type testCipher struct{}

func (testCipher) Encrypt(p string) (string, error) { return "enc:" + p, nil }
func (testCipher) Decrypt(c string) (string, error) {
	if !strings.HasPrefix(c, "enc:") {
		return "", errors.New("bad")
	}
	return strings.TrimPrefix(c, "enc:"), nil
}

func newReseedWorkerForTest(t *testing.T) (*ReseedWorker, *reseed.Service, *gorm.DB) {
	t.Helper()
	// 测试库不刷盘（同 core.NewTempDBDir）：Windows runner 上每次写都要刷盘，一条 SQL 能慢到几秒
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "r.db")+"?_pragma=journal_mode(MEMORY)&_pragma=synchronous(OFF)"), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { closeTestGorm(db) })
	require.NoError(t, db.AutoMigrate(&models.ReseedSetting{}, &models.ReseedRecord{}, &models.ReseedRun{}, &models.ReseedAttempt{}, &models.ReseedIYUUDaily{}, &models.ReseedDailySkip{},
		&models.TorrentTransferJob{}, &models.DownloaderSetting{}))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"sites": []any{}}})
	}))
	t.Cleanup(srv.Close)
	svc := reseed.New(reseed.Config{
		DB: db, Cipher: testCipher{}, Transfer: transfer.New(transfer.Config{DB: db}),
		Downloaders: transfer.DownloadersFunc(func(context.Context, uint) (downloader.Downloader, models.DownloaderSetting, error) {
			return nil, models.DownloaderSetting{}, errors.New("下载器未启用")
		}),
		NewClient: func(token string) *iyuu.Client {
			c := iyuu.New(token)
			c.BaseURL = srv.URL
			return c
		},
		Sleep: func(context.Context, time.Duration) error { return nil },
	})
	w := NewReseedWorker(ReseedWorkerConfig{Service: svc, StartupDelay: 10 * time.Millisecond, Tick: 20 * time.Millisecond})
	t.Cleanup(w.Stop)
	return w, svc, db
}

func lastReseedResult(t *testing.T, db *gorm.DB) string {
	var r models.ReseedSetting
	if err := db.First(&r, 1).Error; err != nil {
		return ""
	}
	return r.LastResult
}

func reseedRunRow(t *testing.T, db *gorm.DB, id uint) models.ReseedRun {
	t.Helper()
	var r models.ReseedRun
	require.NoError(t, db.First(&r, id).Error)
	return r
}

// 开启并到期时定时跑一轮，把结果（这里是没有可辅种的站点）记下来，也写一条轮次记录。
func TestReseedWorker_RunsWhenDue(t *testing.T) {
	w, svc, db := newReseedWorkerForTest(t)
	tok := "tok"
	_, err := svc.SaveSettings(context.Background(), reseed.SettingsUpdate{Enabled: true, Token: &tok})
	require.NoError(t, err)
	w.Start()
	w.Start()
	require.Eventually(t, func() bool { return strings.Contains(lastReseedResult(t, db), "没有可以辅种的站点") }, 5*time.Second, 20*time.Millisecond)
	w.Stop()
	assert.False(t, w.Running())
	_, err = w.RunNow()
	assert.Error(t, err, "停止之后不再运行")
	w.Start()
	assert.Same(t, svc, w.Service())

	var runs []models.ReseedRun
	require.NoError(t, db.Find(&runs).Error)
	require.NotEmpty(t, runs)
	assert.Equal(t, models.ReseedTriggerSchedule, runs[0].Trigger)
	assert.Equal(t, models.ReseedRunFailed, runs[0].Status, "未能开始：没有站点")
	assert.Contains(t, runs[0].Error, "没有可以辅种的站点")
	assert.NotNil(t, runs[0].FinishedAt)
}

// 「立即运行」在后台跑；正在跑时再点返回 ErrReseedBusy。
func TestReseedWorker_RunNow(t *testing.T) {
	w, _, db := newReseedWorkerForTest(t)
	release := make(chan struct{})
	id, err := w.start(models.ReseedTriggerManual, func(context.Context) (string, *uint, error) { return "", nil, nil },
		func(ctx context.Context, opts reseed.RunOptions) (reseed.RunResult, error) {
			opts.Progress(reseed.RunResult{Hashes: 3, Created: 1})
			<-release
			return reseed.RunResult{Hashes: 3, Created: 1}, nil
		})
	require.NoError(t, err)
	assert.True(t, w.Running())
	_, err = w.RunNow()
	assert.ErrorIs(t, err, ErrReseedBusy)
	require.Eventually(t, func() bool { c := w.Current(); return c != nil && c.Created == 1 }, 5*time.Second, 10*time.Millisecond)
	cur := w.Current()
	assert.Equal(t, id, cur.ID)
	assert.Equal(t, models.ReseedRunRunning, cur.Status)
	close(release)
	require.Eventually(t, func() bool { return !w.Running() }, 5*time.Second, 10*time.Millisecond)
	row := reseedRunRow(t, db, id)
	assert.Equal(t, models.ReseedRunOK, row.Status)
	assert.Equal(t, 1, row.Created)
	assert.Contains(t, row.Summary, "加入 1 个")

	id2, err := w.RunNow()
	require.NoError(t, err)
	require.Eventually(t, func() bool { return strings.Contains(lastReseedResult(t, db), "没有设置 IYUU token") }, 5*time.Second, 20*time.Millisecond)
	require.Eventually(t, func() bool { return !w.Running() }, 5*time.Second, 10*time.Millisecond)
	assert.Equal(t, models.ReseedRunFailed, reseedRunRow(t, db, id2).Status)
	w.Stop()

	var nilWorker *ReseedWorker
	nilWorker.Start()
	nilWorker.Stop()
	assert.False(t, nilWorker.Running())
	assert.Nil(t, nilWorker.Service())
	assert.Nil(t, nilWorker.Current())
	_, err = nilWorker.RunNow()
	assert.Error(t, err)
	assert.ErrorIs(t, nilWorker.Pause(1), ErrReseedNotCurrent)

	mgr := &Manager{}
	w2, _, _ := newReseedWorkerForTest(t)
	mgr.SetReseedWorker(w2)
	assert.Same(t, w2, mgr.GetReseedWorker())
	w3, _, _ := newReseedWorkerForTest(t)
	mgr.SetReseedWorker(w3)
	mgr.StopAll()
	assert.Nil(t, mgr.GetReseedWorker())
}

// 暂停、继续、停止：只对正在跑的那一轮；状态写进轮次记录；停止时可以一起取消这一轮还没加入下载器的任务。
func TestReseedWorker_PauseResumeStop(t *testing.T) {
	w, _, db := newReseedWorkerForTest(t)
	steps := make(chan int)
	next := make(chan struct{})
	var jobID uint
	id, err := w.start(models.ReseedTriggerManual, func(context.Context) (string, *uint, error) { return "", nil, nil },
		func(ctx context.Context, opts reseed.RunOptions) (reseed.RunResult, error) {
			// 这一轮建了一个还在等待加入的任务
			job := models.TorrentTransferJob{Kind: models.JobKindReseed, InfoHash: "abc", State: models.TransferExported, TargetDownloaderID: 1}
			assert.NoError(t, db.Create(&job).Error)
			jobID = job.ID
			assert.NoError(t, db.Create(&models.ReseedAttempt{
				RunID: &opts.RunID, StartedAt: time.Now(), SiteName: "hdsky", InfoHash: "abc",
				Outcome: models.ReseedAttemptCreated, JobID: &job.ID,
			}).Error)
			var res reseed.RunResult
			for i := 0; ; i++ {
				if _, err := opts.Control.Wait(ctx); err != nil {
					res.End, res.Stopped = reseed.EndStopped, context.Cause(ctx).Error()
					return res, nil
				}
				steps <- i
				<-next
			}
		})
	require.NoError(t, err)
	assert.Equal(t, 0, <-steps)

	assert.ErrorIs(t, w.Pause(id+1), ErrReseedNotCurrent, "不是正在跑的那一轮")
	require.NoError(t, w.Pause(id))
	require.NoError(t, w.Pause(id), "已经暂停时什么都不做")
	assert.Equal(t, models.ReseedRunPaused, reseedRunRow(t, db, id).Status)
	assert.NotNil(t, reseedRunRow(t, db, id).PausedAt)
	assert.Equal(t, models.ReseedRunPaused, w.Current().Status)
	next <- struct{}{} // 下一个检查点停在暂停上

	require.NoError(t, w.Resume(id))
	require.NoError(t, w.Resume(id), "没在暂停时什么都不做")
	assert.Equal(t, 1, <-steps, "继续以后接着跑")
	row := reseedRunRow(t, db, id)
	assert.Equal(t, models.ReseedRunRunning, row.Status)
	assert.Nil(t, row.PausedAt)

	require.NoError(t, w.StopRun(id, true))
	next <- struct{}{}
	require.Eventually(t, func() bool { return !w.Running() }, 5*time.Second, 10*time.Millisecond)
	row = reseedRunRow(t, db, id)
	assert.Equal(t, models.ReseedRunStopped, row.Status)
	assert.Equal(t, reseed.ErrRunStopped.Error(), row.Error)
	var job models.TorrentTransferJob
	require.NoError(t, db.First(&job, jobID).Error)
	assert.Equal(t, models.TransferCanceled, job.State, "停止时一起取消了这一轮还没加入下载器的任务")
	assert.ErrorIs(t, w.StopRun(id, false), ErrReseedNotCurrent, "已经结束")
}

// 暂停太久自动停止；时限不算暂停的时间（用可控的计时器）。
func TestReseedWorker_PausedTooLong(t *testing.T) {
	w, _, db := newReseedWorkerForTest(t)
	timers := make(chan *fakeTimer, 4)
	w.cfg.NewTimer = func(d time.Duration) reseed.Timer {
		ft := &fakeTimer{d: d, c: make(chan time.Time, 1)}
		timers <- ft
		return ft
	}
	waiting := make(chan struct{})
	id, err := w.start(models.ReseedTriggerManual, func(context.Context) (string, *uint, error) { return "", nil, nil },
		func(ctx context.Context, opts reseed.RunOptions) (reseed.RunResult, error) {
			close(waiting)
			<-ctx.Done()
			return reseed.RunResult{End: reseed.EndStopped, Stopped: context.Cause(ctx).Error()}, nil
		})
	require.NoError(t, err)
	<-waiting
	budget := <-timers
	assert.Equal(t, reseedRunTimeout, budget.d, "没暂停时按这一轮的时限计时")
	require.NoError(t, w.Pause(id))
	pause := <-timers
	assert.Equal(t, reseedMaxPause, pause.d, "暂停时按一次暂停的最长时间计时")
	pause.c <- time.Now()
	require.Eventually(t, func() bool { return !w.Running() }, 5*time.Second, 10*time.Millisecond)
	row := reseedRunRow(t, db, id)
	assert.Equal(t, models.ReseedRunStopped, row.Status)
	assert.Contains(t, row.Error, "暂停太久")
}

type fakeTimer struct {
	d time.Duration
	c chan time.Time
}

func (f *fakeTimer) C() <-chan time.Time { return f.c }
func (f *fakeTimer) Stop() bool          { return true }

// 启动时把上一个进程留下的进行中轮次与尝试记成被打断；「继续」只对最近一个提前停下的轮次可用。
func TestReseedWorker_InterruptedAndContinue(t *testing.T) {
	w, svc, db := newReseedWorkerForTest(t)
	ctx := context.Background()
	old := time.Now().Add(-time.Hour)
	stale := models.ReseedRun{Trigger: models.ReseedTriggerSchedule, StartedAt: old, Status: models.ReseedRunPaused, Cursor: "cccc", After: "aaaa"}
	require.NoError(t, db.Create(&stale).Error)
	require.NoError(t, db.Create(&models.ReseedAttempt{
		RunID: &stale.ID, StartedAt: old, SiteName: "hdsky", InfoHash: "x",
		Outcome: models.ReseedAttemptRunning,
	}).Error)
	w.cfg.StartupDelay = time.Hour
	w.Start()
	// 轮次与尝试是先后两条 UPDATE：两个都改完才算补记完
	require.Eventually(t, func() bool {
		var att models.ReseedAttempt
		return reseedRunRow(t, db, stale.ID).Status == models.ReseedRunInterrupted &&
			db.First(&att).Error == nil && att.Outcome == models.ReseedAttemptInterrupted
	}, 5*time.Second, 10*time.Millisecond)

	after, from, err := svc.ContinuePoint(ctx)
	require.NoError(t, err)
	assert.Equal(t, "cccc", after, "定时轮次接着被打断的那一轮往下查")
	assert.Equal(t, stale.ID, *from)

	_, err = w.Continue(stale.ID + 100)
	require.ErrorIs(t, err, reseed.ErrRunNotFound)
	id, err := w.Continue(stale.ID)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return !w.Running() }, 5*time.Second, 10*time.Millisecond)
	row := reseedRunRow(t, db, id)
	assert.Equal(t, models.ReseedTriggerContinue, row.Trigger)
	assert.Equal(t, stale.ID, *row.ContinuedFrom)
	assert.Equal(t, "cccc", row.After, "从被打断的那一轮完整处理完的最后一批之后开始")
	assert.Equal(t, models.ReseedRunFailed, row.Status, "没有 token")

	_, err = w.Continue(stale.ID)
	assert.ErrorIs(t, err, reseed.ErrNotContinuable, "已经不是最近一轮了")
}

// 重试在后台跑一轮（触发方式 retry），不改「上次运行」与定时间隔；选中的记录都不能重试时不开始。
func TestReseedWorker_Retry(t *testing.T) {
	w, _, db := newReseedWorkerForTest(t)
	failed := models.ReseedRecord{InfoHash: "aa", SiteName: "hdsky", TorrentID: "1", SourceHash: "src", DownloaderID: 1, State: models.ReseedFailed}
	done := models.ReseedRecord{InfoHash: "bb", SiteName: "hdsky", TorrentID: "2", SourceHash: "src", DownloaderID: 1, State: models.ReseedDone}
	require.NoError(t, db.Create(&failed).Error)
	require.NoError(t, db.Create(&done).Error)

	_, err := w.Retry([]uint{done.ID})
	require.ErrorIs(t, err, reseed.ErrNothingToRetry)
	var n int64
	require.NoError(t, db.Model(&models.ReseedRun{}).Count(&n).Error)
	assert.Zero(t, n, "没有开始就不记轮次")
	assert.False(t, w.Running())

	id, err := w.Retry([]uint{failed.ID, done.ID})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return !w.Running() }, 5*time.Second, 10*time.Millisecond)
	row := reseedRunRow(t, db, id)
	assert.Equal(t, models.ReseedTriggerRetry, row.Trigger)
	assert.Equal(t, models.ReseedRunPartial, row.Status, "下载器不可用")
	assert.Equal(t, 2, row.Hashes)
	assert.Contains(t, row.Summary, "重试 2 条记录")
	assert.Contains(t, row.SkippedDetail, reseed.SkipNoDownloader)
	assert.Contains(t, row.SkippedDetail, reseed.SkipNotRetryable)
	var sets []models.ReseedSetting
	require.NoError(t, db.Find(&sets).Error)
	for _, set := range sets {
		assert.Nil(t, set.LastRunAt, "重试不算进定时运行的间隔")
	}

	var nilWorker *ReseedWorker
	_, err = nilWorker.Retry([]uint{1})
	assert.Error(t, err)
}

// IYUU 限流冷却中：立即运行、继续不开始（返回冷却的错误），定时轮次不到期；读站点列表时就被限流的一轮记为提前停下。
func TestReseedWorker_IYUUCooling(t *testing.T) {
	w, svc, db := newReseedWorkerForTest(t)
	ctx := context.Background()
	tok := "tok"
	_, err := svc.SaveSettings(ctx, reseed.SettingsUpdate{Enabled: true, Token: &tok})
	require.NoError(t, err)
	until := time.Now().Add(time.Hour)
	require.NoError(t, db.Model(&models.ReseedSetting{}).Where("id = 1").Update("iyuu_cooldown_until", until).Error)
	stopped := models.ReseedRun{Trigger: models.ReseedTriggerManual, StartedAt: time.Now().Add(-time.Hour), Status: models.ReseedRunStopped}
	require.NoError(t, db.Create(&stopped).Error)

	var ce *reseed.CoolingError
	_, err = w.RunNow()
	require.ErrorAs(t, err, &ce)
	_, err = w.Continue(stopped.ID)
	require.ErrorAs(t, err, &ce)
	assert.False(t, w.Running())
	var n int64
	require.NoError(t, db.Model(&models.ReseedRun{}).Count(&n).Error)
	assert.EqualValues(t, 1, n, "没有开始新的一轮")
	assert.False(t, svc.Due(ctx), "冷却中不跑定时轮次")

	status, text := reseedRunStatus(ctx, reseed.RunResult{}, fmt.Errorf("读取 IYUU 站点列表失败: %w", &reseed.CoolingError{Until: until}))
	assert.Equal(t, models.ReseedRunLimited, status)
	assert.Contains(t, text, "冷却中")
	status, _ = reseedRunStatus(ctx, reseed.RunResult{}, fmt.Errorf("向 IYUU 报告站点失败: %w", reseed.ErrIYUUDailyCap))
	assert.Equal(t, models.ReseedRunLimited, status)
}
