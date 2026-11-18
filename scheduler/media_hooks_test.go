package scheduler

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

// completedEvents 订阅下载完成事件；返回的函数取出到现在为止收到的（只留标题以 prefix 开头的，免得混进别的测试的）。
func completedEvents(t *testing.T, prefix string) func() []events.TorrentCompletedPayload {
	t.Helper()
	_, ch, cancel := events.Subscribe(64)
	t.Cleanup(cancel)
	return func() []events.TorrentCompletedPayload {
		var out []events.TorrentCompletedPayload
		for {
			select {
			case e := <-ch:
				if e.Type != events.EvtTorrentCompleted {
					continue
				}
				var p events.TorrentCompletedPayload
				require.NoError(t, json.Unmarshal(e.Payload, &p))
				if len(p.Title) >= len(prefix) && p.Title[:len(prefix)] == prefix {
					out = append(out, p)
				}
			default:
				return out
			}
		}
	}
}

func TestFreeEndMonitorPublishesCompletedOnce(t *testing.T) {
	got := completedEvents(t, "Evt")
	fake := newSchedFakeDownloader("qb1")
	fake.torrentByID["task-e"] = downloader.Torrent{ID: "task-e", InfoHash: "ABCDEF", Progress: 1, State: downloader.TorrentSeeding, TotalSize: 100}
	fake.torrentByID["task-p"] = downloader.Torrent{ID: "task-p", InfoHash: "123", Progress: 0.4, State: downloader.TorrentDownloading, TotalSize: 100}
	m, db := newFreeEndMonitorWithFake(t, fake)

	pushed := true
	dlID := uint(3)
	future := time.Now().Add(time.Hour)
	done := models.TorrentInfo{
		SiteName: "s", TorrentID: "e1", Title: "EvtDone", IsPushed: &pushed, PauseOnFreeEnd: true, FreeEndTime: &future,
		DownloaderTaskID: "task-e", DownloaderName: "qb1", DownloaderID: &dlID,
	}
	part := models.TorrentInfo{SiteName: "s", TorrentID: "e2", Title: "EvtPart", IsPushed: &pushed, DownloaderTaskID: "task-p", DownloaderName: "qb1"}
	require.NoError(t, db.DB.Create(&done).Error)
	require.NoError(t, db.DB.Create(&part).Error)

	// 两个进度循环都看到它下载完了：只有真正改成完成的那一次发布
	m.updateAllMonitoredProgress()
	m.updateAllPushedTasksProgress()
	evs := got()
	require.Len(t, evs, 1, "没下载完的不发布；同一个种子只发布一次")
	assert.Equal(t, events.TorrentCompletedPayload{
		TorrentID: "e1", SiteName: "s", Title: "EvtDone", DownloaderID: 3, DownloaderName: "qb1", TaskID: "task-e", InfoHash: "abcdef",
	}, evs[0])

	// 免费到期处理时发现已经下载完（还没被进度循环标记）：发布一次；再标记一次不发布
	fake.torrentByID["task-p"] = downloader.Torrent{ID: "task-p", Progress: 1, State: downloader.TorrentSeeding, TotalSize: 100}
	hash := "FEED"
	require.NoError(t, db.DB.Model(&models.TorrentInfo{}).Where("id = ?", part.ID).Update("torrent_hash", &hash).Error)
	part.TorrentHash = &hash
	m.markCompleted(part, 100)
	m.markCompleted(part, 100)
	evs = got()
	require.Len(t, evs, 1)
	assert.Equal(t, "feed", evs[0].InfoHash, "没有下载器给的 hash 时用记录里的")
	var after models.TorrentInfo
	require.NoError(t, db.DB.First(&after, part.ID).Error)
	assert.True(t, after.IsCompleted)

	// 种子从下载器里删了：标记完成但不是下载完，不发布
	gone := models.TorrentInfo{SiteName: "s", TorrentID: "e3", Title: "EvtGone", IsPushed: &pushed, DownloaderTaskID: "task-gone", DownloaderName: "qb1"}
	require.NoError(t, db.DB.Create(&gone).Error)
	m.updateAllPushedTasksProgress()
	assert.Empty(t, got())
	var removed models.TorrentInfo
	require.NoError(t, db.DB.First(&removed, gone.ID).Error)
	assert.True(t, removed.IsCompleted)
}

func TestEmergencyCleanupSkipsHardlinkedTorrents(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	a := downloader.Torrent{ID: "a", InfoHash: "AAAA", TotalSize: 10 << 30}
	b := downloader.Torrent{ID: "b", InfoHash: "bbbb", TotalSize: 10 << 30}
	c := downloader.Torrent{ID: "c", InfoHash: "cccc", TotalSize: 10 << 30}
	all := []downloader.Torrent{a, b, c}

	assert.Equal(t, all, cm.withoutLinked(all, "qb1"), "还没有整理记录的表时不去掉")
	require.NoError(t, cm.db.AutoMigrate(&models.MediaTransferHistory{}, &models.MediaOrganizeSetting{}))
	assert.Equal(t, all, cm.withoutLinked(all, "qb1"), "没有整理记录时不去掉")
	require.NoError(t, cm.db.Create(&[]models.MediaTransferHistory{
		{DownloaderName: "qb1", InfoHash: "aaaa", SourcePath: "/d/a.mkv", Status: models.MediaTransferDone, Mode: models.MediaModeHardlink},
		{DownloaderName: "qb1", InfoHash: "bbbb", SourcePath: "/d/b.mkv", Status: models.MediaTransferDone, Mode: models.MediaModeSymlink},
		{DownloaderName: "qb1", InfoHash: "cccc", SourcePath: "/d/c.mkv", Status: models.MediaTransferFailed, Mode: models.MediaModeHardlink},
		{DownloaderName: "qb2", InfoHash: "bbbb", SourcePath: "/d2/b.mkv", Status: models.MediaTransferDone, Mode: models.MediaModeHardlink},
	}).Error)
	assert.Equal(t, []downloader.Torrent{b, c}, cm.withoutLinked(all, "qb1"),
		"只去掉这个下载器里硬链接整理成功的（软链接、失败的、别的下载器的不算）")

	cfg := baseCfg()
	cfg.CleanupRemoveData = true
	cfg.CleanupMinDiskSpaceGB = 100
	result := cm.emergencyCleanup(cfg, downloader.NewDataSharing(all, nil), cm.withoutLinked(all, "qb1"), nil, 60)
	for _, r := range result {
		assert.NotEqual(t, "a", r.ID, "硬链接入库的删了也腾不出空间")
	}

	require.NoError(t, cm.db.Create(&models.MediaOrganizeSetting{ID: 1, DeleteLinksOnRemove: true}).Error)
	assert.Equal(t, all, cm.withoutLinked(all, "qb1"), "打开了「删种时一并删除入库链接」时不去掉")
	assert.Empty(t, cm.withoutLinked(nil, "qb1"))
}
