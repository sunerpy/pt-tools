// MIT License
// Copyright (c) 2025 pt-tools

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
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func TestGetDownloaderForRSSImpl_RSSDownloaderIDMissing(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	id := uint(999)
	_, _, err := getDownloaderForRSSImpl(models.RSSConfig{DownloaderID: &id}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "获取指定下载器")
}

func TestGetDownloaderForRSSImpl_RSSDownloaderDisabled(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	ds := models.DownloaderSetting{Name: "off", Type: "qbittorrent", URL: "http://x", Enabled: false}
	require.NoError(t, db.DB.Create(&ds).Error)
	_, _, err := getDownloaderForRSSImpl(models.RSSConfig{DownloaderID: &ds.ID}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "未启用")
}

func TestGetDownloaderForRSSImpl_SiteBoundDisabled(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	ds := models.DownloaderSetting{Name: "sbd", Type: "qbittorrent", URL: "http://x", Enabled: false}
	require.NoError(t, db.DB.Create(&ds).Error)
	require.NoError(t, db.DB.Create(&models.SiteSetting{Name: "mteam", DownloaderID: &ds.ID}).Error)
	_, _, err := getDownloaderForRSSImpl(models.RSSConfig{}, "mteam")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "未启用")
}

func TestGetDownloaderForRSSImpl_DefaultDisabled(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	ds := models.DownloaderSetting{Name: "def", Type: "qbittorrent", URL: "http://x", Enabled: false, IsDefault: true}
	require.NoError(t, db.DB.Create(&ds).Error)
	_, _, err := getDownloaderForRSSImpl(models.RSSConfig{}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "默认下载器")
}

func TestGetDownloaderForRSSImpl_NilDB(t *testing.T) {
	global.GlobalDB = nil
	_, _, err := getDownloaderForRSSImpl(models.RSSConfig{}, "")
	require.Error(t, err)
}

func TestGetDownloaderForRSSImpl_NoDefaultConfigured(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	_, _, err := getDownloaderForRSSImpl(models.RSSConfig{}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "获取默认下载器")
}

func TestProcessSingleWithDownloader_SuccessPushSchedules(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	future := time.Now().Add(2 * time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "ps", FreeEndTime: &future, IsPushed: &pushed}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	var scheduled string
	RegisterTorrentScheduler(func(ti models.TorrentInfo) { scheduled = ti.TorrentID })
	t.Cleanup(func() { RegisterTorrentScheduler(nil) })

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("d").AnyTimes()
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(int64(1)<<50, nil).AnyTimes()
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil).AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	mockDl.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).
		Return(downloader.AddTorrentResult{Success: true, Hash: hash}, nil)

	dlInfo := &DownloaderInfo{ID: 3, Name: "d", AutoStart: true}
	require.NoError(t, processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", models.SiteGroup("springsunday"), true))
	assert.Equal(t, "ps", scheduled)
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
}

func TestProcessSingleWithDownloader_AddErrorIncrementsRetry(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	future := time.Now().Add(2 * time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "pae", FreeEndTime: &future, IsPushed: &pushed}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("d").AnyTimes()
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(int64(1)<<50, nil).AnyTimes()
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil).AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	mockDl.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).
		Return(downloader.AddTorrentResult{}, assertDLErr)

	dlInfo := &DownloaderInfo{ID: 1, Name: "d", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	require.Error(t, err)
	got, gerr := db.GetTorrentBySiteAndHash("springsunday", hash)
	require.NoError(t, gerr)
	assert.Equal(t, 1, got.RetryCount)
}

func TestProcessSingleWithDownloader_CheckExistsError(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	future := time.Now().Add(2 * time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "pce", FreeEndTime: &future, IsPushed: &pushed}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("d").AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, assertDLErr)

	dlInfo := &DownloaderInfo{ID: 1, Name: "d"}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "检查种子存在")
}
