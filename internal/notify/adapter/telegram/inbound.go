package telegram

import (
	"context"
	"strconv"
	"strings"

	"github.com/mymmrac/telego"

	"github.com/sunerpy/pt-tools/internal/notify"
)

const denyMessage = "您没有权限执行此操作。"

const adminOnlyMessage = "只有管理员才有权限执行此命令。"

func (c *TelegramChannel) runInbound(ctx context.Context, src updateSource) {
	c.mu.RLock()
	done := c.pollDone
	c.mu.RUnlock()
	defer func() {
		if done != nil {
			close(done)
		}
	}()

	if src == nil {
		return
	}

	updates, err := src(ctx)
	if err != nil {
		c.logger.Warnf("telegram: 启动 long-poll 失败 conf=%d: %v", c.confID, err)
		c.markUnhealthy()
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case upd, ok := <-updates:
			if !ok {
				return
			}
			c.handleUpdate(ctx, upd)
		}
	}
}

func (c *TelegramChannel) handleUpdate(ctx context.Context, upd telego.Update) {
	if upd.CallbackQuery != nil {
		c.handleCallbackQuery(ctx, upd.CallbackQuery)
		return
	}
	msg := upd.Message
	if msg == nil || msg.From == nil {
		return
	}

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	userID := msg.From.ID
	chatID := msg.Chat.ID
	isCommand := strings.HasPrefix(text, "/")

	c.mu.RLock()
	cfg := c.cfg
	handler := c.handler
	c.mu.RUnlock()

	if cfg == nil {
		return
	}

	if !permitted(userID, cfg, isCommand) {
		c.replyDenied(ctx, chatID, isCommand)
		c.logger.Infof("telegram: 拒绝消息 conf=%d user=%d cmd=%v reason=%s",
			c.confID, userID, isCommand, denyReason(userID, cfg, isCommand))
		return
	}

	if handler == nil {
		return
	}

	in := notify.InboundMessage{
		ChannelType:   ChannelType,
		SourceConfID:  c.confID,
		ChannelUserID: strconv.FormatInt(userID, 10),
		Username:      msg.From.Username,
		ChatID:        strconv.FormatInt(chatID, 10),
		Text:          text,
	}
	if err := handler(ctx, in); err != nil {
		c.logger.Warnf("telegram: inbound handler 错误 conf=%d user=%d: %v",
			c.confID, userID, err)
	}
}

func permitted(userID int64, cfg *Config, isCommand bool) bool {
	admins := cfg.AdminUsersList()
	allowed := cfg.AllowedUsersList()
	if isCommand {
		if !contains(admins, userID) {
			return false
		}
	}
	if !contains(allowed, userID) && !contains(admins, userID) {
		return false
	}
	return true
}

func denyReason(userID int64, cfg *Config, isCommand bool) string {
	admins := cfg.AdminUsersList()
	allowed := cfg.AllowedUsersList()
	if isCommand && !contains(admins, userID) {
		if contains(allowed, userID) {
			return "denied:not_admin"
		}
		return "denied:not_in_whitelist"
	}
	return "denied:not_in_whitelist"
}

func (c *TelegramChannel) replyDenied(ctx context.Context, chatID int64, isCommand bool) {
	text := denyMessage
	if isCommand {
		text = adminOnlyMessage
	}
	c.mu.RLock()
	bot := c.bot
	c.mu.RUnlock()
	if bot == nil {
		return
	}
	if _, err := bot.SendMessage(ctx, &telego.SendMessageParams{
		ChatID: telego.ChatID{ID: chatID},
		Text:   text,
	}); err != nil {
		c.logger.Warnf("telegram: 发送拒绝消息失败 conf=%d chat=%d: %v",
			c.confID, chatID, err)
	}
}

func contains(xs []int64, x int64) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// handleCallbackQuery routes inline-keyboard button presses. When a
// CallbackActionHandler is installed (S5+), `dl:<logID>` and `ig:<logID>`
// payloads invoke OnRSSDownload / OnRSSIgnore and the originating message's
// inline keyboard is cleared via EditMessageReplyMarkup so users cannot
// double-click. Without an action handler the call is acknowledged with a
// "处理中" stub message (S4 behavior, retained for tests).
func (c *TelegramChannel) handleCallbackQuery(ctx context.Context, cq *telego.CallbackQuery) {
	if cq == nil {
		return
	}
	c.mu.RLock()
	bot := c.bot
	cfg := c.cfg
	handler := c.handler
	action := c.actionHandler
	c.mu.RUnlock()
	if bot == nil || cfg == nil {
		return
	}

	userID := cq.From.ID
	if !contains(cfg.AllowedUsersList(), userID) && !contains(cfg.AdminUsersList(), userID) {
		_ = bot.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
			CallbackQueryID: cq.ID,
			Text:            denyMessage,
			ShowAlert:       true,
		})
		c.logger.Infof("telegram: 拒绝 callback conf=%d user=%d", c.confID, userID)
		return
	}

	data := cq.Data
	parts := strings.SplitN(data, ":", 2)
	verb := ""
	payload := ""
	if len(parts) == 2 {
		verb = parts[0]
		payload = parts[1]
	}

	ackText := dispatchCallbackAction(ctx, action, verb, payload, userID, c.logger, c.confID)

	_ = bot.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{
		CallbackQueryID: cq.ID,
		Text:            ackText,
	})

	if action != nil && (verb == "dl" || verb == "ig") {
		clearInlineKeyboard(ctx, bot, cq, c.logger, c.confID)
	}

	c.logger.Infof("telegram: callback conf=%d user=%d data=%q action=%s payload=%s",
		c.confID, userID, data, verb, payload)

	if handler != nil && data != "" {
		in := notify.InboundMessage{
			ChannelType:   ChannelType,
			SourceConfID:  c.confID,
			ChannelUserID: strconv.FormatInt(userID, 10),
			Username:      cq.From.Username,
			Text:          data,
			IsCallback:    true,
			CallbackData:  data,
		}
		if msg, ok := cq.Message.(interface{ GetChat() telego.Chat }); ok {
			in.ChatID = strconv.FormatInt(msg.GetChat().ID, 10)
		}
		if err := handler(ctx, in); err != nil {
			c.logger.Warnf("telegram: callback handler 错误 conf=%d data=%q: %v",
				c.confID, data, err)
		}
	}
}

func dispatchCallbackAction(
	ctx context.Context,
	action CallbackActionHandler,
	verb, payload string,
	userID int64,
	logger interface{ Warnf(string, ...any) },
	confID uint,
) string {
	switch verb {
	case "dl":
		if action == nil {
			return "已记录下载请求 #" + payload + "（处理中）"
		}
		logID, ok := parseLogID(payload)
		if !ok {
			return "下载请求参数无效"
		}
		if err := action.OnRSSDownload(ctx, logID, userID); err != nil {
			logger.Warnf("telegram: OnRSSDownload 失败 conf=%d log=%d: %v", confID, logID, err)
			return "下载触发失败：" + err.Error()
		}
		return "已加入下载队列 #" + payload
	case "ig":
		if action == nil {
			return "已忽略 #" + payload
		}
		logID, ok := parseLogID(payload)
		if !ok {
			return "忽略请求参数无效"
		}
		if err := action.OnRSSIgnore(ctx, logID, userID); err != nil {
			logger.Warnf("telegram: OnRSSIgnore 失败 conf=%d log=%d: %v", confID, logID, err)
			return "忽略失败：" + err.Error()
		}
		return "已忽略 #" + payload
	case "dt":
		return "请查看消息中的链接"
	default:
		return "未知操作"
	}
}

func parseLogID(s string) (uint, bool) {
	v, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	if err != nil || v == 0 {
		return 0, false
	}
	return uint(v), true
}

func clearInlineKeyboard(
	ctx context.Context,
	bot botAPI,
	cq *telego.CallbackQuery,
	logger interface{ Warnf(string, ...any) },
	confID uint,
) {
	if cq.Message == nil {
		return
	}
	getter, ok := cq.Message.(interface {
		GetChat() telego.Chat
		GetMessageID() int
	})
	if !ok {
		return
	}
	chat := getter.GetChat()
	msgID := getter.GetMessageID()
	if msgID == 0 {
		return
	}
	if _, err := bot.EditMessageReplyMarkup(ctx, &telego.EditMessageReplyMarkupParams{
		ChatID:    telego.ChatID{ID: chat.ID},
		MessageID: msgID,
	}); err != nil {
		logger.Warnf("telegram: EditMessageReplyMarkup 失败 conf=%d msg=%d: %v", confID, msgID, err)
	}
}
