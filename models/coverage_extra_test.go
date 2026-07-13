package models

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func newMemDB(t *testing.T, tables ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(tables...))
	return db
}

// ---------------------------------------------------------------------------
// php_torrent.go metadata getters + free-level + CanbeFinished
// ---------------------------------------------------------------------------

func TestPHPTorrentInfo_MetadataGetters(t *testing.T) {
	p := PHPTorrentInfo{
		Title:    "Some.Movie.2026",
		SubTitle: "中文字幕",
		SizeMB:   2048, // 2 GB
		Discount: DISCOUNT_FREE,
	}
	assert.Equal(t, "Some.Movie.2026", p.GetName())
	assert.Equal(t, "中文字幕", p.GetSubTitle())
	assert.Equal(t, int64(2048*1024*1024), p.GetSizeBytes())
	assert.Equal(t, "free", p.GetFreeLevel())

	// empty discount → "failed"
	pEmpty := PHPTorrentInfo{}
	assert.Equal(t, "failed", pEmpty.GetFreeLevel())
	assert.False(t, pEmpty.IsFree())

	assert.True(t, PHPTorrentInfo{Discount: DISCOUNT_TWO_X_FREE}.IsFree())
}

func TestPHPTorrentInfo_CanbeFinished_Extra(t *testing.T) {
	logger := zap.NewNop().Sugar()

	// size within limit, no speed check → true
	p := PHPTorrentInfo{SizeMB: 1024}
	assert.True(t, p.CanbeFinished(logger, false, 0, 2))

	// size exceeds limit → false
	big := PHPTorrentInfo{SizeMB: 5 * 1024}
	assert.False(t, big.CanbeFinished(logger, false, 0, 2))

	// speed check with plenty of time → true
	future := PHPTorrentInfo{SizeMB: 1, EndTime: time.Now().Add(24 * time.Hour)}
	assert.True(t, future.CanbeFinished(logger, true, 10, 0))

	// speed check but free window already gone → cannot finish
	past := PHPTorrentInfo{SizeMB: 1024 * 1024, EndTime: time.Now().Add(-time.Hour)}
	assert.False(t, past.CanbeFinished(logger, true, 1, 0))
}

func TestPHPTorrentInfo_GetFreeEndTime_Extra(t *testing.T) {
	end := time.Now().Add(time.Hour)
	p := PHPTorrentInfo{EndTime: end}
	got := p.GetFreeEndTime()
	require.NotNil(t, got)
	assert.Equal(t, end, *got)
}

// ---------------------------------------------------------------------------
// resp.go MTTorrentDetail metadata getters + free level + CanbeFinished
// ---------------------------------------------------------------------------

func TestMTTorrentDetail_MetadataGetters(t *testing.T) {
	d := MTTorrentDetail{
		Name:       "Movie CN",
		SmallDescr: "副标题",
		Size:       "1073741824", // 1 GiB
		Status:     &Status{Discount: "FREE"},
	}
	assert.Equal(t, "Movie CN", d.GetName())
	assert.Equal(t, "副标题", d.GetSubTitle())
	assert.Equal(t, int64(1073741824), d.GetSizeBytes())
	assert.Equal(t, "FREE", d.GetFreeLevel())

	// unparsable size → 0
	bad := MTTorrentDetail{Size: "not-a-number"}
	assert.Equal(t, int64(0), bad.GetSizeBytes())

	// no status → "failed"
	assert.Equal(t, "failed", MTTorrentDetail{}.GetFreeLevel())
	// status present but empty discount → "failed"
	assert.Equal(t, "failed", MTTorrentDetail{Status: &Status{}}.GetFreeLevel())
}

func TestMTTorrentDetail_IsFree(t *testing.T) {
	assert.True(t, MTTorrentDetail{Status: &Status{Discount: "free"}}.IsFree())
	assert.True(t, MTTorrentDetail{Status: &Status{PromotionRule: &PromotionRule{Discount: "FREE"}}}.IsFree())
	assert.False(t, MTTorrentDetail{Status: &Status{Discount: "50%"}}.IsFree())
	assert.False(t, MTTorrentDetail{}.IsFree())
}

func TestMTTorrentDetail_CanbeFinished(t *testing.T) {
	logger := zap.NewNop().Sugar()

	// nil status → false
	assert.False(t, MTTorrentDetail{}.CanbeFinished(logger, false, 0, 0))

	// unparsable size → false
	assert.False(t, MTTorrentDetail{Status: &Status{ID: "1"}, Size: "abc"}.CanbeFinished(logger, false, 0, 0))

	// size within limit, no speed check → true
	ok := MTTorrentDetail{Status: &Status{ID: "1"}, Size: "1048576"} // 1 MiB
	assert.True(t, ok.CanbeFinished(logger, false, 0, 5))

	// size exceeds limit → false
	big := MTTorrentDetail{Status: &Status{ID: "1"}, Size: "5368709120"} // 5 GiB
	assert.False(t, big.CanbeFinished(logger, false, 0, 1))

	// speed check but DiscountEndTime empty → false
	noEnd := MTTorrentDetail{Status: &Status{ID: "1", DiscountEndTime: ""}, Size: "1048576"}
	assert.False(t, noEnd.CanbeFinished(logger, true, 10, 0))

	// speed check with bad time format → false
	badTime := MTTorrentDetail{Status: &Status{ID: "1", DiscountEndTime: "not-a-time"}, Size: "1048576"}
	assert.False(t, badTime.CanbeFinished(logger, true, 10, 0))
}

func TestMTTorrentDetail_GetFreeEndTime(t *testing.T) {
	// valid CST time string
	d := MTTorrentDetail{Status: &Status{DiscountEndTime: "2030-01-02 15:04:05"}}
	got := d.GetFreeEndTime()
	require.NotNil(t, got)

	// invalid time string → nil
	bad := MTTorrentDetail{Status: &Status{DiscountEndTime: "bad"}}
	assert.Nil(t, bad.GetFreeEndTime())
}

// ---------------------------------------------------------------------------
// config_models.go effective peer-ratio getters + RSSConfig getters
// ---------------------------------------------------------------------------

func TestSettingsGlobal_GetEffectivePeerRatio(t *testing.T) {
	tests := []struct {
		name        string
		maxSL       float64
		intervalMin int
		wantMaxSL   float64
		wantInt     int
	}{
		{"below min falls back to default", 0.5, 2, DefaultPeerRatioMaxSL, DefaultPeerRatioIntervalMin},
		{"at min kept", MinPeerRatioMaxSL, MinPeerRatioIntervalMin, MinPeerRatioMaxSL, MinPeerRatioIntervalMin},
		{"above min kept", 50.0, 20, 50.0, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := SettingsGlobal{PeerRatioMaxSL: tt.maxSL, PeerRatioIntervalMin: tt.intervalMin}
			assert.Equal(t, tt.wantMaxSL, s.GetEffectivePeerRatioMaxSL())
			assert.Equal(t, tt.wantInt, s.GetEffectivePeerRatioIntervalMin())
		})
	}
}

func TestRSSConfig_GetEffectiveConcurrency_Extra(t *testing.T) {
	global := &SettingsGlobal{DefaultConcurrency: 5}

	// RSS value in range wins
	assert.Equal(t, int32(4), (&RSSConfig{Concurrency: 4}).GetEffectiveConcurrency(global))
	// negative RSS value is not >0 so falls back to default (nil global)
	assert.Equal(t, DefaultConcurrency, (&RSSConfig{Concurrency: -1}).GetEffectiveConcurrency(nil))
	// zero → falls back to global
	assert.Equal(t, int32(5), (&RSSConfig{Concurrency: 0}).GetEffectiveConcurrency(global))
	// RSS above max clamps
	assert.Equal(t, MaxConcurrency, (&RSSConfig{Concurrency: 999}).GetEffectiveConcurrency(global))
	// zero + nil global → default
	assert.Equal(t, DefaultConcurrency, (&RSSConfig{}).GetEffectiveConcurrency(nil))
}

func TestRSSConfig_MiscGetters(t *testing.T) {
	assert.True(t, (&RSSConfig{IsExample: true}).ShouldSkip())
	assert.True(t, (&RSSConfig{URL: ""}).ShouldSkip())
	assert.False(t, (&RSSConfig{URL: "http://x"}).ShouldSkip())

	r := &RSSConfig{DownloadPath: "/dl"}
	assert.Equal(t, "/dl", r.GetEffectiveDownloadPath())
	assert.True(t, r.HasCustomDownloadPath())
	assert.False(t, (&RSSConfig{}).HasCustomDownloadPath())
}

// ---------------------------------------------------------------------------
// migration_state.go
// ---------------------------------------------------------------------------

func TestMigrationState_Lifecycle(t *testing.T) {
	db := newMemDB(t, &MigrationState{})

	// nil db is safe
	_, ok := GetLatestMigrationCompletedAt(nil)
	assert.False(t, ok)
	require.NoError(t, UpsertMigrationState(nil, 1, time.Now()))
	require.NoError(t, MarkBroadcastSent(nil, 1))
	_, ok = GetMigrationState(nil, 1)
	assert.False(t, ok)

	// no rows yet
	_, ok = GetLatestMigrationCompletedAt(db)
	assert.False(t, ok)
	_, ok = GetMigrationState(db, 10)
	assert.False(t, ok)

	// insert
	ts := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, UpsertMigrationState(db, 10, ts))
	got, ok := GetLatestMigrationCompletedAt(db)
	require.True(t, ok)
	assert.Equal(t, ts, got.UTC())

	state, ok := GetMigrationState(db, 10)
	require.True(t, ok)
	assert.Equal(t, 10, state.SchemaVersion)
	assert.False(t, state.BroadcastSent)

	// update existing row (idempotent upsert)
	ts2 := ts.Add(time.Hour)
	require.NoError(t, UpsertMigrationState(db, 10, ts2))
	got, ok = GetLatestMigrationCompletedAt(db)
	require.True(t, ok)
	assert.Equal(t, ts2, got.UTC())

	// mark broadcast sent
	require.NoError(t, MarkBroadcastSent(db, 10))
	state, ok = GetMigrationState(db, 10)
	require.True(t, ok)
	assert.True(t, state.BroadcastSent)
}

// ---------------------------------------------------------------------------
// site_login_state.go additional coverage
// ---------------------------------------------------------------------------

func TestSiteLoginState_ListAndUpdateProbe(t *testing.T) {
	db := newMemDB(t, &SiteLoginState{})
	repo := NewSiteLoginStateRepository(db)

	require.NoError(t, repo.UpsertLoginState("hdsky", map[string]any{
		"LastProbeStatus":    "OK",
		"ProbeJitterSeconds": 30,
		"ProbeMode":          "api",
	}))
	require.NoError(t, repo.UpsertLoginState("mteam", map[string]any{"LastProbeStatus": "FAIL"}))

	all, err := repo.ListLoginStates(false)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	okOnly, err := repo.ListLoginStates(true)
	require.NoError(t, err)
	assert.Len(t, okOnly, 1)
	assert.Equal(t, "hdsky", okOnly[0].SiteName)

	// UpdateProbeResult with login/access times + error
	login := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	access := login.Add(time.Hour)
	require.NoError(t, repo.UpdateProbeResult("hdsky", "OK", &login, &access, assert.AnError))
	st, err := repo.GetLoginState("hdsky")
	require.NoError(t, err)
	require.NotNil(t, st.LastLoginAt)
	require.NotNil(t, st.LastAccessAt)
	assert.Equal(t, assert.AnError.Error(), st.LastProbeError)

	// nil-time, nil-error path clears error message
	require.NoError(t, repo.UpdateProbeResult("hdsky", "OK", nil, nil, nil))
	st, err = repo.GetLoginState("hdsky")
	require.NoError(t, err)
	assert.Empty(t, st.LastProbeError)
}

func TestSiteLoginState_EmptyNameErrors(t *testing.T) {
	db := newMemDB(t, &SiteLoginState{})
	repo := NewSiteLoginStateRepository(db)

	assert.Error(t, repo.UpsertLoginState("", nil))
	_, err := repo.GetLoginState("")
	assert.Error(t, err)
	assert.Error(t, repo.UpdateProbeResult("", "OK", nil, nil, nil))
	assert.Error(t, repo.ClampLastVisit("", time.Now(), time.Now()))
	assert.Error(t, repo.IncrProbeFailures(""))
	assert.Error(t, repo.ResetProbeFailures(""))

	// GetLoginState on missing site → error
	_, err = repo.GetLoginState("does-not-exist")
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// rss_filter_association.go remaining methods
// ---------------------------------------------------------------------------

func TestRSSFilterAssociation_GetFilterRuleIDsForRSS(t *testing.T) {
	db := newMemDB(t, &FilterRule{}, &RSSSubscription{}, &RSSFilterAssociation{})
	assocDB := NewRSSFilterAssociationDB(db)

	require.NoError(t, assocDB.SetFilterRulesForRSS(7, []uint{11, 22}))
	ids, err := assocDB.GetFilterRuleIDsForRSS(7)
	require.NoError(t, err)
	assert.ElementsMatch(t, []uint{11, 22}, ids)

	require.NoError(t, assocDB.DeleteByRSSID(7))
	ids, err = assocDB.GetFilterRuleIDsForRSS(7)
	require.NoError(t, err)
	assert.Empty(t, ids)
}

// ---------------------------------------------------------------------------
// site_repository.go remaining downloader methods + not-found paths
// ---------------------------------------------------------------------------

func TestSiteRepository_UpdateDownloaderMethods(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &RSSSubscription{})
	repo := NewSiteRepository(db)

	id, err := repo.CreateSite(SiteData{Name: "s1", AuthMethod: "cookie"})
	require.NoError(t, err)

	dlID := uint(42)
	require.NoError(t, repo.UpdateSiteDownloader("s1", &dlID))
	site, err := repo.GetSiteByName("s1")
	require.NoError(t, err)
	require.NotNil(t, site.DownloaderID)
	assert.Equal(t, dlID, *site.DownloaderID)

	dlID2 := uint(99)
	require.NoError(t, repo.UpdateSiteDownloaderByID(id, &dlID2))
	site, err = repo.GetSiteByID(id)
	require.NoError(t, err)
	require.NotNil(t, site.DownloaderID)
	assert.Equal(t, dlID2, *site.DownloaderID)

	// clear downloader
	require.NoError(t, repo.UpdateSiteDownloader("s1", nil))
	site, _ = repo.GetSiteByName("s1")
	assert.Nil(t, site.DownloaderID)

	// empty batch → 0 rows, no error
	rows, err := repo.BatchUpdateSiteDownloader(nil, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(0), rows)
}

func TestSiteRepository_NotFoundPaths(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &RSSSubscription{})
	repo := NewSiteRepository(db)

	_, err := repo.GetSiteByName("nope")
	assert.Error(t, err)
	_, err = repo.GetSiteByID(12345)
	assert.Error(t, err)

	exists, err := repo.SiteExistsByName("nope")
	require.NoError(t, err)
	assert.False(t, exists)

	// CreateSite empty auth method
	_, err = repo.CreateSite(SiteData{Name: "x"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "认证方式不能为空")
}

// ---------------------------------------------------------------------------
// presets.go: migrateCmctToSpringSunday merge branch + MigrateExampleRSS
// ---------------------------------------------------------------------------

func TestMigrateCmctToSpringSunday_MergeExisting(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &RSSSubscription{})

	// both cmct and springsunday exist; springsunday lacks cookie/apikey/enabled
	cmct := SiteSetting{Name: "cmct", AuthMethod: "cookie", Cookie: "ck", APIKey: "ak", Enabled: true}
	require.NoError(t, db.Create(&cmct).Error)
	spring := SiteSetting{Name: "springsunday", AuthMethod: "cookie"}
	require.NoError(t, db.Create(&spring).Error)

	// an RSS attached to cmct should be reparented to springsunday
	require.NoError(t, db.Create(&RSSSubscription{SiteID: cmct.ID, Name: "r", URL: "http://x", IntervalMinutes: 5}).Error)

	require.NoError(t, SyncSitesFromRegistry(db, []RegisteredSite{
		{ID: "springsunday", Name: "SpringSunday", AuthMethod: "cookie"},
	}))

	// cmct removed, springsunday inherited user config
	var cnt int64
	db.Model(&SiteSetting{}).Where("name = ?", "cmct").Count(&cnt)
	assert.Equal(t, int64(0), cnt)

	var merged SiteSetting
	require.NoError(t, db.Where("name = ?", "springsunday").First(&merged).Error)
	assert.Equal(t, "ck", merged.Cookie)
	assert.Equal(t, "ak", merged.APIKey)
	assert.True(t, merged.Enabled)

	// RSS reparented
	var rss RSSSubscription
	require.NoError(t, db.Where("name = ?", "r").First(&rss).Error)
	assert.Equal(t, merged.ID, rss.SiteID)
}

func TestMigrateExampleRSS(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &RSSSubscription{})

	require.NoError(t, db.Create(&RSSSubscription{Name: "example", URL: "https://example.com/rss", IntervalMinutes: 5}).Error)
	require.NoError(t, db.Create(&RSSSubscription{Name: "real", URL: "https://hdsky.me/rss", IntervalMinutes: 5}).Error)

	require.NoError(t, MigrateExampleRSS(db))

	var ex RSSSubscription
	require.NoError(t, db.Where("name = ?", "example").First(&ex).Error)
	assert.True(t, ex.IsExample)

	var real RSSSubscription
	require.NoError(t, db.Where("name = ?", "real").First(&real).Error)
	assert.False(t, real.IsExample)
}

// ---------------------------------------------------------------------------
// schema_version.go: exercise the simple ALTER-based migrators directly
// ---------------------------------------------------------------------------

func TestMigrators_V5V6V7V8(t *testing.T) {
	db := newMemDB(t, &RSSSubscription{}, &FilterRule{}, &NotificationConf{}, &RSSNotificationLog{})

	// v5→v6 is idempotent when columns already exist (AutoMigrate created them)
	require.NoError(t, migrateV5ToV6(db))
	assert.True(t, db.Migrator().HasTable(&RSSNotificationLog{}))

	// v6→v7 backfills purpose; column already present via AutoMigrate
	require.NoError(t, migrateV6ToV7(db))

	// v7→v8 adds quiet-hours columns; idempotent when present
	require.NoError(t, migrateV7ToV8(db))
}

func TestMigrators_V6V7_NoTables(t *testing.T) {
	// FilterRule / NotificationConf tables absent → migrators no-op cleanly
	db := newMemDB(t, &RSSSubscription{})
	require.NoError(t, migrateV6ToV7(db))
	require.NoError(t, migrateV7ToV8(db))
}

func TestMigrateV4ToV5_NoLegacyTable(t *testing.T) {
	db := newMemDB(t, &SiteSetting{})
	// no dynamic_site_settings table → sets defaults only
	require.NoError(t, db.Create(&SiteSetting{Name: "abc", AuthMethod: "cookie"}).Error)
	require.NoError(t, migrateV4ToV5(db))

	var site SiteSetting
	require.NoError(t, db.Where("name = ?", "abc").First(&site).Error)
	assert.Equal(t, "abc", site.DisplayName) // defaulted from Name
	assert.True(t, site.IsBuiltin)
}

func TestMigrateV2ToV3_CreatesUserInfo(t *testing.T) {
	db := newMemDB(t, &SiteSetting{})
	assert.False(t, db.Migrator().HasTable("user_info_records"))
	require.NoError(t, migrateV2ToV3(db))
	assert.True(t, db.Migrator().HasTable("user_info_records"))
}

func TestMigrateV3ToV4_Noop(t *testing.T) {
	db := newMemDB(t, &SiteSetting{})
	require.NoError(t, migrateV3ToV4(db))
}

// ---------------------------------------------------------------------------
// init.go: WithTransactionContext + GetTorrentBySiteAndID not-found
// ---------------------------------------------------------------------------

func TestTorrentDB_WithTransactionContext(t *testing.T) {
	db := newMemDB(t, &TorrentInfo{})
	tdb := &TorrentDB{DB: db}

	require.NoError(t, tdb.WithTransactionContext(context.Background(), func(tx *gorm.DB) error {
		return tx.Create(&TorrentInfo{SiteName: "s", TorrentID: "t"}).Error
	}))

	got, err := tdb.GetTorrentBySiteAndID("s", "t")
	require.NoError(t, err)
	require.NotNil(t, got)

	// missing → nil, nil
	miss, err := tdb.GetTorrentBySiteAndID("s", "missing")
	require.NoError(t, err)
	assert.Nil(t, miss)

	missHash, err := tdb.GetTorrentBySiteAndHash("s", "nohash")
	require.NoError(t, err)
	assert.Nil(t, missHash)
}
