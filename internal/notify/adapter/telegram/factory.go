package telegram

import (
	"context"
	"fmt"

	"github.com/mymmrac/telego"
)

const defaultPollingTimeoutSeconds = 30

func defaultBotFactory(cfg *Config) (botAPI, updateSource, error) {
	opts := []telego.BotOption{telego.WithDiscardLogger()}
	if cfg.APIServer != "" {
		opts = append(opts, telego.WithAPIServer(cfg.APIServer))
	}

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
