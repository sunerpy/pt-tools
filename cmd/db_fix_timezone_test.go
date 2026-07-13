package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// prepareRuntimeDB ensures the singleton runtime is initialized once, then
// replaces global.GlobalDB with a fresh temp DB so command handlers that call
// initTools() operate against a clean, isolated schema. InitRuntime uses
// sync.Once, so after the first call it will not clobber the temp DB we set.
func prepareRuntimeDB(t *testing.T) *models.TorrentDB {
	t.Helper()
	_, _ = core.InitRuntime()
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	return db
}

func TestTruncateTitle(t *testing.T) {
	assert.Equal(t, "short", truncateTitle("short", 60))
	// Exactly at limit returns unchanged.
	assert.Equal(t, "abcde", truncateTitle("abcde", 5))
	// Over limit gets truncated with ellipsis; result rune length == maxLen.
	long := strings.Repeat("x", 100)
	got := truncateTitle(long, 10)
	assert.Equal(t, 10, len([]rune(got)))
	assert.True(t, strings.HasSuffix(got, "..."))
	// Multibyte-safe truncation.
	cn := strings.Repeat("测", 100)
	gotCN := truncateTitle(cn, 10)
	assert.Equal(t, 10, len([]rune(gotCN)))
}

func TestRunFixTimezone_NoRecords(t *testing.T) {
	prepareRuntimeDB(t)
	fixTimezoneDryRun = false
	require.NoError(t, runFixTimezone(&cobra.Command{}, nil))
}

func TestRunFixTimezone_DryRun(t *testing.T) {
	db := prepareRuntimeDB(t)
	ft := time.Date(2024, 1, 2, 15, 4, 5, 0, time.UTC)
	require.NoError(t, db.DB.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "t1", Title: strings.Repeat("标题", 50),
		FreeEndTime: &ft,
	}).Error)
	require.NoError(t, db.DB.Create(&models.TorrentInfoArchive{
		SiteName: "hdsky", TorrentID: "a1", FreeEndTime: &ft,
	}).Error)

	fixTimezoneDryRun = true
	t.Cleanup(func() { fixTimezoneDryRun = false })
	require.NoError(t, runFixTimezone(&cobra.Command{}, nil))

	// Dry-run must NOT mutate the stored time.
	var got models.TorrentInfo
	require.NoError(t, db.DB.First(&got, "torrent_id = ?", "t1").Error)
	require.NotNil(t, got.FreeEndTime)
	assert.True(t, got.FreeEndTime.Equal(ft), "dry-run must not modify data")
}

func TestRunFixTimezone_AppliesOffset(t *testing.T) {
	db := prepareRuntimeDB(t)
	ft := time.Date(2024, 1, 2, 15, 4, 5, 0, time.UTC)
	require.NoError(t, db.DB.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "t1", Title: "hello", FreeEndTime: &ft,
	}).Error)
	archiveTime := time.Date(2024, 3, 4, 5, 6, 7, 0, time.UTC)
	require.NoError(t, db.DB.Create(&models.TorrentInfoArchive{
		SiteName: "hdsky", TorrentID: "a1", FreeEndTime: &archiveTime,
	}).Error)

	fixTimezoneDryRun = false
	require.NoError(t, runFixTimezone(&cobra.Command{}, nil))

	var got models.TorrentInfo
	require.NoError(t, db.DB.First(&got, "torrent_id = ?", "t1").Error)
	require.NotNil(t, got.FreeEndTime)
	assert.True(t, got.FreeEndTime.Equal(ft.Add(-8*time.Hour)),
		"free_end_time should be shifted back by 8h")

	var arch models.TorrentInfoArchive
	require.NoError(t, db.DB.First(&arch, "torrent_id = ?", "a1").Error)
	require.NotNil(t, arch.FreeEndTime)
	assert.True(t, arch.FreeEndTime.Equal(archiveTime.Add(-8*time.Hour)))
}

func TestRunFixTimezone_NilDB(t *testing.T) {
	_, _ = core.InitRuntime()
	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })
	err := runFixTimezone(&cobra.Command{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "数据库未初始化")
}
