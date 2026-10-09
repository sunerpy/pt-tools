package cmd

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

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

// relay serve 起来以后 /healthz 能访问，ctx 结束以后退出
func TestRelayServeRuns(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	setRelayFlags(t, addr, "ws://"+addr, "", "")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runRelay(ctx) }()
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 2 * time.Second}
	require.Eventually(t, func() bool {
		resp, err := client.Get("http://" + addr + "/healthz")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 10*time.Second, 50*time.Millisecond)
	cancel()
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
	assert.NotNil(t, c.Flags().Lookup("public-url"))
	assert.NotNil(t, c.Flags().Lookup("daily-bytes-per-host"))
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
	assert.True(t, envBool("PT_TOOLS_RELAY_B", &errs))
	t.Setenv("PT_TOOLS_RELAY_B", "yes")
	assert.False(t, envBool("PT_TOOLS_RELAY_B", &errs))
	assert.Len(t, errs, 2)
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
}
