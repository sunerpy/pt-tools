package assistant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/chatops"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/models"
)

// ChatOpsReplier 是 ChatOps 的回复出口（cmd 的 liveNotifyManager：按消息来源的通道配置找到在线的通道发送）。
type ChatOpsReplier interface {
	Reply(ctx context.Context, msg notify.InboundMessage, reply chatops.Reply) error
}

// ChatOpsHandler 把通知渠道里绑定用户的非命令消息交给助手（实现 chatops.AssistantHandler 与 /newchat 的 AssistantService）：
//   - 每个绑定一个滚动会话（属主 chatops:<绑定编号>），/newchat 开新的；
//   - 有待确认的操作时，下一条非命令消息由这里解读：「确认」经条件更新批准，别的内容一律拒绝；
//   - 只登记、立即返回，回答、确认提示、超时提示都由后台运行经 Replier 发出；
//   - 不是管理员的绑定拿不到改动类工具。
type ChatOpsHandler struct {
	svc     *Service
	replier ChatOpsReplier
	// locks 是每个属主一把锁：找滚动会话（没有就建）与发消息要一起做，同时到的两条消息不会各建一个会话
	locksMu sync.Mutex
	locks   map[string]*sync.Mutex
}

// NewChatOpsHandler 建处理器。
func NewChatOpsHandler(svc *Service, replier ChatOpsReplier) *ChatOpsHandler {
	return &ChatOpsHandler{svc: svc, replier: replier, locks: map[string]*sync.Mutex{}}
}

func (h *ChatOpsHandler) ownerLock(owner string) *sync.Mutex {
	h.locksMu.Lock()
	defer h.locksMu.Unlock()
	l := h.locks[owner]
	if l == nil {
		l = &sync.Mutex{}
		h.locks[owner] = l
	}
	return l
}

// 一条 ChatOps 回复的字数上限（Telegram 一条消息最多 4096 个字符）；更长的回答分几条发，最多 chatopsReplyParts 条。
const (
	chatopsReplyRunes = 3500
	chatopsReplyParts = 4
)

// Enabled 报告助手启用了、接口配好了、ChatOps 开关也打开了。
func (h *ChatOpsHandler) Enabled(ctx context.Context) bool {
	if h == nil || h.svc == nil {
		return false
	}
	set, err := h.svc.effectiveSettings(ctx)
	return err == nil && set.chatOps
}

func chatopsOwner(src chatops.Source) string { return fmt.Sprintf("chatops:%d", src.BindingID) }

func (h *ChatOpsHandler) actor(msg notify.InboundMessage, src chatops.Source) Actor {
	name := src.Username
	if name == "" {
		name = src.ChannelUserID
	}
	a := Actor{Owner: chatopsOwner(src), Entry: models.AssistantEntryChatOps, Name: name, CanWrite: src.PtAdmin}
	lang := src.ReplyLang
	a.Notify = func(ctx context.Context, ev Event) {
		text := chatopsEventText(ev, lang)
		if text == "" || h.replier == nil {
			return
		}
		for _, part := range splitReply(text, lang) {
			if err := h.replier.Reply(ctx, msg, chatops.Reply{Text: part}); err != nil {
				global.GetSlogger().Warnf("[AI 助手] ChatOps 回复失败 conf=%d: %v", msg.SourceConfID, err)
				return
			}
		}
	}
	return a
}

func tr(lang, zh, en string) string {
	if strings.HasPrefix(strings.ToLower(lang), "en") {
		return en
	}
	return zh
}

// yesWords 是确认的说法（去掉首尾空白与句末标点、转小写以后比）。
var yesWords = map[string]bool{
	"确认": true, "确定": true, "是": true, "是的": true, "好": true, "好的": true, "执行": true, "确认执行": true,
	"yes": true, "y": true, "ok": true, "okay": true, "confirm": true,
}

func isYes(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	t = strings.TrimRight(t, "。.!！~～ ")
	return yesWords[t]
}

func chatReply(text string) chatops.Reply { return chatops.Reply{Text: text} }

// Handle 处理一条非命令消息。返回立即回复与审计结果。
func (h *ChatOpsHandler) Handle(ctx context.Context, in chatops.InboundAssistantMessage) (chatops.Reply, string) {
	a := h.actor(in.Msg, in.Src)
	lang := in.Src.ReplyLang
	text := strings.TrimSpace(in.Msg.Text)
	if pc, ok, err := h.svc.PendingConfirmation(ctx, a); err != nil {
		return chatReply(tr(lang, "AI 助手出错了，稍后再试", "the AI assistant failed, try again later")), "error:assistant"
	} else if ok {
		approve := isYes(text)
		_, err := h.svc.Decide(ctx, a, pc.ID, approve)
		switch {
		case errors.Is(err, ErrDecided):
			return chatReply(tr(lang, "这个操作已经处理过了。", "That action was already handled.")), "denied:decided"
		case errors.Is(err, ErrForbidden):
			return chatReply(tr(lang, "这个绑定不能确认改动类操作。", "This binding cannot confirm changes.")), "denied:not_admin"
		case err != nil:
			return chatReply(tr(lang, "处理确认失败，稍后再试", "could not handle the confirmation")), "error:confirm"
		case approve:
			return chatReply(tr(lang, "已确认，正在执行。", "Confirmed, running it.")), "success"
		default:
			return chatReply(tr(lang, "已取消这个操作。要问别的，等这次回答结束再发。",
				"Canceled. Send your next question after this answer finishes.")), "denied:user"
		}
	}
	l := h.ownerLock(a.Owner)
	l.Lock()
	defer l.Unlock()
	// 一个绑定同时只有一次运行（不管哪个会话）：通知渠道里回「确认」认的是最近那一个待确认，
	// 两次运行各挂一个待确认时会认错
	if h.svc.ownerBusy(a.Owner) {
		return chatReply(tr(lang, "上一个问题还在回答，等它说完再问。", "Still answering the previous question.")), "denied:busy"
	}
	conv, ok, err := h.svc.LatestConversation(ctx, a)
	if err == nil && !ok {
		conv, err = h.svc.CreateConversation(ctx, a, "")
	}
	if err != nil {
		return chatReply(tr(lang, "AI 助手出错了，稍后再试", "the AI assistant failed, try again later")), "error:assistant"
	}
	_, err = h.svc.Send(ctx, a, conv.ID, text)
	switch {
	case err == nil:
		return chatops.Reply{}, "success"
	case errors.Is(err, ErrBusy):
		return chatReply(tr(lang, "上一个问题还在回答，等它说完再问。", "Still answering the previous question.")), "denied:busy"
	case errors.Is(err, ErrTooMany):
		return chatReply(tr(lang, "同时在回答的问题太多，稍后再试。", "Too many answers in progress, try again later.")), "denied:too_many"
	case errors.Is(err, ErrDisabled):
		return chatReply(tr(lang, "AI 助手没有启用。", "The AI assistant is not enabled.")), "denied:disabled"
	case errors.Is(err, ErrInvalid):
		return chatReply(err.Error()), "error:invalid"
	}
	return chatReply(tr(lang, "AI 助手出错了，稍后再试", "the AI assistant failed, try again later")), "error:assistant"
}

// NewChat 给这个绑定开一个新会话（/newchat）。
func (h *ChatOpsHandler) NewChat(ctx context.Context, src chatops.Source) error {
	if !h.Enabled(ctx) {
		return errors.New(tr(src.ReplyLang, "AI 助手没有启用，或者没有接到通知渠道", "the AI assistant is not enabled for notification channels"))
	}
	owner := chatopsOwner(src)
	l := h.ownerLock(owner)
	l.Lock()
	defer l.Unlock()
	if h.svc.ownerBusy(owner) {
		return errors.New(tr(src.ReplyLang,
			"上一个回答还没结束：等它说完再开新对话（在等确认的话，回复「取消」就结束了）",
			"the previous answer is still running; wait for it (reply \"cancel\" if it is waiting for a confirmation)"))
	}
	_, err := h.svc.CreateConversation(ctx, Actor{Owner: owner, Entry: models.AssistantEntryChatOps}, "")
	return err
}

// chatopsEventText 是一个事件在通知渠道里发的文字：最终回答、pt-tools 的提示、要确认的操作与确认超时；别的不发。
func chatopsEventText(ev Event, lang string) string {
	switch ev.Type {
	case EventMessage:
		m := ev.Message
		if m == nil {
			return ""
		}
		switch {
		case m.Role == models.AssistantRoleAssistant && len(m.ToolCalls) == 0 && strings.TrimSpace(m.Content) != "":
			return plainText(m.Content)
		case m.Role == models.AssistantRoleNotice:
			return m.Content
		}
	case EventConfirmation:
		c := ev.Confirmation
		if c == nil {
			return ""
		}
		switch {
		case c.Status == models.AssistantConfirmPending:
			return tr(lang,
				fmt.Sprintf("需要你确认：%s\n回复「确认」执行，回复别的内容取消（%s 以前有效）。", c.Summary, c.ExpiresAt.Local().Format("15:04")),
				fmt.Sprintf("Please confirm: %s\nReply \"yes\" to run it; anything else cancels (valid until %s).", c.Summary, c.ExpiresAt.Local().Format("15:04")))
		case c.Status == models.AssistantConfirmExpired && c.DecidedBy == decidedByTimeout:
			return tr(lang, "确认超时，没有执行："+c.Summary, "Confirmation timed out, not run: "+c.Summary)
		}
	}
	return ""
}

// plainText 去掉回答里常见的 Markdown 记号（通知渠道按纯文本发）。
func plainText(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if t := strings.TrimSpace(l); strings.HasPrefix(t, "#") {
			lines[i] = strings.TrimSpace(strings.TrimLeft(t, "#"))
		}
	}
	return strings.Join(lines, "\n")
}

// splitReply 把太长的回答分成几条（每条不超过 chatopsReplyRunes 个字，尽量在换行处分），最多 chatopsReplyParts 条，
// 再多的省略并注明（注明的话也算在字数里）。
func splitReply(s, lang string) []string {
	var parts []string
	rest := []rune(s)
	for len(rest) > chatopsReplyRunes && len(parts) < chatopsReplyParts-1 {
		cut := chatopsReplyRunes
		for i := chatopsReplyRunes; i > chatopsReplyRunes/2; i-- {
			if rest[i-1] == '\n' {
				cut = i
				break
			}
		}
		parts = append(parts, strings.TrimRight(string(rest[:cut]), "\n"))
		rest = rest[cut:]
	}
	if len(rest) > chatopsReplyRunes {
		note := []rune(tr(lang, "\n…（回答太长，后面的省略了）", "\n… (the answer is too long; the rest is left out)"))
		rest = append(rest[:chatopsReplyRunes-len(note):chatopsReplyRunes-len(note)], note...)
	}
	return append(parts, strings.TrimRight(string(rest), "\n"))
}
