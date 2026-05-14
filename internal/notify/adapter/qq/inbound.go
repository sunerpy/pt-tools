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
	if evt.PostType != "message" {
		return nil
	}
	if !q.isUserAllowed(evt.UserID) {
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
		return nil
	}

	ctx := context.Background()
	if q.lifecycleCtx != nil {
		ctx = q.lifecycleCtx
	}
	return handler(ctx, msg)
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
