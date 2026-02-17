package scheduler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

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

// === shouldDelete OR mode ===

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

// === shouldDelete AND mode ===

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

// === shouldDelete: no conditions configured ===

func TestShouldDelete_NoConditions(t *testing.T) {
	cm := newTestCleanupMonitor(t)
	cfg := baseCfg()

	t1 := seedingTorrent("1", "aaa", "T1", 100, 5.0)
	assert.False(t, cm.shouldDelete(cfg, t1))
}

// === shouldDelete: free expired ===

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

// === splitProtected: downloading ===

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

// === splitProtected: min retain time ===

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

// === splitProtected: tags ===

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

// === splitProtected: H&R from DB ===

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

// === filterManagedTorrents ===

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

// === calcPriority ===

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

// === emergencyCleanup ===

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

// === helpers ===

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

// === AND/OR edge cases ===

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
