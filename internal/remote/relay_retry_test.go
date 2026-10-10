package remote

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// relay 在升级前回 503：至少等它给的 Retry-After（加最多一半的抖动）；没有 Retry-After 时照常退避
func TestRelayRetryAfter(t *testing.T) {
	busy := &relayBusyError{retryAfter: 5 * time.Second, err: errors.New("503")}
	wait, next := relayRetry(busy, time.Second, 0)
	assert.Equal(t, 5*time.Second, wait)
	assert.Equal(t, 2*time.Second, next)
	wait, _ = relayRetry(busy, time.Second, 0.999)
	assert.Greater(t, wait, 7*time.Second)
	assert.Less(t, wait, 7500*time.Millisecond)
	wait, _ = relayRetry(busy, time.Minute, 0.5)
	assert.Equal(t, time.Minute, wait, "退避更长时按退避")
	wait, _ = relayRetry(&relayBusyError{err: errors.New("503")}, 2*time.Second, 0.5)
	assert.Equal(t, 2*time.Second, wait, "没有 Retry-After")

	assert.Equal(t, 5*time.Second, parseRetryAfter(" 5 "))
	assert.Equal(t, maxRetryAfter, parseRetryAfter("86400"))
	assert.Equal(t, time.Duration(0), parseRetryAfter("Wed, 21 Oct 2015 07:28:00 GMT"))
	assert.Equal(t, time.Duration(0), parseRetryAfter("-3"))
	assert.Equal(t, "503", busy.Error())
	assert.ErrorIs(t, busy, busy.err)
}

// relay 在升级前回 503 与 Retry-After：状态里写明 503，按 Retry-After 等，不会一秒后就重试
func TestRelayDialBusy(t *testing.T) {
	var hits atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(ts.Close)
	th := newTestHost(t, Settings{Relays: []string{"ws://" + strings.TrimPrefix(ts.URL, "http://")}})
	require.Eventually(t, func() bool {
		ov, err := th.Overview(context.Background())
		return err == nil && len(ov.RelayStatus) == 1 && strings.Contains(ov.RelayStatus[0].Error, "503")
	}, 5*time.Second, 20*time.Millisecond)
	// 普通的退避一秒左右就会再连；这里看两秒，只能有最初那一次
	time.Sleep(2 * time.Second)
	assert.Equal(t, int32(1), hits.Load())
}
