package internal

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	sm "github.com/sunerpy/pt-tools/mocks"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

/*
 * 辅种的种子和原种子共用同一份文件，不重复计入站点做种容量（站点详情里的「刷流容量上限」）。
 * 2026-11-08 用户反馈：「IYUU 辅种可以改为不占用刷流容量上限吗？本身辅种不占用硬盘新的空间，辅种后很容易顶穿刷流容量上限」；
 * 用户选了「同一份数据只算一次」：原种子还在时辅种的种子不计入，原种子删了以后改为计入。
 */

func TestSharedDataSkips(t *testing.T) {
	torrents := []downloader.Torrent{
		{InfoHash: "X", DateAdded: 100}, // 刷流下的原种子
		{InfoHash: "b", DateAdded: 200}, // 辅种：原种子 x
		{InfoHash: "c", DateAdded: 300}, // 辅种：原种子是另一个辅种 b
		{InfoHash: "y", DateAdded: 50},  // 无关
	}
	links := map[string]string{"b": "x", "c": "b", "d": "x", "e": "z"}

	skips, present := sharedDataSkips(torrents, links)
	assert.Equal(t, map[string]bool{"b": true, "c": true}, skips, "一组里只算原种子；不在下载器里的辅种（d、e）不连")
	assert.True(t, present["x"], "大小写不敏感")

	t.Run("原种子删了：数据靠辅种的种子留着，最早加进来的那个算", func(t *testing.T) {
		skips, _ := sharedDataSkips(torrents[1:], links)
		assert.Equal(t, map[string]bool{"c": true}, skips)
	})

	t.Run("原种子删了又被辅种加回来（两条记录互为原种子）：只算一个", func(t *testing.T) {
		skips, _ := sharedDataSkips(
			[]downloader.Torrent{{InfoHash: "x", DateAdded: 500}, {InfoHash: "b", DateAdded: 200}},
			map[string]string{"x": "b", "b": "x"},
		)
		assert.Equal(t, map[string]bool{"x": true}, skips, "都是辅种加的：算先加进来的 b")
	})

	t.Run("没有辅种记录时什么都不跳过", func(t *testing.T) {
		skips, _ := sharedDataSkips(torrents, nil)
		assert.Empty(t, skips)
	})
}

func TestSiteSeedingUsage_ReseedSharesData(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	for _, r := range []models.ReseedRecord{
		{InfoHash: "bbb", SiteName: "siteb", SourceHash: "AAA", State: models.ReseedQueued},
		{InfoHash: "ccc", SiteName: "siteb", SourceHash: "gone", State: models.ReseedDone},
		{InfoHash: "ddd", SiteName: "siteb", SourceHash: "aaa", State: models.ReseedFailed},
		{InfoHash: "fff", SiteName: "siteb", SourceHash: "aaa", State: models.ReseedQueued},
		{InfoHash: "ggg", SiteName: "siteb", SourceHash: "gone", State: models.ReseedQueued},
	} {
		require.NoError(t, db.DB.Create(&r).Error)
	}
	dl := sm.NewMockDownloader(gomock.NewController(t))
	dl.EXPECT().GetAllTorrents().Return([]downloader.Torrent{
		{InfoHash: "aaa", TotalSize: 10 * gb, Category: "sitea"},
		{InfoHash: "bbb", TotalSize: 10 * gb, Category: "siteb"}, // 原种子在：不算
		{InfoHash: "ccc", TotalSize: 7 * gb, Category: "siteb"},  // 原种子不在了：算
		{InfoHash: "ddd", TotalSize: 3 * gb, Category: "siteb"},  // 辅种失败的记录：这个种子不是辅种留下的，算
		{InfoHash: "eee", TotalSize: 5 * gb, Category: "siteb"},
	}, nil).Times(3)

	u, err := siteSeedingUsage(context.Background(), "siteb", dl, "")
	require.NoError(t, err)
	assert.Equal(t, 15*gb, u.used)
	assert.Empty(t, u.sharedSource)

	u, err = siteSeedingUsage(context.Background(), "siteb", dl, "FFF")
	require.NoError(t, err)
	assert.Equal(t, "aaa", u.sharedSource, "要推送的辅种和下载器里的 aaa 共用数据")

	u, err = siteSeedingUsage(context.Background(), "siteb", dl, "ggg")
	require.NoError(t, err)
	assert.Empty(t, u.sharedSource, "原种子不在下载器里：照常算")
}

// reseedPushSetup 建一个站点容量上限 capGB 的站点与下载器，下载器里有 torrents；返回下载器 ID 与假下载器。
func reseedPushSetup(t *testing.T, capGB float64, torrents []downloader.Torrent) (uint, *sm.MockDownloader) {
	t.Helper()
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	GetDiskBudget().Reset()
	t.Cleanup(func() { GetDiskBudget().Reset() })
	disableDiskProtect(t, db)
	require.NoError(t, db.DB.Create(&models.SiteSetting{Name: "siteb", SeedingCapacityGB: capGB}).Error)
	dl := reuseTestDownloader(t)
	dl.EXPECT().GetAllTorrents().Return(torrents, nil).AnyTimes()
	return seedQbitDownloader(t, "http://127.0.0.1:9"), dl
}

func reseedRecord(t *testing.T, data []byte, source string) {
	t.Helper()
	hash, err := qbit.ComputeTorrentHash(data)
	require.NoError(t, err)
	require.NoError(t, global.GlobalDB.DB.Create(&models.ReseedRecord{
		InfoHash: hash, SiteName: "siteb", TorrentID: "r1", SourceHash: source, State: models.ReseedQueued,
	}).Error)
}

// 辅种：原种子在下载器里，站点做种总量早就超过上限也照样加进去（不多占空间）。
func TestPushTorrent_ReseedSharingDataSkipsSiteCapacity(t *testing.T) {
	dlID, dl := reseedPushSetup(t, 1, []downloader.Torrent{
		{InfoHash: "src", TotalSize: 50 * gb, Category: "sitea"},
		{InfoHash: "own", TotalSize: 5 * gb, Category: "siteb"},
	})
	dl.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).Return(downloader.AddTorrentResult{Success: true, Hash: "h"}, nil)
	data := makeSizedTorrentBytes(t, "shared", 50*gb)
	reseedRecord(t, data, "src")

	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "siteb", TorrentID: "r1", TorrentData: data, DownloaderID: dlID, Source: "reseed", ReuseExistingData: true,
	})
	require.NoError(t, err)
	assert.True(t, res.Success, res.Message)
}

// 原种子已经不在下载器里：辅种的种子会自己占着这份数据，照常检查站点容量。
func TestPushTorrent_ReseedWithoutSourceKeepsSiteCapacity(t *testing.T) {
	dlID, _ := reseedPushSetup(t, 1, []downloader.Torrent{{InfoHash: "own", TotalSize: 5 * gb, Category: "siteb"}})
	data := makeSizedTorrentBytes(t, "shared", 50*gb)
	reseedRecord(t, data, "src")

	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "siteb", TorrentID: "r1", TorrentData: data, DownloaderID: dlID, Source: "reseed", ReuseExistingData: true,
	})
	require.NoError(t, err)
	assert.False(t, res.Success)
	assert.Contains(t, res.Message, "站点容量已满")
}

// 用户反馈的情形：辅种加了 40 GB，站点自己下的只有 5 GB；上限 10 GB 时刷流、RSS 照样能加 4 GB 的种子，加 6 GB 的才超。
func TestPushTorrent_ReseedTorrentsDoNotFillSiteCapacity(t *testing.T) {
	dlID, dl := reseedPushSetup(t, 10, []downloader.Torrent{
		{InfoHash: "src", TotalSize: 40 * gb, Category: "sitea"},
		{InfoHash: "reseeded", TotalSize: 40 * gb, Category: "siteb"},
		{InfoHash: "own", TotalSize: 5 * gb, Category: "siteb"},
	})
	require.NoError(t, global.GlobalDB.DB.Create(&models.ReseedRecord{
		InfoHash: "reseeded", SiteName: "siteb", TorrentID: "r0", SourceHash: "src", State: models.ReseedDone,
	}).Error)
	dl.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).Return(downloader.AddTorrentResult{Success: true, Hash: "h"}, nil)

	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "siteb", TorrentID: "n1", TorrentData: makeSizedTorrentBytes(t, "small", 4*gb), DownloaderID: dlID,
	})
	require.NoError(t, err)
	assert.True(t, res.Success, res.Message)

	res, err = PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "siteb", TorrentID: "n2", TorrentData: makeSizedTorrentBytes(t, "big", 6*gb), DownloaderID: dlID,
	})
	require.NoError(t, err)
	assert.False(t, res.Success)
	assert.Contains(t, res.Message, "当前 5.0 GB")
}
