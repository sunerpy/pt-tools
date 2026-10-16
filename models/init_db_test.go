package models

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"moul.io/zapgorm2"
)

func TestNewDB_InitializesAndMigrates(t *testing.T) {
	{
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
	}
	lg := zapgorm2.Logger{ZapLogger: zap.NewNop()}
	db, err := NewDB(lg)
	if err != nil {
		t.Fatalf("newdb: %v", err)
	}
	if db == nil || db.DB == nil {
		t.Fatalf("db nil")
	}
	t.Cleanup(func() { closeTestGorm(db.DB) })
}

func TestTorrentInfo_GetExpired_IsExpiredFlag(t *testing.T) {
	assert.True(t, (&TorrentInfo{IsExpired: true}).GetExpired())
	assert.False(t, (&TorrentInfo{FreeLevel: "free"}).GetExpired())
}

func TestNewDBWithVersionAndHooks_PersistsTorrent(t *testing.T) {
	{
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
	}

	backup := func(db *gorm.DB, table string) (string, error) { return "backup/" + table, nil }
	enc := func(plain string) (string, error) { return "enc:" + plain, nil }
	dec := func(cipher string) (string, error) { return cipher[4:], nil }

	tdb, err := NewDBWithVersionAndHooks(zapNopGormLogger(), "2.0.0", backup, enc, dec)
	require.NoError(t, err)
	require.NotNil(t, tdb)
	t.Cleanup(func() { closeTestGorm(tdb.DB) })

	require.NoError(t, tdb.UpsertTorrent(&TorrentInfo{SiteName: "hdsky", TorrentID: "abc", IsFree: true}))
	got, err := tdb.GetTorrentBySiteAndID("hdsky", "abc")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.IsFree)

	var glCnt int64
	require.NoError(t, tdb.DB.Model(&SettingsGlobal{}).Count(&glCnt).Error)
	assert.Equal(t, int64(1), glCnt)
	// 新安装默认提前 5 分钟暂停免费到期的种子（留出 tracker 汇报延迟）
	var gl SettingsGlobal
	require.NoError(t, tdb.DB.First(&gl).Error)
	assert.Equal(t, 5, gl.FreeEndAdvanceMinutes)
}

func TestTorrentDB_WithTransactionContext(t *testing.T) {
	db := newMemDB(t, &TorrentInfo{})
	tdb := &TorrentDB{DB: db}

	require.NoError(t, tdb.WithTransactionContext(context.Background(), func(tx *gorm.DB) error {
		return tx.Create(&TorrentInfo{SiteName: "s", TorrentID: "t"}).Error
	}))

	got, err := tdb.GetTorrentBySiteAndID("s", "t")
	require.NoError(t, err)
	require.NotNil(t, got)

	// missing → nil, nil
	miss, err := tdb.GetTorrentBySiteAndID("s", "missing")
	require.NoError(t, err)
	assert.Nil(t, miss)

	missHash, err := tdb.GetTorrentBySiteAndHash("s", "nohash")
	require.NoError(t, err)
	assert.Nil(t, missHash)
}

// free_ended_at 是只增不改的可空列：上一版的库启动时由 AutoMigrate 加上，已有记录是 NULL，其他值不变（不需要编号迁移）。
func TestNewDB_UpgradeAddsFreeEndedAt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	tdb, err := NewDB(zapNopGormLogger())
	require.NoError(t, err)
	for _, m := range []any{&TorrentInfo{}, &TorrentInfoArchive{}} {
		require.NoError(t, tdb.DB.Migrator().DropColumn(m, "free_ended_at"), "模拟上一版没有这一列")
	}
	end := time.Date(2026, 10, 20, 8, 0, 0, 0, time.Local)
	require.NoError(t, tdb.DB.Table("torrent_infos").Create(map[string]any{
		"site_name": "s", "torrent_id": "old", "is_free": true, "free_end_time": end, "is_pushed": true,
	}).Error)
	closeTestGorm(tdb.DB)

	tdb, err = NewDB(zapNopGormLogger())
	require.NoError(t, err)
	t.Cleanup(func() { closeTestGorm(tdb.DB) })
	assert.True(t, tdb.DB.Migrator().HasColumn(&TorrentInfo{}, "free_ended_at"))
	assert.True(t, tdb.DB.Migrator().HasColumn(&TorrentInfoArchive{}, "free_ended_at"))
	got, err := tdb.GetTorrentBySiteAndID("s", "old")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Nil(t, got.FreeEndedAt)
	assert.True(t, got.IsFree)
	require.NotNil(t, got.FreeEndTime)
	assert.True(t, got.FreeEndTime.Equal(end))
}
