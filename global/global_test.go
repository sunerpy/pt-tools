package global

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestInitAndGetLogger(t *testing.T) {
	InitLogger(zap.NewNop())
	require.NotNil(t, GetLogger())
	require.NotNil(t, GetSlogger())
}

func TestGetSloggerSafe(t *testing.T) {
	// After InitLogger has run, GlobalLogger is non-nil.
	InitLogger(zap.NewNop())
	require.NotNil(t, GetSloggerSafe())
}

func TestLogLevelGetSet(t *testing.T) {
	InitLogger(zap.NewNop())

	SetLogLevel(LogLevelDebug)
	assert.Equal(t, LogLevelDebug, GetLogLevel())
	assert.True(t, IsDebugEnabled())

	SetLogLevel(LogLevelInfo)
	assert.Equal(t, LogLevelInfo, GetLogLevel())
	assert.False(t, IsDebugEnabled())

	SetLogLevel(LogLevelWarn)
	assert.Equal(t, LogLevelWarn, GetLogLevel())

	SetLogLevel(LogLevelError)
	assert.Equal(t, LogLevelError, GetLogLevel())
}

func TestParseLogLevel(t *testing.T) {
	cases := []struct {
		in    string
		want  LogLevel
		valid bool
	}{
		{"debug", LogLevelDebug, true},
		{"info", LogLevelInfo, true},
		{"warn", LogLevelWarn, true},
		{"error", LogLevelError, true},
		{"", LogLevelInfo, false},
		{"nonsense", LogLevelInfo, false},
	}
	for _, c := range cases {
		got, ok := ParseLogLevel(c.in)
		assert.Equal(t, c.valid, ok, "in=%q", c.in)
		assert.Equal(t, c.want, got, "in=%q", c.in)
	}
}

func TestGetSloggerSafeNilLogger(t *testing.T) {
	saved := GlobalLogger
	GlobalLogger = nil
	t.Cleanup(func() { GlobalLogger = saved })
	assert.Nil(t, GetSloggerSafe())
}

func TestToZapLevel(t *testing.T) {
	cases := []struct {
		lvl  LogLevel
		want zapcore.Level
	}{
		{LogLevelDebug, zapcore.DebugLevel},
		{LogLevelInfo, zapcore.InfoLevel},
		{LogLevelWarn, zapcore.WarnLevel},
		{LogLevelError, zapcore.ErrorLevel},
		{LogLevel("unknown"), zapcore.InfoLevel},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, c.lvl.ToZapLevel(), "lvl=%q", c.lvl)
	}
}
