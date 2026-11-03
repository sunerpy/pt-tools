package organize

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func (e *env) putRows(rows ...models.MediaTransferHistory) []models.MediaTransferHistory {
	e.t.Helper()
	for i := range rows {
		if rows[i].SourcePath == "" {
			var n int64
			require.NoError(e.t, e.db.Model(&models.MediaTransferHistory{}).Count(&n).Error)
			rows[i].SourcePath = fmt.Sprintf("/downloads/row-%d", n)
		}
		require.NoError(e.t, e.db.Create(&rows[i]).Error)
	}
	return rows
}

func historyIDs(p HistoryPage) []uint {
	var out []uint
	for _, it := range p.Items {
		out = append(out, it.ID)
	}
	return out
}

// 按分类筛选只看失败与跳过的记录；「其他」也包括升级前没有分类的
func TestHistoryKindFilter(t *testing.T) {
	e := newEnv(t)
	rows := e.putRows(
		models.MediaTransferHistory{TorrentName: "done", Status: models.MediaTransferDone},
		models.MediaTransferHistory{TorrentName: "recognize", Status: models.MediaTransferFailed, FailKind: models.MediaFailRecognize},
		models.MediaTransferHistory{TorrentName: "exists", Status: models.MediaTransferSkipped, FailKind: models.MediaFailExists},
		models.MediaTransferHistory{TorrentName: "old", Status: models.MediaTransferFailed},
		models.MediaTransferHistory{TorrentName: "other", Status: models.MediaTransferSkipped, FailKind: models.MediaFailOther},
		models.MediaTransferHistory{TorrentName: "removed", Status: models.MediaTransferRemoved, FailKind: models.MediaFailOther},
		models.MediaTransferHistory{TorrentName: "removed-recognize", Status: models.MediaTransferRemoved, FailKind: models.MediaFailRecognize},
	)
	page, err := e.svc.History(e.ctx, HistoryQuery{Kind: models.MediaFailRecognize})
	require.NoError(t, err)
	assert.Equal(t, []uint{rows[1].ID}, historyIDs(page), "只列失败与跳过的：已删除的带着分类也不列")
	assert.Equal(t, models.MediaFailRecognize, page.Items[0].FailKind)

	page, err = e.svc.History(e.ctx, HistoryQuery{Kind: models.MediaFailOther})
	require.NoError(t, err)
	assert.ElementsMatch(t, []uint{rows[3].ID, rows[4].ID}, historyIDs(page), "其他包括没有分类的失败、跳过记录，不含已整理与已删除的")
	assert.Equal(t, int64(2), page.Total)

	page, err = e.svc.History(e.ctx, HistoryQuery{Status: models.MediaTransferSkipped, Kind: models.MediaFailOther})
	require.NoError(t, err)
	assert.Equal(t, []uint{rows[4].ID}, historyIDs(page))
	page, err = e.svc.History(e.ctx, HistoryQuery{Status: models.MediaTransferDone, Kind: models.MediaFailExists})
	require.NoError(t, err)
	assert.Empty(t, page.Items)

	_, err = e.svc.History(e.ctx, HistoryQuery{Kind: "nope"})
	require.ErrorIs(t, err, ErrInvalid)
}

// 按条目分组：识别出的按类型、编号与季，没识别出的按种子名；按组里最近的时刻倒序（不按文本比较时区不同的时间）；
// 组的条件列出这一组的记录
func TestHistoryGroups(t *testing.T) {
	e := newEnv(t)
	cst := time.FixedZone("CST", 8*3600)
	at := func(h int, loc *time.Location) time.Time { return time.Date(2026, 11, 3, h, 0, 0, 0, loc) }
	rows := e.putRows(
		models.MediaTransferHistory{
			TorrentName: "TLOU.S01", MediaType: tmdb.KindTV, TMDBID: 100088, Season: 1, Episode: 1, Title: "最后生还者", Year: 2023,
			Status: models.MediaTransferDone, UpdatedAt: at(9, cst),
		},
		models.MediaTransferHistory{
			TorrentName: "TLOU.S01", MediaType: tmdb.KindTV, TMDBID: 100088, Season: 1, Episode: 2, Title: "最后生还者", Year: 2023,
			Status: models.MediaTransferFailed, FailKind: models.MediaFailExists, UpdatedAt: at(10, cst),
		},
		// 第二季、另一个种子：另一组
		models.MediaTransferHistory{
			TorrentName: "TLOU.S02", MediaType: tmdb.KindTV, TMDBID: 100088, Season: 2, Episode: 1, Title: "最后生还者", Year: 2023,
			Status: models.MediaTransferDone, UpdatedAt: at(1, time.UTC),
		},
		// 文本上「2026-11-03 05:00:00+00:00」小于「2026-11-03 10:00:00+08:00」，时刻上更晚
		models.MediaTransferHistory{
			TorrentName: "Oppenheimer", MediaType: tmdb.KindMovie, TMDBID: 872585, Title: "奥本海默", Year: 2023,
			Status: models.MediaTransferDone, UpdatedAt: at(5, time.UTC),
		},
		models.MediaTransferHistory{TorrentName: "Foo.2020", Status: models.MediaTransferFailed, FailKind: models.MediaFailRecognize, UpdatedAt: at(3, cst)},
		models.MediaTransferHistory{TorrentName: "Foo.2020", Status: models.MediaTransferSkipped, UpdatedAt: at(4, cst)},
		models.MediaTransferHistory{TorrentName: "Bar.2021", Status: models.MediaTransferRemoved, UpdatedAt: at(0, time.UTC)},
	)

	page, err := e.svc.HistoryGroups(e.ctx, HistoryQuery{TMDBID: 1, Torrent: "x"})
	require.NoError(t, err, "组的条件不看")
	assert.Equal(t, int64(5), page.Total)
	require.Len(t, page.Groups, 5)
	var order []string
	for _, g := range page.Groups {
		order = append(order, fmt.Sprintf("%s/%d/%d/%s", g.MediaType, g.TMDBID, g.Season, g.Torrent))
	}
	// 按时刻：奥本海默 05:00Z、第一季 02:00Z、第二季 01:00Z、Bar 00:00Z、Foo 前一天 20:00Z（按文本比较会是第一季、奥本海默、Foo…）
	assert.Equal(t, []string{"movie/872585/0/", "tv/100088/1/", "tv/100088/2/", "/0/0/Bar.2021", "/0/0/Foo.2020"}, order)
	tlou := page.Groups[1]
	assert.Equal(t, HistoryGroup{
		MediaType: tmdb.KindTV, TMDBID: 100088, Season: 1, Title: "最后生还者", Year: 2023, Total: 2, Done: 1, Failed: 1,
		LatestAt: tlou.LatestAt,
	}, tlou)
	assert.True(t, tlou.LatestAt.Equal(at(10, cst)), tlou.LatestAt)
	foo := page.Groups[4]
	assert.Equal(t, 2, foo.Total)
	assert.Equal(t, 1, foo.Failed)
	assert.Equal(t, 1, foo.Skipped)
	assert.Empty(t, foo.Title)
	assert.Equal(t, 1, page.Groups[3].Removed)

	// 分页与筛选
	page, err = e.svc.HistoryGroups(e.ctx, HistoryQuery{Limit: 2, Offset: 1})
	require.NoError(t, err)
	assert.Equal(t, int64(5), page.Total)
	require.Len(t, page.Groups, 2)
	assert.Equal(t, 1, page.Groups[0].Season)
	page, err = e.svc.HistoryGroups(e.ctx, HistoryQuery{Status: models.MediaTransferFailed})
	require.NoError(t, err)
	require.Len(t, page.Groups, 2)
	assert.Equal(t, 1, page.Groups[0].Total, "只数筛选出的记录")
	assert.Equal(t, "Foo.2020", page.Groups[1].Torrent)
	page, err = e.svc.HistoryGroups(e.ctx, HistoryQuery{Kind: models.MediaFailOther})
	require.NoError(t, err)
	require.Len(t, page.Groups, 1)
	assert.Equal(t, "Foo.2020", page.Groups[0].Torrent)
	_, err = e.svc.HistoryGroups(e.ctx, HistoryQuery{Status: "bad"})
	require.ErrorIs(t, err, ErrInvalid)

	// 一组的记录
	list, err := e.svc.History(e.ctx, HistoryQuery{MediaType: tmdb.KindTV, TMDBID: 100088, Season: 1})
	require.NoError(t, err)
	assert.ElementsMatch(t, []uint{rows[0].ID, rows[1].ID}, historyIDs(list))
	list, err = e.svc.History(e.ctx, HistoryQuery{MediaType: tmdb.KindMovie, TMDBID: 872585})
	require.NoError(t, err)
	assert.Equal(t, []uint{rows[3].ID}, historyIDs(list))
	list, err = e.svc.History(e.ctx, HistoryQuery{Torrent: "Foo.2020"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []uint{rows[4].ID, rows[5].ID}, historyIDs(list))
	list, err = e.svc.History(e.ctx, HistoryQuery{Torrent: "TLOU.S01"})
	require.NoError(t, err)
	assert.Empty(t, list.Items, "按种子名的一组只是没识别出条目的：认出的按条目分组")
	for _, bad := range []HistoryQuery{{TMDBID: 100088}, {MediaType: "music", TMDBID: 1}, {TMDBID: -1}, {Season: -1}} {
		_, err = e.svc.History(e.ctx, bad)
		require.ErrorIs(t, err, ErrInvalid, bad)
	}
}

func batchMessages(r *BatchResult) map[uint]BatchItem {
	out := map[uint]BatchItem{}
	for _, it := range r.Items {
		out[it.ID] = it
	}
	return out
}

// 批量重试：同一个种子的几条记录只整理一次；记录不在的逐条写明；后台没在跑时直接整理
func TestBatchRetry(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()
	e.lastOfUs()
	e.tmdb.setDown(true)
	for _, h := range []string{oppHash, tlouHash} {
		_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: h})
		require.NoError(t, err)
	}
	rows := e.history()
	require.Len(t, rows, 3, "剧集的两集各一条")
	e.tmdb.setDown(false)

	var ids []uint
	for _, r := range rows {
		require.Equal(t, models.MediaTransferFailed, r.Status)
		ids = append(ids, r.ID)
	}
	res, err := e.svc.Batch(e.ctx, append(ids, 9999, ids[0], 0), BatchRetry)
	require.NoError(t, err)
	require.Len(t, res.Items, 4, "重复与 0 的编号不算")
	assert.Equal(t, 3, res.OK)
	assert.Equal(t, 1, res.Failed)
	got := batchMessages(res)
	assert.Equal(t, "新入库 1 个，已在库里 0 个，跳过 0 个，失败 0 个", got[rows[0].ID].Message)
	assert.Equal(t, "新入库 2 个，已在库里 0 个，跳过 0 个，失败 0 个", got[rows[1].ID].Message)
	assert.Equal(t, fmt.Sprintf("和记录 %d 是同一个种子（或同一项），一起重试", rows[1].ID), got[rows[2].ID].Message)
	assert.False(t, got[9999].OK)
	assert.Equal(t, ErrNotFound.Error(), got[9999].Message)
	for _, r := range e.history() {
		assert.Equal(t, models.MediaTransferDone, r.Status, r.SourcePath)
		assert.Empty(t, r.FailKind)
	}

	// 重试以后还是整理不了：写明原因
	_, err = e.svc.SaveLibrary(e.ctx, e.libraryID(models.MediaKindMovie), LibraryInput{Name: "电影", Kind: models.MediaKindMovie, Path: e.movies, Enabled: false})
	require.NoError(t, err)
	res, err = e.svc.Batch(e.ctx, []uint{rows[0].ID}, BatchRetry)
	require.NoError(t, err)
	assert.True(t, res.Items[0].OK, "重试做了，整理不了的原因写在说明里")
	assert.Contains(t, res.Items[0].Message, "指定的媒体库不存在、没有启用", "手动指定过条目的沿用那个条目与媒体库")

	for _, bad := range [][]uint{nil, {0}, make([]uint, maxBatch+1)} {
		if len(bad) > maxBatch {
			for i := range bad {
				bad[i] = uint(i + 1)
			}
		}
		_, err = e.svc.Batch(e.ctx, bad, BatchRetry)
		require.ErrorIs(t, err, ErrInvalid)
	}
	_, err = e.svc.Batch(e.ctx, ids, "explode")
	require.ErrorIs(t, err, ErrInvalid)
}

func (e *env) libraryID(kind string) uint {
	e.t.Helper()
	var lib models.MediaLibrary
	require.NoError(e.t, e.db.Where("kind = ?", kind).First(&lib).Error)
	return lib.ID
}

// 后台在跑时排进队列就回，整理在后台做完；记录的种子不对时写明原因
func TestBatchRetryQueued(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()
	e.tmdb.setDown(true)
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	e.tmdb.setDown(false)
	bad := e.putRows(models.MediaTransferHistory{
		TorrentName: "x", TMDBID: 5, MediaType: "music", Trigger: models.MediaTriggerManual,
		DownloaderID: 1, InfoHash: "cc", Status: models.MediaTransferFailed,
	})
	e.svc.Start()
	t.Cleanup(e.svc.Stop)
	id := e.history()[0].ID
	res, err := e.svc.Batch(e.ctx, []uint{id, bad[0].ID}, BatchRetry)
	require.NoError(t, err)
	got := batchMessages(res)
	assert.Equal(t, BatchItem{ID: id, OK: true, Message: "排进了整理队列"}, got[id])
	assert.False(t, got[bad[0].ID].OK)
	assert.Contains(t, got[bad[0].ID].Message, "指定条目时要写明类型")
	require.Eventually(t, func() bool { return e.row(id).Status == models.MediaTransferDone }, 5*time.Second, 20*time.Millisecond)
}

func TestBatchQueuedNotes(t *testing.T) {
	assert.Equal(t, BatchItem{ID: 1, Message: "整理队列已满，稍后再试"}, queued(1, queueFull))
	assert.Equal(t, BatchItem{ID: 2, OK: true, Message: "已经在整理队列里"}, queued(2, queueDup))
	assert.Equal(t, BatchItem{ID: 3, OK: true, Message: "排进了整理队列"}, queued(3, queueAdded))
	assert.Equal(t, "没有可整理的视频文件", resultNote(&Result{Plan: &Plan{Problem: "没有可整理的视频文件"}}))
}

// 批量删除：只删记录、连文件删；移动整理的不连文件删，写明原因
func TestBatchDelete(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	done := e.history()[0]
	require.True(t, exists(done.TargetPath))
	moved := e.putRows(models.MediaTransferHistory{
		TorrentName: "m", Status: models.MediaTransferDone, Mode: models.MediaModeMove,
		TargetPath: filepath.Join(e.movies, "m.mkv"), TargetFileID: "1:2",
	})[0]
	failed := e.putRows(models.MediaTransferHistory{TorrentName: "f", Status: models.MediaTransferFailed})[0]

	res, err := e.svc.Batch(e.ctx, []uint{done.ID, moved.ID, failed.ID}, BatchDeleteFiles)
	require.NoError(t, err)
	got := batchMessages(res)
	assert.True(t, got[done.ID].OK)
	assert.False(t, exists(done.TargetPath), "库里的文件一起删")
	assert.True(t, exists(filepath.Join(e.dlDir, oppName, oppName+".mkv")), "下载目录里的不动")
	assert.False(t, got[moved.ID].OK)
	assert.Contains(t, got[moved.ID].Message, "移动整理的文件是唯一的一份")
	assert.False(t, strings.HasPrefix(got[moved.ID].Message, ErrInvalid.Error()), "不带「参数无效」")
	assert.True(t, got[failed.ID].OK)
	assert.Equal(t, []uint{moved.ID}, rowIDs(e.history()))

	res, err = e.svc.Batch(e.ctx, []uint{moved.ID}, BatchDelete)
	require.NoError(t, err)
	assert.Equal(t, 1, res.OK)
	assert.Empty(t, e.history())
}

func rowIDs(rows []models.MediaTransferHistory) []uint {
	var out []uint
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

// 连文件删时留着的文件（被换过的）写在结果里
func TestBatchDeleteKept(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	done := e.history()[0]
	require.NoError(t, os.Remove(done.TargetPath))
	require.NoError(t, os.WriteFile(done.TargetPath, []byte("replaced"), 0o644))
	res, err := e.svc.Batch(e.ctx, []uint{done.ID}, BatchDeleteFiles)
	require.NoError(t, err)
	assert.True(t, res.Items[0].OK)
	assert.Contains(t, res.Items[0].Message, "这些文件留着：")
	assert.True(t, exists(done.TargetPath))
}

// 批量重新刮削：同一个库里的同一部只刮一次；没整理好的写明原因
func TestBatchRescrape(t *testing.T) {
	e := newEnv(t)
	e.settings(SettingsInput{MinVideoMB: 1})
	e.library(LibraryInput{Name: "剧集", Kind: models.MediaKindTV, Path: e.tv, Scrape: true, ScrapeItems: []string{"nfo"}})
	e.lastOfUs()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: tlouHash})
	require.NoError(t, err)
	rows := e.history()
	require.Len(t, rows, 2)
	failed := e.putRows(models.MediaTransferHistory{TorrentName: "f", Status: models.MediaTransferFailed})[0]

	res, err := e.svc.Batch(e.ctx, []uint{rows[0].ID, rows[1].ID, failed.ID}, BatchRescrape)
	require.NoError(t, err)
	got := batchMessages(res)
	assert.True(t, got[rows[0].ID].OK)
	assert.Contains(t, got[rows[0].ID].Message, "刮削了 2 条记录")
	assert.Equal(t, fmt.Sprintf("和记录 %d 是同一部，一起重新刮削", rows[0].ID), got[rows[1].ID].Message)
	assert.False(t, got[failed.ID].OK)
	assert.Equal(t, "只有整理好的记录能重新刮削", got[failed.ID].Message)

	// 后台在跑时排进队列
	e.svc.Start()
	t.Cleanup(e.svc.Stop)
	res, err = e.svc.Batch(e.ctx, []uint{rows[1].ID}, BatchRescrape)
	require.NoError(t, err)
	assert.Equal(t, "排进了整理队列", res.Items[0].Message)
}

// 配好路径映射以后重试：之前按下载器的路径记下的「路径映射」失败记录删掉，不留在失败里；
// 只是预览不删，别的种子的、记着库里文件的、这次还是找不到文件的都不删
func TestOrganizeDropsStalePathRows(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.addTorrent(oppHash, oppName, map[string]int{oppName + "/" + oppName + ".mkv": 2}, func(t *downloader.Torrent) {
		t.SavePath, t.ContentPath = "/elsewhere", "/elsewhere/"+oppName
	})
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	require.Equal(t, models.MediaFailPath, res.Plan.FailKind)
	var stale models.MediaTransferHistory
	require.NoError(t, e.db.Where("info_hash = ?", oppHash).First(&stale).Error)
	require.Equal(t, models.MediaFailPath, stale.FailKind)
	require.True(t, strings.HasPrefix(stale.SourcePath, "/elsewhere/"), stale.SourcePath)

	// 这次还是找不到文件：不删
	_, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	require.NoError(t, e.db.First(&models.MediaTransferHistory{}, stale.ID).Error)

	keep := []models.MediaTransferHistory{
		// 同一个种子、记着库里的文件
		{
			DownloaderID: 1, InfoHash: oppHash, TorrentName: oppName, SourcePath: "/elsewhere/old/a.mkv", TargetPath: "/lib/a.mkv",
			Status: models.MediaTransferFailed, FailKind: models.MediaFailPath, CreatedAt: e.now, UpdatedAt: e.now,
		},
		// 别的种子
		{
			DownloaderID: 1, InfoHash: tlouHash, TorrentName: tlouName, SourcePath: "/elsewhere/tlou/b.mkv",
			Status: models.MediaTransferFailed, FailKind: models.MediaFailPath, CreatedAt: e.now, UpdatedAt: e.now,
		},
		// 同一个种子、别的分类
		{
			DownloaderID: 1, InfoHash: oppHash, TorrentName: oppName, SourcePath: "/elsewhere/old/c.mkv",
			Status: models.MediaTransferFailed, FailKind: models.MediaFailOther, CreatedAt: e.now, UpdatedAt: e.now,
		},
	}
	require.NoError(t, e.db.Create(&keep).Error)

	// 映射配错了：这次还是找不到文件（源路径换了），旧的也不删
	pm, err := e.svc.SavePathMap(e.ctx, 0, PathMapInput{DownloaderID: 1, DownloaderPrefix: "/elsewhere", LocalPrefix: filepath.Join(e.root, "wrong")})
	require.NoError(t, err)
	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	require.Equal(t, models.MediaFailPath, res.Plan.FailKind)
	require.NoError(t, e.db.First(&models.MediaTransferHistory{}, stale.ID).Error, "这次也是路径映射的问题")

	_, err = e.svc.SavePathMap(e.ctx, pm.ID, PathMapInput{DownloaderID: 1, DownloaderPrefix: "/elsewhere", LocalPrefix: e.dlDir})
	require.NoError(t, err)
	_, err = e.svc.Preview(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	require.NoError(t, e.db.First(&models.MediaTransferHistory{}, stale.ID).Error, "只是预览不删")

	res, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	require.Equal(t, 1, res.Created, res.Plan.Problem)
	require.ErrorIs(t, e.db.First(&models.MediaTransferHistory{}, stale.ID).Error, gorm.ErrRecordNotFound, "旧的失败记录删了")
	for _, k := range keep {
		require.NoError(t, e.db.First(&models.MediaTransferHistory{}, k.ID).Error, k.SourcePath)
	}
	var done []models.MediaTransferHistory
	require.NoError(t, e.db.Where("info_hash = ? AND status = ?", oppHash, models.MediaTransferDone).Find(&done).Error)
	require.Len(t, done, 1)
	assert.Equal(t, filepath.Join(e.dlDir, oppName, oppName+".mkv"), done[0].SourcePath)
}
