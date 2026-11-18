package commands

import (
	"context"
	"sync/atomic"

	"github.com/sunerpy/pt-tools/internal/chatops"
)

// AssistantService 是 /newchat 用到的内置 AI 助手能力（路线图 M19）。
type AssistantService interface {
	// NewChat 给这个绑定开一个新会话：之后的消息不带以前的上下文。助手没有启用时返回错误。
	NewChat(ctx context.Context, src chatops.Source) error
}

type assistantRef struct{ svc AssistantService }

// assistant 在通道启动以后才接上，单独放一个原子指针，不和 Services 一起替换。
var assistant atomic.Pointer[assistantRef]

// SetAssistant 接上内置 AI 助手（nil 摘掉）。
func SetAssistant(svc AssistantService) {
	if svc == nil {
		assistant.Store(nil)
		return
	}
	assistant.Store(&assistantRef{svc: svc})
}

func init() {
	chatops.RegisterCommand(chatops.CommandSpec{
		Name:        "newchat",
		Description: "AI 助手开一个新对话，之后的消息不带以前的上下文 (Start a new AI assistant conversation)",
		Handler:     newchatHandler,
	})
}

func newchatHandler(ctx context.Context, _ []string, src chatops.Source) (chatops.Reply, error) {
	ref := assistant.Load()
	if ref == nil {
		return errReply(src.ReplyLang, "AI 助手不可用", "the AI assistant is unavailable"), nil
	}
	if err := ref.svc.NewChat(ctx, src); err != nil {
		return errReply(src.ReplyLang, "开新对话失败: %v", "could not start a new conversation: %v", err), nil
	}
	return okReply(tr(src.ReplyLang,
		"已开新对话：接下来的消息不带以前的上下文。",
		"New conversation started: the next messages carry no earlier context.")), nil
}
