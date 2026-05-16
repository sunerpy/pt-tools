package telegram

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/mymmrac/telego"
)

const defaultPollingTimeoutSeconds = 30

func defaultBotFactory(cfg *Config) (botAPI, updateSource, error) {
	opts := []telego.BotOption{telego.WithDiscardLogger()}
	if cfg.APIServer != "" {
		opts = append(opts, telego.WithAPIServer(cfg.APIServer))
	}
	// Inject net/http client. If cfg.ProxyURL is set, use it explicitly;
	// otherwise fall back to HTTPS_PROXY/HTTP_PROXY env vars via
	// http.ProxyFromEnvironment. telego's default fasthttp client ignores
	// both, so we always replace it with net/http.
	proxyFn := http.ProxyFromEnvironment
	if cfg.ProxyURL != "" {
		parsed, perr := url.Parse(cfg.ProxyURL)
		if perr != nil {
			return nil, nil, fmt.Errorf("telegram: 无效的 proxy_url %q: %w", cfg.ProxyURL, perr)
		}
		proxyFn = http.ProxyURL(parsed)
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			Proxy:                 proxyFn,
			DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		Timeout: 0,
	}
	opts = append(opts, telego.WithHTTPClient(httpClient))

	bot, err := telego.NewBot(cfg.BotToken, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("telego.NewBot: %w", err)
	}

	timeout := cfg.PollingTimeoutSeconds
	if timeout <= 0 {
		timeout = defaultPollingTimeoutSeconds
	}

	src := func(ctx context.Context) (<-chan telego.Update, error) {
		return bot.UpdatesViaLongPolling(ctx, &telego.GetUpdatesParams{
			Timeout: timeout,
		})
	}

	return bot, src, nil
}
