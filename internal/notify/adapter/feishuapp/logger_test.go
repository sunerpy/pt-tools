package feishuapp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// SDK 在 Debug 级打印每条入站消息的全文（/addrss 会带 passkey），一律丢掉；其余级别脱敏后转给 zap。
func TestSDKLogger(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	l := sdkLogger{log: func() *zap.SugaredLogger { return zap.New(core).Sugar() }, red: redactor{secret: "app-secret-x"}}
	ctx := context.Background()

	l.Debug(ctx, "receive message, payload: ", "passkey=abc")
	require.Equal(t, 0, logs.Len(), "Debug 一律丢掉")

	l.Info(ctx, []any{"connected to wss://open.feishu.cn"}...)
	l.Info(ctx, []any{"websocket reconnecting: attempt 1"})
	l.Warn(ctx, "token", "t-g1044ghJRUIJJ5ELPZNLNRIHCAAXQ1")
	l.Error(ctx, "bad secret app-secret-x")

	entries := logs.All()
	require.Len(t, entries, 4)
	assert.Equal(t, zapcore.InfoLevel, entries[0].Level)
	assert.Equal(t, "飞书 SDK: connected to wss://open.feishu.cn", entries[0].Message)
	assert.Equal(t, "飞书 SDK: websocket reconnecting: attempt 1", entries[1].Message, "SDK 有时把参数包成一个切片")
	assert.Equal(t, zapcore.WarnLevel, entries[2].Level)
	assert.Equal(t, "飞书 SDK: token t-***", entries[2].Message)
	assert.Equal(t, zapcore.ErrorLevel, entries[3].Level)
	assert.Equal(t, "飞书 SDK: bad secret ***", entries[3].Message)
}

func TestSLoggerWithoutGlobal(t *testing.T) {
	assert.NotNil(t, sLogger())
}
