package cmd

import (
	"errors"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// TestRunCmdFunc_PersistentSignalExit drives the persistent branch of runCmdFunc.
// The test keeps its own SIGTERM subscription for the whole run so an early
// signal (before runCmdFunc's goroutine registers) can never trigger the default
// terminate; repeated SIGTERMs then reach runCmdFunc's handler, cancel its ctx,
// and genTorrentsWithRSS returns cleanly.
func TestRunCmdFunc_PersistentSignalExit(t *testing.T) {
	_, _ = core.InitRuntime()
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1,
	}))
	_ = os.Remove("/tmp/pt-tools.lock")

	guard := make(chan os.Signal, 4)
	signal.Notify(guard, syscall.SIGTERM)
	t.Cleanup(func() { signal.Stop(guard) })

	cmd := &cobra.Command{}
	cmd.Flags().String("mode", "persistent", "")
	require.NoError(t, cmd.Flags().Set("mode", "persistent"))

	done := make(chan struct{})
	go func() {
		captureStdout(t, func() { runCmdFunc(cmd, nil) })
		close(done)
	}()

	proc, err := os.FindProcess(os.Getpid())
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_ = proc.Signal(syscall.SIGTERM)
		select {
		case <-done:
			return true
		default:
			return false
		}
	}, 8*time.Second, 100*time.Millisecond, "persistent runCmdFunc should exit on SIGTERM")
}

// TestRunCmdFunc_SingleSuccess drives runCmdFunc through the full single-run
// success path: acquire the file lock, run genTorrentsWithRSSOnce (no enabled
// sites -> completes immediately), and print the success line. os.Exit is only
// reached on error, so the happy path returns normally.
func TestRunCmdFunc_SingleSuccess(t *testing.T) {
	_, _ = core.InitRuntime()
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	global.GlobalDB = db
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), DefaultIntervalMinutes: 1, DefaultEnabled: true,
	}))

	// Remove any stale lock so Lock() succeeds in this process.
	_ = os.Remove("/tmp/pt-tools.lock")

	cmd := &cobra.Command{}
	cmd.Flags().String("mode", "single", "")
	require.NoError(t, cmd.Flags().Set("mode", "single"))
	captureStdout(t, func() { runCmdFunc(cmd, nil) })
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
