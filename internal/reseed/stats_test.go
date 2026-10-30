package reseed

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/utils"
)

// 范围：最近几段（最后一段是当前这一段）或者自定义的两天（含两头，第一段与最后一段裁到范围以内）。
func TestStatsRange(t *testing.T) {
	now := time.Date(2026, 10, 30, 10, 0, 0, 0, time.Local) // 星期五
	gran, from, to, starts, custom, err := statsRange(StatsQuery{}, now)
	require.NoError(t, err)
	assert.Equal(t, utils.PeriodDay, gran)
	assert.False(t, custom)
	assert.Len(t, starts, 30)
	assert.Equal(t, "2026-10-01", from.Format(time.DateOnly))
	assert.Equal(t, "2026-10-30", to.Format(time.DateOnly))

	_, from, _, starts, _, err = statsRange(StatsQuery{Granularity: "week", Periods: 2}, now)
	require.NoError(t, err)
	assert.Equal(t, "2026-10-19", from.Format(time.DateOnly), "周从周一开始")
	assert.Len(t, starts, 2)

	_, from, to, starts, custom, err = statsRange(StatsQuery{Granularity: "month", From: "2026-08-15", To: "2026-10-03"}, now)
	require.NoError(t, err)
	assert.True(t, custom)
	assert.Equal(t, "2026-08-15", from.Format(time.DateOnly))
	assert.Equal(t, "2026-10-03", to.Format(time.DateOnly))
	require.Len(t, starts, 3)
	assert.Equal(t, "2026-08-01", starts[0].Format(time.DateOnly))

	for _, q := range []StatsQuery{
		{Granularity: "hour"},
		{Granularity: "day", Periods: 367},
		{From: "2026-10-01"},
		{From: "2026/10/01", To: "2026-10-02"},
		{From: "2026-10-05", To: "2026-10-01"},
		{Granularity: "month", From: "2020-01-01", To: "2026-10-01"},
		{Granularity: "day", From: "2025-01-01", To: "2026-10-01"},
	} {
		_, _, _, _, _, err := statsRange(q, now)
		require.ErrorIs(t, err, ErrInvalid, "%+v", q)
	}
}

func dayAt(t *testing.T, day string, hour int) time.Time {
	t.Helper()
	d, err := time.ParseInLocation(time.DateOnly, day, time.Local)
	require.NoError(t, err)
	return d.Add(time.Duration(hour) * time.Hour)
}

// 统计：尝试按开始的那一天，任务结果按完成的那一天（清除任务时抄进尝试行的优先），跳过与 IYUU 请求按天的计数；
// 被打断、还在尝试、任务被删掉不知道结果的都不算。
func TestStatsCounts(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.now = dayAt(t, "2026-10-30", 10)
	mkJob := func(state string, finished *time.Time) *uint {
		j := models.TorrentTransferJob{Kind: models.JobKindReseed, TargetDownloaderID: 1, InfoHash: "x", State: state, FinishedAt: finished}
		require.NoError(t, e.db.Create(&j).Error)
		return &j.ID
	}
	doneAt := dayAt(t, "2026-10-30", 8)
	missing := uint(999)
	copiedAt := dayAt(t, "2026-10-28", 9)
	for _, a := range []models.ReseedAttempt{
		// 10-29 建成、10-30 校验通过
		{StartedAt: dayAt(t, "2026-10-29", 23), SiteName: "hdsky", Outcome: models.ReseedAttemptCreated, JobID: mkJob(models.TransferDone, &doneAt)},
		// 还在校验：只算建成
		{StartedAt: dayAt(t, "2026-10-30", 1), SiteName: "hdsky", Outcome: models.ReseedAttemptCreated, JobID: mkJob(models.TransferChecking, nil)},
		// 清除任务时抄进来的结果
		{StartedAt: dayAt(t, "2026-10-28", 7), SiteName: "ourbits", Outcome: models.ReseedAttemptCreated, JobID: &missing, JobState: models.TransferRolledBack, JobFinishedAt: &copiedAt},
		// 任务被别的途径删掉、结果不知道
		{StartedAt: dayAt(t, "2026-10-28", 8), SiteName: "ourbits", Outcome: models.ReseedAttemptCreated, JobID: func() *uint { v := uint(1000); return &v }()},
		{StartedAt: dayAt(t, "2026-10-30", 2), SiteName: "hdsky", Outcome: models.ReseedAttemptFailed},
		{StartedAt: dayAt(t, "2026-10-30", 3), SiteName: "hdsky", Outcome: models.ReseedAttemptGone},
		{StartedAt: dayAt(t, "2026-10-30", 4), SiteName: "hdsky", Outcome: models.ReseedAttemptActive},
		{StartedAt: dayAt(t, "2026-10-30", 5), SiteName: "hdsky", Outcome: models.ReseedAttemptInterrupted},
		{StartedAt: dayAt(t, "2026-10-30", 6), SiteName: "hdsky", Outcome: models.ReseedAttemptRunning},
		// 范围以外
		{StartedAt: dayAt(t, "2026-10-20", 6), SiteName: "hdsky", Outcome: models.ReseedAttemptFailed},
	} {
		a.InfoHash = "h"
		require.NoError(t, e.db.Create(&a).Error)
	}
	for _, k := range []models.ReseedDailySkip{
		{Day: "2026-10-29", SiteName: "hdsky", Present: 5, Tried: 2},
		{Day: "2026-10-30", SiteName: "hdsky", DailyLimit: 1},
		{Day: "2026-10-30", SiteName: "", NotSelected: 4},
	} {
		require.NoError(t, e.db.Create(&k).Error)
	}
	last := dayAt(t, "2026-10-30", 9)
	for _, c := range []models.ReseedIYUUDaily{
		{Day: "2026-10-29", Endpoint: models.IYUUEndpointQuery, Calls: 3, OK: 2, RateLimited: 1, Hashes: 400},
		{Day: "2026-10-30", Endpoint: models.IYUUEndpointQuery, Calls: 1, OK: 1, Hashes: 200, LastAt: &last},
		{Day: "2026-10-30", Endpoint: models.IYUUEndpointSites, Calls: 1, OK: 1},
	} {
		require.NoError(t, e.db.Create(&c).Error)
	}

	st, err := e.svc.Stats(ctx, StatsQuery{Granularity: "day", Periods: 3})
	require.NoError(t, err)
	require.Len(t, st.Series, 3)
	assert.Equal(t, "2026-10-28", st.Series[0].Date)
	d28, d29, d30 := st.Series[0].StatsCounts, st.Series[1].StatsCounts, st.Series[2].StatsCounts
	assert.Equal(t, 2, d28.Created)
	assert.Equal(t, 1, d28.RolledBack, "清除任务时抄进来的结果")
	assert.Equal(t, 1, d29.Created)
	assert.Equal(t, 5, d29.SkipPresent)
	assert.Equal(t, 2, d29.SkipTried)
	assert.Equal(t, 3, d29.IYUUCalls)
	assert.Equal(t, 1, d29.IYUURateLimited)
	assert.Equal(t, 1, d30.Created)
	assert.Equal(t, 1, d30.Succeeded, "按任务完成的那一天")
	assert.Equal(t, 1, d30.Failed)
	assert.Equal(t, 1, d30.Gone)
	assert.Equal(t, 1, d30.Active)
	assert.Equal(t, 1, d30.SkipDailyLimit)
	assert.Equal(t, 4, d30.SkipNotSelected)
	assert.Equal(t, 2, d30.IYUUCalls)
	assert.Equal(t, 4, st.Totals.Created)
	assert.Equal(t, 600, st.Totals.IYUUHashes)
	assert.Zero(t, st.Totals.JobFailed+st.Totals.Canceled)

	bySite := map[string]StatsSite{}
	for _, site := range st.Sites {
		bySite[site.SiteName] = site
	}
	assert.Equal(t, "hdsky", st.Sites[0].SiteName, "试得多的在前")
	assert.Equal(t, 2, bySite["hdsky"].Created)
	assert.Equal(t, 1, bySite["hdsky"].Succeeded)
	assert.Equal(t, 6, bySite["hdsky"].SkipPresent+bySite["hdsky"].SkipDailyLimit)
	assert.Equal(t, 2, bySite["ourbits"].Created)
	assert.Equal(t, 4, bySite[""].SkipNotSelected, "不在辅种范围的站点")
	assert.Zero(t, bySite["hdsky"].IYUUCalls, "IYUU 请求不分站点")

	require.Len(t, st.IYUU, 2)
	assert.Equal(t, models.IYUUEndpointQuery, st.IYUU[0].Endpoint)
	assert.Equal(t, 4, st.IYUU[0].Calls)
	require.NotNil(t, st.LastIYUUAt)
	assert.True(t, st.LastIYUUAt.Equal(last))
	assert.Equal(t, "2026-10-20", st.FirstDay)

	// 自定义范围只算范围以内
	st, err = e.svc.Stats(ctx, StatsQuery{Granularity: "week", From: "2026-10-29", To: "2026-10-29"})
	require.NoError(t, err)
	require.Len(t, st.Series, 1)
	assert.Equal(t, "2026-10-29", st.Series[0].Date)
	assert.Equal(t, "2026-10-29", st.Series[0].End)
	assert.Equal(t, 1, st.Totals.Created)
	assert.Zero(t, st.Totals.Succeeded)

	empty := newEnv(t)
	st, err = empty.svc.Stats(ctx, StatsQuery{Granularity: "year"})
	require.NoError(t, err)
	assert.Len(t, st.Series, 5)
	assert.Empty(t, st.FirstDay)
	assert.Nil(t, st.LastIYUUAt)
	assert.Empty(t, st.Sites)
}

// 清除已结束的辅种任务时，任务结果抄进建成它的尝试行，统计照样算。
func TestClearFinishedJobsCopiesResults(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.enable(nil)
	good := e.candidate("61", map[string]int64{"a.mkv": 100, "sub/b.srt": 1})
	e.setResults(map[string]any{"sid": 1, "torrent_id": 61, "info_hash": good})
	res, err := e.svc.Run(ctx, RunOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created)
	var att models.ReseedAttempt
	require.NoError(t, e.db.Where("outcome = ?", models.ReseedAttemptCreated).First(&att).Error)
	done := e.now.Add(time.Hour)
	require.NoError(t, e.db.Model(&models.TorrentTransferJob{}).Where("id = ?", *att.JobID).
		Updates(map[string]any{"state": models.TransferDone, "finished_at": done}).Error)
	n, err := e.svc.ClearFinishedJobs(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	require.NoError(t, e.db.First(&att, att.ID).Error)
	assert.Equal(t, models.TransferDone, att.JobState)
	require.NotNil(t, att.JobFinishedAt)
	assert.True(t, att.JobFinishedAt.Equal(done))

	e.now = done
	st, err := e.svc.Stats(ctx, StatsQuery{Granularity: "day", Periods: 1})
	require.NoError(t, err)
	assert.Equal(t, 1, st.Totals.Created)
	assert.Equal(t, 1, st.Totals.Succeeded)
}

// 每一轮按站点累计下载以前就跳过的个数，结束时加进当天的统计。
func TestRunRecordsDailySkips(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.enable(nil)
	good := e.candidate("71", map[string]int64{"a.mkv": 100, "sub/b.srt": 1})
	e.setResults(
		map[string]any{"sid": 1, "torrent_id": 71, "info_hash": good},
		map[string]any{"sid": 2, "torrent_id": 9, "info_hash": "other"}, // ourbits 没有配置：不在辅种范围
		map[string]any{"sid": 1, "torrent_id": 72, "info_hash": e.srcHash},
	)
	_, err := e.svc.Run(ctx, RunOptions{})
	require.NoError(t, err)
	_, err = e.svc.Run(ctx, RunOptions{}) // 第二轮：71 已经尝试过
	require.NoError(t, err)
	var rows []models.ReseedDailySkip
	require.NoError(t, e.db.Order("site_name").Find(&rows).Error)
	require.Len(t, rows, 2)
	assert.Equal(t, "", rows[0].SiteName)
	assert.Equal(t, 2, rows[0].NotSelected)
	assert.Equal(t, "hdsky", rows[1].SiteName)
	assert.Equal(t, 2, rows[1].Present, "源种子自己：下载器里已经有，每一轮都算")
	assert.Equal(t, 1, rows[1].Tried)
	assert.Equal(t, e.now.Format(time.DateOnly), rows[1].Day)
}
