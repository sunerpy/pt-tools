package scheduler

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 免费到期处理和进度更新用的是 DownloaderManager 的共享实例，用完不能 Close：
// 关掉会断开 RSS 推送等其他调用方正在用的会话，并让下一次获取走重连。
func TestFreeEndMonitor_DoesNotCloseSharedDownloader(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.torrentByID["task-1"] = downloader.Torrent{ID: "task-1", Progress: 0.3, State: downloader.TorrentDownloading, TotalSize: 100}
	m, db := newFreeEndMonitorWithFake(t, fake)

	past := time.Now().Add(-time.Minute)
	pushed := true
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "1", Title: "T", PauseOnFreeEnd: true, IsPushed: &pushed,
		FreeEndTime: &past, DownloaderTaskID: "task-1", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	m.updateAllMonitoredProgress()
	m.updateAllPushedTasksProgress()
	m.handleFreeEndedTorrent(tor)

	assert.Equal(t, []string{"task-1"}, fake.pausedIDs, "免费到期的种子被暂停")
	assert.Zero(t, fake.closeCount, "共享实例不能被关闭")
}

// 独立定时器和周期巡检同时拿到同一个到期种子时，只处理一次。
func TestFreeEndMonitor_ConcurrentHandlingProcessesOnce(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.torrentByID["task-c"] = downloader.Torrent{ID: "task-c", Progress: 0.5, State: downloader.TorrentDownloading, TotalSize: 100}
	m, db := newFreeEndMonitorWithFake(t, fake)

	past := time.Now().Add(-time.Minute)
	tor := models.TorrentInfo{
		SiteName: "s", TorrentID: "c", Title: "C", PauseOnFreeEnd: true,
		FreeEndTime: &past, DownloaderTaskID: "task-c", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&tor).Error)

	// 先占住处理权，模拟另一个协程正在处理
	require.True(t, m.beginProcessing(tor.ID))
	m.handleFreeEndedTorrent(tor)
	assert.Empty(t, fake.pausedIDs, "正在处理中的种子不会被第二次处理")
	m.endProcessing(tor.ID)

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() { m.handleFreeEndedTorrent(tor) })
	}
	wg.Wait()
	assert.Len(t, fake.pausedIDs, 1, "并发到达也只暂停一次")
}

// 进度更新按上次检查时间轮转：任务多于一批时，第二轮处理的是第一轮没轮到的。
func TestUpdateAllPushedTasksProgress_RotatesBatches(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)

	pushed := true
	total := progressUpdateBatchSize + 10
	for i := range total {
		id := fmt.Sprintf("task-%d", i)
		fake.torrentByID[id] = downloader.Torrent{ID: id, Progress: 0.2, State: downloader.TorrentDownloading, TotalSize: 100}
		require.NoError(t, db.DB.Create(&models.TorrentInfo{
			SiteName: "s", TorrentID: fmt.Sprintf("r%d", i), Title: id, IsPushed: &pushed,
			DownloaderTaskID: id, DownloaderName: "qb1",
		}).Error)
	}

	m.updateAllPushedTasksProgress()
	m.updateAllPushedTasksProgress()

	var unchecked int64
	require.NoError(t, db.DB.Model(&models.TorrentInfo{}).Where("check_count = 0").Count(&unchecked).Error)
	assert.Zero(t, unchecked, "两轮之后每个任务都至少更新过一次")
}

// 获取信息失败的任务也排到队尾，持续失败的任务不会一直占着每批最前面的名额。
func TestUpdateAllPushedTasksProgress_FailingRowsRotateToo(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)

	pushed := true
	// 一批全是下载器名缺失的坏记录，排在前面
	for i := range progressUpdateBatchSize {
		require.NoError(t, db.DB.Create(&models.TorrentInfo{
			SiteName: "s", TorrentID: fmt.Sprintf("bad%d", i), Title: "bad", IsPushed: &pushed,
			DownloaderTaskID: fmt.Sprintf("bad-%d", i), DownloaderName: "",
		}).Error)
	}
	fake.torrentByID["good"] = downloader.Torrent{ID: "good", Progress: 0.2, State: downloader.TorrentDownloading, TotalSize: 100}
	good := models.TorrentInfo{SiteName: "s", TorrentID: "good", Title: "good", IsPushed: &pushed, DownloaderTaskID: "good", DownloaderName: "qb1"}
	require.NoError(t, db.DB.Create(&good).Error)

	m.updateAllPushedTasksProgress()
	m.updateAllPushedTasksProgress()

	var got models.TorrentInfo
	require.NoError(t, db.DB.First(&got, good.ID).Error)
	assert.Positive(t, got.CheckCount, "坏记录轮到队尾后，正常任务能被更新")
}

// 补预约时排除已在内存里预约的种子：前一批都已预约，也能补上后面的。
func TestRescheduleMissingFutureTorrents_ReachesBeyondFirstBatch(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)

	future := time.Now().Add(2 * time.Hour)
	total := progressUpdateBatchSize + 5
	ids := make([]uint, 0, total)
	for i := range total {
		tor := models.TorrentInfo{
			SiteName: "s", TorrentID: fmt.Sprintf("f%d", i), Title: "F", PauseOnFreeEnd: true,
			FreeEndTime: &future, DownloaderTaskID: fmt.Sprintf("task-f%d", i), DownloaderName: "qb1",
		}
		require.NoError(t, db.DB.Create(&tor).Error)
		ids = append(ids, tor.ID)
	}
	t.Cleanup(func() {
		for _, id := range ids {
			m.CancelTorrent(id)
		}
	})

	m.rescheduleMissingFutureTorrents(time.Now())
	m.rescheduleMissingFutureTorrents(time.Now())

	m.mu.Lock()
	defer m.mu.Unlock()
	assert.Len(t, m.pendingTasks, total, "两轮之后全部种子都已预约")
}
