package reseed

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/transfer"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// recordFixture 直接写一条记录（jobState 不为空时先建任务；"missing" 是指向一个不存在的任务）。
type recordFixture struct {
	key, state, jobState, site, name string
}

func (e *env) seedRecords(fs []recordFixture) map[string]uint {
	e.t.Helper()
	ids := map[string]uint{}
	at := e.now.Add(-time.Duration(len(fs)) * time.Minute)
	for i, f := range fs {
		rec := models.ReseedRecord{
			InfoHash: fmt.Sprintf("%040x", i+1), SiteName: f.site, TorrentID: f.key, Name: f.name,
			State: f.state, DownloaderID: 1, SourceHash: e.srcHash, CreatedAt: at, UpdatedAt: at,
		}
		switch f.jobState {
		case "":
		case "missing":
			gone := uint(9000 + i)
			rec.JobID = &gone
		default:
			job := models.TorrentTransferJob{Kind: models.JobKindReseed, TargetDownloaderID: 1, InfoHash: rec.InfoHash, State: f.jobState}
			require.NoError(e.t, e.db.Create(&job).Error)
			rec.JobID = &job.ID
		}
		require.NoError(e.t, e.db.Create(&rec).Error)
		ids[f.key] = rec.ID
		at = at.Add(time.Minute)
	}
	return ids
}

func keysOf(views []RecordView) []string {
	out := make([]string, 0, len(views))
	for _, v := range views {
		out = append(out, v.TorrentID)
	}
	sort.Strings(out)
	return out
}

// 结果分组：记录自己的状态说了算的直接用，建了任务的看任务现在的状态；SQL 的筛选与每行算出来的结果一致。
func TestRecordsFilter(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.seedRecords([]recordFixture{
		{"r01", models.ReseedFailed, "", "hdsky", "Alpha"},
		{"r02", models.ReseedGone, "", "ourbits", "Bravo"},
		{"r03", models.ReseedDone, "", "hdsky", "100%_done"},
		{"r04", models.ReseedRolledBack, "", "hdsky", "Delta"},
		{"r05", models.ReseedCanceled, "", "hdsky", "Echo"},
		{"r06", models.ReseedQueued, models.TransferPending, "hdsky", "Foxtrot"},
		{"r07", models.ReseedQueued, models.TransferChecking, "hdsky", "Golf"},
		{"r08", models.ReseedQueued, models.TransferDone, "hdsky", "Hotel"},
		{"r09", models.ReseedQueued, models.TransferFailed, "hdsky", "India"},
		{"r10", models.ReseedQueued, models.TransferRolledBack, "hdsky", "Juliet"},
		{"r11", models.ReseedQueued, models.TransferCanceled, "hdsky", "Kilo"},
		{"r12", models.ReseedQueued, "missing", "hdsky", "Lima"},
	})
	want := map[string][]string{
		RecordFailed:     {"r01", "r09"},
		RecordGone:       {"r02"},
		RecordDone:       {"r03", "r08"},
		RecordRolledBack: {"r04", "r10"},
		RecordCanceled:   {"r05", "r11"},
		RecordActive:     {"r06", "r07"},
		RecordMissing:    {"r12"},
	}
	all := e.records(RecordFilter{})
	require.Len(t, all, 12)
	assert.Equal(t, "r12", all[0].TorrentID, "最后一次尝试的在前面")
	resultOf := map[string]string{}
	for _, v := range all {
		resultOf[v.TorrentID] = v.Result
	}
	for result, keys := range want {
		got := e.records(RecordFilter{Result: result})
		assert.Equal(t, keys, keysOf(got), result)
		for _, k := range keys {
			assert.Equal(t, result, resultOf[k], k)
		}
	}
	assert.True(t, all[0].JobMissing, "任务已经不在了")
	byKey := map[string]RecordView{}
	for _, v := range all {
		byKey[v.TorrentID] = v
	}
	assert.Equal(t, models.TransferChecking, byKey["r07"].JobState)
	for k, retryable := range map[string]bool{"r01": true, "r02": true, "r03": false, "r04": true, "r05": true, "r06": false, "r08": false, "r09": true, "r11": true, "r12": true} {
		assert.Equal(t, retryable, RecordRetryable(byKey[k].Result), k)
	}

	// 站点、关键词（种子名不分大小写；info hash 大写也能找到；% 和 _ 不当通配符）
	assert.Equal(t, []string{"r02"}, keysOf(e.records(RecordFilter{Site: "ourbits"})))
	assert.Equal(t, []string{"r01"}, keysOf(e.records(RecordFilter{Query: "alp"})))
	assert.Equal(t, []string{"r03"}, keysOf(e.records(RecordFilter{Query: "0%_"})))
	assert.Equal(t, []string{"r03"}, keysOf(e.records(RecordFilter{Query: "%"})))
	assert.Equal(t, []string{"r12"}, keysOf(e.records(RecordFilter{Query: fmt.Sprintf("%040X", 12)})))
	assert.Equal(t, []string{"r09"}, keysOf(e.records(RecordFilter{Result: RecordFailed, Query: "ind"})))

	// 分页
	page, err := e.svc.Records(ctx, RecordFilter{}, 3, 5)
	require.NoError(t, err)
	assert.EqualValues(t, 12, page.Total)
	assert.Equal(t, []string{"r01", "r02"}, keysOf(page.Items))
	assert.Equal(t, []string{"hdsky", "ourbits"}, page.Sites)

	_, err = e.svc.Records(ctx, RecordFilter{Result: "nope"}, 1, 20)
	require.ErrorIs(t, err, ErrInvalid)
}

// 删除：任务还没结束的不删；判断与删除在同一条语句里。
func TestDeleteRecords(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	ids := e.seedRecords([]recordFixture{
		{"a", models.ReseedFailed, "", "hdsky", "A"},
		{"b", models.ReseedQueued, models.TransferAdding, "hdsky", "B"},
		{"c", models.ReseedQueued, models.TransferDone, "hdsky", "C"},
		{"d", models.ReseedQueued, "missing", "hdsky", "D"},
		{"e", models.ReseedDone, "", "hdsky", "E"},
	})
	got, err := e.svc.DeleteRecords(ctx, []uint{ids["a"], ids["b"], ids["c"], ids["d"], ids["e"], ids["e"], 999})
	require.NoError(t, err)
	assert.EqualValues(t, 4, got.Deleted)
	assert.EqualValues(t, 1, got.Active)
	assert.Equal(t, []string{"b"}, keysOf(e.records(RecordFilter{})))

	_, err = e.svc.DeleteRecords(ctx, nil)
	require.ErrorIs(t, err, ErrInvalid)
	many := make([]uint, MaxDeleteRecords+1)
	for i := range many {
		many[i] = uint(i + 1)
	}
	_, err = e.svc.DeleteRecords(ctx, many)
	require.ErrorIs(t, err, ErrInvalid)
}

// 取消：只有还在等待加入下载器的任务能取消；已经加入的不能取消。
func TestCancelRecordJob(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	ids := e.seedRecords([]recordFixture{
		{"pending", models.ReseedQueued, models.TransferPending, "hdsky", "P"},
		{"checking", models.ReseedQueued, models.TransferChecking, "hdsky", "C"},
		{"failed", models.ReseedFailed, "", "hdsky", "F"},
		{"missing", models.ReseedQueued, "missing", "hdsky", "M"},
	})
	require.NoError(t, e.svc.CancelRecordJob(ctx, ids["pending"]))
	v, err := e.svc.record(ctx, ids["pending"])
	require.NoError(t, err)
	assert.Equal(t, RecordCanceled, v.Result)
	assert.True(t, RecordRetryable(v.Result), "取消了的可以再重试")

	require.ErrorIs(t, e.svc.CancelRecordJob(ctx, ids["checking"]), transfer.ErrNotCancelable)
	require.ErrorIs(t, e.svc.CancelRecordJob(ctx, ids["failed"]), ErrNoPendingJob)
	require.ErrorIs(t, e.svc.CancelRecordJob(ctx, ids["missing"]), ErrNoPendingJob)
	require.ErrorIs(t, e.svc.CancelRecordJob(ctx, ids["pending"]), ErrNoPendingJob, "已经取消了")
	require.ErrorIs(t, e.svc.CancelRecordJob(ctx, 999), ErrRecordNotFound)
}

// 重试：不查 IYUU，在记录的下载器里找原种子；名额按尝试行算（删除记录不退）；原种子不在、目标里已经有、名额用完时跳过。
func TestRetryRecords(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.enable(func(u *SettingsUpdate) { u.MaxPerSitePerDay = 4 })
	good := e.candidate("21", map[string]int64{"a.mkv": 100, "sub/b.srt": 1})
	data := e.site.data["21"]
	delete(e.site.data, "21") // 第一次下载不到
	bad := e.candidate("22", map[string]int64{"a.mkv": 100})
	e.setResults(map[string]any{"sid": 1, "torrent_id": 21, "info_hash": good}, map[string]any{"sid": 1, "torrent_id": 22, "info_hash": bad})
	res, err := e.svc.Run(ctx, RunOptions{})
	require.NoError(t, err)
	require.Equal(t, 2, res.Failed)
	ids := map[string]uint{}
	for _, v := range e.records(RecordFilter{Result: RecordFailed}) {
		ids[v.TorrentID] = v.ID
	}
	require.Len(t, ids, 2)
	queries := e.iyuu.queries

	// 种子又能下载了：重试建成任务，尝试记在重试轮次名下
	e.site.data["21"] = data
	run, err := e.svc.StartRun(ctx, models.ReseedTriggerRetry, "", nil)
	require.NoError(t, err)
	res, err = e.svc.Retry(ctx, RunOptions{RunID: run.ID}, []uint{ids["21"], ids["21"]})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Hashes, "重复的编号只算一次")
	assert.Equal(t, 1, res.Created)
	assert.Contains(t, res.Summary(), "重试 1 条记录，加入 1 个")
	assert.Equal(t, queries, e.iyuu.queries, "重试不查 IYUU")
	v, err := e.svc.record(ctx, ids["21"])
	require.NoError(t, err)
	assert.Equal(t, RecordActive, v.Result)
	var atts []models.ReseedAttempt
	require.NoError(t, e.db.Where("run_id = ?", run.ID).Find(&atts).Error)
	require.Len(t, atts, 1)
	assert.Equal(t, models.ReseedAttemptCreated, atts[0].Outcome)

	// 任务还没结束：不能重试
	res, err = e.svc.Retry(ctx, RunOptions{}, []uint{ids["21"]})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Skipped[SkipNotRetryable])
	require.ErrorIs(t, e.svc.CheckRetry(ctx, []uint{ids["21"]}), ErrNothingToRetry)
	require.NoError(t, e.svc.CheckRetry(ctx, []uint{ids["21"], ids["22"]}))

	// 文件对不上的也能点名重试（又失败一次）；今天的名额（4 次）用完以后跳过
	res, err = e.svc.Retry(ctx, RunOptions{}, []uint{ids["22"]})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Failed)
	res, err = e.svc.Retry(ctx, RunOptions{}, []uint{ids["22"]})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Skipped[SkipDailyLimit])
	assert.Equal(t, 2, e.site.downloads["22"])

	// 第二天：原种子不在下载器里、目标里已经有这个种子时跳过
	e.now = e.now.Add(24 * time.Hour)
	saved := e.dl.torrents
	e.dl.torrents = saved[1:]
	res, err = e.svc.Retry(ctx, RunOptions{}, []uint{ids["22"]})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Skipped[SkipNoSource])
	e.dl.torrents = append(append([]downloader.Torrent{}, saved...), downloader.Torrent{ID: "x", InfoHash: bad, Progress: 1})
	res, err = e.svc.Retry(ctx, RunOptions{}, []uint{ids["22"]})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Skipped[SkipExists])
	e.dl.torrents = saved
	assert.Equal(t, 2, e.site.downloads["22"], "跳过的没有下载")

	// 删除记录不退名额：尝试行不动
	before, err := e.svc.todayCounts(ctx)
	require.NoError(t, err)
	del, err := e.svc.DeleteRecords(ctx, []uint{ids["22"]})
	require.NoError(t, err)
	assert.EqualValues(t, 1, del.Deleted)
	after, err := e.svc.todayCounts(ctx)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	res, err = e.svc.Retry(ctx, RunOptions{}, []uint{ids["22"]})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Skipped[SkipRecordMissing])

	// 下载器不可用
	missingDL := e.seedRecords([]recordFixture{{"23", models.ReseedFailed, "", "hdsky", "N"}})
	require.NoError(t, e.db.Model(&models.ReseedRecord{}).Where("id = ?", missingDL["23"]).Update("downloader_id", 7).Error)
	res, err = e.svc.Retry(ctx, RunOptions{}, []uint{missingDL["23"]})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Skipped[SkipNoDownloader])
	require.NotEmpty(t, res.Errors)

	require.ErrorIs(t, e.svc.CheckRetry(ctx, nil), ErrInvalid)
	many := make([]uint, MaxRetryRecords+1)
	for i := range many {
		many[i] = uint(i + 1)
	}
	require.ErrorIs(t, e.svc.CheckRetry(ctx, many), ErrInvalid)
}

// 重试轮次在每条记录之前检查暂停与停止：停止时正在下载的那一个记为被打断（不改记录），剩下的不再试。
func TestRetryStops(t *testing.T) {
	e := newEnv(t)
	ids := e.seedRecords([]recordFixture{
		{"31", models.ReseedFailed, "", "hdsky", "A"},
		{"32", models.ReseedFailed, "", "hdsky", "B"},
	})
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	e.site.onDownload = func(context.Context) error {
		cancel(ErrRunStopped)
		return context.Canceled
	}
	res, err := e.svc.Retry(ctx, RunOptions{}, []uint{ids["31"], ids["32"]})
	require.NoError(t, err)
	assert.Equal(t, EndStopped, res.End)
	assert.Equal(t, 1, res.Interrupted)
	assert.Equal(t, 1, e.site.downloads["31"])
	assert.Zero(t, e.site.downloads["32"])
	v, err := e.svc.record(context.Background(), ids["31"])
	require.NoError(t, err)
	assert.Equal(t, RecordFailed, v.Result, "被打断的不改记录")
}
