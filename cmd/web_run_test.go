package cmd

import (
	"context"
	"net"
	"net/http"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/notify"
	telegramadapter "github.com/sunerpy/pt-tools/internal/notify/adapter/telegram"
	"github.com/sunerpy/pt-tools/models"
)

// freeTCPPort grabs an OS-assigned free port on loopback and releases it, so
// the web command can bind it moments later. A small TOCTOU window exists but
// is acceptable for a single serialized test.
func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	p := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())
	return p
}

// TestWebCmdRun_StartAndGracefulShutdown drives the real webCmd.Run closure end
// to end: it initializes runtime, wires ChatOps + login reminder + HTTP server,
// serves on a loopback port, then delivers SIGTERM so installShutdownHandler
// tears everything down and Serve returns. Version checking is pointed at a
// blackhole proxy so no real external network egress occurs.
func TestWebCmdRun_StartAndGracefulShutdown(t *testing.T) {
	if raceEnabled {
		t.Skip("skips under -race: web.Server.Serve/Shutdown has a pre-existing httpServer field race owned by the web package")
	}
	// Blackhole any outbound version-check HTTP so the background goroutine
	// fails fast instead of reaching GitHub.
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:1")
	t.Setenv("PT_TOOLS_SECRET_KEY", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")

	// Initialize the runtime singleton once, then swap in a fully-migrated
	// temp DB (incl. chatops + userinfo tables) so bootstrapChatOps succeeds
	// and the bs != nil wiring branch is exercised.
	_, _ = core.InitRuntime()
	db, err := core.NewTempDBDir(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, db.DB.AutoMigrate(
		&models.NotificationConf{},
		&models.ChannelBinding{},
		&models.ActionAudit{},
		&models.BotToken{},
		&models.NotificationOutbox{},
		&models.SiteLoginState{},
	))
	prevDB := global.GlobalDB
	global.GlobalDB = db
	t.Cleanup(func() { global.GlobalDB = prevDB })

	// Seed a global config with a download dir but AutoStart off, so the
	// startup reload path logs the "waiting for manual start" branch without
	// attempting real downloader connections.
	store := core.NewConfigStore(db)
	require.NoError(t, store.SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), AutoStart: false, DefaultIntervalMinutes: 10,
	}))

	// Enable a built-in site so webCmd.Run exercises the site-registration loop
	// (CreateSite + RegisterSite on UserInfoService/SearchOrchestrator).
	enabled := true
	_, err = store.UpsertSite("hdsky", models.SiteConfig{
		Enabled: &enabled, AuthMethod: "cookie", Cookie: "c_secure_uid=1; c_secure_pass=x",
	})
	require.NoError(t, err)

	// Register a fake channel type and an enabled conf so bootstrapChatOps
	// builds a live channel, driving the RSS callback-registration loop
	// (SetCallbackActionHandler) inside webCmd.Run.
	registerFakeCallbackChannelOnce()
	require.NoError(t, db.DB.Create(&models.NotificationConf{
		ChannelType: fakeCallbackChannelType, Name: "fake", Enabled: true,
	}).Error)

	// Enable an mtorrent site with no API key so CreateSite fails inside the
	// site-registration loop, covering the createErr continue branch.
	_, err = store.UpsertSite("mteam", models.SiteConfig{
		Enabled: &enabled, AuthMethod: "apikey", APIKey: "",
	})
	require.NoError(t, err)

	p := freeTCPPort(t)
	prevHost, prevPort := host, port
	host = "127.0.0.1"
	port = p
	t.Cleanup(func() { host, port = prevHost, prevPort })

	done := make(chan struct{})
	go func() {
		webCmd.Run(webCmd, []string{})
		close(done)
	}()

	// Wait until the HTTP server is accepting connections on /api/ping.
	base := "http://127.0.0.1:" + itoa(p)
	require.Eventually(t, func() bool {
		resp, gerr := http.Get(base + "/api/ping")
		if gerr != nil {
			return false
		}
		_ = resp.Body.Close()
		return true
	}, 10*time.Second, 50*time.Millisecond, "web server should start listening")

	// Deliver SIGTERM to trigger installShutdownHandler → srv.Shutdown → Serve
	// returns → Run unblocks. The signal is intercepted by signal.Notify, so it
	// does not kill the test process.
	proc, err := os.FindProcess(os.Getpid())
	require.NoError(t, err)
	require.NoError(t, proc.Signal(syscall.SIGTERM))

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("webCmd.Run did not return after SIGTERM")
	}
}

const fakeCallbackChannelType = "fakecb"

var fakeCallbackRegisterOnce sync.Once

// fakeCallbackChannel implements notify.Channel plus the SetCallbackActionHandler
// seam that webCmd.Run type-asserts on, so the callback-registration loop runs.
type fakeCallbackChannel struct{}

func (fakeCallbackChannel) Type() string { return fakeCallbackChannelType }
func (fakeCallbackChannel) Init(context.Context, *models.NotificationConf) error {
	return nil
}

func (fakeCallbackChannel) SupportsInbound() bool { return true }

func (fakeCallbackChannel) Send(context.Context, notify.Notification) error { return nil }
func (fakeCallbackChannel) OnInbound(notify.InboundHandler)                 {}
func (fakeCallbackChannel) Close(context.Context) error                     { return nil }

func (fakeCallbackChannel) Healthy() bool                                                  { return true }
func (fakeCallbackChannel) SetCallbackActionHandler(telegramadapter.CallbackActionHandler) {}

func registerFakeCallbackChannelOnce() {
	fakeCallbackRegisterOnce.Do(func() {
		notify.DefaultRegistry().Register(fakeCallbackChannelType, func() notify.Channel {
			return fakeCallbackChannel{}
		})
	})
}

// itoa is a tiny int->string helper avoiding an extra import in the test.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

var _ = context.Background
