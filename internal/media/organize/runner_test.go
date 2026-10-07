package organize

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func (e *env) historyCount(hash string) int64 {
	var n int64
	require.NoError(e.t, e.db.Model(&models.MediaTransferHistory{}).Where("info_hash = ?", hash).Count(&n).Error)
	return n
}

// drain 把队列里的种子一个个整理完（后台循环没在跑时用）。
func (e *env) drain() int {
	n := 0
	for {
		select {
		case j := <-e.svc.jobs:
			e.svc.qmu.Lock()
			delete(e.svc.queued, jobKey(j.req.DownloaderID, j.dlName, j.req.Hash))
			e.svc.qmu.Unlock()
			_, _ = e.svc.runJob(e.ctx, j)
			n++
		default:
			return n
		}
	}
}

func completedEvent(t *testing.T, hash string) events.Event {
	t.Helper()
	b, err := json.Marshal(events.TorrentCompletedPayload{Title: hash, DownloaderName: "qb", TaskID: hash, InfoHash: hash})
	require.NoError(t, err)
	return events.Event{Type: events.EvtTorrentCompleted, Payload: b}
}

func TestOnEvent(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.oppenheimer()
	e.lastOfUs()

	e.svc.onEvent(e.ctx, completedEvent(t, oppHash))
	assert.Len(t, e.svc.jobs, 0, "没打开自动整理：不排队")

	e.settings(SettingsInput{AutoEnabled: true, MinVideoMB: 1, Categories: []string{"movies"}})
	e.svc.onEvent(e.ctx, events.Event{Type: events.EvtDiskLow})
	e.svc.onEvent(e.ctx, events.Event{Type: events.EvtTorrentCompleted, Payload: []byte(`{"title":"x"}`)})
	assert.Len(t, e.svc.jobs, 0, "别的事件、没有下载器的事件不排队")

	e.svc.onEvent(e.ctx, completedEvent(t, oppHash))
	e.svc.onEvent(e.ctx, completedEvent(t, oppHash))
	e.svc.onEvent(e.ctx, completedEvent(t, tlouHash))
	assert.Len(t, e.svc.jobs, 2, "同一个种子只排一次")
	assert.Equal(t, 2, e.drain())
	assert.Equal(t, int64(1), e.historyCount(oppHash))
	assert.Equal(t, models.MediaTriggerAuto, e.history()[0].Trigger)
	assert.Zero(t, e.historyCount(tlouHash), "分类不在范围里")
}

func TestCompletedEventThroughBus(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{AutoEnabled: true, MinVideoMB: 1})
	e.oppenheimer()
	e.svc.Start()
	t.Cleanup(e.svc.Stop)
	require.NoError(t, events.PublishWithPayload(events.EvtTorrentCompleted, events.TorrentCompletedPayload{
		Title: oppName, DownloaderName: "qb", TaskID: oppHash, InfoHash: oppHash,
	}))
	require.Eventually(t, func() bool { return e.historyCount(oppHash) == 1 }, 5*time.Second, 20*time.Millisecond)
}

func TestOrganizeQueuedWhenRunning(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.oppenheimer()
	e.svc.Start()
	e.svc.Start() // 重复调用无副作用
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created, "在跑时排进队列，等到结果")
	_, err = e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: "nope"})
	require.ErrorIs(t, err, ErrNotFound)
	_, err = e.svc.Organize(e.ctx, Request{})
	require.ErrorIs(t, err, ErrInvalid)
	e.svc.Stop()
	e.svc.Stop()
	assert.False(t, e.svc.isRunning())
}

func TestSweepCompletedPushedTorrents(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	set := e.settings(SettingsInput{AutoEnabled: true, MinVideoMB: 1})
	require.NotNil(t, set.AutoSince)
	e.oppenheimer()
	e.lastOfUs()
	pushed := true
	dlID := uint(1)
	before := set.AutoSince.Add(-time.Hour)
	after := set.AutoSince.Add(time.Minute)
	opp, tlou := oppHash, tlouHash
	require.NoError(t, e.db.Create(&[]models.TorrentInfo{
		{SiteName: "s", TorrentID: "1", TorrentHash: &opp, IsPushed: &pushed, IsCompleted: true, CompletedAt: &after, DownloaderID: &dlID, DownloaderName: "qb", DownloaderTaskID: oppHash},
		{SiteName: "s", TorrentID: "2", TorrentHash: &tlou, IsPushed: &pushed, IsCompleted: true, CompletedAt: &before, DownloaderID: &dlID, DownloaderName: "qb", DownloaderTaskID: tlouHash},
	}).Error)

	e.svc.sweep(e.ctx, set)
	assert.Equal(t, 1, e.drain(), "只补查打开自动整理以后完成的")
	assert.Equal(t, int64(1), e.historyCount(oppHash))
	assert.Zero(t, e.historyCount(tlouHash))

	e.svc.sweep(e.ctx, set)
	assert.Zero(t, e.drain(), "已经有整理记录的不再补查")

	// 下载器里找不到的进退避：下一轮补查不排它
	gone := "4444444444444444444444444444444444444444"
	require.NoError(t, e.db.Create(&models.TorrentInfo{SiteName: "s", TorrentID: "3", TorrentHash: &gone, IsPushed: &pushed, IsCompleted: true, CompletedAt: &after, DownloaderID: &dlID, DownloaderName: "qb", DownloaderTaskID: gone}).Error)
	e.svc.sweep(e.ctx, set)
	assert.Equal(t, 1, e.drain())
	e.svc.sweep(e.ctx, set)
	assert.Zero(t, e.drain(), "退避期内不排")
	e.advance(25 * time.Minute)
	e.svc.sweep(e.ctx, set)
	assert.Equal(t, 1, e.drain(), "退避过了再试")
}

func TestScanDownloaders(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	set := e.settings(SettingsInput{ScanEnabled: true, MinVideoMB: 1, Tags: []string{"keep"}, SavePaths: []string{"/downloads"}})
	e.oppenheimer()
	e.lastOfUs()
	e.addTorrent("5555555555555555555555555555555555555555", "Tagged.Show.S01E01.mkv", map[string]int{"Tagged.Show.S01E01.mkv": 2}, func(t *downloader.Torrent) { t.Tags = "a, Keep" })
	e.dl.mu.Lock()
	tr := e.dl.torrents[oppHash]
	tr.Tags = "keep"
	e.dl.torrents[oppHash] = tr
	e.dl.mu.Unlock()

	e.svc.scan(e.ctx, set)
	assert.Equal(t, 2, e.drain(), "只扫带 keep 标签的（不分大小写）")
	assert.Equal(t, int64(1), e.historyCount(oppHash))
	assert.Zero(t, e.historyCount(tlouHash))
	assert.Equal(t, models.MediaTriggerScan, e.history()[0].Trigger)

	e.svc.scan(e.ctx, set)
	assert.Zero(t, e.drain(), "有整理记录的不再扫")

	set.Downloaders = []uint{9}
	e.svc.scan(e.ctx, set)
	assert.Zero(t, e.drain(), "范围里没有这个下载器")

	e.dl.listErr = assert.AnError
	set.Downloaders = nil
	e.svc.scan(e.ctx, set)
	assert.Zero(t, e.drain())
}

func TestRetryDue(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.tmdb.setDown(true)
	e.oppenheimer()
	res, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	require.Contains(t, res.Plan.Problem, "识别失败")
	e.tmdb.setDown(false)

	e.svc.retryDue(e.ctx)
	assert.Zero(t, e.drain(), "还没到重试时间")
	e.advance(11 * time.Minute)
	e.svc.retryDue(e.ctx)
	assert.Equal(t, 1, e.drain())
	rows := e.history()
	require.Len(t, rows, 1)
	assert.Equal(t, models.MediaTransferDone, rows[0].Status)
	assert.Nil(t, rows[0].NextRetryAt)
	assert.Zero(t, rows[0].Attempts)
	assert.Equal(t, models.MediaTriggerManual, rows[0].Trigger, "沿用原来的触发方式")

	// 超过重试次数后不再自动重试
	e.tmdb.setDown(true)
	e.addTorrent("6666666666666666666666666666666666666666", "Other.2019.mkv", map[string]int{"Other.2019.mkv": 2}, nil)
	for range len(retryDelays) + 1 {
		_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: "6666666666666666666666666666666666666666"})
		require.NoError(t, err)
	}
	var row models.MediaTransferHistory
	require.NoError(t, e.db.Where("info_hash = ?", "6666666666666666666666666666666666666666").First(&row).Error)
	assert.Equal(t, len(retryDelays)+1, row.Attempts)
	assert.Nil(t, row.NextRetryAt)
}

func TestEnqueueDedupAndFull(t *testing.T) {
	e := newEnv(t)
	j := job{req: Request{DownloaderID: 1, Hash: "x"}, trigger: models.MediaTriggerScan}
	assert.True(t, e.svc.enqueue(j))
	assert.True(t, e.svc.enqueue(j), "已经在队列里的不重复放")
	assert.Len(t, e.svc.jobs, 1)
	for i := range jobQueueSize - 1 {
		require.True(t, e.svc.enqueue(job{req: Request{DownloaderID: 1, Hash: string(rune('a'+i%26)) + time.Duration(i).String()}}))
	}
	assert.False(t, e.svc.enqueue(job{req: Request{DownloaderID: 1, Hash: "overflow"}}), "队列满了")
}

func TestScopeMatch(t *testing.T) {
	tr := downloader.Torrent{Category: "Movies", Tags: "pt, keep", SavePath: "/data/downloads/movies"}
	assert.True(t, scope{}.match(1, tr), "空范围不限制")
	assert.True(t, scope{downloaders: []uint{1, 2}, categories: []string{"movies"}, tags: []string{"KEEP"}, savePaths: []string{"/data/downloads"}}.match(1, tr))
	assert.False(t, scope{downloaders: []uint{2}}.match(1, tr))
	assert.False(t, scope{categories: []string{"tv"}}.match(1, tr))
	assert.False(t, scope{tags: []string{"other"}}.match(1, tr))
	assert.False(t, scope{savePaths: []string{"/data/down"}}.match(1, tr), "按路径分段匹配")
}

// 别的下载器里同一个 hash 的整理记录不影响这个下载器的补查与扫描
func TestSweepAndScanPerDownloader(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	set := e.settings(SettingsInput{AutoEnabled: true, ScanEnabled: true, MinVideoMB: 1})
	e.oppenheimer()
	require.NoError(t, e.db.Create(&models.MediaTransferHistory{DownloaderID: 2, InfoHash: oppHash, SourcePath: "/other/downloader/file.mkv", Status: models.MediaTransferDone}).Error)
	pushed := true
	dlID := uint(1)
	after := set.AutoSince.Add(time.Minute)
	opp := oppHash
	require.NoError(t, e.db.Create(&models.TorrentInfo{SiteName: "s", TorrentID: "1", TorrentHash: &opp, IsPushed: &pushed, IsCompleted: true, CompletedAt: &after, DownloaderID: &dlID, DownloaderName: "qb", DownloaderTaskID: oppHash}).Error)
	e.svc.sweep(e.ctx, set)
	assert.Len(t, e.svc.jobs, 1, "补查只看这个下载器的记录")
	e.drain()
	require.NoError(t, e.db.Where("downloader_id = ? AND info_hash = ?", 1, oppHash).Delete(&models.MediaTransferHistory{}).Error)
	e.svc.scan(e.ctx, set)
	assert.Len(t, e.svc.jobs, 1, "扫描只看这个下载器的记录")
}

// 队列满了排不上时，失败记录的重试时间不会被清掉
func TestRetryDueKeepsScheduleWhenQueueFull(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{MinVideoMB: 1})
	e.tmdb.setDown(true)
	e.oppenheimer()
	_, err := e.svc.Organize(e.ctx, Request{DownloaderID: 1, Hash: oppHash})
	require.NoError(t, err)
	for i := range jobQueueSize {
		require.True(t, e.svc.enqueue(job{req: Request{DownloaderID: 9, Hash: time.Duration(i).String()}}))
	}
	e.advance(11 * time.Minute)
	e.svc.retryDue(e.ctx)
	var row models.MediaTransferHistory
	require.NoError(t, e.db.First(&row).Error)
	require.NotNil(t, row.NextRetryAt, "排不上时保留重试时间")
	assert.Equal(t, e.now.Add(tickInterval), row.NextRetryAt.UTC(), "下一拍再试")
}

// 自动整理失败后的重试按现在的设置：自动整理关了、种子不在范围里时不整理
func TestRetryFollowsCurrentAutomation(t *testing.T) {
	e := newEnv(t)
	e.defaultLibraries()
	e.settings(SettingsInput{AutoEnabled: true, MinVideoMB: 1})
	e.tmdb.setDown(true)
	e.oppenheimer()
	require.True(t, e.svc.enqueue(job{req: Request{DownloaderID: 1, Hash: oppHash}, trigger: models.MediaTriggerAuto}))
	e.drain()
	row := e.history()[0]
	require.Equal(t, models.MediaTransferFailed, row.Status)
	require.Equal(t, models.MediaTriggerAuto, row.Trigger)
	require.NotNil(t, row.NextRetryAt)
	e.tmdb.setDown(false)
	due := func() {
		t.Helper()
		require.NoError(t, e.db.Model(&models.MediaTransferHistory{}).Where("1 = 1").Update("next_retry_at", e.Now()).Error)
		e.svc.retryDue(e.ctx)
		e.drain()
	}

	e.settings(SettingsInput{MinVideoMB: 1})
	due()
	assert.Equal(t, models.MediaTransferFailed, e.history()[0].Status, "自动整理关掉以后不再重试")

	e.settings(SettingsInput{AutoEnabled: true, MinVideoMB: 1, Categories: []string{"tv"}})
	due()
	assert.Equal(t, models.MediaTransferFailed, e.history()[0].Status, "不在范围里不重试")

	e.settings(SettingsInput{AutoEnabled: true, MinVideoMB: 1, Categories: []string{"movies"}})
	due()
	assert.Equal(t, models.MediaTransferDone, e.history()[0].Status)
}

// 完成事件带了下载器名字，补查与扫描只有编号：排队去重与退避用同一个键
func TestJobKeyIgnoresNameWithID(t *testing.T) {
	e := newEnv(t)
	assert.Equal(t, jobKey(1, "", "ABC"), jobKey(1, "qb", "abc"))
	assert.NotEqual(t, jobKey(0, "qb", "abc"), jobKey(0, "", "abc"), "只有名字时按名字")
	ev := job{req: Request{DownloaderID: 1, Hash: "abc"}, dlName: "qb", trigger: models.MediaTriggerAuto}
	require.True(t, e.svc.enqueue(ev))
	require.True(t, e.svc.enqueue(job{req: Request{DownloaderID: 1, Hash: "abc"}, trigger: models.MediaTriggerAuto}))
	assert.Len(t, e.svc.jobs, 1, "补查不重复排完成事件已经排上的种子")
	e.svc.fail(ev)
	assert.True(t, e.svc.backedOff(1, "abc"), "完成事件失败后，补查与扫描看得到退避")
}
