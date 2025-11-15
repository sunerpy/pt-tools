package core

import (
	"path/filepath"

	"github.com/glebarez/sqlite"
	"github.com/sunerpy/pt-tools/models"
	"gorm.io/gorm"
)

// NewTempDBDir creates a temporary sqlite db under the given directory and migrates schema
func NewTempDBDir(dir string) (*models.TorrentDB, error) {
	dbFile := filepath.Join(dir, "torrents.db")
	db, err := gorm.Open(sqlite.Open("file:"+dbFile), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&models.SettingsGlobal{}, &models.QbitSettings{}, &models.SiteSetting{}, &models.RSSSubscription{}, &models.TorrentInfo{}, &models.AdminUser{}); err != nil {
		return nil, err
	}
	return &models.TorrentDB{DB: db}, nil
}
