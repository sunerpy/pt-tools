package cmd

import (
	"errors"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

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

// TestRunCmd_FlagErrorFunc_NonMatchingReturnsErr drives the run command's
// SetFlagErrorFunc closure with an error that does NOT match the special
// "-m needs argument" string, so it takes the plain return-err branch.
func TestRunCmd_FlagErrorFunc_NonMatchingReturnsErr(t *testing.T) {
	fn := runCmd.Flags().ShorthandLookup("m")
	require.NotNil(t, fn, "run command must define -m/--mode flag")

	sentinel := errors.New("some other flag error")
	// runCmd is the *cobra.Command that owns the FlagErrorFunc.
	got := runCmd.FlagErrorFunc()(runCmd, sentinel)
	assert.Equal(t, sentinel, got, "non-matching flag error must be returned unchanged")
}
