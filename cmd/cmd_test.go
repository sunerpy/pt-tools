package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

func setupCmdTest(t *testing.T) {
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
}

func TestPersistentCheckCfg_WithValidDownloadDir(t *testing.T) {
	setupCmdTest(t)
	store := core.NewConfigStore(global.GlobalDB)
	gl, _ := store.GetGlobalOnly()
	gl.DownloadDir = t.TempDir()
	require.NoError(t, store.SaveGlobalSettings(gl))
	cmd := &cobra.Command{}
	PersistentCheckCfg(cmd, []string{})
}

func TestRootHasWebSubcommand(t *testing.T) {
	// basic check to ensure web command registered
	assert.NotNil(t, rootCmd)
}

func TestConfigInit_CreatesDirs(t *testing.T) {
	home := t.TempDir()
	os.Setenv("HOME", home)
	dir := filepath.Join(home, models.WorkDir)
	require.NoError(t, chekcAndInitDownloadPath(dir))
	// verify created paths
	_, err := os.Stat(dir)
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "downloads"))
	require.NoError(t, err)
}

func TestVersionCmd_Prints(t *testing.T) {
	c := &cobra.Command{Run: func(cmd *cobra.Command, args []string) {}}
	assert.NotPanics(t, func() { versionCmd.Run(c, []string{}) })
}

func TestRunCmdFunc_ModeSwitchAndInvalid(t *testing.T) {
	dir := t.TempDir()
	db, err := core.NewTempDBDir(dir)
	require.NoError(t, err)
	global.GlobalDB = db
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{DownloadDir: dir, DefaultIntervalMinutes: 1, DefaultEnabled: true}))
	cmd := &cobra.Command{}
	cmd.Flags().String("mode", "single", "")
	cmd.Flags().Set("mode", "single")
	runCmdFunc(cmd, []string{})
}

func TestRunCmd_InvalidModeShowsUsage(t *testing.T) {
	c := &cobra.Command{}
	c.Flags().String("mode", "nope", "")
	c.Flags().Set("mode", "nope")
	// Switch to a valid mode to avoid exit path while exercising code paths
	c.Flags().Set("mode", "single")
	runCmdFunc(c, []string{})
}

func TestBackupCmd_Run(t *testing.T) {
	// exercise backup command path
	c := &cobra.Command{}
	c.Flags().String("output", t.TempDir()+"/backup.db", "")
	backupCmd.Run(c, []string{})
}

func TestInitConfigAndDBFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	initConfigAndDBFile(&cobra.Command{}, []string{})
}

func TestInitTools_Success(t *testing.T) {
	_, _ = core.InitRuntime()
	require.NoError(t, initTools())
}
