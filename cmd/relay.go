package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
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
	relayMaxConns       int
	relayDrainGrace     time.Duration
	relayMetrics        bool
	relayClientIPHeader string
	relayTLSCert        string
	relayTLSKey         string
	// relayEnvErrs 是写错的 PT_TOOLS_RELAY_* 环境变量：注册参数时记下，运行时拒绝启动（不能悄悄退回默认值）
	relayEnvErrs []error
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
放在反向代理之后时，代理要转发 WebSocket；要按客户端 IP 限流，用 --client-ip-header 指定代理写的头。
健康检查用 GET /ready（不接新连接时 503），监控抓 GET /metrics（Prometheus 文本）。
收到 SIGTERM 时先不接新连接，等 --drain-grace，再以 1012 关掉所有连接（pt-tools 与 App 随后几秒内重连）。`,
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
	errs := &relayEnvErrs
	f.IntVar(&relayMaxStreams, "max-streams-per-host", int(envInt("PT_TOOLS_RELAY_MAX_STREAMS_PER_HOST", relayserver.DefaultMaxStreamsPerHost, errs)), "每台主机同时打开的客户端流上限")
	f.Int64Var(&relayDailyBytes, "daily-bytes-per-host", envInt("PT_TOOLS_RELAY_DAILY_BYTES_PER_HOST", 0, errs), "每台主机每天（00:00 UTC 重置）转发的字节上限，0 = 不限")
	f.IntVar(&relayMaxConnPerIP, "max-conn-per-ip-per-min", int(envInt("PT_TOOLS_RELAY_MAX_CONN_PER_IP_PER_MIN", relayserver.DefaultMaxConnPerIPPerMin, errs)), "每个 IP 每分钟新建连接的上限，负数 = 不限")
	f.BoolVar(&relayDisabled, "disabled", envBool("PT_TOOLS_RELAY_DISABLED", false, errs), "暂停服务：所有连接以 4503 关闭")
	f.IntVar(&relayMaxConns, "max-connections", int(envInt("PT_TOOLS_RELAY_MAX_CONNECTIONS", relayserver.DefaultMaxConnections, errs)), "同时连着的连接上限（主机加客户端），满了新连接回 503；负数 = 不限")
	f.DurationVar(&relayDrainGrace, "drain-grace", envDuration("PT_TOOLS_RELAY_DRAIN_GRACE", 0, errs), "收到 SIGTERM 以后先不接新连接、等这么久再关掉所有连接（前面有负载均衡按 /ready 摘流量时设几秒）")
	f.BoolVar(&relayMetrics, "metrics", envBool("PT_TOOLS_RELAY_METRICS", true, errs), "提供 GET /metrics（Prometheus 文本，只有聚合的计数）")
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

// envInt 读整数环境变量（64 位）；没有设时是 def，写错时记进 errs 并返回 def。
func envInt(key string, def int64, errs *[]error) int64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("环境变量 %s=%q 不是整数", key, v))
		return def
	}
	return n
}

// envBool 读布尔环境变量（true/false/1/0 等，大小写都认）；没有设时是 def，写错时记进 errs 并返回 def。
func envBool(key string, def bool, errs *[]error) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("环境变量 %s=%q 不是 true 或 false", key, v))
		return def
	}
	return b
}

// envDuration 读时长环境变量（5s、1m30s 这种写法）；没有设时是 def，写错时记进 errs 并返回 def。
func envDuration(key string, def time.Duration, errs *[]error) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("环境变量 %s=%q 不是时长（例如 5s）", key, v))
		return def
	}
	return d
}

// maxDrainGrace 是 --drain-grace 的上限：再长就该换成先摘流量再停的部署流程了
const maxDrainGrace = 5 * time.Minute

func runRelay(ctx context.Context) error {
	if len(relayEnvErrs) > 0 {
		return errors.Join(relayEnvErrs...)
	}
	switch {
	case relayMaxStreams < 0:
		return errors.New("--max-streams-per-host 不能是负数")
	case relayDailyBytes < 0:
		return errors.New("--daily-bytes-per-host 不能是负数（0 = 不限）")
	case relayDrainGrace < 0 || relayDrainGrace > maxDrainGrace:
		return fmt.Errorf("--drain-grace 要在 0 到 %s 之间", maxDrainGrace)
	}
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
		MaxConnections: relayMaxConns, NoMetrics: !relayMetrics,
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
	// 第二次 Ctrl-C / SIGTERM 照常直接结束进程
	stop()
	srv.Drain()
	if relayDrainGrace > 0 {
		fmt.Fprintf(os.Stderr, "relay 不再接新连接，%s 后关掉所有连接\n", relayDrainGrace)
		t := time.NewTimer(relayDrainGrace)
		select {
		case <-t.C:
		case err := <-errc:
			t.Stop()
			if !errors.Is(err, http.ErrServerClosed) {
				return err
			}
		}
	}
	srv.Close()
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return hs.Shutdown(sctx)
}
