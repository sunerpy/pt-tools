package models

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// v16：做种竞争度并进自动删种。升级前只开了竞争度、没开自动删种的安装打开自动删种，只按竞争度处理：
// 检查间隔沿用竞争度的（小于 5 分钟按 10 分钟），常规条件清零，不删免费到期的、不做紧急清理。其他安装不改设置。
func TestMigrationV15ToV16MergesPeerRatioIntoCleanup(t *testing.T) {
	open := func(t *testing.T, gs SettingsGlobal) (*gorm.DB, *SchemaManager, *spyHooks) {
		t.Helper()
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "v15.db")), &gorm.Config{})
		require.NoError(t, err)
		t.Cleanup(func() { closeTestGorm(db) })
		require.NoError(t, db.AutoMigrate(&SchemaVersion{}, &SettingsGlobal{}))
		require.NoError(t, db.Create(&SchemaVersion{Version: 15, Description: "test v15", AppVersion: "test"}).Error)
		gs.DownloadDir = "downloads"
		require.NoError(t, db.Create(&gs).Error)
		// 升级前的库没有 cleanup_emergency 这一列：AutoMigrate 加列时取默认值 1
		require.NoError(t, db.Model(&SettingsGlobal{}).Where("id = ?", gs.ID).Update("cleanup_emergency", true).Error)
		hooks := &spyHooks{}
		return db, NewSchemaManagerWithHooks(db, "test", hooks.BackupTable, hooks.EncryptCookie, hooks.DecryptCookie), hooks
	}
	read := func(t *testing.T, db *gorm.DB) SettingsGlobal {
		t.Helper()
		var gs SettingsGlobal
		require.NoError(t, db.First(&gs).Error)
		return gs
	}
	// 升级前的自动删种设置：条件都填过、开着免费到期删除（默认）
	before := SettingsGlobal{
		CleanupIntervalMin: 60, CleanupScope: "all", CleanupMaxSeedTimeH: 72, CleanupMinRatio: 2, CleanupMaxInactiveH: 12,
		CleanupSlowSeedTimeH: 48, CleanupSlowMaxRatio: 0.5, CleanupDelFreeExpired: true, CleanupDiskProtect: true,
		PeerRatioMaxSL: 8, PeerRatioRemoveData: true,
	}

	t.Run("只开了竞争度", func(t *testing.T) {
		gs := before
		gs.PeerRatioEnabled, gs.PeerRatioIntervalMin = true, 20
		db, sm, hooks := open(t, gs)
		require.NoError(t, db.Model(&SettingsGlobal{}).Where("1 = 1").Update("cleanup_enabled", false).Error)
		require.NoError(t, sm.RunMigrations())

		got := read(t, db)
		assert.True(t, got.CleanupEnabled)
		assert.Equal(t, "database", got.CleanupScope)
		assert.Equal(t, 20, got.CleanupIntervalMin, "沿用竞争度原来的间隔")
		assert.Zero(t, got.CleanupMaxSeedTimeH)
		assert.Zero(t, got.CleanupMinRatio)
		assert.Zero(t, got.CleanupMaxInactiveH)
		assert.Zero(t, got.CleanupSlowSeedTimeH)
		assert.Zero(t, got.CleanupSlowMaxRatio)
		assert.False(t, got.CleanupDelFreeExpired)
		assert.False(t, got.CleanupEmergency)
		assert.True(t, got.PeerRatioEnabled)
		assert.Equal(t, 8.0, got.PeerRatioMaxSL)
		assert.True(t, got.PeerRatioRemoveData)
		assert.EqualValues(t, 1, hooks.backupCalls.Load(), "改设置前先备份 settings_globals")
		version, err := sm.GetCurrentVersion()
		require.NoError(t, err)
		assert.Equal(t, CurrentSchemaVersion, version)

		// 中途退出、版本号没写上时再跑一次：改过的行自动删种已经开着，不再改
		require.NoError(t, db.Model(&SettingsGlobal{}).Where("1 = 1").Update("cleanup_max_seed_time_h", 5).Error)
		require.NoError(t, sm.migrateV15ToV16(db))
		assert.Equal(t, 5, read(t, db).CleanupMaxSeedTimeH)
	})

	t.Run("竞争度间隔小于 5 分钟按 10 分钟", func(t *testing.T) {
		gs := before
		gs.PeerRatioEnabled, gs.PeerRatioIntervalMin = true, 3
		db, sm, _ := open(t, gs)
		require.NoError(t, db.Model(&SettingsGlobal{}).Where("1 = 1").Update("cleanup_enabled", false).Error)
		require.NoError(t, sm.RunMigrations())
		got := read(t, db)
		assert.True(t, got.CleanupEnabled)
		assert.Equal(t, 10, got.CleanupIntervalMin)
	})

	for name, gs := range map[string]SettingsGlobal{
		"两个都开着":   {CleanupEnabled: true, PeerRatioEnabled: true, PeerRatioIntervalMin: 20},
		"都没开":     {},
		"只开了自动删种": {CleanupEnabled: true},
	} {
		t.Run(name, func(t *testing.T) {
			g := before
			g.CleanupEnabled, g.PeerRatioEnabled, g.PeerRatioIntervalMin = gs.CleanupEnabled, gs.PeerRatioEnabled, gs.PeerRatioIntervalMin
			db, sm, hooks := open(t, g)
			require.NoError(t, db.Model(&SettingsGlobal{}).Where("1 = 1").Update("cleanup_enabled", gs.CleanupEnabled).Error)
			require.NoError(t, sm.RunMigrations())
			got := read(t, db)
			assert.Equal(t, gs.CleanupEnabled, got.CleanupEnabled)
			assert.Equal(t, "all", got.CleanupScope)
			assert.Equal(t, 60, got.CleanupIntervalMin)
			assert.Equal(t, 72, got.CleanupMaxSeedTimeH)
			assert.True(t, got.CleanupDelFreeExpired)
			assert.True(t, got.CleanupEmergency, "紧急清理照升级前一样做")
			assert.Zero(t, hooks.backupCalls.Load(), "没有要改的行时不用备份")
		})
	}
}

// 有要改的行、却没有备份钩子时迁移报错，不改设置（同 v9）；没有要改的行时不需要钩子。
func TestMigrationV16RequiresBackupHookOnlyWhenChanging(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "v15.db")), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { closeTestGorm(db) })
	require.NoError(t, db.AutoMigrate(&SchemaVersion{}, &SettingsGlobal{}))
	sm := NewSchemaManager(db, "test")
	require.NoError(t, sm.migrateV15ToV16(db), "没有全局设置时什么也不做")

	require.NoError(t, db.Create(&SettingsGlobal{DownloadDir: "downloads", PeerRatioEnabled: true}).Error)
	require.Error(t, sm.migrateV15ToV16(db))
	var gs SettingsGlobal
	require.NoError(t, db.First(&gs).Error)
	assert.False(t, gs.CleanupEnabled)
}
