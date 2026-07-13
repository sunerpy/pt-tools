package cmd

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/version"
)

// TestStartVersionChecker_RunsFirstCheck drives the initial checkVersion() call
// with outbound HTTP pointed at a blackhole so CheckForUpdates returns an error,
// exercising the error-log branch. The function then blocks on a 24h ticker, so
// it is run in a goroutine and left to be reclaimed at process exit.
func TestStartVersionChecker_RunsFirstCheck(t *testing.T) {
	global.InitLogger(zap.NewNop())
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:1")

	before := version.GetChecker()
	_ = before

	go startVersionChecker()
	// Allow the first checkVersion() to run and log its result before returning.
	time.Sleep(300 * time.Millisecond)
}

// TestStartVersionChecker_HasUpdateBranch seeds the checker cache (empty proxy,
// matching startVersionChecker's default opts) so its first checkVersion() hits
// the cache and logs the has-update branch without a re-fetch. It must NOT mutate
// the package-global version.Version: the checker goroutine is a leaked 24h-ticker
// loop, and writing/restoring that global would race its reads under -race.
func TestStartVersionChecker_HasUpdateBranch(t *testing.T) {
	global.InitLogger(zap.NewNop())

	c := version.GetChecker()
	res, err := c.CheckForUpdates(context.Background(), version.CheckOptions{Force: true, ProxyURL: ""})
	_ = err
	res.HasUpdate = true
	res.NewReleases = []version.ReleaseInfo{{Version: "v2.0.0", URL: "https://x/v2"}}

	go startVersionChecker()
	time.Sleep(200 * time.Millisecond)
}

var errShutdown = errors.New("shutdown boom")

// TestInstallShutdownHandler_ShutdownErrorsLogged wires a bootstrap whose channel
// Close fails, so installShutdownHandler's bs.Shutdown error-log branch runs on
// SIGTERM.
func TestInstallShutdownHandler_ShutdownErrorsLogged(t *testing.T) {
	global.InitLogger(zap.NewNop())
	bs := &chatopsBootstrap{channels: map[uint]notify.Channel{1: &closeRecordingChannel{closeErr: errShutdown}}}
	done := installShutdownHandler(nil, bs)

	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("find process: %v", err)
	}
	waitShutdownAfterSIGTERM(t, proc, done)
}

// TestInstallShutdownHandler_NilServerAndBootstrap verifies the handler installs
// its signal trap, then unwinds cleanly when both srv and bs are nil after a
// SIGTERM is delivered (both nil-guard branches taken).
func TestInstallShutdownHandler_NilServerAndBootstrap(t *testing.T) {
	global.InitLogger(zap.NewNop())
	done := installShutdownHandler(nil, nil)

	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("find process: %v", err)
	}
	waitShutdownAfterSIGTERM(t, proc, done)
}

// waitShutdownAfterSIGTERM 等待 installShutdownHandler 的 goroutine 先注册
// signal.Notify，再发送 SIGTERM。注册前 SIGTERM 的默认动作会直接终止进程
// （signal: terminated），故先给一小段时间让 Notify 生效；注册后 SIGTERM 被捕获，
// 此时周期性重发可覆盖首个信号仍偶发早到的情况，确保被处理而非丢失或杀进程。
func waitShutdownAfterSIGTERM(t *testing.T, proc *os.Process, done <-chan struct{}) {
	t.Helper()
	time.Sleep(200 * time.Millisecond)
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			_ = proc.Signal(syscall.SIGTERM)
		case <-deadline:
			t.Fatal("shutdown handler did not complete after SIGTERM")
		}
	}
}
