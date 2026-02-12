package models_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/models"
)

func TestWithTransaction_Success(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	err = db.WithTransaction(func(tx *gorm.DB) error {
		return tx.Create(&models.TorrentInfo{SiteName: "cmct", TorrentID: "t1"}).Error
	})
	require.NoError(t, err)
	items, err := db.GetAllTorrents()
	require.NoError(t, err)
	require.Equal(t, 1, len(items))
}

func TestWithTransaction_Error(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	e := db.WithTransaction(func(tx *gorm.DB) error { return fmt.Errorf("txerr") })
	require.Error(t, e)
}

func TestGetAllTorrents_AfterInsert(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	ti := &models.TorrentInfo{SiteName: "cmct", TorrentID: "insert-1"}
	require.NoError(t, db.UpsertTorrent(ti))
	items, err := db.GetAllTorrents()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(items), 1)
}

func TestTorrentInfo_GetExpired(t *testing.T) {
	now := time.Now()
	// FreeEndTime=nil + FreeLevel="" → 非免费，视为过期
	ti := &models.TorrentInfo{FreeEndTime: nil}
	require.True(t, ti.GetExpired())
	// FreeEndTime=nil + FreeLevel="FREE" → 永久免费，不过期
	tiFree := &models.TorrentInfo{FreeEndTime: nil, FreeLevel: "FREE"}
	require.False(t, tiFree.GetExpired())
	// FreeEndTime=nil + FreeLevel="NONE" → 非免费，视为过期
	tiNone := &models.TorrentInfo{FreeEndTime: nil, FreeLevel: "NONE"}
	require.True(t, tiNone.GetExpired())
	future := now.Add(10 * time.Minute)
	ti2 := &models.TorrentInfo{FreeEndTime: &future}
	require.False(t, ti2.GetExpired())
	past := now.Add(-10 * time.Minute)
	ti3 := &models.TorrentInfo{FreeEndTime: &past}
	require.True(t, ti3.GetExpired())
}

func TestTorrentDB_CRUDAndQueries(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	ti := &models.TorrentInfo{SiteName: "cmct", TorrentID: "guid-x"}
	h := "deadbeef"
	ti.TorrentHash = &h
	require.NoError(t, db.UpsertTorrent(ti))
	got, err := db.GetTorrentBySiteAndID("cmct", "guid-x")
	require.NoError(t, err)
	require.NotNil(t, got)
	g2, err := db.GetTorrentBySiteAndHash("cmct", h)
	require.NoError(t, err)
	require.NotNil(t, g2)
	now := time.Now()
	require.NoError(t, db.UpdateTorrentStatus(h, true, true, &now))
	g3, err := db.GetTorrentBySiteAndHash("cmct", h)
	require.NoError(t, err)
	require.NotNil(t, g3)
	require.NotNil(t, g3.IsPushed)
	require.True(t, *g3.IsPushed)
	require.True(t, g3.IsDownloaded)
	require.NoError(t, db.DeleteTorrent(h))
	g4, err := db.GetTorrentBySiteAndHash("cmct", h)
	require.NoError(t, err)
	require.Nil(t, g4)
}
