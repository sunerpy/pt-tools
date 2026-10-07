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

// 辅种与原种子共用同一份数据：自动删种删其中一个时只删种子、保留数据；数据没人共用的照常连数据删。
func TestCleanup_KeepsSharedData(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	cfg := baseCfg()
	cfg.CleanupMinRatio = 2
	cfg.CleanupProtectDL, cfg.CleanupProtectHR = false, false

	orig := seedingTorrent("orig", "hash-orig", "Movie", 10, 3)
	orig.ContentPath = "/data/Movie"
	reseed := seedingTorrent("reseed", "hash-reseed", "Movie", 10, 0.5)
	reseed.ContentPath = "/data/Movie"
	solo := seedingTorrent("solo", "hash-solo", "Other", 10, 3)
	solo.ContentPath = "/data/Other"
	fake.torrents = []downloader.Torrent{orig, reseed, solo}

	cm.processDownloader(cfg, fake, "qb1")
	require.Len(t, fake.removedBatch, 2)
	assert.Equal(t, []string{"solo"}, fake.removedBatch[0])
	assert.Equal(t, []string{"orig"}, fake.removedBatch[1])
	assert.Equal(t, []bool{true, false}, fake.removeDataFlags, "共用数据的只删种子")
}

func TestPeerRatio_KeepsSharedData(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	pm, db := newPeerRatioMonitorWithFake(t, fake)
	tor := completedManagedTorrent(t, db, "pr9", "peer-shared", "qb1")
	tor.ContentPath = "/data/Movie"
	reseed := downloader.Torrent{ID: "rs", InfoHash: "peer-reseed", ContentPath: "/data/Movie", State: downloader.TorrentSeeding}
	fake.torrents = []downloader.Torrent{tor, reseed}
	fake.trackers[tor.ID] = []downloader.TorrentTracker{{Status: 2, Seeds: 50, Leeches: 0}}

	pm.processDownloader(fake, "qb1", 5.0, true)
	require.Equal(t, []string{"pr9"}, fake.removedSingle)
	assert.Equal(t, []bool{false}, fake.removeDataFlags)
}

func TestBrushMonitor_KeepsSharedData(t *testing.T) {
	r := newBrushRig(t)
	task := r.task(t, func(bt *models.BrushTask) { bt.RemoveRatio = 2; bt.Enabled = false; bt.RemoveWithData = true })
	tag := models.BrushTaskTag(task.ID)
	repo := models.NewBrushRepository(r.db.DB)
	require.NoError(t, repo.RecordAdded(&models.BrushTorrent{
		TaskID: task.ID, InfoHash: "done", SiteName: "hdsky", TorrentID: "done", Title: "T-done",
		AddedAt: r.clock.Now().Add(-time.Hour), State: models.BrushTorrentActive, DownloaderID: 1,
	}, "2026-10-06"))
	r.dl.torrents = []downloader.Torrent{
		{ID: "id-done", InfoHash: "done", Tags: tag, Progress: 1, Ratio: 2.5, ContentPath: "/dl/Movie"},
		{ID: "id-reseed", InfoHash: "other", Tags: "ourbits,pt-tools-reseed", Progress: 1, ContentPath: "/dl/Movie"},
	}
	_, err := r.mon.RunTask(context.Background(), task.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"id-done"}, r.dl.removedSingle)
	assert.Equal(t, []bool{false}, r.dl.removeDataFlags, "给它加的辅种还用着数据")
}

// 两个要删的种子共用数据：先删的只删种子，最后一个连数据删。
func TestPeerRatio_LastSharedRemovesData(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	pm, db := newPeerRatioMonitorWithFake(t, fake)
	a := completedManagedTorrent(t, db, "pa", "peer-a", "qb1")
	b := completedManagedTorrent(t, db, "pb", "peer-b", "qb1")
	a.ContentPath, b.ContentPath = "/data/Movie", "/data/Movie"
	fake.torrents = []downloader.Torrent{a, b}
	for _, id := range []string{"pa", "pb"} {
		fake.trackers[id] = []downloader.TorrentTracker{{Status: 2, Seeds: 50, Leeches: 0}}
	}

	pm.processDownloader(fake, "qb1", 5.0, true)
	assert.Equal(t, []string{"pa", "pb"}, fake.removedSingle)
	assert.Equal(t, []bool{false, true}, fake.removeDataFlags)
}

func TestHandleFreeEnded_AutoDelete_KeepsSharedData(t *testing.T) {
	setup := func(t *testing.T) (*schedFakeDownloader, *FreeEndMonitor, models.TorrentInfo) {
		fake := newSchedFakeDownloader("qb1")
		m, db := newFreeEndMonitorWithFake(t, fake)
		require.NoError(t, db.DB.Create(&models.SettingsGlobal{DownloadDir: t.TempDir(), AutoDeleteOnFreeEnd: true}).Error)
		inc := downloader.Torrent{ID: "task-sh", InfoHash: "task-sh", Progress: 0.4, State: downloader.TorrentDownloading, ContentPath: "/d/Movie"}
		fake.torrentByID["task-sh"] = inc
		fake.torrents = []downloader.Torrent{inc, {ID: "other", InfoHash: "other", Progress: 1, ContentPath: "/d/Movie"}}
		freeEnd := time.Now().Add(-time.Minute)
		tor := models.TorrentInfo{
			SiteName: "s", TorrentID: "ad9", Title: "Shared", PauseOnFreeEnd: true,
			FreeEndTime: &freeEnd, DownloaderTaskID: "task-sh", DownloaderName: "qb1",
		}
		require.NoError(t, db.DB.Create(&tor).Error)
		return fake, m, tor
	}

	fake, m, tor := setup(t)
	m.handleFreeEndedTorrent(tor)
	assert.Equal(t, []string{"task-sh"}, fake.removedSingle)
	assert.Equal(t, []bool{false}, fake.removeDataFlags, "别的种子还用着数据")

	fake, m, tor = setup(t)
	fake.getAllErr = errors.New("list boom")
	m.handleFreeEndedTorrent(tor)
	assert.Empty(t, fake.removedSingle, "读不到种子列表时不删")
	var updated models.TorrentInfo
	require.NoError(t, m.db.First(&updated, tor.ID).Error)
	assert.False(t, updated.IsPausedBySystem)
	assert.Greater(t, updated.RetryCount, 0, "下次再试")
}

// 紧急清理：数据还被不删的种子用着的删了腾不出空间，不额外挑；和要删的种子共用数据的可以一起删，空间只算一次。
func TestEmergencyCleanup_SharedData(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupMinDiskSpaceGB = 100 // 目标 120 GB

	marked := downloader.Torrent{ID: "m", InfoHash: "m", TotalSize: 30 << 30, ContentPath: "/d/A", State: downloader.TorrentSeeding}
	markedTwin := downloader.Torrent{ID: "mt", InfoHash: "mt", TotalSize: 30 << 30, ContentPath: "/d/A", State: downloader.TorrentPaused, Ratio: 9}
	sharedBig := downloader.Torrent{ID: "sb", InfoHash: "sb", TotalSize: 400 << 30, ContentPath: "/d/B", State: downloader.TorrentPaused, Ratio: 9}
	keeper := downloader.Torrent{ID: "k", InfoHash: "k", TotalSize: 400 << 30, ContentPath: "/d/B"} // 不在管理范围，不删
	plain := downloader.Torrent{ID: "p", InfoHash: "p", TotalSize: 50 << 30, ContentPath: "/d/C", State: downloader.TorrentSeeding}
	all := []downloader.Torrent{marked, markedTwin, sharedBig, keeper, plain}
	candidates := []downloader.Torrent{marked, markedTwin, sharedBig, plain}

	// 当前 60 GB，还差 60 GB：已选中的 30 GB + 和它共用数据的 0 GB + plain 50 GB
	result := cm.emergencyCleanup(cfg, downloader.NewDataSharing(all, nil), candidates, []downloader.Torrent{marked}, 60)
	ids := map[string]bool{}
	for _, r := range result {
		ids[r.ID] = true
	}
	assert.False(t, ids["sb"], "数据还被不删的种子用着，删了也腾不出空间")
	assert.True(t, ids["mt"], "和要删的种子共用数据，可以一起删")
	assert.True(t, ids["p"], "共用的那份只算一次，还不够，再删一个")
	assert.True(t, ids["m"])
}

// 紧急清理：共用数据的两个种子都只是候选（常规规则都没选中）时，整组一起挑、空间只算一次；组里有受保护的就不挑。
func TestEmergencyCleanup_PicksSharedGroup(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupMinDiskSpaceGB = 100 // 目标 120 GB

	a := downloader.Torrent{ID: "a", InfoHash: "a", TotalSize: 80 << 30, ContentPath: "/d/A", State: downloader.TorrentPaused, Ratio: 9}
	b := downloader.Torrent{ID: "b", InfoHash: "b", TotalSize: 80 << 30, ContentPath: "/d/A", State: downloader.TorrentSeeding}
	result := cm.emergencyCleanup(cfg, downloader.NewDataSharing([]downloader.Torrent{a, b}, nil), []downloader.Torrent{a, b}, nil, 60)
	ids := []string{}
	for _, r := range result {
		ids = append(ids, r.ID)
	}
	assert.ElementsMatch(t, []string{"a", "b"}, ids, "整组一起删才腾得出空间")

	// 组里还有一个不在候选里（受保护或不在管理范围）：整组不挑，挑别的
	keep := downloader.Torrent{ID: "k", InfoHash: "k", TotalSize: 80 << 30, ContentPath: "/d/A"}
	c := downloader.Torrent{ID: "c", InfoHash: "c", TotalSize: 70 << 30, ContentPath: "/d/C", State: downloader.TorrentSeeding}
	result = cm.emergencyCleanup(cfg, downloader.NewDataSharing([]downloader.Torrent{a, b, keep, c}, nil), []downloader.Torrent{a, b, c}, nil, 60)
	ids = ids[:0]
	for _, r := range result {
		ids = append(ids, r.ID)
	}
	assert.Equal(t, []string{"c"}, ids)
}

// 紧急清理：同一目录下文件不同的平铺种子不是同一份数据，空间各算各的，够了就不再多删。
func TestEmergencyCleanup_FlatTorrentsCountedSeparately(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupMinDiskSpaceGB = 100 // 目标 120 GB，当前 60 GB，还差 60 GB

	flat := func(id string, ratio float64) downloader.Torrent {
		return downloader.Torrent{ID: id, InfoHash: id, Name: id, SavePath: "/data", ContentPath: "/data", TotalSize: 40 << 30, State: downloader.TorrentSeeding, Ratio: ratio}
	}
	a, b, c := flat("a", 3), flat("b", 2), flat("c", 1)
	files := func(t downloader.Torrent) ([]downloader.TorrentFile, error) {
		return []downloader.TorrentFile{{Name: t.ID + ".mkv", Size: t.TotalSize}}, nil
	}
	all := []downloader.Torrent{a, b, c}
	result := cm.emergencyCleanup(cfg, downloader.NewDataSharing(all, files), all, nil, 60)
	ids := []string{}
	for _, r := range result {
		ids = append(ids, r.ID)
	}
	assert.Len(t, ids, 2, "删两个 40 GB 就够了：%v", ids)
	assert.NotContains(t, ids, "c", "分享率最低的留下")
}
