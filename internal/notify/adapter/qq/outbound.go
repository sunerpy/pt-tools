package qq

import (
	"context"
	"errors"
	"strconv"
	"strings"

	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

func (q *QQChannel) sendOutbound(ctx context.Context, chatID, text, messageType string) error {
	caller := q.activeCaller()
	if caller == nil {
		return errors.New("QQ 通道未连接 (NapCat 尚未握手)")
	}

	id, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil || id <= 0 {
		return errors.New("无效的 QQ 目标 ID: " + chatID)
	}

	action := "send_group_msg"
	idKey := "group_id"
	if strings.EqualFold(messageType, "private") {
		action = "send_private_msg"
		idKey = "user_id"
	}

	req := zero.APIRequest{
		Action: action,
		Params: map[string]interface{}{
			idKey:     id,
			"message": message.Message{message.Text(text)},
		},
	}
	resp, err := caller.CallAPI(ctx, req)
	if err != nil {
		return err
	}
	if resp.RetCode != 0 {
		return errors.New("OneBot send 失败: " + resp.Message)
	}
	return nil
}
