package migration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/sunerpy/pt-tools/models"
)

func closedMigDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupTestDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	return db
}

func TestCreateBackup_MkdirError(t *testing.T) {
	db := setupTestDB(t)
	badDir := filepath.Join(os.DevNull, "nope")
	svc := NewMigrationServiceWithBackupDir(db, badDir)
	_, err := svc.CreateBackup()
	require.Error(t, err)
}

func TestCreateBackup_QueryError(t *testing.T) {
	db := closedMigDB(t)
	svc := NewMigrationServiceWithBackupDir(db, t.TempDir())
	_, err := svc.CreateBackup()
	require.Error(t, err)
}

func TestMigrateV1ToV2_FullSuccess(t *testing.T) {
	db := setupTestDB(t)
	svc := NewMigrationServiceWithBackupDir(db, t.TempDir())

	require.NoError(t, db.Create(&models.QbitSettings{
		URL: "http://qb:8080", User: "u", Password: "p", Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "hdsky", AuthMethod: "cookie"}).Error)
	require.NoError(t, db.Create(&models.SiteSetting{Name: "mteam", AuthMethod: "api_key"}).Error)

	res := svc.MigrateV1ToV2()
	require.True(t, res.Success)
	assert.Equal(t, 1, res.DownloadersMigrated)
	assert.Equal(t, 2, res.SitesMigrated)

	var dl models.DownloaderSetting
	require.NoError(t, db.Where("name = ?", "qbittorrent-default").First(&dl).Error)
	assert.Equal(t, "http://qb:8080", dl.URL)
	assert.True(t, dl.IsDefault)

	var site models.SiteSetting
	require.NoError(t, db.Where("name = ?", "hdsky").First(&site).Error)
	assert.Equal(t, "hdsky", site.DisplayName)
	assert.True(t, site.IsBuiltin)
}

func TestMigrateV1ToV2_BackupFailure(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.Create(&models.QbitSettings{URL: "http://x", User: "u", Password: "p"}).Error)
	svc := NewMigrationServiceWithBackupDir(db, filepath.Join(os.DevNull, "nope"))

	res := svc.MigrateV1ToV2()
	assert.False(t, res.Success)
	assert.Contains(t, res.Message, "创建备份失败")
}

func TestMigrateV1ToV2_TransactionFailureTriggersRestore(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.Create(&models.QbitSettings{URL: "http://x", User: "u", Password: "p"}).Error)
	tmp := t.TempDir()
	svc := NewMigrationServiceWithBackupDir(db, tmp)

	backupPath, err := svc.CreateBackup()
	require.NoError(t, err)
	require.FileExists(t, backupPath)

	require.NoError(t, db.Migrator().DropTable(&models.DownloaderSetting{}))

	res := svc.MigrateV1ToV2()
	assert.False(t, res.Success)
	assert.Contains(t, res.Message, "迁移失败")
}

func TestRestoreBackup_DeleteError(t *testing.T) {
	db := setupTestDB(t)
	svc := NewMigrationServiceWithBackupDir(db, t.TempDir())
	backupPath, err := svc.CreateBackup()
	require.NoError(t, err)

	require.NoError(t, db.Migrator().DropTable(&models.DownloaderSetting{}))
	err = svc.RestoreBackup(backupPath)
	require.Error(t, err)
}

func TestRestoreBackup_AllEntities(t *testing.T) {
	src := setupTestDB(t)
	tmp := t.TempDir()
	svc := NewMigrationServiceWithBackupDir(src, tmp)

	require.NoError(t, src.Create(&models.SettingsGlobal{DownloadDir: "/dl", DefaultIntervalMinutes: 12}).Error)
	require.NoError(t, src.Create(&models.QbitSettings{URL: "http://qb", User: "u", Password: "p"}).Error)
	require.NoError(t, src.Create(&models.SiteSetting{Name: "hdsky", AuthMethod: "cookie"}).Error)
	site := models.SiteSetting{}
	require.NoError(t, src.Where("name = ?", "hdsky").First(&site).Error)
	require.NoError(t, src.Create(&models.RSSSubscription{SiteID: site.ID, Name: "r", URL: "https://x", IntervalMinutes: 5}).Error)

	backupPath, err := svc.CreateBackup()
	require.NoError(t, err)

	dst := setupTestDB(t)
	dstSvc := NewMigrationServiceWithBackupDir(dst, tmp)
	require.NoError(t, dstSvc.RestoreBackup(backupPath))

	var gl models.SettingsGlobal
	require.NoError(t, dst.First(&gl).Error)
	assert.Equal(t, "/dl", gl.DownloadDir)

	var qb models.QbitSettings
	require.NoError(t, dst.First(&qb).Error)
	assert.Equal(t, "http://qb", qb.URL)

	var sites []models.SiteSetting
	require.NoError(t, dst.Find(&sites).Error)
	assert.Len(t, sites, 1)

	var rss []models.RSSSubscription
	require.NoError(t, dst.Find(&rss).Error)
	assert.Len(t, rss, 1)
}

func TestRestoreBackup_SaveQbitError(t *testing.T) {
	src := setupTestDB(t)
	tmp := t.TempDir()
	svc := NewMigrationServiceWithBackupDir(src, tmp)
	require.NoError(t, src.Create(&models.QbitSettings{URL: "http://x", User: "u", Password: "p"}).Error)
	backupPath, err := svc.CreateBackup()
	require.NoError(t, err)

	require.NoError(t, src.Exec("DROP TABLE qbit_settings").Error)
	require.NoError(t, src.Exec("CREATE TABLE qbit_settings (id INTEGER PRIMARY KEY, wrong_col TEXT NOT NULL)").Error)
	err = svc.RestoreBackup(backupPath)
	require.Error(t, err)
}

func TestRestoreBackup_SaveSiteError(t *testing.T) {
	src := setupTestDB(t)
	tmp := t.TempDir()
	svc := NewMigrationServiceWithBackupDir(src, tmp)
	require.NoError(t, src.Create(&models.SiteSetting{Name: "hdsky", AuthMethod: "cookie"}).Error)
	backupPath, err := svc.CreateBackup()
	require.NoError(t, err)

	require.NoError(t, src.Exec("DROP TABLE site_settings").Error)
	require.NoError(t, src.Exec("CREATE TABLE site_settings (id INTEGER PRIMARY KEY, wrong_col TEXT NOT NULL)").Error)
	err = svc.RestoreBackup(backupPath)
	require.Error(t, err)
}

func TestCreateBackup_QbitQueryError(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.Migrator().DropTable(&models.QbitSettings{}))
	svc := NewMigrationServiceWithBackupDir(db, t.TempDir())
	_, err := svc.CreateBackup()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "qBittorrent")
}

func TestCreateBackup_SitesQueryError(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.Migrator().DropTable(&models.SiteSetting{}))
	svc := NewMigrationServiceWithBackupDir(db, t.TempDir())
	_, err := svc.CreateBackup()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "站点")
}

func TestCreateBackup_RSSQueryError(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.Migrator().DropTable(&models.RSSSubscription{}))
	svc := NewMigrationServiceWithBackupDir(db, t.TempDir())
	_, err := svc.CreateBackup()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RSS")
}

func TestRestoreBackup_SaveGlobalError(t *testing.T) {
	src := setupTestDB(t)
	tmp := t.TempDir()
	svc := NewMigrationServiceWithBackupDir(src, tmp)
	require.NoError(t, src.Create(&models.SettingsGlobal{DownloadDir: "/dl", DefaultIntervalMinutes: 12}).Error)
	backupPath, err := svc.CreateBackup()
	require.NoError(t, err)

	require.NoError(t, src.Migrator().DropTable(&models.SettingsGlobal{}))
	err = svc.RestoreBackup(backupPath)
	require.Error(t, err)
}

func TestDumpTableJSON_DefaultDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.SiteSetting{}))
	require.NoError(t, db.Create(&models.SiteSetting{Name: "s", AuthMethod: "cookie"}).Error)

	path, err := DumpTableJSON(db, "site_settings", "", 8, 9)
	require.NoError(t, err)
	require.FileExists(t, path)
	assert.Contains(t, path, filepath.Join(".pt-tools", "backups"))
}

func TestDumpTableJSON_QueryError(t *testing.T) {
	db := closedMigDB(t)
	_, err := DumpTableJSON(db, "site_settings", t.TempDir(), 8, 9)
	require.Error(t, err)
}

func TestDumpTableJSON_MkdirError(t *testing.T) {
	db := setupTestDB(t)
	_, err := DumpTableJSON(db, "site_settings", filepath.Join(os.DevNull, "nope"), 8, 9)
	require.Error(t, err)
}
