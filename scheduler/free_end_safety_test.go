package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 免费结束监控的安全语义（docs/design/free-expiry-safety.md 附录 B.2.10）。

func monitoredRow(t *testing.T, m *FreeEndMonitor, id string, end time.Time) models.TorrentInfo {
	t.Helper()
	pushed := true
	row := models.TorrentInfo{
		SiteName: "s", TorrentID: id, Title: "T" + id, PauseOnFreeEnd: true, IsFree: true, IsPushed: &pushed,
		FreeLevel: "free", FreeValidity: "free_until", FreeEndTime: &end, DownloaderTaskID: "task-" + id, DownloaderName: "qb1",
	}
	require.NoError(t, m.db.Create(&row).Error)
	return row
}

func reload(t *testing.T, m *FreeEndMonitor, id uint) models.TorrentInfo {
	t.Helper()
	var got models.TorrentInfo
	require.NoError(t, m.db.First(&got, id).Error)
	return got
}

func (m *FreeEndMonitor) pending(id uint) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.pendingTasks[id]
	return ok
}

// 暂停以后回读下载器，状态是暂停才算暂停成功；记下请求暂停与确认的时间。
func TestFreeEnd_PauseConfirmedByReadback(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, _ := newFreeEndMonitorWithFake(t, fake)
	m.pauseConfirmWait = time.Millisecond
	row := monitoredRow(t, m, "c1", time.Now().Add(-time.Minute))
	fake.torrentByID["task-c1"] = downloader.Torrent{ID: "task-c1", Progress: 0.4, State: downloader.TorrentDownloading}

	m.handleFreeEndedTorrent(row)

	got := reload(t, m, row.ID)
	assert.True(t, got.IsPausedBySystem)
	require.NotNil(t, got.PauseRequestedAt)
	require.NotNil(t, got.PausedAt)
	assert.Zero(t, got.FreeEndFailures)
	assert.Equal(t, []string{"task-c1"}, fake.pausedIDs)
}

// 暂停接口返回成功但下载器里还在下载：不算暂停，记一次失败并安排重试。
func TestFreeEnd_PauseAcceptedButStateUnchanged(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.pauseNoop = true
	m, _ := newFreeEndMonitorWithFake(t, fake)
	m.pauseConfirmWait = time.Millisecond
	row := monitoredRow(t, m, "u1", time.Now().Add(-time.Minute))
	fake.torrentByID["task-u1"] = downloader.Torrent{ID: "task-u1", Progress: 0.4, State: downloader.TorrentDownloading}

	m.handleFreeEndedTorrent(row)

	got := reload(t, m, row.ID)
	assert.False(t, got.IsPausedBySystem, "状态没变不能记成已暂停")
	assert.Nil(t, got.PausedAt)
	require.NotNil(t, got.PauseRequestedAt)
	assert.Equal(t, 1, got.FreeEndFailures)
	assert.Contains(t, got.LastError, "仍是")
	assert.Zero(t, got.RetryCount, "监控失败不占推送的重试次数")
	assert.True(t, m.pending(row.ID), "安排了重试")
	m.Stop()
}

// 暂时失败一直按退避重试，不因次数放弃；连续 3 次失败起告警一次。
func TestFreeEnd_PersistentBackoffAndSingleAlert(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.pauseErr = errors.New("连接被拒绝")
	m, db := newFreeEndMonitorWithFake(t, fake)
	m.pauseConfirmWait = time.Millisecond
	require.NoError(t, db.DB.AutoMigrate(&models.NotificationConf{}, &models.MonitorNotificationLog{}))
	require.NoError(t, db.DB.Create(&models.NotificationConf{Name: "tg", ChannelType: "telegram", Enabled: true}).Error)
	sent := 0
	m.SetNotifier(NewMonitorNotifier(db.DB, MonitorSenderFunc(func(context.Context, uint, MonitorMessage) error {
		sent++
		return nil
	}), nil, nil))
	row := monitoredRow(t, m, "b1", time.Now().Add(-time.Minute))
	fake.torrentByID["task-b1"] = downloader.Torrent{ID: "task-b1", Progress: 0.4, State: downloader.TorrentDownloading}

	for i := 1; i <= 5; i++ {
		m.handleFreeEndedTorrent(reload(t, m, row.ID))
		got := reload(t, m, row.ID)
		assert.Equal(t, i, got.FreeEndFailures)
		assert.False(t, got.IsPausedBySystem)
		assert.True(t, m.pending(row.ID), "第 %d 次失败以后仍安排重试", i)
		if i < 3 {
			assert.Nil(t, got.FreeEndAlertedAt)
		} else {
			assert.NotNil(t, got.FreeEndAlertedAt, "第 %d 次", i)
		}
	}
	var logs []models.MonitorNotificationLog
	require.NoError(t, db.DB.Where("source = ?", freeEndNotifySource).Find(&logs).Error)
	require.Len(t, logs, 1, "同一个免费期只告警一次")
	assert.Contains(t, logs[0].PayloadJSON, "Tb1")
	assert.NotContains(t, logs[0].PayloadJSON, "passkey")

	// 退避：30 秒起，翻倍，10 分钟封顶
	assert.Equal(t, 30*time.Second, freeEndBackoff(1))
	assert.Equal(t, time.Minute, freeEndBackoff(2))
	assert.Equal(t, 10*time.Minute, freeEndBackoff(10))
	m.Stop()
}

// 处理前重读记录：关掉了到期暂停的不再暂停；免费延长了的按新的结束时间重新预约。
func TestFreeEnd_RereadsRecordBeforeActing(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)
	m.pauseConfirmWait = time.Millisecond
	off := monitoredRow(t, m, "r1", time.Now().Add(-time.Minute))
	fake.torrentByID["task-r1"] = downloader.Torrent{ID: "task-r1", Progress: 0.4, State: downloader.TorrentDownloading}
	require.NoError(t, db.DB.Model(&models.TorrentInfo{}).Where("id = ?", off.ID).Update("pause_on_free_end", false).Error)

	m.handleFreeEndedTorrent(off) // 带着登记时的旧结构体
	assert.Empty(t, fake.pausedIDs, "已经关掉到期暂停")

	ext := monitoredRow(t, m, "r2", time.Now().Add(-time.Minute))
	fake.torrentByID["task-r2"] = downloader.Torrent{ID: "task-r2", Progress: 0.4, State: downloader.TorrentDownloading}
	later := time.Now().Add(3 * time.Hour)
	require.NoError(t, db.DB.Model(&models.TorrentInfo{}).Where("id = ?", ext.ID).Update("free_end_time", later).Error)

	m.handleFreeEndedTorrent(ext)
	assert.Empty(t, fake.pausedIDs, "免费延长了，还不到暂停的时候")
	assert.False(t, reload(t, m, ext.ID).IsPausedBySystem)
	assert.True(t, m.pending(ext.ID), "按新的结束时间重新预约")
	m.Stop()
}

// 重启：新的监控器从库里重建预约；已经到期没暂停的立刻处理；结束时间未知的、已暂停的不预约。
func TestFreeEnd_RestartRebuildsMonitoring(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m1, db := newFreeEndMonitorWithFake(t, fake)
	future := monitoredRow(t, m1, "f1", time.Now().Add(2*time.Hour))
	due := monitoredRow(t, m1, "d1", time.Now().Add(-time.Minute))
	fake.torrentByID["task-d1"] = downloader.Torrent{ID: "task-d1", Progress: 0.4, State: downloader.TorrentDownloading}
	pushed := true
	unknown := models.TorrentInfo{
		SiteName: "s", TorrentID: "n1", Title: "Tn1", PauseOnFreeEnd: true, IsFree: true, IsPushed: &pushed,
		FreeLevel: "free", FreeValidity: "unknown", DownloaderTaskID: "task-n1", DownloaderName: "qb1",
	}
	require.NoError(t, db.DB.Create(&unknown).Error)

	m1.ScheduleTorrent(future)
	require.True(t, m1.pending(future.ID))
	m1.Stop() // 进程退出，内存里的预约没了

	m2 := NewFreeEndMonitor(db.DB, m1.downloaderMgr)
	m2.pauseConfirmWait = time.Millisecond
	m2.checkInterval = time.Hour
	require.NoError(t, m2.Start())
	t.Cleanup(m2.Stop)

	assert.True(t, m2.pending(future.ID), "未到期的从库里重建")
	assert.False(t, m2.pending(unknown.ID), "结束时间未知的不预约")
	require.Eventually(t, func() bool { return reload(t, m2, due.ID).IsPausedBySystem }, 5*time.Second, 10*time.Millisecond,
		"重启前已经到期、还没暂停的立刻处理")
	assert.Contains(t, fake.pausedIDs, "task-d1")
}

// 承诺到期暂停的推送加进了下载器、记账没写成：周期对账按哈希找回任务，补上记账并登记监控，保持暂停。
func TestFreeEnd_ReconcilesPendingPush(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	m, db := newFreeEndMonitorWithFake(t, fake)
	ds := models.DownloaderSetting{Name: "qb1", Type: "qbittorrent", URL: "http://x", Enabled: true}
	require.NoError(t, db.DB.Create(&ds).Error)
	end := time.Now().Add(6 * time.Hour)
	pendingAt := time.Now().Add(-3 * time.Minute)
	hash := "abcdef0123456789abcdef0123456789abcdef01"
	notPushed := false
	row := models.TorrentInfo{
		SiteName: "s", TorrentID: "p1", Title: "Tp1", PauseOnFreeEnd: true, IsFree: true, IsPushed: &notPushed,
		FreeLevel: "free", FreeValidity: "free_until", FreeEndTime: &end, TorrentHash: &hash,
		DownloaderID: &ds.ID, DownloaderName: "qb1", PushPendingAt: &pendingAt,
	}
	require.NoError(t, db.DB.Create(&row).Error)
	fake.torrents = append(fake.torrents, downloader.Torrent{ID: hash, InfoHash: hash, State: downloader.TorrentPaused})

	goneAt := time.Now().Add(-40 * time.Minute)
	gone := models.TorrentInfo{
		SiteName: "s", TorrentID: "p2", Title: "Tp2", PauseOnFreeEnd: true, IsFree: true, IsPushed: &notPushed,
		FreeLevel: "free", FreeValidity: "free_until", FreeEndTime: &end, TorrentHash: strPtr("ffff0123456789abcdef0123456789abcdef0123"),
		DownloaderID: &ds.ID, DownloaderName: "qb1", PushPendingAt: &goneAt,
	}
	require.NoError(t, db.DB.Create(&gone).Error)

	m.reconcilePendingPushes()

	got := reload(t, m, row.ID)
	require.NotNil(t, got.IsPushed)
	assert.True(t, *got.IsPushed)
	assert.Equal(t, hash, got.DownloaderTaskID)
	assert.Nil(t, got.PushPendingAt)
	require.NotNil(t, got.PausePlannedAt)
	assert.True(t, m.pending(row.ID), "补上记账以后登记了监控")
	assert.Empty(t, fake.resumedIDs, "补记账时保持暂停，不自动开始")

	g := reload(t, m, gone.ID)
	assert.Nil(t, g.PushPendingAt, "下载器里没有、标记也早就过了：加入没发生，清掉标记")
	assert.False(t, *g.IsPushed)
	m.Stop()
}

func strPtr(s string) *string { return &s }
