package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/sunerpy/pt-tools/internal/remote/relayserver"
	"github.com/sunerpy/pt-tools/version"
)

// relay 的参数（也认 PT_TOOLS_RELAY_* 环境变量，Docker 里 PT_MODE=relay 时用）。
var (
	relayListen         string
	relayPublicURL      string
	relayMaxStreams     int
	relayDailyBytes     int64
	relayMaxConnPerIP   int
	relayDisabled       bool
	relayClientIPHeader string
	relayTLSCert        string
	relayTLSKey         string
)

var relayCmd = &cobra.Command{
	Use:   "relay",
	Short: "远程访问的 relay（自建）",
}

var relayServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "运行 relay：在 VPS 或 NAS 上给手机 App 与 pt-tools 转发加密连接",
	Long: `运行远程访问的 relay（只做转发，看不到内容）。不打开数据库，也不读写 ~/.pt-tools。

--public-url 是 relay 对外的地址（App 与 pt-tools 里填的就是它），主机按它签名，必须和实际访问的地址一致。
放在反向代理之后时，代理要转发 WebSocket；要按客户端 IP 限流，用 --client-ip-header 指定代理写的头。`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runRelay(cmd.Context())
	},
}

func init() {
	rootCmd.AddCommand(relayCmd)
	relayCmd.AddCommand(relayServeCmd)
	f := relayServeCmd.Flags()
	f.StringVar(&relayListen, "listen", envOr("PT_TOOLS_RELAY_LISTEN", "0.0.0.0:8443"), "监听地址")
	f.StringVar(&relayPublicURL, "public-url", os.Getenv("PT_TOOLS_RELAY_PUBLIC_URL"), "relay 对外的地址（ws:// 或 wss://），必填")
	f.IntVar(&relayMaxStreams, "max-streams-per-host", envInt("PT_TOOLS_RELAY_MAX_STREAMS_PER_HOST", relayserver.DefaultMaxStreamsPerHost), "每台主机同时打开的客户端流上限")
	f.Int64Var(&relayDailyBytes, "daily-bytes-per-host", int64(envInt("PT_TOOLS_RELAY_DAILY_BYTES_PER_HOST", 0)), "每台主机每天（00:00 UTC 重置）转发的字节上限，0 = 不限")
	f.IntVar(&relayMaxConnPerIP, "max-conn-per-ip-per-min", envInt("PT_TOOLS_RELAY_MAX_CONN_PER_IP_PER_MIN", relayserver.DefaultMaxConnPerIPPerMin), "每个 IP 每分钟新建连接的上限，负数 = 不限")
	f.BoolVar(&relayDisabled, "disabled", os.Getenv("PT_TOOLS_RELAY_DISABLED") == "true", "暂停服务：所有连接以 4503 关闭")
	f.StringVar(&relayClientIPHeader, "client-ip-header", os.Getenv("PT_TOOLS_RELAY_CLIENT_IP_HEADER"), "从这个请求头取客户端 IP（放在反向代理之后时，例如 X-Real-IP）")
	f.StringVar(&relayTLSCert, "tls-cert", os.Getenv("PT_TOOLS_RELAY_TLS_CERT"), "TLS 证书文件（不填时是明文，交给反向代理做 TLS）")
	f.StringVar(&relayTLSKey, "tls-key", os.Getenv("PT_TOOLS_RELAY_TLS_KEY"), "TLS 私钥文件")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	var n int
	if v := os.Getenv(key); v != "" {
		if _, err := fmt.Sscan(v, &n); err == nil {
			return n
		}
	}
	return def
}

func runRelay(ctx context.Context) error {
	if relayPublicURL == "" {
		return errors.New("要用 --public-url（或 PT_TOOLS_RELAY_PUBLIC_URL）给出 relay 对外的地址，例如 wss://relay.example.com")
	}
	if (relayTLSCert == "") != (relayTLSKey == "") {
		return errors.New("--tls-cert 与 --tls-key 要一起给")
	}
	logger, _ := zap.NewProduction()
	defer func() { _ = logger.Sync() }()
	srv, err := relayserver.New(relayserver.Config{
		PublicURL: relayPublicURL, MaxStreamsPerHost: relayMaxStreams, DailyBytesPerHost: relayDailyBytes,
		MaxConnPerIPPerMin: relayMaxConnPerIP, Disabled: relayDisabled, ClientIPHeader: relayClientIPHeader,
		Version: version.GetVersionInfo().Version, Logger: logger.Sugar(),
	})
	if err != nil {
		return err
	}
	hs := &http.Server{Addr: relayListen, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	errc := make(chan error, 1)
	go func() {
		fmt.Fprintf(os.Stderr, "relay 监听 %s，对外地址 %s\n", relayListen, relayPublicURL)
		if relayTLSCert != "" {
			errc <- hs.ListenAndServeTLS(relayTLSCert, relayTLSKey)
		} else {
			errc <- hs.ListenAndServe()
		}
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	}
	srv.Close()
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return hs.Shutdown(sctx)
}
