package feishuapp

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/sunerpy/pt-tools/internal/notify"
)

// expireAfter：发出超过 30 分钟才送到的消息不执行（D6）。pt-tools 离线时平台会在 1 小时、6 小时后重推，
// 几小时前的 /delete 不该再执行；取 30 分钟而不是 10 分钟，是给 NAS 的时钟偏差留余量。
const expireAfter = 30 * time.Minute

var (
	errClosing    = errors.New("飞书应用机器人通道正在关闭，请平台稍后重推")
	errQueueFull  = errors.New("飞书应用机器人：这个人排队的消息太多，请平台稍后重推")
	errBotUnknown = errors.New("飞书应用机器人：还没取到机器人信息，判断不了群消息有没有 @ 它，请平台稍后重推")
)

// onMessage 处理一条「接收消息」事件。平台要求 3 秒内应答且不报错，否则重推：这里只做内存操作，
// 要发消息的提示放进后台协程，命令交给按用户串行、按发出时间排的队列（dispatch.go）。
// 返回 nil 时 SDK 回 200，返回 error 时回 500。
func (c *Channel) onMessage(ctx context.Context, ev *larkim.P2MessageReceiveV1) error {
	if ev == nil || ev.Event == nil || ev.Event.Sender == nil || ev.Event.Message == nil {
		return nil
	}
	sender, msg := ev.Event.Sender, ev.Event.Message
	if deref(sender.SenderType) != "user" {
		return nil
	}
	var openID string
	if sender.SenderId != nil {
		openID = deref(sender.SenderId.OpenId)
	}
	messageID := deref(msg.MessageId)
	if openID == "" || messageID == "" {
		return nil
	}
	if c.closed.Load() {
		// 热重载时旧实例还没断开：回 500 让平台重推，新实例会接手
		return errClosing
	}
	c.mu.RLock()
	cfg := c.cfg
	c.mu.RUnlock()
	if cfg == nil {
		return nil
	}

	key := cfg.AppID + "\x00" + messageID
	if seenMessages.check(key) {
		return nil
	}
	chatType := deref(msg.ChatType)
	group := chatType != "p2p"
	chatID := deref(msg.ChatId)
	log := c.logger()

	botID := c.botOpenID()
	if group && botID == "" && len(msg.Mentions) > 0 {
		// 还没取到本机器人的 open_id（刚启动、刚热重载，或者取机器人信息失败在重试）：判断不了有没有 @ 它，
		// 移出去重表、回 500，让平台稍后重推，取到以后照常处理。没 @ 任何人的群消息不可能是给它的，下面照常忽略。
		seenMessages.forget(key)
		log.Infof("飞书应用机器人(%d): 还没取到机器人信息，判断不了群消息有没有 @ 机器人，请飞书稍后重推 message_id=%s", c.confID, messageID)
		return errBotUnknown
	}
	if group && !mentionsBot(msg.Mentions, botID) {
		// 应用开了「获取群组中所有消息」时群里每句话都会推过来，只认 @ 了本机器人的
		log.Debugf("飞书应用机器人(%d): 群消息没有 @ 本机器人，忽略 message_id=%s", c.confID, messageID)
		return nil
	}
	if !cfg.permitted(openID) {
		log.Infof("飞书应用机器人(%d): 拒绝名单外的消息 open_id=%s chat_type=%s message_id=%s", c.confID, openID, chatType, messageID)
		if !group && denyNotices.allow(cfg.AppID+"\x00"+openID) {
			c.sendNotice(noticeTarget(chatID, openID), denyText(openID))
		}
		return nil
	}
	now := c.now()
	sent := now // 没有 create_time 时按收到的时间排
	if created, ok := createTime(msg.CreateTime); ok {
		if age := now.Sub(created); age > expireAfter {
			log.Infof("飞书应用机器人(%d): 消息发出已 %s，不执行 open_id=%s message_id=%s", c.confID, age.Round(time.Minute), openID, messageID)
			c.sendNotice(noticeTarget(chatID, openID), expiredText(age))
			return nil
		}
		sent = created
	}
	text := extractText(deref(msg.MessageType), deref(msg.Content), msg.Mentions, botID)
	if text == "" {
		return nil
	}
	in := notify.InboundMessage{
		ChannelType:   ChannelType,
		SourceConfID:  c.confID,
		ChannelUserID: openID,
		ChatID:        chatID,
		MessageType:   "private",
		Text:          text,
	}
	if group {
		in.MessageType = "group"
	}
	if !c.enqueue(openID, newQueued(in, sent, messageID), false) {
		seenMessages.forget(key)
		log.Warnf("飞书应用机器人(%d): 处理不过来，请平台重推 open_id=%s message_id=%s", c.confID, openID, messageID)
		if c.closed.Load() {
			return errClosing
		}
		return errQueueFull
	}
	// 不记正文：/addrss 带着 RSS 地址里的 passkey
	// chat_id 要写：群的会话 ID（oc_ 开头）用户只能从这一行拿，填进「默认会话 ID」
	log.Infof("飞书应用机器人(%d): 收到消息 open_id=%s chat_type=%s chat_id=%s message_id=%s len=%d",
		c.confID, openID, chatType, chatID, messageID, len(text))
	if d := qaSlowAck(); d > 0 {
		sleepCtx(ctx, d)
	}
	return nil
}

func createTime(ms *string) (time.Time, bool) {
	n, err := strconv.ParseInt(deref(ms), 10, 64)
	if err != nil || n <= 0 {
		return time.Time{}, false
	}
	return time.UnixMilli(n), true
}

// noticeTarget 是提示发到哪：私聊会话的 chat_id（oc_），没有时直接发给这个人。
func noticeTarget(chatID, openID string) string {
	if chatID != "" {
		return chatID
	}
	return openID
}

// denyText 是名单外的人私聊机器人时的回复（D4）：用户自己很难查到 open_id，这是最省事的拿法。
func denyText(openID string) string {
	return "你不在这个 pt-tools 机器人的名单里，发来的消息没有处理。\n你的 open_id 是 " + openID +
		"\n请 pt-tools 的管理员在通道设置里把它填进「管理员 open_id」或「允许的用户 open_id」。"
}

// expiredText 是过期消息的回复。
func expiredText(age time.Duration) string {
	ago := fmt.Sprintf("%d 分钟前", int(age.Minutes()))
	if age >= 2*time.Hour {
		ago = fmt.Sprintf("约 %d 小时前", int(age.Round(time.Hour).Hours()))
	}
	return "这条消息是 " + ago + "发的，pt-tools 当时没连上飞书，没有执行；需要的话请重新发送。"
}

// sendNotice 在后台协程里发一条提示（受生命周期 ctx 管，Close 时取消），不拖慢事件应答。
func (c *Channel) sendNotice(target, text string) {
	c.spawn(func() {
		ctx, cancel := context.WithTimeout(c.ctx, c.timing(c.noticeTimeout, httpTimeout))
		defer cancel()
		typ, err := receiveIDType(target)
		if err == nil {
			err = c.sendContent(ctx, c.api, typ, target, textContent(text))
		}
		if err != nil && c.ctx.Err() == nil {
			c.logger().Warnf("飞书应用机器人(%d): 发送提示失败: %v", c.confID, c.red.wrap(err))
		}
	})
}

// botOpenID 是本应用机器人的 open_id；还没取到时为空串。
func (c *Channel) botOpenID() string {
	if p := c.botID.Load(); p != nil {
		return *p
	}
	return ""
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
