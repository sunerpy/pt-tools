package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newBrushTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return newMemDB(t, &BrushTask{}, &BrushTorrent{}, &BrushTorrentSample{}, &BrushDailyStat{})
}

func TestBrushTask_TagList(t *testing.T) {
	task := BrushTask{ID: 7, SiteName: "hdsky", Tags: " 刷流 ,hdsky, PT-TOOLS-BRUSH ,,extra"}
	assert.Equal(t, []string{"hdsky", "pt-tools-brush", "pt-brush-7", "刷流", "extra"}, task.TagList(),
		"站点名、总标签、任务标签在前，用户标签去重（大小写不敏感）后跟上")
}

func TestBrushRepository_SaveTaskKeepsRunColumns(t *testing.T) {
	repo := NewBrushRepository(newBrushTestDB(t))
	task := &BrushTask{Name: "a", SiteName: "hdsky", DownloaderID: 1, IntervalMin: 10, ExcludeHR: true}
	require.NoError(t, repo.SaveTask(task))
	require.NotZero(t, task.ID)
	at := time.Date(2026, 10, 6, 1, 0, 0, 0, time.UTC)
	require.NoError(t, repo.MarkRun(task.ID, at, "加入 1 个", ""))

	task.Enabled = true
	task.ExcludeHR = false
	task.LastResult = "不该写进去"
	require.NoError(t, repo.SaveTask(task))

	got, err := repo.GetTask(task.ID)
	require.NoError(t, err)
	assert.True(t, got.Enabled)
	assert.False(t, got.ExcludeHR, "零值也要写回（整行更新）")
	assert.Equal(t, "加入 1 个", got.LastResult, "保存配置不覆盖运行状态列")
	require.NotNil(t, got.LastRunAt)
	assert.True(t, got.LastRunAt.Equal(at))

	_, err = repo.GetTask(999)
	assert.ErrorIs(t, err, ErrBrushTaskNotFound)
}

func TestBrushRepository_RecordAddedIsIdempotent(t *testing.T) {
	repo := NewBrushRepository(newBrushTestDB(t))
	bt := &BrushTorrent{TaskID: 1, InfoHash: "abc", SiteName: "hdsky", TorrentID: "11", SizeBytes: 100, AddedAt: time.Now(), State: BrushTorrentActive}
	require.NoError(t, repo.RecordAdded(bt, "2026-10-06"))
	dup := &BrushTorrent{TaskID: 1, InfoHash: "abc", SiteName: "hdsky", TorrentID: "11", SizeBytes: 100, AddedAt: time.Now(), State: BrushTorrentActive}
	require.NoError(t, repo.RecordAdded(dup, "2026-10-06"))

	stats, err := repo.DailyStats(1, "2026-10-06", "2026-10-06")
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.Equal(t, 1, stats[0].Added, "同一个种子只记一次")
	assert.EqualValues(t, 100, stats[0].AddedBytes)

	seen, err := repo.SeenTorrentIDs(1)
	require.NoError(t, err)
	assert.True(t, seen["11"])
}

// 第一次采样把累计值整个算进当天（推送前下载器里没有它）；之后只计正的差值；没有增长时活动时间不动。
func TestBrushRepository_RecordSampleAccumulatesDeltas(t *testing.T) {
	repo := NewBrushRepository(newBrushTestDB(t))
	t0 := time.Date(2026, 10, 6, 1, 0, 0, 0, time.UTC)
	bt := &BrushTorrent{TaskID: 1, InfoHash: "abc", SiteName: "hdsky", TorrentID: "11", AddedAt: t0, State: BrushTorrentActive}
	require.NoError(t, repo.RecordAdded(bt, "2026-10-06"))

	require.NoError(t, repo.RecordSample(bt, BrushTorrentSample{At: t0.Add(time.Minute), Uploaded: 50, Downloaded: 200}, 0.5, 0.25, 0, "2026-10-06"))
	require.NoError(t, repo.RecordSample(bt, BrushTorrentSample{At: t0.Add(2 * time.Minute), Uploaded: 80, Downloaded: 400}, 1, 0.2, 60, "2026-10-06"))
	activity := *bt.LastActivityAt
	// 下载器重启后累计值变小：按 0 计，不倒扣
	require.NoError(t, repo.RecordSample(bt, BrushTorrentSample{At: t0.Add(3 * time.Minute), Uploaded: 70, Downloaded: 400}, 1, 0.17, 120, "2026-10-06"))
	assert.True(t, bt.LastActivityAt.Equal(activity), "没有增长时活动时间不前进")
	// 次日的增量记到次日
	require.NoError(t, repo.RecordSample(bt, BrushTorrentSample{At: t0.Add(24 * time.Hour), Uploaded: 170, Downloaded: 400}, 1, 0.42, 3600, "2026-10-07"))

	stats, err := repo.DailyStats(1, "2026-10-06", "2026-10-07")
	require.NoError(t, err)
	require.Len(t, stats, 2)
	assert.EqualValues(t, 80, stats[0].Uploaded)
	assert.EqualValues(t, 400, stats[0].Downloaded)
	assert.EqualValues(t, 100, stats[1].Uploaded)
	assert.EqualValues(t, 0, stats[1].Downloaded)

	var row BrushTorrent
	require.NoError(t, repo.db.First(&row, bt.ID).Error)
	assert.EqualValues(t, 170, row.Uploaded)
	assert.EqualValues(t, 3600, row.SeedingTimeSec)
	assert.InDelta(t, 0.42, row.Ratio, 1e-9)

	samples, err := repo.SamplesSince(bt.ID, t0.Add(150*time.Second))
	require.NoError(t, err)
	require.Len(t, samples, 3, "窗口起点之前最近的一条也带上")
	assert.EqualValues(t, 80, samples[0].Uploaded)

	n, err := repo.PruneSamples(t0.Add(150 * time.Second))
	require.NoError(t, err)
	assert.EqualValues(t, 2, n)
}

func TestBrushRepository_MarkEndedCountsRemovedOnce(t *testing.T) {
	repo := NewBrushRepository(newBrushTestDB(t))
	now := time.Date(2026, 10, 6, 1, 0, 0, 0, time.UTC)
	removed := &BrushTorrent{TaskID: 1, InfoHash: "a", SiteName: "s", TorrentID: "1", AddedAt: now, State: BrushTorrentActive}
	gone := &BrushTorrent{TaskID: 1, InfoHash: "b", SiteName: "s", TorrentID: "2", AddedAt: now, State: BrushTorrentActive}
	require.NoError(t, repo.RecordAdded(removed, "2026-10-06"))
	require.NoError(t, repo.RecordAdded(gone, "2026-10-06"))

	require.NoError(t, repo.MarkEnded(removed, BrushTorrentRemoved, "分享率 2.00 ≥ 2", now, "2026-10-06"))
	require.NoError(t, repo.MarkEnded(removed, BrushTorrentRemoved, "再来一次", now, "2026-10-06"))
	require.NoError(t, repo.MarkEnded(gone, BrushTorrentGone, "下载器里已经没有了", now, "2026-10-06"))

	stats, err := repo.DailyStats(1, "2026-10-06", "2026-10-06")
	require.NoError(t, err)
	require.Len(t, stats, 1)
	assert.Equal(t, 1, stats[0].Removed, "只有刷流删掉的计删除，重复标记不重复计")

	active, err := repo.ActiveTorrents(1)
	require.NoError(t, err)
	assert.Empty(t, active)

	rows, total, err := repo.ListTorrents(1, BrushTorrentRemoved, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	assert.Equal(t, "分享率 2.00 ≥ 2", rows[0].RemoveReason)
}

func TestBrushRepository_DeleteTaskCascades(t *testing.T) {
	db := newBrushTestDB(t)
	repo := NewBrushRepository(db)
	task := &BrushTask{Name: "a", SiteName: "hdsky", DownloaderID: 1}
	require.NoError(t, repo.SaveTask(task))
	bt := &BrushTorrent{TaskID: task.ID, InfoHash: "a", SiteName: "hdsky", TorrentID: "1", AddedAt: time.Now(), State: BrushTorrentActive}
	require.NoError(t, repo.RecordAdded(bt, "2026-10-06"))
	require.NoError(t, repo.RecordSample(bt, BrushTorrentSample{At: time.Now(), Uploaded: 1}, 0, 0, 0, "2026-10-06"))

	require.NoError(t, repo.DeleteTask(task.ID))
	for _, model := range []any{&BrushTask{}, &BrushTorrent{}, &BrushTorrentSample{}, &BrushDailyStat{}} {
		var n int64
		require.NoError(t, db.Model(model).Count(&n).Error)
		assert.Zero(t, n)
	}
	assert.ErrorIs(t, repo.DeleteTask(task.ID), ErrBrushTaskNotFound)
}

func TestBrushRepository_TaskTotals(t *testing.T) {
	repo := NewBrushRepository(newBrushTestDB(t))
	now := time.Date(2026, 10, 6, 1, 0, 0, 0, time.UTC)
	for i, day := range []string{"2026-10-05", "2026-10-06"} {
		bt := &BrushTorrent{TaskID: 3, InfoHash: day, SiteName: "s", TorrentID: day, AddedAt: now, State: BrushTorrentActive}
		require.NoError(t, repo.RecordAdded(bt, day))
		require.NoError(t, repo.RecordSample(bt, BrushTorrentSample{At: now.Add(time.Duration(i) * time.Hour), Uploaded: 10, Downloaded: 5}, 1, 2, 0, day))
	}
	totals, err := repo.TaskTotals()
	require.NoError(t, err)
	assert.EqualValues(t, 20, totals[3].Uploaded)
	assert.EqualValues(t, 10, totals[3].Downloaded)
	assert.Equal(t, 2, totals[3].Added)
}
