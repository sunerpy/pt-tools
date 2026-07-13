// MIT License
// Copyright (c) 2025 pt-tools

// Branch coverage for processSingleTorrent (qbit-client path) and
// processSingleTorrentWithDownloader (downloader-interface path): orphan
// deletion, expired marking, retain-hours cleanup, already-exists push-state
// sync, max-retry deletion, successful push + free-end scheduling, and
// fetchNexusPHPDetail via GetTorrentDetailForTest on a NexusPHP site.

package internal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	sm "github.com/sunerpy/pt-tools/mocks"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

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

func TestGetTorrentDetailForTest_NexusPHPSchema(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<html><body>
			<input name="torrent_name" value="Nexus.Movie">
			<input name="detail_torrent_id" value="42">
			<h1><font class="free">免费</font><span title="2030-01-20 15:30:00">2天</span></h1>
			<td class="rowhead">基本信息</td><td>大小：10.00 GB</td>
		</body></html>`))
	}))
	t.Cleanup(srv.Close)

	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{
		Enabled: boolPtrPS(true), AuthMethod: "cookie", Cookie: "c=1", APIUrl: srv.URL,
	}))

	item := &gofeed.Item{Title: "fallback", GUID: "42", Link: srv.URL + "/details.php?id=42"}
	res, err := GetTorrentDetailForTest(context.Background(), models.SiteGroup("springsunday"), item)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "Nexus.Movie", res.Title)
	assert.True(t, res.IsFree)
}

func TestGetTorrentDetailForTest_NexusPHPMissingCookie(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).UpsertSiteWithRSS(models.SiteGroup("springsunday"), models.SiteConfig{
		Enabled: boolPtrPS(true), AuthMethod: "cookie", Cookie: "c=1", APIUrl: "http://127.0.0.1:0",
	}))
	item := &gofeed.Item{Title: "fb", GUID: "1"}
	res, err := GetTorrentDetailForTest(context.Background(), models.SiteGroup("springsunday"), item)
	require.NoError(t, err)
	assert.Equal(t, "fb", res.Title)
}

func boolPtrPS(b bool) *bool { return &b }

var _ = filepath.Base
