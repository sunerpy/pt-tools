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

// TestRunFixTimezone_SkipsNilFreeEndTime seeds rows that have free_end_time set
// at the SQL level but decode with a nil pointer is not possible; instead this
// verifies the loop handles a mix and updates only non-nil entries. It drives
// the archive query success + archive update path together.
func TestRunFixTimezone_MixedTorrentAndArchive(t *testing.T) {
	db := prepareRuntimeDB(t)
	ft := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, db.DB.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "t1", Title: "x", FreeEndTime: &ft,
	}).Error)
	require.NoError(t, db.DB.Create(&models.TorrentInfoArchive{
		SiteName: "hdsky", TorrentID: "a1", FreeEndTime: &ft,
	}).Error)

	fixTimezoneDryRun = false
	require.NoError(t, runFixTimezone(&cobra.Command{}, nil))

	var got models.TorrentInfo
	require.NoError(t, db.DB.First(&got, "torrent_id = ?", "t1").Error)
	assert.True(t, got.FreeEndTime.Equal(ft.Add(-8*time.Hour)))
}

// TestRunFixTimezone_UpdateErrorsContinue drives the update-failure branches
// for both torrent and archive rows. A BEFORE UPDATE trigger lets the SELECT
// succeed but aborts every UPDATE, so runFixTimezone logs the failure and
// continues (fixedCount stays 0) without returning an error.
func TestRunFixTimezone_UpdateErrorsContinue(t *testing.T) {
	db := prepareRuntimeDB(t)
	ft := time.Date(2024, 7, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, db.DB.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "t1", Title: "x", FreeEndTime: &ft,
	}).Error)
	require.NoError(t, db.DB.Create(&models.TorrentInfoArchive{
		SiteName: "hdsky", TorrentID: "a1", FreeEndTime: &ft,
	}).Error)

	require.NoError(t, db.DB.Exec(
		`CREATE TRIGGER block_ti BEFORE UPDATE ON torrent_infos BEGIN SELECT RAISE(ABORT,'no update'); END;`,
	).Error)
	require.NoError(t, db.DB.Exec(
		`CREATE TRIGGER block_tia BEFORE UPDATE ON torrent_info_archives BEGIN SELECT RAISE(ABORT,'no update'); END;`,
	).Error)

	fixTimezoneDryRun = false
	require.NoError(t, runFixTimezone(&cobra.Command{}, nil))

	var got models.TorrentInfo
	require.NoError(t, db.DB.First(&got, "torrent_id = ?", "t1").Error)
	assert.True(t, got.FreeEndTime.Equal(ft), "update must have been aborted, value unchanged")
}

// TestRunFixTimezone_TorrentQueryError forces the torrent_infos Find to fail by
// dropping the table after runtime init, exercising the query-error return.
func TestRunFixTimezone_TorrentQueryError(t *testing.T) {
	db := prepareRuntimeDB(t)
	require.NoError(t, db.DB.Migrator().DropTable(&models.TorrentInfo{}))
	fixTimezoneDryRun = false
	err := runFixTimezone(&cobra.Command{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "查询种子信息失败")
}

// TestRunFixTimezone_ArchiveQueryErrorWarns drops only the archive table so the
// torrent pass succeeds but the archive Find fails, hitting the warn branch
// (which does not return an error).
func TestRunFixTimezone_ArchiveQueryErrorWarns(t *testing.T) {
	db := prepareRuntimeDB(t)
	ft := time.Date(2024, 8, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.DB.Create(&models.TorrentInfo{
		SiteName: "hdsky", TorrentID: "t1", Title: "x", FreeEndTime: &ft,
	}).Error)
	require.NoError(t, db.DB.Migrator().DropTable(&models.TorrentInfoArchive{}))

	fixTimezoneDryRun = false
	require.NoError(t, runFixTimezone(&cobra.Command{}, nil))
}
