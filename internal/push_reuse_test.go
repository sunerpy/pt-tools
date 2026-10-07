package internal

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	sm "github.com/sunerpy/pt-tools/mocks"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// reuseTestDownloader 换上一个假下载器：不该读磁盘空间（没有 GetClientFreeSpace 的期望，调了就失败）。
func reuseTestDownloader(t *testing.T) *sm.MockDownloader {
	t.Helper()
	dl := sm.NewMockDownloader(gomock.NewController(t))
	dl.EXPECT().Close().Return(nil).AnyTimes()
	dl.EXPECT().CheckTorrentExists(gomock.Any()).Return(false, nil).AnyTimes()
	t.Cleanup(SwapPushDownloaderFactory(func(models.DownloaderSetting) (downloader.Downloader, error) { return dl, nil }))
	return dl
}

// 转移做种与辅种：数据已经在盘上——强制暂停添加、不占磁盘预算、不写种子记录（成功后由调用方改记录）。
func TestPushTorrent_ReuseExistingData(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	GetDiskBudget().Reset()
	t.Cleanup(func() { GetDiskBudget().Reset() })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), CleanupDiskProtect: true, CleanupMinDiskSpaceGB: 5,
	}))
	dl := reuseTestDownloader(t)
	var got downloader.AddTorrentOptions
	dl.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ []byte, o downloader.AddTorrentOptions) (downloader.AddTorrentResult, error) {
			got = o
			return downloader.AddTorrentResult{Success: true, Hash: "h"}, nil
		},
	)
	dlID := seedQbitDownloader(t, "http://127.0.0.1:9") // AutoStart = true

	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "t1", TorrentData: makeSizedTorrentBytes(t, "x", 500*gb),
		DownloaderID: dlID, SavePath: "/data/x", Category: "movies", ReuseExistingData: true,
	})
	require.NoError(t, err)
	require.True(t, res.Success, res.Message)
	assert.True(t, got.AddAtPaused, "校验完之前不能开始")
	assert.Equal(t, "/data/x", got.SavePath)
	assert.Equal(t, "movies", got.Category)
	assert.Zero(t, GetDiskBudget().Reserved(), "不占磁盘预算")
	var n int64
	require.NoError(t, db.DB.Model(&models.TorrentInfo{}).Count(&n).Error)
	assert.Zero(t, n, "不写种子记录")
}

// 站点容量闸门照常生效。
func TestPushTorrent_ReuseExistingDataKeepsSiteCapacity(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	GetDiskBudget().Reset()
	t.Cleanup(func() { GetDiskBudget().Reset() })
	disableDiskProtect(t, db)
	require.NoError(t, db.DB.Create(&models.SiteSetting{Name: "springsunday", SeedingCapacityGB: 1}).Error)
	dl := reuseTestDownloader(t)
	dl.EXPECT().GetAllTorrents().Return([]downloader.Torrent{}, nil)
	dlID := seedQbitDownloader(t, "http://127.0.0.1:9")

	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "t2", TorrentData: makeSizedTorrentBytes(t, "big", 5*gb),
		DownloaderID: dlID, ReuseExistingData: true,
	})
	require.NoError(t, err)
	assert.False(t, res.Success)
	assert.Contains(t, res.Message, "站点容量")
}
