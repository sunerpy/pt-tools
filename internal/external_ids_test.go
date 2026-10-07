package internal

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	sm "github.com/sunerpy/pt-tools/mocks"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// RSS 新建的记录写上详情里的外部编号。
func TestFetchUnified_NewRowWritesExternalIDs(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	srv := feedServerUnified(t, rssBody(itemXML("e1", "Ext", "http://x/e.torrent")))
	site := &unifiedFake{
		enabled: true, writeFile: true,
		detail: &v2.TorrentItem{ID: "e1", Title: "Ext", DiscountLevel: v2.DiscountFree, SizeBytes: 1024, IMDbID: "tt0111161", DoubanID: "1292052"},
	}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site, models.RSSConfig{Name: "r", URL: srv.URL, Tag: "movie"}))
	ti, err := db.GetTorrentBySiteAndID("springsunday", "e1")
	require.NoError(t, err)
	require.NotNil(t, ti)
	assert.Equal(t, "tt0111161", ti.IMDbID)
	assert.Equal(t, "1292052", ti.DoubanID)
}

// 已有记录重新检查时：详情里有的编号用新值，没有的保留库里原有的值。
func TestFetchUnified_RecheckUpdatesExternalIDs(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	checked := time.Now().Add(-(SkipRecheckHours + 1) * time.Hour)
	require.NoError(t, db.DB.Create(&models.TorrentInfo{
		SiteName: "springsunday", TorrentID: "e2", IsSkipped: true, LastCheckTime: &checked,
		IMDbID: "tt0000001", DoubanID: "111111",
	}).Error)
	srv := feedServerUnified(t, rssBody(itemXML("e2", "Ext2", "http://x/e2.torrent")))
	site := &unifiedFake{
		enabled: true,
		detail:  &v2.TorrentItem{ID: "e2", Title: "Ext2", DiscountLevel: v2.DiscountNone, SizeBytes: 1024, IMDbID: "tt7654321"},
	}
	require.NoError(t, FetchAndDownloadFreeRSSUnified(context.Background(), site, models.RSSConfig{Name: "r", URL: srv.URL, Tag: "movie"}))
	require.Equal(t, int32(1), site.detailCalls.Load(), "重新检查取了详情")
	ti, err := db.GetTorrentBySiteAndID("springsunday", "e2")
	require.NoError(t, err)
	assert.Equal(t, "tt7654321", ti.IMDbID, "详情里有新值时更新")
	assert.Equal(t, "111111", ti.DoubanID, "详情里没有时保留原有的值")
}

// 推送入口：带编号时写进记录；不带编号的推送不清掉已有的；下载器里已有该种子时只更新已有的记录，不新建。
func TestPushTorrentToDownloader_ExternalIDs(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	ds := models.DownloaderSetting{Name: "qb-main", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true}
	require.NoError(t, db.DB.Create(&ds).Error)
	disableDiskProtect(t, db)

	ctrl := gomock.NewController(t)
	dl := sm.NewMockDownloader(ctrl)
	dl.EXPECT().CheckTorrentExists(gomock.Any()).Return(false, nil).Times(2)
	dl.EXPECT().CheckTorrentExists(gomock.Any()).Return(true, nil).Times(2)
	dl.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).Return(downloader.AddTorrentResult{Success: true, Hash: "h"}, nil).Times(2)
	dl.EXPECT().Close().Return(nil).Times(4)
	restore := SwapPushDownloaderFactory(func(models.DownloaderSetting) (downloader.Downloader, error) { return dl, nil })
	t.Cleanup(restore)
	data := makeSizedTorrentBytes(t, "z", gb)
	get := func(id string) *models.TorrentInfo {
		ti, err := db.GetTorrentBySiteAndID("hdsky", id)
		require.NoError(t, err)
		return ti
	}

	// 带编号的推送写进记录（链接形式的编号也规整）
	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "hdsky", TorrentID: "x1", TorrentData: data, DownloaderID: ds.ID,
		IMDbID: "https://www.imdb.com/title/tt0111161/", DoubanID: "1292052",
	})
	require.NoError(t, err)
	require.True(t, res.Success)
	assert.Equal(t, "tt0111161", get("x1").IMDbID)
	assert.Equal(t, "1292052", get("x1").DoubanID)

	// 不带编号的推送（手动推送、ChatOps 推送）不清掉已有的编号
	_, err = PushTorrentToDownloader(context.Background(), PushTorrentRequest{SiteID: "hdsky", TorrentID: "x1", TorrentData: data, DownloaderID: ds.ID})
	require.NoError(t, err)
	assert.Equal(t, "tt0111161", get("x1").IMDbID)
	assert.Equal(t, "1292052", get("x1").DoubanID)

	// 下载器里已有该种子、库里有记录：只写非空的编号
	res, err = PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "hdsky", TorrentID: "x1", TorrentData: data, DownloaderID: ds.ID, IMDbID: "tt7654321",
	})
	require.NoError(t, err)
	assert.True(t, res.Skipped)
	assert.Equal(t, "tt7654321", get("x1").IMDbID)
	assert.Equal(t, "1292052", get("x1").DoubanID)

	// 下载器里已有、库里没有记录：不为编号新建记录
	res, err = PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "hdsky", TorrentID: "x2", TorrentData: data, DownloaderID: ds.ID, IMDbID: "tt0111161", DoubanID: "1292052",
	})
	require.NoError(t, err)
	assert.True(t, res.Skipped)
	assert.Nil(t, get("x2"))
}
