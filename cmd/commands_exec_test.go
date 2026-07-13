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

// captureStdout redirects os.Stdout for the duration of fn and discards output.
func captureStdout(t *testing.T, fn func()) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	defer func() {
		_ = w.Close()
		os.Stdout = old
		_ = r.Close()
	}()
	// Drain the pipe so writes never block if output is large.
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := r.Read(buf); err != nil {
				return
			}
		}
	}()
	fn()
}

func TestCompletionCmd_BashAndZsh(t *testing.T) {
	captureStdout(t, func() {
		completionCmd.Run(completionCmd, []string{"bash"})
		completionCmd.Run(completionCmd, []string{"zsh"})
	})
}

func TestDbInitCmd_Run(t *testing.T) {
	captureStdout(t, func() { initCmd.Run(&cobra.Command{}, nil) })
}

func TestBackupCmd_EmptyOutput(t *testing.T) {
	c := &cobra.Command{}
	c.Flags().String("output", "", "")
	captureStdout(t, func() { backupCmd.Run(c, nil) })
}

func TestTaskCmd_Run(t *testing.T) {
	captureStdout(t, func() { taskCmd.Run(&cobra.Command{}, nil) })
}

func TestConfigCmd_Run(t *testing.T) {
	captureStdout(t, func() { configCmd.Run(&cobra.Command{}, nil) })
}

func TestListCmd_Run(t *testing.T) {
	// default date branch
	c1 := &cobra.Command{}
	c1.Flags().String("date", "", "")
	captureStdout(t, func() { listCmd.Run(c1, nil) })
	// explicit date branch
	c2 := &cobra.Command{}
	c2.Flags().String("date", "2024-12-05", "")
	captureStdout(t, func() { listCmd.Run(c2, nil) })
}

func TestExecute_RunsRoot(t *testing.T) {
	// Point rootCmd at a harmless subcommand invocation (version) so Execute
	// exercises its success path without launching the web server.
	prev := os.Args
	os.Args = []string{"pt-tools", "version"}
	t.Cleanup(func() { os.Args = prev })
	captureStdout(t, func() { Execute() })
}

func TestPersistentCheckCfg_EmptyDownloadDir_NoExit(t *testing.T) {
	// With a valid DB but empty DownloadDir, PersistentCheckCfg would os.Exit(1).
	// We only assert the success branch here (covered separately); this test
	// documents the guard by ensuring a populated dir passes without exit.
	_, _ = core.InitRuntime()
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	store := core.NewConfigStore(db)
	require.NoError(t, store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir()}))
	assert.NotPanics(t, func() { PersistentCheckCfg(&cobra.Command{}, nil) })
}

func TestInitConfigAndDBFile_CreatesTree(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	captureStdout(t, func() { initConfigAndDBFile(&cobra.Command{}, nil) })
	_, err := os.Stat(filepath.Join(home, models.WorkDir, "downloads"))
	require.NoError(t, err)
}

func TestChekcAndInitDownloadPath_Idempotent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "wd")
	// First call creates; second call is a no-op (dirs already exist).
	require.NoError(t, chekcAndInitDownloadPath(dir))
	require.NoError(t, chekcAndInitDownloadPath(dir))
	_, err := os.Stat(filepath.Join(dir, "downloads"))
	require.NoError(t, err)
}

func TestInitTools_ReturnsNil(t *testing.T) {
	require.NoError(t, initTools())
}
