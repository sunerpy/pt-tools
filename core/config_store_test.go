package core

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/sunerpy/pt-tools/models"
	"gorm.io/gorm"
)

func newTempDB(t *testing.T) *models.TorrentDB {
	t.Helper()
	dir := t.TempDir()
	dbFile := filepath.Join(dir, "torrents.db")
	db, err := gorm.Open(sqlite.Open("file:"+dbFile), &gorm.Config{})
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
