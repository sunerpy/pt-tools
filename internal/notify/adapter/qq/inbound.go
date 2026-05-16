package qq

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

type onebotEvent struct {
	PostType    string          `json:"post_type"`
	MessageType string          `json:"message_type"`
	SubType     string          `json:"sub_type"`
	SelfID      int64           `json:"self_id"`
	UserID      int64           `json:"user_id"`
	GroupID     int64           `json:"group_id"`
	MessageID   int64           `json:"message_id"`
	RawMessage  string          `json:"raw_message"`
	Message     json.RawMessage `json:"message"`
	Sender      struct {
		UserID   int64  `json:"user_id"`
		Nickname string `json:"nickname"`
	} `json:"sender"`
}

func (q *QQChannel) HandleRawEvent(payload []byte) error {
	var evt onebotEvent
	if err := json.Unmarshal(payload, &evt); err != nil {
		return fmt.Errorf("解析 OneBot 事件失败: %w", err)
	}
	if isHeartbeatEvent(evt) {
		qqLogger().Debugf("QQ 适配器(%d): 收到事件 post_type=%s msg_type=%s user=%d group=%d",
			q.confID, evt.PostType, evt.MessageType, evt.UserID, evt.GroupID)
	} else {
		qqLogger().Infof("QQ 适配器(%d): 收到事件 post_type=%s msg_type=%s user=%d group=%d text=%q",
			q.confID, evt.PostType, evt.MessageType, evt.UserID, evt.GroupID, evt.RawMessage)
	}
	if evt.PostType != "message" {
		return nil
	}
	if !q.isUserAllowed(evt.UserID) {
		warnLogger().Warnf("QQ 适配器(%d): 拒绝非授权用户 user=%d (admin_count=%d allowed_count=%d)",
			q.confID, evt.UserID, len(q.adminUsers), len(q.allowedUsers))
		return nil
	}

	text := evt.RawMessage
	if text == "" {
		text = decodeMessageField(evt.Message)
	}

	msg := q.eventToInbound(evt, text)

	q.handlerMu.RLock()
	handler := q.inboundHandler
	q.handlerMu.RUnlock()
	if handler == nil {
		warnLogger().Warnf("QQ 适配器(%d): inboundHandler 未设置，丢弃消息 text=%q", q.confID, text)
		return nil
	}

	qqLogger().Infof("QQ 适配器(%d): 路由到 ChatOps user=%d text=%q", q.confID, evt.UserID, text)
	ctx := context.Background()
	if q.lifecycleCtx != nil {
		ctx = q.lifecycleCtx
	}
	// Run handler in its own goroutine so the WS read loop can keep draining
	// inbound frames (esp. our own outbound API echo responses) while the chain
	// processes commands. Without this, /sites or any handler that triggers an
	// outbound CallAPI deadlocks: CallAPI waits for the response, but the
	// response can only be dispatched by the same read goroutine that's blocked
	// inside the handler.
	go func(ctx context.Context, msg inboundMessage) {
		if err := handler(ctx, msg); err != nil {
			warnLogger().Warnf("QQ 适配器(%d): ChatOps handler 异常: %v", q.confID, err)
		}
	}(ctx, msg)
	return nil
}

func (q *QQChannel) eventToInbound(evt onebotEvent, text string) inboundMessage {
	chatID := strconv.FormatInt(evt.UserID, 10)
	if evt.MessageType == "group" && evt.GroupID > 0 {
		chatID = strconv.FormatInt(evt.GroupID, 10)
	}
	return inboundMessage{
		ChannelType:   q.Type(),
		SourceConfID:  q.confID,
		ChannelUserID: strconv.FormatInt(evt.UserID, 10),
		Username:      evt.Sender.Nickname,
		ChatID:        chatID,
		MessageType:   evt.MessageType,
		Text:          text,
	}
}

func (q *QQChannel) isUserAllowed(userID int64) bool {
	if _, ok := q.adminUsers[userID]; ok {
		return true
	}
	if _, ok := q.allowedUsers[userID]; ok {
		return true
	}
	return false
}

func decodeMessageField(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var arr []struct {
		Type string                 `json:"type"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(raw, &arr); err == nil {
		var out string
		for _, seg := range arr {
			if seg.Type == "text" {
				if v, ok := seg.Data["text"].(string); ok {
					out += v
				}
			}
		}
		return out
	}
	return ""
}

func isHeartbeatEvent(evt onebotEvent) bool {
	return evt.PostType == "meta_event"
}
