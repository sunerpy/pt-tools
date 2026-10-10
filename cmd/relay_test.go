package cmd

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setRelayFlags 改 relay serve 的参数，测试结束时还原。
func setRelayFlags(t *testing.T, listen, publicURL, cert, key string) {
	t.Helper()
	old := []string{relayListen, relayPublicURL, relayTLSCert, relayTLSKey}
	relayListen, relayPublicURL, relayTLSCert, relayTLSKey = listen, publicURL, cert, key
	t.Cleanup(func() { relayListen, relayPublicURL, relayTLSCert, relayTLSKey = old[0], old[1], old[2], old[3] })
}

func TestRelayServeValidation(t *testing.T) {
	setRelayFlags(t, "127.0.0.1:0", "", "", "")
	assert.ErrorContains(t, runRelay(context.Background()), "--public-url")
	setRelayFlags(t, "127.0.0.1:0", "https://relay.example.com", "", "")
	assert.ErrorContains(t, runRelay(context.Background()), "PublicURL")
	setRelayFlags(t, "127.0.0.1:0", "wss://relay.example.com", "cert.pem", "")
	assert.ErrorContains(t, runRelay(context.Background()), "--tls-key")
}

// relay serve 起来以后 /healthz、/ready、/metrics 能访问；ctx 结束以后先排空（/ready 回 503 draining），
// 过了 --drain-grace 以 1012 关掉连着的主机，然后退出
func TestRelayServeRuns(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	setRelayFlags(t, addr, "ws://"+addr, "", "")
	oldGrace, oldMetrics := relayDrainGrace, relayMetrics
	t.Cleanup(func() { relayDrainGrace, relayMetrics = oldGrace, oldMetrics })
	relayDrainGrace, relayMetrics = 2*time.Second, true
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runRelay(ctx) }()
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 2 * time.Second}
	status := func(path string) int {
		resp, gerr := client.Get("http://" + addr + path)
		if gerr != nil {
			return 0
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	require.Eventually(t, func() bool { return status("/healthz") == http.StatusOK }, 10*time.Second, 50*time.Millisecond)
	assert.Equal(t, http.StatusOK, status("/ready"))
	assert.Equal(t, http.StatusOK, status("/metrics"))
	// 连着的主机（只连上、还没认证）：停的时候收到 1012
	dctx, dcancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer dcancel()
	ws, _, err := websocket.Dial(dctx, "ws://"+addr+"/v1/host/"+strings.Repeat("a", 26), &websocket.DialOptions{HTTPClient: client})
	require.NoError(t, err)
	defer ws.CloseNow()
	closed := make(chan websocket.StatusCode, 1)
	go func() {
		for {
			if _, _, err := ws.Read(dctx); err != nil {
				closed <- websocket.CloseStatus(err)
				return
			}
		}
	}()
	t0 := time.Now()
	cancel()
	require.Eventually(t, func() bool { return status("/ready") == http.StatusServiceUnavailable }, 2*time.Second, 20*time.Millisecond, "排空时 /ready 回 503")
	select {
	case code := <-closed:
		assert.Equal(t, websocket.StatusServiceRestart, code)
		assert.GreaterOrEqual(t, time.Since(t0), 2*time.Second, "等过了 --drain-grace 才关")
	case <-time.After(10 * time.Second):
		t.Fatal("没有收到 1012")
	}
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("ctx 结束以后 relay 没有退出")
	}
}

func TestRelayCommandRegistered(t *testing.T) {
	c, _, err := rootCmd.Find([]string{"relay", "serve"})
	require.NoError(t, err)
	assert.Equal(t, "serve", c.Name())
	for _, name := range []string{"public-url", "daily-bytes-per-host", "max-connections", "drain-grace", "metrics"} {
		assert.NotNil(t, c.Flags().Lookup(name), name)
	}
	assert.Equal(t, "true", c.Flags().Lookup("metrics").DefValue, "默认提供 /metrics")
}

func TestRelayEnvDefaults(t *testing.T) {
	var errs []error
	t.Setenv("PT_TOOLS_RELAY_X", "42")
	assert.Equal(t, int64(42), envInt("PT_TOOLS_RELAY_X", 7, &errs))
	t.Setenv("PT_TOOLS_RELAY_X", "3221225472")
	assert.Equal(t, int64(3221225472), envInt("PT_TOOLS_RELAY_X", 7, &errs), "GiB 级的值按 64 位解析")
	assert.Empty(t, errs)
	t.Setenv("PT_TOOLS_RELAY_X", "abc")
	assert.Equal(t, int64(7), envInt("PT_TOOLS_RELAY_X", 7, &errs))
	require.Len(t, errs, 1, "写错的值要记下来，运行时报错")
	assert.ErrorContains(t, errs[0], "PT_TOOLS_RELAY_X")
	t.Setenv("PT_TOOLS_RELAY_B", "TRUE")
	assert.True(t, envBool("PT_TOOLS_RELAY_B", false, &errs))
	t.Setenv("PT_TOOLS_RELAY_B", "yes")
	assert.True(t, envBool("PT_TOOLS_RELAY_B", true, &errs), "写错时返回默认值")
	assert.Len(t, errs, 2)
	assert.True(t, envBool("PT_TOOLS_RELAY_NOT_SET", true, &errs), "没有设时是默认值")
	t.Setenv("PT_TOOLS_RELAY_D", "1m30s")
	assert.Equal(t, 90*time.Second, envDuration("PT_TOOLS_RELAY_D", 0, &errs))
	assert.Equal(t, 5*time.Second, envDuration("PT_TOOLS_RELAY_NOT_SET", 5*time.Second, &errs))
	t.Setenv("PT_TOOLS_RELAY_D", "5")
	assert.Equal(t, time.Duration(0), envDuration("PT_TOOLS_RELAY_D", 0, &errs), "没有单位不认")
	require.Len(t, errs, 3)
	assert.ErrorContains(t, errs[2], "PT_TOOLS_RELAY_D")
	assert.Equal(t, "d", envOr("PT_TOOLS_RELAY_NOT_SET", "d"))
}

// 环境变量写错、参数是负数时不启动
func TestRelayServeRejectsBadConfig(t *testing.T) {
	setRelayFlags(t, "127.0.0.1:0", "ws://127.0.0.1:1", "", "")
	old := relayEnvErrs
	t.Cleanup(func() { relayEnvErrs = old })
	relayEnvErrs = []error{errors.New("PT_TOOLS_RELAY_DAILY_BYTES_PER_HOST=\"2G\" 不是整数")}
	assert.ErrorContains(t, runRelay(context.Background()), "PT_TOOLS_RELAY_DAILY_BYTES_PER_HOST")
	relayEnvErrs = nil
	oldDaily := relayDailyBytes
	t.Cleanup(func() { relayDailyBytes = oldDaily })
	relayDailyBytes = -1
	assert.ErrorContains(t, runRelay(context.Background()), "--daily-bytes-per-host")
	relayDailyBytes = 0
	oldGrace := relayDrainGrace
	t.Cleanup(func() { relayDrainGrace = oldGrace })
	for _, d := range []time.Duration{-time.Second, 6 * time.Minute} {
		relayDrainGrace = d
		assert.ErrorContains(t, runRelay(context.Background()), "--drain-grace", d)
	}
}
