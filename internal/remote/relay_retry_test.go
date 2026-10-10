package remote

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
)

// relay 以 1012 关（重启）：退避回到最短，在 1–5 秒里随机等；1013（太忙）：至少 10–20 秒；别的按退避（±20%）翻倍，最长一分钟
func TestRelayRetry(t *testing.T) {
	restart := fmt.Errorf("读 relay: %w", websocket.CloseError{Code: websocket.StatusServiceRestart, Reason: "relay restarting"})
	busy := fmt.Errorf("连接 relay 失败: %w", websocket.CloseError{Code: websocket.StatusTryAgainLater})
	other := errors.New("connection refused")

	wait, next := relayRetry(restart, 32*time.Second, 0)
	assert.Equal(t, time.Second, wait)
	assert.Equal(t, relayBackoffMin, next, "重启以后退避从头算")
	wait, _ = relayRetry(restart, 32*time.Second, 0.999)
	assert.Less(t, wait, 5*time.Second)
	assert.Greater(t, wait, 4*time.Second)

	wait, next = relayRetry(busy, time.Second, 0)
	assert.Equal(t, 10*time.Second, wait)
	assert.Equal(t, 2*time.Second, next)
	wait, _ = relayRetry(busy, time.Minute, 0.5)
	assert.Equal(t, time.Minute, wait, "退避已经比 10–20 秒长时按退避")

	wait, next = relayRetry(other, 4*time.Second, 0.5)
	assert.Equal(t, 4*time.Second, wait)
	assert.Equal(t, 8*time.Second, next)
	wait, _ = relayRetry(other, 4*time.Second, 0)
	assert.Equal(t, 3200*time.Millisecond, wait, "抖动 -20%")
	_, next = relayRetry(other, 50*time.Second, 0.5)
	assert.Equal(t, relayBackoffMax, next)
	_, next = relayRetry(nil, relayBackoffMin, 0.5)
	assert.Equal(t, 2*relayBackoffMin, next)
}
