package internal

import (
	"context"
	"os"
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

// disableDiskProtect 关掉磁盘保护（gorm 的 default:true 让 SaveGlobalSettings 存不进 false），
// 让测试只走站点容量闸门或推送本身。
func disableDiskProtect(t *testing.T, db *models.TorrentDB) {
	t.Helper()
	require.NoError(t, db.DB.Model(&models.SettingsGlobal{}).Where("1 = 1").Update("cleanup_disk_protect", false).Error)
}

// 站点做种总量按 pt-tools 记录过的 info hash 归属，不依赖用户给 RSS 填的分类和标签。
func TestSumSiteSeedingSizeWithHashes_CountsRecordedTorrents(t *testing.T) {
	torrents := []downloader.Torrent{
		{InfoHash: "AAAA", TotalSize: 10, Category: "movies", Tags: "4k"},
		{InfoHash: "bbbb", TotalSize: 20, Category: "springsunday"},
		{InfoHash: "cccc", TotalSize: 40, Category: "movies"},
		{InfoHash: "", TotalSize: 80},
	}
	hashes := map[string]struct{}{"aaaa": {}, "": {}}
	assert.Equal(t, int64(30), sumSiteSeedingSizeWithHashes("springsunday", torrents, hashes, nil),
		"记录过的种子（大小写不敏感）+ 分类命中站点名的种子；空 hash 不算")
	assert.Equal(t, int64(20), sumSiteSeedingSize("springsunday", torrents), "不带记录时只按分类和标签")
}

// 手动添加、没打站点分类或标签的种子按 tracker 归属站点，也计入站点做种总量。
func TestSumSiteSeedingSize_CountsByTracker(t *testing.T) {
	resolver := v2.NewTrackerResolverFrom(&v2.SiteDefinition{ID: "springsunday", Schema: v2.SchemaNexusPHP, URLs: []string{"https://springsunday.net/"}})
	torrents := []downloader.Torrent{
		{InfoHash: "a", TotalSize: 10, Tracker: "https://on.springsunday.net/announce.php?passkey=x"},
		{InfoHash: "b", TotalSize: 20, Tracker: "https://tracker.other.org/announce"},
		{InfoHash: "c", TotalSize: 40, Category: "springsunday", Tracker: "https://on.springsunday.net/announce.php"},
		{InfoHash: "d", TotalSize: 80},
	}
	assert.Equal(t, int64(50), sumSiteSeedingSizeWithHashes("springsunday", torrents, nil, resolver),
		"按 tracker 命中的 10 + 分类命中的 40，同一个种子不重复计")
	assert.Equal(t, int64(40), sumSiteSeedingSizeWithHashes("springsunday", torrents, nil, nil), "不给解析器时只按分类和标签")
}

func TestGetSiteSeedingSizeBytes_UsesRecordedHashes(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	hash := "abcdef0123"
	require.NoError(t, db.DB.Create(&models.TorrentInfo{SiteName: "springsunday", TorrentID: "1", TorrentHash: &hash}).Error)

	ctrl := gomock.NewController(t)
	dl := sm.NewMockDownloader(ctrl)
	dl.EXPECT().GetAllTorrents().Return([]downloader.Torrent{
		{InfoHash: "ABCDEF0123", TotalSize: 7 * gb, Category: "movies"},
		{InfoHash: "other", TotalSize: 5 * gb, Category: "movies"},
	}, nil)

	used, err := getSiteSeedingSizeBytes(context.Background(), "springsunday", dl)
	require.NoError(t, err)
	assert.Equal(t, 7*gb, used)
}

// RSS 推送同样受站点容量限制（此前只有手动推送检查）：超限时不推送、文件留到下一轮。
func TestProcessSingleWithDownloader_SiteCapacityGate(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	GetDiskBudget().Reset()
	t.Cleanup(func() { GetDiskBudget().Reset() })
	require.NoError(t, db.DB.Create(&models.SiteSetting{Name: "springsunday", SeedingCapacityGB: 10}).Error)
	disableDiskProtect(t, db)

	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	future := time.Now().Add(time.Hour)
	pushed := false
	require.NoError(t, db.UpsertTorrent(&models.TorrentInfo{
		SiteName: "springsunday", TorrentHash: &hash, IsPushed: &pushed, FreeEndTime: &future,
		TorrentSize: 2 * gb, IsDownloaded: true,
	}))
	seeded := "seeded-hash"
	require.NoError(t, db.DB.Create(&models.TorrentInfo{SiteName: "springsunday", TorrentID: "old", TorrentHash: &seeded}).Error)

	ctrl := gomock.NewController(t)
	dl := sm.NewMockDownloader(ctrl)
	dl.EXPECT().GetName().Return("qb").AnyTimes()
	dl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	// 已有 9 GB 是这个站点 RSS 推送的种子，分类、标签都不是站点名
	dl.EXPECT().GetAllTorrents().Return([]downloader.Torrent{
		{InfoHash: seeded, TotalSize: 9 * gb, Category: "movies", Tags: "4k"},
	}, nil)
	// AddTorrentFileEx 不应被调用

	err := processSingleTorrentWithDownloader(context.Background(), dl, &DownloaderInfo{ID: 1, Name: "qb", AutoStart: true},
		path, "movies", "4k", "", models.SiteGroup("springsunday"), false)
	require.ErrorIs(t, err, ErrSiteCapacityExceeded)
	_, statErr := os.Stat(path)
	require.NoError(t, statErr, "文件留在暂存目录")

	var got models.TorrentInfo
	require.NoError(t, db.DB.Where("torrent_hash = ?", hash).First(&got).Error)
	assert.Contains(t, got.LastError, "站点做种容量已满")
}

// 站点容量满了就停止本轮推送：同一站点后面的种子也推不进去。
func TestRunPushLoop_StopsWhenSiteCapacityFull(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, db.DB.Create(&models.SiteSetting{Name: "springsunday", SeedingCapacityGB: 1}).Error)
	disableDiskProtect(t, db)

	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	future := time.Now().Add(time.Hour)
	pushed := false
	require.NoError(t, db.UpsertTorrent(&models.TorrentInfo{
		SiteName: "springsunday", TorrentHash: &hash, IsPushed: &pushed, FreeEndTime: &future,
		TorrentSize: 2 * gb, IsDownloaded: true,
	}))

	ctrl := gomock.NewController(t)
	dl := sm.NewMockDownloader(ctrl)
	dl.EXPECT().GetName().Return("qb").AnyTimes()
	dl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	dl.EXPECT().GetAllTorrents().Return(nil, nil)

	result := runPushLoop(context.Background(), dl, &DownloaderInfo{ID: 1, Name: "qb"}, dir, []string{path, path},
		"", "", "", models.SiteGroup("springsunday"), false, 0)
	assert.True(t, result.capacityFull)
	assert.Zero(t, result.success)
	assert.Zero(t, result.failed, "第二个文件没有再尝试")
}

// 手动推送：每次新建的下载器实例用完关闭；成功后与 RSS 路径一样记下下载器信息，
// 免费到期进度更新和自动删种的「数据库」范围才找得到这个任务。
func TestPushTorrentToDownloader_ClosesInstanceAndRecordsDownloader(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	ds := models.DownloaderSetting{Name: "qb-main", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true}
	require.NoError(t, db.DB.Create(&ds).Error)
	disableDiskProtect(t, db)

	data := makeSizedTorrentBytes(t, "x", gb)
	ctrl := gomock.NewController(t)
	dl := sm.NewMockDownloader(ctrl)
	dl.EXPECT().CheckTorrentExists(gomock.Any()).Return(false, nil)
	dl.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).Return(downloader.AddTorrentResult{Success: true, Hash: "task-hash"}, nil)
	dl.EXPECT().Close().Return(nil).Times(1)

	orig := newPushDownloader
	newPushDownloader = func(models.DownloaderSetting) (downloader.Downloader, error) { return dl, nil }
	t.Cleanup(func() { newPushDownloader = orig })

	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "m1", TorrentData: data, DownloaderID: ds.ID,
	})
	require.NoError(t, err)
	require.True(t, res.Success)

	var got models.TorrentInfo
	require.NoError(t, db.DB.Where("site_name = ? AND torrent_id = ?", "springsunday", "m1").First(&got).Error)
	assert.Equal(t, "qb-main", got.DownloaderName)
	require.NotNil(t, got.DownloaderID)
	assert.Equal(t, ds.ID, *got.DownloaderID)
	assert.Equal(t, "task-hash", got.DownloaderTaskID)
	require.NotNil(t, got.IsPushed)
	assert.True(t, *got.IsPushed)
}

// 下载器已添加、本地记账失败时如实告诉调用方，而不是静默当作一切正常。
func TestPushTorrentToDownloader_ReportsBookkeepingFailure(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	ds := models.DownloaderSetting{Name: "qb-main", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true}
	require.NoError(t, db.DB.Create(&ds).Error)
	disableDiskProtect(t, db)
	// 推送成功后的更新语句触发失败
	require.NoError(t, db.DB.Exec(`CREATE TRIGGER fail_push_update BEFORE UPDATE OF is_pushed ON torrent_infos
		BEGIN SELECT RAISE(ABORT, 'disk I/O error'); END`).Error)

	ctrl := gomock.NewController(t)
	dl := sm.NewMockDownloader(ctrl)
	dl.EXPECT().CheckTorrentExists(gomock.Any()).Return(false, nil)
	dl.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).Return(downloader.AddTorrentResult{Success: true, Hash: "h"}, nil)
	dl.EXPECT().Close().Return(nil)
	orig := newPushDownloader
	newPushDownloader = func(models.DownloaderSetting) (downloader.Downloader, error) { return dl, nil }
	t.Cleanup(func() { newPushDownloader = orig })

	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "m2", TorrentData: makeSizedTorrentBytes(t, "y", gb), DownloaderID: ds.ID,
	})
	require.NoError(t, err)
	assert.True(t, res.Success, "种子已经在下载器里")
	assert.Contains(t, res.Message, "本地记录更新失败")
}

// 刷流推送带上来源与种子信息：download_source、H&R、体积写进记录（自动清理的 H&R 保护要用）；
// 之后不带信息的手动推送不清掉这些列，来源回到 manual_push。
func TestPushTorrentToDownloader_SourceAndMeta(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	ds := models.DownloaderSetting{Name: "qb-main", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true}
	require.NoError(t, db.DB.Create(&ds).Error)
	disableDiskProtect(t, db)

	ctrl := gomock.NewController(t)
	dl := sm.NewMockDownloader(ctrl)
	dl.EXPECT().CheckTorrentExists(gomock.Any()).Return(false, nil).Times(2)
	dl.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).Return(downloader.AddTorrentResult{Success: true, Hash: "h"}, nil).Times(2)
	dl.EXPECT().Close().Return(nil).Times(2)
	restore := SwapPushDownloaderFactory(func(models.DownloaderSetting) (downloader.Downloader, error) { return dl, nil })
	t.Cleanup(restore)

	freeEnd := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	data := makeSizedTorrentBytes(t, "z", gb)
	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "hdsky", TorrentID: "b1", TorrentData: data, DownloaderID: ds.ID, Source: "brush",
		Meta: &PushTorrentMeta{SizeBytes: gb, HasHR: true, HRSeedTimeH: 72, IsFree: true, FreeLevel: "FREE", FreeEndTime: &freeEnd},
	})
	require.NoError(t, err)
	require.True(t, res.Success)

	var got models.TorrentInfo
	require.NoError(t, db.DB.Where("site_name = ? AND torrent_id = ?", "hdsky", "b1").First(&got).Error)
	assert.Equal(t, "brush", got.DownloadSource)
	assert.True(t, got.HasHR)
	assert.Equal(t, 72, got.HRSeedTimeH)
	assert.EqualValues(t, gb, got.TorrentSize)
	assert.True(t, got.IsFree)
	assert.Equal(t, "FREE", got.FreeLevel)
	require.NotNil(t, got.FreeEndTime)
	assert.True(t, got.FreeEndTime.Equal(freeEnd))

	_, err = PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "hdsky", TorrentID: "b1", TorrentData: data, DownloaderID: ds.ID,
	})
	require.NoError(t, err)
	require.NoError(t, db.DB.Where("site_name = ? AND torrent_id = ?", "hdsky", "b1").First(&got).Error)
	assert.Equal(t, "manual_push", got.DownloadSource)
	assert.True(t, got.HasHR, "不带种子信息的推送不清掉已有的 H&R")
	assert.EqualValues(t, gb, got.TorrentSize)
}
