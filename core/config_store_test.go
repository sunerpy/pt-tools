package core

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func newTempDB(t *testing.T) *models.TorrentDB {
	t.Helper()
	dir := t.TempDir()
	dbFile := filepath.Join(dir, "torrents.db")
	db, err := gorm.Open(sqlite.Open("file:"+dbFile+"?cache=shared&_journal_mode=WAL"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.SettingsGlobal{}, &models.QbitSettings{}, &models.SiteSetting{}, &models.RSSSubscription{}, &models.TorrentInfo{}, &models.AdminUser{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return &models.TorrentDB{DB: db}
}

// NewTestDB exposes test DB to other packages' tests
func NewTestDB(t *testing.T) *models.TorrentDB { return newTempDB(t) }

func TestLoadDefaultPersistence(t *testing.T) {
	db := newTempDB(t)
	store := NewConfigStore(db)
	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Global.DownloadDir == "" {
		t.Fatalf("default download dir empty")
	}
	// second load should read the same persisted defaults
	cfg2, err := store.Load()
	if err != nil {
		t.Fatalf("load2: %v", err)
	}
	if cfg2.Global.DownloadDir != cfg.Global.DownloadDir {
		t.Fatalf("download dir mismatch: %s vs %s", cfg2.Global.DownloadDir, cfg.Global.DownloadDir)
	}
}

func TestLoadSnapshotConsistency(t *testing.T) {
	db := newTempDB(t)
	store := NewConfigStore(db)
	// write global & qbit & site/rss
	if err := store.SaveGlobal(models.SettingsGlobal{DefaultIntervalMinutes: 30, DownloadDir: "data"}); err != nil {
		t.Fatalf("save global: %v", err)
	}
	if err := store.SaveQbit(models.QbitSettings{Enabled: true, URL: "http://localhost:8080", User: "u", Password: "p"}); err != nil {
		t.Fatalf("save qbit: %v", err)
	}
	sc := models.SiteConfig{Enabled: boolPtr(true), AuthMethod: "cookie", Cookie: "ck", APIUrl: "http://api"}
	sc.RSS = []models.RSSConfig{{Name: "cmct", URL: "https://rss", IntervalMinutes: 10}}
	if err := store.UpsertSiteWithRSS(models.CMCT, sc); err != nil {
		t.Fatalf("save site: %v", err)
	}
	// load snapshot
	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Global.DownloadDir == "" {
		t.Fatalf("download dir empty")
	}
	if cfg.Qbit.URL == "" {
		t.Fatalf("qbit url empty")
	}
	if len(cfg.Sites[models.CMCT].RSS) != 1 {
		t.Fatalf("rss count mismatch")
	}
}

func TestSaveGlobal_InvalidDir(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	err = s.SaveGlobal(models.SettingsGlobal{DownloadDir: "", DefaultIntervalMinutes: models.MinIntervalMinutes})
	assert.Error(t, err)
}

func TestSaveQbit_InvalidFields(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	err = s.SaveQbit(models.QbitSettings{Enabled: true})
	assert.Error(t, err)
}

func TestAdminCRUD(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.InitLogger(zap.NewNop())
	s := NewConfigStore(db)
	require.NoError(t, s.EnsureAdmin("admin", "hash1"))
	u, err := s.GetAdmin("admin")
	require.NoError(t, err)
	require.NotNil(t, u)
	assert.Equal(t, "admin", u.Username)
	u.PasswordHash = "hash2"
	require.NoError(t, s.UpdateAdmin(u))
	u2, err := s.GetAdmin("admin")
	require.NoError(t, err)
	assert.Equal(t, "hash2", u2.PasswordHash)
	require.NoError(t, s.UpdateAdminPassword("admin", "hash3"))
	u3, err := s.GetAdmin("admin")
	require.NoError(t, err)
	assert.Equal(t, "hash3", u3.PasswordHash)
	cnt, err := s.AdminCount()
	require.NoError(t, err)
	assert.True(t, cnt >= 1)
}

func TestQbitSettings_SaveAndGet(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	qb := models.QbitSettings{Enabled: true, URL: "http://localhost", User: "u", Password: "p"}
	require.NoError(t, s.SaveQbitSettings(qb))
	out, err := s.GetQbitSettings()
	require.NoError(t, err)
	assert.Equal(t, qb.URL, out.URL)
	err = s.SaveQbitSettings(models.QbitSettings{Enabled: true})
	assert.Error(t, err)
}

func TestGetGlobalSettings_DefaultAndSave(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	gl, err := s.GetGlobalSettings()
	require.NoError(t, err)
	assert.NotEmpty(t, gl.DownloadDir)
	gl.DownloadDir = "downloads"
	gl.DefaultIntervalMinutes = models.MinIntervalMinutes
	require.NoError(t, s.SaveGlobalSettings(gl))
}

func TestSaveQbitSettings_InvalidAndValid(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	// invalid empty fields
	err = s.SaveQbitSettings(models.QbitSettings{Enabled: true})
	require.Error(t, err)
	// valid
	require.NoError(t, s.SaveQbitSettings(models.QbitSettings{Enabled: true, URL: "http://u", User: "x", Password: "y"}))
	out, err := s.GetQbitSettings()
	require.NoError(t, err)
	require.Equal(t, "http://u", out.URL)
}

func TestSaveGlobalSettings_Invalids(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	err = s.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: "", DefaultIntervalMinutes: models.MinIntervalMinutes})
	require.Error(t, err)
	// interval less than min should be coerced
	dir := t.TempDir()
	gl := models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 1}
	require.NoError(t, s.SaveGlobalSettings(gl))
	got, err := s.GetGlobalOnly()
	require.NoError(t, err)
	require.GreaterOrEqual(t, int(got.DefaultIntervalMinutes), int(models.MinIntervalMinutes))
}

func TestUpsertSiteWithRSS_Validations(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	err = s.UpsertSiteWithRSS(models.CMCT, models.SiteConfig{AuthMethod: "invalid", APIUrl: "http://x", RSS: []models.RSSConfig{{Name: "r", URL: "http://u"}}})
	require.Error(t, err)
	err = s.UpsertSiteWithRSS(models.CMCT, models.SiteConfig{AuthMethod: "cookie", APIUrl: "", RSS: []models.RSSConfig{{Name: "r", URL: "http://u"}}})
	require.Error(t, err)
	err = s.UpsertSiteWithRSS(models.MTEAM, models.SiteConfig{AuthMethod: "api_key", APIUrl: models.DefaultAPIUrlMTeam, APIKey: "", RSS: []models.RSSConfig{{Name: "r", URL: "http://u"}}})
	require.Error(t, err)
	e := true
	err = s.UpsertSiteWithRSS(models.MTEAM, models.SiteConfig{Enabled: &e, AuthMethod: "api_key", APIUrl: models.DefaultAPIUrlMTeam, APIKey: "k", RSS: []models.RSSConfig{{Name: "r", URL: "http://u"}}})
	require.NoError(t, err)
}

func TestUpsertSiteWithRSS_SaveAndList(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	sc := models.SiteConfig{Enabled: boolPtr(true), AuthMethod: "cookie", Cookie: "c", APIUrl: "http://api", RSS: []models.RSSConfig{{Name: "r", URL: "http://rss", IntervalMinutes: 10}}}
	require.NoError(t, s.UpsertSiteWithRSS(models.CMCT, sc))
	out, err := s.ListSites()
	require.NoError(t, err)
	require.Equal(t, 1, len(out))
	require.Equal(t, "cookie", out[models.CMCT].AuthMethod)
}

func TestConfigStore_GlobalCRUD(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	s := NewConfigStore(db)
	gl := models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true, AutoStart: false}
	if err = s.SaveGlobalSettings(gl); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := s.GetGlobalOnly()
	if err != nil || got.DownloadDir == "" {
		t.Fatalf("get: %v %v", err, got.DownloadDir)
	}
}

func TestConfigStore_ListSites_Empty(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	out, err := s.ListSites()
	require.NoError(t, err)
	require.Equal(t, 0, len(out))
}

func TestConfigStore_SiteCRUD(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	s := NewConfigStore(db)
	sc := models.SiteConfig{Enabled: boolPtr(true), AuthMethod: "cookie", Cookie: "c"}
	id, err := s.UpsertSite(models.CMCT, sc)
	if err != nil || id == 0 {
		t.Fatalf("upsert: %v %d", err, id)
	}
	rss := []models.RSSConfig{{Name: "r", URL: "http://example/rss", Category: "cat", Tag: "tag", IntervalMinutes: 10}}
	if err = s.ReplaceSiteRSS(id, rss); err != nil {
		t.Fatalf("rss: %v", err)
	}
	sites, err := s.ListSites()
	if err != nil || len(sites) == 0 {
		t.Fatalf("list: %v %d", err, len(sites))
	}
}

func TestPublishOnSave(t *testing.T) {
	db := newTempDB(t)
	store := NewConfigStore(db)
	_, ch, cancel := events.Subscribe(4)
	defer cancel()
	if err := store.SaveGlobal(models.SettingsGlobal{DefaultIntervalMinutes: 20, DownloadDir: "download"}); err != nil {
		t.Fatalf("save global: %v", err)
	}
	select {
	case e := <-ch:
		if e.Type != events.ConfigChanged {
			t.Fatalf("expected ConfigChanged")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("no event received on SaveGlobal")
	}
}

func TestSaveQbit_PublishEvent(t *testing.T) {
	db := newTempDB(t)
	store := NewConfigStore(db)
	_, ch, cancel := events.Subscribe(4)
	defer cancel()
	if err := store.SaveQbit(models.QbitSettings{Enabled: true, URL: "http://u", User: "x", Password: "y"}); err != nil {
		t.Fatalf("save qbit: %v", err)
	}
	select {
	case e := <-ch:
		if e.Type != events.ConfigChanged {
			t.Fatalf("expected ConfigChanged for qbit")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("no event received on SaveQbit")
	}
}

func TestConfigStore_SaveAndGetGlobal(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	s := NewConfigStore(db)
	gl := models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10, DefaultEnabled: true}
	if err = s.SaveGlobalSettings(gl); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err := s.GetGlobalOnly()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.DownloadDir == "" {
		t.Fatalf("empty dir")
	}
}

func TestConfigStore_QbitOnlyAndSiteConf(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	s := NewConfigStore(db)
	qb := models.QbitSettings{Enabled: true, URL: "http://localhost", User: "u", Password: "p"}
	if err = s.SaveQbitSettings(qb); err != nil {
		t.Fatalf("save qbit: %v", err)
	}
	got, err := s.GetQbitOnly()
	if err != nil {
		t.Fatalf("get qbit only: %v", err)
	}
	if got.URL == "" || got.User == "" || got.Password == "" {
		t.Fatalf("qbit empty fields")
	}
	e := true
	siteID, err := s.UpsertSite(models.CMCT, models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
	if err != nil {
		t.Fatalf("upsert site: %v", err)
	}
	if err = s.ReplaceSiteRSS(siteID, []models.RSSConfig{{Name: "r1", URL: "http://example/rss", Tag: "tag", IntervalMinutes: 10}}); err != nil {
		t.Fatalf("rss: %v", err)
	}
	sc, err := s.GetSiteConf(models.CMCT)
	if err != nil {
		t.Fatalf("get site conf: %v", err)
	}
	if sc.Enabled == nil || !*sc.Enabled {
		t.Fatalf("enabled not set")
	}
	if sc.AuthMethod != "cookie" {
		t.Fatalf("auth method unexpected: %s", sc.AuthMethod)
	}
}

func TestGetSiteConf_ApplyDefaults_HDSKY(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	e := true
	id, err := s.UpsertSite(models.HDSKY, models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
	require.NoError(t, err)
	require.NoError(t, s.ReplaceSiteRSS(id, []models.RSSConfig{{Name: "r", URL: "http://rss", IntervalMinutes: 10}}))
	sc, err := s.GetSiteConf(models.HDSKY)
	require.NoError(t, err)
	require.NotNil(t, sc.Enabled)
	require.Equal(t, "cookie", sc.AuthMethod)
	require.Equal(t, "", sc.APIUrl)
	require.Equal(t, 1, len(sc.RSS))
}

func TestDeleteSite_ValidateAndDelete(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	e := true
	_, _ = s.UpsertSite(models.CMCT, models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
	assert.Error(t, s.DeleteSite("cmct"))
	_, _ = s.UpsertSite(models.SiteGroup("custom"), models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
	assert.NoError(t, s.DeleteSite("custom"))
}

func TestUpsertSiteWithRSS_Validation(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	err = s.UpsertSiteWithRSS(models.CMCT, models.SiteConfig{AuthMethod: "bad", APIUrl: "http://api", RSS: []models.RSSConfig{{Name: "r", URL: "u"}}})
	assert.Error(t, err)
	err = s.UpsertSiteWithRSS(models.CMCT, models.SiteConfig{AuthMethod: "cookie", APIUrl: "", Cookie: "c", RSS: []models.RSSConfig{{Name: "r", URL: "u"}}})
	assert.Error(t, err)
	err = s.UpsertSiteWithRSS(models.CMCT, models.SiteConfig{AuthMethod: "cookie", APIUrl: "http://api", Cookie: "c", APIKey: "k", RSS: []models.RSSConfig{{Name: "r", URL: "u"}}})
	assert.Error(t, err)
	err = s.UpsertSiteWithRSS(models.MTEAM, models.SiteConfig{AuthMethod: "api_key", APIUrl: "http://api", Cookie: "c", RSS: []models.RSSConfig{{Name: "r", URL: "u"}}})
	assert.Error(t, err)
	err = s.UpsertSiteWithRSS(models.CMCT, models.SiteConfig{AuthMethod: "cookie", APIUrl: "http://api", Cookie: "c", RSS: []models.RSSConfig{}})
	assert.Error(t, err)
	err = s.UpsertSiteWithRSS(models.CMCT, models.SiteConfig{AuthMethod: "cookie", APIUrl: "http://api", Cookie: "c", RSS: []models.RSSConfig{{Name: "r", URL: "http://rss"}}})
	assert.NoError(t, err)
}

func TestListSites_ApplyDefaults(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	e := true
	_, _ = s.UpsertSite(models.MTEAM, models.SiteConfig{Enabled: &e, AuthMethod: "api_key", APIUrl: models.DefaultAPIUrlMTeam})
	_, _ = s.UpsertSite(models.CMCT, models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
	_, _ = s.UpsertSite(models.HDSKY, models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
	out, err := s.ListSites()
	require.NoError(t, err)
	if out[models.MTEAM].AuthMethod != "api_key" {
		t.Fatalf("mteam auth default")
	}
	if out[models.CMCT].AuthMethod != "cookie" {
		t.Fatalf("cmct auth default")
	}
	if out[models.HDSKY].AuthMethod != "cookie" {
		t.Fatalf("hdsky auth default")
	}
}

func TestReplaceSiteRSS_DeleteSite(t *testing.T) {
	db := NewTestDB(t)
	store := NewConfigStore(db)
	id, err := store.UpsertSite(models.SiteGroup("custom"), models.SiteConfig{Enabled: boolPtr(true), AuthMethod: "cookie", Cookie: "ck", APIUrl: "http://api"})
	require.NoError(t, err)
	err = store.ReplaceSiteRSS(id, []models.RSSConfig{{Name: "r", URL: "u", IntervalMinutes: 10}})
	require.NoError(t, err)
	require.NoError(t, store.DeleteSite("custom"))
	require.Error(t, store.DeleteSite("cmct"))
}
