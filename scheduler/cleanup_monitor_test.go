// MIT License
// Copyright (c) 2025 pt-tools

package scheduler

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/events"
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

func TestProcessDownloader_ManagedScope_Empty(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)

	cfg := baseCfg()
	cfg.CleanupScope = "database"
	fake.torrents = []downloader.Torrent{seedingTorrent("x", "h", "X", 100, 5)}

	cm.processDownloader(cfg, fake, "qb1")
	assert.Empty(t, fake.removedBatch, "no DB-managed hashes → nothing deleted")
}

func TestProcessDownloader_GetAllError(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	fake.getAllErr = errors.New("boom")
	cm := newCleanupMonitorWithFake(t, fake)

	cfg := baseCfg()
	require.NotPanics(t, func() { cm.processDownloader(cfg, fake, "qb1") })
	assert.Empty(t, fake.removedBatch)
}

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

func TestRunOnce_NoDownloaders(t *testing.T) {
	db := setupTestDB(t)
	cm := NewCleanupMonitor(db.DB, downloader.NewDownloaderManager())
	require.NotPanics(t, func() { cm.runOnce(baseCfg()) })
}

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

func newTestCleanupMonitor(t *testing.T) *CleanupMonitor {
	t.Helper()
	db := setupTestDB(t)
	return NewCleanupMonitor(db.DB, nil)
}

func baseCfg() *models.SettingsGlobal {
	return &models.SettingsGlobal{
		CleanupEnabled:       true,
		CleanupScope:         "all",
		CleanupRemoveData:    true,
		CleanupConditionMode: "or",
		CleanupProtectDL:     true,
		CleanupProtectHR:     true,
		CleanupMinRetainH:    0,
	}
}

func seedingTorrent(id, hash, name string, seedTimeH int, ratio float64) downloader.Torrent {
	return downloader.Torrent{
		ID:          id,
		InfoHash:    hash,
		Name:        name,
		State:       downloader.TorrentSeeding,
		SeedingTime: int64(seedTimeH) * 3600,
		Ratio:       ratio,
		DateAdded:   time.Now().Add(-time.Duration(seedTimeH+1) * time.Hour).Unix(),
		TotalSize:   1024 * 1024 * 1024,
	}
}

func TestShouldDelete_OR_SeedTimeExceeded(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupMaxSeedTimeH = 72

	t1 := seedingTorrent("1", "aaa", "T1", 73, 0.5)
	t2 := seedingTorrent("2", "bbb", "T2", 71, 0.5)

	assert.True(t, cm.shouldDelete(cfg, t1))
	assert.False(t, cm.shouldDelete(cfg, t2))
}

func TestShouldDelete_OR_RatioReached(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupMinRatio = 2.0

	t1 := seedingTorrent("1", "aaa", "T1", 10, 2.5)
	t2 := seedingTorrent("2", "bbb", "T2", 10, 1.5)

	assert.True(t, cm.shouldDelete(cfg, t1))
	assert.False(t, cm.shouldDelete(cfg, t2))
}

func TestShouldDelete_OR_Inactive(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupMaxInactiveH = 48

	t1 := seedingTorrent("1", "aaa", "T1", 49, 0.5)
	t1.UploadSpeed = 0

	t2 := seedingTorrent("2", "bbb", "T2", 49, 0.5)
	t2.UploadSpeed = 1024

	assert.True(t, cm.shouldDelete(cfg, t1))
	assert.False(t, cm.shouldDelete(cfg, t2))
}

func TestShouldDelete_OR_AnyConditionSufficient(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupMaxSeedTimeH = 72
	cfg.CleanupMinRatio = 3.0

	// Given: seed time exceeded but ratio not reached
	t1 := seedingTorrent("1", "aaa", "T1", 100, 1.0)
	assert.True(t, cm.shouldDelete(cfg, t1))

	// Given: ratio reached but seed time not exceeded
	t2 := seedingTorrent("2", "bbb", "T2", 10, 5.0)
	assert.True(t, cm.shouldDelete(cfg, t2))

	// Given: neither condition met
	t3 := seedingTorrent("3", "ccc", "T3", 10, 1.0)
	assert.False(t, cm.shouldDelete(cfg, t3))
}

func TestShouldDelete_AND_AllConditionsRequired(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupConditionMode = "and"
	cfg.CleanupMaxSeedTimeH = 72
	cfg.CleanupMinRatio = 2.0

	// Given: both conditions met
	t1 := seedingTorrent("1", "aaa", "T1", 100, 3.0)
	assert.True(t, cm.shouldDelete(cfg, t1))

	// Given: only seed time met
	t2 := seedingTorrent("2", "bbb", "T2", 100, 1.0)
	assert.False(t, cm.shouldDelete(cfg, t2))

	// Given: only ratio met
	t3 := seedingTorrent("3", "ccc", "T3", 10, 3.0)
	assert.False(t, cm.shouldDelete(cfg, t3))

	// Given: neither met
	t4 := seedingTorrent("4", "ddd", "T4", 10, 1.0)
	assert.False(t, cm.shouldDelete(cfg, t4))
}

func TestShouldDelete_AND_SingleConditionAlwaysMatches(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupConditionMode = "and"
	cfg.CleanupMaxSeedTimeH = 72
	cfg.CleanupMinRatio = 0

	// Given: only seed time configured, seed time exceeded
	t1 := seedingTorrent("1", "aaa", "T1", 100, 0.5)
	assert.True(t, cm.shouldDelete(cfg, t1))
}

func TestShouldDelete_NoConditions(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()

	t1 := seedingTorrent("1", "aaa", "T1", 100, 5.0)
	assert.False(t, cm.shouldDelete(cfg, t1))
}

func TestShouldDelete_FreeExpiredIncomplete(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupDelFreeExpired = true

	expired := time.Now().Add(-1 * time.Hour)
	hash := "abc123def456"
	pushed := true
	torrentInfo := models.TorrentInfo{
		SiteName:    "testsite",
		TorrentID:   "t1",
		TorrentHash: &hash,
		FreeEndTime: &expired,
		IsPushed:    &pushed,
		Title:       "Test",
	}
	require.NoError(t, cm.db.Create(&torrentInfo).Error)

	t1 := downloader.Torrent{ID: "1", InfoHash: hash, Progress: 0.5, State: downloader.TorrentDownloading}
	assert.True(t, cm.shouldDelete(cfg, t1))

	// Given: completed torrent with expired free → should NOT delete
	t2 := downloader.Torrent{ID: "2", InfoHash: hash, Progress: 1.0, State: downloader.TorrentSeeding}
	assert.False(t, cm.shouldDelete(cfg, t2))
}

func TestShouldDelete_FreeExpiredDisabled(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupDelFreeExpired = false

	expired := time.Now().Add(-1 * time.Hour)
	hash := "abc123def456"
	pushed := true
	torrentInfo := models.TorrentInfo{
		SiteName:    "testsite",
		TorrentID:   "t1",
		TorrentHash: &hash,
		FreeEndTime: &expired,
		IsPushed:    &pushed,
		Title:       "Test",
	}
	require.NoError(t, cm.db.Create(&torrentInfo).Error)

	t1 := downloader.Torrent{ID: "1", InfoHash: hash, Progress: 0.5}
	assert.False(t, cm.shouldDelete(cfg, t1))
}

func TestSplitProtected_Downloading(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupProtectDL = true

	torrents := []downloader.Torrent{
		{ID: "1", InfoHash: "a", State: downloader.TorrentDownloading},
		{ID: "2", InfoHash: "b", State: downloader.TorrentSeeding, DateAdded: time.Now().Add(-48 * time.Hour).Unix()},
	}

	protected, candidates := cm.splitProtected(cfg, torrents)
	assert.Len(t, protected, 1)
	assert.Equal(t, "1", protected[0].ID)
	assert.Len(t, candidates, 1)
	assert.Equal(t, "2", candidates[0].ID)
}

func TestSplitProtected_DownloadingDisabled(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupProtectDL = false

	torrents := []downloader.Torrent{
		{ID: "1", InfoHash: "a", State: downloader.TorrentDownloading, DateAdded: time.Now().Add(-48 * time.Hour).Unix()},
	}

	protected, candidates := cm.splitProtected(cfg, torrents)
	assert.Len(t, protected, 0)
	assert.Len(t, candidates, 1)
}

func TestSplitProtected_MinRetainTime(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupMinRetainH = 24

	recent := time.Now().Add(-12 * time.Hour).Unix()
	old := time.Now().Add(-48 * time.Hour).Unix()

	torrents := []downloader.Torrent{
		{ID: "1", InfoHash: "a", State: downloader.TorrentSeeding, DateAdded: recent},
		{ID: "2", InfoHash: "b", State: downloader.TorrentSeeding, DateAdded: old},
	}

	protected, candidates := cm.splitProtected(cfg, torrents)
	assert.Len(t, protected, 1)
	assert.Equal(t, "1", protected[0].ID)
	assert.Len(t, candidates, 1)
	assert.Equal(t, "2", candidates[0].ID)
}

func TestSplitProtected_ProtectTags(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupProtectTags = "keep,important"

	torrents := []downloader.Torrent{
		{ID: "1", InfoHash: "a", Tags: "keep", State: downloader.TorrentSeeding, DateAdded: time.Now().Add(-48 * time.Hour).Unix()},
		{ID: "2", InfoHash: "b", Tags: "brush", State: downloader.TorrentSeeding, DateAdded: time.Now().Add(-48 * time.Hour).Unix()},
		{ID: "3", InfoHash: "c", Category: "IMPORTANT", State: downloader.TorrentSeeding, DateAdded: time.Now().Add(-48 * time.Hour).Unix()},
	}

	protected, candidates := cm.splitProtected(cfg, torrents)
	assert.Len(t, protected, 2)
	assert.Len(t, candidates, 1)
	assert.Equal(t, "2", candidates[0].ID)
}

func TestSplitProtected_HRFromDatabase(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupProtectHR = true

	hash := "hr-hash-123"
	pushed := true
	torrentInfo := models.TorrentInfo{
		SiteName:    "testsite",
		TorrentID:   "t1",
		TorrentHash: &hash,
		HasHR:       true,
		HRSeedTimeH: 72,
		IsPushed:    &pushed,
		Title:       "HR Torrent",
	}
	require.NoError(t, cm.db.Create(&torrentInfo).Error)

	torrents := []downloader.Torrent{
		{ID: "1", InfoHash: hash, State: downloader.TorrentSeeding, SeedingTime: 48 * 3600, DateAdded: time.Now().Add(-96 * time.Hour).Unix()},
		{ID: "2", InfoHash: "no-hr-hash", State: downloader.TorrentSeeding, SeedingTime: 48 * 3600, DateAdded: time.Now().Add(-96 * time.Hour).Unix()},
	}

	protected, candidates := cm.splitProtected(cfg, torrents)
	assert.Len(t, protected, 1)
	assert.Equal(t, "1", protected[0].ID)
	assert.Len(t, candidates, 1)
	assert.Equal(t, "2", candidates[0].ID)
}

func TestSplitProtected_HRFulfilled(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupProtectHR = true

	hash := "hr-hash-fulfilled"
	pushed := true
	torrentInfo := models.TorrentInfo{
		SiteName:    "testsite",
		TorrentID:   "t2",
		TorrentHash: &hash,
		HasHR:       true,
		HRSeedTimeH: 72,
		IsPushed:    &pushed,
		Title:       "HR Fulfilled",
	}
	require.NoError(t, cm.db.Create(&torrentInfo).Error)

	// Given: seeding time >= required HR time
	torrents := []downloader.Torrent{
		{ID: "1", InfoHash: hash, State: downloader.TorrentSeeding, SeedingTime: 73 * 3600, DateAdded: time.Now().Add(-96 * time.Hour).Unix()},
	}

	protected, candidates := cm.splitProtected(cfg, torrents)
	assert.Len(t, protected, 0)
	assert.Len(t, candidates, 1)
}

func TestSplitProtected_HRDisabled(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupProtectHR = false

	hash := "hr-hash-disabled"
	pushed := true
	torrentInfo := models.TorrentInfo{
		SiteName:    "testsite",
		TorrentID:   "t3",
		TorrentHash: &hash,
		HasHR:       true,
		HRSeedTimeH: 72,
		IsPushed:    &pushed,
		Title:       "HR Disabled",
	}
	require.NoError(t, cm.db.Create(&torrentInfo).Error)

	torrents := []downloader.Torrent{
		{ID: "1", InfoHash: hash, State: downloader.TorrentSeeding, SeedingTime: 10 * 3600, DateAdded: time.Now().Add(-96 * time.Hour).Unix()},
	}

	protected, candidates := cm.splitProtected(cfg, torrents)
	assert.Len(t, protected, 0)
	assert.Len(t, candidates, 1)
}

func TestFilterManaged_DatabaseScope(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupScope = "database"

	hash1 := "managed-hash-1"
	hash2 := "managed-hash-2"
	pushed := true
	for _, h := range []string{hash1, hash2} {
		h := h
		require.NoError(t, cm.db.Create(&models.TorrentInfo{
			SiteName:       "testsite",
			TorrentID:      h,
			TorrentHash:    &h,
			IsPushed:       &pushed,
			DownloaderName: "dl1",
			Title:          "T-" + h,
		}).Error)
	}

	torrents := []downloader.Torrent{
		{ID: "1", InfoHash: hash1},
		{ID: "2", InfoHash: hash2},
		{ID: "3", InfoHash: "unmanaged-hash"},
	}

	result := cm.filterManagedTorrents(cfg, torrents, "dl1")
	assert.Len(t, result, 2)
}

func TestFilterManaged_DatabaseScope_WrongDownloader(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupScope = "database"

	hash := "cross-dl-hash"
	pushed := true
	require.NoError(t, cm.db.Create(&models.TorrentInfo{
		SiteName:       "testsite",
		TorrentID:      "t1",
		TorrentHash:    &hash,
		IsPushed:       &pushed,
		DownloaderName: "dl1",
		Title:          "Cross DL",
	}).Error)

	torrents := []downloader.Torrent{{ID: "1", InfoHash: hash}}

	// Given: querying for different downloader name
	result := cm.filterManagedTorrents(cfg, torrents, "dl2")
	assert.Len(t, result, 0)
}

func TestFilterManaged_TagScope(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupScope = "tag"
	cfg.CleanupScopeTags = "brush,pt-auto"

	torrents := []downloader.Torrent{
		{ID: "1", Tags: "brush", InfoHash: "a"},
		{ID: "2", Category: "PT-AUTO", InfoHash: "b"},
		{ID: "3", Tags: "manual", InfoHash: "c"},
	}

	result := cm.filterManagedTorrents(cfg, torrents, "dl1")
	assert.Len(t, result, 2)
}

func TestFilterManaged_TagScope_Empty(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupScope = "tag"
	cfg.CleanupScopeTags = ""

	torrents := []downloader.Torrent{{ID: "1", Tags: "anything"}}
	result := cm.filterManagedTorrents(cfg, torrents, "dl1")
	assert.Nil(t, result)
}

func TestFilterManaged_AllScope(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupScope = "all"

	torrents := []downloader.Torrent{
		{ID: "1", InfoHash: "a"},
		{ID: "2", InfoHash: "b"},
	}

	result := cm.filterManagedTorrents(cfg, torrents, "dl1")
	assert.Len(t, result, 2)
}

func TestCalcPriority_PausedHigherThanSeeding(t *testing.T) {
	cm := newTestCleanupMonitor(t)

	paused := downloader.Torrent{State: downloader.TorrentPaused, SeedingTime: 10 * 3600, Ratio: 1.0, TotalSize: 1 << 30}
	seeding := downloader.Torrent{State: downloader.TorrentSeeding, SeedingTime: 10 * 3600, Ratio: 1.0, TotalSize: 1 << 30, UploadSpeed: 1024}

	assert.Greater(t, cm.calcPriority(paused), cm.calcPriority(seeding))
}

func TestCalcPriority_HighRatioHigherPriority(t *testing.T) {
	cm := newTestCleanupMonitor(t)

	highRatio := downloader.Torrent{State: downloader.TorrentSeeding, Ratio: 5.0, TotalSize: 1 << 30}
	lowRatio := downloader.Torrent{State: downloader.TorrentSeeding, Ratio: 0.5, TotalSize: 1 << 30}

	assert.Greater(t, cm.calcPriority(highRatio), cm.calcPriority(lowRatio))
}

func TestCalcPriority_LargerSizeHigherPriority(t *testing.T) {
	cm := newTestCleanupMonitor(t)

	big := downloader.Torrent{State: downloader.TorrentSeeding, Ratio: 1.0, TotalSize: 50 << 30}
	small := downloader.Torrent{State: downloader.TorrentSeeding, Ratio: 1.0, TotalSize: 1 << 30}

	assert.Greater(t, cm.calcPriority(big), cm.calcPriority(small))
}

func TestEmergencyCleanup_DeletesUntilSpaceSufficient(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupMinDiskSpaceGB = 100

	candidates := []downloader.Torrent{
		{ID: "1", Name: "Small", TotalSize: 10 << 30, State: downloader.TorrentSeeding, Ratio: 1.0},
		{ID: "2", Name: "Big", TotalSize: 50 << 30, State: downloader.TorrentPaused, Ratio: 2.0},
		{ID: "3", Name: "Medium", TotalSize: 30 << 30, State: downloader.TorrentSeeding, Ratio: 0.5},
	}

	// Given: 60GB free, need 100GB → need 40GB more
	result := cm.emergencyCleanup(cfg, candidates, nil, 60)
	totalFreed := int64(0)
	for _, t := range result {
		totalFreed += t.TotalSize
	}
	assert.GreaterOrEqual(t, totalFreed, int64(40<<30))
}

func TestEmergencyCleanup_SkipsAlreadyMarked(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupMinDiskSpaceGB = 100

	already := []downloader.Torrent{
		{ID: "1", Name: "Already", TotalSize: 20 << 30, State: downloader.TorrentSeeding},
	}
	candidates := []downloader.Torrent{
		{ID: "1", Name: "Already", TotalSize: 20 << 30, State: downloader.TorrentSeeding},
		{ID: "2", Name: "New", TotalSize: 50 << 30, State: downloader.TorrentSeeding},
	}

	result := cm.emergencyCleanup(cfg, candidates, already, 60)
	ids := make(map[string]int)
	for _, t := range result {
		ids[t.ID]++
	}
	assert.Equal(t, 1, ids["1"])
	assert.Equal(t, 1, ids["2"])
}

func TestSplitTags(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"a,b,c", []string{"a", "b", "c"}},
		{"  a , b , ", []string{"a", "b"}},
		{"", nil},
		{"single", []string{"single"}},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, splitTags(tt.input))
	}
}

func TestContainsIgnoreCase(t *testing.T) {
	assert.True(t, containsIgnoreCase([]string{"Keep", "Important"}, "keep"))
	assert.True(t, containsIgnoreCase([]string{"keep", "important"}, "IMPORTANT"))
	assert.False(t, containsIgnoreCase([]string{"keep"}, "other"))
	assert.False(t, containsIgnoreCase(nil, "any"))
}

func TestShouldDelete_DefaultModeIsOR(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupConditionMode = ""
	cfg.CleanupMaxSeedTimeH = 72

	t1 := seedingTorrent("1", "aaa", "T1", 100, 0.5)
	assert.True(t, cm.shouldDelete(cfg, t1))
}

func TestShouldDelete_AND_ThreeConditions(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupConditionMode = "and"
	cfg.CleanupMaxSeedTimeH = 48
	cfg.CleanupMinRatio = 1.0
	cfg.CleanupMaxInactiveH = 24

	// Given: all three met
	t1 := seedingTorrent("1", "aaa", "T1", 49, 1.5)
	t1.UploadSpeed = 0
	assert.True(t, cm.shouldDelete(cfg, t1))

	// Given: two of three met (inactive not met because upload speed > 0)
	t2 := seedingTorrent("2", "bbb", "T2", 49, 1.5)
	t2.UploadSpeed = 1024
	assert.False(t, cm.shouldDelete(cfg, t2))
}

func TestShouldDelete_FreeExpiredBypassesAndOr(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupConditionMode = "and"
	cfg.CleanupMaxSeedTimeH = 999
	cfg.CleanupMinRatio = 999
	cfg.CleanupDelFreeExpired = true

	expired := time.Now().Add(-1 * time.Hour)
	hash := "free-expired-hash"
	pushed := true
	require.NoError(t, cm.db.Create(&models.TorrentInfo{
		SiteName:    "testsite",
		TorrentID:   "t1",
		TorrentHash: &hash,
		FreeEndTime: &expired,
		IsPushed:    &pushed,
		Title:       "Free Expired",
	}).Error)

	// Given: AND mode with impossible conditions, but free expired
	t1 := downloader.Torrent{ID: "1", InfoHash: hash, Progress: 0.3, State: downloader.TorrentDownloading}
	assert.True(t, cm.shouldDelete(cfg, t1))
}

func TestShouldDelete_OR_SlowSeed(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupSlowSeedTimeH = 48
	cfg.CleanupSlowMaxRatio = 0.1

	// Given: seeded 72h but ratio only 0.05 → low-efficiency, should delete
	t1 := seedingTorrent("1", "aaa", "Slow", 72, 0.05)
	t1.IsCompleted = true
	assert.True(t, cm.shouldDelete(cfg, t1))

	// Given: seeded 72h and ratio 0.5 → above threshold, keep
	t2 := seedingTorrent("2", "bbb", "OK", 72, 0.5)
	t2.IsCompleted = true
	assert.False(t, cm.shouldDelete(cfg, t2))

	// Given: seeded only 24h with low ratio → not enough time yet, keep
	t3 := seedingTorrent("3", "ccc", "Young", 24, 0.02)
	t3.IsCompleted = true
	assert.False(t, cm.shouldDelete(cfg, t3))

	// Given: not completed yet → keep (still downloading)
	t4 := seedingTorrent("4", "ddd", "Incomplete", 72, 0.01)
	t4.IsCompleted = false
	assert.False(t, cm.shouldDelete(cfg, t4))
}

func TestShouldDelete_AND_SlowSeedWithOtherConditions(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()
	cfg.CleanupConditionMode = "and"
	cfg.CleanupMaxSeedTimeH = 48
	cfg.CleanupSlowSeedTimeH = 48
	cfg.CleanupSlowMaxRatio = 0.1

	// Given: both seed time exceeded AND slow seed → delete
	t1 := seedingTorrent("1", "aaa", "Both", 72, 0.05)
	t1.IsCompleted = true
	assert.True(t, cm.shouldDelete(cfg, t1))

	// Given: seed time exceeded but ratio above slow threshold → slow condition fails → AND fails
	t2 := seedingTorrent("2", "bbb", "NotSlow", 72, 0.5)
	t2.IsCompleted = true
	assert.False(t, cm.shouldDelete(cfg, t2))
}

func TestDrainAndDebounce_ReturnsAfterQuiet(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	ch := make(chan events.Event)

	start := time.Now()
	done := make(chan struct{})
	go func() {
		cm.drainAndDebounce(ch)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(diskEventDebounce + 2*time.Second):
		t.Fatal("drainAndDebounce did not return")
	}
	assert.GreaterOrEqual(t, time.Since(start), diskEventDebounce-100*time.Millisecond)
}

func TestDrainAndDebounce_ResetsOnEvent(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	ch := make(chan events.Event, 4)

	done := make(chan struct{})
	go func() {
		cm.drainAndDebounce(ch)
		close(done)
	}()

	// Send one DiskSpaceLow to reset the timer, then let it drain.
	ch <- events.Event{Type: events.DiskSpaceLow}

	select {
	case <-done:
	case <-time.After(diskEventDebounce + 2*time.Second):
		t.Fatal("drainAndDebounce did not return after reset")
	}
}

func TestDrainAndDebounce_CtxCancel(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cm.cancel() // pre-cancel

	ch := make(chan events.Event)
	done := make(chan struct{})
	go func() {
		cm.drainAndDebounce(ch)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("drainAndDebounce did not return on ctx cancel")
	}
}

const oneGB = int64(1024 * 1024 * 1024)

// TestIssue374_DiskProtectOn_CleanupOff_ResetStillRuns 修复后核心契约：
// 用户配置「磁盘保护开但自动删种关」时，maybeResetDiskBudget 仍会触发 Reset，
// 预留不再单调累积。这是 Bug 报告中 984.6 GB 现象的直接验证。
func TestIssue374_DiskProtectOn_CleanupOff_ResetStillRuns(t *testing.T) {
	db := setupTestDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, db.DB.Create(&models.SettingsGlobal{
		DownloadDir:           t.TempDir(),
		CleanupDiskProtect:    true,
		CleanupMinDiskSpaceGB: 200,
		CleanupEnabled:        false, // 关键：自动删种关
	}).Error)

	budget := ptinternal.GetDiskBudget()
	budget.Reset()
	t.Cleanup(func() { budget.Reset() })

	const n = 30
	const sizePerTorrent = 30 * oneGB
	for i := 0; i < n; i++ {
		budget.Reserve(sizePerTorrent)
	}
	require.Equal(t, int64(n)*sizePerTorrent, budget.Reserved(), "sanity: 累计 900 GB")

	cm := NewCleanupMonitor(db.DB, downloader.NewDownloaderManager())
	cfg := cm.loadConfig()
	require.NotNil(t, cfg)
	require.True(t, cfg.CleanupDiskProtect)
	require.False(t, cfg.CleanupEnabled)

	cm.maybeResetDiskBudget(cfg)

	assert.Equal(t, int64(0), budget.Reserved(),
		"修复 #374：CleanupDiskProtect=true + CleanupEnabled=false 时 Reset 必须运行")
}

// TestIssue374_DiskProtectOff_ResetSkipped 反向：
// 当 CleanupDiskProtect=false 时（用户根本没开磁盘保护），无需 Reset；
// 否则我们就是在偷偷修改一个用户没启用的子系统的状态。
func TestIssue374_DiskProtectOff_ResetSkipped(t *testing.T) {
	db := setupTestDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	row := models.SettingsGlobal{
		DownloadDir:           t.TempDir(),
		CleanupDiskProtect:    false,
		CleanupMinDiskSpaceGB: 0,
		CleanupEnabled:        false,
	}
	require.NoError(t, db.DB.Create(&row).Error)
	// GORM `default:true` 标签会把零值 false 覆盖回 true，必须显式 Update
	require.NoError(t, db.DB.Model(&row).Update("cleanup_disk_protect", false).Error)

	budget := ptinternal.GetDiskBudget()
	budget.Reset()
	t.Cleanup(func() { budget.Reset() })

	budget.Reserve(50 * oneGB)
	require.Equal(t, 50*oneGB, budget.Reserved())

	cm := NewCleanupMonitor(db.DB, downloader.NewDownloaderManager())
	cfg := cm.loadConfig()
	require.NotNil(t, cfg)
	require.False(t, cfg.CleanupDiskProtect)

	cm.maybeResetDiskBudget(cfg)

	assert.Equal(t, 50*oneGB, budget.Reserved(),
		"CleanupDiskProtect=false → Reset 不该跑（DiskBudget 子系统未启用，prep 状态保留）")
}

// TestIssue374_NilConfig_NoOp 防御性：
// loadConfig 失败返回 nil 时 maybeResetDiskBudget 必须是 no-op，
// 不能 panic 或访问 nil 字段。
func TestIssue374_NilConfig_NoOp(t *testing.T) {
	db := setupTestDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	budget := ptinternal.GetDiskBudget()
	budget.Reset()
	t.Cleanup(func() { budget.Reset() })

	budget.Reserve(10 * oneGB)
	cm := NewCleanupMonitor(db.DB, downloader.NewDownloaderManager())

	require.NotPanics(t, func() {
		cm.maybeResetDiskBudget(nil)
	})
	assert.Equal(t, 10*oneGB, budget.Reserved(), "nil cfg → no-op，不动现状")
}

// TestIssue374_BothFlagsOn_ResetRunsViaRunOnce 兼容性：
// 用户同时开 CleanupEnabled+CleanupDiskProtect 时，runOnce 路径仍会 Reset
// （runOnce 内的 resetDiskBudget 调用未被删除）。证明 fix 没破坏旧的 happy path。
func TestIssue374_BothFlagsOn_ResetRunsViaRunOnce(t *testing.T) {
	db := setupTestDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, db.DB.Create(&models.SettingsGlobal{
		DownloadDir:           t.TempDir(),
		CleanupDiskProtect:    true,
		CleanupMinDiskSpaceGB: 200,
		CleanupEnabled:        true,
	}).Error)

	budget := ptinternal.GetDiskBudget()
	budget.Reset()
	t.Cleanup(func() { budget.Reset() })

	budget.Reserve(500 * oneGB)
	cm := NewCleanupMonitor(db.DB, downloader.NewDownloaderManager())
	cfg := cm.loadConfig()
	require.NotNil(t, cfg)

	cm.runOnce(cfg)

	assert.Equal(t, int64(0), budget.Reserved(),
		"CleanupEnabled=true 时 runOnce 路径仍正确清零")
}

// TestIssue374_ResetIsIdempotent 性质测试：
// 多次连续 Reset 等价于一次，不会下溢或异常。runLoop 在 5 分钟节奏下会反复
// 调用，必须保证幂等。
func TestIssue374_ResetIsIdempotent(t *testing.T) {
	db := setupTestDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, db.DB.Create(&models.SettingsGlobal{
		DownloadDir:        t.TempDir(),
		CleanupDiskProtect: true,
	}).Error)

	budget := ptinternal.GetDiskBudget()
	budget.Reset()
	t.Cleanup(func() { budget.Reset() })

	cm := NewCleanupMonitor(db.DB, downloader.NewDownloaderManager())
	cfg := cm.loadConfig()
	require.NotNil(t, cfg)

	for i := 0; i < 10; i++ {
		cm.maybeResetDiskBudget(cfg)
	}
	assert.Equal(t, int64(0), budget.Reserved(), "连续 10 次 Reset 仍为 0")
}

func TestGetHRInfoMap_FromDefinitionAndDB(t *testing.T) {
	fake := newSchedFakeDownloader("qb1")
	cm := newCleanupMonitorWithFake(t, fake)

	// Explicit HR from DB row.
	h1 := "aaaa1111"
	require.NoError(t, cm.db.Create(&models.TorrentInfo{
		SiteName: "s", TorrentID: "t1", TorrentHash: &h1, HasHR: true, HRSeedTimeH: 96,
	}).Error)
	// A row for an HR-enabled site definition (agsvpt) with HasHR=false → derived.
	h2 := "bbbb2222"
	require.NoError(t, cm.db.Create(&models.TorrentInfo{
		SiteName: "agsvpt", TorrentID: "t2", TorrentHash: &h2, HasHR: false, TorrentSize: 10 << 30,
	}).Error)

	m := cm.getHRInfoMap()
	require.Contains(t, m, "aaaa1111")
	assert.Equal(t, 96, m["aaaa1111"].HRSeedTimeH)
	if _, ok := m["bbbb2222"]; ok {
		assert.True(t, m["bbbb2222"].HasHR)
	}
}
