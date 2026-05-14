package commands

import (
	"context"

	"github.com/sunerpy/pt-tools/internal/chatops"
)

func init() {
	chatops.RegisterCommand(chatops.CommandSpec{
		Name:        "unbind",
		Description: "Revoke this user's binding (admin self-service)",
		AdminOnly:   true,
		Handler:     unbindHandler,
	})
}

func unbindHandler(ctx context.Context, _ []string, src chatops.Source) (chatops.Reply, error) {
	svc := getServices()
	if svc == nil || svc.Binding == nil || svc.Bindings == nil {
		return errReply(src.ReplyLang, "绑定服务不可用", "binding service unavailable"), nil
	}
	id, ok, err := svc.Bindings.FindByChannelUser(ctx, src.ChannelType, src.ChannelUserID)
	if err != nil {
		return errReply(src.ReplyLang, "查询绑定失败: %v", "lookup binding failed: %v", err), nil
	}
	if !ok || id == 0 {
		return errReply(src.ReplyLang, "未找到当前绑定", "binding not found"), nil
	}
	if err := svc.Binding.Revoke(ctx, id); err != nil {
		return errReply(src.ReplyLang, "解绑失败: %v", "unbind failed: %v", err), nil
	}
	return okReply(tr(src.ReplyLang, "已解绑", "unbound")), nil
}
