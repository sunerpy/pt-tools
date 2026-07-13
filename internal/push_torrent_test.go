// MIT License
// Copyright (c) 2025 pt-tools

// Coverage for internal/push.go: PushTorrentToDownloader error/guard paths,
// createDownloaderInstanceForPush factory branches, and
// recordPushDiskProtectError DB writeback. The happy push path is exercised
// via the RSS path in disk_protect_test.go; here we drive the failure branches
// that don't require a live downloader.

package internal

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func TestPushTorrent_DBNil(t *testing.T) {
	global.GlobalDB = nil
	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "数据库未初始化")
}

func TestPushTorrent_DownloaderNotFound(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		DownloaderID: 999, // nonexistent
	})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "获取下载器失败")
}

func TestPushTorrent_DownloaderDisabled(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	ds := models.DownloaderSetting{Name: "off", Type: "qbittorrent", URL: "http://x", Enabled: false}
	require.NoError(t, db.DB.Create(&ds).Error)

	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{DownloaderID: ds.ID})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "未启用")
}

func TestPushTorrent_UnsupportedType(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	ds := models.DownloaderSetting{Name: "weird", Type: "aria2", URL: "http://x", Enabled: true}
	require.NoError(t, db.DB.Create(&ds).Error)

	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{DownloaderID: ds.ID})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "创建下载器实例失败")
}

func TestPushTorrent_InstanceCreationFailsUnreachable(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	// qbittorrent constructor eagerly authenticates; an unreachable URL makes
	// createDownloaderInstanceForPush fail before hashing.
	ds := models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:0", Enabled: true}
	require.NoError(t, db.DB.Create(&ds).Error)

	res, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID:       "springsunday",
		TorrentID:    "t1",
		TorrentData:  []byte("not a torrent"),
		DownloaderID: ds.ID,
	})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Contains(t, err.Error(), "创建下载器实例失败")
}

func TestCreateDownloaderInstanceForPush(t *testing.T) {
	// qbittorrent branch executes (connection may fail; we only cover the branch).
	_, _ = createDownloaderInstanceForPush(models.DownloaderSetting{
		Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:0",
	})

	// transmission branch executes.
	_, _ = createDownloaderInstanceForPush(models.DownloaderSetting{
		Name: "tr", Type: "Transmission", URL: "http://127.0.0.1:0", AutoStart: true,
	})

	// unsupported -> error.
	_, err := createDownloaderInstanceForPush(models.DownloaderSetting{Name: "x", Type: "deluge"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不支持的下载器类型")
}

func TestRecordPushDiskProtectError(t *testing.T) {
	// DB nil -> no-op, no panic.
	global.GlobalDB = nil
	recordPushDiskProtectError("s", "t", "msg")

	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "t9", IsPushed: &pushed}
	require.NoError(t, db.UpsertTorrent(ti))

	recordPushDiskProtectError("springsunday", "t9", "磁盘空间不足")

	got, err := db.GetTorrentBySiteAndID("springsunday", "t9")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "磁盘空间不足", got.LastError)
}

func TestApplySiteSpeedLimits(t *testing.T) {
	// nil opts -> no-op, no panic.
	applySiteSpeedLimits(nil, "s")

	// empty site name -> no-op leaves opts zeroed.
	opts := &downloader.AddTorrentOptions{}
	applySiteSpeedLimits(opts, "")
	assert.Zero(t, opts.UploadSpeedLimitKBs)

	// DB nil -> no-op.
	global.GlobalDB = nil
	applySiteSpeedLimits(opts, "mteam")
	assert.Zero(t, opts.UploadSpeedLimitKBs)

	// Unknown site (lookup fails) -> no-op leaves zeroed.
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	_ = db
	applySiteSpeedLimits(opts, "nosuchsite")
	assert.Zero(t, opts.UploadSpeedLimitKBs)

	// Known site -> limits applied.
	require.NoError(t, db.DB.Create(&models.SiteSetting{
		Name: "mteam", UploadLimitKBs: 100, DownloadLimitKBs: 200,
	}).Error)
	applySiteSpeedLimits(opts, "mteam")
	assert.Equal(t, 100, opts.UploadSpeedLimitKBs)
	assert.Equal(t, 200, opts.DownloadSpeedLimitKBs)
}
