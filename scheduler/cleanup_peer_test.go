package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 做种竞争度是自动删种的一条删除条件：S/L 超过上限的做种种子按设置删除或只暂停。

func peerCfg() *models.SettingsGlobal {
	cfg := baseCfg()
	cfg.PeerRatioEnabled, cfg.PeerRatioMaxSL = true, 5
	return cfg
}

// peerTorrent 是做种中、已经下完的种子，整个种子群 seeds 人做种、leeches 人下载（-1 是种子列表里没给）。
func peerTorrent(id, hash string, seeds, leeches int) downloader.Torrent {
	tor := seedingTorrent(id, hash, "T-"+id, 10, 0.5)
	tor.Progress, tor.IsCompleted = 1, true
	tor.SwarmSeeds, tor.SwarmLeechers = seeds, leeches
	return tor
}

// pushedRecord 记一条推到 dlName 的 pt-tools 推送记录。
func pushedRecord(t *testing.T, db *gorm.DB, dlName, id, hash string, mod func(*models.TorrentInfo)) {
	t.Helper()
	pushed := true
	ti := models.TorrentInfo{SiteName: "s", TorrentID: id, TorrentHash: &hash, IsPushed: &pushed, DownloaderName: dlName, Title: "T-" + id}
	if mod != nil {
		mod(&ti)
	}
	require.NoError(t, db.Create(&ti).Error)
}

func recordOf(t *testing.T, db *gorm.DB, id string) models.TorrentInfo {
	t.Helper()
	var ti models.TorrentInfo
	require.NoError(t, db.Where("torrent_id = ?", id).First(&ti).Error)
	return ti
}

func TestCleanupPeer_PausesOverThreshold(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	pushedRecord(t, cm.db, "qb1", "p1", "h1", nil)
	pushedRecord(t, cm.db, "qb1", "p2", "h2", nil)
	fake.torrents = []downloader.Torrent{peerTorrent("p1", "h1", 100, 1), peerTorrent("p2", "h2", 3, 2)}

	res := cm.processDownloader(peerCfg(), fake, "qb1")
	assert.Equal(t, []string{"p1"}, fake.pausedIDs, "100/1 超过 5，3/2 没超过")
	assert.Empty(t, fake.removedBatch)
	assert.Equal(t, 1, res.Paused)
	assert.Zero(t, res.Deleted)
	require.Len(t, res.Items, 1)
	assert.Equal(t, "pause", res.Items[0].Action)
	assert.Contains(t, res.Items[0].Reasons[0], "做种竞争度过高")

	got := recordOf(t, cm.db, "p1")
	assert.True(t, got.IsPausedBySystem)
	assert.Equal(t, PauseReasonPeerRatio, got.PauseReason)
	assert.NotNil(t, got.PausedAt)
	assert.Equal(t, 100, got.Seeders)
	assert.Equal(t, 1, got.Leechers)
	assert.False(t, recordOf(t, cm.db, "p2").IsPausedBySystem)
}

// 设置成删除时走自动删种的批量删除，连不连数据跟着「删除时连数据文件一起删」；没有人在下载时 S/L 按做种人数算。
func TestCleanupPeer_DeletesWhenSetToDelete(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	pushedRecord(t, cm.db, "qb1", "p1", "h1", nil)
	fake.torrents = []downloader.Torrent{peerTorrent("p1", "h1", 50, 0)}
	cfg := peerCfg()
	cfg.PeerRatioRemoveData, cfg.CleanupRemoveData = true, false

	res := cm.processDownloader(cfg, fake, "qb1")
	assert.Equal(t, [][]string{{"p1"}}, fake.removedBatch)
	assert.Equal(t, []bool{false}, fake.removeDataFlags)
	assert.Empty(t, fake.pausedIDs)
	assert.Equal(t, 1, res.Deleted)
	require.Len(t, res.Items, 1)
	assert.Equal(t, "delete", res.Items[0].Action)
	assert.True(t, recordOf(t, cm.db, "p1").IsExpired)
}

// 做种人数为 0 的不判断；读不到整个种子群人数的不判断，并写明。
func TestCleanupPeer_SkipsZeroSeedsAndUnknownCounts(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.trackersErr = errors.New("tracker boom")
	cm := newCleanupMonitorWithFake(t, fake)
	pushedRecord(t, cm.db, "qb1", "zero", "hz", nil)
	pushedRecord(t, cm.db, "qb1", "unk", "hu", nil)
	fake.torrents = []downloader.Torrent{peerTorrent("zero", "hz", 0, 5), peerTorrent("unk", "hu", -1, -1)}

	res := cm.processDownloader(peerCfg(), fake, "qb1")
	assert.Empty(t, fake.pausedIDs)
	assert.Empty(t, res.Items)
	require.Len(t, res.Notes, 1)
	assert.Contains(t, res.Notes[0], "1 个种子读不到")
}

// 种子列表里没有整个种子群的人数时逐个读 tracker：只看工作中的 tracker（状态 2 以上），取做种、下载人数各自的最大值。
func TestCleanupPeer_FallsBackToTorrentTrackers(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	pushedRecord(t, cm.db, "qb1", "p1", "h1", nil)
	fake.torrents = []downloader.Torrent{peerTorrent("p1", "h1", -1, -1)}
	fake.trackers["p1"] = []downloader.TorrentTracker{
		{Status: 1, Seeds: 999, Leeches: 0},
		{Status: 2, Seeds: 10, Leeches: 3},
		{Status: 4, Seeds: 20, Leeches: 2},
	}

	res := cm.processDownloader(peerCfg(), fake, "qb1")
	assert.Equal(t, []string{"p1"}, fake.pausedIDs, "20/3 超过 5")
	got := recordOf(t, cm.db, "p1")
	assert.Equal(t, 20, got.Seeders)
	assert.Equal(t, 3, got.Leechers)
	assert.Equal(t, 1, res.Paused)
}

// bulkTrackerFake 是能一次读出全部种子 tracker 状态的下载器（Transmission）。
type bulkTrackerFake struct {
	*schedFakeDownloader
	bulk  map[string][]downloader.TorrentTracker
	calls int
}

func (b *bulkTrackerFake) GetAllTorrentTrackers(context.Context) (map[string][]downloader.TorrentTracker, error) {
	b.calls++
	return b.bulk, nil
}

// Transmission：一轮只读一次全部种子的 tracker 状态，不逐个读。
func TestCleanupPeer_UsesBulkTrackers(t *testing.T) {
	inner := newSchedFakeDownloader("tr1")
	inner.dlType = downloader.DownloaderTransmission
	inner.trackersErr = errors.New("不该逐个读")
	fake := &bulkTrackerFake{schedFakeDownloader: inner, bulk: map[string][]downloader.TorrentTracker{
		"h1": {{Status: 2, Seeds: 60, Leeches: 2}},
		"h2": {{Status: 2, Seeds: 4, Leeches: 2}},
	}}
	cm := newCleanupMonitorWithFake(t, inner)
	pushedRecord(t, cm.db, "tr1", "1", "H1", nil)
	pushedRecord(t, cm.db, "tr1", "2", "h2", nil)
	inner.torrents = []downloader.Torrent{peerTorrent("1", "H1", -1, -1), peerTorrent("2", "h2", -1, -1)}

	res := cm.processDownloader(peerCfg(), fake, "tr1")
	assert.Equal(t, 1, fake.calls)
	assert.Equal(t, []string{"1"}, inner.pausedIDs)
	assert.Empty(t, res.Notes)
}

// 只判断正在做种、已经下完的种子：暂停着的（包括竞争度暂停过的）不会被重复暂停，没下完的不判断。
func TestCleanupPeer_OnlyJudgesCompletedSeeding(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	pushedRecord(t, cm.db, "qb1", "paused", "hp", func(ti *models.TorrentInfo) {
		ti.IsPausedBySystem, ti.PauseReason = true, PauseReasonPeerRatio
	})
	pushedRecord(t, cm.db, "qb1", "partial", "hpa", nil)
	paused := peerTorrent("paused", "hp", 100, 1)
	paused.State = downloader.TorrentPaused
	partial := peerTorrent("partial", "hpa", 100, 1)
	partial.Progress, partial.IsCompleted = 0.9, false
	fake.torrents = []downloader.Torrent{paused, partial}

	res := cm.processDownloader(peerCfg(), fake, "qb1")
	assert.Empty(t, fake.pausedIDs)
	assert.Empty(t, res.Items)
	assert.True(t, recordOf(t, cm.db, "paused").IsPausedBySystem, "还暂停着的仍算系统暂停")
}

// 竞争度暂停过的种子，用户手动开始以后（下载器里做种、下载或排队）不再暂停：清掉暂停标记、记下手动开始过。
// 暂停、停止着的仍算系统暂停的；校验中的这一轮不判断。
func TestCleanupPeer_ManualStartIsNotPausedAgain(t *testing.T) {
	cases := []struct {
		state    downloader.TorrentState
		released bool
	}{
		{downloader.TorrentSeeding, true},
		{downloader.TorrentQueued, true},
		{downloader.TorrentDownloading, true},
		{downloader.TorrentStopped, false},
		{downloader.TorrentPaused, false},
		{downloader.TorrentChecking, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.state), func(t *testing.T) {
			fake := newSchedFakeDownloader("qb1")
			cm := newCleanupMonitorWithFake(t, fake)
			pushedRecord(t, cm.db, "qb1", "p1", "h1", func(ti *models.TorrentInfo) {
				now := time.Now().Add(-time.Hour)
				ti.IsPausedBySystem, ti.PauseReason, ti.PausedAt = true, PauseReasonPeerRatio, &now
			})
			tor := peerTorrent("p1", "h1", 100, 1)
			tor.State = tc.state
			fake.torrents = []downloader.Torrent{tor}

			cm.processDownloader(peerCfg(), fake, "qb1")
			assert.Empty(t, fake.pausedIDs, "不重复暂停")
			got := recordOf(t, cm.db, "p1")
			assert.Equal(t, tc.released, got.PeerPauseOverridden)
			assert.Equal(t, !tc.released, got.IsPausedBySystem)
			if tc.released {
				assert.Empty(t, got.PauseReason)
				assert.Nil(t, got.PausedAt)
			}

			// 再来一轮：手动开始过的仍不暂停
			cm.processDownloader(peerCfg(), fake, "qb1")
			assert.Empty(t, fake.pausedIDs)
		})
	}
}

// 在「暂停任务」页恢复过（记录上记了手动开始过）的种子不再暂停。
func TestCleanupPeer_OverriddenIsNotPaused(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	pushedRecord(t, cm.db, "qb1", "p1", "h1", func(ti *models.TorrentInfo) { ti.PeerPauseOverridden = true })
	fake.torrents = []downloader.Torrent{peerTorrent("p1", "h1", 100, 1)}

	cm.processDownloader(peerCfg(), fake, "qb1")
	assert.Empty(t, fake.pausedIDs)
}

// 竞争度暂停着的种子照样按常规条件删（以前常规条件跳过它们）。
func TestCleanupPeer_PausedIsDeletedByRegularConditions(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	pushedRecord(t, cm.db, "qb1", "p1", "h1", func(ti *models.TorrentInfo) {
		ti.IsPausedBySystem, ti.PauseReason = true, PauseReasonPeerRatio
	})
	tor := seedingTorrent("p1", "h1", "Paused", 100, 1.0)
	tor.State = downloader.TorrentPaused
	fake.torrents = []downloader.Torrent{tor}
	cfg := peerCfg()
	cfg.CleanupMaxSeedTimeH = 72
	cfg.CleanupProtectHR = false

	cm.processDownloader(cfg, fake, "qb1")
	assert.Equal(t, [][]string{{"p1"}}, fake.removedBatch)
}

// 竞争度只看 pt-tools 推送过的种子：没有推送记录、推送前的记录（is_pushed 为空）的不判断，即使管理范围是全部种子。
func TestCleanupPeer_OnlyPushedRecords(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	hash := "hs"
	require.NoError(t, cm.db.Create(&models.TorrentInfo{SiteName: "s", TorrentID: "staged", TorrentHash: &hash, DownloaderName: "qb1"}).Error)
	fake.torrents = []downloader.Torrent{peerTorrent("staged", "hs", 100, 1), peerTorrent("manual", "hm", 100, 1)}

	res := cm.processDownloader(peerCfg(), fake, "qb1")
	assert.Equal(t, 2, res.Managed)
	assert.Empty(t, fake.pausedIDs)
}

// 和别的删除条件一样遵守保护规则：刷流任务正在管的、H&R 没做满的、带保留或保护标签的、加进下载器不满最短保种时间的都不处理。
func TestCleanupPeer_RespectsProtection(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	bt := addActiveBrush(t, cm.db, "qb1", "h-brush")
	dlID := bt.DownloaderID
	mk := func(id string, mod func(*models.TorrentInfo, *downloader.Torrent)) downloader.Torrent {
		hash := "h-" + id
		pushed := true
		ti := models.TorrentInfo{SiteName: "s", TorrentID: id, TorrentHash: &hash, IsPushed: &pushed, DownloaderID: &dlID, DownloaderName: "qb1"}
		tor := peerTorrent(id, hash, 100, 1)
		tor.DateAdded = time.Now().Add(-48 * time.Hour).Unix()
		if mod != nil {
			mod(&ti, &tor)
		}
		require.NoError(t, cm.db.Create(&ti).Error)
		return tor
	}
	fake.torrents = []downloader.Torrent{
		mk("brush", func(_ *models.TorrentInfo, tor *downloader.Torrent) { tor.Tags = "s," + models.BrushTagAll }),
		mk("hr", func(ti *models.TorrentInfo, _ *downloader.Torrent) { ti.HasHR, ti.HRSeedTimeH = true, 72 }),
		mk("keep", func(_ *models.TorrentInfo, tor *downloader.Torrent) { tor.Tags = models.KeepTag }),
		mk("fav", func(_ *models.TorrentInfo, tor *downloader.Torrent) { tor.Tags = "fav" }),
		mk("young", func(_ *models.TorrentInfo, tor *downloader.Torrent) {
			tor.DateAdded = time.Now().Add(-time.Hour).Unix()
		}),
		mk("plain", nil),
	}
	cfg := peerCfg()
	cfg.CleanupProtectTags, cfg.CleanupMinRetainH = "fav", 24

	res := cm.processDownloader(cfg, fake, "qb1")
	assert.Equal(t, []string{"plain"}, fake.pausedIDs)
	assert.Equal(t, 1, res.Brush)
}

// 判断以后、执行以前种子带上了保留标签或者不在做种了：删除锁里按重新读到的列表复查，不暂停。
func TestCleanupPeer_RechecksUnderRemovalLock(t *testing.T) {
	for name, change := range map[string]func(*downloader.Torrent){
		"带上保留标签": func(tor *downloader.Torrent) { tor.Tags = models.KeepTag },
		"不在做种了":  func(tor *downloader.Torrent) { tor.State = downloader.TorrentPaused },
	} {
		t.Run(name, func(t *testing.T) {
			fake := newSchedFakeDownloader("qb1")
			cm := newCleanupMonitorWithFake(t, fake)
			pushedRecord(t, cm.db, "qb1", "p1", "h1", nil)
			tor := peerTorrent("p1", "h1", 100, 1)
			fake.torrents = []downloader.Torrent{tor}
			fake.beforeSecondList = func() {
				changed := tor
				change(&changed)
				fake.torrents = []downloader.Torrent{changed}
			}

			res := cm.processDownloader(peerCfg(), fake, "qb1")
			assert.Empty(t, fake.pausedIDs)
			assert.Zero(t, res.Paused)
			assert.Empty(t, res.Items)
			assert.False(t, recordOf(t, cm.db, "p1").IsPausedBySystem)
		})
	}
}

// 只读写这台下载器上的记录：同一个种子在别的下载器上的记录不动。
func TestCleanupPeer_ScopesRecordsToDownloader(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	pushedRecord(t, cm.db, "qb1", "p1", "h1", nil)
	pushedRecord(t, cm.db, "qb2", "other", "h1", func(ti *models.TorrentInfo) { ti.SiteName = "s2" })
	fake.torrents = []downloader.Torrent{peerTorrent("p1", "h1", 100, 1)}

	cm.processDownloader(peerCfg(), fake, "qb1")
	assert.Equal(t, []string{"p1"}, fake.pausedIDs)
	other := recordOf(t, cm.db, "other")
	assert.False(t, other.IsPausedBySystem)
	assert.Zero(t, other.Seeders)
}

// 竞争度删除和别的条件一样：数据还被别的种子用着时只删种子、保留数据。
func TestCleanupPeer_DeleteKeepsSharedData(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	pushedRecord(t, cm.db, "qb1", "p1", "h1", nil)
	tor := peerTorrent("p1", "h1", 50, 0)
	tor.ContentPath = "/data/Movie"
	reseed := downloader.Torrent{ID: "rs", InfoHash: "hrs", ContentPath: "/data/Movie", State: downloader.TorrentSeeding}
	fake.torrents = []downloader.Torrent{tor, reseed}
	cfg := peerCfg()
	cfg.CleanupScope = "database"
	cfg.PeerRatioRemoveData = true

	cm.processDownloader(cfg, fake, "qb1")
	assert.Equal(t, [][]string{{"p1"}}, fake.removedBatch)
	assert.Equal(t, []bool{false}, fake.removeDataFlags)
}

// 竞争度没开时不判断。
func TestCleanupPeer_Disabled(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	pushedRecord(t, cm.db, "qb1", "p1", "h1", nil)
	fake.torrents = []downloader.Torrent{peerTorrent("p1", "h1", 100, 1)}
	cfg := peerCfg()
	cfg.PeerRatioEnabled = false

	cm.processDownloader(cfg, fake, "qb1")
	assert.Empty(t, fake.pausedIDs)
}

// 预览只算不改库：列出要暂停的（action = pause），不暂停、不改记录；手动开始过的（记录标着竞争度暂停、下载器里在做种）不列。
func TestCleanupPeer_PreviewDoesNotWrite(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	pushedRecord(t, cm.db, "qb1", "p1", "h1", nil)
	pushedRecord(t, cm.db, "qb1", "started", "h2", func(ti *models.TorrentInfo) {
		ti.IsPausedBySystem, ti.PauseReason = true, PauseReasonPeerRatio
	})
	fake.torrents = []downloader.Torrent{peerTorrent("p1", "h1", 100, 1), peerTorrent("started", "h2", 100, 1)}

	r := cm.Preview(t.Context(), peerCfg())[0]
	require.Len(t, r.Items, 1)
	assert.Equal(t, "T-p1", r.Items[0].Name)
	assert.Equal(t, "pause", r.Items[0].Action)
	assert.Empty(t, fake.pausedIDs)
	assert.False(t, recordOf(t, cm.db, "p1").IsPausedBySystem)
	started := recordOf(t, cm.db, "started")
	assert.True(t, started.IsPausedBySystem, "预览不清暂停标记")
	assert.False(t, started.PeerPauseOverridden)
}

// 紧急清理挑中的种子只删，不再按竞争度暂停。
func TestCleanupPeer_EmergencyDeleteWinsOverPause(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	pushedRecord(t, cm.db, "qb1", "p1", "h1", nil)
	tor := peerTorrent("p1", "h1", 100, 1)
	tor.TotalSize = 200 * oneGB
	fake.torrents = []downloader.Torrent{tor}
	fake.diskInfo = downloader.DiskInfo{FreeSpace: 10 * oneGB}
	cfg := peerCfg()
	cfg.CleanupDiskProtect, cfg.CleanupEmergency, cfg.CleanupMinDiskSpaceGB = true, true, 100

	res := cm.processDownloader(cfg, fake, "qb1")
	assert.Equal(t, [][]string{{"p1"}}, fake.removedBatch)
	assert.Empty(t, fake.pausedIDs)
	require.Len(t, res.Items, 1)
	assert.Equal(t, "delete", res.Items[0].Action)
}

// 「空间低于下限时紧急清理」关着时，空间不足也不多挑种子删。
func TestCleanupEmergency_OffDoesNotPick(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)
	big := seedingTorrent("big", "hbig", "Big", 10, 1.0)
	big.TotalSize = 200 * oneGB
	fake.torrents = []downloader.Torrent{big}
	fake.diskInfo = downloader.DiskInfo{FreeSpace: 10 * oneGB}
	cfg := baseCfg()
	cfg.CleanupProtectHR, cfg.CleanupProtectDL = false, false
	cfg.CleanupDiskProtect, cfg.CleanupMinDiskSpaceGB, cfg.CleanupEmergency = true, 100, false

	r := cm.Preview(t.Context(), cfg)[0]
	assert.False(t, r.Emergency)
	assert.Empty(t, r.Items)
}

// 收到磁盘空间不足：紧急清理开着时短暂去抖后提前开始一轮，关着时接着等。
func TestCleanupWaitNext_DiskSpaceLow(t *testing.T) {
	run := func(t *testing.T, emergency bool) (*CleanupMonitor, chan events.Event, chan bool) {
		t.Helper()
		cm := newTestCleanupMonitor(t)
		cm.diskDebounce = time.Millisecond
		require.NoError(t, cm.db.Create(&models.SettingsGlobal{DownloadDir: t.TempDir(), CleanupEnabled: true}).Error)
		require.NoError(t, cm.db.Model(&models.SettingsGlobal{}).Where("1 = 1").Update("cleanup_emergency", emergency).Error)
		ch := make(chan events.Event)
		out := make(chan bool, 1)
		go func() { out <- cm.waitNext(ch, time.Now().Add(time.Hour)) }()
		return cm, ch, out
	}

	t.Run("开着", func(t *testing.T) {
		_, ch, out := run(t, true)
		ch <- events.Event{Type: events.DiskSpaceLow}
		select {
		case ok := <-out:
			assert.True(t, ok)
		case <-time.After(5 * time.Second):
			t.Fatal("没有提前开始")
		}
	})
	t.Run("关着", func(t *testing.T) {
		cm, ch, out := run(t, false)
		// 两次都被收下，说明收到第一次以后还在等
		ch <- events.Event{Type: events.DiskSpaceLow}
		ch <- events.Event{Type: events.DiskSpaceLow}
		cm.cancel()
		select {
		case ok := <-out:
			assert.False(t, ok, "监控停了")
		case <-time.After(5 * time.Second):
			t.Fatal("没有返回")
		}
	})
	t.Run("配置变更提前开始", func(t *testing.T) {
		_, ch, out := run(t, false)
		ch <- events.Event{Type: events.ConfigChanged}
		select {
		case ok := <-out:
			assert.True(t, ok)
		case <-time.After(5 * time.Second):
			t.Fatal("没有提前开始")
		}
	})
}
