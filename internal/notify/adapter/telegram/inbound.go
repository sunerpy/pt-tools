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
	if isCommand {
		if !contains(cfg.AdminUsers, userID) {
			return false
		}
	}
	if !contains(cfg.AllowedUsers, userID) && !contains(cfg.AdminUsers, userID) {
		return false
	}
	return true
}

func denyReason(userID int64, cfg *Config, isCommand bool) string {
	if isCommand && !contains(cfg.AdminUsers, userID) {
		if contains(cfg.AllowedUsers, userID) {
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
