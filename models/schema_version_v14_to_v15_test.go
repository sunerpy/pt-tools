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

// v15：升级前的每条辅种记录补一行尝试；各个结果的映射；连续跑两次 v15 每条记录也只有一行回填。
func TestMigrationV14ToV15BackfillsReseedAttempts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "v14.db")), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { closeTestGorm(db) })
	require.NoError(t, db.AutoMigrate(&SchemaVersion{}, &ReseedRecord{}, &ReseedAttempt{}))
	require.NoError(t, db.Create(&SchemaVersion{Version: 14, Description: "test v14", AppVersion: "test"}).Error)
	at := time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)
	job := uint(7)
	for i, r := range []ReseedRecord{
		{TorrentID: "failed", State: ReseedFailed, Message: "文件列表与原种子不一致"},
		{TorrentID: "gone", State: ReseedGone, Message: "站点上已经没有这个种子"},
		{TorrentID: "queued", State: ReseedQueued, JobID: &job},
		{TorrentID: "done", State: ReseedDone, Message: "已开始做种"},
		{TorrentID: "rolled", State: ReseedRolledBack},
		{TorrentID: "canceled", State: ReseedCanceled},
	} {
		r.InfoHash, r.SiteName, r.DownloaderID, r.Name = string(rune('a'+i)), "hdsky", 1, "Movie"
		r.CreatedAt, r.UpdatedAt = at, at.Add(time.Duration(i)*time.Minute)
		require.NoError(t, db.Create(&r).Error)
	}

	sm := NewSchemaManager(db, "test")
	require.NoError(t, sm.RunMigrations())
	version, err := sm.GetCurrentVersion()
	require.NoError(t, err)
	assert.Equal(t, CurrentSchemaVersion, version)
	assert.GreaterOrEqual(t, CurrentSchemaVersion, 15)

	var atts []ReseedAttempt
	require.NoError(t, db.Order("id").Find(&atts).Error)
	require.Len(t, atts, 6)
	by := map[string]ReseedAttempt{}
	for _, a := range atts {
		by[a.TorrentID] = a
		assert.True(t, a.Backfilled)
		assert.Nil(t, a.RunID)
		assert.Equal(t, "hdsky", a.SiteName)
		require.NotNil(t, a.FinishedAt)
		assert.True(t, a.FinishedAt.Equal(a.StartedAt))
	}
	assert.Equal(t, ReseedAttemptFailed, by["failed"].Outcome)
	assert.Equal(t, "文件列表与原种子不一致", by["failed"].Message)
	assert.Equal(t, ReseedAttemptGone, by["gone"].Outcome)
	assert.Equal(t, ReseedAttemptCreated, by["queued"].Outcome)
	require.NotNil(t, by["queued"].JobID)
	assert.Equal(t, job, *by["queued"].JobID)
	assert.Empty(t, by["queued"].JobState, "任务还在：结果从任务表读")
	assert.Nil(t, by["queued"].JobFinishedAt)
	assert.True(t, by["queued"].StartedAt.Equal(at.Add(2*time.Minute)), "开始时间取记录的更新时间")
	assert.Equal(t, TransferDone, by["done"].JobState)
	assert.Empty(t, by["done"].Message)
	require.NotNil(t, by["done"].JobFinishedAt)
	assert.True(t, by["done"].JobFinishedAt.Equal(at.Add(3*time.Minute)))
	assert.Equal(t, TransferRolledBack, by["rolled"].JobState)
	assert.Equal(t, TransferCanceled, by["canceled"].JobState)

	// 再跑一次 v15（中途退出、版本号没写上的情形）：已经有回填行，不再补
	require.NoError(t, sm.migrateV14ToV15(db))
	var n int64
	require.NoError(t, db.Model(&ReseedAttempt{}).Count(&n).Error)
	assert.EqualValues(t, 6, n)
}

// 全新安装直接记最新版本，不回填。
func TestMigrationV15FreshInstallSkipsBackfill(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "fresh.db")), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { closeTestGorm(db) })
	require.NoError(t, db.AutoMigrate(&SchemaVersion{}, &SiteSetting{}, &SettingsGlobal{}, &RSSSubscription{}, &ReseedRecord{}, &ReseedAttempt{}))
	require.NoError(t, NewSchemaManager(db, "test").RunMigrations())
	var n int64
	require.NoError(t, db.Model(&ReseedAttempt{}).Count(&n).Error)
	assert.Zero(t, n)

	// 没有辅种表的旧库：跳过
	bare, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "bare.db")), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() { closeTestGorm(bare) })
	require.NoError(t, NewSchemaManager(bare, "test").migrateV14ToV15(bare))
}
