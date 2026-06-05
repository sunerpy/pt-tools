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
)

// TestRSSPushPath_AppliesSiteSpeedLimits is the end-to-end regression guard for
// the bug where the RSS/common push path never applied per-site speed limits
// (only the manual push path did). It drives processSingleTorrentWithDownloader
// and captures the AddTorrentOptions handed to the downloader, asserting the
// site's UploadLimitKBs/DownloadLimitKBs actually reach AddTorrentFileEx.
func TestRSSPushPath_AppliesSiteSpeedLimits(t *testing.T) {
	setUpDiskProtectTest(t)
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	makeTorrentInfoWithSize(t, global.GlobalDB, hash, 1*gb)

	require.NoError(t, global.GlobalDB.DB.Create(&models.SiteSetting{
		Name:             "springsunday",
		AuthMethod:       "cookie",
		UploadLimitKBs:   500,
		DownloadLimitKBs: 2000,
	}).Error)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDl := sm.NewMockDownloader(ctrl)
	mockDl.EXPECT().GetName().Return("test-dl").AnyTimes()
	mockDl.EXPECT().GetType().Return(downloader.DownloaderQBittorrent).AnyTimes()
	mockDl.EXPECT().CheckTorrentExists(hash).Return(false, nil)
	mockDl.EXPECT().GetClientFreeSpace(gomock.Any()).Return(80*gb, nil)
	mockDl.EXPECT().GetIncompletePendingBytes(gomock.Any()).Return(int64(0), nil)

	var captured downloader.AddTorrentOptions
	mockDl.EXPECT().AddTorrentFileEx(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ []byte, opt downloader.AddTorrentOptions) (downloader.AddTorrentResult, error) {
			captured = opt
			return downloader.AddTorrentResult{Success: true, Hash: hash}, nil
		})

	dlInfo := &DownloaderInfo{ID: 1, Name: "test-dl", AutoStart: true}
	err := processSingleTorrentWithDownloader(context.Background(), mockDl, dlInfo,
		path, "cat", "tag", "", models.SiteGroup("springsunday"), false)
	require.NoError(t, err)

	assert.Equal(t, 500, captured.UploadSpeedLimitKBs,
		"RSS push path must apply site upload limit (regression: previously never called applySiteSpeedLimits)")
	assert.Equal(t, 2000, captured.DownloadSpeedLimitKBs,
		"RSS push path must apply site download limit")
}

// TestApplySiteSpeedLimits_PopulatesFromSiteRow verifies the core integration
// point for issue #276: push flow reads per-site speed limits from SiteSetting
// and populates AddTorrentOptions, so the downstream downloader.AddTorrentFileEx
// applies them atomically.
func TestApplySiteSpeedLimits_PopulatesFromSiteRow(t *testing.T) {
	db := setupDB(t)
	require.NoError(t, db.DB.Create(&models.SiteSetting{
		Name:             "springsunday",
		AuthMethod:       "cookie",
		UploadLimitKBs:   500,
		DownloadLimitKBs: 2000,
	}).Error)

	opts := downloader.AddTorrentOptions{}
	applySiteSpeedLimits(&opts, "springsunday")

	assert.Equal(t, 500, opts.UploadSpeedLimitKBs)
	assert.Equal(t, 2000, opts.DownloadSpeedLimitKBs)
}

// TestApplySiteSpeedLimits_ZeroLimits verifies that sites with no limits
// configured leave opts at zero (meaning "unlimited" downstream). Regression
// guard: ensure the feature is truly opt-in.
func TestApplySiteSpeedLimits_ZeroLimits(t *testing.T) {
	db := setupDB(t)
	require.NoError(t, db.DB.Create(&models.SiteSetting{
		Name:       "springsunday",
		AuthMethod: "cookie",
	}).Error)

	opts := downloader.AddTorrentOptions{}
	applySiteSpeedLimits(&opts, "springsunday")

	assert.Equal(t, 0, opts.UploadSpeedLimitKBs)
	assert.Equal(t, 0, opts.DownloadSpeedLimitKBs)
}

// TestApplySiteSpeedLimits_UnknownSiteNoOp verifies that an unknown site name
// is silently a no-op (opts unchanged). This is the safety contract that
// allows the push flow to pass any SiteID without risking a panic or error.
func TestApplySiteSpeedLimits_UnknownSiteNoOp(t *testing.T) {
	_ = setupDB(t)

	opts := downloader.AddTorrentOptions{UploadSpeedLimitKBs: 123, DownloadSpeedLimitKBs: 456}
	applySiteSpeedLimits(&opts, "nonexistent-site")

	assert.Equal(t, 123, opts.UploadSpeedLimitKBs, "pre-existing value must not be wiped")
	assert.Equal(t, 456, opts.DownloadSpeedLimitKBs)
}

// TestApplySiteSpeedLimits_EmptySiteNameNoOp verifies no-op on empty siteName.
func TestApplySiteSpeedLimits_EmptySiteNameNoOp(t *testing.T) {
	_ = setupDB(t)
	opts := downloader.AddTorrentOptions{}
	applySiteSpeedLimits(&opts, "")
	assert.Equal(t, 0, opts.UploadSpeedLimitKBs)
	assert.Equal(t, 0, opts.DownloadSpeedLimitKBs)
}

// TestApplySiteSpeedLimits_NilOpts verifies no panic on nil.
func TestApplySiteSpeedLimits_NilOpts(t *testing.T) {
	_ = setupDB(t)
	assert.NotPanics(t, func() {
		applySiteSpeedLimits(nil, "any-site")
	})
}

// TestApplySiteSpeedLimits_NilDB verifies no panic when DB is not initialized.
// Regression guard: push flow should not crash during early-stage testing
// where global.GlobalDB may be unset.
func TestApplySiteSpeedLimits_NilDB(t *testing.T) {
	origDB := global.GlobalDB
	global.GlobalDB = nil
	defer func() { global.GlobalDB = origDB }()

	opts := downloader.AddTorrentOptions{}
	assert.NotPanics(t, func() {
		applySiteSpeedLimits(&opts, "any-site")
	})
	assert.Equal(t, 0, opts.UploadSpeedLimitKBs)
}

// TestApplySiteSpeedLimits_SiteRowUpdates verifies that changes to the site
// row are reflected immediately (no caching). Regression guard: if caching
// is ever introduced, it must correctly invalidate on settings change.
func TestApplySiteSpeedLimits_SiteRowUpdates(t *testing.T) {
	db := setupDB(t)
	site := &models.SiteSetting{
		Name:             "hdsky",
		AuthMethod:       "cookie",
		UploadLimitKBs:   100,
		DownloadLimitKBs: 200,
	}
	require.NoError(t, db.DB.Create(site).Error)

	opts1 := downloader.AddTorrentOptions{}
	applySiteSpeedLimits(&opts1, "hdsky")
	assert.Equal(t, 100, opts1.UploadSpeedLimitKBs)

	site.UploadLimitKBs = 999
	require.NoError(t, db.DB.Save(site).Error)

	opts2 := downloader.AddTorrentOptions{}
	applySiteSpeedLimits(&opts2, "hdsky")
	assert.Equal(t, 999, opts2.UploadSpeedLimitKBs, "updated limit must be reflected on next push")
}
