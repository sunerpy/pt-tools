package core

import (
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

// TB 是测试里用到的几个方法（*testing.T、*testing.B 都有），这里不引用 testing 包。
type TB interface {
	Helper()
	TempDir() string
	Cleanup(func())
}

// OpenTestDB 在 t.TempDir() 下建临时库，测试结束时关掉。Windows 上打开着的文件删不掉，
// 不关的话 t.TempDir() 的清理会失败，测试被判为 FAIL。
func OpenTestDB(t TB) (*models.TorrentDB, error) {
	t.Helper()
	return OpenTestDBIn(t, t.TempDir())
}

// OpenTestDBIn 在 dir 下建临时库（dir 一般也是 t.TempDir()），测试结束时关掉。
func OpenTestDBIn(t TB, dir string) (*models.TorrentDB, error) {
	t.Helper()
	db, err := NewTempDBDir(dir)
	if err != nil {
		return nil, err
	}
	t.Cleanup(func() { CloseDB(db) })
	return db, nil
}

// CloseDB 关掉库的连接（nil 安全）。
func CloseDB(db *models.TorrentDB) {
	if db == nil || db.DB == nil {
		return
	}
	if sqlDB, err := db.DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

// NewTempDBDir creates a temporary sqlite db under the given directory and migrates schema.
// 测试库不像正式库那样只开一个连接：几个连接同时写时等锁（最多 10 秒），事务和正式库一样一开始就拿写锁
// （先读后写的事务不会因为升级写锁撞上别的连接而立刻报 database is locked）。
// 测试不需要掉电保护：日志放内存、不 fsync。Windows 上每次建表都落盘、刷盘，整套测试慢好几倍。
func NewTempDBDir(dir string) (*models.TorrentDB, error) {
	dbFile := filepath.Join(dir, "torrents.db")
	dsn := "file:" + dbFile + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(MEMORY)&_pragma=synchronous(OFF)&_txlock=immediate"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(
		&models.SettingsGlobal{},
		&models.QbitSettings{},
		&models.SiteSetting{},
		&models.RSSSubscription{},
		&models.TorrentInfo{},
		&models.TorrentInfoArchive{},
		&models.AdminUser{},
		&models.DownloaderSetting{},
		&models.SiteTemplate{},
		&models.FilterRule{},
		&models.RSSFilterAssociation{},
		&models.AssistantSetting{},
		&models.AssistantConversation{},
		&models.AssistantMessage{},
		&models.AssistantConfirmation{},
		&models.AssistantAttachment{},
		&models.NotificationTemplate{},
	); err != nil {
		return nil, err
	}
	return &models.TorrentDB{DB: db}, nil
}
