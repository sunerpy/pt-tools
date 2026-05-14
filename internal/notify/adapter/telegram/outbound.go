package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/mymmrac/telego"

	"github.com/sunerpy/pt-tools/internal/notify"
)

const targetParseModeKey = "parse_mode"

const targetChatIDKey = "chat_id"

func (c *TelegramChannel) Send(ctx context.Context, n notify.Notification) error {
	c.mu.RLock()
	bot := c.bot
	cfg := c.cfg
	healthy := c.healthy
	c.mu.RUnlock()

	if !healthy || bot == nil || cfg == nil {
		return errors.New("telegram: channel not initialized")
	}

	chatID, err := resolveChatID(n, cfg)
	if err != nil {
		return err
	}

	parseMode := ""
	if n.Targets != nil {
		if v, ok := n.Targets[targetParseModeKey]; ok {
			parseMode = v
		}
	}

	params := &telego.SendMessageParams{
		ChatID: telego.ChatID{ID: chatID},
		Text:   composeText(n, parseMode),
	}
	if parseMode != "" {
		params.ParseMode = parseMode
	}
	if n.DisableWebPreview {
		params.LinkPreviewOptions = &telego.LinkPreviewOptions{IsDisabled: true}
	}

	if _, err := bot.SendMessage(ctx, params); err != nil {
		return fmt.Errorf("telegram: SendMessage 失败: %w", err)
	}
	return nil
}

func resolveChatID(n notify.Notification, cfg *Config) (int64, error) {
	if n.Targets != nil {
		if raw, ok := n.Targets[targetChatIDKey]; ok && raw != "" {
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("telegram: 无效 targets.chat_id %q: %w", raw, err)
			}
			return id, nil
		}
	}
	if n.UserID != "" {
		id, err := strconv.ParseInt(n.UserID, 10, 64)
		if err == nil {
			return id, nil
		}
	}
	if cfg.DefaultChatID != 0 {
		return cfg.DefaultChatID, nil
	}
	return 0, errors.New("telegram: 未指定 chat_id 且 default_chat_id 为空")
}

func composeText(n notify.Notification, parseMode string) string {
	switch {
	case n.Title != "" && n.Text != "":
		if parseMode == "HTML" {
			return "<b>" + n.Title + "</b>\n" + n.Text
		}
		return n.Title + "\n" + n.Text
	case n.Title != "":
		return n.Title
	default:
		return n.Text
	}
}
