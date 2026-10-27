package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func onlyRun(t *testing.T, r *brushRig, taskID uint) models.BrushRun {
	t.Helper()
	rows, total, err := models.NewBrushRepository(r.db.DB).ListRuns(taskID, 10, 0)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	return rows[0]
}

func stepTexts(run models.BrushRun, kind string) []string {
	var out []string
	for _, st := range run.StepList() {
		if st.Kind == kind {
			out = append(out, st.Text)
		}
	}
	return out
}

// 一轮跑完写一条运行记录：读到的列表、不符合条件的种子与原因、加入的、删掉的与不见了的种子都在步骤里，计数与摘要和这一轮的结果一致。
func TestBrushRun_RecordsStepsAndCounts(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, func(bt *models.BrushTask) { bt.RemoveRatio = 2; bt.MaxDownloading = 5 })
	repo := models.NewBrushRepository(r.db.DB)
	for _, h := range []string{"done", "missing"} {
		require.NoError(t, repo.RecordAdded(&models.BrushTorrent{
			TaskID: task.ID, InfoHash: h, SiteName: "hdsky", TorrentID: "old-" + h, Title: "T-" + h,
			AddedAt: r.clock.Now().Add(-time.Hour), State: models.BrushTorrentActive, DownloaderID: 1, SizeBytes: gib,
		}, "2026-10-06"))
	}
	r.dl.torrents = []downloader.Torrent{
		{ID: "id-done", InfoHash: "done", Tags: models.BrushTaskTag(task.ID), Progress: 1, Ratio: 2.5, TotalUploaded: 9000},
	}
	r.addItem(t, "1", nil)
	r.addItem(t, "2", func(i *v2.TorrentItem) { i.HasHR = true })
	r.addItem(t, "3", func(i *v2.TorrentItem) { i.DiscountLevel = v2.DiscountNone })

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)

	run := onlyRun(t, r, task.ID)
	assert.Equal(t, models.BrushRunSchedule, run.Trigger)
	assert.Equal(t, models.BrushRunOK, run.Status)
	assert.Empty(t, run.Error)
	assert.Equal(t, res.Summary(), run.Summary)
	assert.Equal(t, []int{3, 1, 1, 1, 1}, []int{run.Listed, run.Eligible, run.Added, run.Removed, run.Gone})
	require.NotNil(t, run.FinishedAt)
	assert.False(t, run.StepsTruncated)

	assert.Equal(t, []string{"T-missing：下载器里已经没有这个种子"}, stepTexts(run, brushStepGone))
	removed := stepTexts(run, brushStepRemove)
	require.Len(t, removed, 1)
	assert.True(t, strings.HasPrefix(removed[0], "T-done：分享率"), removed[0])
	assert.Equal(t, []string{"读取 hdsky 的免费种子列表：3 个"}, stepTexts(run, brushStepList))
	assert.Equal(t, []string{
		"任务名下在做 0 个种子（下载中 0 个，共 0.00 GB），今天已加入 2.00 GB",
		"不符合条件 2 个、符合条件 1 个",
	}, stepTexts(run, brushStepInfo))
	assert.Equal(t, []string{
		"原因汇总：优惠类型 NONE 不在允许范围 1 个、有 H&R 1 个",
		"Title 2：有 H&R",
		"Title 3：优惠类型 NONE 不在允许范围",
	}, stepTexts(run, brushStepReject))
	assert.Equal(t, []string{"Title 1（免费，4.00 GB，优惠到 10-06 16:00）"}, stepTexts(run, brushStepAdd))

	// 时间线的顺序：先处理已有的种子（按加入顺序），再读列表、判定，最后加种
	var kinds []string
	for _, st := range run.StepList() {
		if len(kinds) == 0 || kinds[len(kinds)-1] != st.Kind {
			kinds = append(kinds, st.Kind)
		}
	}
	assert.Equal(t, []string{brushStepRemove, brushStepGone, brushStepInfo, brushStepList, brushStepInfo, brushStepReject, brushStepAdd}, kinds)
}

// 「立即运行」记成手动；整轮失败（列表读不到）时记成失败，原因写在错误与最后一步里。
func TestBrushRun_ManualFailure(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, nil)
	r.site.searchErr = errors.New("站点返回 502")

	_, err := r.mon.RunTaskManual(context.Background(), task.ID)
	require.Error(t, err)

	run := onlyRun(t, r, task.ID)
	assert.Equal(t, models.BrushRunManual, run.Trigger)
	assert.Equal(t, models.BrushRunFailed, run.Status)
	assert.Contains(t, run.Error, "站点返回 502")
	steps := run.StepList()
	require.NotEmpty(t, steps)
	last := steps[len(steps)-1]
	assert.Equal(t, brushStepError, last.Kind)
	assert.Contains(t, last.Text, "站点返回 502")
}

// 跑完了但有个别种子出错：记成部分失败，出错的种子与加入的种子都在步骤里。
func TestBrushRun_PartialWhenSomeTorrentsFail(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 5 })
	r.addItem(t, "1", func(i *v2.TorrentItem) { i.Leechers = 90 })
	delete(r.site.files, "1")
	r.addItem(t, "2", nil)

	res, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, 1, res.Added)

	run := onlyRun(t, r, task.ID)
	assert.Equal(t, models.BrushRunPartial, run.Status)
	assert.Contains(t, run.Error, "下载 Title 1 的种子文件失败")
	assert.Len(t, stepTexts(run, brushStepError), 1)
	assert.Len(t, stepTexts(run, brushStepAdd), 1)
}

// 被闸门拒绝、下载器里已有：停下的原因与跳过的种子都记成一步，这一轮仍算成功（不是出错）。
func TestBrushRun_StopAndSkipSteps(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, func(bt *models.BrushTask) { bt.MaxDownloading = 1 })
	r.addItem(t, "1", nil)
	r.addItem(t, "2", nil)

	_, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)

	run := onlyRun(t, r, task.ID)
	assert.Equal(t, models.BrushRunOK, run.Status)
	assert.Equal(t, []string{"同时下载已有 1 个，达到上限"}, stepTexts(run, brushStepStop))
}

// 不符合条件的种子逐个列出最多 30 个，其余的只写个数；按原因的汇总在前面。
func TestBrushRun_RejectsCapped(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, nil)
	for i := range 35 {
		r.addItem(t, fmt.Sprint(100+i), func(it *v2.TorrentItem) { it.DiscountLevel = v2.DiscountNone })
	}

	_, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)

	rejects := stepTexts(onlyRun(t, r, task.ID), brushStepReject)
	require.Len(t, rejects, 1+brushRunMaxRejects+1)
	assert.Equal(t, "原因汇总：优惠类型 NONE 不在允许范围 35 个", rejects[0])
	assert.Equal(t, "Title 100：优惠类型 NONE 不在允许范围", rejects[1])
	assert.Equal(t, "另有 5 个不符合条件的种子没有逐个列出", rejects[len(rejects)-1])
}

// 步骤最多记 16 KiB：超出的不记，并标出记不全；单步的文字也有上限。
func TestBrushRunLog_Limits(t *testing.T) {
	var l brushRunLog
	long := strings.Repeat("长", brushRunStepText+50)
	for range 200 {
		l.add(brushStepInfo, long)
	}
	assert.True(t, l.truncated)
	assert.LessOrEqual(t, l.size, brushRunMaxSteps)
	require.NotEmpty(t, l.steps)
	assert.Equal(t, brushRunStepText+1, len([]rune(l.steps[0].Text)), "截到上限再加省略号")
	raw, truncated := l.encoded()
	assert.True(t, truncated)
	assert.NotEmpty(t, raw)

	var nilLog *brushRunLog
	nilLog.add(brushStepInfo, "x") // 不 panic
	raw, truncated = nilLog.encoded()
	assert.Empty(t, raw)
	assert.False(t, truncated)
}

// 启动时把上一个进程留下的「进行中」记成没跑完；这个进程里开始的一轮不动。
func TestBrushRun_StartMarksInterrupted(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, nil)
	repo := models.NewBrushRepository(r.db.DB)
	old, err := repo.StartRun(task.ID, models.BrushRunSchedule, r.clock.Now().Add(-time.Hour))
	require.NoError(t, err)
	cur, err := repo.StartRun(task.ID, models.BrushRunManual, r.clock.Now())
	require.NoError(t, err)

	r.mon.Start()
	r.mon.Stop()

	rows, _, err := repo.ListRuns(task.ID, 10, 0)
	require.NoError(t, err)
	status := map[uint]models.BrushRun{}
	for _, row := range rows {
		status[row.ID] = row
	}
	assert.Equal(t, models.BrushRunInterrupted, status[old.ID].Status)
	assert.Contains(t, status[old.ID].Error, "中途退出")
	assert.NotNil(t, status[old.ID].FinishedAt)
	assert.Equal(t, models.BrushRunRunning, status[cur.ID].Status)
}

// 进程退出时取消了正在跑的一轮：记成没跑完，不当成失败。
func TestBrushRun_InterruptedByStop(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, nil)
	site := blockingSite{fakeBrushSite: r.site, entered: make(chan struct{})}
	r.mon.cfg.Sites = BrushSitesFunc(func(string) (v2.Site, bool) { return site, true })

	done := make(chan error, 1)
	go func() {
		_, err := r.mon.RunTaskManual(context.Background(), task.ID)
		done <- err
	}()
	<-site.entered
	r.mon.Stop() // 等这一轮退出（运行记录在退出前写完）
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop 之后手动运行没有退出")
	}

	run := onlyRun(t, r, task.ID)
	assert.Equal(t, models.BrushRunInterrupted, run.Status)
	assert.True(t, strings.HasPrefix(run.Error, "pt-tools 正在退出，这一轮没有跑完；"), run.Error)
}

// 每小时清理一次：超过 14 天的运行记录删掉。
func TestBrushRun_PrunedHourly(t *testing.T) {
	r := newBrushRig(t)
	repo := models.NewBrushRepository(r.db.DB)
	_, err := repo.StartRun(99, models.BrushRunSchedule, r.clock.Now().Add(-15*24*time.Hour))
	require.NoError(t, err)
	recent, err := repo.StartRun(99, models.BrushRunSchedule, r.clock.Now().Add(-24*time.Hour))
	require.NoError(t, err)

	r.mon.RunOnce(context.Background())

	rows, total, err := repo.ListRuns(99, 10, 0)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	assert.Equal(t, recent.ID, rows[0].ID)
}

func TestBrushAddedText(t *testing.T) {
	end := time.Date(2026, 10, 7, 12, 30, 0, 0, time.UTC)
	cst := time.FixedZone("CST", 8*3600)
	assert.Equal(t, "A（2x 免费，1.50 GB，优惠到 10-07 20:30，H&R 做种 72 小时）",
		brushAddedText("A", v2.Discount2xFree, gib*3/2, &end, true, 72, cst))
	assert.Equal(t, "B（免费，2.00 GB，H&R）", brushAddedText("B", v2.DiscountFree, 2*gib, nil, true, 0, cst))
	assert.Equal(t, "C（无优惠，0.00 GB）", brushAddedText("C", "", 0, nil, false, 0, cst))
	assert.Equal(t, "D（WEIRD，0.00 GB）", brushAddedText("D", "WEIRD", 0, nil, false, 0, cst))
}
