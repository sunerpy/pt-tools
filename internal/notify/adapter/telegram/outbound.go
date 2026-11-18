package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/mymmrac/telego"

	"github.com/sunerpy/pt-tools/internal/notify"
)

const targetParseModeKey = "parse_mode"

const targetChatIDKey = "chat_id"

// Send 不看 healthy：它跟着长轮询（收消息）走，getUpdates 返回 502/504、同一个 bot 在别处被轮询（409）
// 时为假，sendMessage 却不一定发不出去。原来按它拒发，长轮询出问题的几个小时里每条通知都报
// not initialized、根本没去发。现在只在没初始化成功或已关闭时拒发，发不出去时报 Telegram 回的原因。
func (c *TelegramChannel) Send(ctx context.Context, n notify.Notification) error {
	c.mu.RLock()
	bot := c.bot
	cfg := c.cfg
	closed := c.pollCtx != nil && c.pollCtx.Err() != nil
	c.mu.RUnlock()

	if bot == nil || cfg == nil {
		return errors.New("telegram: channel not initialized")
	}
	if closed {
		return errors.New("telegram: 通道已关闭")
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
		ChatID: chatID,
		Text:   composeText(n, parseMode),
	}
	if parseMode != "" {
		params.ParseMode = parseMode
	}
	if n.DisableWebPreview {
		params.LinkPreviewOptions = &telego.LinkPreviewOptions{IsDisabled: true}
	}
	if markup := buildInlineKeyboard(n.Buttons); markup != nil {
		params.ReplyMarkup = markup
	}

	if _, err := bot.SendMessage(ctx, params); err != nil {
		return redactBotToken(fmt.Errorf("telegram: SendMessage 失败: %w", err), cfg.BotToken)
	}
	return nil
}

// tokenRedactedError 的文字里令牌换成 <bot_token>，Unwrap 仍是原错误。
// net/http 的错误带着请求地址（https://api.telegram.org/bot<令牌>/sendMessage），
// 发送错误会写进日志、通知记录与操作审计。
type tokenRedactedError struct {
	err   error
	token string
}

func (e tokenRedactedError) Error() string {
	return strings.ReplaceAll(e.err.Error(), e.token, "<bot_token>")
}

func (e tokenRedactedError) Unwrap() error { return e.err }

func redactBotToken(err error, token string) error {
	if err == nil || token == "" {
		return err
	}
	return tokenRedactedError{err: err, token: token}
}

func buildInlineKeyboard(rows [][]notify.Button) *telego.InlineKeyboardMarkup {
	if len(rows) == 0 {
		return nil
	}
	out := make([][]telego.InlineKeyboardButton, 0, len(rows))
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		buttons := make([]telego.InlineKeyboardButton, 0, len(row))
		for _, b := range row {
			if b.Text == "" {
				continue
			}
			btn := telego.InlineKeyboardButton{Text: b.Text}
			if b.URL != "" {
				btn.URL = b.URL
			} else if b.CallbackData != "" {
				btn.CallbackData = b.CallbackData
			} else {
				continue
			}
			buttons = append(buttons, btn)
		}
		if len(buttons) > 0 {
			out = append(out, buttons)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return &telego.InlineKeyboardMarkup{InlineKeyboard: out}
}

// resolveChatID returns the telego.ChatID built from notification targets and
// channel config. Order of precedence:
//  1. n.Targets["chat_id"] — accepts numeric string or @username
//  2. n.UserID — numeric string only
//  3. cfg.DefaultChatIDInt() — int (or string-quoted int) from raw JSON
//  4. cfg.DefaultChatIDUsername() — @channelusername fallback
func resolveChatID(n notify.Notification, cfg *Config) (telego.ChatID, error) {
	if n.Targets != nil {
		if raw, ok := n.Targets[targetChatIDKey]; ok && raw != "" {
			return parseChatIDString(raw)
		}
	}
	if n.UserID != "" {
		if id, err := strconv.ParseInt(n.UserID, 10, 64); err == nil {
			return telego.ChatID{ID: id}, nil
		}
	}
	if id, ok := cfg.DefaultChatIDInt(); ok && id != 0 {
		return telego.ChatID{ID: id}, nil
	}
	if name, ok := cfg.DefaultChatIDUsername(); ok && name != "" {
		return telego.ChatID{Username: name}, nil
	}
	return telego.ChatID{}, errors.New("telegram: 未指定 chat_id 且 default_chat_id 为空")
}

// parseChatIDString turns a string into telego.ChatID. Numeric → ID,
// @-prefixed or non-numeric → Username (with @ prepended if missing).
func parseChatIDString(s string) (telego.ChatID, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return telego.ChatID{}, errors.New("telegram: chat_id 为空")
	}
	if id, err := strconv.ParseInt(s, 10, 64); err == nil {
		return telego.ChatID{ID: id}, nil
	}
	if !strings.HasPrefix(s, "@") {
		s = "@" + s
	}
	return telego.ChatID{Username: s}, nil
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
