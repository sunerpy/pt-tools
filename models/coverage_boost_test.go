package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"moul.io/zapgorm2"
)

func zapNopGormLogger() zapgorm2.Logger {
	return zapgorm2.Logger{ZapLogger: zap.NewNop()}
}

func zapNopLogger() *zap.SugaredLogger { return zap.NewNop().Sugar() }

func cstLoc() *time.Location { return time.FixedZone("CST", 8*3600) }

func closedDB(t *testing.T, tables ...any) *gorm.DB {
	t.Helper()
	db := newMemDB(t, tables...)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	return db
}

func TestErrorPaths_ClosedDB(t *testing.T) {
	db := closedDB(t, &SiteSetting{}, &RSSSubscription{}, &FilterRule{},
		&RSSFilterAssociation{}, &SiteLoginState{})

	siteRepo := NewSiteRepository(db)
	_, err := siteRepo.ListSites()
	assert.Error(t, err)
	_, err = siteRepo.ListEnabledSites()
	assert.Error(t, err)
	_, err = siteRepo.SiteExistsByName("x")
	assert.Error(t, err)
	_, err = siteRepo.CreateSite(SiteData{Name: "n", AuthMethod: "cookie"})
	assert.Error(t, err)

	assocDB := NewRSSFilterAssociationDB(db)
	_, err = assocDB.GetByRSSID(1)
	assert.Error(t, err)
	_, err = assocDB.GetByFilterRuleID(1)
	assert.Error(t, err)
	assert.Error(t, assocDB.SetFilterRulesForRSS(1, []uint{2}))

	loginRepo := NewSiteLoginStateRepository(db)
	assert.Error(t, loginRepo.UpsertLoginState("s", map[string]any{"LastProbeStatus": "OK"}))
	_, err = loginRepo.GetLoginState("s")
	assert.Error(t, err)
	_, err = loginRepo.ListLoginStates(false)
	assert.Error(t, err)

	assert.Error(t, MigrateExampleRSS(db))
	assert.Error(t, SyncSitesFromRegistry(db, []RegisteredSite{{ID: "x", AuthMethod: "cookie"}}))
	assert.Error(t, setDefaultsForSiteSetting(db))
	assert.Error(t, migrateV1ToV2(db))

	assert.Error(t, loginRepo.UpdateProbeResult("s", "OK", nil, nil, nil))
	assert.Error(t, loginRepo.ClampLastVisit("s", time.Now(), time.Now()))
	assert.Error(t, loginRepo.IncrProbeFailures("s"))
	assert.Error(t, loginRepo.ResetProbeFailures("s"))
	assert.Error(t, assocDB.DeleteByRSSID(1))
	assert.Error(t, assocDB.DeleteByFilterRuleID(1))
	_, err = assocDB.HasAssociations(1)
	assert.Error(t, err)
	_, err = assocDB.Exists(1, 2)
	assert.Error(t, err)

	sm := NewSchemaManager(db, "1.0.0")
	assert.Error(t, sm.RunMigrations())
	_, err = sm.isLegacyDatabase()
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// schema_version.go: migrateV4ToV5 with a populated legacy table (create + merge)
// ---------------------------------------------------------------------------

type dynamicSiteSettingLegacyRow struct {
	ID           uint   `gorm:"primaryKey"`
	Name         string `gorm:"uniqueIndex;size:64;not null"`
	DisplayName  string `gorm:"size:128"`
	BaseURL      string `gorm:"size:512"`
	Enabled      bool
	AuthMethod   string `gorm:"size:16;not null"`
	Cookie       string `gorm:"size:2048"`
	APIKey       string `gorm:"size:512"`
	APIURL       string `gorm:"size:512"`
	DownloaderID *uint  `gorm:"index"`
	ParserConfig string `gorm:"type:text"`
	IsBuiltin    bool
	TemplateID   *uint `gorm:"index"`
}

func (dynamicSiteSettingLegacyRow) TableName() string { return "dynamic_site_settings" }

func TestMigrateV4ToV5_CreateAndMerge(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &dynamicSiteSettingLegacyRow{})

	dlID := uint(7)
	require.NoError(t, db.Create(&dynamicSiteSettingLegacyRow{
		Name: "newsite", DisplayName: "New Site", BaseURL: "https://new.example",
		Enabled: true, AuthMethod: "cookie", Cookie: "ck", APIKey: "ak",
		APIURL: "https://api.new", DownloaderID: &dlID, ParserConfig: "{}", IsBuiltin: true,
	}).Error)
	tmpl := uint(3)
	require.NoError(t, db.Create(&dynamicSiteSettingLegacyRow{
		Name: "existing", DisplayName: "Legacy Name", BaseURL: "https://legacy",
		AuthMethod: "cookie", Cookie: "legacyck", APIKey: "legacyak", APIURL: "https://legacy-api",
		DownloaderID: &tmpl, ParserConfig: "{\"a\":1}", TemplateID: &tmpl, IsBuiltin: true,
	}).Error)

	require.NoError(t, db.Create(&SiteSetting{Name: "existing", AuthMethod: "cookie"}).Error)

	require.NoError(t, migrateV4ToV5(db))

	assert.False(t, db.Migrator().HasTable("dynamic_site_settings"))

	var created SiteSetting
	require.NoError(t, db.Where("name = ?", "newsite").First(&created).Error)
	assert.Equal(t, "New Site", created.DisplayName)
	assert.Equal(t, "ck", created.Cookie)
	require.NotNil(t, created.DownloaderID)
	assert.Equal(t, uint(7), *created.DownloaderID)

	var merged SiteSetting
	require.NoError(t, db.Where("name = ?", "existing").First(&merged).Error)
	assert.Equal(t, "Legacy Name", merged.DisplayName)
	assert.Equal(t, "legacyck", merged.Cookie)
	assert.Equal(t, "https://legacy-api", merged.APIUrl)
	assert.Equal(t, "{\"a\":1}", merged.ParserConfig)
	require.NotNil(t, merged.DownloaderID)
	require.NotNil(t, merged.TemplateID)
	assert.True(t, merged.IsBuiltin)
}

func TestMigrateV4ToV5_EmptyLegacyTable(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &dynamicSiteSettingLegacyRow{})
	require.NoError(t, db.Create(&SiteSetting{Name: "s1", AuthMethod: "cookie"}).Error)

	require.NoError(t, migrateV4ToV5(db))

	assert.False(t, db.Migrator().HasTable("dynamic_site_settings"))
	var s SiteSetting
	require.NoError(t, db.Where("name = ?", "s1").First(&s).Error)
	assert.Equal(t, "s1", s.DisplayName)
	assert.True(t, s.IsBuiltin)
}

// ---------------------------------------------------------------------------
// schema_version.go: migrateV5ToV6 / V6ToV7 ALTER path (columns absent)
// ---------------------------------------------------------------------------

func TestMigrateV5ToV6_AddsColumns(t *testing.T) {
	db := newMemDB(t)
	require.NoError(t, db.Exec("CREATE TABLE rss_subscriptions (id INTEGER PRIMARY KEY)").Error)

	require.NoError(t, migrateV5ToV6(db))

	assert.True(t, db.Migrator().HasColumn(&RSSSubscription{}, "NotifyMode"))
	assert.True(t, db.Migrator().HasColumn(&RSSSubscription{}, "NotifyConfIDs"))
	assert.True(t, db.Migrator().HasColumn(&RSSSubscription{}, "MaxNotificationsPerHour"))
	assert.True(t, db.Migrator().HasTable(&RSSNotificationLog{}))
}

func TestMigrateV6ToV7_AddsPurposeColumn(t *testing.T) {
	db := newMemDB(t)
	require.NoError(t, db.Exec("CREATE TABLE filter_rules (id INTEGER PRIMARY KEY)").Error)

	require.NoError(t, migrateV6ToV7(db))

	assert.True(t, db.Migrator().HasColumn(&FilterRule{}, "Purpose"))
}

func TestMigrateV7ToV8_AddsQuietHours(t *testing.T) {
	db := newMemDB(t)
	require.NoError(t, db.Exec("CREATE TABLE notification_conf (id INTEGER PRIMARY KEY)").Error)

	require.NoError(t, migrateV7ToV8(db))

	assert.True(t, db.Migrator().HasColumn(&NotificationConf{}, "QuietHoursStart"))
	assert.True(t, db.Migrator().HasColumn(&NotificationConf{}, "QuietHoursEnd"))
}

// ---------------------------------------------------------------------------
// schema_version.go: RunMigrations full fresh-install + GetCurrentVersion
// ---------------------------------------------------------------------------

func TestRunMigrations_LegacyChainV1ToV10(t *testing.T) {
	db := newMemDB(
		t,
		&SchemaVersion{}, &SettingsGlobal{}, &SiteSetting{}, &RSSSubscription{},
		&FilterRule{}, &NotificationConf{},
	)

	require.NoError(t, db.Create(&SiteSetting{Name: "hdsky", AuthMethod: "cookie", Cookie: "uid=1;pass=2", Enabled: true}).Error)
	require.NoError(t, db.Create(&RSSSubscription{Name: "ex", URL: "https://example.com/rss", IntervalMinutes: 5}).Error)

	backup := func(*gorm.DB, string) (string, error) { return "b", nil }
	enc := func(p string) (string, error) { return "enc:" + p, nil }
	dec := func(c string) (string, error) { return c[4:], nil }

	sm := NewSchemaManagerWithHooks(db, "1.0.0", backup, enc, dec)
	require.NoError(t, sm.RunMigrations())

	got, err := sm.GetCurrentVersion()
	require.NoError(t, err)
	assert.Equal(t, CurrentSchemaVersion, got)

	assert.True(t, db.Migrator().HasColumn(&SiteSetting{}, "CookieEncrypted"))
	assert.True(t, db.Migrator().HasTable(&SiteLoginState{}))
	assert.True(t, db.Migrator().HasColumn(&SiteLoginState{}, "ApiLastLoginAt"))

	var cipher string
	require.NoError(t, db.Model(&SiteSetting{}).Where("name = ?", "hdsky").Pluck("cookie_encrypted", &cipher).Error)
	assert.Equal(t, "enc:uid=1;pass=2", cipher)

	var rss RSSSubscription
	require.NoError(t, db.Where("name = ?", "ex").First(&rss).Error)
	assert.True(t, rss.IsExample)

	_, ok := GetMigrationState(db, 10)
	assert.True(t, ok)
}

func TestRunMigrations_FreshInstall(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &SettingsGlobal{}, &RSSSubscription{})
	sm := NewSchemaManager(db, "1.2.3")

	require.NoError(t, sm.RunMigrations())

	got, err := sm.GetCurrentVersion()
	require.NoError(t, err)
	assert.Equal(t, CurrentSchemaVersion, got)
}

func TestGetCurrentVersion_NoTable(t *testing.T) {
	db := newMemDB(t)
	sm := NewSchemaManager(db, "1.0.0")

	got, err := sm.GetCurrentVersion()
	require.NoError(t, err)
	assert.Equal(t, 0, got)
}

func TestSchemaManager_RecordAndReadVersion(t *testing.T) {
	db := newMemDB(t)
	sm := NewSchemaManager(db, "9.9.9")
	require.NoError(t, sm.EnsureSchemaVersionTable())
	require.NoError(t, sm.RecordVersion(3, "test v3"))

	got, err := sm.GetCurrentVersion()
	require.NoError(t, err)
	assert.Equal(t, 3, got)
}

// ---------------------------------------------------------------------------
// presets.go: migrateCmctToSpringSunday rename branch + torrent_infos rename
// ---------------------------------------------------------------------------

func TestMigrateCmctToSpringSunday_RenameBranch(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &RSSSubscription{}, &TorrentInfo{})

	cmct := SiteSetting{Name: "cmct", AuthMethod: "cookie", Cookie: "ck", Enabled: true}
	require.NoError(t, db.Create(&cmct).Error)
	require.NoError(t, db.Create(&TorrentInfo{SiteName: "cmct", TorrentID: "t1"}).Error)

	require.NoError(t, SyncSitesFromRegistry(db, nil))

	var spring SiteSetting
	require.NoError(t, db.Where("name = ?", "springsunday").First(&spring).Error)
	assert.Equal(t, "ck", spring.Cookie)
	assert.True(t, spring.Enabled)

	var cnt int64
	db.Model(&SiteSetting{}).Where("name = ?", "cmct").Count(&cnt)
	assert.Equal(t, int64(0), cnt)

	var ti TorrentInfo
	require.NoError(t, db.Where("torrent_id = ?", "t1").First(&ti).Error)
	assert.Equal(t, "springsunday", ti.SiteName)
}

func TestSyncSitesFromRegistry_CreateAndUpdate(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &RSSSubscription{})

	require.NoError(t, db.Create(&SiteSetting{Name: "mteam", AuthMethod: "cookie", Cookie: "userck", Enabled: true}).Error)

	require.NoError(t, SyncSitesFromRegistry(db, []RegisteredSite{
		{ID: "MTEAM", Name: "M-Team", AuthMethod: "api_key", DefaultBaseURL: "https://kp.m-team.cc", APIUrls: []string{"https://api.m-team.cc", "https://api2.m-team.cc"}},
		{ID: "hdsky", Name: "HDSky", AuthMethod: "cookie", DefaultBaseURL: "https://hdsky.me"},
	}))

	var mteam SiteSetting
	require.NoError(t, db.Where("name = ?", "mteam").First(&mteam).Error)
	assert.Equal(t, "api_key", mteam.AuthMethod)
	assert.Equal(t, "userck", mteam.Cookie)
	assert.True(t, mteam.Enabled)
	assert.Equal(t, "https://api.m-team.cc", mteam.APIUrl)
	assert.Contains(t, mteam.APIUrls, "api2.m-team.cc")

	var hdsky SiteSetting
	require.NoError(t, db.Where("name = ?", "hdsky").First(&hdsky).Error)
	assert.False(t, hdsky.Enabled)
	assert.True(t, hdsky.IsBuiltin)
}

// ---------------------------------------------------------------------------
// site_login_state.go: Incr/Reset/Clamp remaining branches
// ---------------------------------------------------------------------------

func TestSiteLoginState_IncrResetClamp(t *testing.T) {
	db := newMemDB(t, &SiteLoginState{})
	repo := NewSiteLoginStateRepository(db)

	require.NoError(t, repo.UpsertLoginState("hdsky", map[string]any{
		"BanThresholdDays":       45,
		"RemindBeforeDays":       7,
		"ReminderCron":           "0 8 * * *",
		"NotificationChannelIDs": "[1,2,3]",
	}))

	require.NoError(t, repo.IncrProbeFailures("hdsky"))
	require.NoError(t, repo.IncrProbeFailures("hdsky"))
	st, err := repo.GetLoginState("hdsky")
	require.NoError(t, err)
	assert.Equal(t, 2, st.ConsecutiveProbeFailures)
	assert.Equal(t, 45, st.BanThresholdDays)
	assert.Equal(t, "0 8 * * *", st.ReminderCron)
	assert.Equal(t, "[1,2,3]", st.NotificationChannelIDs)

	require.NoError(t, repo.ResetProbeFailures("hdsky"))
	st, err = repo.GetLoginState("hdsky")
	require.NoError(t, err)
	assert.Equal(t, 0, st.ConsecutiveProbeFailures)

	now := time.Now()
	future := now.Add(24 * time.Hour)
	require.NoError(t, repo.ClampLastVisit("hdsky", future, now))
	st, err = repo.GetLoginState("hdsky")
	require.NoError(t, err)
	require.NotNil(t, st.LastVisitAt)
	assert.WithinDuration(t, now, *st.LastVisitAt, 2*time.Second)

	past := now.Add(-time.Hour)
	require.NoError(t, repo.ClampLastVisit("hdsky", past, now))
	st, err = repo.GetLoginState("hdsky")
	require.NoError(t, err)
	assert.WithinDuration(t, past, *st.LastVisitAt, 2*time.Second)
}

// ---------------------------------------------------------------------------
// site_repository.go: list + credential update paths
// ---------------------------------------------------------------------------

func TestSiteRepository_BatchUpdateAndByID(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &RSSSubscription{})
	repo := NewSiteRepository(db)

	id1, err := repo.CreateSite(SiteData{Name: "a", AuthMethod: "cookie"})
	require.NoError(t, err)
	id2, err := repo.CreateSite(SiteData{Name: "b", AuthMethod: "cookie"})
	require.NoError(t, err)

	require.NoError(t, db.Create(&RSSSubscription{Name: "r", URL: "http://x", IntervalMinutes: 5, SiteID: id1}).Error)

	rows, err := repo.BatchUpdateSiteDownloader([]uint{id1, id2}, 99)
	require.NoError(t, err)
	assert.Equal(t, int64(2), rows)

	site, err := repo.GetSiteByID(id1)
	require.NoError(t, err)
	require.NotNil(t, site.DownloaderID)
	assert.Equal(t, uint(99), *site.DownloaderID)

	var rss RSSSubscription
	require.NoError(t, db.Where("name = ?", "r").First(&rss).Error)
	require.NotNil(t, rss.DownloaderID)
	assert.Equal(t, uint(99), *rss.DownloaderID)
}

func TestSchemaManager_IsLegacyByRSSOnly(t *testing.T) {
	db := newMemDB(t, &SchemaVersion{}, &SettingsGlobal{}, &SiteSetting{}, &RSSSubscription{},
		&FilterRule{}, &NotificationConf{})
	require.NoError(t, db.Create(&RSSSubscription{Name: "r", URL: "http://x", IntervalMinutes: 5}).Error)

	backup := func(*gorm.DB, string) (string, error) { return "b", nil }
	enc := func(p string) (string, error) { return "e:" + p, nil }
	dec := func(c string) (string, error) { return c[2:], nil }
	sm := NewSchemaManagerWithHooks(db, "1.0.0", backup, enc, dec)

	require.NoError(t, sm.RunMigrations())
	got, err := sm.GetCurrentVersion()
	require.NoError(t, err)
	assert.Equal(t, CurrentSchemaVersion, got)
}

func TestMigrateV8ToV9_NilHooks(t *testing.T) {
	db := newMemDB(t, &SiteSetting{})
	sm := NewSchemaManager(db, "1.0.0")
	assert.Error(t, sm.migrateV8ToV9(db))

	sm.BackupTable = func(*gorm.DB, string) (string, error) { return "b", nil }
	assert.Error(t, sm.migrateV8ToV9(db))
}

func TestMigrateV8ToV9_DecryptSanityMismatch(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &SiteLoginState{})
	require.NoError(t, db.Create(&SiteSetting{Name: "s", AuthMethod: "cookie", Cookie: "raw"}).Error)

	sm := NewSchemaManagerWithHooks(
		db, "1.0.0",
		func(*gorm.DB, string) (string, error) { return "b", nil },
		func(p string) (string, error) { return "e:" + p, nil },
		func(string) (string, error) { return "TAMPERED", nil },
	)
	err := sm.migrateV8ToV9(db)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sanity")
}

func TestMigrateV9ToV10_NilBackupHook(t *testing.T) {
	db := newMemDB(t, &SiteLoginState{})
	sm := NewSchemaManager(db, "1.0.0")
	assert.Error(t, sm.migrateV9ToV10(db))
}

func TestMigrateV2ToV3_UserInfoTableExists(t *testing.T) {
	db := newMemDB(t, &SiteSetting{})
	require.NoError(t, db.Exec("CREATE TABLE user_info (id INTEGER PRIMARY KEY)").Error)
	require.NoError(t, migrateV2ToV3(db))
	assert.True(t, db.Migrator().HasTable("user_info"))
}

func TestMigrateV8ToV9_IdempotentColumnPresent(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &SiteLoginState{})
	require.NoError(t, db.Create(&SiteSetting{Name: "s", AuthMethod: "cookie", Cookie: "raw"}).Error)

	backup := func(*gorm.DB, string) (string, error) { return "b", nil }
	enc := func(p string) (string, error) { return "e:" + p, nil }
	dec := func(c string) (string, error) { return c[2:], nil }
	sm := NewSchemaManagerWithHooks(db, "1.0.0", backup, enc, dec)

	require.NoError(t, sm.migrateV8ToV9(db))

	var cipher string
	require.NoError(t, db.Model(&SiteSetting{}).Where("name = ?", "s").Pluck("cookie_encrypted", &cipher).Error)
	assert.Equal(t, "e:raw", cipher)
}

func TestSiteRepository_ListAndCredentials(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &RSSSubscription{})
	repo := NewSiteRepository(db)

	_, err := repo.CreateSite(SiteData{Name: "a", AuthMethod: "cookie", Enabled: true})
	require.NoError(t, err)
	_, err = repo.CreateSite(SiteData{Name: "b", AuthMethod: "cookie", Enabled: false})
	require.NoError(t, err)

	all, err := repo.ListSites()
	require.NoError(t, err)
	assert.Len(t, all, 2)

	enabled, err := repo.ListEnabledSites()
	require.NoError(t, err)
	require.Len(t, enabled, 1)
	assert.Equal(t, "a", enabled[0].Name)

	exists, err := repo.SiteExistsByName("a")
	require.NoError(t, err)
	assert.True(t, exists)

	on := true
	require.NoError(t, repo.UpdateSiteCredentials("a", &on, "api_key", "ck", "ak", "https://api", "pk"))
	site, err := repo.GetSiteByName("a")
	require.NoError(t, err)
	assert.Equal(t, "api_key", site.AuthMethod)
	assert.Equal(t, "ak", site.APIKey)
	assert.Equal(t, "pk", site.Passkey)

	require.NoError(t, repo.UpdateSiteCredentials("brand-new", nil, "cookie", "c", "", "", ""))
	brand, err := repo.GetSiteByName("brand-new")
	require.NoError(t, err)
	assert.True(t, brand.IsBuiltin)

	require.NoError(t, repo.DeleteSite("b"))
	exists, err = repo.SiteExistsByName("b")
	require.NoError(t, err)
	assert.False(t, exists)

	_, err = repo.CreateSite(SiteData{Name: "a", AuthMethod: "cookie"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "已存在")

	_, err = repo.CreateSite(SiteData{AuthMethod: "cookie"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "站点名称不能为空")
}

// ---------------------------------------------------------------------------
// rss_filter_association.go: remaining query helpers
// ---------------------------------------------------------------------------

func TestRSSFilterAssociation_QueryHelpers(t *testing.T) {
	db := newMemDB(t, &FilterRule{}, &RSSSubscription{}, &RSSFilterAssociation{})
	assocDB := NewRSSFilterAssociationDB(db)

	r1 := FilterRule{Name: "r1", Enabled: true, Priority: 1}
	r2 := FilterRule{Name: "r2", Enabled: true, Priority: 2}
	require.NoError(t, db.Create(&r1).Error)
	require.NoError(t, db.Create(&r2).Error)

	require.NoError(t, assocDB.SetFilterRulesForRSS(5, []uint{r1.ID, r2.ID}))

	rssIDs, err := assocDB.GetByFilterRuleID(r1.ID)
	require.NoError(t, err)
	assert.Equal(t, []uint{5}, rssIDs)

	rules, err := assocDB.GetFilterRulesForRSS(5)
	require.NoError(t, err)
	require.Len(t, rules, 2)
	assert.Equal(t, "r1", rules[0].Name)

	has, err := assocDB.HasAssociations(5)
	require.NoError(t, err)
	assert.True(t, has)

	ok, err := assocDB.Exists(5, r1.ID)
	require.NoError(t, err)
	assert.True(t, ok)

	require.NoError(t, assocDB.DeleteByFilterRuleID(r1.ID))
	ok, err = assocDB.Exists(5, r1.ID)
	require.NoError(t, err)
	assert.False(t, ok)
}

// ---------------------------------------------------------------------------
// config_models.go: interval clamp branches (reachable, unlike concurrency min)
// ---------------------------------------------------------------------------

func TestRSSConfig_GetEffectiveIntervalMinutes_Clamps(t *testing.T) {
	global := &SettingsGlobal{DefaultIntervalMinutes: 20}

	assert.Equal(t, int32(30), (&RSSConfig{IntervalMinutes: 30}).GetEffectiveIntervalMinutes(global))
	assert.Equal(t, MinIntervalMinutes, (&RSSConfig{IntervalMinutes: 1}).GetEffectiveIntervalMinutes(global))
	assert.Equal(t, MaxIntervalMinutes, (&RSSConfig{IntervalMinutes: 99999}).GetEffectiveIntervalMinutes(global))
	assert.Equal(t, int32(20), (&RSSConfig{IntervalMinutes: 0}).GetEffectiveIntervalMinutes(global))
	assert.Equal(t, DefaultIntervalMinutes, (&RSSConfig{}).GetEffectiveIntervalMinutes(nil))
}

func TestSettingsGlobal_EffectiveBounds(t *testing.T) {
	assert.Equal(t, DefaultIntervalMinutes, (&SettingsGlobal{DefaultIntervalMinutes: 0}).GetEffectiveIntervalMinutes())
	assert.Equal(t, MinIntervalMinutes, (&SettingsGlobal{DefaultIntervalMinutes: 2}).GetEffectiveIntervalMinutes())
	assert.Equal(t, MaxIntervalMinutes, (&SettingsGlobal{DefaultIntervalMinutes: 99999}).GetEffectiveIntervalMinutes())

	assert.Equal(t, DefaultConcurrency, (&SettingsGlobal{DefaultConcurrency: 0}).GetEffectiveConcurrency())
	assert.Equal(t, MaxConcurrency, (&SettingsGlobal{DefaultConcurrency: 999}).GetEffectiveConcurrency())
	assert.Equal(t, int32(4), (&SettingsGlobal{DefaultConcurrency: 4}).GetEffectiveConcurrency())
}

func TestMTTorrentDetail_CanbeFinished_TimeInsufficient(t *testing.T) {
	logger := zapNopLogger()
	future := time.Now().In(cstLoc()).Add(2 * time.Second).Format("2006-01-02 15:04:05")
	d := MTTorrentDetail{
		Status: &Status{ID: "1", DiscountEndTime: future},
		Size:   "2199023255552",
	}
	assert.False(t, d.CanbeFinished(logger, true, 1, 0))
}

// ---------------------------------------------------------------------------
// init.go: TorrentInfo.GetExpired IsExpired short-circuit
// ---------------------------------------------------------------------------

func TestTorrentInfo_GetExpired_IsExpiredFlag(t *testing.T) {
	assert.True(t, (&TorrentInfo{IsExpired: true}).GetExpired())
	assert.False(t, (&TorrentInfo{FreeLevel: "free"}).GetExpired())
}

// ---------------------------------------------------------------------------
// schema_version.go: migrateV1ToV2 marks example RSS + backfills concurrency
// ---------------------------------------------------------------------------

func TestMigrateV1ToV2_Full(t *testing.T) {
	db := newMemDB(t, &SettingsGlobal{}, &RSSSubscription{})

	require.NoError(t, db.Create(&SettingsGlobal{DefaultConcurrency: 0}).Error)
	require.NoError(t, db.Create(&RSSSubscription{Name: "ex", URL: "https://example.com/rss", IntervalMinutes: 5}).Error)

	require.NoError(t, migrateV1ToV2(db))

	var gl SettingsGlobal
	require.NoError(t, db.First(&gl).Error)
	assert.Equal(t, DefaultConcurrency, gl.DefaultConcurrency)

	var rss RSSSubscription
	require.NoError(t, db.Where("name = ?", "ex").First(&rss).Error)
	assert.True(t, rss.IsExample)
}

// ---------------------------------------------------------------------------
// presets.go: migrateCmctToSpringSunday user_info reparent branch
// ---------------------------------------------------------------------------

func TestMigrateCmctToSpringSunday_UserInfoReparent(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &RSSSubscription{})
	require.NoError(t, db.Exec("CREATE TABLE user_info (id INTEGER PRIMARY KEY, site TEXT, username TEXT)").Error)
	require.NoError(t, db.Exec("INSERT INTO user_info (site, username) VALUES ('cmct','alice')").Error)

	require.NoError(t, db.Create(&SiteSetting{Name: "cmct", AuthMethod: "cookie", Cookie: "ck"}).Error)

	require.NoError(t, SyncSitesFromRegistry(db, nil))

	var uname string
	require.NoError(t, db.Raw("SELECT username FROM user_info WHERE site = ?", "springsunday").Scan(&uname).Error)
	assert.Equal(t, "alice", uname)
}

func TestMigrateCmctToSpringSunday_UserInfoKeepExisting(t *testing.T) {
	db := newMemDB(t, &SiteSetting{}, &RSSSubscription{})
	require.NoError(t, db.Exec("CREATE TABLE user_info (id INTEGER PRIMARY KEY, site TEXT, username TEXT)").Error)
	require.NoError(t, db.Exec("INSERT INTO user_info (site, username) VALUES ('cmct','old')").Error)
	require.NoError(t, db.Exec("INSERT INTO user_info (site, username) VALUES ('springsunday','current')").Error)

	require.NoError(t, db.Create(&SiteSetting{Name: "cmct", AuthMethod: "cookie"}).Error)
	require.NoError(t, db.Create(&SiteSetting{Name: "springsunday", AuthMethod: "cookie"}).Error)

	require.NoError(t, SyncSitesFromRegistry(db, nil))

	var cmctCount int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM user_info WHERE site = ?", "cmct").Scan(&cmctCount).Error)
	assert.Equal(t, int64(0), cmctCount)

	var uname string
	require.NoError(t, db.Raw("SELECT username FROM user_info WHERE site = ?", "springsunday").Scan(&uname).Error)
	assert.Equal(t, "current", uname)
}

// ---------------------------------------------------------------------------
// init.go: NewDBWithVersionAndHooks with encryption hooks + persistence round-trip
// ---------------------------------------------------------------------------

func TestNewDBWithVersionAndHooks_PersistsTorrent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	backup := func(db *gorm.DB, table string) (string, error) { return "backup/" + table, nil }
	enc := func(plain string) (string, error) { return "enc:" + plain, nil }
	dec := func(cipher string) (string, error) { return cipher[4:], nil }

	tdb, err := NewDBWithVersionAndHooks(zapNopGormLogger(), "2.0.0", backup, enc, dec)
	require.NoError(t, err)
	require.NotNil(t, tdb)

	require.NoError(t, tdb.UpsertTorrent(&TorrentInfo{SiteName: "hdsky", TorrentID: "abc", IsFree: true}))
	got, err := tdb.GetTorrentBySiteAndID("hdsky", "abc")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.IsFree)

	var glCnt int64
	require.NoError(t, tdb.DB.Model(&SettingsGlobal{}).Count(&glCnt).Error)
	assert.Equal(t, int64(1), glCnt)
}
