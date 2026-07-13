// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

func TestDiskBudget_ReservedNormalizesNegative(t *testing.T) {
	b := &DiskBudget{}
	b.Release(50 * gb) // release with nothing reserved → clamps to 0
	assert.Equal(t, int64(0), b.Reserved())
}

func TestIsEnabled_Branches(t *testing.T) {
	global.GlobalDB = nil
	impl := &UnifiedSiteImpl{siteGroup: models.SiteGroup("springsunday")}
	assert.False(t, impl.IsEnabled())

	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	// Site not present in config → false.
	impl2, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("springsunday"))
	require.NoError(t, err)
	assert.False(t, impl2.IsEnabled())

	// Enabled=true → true.
	e := true
	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{
		Enabled: &e, AuthMethod: "cookie", Cookie: "c=1",
	}))
	impl3, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("springsunday"))
	require.NoError(t, err)
	assert.True(t, impl3.IsEnabled())
}

func TestRecordDiskProtectError_NilDB(t *testing.T) {
	global.GlobalDB = nil
	require.NotPanics(t, func() { recordDiskProtectError(models.SiteGroup("s"), "h", "msg") })
}

func TestRecordPushDiskProtectError_NilDB(t *testing.T) {
	global.GlobalDB = nil
	require.NotPanics(t, func() { recordPushDiskProtectError("s", "id", "msg") })
}

func TestRecordPushDiskProtectError_WritesLastError(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, db.DB.Create(&models.TorrentInfo{SiteName: "springsunday", TorrentID: "rp1"}).Error)
	recordPushDiskProtectError("springsunday", "rp1", "磁盘满")
	got, err := db.GetTorrentBySiteAndID("springsunday", "rp1")
	require.NoError(t, err)
	assert.Equal(t, "磁盘满", got.LastError)
}

func TestPushTorrentToDownloader_NilDB(t *testing.T) {
	global.GlobalDB = nil
	_, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "数据库未初始化")
}

func TestPushTorrentToDownloader_ComputeHashFails(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	srv := fakeQbitServer(t, false, 500*gb)
	dlID := seedQbitDownloader(t, srv.URL)
	_ = db

	_, err := PushTorrentToDownloader(context.Background(), PushTorrentRequest{
		SiteID: "springsunday", TorrentID: "bad", TorrentData: []byte("not-a-torrent"), DownloaderID: dlID,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "计算种子哈希")
}

func TestFetchMTorrentDetail_MissingConfig(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, db.DB.Create(&models.SiteSetting{Name: "mteam", AuthMethod: "api_key"}).Error)
	_, ferr := fetchMTorrentDetail(context.Background(), models.SiteGroup("mteam"), &gofeed.Item{GUID: "1"})
	require.Error(t, ferr)
	assert.Contains(t, ferr.Error(), "API 未配置")
}

func TestFetchMTorrentDetail_APIErrorCode(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"500","message":"boom"}`))
	}))
	t.Cleanup(srv.Close)
	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("mteam"), models.SiteConfig{
		Enabled: boolPtrIX(true), AuthMethod: "api_key", APIKey: "k", APIUrl: srv.URL,
	}))
	_, err := fetchMTorrentDetail(context.Background(), models.SiteGroup("mteam"), &gofeed.Item{GUID: "1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API error")
}

func TestFetchMTorrentDetail_Non200(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("mteam"), models.SiteConfig{
		Enabled: boolPtrIX(true), AuthMethod: "api_key", APIKey: "k", APIUrl: srv.URL,
	}))
	_, err := fetchMTorrentDetail(context.Background(), models.SiteGroup("mteam"), &gofeed.Item{GUID: "1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "状态码")
}

func TestFetchNexusPHPDetail_MissingCookie(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, db.DB.Create(&models.SiteSetting{Name: "springsunday", AuthMethod: "cookie"}).Error)
	_, err := fetchNexusPHPDetail(context.Background(), models.SiteGroup("springsunday"), &gofeed.Item{GUID: "1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Cookie 未配置")
}

func TestSweepStagingDir_RemovesOrphan(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	_ = db
	dir := t.TempDir()
	fp := filepath.Join(dir, "springsunday-orphan.torrent")
	require.NoError(t, os.WriteFile(fp, []byte("d4:infod4:name1:aee"), 0o644))
	// No DB row → shouldSweep returns true (orphan).
	sweepStagingDir(dir, models.SiteGroup("springsunday"), 24)
	_, statErr := os.Stat(fp)
	assert.True(t, os.IsNotExist(statErr))
}

func TestShouldSweep_MaxRetryExceeded(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), MaxRetry: 2,
	}))
	dir := t.TempDir()
	fp, hash := makeTorrentFile(t, dir)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "sw", IsPushed: &pushed, RetryCount: 5}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))
	assert.True(t, shouldSweep(fp, models.SiteGroup("springsunday"), 24))
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

func TestSendTorrentToDownloader_MissingDirNoOp(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	require.NoError(t, core.NewConfigStore(db).SaveGlobal(models.SettingsGlobal{
		DownloadDir: dir, DefaultIntervalMinutes: 10, DefaultEnabled: true,
	}))
	impl, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("springsunday"))
	require.NoError(t, err)
	// Tag path that does not exist yet gets MkdirAll'd then reported empty → no-op.
	require.NoError(t, impl.SendTorrentToDownloader(context.Background(), models.RSSConfig{Tag: "brand-new-tag"}))
}

func TestGetTorrentDetails_Disabled(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	_ = db
	impl, err := NewUnifiedSiteImpl(context.Background(), models.SiteGroup("springsunday"))
	require.NoError(t, err)
	_, err = impl.GetTorrentDetails(&gofeed.Item{GUID: "1"})
	require.Error(t, err)
}

func boolPtrIX(b bool) *bool { return &b }

var _ = strings.Contains
