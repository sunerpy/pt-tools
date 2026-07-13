package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

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
