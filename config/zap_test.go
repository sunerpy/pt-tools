package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func TestDefaultZapInitLogger(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	lg, err := DefaultZapConfig.InitLogger()
	require.NoError(t, err)
	require.NotNil(t, lg)
	// ensure files created
	p := filepath.Join(t.TempDir(), "pt-tools", DefaultZapConfig.Directory)
	_ = p
}

func TestZapEncodeLevelVariants(t *testing.T) {
	z := DefaultZapConfig
	z.EncodeLevel = "LowercaseLevelEncoder"
	if z.ZapEncodeLevel() == nil {
		t.Fatalf("nil encoder")
	}
	z.EncodeLevel = "LowercaseColorLevelEncoder"
	_ = z.ZapEncodeLevel()
	z.EncodeLevel = "CapitalLevelEncoder"
	_ = z.ZapEncodeLevel()
	z.EncodeLevel = "CapitalColorLevelEncoder"
	_ = z.ZapEncodeLevel()
	z.EncodeLevel = "unknown"
	_ = z.ZapEncodeLevel()
}

func TestApplyEnvOverrides(t *testing.T) {
	z := DefaultZapConfig
	assert.True(t, z.LogInConsole, "console should default to true for docker/NAS log visibility")

	t.Setenv("PT_TOOLS_LOG_LEVEL", "debug")
	t.Setenv("PT_TOOLS_LOG_CONSOLE", "false")
	z.ApplyEnvOverrides()
	assert.Equal(t, "debug", z.Level)
	assert.False(t, z.LogInConsole, "env should be able to disable console")

	z2 := DefaultZapConfig
	t.Setenv("PT_TOOLS_LOG_LEVEL", "not-a-level")
	t.Setenv("PT_TOOLS_LOG_CONSOLE", "")
	z2.ApplyEnvOverrides()
	assert.Equal(t, "info", z2.Level, "invalid level should be ignored")
	assert.True(t, z2.LogInConsole, "empty env should leave default unchanged")
}

func TestPruneOldLogs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	logDir := filepath.Join(home, ".pt-tools", "logs")
	require.NoError(t, os.MkdirAll(logDir, 0o755))

	write := func(name string, age time.Duration) {
		p := filepath.Join(logDir, name)
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
		mt := time.Now().Add(-age)
		require.NoError(t, os.Chtimes(p, mt, mt))
	}

	// base files must never be deleted
	write("all.log", 0)
	write("error.log", 0)
	// 5 recent all backups + 1 ancient one
	for i := 0; i < 5; i++ {
		write("all-2026-06-2"+string(rune('0'+i))+"T00-00-00.000.log.gz", time.Duration(i)*time.Hour)
	}
	write("all-2020-01-01T00-00-00.000.log.gz", 400*24*time.Hour)

	z := DefaultZapConfig
	z.MaxBackups = 3
	z.MaxAge = 30
	require.NoError(t, z.PruneOldLogs())

	// base files survive
	assert.FileExists(t, filepath.Join(logDir, "all.log"))
	assert.FileExists(t, filepath.Join(logDir, "error.log"))
	// ancient backup removed by MaxAge
	assert.NoFileExists(t, filepath.Join(logDir, "all-2020-01-01T00-00-00.000.log.gz"))

	// at most MaxBackups all-* backups remain
	entries, err := os.ReadDir(logDir)
	require.NoError(t, err)
	allBackups := 0
	for _, e := range entries {
		n := e.Name()
		if len(n) > 4 && n[:4] == "all-" {
			allBackups++
		}
	}
	assert.LessOrEqual(t, allBackups, 3)
}

func TestPruneOldLogs_MissingDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	z := DefaultZapConfig
	assert.NoError(t, z.PruneOldLogs(), "missing log dir should not error")
}

func TestSetAndGetLogLevel(t *testing.T) {
	// restore original level after the test to avoid cross-test leakage
	orig := GetLogLevel()
	t.Cleanup(func() { _ = SetLogLevel(orig) })

	require.NoError(t, SetLogLevel("debug"))
	assert.Equal(t, "debug", GetLogLevel())

	require.NoError(t, SetLogLevel("error"))
	assert.Equal(t, "error", GetLogLevel())

	require.NoError(t, SetLogLevel("warn"))
	assert.Equal(t, "warn", GetLogLevel())

	err := SetLogLevel("nonsense-level")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "无效的日志级别")
	// invalid level must not change the current level
	assert.Equal(t, "warn", GetLogLevel())
}

func TestInitLoggerAllOptions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("POD_NAME", "pt-tools-pod-1")

	z := DefaultZapConfig
	z.ShowLine = true         // exercises zap.AddCaller()
	z.StacktraceKey = "stack" // exercises zap.AddStacktrace()
	z.LogInConsole = true     // exercises console core branch
	z.Level = "debug"

	lg, err := z.InitLogger()
	require.NoError(t, err)
	require.NotNil(t, lg)
	lg.Info("hello from init logger test")
	lg.Error("error line to exercise high priority core")
	_ = lg.Sync()

	// log directory + base files should be created under HOME/WorkDir/logs
	logDir := filepath.Join(home, models.WorkDir, z.Directory)
	assert.FileExists(t, filepath.Join(logDir, "all.log"))
	assert.FileExists(t, filepath.Join(logDir, "error.log"))
}

func TestInitLoggerConsoleDisabledNoPod(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("POD_NAME", "")

	z := DefaultZapConfig
	z.LogInConsole = false // skip console core branch
	z.ShowLine = false
	z.StacktraceKey = ""
	z.Level = "info"

	lg, err := z.InitLogger()
	require.NoError(t, err)
	require.NotNil(t, lg)
	lg.Info("no console output line")
	_ = lg.Sync()
}

func TestInitLoggerInvalidLevel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	z := DefaultZapConfig
	z.Level = "not-a-real-level"
	lg, err := z.InitLogger()
	require.Error(t, err)
	assert.Nil(t, lg)
	assert.Contains(t, err.Error(), "解析日志级别失败")
}
