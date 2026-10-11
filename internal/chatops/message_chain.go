package chatops

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/utils"
)

// BindingInfo is the subset of binding data needed by the chain.
// Defined locally (instead of importing internal/app) to avoid an import
// cycle: internal/app already imports internal/chatops for GenerateBindCode.
type BindingInfo struct {
	ID            uint
	ConfID        uint
	ChannelType   string
	ChannelUserID string
	ReplyLang     string
	PtAdmin       bool
	Allowed       bool
}

// AuditEntry is the audit row written by the chain.
type AuditEntry struct {
	NotificationConfID uint
	ChannelType        string
	ChannelUserID      string
	Command            string
	Args               map[string]any
	Result             string
	LatencyMs          int64
}

// CommandRegistryAPI abstracts CommandRegistry so the chain can be tested
// without depending on the concrete registry implementation.
type CommandRegistryAPI interface {
	Get(name string) (CommandSpec, bool)
	List() []CommandSpec
}

// RateLimiterAPI abstracts RateLimiter for the chain.
type RateLimiterAPI interface {
	Allow(channel, userID, command string) bool
}

// SessionStoreAPI abstracts SessionStore for the chain.
type SessionStoreAPI interface {
	Pending(channel string, confID uint, userID string) (SessionState, bool)
	Set(channel string, confID uint, userID string, state SessionState, ttl time.Duration)
	Clear(channel string, confID uint, userID string)
}

// Replier is the channel-side reply egress (implemented by adapters T17-T20).
type Replier interface {
	Reply(ctx context.Context, msg notify.InboundMessage, reply Reply) error
}

// BindingLookup resolves a (notification conf, channel, user) triple to a BindingInfo.
// 绑定属于具体的通知通道配置：同一个用户在另一个 Bot/通道上发消息，不能沿用这里的绑定。
type BindingLookup interface {
	FindByChannelUser(ctx context.Context, confID uint, channelType, channelUserID string) (BindingInfo, bool, error)
}

// BindCodeConsumer consumes a /bind <code> request. Implemented by app.BindingService.
// confID 是消息来源的通知通道配置，绑定码只能在签发它的那个通道上兑换。
type BindCodeConsumer interface {
	ConsumeCode(ctx context.Context, code string, confID uint, channelType, channelUserID string) error
}

// AuditRecorder writes a single audit row. Implemented by app.AuditService.
type AuditRecorder interface {
	Record(ctx context.Context, e AuditEntry) error
}

// AssistantHandler 接管绑定用户的非命令消息（内置 AI 助手，路线图 M19）。Handle 只登记、立即返回：
// 回答由助手在后台经 Replier 异步发出，不在入站路径上等模型（Telegram 的长轮询是同步调用 Process 的）。
type AssistantHandler interface {
	// Enabled 报告现在要不要把非命令消息交给助手（助手启用、ChatOps 开关打开）。
	Enabled(ctx context.Context) bool
	// Handle 处理一条非命令消息，返回立即回复（可以为空）与审计结果。
	Handle(ctx context.Context, msg InboundAssistantMessage) (Reply, string)
}

// InboundAssistantMessage 是交给助手的一条消息：原始入站消息（回复要用）与绑定信息。
type InboundAssistantMessage struct {
	Msg notify.InboundMessage
	Src Source
}

// assistantRef 包一层，好放进 atomic.Pointer（链在通道启动以后才接上助手）。
type assistantRef struct{ h AssistantHandler }

// MessageChain dispatches inbound messages through the permission gate per
// plan T22 (binding → allowed → command parse → session → registry →
// ratelimit → admin check → handler).
type MessageChain struct {
	registry    CommandRegistryAPI
	bindings    BindingLookup
	bindCoder   BindCodeConsumer
	auditSvc    AuditRecorder
	rateLimiter RateLimiterAPI
	sessions    SessionStoreAPI
	replier     Replier
	now         func() time.Time
	logf        func(format string, args ...any)
	assistant   atomic.Pointer[assistantRef]
}

// SetAssistant 接上内置 AI 助手（nil 摘掉）：之后绑定用户的非命令消息交给它。可以在通道运行中调用。
func (mc *MessageChain) SetAssistant(h AssistantHandler) {
	if h == nil {
		mc.assistant.Store(nil)
		return
	}
	mc.assistant.Store(&assistantRef{h: h})
}

// SetLogf 注入日志函数，用于记录审计写入失败。
func (mc *MessageChain) SetLogf(fn func(format string, args ...any)) { mc.logf = fn }

// NewMessageChain wires the dependencies. replier may be nil when there is
// no outbound channel (e.g. CLI debugging).
func NewMessageChain(
	registry CommandRegistryAPI,
	bindings BindingLookup,
	bindCoder BindCodeConsumer,
	audit AuditRecorder,
	rl RateLimiterAPI,
	sessions SessionStoreAPI,
	replier Replier,
) *MessageChain {
	return &MessageChain{
		registry:    registry,
		bindings:    bindings,
		bindCoder:   bindCoder,
		auditSvc:    audit,
		rateLimiter: rl,
		sessions:    sessions,
		replier:     replier,
		now:         time.Now,
	}
}

// Process is the inbound entry point. It returns an error only on
// unrecoverable conditions (lookup failure, nil handler); business denies
// are surfaced via audit + reply, never as errors.
func (mc *MessageChain) Process(ctx context.Context, msg notify.InboundMessage) error {
	start := mc.now()

	text := strings.TrimSpace(msg.Text)
	cmdName, args := parseCommand(text)

	binding, hasBinding, err := mc.bindings.FindByChannelUser(ctx, msg.SourceConfID, msg.ChannelType, msg.ChannelUserID)
	if err != nil {
		mc.recordAudit(ctx, msg, 0, cmdName, args, "error:lookup_binding", start)
		mc.tryReply(ctx, msg, Reply{Text: "内部错误，请稍后重试"})
		return err
	}

	if !hasBinding {
		if cmdName == "bind" {
			return mc.executeBind(ctx, msg, args, start)
		}
		mc.recordAudit(ctx, msg, 0, cmdName, args, "denied:not_bound", start)
		mc.tryReply(ctx, msg, Reply{Text: "请先 /bind <绑定码> 完成绑定"})
		return nil
	}

	if !binding.Allowed {
		mc.recordAudit(ctx, msg, binding.ConfID, cmdName, args, "denied:not_allowed", start)
		mc.tryReply(ctx, msg, Reply{Text: "该绑定已被禁用"})
		return nil
	}

	src := Source{
		ChannelType:   msg.ChannelType,
		ChannelConfID: binding.ConfID,
		ChannelUserID: msg.ChannelUserID,
		Username:      msg.Username,
		ChatID:        msg.ChatID,
		ReplyLang:     binding.ReplyLang,
		PtAdmin:       binding.PtAdmin,
		IsAdmin:       binding.PtAdmin,
		BindingID:     binding.ID,
	}

	if state, ok := mc.sessions.Pending(msg.ChannelType, binding.ConfID, msg.ChannelUserID); ok {
		mc.sessions.Clear(msg.ChannelType, binding.ConfID, msg.ChannelUserID)
		if state.Handler != nil {
			reply, herr := state.Handler(ctx, []string{text}, src)
			result := "success"
			switch {
			case herr != nil:
				result = "error:session_handler"
			case reply.Failed:
				result = "error:session_failed"
			}
			mc.recordAudit(ctx, msg, binding.ConfID, "session:"+state.Step, args, result, start)
			mc.tryReply(ctx, msg, reply)
			return nil
		}
	}

	if cmdName == "" {
		// 按钮回调（RSS 通知的下载 / 忽略）由通道自己处理，不当成和助手说话
		if ref := mc.assistant.Load(); ref != nil && !msg.IsCallback && ref.h.Enabled(ctx) {
			// 消息原文不进审计（可能很长、也可能带私事）；工具调用由助手另记
			if !mc.rateLimiter.Allow(msg.ChannelType, msg.ChannelUserID, "assistant") {
				mc.recordAudit(ctx, msg, binding.ConfID, "assistant", nil, "denied:rate_limit", start)
				return nil
			}
			reply, result := ref.h.Handle(ctx, InboundAssistantMessage{Msg: msg, Src: src})
			mc.recordAudit(ctx, msg, binding.ConfID, "assistant", nil, result, start)
			mc.tryReply(ctx, msg, reply)
			return nil
		}
		mc.recordAudit(ctx, msg, binding.ConfID, "", nil, "user_message_hinted", start)
		mc.tryReply(ctx, msg, Reply{
			Text: "🤖 我只识别命令消息（以 / 开头）。\n\n发送 /help 查看支持的命令清单。",
		})
		return nil
	}

	spec, ok := mc.registry.Get(cmdName)
	if !ok {
		mc.recordAudit(ctx, msg, binding.ConfID, cmdName, args, "denied:unknown_command", start)
		mc.tryReply(ctx, msg, Reply{Text: "命令未知，发送 /help 查看"})
		return nil
	}

	if !mc.rateLimiter.Allow(msg.ChannelType, msg.ChannelUserID, spec.Name) {
		mc.recordAudit(ctx, msg, binding.ConfID, spec.Name, args, "denied:rate_limit", start)
		return nil
	}

	if spec.AdminOnly && !binding.PtAdmin {
		mc.recordAudit(ctx, msg, binding.ConfID, spec.Name, args, "denied:not_admin", start)
		mc.tryReply(ctx, msg, Reply{Text: "需管理员权限"})
		return nil
	}

	if spec.Handler == nil {
		mc.recordAudit(ctx, msg, binding.ConfID, spec.Name, args, "error:nil_handler", start)
		return errors.New("command handler is nil: " + spec.Name)
	}
	reply, herr := spec.Handler(ctx, args, src)
	result := "success"
	switch {
	case herr != nil:
		result = "error:handler"
	case reply.Failed:
		// 处理器把业务错误写进回复、返回 nil error：按回复的 Failed 记失败
		result = "error:command_failed"
	}
	mc.recordAudit(ctx, msg, binding.ConfID, spec.Name, args, result, start)
	mc.tryReply(ctx, msg, reply)
	return nil
}

func (mc *MessageChain) executeBind(ctx context.Context, msg notify.InboundMessage, args []string, start time.Time) error {
	if len(args) == 0 {
		mc.recordAudit(ctx, msg, 0, "bind", args, "denied:missing_code", start)
		mc.tryReply(ctx, msg, Reply{Text: "用法: /bind <绑定码>"})
		return nil
	}
	code := args[0]
	if mc.bindCoder == nil {
		mc.recordAudit(ctx, msg, 0, "bind", args, "error:no_binding_service", start)
		return errors.New("binding service unavailable")
	}
	if err := mc.bindCoder.ConsumeCode(ctx, code, msg.SourceConfID, msg.ChannelType, msg.ChannelUserID); err != nil {
		mc.recordAudit(ctx, msg, 0, "bind", args, "error:bind_failed", start)
		mc.tryReply(ctx, msg, Reply{Text: "绑定失败：" + err.Error()})
		return nil
	}
	mc.recordAudit(ctx, msg, 0, "bind", args, "success", start)
	mc.tryReply(ctx, msg, Reply{Text: "绑定成功"})
	return nil
}

func (mc *MessageChain) recordAudit(ctx context.Context, msg notify.InboundMessage, confID uint, command string, args []string, result string, start time.Time) {
	if mc.auditSvc == nil {
		return
	}
	latency := mc.now().Sub(start).Milliseconds()
	argsMap := map[string]any{}
	if len(args) > 0 {
		argsMap["args"] = auditArgs(command, args)
	}
	if err := mc.auditSvc.Record(ctx, AuditEntry{
		NotificationConfID: confID,
		ChannelType:        msg.ChannelType,
		ChannelUserID:      msg.ChannelUserID,
		Command:            command,
		Args:               argsMap,
		Result:             result,
		LatencyMs:          latency,
	}); err != nil && mc.logf != nil {
		// 命令可能已经生效（比如已经暂停了种子），审计却没写进去：至少留一条日志
		mc.logf("ChatOps 审计写入失败 command=%s result=%s channel=%s user=%s: %v",
			command, result, msg.ChannelType, msg.ChannelUserID, err)
	}
}

// auditArgs 返回写进审计的参数副本：/bind 的绑定码整体隐去，像 URL 的参数（如 /addrss 带的 RSS 地址）
// 把 passkey、token 等查询参数脱敏。审计的键名脱敏只看键名，管不到位置参数里的内容。
func auditArgs(command string, args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		switch {
		case command == "bind":
			out[i] = "[REDACTED]"
		case strings.Contains(a, "://"):
			out[i] = utils.SanitizeURL(a)
		default:
			out[i] = a
		}
	}
	return out
}

func (mc *MessageChain) tryReply(ctx context.Context, msg notify.InboundMessage, reply Reply) {
	if mc.replier == nil || reply.SilentDrop {
		return
	}
	if reply.Text == "" && len(reply.Buttons) == 0 {
		return
	}
	_ = mc.replier.Reply(ctx, msg, reply)
}

func parseCommand(text string) (string, []string) {
	if !strings.HasPrefix(text, "/") {
		return "", nil
	}
	parts := strings.Fields(text[1:])
	if len(parts) == 0 {
		return "", nil
	}
	name := strings.ToLower(parts[0])
	if at := strings.Index(name, "@"); at >= 0 {
		name = name[:at]
	}
	return name, parts[1:]
}
