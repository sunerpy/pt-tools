package scheduler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 竞争度遵守和自动删种一样的保护：刷流任务正在管的、H&R 没做满的（打开了保护 H&R 时）、带保留或保护标签的、
// 加进下载器不满最短保种时间的都不删、不暂停。
func TestPeerRatio_RespectsProtection(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	pm, db := newPeerRatioMonitorWithFake(t, fake)
	require.NoError(t, db.DB.Create(&models.SettingsGlobal{
		CleanupProtectHR: true, CleanupProtectTags: "fav", CleanupMinRetainH: 24,
	}).Error)
	bt := addActiveBrush(t, db.DB, "qb1", "h-brush")
	dlID := bt.DownloaderID
	old := time.Now().Add(-48 * time.Hour).Unix()
	mk := func(id string, mod func(*models.TorrentInfo, *downloader.Torrent)) downloader.Torrent {
		hash := "h-" + id
		pushed := true
		ti := models.TorrentInfo{
			SiteName: "s", TorrentID: id, TorrentHash: &hash, IsPushed: &pushed, IsCompleted: true,
			DownloaderID: &dlID, DownloaderName: "qb1", Title: id,
		}
		tor := downloader.Torrent{ID: id, InfoHash: hash, Name: id, State: downloader.TorrentSeeding, Progress: 1, DateAdded: old, SeedingTime: 3600}
		if mod != nil {
			mod(&ti, &tor)
		}
		require.NoError(t, db.DB.Create(&ti).Error)
		fake.trackers[id] = []downloader.TorrentTracker{{Status: 2, Seeds: 100, Leeches: 1}}
		return tor
	}
	fake.torrents = []downloader.Torrent{
		mk("brush", func(_ *models.TorrentInfo, tor *downloader.Torrent) { tor.Tags = "s,pt-tools-brush" }),
		mk("hr", func(ti *models.TorrentInfo, _ *downloader.Torrent) { ti.HasHR, ti.HRSeedTimeH = true, 72 }),
		mk("keep", func(_ *models.TorrentInfo, tor *downloader.Torrent) { tor.Tags = models.KeepTag }),
		mk("fav", func(_ *models.TorrentInfo, tor *downloader.Torrent) { tor.Tags = "fav" }),
		mk("young", func(_ *models.TorrentInfo, tor *downloader.Torrent) {
			tor.DateAdded = time.Now().Add(-time.Hour).Unix()
		}),
		mk("plain", nil),
	}

	pm.processDownloader(fake, "qb1", 5.0, true)
	assert.Equal(t, []string{"plain"}, fake.removedSingle)
}

// 判断以后、处理以前种子带上了保留标签：删除锁里重新读列表、复查标签，不处理。
func TestPeerRatio_RechecksUnderRemovalLock(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	pm, db := newPeerRatioMonitorWithFake(t, fake)
	tor := completedManagedTorrent(t, db, "pr1", "peerhash1", "qb1")
	fake.torrents = []downloader.Torrent{tor}
	fake.trackers[tor.ID] = []downloader.TorrentTracker{{Status: 2, Seeds: 100, Leeches: 1}}
	fake.beforeSecondList = func() {
		kept := tor
		kept.Tags = models.KeepTag
		fake.torrents = []downloader.Torrent{kept}
	}

	pm.processDownloader(fake, "qb1", 5.0, false)
	assert.Empty(t, fake.pausedIDs)
	var info models.TorrentInfo
	require.NoError(t, db.DB.Where("torrent_id = ?", "pr1").First(&info).Error)
	assert.False(t, info.IsPausedBySystem)
}

// 竞争度只读写这台下载器上的记录：同一个种子在别的下载器上的记录不动。
func TestPeerRatio_ScopesRecordsToDownloader(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	pm, db := newPeerRatioMonitorWithFake(t, fake)
	tor := completedManagedTorrent(t, db, "pr1", "peerhash1", "qb1")
	hash := "peerhash1"
	pushed := true
	require.NoError(t, db.DB.Create(&models.TorrentInfo{
		SiteName: "s2", TorrentID: "other", TorrentHash: &hash, IsPushed: &pushed, IsCompleted: true, DownloaderName: "qb2",
	}).Error)
	fake.torrents = []downloader.Torrent{tor}
	fake.trackers[tor.ID] = []downloader.TorrentTracker{{Status: 2, Seeds: 100, Leeches: 1}}

	pm.processDownloader(fake, "qb1", 5.0, false)
	assert.Equal(t, []string{"pr1"}, fake.pausedIDs)
	var other models.TorrentInfo
	require.NoError(t, db.DB.Where("torrent_id = ?", "other").First(&other).Error)
	assert.False(t, other.IsPausedBySystem)
	assert.Zero(t, other.Seeders, "别的下载器上的记录不更新人数")
}
