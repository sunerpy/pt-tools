package models

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupV11MigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "v11.db")), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { closeTestGorm(db) })
	require.NoError(t, db.AutoMigrate(&SchemaVersion{}, &TorrentInfo{}, &FilterRule{}))
	require.NoError(t, db.Create(&SchemaVersion{Version: 11, Description: "test v11", AppVersion: "test"}).Error)
	return db
}

// v12：旧记录按 is_free 与 free_end_time 回填免费状态。免费但没有结束时间的是 unknown，不推断成永久；
// 要求免费只回填 RSS 免费通道与要求免费的过滤规则；不动推送、暂停、下载器这些字段，不自动删除或恢复任务。
func TestMigrationV11ToV12BackfillsFreeValidity(t *testing.T) {
	db := setupV11MigrationDB(t)
	strict := FilterRule{Name: "strict", Pattern: "a", RequireFree: true}
	loose := FilterRule{Name: "loose", Pattern: "b", RequireFree: false}
	require.NoError(t, db.Create(&strict).Error)
	require.NoError(t, db.Create(&loose).Error)
	// RequireFree 的零值不会写进库（默认 true），明确改成 false
	require.NoError(t, db.Model(&FilterRule{}).Where("id = ?", loose.ID).Update("require_free", false).Error)
	end := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	pushed := true
	rows := []TorrentInfo{
		{SiteName: "s", TorrentID: "free-end", IsFree: true, FreeLevel: "FREE", FreeEndTime: &end, DownloadSource: "free_download"},
		{SiteName: "s", TorrentID: "free-no-end", IsFree: true, FreeLevel: "FREE", DownloadSource: "free_download",
			IsPushed: &pushed, DownloaderTaskID: "abc", PauseOnFreeEnd: true},
		{SiteName: "s", TorrentID: "not-free", IsFree: false, FreeLevel: "PERCENT_50", DownloadSource: "free_download"},
		{SiteName: "s", TorrentID: "rule-strict", IsFree: true, FreeLevel: "FREE", DownloadSource: "filter_rule", FilterRuleID: &strict.ID},
		{SiteName: "s", TorrentID: "rule-loose", IsFree: false, DownloadSource: "filter_rule", FilterRuleID: &loose.ID},
		{SiteName: "s", TorrentID: "manual", IsFree: true, FreeLevel: "FREE", DownloadSource: "manual_push"},
	}
	for i := range rows {
		require.NoError(t, db.Create(&rows[i]).Error)
	}

	sm := NewSchemaManager(db, "test")
	require.NoError(t, sm.RunMigrations())

	load := func(id string) TorrentInfo {
		var ti TorrentInfo
		require.NoError(t, db.Where("torrent_id = ?", id).First(&ti).Error)
		return ti
	}
	for id, want := range map[string]string{
		"free-end": "free_until", "free-no-end": "unknown", "not-free": "not_free",
		"rule-strict": "unknown", "rule-loose": "not_free", "manual": "unknown",
	} {
		ti := load(id)
		assert.Equal(t, want, ti.FreeValidity, id)
		assert.Equal(t, "legacy", ti.FreeStatusSource, id)
		assert.Nil(t, ti.FreeVerifiedAt, id+"：旧数据不算验证过")
	}
	for id, want := range map[string]bool{
		"free-end": true, "free-no-end": true, "not-free": true, "rule-strict": true, "rule-loose": false, "manual": false,
	} {
		assert.Equal(t, want, load(id).RequireFree, id)
	}
	kept := load("free-no-end")
	require.NotNil(t, kept.IsPushed)
	assert.True(t, *kept.IsPushed)
	assert.Equal(t, "abc", kept.DownloaderTaskID)
	assert.True(t, kept.PauseOnFreeEnd)
	assert.False(t, kept.IsPausedBySystem)
	assert.True(t, load("free-end").FreeEndTime.Equal(end))

	version, err := sm.GetCurrentVersion()
	require.NoError(t, err)
	assert.Equal(t, CurrentSchemaVersion, version)
	assert.Equal(t, 12, CurrentSchemaVersion)

	// 再跑一次：已经回填的行不再改（新代码写过的状态不会被旧规则覆盖）
	require.NoError(t, db.Model(&TorrentInfo{}).Where("torrent_id = ?", "free-no-end").
		Updates(map[string]any{"free_validity": "free_until", "free_status_source": "base"}).Error)
	require.NoError(t, sm.migrateV11ToV12(db))
	assert.Equal(t, "free_until", load("free-no-end").FreeValidity)
	assert.Equal(t, "base", load("free-no-end").FreeStatusSource)
}

// 没有种子表（不太可能，但迁移要能跑）时什么也不做
func TestMigrationV11ToV12NoTorrentTable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "empty.db")), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { closeTestGorm(db) })
	sm := NewSchemaManager(db, "test")
	require.NoError(t, sm.migrateV11ToV12(db))
}
