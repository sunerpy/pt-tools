// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

func TestProcessSingle_NoRecordDeletesFile(t *testing.T) {
	_ = setupDB(t)
	dir := t.TempDir()
	path, _ := makeTorrentFile(t, dir)
	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected file removed")
	}
}

func TestProcessSingle_PushedDeletes(t *testing.T) {
	db := setupDB(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	pushed := true
	now := time.Now()
	ti1 := &models.TorrentInfo{SiteName: string(models.SiteGroup("springsunday")), IsPushed: &pushed, PushTime: &now}
	ti1.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti1))
	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected file removed")
	}
}

func TestProcessSingle_ExistsUpdatesAndDeletes(t *testing.T) {
	db := setupDB(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	pushed := false
	ti2 := &models.TorrentInfo{SiteName: string(models.SiteGroup("springsunday")), IsPushed: &pushed}
	end := time.Now().Add(1 * time.Hour)
	ti2.FreeEndTime = &end
	ti2.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti2))
	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props200Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected file removed")
	}
	ti, err := db.GetTorrentBySiteAndHash(string(models.SiteGroup("springsunday")), hash)
	require.NoError(t, err)
	require.NotNil(t, ti)
	require.NotNil(t, ti.IsPushed)
	require.True(t, *ti.IsPushed)
	require.NotNil(t, ti.PushTime)
}

func TestProcessSingle_ExpiredDeletes(t *testing.T) {
	db := setupDB(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	past := time.Now().Add(-1 * time.Hour)
	ti := &models.TorrentInfo{SiteName: string(models.SiteGroup("springsunday")), FreeEndTime: &past}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))
	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected file removed")
	}
}

func TestProcessSingle_RetainHoursDeletes(t *testing.T) {
	db := setupDB(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	past := time.Now().Add(-48 * time.Hour)
	future := time.Now().Add(1 * time.Hour)
	ti := &models.TorrentInfo{SiteName: string(models.SiteGroup("springsunday")), LastCheckTime: &past, FreeEndTime: &future}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))
	store := core.NewConfigStore(db)
	require.NoError(t, store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true, RetainHours: 1}))
	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected file removed")
	}
}

func TestProcessSingle_MaxRetryDeletes(t *testing.T) {
	db := setupDB(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	ti3 := &models.TorrentInfo{SiteName: string(models.SiteGroup("springsunday")), RetryCount: 1}
	ti3.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti3))
	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected file removed")
	}
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

func TestProcessSingleTorrent_HashComputeFails(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	bad := filepath.Join(dir, "notatorrent.torrent")
	require.NoError(t, os.WriteFile(bad, []byte("not-bencode-data"), 0o644))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	err := processSingleTorrent(context.Background(), client, bad, "cat", "tag", models.SiteGroup("springsunday"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "计算种子哈希")
}

func TestProcessSingleTorrent_GetExpiredNoFreeEndDeletes(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	// FreeEndTime nil + FreeLevel "" → GetExpired() true → mark expired + delete.
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "gx", IsPushed: &pushed, FreeLevel: ""}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
}

func TestProcessSingleTorrent_CheckExistsError(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	future := time.Now().Add(time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "ce", FreeEndTime: &future, IsPushed: &pushed, FreeLevel: "free"}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &checkExistsErrTransport{}}, "http://example")
	err := processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "检查种子存在")
}

func (checkExistsErrTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/api/v2/auth/login" {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	}
	if req.URL.Path == "/api/v2/torrents/properties" {
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: http.NoBody, Header: make(http.Header)}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
}

func TestProcessSingleTorrent_OrphanNoDBRow(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, _ := makeTorrentFile(t, dir)

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "orphan (no DB row) must be deleted")
}

func TestProcessSingleTorrent_PushedRowDeletesFile(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	pushed := true
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "pr", IsPushed: &pushed, FreeLevel: "free"}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
}

func TestProcessSingleTorrent_PushFailUpdatesRetry(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	future := time.Now().Add(time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "pfu", FreeEndTime: &future, IsPushed: &pushed, FreeLevel: "free"}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: addFailTransport{}}, "http://example")
	err := processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday"))
	require.Error(t, err)
	got, gerr := db.GetTorrentBySiteAndHash("springsunday", hash)
	require.NoError(t, gerr)
	assert.Equal(t, 1, got.RetryCount)
	assert.NotEmpty(t, got.LastError)
}

func TestProcessSingleTorrent_MaxRetryDeletesFree(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), MaxRetry: 2,
	}))
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	future := time.Now().Add(time.Hour)
	pushed := false
	ti := &models.TorrentInfo{
		SiteName: "springsunday", TorrentID: "mrf", FreeEndTime: &future, IsPushed: &pushed,
		FreeLevel: "free", RetryCount: 9,
	}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "over-max-retry file must be deleted")
}

func TestProcessSingleTorrent_RetainHoursDeletes(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), RetainHours: 1,
	}))
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	oldCheck := time.Now().Add(-3 * time.Hour)
	pushed := false
	future := time.Now().Add(time.Hour)
	ti := &models.TorrentInfo{
		SiteName: "springsunday", TorrentID: "ret", IsPushed: &pushed,
		LastCheckTime: &oldCheck, FreeEndTime: &future,
	}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "retain-hours-expired unpushed file must be deleted")
}

func TestProcessSingleTorrent_AlreadyPushedDeletes(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	pushedTrue := true
	future := time.Now().Add(time.Hour)
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "ap", IsPushed: &pushedTrue, FreeEndTime: &future}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
}

func TestProcessSingleTorrent_ExpiredMarksAndDeletes(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	past := time.Now().Add(-time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "exp", FreeEndTime: &past, IsPushed: &pushed}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
	got, err := db.GetTorrentBySiteAndHash("springsunday", hash)
	require.NoError(t, err)
	assert.True(t, got.IsExpired)
}

func TestProcessSingleTorrent_ExistsSyncsPushed(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	future := time.Now().Add(time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "ex", FreeEndTime: &future, IsPushed: &pushed}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: existsTransport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
	got, err := db.GetTorrentBySiteAndHash("springsunday", hash)
	require.NoError(t, err)
	require.NotNil(t, got.IsPushed)
	assert.True(t, *got.IsPushed)
}

func TestProcessSingleTorrent_PushFailIncrementsRetry(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	future := time.Now().Add(time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "pf", FreeEndTime: &future, IsPushed: &pushed}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: addFailTransport{}}, "http://example")
	err := processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday"))
	require.Error(t, err)
	got, gerr := db.GetTorrentBySiteAndHash("springsunday", hash)
	require.NoError(t, gerr)
	assert.Equal(t, 1, got.RetryCount)
}

func TestProcessSingleTorrent_RetainHoursDeletesUnpushed(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), RetainHours: 1,
	}))
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	oldCheck := time.Now().Add(-5 * time.Hour)
	future := time.Now().Add(time.Hour)
	pushed := false
	ti := &models.TorrentInfo{
		SiteName: "springsunday", TorrentID: "rh", FreeEndTime: &future,
		IsPushed: &pushed, LastCheckTime: &oldCheck,
	}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
}

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

func (e *dlGenericErr) Error() string { return e.s }

func (addOKTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ok := func(body string) *http.Response {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(body)), Header: make(http.Header)}
	}
	switch req.URL.Path {
	case "/api/v2/auth/login":
		return ok("Ok."), nil
	case "/api/v2/torrents/properties":
		return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: make(http.Header)}, nil
	case "/api/v2/sync/maindata":
		return ok(`{"server_state":{"free_space_on_disk":107374182400}}`), nil
	case "/api/v2/torrents/add":
		return ok("Ok."), nil
	default:
		return ok("{}"), nil
	}
}

func TestProcessSingle_SuccessfulPush(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	future := time.Now().Add(1 * time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "ok1", FreeEndTime: &future, IsPushed: &pushed}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: addOKTransport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "pushed torrent file must be removed")
	got, err := db.GetTorrentBySiteAndHash("springsunday", hash)
	require.NoError(t, err)
	require.NotNil(t, got.IsPushed)
	assert.True(t, *got.IsPushed)
}

func TestProcessSingle_MaxRetryWithFutureFree(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), MaxRetry: 2,
	}))
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	future := time.Now().Add(1 * time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "mr2", FreeEndTime: &future, IsPushed: &pushed, RetryCount: 5}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
}

func TestProcessSingleWithDownloader_OrphanDeletes(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, _ := makeTorrentFile(t, dir)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("d").AnyTimes()

	dlInfo := &DownloaderInfo{ID: 1, Name: "d"}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	require.NoError(t, err)
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "orphan file must be deleted")
}

func TestProcessSingleWithDownloader_ExpiredMarks(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	past := time.Now().Add(-1 * time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "exp", FreeEndTime: &past, IsPushed: &pushed}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("d").AnyTimes()

	dlInfo := &DownloaderInfo{ID: 1, Name: "d"}
	require.NoError(t, processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", models.SiteGroup("springsunday"), false))

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
	got, err := db.GetTorrentBySiteAndHash("springsunday", hash)
	require.NoError(t, err)
	assert.True(t, got.IsExpired)
}

func TestProcessSingleWithDownloader_AlreadyExistsSyncsPushed(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	future := time.Now().Add(1 * time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "ex", FreeEndTime: &future, IsPushed: &pushed}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("d").AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(true, nil)

	dlInfo := &DownloaderInfo{ID: 7, Name: "d"}
	require.NoError(t, processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", models.SiteGroup("springsunday"), false))

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
	got, err := db.GetTorrentBySiteAndHash("springsunday", hash)
	require.NoError(t, err)
	require.NotNil(t, got.IsPushed)
	assert.True(t, *got.IsPushed)
}

func TestProcessSingleWithDownloader_ExistsSchedulesFreeEnd(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	future := time.Now().Add(1 * time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "exsc", FreeEndTime: &future, IsPushed: &pushed}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	var scheduled string
	RegisterTorrentScheduler(func(ti models.TorrentInfo) { scheduled = ti.TorrentID })
	t.Cleanup(func() { RegisterTorrentScheduler(nil) })

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("d").AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(true, nil)

	dlInfo := &DownloaderInfo{ID: 8, Name: "d"}
	require.NoError(t, processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", models.SiteGroup("springsunday"), true))
	assert.Equal(t, "exsc", scheduled)
}

func TestProcessSingleWithDownloader_MaxRetryDeletes(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), MaxRetry: 2,
	}))
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	future := time.Now().Add(1 * time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "mr", FreeEndTime: &future, IsPushed: &pushed, RetryCount: 5}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("d").AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)

	dlInfo := &DownloaderInfo{ID: 1, Name: "d"}
	require.NoError(t, processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", models.SiteGroup("springsunday"), false))
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "over-max-retry file must be deleted")
}

func TestProcessSingleWithDownloader_SuccessSchedulesFreeEnd(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	future := time.Now().Add(2 * time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "sc", FreeEndTime: &future, IsPushed: &pushed}
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

	assert.Equal(t, "sc", scheduled, "pauseOnFreeEnd torrent with free-end must be scheduled")
	got, err := db.GetTorrentBySiteAndHash("springsunday", hash)
	require.NoError(t, err)
	require.NotNil(t, got.IsPushed)
	assert.True(t, *got.IsPushed)
}

func TestProcessSingleWithDownloader_PushFailIncrementsRetry(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	future := time.Now().Add(2 * time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "pf", FreeEndTime: &future, IsPushed: &pushed}
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
		Return(downloader.AddTorrentResult{Success: false, Message: "boom"}, nil)

	dlInfo := &DownloaderInfo{ID: 1, Name: "d", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	require.Error(t, err)

	got, err := db.GetTorrentBySiteAndHash("springsunday", hash)
	require.NoError(t, err)
	assert.Equal(t, 1, got.RetryCount)
	assert.Contains(t, got.LastError, "boom")
}
