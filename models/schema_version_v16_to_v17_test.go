package models

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// v17：免费结束时删除还是暂停改成每条 RSS 自己选（free_end_delete）。升级前开着全局「免费结束自动删除」时，
// 开着免费结束处理的 RSS 与已推送的记录都记成删除，处理方式和升级前一样；没开时都是暂停。全局那一列不动。
func TestMigrationV16ToV17CarriesGlobalFreeEndDelete(t *testing.T) {
	open := func(t *testing.T, autoDelete bool) (*gorm.DB, *SchemaManager) {
		t.Helper()
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "v16.db")), &gorm.Config{})
		require.NoError(t, err)
		t.Cleanup(func() { closeTestGorm(db) })
		require.NoError(t, db.AutoMigrate(&SchemaVersion{}, &SettingsGlobal{}, &RSSSubscription{}, &TorrentInfo{}))
		require.NoError(t, db.Create(&SchemaVersion{Version: 16, Description: "test v16", AppVersion: "test"}).Error)
		require.NoError(t, db.Create(&SettingsGlobal{DownloadDir: "downloads", AutoDeleteOnFreeEnd: autoDelete}).Error)
		for i, pause := range []bool{true, false} {
			require.NoError(t, db.Create(&RSSSubscription{
				SiteID: 1, Name: "rss", URL: "https://example.test/rss/" + string(rune('a'+i)), IntervalMinutes: 10, PauseOnFreeEnd: pause,
			}).Error)
			require.NoError(t, db.Create(&TorrentInfo{
				SiteName: "s", TorrentID: string(rune('a' + i)), Title: "t", PauseOnFreeEnd: pause,
			}).Error)
		}
		hooks := &spyHooks{}
		return db, NewSchemaManagerWithHooks(db, "test", hooks.BackupTable, hooks.EncryptCookie, hooks.DecryptCookie)
	}
	deletes := func(t *testing.T, db *gorm.DB, model any) map[bool]bool {
		t.Helper()
		var rows []struct{ PauseOnFreeEnd, FreeEndDelete bool }
		require.NoError(t, db.Model(model).Select("pause_on_free_end, free_end_delete").Scan(&rows).Error)
		require.Len(t, rows, 2)
		out := map[bool]bool{}
		for _, r := range rows {
			out[r.PauseOnFreeEnd] = r.FreeEndDelete
		}
		return out
	}

	t.Run("升级前开着全局免费结束自动删除", func(t *testing.T) {
		db, sm := open(t, true)
		require.NoError(t, sm.RunMigrations())
		for _, model := range []any{&RSSSubscription{}, &TorrentInfo{}} {
			got := deletes(t, db, model)
			assert.True(t, got[true], "%T：开着免费结束处理的记成删除", model)
			assert.False(t, got[false], "%T：没开免费结束处理的不记", model)
		}
		var gs SettingsGlobal
		require.NoError(t, db.First(&gs).Error)
		assert.True(t, gs.AutoDeleteOnFreeEnd, "全局那一列不动，回退到旧版时含义不变")
		version, err := sm.GetCurrentVersion()
		require.NoError(t, err)
		assert.Equal(t, CurrentSchemaVersion, version)

		// 中途退出、版本号没写上时再跑一次：结果一样
		require.NoError(t, sm.migrateV16ToV17(db))
		assert.True(t, deletes(t, db, &RSSSubscription{})[true])
	})

	t.Run("升级前没开：都是暂停", func(t *testing.T) {
		db, sm := open(t, false)
		require.NoError(t, sm.RunMigrations())
		for _, model := range []any{&RSSSubscription{}, &TorrentInfo{}} {
			got := deletes(t, db, model)
			assert.False(t, got[true], "%T", model)
			assert.False(t, got[false], "%T", model)
		}
	})

	t.Run("升级前的库还没有这一列", func(t *testing.T) {
		db, sm := open(t, true)
		require.NoError(t, db.Migrator().DropColumn(&RSSSubscription{}, "FreeEndDelete"))
		require.NoError(t, db.Migrator().DropColumn(&TorrentInfo{}, "FreeEndDelete"))
		require.NoError(t, sm.RunMigrations())
		assert.True(t, deletes(t, db, &RSSSubscription{})[true])
		assert.True(t, deletes(t, db, &TorrentInfo{})[true])
	})

	t.Run("没有 RSS 与种子记录表", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "bare.db")), &gorm.Config{})
		require.NoError(t, err)
		t.Cleanup(func() { closeTestGorm(db) })
		require.NoError(t, db.AutoMigrate(&SchemaVersion{}, &SettingsGlobal{}))
		require.NoError(t, db.Create(&SettingsGlobal{DownloadDir: "downloads", AutoDeleteOnFreeEnd: true}).Error)
		sm := NewSchemaManager(db, "test")
		require.NoError(t, sm.migrateV16ToV17(db))
	})
}
