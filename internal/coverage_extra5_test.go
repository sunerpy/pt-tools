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

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	sm "github.com/sunerpy/pt-tools/mocks"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

func TestProcessSingleWithDownloader_FilterRuleSkipsExpireCheck(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	rule := models.FilterRule{Name: "r", Pattern: ".*", PatternType: "regex", MatchField: "both", RequireFree: false, Enabled: true}
	require.NoError(t, db.DB.Create(&rule).Error)
	require.NoError(t, db.DB.Model(&models.FilterRule{}).Where("id = ?", rule.ID).Update("require_free", false).Error)

	past := time.Now().Add(-time.Hour)
	pushed := false
	ti := &models.TorrentInfo{
		SiteName: "springsunday", TorrentID: "fr", FreeEndTime: &past, IsPushed: &pushed,
		DownloadSource: "filter_rule", FilterRuleID: &rule.ID,
	}
	ti.TorrentHash = &hash
	require.NoError(t, db.DB.Create(ti).Error)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("d").AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(true, nil)

	dlInfo := &DownloaderInfo{ID: 1, Name: "d"}
	require.NoError(t, processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", models.SiteGroup("springsunday"), false))
	got, err := db.GetTorrentBySiteAndHash("springsunday", hash)
	require.NoError(t, err)
	assert.False(t, got.IsExpired, "filter-rule non-free torrent must skip expire marking")
	require.NotNil(t, got.IsPushed)
	assert.True(t, *got.IsPushed)
}

func TestProcessSingleWithDownloader_DiskProtectInsufficient(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	GetDiskBudget().Reset()
	t.Cleanup(func() { GetDiskBudget().Reset() })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), CleanupDiskProtect: true, CleanupMinDiskSpaceGB: 20,
	}))
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	future := time.Now().Add(time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "dp", FreeEndTime: &future, IsPushed: &pushed}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("d").AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	// free 5GB <= min 20GB → insufficient.
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(int64(5)*(1<<30), nil).AnyTimes()
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil).AnyTimes()

	dlInfo := &DownloaderInfo{ID: 1, Name: "d"}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	require.ErrorIs(t, err, downloader.ErrInsufficientSpace)
	_, statErr := os.Stat(path)
	assert.NoError(t, statErr, "file kept for retry when disk full")
}

func TestProcessSingleWithDownloader_DiskProtectTooLarge(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	GetDiskBudget().Reset()
	t.Cleanup(func() { GetDiskBudget().Reset() })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), CleanupDiskProtect: true, CleanupMinDiskSpaceGB: 10,
	}))
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	future := time.Now().Add(time.Hour)
	pushed := false
	ti := &models.TorrentInfo{
		SiteName: "springsunday", TorrentID: "big", FreeEndTime: &future, IsPushed: &pushed,
		TorrentSize: 25 * (1 << 30),
	}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("d").AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	// free 30GB, torrent 25GB, min 10GB → 30-25=5 < 10 → too large.
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(int64(30)*(1<<30), nil).AnyTimes()
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil).AnyTimes()

	dlInfo := &DownloaderInfo{ID: 1, Name: "d"}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	require.ErrorIs(t, err, downloader.ErrTorrentTooLarge)
}

func TestProcessSingleWithDownloader_DiskProtectFreeSpaceReadFails(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	GetDiskBudget().Reset()
	t.Cleanup(func() { GetDiskBudget().Reset() })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), CleanupDiskProtect: true, CleanupMinDiskSpaceGB: 10,
	}))
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	future := time.Now().Add(time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "fsp", FreeEndTime: &future, IsPushed: &pushed}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("d").AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(int64(0), assertDLErr).AnyTimes()

	dlInfo := &DownloaderInfo{ID: 1, Name: "d"}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	require.ErrorIs(t, err, downloader.ErrInsufficientSpace)
}

var assertDLErr = &dlGenericErr{"disk read boom"}

type dlGenericErr struct{ s string }

func (e *dlGenericErr) Error() string { return e.s }
