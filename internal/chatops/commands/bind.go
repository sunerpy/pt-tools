package commands

import (
	"context"
	"strings"

	"github.com/sunerpy/pt-tools/internal/chatops"
)

func init() {
	chatops.RegisterCommand(chatops.CommandSpec{
		Name:        "bind",
		Description: "绑定当前用户到通知通道 (Bind this user to a notification config)",
		Handler:     bindHandler,
	})
}

func bindHandler(ctx context.Context, args []string, src chatops.Source) (chatops.Reply, error) {
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		return errReply(src.ReplyLang, "用法: /bind <绑定码>", "usage: /bind <code>"), nil
	}
	code := strings.TrimSpace(args[0])
	svc := getServices()
	if svc == nil || svc.Binding == nil {
		return errReply(src.ReplyLang, "绑定服务不可用", "binding service unavailable"), nil
	}
	if _, err := svc.Binding.ConsumeCode(ctx, code, src.ChannelType, src.ChannelUserID); err != nil {
		return errReply(src.ReplyLang, "绑定失败: %v", "bind failed: %v", err), nil
	}
	return okReply(tr(src.ReplyLang, "绑定成功", "bound successfully")), nil
}
