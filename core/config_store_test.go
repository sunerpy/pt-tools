package core

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
	_ "github.com/sunerpy/pt-tools/site/v2/definitions"
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
	if err := store.UpsertSiteWithRSS(models.SiteGroup("springsunday"), sc); err != nil {
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
	if len(cfg.Sites[models.SiteGroup("springsunday")].RSS) != 1 {
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
	err = s.UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{AuthMethod: "invalid", APIUrl: "http://x", RSS: []models.RSSConfig{{Name: "r", URL: "http://u"}}})
	require.Error(t, err)
	err = s.UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{AuthMethod: "cookie", APIUrl: "", RSS: []models.RSSConfig{{Name: "r", URL: "http://u"}}})
	require.Error(t, err)
	err = s.UpsertSiteWithRSS(models.SiteGroup("mteam"), models.SiteConfig{AuthMethod: "api_key", APIUrl: "https://api.m-team.cc", APIKey: "", RSS: []models.RSSConfig{{Name: "r", URL: "http://u"}}})
	require.Error(t, err)
	e := true
	err = s.UpsertSiteWithRSS(models.SiteGroup("mteam"), models.SiteConfig{Enabled: &e, AuthMethod: "api_key", APIUrl: "https://api.m-team.cc", APIKey: "k", RSS: []models.RSSConfig{{Name: "r", URL: "http://u"}}})
	require.NoError(t, err)
}

func TestUpsertSiteWithRSS_SaveAndList(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	sc := models.SiteConfig{Enabled: boolPtr(true), AuthMethod: "cookie", Cookie: "c", APIUrl: "http://api", RSS: []models.RSSConfig{{Name: "r", URL: "http://rss", IntervalMinutes: 10}}}
	require.NoError(t, s.UpsertSiteWithRSS(models.SiteGroup("springsunday"), sc))
	out, err := s.ListSites()
	require.NoError(t, err)
	require.Equal(t, 1, len(out))
	require.Equal(t, "cookie", out[models.SiteGroup("springsunday")].AuthMethod)
}

func TestAppendRSSToSite_AppendsOneRowAndPublishesConfigChanged(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	site := models.SiteSetting{Name: "mteam", Enabled: true, AuthMethod: "api_key", APIUrl: "https://api.m-team.cc", APIKey: "k"}
	require.NoError(t, db.DB.Create(&site).Error)
	downloader := models.DownloaderSetting{Name: "qbit", Type: "qbittorrent", URL: "http://127.0.0.1:8080", IsDefault: true, Enabled: true}
	require.NoError(t, db.DB.Create(&downloader).Error)
	rule := models.FilterRule{Name: "free", Pattern: "*", Enabled: true}
	require.NoError(t, db.DB.Create(&rule).Error)
	existing := []models.RSSSubscription{
		{SiteID: site.ID, Name: "old-a", URL: "https://example.com/a", Category: "cat-a", Tag: "tag-a", IntervalMinutes: 10, DownloadPath: "/downloads/a", NotifyMode: "all", NotifyConfIDs: "[1]", MaxNotificationsPerHour: 5},
		{SiteID: site.ID, Name: "old-b", URL: "https://example.com/b", Category: "cat-b", Tag: "tag-b", IntervalMinutes: 20, DownloadPath: "/downloads/b", NotifyMode: "filtered", NotifyConfIDs: "[]", MaxNotificationsPerHour: 10},
	}
	require.NoError(t, db.DB.Create(&existing).Error)

	_, ch, cancel := events.Subscribe(1)
	defer cancel()
	created, err := s.AppendRSSToSite("mteam", models.RSSConfig{
		Name:                    "new",
		URL:                     "https://example.com/new",
		Category:                "cat-new",
		Tag:                     "tag-new",
		IntervalMinutes:         1,
		DownloaderID:            &downloader.ID,
		DownloadPath:            "/downloads/new",
		FilterMode:              models.FilterMode("any"),
		FilterRuleIDs:           []uint{rule.ID},
		NotifyMode:              "all",
		NotifyConfIDs:           "[2]",
		MaxNotificationsPerHour: 7,
	})
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	assert.Equal(t, models.MinIntervalMinutes, created.IntervalMinutes)

	select {
	case event := <-ch:
		assert.Equal(t, events.ConfigChanged, event.Type)
		assert.Equal(t, "sites", event.Source)
	case <-time.After(time.Second):
		t.Fatal("expected ConfigChanged event")
	}

	var rows []models.RSSSubscription
	require.NoError(t, db.DB.Where("site_id = ?", site.ID).Order("id ASC").Find(&rows).Error)
	require.Len(t, rows, 3)
	assert.Equal(t, existing[0].Name, rows[0].Name)
	assert.Equal(t, existing[0].URL, rows[0].URL)
	assert.Equal(t, existing[0].Category, rows[0].Category)
	assert.Equal(t, existing[0].Tag, rows[0].Tag)
	assert.Equal(t, existing[0].DownloadPath, rows[0].DownloadPath)
	assert.Equal(t, existing[1].Name, rows[1].Name)
	assert.Equal(t, existing[1].URL, rows[1].URL)
	assert.Equal(t, "new", rows[2].Name)
	assert.Equal(t, "https://example.com/new", rows[2].URL)
	assert.Equal(t, "cat-new", rows[2].Category)
	assert.Equal(t, "tag-new", rows[2].Tag)
	assert.Equal(t, models.MinIntervalMinutes, rows[2].IntervalMinutes)
	require.NotNil(t, rows[2].DownloaderID)
	assert.Equal(t, downloader.ID, *rows[2].DownloaderID)
	assert.Equal(t, "/downloads/new", rows[2].DownloadPath)
	assert.Equal(t, models.FilterMode("any"), rows[2].FilterMode)
	assert.Equal(t, "all", rows[2].NotifyMode)
	assert.Equal(t, "[2]", rows[2].NotifyConfIDs)
	assert.Equal(t, 7, rows[2].MaxNotificationsPerHour)

	var associations []models.RSSFilterAssociation
	require.NoError(t, db.DB.Where("rss_id = ?", created.ID).Find(&associations).Error)
	require.Len(t, associations, 1)
	assert.Equal(t, rule.ID, associations[0].FilterRuleID)
}

func TestAppendRSSToSite_RejectsDuplicateURLWithoutInsert(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	site := models.SiteSetting{Name: "mteam", Enabled: true, AuthMethod: "api_key", APIUrl: "https://api.m-team.cc", APIKey: "k"}
	require.NoError(t, db.DB.Create(&site).Error)
	require.NoError(t, db.DB.Create(&models.RSSSubscription{SiteID: site.ID, Name: "old", URL: "https://example.com/rss", IntervalMinutes: 10}).Error)

	_, err = s.AppendRSSToSite("mteam", models.RSSConfig{Name: "dup", URL: "  HTTPS://EXAMPLE.COM/RSS  ", IntervalMinutes: 10})
	require.Error(t, err)

	var count int64
	require.NoError(t, db.DB.Model(&models.RSSSubscription{}).Where("site_id = ?", site.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestAppendRSSToSite_RejectsNonexistentSite(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)

	_, err = s.AppendRSSToSite("missing", models.RSSConfig{Name: "rss", URL: "https://example.com/rss", IntervalMinutes: 10})
	require.Error(t, err)
}

func TestAppendRSSToSite_RejectsNonexistentDownloader(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	site := models.SiteSetting{Name: "mteam", Enabled: true, AuthMethod: "api_key", APIUrl: "https://api.m-team.cc", APIKey: "k"}
	require.NoError(t, db.DB.Create(&site).Error)
	missingID := uint(404)

	_, err = s.AppendRSSToSite("mteam", models.RSSConfig{Name: "rss", URL: "https://example.com/rss", IntervalMinutes: 10, DownloaderID: &missingID})
	require.Error(t, err)

	var count int64
	require.NoError(t, db.DB.Model(&models.RSSSubscription{}).Where("site_id = ?", site.ID).Count(&count).Error)
	assert.Zero(t, count)
}

func TestDeleteRSSFromSite_DeletesRowAndAssociationsAndPublishes(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	site := models.SiteSetting{Name: "mteam", Enabled: true, AuthMethod: "api_key", APIUrl: "https://api.m-team.cc", APIKey: "k"}
	require.NoError(t, db.DB.Create(&site).Error)
	rule := models.FilterRule{Name: "free", Pattern: "*", Enabled: true}
	require.NoError(t, db.DB.Create(&rule).Error)
	target := models.RSSSubscription{SiteID: site.ID, Name: "to-delete", URL: "https://example.com/del", IntervalMinutes: 10}
	require.NoError(t, db.DB.Create(&target).Error)
	keep := models.RSSSubscription{SiteID: site.ID, Name: "keep", URL: "https://example.com/keep", IntervalMinutes: 10}
	require.NoError(t, db.DB.Create(&keep).Error)
	assoc := models.NewRSSFilterAssociationDB(db.DB)
	require.NoError(t, assoc.SetFilterRulesForRSS(target.ID, []uint{rule.ID}))

	_, ch, cancel := events.Subscribe(1)
	defer cancel()
	deleted, err := s.DeleteRSSFromSite("mteam", target.ID)
	require.NoError(t, err)
	assert.Equal(t, target.ID, deleted.ID)
	assert.Equal(t, "to-delete", deleted.Name)

	select {
	case event := <-ch:
		assert.Equal(t, events.ConfigChanged, event.Type)
		assert.Equal(t, "sites", event.Source)
	case <-time.After(time.Second):
		t.Fatal("expected ConfigChanged event")
	}

	var rows []models.RSSSubscription
	require.NoError(t, db.DB.Where("site_id = ?", site.ID).Order("id ASC").Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, "keep", rows[0].Name)

	var assocCount int64
	require.NoError(t, db.DB.Model(&models.RSSFilterAssociation{}).Where("rss_id = ?", target.ID).Count(&assocCount).Error)
	assert.Zero(t, assocCount)
}

func TestDeleteRSSFromSite_RejectsNonexistentRSS(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	site := models.SiteSetting{Name: "mteam", Enabled: true, AuthMethod: "api_key", APIUrl: "https://api.m-team.cc", APIKey: "k"}
	require.NoError(t, db.DB.Create(&site).Error)
	other := models.RSSSubscription{SiteID: site.ID, Name: "other", URL: "https://example.com/other", IntervalMinutes: 10}
	require.NoError(t, db.DB.Create(&other).Error)

	_, err = s.DeleteRSSFromSite("mteam", 9999)
	require.Error(t, err)

	var count int64
	require.NoError(t, db.DB.Model(&models.RSSSubscription{}).Where("site_id = ?", site.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestDeleteRSSFromSite_RejectsCrossSiteRSS(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	siteA := models.SiteSetting{Name: "mteam", Enabled: true, AuthMethod: "api_key", APIUrl: "https://api.m-team.cc", APIKey: "k"}
	require.NoError(t, db.DB.Create(&siteA).Error)
	siteB := models.SiteSetting{Name: "hdsky", Enabled: true, AuthMethod: "cookie", APIUrl: "https://hdsky.me"}
	require.NoError(t, db.DB.Create(&siteB).Error)
	rssB := models.RSSSubscription{SiteID: siteB.ID, Name: "b-rss", URL: "https://example.com/b", IntervalMinutes: 10}
	require.NoError(t, db.DB.Create(&rssB).Error)

	_, err = s.DeleteRSSFromSite("mteam", rssB.ID)
	require.Error(t, err)

	var count int64
	require.NoError(t, db.DB.Model(&models.RSSSubscription{}).Where("site_id = ?", siteB.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestListRSSForSite_ReturnsRowsWithFilterRuleIDs(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	site := models.SiteSetting{Name: "mteam", Enabled: true, AuthMethod: "api_key", APIUrl: "https://api.m-team.cc", APIKey: "k"}
	require.NoError(t, db.DB.Create(&site).Error)
	rule := models.FilterRule{Name: "free", Pattern: "*", Enabled: true}
	require.NoError(t, db.DB.Create(&rule).Error)
	r1 := models.RSSSubscription{SiteID: site.ID, Name: "feed-1", URL: "https://example.com/1", IntervalMinutes: 10}
	require.NoError(t, db.DB.Create(&r1).Error)
	r2 := models.RSSSubscription{SiteID: site.ID, Name: "feed-2", URL: "https://example.com/2", IntervalMinutes: 10}
	require.NoError(t, db.DB.Create(&r2).Error)
	assoc := models.NewRSSFilterAssociationDB(db.DB)
	require.NoError(t, assoc.SetFilterRulesForRSS(r1.ID, []uint{rule.ID}))

	list, err := s.ListRSSForSite("mteam")
	require.NoError(t, err)
	require.Len(t, list, 2)
	byName := map[string]models.RSSConfig{}
	for _, r := range list {
		byName[r.Name] = r
	}
	assert.Equal(t, []uint{rule.ID}, byName["feed-1"].FilterRuleIDs)
	assert.Empty(t, byName["feed-2"].FilterRuleIDs)
}

func TestListRSSForSite_RejectsNonexistentSite(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)

	_, err = s.ListRSSForSite("missing")
	require.Error(t, err)
}

func TestUpsertSiteWithRSS_PreservesLoginStateCookieForAPIKeySite(t *testing.T) {
	writeTestSecretKey(t)
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)

	loginCookie := "session=login-state; uid=123"
	require.NoError(t, s.UpsertSiteWithRSS(models.SiteGroup("mteam"), models.SiteConfig{
		AuthMethod: "api_key",
		Cookie:     loginCookie,
		APIKey:     "api-key",
		APIUrl:     "https://api.m-team.cc",
		RSS:        []models.RSSConfig{{Name: "r", URL: "http://rss", IntervalMinutes: 10}},
	}))

	var saved models.SiteSetting
	require.NoError(t, db.DB.Where("name = ?", "mteam").First(&saved).Error)
	require.NotEmpty(t, saved.CookieEncrypted)
	plain, err := s.DecryptCookie(saved.CookieEncrypted)
	require.NoError(t, err)
	require.Equal(t, loginCookie, plain)

	require.NoError(t, s.UpsertSiteWithRSS(models.SiteGroup("mteam"), models.SiteConfig{
		AuthMethod: "api_key",
		APIKey:     "api-key-updated",
		APIUrl:     "https://api.m-team.cc",
		RSS:        []models.RSSConfig{{Name: "r", URL: "http://rss", IntervalMinutes: 10}},
	}))

	var updated models.SiteSetting
	require.NoError(t, db.DB.Where("name = ?", "mteam").First(&updated).Error)
	require.Empty(t, updated.Cookie)
	require.NotEmpty(t, updated.CookieEncrypted)
	plain, err = s.DecryptCookie(updated.CookieEncrypted)
	require.NoError(t, err)
	require.Equal(t, loginCookie, plain)

	loaded, err := s.GetSiteConf(models.SiteGroup("mteam"))
	require.NoError(t, err)
	require.Equal(t, loginCookie, loaded.Cookie)
}

func TestUpsertSiteWithRSS_CookieAuthStillEncryptsCookie(t *testing.T) {
	writeTestSecretKey(t)
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)

	cookie := "sid=cookie-auth"
	require.NoError(t, s.UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{
		AuthMethod: "cookie",
		Cookie:     cookie,
		APIUrl:     "http://api",
		RSS:        []models.RSSConfig{{Name: "r", URL: "http://rss", IntervalMinutes: 10}},
	}))

	var saved models.SiteSetting
	require.NoError(t, db.DB.Where("name = ?", "springsunday").First(&saved).Error)
	require.NotEmpty(t, saved.CookieEncrypted)
	plain, err := s.DecryptCookie(saved.CookieEncrypted)
	require.NoError(t, err)
	require.Equal(t, cookie, plain)

	loaded, err := s.GetSiteConf(models.SiteGroup("springsunday"))
	require.NoError(t, err)
	require.Equal(t, cookie, loaded.Cookie)
	require.NoError(t, s.UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{
		AuthMethod: "cookie",
		APIUrl:     "http://api",
		RSS:        []models.RSSConfig{{Name: "r", URL: "http://rss", IntervalMinutes: 10}},
	}))

	var updated models.SiteSetting
	require.NoError(t, db.DB.Where("name = ?", "springsunday").First(&updated).Error)
	require.Empty(t, updated.Cookie)
	require.NotEmpty(t, updated.CookieEncrypted)
	plain, err = s.DecryptCookie(updated.CookieEncrypted)
	require.NoError(t, err)
	require.Equal(t, cookie, plain)
}

func TestUpsertSiteWithRSS_CookieAndAPIKeyAcceptsEmptyCookieWhenStored(t *testing.T) {
	writeTestSecretKey(t)
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)

	storedCookie := "uid=123; session=stored"
	require.NoError(t, s.UpsertSiteWithRSS(models.SiteGroup("hddolby"), models.SiteConfig{
		AuthMethod: "cookie_and_api_key",
		Cookie:     storedCookie,
		APIKey:     "api-key",
		APIUrl:     "https://www.hddolby.com",
		RSS:        []models.RSSConfig{{Name: "r", URL: "http://rss", IntervalMinutes: 10}},
	}))

	require.NoError(t, s.UpsertSiteWithRSS(models.SiteGroup("hddolby"), models.SiteConfig{
		AuthMethod: "cookie_and_api_key",
		APIKey:     "api-key-updated",
		APIUrl:     "https://www.hddolby.com",
		RSS:        []models.RSSConfig{{Name: "r", URL: "http://rss", IntervalMinutes: 10}},
	}))

	var updated models.SiteSetting
	require.NoError(t, db.DB.Where("name = ?", "hddolby").First(&updated).Error)
	require.Empty(t, updated.Cookie)
	require.NotEmpty(t, updated.CookieEncrypted)
	plain, err := s.DecryptCookie(updated.CookieEncrypted)
	require.NoError(t, err)
	require.Equal(t, storedCookie, plain)
}

func TestUpsertSiteWithRSS_CookieAndAPIKeyRequiresCookieWhenNoneStored(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)

	err = s.UpsertSiteWithRSS(models.SiteGroup("hddolby"), models.SiteConfig{
		AuthMethod: "cookie_and_api_key",
		APIKey:     "api-key",
		APIUrl:     "https://www.hddolby.com",
		RSS:        []models.RSSConfig{{Name: "r", URL: "http://rss", IntervalMinutes: 10}},
	})
	require.EqualError(t, err, "Cookie 不能为空")
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
	id, err := s.UpsertSite(models.SiteGroup("springsunday"), sc)
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
	siteID, err := s.UpsertSite(models.SiteGroup("springsunday"), models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
	if err != nil {
		t.Fatalf("upsert site: %v", err)
	}
	if err = s.ReplaceSiteRSS(siteID, []models.RSSConfig{{Name: "r1", URL: "http://example/rss", Tag: "tag", IntervalMinutes: 10}}); err != nil {
		t.Fatalf("rss: %v", err)
	}
	sc, err := s.GetSiteConf(models.SiteGroup("springsunday"))
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
	id, err := s.UpsertSite(models.SiteGroup("hdsky"), models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
	require.NoError(t, err)
	require.NoError(t, s.ReplaceSiteRSS(id, []models.RSSConfig{{Name: "r", URL: "http://rss", IntervalMinutes: 10}}))
	sc, err := s.GetSiteConf(models.SiteGroup("hdsky"))
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
	_, _ = s.UpsertSite(models.SiteGroup("springsunday"), models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
	assert.Error(t, s.DeleteSite("cmct"))
	_, _ = s.UpsertSite(models.SiteGroup("custom"), models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
	assert.NoError(t, s.DeleteSite("custom"))
}

func TestUpsertSiteWithRSS_Validation(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	err = s.UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{AuthMethod: "bad", APIUrl: "http://api", RSS: []models.RSSConfig{{Name: "r", URL: "u"}}})
	assert.Error(t, err)
	// 预置站点（SpringSunday）不需要 APIUrl，由后端常量提供
	err = s.UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{AuthMethod: "cookie", APIUrl: "", Cookie: "c", RSS: []models.RSSConfig{{Name: "r", URL: "u"}}})
	assert.NoError(t, err) // 预置站点允许空 APIUrl
	err = s.UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{AuthMethod: "cookie", APIUrl: "http://api", Cookie: "c", APIKey: "k", RSS: []models.RSSConfig{{Name: "r", URL: "u"}}})
	assert.Error(t, err)
	err = s.UpsertSiteWithRSS(models.SiteGroup("mteam"), models.SiteConfig{AuthMethod: "api_key", APIUrl: "http://api", Cookie: "c", APIKey: "k", RSS: []models.RSSConfig{{Name: "r", URL: "u"}}})
	assert.NoError(t, err)
	// RSS 列表允许为空
	err = s.UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{AuthMethod: "cookie", APIUrl: "http://api", Cookie: "c", RSS: []models.RSSConfig{}})
	assert.NoError(t, err)
	err = s.UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{AuthMethod: "cookie", APIUrl: "http://api", Cookie: "c", RSS: []models.RSSConfig{{Name: "r", URL: "http://rss"}}})
	assert.NoError(t, err)
}

func TestListSites_ApplyDefaults(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	e := true
	_, _ = s.UpsertSite(models.SiteGroup("mteam"), models.SiteConfig{Enabled: &e, AuthMethod: "api_key", APIUrl: "https://api.m-team.cc"})
	_, _ = s.UpsertSite(models.SiteGroup("springsunday"), models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
	_, _ = s.UpsertSite(models.SiteGroup("hdsky"), models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
	out, err := s.ListSites()
	require.NoError(t, err)
	if out[models.SiteGroup("mteam")].AuthMethod != "api_key" {
		t.Fatalf("mteam auth default")
	}
	if out[models.SiteGroup("springsunday")].AuthMethod != "cookie" {
		t.Fatalf("cmct auth default")
	}
	if out[models.SiteGroup("hdsky")].AuthMethod != "cookie" {
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

func TestGetGlobalOnly_NotFound(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	// Don't save any global settings, so GetGlobalOnly should return default values
	out, err := s.GetGlobalOnly()
	// GetGlobalOnly returns default values when no record exists
	assert.NoError(t, err)
	assert.NotNil(t, out)
}

func TestSaveGlobalSettings_AllValidations(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)

	tests := []struct {
		name      string
		settings  models.SettingsGlobal
		wantError bool
	}{
		{
			name:      "empty download dir",
			settings:  models.SettingsGlobal{DownloadDir: "", DefaultIntervalMinutes: 10},
			wantError: true,
		},
		{
			name:      "valid settings",
			settings:  models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10},
			wantError: false,
		},
		{
			name:      "interval below minimum gets coerced",
			settings:  models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.SaveGlobalSettings(tt.settings)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSaveQbitSettings_AllValidations(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)

	tests := []struct {
		name      string
		settings  models.QbitSettings
		wantError bool
	}{
		{
			name:      "enabled but missing URL",
			settings:  models.QbitSettings{Enabled: true, URL: "", User: "u", Password: "p"},
			wantError: true,
		},
		{
			name:      "enabled but missing user",
			settings:  models.QbitSettings{Enabled: true, URL: "http://localhost", User: "", Password: "p"},
			wantError: true,
		},
		{
			name:      "enabled but missing password",
			settings:  models.QbitSettings{Enabled: true, URL: "http://localhost", User: "u", Password: ""},
			wantError: true,
		},
		{
			name:      "disabled with missing fields still requires validation",
			settings:  models.QbitSettings{Enabled: false, URL: "", User: "", Password: ""},
			wantError: true,
		},
		{
			name:      "valid enabled settings",
			settings:  models.QbitSettings{Enabled: true, URL: "http://localhost", User: "u", Password: "p"},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.SaveQbitSettings(tt.settings)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDeleteSite_AllCases(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)

	// Test deleting non-existent site
	err = s.DeleteSite("non-existent")
	assert.Error(t, err)

	// Test deleting preset site (should fail)
	e := true
	_, _ = s.UpsertSite(models.SiteGroup("springsunday"), models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
	err = s.DeleteSite("cmct")
	assert.Error(t, err)

	// Test deleting custom site (should succeed)
	_, _ = s.UpsertSite(models.SiteGroup("custom-site"), models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
	err = s.DeleteSite("custom-site")
	assert.NoError(t, err)
}

func TestGetQbitSettings_NotFound(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	// Don't save any qbit settings
	out, err := s.GetQbitSettings()
	// Should return default/empty settings without error
	assert.NoError(t, err)
	assert.NotNil(t, out)
}

func TestReplaceSiteRSS_EmptyRSS(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	e := true
	id, err := s.UpsertSite(models.SiteGroup("springsunday"), models.SiteConfig{Enabled: &e, AuthMethod: "cookie", Cookie: "c"})
	require.NoError(t, err)
	// Replace with empty RSS list
	err = s.ReplaceSiteRSS(id, []models.RSSConfig{})
	assert.NoError(t, err)
}

func TestGetSiteConf_NotFound(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	s := NewConfigStore(db)
	// Try to get non-existent site
	_, err = s.GetSiteConf(models.SiteGroup("non-existent"))
	assert.Error(t, err)
}

func TestUpdateAdminPassword_NotFound(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.InitLogger(zap.NewNop())
	s := NewConfigStore(db)
	// Try to update password for non-existent user
	err = s.UpdateAdminPassword("non-existent", "newhash")
	assert.Error(t, err)
}

func TestGetAdmin_NotFound(t *testing.T) {
	db, err := NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.InitLogger(zap.NewNop())
	s := NewConfigStore(db)
	// Try to get non-existent admin
	_, err = s.GetAdmin("non-existent")
	assert.Error(t, err)
}
