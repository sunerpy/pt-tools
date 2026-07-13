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

// newCleanupMonitorWithFake builds a CleanupMonitor whose DownloaderManager has
// a single registered fake downloader (default). Returns the monitor and fake
// so tests can inspect recorded RemoveTorrents/PauseTorrent side effects.
func newCleanupMonitorWithFake(t *testing.T, fake *schedFakeDownloader) *CleanupMonitor {
	t.Helper()
	db := setupTestDB(t)
	dm := downloader.NewDownloaderManager()
	registerFakeDownloader(t, dm, fake, true)
	// Force instantiation so ListDownloaders + GetDownloader return the fake.
	_, err := dm.GetDownloader(fake.name)
	require.NoError(t, err)
	return NewCleanupMonitor(db.DB, dm)
}

// === runOnce / processDownloader happy path: deletes matching torrents ===

func TestProcessDownloader_DeletesAndUpdatesDB(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)

	cfg := baseCfg()
	cfg.CleanupScope = "all"
	cfg.CleanupMaxSeedTimeH = 72
	cfg.CleanupProtectDL = false
	cfg.CleanupProtectHR = false

	// One torrent past seed-time threshold, one below.
	del := seedingTorrent("del1", "hashdelete", "OldSeed", 100, 1.0)
	keep := seedingTorrent("keep1", "hashkeep", "Young", 10, 1.0)
	fake.torrents = []downloader.Torrent{del, keep}

	// DB row so updateDatabase has something to mark expired.
	hash := "hashdelete"
	pushed := true
	require.NoError(t, cm.db.Create(&models.TorrentInfo{
		SiteName: "s", TorrentID: "t1", TorrentHash: &hash, IsPushed: &pushed,
		DownloaderName: "qb1", Title: "OldSeed",
	}).Error)

	cm.processDownloader(cfg, fake, "qb1")

	require.Len(t, fake.removedBatch, 1)
	assert.Equal(t, []string{"del1"}, fake.removedBatch[0])

	var info models.TorrentInfo
	require.NoError(t, cm.db.Where("torrent_id = ?", "t1").First(&info).Error)
	assert.True(t, info.IsExpired, "deleted torrent should be marked expired in DB")
}

// === processDownloader: nothing matches → no delete call ===

func TestProcessDownloader_NoCandidatesNoDelete(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)

	cfg := baseCfg()
	cfg.CleanupScope = "all"
	cfg.CleanupMaxSeedTimeH = 72
	fake.torrents = []downloader.Torrent{seedingTorrent("y", "h", "Young", 1, 0.1)}

	cm.processDownloader(cfg, fake, "qb1")
	assert.Empty(t, fake.removedBatch)
}

// === processDownloader: managed scope with no DB rows → empty managed set ===

func TestProcessDownloader_ManagedScope_Empty(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)

	cfg := baseCfg()
	cfg.CleanupScope = "database"
	fake.torrents = []downloader.Torrent{seedingTorrent("x", "h", "X", 100, 5)}

	cm.processDownloader(cfg, fake, "qb1")
	assert.Empty(t, fake.removedBatch, "no DB-managed hashes → nothing deleted")
}

// === processDownloader: GetAllTorrents error path ===

func TestProcessDownloader_GetAllError(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.getAllErr = errors.New("boom")
	cm := newCleanupMonitorWithFake(t, fake)

	cfg := baseCfg()
	require.NotPanics(t, func() { cm.processDownloader(cfg, fake, "qb1") })
	assert.Empty(t, fake.removedBatch)
}

// === processDownloader: skips torrents already paused for peer ratio ===

func TestProcessDownloader_SkipsPeerRatioPaused(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)

	cfg := baseCfg()
	cfg.CleanupScope = "all"
	cfg.CleanupMaxSeedTimeH = 72
	cfg.CleanupProtectHR = false

	hash := "peerpausedhash"
	tor := seedingTorrent("p1", hash, "PeerPaused", 100, 1.0)
	fake.torrents = []downloader.Torrent{tor}

	// Mark it paused-for-peer-ratio in DB so isPausedForPeerRatio returns true.
	pushed := true
	require.NoError(t, cm.db.Create(&models.TorrentInfo{
		SiteName: "s", TorrentID: "p1", TorrentHash: &hash, IsPushed: &pushed,
		IsPausedBySystem: true, PauseReason: PauseReasonPeerRatio, Title: "PeerPaused",
	}).Error)

	cm.processDownloader(cfg, fake, "qb1")
	assert.Empty(t, fake.removedBatch, "peer-ratio-paused torrent must be skipped")
}

// === processDownloader: RemoveTorrents error is tolerated (no DB update) ===

func TestProcessDownloader_RemoveError(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.removeErr = errors.New("remove failed")
	cm := newCleanupMonitorWithFake(t, fake)

	cfg := baseCfg()
	cfg.CleanupScope = "all"
	cfg.CleanupMaxSeedTimeH = 72
	cfg.CleanupProtectHR = false

	hash := "failhash"
	pushed := true
	require.NoError(t, cm.db.Create(&models.TorrentInfo{
		SiteName: "s", TorrentID: "f1", TorrentHash: &hash, IsPushed: &pushed,
		DownloaderName: "qb1", Title: "Fail",
	}).Error)
	fake.torrents = []downloader.Torrent{seedingTorrent("f1", hash, "Fail", 100, 1.0)}

	cm.processDownloader(cfg, fake, "qb1")

	var info models.TorrentInfo
	require.NoError(t, cm.db.Where("torrent_id = ?", "f1").First(&info).Error)
	assert.False(t, info.IsExpired, "remove failure must not mark expired")
}

// === processDownloader: emergency cleanup path when disk low ===

func TestProcessDownloader_EmergencyCleanupWhenDiskLow(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)

	cfg := baseCfg()
	cfg.CleanupScope = "all"
	cfg.CleanupProtectHR = false
	cfg.CleanupProtectDL = false
	cfg.CleanupDiskProtect = true
	cfg.CleanupMinDiskSpaceGB = 100
	// No rule matches (no seed-time/ratio), so toDelete starts empty; the disk
	// being low forces emergencyCleanup to pick candidates by priority.
	fake.diskInfo = downloader.DiskInfo{FreeSpace: 10 * oneGB} // 10GB < 100GB

	big := seedingTorrent("big", "hbig", "Big", 10, 1.0)
	big.TotalSize = 200 * oneGB
	fake.torrents = []downloader.Torrent{big}

	cm.processDownloader(cfg, fake, "qb1")
	require.Len(t, fake.removedBatch, 1)
	assert.Contains(t, fake.removedBatch[0], "big")
}

// === runOnce: no downloaders → no-op ===

func TestRunOnce_NoDownloaders(t *testing.T) {
	db := setupTestDB(t)
	cm := NewCleanupMonitor(db.DB, downloader.NewDownloaderManager())
	require.NotPanics(t, func() { cm.runOnce(baseCfg()) })
}

// === runOnce: unhealthy downloader skipped ===

func TestRunOnce_UnhealthySkipped(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.healthy = false
	cm := newCleanupMonitorWithFake(t, fake)

	cfg := baseCfg()
	cfg.CleanupScope = "all"
	cfg.CleanupMaxSeedTimeH = 1
	fake.torrents = []downloader.Torrent{seedingTorrent("x", "h", "X", 100, 5)}

	cm.runOnce(cfg)
	assert.Empty(t, fake.removedBatch, "unhealthy downloader must be skipped")
}

// === runOnce: healthy path deletes ===

func TestRunOnce_HealthyDeletes(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)

	cfg := baseCfg()
	cfg.CleanupScope = "all"
	cfg.CleanupMaxSeedTimeH = 1
	cfg.CleanupProtectHR = false
	cfg.CleanupProtectDL = false
	fake.torrents = []downloader.Torrent{seedingTorrent("d", "h", "D", 100, 5)}

	cm.runOnce(cfg)
	require.Len(t, fake.removedBatch, 1)
}

// === RunManual ===

func TestRunManual_NoConfig(t *testing.T) {
	db := setupTestDB(t)
	cm := NewCleanupMonitor(db.DB, downloader.NewDownloaderManager())
	_, err := cm.RunManual()
	require.Error(t, err)
}

func TestRunManual_Disabled(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.DB.Create(&models.SettingsGlobal{
		DownloadDir: t.TempDir(), CleanupEnabled: false,
	}).Error)
	cm := NewCleanupMonitor(db.DB, downloader.NewDownloaderManager())
	_, err := cm.RunManual()
	require.Error(t, err)
}

func TestRunManual_Enabled(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	db := setupTestDB(t)
	dm := downloader.NewDownloaderManager()
	registerFakeDownloader(t, dm, fake, true)
	_, err := dm.GetDownloader(fake.name)
	require.NoError(t, err)
	cm := NewCleanupMonitor(db.DB, dm)

	require.NoError(t, db.DB.Create(&models.SettingsGlobal{
		DownloadDir: t.TempDir(), CleanupEnabled: true, CleanupScope: "all",
		CleanupMaxSeedTimeH: 1,
	}).Error)
	fake.torrents = []downloader.Torrent{seedingTorrent("d", "h", "D", 100, 5)}

	n, err := cm.RunManual()
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	require.Len(t, fake.removedBatch, 1)
}

// === updateDatabase directly ===

func TestUpdateDatabase_MarksExpired(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	hash := "updatehash"
	pushed := true
	require.NoError(t, cm.db.Create(&models.TorrentInfo{
		SiteName: "s", TorrentID: "u1", TorrentHash: &hash, IsPushed: &pushed,
		DownloaderName: "qb1", Title: "U",
	}).Error)

	cm.updateDatabase([]downloader.Torrent{{ID: "u1", InfoHash: hash}}, "qb1")

	var info models.TorrentInfo
	require.NoError(t, cm.db.Where("torrent_id = ?", "u1").First(&info).Error)
	assert.True(t, info.IsExpired)
	require.NotNil(t, info.LastCheckTime)
}

// === isPausedForPeerRatio ===

func TestIsPausedForPeerRatio(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	hash := "prhash"
	pushed := true
	require.NoError(t, cm.db.Create(&models.TorrentInfo{
		SiteName: "s", TorrentID: "pr1", TorrentHash: &hash, IsPushed: &pushed,
		IsPausedBySystem: true, PauseReason: PauseReasonPeerRatio, Title: "PR",
	}).Error)

	assert.True(t, cm.isPausedForPeerRatio(hash))
	assert.False(t, cm.isPausedForPeerRatio("nonexistent"))
}

// === getManagedHashes includes archive rows ===

func TestGetManagedHashes_IncludesArchive(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	pushed := true

	liveHash := "livehash"
	require.NoError(t, cm.db.Create(&models.TorrentInfo{
		SiteName: "s", TorrentID: "l1", TorrentHash: &liveHash, IsPushed: &pushed,
		DownloaderName: "qb1", Title: "Live",
	}).Error)

	archHash := "archivehash"
	require.NoError(t, cm.db.Create(&models.TorrentInfoArchive{
		OriginalID: 999, SiteName: "s", TorrentID: "a1", TorrentHash: &archHash,
		IsPushed: &pushed, DownloaderName: "qb1", Title: "Arch",
	}).Error)

	hashes := cm.getManagedHashes("qb1")
	_, hasLive := hashes["livehash"]
	_, hasArch := hashes["archivehash"]
	assert.True(t, hasLive)
	assert.True(t, hasArch)
}

// === getHRInfoMap: reads HasHR rows ===

func TestGetHRInfoMap_ReadsHRRows(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	hash := "HRHASHUPPER"
	require.NoError(t, cm.db.Create(&models.TorrentInfo{
		SiteName: "s", TorrentID: "hr1", TorrentHash: &hash, HasHR: true,
		HRSeedTimeH: 120, Title: "HR",
	}).Error)

	m := cm.getHRInfoMap()
	info, ok := m["hrhashupper"]
	require.True(t, ok, "hash should be lowercased in map")
	assert.True(t, info.HasHR)
	assert.Equal(t, 120, info.HRSeedTimeH)
}

// === maybeResetDiskBudget + resetDiskBudget already covered by issue374 tests;
// add a Start/Stop lifecycle smoke test that does not depend on the 10s sleep. ===

func TestCleanupMonitor_StartStop(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)

	require.NoError(t, cm.Start())
	// double start is a no-op
	require.NoError(t, cm.Start())

	// give the goroutine a moment; it sleeps 10s before doing anything so no work happens
	time.Sleep(20 * time.Millisecond)

	cm.Stop()
	// double stop safe
	require.NotPanics(t, func() { cm.Stop() })
}
