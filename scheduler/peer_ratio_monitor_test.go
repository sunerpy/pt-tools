package scheduler

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func newPeerRatioMonitorWithFake(t *testing.T, fake *schedFakeDownloader) (*PeerRatioMonitor, *models.TorrentDB) {
	t.Helper()
	db := setupTestDB(t)
	dm := downloader.NewDownloaderManager()
	registerFakeDownloader(t, dm, fake, true)
	_, err := dm.GetDownloader(fake.name)
	require.NoError(t, err)
	return NewPeerRatioMonitor(db.DB, dm), db
}

// completedManagedTorrent creates a seeding torrent whose hash is tracked as
// completed+pushed in the DB, so filterManagedSeedingTorrents picks it up.
func completedManagedTorrent(t *testing.T, db *models.TorrentDB, id, hash, dlName string) downloader.Torrent {
	t.Helper()
	pushed := true
	require.NoError(t, db.DB.Create(&models.TorrentInfo{
		SiteName: "s", TorrentID: id, TorrentHash: &hash, IsPushed: &pushed,
		IsCompleted: true, DownloaderName: dlName, Title: "T-" + id,
	}).Error)
	return downloader.Torrent{ID: id, InfoHash: hash, Name: "T-" + id, State: downloader.TorrentSeeding}
}

// === processDownloader: pause when S/L ratio exceeds threshold ===

func TestPeerRatio_ProcessDownloader_Pauses(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	pm, db := newPeerRatioMonitorWithFake(t, fake)

	hash := "peerhash1"
	tor := completedManagedTorrent(t, db, "pr1", hash, "qb1")
	fake.torrents = []downloader.Torrent{tor}
	// seeds=100, leeches=1 → ratio 100 > maxSL(5)
	fake.trackers[tor.ID] = []downloader.TorrentTracker{
		{Status: 2, Seeds: 100, Leeches: 1},
	}

	pm.processDownloader(fake, "qb1", 5.0, false)

	assert.Equal(t, []string{"pr1"}, fake.pausedIDs)

	var info models.TorrentInfo
	require.NoError(t, db.DB.Where("torrent_id = ?", "pr1").First(&info).Error)
	assert.True(t, info.IsPausedBySystem)
	assert.Equal(t, PauseReasonPeerRatio, info.PauseReason)
	assert.Equal(t, 100, info.Seeders)
	assert.Equal(t, 1, info.Leechers)
}

// === processDownloader: remove-data mode deletes torrent ===

func TestPeerRatio_ProcessDownloader_RemovesData(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	pm, db := newPeerRatioMonitorWithFake(t, fake)

	hash := "peerhash2"
	tor := completedManagedTorrent(t, db, "pr2", hash, "qb1")
	fake.torrents = []downloader.Torrent{tor}
	fake.trackers[tor.ID] = []downloader.TorrentTracker{{Status: 2, Seeds: 50, Leeches: 0}}

	pm.processDownloader(fake, "qb1", 5.0, true)

	require.Len(t, fake.removedSingle, 1)
	assert.Equal(t, "pr2", fake.removedSingle[0])

	var info models.TorrentInfo
	require.NoError(t, db.DB.Where("torrent_id = ?", "pr2").First(&info).Error)
	assert.True(t, info.IsExpired)
	assert.Equal(t, PauseReasonPeerRatio, info.PauseReason)
}

// === processDownloader: ratio within threshold → no action ===

func TestPeerRatio_ProcessDownloader_WithinThreshold(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	pm, db := newPeerRatioMonitorWithFake(t, fake)

	hash := "peerhash3"
	tor := completedManagedTorrent(t, db, "pr3", hash, "qb1")
	fake.torrents = []downloader.Torrent{tor}
	fake.trackers[tor.ID] = []downloader.TorrentTracker{{Status: 2, Seeds: 3, Leeches: 2}} // 1.5 < 5

	pm.processDownloader(fake, "qb1", 5.0, false)
	assert.Empty(t, fake.pausedIDs)

	// DB peer counts still updated.
	var info models.TorrentInfo
	require.NoError(t, db.DB.Where("torrent_id = ?", "pr3").First(&info).Error)
	assert.Equal(t, 3, info.Seeders)
	assert.Equal(t, 2, info.Leechers)
}

// === processDownloader: seeds==0 → skip (avoid div-by-zero + no action) ===

func TestPeerRatio_ProcessDownloader_ZeroSeeds(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	pm, db := newPeerRatioMonitorWithFake(t, fake)

	hash := "peerhash4"
	tor := completedManagedTorrent(t, db, "pr4", hash, "qb1")
	fake.torrents = []downloader.Torrent{tor}
	fake.trackers[tor.ID] = []downloader.TorrentTracker{{Status: 2, Seeds: 0, Leeches: 5}}

	pm.processDownloader(fake, "qb1", 5.0, false)
	assert.Empty(t, fake.pausedIDs)
}

// === processDownloader: already paused for peer ratio → skip re-pause ===

func TestPeerRatio_ProcessDownloader_AlreadyPaused(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	pm, db := newPeerRatioMonitorWithFake(t, fake)

	hash := "peerhash5"
	pushed := true
	require.NoError(t, db.DB.Create(&models.TorrentInfo{
		SiteName: "s", TorrentID: "pr5", TorrentHash: &hash, IsPushed: &pushed,
		IsCompleted: true, DownloaderName: "qb1", Title: "Already",
		IsPausedBySystem: true, PauseReason: PauseReasonPeerRatio,
	}).Error)
	tor := downloader.Torrent{ID: "pr5", InfoHash: hash, State: downloader.TorrentSeeding}
	fake.torrents = []downloader.Torrent{tor}
	fake.trackers[tor.ID] = []downloader.TorrentTracker{{Status: 2, Seeds: 100, Leeches: 1}}

	pm.processDownloader(fake, "qb1", 5.0, false)
	assert.Empty(t, fake.pausedIDs, "already-paused torrent must not be paused again")
}

// === processDownloader: GetAllTorrents error ===

func TestPeerRatio_ProcessDownloader_GetAllError(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.getAllErr = errors.New("list boom")
	pm, _ := newPeerRatioMonitorWithFake(t, fake)

	require.NotPanics(t, func() { pm.processDownloader(fake, "qb1", 5.0, false) })
}

// === processDownloader: tracker error skips torrent ===

func TestPeerRatio_ProcessDownloader_TrackerError(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.trackersErr = errors.New("tracker boom")
	pm, db := newPeerRatioMonitorWithFake(t, fake)

	hash := "peerhash6"
	tor := completedManagedTorrent(t, db, "pr6", hash, "qb1")
	fake.torrents = []downloader.Torrent{tor}

	pm.processDownloader(fake, "qb1", 5.0, false)
	assert.Empty(t, fake.pausedIDs)
}

// === processDownloader: no managed seeding torrents ===

func TestPeerRatio_ProcessDownloader_NoManaged(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	pm, _ := newPeerRatioMonitorWithFake(t, fake)
	fake.torrents = []downloader.Torrent{
		{ID: "unmanaged", InfoHash: "nope", State: downloader.TorrentSeeding},
	}
	require.NotPanics(t, func() { pm.processDownloader(fake, "qb1", 5.0, false) })
	assert.Empty(t, fake.pausedIDs)
}

// === processDownloader: pause error tolerated (no DB mark) ===

func TestPeerRatio_ProcessDownloader_PauseError(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.pauseErr = errors.New("pause boom")
	pm, db := newPeerRatioMonitorWithFake(t, fake)

	hash := "peerhash7"
	tor := completedManagedTorrent(t, db, "pr7", hash, "qb1")
	fake.torrents = []downloader.Torrent{tor}
	fake.trackers[tor.ID] = []downloader.TorrentTracker{{Status: 2, Seeds: 100, Leeches: 1}}

	pm.processDownloader(fake, "qb1", 5.0, false)

	var info models.TorrentInfo
	require.NoError(t, db.DB.Where("torrent_id = ?", "pr7").First(&info).Error)
	assert.False(t, info.IsPausedBySystem, "pause failure must not mark paused")
}

// === filterManagedSeedingTorrents: only seeding + managed ===

func TestPeerRatio_FilterManagedSeeding(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	pm, db := newPeerRatioMonitorWithFake(t, fake)

	h1 := "fh1"
	completedManagedTorrent(t, db, "m1", h1, "qb1")

	torrents := []downloader.Torrent{
		{ID: "m1", InfoHash: h1, State: downloader.TorrentSeeding},          // managed + seeding
		{ID: "m1d", InfoHash: h1, State: downloader.TorrentDownloading},     // managed but not seeding
		{ID: "un", InfoHash: "unmanaged", State: downloader.TorrentSeeding}, // not managed
	}
	result := pm.filterManagedSeedingTorrents(torrents, "qb1")
	require.Len(t, result, 1)
	assert.Equal(t, "m1", result[0].ID)
}

func TestPeerRatio_FilterManagedSeeding_NoHashes(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	pm, _ := newPeerRatioMonitorWithFake(t, fake)
	result := pm.filterManagedSeedingTorrents(
		[]downloader.Torrent{{ID: "x", State: downloader.TorrentSeeding}}, "qb1",
	)
	assert.Nil(t, result)
}

// === getTrackerPeerCounts: picks max across working trackers, skips inactive ===

func TestPeerRatio_GetTrackerPeerCounts(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	pm, _ := newPeerRatioMonitorWithFake(t, fake)

	fake.trackers["tid"] = []downloader.TorrentTracker{
		{Status: 1, Seeds: 999, Leeches: 999}, // status<2 skipped
		{Status: 2, Seeds: 10, Leeches: 3},
		{Status: 4, Seeds: 20, Leeches: 2},
	}
	seeds, leeches, err := pm.getTrackerPeerCounts(fake, downloader.Torrent{ID: "tid"})
	require.NoError(t, err)
	assert.Equal(t, 20, seeds)
	assert.Equal(t, 3, leeches)
}

// === runOnce: no downloaders → no-op ===

func TestPeerRatio_RunOnce_NoDownloaders(t *testing.T) {
	db := setupTestDB(t)
	pm := NewPeerRatioMonitor(db.DB, downloader.NewDownloaderManager())
	require.NotPanics(t, func() { pm.runOnce(&models.SettingsGlobal{}, 5.0) })
}

// === runOnce: unhealthy skipped ===

func TestPeerRatio_RunOnce_UnhealthySkipped(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.healthy = false
	pm, db := newPeerRatioMonitorWithFake(t, fake)
	hash := "runhash"
	tor := completedManagedTorrent(t, db, "run1", hash, "qb1")
	fake.torrents = []downloader.Torrent{tor}
	fake.trackers[tor.ID] = []downloader.TorrentTracker{{Status: 2, Seeds: 100, Leeches: 1}}

	pm.runOnce(&models.SettingsGlobal{}, 5.0)
	assert.Empty(t, fake.pausedIDs)
}

// === runOnce: healthy → pauses ===

func TestPeerRatio_RunOnce_Healthy(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	pm, db := newPeerRatioMonitorWithFake(t, fake)
	hash := "runhash2"
	tor := completedManagedTorrent(t, db, "run2", hash, "qb1")
	fake.torrents = []downloader.Torrent{tor}
	fake.trackers[tor.ID] = []downloader.TorrentTracker{{Status: 2, Seeds: 100, Leeches: 1}}

	pm.runOnce(&models.SettingsGlobal{PeerRatioRemoveData: false}, 5.0)
	assert.Equal(t, []string{"run2"}, fake.pausedIDs)
}

// === loadConfig ===

func TestPeerRatio_LoadConfig(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.DB.Create(&models.SettingsGlobal{
		DownloadDir: t.TempDir(), PeerRatioEnabled: true, PeerRatioMaxSL: 8,
	}).Error)
	pm := NewPeerRatioMonitor(db.DB, downloader.NewDownloaderManager())
	cfg := pm.loadConfig()
	require.NotNil(t, cfg)
	assert.True(t, cfg.PeerRatioEnabled)
}

// === Start/Stop lifecycle ===

func TestPeerRatio_StartStop(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	pm, _ := newPeerRatioMonitorWithFake(t, fake)

	require.NoError(t, pm.Start())
	require.NoError(t, pm.Start()) // idempotent
	time.Sleep(20 * time.Millisecond)
	pm.Stop()
	require.NotPanics(t, func() { pm.Stop() })
}
