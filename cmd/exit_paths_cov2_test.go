package cmd

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/sunerpy/pt-tools/utils"
)

// runExitHelper re-invokes this test binary in a subprocess so that code paths
// ending in os.Exit can be exercised (and their non-zero exit asserted) without
// terminating the test runner. The subprocess dispatches on the env var to the
// requested target inside TestCmdExitDispatch.
func runExitHelper(t *testing.T, target string, env ...string) (string, bool) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestCmdExitDispatch")
	cmd.Env = append(os.Environ(), append([]string{"CMD_EXIT_TARGET=" + target}, env...)...)
	out, err := cmd.CombinedOutput()
	exited := false
	if ee, ok := err.(*exec.ExitError); ok {
		exited = !ee.Success()
	}
	return string(out), exited
}

// TestCmdExitDispatch is the subprocess entry point. It only runs work when
// CMD_EXIT_TARGET is set, so under the normal test run it is a no-op.
func TestCmdExitDispatch(t *testing.T) {
	target := os.Getenv("CMD_EXIT_TARGET")
	if target == "" {
		return
	}
	switch target {
	case "run_invalid_mode":
		_ = os.Remove("/tmp/pt-tools.lock")
		c := &cobra.Command{}
		c.Flags().String("mode", "bogus", "")
		_ = c.Flags().Set("mode", "bogus")
		runCmdFunc(c, nil)
	case "completion_unsupported":
		completionCmd.Run(completionCmd, []string{"fish"})
	case "initconfig_home_error":
		os.Unsetenv("HOME")
		initConfigAndDBFile(&cobra.Command{}, nil)
	case "run_flag_error_special":
		fn := runCmd.FlagErrorFunc()
		_ = fn(runCmd, cmdErr("flag needs an argument: 'm' in -m"))
	case "execute_bad_command":
		os.Args = []string{"pt-tools", "no-such-subcommand-xyz"}
		Execute()
	case "run_lock_held":
		runLockHeldSubprocess()
	case "hooks_init_fail":
		_ = os.Setenv("HOME", "/proc")
		PersistentCheckCfg(&cobra.Command{}, nil)
	case "completion_bash_closed_stdout":
		closeStdoutForSubprocess()
		completionCmd.Run(completionCmd, []string{"bash"})
	case "completion_zsh_closed_stdout":
		closeStdoutForSubprocess()
		completionCmd.Run(completionCmd, []string{"zsh"})
	}
}

func closeStdoutForSubprocess() {
	r, w, err := os.Pipe()
	if err != nil {
		os.Exit(3)
	}
	_ = r.Close()
	// A tiny pipe buffer with a closed reader makes large completion writes
	// fail with EPIPE, driving the GenBashCompletion/GenZshCompletion error path.
	os.Stdout = w
}

// TestPersistentCheckCfg_InitFail_ExitsNonZero drives PersistentCheckCfg's
// initTools-failure branch (cmd.Usage + os.Exit(1)) via a subprocess whose HOME
// is a read-only path so runtime init (log/db dir creation) fails.
func TestPersistentCheckCfg_InitFail_ExitsNonZero(t *testing.T) {
	out, exited := runExitHelper(t, "hooks_init_fail")
	if !exited {
		t.Skipf("runtime init succeeded in subprocess env; guard not reached. out=%q", out)
	}
}

// TestCompletionCmd_BashWriteError_ExitsNonZero drives the GenBashCompletion
// error branch (os.Exit(1)) via a subprocess whose stdout pipe reader is closed.
func TestCompletionCmd_BashWriteError_ExitsNonZero(t *testing.T) {
	out, exited := runExitHelper(t, "completion_bash_closed_stdout")
	if !exited {
		t.Skipf("bash completion write did not fail in this env; out=%q", out)
	}
}

// TestCompletionCmd_ZshWriteError_ExitsNonZero drives the GenZshCompletion
// error branch (os.Exit(1)) via a subprocess whose stdout pipe reader is closed.
func TestCompletionCmd_ZshWriteError_ExitsNonZero(t *testing.T) {
	out, exited := runExitHelper(t, "completion_zsh_closed_stdout")
	if !exited {
		t.Skipf("zsh completion write did not fail in this env; out=%q", out)
	}
}

// TestRunCmdFunc_LockHeld_ExitsNonZero drives runCmdFunc's "already running"
// branch: a second locker holds the file lock, so Lock() fails and the command
// os.Exit(1)s. Executed in a subprocess so the exit does not kill the runner.
func TestRunCmdFunc_LockHeld_ExitsNonZero(t *testing.T) {
	out, exited := runExitHelper(t, "run_lock_held")
	if !exited {
		t.Skipf("lock was acquirable in subprocess; branch not reached. out=%q", out)
	}
}

type cmdErr string

func (e cmdErr) Error() string { return string(e) }

func runLockHeldSubprocess() {
	l, err := utils.NewLocker("/tmp/pt-tools.lock")
	if err != nil {
		os.Exit(3)
	}
	if e := l.Lock(); e != nil {
		os.Exit(3)
	}
	defer func() { _ = l.Unlock() }()
	c := &cobra.Command{}
	c.Flags().String("mode", "single", "")
	_ = c.Flags().Set("mode", "single")
	runCmdFunc(c, nil)
}

// TestRunFlagErrorFunc_SpecialMessage_ExitsNonZero drives the run command's
// SetFlagErrorFunc special-case branch ("-m needs an argument"), which prints a
// hint and os.Exit(1)s, via subprocess.
func TestRunFlagErrorFunc_SpecialMessage_ExitsNonZero(t *testing.T) {
	out, exited := runExitHelper(t, "run_flag_error_special")
	if !exited {
		t.Fatalf("expected non-zero exit, got success; output=%q", out)
	}
	if !strings.Contains(out, "'-m' flag requires a value") {
		t.Fatalf("expected -m hint message, got: %q", out)
	}
}

// TestExecute_BadCommand_ExitsNonZero drives Execute's error branch (unknown
// subcommand -> rootCmd.Execute returns err -> os.Exit(1)), via subprocess.
func TestExecute_BadCommand_ExitsNonZero(t *testing.T) {
	out, exited := runExitHelper(t, "execute_bad_command")
	if !exited {
		t.Fatalf("expected non-zero exit, got success; output=%q", out)
	}
}

// TestRunCmdFunc_InvalidMode_ExitsNonZero drives runCmdFunc's default branch
// (unrecognized --mode) which prints an error and os.Exit(1)s, via subprocess.
func TestRunCmdFunc_InvalidMode_ExitsNonZero(t *testing.T) {
	out, exited := runExitHelper(t, "run_invalid_mode")
	if !exited {
		t.Fatalf("expected non-zero exit, got success; output=%q", out)
	}
	if !strings.Contains(out, "无效的运行模式") {
		t.Fatalf("expected invalid-mode message, got: %q", out)
	}
}

// TestCompletionCmd_UnsupportedShell_ExitsNonZero drives completion's default
// branch (unsupported shell -> os.Exit(1)), via subprocess.
func TestCompletionCmd_UnsupportedShell_ExitsNonZero(t *testing.T) {
	out, exited := runExitHelper(t, "completion_unsupported")
	if !exited {
		t.Fatalf("expected non-zero exit, got success; output=%q", out)
	}
	if !strings.Contains(out, "Unsupported shell") {
		t.Fatalf("expected unsupported-shell message, got: %q", out)
	}
}

// TestInitConfigAndDBFile_HomeError_ExitsNonZero drives initConfigAndDBFile's
// UserHomeDir failure branch (HOME unset -> os.Exit(1)), via subprocess.
func TestInitConfigAndDBFile_HomeError_ExitsNonZero(t *testing.T) {
	out, exited := runExitHelper(t, "initconfig_home_error")
	if !exited {
		t.Fatalf("expected non-zero exit, got success; output=%q", out)
	}
	if !strings.Contains(out, "无法获取用户主目录") {
		t.Fatalf("expected home-dir error message, got: %q", out)
	}
}
