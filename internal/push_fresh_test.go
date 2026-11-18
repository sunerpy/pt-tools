package internal

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// seedCompleted 记一条以前下完过的种子记录。
func seedCompleted(t *testing.T, db *models.TorrentDB, id string) {
	t.Helper()
	done := time.Now().Add(-time.Hour)
	require.NoError(t, db.DB.Create(&models.TorrentInfo{
		SiteName: "springsunday", TorrentID: id, IsCompleted: true, CompletedAt: &done, Progress: 100,
	}).Error)
}

// 以前下完、从下载器里删掉以后再推的种子：记录改回没下完，进度监控接着看它、下完时再发完成事件。
func TestPushTorrent_FreshAddClearsCompletion(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	GetDiskBudget().Reset()
	t.Cleanup(func() { GetDiskBudget().Reset() })
	seedCompleted(t, db, "again-1")

	srv := fakeQbitServer(t, false, 500*gb)
	dlID := seedQbitDownloader(t, srv.URL)
	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "again-1", TorrentData: makeSizedTorrentBytes(t, "again", gb), DownloaderID: dlID,
	})
	require.NoError(t, err)
	require.True(t, res.Success)
	require.False(t, res.Skipped)

	ti, err := db.GetTorrentBySiteAndID("springsunday", "again-1")
	require.NoError(t, err)
	assert.False(t, ti.IsCompleted)
	assert.Nil(t, ti.CompletedAt)
	assert.Zero(t, ti.Progress)
}

// 下载器里本来就有的（补记账）：完成状态不动。
func TestPushTorrent_AdoptExistingKeepsCompletion(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	seedCompleted(t, db, "kept-1")

	srv := fakeQbitServer(t, true, 500*gb)
	dlID := seedQbitDownloader(t, srv.URL)
	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "kept-1", TorrentData: makeSizedTorrentBytes(t, "kept", gb), DownloaderID: dlID,
		AdoptExisting: true,
	})
	require.NoError(t, err)
	require.True(t, res.Success)
	require.True(t, res.Skipped)

	ti, err := db.GetTorrentBySiteAndID("springsunday", "kept-1")
	require.NoError(t, err)
	assert.True(t, ti.IsCompleted)
	assert.NotNil(t, ti.CompletedAt)
	require.NotNil(t, ti.IsPushed, "照样记账")
	assert.True(t, *ti.IsPushed)
}
