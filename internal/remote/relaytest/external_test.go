package relaytest

import (
	"os"
	"strconv"
	"testing"
)

// TestExternalRelay 对着一个已经起好的 relay 跑一致性测试（relay.yml 里对 wrangler dev 跑）：
//
//	RELAY_CONFORMANCE_URL=ws://127.0.0.1:8787 RELAY_CONFORMANCE_PROFILE=main RELAY_CONFORMANCE_MAX_STREAMS=4 \
//	  go test ./internal/remote/relaytest -run TestExternalRelay -count=1
//
// 没给地址时跳过（平时的 go test 不连外部服务）。
func TestExternalRelay(t *testing.T) {
	u := os.Getenv("RELAY_CONFORMANCE_URL")
	if u == "" {
		t.Skip("没有给 RELAY_CONFORMANCE_URL")
	}
	envInt := func(k string) int {
		n, _ := strconv.Atoi(os.Getenv(k))
		return n
	}
	Run(t, Target{
		URL:                u,
		Origin:             os.Getenv("RELAY_CONFORMANCE_ORIGIN"),
		Profile:            Profile(os.Getenv("RELAY_CONFORMANCE_PROFILE")),
		MaxStreamsPerHost:  envInt("RELAY_CONFORMANCE_MAX_STREAMS"),
		MaxConnPerIPPerMin: envInt("RELAY_CONFORMANCE_MAX_CONN_PER_IP"),
		DailyBytesPerHost:  int64(envInt("RELAY_CONFORMANCE_DAILY_BYTES")),
	})
}

// TestExternalRelayInterop 用真正的主机端经外部 relay 走一遍配对与会话（RELAY_CONFORMANCE_INTEROP=1 时跑，要求 relay 每 IP 不限或足够大）。
func TestExternalRelayInterop(t *testing.T) {
	u := os.Getenv("RELAY_CONFORMANCE_URL")
	if u == "" || os.Getenv("RELAY_CONFORMANCE_INTEROP") != "1" {
		t.Skip("没有给 RELAY_CONFORMANCE_URL 与 RELAY_CONFORMANCE_INTEROP=1")
	}
	RunHostInterop(t, u)
}
