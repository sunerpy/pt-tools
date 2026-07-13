package models

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"moul.io/zapgorm2"
)

func TestNewDB_InitializesAndMigrates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	lg := zapgorm2.Logger{ZapLogger: zap.NewNop()}
	db, err := NewDB(lg)
	if err != nil {
		t.Fatalf("newdb: %v", err)
	}
	if db == nil || db.DB == nil {
		t.Fatalf("db nil")
	}
}

func TestTorrentInfo_GetExpired_IsExpiredFlag(t *testing.T) {
	assert.True(t, (&TorrentInfo{IsExpired: true}).GetExpired())
	assert.False(t, (&TorrentInfo{FreeLevel: "free"}).GetExpired())
}

func TestNewDBWithVersionAndHooks_PersistsTorrent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	backup := func(db *gorm.DB, table string) (string, error) { return "backup/" + table, nil }
	enc := func(plain string) (string, error) { return "enc:" + plain, nil }
	dec := func(cipher string) (string, error) { return cipher[4:], nil }

	tdb, err := NewDBWithVersionAndHooks(zapNopGormLogger(), "2.0.0", backup, enc, dec)
	require.NoError(t, err)
	require.NotNil(t, tdb)

	require.NoError(t, tdb.UpsertTorrent(&TorrentInfo{SiteName: "hdsky", TorrentID: "abc", IsFree: true}))
	got, err := tdb.GetTorrentBySiteAndID("hdsky", "abc")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.IsFree)

	var glCnt int64
	require.NoError(t, tdb.DB.Model(&SettingsGlobal{}).Count(&glCnt).Error)
	assert.Equal(t, int64(1), glCnt)
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
