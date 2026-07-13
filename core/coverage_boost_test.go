package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

func newCloakDB(t *testing.T) *models.TorrentDB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "c.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.SettingsGlobal{}, &models.QbitSettings{}, &models.SiteSetting{},
		&models.RSSSubscription{}, &models.TorrentInfo{}, &models.AdminUser{},
		&models.DownloaderSetting{}, &models.SiteTemplate{}, &models.FilterRule{},
		&models.RSSFilterAssociation{}, &models.CloakSettings{}, &models.MigrationState{},
	))
	return &models.TorrentDB{DB: db}
}

// ---------------------------------------------------------------------------
// Cloak config: create + update paths with real encrypted round-trips
// ---------------------------------------------------------------------------

func TestCloakConfig_Lifecycle(t *testing.T) {
	writeTestSecretKey(t)
	db := newCloakDB(t)
	s := NewConfigStore(db)

	cfg, err := s.GetCloakConfig()
	require.NoError(t, err)
	assert.Empty(t, cfg.Endpoint)
	assert.False(t, cfg.HasToken)

	tok, err := s.GetCloakToken()
	require.NoError(t, err)
	assert.Empty(t, tok)

	require.NoError(t, s.SetCloakEndpoint("https://cloak.local"))
	ep, err := s.GetCloakEndpoint()
	require.NoError(t, err)
	assert.Equal(t, "https://cloak.local", ep)

	require.NoError(t, s.SetCloakToken("super-secret"))
	cfg, err = s.GetCloakConfig()
	require.NoError(t, err)
	assert.True(t, cfg.HasToken)
	assert.Equal(t, "https://cloak.local", cfg.Endpoint)

	tok, err = s.GetCloakToken()
	require.NoError(t, err)
	assert.Equal(t, "super-secret", tok)

	require.NoError(t, s.SetCloakEndpoint("https://cloak2.local"))
	tok, err = s.GetCloakToken()
	require.NoError(t, err)
	assert.Equal(t, "super-secret", tok)

	require.NoError(t, s.SetCloakToken(""))
	cfg, err = s.GetCloakConfig()
	require.NoError(t, err)
	assert.False(t, cfg.HasToken)
}

func TestSaveCloakConfig_CreateUpdateClear(t *testing.T) {
	writeTestSecretKey(t)
	db := newCloakDB(t)
	s := NewConfigStore(db)

	require.NoError(t, s.SaveCloakConfig("https://a", "tok-a", false))
	cfg, err := s.GetCloakConfig()
	require.NoError(t, err)
	assert.Equal(t, "https://a", cfg.Endpoint)
	assert.True(t, cfg.HasToken)

	require.NoError(t, s.SaveCloakConfig("https://b", "", false))
	tok, err := s.GetCloakToken()
	require.NoError(t, err)
	assert.Equal(t, "tok-a", tok)
	ep, err := s.GetCloakEndpoint()
	require.NoError(t, err)
	assert.Equal(t, "https://b", ep)

	require.NoError(t, s.SaveCloakConfig("https://b", "ignored", true))
	cfg, err = s.GetCloakConfig()
	require.NoError(t, err)
	assert.False(t, cfg.HasToken)
}

func TestSetCloakToken_CreateWithToken(t *testing.T) {
	writeTestSecretKey(t)
	db := newCloakDB(t)
	s := NewConfigStore(db)

	require.NoError(t, s.SetCloakToken("brand-new"))
	tok, err := s.GetCloakToken()
	require.NoError(t, err)
	assert.Equal(t, "brand-new", tok)
}

// ---------------------------------------------------------------------------
// SyncSites delegates to models.SyncSitesFromRegistry
// ---------------------------------------------------------------------------

func TestConfigStore_SyncSites(t *testing.T) {
	db := newCloakDB(t)
	s := NewConfigStore(db)

	require.NoError(t, s.SyncSites([]models.RegisteredSite{
		{ID: "hdsky", Name: "HDSky", AuthMethod: "cookie", DefaultBaseURL: "https://hdsky.me"},
	}))

	sites, err := s.ListSites()
	require.NoError(t, err)
	_, ok := sites["hdsky"]
	assert.True(t, ok)
}

// ---------------------------------------------------------------------------
// Load: reads persisted global + qbit + sites with encrypted cookie
// ---------------------------------------------------------------------------

func TestConfigStore_LoadWithSitesAndQbit(t *testing.T) {
	writeTestSecretKey(t)
	db := newCloakDB(t)
	s := NewConfigStore(db)

	require.NoError(t, s.SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 15, DefaultEnabled: true,
	}))
	require.NoError(t, s.SaveQbitSettings(models.QbitSettings{
		Enabled: true, URL: "http://qb:8080", User: "u", Password: "p",
	}))

	id, err := s.UpsertSite("hdsky", models.SiteConfig{
		Enabled: boolPtr(true), AuthMethod: "cookie", Cookie: "uid=1;pass=2",
	})
	require.NoError(t, err)
	require.NoError(t, s.ReplaceSiteRSS(id, []models.RSSConfig{
		{Name: "r1", URL: "https://hdsky.me/rss", IntervalMinutes: 10},
	}))

	cfg, err := s.Load()
	require.NoError(t, err)
	assert.Equal(t, int32(15), cfg.Global.DefaultIntervalMinutes)
	assert.True(t, cfg.Qbit.Enabled)
	assert.Equal(t, "http://qb:8080", cfg.Qbit.URL)

	sc, ok := cfg.Sites["hdsky"]
	require.True(t, ok)
	assert.Equal(t, "uid=1;pass=2", sc.Cookie)
	require.Len(t, sc.RSS, 1)
	assert.Equal(t, "r1", sc.RSS[0].Name)
}

// ---------------------------------------------------------------------------
// SaveGlobal (legacy) + GetGlobalOnly defaults
// ---------------------------------------------------------------------------

func TestConfigStore_SaveGlobalAndDefaults(t *testing.T) {
	db := newCloakDB(t)
	s := NewConfigStore(db)

	got, err := s.GetGlobalOnly()
	require.NoError(t, err)
	assert.Equal(t, "download", got.DownloadDir)
	assert.True(t, got.CleanupDiskProtect)
	assert.Equal(t, float64(50), got.CleanupMinDiskSpaceGB)

	require.NoError(t, s.SaveGlobal(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, AutoStart: true,
	}))
	got, err = s.GetGlobalOnly()
	require.NoError(t, err)
	assert.True(t, got.AutoStart)
	assert.Equal(t, models.MinIntervalMinutes, got.DefaultIntervalMinutes)

	require.Error(t, s.SaveGlobal(models.SettingsGlobal{DownloadDir: "  "}))
}

// ---------------------------------------------------------------------------
// SaveQbit (legacy) validation + persistence
// ---------------------------------------------------------------------------

func TestConfigStore_SaveQbitLegacy(t *testing.T) {
	db := newCloakDB(t)
	s := NewConfigStore(db)

	require.Error(t, s.SaveQbit(models.QbitSettings{URL: "", User: "u", Password: "p"}))

	require.NoError(t, s.SaveQbit(models.QbitSettings{URL: "http://x", User: "u", Password: "p", Enabled: true}))
	q, err := s.GetQbitOnly()
	require.NoError(t, err)
	assert.True(t, q.Enabled)
	assert.Equal(t, "http://x", q.URL)
}

// ---------------------------------------------------------------------------
// DeleteSite: preset guard + real delete
// ---------------------------------------------------------------------------

func TestConfigStore_DeleteSite(t *testing.T) {
	db := newCloakDB(t)
	s := NewConfigStore(db)

	require.Error(t, s.DeleteSite("hdsky"))

	_, err := s.UpsertSite("customsite", models.SiteConfig{AuthMethod: "cookie"})
	require.NoError(t, err)
	require.NoError(t, s.DeleteSite("customsite"))

	_, err = s.GetSiteConf("customsite")
	require.Error(t, err)

	require.Error(t, s.DeleteSite("nonexistent"))
}

// ---------------------------------------------------------------------------
// ensureCookieKeyAvailable via env key + GetCloakToken with missing key
// ---------------------------------------------------------------------------

func TestEncryptCookie_EnvKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PT_TOOLS_SECRET_KEY", "MDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDA=")
	s := NewConfigStore(nil)
	require.NoError(t, s.ensureCookieKeyAvailable())
}

func TestEncryptCookie_EnvKeyInvalid(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PT_TOOLS_SECRET_KEY", "not-32-bytes")
	s := NewConfigStore(nil)
	_, err := s.EncryptCookie("x")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrKeyMissing)
}

// ---------------------------------------------------------------------------
// v2_broadcast: dispatchV2BroadcastIfReady + MaybeSendV2Broadcast paths
// ---------------------------------------------------------------------------

func TestDispatchV2BroadcastIfReady(t *testing.T) {
	db := newCloakDB(t)
	require.NoError(t, models.UpsertMigrationState(db.DB, V2BroadcastSchemaVersion, time.Now().UTC()))

	var called bool
	SetV2Broadcaster(V2BroadcasterFunc(func(context.Context) error {
		called = true
		return nil
	}))
	t.Cleanup(func() { SetV2Broadcaster(nil) })

	dispatchV2BroadcastIfReady(db.DB, nil)
	assert.True(t, called)

	st, ok := models.GetMigrationState(db.DB, V2BroadcastSchemaVersion)
	require.True(t, ok)
	assert.True(t, st.BroadcastSent)
}

func TestDispatchV2BroadcastIfReady_NoBroadcaster(t *testing.T) {
	SetV2Broadcaster(nil)
	db := newCloakDB(t)
	dispatchV2BroadcastIfReady(db.DB, nil)
}

// ---------------------------------------------------------------------------
// NewTempDBDir error path (unwritable dir)
// ---------------------------------------------------------------------------

func TestNewTempDBDir_BadPath(t *testing.T) {
	_, err := NewTempDBDir(filepath.Join(os.DevNull, "nope"))
	require.Error(t, err)
}

func TestMaybeSendV2Broadcast_ZeroNowAndCompletedZero(t *testing.T) {
	db := newCloakDB(t)
	require.NoError(t, db.DB.Create(&models.MigrationState{
		SchemaVersion: V2BroadcastSchemaVersion,
	}).Error)

	res := MaybeSendV2Broadcast(context.Background(), db.DB,
		V2BroadcasterFunc(func(context.Context) error { return nil }), nil, time.Time{})
	assert.Equal(t, "completed_at_zero", res.Reason)
}

func TestMaybeSendV2Broadcast_MarkSentFailure(t *testing.T) {
	db := newCloakDB(t)
	require.NoError(t, models.UpsertMigrationState(db.DB, V2BroadcastSchemaVersion, time.Now().UTC()))

	sqlDB, err := db.DB.DB()
	require.NoError(t, err)

	var called bool
	res := MaybeSendV2Broadcast(context.Background(), db.DB,
		V2BroadcasterFunc(func(context.Context) error {
			called = true
			require.NoError(t, sqlDB.Close())
			return nil
		}), nil, time.Now().UTC())
	assert.True(t, called)
	assert.True(t, res.Sent)
	assert.Equal(t, "mark_failed", res.Reason)
}

func TestUpsertSiteWithRSS_PasskeyAndFilterRules(t *testing.T) {
	writeTestSecretKey(t)
	db := newCloakDB(t)
	s := NewConfigStore(db)

	fr := models.FilterRule{Name: "keep", Enabled: true}
	require.NoError(t, db.DB.Create(&fr).Error)

	err := s.UpsertSiteWithRSS("hdsky", models.SiteConfig{
		Enabled:    boolPtr(true),
		AuthMethod: "passkey",
		Passkey:    "pk-123",
		APIUrl:     "https://hdsky.me",
		RSS: []models.RSSConfig{
			{Name: "r1", URL: "https://hdsky.me/rss", IntervalMinutes: 1, FilterRuleIDs: []uint{fr.ID}},
		},
	})
	require.NoError(t, err)

	sc, err := s.GetSiteConf("hdsky")
	require.NoError(t, err)
	assert.Equal(t, "passkey", sc.AuthMethod)
	require.Len(t, sc.RSS, 1)
	assert.Equal(t, models.MinIntervalMinutes, sc.RSS[0].IntervalMinutes)
	assert.ElementsMatch(t, []uint{fr.ID}, sc.RSS[0].FilterRuleIDs)
}

func TestUpsertSiteWithRSS_ValidationErrors(t *testing.T) {
	db := newCloakDB(t)
	s := NewConfigStore(db)

	require.Error(t, s.UpsertSiteWithRSS("customx", models.SiteConfig{AuthMethod: "bad"}))
	require.Error(t, s.UpsertSiteWithRSS("customx", models.SiteConfig{AuthMethod: "cookie"}))
	require.Error(t, s.UpsertSiteWithRSS("customx", models.SiteConfig{
		AuthMethod: "passkey", APIUrl: "https://x",
	}))

	err := s.UpsertSiteWithRSS("customx", models.SiteConfig{
		AuthMethod: "cookie", APIUrl: "https://x", Cookie: "c",
		RSS: []models.RSSConfig{
			{Name: "a", URL: "https://dup", IntervalMinutes: 5},
			{Name: "b", URL: "https://dup", IntervalMinutes: 5},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "重复")
}

func TestGetGlobalSettings_DefaultsWhenEmpty(t *testing.T) {
	db := newCloakDB(t)
	s := NewConfigStore(db)

	gs, err := s.GetGlobalSettings()
	require.NoError(t, err)
	assert.Equal(t, "download", gs.DownloadDir)
	assert.True(t, gs.CleanupDiskProtect)

	q, err := s.GetQbitSettings()
	require.NoError(t, err)
	assert.False(t, q.Enabled)
}

func TestAppendRSSToSite_WithFilterRules(t *testing.T) {
	db := newCloakDB(t)
	s := NewConfigStore(db)

	_, err := s.UpsertSite("hdsky", models.SiteConfig{AuthMethod: "cookie"})
	require.NoError(t, err)
	fr := models.FilterRule{Name: "f", Enabled: true}
	require.NoError(t, db.DB.Create(&fr).Error)

	created, err := s.AppendRSSToSite("hdsky", models.RSSConfig{
		Name: "r", URL: "https://hdsky.me/feed", IntervalMinutes: 10, FilterRuleIDs: []uint{fr.ID},
	})
	require.NoError(t, err)
	assert.NotZero(t, created.ID)

	list, err := s.ListRSSForSite("hdsky")
	require.NoError(t, err)
	require.Len(t, list, 1)
}

func TestUpsertSiteWithRSS_ApiKeyAndRSSFieldErrors(t *testing.T) {
	db := newCloakDB(t)
	s := NewConfigStore(db)

	require.Error(t, s.UpsertSiteWithRSS("customx", models.SiteConfig{
		AuthMethod: "api_key", APIUrl: "https://x",
	}))

	require.Error(t, s.UpsertSiteWithRSS("customx", models.SiteConfig{
		AuthMethod: "cookie_and_api_key", APIUrl: "https://x", APIKey: "k",
	}))

	nameErr := s.UpsertSiteWithRSS("customx", models.SiteConfig{
		AuthMethod: "cookie", APIUrl: "https://x", Cookie: "c",
		RSS: []models.RSSConfig{{Name: "  ", URL: "https://a", IntervalMinutes: 5}},
	})
	require.Error(t, nameErr)
	assert.Contains(t, nameErr.Error(), "name")

	urlErr := s.UpsertSiteWithRSS("customx", models.SiteConfig{
		AuthMethod: "cookie", APIUrl: "https://x", Cookie: "c",
		RSS: []models.RSSConfig{{Name: "n", URL: "  ", IntervalMinutes: 5}},
	})
	require.Error(t, urlErr)
	assert.Contains(t, urlErr.Error(), "url")

	require.Error(t, s.UpsertSiteWithRSS("customx", models.SiteConfig{
		AuthMethod: "cookie", APIUrl: "https://x", Cookie: "c", APIKey: "notempty",
	}))
}

func TestGetCloakToken_NoTokenSet(t *testing.T) {
	writeTestSecretKey(t)
	db := newCloakDB(t)
	s := NewConfigStore(db)

	require.NoError(t, s.SetCloakEndpoint("https://only-endpoint"))
	tok, err := s.GetCloakToken()
	require.NoError(t, err)
	assert.Empty(t, tok)
}

func TestLoad_NoQbitRow(t *testing.T) {
	db := newCloakDB(t)
	s := NewConfigStore(db)

	cfg, err := s.Load()
	require.NoError(t, err)
	assert.False(t, cfg.Qbit.Enabled)
	assert.NotEmpty(t, cfg.Global.DownloadDir)
}

func TestConfigStore_ErrorPaths_ClosedDB(t *testing.T) {
	writeTestSecretKey(t)
	db := newCloakDB(t)
	s := NewConfigStore(db)

	dir := t.TempDir()
	_, err := s.UpsertSite("hdsky", models.SiteConfig{AuthMethod: "cookie", Cookie: "c"})
	require.NoError(t, err)

	sqlDB, err := db.DB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = s.Load()
	assert.Error(t, err)
	_, err = s.ListSites()
	assert.Error(t, err)
	_, err = s.GetSiteConf("hdsky")
	assert.Error(t, err)
	assert.Error(t, s.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 10}))
	assert.Error(t, s.SaveQbitSettings(models.QbitSettings{URL: "http://x", User: "u", Password: "p"}))
	assert.Error(t, s.SaveGlobal(models.SettingsGlobal{DownloadDir: dir}))
	assert.Error(t, s.SaveQbit(models.QbitSettings{URL: "http://x", User: "u", Password: "p"}))
	_, err = s.UpsertSite("x", models.SiteConfig{AuthMethod: "cookie"})
	assert.Error(t, err)
	assert.Error(t, s.ReplaceSiteRSS(1, []models.RSSConfig{{Name: "r", URL: "u"}}))
	assert.Error(t, s.EnsureAdmin("admin", "hash"))
	_, err = s.AppendRSSToSite("hdsky", models.RSSConfig{Name: "r", URL: "https://x", IntervalMinutes: 5})
	assert.Error(t, err)
	_, err = s.DeleteRSSFromSite("hdsky", 1)
	assert.Error(t, err)
	assert.Error(t, s.DeleteSite("customx"))
	_, err = s.GetCloakConfig()
	assert.Error(t, err)
	_, err = s.GetCloakToken()
	assert.Error(t, err)
	assert.Error(t, s.SetCloakEndpoint("https://y"))
	assert.Error(t, s.SetCloakToken("tok"))
	assert.Error(t, s.SaveCloakConfig("https://y", "tok", false))
	_, err = s.GetGlobalOnly()
	assert.NoError(t, err)
}

func TestUpsertSiteWithRSS_CookieAndAPIKeyValid(t *testing.T) {
	writeTestSecretKey(t)
	db := newCloakDB(t)
	s := NewConfigStore(db)

	err := s.UpsertSiteWithRSS("customsite", models.SiteConfig{
		Enabled:    boolPtr(true),
		AuthMethod: "cookie_and_api_key",
		APIUrl:     "https://custom.example",
		APIKey:     "k123",
		Cookie:     "uid=9;pass=8",
	})
	require.NoError(t, err)

	sc, err := s.GetSiteConf("customsite")
	require.NoError(t, err)
	assert.Equal(t, "cookie_and_api_key", sc.AuthMethod)
	assert.Equal(t, "k123", sc.APIKey)
	assert.Equal(t, "uid=9;pass=8", sc.Cookie)
}

func TestSaveQbitSettings_CreateBranch(t *testing.T) {
	db := newCloakDB(t)
	s := NewConfigStore(db)

	require.NoError(t, s.SaveQbitSettings(models.QbitSettings{
		URL: "http://qb", User: "u", Password: "p", Enabled: true,
	}))
	q, err := s.GetQbitSettings()
	require.NoError(t, err)
	assert.True(t, q.Enabled)
	assert.Equal(t, "http://qb", q.URL)
}

func TestDecryptCookie_KeyMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PT_TOOLS_SECRET_KEY", "")
	s := NewConfigStore(nil)
	_, err := s.DecryptCookie("garbage")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrKeyMissing)
}

func TestUpsertSiteWithRSS_CookieAndAPIKeyMissingAPIKey(t *testing.T) {
	db := newCloakDB(t)
	s := NewConfigStore(db)

	err := s.UpsertSiteWithRSS("customsite", models.SiteConfig{
		AuthMethod: "cookie_and_api_key",
		APIUrl:     "https://custom.example",
		Cookie:     "uid=1",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API Key")
}

func TestGetQbitOnly_NoRow(t *testing.T) {
	db := newCloakDB(t)
	s := NewConfigStore(db)
	q, err := s.GetQbitOnly()
	require.NoError(t, err)
	assert.False(t, q.Enabled)
}

func TestCloakToken_EncryptErrorPaths(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PT_TOOLS_SECRET_KEY", "")
	db := newCloakDB(t)
	s := NewConfigStore(db)

	require.Error(t, s.SetCloakToken("plaintext"))
	require.Error(t, s.SaveCloakConfig("https://x", "plaintext", false))

	require.NoError(t, db.DB.Create(&models.CloakSettings{Endpoint: "https://pre"}).Error)
	require.Error(t, s.SetCloakToken("plaintext"))
}

func TestLoad_CreatesDefaultGlobalRow(t *testing.T) {
	db := newCloakDB(t)
	s := NewConfigStore(db)

	cfg, err := s.Load()
	require.NoError(t, err)
	assert.Equal(t, "download", cfg.Global.DownloadDir)

	var cnt int64
	require.NoError(t, db.DB.Model(&models.SettingsGlobal{}).Count(&cnt).Error)
	assert.Equal(t, int64(1), cnt)
}

func TestUpsertSite_ClearCookieWhenEmpty(t *testing.T) {
	writeTestSecretKey(t)
	db := newCloakDB(t)
	s := NewConfigStore(db)

	id, err := s.UpsertSite("hdsky", models.SiteConfig{AuthMethod: "cookie", Cookie: "uid=1"})
	require.NoError(t, err)

	id2, err := s.UpsertSite("hdsky", models.SiteConfig{AuthMethod: "cookie", Cookie: ""})
	require.NoError(t, err)
	assert.Equal(t, id, id2)

	var row models.SiteSetting
	require.NoError(t, db.DB.First(&row, id).Error)
	assert.Empty(t, row.CookieEncrypted)
}
