package relaytest

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/remote/relayserver"
)

// 这套用例对 Go 版 relay（relayserver，参考实现）跑一遍：既测用例本身，覆盖率也算在这个包上
// （relayserver 的测试里跑同一套，那边的覆盖率算在 relayserver 上）。
func startRelay(t *testing.T, cfg relayserver.Config) string {
	t.Helper()
	ts := httptest.NewUnstartedServer(nil)
	u := "ws://" + ts.Listener.Addr().String()
	cfg.PublicURL = u
	cfg.Version = "selftest"
	srv, err := relayserver.New(cfg)
	require.NoError(t, err)
	ts.Config.Handler = srv.Handler()
	ts.Start()
	t.Cleanup(func() {
		srv.Close()
		ts.Close()
	})
	return u
}

func TestSelfMain(t *testing.T) {
	u := startRelay(t, relayserver.Config{MaxStreamsPerHost: 4, MaxConnPerIPPerMin: -1})
	Run(t, Target{URL: u, Profile: ProfileMain, MaxStreamsPerHost: 4})
}

func TestSelfLimits(t *testing.T) {
	u := startRelay(t, relayserver.Config{MaxStreamsPerHost: 16, MaxConnPerIPPerMin: 8, DailyBytesPerHost: 200_000})
	Run(t, Target{URL: u, Profile: ProfileLimits, MaxConnPerIPPerMin: 8, DailyBytesPerHost: 200_000})
}

func TestSelfDisabled(t *testing.T) {
	u := startRelay(t, relayserver.Config{Disabled: true})
	Run(t, Target{URL: u, Profile: ProfileDisabled})
}

func TestSelfInterop(t *testing.T) {
	RunHostInterop(t, startRelay(t, relayserver.Config{MaxConnPerIPPerMin: -1}))
}
